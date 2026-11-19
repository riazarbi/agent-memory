package main

// CLI6: there is no install step. Every verb seeds <parent>/memory/notes/
// with the first level when notes/ is missing; the parent must already
// exist; init is gone (docs/spec/agent-memory.md Part III.1).

import (
	"bytes"
	"encoding/xml"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// seedStems are the first-level nodes a fresh store gets (D19).
var seedStems = []string{"people", "projects", "topics", "user"}

// seedFiles is notes/'s content right after seeding, relative to the
// parent dir.
func seedFiles() []string {
	out := make([]string, 0, len(seedStems))
	for _, s := range seedStems {
		out = append(out, "memory/notes/"+s+"/"+s+".md")
	}
	return out
}

// freshParent makes a working dir W holding an empty W/.agent, runBin's
// default parent, and returns W and W/.agent.
func freshParent(t *testing.T) (w, parent string) {
	t.Helper()
	w = t.TempDir()
	parent = filepath.Join(w, ".agent")
	mkdirs(t, parent)
	return w, parent
}

// firstCall is a verb run as the first call on a fresh store, and what its
// success looks like.
type firstCall struct {
	args  []string
	check func(t *testing.T, r result)
	extra []string // files beyond the seeds it leaves, relative to the parent
}

func firstCalls() []firstCall {
	return []firstCall{
		{args: []string{"read"}, check: readsRoot},
		{args: []string{"index"}, check: func(t *testing.T, r result) {
			t.Helper()
			mustSucceed(t, r)
			var ix struct {
				XMLName xml.Name
				Nodes   []xEntry `xml:"node"`
			}
			decode(t, r.stdout, &ix)
			check(t, "index element", ix.XMLName.Local, "memory_index")
			stems := make([]string, 0, len(ix.Nodes))
			for _, n := range ix.Nodes {
				stems = append(stems, n.Stem)
			}
			check(t, "index stems", stems, seedStems)
		}},
		{args: []string{"search", "anything"}, check: func(t *testing.T, r result) {
			t.Helper()
			mustSucceed(t, r)
			var s xSearch
			decode(t, r.stdout, &s)
			check(t, "search element, matched", []string{s.XMLName.Local, s.Matched}, []string{"memory_search", "0"})
		}},
		{
			args: []string{"write", "kettle", "--parent", "topics", "--description", "d", "--add", "a"},
			check: func(t *testing.T, r result) {
				t.Helper()
				mustSucceed(t, r)
				var w struct {
					XMLName xml.Name
					Created string `xml:"created,attr"`
				}
				decode(t, r.stdout, &w)
				check(t, "write element, created", []string{w.XMLName.Local, w.Created}, []string{"memory_write", "true"})
			},
			extra: []string{"memory/notes/topics/kettle/kettle.md", "memory/notes/.write.lock"},
		},
	}
}

// Acceptance criterion 1: any verb, as the first call on a fresh store,
// seeds it silently and does its job; the parent then holds the seeds (and
// what the verb itself wrote) and nothing else.
func TestFirstCallOfAnyVerbSeeds(t *testing.T) {
	for _, fc := range firstCalls() {
		t.Run(fc.args[0], func(t *testing.T) {
			w, parent := freshParent(t)
			fc.check(t, runBin(t, w, nil, fc.args...)) // mustSucceed: stderr empty
			check(t, "the parent after the first call", treeUnder(t, parent), withDirs("", append(seedFiles(), fc.extra...)))
		})
	}
}

// Acceptance criterion 1: seeding is silent, so a read that seeded prints
// exactly what the same read prints on a store an earlier call seeded.
func TestSeedingReadMatchesSeededRead(t *testing.T) {
	w, _ := freshParent(t)
	first := runBin(t, w, nil, "read")
	readsRoot(t, first)
	again := runBin(t, w, nil, "read")
	mustSucceed(t, again)

	other, _ := seeded(t) // seeded by an index call
	later := runBin(t, other, nil, "read")
	mustSucceed(t, later)
	if first.stdout != again.stdout || first.stdout != later.stdout {
		t.Errorf("a seeding read differs from a read of a seeded store:\nseeding: %s\nsame store again: %s\nseeded earlier: %s",
			first.stdout, again.stdout, later.stdout)
	}
}

// Acceptance criterion 1: the seeds are encoded exactly as a write encodes
// a new node with that description and no blocks, and the directories and
// files get the store's modes, not a temporary directory's.
func TestSeedsAreWriteEncoded(t *testing.T) {
	w, parent := freshParent(t)
	mustSucceed(t, runBin(t, w, nil, "index"))
	for stem, desc := range seedDescriptions {
		enc, err := memory.Encoder{}.EncodeNew(memory.NewNode{Description: desc})
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(parent, "memory", "notes", stem, stem+".md")
		got, err := os.ReadFile(p) //nolint:gosec // a file this test's call created
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, enc.Bytes) {
			t.Errorf("%s:\n%q\nwant\n%q", stem, got, enc.Bytes)
		}
		modeIs(t, p, 0o644)
	}
	modeIs(t, filepath.Join(parent, "memory"), 0o755|fs.ModeDir)
	modeIs(t, filepath.Join(parent, "memory", "notes"), 0o755|fs.ModeDir)
}

