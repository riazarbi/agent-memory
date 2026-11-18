package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Criterion 1: a valid tree loads with its shape; dot-entries are neither
// loaded nor reported; tags and extra properties come through; modified is
// the folder note's mtime, not the folder's.
func TestLoad_ValidTreeShape(t *testing.T) {
	root := writeTree(t, map[string]string{
		"user/user.md":     note("Who the operator is", ""),
		"topics/topics.md": note("Everything else worth knowing", ""),
		"topics/outbox-etiquette/outbox-etiquette.md": fmNote(
			"description: How and when to cc people\ntags: [\"#escalation\", outbox]\npriority: 3\n",
			"Forward urgent mail via outbox cc.\n"),
		".obsidian/app.json":     "{}",
		".DS_Store":              "junk",
		"topics/.DS_Store":       "junk",
		".trash/old/old.md":      "not a note at all",
		"user/.obsidian-tmp.txt": "x",
	})
	notePath := filepath.Join(root, "topics", "outbox-etiquette", "outbox-etiquette.md")
	noteTime := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	dirTime := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	if err := os.Chtimes(notePath, noteTime, noteTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Dir(notePath), dirTime, dirTime); err != nil {
		t.Fatal(err)
	}

	s := Load(root)

	wantProblems(t, s)
	wantStems(t, s.Nodes(), "topics", "outbox-etiquette", "user")
	wantStems(t, s.FirstLevel(), "topics", "user")
	checkShape(t, s)
	checkOutboxEtiquette(t, mustNode(t, s, "outbox-etiquette"), noteTime)
}

func checkShape(t *testing.T, s *Store) {
	t.Helper()
	oe := mustNode(t, s, "outbox-etiquette")
	topics := mustNode(t, s, "topics")
	if oe.Parent() != topics {
		t.Errorf("outbox-etiquette parent = %v, want topics", oe.Parent())
	}
	if p := topics.Parent(); p != nil {
		t.Errorf("topics parent = %q, want none", p.Stem)
	}
	wantStems(t, topics.Children(), "outbox-etiquette")
	wantStems(t, mustNode(t, s, "user").Children())
	if oe.Path != "topics/outbox-etiquette" {
		t.Errorf("Path = %q", oe.Path)
	}
}

func checkOutboxEtiquette(t *testing.T, oe *Node, noteTime time.Time) {
	t.Helper()
	if oe.Description != "How and when to cc people" {
		t.Errorf("Description = %q", oe.Description)
	}
	if strings.Join(oe.Tags, ",") != "escalation,outbox" {
		t.Errorf("Tags = %q", oe.Tags)
	}
	if v, ok := oe.Property("priority"); !ok || v != 3 {
		t.Errorf("priority = %#v, %v; want 3", v, ok)
	}
	for _, reserved := range []string{"description", "tags"} {
		if _, ok := oe.Property(reserved); ok {
			t.Errorf("%s must not appear among the other properties", reserved)
		}
	}
	if !oe.Modified.Equal(noteTime) {
		t.Errorf("Modified = %v, want folder note mtime %v", oe.Modified, noteTime)
	}
	if len(oe.Blocks) != 1 || oe.Blocks[0].Text != "Forward urgent mail via outbox cc." {
		t.Errorf("Blocks = %#v", oe.Blocks)
	}
}

// Children and first-level nodes are ordered by stem, and every child sees
// its parent.
func TestLoad_ChildrenOrderedByStem(t *testing.T) {
	root := writeTree(t, map[string]string{
		"zeta/zeta.md":         note("z", ""),
		"alpha/alpha.md":       note("a", ""),
		"alpha/m-two/m-two.md": note("m", ""),
		"alpha/b2/b2.md":       note("b", ""),
		"alpha/b10/b10.md":     note("b10", ""),
	})
	s := Load(root)
	wantProblems(t, s)
	wantStems(t, s.FirstLevel(), "alpha", "zeta")
	wantStems(t, mustNode(t, s, "alpha").Children(), "b10", "b2", "m-two")
	for _, c := range mustNode(t, s, "alpha").Children() {
		if c.Parent() == nil || c.Parent().Stem != "alpha" {
			t.Errorf("%s parent wrong", c.Stem)
		}
	}
}

