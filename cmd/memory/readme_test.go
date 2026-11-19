package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wiringSteps is the README wiring guide's steps, in order, each found by a
// word its heading must hold (case-insensitive).
var wiringSteps = []string{"install", "path", "agent"}

// readmeSection returns the body of the "## <title…>" section, up to the
// next "## " heading.
func readmeSection(t *testing.T, readme, title string) string {
	t.Helper()
	start := strings.Index(readme, "\n## "+title)
	if start < 0 {
		t.Fatalf("README has no %q section", "## "+title)
	}
	body := readme[start+1:]
	if end := strings.Index(body[1:], "\n## "); end >= 0 {
		body = body[:end+1]
	}
	return body
}

// splitSteps splits a section at its "### " headings: each heading line
// with the text up to the next.
func splitSteps(section string) (headings, bodies []string) {
	for _, part := range strings.Split(section, "\n### ")[1:] {
		heading, body, _ := strings.Cut(part, "\n")
		headings = append(headings, heading)
		bodies = append(bodies, body)
	}
	return headings, bodies
}

// Acceptance criterion 4: the README's wiring guide documents install,
// PATH, copying the tool files, placing usage.md and a live-context
// entry, in that order, and the entry is valid JSON in the live-context
// schema (command, description, args).
func TestReadmeWiringGuide(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	section := readmeSection(t, string(data), "Wiring it to an agent")
	headings, bodies := splitSteps(section)
	if len(headings) != len(wiringSteps) {
		t.Fatalf("the wiring guide has %d steps %q, want %d", len(headings), headings, len(wiringSteps))
	}
	for i, word := range wiringSteps {
		if !strings.Contains(strings.ToLower(headings[i]), word) {
			t.Errorf("step %d is %q; want the %s step", i+1, headings[i], word)
		}
	}

	mentions := map[int][]string{
		0: {"memory_<tag>_<os>_<arch>.tar.gz", "checksums.txt", "go build"},
		2: {"agent/README.md"},
	}
	for step, words := range mentions {
		for _, w := range words {
			if !strings.Contains(bodies[step], w) {
				t.Errorf("step %d (%s) does not mention %q", step+1, headings[step], w)
			}
		}
	}

	entry, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "livecontext", "memory_index.json")) //nolint:gosec // the repo's own file
	if err != nil {
		t.Fatal(err)
	}
	checkLiveContextEntry(t, string(entry))
}

// checkLiveContextEntry: block is one JSON object in the agent's
// live-context schema (command, description, args) running index.
func checkLiveContextEntry(t *testing.T, block string) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(block), &raw); err != nil {
		t.Fatalf("the live-context entry is not JSON: %v\n%s", err, block)
	}
	for k := range raw {
		if k != "command" && k != "description" && k != "args" {
			t.Errorf("the live-context entry has key %q, outside the schema", k)
		}
	}
	var entry struct {
		Command     string   `json:"command"`
		Description string   `json:"description"`
		Args        []string `json:"args"`
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(block)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entry); err != nil {
		t.Fatalf("the live-context entry: %v", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Error("the live-context block holds more than one JSON value")
	}
	check(t, "entry command", entry.Command, "memory")
	check(t, "entry args", entry.Args, []string{"--parent-dir", "${AGENT_STATE_DIR}", "index"})
	if strings.TrimSpace(entry.Description) == "" {
		t.Error("the live-context entry has no description")
	}
}
