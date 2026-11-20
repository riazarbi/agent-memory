package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DeleteRequest is one delete (Part III.8 "delete, exactly"): with Block,
// that block of the node Stem; without, the node, and with Recursive
// everything in its folder.
type DeleteRequest struct {
	Stem string
	// Block is a block ID of Stem, or empty to delete the node.
	Block string
	// Recursive lets a node delete remove a folder that holds more than
	// the folder note. It cannot be given with Block.
	Recursive bool
}

// DeleteResult is a successful delete.
type DeleteResult struct {
	// Nodes and Blocks count what was removed: the loaded nodes of the
	// deleted subtree and their blocks, or 0 and 1 for a block delete.
	Nodes  int
	Blocks int
	// Dangling lists every link the delete broke: by holding node's stem,
	// then that node's block order, then position in the block.
	Dangling []DanglingLink
}

// DanglingLink is one link left pointing at something deleted. It is
// reported, never rewritten.
type DanglingLink struct {
	// Stem and Block name the node and block that hold the link; Block is
	// empty when that block has no ID.
	Stem  string
	Block string
	// Link is the link as written, brackets and alias included.
	Link string
}

// Delete removes one block, or a node's folder. Links to what it removes
// are not rewritten; the result lists them. A refusal changes nothing: a
// *NoNodeError for an unknown stem, a *FieldError naming "block",
// "recursive" or "stem" (a node whose folder holds more than its folder
// note, without Recursive).
func (w *Writer) Delete(req DeleteRequest) (DeleteResult, error) {
	if req.Block != "" && req.Recursive {
		return DeleteResult{}, fieldErr("recursive", "--recursive deletes a node with everything under it, so it cannot be given with --block; give one or the other")
	}
	st, unlock, err := w.locked()
	if err != nil {
		return DeleteResult{}, err
	}
	defer unlock()
	n, ok := st.Node(req.Stem)
	if !ok {
		return DeleteResult{}, noNode(st, req.Stem)
	}
	if req.Block != "" {
		return w.deleteBlock(st, n, req.Block)
	}
	return w.deleteNode(st, n, req.Recursive)
}

// deleteBlock removes n's first block carrying id from its folder note and
// changes nothing else in it.
func (w *Writer) deleteBlock(st *Store, n *Node, id string) (DeleteResult, error) {
	idx := -1
	for i, b := range n.Blocks {
		if b.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return DeleteResult{}, fieldErr("block", "no block %q in node %s", id, n.Stem)
	}
	data, err := withoutBlock(n, idx)
	if err != nil {
		return DeleteResult{}, err
	}
	after, err := w.replaceNotes(st, []noteEdit{{node: n, data: data}}, "deleting block "+id+" of "+n.Stem)
	if err != nil {
		return DeleteResult{}, err
	}
	gone := func(holder *Node, block int) bool { return holder == n && block == idx }
	return DeleteResult{Blocks: 1, Dangling: brokenLinks(st, after, gone)}, nil
}

// withoutBlock returns n's folder note without block idx: its source lines
// are removed together with the blank lines that separated it from the
// next block (or, for the last block, from what came before it). Every
// other byte is copied.
func withoutBlock(n *Node, idx int) ([]byte, error) {
	lines, bodyStart, runs, err := bodyLines(n)
	if err != nil {
		return nil, err
	}
	from, to := runs[idx].start, runs[idx].end
	switch {
	case idx+1 < len(runs):
		to = runs[idx+1].start
	case idx > 0:
		from = runs[idx-1].end
	default:
		from = 0
	}
	out := strings.Join(lines[:bodyStart+from], "") + strings.Join(lines[bodyStart+to:], "")
	if n.BOM {
		out = bomText + out
	}
	want, err := expectationOf(n)
	if err != nil {
		return nil, err
	}
	want.blocks = append(want.blocks[:idx:idx], want.blocks[idx+1:]...)
	if err := verifyOutput([]byte(out), want); err != nil {
		return nil, fmt.Errorf("node %s: %w", n.Stem, err)
	}
	return []byte(out), nil
}

// brokenLinks lists the links a delete broke: every edge, in a block of a
// node that is still in after, that resolved in before and does not
// resolve in after. gone reports the blocks that were themselves deleted.
// A link that was already dangling is not listed, nor one that still
// resolves (to the second holder of a duplicated block ID, say).
func brokenLinks(before, after *Store, gone func(holder *Node, block int) bool) []DanglingLink {
	holders := before.Nodes()
	sort.Slice(holders, func(i, j int) bool { return holders[i].Stem < holders[j].Stem })
	var out []DanglingLink
	for _, n := range holders {
		if _, still := after.Node(n.Stem); !still {
			continue
		}
		for bi, b := range n.Blocks {
			if gone(n, bi) {
				continue
			}
			spans, _ := scanLinkSpans(b.Text)
			for _, s := range spans {
				if before.resolves(s.edge) && !after.resolves(s.edge) {
					out = append(out, DanglingLink{Stem: n.Stem, Block: b.ID, Link: b.Text[s.start:s.end]})
				}
			}
		}
	}
	return out
}

