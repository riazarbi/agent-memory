package memory

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
)

// SplitRequest is one split (Part III.8 "split, exactly"): the blocks
// Blocks of the node Stem go, in the order given, into a new node New made
// under Parent (RootParent for the root) with the description Description.
type SplitRequest struct {
	Stem        string
	New         string
	Parent      string
	Description string
	// Blocks are block IDs of Stem, as a read prints them.
	Blocks []string
}

// SplitResult is a successful split.
type SplitResult struct {
	// Path is the new node's folder relative to notes/.
	Path string
	// Blocks is the number of blocks moved, Left the number remaining in
	// the split node, Links the number of links whose target was
	// rewritten.
	Blocks int
	Left   int
	Links  int
}

// Split creates the node req.New under req.Parent, as a write creates a
// node, and moves the named blocks of req.Stem into it in the order given,
// with their IDs and text. The new node gets no tags and no properties.
// Each block leaves the old note as Delete removes one; every link to a
// moved block, in any note, the moved blocks included, is retargeted to the
// new node; links to the old node are not touched. The old node stays
// where it is and may be left with no blocks. Only notes that change are
// written.
//
// A refusal changes nothing: a *NoNodeError for an unknown stem or parent
// (its Stem says which), or a *FieldError naming "stem" (the root), "into"
// (the new stem: not valid, in use, or refused by a create's rules),
// "parent", "description" or "block" (none given, one given twice, not a
// block of the node, or an ID more than one block carries). All or
// nothing: if the new node cannot be made, a note cannot be replaced, or
// the reload shows an error the store did not have, every replaced note is
// restored and the new node's folder removed.
func (w *Writer) Split(req SplitRequest) (SplitResult, error) {
	if err := req.check(); err != nil {
		return SplitResult{}, err
	}
	st, unlock, err := w.locked()
	if err != nil {
		return SplitResult{}, err
	}
	defer unlock()
	n, ok := st.Node(req.Stem)
	if !ok {
		return SplitResult{}, noNode(st, req.Stem)
	}
	idxs, err := splitBlocks(st, n, req.Blocks)
	if err != nil {
		return SplitResult{}, err
	}
	parentPath, err := w.splitTarget(st, req)
	if err != nil {
		return SplitResult{}, err
	}
	// Every note is worked out before anything is written.
	note, edits, links, err := splitEdits(st, n, idxs, req)
	if err != nil {
		return SplitResult{}, err
	}
	rel := path.Join(parentPath, req.New)
	if err := w.commitSplit(st, n, req.New, rel, note, edits); err != nil {
		return SplitResult{}, err
	}
	return SplitResult{Path: rel, Blocks: len(idxs), Left: len(n.Blocks) - len(idxs), Links: links}, nil
}

// check refuses what a split's arguments alone show to be wrong, before
// the store is looked at.
func (req SplitRequest) check() error {
	if req.Stem == RootParent {
		return fieldErr("stem", "%s is the root, which holds no blocks to split off; give the stem of a node", RootParent)
	}
	if req.New == RootParent {
		return fieldErr("into", "%s is the root, never a node's stem; give a lowercase-hyphenated stem for the new node", RootParent)
	}
	if err := CheckStem(req.New); err != nil {
		return fieldErr("into", "%v", err)
	}
	if req.Parent == "" {
		return fieldErr("parent", "is required: a split says where the new node %q goes - the stem of the node to put it under (the split node's own stem makes it a child), or %s for the top level", req.New, RootParent)
	}
	if err := validDescription(req.Description); err != nil {
		return err
	}
	if len(req.Blocks) == 0 {
		return fieldErr("block", "is required: name at least one block of %q to move into %q, by its ID as a read prints it (m-7f3a2k)", req.Stem, req.New)
	}
	seen := map[string]bool{}
	for _, id := range req.Blocks {
		if id == "" {
			return fieldErr("block", "needs a block ID, as a read prints it (m-7f3a2k)")
		}
		if seen[id] {
			return fieldErr("block", "block %q is named twice; name each block to move once", id)
		}
		seen[id] = true
	}
	return nil
}

// splitBlocks resolves a split's block IDs to indices into n's blocks, in
// the order given. An ID that is not a block of n, or that more than one
// block in the store carries (as MoveBlock refuses it), is a *FieldError
// naming "block".
func splitBlocks(st *Store, n *Node, ids []string) ([]int, error) {
	idxs := make([]int, 0, len(ids))
	for _, id := range ids {
		idx := -1
		for i, b := range n.Blocks {
			if b.ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, fieldErr("block", "no block %q in node %s; a split moves blocks of the node it splits", id, n.Stem)
		}
		if _, _, err := blockHolder(st, id); err != nil {
			var fe *FieldError
			if errors.As(err, &fe) {
				return nil, fieldErr("block", "%s", fe.Reason)
			}
			return nil, err
		}
		idxs = append(idxs, idx)
	}
	return idxs, nil
}

