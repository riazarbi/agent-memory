package memory

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// unchangedCorpus is criterion 4's corpus: valid, fully-IDed folder notes,
// each exercising one source convention an encoder could normalise away.
var unchangedCorpus = map[string]string{
	"lf":         "---\ndescription: plain LF\n---\n\nA claim. ^m-c4aaa1\n",
	"crlf-bom":   "\ufeff---\r\ndescription: CRLF with BOM\r\ntags: [a]\r\n---\r\n\r\nA claim. ^m-c4aaa2\r\n\r\nAnother. ^m-c4aaa3\r\n",
	"comments":   "---\n# head comment\n\ndescription:    spaced   # why\ntags:   [ a,b ]\nx:    1   # one\n\n\ny: \"q\"\n# foot\n---\n\nA claim. ^m-c4aaa4\n",
	"delim-ws":   "---   \ndescription: delimiters with trailing spaces\n---\t \n\nA claim. ^m-c4aaa5\n",
	"alias":      "---\ndescription: alias\na: &v shared\nb: *v\n---\n\nA claim. ^m-c4aaa6\n",
	"tab-id":     "---\ndescription: tab before ID\n---\n\nA claim.\t^m-c4aaa7\n",
	"alone-id":   "---\ndescription: ID alone on last line\n---\n\nA claim\nover two lines\n^m-c4aaa8\n",
	"two-blanks": "---\ndescription: two blank lines\n---\n\nOne. ^m-c4aaa9\n\n\nTwo. ^m-c4aab1\n",
	"trailing":   "---\ndescription: trailing spaces\n---\n\nHard break  \nnext line. ^m-c4aab2   \n",
	"no-final":   "---\ndescription: no final newline\n---\n\nA claim. ^m-c4aab3",
	"no-blank":   "---\ndescription: no blank after delimiter\n---\nA claim. ^m-c4aab4\n",
	"empty-body": "---\ndescription: no blocks\n---",
}

func corpusFiles(corpus map[string]string) map[string]string {
	files := map[string]string{}
	for stem, content := range corpus {
		files[stem+"/"+stem+".md"] = content
	}
	return files
}

// U5 criterion 4: loading a node and encoding it with no change returns
// exactly the file's bytes.
func TestEncodeChange_UnchangedIsByteIdentical(t *testing.T) {
	st, dir := storeOf(t, corpusFiles(unchangedCorpus))
	for stem := range unchangedCorpus {
		t.Run(stem, func(t *testing.T) {
			out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange(stem, Change{})
			if err != nil {
				t.Fatal(err)
			}
			if want := fileBytes(t, dir, stem+"/"+stem+".md"); string(out.Bytes) != string(want) {
				t.Errorf("output differs from the file\n got %q\nwant %q", out.Bytes, want)
			}
			if len(out.BlockIDs) != 0 || len(out.Adopted) != 0 {
				t.Errorf("assigned IDs %q %q on a no-op", out.BlockIDs, out.Adopted)
			}
		})
	}
}

// Requesting every field's current value is no change either (ruling of
// 2026-11-15: equal decoded value is no change).
func TestEncodeChange_EqualValuesAreNoChange(t *testing.T) {
	st, dir := storeOf(t, corpusFiles(unchangedCorpus))
	c := Change{
		Description: strp("spaced"),
		Tags:        tagsp("#a", "b"),
		Properties:  map[string]any{"x": 1.0, "y": "q"},
		Blocks:      []BlockEdit{{ID: "m-c4aaa4", Text: "A claim."}},
	}
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("comments", c)
	if err != nil {
		t.Fatal(err)
	}
	if want := fileBytes(t, dir, "comments/comments.md"); string(out.Bytes) != string(want) {
		t.Errorf("equal values changed the file\n got %q\nwant %q", out.Bytes, want)
	}
}

// bodyFixture is criterion 5's node: a frontmatter comment, a tab-separated
// ID, a standalone-ID line, an ID-less operator paragraph, and irregular
// blank-line runs.
const bodyFixture = "---\n# operator comment\ndescription: body changes   # kept\n---\n\n" +
	"Tabbed claim.\t^m-b5aaa1\n\n\n" +
	"Standalone claim\nover two lines\n^m-b5aaa2\n\n" +
	"Operator paragraph without an ID.\n\n\n\n" +
	"Hard break  \nlast claim. ^m-b5aaa3\n"

