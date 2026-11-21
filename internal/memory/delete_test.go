//go:build unix

package memory

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

const fm = "---\ndescription: d\n---\n"

// deleteTree is the delete fixture: plan (with the child task), a holder of
// links into both, and a bystander.
func deleteTree(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"plan/plan.md":           fm + "\nPlan one. ^m-plan01\n\nPlan two. ^m-plan02\n",
		"plan/task/task.md":      fm + "\nSee [[plan]] and [[plan#^m-plan01]]. ^m-task01\n",
		"holder/holder.md":       fm + "\nPer [[plan|the plan]], then [[task#^m-task01]] and `[[plan]]`. ^m-hold01\n\nNo ID, see [[task]] and [[ghost]].\n",
		"bystander/bystander.md": fm + "\nNothing to see. ^m-byst01\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	return writeTree(t, files)
}

func mustDelete(t *testing.T, dir string, req DeleteRequest) DeleteResult {
	t.Helper()
	res, err := writerFor(t, dir).Delete(req)
	if err != nil {
		t.Fatalf("delete %+v: %v", req, err)
	}
	return res
}

// A block delete removes the paragraph and the blank lines that separated
// it, and copies every other byte: line endings, a byte-order mark, runs of
// blank lines elsewhere, ID-less paragraphs (not adopted), a missing final
// newline. The one byte range it adds: the note lost content, so it is dated
// with the time of the call, as the last line of its frontmatter (III.9).
func TestDeleteBlock_Bytes(t *testing.T) {
	atCallTime(t)
	cases := map[string]struct{ src, id, want string }{
		"middle of blank-line runs": {
			fm + "\n\nA. ^m-aaaaa1\n\n\nB. ^m-bbbbb1\n\nC. ^m-ccccc1\n", "m-bbbbb1",
			fm + "\n\nA. ^m-aaaaa1\n\n\nC. ^m-ccccc1\n",
		},
		"first": {
			fm + "\nA. ^m-aaaaa1\n\n\nB. ^m-bbbbb1\n", "m-aaaaa1",
			fm + "\nB. ^m-bbbbb1\n",
		},
		"last, no final newline": {
			fm + "\nA. ^m-aaaaa1\n\nB. ^m-bbbbb1", "m-bbbbb1",
			fm + "\nA. ^m-aaaaa1\n",
		},
		"last, trailing blank lines kept": {
			fm + "\nA. ^m-aaaaa1\n\nB. ^m-bbbbb1\n\n\n", "m-bbbbb1",
			fm + "\nA. ^m-aaaaa1\n\n\n",
		},
		"the only block": {
			fm + "\nA. ^m-aaaaa1\n", "m-aaaaa1",
			fm,
		},
		"CRLF": {
			"---\r\ndescription: d\r\n---\r\n\r\nA. ^m-aaaaa1\r\n\r\nB. ^m-bbbbb1\r\n", "m-aaaaa1",
			"---\r\ndescription: d\r\n---\r\n\r\nB. ^m-bbbbb1\r\n",
		},
		"byte-order mark": {
			bomText + fm + "\nA. ^m-aaaaa1\n\nB. ^m-bbbbb1\n", "m-bbbbb1",
			bomText + fm + "\nA. ^m-aaaaa1\n",
		},
		"ID-less neighbours stay ID-less": {
			fm + "\nNo ID before.\n\nB. ^m-bbbbb1\n\nNo ID after.  \n", "m-bbbbb1",
			fm + "\nNo ID before.\n\nNo ID after.  \n",
		},
		"several lines, ID on its own line": {
			fm + "\nA. ^m-aaaaa1\n\nB one\nB two\n^m-bbbbb1  \n\nC. ^m-ccccc1\n", "m-bbbbb1",
			fm + "\nA. ^m-aaaaa1\n\nC. ^m-ccccc1\n",
		},
		"a duplicated ID: the first goes": {
			fm + "\nFirst. ^m-aaaaa1\n\nSecond. ^m-aaaaa1\n", "m-aaaaa1",
			fm + "\nSecond. ^m-aaaaa1\n",
		},
		"operator frontmatter untouched": {
			"---  \n# a comment\ndescription:   \"d\"\ntags: [ a,b ]\n---\t\n\nA. ^m-aaaaa1\n", "m-aaaaa1",
			"---  \n# a comment\ndescription:   \"d\"\ntags: [ a,b ]\n---\t\n",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"x/x.md": c.src, "y/y.md": fm})
			old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
			other := filepath.Join(dir, "y", "y.md")
			if err := os.Chtimes(other, old, old); err != nil {
				t.Fatal(err)
			}
			res := mustDelete(t, dir, DeleteRequest{Stem: "x", Block: c.id})
			if got, want := string(fileBytes(t, dir, "x/x.md")), stamped(c.want, callText); got != want {
				t.Errorf("x.md\n got %q\nwant %q", got, want)
			}
			if res.Nodes != 0 || res.Blocks != 1 || len(res.Dangling) != 0 {
				t.Errorf("result %+v, want 0 nodes, 1 block, no dangling", res)
			}
			if info, err := os.Stat(other); err != nil || !info.ModTime().Equal(old) {
				t.Errorf("the other note was touched: %v", err)
			}
			noTempFiles(t, dir)
		})
	}
}