// splitTarget checks the new node as a write checks a node it creates
// (D28's create rules, and so as Rename checks its new stem), and returns
// the parent's path relative to notes/: "" for the root.
func (w *Writer) splitTarget(st *Store, req SplitRequest) (string, error) {
	if other, taken := st.Node(req.New); taken {
		return "", fieldErr("into", "stem %q is in use by the node at %s; stems are unique store-wide - choose another (to put the blocks in a node that exists, move them one by one)", req.New, other.Path)
	}
	if req.Parent != RootParent {
		if _, ok := st.Node(req.Parent); !ok {
			return "", noNode(st, req.Parent)
		}
	}
	parentPath, err := w.checkCreate(st, WriteRequest{Stem: req.New, Parent: req.Parent, Description: &req.Description})
	if err == nil {
		return parentPath, nil
	}
	// A create's refusals are about its stem: here, the new node's.
	var fe *FieldError
	switch {
	case !errors.As(err, &fe):
		return "", fieldErr("into", "%v", err)
	case fe.Field == "stem":
		return "", fieldErr("into", "%s", fe.Reason)
	}
	return "", err
}

// splitEdits works out a split: the new node's folder note, the folder
// notes the split changes with their new bytes, and the number of links
// rewritten. n's note loses the moved blocks; in every note, n's and the
// moved blocks' own text included, each link to a moved block is
// retargeted to the new node.
func splitEdits(st *Store, n *Node, idxs []int, req SplitRequest) (note []byte, edits []noteEdit, links int, err error) {
	moving := map[string]bool{}
	for _, idx := range idxs {
		moving[n.Blocks[idx].ID] = true
	}
	toMoved := func(e Edge) bool { return e.Stem == n.Stem && moving[e.Block] }
	for _, m := range st.order {
		ch, count := retargeting(m, req.New, toMoved)
		links += count
		switch {
		case m == n:
			ch.drops = append([]int(nil), idxs...)
			sort.Ints(ch.drops)
		case count == 0:
			continue
		}
		data, err := rewritten(m, ch)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("splitting %s: %w", n.Stem, err)
		}
		edits = append(edits, noteEdit{node: m, data: data})
	}
	// The new note: laid out as a create lays one out, each block with
	// the ID it has and the reader warnings its text already had.
	want := expectation{description: req.Description}
	for _, idx := range idxs {
		b := n.Blocks[idx]
		text := applyCuts(b.Text, retargetCuts(b.Text, req.New, toMoved))
		want.blocks = append(want.blocks, Block{ID: b.ID, Text: text})
		_, warnings := scanLinkSpans(text)
		want.warnings = append(want.warnings, warnings...)
	}
	note, err = newNote(want)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("splitting %s into %s: %w", n.Stem, req.New, err)
	}
	return note, edits, links, nil
}

// commitSplit makes the new node at rel, replaces the edited notes and
// reloads the store to check it. The new node is made first, so a crash
// part-way leaves the moved blocks in both notes, never in neither.
// Whatever fails, the replaced notes are restored and the new node's
// folder removed - except when the reload finds a note this call wrote
// holding other bytes: something that does not take the lock edited it,
// which is ErrChangedOnDisk, and nothing is rolled back over the other
// content (D28).
func (w *Writer) commitSplit(before *Store, n *Node, stem, rel string, note []byte, edits []noteEdit) error {
	undoNew, err := w.makeNode(rel, note)
	if err != nil {
		return err
	}
	done, err := w.replaceAll(edits)
	if err != nil {
		return withRollback(err, undoNew())
	}
	after := w.reload(rel)
	if err := keptBytes(done); err != nil {
		return err
	}
	if cur, err := os.ReadFile(w.notePath(rel)); err != nil || !bytes.Equal(cur, note) {
		return changedErr(rel, "after this call wrote it; the other content is kept, not rolled back")
	}
	// No folder moves, so each error is compared at the path it had.
	if p, bad := unsound(before, after, stem, rel, rel); bad {
		return newErrorErr(p, "splitting "+n.Stem+" into "+stem, errors.Join(restoreNotes(done), undoNew()))
	}
	return nil
}
