package main

// CLI16: a note stores the date its content last changed, as the modified
// property, and that is the node's date (docs/spec/agent-memory.md Part
// III.9). All through the built binary.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	// dfDate is the date the fixture's notes are given; dfOld is the mtime
	// they are given with it, so the two can be told apart.
	dfDate    = "2026-01-15T10:00:00Z"
	dfDay     = "2026-01-15"
	dfOldDay  = "2020-01-01"
	dfP1      = "Priya leads platform."
	dfP2      = "Atlas cutover is 2026-12-01."
	dfK1      = "Use citric acid."
	writeForm = "2006-01-02T15:04:05Z"
	unreadMsg = "modified: cannot be read as a date or date-time; the file's time is used"
)

var dfOld = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

var modifiedLineRE = regexp.MustCompile(`(?m)^modified:.*\n`)

// dateF is the unit's fixture store F: the shipped starting tree plus
// priya, atlas and kettle, made with write and then dated dfDate.
type dateF struct {
	dir, notes             string
	p1, p2, a1, a2, k1     string
	priya, atlas, kettle   string // folder note paths
	textA1, textA2, textK2 string
}

func newDateF(t *testing.T) *dateF {
	t.Helper()
	dir, notes := seeded(t)
	f := &dateF{dir: dir, notes: notes}
	ids := f.write(t, "priya", "--parent", "people", "--description", "Priya", "--tag", "platform", "--tag", "atlas",
		"--add", dfP1, "--add", dfP2)
	f.p1, f.p2 = ids[0], ids[1]
	f.textA1, f.textA2 = "Owner is [[priya]].", "Date: [[priya#^"+f.p2+"]]."
	ids = f.write(t, "atlas", "--parent", "projects", "--description", "The Atlas project", "--tag", "atlas",
		"--add", f.textA1, "--add", f.textA2)
	f.a1, f.a2 = ids[0], ids[1]
	f.k1 = f.write(t, "kettle", "--parent", "topics", "--description", "Kettle care", "--add", dfK1)[0]
	f.priya = filepath.Join(notes, "people", "priya", "priya.md")
	f.atlas = filepath.Join(notes, "projects", "atlas", "atlas.md")
	f.kettle = filepath.Join(notes, "topics", "kettle", "kettle.md")
	for _, p := range []string{f.priya, f.atlas, f.kettle} {
		dateNote(t, p, dfDate)
	}
	return f
}

// withK2 adds criterion 5's block K2 to kettle, a link to priya's second
// block, and dates kettle again.
func (f *dateF) withK2(t *testing.T) *dateF {
	t.Helper()
	f.textK2 = "Cutover: [[priya#^" + f.p2 + "]]."
	f.write(t, "kettle", "--add", f.textK2)
	dateNote(t, f.kettle, dfDate)
	return f
}

// write runs write in f's store and returns the IDs it assigned.
func (f *dateF) write(t *testing.T, args ...string) []string {
	t.Helper()
	return decodeWrite(t, runBin(t, f.dir, nil, append([]string{"write"}, args...)...)).Assigned
}

func (f *dateF) ok(t *testing.T, args ...string) result {
	t.Helper()
	r := runBin(t, f.dir, nil, args...)
	mustSucceed(t, r)
	return r
}

// timed runs a call that must succeed and returns the seconds it started
// and ended in: <NOW> is a value not before the one and not after the other.
func (f *dateF) timed(t *testing.T, args ...string) (start, end time.Time) {
	t.Helper()
	start = time.Now().UTC().Truncate(time.Second)
	f.ok(t, args...)
	return start, time.Now().UTC().Truncate(time.Second)
}

// frontmatterOf is the text between a note's two --- lines.
func frontmatterOf(t *testing.T, p string) string {
	t.Helper()
	note := readText(t, p)
	rest, ok := strings.CutPrefix(note, "---\n")
	end := strings.Index(rest, "---\n")
	if !ok || end < 0 {
		t.Fatalf("%s has no frontmatter:\n%s", p, note)
	}
	return rest[:end]
}

// setFrontmatter replaces a note's frontmatter by hand, keeping its body.
func setFrontmatter(t *testing.T, p, fm string) {
	t.Helper()
	note := readText(t, p)
	old := frontmatterOf(t, p)
	writeFile(t, p, "---\n"+fm+strings.TrimPrefix(note, "---\n"+old))
}

