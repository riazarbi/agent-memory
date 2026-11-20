# CLI11 — `delete` and `retag`

**Milestone:** restructuring  |  **Builds on:** CLI10

## Goal
Memory can lose things. `memory delete` removes one block, a node, or with `--recursive` a node
and everything under it, and reports every link it left dangling. `memory retag` renames or
removes a tag across the whole store, in frontmatter and in block text.

## Scope
- `internal/memory`: the two operations, under D28's write rules (the lock, a fresh load, atomic
  replaces, read-back, roll back).
- `cmd/memory`: the verbs `delete` and `retag`, their arguments, results, errors and exit codes;
  the usage text and package comment list them.
- `README.md`: the Usage section documents both verbs; the status line lists them.
- Tests and the CHANGELOG entry.

## Not in scope
- `move`, `rename`, `move-block`, `split`, `merge` (CLI12–CLI14). No store-wide link rewriting:
  `delete` reports links, it never edits them.
- Tool files, the skills, `memory_purpose.md`, `agent/README.md` and anything else under `agent/`
  (CLI15). `agent/` is byte-for-byte unchanged by this unit.
- Any change to `read`, `write`, `search`, `index` or `lint`, or to the library's checks and
  reason texts.
- Revising §4.5's guidance block.

## Governing spec
`docs/spec/agent-memory.md`: **III.8** (the verb table rows for `delete` and `retag`, *Refusals*,
*`delete`, exactly*, *`retag`, exactly*, *Deleting does not rewrite links*, *Postcondition*,
*Results*); III.1 (finding the store; a missing store is created empty); III.3 (errors, exit 75);
D28 (*Writing to disk*); D25 (*Reading the store*); D17 (tag syntax); D31 (inline tags); III.7
(`lint`, used here as the postcondition check).

## Prerequisite spec changes
III.8 is new and is committed with this unit's lock, together with the passages it revises in
place (III.2, III.4, III.5, III.6, §4.5's note, §4.6, §6, the D33 row).

## Existing tests this unit supersedes
None known. If a test pins the usage text or the verb list exactly, stop and report it.

## Acceptance criteria
All through the real binary. The fixture store `F` is the shipped starting tree plus, made with
`write` (IDs are whatever `write` assigns; `<A>` etc. name them):

- `outbox-etiquette` under `topics`, tags `escalations`, `project/atlas`, blocks
  `<A>` "Urgent mail goes to Priya #escalations first." and `<B>` "See [[old-plan]] for the rota."
- `old-plan` under `projects`, tags `escalation`, `escalations`, `projects`, blocks `<P1>`
  "Rota starts 2026-12-01.", `<P2>` "Agreed with Priya."
- `old-task` under `old-plan`, block `<T1>` "Draft the rota. See [[old-plan]]."
- `priya` under `people`, block `<R1>` "Owns [[old-task#^<T1>]] and [[old-plan]]. #project/atlas"
- `misc-note` under `topics`, tag `misc`, blocks `<M1>` "Keep this #misc note.", `<M2>`
  "See [[nowhere]]."

1. **A block is deleted and nothing else changes.** `delete outbox-etiquette --block <A>` → exit 0,
   stderr empty, stdout `<memory_delete stem="outbox-etiquette" block="<A>" nodes="0" blocks="1">`
   with no `<dangling>`. `outbox-etiquette.md` equals its old bytes minus `<A>`'s paragraph (and
   the blank line that separated it); `<B>` keeps its ID and bytes; every other file in the store
   keeps its bytes and mtime. Then `delete outbox-etiquette --block <B>` leaves a node with no
   blocks that `read` still shows and `lint` reports clean.
   *Excludes:* re-encoding the whole note; reassigning IDs; refusing to empty a node; deleting the
   node when its last block goes.
2. **A block delete reports the links it broke, including from the same node.** On `F`, add to
   `outbox-etiquette` a block `<C>` "As [[outbox-etiquette#^<A>]] says." and to `priya` a block
   `<R2>` "Per [[outbox-etiquette#^<A>]]." `delete outbox-etiquette --block <A>` → two
   `<dangling>`, in this order: `stem="outbox-etiquette" block="<C>"`, then `stem="priya"
   block="<R2>"`, each with the link exactly as written. Both blocks' text is unchanged on disk.
   `lint --exit-zero` afterwards has exactly two more warnings than before, and `errors="0"`.
   *Excludes:* stripping or rewriting the links; listing only links from other nodes; counting
   blocks instead of links.