func TestDelete_Refusals(t *testing.T) {
	dir := deleteTree(t, map[string]string{
		"holder/stray.txt":       "x",
		"bystander/bad/bad.md":   "no frontmatter",
		"bystander/.DS_Store":    "x",
		"plan/task/.task.md.tmp": "x",
	})
	before := treeState(t, dir)
	w := writerFor(t, dir)

	_, err := w.Delete(DeleteRequest{Stem: "plann"})
	var none *NoNodeError
	if !errors.As(err, &none) || none.Stem != "plann" || !reflect.DeepEqual(none.Stems, []string{"bystander", "holder", "plan", "task"}) {
		t.Errorf("unknown stem: %v (%T), want a NoNodeError listing the loaded stems", err, err)
	}
	// A node the reader skipped is not a node to delete.
	if _, err := w.Delete(DeleteRequest{Stem: "bad", Recursive: true}); !errors.As(err, &none) {
		t.Errorf("delete of a skipped node: %v, want NoNodeError", err)
	}

	_, err = w.Delete(DeleteRequest{Stem: "plan", Block: "m-zzzzzz"})
	wantFieldRefusal(t, err, "block", "m-zzzzzz", "plan")
	_, err = w.Delete(DeleteRequest{Stem: "plan", Block: "m-task01"})
	wantFieldRefusal(t, err, "block", "m-task01", "plan")
	_, err = w.Delete(DeleteRequest{Stem: "plan", Block: "m-plan01", Recursive: true})
	wantFieldRefusal(t, err, "recursive")

	_, err = w.Delete(DeleteRequest{Stem: "plan"})
	wantFieldRefusal(t, err, "stem", "plan", "1 child", "--recursive")
	_, err = w.Delete(DeleteRequest{Stem: "holder"})
	wantFieldRefusal(t, err, "stem", "holder", "stray.txt", "--recursive")
	_, err = w.Delete(DeleteRequest{Stem: "bystander"})
	wantFieldRefusal(t, err, "stem", "bystander", "bad", "--recursive")
	wantTree(t, dir, before)

	// Dot-entries are not content: they go with the folder.
	res := mustDelete(t, dir, DeleteRequest{Stem: "task"})
	if res.Nodes != 1 || res.Blocks != 1 {
		t.Errorf("delete task: %+v", res)
	}
	if _, err := os.Lstat(filepath.Join(dir, "plan", "task")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("plan/task is still there: %v", err)
	}
}

func TestDelete_SeveralChildrenAndStray(t *testing.T) {
	dir := deleteTree(t, map[string]string{"plan/other/other.md": fm, "plan/zz.txt": "x"})
	_, err := writerFor(t, dir).Delete(DeleteRequest{Stem: "plan"})
	wantFieldRefusal(t, err, "stem", "2 children", "zz.txt", "--recursive")
}

