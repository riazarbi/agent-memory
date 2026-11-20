# CLI12 — `move` and `rename`

**Milestone:** restructuring  |  **Builds on:** CLI11

## Goal
A node can change place and name. `memory move` relocates a node with its subtree, and
`memory rename` gives it a new stem and rewrites every link to it, so no link breaks.

## Scope
- `internal/memory`: the two operations, under D28's write rules, reusing CLI11's shared helpers
  (the lock, the fresh load, replacing several notes with roll back).
- `cmd/memory`: the verbs `move` and `rename`, their arguments, results, errors and exit codes;
  the usage text and package comment list them.
- `README.md`: the Usage section documents both verbs; the status line lists them.
- Tests and the CHANGELOG entry.

## Not in scope
- `move-block`, `split`, `merge` (CLI13, CLI14). Rewriting links to a *block* that changes node
  is CLI13; here a block link changes only because its node's stem changed.
- Tool files, skills, `memory_purpose.md`, `agent/README.md`, anything under `agent/` (CLI15).
- Any change to `read`, `write`, `search`, `index`, `lint`, `delete` or `retag`, or to the
  library's checks and reason texts.

## Governing spec
`docs/spec/agent-memory.md`: **III.8** (the verb table rows for `move` and `rename`, *Refusals*,
*`move`, exactly*, *`rename`, exactly*, *Postcondition*, *Results*); III.6 (the root, `/`); III.3;
D28; D25 (stem syntax, link forms, what is an edge); §4.3 (links resolve by stem).

## Prerequisite spec changes
III.8's *`move`, exactly* and *`rename`, exactly*, committed with this unit's lock.

## Existing tests this unit supersedes
None known. If a test pins the usage text or the verb list exactly, stop and report it.

## Acceptance criteria
All through the real binary. The fixture store `F` is the shipped starting tree plus, made with
`write` (`<X>` names the ID `write` assigned):

- `atlas-migration` under `people`, tag `atlas`, blocks `<A1>` "Cutover is 2026-12-01.", `<A2>`
  "Rollback plan is in [[atlas-migration#^<A1>]] and [[atlas-runbook]]."
