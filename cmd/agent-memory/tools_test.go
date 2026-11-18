package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// repoRoot is the repository root, two levels above this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// shellTool is a file in the agent's shell-tool schema: exactly these six
// keys. A pointer is a key that may be omitted.
type shellTool struct {
	Command       string   `json:"command"`
	Description   string   `json:"description"`
	ReadOnly      *bool    `json:"read_only"`
	PrependArgs   []string `json:"prepend_args"`
	ForbiddenArgs []string `json:"forbidden_args"`
	AllowUserArgs *bool    `json:"allow_user_args"`
}

var shellToolKeys = map[string]bool{
	"command": true, "description": true, "read_only": true,
	"prepend_args": true, "forbidden_args": true, "allow_user_args": true,
}

// shippedTools is each file in tools/ and the verb it runs.
var shippedTools = map[string]string{
	"memory_read.json":   "read",
	"memory_write.json":  "write",
	"memory_search.json": "search",
	"memory_index.json":  "index",
}

// verbFlags is each verb's flag names, from the code that parses them.
// search takes its query verbatim and has no flags (CLI2); index takes no
// arguments at all.
func verbFlags(verb string) []string {
	var fs *flag.FlagSet
	switch verb {
	case "read":
		fs = readFlags(&readArgs{})
	case "write":
		fs = writeFlags(&writeArgs{})
	default:
		return nil
	}
	var names []string
	fs.VisitAll(func(f *flag.Flag) { names = append(names, "--"+f.Name) })
	return names
}

// loadTool parses a tool file strictly: only the six schema keys, spelled
// exactly (encoding/json alone matches keys case-insensitively), and
// nothing after the object.
func loadTool(t *testing.T, path string) shellTool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for k := range raw {
		if !shellToolKeys[k] {
			t.Errorf("%s: unknown key %q", filepath.Base(path), k)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var tool shellTool
	if err := dec.Decode(&tool); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Errorf("%s: data after the object", filepath.Base(path))
	}
	return tool
}

var flagMention = regexp.MustCompile(`--[a-z][a-z0-9-]*`)

// Acceptance criterion 3: each shipped tool file parses with the six schema
// keys, runs agent-memory with its verb prepended, has the right read_only
// and allow_user_args, and its description names exactly its verb's flags.
func TestToolFilesSchema(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "tools")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	check(t, "files in tools/", names, []string{"memory_index.json", "memory_read.json", "memory_search.json", "memory_write.json"})

	for file, verb := range shippedTools {
		tool := loadTool(t, filepath.Join(dir, file))
		check(t, file+" command", tool.Command, "agent-memory")
		check(t, file+" prepend_args", tool.PrependArgs, []string{verb})
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("%s: empty description", file)
		}
		readOnly := tool.ReadOnly != nil && *tool.ReadOnly
		check(t, file+" read_only", readOnly, verb != "write")
		allowArgs := tool.AllowUserArgs == nil || *tool.AllowUserArgs
		check(t, file+" allow_user_args", allowArgs, verb != "index")
		checkDescriptionFlags(t, file, verb, tool)
		if !strings.Contains(tool.Description, "75") || !strings.Contains(tool.Description, "same command again") {
			t.Errorf("%s does not say exit 75 means run the same command again", file)
		}
	}
}

// checkDescriptionFlags: the description names exactly the flags the
// verb's code accepts, and forbidden_args blocks none of them.
func checkDescriptionFlags(t *testing.T, file, verb string, tool shellTool) {
	t.Helper()
	want := verbFlags(verb)
	for _, f := range want {
		for _, forbidden := range tool.ForbiddenArgs {
			if forbidden == f || strings.HasPrefix(forbidden, f+"=") {
				t.Errorf("%s forbids its own flag %s", file, f)
			}
		}
	}
	mentioned := map[string]bool{}
	for _, m := range flagMention.FindAllString(tool.Description, -1) {
		mentioned[m] = true
	}
	got := make([]string, 0, len(mentioned))
	for m := range mentioned {
		got = append(got, m)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) && (len(got) != 0 || len(want) != 0) {
		t.Errorf("%s names flags %v; %s accepts %v", file, got, verb, want)
	}
}

// toolCaller runs the binary in dir as the agent runs a shell tool: the
// file's prepend_args, then the model's arguments, which the file must
// allow.
func toolCaller(t *testing.T, dir string) func(file string, userArgs ...string) string {
	tools := map[string]shellTool{}
	for file := range shippedTools {
		tools[file] = loadTool(t, filepath.Join(repoRoot(t), "tools", file))
	}
	return func(file string, userArgs ...string) string {
		t.Helper()
		tool := tools[file]
		if tool.AllowUserArgs != nil && !*tool.AllowUserArgs && len(userArgs) > 0 {
			t.Fatalf("%s takes no user arguments", file)
		}
		for _, a := range userArgs {
			if slices.Contains(tool.ForbiddenArgs, a) {
				t.Fatalf("%s forbids %q, which this call needs", file, a)
			}
		}
		r := runBin(t, dir, nil, append(append([]string(nil), tool.PrependArgs...), userArgs...)...)
		mustSucceed(t, r)
		return r.stdout
	}
}

// Acceptance criterion 3: run as shipped, each file's prepend_args make the
// binary behave as that verb against a seeded store.
func TestToolFilesRun(t *testing.T) {
	dir, _ := seeded(t)
	call := toolCaller(t, dir)

	var w struct {
		XMLName  xml.Name
		Created  string   `xml:"created,attr"`
		Stem     string   `xml:"stem,attr"`
		Assigned []string `xml:"assigned"`
	}
	decode(t, call("memory_write.json", "kettle-descaling", "--parent", "topics", "--description", "How to descale the kettle",
		"--tag", "home", "--add", "Use citric acid."), &w)
	check(t, "write element", w.XMLName.Local, "memory_write")
	check(t, "write created, stem", []string{w.Created, w.Stem}, []string{"true", "kettle-descaling"})
	if len(w.Assigned) != 1 {
		t.Fatalf("write assigned %v, want one ID", w.Assigned)
	}

	var n xNode
	decode(t, call("memory_read.json", "kettle-descaling"), &n)
	check(t, "read element", n.XMLName.Local, "node")
	check(t, "read node", attrsOf(n), attrs{"kettle-descaling", "topics/kettle-descaling", "topics", "home", "How to descale the kettle"})
	check(t, "read blocks", n.Blocks, []xBlock{{ID: w.Assigned[0], Text: "Use citric acid."}})

	var s xSearch
	decode(t, call("memory_search.json", "citric"), &s)
	check(t, "search element", s.XMLName.Local, "memory_search")
	check(t, "search hits", stemsOf(s), []string{"kettle-descaling"})

	out := call("memory_index.json")
	var ix struct {
		XMLName xml.Name
		Nodes   []xEntry `xml:"node"`
	}
	decode(t, out, &ix)
	check(t, "index element", ix.XMLName.Local, "memory_index")
	var topics []string
	for _, f := range ix.Nodes {
		if f.Stem == "topics" {
			for _, k := range f.Nodes {
				topics = append(topics, k.Stem)
			}
		}
	}
	check(t, "index children of topics", topics, []string{"kettle-descaling"})
}
