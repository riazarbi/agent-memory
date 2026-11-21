//go:build unix

package memory

// CLI14: Writer.Split and Writer.Merge (docs/spec/agent-memory.md Part
// III.8, "split, exactly" and "merge, exactly").

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The split and merge fixture's notes. priya holds four blocks; its third
// links to its second and to the node. rules links to both, quotes one
// link in inline code, and holds a hand-written paragraph with no ID after
// a run of blank lines; archive, its child, links to rules; etiquette
// links to rules and to its first block, and has a property with a
// comment, a run of blank lines and a hand-written last paragraph.
const (
	smP3        = "Sam owns the rollback; see [[priya#^m-ppppp2]] and [[priya]]."
	smPriya     = "---\ndescription: Priya\ntags: [platform, atlas]\n---\n\nPriya leads platform. ^m-ppppp1\n\nAtlas cutover is 2026-12-01. ^m-ppppp2\n\n" + smP3 + " ^m-ppppp3\n\nPrefers async updates. ^m-ppppp4\n"
	smRules     = "---\ndescription: The rules\ntags: [outbox, atlas]\n---\n\nCc Priya on urgent replies. ^m-ooooo1\n\n\n\nHand-written, no ID.\n\nDates: [[priya#^m-ppppp2|the date]], [[priya#^m-ppppp3]], `[[priya#^m-ppppp2]]`. ^m-ooooo2\n"
	smArchive   = fm + "\nOld rules, replaced by [[rules|the rules]]. ^m-aaaaa1\n"
	smEtiquette = "---\ndescription: Etiquette\ntags: [outbox, etiquette]\nkind: guide\n---\n\n\nBe brief. See [[rules]] and [[rules#^m-ooooo1]]. ^m-eeeee1\n\n\nOperator line.\n"

	smPriyaAfter = "---\ndescription: Priya\ntags: [platform, atlas]\n---\n\nPriya leads platform. ^m-ppppp1\n\nPrefers async updates. ^m-ppppp4\n"
	smCutover    = "---\ndescription: The Atlas cutover\n---\n\nSam owns the rollback; see [[cutover#^m-ppppp2]] and [[priya]]. ^m-ppppp3\n\nAtlas cutover is 2026-12-01. ^m-ppppp2\n"
	smRulesAfter = "---\ndescription: The rules\ntags: [outbox, atlas]\n---\n\nCc Priya on urgent replies. ^m-ooooo1\n\n\n\nHand-written, no ID.\n\nDates: [[cutover#^m-ppppp2|the date]], [[cutover#^m-ppppp3]], `[[priya#^m-ppppp2]]`. ^m-ooooo2\n"

	// After merging rules into etiquette: the frontmatter re-emitted with
	// the one tag etiquette lacked, the body as it was, link targets
	// apart, then rules' three paragraphs, the hand-written one still
	// without an ID.
	smEtiquetteAfter = "---\ndescription: Etiquette\ntags: [outbox, etiquette, atlas]\nkind: guide\n---\n\n\nBe brief. See [[etiquette]] and [[etiquette#^m-ooooo1]]. ^m-eeeee1\n\n\nOperator line.\n\nCc Priya on urgent replies. ^m-ooooo1\n\nHand-written, no ID.\n\nDates: [[priya#^m-ppppp2|the date]], [[priya#^m-ppppp3]], `[[priya#^m-ppppp2]]`. ^m-ooooo2\n"
	smArchiveAfter   = fm + "\nOld rules, replaced by [[etiquette|the rules]]. ^m-aaaaa1\n"
)

// smTree is the split and merge fixture.
func smTree(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"people/people.md":                fm,
		"projects/projects.md":            fm,
		"topics/topics.md":                fm,
		"people/priya/priya.md":           smPriya,
		"topics/rules/rules.md":           smRules,
		"topics/rules/archive/archive.md": smArchive,
		"topics/etiquette/etiquette.md":   smEtiquette,
		"topics/misc/misc.md":             miscNote,
	}
	for k, v := range extra {
		files[k] = v
	}
	return writeTree(t, files)
}

// cutoverSplit is the fixture's split: priya's third and second blocks,
// in that order, into a new node cutover under projects.
func cutoverSplit() SplitRequest {
	return SplitRequest{Stem: "priya", New: "cutover", Parent: "projects", Description: "The Atlas cutover",
		Blocks: []string{"m-ppppp3", "m-ppppp2"}}
}

func mustSplit(t *testing.T, dir string, req SplitRequest) SplitResult {
	t.Helper()
	res, err := writerFor(t, dir).Split(req)
	if err != nil {
		t.Fatalf("split %+v: %v", req, err)
	}
	return res
}

func mustMerge(t *testing.T, dir, from, to string) MergeResult {
	t.Helper()
	res, err := writerFor(t, dir).Merge(from, to)
	if err != nil {
		t.Fatalf("merge %s into %s: %v", from, to, err)
	}
	return res
}

