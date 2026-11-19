package main

// CLI8: memory lint reports every problem the reader finds, and exits 0, 2
// or 1 (docs/spec/agent-memory.md Part III.7).

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// wantLintExit is the exit code lint gives when the store has errors,
// declared here so a change to the production constant fails a test.
const wantLintExit = 2

// xLint is lint's result as a reader sees it, declared independently of
// the production types.
type xLint struct {
	XMLName  xml.Name
	Nodes    string     `xml:"nodes,attr"`
	Errors   string     `xml:"errors,attr"`
	Warnings string     `xml:"warnings,attr"`
	Problems []xProblem `xml:"problem"`
}

type xProblem struct {
	Severity string `xml:"severity,attr"`
	Path     string `xml:"path,attr"`
	Reason   string `xml:",chardata"`
}

// libraryLint is what lint must report for notes, from the library itself:
// the loaded node count, the error and warning counts, and every problem in
// the library's order.
func libraryLint(notes string) xLint {
	st := memory.Load(notes)
	var errs, warns int
	all := st.Problems()
	problems := make([]xProblem, 0, len(all))
	for _, p := range all {
		switch p.Severity {
		case memory.SeverityError:
			errs++
		case memory.SeverityWarning:
			warns++
		}
		problems = append(problems, xProblem{Severity: string(p.Severity), Path: p.Path, Reason: p.Reason})
	}
	return xLint{
		Nodes:    strconv.Itoa(len(st.Nodes())),
		Errors:   strconv.Itoa(errs),
		Warnings: strconv.Itoa(warns),
		Problems: problems,
	}
}

// runLint runs lint in w, checks it exited code with stderr empty and XML
// on stdout, and returns the decoded report and the raw stdout.
func runLint(t *testing.T, w string, code int) (xLint, string) {
	t.Helper()
	r := runBin(t, w, nil, "lint")
	if r.code != code || r.stderr != "" {
		t.Fatalf("lint: exit %d, stderr %q, want exit %d and empty stderr (stdout %q)", r.code, r.stderr, code, r.stdout)
	}
	var got xLint
	decode(t, r.stdout, &got)
	check(t, "result element", got.XMLName.Local, "memory_lint")
	if got.Problems == nil {
		got.Problems = []xProblem{}
	}
	return got, r.stdout
}

// sameReport checks got against the library's report for notes: counts and
// every problem, in order, exactly.
func sameReport(t *testing.T, got xLint, notes string) xLint {
	t.Helper()
	want := libraryLint(notes)
	check(t, "counts", [3]string{got.Nodes, got.Errors, got.Warnings}, [3]string{want.Nodes, want.Errors, want.Warnings})
	check(t, "problems", got.Problems, want.Problems)
	return want
}

// Acceptance criterion 1: the shipped starting tree is clean: exit 0, the
// childless element with nodes="4". A missing store is empty, not an
// error, and lint does not create it.
func TestLintCleanStore(t *testing.T) {
	w, notes := seeded(t)
	got, out := runLint(t, w, 0)
	check(t, "stdout", out, "<memory_lint nodes=\"4\" errors=\"0\" warnings=\"0\"></memory_lint>\n")
	check(t, "problems", got.Problems, []xProblem{})
	sameReport(t, got, notes)

	w, parent := freshParent(t)
	got, _ = runLint(t, w, 0)
	check(t, "empty store counts", [3]string{got.Nodes, got.Errors, got.Warnings}, [3]string{"0", "0", "0"})
	check(t, "empty store problems", got.Problems, []xProblem{})
	isEmptyDir(t, "lint on a missing store", parent)
}

// Acceptance criterion 2: errors exit 2 with the full report on stdout and
// nothing on stderr. The child of a broken node is in its skipped subtree,
// so it is neither counted nor reported.
func TestLintErrorsExit2(t *testing.T) {
	w, notes := seeded(t)
	writeNode(t, notes, "b", "no frontmatter here\n")
	writeNode(t, notes, "topics/c", "---\ndescription: C\n---\n\n# heading\n")
	writeNode(t, notes, "topics/c/d", "---\ndescription: D\n---\n\nA valid child.\n")
	got, _ := runLint(t, w, wantLintExit)
	check(t, "counts", [3]string{got.Nodes, got.Errors, got.Warnings}, [3]string{"4", "2", "0"})
	want := sameReport(t, got, notes)
	paths := make([]string, 0, len(want.Problems))
	for _, p := range want.Problems {
		paths = append(paths, p.Severity+" "+p.Path)
	}
	check(t, "problem paths", paths, []string{"error b", "error topics/c"})
}

// Acceptance criterion 3: warnings alone pass, and each is listed.
func TestLintWarningsPass(t *testing.T) {
	w, notes := seeded(t)
	writeNode(t, notes, "topics/k", "---\ndescription: K\n---\n\nSee [[nowhere]] for more. ^m-aaaaaa\n")
	writeNode(t, notes, "user/j", "---\ndescription: J\n---\n\nThe same ID again. ^m-aaaaaa\n")
	writeFile(t, filepath.Join(notes, "topics", "notes.txt"), "stray\n")
	got, _ := runLint(t, w, 0)
	want := sameReport(t, got, notes)
	check(t, "errors", got.Errors, "0")
	n, err := strconv.Atoi(got.Warnings)
	if err != nil || n < 3 {
		t.Fatalf("warnings=%q, want at least 3", got.Warnings)
	}
	if len(got.Problems) != n {
		t.Errorf("%d problems listed, want all %d warnings", len(got.Problems), n)
	}
	for _, p := range got.Problems {
		if p.Severity != "warning" {
			t.Errorf("problem %+v is not a warning", p)
		}
	}
	check(t, "nodes", got.Nodes, want.Nodes)
	check(t, "nodes loaded", got.Nodes, "6")
}

