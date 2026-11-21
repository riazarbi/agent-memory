package memory

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The stored modified date (docs/spec/agent-memory.md Part III.9): a folder
// note carries, as the frontmatter property modified, the time its content
// last changed. Only this package sets it: to the time of the call on a
// content change, and from the file's time, once, on a mechanical rewrite
// of a note that has no readable value yet.

const (
	keyModified  = "modified"
	tagTimestamp = "!!timestamp"
	// modifiedLayout is the one form memory writes: UTC, to the second.
	modifiedLayout = "2006-01-02T15:04:05Z"
	// unreadableModified is the reader's warning for a value that is not a
	// date or date-time: the node loads and takes its date from the file.
	unreadableModified = "modified: cannot be read as a date or date-time; the file's time is used"
)

// now is the clock, a seam for tests.
var now = time.Now

// callTime is the time of a call as modified records it: UTC, cut to the
// second. A call that writes several notes reads it once.
func callTime() time.Time { return now().UTC().Truncate(time.Second) }

// modifiedText is t as memory writes it.
func modifiedText(t time.Time) string { return t.UTC().Format(modifiedLayout) }

// localLayouts are the forms read without an offset, taken in the local
// time zone as Obsidian writes them: a date-time with seconds optional, or
// a date, which is the start of that day.
var localLayouts = []string{
	"2006-01-02T15:04:05", "2006-01-02T15:04",
	"2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006-01-02",
}

