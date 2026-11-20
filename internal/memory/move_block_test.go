//go:build unix

package memory

// CLI13: Writer.MoveBlock, and Rename's check of its new stem
// (docs/spec/agent-memory.md Part III.8, "move-block, exactly" and
// "rename, exactly").

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The block fixture's notes. priya holds the block to move, m-ppppp2, and
// a link to it; cutover links to it with an alias, quotes the link in
// inline code, and holds a hand-written paragraph with no ID after three
// blank lines; runbook, cutover's child, links to it; misc does not.
const (
	blockP2      = "Atlas cutover is 2026-12-01; rollback is [[runbook]]."
	mbPriyaNote  = fm + "\nPriya leads platform. ^m-ppppp1\n\n" + blockP2 + " ^m-ppppp2\n\nThe date in [[priya#^m-ppppp2]] is firm. ^m-ppppp3\n"
	mbCutover    = fm + "\nOwner is [[priya]]. ^m-ccccc1\n\n\n\nHand-written, no ID.\n\nSee [[priya#^m-ppppp2|the date]] and `[[priya#^m-ppppp2]]`. ^m-ccccc2\n"
	mbRunbook    = fm + "\nFollows [[priya#^m-ppppp2]]. ^m-rrrrr1\n"
	mbPriyaAfter = fm + "\nPriya leads platform. ^m-ppppp1\n\nThe date in [[cutover#^m-ppppp2]] is firm. ^m-ppppp3\n"
	mbCutAfter   = fm + "\nOwner is [[priya]]. ^m-ccccc1\n\n\n\nHand-written, no ID.\n\nSee [[cutover#^m-ppppp2|the date]] and `[[priya#^m-ppppp2]]`. ^m-ccccc2\n\n" + blockP2 + " ^m-ppppp2\n"
	mbRunAfter   = fm + "\nFollows [[cutover#^m-ppppp2]]. ^m-rrrrr1\n"
)

// blockTree is the move-block fixture.
func blockTree(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"people/people.md":                    fm,
		"projects/projects.md":                fm,
		"topics/topics.md":                    fm,
		"people/priya/priya.md":               mbPriyaNote,
		"projects/cutover/cutover.md":         mbCutover,
		"projects/cutover/runbook/runbook.md": mbRunbook,
		"topics/misc/misc.md":                 miscNote,
	}
	for k, v := range extra {
		files[k] = v
	}
	return writeTree(t, files)
}

func mustMoveBlock(t *testing.T, dir, id, to string) MoveBlockResult {
	t.Helper()
	res, err := writerFor(t, dir).MoveBlock(id, to)
	if err != nil {
		t.Fatalf("move-block %s to %s: %v", id, to, err)
	}
	return res
}

// The block leaves its note as delete --block removes it, arrives as the
// target's last paragraph with its ID and text, and every link to it, and
// nothing else, is retargeted. The target is not tidied: its ID-less
// paragraph gets no ID and its run of blank lines stays. Notes holding no
// link to the block are not written.
func TestMoveBlock_MovesAndRelinks(t *testing.T) {
	dir := blockTree(t, nil)
	want := treeState(t, dir)
	ageNotes(t, dir)
	res := mustMoveBlock(t, dir, "m-ppppp2", "cutover")
	if wantRes := (MoveBlockResult{From: "priya", To: "cutover", Links: 3}); res != wantRes {
		t.Errorf("result %+v, want %+v", res, wantRes)
	}
	want["people/priya/priya.md"] = mbPriyaAfter
	want["projects/cutover/cutover.md"] = mbCutAfter
	want["projects/cutover/runbook/runbook.md"] = mbRunAfter
	wantTree(t, dir, want)
	for _, rel := range []string{"topics/misc/misc.md", "people/people.md", "projects/projects.md", "topics/topics.md"} {
		wantAged(t, dir, rel)
	}
	noTempFiles(t, dir)

	st := Load(dir)
	cut := mustNode(t, st, "cutover")
	last := cut.Blocks[len(cut.Blocks)-1]
	if last.ID != "m-ppppp2" || last.Text != blockP2 {
		t.Errorf("cutover's last block is ^%s %q", last.ID, last.Text)
	}
	if cut.Blocks[1].ID != "" {
		t.Errorf("the hand-written paragraph was given the ID %s", cut.Blocks[1].ID)
	}
	if _, still := st.Block("priya", "m-ppppp2"); still {
		t.Error("the block is still in priya")
	}
	for _, p := range st.Problems() {
		t.Errorf("problem after the move: %s %s: %s", p.Severity, p.Path, p.Reason)
	}
	wantRefs := []Ref{{Stem: "cutover", Block: "m-ccccc2"}, {Stem: "priya", Block: "m-ppppp3"}, {Stem: "runbook", Block: "m-rrrrr1"}}
	if got := st.BlockBacklinks("cutover", "m-ppppp2"); !reflect.DeepEqual(got, wantRefs) {
		t.Errorf("backlinks %+v, want %+v", got, wantRefs)
	}
}

