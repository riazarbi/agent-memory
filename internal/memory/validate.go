package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// FieldError is a write rejected before any output: Field names what to
// fix, in the shape a tool argument would ("description", "tags[1]",
// `properties["x"]`, `properties["x"][0]`, "blocks[2]"), and Reason says
// how.
type FieldError struct {
	Field  string
	Reason string
	// Part, for a "blocks[i]" field, is "id" when the block's id is at
	// fault rather than its text, so a caller can name the argument
	// exactly (blocks[i].id vs blocks[i].text); empty otherwise.
	Part string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Reason }

func fieldErr(field, format string, args ...any) *FieldError {
	return &FieldError{Field: field, Reason: fmt.Sprintf(format, args...)}
}

// blockIDErr is a FieldError about the id of the i-th requested block.
func blockIDErr(i int, format string, args ...any) *FieldError {
	fe := fieldErr(fmt.Sprintf("blocks[%d]", i), format, args...)
	fe.Part = "id"
	return fe
}

// nonLFBreaks are the line breaks other than LF that YAML (yaml.v3) or
// CommonMark treat as line breaks: CR, NEL, LINE SEPARATOR and PARAGRAPH
// SEPARATOR. A string carrying one can emit a file the reader rejects, so
// every supplied string refuses them (D27).
var nonLFBreaks = []struct {
	r    rune
	name string
}{
	{'\r', `a carriage return (\r)`},
	{'\u0085', "NEL (U+0085)"},
	{'\u2028', "a line separator (U+2028)"},
	{'\u2029', "a paragraph separator (U+2029)"},
}

// checkText refuses invalid UTF-8 and every line break other than LF.
func checkText(field, s string) error {
	if !utf8.ValidString(s) {
		return fieldErr(field, "is not valid UTF-8 text")
	}
	for _, b := range nonLFBreaks {
		if strings.ContainsRune(s, b.r) {
			return fieldErr(field, "contains %s; the only line break allowed is a plain newline (\\n)", b.name)
		}
	}
	return nil
}

// checkLine is checkText for a value that must also be one line.
func checkLine(field, s string) error {
	if err := checkText(field, s); err != nil {
		return err
	}
	if strings.Contains(s, "\n") {
		return fieldErr(field, "must be a single line (it contains a line break)")
	}
	return nil
}

// validDescription checks a description: one non-empty line.
func validDescription(s string) error {
	const field = keyDescription
	if err := checkLine(field, s); err != nil {
		return err
	}
	if strings.TrimSpace(s) == "" {
		return fieldErr(field, "is required: one line of text saying what the node holds")
	}
	return nil
}

// validTags checks tags and returns them with any leading '#' stripped.
func validTags(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for i, raw := range in {
		field := fmt.Sprintf("tags[%d]", i)
		if err := checkLine(field, raw); err != nil {
			return nil, err
		}
		t, err := tag(raw)
		if err != nil {
			return nil, fieldErr(field, "%v", err)
		}
		out = append(out, t)
	}
	return out, nil
}

// validProperties checks properties and returns them normalised (see
// propertyValueIn), in sorted key order.
func validProperties(in map[string]any) ([]Property, error) {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Property, 0, len(keys))
	for _, k := range keys {
		field := fmt.Sprintf("properties[%q]", k)
		if err := validPropertyKey(field, k); err != nil {
			return nil, err
		}
		v, err := propertyValueIn(field, in[k])
		if err != nil {
			return nil, err
		}
		out = append(out, Property{Key: k, Value: v})
	}
	return out, nil
}

func validPropertyKey(field, k string) error {
	if k == "" {
		return fieldErr(field, "a property key must not be empty")
	}
	if err := checkLine(field, k); err != nil {
		return err
	}
	if k == keyDescription || k == keyTags {
		return fieldErr(field, "%s is not an extra property; give it as the %s field", k, k)
	}
	return nil
}

// propertyValueIn normalises a supplied property value: a string, an int64
// or float64 number, a bool, nil, or a []string. Anything else - a nested
// object, a list holding anything but strings - is refused.
func propertyValueIn(field string, v any) (any, error) {
	switch x := v.(type) {
	case nil, bool:
		return x, nil
	case string:
		return x, checkText(field, x)
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case float64:
		return finite(field, x)
	case json.Number:
		return jsonNumber(field, x)
	case []string:
		return stringList(field, len(x), func(i int) (any, bool) { return x[i], true })
	case []any:
		return stringList(field, len(x), func(i int) (any, bool) { return x[i], true })
	default:
		return nil, fieldErr(field, "has type %T; a property value is text, a number, true/false, null, or a list of text (no nested objects)", v)
	}
}

func finite(field string, f float64) (any, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fieldErr(field, "is not a finite number")
	}
	return f, nil
}

func jsonNumber(field string, n json.Number) (any, error) {
	if i, err := n.Int64(); err == nil {
		return i, nil
	}
	f, err := n.Float64()
	if err != nil {
		return nil, fieldErr(field, "%q is not a number", n.String())
	}
	return finite(field, f)
}

// stringList checks that every item of a list is text.
func stringList(field string, n int, item func(int) (any, bool)) (any, error) {
	out := make([]string, 0, n)
	for i := range n {
		v, _ := item(i)
		s, ok := v.(string)
		itemField := fmt.Sprintf("%s[%d]", field, i)
		if !ok {
			return nil, fieldErr(itemField, "has type %T; a list property holds text only", v)
		}
		if err := checkText(itemField, s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// validBlockText checks block text against D27: one non-empty plain
// paragraph (D25's rules) that the reader will read back exactly once an
// ID is appended to its last line.
func validBlockText(field, text string) error {
	if err := checkText(field, text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fieldErr(field, "block text is empty")
	}
	if strings.HasPrefix(text, "\n") || strings.HasSuffix(text, "\n") {
		return fieldErr(field, "block text must not begin or end with a line break")
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if err := checkBlockLine(field, line, i == 0); err != nil {
			return err
		}
	}
	return checkBlockEnd(field, lines[len(lines)-1], text)
}

func checkBlockLine(field, line string, first bool) error {
	if blankLine(line) {
		return fieldErr(field, "block text holds more than one paragraph (it contains a blank line); split it into one block per paragraph")
	}
	if what := nonParagraph(line, first); what != "" {
		return fieldErr(field, "line %q is %s; a block is one plain paragraph (no headings, lists, quotes, fences, tables or code)", line, what)
	}
	return nil
}

func checkBlockEnd(field, last, text string) error {
	if strings.TrimRight(last, " \t") != last {
		return fieldErr(field, "block text must not end with spaces or tabs")
	}
	if caretTokenRE.MatchString(last) || blockIDRE.MatchString(last) {
		return fieldErr(field, "block text must not end with a ^token; block IDs are assigned by the harness")
	}
	if _, warnings := scanLinks(text); len(warnings) > 0 {
		return fieldErr(field, "%s; links are [[stem]], [[stem|alias]] or [[stem#^m-xxxxxx]], and embeds ![[...]] are not allowed", warnings[0])
	}
	return nil
}
