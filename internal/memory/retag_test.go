//go:build unix

package memory

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func tagged(tags, body string) string {
	return "---\ndescription: d\ntags: " + tags + "\n---\n" + body
}

func mustRetag(t *testing.T, dir, old, new string) RetagResult {
	t.Helper()
	res, err := writerFor(t, dir).Retag(old, new)
	if err != nil {
		t.Fatalf("retag %q %q: %v", old, new, err)
	}
	return res
}

// retagOne runs one retag on a store holding src as x/x.md, its mtime set
// by ageNotes, and returns the note afterwards.
func retagOne(t *testing.T, src, old, new string) (string, RetagResult) {
	t.Helper()
	dir := writeTree(t, map[string]string{"x/x.md": src})
	ageNotes(t, dir)
	res := mustRetag(t, dir, old, new)
	noTempFiles(t, dir)
	return string(fileBytes(t, dir, "x/x.md")), res
}

// Only the tag token changes in block text: line endings, how the ID is
// written, ID-less paragraphs (not adopted), blank-line runs, trailing
// spaces and a missing final newline all stay. A rewritten note keeps its
// date: having no modified key, it is given its file's time, never the
// time of the call (III.9); a note retag leaves alone gains nothing.
func TestRetag_InlineBytes(t *testing.T) {
	cases := map[string]struct{ src, old, new, want string }{
		"rename, everything else copied": {
			fm + "\n\nFirst #old here.  \nsecond line #old/sub, #older\n^m-aaaaa1  \n\n\nNo ID #old\n\n#old at start ^m-bbbbb1",
			"old", "new",
			fm + "\n\nFirst #new here.  \nsecond line #new/sub, #older\n^m-aaaaa1  \n\n\nNo ID #new\n\n#new at start ^m-bbbbb1",
		},
		"CRLF": {
			"---\r\ndescription: d\r\n---\r\n\r\nA #old\r\nb #old ^m-aaaaa1\r\n", "old", "new",
			"---\r\ndescription: d\r\n---\r\n\r\nA #new\r\nb #new ^m-aaaaa1\r\n",
		},
		"not a tag: glued, in a word, numeric, another tag": {
			fm + "\nx#old a#old/b #olds #old_x #old-x #123 ^m-aaaaa1\n", "old", "new",
			fm + "\nx#old a#old/b #olds #old_x #old-x #123 ^m-aaaaa1\n",
		},
		"inline code is not special": {
			fm + "\nRun ` #old ` now. ^m-aaaaa1\n", "old", "new",
			fm + "\nRun ` #new ` now. ^m-aaaaa1\n",
		},
		"case matters": {
			fm + "\n#Old and #old ^m-aaaaa1\n", "old", "new",
			fm + "\n#Old and #new ^m-aaaaa1\n",
		},
		"remove: mid-line takes the space before": {
			fm + "\nKeep this #old note. ^m-aaaaa1\n", "old", "",
			fm + "\nKeep this note. ^m-aaaaa1\n",
		},
		"remove: before punctuation": {
			fm + "\nKeep #old, then. ^m-aaaaa1\n", "old", "",
			fm + "\nKeep, then. ^m-aaaaa1\n",
		},
		"remove: at the end, before the ID": {
			fm + "\nOwns it. #old/sub ^m-aaaaa1\n", "old", "",
			fm + "\nOwns it. ^m-aaaaa1\n",
		},
		"remove: at the start takes the space after": {
			fm + "\n#old Keep this. ^m-aaaaa1\n", "old", "",
			fm + "\nKeep this. ^m-aaaaa1\n",
		},
		"remove: at the start of a later line": {
			fm + "\nLine one\n#old line two ^m-aaaaa1\n", "old", "",
			fm + "\nLine one\nline two ^m-aaaaa1\n",
		},
		"remove: a line of its own goes with its line break": {
			fm + "\nLine one\r\n#old\r\nline three ^m-aaaaa1\r\n", "old", "",
			fm + "\nLine one\r\nline three ^m-aaaaa1\r\n",
		},
		"remove: the last line, ID on its own line": {
			fm + "\nLine one\n#old\n^m-aaaaa1\n", "old", "",
			fm + "\nLine one\n^m-aaaaa1\n",
		},
		"remove: two in a row": {
			fm + "\nA #old #old/x b ^m-aaaaa1\n", "old", "",
			fm + "\nA b ^m-aaaaa1\n",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, res := retagOne(t, c.src, c.old, c.new)
			want := c.want
			if c.src != c.want {
				want = stamped(c.want, agedText)
			}
			if got != want {
				t.Errorf("x.md\n got %q\nwant %q", got, want)
			}
			if wantNodes := map[bool]int{true: 1, false: 0}[c.src != c.want]; res.Nodes != wantNodes {
				t.Errorf("nodes = %d, want %d", res.Nodes, wantNodes)
			}
		})
	}
}

