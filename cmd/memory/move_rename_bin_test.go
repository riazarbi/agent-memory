package main

// CLI12: memory move and memory rename, through the built binary
// (docs/spec/agent-memory.md Part III.8, "move, exactly" and "rename,
// exactly").

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// xMove and xRename are the results as a reader sees them, declared
// independently of the production types.
type xMove struct {
	XMLName xml.Name
	Stem    string `xml:"stem,attr"`
	From    string `xml:"from,attr"`
	Parent  string `xml:"parent,attr"`
	Path    string `xml:"path,attr"`
}

type xRename struct {
	XMLName xml.Name
	From    string `xml:"from,attr"`
	Stem    string `xml:"stem,attr"`
	Links   string `xml:"links_rewritten,attr"`
}

// atlasF is the unit's fixture store F: the shipped starting tree plus
// four nodes made with write, and the block IDs write assigned.
type atlasF struct {
	storeF
	a1, a2, b1, p1, p2, n1 string
	// The block texts as written, by block.
	textA2, textB1, textP1, textP2 string
}

const (
	atlasA1 = "Cutover is 2026-12-01."
	atlasN1 = "Nothing about Atlas here."
)

func newAtlasF(t *testing.T) *atlasF {
	t.Helper()
	dir, notes := seeded(t)
	f := &atlasF{storeF: storeF{dir: dir, notes: notes, state: filepath.Join(dir, ".agent")}}
	f.a1 = f.add(t, "atlas-migration", "--parent", "people", "--description", "The Atlas migration",
		"--tag", "atlas", "--add", atlasA1)[0]
	f.textA2 = "Rollback plan is in [[atlas-migration#^" + f.a1 + "]] and [[atlas-runbook]]."
	f.a2 = f.add(t, "atlas-migration", "--add", f.textA2)[0]
	f.textB1 = "Steps for [[atlas-migration|the cutover]]."
	f.b1 = f.add(t, "atlas-runbook", "--parent", "atlas-migration", "--description", "How to run the cutover",
		"--add", f.textB1)[0]
	f.textP1 = "Leads [[atlas-migration]]; see [[atlas-migration#^" + f.a1 + "|the date]]."
	f.textP2 = "Wrote `[[atlas-migration]]` in the wiki and [[atlas-cutover]] is planned."
	ids := f.add(t, "priya", "--parent", "people", "--description", "Priya, who leads the migration",
		"--add", f.textP1, "--add", f.textP2)
	f.p1, f.p2 = ids[0], ids[1]
	f.n1 = f.add(t, "notes-misc", "--parent", "topics", "--description", "Odds and ends", "--add", atlasN1)[0]
	return f
}

// note is the folder note path of the node at rel under notes/.
func (f *atlasF) note(rel string) string {
	return filepath.Join(f.notes, filepath.FromSlash(rel), filepath.Base(rel)+".md")
}

// read is read's node for stem.
func (f *atlasF) read(t *testing.T, args ...string) xNode {
	t.Helper()
	return readNode(t, f.dir, args...)
}

func decodeMove(t *testing.T, r result) xMove {
	t.Helper()
	var x xMove
	decode(t, r.stdout, &x)
	check(t, "result element", x.XMLName.Local, "memory_move")
	return x
}

func decodeRename(t *testing.T, r result) xRename {
	t.Helper()
	var x xRename
	decode(t, r.stdout, &x)
	check(t, "result element", x.XMLName.Local, "memory_rename")
	return x
}

// movedPaths re-roots every entry of a snapshot at or under from at to.
func movedPaths(snap map[string]fileState, from, to string) map[string]fileState {
	out := map[string]fileState{}
	for p, st := range snap {
		if p == from || strings.HasPrefix(p, from+string(filepath.Separator)) {
			p = to + p[len(from):]
		}
		out[p] = st
	}
	return out
}

// linksTo returns the links of n whose target node is stem.
func linksTo(n xNode, stem string) []xLink {
	var out []xLink
	for _, l := range links(n.Links) {
		if l.Stem == stem {
			out = append(out, l)
		}
	}
	return out
}

