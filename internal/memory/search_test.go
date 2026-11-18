package memory

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// describe renders a parsed query canonically: alternatives joined by
// " | ", terms by " ", each as kind(value) with a leading "-" when
// excluded. Values are shown as they are compared (lower case, every
// whitespace run one space, untrimmed), so `-OR` reads -word(or).
func describe(q *Query) string {
	alts := make([]string, 0, len(q.alts))
	for _, alt := range q.alts {
		terms := make([]string, 0, len(alt))
		for _, tm := range alt {
			s := ""
			if tm.exclude {
				s = "-"
			}
			switch tm.kind {
			case termWord:
				s += "word(" + foldText(tm.value) + ")"
			case termTag:
				s += "tag(" + foldText(tm.value) + ")"
			case termPath:
				s += "path(" + foldText(tm.value) + ")"
			case termFile:
				s += "file(" + foldText(tm.value) + ")"
			case termProperty:
				if tm.hasValue {
					s += "prop(" + foldText(tm.key) + ":" + foldText(tm.value) + ")"
				} else {
					s += "prop(" + foldText(tm.key) + ")"
				}
			}
			terms = append(terms, s)
		}
		alts = append(alts, strings.Join(terms, " "))
	}
	return strings.Join(alts, " | ")
}

// --- criterion 2: query syntax ---

func TestParseQuery(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{`a b OR c d`, `word(a) word(b) | word(c) word(d)`},
		{`"two words"`, `word(two words)`},
		{`-x -tag:y z`, `-word(x) -tag(y) word(z)`},
		{`tag:#work`, `tag(work)`},
		{`tag:work`, `tag(work)`},
		{`[status:in review]`, `prop(status:in review)`},
		{`[status:]`, `prop(status)`},
		{`[status]`, `prop(status)`},
		{`TAG:x`, `word(tag:x)`},
		{`Tag:x`, `word(tag:x)`},
		{`a or b`, `word(a) word(or) word(b)`},
		// Recognition happens on the raw token, before quotes go.
		{`"tag:x"`, `word(tag:x)`},
		{`"OR"`, `word(or)`},
		{`"-x"`, `word(-x)`},
		{`tag:##x`, `tag(#x)`},
		{`[[outbox-etiquette]]`, `word([[outbox-etiquette]])`},
		{`[status:"in review"]`, `prop(status:in review)`},
		{`[a:b]c`, `prop(a:b) word(c)`},
		{`x"y z"`, `word(xy z)`},
		{`a -OR`, `word(a) -word(or)`},
		// More of the pinned tokenizer and operator rules.
		{`path:"two words" file:x`, `path(two words) file(x)`},
		{`-[status:done] a`, `-prop(status:done) word(a)`},
		{`" cat"`, `word( cat)`},
		{`[ Status :  in review ]`, `prop(status:in review)`},
		{`a -[[link]]`, `word(a) -word([[link]])`},
		{"a\tb\n OR  c", `word(a) word(b) | word(c)`},
		{`"a   b"`, `word(a b)`},
		// Choices where D31 is silent (CHANGELOG): the bracket splits at the
		// first : outside quotes; trim happens before quotes go; an empty
		// value is [property]; tag: strips its # after quotes go.
		{`[k:""]`, `prop(k)`},
		{`[k:" v"]`, `prop(k: v)`},
		{`["a:b":c]`, `prop(a:b:c)`},
		{`tag:"#x"`, `tag(x)`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := ParseQuery(tc.query)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tc.query, err)
			}
			if got := describe(q); got != tc.want {
				t.Errorf("ParseQuery(%q) = %s, want %s", tc.query, got, tc.want)
			}
		})
	}
}

