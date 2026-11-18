package main

// Tests that run the built agent-memory binary, each in a fresh temp
// directory as its working directory, with a controlled environment.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// binPath is the agent-memory binary built once for the whole package.
var binPath string

func TestMain(m *testing.M) {
	os.Exit(buildAndRun(m))
}

func buildAndRun(m *testing.M) int {
	dir, err := os.MkdirTemp("", "agent-memory-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	binPath = filepath.Join(dir, "agent-memory")
	build := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building agent-memory: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

type result struct {
	stdout string
	stderr string
	code   int
}

// run executes the binary in dir. The inherited environment loses
// AGENT_STATE_DIR and TZ; env adds variables (TZ defaults to UTC).
func runBin(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...) //nolint:gosec // the binary this package builds
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AGENT_STATE_DIR=") || strings.HasPrefix(kv, "TZ=") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	if _, ok := env["TZ"]; !ok {
		cmd.Env = append(cmd.Env, "TZ=UTC")
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running agent-memory %v: %v", args, err)
		}
		code = exit.ExitCode()
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// mustSucceed fails unless r exited 0 with empty stderr.
func mustSucceed(t *testing.T, r result) {
	t.Helper()
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("exit %d, stderr %q, stdout %q", r.code, r.stderr, r.stdout)
	}
}

// mustFail checks the error convention: exit 1, stdout empty, one line on
// stderr. It returns that line.
func mustFail(t *testing.T, r result) string {
	t.Helper()
	if r.code != 1 {
		t.Fatalf("exit %d, want 1 (stdout %q, stderr %q)", r.code, r.stdout, r.stderr)
	}
	if r.stdout != "" {
		t.Fatalf("stdout %q, want empty on failure", r.stdout)
	}
	line := strings.TrimSuffix(r.stderr, "\n")
	if line == "" || strings.Contains(line, "\n") {
		t.Fatalf("stderr %q, want exactly one line", r.stderr)
	}
	return line
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// writeNode writes a folder note at notes/<rel>/<stem>.md.
func writeNode(t *testing.T, notes, rel, content string) string {
	t.Helper()
	p := filepath.Join(notes, filepath.FromSlash(rel), filepath.Base(rel)+".md")
	writeFile(t, p, content)
	return p
}

// The XML shapes a reader of the output sees, declared independently of the
// production types so a wrong tag or attribute there fails here.
type xNode struct {
	XMLName     xml.Name
	Stem        string    `xml:"stem,attr"`
	Path        string    `xml:"path,attr"`
	Parent      string    `xml:"parent,attr"`
	Tags        string    `xml:"tags,attr"`
	Modified    string    `xml:"modified,attr"`
	Description string    `xml:"description"`
	Blocks      []xBlock  `xml:"block"`
	Children    *xEntries `xml:"children"`
	Links       *xLinks   `xml:"links"`
	Backlinks   *xLinks   `xml:"backlinks"`
}

type xBlock struct {
	ID   string `xml:"id,attr"`
	Text string `xml:",chardata"`
}

type xEntries struct {
	Nodes []xEntry `xml:"node"`
}

type xEntry struct {
	Stem     string   `xml:"stem,attr"`
	Tags     string   `xml:"tags,attr"`
	Children string   `xml:"children,attr"`
	Modified string   `xml:"modified,attr"`
	Text     string   `xml:",chardata"`
	Nodes    []xEntry `xml:"node"`
}

type xLinks struct {
	Links []xLink `xml:"link"`
}

type xLink struct {
	Stem     string `xml:"stem,attr"`
	Block    string `xml:"block,attr"`
	Dangling string `xml:"dangling,attr"`
}

type xRoot struct {
	XMLName xml.Name
	Nodes   []xEntry `xml:"node"`
	Tags    *string  `xml:"tags"`
}

type xInit struct {
	XMLName xml.Name
	Store   string   `xml:"store,attr"`
	Seeded  string   `xml:"seeded,attr"`
	Created []string `xml:"created"`
	Text    string   `xml:",chardata"`
}

func decode(t *testing.T, out string, v any) {
	t.Helper()
	if err := xml.Unmarshal([]byte(out), v); err != nil {
		t.Fatalf("output is not XML: %v\n%s", err, out)
	}
}

func (e xEntry) desc() string { return strings.TrimSpace(e.Text) }

// check reports got != want (deep equality) under what.
func check(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %+v, want %+v", what, got, want)
	}
}

// attrs is a node result's attributes and description, compared as one.
type attrs struct{ Stem, Path, Parent, Tags, Description string }

func attrsOf(n xNode) attrs {
	return attrs{n.Stem, n.Path, n.Parent, n.Tags, n.Description}
}

// entry is a child entry's attributes and text, compared as one.
type entry struct{ Stem, Tags, Children, Desc string }

func entryOf(e xEntry) entry { return entry{e.Stem, e.Tags, e.Children, e.desc()} }
