package main

// CLI7: no built-in typology. The binary seeds nothing; a missing store is
// an empty one and only write creates it; the root is "/"; the starting
// tree ships as files in agent/memory/notes/ (docs/spec/agent-memory.md
// Part III.6, III.1).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// binCmd is runBin's command, unstarted: the same environment rules, so
// it can run in a goroutine or under a wrapper, where runBin's t.Fatalf
// cannot.
func binCmd(dir string, env map[string]string, args ...string) *exec.Cmd {
	cmd := exec.Command(binPath, args...) //nolint:gosec // the binary this package builds
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "MEMORY_DIR", "AGENT_STATE_DIR", "TZ", "HOME", "MEMORY_PARENT_DIR":
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	vars := runBinDefaults(dir)
	for k, v := range env {
		vars[k] = v
	}
	for k, v := range vars {
		if v != envUnset {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	return cmd
}

// execute runs cmd and returns its result; an error that is not an exit
// status is returned as such.
func execute(cmd *exec.Cmd) (result, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return result{}, err
		}
		code = exit.ExitCode()
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}, nil
}

// runBinUmask is runBin with the process umask set to mask first, so modes
// the binary forces show as forced.
func runBinUmask(t *testing.T, dir string, mask int, args ...string) result {
	t.Helper()
	cmd := binCmd(dir, nil)
	cmd.Path = "/bin/sh"
	cmd.Args = append([]string{"sh", "-c", fmt.Sprintf(`umask %03o && exec "$0" "$@"`, mask), binPath}, args...)
	r, err := execute(cmd)
	if err != nil {
		t.Fatalf("running memory %v: %v", args, err)
	}
	return r
}

// rootStems is the stems a successful root read lists.
func rootStems(t *testing.T, r result) []string {
	t.Helper()
	mustSucceed(t, r)
	var root xRoot
	decode(t, r.stdout, &root)
	check(t, "root element", root.XMLName.Local, "memory")
	stems := make([]string, 0, len(root.Nodes))
	for _, n := range root.Nodes {
		stems = append(stems, n.Stem)
	}
	return stems
}

// failsNaming checks the error convention and that the line starts with
// name and a colon: the argument as the user typed it.
func failsNaming(t *testing.T, r result, name string) string {
	t.Helper()
	line := mustFail(t, r)
	if !strings.HasPrefix(line, name+":") {
		t.Errorf("stderr %q does not name %s", line, name)
	}
	return line
}

// Acceptance criterion 1: a missing store - no memory/, or memory/ without
// notes/ - reads as an empty one, and reading it creates nothing.
func TestMissingStoreIsEmpty(t *testing.T) {
	for _, withMemory := range []bool{false, true} {
		what := fmt.Sprintf("memory/ exists: %v", withMemory)
		w, parent := freshParent(t)
		if withMemory {
			mkdirs(t, filepath.Join(parent, "memory"))
		}
		read := runBin(t, w, nil, "read")
		mustSucceed(t, read)
		var root xRoot
		decode(t, read.stdout, &root)
		check(t, what+": root element", root.XMLName.Local, "memory")
		check(t, what+": root nodes", len(root.Nodes), 0)
		check(t, what+": root has <tags>", root.Tags != nil, false)

		slash := runBin(t, w, nil, "read", "/")
		mustSucceed(t, slash)
		check(t, what+": read / is read", slash.stdout, read.stdout)

		index := runBin(t, w, nil, "index")
		mustSucceed(t, index)
		check(t, what+": index stdout", index.stdout, wantEmptyIndex)

		search := runBin(t, w, nil, "search", "anything")
		mustSucceed(t, search)
		var s xSearch
		decode(t, search.stdout, &s)
		check(t, what+": search element", s.XMLName.Local, "memory_search")
		check(t, what+": search matched, shown", []string{s.Matched, s.Shown}, []string{"0", "0"})

		line := mustFail(t, runBin(t, w, nil, "read", "kettle"))
		check(t, what+": unknown stem", line, `STEM "kettle": no such node`)

		check(t, what+": the parent", treeUnder(t, parent), emptyStoreTree)
	}
}

