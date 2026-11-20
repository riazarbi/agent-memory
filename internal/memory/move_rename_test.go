//go:build unix

package memory

// CLI12: Writer.Move and Writer.Rename (docs/spec/agent-memory.md Part
// III.8, "move, exactly" and "rename, exactly").

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The atlas fixture's notes. atlas links to itself and to its child;
// runbook and priya link to atlas in every edge form; priya also holds a
// link in inline code, a dangling link and an unsupported form, none of
// which is an edge to atlas.
const (
	atlasNote   = fm + "\nCutover is 2026-12-01. ^m-aaaaa1\n\nRollback plan is in [[atlas#^m-aaaaa1]] and [[runbook]]. ^m-aaaaa2\n"
	runbookNote = fm + "\nSteps for [[atlas|the cutover]]. ^m-bbbbb1\n"
	priyaNote   = fm + "\nLeads [[atlas]]; see [[atlas#^m-aaaaa1|the date]]. ^m-ppppp1\n\n" +
		"Wrote `[[atlas]]` in the wiki, [[atlas#Plan]] is a heading and [[go-live]] is planned. ^m-ppppp2\n"
	miscNote = fm + "\nNothing about Atlas here. ^m-nnnnn1\n"
)

// atlasTree is the move and rename fixture: people holds atlas (with the
// child runbook) and priya; projects and topics/misc stand by.
func atlasTree(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"people/people.md":                fm,
		"projects/projects.md":            fm,
		"topics/topics.md":                fm,
		"people/atlas/atlas.md":           atlasNote,
		"people/atlas/runbook/runbook.md": runbookNote,
		"people/priya/priya.md":           priyaNote,
		"topics/misc/misc.md":             miscNote,
	}
	for k, v := range extra {
		files[k] = v
	}
	return writeTree(t, files)
}

// ageNotes sets every mtime under dir into the past, so a rewrite shows.
func ageNotes(t *testing.T, dir string) {
	t.Helper()
	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	err := filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, past, past) //nolint:gosec // a test temp dir
	})
	if err != nil {
		t.Fatal(err)
	}
}

// wantAged fails unless the file at rel still has the mtime ageNotes gave
// it: it was not rewritten.
func wantAged(t *testing.T, dir, rel string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Year() != 2020 {
		t.Errorf("%s was rewritten: mtime %v", rel, info.ModTime())
	}
}

// moved returns state with every entry at or under from re-rooted at to.
func moved(state map[string]string, from, to string) map[string]string {
	out := map[string]string{}
	for k, v := range state {
		switch {
		case k == from:
			k = to
		case strings.HasPrefix(k, from+"/"):
			k = to + k[len(from):]
		}
		out[k] = v
	}
	return out
}

