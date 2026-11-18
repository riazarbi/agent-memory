//go:build unix

package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// U6 criterion 1: a create under umask 077 makes the folder inside the
// parent's folder with mode 0755 and the folder note with mode 0644, and
// returns the node as read back with the block's assigned ID.
func TestWrite_Create(t *testing.T) {
	dir := writerTree(t, nil)
	withUmask(t, 0o077)
	res, err := writerFor(t, dir).Write(WriteRequest{
		Stem: "outbox-etiquette", Parent: "topics", Description: strp("How to cc"),
		Blocks: []BlockEdit{{Text: "A claim."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	notePath := wantCreatedAt(t, dir, "topics", "outbox-etiquette")
	n := mustNode(t, loadClean(t, dir), "outbox-etiquette")
	if n.Path != "topics/outbox-etiquette" || n.Parent() == nil || n.Parent().Stem != "topics" {
		t.Errorf("fresh load: path %q parent %v", n.Path, n.Parent())
	}
	if len(res.BlockIDs) != 1 || !reflect.DeepEqual(blockTexts(n), map[string]string{res.BlockIDs[0]: "A claim."}) {
		t.Fatalf("returned IDs %q, blocks on disk %v", res.BlockIDs, blockTexts(n))
	}
	if !res.Created {
		t.Error("create not reported as created")
	}
	wantReadBack(t, res.Node, n, notePath)
}

// wantCreatedAt fails unless the node stem was created inside the parent's
// folder, and nowhere else, with folder mode 0755 and note mode 0644. It
// returns the folder note's path.
func wantCreatedAt(t *testing.T, dir, parent, stem string) string {
	t.Helper()
	folder := filepath.Join(dir, parent, stem)
	notePath := filepath.Join(folder, stem+".md")
	if m := fileMode(t, folder); m != 0o755 {
		t.Errorf("folder mode %v, want 0755", m)
	}
	if m := fileMode(t, notePath); m != 0o644 {
		t.Errorf("folder note mode %v, want 0644", m)
	}
	for _, wrong := range []string{stem, "user/" + stem} {
		if _, err := os.Lstat(filepath.Join(dir, wrong)); err == nil {
			t.Errorf("node created at %s", wrong)
		}
	}
	return notePath
}

// wantReadBack fails unless got - the node a write returned - is the node
// as read back from disk (its source and mtime are the file's), and matches
// a fresh load.
func wantReadBack(t *testing.T, got, fresh *Node, notePath string) {
	t.Helper()
	if got == nil || got.Path != fresh.Path || got.Parent() == nil || got.Parent().Stem != fresh.Parent().Stem {
		t.Fatalf("returned node %+v, fresh %+v", got, fresh)
	}
	info, err := os.Stat(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != string(fileBytes(t, filepath.Dir(notePath), filepath.Base(notePath))) || !got.Modified.Equal(info.ModTime()) {
		t.Errorf("returned node is not the file as read back: source %q modified %v (file %v)", got.Source, got.Modified, info.ModTime())
	}
	if !reflect.DeepEqual(blockTexts(got), blockTexts(fresh)) || got.Description != fresh.Description {
		t.Errorf("returned node blocks %v description %q, fresh %v %q", blockTexts(got), got.Description, blockTexts(fresh), fresh.Description)
	}
}

// U6 criterion 2: each refused create names what to fix and leaves notes/
// unchanged. Each fixture gives its refusal a sole cause.
func TestWrite_CreateRefusals(t *testing.T) {
	desc := strp("A description")
	block := []BlockEdit{{Text: "A claim."}}
	cases := []struct {
		name  string
		extra map[string]string
		req   WriteRequest
		field string // a *FieldError naming it, or "" for a plain refusal
		words []string
	}{
		{"invalid stem", nil, WriteRequest{Stem: "Bad_Name", Parent: "topics", Description: desc, Blocks: block}, "stem", []string{"Bad_Name"}},
		{"stem exists", nil, WriteRequest{Stem: "user", Parent: "topics", Description: desc, Blocks: block}, "parent", []string{"already exists", "update"}},
		{
			"stem used by a skipped folder under another parent",
			map[string]string{"topics/ghost/ghost.md": "no frontmatter here\n"},
			WriteRequest{Stem: "ghost", Parent: "user", Description: desc, Blocks: block}, "",
			[]string{"topics/ghost", "missing frontmatter"},
		},
		{"missing parent", nil, WriteRequest{Stem: "new-node", Parent: "nowhere", Description: desc, Blocks: block}, "parent", []string{"nowhere"}},
		{"no parent", nil, WriteRequest{Stem: "new-node", Description: desc, Blocks: block}, "parent", []string{"top level"}},
		{"no description", nil, WriteRequest{Stem: "new-node", Parent: "topics", Blocks: block}, "description", []string{"required"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writerTree(t, tc.extra)
			before := treeState(t, dir)
			_, err := writerFor(t, dir).Write(tc.req)
			if tc.field != "" {
				wantFieldRefusal(t, err, tc.field, tc.words...)
			} else {
				wantRefused(t, err, tc.words...)
			}
			wantTree(t, dir, before)
		})
	}
}

// U6 criterion 2: a stem that differs from an existing folder only by
// letter case is refused as a case collision - never as "changed on disk -
// retry", which could never succeed. On a case-insensitive filesystem
// (macOS) topics/ghost would be topics/Ghost; the writer refuses the same
// way on every platform.
func TestWrite_CreateRefusesCaseCollision(t *testing.T) {
	for _, parent := range []string{"topics", "user"} {
		t.Run("parent "+parent, func(t *testing.T) {
			dir := writerTree(t, map[string]string{"topics/Ghost/Ghost.md": note("Wrong case", "")})
			before := treeState(t, dir)
			_, err := writerFor(t, dir).Write(WriteRequest{Stem: "ghost", Parent: parent, Description: strp("d"), Blocks: []BlockEdit{{Text: "A claim."}}})
			wantFieldRefusal(t, err, "stem", "letter case", "topics/Ghost")
			wantTree(t, dir, before)
		})
	}
}

// U6 criterion 3: an update with no parent appends; with a parent that is
// not the node's own it is refused and nothing changes.
func TestWrite_Update(t *testing.T) {
	dir := writerTree(t, nil)
	w := writerFor(t, dir)
	res, err := w.Write(appendTo("user", "Likes tables."))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || len(res.BlockIDs) != 1 {
		t.Fatalf("created %v IDs %q", res.Created, res.BlockIDs)
	}
	n := mustNode(t, loadClean(t, dir), "user")
	want := map[string]string{"m-u6aaa1": "Prefers short answers.", res.BlockIDs[0]: "Likes tables."}
	if !reflect.DeepEqual(blockTexts(n), want) || n.Path != "user" {
		t.Fatalf("user on disk: path %q blocks %v, want %v", n.Path, blockTexts(n), want)
	}

	before := treeState(t, dir)
	mismatched := appendTo("user", "Another.")
	mismatched.Parent = "topics"
	_, err = w.Write(mismatched)
	wantFieldRefusal(t, err, "parent", "topics")
	wantTree(t, dir, before)

	// Naming the actual parent of a nested node is an update too.
	if _, err := w.Write(WriteRequest{Stem: "child", Parent: "topics", Description: strp("d")}); err != nil {
		t.Fatal(err)
	}
	nested := appendTo("child", "Nested claim.")
	nested.Parent = "topics"
	if res, err := w.Write(nested); err != nil || res.Created {
		t.Fatalf("update naming the actual parent: created %v, %v", res.Created, err)
	}
}

// U6 criterion 3: a property merge on topics changes only that value; the
// rest of the frontmatter keeps its form (D26) and the body its exact bytes
// (D27 "frontmatter-only changes").
func TestWrite_UpdatePropertyMergeKeepsOtherBytes(t *testing.T) {
	dir := writerTree(t, nil)
	if _, err := writerFor(t, dir).Write(WriteRequest{Stem: "topics", Properties: map[string]any{"rank": 3}}); err != nil {
		t.Fatal(err)
	}
	want := "---\ndescription: Everything else\nkind: hub # the hub\nrank: 3\ncode: \"007\"\n---\n" +
		"\n\nTopics claim. ^m-u6bbb1\n\n\nSecond claim. ^m-u6bbb2"
	if got := string(fileBytes(t, dir, "topics/topics.md")); got != want {
		t.Errorf("topics.md\n got %q\nwant %q", got, want)
	}
}

// U6 criterion 4: a write to a node the reader skipped is refused with the
// reader's problem quoted, and the file is byte-identical afterwards -
// whether the write names a parent or not.
func TestWrite_SkippedNodeRefused(t *testing.T) {
	for name, parent := range map[string]string{"no parent": "", "its own parent": "topics"} {
		t.Run(name, func(t *testing.T) {
			dir := writerTree(t, map[string]string{"topics/broken/broken.md": "Just text, no frontmatter.\n"})
			problem := Load(dir).Problems()[0]
			if problem.Path != "topics/broken" || problem.Severity != SeverityError {
				t.Fatalf("fixture: %s", dump(Load(dir).Problems()))
			}
			before := treeState(t, dir)
			req := appendTo("broken", "A claim.")
			req.Parent, req.Description = parent, strp("d")
			_, err := writerFor(t, dir).Write(req)
			wantRefused(t, err, problem.Reason, "topics/broken")
			wantTree(t, dir, before)
		})
	}
}

// U6 criterion 5: 20 goroutines appending to user at once lose nothing.
func TestWrite_ConcurrentAppendsSerialize(t *testing.T) {
	dir := writerTree(t, nil)
	const writers = 20
	ids := make([][]string, writers)
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			w, err := WriterFor(dir)
			if err != nil {
				errs[i] = err
				return
			}
			res, err := w.Write(appendTo("user", fmt.Sprintf("Concurrent claim %d.", i)))
			ids[i], errs[i] = res.BlockIDs, err
		})
	}
	wg.Wait()
	want := map[string]string{"m-u6aaa1": "Prefers short answers."}
	for i := range writers {
		if errs[i] != nil {
			t.Fatalf("writer %d: %v", i, errs[i])
		}
		want[ids[i][0]] = fmt.Sprintf("Concurrent claim %d.", i)
	}
	n := mustNode(t, loadClean(t, dir), "user")
	if len(n.Blocks) != writers+1 || !reflect.DeepEqual(blockTexts(n), want) {
		t.Fatalf("user has %d blocks %v, want %v", len(n.Blocks), blockTexts(n), want)
	}
}

// One writer per notes/ per process (D28): every caller shares it.
func TestWriterFor_SharedPerDirectory(t *testing.T) {
	dir := writerTree(t, nil)
	a, b := writerFor(t, dir), writerFor(t, dir+string(filepath.Separator)+".")
	if a != b {
		t.Error("two writers for one notes/ directory")
	}
	if c := writerFor(t, writerTree(t, nil)); c == a {
		t.Error("one writer shared by two notes/ directories")
	}
}

// U6 criterion 5: each write starts from a fresh load, so a paragraph the
// operator adds between writes is kept (and given an ID).
func TestWrite_FreshLoadSeesOperatorEdit(t *testing.T) {
	dir := writerTree(t, nil)
	w := writerFor(t, dir)
	if _, err := w.Write(appendTo("user", "First write.")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "user", "user.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\nOperator paragraph.\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := w.Write(appendTo("user", "Second write."))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Adopted) != 1 || blockTexts(res.Node)[res.Adopted[0]] != "Operator paragraph." {
		t.Fatalf("operator paragraph not kept: adopted %q, blocks %v", res.Adopted, blockTexts(res.Node))
	}
	if n := mustNode(t, loadClean(t, dir), "user"); len(n.Blocks) != 4 || blockTexts(n)[res.Adopted[0]] != "Operator paragraph." {
		t.Fatalf("on disk: %v", blockTexts(n))
	}
}

var errInjectedRename = errors.New("injected rename failure")

// U6 criterion 5: when the rename fails, the original is byte-identical
// and no temporary file remains - for an update and for a create.
func TestWrite_RenameFailureLeavesStoreUnchanged(t *testing.T) {
	withHook(t, &renameFile, func(string, string) error { return errInjectedRename })
	for name, req := range map[string]WriteRequest{
		"update": appendTo("user", "Never written."),
		"create": {Stem: "new-node", Parent: "topics", Description: strp("d"), Blocks: []BlockEdit{{Text: "x"}}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writerTree(t, nil)
			before := treeState(t, dir)
			_, err := writerFor(t, dir).Write(req)
			if !errors.Is(err, errInjectedRename) {
				t.Fatalf("err = %v, want the rename failure", err)
			}
			wantTree(t, dir, before)
		})
	}
}

// U6 criterion 6: an external edit between the write's read and its rename
// is refused as "changed on disk", and the file keeps exactly that edit.
// The edit keeps the size and the modification time, so only comparing
// contents catches it.
func TestWrite_EditRaceRefused(t *testing.T) {
	dir := writerTree(t, nil)
	path := filepath.Join(dir, "user", "user.md")
	var external string
	withHook(t, &testHookBeforeRename, func(string) { external = sameMtimeEdit(t, path, "short", "SHORT") })
	want := treeState(t, dir)
	_, err := writerFor(t, dir).Write(appendTo("user", "Lost?"))
	if !errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("err = %v, want ErrChangedOnDisk", err)
	}
	want["user/user.md"] = external
	wantTree(t, dir, want)
}

