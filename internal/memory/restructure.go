package memory

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// The restructuring operations (docs/spec/agent-memory.md Part III.8): the
// verbs a write cannot express. Each is one call under D28's rules: it
// holds the write lock from before its fresh load until after its reload,
// replaces notes atomically, and leaves the store as it was when it fails.

// NoNodeError is a restructuring call naming a stem that is not a loaded
// node. Stems lists every loaded stem, sorted, so a caller can suggest
// near matches.
type NoNodeError struct {
	Stem  string
	Stems []string
}

func (e *NoNodeError) Error() string { return fmt.Sprintf("no node %q", e.Stem) }

func noNode(st *Store, stem string) *NoNodeError {
	e := &NoNodeError{Stem: stem}
	for _, n := range st.order {
		e.Stems = append(e.Stems, n.Stem)
	}
	sort.Strings(e.Stems)
	return e
}

// locked takes the write lock and loads the store fresh under it. The
// caller releases the lock when its reload (or roll back) is done.
func (w *Writer) locked() (*Store, func(), error) {
	unlock, err := w.lock()
	if err != nil {
		return nil, nil, err
	}
	if testHookLocked != nil {
		testHookLocked()
	}
	return Load(w.dir), unlock, nil
}

// reload loads the store again after a change, for the postcondition
// check. rel names the node the call was about.
func (w *Writer) reload(rel string) *Store {
	after := Load(w.dir)
	if testHookReadBack != nil {
		testHookReadBack(after, rel)
	}
	return after
}

// fileText is the folder note's bytes as read, byte-order mark included.
func (n *Node) fileText() string {
	if n.BOM {
		return bomText + n.Source
	}
	return n.Source
}

// newErrors returns the error-severity problems in after that before did
// not have, counted as a multiset: III.8's postcondition is that there are
// none.
func newErrors(before, after *Store) []Problem {
	left := map[Problem]int{}
	for _, p := range before.problems {
		if p.Severity == SeverityError {
			left[p]++
		}
	}
	var out []Problem
	for _, p := range after.problems {
		if p.Severity != SeverityError {
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

// newErrorErr is the failure a call reports after rolling back because the
// store would have had a new error. rollbackErr is the roll back's own
// failure, if any.
func newErrorErr(p Problem, what string, rollbackErr error) error {
	if rollbackErr != nil {
		return fmt.Errorf("%s would leave the store with a new error (%s: %s), and rolling it back failed: %v", what, p.Path, p.Reason, rollbackErr)
	}
	return fmt.Errorf("%s would leave the store with a new error (%s: %s); it was rolled back and nothing changed", what, p.Path, p.Reason)
}

// noteEdit is one loaded node's folder note and the bytes to replace it
// with.
type noteEdit struct {
	node *Node
	data []byte
}

// replacedNote is a folder note replaceNotes has replaced, and what it
// takes to put it back.
type replacedNote struct {
	target string
	rel    string
	orig   []byte
	data   []byte
	perm   fs.FileMode
}

// replaceNotes replaces each edited folder note atomically, refusing one
// that changed on disk since before was loaded, then reloads the store.
// If a replace fails, or the reload shows an error before did not have,
// every note already replaced is restored and an error returned: the store
// is as it was. A note found holding other bytes at the reload was edited
// by something that does not take the lock: that is ErrChangedOnDisk, and
// nothing is rolled back over the other content (D28). what names the call
// in errors. On success it returns the reloaded store.
func (w *Writer) replaceNotes(before *Store, edits []noteEdit, what string) (*Store, error) {
	done := make([]replacedNote, 0, len(edits))
	for _, ed := range edits {
		r, err := w.replaceNote(ed)
		if err != nil {
			if rerr := restoreNotes(done); rerr != nil {
				return nil, fmt.Errorf("%w; restoring the notes already replaced failed: %v", err, rerr)
			}
			return nil, err
		}
		done = append(done, r)
	}
	after := w.reload(edits[0].node.Path)
	for _, r := range done {
		if cur, err := os.ReadFile(r.target); err != nil || !bytes.Equal(cur, r.data) {
			return nil, changedErr(r.rel, "after this call replaced it; the other content is kept, not rolled back")
		}
	}
	if errs := newErrors(before, after); len(errs) > 0 {
		return nil, newErrorErr(errs[0], what, restoreNotes(done))
	}
	return after, nil
}

// restoreNotes puts back the bytes each replaced note had, newest first,
// and returns the first failure.
func restoreNotes(done []replacedNote) error {
	var first error
	for i := len(done) - 1; i >= 0; i-- {
		if err := replaceFile(done[i].target, done[i].orig, done[i].perm); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// replaceNote replaces one folder note, through a symlinked note to its
// target, keeping the file's mode.
func (w *Writer) replaceNote(ed noteEdit) (replacedNote, error) {
	n := ed.node
	target, perm, err := w.noteTarget(n)
	if err != nil {
		return replacedNote{}, err
	}
	orig := n.fileText()
	unchanged := func() bool {
		cur, err := os.ReadFile(target)
		return err == nil && string(cur) == orig
	}
	if err := commit(target, ed.data, perm, unchanged, func() {}); err != nil {
		return replacedNote{}, wrapCommit(err, n.Path, "since this call read it")
	}
	return replacedNote{target: target, rel: n.Path, orig: []byte(orig), data: ed.data, perm: perm}, nil
}

// bodyLines splits a loaded node's source into its lines, each with its
// own ending, and returns them with the index of the first body line and
// the body's blocks as runs of those lines (relative to that index): run i
// is n.Blocks[i].
func bodyLines(n *Node) (lines []string, bodyStart int, runs []lineRun, err error) {
	_, body, closing, err := splitFrontmatter(normalise(n.Source))
	if err == nil {
		runs = splitRuns(body)
		if len(runs) != len(n.Blocks) {
			err = fmt.Errorf("%d paragraphs for %d blocks", len(runs), len(n.Blocks))
		}
	}
	if err != nil {
		return nil, 0, nil, fmt.Errorf("node %s no longer parses as loaded: %w", n.Stem, err)
	}
	return strings.SplitAfter(n.Source, "\n"), closing + 1, runs, nil
}

// expectationOf is what n's note must load back as when nothing in it is
// changed: a restructuring call adjusts the fields it changes.
func expectationOf(n *Node) (expectation, error) {
	want := expectation{description: n.Description, tags: n.Tags, props: n.Properties}
	for _, b := range n.Blocks {
		want.blocks = append(want.blocks, Block{ID: b.ID, Text: b.Text})
	}
	_, warnings, err := parseNote([]byte(n.Source))
	if err != nil {
		return want, fmt.Errorf("node %s no longer parses as loaded: %w", n.Stem, err)
	}
	want.warnings = warnings
	return want, nil
}