// Move is one directory rename: the subtree arrives whole, with whatever
// else the folder held, and no file is rewritten.
func TestMove_RelocatesSubtree(t *testing.T) {
	dir := atlasTree(t, map[string]string{
		"people/atlas/stray.txt":         "not a node",
		"people/atlas/Broken/readme.txt": "a folder that does not load",
	})
	before := treeState(t, dir)
	ageNotes(t, dir)
	res, err := writerFor(t, dir).Move("atlas", "projects")
	if err != nil {
		t.Fatal(err)
	}
	if want := (MoveResult{From: "people", Parent: "projects", Path: "projects/atlas"}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	wantTree(t, dir, moved(before, "people/atlas", "projects/atlas"))
	for rel, content := range moved(before, "people/atlas", "projects/atlas") {
		if content != "dir" {
			wantAged(t, dir, rel)
		}
	}
	st := Load(dir)
	if n := mustNode(t, st, "runbook"); n.Path != "projects/atlas/runbook" {
		t.Errorf("runbook is at %s", n.Path)
	}
	wantNoDangling(t, st, "atlas", "priya", "runbook")
}

// wantNoDangling fails if any link to the node target, in the nodes
// holders, dangles.
func wantNoDangling(t *testing.T, st *Store, target string, holders ...string) {
	t.Helper()
	for _, h := range holders {
		for _, e := range mustNode(t, st, h).Edges() {
			if e.Stem == target && e.Dangling {
				t.Errorf("%s's link %+v dangles", h, e)
			}
		}
	}
}

// The root is a parent like any other, written "/"; a move to the parent
// the node already has renames nothing.
func TestMove_RootAndNoOp(t *testing.T) {
	dir := atlasTree(t, nil)
	w := writerFor(t, dir)
	start := treeState(t, dir)
	res, err := w.Move("atlas", RootParent)
	if err != nil {
		t.Fatal(err)
	}
	if want := (MoveResult{From: "people", Parent: "/", Path: "atlas"}); res != want {
		t.Errorf("to the root: %+v, want %+v", res, want)
	}
	wantTree(t, dir, moved(start, "people/atlas", "atlas"))
	if n := mustNode(t, Load(dir), "atlas"); n.Parent() != nil {
		t.Errorf("atlas still has the parent %s", n.Parent().Stem)
	}

	for _, parent := range []string{RootParent, "people", "people"} {
		renames := 0
		withHook(t, &renameFile, func(from, to string) error {
			renames++
			return os.Rename(from, to)
		})
		st := mustNode(t, Load(dir), "atlas")
		from := RootParent
		if p := st.Parent(); p != nil {
			from = p.Stem
		}
		res, err := w.Move("atlas", parent)
		if err != nil {
			t.Fatal(err)
		}
		wantPath := "atlas"
		if parent != RootParent {
			wantPath = parent + "/atlas"
		}
		if want := (MoveResult{From: from, Parent: parent, Path: wantPath}); res != want {
			t.Errorf("move to %s: %+v, want %+v", parent, res, want)
		}
		if wantRenames := map[bool]int{true: 0, false: 1}[from == parent]; renames != wantRenames {
			t.Errorf("move from %s to %s made %d renames, want %d", from, parent, renames, wantRenames)
		}
	}
	wantTree(t, dir, start)
}

func TestMove_Refusals(t *testing.T) {
	dir := atlasTree(t, map[string]string{"projects/atlas": "a plain file in the way"})
	before := treeState(t, dir)
	w := writerFor(t, dir)

	_, err := w.Move("atlas", "atlas")
	wantFieldRefusal(t, err, "parent", "itself")
	_, err = w.Move("atlas", "runbook")
	wantFieldRefusal(t, err, "parent", "runbook", "under")
	_, err = w.Move("people", "runbook")
	wantFieldRefusal(t, err, "parent", "runbook", "under")
	_, err = w.Move("atlas", "projects")
	wantFieldRefusal(t, err, "parent", "already", "atlas")

	var none *NoNodeError
	if _, err = w.Move("atlas", "nowhere"); !errors.As(err, &none) || none.Stem != "nowhere" {
		t.Errorf("unknown parent: %v, want a NoNodeError for nowhere", err)
	}
	if _, err = w.Move("nowhere", "people"); !errors.As(err, &none) || none.Stem != "nowhere" || len(none.Stems) == 0 {
		t.Errorf("unknown stem: %v, want a NoNodeError for nowhere with the loaded stems", err)
	}
	if _, err = w.Move(RootParent, "people"); !errors.As(err, &none) {
		t.Errorf("moving the root: %v, want a NoNodeError", err)
	}
	wantTree(t, dir, before)
}

// A reload that shows an error the store did not have renames the folder
// back; a rename that fails changes nothing.
func TestMove_FailureLeavesStoreAsItWas(t *testing.T) {
	t.Run("the reload shows a new error", func(t *testing.T) {
		dir := atlasTree(t, nil)
		before := treeState(t, dir)
		var seen bool
		withHook(t, &testHookReadBack, func(s *Store, _ string) {
			n, ok := s.Node("atlas")
			seen = ok && n.Path == "projects/atlas"
			s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "topics", Reason: "injected failure"})
		})
		_, err := writerFor(t, dir).Move("atlas", "projects")
		wantRefused(t, err, "injected failure", "rolled back")
		if !seen {
			t.Error("the reload did not see the move: the roll back was not exercised")
		}
		wantTree(t, dir, before)
	})
	t.Run("the rename fails", func(t *testing.T) {
		dir := atlasTree(t, nil)
		before := treeState(t, dir)
		withHook(t, &renameFile, func(string, string) error { return errors.New("injected rename failure") })
		_, err := writerFor(t, dir).Move("atlas", "projects")
		wantRefused(t, err, "injected rename failure")
		wantTree(t, dir, before)
	})
}