// dateNote makes a note "dated D": its frontmatter holds modified: D, in
// place of the modified line it has (or last), and its file's mtime is
// 2020-01-01T00:00:00Z.
func dateNote(t *testing.T, p, date string) {
	t.Helper()
	fm, line := frontmatterOf(t, p), "modified: "+date+"\n"
	if modifiedLineRE.MatchString(fm) {
		fm = modifiedLineRE.ReplaceAllLiteralString(fm, line)
	} else {
		fm += line
	}
	setFrontmatter(t, p, fm)
	setMtime(t, p, dfOld)
}

// keyless removes the modified line from a note by hand.
func keyless(t *testing.T, p string) {
	t.Helper()
	fm := frontmatterOf(t, p)
	if !modifiedLineRE.MatchString(fm) {
		t.Fatalf("%s has no modified line to remove:\n%s", p, fm)
	}
	setFrontmatter(t, p, modifiedLineRE.ReplaceAllLiteralString(fm, ""))
}

// modifiedOf is the value of a note's one modified line. No such line, or
// more than one, fails the test.
func modifiedOf(t *testing.T, p string) string {
	t.Helper()
	lines := modifiedLineRE.FindAllString(frontmatterOf(t, p), -1)
	if len(lines) != 1 {
		t.Fatalf("%s holds %d modified lines, want 1:\n%s", p, len(lines), readText(t, p))
	}
	value, ok := strings.CutPrefix(strings.TrimSuffix(lines[0], "\n"), "modified: ")
	if !ok {
		t.Fatalf("%s: modified line %q is not \"modified: <value>\"", p, lines[0])
	}
	return value
}

// wantNow fails unless value has the form YYYY-MM-DDTHH:MM:SSZ and is not
// before start nor after end.
func wantNow(t *testing.T, what, value string, start, end time.Time) {
	t.Helper()
	got, err := time.Parse(writeForm, value)
	if err != nil || got.Format(writeForm) != value {
		t.Errorf("%s: modified is %q, want the form YYYY-MM-DDTHH:MM:SSZ", what, value)
		return
	}
	if got.Before(start) || got.After(end) {
		t.Errorf("%s: modified is %s, want the time of the call, between %s and %s", what, value, start.Format(writeForm), end.Format(writeForm))
	}
}

// wantStamped checks a note's frontmatter is before's with the modified
// line now holding <NOW> where it stood (or added last when before had
// none), and nothing else changed.
func wantStamped(t *testing.T, what, p, before string, start, end time.Time) {
	t.Helper()
	value := modifiedOf(t, p)
	wantNow(t, what, value, start, end)
	line := "modified: " + value + "\n"
	want := before + line
	if modifiedLineRE.MatchString(before) {
		want = modifiedLineRE.ReplaceAllLiteralString(before, line)
	}
	if got := frontmatterOf(t, p); got != want {
		t.Errorf("%s: frontmatter\n got %q\nwant %q", what, got, want)
	}
}

// shownDates is the modified attribute a node carries in read, in search
// and in the index.
func shownDates(t *testing.T, dir string, env map[string]string, stem, query string) (read, search, index string) {
	t.Helper()
	r := runBin(t, dir, env, "read", stem)
	mustSucceed(t, r)
	var n xNode
	decode(t, r.stdout, &n)
	r = runBin(t, dir, env, "search", query)
	mustSucceed(t, r)
	var s xSearch
	decode(t, r.stdout, &s)
	for _, hit := range s.Nodes {
		if hit.Stem == stem {
			search = hit.Modified
		}
	}
	r = runBin(t, dir, env, "index")
	mustSucceed(t, r)
	var ix xRoot
	decode(t, r.stdout, &ix)
	for _, first := range ix.Nodes {
		for _, e := range append([]xEntry{first}, first.Nodes...) {
			if e.Stem == stem {
				index = e.Modified
			}
		}
	}
	return n.Modified, search, index
}

func (f *dateF) readDay(t *testing.T, stem string) string {
	t.Helper()
	return readNode(t, f.dir, stem).Modified
}

