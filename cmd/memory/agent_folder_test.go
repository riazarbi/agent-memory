package main

// CLI5: everything an operator copies into an agent ships in agent/, laid
// out like the agent's state dir; init seeds notes/ only; release archives
// carry agent/ beside the binary (docs/spec/agent-memory.md Part III.4).

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// agentFiles is the agent/ folder's contents, relative to it (III.4).
var agentFiles = []string{
	"README.md",
	"livecontext/memory_index.json",
	"memory/notes/people/people.md",
	"memory/notes/projects/projects.md",
	"memory/notes/topics/topics.md",
	"memory/notes/user/user.md",
	"prompt/memory_usage.md",
	"tools/memory_index.json",
	"tools/memory_read.json",
	"tools/memory_search.json",
	"tools/memory_write.json",
}

// treeUnder lists every entry under root, slash-separated and relative to
// it, directories with a trailing slash, sorted. Anything that is neither a
// directory nor a regular file fails the test.
func treeUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir():
			out = append(out, rel+"/")
		case d.Type().IsRegular():
			out = append(out, rel)
		default:
			t.Errorf("%s is neither a directory nor a regular file (%v)", rel, d.Type())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// withDirs adds every ancestor directory of files (as "dir/") to the list,
// sorted, prefixed by prefix.
func withDirs(prefix string, files []string) []string {
	set := map[string]bool{}
	for _, f := range files {
		f = prefix + f
		set[f] = true
		for d := path.Dir(f); d != "." && d != "/"; d = path.Dir(d) {
			set[d+"/"] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// guidanceMarker introduces §4.5's fenced guidance text in the spec.
const guidanceMarker = "shipped verbatim as `agent/prompt/memory_usage.md`"

// specGuidance returns the content of the first fenced block after
// guidanceMarker: the lines between its fences, each with its newline.
func specGuidance(t *testing.T) []byte {
	t.Helper()
	spec, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "spec", "agent-memory.md"))
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.Index(spec, []byte(guidanceMarker))
	if at < 0 {
		t.Fatalf("the spec has no %q", guidanceMarker)
	}
	lines := strings.SplitAfter(string(spec[at:]), "\n")
	open := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "```") {
			open = i
			break
		}
	}
	if open < 0 {
		t.Fatal("no fenced block after the marker")
	}
	var b strings.Builder
	for _, l := range lines[open+1:] {
		if strings.TrimRight(l, "\n") == "```" {
			return []byte(b.String())
		}
		b.WriteString(l)
	}
	t.Fatal("the guidance block is not closed")
	return nil
}

// Acceptance criterion 2: agent/prompt/memory_usage.md is byte-equal to the
// spec's §4.5 text, which no longer places the index in the system prompt.
func TestShippedGuidanceIsTheSpecText(t *testing.T) {
	want := specGuidance(t)
	if !bytes.HasPrefix(want, []byte("# Memory\n")) || !bytes.Contains(want, []byte("## Before you answer\n")) {
		t.Fatalf("the spec block found is not the guidance:\n%s", want)
	}
	got, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "prompt", "memory_usage.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("agent/prompt/memory_usage.md differs from the spec text:\n--- got\n%s\n--- want\n%s", got, want)
	}
	if strings.Contains(strings.Join(strings.Fields(string(got)), " "), "system prompt") {
		t.Error("the guidance still says the index is in the system prompt")
	}
}

// Acceptance criterion 3: agent/ holds exactly the shipped files, and no
// copy of the tool files is left at the top of the repo.
func TestAgentFolderIsComplete(t *testing.T) {
	root := repoRoot(t)
	check(t, "agent/", treeUnder(t, filepath.Join(root, "agent")), withDirs("", agentFiles))
	if exists(filepath.Join(root, "tools")) {
		t.Error("a top-level tools/ is still there")
	}
}

// Acceptance criterion 4: agent/README.md names every shipped file and the
// state-dir folder each goes to, and states the rules an operator needs.
func TestAgentReadmeNamesEveryFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	want := []string{
		// the five shipped paths
		"tools/memory_read.json", "tools/memory_write.json", "tools/memory_search.json",
		"tools/memory_index.json", "prompt/memory_usage.md", "livecontext/memory_index.json",
		// the three destinations
		"<state dir>/tools/", "<state dir>/prompt/", "<state dir>/livecontext/",
		// substitution, and what an older agent shows (Part III.1)
		"${AGENT_STATE_DIR}", "parent dir ${AGENT_STATE_DIR} does not exist (from --parent-dir)",
	}
	for _, s := range want {
		if !strings.Contains(readme, s) {
			t.Errorf("agent/README.md does not mention %q", s)
		}
	}
}

// bashBlocks returns the bodies of the ```bash fenced blocks in md.
func bashBlocks(md string) []string {
	var out []string
	for _, part := range strings.Split(md, "```bash\n")[1:] {
		body, _, ok := strings.Cut(part, "```")
		if ok {
			out = append(out, body)
		}
	}
	return out
}

