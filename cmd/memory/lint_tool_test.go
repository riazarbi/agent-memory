package main

// CLI9: lint --exit-zero keeps the report and drops the verdict, and
// agent/tools/memory_lint.json lets the agent lint its own store
// (docs/spec/agent-memory.md Part III.7).

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lintTool is the shipped lint tool file.
const lintTool = "memory_lint.json"

// brokenStore is a seeded store with exactly one error (a folder note with
// no frontmatter) and one warning (a stray file in a node folder).
func brokenStore(t *testing.T) (w, notes string) {
	t.Helper()
	w, notes = seeded(t)
	writeNode(t, notes, "b", "no frontmatter here\n")
	writeFile(t, filepath.Join(notes, "topics", "notes.txt"), "stray\n")
	return w, notes
}

// Acceptance criterion 1: on a store with an error, lint exits 2 and lint
// --exit-zero exits 0 with nothing on stderr and the same stdout, byte for
// byte; on a clean store, and a missing one, both exit 0 alike.
func TestLintExitZeroKeepsTheReport(t *testing.T) {
	w, notes := brokenStore(t)
	plain := runBin(t, w, nil, "lint")
	if plain.code != wantLintExit || plain.stderr != "" {
		t.Fatalf("lint: exit %d, stderr %q, want %d and empty", plain.code, plain.stderr, wantLintExit)
	}
	var report xLint
	decode(t, plain.stdout, &report)
	check(t, "fixture counts", [3]string{report.Nodes, report.Errors, report.Warnings}, [3]string{"4", "1", "1"})
	sameReport(t, report, notes)

	zero := runBin(t, w, nil, "lint", "--exit-zero")
	mustSucceed(t, zero)
	check(t, "lint --exit-zero stdout", zero.stdout, plain.stdout)

	// The single-dash form is the same flag, as for every other verb.
	single := runBin(t, w, nil, "lint", "-exit-zero")
	mustSucceed(t, single)
	check(t, "lint -exit-zero stdout", single.stdout, plain.stdout)

	// A lone -- after the flag ends the flags; the flag took no value.
	ended := runBin(t, w, nil, "lint", "--exit-zero", "--")
	mustSucceed(t, ended)
	check(t, "lint --exit-zero -- stdout", ended.stdout, plain.stdout)

	w, _ = seeded(t)
	clean := runBin(t, w, nil, "lint")
	mustSucceed(t, clean)
	cleanZero := runBin(t, w, nil, "lint", "--exit-zero")
	mustSucceed(t, cleanZero)
	check(t, "clean store stdout", cleanZero.stdout, clean.stdout)
	check(t, "clean store report", clean.stdout, "<memory_lint nodes=\"4\" errors=\"0\" warnings=\"0\"></memory_lint>\n")

	w, parent := freshParent(t)
	missing := runBin(t, w, nil, "lint", "--exit-zero")
	mustSucceed(t, missing)
	check(t, "missing store report", missing.stdout, "<memory_lint nodes=\"0\" errors=\"0\" warnings=\"0\"></memory_lint>\n")
	isEmptyDir(t, "lint --exit-zero on a missing store", parent)
}

