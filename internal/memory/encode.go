package memory

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// NewNode is the content of a node to create (D27 "A new node").
type NewNode struct {
	Description string
	// Tags may carry a leading '#', which is stripped.
	Tags []string
	// Properties are the extra frontmatter properties: each value a
	// string, number, bool, nil, or list of strings (as JSON decodes them).
	Properties map[string]any
	// Blocks are the block texts, each one plain paragraph without an ID.
	Blocks []string
}

// BlockEdit is one requested block change. With an empty ID, Text is
// appended as a new block; with an ID, it replaces the text of that block
// (its first occurrence, if the ID is duplicated in the node).
type BlockEdit struct {
	ID   string
	Text string
}

// Change is what a write asks of a loaded node (D27 "Rewriting a loaded
// node"). The zero Change asks for nothing.
type Change struct {
	// Description, if non-nil, replaces the description.
	Description *string
	// Tags, if non-nil, replaces the whole tag list; an empty list leaves
	// the node with tags: [].
	Tags *[]string
	// Properties are merged: a key the node has is updated in place, a new
	// key is appended (new keys in sorted order).
	Properties map[string]any
	// Blocks are appended or replace existing blocks, in order.
	Blocks []BlockEdit
}

// Encoded is the folder note an encode produced. Nothing is written.
type Encoded struct {
	// Bytes is the complete folder note.
	Bytes []byte
	// BlockIDs holds one ID per requested block, in request order: a new
	// ID for an appended (or new-node) block, the given ID for a
	// replacement.
	BlockIDs []string
	// Adopted holds the IDs given to existing paragraphs that had none,
	// in file order.
	Adopted []string
}

// Encoder turns nodes into the bytes of their folder notes (D27). It is
// pure: it writes no files and never changes Store.
type Encoder struct {
	// Store is the loaded store: every new block ID is unique against its
	// IDs, and EncodeChange rewrites its nodes.
	Store *Store
	// Rand is the random source for block IDs; nil means crypto/rand.
	Rand io.Reader
}

// testHookOutput, set only by tests, alters the assembled bytes before the
// final load check, to prove the check refuses what does not load.
var testHookOutput func([]byte) []byte

// EncodeNew encodes a new node: frontmatter with description, tags (if
// any), then the other properties in sorted key order, then modified, the
// time of the call (III.9); a blank line; each block followed by " ^<id>",
// blocks separated by one blank line; LF line endings, no byte-order mark.
// A node without blocks ends at the closing "---" line.
func (e Encoder) EncodeNew(n NewNode) (Encoded, error) {
	want, err := validNew(n)
	if err != nil {
		return Encoded{}, err
	}
	want.modified, want.hasModified = modifiedText(callTime()), true
	gen := e.idGen()
	ids := make([]string, len(n.Blocks))
	for i, text := range n.Blocks {
		if ids[i], err = gen.next(); err != nil {
			return Encoded{}, err
		}
		want.blocks = append(want.blocks, Block{ID: ids[i], Text: text})
	}
	out, err := newNote(want)
	if err != nil {
		return Encoded{}, err
	}
	return Encoded{Bytes: out, BlockIDs: ids}, nil
}

// newNote lays out a new node's folder note from what it must load back
// as (D27 "A new node"), and checks that it does.
func newNote(want expectation) ([]byte, error) {
	fm, err := encodeYAML(newFrontmatter(want))
	if err != nil {
		return nil, fmt.Errorf("encoding frontmatter: %w", err)
	}
	var b strings.Builder
	b.WriteString(delimiter + lf + fm + delimiter + lf)
	rendered := make([]string, len(want.blocks))
	for i, blk := range want.blocks {
		rendered[i] = paragraph(blk, lf)
	}
	if len(rendered) > 0 {
		b.WriteString(lf + strings.Join(rendered, lf))
	}
	return finish(b.String(), want)
}

const delimiter = "---"

func validNew(n NewNode) (expectation, error) {
	var want expectation
	if err := validDescription(n.Description); err != nil {
		return want, err
	}
	tags, err := validTags(n.Tags)
	if err != nil {
		return want, err
	}
	props, err := validProperties(n.Properties)
	if err != nil {
		return want, err
	}
	for i, text := range n.Blocks {
		if err := validBlockText(fmt.Sprintf("blocks[%d]", i), text); err != nil {
			return want, err
		}
	}
	return expectation{description: n.Description, tags: tags, props: props}, nil
}

// newFrontmatter lays out a new node's frontmatter document.
func newFrontmatter(want expectation) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, strNode(keyDescription), strNode(want.description))
	if len(want.tags) > 0 {
		m.Content = append(m.Content, strNode(keyTags), listNode(want.tags))
	}
	for _, p := range want.props {
		m.Content = append(m.Content, strNode(p.Key), valueNode(p.Value))
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{m}}
	if want.hasModified {
		stampDoc(doc, want.modified)
	}
	return doc
}

// renderBlock writes block text with its ID, every line ending in eol.
func renderBlock(text, id, eol string) string {
	return strings.ReplaceAll(text, "\n", eol) + " ^" + id + eol
}

