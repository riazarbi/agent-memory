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
	done, err := w.replaceAll(edits)
	if err != nil {
		return nil, err
	}
	after := w.reload(edits[0].node.Path)
	if err := keptBytes(done); err != nil {
		return nil, err
	}
	if errs := newErrors(before, after); len(errs) > 0 {
		return nil, newErrorErr(errs[0], what, restoreNotes(done))
	}
	return after, nil
}

// keptBytes checks, after the reload, that every replaced note still holds
// the bytes this call wrote. One that does not was edited by something
// that does not take the lock: that is ErrChangedOnDisk, and the caller
// rolls nothing back over the other content (D28).
func keptBytes(done []replacedNote) error {
	for _, r := range done {
		if cur, err := os.ReadFile(r.target); err != nil || !bytes.Equal(cur, r.data) {
			return changedErr(r.rel, "after this call replaced it; the other content is kept, not rolled back")
		}
	}
	return nil
}

// replaceAll replaces each edited folder note atomically, in order,
// refusing one that changed on disk since it was loaded. If a replace
// fails, every note already replaced is restored and the error returned.
// On success it returns what restoreNotes needs to undo them all.
func (w *Writer) replaceAll(edits []noteEdit) ([]replacedNote, error) {
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
	return done, nil
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

// noBlock is noteChange.drop when no block is removed.
const noBlock = -1

// noteChange is what a restructuring call does to one loaded folder note,
// by byte surgery: every byte it does not name stays, so line endings, a
// byte-order mark, ID-less paragraphs, runs of blank lines and the
// frontmatter are as they were.
type noteChange struct {
	// cuts are the changes to each block's text, by block; nil for none.
	cuts [][]cut
	// drop is the block to remove, with the blank lines that separated
	// it, or noBlock.
	drop int
	// add is a block (ID and Text) to append as the last paragraph, or
	// nil.
	add *Block
}

// rewritten returns n's folder note with ch applied. The output is loaded
// through the folder-note parser before it is returned: it must read back
// as the note it was, changed exactly as asked.
func rewritten(n *Node, ch noteChange) ([]byte, error) {
	lines, bodyStart, runs, err := bodyLines(n)
	if err != nil {
		return nil, err
	}
	want, err := expectationOf(n)
	if err != nil {
		return nil, err
	}
	body := append([]string(nil), lines[bodyStart:]...)
	for i, cuts := range ch.cuts {
		if len(cuts) == 0 || i == ch.drop {
			continue
		}
		cutBlock(body, runs[i], n.Blocks[i], cuts)
		want.blocks[i].Text = applyCuts(n.Blocks[i].Text, cuts)
	}
	if ch.drop != noBlock {
		from, to := dropRange(runs, ch.drop)
		body = append(body[:from:from], body[to:]...)
		want.blocks = append(want.blocks[:ch.drop:ch.drop], want.blocks[ch.drop+1:]...)
	}
	head := strings.Join(lines[:bodyStart], "")
	out := head + strings.Join(body, "")
	if ch.add != nil {
		out = withLastBlock(head, out, len(want.blocks) > 0, n.LineEnding, *ch.add)
		want.blocks = append(want.blocks, Block{ID: ch.add.ID, Text: ch.add.Text})
		// The text arrives with the reader warnings it already had.
		_, warnings := scanLinkSpans(ch.add.Text)
		want.warnings = append(append([]string(nil), want.warnings...), warnings...)
	}
	if n.BOM {
		out = bomText + out
	}
	if err := verifyOutput([]byte(out), want); err != nil {
		return nil, fmt.Errorf("node %s: %w", n.Stem, err)
	}
	return []byte(out), nil
}

// dropRange is the body lines to remove with block idx: its own, together
// with the blank lines that separated it from the next block (or, for the
// last block, from what came before it).
func dropRange(runs []lineRun, idx int) (from, to int) {
	from, to = runs[idx].start, runs[idx].end
	switch {
	case idx+1 < len(runs):
		to = runs[idx+1].start
	case idx > 0:
		from = runs[idx-1].end
	default:
		from = 0
	}
	return from, to
}

// withLastBlock returns note with b appended as its last paragraph: the
// text, a space and ^ID, after exactly one blank line and with one line
// ending, in the line ending eol (Part III.8 "move-block, exactly").
// Nothing already in the note changes: its last line gets an ending if it
// had none, and blank lines it already ends with are kept, in which case
// none is added. A note with no blocks (hasBlocks false) gets the block
// after its frontmatter, head, laid out as a write lays out a first block.
// A block with no text is written as its ID alone.
func withLastBlock(head, note string, hasBlocks bool, eol string, b Block) string {
	para := "^" + b.ID + eol
	if b.Text != "" {
		para = renderBlock(b.Text, b.ID, eol)
	}
	if !hasBlocks {
		return withEnding(head, true, eol) + eol + para
	}
	note = withEnding(note, true, eol)
	if last := note[strings.LastIndex(note[:len(note)-1], "\n")+1:]; !blankLine(last) {
		note += eol
	}
	return note + para
}
