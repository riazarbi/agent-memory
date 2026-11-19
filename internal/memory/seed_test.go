package memory

import (
	"os"
	"path/filepath"
	"testing"
)

// The first level the spec seeds (D29), stem -> description.
var specFirstLevel = map[string]string{
	"user":     "Who the operator is and how they want work done",
	"projects": "Ongoing efforts, goals, incidents and commitments",
	"people":   "Who is who, and how to deal with them",
	"topics":   "Everything else worth knowing",
}

// The guidance is not seeded: it ships as agent/prompt/memory_usage.md
// (Part III.4). memory/ holds notes/ and nothing else.
func TestSeedDefaults_FreshCreatesFirstLevelOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	if err := SeedDefaults(dir); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	top, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Name() != "notes" {
		t.Errorf("memory/ holds %v, want only notes/", top)
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
