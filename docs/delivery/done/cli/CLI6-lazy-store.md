# CLI6 — The store is created on first use; `init` is gone

**Milestone:** cli  |  **Builds on:** CLI5

## Goal
There is no install step for the store. Any verb seeds `<parent>/memory/notes/` with the four
first-level nodes when `notes/` is missing, then does its job. The parent must already exist, so a
mistyped address fails loudly instead of starting a store somewhere unexpected. The `init` verb is
removed.

## Scope
- `docs/spec/agent-memory.md` **Part III.1** *The store is created on first use* (added in this
  unit's lock commit), III.2 (no `init` row), III.3's error line, III.4's older-agent sentence and
  known gaps, D19/D29 notes, D32.
- `cmd/memory`: every verb, after its argument checks and store resolution, checks the parent and
  seeds a missing `notes/` (one place, shared by all verbs). Remove the `init` verb (`initverb.go`,
  its `verbs()` entry, its usage line), `errNoNotes`, and `filesUnder`; keep `noArgs` (used by
  `index`). The usage text no longer mentions `init`.
- `internal/memory`: `SeedDefaults(memoryDir)` changes contract: it seeds `memoryDir/notes` when
  `notes` does not exist (Lstat), creating `memoryDir` if it is missing but never `memoryDir`'s
  parent (a missing parent is an error); existing entries in `memoryDir` are untouched. The temporary
  build directory (`.notes-seed-*`) is inside `memoryDir`, renamed to `notes`; if the rename fails
  because another process's `notes` now exists, that is success. Mode rules unchanged.
- `README.md`: the Usage block and paragraphs lose `init` (plain `memory read` gives `~/memory`; a
  paragraph says the first call seeds the store and the parent must exist; the error paragraph gives
  the parent error instead of the "run memory init" one). The wiring guide becomes three steps
  (install, PATH, copy the agent files), the third pointing at `agent/README.md`.
- `agent/README.md`: no seeding step (install is the copy commands; the store appears on the
  agent's first memory call); the older-agent paragraph names
  `parent dir ${AGENT_STATE_DIR} does not exist (from --parent-dir)`; the live-context paragraph
  drops "Before `init` it shows an error…".
- Tests and the CHANGELOG entry.

## Not in scope
- Recall or access logging of any kind (dropped by the operator).
- Part II's text (Part III overrides it), any other known gap, any agent-side change.
- Changing any verb's output: seeding is silent.

## Governing spec
- `docs/spec/agent-memory.md`: III.1 (whole, especially *The store is created on first use*),
  III.2, III.3, III.4, D19, D32.

## Existing tests this unit supersedes
Edited in the lock commit; builders do not touch them:
- `cmd/memory/init_bin_test.go` → renamed `snapshot_bin_test.go`, holding only the snapshot helpers;
  the four `TestInit*` tests deleted. `xInit` removed from `binary_test.go`; `seedDescriptions`
  moved to `read_bin_test.go`.
- `seeded()` (`read_bin_test.go`) creates `.agent/` and seeds with a first `index` call.
- `TestReadWithoutNotes`, `TestWriteWithoutNotes`, `TestSearchWithoutNotes`,
  `TestIndexWithoutNotes` deleted (a missing `notes/` is now seeded, not an error).
- `TestUsage` no longer requires "init" in the usage text.
- `agent_folder_test.go`: `TestInitTouchesOnlyNotes` and `TestUsageSaysInitSeedsNotes` deleted;
  `TestAgentReadmeNamesEveryFile` expects the new older-agent error and no init command;
  `TestAgentReadmeCommandsInstall` no longer expects a store after the copy commands;
  `TestReadmeAgentFolder` runs step 3's commands.
- `readme_test.go`: three wiring steps (install, path, agent); step 3 mentions `agent/README.md`.
- `resolve_bin_test.go`, `addressing_bin_test.go`: `init` replaced by `index` or `read`; parents
  the tests point at are created first; the `init --parent-dir` row of `TestParentDirAfterVerb` gone.
- `internal/memory/seed_test.go`: `TestSeedDefaults_EmptyMemoryDirSeedsNothing` deleted.

At the lock commit most `cmd/memory` binary tests fail (they rely on lazy seeding); the lead checked
them against a throwaway prototype of this unit, where all pass once the READMEs are updated.

## Acceptance criteria
`W` is a fresh temp dir; "the default parent" is `W/.agent` (runBin's `MEMORY_PARENT_DIR`).

1. **Any verb seeds a fresh store, silently.** With `W/.agent` existing and empty, each of these as
   the first call, in its own `W`: `read` (exit 0, root lists `people`, `projects`, `topics`,
   `user`); `index` (exit 0, `<memory_index>` listing the four); `search 'anything'` (exit 0,
   `matched="0"`); `write kettle --parent topics --description d --add a` (exit 0,
   `created="true"`). Stderr is empty each time. Afterwards `W/.agent` holds exactly
   `memory/notes/<stem>/<stem>.md` for the four seeds (plus `notes/topics/kettle/kettle.md` and
   `notes/.write.lock` after the `write`): no `.notes-seed-*`, nothing else. A `read` output on a
   freshly seeded store is byte-equal to the same `read` on a store seeded by an earlier call.
   *Excludes:* seeding only in `write`; printing a "seeded" notice; leaving the temp dir; a lock file
   from a read verb.
2. **The parent must exist.** `--parent-dir W/nope read` → exit 1, stderr exactly
   `parent dir W/nope does not exist (from --parent-dir)`, `W` unchanged. `MEMORY_PARENT_DIR=nope`
   (relative, cwd `W`) → `parent dir nope does not exist (from MEMORY_PARENT_DIR)`.
   `MEMORY_PARENT_DIR` unset, `HOME=W/h` missing → `… (from HOME)`. A parent that is a regular file
   → `parent dir <p> is not a directory (from <source>)`. `--parent-dir '${AGENT_STATE_DIR}' read`
   (the literal text) → `parent dir ${AGENT_STATE_DIR} does not exist (from --parent-dir)`, and no
   `${AGENT_STATE_DIR}` directory is created. A symlink to a directory works as the parent.
   *Excludes:* `MkdirAll` of the parent; making the path absolute in the message; naming the wrong
   source.
3. **The trigger is `notes/`.** (a) `W/.agent/memory/keep.txt` exists, no `notes/` → `read` seeds
   `notes/`; `keep.txt`'s bytes and mtime unchanged. (b) After seeding, `rm -r notes/people`, then
   `read` → root lists three stems; `people` is not recreated. (c) `notes/` exists and is empty →
   `index` prints nothing, exit 0, and nothing is created. (d) After seeding, `rm -r notes` → the
   next `read` seeds all four again.
   *Excludes:* triggering on `memory/` (CLI1's directory contract); topping up a partial `notes/`.
4. **Errors come before seeding.** On a fresh `W/.agent`: `read --depth x`, `index extra`,
   `write` (no stem), `search a b` each exit 1 and leave `W/.agent` empty. `W/.agent/memory` a
   regular file → any verb exits 1 with `W/.agent/memory is not a directory` (the path as resolved),
   changing nothing; `W/.agent/memory/notes` a regular file → `W/.agent/memory/notes is not a
   directory`.
   *Excludes:* seeding before argument validation; treating a non-directory `notes` as missing.
5. **Concurrent first calls.** Eight `read`s started together on one fresh `W/.agent` all exit 0
   with identical stdout (the four-stem root); afterwards `notes/` holds exactly the four seeds and
   `memory/` holds only `notes/`.
   *Excludes:* non-atomic seeding (mkdir then files in place), which lets a racing call read a
   partial `notes/` or fail on `EEXIST`.
6. **`init` is gone.** `memory init` → exit 1 with `unknown verb "init"` and the usage text; the
   usage text contains no `init`. `SeedDefaults` (library test): an existing empty `memoryDir` gets
   `notes/` with the four seeds; an existing `notes` is untouched; a `memoryDir` whose parent is
   missing is an error and creates nothing. The READMEs follow Scope.
   *Excludes:* keeping `init` as a hidden alias; `SeedDefaults` keeping the old `memory/`-level
   trigger.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- Argument parsing already happens before `loc.notesDir()` in every verb; keep that order, and put
  the parent check and seeding in the store lookup so all four verbs share it.
- The error for a missing parent needs the source of the parent, which `memoryDir()` currently
  discards.
- `HOME` is always set in tests by `runBin` (to `W/home`, which does not exist); tests that want
  `HOME` as the parent must create it.
- Keep the tree lint-clean (`golangci-lint run ./...`, full tree).

## Post-verification notes (2026-11-19)
- Built in one pass. Pre-filter green (race tests, full lint 0, tidy, release tests); no existing
  test touched. Production change about +60 net lines (`store.go`, `seed.go`), `initverb.go`
  deleted; new tests `cmd/memory/lazy_bin_test.go`, `internal/memory/seed_lazy_test.go`.
- Verifier: all six criteria PASS through the real binary (40 trials of 8 concurrent first reads, 0
  failures); 8/8 inverted assertions failed; 16 of 17 wrong implementations caught; tamper and scope
  clean; govulncheck clean.
- Recorded as known gaps (III.4), not reworked: argument errors found after the store is open
  (empty `search` query, invalid `write` stem) come after seeding; dangling-symlink and mode
  behaviour as built.
- Not covered by a test: the temp dir's location inside `memory/` (moving it to the parent survives
  the suite). Low value; left.
- Builder spec gaps resolved here: D30 got a CLI6 supersession note. Others (rename onto an empty
  `notes/` racing the seed, EACCES wording, midnight in the byte-equal read) judged not worth
  pinning.
- Process: the builder's compound command included `git status` (refused); the lead's banking step
  ran one `git status` by mistake (no effect).