// Acceptance criterion 1: a new node carries the date, in UTC whatever the
// zone, and the date read, search and the index show comes from it, not
// from the file.
func TestNewNodeCarriesTheDate(t *testing.T) {
	t.Parallel()
	dir, notes := seeded(t)
	start := time.Now().UTC().Truncate(time.Second)
	r := runBin(t, dir, nil, "write", "descaling", "--parent", "topics", "--description", "How to descale", "--add", "Rinse twice.")
	end := time.Now().UTC().Truncate(time.Second)
	id := decodeWrite(t, r).Assigned[0]
	note := filepath.Join(notes, "topics", "descaling", "descaling.md")
	value := modifiedOf(t, note)
	wantNow(t, "descaling", value, start, end)
	check(t, "the new note", readText(t, note), "---\ndescription: How to descale\nmodified: "+value+"\n---\n\nRinse twice. ^"+id+"\n")

	setMtime(t, note, dfOld)
	read, search, index := shownDates(t, dir, nil, "descaling", "rinse")
	for what, got := range map[string]string{"read": read, "search": search, "index": index} {
		if got != value[:10] {
			t.Errorf("%s shows modified=%q, want %s (the stored date; the file says %s)", what, got, value[:10], dfOldDay)
		}
	}

	// To search it is a property like any other, matched as written.
	s, _ := runSearch(t, dir, "[modified:"+value[:7]+"]")
	if stems := stemsOf(s); len(stems) != 1 || stems[0] != "descaling" {
		t.Errorf("search [modified:%s] found %v, want descaling alone", value[:7], stems)
	}

	// The stored value is UTC even where the process's zone is not.
	start = time.Now().UTC().Truncate(time.Second)
	mustSucceed(t, runBin(t, dir, map[string]string{"TZ": "Asia/Tokyo"}, "write", "tokyo-note", "--parent", "topics", "--description", "d"))
	end = time.Now().UTC().Truncate(time.Second)
	note = filepath.Join(notes, "topics", "tokyo-note", "tokyo-note.md")
	wantNow(t, "written under TZ=Asia/Tokyo", modifiedOf(t, note), start, end)
	check(t, "the note written under TZ=Asia/Tokyo", readText(t, note), "---\ndescription: d\nmodified: "+modifiedOf(t, note)+"\n---\n")
}

// Acceptance criterion 2: a content change rewrites only the modified
// line, where it stands or as the last line, also in a note whose
// frontmatter cannot be re-emitted; that note's frontmatter change is
// still refused.
func TestContentChangeRewritesOnlyThatLine(t *testing.T) {
	t.Parallel()
	f := newDateF(t)

	byHand := "description: \"Priya, who leads platform\"   # keep\ntags: [platform,   atlas]\nmodified: \"2026-01-15 10:00\"\nowner: \"007\"\n"
	setFrontmatter(t, f.priya, byHand)
	start, end := f.timed(t, "write", "priya", "--add", "Prefers async.")
	wantStamped(t, "priya", f.priya, byHand, start, end)
	lines := strings.Split(frontmatterOf(t, f.priya), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[2], "modified: ") {
		t.Errorf("priya: modified is not the third of four lines: %q", lines)
	}

	keyless(t, f.kettle)
	before := frontmatterOf(t, f.kettle)
	start, end = f.timed(t, "write", "kettle", "--add", "Descale monthly.")
	wantStamped(t, "kettle", f.kettle, before, start, end)
	if got := frontmatterOf(t, f.kettle); !strings.HasSuffix(got, "modified: "+modifiedOf(t, f.kettle)+"\n") {
		t.Errorf("kettle: modified is not the last frontmatter line:\n%s", got)
	}

	// A literal block holding a raw U+2028: D27 cannot re-emit it, so the
	// frontmatter is edit-locked. The body is not.
	locked := "description: The Atlas project\ntags: [atlas]\nnote: |+\n  " + string(rune(0x2028)) + "\nmodified: " + dfDate + "\nowner: x\n"
	setFrontmatter(t, f.atlas, locked)
	setMtime(t, f.atlas, dfOld)
	start, end = f.timed(t, "write", "atlas", "--add", "Go.")
	wantStamped(t, "atlas", f.atlas, locked, start, end)
	if blocks := readNode(t, f.dir, "atlas").Blocks; len(blocks) != 3 || blocks[2].Text != "Go." {
		t.Errorf("atlas: blocks %+v, want the added block last", blocks)
	}
	note := readText(t, f.atlas)
	r := runBin(t, f.dir, nil, "write", "atlas", "--tag", "x")
	mustFail(t, r)
	check(t, "atlas after the refused --tag", readText(t, f.atlas), note)
}

