package main

// CLI4: the store is <parent>/memory, the parent being --parent-dir (before
// the verb), else MEMORY_PARENT_DIR, else HOME (Part III.1).

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// mkdirs creates each directory and returns them.
func mkdirs(t *testing.T, dirs ...string) []string {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dirs
}

// isEmptyDir fails unless dir exists and holds nothing.
func isEmptyDir(t *testing.T, what, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if len(entries) != 0 {
		t.Errorf("%s (%s) holds %d entries, want none", what, dir, len(entries))
	}
}

// isStore fails unless parent/memory is a seeded store.
func isStore(t *testing.T, parent string) {
	t.Helper()
	for _, p := range []string{"memory/notes/topics/topics.md", "memory/notes/user/user.md"} {
		if !exists(filepath.Join(parent, filepath.FromSlash(p))) {
			t.Errorf("%s does not exist", filepath.Join(parent, p))
		}
	}
}

// readsRoot fails unless r is a successful root read of a seeded store.
func readsRoot(t *testing.T, r result) {
	t.Helper()
	mustSucceed(t, r)
	var root xRoot
	decode(t, r.stdout, &root)
	stems := make([]string, 0, len(root.Nodes))
	for _, n := range root.Nodes {
		stems = append(stems, n.Stem)
	}
	check(t, "root stems", stems, []string{"people", "projects", "topics", "user"})
}

// Acceptance criterion 1: --parent-dir, in both spellings, beats
// MEMORY_PARENT_DIR and HOME; a relative one is taken against the working
// directory; a later read with the flag reads that store.
func TestParentDirFlagWins(t *testing.T) {
	for _, spelling := range [][]string{{"--parent-dir", "P"}, {"--parent-dir=P"}} {
		w := t.TempDir()
		p := mkdirs(t, filepath.Join(w, "p"))[0]
		dirs := mkdirs(t, filepath.Join(w, "q"), filepath.Join(w, "h"))
		env := map[string]string{"MEMORY_PARENT_DIR": dirs[0], "HOME": dirs[1]}
		flag := slices.Clone(spelling)
		flag[len(flag)-1] = strings.Replace(flag[len(flag)-1], "P", p, 1)

		mustSucceed(t, runBin(t, w, env, append(flag, "index")...))
		isStore(t, p)
		isEmptyDir(t, spelling[0]+": Q", dirs[0])
		isEmptyDir(t, spelling[0]+": H", dirs[1])
		if exists(filepath.Join(w, ".agent")) {
			t.Errorf("%v: the harness default store was created", spelling)
		}
		readsRoot(t, runBin(t, w, env, append(flag, "read")...))
	}

	w := t.TempDir()
	dirs := mkdirs(t, filepath.Join(w, "q"), filepath.Join(w, "h"), filepath.Join(w, "rel"))
	env := map[string]string{"MEMORY_PARENT_DIR": dirs[0], "HOME": dirs[1]}
	mustSucceed(t, runBin(t, w, env, "--parent-dir", "rel", "index"))
	isStore(t, filepath.Join(w, "rel"))
	isEmptyDir(t, "relative: Q", dirs[0])
	isEmptyDir(t, "relative: H", dirs[1])
	readsRoot(t, runBin(t, w, env, "--parent-dir", "rel", "read"))
}

// Acceptance criterion 2: with no flag, MEMORY_PARENT_DIR beats HOME, and
// each names the parent of the store, never the store itself.
func TestParentFromEnvironment(t *testing.T) {
	w := t.TempDir()
	dirs := mkdirs(t, filepath.Join(w, "q"), filepath.Join(w, "h"))
	env := map[string]string{"MEMORY_PARENT_DIR": dirs[0], "HOME": dirs[1]}
	mustSucceed(t, runBin(t, w, env, "index"))
	isStore(t, dirs[0])
	if exists(filepath.Join(dirs[0], "notes")) {
		t.Error("MEMORY_PARENT_DIR was taken as the store itself")
	}
	isEmptyDir(t, "H, with MEMORY_PARENT_DIR set", dirs[1])
	readsRoot(t, runBin(t, w, env, "read"))

	w = t.TempDir()
	h := mkdirs(t, filepath.Join(w, "h"))[0]
	env = map[string]string{"MEMORY_PARENT_DIR": envUnset, "HOME": h}
	mustSucceed(t, runBin(t, w, env, "index"))
	isStore(t, h)
	if exists(filepath.Join(h, "notes")) {
		t.Error("HOME was taken as the store itself")
	}
	readsRoot(t, runBin(t, w, env, "read"))
}

