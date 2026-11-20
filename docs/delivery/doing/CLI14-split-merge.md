# CLI14 — `split` and `merge`

**Milestone:** restructuring  |  **Builds on:** CLI11, CLI12, CLI13

## Goal
A node that covers two subjects can be divided, and two nodes about one subject can be joined,
each in one call. `memory split` moves named blocks into a new node placed where the caller says.
`memory merge` folds one node into another: blocks, children, links and tags.

## Scope
- `internal/memory`: the two operations, under D28's write rules, built from the pieces CLI11–CLI13
  left (the lock and fresh load, replacing several notes with restore, block removal and block
  append by byte surgery, link-target rewriting, the create check `write` uses, the node delete,
  the same-skipped-folder postcondition check).
- `cmd/memory`: the verbs `split` and `merge`, their arguments, results, errors and exit codes;
  the usage text and package comment list them.
- `README.md`: the Usage section documents both verbs; the status line lists them.
- Tests and the CHANGELOG entry.

## Not in scope
- Tool files, skills, `memory_purpose.md`, `agent/README.md`, anything under `agent/` (CLI15).
- Any change to `read`, `write`, `search`, `index`, `lint`, `delete`, `retag`, `move`, `rename`
  or `move-block`, or to the library's checks and reason texts. Lifting existing lines into a
  shared function, with no behaviour change, is allowed.
- Copying tags or properties to a split-off node; a `--tag` flag on `split`; choosing a block's
  position.

## Governing spec
`docs/spec/agent-memory.md`: **III.8** (the `split` and `merge` rows, *`split` says which way*,
*Refusals*, *`split`, exactly*, *`merge`, exactly*, *`move-block`, exactly* (how a block is
appended), *`delete`, exactly* (how a node folder is removed), *`rename`, exactly* (the strict
check of `NEW`), *Postcondition*, *Results*); D28 (*Create*); D27; D26; D25; III.6 (`/`); III.3.

## Prerequisite spec changes
III.8's *`split`, exactly* and *`merge`, exactly*, committed with this unit's lock.

## Existing tests this unit supersedes
Edited by the delivery lead in this unit's lock commit; the builder edits no existing test.
- `cmd/memory/move_rename_bin_test.go`, `TestUsageListsMoveAndRename`: the pinned usage sentence
  `"write, delete, retag, move and rename take the store's write lock"` is replaced by
  `"write lock"`.
- `cmd/memory/move_block_bin_test.go`, `TestUsageListsMoveBlock`: `"so does move-block"` is
  replaced by `"write lock"`.

The usage text's lock sentence is rewritten by this unit to name every writing verb once: `write`,
`delete`, `retag`, `move`, `rename`, `move-block`, `split` and `merge`.

## Acceptance criteria
All through the real binary. The fixture store `F` is the shipped starting tree plus, made with
`write` (`<X>` names the ID `write` assigned):

- `priya` under `people`, tags `platform`, `atlas`, blocks `<P1>` "Priya leads platform.", `<P2>`
  "Atlas cutover is 2026-12-01.", `<P3>` "Atlas rollback owner is Sam; see [[priya#^<P2>]].",
  `<P4>` "Prefers async updates."
