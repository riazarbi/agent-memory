package memory

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// MergeResult is a successful merge (Part III.8 "merge, exactly").
type MergeResult struct {
	// Blocks is the number of blocks moved into the kept node, Children
	// the number of loaded child nodes moved under it, TagsAdded the
	// number of tags it gained, Links the number of links whose target
	// was rewritten.
	Blocks    int
	Children  int
	TagsAdded int
	Links     int
}

// Merge folds the node from into the node to and removes from. from's
// blocks are appended to to's note in file order, each as MoveBlock
// appends one; everything in from's folder but its folder note and
// dot-entries (its child nodes with their subtrees, and whatever did not
// load) moves into to's folder; every link to from or to one of its
// blocks, in any note, to's and the moved blocks included, is retargeted
// to to; to gains the tags of from it lacks, in from's order, which
// re-emits its frontmatter only then (D26). to keeps its description and
// properties; from's go with its note. from's folder is then removed as
// Delete removes a node. Only notes that change are written.
//
// A refusal changes nothing: a *NoNodeError for an unknown node (its Stem
// says which), a *FieldError naming "from" (the root) or "into" (the root,
// from itself, a descendant of from, or an entry in to's folder with the
// name of one that would move there). All or nothing: if a note cannot be
// replaced, an entry cannot be moved, or the reload shows an error the
// store did not have, every replaced note is restored and every entry put
// back.
func (w *Writer) Merge(from, to string) (MergeResult, error) {
	if from == RootParent {
		return MergeResult{}, fieldErr("from", "%s is the root, which is never merged away; give the stem of a node", RootParent)
	}
	if to == RootParent {
		return MergeResult{}, fieldErr("into", "%s is the root, which holds no blocks; give the stem of the node to keep", RootParent)
	}
	st, unlock, err := w.locked()
	if err != nil {
		return MergeResult{}, err
	}
	defer unlock()
	mg, err := w.merging(st, from, to)
	if err != nil {
		return MergeResult{}, err
	}
	// Every note is worked out before anything is written.
	edits, res, err := mergeEdits(st, mg.from, mg.to)
	if err != nil {
		return MergeResult{}, err
	}
	// from's note goes with its folder: it must still be what was read.
	if err := w.unchangedOnDisk([]*Node{mg.from}); err != nil {
		return MergeResult{}, err
	}
	if err := mg.commit(st, edits); err != nil {
		return MergeResult{}, err
	}
	return res, nil
}

// merging is one merge: the node folded in, the node kept, their folders,
// and the names of the entries that move from the one to the other.
type merging struct {
	w              *Writer
	from, to       *Node
	fromDir, toDir string
	carried        []string
	// moved is how many of carried are in to's folder; hidden is from's
	// folder once it is renamed aside.
	moved  int
	hidden string
}

// merging resolves a merge's arguments against the loaded store and
// refuses what cannot be merged.
func (w *Writer) merging(st *Store, from, to string) (*merging, error) {
	f, ok := st.Node(from)
	if !ok {
		return nil, noNode(st, from)
	}
	t, ok := st.Node(to)
	if !ok {
		return nil, noNode(st, to)
	}
	if t == f {
		return nil, fieldErr("into", "%q is the node being merged; give a different node to merge it into", to)
	}
	for a := t.parent; a != nil; a = a.parent {
		if a == f {
			return nil, fieldErr("into", "node %q is under %q (at %s), and a node cannot be merged into its own descendant; merge %q into %q instead", t.Stem, f.Stem, t.Path, t.Stem, f.Stem)
		}
	}
	mg := &merging{
		w: w, from: f, to: t,
		fromDir: filepath.Join(w.dir, filepath.FromSlash(f.Path)),
		toDir:   filepath.Join(w.dir, filepath.FromSlash(t.Path)),
	}
	if err := mg.carry(); err != nil {
		return nil, err
	}
	return mg, nil
}

// carry lists what moves from from's folder into to's: everything but
// from's folder note and dot-entries. An entry whose name is already taken
// in to's folder is a *FieldError naming "into": nothing is overwritten.
func (mg *merging) carry() error {
	f, t := mg.from, mg.to
	entries, err := os.ReadDir(mg.fromDir)
	if err != nil {
		return fmt.Errorf("merging %s: %w", f.Path, err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == f.Stem+".md" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(mg.toDir, name)); err == nil {
			return fieldErr("into", "the folder of node %q already holds an entry named %q, and the merge would move the entry of that name in the folder of %q there; move, rename or remove one of them first", t.Stem, name, f.Stem)
		}
		mg.carried = append(mg.carried, name)
	}
	return nil
}

