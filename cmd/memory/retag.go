package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// retagXML is <memory_retag> (Part III.8): the old tag, the new one (no
// tag attribute for a removal), and the number of nodes changed, zero
// included.
type retagXML struct {
	XMLName xml.Name `xml:"memory_retag"`
	From    string   `xml:"from,attr"`
	Tag     string   `xml:"tag,attr,omitempty"`
	Nodes   int      `xml:"nodes,attr"`
}

// parseRetagArgs takes retag's arguments, OLD and an optional NEW, each a
// valid tag with or without a leading #. It takes no flags.
func parseRetagArgs(args []string) (old, new string, err error) {
	flags := flag.NewFlagSet("retag", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pos, _, err := parseArgs(flags, args)
	if err != nil {
		return "", "", err
	}
	const maxTags = 2
	switch {
	case len(pos) == 0:
		return "", "", errors.New("OLD: retag needs the tag to rename or remove")
	case len(pos) > maxTags:
		return "", "", fmt.Errorf("%q: retag takes OLD and at most one NEW, got %d arguments", pos[maxTags], len(pos))
	}
	if _, err := memory.CleanTag(pos[0]); err != nil {
		return "", "", fmt.Errorf("OLD: %w", err)
	}
	if len(pos) == 1 {
		return pos[0], "", nil
	}
	// An empty NEW is a bad tag, not a removal: removing is giving no NEW.
	if _, err := memory.CleanTag(pos[1]); err != nil {
		return "", "", fmt.Errorf("NEW: %w", err)
	}
	return pos[0], pos[1], nil
}

// retagVerb is III.8's retag: OLD becomes NEW on every node that has it,
// in frontmatter tags and in inline #tags, or is removed when no NEW is
// given. Argument errors come before the store is looked at; then a
// missing store is created empty (Part III.1), where nobody has the tag.
func retagVerb(loc location, args []string) ([]byte, error) {
	old, new, err := parseRetagArgs(args)
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
	res, err := w.Retag(old, new)
	if err != nil {
		return nil, retagArgError(err)
	}
	return render(retagXML{From: res.Old, Tag: res.New, Nodes: res.Nodes})
}

// retagArgError renames the library's field errors to OLD and NEW. The
// arguments are checked before the call, so this is a second line of
// defence; any other error is returned as is.
func retagArgError(err error) error {
	var fe *memory.FieldError
	if !errors.As(err, &fe) {
		return err
	}
	switch fe.Field {
	case "old":
		return fmt.Errorf("OLD: %s", fe.Reason)
	case "new":
		return fmt.Errorf("NEW: %s", fe.Reason)
	}
	return err
}