// Acceptance criterion 3: a call that changes nothing leaves the note's
// bytes, mtime and date exactly as they were.
func TestCallThatChangesNothingLeavesTheDate(t *testing.T) {
	t.Parallel()
	f := newDateF(t)
	before := snapshot(t, f.notes)
	for _, args := range [][]string{
		{"write", "priya", "--description", "Priya"},
		{"write", "priya", "--tag", "platform", "--tag", "atlas"},
		{"write", "priya", "--replace", f.p1 + "=" + dfP1},
	} {
		r := f.ok(t, args...)
		if w := decodeWrite(t, r); w.Node.Modified != dfDay {
			t.Errorf("%v: the result shows modified=%q, want %s", args, w.Node.Modified, dfDay)
		}
		sameSnapshot(t, before, snapshot(t, f.notes))
	}
	check(t, "priya's modified", modifiedOf(t, f.priya), dfDate)
}

// Acceptance criterion 4: a mechanical rewrite keeps the node's date. A
// dated note keeps its line; a keyless one gains its file's prior mtime,
// cut to the second, as the last frontmatter line; the renamed node is
// left alone; a retag re-emits the frontmatter and still keeps the date.
func TestMechanicalRewriteKeepsTheDate(t *testing.T) {
	t.Parallel()
	f := newDateF(t)
	keyless(t, f.kettle)
	f.write(t, "kettle", "--add", "See [[priya]].")
	keyless(t, f.kettle)
	setMtime(t, f.kettle, time.Date(2026, 3, 2, 8, 30, 45, 500_000_000, time.UTC))
	kettleBefore := frontmatterOf(t, f.kettle)
	priyaBefore := readText(t, f.priya)

	check(t, "rename", f.ok(t, "rename", "priya", "priya-lead").stdout,
		`<memory_rename from="priya" stem="priya-lead" links_rewritten="3"></memory_rename>`+"\n")
	priyaLead := filepath.Join(f.notes, "people", "priya-lead", "priya-lead.md")

	if !strings.Contains(readText(t, f.atlas), "[[priya-lead#^"+f.p2+"]]") {
		t.Fatalf("fixture: atlas was not rewritten:\n%s", readText(t, f.atlas))
	}
	check(t, "atlas's modified after the rename", modifiedOf(t, f.atlas), dfDate)
	check(t, "read atlas", f.readDay(t, "atlas"), dfDay)

	check(t, "kettle's frontmatter", frontmatterOf(t, f.kettle), kettleBefore+"modified: 2026-03-02T08:30:45Z\n")
	check(t, "read kettle", f.readDay(t, "kettle"), "2026-03-02")

	check(t, "the renamed node's note", readText(t, priyaLead), priyaBefore)
	check(t, "read priya-lead", f.readDay(t, "priya-lead"), dfDay)

	check(t, "retag", f.ok(t, "retag", "atlas", "atlas-2026").stdout,
		`<memory_retag from="atlas" tag="atlas-2026" nodes="2"></memory_retag>`+"\n")
	for stem, p := range map[string]string{"priya-lead": priyaLead, "atlas": f.atlas} {
		if !strings.Contains(frontmatterOf(t, p), "atlas-2026") {
			t.Fatalf("fixture: %s was not retagged:\n%s", stem, readText(t, p))
		}
		check(t, stem+"'s modified after the retag", modifiedOf(t, p), dfDate)
		check(t, "read "+stem+" after the retag", f.readDay(t, stem), dfDay)
	}
}