// U6 criterion 6: the same for a create whose folder note appears between
// the write's checks and its rename.
func TestWrite_EditRaceRefusedOnCreate(t *testing.T) {
	dir := writerTree(t, nil)
	const external = "---\ndescription: written by someone else\n---\n"
	notePath := filepath.Join(dir, "topics", "new-node", "new-node.md")
	withHook(t, &testHookBeforeRename, func(string) { writeFile(t, notePath, external) })
	want := treeState(t, dir)
	_, err := writerFor(t, dir).Write(WriteRequest{Stem: "new-node", Parent: "topics", Description: strp("d"), Blocks: []BlockEdit{{Text: "Mine."}}})
	if !errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("err = %v, want ErrChangedOnDisk", err)
	}
	want["topics/new-node"] = "dir"
	want["topics/new-node/new-node.md"] = external
	wantTree(t, dir, want)
}

// U6 criterion 7: when the read-back reports a new error for the written
// node, an update is rolled back to the previous bytes and a create's
// folder is removed; either way an error is returned and nothing changed.
func TestWrite_ReadBackFailureRollsBack(t *testing.T) {
	for name, req := range map[string]WriteRequest{
		"update": appendTo("user", "Rolled back."),
		"create": {Stem: "new-node", Parent: "topics", Description: strp("d"), Blocks: []BlockEdit{{Text: "Rolled back."}}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writerTree(t, nil)
			before := treeState(t, dir)
			withHook(t, &testHookReadBack, func(s *Store, rel string) {
				s.problems = append(s.problems, Problem{Severity: SeverityError, Path: rel, Reason: "injected read-back failure"})
			})
			_, err := writerFor(t, dir).Write(req)
			wantRefused(t, err, "injected read-back failure", "rolled back")
			wantTree(t, dir, before)
		})
	}
}

