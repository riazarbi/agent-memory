package memory

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WriteRequest is one memory_write (docs/spec/agent-memory.md §4.5): the
// stem names the node; a loaded stem is an update, any other a create.
type WriteRequest struct {
	Stem string
	// Parent is the stem of the node to create under, or RootParent to
	// create a first-level node; empty means not given. An update accepts
	// it only if it names the node's parent (RootParent for a first-level
	// node).
	Parent string
	// Description is required to create; on an update, if non-nil, it
	// replaces the description.
	Description *string
	// Tags, if non-nil, replaces the whole tag list.
	Tags *[]string
	// Properties are merged into the frontmatter.
	Properties map[string]any
	// Blocks are appended (no ID) or replace the block with that ID.
	Blocks []BlockEdit
}

// WriteResult is a successful write.
type WriteResult struct {
	// Node is the written node as the read-back loaded it from disk.
	Node *Node
	// Created reports whether the write created the node.
	Created bool
	// BlockIDs holds one ID per requested block, in request order.
	BlockIDs []string
	// Adopted holds the IDs given to ID-less operator paragraphs.
	Adopted []string
	// Store is the store Node was loaded in: the read-back's, or the
	// write's own fresh load when the write changed nothing. It answers
	// the node's backlinks as of this write.
	Store *Store
}

var (
	// ErrBusy is returned when another write held the lock for the whole
	// wait; nothing was written.
	ErrBusy = errors.New("memory is busy — retry")
	// ErrChangedOnDisk is returned when the folder note changed on disk
	// while the write ran, by an editor that does not take the lock. A
	// refusal before the rename wrote nothing; one found at the read-back
	// leaves the other content in place.
	ErrChangedOnDisk = errors.New("changed on disk — retry")
)

// RootParent is the root, notes/ itself, as a write's parent (spec III.6):
// a create under it makes a first-level node. It is never a stem.
const RootParent = "/"

// lockName is the lock file every write holds (D28), a dot-entry so the
// reader and Obsidian ignore it. It is never removed: removing a lock file
// lets two writers lock two different files.
const lockName = ".write.lock"

// Modes a create sets explicitly, whatever the umask (D28).
const (
	folderMode fs.FileMode = 0o755
	noteMode   fs.FileMode = 0o644
)

// Test seams. Production never sets them; tests use them to make the
// rename fail, to edit files mid-write, and to fail the read-back.
var (
	// lockWait is how long a write waits for the lock (D28: 10 s).
	lockWait = 10 * time.Second
	// renameFile replaces a folder note with its temporary file.
	renameFile = os.Rename
	// testHookLocked runs once the write holds the lock.
	testHookLocked func()
	// testHookBeforeRename runs after the temporary file is written and
	// before the edit-race check, with the folder note's path.
	testHookBeforeRename func(notePath string)
	// testHookAfterRename runs between the rename and the read-back.
	testHookAfterRename func(notePath string)
	// testHookReadBack sees the read-back's store and the written node's
	// path before it is judged.
	testHookReadBack func(s *Store, rel string)
)

// Writer executes writes against one notes/ directory (D28). There is one
// per directory per process, shared by every caller: get it with
// WriterFor.
type Writer struct {
	dir string
	// slot serializes this process's writes before they contend for the
	// OS lock, which serializes writes across processes.
	slot chan struct{}
}

var (
	writersMu sync.Mutex
	writers   = map[string]*Writer{}
)

// WriterFor returns the process's writer for the notes/ directory at
// notesDir, creating it on first use. Every spelling of the same absolute
// path shares one writer.
func WriterFor(notesDir string) (*Writer, error) {
	abs, err := filepath.Abs(notesDir)
	if err != nil {
		return nil, fmt.Errorf("memory notes directory %q: %w", notesDir, err)
	}
	writersMu.Lock()
	defer writersMu.Unlock()
	w, ok := writers[abs]
	if !ok {
		w = &Writer{dir: abs, slot: make(chan struct{}, 1)}
		writers[abs] = w
	}
	return w, nil
}