// Acceptance criterion 5: each restructuring verb dates the notes whose
// content it changes, and no others.
func TestRestructuringDatesTheNotesItChanges(t *testing.T) {
	t.Parallel()
	t.Run("move-block", func(t *testing.T) {
		t.Parallel()
		f := newDateF(t).withK2(t)
		start, end := f.timed(t, "move-block", f.p2, "--to", "kettle")
		wantNow(t, "priya", modifiedOf(t, f.priya), start, end)
		wantNow(t, "kettle", modifiedOf(t, f.kettle), start, end)
		if !strings.Contains(readText(t, f.atlas), "[[kettle#^"+f.p2+"]]") {
			t.Fatalf("fixture: atlas's link was not rewritten:\n%s", readText(t, f.atlas))
		}
		check(t, "atlas's modified (a link rewritten)", modifiedOf(t, f.atlas), dfDate)
	})
	t.Run("split", func(t *testing.T) {
		t.Parallel()
		f := newDateF(t).withK2(t)
		start, end := f.timed(t, "split", "priya", "--into", "priya-atlas", "--parent", "priya",
			"--description", "Atlas facts", "--block", f.p2)
		wantNow(t, "priya", modifiedOf(t, f.priya), start, end)
		newNote := filepath.Join(f.notes, "people", "priya", "priya-atlas", "priya-atlas.md")
		wantNow(t, "priya-atlas", modifiedOf(t, newNote), start, end)
		check(t, "priya-atlas's frontmatter", frontmatterOf(t, newNote), "description: Atlas facts\nmodified: "+modifiedOf(t, newNote)+"\n")
		for stem, p := range map[string]string{"atlas": f.atlas, "kettle": f.kettle} {
			if !strings.Contains(readText(t, p), "[[priya-atlas#^"+f.p2+"]]") {
				t.Fatalf("fixture: %s's link was not rewritten:\n%s", stem, readText(t, p))
			}
			check(t, stem+"'s modified (a link rewritten)", modifiedOf(t, p), dfDate)
		}
	})
	t.Run("delete --block", func(t *testing.T) {
		t.Parallel()
		f := newDateF(t).withK2(t)
		before := snapshot(t, f.notes)
		start, end := f.timed(t, "delete", "priya", "--block", f.p1)
		wantNow(t, "priya", modifiedOf(t, f.priya), start, end)
		after := snapshot(t, f.notes)
		for p, b := range before {
			if p != f.priya && filepath.Ext(p) == ".md" && !untouched(b, after[p]) {
				t.Errorf("%s changed", p)
			}
		}
	})
	t.Run("merge", func(t *testing.T) {
		t.Parallel()
		f := newDateF(t).withK2(t)
		start, end := f.timed(t, "merge", "kettle", "--into", "atlas")
		wantNow(t, "atlas", modifiedOf(t, f.atlas), start, end)
		check(t, "priya's modified", modifiedOf(t, f.priya), dfDate)

		// A node with no blocks and no tags gives the kept node nothing.
		f.write(t, "empty", "--parent", "topics", "--description", "Nothing here")
		dateNote(t, f.atlas, dfDate)
		note := readText(t, f.atlas)
		check(t, "merge of an empty node", f.ok(t, "merge", "empty", "--into", "atlas").stdout,
			`<memory_merge from="empty" stem="atlas" blocks="0" children="0" tags_added="0" links_rewritten="0"></memory_merge>`+"\n")
		check(t, "empty is gone", exists(filepath.Join(f.notes, "topics", "empty")), false)
		check(t, "atlas's modified after merging an empty node", modifiedOf(t, f.atlas), dfDate)
		check(t, "atlas's note after merging an empty node", readText(t, f.atlas), note)
	})
	t.Run("move", func(t *testing.T) {
		t.Parallel()
		f := newDateF(t).withK2(t)
		contents := func() map[string]string {
			out := map[string]string{}
			for p, b := range bytesOf(t, f.notes) {
				if filepath.Ext(p) == ".md" {
					out[filepath.Base(p)] = b
				}
			}
			return out
		}
		before := contents()
		f.ok(t, "move", "kettle", "--parent", "people")
		check(t, "kettle moved", exists(filepath.Join(f.notes, "people", "kettle", "kettle.md")), true)
		check(t, "every note's bytes after the move", contents(), before)
	})
}