// Moved back, the block is last in its old node, not where it was, and
// the links follow it again.
func TestMoveBlock_RoundTrip(t *testing.T) {
	dir := blockTree(t, nil)
	mustMoveBlock(t, dir, "m-ppppp2", "cutover")
	res := mustMoveBlock(t, dir, "m-ppppp2", "priya")
	if want := (MoveBlockResult{From: "cutover", To: "priya", Links: 3}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	want := blockTree(t, nil)
	state := treeState(t, want)
	state["people/priya/priya.md"] = fm + "\nPriya leads platform. ^m-ppppp1\n\nThe date in [[priya#^m-ppppp2]] is firm. ^m-ppppp3\n\n" + blockP2 + " ^m-ppppp2\n"
	wantTree(t, dir, state)
}

// The target is appended to by byte surgery: one blank line, the block,
// one line ending, in the note's own line ending; every byte already
// there stays.
func TestMoveBlock_TargetBytes(t *testing.T) {
	const src = fm + "\nMoved text. ^m-sssss1\n\nStays. ^m-sssss2\n"
	crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	cases := map[string]struct{ src, target, want string }{
		"plain": {
			src, fm + "\nA. ^m-ttttt1\n",
			fm + "\nA. ^m-ttttt1\n\nMoved text. ^m-sssss1\n",
		},
		"no final newline": {
			src, fm + "\nA. ^m-ttttt1",
			fm + "\nA. ^m-ttttt1\n\nMoved text. ^m-sssss1\n",
		},
		"CRLF target, LF source": {
			src, crlf(fm + "\nA. ^m-ttttt1\n"),
			crlf(fm + "\nA. ^m-ttttt1\n\nMoved text. ^m-sssss1\n"),
		},
		"CRLF target without a final line ending": {
			src, crlf(fm + "\nA. ^m-ttttt1"),
			crlf(fm + "\nA. ^m-ttttt1\n\nMoved text. ^m-sssss1\n"),
		},
		"byte-order mark, ID-less paragraph, blank-line runs": {
			src, bomText + fm + "\n\n\nNo ID here.\n\n\nA.\n^m-ttttt1  \n",
			bomText + fm + "\n\n\nNo ID here.\n\n\nA.\n^m-ttttt1  \n\nMoved text. ^m-sssss1\n",
		},
		"blank lines already at the end are kept, and none added": {
			src, fm + "\nA. ^m-ttttt1\n\n\n",
			fm + "\nA. ^m-ttttt1\n\n\nMoved text. ^m-sssss1\n",
		},
		"no blocks: laid out as write lays out a first block": {
			src, fm,
			fm + "\nMoved text. ^m-sssss1\n",
		},
		"no blocks, no line ending after the frontmatter": {
			src, strings.TrimSuffix(fm, "\n"),
			fm + "\nMoved text. ^m-sssss1\n",
		},
		"no blocks, blank lines after the frontmatter": {
			src, fm + "\n\n\n",
			fm + "\nMoved text. ^m-sssss1\n",
		},
		"a block of several lines, from CRLF to LF": {
			crlf(fm + "\nLine one\nline two ^m-sssss1\n\nStays. ^m-sssss2\n"), fm + "\nA. ^m-ttttt1\n",
			fm + "\nA. ^m-ttttt1\n\nLine one\nline two ^m-sssss1\n",
		},
		"an ID on its own line is written after the text": {
			fm + "\nMoved text.\n^m-sssss1\n\nStays. ^m-sssss2\n", fm + "\nA. ^m-ttttt1\n",
			fm + "\nA. ^m-ttttt1\n\nMoved text. ^m-sssss1\n",
		},
		"a block that is only its ID": {
			fm + "\n^m-sssss1\n\nStays. ^m-sssss2\n", fm + "\nA. ^m-ttttt1\n",
			fm + "\nA. ^m-ttttt1\n\n^m-sssss1\n",
		},
		"an unsupported link form moves as text, warning and all": {
			fm + "\nSee [[target#Heading]] and ![[target]]. ^m-sssss1\n\nStays. ^m-sssss2\n", fm + "\nA. ^m-ttttt1\n",
			fm + "\nA. ^m-ttttt1\n\nSee [[target#Heading]] and ![[target]]. ^m-sssss1\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"source/source.md": tc.src, "target/target.md": tc.target})
			res := mustMoveBlock(t, dir, "m-sssss1", "target")
			if want := (MoveBlockResult{From: "source", To: "target"}); res != want {
				t.Errorf("result %+v, want %+v", res, want)
			}
			if got := string(fileBytes(t, dir, "target/target.md")); got != tc.want {
				t.Errorf("target:\n   got %s\n  want %s", quote(got), quote(tc.want))
			}
			st := Load(dir)
			if n := mustNode(t, st, "source"); len(n.Blocks) != 1 || n.Blocks[0].ID != "m-sssss2" {
				t.Errorf("source holds %+v", n.Blocks)
			}
		})
	}
}

