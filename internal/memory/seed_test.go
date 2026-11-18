package memory

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specUsageText returns the default usage.md exactly as the spec pins it:
// the lines of the ```markdown fence that follows "seeded verbatim" in
// docs/spec/agent-memory.md, each ending in a newline.
func specUsageText(t *testing.T) string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "docs", "spec", "agent-memory.md"))
	if err != nil {
		t.Fatalf("open spec: %v", err)
	}
	defer func() { _ = f.Close() }()
	var b strings.Builder
	state := 0 // 0: looking for the anchor, 1: for the fence, 2: inside
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch state {
		case 0:
			if strings.Contains(line, "`usage.md`, seeded verbatim") {
				state = 1
			}
		case 1:
			if line == "```markdown" {
				state = 2
			}
		case 2:
			if line == "```" {
				return b.String()
			}
			b.WriteString(line + "\n")
		}
	}
	t.Fatal("spec has no seeded usage.md block")
	return ""
}

// The first level the spec seeds (D29), stem -> description.
var specFirstLevel = map[string]string{
	"user":     "Who the operator is and how they want work done",
	"projects": "Ongoing efforts, goals, incidents and commitments",
	"people":   "Who is who, and how to deal with them",
	"topics":   "Everything else worth knowing",
}

func TestUsageTextIsTheSpecText(t *testing.T) {
	if want := specUsageText(t); UsageText != want {
		t.Errorf("UsageText differs from the spec's usage.md:\n got %q\nwant %q", UsageText, want)
	}
}

func TestSeedDefaults_FreshCreatesUsageAndFirstLevel(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	if err := SeedDefaults(dir); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "protocol", "usage.md"))
	if err != nil {
		t.Fatalf("usage.md: %v", err)
	}
	if string(got) != specUsageText(t) {
		t.Errorf("usage.md is not byte-equal to the spec text:\n%q", got)
	}
	assertSeededFirstLevel(t, Load(filepath.Join(dir, "notes")))
	// Nothing else is seeded: no stray temp dirs beside memory/.
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("seeding left extra entries beside memory/: %v", entries)
	}
}

// assertSeededFirstLevel checks the store is exactly the spec's first
// level: clean, described, empty.
func assertSeededFirstLevel(t *testing.T, st *Store) {
	t.Helper()
	if p := st.Problems(); len(p) != 0 {
		t.Fatalf("seeded store has problems: %+v", p)
	}
	first := st.FirstLevel()
	if len(first) != len(specFirstLevel) {
		t.Fatalf("first level = %d nodes, want %d", len(first), len(specFirstLevel))
	}
	for _, n := range first {
		want, ok := specFirstLevel[n.Stem]
		if !ok || n.Description != want || n.Path != n.Stem {
			t.Errorf("%s: description %q (want %q), path %q", n.Stem, n.Description, want, n.Path)
		}
		if len(n.Blocks) != 0 || len(n.Tags) != 0 || len(n.Properties) != 0 {
			t.Errorf("%s: seeded with content: blocks=%v tags=%v props=%v", n.Stem, n.Blocks, n.Tags, n.Properties)
		}
	}
}

func seeded(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "memory")
	if err := SeedDefaults(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSeedDefaults_RerunChangesNothing(t *testing.T) {
	dir := seeded(t)
	before := snapshot(t, dir)
	if err := SeedDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, dir); !sameSnapshot(before, after) {
		t.Errorf("second seed changed memory/:\nbefore %v\nafter  %v", before, after)
	}
}

func TestSeedDefaults_DeletedUsageStaysDeleted(t *testing.T) {
	dir := seeded(t)
	usage := filepath.Join(dir, "protocol", "usage.md")
	if err := os.Remove(usage); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(usage); !os.IsNotExist(err) {
		t.Errorf("usage.md was resurrected: %v", err)
	}
}

func TestSeedDefaults_EmptyMemoryDirSeedsNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("seeded into an existing memory/: %v", entries)
	}
}

// snapshot maps every path under root to its content and mode, so a
// rerun that rewrites a file with the same bytes is still caught by
// mtime.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		v := info.Mode().String() + " " + info.ModTime().String()
		if !info.IsDir() {
			data, err := os.ReadFile(p) //nolint:gosec // a test temp dir
			if err != nil {
				return err
			}
			v += " " + string(data)
		}
		out[rel] = v
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