// Each of the six errors, D31's exact text.
func TestParseQuery_Errors(t *testing.T) {
	const (
		empty      = "query: empty"
		quote      = "query: unclosed quote"
		bracket    = "query: unclosed ["
		emptyTerm  = "query: empty term"
		orSides    = "query: OR needs a term on each side"
		exclusions = "query: nothing to match, only exclusions"
	)
	for _, tc := range []struct{ query, want string }{
		{``, empty},
		{`   `, empty},
		{"\t\n", empty},
		{`"abc`, quote},
		{`5"`, quote},
		{`[x`, bracket},
		{`[a:"x]y"]`, quote},
		{`[x "abc`, quote},
		{`[a"] [x`, quote},
		{`[a] [x`, bracket},
		{`""`, emptyTerm},
		{`"  "`, emptyTerm},
		{`-`, emptyTerm},
		{`tag:`, emptyTerm},
		{`tag:#`, emptyTerm},
		{`path:`, emptyTerm},
		{`file:`, emptyTerm},
		{`[]`, emptyTerm},
		{`[:x]`, emptyTerm},
		{`OR a`, orSides},
		{`a OR`, orSides},
		{`a OR OR b`, orSides},
		{`-a -b`, exclusions},
		{`a OR -b`, exclusions},
		// Precedence: the first failing check, in D31's order.
		{`- "abc`, quote},
		{`OR "abc`, quote},
		{`[x - OR`, bracket},
		{`- OR a`, emptyTerm},
		{`OR -a`, orSides},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := ParseQuery(tc.query)
			if err == nil {
				t.Fatalf("ParseQuery(%q) = %s, want error %q", tc.query, describe(q), tc.want)
			}
			if err.Error() != tc.want {
				t.Errorf("ParseQuery(%q) error = %q, want %q", tc.query, err.Error(), tc.want)
			}
			var qe *QueryError
			if !errors.As(err, &qe) {
				t.Errorf("error %T is not a *QueryError", err)
			}
		})
	}
}

// --- criterion 3: matching ---

// searchFixture is criterion 3's store, every folder note at its own
// controlled mtime.
func searchFixture(t *testing.T) *Store {
	t.Helper()
	files := map[string]string{
		"notes.md":              "rootonlyword lives only in the root\n",
		"user/user.md":          note("Who the operator is", ""),
		"people/people.md":      note("Who is who", ""),
		"projects/projects.md":  note("Ongoing efforts", ""),
		"topics/topics.md":      note("Everything else worth knowing", ""),
		"people/priya/priya.md": fmNote("description: Priya, the escalation lead\naliases: [Pri, Priya K]\n", "\nPrefers email over chat. ^m-ddddd1\n"),
		"projects/launch/launch.md": fmNote("description: The spring launch\nStatus: in review\n",
			"\nThe quarterly report is due\non Friday. ^m-ccccc1\n\nTrack it under #work/urgent today. ^m-ccccc2\n"),
		"topics/outbox-etiquette/outbox-etiquette.md": fmNote(
			"description: How and when to cc people on outbox replies\ntags: [escalation, outbox]\n",
			"\nForward urgent issues to Priya via outbox cc. ^m-aaaaa1\n\nSee [[cc-rules]] before replying. ^m-aaaaa2\n"),
		"topics/cc-rules/cc-rules.md": note("Rules for cc lines", "\nNever cc on routine answers. ^m-bbbbb1\n"),
		// Malformed (no description): skipped, though it holds the word.
		"topics/broken/broken.md": fmNote("tags: [x]\n", "\nzebraword lives only in a skipped node.\n"),
	}
	notes := writeTree(t, files)
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	base := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	for i, rel := range rels {
		m := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(filepath.Join(notes, filepath.FromSlash(rel)), m, m); err != nil {
			t.Fatal(err)
		}
	}
	st := Load(notes)
	wantProblems(t, st, pk{SeverityWarning, "notes.md"}, pk{SeverityError, "topics/broken"})
	return st
}

func matchedStems(t *testing.T, st *Store, query string) []string {
	t.Helper()
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", query, err)
	}
	results := Search(st, q)
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Node.Stem)
	}
	sort.Strings(out)
	return out
}

