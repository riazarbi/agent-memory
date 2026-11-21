//go:build unix

package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// CLI16: the stored modified date (docs/spec/agent-memory.md Part III.9).

// The fixed times these tests use: the time of the call, the date already
// stored in a note, and the mtime the fixture files are given. All three
// differ, so a value can only have come from one of them.
var (
	callAt   = time.Date(2026, 11, 21, 14, 3, 7, 0, time.UTC)
	storedAt = "2026-01-15T10:00:00Z"
	fileAt   = time.Date(2020, 1, 1, 0, 0, 0, 500_000_000, time.UTC)
)

const (
	callText = "2026-11-21T14:03:07Z"
	fileText = "2020-01-01T00:00:00Z"
)

// atCallTime makes the clock read callAt plus a fraction of a second, so a
// value that is not cut to the second shows.
func atCallTime(t *testing.T) {
	t.Helper()
	withHook(t, &now, func() time.Time { return callAt.Add(900 * time.Millisecond).In(time.FixedZone("x", 2*3600)) })
}

// datedTree is writeTree with every file's mtime set to fileAt.
func datedTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := writeTree(t, files)
	for rel := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.Chtimes(p, fileAt, fileAt); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readNote(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// wantNote fails unless the note at rel is exactly want.
func wantNote(t *testing.T, root, rel, want string) {
	t.Helper()
	if got := readNote(t, root, rel); got != want {
		t.Errorf("%s\n got %q\nwant %q", rel, got, want)
	}
}

func yamlScalar(text string) *yaml.Node {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("k: "+text+"\n"), &doc); err != nil {
		panic(err)
	}
	return doc.Content[0].Content[1]
}

// What can be read: RFC 3339 with Z or an offset; a date-time without an
// offset, seconds optional, in the local zone; a date, the start of that
// day in the local zone; quoted or not. An offset-less value is never
// taken as UTC, which is how yaml.v3 decodes it.
func TestReadModified(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	in := func(y int, m time.Month, d, h, mi, s int, loc *time.Location) time.Time {
		return time.Date(y, m, d, h, mi, s, 0, loc)
	}
	readable := []struct {
		text string
		want time.Time
	}{
		{"2026-11-21T23:30:00Z", in(2026, 11, 21, 23, 30, 0, time.UTC)},
		{"2026-11-21T23:30:00-05:00", in(2026, 11, 22, 4, 30, 0, time.UTC)},
		{`"2026-11-21T23:30:00+09:00"`, in(2026, 11, 21, 14, 30, 0, time.UTC)},
		{"2026-11-21T14:03:00", in(2026, 11, 21, 14, 3, 0, tokyo)},
		{"2026-11-21T14:03", in(2026, 11, 21, 14, 3, 0, tokyo)},
		{"2026-11-21 14:03:09", in(2026, 11, 21, 14, 3, 9, tokyo)},
		{`"2026-11-21 14:03"`, in(2026, 11, 21, 14, 3, 0, tokyo)},
		{"'2026-11-21 14:03'", in(2026, 11, 21, 14, 3, 0, tokyo)},
		{"2026-11-21", in(2026, 11, 21, 0, 0, 0, tokyo)},
		{`"2026-11-21"`, in(2026, 11, 21, 0, 0, 0, tokyo)},
	}
	for _, tc := range readable {
		got, ok := readModified(yamlScalar(tc.text), tokyo)
		if !ok || !got.Equal(tc.want) {
			t.Errorf("%s: read as %v (ok %v), want %v", tc.text, got, ok, tc.want)
		}
	}
	for _, text := range []string{"banana", "12", "[a, b]", "", "~", "2026-11", "14:03", "2026-13-40", `"2026-11-21 "`, "true", "2026-11-21T14:03:00 banana"} {
		if got, ok := readModified(yamlScalar(text), tokyo); ok {
			t.Errorf("%q: read as %v, want unreadable", text, got)
		}
	}
}