// wantResolving fails unless n has want links to stem and none dangles.
func wantResolving(t *testing.T, n xNode, stem string, want int) {
	t.Helper()
	got := linksTo(n, stem)
	if len(got) != want {
		t.Errorf("%s has %d links to %s, want %d: %+v", n.Stem, len(got), stem, want, links(n.Links))
	}
	for _, l := range got {
		if l.Dangling != "" {
			t.Errorf("%s's link to %s dangles: %+v", n.Stem, stem, l)
		}
	}
}

// entryNames lists the names of every entry under root, sorted.
func entryNames(t *testing.T, root string) []string {
	t.Helper()
	snap := snapshot(t, root)
	out := make([]string, 0, len(snap))
	for p := range snap {
		out = append(out, filepath.Base(p))
	}
	sort.Strings(out)
	return out
}

// Acceptance criterion 1: move relocates the subtree and touches no file.
func TestMoveRelocatesSubtree(t *testing.T) {
	t.Parallel()
	f := newAtlasF(t)
	ageTree(t, f.state)
	before := snapshot(t, f.notes)
	lintBefore := f.lintOf(t)

	r := f.ok(t, "move", "atlas-migration", "--parent", "projects")
	check(t, "result", decodeMove(t, r), xMove{
		XMLName: xml.Name{Local: "memory_move"},
		Stem:    "atlas-migration", From: "people", Parent: "projects", Path: "projects/atlas-migration",
	})
	if !strings.HasPrefix(r.stdout, `<memory_move stem="atlas-migration" from="people" parent="projects" path="projects/atlas-migration">`) {
		t.Errorf("stdout %q", r.stdout)
	}

	from := filepath.Join(f.notes, "people", "atlas-migration")
	to := filepath.Join(f.notes, "projects", "atlas-migration")
	if exists(from) {
		t.Error("notes/people/atlas-migration is still there")
	}
	mustExist(t, filepath.Join(to, "atlas-migration.md"), filepath.Join(to, "atlas-runbook", "atlas-runbook.md"))
	// Every file has the bytes and mtime it had, at its new path where it
	// moved, and nothing else appeared: no copy, no rewrite.
	sameFiles(t, movedPaths(before, from, to), snapshot(t, f.notes))

	check(t, "runbook path", f.read(t, "atlas-runbook").Path, "projects/atlas-migration/atlas-runbook")
	check(t, "runbook parent", f.read(t, "atlas-runbook").Parent, "atlas-migration")
	check(t, "moved node parent", f.read(t, "atlas-migration").Parent, "projects")
	wantResolving(t, f.read(t, "priya"), "atlas-migration", 2)
	check(t, "lint after the move", f.lintOf(t), lintBefore)
}

// Acceptance criterion 2: the root is a parent, written /; a move to the
// parent the node already has prints its element and writes nothing.
func TestMoveRootAndNoOp(t *testing.T) {
	t.Parallel()
	f := newAtlasF(t)
	x := decodeMove(t, f.ok(t, "move", "atlas-migration", "--parent", "/"))
	check(t, "to the root", [...]string{x.Stem, x.From, x.Parent, x.Path}, [...]string{"atlas-migration", "people", "/", "atlas-migration"})
	if !exists(filepath.Join(f.notes, "atlas-migration", "atlas-runbook", "atlas-runbook.md")) {
		t.Error("the subtree is not under notes/ itself")
	}
	var root xRoot
	r := runBin(t, f.dir, nil, "read")
	mustSucceed(t, r)
	decode(t, r.stdout, &root)
	top := map[string]bool{}
	for _, e := range root.Nodes {
		top[e.Stem] = true
	}
	if !top["atlas-migration"] || !top["people"] {
		t.Errorf("the root read does not list atlas-migration among the top-level nodes: %v", top)
	}

	x = decodeMove(t, f.ok(t, "move", "atlas-migration", "--parent", "people"))
	check(t, "back under people", [...]string{x.Stem, x.From, x.Parent, x.Path}, [...]string{"atlas-migration", "/", "people", "people/atlas-migration"})
	check(t, "path after moving back", f.read(t, "atlas-migration").Path, "people/atlas-migration")

	// The no-op: no file or directory mtime changes, nothing appears.
	ageTree(t, f.state)
	before := snapshot(t, f.state)
	r = runBin(t, f.dir, nil, "move", "atlas-migration", "--parent", "people")
	mustSucceed(t, r)
	x = decodeMove(t, r)
	check(t, "no-op", [...]string{x.Stem, x.From, x.Parent, x.Path}, [...]string{"atlas-migration", "people", "people", "people/atlas-migration"})
	sameSnapshot(t, before, snapshot(t, f.state))

	// A first-level node moved to the root is a no-op too.
	before = snapshot(t, f.state)
	x = decodeMove(t, runBin(t, f.dir, nil, "move", "people", "--parent", "/"))
	check(t, "root no-op", [...]string{x.Stem, x.From, x.Parent, x.Path}, [...]string{"people", "/", "/", "people"})
	sameSnapshot(t, before, snapshot(t, f.state))
}