- `outbox-rules` under `topics`, tags `outbox`, `atlas`, blocks `<O1>` "Cc Priya on urgent
  replies.", `<O2>` "Dates come from [[priya#^<P2>|the cutover date]] and [[priya#^<P3>]]."
- `outbox-etiquette` under `topics`, tags `outbox`, `etiquette`, block `<E1>` "Be brief. See
  [[outbox-rules]] and [[outbox-rules#^<O1>]]."
- `outbox-archive` under `outbox-rules`, block `<A1>` "Old rules, replaced by [[outbox-rules|the
  rules]]."

1. **`split` puts the named blocks in a new node where the caller says.** `split priya --into
   atlas-cutover --parent projects --description "The Atlas cutover: date and owners" --block
   <P3> --block <P2>` → exit 0, stderr empty, stdout `<memory_split from="priya"
   stem="atlas-cutover" parent="projects" path="projects/atlas-cutover" blocks="2" left="2"
   links_rewritten="3">`. `read atlas-cutover` shows that description, no tags, and blocks `<P3>`
   then `<P2>` (the order given, not file order), `<P3>` reading "Atlas rollback owner is Sam; see
   [[atlas-cutover#^<P2>]]." `read priya` shows `<P1>`, `<P4>`, its tags unchanged. `<O2>` reads
   "Dates come from [[atlas-cutover#^<P2>|the cutover date]] and [[atlas-cutover#^<P3>]]."
   `priya.md` equals its old bytes minus the two paragraphs and their separating blank lines.
   `atlas-cutover.md` is byte-equal to the note that `write atlas-cutover --parent projects
   --description "The Atlas cutover: date and owners" --add <text of P3, rewritten> --add <text
   of P2>` makes on a scratch store, once that note's two assigned IDs are replaced by `<P3>` and
   `<P2>`. Notes holding no link to a moved block keep bytes and mtime. `lint --exit-zero` has no
   error and no warning it did not have before.
   *Excludes:* file order instead of argv order; new IDs; copying tags; a default parent;
   leaving a moved block's own link pointing at the old node.
2. **Every direction, and an emptied node.** On fresh `F`: `split priya --into priya-atlas
   --parent priya --description "Priya's Atlas work" --block <P2>` → `parent="priya"
   path="people/priya/priya-atlas" blocks="1" left="3" links_rewritten="2"`, and `read priya`
   lists `priya-atlas` as a child, `<P3>` now reading "…see [[priya-atlas#^<P2>]]." On fresh `F`:
   the same split with `--parent /` → `parent="/" path="priya-atlas"`, listed among the top-level
   nodes. On fresh `F`: naming all four blocks → `blocks="4" left="0"`, and `read priya` still
   shows the node, with no blocks.
   *Excludes:* refusing the node itself as parent; refusing `/`; refusing to empty the node.
3. **`split` refusals change nothing.** Each → exit 1, stdout empty, one stderr line naming the
   argument as typed, store unchanged (bytes and mtimes), and no folder for the new stem
   anywhere: the call of criterion 1 with `--into` dropped; with `--parent` dropped; with
   `--description` dropped; with both `--block` dropped; with `--block <O1>` (not a block of
   `priya`: names `--block`); with `--block <P2> --block <P2>`; with `--into Atlas_Cutover`; with
   `--into outbox-rules` (in use); with `--parent nowhere`; `split nowhere …` (names `STEM`);
   `split / …`; `--into /`. With a folder `notes/topics/ghost/ghost.md` that has no frontmatter,
   `--into ghost` → exit 1 naming `--into`.
   *Excludes:* creating the node before checking the blocks; a split that merges into an existing
   node.
4. **`merge` folds blocks, children, links and tags into the target.** On `F`: `merge
   outbox-rules --into outbox-etiquette` → exit 0, `<memory_merge from="outbox-rules"
   stem="outbox-etiquette" blocks="2" children="1" tags_added="1" links_rewritten="3">`.
   `read outbox-etiquette` shows its own description, tags `outbox`, `etiquette`, `atlas`, blocks
   `<E1>`, `<O1>`, `<O2>` and the child `outbox-archive`; `<E1>` reads "Be brief. See
   [[outbox-etiquette]] and [[outbox-etiquette#^<O1>]]."; `<O2>` is unchanged; `<A1>` reads "Old
   rules, replaced by [[outbox-etiquette|the rules]]." `read outbox-archive` shows
   `path="topics/outbox-etiquette/outbox-archive"`. `read outbox-rules` is an unknown stem and
   `notes/topics/outbox-rules` is gone, with no hidden leftover. `priya.md` and the starting-tree
   notes keep bytes and mtime. `lint --exit-zero` has no error and no warning it did not have
   before.
   *Excludes:* leaving links pointing at the removed node; dropping the children; duplicating
   the shared tag; replacing the target's description; putting the blocks first.
5. **`merge` carries what it cannot see, merges upward, and refuses cleanly.** On fresh `F` with
   a file `notes/topics/outbox-rules/notes.txt` and a folder
   `notes/topics/outbox-rules/broken/broken.md` that has no frontmatter: the merge of criterion 4
   succeeds, both now sit in `notes/topics/outbox-etiquette/`, and `lint` shows the same number of
   errors as before. With instead `notes/topics/outbox-etiquette/notes.txt` also present: exit 1
   naming `notes.txt`, store unchanged. On fresh `F`: `merge outbox-archive --into outbox-rules`
   (a child into its parent) → `blocks="1" children="0" tags_added="0" links_rewritten="0"`, and
   `outbox-rules` lists `<O1>`, `<O2>`, `<A1>`. Each of these → exit 1, one line naming the
   argument, store unchanged: `merge outbox-rules` (no `--into`); `merge outbox-rules --into
   outbox-rules`; `merge outbox-rules --into outbox-archive` (its own descendant); `merge nowhere
   --into priya`; `merge priya --into nowhere`; `merge / --into priya`; `merge priya --into /`.
   *Excludes:* deleting unloaded content with the folded-in node; overwriting an entry in the
   target's folder; a parent merged into its child.
6. **Both are all or nothing.** With `notes/topics/outbox-rules/` made read-only (so `<O2>`'s
   note cannot be replaced), the split of criterion 1 → non-zero exit, the store byte-for-byte
   what it was, and no `atlas-cutover` folder. With
   `notes/topics/outbox-rules/outbox-archive/` made read-only (so `<A1>`'s note cannot be
   replaced), the merge of criterion 4 → non-zero exit, the store byte-for-byte what it was,
   every folder under its old name. The library tests inject a failure at each step of each verb.
   *Excludes:* a half-made node; blocks in both notes or in neither; children moved but links
   not rewritten.
7. **Second-order: the two undo each other and work with the other verbs.** After criterion 4:
   `split outbox-etiquette --into outbox-urgent --parent outbox-etiquette --description "Urgent
   replies" --block <O1>` → `blocks="1" left="2" links_rewritten="1"`, and `<E1>` reads "…and
   [[outbox-urgent#^<O1>]]." Then `merge outbox-urgent --into outbox-etiquette` →
   `blocks="1" children="0" tags_added="0" links_rewritten="1"`, `outbox-etiquette` lists `<E1>`,
   `<O2>`, `<O1>`, and `<E1>` reads "…and [[outbox-etiquette#^<O1>]]." again. Then `rename
   outbox-etiquette outbox-guide`, `move outbox-guide --parent people` and `delete outbox-guide
   --recursive` each succeed.
   *Excludes:* stale link targets after a round trip; state that breaks the next verb.
8. **The lock, lint and usage.** While another process holds `notes/.write.lock`, the split of
   criterion 1 and the merge of criterion 4 each wait and then exit 75 with the "busy" line,
   writing nothing. After every successful call in criteria 1, 2, 4, 5 and 7, `lint` reports no
   error that was not there before. The usage text lists `split` and `merge`, and one sentence
   names all eight writing verbs as taking the write lock. `TestAgentFolderIsComplete` passes
   unchanged.
   *Excludes:* writing without the lock.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- The QA gate is in `CLAUDE.md`. The whole tree stays lint-clean.
- Everything these verbs do is a composition of what CLI11–CLI13 built in `internal/memory`:
  `rewritten`/`noteChange` (link cuts, drop a block, append a block), `replaceAll` and its
  restore, `locked`, `reload`, `unsound`, the node delete's rename-aside, `findFolders` and the
  create path `write` uses, `rewriteFrontmatter` for tags. Compose them inside one lock; do not
  call the verbs one after another, and do not write a second parser, collision rule or encoder.
- A note can need several kinds of edit in one replace: the target of a merge gains blocks, has
  its own links to `FROM` retargeted, and may gain tags.
- `cmd/memory/move_block_busy_bin_test.go` shows how the lock is held from another process.
- The library does not log, and takes no new dependency.
