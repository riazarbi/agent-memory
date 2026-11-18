package memory

import (
	"reflect"
	"strings"
	"testing"
)

// Criterion 7: links and backlinks.
func TestLinks_EdgesAndBacklinks(t *testing.T) {
	block := "See [[b]], [[b|Bee]], [[c#^m-cccccc]], `[[b]]`, [[missing]], [[c#^m-zzzzzz]], " +
		"[[c#Heading]], [[topics/c]], [[c.md]] and ![[c]]. ^m-aaaaa1"
	s := Load(writeTree(t, map[string]string{
		"a/a.md": fmNote("description: a\nrelated: \"[[b]]\"\n", block+"\n"),
		"b/b.md": note("b", "Plain.\n"),
		"c/c.md": note("c", "Target block. ^m-cccccc\n\nOther. ^m-dddddd\n"),
	}))

	wantEdges := []Edge{
		{Stem: "b"},
		{Stem: "b"},
		{Stem: "c", Block: "m-cccccc"},
		{Stem: "missing", Dangling: true},
		{Stem: "c", Block: "m-zzzzzz", Dangling: true},
	}
	a := mustNode(t, s, "a")
	if got := a.Edges(); !reflect.DeepEqual(got, wantEdges) {
		t.Errorf("node edges = %#v\nwant %#v", got, wantEdges)
	}
	if got, _ := s.BlockEdges("a", "m-aaaaa1"); !reflect.DeepEqual(got, wantEdges) {
		t.Errorf("block edges = %#v", got)
	}

	// 4 non-edge forms + 2 dangling edges, all on a; nothing for the
	// inline-code link or the frontmatter.
	wantProblems(t, s,
		pk{SeverityWarning, "a"}, pk{SeverityWarning, "a"}, pk{SeverityWarning, "a"},
		pk{SeverityWarning, "a"}, pk{SeverityWarning, "a"}, pk{SeverityWarning, "a"})

	text := a.Blocks[0].Text
	for _, plain := range []string{"[[c#Heading]]", "[[topics/c]]", "[[c.md]]", "![[c]]", "`[[b]]`"} {
		if !strings.Contains(text, plain) {
			t.Errorf("block text lost %q: %q", plain, text)
		}
	}

	src := []Ref{{Stem: "a", Block: "m-aaaaa1"}}
	if got := s.Backlinks("b"); !reflect.DeepEqual(got, src) {
		t.Errorf("b backlinks = %#v, want %#v", got, src)
	}
	if got := s.Backlinks("c"); !reflect.DeepEqual(got, src) {
		t.Errorf("c backlinks = %#v, want %#v", got, src)
	}
	if got := s.BlockBacklinks("c", "m-cccccc"); !reflect.DeepEqual(got, src) {
		t.Errorf("c#m-cccccc backlinks = %#v, want %#v", got, src)
	}
	if got := s.BlockBacklinks("c", "m-dddddd"); len(got) != 0 {
		t.Errorf("c#m-dddddd backlinks = %#v, want none", got)
	}
	if got := s.Backlinks("a"); len(got) != 0 {
		t.Errorf("a backlinks = %#v, want none", got)
	}
}

func TestLinks_WarningsNameTheLink(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("a", "[[missing]] and ![[a]]\n"),
	}))
	ps := s.Problems()
	if len(ps) != 2 {
		t.Fatalf("problems %s", dump(ps))
	}
	reasons := ps[0].Reason + "\n" + ps[1].Reason
	for _, want := range []string{"[[missing]]", "![[a]]"} {
		if !strings.Contains(reasons, want) {
			t.Errorf("no problem quotes %q: %s", want, dump(ps))
		}
	}
}

// Backlinks are unique (stem, block) pairs ordered by stem then block ID;
// an unlabelled source block contributes (stem, "").
func TestLinks_BacklinkOrderingAndUnlabelledSources(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"t/t.md": note("t", "Target. ^m-tttttt\n"),
		"z/z.md": note("z", "[[t]] ^m-zzzzz2\n\n[[t#^m-tttttt]] ^m-zzzzz1\n\nAgain [[t]] [[t|x]] ^m-zzzzz1\n"),
		"m/m.md": note("m", "No id [[t]].\n\nAlso no id [[t#^m-tttttt|alias]].\n"),
	}))
	wantProblems(t, s, pk{SeverityWarning, "z"}) // duplicate m-zzzzz1 in z
	want := []Ref{{"m", ""}, {"z", "m-zzzzz1"}, {"z", "m-zzzzz2"}}
	if got := s.Backlinks("t"); !reflect.DeepEqual(got, want) {
		t.Errorf("t backlinks = %#v, want %#v", got, want)
	}
	wantBlock := []Ref{{"m", ""}, {"z", "m-zzzzz1"}}
	if got := s.BlockBacklinks("t", "m-tttttt"); !reflect.DeepEqual(got, wantBlock) {
		t.Errorf("t#m-tttttt backlinks = %#v, want %#v", got, wantBlock)
	}
}

