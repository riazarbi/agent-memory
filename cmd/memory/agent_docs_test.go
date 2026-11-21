package main

// CLI15: agent/README.md and README.md tell an operator how to install the
// restructuring tools, the purpose text and the skills, and how to get rid
// of the prompt file they replace (docs/spec/agent-memory.md Part III.8,
// III.4).

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const oldPromptFile = "memory_usage.md"

// repoFile is a file of the repo, as text.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// installRuns is each README's install commands and the folder they are
// run from.
func installRuns(t *testing.T) map[string]struct {
	dir    string
	blocks []string
} {
	t.Helper()
	root := repoRoot(t)
	_, steps := splitSteps(readmeSection(t, repoFile(t, "README.md"), "Wiring it to an agent"))
	if len(steps) != len(wiringSteps) {
		t.Fatalf("the wiring guide has %d steps", len(steps))
	}
	return map[string]struct {
		dir    string
		blocks []string
	}{
		"agent/README.md":  {filepath.Join(root, "agent"), bashBlocks(repoFile(t, "agent/README.md"))},
		"README.md step 3": {root, bashBlocks(steps[2])},
	}
}

// Acceptance criteria 6 and 7: run on a state dir that has the old prompt
// file, each README's install commands leave memory_purpose.md and not
// memory_usage.md, install the three skills, and touch nothing else the
// operator had in prompt/ or skills/.
func TestInstallReplacesTheOldPromptFile(t *testing.T) {
	for name, run := range installRuns(t) {
		state := filepath.Join(t.TempDir(), "state")
		old := filepath.Join(state, "prompt", oldPromptFile)
		own := filepath.Join(state, "prompt", "house_rules.md")
		ownSkill := filepath.Join(state, "skills", "own-skill", skillFile)
		writeFile(t, old, "# Memory\n\n## What to save\n")
		writeFile(t, own, "Mine.\n")
		writeFile(t, ownSkill, "Mine too.\n")

		runCommandsIn(t, run.dir, state, run.blocks)

		if exists(old) {
			t.Errorf("%s: the commands leave %s beside memory_purpose.md", name, oldPromptFile)
		}
		checkInstalled(t, state)
		check(t, name+": the operator's own prompt file", readText(t, own), "Mine.\n")
		check(t, name+": the operator's own skill", readText(t, ownSkill), "Mine too.\n")
		check(t, name+": prompt/", treeUnder(t, filepath.Join(state, "prompt")), []string{"house_rules.md", "memory_purpose.md"})
		installed := withDirs("", []string{"memory-capture/SKILL.md", "memory-restructure/SKILL.md", "memory-usage/SKILL.md", "own-skill/SKILL.md"})
		check(t, name+": skills/", treeUnder(t, filepath.Join(state, "skills")), installed)
	}
}

// linesNaming is the lines of text that contain s.
func linesNaming(text, s string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, s) {
			out = append(out, l)
		}
	}
	return out
}

// Acceptance criterion 7: neither README, the usage text nor main.go names
// memory_usage.md except where telling the operator to delete it, and
// agent/README.md does tell them, in words and not only as a command.
func TestOldPromptFileIsNamedOnlyToDeleteIt(t *testing.T) {
	removal := regexp.MustCompile(`(?i)\bdelete\b|\brm -f\b`)
	for _, name := range []string{"README.md", "agent/README.md"} {
		for _, l := range linesNaming(repoFile(t, name), oldPromptFile) {
			if !removal.MatchString(l) {
				t.Errorf("%s names %s without saying to delete it: %q", name, oldPromptFile, l)
			}
		}
	}
	told := slices.ContainsFunc(linesNaming(repoFile(t, "agent/README.md"), "<state dir>/prompt/"+oldPromptFile), func(l string) bool {
		return strings.Contains(strings.ToLower(l), "delete")
	})
	if !told {
		t.Errorf("agent/README.md does not say, in words, to delete <state dir>/prompt/%s", oldPromptFile)
	}
	if usage := runBin(t, t.TempDir(), nil).stderr; strings.Contains(usage, oldPromptFile) || usage == "" {
		t.Errorf("the usage text is empty or names %s:\n%s", oldPromptFile, usage)
	}
	if strings.Contains(repoFile(t, "cmd/memory/main.go"), oldPromptFile) {
		t.Errorf("cmd/memory/main.go names %s", oldPromptFile)
	}
}

// Acceptance criterion 7: README.md names the purpose text, the three
// skills and the seven restructuring tool files, and no longer calls the
// prompt file guidance on when and what to save.
func TestReadmeNamesTheNewAgentFiles(t *testing.T) {
	readme := repoFile(t, "README.md")
	want := append([]string{"memory_purpose.md", "<state dir>/skills"}, shippedSkills...)
	for file := range restructuringTools {
		want = append(want, file)
	}
	for _, s := range want {
		if !strings.Contains(readme, s) {
			t.Errorf("README.md does not name %q", s)
		}
	}
	status := ""
	for _, l := range strings.Split(readme, "\n") {
		if strings.HasPrefix(l, "**Status:**") {
			status = l
		}
	}
	for _, s := range []string{"skills", "restructuring", "what memory is for"} {
		if !strings.Contains(status, s) {
			t.Errorf("README.md's status line does not mention %s: %q", s, status)
		}
	}
}

