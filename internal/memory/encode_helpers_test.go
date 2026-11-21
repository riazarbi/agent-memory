package memory

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedRand is an ID source that draws exactly the given suffixes, then
// fails: a test that needs more IDs than it planned for errors out.
func fixedRand(t *testing.T, suffixes ...string) io.Reader {
	t.Helper()
	return bytes.NewReader(idDraws(t, suffixes...))
}

// loadsClean writes data as the folder note of node stem in a fresh notes/
// tree, loads it through Load - the reader is the oracle - and fails unless
// it loads with no errors and no warnings except dangling-link warnings. It
// returns the loaded node.
func loadsClean(t *testing.T, stem string, data []byte) *Node {
	t.Helper()
	s := Load(writeTree(t, map[string]string{stem + "/" + stem + ".md": string(data)}))
	for _, p := range s.Problems() {
		if p.Severity == SeverityWarning && strings.Contains(p.Reason, "points to a missing") {
			continue
		}
		t.Fatalf("encoded note does not load clean: %s %s: %s\n--- note ---\n%s", p.Severity, p.Path, p.Reason, data)
	}
	return mustNode(t, s, stem)
}

// storeOf writes files into a fresh notes/ tree and loads it, failing on
// any problem other than a dangling link.
func storeOf(t *testing.T, files map[string]string) (*Store, string) {
	t.Helper()
	dir := writeTree(t, files)
	s := Load(dir)
	for _, p := range s.Problems() {
		if p.Severity == SeverityWarning && strings.Contains(p.Reason, "points to a missing") {
			continue
		}
		t.Fatalf("fixture does not load clean: %s", dump(s.Problems()))
	}
	return s, dir
}

// fileBytes reads the folder note of stem under notes/.
func fileBytes(t *testing.T, notesDir, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(notesDir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// wantFieldError fails unless err is a *FieldError naming field, and enc
// carries no bytes.
func wantFieldError(t *testing.T, enc Encoded, err error, field string) {
	t.Helper()
	fe, ok := err.(*FieldError)
	if !ok {
		t.Fatalf("err = %v (%T), want a *FieldError naming %s", err, err, field)
	}
	if fe.Field != field {
		t.Errorf("error names field %q, want %q (%v)", fe.Field, field, fe)
	}
	if enc.Bytes != nil || enc.BlockIDs != nil || enc.Adopted != nil {
		t.Errorf("rejected encode produced output: %q", enc.Bytes)
	}
}

// frontmatterKeys returns the node's frontmatter keys in file order.
func frontmatterKeys(n *Node) []string {
	out := make([]string, 0, len(n.Frontmatter.Fields))
	for _, f := range n.Frontmatter.Fields {
		out = append(out, f.Key.Value)
	}
	return out
}

func strp(s string) *string { return &s }

func tagsp(tags ...string) *[]string {
	if tags == nil {
		tags = []string{}
	}
	return &tags
}

// frontmatterRegion returns the bytes of a note up to and including its
// closing --- line.
func frontmatterRegion(t *testing.T, data []byte) string {
	t.Helper()
	s := string(data)
	lines := strings.SplitAfter(s, "\n")
	for i := 1; i < len(lines); i++ {
		if isDelimiter(strings.TrimRight(lines[i], "\r\n")) {
			return strings.Join(lines[:i+1], "")
		}
	}
	t.Fatalf("no closing delimiter in %q", s)
	return ""
}

// clockText is the stored modified value of a content change made while
// fixClock holds the clock (III.9).
const clockText = "2026-11-21T14:03:07Z"

// fixClock makes the time of every call clockText plus a fraction of a
// second, so the modified line a content change writes is an exact string.
func fixClock(t *testing.T) {
	t.Helper()
	old := now
	now = func() time.Time { return time.Date(2026, 11, 21, 14, 3, 7, 400_000_000, time.UTC) }
	t.Cleanup(func() { now = old })
}

// stamped is note with the line "modified: value" added as the last line of
// its frontmatter, directly before the closing --- line and with the opening
// --- line's ending: what III.9 makes of a note without the key when memory
// changes its content (value the time of the call) or rewrites it
// mechanically (value the file's prior mtime).
func stamped(note, value string) string {
	first := strings.Index(note, "\n")
	if first < 0 {
		panic("stamped: no frontmatter in " + note)
	}
	eol := "\n"
	if first > 0 && note[first-1] == '\r' {
		eol = "\r\n"
	}
	for i := first + 1; i < len(note); {
		end := strings.Index(note[i:], "\n")
		line := note[i:]
		if end >= 0 {
			line = note[i : i+end]
		}
		if isDelimiter(strings.TrimRight(line, "\r")) {
			return note[:i] + "modified: " + value + eol + note[i:]
		}
		if end < 0 {
			break
		}
		i += end + 1
	}
	panic("stamped: no closing --- in " + note)
}
