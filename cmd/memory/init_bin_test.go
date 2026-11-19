package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// The D29 seeding descriptions, as the spec states them.
var seedDescriptions = map[string]string{
	"user":     "Who the operator is and how they want work done",
	"projects": "Ongoing efforts, goals, incidents and commitments",
	"people":   "Who is who, and how to deal with them",
	"topics":   "Everything else worth knowing",
}

// Acceptance criterion 2, first part: on a missing store init seeds the four
// first-level nodes (and, since CLI5, nothing else), exits 0 and says what it
// created.
func TestInitSeedsAMissingStore(t *testing.T) {
	dir := t.TempDir()
	r := runBin(t, dir, nil, "init")
	mustSucceed(t, r)
	store := filepath.Join(dir, ".agent", "memory")

	if exists(filepath.Join(store, "protocol")) {
		t.Error("init seeded protocol/; the guidance ships as agent/prompt/memory_usage.md")
	}

	st := memory.Load(filepath.Join(store, "notes"))
	check(t, "problems in the seeded notes/", st.Problems(), []memory.Problem(nil))
	got := map[string]string{}
	for _, n := range st.Nodes() {
		check(t, n.Path+" is first-level", n.Parent() == nil, true)
		check(t, n.Stem+" blocks", len(n.Blocks), 0)
		got[n.Stem] = n.Description
	}
	check(t, "seeded first level", got, seedDescriptions)

	var out xInit
	decode(t, r.stdout, &out)
	check(t, "result element", out.XMLName.Local, "init")
	check(t, "seeded", out.Seeded, "true")
	check(t, "created", out.Created, []string{"notes/people/people.md", "notes/projects/projects.md", "notes/topics/topics.md", "notes/user/user.md"})
}

type fileState struct {
	data  []byte
	mtime time.Time
	mode  fs.FileMode
}

// snapshot records every entry under root: bytes, mtime and mode.
func snapshot(t *testing.T, root string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		st := fileState{mtime: info.ModTime(), mode: info.Mode()}
		if d.Type().IsRegular() {
			if st.data, err = os.ReadFile(p); err != nil { //nolint:gosec // a test temp dir
				return err
			}
		}
		out[p] = st
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ageTree sets every mtime under root into the past, so any rewrite shows.
func ageTree(t *testing.T, root string) {
	t.Helper()
	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, past, past) //nolint:gosec // a test temp dir
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sameSnapshot(t *testing.T, before, after map[string]fileState) {
	t.Helper()
	for p, b := range before {
		a, ok := after[p]
		switch {
		case !ok:
			t.Errorf("%s disappeared", p)
		case !bytes.Equal(a.data, b.data):
			t.Errorf("%s changed content", p)
		case !a.mtime.Equal(b.mtime):
			t.Errorf("%s mtime changed %v -> %v", p, b.mtime, a.mtime)
		case a.mode != b.mode:
			t.Errorf("%s mode changed %v -> %v", p, b.mode, a.mode)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s appeared", p)
		}
	}
}

// Acceptance criterion 2, second part: a second run changes nothing, exits 0
// and says nothing was needed.
func TestInitSecondRunChangesNothing(t *testing.T) {
	dir := t.TempDir()
	mustSucceed(t, runBin(t, dir, nil, "init"))
	state := filepath.Join(dir, ".agent")
	ageTree(t, state)
	before := snapshot(t, state)

	r := runBin(t, dir, nil, "init")
	mustSucceed(t, r)
	sameSnapshot(t, before, snapshot(t, state))

	var out xInit
	decode(t, r.stdout, &out)
	if out.Seeded != "false" || len(out.Created) != 0 {
		t.Errorf("second init result %+v, want seeded=\"false\" and nothing created", out)
	}
	if !strings.Contains(out.Text, "nothing") {
		t.Errorf("second init says %q, want it to say nothing was needed", out.Text)
	}
}

// Acceptance criterion 2, third part: an existing memory/ is never topped up.
func TestInitNeverTopsUp(t *testing.T) {
	t.Run("deleted first-level node", func(t *testing.T) {
		dir := t.TempDir()
		mustSucceed(t, runBin(t, dir, nil, "init"))
		people := filepath.Join(dir, ".agent", "memory", "notes", "people")
		if err := os.RemoveAll(people); err != nil {
			t.Fatal(err)
		}
		mustSucceed(t, runBin(t, dir, nil, "init"))
		if exists(people) {
			t.Error("init recreated a deleted first-level node")
		}
	})
	t.Run("empty memory/", func(t *testing.T) {
		dir := t.TempDir()
		store := filepath.Join(dir, ".agent", "memory")
		if err := os.MkdirAll(store, 0o755); err != nil {
			t.Fatal(err)
		}
		r := runBin(t, dir, nil, "init")
		mustSucceed(t, r)
		entries, err := os.ReadDir(store)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("init seeded into an existing empty memory/: %v", entries)
		}
		var out xInit
		decode(t, r.stdout, &out)
		if out.Seeded != "false" {
			t.Errorf("init reported seeded=%q for an existing memory/", out.Seeded)
		}
	})
}

// init takes no arguments.
func TestInitRefusesArguments(t *testing.T) {
	dir := t.TempDir()
	mustFail(t, runBin(t, dir, nil, "init", "extra"))
	if exists(filepath.Join(dir, ".agent")) {
		t.Error("a refused init created the store")
	}
}
