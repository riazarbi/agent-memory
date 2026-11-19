package main

// CLI10: every verb, once its arguments pass, creates the empty
// memory/notes/ of a missing store, and index with no loadable node prints
// the trust line and <empty>No nodes yet.</empty> (docs/spec/agent-memory.md
// Part III.1, III.2, III.7).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The empty-store results (III.1), stated independently of production.
const (
	emptyIndexOut = "<memory_index>" + trustLine + "\n  <empty>No nodes yet.</empty>\n</memory_index>\n"
	emptyRootOut  = "<memory></memory>\n"
	emptyLintOut  = "<memory_lint nodes=\"0\" errors=\"0\" warnings=\"0\"></memory_lint>\n"
)

// emptySearchOut is search's result for query on an empty store.
func emptySearchOut(query string) string {
	return `<memory_search query="` + query + `" matched="0" shown="0"></memory_search>` + "\n"
}

// firstCalls is every successful first call on a missing store, with its
// empty-store result.
var firstCalls = []struct {
	args []string
	out  string
}{
	{[]string{"read"}, emptyRootOut},
	{[]string{"read", "/"}, emptyRootOut},
	{[]string{"index"}, emptyIndexOut},
	{[]string{"search", "anything"}, emptySearchOut("anything")},
	{[]string{"lint"}, emptyLintOut},
	{[]string{"lint", "--exit-zero"}, emptyLintOut},
}

// readVerbs is one valid call of each verb that only reads.
var readVerbs = [][]string{{"read"}, {"read", "/"}, {"index"}, {"search", "anything"}, {"lint"}, {"lint", "--exit-zero"}}

// isEmptyStore fails unless parent holds exactly memory/ and memory/notes/
// (no lock file, no node), with the given modes.
func isEmptyStore(t *testing.T, what, parent string, memoryMode, notesMode os.FileMode) {
	t.Helper()
	check(t, what+": the parent", treeUnder(t, parent), emptyStoreTree)
	check(t, what+": memory/ mode", fileMode(t, filepath.Join(parent, "memory")), memoryMode)
	check(t, what+": notes/ mode", fileMode(t, filepath.Join(parent, "memory", "notes")), notesMode)
}

// Acceptance criterion 1: index on a missing store, an empty notes/, and a
// notes/ whose only folder is a node that does not load prints exactly the
// trust line and <empty>, and no <tags>; a store with nodes has no <empty>.
func TestEmptyIndexIsSaid(t *testing.T) {
	w, _ := freshParent(t)
	check(t, "index on a missing store", runIndex(t, w), emptyIndexOut)
	// Now notes/ exists and is empty.
	check(t, "index on an empty notes/", runIndex(t, w), emptyIndexOut)

	w, parent := freshParent(t)
	notes := filepath.Join(parent, "memory", "notes")
	// Tagged, but it does not load (a heading in its body), so its tag is
	// not in the store either.
	writeNode(t, notes, "broken", "---\ndescription: Broken\ntags: [kettle]\n---\n\n# Heading\n")
	out := runIndex(t, w)
	check(t, "index on a notes/ with only a broken node", out, emptyIndexOut)
	if strings.Contains(out, "<tags") {
		t.Errorf("the empty index renders <tags>:\n%s", out)
	}

	w, _ = seeded(t)
	out = runIndex(t, w)
	if strings.Contains(out, "<empty") {
		t.Errorf("a store with nodes renders <empty>:\n%s", out)
	}
	if !strings.HasPrefix(out, "<memory_index>"+trustLine+"\n  <node stem=\"people\"") {
		t.Errorf("the index of the starting tree does not start with the trust line and its first node:\n%s", out)
	}
}

