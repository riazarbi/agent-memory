package memory

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

// withOutputHook installs a corruption of the encoder's assembled bytes for
// the rest of the test.
func withOutputHook(t *testing.T, hook func([]byte) []byte) {
	t.Helper()
	testHookOutput = hook
	t.Cleanup(func() { testHookOutput = nil })
}

func replaceOnce(old, new string) func([]byte) []byte {
	return func(b []byte) []byte { return []byte(strings.Replace(string(b), old, new, 1)) }
}

// U5 criterion 9: the encoder loads its complete output - delimiter lines
// included - through the read model, and returns an error and no bytes
// when it would not load or would not read back as supplied.
func TestGuard_RefusesWhatDoesNotLoad(t *testing.T) {
	newNode := NewNode{Description: "guarded", Properties: map[string]any{"p": "v"}, Blocks: []string{"First claim."}}
	cases := []struct {
		name string
		hook func([]byte) []byte
	}{
		{"corrupt frontmatter YAML", replaceOnce("description: guarded", "description: [guarded")},
		{"broken opening delimiter", replaceOnce("---\n", "--x\n")},
		{"broken closing delimiter", replaceOnce("\n---\n", "\n-- -\n")},
		{"BOM inside the opening line", replaceOnce("---\n", "-\ufeff--\n")},
		{"body no longer a paragraph", replaceOnce("First claim.", "- First claim.")},
		{"value reads back differently", replaceOnce("p: v", "p: w")},
		{"block text reads back differently", replaceOnce("First claim.", "First claim!")},
		{"block ID lost", replaceOnce(" ^m-", " m-")},
		{"supplied content now warns", replaceOnce("First claim.", "First ![[x]] claim.")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withOutputHook(t, tc.hook)
			out, err := Encoder{}.EncodeNew(newNode)
			if err == nil {
				t.Fatalf("encode succeeded with corrupted output %q", out.Bytes)
			}
			var fe *FieldError
			if errors.As(err, &fe) {
				t.Errorf("guard failure reported as a validation error: %v", err)
			}
			if out.Bytes != nil || out.BlockIDs != nil {
				t.Errorf("refused encode returned output %q %q", out.Bytes, out.BlockIDs)
			}
		})
	}
	// Control: the hook runs on every encode, and an untouched output passes.
	called := false
	withOutputHook(t, func(b []byte) []byte { called = true; return b })
	if _, err := (Encoder{}).EncodeNew(newNode); err != nil || !called {
		t.Fatalf("identity hook: err %v, called %v", err, called)
	}
}

// The same check guards a rewrite, including a no-change rewrite.
func TestGuard_RefusesRewriteThatDoesNotLoad(t *testing.T) {
	st, _ := storeOf(t, map[string]string{"n/n.md": bodyFixture})
	for name, c := range map[string]Change{
		"no change":     {},
		"append":        {Blocks: []BlockEdit{{Text: "New."}}},
		"field changed": {Description: strp("changed")},
	} {
		t.Run(name, func(t *testing.T) {
			withOutputHook(t, replaceOnce("last claim.", "last claim.\n\n> quote"))
			out, err := Encoder{Store: st}.EncodeChange("n", c)
			if err == nil || out.Bytes != nil {
				t.Fatalf("rewrite returned %q, %v; want an error and no bytes", out.Bytes, err)
			}
		})
	}
}

// fuzzPieces is the corpus fuzzed strings are drawn from: every Unicode
// line-break character, YAML-special words and indicators, quote and escape
// characters, and ordinary text.
var fuzzPieces = []string{
	"\n", "\r", "\r\n", "\v", "\f", "\u0085", "\u2028", "\u2029",
	"true", "False", "yes", "no", "on", "off", "null", "Null", "~", "<<", "=", "-", "--", "---", "...",
	"?", "? ", ": ", ":", "!", "!!str", "!foo", "&a", "*a", "#", " #", "%", "@", "`", "|", ">", "|-",
	"0x1F", "0o17", "007", "1e3", "1_000", "+1", "-1.5", ".inf", "-.inf", ".nan", "3.0",
	"2026-11-14", "2026-11-14T10:00:00Z", "12:30:00",
	"'", "\"", "''", "\\", "\\n", "\\\"",
	" ", "  ", "\t", "\u00a0", "\ufeff", "\x00", "\x1b",
	"[", "]", "{", "}", ",", "[[x]]", "[[a#^m-abc123]]", "^m-abc123", "^foo", "^",
	"> q", "- i", "# h", "1. n", "|t|", "<b>", "$$", "%%", "[^1]:",
	"a", "word", "e\u0301", "\u65e5\u672c", "#tag", "a/b", "_",
}

