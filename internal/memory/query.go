package memory

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The memory_search query language (docs/spec/agent-memory.md §4.5,
// "`memory_search`, exactly", D31): a subset of Obsidian's search syntax —
// words, "quoted phrases", -exclusions, OR, tag:, path:, file:,
// [property:value] and [property]. No parentheses, regex, line: or block:.

// QueryError is a query the parser refuses. Its text is D31's, exactly,
// naming the tool's one argument: "query: empty", "query: unclosed quote",
// and so on.
type QueryError struct{ msg string }

func (e *QueryError) Error() string { return e.msg }

// The six errors, in D31's order of precedence: when a query has several
// faults, the first in this list is reported.
var (
	errQueryEmpty     = &QueryError{"query: empty"}
	errUnclosedQuote  = &QueryError{"query: unclosed quote"}
	errUnclosedSquare = &QueryError{"query: unclosed ["}
	errEmptyTerm      = &QueryError{"query: empty term"}
	errORSides        = &QueryError{"query: OR needs a term on each side"}
	errOnlyExclusions = &QueryError{"query: nothing to match, only exclusions"}
)

// orToken is the separator, recognised only as this exact raw token.
const orToken = "OR"

// The operators, lowercase only (TAG:x is a plain word).
const (
	prefixTag  = "tag:"
	prefixPath = "path:"
	prefixFile = "file:"
)

type termKind int

const (
	termWord termKind = iota
	termTag
	termPath
	termFile
	termProperty
)

// term is one search term. value is as written once quotes are removed,
// untrimmed: words and operator values are compared with their
// whitespace (" cat" needs the space). For a property, key and value
// are trimmed (D31) and hasValue is false for [property] and [property:].
type term struct {
	kind     termKind
	exclude  bool
	value    string
	key      string
	hasValue bool
}

// Query is a parsed query: alternatives separated by OR, each a list of
// terms that must all hold (the implicit AND binds tighter than OR).
type Query struct {
	alts [][]term
}

// rawToken is one token as written: quotes still in place. bracket marks
// a property bracket.
type rawToken struct {
	text    string
	bracket bool
}

// ParseQuery parses a memory_search query. Its errors are *QueryError
// with D31's exact text; the checks run in D31's order of precedence over
// the whole query, and the first failure is reported.
func ParseQuery(query string) (*Query, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errQueryEmpty
	}
	toks, unclosedQuote, unclosedSquare := tokenize(query)
	switch {
	case unclosedQuote:
		return nil, errUnclosedQuote
	case unclosedSquare:
		return nil, errUnclosedSquare
	}
	items := make([]*term, len(toks)) // nil marks OR
	for i, tok := range toks {
		if !tok.bracket && tok.text == orToken {
			continue
		}
		tm, ok := classify(tok)
		if !ok {
			return nil, errEmptyTerm
		}
		items[i] = &tm
	}
	alts, err := splitAlternatives(items)
	if err != nil {
		return nil, err
	}
	return &Query{alts: alts}, nil
}

// splitAlternatives splits the terms at each OR (a nil item). A leading,
// trailing or doubled OR is an error, checked over the whole query before
// any alternative is judged for having no positive term.
func splitAlternatives(items []*term) ([][]term, error) {
	var alts [][]term
	var cur []term
	for i, it := range items {
		if it == nil {
			if i == 0 || i == len(items)-1 || items[i-1] == nil {
				return nil, errORSides
			}
			alts = append(alts, cur)
			cur = nil
			continue
		}
		cur = append(cur, *it)
	}
	alts = append(alts, cur)
	for _, alt := range alts {
		if !hasPositive(alt) {
			return nil, errOnlyExclusions
		}
	}
	return alts, nil
}

func hasPositive(alt []term) bool {
	for _, tm := range alt {
		if !tm.exclude {
			return true
		}
	}
	return false
}