// U5 criterion 5: appending keeps the frontmatter bytes, but for the one
// modified line a content change adds last (III.9), and every untouched
// block's source lines, gives the operator paragraph and the new block
// distinct new IDs, and separates blocks by exactly one blank line.
func TestEncodeChange_AppendKeepsUntouchedBlocks(t *testing.T) {
	fixClock(t)
	st, dir := storeOf(t, map[string]string{"n/n.md": bodyFixture})
	src := fileBytes(t, dir, "n/n.md")
	enc := Encoder{Store: st, Rand: fixedRand(t, "b5aaa1", "b5bbb1", "b5bbb2")}
	out, err := enc.EncodeChange("n", Change{Blocks: []BlockEdit{{Text: "Appended claim."}}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frontmatterRegion(t, out.Bytes), stamped(frontmatterRegion(t, src), clockText); got != want {
		t.Errorf("frontmatter bytes changed\n got %q\nwant %q", got, want)
	}
	// The first draw collides with a block already in the node.
	if !reflect.DeepEqual(out.Adopted, []string{"m-b5bbb1"}) || !reflect.DeepEqual(out.BlockIDs, []string{"m-b5bbb2"}) {
		t.Fatalf("Adopted %q BlockIDs %q, want [m-b5bbb1] [m-b5bbb2]", out.Adopted, out.BlockIDs)
	}
	wantBody := "\n" +
		"Tabbed claim.\t^m-b5aaa1\n\n" +
		"Standalone claim\nover two lines\n^m-b5aaa2\n\n" +
		"Operator paragraph without an ID. ^m-b5bbb1\n\n" +
		"Hard break  \nlast claim. ^m-b5aaa3\n\n" +
		"Appended claim. ^m-b5bbb2\n"
	if got := string(out.Bytes[len(frontmatterRegion(t, out.Bytes)):]); got != wantBody {
		t.Errorf("body\n got %q\nwant %q", got, wantBody)
	}
	orig := mustNode(t, st, "n")
	n := loadsClean(t, "n", out.Bytes)
	sameSourceLines(t, n, orig, 0, 1, 3)
	checkBlocksReadBack(t, n, []Block{
		orig.Blocks[0], orig.Blocks[1],
		{ID: "m-b5bbb1", Text: "Operator paragraph without an ID."},
		orig.Blocks[3],
		{ID: "m-b5bbb2", Text: "Appended claim."},
	})
}

// sameSourceLines fails unless the blocks at the given indexes have the
// same exact source lines in got as in orig.
func sameSourceLines(t *testing.T, got, orig *Node, idx ...int) {
	t.Helper()
	for _, i := range idx {
		if !reflect.DeepEqual(got.Blocks[i].Source, orig.Blocks[i].Source) {
			t.Errorf("untouched block %d source %q, want %q", i, got.Blocks[i].Source, orig.Blocks[i].Source)
		}
	}
}

// Criterion 5: replacing a block by ID gives it the new text and keeps its
// ID; every other block keeps its source lines. An ID not in the node is
// an error with no output.
func TestEncodeChange_ReplaceByID(t *testing.T) {
	st, _ := storeOf(t, map[string]string{"n/n.md": bodyFixture})
	enc := Encoder{Store: st, Rand: fixedRand(t, "b5ccc1")}
	out, err := enc.EncodeChange("n", Change{Blocks: []BlockEdit{{ID: "m-b5aaa2", Text: "Replaced\ntext."}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.BlockIDs, []string{"m-b5aaa2"}) {
		t.Errorf("BlockIDs = %q", out.BlockIDs)
	}
	orig := mustNode(t, st, "n")
	n := loadsClean(t, "n", out.Bytes)
	if n.Blocks[1].ID != "m-b5aaa2" || n.Blocks[1].Text != "Replaced\ntext." {
		t.Errorf("replaced block = %q %q", n.Blocks[1].ID, n.Blocks[1].Text)
	}
	sameSourceLines(t, n, orig, 0, 3)
	missing, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Blocks: []BlockEdit{{ID: "m-zzzzzz", Text: "x"}}})
	wantFieldError(t, missing, err, "blocks[0]")
}

// D27: a frontmatter-only change copies the body exactly as read, blank-line
// runs and a missing final newline included.
func TestEncodeChange_FrontmatterOnlyCopiesBody(t *testing.T) {
	body := "\n\nOne. ^m-b6aaa1\n\n\n\nTwo  \nlines. ^m-b6aaa2"
	st, _ := storeOf(t, map[string]string{"n/n.md": fmNote("description: d\n", body)})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Description: strp("changed")})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out.Bytes[len(frontmatterRegion(t, out.Bytes)):]); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
	if n := loadsClean(t, "n", out.Bytes); n.Description != "changed" {
		t.Errorf("description = %q", n.Description)
	}
}