// A node's date is the stored value when it can be read, else the file's
// mtime; an unreadable value is one warning at the node's path, and the
// node loads. modified is not among the node's other properties, and a
// value no property may hold is still an error.
func TestLoad_NodeDate(t *testing.T) {
	root := datedTree(t, map[string]string{
		"stored/stored.md":   fmNote("description: d\nmodified: "+storedAt+"\n", ""),
		"keyless/keyless.md": fmNote("description: d\nowner: x\n", ""),
		"banana/banana.md":   fmNote("description: d\nmodified: banana\n", ""),
		"number/number.md":   fmNote("description: d\nmodified: 12\n", ""),
		"list/list.md":       fmNote("description: d\nmodified: [a, b]\n", ""),
		"empty/empty.md":     fmNote("description: d\nmodified:\n", ""),
		"nested/nested.md":   fmNote("description: d\nmodified: {a: b}\n", ""),
		"upper/upper.md":     fmNote("description: d\nModified: banana\n", ""),
	})
	s := Load(root)
	wantProblems(t, s,
		pk{SeverityWarning, "banana"}, pk{SeverityWarning, "empty"}, pk{SeverityWarning, "list"},
		pk{SeverityError, "nested"}, pk{SeverityWarning, "number"})
	for _, p := range s.Problems() {
		if p.Severity == SeverityWarning && p.Reason != "modified: cannot be read as a date or date-time; the file's time is used" {
			t.Errorf("%s: warning %q", p.Path, p.Reason)
		}
	}
	want := map[string]time.Time{"stored": time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)}
	for _, stem := range []string{"keyless", "banana", "number", "list", "empty", "upper"} {
		want[stem] = fileAt
	}
	for stem, date := range want {
		n := mustNode(t, s, stem)
		if !n.Modified.Equal(date) {
			t.Errorf("%s: date %v, want %v", stem, n.Modified, date)
		}
		// modified is never among the other properties.
		if _, listed := n.Property("modified"); listed {
			t.Errorf("%s: modified is listed among the other properties", stem)
		}
	}
	// Modified, with a capital, is an ordinary property.
	if v, ok := mustNode(t, s, "upper").Property("Modified"); !ok || v != "banana" {
		t.Errorf("upper: Modified = %v, %v; want an ordinary property", v, ok)
	}
}

