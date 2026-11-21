package memory

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// U5 criterion 2: a new node is laid out as D27 fixes, keeps every value's
// JSON type, and loads clean.
func TestEncodeNew_Layout(t *testing.T) {
	enc := Encoder{Rand: fixedRand(t, "aaaaa1", "bbbbb2")}
	out, err := enc.EncodeNew(NewNode{
		Description: "How to cc",
		Tags:        []string{"#escalation", "outbox"},
		Properties: map[string]any{
			"zeta": "2026-11-14", "alpha": "007", "flag": "true", "n": 3, "b": false,
			"list": []string{"a", "b"}, "none": nil,
		},
		Blocks: []string{"First claim.", "Second claim, see [[people]]."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"m-aaaaa1", "m-bbbbb2"}; !reflect.DeepEqual(out.BlockIDs, want) {
		t.Fatalf("BlockIDs = %q, want %q", out.BlockIDs, want)
	}
	newNodeOutput(t, out.Bytes)

	n := loadsClean(t, "n", out.Bytes)
	// A new node is dated: modified is its last key (III.9).
	wantKeys := []string{"description", "tags", "alpha", "b", "flag", "list", "n", "none", "zeta", "modified"}
	if got := frontmatterKeys(n); !reflect.DeepEqual(got, wantKeys) {
		t.Errorf("frontmatter keys = %q, want %q", got, wantKeys)
	}
	if n.Description != "How to cc" || !reflect.DeepEqual(n.Tags, []string{"escalation", "outbox"}) {
		t.Errorf("description %q tags %q", n.Description, n.Tags)
	}
	// Exact types: "2026-11-14", "007" and "true" stay strings; 3 an int.
	wantProps := []Property{
		{"alpha", "007"}, {"b", false}, {"flag", "true"}, {"list", []string{"a", "b"}},
		{"n", 3}, {"none", nil}, {"zeta", "2026-11-14"},
	}
	if !reflect.DeepEqual(n.Properties, wantProps) {
		t.Errorf("properties = %#v\nwant %#v", n.Properties, wantProps)
	}
	checkBlocksReadBack(t, n, []Block{{ID: "m-aaaaa1", Text: "First claim."}, {ID: "m-bbbbb2", Text: "Second claim, see [[people]]."}})
}

// newNodeOutput checks the bytes of criterion 2's node: no BOM, LF only,
// and the body laid out exactly.
func newNodeOutput(t *testing.T, out []byte) {
	t.Helper()
	s := string(out)
	if strings.HasPrefix(s, bomText) || strings.Contains(s, "\r") {
		t.Errorf("output has a BOM or CR: %q", s)
	}
	fm := frontmatterRegion(t, out)
	if !strings.HasPrefix(fm, "---\n") || !strings.HasSuffix(fm, "\n---\n") {
		t.Errorf("frontmatter not delimited by plain --- lines: %q", fm)
	}
	if body, want := s[len(fm):], "\nFirst claim. ^m-aaaaa1\n\nSecond claim, see [[people]]. ^m-bbbbb2\n"; body != want {
		t.Errorf("body = %q\nwant   %q", body, want)
	}
}

// Values arrive as JSON decodes them (float64, []any, json.Number) and keep
// their type; a node with no tags and no blocks has neither.
func TestEncodeNew_JSONValuesAndMinimalNode(t *testing.T) {
	var props map[string]any
	if err := json.Unmarshal([]byte(`{"f": 1.25, "i": 42, "big": 1e21, "whole": 3.0, "l": ["x", "007"], "e": []}`), &props); err != nil {
		t.Fatal(err)
	}
	props["num"] = json.Number("12")
	out, err := Encoder{}.EncodeNew(NewNode{Description: "d", Properties: props})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(out.Bytes), "\n---\n") {
		t.Errorf("a node without blocks should end at its closing line: %q", out.Bytes)
	}
	n := loadsClean(t, "n", out.Bytes)
	if got := frontmatterKeys(n); !reflect.DeepEqual(got, []string{"description", "big", "e", "f", "i", "l", "num", "whole", "modified"}) {
		t.Errorf("keys = %q", got)
	}
	want := map[string]any{"f": 1.25, "i": 42, "big": 1e21, "whole": 3, "l": []string{"x", "007"}, "e": []string{}, "num": 12}
	for k, w := range want {
		if got, _ := n.Property(k); !reflect.DeepEqual(got, w) {
			t.Errorf("%s = %#v (%T), want %#v (%T)", k, got, got, w, w)
		}
	}
	if len(n.Tags) != 0 || len(n.Blocks) != 0 {
		t.Errorf("tags %q blocks %d, want none", n.Tags, len(n.Blocks))
	}
}

// U5 criterion 1 at the encoder: a draw colliding with an ID already in
// the store is redrawn, and 3 blocks in one encode get 3 distinct IDs
// even when the source repeats an ID it already gave this encode.
func TestEncodeNew_IDsUniqueStoreWideAndWithinWrite(t *testing.T) {
	st, _ := storeOf(t, map[string]string{
		"other/other.md": note("o", "Taken. ^m-aaaaaa\n"),
	})
	enc := Encoder{Store: st, Rand: fixedRand(t, "aaaaaa", "bbbbbb", "bbbbbb", "cccccc", "aaaaaa", "dddddd")}
	out, err := enc.EncodeNew(NewNode{Description: "d", Blocks: []string{"One.", "Two.", "Three."}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"m-bbbbbb", "m-cccccc", "m-dddddd"}
	if !reflect.DeepEqual(out.BlockIDs, want) {
		t.Fatalf("BlockIDs = %q, want %q", out.BlockIDs, want)
	}
	n := loadsClean(t, "n", out.Bytes)
	for i, b := range n.Blocks {
		if b.ID != want[i] {
			t.Errorf("block %d loads with ID %q, want %q", i, b.ID, want[i])
		}
	}
}

// With the default (cryptographic) source, many blocks in one encode all
// get distinct, well-formed IDs.
func TestEncodeNew_DefaultSourceDistinctIDs(t *testing.T) {
	blocks := make([]string, 50)
	for i := range blocks {
		blocks[i] = "Claim."
	}
	out, err := Encoder{}.EncodeNew(NewNode{Description: "d", Blocks: blocks})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, id := range out.BlockIDs {
		if !generatedIDRE.MatchString(id) || seen[id] {
			t.Fatalf("bad or repeated ID %q in %q", id, out.BlockIDs)
		}
		seen[id] = true
	}
	loadsClean(t, "n", out.Bytes)
}
