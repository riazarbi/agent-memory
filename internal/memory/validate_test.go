package memory

import (
	"strings"
	"testing"
)

// validNewNode is a node every case below starts from; each case breaks
// exactly one thing, so the rejection has a sole cause.
func validNewNode() NewNode {
	return NewNode{
		Description: "fine",
		Tags:        []string{"ok"},
		Properties:  map[string]any{"p": "fine"},
		Blocks:      []string{"Fine block."},
	}
}

// U5 criterion 3: each invalid input is rejected before any output, with a
// FieldError naming the offending field.
func TestValidate_NewNodeRejections(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*NewNode)
		field string
	}{
		{"empty description", func(n *NewNode) { n.Description = "" }, "description"},
		{"blank description", func(n *NewNode) { n.Description = "   " }, "description"},
		{"multi-line description", func(n *NewNode) { n.Description = "a\nb" }, "description"},
		{"description with CR", func(n *NewNode) { n.Description = "a\rb" }, "description"},
		{"tag with a space", func(n *NewNode) { n.Tags = []string{"ok", "two words"} }, "tags[1]"},
		{"numeric tag", func(n *NewNode) { n.Tags = []string{"123"} }, "tags[0]"},
		{"tag with U+2028", func(n *NewNode) { n.Tags = []string{"a\u2028b"} }, "tags[0]"},
		{"nested object", func(n *NewNode) { n.Properties = map[string]any{"x": map[string]any{"y": 1}} }, `properties["x"]`},
		{"list of numbers", func(n *NewNode) { n.Properties = map[string]any{"x": []any{1, 2}} }, `properties["x"][0]`},
		{"list of ints", func(n *NewNode) { n.Properties = map[string]any{"x": []int{1, 2}} }, `properties["x"]`},
		{"property string with LF then U+2028", func(n *NewNode) { n.Properties = map[string]any{"x": "a\n\u2028"} }, `properties["x"]`},
		{"property string with U+2029", func(n *NewNode) { n.Properties = map[string]any{"x": "a\u2029b"} }, `properties["x"]`},
		{"property string with CR", func(n *NewNode) { n.Properties = map[string]any{"x": "a\r\nb"} }, `properties["x"]`},
		{"list item with NEL", func(n *NewNode) { n.Properties = map[string]any{"x": []string{"ok", "a\u0085b"}} }, `properties["x"][1]`},
		{"empty property key", func(n *NewNode) { n.Properties = map[string]any{"": "v"} }, `properties[""]`},
		{"multi-line property key", func(n *NewNode) { n.Properties = map[string]any{"a\nb": "v"} }, "properties[\"a\\nb\"]"},
		{"description as a property", func(n *NewNode) { n.Properties = map[string]any{"description": "v"} }, `properties["description"]`},
		{"tags as a property", func(n *NewNode) { n.Properties = map[string]any{"tags": []string{"a"}} }, `properties["tags"]`},
		{"invalid UTF-8 value", func(n *NewNode) { n.Properties = map[string]any{"x": "a\xffb"} }, `properties["x"]`},
		{"empty block", func(n *NewNode) { n.Blocks = []string{"Fine.", ""} }, "blocks[1]"},
		{"two paragraphs", func(n *NewNode) { n.Blocks = []string{"one\n\ntwo"} }, "blocks[0]"},
		{"whitespace-only line", func(n *NewNode) { n.Blocks = []string{"one\n \t\ntwo"} }, "blocks[0]"},
		{"list item", func(n *NewNode) { n.Blocks = []string{"- item"} }, "blocks[0]"},
		{"heading", func(n *NewNode) { n.Blocks = []string{"## heading"} }, "blocks[0]"},
		{"list on a later line", func(n *NewNode) { n.Blocks = []string{"intro\n- item"} }, "blocks[0]"},
		{"trailing harness ID", func(n *NewNode) { n.Blocks = []string{"text ^m-abc123"} }, "blocks[0]"},
		{"trailing other token", func(n *NewNode) { n.Blocks = []string{"text ^foo"} }, "blocks[0]"},
		{"heading link", func(n *NewNode) { n.Blocks = []string{"see [[c#Heading]]"} }, "blocks[0]"},
		{"embed", func(n *NewNode) { n.Blocks = []string{"see ![[c]]"} }, "blocks[0]"},
		{"path link", func(n *NewNode) { n.Blocks = []string{"see [[path/c]]"} }, "blocks[0]"},
		{"leading line break", func(n *NewNode) { n.Blocks = []string{"\ntext"} }, "blocks[0]"},
		{"trailing line break", func(n *NewNode) { n.Blocks = []string{"text\n"} }, "blocks[0]"},
		{"trailing spaces", func(n *NewNode) { n.Blocks = []string{"text  "} }, "blocks[0]"},
		{"trailing tab", func(n *NewNode) { n.Blocks = []string{"text\t"} }, "blocks[0]"},
		{"block with CR", func(n *NewNode) { n.Blocks = []string{"one\rtwo"} }, "blocks[0]"},
		{"indented code", func(n *NewNode) { n.Blocks = []string{"    code"} }, "blocks[0]"},
		{"setext underline", func(n *NewNode) { n.Blocks = []string{"Title\n==="} }, "blocks[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := validNewNode()
			tc.edit(&n)
			out, err := Encoder{}.EncodeNew(n)
			wantFieldError(t, out, err, tc.field)
		})
	}
	// The baseline itself is valid, so each rejection above has its edit
	// as its sole cause.
	if _, err := (Encoder{}).EncodeNew(validNewNode()); err != nil {
		t.Fatalf("baseline node rejected: %v", err)
	}
}