// A new node's frontmatter is description, tags, the other properties in
// sorted order, then modified last: the time of the call, UTC, cut to the
// second, unquoted, on one line.
func TestEncodeNew_ModifiedLast(t *testing.T) {
	atCallTime(t)
	enc, err := Encoder{}.EncodeNew(NewNode{
		Description: "How to descale",
		Tags:        []string{"home"},
		Properties:  map[string]any{"zeta": "z", "alpha": "a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ndescription: How to descale\ntags: [home]\nalpha: a\nzeta: z\nmodified: " + callText + "\n---\n"
	if string(enc.Bytes) != want {
		t.Errorf("new node\n got %q\nwant %q", enc.Bytes, want)
	}
	enc, err = Encoder{}.EncodeNew(NewNode{Description: "d", Blocks: []string{"Rinse twice."}})
	if err != nil {
		t.Fatal(err)
	}
	want = "---\ndescription: d\nmodified: " + callText + "\n---\n\nRinse twice. ^" + enc.BlockIDs[0] + "\n"
	if string(enc.Bytes) != want {
		t.Errorf("minimal node\n got %q\nwant %q", enc.Bytes, want)
	}
}

// encodeChange loads one note as the node x and encodes c against it.
func encodeChange(t *testing.T, note string, c Change) (string, error) {
	t.Helper()
	st := Load(datedTree(t, map[string]string{"x/x.md": note}))
	mustNode(t, st, "x")
	enc, err := Encoder{Store: st}.EncodeChange("x", c)
	return string(enc.Bytes), err
}

// A content change that touches no other frontmatter field rewrites only
// the modified line: replaced where it stands, whatever lines its old value
// took, or added as the last line. Every other byte of the frontmatter is
// the source's.
func TestEncodeChange_OnlyTheModifiedLine(t *testing.T) {
	atCallTime(t)
	stamp := "modified: " + callText
	cases := map[string]struct{ fm, want string }{
		"quoted value, mid-list, operator layout kept": {
			"description: \"Priya, who leads platform\"   # keep\ntags: [platform,   atlas]\nmodified: \"2026-01-15 10:00\"\nowner: \"007\"\n",
			"description: \"Priya, who leads platform\"   # keep\ntags: [platform,   atlas]\n" + stamp + "\nowner: \"007\"\n",
		},
		"no key: the line goes last": {
			"# top\ndescription:   d\nowner: x   # why\n\n",
			"# top\ndescription:   d\nowner: x   # why\n\n" + stamp + "\n",
		},
		"a comment on the line goes with the old value": {
			"description: d\nmodified: 2026-01-15   # set by a plugin\nowner: x\n",
			"description: d\n" + stamp + "\nowner: x\n",
		},
		"a block list leaves no line behind": {
			"description: d\nmodified:\n  - a\n  - b\nowner: x\n",
			"description: d\n" + stamp + "\nowner: x\n",
		},
		"a block list at the key's own indent": {
			"description: d\nmodified:\n- a\n- b\n# about owner\nowner: x\n",
			"description: d\n" + stamp + "\n# about owner\nowner: x\n",
		},
		"a flow list": {
			"description: d\nmodified: [a, b]\n",
			"description: d\n" + stamp + "\n",
		},
		"an empty value, last key, blank line after": {
			"description: d\nmodified:\n\n",
			"description: d\n" + stamp + "\n\n",
		},
		"a literal block": {
			"description: d\nmodified: |\n  some text\n  # not a comment\nowner: x\n",
			"description: d\n" + stamp + "\nowner: x\n",
		},
		"a quoted key": {
			"description: d\n\"modified\" : banana\nowner: x\n",
			"description: d\n" + stamp + "\nowner: x\n",
		},
		"indented keys keep their indent": {
			"  description: d\n  modified: banana\n  owner: x\n",
			"  description: d\n  " + stamp + "\n  owner: x\n",
		},
		"another key that only looks like it": {
			"description: d\nmodified-by: me\nnote: |\n  modified: no\n",
			"description: d\nmodified-by: me\nnote: |\n  modified: no\n" + stamp + "\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := encodeChange(t, fmNote(tc.fm, "\nA. ^m-aaaaa1\n"), Change{Blocks: []BlockEdit{{ID: "m-aaaaa1", Text: "B."}}})
			if err != nil {
				t.Fatal(err)
			}
			if want := fmNote(tc.want, "\nB. ^m-aaaaa1\n"); got != want {
				t.Errorf("\n got %q\nwant %q", got, want)
			}
		})
	}
}

// The line is written in the note's own line ending, after a byte-order
// mark, and into a frontmatter that closes the file.
func TestEncodeChange_ModifiedLineKeepsFileConventions(t *testing.T) {
	atCallTime(t)
	add := Change{Blocks: []BlockEdit{{Text: "New."}}}
	got, err := encodeChange(t, "\ufeff---\r\ndescription: d\r\nmodified: banana\r\nowner: x\r\n---\r\n\r\nA. ^m-aaaaa1\r\n", add)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "\ufeff---\r\ndescription: d\r\nmodified: "+callText+"\r\nowner: x\r\n---\r\n\r\nA. ^m-aaaaa1\r\n\r\nNew. ^m-") {
		t.Errorf("CRLF with a byte-order mark: %q", got)
	}
	got, err = encodeChange(t, "---\ndescription: d\n---", add)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "---\ndescription: d\nmodified: "+callText+"\n---\n\nNew. ^m-") {
		t.Errorf("frontmatter closing the file: %q", got)
	}
}