// Acceptance criterion 4: the report is real XML: a reason holding <, >
// and & is escaped and decodes back exactly; problems keep the library's
// path order, never grouped by severity.
func TestLintReportIsXML(t *testing.T) {
	w, notes := seeded(t)
	writeNode(t, notes, "a-bad", "---\ndescription: A\n---\n\n# a<b & c>\n")
	writeFile(t, filepath.Join(notes, "people", "x.txt"), "stray\n")
	writeNode(t, notes, "user/zz", "---\ndescription: [not, one, line]\n---\n")
	got, out := runLint(t, w, wantLintExit)
	want := sameReport(t, got, notes)
	sevs := make([]string, 0, len(got.Problems))
	for _, p := range got.Problems {
		sevs = append(sevs, p.Severity+" "+p.Path)
	}
	check(t, "order", sevs, []string{"error a-bad", "warning people/x.txt", "error user/zz"})
	if !strings.Contains(want.Problems[0].Reason, "# a<b & c>") {
		t.Fatalf("library reason %q does not quote the line; the fixture is wrong", want.Problems[0].Reason)
	}
	if !strings.Contains(out, "# a&lt;b &amp; c&gt;") {
		t.Errorf("stdout does not escape the reason:\n%s", out)
	}
}

// Acceptance criterion 4: a problem about notes/ itself has path ".".
// notes/ that cannot be listed is such a problem, an error.
func TestLintNotesItselfIsDot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 directory")
	}
	w, notes := seeded(t)
	if err := os.Chmod(notes, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(notes, 0o755) }) //nolint:gosec // a test temp dir
	got, _ := runLint(t, w, wantLintExit)
	sameReport(t, got, notes)
	if len(got.Problems) != 1 || got.Problems[0].Path != "." || got.Problems[0].Severity != "error" {
		t.Errorf("problems %+v, want one error at .", got.Problems)
	}
	check(t, "nodes", got.Nodes, "0")
}

// Acceptance criterion 5: when lint cannot run it exits 1 with one line on
// stderr and nothing on stdout, never a report, and creates nothing.
func TestLintCannotRun(t *testing.T) {
	w, _ := seeded(t)
	line := mustFailCleanly(t, w, nil, "lint", "extra")
	if !strings.Contains(line, `"extra"`) || !strings.Contains(line, "lint") {
		t.Errorf("stderr %q does not refuse lint's argument", line)
	}
	mustFailCleanly(t, w, nil, "lint", "--strict")
	mustFailCleanly(t, w, nil, "lint", "--fix")

	w, parent := freshParent(t)
	nope := filepath.Join(w, "nope")
	failsExactly(t, w, nil, []string{"--parent-dir", nope, "lint"},
		"parent dir "+nope+" does not exist (from --parent-dir)")
	isEmptyDir(t, "lint with a missing parent", parent)

	w, parent = freshParent(t)
	mem := filepath.Join(parent, "memory")
	writeFile(t, mem, "a file")
	failsExactly(t, w, nil, []string{"lint"}, mem+" is not a directory")

	w, parent = freshParent(t)
	notes := filepath.Join(parent, "memory", "notes")
	writeFile(t, notes, "a file")
	failsExactly(t, w, nil, []string{"lint"}, notes+" is not a directory")
}

// mustFailCleanly runs args in w and checks exit 1, one line on stderr,
// empty stdout, and that nothing under w changed. It returns the line.
func mustFailCleanly(t *testing.T, w string, env map[string]string, args ...string) string {
	t.Helper()
	before := snapshot(t, w)
	line := mustFail(t, runBin(t, w, env, args...))
	sameSnapshot(t, before, snapshot(t, w))
	return line
}

// Acceptance criterion 6: lint writes nothing: on a store with errors and
// warnings, the whole state dir keeps its bytes, mtimes and modes, and no
// lock file appears.
func TestLintWritesNothing(t *testing.T) {
	w, notes := seeded(t)
	writeNode(t, notes, "b", "no frontmatter here\n")
	writeNode(t, notes, "topics/k", "---\ndescription: K\n---\n\nSee [[nowhere]].\n")
	writeFile(t, filepath.Join(notes, "topics", "notes.txt"), "stray\n")
	state := filepath.Join(w, ".agent")
	ageTree(t, state)
	before := snapshot(t, state)
	got, _ := runLint(t, w, wantLintExit)
	if got.Errors == "0" || got.Warnings == "0" {
		t.Fatalf("fixture has errors=%s warnings=%s, want both", got.Errors, got.Warnings)
	}
	sameSnapshot(t, before, snapshot(t, state))
	if exists(filepath.Join(notes, ".write.lock")) {
		t.Error("lint created the write lock file")
	}
}

// Acceptance criterion 6: the usage text lists lint.
func TestUsageListsLint(t *testing.T) {
	r := runBin(t, t.TempDir(), nil)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	found := false
	for _, l := range strings.Split(r.stderr, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "lint ") {
			found = true
		}
	}
	if !found {
		t.Errorf("usage does not list lint:\n%s", r.stderr)
	}
}

// The README's Usage section documents lint: the command, what it reports,
// and its three exit codes; the status line lists it among the verbs.
func TestReadmeDocumentsLint(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	usage := readmeSection(t, readme, "Usage")
	for _, want := range []string{"memory lint", "<memory_lint", "exit 0", "exit 2", "exit 1", "|| "} {
		if !strings.Contains(usage, want) {
			t.Errorf("the Usage section does not mention %q", want)
		}
	}
	status, _, _ := strings.Cut(readme[strings.Index(readme, "**Status:**"):], "\n")
	if !strings.Contains(status, "`lint`") {
		t.Errorf("the status line does not list lint: %s", status)
	}
}
