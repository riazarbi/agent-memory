# CLI13 — `move-block`, and `rename` checks its new stem as `write` does

**Milestone:** restructuring  |  **Builds on:** CLI11, CLI12

## Goal
A single claim can change node. `memory move-block` moves one block to another existing node,
keeping its ID, and rewrites every link to it. And `rename` refuses a new stem in every case
where `write` would refuse to create it.

## Scope
- `internal/memory`: the `move-block` operation, under D28's write rules, reusing CLI11's and
  CLI12's helpers (the lock, the fresh load, replacing several notes with restore, block removal
  by byte surgery, link rewriting at `scanLinkSpans` positions, the same-skipped-folder
  postcondition check).
- `internal/memory`: `rename`'s check of `NEW` extended to D28's create rules (any folder in the
  tree carrying the stem, loaded or not; a name differing only by letter case).
- `cmd/memory`: the verb `move-block`, its arguments, result, errors and exit codes; the usage
  text and package comment list it.
- `README.md`: the Usage section documents `move-block` and the stricter `rename`; the status
  line lists the verb.
- Tests and the CHANGELOG entry.

## Not in scope
- `split` and `merge` (CLI14).
- Tool files, skills, `memory_purpose.md`, `agent/README.md`, anything under `agent/` (CLI15).
- Any other change to `rename`, and any change to `read`, `write`, `search`, `index`, `lint`,
  `delete`, `retag` or `move`, or to the library's checks and reason texts. Lifting existing
  lines into a shared function, with no behaviour change, is allowed.

## Governing spec
`docs/spec/agent-memory.md`: **III.8** (the `move-block` row, *Refusals*, *`move-block`,
exactly*, *`rename`, exactly* — the paragraph "`NEW` is checked as strictly as `write` checks a new
stem", *Postcondition*, *Results*); D28 (*Create*: the stem rules; the write rules); D27 (how an
added block is laid out); D25 (link forms, block IDs); III.3; III.7 (the duplicate-ID warning).