// Criterion 2: a skipped subtree yields exactly one problem, whatever is
// inside it, and nothing from inside it loads.
func TestLoad_SkippedSubtreeOneProblem(t *testing.T) {
	people := "No frontmatter here.\n"
	cases := []struct {
		name  string
		files map[string]string
		top   string
	}{
		{"a missing folder note", map[string]string{
			"projects/a/a.md": note("a", ""),
		}, "projects"},
		{"b child without folder note", map[string]string{
			"people/people.md": people,
			"people/kid/x.md":  note("x", ""),
		}, "people"},
		{"c child with invalid stem", map[string]string{
			"people/people.md":      people,
			"people/Bad_X/Bad_X.md": note("bad", ""),
		}, "people"},
		{"d stray file", map[string]string{
			"people/people.md":  people,
			"people/readme.txt": "hello",
		}, "people"},
		{"e malformed child", map[string]string{
			"people/people.md":  people,
			"people/kid/kid.md": "No frontmatter either.\n",
		}, "people"},
		{"invalid stem alone", map[string]string{
			"Bad_Name/Bad_Name.md": note("bad", ""),
		}, "Bad_Name"},
		{"invalid stem with broken children", map[string]string{
			"Bad_Name/Bad_Name.md":   note("bad", ""),
			"Bad_Name/kid/stray.txt": "x",
			"Bad_Name/good/good.md":  "no frontmatter",
		}, "Bad_Name"},
		{"valid parent stem, collision-free, broken deep", map[string]string{
			"projects/x.txt":        "x",
			"projects/deep/deep.md": note("deep", ""),
		}, "projects"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Load(writeTree(t, tc.files))
			wantProblems(t, s, pk{SeverityError, tc.top})
			wantStems(t, s.Nodes())
		})
	}
}

// Criterion 3: stem collisions — the first loadable folder in component
// order keeps the stem; the later one is skipped with one error and nothing
// beneath it is reported.
func TestLoad_StemCollisionSkipsLaterSubtree(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"topics/topics.md":      note("t", ""),
		"topics/dup/dup.md":     note("first", ""),
		"user/user.md":          note("u", ""),
		"user/dup/dup.md":       note("second", ""),
		"user/dup/kid/other.md": note("orphan", ""),
		"user/dup/stray.txt":    "x",
	}))
	wantProblems(t, s, pk{SeverityError, "user/dup"})
	if d := mustNode(t, s, "dup"); d.Path != "topics/dup" || d.Description != "first" {
		t.Errorf("dup = %q %q, want topics/dup first", d.Path, d.Description)
	}
	wantStems(t, mustNode(t, s, "user").Children())
	if r := s.Problems()[0].Reason; !strings.Contains(r, "topics/dup") {
		t.Errorf("collision reason %q should name the folder that kept the stem", r)
	}
}

func TestLoad_StemCollisionComponentOrder(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a-b/a-b.md": note("ab", ""),
		"a-b/x/x.md": note("from a-b", ""),
		"a/a.md":     note("a", ""),
		"a/x/x.md":   note("from a", ""),
	}))
	wantProblems(t, s, pk{SeverityError, "a-b/x"})
	if x := mustNode(t, s, "x"); x.Path != "a/x" {
		t.Errorf("x.Path = %q, want a/x (component order)", x.Path)
	}
}