// Acceptance criterion 3: move's refusals change nothing, exit 1, and
// name the argument as typed.
func TestMoveRefusals(t *testing.T) {
	t.Parallel()
	f := newAtlasF(t)
	for _, c := range []struct {
		args  []string
		wants []string
	}{
		{[]string{"atlas-migration"}, []string{"--parent"}},
		{[]string{"atlas-migration", "--parent", "atlas-migration"}, []string{"--parent", "itself"}},
		{[]string{"atlas-migration", "--parent", "atlas-runbook"}, []string{"--parent", "atlas-runbook", "descendant"}},
		{[]string{"people", "--parent", "atlas-runbook"}, []string{"--parent", "atlas-runbook", "descendant"}},
		{[]string{"atlas-migration", "--parent", "nowhere"}, []string{`--parent "nowhere"`, "no such node"}},
		{[]string{"atlas-migration", "-parent=nowhere"}, []string{`-parent "nowhere"`, "no such node"}},
		{[]string{"atlas-migration", "--parent", "atlas-runbok"}, []string{"--parent", "near matches", "atlas-runbook"}},
		{[]string{"atlas-migration", "--parent", ""}, []string{"--parent"}},
		{[]string{"atlas-migration", "--parent"}, []string{"--parent"}},
		{[]string{"nowhere", "--parent", "people"}, []string{`STEM "nowhere"`, "no such node"}},
		{[]string{"atlas-migratio", "--parent", "people"}, []string{"STEM", "near matches", "atlas-migration"}},
		{[]string{"/", "--parent", "people"}, []string{"STEM", "root"}},
		{[]string{"--parent", "people"}, []string{"STEM"}},
		{[]string{"atlas-migration", "priya", "--parent", "projects"}, []string{"STEM"}},
		{[]string{"atlas-migration", "--parent", "projects", "--recursive"}, []string{"--recursive"}},
		{[]string{"atlas-migration", "--parent-dir", f.state, "--parent", "projects"}, []string{"--parent-dir"}},
	} {
		line := f.refused(t, append([]string{"move"}, c.args...)...)
		mustContain(t, strings.Join(c.args, " "), line, c.wants...)
	}
	// STEM errors name STEM first, never --parent.
	for _, args := range [][]string{{"nowhere", "--parent", "people"}, {"/", "--parent", "people"}} {
		if line := f.refused(t, append([]string{"move"}, args...)...); !strings.HasPrefix(line, "STEM") {
			t.Errorf("%v: %q does not name STEM first", args, line)
		}
	}

	// An entry of the node's name already in the parent's folder: here a
	// plain file, which no load could have shown.
	writeFile(t, filepath.Join(f.notes, "projects", "atlas-migration"), "in the way")
	line := f.refused(t, "move", "atlas-migration", "--parent", "projects")
	mustContain(t, "an entry in the way", line, "--parent", "atlas-migration", "already")
	check(t, "the file in the way", readText(t, filepath.Join(f.notes, "projects", "atlas-migration")), "in the way")
}

