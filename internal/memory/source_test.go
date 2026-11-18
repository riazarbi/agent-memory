package memory

import (
	"reflect"
	"testing"
)

// U5 read-side additions (D27): the read model records whether the file
// began with a byte-order mark, the opening delimiter line's ending, the
// full source after the BOM, and each block's exact source lines.
func TestSource_FileConventions(t *testing.T) {
	cases := []struct {
		name, content string
		bom           bool
		eol, source   string
	}{
		{"LF", "---\ndescription: d\n---\nA. ^m-aaaaaa\n", false, "\n", "---\ndescription: d\n---\nA. ^m-aaaaaa\n"},
		{"BOM and CRLF", "\ufeff---\r\ndescription: d\r\n---\r\n", true, "\r\n", "---\r\ndescription: d\r\n---\r\n"},
		{"CRLF opening, LF rest", "---\r\ndescription: d\n---\n", false, "\r\n", "---\r\ndescription: d\n---\n"},
		{"LF opening, CRLF rest", "--- \ndescription: d\r\n---\r\nA.", false, "\n", "--- \ndescription: d\r\n---\r\nA."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := loadOne(t, tc.content)
			if n.BOM != tc.bom {
				t.Errorf("BOM = %v, want %v", n.BOM, tc.bom)
			}
			if n.LineEnding != tc.eol {
				t.Errorf("LineEnding = %q, want %q", n.LineEnding, tc.eol)
			}
			if n.Source != tc.source {
				t.Errorf("Source = %q, want %q", n.Source, tc.source)
			}
		})
	}
}

func TestSource_BlockSourceLines(t *testing.T) {
	content := "---\r\ndescription: d\r\n---\r\n\r\n" +
		"First line  \r\nsecond line\t^m-aaaaaa\r\n" +
		"\n\n" +
		"Standalone ID below\n^m-bbbbbb   \n" +
		"  \n" +
		"No ID, no final newline"
	n := loadOne(t, content)
	want := [][]string{
		{"First line  \r\n", "second line\t^m-aaaaaa\r\n"},
		{"Standalone ID below\n", "^m-bbbbbb   \n"},
		{"No ID, no final newline"},
	}
	if len(n.Blocks) != len(want) {
		t.Fatalf("blocks = %d, want %d", len(n.Blocks), len(want))
	}
	for i, b := range n.Blocks {
		if !reflect.DeepEqual(b.Source, want[i]) {
			t.Errorf("block %d Source = %q, want %q", i, b.Source, want[i])
		}
	}
	// The decoded view is unchanged by the addition.
	if n.Blocks[0].Text != "First line  \nsecond line" || n.Blocks[0].ID != "m-aaaaaa" {
		t.Errorf("block 0 = %q %q", n.Blocks[0].ID, n.Blocks[0].Text)
	}
	if n.Blocks[1].Text != "Standalone ID below" || n.Blocks[1].ID != "m-bbbbbb" {
		t.Errorf("block 1 = %q %q", n.Blocks[1].ID, n.Blocks[1].Text)
	}
}