// The read-back runs after the rename: a control for the seam above, so
// the roll back test cannot pass by failing before anything was written.
func TestWrite_ReadBackSeesTheWrittenFile(t *testing.T) {
	dir := writerTree(t, nil)
	var seen []byte
	withHook(t, &testHookReadBack, func(*Store, string) { seen = fileBytes(t, dir, "user/user.md") })
	res, err := writerFor(t, dir).Write(appendTo("user", "Read back."))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(seen), "Read back. ^"+res.BlockIDs[0]) {
		t.Fatalf("read-back saw %q", seen)
	}
}

// U6 criterion 9 (in-process part): an editor that ignores the lock and
// overwrites the folder note between the rename and the read-back makes
// the write return "changed on disk"; the other content stays (no roll
// back).
func TestWrite_OverwrittenBeforeReadBackIsNotSuccess(t *testing.T) {
	dir := writerTree(t, nil)
	const other = "---\ndescription: Someone else's valid content\n---\n\nTheirs. ^m-u6zzz1\n"
	path := filepath.Join(dir, "user", "user.md")
	withHook(t, &testHookAfterRename, func(string) { writeFile(t, path, other) })
	_, err := writerFor(t, dir).Write(appendTo("user", "Mine."))
	if !errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("err = %v, want ErrChangedOnDisk", err)
	}
	if got := string(fileBytes(t, dir, "user/user.md")); got != other {
		t.Fatalf("user.md = %q, want the other writer's content", got)
	}
}