// An argument error comes before the store is looked at: it creates
// nothing (Part III.1).
func TestMoveRenameArgumentErrorsCreateNothing(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"move", "a"}, {"move", "/", "--parent", "b"}, {"move"},
		{"rename", "a"}, {"rename", "a", "B_c"}, {"rename", "/", "b"}, {"rename", "a", "/"}, {"rename", "a", "b", "c"},
	} {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".agent"), 0o755); err != nil { //nolint:gosec // a test temp dir
			t.Fatal(err)
		}
		mustFail(t, runBin(t, dir, nil, args...))
		if exists(filepath.Join(dir, ".agent", "memory")) {
			t.Errorf("%v created the store", args)
		}
	}
	// With good arguments a missing store is created empty, and the stem
	// is unknown on it.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".agent"), 0o755); err != nil { //nolint:gosec // a test temp dir
		t.Fatal(err)
	}
	mustContain(t, "move on an empty store", mustFail(t, runBin(t, dir, nil, "move", "a", "--parent", "/")), "STEM", "no such node")
	mustContain(t, "rename on an empty store", mustFail(t, runBin(t, dir, nil, "rename", "a", "b")), "STEM", "no such node")
	if !exists(filepath.Join(dir, ".agent", "memory", "notes")) {
		t.Error("the empty store was not created")
	}
}

// renameToGoLive runs criterion 4's rename on a fresh F and returns the
// snapshot of notes/ taken just before it.
func renameToGoLive(t *testing.T, f *atlasF) (before map[string]fileState, r result) {
	t.Helper()
	ageTree(t, f.state)
	before = snapshot(t, f.notes)
	return before, f.ok(t, "rename", "atlas-migration", "atlas-go-live")
}

// Acceptance criterion 4: rename rewrites every link form, and only
// links.
func TestRenameRewritesEveryLinkForm(t *testing.T) {
	t.Parallel()
	f := newAtlasF(t)
	oldA, oldB, oldP := readText(t, f.note("people/atlas-migration")), readText(t, f.note("people/atlas-migration/atlas-runbook")), readText(t, f.note("people/priya"))
	before, r := renameToGoLive(t, f)
	check(t, "result", decodeRename(t, r), xRename{
		XMLName: xml.Name{Local: "memory_rename"}, From: "atlas-migration", Stem: "atlas-go-live", Links: "4",
	})
	if !strings.HasPrefix(r.stdout, `<memory_rename from="atlas-migration" stem="atlas-go-live" links_rewritten="4">`) {
		t.Errorf("stdout %q", r.stdout)
	}

	// On disk: the folder and its note have the new name, the child is
	// beside the note, and nothing is named atlas-migration any more.
	newA, newB := f.note("people/atlas-go-live"), f.note("people/atlas-go-live/atlas-runbook")
	mustExist(t, newA, newB)
	for _, name := range entryNames(t, f.notes) {
		if strings.Contains(name, "atlas-migration") {
			t.Errorf("an entry is still named %s", name)
		}
	}

	// The four links, one in A2, one in B1 and two in P1, and nothing
	// else: each note is its old bytes with exactly those targets changed.
	a2 := "Rollback plan is in [[atlas-go-live#^" + f.a1 + "]] and [[atlas-runbook]]."
	b1 := "Steps for [[atlas-go-live|the cutover]]."
	p1 := "Leads [[atlas-go-live]]; see [[atlas-go-live#^" + f.a1 + "|the date]]."
	check(t, "the renamed node's note", readText(t, newA), strings.Replace(oldA, f.textA2, a2, 1))
	check(t, "the runbook's note", readText(t, newB), strings.Replace(oldB, f.textB1, b1, 1))
	check(t, "priya's note", readText(t, f.note("people/priya")), strings.Replace(oldP, f.textP1, p1, 1))
	check(t, "times each block text is in its note", [...]int{strings.Count(oldA, f.textA2), strings.Count(oldB, f.textB1), strings.Count(oldP, f.textP1)}, [...]int{1, 1, 1})
	check(t, "A2", blockText(t, f.notes, "atlas-go-live", f.a2), a2)
	check(t, "A1 keeps its ID and text", blockText(t, f.notes, "atlas-go-live", f.a1), atlasA1)
	check(t, "B1", blockText(t, f.notes, "atlas-runbook", f.b1), b1)
	check(t, "P1", blockText(t, f.notes, "priya", f.p1), p1)
	check(t, "P2, backticked text included", blockText(t, f.notes, "priya", f.p2), f.textP2)

	// Notes that hold no link keep bytes and mtime; nothing else appeared.
	after := snapshot(t, f.notes)
	oldFolder := filepath.Join(f.notes, "people", "atlas-migration")
	newFolder := filepath.Join(f.notes, "people", "atlas-go-live")
	want := movedPaths(before, oldFolder, newFolder)
	want[newA] = want[filepath.Join(newFolder, "atlas-migration.md")]
	delete(want, filepath.Join(newFolder, "atlas-migration.md"))
	sameFiles(t, want, after, newA, newB, f.note("people/priya"))
	for _, rel := range []string{"topics/notes-misc", "user", "projects", "people", "topics"} {
		if !untouched(before[f.note(rel)], after[f.note(rel)]) {
			t.Errorf("%s was rewritten", rel)
		}
	}

	n := f.read(t, "atlas-go-live")
	check(t, "tags", n.Tags, "atlas")
	check(t, "description", n.Description, "The Atlas migration")
	check(t, "path", n.Path, "people/atlas-go-live")
}

