package main

// CLI15: one tool file per restructuring verb in agent/tools/, each the
// spec's text and each working when run as the agent runs it
// (docs/spec/agent-memory.md Part III.8, "Tool files" and "Tool
// descriptions").

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// restructuringTools is each restructuring tool file and the verb it runs.
var restructuringTools = map[string]string{
	"memory_move.json":       "move",
	"memory_rename.json":     "rename",
	"memory_move_block.json": "move-block",
	"memory_split.json":      "split",
	"memory_merge.json":      "merge",
	"memory_delete.json":     "delete",
	"memory_retag.json":      "retag",
}

const restructuringHeading = "## III.8 Restructuring verbs"

// toolPath is a shipped tool file's path.
func toolPath(t *testing.T, file string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "agent", "tools", file)
}

// hasEveryToolKey fails unless the file holds all six schema keys (loadTool
// refuses any other key, so together they mean exactly those six).
func hasEveryToolKey(t *testing.T, file string) {
	t.Helper()
	data, err := os.ReadFile(toolPath(t, file))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	decodeJSON(t, data, &raw)
	for k := range shellToolKeys {
		if _, ok := raw[k]; !ok {
			t.Errorf("%s has no key %q", file, k)
		}
	}
	check(t, file+" number of keys", len(raw), len(shellToolKeys))
}

// Acceptance criterion 3: each restructuring tool file has exactly the six
// schema keys, runs memory with its verb pinned after the store, is not
// read-only, takes the model's arguments, forbids none, and carries III.8's
// description for it, read from the spec and never restated here.
func TestRestructuringToolFilesAreTheSpecs(t *testing.T) {
	section := specSection(t, restructuringHeading)
	for file, verb := range restructuringTools {
		hasEveryToolKey(t, file)
		tool := loadTool(t, toolPath(t, file))
		check(t, file+" command", tool.Command, "memory")
		check(t, file+" prepend_args", tool.PrependArgs, []string{"--parent-dir", "${AGENT_STATE_DIR}", verb})
		if tool.ReadOnly == nil || *tool.ReadOnly {
			t.Errorf("%s: read_only is not false", file)
		}
		if tool.AllowUserArgs == nil || !*tool.AllowUserArgs {
			t.Errorf("%s: allow_user_args is not true", file)
		}
		if tool.ForbiddenArgs == nil || len(tool.ForbiddenArgs) != 0 {
			t.Errorf("%s: forbidden_args is %#v, want []", file, tool.ForbiddenArgs)
		}
		want := joinedQuote(t, section, "- `"+file+"`:")
		if !strings.Contains(want, "durable memory") || !strings.Contains(want, "Example") {
			t.Fatalf("the quote found in III.8 for %s is not a tool description: %q", file, want)
		}
		check(t, file+" description", tool.Description, want)
	}
}

// Acceptance criterion 3: no restructuring description says "area", names a
// starting-tree stem as a word, or mentions --parent-dir.
func TestRestructuringDescriptionsPresumeNoTypology(t *testing.T) {
	for file := range restructuringTools {
		d := loadTool(t, toolPath(t, file)).Description
		if strings.Contains(strings.ToLower(d), "area") {
			t.Errorf("%s says area: %q", file, d)
		}
		if w := typologyWord.FindString(d); w != "" {
			t.Errorf("%s names %q: %q", file, w, d)
		}
		if strings.Contains(d, "parent-dir") {
			t.Errorf("%s mentions --parent-dir: %q", file, d)
		}
	}
}

// exampleArray matches a JSON array of strings, as a description writes an
// example argument list.
var exampleArray = regexp.MustCompile(`\["(?:[^"\\]|\\.)*"(?:, "(?:[^"\\]|\\.)*")*\]`)

// examplesIn is every example argument list in a tool's description, in
// order. Every `["` in the description must open one, so none is missed.
func examplesIn(t *testing.T, file string) [][]string {
	t.Helper()
	d := loadTool(t, toolPath(t, file)).Description
	found := exampleArray.FindAllString(d, -1)
	if len(found) == 0 || len(found) != strings.Count(d, `["`) {
		t.Fatalf("%s: found %d examples %q, but the description opens %d", file, len(found), found, strings.Count(d, `["`))
	}
	out := make([][]string, 0, len(found))
	for _, f := range found {
		var args []string
		decodeJSON(t, []byte(f), &args)
		out = append(out, args)
	}
	return out
}

// parentPlaceholder is how an example writes "some node of yours".
const parentPlaceholder = "<parent stem>"