// Acceptance criterion 2: read / is the root read, the same as read with no
// stem, on a store with nodes too, and with --depth.
func TestReadSlashIsTheRoot(t *testing.T) {
	w, _ := seeded(t)
	for _, extra := range [][]string{nil, {"--depth", "0"}, {"--depth", "2"}} {
		plain := runBin(t, w, nil, append([]string{"read"}, extra...)...)
		mustSucceed(t, plain)
		slash := runBin(t, w, nil, append([]string{"read", "/"}, extra...)...)
		mustSucceed(t, slash)
		check(t, fmt.Sprintf("read / %v", extra), slash.stdout, plain.stdout)
	}
	check(t, "root stems", rootStems(t, runBin(t, w, nil, "read", "/")), []string{"people", "projects", "topics", "user"})
}

// Acceptance criterion 2: the root is "/". write --parent / creates a
// first-level node on an empty store, making memory/ and notes/ with mode
// 0755 whatever the umask; a child goes under it; an update of a
// first-level node may give --parent /, an update of a nested node may
// not; "/" is never a stem to write or a node to read a block from.
func TestRootIsSlash(t *testing.T) {
	w, parent := freshParent(t)
	notes := filepath.Join(parent, "memory", "notes")
	created := decodeWrite(t, runBinUmask(t, w, 0o077, "write", "kettle", "--parent", "/", "--description", "d"))
	check(t, "created", created.Created, "true")
	check(t, "kettle", attrsOf(*created.Node), attrs{Stem: "kettle", Path: "kettle", Description: "d"})
	if !exists(filepath.Join(notes, "kettle", "kettle.md")) {
		t.Fatal("notes/kettle/kettle.md does not exist")
	}
	for _, d := range []string{filepath.Join(parent, "memory"), notes} {
		info, err := os.Stat(d)
		if err != nil {
			t.Fatal(err)
		}
		if m := info.Mode().Perm(); m != 0o755 {
			t.Errorf("%s mode %v, want 0755 (forced, under umask 077)", d, m)
		}
	}
	check(t, "root stems", rootStems(t, runBin(t, w, nil, "read")), []string{"kettle"})

	child := decodeWrite(t, runBin(t, w, nil, "write", "descale", "--parent", "kettle", "--description", "e", "--add", "a"))
	check(t, "child created", child.Created, "true")
	check(t, "child", attrsOf(*child.Node), attrs{Stem: "descale", Path: "kettle/descale", Parent: "kettle", Description: "e"})

	update := decodeWrite(t, runBin(t, w, nil, "write", "kettle", "--parent", "/", "--add", "b"))
	check(t, "update created", update.Created, "false")
	check(t, "update blocks", len(update.Node.Blocks), 1)

	ageTree(t, parent)
	before := snapshot(t, parent)
	line := failsNaming(t, runBin(t, w, nil, "write", "descale", "--parent", "/", "--add", "c"), "--parent")
	if !strings.Contains(line, "kettle") {
		t.Errorf("stderr %q does not name descale's real parent", line)
	}
	failsNaming(t, runBin(t, w, nil, "write", "/", "--description", "x"), "STEM")
	failsNaming(t, runBin(t, w, nil, "write", "/", "--parent", "/", "--description", "x"), "STEM")
	failsNaming(t, runBin(t, w, nil, "read", "/", "--block", "m-aaaaaa"), "STEM")
	failsNaming(t, runBin(t, w, nil, "read", "--block", "m-aaaaaa", "/"), "STEM")
	sameSnapshot(t, before, snapshot(t, parent))
}