// runCommands runs blocks as one bash script in dir, with <state dir>
// replaced by a fresh state dir and the test binary first on the PATH, and
// returns that state dir.
func runCommands(t *testing.T, dir string, blocks []string) string {
	t.Helper()
	if len(blocks) == 0 {
		t.Fatal("no bash commands to run")
	}
	state := filepath.Join(t.TempDir(), "state")
	script := "set -euo pipefail\n" + strings.ReplaceAll(strings.Join(blocks, "\n"), "<state dir>", state)
	cmd := exec.Command("bash", "-c", script) //nolint:gosec // the repo's own README commands
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(binPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running the commands: %v\n%s\n%s", err, script, out)
	}
	return state
}

// checkInstalled fails unless every shipped file but agent/README.md is in
// state at its path under agent/, byte-equal, and the README is not.
func checkInstalled(t *testing.T, state string) {
	t.Helper()
	for _, rel := range agentFiles {
		if rel == "README.md" {
			continue
		}
		want, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", filepath.FromSlash(rel))) //nolint:gosec // the repo's own file
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(state, filepath.FromSlash(rel))) //nolint:gosec // a file this test created
		if err != nil {
			t.Errorf("%s was not installed: %v", rel, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s was installed changed", rel)
		}
	}
	if exists(filepath.Join(state, "README.md")) {
		t.Error("agent/README.md was copied into the state dir")
	}
}

// Acceptance criterion 4: the commands agent/README.md gives, run from agent/
// with <state dir> filled in and memory on the PATH, put
// every shipped file, unchanged, where III.4 says.
func TestAgentReadmeCommandsInstall(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Installing is a copy; the store is created by the first call (III.1).
	checkInstalled(t, runCommands(t, filepath.Join(repoRoot(t), "agent"), bashBlocks(string(data))))
}

// Step 1 of the README says what an archive holds, step 3 gives the copy
// commands from agent/, and nothing still sends the operator to usage.md.
func TestReadmeAgentFolder(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	headings, bodies := splitSteps(readmeSection(t, string(data), "Wiring it to an agent"))
	if len(headings) != len(wiringSteps) {
		t.Fatalf("the wiring guide has %d steps, want %d", len(headings), len(wiringSteps))
	}
	for _, s := range []string{"memory_<tag>_<os>_<arch>/memory", "memory_<tag>_<os>_<arch>/agent/"} {
		if !strings.Contains(bodies[0], s) {
			t.Errorf("step 1 does not say the archive holds %q", s)
		}
	}
	// Step 3's commands, run from the folder holding agent/, install it all.
	checkInstalled(t, runCommands(t, root, bashBlocks(bodies[2])))
	for _, s := range []string{"protocol/usage.md", "cp tools/"} {
		if strings.Contains(string(data), s) {
			t.Errorf("README.md still mentions %q", s)
		}
	}

	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	ci := readmeSection(t, string(claude), "CI")
	if !strings.Contains(ci, "`agent/`") {
		t.Errorf("CLAUDE.md's CI paragraph does not say the archives carry agent/:\n%s", ci)
	}
}

// Acceptance criterion 5: an archive holds memory_<tag>_<os>_<arch>/memory
// and every file of the repo's agent/, byte-equal, at
// memory_<tag>_<os>_<arch>/agent/<same path>, and nothing else.
func TestBuildBinariesCarryAgentFolder(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "dist")
	const tag = "20261119.3"
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "release", "build-binaries.sh"), tag, out) //nolint:gosec // the repo's own script
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-binaries.sh: %v\n%s", err, b)
	}

	name := "memory_" + tag + "_linux_amd64"
	entries := readArchive(t, filepath.Join(out, name+".tar.gz"))

	files := make([]string, 0, len(agentFiles)+1)
	files = append(files, "memory")
	for _, f := range agentFiles {
		files = append(files, "agent/"+f)
	}
	listing := make([]string, 0, len(entries))
	for n := range entries {
		listing = append(listing, n)
	}
	sort.Strings(listing)
	check(t, "archive listing", listing, withDirs(name+"/", files))

	for _, f := range agentFiles {
		want, err := os.ReadFile(filepath.Join(root, "agent", filepath.FromSlash(f))) //nolint:gosec // the repo's own file
		if err != nil {
			t.Fatal(err)
		}
		if got := entries[name+"/agent/"+f]; !bytes.Equal(got, want) {
			t.Errorf("%s/agent/%s differs from the repo's", name, f)
		}
	}
	elfMachine(elf.EM_X86_64)(t, entries[name+"/memory"])
}

// readArchive maps every entry of a .tar.gz to its content, directories as
// "name/" with no content. Any other entry type fails the test.
func readArchive(t *testing.T, archive string) map[string][]byte {
	t.Helper()
	f, err := os.Open(archive) //nolint:gosec // a file this test created
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(h.Name, "./")
		switch h.Typeflag {
		case tar.TypeDir:
			out[strings.TrimSuffix(name, "/")+"/"] = nil
		case tar.TypeReg:
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			out[name] = b
		default:
			t.Errorf("%s: entry %s has type %q", filepath.Base(archive), h.Name, h.Typeflag)
		}
	}
}