// fuzzString draws 1 to 4 corpus pieces.
func fuzzString(r *rand.Rand) string {
	var b strings.Builder
	for range 1 + r.IntN(4) {
		b.WriteString(fuzzPieces[r.IntN(len(fuzzPieces))])
	}
	return b.String()
}

// fuzzNode draws a node's content from the corpus. Most draws fuzz one
// field of an otherwise valid node, so each field's emission is exercised
// rather than drowned by another field's rejection; one in four fuzzes
// every field at once.
func fuzzNode(r *rand.Rand) NewNode {
	n := NewNode{
		Description: "d",
		Tags:        []string{"t"},
		Properties:  map[string]any{"p": "v", "l": []string{"x"}},
		Blocks:      []string{"b"},
	}
	mutators := []func(){
		func() { n.Description = fuzzString(r) },
		func() { n.Tags = append(n.Tags, fuzzString(r)) },
		func() { n.Properties["k"+fuzzString(r)] = "v" },
		func() { n.Properties["p"] = fuzzString(r) },
		func() { n.Properties["l"] = []string{fuzzString(r), fuzzString(r)} },
		func() { n.Blocks = append(n.Blocks, fuzzString(r)) },
	}
	applyMutators(r, mutators)
	return n
}

// applyMutators applies one mutator, or (one time in four) all of them.
func applyMutators(r *rand.Rand, mutators []func()) {
	if r.IntN(4) > 0 {
		mutators[r.IntN(len(mutators))]()
		return
	}
	for _, m := range mutators {
		m()
	}
}

// fuzzChange draws a change to fuzzNodeFixture the same way.
func fuzzChange(r *rand.Rand) Change {
	c := Change{Properties: map[string]any{}}
	mutators := []func(){
		func() { c.Description = strp(fuzzString(r)) },
		func() { c.Tags = tagsp(fuzzString(r)) },
		func() { c.Properties["a"] = fuzzString(r) }, // the anchored field
		func() {
			c.Properties["k"+fuzzString(r)] = fuzzString(r)
			c.Properties["l"] = []string{fuzzString(r)}
		},
		func() { c.Blocks = append(c.Blocks, BlockEdit{ID: "m-f9aaa1", Text: fuzzString(r)}) },
		func() { c.Blocks = append(c.Blocks, BlockEdit{Text: fuzzString(r)}) },
	}
	applyMutators(r, mutators)
	return c
}

// checkFuzzedNew is the fuzz property for a new node: rejected by
// validation, or loads clean and decodes back exactly.
func checkFuzzedNew(t *testing.T, n NewNode) {
	t.Helper()
	out, err := Encoder{}.EncodeNew(n)
	if err != nil {
		var fe *FieldError
		if !errors.As(err, &fe) {
			t.Fatalf("input passed validation but the encoder could not emit it (%v):\n%#v", err, n)
		}
		return
	}
	got := loadsClean(t, "n", out.Bytes)
	wantTags := make([]string, 0, len(n.Tags))
	for _, tg := range n.Tags {
		wantTags = append(wantTags, strings.TrimPrefix(tg, "#"))
	}
	checkReadBack(t, got, out, n.Description, wantTags, n.Properties)
	wantBlocks := make([]Block, 0, len(n.Blocks))
	for i, text := range n.Blocks {
		wantBlocks = append(wantBlocks, Block{ID: out.BlockIDs[i], Text: text})
	}
	checkBlocksReadBack(t, got, wantBlocks)
}

// checkReadBack fails unless got carries the description, tags and
// properties a fuzzed write supplied.
func checkReadBack(t *testing.T, got *Node, out Encoded, desc string, tags []string, props map[string]any) {
	t.Helper()
	if got.Description != desc {
		t.Fatalf("description %q reads back as %q", desc, got.Description)
	}
	if !sameStrings(got.Tags, tags) {
		t.Fatalf("tags %q read back as %q", tags, got.Tags)
	}
	for k, v := range props {
		if g, ok := got.Property(k); !ok || !reflect.DeepEqual(g, v) {
			t.Fatalf("property %q = %#v reads back as %#v (%v)\n%s", k, v, g, ok, out.Bytes)
		}
	}
}

// checkBlocksReadBack fails unless got's blocks are want (an empty want ID
// means any non-empty ID).
func checkBlocksReadBack(t *testing.T, got *Node, want []Block) {
	t.Helper()
	if len(got.Blocks) != len(want) {
		t.Fatalf("%d blocks read back, want %d", len(got.Blocks), len(want))
	}
	for i, b := range got.Blocks {
		w := want[i]
		if b.Text != w.Text || b.ID == "" || (w.ID != "" && b.ID != w.ID) {
			t.Fatalf("block %d reads back as ^%s %q, want ^%s %q", i, b.ID, b.Text, w.ID, w.Text)
		}
	}
}

