package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// moveBlockXML is <memory_move_block> (Part III.8): the block, the node it
// left, the node it is in now, and the number of links rewritten, zero
// included.
type moveBlockXML struct {
	XMLName xml.Name `xml:"memory_move_block"`
	ID      string   `xml:"id,attr"`
	From    string   `xml:"from,attr"`
	Stem    string   `xml:"stem,attr"`
	Links   int      `xml:"links_rewritten,attr"`
}

// flagTo is move-block's one flag: the node the block goes to.
const flagTo = "to"

// moveBlockArgs are move-block's parsed arguments, and --to as it was
// typed.
type moveBlockArgs struct {
	id, to string
	toFlag string
}

// parseMoveBlockArgs takes move-block's arguments: ID, as read prints it
// and not normalised, and --to STEM. The root holds no blocks, so --to /
// is refused here, before the store is looked at.
func parseMoveBlockArgs(args []string) (moveBlockArgs, error) {
	ma := moveBlockArgs{toFlag: "--" + flagTo}
	flags := flag.NewFlagSet("move-block", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&ma.to, flagTo, "", "")
	pos, typed, err := parseArgs(flags, args)
	if err != nil {
		return ma, err
	}
	name, given := typed[flagTo]
	if given {
		ma.toFlag = name
	}
	switch {
	case len(pos) == 0:
		return ma, errors.New("ID: move-block needs the ID of the block to move, as read prints it (m-7f3a2k)")
	case len(pos) > 1:
		return ma, fmt.Errorf("%q: move-block takes one ID, got %d arguments", pos[1], len(pos))
	case pos[0] == "":
		return ma, errors.New("ID: needs a block ID, as read prints it (m-7f3a2k)")
	case !given:
		return ma, fmt.Errorf("%s: move-block needs the stem of the node to move the block to", ma.toFlag)
	case ma.to == "":
		return ma, fmt.Errorf("%s: needs the stem of a node", ma.toFlag)
	case ma.to == rootArg:
		return ma, fmt.Errorf("%s: / is the root, which holds no blocks; give the stem of a node", ma.toFlag)
	}
	ma.id = pos[0]
	return ma, nil
}

// moveBlockVerb is III.8's move-block: the block goes to the end of the
// node --to, keeping its ID, and every link to it is rewritten. Argument
// errors come before the store is looked at; then a missing store is
// created empty (Part III.1), on which every ID is unknown. The library's
// refusals are renamed to the arguments as typed.
func moveBlockVerb(loc location, args []string) ([]byte, error) {
	ma, err := parseMoveBlockArgs(args)
	if err != nil {
		return nil, err
	}
	w, err := writerAt(loc)
	if err != nil {
		return nil, err
	}
	res, err := w.MoveBlock(ma.id, ma.to)
	if err != nil {
		return nil, ma.argError(err)
	}
	return render(moveBlockXML{ID: ma.id, From: res.From, Stem: res.To, Links: res.Links})
}

// argError renames the library's refusals to the arguments as typed: an
// unknown node is --to's, with near matches (Part III.3); a field error is
// about ID or about --to. Any other error, busy and changed on disk
// included, is returned as is.
func (ma moveBlockArgs) argError(err error) error {
	var none *memory.NoNodeError
	if errors.As(err, &none) {
		return unknownNodeAmong(ma.toFlag, none.Stem, none.Stems)
	}
	var fe *memory.FieldError
	if !errors.As(err, &fe) {
		return err
	}
	if fe.Field == flagTo {
		return fmt.Errorf("%s: %s", ma.toFlag, fe.Reason)
	}
	return fmt.Errorf("ID: %s", fe.Reason)
}