// Dir returns the absolute notes/ directory this writer serves.
func (w *Writer) Dir() string { return w.dir }

// Write executes one write. On success the node is on disk exactly as
// encoded and loads; on failure the store is as it was, except where an
// editor that ignores the lock changed it (ErrChangedOnDisk).
func (w *Writer) Write(req WriteRequest) (WriteResult, error) {
	if !validStem(req.Stem) {
		return WriteResult{}, fieldErr("stem", "%q is not a valid stem: use lowercase letters, digits and single hyphens (e.g. outbox-etiquette), at most %d characters", req.Stem, maxStemLen)
	}
	unlock, err := w.lock()
	if err != nil {
		return WriteResult{}, err
	}
	defer unlock()
	if testHookLocked != nil {
		testHookLocked()
	}
	st := Load(w.dir)
	if n, ok := st.Node(req.Stem); ok {
		return w.update(st, n, req)
	}
	return w.create(st, req)
}

// lock takes this process's slot, then the OS lock, within one bounded
// wait.
func (w *Writer) lock() (func(), error) {
	deadline := time.Now().Add(lockWait)
	timer := time.NewTimer(lockWait)
	defer timer.Stop()
	select {
	case w.slot <- struct{}{}:
	case <-timer.C:
		return nil, ErrBusy
	}
	release, err := lockFile(filepath.Join(w.dir, lockName), deadline)
	if err != nil {
		<-w.slot
		return nil, err
	}
	return func() {
		release()
		<-w.slot
	}, nil
}

// update rewrites a loaded node through the encoder.
func (w *Writer) update(st *Store, n *Node, req WriteRequest) (WriteResult, error) {
	if err := checkUpdateParent(n, req.Parent); err != nil {
		return WriteResult{}, err
	}
	enc, err := Encoder{Store: st}.EncodeChange(n.Stem, Change{
		Description: req.Description, Tags: req.Tags, Properties: req.Properties, Blocks: req.Blocks,
	})
	if err != nil {
		return WriteResult{}, err
	}
	orig := n.Source
	if n.BOM {
		orig = bomText + orig
	}
	if string(enc.Bytes) == orig {
		// Nothing to change: the file is left alone, mtime included.
		return WriteResult{Node: n, BlockIDs: enc.BlockIDs, Adopted: enc.Adopted, Store: st}, nil
	}
	target, perm, err := w.noteTarget(n)
	if err != nil {
		return WriteResult{}, err
	}
	unchanged := func() bool {
		cur, err := os.ReadFile(target) //nolint:gosec // a folder note under notes/
		return err == nil && string(cur) == orig
	}
	if err := commit(target, enc.Bytes, perm, unchanged, func() {}); err != nil {
		return WriteResult{}, wrapCommit(err, n.Path, "since this write read it")
	}
	rollback := func() error { return replaceFile(target, []byte(orig), perm) }
	return w.readBack(st, n.Path, n.Stem, target, enc, false, rollback)
}

// noteTarget returns the file a node's folder note is written to, and its
// mode. A symlinked folder note is written through to its target, so the
// link stays a link and a bound shared node stays shared.
func (w *Writer) noteTarget(n *Node) (string, fs.FileMode, error) {
	target, err := filepath.EvalSymlinks(w.notePath(n.Path))
	if err != nil {
		return "", 0, fmt.Errorf("cannot resolve the folder note of %s: %w", n.Path, err)
	}
	info, err := os.Stat(target) //nolint:gosec // a folder note under notes/
	if err != nil {
		return "", 0, fmt.Errorf("cannot stat the folder note of %s: %w", n.Path, err)
	}
	return target, info.Mode().Perm(), nil
}

// errRaced marks a commit refused by its edit-race check.
var errRaced = errors.New("raced")