// Frontmatter tags are matched by whole path segment, nested tags follow
// their parent, and a tag the node ends with twice is kept once, first
// position. Every other property keeps its value.
func TestRetag_Frontmatter(t *testing.T) {
	cases := map[string]struct {
		tags, old, new string
		want           []string
	}{
		"rename":                     {"[a, old, b]", "old", "new", []string{"a", "new", "b"}},
		"nested follows":             {"[old/x/y, old, olds]", "old", "new", []string{"new/x/y", "new", "olds"}},
		"only a nested tag":          {"[a/b/c]", "a/b", "z", []string{"z/c"}},
		"deeper prefix is not it":    {"[a/bc]", "a/b", "z", []string{"a/bc"}},
		"already has new, new first": {"[new, old, b]", "old", "new", []string{"new", "b"}},
		"already has new, old first": {"[old, b, new]", "old", "new", []string{"new", "b"}},
		"two become one":             {"[old/x, a, new/x]", "old", "new", []string{"new/x", "a"}},
		"remove":                     {"[a, old, old/x, olds]", "old", "", []string{"a", "olds"}},
		"remove the last":            {"[old]", "old", "", nil},
		"legacy single string":       {"old", "old", "new", []string{"new"}},
		"written with a #":           {"[\"#old\", b]", "#old", "#new", []string{"new", "b"}},
		"into a nested tag":          {"[old]", "old", "area/new", []string{"area/new"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			src := "---\n# kept\ndescription: \"d\"\ntags: " + c.tags + "\nrank: 1.50\ncode: \"007\"\n---\n\nBody. ^m-aaaaa1\n"
			dir := writeTree(t, map[string]string{"x/x.md": src})
			res := mustRetag(t, dir, c.old, c.new)
			n := mustNode(t, loadClean(t, dir), "x")
			if !reflect.DeepEqual(append([]string(nil), n.Tags...), c.want) {
				t.Errorf("tags %q, want %q", n.Tags, c.want)
			}
			got := string(fileBytes(t, dir, "x/x.md"))
			if changed := got != src; changed != (res.Nodes == 1) {
				t.Errorf("nodes = %d but file changed = %v", res.Nodes, changed)
			}
			for _, keep := range []string{"# kept\n", "description: \"d\"\n", "rank: 1.50\n", "code: \"007\"\n", "\n---\n\nBody. ^m-aaaaa1\n"} {
				if !strings.Contains(got, keep) {
					t.Errorf("%q is gone from:\n%s", keep, got)
				}
			}
		})
	}
}

// Only notes that change are written; the count is of those. Each written
// note, having no date, is given its file's (III.9).
func TestRetag_WritesOnlyChangedNotes(t *testing.T) {
	files := map[string]string{
		"a/a.md":     tagged("[old]", ""),
		"a/b/b.md":   fm + "\nInline #old only. ^m-bbbbb1\n",
		"c/c.md":     tagged("[olds, other]", "\nNothing #olds here. ^m-ccccc1\n"),
		"d/d.md":     tagged("[new]", "\nAlready #new. ^m-ddddd1\n"),
		"c/stray.md": "#old in a file that is not a node",
	}
	dir := writeTree(t, files)
	ageNotes(t, dir)
	res := mustRetag(t, dir, "old", "new")
	if res != (RetagResult{Old: "old", New: "new", Nodes: 2}) {
		t.Errorf("result %+v", res)
	}
	want := map[string]string{".": "dir", "a": "dir", "a/b": "dir", "c": "dir", "d": "dir"}
	for k, v := range files {
		want[k] = v
	}
	want["a/a.md"] = stamped(tagged("[new]", ""), agedText)
	want["a/b/b.md"] = stamped(fm+"\nInline #new only. ^m-bbbbb1\n", agedText)
	wantTree(t, dir, want)

	// A tag nobody has, and NEW equal to OLD, write nothing.
	for _, pair := range [][2]string{{"nobody", "x"}, {"nobody", ""}, {"new", "new"}, {"#new", "new"}} {
		if res := mustRetag(t, dir, pair[0], pair[1]); res.Nodes != 0 {
			t.Errorf("retag %v: nodes = %d", pair, res.Nodes)
		}
	}
	wantTree(t, dir, want)
}

func TestRetag_BadTags(t *testing.T) {
	dir := writeTree(t, map[string]string{"a/a.md": tagged("[old]", "")})
	before := treeState(t, dir)
	w := writerFor(t, dir)
	for _, bad := range []string{"", "bad tag", "123", "#", "a#b"} {
		_, err := w.Retag(bad, "x")
		wantFieldRefusal(t, err, "old")
		if bad == "" {
			continue // an empty NEW means remove
		}
		_, err = w.Retag("old", bad)
		wantFieldRefusal(t, err, "new")
	}
	wantTree(t, dir, before)
}

