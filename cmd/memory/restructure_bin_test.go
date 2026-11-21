package main

// CLI11: memory delete and memory retag, through the built binary
// (docs/spec/agent-memory.md Part III.8, "delete, exactly" and "retag,
// exactly").

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// xDelete and xRetag are the results as a reader sees them, declared
// independently of the production types.
type xDelete struct {
	XMLName  xml.Name
	Stem     string      `xml:"stem,attr"`
	Block    string      `xml:"block,attr"`
	Nodes    string      `xml:"nodes,attr"`
	Blocks   string      `xml:"blocks,attr"`
	Dangling []xDangling `xml:"dangling"`
}

type xDangling struct {
	Stem  string `xml:"stem,attr"`
	Block string `xml:"block,attr"`
	Link  string `xml:",chardata"`
}

// storeF is the unit's fixture store F: the shipped starting tree plus five
// nodes made with write, and the block IDs write assigned.
type storeF struct {
	dir, notes, state      string
	a, b, p1, p2, t1, r1   string
	m1, m2                 string
	outbox, oldPlan, priya string // folder note paths
	oldTask, misc          string
}

const (
	textA  = "Urgent mail goes to Priya #escalations first."
	textB  = "See [[old-plan]] for the rota."
	textP1 = "Rota starts 2026-12-01."
	textP2 = "Agreed with Priya."
	textT1 = "Draft the rota. See [[old-plan]]."
	textM1 = "Keep this #misc note."
	textM2 = "See [[nowhere]]."
)

// add runs write in f's store and returns the IDs it assigned.
func (f *storeF) add(t *testing.T, args ...string) []string {
	t.Helper()
	return decodeWrite(t, runBin(t, f.dir, nil, append([]string{"write"}, args...)...)).Assigned
}

func newStoreF(t *testing.T) *storeF {
	t.Helper()
	dir, notes := seeded(t)
	f := &storeF{dir: dir, notes: notes, state: filepath.Join(dir, ".agent")}
	ids := f.add(t, "outbox-etiquette", "--parent", "topics", "--description", "How and when to cc people",
		"--tag", "escalations", "--tag", "project/atlas", "--add", textA, "--add", textB)
	f.a, f.b = ids[0], ids[1]
	ids = f.add(t, "old-plan", "--parent", "projects", "--description", "The old rota plan",
		"--tag", "escalation", "--tag", "escalations", "--tag", "projects", "--add", textP1, "--add", textP2)
	f.p1, f.p2 = ids[0], ids[1]
	f.t1 = f.add(t, "old-task", "--parent", "old-plan", "--description", "Drafting the rota", "--add", textT1)[0]
	f.r1 = f.add(t, "priya", "--parent", "people", "--description", "Priya, who runs the rota",
		"--add", "Owns [[old-task#^"+f.t1+"]] and [[old-plan]]. #project/atlas")[0]
	ids = f.add(t, "misc-note", "--parent", "topics", "--description", "Odds and ends",
		"--tag", "misc", "--add", textM1, "--add", textM2)
	f.m1, f.m2 = ids[0], ids[1]
	f.outbox = filepath.Join(notes, "topics", "outbox-etiquette", "outbox-etiquette.md")
	f.oldPlan = filepath.Join(notes, "projects", "old-plan", "old-plan.md")
	f.oldTask = filepath.Join(notes, "projects", "old-plan", "old-task", "old-task.md")
	f.priya = filepath.Join(notes, "people", "priya", "priya.md")
	f.misc = filepath.Join(notes, "topics", "misc-note", "misc-note.md")
	return f
}

func readText(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p) //nolint:gosec // a test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// callStart is the earliest value a call starting now can store as
// modified: the current second, UTC (III.9).
func callStart() time.Time { return time.Now().UTC().Truncate(time.Second) }

