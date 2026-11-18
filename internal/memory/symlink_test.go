//go:build unix

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// loadWithin runs Load and fails the test (rather than hanging it) if Load
// does not return within limit.
func loadWithin(t *testing.T, root string, limit time.Duration) *Store {
	t.Helper()
	done := make(chan *Store, 1)
	go func() { done <- Load(root) }()
	select {
	case s := <-done:
		return s
	case <-time.After(limit):
		t.Fatalf("Load did not complete within %v", limit)
		return nil
	}
}

// Criterion 8: a symlink to a directory outside notes/ loads at the
// symlink's location, as the link's stem.
func TestSymlink_BoundSubtreeOutsideNotes(t *testing.T) {
	root := writeTree(t, map[string]string{"user/user.md": note("u", "")})
	ext := filepath.Join(t.TempDir(), "library")
	writeFile(t, filepath.Join(ext, "shared.md"), note("Shared notes", "Bound content. ^m-sh0001\n"))
	writeFile(t, filepath.Join(ext, "kid", "kid.md"), note("Shared child", ""))
	symlink(t, ext, filepath.Join(root, "shared"))

	s := Load(root)
	wantProblems(t, s)
	wantStems(t, s.FirstLevel(), "shared", "user")
	sh := mustNode(t, s, "shared")
	if sh.Path != "shared" || sh.Parent() != nil {
		t.Errorf("shared path %q parent %v", sh.Path, sh.Parent())
	}
	if k := mustNode(t, s, "kid"); k.Path != "shared/kid" || k.Parent() != sh {
		t.Errorf("kid path %q", k.Path)
	}
}

// notes/ itself may be a symlink.
func TestSymlink_RootIsSymlink(t *testing.T) {
	real := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	link := filepath.Join(t.TempDir(), "notes-link")
	symlink(t, real, link)
	s := Load(link)
	wantProblems(t, s)
	if a := mustNode(t, s, "a"); a.Path != "a" {
		t.Errorf("path %q", a.Path)
	}
}

func TestSymlink_CycleToAncestor(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a/a.md":   note("a", ""),
		"a/b/b.md": note("b", ""),
		"z/z.md":   note("z", ""),
	})
	symlink(t, filepath.Join(root, "a"), filepath.Join(root, "a", "b", "loop"))
	symlink(t, "..", filepath.Join(root, "z", "up")) // back to notes/ itself
	s := loadWithin(t, root, 2*time.Second)
	wantProblems(t, s, pk{SeverityError, "a/b/loop"}, pk{SeverityError, "z/up"})
	wantStems(t, s.Nodes(), "a", "b", "z")
}

// A cycle inside a bound subtree outside notes/ is caught by identity.
func TestSymlink_CycleInsideBoundSubtree(t *testing.T) {
	root := writeTree(t, map[string]string{})
	ext := filepath.Join(t.TempDir(), "library")
	writeFile(t, filepath.Join(ext, "shared.md"), note("s", ""))
	symlink(t, ext, filepath.Join(ext, "lib-loop"))
	symlink(t, ext, filepath.Join(root, "shared"))
	s := loadWithin(t, root, 2*time.Second)
	wantProblems(t, s, pk{SeverityError, "shared/lib-loop"})
	wantStems(t, s.Nodes(), "shared")
}

// A symlink out of notes/ to a directory that contains notes/ must not let
// the walk re-enter notes/.
func TestSymlink_OutsideRouteBackIntoNotes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "notes")
	writeFile(t, filepath.Join(root, "a", "a.md"), note("a", ""))
	writeFile(t, filepath.Join(base, "base.md"), note("outer", ""))
	// notes/notes.md makes re-entry observable: walked again as a folder
	// named "notes", notes/ would load as a node.
	writeFile(t, filepath.Join(root, "notes.md"), note("stray", ""))
	symlink(t, base, filepath.Join(root, "a", "base"))
	s := loadWithin(t, root, 2*time.Second)
	wantProblems(t, s, pk{SeverityError, "a/base/notes"}, pk{SeverityWarning, "notes.md"})
	wantStems(t, s.Nodes(), "a", "base")
}