// commit writes data to a temporary file beside dest, then - unless
// unchanged reports that dest changed meanwhile (errRaced) - renames it
// over dest. On any failure the temporary file is removed and cleanup
// runs; nothing else is touched.
func commit(dest string, data []byte, perm fs.FileMode, unchanged func() bool, cleanup func()) error {
	tmp, err := writeTemp(dest, data, perm)
	if err != nil {
		cleanup()
		return err
	}
	if testHookBeforeRename != nil {
		testHookBeforeRename(dest)
	}
	if !unchanged() {
		_ = os.Remove(tmp)
		cleanup()
		return errRaced
	}
	if err := renameFile(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		cleanup()
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return nil
}

// wrapCommit turns a commit's edit-race refusal into ErrChangedOnDisk.
func wrapCommit(err error, rel, when string) error {
	if errors.Is(err, errRaced) {
		return changedErr(rel, when+"; nothing was written")
	}
	return err
}

// checkUpdateParent refuses a parent that is given and is not the node's:
// a first-level node's parent is the root, RootParent.
func checkUpdateParent(n *Node, parent string) error {
	if parent == "" {
		return nil
	}
	if p := n.Parent(); p != nil {
		if p.Stem == parent {
			return nil
		}
		return fieldErr("parent", "node %q already exists under %q, so this write is an update: omit parent or give %q (moving a node is not a write)", n.Stem, p.Stem, p.Stem)
	}
	if parent == RootParent {
		return nil
	}
	return fieldErr("parent", "node %q already exists at the top level, so this write is an update: omit parent or give %s (moving a node is not a write); %q is not its parent", n.Stem, RootParent, parent)
}

// create makes a new node under a loaded parent, or under the root.
func (w *Writer) create(st *Store, req WriteRequest) (WriteResult, error) {
	parentPath, err := w.checkCreate(st, req)
	if err != nil {
		return WriteResult{}, err
	}
	nn, err := newNodeOf(req)
	if err != nil {
		return WriteResult{}, err
	}
	enc, err := Encoder{Store: st}.EncodeNew(nn)
	if err != nil {
		return WriteResult{}, err
	}
	rel := path.Join(parentPath, req.Stem)
	rollback, err := w.makeNode(rel, enc.Bytes)
	if err != nil {
		return WriteResult{}, err
	}
	return w.readBack(st, rel, req.Stem, w.notePath(rel), enc, true, rollback)
}

// makeNode makes the folder of a new node at rel (mode 0755) and writes
// data as its folder note (mode 0644). On failure nothing is left. On
// success it returns the roll back: the note, then the folder, removed.
func (w *Writer) makeNode(rel string, data []byte) (rollback func() error, err error) {
	folder := filepath.Join(w.dir, filepath.FromSlash(rel))
	notePath := w.notePath(rel)
	if err := makeFolder(folder, rel); err != nil {
		return nil, err
	}
	// Removing the folder fails, leaving it, if anything else is in it.
	removeFolder := func() error { return os.Remove(folder) }
	absent := func() bool {
		_, err := os.Lstat(notePath)
		return errors.Is(err, fs.ErrNotExist)
	}
	if err := commit(notePath, data, noteMode, absent, func() { _ = removeFolder() }); err != nil {
		return nil, wrapCommit(err, rel, "(its folder note appeared while this write ran)")
	}
	return func() error {
		if err := os.Remove(notePath); err != nil {
			return err
		}
		return removeFolder()
	}, nil
}

// newNodeOf is a create's request as the encoder takes it. A new node has
// no blocks yet, so no block edit may name an ID.
func newNodeOf(req WriteRequest) (NewNode, error) {
	nn := NewNode{Description: *req.Description, Properties: req.Properties, Blocks: make([]string, len(req.Blocks))}
	if req.Tags != nil {
		nn.Tags = *req.Tags
	}
	for i, b := range req.Blocks {
		if b.ID != "" {
			return nn, blockIDErr(i, "node %q does not exist yet, so it has no block ^%s; omit the id to add a new block", req.Stem, b.ID)
		}
		nn.Blocks[i] = b.Text
	}
	return nn, nil
}

// makeFolder creates a new node's folder with mode 0755, whatever the
// umask.
func makeFolder(folder, rel string) error {
	if err := os.Mkdir(folder, folderMode); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return changedErr(rel, "(the folder appeared since this write loaded the store); nothing was written")
		}
		return fmt.Errorf("creating %s: %w", rel, err)
	}
	if err := os.Chmod(folder, folderMode); err != nil {
		_ = os.Remove(folder)
		return fmt.Errorf("creating %s: %w", rel, err)
	}
	return nil
}