// redatedBy is want, the bytes a note should hold, with its one modified
// line carrying the value the note at p holds now. It first checks that
// value is the time of a call made between start and now, so a note the
// call was to date (III.9) but did not, or dated with anything else, fails.
// A test dates the note in the past first (dateNote), so an unchanged line
// cannot pass.
func redatedBy(t *testing.T, what, p, want string, start time.Time) string {
	t.Helper()
	value := modifiedOf(t, p)
	wantNow(t, what, value, start, time.Now().UTC())
	if n := len(modifiedLineRE.FindAllString(want, -1)); n != 1 {
		t.Fatalf("%s: the expected note holds %d modified lines, want 1:\n%s", what, n, want)
	}
	return modifiedLineRE.ReplaceAllLiteralString(want, "modified: "+value+"\n")
}

// lintOf is lint --exit-zero's report on f's store.
func (f *storeF) lintOf(t *testing.T) xLint {
	t.Helper()
	r := runBin(t, f.dir, nil, "lint", "--exit-zero")
	mustSucceed(t, r)
	var x xLint
	decode(t, r.stdout, &x)
	return x
}

func lintErrors(x xLint) map[xProblem]int {
	out := map[xProblem]int{}
	for _, p := range x.Problems {
		if p.Severity == "error" {
			out[p]++
		}
	}
	return out
}

// ok runs a call that must succeed (exit 0, stderr empty) and, as criterion
// 8 requires of every successful call, must leave no lint error that was
// not there before it.
func (f *storeF) ok(t *testing.T, args ...string) result {
	t.Helper()
	before := lintErrors(f.lintOf(t))
	r := runBin(t, f.dir, nil, args...)
	mustSucceed(t, r)
	for p, n := range lintErrors(f.lintOf(t)) {
		if n > before[p] {
			t.Errorf("%v added the lint error %s: %s", args, p.Path, p.Reason)
		}
	}
	return r
}

// refused runs a call that must fail with exit 1, nothing on stdout and
// one line on stderr, and change nothing under the state dir: no byte, no
// mtime, no entry. It returns the line.
func (f *storeF) refused(t *testing.T, args ...string) string {
	t.Helper()
	ageTree(t, f.state)
	before := snapshot(t, f.state)
	line := mustFail(t, runBin(t, f.dir, nil, args...))
	sameSnapshot(t, before, snapshot(t, f.state))
	return line
}

// sameFiles fails unless every regular file in before, other than those in
// changed, is in after with the same bytes and mtime, and after holds no
// file before did not (removed lists files that may be gone).
func sameFiles(t *testing.T, before, after map[string]fileState, changed ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, p := range changed {
		skip[p] = true
	}
	for p, b := range before {
		if skip[p] || !b.mode.IsRegular() {
			continue
		}
		if a, ok := after[p]; !ok || !untouched(b, a) {
			t.Errorf("%s was removed or rewritten (still there: %v, mtime %v -> %v)", p, ok, b.mtime, a.mtime)
		}
	}
	for p := range after {
		if _, was := before[p]; !was && !skip[p] {
			t.Errorf("%s appeared", p)
		}
	}
}

// untouched reports whether a file has the same bytes and mtime.
func untouched(before, after fileState) bool {
	return bytes.Equal(after.data, before.data) && after.mtime.Equal(before.mtime)
}

func mustContain(t *testing.T, what, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%s: %q does not mention %q", what, s, w)
		}
	}
}

func decodeDelete(t *testing.T, r result) xDelete {
	t.Helper()
	var x xDelete
	decode(t, r.stdout, &x)
	check(t, "result element", x.XMLName.Local, "memory_delete")
	return x
}

// noHiddenLeftovers fails if anything under notes/ but the lock file has a
// dot name: a folder renamed aside and not removed, or a temporary file.
func noHiddenLeftovers(t *testing.T, notes string) {
	t.Helper()
	for p := range snapshot(t, notes) {
		if base := filepath.Base(p); strings.HasPrefix(base, ".") && base != ".write.lock" {
			t.Errorf("hidden entry left behind: %s", p)
		}
	}
}