func TestLoad_StemCollisionMalformedClaimsNothing(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"topics/topics.md":  note("t", ""),
		"topics/dup/dup.md": "no frontmatter\n",
		"user/user.md":      note("u", ""),
		"user/dup/dup.md":   note("valid", ""),
	}))
	wantProblems(t, s, pk{SeverityError, "topics/dup"})
	if d := mustNode(t, s, "dup"); d.Path != "user/dup" {
		t.Errorf("dup.Path = %q, want user/dup", d.Path)
	}
	wantStems(t, mustNode(t, s, "user").Children(), "dup")
	wantStems(t, mustNode(t, s, "topics").Children())
}

// A first-level folder collides too: the stem is unique store-wide.
func TestLoad_StemCollisionAcrossDepths(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"aa/aa.md":      note("aa", ""),
		"aa/zz/zz.md":   note("nested", ""),
		"zz/zz.md":      note("top", ""),
		"zz/kid/kid.md": note("kid", ""),
	}))
	wantProblems(t, s, pk{SeverityError, "zz"})
	if z := mustNode(t, s, "zz"); z.Path != "aa/zz" {
		t.Errorf("zz.Path = %q", z.Path)
	}
	wantAbsent(t, s, "kid")
}

func TestLoad_StrayFilesWarnAndChangeNothing(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"topics/topics.md": note("t", ""),
		"topics/notes.txt": "stray",
		"notes.md":         note("root note", ""),
		"user/user.md":     note("u", ""),
		"user/dup/dup.md":  note("d", ""),
	}))
	wantProblems(t, s,
		pk{SeverityWarning, "notes.md"},
		pk{SeverityWarning, "topics/notes.txt"})
	wantStems(t, s.Nodes(), "topics", "user", "dup")
	wantAbsent(t, s, "notes")
}

// Problems are ordered by path (component by component), then found order.
func TestLoad_ProblemsOrderedByPath(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"b/b.md":     note("b", ""),
		"b/z.txt":    "x",
		"b/a.txt":    "x",
		"a-b/a-b.md": note("ab", ""),
		"a-b/s.txt":  "x",
		"a/a.md":     note("a", ""),
		"a/x/x.md":   note("x", ""),
		"a/x/s.txt":  "x",
		"zz.txt":     "x",
	}))
	wantProblems(t, s,
		pk{SeverityWarning, "a/x/s.txt"},
		pk{SeverityWarning, "a-b/s.txt"},
		pk{SeverityWarning, "b/a.txt"},
		pk{SeverityWarning, "b/z.txt"},
		pk{SeverityWarning, "zz.txt"})
}

// Criterion 9: loading is total.
func TestLoad_MissingRootIsEmpty(t *testing.T) {
	s := Load(filepath.Join(t.TempDir(), "notes"))
	wantProblems(t, s)
	wantStems(t, s.Nodes())
	wantStems(t, s.FirstLevel())
}

func TestLoad_RootIsAFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notes")
	writeFile(t, p, "not a dir")
	s := Load(p)
	wantProblems(t, s, pk{SeverityError, "."})
	wantStems(t, s.Nodes())
}

func TestLoad_EveryNodeMalformed(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md":       "no frontmatter",
		"a/kid/kid.md": note("kid", ""),
		"b/c/c.md":     note("c", ""),
		"c/c.md":       fmNote("tags: [x]\n", ""),
		"d/d.md":       note("d", "## heading\n"),
	}))
	wantProblems(t, s,
		pk{SeverityError, "a"},
		pk{SeverityError, "b"},
		pk{SeverityError, "c"},
		pk{SeverityError, "d"})
	wantStems(t, s.Nodes())
}

func TestLoad_UnreadableFolderNote(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	root := writeTree(t, map[string]string{
		"a/a.md":       note("a", ""),
		"a/kid/kid.md": note("kid", ""),
		"b/b.md":       note("b", ""),
	})
	p := filepath.Join(root, "a", "a.md")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	s := Load(root)
	wantProblems(t, s, pk{SeverityError, "a"})
	wantStems(t, s.Nodes(), "b")
}