// checkCreate refuses a create that cannot succeed, and returns the
// parent's path relative to notes/: "" for the root (RootParent), whose
// folder is notes/ itself.
func (w *Writer) checkCreate(st *Store, req WriteRequest) (string, error) {
	exact, folded := w.findFolders(req.Stem)
	if exact != "" {
		return "", fmt.Errorf("stem %q is taken by the folder %s, which the reader skipped (%s); fix or remove that folder first - the write cannot use the stem until then, and the folder is untouched", req.Stem, exact, problemFor(st, exact))
	}
	if folded != "" {
		return "", fieldErr("stem", "%q collides by letter case with the folder %s; stems are unique store-wide and a case-insensitive filesystem cannot hold both - choose another stem", req.Stem, folded)
	}
	if req.Parent == "" {
		return "", fieldErr("parent", "is required to create node %q: name the existing node to create it under, or give %s to create it at the top level", req.Stem, RootParent)
	}
	parentPath := ""
	if req.Parent != RootParent {
		pn, ok := st.Node(req.Parent)
		if !ok {
			return "", fieldErr("parent", "no node %q exists to create %q under; give the stem of an existing node, or %s for the top level", req.Parent, req.Stem, RootParent)
		}
		parentPath = pn.Path
	}
	if req.Description == nil {
		return "", fieldErr("description", "is required to create node %q: one line saying what the node holds", req.Stem)
	}
	rel := path.Join(parentPath, req.Stem)
	if _, err := os.Lstat(filepath.Join(w.dir, filepath.FromSlash(rel))); err == nil {
		return "", fmt.Errorf("%s already exists and is not a node folder; remove it or choose another stem", rel)
	}
	return parentPath, nil
}

// folderSearch is one walk of findFolders.
type folderSearch struct {
	stem          string
	exact, folded string
	seen          []os.FileInfo
}

// findFolders walks every folder under notes/ - into subtrees the reader
// skipped, and through symlinked directories - and returns the first
// folder named exactly stem and the first whose name differs from stem only
// by letter case, as paths relative to notes/. Dot-entries are ignored,
// as the reader ignores them. Each real directory is walked once.
func (w *Writer) findFolders(stem string) (exact, folded string) {
	s := &folderSearch{stem: stem}
	s.walk(w.dir, "")
	return s.exact, s.folded
}

func (s *folderSearch) walk(abs, rel string) {
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() || s.walked(info) {
		return
	}
	s.seen = append(s.seen, info)
	entries, err := os.ReadDir(abs)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		childAbs, childRel := filepath.Join(abs, name), path.Join(rel, name)
		if strings.HasPrefix(name, ".") || !isDir(childAbs) {
			continue
		}
		s.match(name, childRel)
		s.walk(childAbs, childRel)
	}
}

// match records the folder at rel if its name is the stem, exactly or
// up to letter case.
func (s *folderSearch) match(name, rel string) {
	switch {
	case name == s.stem:
		if s.exact == "" {
			s.exact = rel
		}
	case strings.EqualFold(name, s.stem):
		if s.folded == "" {
			s.folded = rel
		}
	}
}

func (s *folderSearch) walked(info os.FileInfo) bool {
	for _, prev := range s.seen {
		if os.SameFile(prev, info) {
			return true
		}
	}
	return false
}