// Acceptance criterion 1: a block is deleted and nothing else changes, but
// for the note's modified line, which each delete sets to the time of its
// call (III.9).
func TestDeleteBlock(t *testing.T) {
	t.Parallel()
	f := newStoreF(t)
	dateNote(t, f.outbox, dfDate)
	ageTree(t, f.state)
	before := snapshot(t, f.state)
	old := readText(t, f.outbox)

	start := callStart()
	r := f.ok(t, "delete", "outbox-etiquette", "--block", f.a)
	check(t, "stdout", r.stdout,
		`<memory_delete stem="outbox-etiquette" block="`+f.a+`" nodes="0" blocks="1"></memory_delete>`+"\n")

	paragraph := textA + " ^" + f.a + "\n\n"
	if !strings.Contains(old, paragraph) {
		t.Fatalf("fixture: %q is not in the note:\n%s", paragraph, old)
	}
	afterA := redatedBy(t, "outbox-etiquette.md after deleting <A>", f.outbox, strings.Replace(old, paragraph, "", 1), start)
	check(t, "outbox-etiquette.md after deleting <A>", readText(t, f.outbox), afterA)
	mustContain(t, "<B> keeps its ID and bytes", afterA, textB+" ^"+f.b+"\n")
	sameFiles(t, before, snapshot(t, f.state), f.outbox)
	noHiddenLeftovers(t, f.notes)

	// Deleting the last block leaves an empty node, not a deleted one.
	dateNote(t, f.outbox, "2026-02-01T09:00:00Z")
	start = callStart()
	r = f.ok(t, "delete", "outbox-etiquette", "--block", f.b)
	check(t, "stdout", r.stdout,
		`<memory_delete stem="outbox-etiquette" block="`+f.b+`" nodes="0" blocks="1"></memory_delete>`+"\n")
	empty := readText(t, f.outbox)
	check(t, "outbox-etiquette.md with no blocks", empty,
		redatedBy(t, "outbox-etiquette.md with no blocks", f.outbox, strings.Replace(afterA, "\n"+textB+" ^"+f.b+"\n", "", 1), start))
	if !strings.HasSuffix(empty, "---\n") {
		t.Errorf("the emptied note does not end at its frontmatter:\n%s", empty)
	}
	n := readNode(t, f.dir, "outbox-etiquette")
	check(t, "read after emptying", attrsOf(n), attrs{"outbox-etiquette", "topics/outbox-etiquette", "topics", "escalations,project/atlas", "How and when to cc people"})
	check(t, "blocks after emptying", len(n.Blocks), 0)
	lint := f.lintOf(t)
	check(t, "lint errors", lint.Errors, "0")
	for _, p := range lint.Problems {
		if p.Path == "topics/outbox-etiquette" {
			t.Errorf("lint reports the emptied node: %s", p.Reason)
		}
	}
}

// Acceptance criterion 2: a block delete reports the links it broke,
// including those from the same node, and rewrites none of them.
func TestDeleteBlockReportsDangling(t *testing.T) {
	t.Parallel()
	f := newStoreF(t)
	linkA := "[[outbox-etiquette#^" + f.a + "]]"
	c := f.add(t, "outbox-etiquette", "--add", "As "+linkA+" says.")[0]
	r2 := f.add(t, "priya", "--add", "Per "+linkA+".")[0]
	lintBefore := f.lintOf(t)
	ageTree(t, f.state)
	before := snapshot(t, f.state)

	x := decodeDelete(t, f.ok(t, "delete", "outbox-etiquette", "--block", f.a))
	check(t, "result", [...]string{x.Stem, x.Block, x.Nodes, x.Blocks}, [...]string{"outbox-etiquette", f.a, "0", "1"})
	check(t, "dangling", x.Dangling, []xDangling{
		{Stem: "outbox-etiquette", Block: c, Link: linkA},
		{Stem: "priya", Block: r2, Link: linkA},
	})

	// The links are reported, never edited.
	mustContain(t, "<C> on disk", readText(t, f.outbox), "As "+linkA+" says. ^"+c+"\n")
	sameFiles(t, before, snapshot(t, f.state), f.outbox)
	check(t, "priya.md", readText(t, f.priya), string(before[f.priya].data))

	lintAfter := f.lintOf(t)
	check(t, "lint errors after", lintAfter.Errors, "0")
	check(t, "warnings added", len(lintAfter.Problems)-len(lintBefore.Problems), 2)
	warnsBefore, err := strconv.Atoi(lintBefore.Warnings)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "lint warnings after", lintAfter.Warnings, strconv.Itoa(warnsBefore+2))
}