// deleteNode removes n's folder. Without recursive it refuses a folder
// holding anything but the folder note and dot-entries. The folder is
// first renamed to a hidden name beside itself, so no half-removed node is
// ever visible; the store is reloaded, and if it has an error it did not
// have before, the folder is renamed back and the call fails. Only then is
// the hidden folder removed.
func (w *Writer) deleteNode(st *Store, n *Node, recursive bool) (DeleteResult, error) {
	folder := filepath.Join(w.dir, filepath.FromSlash(n.Path))
	if !recursive {
		if err := onlyFolderNote(folder, n); err != nil {
			return DeleteResult{}, err
		}
	}
	subtree := subtreeOf(n)
	if err := w.unchangedOnDisk(subtree); err != nil {
		return DeleteResult{}, err
	}
	hidden, err := hiddenBeside(folder)
	if err != nil {
		return DeleteResult{}, err
	}
	if err := renameFile(folder, hidden); err != nil {
		return DeleteResult{}, fmt.Errorf("deleting %s: %w", n.Path, err)
	}
	after := w.reload(n.Path)
	if errs := newErrors(st, after); len(errs) > 0 {
		return DeleteResult{}, newErrorErr(errs[0], "deleting "+n.Stem, renameFile(hidden, folder))
	}
	res := DeleteResult{Nodes: len(subtree), Dangling: brokenLinks(st, after, func(*Node, int) bool { return false })}
	for _, d := range subtree {
		res.Blocks += len(d.Blocks)
	}
	if err := os.RemoveAll(hidden); err != nil {
		return DeleteResult{}, fmt.Errorf("node %s is deleted, but its folder, renamed to %s, could not be removed: %w", n.Stem, filepath.Base(hidden), err)
	}
	return res, nil
}

// unchangedOnDisk is D28's edit-race check for a delete: every loaded
// folder note about to go must still hold the bytes this call read, or
// something that does not take the lock edited it meanwhile.
func (w *Writer) unchangedOnDisk(nodes []*Node) error {
	if testHookBeforeRename != nil {
		testHookBeforeRename(w.notePath(nodes[0].Path))
	}
	for _, d := range nodes {
		cur, err := os.ReadFile(w.notePath(d.Path))
		if err != nil || string(cur) != d.fileText() {
			return changedErr(d.Path, "since this call read it; nothing was deleted")
		}
	}
	return nil
}

// subtreeOf returns n and every loaded node under it.
func subtreeOf(n *Node) []*Node {
	out := make([]*Node, 0, 1+len(n.children))
	out = append(out, n)
	for _, c := range n.children {
		out = append(out, subtreeOf(c)...)
	}
	return out
}

// onlyFolderNote refuses a plain delete of a node whose folder holds
// anything besides its folder note: child nodes, a folder that did not
// load, any other file. So a plain delete never removes something the
// caller could not see. Dot-entries are not content.
func onlyFolderNote(folder string, n *Node) error {
	kids, other, err := folderContents(folder, n)
	if err != nil {
		return fmt.Errorf("deleting %s: %w", n.Path, err)
	}
	const advice = "add --recursive to delete it with everything in its folder"
	has := strconv.Itoa(kids) + " children"
	if kids == 1 {
		has = "1 child"
	}
	switch {
	case kids > 0 && other != "":
		return fieldErr("stem", "node %q has %s and its folder also holds %q; %s", n.Stem, has, other, advice)
	case kids > 0:
		return fieldErr("stem", "node %q has %s; %s", n.Stem, has, advice)
	case other != "":
		return fieldErr("stem", "the folder of node %q also holds %q, which is not a loaded node; %s", n.Stem, other, advice)
	}
	return nil
}

// folderContents says what n's folder holds besides its folder note and
// dot-entries: how many loaded child nodes, and the first entry by name
// that is anything else ("" if none).
func folderContents(folder string, n *Node) (kids int, other string, err error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return 0, "", err
	}
	children := map[string]bool{}
	for _, c := range n.children {
		children[c.Stem] = true
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasPrefix(name, "."), name == n.Stem+".md":
		case children[name]:
			kids++
		case other == "":
			other = name // entries come sorted: the first by name
		}
	}
	return kids, other, nil
}

// hiddenBeside returns an unused dot-name beside folder to rename it to.
// The reader and Obsidian ignore dot-entries. A name left by an
// interrupted delete is never reused.
func hiddenBeside(folder string) (string, error) {
	base := filepath.Join(filepath.Dir(folder), "."+filepath.Base(folder)+".deleting")
	name := base
	for i := 1; ; i++ {
		_, err := os.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			return name, nil
		}
		if err != nil {
			return "", fmt.Errorf("deleting %s: %w", filepath.Base(folder), err)
		}
		name = base + "." + strconv.Itoa(i)
	}
}
