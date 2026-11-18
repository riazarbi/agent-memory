package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Acceptance criterion 5: an unknown stem exits 1, stdout empty, and stderr
// lists up to five near matches.
func TestReadUnknownStem(t *testing.T) {
	dir, notes := seeded(t)
	line := mustFail(t, runBin(t, dir, nil, "read", "topic"))
	if !strings.Contains(line, "topic") || !strings.Contains(line, "topics") {
		t.Errorf("stderr %q does not name the stem and its near match topics", line)
	}

	for _, s := range []string{"topic-a", "topic-b", "topic-c", "topic-d", "topic-e", "topic-f", "topic-gg"} {
		writeNode(t, notes, "topics/"+s, "---\ndescription: "+s+"\n---\n")
	}
	line = mustFail(t, runBin(t, dir, nil, "read", "topic"))
	_, list, ok := strings.Cut(line, "near matches: ")
	if !ok {
		t.Fatalf("stderr %q has no near-match list", line)
	}
	matches := strings.Split(list, ", ")
	if len(matches) != 5 {
		t.Fatalf("near matches %v, want exactly five of the eight candidates", matches)
	}
	if matches[0] != "topics" {
		t.Errorf("near matches %v: the closest (topics) is not first", matches)
	}
	for _, m := range matches {
		if m == "topic-gg" {
			t.Errorf("near matches %v include the farthest candidate", matches)
		}
	}

	line = mustFail(t, runBin(t, dir, nil, "read", "zzzzzzzzzzzzzzzz"))
	if strings.Contains(line, "near matches") {
		t.Errorf("stderr %q lists near matches for a stem resembling nothing", line)
	}
}

// An unknown flag, or a missing or invalid --depth, exits 1 and stderr
// names the flag as typed.
func TestReadFlagErrors(t *testing.T) {
	dir, _ := seeded(t)
	cases := []struct {
		args  []string
		names string
		not   string
	}{
		{args: []string{"--bogus"}, names: "--bogus"},
		{args: []string{"user", "--bogus=1"}, names: "--bogus"},
		{args: []string{"-bogus"}, names: "-bogus", not: "--bogus"},
		{args: []string{"--depth"}, names: "--depth"},
		{args: []string{"user", "--depth"}, names: "--depth"},
		{args: []string{"-depth"}, names: "-depth", not: "--depth"},
		{args: []string{"--depth", "x", "user"}, names: "--depth"},
		{args: []string{"user", "--depth=abc"}, names: "--depth"},
		{args: []string{"user", "--depth", "-1"}, names: "--depth"},
		{args: []string{"user", "-depth", "1.5"}, names: "-depth", not: "--depth"},
		{args: []string{"user", "--depth="}, names: "--depth"},
		{args: []string{"user", "--block"}, names: "--block"},
	}
	for _, tc := range cases {
		line := mustFail(t, runBin(t, dir, nil, append([]string{"read"}, tc.args...)...))
		if !strings.Contains(line, tc.names) {
			t.Errorf("%v: stderr %q does not name %s", tc.args, line, tc.names)
		}
		if tc.not != "" && strings.Contains(line, tc.not) {
			t.Errorf("%v: stderr %q names %s, not the flag as typed", tc.args, line, tc.not)
		}
		if strings.Contains(line, "provided but not defined") {
			t.Errorf("%v: stderr %q is the flag package's raw message", tc.args, line)
		}
	}
}

// Other argument errors of read: an unknown block, a block without a stem,
// more than one stem.
func TestReadArgumentErrors(t *testing.T) {
	dir, notes := seeded(t)
	writeNode(t, notes, "topics/x", "---\ndescription: X\n---\n\nA claim. ^m-xxxxx1\n")
	line := mustFail(t, runBin(t, dir, nil, "read", "x", "--block", "m-zzzzz9"))
	if !strings.Contains(line, "--block") || !strings.Contains(line, "m-zzzzz9") {
		t.Errorf("unknown block: stderr %q", line)
	}
	line = mustFail(t, runBin(t, dir, nil, "read", "--block", "m-xxxxx1"))
	if !strings.Contains(line, "--block") || !strings.Contains(line, "STEM") {
		t.Errorf("block without stem: stderr %q", line)
	}
	line = mustFail(t, runBin(t, dir, nil, "read", "x", "user"))
	if !strings.Contains(line, "STEM") {
		t.Errorf("two stems: stderr %q", line)
	}
}