// Dangling-link warnings are found after the walk but still sort by path.
func TestLinks_ProblemsSortedByPath(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md":  note("a", "[[nowhere]]\n"),
		"b/b.md":  note("b", ""),
		"b/x.txt": "stray",
	}))
	wantProblems(t, s, pk{SeverityWarning, "a"}, pk{SeverityWarning, "b/x.txt"})
}

// Links never come from a skipped node, and a link into a skipped node is
// dangling.
func TestLinks_SkippedNodesContributeNothing(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("a", "[[b]] ^m-aaaaaa\n"),
		"b/b.md": "no frontmatter [[a]]\n",
	}))
	wantProblems(t, s, pk{SeverityWarning, "a"}, pk{SeverityError, "b"})
	if got := s.Backlinks("a"); len(got) != 0 {
		t.Errorf("a backlinks = %#v", got)
	}
	if e := mustNode(t, s, "a").Edges(); len(e) != 1 || !e[0].Dangling {
		t.Errorf("edges = %#v", e)
	}
}

func TestLinks_InlineCodeSpans(t *testing.T) {
	cases := []struct {
		name, text string
		edges      []string
	}{
		{"single backticks", "`[[b]]` [[c]]", []string{"c"}},
		{"double backticks", "``code ` [[b]]`` [[c]]", []string{"c"}},
		{"unmatched backtick is literal", "a ` b [[c]]", []string{"c"}},
		{"span across lines", "`start\n[[b]] end` [[c]]", []string{"c"}},
		{"mismatched run lengths", "``[[b]]` [[c]]", []string{"b", "c"}},
		{"longer run inside a span", "`x``[[b]]` [[c]]", []string{"c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Load(writeTree(t, map[string]string{
				"a/a.md": note("a", tc.text+"\n"),
				"b/b.md": note("b", ""),
				"c/c.md": note("c", ""),
			}))
			wantProblems(t, s)
			var got []string
			for _, e := range mustNode(t, s, "a").Edges() {
				got = append(got, e.Stem)
			}
			if !reflect.DeepEqual(got, tc.edges) {
				t.Errorf("edges = %v, want %v", got, tc.edges)
			}
		})
	}
}

func TestLinks_EdgeForms(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("a", "[[c#^m-cccccc|alias]] [[c|]] [[c#^a1b2c3]] [[C]] [[c#^m-cccccc#x]]\n"),
		"c/c.md": note("c", "T. ^m-cccccc\n"),
	}))
	wantProblems(t, s, pk{SeverityWarning, "a"}, pk{SeverityWarning, "a"}, pk{SeverityWarning, "a"})
	want := []Edge{{Stem: "c", Block: "m-cccccc"}, {Stem: "c"}}
	if got := mustNode(t, s, "a").Edges(); !reflect.DeepEqual(got, want) {
		t.Errorf("edges = %#v, want %#v", got, want)
	}
}

// U4 criterion 5(a): a block link to an existing node but a missing block
// is dangling — one warning — and is not a backlink of the node.
func TestLinks_DanglingBlockLinkIsNotANodeBacklink(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"a/a.md": note("a", "See [[c#^m-zzzzzz]]. ^m-aaaaaa\n"),
		"c/c.md": note("c", "Real block. ^m-cccccc\n"),
	}))
	wantProblems(t, s, pk{SeverityWarning, "a"})
	if r := s.Problems()[0].Reason; !strings.Contains(r, "[[c#^m-zzzzzz]]") {
		t.Errorf("warning %q should quote the dangling link", r)
	}
	want := []Edge{{Stem: "c", Block: "m-zzzzzz", Dangling: true}}
	if got := mustNode(t, s, "a").Edges(); !reflect.DeepEqual(got, want) {
		t.Errorf("edges = %#v, want %#v", got, want)
	}
	if got := s.Backlinks("c"); len(got) != 0 {
		t.Errorf("c backlinks = %#v, want none", got)
	}
	if got := s.BlockBacklinks("c", "m-zzzzzz"); len(got) != 0 {
		t.Errorf("c#m-zzzzzz backlinks = %#v, want none", got)
	}
	if got := s.BlockBacklinks("c", "m-cccccc"); len(got) != 0 {
		t.Errorf("c#m-cccccc backlinks = %#v, want none", got)
	}
}
