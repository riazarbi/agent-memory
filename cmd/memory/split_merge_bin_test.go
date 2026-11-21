package main

// CLI14: memory split and memory merge, through the built binary
// (docs/spec/agent-memory.md Part III.8, "split, exactly" and "merge,
// exactly").

import (
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// xSplit and xMerge are the results as a reader sees them, declared
// independently of the production types.
type xSplit struct {
	XMLName xml.Name
	From    string `xml:"from,attr"`
	Stem    string `xml:"stem,attr"`
	Parent  string `xml:"parent,attr"`
	Path    string `xml:"path,attr"`
	Blocks  string `xml:"blocks,attr"`
	Left    string `xml:"left,attr"`
	Links   string `xml:"links_rewritten,attr"`
}

type xMerge struct {
	XMLName   xml.Name
	From      string `xml:"from,attr"`
	Stem      string `xml:"stem,attr"`
	Blocks    string `xml:"blocks,attr"`
	Children  string `xml:"children,attr"`
	TagsAdded string `xml:"tags_added,attr"`
	Links     string `xml:"links_rewritten,attr"`
}

// smF is the unit's fixture store F: the shipped starting tree plus four
// nodes made with write, and the block IDs write assigned.
type smF struct {
	storeF
	p1, p2, p3, p4, o1, o2, e1, a1 string
	// The block texts as written, where they hold links.
	textP3, textO2, textE1 string
	// The folder notes.
	priya, rules, etiquette, archive string
}

const (
	smP1    = "Priya leads platform."
	smP2    = "Atlas cutover is 2026-12-01."
	smP4    = "Prefers async updates."
	smO1    = "Cc Priya on urgent replies."
	smA1    = "Old rules, replaced by [[outbox-rules|the rules]]."
	smDescr = "The Atlas cutover: date and owners"
)

func newSMF(t *testing.T) *smF {
	t.Helper()
	dir, notes := seeded(t)
	f := &smF{storeF: storeF{dir: dir, notes: notes, state: filepath.Join(dir, ".agent")}}
	ids := f.add(t, "priya", "--parent", "people", "--description", "Priya, who leads platform",
		"--tag", "platform", "--tag", "atlas", "--add", smP1, "--add", smP2)
	f.p1, f.p2 = ids[0], ids[1]
	f.textP3 = "Atlas rollback owner is Sam; see [[priya#^" + f.p2 + "]]."
	ids = f.add(t, "priya", "--add", f.textP3, "--add", smP4)
	f.p3, f.p4 = ids[0], ids[1]
	f.textO2 = "Dates come from [[priya#^" + f.p2 + "|the cutover date]] and [[priya#^" + f.p3 + "]]."
	ids = f.add(t, "outbox-rules", "--parent", "topics", "--description", "The rules for outbox replies",
		"--tag", "outbox", "--tag", "atlas", "--add", smO1, "--add", f.textO2)
	f.o1, f.o2 = ids[0], ids[1]
	f.textE1 = "Be brief. See [[outbox-rules]] and [[outbox-rules#^" + f.o1 + "]]."
	f.e1 = f.add(t, "outbox-etiquette", "--parent", "topics", "--description", "How to write outbox replies",
		"--tag", "outbox", "--tag", "etiquette", "--add", f.textE1)[0]
	f.a1 = f.add(t, "outbox-archive", "--parent", "outbox-rules", "--description", "Rules no longer in force", "--add", smA1)[0]
	f.priya = filepath.Join(notes, "people", "priya", "priya.md")
	f.rules = filepath.Join(notes, "topics", "outbox-rules", "outbox-rules.md")
	f.etiquette = filepath.Join(notes, "topics", "outbox-etiquette", "outbox-etiquette.md")
	f.archive = filepath.Join(notes, "topics", "outbox-rules", "outbox-archive", "outbox-archive.md")
	return f
}

// splitCall is criterion 1's call.
func (f *smF) splitCall() []string {
	return []string{"split", "priya", "--into", "atlas-cutover", "--parent", "projects",
		"--description", smDescr, "--block", f.p3, "--block", f.p2}
}

// mergeCall is criterion 4's call.
func mergeCall() []string { return []string{"merge", "outbox-rules", "--into", "outbox-etiquette"} }

// text is the text read STEM --block ID shows.
func (f *smF) text(t *testing.T, stem, id string) string {
	t.Helper()
	n := readNode(t, f.dir, stem, "--block", id)
	if len(n.Blocks) != 1 {
		t.Fatalf("read %s --block %s shows %d blocks", stem, id, len(n.Blocks))
	}
	return n.Blocks[0].Text
}

func idsOf(n xNode) []string {
	ids := make([]string, 0, len(n.Blocks))
	for _, b := range n.Blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

func childStems(n xNode) []string {
	var out []string
	if n.Children != nil {
		for _, c := range n.Children.Nodes {
			out = append(out, c.Stem)
		}
	}
	return out
}

// okNoNewProblem runs a call that must succeed and, beyond f.ok's check on
// lint errors, must add no lint warning either.
func (f *smF) okNoNewProblem(t *testing.T, args ...string) result {
	t.Helper()
	before := map[xProblem]int{}
	for _, p := range f.lintOf(t).Problems {
		before[p]++
	}
	r := f.ok(t, args...)
	for _, p := range f.lintOf(t).Problems {
		if before[p] == 0 {
			t.Errorf("%v added the lint %s %s: %s", args, p.Severity, p.Path, p.Reason)
		}
		before[p]--
	}
	return r
}

// line is a block as write lays it out in a note.
func line(text, id string) string { return text + " ^" + id + "\n" }

// noFolderNamed fails if any entry under notes/ has that name.
func noFolderNamed(t *testing.T, notes, name string) {
	t.Helper()
	err := filepath.WalkDir(notes, func(p string, _ fs.DirEntry, err error) error {
		if err == nil && filepath.Base(p) == name {
			t.Errorf("an entry named %s exists: %s", name, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Acceptance criterion 1: split puts the named blocks, in argv order and
// with their IDs, in a new node where the caller says, and rewrites every
// link to them.
func TestSplitMovesNamedBlocksIntoNewNode(t *testing.T) {
	t.Parallel()
	f := newSMF(t)
	oldPriya, oldRules := readText(t, f.priya), readText(t, f.rules)
	ageTree(t, f.state)
	before := snapshot(t, f.state)

	r := f.okNoNewProblem(t, f.splitCall()...)
	check(t, "stdout", r.stdout, `<memory_split from="priya" stem="atlas-cutover" parent="projects" path="projects/atlas-cutover" blocks="2" left="2" links_rewritten="3"></memory_split>`+"\n")

	newP3 := "Atlas rollback owner is Sam; see [[atlas-cutover#^" + f.p2 + "]]."
	cut := readNode(t, f.dir, "atlas-cutover")
	check(t, "the new node's description", cut.Description, smDescr)
	check(t, "the new node's tags", cut.Tags, "")
	check(t, "the new node's place", []string{cut.Parent, cut.Path}, []string{"projects", "projects/atlas-cutover"})
	check(t, "the new node's blocks, in the order given", blocksOf(cut), []xBlock{{ID: f.p3, Text: newP3}, {ID: f.p2, Text: smP2}})

	priya := readNode(t, f.dir, "priya")
	check(t, "priya's blocks", idsOf(priya), []string{f.p1, f.p4})
	check(t, "priya's tags", priya.Tags, "platform,atlas")
	check(t, "priya's place", priya.Path, "people/priya")
	check(t, "O2", f.text(t, "outbox-rules", f.o2),
		"Dates come from [[atlas-cutover#^"+f.p2+"|the cutover date]] and [[atlas-cutover#^"+f.p3+"]].")

	// priya.md: its old bytes minus the two paragraphs and their
	// separating blank lines.
	p2Para, p3Para := line(smP2, f.p2)+"\n", line(f.textP3, f.p3)+"\n"
	if !strings.Contains(oldPriya, p2Para+p3Para) {
		t.Fatalf("the fixture does not exercise the removal: %q", oldPriya)
	}
	check(t, "priya.md", readText(t, f.priya), strings.Replace(oldPriya, p2Para+p3Para, "", 1))
	check(t, "outbox-rules.md", readText(t, f.rules), strings.ReplaceAll(oldRules, "[[priya#^", "[[atlas-cutover#^"))

	// atlas-cutover.md: what write makes of the same node on a scratch
	// store, once its two assigned IDs are replaced by P3 and P2.
	scratch, scratchNotes := seeded(t)
	w := decodeWrite(t, runBin(t, scratch, nil, "write", "atlas-cutover", "--parent", "projects",
		"--description", smDescr, "--add", newP3, "--add", smP2))
	if len(w.Assigned) != 2 {
		t.Fatalf("the scratch write assigned %v", w.Assigned)
	}
	want := readText(t, filepath.Join(scratchNotes, "projects", "atlas-cutover", "atlas-cutover.md"))
	want = strings.NewReplacer(w.Assigned[0], f.p3, w.Assigned[1], f.p2).Replace(want)
	newDir := filepath.Join(f.notes, "projects", "atlas-cutover")
	newNote := filepath.Join(newDir, "atlas-cutover.md")
	check(t, "atlas-cutover.md", readText(t, newNote), want)
	check(t, "the new folder's mode", fileMode(t, newDir).Perm(), os.FileMode(0o755))
	check(t, "the new note's mode", fileMode(t, newNote).Perm(), os.FileMode(0o644))

	// Notes holding no link to a moved block keep bytes and mtime.
	sameFiles(t, before, snapshot(t, f.state), f.priya, f.rules, newDir, newNote)
	noHiddenLeftovers(t, f.notes)
	lint := f.lintOf(t)
	check(t, "lint errors and warnings", []string{lint.Errors, lint.Warnings}, []string{"0", "0"})
}

// Acceptance criterion 2: the new node goes wherever --parent says - under
// the split node itself, or at the root - and a split may empty the node.
func TestSplitEveryDirectionAndAnEmptiedNode(t *testing.T) {
	t.Parallel()
	t.Run("a child of the split node", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		r := f.okNoNewProblem(t, "split", "priya", "--into", "priya-atlas", "--parent", "priya",
			"--description", "Priya's Atlas work", "--block", f.p2)
		check(t, "stdout", r.stdout, `<memory_split from="priya" stem="priya-atlas" parent="priya" path="people/priya/priya-atlas" blocks="1" left="3" links_rewritten="2"></memory_split>`+"\n")
		priya := readNode(t, f.dir, "priya")
		check(t, "priya's children", childStems(priya), []string{"priya-atlas"})
		check(t, "priya's blocks", idsOf(priya), []string{f.p1, f.p3, f.p4})
		check(t, "P3", f.text(t, "priya", f.p3), "Atlas rollback owner is Sam; see [[priya-atlas#^"+f.p2+"]].")
		check(t, "the child's blocks", blocksOf(readNode(t, f.dir, "priya-atlas")), []xBlock{{ID: f.p2, Text: smP2}})
	})
	t.Run("at the root", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		r := f.okNoNewProblem(t, "split", "priya", "--into", "priya-atlas", "--parent", "/",
			"--description", "Priya's Atlas work", "--block", f.p2)
		var x xSplit
		decode(t, r.stdout, &x)
		check(t, "result", x, xSplit{XMLName: xml.Name{Local: "memory_split"}, From: "priya", Stem: "priya-atlas",
			Parent: "/", Path: "priya-atlas", Blocks: "1", Left: "3", Links: "2"})
		top := rootStems(t, runBin(t, f.dir, nil, "read"))
		if !slices.Contains(top, "priya-atlas") {
			t.Errorf("the top-level nodes %v do not list priya-atlas", top)
		}
		mustExist(t, filepath.Join(f.notes, "priya-atlas", "priya-atlas.md"))
	})
	t.Run("every block: the node is left empty, not removed", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		r := f.okNoNewProblem(t, "split", "priya", "--into", "priya-atlas", "--parent", "people",
			"--description", "Priya's Atlas work", "--block", f.p1, "--block", f.p2, "--block", f.p3, "--block", f.p4)
		var x xSplit
		decode(t, r.stdout, &x)
		check(t, "counts", []string{x.Blocks, x.Left}, []string{"4", "0"})
		priya := readNode(t, f.dir, "priya")
		check(t, "priya still reads", []string{priya.Stem, priya.Path, priya.Tags}, []string{"priya", "people/priya", "platform,atlas"})
		check(t, "priya's blocks", len(priya.Blocks), 0)
		check(t, "the new node's blocks", idsOf(readNode(t, f.dir, "priya-atlas")), []string{f.p1, f.p2, f.p3, f.p4})
	})
}

// Acceptance criterion 3: every split refusal is exit 1 with one line
// naming the argument as typed, and changes nothing: no byte, no mtime,
// and no folder for the new stem anywhere.
func TestSplitRefusalsChangeNothing(t *testing.T) {
	t.Parallel()
	f := newSMF(t)
	// The store exists, with its lock file, before the snapshots.
	f.ok(t, "write", "priya", "--description", "Priya, who leads platform")
	split := func(stem string, flags ...string) []string { return append([]string{"split", stem}, flags...) }
	into, parent, descr := []string{"--into", "atlas-cutover"}, []string{"--parent", "projects"}, []string{"--description", smDescr}
	blocks := []string{"--block", f.p3, "--block", f.p2}
	join := func(parts ...[]string) []string { return slices.Concat(parts...) }
	for name, c := range map[string]struct {
		args  []string
		names string
		wants []string
	}{
		"no --into":             {split("priya", join(parent, descr, blocks)...), "--into", nil},
		"no --parent":           {split("priya", join(into, descr, blocks)...), "--parent", nil},
		"no --description":      {split("priya", join(into, parent, blocks)...), "--description", nil},
		"no --block":            {split("priya", join(into, parent, descr)...), "--block", nil},
		"a block of another":    {split("priya", join(into, parent, descr, []string{"--block", f.o1})...), "--block", []string{f.o1, "priya"}},
		"one good, one not":     {split("priya", join(into, parent, descr, []string{"--block", f.p2, "--block", "m-zzzzzz"})...), "--block", []string{"m-zzzzzz"}},
		"the same block twice":  {split("priya", join(into, parent, descr, []string{"--block", f.p2, "--block", f.p2})...), "--block", []string{f.p2, "twice"}},
		"an invalid new stem":   {split("priya", join([]string{"--into", "Atlas_Cutover"}, parent, descr, blocks)...), "--into", []string{"Atlas_Cutover"}},
		"a stem in use":         {split("priya", join([]string{"--into", "outbox-rules"}, parent, descr, blocks)...), "--into", []string{"outbox-rules", "in use"}},
		"an unknown parent":     {split("priya", join(into, []string{"--parent", "nowhere"}, descr, blocks)...), "--parent", []string{"nowhere"}},
		"an unknown stem":       {split("nowhere", join(into, parent, descr, blocks)...), "STEM", []string{"nowhere"}},
		"the root as STEM":      {split("/", join(into, parent, descr, blocks)...), "STEM", []string{"root"}},
		"the root as NEW":       {split("priya", join([]string{"--into", "/"}, parent, descr, blocks)...), "--into", []string{"root"}},
		"an empty description":  {split("priya", join(into, parent, []string{"--description", ""}, blocks)...), "--description", nil},
		"the flag as typed":     {split("priya", join([]string{"-into", "Atlas_Cutover"}, parent, descr, blocks)...), "-into", nil},
		"an unknown flag":       {split("priya", join(into, parent, descr, blocks, []string{"--tag", "atlas"})...), "--tag", nil},
		"a second stem":         {split("priya", join([]string{"people"}, into, parent, descr, blocks)...), "STEM", nil},
		"the new stem's parent": {split("priya", join(into, []string{"--parent", "atlas-cutover"}, descr, blocks)...), "--parent", []string{"atlas-cutover"}},
	} {
		line := f.refused(t, c.args...)
		if !strings.HasPrefix(line, c.names+" ") && !strings.HasPrefix(line, c.names+":") {
			t.Errorf("%s: %q does not name %s first", name, line, c.names)
		}
		mustContain(t, name, line, c.wants...)
		noFolderNamed(t, f.notes, "atlas-cutover")
		noFolderNamed(t, f.notes, "Atlas_Cutover")
	}
	check(t, "priya's blocks after the refusals", idsOf(readNode(t, f.dir, "priya")), []string{f.p1, f.p2, f.p3, f.p4})

	// A folder that did not load carries the stem: write would refuse to
	// create it, so split does, naming --into.
	writeFile(t, filepath.Join(f.notes, "topics", "ghost", "ghost.md"), "no frontmatter here\n")
	line := f.refused(t, split("priya", join([]string{"--into", "ghost"}, parent, descr, blocks)...)...)
	if !strings.HasPrefix(line, "--into") {
		t.Errorf("%q does not name --into first", line)
	}
	mustContain(t, "a stem a skipped folder has", line, "ghost", "topics/ghost")
	check(t, "a second ghost folder exists", exists(filepath.Join(f.notes, "projects", "ghost")), false)
	// And one differing only by letter case.
	line = f.refused(t, split("priya", join([]string{"--into", "outbox-rules"}, parent, descr, blocks)...)...)
	mustContain(t, "a stem in use", line, "--into")
	writeFile(t, filepath.Join(f.notes, "topics", "Atlas-Cutover", "readme.txt"), "not a node")
	line = f.refused(t, f.splitCall()...)
	mustContain(t, "a stem differing by case", line, "--into", "letter case", "topics/Atlas-Cutover")
}

// Acceptance criterion 4: merge folds the blocks, children, links and tags
// of one node into another and removes it.
func TestMergeFoldsANodeIntoAnother(t *testing.T) {
	t.Parallel()
	f := newSMF(t)
	oldEtiquette := readText(t, f.etiquette)
	etiquetteDescr := readNode(t, f.dir, "outbox-etiquette").Description
	ageTree(t, f.state)
	before := snapshot(t, f.state)

	r := f.okNoNewProblem(t, mergeCall()...)
	check(t, "stdout", r.stdout, `<memory_merge from="outbox-rules" stem="outbox-etiquette" blocks="2" children="1" tags_added="1" links_rewritten="3"></memory_merge>`+"\n")

	newE1 := "Be brief. See [[outbox-etiquette]] and [[outbox-etiquette#^" + f.o1 + "]]."
	kept := readNode(t, f.dir, "outbox-etiquette")
	check(t, "the kept node's description", kept.Description, etiquetteDescr)
	check(t, "the kept node's tags", kept.Tags, "outbox,etiquette,atlas")
	check(t, "the kept node's place", kept.Path, "topics/outbox-etiquette")
	check(t, "the kept node's blocks", blocksOf(kept), []xBlock{{ID: f.e1, Text: newE1}, {ID: f.o1, Text: smO1}, {ID: f.o2, Text: f.textO2}})
	check(t, "the kept node's children", childStems(kept), []string{"outbox-archive"})
	archive := readNode(t, f.dir, "outbox-archive")
	check(t, "the child's place", []string{archive.Parent, archive.Path}, []string{"outbox-etiquette", "topics/outbox-etiquette/outbox-archive"})
	check(t, "A1", blocksOf(archive), []xBlock{{ID: f.a1, Text: "Old rules, replaced by [[outbox-etiquette|the rules]]."}})

	// The body of the kept note: its own, link targets apart, then the
	// two blocks, each after one blank line.
	wantBody := strings.ReplaceAll(bodyOf(oldEtiquette), "[[outbox-rules", "[[outbox-etiquette") +
		"\n" + line(smO1, f.o1) + "\n" + line(f.textO2, f.o2)
	check(t, "outbox-etiquette.md's body", bodyOf(readText(t, f.etiquette)), wantBody)

	// The folded-in node is gone, with no hidden leftover.
	line := mustFail(t, runBin(t, f.dir, nil, "read", "outbox-rules"))
	mustContain(t, "read outbox-rules", line, "STEM", "outbox-rules", "no such node")
	check(t, "notes/topics/outbox-rules exists", exists(filepath.Join(f.notes, "topics", "outbox-rules")), false)
	noHiddenLeftovers(t, f.notes)

	// priya.md and the starting-tree notes keep bytes and mtime; the
	// folded-in node's files are the ones that are gone or moved.
	after := snapshot(t, f.state)
	for _, p := range []string{f.priya, filepath.Join(f.notes, "topics", "topics.md"), filepath.Join(f.notes, "people", "people.md"),
		filepath.Join(f.notes, "projects", "projects.md"), filepath.Join(f.notes, "user", "user.md")} {
		if a, ok := after[p]; !ok || !untouched(before[p], a) {
			t.Errorf("%s was removed or rewritten", p)
		}
	}
	lint := f.lintOf(t)
	check(t, "lint errors and warnings", []string{lint.Errors, lint.Warnings}, []string{"0", "0"})
}

// Acceptance criterion 5: merge carries along what did not load, refuses
// to overwrite an entry in the target's folder, merges a child up into its
// parent, and refuses cleanly.
func TestMergeCarriesMergesUpAndRefuses(t *testing.T) {
	t.Parallel()
	plant := func(f *smF) {
		writeFile(t, filepath.Join(f.notes, "topics", "outbox-rules", "notes.txt"), "a stray file\n")
		writeFile(t, filepath.Join(f.notes, "topics", "outbox-rules", "broken", "broken.md"), "no frontmatter here\n")
	}
	t.Run("what did not load goes along", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		plant(f)
		errorsBefore := lintErrors(f.lintOf(t))
		check(t, "lint errors before", len(errorsBefore), 1)
		// Not f.ok: the one error is the same skipped folder at its new
		// path (III.8's postcondition), which f.ok would call new.
		r := runBin(t, f.dir, nil, mergeCall()...)
		mustSucceed(t, r)
		check(t, "stdout", r.stdout, `<memory_merge from="outbox-rules" stem="outbox-etiquette" blocks="2" children="1" tags_added="1" links_rewritten="3"></memory_merge>`+"\n")
		kept := filepath.Join(f.notes, "topics", "outbox-etiquette")
		check(t, "notes.txt", readText(t, filepath.Join(kept, "notes.txt")), "a stray file\n")
		check(t, "broken/broken.md", readText(t, filepath.Join(kept, "broken", "broken.md")), "no frontmatter here\n")
		check(t, "notes/topics/outbox-rules exists", exists(filepath.Join(f.notes, "topics", "outbox-rules")), false)
		noHiddenLeftovers(t, f.notes)
		errorsAfter := map[xProblem]int{}
		for p, n := range errorsBefore {
			p.Path = strings.Replace(p.Path, "topics/outbox-rules/", "topics/outbox-etiquette/", 1)
			errorsAfter[p] = n
		}
		check(t, "lint errors after: the same skipped folder, where it now is", lintErrors(f.lintOf(t)), errorsAfter)
	})
	t.Run("an entry of the same name in the target's folder", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		plant(f)
		writeFile(t, filepath.Join(f.notes, "topics", "outbox-etiquette", "notes.txt"), "the target's own\n")
		line := f.refused(t, mergeCall()...)
		mustContain(t, "the collision", line, "notes.txt", "--into")
		check(t, "the target's notes.txt", readText(t, filepath.Join(f.notes, "topics", "outbox-etiquette", "notes.txt")), "the target's own\n")
	})
	t.Run("a child into its parent", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		r := f.okNoNewProblem(t, "merge", "outbox-archive", "--into", "outbox-rules")
		var x xMerge
		decode(t, r.stdout, &x)
		check(t, "result", x, xMerge{XMLName: xml.Name{Local: "memory_merge"}, From: "outbox-archive", Stem: "outbox-rules",
			Blocks: "1", Children: "0", TagsAdded: "0", Links: "0"})
		rules := readNode(t, f.dir, "outbox-rules")
		check(t, "outbox-rules' blocks", idsOf(rules), []string{f.o1, f.o2, f.a1})
		check(t, "outbox-rules' children", len(childStems(rules)), 0)
		check(t, "A1 now links to its own node", f.text(t, "outbox-rules", f.a1), smA1)
		check(t, "the child's folder exists", exists(filepath.Dir(f.archive)), false)
		noHiddenLeftovers(t, f.notes)
	})
	t.Run("refusals", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		for name, c := range map[string]struct {
			args  []string
			names string
			wants []string
		}{
			"no --into":             {[]string{"merge", "outbox-rules"}, "--into", nil},
			"into itself":           {[]string{"merge", "outbox-rules", "--into", "outbox-rules"}, "--into", []string{"outbox-rules"}},
			"into its descendant":   {[]string{"merge", "outbox-rules", "--into", "outbox-archive"}, "--into", []string{"outbox-archive", "descendant"}},
			"an unknown FROM":       {[]string{"merge", "nowhere", "--into", "priya"}, "FROM", []string{"nowhere"}},
			"an unknown TO":         {[]string{"merge", "priya", "--into", "nowhere"}, "--into", []string{"nowhere"}},
			"the root as FROM":      {[]string{"merge", "/", "--into", "priya"}, "FROM", []string{"root"}},
			"the root as TO":        {[]string{"merge", "priya", "--into", "/"}, "--into", []string{"root"}},
			"no arguments":          {[]string{"merge"}, "FROM", nil},
			"two stems":             {[]string{"merge", "priya", "outbox-rules"}, "FROM", nil},
			"an unknown flag":       {[]string{"merge", "priya", "--into", "outbox-rules", "--recursive"}, "--recursive", nil},
			"--into as it is typed": {[]string{"merge", "priya", "-into", "nowhere"}, "-into", []string{"nowhere"}},
		} {
			line := f.refused(t, c.args...)
			if !strings.HasPrefix(line, c.names+" ") && !strings.HasPrefix(line, c.names+":") {
				t.Errorf("%s: %q does not name %s first", name, line, c.names)
			}
			mustContain(t, name, line, c.wants...)
		}
		mustExist(t, f.rules, f.archive, f.etiquette, f.priya)
	})
}