// failsCleanly runs args in a fresh working dir holding only the dirs Q and
// H, and checks the error convention, that nothing was created, and that
// the line holds each of want.
func failsCleanly(t *testing.T, env map[string]string, args []string, want ...string) {
	t.Helper()
	w := t.TempDir()
	q, h := filepath.Join(w, "q"), filepath.Join(w, "h")
	mkdirs(t, q, h)
	vars := map[string]string{"MEMORY_PARENT_DIR": q, "HOME": h}
	for k, v := range env {
		vars[k] = v
	}
	before := snapshot(t, w)
	line := mustFail(t, runBin(t, w, vars, args...))
	sameSnapshot(t, before, snapshot(t, w))
	for _, s := range want {
		if !strings.Contains(line, s) {
			t.Errorf("%v: stderr %q does not contain %q", args, line, s)
		}
	}
}

// Acceptance criterion 3: a source that is set but empty is an error, never
// a fall-through, and so is having no source at all.
func TestEmptyOrMissingParent(t *testing.T) {
	writeArgs := []string{"write", "kettle", "--parent", "topics", "--description", "d", "--add", "a"}
	for _, verb := range [][]string{{"index"}, {"read"}, writeArgs} {
		failsCleanly(t, map[string]string{"MEMORY_PARENT_DIR": ""}, verb, "MEMORY_PARENT_DIR is set but empty")
		failsCleanly(t, map[string]string{"MEMORY_PARENT_DIR": envUnset, "HOME": ""}, verb, "HOME is set but empty")
		failsCleanly(t, nil, append([]string{"--parent-dir", ""}, verb...), "--parent-dir")
		failsCleanly(t, nil, append([]string{"--parent-dir="}, verb...), "--parent-dir")
		failsCleanly(t, map[string]string{"MEMORY_PARENT_DIR": envUnset, "HOME": envUnset}, verb,
			"--parent-dir", "MEMORY_PARENT_DIR", "HOME")
	}
}

// Acceptance criterion 4: --parent-dir without a value, or twice before the
// verb, is an error naming it; the last occurrence never wins.
func TestMalformedParentDir(t *testing.T) {
	failsCleanly(t, nil, []string{"--parent-dir"}, "--parent-dir")
	failsCleanly(t, nil, []string{"--parent-dir", "p", "--parent-dir", "q", "read"}, "--parent-dir")
	failsCleanly(t, nil, []string{"--parent-dir=p", "--parent-dir=q", "read"}, "--parent-dir")

	// With both stores seeded, a read that let either occurrence win would
	// succeed.
	w := t.TempDir()
	p, q := filepath.Join(w, "p"), filepath.Join(w, "q")
	mkdirs(t, p, q)
	mustSucceed(t, runBin(t, w, nil, "--parent-dir", p, "index"))
	mustSucceed(t, runBin(t, w, nil, "--parent-dir", q, "index"))
	before := snapshot(t, w)
	line := mustFail(t, runBin(t, w, nil, "--parent-dir", p, "--parent-dir", q, "read"))
	sameSnapshot(t, before, snapshot(t, w))
	if !strings.Contains(line, "--parent-dir") {
		t.Errorf("stderr %q does not name --parent-dir", line)
	}
}