// Acceptance criterion 2: the shipped memory_write.json, called as the
// agent calls it, creates a first-level node on an empty store.
func TestShippedWriteToolCreatesAtTheRoot(t *testing.T) {
	w, parent := freshParent(t)
	call := toolCaller(t, w)
	var res xWrite
	decode(t, call("memory_write.json", "kettle", "--parent", "/", "--description", "d"), &res)
	check(t, "created, stem", []string{res.Created, res.Stem}, []string{"true", "kettle"})
	if !exists(filepath.Join(parent, "memory", "notes", "kettle", "kettle.md")) {
		t.Error("the tool's write did not create notes/kettle/kettle.md in the agent's store")
	}
}

// Acceptance criterion 3: a refused write on a missing store leaves exactly
// memory/, memory/notes/ and the lock file; an argument error leaves
// nothing at all.
func TestWriteCreatesTheStore(t *testing.T) {
	w, parent := freshParent(t)
	failsNaming(t, runBin(t, w, nil, "write", "kettle", "--parent", "nope", "--description", "d"), "--parent")
	check(t, "the parent after a refused write", treeUnder(t, parent),
		[]string{"memory/", "memory/notes/", "memory/notes/.write.lock"})

	// memory/ already there: only notes/ is made, and memory/ keeps its mode.
	w, parent = freshParent(t)
	mem := filepath.Join(parent, "memory")
	mkdirs(t, mem)
	if err := os.Chmod(mem, 0o700); err != nil {
		t.Fatal(err)
	}
	mustSucceed(t, runBin(t, w, nil, firstWrite...))
	check(t, "the parent after a write into an existing memory/", treeUnder(t, parent),
		[]string{"memory/", "memory/notes/", "memory/notes/.write.lock", "memory/notes/kettle/", "memory/notes/kettle/kettle.md"})
	if m := fileMode(t, mem); m != 0o700 {
		t.Errorf("an existing memory/ changed mode to %v", m)
	}

	for _, args := range [][]string{
		{"write"},
		{"write", "--parent", "/", "--description", "d"},
		{"write", "/", "--parent", "/", "--description", "d"},
		{"write", "kettle", "--bogus"},
		{"write", "kettle", "other", "--parent", "/"},
	} {
		w, parent := freshParent(t)
		mustFail(t, runBin(t, w, nil, args...))
		isEmptyDir(t, strings.Join(args, " "), parent)
	}
}

func fileMode(t *testing.T, p string) os.FileMode {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// Acceptance criterion 3: eight writes started together on a fresh store
// all succeed - none fails racing another to create memory/ or notes/ -
// leaving eight first-level nodes. Several rounds, as the race is timing.
func TestConcurrentFirstWrites(t *testing.T) {
	const writers = 8
	for round := range 5 {
		w, _ := freshParent(t)
		cmds := make([]*exec.Cmd, writers)
		for i := range cmds {
			cmds[i] = binCmd(w, nil, "write", fmt.Sprintf("k%d", i), "--parent", "/", "--description", "d")
		}
		results := make([]result, writers)
		errs := make([]error, writers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, cmd := range cmds {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results[i], errs[i] = execute(cmd)
			}()
		}
		close(start)
		wg.Wait()
		for i := range cmds {
			if errs[i] != nil {
				t.Fatalf("round %d, writer k%d: %v", round, i, errs[i])
			}
			if r := results[i]; r.code != 0 || r.stderr != "" {
				t.Errorf("round %d, writer k%d: exit %d, stderr %q", round, i, r.code, r.stderr)
			}
		}
		want := make([]string, 0, writers)
		for i := range writers {
			want = append(want, fmt.Sprintf("k%d", i))
		}
		check(t, fmt.Sprintf("round %d: root stems", round), rootStems(t, runBin(t, w, nil, "read")), want)
	}
}

// starterStems is the shipped starting tree's first level (III.6).
var starterStems = []string{"people", "projects", "topics", "user"}

// specSection returns the spec's text from the heading line starting with
// heading up to the next "## " heading.
func specSection(t *testing.T, heading string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "spec", "agent-memory.md"))
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	start := strings.Index(spec, "\n"+heading)
	if start < 0 {
		t.Fatalf("the spec has no heading %q", heading)
	}
	body := spec[start+1:]
	if end := strings.Index(body[1:], "\n## "); end >= 0 {
		body = body[:end+1]
	}
	return body
}

