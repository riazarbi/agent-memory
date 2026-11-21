package main

// CLI13: memory move-block, and rename's check of its new stem, through
// the built binary (docs/spec/agent-memory.md Part III.8, "move-block,
// exactly" and "rename, exactly").

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// xMoveBlock is the result as a reader sees it, declared independently of
// the production type.
type xMoveBlock struct {
	XMLName xml.Name
	ID      string `xml:"id,attr"`
	From    string `xml:"from,attr"`
	Stem    string `xml:"stem,attr"`
	Links   string `xml:"links_rewritten,attr"`
}

// blockF is the unit's fixture store F: the shipped starting tree plus
// four nodes made with write, and the block IDs write assigned.
type blockF struct {
	storeF
	p1, p2, p3, c1, c2, r1, n1 string
	// The block texts as written.
	textP3, textC2, textR1 string
	// The folder notes.
	priya, cutover, runbook, misc string
}

const (
	bfP1 = "Priya leads platform."
	bfP2 = "Atlas cutover is 2026-12-01; rollback is [[atlas-runbook]]."
	bfC1 = "Owner is [[priya]]."
	bfN1 = "Nothing about Atlas here."
)

func newBlockF(t *testing.T) *blockF {
	t.Helper()
	dir, notes := seeded(t)
	f := &blockF{storeF: storeF{dir: dir, notes: notes, state: filepath.Join(dir, ".agent")}}
	ids := f.add(t, "priya", "--parent", "people", "--description", "Priya, who leads platform", "--add", bfP1, "--add", bfP2)
	f.p1, f.p2 = ids[0], ids[1]
	f.textP3 = "The date in [[priya#^" + f.p2 + "]] is firm."
	f.p3 = f.add(t, "priya", "--add", f.textP3)[0]
	f.textC2 = "See [[priya#^" + f.p2 + "|the date]] and `[[priya#^" + f.p2 + "]]`."
	ids = f.add(t, "atlas-cutover", "--parent", "projects", "--description", "The Atlas cutover", "--add", bfC1, "--add", f.textC2)
	f.c1, f.c2 = ids[0], ids[1]
	f.textR1 = "Follows [[priya#^" + f.p2 + "]]."
	f.r1 = f.add(t, "atlas-runbook", "--parent", "atlas-cutover", "--description", "How to run the cutover", "--add", f.textR1)[0]
	f.n1 = f.add(t, "notes-misc", "--parent", "topics", "--description", "Odds and ends", "--add", bfN1)[0]
	f.priya = filepath.Join(notes, "people", "priya", "priya.md")
	f.cutover = filepath.Join(notes, "projects", "atlas-cutover", "atlas-cutover.md")
	f.runbook = filepath.Join(notes, "projects", "atlas-cutover", "atlas-runbook", "atlas-runbook.md")
	f.misc = filepath.Join(notes, "topics", "notes-misc", "notes-misc.md")
	return f
}

func decodeMoveBlock(t *testing.T, r result) xMoveBlock {
	t.Helper()
	var x xMoveBlock
	decode(t, r.stdout, &x)
	check(t, "result element", x.XMLName.Local, "memory_move_block")
	return x
}