// Acceptance criterion 6: search's order and the index's dropping order
// follow the stored date, not the file's time.
func TestOrderFollowsTheStoredDate(t *testing.T) {
	t.Parallel()
	dir, notes := seeded(t)
	now := time.Now()
	ids := 0
	node := func(stem, date, word, desc string, mtime time.Time) {
		ids++
		p := writeNode(t, notes, "topics/"+stem, fmt.Sprintf("---\ndescription: %s\nmodified: %s\n---\n\nA %s here. ^m-zz%04d\n", desc, date, word, ids))
		setMtime(t, p, mtime)
	}
	// Long descriptions put the index over its 25,000 bytes by one entry.
	long := func(c string) string { return strings.Repeat(c, 2500) }
	node("old-note", "2026-01-15T10:00:00Z", "zebra", long("o"), now)
	node("new-note", "2026-06-01T10:00:00Z", "zebra", long("n"), dfOld)
	// The same pair with stems in the other order, so stem order cannot
	// stand in for the date.
	node("a-old", "2026-02-15T10:00:00Z", "yak", "A", now)
	node("b-new", "2026-07-01T10:00:00Z", "yak", "B", dfOld)

	s, _ := runSearch(t, dir, "zebra")
	check(t, "search zebra", stemsOf(s), []string{"new-note", "old-note"})
	s, _ = runSearch(t, dir, "yak")
	check(t, "search yak", stemsOf(s), []string{"b-new", "a-old"})

	// Eight more second-level entries, newer than both by either measure.
	for i := range 8 {
		node(fmt.Sprintf("filler-%d", i), "2026-10-01T00:00:00Z", "filler", long(string(rune('a'+i))), now.Add(time.Hour))
	}
	for _, stem := range []string{"a-old", "b-new"} {
		if err := os.RemoveAll(filepath.Join(notes, "topics", stem)); err != nil {
			t.Fatal(err)
		}
	}
	r := runBin(t, dir, nil, "index")
	mustSucceed(t, r)
	if n := len(strings.TrimSuffix(r.stdout, "\n")); n > indexMaxBytes {
		t.Fatalf("the index is %d bytes, over its budget", n)
	}
	if !strings.Contains(r.stdout, `<omitted nodes="1">`) {
		t.Fatalf("fixture: the index did not drop exactly one entry:\n%.400s", r.stdout)
	}
	check(t, "old-note (the older stored date) is shown", strings.Contains(r.stdout, `stem="old-note"`), false)
	check(t, "new-note (the older file) is shown", strings.Contains(r.stdout, `stem="new-note"`), true)
}

// lintReport runs plain lint, which must exit 0, and returns its report.
func lintReport(t *testing.T, dir string) xLint {
	t.Helper()
	r := runBin(t, dir, nil, "lint")
	mustSucceed(t, r)
	var x xLint
	decode(t, r.stdout, &x)
	return x
}

// Acceptance criterion 7: values an operator or a plugin wrote are read,
// an offset-less one in the local zone; anything else falls back to the
// file's time with one lint warning, and the next write or mechanical
// rewrite replaces the value.
func TestStoredValuesAreReadOrFallBack(t *testing.T) {
	t.Parallel()
	store := func(t *testing.T, value string) (dir, note string) {
		t.Helper()
		dir, notes := seeded(t)
		note = writeNode(t, notes, "topics/v", "---\ndescription: d\nmodified:"+value+"\nowner: x\n---\n\nText. ^m-vvvvv1\n")
		setMtime(t, note, dfOld)
		return dir, note
	}
	readable := []struct{ value, tz, day string }{
		{" 2026-11-21T23:30:00Z", "Asia/Tokyo", "2026-11-22"},
		{" 2026-11-21T23:30:00-05:00", "UTC", "2026-11-22"},
		{" 2026-11-21T14:03:00", "Asia/Tokyo", "2026-11-21"},
		{` "2026-11-21 14:03"`, "UTC", "2026-11-21"},
		{" 2026-11-21", "UTC", "2026-11-21"},
		// Read in the local zone, never as UTC: taken as UTC, 23:30 would
		// be the next day in Tokyo, and 00:30 or the start of the day the
		// day before in New York.
		{" 2026-11-21T23:30:00", "Asia/Tokyo", "2026-11-21"},
		{" 2026-11-21 00:30", "America/New_York", "2026-11-21"},
		{" 2026-11-21", "America/New_York", "2026-11-21"},
	}
	for _, tc := range readable {
		t.Run("readable"+tc.value+" "+tc.tz, func(t *testing.T) {
			t.Parallel()
			dir, _ := store(t, tc.value)
			env := map[string]string{"TZ": tc.tz}
			read, search, index := shownDates(t, dir, env, "v", "text")
			check(t, "read, search, index", []string{read, search, index}, []string{tc.day, tc.day, tc.day})
			r := runBin(t, dir, env, "lint")
			mustSucceed(t, r)
			check(t, "lint", r.stdout, `<memory_lint nodes="5" errors="0" warnings="0"></memory_lint>`+"\n")
		})
	}
	unreadable := map[string]string{
		"text":         " banana",
		"a number":     " 12",
		"a flow list":  " [a, b]",
		"no value":     "",
		"a block list": "\n  - a\n  - b",
	}
	for name, value := range unreadable {
		t.Run("unreadable, "+name, func(t *testing.T) {
			t.Parallel()
			dir, note := store(t, value)
			read, search, index := shownDates(t, dir, nil, "v", "text")
			check(t, "read, search, index", []string{read, search, index}, []string{dfOldDay, dfOldDay, dfOldDay})
			x := lintReport(t, dir)
			check(t, "lint counts", []string{x.Nodes, x.Errors, x.Warnings}, []string{"5", "0", "1"})
			check(t, "lint problems", x.Problems, []xProblem{{Severity: "warning", Path: "topics/v", Reason: unreadMsg}})

			start := time.Now().UTC().Truncate(time.Second)
			mustSucceed(t, runBin(t, dir, nil, "write", "v", "--add", "More."))
			end := time.Now().UTC().Truncate(time.Second)
			value := modifiedOf(t, note)
			wantNow(t, "v", value, start, end)
			check(t, "frontmatter after the write", frontmatterOf(t, note), "description: d\nmodified: "+value+"\nowner: x\n")
			check(t, "lint after the write", lintReport(t, dir).Warnings, "0")
		})
		t.Run("unreadable, "+name+", then a rename rewrites a link", func(t *testing.T) {
			t.Parallel()
			dir, notes := seeded(t)
			note := writeNode(t, notes, "topics/w", "---\ndescription: d\nmodified:"+value+"\nowner: x\n---\n\nSee [[target]]. ^m-wwwww1\n")
			setMtime(t, note, time.Date(2021, 5, 6, 7, 8, 9, 999_000_000, time.UTC))
			writeNode(t, notes, "topics/target", "---\ndescription: t\nmodified: "+dfDate+"\n---\n")
			check(t, "lint warnings before", lintReport(t, dir).Warnings, "1")
			mustSucceed(t, runBin(t, dir, nil, "rename", "target", "target-2"))
			check(t, "the note after the rename", readText(t, note),
				"---\ndescription: d\nmodified: 2021-05-06T07:08:09Z\nowner: x\n---\n\nSee [[target-2]]. ^m-wwwww1\n")
			check(t, "read w", readNode(t, dir, "w").Modified, "2021-05-06")
			check(t, "lint after the rename", lintReport(t, dir).Warnings, "0")
		})
	}
}