const noTypologyHeading = "## III.6 No built-in typology"

// Acceptance criterion 4: agent/memory/ holds exactly the four starting
// nodes, each byte-equal to what write <stem> --parent / --description <D>
// creates on an empty store, with III.6's descriptions; they load clean.
func TestShippedStartingTree(t *testing.T) {
	shipped := filepath.Join(repoRoot(t), "agent", "memory")
	files := make([]string, 0, len(starterStems))
	for _, s := range starterStems {
		files = append(files, "notes/"+s+"/"+s+".md")
	}
	check(t, "agent/memory/", treeUnder(t, shipped), withDirs("", files))

	section := strings.Join(strings.Fields(specSection(t, noTypologyHeading)), " ")
	for _, stem := range starterStems {
		desc := seedDescriptions[stem]
		if !strings.Contains(section, fmt.Sprintf("`%s` (%q)", stem, desc)) {
			t.Errorf("III.6 does not give %s the description %q", stem, desc)
		}
		w, parent := freshParent(t)
		mustSucceed(t, runBin(t, w, nil, "write", stem, "--parent", "/", "--description", desc))
		rel := filepath.Join("notes", stem, stem+".md")
		want, err := os.ReadFile(filepath.Join(parent, "memory", rel)) //nolint:gosec // a file this test created
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(shipped, rel)) //nolint:gosec // the repo's own file
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("agent/memory/%s\n is %q\nwant %q (what write creates)", filepath.ToSlash(rel), got, want)
		}
	}

	st := memory.Load(filepath.Join(shipped, "notes"))
	check(t, "problems loading the starting tree", st.Problems(), []memory.Problem(nil))
	got := make([]string, 0, len(starterStems))
	for _, n := range st.FirstLevel() {
		got = append(got, n.Stem)
		check(t, n.Stem+" description", n.Description, seedDescriptions[n.Stem])
		check(t, n.Stem+" blocks", len(n.Blocks), 0)
	}
	sort.Strings(got)
	check(t, "first-level stems", got, starterStems)
	check(t, "nodes", len(st.Nodes()), len(starterStems))
}

// runCommandsIn runs blocks as one bash script in dir, with <state dir>
// replaced by state and the test binary first on the PATH.
func runCommandsIn(t *testing.T, dir, state string, blocks []string) {
	t.Helper()
	if len(blocks) == 0 {
		t.Fatal("no bash commands to run")
	}
	script := "set -euo pipefail\n" + strings.ReplaceAll(strings.Join(blocks, "\n"), "<state dir>", state)
	cmd := exec.Command("bash", "-c", script) //nolint:gosec // the repo's own README commands
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(binPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running the commands: %v\n%s\n%s", err, script, out)
	}
}

// Acceptance criterion 4: the install commands of agent/README.md and of
// README.md's step 3 never copy the starting tree into an existing store:
// a state dir whose memory/notes/ holds own/own.md keeps memory/ exactly,
// while the other files still install.
func TestInstallNeverOverwritesAStore(t *testing.T) {
	root := repoRoot(t)
	agentReadme, err := os.ReadFile(filepath.Join(root, "agent", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, steps := splitSteps(readmeSection(t, string(readme), "Wiring it to an agent"))
	if len(steps) < 3 {
		t.Fatalf("the wiring guide has %d steps", len(steps))
	}
	for name, run := range map[string]struct {
		dir    string
		blocks []string
	}{
		"agent/README.md":  {filepath.Join(root, "agent"), bashBlocks(string(agentReadme))},
		"README.md step 3": {root, bashBlocks(steps[2])},
	} {
		for _, own := range []bool{true, false} {
			state := filepath.Join(t.TempDir(), "state")
			mem := filepath.Join(state, "memory")
			if own {
				writeNode(t, filepath.Join(mem, "notes"), "own", "---\ndescription: Mine\n---\n")
			} else {
				// An empty memory/ is a store too: still not copied into.
				mkdirs(t, mem)
			}
			ageTree(t, state)
			before := snapshot(t, mem)
			runCommandsIn(t, run.dir, state, run.blocks)
			sameSnapshot(t, before, snapshot(t, mem))
			for _, f := range []string{"tools/memory_write.json", "prompt/memory_purpose.md", "livecontext/memory_index.json"} {
				if !exists(filepath.Join(state, filepath.FromSlash(f))) {
					t.Errorf("%s: %s was not installed beside an existing store", name, f)
				}
			}
		}
	}
}

// joinedQuote is the "> " blockquote that starts on the first line after
// marker in text, its lines joined by single spaces.
func joinedQuote(t *testing.T, text, marker string) string {
	t.Helper()
	at := strings.Index(text, marker)
	if at < 0 {
		t.Fatalf("no %q in the text", marker)
	}
	lines := strings.Split(text[at:], "\n")[1:]
	var parts []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, ">") {
			break
		}
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(l, ">")))
	}
	if len(parts) == 0 {
		t.Fatalf("no blockquote after %q", marker)
	}
	return strings.Join(parts, " ")
}

