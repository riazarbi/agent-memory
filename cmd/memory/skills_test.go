package main

// CLI15: the every-turn prompt file says only what memory is for, and how
// to use memory ships as three skills, each III.8's text and each loadable
// under the agent's skill format (docs/spec/agent-memory.md Part III.8,
// "The prompt and the skills").

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// shippedSkills is the skills in agent/skills/, by folder name.
var shippedSkills = []string{"memory-capture", "memory-restructure", "memory-usage"}

// The agent's skill format (III.8): the limits its loader enforces.
const (
	skillFile           = "SKILL.md"
	maxSkillName        = 64
	maxSkillDescription = 1024
)

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// guidanceWord matches the starting-tree stems the purpose text and the
// skills must not name. "people" is left out: the texts use it as English.
var guidanceWord = regexp.MustCompile(`\b(user|projects|topics)\b`)

// specFence returns the content of the first fenced block after marker in
// the spec: the lines between its fences, each with its newline.
func specFence(t *testing.T, marker string) []byte {
	t.Helper()
	spec, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "spec", "agent-memory.md"))
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.Index(spec, []byte(marker))
	if at < 0 {
		t.Fatalf("the spec has no %q", marker)
	}
	_, rest, ok := strings.Cut(string(spec[at:]), "\n```")
	if !ok {
		t.Fatalf("no fenced block after %q", marker)
	}
	_, rest, _ = strings.Cut(rest, "\n") // the rest of the opening fence line
	body, _, ok := strings.Cut(rest, "\n```\n")
	if !ok {
		t.Fatalf("the fenced block after %q is not closed", marker)
	}
	return []byte(body + "\n")
}

// typologyProblems is what text says that presumes a typology: "area", or
// user, projects or topics as a word.
func typologyProblems(what, text string) []string {
	var out []string
	if strings.Contains(strings.ToLower(text), "area") {
		out = append(out, what+" says area")
	}
	if w := guidanceWord.FindString(text); w != "" {
		out = append(out, fmt.Sprintf("%s names %q", what, w))
	}
	return out
}

// Acceptance criteria 1 and 2: the purpose text is III.8's block byte for
// byte, has no ## heading, carries none of the how-to the skills now hold,
// presumes no typology, and the old prompt file is gone.
func TestPurposeTextSaysOnlyWhatMemoryIsFor(t *testing.T) {
	prompt := filepath.Join(repoRoot(t), "agent", "prompt")
	got, err := os.ReadFile(filepath.Join(prompt, "memory_purpose.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := specFence(t, guidanceMarker)
	check(t, "the block found by specFence", string(want), string(specGuidance(t)))
	check(t, "agent/prompt/memory_purpose.md", string(got), string(want))

	for _, l := range strings.Split(string(got), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "##") {
			t.Errorf("the purpose text has the heading %q", l)
		}
	}
	for _, s := range []string{"What to save", "How to write it", "Before you answer", "Never save"} {
		if strings.Contains(string(got), s) {
			t.Errorf("the purpose text still carries %q", s)
		}
	}
	for _, p := range typologyProblems("the purpose text", string(got)) {
		t.Error(p)
	}

	entries, err := os.ReadDir(prompt)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "files in agent/prompt/", entryNamesOf(entries), []string{"memory_purpose.md"})
}

func entryNamesOf(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// skillMeta is the frontmatter a shipped skill may hold: name and
// description, and nothing else.
type skillMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// skillParts splits a skill file into its frontmatter (the lines between
// the opening --- line and the next --- line) and its body.
func skillParts(data []byte) (frontmatter, body string, err error) {
	rest, ok := strings.CutPrefix(string(data), "---\n")
	if !ok {
		return "", "", errors.New("does not start with a --- line")
	}
	frontmatter, body, ok = strings.Cut(rest, "\n---\n")
	if !ok {
		return "", "", errors.New("no --- line closes the frontmatter")
	}
	return frontmatter + "\n", body, nil
}

// loadSkillMeta parses frontmatter as the agent's loader does, rejecting
// unknown keys, and also refuses anything but exactly the two keys.
func loadSkillMeta(frontmatter string) (skillMeta, error) {
	dec := yaml.NewDecoder(strings.NewReader(frontmatter))
	dec.KnownFields(true)
	var meta skillMeta
	if err := dec.Decode(&meta); err != nil {
		return meta, err
	}
	var keys map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &keys); err != nil {
		return meta, err
	}
	_, hasName := keys["name"]
	_, hasDescription := keys["description"]
	if !hasName || !hasDescription || len(keys) != 2 {
		return meta, fmt.Errorf("frontmatter holds %d keys, want exactly name and description", len(keys))
	}
	return meta, nil
}

