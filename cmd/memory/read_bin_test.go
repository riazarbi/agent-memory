package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// seeded runs init in a fresh directory and returns it with its notes/.
func seeded(t *testing.T) (dir, notes string) {
	t.Helper()
	dir = t.TempDir()
	mustSucceed(t, runBin(t, dir, nil, "init"))
	return dir, filepath.Join(dir, ".agent", "memory", "notes")
}

// Acceptance criterion 3: read with no stem is the root — the first-level
// nodes as child entries and every tag with its node count.
func TestReadRoot(t *testing.T) {
	dir, notes := seeded(t)
	writeNode(t, notes, "topics/outbox", "---\ndescription: Outbox rules\ntags: [escalation, outbox]\n---\n")
	writeNode(t, notes, "people/priya", "---\ndescription: Priya\ntags: [escalation, people/priya]\n---\n")

	r := runBin(t, dir, nil, "read")
	mustSucceed(t, r)
	var root xRoot
	decode(t, r.stdout, &root)
	check(t, "root element", root.XMLName.Local, "memory")
	got := make([]entry, 0, len(root.Nodes))
	for _, e := range root.Nodes {
		got = append(got, entryOf(e))
		check(t, e.Stem+" nested entries at depth 1", len(e.Nodes), 0)
		check(t, e.Stem+" has a date", len(e.Modified), len("2006-01-02"))
	}
	check(t, "root entries", got, []entry{
		{Stem: "people", Children: "1", Desc: seedDescriptions["people"]},
		{Stem: "projects", Children: "0", Desc: seedDescriptions["projects"]},
		{Stem: "topics", Children: "1", Desc: seedDescriptions["topics"]},
		{Stem: "user", Children: "0", Desc: seedDescriptions["user"]},
	})
	if root.Tags == nil {
		t.Fatal("root has no <tags>")
	}
	check(t, "<tags>", *root.Tags, "escalation(2) outbox(1) people/priya(1)")
}

// The root's tag list has no top-50 cap (D29 ruling): every tag appears.
func TestReadRootListsEveryTag(t *testing.T) {
	dir, notes := seeded(t)
	tags := make([]string, 0, 60)
	for i := range 60 {
		tags = append(tags, fmt.Sprintf("tag-%02d", i))
	}
	writeNode(t, notes, "topics/many", "---\ndescription: Many tags\ntags: ["+strings.Join(tags, ", ")+"]\n---\n")
	writeNode(t, notes, "user/one", "---\ndescription: One tag\ntags: [tag-59]\n---\n")

	r := runBin(t, dir, nil, "read")
	mustSucceed(t, r)
	var root xRoot
	decode(t, r.stdout, &root)
	if root.Tags == nil {
		t.Fatal("root has no <tags>")
	}
	got := strings.Fields(*root.Tags)
	if len(got) != 60 {
		t.Fatalf("<tags> lists %d tags, want all 60: %q", len(got), *root.Tags)
	}
	if got[0] != "tag-59(2)" || got[1] != "tag-00(1)" || got[59] != "tag-58(1)" {
		t.Errorf("<tags> order: most used first, ties by name; got %v", got)
	}
}

// edgeStore builds the criterion-4 fixture: outbox-etiquette under topics,
// with two blocks, a child (with its own child), a link to people, a link to
// a missing ghost, and backlinks from priya (to a block) and from user (to
// the node).
func edgeStore(t *testing.T) string {
	t.Helper()
	dir, notes := seeded(t)
	writeNode(t, notes, "topics/outbox-etiquette", `---
description: How and when to cc people on outbox replies
tags: [escalation, outbox]
status: draft
---

Forward urgent things to Priya via outbox cc, see [[people]]. Text with <b> & "quotes". ^m-aaaaa1

Never cc on routine answers; ask [[ghost]] about it. ^m-aaaaa2
`)
	writeNode(t, notes, "topics/outbox-etiquette/cc-rules", "---\ndescription: The cc rules\ntags: [outbox]\n---\n")
	writeNode(t, notes, "topics/outbox-etiquette/cc-rules/cc-deep", "---\ndescription: Deeper rules\n---\n")
	writeNode(t, notes, "people/priya", "---\ndescription: Priya\n---\n\nPriya asked for [[outbox-etiquette#^m-aaaaa1]]. ^m-bbbbb1\n")
	writeNode(t, notes, "user/prefs", "---\ndescription: Preferences\n---\n\nSee [[outbox-etiquette]]. ^m-ccccc1\n")
	return dir
}

func readNode(t *testing.T, dir string, args ...string) xNode {
	t.Helper()
	r := runBin(t, dir, nil, append([]string{"read"}, args...)...)
	mustSucceed(t, r)
	var n xNode
	decode(t, r.stdout, &n)
	if n.XMLName.Local != "node" {
		t.Fatalf("result element <%s>, want <node>", n.XMLName.Local)
	}
	return n
}