// A recursive delete removes the folder with everything in it, counts the
// loaded nodes and blocks, and lists each broken link once: by holder
// stem, block order, then position; as written; never a link from inside
// the subtree, inside inline code, or dangling before.
func TestDelete_RecursiveDangling(t *testing.T) {
	dir := deleteTree(t, map[string]string{"plan/stray.txt": "x", "plan/bad/bad.md": "no frontmatter"})
	res := mustDelete(t, dir, DeleteRequest{Stem: "plan", Recursive: true})
	want := DeleteResult{Nodes: 2, Blocks: 3, Dangling: []DanglingLink{
		{Stem: "holder", Block: "m-hold01", Link: "[[plan|the plan]]"},
		{Stem: "holder", Block: "m-hold01", Link: "[[task#^m-task01]]"},
		{Stem: "holder", Block: "", Link: "[[task]]"},
	}}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("result\n got %+v\nwant %+v", res, want)
	}
	wantTree(t, dir, map[string]string{
		".":                      "dir",
		"holder":                 "dir",
		"holder/holder.md":       fm + "\nPer [[plan|the plan]], then [[task#^m-task01]] and `[[plan]]`. ^m-hold01\n\nNo ID, see [[task]] and [[ghost]].\n",
		"bystander":              "dir",
		"bystander/bystander.md": fm + "\nNothing to see. ^m-byst01\n",
	})
}

// A block delete lists links from other blocks of the same node, and a
// link that still resolves (a duplicated ID's second holder) is not
// dangling.
func TestDeleteBlock_Dangling(t *testing.T) {
	dir := deleteTree(t, map[string]string{
		"plan/plan.md": fm + "\nPlan one, which is [[plan#^m-plan01]]. ^m-plan01\n\nAs [[plan#^m-plan01]] says, see [[plan]]. ^m-plan02\n",
		"dup/dup.md":   fm + "\nFirst. ^m-dup001\n\nSecond. ^m-dup001\n\nSee [[dup#^m-dup001]]. ^m-dup002\n",
	})
	res := mustDelete(t, dir, DeleteRequest{Stem: "plan", Block: "m-plan01"})
	want := []DanglingLink{
		{Stem: "plan", Block: "m-plan02", Link: "[[plan#^m-plan01]]"},
		{Stem: "task", Block: "m-task01", Link: "[[plan#^m-plan01]]"},
	}
	if !reflect.DeepEqual(res.Dangling, want) {
		t.Errorf("dangling\n got %+v\nwant %+v", res.Dangling, want)
	}
	if res := mustDelete(t, dir, DeleteRequest{Stem: "dup", Block: "m-dup001"}); len(res.Dangling) != 0 {
		t.Errorf("a link the second holder of the ID still answers is reported: %+v", res.Dangling)
	}
}

// When the reload after the rename shows an error the store did not have,
// the folder is renamed back and the call fails; nothing changed.
func TestDelete_NewErrorRollsBack(t *testing.T) {
	for name, req := range map[string]DeleteRequest{
		"node":  {Stem: "plan", Recursive: true},
		"block": {Stem: "plan", Block: "m-plan01"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := deleteTree(t, nil)
			before := treeState(t, dir)
			var gone bool
			withHook(t, &testHookReadBack, func(s *Store, _ string) {
				_, has := s.Block("plan", "m-plan01")
				gone = !has
				s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "holder", Reason: "injected failure"})
			})
			_, err := writerFor(t, dir).Delete(req)
			wantRefused(t, err, "injected failure", "holder")
			if !gone {
				t.Error("the reload did not see the delete: the roll back was not exercised")
			}
			wantTree(t, dir, before)
		})
	}
}

// A note edited by something that does not take the lock, between the
// load and the delete, refuses the delete as "changed on disk".
func TestDelete_ChangedOnDisk(t *testing.T) {
	for name, req := range map[string]DeleteRequest{
		"node":  {Stem: "plan", Recursive: true},
		"block": {Stem: "plan", Block: "m-plan01"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := deleteTree(t, nil)
			edited := fm + "\nPlan one, edited. ^m-plan01\n"
			withHook(t, &testHookBeforeRename, func(string) {
				writeFile(t, filepath.Join(dir, "plan", "plan.md"), edited)
			})
			_, err := writerFor(t, dir).Delete(req)
			if !errors.Is(err, ErrChangedOnDisk) {
				t.Fatalf("err = %v, want ErrChangedOnDisk", err)
			}
			if got := string(fileBytes(t, dir, "plan/plan.md")); got != edited {
				t.Errorf("the external edit was lost: %q", got)
			}
			if _, err := os.Stat(filepath.Join(dir, "plan", "task", "task.md")); err != nil {
				t.Errorf("the subtree was removed: %v", err)
			}
			noTempFiles(t, dir)
		})
	}
}