// skillFormatProblems applies the agent's skill-format rules (III.8) to a
// skill file's bytes, for the skill in the folder called name, and returns
// what breaks them; none means the agent's loader would load it.
func skillFormatProblems(name string, data []byte) []string {
	frontmatter, body, err := skillParts(data)
	if err != nil {
		return []string{err.Error()}
	}
	meta, err := loadSkillMeta(frontmatter)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	if meta.Name != name {
		out = append(out, fmt.Sprintf("name %q is not the folder name", meta.Name))
	}
	if !skillName.MatchString(meta.Name) || len(meta.Name) > maxSkillName {
		out = append(out, fmt.Sprintf("name %q is not lowercase-hyphenated of at most %d characters", meta.Name, maxSkillName))
	}
	if strings.TrimSpace(meta.Description) == "" || len(meta.Description) > maxSkillDescription {
		out = append(out, fmt.Sprintf("description is empty or over %d bytes (%d)", maxSkillDescription, len(meta.Description)))
	}
	// YAML must read the description as the one line the file writes.
	if !strings.Contains("\n"+frontmatter, "\ndescription: "+meta.Description+"\n") {
		out = append(out, fmt.Sprintf("YAML reads the description as %q, not as the line written", meta.Description))
	}
	if strings.TrimSpace(body) == "" {
		out = append(out, "empty body")
	}
	out = append(out, typologyProblems("the body", body)...)
	return append(out, typologyProblems("the description", meta.Description)...)
}

// Acceptance criterion 5: each shipped skill is III.8's fenced block byte
// for byte, is the only file in a folder named as its frontmatter names it,
// and passes the agent's skill-format rules.
func TestShippedSkillsAreTheSpecTextAndLoad(t *testing.T) {
	skills := filepath.Join(repoRoot(t), "agent", "skills")
	entries, err := os.ReadDir(skills)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "folders in agent/skills/", entryNamesOf(entries), shippedSkills)

	for _, name := range shippedSkills {
		files, err := os.ReadDir(filepath.Join(skills, name))
		if err != nil {
			t.Fatal(err)
		}
		// The name as it is on disk: Skill.md is not SKILL.md, even where
		// the filesystem would open either.
		check(t, "files in agent/skills/"+name+"/", entryNamesOf(files), []string{skillFile})

		got, err := os.ReadFile(filepath.Join(skills, name, skillFile))
		if err != nil {
			t.Fatal(err)
		}
		want := specFence(t, "\n`agent/skills/"+name+"/"+skillFile+"`")
		if !bytes.HasPrefix(want, []byte("---\nname: "+name+"\n")) {
			t.Fatalf("the spec block found for %s is not its skill:\n%s", name, want)
		}
		check(t, "agent/skills/"+name+"/"+skillFile, string(got), string(want))
		for _, p := range skillFormatProblems(name, got) {
			t.Errorf("agent/skills/%s/%s: %s", name, skillFile, p)
		}
	}
}

// Acceptance criterion 5: the format check itself refuses what the agent's
// loader refuses, so a skill that passes it loads. Each case is a shipped
// skill with one thing wrong.
func TestSkillFormatCheckRefusesBadSkills(t *testing.T) {
	const name = "memory-usage"
	good, err := os.ReadFile(filepath.Join(repoRoot(t), "agent", "skills", name, skillFile))
	if err != nil {
		t.Fatal(err)
	}
	text := string(good)
	_, description, _ := strings.Cut(text, "\ndescription: ")
	description, _, _ = strings.Cut(description, "\n")
	half := len(description) / 2
	for what, bad := range map[string]string{
		"an extra frontmatter key":       strings.Replace(text, "\n---\n", "\nlicense: MIT\n---\n", 1),
		"a name that is not the folder":  strings.Replace(text, "name: "+name, "name: memory-use", 1),
		"no description":                 strings.Replace(text, "description: "+description+"\n", "", 1),
		"an empty description":           strings.Replace(text, "description: "+description, `description: ""`, 1),
		"a description over 1024 bytes":  strings.Replace(text, description, strings.Repeat("x", maxSkillDescription+1), 1),
		"a description YAML reads apart": strings.Replace(text, description, description[:half]+"\n\n  "+description[half:], 1),
		"no frontmatter":                 strings.TrimPrefix(text, "---\n"),
		"an empty body":                  text[:strings.Index(text, "\n---\n")+len("\n---\n")],
		"a body that names a node":       text + "File it under topics.\n",
		"a body that says area":          text + "Pick the right area.\n",
	} {
		if bad == text {
			t.Fatalf("%s: the case changes nothing", what)
		}
		if len(skillFormatProblems(name, []byte(bad))) == 0 {
			t.Errorf("%s: the format check accepts it", what)
		}
	}
	if p := skillFormatProblems(name, good); len(p) != 0 {
		t.Errorf("the format check refuses the shipped skill: %q", p)
	}
}