// Criterion 3, accepted forms: a dangling link, a link inside inline code,
// and a paragraph over two lines.
func TestValidate_AcceptedBlocks(t *testing.T) {
	cases := []struct{ name, text string }{
		{"dangling link", "see [[missing]]"},
		{"link in inline code", "code `[[x]]` here"},
		{"embed in inline code", "code `![[x]]` here"},
		{"one paragraph over two lines", "two\nlines"},
		{"block link and alias", "see [[missing#^m-abc123|there]]"},
		{"caret not ending the text", "a ^m-abc123 mid-text"},
		{"caret glued to a word", "text^foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Encoder{}.EncodeNew(NewNode{Description: "d", Blocks: []string{tc.text}})
			if err != nil {
				t.Fatalf("rejected: %v", err)
			}
			n := loadsClean(t, "n", out.Bytes)
			if len(n.Blocks) != 1 || n.Blocks[0].Text != tc.text || n.Blocks[0].ID != out.BlockIDs[0] {
				t.Errorf("blocks = %#v, want one block %q ^%s", n.Blocks, tc.text, out.BlockIDs[0])
			}
		})
	}
}

// The same validation guards a rewrite: description, tags, properties and
// block edits are each rejected naming the field, with no output.
func TestValidate_ChangeRejections(t *testing.T) {
	st, _ := storeOf(t, map[string]string{
		"n/n.md": fmNote("description: d\nx: 1\n", "\nA. ^m-aaaaaa\n"),
	})
	cases := []struct {
		name  string
		c     Change
		field string
	}{
		{"empty description", Change{Description: strp("")}, "description"},
		{"description with LS", Change{Description: strp("a\u2028")}, "description"},
		{"bad tag", Change{Tags: tagsp("ok", "a b")}, "tags[1]"},
		{"nested property", Change{Properties: map[string]any{"x": map[string]any{}}}, `properties["x"]`},
		{"description as property", Change{Properties: map[string]any{"description": "v"}}, `properties["description"]`},
		{"bad appended block", Change{Blocks: []BlockEdit{{Text: "ok"}, {Text: "> quote"}}}, "blocks[1]"},
		{"bad replacement text", Change{Blocks: []BlockEdit{{ID: "m-aaaaaa", Text: "one\n\ntwo"}}}, "blocks[0]"},
		{"replace an ID not in the node", Change{Blocks: []BlockEdit{{ID: "m-zzzzzz", Text: "new"}}}, "blocks[0]"},
		{"replace the same ID twice", Change{Blocks: []BlockEdit{{ID: "m-aaaaaa", Text: "x"}, {ID: "m-aaaaaa", Text: "y"}}}, "blocks[1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Encoder{Store: st}.EncodeChange("n", tc.c)
			wantFieldError(t, out, err, tc.field)
		})
	}
}

// The error is corrective: it says what is wrong in words a model can act
// on, not only which field.
func TestValidate_ReasonsAreCorrective(t *testing.T) {
	cases := []struct {
		n    NewNode
		want string
	}{
		{NewNode{Description: "d", Blocks: []string{"one\n\ntwo"}}, "split"},
		{NewNode{Description: "d", Blocks: []string{"text ^m-abc123"}}, "assigned by the harness"},
		{NewNode{Description: "d", Properties: map[string]any{"x": "a\u2029"}}, "U+2029"},
		{NewNode{Description: "d", Blocks: []string{"see ![[c]]"}}, "embed"},
	}
	for _, tc := range cases {
		_, err := Encoder{}.EncodeNew(tc.n)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("error %v does not mention %q", err, tc.want)
		}
	}
}