// failsWhole runs a call that must fail with nothing on stdout and one
// line on stderr, and returns after checking the store is byte-for-byte
// what it was.
func (f *smF) failsWhole(t *testing.T, args ...string) {
	t.Helper()
	before := bytesOf(t, f.notes)
	r := runBin(t, f.dir, nil, args...)
	if r.code == 0 || r.stdout != "" {
		t.Fatalf("%v: exit %d, stdout %q; want a failure with nothing on stdout", args, r.code, r.stdout)
	}
	if line := strings.TrimSuffix(r.stderr, "\n"); line == "" || strings.Contains(line, "\n") {
		t.Errorf("%v: stderr %q, want one line", args, r.stderr)
	}
	sameBytes(t, before, bytesOf(t, f.notes))
}

func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o555); err != nil { //nolint:gosec // a test temp dir
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // a test temp dir
}

// Acceptance criterion 6: both verbs are all or nothing. A linking note
// that cannot be replaced fails the call after other notes were already
// written, and the store is byte-for-byte what it was.
func TestSplitAndMergeAreAllOrNothing(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	t.Run("split", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		lintBefore := f.lintOf(t)
		// O2's note cannot be replaced; priya's, earlier in path order, can.
		readOnly(t, filepath.Dir(f.rules))
		f.failsWhole(t, f.splitCall()...)
		noFolderNamed(t, f.notes, "atlas-cutover")
		check(t, "priya's blocks", idsOf(readNode(t, f.dir, "priya")), []string{f.p1, f.p2, f.p3, f.p4})
		check(t, "P3", f.text(t, "priya", f.p3), f.textP3)
		check(t, "O2", f.text(t, "outbox-rules", f.o2), f.textO2)
		check(t, "lint after the failed split", f.lintOf(t), lintBefore)
	})
	t.Run("merge", func(t *testing.T) {
		t.Parallel()
		f := newSMF(t)
		lintBefore := f.lintOf(t)
		// A1's note cannot be replaced; the target's, earlier in path
		// order, can.
		readOnly(t, filepath.Dir(f.archive))
		f.failsWhole(t, mergeCall()...)
		mustExist(t, f.rules, f.archive, f.etiquette)
		check(t, "the child moved", exists(filepath.Join(f.notes, "topics", "outbox-etiquette", "outbox-archive")), false)
		check(t, "the kept node's blocks", idsOf(readNode(t, f.dir, "outbox-etiquette")), []string{f.e1})
		check(t, "E1", f.text(t, "outbox-etiquette", f.e1), f.textE1)
		check(t, "the kept node's tags", readNode(t, f.dir, "outbox-etiquette").Tags, "outbox,etiquette")
		check(t, "the folded-in node's blocks", idsOf(readNode(t, f.dir, "outbox-rules")), []string{f.o1, f.o2})
		check(t, "lint after the failed merge", f.lintOf(t), lintBefore)
	})
}

