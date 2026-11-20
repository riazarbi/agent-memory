package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// renameXML is <memory_rename> (Part III.8): the old stem, the stem the
// node has now, and the number of links rewritten, zero included.
type renameXML struct {
	XMLName xml.Name `xml:"memory_rename"`
	From    string   `xml:"from,attr"`
	Stem    string   `xml:"stem,attr"`
	Links   int      `xml:"links_rewritten,attr"`
}

// parseRenameArgs takes rename's arguments, STEM and NEW. It takes no
// flags. The root is neither, and NEW must be valid stem syntax (D25).
func parseRenameArgs(args []string) (stem, new string, err error) {
	flags := flag.NewFlagSet("rename", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pos, _, err := parseArgs(flags, args)
	if err != nil {
		return "", "", err
	}
	const stems = 2
	switch {
	case len(pos) == 0:
		return "", "", errors.New("STEM: rename needs the node's stem, then its new stem")
	case pos[0] == rootArg:
		return "", "", errors.New("STEM: / is the root, which is never renamed; give the stem of a node")
	case len(pos) == 1:
		return "", "", errors.New("NEW: rename needs the node's new stem after STEM")
	case len(pos) > stems:
		return "", "", fmt.Errorf("%q: rename takes STEM and NEW, got %d arguments", pos[stems], len(pos))
	case pos[1] == rootArg:
		return "", "", errors.New("NEW: / is the root, never a node's stem; give a lowercase-hyphenated stem")
	}
	if err := memory.CheckStem(pos[1]); err != nil {
		return "", "", fmt.Errorf("NEW: %w", err)
	}
	return pos[0], pos[1], nil
}

// renameVerb is III.8's rename: the node gets the stem NEW, and every
// link to it is rewritten. Argument errors come before the store is
// looked at; then a missing store is created empty (Part III.1), on which
// every stem is unknown.
func renameVerb(loc location, args []string) ([]byte, error) {
	stem, new, err := parseRenameArgs(args)
	if err != nil {
		return nil, err
	}
	w, err := writerAt(loc)
	if err != nil {
		return nil, err
	}
	res, err := w.Rename(stem, new)
	if err != nil {
		return nil, renameArgError(err)
	}
	return render(renameXML{From: res.Old, Stem: res.New, Links: res.Links})
}

// renameArgError renames the library's refusals to the arguments as
// typed: an unknown stem is STEM's, with near matches (Part III.3), and a
// field error is about NEW. Any other error, busy and changed on disk
// included, is returned as is.
func renameArgError(err error) error {
	var none *memory.NoNodeError
	if errors.As(err, &none) {
		return unknownStemAmong(none.Stem, none.Stems)
	}
	var fe *memory.FieldError
	if errors.As(err, &fe) {
		return fmt.Errorf("NEW: %s", fe.Reason)
	}
	return err
}
