package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// xSearch is search's result as a reader sees it.
type xSearch struct {
	XMLName xml.Name
	Query   string    `xml:"query,attr"`
	Matched string    `xml:"matched,attr"`
	Shown   string    `xml:"shown,attr"`
	Nodes   []xHit    `xml:"node"`
	Omitted *xOmitted `xml:"omitted"`
}

type xHit struct {
	Stem        string   `xml:"stem,attr"`
	Path        string   `xml:"path,attr"`
	Tags        string   `xml:"tags,attr"`
	Modified    string   `xml:"modified,attr"`
	Hits        string   `xml:"hits,attr"`
	Description string   `xml:"description"`
	Blocks      []xBlock `xml:"block"`
}

type xOmitted struct {
	Nodes string `xml:"nodes,attr"`
	Text  string `xml:",chardata"`
}

func runSearch(t *testing.T, dir string, args ...string) (xSearch, string) {
	t.Helper()
	r := runBin(t, dir, nil, append([]string{"search"}, args...)...)
	mustSucceed(t, r)
	var s xSearch
	decode(t, r.stdout, &s)
	check(t, "result element", s.XMLName.Local, "memory_search")
	return s, r.stdout
}

// libraryOrder is the library's ranking of query's matches, as stems.
func libraryOrder(t *testing.T, notes, query string) []string {
	t.Helper()
	q, err := memory.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	results := memory.Search(memory.Load(notes), q)
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Node.Stem)
	}
	return out
}

func stemsOf(s xSearch) []string {
	out := make([]string, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		out = append(out, n.Stem)
	}
	return out
}

