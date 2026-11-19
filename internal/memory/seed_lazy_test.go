package memory

// CLI6: SeedDefaults is triggered by notes/, not memory/ (Part III.1).

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// entriesOf lists the names in dir.
func entriesOf(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// An existing, empty memory/ gets notes/ with the first level, and nothing
// else.
func TestSeedDefaults_EmptyMemoryDirGetsNotes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(dir); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	if got := entriesOf(t, dir); len(got) != 1 || got[0] != "notes" {
		t.Errorf("memory/ holds %v, want only notes", got)
	}
	assertSeededFirstLevel(t, Load(filepath.Join(dir, "notes")))
	// An existing memory/ keeps its mode; notes/ gets the folder mode, not
	// the temporary directory's 0700.
	assertMode(t, dir, 0o700)
	assertMode(t, filepath.Join(dir, "notes"), folderMode)
}

// A memory/ that SeedDefaults creates gets the folder mode, as notes/ and
// the seeded files get theirs.
func TestSeedDefaults_Modes(t *testing.T) {
	dir := seeded(t)
	assertMode(t, dir, folderMode)
	assertMode(t, filepath.Join(dir, "notes"), folderMode)
	for _, n := range seedFirstLevel {
		assertMode(t, filepath.Join(dir, "notes", n.stem, n.stem+".md"), noteMode)
	}
}

func assertMode(t *testing.T, p string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Errorf("%s: mode %v, want %v", p, info.Mode().Perm(), want)
	}
}

// Entries already in memory/ are left as they are; notes/ is added beside
// them.
func TestSeedDefaults_KeepsOtherEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	keep := filepath.Join(dir, "protocol", "usage.md")
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("operator text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, filepath.Join(dir, "protocol"))
	if err := SeedDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, filepath.Join(dir, "protocol")); !sameSnapshot(before, after) {
		t.Errorf("protocol/ changed:\nbefore %v\nafter  %v", before, after)
	}
	if got := strings.Join(entriesOf(t, dir), " "); got != "notes protocol" {
		t.Errorf("memory/ holds %q, want notes and protocol", got)
	}
	assertSeededFirstLevel(t, Load(filepath.Join(dir, "notes")))
}

// An existing notes/ is untouched — empty, or missing a first-level node.
func TestSeedDefaults_ExistingNotesUntouched(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(filepath.Join(empty, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := seeded(t)
	if err := os.RemoveAll(filepath.Join(partial, "notes", "people")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{empty, partial} {
		before := snapshot(t, dir)
		if err := SeedDefaults(dir); err != nil {
			t.Fatal(err)
		}
		if after := snapshot(t, dir); !sameSnapshot(before, after) {
			t.Errorf("%s changed:\nbefore %v\nafter  %v", dir, before, after)
		}
	}
}

// memory/'s parent is never created: a missing parent is an error, and
// nothing is created.
func TestSeedDefaults_MissingParentIsAnError(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "nope", "memory")
	if err := SeedDefaults(dir); err == nil {
		t.Fatal("SeedDefaults succeeded with memory/'s parent missing")
	}
	if got := entriesOf(t, root); len(got) != 0 {
		t.Errorf("a failed seed created %v", got)
	}
}

// A memory/ that is a regular file is an error, and is left alone.
func TestSeedDefaults_MemoryDirIsAFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.WriteFile(dir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, filepath.Dir(dir))
	if err := SeedDefaults(dir); err == nil {
		t.Fatal("SeedDefaults succeeded with memory/ a regular file")
	}
	if after := snapshot(t, filepath.Dir(dir)); !sameSnapshot(before, after) {
		t.Errorf("a failed seed changed things:\nbefore %v\nafter  %v", before, after)
	}
}

// A seed that cannot build its temporary directory fails, leaving no
// notes/ and no temporary directory.
func TestSeedDefaults_FailureLeavesNoNotes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := SeedDefaults(dir); err == nil {
		t.Fatal("SeedDefaults succeeded in a read-only memory/")
	}
	if got := entriesOf(t, dir); len(got) != 0 {
		t.Errorf("a failed seed left %v", got)
	}
}

// Concurrent seeds of one memory/ all succeed; exactly one notes/ results,
// and no temporary directory survives.
func TestSeedDefaults_Concurrent(t *testing.T) {
	for range 20 {
		dir := filepath.Join(t.TempDir(), "memory")
		const n = 8
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[i] = SeedDefaults(dir)
			}()
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("seed %d: %v", i, err)
			}
		}
		if got := entriesOf(t, dir); len(got) != 1 || got[0] != "notes" {
			t.Fatalf("memory/ holds %v, want only notes", got)
		}
		assertSeededFirstLevel(t, Load(filepath.Join(dir, "notes")))
		if got := entriesOf(t, filepath.Join(dir, "notes")); len(got) != len(seedFirstLevel) {
			t.Fatalf("notes/ holds %v", got)
		}
	}
}