// The old note loses the block exactly as delete --block removes it.
func TestMoveBlock_OldNoteBytes(t *testing.T) {
	cases := map[string]struct{ src, id, want string }{
		"middle of blank-line runs": {
			fm + "\n\nA. ^m-aaaaa1\n\n\nB. ^m-bbbbb1\n\nC. ^m-ccccc1\n", "m-bbbbb1",
			fm + "\n\nA. ^m-aaaaa1\n\n\nC. ^m-ccccc1\n",
		},
		"last, no final newline, with a link to it before": {
			fm + "\nA, see [[old#^m-bbbbb1|B]]. ^m-aaaaa1\n\nB. ^m-bbbbb1", "m-bbbbb1",
			fm + "\nA, see [[new#^m-bbbbb1|B]]. ^m-aaaaa1\n",
		},
		"the only block": {
			fm + "\nA. ^m-aaaaa1\n", "m-aaaaa1",
			fm,
		},
		"CRLF and an ID-less paragraph": {
			strings.ReplaceAll(fm+"\nNo ID.\n\nB. ^m-bbbbb1\n\nC. ^m-ccccc1\n", "\n", "\r\n"), "m-bbbbb1",
			strings.ReplaceAll(fm+"\nNo ID.\n\nC. ^m-ccccc1\n", "\n", "\r\n"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"old/old.md": tc.src, "new/new.md": fm})
			mustMoveBlock(t, dir, tc.id, "new")
			if got := string(fileBytes(t, dir, "old/old.md")); got != tc.want {
				t.Errorf("old note:\n   got %s\n  want %s", quote(got), quote(tc.want))
			}
		})
	}
}