// Acceptance criterion 7: split and merge undo each other, and what they
// leave works with the other verbs.
func TestSplitAndMergeUndoEachOther(t *testing.T) {
	t.Parallel()
	f := newSMF(t)
	f.okNoNewProblem(t, mergeCall()...)

	r := f.okNoNewProblem(t, "split", "outbox-etiquette", "--into", "outbox-urgent", "--parent", "outbox-etiquette",
		"--description", "Urgent replies", "--block", f.o1)
	check(t, "split", r.stdout, `<memory_split from="outbox-etiquette" stem="outbox-urgent" parent="outbox-etiquette" path="topics/outbox-etiquette/outbox-urgent" blocks="1" left="2" links_rewritten="1"></memory_split>`+"\n")
	check(t, "E1 after the split", f.text(t, "outbox-etiquette", f.e1),
		"Be brief. See [[outbox-etiquette]] and [[outbox-urgent#^"+f.o1+"]].")
	check(t, "outbox-urgent's blocks", blocksOf(readNode(t, f.dir, "outbox-urgent")), []xBlock{{ID: f.o1, Text: smO1}})

	r = f.okNoNewProblem(t, "merge", "outbox-urgent", "--into", "outbox-etiquette")
	check(t, "merge", r.stdout, `<memory_merge from="outbox-urgent" stem="outbox-etiquette" blocks="1" children="0" tags_added="0" links_rewritten="1"></memory_merge>`+"\n")
	kept := readNode(t, f.dir, "outbox-etiquette")
	check(t, "the kept node's blocks", idsOf(kept), []string{f.e1, f.o2, f.o1})
	check(t, "the kept node's children", childStems(kept), []string{"outbox-archive"})
	check(t, "E1 after the merge", f.text(t, "outbox-etiquette", f.e1),
		"Be brief. See [[outbox-etiquette]] and [[outbox-etiquette#^"+f.o1+"]].")
	for _, l := range links(kept.Links) {
		if l.Dangling != "" {
			t.Errorf("a dangling link after the round trip: %+v", l)
		}
	}

	f.ok(t, "rename", "outbox-etiquette", "outbox-guide")
	f.ok(t, "move", "outbox-guide", "--parent", "people")
	check(t, "the node after rename and move", readNode(t, f.dir, "outbox-guide").Path, "people/outbox-guide")
	check(t, "A1 after the rename", f.text(t, "outbox-archive", f.a1), "Old rules, replaced by [[outbox-guide|the rules]].")
	var del xDelete
	decode(t, f.ok(t, "delete", "outbox-guide", "--recursive").stdout, &del)
	check(t, "nodes deleted", del.Nodes, "2")
	check(t, "the folder exists", exists(filepath.Join(f.notes, "people", "outbox-guide")), false)
	noHiddenLeftovers(t, f.notes)
}