// A real new error: moving a node so that a skipped folder with the same
// stem now comes first in path order would take the stem from it.
func TestMove_RealCollisionRollsBack(t *testing.T) {
	dir := atlasTree(t, map[string]string{"topics/atlas/atlas.md": fm + "\nThe loser of the stem. ^m-ccccc1\n"})
	before := treeState(t, dir)
	// people/atlas holds the stem; topics/atlas is the collision error.
	// Under topics/misc the first would sort after the second and lose it.
	_, err := writerFor(t, dir).Move("atlas", "misc")
	wantRefused(t, err, "new error", "rolled back")
	wantTree(t, dir, before)
}

// renamedNotes is the atlas fixture's three linking notes after atlas is
// renamed to new: every edge to atlas retargeted, nothing else touched.
func renamedNotes(new string) (atlas, runbook, priya string) {
	atlas = fm + "\nCutover is 2026-12-01. ^m-aaaaa1\n\nRollback plan is in [[" + new + "#^m-aaaaa1]] and [[runbook]]. ^m-aaaaa2\n"
	runbook = fm + "\nSteps for [[" + new + "|the cutover]]. ^m-bbbbb1\n"
	priya = fm + "\nLeads [[" + new + "]]; see [[" + new + "#^m-aaaaa1|the date]]. ^m-ppppp1\n\n" +
		"Wrote `[[atlas]]` in the wiki, [[atlas#Plan]] is a heading and [[go-live]] is planned. ^m-ppppp2\n"
	return atlas, runbook, priya
}