// Acceptance criterion 1: --exit-zero never turns a failure to run into
// exit 0: an extra argument, a value given to the flag, an unknown flag, a
// missing parent, or memory/ not a directory is exit 1 with one line on
// stderr, nothing on stdout, and nothing created.
func TestLintExitZeroStillFailsToRun(t *testing.T) {
	w, _ := brokenStore(t)
	for _, args := range [][]string{
		{"lint", "--exit-zero", "extra"},
		{"lint", "extra", "--exit-zero"},
	} {
		line := mustFailCleanly(t, w, nil, args...)
		if !strings.Contains(line, `"extra"`) || strings.Contains(line, "invalid value") {
			t.Errorf("%v: stderr %q does not refuse the argument extra", args, line)
		}
	}
	for _, args := range [][]string{
		{"lint", "--exit-zero=true"},
		{"lint", "--exit-zero=false"},
		{"lint", "--exit-zero", "--strict"},
		{"lint", "--exit-zero", "--", "extra"},
	} {
		mustFailCleanly(t, w, nil, args...)
	}
	// A resolution error: a variable set but empty (III.1).
	line := mustFailCleanly(t, w, map[string]string{"MEMORY_PARENT_DIR": ""}, "lint", "--exit-zero")
	check(t, "empty MEMORY_PARENT_DIR", line, "MEMORY_PARENT_DIR is set but empty")

	w, parent := freshParent(t)
	nope := filepath.Join(w, "nope")
	failsExactly(t, w, nil, []string{"--parent-dir", nope, "lint", "--exit-zero"},
		"parent dir "+nope+" does not exist (from --parent-dir)")
	isEmptyDir(t, "lint --exit-zero with a missing parent", parent)

	w, parent = freshParent(t)
	mem := filepath.Join(parent, "memory")
	writeFile(t, mem, "a file")
	failsExactly(t, w, nil, []string{"lint", "--exit-zero"}, mem+" is not a directory")
}

// Acceptance criterion 1: lint's flag set holds exactly one flag,
// --exit-zero, a boolean that defaults to off.
func TestLintFlags(t *testing.T) {
	var exitZero bool
	fs := lintFlags(&exitZero)
	var names []string
	fs.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	check(t, "lint flags", names, []string{"exit-zero"})
	pos, _, err := parseArgs(fs, []string{"--exit-zero"})
	if err != nil || len(pos) != 0 || !exitZero {
		t.Errorf("parsing --exit-zero: positional %q, err %v, exitZero %v", pos, err, exitZero)
	}
	exitZero = false
	if _, _, err := parseArgs(lintFlags(&exitZero), nil); err != nil || exitZero {
		t.Errorf("no arguments: err %v, exitZero %v; want nil, false", err, exitZero)
	}
}

const lintHeading = "## III.7 `lint`"

// Acceptance criterion 2: memory_lint.json is exactly III.7's tool file,
// its description read from the spec, never restated here.
func TestLintToolFileIsTheSpecs(t *testing.T) {
	tool := loadTool(t, filepath.Join(repoRoot(t), "agent", "tools", lintTool))
	check(t, "command", tool.Command, "memory")
	check(t, "prepend_args", tool.PrependArgs, []string{"--parent-dir", "${AGENT_STATE_DIR}", "lint", "--exit-zero"})
	if tool.ReadOnly == nil || !*tool.ReadOnly {
		t.Error("read_only is not true")
	}
	if tool.AllowUserArgs == nil || *tool.AllowUserArgs {
		t.Error("allow_user_args is not false")
	}
	if tool.ForbiddenArgs == nil || len(tool.ForbiddenArgs) != 0 {
		t.Errorf("forbidden_args is %#v, want []", tool.ForbiddenArgs)
	}

	want := joinedQuote(t, specSection(t, lintHeading), "and this description:")
	if !strings.HasPrefix(want, "Check your durable memory") {
		t.Fatalf("the quote found in III.7 is not the tool description: %q", want)
	}
	check(t, "description", tool.Description, want)
	if strings.Contains(tool.Description, "75") {
		t.Error("the description mentions 75; lint cannot be busy")
	}
	if m := flagMention.FindAllString(tool.Description, -1); len(m) > 0 {
		t.Errorf("the description names flags %v", m)
	}
}

// callLintTool runs memory_lint.json as the agent runs a shell tool:
// prepend_args with ${AGENT_STATE_DIR} replaced by <w>/.agent and no model
// arguments (the file allows none), with MEMORY_PARENT_DIR pointing at an
// existing, empty decoy. It returns the result and the decoy.
func callLintTool(t *testing.T, w string) (result, string) {
	t.Helper()
	tool := loadTool(t, filepath.Join(repoRoot(t), "agent", "tools", lintTool))
	argv := make([]string, 0, len(tool.PrependArgs))
	for _, a := range tool.PrependArgs {
		argv = append(argv, strings.ReplaceAll(a, "${AGENT_STATE_DIR}", filepath.Join(w, ".agent")))
	}
	decoy := filepath.Join(w, "decoy")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatal(err)
	}
	return runBin(t, w, map[string]string{"MEMORY_PARENT_DIR": decoy}, argv...), decoy
}

