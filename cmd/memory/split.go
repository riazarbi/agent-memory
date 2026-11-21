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

// splitXML is <memory_split> (Part III.8): the node the blocks left, the
// new node, its parent as the call named it (/ for the root), its path
// under notes/ as read gives it, and the counts, zero included.
type splitXML struct {
	XMLName xml.Name `xml:"memory_split"`
	From    string   `xml:"from,attr"`
	Stem    string   `xml:"stem,attr"`
	Parent  string   `xml:"parent,attr"`
	Path    string   `xml:"path,attr"`
	Blocks  int      `xml:"blocks,attr"`
	Left    int      `xml:"left,attr"`
	Links   int      `xml:"links_rewritten,attr"`
}

// split's and merge's flags; --parent is move's flagParent and --block
// delete's flagBlock.
const (
	flagInto        = "into"
	flagDescription = "description"
)

// splitArgs are split's parsed arguments, and how each flag was typed.
type splitArgs struct {
	req   memory.SplitRequest
	typed map[string]string
}

// flagName is the flag as the user typed it, or --name if it was not
// given.
func (sa splitArgs) flagName(name string) string {
	if t, ok := sa.typed[name]; ok {
		return t
	}
	return "--" + name
}

// parseSplitArgs takes split's arguments: STEM, --into NEW, --parent P,
// --description D and one --block ID or more, whose order is the blocks'
// order in the new node. All four flags are required: a split has no
// default direction. The root is a --parent only, and NEW must be valid
// stem syntax (D25); both are refused here, before the store is looked at.
func parseSplitArgs(args []string) (splitArgs, error) {
	var sa splitArgs
	flags := flag.NewFlagSet("split", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&sa.req.New, flagInto, "", "")
	flags.StringVar(&sa.req.Parent, flagParent, "", "")
	flags.StringVar(&sa.req.Description, flagDescription, "", "")
	flags.Func(flagBlock, "", func(s string) error { sa.req.Blocks = append(sa.req.Blocks, s); return nil })
	pos, typed, err := parseArgs(flags, args)
	if err != nil {
		return sa, err
	}
	sa.typed = typed
	switch {
	case len(pos) == 0:
		return sa, errors.New("STEM: split needs the stem of the node to split")
	case len(pos) > 1:
		return sa, fmt.Errorf("STEM: split takes one stem, got %d: %s (quote a value that holds spaces)", len(pos), strings.Join(pos, " "))
	case pos[0] == rootArg:
		return sa, errors.New("STEM: / is the root, which holds no blocks to split off; give the stem of a node")
	}
	if err := sa.missingFlag(pos[0]); err != nil {
		return sa, err
	}
	switch {
	case sa.req.New == rootArg:
		return sa, fmt.Errorf("%s: / is the root, never a node's stem; give a lowercase-hyphenated stem for the new node (--parent / puts it at the top level)", sa.flagName(flagInto))
	case sa.req.Parent == "":
		return sa, fmt.Errorf("%s: needs the stem of a node, or / for the top level", sa.flagName(flagParent))
	}
	if err := memory.CheckStem(sa.req.New); err != nil {
		return sa, fmt.Errorf("%s: %w", sa.flagName(flagInto), err)
	}
	sa.req.Stem = pos[0]
	return sa, nil
}

// missingFlag refuses a split that lacks one of its four flags, naming
// the first missing in the order the usage text gives them.
func (sa splitArgs) missingFlag(stem string) error {
	for _, need := range []struct{ flag, why string }{
		{flagInto, "split needs the stem of the new node the blocks go into"},
		{flagParent, "split needs to be told where the new node goes: the stem of the node to put it under (" + stem + " itself makes it a child), or / for the top level"},
		{flagDescription, "split needs a one-line description of the new node"},
		{flagBlock, "split needs at least one block of " + stem + " to move, by its ID as read prints it (m-7f3a2k); repeat it for several"},
	} {
		if _, given := sa.typed[need.flag]; !given {
			return fmt.Errorf("%s: %s", sa.flagName(need.flag), need.why)
		}
	}
	return nil
}

// splitVerb is III.8's split: the named blocks of STEM go, in argv order,
// into the new node --into, made under --parent. Argument errors come
// before the store is looked at; then a missing store is created empty
// (Part III.1), on which every stem is unknown. The library's refusals are
// renamed to the arguments as typed.
func splitVerb(loc location, args []string) ([]byte, error) {
	sa, err := parseSplitArgs(args)
	if err != nil {
		return nil, err
	}
	w, err := writerAt(loc)
	if err != nil {
		return nil, err
	}
	res, err := w.Split(sa.req)
	if err != nil {
		return nil, sa.argError(err)
	}
	return render(splitXML{
		From: sa.req.Stem, Stem: sa.req.New, Parent: sa.req.Parent, Path: res.Path,
		Blocks: res.Blocks, Left: res.Left, Links: res.Links,
	})
}

// argError renames the library's refusals to the arguments as typed: an
// unknown node is STEM's or --parent's (STEM is looked up first, so an
// unknown stem equal to STEM is STEM's), with near matches (Part III.3);
// a field error is about STEM or the flag of its field. Any other error,
// busy and changed on disk included, is returned as is.
func (sa splitArgs) argError(err error) error {
	var none *memory.NoNodeError
	if errors.As(err, &none) {
		if none.Stem == sa.req.Stem {
			return unknownStemAmong(none.Stem, none.Stems)
		}
		return unknownNodeAmong(sa.flagName(flagParent), none.Stem, none.Stems)
	}
	var fe *memory.FieldError
	if !errors.As(err, &fe) {
		return err
	}
	switch fe.Field {
	case flagInto, flagParent, flagDescription, flagBlock:
		return fmt.Errorf("%s: %s", sa.flagName(fe.Field), fe.Reason)
	}
	return fmt.Errorf("STEM: %s", fe.Reason)
}