// Rename renames the folder and its note where they are and rewrites the
// target of every edge to the node, the node's own included; only those
// bytes change, and notes holding no such link are not written.
func TestRename_RewritesEveryLinkForm(t *testing.T) {
	dir := atlasTree(t, nil)
	ageNotes(t, dir)
	res, err := writerFor(t, dir).Rename("atlas", "cutover")
	if err != nil {
		t.Fatal(err)
	}
	if want := (RenameResult{Old: "atlas", New: "cutover", Links: 4}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	atlas, runbook, priya := renamedNotes("cutover")
	wantTree(t, dir, map[string]string{
		".": "dir", "people": "dir", "projects": "dir", "topics": "dir", "topics/misc": "dir",
		"people/cutover": "dir", "people/cutover/runbook": "dir", "people/priya": "dir",
		"people/people.md":                  fm,
		"projects/projects.md":              fm,
		"topics/topics.md":                  fm,
		"people/cutover/cutover.md":         atlas,
		"people/cutover/runbook/runbook.md": runbook,
		"people/priya/priya.md":             priya,
		"topics/misc/misc.md":               miscNote,
	})
	for _, rel := range []string{"people/people.md", "projects/projects.md", "topics/topics.md", "topics/misc/misc.md"} {
		wantAged(t, dir, rel)
	}
	noTempFiles(t, dir)
	st := Load(dir)
	if _, ok := st.Node("atlas"); ok {
		t.Error("the old stem still loads")
	}
	n := mustNode(t, st, "cutover")
	if n.Path != "people/cutover" || len(n.Blocks) != 2 || n.Blocks[0].ID != "m-aaaaa1" {
		t.Errorf("renamed node: path %s, blocks %+v", n.Path, n.Blocks)
	}
	wantNoDangling(t, st, "cutover", "priya", "runbook", "cutover")
}

// A node whose own note holds no link to itself is renamed, not rewritten.
func TestRename_NoLinksWritesNoNote(t *testing.T) {
	dir := atlasTree(t, nil)
	ageNotes(t, dir)
	res, err := writerFor(t, dir).Rename("misc", "sundries")
	if err != nil {
		t.Fatal(err)
	}
	if res.Links != 0 {
		t.Errorf("links rewritten = %d, want 0", res.Links)
	}
	wantAged(t, dir, "topics/sundries/sundries.md")
	if got := string(fileBytes(t, dir, "topics/sundries/sundries.md")); got != miscNote {
		t.Errorf("the note changed: %q", got)
	}
	for _, rel := range []string{"people/atlas/atlas.md", "people/priya/priya.md"} {
		wantAged(t, dir, rel)
	}
}

// Only link targets change: CRLF endings, a byte-order mark, an ID-less
// paragraph, a link on a block's second line, a run of blank lines and a
// missing final newline all survive. Two links in one block are two.
func TestRename_KeepsEveryOtherByte(t *testing.T) {
	src := bomText + "---\r\ndescription: d\r\nkind: hub  # kept\r\n---\r\n\r\n\r\n" +
		"First line\r\nthen [[old]] and [[old|again]]. ^m-aaaaa1\r\n\r\n\r\n" +
		"No ID here, see [[old#^m-aaaaa1]].\r\n\r\n" +
		"Untouched [[other]] and `[[old]]`.   ^m-aaaaa3"
	dir := writeTree(t, map[string]string{"old/old.md": src, "other/other.md": fm})
	res, err := writerFor(t, dir).Rename("old", "brand-new")
	if err != nil {
		t.Fatal(err)
	}
	if res.Links != 3 {
		t.Errorf("links rewritten = %d, want 3", res.Links)
	}
	want := bomText + "---\r\ndescription: d\r\nkind: hub  # kept\r\n---\r\n\r\n\r\n" +
		"First line\r\nthen [[brand-new]] and [[brand-new|again]]. ^m-aaaaa1\r\n\r\n\r\n" +
		"No ID here, see [[brand-new#^m-aaaaa1]].\r\n\r\n" +
		"Untouched [[other]] and `[[old]]`.   ^m-aaaaa3"
	if got := string(fileBytes(t, dir, "brand-new/brand-new.md")); got != want {
		t.Errorf("note after the rename:\n got %q\nwant %q", got, want)
	}
}

// A folder inside the node that did not load before the rename goes along
// and is the same error at its new path, not a new one: the rename is not
// rolled back.
func TestRename_CarriesUnloadedFolder(t *testing.T) {
	dir := atlasTree(t, map[string]string{"people/atlas/Broken/readme.txt": "a folder that does not load"})
	if _, err := writerFor(t, dir).Rename("atlas", "cutover"); err != nil {
		t.Fatal(err)
	}
	wantProblems(t, Load(dir),
		pk{SeverityError, "people/cutover/Broken"},
		pk{SeverityWarning, "people/priya"}, pk{SeverityWarning, "people/priya"})
	if got := string(fileBytes(t, dir, "people/cutover/Broken/readme.txt")); got != "a folder that does not load" {
		t.Errorf("the unloaded folder's file: %q", got)
	}
}

// A link [[NEW]] that dangled before the call resolves afterwards and is
// not counted: it was not rewritten.
func TestRename_DanglingLinkToNewStemResolves(t *testing.T) {
	dir := atlasTree(t, nil)
	res, err := writerFor(t, dir).Rename("atlas", "go-live")
	if err != nil {
		t.Fatal(err)
	}
	if res.Links != 4 {
		t.Errorf("links rewritten = %d, want 4 (the dangling [[go-live]] is not rewritten)", res.Links)
	}
	_, _, priya := renamedNotes("go-live")
	if got := string(fileBytes(t, dir, "people/priya/priya.md")); got != priya {
		t.Errorf("priya after the rename: %q", got)
	}
	for _, e := range mustNode(t, Load(dir), "priya").Edges() {
		if e.Dangling {
			t.Errorf("priya's link %+v still dangles", e)
		}
	}
}

func TestRename_Refusals(t *testing.T) {
	dir := atlasTree(t, map[string]string{
		"people/in-the-way":          "a plain file beside atlas",
		"people/atlas/note-taken.md": "a stray file inside atlas",
	})
	before := treeState(t, dir)
	w := writerFor(t, dir)
	for _, bad := range []string{"Atlas_Go", "", "/", "a--b", "-a", "a/b", strings.Repeat("a", 65)} {
		_, err := w.Rename("atlas", bad)
		wantFieldRefusal(t, err, "new", "not a valid stem")
	}
	_, err := w.Rename("atlas", "priya")
	wantFieldRefusal(t, err, "new", "priya", "in use")
	_, err = w.Rename("atlas", "runbook")
	wantFieldRefusal(t, err, "new", "runbook", "in use")
	_, err = w.Rename("atlas", "atlas")
	wantFieldRefusal(t, err, "new", "already")
	_, err = w.Rename("atlas", "in-the-way")
	wantFieldRefusal(t, err, "new", "in-the-way", "already")
	_, err = w.Rename("atlas", "note-taken")
	wantFieldRefusal(t, err, "new", "note-taken.md", "already")
	var none *NoNodeError
	if _, err = w.Rename("nowhere", "x-y"); !errors.As(err, &none) || none.Stem != "nowhere" || len(none.Stems) == 0 {
		t.Errorf("unknown stem: %v, want a NoNodeError for nowhere with the loaded stems", err)
	}
	wantTree(t, dir, before)
}

// All or nothing: whichever step fails - replacing a note, renaming the
// folder note, renaming the folder, or the reload - every rewritten note
// is restored and the folder and note keep their old names.
func TestRename_AllOrNothing(t *testing.T) {
	failing := func(match func(from, to string) bool, ran *int) func(string, string) error {
		return func(from, to string) error {
			if match(from, to) {
				return errors.New("injected rename failure")
			}
			*ran++
			return os.Rename(from, to)
		}
	}
	for name, match := range map[string]func(from, to string) bool{
		"the last note cannot be replaced":  func(_, to string) bool { return strings.HasSuffix(to, "priya.md") },
		"the folder note cannot be renamed": func(_, to string) bool { return strings.HasSuffix(to, "cutover.md") },
		"the folder cannot be renamed":      func(_, to string) bool { return strings.HasSuffix(to, "cutover") },
	} {
		t.Run(name, func(t *testing.T) {
			dir := atlasTree(t, nil)
			before := treeState(t, dir)
			ran := 0
			withHook(t, &renameFile, failing(match, &ran))
			_, err := writerFor(t, dir).Rename("atlas", "cutover")
			wantRefused(t, err, "injected rename failure")
			if ran < 2 {
				t.Fatalf("only %d renames ran before the failure: the roll back was not exercised", ran)
			}
			wantTree(t, dir, before)
		})
	}
	t.Run("the reload shows a new error", func(t *testing.T) {
		dir := atlasTree(t, nil)
		before := treeState(t, dir)
		var seen bool
		withHook(t, &testHookReadBack, func(s *Store, _ string) {
			n, ok := s.Node("cutover")
			seen = ok && n.Path == "people/cutover" && strings.Contains(mustNode(t, s, "priya").Blocks[0].Text, "[[cutover]]")
			s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "topics", Reason: "injected failure"})
		})
		_, err := writerFor(t, dir).Rename("atlas", "cutover")
		wantRefused(t, err, "injected failure", "rolled back")
		if !seen {
			t.Error("the reload did not see the rename: the roll back was not exercised")
		}
		wantTree(t, dir, before)
	})
	t.Run("a note changed on disk before it was replaced", func(t *testing.T) {
		dir := atlasTree(t, nil)
		edited := priyaNote + "\nAdded in an editor. ^m-ppppp3\n"
		want := treeState(t, dir)
		want["people/priya/priya.md"] = edited
		withHook(t, &testHookBeforeRename, func(notePath string) {
			if strings.HasSuffix(notePath, "priya.md") {
				writeFile(t, notePath, edited)
			}
		})
		_, err := writerFor(t, dir).Rename("atlas", "cutover")
		if !errors.Is(err, ErrChangedOnDisk) {
			t.Fatalf("err = %v, want ErrChangedOnDisk", err)
		}
		wantTree(t, dir, want)
	})
}