// blockIDs lists the IDs of read STEM's blocks, in order.
func (f *blockF) blockIDs(t *testing.T, stem string) []string {
	t.Helper()
	blocks := readNode(t, f.dir, stem).Blocks
	ids := make([]string, 0, len(blocks))
	for _, b := range blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

// text is the text read STEM --block ID shows.
func (f *blockF) text(t *testing.T, stem, id string) string {
	t.Helper()
	n := readNode(t, f.dir, stem, "--block", id)
	if len(n.Blocks) != 1 {
		t.Fatalf("read %s --block %s shows %d blocks", stem, id, len(n.Blocks))
	}
	return n.Blocks[0].Text
}

// moveP2 runs criterion 1's call on f and returns the snapshot of the
// state dir taken just before it.
func (f *blockF) moveP2(t *testing.T) (before map[string]fileState, r result) {
	t.Helper()
	ageTree(t, f.state)
	before = snapshot(t, f.state)
	return before, f.ok(t, "move-block", f.p2, "--to", "atlas-cutover")
}

// p2Line is the moved block as it is written in a note.
func (f *blockF) p2Line() string { return bfP2 + " ^" + f.p2 + "\n" }

// Acceptance criterion 1: the block moves with its ID and text, and
// nothing else in either note changes, but for each one's modified line,
// which the move sets to the time of the call; the note with only a link
// retargeted keeps the date it has, byte for byte (III.9).
func TestMoveBlockMovesTheBlock(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)
	dateNote(t, f.priya, dfDate)
	dateNote(t, f.cutover, dfDate)
	oldPriya, oldCutover, oldRunbook := readText(t, f.priya), readText(t, f.cutover), readText(t, f.runbook)
	start := callStart()
	before, r := f.moveP2(t)

	check(t, "result", decodeMoveBlock(t, r), xMoveBlock{
		XMLName: xml.Name{Local: "memory_move_block"}, ID: f.p2, From: "priya", Stem: "atlas-cutover", Links: "3",
	})
	check(t, "stdout", r.stdout, `<memory_move_block id="`+f.p2+`" from="priya" stem="atlas-cutover" links_rewritten="3"></memory_move_block>`+"\n")

	check(t, "atlas-cutover's blocks", f.blockIDs(t, "atlas-cutover"), []string{f.c1, f.c2, f.p2})
	check(t, "the moved block's text", f.text(t, "atlas-cutover", f.p2), bfP2)
	check(t, "priya's blocks", f.blockIDs(t, "priya"), []string{f.p1, f.p3})

	// priya.md: its old bytes minus the paragraph and its separating
	// blank line, with P3's link target changed.
	wantPriya := strings.Replace(oldPriya, f.p2Line()+"\n", "", 1)
	wantPriya = strings.Replace(wantPriya, "[[priya#^"+f.p2+"]] is firm", "[[atlas-cutover#^"+f.p2+"]] is firm", 1)
	check(t, "priya.md", readText(t, f.priya), redatedBy(t, "priya.md", f.priya, wantPriya, start))
	if strings.Contains(wantPriya, f.p2Line()) || !strings.Contains(oldPriya, f.p2Line()+"\n") {
		t.Fatalf("the fixture does not exercise the removal: %q", oldPriya)
	}
	// atlas-cutover.md: its old bytes with C2's first link target changed,
	// one blank line, then the block's line.
	wantCutover := strings.Replace(oldCutover, "[[priya#^"+f.p2+"|the date]]", "[[atlas-cutover#^"+f.p2+"|the date]]", 1) + "\n" + f.p2Line()
	check(t, "atlas-cutover.md", readText(t, f.cutover), redatedBy(t, "atlas-cutover.md", f.cutover, wantCutover, start))
	check(t, "atlas-runbook.md", readText(t, f.runbook), strings.Replace(oldRunbook, "[[priya#^", "[[atlas-cutover#^", 1))

	// Nothing else is written: notes-misc and the starting tree keep
	// bytes and mtime.
	sameFiles(t, before, snapshot(t, f.state), f.priya, f.cutover, f.runbook)
	noHiddenLeftovers(t, f.notes)
}

// Acceptance criterion 1, "the target is not tidied": a hand-written
// paragraph with no ID stays without one, and a run of blank lines stays.
func TestMoveBlockDoesNotTidyTheTarget(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)
	c1Line := bfC1 + " ^" + f.c1 + "\n"
	old := readText(t, f.cutover)
	edited := strings.Replace(old, c1Line+"\n", c1Line+"\n\n\nHand-written, no ID.\n\n", 1)
	if edited == old {
		t.Fatalf("could not place the hand-written paragraph in %q", old)
	}
	writeFile(t, f.cutover, edited)
	dateNote(t, f.cutover, dfDate)
	edited = readText(t, f.cutover)
	start := callStart()
	f.moveP2(t)

	// Every byte stays but the modified line, set to the time of the
	// call: the note gained a block (III.9).
	want := strings.Replace(edited, "[[priya#^"+f.p2+"|the date]]", "[[atlas-cutover#^"+f.p2+"|the date]]", 1) + "\n" + f.p2Line()
	check(t, "atlas-cutover.md", readText(t, f.cutover), redatedBy(t, "atlas-cutover.md", f.cutover, want, start))
	n := readNode(t, f.dir, "atlas-cutover")
	check(t, "atlas-cutover's blocks", blocksOf(n), []xBlock{
		{ID: f.c1, Text: bfC1},
		{ID: "", Text: "Hand-written, no ID."},
		{ID: f.c2, Text: "See [[atlas-cutover#^" + f.p2 + "|the date]] and `[[priya#^" + f.p2 + "]]`."},
		{ID: f.p2, Text: bfP2},
	})
}

