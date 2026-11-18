package memory

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// U4 criterion 1: the raw frontmatter is the source bytes between the
// opening and closing --- lines — comments, spacing, quoting and line
// endings untouched, never a re-encoded or normalised copy.
func TestRoundTrip_RawFrontmatterIsSourceBytes(t *testing.T) {
	fm := "# a comment line\n" +
		"description:    How and when to cc people\n" +
		"tags: [escalation,   outbox]\n" +
		"quoted: \"a: quoted value\"\n" +
		"flow: [ a,b ]\n" +
		"block:\n" +
		"    - one\n" +
		"    -   two\n"
	crlf := bytes.ReplaceAll([]byte(fm), []byte("\n"), []byte("\r\n"))
	cases := []struct {
		name, content, want string
	}{
		{"LF", "---\n" + fm + "---\nBody. ^m-aaaaaa\n", fm},
		{"BOM and CRLF", "\ufeff---\r\n" + string(crlf) + "---\r\nBody. ^m-aaaaaa\r\n", string(crlf)},
		{"delimiters with trailing spaces, no final newline", "--- \n" + fm + "---\t", fm},
		{"BOM inside a value", "---\ndescription: d\nodd: \"x\ufeffy\"\n---\n", "description: d\nodd: \"x\ufeffy\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := loadOne(t, tc.content)
			if n.Frontmatter.Raw != tc.want {
				t.Errorf("Raw = %q\nwant  %q", n.Frontmatter.Raw, tc.want)
			}
		})
	}
	// The decoded values are what U3 returned, however the source is spaced.
	n := loadOne(t, "---\n"+fm+"---\n")
	if n.Description != "How and when to cc people" {
		t.Errorf("Description = %q", n.Description)
	}
	if !reflect.DeepEqual(n.Tags, []string{"escalation", "outbox"}) {
		t.Errorf("Tags = %q", n.Tags)
	}
	for key, want := range map[string]any{
		"quoted": "a: quoted value",
		"flow":   []string{"a", "b"},
		"block":  []string{"one", "two"},
	} {
		if got, _ := n.Property(key); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

// encodeField encodes one property's retained key and value nodes with
// yaml.v3, as a one-entry mapping.
func encodeField(t *testing.T, f Field) string {
	t.Helper()
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{f.Key, f.Value}}); err != nil {
		t.Fatalf("encode %q: %v", f.Key.Value, err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// U4 criterion 2: each property — description and tags included — keeps
// its YAML key and value nodes, so encoding them reproduces the source's
// scalar text, tag, style and comment, while the decoded values stay what
// U3 returned.
func TestRoundTrip_PropertiesKeepSourceForm(t *testing.T) {
	n := loadOne(t, fmNote(`description: 'Single-quoted: description'  # why
tags: [a, b]  # tag note
d: 2026-11-14
t: 2026-11-14T00:00:00Z
f: 1.50
q: "007"
l: [a, b]
m:
  - a
  - b
c: x  # note
`, ""))
	checkFieldForms(t, n, []fieldForm{
		{"description", "description: 'Single-quoted: description' # why\n", tagStr, yaml.SingleQuotedStyle},
		{"tags", "tags: [a, b] # tag note\n", tagSeq, yaml.FlowStyle},
		{"d", "d: 2026-11-14\n", "!!timestamp", 0},
		{"t", "t: 2026-11-14T00:00:00Z\n", "!!timestamp", 0},
		{"f", "f: 1.50\n", "!!float", 0},
		{"q", "q: \"007\"\n", tagStr, yaml.DoubleQuotedStyle},
		{"l", "l: [a, b]\n", tagSeq, yaml.FlowStyle},
		{"m", "m:\n  - a\n  - b\n", tagSeq, 0},
		{"c", "c: x # note\n", tagStr, 0},
	})
	fields := n.Frontmatter.Fields
	if fields[2].Value.Value != "2026-11-14" || fields[4].Value.Value != "1.50" {
		t.Errorf("source scalar text lost: d=%q f=%q", fields[2].Value.Value, fields[4].Value.Value)
	}
	if c := fields[8].Value.LineComment; c != "# note" {
		t.Errorf("c line comment = %q, want %q", c, "# note")
	}
	checkFieldsAreDocNodes(t, n)

	// Decoded values are exactly what U3 returned.
	if n.Description != "Single-quoted: description" {
		t.Errorf("Description = %q", n.Description)
	}
	if !reflect.DeepEqual(n.Tags, []string{"a", "b"}) {
		t.Errorf("Tags = %q", n.Tags)
	}
	midnight := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	decoded := []struct {
		key  string
		want any
	}{
		{"d", midnight},
		{"t", midnight},
		{"f", 1.5},
		{"q", "007"},
		{"l", []string{"a", "b"}},
		{"m", []string{"a", "b"}},
		{"c", "x"},
	}
	for _, w := range decoded {
		if got, _ := n.Property(w.key); !sameValue(got, w.want) {
			t.Errorf("%s decodes to %#v (%T), want %#v (%T)", w.key, got, got, w.want, w.want)
		}
	}
}

// fieldForm is what one retained property must encode as, and the tag and
// style of its value node.
type fieldForm struct {
	key, encoded, tag string
	style             yaml.Style
}

// checkFieldForms fails unless n's fields are exactly want, in file order,
// each encoding as the source wrote it.
func checkFieldForms(t *testing.T, n *Node, want []fieldForm) {
	t.Helper()
	fields := n.Frontmatter.Fields
	if len(fields) != len(want) {
		t.Fatalf("got %d fields, want %d (every property, description and tags included)", len(fields), len(want))
	}
	for i, w := range want {
		f := fields[i]
		if f.Key.Value != w.key {
			t.Errorf("field %d key = %q, want %q (file order)", i, f.Key.Value, w.key)
			continue
		}
		if got, ok := n.Frontmatter.Field(w.key); !ok || got != f {
			t.Errorf("Field(%q) = %v, %v; want the field at position %d", w.key, got, ok, i)
		}
		if got := encodeField(t, f); got != w.encoded {
			t.Errorf("%s encodes as %q, want %q", w.key, got, w.encoded)
		}
		if f.Value.Tag != w.tag || f.Value.Style != w.style {
			t.Errorf("%s value tag/style = %s/%v, want %s/%v", w.key, f.Value.Tag, f.Value.Style, w.tag, w.style)
		}
	}
}

// checkFieldsAreDocNodes fails unless every field is the parsed document's
// own key and value node, in place.
func checkFieldsAreDocNodes(t *testing.T, n *Node) {
	t.Helper()
	doc := n.Frontmatter.Doc
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		t.Fatalf("Doc = %#v, want the parsed YAML document", doc)
	}
	m := doc.Content[0]
	if len(m.Content) != 2*len(n.Frontmatter.Fields) {
		t.Fatalf("document has %d mapping nodes for %d fields", len(m.Content), len(n.Frontmatter.Fields))
	}
	for i, f := range n.Frontmatter.Fields {
		if m.Content[2*i] != f.Key || m.Content[2*i+1] != f.Value {
			t.Errorf("field %q is not the document's own node", f.Key.Value)
		}
	}
}

// Tags and description keep their source form in their other shapes too: a
// block list of tags with a comment on the key, and a plain description.
func TestRoundTrip_TagsBlockListAndPlainDescription(t *testing.T) {
	n := loadOne(t, fmNote("tags:  # key note\n  - \"#x\"\n  - y\ndescription: plain one\n", ""))
	tags, ok := n.Frontmatter.Field("tags")
	if !ok {
		t.Fatal("no tags field")
	}
	if got, want := encodeField(t, tags), "tags: # key note\n  - \"#x\"\n  - y\n"; got != want {
		t.Errorf("tags encodes as %q, want %q", got, want)
	}
	desc, ok := n.Frontmatter.Field("description")
	if !ok {
		t.Fatal("no description field")
	}
	if got, want := encodeField(t, desc), "description: plain one\n"; got != want {
		t.Errorf("description encodes as %q, want %q", got, want)
	}
	if _, ok := n.Frontmatter.Field("absent"); ok {
		t.Error("Field(absent) found a field")
	}
	if !reflect.DeepEqual(n.Tags, []string{"x", "y"}) || n.Description != "plain one" {
		t.Errorf("decoded tags %q description %q", n.Tags, n.Description)
	}
}

// A property given through a YAML alias keeps the alias node, so the
// anchor's single home is what a re-emitted tree still refers to; the
// decoded value follows the alias as U3 did.
func TestRoundTrip_AliasKeptAsWritten(t *testing.T) {
	n := loadOne(t, fmNote("description: d\nfirst: &v hello\nsecond: *v\n", ""))
	f, ok := n.Frontmatter.Field("second")
	if !ok {
		t.Fatal("no second field")
	}
	if f.Value.Kind != yaml.AliasNode || f.Value.Value != "v" {
		t.Errorf("second value node = kind %v value %q, want the alias *v", f.Value.Kind, f.Value.Value)
	}
	if got, _ := n.Property("second"); got != "hello" {
		t.Errorf("second decodes to %#v, want hello", got)
	}
}
