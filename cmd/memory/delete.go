package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// deleteXML is <memory_delete> (Part III.8): the node named in the call,
// the block for a block delete, the nodes and blocks removed in total
// (always present, zero included), and one <dangling> per link left
// pointing at something deleted.
type deleteXML struct {
	XMLName  xml.Name      `xml:"memory_delete"`
	Stem     string        `xml:"stem,attr"`
	Block    string        `xml:"block,attr,omitempty"`
	Nodes    int           `xml:"nodes,attr"`
	Blocks   int           `xml:"blocks,attr"`
	Dangling []danglingXML `xml:"dangling"`
}

// danglingXML is one broken link: the node and block that hold it (no
// block attribute when that block has no ID), and the link as written.
type danglingXML struct {
	Stem  string `xml:"stem,attr"`
	Block string `xml:"block,attr,omitempty"`
	Link  string `xml:",chardata"`
}

// deleteArgs are delete's parsed arguments, and how each flag was typed.
type deleteArgs struct {
	req   memory.DeleteRequest
	typed map[string]string
}

const (
	flagBlock     = "block"
	flagRecursive = "recursive"
)

func deleteFlags(da *deleteArgs) *flag.FlagSet {
	flags := flag.NewFlagSet("delete", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&da.req.Block, flagBlock, "", "")
	flags.BoolVar(&da.req.Recursive, flagRecursive, false, "")
	return flags
}

func parseDeleteArgs(args []string) (deleteArgs, error) {
	var da deleteArgs
	pos, typed, err := parseArgs(deleteFlags(&da), args)
	if err != nil {
		return da, err
	}
	da.typed = typed
	switch {
	case len(pos) == 0:
		return da, errors.New("STEM: delete needs the stem of the node to delete, or to delete a block from")
	case len(pos) > 1:
		return da, fmt.Errorf("STEM: delete takes one stem, got %d: %s", len(pos), strings.Join(pos, " "))
	case pos[0] == rootArg:
		return da, errors.New("STEM: / is the root, which is never deleted; give the stem of a node")
	}
	da.req.Stem = pos[0]
	block, hasBlock := typed[flagBlock]
	if hasBlock && da.req.Block == "" {
		return da, fmt.Errorf("%s: needs a block ID", block)
	}
	if hasBlock && da.req.Recursive {
		return da, fmt.Errorf("%s: deletes a node with everything under it, so it cannot be given with %s; give one or the other", typed[flagRecursive], block)
	}
	return da, nil
}

// deleteVerb is III.8's delete: one block with --block, else the node,
// and with --recursive everything in its folder. Argument errors come
// before the store is looked at; then a missing store is created empty
// (Part III.1), on which every stem is unknown. The library's refusals are
// renamed to the arguments as typed.
func deleteVerb(loc location, args []string) ([]byte, error) {
	da, err := parseDeleteArgs(args)
	if err != nil {
		return nil, err
	}
	notes, err := loc.notesDir()
	if err != nil {
		return nil, err
	}
	w, err := memory.WriterFor(notes)
	if err != nil {
		return nil, err
	}
	res, err := w.Delete(da.req)
	if err != nil {
		return nil, da.argError(err)
	}
	x := deleteXML{Stem: da.req.Stem, Block: da.req.Block, Nodes: res.Nodes, Blocks: res.Blocks}
	for _, d := range res.Dangling {
		x.Dangling = append(x.Dangling, danglingXML{Stem: d.Stem, Block: d.Block, Link: d.Link})
	}
	return render(x)
}

// argError renames the library's refusals to the arguments as typed: an
// unknown stem lists near matches (Part III.3), and a field error names
// STEM, --block or --recursive. Any other error, busy and changed on disk
// included, is returned as is.
func (da deleteArgs) argError(err error) error {
	var none *memory.NoNodeError
	if errors.As(err, &none) {
		return unknownStemAmong(none.Stem, none.Stems)
	}
	var fe *memory.FieldError
	if !errors.As(err, &fe) {
		return err
	}
	if fe.Field == flagBlock || fe.Field == flagRecursive {
		name, ok := da.typed[fe.Field]
		if !ok {
			name = "--" + fe.Field
		}
		return fmt.Errorf("%s: %s", name, fe.Reason)
	}
	// The library's only other refusal is about the node itself.
	return fmt.Errorf("STEM: %s", fe.Reason)
}