// isDir reports whether p is a directory, following a symlink.
func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// problemFor quotes the reader's error for the folder at rel, or for the
// nearest skipped folder above it.
func problemFor(st *Store, rel string) string {
	for p := rel; p != "." && p != "/"; p = path.Dir(p) {
		for _, pr := range st.Problems() {
			if pr.Path == p && pr.Severity == SeverityError {
				return pr.Path + ": " + pr.Reason
			}
		}
	}
	return "no problem reported"
}

// readBack loads the store again after the rename (D28). Bytes on disk
// that are not this write's mean another editor got there in between: the
// write reports ErrChangedOnDisk and does not roll back over them. A node
// that does not load, or has a new problem at its path (dangling links
// excepted), is rolled back.
func (w *Writer) readBack(before *Store, rel, stem, notePath string, enc Encoded, created bool, rollback func() error) (WriteResult, error) {
	if testHookAfterRename != nil {
		testHookAfterRename(notePath)
	}
	after := Load(w.dir)
	if testHookReadBack != nil {
		testHookReadBack(after, rel)
	}
	if cur, err := os.ReadFile(notePath); err != nil || !bytes.Equal(cur, enc.Bytes) { //nolint:gosec // a folder note under notes/
		return WriteResult{}, changedErr(rel, "after this write replaced it; the other content is kept, not rolled back")
	}
	n, ok := after.Node(stem)
	var problems []Problem
	if ok && n.Path == rel {
		problems = newProblems(before, after, rel, n)
	} else {
		problems = []Problem{{Severity: SeverityError, Path: rel, Reason: "the node did not load"}}
	}
	if len(problems) > 0 {
		reason := problems[0].Reason
		if err := rollback(); err != nil {
			return WriteResult{}, fmt.Errorf("written node %s did not load back cleanly (%s), and rolling it back failed: %v", rel, reason, err)
		}
		return WriteResult{}, fmt.Errorf("written node %s did not load back cleanly (%s); the write was rolled back", rel, reason)
	}
	return WriteResult{Node: n, Created: created, BlockIDs: enc.BlockIDs, Adopted: enc.Adopted, Store: after}, nil
}

// newProblems returns the problems at rel in after that were not there
// before, leaving out the warnings for n's dangling links (D27 lets a write
// add them). Problems are counted as a multiset.
func newProblems(before, after *Store, rel string, n *Node) []Problem {
	left := map[Problem]int{}
	for _, p := range before.Problems() {
		if p.Path == rel {
			left[p]++
		}
	}
	for _, e := range n.Edges() {
		if e.Dangling {
			left[Problem{Severity: SeverityWarning, Path: rel, Reason: danglingReason(e)}]++
		}
	}
	var out []Problem
	for _, p := range after.Problems() {
		if p.Path != rel {
			continue
		}
		if left[p] > 0 {
			left[p]--
			continue
		}
		out = append(out, p)
	}
	return out
}

func changedErr(rel, detail string) error {
	return fmt.Errorf("%w: %s changed on disk %s", ErrChangedOnDisk, rel, detail)
}

// notePath returns the folder note path of the node folder at rel.
func (w *Writer) notePath(rel string) string {
	return filepath.Join(w.dir, filepath.FromSlash(rel), path.Base(rel)+".md")
}

// writeTemp writes data to a new hidden temporary file beside dest, synced
// and with mode perm, and returns its path. On error nothing is left.
func writeTemp(dest string, data []byte, perm fs.FileMode) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", dest, err)
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, perm)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("writing %s: %w", dest, err)
	}
	return tmp, nil
}

// replaceFile atomically replaces dest with data, mode perm.
func replaceFile(dest string, data []byte, perm fs.FileMode) error {
	tmp, err := writeTemp(dest, data, perm)
	if err != nil {
		return err
	}
	if err := renameFile(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
