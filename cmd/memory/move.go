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

// moveXML is <memory_move> (Part III.8): the node, the parent it left and
// the parent it has now (/ for the root), and its path under notes/ as
// read gives it.
type moveXML struct {
	XMLName xml.Name `xml:"memory_move"`
	Stem    string   `xml:"stem,attr"`
	From    string   `xml:"from,attr"`
	Parent  string   `xml:"parent,attr"`
	Path    string   `xml:"path,attr"`
}

// flagParent is move's one flag: the node to move under, or the root.
const flagParent = "parent"

// moveArgs are move's parsed arguments, and --parent as it was typed.
type moveArgs struct {
	stem, parent string
	parentFlag   string
}

func parseMoveArgs(args []string) (moveArgs, error) {
	ma := moveArgs{parentFlag: "--" + flagParent}
	flags := flag.NewFlagSet("move", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&ma.parent, flagParent, "", "")
	pos, typed, err := parseArgs(flags, args)
	if err != nil {
		return ma, err
	}
	name, given := typed[flagParent]
	if given {
		ma.parentFlag = name
	}
	switch {
	case len(pos) == 0:
		return ma, errors.New("STEM: move needs the stem of the node to move")
	case len(pos) > 1:
		return ma, fmt.Errorf("STEM: move takes one stem, got %d: %s", len(pos), strings.Join(pos, " "))
	case pos[0] == rootArg:
		return ma, errors.New("STEM: / is the root, which is never moved; give the stem of a node")
	case !given:
		return ma, fmt.Errorf("%s: move needs the stem of the node to move it under, or / for the top level", ma.parentFlag)
	case ma.parent == "":
		return ma, fmt.Errorf("%s: needs the stem of a node, or / for the top level", ma.parentFlag)
	}
	ma.stem = pos[0]
	return ma, nil
}

// moveVerb is III.8's move: the node and everything in its folder go under
// --parent, / being the root. Argument errors come before the store is
// looked at; then a missing store is created empty (Part III.1), on which
// every stem is unknown. The library's refusals are renamed to the
// arguments as typed.
func moveVerb(loc location, args []string) ([]byte, error) {
	ma, err := parseMoveArgs(args)
	if err != nil {
		return nil, err
	}
	w, err := writerAt(loc)
	if err != nil {
		return nil, err
	}
	res, err := w.Move(ma.stem, ma.parent)
	if err != nil {
		return nil, ma.argError(err)
	}
	return render(moveXML{Stem: ma.stem, From: res.From, Parent: res.Parent, Path: res.Path})
}

// writerAt returns the writer for the store at loc, creating a missing
// store empty first (Part III.1).
func writerAt(loc location) (*memory.Writer, error) {
	notes, err := loc.notesDir()
	if err != nil {
		return nil, err
	}
	return memory.WriterFor(notes)
}

// argError renames the library's refusals to the arguments as typed: an
// unknown node is STEM's or --parent's (STEM is looked up first, so an
// unknown stem equal to STEM is STEM's), with near matches (Part III.3),
// and a field error is about --parent. Any other error, busy included, is
// returned as is.
func (ma moveArgs) argError(err error) error {
	var none *memory.NoNodeError
	if errors.As(err, &none) {
		if none.Stem == ma.stem {
			return unknownStemAmong(none.Stem, none.Stems)
		}
		return unknownNodeAmong(ma.parentFlag, none.Stem, none.Stems)
	}
	var fe *memory.FieldError
	if errors.As(err, &fe) {
		return fmt.Errorf("%s: %s", ma.parentFlag, fe.Reason)
	}
	return err
}