- `atlas-runbook` under `atlas-migration`, block `<B1>` "Steps for [[atlas-migration|the cutover]]."
- `priya` under `people`, blocks `<P1>` "Leads [[atlas-migration]]; see
  [[atlas-migration#^<A1>|the date]].", `<P2>` "Wrote `[[atlas-migration]]` in the wiki and
  [[atlas-cutover]] is planned."
- `notes-misc` under `topics`, block `<N1>` "Nothing about Atlas here."

1. **`move` relocates the subtree and touches no file.** `move atlas-migration --parent projects`
   → exit 0, stderr empty, stdout `<memory_move stem="atlas-migration" from="people"
   parent="projects" path="projects/atlas-migration">`. `notes/projects/atlas-migration/` holds the
   note and `atlas-runbook/`; `notes/people/atlas-migration` is gone. Every note in the store has
   the bytes and mtime it had (at its new path where it moved). `read atlas-runbook` shows
   `path="projects/atlas-migration/atlas-runbook"`; `read priya` shows its links to
   `atlas-migration` resolving (no `dangling`). `lint --exit-zero` is as before the call.
   *Excludes:* copy-and-delete that changes mtimes; rewriting links on a move; moving the node
   without its children.
2. **`move` to and from the root, and a no-op.** `move atlas-migration --parent /` →
   `from="people" parent="/" path="atlas-migration"`, and `read` with no stem lists it among the
   top-level nodes. Then `move atlas-migration --parent people` puts it back
   (`from="/" parent="people"`). Then `move atlas-migration --parent people` again → exit 0, the
   same element with `from="people" parent="people"`, and no file or directory mtime in the store
   changes.
   *Excludes:* treating `/` as an unknown stem; an error or a rewrite on a no-op.
3. **`move` refusals change nothing.** Each → exit 1, stdout empty, one stderr line naming the
   argument as typed, store unchanged (bytes and mtimes): `move atlas-migration` (no `--parent`);
   `move atlas-migration --parent atlas-migration`; `move atlas-migration --parent atlas-runbook`
   (its own descendant); `move atlas-migration --parent nowhere` (unknown, names `--parent`);
   `move nowhere --parent people` (names `STEM`); `move / --parent people` (names `STEM`). With a
   plain file `notes/projects/atlas-migration` created by hand, `move atlas-migration --parent
   projects` → exit 1 naming `--parent`, saying an entry of that name is already there.
   *Excludes:* a cycle that makes the subtree vanish; overwriting an existing entry.
4. **`rename` rewrites every link form, and only links.** On `F`: `rename atlas-migration
   atlas-go-live` → exit 0, `<memory_rename from="atlas-migration" stem="atlas-go-live"
   links_rewritten="4">`. On disk: `notes/people/atlas-go-live/atlas-go-live.md` exists with
   `atlas-runbook/` beside it and no `atlas-migration` folder or note anywhere. `<A2>` reads
   "Rollback plan is in [[atlas-go-live#^<A1>]] and [[atlas-runbook]]."; `<B1>` reads "Steps for
   [[atlas-go-live|the cutover]]."; `<P1>` reads "Leads [[atlas-go-live]]; see
   [[atlas-go-live#^<A1>|the date]]."; `<P2>` is unchanged, backticked text included. The four
   links are one in `<A2>`, one in `<B1>` and two in `<P1>`. Every other byte of the three rewritten notes is unchanged; `<A1>`
   keeps its ID; `notes-misc.md` and the starting-tree notes keep bytes and mtime; the tag
   `atlas` and the description are unchanged.
   *Excludes:* a text replace that rewrites the backticked `[[atlas-migration]]`; dropping
   aliases or block IDs; counting blocks or notes instead of links; rewriting notes that hold no
   link.
5. **After a rename the old stem is gone and the new one works everywhere.** Following
   criterion 4: `read atlas-migration` is an unknown-stem error; `read atlas-go-live` shows both
   blocks, the child `atlas-runbook`, and backlinks from `priya` and `atlas-runbook`;
   `search atlas-go-live` finds it; `lint --exit-zero` has no error and no warning it did not have
   before. Second-order: `rename atlas-go-live atlas-cutover` → `links_rewritten="4"`, and the
   `[[atlas-cutover]]` in `<P2>`, dangling until now, resolves in `read priya` and is no longer a
   lint warning. Then `move atlas-cutover --parent projects` and `delete atlas-cutover
   --recursive` both work on the renamed node, the delete reporting the links in `priya` as
   dangling under the name `atlas-cutover`.
   *Excludes:* renaming the folder but not the note (or the reverse); leaving stale state that
   breaks the next verb.
6. **`rename` refusals change nothing.** Each → exit 1, stdout empty, one stderr line naming the
   argument as typed, store unchanged: `rename atlas-migration` (no `NEW`); `rename atlas-migration
   Atlas_Go` (invalid stem, names `NEW`); `rename atlas-migration priya` (in use, names `NEW`);
   `rename atlas-migration atlas-migration` (names `NEW`); `rename nowhere x-y` (names `STEM`);
   `rename / x-y` and `rename priya /` (name `STEM`, `NEW`); `rename a b c`. With a plain file
   `notes/people/atlas-go-live` created by hand, `rename atlas-migration atlas-go-live` → exit 1
   naming `NEW`.
   *Excludes:* a rename that merges into an existing node; partial rewrites before a refusal.
7. **`rename` is all or nothing.** Make `notes/people/priya/priya.md` unwritable in a way that
   makes its replacement fail (the library tests may inject the failure; through the binary, a
   read-only `notes/people/priya/` directory). `rename atlas-migration atlas-go-live` → non-zero
   exit, and afterwards the store is byte-for-byte what it was: the folder and note have their
   old names, `<A2>` and `<B1>` still link to `atlas-migration`, and `lint` is as before.
   *Excludes:* leaving some notes pointing at a stem that no longer exists.
8. **Both verbs obey the lock and add no lint error.** While another process holds
   `notes/.write.lock`, `move atlas-migration --parent projects` and `rename atlas-migration
   atlas-go-live` each wait and then exit 75 with the "busy" line, writing nothing. After every
   successful call in criteria 1–5, `lint` reports no error that was not there before the call.
   The usage text lists `move` and `rename`; `TestAgentFolderIsComplete` passes unchanged.
   *Excludes:* writing without the lock.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- The QA gate is in `CLAUDE.md`. The whole tree stays lint-clean.
- CLI11 left the pieces to reuse: `locked`, `reload`, `replaceNotes` and `newErrors` in
  `internal/memory/restructure.go`, and `scanLinkSpans`, which returns each link's position. Link
  targets are rewritten in the note's source bytes at those positions, not by re-encoding the
  note. Do not write a second link parser.
- `cmd/memory/restructure_busy_bin_test.go` shows how the lock is held from another process.
- The library does not log, and takes no new dependency.
