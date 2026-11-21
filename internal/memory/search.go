package memory

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Matching for memory_search (docs/spec/agent-memory.md D31). Pure over a
// loaded *Store: it reads nothing from disk, and the root and every node
// the loader skipped are never results, because they are not in the store.

// Result is one matching node and its hit blocks: every block holding a
// positive word of an alternative the node matched, or an inline tag a
// positive tag: term of such an alternative matches — in file order, all
// of them (capping is the caller's).
type Result struct {
	Node *Node
	Hits []Block
}

// Search returns the nodes q matches, ranked: most hit blocks first, then
// the newest node (the node's date, III.9, at full precision), then stem.
func Search(st *Store, q *Query) []Result {
	var out []Result
	for _, n := range st.Nodes() {
		doc := newSearchDoc(n)
		matched := false
		hit := make([]bool, len(n.Blocks))
		for _, alt := range q.alts {
			if !doc.matchesAll(alt) {
				continue
			}
			matched = true
			doc.markHits(alt, hit)
		}
		if !matched {
			continue
		}
		r := Result{Node: n}
		for i, b := range n.Blocks {
			if hit[i] {
				r.Hits = append(r.Hits, b)
			}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if len(a.Hits) != len(b.Hits) {
			return len(a.Hits) > len(b.Hits)
		}
		if !a.Node.Modified.Equal(b.Node.Modified) {
			return a.Node.Modified.After(b.Node.Modified)
		}
		return a.Node.Stem < b.Node.Stem
	})
	return out
}

// searchDoc is a node's searchable text, each field normalised once.
type searchDoc struct {
	stem, path, description string
	tags                    []string // frontmatter tags, each on its own
	blocks                  []string // block text, raw: links and #tags included
	blockTags               [][]string
	props                   []searchProp
}

// searchProp is one frontmatter property, description and tags included:
// its key and its value as written (the YAML scalar text), one entry per
// list item.
type searchProp struct {
	key    string
	values []string
}

func newSearchDoc(n *Node) *searchDoc {
	d := &searchDoc{
		stem:        foldText(n.Stem),
		path:        foldText(n.Path),
		description: foldText(n.Description),
	}
	for _, t := range n.Tags {
		d.tags = append(d.tags, foldText(t))
	}
	for _, b := range n.Blocks {
		d.blocks = append(d.blocks, foldText(b.Text))
		var tags []string
		for _, t := range inlineTags(b.Text) {
			tags = append(tags, foldText(t))
		}
		d.blockTags = append(d.blockTags, tags)
	}
	for _, f := range n.Frontmatter.Fields {
		d.props = append(d.props, searchProp{key: foldText(f.Key.Value), values: scalarTexts(f.Value)})
	}
	return d
}

// scalarTexts is a property value as written: the scalar's text, or each
// list item's. Aliases are followed.
func scalarTexts(v *yaml.Node) []string {
	v = deref(v)
	if len(v.Content) == 0 {
		return []string{foldText(v.Value)}
	}
	out := make([]string, 0, len(v.Content))
	for _, item := range v.Content {
		out = append(out, foldText(deref(item).Value))
	}
	return out
}

// matchesAll reports whether every positive term of alt matches and no
// excluded term does.
func (d *searchDoc) matchesAll(alt []term) bool {
	for _, tm := range alt {
		if d.matches(tm) == tm.exclude {
			return false
		}
	}
	return true
}

func (d *searchDoc) matches(tm term) bool {
	v := foldText(tm.value)
	switch tm.kind {
	case termTag:
		return d.hasTag(v)
	case termPath:
		return strings.Contains(d.path, v)
	case termFile:
		return strings.Contains(d.stem, v)
	case termProperty:
		return d.hasProperty(foldText(tm.key), v, tm.hasValue)
	}
	return d.hasWord(v)
}

// hasWord: the word occurs in the stem, the description, any one tag or
// any one block.
func (d *searchDoc) hasWord(w string) bool {
	if strings.Contains(d.stem, w) || strings.Contains(d.description, w) {
		return true
	}
	return anyContains(d.tags, w) || anyContains(d.blocks, w)
}

// hasTag: a frontmatter tag or an inline tag in any block is x, or nested
// under it (x/…).
func (d *searchDoc) hasTag(x string) bool {
	if anyTagMatches(d.tags, x) {
		return true
	}
	for _, tags := range d.blockTags {
		if anyTagMatches(tags, x) {
			return true
		}
	}
	return false
}

func (d *searchDoc) hasProperty(key, value string, hasValue bool) bool {
	for _, p := range d.props {
		if p.key != key {
			continue
		}
		if !hasValue || anyContains(p.values, value) {
			return true
		}
	}
	return false
}

// markHits marks every block holding a positive word of alt, or an inline
// tag a positive tag: term of alt matches.
func (d *searchDoc) markHits(alt []term, hit []bool) {
	for _, tm := range alt {
		if tm.exclude || (tm.kind != termWord && tm.kind != termTag) {
			continue
		}
		v := foldText(tm.value)
		for i := range d.blocks {
			if tm.kind == termWord && strings.Contains(d.blocks[i], v) ||
				tm.kind == termTag && anyTagMatches(d.blockTags[i], v) {
				hit[i] = true
			}
		}
	}
}

func anyContains(fields []string, s string) bool {
	for _, f := range fields {
		if strings.Contains(f, s) {
			return true
		}
	}
	return false
}

func anyTagMatches(tags []string, x string) bool {
	for _, t := range tags {
		if t == x || strings.HasPrefix(t, x+"/") {
			return true
		}
	}
	return false
}

// foldText is how every comparison sees text: lower case, and every run
// of whitespace collapsed to one space — never trimmed.
func foldText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) {
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// inlineTags returns a block's inline tags in order: a # at the start of
// the text or after whitespace, followed by the longest run of D17 tag
// characters, kept when that run is a valid tag (not empty, not only
// digits).
func inlineTags(text string) []string {
	spans := inlineTagSpans(text)
	if len(spans) == 0 {
		return nil
	}
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.tag)
	}
	return out
}

// tagSpan is one inline tag as written: text[start:end] is its #tag.
type tagSpan struct {
	tag        string
	start, end int
}

// inlineTagSpans is the inline-tag scanner: inlineTags' tags, each with
// where it is written in text.
func inlineTagSpans(text string) []tagSpan {
	var out []tagSpan
	prevSpace := true
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == '#' && prevSpace {
			end := i + size
			for end < len(text) {
				tr, ts := utf8.DecodeRuneInString(text[end:])
				if !tagRune(tr) {
					break
				}
				end += ts
			}
			if t, err := tag(text[i+size : end]); err == nil && end > i+size {
				out = append(out, tagSpan{tag: t, start: i, end: end})
			}
		}
		prevSpace = unicode.IsSpace(r)
		i += size
	}
	return out
}
