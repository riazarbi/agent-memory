package main

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// D31's caps and byte budget. The library ranks and returns every match;
// capping is the caller's.
const (
	maxSearchNodes  = 20
	maxSearchBlocks = 5
	// searchBudget bounds the rendered <memory_search> element, in bytes.
	searchBudget = 30000
)

// searchXML is D31's result.
type searchXML struct {
	XMLName xml.Name    `xml:"memory_search"`
	Query   string      `xml:"query,attr"`
	Matched int         `xml:"matched,attr"`
	Shown   int         `xml:"shown,attr"`
	Nodes   []hitXML    `xml:"node"`
	Omitted *omittedXML `xml:"omitted"`
}

// hitXML is one matching node: hits is its true number of hit blocks,
// however many are shown.
type hitXML struct {
	Stem        string     `xml:"stem,attr"`
	Path        string     `xml:"path,attr"`
	Tags        string     `xml:"tags,attr,omitempty"`
	Modified    string     `xml:"modified,attr"`
	Hits        int        `xml:"hits,attr"`
	Description string     `xml:"description"`
	Blocks      []blockXML `xml:"block"`
}

type omittedXML struct {
	Nodes int    `xml:"nodes,attr"`
	Text  string `xml:",chardata"`
}

// searchVerb is D31's search. The query is one argument, taken verbatim:
// search has no flags, so a query starting with - (an exclusion) is not
// one; a leading -- is dropped, as the end of flags. As in memory_search,
// the arguments are checked, then notes/, then the query is parsed.
func searchVerb(args []string) ([]byte, error) {
	if len(args) > 0 && args[0] == endOfFlags {
		args = args[1:]
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("QUERY: search takes the query as one argument, got %d; quote it: agent-memory search '%s'", len(args), strings.Join(args, " "))
	}
	notes, err := notesDir()
	if err != nil {
		return nil, err
	}
	query := strings.Join(args, "")
	q, err := memory.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	return renderSearch(query, memory.Search(memory.Load(notes), q))
}

// renderSearch renders results, ranked by the library, capped at 20 nodes
// of at most 5 hit blocks, then shrunk to the byte budget.
func renderSearch(query string, results []memory.Result) ([]byte, error) {
	x := searchXML{Query: query, Matched: len(results)}
	for _, r := range results[:min(len(results), maxSearchNodes)] {
		x.Nodes = append(x.Nodes, hitOf(r))
	}
	for {
		x.Shown = len(x.Nodes)
		x.Omitted = nil
		if n := x.Matched - x.Shown; n > 0 {
			x.Omitted = &omittedXML{Nodes: n, Text: fmt.Sprintf("%d more %s matched: narrow the query.", n, nodeNoun(n))}
		}
		out, err := render(x)
		// render adds a final newline, which is not part of the element.
		if err != nil || len(out)-1 <= searchBudget || !x.shrink() {
			return out, err
		}
	}
}

// shrink drops the lowest-ranked node; with one node left, its last shown
// block. A node is never dropped below its description. It reports
// whether anything was dropped.
func (x *searchXML) shrink() bool {
	switch {
	case len(x.Nodes) > 1:
		x.Nodes = x.Nodes[:len(x.Nodes)-1]
	case len(x.Nodes) == 1 && len(x.Nodes[0].Blocks) > 0:
		x.Nodes[0].Blocks = x.Nodes[0].Blocks[:len(x.Nodes[0].Blocks)-1]
	default:
		return false
	}
	return true
}

func hitOf(r memory.Result) hitXML {
	n := r.Node
	x := hitXML{
		Stem:        n.Stem,
		Path:        n.Path,
		Tags:        strings.Join(n.Tags, ","),
		Modified:    n.Modified.Local().Format(dateLayout),
		Hits:        len(r.Hits),
		Description: n.Description,
	}
	for _, b := range r.Hits[:min(len(r.Hits), maxSearchBlocks)] {
		x.Blocks = append(x.Blocks, blockXML{ID: b.ID, Text: b.Text})
	}
	return x
}