// replacement is a III.6 bullet's `"old" becomes "new"` pair.
var replacement = regexp.MustCompile(`"([^"]+)" becomes "([^"]+)"`)

// typologyWord matches the starting tree's stems as words.
var typologyWord = regexp.MustCompile(`\b(user|projects|people|topics)\b`)

// liveContextDescription is agent/livecontext/memory_index.json's
// description.
func liveContextDescription(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "livecontext", "memory_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entry struct {
		Description string `json:"description"`
	}
	decodeJSON(t, data, &entry)
	return entry.Description
}

// shippedDescriptions maps each shipped file the model reads a description
// from (the four tool files and the live-context entry) to it.
func shippedDescriptions(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "agent", "tools")
	descriptions := map[string]string{"livecontext/memory_index.json": liveContextDescription(t)}
	for file := range shippedTools {
		descriptions["tools/"+file] = loadTool(t, filepath.Join(dir, file)).Description
	}
	return descriptions
}

// Acceptance criterion 5: the write tool's description is III.6's text
// exactly, and the read, index and live-context descriptions carry III.6's
// replacement sentences (both read from the spec, not restated here).
func TestShippedDescriptionsAreTheSpecText(t *testing.T) {
	section := specSection(t, noTypologyHeading)
	descriptions := shippedDescriptions(t)
	check(t, "memory_write.json description", descriptions["tools/memory_write.json"],
		joinedQuote(t, section, "- `memory_write.json`:"))

	flat := strings.Join(strings.Fields(section), " ")
	for marker, file := range map[string]string{
		"- `memory_read.json`:":              "tools/memory_read.json",
		"- `memory_index.json`:":             "tools/memory_index.json",
		"- `livecontext/memory_index.json`:": "livecontext/memory_index.json",
	} {
		at := strings.Index(flat, marker)
		if at < 0 {
			t.Fatalf("III.6 has no bullet %q", marker)
		}
		m := replacement.FindStringSubmatch(flat[at:])
		if m == nil {
			t.Fatalf("III.6's bullet %q has no replacement", marker)
		}
		if strings.Contains(descriptions[file], m[1]) {
			t.Errorf("%s still says %q", file, m[1])
		}
		if !strings.Contains(descriptions[file], m[2]) {
			t.Errorf("%s does not say %q", file, m[2])
		}
	}
}

// exampleDescription is the description quoted in the example of III.6's
// pinned write text. Its "people" is the English word, not a stem, so that
// quoted value alone is left out of the word check (see the CHANGELOG).
const exampleDescription = `"How and when to cc people on outbox replies"`