// U5 criterion 9: over strings drawn from the corpus, every input is either
// rejected by validation or produces a note that loads clean and decodes
// back exactly - never a file the reader rejects. Deterministic, so it
// runs in every go test; FuzzEncodeNew explores further under -fuzz.
func TestGuard_FuzzedValuesRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 27)) //nolint:gosec // a seeded, reproducible fuzz corpus
	accepted := 0
	for range 1500 {
		n := fuzzNode(r)
		checkFuzzedNew(t, n)
		if _, err := (Encoder{}).EncodeNew(n); err == nil {
			accepted++
		}
	}
	// Every piece alone, in every position, too.
	for _, p := range fuzzPieces {
		for _, s := range []string{p, "x" + p, p + "x", "x" + p + "y"} {
			checkFuzzedNew(t, NewNode{Description: s, Tags: []string{s}, Properties: map[string]any{"v": s, "l": []string{s}}, Blocks: []string{"b"}})
			checkFuzzedNew(t, NewNode{Description: "d", Properties: map[string]any{s: "v"}})
			checkFuzzedNew(t, NewNode{Description: "d", Blocks: []string{s}})
		}
	}
	if accepted < 500 {
		t.Errorf("only %d of 1500 fuzzed nodes passed validation; the corpus no longer exercises emission", accepted)
	}
}

// fuzzNodeFixture is the loaded node fuzzed changes rewrite: CRLF, a BOM,
// an anchored field with an alias, and an ID-less operator paragraph.
const fuzzNodeFixture = "\ufeff---\r\ndescription: d\r\na: &v anchored  # c\r\nb: *v\r\nl: [x, y]\r\n---\r\n\r\n" +
	"Old. ^m-f9aaa1\r\n\r\nOperator text.\r\n"

// The same property for a rewrite of a loaded node: every change is
// rejected by validation, or the note loads clean and reads back exactly.
func TestGuard_FuzzedChangesRoundTrip(t *testing.T) {
	st, _ := storeOf(t, map[string]string{"n/n.md": fuzzNodeFixture})
	r := rand.New(rand.NewPCG(9, 27)) //nolint:gosec // a seeded, reproducible fuzz corpus
	accepted := 0
	for range 1000 {
		c := fuzzChange(r)
		out, err := Encoder{Store: st}.EncodeChange("n", c)
		if err != nil {
			var fe *FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("change passed validation but could not be emitted (%v):\n%#v", err, c)
			}
			continue
		}
		accepted++
		checkFuzzedChange(t, c, out)
	}
	if accepted < 300 {
		t.Errorf("only %d of 1000 fuzzed changes passed validation; the corpus no longer exercises emission", accepted)
	}
}

func checkFuzzedChange(t *testing.T, c Change, out Encoded) {
	t.Helper()
	got := loadsClean(t, "n", out.Bytes)
	desc, tags := "d", []string(nil)
	if c.Description != nil {
		desc = *c.Description
	}
	if c.Tags != nil {
		tags = []string{strings.TrimPrefix((*c.Tags)[0], "#")}
	}
	checkReadBack(t, got, out, desc, tags, c.Properties)
	if b, _ := got.Property("b"); b != "anchored" {
		t.Fatalf("aliased b reads back as %#v\n%s", b, out.Bytes)
	}
	want := []Block{{ID: "m-f9aaa1", Text: "Old."}, {Text: "Operator text."}}
	for _, ed := range c.Blocks {
		if ed.ID != "" {
			want[0].Text = ed.Text
		} else {
			want = append(want, Block{Text: ed.Text})
		}
	}
	checkBlocksReadBack(t, got, want)
	s := string(out.Bytes)
	if !strings.HasPrefix(s, bomText) || strings.Count(s, "\n") != strings.Count(s, "\r\n") {
		t.Fatalf("CRLF/BOM convention lost: %q", s)
	}
}

// FuzzEncodeNew runs the round-trip property under go test -fuzz; its seed
// corpus is every piece above, so plain go test runs those too.
func FuzzEncodeNew(f *testing.F) {
	for _, p := range fuzzPieces {
		f.Add(p, p, p, p)
	}
	f.Fuzz(func(t *testing.T, desc, tg, value, block string) {
		checkFuzzedNew(t, NewNode{
			Description: desc,
			Tags:        []string{tg},
			Properties:  map[string]any{"v": value, "l": []string{value, tg}},
			Blocks:      []string{block},
		})
	})
}