func TestSearch_Matching(t *testing.T) {
	st := searchFixture(t)
	for _, tc := range []struct {
		query string
		want  []string
	}{
		// Node-level AND: "forward" and "replying" sit in different blocks.
		{`forward replying`, []string{"outbox-etiquette"}},
		// A phrase across the paragraph's line break.
		{`"due on friday"`, []string{"launch"}},
		{`"report is due on"`, []string{"launch"}},
		// Case-insensitive, on stem, description, tags and blocks.
		{`PRIYA`, []string{"outbox-etiquette", "priya"}},
		{`Escalation`, []string{"outbox-etiquette", "priya"}},
		{`lead`, []string{"priya"}},
		// -x excludes on any field: block text, tag, description, stem, path.
		{`priya -email`, []string{"outbox-etiquette"}},
		{`cc -outbox`, []string{"cc-rules"}},
		{`cc -tag:outbox`, []string{"cc-rules"}},
		{`cc -path:topics/outbox`, []string{"cc-rules"}},
		{`cc -file:rules`, []string{"outbox-etiquette"}},
		{`escalation -lead`, []string{"outbox-etiquette"}},
		{`cc -[description:rules]`, []string{"outbox-etiquette"}},
		// tag: matches an inline nested tag by its parent, never a prefix.
		{`tag:work`, []string{"launch"}},
		{`tag:#work`, []string{"launch"}},
		{`tag:WORK/urgent`, []string{"launch"}},
		{`tag:wor`, []string{}},
		{`tag:urgent`, []string{}},
		{`tag:escalation`, []string{"outbox-etiquette"}},
		// path: by location, file: by stem.
		{`path:topics/`, []string{"cc-rules", "outbox-etiquette"}},
		{`path:people/pri`, []string{"priya"}},
		{`file:cc`, []string{"cc-rules"}},
		{`file:topics/`, []string{}},
		// Properties: a list item; a key whose case differs; a phrase.
		{`[aliases:priya]`, []string{"priya"}},
		{`[aliases:k]`, []string{"priya"}},
		{`[status:review]`, []string{"launch"}},
		{`[STATUS]`, []string{"launch"}},
		{`[status:"in review"]`, []string{"launch"}},
		{`[status:done]`, []string{}},
		{`[tags:outbox]`, []string{"outbox-etiquette"}},
		{`[description:spring]`, []string{"launch"}},
		// A link's text, as written in the block.
		{`[[cc-rules]]`, []string{"outbox-etiquette"}},
		// OR.
		{`email OR quarterly`, []string{"launch", "priya"}},
		// Never the root, never a skipped node.
		{`rootonlyword`, []string{}},
		{`zebraword`, []string{}},
		{`[description]`, []string{"cc-rules", "launch", "outbox-etiquette", "people", "priya", "projects", "topics", "user"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			if got := matchedStems(t, st, tc.query); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s matched %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// A raw substring across the newline must not be what matches: the
// phrase matches only because whitespace is collapsed on both sides.
func TestSearch_PhraseNeedsCollapse(t *testing.T) {
	st := searchFixture(t)
	n := mustNode(t, st, "launch")
	if strings.Contains(strings.ToLower(n.Blocks[0].Text), "due on") {
		t.Fatal("fixture block is not wrapped across a line break")
	}
	if got := matchedStems(t, st, `"due   on"`); !reflect.DeepEqual(got, []string{"launch"}) {
		t.Errorf(`"due   on" matched %v, want [launch]`, got)
	}
}

// --- criterion 4 (pure half): hits and ranking ---

func hitIDs(r Result) []string {
	out := make([]string, 0, len(r.Hits))
	for _, b := range r.Hits {
		out = append(out, b.ID)
	}
	return out
}

func TestSearch_Hits(t *testing.T) {
	st := searchFixture(t)
	for _, tc := range []struct {
		query, stem string
		want        []string
	}{
		// Words of the matched alternative make hits, in file order.
		{`cc`, "outbox-etiquette", []string{"m-aaaaa1", "m-aaaaa2"}},
		{`forward replying`, "outbox-etiquette", []string{"m-aaaaa1", "m-aaaaa2"}},
		{`cc-rules`, "outbox-etiquette", []string{"m-aaaaa2"}},
		// A positive tag: term makes a hit of the block holding the tag.
		{`tag:work`, "launch", []string{"m-ccccc2"}},
		// Matched by description, path, file or property only: no hits.
		{`spring`, "launch", []string{}},
		{`path:projects/launch`, "launch", []string{}},
		{`[status:review]`, "launch", []string{}},
		// Only the alternative that matched counts.
		{`friday OR priya`, "launch", []string{"m-ccccc1"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range Search(st, q) {
				if r.Node.Stem != tc.stem {
					continue
				}
				if got := hitIDs(r); !reflect.DeepEqual(got, tc.want) {
					t.Errorf("%s: %s hits %v, want %v", tc.query, tc.stem, got, tc.want)
				}
				return
			}
			t.Fatalf("%s did not match %s", tc.query, tc.stem)
		})
	}
}

// Hits first, then mtime at full precision (newest first), then stem.
func TestSearch_Ranking(t *testing.T) {
	notes := writeTree(t, map[string]string{
		"people/people.md":              note("People", ""),
		"topics/topics.md":              note("Topics", ""),
		"people/zed-tie/zed-tie.md":     note("Tie", "\nkiwi one\n"),
		"topics/alpha-tie/alpha-tie.md": note("Tie", "\nkiwi two\n"),
		"topics/b-new/b-new.md":         note("New", "\nkiwi three\n"),
		"topics/a-old/a-old.md":         note("Old", "\nkiwi four\n"),
		"topics/many/many.md":           note("Many", "\nkiwi five\n\nkiwi six\n"),
	})
	day := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for rel, m := range map[string]time.Time{
		"people/people.md": day, "topics/topics.md": day,
		"people/zed-tie/zed-tie.md":     day.Add(-time.Hour),
		"topics/alpha-tie/alpha-tie.md": day.Add(-time.Hour),
		// Same day, one second apart: the date alone cannot order them.
		"topics/b-new/b-new.md": day.Add(time.Second),
		"topics/a-old/a-old.md": day,
		// Oldest, but two hits.
		"topics/many/many.md": day.Add(-48 * time.Hour),
	} {
		p := filepath.Join(notes, filepath.FromSlash(rel))
		if err := os.Chtimes(p, m, m); err != nil {
			t.Fatal(err)
		}
	}
	st := Load(notes)
	wantProblems(t, st)
	// Store order puts people/ before topics/: zed-tie before alpha-tie.
	if got := stemsOf(st.Nodes()); strings.Index(strings.Join(got, ","), "zed-tie") > strings.Index(strings.Join(got, ","), "alpha-tie") {
		t.Fatalf("fixture: store order %v does not put zed-tie first", got)
	}
	q, err := ParseQuery("kiwi")
	if err != nil {
		t.Fatal(err)
	}
	results := Search(st, q)
	got := make([]string, 0, len(results))
	for _, r := range results {
		got = append(got, r.Node.Stem)
	}
	want := []string{"many", "b-new", "a-old", "alpha-tie", "zed-tie"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ranking = %v, want %v", got, want)
	}
}

func TestInlineTags(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"#work/urgent now", []string{"work/urgent"}},
		{"a #Tag. b", []string{"Tag"}},
		{"a#notatag #ok", []string{"ok"}},
		{"#123 #12a #", []string{"12a"}},
		{"line\n#next-line_tag", []string{"next-line_tag"}},
		{"##double", []string{}},
		{"[[x#^m-abcdef]]", []string{}},
	} {
		got := inlineTags(tc.text)
		if got == nil {
			got = []string{}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("inlineTags(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