// readModified reads a modified value: a single scalar that is an RFC 3339
// date-time with Z or an offset, or a date-time or date without one, taken
// in loc. It reads the scalar's text, quoted or not, never yaml.v3's
// decoded timestamp, which takes a value without an offset as UTC.
func readModified(v *yaml.Node, loc *time.Location) (time.Time, bool) {
	if v.Kind != yaml.ScalarNode {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v.Value); err == nil {
		return t, true
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, v.Value, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// modifiedField is the text of n's modified property as written, and
// whether n has the key. A value that is not a scalar has no text.
func modifiedField(n *Node) (string, bool) {
	f, ok := n.Frontmatter.Field(keyModified)
	if !ok {
		return "", false
	}
	if v := deref(f.Value); v.Kind == yaml.ScalarNode {
		return v.Value, true
	}
	return "", true
}

// stampFor is the modified value a rewrite of n's note writes, or "" when
// the note keeps what it has. A content change sets the time of the call,
// at. A mechanical rewrite (retargeted links, a renamed or removed tag)
// keeps a value that can be read, byte for byte; a note with none, or one
// that cannot be read, gets the time its file had before the call, so its
// date is the same after the call as before it.
func (n *Node) stampFor(content bool, at time.Time) string {
	switch {
	case content:
		return modifiedText(at)
	case n.dated:
		return ""
	}
	return modifiedText(n.Modified)
}

// closingLine is the index, among the lines of n.Source, of the --- line
// that closes the frontmatter.
func (n *Node) closingLine() int { return 1 + strings.Count(n.Frontmatter.Raw, "\n") }

// headWith returns n's frontmatter as a rewrite emits it, delimiter lines
// included. It is the one place modified is written into a loaded note, so
// a write and a restructuring call give the same bytes.
//
// doc is the frontmatter document to re-emit when the rewrite changes
// another field (D26), nil when it changes none. stamp is the modified
// value to set, "" to leave it alone. With no doc the frontmatter is the
// source's, byte for byte, except the one modified line; a note the line
// edit cannot serve (a flow mapping, a key written in an unusual form) is
// re-emitted with the key set instead.
func headWith(n *Node, doc *yaml.Node, stamp string) (string, error) {
	lines := strings.SplitAfter(n.Source, "\n")
	closing := n.closingLine()
	if doc == nil {
		if stamp == "" {
			return strings.Join(lines[:closing+1], ""), nil
		}
		if raw, ok := stampRaw(n, stamp); ok {
			return lines[0] + raw + lines[closing], nil
		}
		doc = copyYAML(n.Frontmatter.Doc)
	}
	if stamp != "" {
		stampDoc(doc, stamp)
	}
	text, err := encodeYAML(doc)
	if err != nil {
		return "", fmt.Errorf("encoding frontmatter: %w", err)
	}
	eol := n.LineEnding
	return delimiter + eol + strings.ReplaceAll(text, "\n", eol) + delimiter + eol, nil
}

// stampDoc sets modified in a frontmatter document about to be re-emitted:
// an existing key keeps its place, a new one goes last.
func stampDoc(doc *yaml.Node, stamp string) {
	m := doc.Content[0]
	v := &yaml.Node{Kind: yaml.ScalarNode, Tag: tagTimestamp, Value: stamp}
	if i := valueIndex(m, keyModified); i >= 0 {
		replaceValue(doc, m, i, v)
		return
	}
	m.Content = append(m.Content, strNode(keyModified), v)
}

// stampRaw returns n's frontmatter source (the lines between the
// delimiters) with modified set to stamp and every other byte as it was:
// an existing key is replaced where it stands by the one line
// "modified: <stamp>", whatever lines its old value took; a note without
// the key gains the line last. It reports false when it cannot make that
// edit, or when the result does not read back as the same frontmatter with
// that value: the caller then re-emits.
func stampRaw(n *Node, stamp string) (string, bool) {
	fields := n.Frontmatter.Fields
	if len(fields) == 0 || n.Frontmatter.Doc.Content[0].Style&yaml.FlowStyle != 0 {
		return "", false
	}
	indent := fields[0].Key.Column - 1
	lines := strings.SplitAfter(n.Frontmatter.Raw, "\n")
	lines = lines[:len(lines)-1] // every line ends in "\n": drop the empty tail
	line := strings.Repeat(" ", indent) + keyModified + ": " + stamp + n.LineEnding
	field, has := n.Frontmatter.Field(keyModified)
	out := make([]string, 0, len(lines)+1)
	if has {
		at := modifiedLine(lines, indent)
		if at < 0 {
			return "", false
		}
		out = append(out, lines[:at]...)
		out = append(out, line)
		out = append(out, lines[valueEnd(lines, at, indent, deref(field.Value)):]...)
	} else {
		out = append(append(out, lines...), line)
	}
	text := strings.Join(out, "")
	if !stampReadsBack(n, text, stamp, has) {
		return "", false
	}
	return text, true
}

// modifiedLine finds the one frontmatter line that holds the modified key,
// at the mapping's indent; -1 when there is not exactly one.
func modifiedLine(lines []string, indent int) int {
	at := -1
	for i, l := range lines {
		lead, rest := indentOf(l)
		if lead != indent || !isModifiedKey(rest) {
			continue
		}
		if at >= 0 {
			return -1
		}
		at = i
	}
	return at
}

// indentOf splits a source line into its leading spaces, counted, and the
// rest without its line ending.
func indentOf(line string) (int, string) {
	rest := strings.TrimLeft(strings.TrimRight(line, "\r\n"), " ")
	return len(strings.TrimRight(line, "\r\n")) - len(rest), rest
}

// isModifiedKey reports whether a line's text opens with the key modified,
// plain or quoted, and its colon.
func isModifiedKey(text string) bool {
	for _, form := range []string{keyModified, `"` + keyModified + `"`, "'" + keyModified + "'"} {
		after, ok := strings.CutPrefix(text, form)
		if !ok {
			continue
		}
		after = strings.TrimLeft(after, " \t")
		if after == ":" || strings.HasPrefix(after, ": ") || strings.HasPrefix(after, ":\t") {
			return true
		}
	}
	return false
}

// valueEnd is the index of the first line after the value of the key on
// line at: the value takes the lines indented deeper than the key, and,
// when it is a block list, its entries at the key's own indent. Blank
// lines after the value are not part of it.
func valueEnd(lines []string, at, indent int, value *yaml.Node) int {
	list := value.Kind == yaml.SequenceNode && value.Style&yaml.FlowStyle == 0
	end := at + 1
	for end < len(lines) && continuesValue(lines[end], indent, list) {
		end++
	}
	for end > at+1 && blankLine(lines[end-1]) {
		end--
	}
	return end
}

// continuesValue reports whether a line after a key at indent can still be
// part of that key's value: it is blank, or indented deeper than the key,
// or, for a block list, an entry ("- ...") at the key's own indent.
func continuesValue(line string, indent int, list bool) bool {
	lead, rest := indentOf(line)
	if rest == "" || lead > indent {
		return true
	}
	return list && lead == indent && (rest == "-" || strings.HasPrefix(rest, "- ") || strings.HasPrefix(rest, "-\t"))
}

// stampReadsBack reports whether the edited frontmatter text reads back as
// n's frontmatter - the same keys in the same order, with modified last if
// it is new, and every other value as it was - holding modified as stamp.
func stampReadsBack(n *Node, text, stamp string, had bool) bool {
	var got Node
	if err := parseFrontmatter(normalise(text), &got); err != nil {
		return false
	}
	want := make([]string, 0, len(n.Frontmatter.Fields)+1)
	for _, f := range n.Frontmatter.Fields {
		want = append(want, f.Key.Value)
	}
	if !had {
		want = append(want, keyModified)
	}
	keys := make([]string, 0, len(got.Frontmatter.Fields))
	for _, f := range got.Frontmatter.Fields {
		keys = append(keys, f.Key.Value)
	}
	if !sameStrings(keys, want) || got.Description != n.Description || !sameStrings(got.Tags, n.Tags) {
		return false
	}
	if compareProperties(got.Properties, n.Properties) != nil {
		return false
	}
	value, _ := modifiedField(&got)
	return got.dated && value == stamp
}