// A block that would be left with no text fails the whole call, naming the
// block, before anything is written.
func TestRetag_EmptyBlockWritesNothing(t *testing.T) {
	for name, body := range map[string]string{
		"with an ID":       "\nFine #old here. ^m-zzzzz1\n\n#old ^m-zzzzz2\n",
		"ID on its line":   "\n#old/sub\n^m-zzzzz2\n",
		"without an ID":    "\nFine. ^m-zzzzz1\n\n#old\n",
		"several tags":     "\n#old #old/x ^m-zzzzz2\n",
		"becomes a list":   "\n#old - not a list yet ^m-zzzzz2\n",
		"becomes a quote":  "\nText\n#old > quoted ^m-zzzzz2\n",
		"becomes a header": "\n#old # heading ^m-zzzzz2\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"a/a.md": tagged("[old, keep]", "\nAlso #old. ^m-aaaaa1\n"),
				"z/z.md": fm + body,
			})
			before := treeState(t, dir)
			_, err := writerFor(t, dir).Retag("old", "")
			wantRefused(t, err, "z")
			if strings.Contains(body, "m-zzzzz2") && !strings.Contains(err.Error(), "m-zzzzz2") {
				t.Errorf("error %q does not name the block", err)
			}
			if empties := !strings.HasPrefix(name, "becomes"); empties != strings.Contains(err.Error(), "delete the block instead") {
				t.Errorf("error %q: says to delete the block = %v, want %v", err, !empties, empties)
			}
			wantTree(t, dir, before)
		})
	}
}

// A failure after some notes were replaced restores them: the store is as
// it was.
func TestRetag_FailureMidwayRollsBack(t *testing.T) {
	files := map[string]string{
		"a/a.md": tagged("[old]", "\nAlso #old. ^m-aaaaa1\n"),
		"b/b.md": tagged("[old]", ""),
		"c/c.md": tagged("[old]", ""),
	}
	t.Run("a rename fails", func(t *testing.T) {
		dir := writeTree(t, files)
		before := treeState(t, dir)
		renames := 0
		withHook(t, &renameFile, func(from, to string) error {
			if renames++; renames == 2 {
				return errors.New("injected rename failure")
			}
			return os.Rename(from, to)
		})
		_, err := writerFor(t, dir).Retag("old", "new")
		wantRefused(t, err, "injected rename failure")
		if renames < 2 {
			t.Fatalf("only %d renames ran: nothing was written before the failure", renames)
		}
		wantTree(t, dir, before)
	})
	t.Run("the reload shows a new error", func(t *testing.T) {
		dir := writeTree(t, files)
		before := treeState(t, dir)
		var written bool
		withHook(t, &testHookReadBack, func(s *Store, _ string) {
			written = reflect.DeepEqual(mustNode(t, s, "c").Tags, []string{"new"})
			s.problems = append(s.problems, Problem{Severity: SeverityError, Path: "b", Reason: "injected failure"})
		})
		_, err := writerFor(t, dir).Retag("old", "new")
		wantRefused(t, err, "injected failure")
		if !written {
			t.Error("the reload did not see the retag: the roll back was not exercised")
		}
		wantTree(t, dir, before)
	})
	t.Run("a note changed on disk", func(t *testing.T) {
		dir := writeTree(t, files)
		edited := tagged("[old, extra]", "")
		calls := 0
		withHook(t, &testHookBeforeRename, func(string) {
			if calls++; calls == 2 {
				writeFile(t, dir+"/b/b.md", edited)
			}
		})
		_, err := writerFor(t, dir).Retag("old", "new")
		if !errors.Is(err, ErrChangedOnDisk) {
			t.Fatalf("err = %v, want ErrChangedOnDisk", err)
		}
		want := map[string]string{".": "dir", "a": "dir", "b": "dir", "c": "dir"}
		for k, v := range files {
			want[k] = v
		}
		want["b/b.md"] = edited
		wantTree(t, dir, want)
	})
}

// inlineTagSpans is inlineTags with each tag's place in the text.
func TestInlineTagSpans(t *testing.T) {
	text := "#a b #c/d, x#e #123 \t#é_1\n#f"
	spans := inlineTagSpans(text)
	tags, written := make([]string, 0, len(spans)), make([]string, 0, len(spans))
	for _, s := range spans {
		tags = append(tags, s.tag)
		written = append(written, text[s.start:s.end])
	}
	if want := []string{"a", "c/d", "é_1", "f"}; !reflect.DeepEqual(tags, want) || !reflect.DeepEqual(inlineTags(text), want) {
		t.Errorf("tags %q (inlineTags %q), want %q", tags, inlineTags(text), want)
	}
	if want := []string{"#a", "#c/d", "#é_1", "#f"}; !reflect.DeepEqual(written, want) {
		t.Errorf("as written %q, want %q", written, want)
	}
}
