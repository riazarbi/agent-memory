package memory

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// RenameResult is a successful rename (Part III.8 "rename, exactly"): the
// old and new stems, and the number of links whose target was rewritten.
type RenameResult struct {
	Old   string
	New   string
	Links int
}

// CheckStem says why s cannot be a node's stem (D25), or nil when it can.
func CheckStem(s string) error {
	if validStem(s) {
		return nil
	}
	return fmt.Errorf("%q is not a valid stem: use lowercase letters, digits and single hyphens (e.g. outbox-etiquette), at most %d characters", s, maxStemLen)
}

// Rename gives the node stem the stem new: its folder and folder note are
// renamed where they are, and the target of every link to the node, in
// any note, its own included, is rewritten. Only those bytes change, and
// notes holding no such link are not written. A refusal changes nothing:
// a *NoNodeError for an unknown stem, a *FieldError naming "new" for a
// stem that is not valid, is the node's own, is in use, or is the name of
// an entry already in the way. All or nothing: if a note cannot be
// replaced, a rename fails, or the reload shows an error the store did not
// have, every rewritten note is restored and the old names put back.
func (w *Writer) Rename(stem, new string) (RenameResult, error) {
	if err := CheckStem(new); err != nil {
		return RenameResult{}, fieldErr("new", "%v", err)
	}
	st, unlock, err := w.locked()
	if err != nil {
		return RenameResult{}, err
	}
	defer unlock()
	n, ok := st.Node(stem)
	if !ok {
		return RenameResult{}, noNode(st, stem)
	}
	rn := renaming{w: w, node: n, new: new, rel: path.Join(path.Dir(n.Path), new)}
	if err := rn.refusal(st); err != nil {
		return RenameResult{}, err
	}
	// Every note is worked out before any is written.
	edits, links, err := relinkEdits(st, stem, new)
	if err != nil {
		return RenameResult{}, err
	}
	if err := rn.commit(st, edits); err != nil {
		return RenameResult{}, err
	}
	return RenameResult{Old: stem, New: new, Links: links}, nil
}

// commit replaces the edited notes, renames the folder note and the
// folder, and reloads the store to check it. Whatever fails, the notes
// already replaced are restored and the old names put back - except when
// the reload finds a replaced note holding other bytes: something that
// does not take the lock edited it, which is ErrChangedOnDisk, and nothing
// is rolled back over the other content (D28).
func (rn renaming) commit(before *Store, edits []noteEdit) error {
	w, n := rn.w, rn.node
	done, err := w.replaceAll(edits)
	if err != nil {
		return err
	}
	if err := rn.apply(); err != nil {
		return withRollback(err, restoreNotes(done))
	}
	after := w.reload(rn.rel)
	for _, r := range done {
		if cur, err := os.ReadFile(w.notePath(relocated(r.rel, n.Path, rn.rel))); err != nil || !bytes.Equal(cur, r.data) {
			return changedErr(r.rel, "after this call replaced it; the other content is kept, not rolled back")
		}
	}
	if p, bad := unsound(before, after, rn.new, n.Path, rn.rel); bad {
		return newErrorErr(p, "renaming "+n.Stem+" to "+rn.new, errors.Join(rn.undo(), restoreNotes(done)))
	}
	return nil
}

// renaming is one rename's node, new stem, and new path relative to
// notes/.
type renaming struct {
	w    *Writer
	node *Node
	new  string
	rel  string
}

// refusal checks what a rename cannot do: take the node's own stem, a
// loaded node's, the name of any folder in the tree (exactly, or up to
// letter case: what write refuses to create), or the name of an entry that
// is already there, beside the node's folder or (as NEW.md) inside it.
func (rn renaming) refusal(st *Store) error {
	n, new := rn.node, rn.new
	if new == n.Stem {
		return fieldErr("new", "%q is already the node's stem; give a different one", new)
	}
	if other, taken := st.Node(new); taken {
		return fieldErr("new", "stem %q is in use by the node at %s; stems are unique store-wide - choose another", new, other.Path)
	}
	// As strictly as write checks a stem it creates (D28): any folder in
	// the tree, loaded or not, and a name differing only by case. The
	// walk compares names, so it does not depend on how the filesystem
	// treats case.
	exact, folded := rn.w.findFolders(new)
	if exact != "" {
		return fieldErr("new", "stem %q is taken by the folder %s, which the reader skipped (%s); fix or remove that folder first, or choose another stem", new, exact, problemFor(st, exact))
	}
	if folded != "" {
		return fieldErr("new", "%q differs only by letter case from the folder %s; stems are unique store-wide and a case-insensitive filesystem cannot hold both - choose another stem", new, folded)
	}
	if _, err := os.Lstat(rn.newFolder()); err == nil {
		return fieldErr("new", "an entry named %q is already there beside the node's folder, and it is not a loaded node; move or remove it first, or choose another stem", new)
	}
	if _, err := os.Lstat(filepath.Join(rn.folder(), new+".md")); err == nil {
		return fieldErr("new", "the node's folder already holds a file named %s, which the rename would overwrite; move or remove it first, or choose another stem", new+".md")
	}
	return nil
}