// fieldsFixture is criterion 6's frontmatter.
const fieldsFixture = "---\ndescription: old\ntags: [a, b]\nd: 2026-11-14\nf: 1.50\nq: \"007\"\nl: [a, b]\nc: x  # note\nkeep: 1\n---\n\nA claim. ^m-b6bbb1\n"

// U5 criterion 6: a merged key updates in place, a new key is appended, and
// every untouched property keeps its source form and comment.
func TestEncodeChange_FrontmatterReemittedFaithfully(t *testing.T) {
	st, _ := storeOf(t, map[string]string{"n/n.md": fieldsFixture})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{
		Description: strp("new description"),
		Properties:  map[string]any{"keep": 2, "new": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantLines(t, frontmatterRegion(t, out.Bytes),
		"d: 2026-11-14\n", "f: 1.50\n", "q: \"007\"\n", "l: [a, b]\n", "c: x # note\n", "keep: 2\n", "tags: [a, b]\n")
	n := loadsClean(t, "n", out.Bytes)
	if got, want := frontmatterKeys(n), []string{"description", "tags", "d", "f", "q", "l", "c", "keep", "new", "modified"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %q, want %q", got, want)
	}
	d, _ := n.Frontmatter.Field("d")
	if d.Value.Tag != "!!timestamp" || d.Value.Value != "2026-11-14" {
		t.Errorf("d re-emitted as %s %q, want the date as written", d.Value.Tag, d.Value.Value)
	}
	want := []Property{
		{"d", time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)}, {"f", 1.5}, {"q", "007"},
		{"l", []string{"a", "b"}}, {"c", "x"}, {"keep", 2}, {"new", "v"},
	}
	if !reflect.DeepEqual(n.Properties, want) {
		t.Errorf("properties = %#v\nwant %#v", n.Properties, want)
	}
	if n.Description != "new description" || !reflect.DeepEqual(n.Tags, []string{"a", "b"}) {
		t.Errorf("description %q tags %q", n.Description, n.Tags)
	}
	if c, _ := n.Frontmatter.Field("c"); c.Value.LineComment != "# note" {
		t.Errorf("c comment = %q", c.Value.LineComment)
	}
}

// wantLines fails unless text holds each of lines as a whole line.
func wantLines(t *testing.T, text string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(text, "\n"+line) {
			t.Errorf("lacks line %q:\n%s", line, text)
		}
	}
}

// Criterion 6: tags are replaced as a whole, never merged.
func TestEncodeChange_TagsReplaced(t *testing.T) {
	st, _ := storeOf(t, map[string]string{"n/n.md": fieldsFixture})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Tags: tagsp("x")})
	if err != nil {
		t.Fatal(err)
	}
	n := loadsClean(t, "n", out.Bytes)
	if !reflect.DeepEqual(n.Tags, []string{"x"}) {
		t.Errorf("tags = %q, want [x]", n.Tags)
	}
	if got := frontmatterKeys(n); got[1] != "tags" {
		t.Errorf("tags moved: keys %q", got)
	}
}

// aliasFixture is criterion 7's anchored property and its alias, plus an
// alias inside a list.
const aliasFixture = "---\ndescription: aliases\na: &v old\nb: *v  # via alias\nl: [*v, x]\n---\n\nA claim. ^m-b7aaa1\n"

