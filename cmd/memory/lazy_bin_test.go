package main

// CLI6, as revised by CLI7: there is no install step; the parent must
// already exist; a missing store is an empty one; init is gone
// (docs/spec/agent-memory.md Part III.1).

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// freshParent makes a working dir W holding an empty W/.agent, runBin's
// default parent, and returns W and W/.agent.
func freshParent(t *testing.T) (w, parent string) {
	t.Helper()
	w = t.TempDir()
	parent = filepath.Join(w, ".agent")
	mkdirs(t, parent)
	return w, parent
}

// allVerbs is one valid call of each verb.
var allVerbs = [][]string{
	{"read"},
	{"index"},
	{"search", "anything"},
	{"write", "kettle", "--parent", "topics", "--description", "d", "--add", "a"},
}

// failsExactly runs args in w with env, and checks the error convention,
// the exact line, and that nothing under w changed.
func failsExactly(t *testing.T, w string, env map[string]string, args []string, want string) {
	t.Helper()
	before := snapshot(t, w)
	line := mustFail(t, runBin(t, w, env, args...))
	if line != want {
		t.Errorf("%v: stderr %q, want %q", args, line, want)
	}
	sameSnapshot(t, before, snapshot(t, w))
}

// Acceptance criterion 2: a missing parent fails, naming the parent as
// given and its source, and creates nothing — whichever source named it.
func TestParentMustExist(t *testing.T) {
	for _, verb := range allVerbs {
		w := t.TempDir()
		nope := filepath.Join(w, "nope")
		failsExactly(t, w, nil, append([]string{"--parent-dir", nope}, verb...),
			"parent dir "+nope+" does not exist (from --parent-dir)")
		failsExactly(t, w, nil, append([]string{"--parent-dir=" + nope}, verb...),
			"parent dir "+nope+" does not exist (from --parent-dir)")
		// Relative, it stays relative in the message.
		failsExactly(t, w, map[string]string{"MEMORY_PARENT_DIR": "nope"}, verb,
			"parent dir nope does not exist (from MEMORY_PARENT_DIR)")
		h := filepath.Join(w, "h")
		failsExactly(t, w, map[string]string{"MEMORY_PARENT_DIR": envUnset, "HOME": h}, verb,
			"parent dir "+h+" does not exist (from HOME)")
		// runBin's own default: HOME is <w>/home, which does not exist.
		failsExactly(t, w, map[string]string{"MEMORY_PARENT_DIR": envUnset}, verb,
			"parent dir "+filepath.Join(w, "home")+" does not exist (from HOME)")
	}
}

// Acceptance criterion 2: a parent that is not a directory fails the same
// way, for each source.
func TestParentMustBeADirectory(t *testing.T) {
	w := t.TempDir()
	f := filepath.Join(w, "file")
	writeFile(t, f, "not a dir")
	for _, verb := range allVerbs {
		failsExactly(t, w, nil, append([]string{"--parent-dir", f}, verb...),
			"parent dir "+f+" is not a directory (from --parent-dir)")
		failsExactly(t, w, map[string]string{"MEMORY_PARENT_DIR": "file"}, verb,
			"parent dir file is not a directory (from MEMORY_PARENT_DIR)")
		failsExactly(t, w, map[string]string{"MEMORY_PARENT_DIR": envUnset, "HOME": f}, verb,
			"parent dir "+f+" is not a directory (from HOME)")
	}
}

// Acceptance criterion 2: an older agent passes ${AGENT_STATE_DIR}
// literally; that fails loudly and creates no such directory.
func TestLiteralStateDirPlaceholderFails(t *testing.T) {
	const literal = "${AGENT_STATE_DIR}"
	for _, verb := range allVerbs {
		w := t.TempDir()
		failsExactly(t, w, nil, append([]string{"--parent-dir", literal}, verb...),
			"parent dir "+literal+" does not exist (from --parent-dir)")
		if exists(filepath.Join(w, literal)) {
			t.Fatalf("%v created %s", verb, literal)
		}
	}
}

// Acceptance criterion 2: a symlink to a directory works as the parent, and
// the store lands in the directory it points at.
func TestSymlinkedParent(t *testing.T) {
	w := t.TempDir()
	real := mkdirs(t, filepath.Join(w, "real"))[0]
	link := filepath.Join(w, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	mustSucceed(t, runBin(t, w, nil, append([]string{"--parent-dir", link}, firstWrite...)...))
	isStore(t, real)
	readsRoot(t, runBin(t, w, map[string]string{"MEMORY_PARENT_DIR": "link"}, "read"))
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the parent symlink was replaced: %v %v", info, err)
	}
}

// Acceptance criterion 3b: an existing notes/ is never topped up, so a
// deleted first-level node stays deleted.
func TestDeletedFirstLevelNodeStaysDeleted(t *testing.T) {
	w, notes := seeded(t)
	if err := os.RemoveAll(filepath.Join(notes, "people")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r := runBin(t, w, nil, "read")
		mustSucceed(t, r)
		var root xRoot
		decode(t, r.stdout, &root)
		stems := make([]string, 0, len(root.Nodes))
		for _, n := range root.Nodes {
			stems = append(stems, n.Stem)
		}
		check(t, "root stems", stems, []string{"projects", "topics", "user"})
	}
	if exists(filepath.Join(notes, "people")) {
		t.Error("people was recreated")
	}
}

