package memory

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

// U6 criterion 8 carries three test-strength findings from U5's
// verification: a fuzzer that can rediscover the U+2028 class, a
// frontmatter fixture whose updated key sits mid-list, and a test that
// isolates the final check's warnings comparison.

// breakTails are the characters a line-break-terminated string ends in:
// every line break YAML or CommonMark knows, plus the whitespace and
// control characters that sit beside them.
var breakTails = []string{
	"\u0085", "\u2028", "\u2029", "\r", "\r\n", "\n", "\v", "\f", " ", "\t", "\u00a0", "",
}

// breakHeads are the text a break-terminated string starts with.
var breakHeads = []string{"x", "a b", "word", "007", "true", "- i", "#t", "<<", ":", "'", "\""}

// breakString draws one line-break-terminated shape: text, a line feed,
// and a trailing break (or near-break), in one of several arrangements.
func breakString(r *rand.Rand) string {
	head := breakHeads[r.IntN(len(breakHeads))]
	tail := breakTails[r.IntN(len(breakTails))]
	tail2 := breakTails[r.IntN(len(breakTails))]
	shapes := []string{
		head + "\n" + tail,               // LF then a trailing NEL / LS / PS
		head + tail,                      // a bare trailing break
		head + "\n" + tail + tail2,       // two trailing breaks
		head + "\n" + tail + "\n",        // break, then LF last
		head + "\n" + head + tail,        // a second line ending in a break
		tail + head,                      // a leading break
		head + tail + "\n" + head + tail, // breaks on both lines
	}
	return shapes[r.IntN(len(shapes))]
}

// breakNode puts one break-terminated string into one field of an
// otherwise valid node. Property values and list items carry most draws:
// they are the only fields that may hold a line feed, so they are where
// an encoder can emit a multi-line scalar the reader then rejects.
func breakNode(r *rand.Rand) NewNode {
	n := NewNode{
		Description: "d",
		Tags:        []string{"t"},
		Properties:  map[string]any{"p": "v", "l": []string{"x"}},
		Blocks:      []string{"b"},
	}
	s := breakString(r)
	switch r.IntN(8) {
	case 0, 1, 2:
		n.Properties["p"] = s
	case 3, 4:
		n.Properties["l"] = []string{s, "x"}
	case 5:
		n.Description = s
	case 6:
		n.Blocks = append(n.Blocks, s)
	default:
		n.Properties["k"+s] = "v"
	}
	return n
}

// Criterion 8(a): over line-break-terminated strings, every input is
// rejected by validation or produces a note that loads clean and decodes
// back exactly. Removing both the final check and the non-LF line-break
// validation makes this fail (recorded in CHANGELOG.md): the encoder then
// emits a property ending in a line break that the reader rejects. The
// acceptance floor keeps the fuzzer honest: a corpus that validation
// rejects wholesale never reaches emission.
func TestGuard_FuzzedLineBreakTails(t *testing.T) {
	r := rand.New(rand.NewPCG(28, 2028)) //nolint:gosec // a seeded, reproducible fuzz corpus
	const draws = 3000
	accepted := 0
	for range draws {
		n := breakNode(r)
		checkFuzzedNew(t, n)
		if _, err := (Encoder{}).EncodeNew(n); err == nil {
			accepted++
		}
	}
	// Every head and tail pair in every shape, in a property value and a
	// list item, too.
	for _, head := range breakHeads {
		for _, tail := range breakTails {
			for _, s := range []string{head + "\n" + tail, head + tail, head + "\n" + tail + "\n", tail + head} {
				checkFuzzedNew(t, NewNode{Description: "d", Properties: map[string]any{"p": s, "l": []string{s}}})
			}
		}
	}
	if min := draws / 4; accepted < min {
		t.Errorf("only %d of %d break-terminated nodes passed validation (want at least %d); the corpus no longer reaches emission", accepted, draws, min)
	}
	t.Logf("%d of %d break-terminated nodes accepted", accepted, draws)
}

// midListFixture has the key a change updates in the middle of the
// frontmatter, with properties on both sides of it.
const midListFixture = "---\ndescription: mid-list\nfirst: 1\nmid: old\nlast: 2\n---\n\nA claim. ^m-b8aaa1\n"

// Criterion 8(b): an updated key keeps its position. The key sits mid-list,
// so an encoder that appended updated keys at the end would move it after
// "last" and fail here. The change dates the note: modified, new to it, goes
// last (III.9).
func TestEncodeChange_UpdatedKeyMidListKeepsPosition(t *testing.T) {
	fixClock(t)
	st, _ := storeOf(t, map[string]string{"n/n.md": midListFixture})
	out, err := Encoder{Store: st, Rand: fixedRand(t)}.EncodeChange("n", Change{Properties: map[string]any{"mid": "new"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frontmatterRegion(t, out.Bytes), "---\ndescription: mid-list\nfirst: 1\nmid: new\nlast: 2\nmodified: "+clockText+"\n---\n"; got != want {
		t.Errorf("frontmatter\n got %q\nwant %q", got, want)
	}
	n := loadsClean(t, "n", out.Bytes)
	if got, want := frontmatterKeys(n), []string{"description", "first", "mid", "last", "modified"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %q, want %q", got, want)
	}
}

// Criterion 8(c): the final check's warnings comparison, isolated. The
// note below reads back with exactly the values expected - description,
// no tags, no properties, the block's ID and text - so only the warning
// its block text raises tells it apart from a clean note. Removing the
// warnings comparison from verifyOutput makes this fail.
func TestGuard_WarningsComparisonIsolated(t *testing.T) {
	cases := []struct {
		name, text, warning string
	}{
		{"embed", "See ![[x]] here.", "embed ![[x]] is not supported; kept as plain text"},
		{"unsupported link form", "See [[x#heading]] here.", "link [[x#heading]] is not a supported form ([[stem]], [[stem|alias]], [[stem#^m-xxxxxx]]); kept as plain text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := []byte("---\ndescription: d\n---\n\n" + tc.text + " ^m-b8ccc1\n")
			want := expectation{description: "d", blocks: []Block{{ID: "m-b8ccc1", Text: tc.text}}}
			// Every value matches: the comparison of values alone passes.
			got, warnings, err := parseNote(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := compareNode(got, want); err != nil {
				t.Fatalf("fixture does not isolate the warnings check; values differ: %v", err)
			}
			if !reflect.DeepEqual(warnings, []string{tc.warning}) {
				t.Fatalf("fixture warnings = %q, want exactly %q", warnings, tc.warning)
			}
			err = verifyOutput(out, want)
			if err == nil || !strings.Contains(err.Error(), tc.warning) {
				t.Fatalf("verifyOutput = %v; want a refusal naming the added warning", err)
			}
			var fe *FieldError
			if errors.As(err, &fe) {
				t.Errorf("refusal reported as a validation error: %v", err)
			}
			// Control: the same note passes when that warning was already
			// the source's (untouched operator content).
			want.warnings = []string{tc.warning}
			if err := verifyOutput(out, want); err != nil {
				t.Errorf("an already-present warning was refused: %v", err)
			}
		})
	}
}