// Acceptance criterion 3: a plain delete never removes what the caller
// cannot see, and says what is in the way.
func TestDeleteRefusesHiddenContent(t *testing.T) {
	t.Parallel()
	t.Run("a child node", func(t *testing.T) {
		t.Parallel()
		f := newStoreF(t)
		line := f.refused(t, "delete", "old-plan")
		mustContain(t, "refusal", line, "old-plan", "1 child", "--recursive")
		if strings.Contains(line, "1 children") {
			t.Errorf("refusal says %q", line)
		}

		// Second-order: with the child gone, the same call succeeds.
		x := decodeDelete(t, f.ok(t, "delete", "old-task"))
		check(t, "delete old-task", [...]string{x.Stem, x.Block, x.Nodes, x.Blocks}, [...]string{"old-task", "", "1", "1"})
		if exists(filepath.Dir(f.oldTask)) {
			t.Error("old-task's folder is still there")
		}
		x = decodeDelete(t, f.ok(t, "delete", "old-plan"))
		check(t, "delete old-plan", [...]string{x.Stem, x.Nodes, x.Blocks}, [...]string{"old-plan", "1", "2"})
		if exists(filepath.Dir(f.oldPlan)) {
			t.Error("old-plan's folder is still there")
		}
		noHiddenLeftovers(t, f.notes)
	})
	t.Run("a stray file", func(t *testing.T) {
		t.Parallel()
		f := newStoreF(t)
		writeFile(t, filepath.Join(filepath.Dir(f.misc), "notes.txt"), "kept by hand\n")
		mustContain(t, "refusal", f.refused(t, "delete", "misc-note"), "misc-note", "notes.txt", "--recursive")
	})
	t.Run("a child folder that did not load", func(t *testing.T) {
		t.Parallel()
		f := newStoreF(t)
		writeFile(t, filepath.Join(filepath.Dir(f.misc), "broken", "broken.md"), "no frontmatter here\n")
		if _, loaded := memory.Load(f.notes).Node("broken"); loaded {
			t.Fatal("fixture: broken loaded")
		}
		line := f.refused(t, "delete", "misc-note")
		mustContain(t, "refusal", line, "misc-note", "broken", "--recursive")
	})
	t.Run("only a dot-entry", func(t *testing.T) {
		t.Parallel()
		f := newStoreF(t)
		writeFile(t, filepath.Join(filepath.Dir(f.misc), ".DS_Store"), "\x00\x01")
		before := snapshot(t, f.state)
		r := f.ok(t, "delete", "misc-note")
		check(t, "stdout", r.stdout, `<memory_delete stem="misc-note" nodes="1" blocks="2"></memory_delete>`+"\n")
		if exists(filepath.Dir(f.misc)) {
			t.Error("misc-note's folder is still there")
		}
		noHiddenLeftovers(t, f.notes)
		after := snapshot(t, f.state)
		for p := range before {
			if _, still := after[p]; !still && !strings.HasPrefix(p, filepath.Dir(f.misc)) {
				t.Errorf("%s was removed with misc-note", p)
			}
		}
	})
}

