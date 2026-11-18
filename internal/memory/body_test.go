package memory

import (
	"strings"
	"testing"
)

// Criterion 5: any block line Obsidian would render as something other
// than a paragraph makes the node malformed.
func TestBody_NonParagraphRejected(t *testing.T) {
	cases := []struct{ name, body string }{
		{"heading", "## Heading"},
		{"heading h1 bare", "#"},
		{"heading h6", "###### six"},
		{"heading with 3-space indent", "   # Heading"},
		{"dash list", "- item"},
		{"star list", "* item"},
		{"plus list", "+ item"},
		{"ordered list", "1. item"},
		{"ordered list paren", "2) item"},
		{"empty list item", "-"},
		{"backtick fence", "```"},
		{"tilde fence", "~~~"},
		{"blockquote", "> quote"},
		{"table row", "| a |"},
		{"thematic break dashes", "---"},
		{"thematic break stars", "***"},
		{"thematic break underscores", "___"},
		{"thematic break spaced", "_ _ _"},
		{"html block", "<div>"},
		{"html closing tag", "</div>"},
		{"html comment", "<!-- hidden -->"},
		{"autolink at line start", "<https://e.com> is a link"},
		{"uppercase html block", "<DIV>"},
		{"indented code", "    code"},
		{"tab-indented code", "\tcode"},
		{"math block", "$$"},
		{"comment block", "%% comment %%"},
		{"footnote definition", "[^1]: note"},
		{"footnote definition without space", "[^note]:text"},
		{"link reference definition", "[x]: https://e.com"},
		{"setext equals", "Title\n==="},
		{"setext dashes", "Title\n--"},
		{"bad line mid-paragraph", "First line\n- item\nthird line"},
		{"bad line in second block", "Fine paragraph.\n\n> quote"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Load(writeTree(t, map[string]string{
				"bad/bad.md": note("d", "Opening paragraph.\n\n"+tc.body+"\n"),
				"ok/ok.md":   note("valid sibling", ""),
			}))
			wantProblems(t, s, pk{SeverityError, "bad"})
			wantStems(t, s.Nodes(), "ok")
		})
	}
}

func TestBody_ParagraphsLoad(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("d", "Line one\nline two\nline three\n\n"+
			"Has **bold**, `code`, an inline #tag, inline $x$ and a mid-line [[b]] here.\n"),
		"b/b.md": note("b", ""),
	}))
	wantProblems(t, s)
	a := mustNode(t, s, "a")
	if len(a.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2: %#v", len(a.Blocks), a.Blocks)
	}
	if a.Blocks[0].Text != "Line one\nline two\nline three" {
		t.Errorf("block 0 = %q", a.Blocks[0].Text)
	}
	if !strings.HasPrefix(a.Blocks[1].Text, "Has **bold**") {
		t.Errorf("block 1 = %q", a.Blocks[1].Text)
	}
}

// Lines that merely look close to a block construct stay paragraph text.
func TestBody_LookalikesAreParagraphs(t *testing.T) {
	lines := []string{
		"#tag at line start",
		"####### seven hashes",
		"-not a list",
		"1.5 is a number",
		"*emphasis* at start",
		"`inline code` at start",
		"a < b and c > d",
		"$x$ inline math",
		"% percent",
		"[[b]] link at start",
		"[text](https://e.com) at start",
		"   three-space indent",
		"x ^ y caret",
		"===",
		"--",
		"<3 a heart",
		"[label] no colon",
	}
	body := strings.Join(lines, "\n\n") + "\n"
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("d", body),
		"b/b.md": note("b", ""),
	}))
	wantProblems(t, s)
	if got := len(mustNode(t, s, "a").Blocks); got != len(lines) {
		t.Errorf("blocks = %d, want %d", got, len(lines))
	}
}

func TestBody_BlankLinesAndNoBlocks(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"empty/empty.md":   note("d", ""),
		"blanks/blanks.md": note("d", "\n\n   \n"),
		"spaced/spaced.md": note("d", "\n\nOne.\n \n\n\nTwo.\n\n"),
	}))
	wantProblems(t, s)
	for stem, want := range map[string]int{"empty": 0, "blanks": 0, "spaced": 2} {
		if got := len(mustNode(t, s, stem).Blocks); got != want {
			t.Errorf("%s: %d blocks, want %d", stem, got, want)
		}
	}
}

