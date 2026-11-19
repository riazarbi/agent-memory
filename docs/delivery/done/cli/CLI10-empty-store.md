# CLI10 — Every verb creates a missing store; an empty index says so

**Milestone:** cli  |  **Builds on:** CLI9

## Goal
`index` on a missing or empty store no longer prints nothing. Every verb, once its arguments pass,
creates the empty `memory/notes/` of a missing store, and `index` with no loadable node prints the
trust line and `<empty>No nodes yet.</empty>`, so a live-context entry tells the model its memory
exists and is empty.

## Scope
- `docs/spec/agent-memory.md` III.1 *A missing store is created empty*, III.2's `index` row, III.7's
  missing-store and writes-nothing bullets, III.4 known gaps, the D30 note (all in this unit's lock
  commit).
- `cmd/memory`: `read`, `search`, `index` and `lint` create `memory/` and `memory/notes/` as `write`
  already does (after argument checks; 0755 forced on what is created; a concurrent creator is fine;
  no lock file); `index` renders the empty index when no node loads. Usage text and package comment
  updated where they say a missing store is not created.
- `README.md` and `agent/README.md`: sentences saying read verbs create nothing, or that `index`
  prints nothing on an empty store, corrected.
- Tests and the CHANGELOG entry.

## Not in scope
- Any change to the parent-must-exist rule, the non-directory errors, or argument ordering.
- Any other change to the index layout, budget or notice; any library change.

## Governing spec
- `docs/spec/agent-memory.md`: III.1 (whole), III.2 (`index`), III.7, D30.

## Existing tests this unit supersedes
Edited in the lock commit; builders do not touch them:
- `index_bin_test.go`: new `wantEmptyIndex` and `emptyStoreTree`; `TestIndexEmpty` expects the empty
  index for an empty `notes/` and for one with no loadable node.
- `lazy_bin_test.go` `TestEmptyNotesIsNotSeeded`, `typology_bin_test.go` `TestMissingStoreIsEmpty`:
  `index` prints the empty index; a missing store leaves exactly `memory/` and `memory/notes/`.
- `lint_bin_test.go` `TestLintCleanStore`, `lint_tool_test.go` `TestLintExitZeroKeepsTheReport`: a
  missing store leaves exactly `memory/` and `memory/notes/`.

At the lock commit those five tests fail; the lead checked the edits against a throwaway prototype,
where the suite passes.

## Acceptance criteria
`W` has an empty `W/.agent` (runBin's default parent) unless stated.

1. **The empty index.** On `W`, `index` → exit 0, stderr empty, stdout exactly
   `<memory_index>` + the trust line + `\n  <empty>No nodes yet.</empty>\n</memory_index>\n`. The same
   bytes for an existing empty `notes/`, and for a `notes/` whose only folder is a node that does
   not load. No `<tags>`. On the starting tree the index is unchanged from CLI9 (no `<empty>`).
   *Excludes:* printing nothing; a bare element without the trust line; `<empty>` on a store with
   nodes.
2. **Every verb creates a missing store.** For each of `read`, `read /`, `index`, `search anything`,
   `lint`, `lint --exit-zero` as the first call on a fresh `W` (and on `W` with `memory/` but no
   `notes/`): exit 0, its empty-store result (III.1), and `W/.agent` then holds exactly `memory/` and
   `memory/notes/`, both 0755 under umask 077, with no lock file. `read kettle` exits 1 (unknown stem)
   and also leaves the empty store. A second call changes nothing (tree snapshot equal).
   *Excludes:* read verbs creating nothing (CLI7); creating a lock file; creating nodes.
3. **Errors still come first.** `read --depth x`, `index extra`, `search a b`, `lint extra` on a fresh
   `W` leave `W/.agent` empty; a missing parent and a non-directory `memory/` or `notes` still fail
   with III.1's lines and create nothing (the existing CLI6/CLI7 tests keep passing).
   *Excludes:* creating the store before argument checks or after a resolution error.
4. **Concurrent first calls.** Eight mixed first calls (`read`, `index`, `search x`, `lint`, two of
   each) started together on one fresh `W` all exit 0 with empty stderr, and leave exactly
   `memory/` and `memory/notes/`.
   *Excludes:* a racing creator failing on `EEXIST`.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- `makeNotesDir()` already does what `write` needs; the read verbs use `notesDir()`, which creates
  nothing. Keep one shared path.
- `index.go` returns `nil, nil` when no first-level node loads; that is the case to render.
- `indexTrust` is the trust line constant; the lock-commit tests use it.
- Keep the tree lint-clean (`golangci-lint run ./...`, full tree).

## Post-verification notes (2026-11-19)
- Built in one pass. Pre-filter green; no existing test touched. Production change: `makeNotesDir`
  folded into `notesDir` (every verb creates the empty store after its argument checks); `index`
  renders `indexXML{Trust, Empty}` when no first-level node loads. New tests
  `cmd/memory/empty_store_bin_test.go`.
- Verifier: all four criteria PASS through the real binary; stores with nodes byte-identical to
  CLI9's index (including a 250-node overflowing store); 200 rounds of 8 concurrent first calls,
  0 failures; every wrong-reading mutant killed (one equivalent survivor, `Mkdir`→`MkdirAll`); 9/9
  inverted assertions failed; tamper and scope clean; govulncheck clean.
- Recorded as III.4 known gaps: malformed `search` queries fail after store creation; the install
  guard skips the starting tree once the empty store exists (install the tree first); a read-only
  parent with no store now fails.
- Lead edit at banking: one README sentence whose "and so" did not follow, reworded.
- `wantEmptyIndex` (lock commit) is built from the production `indexTrust`; the builder's test uses
  an independent literal.
- Process: the builder's gate command ended with `git status --short | head -0` (read-only, errored).
