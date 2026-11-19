# CLI4 — `memory` is addressed, not agent-aware

**Milestone:** cli  |  **Builds on:** CLI1–CLI3 and the unreleased `MEMORY_DIR` change

## Goal
`memory` finds its store from `--parent-dir` (before the verb), else `MEMORY_PARENT_DIR`, else
`HOME`, always as `<parent>/memory`, and knows nothing about agents: no `.env`, no `AGENT_STATE_DIR`.
The shipped tool files pin an agent's store with `--parent-dir ${AGENT_STATE_DIR}`, which the agent
substitutes, so an operator's exported `MEMORY_PARENT_DIR` can never redirect an agent's memory.

## Scope
- `docs/spec/agent-memory.md` **Part III.1** in full, and the III.4 bullets on tool files and the
  index (as amended in this unit's lock commit), with D32.
- `cmd/memory`: the store resolution (`store.go`), the global `--parent-dir` option before the verb
  (`main.go`'s dispatch), the usage text and the package comment. Remove the `.env` reader and every
  `AGENT_STATE_DIR` / `MEMORY_DIR` reference from production code.
- `tools/*.json`: `prepend_args` becomes `["--parent-dir", "${AGENT_STATE_DIR}", "<verb>"]`; the
  descriptions are otherwise unchanged and never mention `--parent-dir`.
- README: the usage section and store paragraph; the wiring guide's `init` step, tool-file step and
  live-context entry (`args` `["--parent-dir", "${AGENT_STATE_DIR}", "index"]`), and a note that the
  tool files need an agent that substitutes `${AGENT_STATE_DIR}` (agent CHANGELOG 2026-11-19).
- `CLAUDE.md`'s CI section still names the binaries `agent-memory`: correct it to `memory` (a stale
  line from the rename).
- Tests and the CHANGELOG entry.

## Not in scope
- `internal/memory` (the library): unchanged.
- Any agent-side change, `~` expansion, a `--store` flag, a capability line.
- The other known gaps in Part III.4.

## Governing spec
- `docs/spec/agent-memory.md`: III.1, III.3 (error format), III.4, D32.

## Existing tests this unit supersedes
Edited in the lock commit; builders do not touch them:
- `cmd/memory/binary_test.go` `runBin`: strips `HOME`, `MEMORY_PARENT_DIR`, `MEMORY_DIR`,
  `AGENT_STATE_DIR` and `TZ` from the inherited environment, then sets `TZ=UTC`,
  `MEMORY_PARENT_DIR=<dir>/.agent` and `HOME=<dir>/home` unless the test's `env` names them; an
  `env` value of `envUnset` leaves a variable out. Every existing test keeps its store at
  `<dir>/.agent/memory`.
- `cmd/memory/resolve_bin_test.go`: `TestStoreResolution` (the `.env`, `AGENT_STATE_DIR` and
  `MEMORY_DIR` cases) deleted. `TestNoStoreFlag` kept.
- `cmd/memory/unit_test.go`: `TestDotenvValue` deleted.
- `cmd/memory/tools_test.go`: `TestToolFilesSchema` expects the new `prepend_args`; `toolCaller`
  substitutes `${AGENT_STATE_DIR}` with `<dir>/.agent`, as the agent would, and runs with a decoy
  `MEMORY_PARENT_DIR`, so the store must come from the file's `--parent-dir`.
- `cmd/memory/readme_test.go`: the live-context entry's `args` are the new triple.

At the lock commit, `TestToolFilesSchema` and `TestReadmeWiringGuide` fail (the tool files and the
README are this unit's to change); everything else passes.

## Acceptance criteria

Each runs the built binary in a fresh temp working dir `<W>` through `runBin`, with `P`, `Q`, `H`
distinct directories under `<W>`. "Creates nothing" means no new file or directory anywhere under
`<W>`.

1. **The flag wins, in both spellings.** With `MEMORY_PARENT_DIR=Q` and `HOME=H` set,
   `memory --parent-dir P init` and `memory --parent-dir=P init` each create `P/memory/notes/` and
   nothing under `Q` or `H`. `memory --parent-dir rel init` creates `<W>/rel/memory`. A following
   `memory --parent-dir P read` reads that store.
   *Excludes:* the environment beating the flag; supporting only one spelling; resolving a relative
   value against anything but the working directory.
2. **Then `MEMORY_PARENT_DIR`, then `HOME`, always plus `/memory`.** With no flag:
   `MEMORY_PARENT_DIR=Q`, `HOME=H` → `Q/memory`; `MEMORY_PARENT_DIR` unset, `HOME=H` → `H/memory`.
   *Excludes:* `HOME` beating `MEMORY_PARENT_DIR`; treating either as the store itself (`Q/notes`).
3. **Set but empty is an error, and so is nothing at all.** Each exits 1 with one stderr line,
   creates nothing, and the line contains the quoted text:
   - `MEMORY_PARENT_DIR=""`, `HOME=H`: `MEMORY_PARENT_DIR is set but empty`;
   - `MEMORY_PARENT_DIR` unset, `HOME=""`: `HOME is set but empty`;
   - `--parent-dir ""` and `--parent-dir=`: `--parent-dir`;
   - both variables unset, no flag: `--parent-dir`, `MEMORY_PARENT_DIR` and `HOME`.

   *Excludes:* an empty value falling through to the next source; a fallback to `.agent` or the
   working directory.
4. **Malformed flag use is an error.** `memory --parent-dir` (no value, nothing after it) and
   `memory --parent-dir P --parent-dir Q read` each exit 1, naming `--parent-dir`, creating nothing.
   *Excludes:* the last occurrence silently winning.
5. **After the verb it never changes the store.** On a store initialised at `<W>/.agent/memory`
   (the harness default) and with `X = <W>/x`:
   - `memory read --parent-dir X`, `memory index --parent-dir X`, `memory init --parent-dir X` and
     `memory write kettle --parent-dir X --description d --add a` each exit 1 naming
     `--parent-dir`; `X` does not exist afterwards, and no `kettle` node exists in the store;
   - `memory search --parent-dir X` exits 1 (two arguments);
   - emulating a tool call: `memory --parent-dir <W>/.agent read --parent-dir X` exits 1, and
     `X` does not exist.

   *Excludes:* scanning all of argv for `--parent-dir` (a model's later occurrence would win, or be
   honoured at all); `write`'s `--parent` matching `--parent-dir` by prefix.
6. **No agent awareness left.** With `HOME=H`, `MEMORY_PARENT_DIR` unset, `AGENT_STATE_DIR=<W>/s`
   exported, and a `.env` in `<W>` holding `MEMORY_PARENT_DIR=<W>/e` and `AGENT_STATE_DIR=<W>/e2`:
   `memory init` creates `H/memory` and nothing named `s`, `e` or `e2`. `memory` with no arguments
   prints usage naming `--parent-dir`, `MEMORY_PARENT_DIR` and `HOME`, and not `AGENT_STATE_DIR`,
   `.env` or `MEMORY_DIR`. No production file under `cmd/memory` mentions `AGENT_STATE_DIR`,
   `MEMORY_DIR` or `.env`.
   *Excludes:* leftover `.env` or agent-variable resolution.
7. **Tool files and README.** The superseded `TestToolFilesSchema`, `TestToolFilesRun` and
   `TestReadmeWiringGuide` pass: each shipped file's `prepend_args` are
   `["--parent-dir", "${AGENT_STATE_DIR}", "<verb>"]`, and, substituted as the agent substitutes
   them, they reach `<W>/.agent/memory` despite a decoy `MEMORY_PARENT_DIR`. The README's store
   paragraph states III.1's order and its empty-value rule.
   *Excludes:* tool files that rely on the environment for the store.

## Environment
- Clean-room copy of the committed baseline, no `.git`. Hermetic; no network beyond the Go module
  cache already present. Never touch a real `~/memory`: every test runs with `HOME` inside its temp
  dir (the harness does this).
- The QA gate is `CLAUDE.md`'s, with full uncapped lint: the tree is lint-clean and must stay so.

## Notes for the builder
- Keep it small. Production code should shrink: the `.env` reader goes, and resolution is three
  lookups. The global option needs only a few lines in `run` before the verb is looked up.
- `TestNoStoreFlag` must keep passing: `--store` after the verb is still an unknown flag.