// Acceptance criterion 5: after a rename the old stem is gone and the new
// one works with every verb, a second rename and a move and delete
// included.
func TestRenamedNodeWorksEverywhere(t *testing.T) {
	t.Parallel()
	f := newAtlasF(t)
	lintBefore := f.lintOf(t)
	check(t, "warnings before: the dangling [[atlas-cutover]]", lintBefore.Warnings, "1")
	renameToGoLive(t, f)

	mustContain(t, "read of the old stem", mustFail(t, runBin(t, f.dir, nil, "read", "atlas-migration")), `STEM "atlas-migration"`, "no such node")
	n := f.read(t, "atlas-go-live")
	check(t, "blocks", blocksOf(n), []xBlock{
		{ID: f.a1, Text: atlasA1},
		{ID: f.a2, Text: "Rollback plan is in [[atlas-go-live#^" + f.a1 + "]] and [[atlas-runbook]]."},
	})
	if n.Children == nil || len(n.Children.Nodes) != 1 || n.Children.Nodes[0].Stem != "atlas-runbook" {
		t.Errorf("children: %+v", n.Children)
	}
	back := map[string]bool{}
	for _, l := range links(n.Backlinks) {
		back[l.Stem] = true
	}
	check(t, "backlinks from priya and atlas-runbook", [...]bool{back["priya"], back["atlas-runbook"]}, [...]bool{true, true})
	wantResolving(t, f.read(t, "priya"), "atlas-go-live", 2)
	wantResolving(t, f.read(t, "atlas-runbook"), "atlas-go-live", 1)

	var found xSearch
	r := runBin(t, f.dir, nil, "search", "atlas-go-live")
	mustSucceed(t, r)
	decode(t, r.stdout, &found)
	hit := map[string]bool{}
	for _, h := range found.Nodes {
		hit[h.Stem] = true
	}
	check(t, "search finds the renamed node and a note linking to it", [...]bool{hit["atlas-go-live"], hit["priya"]}, [...]bool{true, true})
	check(t, "lint after the rename", f.lintOf(t), lintBefore)

	// Second-order: the [[atlas-cutover]] in P2 dangled until a node took
	// that stem. It resolves now, is no longer a warning, and was not
	// counted, because it was not rewritten.
	x := decodeRename(t, f.ok(t, "rename", "atlas-go-live", "atlas-cutover"))
	check(t, "second rename", [...]string{x.From, x.Stem, x.Links}, [...]string{"atlas-go-live", "atlas-cutover", "4"})
	wantResolving(t, f.read(t, "priya"), "atlas-cutover", 3)
	check(t, "P2 is unchanged", blockText(t, f.notes, "priya", f.p2), f.textP2)
	lint := f.lintOf(t)
	check(t, "lint after the second rename", [...]string{lint.Errors, lint.Warnings}, [...]string{"0", "0"})

	m := decodeMove(t, f.ok(t, "move", "atlas-cutover", "--parent", "projects"))
	check(t, "move of the renamed node", [...]string{m.Stem, m.From, m.Parent, m.Path}, [...]string{"atlas-cutover", "people", "projects", "projects/atlas-cutover"})
	d := decodeDelete(t, f.ok(t, "delete", "atlas-cutover", "--recursive"))
	check(t, "delete counts", [...]string{d.Stem, d.Nodes, d.Blocks}, [...]string{"atlas-cutover", "2", "3"})
	check(t, "dangling links, under the new name", d.Dangling, []xDangling{
		{Stem: "priya", Block: f.p1, Link: "[[atlas-cutover]]"},
		{Stem: "priya", Block: f.p1, Link: "[[atlas-cutover#^" + f.a1 + "|the date]]"},
		{Stem: "priya", Block: f.p2, Link: "[[atlas-cutover]]"},
	})
	if exists(filepath.Join(f.notes, "projects", "atlas-cutover")) {
		t.Error("the renamed node's folder survived the delete")
	}
}

