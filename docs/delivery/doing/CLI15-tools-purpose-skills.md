# CLI15 — The restructuring tool files, `memory_purpose.md` and the three skills

**Milestone:** restructuring  |  **Builds on:** CLI11–CLI14

## Goal
An agent can be given the restructuring verbs, and is told what memory is for on every turn and
how to use it only when it asks. `agent/` gains one tool file per restructuring verb and three
skills; the every-turn prompt file becomes `memory_purpose.md`; the READMEs say how to install all
of it.

## Scope
- `agent/tools/`: `memory_move.json`, `memory_rename.json`, `memory_move_block.json`,
  `memory_split.json`, `memory_merge.json`, `memory_delete.json`, `memory_retag.json` (new).
- `agent/prompt/`: `memory_purpose.md` (new); `memory_usage.md` is deleted.
- `agent/skills/`: `memory-usage/SKILL.md`, `memory-capture/SKILL.md`,
  `memory-restructure/SKILL.md` (new).
- `agent/README.md`, and `README.md`'s wiring guide, status line and any sentence the change makes
  false. The usage text in `cmd/memory/main.go` only if it names a shipped file that changed.
- New tests pinning the new files, and the CHANGELOG entry.

## Not in scope
- Any change to what a verb does, accepts, prints or refuses. No production Go code changes
  except the usage text and package comment, if they name a shipped file.
- Any change to the existing tool files (`memory_read.json`, `memory_write.json`,
  `memory_search.json`, `memory_index.json`, `memory_lint.json`), the live-context entry or the
  starting tree. `memory_write.json` keeps "Tags cannot be cleared and blocks cannot be
  deleted.", which is true of `write`.
- A dream or audit recipe, a `memory-lint` skill, any agent-side change, a Docker image.

## Governing spec
`docs/spec/agent-memory.md`: **III.8** (*Tool files*, *Tool descriptions*, *The prompt and the
skills* — the purpose text, the format rules and the three skill texts — and the rulings at the
head of the section); III.4 (what the operator installs, and where); III.6 (shipped text presumes
no typology); `CLAUDE.md` (*Docs and agent files travel with the code*).

## Prerequisite spec changes
Committed with this unit's lock: §4.5's old guidance block is removed and points to III.8; III.4's
table and bullets name `memory_purpose.md` and the skills; III.6's "who changes the top of the
tree"; `CLAUDE.md` names the new files.

## Existing tests this unit supersedes
Edited by the delivery lead in this unit's lock commit; the builder edits no existing test. After
the lock these are red, and this unit turns them green.
- `cmd/memory/agent_folder_test.go`:
  - `agentFiles`: `prompt/memory_usage.md` becomes `prompt/memory_purpose.md`; the three
    `skills/<name>/SKILL.md` and the seven new `tools/*.json` are added.
  - `guidanceMarker` now finds III.8's purpose block (``**`agent/prompt/memory_purpose.md`**,
    byte-equal to this block``).
  - `TestShippedGuidanceIsTheSpecText` reads `memory_purpose.md` and expects the purpose text
    ("It is for what would still matter", no longer "## Before you answer").
  - `TestAgentReadmeNamesEveryFile` expects the new paths and a fourth destination,
    `<state dir>/skills/`.
- `cmd/memory/typology_bin_test.go`: the two paths to `prompt/memory_usage.md` become
  `prompt/memory_purpose.md` (`TestInstallNeverOverwritesAStore`,
  `TestGuidanceAndReadmesPresumeNoTypology`).

Red at lock for those reasons: `TestShippedGuidanceIsTheSpecText`, `TestAgentFolderIsComplete`,
`TestAgentReadmeNamesEveryFile`, `TestAgentReadmeCommandsInstall`, `TestReadmeAgentFolder`,
`TestBuildBinariesCarryAgentFolder`, `TestInstallNeverOverwritesAStore`,
`TestGuidanceAndReadmesPresumeNoTypology`.

## Acceptance criteria
1. **`agent/` holds exactly the shipped files.** `agent/` contains the files in `agentFiles` and
   nothing else; `agent/prompt/memory_usage.md` does not exist. The eight tests listed above pass
   with no further edit to them.
   *Excludes:* keeping the old prompt file beside the new one; a stray file under `agent/`.
2. **The purpose text is III.8's, byte for byte.** `agent/prompt/memory_purpose.md` equals the
   fenced block after ``**`agent/prompt/memory_purpose.md`**, byte-equal to this block`` in III.8.
   It has no `##` heading, and says neither "area" nor `user`, `projects` or `topics` as a word.
   *Excludes:* carrying over "What to save", "How to write it" or "Before you answer".