// A body "---" after the frontmatter is a thematic break, not a second
// closing delimiter swallowed into the frontmatter.
func TestBody_FirstClosingDelimiterWins(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": "---\ndescription: d\n---\nText.\n\n---\n\nMore.\n",
	}))
	wantProblems(t, s, pk{SeverityError, "a"})
}

// Criterion 6: block IDs.
func TestBody_BlockIDs(t *testing.T) {
	cases := []struct {
		name, body, id, text string
		warn                 bool
	}{
		{"trailing id", "Claim text. ^m-7f3a2k", "m-7f3a2k", "Claim text.", false},
		{"id alone on last line", "Claim text.\n^m-7f3a2k", "m-7f3a2k", "Claim text.", false},
		{"trailing spaces after id", "Claim text. ^m-7f3a2k   ", "m-7f3a2k", "Claim text.", false},
		{"multi-line block id", "Line one\nline two ^m-0d4e9x", "m-0d4e9x", "Line one\nline two", false},
		{"only an id", "^m-7f3a2k", "m-7f3a2k", "", false},
		{"no id", "No id here.", "", "No id here.", false},
		{"obsidian id", "Obsidian made this. ^a1b2c3", "", "Obsidian made this. ^a1b2c3", true},
		{"uppercase m id", "Shouty. ^m-ABCDEF", "", "Shouty. ^m-ABCDEF", true},
		{"seven chars", "Long. ^m-7f3a2kx", "", "Long. ^m-7f3a2kx", true},
		{"obsidian id alone", "Text.\n^a1b2c3", "", "Text.\n^a1b2c3", true},
		{"id not at end is text", "Mentions ^m-7f3a2k mid-line.", "", "Mentions ^m-7f3a2k mid-line.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Load(writeTree(t, map[string]string{"a/a.md": note("d", tc.body+"\n")}))
			if tc.warn {
				wantProblems(t, s, pk{SeverityWarning, "a"})
			} else {
				wantProblems(t, s)
			}
			a := mustNode(t, s, "a")
			if len(a.Blocks) != 1 {
				t.Fatalf("blocks = %#v", a.Blocks)
			}
			if b := a.Blocks[0]; b.ID != tc.id || b.Text != tc.text {
				t.Errorf("block = {%q %q}, want {%q %q}", b.ID, b.Text, tc.id, tc.text)
			}
			if tc.id != "" {
				if b, ok := s.Block("a", tc.id); !ok || b.Text != tc.text {
					t.Errorf("Block(a, %s) = %#v, %v", tc.id, b, ok)
				}
			}
		})
	}
}

func TestBody_DuplicateIDInNode(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("d", "First. ^m-aaaaaa\n\nMiddle.\n\nSecond. ^m-aaaaaa\n"),
	}))
	wantProblems(t, s, pk{SeverityWarning, "a"})
	if b, ok := s.Block("a", "m-aaaaaa"); !ok || b.Text != "First." {
		t.Errorf("Block = %#v, %v; want the first occurrence", b, ok)
	}
	if n := len(mustNode(t, s, "a").Blocks); n != 3 {
		t.Errorf("blocks = %d, want 3 (a duplicate ID never drops a block)", n)
	}
}

func TestBody_DuplicateIDAcrossNodes(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("d", "In a. ^m-aaaaaa\n"),
		"b/b.md": note("d", "In b. ^m-aaaaaa\n"),
	}))
	wantProblems(t, s, pk{SeverityWarning, "b"})
	if r := s.Problems()[0].Reason; !strings.Contains(r, "a") || !strings.Contains(r, "m-aaaaaa") {
		t.Errorf("reason %q should name the earlier node and the ID", r)
	}
	if b, ok := s.Block("a", "m-aaaaaa"); !ok || b.Text != "In a." {
		t.Errorf("Block(a) = %#v, %v", b, ok)
	}
	if b, ok := s.Block("b", "m-aaaaaa"); !ok || b.Text != "In b." {
		t.Errorf("Block(b) = %#v, %v", b, ok)
	}
}

func TestBody_BlockLookupMisses(t *testing.T) {
	s := Load(writeTree(t, map[string]string{"a/a.md": note("d", "Text. ^m-aaaaaa\n\nNo id.\n")}))
	if _, ok := s.Block("a", "m-zzzzzz"); ok {
		t.Error("unknown ID resolved")
	}
	if _, ok := s.Block("nope", "m-aaaaaa"); ok {
		t.Error("unknown stem resolved")
	}
	if _, ok := s.Block("a", ""); ok {
		t.Error("empty ID must not resolve to an unlabelled block")
	}
}
