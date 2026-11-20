package memory

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	linkOpen  = "[["
	linkClose = "]]"
	embedOpen = "![["
)

var linkBlockIDRE = regexp.MustCompile(`^m-[a-z0-9]{6}$`)

// linkSpan is one edge as written: text[start:end] is its [[…]].
type linkSpan struct {
	edge       Edge
	start, end int
}

// scanLinks finds the edges in a block's text, in order, skipping inline
// code spans. Other [[…]] forms and every embed stay plain text and give
// one warning per occurrence.
func scanLinks(text string) ([]Edge, []string) {
	spans, warnings := scanLinkSpans(text)
	if len(spans) == 0 {
		return nil, warnings
	}
	edges := make([]Edge, 0, len(spans))
	for _, s := range spans {
		edges = append(edges, s.edge)
	}
	return edges, warnings
}

// scanLinkSpans is the link scanner: scanLinks' edges, each with where it
// is written in text.
func scanLinkSpans(text string) ([]linkSpan, []string) {
	var (
		spans    []linkSpan
		warnings []string
	)
	for i := 0; i < len(text); {
		switch {
		case text[i] == '`':
			i = skipCodeSpan(text, i)
		case strings.HasPrefix(text[i:], embedOpen):
			inner, next, ok := wikiInner(text, i+len(embedOpen))
			if ok {
				warnings = append(warnings, fmt.Sprintf("embed ![[%s]] is not supported; kept as plain text", inner))
			}
			i = next
		case strings.HasPrefix(text[i:], linkOpen):
			inner, next, ok := wikiInner(text, i+len(linkOpen))
			if ok {
				if e, valid := parseLink(inner); valid {
					spans = append(spans, linkSpan{edge: e, start: i, end: next})
				} else {
					warnings = append(warnings, fmt.Sprintf("link [[%s]] is not a supported form ([[stem]], [[stem|alias]], [[stem#^m-xxxxxx]]); kept as plain text", inner))
				}
			}
			i = next
		default:
			i++
		}
	}
	return spans, warnings
}

// wikiInner returns the text between an opening [[ (ending at from) and
// the next ]] on the same line. If there is none, or another [[ opens
// first, the opening is literal text and scanning resumes at from.
func wikiInner(text string, from int) (inner string, next int, ok bool) {
	end := strings.Index(text[from:], linkClose)
	if end < 0 {
		return "", from, false
	}
	inner = text[from : from+end]
	if strings.Contains(inner, "\n") || strings.Contains(inner, linkOpen) {
		return "", from, false
	}
	return inner, from + end + len(linkClose), true
}

// parseLink reads [[stem]], [[stem|alias]], [[stem#^m-xxxxxx]] and
// [[stem#^m-xxxxxx|alias]].
func parseLink(inner string) (Edge, bool) {
	target, _, _ := strings.Cut(inner, "|")
	if validStem(target) {
		return Edge{Stem: target}, true
	}
	stem, id, found := strings.Cut(target, "#^")
	if found && validStem(stem) && linkBlockIDRE.MatchString(id) {
		return Edge{Stem: stem, Block: id}, true
	}
	return Edge{}, false
}

// skipCodeSpan returns the index after the inline code span opening at i:
// a run of n backticks closed by the next run of exactly n. An unclosed
// run is literal text.
func skipCodeSpan(text string, i int) int {
	n := backtickRun(text, i)
	for k := i + n; k < len(text); {
		if text[k] != '`' {
			k++
			continue
		}
		m := backtickRun(text, k)
		if m == n {
			return k + m
		}
		k += m
	}
	return i + n
}

func backtickRun(text string, i int) int {
	n := 0
	for i+n < len(text) && text[i+n] == '`' {
		n++
	}
	return n
}

// resolveLinks marks dangling edges and builds backlinks, once every node
// has loaded.
func (l *loader) resolveLinks() {
	st := l.store
	for _, n := range st.order {
		for bi := range n.Blocks {
			b := &n.Blocks[bi]
			src := Ref{Stem: n.Stem, Block: b.ID}
			for ei := range b.Edges {
				e := &b.Edges[ei]
				if !st.resolves(*e) {
					e.Dangling = true
					l.problemf(SeverityWarning, n.Path, "%s", danglingReason(*e))
					continue
				}
				st.addBacklink(Ref{Stem: e.Stem}, src)
				if e.Block != "" {
					st.addBacklink(Ref{Stem: e.Stem, Block: e.Block}, src)
				}
			}
		}
	}
}

func (s *Store) resolves(e Edge) bool {
	if e.Block == "" {
		_, ok := s.byStem[e.Stem]
		return ok
	}
	_, ok := s.Block(e.Stem, e.Block)
	return ok
}

// danglingReason is the warning an edge to a missing target is reported
// with.
func danglingReason(e Edge) string {
	return fmt.Sprintf("link %s points to a missing %s", linkText(e), missingWhat(e))
}

func linkText(e Edge) string {
	if e.Block == "" {
		return "[[" + e.Stem + "]]"
	}
	return "[[" + e.Stem + "#^" + e.Block + "]]"
}

func missingWhat(e Edge) string {
	if e.Block == "" {
		return "node"
	}
	return "block"
}