// mergeEdits works out a merge's notes: the folder notes it changes, with
// their new bytes, and the result's counts. In every note each link to
// from is retargeted to to; to's note also gains from's blocks, their own
// links retargeted, and the tags of from it lacks. from's note is not
// edited: it goes with its folder.
func mergeEdits(st *Store, from, to *Node) ([]noteEdit, MergeResult, error) {
	res := MergeResult{Blocks: len(from.Blocks), Children: len(from.children)}
	tags, added := unionTags(to.Tags, from.Tags)
	res.TagsAdded = added
	toFrom := func(e Edge) bool { return e.Stem == from.Stem }
	var edits []noteEdit
	for _, n := range st.order {
		ch, count := retargeting(n, to.Stem, toFrom)
		res.Links += count
		if n == from {
			continue
		}
		if n == to {
			for _, b := range from.Blocks {
				ch.adds = append(ch.adds, Block{ID: b.ID, Text: applyCuts(b.Text, relinkCuts(b.Text, from.Stem, to.Stem))})
			}
			if added > 0 {
				ch.tags = &tags
			}
		}
		if count == 0 && len(ch.adds) == 0 && ch.tags == nil {
			continue
		}
		data, err := rewritten(n, ch)
		if err != nil {
			return nil, MergeResult{}, fmt.Errorf("merging %s into %s: %w", from.Stem, to.Stem, err)
		}
		edits = append(edits, noteEdit{node: n, data: data})
	}
	return edits, res, nil
}

// unionTags is the tags of a kept node after a merge: its own, then those
// of the folded-in node it lacks, in that node's order; and how many were
// added.
func unionTags(kept, folded []string) ([]string, int) {
	tags := append([]string(nil), kept...)
	added := 0
	for _, t := range folded {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
			added++
		}
	}
	return tags, added
}

// commit replaces the edited notes, moves the carried entries into to's
// folder, renames from's folder aside and reloads the store to check it;
// only then is the folder removed. Whatever fails before that, from's
// folder gets its name back, the entries are put back and the replaced
// notes restored - except when the reload finds a replaced note holding
// other bytes: something that does not take the lock edited it, which is
// ErrChangedOnDisk, and nothing is rolled back over the other content
// (D28); the merge is then complete on disk.
func (mg *merging) commit(before *Store, edits []noteEdit) error {
	w, f, t := mg.w, mg.from, mg.to
	what := "merging " + f.Stem + " into " + t.Stem
	done, err := w.replaceAll(edits)
	if err != nil {
		return err
	}
	if err := mg.apply(); err != nil {
		return withRollback(err, errors.Join(mg.undo(), restoreNotes(done)))
	}
	after := w.reload(t.Path)
	var changed error
	for _, r := range done {
		if cur, err := os.ReadFile(w.notePath(relocated(r.rel, f.Path, t.Path))); err != nil || !bytes.Equal(cur, r.data) {
			changed = changedErr(r.rel, "after this call replaced it; the other content is kept, not rolled back")
			break
		}
	}
	if changed == nil {
		if p, bad := unsound(before, after, t.Stem, f.Path, t.Path); bad {
			return newErrorErr(p, what, errors.Join(mg.undo(), restoreNotes(done)))
		}
	}
	if err := os.RemoveAll(mg.hidden); err != nil {
		return fmt.Errorf("node %s is merged into %s, but its folder, renamed to %s, could not be removed: %w", f.Stem, t.Stem, filepath.Base(mg.hidden), err)
	}
	return changed
}

// apply moves the carried entries into to's folder, then renames from's
// folder, left with its note and dot-entries, to a hidden name beside
// itself, as a delete does. On failure, what it did is for undo to put
// back.
func (mg *merging) apply() error {
	for _, name := range mg.carried {
		if err := renameFile(filepath.Join(mg.fromDir, name), filepath.Join(mg.toDir, name)); err != nil {
			return fmt.Errorf("merging %s: moving %s: %w", mg.from.Path, name, err)
		}
		mg.moved++
	}
	hidden, err := hiddenBeside(mg.fromDir)
	if err != nil {
		return err
	}
	if err := renameFile(mg.fromDir, hidden); err != nil {
		return fmt.Errorf("merging %s: %w", mg.from.Path, err)
	}
	mg.hidden = hidden
	return nil
}

// undo gives from's folder its name back and puts back every entry that
// moved, newest first. It returns the first failure.
func (mg *merging) undo() error {
	var first error
	if mg.hidden != "" {
		if err := renameFile(mg.hidden, mg.fromDir); err != nil {
			return err // the entries cannot go back into a folder that is not there
		}
		mg.hidden = ""
	}
	for mg.moved > 0 {
		name := mg.carried[mg.moved-1]
		if err := renameFile(filepath.Join(mg.toDir, name), filepath.Join(mg.fromDir, name)); err != nil && first == nil {
			first = err
		}
		mg.moved--
	}
	return first
}