// Acceptance criterion 6: rename's refusals change nothing, exit 1, and
// name the argument as typed.
func TestRenameRefusals(t *testing.T) {
	t.Parallel()
	f := newAtlasF(t)
	for _, c := range []struct {
		args  []string
		wants []string
	}{
		{[]string{"atlas-migration"}, []string{"NEW"}},
		{[]string{"atlas-migration", "Atlas_Go"}, []string{"NEW", "Atlas_Go", "not a valid stem"}},
		{[]string{"atlas-migration", "priya"}, []string{"NEW", "priya", "in use"}},
		{[]string{"atlas-migration", "atlas-runbook"}, []string{"NEW", "atlas-runbook", "in use"}},
		{[]string{"atlas-migration", "atlas-migration"}, []string{"NEW", "atlas-migration", "already"}},
		{[]string{"nowhere", "x-y"}, []string{`STEM "nowhere"`, "no such node"}},
		{[]string{"atlas-migratio", "x-y"}, []string{"STEM", "near matches", "atlas-migration"}},
		{[]string{"/", "x-y"}, []string{"STEM", "root"}},
		{[]string{"priya", "/"}, []string{"NEW", "root"}},
		{[]string{"a", "b", "c"}, []string{`"c"`, "STEM and NEW"}},
		{nil, []string{"STEM"}},
		{[]string{"atlas-migration", "atlas-go-live", "--parent", "people"}, []string{"--parent"}},
	} {
		line := f.refused(t, append([]string{"rename"}, c.args...)...)
		mustContain(t, strings.Join(c.args, " "), line, c.wants...)
	}
	for args, first := range map[[2]string]string{
		{"atlas-migration", "Atlas_Go"}: "NEW", {"atlas-migration", "priya"}: "NEW", {"atlas-migration", "atlas-migration"}: "NEW",
		{"nowhere", "x-y"}: "STEM", {"/", "x-y"}: "STEM", {"priya", "/"}: "NEW",
	} {
		if line := f.refused(t, "rename", args[0], args[1]); !strings.HasPrefix(line, first) {
			t.Errorf("%v: %q does not name %s first", args, line, first)
		}
	}

	// An entry named NEW already beside the node's folder: a plain file.
	writeFile(t, filepath.Join(f.notes, "people", "atlas-go-live"), "in the way")
	line := f.refused(t, "rename", "atlas-migration", "atlas-go-live")
	mustContain(t, "an entry in the way", line, "NEW", "atlas-go-live", "already")
	if !strings.HasPrefix(line, "NEW") {
		t.Errorf("%q does not name NEW first", line)
	}
	// And a file NEW.md already inside the folder, which the rename of
	// the folder note would overwrite.
	writeFile(t, filepath.Join(f.notes, "people", "atlas-migration", "atlas-live.md"), "a stray file")
	line = f.refused(t, "rename", "atlas-migration", "atlas-live")
	mustContain(t, "a note name in the way", line, "NEW", "atlas-live.md", "already")
}