// A block that links to itself has that link retargeted as it moves, and
// counted; moving a node's last block leaves the node, with no blocks.
func TestMoveBlock_SelfLinkAndEmptiedNode(t *testing.T) {
	dir := blockTree(t, map[string]string{
		"topics/misc/misc.md": fm + "\nNothing about Atlas here. ^m-nnnnn1\n\nThis claim, [[misc#^m-nnnnn2]], stands alone; not [[misc]] nor [[misc#^m-nnnnn1]]. ^m-nnnnn2\n",
	})
	res := mustMoveBlock(t, dir, "m-nnnnn2", "priya")
	if want := (MoveBlockResult{From: "misc", To: "priya", Links: 1}); res != want {
		t.Errorf("result %+v, want %+v", res, want)
	}
	st := Load(dir)
	if b, ok := st.Block("priya", "m-nnnnn2"); !ok || b.Text != "This claim, [[priya#^m-nnnnn2]], stands alone; not [[misc]] nor [[misc#^m-nnnnn1]]." {
		t.Errorf("moved block: %v %q", ok, b.Text)
	}
	mustMoveBlock(t, dir, "m-nnnnn1", "priya")
	st = Load(dir)
	if n := mustNode(t, st, "misc"); len(n.Blocks) != 0 {
		t.Errorf("misc still holds %+v", n.Blocks)
	}
	if got := string(fileBytes(t, dir, "topics/misc/misc.md")); got != fm {
		t.Errorf("misc's note is %s", quote(got))
	}
	for _, p := range st.Problems() {
		t.Errorf("problem after the moves: %s %s: %s", p.Severity, p.Path, p.Reason)
	}
}

// Only links to the block under its old node are rewritten and counted: a
// dangling [[target#^ID]] resolves afterwards uncounted, and a link naming
// the ID under some other node is left alone.
func TestMoveBlock_OnlyLinksToTheBlock(t *testing.T) {
	const holder = fm + "\nEarly [[cutover#^m-ppppp2]], stray [[misc#^m-ppppp2]], twice [[priya#^m-ppppp2]] [[priya#^m-ppppp2|again]]. ^m-hhhhh1\n"
	dir := blockTree(t, map[string]string{"topics/holder/holder.md": holder})
	res := mustMoveBlock(t, dir, "m-ppppp2", "cutover")
	if res.Links != 5 {
		t.Errorf("links rewritten: %d, want 5 (links, not blocks or notes)", res.Links)
	}
	want := fm + "\nEarly [[cutover#^m-ppppp2]], stray [[misc#^m-ppppp2]], twice [[cutover#^m-ppppp2]] [[cutover#^m-ppppp2|again]]. ^m-hhhhh1\n"
	if got := string(fileBytes(t, dir, "topics/holder/holder.md")); got != want {
		t.Errorf("holder:\n   got %s\n  want %s", quote(got), quote(want))
	}
	edges := mustNode(t, Load(dir), "holder").Edges()
	if edges[0].Dangling || !edges[1].Dangling {
		t.Errorf("edges %+v: want the early link resolving and the stray one dangling", edges)
	}
}

// Refusals change nothing, and are checked in the spec's order: the ID
// unknown, the ID carried more than once, the target unknown, the block
// already there.
func TestMoveBlock_Refusals(t *testing.T) {
	dir := blockTree(t, map[string]string{
		// m-ppppp1 again in another node; m-ddddd1 twice within one.
		"topics/misc/misc.md": fm + "\nPasted. ^m-ppppp1\n\nOne. ^m-ddddd1\n\nTwo. ^m-ddddd1\n",
	})
	before := treeState(t, dir)
	w := writerFor(t, dir)

	_, err := w.MoveBlock("m-zzzzzz", "priya")
	wantFieldRefusal(t, err, "id", "m-zzzzzz", "no block")
	_, err = w.MoveBlock("m-zzzzzz", "nowhere")
	wantFieldRefusal(t, err, "id", "no block")
	_, err = w.MoveBlock("^m-ppppp2", "cutover")
	wantFieldRefusal(t, err, "id", "no block")
	_, err = w.MoveBlock("", "cutover")
	wantFieldRefusal(t, err, "id")

	_, err = w.MoveBlock("m-ppppp1", "cutover")
	wantFieldRefusal(t, err, "id", "more than one block", "priya", "misc")
	_, err = w.MoveBlock("m-ppppp1", "nowhere")
	wantFieldRefusal(t, err, "id", "more than one block")
	_, err = w.MoveBlock("m-ddddd1", "cutover")
	wantFieldRefusal(t, err, "id", "more than one block", "misc")

	var none *NoNodeError
	if _, err = w.MoveBlock("m-ppppp2", "nowhere"); !errors.As(err, &none) || none.Stem != "nowhere" || len(none.Stems) == 0 {
		t.Errorf("unknown target: %v, want a NoNodeError for nowhere with the loaded stems", err)
	}
	_, err = w.MoveBlock("m-ppppp2", RootParent)
	wantFieldRefusal(t, err, "to", "root")
	_, err = w.MoveBlock("m-ppppp2", "priya")
	wantFieldRefusal(t, err, "to", "already", "priya")
	wantTree(t, dir, before)
}