// Acceptance criterion 4: --recursive removes the subtree and reports
// every link it broke, once each, in order.
func TestDeleteRecursive(t *testing.T) {
	t.Parallel()
	f := newStoreF(t)
	ageTree(t, f.state)
	before := snapshot(t, f.state)

	r := f.ok(t, "delete", "old-plan", "--recursive")
	if !strings.HasPrefix(r.stdout, `<memory_delete stem="old-plan" nodes="2" blocks="3">`+"\n") {
		t.Errorf("stdout starts %q", r.stdout)
	}
	x := decodeDelete(t, r)
	check(t, "dangling", x.Dangling, []xDangling{
		{Stem: "outbox-etiquette", Block: f.b, Link: "[[old-plan]]"},
		{Stem: "priya", Block: f.r1, Link: "[[old-task#^" + f.t1 + "]]"},
		{Stem: "priya", Block: f.r1, Link: "[[old-plan]]"},
	})

	if exists(filepath.Join(f.notes, "projects", "old-plan")) {
		t.Error("notes/projects/old-plan is still there")
	}
	noHiddenLeftovers(t, f.notes)
	after := snapshot(t, f.state)
	gone := filepath.Join(f.notes, "projects", "old-plan")
	for p, b := range before {
		if strings.HasPrefix(p, gone) || !b.mode.IsRegular() {
			continue
		}
		if a, ok := after[p]; !ok || !untouched(b, a) {
			t.Errorf("%s was changed by the delete", p)
		}
	}
	for _, stem := range []string{"old-plan", "old-task"} {
		line := mustFail(t, runBin(t, f.dir, nil, "read", stem))
		mustContain(t, "read "+stem, line, "STEM", stem, "no such node")
	}
	check(t, "lint errors after", f.lintOf(t).Errors, "0")
}

// Acceptance criterion 5: bad calls change nothing, exit 1, and name the
// argument as typed.
func TestDeleteRetagBadCalls(t *testing.T) {
	t.Parallel()
	f := newStoreF(t)
	cases := []struct {
		args  []string
		wants []string
	}{
		{[]string{"delete"}, []string{"STEM"}},
		{[]string{"delete", "/"}, []string{"STEM", "root"}},
		{[]string{"delete", "old-plann"}, []string{"STEM", `"old-plann"`, "near matches: old-plan"}},
		{[]string{"delete", "priya", "--block", "m-zzzzzz"}, []string{"--block", "m-zzzzzz", "priya"}},
		{[]string{"delete", "priya", "--block", f.a}, []string{"--block", f.a, "priya"}},
		{[]string{"delete", "old-plan", "--block", f.p1, "--recursive"}, []string{"--recursive"}},
		{[]string{"delete", "old-plan", "-recursive", "-block", f.p1}, []string{"-recursive"}},
		{[]string{"delete", "old-plan", "old-task"}, []string{"STEM"}},
		{[]string{"delete", "old-plan", "--force"}, []string{"--force"}},
		{[]string{"delete", "priya", "--block", ""}, []string{"--block"}},
		{[]string{"retag"}, []string{"OLD"}},
		{[]string{"retag", "bad tag", "x"}, []string{"OLD", "bad tag"}},
		{[]string{"retag", "misc", "bad tag"}, []string{"NEW", "bad tag"}},
		{[]string{"retag", "misc", ""}, []string{"NEW"}},
		{[]string{"retag", "a", "b", "c"}, []string{"retag", `"c"`}},
		{[]string{"retag", "misc", "--recursive"}, []string{"--recursive"}},
	}
	for _, c := range cases {
		line := f.refused(t, c.args...)
		mustContain(t, strings.Join(c.args, " "), line, c.wants...)
		for _, internal := range []string{"stem:", "block:", "recursive:", "old:", "new:", "blocks["} {
			if strings.HasPrefix(line, internal) {
				t.Errorf("%v: error names the internal field: %q", c.args, line)
			}
		}
	}
	if line := f.refused(t, "delete", "priya", "--block", f.a); strings.Contains(line, "near matches") {
		t.Errorf("a block of another node is reported as an unknown stem: %q", line)
	}

	// No store yet: the stem is unknown, and the empty store is created
	// (III.1).
	w, parent := freshParent(t)
	line := mustFail(t, runBin(t, w, nil, "delete", "x"))
	mustContain(t, "delete on a missing store", line, "STEM", `"x"`, "no such node")
	notes := filepath.Join(parent, "memory", "notes")
	if info, err := os.Stat(notes); err != nil || !info.IsDir() {
		t.Fatalf("memory/notes/ was not created: %v", err)
	}
	check(t, "nodes in the created store", len(memory.Load(notes).Nodes()), 0)
	line = mustFail(t, runBin(t, w, nil, "retag", "bad tag"))
	mustContain(t, "retag with a bad tag", line, "OLD")
}