var spaces = regexp.MustCompile(`\s+`)

// Acceptance criterion 8: the usage text lists both verbs, and one
// sentence names all eight writing verbs as taking the write lock.
func TestUsageListsSplitAndMerge(t *testing.T) {
	t.Parallel()
	r := runBin(t, t.TempDir(), nil)
	if r.code != 1 || r.stdout != "" {
		t.Fatalf("no arguments: exit %d, stdout %q", r.code, r.stdout)
	}
	mustContain(t, "usage", r.stderr,
		"split STEM --into NEW --parent P --description D --block ID [--block ID]...", "merge FROM --into TO")
	flat := spaces.ReplaceAllString(r.stderr, " ")
	mustContain(t, "the usage text's lock sentence", flat,
		"write, delete, retag, move, rename, move-block, split and merge take the store's write lock")
	if strings.Count(flat, "write lock") != 1 {
		t.Errorf("the usage text speaks of the write lock %d times, want one sentence", strings.Count(flat, "write lock"))
	}
	// The package comment lists them too.
	src := readText(t, filepath.Join(repoRoot(t), "cmd", "memory", "main.go"))
	comment, _, _ := strings.Cut(src, "\npackage main")
	mustContain(t, "the package comment", spaces.ReplaceAllString(strings.ReplaceAll(comment, "//", ""), " "), "split", "merge folds")
}