// The root is refused before the store is touched: no lock file is made.
func TestMoveBlock_RootRefusedBeforeTheLock(t *testing.T) {
	dir := blockTree(t, nil)
	_, err := writerFor(t, dir).MoveBlock("m-ppppp2", RootParent)
	wantFieldRefusal(t, err, "to", "root")
	if _, err := os.Lstat(dir + "/" + lockName); err == nil {
		t.Error("a refused --to / took the lock")
	}
}

// All or nothing: whichever note cannot be replaced - the old note, the
// target, a linking note - or when the reload shows a new error, every
// note already replaced is restored.
func TestMoveBlock_AllOrNothing(t *testing.T) {
	// Notes are replaced in path order: priya, cutover, runbook.
	for name, c := range map[string]struct {
		note string
		ran  int
	}{
		"removing from the old note": {"priya.md", 0},
		"adding to the target":       {"cutover.md", 1},
		"the last linking note":      {"runbook.md", 2},
	} {
		t.Run(name, func(t *testing.T) {
			dir := blockTree(t, nil)
			before := treeState(t, dir)
			ran, failed := 0, false
			withHook(t, &renameFile, func(from, to string) error {
				if !failed && strings.HasSuffix(to, c.note) {
					failed = true
					return errors.New("injected rename failure")
				}
				if !failed {
					ran++
				}
				return os.Rename(from, to)
			})
			_, err := writerFor(t, dir).MoveBlock("m-ppppp2", "cutover")
			wantRefused(t, err, "injected rename failure")
			if ran != c.ran {
				t.Fatalf("%d notes were replaced before the failure, want %d", ran, c.ran)
			}
			wantTree(t, dir, before)
		})
	}
}

// All or nothing at the reload, and D28's exception: a note an outside
// editor changed is ErrChangedOnDisk, with the other content kept.
func TestMoveBlock_ReloadAndOutsideEdit(t *testing.T) {
	t.Run("the reload shows a new error", func(t *testing.T) {
		dir := blockTree(t, nil)
		before := treeState(t, dir)
		var seen bool
		withHook(t, &testHookReadBack, func(s *Store, rel string) {
			_, moved := s.Block("cutover", "m-ppppp2")
			_, left := s.Block("priya", "m-ppppp2")
			seen = moved && !left && rel == "projects/cutover"
			s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "topics", Reason: "injected failure"})
		})
		_, err := writerFor(t, dir).MoveBlock("m-ppppp2", "cutover")
		wantRefused(t, err, "injected failure", "rolled back")
		if !seen {
			t.Error("the reload did not see the move: the roll back was not exercised")
		}
		wantTree(t, dir, before)
	})
	t.Run("a note changed on disk before it was replaced", func(t *testing.T) {
		dir := blockTree(t, nil)
		edited := mbRunbook + "\nAdded in an editor. ^m-rrrrr2\n"
		want := treeState(t, dir)
		want["projects/cutover/runbook/runbook.md"] = edited
		withHook(t, &testHookBeforeRename, func(notePath string) {
			if strings.HasSuffix(notePath, "runbook.md") {
				writeFile(t, notePath, edited)
			}
		})
		_, err := writerFor(t, dir).MoveBlock("m-ppppp2", "cutover")
		if !errors.Is(err, ErrChangedOnDisk) {
			t.Fatalf("err = %v, want ErrChangedOnDisk", err)
		}
		wantTree(t, dir, want)
	})
}