// A note whose frontmatter cannot be re-emitted (D27's edit-locked node)
// still takes a body change, and its modified is set by the line edit; a
// frontmatter change is refused as before.
func TestEncodeChange_EditLockedNoteIsStillDated(t *testing.T) {
	atCallTime(t)
	locked := "description: d\nnote: |+\n  \u2028\n"
	for name, tc := range map[string]struct{ fm, want string }{
		"dated":   {locked + "modified: " + storedAt + "\nowner: x\n", locked + "modified: " + callText + "\nowner: x\n"},
		"keyless": {locked + "owner: x\n", locked + "owner: x\nmodified: " + callText + "\n"},
		// Re-emission cannot stand in for the line edit here, so the old
		// value's lines must all go with it.
		"a block list":               {locked + "modified:\n  - a\n  - b\nowner: x\n", locked + "modified: " + callText + "\nowner: x\n"},
		"a list at the key's indent": {locked + "modified:\n- a\n- b\n\n# owner\nowner: x\n", locked + "modified: " + callText + "\n\n# owner\nowner: x\n"},
		"last":                       {"description: d\nowner: x\nnote: |+\n  \u2028\n", "description: d\nowner: x\nnote: |+\n  \u2028\nmodified: " + callText + "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			src := fmNote(tc.fm, "\nA. ^m-aaaaa1\n")
			tags := []string{"x"}
			if _, err := encodeChange(t, src, Change{Tags: &tags}); err == nil {
				t.Fatal("fixture: a frontmatter change to this note is not refused, so it is not edit-locked")
			}
			got, err := encodeChange(t, src, Change{Blocks: []BlockEdit{{ID: "m-aaaaa1", Text: "Go."}}})
			if err != nil {
				t.Fatalf("a body change is refused: %v", err)
			}
			if want := fmNote(tc.want, "\nGo. ^m-aaaaa1\n"); got != want {
				t.Errorf("\n got %q\nwant %q", got, want)
			}
		})
	}
}

// A frontmatter the line edit cannot serve is re-emitted with the key set:
// a flow mapping, and a key written in the explicit "? key" form.
func TestEncodeChange_ModifiedFallsBackToReemission(t *testing.T) {
	atCallTime(t)
	for name, fm := range map[string]string{
		"flow mapping":          "{description: d, modified: banana, owner: x}\n",
		"flow mapping, keyless": "{description: d, owner: x}\n",
		"explicit key":          "description: d\n? modified\n: banana\nowner: x\n",
		"an aliased old value":  "description: d\nmodified: &m banana\nowner: x\nother: *m\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := encodeChange(t, fmNote(fm, "\nA. ^m-aaaaa1\n"), Change{Blocks: []BlockEdit{{Text: "New."}}})
			if err != nil {
				t.Fatal(err)
			}
			n, warnings, err := parseNote([]byte(got))
			if err != nil || len(warnings) != 0 {
				t.Fatalf("output does not load clean: %v %v\n%s", err, warnings, got)
			}
			if text, _ := modifiedField(n); text != callText || !n.dated {
				t.Errorf("modified reads back as %q\n%s", text, got)
			}
			if v, _ := n.Property("owner"); v != "x" || n.Description != "d" || strings.Count(got, "modified") != 1 {
				t.Errorf("another field changed, or modified is there twice:\n%s", got)
			}
		})
	}
}