// Delete and Retag hold the write lock: while another holder has it they
// wait, then report busy, and write nothing. A control proves the same
// calls succeed once it is released.
func TestDeleteRetag_Busy(t *testing.T) {
	dir := deleteTree(t, map[string]string{"bystander/bystander.md": "---\ndescription: d\ntags: [x]\n---\n"})
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	fd := int(lock.Fd()) //nolint:gosec // a file descriptor fits in an int
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	before := treeState(t, dir)
	withHook(t, &lockWait, 150*time.Millisecond)
	w := writerFor(t, dir)
	if _, err := w.Delete(DeleteRequest{Stem: "bystander"}); !errors.Is(err, ErrBusy) {
		t.Errorf("delete while locked: %v, want ErrBusy", err)
	}
	if _, err := w.Retag("x", "y"); !errors.Is(err, ErrBusy) {
		t.Errorf("retag while locked: %v, want ErrBusy", err)
	}
	wantTree(t, dir, before)
	if err := syscall.Flock(fd, syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if res, err := w.Retag("x", "y"); err != nil || res.Nodes != 1 {
		t.Errorf("retag once released: %+v, %v", res, err)
	}
	mustDelete(t, dir, DeleteRequest{Stem: "bystander"})
}

// The lock is held from before the load until after the reload.
func TestDelete_HoldsLockThroughout(t *testing.T) {
	dir := deleteTree(t, nil)
	var atLoad, atReload bool
	withHook(t, &testHookLocked, func() { atLoad = lockHeld(t, dir) })
	withHook(t, &testHookReadBack, func(*Store, string) { atReload = lockHeld(t, dir) })
	mustDelete(t, dir, DeleteRequest{Stem: "bystander"})
	if !atLoad || !atReload {
		t.Errorf("lock held at load: %v, at reload: %v", atLoad, atReload)
	}
	if lockHeld(t, dir) {
		t.Error("the lock is still held after the delete")
	}
}

// A hidden folder left by an earlier, interrupted delete does not block
// the next one.
func TestDelete_LeftoverHiddenFolder(t *testing.T) {
	dir := deleteTree(t, map[string]string{".bystander.deleting/x.md": "stale"})
	mustDelete(t, dir, DeleteRequest{Stem: "bystander"})
	if _, err := os.Lstat(filepath.Join(dir, "bystander")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("bystander is still there: %v", err)
	}
	for rel := range treeState(t, dir) {
		if strings.HasPrefix(rel, ".bystander.deleting.") {
			t.Errorf("this delete left %s behind", rel)
		}
	}
}

// scanLinkSpans is scanLinks with each link's text as written.
func TestScanLinkSpans(t *testing.T) {
	text := "See [[a]], [[b|alias]] and `[[c]]`, then [[d#^m-abc123|x]] and [[bad#heading]]."
	spans, warnings := scanLinkSpans(text)
	written := make([]string, 0, len(spans))
	edges := make([]Edge, 0, len(spans))
	for _, s := range spans {
		written = append(written, text[s.start:s.end])
		edges = append(edges, s.edge)
	}
	if want := []string{"[[a]]", "[[b|alias]]", "[[d#^m-abc123|x]]"}; !reflect.DeepEqual(written, want) {
		t.Errorf("as written: %q, want %q", written, want)
	}
	plain, plainWarnings := scanLinks(text)
	if !reflect.DeepEqual(edges, plain) || !reflect.DeepEqual(warnings, plainWarnings) || len(warnings) != 1 {
		t.Errorf("scanLinkSpans and scanLinks disagree: %v %v / %v %v", edges, warnings, plain, plainWarnings)
	}
}
