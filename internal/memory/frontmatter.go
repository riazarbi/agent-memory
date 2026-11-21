package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	keyDescription = "description"
	keyTags        = "tags"
	tagStr         = "!!str"
	tagNull        = "!!null"
	tagSeq         = "!!seq"
)

// scalarTags are the YAML types that are Obsidian property types: text,
// number, checkbox, date/datetime, and empty. Anything else — !!binary, a
// custom !foo — cannot be displayed.
var scalarTags = map[string]bool{
	tagStr: true, "!!int": true, "!!float": true, "!!bool": true,
	tagNull: true, "!!timestamp": true,
}

// parseNote parses a folder note: frontmatter, then a paragraphs-only body.
// A leading byte-order mark is dropped and CRLF read as LF; the frontmatter's
// source bytes are kept as they were. An error means the note is malformed;
// warnings are reported but the note loads.
func parseNote(data []byte) (*Node, []string, error) {
	src, bom := strings.CutPrefix(string(data), bomText)
	fm, body, closing, err := splitFrontmatter(normalise(src))
	if err != nil {
		return nil, nil, err
	}
	srcLines := strings.SplitAfter(src, "\n")
	n := &Node{
		Frontmatter: Frontmatter{Raw: strings.Join(srcLines[1:closing], "")},
		Source:      src,
		BOM:         bom,
		LineEnding:  lineEnding(srcLines[0]),
	}
	if err := parseFrontmatter(fm, n); err != nil {
		return nil, nil, err
	}
	blocks, warnings, err := parseBody(body, srcLines[closing+1:])
	if err != nil {
		return nil, nil, err
	}
	n.Blocks = blocks
	if _, has := n.Frontmatter.Field(keyModified); has && !n.dated {
		// The frontmatter's one warning, before the body's (Part III.9).
		warnings = append([]string{unreadableModified}, warnings...)
	}
	return n, warnings, nil
}

// bomText is a UTF-8 byte-order mark.
const bomText = "\ufeff"

// lineEnding returns line's ending: CRLF or LF.
func lineEnding(line string) string {
	if strings.HasSuffix(line, "\r\n") {
		return crlf
	}
	return lf
}

const (
	lf   = "\n"
	crlf = "\r\n"
)