// When a write changes another frontmatter field the frontmatter is
// re-emitted (D26): modified keeps its place among the keys, or goes last
// if it is new.
func TestEncodeChange_ModifiedOnReemission(t *testing.T) {
	atCallTime(t)
	desc := "new"
	got, err := encodeChange(t, fmNote("description: d\nmodified: \"2026-01-15 10:00\"\nowner: \"007\"\n", "\nA. ^m-aaaaa1\n"), Change{Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	if want := fmNote("description: new\nmodified: "+callText+"\nowner: \"007\"\n", "\nA. ^m-aaaaa1\n"); got != want {
		t.Errorf("key in place\n got %q\nwant %q", got, want)
	}
	tags := []string{"t"}
	got, err = encodeChange(t, fmNote("description: d\nowner: x\n", ""), Change{Tags: &tags, Properties: map[string]any{"zeta": "z"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := fmNote("description: d\ntags: [t]\nowner: x\nzeta: z\nmodified: "+callText+"\n", ""); got != want {
		t.Errorf("new key\n got %q\nwant %q", got, want)
	}
}

// Whether a write changes anything is judged without modified: a request
// that changes nothing returns the file as read, date and all, even when
// the stored value is old, missing or unreadable.
func TestEncodeChange_NoChangeLeavesTheDate(t *testing.T) {
	atCallTime(t)
	desc, tags := "d", []string{"a", "b"}
	same := Change{
		Description: &desc, Tags: &tags, Properties: map[string]any{"owner": "x"},
		Blocks: []BlockEdit{{ID: "m-aaaaa1", Text: "A."}},
	}
	for name, fm := range map[string]string{
		"dated":      "description: d\ntags: [a,   b]\nmodified: " + storedAt + "\nowner: x\n",
		"keyless":    "description: d\ntags: [a,   b]\nowner: x\n",
		"unreadable": "description: d\ntags: [a,   b]\nmodified: [a, b]\nowner: x\n",
	} {
		src := fmNote(fm, "\nA. ^m-aaaaa1\n")
		for what, c := range map[string]Change{"nothing asked": {}, "the same values": same} {
			got, err := encodeChange(t, src, c)
			if err != nil || got != src {
				t.Errorf("%s, %s: err %v\n got %q\nwant %q", name, what, err, got, src)
			}
		}
	}
}

// Only memory sets it: the key modified is refused as a caller's property,
// on a create and on an update, by exact name.
func TestModifiedIsNotACallersProperty(t *testing.T) {
	_, err := Encoder{}.EncodeNew(NewNode{Description: "d", Properties: map[string]any{"modified": "2020-01-01"}})
	wantFieldRefusal(t, err, `properties["modified"]`, `"modified" is set by memory`)

	root := datedTree(t, map[string]string{"x/x.md": fmNote("description: d\nmodified: "+storedAt+"\n", "")})
	before := treeState(t, root)
	w := writerFor(t, root)
	_, err = w.Write(WriteRequest{Stem: "x", Properties: map[string]any{"modified": "2020-01-01"}})
	wantFieldRefusal(t, err, `properties["modified"]`, `"modified" is set by memory`)
	desc := "d"
	_, err = w.Write(WriteRequest{Stem: "y", Parent: "x", Description: &desc, Properties: map[string]any{"modified": "x"}})
	wantFieldRefusal(t, err, `properties["modified"]`, `"modified" is set by memory`)
	wantTree(t, root, before)

	atCallTime(t)
	if _, err := w.Write(WriteRequest{Stem: "x", Properties: map[string]any{"Modified": "x"}}); err != nil {
		t.Fatalf("Modified, an ordinary property, is refused: %v", err)
	}
	wantNote(t, root, "x/x.md", fmNote("description: d\nmodified: "+callText+"\nModified: x\n", ""))
}

// The fixture for the writer tests: priya (two blocks), atlas (links to
// priya and to her second block), kettle (a link to priya's second block)
// and bare (no links), each dated storedAt with an mtime of fileAt; and
// loose, which links to priya and has no modified key.
func dateStore(t *testing.T) string {
	t.Helper()
	dated := "description: d\ntags: [atlas]\nmodified: " + storedAt + "\n"
	return datedTree(t, map[string]string{
		"priya/priya.md":   fmNote(dated, "\nPriya leads platform. ^m-ppppp1\n\nAtlas cutover is 2026-12-01. ^m-ppppp2\n"),
		"atlas/atlas.md":   fmNote(dated, "\nOwner is [[priya]]. ^m-aaaaa1\n\nDate: [[priya#^m-ppppp2]]. ^m-aaaaa2\n"),
		"kettle/kettle.md": fmNote("description: d\nmodified: "+storedAt+"\n", "\nUse citric acid. ^m-kkkkk1\n\nCutover: [[priya#^m-ppppp2]]. ^m-kkkkk2\n"),
		"bare/bare.md":     fmNote("description: d\nmodified: "+storedAt+"\n", ""),
		"loose/loose.md":   fmNote("description: d\n", "\nSee [[priya]] and [[priya#^m-ppppp2]]. ^m-lllll1\n"),
	})
}

// wantDates fails unless each named node's note (by path under notes/)
// holds exactly one modified line with the given value, or, for "", no
// modified key at all.
func wantDates(t *testing.T, root string, want map[string]string) {
	t.Helper()
	for rel, value := range want {
		got := modifiedLines(readNote(t, root, rel+"/"+filepath.Base(rel)+".md"))
		switch {
		case value == "" && len(got) != 0:
			t.Errorf("%s: modified %q, want no key", rel, got)
		case value != "" && (len(got) != 1 || got[0] != value):
			t.Errorf("%s: modified %q, want exactly %q", rel, got, value)
		}
	}
}

// A write that changes content dates the note with the time of the call;
// one that changes nothing leaves bytes, mtime and date alone.
func TestWrite_DatesContentChangesOnly(t *testing.T) {
	root := dateStore(t)
	atCallTime(t)
	w := writerFor(t, root)
	before := treeState(t, root)
	desc, tags := "d", []string{"atlas"}
	for name, req := range map[string]WriteRequest{
		"same description": {Stem: "priya", Description: &desc},
		"same tags":        {Stem: "priya", Tags: &tags},
		"same block text":  {Stem: "priya", Blocks: []BlockEdit{{ID: "m-ppppp1", Text: "Priya leads platform."}}},
	} {
		if _, err := w.Write(req); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	wantTree(t, root, before)
	info, err := os.Stat(filepath.Join(root, "priya", "priya.md"))
	if err != nil || !info.ModTime().Equal(fileAt) {
		t.Errorf("priya.md was rewritten: mtime %v, err %v", info.ModTime(), err)
	}

	res, err := w.Write(appendTo("priya", "Prefers async."))
	if err != nil {
		t.Fatal(err)
	}
	wantDates(t, root, map[string]string{"priya": callText, "atlas": storedAt, "loose": ""})
	if !res.Node.Modified.Equal(callAt) {
		t.Errorf("the returned node's date is %v, want the time of the call", res.Node.Modified)
	}
	res, err = w.Write(WriteRequest{Stem: "fresh", Parent: "bare", Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	wantNote(t, root, "bare/fresh/fresh.md", "---\ndescription: d\nmodified: "+callText+"\n---\n")
	if !res.Node.Modified.Equal(callAt) {
		t.Errorf("the created node's date is %v, want the time of the call", res.Node.Modified)
	}
	wantDates(t, root, map[string]string{"bare": storedAt})
}

// rename and retag are mechanical rewrites: a dated note keeps its value
// byte for byte; a note without one gains the mtime its file had, cut to
// the second, as its last frontmatter line; the renamed node is not dated.
func TestMechanicalRewritesKeepTheNodesDate(t *testing.T) {
	root := dateStore(t)
	atCallTime(t)
	w := writerFor(t, root)
	if _, err := w.Rename("priya", "priya-lead"); err != nil {
		t.Fatal(err)
	}
	wantDates(t, root, map[string]string{"priya-lead": storedAt, "atlas": storedAt, "kettle": storedAt, "bare": storedAt, "loose": fileText})
	wantNote(t, root, "loose/loose.md", fmNote("description: d\nmodified: "+fileText+"\n", "\nSee [[priya-lead]] and [[priya-lead#^m-ppppp2]]. ^m-lllll1\n"))
	s := loadClean(t, root)
	if n := mustNode(t, s, "loose"); !n.Modified.Equal(fileAt.Truncate(time.Second)) {
		t.Errorf("loose's date moved from %v to %v", fileAt, n.Modified)
	}

	if res, err := w.Retag("atlas", "atlas-2026"); err != nil || res.Nodes != 2 {
		t.Fatalf("retag: %+v, %v", res, err)
	}
	wantDates(t, root, map[string]string{"priya-lead": storedAt, "atlas": storedAt})
	wantNote(t, root, "atlas/atlas.md", fmNote("description: d\ntags: [atlas-2026]\nmodified: "+storedAt+"\n",
		"\nOwner is [[priya-lead]]. ^m-aaaaa1\n\nDate: [[priya-lead#^m-ppppp2]]. ^m-aaaaa2\n"))
}

// A retag of a note with no readable date writes the file's time, whether
// only its body changes (the line edit) or its tag list does (re-emitted).
func TestRetag_DatesAnUndatedNoteFromItsFile(t *testing.T) {
	root := datedTree(t, map[string]string{
		"inline/inline.md": fmNote("description: d\nmodified: banana\nowner: x\n", "\nAn #old tag. ^m-aaaaa1\n"),
		"listed/listed.md": fmNote("description: d\ntags: [old]\n", ""),
	})
	atCallTime(t)
	if _, err := writerFor(t, root).Retag("old", "new"); err != nil {
		t.Fatal(err)
	}
	wantNote(t, root, "inline/inline.md", fmNote("description: d\nmodified: "+fileText+"\nowner: x\n", "\nAn #new tag. ^m-aaaaa1\n"))
	wantNote(t, root, "listed/listed.md", fmNote("description: d\ntags: [new]\nmodified: "+fileText+"\n", ""))
	wantProblems(t, Load(root))
}

// Each restructuring verb dates the notes whose content it changes with the
// time of the call, and no others: a note that only has a link retargeted
// keeps its date, or first gains it from its file.
func TestRestructuringDatesContentChanges(t *testing.T) {
	cases := map[string]struct {
		call func(w *Writer) error
		want map[string]string
	}{
		"delete --block": {
			func(w *Writer) error { _, err := w.Delete(DeleteRequest{Stem: "priya", Block: "m-ppppp1"}); return err },
			map[string]string{"priya": callText, "atlas": storedAt, "kettle": storedAt, "bare": storedAt, "loose": ""},
		},
		"move-block": {
			func(w *Writer) error { _, err := w.MoveBlock("m-ppppp2", "kettle"); return err },
			map[string]string{"priya": callText, "kettle": callText, "atlas": storedAt, "bare": storedAt, "loose": fileText},
		},
		"split": {
			func(w *Writer) error {
				_, err := w.Split(SplitRequest{Stem: "priya", New: "priya-atlas", Parent: "priya", Description: "Atlas facts", Blocks: []string{"m-ppppp2"}})
				return err
			},
			map[string]string{"priya": callText, "priya/priya-atlas": callText, "atlas": storedAt, "kettle": storedAt, "bare": storedAt, "loose": fileText},
		},
		"merge that adds blocks": {
			func(w *Writer) error { _, err := w.Merge("kettle", "atlas"); return err },
			map[string]string{"atlas": callText, "priya": storedAt, "bare": storedAt, "loose": ""},
		},
		"merge that adds only a tag": {
			func(w *Writer) error {
				// atlas is left with its tag and no blocks, then folded in.
				for _, id := range []string{"m-aaaaa1", "m-aaaaa2"} {
					if _, err := w.Delete(DeleteRequest{Stem: "atlas", Block: id}); err != nil {
						return err
					}
				}
				_, err := w.Merge("atlas", "bare")
				return err
			},
			map[string]string{"bare": callText, "priya": storedAt, "kettle": storedAt, "loose": ""},
		},
		"merge that adds nothing": {
			func(w *Writer) error { _, err := w.Merge("bare", "atlas"); return err },
			map[string]string{"atlas": storedAt, "priya": storedAt, "kettle": storedAt, "loose": ""},
		},
		"move": {
			func(w *Writer) error { _, err := w.Move("kettle", "priya"); return err },
			map[string]string{"priya": storedAt, "priya/kettle": storedAt, "atlas": storedAt, "bare": storedAt, "loose": ""},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := dateStore(t)
			atCallTime(t)
			if err := tc.call(writerFor(t, root)); err != nil {
				t.Fatal(err)
			}
			wantDates(t, root, tc.want)
			loadClean(t, root)
		})
	}
}

func modifiedLines(note string) []string {
	var got []string
	for _, line := range strings.Split(note, "\n") {
		if strings.HasPrefix(line, "modified:") {
			got = append(got, strings.TrimPrefix(line, "modified: "))
		}
	}
	return got
}

// A call that fails part-way and rolls back leaves every note's bytes,
// modified lines included, as they were.
func TestRollbackRestoresModified(t *testing.T) {
	root := dateStore(t)
	atCallTime(t)
	before := treeState(t, root)
	// The rename replaces atlas, kettle and loose, then fails the reload.
	withHook(t, &testHookReadBack, func(s *Store, _ string) {
		s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "zz", Reason: "injected"})
	})
	if _, err := writerFor(t, root).Rename("priya", "priya-lead"); err == nil {
		t.Fatal("the rename succeeded, want a roll back")
	}
	wantTree(t, root, before)
}