// Acceptance criterion 3: through the tool harness a store with an error
// exits 0 with the full report naming the broken node; the clean starting
// tree reports errors="0" warnings="0". The decoy parent stays empty.
func TestLintToolShowsTheReport(t *testing.T) {
	w, notes := brokenStore(t)
	r, decoy := callLintTool(t, w)
	mustSucceed(t, r)
	var got xLint
	decode(t, r.stdout, &got)
	check(t, "result element", got.XMLName.Local, "memory_lint")
	check(t, "counts", [3]string{got.Nodes, got.Errors, got.Warnings}, [3]string{"4", "1", "1"})
	sameReport(t, got, notes)
	named := false
	for _, p := range got.Problems {
		if p.Severity == "error" && p.Path == "b" {
			named = true
		}
	}
	if !named {
		t.Errorf("the report %+v does not name the broken node b", got.Problems)
	}
	isEmptyDir(t, "the decoy parent", decoy)

	w, notes = seeded(t)
	r, decoy = callLintTool(t, w)
	mustSucceed(t, r)
	got = xLint{}
	decode(t, r.stdout, &got)
	if got.Problems == nil {
		got.Problems = []xProblem{}
	}
	check(t, "clean counts", [3]string{got.Nodes, got.Errors, got.Warnings}, [3]string{"4", "0", "0"})
	sameReport(t, got, notes)
	if !strings.Contains(r.stdout, `errors="0" warnings="0"`) {
		t.Errorf("the clean report does not say errors=\"0\" warnings=\"0\": %s", r.stdout)
	}
	isEmptyDir(t, "the decoy parent", decoy)
}

// Acceptance criterion 4: the usage text gives lint's --exit-zero.
func TestUsageGivesExitZero(t *testing.T) {
	r := runBin(t, t.TempDir(), nil)
	for _, l := range strings.Split(r.stderr, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "lint ") {
			if !strings.Contains(l, "[--exit-zero]") {
				t.Errorf("the usage's lint line does not give [--exit-zero]: %q", l)
			}
			if !strings.Contains(r.stderr, "--exit-zero:") && !strings.Contains(r.stderr, "with --exit-zero") {
				t.Errorf("the usage does not say what --exit-zero does:\n%s", r.stderr)
			}
			return
		}
	}
	t.Errorf("the usage has no lint line:\n%s", r.stderr)
}

// Acceptance criterion 4: README.md documents --exit-zero beside lint and
// no longer says no tool file ships for lint.
func TestReadmeDocumentsExitZero(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	usage := readmeSection(t, string(data), "Usage")
	for _, want := range []string{"memory lint --exit-zero", "memory_lint.json"} {
		if !strings.Contains(usage, want) {
			t.Errorf("README.md's Usage section does not mention %q", want)
		}
	}
	if strings.Contains(strings.Join(strings.Fields(string(data)), " "), "no tool file ships") {
		t.Error("README.md still says no tool file ships for lint")
	}
}

// Acceptance criterion 4: agent/README.md's table lists
// tools/memory_lint.json, going to <state dir>/tools/, as optional.
func TestAgentReadmeListsLintTool(t *testing.T) {
	agent, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	var row string
	for _, l := range strings.Split(string(agent), "\n") {
		if strings.HasPrefix(l, "| `tools/memory_lint.json` |") {
			row = l
		}
	}
	if row == "" {
		t.Fatal("agent/README.md's table has no row for tools/memory_lint.json")
	}
	for _, want := range []string{"`<state dir>/tools/`", "optional", "memory lint --exit-zero"} {
		if !strings.Contains(strings.ToLower(row), strings.ToLower(want)) {
			t.Errorf("agent/README.md's memory_lint.json row does not say %q: %s", want, row)
		}
	}
}