## Prerequisite spec changes
III.8's *`move-block`, exactly*, committed with this unit's lock. The `rename` ruling is already
in III.8 (CLI12's banking).

## Existing tests this unit supersedes
None known. No CLI12 test renames onto a stem held by an unloaded folder elsewhere in the tree.
If one does, or a test pins the usage text or verb list exactly, stop and report it.

## Acceptance criteria
All through the real binary. The fixture store `F` is the shipped starting tree plus, made with
`write` (`<X>` names the ID `write` assigned):

- `priya` under `people`, blocks `<P1>` "Priya leads platform.", `<P2>` "Atlas cutover is
  2026-12-01; rollback is [[atlas-runbook]].", `<P3>` "The date in [[priya#^<P2>]] is firm."
- `atlas-cutover` under `projects`, blocks `<C1>` "Owner is [[priya]].", `<C2>` "See
  [[priya#^<P2>|the date]] and `[[priya#^<P2>]]`."
- `atlas-runbook` under `atlas-cutover`, block `<R1>` "Follows [[priya#^<P2>]]."
- `notes-misc` under `topics`, block `<N1>` "Nothing about Atlas here."

1. **The block moves with its ID and text, and nothing else in either note changes.**
   `move-block <P2> --to atlas-cutover` → exit 0, stderr empty, stdout `<memory_move_block
   id="<P2>" from="priya" stem="atlas-cutover" links_rewritten="3">`. `read atlas-cutover` lists
   blocks `<C1>`, `<C2>`, `<P2>` in that order, `<P2>` reading "Atlas cutover is 2026-12-01;
   rollback is [[atlas-runbook]]."; `read priya` lists `<P1>`, `<P3>`. `priya.md` equals its old
   bytes minus `<P2>`'s paragraph and its separating blank line, with `<P3>`'s link target
   changed; `atlas-cutover.md` equals its old bytes with `<C2>`'s first link target changed, then
   one blank line, then the line `Atlas cutover is 2026-12-01; rollback is [[atlas-runbook]].
   ^<P2>`. `notes-misc.md` and the starting-tree notes keep bytes and mtime. **The target is not
   tidied:** on a fresh `F`, edit `atlas-cutover.md` by hand to add, between `<C1>` and `<C2>`, a
   paragraph "Hand-written, no ID." preceded by three blank lines; then `move-block <P2> --to
   atlas-cutover` leaves that paragraph without an ID and the three blank lines in place, the
   note being its old bytes plus the one link-target change and the appended block.
   *Excludes:* a new ID for the moved block; re-encoding either note; giving IDs to the target's
   ID-less paragraphs or collapsing its blank lines (what `write --add` does); inserting the
   block anywhere but last; rewriting notes that hold no link to the block.
2. **Every link to the block follows it; links to the node do not.** After criterion 1: `<P3>`
   reads "The date in [[atlas-cutover#^<P2>]] is firm."; `<C2>` reads "See
   [[atlas-cutover#^<P2>|the date]] and `[[priya#^<P2>]]`." (the backticked text untouched);
   `<R1>` reads "Follows [[atlas-cutover#^<P2>]]."; `<C1>` still reads "Owner is [[priya]]."
   Those are the three rewritten links. `read atlas-cutover --block <P2>` shows backlinks from
   `priya`, `atlas-cutover` and `atlas-runbook`. `lint --exit-zero` has no error and no warning
   it did not have before.
   *Excludes:* rewriting `[[priya]]` node links; a text replace that changes the backticked
   text; missing links in the old node or in the target node; counting notes instead of links.
3. **A block that links to itself, and a node emptied by the move.** On `F`, add to `notes-misc`
   a block `<N2>` "This claim, [[notes-misc#^<N2>]], stands alone." (two writes: add, then
   replace with its own ID). `move-block <N2> --to priya` → `links_rewritten="1"`, and in
   `priya` the block reads "This claim, [[priya#^<N2>]], stands alone." Then `move-block <N1> --to
   priya` leaves `notes-misc` with no blocks: `read notes-misc` still shows the node, `lint` has
   no error, and `delete notes-misc` then succeeds with `blocks="0"`.
   *Excludes:* leaving the moved block's self-link pointing at the old node; refusing to empty a
   node; deleting the emptied node.
4. **Second-order: moved blocks keep working with the other verbs.** After criterion 1:
   `move-block <P2> --to priya` moves it back (`from="atlas-cutover" stem="priya"
   links_rewritten="3"`), and `priya` then lists `<P1>`, `<P3>`, `<P2>` (last, not its old
   place). `rename priya priya-shah` → every `[[priya#^<P2>…]]` link now reads
   `[[priya-shah#^<P2>…]]`. `delete priya-shah --block <P2>` reports exactly the three links to
   the block as dangling.
   *Excludes:* stale link targets after a round trip; restoring the old position.
5. **Refusals change nothing.** Each → exit 1, stdout empty, one stderr line naming the argument
   as typed, store unchanged (bytes and mtimes): `move-block` (no ID); `move-block <P2>` (no
   `--to`); `move-block m-zzzzzz --to priya` (names `ID`); `move-block <P2> --to nowhere`
   (names `--to`, near matches if any); `move-block <P2> --to /` (names `--to`);
   `move-block <P2> --to priya` (already there, names `--to`); `move-block <P2> --to
   atlas-cutover extra`. With `<P1>`'s ID pasted by hand onto a paragraph in `notes-misc.md` (so
   `lint` warns the ID is used twice), `move-block <P1> --to atlas-cutover` → exit 1 naming `ID`
   and saying it is carried by more than one block.
   *Excludes:* moving the first of two blocks sharing an ID; creating a node for an unknown
   `--to`.
6. **All or nothing.** With `notes/projects/atlas-cutover/atlas-runbook/` made read-only so
   `atlas-runbook.md` cannot be replaced, `move-block <P2> --to atlas-cutover` → non-zero exit,
   and the store is byte-for-byte what it was: `<P2>` is in `priya` once and in `atlas-cutover`
   not at all, and every link still points at `priya`. The library tests inject a failure at
   each step (removing from the old note, adding to the target, each linking note, the reload).
   *Excludes:* a block present in both notes, or in neither, after a failure.
7. **`rename` refuses a new stem that `write` would refuse to create.** On `F`, with a folder
   `notes/topics/ghost/` holding a `ghost.md` that has no frontmatter (it does not load):
   `rename priya ghost` → exit 1 naming `NEW`, store unchanged; `write ghost --parent people
   --description x` is refused for the same reason (the existing behaviour this matches). With
   instead a folder `notes/topics/Atlas-Go/` (not a valid stem, so it does not load):
   `rename priya atlas-go` → exit 1 naming `NEW` and saying the name differs only by case. With
   neither, `rename priya atlas-go` succeeds. Every CLI12 test passes unchanged.
   *Excludes:* checking only loaded stems; a case check that depends on the filesystem.
8. **The lock, lint and usage.** While another process holds `notes/.write.lock`, `move-block
   <P2> --to atlas-cutover` waits and then exits 75 with the "busy" line, writing nothing. After
   every successful call in criteria 1–4, `lint` reports no error that was not there before. The
   usage text lists `move-block`; `TestAgentFolderIsComplete` passes unchanged.
   *Excludes:* writing without the lock.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- The QA gate is in `CLAUDE.md`. The whole tree stays lint-clean.
- Pieces to reuse, all in `internal/memory`: `locked`, `reload`, `replaceAll`/`replaceNotes` and
  the restore they return (`restructure.go`); the block removal behind `delete --block`
  (`delete.go`); the link-target rewriting behind `rename` (`rename.go`, `cutBlock`); the
  `unsound` postcondition check (`move.go`); the writer's existing create check for a stem
  (the one `write` uses) for criterion 7. Do not write a second parser or a second collision
  rule.
- The target note is appended to by byte surgery, not through the encoder that `write --add`
  uses: that encoder also adopts ID-less paragraphs and collapses blank lines, which this verb
  must not do (III.8, *`move-block`, exactly*).
- The old note and the target note can each also hold links to the moved block, so one note may
  need two kinds of edit in the same replace.
- `cmd/memory/move_rename_busy_bin_test.go` shows how the lock is held from another process.
- The library does not log, and takes no new dependency.

## Rework 1 (2026-11-20)
The first build passed the gate but ran the target note through `write`'s layout code, as the
locked spec's "exactly as `write --add` adds a block" told it to. That silently gave IDs to
hand-written paragraphs in the target and collapsed its blank lines, unlike `delete --block`,
`retag` and `rename`. Operator ruling: append by byte surgery and leave the rest of the target
untouched. III.8 and criterion 1 are amended; the first build is discarded unverified (its
verifier run was lost to a machine restart) and a fresh builder runs from this commit.

## Post-verification notes (2026-11-20)
- Second build, from the reworked spec, in one pass. Production: `internal/memory/moveblock.go`
  (new); `restructure.go` (`rewritten`/`noteChange`: per-block link cuts, drop one block, append
  one block, then verify; `withLastBlock` is the append); `rename.go` (`refusal` calls
  `findFolders`, the walk `write`'s create check uses; `retargetCuts`); `delete.go`
  (`withoutBlock` is one call of `rewritten`); `cmd/memory/moveblock.go` (new), `main.go`. Tests:
  `internal/memory/move_block_test.go`, `cmd/memory/move_block_bin_test.go`,
  `move_block_busy_bin_test.go`. No existing test, document or `agent/` file was touched
  (confirmed with `git diff`).
- Verifier: all eight criteria PASS through the real binary; 19 of 20 mutants killed. The
  survivor removes the changed-on-disk check between the replace and the reload, which no
  `move-block` test covers; recorded as a known gap, not rebuilt.
- Operator accepted 2026-11-20. Spec settled at banking, as built (III.8): trailing blank lines
  in the target are kept; the empty-target exception; ID-on-its-own-line normalised; warnings
  travel with the block; the duplicate-ID check sees loaded notes only.
- `TestUsageListsMoveAndRename` (CLI12) pins the usage sentence "write, delete, retag, move and
  rename take the store's write lock". The builder kept it and added ", and so does move-block".
  CLI14 meets the same sentence.
