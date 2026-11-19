package main

import (
	"encoding/xml"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// xWrite is write's result as a reader sees it.
type xWrite struct {
	XMLName  xml.Name
	Created  string   `xml:"created,attr"`
	Stem     string   `xml:"stem,attr"`
	Assigned []string `xml:"assigned"`
	Adopted  []string `xml:"adopted"`
	Node     *xNode   `xml:"node"`
}

var blockIDPattern = regexp.MustCompile(`^m-[a-z0-9]{6}$`)

// createArgs is criterion 1's command.
var createArgs = []string{
	"write", "kettle-descaling", "--parent", "topics", "--description", "How to descale the kettle",
	"--tag", "home", "--tag", "cleaning", "--property", "status=active",
	"--add", "Use citric acid.", "--add", "Rinse twice.",
}

func decodeWrite(t *testing.T, r result) xWrite {
	t.Helper()
	mustSucceed(t, r)
	var w xWrite
	decode(t, r.stdout, &w)
	check(t, "result element", w.XMLName.Local, "memory_write")
	if w.Node == nil {
		t.Fatalf("result has no <node>:\n%s", r.stdout)
	}
	if strings.Index(r.stdout, "<assigned>") > strings.Index(r.stdout, "<node") {
		t.Errorf("<assigned> does not come before <node>:\n%s", r.stdout)
	}
	return w
}

func blocksOf(n xNode) []xBlock {
	out := make([]xBlock, 0, len(n.Blocks))
	for _, b := range n.Blocks {
		out = append(out, xBlock{ID: b.ID, Text: b.Text})
	}
	return out
}

// loadNode loads the store with the library and returns stem's node.
func loadNode(t *testing.T, notes, stem string) *memory.Node {
	t.Helper()
	n, ok := memory.Load(notes).Node(stem)
	if !ok {
		t.Fatalf("node %s does not load", stem)
	}
	return n
}

// createKettle runs criterion 1's create and returns its two assigned IDs.
func createKettle(t *testing.T, dir string) (id1, id2 string) {
	t.Helper()
	w := decodeWrite(t, runBin(t, dir, nil, createArgs...))
	if len(w.Assigned) != 2 {
		t.Fatalf("assigned %v, want two IDs", w.Assigned)
	}
	return w.Assigned[0], w.Assigned[1]
}

// Acceptance criterion 1: a create prints D29's result with the assigned
// IDs in argv order and the node as read renders it at depth 0, and the
// node is on disk with its blocks, tags and property.
func TestWriteCreate(t *testing.T) {
	dir, notes := seeded(t)
	w := decodeWrite(t, runBin(t, dir, nil, createArgs...))
	check(t, "created", w.Created, "true")
	check(t, "stem", w.Stem, "kettle-descaling")
	if len(w.Assigned) != 2 || w.Assigned[0] == w.Assigned[1] {
		t.Fatalf("assigned %v, want two distinct IDs", w.Assigned)
	}
	for _, id := range w.Assigned {
		if !blockIDPattern.MatchString(id) {
			t.Errorf("assigned ID %q is not m- plus 6 base-36 characters", id)
		}
	}
	check(t, "adopted", len(w.Adopted), 0)
	check(t, "node attributes", attrsOf(*w.Node), attrs{
		Stem: "kettle-descaling", Path: "topics/kettle-descaling", Parent: "topics",
		Tags: "home,cleaning", Description: "How to descale the kettle",
	})
	check(t, "blocks, IDs in argv order", blocksOf(*w.Node), []xBlock{
		{ID: w.Assigned[0], Text: "Use citric acid."},
		{ID: w.Assigned[1], Text: "Rinse twice."},
	})

	read := readNode(t, dir, "kettle-descaling", "--depth", "0")
	check(t, "write's <node> is read's at depth 0", *w.Node, read)

	read = readNode(t, dir, "kettle-descaling")
	check(t, "read: blocks", blocksOf(read), blocksOf(*w.Node))
	check(t, "read: tags", read.Tags, "home,cleaning")
	status, ok := loadNode(t, notes, "kettle-descaling").Property("status")
	check(t, "status property", []any{status, ok}, []any{"active", true})
}

// Acceptance criterion 2: an update applies --add and --replace in argv
// order, replaces the tags and description, and merges properties.
func TestWriteUpdateInArgvOrder(t *testing.T) {
	dir, notes := seeded(t)
	id1, id2 := createKettle(t, dir)

	w := decodeWrite(t, runBin(t, dir, nil, "write", "kettle-descaling",
		"--add", "New A.", "--replace", id1+"=Use citric acid, 2 tbsp.", "--add", "New B.",
		"--tag", "home", "--description", "Kettle descaling"))
	check(t, "created", w.Created, "false")
	if len(w.Assigned) != 3 {
		t.Fatalf("assigned %v, want three IDs", w.Assigned)
	}
	newA, newB := w.Assigned[0], w.Assigned[2]
	check(t, "assigned[1] is the replaced block's ID", w.Assigned[1], id1)
	for _, id := range []string{newA, newB} {
		if !blockIDPattern.MatchString(id) || id == id1 || id == id2 {
			t.Errorf("assigned %v: %q is not a new ID", w.Assigned, id)
		}
	}
	want := []xBlock{
		{ID: id1, Text: "Use citric acid, 2 tbsp."},
		{ID: id2, Text: "Rinse twice."},
		{ID: newA, Text: "New A."},
		{ID: newB, Text: "New B."},
	}
	check(t, "result blocks", blocksOf(*w.Node), want)
	read := readNode(t, dir, "kettle-descaling")
	check(t, "read: blocks", blocksOf(read), want)
	check(t, "read: tags replaced", read.Tags, "home")
	check(t, "read: description replaced", read.Description, "Kettle descaling")

	n := loadNode(t, notes, "kettle-descaling")
	status, ok := n.Property("status")
	check(t, "status unchanged", []any{status, ok}, []any{"active", true})
	check(t, "properties", len(n.Properties), 1)
}

// A --property on an update is merged in: the new key is added and the
// existing one kept.
func TestWritePropertiesMerge(t *testing.T) {
	dir, notes := seeded(t)
	createKettle(t, dir)
	decodeWrite(t, runBin(t, dir, nil, "write", "kettle-descaling", "--property", "owner=ops", "--property", "count=3"))
	n := loadNode(t, notes, "kettle-descaling")
	for k, want := range map[string]any{"status": "active", "owner": "ops", "count": "3"} {
		got, ok := n.Property(k)
		check(t, "property "+k, []any{got, ok}, []any{want, true})
	}
	check(t, "tags untouched without --tag", n.Tags, []string{"home", "cleaning"})
}

// The result's node is read's rendering at depth 0: a node with children
// lists none, and its backlinks are as of this write.
func TestWriteResultIsDepthZero(t *testing.T) {
	dir, _ := seeded(t)
	createKettle(t, dir)
	decodeWrite(t, runBin(t, dir, nil, "write", "user", "--add", "See [[topics]]."))
	w := decodeWrite(t, runBin(t, dir, nil, "write", "topics", "--add", "A claim about topics."))
	if w.Node.Children != nil {
		t.Errorf("write's <node> lists children: %+v", *w.Node.Children)
	}
	check(t, "backlinks", links(w.Node.Backlinks), []xLink{{Stem: "user", Block: readNode(t, dir, "user").Blocks[0].ID}})
	check(t, "write's <node> is read's at depth 0", *w.Node, readNode(t, dir, "topics", "--depth", "0"))
}

// Acceptance criterion 3: each argument error exits 1 with empty stdout
// and one stderr line naming the argument as typed, and writes nothing.
func TestWriteErrorsNameArguments(t *testing.T) {
	dir, _ := seeded(t)
	id1, _ := createKettle(t, dir)
	state := filepath.Join(dir, ".agent")
	ageTree(t, state)
	before := snapshot(t, state)

	const blank = "First line.\n\nSecond paragraph."
	cases := []struct {
		name  string
		args  []string
		names string
	}{
		{"create without --parent", []string{"new-node", "--description", "D", "--add", "x"}, "--parent"},
		{"unknown parent", []string{"new-node", "--parent", "nope", "--description", "D"}, "--parent"},
		{"invalid stem", []string{"Bad_Stem", "--parent", "topics", "--description", "D"}, "STEM"},
		{"second block has a blank line", []string{"kettle-descaling", "--add", "Fine.", "--add", blank}, "--add #2"},
		{"numbered across --replace", []string{"kettle-descaling", "--replace", id1 + "=Fine.", "--add", blank}, "--add #2"},
		{"numbered on a create", []string{"new-node", "--parent", "topics", "--description", "D", "--add", "Fine.", "--add", "Fine too.", "--add", blank}, "--add #3"},
		{"single-dash flag as typed", []string{"kettle-descaling", "-add", "Fine.", "-add", blank}, "-add #2"},
		{"--replace without =", []string{"kettle-descaling", "--replace", "m-abc123"}, "--replace"},
		{"--replace with an empty ID", []string{"kettle-descaling", "--replace", "=text"}, "--replace"},
		{"--replace unknown ID", []string{"kettle-descaling", "--replace", "m-zzzzzz=New text."}, "--replace m-zzzzzz"},
		{"--replace on a create", []string{"new-node", "--parent", "topics", "--description", "D", "--replace", "m-zzzzzz=x"}, "--replace m-zzzzzz"},
		{"--replace text with a blank line", []string{"kettle-descaling", "--replace", id1 + "=" + blank}, "--replace " + id1},
		{"--property without =", []string{"kettle-descaling", "--property", "status"}, "--property"},
		{"--property with an empty key", []string{"kettle-descaling", "--property", "=v"}, `--property ""`},
		{"--property with a reserved key", []string{"kettle-descaling", "--property", "description=x"}, `--property "description"`},
		{"--property with a multi-line key", []string{"kettle-descaling", "--property", "a\nb=v"}, `--property "a\nb"`},
		{"invalid tag", []string{"kettle-descaling", "--tag", "home", "--tag", "two words"}, "--tag #2"},
		{"empty description", []string{"kettle-descaling", "--description", ""}, "--description"},
		{"no stem", []string{"--add", "x"}, "STEM"},
		{"two stems", []string{"kettle-descaling", "other"}, "STEM"},
		{"unknown flag", []string{"kettle-descaling", "--bogus", "x"}, "--bogus"},
	}
	for _, tc := range cases {
		line := mustFail(t, runBin(t, dir, nil, append([]string{"write"}, tc.args...)...))
		if !strings.HasPrefix(line, tc.names+":") {
			t.Errorf("%s: stderr %q does not name %s", tc.name, line, tc.names)
		}
		for _, internal := range []string{"blocks[", "properties[", "tags["} {
			if strings.Contains(line, internal) {
				t.Errorf("%s: stderr %q names the writer's field %s", tc.name, line, internal)
			}
		}
		for _, internal := range []string{"stem:", "parent:", "description:"} {
			if strings.HasPrefix(line, internal) {
				t.Errorf("%s: stderr %q names the writer's field %s", tc.name, line, internal)
			}
		}
		sameSnapshot(t, before, snapshot(t, state))
	}
}