// blockIDsOf lists a node's block IDs, in order; "" for a paragraph
// without one.
func blockIDsOf(n *Node) []string {
	ids := make([]string, 0, len(n.Blocks))
	for _, b := range n.Blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

// wantMode fails unless the entry at rel has the permission bits want.
func wantMode(t *testing.T, dir, rel string, want os.FileMode) {
	t.Helper()
	if m := fileMode(t, filepath.Join(dir, filepath.FromSlash(rel))); m != want {
		t.Errorf("the mode of %s is %v, want %v", rel, m, want)
	}
}

func wantNoProblems(t *testing.T, dir string) *Store {
	t.Helper()
	st := Load(dir)
	for _, p := range st.Problems() {
		t.Errorf("problem after the call: %s %s: %s", p.Severity, p.Path, p.Reason)
	}
	return st
}

// The named blocks arrive in the new node in the order given, with their
// IDs; they leave the old note as a delete removes them; every link to
// them, and nothing else, is retargeted; the new node has no tags. The
// split note and the new one are dated with the time of the call; the note
// with only links retargeted, having no date, is given its file's (III.9).
func TestSplit_MovesBlocksAndRelinks(t *testing.T) {
	atCallTime(t)
	withUmask(t, 0o077)
	dir := smTree(t, nil)
	want := treeState(t, dir)
	ageNotes(t, dir)
	res := mustSplit(t, dir, cutoverSplit())
	if wantRes := (SplitResult{Path: "projects/cutover", Blocks: 2, Left: 2, Links: 3}); res != wantRes {
		t.Errorf("result %+v, want %+v", res, wantRes)
	}
	want["people/priya/priya.md"] = stamped(smPriyaAfter, callText)
	want["projects/cutover"] = "dir"
	want["projects/cutover/cutover.md"] = stamped(smCutover, callText)
	want["topics/rules/rules.md"] = stamped(smRulesAfter, agedText)
	wantTree(t, dir, want)
	for _, rel := range []string{"topics/misc/misc.md", "topics/etiquette/etiquette.md", "topics/rules/archive/archive.md", "projects/projects.md"} {
		wantAged(t, dir, rel)
	}
	noTempFiles(t, dir)
	// The modes a create sets, whatever the umask.
	wantMode(t, dir, "projects/cutover", 0o755)
	wantMode(t, dir, "projects/cutover/cutover.md", 0o644)
}

// After a split the store loads clean: the new node has no tags and no
// properties, the old node keeps its own, and the links to the moved
// blocks reach them where they are now.
func TestSplit_LoadsClean(t *testing.T) {
	dir := smTree(t, nil)
	mustSplit(t, dir, cutoverSplit())
	st := wantNoProblems(t, dir)
	cut := mustNode(t, st, "cutover")
	if len(cut.Tags) != 0 || len(cut.Properties) != 0 || cut.Parent().Stem != "projects" {
		t.Errorf("the new node has tags %v, properties %v, parent %s", cut.Tags, cut.Properties, cut.Parent().Stem)
	}
	if got := mustNode(t, st, "priya").Tags; !reflect.DeepEqual(got, []string{"platform", "atlas"}) {
		t.Errorf("priya's tags are %v", got)
	}
	if got, want := st.BlockBacklinks("cutover", "m-ppppp2"), []Ref{{Stem: "cutover", Block: "m-ppppp3"}, {Stem: "rules", Block: "m-ooooo2"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("backlinks of the moved block %+v, want %+v", got, want)
	}
	// The link to the node priya was not touched.
	if got := st.Backlinks("priya"); !reflect.DeepEqual(got, []Ref{{Stem: "cutover", Block: "m-ppppp3"}}) {
		t.Errorf("backlinks of priya %+v", got)
	}
}

// The new note is byte-equal to the one a write create with the same
// description and block texts makes at the same time of call, each block
// carrying its own ID. The clock is fixed: both notes carry the time of
// their call as modified (III.9).
func TestSplit_NewNoteIsWhatAWriteCreates(t *testing.T) {
	atCallTime(t)
	dir := smTree(t, nil)
	mustSplit(t, dir, cutoverSplit())
	got, err := os.ReadFile(filepath.Join(dir, "projects", "cutover", "cutover.md"))
	if err != nil {
		t.Fatal(err)
	}
	scratch := smTree(t, nil)
	res, err := writerFor(t, scratch).Write(WriteRequest{Stem: "cutover", Parent: "projects", Description: strp("The Atlas cutover"),
		Blocks: []BlockEdit{{Text: "Sam owns the rollback; see [[cutover#^m-ppppp2]] and [[priya]]."}, {Text: "Atlas cutover is 2026-12-01."}}})
	if err != nil {
		t.Fatal(err)
	}
	made, err := os.ReadFile(filepath.Join(scratch, "projects", "cutover", "cutover.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(res.BlockIDs[0], "m-ppppp3", res.BlockIDs[1], "m-ppppp2").Replace(string(made))
	if string(got) != want {
		t.Errorf("the new note is\n%q\nwant what write makes\n%q", got, want)
	}
	if dates := modifiedLines(string(got)); len(dates) != 1 || dates[0] != callText {
		t.Errorf("the new note's modified lines are %q, want exactly %q", dates, callText)
	}
}

// Each block leaves the old note as delete --block removes a block:
// whichever blocks a split names, in whatever order, the old note is
// byte-equal to the one that deleting them one by one leaves at the same
// time of call. The clock is fixed: either way the note is dated with the
// time of the call (III.9).
func TestSplit_OldNoteIsWhatDeletesLeave(t *testing.T) {
	atCallTime(t)
	// Irregular on purpose: runs of blank lines, CRLF endings, a
	// hand-written paragraph, an ID on its own line, no final newline.
	const messy = "---\r\ndescription: d\r\n---\r\n\r\n\r\nOne. ^m-bbbbb1\r\n\r\n\r\n\r\nTwo\r\nlines. ^m-bbbbb2\r\n\r\nHand-written.\r\n\r\nThree.\r\n^m-bbbbb3\r\n\r\n\r\nFour. ^m-bbbbb4"
	ids := []string{"m-bbbbb1", "m-bbbbb2", "m-bbbbb3", "m-bbbbb4"}
	for mask := 1; mask < 1<<len(ids); mask++ {
		var named []string
		for i, id := range ids {
			if mask&(1<<i) != 0 {
				named = append(named, id)
			}
		}
		for _, order := range [][]string{named, reversed(named)} {
			splitVersusDeletes(t, map[string]string{"topics/topics.md": fm, "topics/messy/messy.md": messy}, order)
		}
	}
}

// splitVersusDeletes splits the blocks order off the five-paragraph node
// messy in one copy of files and deletes them one by one in another, and
// fails unless both leave the same note and the new node holds the blocks
// in the order given.
func splitVersusDeletes(t *testing.T, files map[string]string, order []string) {
	t.Helper()
	deleted := writeTree(t, files)
	for _, id := range order {
		mustDelete(t, deleted, DeleteRequest{Stem: "messy", Block: id})
	}
	split := writeTree(t, files)
	res := mustSplit(t, split, SplitRequest{Stem: "messy", New: "tidy", Parent: "messy", Description: "d", Blocks: order})
	if res.Blocks != len(order) || res.Left != 5-len(order) {
		t.Errorf("%v: result %+v", order, res)
	}
	got, want := treeState(t, split)["topics/messy/messy.md"], treeState(t, deleted)["topics/messy/messy.md"]
	if got != want {
		t.Errorf("split of %v leaves\n%q\ndeleting them leaves\n%q", order, got, want)
	}
	if dates := modifiedLines(strings.ReplaceAll(got, "\r\n", "\n")); len(dates) != 1 || dates[0] != callText {
		t.Errorf("split of %v: the old note's modified lines are %q, want exactly %q", order, dates, callText)
	}
	if arrived := blockIDsOf(mustNode(t, Load(split), "tidy")); !reflect.DeepEqual(arrived, order) {
		t.Errorf("the new node's blocks are %v, want the order given %v", arrived, order)
	}
}

func reversed(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[len(in)-1-i] = s
	}
	return out
}

// The new node goes where the caller says: under the split node itself,
// under one of its descendants, or at the root. A split may empty the
// node, which stays, dated with the time of the call (III.9).
func TestSplit_DirectionsAndEmptiedNode(t *testing.T) {
	atCallTime(t)
	for parent, wantPath := range map[string]string{
		"priya": "people/priya/cutover", RootParent: "cutover", "archive": "topics/rules/archive/cutover", "people": "people/cutover",
	} {
		dir := smTree(t, nil)
		req := cutoverSplit()
		req.Parent = parent
		if res := mustSplit(t, dir, req); res.Path != wantPath {
			t.Errorf("--parent %s: path %s, want %s", parent, res.Path, wantPath)
		}
		st := wantNoProblems(t, dir)
		if n := mustNode(t, st, "cutover"); n.Path != wantPath {
			t.Errorf("--parent %s: the node loaded at %s", parent, n.Path)
		}
	}
	dir := smTree(t, nil)
	res := mustSplit(t, dir, SplitRequest{Stem: "priya", New: "cutover", Parent: "priya", Description: "d",
		Blocks: []string{"m-ppppp1", "m-ppppp2", "m-ppppp3", "m-ppppp4"}})
	if want := (SplitResult{Path: "people/priya/cutover", Blocks: 4, Left: 0, Links: 3}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	st := wantNoProblems(t, dir)
	if n := mustNode(t, st, "priya"); len(n.Blocks) != 0 || len(n.Tags) != 2 {
		t.Errorf("the emptied node has %d blocks and tags %v", len(n.Blocks), n.Tags)
	}
	if got := treeState(t, dir)["people/priya/priya.md"]; got != "---\ndescription: Priya\ntags: [platform, atlas]\nmodified: "+callText+"\n---\n" {
		t.Errorf("the emptied note is %q", got)
	}
}

// A moved block keeps the reader warnings its text already had, and a
// block that is only an ID arrives as its ID alone. The new note is dated
// with the time of the call (III.9).
func TestSplit_CarriesWarningsAndAnEmptyBlock(t *testing.T) {
	atCallTime(t)
	dir := smTree(t, map[string]string{
		"topics/odd/odd.md": fm + "\nSee [[rules#Heading]] and ![[rules]]. ^m-ddddd1\n\n^m-ddddd2\n\nStays. ^m-ddddd3\n",
	})
	before := len(Load(dir).Problems())
	mustSplit(t, dir, SplitRequest{Stem: "odd", New: "odder", Parent: "odd", Description: "d", Blocks: []string{"m-ddddd2", "m-ddddd1"}})
	if got := treeState(t, dir)["topics/odd/odder/odder.md"]; got != stamped(fm+"\n^m-ddddd2\n\nSee [[rules#Heading]] and ![[rules]]. ^m-ddddd1\n", callText) {
		t.Errorf("the new note is %q", got)
	}
	if after := len(Load(dir).Problems()); after != before {
		t.Errorf("%d problems before, %d after", before, after)
	}
}

// Every refusal names its field and changes nothing; no folder is made
// for the new stem.
func TestSplit_Refusals(t *testing.T) {
	dir := smTree(t, map[string]string{
		"topics/ghost/ghost.md":       "no frontmatter here\n",
		"topics/Up-Case/readme.txt":   "not a valid stem, so it does not load",
		"projects/in-the-way":         "a file where the new folder would go",
		"topics/dup/dup.md":           fm + "\nOne. ^m-dupdup\n\nTwo. ^m-dupdup\n",
		"topics/dup/other/other.md":   fm + "\nElsewhere. ^m-twice1\n",
		"topics/dup/second/second.md": fm + "\nAgain. ^m-twice1\n",
	})
	before := treeState(t, dir)
	w := writerFor(t, dir)
	with := func(change func(*SplitRequest)) SplitRequest {
		req := cutoverSplit()
		change(&req)
		return req
	}
	for name, c := range map[string]struct {
		req   SplitRequest
		field string
		words []string
	}{
		"the root as the stem":           {with(func(r *SplitRequest) { r.Stem = RootParent }), "stem", []string{"root"}},
		"the root as the new stem":       {with(func(r *SplitRequest) { r.New = RootParent }), "into", []string{"root"}},
		"an invalid new stem":            {with(func(r *SplitRequest) { r.New = "Atlas_Cutover" }), "into", []string{"Atlas_Cutover", "not a valid stem"}},
		"no parent":                      {with(func(r *SplitRequest) { r.Parent = "" }), "parent", []string{"required"}},
		"an empty description":           {with(func(r *SplitRequest) { r.Description = "" }), "description", nil},
		"a two-line description":         {with(func(r *SplitRequest) { r.Description = "a\nb" }), "description", []string{"single line"}},
		"no blocks":                      {with(func(r *SplitRequest) { r.Blocks = nil }), "block", []string{"required"}},
		"an empty block ID":              {with(func(r *SplitRequest) { r.Blocks = []string{""} }), "block", nil},
		"the same block twice":           {with(func(r *SplitRequest) { r.Blocks = []string{"m-ppppp2", "m-ppppp2"} }), "block", []string{"m-ppppp2", "twice"}},
		"a block of another node":        {with(func(r *SplitRequest) { r.Blocks = []string{"m-ppppp2", "m-ooooo1"} }), "block", []string{"m-ooooo1", "priya"}},
		"a block nobody has":             {with(func(r *SplitRequest) { r.Blocks = []string{"m-zzzzzz"} }), "block", []string{"m-zzzzzz"}},
		"an ID two blocks of it carry":   {with(func(r *SplitRequest) { r.Stem, r.Blocks = "dup", []string{"m-dupdup"} }), "block", []string{"m-dupdup", "more than one"}},
		"an ID another node carries too": {with(func(r *SplitRequest) { r.Stem, r.Blocks = "other", []string{"m-twice1"} }), "block", []string{"m-twice1", "more than one", "second"}},
		"a stem in use":                  {with(func(r *SplitRequest) { r.New = "rules" }), "into", []string{"rules", "in use", "topics/rules"}},
		"the node's own stem":            {with(func(r *SplitRequest) { r.New = "priya" }), "into", []string{"in use"}},
		"a stem a skipped folder has":    {with(func(r *SplitRequest) { r.New = "ghost" }), "into", []string{"ghost", "topics/ghost", "missing frontmatter"}},
		"a stem differing by case":       {with(func(r *SplitRequest) { r.New = "up-case" }), "into", []string{"letter case", "topics/Up-Case"}},
		"an entry in the way":            {with(func(r *SplitRequest) { r.New = "in-the-way" }), "into", []string{"projects/in-the-way", "already exists"}},
	} {
		_, err := w.Split(c.req)
		wantFieldRefusal(t, err, c.field, c.words...)
		if t.Failed() {
			t.Fatalf("%s: %v", name, err)
		}
	}
	var none *NoNodeError
	if _, err := w.Split(with(func(r *SplitRequest) { r.Stem = "nowhere" })); !errors.As(err, &none) || none.Stem != "nowhere" || len(none.Stems) == 0 {
		t.Errorf("unknown stem: %v, want a NoNodeError for nowhere with the loaded stems", err)
	}
	if _, err := w.Split(with(func(r *SplitRequest) { r.Parent = "nowhere" })); !errors.As(err, &none) || none.Stem != "nowhere" {
		t.Errorf("unknown parent: %v, want a NoNodeError for nowhere", err)
	}
	// The new node is no parent for itself: it does not exist yet.
	if _, err := w.Split(with(func(r *SplitRequest) { r.Parent = "cutover" })); !errors.As(err, &none) || none.Stem != "cutover" {
		t.Errorf("the new stem as its own parent: %v, want a NoNodeError for cutover", err)
	}
	wantTree(t, dir, before)
}

// What the arguments alone show to be wrong is refused before the store
// is touched: no lock file is made.
func TestSplit_ArgumentsRefusedBeforeTheLock(t *testing.T) {
	dir := smTree(t, nil)
	req := cutoverSplit()
	req.Blocks = []string{"m-ppppp2", "m-ppppp2"}
	_, err := writerFor(t, dir).Split(req)
	wantFieldRefusal(t, err, "block", "twice")
	if _, err := os.Lstat(filepath.Join(dir, lockName)); err == nil {
		t.Error("a refused split took the lock")
	}
}

// failRenameTo makes the first rename whose destination ends in suffix
// fail, and counts the renames that ran before it.
func failRenameTo(t *testing.T, suffix string) (ran *int) {
	t.Helper()
	ran = new(int)
	failed := false
	withHook(t, &renameFile, func(from, to string) error {
		if !failed && strings.HasSuffix(to, suffix) {
			failed = true
			return errors.New("injected rename failure")
		}
		if !failed {
			*ran++
		}
		return os.Rename(from, to)
	})
	return ran
}

// All or nothing: whichever step fails - writing the new note, replacing
// the old note or a linking note, or the reload showing a new error - the
// replaced notes are restored and no folder for the new node is left.
func TestSplit_AllOrNothing(t *testing.T) {
	// The new note is written first, then notes are replaced in path
	// order: priya, rules.
	for name, c := range map[string]struct {
		dest string
		ran  int
	}{
		"writing the new note":     {"cutover.md", 0},
		"replacing the old note":   {"priya.md", 1},
		"replacing a linking note": {"rules.md", 2},
	} {
		t.Run(name, func(t *testing.T) {
			dir := smTree(t, nil)
			before := treeState(t, dir)
			ran := failRenameTo(t, c.dest)
			_, err := writerFor(t, dir).Split(cutoverSplit())
			wantRefused(t, err, "injected rename failure")
			if *ran != c.ran {
				t.Fatalf("%d renames ran before the failure, want %d", *ran, c.ran)
			}
			wantTree(t, dir, before)
		})
	}
	t.Run("the reload shows a new error", func(t *testing.T) {
		dir := smTree(t, nil)
		before := treeState(t, dir)
		var seen bool
		withHook(t, &testHookReadBack, func(s *Store, rel string) {
			_, arrived := s.Block("cutover", "m-ppppp2")
			_, left := s.Block("priya", "m-ppppp2")
			seen = arrived && !left && rel == "projects/cutover"
			s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "topics", Reason: "injected failure"})
		})
		_, err := writerFor(t, dir).Split(cutoverSplit())
		wantRefused(t, err, "injected failure", "rolled back")
		if !seen {
			t.Error("the reload did not see the split: the roll back was not exercised")
		}
		wantTree(t, dir, before)
	})
	t.Run("the new node did not load", func(t *testing.T) {
		dir := smTree(t, nil)
		before := treeState(t, dir)
		withHook(t, &testHookReadBack, func(s *Store, _ string) { delete(s.byStem, "cutover") })
		_, err := writerFor(t, dir).Split(cutoverSplit())
		wantRefused(t, err, "did not load", "rolled back")
		wantTree(t, dir, before)
	})
	t.Run("a note changed on disk before it was replaced", func(t *testing.T) {
		dir := smTree(t, nil)
		edited := smRules + "\nAdded in an editor. ^m-ooooo3\n"
		want := treeState(t, dir)
		want["topics/rules/rules.md"] = edited
		withHook(t, &testHookBeforeRename, func(notePath string) {
			if strings.HasSuffix(notePath, "rules.md") {
				writeFile(t, notePath, edited)
			}
		})
		_, err := writerFor(t, dir).Split(cutoverSplit())
		if !errors.Is(err, ErrChangedOnDisk) {
			t.Fatalf("err = %v, want ErrChangedOnDisk", err)
		}
		// The other writer's bytes are kept; everything else is as it was.
		wantTree(t, dir, want)
	})
}

// A folder that did not load before the call and does not load after it
// is not a new error.
func TestSplit_UnloadedFolderIsNotNew(t *testing.T) {
	dir := smTree(t, map[string]string{"projects/Broken/readme.txt": "a folder that does not load"})
	mustSplit(t, dir, cutoverSplit())
	if _, ok := Load(dir).Block("cutover", "m-ppppp2"); !ok {
		t.Error("the block did not arrive")
	}
}

// wantLocked checks that call reports busy and writes nothing while
// another holder has the lock, and that once it is released the call
// holds the lock from its load to its reload and then lets go.
func wantLocked(t *testing.T, dir string, call func() error) {
	t.Helper()
	release := holdLock(t, dir)
	before := treeState(t, dir)
	withHook(t, &lockWait, 150*time.Millisecond)
	if err := call(); !errors.Is(err, ErrBusy) {
		t.Errorf("while locked: %v, want ErrBusy", err)
	}
	wantTree(t, dir, before)
	release()
	var atLoad, atReload bool
	withHook(t, &testHookLocked, func() { atLoad = lockHeld(t, dir) })
	withHook(t, &testHookReadBack, func(*Store, string) { atReload = lockHeld(t, dir) })
	if err := call(); err != nil {
		t.Fatalf("once released: %v", err)
	}
	if !atLoad || !atReload {
		t.Errorf("lock held at load: %v, at reload: %v", atLoad, atReload)
	}
	if lockHeld(t, dir) {
		t.Error("the lock is still held after the call")
	}
}

func TestSplit_Lock(t *testing.T) {
	dir := smTree(t, nil)
	wantLocked(t, dir, func() error { _, err := writerFor(t, dir).Split(cutoverSplit()); return err })
}

// The folded-in node's blocks arrive at the end of the kept note, in file
// order and as they were; its children and everything else in its folder
// move under the kept node; every link to it is retargeted; the kept node
// gains the tags it lacked and keeps its description and properties; the
// folded-in node's folder is removed. The kept note, having gained blocks
// and a tag, is dated with the time of the call, modified last among its
// re-emitted keys; the child, only relinked and without a date, is given
// its file's (III.9).
func TestMerge_FoldsBlocksChildrenLinksAndTags(t *testing.T) {
	atCallTime(t)
	dir := smTree(t, map[string]string{
		"topics/rules/notes.txt":        "a stray file\n",
		"topics/rules/Broken/broken.md": "a folder that does not load\n",
		"topics/rules/.DS_Store":        "a dot-entry: it goes with the folder",
	})
	want := treeState(t, dir)
	problemsBefore := len(Load(dir).Problems())
	ageNotes(t, dir)
	res := mustMerge(t, dir, "rules", "etiquette")
	if wantRes := (MergeResult{Blocks: 3, Children: 1, TagsAdded: 1, Links: 3}); res != wantRes {
		t.Errorf("result %+v, want %+v", res, wantRes)
	}
	delete(want, "topics/rules/rules.md")
	delete(want, "topics/rules/.DS_Store")
	want = moved(want, "topics/rules", "topics/etiquette")
	want["topics/etiquette"] = "dir"
	want["topics/etiquette/etiquette.md"] = stamped(smEtiquetteAfter, callText)
	want["topics/etiquette/archive/archive.md"] = stamped(smArchiveAfter, agedText)
	wantTree(t, dir, want)
	for _, rel := range []string{"people/priya/priya.md", "topics/misc/misc.md", "topics/topics.md", "topics/etiquette/notes.txt", "topics/etiquette/Broken/broken.md"} {
		wantAged(t, dir, rel)
	}
	noTempFiles(t, dir)
	if got := len(Load(dir).Problems()); got != problemsBefore {
		t.Errorf("%d problems before, %d after:%s", problemsBefore, got, dump(Load(dir).Problems()))
	}
}

// After a merge the kept node has its own description and properties, the
// union of the tags, its blocks and then the folded-in node's (no
// hand-written paragraph is given an ID), and the folded-in node's
// children; links that named the folded-in node reach the kept one.
func TestMerge_KeptNodeAsLoaded(t *testing.T) {
	dir := smTree(t, nil)
	mustMerge(t, dir, "rules", "etiquette")
	st := wantNoProblems(t, dir)
	wantAbsent(t, st, "rules")
	kept := mustNode(t, st, "etiquette")
	if kept.Description != "Etiquette" || !reflect.DeepEqual(kept.Tags, []string{"outbox", "etiquette", "atlas"}) {
		t.Errorf("the kept node: description %q, tags %v", kept.Description, kept.Tags)
	}
	if v, ok := kept.Property("kind"); !ok || v != "guide" {
		t.Errorf("the kept node's property kind is %v (%v)", v, ok)
	}
	if ids, want := blockIDsOf(kept), []string{"m-eeeee1", "", "m-ooooo1", "", "m-ooooo2"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("the kept node's block IDs are %q, want %q", ids, want)
	}
	if a := mustNode(t, st, "archive"); a.Path != "topics/etiquette/archive" || a.Parent() != kept {
		t.Errorf("the child is at %s", a.Path)
	}
	if got, want := st.Backlinks("etiquette"), []Ref{{Stem: "archive", Block: "m-aaaaa1"}, {Stem: "etiquette", Block: "m-eeeee1"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("backlinks of the kept node %+v, want %+v", got, want)
	}
}

// The frontmatter is re-emitted only when a tag is added: with none, its
// bytes stay, hand formatting and comment spacing included, but for the one
// modified line a note that gained blocks is given, last and in the note's
// line ending (III.9); and the blocks are appended in the note's own line
// ending.
func TestMerge_NoTagAddedKeepsFrontmatterBytes(t *testing.T) {
	atCallTime(t)
	const head = "---\r\ndescription:   Kept\r\ntags:   [ outbox,atlas ]\r\nkind: guide    # spaced\r\n---\r\n"
	dir := smTree(t, map[string]string{
		"topics/kept/kept.md": head + "\r\nKept claim. ^m-kkkkk1\r\n",
	})
	res := mustMerge(t, dir, "rules", "kept")
	if res.TagsAdded != 0 || res.Blocks != 3 {
		t.Errorf("result %+v", res)
	}
	const dated = "---\r\ndescription:   Kept\r\ntags:   [ outbox,atlas ]\r\nkind: guide    # spaced\r\nmodified: " + callText + "\r\n---\r\n"
	want := dated + "\r\nKept claim. ^m-kkkkk1\r\n\r\nCc Priya on urgent replies. ^m-ooooo1\r\n\r\nHand-written, no ID.\r\n\r\nDates: [[priya#^m-ppppp2|the date]], [[priya#^m-ppppp3]], `[[priya#^m-ppppp2]]`. ^m-ooooo2\r\n"
	if got := treeState(t, dir)["topics/kept/kept.md"]; got != want {
		t.Errorf("the kept note is\n%q\nwant\n%q", got, want)
	}
}

// A merge that gives the kept node nothing - no block, no tag, no link to
// retarget - does not write its note.
func TestMerge_NothingToFoldLeavesTheKeptNoteAlone(t *testing.T) {
	dir := smTree(t, map[string]string{"topics/empty/empty.md": fm})
	ageNotes(t, dir)
	res := mustMerge(t, dir, "empty", "misc")
	if res != (MergeResult{}) {
		t.Errorf("result %+v, want all zero", res)
	}
	wantAged(t, dir, "topics/misc/misc.md")
	wantAbsent(t, Load(dir), "empty")
	if _, err := os.Lstat(filepath.Join(dir, "topics", "empty")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the folded-in folder is still there: %v", err)
	}
}

// A child merges up into its parent like any other merge: its own
// children land beside it, and its link to the parent, which names no
// folded-in node, moves with its text and is the parent's link to itself.
// The parent, having gained a block, is dated with the time of the call
// (III.9).
func TestMerge_ChildIntoItsParent(t *testing.T) {
	atCallTime(t)
	dir := smTree(t, map[string]string{"topics/rules/archive/older/older.md": fm + "\nOlder still. ^m-lllll1\n"})
	res := mustMerge(t, dir, "archive", "rules")
	if want := (MergeResult{Blocks: 1, Children: 1, TagsAdded: 0, Links: 0}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	st := wantNoProblems(t, dir)
	wantAbsent(t, st, "archive")
	if n := mustNode(t, st, "older"); n.Path != "topics/rules/older" {
		t.Errorf("the grandchild is at %s", n.Path)
	}
	if got := treeState(t, dir)["topics/rules/rules.md"]; got != stamped(smRules+"\nOld rules, replaced by [[rules|the rules]]. ^m-aaaaa1\n", callText) {
		t.Errorf("the parent's note is %q", got)
	}
	noTempFiles(t, dir)
}

// Every refusal names its field and changes nothing.
func TestMerge_Refusals(t *testing.T) {
	dir := smTree(t, map[string]string{
		"topics/rules/notes.txt":     "rules' stray file\n",
		"topics/etiquette/notes.txt": "etiquette's own\n",
		"topics/misc/archive":        "a file with the name of rules' child",
	})
	before := treeState(t, dir)
	w := writerFor(t, dir)
	_, err := w.Merge("rules", "rules")
	wantFieldRefusal(t, err, "into", "rules", "being merged")
	_, err = w.Merge("rules", "archive")
	wantFieldRefusal(t, err, "into", "archive", "descendant", "merge \"archive\" into \"rules\"")
	_, err = w.Merge("rules", "etiquette")
	wantFieldRefusal(t, err, "into", "notes.txt", "etiquette")
	_, err = w.Merge("rules", "misc")
	wantFieldRefusal(t, err, "into", `"archive"`, "misc")
	var none *NoNodeError
	if _, err := w.Merge("nowhere", "priya"); !errors.As(err, &none) || none.Stem != "nowhere" || len(none.Stems) == 0 {
		t.Errorf("unknown from: %v, want a NoNodeError for nowhere with the loaded stems", err)
	}
	if _, err := w.Merge("priya", "nowhere"); !errors.As(err, &none) || none.Stem != "nowhere" {
		t.Errorf("unknown to: %v, want a NoNodeError for nowhere", err)
	}
	wantTree(t, dir, before)
}

// The root is refused before the store is touched: no lock file is made.
func TestMerge_RootRefusedBeforeTheLock(t *testing.T) {
	dir := smTree(t, nil)
	w := writerFor(t, dir)
	_, err := w.Merge(RootParent, "priya")
	wantFieldRefusal(t, err, "from", "root")
	_, err = w.Merge("priya", RootParent)
	wantFieldRefusal(t, err, "into", "root")
	if _, err := os.Lstat(filepath.Join(dir, lockName)); err == nil {
		t.Error("a refused merge took the lock")
	}
}

// All or nothing: whichever step fails - replacing the kept note or a
// linking note, moving a child or a stray file, renaming the folder
// aside, or the reload showing a new error - every replaced note is
// restored and every entry is back where it was.
func TestMerge_AllOrNothing(t *testing.T) {
	extra := map[string]string{"topics/rules/notes.txt": "a stray file\n"}
	// Notes are replaced in path order (etiquette, archive), then the
	// entries move in name order (archive, notes.txt), then the folder is
	// renamed aside.
	for name, c := range map[string]struct {
		dest string
		ran  int
	}{
		"replacing the kept note":   {"etiquette.md", 0},
		"replacing a linking note":  {"archive.md", 1},
		"moving a child":            {"topics/etiquette/archive", 2},
		"moving a stray file":       {"topics/etiquette/notes.txt", 3},
		"renaming the folder aside": {".rules.deleting", 4},
	} {
		t.Run(name, func(t *testing.T) {
			dir := smTree(t, extra)
			before := treeState(t, dir)
			ran := failRenameTo(t, filepath.FromSlash(c.dest))
			_, err := writerFor(t, dir).Merge("rules", "etiquette")
			wantRefused(t, err, "injected rename failure")
			if *ran != c.ran {
				t.Fatalf("%d renames ran before the failure, want %d", *ran, c.ran)
			}
			wantTree(t, dir, before)
		})
	}
}

// All or nothing at the reload, and D28's edit-race checks: a note an
// outside editor changed is ErrChangedOnDisk, and nothing else changes.
func TestMerge_ReloadAndOutsideEdits(t *testing.T) {
	extra := map[string]string{"topics/rules/notes.txt": "a stray file\n"}
	t.Run("the reload shows a new error", func(t *testing.T) {
		dir := smTree(t, extra)
		before := treeState(t, dir)
		var seen bool
		withHook(t, &testHookReadBack, func(s *Store, rel string) {
			_, arrived := s.Block("etiquette", "m-ooooo1")
			_, still := s.Node("rules")
			_, childMoved := s.Block("archive", "m-aaaaa1")
			seen = arrived && !still && childMoved && rel == "topics/etiquette"
			s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "people", Reason: "injected failure"})
		})
		_, err := writerFor(t, dir).Merge("rules", "etiquette")
		wantRefused(t, err, "injected failure", "rolled back")
		if !seen {
			t.Error("the reload did not see the merge: the roll back was not exercised")
		}
		wantTree(t, dir, before)
	})
	for name, c := range map[string]struct{ rel, note string }{
		"the folded-in note changed on disk":                    {"topics/rules/rules.md", smRules},
		"a linking note changed on disk before it was replaced": {"topics/rules/archive/archive.md", smArchive},
	} {
		t.Run(name, func(t *testing.T) {
			dir := smTree(t, extra)
			edited := c.note + "\nAdded in an editor. ^m-edited\n"
			want := treeState(t, dir)
			want[c.rel] = edited
			withHook(t, &testHookBeforeRename, func(notePath string) {
				if strings.HasSuffix(notePath, filepath.FromSlash(c.rel)) {
					writeFile(t, notePath, edited)
				}
			})
			_, err := writerFor(t, dir).Merge("rules", "etiquette")
			if !errors.Is(err, ErrChangedOnDisk) {
				t.Fatalf("err = %v, want ErrChangedOnDisk", err)
			}
			// The other writer's bytes are kept; everything else is as it was.
			wantTree(t, dir, want)
		})
	}
}

// A leftover from an interrupted delete or merge does not block the next
// merge, and is not taken for the folder to remove.
func TestMerge_LeftoverHiddenFolder(t *testing.T) {
	dir := smTree(t, map[string]string{"topics/.rules.deleting/x.md": "stale"})
	mustMerge(t, dir, "rules", "etiquette")
	if got := treeState(t, dir)["topics/.rules.deleting/x.md"]; got != "stale" {
		t.Errorf("the stale leftover is %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "topics", "rules")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rules is still there: %v", err)
	}
}

func TestMerge_Lock(t *testing.T) {
	dir := smTree(t, nil)
	wantLocked(t, dir, func() error { _, err := writerFor(t, dir).Merge("rules", "etiquette"); return err })
}

// Split and merge undo each other: the blocks come back, last, and the
// links follow them both ways. priya, changed by both calls, is dated
// with their time; rules, only relinked and back, is as it was but for
// the date the split took from its file (III.9).
func TestSplitThenMerge_RoundTrip(t *testing.T) {
	atCallTime(t)
	dir := smTree(t, nil)
	ageNotes(t, dir)
	mustSplit(t, dir, cutoverSplit())
	res := mustMerge(t, dir, "cutover", "priya")
	if want := (MergeResult{Blocks: 2, Children: 0, TagsAdded: 0, Links: 3}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	want := "---\ndescription: Priya\ntags: [platform, atlas]\nmodified: " + callText + "\n---\n\nPriya leads platform. ^m-ppppp1\n\nPrefers async updates. ^m-ppppp4\n\n" + smP3 + " ^m-ppppp3\n\nAtlas cutover is 2026-12-01. ^m-ppppp2\n"
	state := treeState(t, dir)
	if got := state["people/priya/priya.md"]; got != want {
		t.Errorf("priya.md is\n%q\nwant\n%q", got, want)
	}
	if got, want := state["topics/rules/rules.md"], stamped(smRules, agedText); got != want {
		t.Errorf("rules.md is\n%q\nwant it as it was, dated from its file\n%q", got, want)
	}
	if _, there := state["projects/cutover"]; there {
		t.Error("the split-off node is still there")
	}
	wantNoProblems(t, dir)
}