// Each real directory outside notes/ is walked at most once: the second
// symlinked route to it, in component order, is an error.
func TestSymlink_SecondRouteToOutsideDir(t *testing.T) {
	root := writeTree(t, map[string]string{})
	ext := filepath.Join(t.TempDir(), "library")
	writeFile(t, filepath.Join(ext, "one.md"), note("one", ""))
	writeFile(t, filepath.Join(ext, "two.md"), note("two", ""))
	symlink(t, ext, filepath.Join(root, "one"))
	symlink(t, ext, filepath.Join(root, "two"))
	s := Load(root)
	// one loads (two.md is a stray beside its folder note); two is a
	// second route to the same directory.
	wantProblems(t, s, pk{SeverityWarning, "one/two.md"}, pk{SeverityError, "two"})
	wantStems(t, s.Nodes(), "one")
}

// Inside a bound subtree, a real directory already walked through an
// earlier symlinked route (in component order) is not walked again.
func TestSymlink_BoundSubtreeRealDirAfterRoute(t *testing.T) {
	root := writeTree(t, map[string]string{})
	ext := filepath.Join(t.TempDir(), "library")
	writeFile(t, filepath.Join(ext, "shared.md"), note("s", ""))
	writeFile(t, filepath.Join(ext, "zed", "zed.md"), note("zed", ""))
	writeFile(t, filepath.Join(ext, "zed", "a-route.md"), note("route", ""))
	symlink(t, filepath.Join(ext, "zed"), filepath.Join(ext, "a-route"))
	symlink(t, ext, filepath.Join(root, "shared"))
	s := loadWithin(t, root, 2*time.Second)
	wantProblems(t, s, pk{SeverityWarning, "shared/a-route/zed.md"}, pk{SeverityError, "shared/zed"})
	wantStems(t, s.Nodes(), "shared", "a-route")
}

// The 24-node converging-links fixture: every node loads once at its real
// path; each of the 45 symlinks is one error; the walk is fast.
func TestSymlink_ConvergingLinksRealLocationsWin(t *testing.T) {
	const n = 24
	root, links := convergingFixture(t, n)
	if len(links) != 45 {
		t.Fatalf("fixture has %d links, want 45", len(links))
	}

	start := time.Now()
	s := loadWithin(t, root, 10*time.Second)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Load took %v, want under 2s", d)
	}

	if got := len(s.Nodes()); got != n {
		t.Errorf("%d nodes, want %d", got, n)
	}
	if got := len(s.FirstLevel()); got != n {
		t.Errorf("%d first-level nodes, want %d", got, n)
	}
	for i := 1; i <= n; i++ {
		nd := mustNode(t, s, nodeName(i))
		if nd.Path != nodeName(i) || nd.Parent() != nil {
			t.Errorf("%s at %q (parent %v), want first-level at its real path", nd.Stem, nd.Path, nd.Parent())
		}
	}
	wantOnlyErrorsAt(t, s.Problems(), links)
}

func nodeName(i int) string { return fmt.Sprintf("n%02d", i) }

// convergingFixture builds n first-level nodes where each nI holds
// symlinks nI-a -> n(I+1) and nI-b -> n(I+2), where those exist.
func convergingFixture(t *testing.T, n int) (string, []string) {
	t.Helper()
	files := map[string]string{}
	for i := 1; i <= n; i++ {
		files[nodeName(i)+"/"+nodeName(i)+".md"] = note("node "+nodeName(i), "")
	}
	root := writeTree(t, files)
	var links []string
	for i := 1; i <= n; i++ {
		for j, suffix := range []string{"-a", "-b"} {
			target := i + j + 1
			if target > n {
				continue
			}
			rel := nodeName(i) + "/" + nodeName(i) + suffix
			symlink(t, filepath.Join(root, nodeName(target)), filepath.Join(root, filepath.FromSlash(rel)))
			links = append(links, rel)
		}
	}
	sort.Strings(links)
	return root, links
}

func wantOnlyErrorsAt(t *testing.T, ps []Problem, paths []string) {
	t.Helper()
	got := make([]string, 0, len(ps))
	for _, p := range ps {
		if p.Severity != SeverityError {
			t.Errorf("non-error problem %s %s: %s", p.Severity, p.Path, p.Reason)
		}
		got = append(got, p.Path)
	}
	if strings.Join(got, " ") != strings.Join(paths, " ") {
		t.Errorf("problem paths =\n%v\nwant\n%v", got, paths)
	}
}