func setMtime(t *testing.T, p string, mt time.Time) {
	t.Helper()
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

// kettleStore holds 25 nodes matching kettle, node i with i%8 hit blocks
// interleaved with filler, mtimes shuffled so ranking needs all its keys,
// and five nodes that do not match. It returns each matching node's hit
// block IDs in file order.
func kettleStore(t *testing.T) (dir, notes string, hits map[string][]string) {
	t.Helper()
	dir, notes = seeded(t)
	hits = map[string][]string{}
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for i := range 25 {
		stem := fmt.Sprintf("k-%02d", i)
		body := make([]string, 0, 2*(i%8))
		for j := range i % 8 {
			id := fmt.Sprintf("m-%02dhit%d", i, j)
			body = append(body, fmt.Sprintf("Descale the kettle, step %d. ^%s", j, id), fmt.Sprintf("Unrelated filler %d. ^m-%02dfil%d", j, i, j))
			hits[stem] = append(hits[stem], id)
		}
		tags := ""
		if i%3 == 0 {
			tags = "tags: [home, appliances]\n"
		}
		content := fmt.Sprintf("---\ndescription: Node %02d about kettles\n%s---\n\n%s\n", i, tags, strings.Join(body, "\n\n"))
		p := writeNode(t, notes, "topics/"+stem, content)
		setMtime(t, p, base.Add(time.Duration((i*7)%25)*time.Hour))
	}
	for i := range 5 {
		writeNode(t, notes, fmt.Sprintf("people/other-%d", i), "---\ndescription: Nothing relevant\n---\n\nAbout tea. ^m-0th3r"+strconv.Itoa(i)+"\n")
	}
	return dir, notes, hits
}

// Acceptance criterion 5: 25 matches show the library's top 20, each with
// at most 5 hit blocks in file order and its true hit count, and an
// omitted notice for the other 5.
func TestSearchResults(t *testing.T) {
	dir, notes, hits := kettleStore(t)
	s, _ := runSearch(t, dir, "kettle")
	check(t, "query", s.Query, "kettle")
	check(t, "matched", s.Matched, "25")
	check(t, "shown", s.Shown, "20")
	order := libraryOrder(t, notes, "kettle")
	if len(order) != 25 {
		t.Fatalf("fixture: library matches %d nodes, want 25", len(order))
	}
	check(t, "nodes in the library's rank order", stemsOf(s), order[:20])
	if s.Nodes[0].Hits != "7" {
		t.Errorf("first node has hits=%q; the most hits (7) rank first", s.Nodes[0].Hits)
	}
	for _, n := range s.Nodes {
		want := hits[n.Stem]
		check(t, n.Stem+" hits", n.Hits, strconv.Itoa(len(want)))
		got := make([]string, 0, len(n.Blocks))
		for _, b := range n.Blocks {
			got = append(got, b.ID)
			if !strings.Contains(b.Text, "kettle") {
				t.Errorf("%s: block %s %q is not a hit", n.Stem, b.ID, b.Text)
			}
		}
		check(t, n.Stem+" shown blocks (first 5 hits, file order)", got, want[:min(5, len(want))])
		check(t, n.Stem+" path", n.Path, "topics/"+n.Stem)
		check(t, n.Stem+" modified", len(n.Modified), len("2006-01-02"))
		i, _ := strconv.Atoi(strings.TrimPrefix(n.Stem, "k-"))
		check(t, n.Stem+" description", n.Description, fmt.Sprintf("Node %02d about kettles", i))
		wantTags := ""
		if i%3 == 0 {
			wantTags = "home,appliances"
		}
		check(t, n.Stem+" tags", n.Tags, wantTags)
	}
	if s.Omitted == nil {
		t.Fatal("no <omitted> notice")
	}
	check(t, "omitted", *s.Omitted, xOmitted{Nodes: "5", Text: "5 more nodes matched: narrow the query."})
}

// No match: matched="0" shown="0", no children, exit 0.
func TestSearchNoMatch(t *testing.T) {
	dir, _, _ := kettleStore(t)
	s, out := runSearch(t, dir, "teapot")
	check(t, "matched, shown", []string{s.Matched, s.Shown}, []string{"0", "0"})
	if len(s.Nodes) != 0 || s.Omitted != nil {
		t.Errorf("a search matching nothing has children:\n%s", out)
	}
}

// One node omitted is "1 more node", and a query beginning with an
// exclusion is a query, not a flag. The query attribute echoes the
// argument untrimmed.
func TestSearchQueryForms(t *testing.T) {
	dir, notes, _ := kettleStore(t)
	s, _ := runSearch(t, dir, "-filler kettle")
	check(t, "a leading exclusion", s.Matched, strconv.Itoa(len(libraryOrder(t, notes, "-filler kettle"))))
	check(t, "query echo", s.Query, "-filler kettle")

	s, _ = runSearch(t, dir, "  kettle ")
	check(t, "untrimmed query echo", s.Query, "  kettle ")
	check(t, "untrimmed query matches", s.Matched, "25")

	s, _ = runSearch(t, dir, "--", "-filler kettle")
	check(t, "after --", s.Query, "-filler kettle")

	for i := range 20 {
		writeNode(t, notes, fmt.Sprintf("user/one-%02d", i), "---\ndescription: Unique zebra note\n---\n")
	}
	writeNode(t, notes, "user/one-xx", "---\ndescription: Unique zebra note\n---\n")
	s, _ = runSearch(t, dir, "zebra")
	if s.Omitted == nil {
		t.Fatal("no <omitted> notice")
	}
	check(t, "one omitted", *s.Omitted, xOmitted{Nodes: "1", Text: "1 more node matched: narrow the query."})
}

// budget is D31's byte budget, stated independently of production.
const budget = 30000

// element is the rendered <memory_search> element: stdout without its
// final newline.
func element(out string) string { return strings.TrimSuffix(out, "\n") }

// Acceptance criterion 6, budget: over 30,000 bytes, the lowest-ranked
// nodes are dropped, one at a time, until the element fits, and counted in
// <omitted>; no more are dropped than needed.
func TestSearchByteBudget(t *testing.T) {
	dir, notes := seeded(t)
	filler := strings.Repeat("a", 1500)
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for i := range 25 {
		body := make([]string, 0, 5)
		for j := range 5 {
			body = append(body, fmt.Sprintf("kettle %s ^m-b%02db%02d", filler, i, j))
		}
		p := writeNode(t, notes, fmt.Sprintf("topics/big-%02d", i), "---\ndescription: Big\n---\n\n"+strings.Join(body, "\n\n")+"\n")
		setMtime(t, p, base.Add(time.Duration(i)*time.Minute))
	}
	s, out := runSearch(t, dir, "kettle")
	el := element(out)
	if len(el) > budget {
		t.Fatalf("the element is %d bytes, over the %d budget", len(el), budget)
	}
	shown := len(s.Nodes)
	if shown == 0 || shown >= 20 {
		t.Fatalf("shown %d nodes; the budget should drop some but not all", shown)
	}
	check(t, "matched", s.Matched, "25")
	check(t, "shown", s.Shown, strconv.Itoa(shown))
	check(t, "the top-ranked nodes are kept", stemsOf(s), libraryOrder(t, notes, "kettle")[:shown])
	for _, n := range s.Nodes {
		check(t, n.Stem+" blocks kept", len(n.Blocks), 5)
	}
	if s.Omitted == nil {
		t.Fatal("no <omitted> notice")
	}
	check(t, "omitted count", s.Omitted.Nodes, strconv.Itoa(25-shown))
	check(t, "omitted text", s.Omitted.Text, fmt.Sprintf("%d more nodes matched: narrow the query.", 25-shown))

	// Every node renders to the same size, so one more would not fit.
	first := strings.Index(el, "\n  <node")
	second := strings.Index(el[first+1:], "\n  <node") + first + 1
	nodeSize := second - first
	if len(el)+nodeSize-1 <= budget {
		t.Errorf("element %d bytes + node %d bytes fits the budget: a node was dropped needlessly", len(el), nodeSize)
	}
}

// Acceptance criterion 6, a single node over budget: its shown blocks are
// dropped from the last until it fits; hits keeps the true count.
func TestSearchByteBudgetOneNode(t *testing.T) {
	dir, notes := seeded(t)
	filler := strings.Repeat("b", 8000)
	body := make([]string, 0, 5)
	ids := make([]string, 0, 5)
	for j := range 5 {
		id := fmt.Sprintf("m-huge%02d", j)
		ids = append(ids, id)
		body = append(body, fmt.Sprintf("kettle %s ^%s", filler, id))
	}
	writeNode(t, notes, "topics/huge", "---\ndescription: Huge\n---\n\n"+strings.Join(body, "\n\n")+"\n")
	s, out := runSearch(t, dir, "kettle")
	el := element(out)
	if len(el) > budget {
		t.Fatalf("the element is %d bytes, over the %d budget", len(el), budget)
	}
	check(t, "matched, shown", []string{s.Matched, s.Shown}, []string{"1", "1"})
	if s.Omitted != nil {
		t.Errorf("an omitted notice with nothing omitted: %+v", *s.Omitted)
	}
	n := s.Nodes[0]
	check(t, "hits keeps the true count", n.Hits, "5")
	check(t, "description kept", n.Description, "Huge")
	kept := len(n.Blocks)
	if kept == 0 || kept >= 5 {
		t.Fatalf("kept %d blocks; the budget should drop some but not all", kept)
	}
	got := make([]string, 0, kept)
	for _, b := range n.Blocks {
		got = append(got, b.ID)
	}
	check(t, "blocks dropped from the last", got, ids[:kept])
	blockSize := len("\n    <block id=\"m-huge00\">kettle ") + len(filler) + len("</block>")
	if len(el)+blockSize <= budget {
		t.Errorf("element %d bytes + block %d bytes fits the budget: a block was dropped needlessly", len(el), blockSize)
	}
}

// Acceptance criterion 6, errors: D31's errors with their exact text, and
// more than one argument tells the user to quote the query.
func TestSearchErrors(t *testing.T) {
	dir, _ := seeded(t)
	state := filepath.Join(dir, ".agent")
	ageTree(t, state)
	before := snapshot(t, state)
	cases := []struct {
		args []string
		want string
	}{
		{nil, "query: empty"},
		{[]string{""}, "query: empty"},
		{[]string{"   "}, "query: empty"},
		{[]string{"\t\n"}, "query: empty"},
		{[]string{`kettle "descal`}, "query: unclosed quote"},
		{[]string{"[status"}, "query: unclosed ["},
		{[]string{"tag:"}, "query: empty term"},
		{[]string{`kettle ""`}, "query: empty term"},
		{[]string{"OR kettle"}, "query: OR needs a term on each side"},
		{[]string{"kettle OR OR tea"}, "query: OR needs a term on each side"},
		{[]string{"-kettle"}, "query: nothing to match, only exclusions"},
		{[]string{"tea OR -kettle"}, "query: nothing to match, only exclusions"},
	}
	for _, tc := range cases {
		if line := mustFail(t, runBin(t, dir, nil, append([]string{"search"}, tc.args...)...)); line != tc.want {
			t.Errorf("search %q: stderr %q, want %q", tc.args, line, tc.want)
		}
	}
	line := mustFail(t, runBin(t, dir, nil, "search", "kettle", "descaling"))
	if !strings.Contains(line, "quote") {
		t.Errorf("two arguments: stderr %q does not tell the user to quote the query", line)
	}
	sameSnapshot(t, before, snapshot(t, state))
}

// Acceptance criterion 7 for search: without notes/, Part III.3's message
// and exit 1, creating nothing.
func TestSearchWithoutNotes(t *testing.T) {
	const want = "memory has no notes/ folder; run memory init"
	dir := t.TempDir()
	state := filepath.Join(dir, ".agent")
	writeFile(t, filepath.Join(state, "memory", "protocol", "usage.md"), "guidance\n")
	ageTree(t, state)
	before := snapshot(t, state)
	if line := mustFail(t, runBin(t, dir, nil, "search", "kettle")); line != want {
		t.Errorf("stderr %q, want %q", line, want)
	}
	sameSnapshot(t, before, snapshot(t, state))
}