// normalise turns CRLF into LF.
func normalise(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// splitFrontmatter splits a note into its frontmatter and its body lines,
// and returns the index of the closing "---" line. normalise only turns CRLF
// into LF, which keeps the line count, so a line index found in the
// normalised text names the same line in the source split after "\n". The frontmatter closes
// at the *first* "---" line after the opening one, so a thematic break
// further down the body never extends it.
func splitFrontmatter(text string) (string, []string, int, error) {
	lines := strings.Split(text, "\n")
	if !isDelimiter(lines[0]) {
		return "", nil, 0, errors.New("missing frontmatter: a folder note must start with a --- line")
	}
	for i := 1; i < len(lines); i++ {
		if isDelimiter(lines[i]) {
			return strings.Join(lines[1:i], "\n") + "\n", lines[i+1:], i, nil
		}
	}
	return "", nil, 0, errors.New("frontmatter is not closed by a --- line")
}

func isDelimiter(line string) bool { return strings.TrimRight(line, " \t") == "---" }

// parseFrontmatter validates the frontmatter mapping and fills n's
// description, tags, date and other properties. A modified value that
// cannot be read is no error: n is then not dated (Part III.9).
func parseFrontmatter(fm string, n *Node) error {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return fmt.Errorf("frontmatter is not valid YAML: %v", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("frontmatter must be a YAML mapping of properties, including description")
	}
	m := doc.Content[0]
	n.Frontmatter.Doc = &doc
	seen := map[string]bool{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, err := propertyKey(m.Content[i])
		if err != nil {
			return err
		}
		if seen[key] {
			return fmt.Errorf("property %q is given twice", key)
		}
		seen[key] = true
		n.Frontmatter.Fields = append(n.Frontmatter.Fields, Field{Key: m.Content[i], Value: m.Content[i+1]})
		if err := setProperty(n, key, deref(m.Content[i+1])); err != nil {
			return err
		}
	}
	if !seen[keyDescription] {
		return errors.New("frontmatter has no description (required: one line of text)")
	}
	return nil
}

func setProperty(n *Node, key string, v *yaml.Node) error {
	var err error
	switch key {
	case keyDescription:
		n.Description, err = description(v)
	case keyTags:
		n.Tags, err = tags(v)
	case keyModified:
		// Typed as any property is (a nested mapping is still an error);
		// its meaning is the node's date, read from the scalar's text.
		if _, err = propertyValue(key, v); err == nil {
			n.Modified, n.dated = readModified(v, time.Local)
		}
	default:
		var val any
		if val, err = propertyValue(key, v); err == nil {
			n.Properties = append(n.Properties, Property{Key: key, Value: val})
		}
	}
	return err
}

// deref follows a YAML alias to the node it names.
func deref(v *yaml.Node) *yaml.Node {
	if v.Kind == yaml.AliasNode && v.Alias != nil {
		return v.Alias
	}
	return v
}

func propertyKey(k *yaml.Node) (string, error) {
	if k.Kind != yaml.ScalarNode || k.Tag != tagStr {
		return "", fmt.Errorf("property key %q is not a string", k.Value)
	}
	return k.Value, nil
}

// description accepts a non-empty YAML string that decodes to one line. A
// plain or folded scalar wrapped across source lines decodes to one line; a
// literal block keeps its line breaks and is refused.
func description(v *yaml.Node) (string, error) {
	const want = "description must be one non-empty line of text"
	if v.Kind != yaml.ScalarNode || v.Tag != tagStr {
		return "", errors.New(want)
	}
	if v.Style&yaml.LiteralStyle != 0 {
		return "", errors.New(want + " (a | block keeps line breaks)")
	}
	s := v.Value
	if v.Style&yaml.FoldedStyle != 0 {
		s = strings.TrimRight(s, "\n")
	}
	if strings.Contains(s, "\n") {
		return "", errors.New(want + " (it contains a line break)")
	}
	if strings.TrimSpace(s) == "" {
		return "", errors.New(want)
	}
	return s, nil
}

// tags accepts a list of tags, a single legacy string, or nothing.
func tags(v *yaml.Node) ([]string, error) {
	switch v.Kind {
	case yaml.ScalarNode:
		if !scalarTags[v.Tag] {
			return nil, fmt.Errorf("tags has an unsupported YAML type %s", v.Tag)
		}
		if v.Tag == tagNull || v.Value == "" {
			return nil, nil
		}
		t, err := tag(v.Value)
		return []string{t}, err
	case yaml.SequenceNode:
		return tagList(v)
	default:
		return nil, errors.New("tags must be a list of tags")
	}
}

// tagList reads a sequence of tags. The list itself may carry only the
// plain sequence tag: !!set, !!str or a custom !foo on it is refused.
func tagList(v *yaml.Node) ([]string, error) {
	if v.Tag != tagSeq {
		return nil, fmt.Errorf("tags has an unsupported YAML type %s (a list may carry only the plain !!seq tag)", v.Tag)
	}
	out := make([]string, 0, len(v.Content))
	for _, item := range v.Content {
		item = deref(item)
		if item.Kind != yaml.ScalarNode || !scalarTags[item.Tag] {
			return nil, errors.New("tags must be a list of tags")
		}
		t, err := tag(item.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// tag validates one tag in Obsidian tag syntax, stripping a leading '#':
// letters, digits, '_', '-' and '/', no spaces, not purely numeric.
func tag(raw string) (string, error) {
	t := strings.TrimPrefix(raw, "#")
	bad := fmt.Errorf("tag %q is not valid Obsidian tag syntax (letters, digits, _, - and /; no spaces; not only digits)", raw)
	if t == "" {
		return "", bad
	}
	numeric := true
	for _, r := range t {
		if !tagRune(r) {
			return "", bad
		}
		if !unicode.IsDigit(r) {
			numeric = false
		}
	}
	if numeric {
		return "", bad
	}
	return t, nil
}

func tagRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' || r == '-' || r == '/'
}

// propertyValue reads any other property: an Obsidian property type or
// empty. Scalars keep their YAML-decoded value; list items are read as
// their text, as Obsidian displays them.
func propertyValue(key string, v *yaml.Node) (any, error) {
	switch v.Kind {
	case yaml.ScalarNode:
		if !scalarTags[v.Tag] {
			return nil, fmt.Errorf("property %q has YAML type %s, which is not an Obsidian property type", key, v.Tag)
		}
		var val any
		if err := v.Decode(&val); err != nil {
			return nil, fmt.Errorf("property %q: %v", key, err)
		}
		return val, nil
	case yaml.SequenceNode:
		if v.Tag != tagSeq {
			return nil, fmt.Errorf("property %q is a list with YAML type %s; a list may carry only the plain !!seq tag", key, v.Tag)
		}
		out := make([]string, 0, len(v.Content))
		for _, item := range v.Content {
			item = deref(item)
			if item.Kind != yaml.ScalarNode || !scalarTags[item.Tag] {
				return nil, fmt.Errorf("property %q: a list may hold only text, numbers, checkboxes and dates, not nested values", key)
			}
			out = append(out, item.Value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("property %q holds a nested mapping, which Obsidian cannot display", key)
	}
}
