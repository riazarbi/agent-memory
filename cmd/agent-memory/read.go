package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// The read result XML: D29's memory_read rendering.

type nodeXML struct {
	XMLName     xml.Name    `xml:"node"`
	Stem        string      `xml:"stem,attr"`
	Path        string      `xml:"path,attr"`
	Parent      string      `xml:"parent,attr,omitempty"`
	Tags        string      `xml:"tags,attr,omitempty"`
	Modified    string      `xml:"modified,attr"`
	Description string      `xml:"description"`
	Blocks      []blockXML  `xml:"block"`
	Children    *entriesXML `xml:"children"`
	Links       *linksXML   `xml:"links"`
	Backlinks   *linksXML   `xml:"backlinks"`
}

type blockXML struct {
	ID   string `xml:"id,attr,omitempty"`
	Text string `xml:",chardata"`
}

type entriesXML struct {
	Nodes []entryXML `xml:"node"`
}

// entryXML is a child entry (a stat record): stem, tags, true child count,
// date, the description as text, and its own children nested while depth
// lasts.
type entryXML struct {
	Stem        string     `xml:"stem,attr"`
	Tags        string     `xml:"tags,attr,omitempty"`
	Children    int        `xml:"children,attr"`
	Modified    string     `xml:"modified,attr"`
	Description string     `xml:",chardata"`
	Nodes       []entryXML `xml:"node"`
}

type linksXML struct {
	Links []linkXML `xml:"link"`
}

type linkXML struct {
	Stem     string `xml:"stem,attr"`
	Block    string `xml:"block,attr,omitempty"`
	Dangling bool   `xml:"dangling,attr,omitempty"`
}

// rootXML is the root's rendering: the first-level nodes and every tag in
// use with its node count (no top-50 cap: that is the index's budget).
type rootXML struct {
	XMLName xml.Name   `xml:"memory"`
	Nodes   []entryXML `xml:"node"`
	Tags    string     `xml:"tags,omitempty"`
}

const dateLayout = "2006-01-02"

// readArgs are read's parsed arguments.
type readArgs struct {
	stem     string
	hasStem  bool
	block    string
	hasBlock bool
	depth    int
	typed    map[string]string
}

func parseReadArgs(args []string) (readArgs, error) {
	ra := readArgs{depth: 1}
	flags := flag.NewFlagSet("read", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&ra.block, "block", "", "")
	flags.Func("depth", "", func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return errors.New("want a whole number, 0 or more")
		}
		ra.depth = n
		return nil
	})
	pos, typed, err := parseArgs(flags, args)
	if err != nil {
		return ra, err
	}
	ra.typed = typed
	_, ra.hasBlock = typed["block"]
	switch {
	case len(pos) > 1:
		return ra, fmt.Errorf("STEM: read takes one stem, got %d: %s", len(pos), strings.Join(pos, " "))
	case len(pos) == 1:
		ra.stem, ra.hasStem = pos[0], true
	}
	if ra.hasBlock && ra.block == "" {
		return ra, fmt.Errorf("%s: needs a block ID", typed["block"])
	}
	if ra.hasBlock && !ra.hasStem {
		return ra, fmt.Errorf("%s: needs a STEM to read the block from", typed["block"])
	}
	return ra, nil
}

// readVerb is D29's memory_read: a node with its blocks, children to
// --depth, links and backlinks; one block with --block; the root with no
// STEM. Argument errors come before the store is looked at.
func readVerb(args []string) ([]byte, error) {
	ra, err := parseReadArgs(args)
	if err != nil {
		return nil, err
	}
	notes, err := notesDir()
	if err != nil {
		return nil, err
	}
	st := memory.Load(notes)
	if !ra.hasStem {
		return render(rootOf(st, ra.depth))
	}
	n, ok := st.Node(ra.stem)
	if !ok {
		return nil, unknownStem(st, ra.stem)
	}
	if ra.hasBlock {
		x, err := blockOf(st, n, ra.block, ra.typed["block"])
		if err != nil {
			return nil, err
		}
		return render(x)
	}
	return render(nodeOf(st, n, ra.depth))
}