// readmeForbiddenArgs is the forbidden_args list agent/README.md shows for
// withholding --recursive, parsed as the JSON it is.
func readmeForbiddenArgs(t *testing.T, readme string) []string {
	t.Helper()
	m := regexp.MustCompile(`"forbidden_args": \[[^\]]*\]`).FindString(readme)
	if m == "" {
		t.Fatal(`agent/README.md shows no "forbidden_args": [...]`)
	}
	var file struct {
		ForbiddenArgs []string `json:"forbidden_args"`
	}
	decodeJSON(t, []byte("{"+m+"}"), &file)
	return file.ForbiddenArgs
}

// Acceptance criterion 6: agent/README.md says each restructuring tool is
// granted by installing its file and withheld by leaving it out, shows
// --recursive withheld with both spellings and says why, says the skills
// are read only when the model loads them, and no longer says that no verb
// moves, renames or deletes a node.
func TestAgentReadmeExplainsRestructuringAndSkills(t *testing.T) {
	readme := repoFile(t, "agent/README.md")
	section := readmeSection(t, readme, "The restructuring tools")
	for file := range restructuringTools {
		if !strings.Contains(section, "`"+file+"`") {
			t.Errorf("the restructuring section does not name %s", file)
		}
	}
	check(t, "the forbidden_args shown", readmeForbiddenArgs(t, section), []string{"--recursive", "-recursive"})
	flat := strings.Join(strings.Fields(readme), " ")
	for what, re := range map[string]string{
		"a tool is withheld by leaving its file out":      `(?i)leav(e|ing) (it|one|a file|its file|the file) out`,
		"installing a file is the grant":                  `(?i)installing (a|its|the) (tool )?file is the grant`,
		"the agent refuses forbidden args by exact match": `(?i)exact match`,
		"memory accepts one dash or two":                  `(?i)one dash or two`,
		"--recursive=true needs no entry":                 `--recursive=true`,
		"skills are read only when loaded":                `(?i)only when the model loads`,
		"deleting the old prompt file":                    `<state dir>/prompt/memory_usage\.md`,
	} {
		if !regexp.MustCompile(re).MatchString(flat) {
			t.Errorf("agent/README.md does not say that %s (no match for %s)", what, re)
		}
	}
	for _, stale := range []string{"no verb moves", "can never remove one", "when and what to save"} {
		if strings.Contains(flat, stale) {
			t.Errorf("agent/README.md still says %q", stale)
		}
	}
}

// Acceptance criterion 6: where agent/README.md says which descriptions
// mention exit 75, it says every one but memory_lint.json's, and that is
// true of the files in agent/tools/, the seven new ones included.
func TestAgentReadmeExit75CoversEveryTool(t *testing.T) {
	var sentence string
	for _, l := range strings.Split(repoFile(t, "agent/README.md"), "\n") {
		if strings.Contains(l, "exit 75") {
			sentence += l
		}
	}
	for _, s := range []string{"every description", "`memory_lint.json`", "restructuring"} {
		if !strings.Contains(sentence, s) {
			t.Errorf("the exit-75 text does not say %q: %q", s, sentence)
		}
	}
	if strings.Contains(sentence, "four") {
		t.Errorf("the exit-75 text still counts four tools: %q", sentence)
	}
	entries, err := os.ReadDir(filepath.Join(repoRoot(t), "agent", "tools"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		d := loadTool(t, toolPath(t, e.Name())).Description
		says := strings.Contains(d, "exits with code 75") && strings.Contains(d, "run the same command again")
		check(t, e.Name()+" says exit 75 means run again", says, e.Name() != lintTool)
	}
}

// Acceptance criterion 6: the two forbidden_args entries the README shows
// are enough, and both are needed. memory takes --recursive with one dash
// or two, so each spelling deletes a subtree and each needs its entry; a
// value on the flag (--recursive=true), which an exact-match list cannot
// catch, is refused by memory itself and deletes nothing.
func TestTwoForbiddenArgsWithholdRecursive(t *testing.T) {
	forbidden := readmeForbiddenArgs(t, repoFile(t, "agent/README.md"))
	for _, spelling := range []string{"--recursive", "-recursive"} {
		f := newStoreF(t)
		if !slices.Contains(forbidden, spelling) {
			t.Errorf("the README's forbidden_args do not hold %s", spelling)
		}
		got := decodeDelete(t, f.ok(t, "delete", "old-plan", spelling))
		check(t, "delete old-plan "+spelling+": nodes removed", got.Nodes, "2")
		if exists(f.oldPlan) || exists(f.oldTask) {
			t.Errorf("delete old-plan %s left the subtree", spelling)
		}
	}
	f := newStoreF(t)
	for _, arg := range []string{"--recursive=true", "-recursive=true", "--recursive=false", "-recursive=false"} {
		if slices.Contains(forbidden, arg) {
			t.Errorf("the README's forbidden_args hold %s, which memory refuses anyway", arg)
		}
		line := f.refused(t, "delete", "old-plan", arg)
		flag, _, _ := strings.Cut(arg, "=")
		if !strings.HasPrefix(line, flag+":") {
			t.Errorf("delete old-plan %s: %q does not name %s", arg, line, flag)
		}
		mustExist(t, f.oldPlan, f.oldTask)
	}
}