// fill is an example's argument list for a fixture: the same arguments in
// the same order, with the parent placeholder replaced by parent and each
// block ID, in order, by the next of ids. Everything else is the
// example's own.
func fill(t *testing.T, example []string, parent string, ids ...string) []string {
	t.Helper()
	out := make([]string, 0, len(example))
	for _, a := range example {
		switch {
		case a == parentPlaceholder:
			a = parent
		case blockIDPattern.MatchString(a):
			if len(ids) == 0 {
				t.Fatalf("the example %q names more block IDs than the fixture gives", example)
			}
			a, ids = ids[0], ids[1:]
		}
		out = append(out, a)
	}
	if len(ids) != 0 {
		t.Fatalf("the example %q names fewer block IDs than the fixture gives (%q left)", example, ids)
	}
	return out
}

// valueOf is the argument after flag in an example.
func valueOf(t *testing.T, example []string, flag string) string {
	t.Helper()
	i := slices.Index(example, flag)
	if i < 0 || i+1 >= len(example) {
		t.Fatalf("the example %q gives no %s", example, flag)
	}
	return example[i+1]
}

// toolRig is an agent's state dir holding no store yet, and a way to run
// the shipped tool files against it as the agent runs a shell tool.
type toolRig struct {
	dir, state string
}

const shelf = "shelf" // a first-level node every fixture starts with

func newToolRig(t *testing.T) *toolRig {
	t.Helper()
	dir := t.TempDir()
	r := &toolRig{dir: dir, state: filepath.Join(dir, ".agent")}
	if err := os.MkdirAll(r.state, 0o755); err != nil {
		t.Fatal(err)
	}
	r.write(t, shelf, "--parent", "/", "--description", "A place to put things")
	return r
}

// run runs a shipped tool file: its command's prepend_args with
// ${AGENT_STATE_DIR} replaced by the state dir, then the model's arguments,
// which the file must allow. MEMORY_PARENT_DIR names a folder that does not
// exist, so the store can only come from the file's --parent-dir.
func (r *toolRig) run(t *testing.T, file string, userArgs ...string) result {
	t.Helper()
	tool := loadTool(t, toolPath(t, file))
	check(t, file+" command", tool.Command, "memory")
	if tool.AllowUserArgs != nil && !*tool.AllowUserArgs && len(userArgs) > 0 {
		t.Fatalf("%s takes no user arguments", file)
	}
	argv := make([]string, 0, len(tool.PrependArgs)+len(userArgs))
	for _, a := range tool.PrependArgs {
		argv = append(argv, strings.ReplaceAll(a, "${AGENT_STATE_DIR}", r.state))
	}
	for _, a := range userArgs {
		if slices.Contains(tool.ForbiddenArgs, a) {
			t.Fatalf("%s forbids %q, which this call needs", file, a)
		}
		argv = append(argv, a)
	}
	return runBin(t, r.dir, map[string]string{"MEMORY_PARENT_DIR": filepath.Join(r.dir, "decoy")}, argv...)
}

// ok is run for a call that must succeed: exit 0 and nothing on stderr.
func (r *toolRig) ok(t *testing.T, file string, userArgs ...string) result {
	t.Helper()
	res := r.run(t, file, userArgs...)
	if res.code != 0 || res.stderr != "" {
		t.Fatalf("%s %q: exit %d, stderr %q", file, userArgs, res.code, res.stderr)
	}
	return res
}

// call is ok's stdout.
func (r *toolRig) call(t *testing.T, file string, userArgs ...string) string {
	t.Helper()
	return r.ok(t, file, userArgs...).stdout
}

// write builds the fixture with the shipped memory_write.json and returns
// the block IDs it assigned.
func (r *toolRig) write(t *testing.T, args ...string) []string {
	t.Helper()
	return decodeWrite(t, r.run(t, "memory_write.json", args...)).Assigned
}

// node reads a node with the shipped memory_read.json.
func (r *toolRig) node(t *testing.T, stem string) xNode {
	t.Helper()
	var n xNode
	decode(t, r.call(t, "memory_read.json", stem), &n)
	check(t, "read "+stem, n.Stem, stem)
	return n
}

// gone fails unless no node has that stem.
func (r *toolRig) gone(t *testing.T, stem string) {
	t.Helper()
	line := mustFail(t, r.run(t, "memory_read.json", stem))
	if !strings.Contains(line, "no such node") {
		t.Errorf("read %s: %q, want no such node", stem, line)
	}
}

// exampleRun exercises one example of one tool file on a fresh fixture.
type exampleRun func(t *testing.T, file string, example []string)