// A folder that did not load before the call and does not load after it
// is not a new error: it never causes a roll back.
func TestMoveBlock_UnloadedFolderIsNotNew(t *testing.T) {
	dir := blockTree(t, map[string]string{"projects/cutover/Broken/readme.txt": "a folder that does not load"})
	mustMoveBlock(t, dir, "m-ppppp2", "cutover")
	if _, ok := Load(dir).Block("cutover", "m-ppppp2"); !ok {
		t.Error("the block did not arrive")
	}
}

// MoveBlock holds the write lock from before the load until after the
// reload; while another holder has it, it waits, reports busy and writes
// nothing.
func TestMoveBlock_Lock(t *testing.T) {
	dir := blockTree(t, nil)
	w := writerFor(t, dir)
	release := holdLock(t, dir)
	before := treeState(t, dir)
	withHook(t, &lockWait, 150*time.Millisecond)
	if _, err := w.MoveBlock("m-ppppp2", "cutover"); !errors.Is(err, ErrBusy) {
		t.Errorf("while locked: %v, want ErrBusy", err)
	}
	wantTree(t, dir, before)
	release()
	var atLoad, atReload bool
	withHook(t, &testHookLocked, func() { atLoad = lockHeld(t, dir) })
	withHook(t, &testHookReadBack, func(*Store, string) { atReload = lockHeld(t, dir) })
	if _, err := w.MoveBlock("m-ppppp2", "cutover"); err != nil {
		t.Fatalf("once released: %v", err)
	}
	if !atLoad || !atReload {
		t.Errorf("lock held at load: %v, at reload: %v", atLoad, atReload)
	}
	if lockHeld(t, dir) {
		t.Error("the lock is still held after the call")
	}
}

// Rename refuses a new stem that write would refuse to create (D28): one
// carried by any folder in the tree, loaded or not, or differing from a
// folder's name only by letter case, wherever that folder is and whatever
// the filesystem makes of case.
func TestRename_NewCheckedAsWriteChecksAStem(t *testing.T) {
	t.Run("a folder elsewhere that did not load", func(t *testing.T) {
		dir := blockTree(t, map[string]string{"topics/ghost/ghost.md": "no frontmatter here\n"})
		before := treeState(t, dir)
		w := writerFor(t, dir)
		_, err := w.Rename("priya", "ghost")
		wantFieldRefusal(t, err, "new", "ghost", "topics/ghost", "missing frontmatter")
		// write refuses to create the same stem, for the same reason.
		_, werr := w.Write(WriteRequest{Stem: "ghost", Parent: "people", Description: strp("x")})
		wantRefused(t, werr, "topics/ghost", "missing frontmatter")
		wantTree(t, dir, before)
	})
	t.Run("a folder under a skipped subtree", func(t *testing.T) {
		dir := blockTree(t, map[string]string{"topics/Broken/deep/ghost/ghost.md": fm})
		before := treeState(t, dir)
		_, err := writerFor(t, dir).Rename("priya", "ghost")
		wantFieldRefusal(t, err, "new", "ghost", "topics/Broken/deep/ghost")
		wantTree(t, dir, before)
	})
	t.Run("a name differing only by case", func(t *testing.T) {
		dir := blockTree(t, map[string]string{"topics/Atlas-Go/readme.txt": "not a valid stem, so it does not load"})
		before := treeState(t, dir)
		_, err := writerFor(t, dir).Rename("priya", "atlas-go")
		wantFieldRefusal(t, err, "new", "atlas-go", "only by letter case", "topics/Atlas-Go")
		wantTree(t, dir, before)
	})
	t.Run("a case-different folder beside the node says case, not an entry in the way", func(t *testing.T) {
		dir := blockTree(t, map[string]string{"people/Atlas-Go/readme.txt": "beside priya"})
		_, err := writerFor(t, dir).Rename("priya", "atlas-go")
		wantFieldRefusal(t, err, "new", "only by letter case", "people/Atlas-Go")
	})
	t.Run("neither", func(t *testing.T) {
		dir := blockTree(t, nil)
		res, err := writerFor(t, dir).Rename("priya", "atlas-go")
		if err != nil {
			t.Fatal(err)
		}
		if want := (RenameResult{Old: "priya", New: "atlas-go", Links: 4}); res != want {
			t.Errorf("result %+v, want %+v", res, want)
		}
	})
}