// Acceptance criterion 3c: an empty notes/ is a store with no nodes: index
// prints nothing and nothing is created.
func TestEmptyNotesIsNotSeeded(t *testing.T) {
	w, parent := freshParent(t)
	mkdirs(t, filepath.Join(parent, "memory", "notes"))
	ageTree(t, parent)
	before := snapshot(t, parent)
	r := runBin(t, w, nil, "index")
	mustSucceed(t, r)
	if r.stdout != "" {
		t.Errorf("index on an empty notes/ printed %q", r.stdout)
	}
	readEmpty := runBin(t, w, nil, "read")
	mustSucceed(t, readEmpty)
	var root xRoot
	decode(t, readEmpty.stdout, &root)
	check(t, "root entries of an empty notes/", len(root.Nodes), 0)
	sameSnapshot(t, before, snapshot(t, parent))
}

// Acceptance criterion 4: an argument error comes before seeding.
func TestArgumentErrorsComeBeforeSeeding(t *testing.T) {
	for _, args := range [][]string{
		{"read", "--depth", "x"},
		{"index", "extra"},
		{"write"},
		{"search", "a", "b"},
		{"read", "--bogus"},
		{"write", "kettle", "--bogus"},
	} {
		w, parent := freshParent(t)
		mustFail(t, runBin(t, w, nil, args...))
		isEmptyDir(t, strings.Join(args, " "), parent)
	}
}

// Acceptance criterion 4: memory/ or memory/notes existing as something
// other than a directory is an error naming that path, and nothing changes.
func TestNonDirectoryStoreIsAnError(t *testing.T) {
	for _, verb := range allVerbs {
		w, parent := freshParent(t)
		mem := filepath.Join(parent, "memory")
		writeFile(t, mem, "a file")
		failsExactly(t, w, nil, verb, mem+" is not a directory")

		w, parent = freshParent(t)
		notes := filepath.Join(parent, "memory", "notes")
		writeFile(t, notes, "a file")
		failsExactly(t, w, nil, verb, notes+" is not a directory")
	}
}

// Acceptance criterion 4: the path is named as resolved from the parent as
// given, so a relative parent gives a relative path.
func TestNonDirectoryStoreNamedAsResolved(t *testing.T) {
	w, _ := freshParent(t)
	writeFile(t, filepath.Join(w, ".agent", "memory"), "a file")
	failsExactly(t, w, map[string]string{"MEMORY_PARENT_DIR": ".agent"}, []string{"read"},
		filepath.Join(".agent", "memory")+" is not a directory")
}

// Acceptance criterion 6: init is gone, not hidden.
func TestInitIsGone(t *testing.T) {
	w, parent := freshParent(t)
	r := runBin(t, w, nil, "init")
	if r.code != 1 || r.stdout != "" {
		t.Fatalf("init: exit %d, stdout %q; want exit 1 and nothing on stdout", r.code, r.stdout)
	}
	first, rest, _ := strings.Cut(r.stderr, "\n")
	check(t, "first stderr line", first, `unknown verb "init"`)
	if !strings.HasPrefix(rest, "usage: memory") {
		t.Errorf("init: stderr after the first line is not the usage text:\n%s", rest)
	}
	isEmptyDir(t, "the parent after init", parent)

	usage := runBin(t, w, nil)
	if usage.code != 1 || !strings.HasPrefix(usage.stderr, "usage: memory") {
		t.Fatalf("no verb: exit %d, stderr %q", usage.code, usage.stderr)
	}
	if strings.Contains(usage.stderr, "init") {
		t.Errorf("the usage text mentions init:\n%s", usage.stderr)
	}
}

// initWord matches init as a word or verb (`init`, memory init), not inside
// other words.
var initWord = regexp.MustCompile(`\binit\b`)

// Acceptance criterion 6: neither README tells the operator to run init or
// shows its error; README.md gives the missing-parent error and says the
// first call creates the store.
func TestReadmesHaveNoInit(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{"README.md", filepath.Join("agent", "README.md")} {
		data, err := os.ReadFile(filepath.Join(root, rel)) //nolint:gosec // the repo's own file
		if err != nil {
			t.Fatal(err)
		}
		if loc := initWord.FindIndex(data); loc != nil {
			t.Errorf("%s mentions init: %q", rel, data[max(0, loc[0]-60):min(len(data), loc[1]+20)])
		}
		if bytes.Contains(data, []byte("has no notes/ folder")) {
			t.Errorf("%s still gives the missing-notes error", rel)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	usage := readmeSection(t, string(data), "Usage")
	for _, s := range []string{"memory read ", "does not exist (from --parent-dir)", "must already exist"} {
		if !strings.Contains(usage, s) {
			t.Errorf("the README's Usage section does not say %q", s)
		}
	}
	if !strings.Contains(storeParagraph(t, usage), "plain `memory read` gives `~/memory`") {
		t.Error("the store paragraph does not say plain `memory read` gives `~/memory`")
	}
}
