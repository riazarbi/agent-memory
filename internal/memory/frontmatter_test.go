package memory

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Criterion 4: each malformed frontmatter skips its node with one error,
// while a valid sibling still loads.
func TestFrontmatter_Rejected(t *testing.T) {
	cases := []struct{ name, content string }{
		{"no frontmatter", "Just a paragraph.\n"},
		{"unclosed frontmatter", "---\ndescription: d\nbody\n"},
		{"unparseable yaml", fmNote("description: [unclosed\n", "")},
		{"not a mapping", fmNote("- a\n- b\n", "")},
		{"empty frontmatter", fmNote("", "")},
		{"no description", fmNote("tags: [a]\n", "")},
		{"empty description", fmNote("description: \"\"\n", "")},
		{"null description", fmNote("description:\n", "")},
		{"two-line description (escaped newline)", fmNote("description: \"one\\ntwo\"\n", "")},
		{"two-line description (folded paragraphs)", fmNote("description: >\n  one\n\n  two\n", "")},
		{"literal description", fmNote("description: |\n  one\n  two\n", "")},
		{"numeric description", fmNote("description: 123\n", "")},
		{"list description", fmNote("description: [a, b]\n", "")},
		{"tag with a space", fmNote("description: d\ntags: [\"two words\"]\n", "")},
		{"numeric tag", fmNote("description: d\ntags: [\"123\"]\n", "")},
		{"unquoted numeric tag", fmNote("description: d\ntags: [123]\n", "")},
		{"tag with punctuation", fmNote("description: d\ntags: [\"a.b\"]\n", "")},
		{"empty tag", fmNote("description: d\ntags: [\"#\"]\n", "")},
		{"tags as mapping", fmNote("description: d\ntags:\n  a: b\n", "")},
		{"nested mapping property", fmNote("description: d\nmeta:\n  a: 1\n", "")},
		{"list containing a mapping", fmNote("description: d\nitems:\n  - a: 1\n", "")},
		{"list containing a list", fmNote("description: d\nitems: [[a]]\n", "")},
		{"duplicate key", fmNote("description: d\nx: 1\nx: 2\n", "")},
		{"duplicate description", fmNote("description: d\ndescription: e\n", "")},
		{"binary value", fmNote("description: d\nblob: !!binary aGVsbG8=\n", "")},
		{"custom tag", fmNote("description: d\nthing: !foo x\n", "")},
		{"non-string key", fmNote("description: d\n1: x\n", "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Load(writeTree(t, map[string]string{
				"bad/bad.md": tc.content,
				"ok/ok.md":   note("valid sibling", ""),
			}))
			wantProblems(t, s, pk{SeverityError, "bad"})
			wantStems(t, s.Nodes(), "ok")
		})
	}
}

func loadOne(t *testing.T, content string) *Node {
	t.Helper()
	s := Load(writeTree(t, map[string]string{"n/n.md": content}))
	wantProblems(t, s)
	return mustNode(t, s, "n")
}