// U6 criterion 10: a link to a missing stem is a warning, not a read-back
// failure - the write succeeds and stays on disk.
func TestWrite_DanglingLinkIsNotRolledBack(t *testing.T) {
	dir := writerTree(t, nil)
	res, err := writerFor(t, dir).Write(appendTo("user", "See [[nowhere-yet]]."))
	if err != nil {
		t.Fatal(err)
	}
	n := mustNode(t, Load(dir), "user")
	if blockTexts(n)[res.BlockIDs[0]] != "See [[nowhere-yet]]." || !n.Blocks[1].Edges[0].Dangling {
		t.Fatalf("on disk: %v", blockTexts(n))
	}
}

// U6 criterion 10: an update keeps the folder note's mode. 0600 is also
// what a temporary file starts with, so 0640 shows the mode is copied.
func TestWrite_UpdateKeepsFileMode(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o640} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := writerTree(t, nil)
			path := filepath.Join(dir, "user", "user.md")
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := writerFor(t, dir).Write(appendTo("user", "Mode kept.")); err != nil {
				t.Fatal(err)
			}
			if got := fileMode(t, path); got != mode {
				t.Errorf("mode %v after update, want %v", got, mode)
			}
		})
	}
}

// U6 criterion 10: a folder note that is a symlink to a file outside
// notes/ stays a symlink, and its target holds the new content.
func TestWrite_SymlinkedFolderNoteWrittenThrough(t *testing.T) {
	dir := writerTree(t, nil)
	target := filepath.Join(t.TempDir(), "shared-note.md")
	writeFile(t, target, note("A shared node", "Shared claim. ^m-u6ccc1\n"))
	mkdir(t, filepath.Join(dir, "topics", "linked"))
	link := filepath.Join(dir, "topics", "linked", "linked.md")
	symlink(t, target, link)
	res, err := writerFor(t, dir).Write(appendTo("linked", "Through the link."))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("folder note is no longer a symlink (%v)", err)
	}
	if dest, _ := os.Readlink(link); dest != target {
		t.Errorf("link now points to %q", dest)
	}
	content, err := os.ReadFile(target) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Through the link. ^"+res.BlockIDs[0]) {
		t.Errorf("target = %q", content)
	}
	entries, _ := os.ReadDir(filepath.Dir(target))
	if len(entries) != 1 {
		t.Errorf("temporary file left beside the target: %v", entries)
	}
	noTempFiles(t, dir)
}
