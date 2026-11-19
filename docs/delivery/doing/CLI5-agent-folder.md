# CLI5 — Everything an agent needs ships in `agent/`, installed by hand

**Milestone:** cli  |  **Builds on:** CLI4

## Goal
An operator wires `memory` into an agent by copying files from one folder, `agent/`, laid out like
the agent's state dir: the tool files, the guidance as the `prompt/` fragment `memory_usage.md`, and
the live-context entry. `memory init` seeds only notes. Release archives carry `agent/` beside the
binary, so the install works from a download alone.

## Scope
- `docs/spec/agent-memory.md` **Part III.4** (*The `agent/` folder*, as amended in this unit's lock
  commit), the III.2 `init` row, the §4.5 guidance text, D15's supersession note and D32.
- Move `tools/*.json` to `agent/tools/` (content unchanged). Add `agent/prompt/memory_usage.md` (the
  §4.5 fenced text, byte for byte), `agent/livecontext/memory_index.json` (the live-context entry
  now in the README's step 6, with `args` `["--parent-dir", "${AGENT_STATE_DIR}", "index"]`), and
  `agent/README.md`.
- `internal/memory`: `SeedDefaults` stops creating `protocol/`; `UsageText` goes.
- `cmd/memory`: `init`'s output and help reflect that it seeds `notes/` only.
- `scripts/release/build-binaries.sh`: each archive holds `memory_<tag>_<os>_<arch>/memory` and
  `memory_<tag>_<os>_<arch>/agent/` (the whole folder, as in the repo).
- `README.md`: the wiring guide becomes four steps (install, PATH, init, agent files), the fourth
  pointing at `agent/README.md`; step 1 says what an archive holds. `CLAUDE.md`'s CI paragraph says
  the archives carry `agent/`.
- Tests and the CHANGELOG entry.

## Not in scope
- Any change to the tool files' content, the verbs, or store resolution.
- Skills (none exist yet), an install command, any agent-side change.
- The other Part III.4 known gaps.

## Governing spec
- `docs/spec/agent-memory.md`: III.2 (`init`), III.4 (whole), §4.5's fenced guidance text, D15, D19,
  D32.

## Existing tests this unit supersedes
Edited in the lock commit; builders do not touch them:
- `internal/memory/seed_test.go`: `specUsageText`, `TestUsageTextIsTheSpecText` and
  `TestSeedDefaults_DeletedUsageStaysDeleted` deleted; `TestSeedDefaults_FreshCreatesUsageAndFirstLevel`
  became `TestSeedDefaults_FreshCreatesFirstLevelOnly` (memory/ holds only `notes/`).
- `cmd/memory/init_bin_test.go`: `TestInitSeedsAMissingStore` expects no `protocol/` and no
  `protocol/usage.md` in `created`; the "deleted usage.md" subtest of `TestInitNeverTopsUp` deleted.
- `cmd/memory/addressing_bin_test.go`: `isStore` checks two seeded notes instead of `usage.md`.
- `cmd/memory/tools_test.go`: reads the tool files from `agent/tools/`.
- `cmd/memory/readme_test.go`: the wiring guide has four steps (install, path, init, agent); the
  fourth mentions `agent/README.md`; the live-context entry checked is the shipped
  `agent/livecontext/memory_index.json`. The unused `firstJSONBlock` helper went.

At the lock commit `TestInitSeedsAMissingStore`, `TestReadmeWiringGuide`, `TestToolFilesSchema`,
`TestToolFilesRun` and `TestSeedDefaults_FreshCreatesFirstLevelOnly` fail; everything else passes.
Test files that plant a `memory/protocol/usage.md` as a stray fixture (errors, index, search, write
tests) are unaffected and stay.

## Acceptance criteria

1. **`init` seeds notes only.** On a fresh parent, `memory init` creates `memory/notes/` with the
   four first-level nodes and nothing else: no `memory/protocol/`, and nothing outside
   `<parent>/memory` (no `prompt/`). Its `created` list is exactly the four `notes/…` paths.
   *Excludes:* still seeding `protocol/usage.md`; writing the guidance into `<parent>/prompt/`.
2. **The guidance is the spec text, shipped.** `agent/prompt/memory_usage.md` is byte-equal to the
   fenced block after "shipped verbatim as `agent/prompt/memory_usage.md`" in §4.5 (a test reads the
   spec), and that text no longer says the index is in the system prompt.
   *Excludes:* a copy that drifts from the spec; keeping the stale sentence.
3. **The `agent/` folder is complete and nothing is left at the top.** `agent/` holds exactly
   `README.md`, `tools/memory_{index,read,search,write}.json`, `prompt/memory_usage.md` and
   `livecontext/memory_index.json`; there is no top-level `tools/`. The superseded tool and README
   tests pass against these paths.
   *Excludes:* leaving a second copy of the tool files at the top level.
4. **`agent/README.md` tells the operator what to do.** It names each file, the `<state dir>`
   subfolder it is copied to, and gives the copy commands; says the tool files and the live-context
   entry need an agent that substitutes `${AGENT_STATE_DIR}` (agent CHANGELOG 2026-11-19) and what an
   older agent shows; says to edit the path (never delete the `--parent-dir` pair) to share a store;
   says the guidance loads every turn and should match the tools installed; and says to run
   `memory --parent-dir <state dir> init` first. A test checks that it mentions each of the five
   shipped paths and its three destinations (`<state dir>/tools/`, `<state dir>/prompt/`,
   `<state dir>/livecontext/`).
   *Excludes:* a README that omits a file or a destination.
5. **Archives carry `agent/`.** `build-binaries.sh`, run into a temp dir, produces archives that each
   contain `memory_<tag>_<os>_<arch>/memory` plus every file under the repo's `agent/`, byte-equal,
   at `memory_<tag>_<os>_<arch>/agent/<same relative path>`, and nothing else. A new test checks one
   archive's full listing and contents (the existing `TestBuildBinaries` stays as it is).
   *Excludes:* archives with the binary only; a flattened layout.

## Environment
- Clean-room copy of the committed baseline, no `.git`. Hermetic. Never touch a real `~/memory` (the
  test harness pins `HOME` inside each temp dir).
- The QA gate is `CLAUDE.md`'s, with full uncapped lint. govulncheck needs the network; if it is
  denied, report it and move on (the lead runs it).

## Notes for the builder
- Mostly moves and docs. Production Go should shrink (`UsageText` and the `protocol/` seeding go).
- The README wiring guide keeps steps 1–3 and replaces 4–6 with one step pointing at
  `agent/README.md`; do not duplicate `agent/README.md`'s content there beyond the copy commands.