func TestFrontmatter_DescriptionForms(t *testing.T) {
	cases := []struct{ name, fm, want string }{
		{"plain", "description: How and when\n", "How and when"},
		{"wrapped plain scalar", "description: How and when\n  to cc people\n", "How and when to cc people"},
		{"folded block", "description: >\n  How and when\n  to cc people\n", "How and when to cc people"},
		{"quoted", "description: \"Quoted: with colon\"\n", "Quoted: with colon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadOne(t, fmNote(tc.fm, "")).Description; got != tc.want {
				t.Errorf("Description = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFrontmatter_TagForms(t *testing.T) {
	cases := []struct {
		name, fm string
		want     []string
	}{
		{"legacy bare string", "tags: solo\n", []string{"solo"}},
		{"empty list", "tags: []\n", nil},
		{"empty value", "tags:\n", nil},
		{"hash stripped", "tags: [\"#escalation\", outbox]\n", []string{"escalation", "outbox"}},
		{"nested and unicode", "tags: [people/priya, café_2, \"#año-nuevo\", y1984]\n",
			[]string{"people/priya", "café_2", "año-nuevo", "y1984"}},
		{"block list", "tags:\n  - a\n  - b\n", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := loadOne(t, fmNote("description: d\n"+tc.fm, ""))
			if len(n.Tags) != len(tc.want) || (len(tc.want) > 0 && !reflect.DeepEqual(n.Tags, tc.want)) {
				t.Errorf("Tags = %#v, want %#v", n.Tags, tc.want)
			}
		})
	}
}

func TestFrontmatter_PropertiesPreserved(t *testing.T) {
	n := loadOne(t, fmNote(`description: d
s: some text
l: [a, b]
i: 42
f: 1.5
b: true
d: 2026-11-14
dq: "2026-11-14"
dt: 2026-11-14T10:30:00+02:00
dtq: "2026-11-14T10:30:00+02:00"
e:
dates: [2026-11-14, true, 3]
multi: |
  line one
  line two
`, ""))
	plus2 := time.FixedZone("", 2*60*60)
	want := []struct {
		key  string
		want any
	}{
		{"s", "some text"},
		{"l", []string{"a", "b"}},
		{"i", 42},
		{"f", 1.5},
		{"b", true},
		{"d", time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)},
		{"dq", "2026-11-14"},
		{"dt", time.Date(2026, 11, 14, 10, 30, 0, 0, plus2)},
		{"dtq", "2026-11-14T10:30:00+02:00"},
		{"e", nil},
		{"dates", []string{"2026-11-14", "true", "3"}},
		{"multi", "line one\nline two\n"},
	}
	gotKeys := make([]string, 0, len(n.Properties))
	for _, p := range n.Properties {
		gotKeys = append(gotKeys, p.Key)
	}
	wantKeys := make([]string, 0, len(want))
	for _, w := range want {
		wantKeys = append(wantKeys, w.key)
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("property keys = %v, want %v (file order, description and tags excluded)", gotKeys, wantKeys)
	}
	for _, w := range want {
		got, _ := n.Property(w.key)
		if !sameValue(got, w.want) {
			t.Errorf("%s = %#v (%T), want %#v (%T)", w.key, got, got, w.want, w.want)
		}
	}
}

func sameValue(got, want any) bool {
	if wt, ok := want.(time.Time); ok {
		gt, ok := got.(time.Time)
		if !ok || !gt.Equal(wt) {
			return false
		}
		_, gotOff := gt.Zone()
		_, wantOff := wt.Zone()
		return gotOff == wantOff
	}
	return reflect.DeepEqual(got, want)
}

// The rejection reason names what is wrong, so an operator can fix it.
func TestFrontmatter_ReasonsAreSpecific(t *testing.T) {
	cases := []struct{ content, want string }{
		{"Just text\n", "frontmatter"},
		{fmNote("tags: [a]\n", ""), "description"},
		{fmNote("description: d\ntags: [\"two words\"]\n", ""), "two words"},
		{fmNote("description: d\nx: 1\nx: 2\n", ""), `"x"`},
		{fmNote("description: d\nmeta:\n  a: 1\n", ""), "meta"},
	}
	for _, tc := range cases {
		s := Load(writeTree(t, map[string]string{"bad/bad.md": tc.content}))
		ps := s.Problems()
		if len(ps) != 1 || !strings.Contains(ps[0].Reason, tc.want) {
			t.Errorf("content %q: problems %s; want one mentioning %q", tc.content, dump(ps), tc.want)
		}
	}
}

// U4 criterion 3: on a list only the plain sequence tag (implicit or an
// explicit !!seq) is allowed; any other tag on a list skips the node with
// one error while a valid sibling loads.
func TestFrontmatter_TagOnAListIsChecked(t *testing.T) {
	rejected := []string{
		"k: !foo [a, b]\n",
		"tags: !foo [a]\n",
		"k: !!binary [a]\n",
		"k: !!set [a]\n",
		"k: !!str [a]\n",
		"tags: !!str [a]\n",
		"k: !foo\n  - a\n",
	}
	for _, fm := range rejected {
		t.Run(fm, func(t *testing.T) {
			s := Load(writeTree(t, map[string]string{
				"bad/bad.md": fmNote("description: d\n"+fm, ""),
				"ok/ok.md":   note("valid sibling", ""),
			}))
			wantProblems(t, s, pk{SeverityError, "bad"})
			wantStems(t, s.Nodes(), "ok")
		})
	}
	accepted := []struct {
		fm, key string
		want    []string
	}{
		{"k: !!seq [a, b]\n", "k", []string{"a", "b"}},
		{"k: [a, b]\n", "k", []string{"a", "b"}},
		{"k: !!seq\n  - a\n  - b\n", "k", []string{"a", "b"}},
		{"tags: !!seq [a, b]\n", "tags", []string{"a", "b"}},
	}
	for _, tc := range accepted {
		t.Run(tc.fm, func(t *testing.T) {
			n := loadOne(t, fmNote("description: d\n"+tc.fm, ""))
			got := n.Tags
			if tc.key != keyTags {
				v, _ := n.Property(tc.key)
				got, _ = v.([]string)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s = %#v, want %#v", tc.key, got, tc.want)
			}
		})
	}
}

// U4 criterion 5(b): a literal block holding a single line is still a
// literal block: the description is refused.
func TestFrontmatter_OneLineLiteralDescriptionRejected(t *testing.T) {
	s := Load(writeTree(t, map[string]string{
		"bad/bad.md": fmNote("description: |\n  only one line\n", ""),
		"ok/ok.md":   note("valid sibling", ""),
	}))
	wantProblems(t, s, pk{SeverityError, "bad"})
	wantStems(t, s.Nodes(), "ok")
	if r := s.Problems()[0].Reason; !strings.Contains(r, "description") {
		t.Errorf("reason %q should name the description", r)
	}
}