// Move and Rename hold the write lock from before the load until after the
// reload; while another holder has it they wait, report busy and write
// nothing.
func TestMoveRename_Lock(t *testing.T) {
	dir := atlasTree(t, nil)
	w := writerFor(t, dir)
	calls := map[string]func() error{
		"move":   func() error { _, err := w.Move("atlas", "projects"); return err },
		"rename": func() error { _, err := w.Rename("misc", "sundries"); return err },
	}
	release := holdLock(t, dir)
	before := treeState(t, dir)
	withHook(t, &lockWait, 150*time.Millisecond)
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrBusy) {
			t.Errorf("%s while locked: %v, want ErrBusy", name, err)
		}
	}
	wantTree(t, dir, before)
	release()
	for name, call := range calls {
		var atLoad, atReload bool
		withHook(t, &testHookLocked, func() { atLoad = lockHeld(t, dir) })
		withHook(t, &testHookReadBack, func(*Store, string) { atReload = lockHeld(t, dir) })
		if err := call(); err != nil {
			t.Fatalf("%s once released: %v", name, err)
		}
		if !atLoad || !atReload {
			t.Errorf("%s: lock held at load: %v, at reload: %v", name, atLoad, atReload)
		}
		if lockHeld(t, dir) {
			t.Errorf("the lock is still held after %s", name)
		}
	}
}

// holdLock takes the write lock of notes/ as another holder would, and
// returns its release.
func holdLock(t *testing.T, dir string) (release func()) {
	t.Helper()
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	fd := int(lock.Fd()) //nolint:gosec // a file descriptor fits in an int
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := syscall.Flock(fd, syscall.LOCK_UN); err != nil {
			t.Fatal(err)
		}
	}
}
