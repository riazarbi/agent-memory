package memory

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// bodyPlan is a rewritten body: each block rendered with its line endings,
// in file order, appended blocks last.
type bodyPlan struct {
	rendered []string
	// changed is false when no block is appended, replaced with new text,
	// or given an ID: the body is then copied exactly as read.
	changed  bool
	blocks   []Block // the blocks the reader must load back (ID, Text)
	blockIDs []string
	adopted  []string
}

// planBody applies block edits to n's body. Untouched blocks keep their
// exact source lines; a replaced block keeps its ID; ID-less paragraphs and
// appended blocks get fresh IDs from gen.
func planBody(n *Node, edits []BlockEdit, gen *idGen) (bodyPlan, error) {
	var p bodyPlan
	replace := map[string]string{}
	for _, ed := range edits {
		if ed.ID != "" {
			replace[ed.ID] = ed.Text
		}
	}
	for _, b := range n.Blocks {
		if err := p.existing(b, replace, gen, n.LineEnding); err != nil {
			return p, err
		}
	}
	for _, ed := range edits {
		if ed.ID != "" {
			p.blockIDs = append(p.blockIDs, ed.ID)
			continue
		}
		id, err := gen.next()
		if err != nil {
			return p, err
		}
		p.add(Block{ID: id, Text: ed.Text}, renderBlock(ed.Text, id, n.LineEnding), true)
		p.blockIDs = append(p.blockIDs, id)
	}
	return p, nil
}

// existing plans one block already in the node. replace maps an ID to its
// new text; an entry is consumed by the ID's first occurrence.
func (p *bodyPlan) existing(b Block, replace map[string]string, gen *idGen, eol string) error {
	if text, ok := replace[b.ID]; ok && b.ID != "" {
		delete(replace, b.ID)
		if text != b.Text {
			p.add(Block{ID: b.ID, Text: text}, renderBlock(text, b.ID, eol), true)
			return nil
		}
	}
	if b.ID != "" {
		p.add(b, copyLines(b.Source, eol), false)
		return nil
	}
	id, err := gen.next()
	if err != nil {
		return err
	}
	p.adopted = append(p.adopted, id)
	p.add(Block{ID: id, Text: strings.TrimRight(b.Text, " \t")}, adoptLines(b.Source, id, eol), true)
	return nil
}

func (p *bodyPlan) add(want Block, rendered string, changed bool) {
	p.blocks = append(p.blocks, Block{ID: want.ID, Text: want.Text})
	p.rendered = append(p.rendered, rendered)
	p.changed = p.changed || changed
}

// copyLines copies a block's source lines; the file's last line, which may
// have no ending, gets eol.
func copyLines(src []string, eol string) string {
	return withEnding(strings.Join(src, ""), true, eol)
}

// adoptLines copies an ID-less paragraph and writes id at the end of its
// last line, which keeps its own line ending (eol if it had none).
func adoptLines(src []string, id, eol string) string {
	last := src[len(src)-1]
	ending := eol
	switch {
	case strings.HasSuffix(last, crlf):
		ending = crlf
	case strings.HasSuffix(last, lf):
		ending = lf
	}
	content := strings.TrimRight(strings.TrimSuffix(last, ending), " \t")
	return strings.Join(src[:len(src)-1], "") + content + " ^" + id + ending
}

// rewriteFrontmatter returns n's frontmatter document with the requested
// fields changed, a deep copy for the caller to re-emit (D26). A requested
// value that decodes equal to the current one is no change. It returns nil
// when nothing changes.
func rewriteFrontmatter(n *Node, req changeReq) *yaml.Node {
	doc := copyYAML(n.Frontmatter.Doc)
	m := doc.Content[0]
	changed := false
	if req.description != nil && *req.description != n.Description {
		replaceValue(doc, m, valueIndex(m, keyDescription), strNode(*req.description))
		changed = true
	}
	if req.tags != nil && !sameStrings(n.Tags, *req.tags) {
		setTags(doc, m, *req.tags)
		changed = true
	}
	if mergeProperties(n, doc, m, req.props) {
		changed = true
	}
	if !changed {
		return nil
	}
	return doc
}

// mergeProperties updates existing keys in place and appends new ones, in
// the (sorted) order given. It reports whether anything changed.
func mergeProperties(n *Node, doc, m *yaml.Node, props []Property) bool {
	changed := false
	for _, p := range props {
		i := valueIndex(m, p.Key)
		if i < 0 {
			m.Content = append(m.Content, strNode(p.Key), valueNode(p.Value))
			changed = true
			continue
		}
		if cur, _ := n.Property(p.Key); valuesEqual(cur, p.Value) {
			continue
		}
		replaceValue(doc, m, i, valueNode(p.Value))
		changed = true
	}
	return changed
}

// setTags replaces the tags value, or inserts tags directly after
// description when the node has none.
func setTags(doc, m *yaml.Node, tags []string) {
	if i := valueIndex(m, keyTags); i >= 0 {
		replaceValue(doc, m, i, listNode(tags))
		return
	}
	at := valueIndex(m, keyDescription) + 1
	m.Content = append(m.Content[:at], append([]*yaml.Node{strNode(keyTags), listNode(tags)}, m.Content[at:]...)...)
}

// valueIndex returns the index of key's value node in mapping m, or -1.
func valueIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i + 1
		}
	}
	return -1
}

// changeExpectation is what the rewritten note must load back as: stamp
// is the modified value it is given, "" when it keeps what it has.
func changeExpectation(n *Node, req changeReq, body bodyPlan, stamp string) (expectation, error) {
	want := expectation{description: n.Description, tags: n.Tags, blocks: body.blocks}
	want.modified, want.hasModified = modifiedField(n)
	if stamp != "" {
		want.modified, want.hasModified = stamp, true
	}
	if req.description != nil {
		want.description = *req.description
	}
	if req.tags != nil {
		want.tags = *req.tags
	}
	want.props = append([]Property(nil), n.Properties...)
	for _, p := range req.props {
		if i := propertyIndex(want.props, p.Key); i >= 0 {
			if !valuesEqual(want.props[i].Value, p.Value) {
				want.props[i].Value = p.Value
			}
			continue
		}
		want.props = append(want.props, p)
	}
	_, warnings, err := parseNote([]byte(n.Source))
	if err != nil {
		return want, fmt.Errorf("node %s no longer parses as loaded: %w", n.Stem, err)
	}
	want.warnings = warnings
	return want, nil
}

func propertyIndex(props []Property, key string) int {
	for i, p := range props {
		if p.Key == key {
			return i
		}
	}
	return -1
}