// tokenize splits query at whitespace, except inside a quoted span or a
// property bracket. A quoted span runs from any " to the next; a property
// bracket is a token whose first character (after an optional -) is [
// and whose second is not [, and it runs to the first ], quotes
// notwithstanding. It reports an unpaired quote (inside a bracket too)
// and a bracket with no ]; an unclosed bracket holding an odd quote is an
// unclosed quote, the higher-precedence error.
func tokenize(query string) (toks []rawToken, unclosedQuote, unclosedSquare bool) {
	i := 0
	for i < len(query) {
		r, size := utf8.DecodeRuneInString(query[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		start := i
		if open, ok := bracketStart(query, i); ok {
			end := strings.IndexByte(query[open:], ']')
			if end < 0 {
				if strings.Count(query[open:], `"`)%2 == 1 {
					return toks, true, false
				}
				return toks, unclosedQuote, true
			}
			i = open + end + 1
			if strings.Count(query[open:i], `"`)%2 == 1 {
				unclosedQuote = true
			}
			toks = append(toks, rawToken{text: query[start:i], bracket: true})
			continue
		}
		var closed bool
		i, closed = wordEnd(query, i)
		if !closed {
			unclosedQuote = true
		}
		toks = append(toks, rawToken{text: query[start:i]})
	}
	return toks, unclosedQuote, false
}

// bracketStart reports whether a token starting at i is a property
// bracket, and the index of its [.
func bracketStart(query string, i int) (int, bool) {
	if query[i] == '-' {
		i++
	}
	if i >= len(query) || query[i] != '[' {
		return 0, false
	}
	if i+1 < len(query) && query[i+1] == '[' {
		return 0, false // [[link]] is a plain word
	}
	return i, true
}

// wordEnd returns where the word token starting at i ends: at whitespace
// outside a quoted span, or the end of the query. closed is false when a
// quote has no partner (the span then runs to the end).
func wordEnd(query string, i int) (end int, closed bool) {
	for i < len(query) {
		if query[i] == '"' {
			k := strings.IndexByte(query[i+1:], '"')
			if k < 0 {
				return len(query), false
			}
			partner := i + 1 + k
			i = partner + 1 // past the closing quote
			continue
		}
		r, size := utf8.DecodeRuneInString(query[i:])
		if unicode.IsSpace(r) {
			break
		}
		i += size
	}
	return i, true
}

// classify turns a raw token (not OR) into a term. Operators are
// recognised on the raw token, before quotes are removed, so "tag:x" is a
// word. ok is false for an empty term: a word, operator value or property
// key that is empty once quotes are removed and whitespace trimmed.
func classify(tok rawToken) (term, bool) {
	body := tok.text
	var tm term
	if rest, ok := strings.CutPrefix(body, "-"); ok {
		tm.exclude = true
		body = rest
	}
	if tok.bracket {
		return classifyProperty(tm, body[1:len(body)-1])
	}
	tm.kind = termWord
	for _, op := range []struct {
		prefix string
		kind   termKind
	}{{prefixTag, termTag}, {prefixPath, termPath}, {prefixFile, termFile}} {
		if rest, ok := strings.CutPrefix(body, op.prefix); ok {
			tm.kind = op.kind
			body = rest
			break
		}
	}
	tm.value = unquote(body)
	if tm.kind == termTag {
		tm.value = strings.TrimPrefix(tm.value, "#")
	}
	return tm, strings.TrimSpace(tm.value) != ""
}

// classifyProperty reads a bracket's inside: key, then an optional value
// after the first : outside quotes. Each is trimmed of surrounding
// whitespace, then its quote characters removed. An empty value is
// [property]; an empty key is an empty term.
func classifyProperty(tm term, inner string) (term, bool) {
	tm.kind = termProperty
	key, value, hasColon := cutOutsideQuotes(inner, ':')
	tm.key = unquote(strings.TrimSpace(key))
	if hasColon {
		tm.value = unquote(strings.TrimSpace(value))
		tm.hasValue = strings.TrimSpace(tm.value) != ""
	}
	if !tm.hasValue {
		tm.value = ""
	}
	return tm, strings.TrimSpace(tm.key) != ""
}

// cutOutsideQuotes cuts s at the first sep that is not inside a quoted
// span.
func cutOutsideQuotes(s string, sep byte) (before, after string, found bool) {
	quoted := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case sep:
			if !quoted {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}

// unquote removes the quote characters of a token whose quotes pair:
// each quoted span keeps its content.
func unquote(s string) string { return strings.ReplaceAll(s, `"`, "") }