// U5 criterion 7: changing an anchored field first expands the aliases to
// it, so they keep the old value and the YAML stays valid.
func TestEncodeChange_AliasExpandedBeforeAnchorChanges(t *testing.T) {
	st, _ := storeOf(t, map[string]string{"n/n.md": aliasFixture})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Properties: map[string]any{"a": "new"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out.Bytes), "*v") || strings.Contains(string(out.Bytes), "&v") {
		t.Errorf("anchor or alias left in output:\n%s", out.Bytes)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Trim(frontmatterRegion(t, out.Bytes), "-\n")), &doc); err != nil {
		t.Errorf("frontmatter is not valid YAML: %v", err)
	}
	n := loadsClean(t, "n", out.Bytes)
	for k, want := range map[string]any{"a": "new", "b": "old", "l": []string{"old", "x"}} {
		if got, _ := n.Property(k); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", k, got, want)
		}
	}
	if b, _ := n.Frontmatter.Field("b"); b.Value.LineComment != "# via alias" {
		t.Errorf("b lost its comment: %q", b.Value.LineComment)
	}
}

// Changing only the alias leaves the anchor where it is.
func TestEncodeChange_ChangingTheAliasKeepsTheAnchor(t *testing.T) {
	st, _ := storeOf(t, map[string]string{"n/n.md": aliasFixture})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Properties: map[string]any{"b": "mine"}})
	if err != nil {
		t.Fatal(err)
	}
	n := loadsClean(t, "n", out.Bytes)
	for k, want := range map[string]any{"a": "old", "b": "mine", "l": []string{"old", "x"}} {
		if got, _ := n.Property(k); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", k, got, want)
		}
	}
	if !strings.Contains(string(out.Bytes), "a: &v old\n") {
		t.Errorf("anchor not kept:\n%s", out.Bytes)
	}
}

// crlfFixture is criterion 7's CRLF file with a BOM.
const crlfFixture = "\ufeff---\r\ndescription: crlf\r\nx: 1\r\n---\r\n\r\nOld claim. ^m-b7bbb1\r\n"

// U5 criterion 7: a CRLF file with a BOM, with a block appended and a
// property changed, keeps the BOM and CRLF on every line; an LF file stays
// LF.
func TestEncodeChange_KeepsBOMAndLineEndings(t *testing.T) {
	st, _ := storeOf(t, map[string]string{
		"crlf/crlf.md": crlfFixture,
		"lf/lf.md":     "---\ndescription: lf\nx: 1\n---\n\nOld claim. ^m-b7bbb2\n",
	})
	change := Change{Properties: map[string]any{"x": 2, "y": "a\nb"}, Blocks: []BlockEdit{{Text: "New\nclaim."}}}
	out, err := Encoder{Store: st, Rand: fixedRand(t, "b7ccc1")}.EncodeChange("crlf", change)
	if err != nil {
		t.Fatal(err)
	}
	allCRLF(t, string(out.Bytes))
	n := loadsClean(t, "crlf", out.Bytes)
	if !n.BOM || n.LineEnding != crlf {
		t.Errorf("reloaded BOM %v ending %q", n.BOM, n.LineEnding)
	}
	if got, _ := n.Property("y"); got != "a\nb" {
		t.Errorf("y = %q", got)
	}

	out, err = Encoder{Store: st, Rand: fixedRand(t, "b7ccc2")}.EncodeChange("lf", change)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out.Bytes), "\r") || strings.HasPrefix(string(out.Bytes), bomText) {
		t.Errorf("LF file gained a CR or a BOM: %q", out.Bytes)
	}
	loadsClean(t, "lf", out.Bytes)
}

// allCRLF fails unless s opens with a BOM and every line ends in CRLF.
func allCRLF(t *testing.T, s string) {
	t.Helper()
	if !strings.HasPrefix(s, bomText+"---\r\n") {
		t.Errorf("BOM or CRLF opening lost: %q", s)
	}
	if strings.Count(s, "\n") != strings.Count(s, "\r\n") || !strings.HasSuffix(s, "\r\n") {
		t.Errorf("a line does not end in CRLF: %q", s)
	}
}

// U5 criterion 10(a): updating f: 1.50 with 1.5 changes nothing.
func TestRuling_EqualNumberIsNoChange(t *testing.T) {
	st, dir := storeOf(t, map[string]string{"n/n.md": fieldsFixture})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Properties: map[string]any{"f": 1.5}})
	if err != nil {
		t.Fatal(err)
	}
	if want := fileBytes(t, dir, "n/n.md"); string(out.Bytes) != string(want) {
		t.Errorf("bytes changed:\n got %q\nwant %q", out.Bytes, want)
	}
}

