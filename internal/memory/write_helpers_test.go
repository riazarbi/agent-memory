//go:build unix

package memory

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The writer fixtures are temp notes/ trees with the first-level nodes
// user and topics (U6's acceptance criteria).
const (
	userNote   = "---\ndescription: Who the operator is\n---\n\nPrefers short answers. ^m-u6aaa1\n"
	topicsNote = "---\ndescription: Everything else\nkind: hub  # the hub\nrank: 1.50\ncode: \"007\"\n---\n\n\nTopics claim. ^m-u6bbb1\n\n\nSecond claim. ^m-u6bbb2"
)

// writerTree builds the user/topics fixture plus extra files, and returns
// notes/.
func writerTree(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{"user/user.md": userNote, "topics/topics.md": topicsNote}
	for k, v := range extra {
		files[k] = v
	}
	return writeTree(t, files)
}

// writerFor returns the process's writer for dir.
func writerFor(t *testing.T, dir string) *Writer {
	t.Helper()
	w, err := WriterFor(dir)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// appendTo is a write that appends one block to stem.
func appendTo(stem, text string) WriteRequest {
	return WriteRequest{Stem: stem, Blocks: []BlockEdit{{Text: text}}}
}

// treeState maps every entry under notes/ (relative, slash-separated) to
// its content: file bytes, "dir", or "-> target" for a symlink. The lock
// file is left out: D28 creates it on the first write and keeps it, and it
// is a dot-entry neither the reader nor Obsidian sees.
func treeState(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch {
		case rel == lockName:
			return nil
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			out[rel] = "-> " + target
		case d.IsDir():
			out[rel] = "dir"
		default:
			b, err := os.ReadFile(p) //nolint:gosec // test fixture path
			if err != nil {
				return err
			}
			out[rel] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// wantTree fails unless the tree under root is exactly want.
func wantTree(t *testing.T, root string, want map[string]string) {
	t.Helper()
	got := treeState(t, root)
	if reflect.DeepEqual(got, want) {
		return
	}
	var diffs []string
	for k, v := range got {
		if w, ok := want[k]; !ok {
			diffs = append(diffs, "added "+k)
		} else if w != v {
			diffs = append(diffs, "changed "+k+":\n   got "+quote(v)+"\n  want "+quote(w))
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			diffs = append(diffs, "removed "+k)
		}
	}
	sort.Strings(diffs)
	t.Fatalf("notes/ changed:\n  %s", strings.Join(diffs, "\n  "))
}

func quote(s string) string { return strings.ReplaceAll(s, "\n", `\n`) }

// wantRefused fails unless err is a refusal - not busy, not "changed on
// disk" - whose text contains every one of words.
func wantRefused(t *testing.T, err error, words ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("write succeeded, want a refusal")
	}
	if errors.Is(err, ErrBusy) || errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("refusal reported as a retry: %v", err)
	}
	for _, w := range words {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not mention %q", err, w)
		}
	}
}

// wantFieldRefusal fails unless err is a *FieldError naming field.
func wantFieldRefusal(t *testing.T, err error, field string, words ...string) {
	t.Helper()
	wantRefused(t, err, words...)
	var fe *FieldError
	if !errors.As(err, &fe) || fe.Field != field {
		t.Errorf("error %v (%T) does not name field %q", err, err, field)
	}
}

// loadClean loads notes/ and fails on any problem but a dangling link.
func loadClean(t *testing.T, root string) *Store {
	t.Helper()
	s := Load(root)
	for _, p := range s.Problems() {
		if p.Severity == SeverityWarning && strings.Contains(p.Reason, "points to a missing") {
			continue
		}
		t.Fatalf("store does not load clean:%s", dump(s.Problems()))
	}
	return s
}

// blockTexts returns a node's block texts by ID.
func blockTexts(n *Node) map[string]string {
	out := map[string]string{}
	for _, b := range n.Blocks {
		out[b.ID] = b.Text
	}
	return out
}

// noTempFiles fails if any hidden temporary file is left under root.
func noTempFiles(t *testing.T, root string) {
	t.Helper()
	for rel := range treeState(t, root) {
		if rel != "." && strings.HasPrefix(path.Base(rel), ".") {
			t.Errorf("temporary file left behind: %s", rel)
		}
	}
}

// withUmask sets the process umask for the rest of the test.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

// withHook sets a package test seam for the rest of the test.
func withHook[T any](t *testing.T, hook *T, v T) {
	t.Helper()
	old := *hook
	*hook = v
	t.Cleanup(func() { *hook = old })
}

// fileMode returns the permission bits of path (not following a symlink
// when lstat is set).
func fileMode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// sameMtimeEdit replaces old with new (same length) in path and restores
// the file's modification time: an edit only a content comparison sees.
func sameMtimeEdit(t *testing.T, path, old, new string) string {
	t.Helper()
	if len(old) != len(new) {
		t.Fatalf("edit must keep the size: %q vs %q", old, new)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(b), old, new, 1)
	if edited == string(b) {
		t.Fatalf("%q not found in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(edited), info.Mode().Perm()); err != nil { //nolint:gosec // test fixture path
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(info.ModTime()) || after.Size() != info.Size() {
		t.Fatalf("edit changed mtime or size")
	}
	return edited
}

// within fails the test if f does not return within limit.
func within[T any](t *testing.T, limit time.Duration, f func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- f() }()
	select {
	case v := <-done:
		return v
	case <-time.After(limit):
		t.Fatalf("did not complete within %v", limit)
		var zero T
		return zero
	}
}