// An argument error comes before the store is created (III.1).
func TestDeleteRetagArgumentErrorsCreateNothing(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"delete"}, {"delete", "/"}, {"delete", "x", "--block", "m-aaaaaa", "--recursive"},
		{"retag"}, {"retag", "bad tag"}, {"retag", "a", "bad tag"}, {"retag", "a", "b", "c"},
	} {
		w, parent := freshParent(t)
		mustFail(t, runBin(t, w, nil, args...))
		if exists(filepath.Join(parent, "memory")) {
			t.Errorf("%v created the store", args)
		}
	}
}

func tagsOf(t *testing.T, notes, stem string) []string {
	t.Helper()
	return append([]string{}, loadNode(t, notes, stem).Tags...)
}

func blockText(t *testing.T, notes, stem, id string) string {
	t.Helper()
	b, ok := memory.Load(notes).Block(stem, id)
	if !ok {
		t.Fatalf("no block %s in %s", id, stem)
	}
	return b.Text
}

// Acceptance criterion 6: retag renames in frontmatter and in block text,
// by whole path segment, and writes only the notes it changes.
func TestRetagRenames(t *testing.T) {
	t.Parallel()
	f := newStoreF(t)
	ageTree(t, f.state)
	before := snapshot(t, f.state)
	oldOutbox, oldPlan := readText(t, f.outbox), readText(t, f.oldPlan)

	r := f.ok(t, "retag", "escalations", "escalation")
	check(t, "stdout", r.stdout, `<memory_retag from="escalations" tag="escalation" nodes="2"></memory_retag>`+"\n")

	check(t, "outbox-etiquette tags", tagsOf(t, f.notes, "outbox-etiquette"), []string{"escalation", "project/atlas"})
	check(t, "<A>", blockText(t, f.notes, "outbox-etiquette", f.a), "Urgent mail goes to Priya #escalation first.")
	check(t, "old-plan tags", tagsOf(t, f.notes, "old-plan"), []string{"escalation", "projects"})

	// Every byte outside the tag tokens is unchanged. The fixture's
	// description does not hold the word, so its two occurrences are the
	// frontmatter tag and the inline tag.
	check(t, "occurrences of the tag in outbox-etiquette.md", strings.Count(oldOutbox, "escalations"), 2)
	check(t, "outbox-etiquette.md", readText(t, f.outbox), strings.ReplaceAll(oldOutbox, "escalations", "escalation"))
	check(t, "old-plan.md", readText(t, f.oldPlan), strings.Replace(oldPlan, "tags: [escalation, escalations, projects]", "tags: [escalation, projects]", 1))
	if readText(t, f.oldPlan) == oldPlan {
		t.Error("old-plan.md was not rewritten")
	}
	sameFiles(t, before, snapshot(t, f.state), f.outbox, f.oldPlan)
	noHiddenLeftovers(t, f.notes)

	s, _ := runSearch(t, f.dir, "tag:escalations")
	check(t, "search tag:escalations", s.Matched, "0")
	mustContain(t, "index", runIndex(t, f.dir), "escalation(2)")

	// A parent tag takes its nested tags along; projects is another tag.
	ageTree(t, f.state)
	before = snapshot(t, f.state)
	r = f.ok(t, "retag", "#project", "proj")
	check(t, "stdout", r.stdout, `<memory_retag from="project" tag="proj" nodes="2"></memory_retag>`+"\n")
	check(t, "outbox-etiquette tags", tagsOf(t, f.notes, "outbox-etiquette"), []string{"escalation", "proj/atlas"})
	if text := blockText(t, f.notes, "priya", f.r1); !strings.HasSuffix(text, " and [[old-plan]]. #proj/atlas") {
		t.Errorf("<R1> is %q", text)
	}
	check(t, "old-plan tags", tagsOf(t, f.notes, "old-plan"), []string{"escalation", "projects"})
	sameFiles(t, before, snapshot(t, f.state), f.outbox, f.priya)
}