// exampleRuns is how each restructuring tool file's examples are run and
// what each must have done.
var exampleRuns = map[string]exampleRun{
	"memory_move.json":       runMoveExample,
	"memory_rename.json":     runRenameExample,
	"memory_move_block.json": runMoveBlockExample,
	"memory_split.json":      runSplitExample,
	"memory_merge.json":      runMergeExample,
	"memory_delete.json":     runDeleteExample,
	"memory_retag.json":      runRetagExample,
}

// Acceptance criterion 4: every example in every restructuring tool's
// description, run through that tool file as the agent runs it, exits 0 and
// does what the verb does. The examples are read out of the descriptions.
func TestRestructuringToolExamplesRun(t *testing.T) {
	check(t, "tool files with an example run", len(exampleRuns), len(restructuringTools))
	for file := range restructuringTools {
		for _, example := range examplesIn(t, file) {
			t.Run(file+" "+strings.Join(example, " "), func(t *testing.T) {
				exampleRuns[file](t, file, example)
			})
		}
	}
}

// The node moves, with its child, under the parent given.
func runMoveExample(t *testing.T, file string, example []string) {
	const other, child = "other-shelf", "moved-child"
	r := newToolRig(t)
	stem := example[0]
	r.write(t, other, "--parent", "/", "--description", "Another place")
	r.write(t, stem, "--parent", shelf, "--description", "Filed in the wrong place")
	r.write(t, child, "--parent", stem, "--description", "Goes where its parent goes")

	got := decodeMove(t, r.ok(t, file, fill(t, example, other)...))
	check(t, "result", got, xMove{XMLName: got.XMLName, Stem: stem, From: shelf, Parent: other, Path: other + "/" + stem})
	n := r.node(t, stem)
	check(t, "moved node's parent, path", []string{n.Parent, n.Path}, []string{other, other + "/" + stem})
	check(t, "its child's path", r.node(t, child).Path, other+"/"+stem+"/"+child)
	check(t, "what is left under the old parent", childStems(r.node(t, shelf)), []string(nil))
}

// The node has the new stem, the old one is gone, and a link follows.
func runRenameExample(t *testing.T, file string, example []string) {
	const pointer = "pointer"
	r := newToolRig(t)
	if len(example) != 2 {
		t.Fatalf("the rename example %q is not an old and a new stem", example)
	}
	old, renamed := example[0], example[1]
	id := r.write(t, old, "--parent", shelf, "--description", "About to be renamed", "--add", "A claim.")[0]
	r.write(t, pointer, "--parent", shelf, "--description", "Points at it", "--add", "See [["+old+"]].")

	got := decodeRename(t, r.ok(t, file, fill(t, example, "")...))
	check(t, "result", got, xRename{XMLName: got.XMLName, From: old, Stem: renamed, Links: "1"})
	r.gone(t, old)
	check(t, "the renamed node's blocks", idsOf(r.node(t, renamed)), []string{id})
	check(t, "the link to it", r.node(t, pointer).Blocks[0].Text, "See [["+renamed+"]].")
}

// The block leaves its node and is the last block of the node named.
func runMoveBlockExample(t *testing.T, file string, example []string) {
	const source = "source-node"
	r := newToolRig(t)
	to := valueOf(t, example, "--to")
	ids := r.write(t, source, "--parent", shelf, "--description", "Holds a misfiled claim", "--add", "Stays.", "--add", "Misfiled.")
	kept := r.write(t, to, "--parent", shelf, "--description", "Where the claim belongs", "--add", "Already here.")[0]

	got := decodeMoveBlock(t, r.ok(t, file, fill(t, example, "", ids[1])...))
	check(t, "result", got, xMoveBlock{XMLName: got.XMLName, ID: ids[1], From: source, Stem: to, Links: "0"})
	check(t, "blocks left in the old node", idsOf(r.node(t, source)), []string{ids[0]})
	check(t, "blocks of the node named", r.node(t, to).Blocks, []xBlock{{ID: kept, Text: "Already here."}, {ID: ids[1], Text: "Misfiled."}})
}