func (e Encoder) idGen() *idGen {
	r := e.Rand
	if r == nil {
		r = rand.Reader
	}
	return newIDGen(r, e.Store.blockIDs())
}

// blockIDs returns every block ID in the store; a nil store has none.
func (s *Store) blockIDs() map[string]bool {
	ids := map[string]bool{}
	if s == nil {
		return ids
	}
	for _, n := range s.order {
		for _, b := range n.Blocks {
			if b.ID != "" {
				ids[b.ID] = true
			}
		}
	}
	return ids
}

// finish runs the final check on the assembled note and returns its bytes,
// or an error and no bytes.
func finish(note string, want expectation) ([]byte, error) {
	out := []byte(note)
	if testHookOutput != nil {
		out = testHookOutput(out)
	}
	if err := verifyOutput(out, want); err != nil {
		return nil, err
	}
	return out, nil
}

// EncodeChange encodes the loaded node stem with the change applied (D27,
// D26). With nothing to change, and every block already IDed, the output
// is the file exactly as read. Whether anything changes is judged without
// modified; when something does, modified is set to the time of the call
// (III.9).
func (e Encoder) EncodeChange(stem string, c Change) (Encoded, error) {
	if e.Store == nil {
		return Encoded{}, errors.New("EncodeChange needs the loaded store")
	}
	n, ok := e.Store.Node(stem)
	if !ok {
		return Encoded{}, fmt.Errorf("node %q is not in the store", stem)
	}
	req, err := validChange(n, c)
	if err != nil {
		return Encoded{}, err
	}
	body, err := planBody(n, req.blocks, e.idGen())
	if err != nil {
		return Encoded{}, err
	}
	doc := rewriteFrontmatter(n, req)
	stamp := ""
	if doc != nil || body.changed {
		stamp = n.stampFor(true, callTime())
	}
	want, err := changeExpectation(n, req, body, stamp)
	if err != nil {
		return Encoded{}, err
	}
	note, err := assemble(n, doc, stamp, body)
	if err != nil {
		return Encoded{}, err
	}
	out, err := finish(note, want)
	if err != nil {
		return Encoded{}, err
	}
	return Encoded{Bytes: out, BlockIDs: body.blockIDs, Adopted: body.adopted}, nil
}

// changeReq is a validated, normalised Change.
type changeReq struct {
	description *string
	tags        *[]string // stripped of '#'
	props       []Property
	blocks      []BlockEdit
}

func validChange(n *Node, c Change) (changeReq, error) {
	var req changeReq
	if c.Description != nil {
		if err := validDescription(*c.Description); err != nil {
			return req, err
		}
		req.description = c.Description
	}
	if c.Tags != nil {
		tags, err := validTags(*c.Tags)
		if err != nil {
			return req, err
		}
		req.tags = &tags
	}
	props, err := validProperties(c.Properties)
	if err != nil {
		return req, err
	}
	req.props = props
	if err := validEdits(n, c.Blocks); err != nil {
		return req, err
	}
	req.blocks = c.Blocks
	return req, nil
}

// validEdits checks each block edit's text, and that each replacement names
// a block of n at most once.
func validEdits(n *Node, edits []BlockEdit) error {
	replaced := map[string]bool{}
	for i, ed := range edits {
		field := fmt.Sprintf("blocks[%d]", i)
		if ed.ID != "" {
			if _, ok := n.block(ed.ID); !ok {
				return blockIDErr(i, "block ^%s is not in node %s; omit the id to append a new block", ed.ID, n.Stem)
			}
			if replaced[ed.ID] {
				return blockIDErr(i, "block ^%s is replaced more than once in this write", ed.ID)
			}
			replaced[ed.ID] = true
		}
		if err := validBlockText(field, ed.Text); err != nil {
			return err
		}
	}
	return nil
}

// assemble builds the rewritten note. Regions copied from the source keep
// their own bytes and line endings; regions written here use the opening
// delimiter line's ending. doc is the frontmatter to re-emit, nil when no
// field changes; stamp is the modified value to set, "" for none.
func assemble(n *Node, doc *yaml.Node, stamp string, body bodyPlan) (string, error) {
	eol := n.LineEnding
	head, err := headWith(n, doc, stamp)
	if err != nil {
		return "", fmt.Errorf("node %s: %w", n.Stem, err)
	}
	var b strings.Builder
	if n.BOM {
		b.WriteString(bomText)
	}
	b.WriteString(withEnding(head, body.changed, eol))
	if body.changed {
		b.WriteString(eol + strings.Join(body.rendered, eol))
	} else {
		b.WriteString(strings.Join(strings.SplitAfter(n.Source, "\n")[n.closingLine()+1:], ""))
	}
	return b.String(), nil
}

// withEnding returns s, with eol added if more follows and s does not end
// a line.
func withEnding(s string, more bool, eol string) string {
	if more && !strings.HasSuffix(s, "\n") {
		return s + eol
	}
	return s
}