// Acceptance criterion 7: retag OLD removes the tag, or refuses without
// writing anything; a tag nobody has, and NEW equal to OLD, write nothing.
func TestRetagRemoves(t *testing.T) {
	t.Parallel()
	f := newStoreF(t)
	ageTree(t, f.state)
	before := snapshot(t, f.state)
	oldMisc := readText(t, f.misc)

	r := f.ok(t, "retag", "misc")
	check(t, "stdout", r.stdout, `<memory_retag from="misc" nodes="1"></memory_retag>`+"\n")
	check(t, "misc-note tags", len(tagsOf(t, f.notes, "misc-note")), 0)
	check(t, "<M1>", blockText(t, f.notes, "misc-note", f.m1), "Keep this note.")
	mustContain(t, "misc-note.md", readText(t, f.misc), "\nKeep this note. ^"+f.m1+"\n\n"+textM2+" ^"+f.m2+"\n")
	check(t, "the body outside the tag", strings.Replace(bodyOf(oldMisc), " #misc", "", 1), bodyOf(readText(t, f.misc)))
	sameFiles(t, before, snapshot(t, f.state), f.misc)

	// A block that is only the tag cannot be emptied: nothing is written,
	// not even to the node that sorts before it.
	m3 := f.add(t, "misc-note", "--add", "#gone")[0]
	f.add(t, "priya", "--tag", "gone")
	line := f.refused(t, "retag", "gone")
	mustContain(t, "refusal", line, m3, "misc-note", "delete the block")
	check(t, "priya tags", tagsOf(t, f.notes, "priya"), []string{"gone"})
	check(t, "<M3>", blockText(t, f.notes, "misc-note", m3), "#gone")

	for _, args := range [][]string{{"retag", "nobody"}, {"retag", "escalation", "escalation"}, {"retag", "nobody", "somebody"}} {
		ageTree(t, f.state)
		before := snapshot(t, f.state)
		r := f.ok(t, args...)
		x := struct {
			XMLName xml.Name
			From    string `xml:"from,attr"`
			Nodes   string `xml:"nodes,attr"`
		}{}
		decode(t, r.stdout, &x)
		check(t, strings.Join(args, " "), [...]string{x.XMLName.Local, x.From, x.Nodes}, [...]string{"memory_retag", args[1], "0"})
		sameSnapshot(t, before, snapshot(t, f.state))
	}
}

// bodyOf is a note after its frontmatter.
func bodyOf(note string) string {
	_, body, _ := strings.Cut(strings.TrimPrefix(note, "---\n"), "\n---\n")
	return body
}

// Acceptance criterion 8: the usage text lists both verbs.
func TestUsageListsDeleteAndRetag(t *testing.T) {
	t.Parallel()
	r := runBin(t, t.TempDir(), nil)
	if r.code != 1 || r.stdout != "" {
		t.Fatalf("no arguments: exit %d, stdout %q", r.code, r.stdout)
	}
	mustContain(t, "usage", r.stderr, "delete STEM [--block ID] [--recursive]", "retag OLD [NEW]", "dangling")
}

// The README's Usage section documents both verbs, and the status line
// lists them.
func TestReadmeDocumentsDeleteAndRetag(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	usage := readmeSection(t, readme, "Usage")
	mustContain(t, "the Usage section", usage,
		"memory delete ", "--block", "--recursive", "<memory_delete", "<dangling",
		"memory retag ", "<memory_retag")
	status, _, _ := strings.Cut(readme[strings.Index(readme, "**Status:**"):], "\n")
	mustContain(t, "the status line", status, "`delete`", "`retag`")
	if strings.Contains(readme, "No verb moves, renames or deletes a node") {
		t.Error("the README still says no verb deletes a node")
	}
}
