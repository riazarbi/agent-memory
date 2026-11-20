package memory

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MoveResult is a successful move (Part III.8 "move, exactly").
type MoveResult struct {
	// From is the stem of the parent the node had, RootParent for a
	// first-level node.
	From string
	// Parent is the parent it has now, as the call named it.
	Parent string
	// Path is the node's folder relative to notes/ after the call.
	Path string
}

// Move puts the node stem, with everything in its folder, under the node
// parent, or under the root when parent is RootParent. It is one directory
// rename: no note is rewritten, since links resolve by stem. A move to the
// parent the node already has changes nothing and succeeds. A refusal
// changes nothing: a *NoNodeError for an unknown stem or parent (its Stem
// says which), a *FieldError naming "parent" for a move under the node
// itself or its own descendant, or onto an entry already in the parent's
// folder. If the reload shows an error the store did not have, the folder
// is renamed back and the call fails.
func (w *Writer) Move(stem, parent string) (MoveResult, error) {
	st, unlock, err := w.locked()
	if err != nil {
		return MoveResult{}, err
	}
	defer unlock()
	n, ok := st.Node(stem)
	if !ok {
		return MoveResult{}, noNode(st, stem)
	}
	parentPath, err := moveTarget(st, n, parent)
	if err != nil {
		return MoveResult{}, err
	}
	res := MoveResult{From: parentStem(n), Parent: parent, Path: path.Join(parentPath, n.Stem)}
	if res.Path == n.Path {
		return res, nil
	}
	folder := filepath.Join(w.dir, filepath.FromSlash(n.Path))
	dest := filepath.Join(w.dir, filepath.FromSlash(res.Path))
	if _, err := os.Lstat(dest); err == nil {
		return MoveResult{}, fieldErr("parent", "an entry named %q is already in the folder of %s, and it is not a loaded node; move or remove it first", n.Stem, parentName(parent))
	}
	if err := renameFile(folder, dest); err != nil {
		return MoveResult{}, fmt.Errorf("moving %s: %w", n.Path, err)
	}
	after := w.reload(res.Path)
	if p, bad := unsound(st, after, n.Stem, n.Path, res.Path); bad {
		return MoveResult{}, newErrorErr(p, "moving "+n.Stem, renameFile(dest, folder))
	}
	return res, nil
}

// moveTarget returns the path, relative to notes/, of the folder a move of
// n to parent puts it in: "" for the root.
func moveTarget(st *Store, n *Node, parent string) (string, error) {
	if parent == RootParent {
		return "", nil
	}
	pn, ok := st.Node(parent)
	if !ok {
		return "", noNode(st, parent)
	}
	if pn == n {
		return "", fieldErr("parent", "node %q cannot be moved under itself; give another node, or %s for the top level", n.Stem, RootParent)
	}
	for a := pn.parent; a != nil; a = a.parent {
		if a == n {
			return "", fieldErr("parent", "node %q is under %q (at %s), and a node cannot be moved under its own descendant", pn.Stem, n.Stem, pn.Path)
		}
	}
	return pn.Path, nil
}

// parentStem is the stem of n's parent, RootParent for a first-level node.
func parentStem(n *Node) string {
	if p := n.Parent(); p != nil {
		return p.Stem
	}
	return RootParent
}

// parentName names a parent in a message: the root has no stem.
func parentName(parent string) string {
	if parent == RootParent {
		return "the root"
	}
	return fmt.Sprintf("node %q", parent)
}

// relocated is where the path rel, relative to notes/, is once the folder
// from has become the folder to: paths at or under from follow it.
func relocated(rel, from, to string) string {
	if rel == from || strings.HasPrefix(rel, from+"/") {
		return to + rel[len(from):]
	}
	return rel
}

// unsound checks III.8's postcondition after the folder from became the
// folder to: the store skips no folder it did not skip before, and the
// node stem loaded at to. It returns the problem to report when the check
// fails.
//
// An error is identified by the folder it skips, not by its reason: a
// folder that did not load before the call and went along with the move
// is the same error at its new path, and reasons quote paths that the
// call changes ("stem already used by <path>", "cannot read folder
// <path>"), so newErrors' whole-problem comparison would roll back every
// move that carries, or is named by, an unloaded folder.
func unsound(before, after *Store, stem, from, to string) (Problem, bool) {
	had := map[string]int{}
	for _, p := range before.problems {
		if p.Severity == SeverityError {
			had[relocated(p.Path, from, to)]++
		}
	}
	for _, p := range after.problems {
		if p.Severity != SeverityError {
			continue
		}
		if had[p.Path] == 0 {
			return p, true
		}
		had[p.Path]--
	}
	if n, ok := after.Node(stem); !ok || n.Path != to {
		return Problem{Severity: SeverityError, Path: to, Reason: "the node did not load there"}, true
	}
	return Problem{}, false
}