// Acceptance criterion 8: only memory sets modified, by exact name, and a
// call that fails and rolls back leaves every modified line as it was.
func TestOnlyMemorySetsModified(t *testing.T) {
	t.Parallel()
	f := newDateF(t)
	state := filepath.Join(f.dir, ".agent")
	before := snapshot(t, state)
	for _, args := range [][]string{
		{"write", "priya", "--property", "modified=2020-01-01"},
		{"write", "newnode", "--parent", "topics", "--description", "d", "--property", "modified=x"},
		{"write", "priya", "--property", "owner=x", "--property=modified=x", "--add", "Text."},
	} {
		line := mustFail(t, runBin(t, f.dir, nil, args...))
		mustContain(t, strings.Join(args, " "), line, `--property: "modified" is set by memory`)
		sameSnapshot(t, before, snapshot(t, state))
	}

	start, end := f.timed(t, "write", "priya", "--property", "Modified=x")
	fm := frontmatterOf(t, f.priya)
	mustContain(t, "priya's frontmatter", fm, "Modified: x\n")
	wantNow(t, "priya", modifiedOf(t, f.priya), start, end)

	// A rename that fails part-way: atlas is replaced, then kettle's note
	// cannot be, and atlas is restored.
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	f.write(t, "kettle", "--add", "See [[priya]].")
	keyless(t, f.kettle)
	keyless(t, f.atlas)
	kettleDir := filepath.Dir(f.kettle)
	readOnly(t, kettleDir)
	notes := bytesOf(t, f.notes)
	r := runBin(t, f.dir, nil, "rename", "priya", "priya-lead")
	if r.code == 0 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q; want a failure with nothing on stdout", r.code, r.stdout)
	}
	sameBytes(t, notes, bytesOf(t, f.notes))
	for stem, p := range map[string]string{"atlas": f.atlas, "kettle": f.kettle} {
		if strings.Contains(readText(t, p), "modified") {
			t.Errorf("%s was left with a modified line after the roll back:\n%s", stem, readText(t, p))
		}
	}
	check(t, "the old stem still reads", readNode(t, f.dir, "priya").Path, "people/priya")
}