func links(l *xLinks) []xLink {
	if l == nil {
		return nil
	}
	return l.Links
}

var outboxAttrs = attrs{
	Stem:        "outbox-etiquette",
	Path:        "topics/outbox-etiquette",
	Parent:      "topics",
	Tags:        "escalation,outbox",
	Description: "How and when to cc people on outbox replies",
}

// Acceptance criterion 4: read STEM renders the node with its edges.
func TestReadNode(t *testing.T) {
	dir := edgeStore(t)
	n := readNode(t, dir, "outbox-etiquette")

	check(t, "attributes", attrsOf(n), outboxAttrs)
	if m := n.Modified; len(m) != len("2006-01-02") || m[4] != '-' || m[7] != '-' {
		t.Errorf("modified=%q, want YYYY-MM-DD", m)
	}
	check(t, "blocks", n.Blocks, []xBlock{
		{ID: "m-aaaaa1", Text: `Forward urgent things to Priya via outbox cc, see [[people]]. Text with <b> & "quotes".`},
		{ID: "m-aaaaa2", Text: "Never cc on routine answers; ask [[ghost]] about it."},
	})
	if n.Children == nil || len(n.Children.Nodes) != 1 {
		t.Fatalf("children %+v, want cc-rules", n.Children)
	}
	child := n.Children.Nodes[0]
	check(t, "child entry", entryOf(child), entry{Stem: "cc-rules", Tags: "outbox", Children: "1", Desc: "The cc rules"})
	check(t, "child date", len(child.Modified), len("2006-01-02"))
	check(t, "grandchildren at the default depth 1", len(child.Nodes), 0)
	check(t, "links", links(n.Links), []xLink{{Stem: "people"}, {Stem: "ghost", Dangling: "true"}})
	// Ordered by stem, then block ID (D25).
	check(t, "backlinks", links(n.Backlinks), []xLink{{Stem: "prefs", Block: "m-ccccc1"}, {Stem: "priya", Block: "m-bbbbb1"}})
}

// --depth 0 omits children; --depth 2 includes grandchildren. Flags work
// before or after STEM, and as --depth=N.
func TestReadDepth(t *testing.T) {
	dir := edgeStore(t)
	for _, args := range [][]string{
		{"outbox-etiquette", "--depth", "0"},
		{"--depth", "0", "outbox-etiquette"},
		{"outbox-etiquette", "--depth=0"},
		{"outbox-etiquette", "-depth", "0"},
	} {
		n := readNode(t, dir, args...)
		what := strings.Join(args, " ")
		check(t, what+": children present", n.Children != nil, false)
		check(t, what+": blocks", len(n.Blocks), 2)
		check(t, what+": backlinks", len(links(n.Backlinks)), 2)
	}
	for _, args := range [][]string{
		{"outbox-etiquette", "--depth", "2"},
		{"--depth", "2", "outbox-etiquette"},
	} {
		n := readNode(t, dir, args...)
		if n.Children == nil || len(n.Children.Nodes) != 1 {
			t.Fatalf("%v: children %+v", args, n.Children)
		}
		kids := n.Children.Nodes[0].Nodes
		grand := make([]entry, 0, len(kids))
		for _, g := range kids {
			grand = append(grand, entryOf(g))
		}
		check(t, strings.Join(args, " ")+": grandchildren", grand, []entry{{Stem: "cc-deep", Children: "0", Desc: "Deeper rules"}})
	}
}

// --block ID returns the attributes, the description and that one block,
// with the block's own links and backlinks, and no children.
func TestReadBlock(t *testing.T) {
	dir := edgeStore(t)
	for _, args := range [][]string{
		{"outbox-etiquette", "--block", "m-aaaaa1"},
		{"--block", "m-aaaaa1", "outbox-etiquette"},
		{"outbox-etiquette", "--block=m-aaaaa1", "--depth", "2"},
	} {
		n := readNode(t, dir, args...)
		what := strings.Join(args, " ") + ": "
		check(t, what+"attributes", attrsOf(n), outboxAttrs)
		check(t, what+"has a date", len(n.Modified), len("2006-01-02"))
		check(t, what+"blocks", n.Blocks, []xBlock{{ID: "m-aaaaa1", Text: `Forward urgent things to Priya via outbox cc, see [[people]]. Text with <b> & "quotes".`}})
		check(t, what+"children present", n.Children != nil, false)
		check(t, what+"block links", links(n.Links), []xLink{{Stem: "people"}})
		check(t, what+"block backlinks", links(n.Backlinks), []xLink{{Stem: "priya", Block: "m-bbbbb1"}})
	}

	n := readNode(t, dir, "outbox-etiquette", "--block", "m-aaaaa2")
	check(t, "m-aaaaa2 links", links(n.Links), []xLink{{Stem: "ghost", Dangling: "true"}})
	check(t, "m-aaaaa2 backlinks", links(n.Backlinks), []xLink(nil))
}