// The README's Usage section documents both verbs, and the status line
// lists them.
func TestReadmeDocumentsSplitAndMerge(t *testing.T) {
	t.Parallel()
	readme := readText(t, filepath.Join(repoRoot(t), "README.md"))
	usage := readmeSection(t, readme, "Usage")
	mustContain(t, "the Usage section", usage,
		"memory split ", "--into", "--description", "<memory_split ", `left="`,
		"memory merge ", "<memory_merge ", `tags_added="`, `children="`)
	status, _, _ := strings.Cut(readme[strings.Index(readme, "**Status:**"):], "\n")
	mustContain(t, "the status line", status, "`split`", "`merge`")
}

// A flag that takes one value, given twice, is refused by every verb and
// changes nothing (spec III.8, "A flag given twice"); the flags a verb takes
// many of still repeat (newSMF's own writes repeat --tag and --add).
func TestRepeatedFlagIsRefused(t *testing.T) {
	f := newSMF(t)
	for name, tc := range map[string]struct {
		args []string
		flag string
	}{
		"merge --into":        {[]string{"merge", "priya", "--into", "outbox-rules", "--into", "outbox-etiquette"}, "--into"},
		"move --parent":       {[]string{"move", "priya", "--parent", "topics", "-parent", "people"}, "-parent"},
		"move-block --to":     {[]string{"move-block", f.p1, "--to", "outbox-rules", "--to=outbox-etiquette"}, "--to"},
		"delete --recursive":  {[]string{"delete", "outbox-rules", "--recursive", "--recursive"}, "--recursive"},
		"delete --block":      {[]string{"delete", "priya", "--block", f.p1, "--block", f.p2}, "--block"},
		"read --depth":        {[]string{"read", "priya", "--depth", "1", "--depth", "1"}, "--depth"},
		"write --parent":      {[]string{"write", "newnode", "--parent", "topics", "--parent", "people", "--description", "d"}, "--parent"},
		"write --description": {[]string{"write", "priya", "--description", "a", "--description", "b"}, "--description"},
		"split --into": {[]string{"split", "priya", "--into", "a", "--into", "b", "--parent", "priya",
			"--description", "d", "--block", f.p1}, "--into"},
	} {
		line := f.refused(t, tc.args...)
		if !strings.Contains(line, tc.flag+": given more than once") {
			t.Errorf("%s: stderr %q, want it to hold %q", name, line, tc.flag+": given more than once")
		}
	}
}