// Acceptance criterion 2: each read verb, as the first call on a missing
// store (no memory/, or memory/ with no notes/), exits 0 with its
// empty-store result and leaves exactly memory/ and memory/notes/, both
// 0755 under umask 077, and no lock file; a second call changes nothing.
// The mode is forced, so a permissive umask (002) gives 0755 too, not 0775.
func TestEveryVerbCreatesAMissingStore(t *testing.T) {
	for _, mask := range []int{0o077, 0o002} {
		for _, call := range firstCalls {
			for _, withMemory := range []bool{false, true} {
				what := fmt.Sprintf("%s (umask %03o, memory/ exists: %v)", strings.Join(call.args, " "), mask, withMemory)
				w, parent := freshParent(t)
				if withMemory {
					mem := mkdirs(t, filepath.Join(parent, "memory"))[0]
					if err := os.Chmod(mem, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				r := runBinUmask(t, w, mask, call.args...)
				mustSucceed(t, r)
				check(t, what+": stdout", r.stdout, call.out)
				isEmptyStore(t, what, parent, 0o755, 0o755)

				ageTree(t, parent)
				before := snapshot(t, parent)
				again := runBinUmask(t, w, mask, call.args...)
				mustSucceed(t, again)
				check(t, what+": second stdout", again.stdout, call.out)
				sameSnapshot(t, before, snapshot(t, parent))
			}
		}
	}
}

// Acceptance criterion 2: read of an unknown stem on a missing store is the
// unknown-stem error, and still leaves the empty store; again, it changes
// nothing.
func TestUnknownStemCreatesTheEmptyStore(t *testing.T) {
	w, parent := freshParent(t)
	line := mustFail(t, runBinUmask(t, w, 0o077, "read", "kettle"))
	check(t, "read kettle", line, `STEM "kettle": no such node`)
	isEmptyStore(t, "read kettle", parent, 0o755, 0o755)

	ageTree(t, parent)
	before := snapshot(t, parent)
	check(t, "read kettle again", mustFail(t, runBin(t, w, nil, "read", "kettle")), `STEM "kettle": no such node`)
	sameSnapshot(t, before, snapshot(t, parent))
}

// Acceptance criterion 2: an existing memory/ keeps its own mode when a
// read verb creates notes/ inside it (III.1: forced only on what is
// created).
func TestExistingMemoryDirKeepsItsMode(t *testing.T) {
	for _, args := range readVerbs {
		w, parent := freshParent(t)
		mem := mkdirs(t, filepath.Join(parent, "memory"))[0]
		if err := os.Chmod(mem, 0o700); err != nil {
			t.Fatal(err)
		}
		mustSucceed(t, runBinUmask(t, w, 0o077, args...))
		isEmptyStore(t, strings.Join(args, " "), parent, 0o700, 0o755)
	}
}

// Acceptance criterion 2: once notes/ exists, the read verbs write
// nothing, on a store with nodes too.
func TestReadVerbsWriteNothingOnAStore(t *testing.T) {
	w, _ := seeded(t)
	state := filepath.Join(w, ".agent")
	ageTree(t, state)
	before := snapshot(t, state)
	for _, args := range append(readVerbs, []string{"read", "topics"}) {
		mustSucceed(t, runBin(t, w, nil, args...))
	}
	sameSnapshot(t, before, snapshot(t, state))
}

// Acceptance criterion 3: an argument error leaves the parent empty, and
// leaves an existing memory/ without notes/.
func TestArgumentErrorsCreateNoStore(t *testing.T) {
	for _, args := range [][]string{
		{"read", "--depth", "x"},
		{"read", "--bogus"},
		{"read", "a", "b"},
		{"read", "--block", "m-aaaaaa"},
		{"index", "extra"},
		{"index", "--bogus"},
		{"search", "a", "b"},
		{"lint", "extra"},
		{"lint", "--bogus"},
		{"lint", "--exit-zero=true"},
		{"lint", "--exit-zero", "extra"},
	} {
		what := strings.Join(args, " ")
		w, parent := freshParent(t)
		mustFail(t, runBin(t, w, nil, args...))
		isEmptyDir(t, what, parent)

		w, parent = freshParent(t)
		mkdirs(t, filepath.Join(parent, "memory"))
		mustFail(t, runBin(t, w, nil, args...))
		check(t, what+" with memory/", treeUnder(t, parent), []string{"memory/"})
	}
}

// Acceptance criterion 3: a missing parent, and memory/ or notes existing
// as something other than a directory (a dangling symlink included), fail
// with III.1's lines and create nothing, for every read verb.
func TestResolutionErrorsCreateNoStore(t *testing.T) {
	for _, args := range readVerbs {
		w := t.TempDir()
		nope := filepath.Join(w, "nope")
		failsExactly(t, w, nil, append([]string{"--parent-dir", nope}, args...),
			"parent dir "+nope+" does not exist (from --parent-dir)")
		failsExactly(t, w, map[string]string{"MEMORY_PARENT_DIR": "nope"}, args,
			"parent dir nope does not exist (from MEMORY_PARENT_DIR)")

		w, parent := freshParent(t)
		mem := filepath.Join(parent, "memory")
		writeFile(t, mem, "a file")
		failsExactly(t, w, nil, args, mem+" is not a directory")

		w, parent = freshParent(t)
		mem = filepath.Join(parent, "memory")
		if err := os.Symlink(filepath.Join(parent, "gone"), mem); err != nil {
			t.Fatal(err)
		}
		failsExactly(t, w, nil, args, mem+" is not a directory")

		w, parent = freshParent(t)
		notes := filepath.Join(parent, "memory", "notes")
		writeFile(t, notes, "a file")
		failsExactly(t, w, nil, args, notes+" is not a directory")

		w, parent = freshParent(t)
		notes = filepath.Join(mkdirs(t, filepath.Join(parent, "memory"))[0], "notes")
		if err := os.Symlink(filepath.Join(parent, "gone"), notes); err != nil {
			t.Fatal(err)
		}
		failsExactly(t, w, nil, args, notes+" is not a directory")
	}
}

// concurrentFirstCalls is AC4's eight mixed first calls, two of each.
var concurrentFirstCalls = [][]string{
	{"read"}, {"read"}, {"index"}, {"index"}, {"search", "x"}, {"search", "x"}, {"lint"}, {"lint"},
}

// Acceptance criterion 4: eight mixed first calls started together on one
// fresh parent all exit 0 with empty stderr, and leave exactly memory/ and
// memory/notes/. Repeated over fresh parents so a creator that loses the
// race (EEXIST) shows.
func TestConcurrentFirstCalls(t *testing.T) {
	for round := range 25 {
		w, parent := freshParent(t)
		results := make([]result, len(concurrentFirstCalls))
		errs := make([]error, len(concurrentFirstCalls))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, args := range concurrentFirstCalls {
			cmd := binCmd(w, nil, args...)
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results[i], errs[i] = execute(cmd)
			}()
		}
		close(start)
		wg.Wait()
		for i, r := range results {
			what := fmt.Sprintf("round %d: %s", round, strings.Join(concurrentFirstCalls[i], " "))
			if errs[i] != nil {
				t.Fatalf("%s: %v", what, errs[i])
			}
			if r.code != 0 || r.stderr != "" {
				t.Errorf("%s: exit %d, stderr %q", what, r.code, r.stderr)
			}
		}
		check(t, fmt.Sprintf("round %d: the parent", round), treeUnder(t, parent), emptyStoreTree)
		if t.Failed() {
			return
		}
	}
}