// Acceptance criterion 2: every link to the block follows it; links to
// the node, and text that only looks like a link, do not.
func TestMoveBlockRewritesLinksToTheBlock(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)
	lintBefore := f.lintOf(t)
	f.moveP2(t)

	check(t, "P3", f.text(t, "priya", f.p3), "The date in [[atlas-cutover#^"+f.p2+"]] is firm.")
	check(t, "C2", f.text(t, "atlas-cutover", f.c2), "See [[atlas-cutover#^"+f.p2+"|the date]] and `[[priya#^"+f.p2+"]]`.")
	check(t, "R1", f.text(t, "atlas-runbook", f.r1), "Follows [[atlas-cutover#^"+f.p2+"]].")
	check(t, "C1", f.text(t, "atlas-cutover", f.c1), bfC1)

	moved := readNode(t, f.dir, "atlas-cutover", "--block", f.p2)
	check(t, "backlinks of the moved block", links(moved.Backlinks), []xLink{
		{Stem: "atlas-cutover", Block: f.c2}, {Stem: "atlas-runbook", Block: f.r1}, {Stem: "priya", Block: f.p3},
	})
	check(t, "lint after the move", f.lintOf(t), lintBefore)
	check(t, "lint problems", len(lintBefore.Problems), 0)
}

// Acceptance criterion 3: a block that links to itself, and a node
// emptied by the move.
func TestMoveBlockSelfLinkAndEmptiedNode(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)
	n2 := f.add(t, "notes-misc", "--add", "This claim stands alone.")[0]
	f.add(t, "notes-misc", "--replace", n2+"=This claim, [[notes-misc#^"+n2+"]], stands alone.")

	x := decodeMoveBlock(t, f.ok(t, "move-block", n2, "--to", "priya"))
	check(t, "self-link move", x, xMoveBlock{XMLName: xml.Name{Local: "memory_move_block"}, ID: n2, From: "notes-misc", Stem: "priya", Links: "1"})
	check(t, "the moved block", f.text(t, "priya", n2), "This claim, [[priya#^"+n2+"]], stands alone.")

	x = decodeMoveBlock(t, f.ok(t, "move-block", f.n1, "--to", "priya"))
	check(t, "emptying move", [...]string{x.From, x.Stem, x.Links}, [...]string{"notes-misc", "priya", "0"})
	misc := readNode(t, f.dir, "notes-misc")
	check(t, "notes-misc still reads", [...]string{misc.Stem, misc.Path}, [...]string{"notes-misc", "topics/notes-misc"})
	check(t, "blocks left in notes-misc", len(misc.Blocks), 0)
	check(t, "lint errors", f.lintOf(t).Errors, "0")
	d := decodeDelete(t, f.ok(t, "delete", "notes-misc"))
	check(t, "delete of the emptied node", [...]string{d.Stem, d.Nodes, d.Blocks}, [...]string{"notes-misc", "1", "0"})
}

// Acceptance criterion 4: a moved block keeps working with the other
// verbs: moved back, renamed around, deleted.
func TestMoveBlockThenOtherVerbs(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)
	f.moveP2(t)

	x := decodeMoveBlock(t, f.ok(t, "move-block", f.p2, "--to", "priya"))
	check(t, "the move back", x, xMoveBlock{XMLName: xml.Name{Local: "memory_move_block"}, ID: f.p2, From: "atlas-cutover", Stem: "priya", Links: "3"})
	check(t, "priya's blocks: last, not its old place", f.blockIDs(t, "priya"), []string{f.p1, f.p3, f.p2})
	check(t, "atlas-cutover's blocks", f.blockIDs(t, "atlas-cutover"), []string{f.c1, f.c2})
	check(t, "P3 after the round trip", f.text(t, "priya", f.p3), f.textP3)
	check(t, "C2 after the round trip", f.text(t, "atlas-cutover", f.c2), f.textC2)
	check(t, "R1 after the round trip", f.text(t, "atlas-runbook", f.r1), f.textR1)

	f.ok(t, "rename", "priya", "priya-shah")
	check(t, "P3 after the rename", f.text(t, "priya-shah", f.p3), "The date in [[priya-shah#^"+f.p2+"]] is firm.")
	check(t, "C2 after the rename", f.text(t, "atlas-cutover", f.c2), "See [[priya-shah#^"+f.p2+"|the date]] and `[[priya#^"+f.p2+"]]`.")
	check(t, "R1 after the rename", f.text(t, "atlas-runbook", f.r1), "Follows [[priya-shah#^"+f.p2+"]].")

	d := decodeDelete(t, f.ok(t, "delete", "priya-shah", "--block", f.p2))
	check(t, "dangling links to the deleted block", d.Dangling, []xDangling{
		{Stem: "atlas-cutover", Block: f.c2, Link: "[[priya-shah#^" + f.p2 + "|the date]]"},
		{Stem: "atlas-runbook", Block: f.r1, Link: "[[priya-shah#^" + f.p2 + "]]"},
		{Stem: "priya-shah", Block: f.p3, Link: "[[priya-shah#^" + f.p2 + "]]"},
	})
}

