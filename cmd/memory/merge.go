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

// mergeXML is <memory_merge> (Part III.8): the node that was folded in,
// the node that was kept, and the counts, zero included.
type mergeXML struct {
	XMLName   xml.Name `xml:"memory_merge"`
	From      string   `xml:"from,attr"`
	Stem      string   `xml:"stem,attr"`
	Blocks    int      `xml:"blocks,attr"`
	Children  int      `xml:"children,attr"`
	TagsAdded int      `xml:"tags_added,attr"`
	Links     int      `xml:"links_rewritten,attr"`
}

// mergeArgs are merge's parsed arguments, and --into as it was typed.
type mergeArgs struct {
	from, to string
	intoFlag string
}

// parseMergeArgs takes merge's arguments: FROM, the node to fold in, and
// --into TO, the node to keep. The root is neither, and is refused here,
// before the store is looked at.
func parseMergeArgs(args []string) (mergeArgs, error) {
	ma := mergeArgs{intoFlag: "--" + flagInto}
	flags := flag.NewFlagSet("merge", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&ma.to, flagInto, "", "")
	pos, typed, err := parseArgs(flags, args)
	if err != nil {
		return ma, err
	}
	name, given := typed[flagInto]
	if given {
		ma.intoFlag = name
	}
	switch {
	case len(pos) == 0:
		return ma, errors.New("FROM: merge needs the stem of the node to fold in")
	case len(pos) > 1:
		return ma, fmt.Errorf("FROM: merge takes one stem, got %d: %s (the node to keep is given with --into)", len(pos), strings.Join(pos, " "))
	case pos[0] == rootArg:
		return ma, errors.New("FROM: / is the root, which is never merged away; give the stem of a node")
	case !given:
		return ma, fmt.Errorf("%s: merge needs the stem of the node to merge %s into, the one that is kept", ma.intoFlag, pos[0])
	case ma.to == "":
		return ma, fmt.Errorf("%s: needs the stem of a node", ma.intoFlag)
	case ma.to == rootArg:
		return ma, fmt.Errorf("%s: / is the root, which holds no blocks; give the stem of the node to keep", ma.intoFlag)
	}
	ma.from = pos[0]
	return ma, nil
}

// mergeVerb is III.8's merge: FROM's blocks, children, links and tags are
// folded into --into, and FROM is removed. Argument errors come before the
// store is looked at; then a missing store is created empty (Part III.1),
// on which every stem is unknown. The library's refusals are renamed to
// the arguments as typed.
func mergeVerb(loc location, args []string) ([]byte, error) {
	ma, err := parseMergeArgs(args)
	if err != nil {
		return nil, err
	}
	w, err := writerAt(loc)
	if err != nil {
		return nil, err
	}
	res, err := w.Merge(ma.from, ma.to)
	if err != nil {
		return nil, ma.argError(err)
	}
	return render(mergeXML{
		From: ma.from, Stem: ma.to,
		Blocks: res.Blocks, Children: res.Children, TagsAdded: res.TagsAdded, Links: res.Links,
	})
}

// argError renames the library's refusals to the arguments as typed: an
// unknown node is FROM's or --into's (FROM is looked up first, so an
// unknown stem equal to FROM is FROM's), with near matches (Part III.3);
// a field error is about FROM or --into. Any other error, busy and changed
// on disk included, is returned as is.
func (ma mergeArgs) argError(err error) error {
	var none *memory.NoNodeError
	if errors.As(err, &none) {
		if none.Stem == ma.from {
			return unknownNodeAmong("FROM", none.Stem, none.Stems)
		}
		return unknownNodeAmong(ma.intoFlag, none.Stem, none.Stems)
	}
	var fe *memory.FieldError
	if !errors.As(err, &fe) {
		return err
	}
	if fe.Field == "from" {
		return fmt.Errorf("FROM: %s", fe.Reason)
	}
	return fmt.Errorf("%s: %s", ma.intoFlag, fe.Reason)
}
