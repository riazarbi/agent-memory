package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// writeXML is D29's memory_write result: one <assigned> per requested
// block in argv order, <adopted> for IDs given to ID-less operator
// paragraphs, and the node as read renders it at depth 0.
type writeXML struct {
	XMLName  xml.Name `xml:"memory_write"`
	Created  bool     `xml:"created,attr"`
	Stem     string   `xml:"stem,attr"`
	Assigned []string `xml:"assigned"`
	Adopted  []string `xml:"adopted"`
	Node     nodeXML
}

// The block flags: their relative order is the blocks' order.
const (
	flagAdd     = "add"
	flagReplace = "replace"
)

// blockArg is the argument that asked for one block: an --add, or a
// --replace with the ID it names.
type blockArg struct {
	flag string
	id   string
}

// writeArgs are write's parsed arguments: the library's request, the
// argument behind each requested block, and how each flag was typed.
type writeArgs struct {
	req    memory.WriteRequest
	blocks []blockArg
	typed  map[string]string
}

func (wa *writeArgs) block(arg blockArg, edit memory.BlockEdit) {
	wa.blocks = append(wa.blocks, arg)
	wa.req.Blocks = append(wa.req.Blocks, edit)
}

// writeFlags is write's flag set, writing into wa. The shipped tool file's
// description must name every flag in it (tools_test.go).
func writeFlags(wa *writeArgs) *flag.FlagSet {
	var tags []string
	flags := flag.NewFlagSet("write", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&wa.req.Parent, "parent", "", "")
	flags.Func("description", "", func(s string) error { wa.req.Description = &s; return nil })
	flags.Func("tag", "", func(s string) error { tags = append(tags, s); wa.req.Tags = &tags; return nil })
	flags.Func("property", "", func(s string) error {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return errors.New("want KEY=VALUE")
		}
		if wa.req.Properties == nil {
			wa.req.Properties = map[string]any{}
		}
		wa.req.Properties[k] = v // a repeated key: the last wins
		return nil
	})
	flags.Func(flagAdd, "", func(s string) error {
		wa.block(blockArg{flag: flagAdd}, memory.BlockEdit{Text: s})
		return nil
	})
	flags.Func(flagReplace, "", func(s string) error {
		id, text, ok := strings.Cut(s, "=")
		if !ok || id == "" {
			// An empty ID would make the library append, not replace.
			return errors.New("want ID=TEXT: a block ID, =, then the block's new text")
		}
		wa.block(blockArg{flag: flagReplace, id: id}, memory.BlockEdit{ID: id, Text: text})
		return nil
	})
	return flags
}

func parseWriteArgs(args []string) (writeArgs, error) {
	var wa writeArgs
	pos, typed, err := parseArgs(writeFlags(&wa), args)
	if err != nil {
		return wa, err
	}
	wa.typed = typed
	switch len(pos) {
	case 0:
		return wa, errors.New("STEM: write needs the stem of the node to create or update")
	case 1:
		wa.req.Stem = pos[0]
		return wa, nil
	default:
		return wa, fmt.Errorf("STEM: write takes one stem, got %d: %s (quote a value that holds spaces)", len(pos), strings.Join(pos, " "))
	}
}

// writeVerb is D29's memory_write: a new stem is a create, a loaded one an
// update. Argument errors come before the store is looked at, and the
// writer's field errors are renamed to the arguments as typed.
func writeVerb(args []string) ([]byte, error) {
	wa, err := parseWriteArgs(args)
	if err != nil {
		return nil, err
	}
	notes, err := notesDir()
	if err != nil {
		return nil, err
	}
	w, err := memory.WriterFor(notes)
	if err != nil {
		return nil, err
	}
	res, err := w.Write(wa.req)
	if err != nil {
		return nil, wa.argError(err)
	}
	return render(writeXML{
		Created:  res.Created,
		Stem:     res.Node.Stem,
		Assigned: res.BlockIDs,
		Adopted:  res.Adopted,
		Node:     nodeOf(res.Store, res.Node, 0),
	})
}

// argError renames a writer field error (stem, parent, description,
// tags[i], properties["k"], blocks[i]) to the argument the user typed.
// Any other error, busy and changed on disk included, is returned as is.
func (wa writeArgs) argError(err error) error {
	var fe *memory.FieldError
	if !errors.As(err, &fe) {
		return err
	}
	return fmt.Errorf("%s: %s", wa.argName(fe.Field), fe.Reason)
}

func (wa writeArgs) argName(field string) string {
	var i int
	switch {
	case field == "stem":
		return "STEM"
	case field == "parent", field == "description":
		return wa.flagName(field)
	case scanIndex(field, "tags[%d]", &i):
		return fmt.Sprintf("%s #%d", wa.flagName("tag"), i+1)
	case scanIndex(field, "blocks[%d]", &i) && i < len(wa.blocks):
		// Blocks are numbered from 1 across --add and --replace; a
		// replacement is named by the ID it gives.
		if b := wa.blocks[i]; b.flag == flagReplace {
			return wa.flagName(flagReplace) + " " + b.id
		}
		return fmt.Sprintf("%s #%d", wa.flagName(flagAdd), i+1)
	case strings.HasPrefix(field, "properties["):
		key, _ := strconv.QuotedPrefix(strings.TrimPrefix(field, "properties["))
		return strings.TrimSpace(wa.flagName("property") + " " + key)
	}
	return field
}

// scanIndex reads the index out of a field such as tags[2].
func scanIndex(field, format string, i *int) bool {
	n, err := fmt.Sscanf(field, format, i)
	return err == nil && n == 1
}

// flagName is the flag as the user typed it, or --name if not typed (a
// create missing --parent).
func (wa writeArgs) flagName(name string) string {
	if t, ok := wa.typed[name]; ok {
		return t
	}
	return "--" + name
}