3. **Each restructuring tool file is the spec's.** For each of the seven files: it parses as JSON
   with exactly the keys `command`, `description`, `read_only`, `prepend_args`, `forbidden_args`,
   `allow_user_args`; `command` is `memory`; `prepend_args` is `["--parent-dir",
   "${AGENT_STATE_DIR}", "<verb>"]` with the verb `move`, `rename`, `move-block`, `split`, `merge`,
   `delete`, `retag` respectively; `read_only` is false; `allow_user_args` is true;
   `forbidden_args` is `[]`. Its `description` equals the quoted text for that file under III.8's
   *Tool descriptions*, joined to one line, read from the spec and not restated in the test. No
   description says "area", names `user`, `projects`, `people` or `topics` as a word, or mentions
   `--parent-dir`.
   *Excludes:* a description retyped from memory; a read-only restructuring tool; a pinned
   `--recursive`.
4. **Each tool file works when run as the agent runs it.** For each of the seven files, running
   `command` with `prepend_args` (`${AGENT_STATE_DIR}` replaced by a temp state dir) followed by
   the argument list from an example in its own description, adapted to a fixture store, exits 0
   and does what the verb does: the node is moved, renamed, split, merged or deleted, the block is
   moved, the tag is renamed. The fixture is built with the shipped `memory_write.json` in the
   same way.
   *Excludes:* a `prepend_args` whose verb name is misspelt (`move_block`); an example in a
   description that the binary rejects.
5. **Each skill is III.8's text and loads under the agent's skill format.** For each of
   `memory-usage`, `memory-capture`, `memory-restructure`: `agent/skills/<name>/SKILL.md` equals
   the fenced block for it in III.8 byte for byte; the file starts with a `---` line, YAML
   frontmatter, a `---` line; the frontmatter parses with unknown keys rejected and holds exactly
   `name` and `description`; `name` equals the folder name, matches `[a-z0-9]+(-[a-z0-9]+)*` and is
   at most 64 characters; `description` is non-empty and at most 1024 bytes; the body is non-empty
   and says neither "area" nor `user`, `projects` or `topics` as a word. The folder holds nothing
   else.
   *Excludes:* a `Skill.md` or `skill.md` file name; a folder name that differs from `name`; an
   extra frontmatter key; a description wrapped onto two lines in a way YAML reads differently
   from the spec's.
6. **`agent/README.md` tells an operator everything.** It names every shipped file and its
   destination, including `<state dir>/skills/`; its `bash` commands, run from `agent/`, install
   every shipped file unchanged (the existing `TestAgentReadmeCommandsInstall`); it says to delete
   an existing `<state dir>/prompt/memory_usage.md`; it says each restructuring tool is granted by
   installing its file and withheld by leaving it out, and shows `--recursive` withheld with
   `"forbidden_args": ["--recursive"]`; it says the skills are read only when the model loads
   them; it no longer says that no verb moves, renames or deletes a node.
   *Excludes:* install commands that skip `skills/`; an instruction that would leave both prompt
   files in place.
7. **`README.md` agrees.** Its wiring guide's step 3 commands, run from the folder holding
   `agent/`, install every shipped file (the existing `TestReadmeAgentFolder`); the README names
   `memory_purpose.md`, the three skills and the seven restructuring tools; neither README, nor the
   usage text, mentions `memory_usage.md` except where telling the operator to delete the old
   file. A release archive carries the new files (the existing
   `TestBuildBinariesCarryAgentFolder`).
   *Excludes:* a README that still sends the operator to the old file.
8. **Nothing else changed.** No file under `internal/` changes. Under `cmd/memory`, no
   non-test file changes except `main.go`'s usage text or package comment, and only if they named
   a shipped file. Every test that existed before this unit passes; the gate is green.
   *Excludes:* a behaviour change slipped in with the docs.

## Environment
Local only: Go toolchain, temp dirs, `bash`. No network, no live agent. The agent's own skill
loader is in another repository and is not available here; criterion 5's rules are that loader's
rules, written out in III.8.

## Notes for the builder
- The QA gate is in `CLAUDE.md`. The whole tree stays lint-clean.
- Copy the texts from the spec; do not retype them. Existing tests show how a text is read out of
  the spec: `specSection` and `joinedQuote` (`cmd/memory/typology_bin_test.go`) for a quoted tool
  description, `specGuidance` (`cmd/memory/agent_folder_test.go`) for a fenced block,
  `TestLintToolFileIsTheSpecs` (`cmd/memory/lint_tool_test.go`) for a whole tool file, and
  `TestToolFilesRun` (`cmd/memory/tools_test.go`) for running a tool file as the agent does.
- `gopkg.in/yaml.v3` is already a dependency; `KnownFields(true)` rejects unknown keys.
- The skill bodies contain the word "people" as English ("facts about people"); the word rule for
  the purpose text and the skills is `user`, `projects`, `topics`, as it is for the guidance today.
- Put new tests in new test files.