// Acceptance criterion 5: after the verb, --parent-dir is not an option and
// never changes the store, including after a tool file's pinned one.
func TestParentDirAfterVerb(t *testing.T) {
	w, notes := seeded(t)
	x := filepath.Join(w, "x")
	for _, args := range [][]string{
		{"read", "--parent-dir", x},
		{"index", "--parent-dir", x},
		{"write", "kettle", "--parent-dir", x, "--description", "d", "--add", "a"},
		{"write", "kettle", "--parent-dir=" + x, "--parent", "topics", "--description", "d", "--add", "a"},
		{"--parent-dir", filepath.Join(w, ".agent"), "read", "--parent-dir", x},
		{"--parent-dir", filepath.Join(w, ".agent"), "write", "kettle", "--parent-dir", x, "--parent", "topics", "--description", "d", "--add", "a"},
	} {
		line := mustFail(t, runBin(t, w, nil, args...))
		if !strings.Contains(line, "--parent-dir") {
			t.Errorf("%v: stderr %q does not name --parent-dir", args, line)
		}
		if exists(x) {
			t.Fatalf("%v created %s", args, x)
		}
	}
	mustFail(t, runBin(t, w, nil, "search", "--parent-dir", x))
	if exists(x) {
		t.Fatalf("search created %s", x)
	}
	err := filepath.WalkDir(notes, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "kettle" {
			t.Errorf("a kettle node exists at %s", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Acceptance criterion 6: no .env and no agent variable is consulted, and
// the usage names the three sources and nothing retired.
func TestNoAgentAwareness(t *testing.T) {
	w := t.TempDir()
	h := mkdirs(t, filepath.Join(w, "h"))[0]
	writeFile(t, filepath.Join(w, ".env"), "MEMORY_PARENT_DIR="+filepath.Join(w, "e")+"\nAGENT_STATE_DIR="+filepath.Join(w, "e2")+"\nMEMORY_DIR="+filepath.Join(w, "e3")+"\n")
	env := map[string]string{
		"HOME":              h,
		"MEMORY_PARENT_DIR": envUnset,
		"AGENT_STATE_DIR":   filepath.Join(w, "s"),
		"MEMORY_DIR":        filepath.Join(w, "m"),
	}
	mustSucceed(t, runBin(t, w, env, "index"))
	isStore(t, h)
	entries, err := os.ReadDir(w)
	if err != nil {
		t.Fatal(err)
	}
	top := make([]string, 0, len(entries))
	for _, e := range entries {
		top = append(top, e.Name())
	}
	check(t, "entries in the working dir after the first call", top, []string{".env", "h"})

	r := runBin(t, w, env)
	if r.code != 1 || r.stdout != "" {
		t.Fatalf("no arguments: exit %d, stdout %q; want usage on stderr, exit 1", r.code, r.stdout)
	}
	for _, s := range []string{"--parent-dir", "MEMORY_PARENT_DIR", "HOME"} {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("usage does not name %s:\n%s", s, r.stderr)
		}
	}
	for _, s := range retiredNames {
		if strings.Contains(r.stderr, s) {
			t.Errorf("usage names %s:\n%s", s, r.stderr)
		}
	}
}

// parseLocation stops at the verb, keeps everything after it untouched, and
// splits --parent-dir=VALUE at the first = only.
func TestParseLocation(t *testing.T) {
	cases := []struct {
		args   []string
		parent string
		rest   []string
	}{
		{[]string{"read", "--parent-dir", "x"}, "", []string{"read", "--parent-dir", "x"}},
		{[]string{"--parent-dir=a=b", "read"}, "a=b", []string{"read"}},
		{[]string{"--parent-dir", "-x", "read", "--parent-dir=y"}, "-x", []string{"read", "--parent-dir=y"}},
		{[]string{"--parent-dir", "p"}, "p", []string{}},
		{[]string{"-parent-dir", "p", "read"}, "", []string{"-parent-dir", "p", "read"}},
	}
	for _, tc := range cases {
		loc, rest, err := parseLocation(tc.args)
		check(t, strings.Join(tc.args, " ")+": error", err, nil)
		check(t, strings.Join(tc.args, " ")+": parsed", []any{loc.parentDir, rest}, []any{tc.parent, tc.rest})
	}
}

// retiredNames are the store sources CLI4 removed.
var retiredNames = []string{"AGENT_STATE_DIR", "MEMORY_DIR", ".env"}

// Acceptance criterion 6: no production file in this package mentions a
// retired source.
func TestNoRetiredNamesInSource(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, s := range retiredNames {
			if strings.Contains(string(data), s) {
				t.Errorf("%s mentions %s", e.Name(), s)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no production files found")
	}
}

// Acceptance criterion 7: the README's store paragraph gives III.1's order
// and its empty-value rule, and nothing retired.
func TestReadmeStoreParagraph(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	para := storeParagraph(t, readmeSection(t, string(data), "Usage"))
	at := -1
	for _, s := range []string{"`<parent>/memory`", "--parent-dir", "MEMORY_PARENT_DIR", "HOME"} {
		i := strings.Index(para, s)
		if i <= at {
			t.Errorf("the store paragraph does not name %s after the previous source:\n%s", s, para)
		}
		at = i
	}
	for _, s := range []string{"set but empty", "before the verb"} {
		if !strings.Contains(para, s) {
			t.Errorf("the store paragraph does not say %q:\n%s", s, para)
		}
	}
	for _, s := range retiredNames {
		if strings.Contains(para, s) {
			t.Errorf("the store paragraph names %s:\n%s", s, para)
		}
	}
	if strings.Contains(string(data), "MEMORY_DIR") {
		t.Error("the README still names the retired MEMORY_DIR")
	}
}

// storeParagraph is the paragraph of section that starts "The store is".
func storeParagraph(t *testing.T, section string) string {
	t.Helper()
	for _, p := range strings.Split(section, "\n\n") {
		if strings.HasPrefix(p, "The store is") {
			return p
		}
	}
	t.Fatal("the Usage section has no paragraph starting \"The store is\"")
	return ""
}