// A symlinked route named after its target's stem, and earlier in path
// order than the real location, must still lose to the real location —
// otherwise the node would load at the route and the real folder would be
// reported as a collision.
func TestSymlink_RealLocationWinsOverEarlierRoute(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a/a.md":   note("a", ""),
		"b/b.md":   note("b", ""),
		"b/c/c.md": note("c", ""),
	})
	symlink(t, filepath.Join(root, "b"), filepath.Join(root, "a", "b"))
	symlink(t, filepath.Join(root, "b", "c"), filepath.Join(root, "a", "c"))
	s := Load(root)
	wantProblems(t, s, pk{SeverityError, "a/b"}, pk{SeverityError, "a/c"})
	if b := mustNode(t, s, "b"); b.Path != "b" || b.Parent() != nil {
		t.Errorf("b at %q, want its real location b", b.Path)
	}
	if c := mustNode(t, s, "c"); c.Path != "b/c" {
		t.Errorf("c at %q, want its real location b/c", c.Path)
	}
	wantStems(t, mustNode(t, s, "a").Children())
	if r := s.Problems()[0].Reason; !strings.Contains(r, "real location is b") {
		t.Errorf("reason %q should name the real location", r)
	}
}

// A real location wins even when it is skipped: the symlinked route is
// still an error and never walked.
func TestSymlink_RouteToSkippedRealDir(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a/a.md": note("a", ""),
		"b/x.md": note("wrong name", ""),
	})
	symlink(t, filepath.Join(root, "b"), filepath.Join(root, "a", "b"))
	s := Load(root)
	wantProblems(t, s, pk{SeverityError, "a/b"}, pk{SeverityError, "b"})
	wantStems(t, s.Nodes(), "a")
}

func TestSymlink_BrokenSymlinkIsStrayWarning(t *testing.T) {
	root := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	symlink(t, filepath.Join(root, "nowhere"), filepath.Join(root, "a", "dangling"))
	s := Load(root)
	wantProblems(t, s, pk{SeverityWarning, "a/dangling"})
	wantStems(t, s.Nodes(), "a")
}

func TestSymlink_SymlinkToFileIsStray(t *testing.T) {
	root := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	other := filepath.Join(t.TempDir(), "f.txt")
	writeFile(t, other, "x")
	symlink(t, other, filepath.Join(root, "a", "link.txt"))
	s := Load(root)
	wantProblems(t, s, pk{SeverityWarning, "a/link.txt"})
}

// Criterion 9: only regular files are read, and nothing that could block
// is ever opened.
func TestSymlink_FolderNoteNamedPipe(t *testing.T) {
	root := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	f := filepath.Join(root, "f")
	mkdir(t, f)
	if err := syscall.Mkfifo(filepath.Join(f, "f.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	s := loadWithin(t, root, 5*time.Second)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Load took %v", d)
	}
	wantProblems(t, s, pk{SeverityError, "f"})
	wantStems(t, s.Nodes(), "a")
}

func TestSymlink_FolderNoteToDevZero(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero")
	}
	root := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	f := filepath.Join(root, "f")
	mkdir(t, f)
	symlink(t, "/dev/zero", filepath.Join(f, "f.md"))
	start := time.Now()
	s := loadWithin(t, root, 5*time.Second)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Load took %v", d)
	}
	wantProblems(t, s, pk{SeverityError, "f"})
	wantStems(t, s.Nodes(), "a")
}

func TestSymlink_FolderNoteToRegularFile(t *testing.T) {
	root := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	elsewhere := filepath.Join(t.TempDir(), "real.md")
	writeFile(t, elsewhere, note("Linked note", "Body. ^m-f00001\n"))
	f := filepath.Join(root, "f")
	mkdir(t, f)
	symlink(t, elsewhere, filepath.Join(f, "f.md"))
	s := Load(root)
	wantProblems(t, s)
	fn := mustNode(t, s, "f")
	if fn.Description != "Linked note" || len(fn.Blocks) != 1 {
		t.Errorf("f = %q %#v", fn.Description, fn.Blocks)
	}
}

func TestSymlink_BrokenFolderNote(t *testing.T) {
	root := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	f := filepath.Join(root, "f")
	mkdir(t, f)
	symlink(t, filepath.Join(root, "gone.md"), filepath.Join(f, "f.md"))
	s := Load(root)
	wantProblems(t, s, pk{SeverityError, "f"})
	wantStems(t, s.Nodes(), "a")
}

// A stray named pipe in a node folder is reported, never opened.
func TestSymlink_StrayNamedPipe(t *testing.T) {
	root := writeTree(t, map[string]string{"a/a.md": note("a", "")})
	if err := syscall.Mkfifo(filepath.Join(root, "a", "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadWithin(t, root, 2*time.Second)
	wantProblems(t, s, pk{SeverityWarning, "a/pipe"})
}