// Criterion 10(b): tags new to a node go directly after description, even
// when description is not the first key; tags: [] emits an empty list.
func TestRuling_TagsPlacementAndEmptyList(t *testing.T) {
	st, _ := storeOf(t, map[string]string{
		"first/first.md":   fmNote("description: d\nz: 1\n", ""),
		"middle/middle.md": fmNote("a: 1\ndescription: d\nz: 2\n", ""),
		"tagged/tagged.md": fmNote("description: d\ntags: [a, b]\nz: 1\n", ""),
	})
	for stem, want := range map[string][]string{
		"first":  {"description", "tags", "z", "modified"},
		"middle": {"a", "description", "tags", "z", "modified"},
	} {
		out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange(stem, Change{Tags: tagsp("x")})
		if err != nil {
			t.Fatal(err)
		}
		n := loadsClean(t, stem, out.Bytes)
		if got := frontmatterKeys(n); !reflect.DeepEqual(got, want) {
			t.Errorf("%s keys = %q, want %q", stem, got, want)
		}
	}
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("tagged", Change{Tags: tagsp()})
	if err != nil {
		t.Fatal(err)
	}
	n := loadsClean(t, "tagged", out.Bytes)
	f, ok := n.Frontmatter.Field("tags")
	if !ok || f.Value.Kind != yaml.SequenceNode || len(f.Value.Content) != 0 || len(n.Tags) != 0 {
		t.Errorf("tags field = %v %+v, want an empty list:\n%s", ok, f.Value, out.Bytes)
	}
}

// Criterion 10(c): with an ID duplicated in the node, a replacement changes
// its first occurrence only; replacing one ID twice in a write is an error.
func TestRuling_DuplicateIDReplacesFirst(t *testing.T) {
	st := Load(writeTree(t, map[string]string{
		"n/n.md": note("d", "\nFirst copy. ^m-aaaaaa\n\nSecond copy. ^m-aaaaaa\n"),
	}))
	wantProblems(t, st, pk{SeverityWarning, "n"})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Blocks: []BlockEdit{{ID: "m-aaaaaa", Text: "Replaced."}}})
	if err != nil {
		t.Fatal(err)
	}
	s := Load(writeTree(t, map[string]string{"n/n.md": string(out.Bytes)}))
	n := mustNode(t, s, "n")
	if n.Blocks[0].Text != "Replaced." || n.Blocks[1].Text != "Second copy." {
		t.Errorf("blocks = %q, %q; want the first replaced", n.Blocks[0].Text, n.Blocks[1].Text)
	}
	// The duplicate's warning is untouched operator content; nothing else.
	wantProblems(t, s, pk{SeverityWarning, "n"})
	twice, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Blocks: []BlockEdit{
		{ID: "m-aaaaaa", Text: "x"}, {ID: "m-aaaaaa", Text: "y"},
	}})
	wantFieldError(t, twice, err, "blocks[1]")
}

// Criterion 10(d): a mixed-ending file (CRLF opening line, LF elsewhere)
// with a block appended keeps the endings of what it copies and writes the
// new block, and the modified line (III.9), with CRLF.
func TestRuling_MixedEndings(t *testing.T) {
	fixClock(t)
	src := "---\r\ndescription: mixed\n---\n\nOld one. ^m-b10aa1\n\nOld two. ^m-b10aa2\n"
	st, _ := storeOf(t, map[string]string{"n/n.md": src})
	out, err := Encoder{Store: st, Rand: fixedRand(t, "b10bb1")}.EncodeChange("n", Change{Blocks: []BlockEdit{{Text: "New\nblock."}}})
	if err != nil {
		t.Fatal(err)
	}
	want := "---\r\ndescription: mixed\n" + // copied frontmatter
		"modified: " + clockText + "\r\n" + // written date
		"---\n" + // copied closing line
		"\r\n" + // written blank line
		"Old one. ^m-b10aa1\n" + "\r\n" + "Old two. ^m-b10aa2\n" + "\r\n" +
		"New\r\nblock. ^m-b10bb1\r\n"
	if string(out.Bytes) != want {
		t.Errorf("output\n got %q\nwant %q", out.Bytes, want)
	}
	loadsClean(t, "n", out.Bytes)
}