// bytesOf maps every entry under root to its bytes (directories to ""),
// leaving mtimes out: what "byte-for-byte what it was" compares.
func bytesOf(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for p, st := range snapshot(t, root) {
		out[p] = string(st.data)
	}
	return out
}

// sameBytes fails unless after holds exactly the entries of before, each
// with the same bytes.
func sameBytes(t *testing.T, before, after map[string]string) {
	t.Helper()
	if reflect.DeepEqual(after, before) {
		return
	}
	for p, b := range before {
		if a, ok := after[p]; !ok || a != b {
			t.Errorf("%s was removed or changed: %q", p, a)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s appeared", p)
		}
	}
}

// mustExist fails for each path that is not there.
func mustExist(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if !exists(p) {
			t.Errorf("%s is missing", p)
		}
	}
}

// Acceptance criterion 7: rename is all or nothing. priya's folder is
// read-only, so its note cannot be replaced, after the notes of the
// renamed node and its child (earlier in path order) already were.
func TestRenameIsAllOrNothing(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	f := newAtlasF(t)
	lintBefore := f.lintOf(t)
	priyaDir := filepath.Join(f.notes, "people", "priya")
	if err := os.Chmod(priyaDir, 0o555); err != nil { //nolint:gosec // a test temp dir
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(priyaDir, 0o755) }) //nolint:gosec // a test temp dir
	before := bytesOf(t, f.notes)

	r := runBin(t, f.dir, nil, "rename", "atlas-migration", "atlas-go-live")
	if r.code == 0 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q; want a failure with nothing on stdout", r.code, r.stdout)
	}
	if line := strings.TrimSuffix(r.stderr, "\n"); line == "" || strings.Contains(line, "\n") {
		t.Errorf("stderr %q, want one line", r.stderr)
	}
	sameBytes(t, before, bytesOf(t, f.notes))
	mustExist(t, f.note("people/atlas-migration"))
	check(t, "a folder with the new name exists", exists(filepath.Join(f.notes, "people", "atlas-go-live")), false)
	check(t, "A2 still links to the old stem", blockText(t, f.notes, "atlas-migration", f.a2), f.textA2)
	check(t, "B1 still links to the old stem", blockText(t, f.notes, "atlas-runbook", f.b1), f.textB1)
	check(t, "lint after the failed rename", f.lintOf(t), lintBefore)
	check(t, "the old stem still reads", f.read(t, "atlas-migration").Path, "people/atlas-migration")
}

// Acceptance criterion 8: the usage text lists both verbs.
func TestUsageListsMoveAndRename(t *testing.T) {
	t.Parallel()
	r := runBin(t, t.TempDir(), nil)
	if r.code != 1 || r.stdout != "" {
		t.Fatalf("no arguments: exit %d, stdout %q", r.code, r.stdout)
	}
	mustContain(t, "usage", r.stderr, "move STEM --parent P", "rename STEM NEW", "links_rewritten",
		"write, delete, retag, move and rename take the store's write lock")
}

// The README's Usage section documents both verbs, and the status line
// lists them.
func TestReadmeDocumentsMoveAndRename(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	usage := readmeSection(t, readme, "Usage")
	mustContain(t, "the Usage section", usage,
		"memory move ", "--parent /", "<memory_move ", `from="people"`,
		"memory rename ", "<memory_rename ", "links_rewritten")
	status, _, _ := strings.Cut(readme[strings.Index(readme, "**Status:**"):], "\n")
	mustContain(t, "the status line", status, "`move`", "`rename`")
	if strings.Contains(readme, "no verb moves or renames one yet") {
		t.Error("the README still says no verb moves or renames a node")
	}
}
