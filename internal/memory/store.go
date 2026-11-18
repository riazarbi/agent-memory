// Package memory is the agent's memory store read model: it loads a
// memory/notes/ tree into an in-memory store of nodes, blocks, properties,
// links and backlinks, following docs/spec/agent-memory.md §4.2 "Reading
// the store" (D25).
//
// Loading is total: Load never fails and never panics. Everything it cannot
// load is reported as a Problem — an error leaves a node (and its subtree)
// out of the store, a warning loads it and reports. The store is
// protocol-neutral (D23): block text and extra properties are carried as
// loaded and never interpreted.
package memory

import (
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// Severity classifies a Problem.
type Severity string

const (
	// SeverityError means the named node (and its subtree) is not in the
	// store, or notes/ itself could not be read.
	SeverityError Severity = "error"
	// SeverityWarning means the thing was loaded (or harmlessly ignored)
	// and is reported so the operator knows.
	SeverityWarning Severity = "warning"
)

// Problem is one thing the loader could not load as written.
type Problem struct {
	Severity Severity
	// Path is relative to notes/, slash-separated: a node's folder
	// ("user/dup"), a stray file ("topics/notes.txt"), or "." for notes/
	// itself.
	Path string
	// Reason is human-readable.
	Reason string
}

// Property is a frontmatter key other than description and tags, with its
// value as loaded: string, int, float64, bool, time.Time (unquoted YAML
// dates and datetimes), nil (empty), or []string (a list, items read as
// their text).
type Property struct {
	Key   string
	Value any
}

// Frontmatter is a node's frontmatter as read: everything a writer needs to
// re-emit it faithfully (docs/spec/agent-memory.md §4.2 "Round-trip of
// frontmatter", D26). A write that changes no field copies Raw; one that
// changes fields re-emits Doc, changing only those fields. Nothing in this
// package writes.
type Frontmatter struct {
	// Raw is the source text between the opening and closing --- lines,
	// byte for byte: comments, spacing, quoting and line endings (CRLF
	// included) as in the file. A byte-order mark before the opening line
	// is not part of it.
	Raw string
	// Doc is the parsed YAML document node. Fields' nodes are its own
	// nodes, in place; comments yaml.v3 attaches to the document or the
	// mapping rather than to a property live only here.
	Doc *yaml.Node
	// Fields are every property — description and tags included — in file
	// order.
	Fields []Field
}

// Field is one frontmatter property's YAML key and value nodes as parsed:
// scalar text, tag, style and comments intact. A value given through a
// YAML alias stays the alias node.
type Field struct {
	Key   *yaml.Node
	Value *yaml.Node
}

// Field returns the property named key.
func (f *Frontmatter) Field(key string) (Field, bool) {
	for _, fd := range f.Fields {
		if fd.Key.Value == key {
			return fd, true
		}
	}
	return Field{}, false
}

// Block is one paragraph of a node's body.
type Block struct {
	// ID is the block's ^m-xxxxxx ID without the caret; empty if it has
	// none.
	ID string
	// Text is the paragraph with the ID removed; its lines are joined
	// with "\n".
	Text string
	// Edges are the block's outgoing links, in order of occurrence.
	Edges []Edge
	// Source is the block's exact source lines, each with its own line
	// ending as in the file (the file's last line may have none), so a
	// writer can copy an untouched block byte for byte (D27).
	Source []string
}

// Edge is one outgoing link occurrence.
type Edge struct {
	// Stem is the target node.
	Stem string
	// Block is the target block ID, or empty for a link to the node.
	Block string
	// Dangling is true when the target node or block does not exist.
	Dangling bool
}

// Ref names a link source: a node and one of its blocks. Block is empty
// when the source block has no ID.
type Ref struct {
	Stem  string
	Block string
}

// Node is a folder plus its folder note.
type Node struct {
	Stem string
	// Path is the node's folder relative to notes/, slash-separated.
	Path        string
	Description string
	Tags        []string
	// Properties are the other frontmatter keys, in file order.
	Properties []Property
	// Modified is the folder note's mtime.
	Modified time.Time
	// Blocks are the body's paragraphs, in file order.
	Blocks []Block
	// Frontmatter is the frontmatter as read, kept for a writer to re-emit
	// faithfully (D26). Description, Tags and Properties are its decoded
	// values.
	Frontmatter Frontmatter
	// Source is the folder note's full text after any byte-order mark, as
	// read: an unchanged node is re-emitted from it byte for byte (D26).
	Source string
	// BOM reports whether the folder note began with a byte-order mark.
	BOM bool
	// LineEnding is the ending of the opening --- line, "\n" or "\r\n":
	// the one a writer uses for every line it writes (D27).
	LineEnding string

	parent   *Node
	children []*Node
}

// Parent returns the node's parent, or nil for a first-level node.
func (n *Node) Parent() *Node { return n.parent }

// Children returns the node's children, ordered by stem.
func (n *Node) Children() []*Node { return append([]*Node(nil), n.children...) }

// Property returns the value of an extra property.
func (n *Node) Property(key string) (any, bool) {
	for _, p := range n.Properties {
		if p.Key == key {
			return p.Value, true
		}
	}
	return nil, false
}

// Edges returns the node's outgoing edges: every block's, in file order.
func (n *Node) Edges() []Edge {
	var out []Edge
	for _, b := range n.Blocks {
		out = append(out, b.Edges...)
	}
	return out
}

// block returns the first block carrying id.
func (n *Node) block(id string) (Block, bool) {
	if id == "" {
		return Block{}, false
	}
	for _, b := range n.Blocks {
		if b.ID == id {
			return b, true
		}
	}
	return Block{}, false
}

// Store is a loaded notes/ tree.
type Store struct {
	byStem     map[string]*Node
	order      []*Node // load order: component path order
	firstLevel []*Node
	problems   []Problem
	backlinks  map[Ref]map[Ref]struct{} // target (stem, block or "") -> sources
}

func newStore() *Store {
	return &Store{
		byStem:    map[string]*Node{},
		backlinks: map[Ref]map[Ref]struct{}{},
	}
}

// Node looks a node up by stem.
func (s *Store) Node(stem string) (*Node, bool) {
	n, ok := s.byStem[stem]
	return n, ok
}

// Nodes returns every loaded node in path order (component by component).
func (s *Store) Nodes() []*Node { return append([]*Node(nil), s.order...) }

// FirstLevel returns the first-level nodes, ordered by stem.
func (s *Store) FirstLevel() []*Node { return append([]*Node(nil), s.firstLevel...) }

// Problems returns every problem, ordered by path, then in the order found.
func (s *Store) Problems() []Problem { return append([]Problem(nil), s.problems...) }

// Block resolves (stem, block ID); with a duplicate ID in the node, the
// first occurrence answers.
func (s *Store) Block(stem, id string) (Block, bool) {
	n, ok := s.byStem[stem]
	if !ok {
		return Block{}, false
	}
	return n.block(id)
}

// BlockEdges returns the outgoing edges of block (stem, id).
func (s *Store) BlockEdges(stem, id string) ([]Edge, bool) {
	b, ok := s.Block(stem, id)
	if !ok {
		return nil, false
	}
	return append([]Edge(nil), b.Edges...), true
}

// Backlinks returns the unique sources of every non-dangling edge into the
// node, whether the edge targets the node or one of its blocks, ordered by
// stem then block ID.
func (s *Store) Backlinks(stem string) []Ref {
	return sortedRefs(s.backlinks[Ref{Stem: stem}])
}

// BlockBacklinks returns the unique sources of the edges that target block
// (stem, id), ordered by stem then block ID.
func (s *Store) BlockBacklinks(stem, id string) []Ref {
	if id == "" {
		return nil
	}
	return sortedRefs(s.backlinks[Ref{Stem: stem, Block: id}])
}

func (s *Store) addBacklink(target, source Ref) {
	set := s.backlinks[target]
	if set == nil {
		set = map[Ref]struct{}{}
		s.backlinks[target] = set
	}
	set[source] = struct{}{}
}

func sortedRefs(set map[Ref]struct{}) []Ref {
	out := make([]Ref, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Stem != out[j].Stem {
			return out[i].Stem < out[j].Stem
		}
		return out[i].Block < out[j].Block
	})
	return out
}