func unknownStem(st *memory.Store, stem string) error {
	all := st.Nodes()
	stems := make([]string, 0, len(all))
	for _, n := range all {
		stems = append(stems, n.Stem)
	}
	msg := fmt.Sprintf("STEM %q: no such node", stem)
	if near := nearMatches(stem, stems); len(near) > 0 {
		msg += "; near matches: " + strings.Join(near, ", ")
	}
	return errors.New(msg)
}

// head is a node's attributes and description, shared by node and block
// reads.
func head(n *memory.Node) nodeXML {
	x := nodeXML{
		Stem:        n.Stem,
		Path:        n.Path,
		Tags:        strings.Join(n.Tags, ","),
		Modified:    n.Modified.Local().Format(dateLayout),
		Description: n.Description,
	}
	if p := n.Parent(); p != nil {
		x.Parent = p.Stem
	}
	return x
}

func nodeOf(st *memory.Store, n *memory.Node, depth int) nodeXML {
	x := head(n)
	for _, b := range n.Blocks {
		x.Blocks = append(x.Blocks, blockXML{ID: b.ID, Text: b.Text})
	}
	if entries := entriesOf(n.Children(), depth); len(entries) > 0 {
		x.Children = &entriesXML{Nodes: entries}
	}
	x.Links = edgesOf(n.Edges())
	x.Backlinks = refsOf(st.Backlinks(n.Stem))
	return x
}

func blockOf(st *memory.Store, n *memory.Node, id, flagName string) (nodeXML, error) {
	b, ok := st.Block(n.Stem, id)
	if !ok {
		return nodeXML{}, fmt.Errorf("%s %q: no such block in node %s", flagName, id, n.Stem)
	}
	x := head(n)
	x.Blocks = []blockXML{{ID: b.ID, Text: b.Text}}
	x.Links = edgesOf(b.Edges)
	x.Backlinks = refsOf(st.BlockBacklinks(n.Stem, id))
	return x, nil
}

// entriesOf renders nodes as child entries, nesting their children while
// depth lasts; depth 0 renders nothing.
func entriesOf(nodes []*memory.Node, depth int) []entryXML {
	if depth <= 0 {
		return nil
	}
	out := make([]entryXML, 0, len(nodes))
	for _, c := range nodes {
		kids := c.Children()
		out = append(out, entryXML{
			Stem:        c.Stem,
			Tags:        strings.Join(c.Tags, ","),
			Children:    len(kids),
			Modified:    c.Modified.Local().Format(dateLayout),
			Description: c.Description,
			Nodes:       entriesOf(kids, depth-1),
		})
	}
	return out
}

func edgesOf(edges []memory.Edge) *linksXML {
	if len(edges) == 0 {
		return nil
	}
	x := &linksXML{}
	for _, e := range edges {
		x.Links = append(x.Links, linkXML{Stem: e.Stem, Block: e.Block, Dangling: e.Dangling})
	}
	return x
}

func refsOf(refs []memory.Ref) *linksXML {
	if len(refs) == 0 {
		return nil
	}
	x := &linksXML{}
	for _, r := range refs {
		x.Links = append(x.Links, linkXML{Stem: r.Stem, Block: r.Block})
	}
	return x
}

// rootOf reads the root: its children are the first-level nodes, so depth
// applies to them as to any node's children.
func rootOf(st *memory.Store, depth int) rootXML {
	counts := map[string]int{}
	for _, n := range st.Nodes() {
		seen := map[string]bool{}
		for _, t := range n.Tags {
			if !seen[t] {
				seen[t] = true
				counts[t]++
			}
		}
	}
	return rootXML{Nodes: entriesOf(st.FirstLevel(), depth), Tags: tagVocabulary(counts)}
}

// tagVocabulary renders tag counts as name(count), most used first, ties by
// name, separated by spaces.
func tagVocabulary(counts map[string]int) string {
	names := make([]string, 0, len(counts))
	for t := range counts {
		names = append(names, t)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, t := range names {
		parts = append(parts, t+"("+strconv.Itoa(counts[t])+")")
	}
	return strings.Join(parts, " ")
}