func modeIs(t *testing.T, p string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != want {
		t.Errorf("%s has mode %v, want %v", p, info.Mode(), want)
	}
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
	readsRoot(t, runBin(t, w, nil, "--parent-dir", link, "read"))
	isStore(t, real)
	readsRoot(t, runBin(t, w, map[string]string{"MEMORY_PARENT_DIR": "link"}, "read"))
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the parent symlink was replaced: %v %v", info, err)
	}
}

// Acceptance criterion 3a: the trigger is notes/, not memory/. Entries
// already in memory/ are left as they are.
func TestSeedsBesideExistingMemoryEntries(t *testing.T) {
	w, parent := freshParent(t)
	keep := filepath.Join(parent, "memory", "keep.txt")
	writeFile(t, keep, "operator's file\n")
	ageTree(t, parent)
	before := snapshot(t, keep)

	readsRoot(t, runBin(t, w, nil, "read"))
	sameSnapshot(t, before, snapshot(t, keep))
	check(t, "the parent", treeUnder(t, parent), withDirs("", append(seedFiles(), "memory/keep.txt")))
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

// Acceptance criterion 3d: deleting notes/ resets the store; the next call
// seeds it again.
func TestDeletedNotesIsSeededAgain(t *testing.T) {
	w, notes := seeded(t)
	writeNode(t, notes, "topics/kettle", "---\ndescription: Kettle\n---\n")
	if err := os.RemoveAll(notes); err != nil {
		t.Fatal(err)
	}
	readsRoot(t, runBin(t, w, nil, "read"))
	check(t, "the parent after a reset", treeUnder(t, filepath.Join(w, ".agent")), withDirs("", seedFiles()))
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

// Acceptance criterion 5: concurrent first calls on one fresh store all
// succeed with the same output, and leave exactly the seeds: one wins the
// seeding, the others use the winner's notes/.
func TestConcurrentFirstCalls(t *testing.T) {
	const calls = 8
	for round := range 10 {
		w, parent := freshParent(t)
		cmds := make([]*exec.Cmd, calls)
		outs := make([]*bytes.Buffer, calls)
		errs := make([]*bytes.Buffer, calls)
		for i := range cmds {
			cmd := exec.Command(binPath, "read") //nolint:gosec // the binary this package builds
			cmd.Dir = w
			cmd.Env = append(os.Environ(), "TZ=UTC", "MEMORY_PARENT_DIR="+parent, "HOME="+filepath.Join(w, "home"))
			outs[i], errs[i] = &bytes.Buffer{}, &bytes.Buffer{}
			cmd.Stdout, cmd.Stderr = outs[i], errs[i]
			cmds[i] = cmd
		}
		for _, cmd := range cmds {
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
		}
		for i, cmd := range cmds {
			err := cmd.Wait()
			r := result{stdout: outs[i].String(), stderr: errs[i].String()}
			if err != nil {
				r.code = cmd.ProcessState.ExitCode()
			}
			readsRoot(t, r)
			if r.stdout != outs[0].String() {
				t.Errorf("round %d: call %d printed %q, call 0 %q", round, i, r.stdout, outs[0].String())
			}
		}
		check(t, "the parent after concurrent first calls", treeUnder(t, parent), withDirs("", seedFiles()))
		if t.Failed() {
			t.Fatalf("round %d failed", round)
		}
	}
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
