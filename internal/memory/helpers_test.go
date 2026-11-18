package memory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeTree builds a notes/ directory under a fresh temp dir from a map of
// slash-separated paths (relative to notes/) to file contents, and returns
// the notes/ path. Directories are created as needed.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// note returns a folder note with the given description and body.
func note(desc, body string) string {
	return "---\ndescription: " + desc + "\n---\n" + body
}

// fmNote returns a folder note with raw frontmatter lines and a body.
func fmNote(frontmatter, body string) string {
	return "---\n" + frontmatter + "---\n" + body
}

type pk struct {
	Severity Severity
	Path     string
}

func keys(ps []Problem) []pk {
	out := make([]pk, 0, len(ps))
	for _, p := range ps {
		out = append(out, pk{p.Severity, p.Path})
	}
	return out
}

func stemsOf(nodes []*Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Stem)
	}
	return out
}

// wantProblems fails unless the store's problems are exactly want, in order.
func wantProblems(t *testing.T, s *Store, want ...pk) {
	t.Helper()
	got := keys(s.Problems())
	if want == nil {
		want = []pk{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %v, want %v\nfull: %s", got, want, dump(s.Problems()))
	}
}

func dump(ps []Problem) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString("\n  " + string(p.Severity) + " " + p.Path + ": " + p.Reason)
	}
	return b.String()
}

func mustNode(t *testing.T, s *Store, stem string) *Node {
	t.Helper()
	n, ok := s.Node(stem)
	if !ok {
		t.Fatalf("node %q not loaded; nodes %v; problems %s", stem, stemsOf(s.Nodes()), dump(s.Problems()))
	}
	return n
}

func wantAbsent(t *testing.T, s *Store, stems ...string) {
	t.Helper()
	for _, stem := range stems {
		if _, ok := s.Node(stem); ok {
			t.Errorf("node %q loaded, want absent", stem)
		}
	}
}

func wantStems(t *testing.T, got []*Node, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if g := stemsOf(got); !reflect.DeepEqual(g, want) {
		t.Fatalf("stems = %v, want %v", g, want)
	}
}