// The new node exists where --parent says, with the example's description
// and the named blocks in the order named; the rest stay.
func runSplitExample(t *testing.T, file string, example []string) {
	r := newToolRig(t)
	stem, into := example[0], valueOf(t, example, "--into")
	ids := r.write(t, stem, "--parent", shelf, "--description", "Covers two subjects",
		"--add", "First.", "--add", "Second.", "--add", "Third.")

	var got xSplit
	decode(t, r.call(t, file, fill(t, example, shelf, ids[2], ids[0])...), &got)
	check(t, "result", got, xSplit{XMLName: got.XMLName, From: stem, Stem: into, Parent: shelf, Path: shelf + "/" + into,
		Blocks: "2", Left: "1", Links: "0"})
	check(t, "result element", got.XMLName.Local, "memory_split")
	n := r.node(t, into)
	check(t, "new node", attrsOf(n), attrs{Stem: into, Path: shelf + "/" + into, Parent: shelf,
		Description: valueOf(t, example, "--description")})
	check(t, "new node's blocks", n.Blocks, []xBlock{{ID: ids[2], Text: "Third."}, {ID: ids[0], Text: "First."}})
	check(t, "blocks left", idsOf(r.node(t, stem)), []string{ids[1]})
}

// The folded-in node is gone; its block, child and tag are the kept node's.
func runMergeExample(t *testing.T, file string, example []string) {
	const child = "merged-child"
	r := newToolRig(t)
	from, to := example[0], valueOf(t, example, "--into")
	moved := r.write(t, from, "--parent", shelf, "--description", "The weaker node", "--tag", "extra", "--add", "From the other.")[0]
	r.write(t, child, "--parent", from, "--description", "Goes to the kept node")
	kept := r.write(t, to, "--parent", shelf, "--description", "The node to keep", "--tag", "own", "--add", "Already here.")[0]

	var got xMerge
	decode(t, r.call(t, file, fill(t, example, "")...), &got)
	check(t, "result", got, xMerge{XMLName: got.XMLName, From: from, Stem: to, Blocks: "1", Children: "1", TagsAdded: "1", Links: "0"})
	check(t, "result element", got.XMLName.Local, "memory_merge")
	r.gone(t, from)
	n := r.node(t, to)
	check(t, "kept node", attrsOf(n), attrs{Stem: to, Path: shelf + "/" + to, Parent: shelf, Tags: "own,extra", Description: "The node to keep"})
	check(t, "kept node's blocks", idsOf(n), []string{kept, moved})
	check(t, "kept node's children", childStems(n), []string{child})
}

// One block goes (--block), or the node (no flag), or the node and what is
// under it (--recursive).
func runDeleteExample(t *testing.T, file string, example []string) {
	const child = "deleted-child"
	r := newToolRig(t)
	stem := example[0]
	ids := r.write(t, stem, "--parent", shelf, "--description", "Has something to delete", "--add", "Wrong.", "--add", "Right.")
	switch {
	case slices.Contains(example, "--block"):
		got := decodeDelete(t, r.ok(t, file, fill(t, example, "", ids[0])...))
		check(t, "result", []string{got.Stem, got.Block, got.Nodes, got.Blocks}, []string{stem, ids[0], "0", "1"})
		check(t, "blocks left", idsOf(r.node(t, stem)), []string{ids[1]})
	case slices.Contains(example, "--recursive"):
		r.write(t, child, "--parent", stem, "--description", "Under the node", "--add", "Also going.")
		got := decodeDelete(t, r.ok(t, file, fill(t, example, "")...))
		check(t, "result", []string{got.Stem, got.Nodes, got.Blocks}, []string{stem, "2", "3"})
		r.gone(t, stem)
		r.gone(t, child)
	default:
		check(t, "the plain delete example", example, []string{stem})
		got := decodeDelete(t, r.ok(t, file, example...))
		check(t, "result", []string{got.Stem, got.Nodes, got.Blocks}, []string{stem, "1", "2"})
		r.gone(t, stem)
	}
	check(t, "the fixture's first-level node is still there", r.node(t, shelf).Stem, shelf)
}

// The tag is renamed (two tags) or removed (one) on the node that has it.
func runRetagExample(t *testing.T, file string, example []string) {
	const tagged = "tagged-node"
	r := newToolRig(t)
	r.write(t, tagged, "--parent", shelf, "--description", "Carries the tag", "--tag", "kept", "--tag", example[0])
	want := "kept"
	switch len(example) {
	case 1:
	case 2:
		want += "," + example[1]
	default:
		t.Fatalf("the retag example %q is neither one tag nor two", example)
	}
	out := r.call(t, file, fill(t, example, "")...)
	if !strings.HasPrefix(out, `<memory_retag from="`+example[0]+`"`) || !strings.Contains(out, ` nodes="1"`) {
		t.Errorf("result %q does not say one node lost the tag %s", out, example[0])
	}
	check(t, "the node's tags", r.node(t, tagged).Tags, want)
}