func (rn renaming) folder() string {
	return filepath.Join(rn.w.dir, filepath.FromSlash(rn.node.Path))
}

func (rn renaming) newFolder() string {
	return filepath.Join(rn.w.dir, filepath.FromSlash(rn.rel))
}

// apply renames the folder note, then the folder. If the second fails the
// first is undone.
func (rn renaming) apply() error {
	oldNote := filepath.Join(rn.folder(), rn.node.Stem+".md")
	newNote := filepath.Join(rn.folder(), rn.new+".md")
	if err := renameFile(oldNote, newNote); err != nil {
		return fmt.Errorf("renaming %s: %w", rn.node.Path, err)
	}
	if err := renameFile(rn.folder(), rn.newFolder()); err != nil {
		return withRollback(fmt.Errorf("renaming %s: %w", rn.node.Path, err), renameFile(newNote, oldNote))
	}
	return nil
}

// undo puts the folder and its note back under their old names.
func (rn renaming) undo() error {
	if err := renameFile(rn.newFolder(), rn.folder()); err != nil {
		return err
	}
	return renameFile(filepath.Join(rn.folder(), rn.new+".md"), filepath.Join(rn.folder(), rn.node.Stem+".md"))
}

// withRollback is err, saying so when rolling back after it failed too.
func withRollback(err, rollbackErr error) error {
	if rollbackErr != nil {
		return fmt.Errorf("%w; and rolling back failed: %v", err, rollbackErr)
	}
	return err
}

// relinkEdits returns the folder notes that hold a link to the node old,
// each with those links retargeted to new, and the number of links
// rewritten across them.
func relinkEdits(st *Store, old, new string) ([]noteEdit, int, error) {
	var edits []noteEdit
	links := 0
	for _, n := range st.order {
		data, count, err := relinked(n, old, new)
		if err != nil {
			return nil, 0, err
		}
		if count > 0 {
			edits = append(edits, noteEdit{node: n, data: data})
			links += count
		}
	}
	return edits, links, nil
}

// relinkCuts returns the changes that retarget a block's links to the node
// old: for each edge the link scanner finds to it, node link or block
// link, the stem is replaced by new.
func relinkCuts(text, old, new string) []cut {
	return retargetCuts(text, new, func(e Edge) bool { return e.Stem == old })
}

// retargetCuts returns the changes that give the stem new to every edge
// in a block's text that match picks. The edges are the link scanner's: a
// [[…]] in inline code or in an unsupported form is no edge and is left
// alone. A link's target begins right after its "[[", so only the stem is
// replaced: the alias and the block ID are not touched.
func retargetCuts(text, new string, match func(Edge) bool) []cut {
	spans, _ := scanLinkSpans(text)
	var cuts []cut
	for _, s := range spans {
		if match(s.edge) {
			at := s.start + len(linkOpen)
			cuts = append(cuts, cut{start: at, end: at + len(s.edge.Stem), repl: new})
		}
	}
	return cuts
}

// relinked returns n's folder note with every link to the node old
// retargeted to new, and how many links that was; with none, no bytes.
// The links are rewritten in the blocks' source lines, so every other byte
// of the note stays as it was.
func relinked(n *Node, old, new string) ([]byte, int, error) {
	count := 0
	blockCuts := make([][]cut, len(n.Blocks))
	for i, b := range n.Blocks {
		blockCuts[i] = relinkCuts(b.Text, old, new)
		count += len(blockCuts[i])
	}
	if count == 0 {
		return nil, 0, nil
	}
	out, err := rewritten(n, noteChange{cuts: blockCuts})
	if err != nil {
		return nil, 0, err
	}
	return out, count, nil
}