func TestLoad_UnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	root := writeTree(t, map[string]string{
		"a/a.md":       note("a", ""),
		"a/kid/kid.md": note("kid", ""),
		"b/b.md":       note("b", ""),
	})
	d := filepath.Join(root, "a")
	if err := os.Chmod(d, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(d, 0o755) })
	s := Load(root)
	wantProblems(t, s, pk{SeverityError, "a"})
	wantStems(t, s.Nodes(), "b")
}

// A folder note that is a directory is not a regular file: missing.
func TestLoad_FolderNoteIsDirectory(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a/a.md/inner.txt": "x",
		"b/b.md":           note("b", ""),
	})
	s := Load(root)
	wantProblems(t, s, pk{SeverityError, "a"})
	wantStems(t, s.Nodes(), "b")
}

func TestLoad_BOMAndCRLFTolerated(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": "\ufeff---\r\ndescription: Windows note\r\ntags: [x]\r\n---\r\nLine one\r\nline two ^m-abc123\r\n\r\nSecond.\r\n",
	}))
	wantProblems(t, s)
	a := mustNode(t, s, "a")
	if a.Description != "Windows note" {
		t.Errorf("Description = %q", a.Description)
	}
	if len(a.Blocks) != 2 || a.Blocks[0].Text != "Line one\nline two" || a.Blocks[0].ID != "m-abc123" {
		t.Errorf("Blocks = %#v", a.Blocks)
	}
}

func TestLoad_StemRules(t *testing.T) {
	long := strings.Repeat("a", 65)
	ok64 := strings.Repeat("b", 64)
	s := Load(writeTree(t, map[string]string{
		long + "/" + long + ".md": note("too long", ""),
		ok64 + "/" + ok64 + ".md": note("max length", ""),
		"a--b/a--b.md":            note("double hyphen", ""),
		"-a/-a.md":                note("leading hyphen", ""),
		"a-/a-.md":                note("trailing hyphen", ""),
		"k8s-2/k8s-2.md":          note("fine", ""),
	}))
	wantProblems(t, s,
		pk{SeverityError, "-a"},
		pk{SeverityError, "a-"},
		pk{SeverityError, "a--b"},
		pk{SeverityError, long})
	wantStems(t, s.Nodes(), ok64, "k8s-2")
}

// U4 criterion 4: the folder note is matched by exact, case-sensitive name
// on every platform. On a case-insensitive filesystem (macOS) a/A.md would
// open as a/a.md; it must still count as missing, and must not also be
// reported as a stray file.
func TestLoad_FolderNoteMatchedByExactName(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/A.md":       note("wrong case", ""),
		"a/kid/kid.md": note("kid", ""),
		"b/b.md":       note("b", ""),
	}))
	wantProblems(t, s, pk{SeverityError, "a"})
	if r := s.Problems()[0].Reason; !strings.Contains(r, "missing folder note a.md") {
		t.Errorf("reason %q, want a missing folder note", r)
	}
	wantStems(t, s.Nodes(), "b")

	s = Load(writeTree(t, map[string]string{"a/a.md": note("right case", "")}))
	wantProblems(t, s)
	if a := mustNode(t, s, "a"); a.Description != "right case" {
		t.Errorf("Description = %q", a.Description)
	}
}

// The exact-name rule holds for a nested folder and for a folder reached
// through a symlink to outside notes/.
func TestLoad_FolderNoteExactNameNestedAndBound(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "shared")
	writeFile(t, filepath.Join(shared, "Shared.md"), note("wrong case", ""))
	root := writeTree(t, map[string]string{
		"p/p.md":     note("p", ""),
		"p/q/Q.md":   note("wrong case", ""),
		"p/q/x/x.md": note("x", ""),
	})
	symlink(t, shared, filepath.Join(root, "p", "shared"))
	s := Load(root)
	wantProblems(t, s, pk{SeverityError, "p/q"}, pk{SeverityError, "p/shared"})
	wantStems(t, s.Nodes(), "p")
}