// A store without notes/ gives the corrective error for read, and creates
// nothing: no notes/, no lock file, no memory/ or state dir.
func TestReadWithoutNotes(t *testing.T) {
	const want = "memory has no notes/ folder; run memory init"
	t.Run("memory/ without notes/", func(t *testing.T) {
		dir := t.TempDir()
		store := filepath.Join(dir, ".agent", "memory")
		writeFile(t, filepath.Join(store, "protocol", "usage.md"), "guidance\n")
		ageTree(t, filepath.Join(dir, ".agent"))
		before := snapshot(t, filepath.Join(dir, ".agent"))
		for _, args := range [][]string{{"read"}, {"read", "user"}, {"read", "user", "--block", "m-aaaaa1"}} {
			if line := mustFail(t, runBin(t, dir, nil, args...)); line != want {
				t.Errorf("%v: stderr %q, want %q", args, line, want)
			}
		}
		sameSnapshot(t, before, snapshot(t, filepath.Join(dir, ".agent")))
		if exists(filepath.Join(store, "notes")) {
			t.Error("read created notes/")
		}
	})
	t.Run("no memory/ at all", func(t *testing.T) {
		dir := t.TempDir()
		if line := mustFail(t, runBin(t, dir, nil, "read")); line != want {
			t.Errorf("stderr %q, want %q", line, want)
		}
		if exists(filepath.Join(dir, ".agent")) {
			t.Error("read created the state dir")
		}
	})
	t.Run("notes/ is a file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".agent", "memory", "notes"), "not a folder\n")
		if line := mustFail(t, runBin(t, dir, nil, "read")); line != want {
			t.Errorf("stderr %q, want %q", line, want)
		}
	})
}

// A seeded store being read writes nothing either: no lock file appears.
func TestReadWritesNothing(t *testing.T) {
	dir, _ := seeded(t)
	state := filepath.Join(dir, ".agent")
	ageTree(t, state)
	before := snapshot(t, state)
	mustSucceed(t, runBin(t, dir, nil, "read"))
	mustSucceed(t, runBin(t, dir, nil, "read", "user", "--depth", "3"))
	sameSnapshot(t, before, snapshot(t, state))
}

// No verb, or an unknown verb, prints usage to stderr and exits 1.
func TestUsage(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{nil, {"frobnicate"}, {"READ"}, {"--depth", "1"}} {
		r := runBin(t, dir, nil, args...)
		if r.code != 1 || r.stdout != "" {
			t.Errorf("%v: exit %d, stdout %q; want 1 and empty", args, r.code, r.stdout)
		}
		if !strings.Contains(r.stderr, "usage: memory") || !strings.Contains(r.stderr, "init") || !strings.Contains(r.stderr, "read [STEM]") {
			t.Errorf("%v: stderr %q is not the usage text", args, r.stderr)
		}
	}
	r := runBin(t, dir, nil, "frobnicate")
	if !strings.Contains(r.stderr, `"frobnicate"`) {
		t.Errorf("stderr %q does not name the unknown verb", r.stderr)
	}
	if exists(filepath.Join(dir, ".agent")) {
		t.Error("usage created a store")
	}
}

// Acceptance criterion 6: modified follows the process's local zone.
func TestDatesFollowLocalZone(t *testing.T) {
	dir, notes := seeded(t)
	p := writeNode(t, notes, "topics/late", "---\ndescription: Late\n---\n")
	mtime := time.Date(2026, 11, 14, 23, 30, 0, 0, time.UTC)
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	for tz, want := range map[string]string{"UTC": "2026-11-14", "Asia/Tokyo": "2026-11-15"} {
		env := map[string]string{"TZ": tz}
		r := runBin(t, dir, env, "read", "late")
		mustSucceed(t, r)
		var n xNode
		decode(t, r.stdout, &n)
		if n.Modified != want {
			t.Errorf("TZ=%s: read late modified=%q, want %q", tz, n.Modified, want)
		}
		r = runBin(t, dir, env, "read", "topics")
		mustSucceed(t, r)
		decode(t, r.stdout, &n)
		if n.Children == nil || len(n.Children.Nodes) != 1 || n.Children.Nodes[0].Modified != want {
			t.Errorf("TZ=%s: child entry %+v, want modified=%q", tz, n.Children, want)
		}
	}
}