// Acceptance criterion 5: none of the descriptions says "area" or names a
// starting-tree stem, and the write tool's says --parent / starts a
// top-level node.
func TestShippedDescriptionsPresumeNoTypology(t *testing.T) {
	descriptions := shippedDescriptions(t)
	if !strings.Contains(descriptions["tools/memory_write.json"], exampleDescription) {
		t.Fatalf("the write description no longer quotes %s; drop the exemption", exampleDescription)
	}
	for file, d := range descriptions {
		if strings.Contains(strings.ToLower(d), "area") {
			t.Errorf("%s says area: %q", file, d)
		}
		if w := typologyWord.FindString(strings.Replace(d, exampleDescription, "", 1)); w != "" {
			t.Errorf("%s names %q: %q", file, w, d)
		}
	}
	if !strings.Contains(descriptions["tools/memory_write.json"], "--parent /") {
		t.Error("memory_write.json does not say --parent / starts a top-level node")
	}
}

// Acceptance criterion 5: the guidance is §4.5's text and says neither
// "area" nor user, projects or topics; the READMEs file nothing under
// topics, and README.md's Usage shows --parent /.
func TestGuidanceAndReadmesPresumeNoTypology(t *testing.T) {
	guidance, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "prompt", "memory_purpose.md"))
	if err != nil {
		t.Fatal(err)
	}
	check(t, "guidance", string(guidance), string(specGuidance(t)))
	if strings.Contains(strings.ToLower(string(guidance)), "area") {
		t.Error("the guidance says area")
	}
	if w := regexp.MustCompile(`\b(user|projects|topics)\b`).FindString(string(guidance)); w != "" {
		t.Errorf("the guidance names %q", w)
	}

	readme, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	usage := readmeSection(t, string(readme), "Usage")
	if !strings.Contains(usage, "--parent /") {
		t.Error("README.md's Usage section does not show --parent /")
	}
	if strings.Contains(usage, "topics") {
		t.Error("README.md's Usage section mentions topics")
	}
	agentReadme, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"README.md": string(readme), "agent/README.md": string(agentReadme)} {
		if strings.Contains(text, "--parent topics") {
			t.Errorf("%s files under topics", name)
		}
	}
}

// Acceptance criterion 1: the usage text says the root is / and only write
// creates the store; it no longer says any verb creates first-level nodes.
func TestUsageHasNoSeeding(t *testing.T) {
	r := runBin(t, t.TempDir(), nil)
	for _, s := range []string{"--parent /", "write"} {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("usage does not say %q:\n%s", s, r.stderr)
		}
	}
	if strings.Contains(r.stderr, "first-level nodes when") || strings.Contains(r.stderr, "Any verb creates") {
		t.Errorf("usage still says verbs seed the store:\n%s", r.stderr)
	}
}

// Acceptance criterion 6: every verb's output on the shipped starting tree
// is what it is on the tree CLI6 seeded, which is what four write --parent /
// calls make: same files, same mtimes, same output.
func TestStartingTreeBehavesAsSeeded(t *testing.T) {
	shippedW, shippedNotes := seeded(t)
	writtenW, writtenParent := freshParent(t)
	for _, stem := range starterStems {
		mustSucceed(t, runBin(t, writtenW, nil, "write", stem, "--parent", "/", "--description", seedDescriptions[stem]))
	}
	writtenNotes := filepath.Join(writtenParent, "memory", "notes")
	mt := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i, stem := range starterStems {
		rel := filepath.Join(stem, stem+".md")
		at := mt.Add(time.Duration(i) * 24 * time.Hour)
		setMtime(t, filepath.Join(shippedNotes, rel), at)
		setMtime(t, filepath.Join(writtenNotes, rel), at)
	}
	for _, args := range [][]string{
		{"read"},
		{"read", "/"},
		{"read", "--depth", "2"},
		{"read", "user"},
		{"read", "topics", "--depth", "0"},
		{"read", "topic"},
		{"index"},
		{"search", "who"},
		{"search", "path:p"},
		{"search", "[description:else]"},
	} {
		a := runBin(t, shippedW, nil, args...)
		b := runBin(t, writtenW, nil, args...)
		check(t, strings.Join(args, " "), a, b)
	}
}

// decodeJSON decodes data into v, failing the test on error.
func decodeJSON(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}