// Acceptance criterion 5: refusals change nothing, exit 1, and name the
// argument as typed.
func TestMoveBlockRefusals(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)
	for _, c := range []struct {
		args  []string
		first string
		wants []string
	}{
		{nil, "ID", nil},
		{[]string{f.p2}, "--to", nil},
		{[]string{"--to", "priya"}, "ID", nil},
		{[]string{"m-zzzzzz", "--to", "priya"}, "ID", []string{"m-zzzzzz", "no block"}},
		{[]string{"^" + f.p2, "--to", "atlas-cutover"}, "ID", []string{"no block"}},
		{[]string{"m-zzzzzz", "--to", "nowhere"}, "ID", []string{"m-zzzzzz"}},
		{[]string{f.p2, "--to", "nowhere"}, `--to "nowhere"`, []string{"no such node"}},
		{[]string{f.p2, "--to", "atlas-cutove"}, "--to", []string{"near matches", "atlas-cutover"}},
		{[]string{f.p2, "-to", "nowhere"}, `-to "nowhere"`, []string{"no such node"}},
		{[]string{f.p2, "--to", "/"}, "--to", []string{"root"}},
		{[]string{f.p2, "--to="}, "--to", nil},
		{[]string{f.p2, "--to", "priya"}, "--to", []string{"already", "priya"}},
		{[]string{f.p2, "-to", "priya"}, "-to:", []string{"already"}},
		{[]string{f.p2, "--to", "atlas-cutover", "extra"}, `"extra"`, []string{"one ID"}},
		{[]string{f.p2, "--to", "atlas-cutover", "--parent", "people"}, "--parent", nil},
	} {
		line := f.refused(t, append([]string{"move-block"}, c.args...)...)
		if !strings.HasPrefix(line, c.first) {
			t.Errorf("%v: %q does not name %s first", c.args, line, c.first)
		}
		mustContain(t, strings.Join(c.args, " "), line, c.wants...)
	}
	// The refused calls left the block where it was.
	check(t, "priya's blocks", f.blockIDs(t, "priya"), []string{f.p1, f.p2, f.p3})
	if exists(filepath.Join(f.notes, "people", "nowhere")) || exists(filepath.Join(f.notes, "nowhere")) {
		t.Error("an unknown --to created a node")
	}

	// P1's ID pasted by hand onto a paragraph of another note: lint warns
	// it is used twice, and the call cannot tell which block is meant.
	writeFile(t, f.misc, readText(t, f.misc)+"\nPasted by hand. ^"+f.p1+"\n")
	lint := f.lintOf(t)
	check(t, "lint with the pasted ID", [...]string{lint.Errors, lint.Warnings}, [...]string{"0", "1"})
	mustContain(t, "the lint warning", lint.Problems[0].Reason, f.p1, "also used")
	line := f.refused(t, "move-block", f.p1, "--to", "atlas-cutover")
	if !strings.HasPrefix(line, "ID") {
		t.Errorf("%q does not name ID first", line)
	}
	mustContain(t, "an ID carried twice", line, f.p1, "more than one block")
	check(t, "priya still holds its first block", f.blockIDs(t, "priya"), []string{f.p1, f.p2, f.p3})
	check(t, "atlas-cutover's blocks", f.blockIDs(t, "atlas-cutover"), []string{f.c1, f.c2})
}

// Missing arguments and --to / are refused before the store is created
// (III.1); an unknown ID is found only once it exists.
func TestMoveBlockArgumentErrorsCreateNothing(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"move-block"}, {"move-block", "m-aaaaaa"}, {"move-block", "--to", "x"},
		{"move-block", "m-aaaaaa", "--to", "/"}, {"move-block", "m-aaaaaa", "--to", "x", "y"},
		{"move-block", "m-aaaaaa", "--to", ""}, {"move-block", "", "--to", "x"},
	} {
		w, parent := freshParent(t)
		mustFail(t, runBin(t, w, nil, args...))
		if exists(filepath.Join(parent, "memory")) {
			t.Errorf("%v created the store", args)
		}
	}
	w, parent := freshParent(t)
	line := mustFail(t, runBin(t, w, nil, "move-block", "m-aaaaaa", "--to", "x"))
	mustContain(t, "move-block on a missing store", line, "ID", "m-aaaaaa", "no block")
	if info, err := os.Stat(filepath.Join(parent, "memory", "notes")); err != nil || !info.IsDir() {
		t.Fatalf("memory/notes/ was not created: %v", err)
	}
}

