# CLI9 — The agent can lint its own store: `lint --exit-zero` and `memory_lint.json`

**Milestone:** cli  |  **Builds on:** CLI8

## Goal
An agent with the shipped tools can check its own store and see the full report, errors included.
`lint` gains `--exit-zero` (report as usual, exit 0), because the agent's shell tools discard
stdout when a command exits non-zero; the new tool file pins it.

## Scope
- `docs/spec/agent-memory.md` III.7 (`--exit-zero`, the tool file and its pinned description, as in
  this unit's lock commit), III.2's `lint` row, III.4's tool-file row.
- `cmd/memory`: `lint` accepts `--exit-zero`; the usage text shows it.
- `agent/tools/memory_lint.json` (new), exactly as III.7 specifies.
- `agent/README.md`: the table lists `tools/memory_lint.json`; a sentence on what it is for (the
  agent checking its store after the operator's edits) and that it is optional. `README.md`:
  documents `--exit-zero` beside `lint`.
- Tests and the CHANGELOG entry.

## Not in scope
- Any change to the agent repo (how shell tools treat non-zero exits).
- A live-context entry for lint; any change to the report itself or to other verbs.

## Governing spec
- `docs/spec/agent-memory.md`: III.7 (whole), III.4 (tool files), agent spec `child-environment.md`
  E3 (substitution), as used by the existing tool-file tests.

## Existing tests this unit supersedes
Edited in the lock commit; builders do not touch them:
- `tools_test.go`: `TestToolFilesSchema` expects five files in `agent/tools/`; `shippedTools` keeps
  the four verb tools (their shared checks — prepend ending in the verb, exit-75 sentence, flag
  mentions — do not fit `lint --exit-zero`), so `memory_lint.json` needs its own test.
- `agent_folder_test.go`: `agentFiles` and `TestAgentReadmeNamesEveryFile` include
  `tools/memory_lint.json`.

At the lock commit `TestToolFilesSchema`, `TestAgentFolderIsComplete`,
`TestAgentReadmeNamesEveryFile` and the README install tests fail (no file yet); the rest pass.

## Acceptance criteria
`W` has `W/.agent/memory/` copied from the shipped starting tree (`seeded()`), unless stated.

1. **`--exit-zero` keeps the report, drops the verdict.** On a store with one error (a folder note
   with no frontmatter) and one warning: `lint` → exit 2; `lint --exit-zero` → exit 0, stderr empty,
   stdout byte-equal to plain `lint`'s stdout. On a clean store both exit 0 with equal stdout.
   `lint --exit-zero extra` → exit 1, one line on stderr, empty stdout; `--parent-dir W/nope lint
   --exit-zero` → exit 1 with III.1's missing-parent line.
   *Excludes:* `--exit-zero` hiding or changing the report; turning a failure to run into exit 0.
2. **The tool file is exactly III.7's.** `agent/tools/memory_lint.json` parses with the six schema
   keys (the existing strict loader): `command` `memory`, `prepend_args`
   `["--parent-dir", "${AGENT_STATE_DIR}", "lint", "--exit-zero"]`, `read_only` true,
   `allow_user_args` false, `forbidden_args` empty, `description` equal to III.7's quoted text
   (read from the spec). The description contains neither `75` nor `--exit-zero` nor any `--` flag.
   *Excludes:* a tool running plain `lint`; a description promising a retry exit or naming a flag.
3. **Through the tool harness, the model sees the report.** Calling `memory_lint.json` the way the
   existing tool tests do (prepend substituted with `W/.agent`, decoy `MEMORY_PARENT_DIR`) on a store
   with an error → exit 0 and a `<memory_lint … errors="1" …>` report naming the broken node's
   path; on the clean starting tree → `errors="0" warnings="0"`. The decoy parent stays empty.
   *Excludes:* the report going missing on errors; linting a store other than the pinned one.
4. **Install and docs.** `agent/README.md`'s install commands put `tools/memory_lint.json` in
   `<state dir>/tools/` (covered by the existing install tests once the lock edits pass);
   `agent/README.md` names the file and says it is optional; the usage text and `README.md` mention
   `--exit-zero`.
   *Excludes:* a tool file the install does not copy; undocumented `--exit-zero`.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- `noArgs` refuses every argument today; `lint` needs a flag set with one bool.
- `run` maps `errLintFound` to exit 2; with `--exit-zero`, `lintVerb` simply returns the report with
  no error.
- `tools_test.go`'s `toolCaller` already substitutes `${AGENT_STATE_DIR}` and sets a decoy parent.
- Keep the tree lint-clean (`golangci-lint run ./...`, full tree).

## Post-verification notes (2026-11-19)
- Built in one pass. Pre-filter green; no existing test touched. Production change: `setFlag` handles
  bool flags (`--exit-zero=VALUE` refused), `lint` gets a flag set, `memory_lint.json` (new). New
  tests `cmd/memory/lint_tool_test.go`.
- Verifier: all four criteria PASS through the real binary and an agent-like harness (stdout shown
  only on exit 0): the model sees the report with errors; 17 code and tool-file mutants killed;
  10/10 inverted assertions failed; tamper and scope clean; govulncheck clean.
- Builder spec gaps resolved here: III.4's tool-file rule and III.3's exit-2 bullet now note the
  lint exception; the CLI3 exit-75 gap reworded for five tool files. The builder also corrected
  README and agent/README sentences CLI9 made false (in scope by implication; the unit should have
  listed them). Left: `toolCaller` and the typology check are wired to the four verb tools.
- Process: the builder's gate command ended with `git status` (refused, never ran).