3. **A plain delete never removes what the caller cannot see.** On `F`: `delete old-plan` → exit 1,
   stdout empty, one stderr line naming `old-plan`, saying it has 1 child and to add `--recursive`;
   the store is unchanged (bytes and mtimes). With a file `notes/topics/misc-note/notes.txt` added,
   `delete misc-note` → exit 1 naming `notes.txt` and `--recursive`. With instead a folder
   `notes/topics/misc-note/broken/broken.md` that has no frontmatter (a lint error, so not a loaded
   child), `delete misc-note` → exit 1 naming `broken`. With instead only
   `notes/topics/misc-note/.DS_Store`, `delete misc-note` → exit 0, `nodes="1" blocks="2"`, the
   folder gone. Second-order: `delete old-task` succeeds (`nodes="1" blocks="1"`), and after it
   `delete old-plan`, refused a moment ago, succeeds.
   *Excludes:* checking only loaded children; treating dot-entries as content; a refusal that does
   not say what is in the way.
4. **`--recursive` removes the subtree and reports every broken link once.** On `F`:
   `delete old-plan --recursive` → `<memory_delete stem="old-plan" nodes="2" blocks="3">` holding
   exactly three `<dangling>`, in order: `stem="outbox-etiquette" block="<B>"` `[[old-plan]]`;
   `stem="priya" block="<R1>"` `[[old-task#^<T1>]]`; `stem="priya" block="<R1>"` `[[old-plan]]`.
   Not listed: `<T1>`'s link to `old-plan` (inside the deleted subtree) and `<M2>`'s `[[nowhere]]`
   (dangling before the call). `notes/projects/old-plan` is gone and `notes/` holds no leftover
   hidden folder. `read old-plan` and `read old-task` are unknown stems. `lint --exit-zero` shows
   `errors="0"`.
   *Excludes:* one `<dangling>` per block; reporting links between deleted nodes; reporting links
   that were already dangling; leaving the renamed-aside folder behind.
5. **Bad calls change nothing.** Each → exit 1, stdout empty, one stderr line naming the argument
   as typed, store unchanged: `delete` (no stem); `delete /` (names `STEM`); `delete old-plann`
   (unknown stem, lists `old-plan` as a near match); `delete priya --block m-zzzzzz` (names
   `--block`); `delete priya --block <A>` (a block of another node: names `--block`);
   `delete old-plan --block <P1> --recursive` (names `--recursive`); `retag` (no tag);
   `retag "bad tag" x` and `retag misc "bad tag"` (name `OLD`, `NEW`); `retag a b c`. On a parent
   dir with no store, `delete x` → unknown stem, exit 1, and the empty `memory/notes/` exists
   afterwards (III.1).
   *Excludes:* a partial delete before an argument is rejected; exit 2; errors naming internal
   field names.
6. **`retag` renames in frontmatter and in text, by whole path segment.** On `F`:
   `retag escalations escalation` → exit 0, `<memory_retag from="escalations" tag="escalation"
   nodes="2">`. `outbox-etiquette`'s tags are `escalation`, `project/atlas` and `<A>` reads
   "Urgent mail goes to Priya #escalation first."; `old-plan`'s tags are `escalation`, `projects`
   (once, first position). Every byte of both notes outside the tag tokens is unchanged; every
   other file keeps its bytes and mtime. `search tag:escalations` finds nothing; `index` lists
   `escalation` with count 2. Then `retag '#project' proj` → `nodes="2"`: `outbox-etiquette` has
   `proj/atlas`, `<R1>` ends "#proj/atlas", and `old-plan` still has `projects`.
   *Excludes:* frontmatter only; a string-prefix replace that turns `projects` into `projs`;
   leaving a duplicate tag; rewriting untouched notes.
7. **`retag OLD` removes, or refuses cleanly.** On `F`: `retag misc` → `<memory_retag from="misc"
   nodes="1">` (no `tag` attribute); `misc-note` has no tags and `<M1>` reads "Keep this note."
   Then add a block `<M3>` whose whole text is "#gone" to `misc-note` and tag `priya` with `gone`:
   `retag gone` → exit 1, a stderr line naming `<M3>`, and nothing written anywhere (`priya` still
   has the tag). `retag nobody` and `retag escalation escalation` → exit 0, `nodes="0"`, no note
   touched.
   *Excludes:* leaving a double space; writing an empty block; changing some nodes before failing
   on another.
8. **Both verbs obey the lock and add no lint error.** While another process holds
   `notes/.write.lock`, `delete priya` and `retag misc` each wait and then exit 75 with the "busy"
   line, writing nothing. After every successful call in criteria 1–7, `lint` reports no error
   that was not there before the call. The usage text lists `delete` and `retag`;
   `TestAgentFolderIsComplete` passes unchanged.
   *Excludes:* writing without the lock; a delete that leaves an unloadable store.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- The QA gate is in `CLAUDE.md`. The whole tree stays lint-clean.
- `internal/memory` already has the writer (the lock, the fresh load, read-back and roll back),
  the encoder that leaves untouched blocks byte-identical, the link scanner (`scanLinks`) and the
  inline-tag scanner used by search. Reuse them; do not write a second parser for links or tags.
- `cmd/memory/retry_bin_test.go` shows how the existing tests hold the lock from another process.
- The library does not log, and takes no new dependency.
- Results are rendered with `encoding/xml`, like the other verbs'.