// Acceptance criterion 6: all or nothing. atlas-runbook's folder is
// read-only, so its note cannot be replaced, after priya's and
// atlas-cutover's (earlier in path order) already were.
func TestMoveBlockIsAllOrNothing(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	f := newBlockF(t)
	lintBefore := f.lintOf(t)
	runbookDir := filepath.Dir(f.runbook)
	if err := os.Chmod(runbookDir, 0o555); err != nil { //nolint:gosec // a test temp dir
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(runbookDir, 0o755) }) //nolint:gosec // a test temp dir
	before := bytesOf(t, f.notes)

	r := runBin(t, f.dir, nil, "move-block", f.p2, "--to", "atlas-cutover")
	if r.code == 0 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q; want a failure with nothing on stdout", r.code, r.stdout)
	}
	if line := strings.TrimSuffix(r.stderr, "\n"); line == "" || strings.Contains(line, "\n") {
		t.Errorf("stderr %q, want one line", r.stderr)
	}
	sameBytes(t, before, bytesOf(t, f.notes))
	check(t, "P2 is in priya once", strings.Count(readText(t, f.priya), "^"+f.p2+"\n"), 1)
	check(t, "priya's blocks", f.blockIDs(t, "priya"), []string{f.p1, f.p2, f.p3})
	check(t, "atlas-cutover's blocks", f.blockIDs(t, "atlas-cutover"), []string{f.c1, f.c2})
	check(t, "P3 still links to priya", f.text(t, "priya", f.p3), f.textP3)
	check(t, "C2 still links to priya", f.text(t, "atlas-cutover", f.c2), f.textC2)
	check(t, "R1 still links to priya", f.text(t, "atlas-runbook", f.r1), f.textR1)
	check(t, "lint after the failed move", f.lintOf(t), lintBefore)
}

// Acceptance criterion 7: rename refuses a new stem that write would
// refuse to create: one carried by a folder that did not load, anywhere
// in the tree, or differing from a folder's name only by case.
func TestRenameChecksNewAsWriteChecksAStem(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)

	ghost := filepath.Join(f.notes, "topics", "ghost")
	writeFile(t, filepath.Join(ghost, "ghost.md"), "no frontmatter here\n")
	line := f.refused(t, "rename", "priya", "ghost")
	if !strings.HasPrefix(line, "NEW") {
		t.Errorf("%q does not name NEW first", line)
	}
	mustContain(t, "rename onto an unloaded folder's stem", line, "ghost", "topics/ghost", "missing frontmatter")
	wline := f.refused(t, "write", "ghost", "--parent", "people", "--description", "x")
	mustContain(t, "write of the same stem", wline, "ghost", "topics/ghost", "missing frontmatter")
	if err := os.RemoveAll(ghost); err != nil {
		t.Fatal(err)
	}

	atlasGo := filepath.Join(f.notes, "topics", "Atlas-Go")
	mkdirs(t, atlasGo)
	line = f.refused(t, "rename", "priya", "atlas-go")
	if !strings.HasPrefix(line, "NEW") {
		t.Errorf("%q does not name NEW first", line)
	}
	mustContain(t, "rename onto a name differing by case", line, "atlas-go", "only by letter case", "topics/Atlas-Go")
	if err := os.RemoveAll(atlasGo); err != nil {
		t.Fatal(err)
	}

	x := decodeRename(t, f.ok(t, "rename", "priya", "atlas-go"))
	check(t, "rename with neither in the way", [...]string{x.From, x.Stem, x.Links}, [...]string{"priya", "atlas-go", "4"})
	check(t, "the renamed node reads", readNode(t, f.dir, "atlas-go").Path, "people/atlas-go")
}

// Acceptance criterion 8: the usage text lists move-block, and says it
// takes the lock.
func TestUsageListsMoveBlock(t *testing.T) {
	t.Parallel()
	r := runBin(t, t.TempDir(), nil)
	if r.code != 1 || r.stdout != "" {
		t.Fatalf("no arguments: exit %d, stdout %q", r.code, r.stdout)
	}
	mustContain(t, "usage", r.stderr, "move-block ID --to STEM", "links_rewritten", "write lock")
}

// The README's Usage section documents move-block and the stricter
// rename, and the status line lists the verb.
func TestReadmeDocumentsMoveBlock(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	usage := readmeSection(t, readme, "Usage")
	mustContain(t, "the Usage section", usage,
		"memory move-block ", "--to ", "<memory_move_block ", `from="priya"`, "links_rewritten",
		"only by letter case", "did not load")
	status, _, _ := strings.Cut(readme[strings.Index(readme, "**Status:**"):], "\n")
	mustContain(t, "the status line", status, "`move-block`")
}
