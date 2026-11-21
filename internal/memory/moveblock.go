package memory

import (
	"fmt"
	"strings"
	"time"
)

// MoveBlockResult is a successful move-block (Part III.8 "move-block,
// exactly"): the node the block left, the node it is in now, and the
// number of links whose target was rewritten.
type MoveBlockResult struct {
	From  string
	To    string
	Links int
}

// MoveBlock takes the block with the ID id out of the node that holds it
// and appends it, with the same ID and text, as the last block of the node
// to. The old note loses the block as Delete removes one; the target note
// gains one last paragraph and nothing else in it changes (it is not run
// through the encoder: no ID-less paragraph is adopted, no blank line
// collapsed). Every link to the block, in any note, the two notes and the
// moved block included, is retargeted to the node to; links to the old
// node are not touched. Only notes that change are written.
//
// A refusal changes nothing: a *FieldError naming "id" when no block or
// more than one block carries the ID, a *NoNodeError for an unknown
// target, a *FieldError naming "to" for the root or for the node the block
// is already in; checked in that order, the root first. All or nothing: if
// a note cannot be replaced, or the reload shows an error the store did
// not have, every note already replaced is restored.
func (w *Writer) MoveBlock(id, to string) (MoveBlockResult, error) {
	if id == "" {
		return MoveBlockResult{}, fieldErr("id", "needs the ID of the block to move, as a read prints it (m-7f3a2k)")
	}
	if to == RootParent {
		return MoveBlockResult{}, fieldErr("to", "%s is the root, which holds no blocks; give the stem of a node", RootParent)
	}
	st, unlock, err := w.locked()
	if err != nil {
		return MoveBlockResult{}, err
	}
	defer unlock()
	from, idx, target, err := blockMove(st, id, to)
	if err != nil {
		return MoveBlockResult{}, err
	}
	// Every note is worked out before any is written.
	edits, links, err := blockMoveEdits(st, from, idx, target, callTime())
	if err != nil {
		return MoveBlockResult{}, err
	}
	done, err := w.replaceAll(edits)
	if err != nil {
		return MoveBlockResult{}, err
	}
	after := w.reload(target.Path)
	if err := keptBytes(done); err != nil {
		return MoveBlockResult{}, err
	}
	// No folder moves, so each error is compared at the path it had.
	if p, bad := unsound(st, after, target.Stem, target.Path, target.Path); bad {
		return MoveBlockResult{}, newErrorErr(p, "moving block "+id+" to "+to, restoreNotes(done))
	}
	return MoveBlockResult{From: from.Stem, To: to, Links: links}, nil
}

// blockMove resolves a move-block's arguments against the loaded store,
// in the spec's order: the block (its node and index there), then the
// target node, which must not be the one the block is in.
func blockMove(st *Store, id, to string) (from *Node, idx int, target *Node, err error) {
	from, idx, err = blockHolder(st, id)
	if err != nil {
		return nil, 0, nil, err
	}
	target, ok := st.Node(to)
	if !ok {
		return nil, 0, nil, noNode(st, to)
	}
	if target == from {
		return nil, 0, nil, fieldErr("to", "block %s is already in node %q; give the node to move it to", id, to)
	}
	return from, idx, target, nil
}

// blockHolder finds the one loaded block carrying id: its node and its
// index there. No such block, or more than one (a lint warning: the call
// cannot tell which is meant, twice within one node included), is a
// *FieldError naming "id".
func blockHolder(st *Store, id string) (*Node, int, error) {
	var (
		holder *Node
		idx    int
		stems  []string
		count  int
	)
	for _, n := range st.order {
		for i, b := range n.Blocks {
			if b.ID != id {
				continue
			}
			if count == 0 {
				holder, idx = n, i
			}
			if len(stems) == 0 || stems[len(stems)-1] != n.Stem {
				stems = append(stems, n.Stem)
			}
			count++
		}
	}
	switch {
	case count == 0:
		return nil, 0, fieldErr("id", "no block %q in the store; give a block's ID as a read prints it (m-7f3a2k)", id)
	case count > 1:
		return nil, 0, fieldErr("id", "block ID %q is carried by more than one block (%d blocks, in %s), so the call cannot tell which is meant; lint reports the ID as used twice - give one of them a new ID by hand first", id, count, strings.Join(stems, ", "))
	}
	return holder, idx, nil
}

// blockMoveEdits returns the folder notes a move of from's block idx to
// target changes, each with its new bytes, and the number of links
// rewritten: from's note without the block, target's with it appended, and
// in every note, those two included, each link to the block retargeted.
// The moved block's own links to itself are rewritten in its text as it
// moves, and counted. The two notes have had a content change, dated at;
// a note that only has links retargeted keeps its date (III.9).
func blockMoveEdits(st *Store, from *Node, idx int, target *Node, at time.Time) ([]noteEdit, int, error) {
	moved := from.Blocks[idx]
	toBlock := func(e Edge) bool { return e.Stem == from.Stem && e.Block == moved.ID }
	var edits []noteEdit
	links := 0
	for _, n := range st.order {
		ch, count := retargeting(n, target.Stem, toBlock)
		links += count
		switch {
		case n == from:
			ch.drops = []int{idx}
		case n == target:
			ch.adds = []Block{{ID: moved.ID, Text: applyCuts(moved.Text, retargetCuts(moved.Text, target.Stem, toBlock))}}
		case count == 0:
			continue
		}
		data, err := rewritten(n, ch, at)
		if err != nil {
			return nil, 0, fmt.Errorf("moving block %s: %w", moved.ID, err)
		}
		edits = append(edits, noteEdit{node: n, data: data})
	}
	return edits, links, nil
}
