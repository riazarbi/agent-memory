# U6 — Write operations on disk

**Milestone:** memory-structure  |  **Builds on:** U3–U5 (read model and encoder, banked)

## Goal
`internal/memory` can execute a write against `memory/notes/` exactly as D28 specifies: create or update
by stem, through the encoder, atomically, serialized, safe against external edits, read back and
rolled back on failure — so a failed write always leaves the store as it was. Plus the encoder test
hardening carried from U5.

## Scope
- A writer for a `notes/` directory: one write operation taking the `memory_write` fields (stem,
  optional parent, optional description, optional tags, optional properties, block edits) and returning
  the node as loaded plus the IDs it assigned, or an error.
- All D28 behaviour: serialization, fresh load per write, create vs update, refusals, atomic replace,
  edit-race check, read-back with roll back.
- The three encoder test-strength items carried from U5 (criterion 8).
- Tests; a `CHANGELOG.md` entry.

## Not in scope
- The `memory_write` / `memory_read` tools, their JSON schemas, events and `renderTextLine` cases — U8.
- Injection and seeding of `memory/` — U7. Restructuring operations (move, rename, delete, create at
  the root) — later, with the dream.
- Any timing heuristic for coordinating writers (e.g. waiting on other writers' temporary files) —
  D28's lock replaces it; building one is a defect. No smoke test: nothing is wired into the runtime.

## Governing spec
- `docs/spec/agent-memory.md` §4.2 *Writing to disk* (D28), *Encoding nodes* (D27), *Reading the store*
  (D25); §4.5 (`memory_write` shape).

## Prerequisite spec changes
D28 is pinned and committed with this unit.

## Acceptance criteria
Fixtures are temp `notes/` trees with first-level nodes `user` and `topics`. "Unchanged" means every
file under `notes/` has the same bytes, and no file or folder was added or removed.

1. **Create** — stem `outbox-etiquette`, parent `topics`, description `"How to cc"`, one block
   `"A claim."`, **run with the process umask set to 077**: afterwards
   `topics/outbox-etiquette/outbox-etiquette.md` exists (file mode 0644, folder 0755), a fresh `Load`
   shows the node with parent `topics` and that block, and the write
   returned that node and the block's assigned ID.
   *Excludes:* creating the folder at the root or beside the parent; returning the requested values
   instead of the node as read back; relying on the umask for the modes.
2. **Create refusals leave the store unchanged** — each refused with a corrective error: stem
   `Bad_Name`; stem `user` (exists — the error says it would be an update or that the stem is taken);
   stem `ghost` **with parent `user`** where a folder `topics/ghost/` exists but is skipped by the
   reader (malformed note) — a different parent, so only the store-wide stem check can refuse it;
   parent `nowhere` (missing); no parent (create at the root); no description.
   On a case-insensitive filesystem, with `topics/Ghost/` present, a create of `ghost` is refused
   with an error saying the stem collides by letter case — not a "retry" error.
   *Excludes:* checking stems only against loaded nodes; creating first-level nodes.
3. **Update** — appending a block to `user` with no parent given succeeds (the block is present on a
   fresh `Load`, its ID returned); the same append with parent `topics` (not `user`'s parent) is refused
   with the store unchanged. A property merge on `topics` keeps its other bytes per D27.
   *Excludes:* ignoring a mismatched parent; re-creating instead of updating.
4. **Skipped nodes are refused** — `topics/broken/broken.md` has no frontmatter (skipped by the
   reader): a write with stem `broken` is refused with an error quoting the reader's problem, and the
   file is byte-identical afterwards.
   *Excludes:* writing over a malformed node (creating a fresh one at the same path).
5. **Serialized, atomic, and fresh** — 20 goroutines each append one block to `user` concurrently:
   afterwards `user` has all 20 blocks, 20 distinct IDs, and loads clean (no lost update). Between two
   writes, the operator edits `user.md` on disk (adds a paragraph): the second write's result keeps
   that paragraph (fresh load, not a cached store). With a test seam that makes the rename fail, the
   original file is byte-identical and no temporary file remains in the folder.
   *Excludes:* an unserialized writer; a cached store; writing in place; leaking temp files.
6. **Edit race is refused** — with a test seam that modifies `user.md` on disk after the write has read
   it but before the rename: the write returns a "changed on disk" error, the file holds exactly the
   external modification, and nothing else changed. The same for a create whose target folder note
   appears in between.
   *Excludes:* last-writer-wins; comparing only modification times (a same-second edit must still be
   caught — compare contents).
7. **Read-back rolls back** — with a test seam that makes the post-write full load report a new error
   for the written node: for an update, the previous bytes are restored; for a create, the new node's
   folder is removed; in both cases an error is returned and the store is unchanged.
   *Excludes:* keeping a written file that does not load; partial roll back (leaving an empty folder).
8. **Encoder test hardening (carried from U5)** — (a) a fuzz test for encoder inputs biased to
   line-break-terminated strings (a line feed followed by a trailing NEL, U+2028 or U+2029, and similar
   shapes), asserting a minimum acceptance rate, which fails when the encoder's final check and its
   non-LF line-break validation are both removed (demonstrate by mutation and record it in the
   CHANGELOG); (b) a frontmatter-change test whose updated key sits mid-list, so "updated keys
   appended at the end" fails that test directly; (c) a test isolating the final check's warnings
   comparison (supplied content that would add a warning while every other value matches), failing
   when that comparison is removed.
   *Excludes:* fuzzers that never reach the failure class; fixtures where the wrong reading produces
   identical output.

9. **Writes serialize across processes, and never report success for bytes not on disk** — 20
   **separate processes** (a small test driver built from the package — re-executing the test binary
   is fine — each opening its own writer on the same `notes/`) each append one block to `user` at once,
   repeated in at least 5 rounds: every write reports success, all 20 blocks and IDs are in `user.md`
   each round, and the node loads clean. With a test seam that holds the lock in one process for longer
   than the wait: a write from another process is refused with the "busy" error and nothing changes.
   A process killed while holding the lock (mid-write, via a seam that blocks after acquiring it) does
   not block the next write: the next write acquires the lock and succeeds. In-process, with a seam
   that overwrites the folder note with different valid content between the rename and the read-back
   (an editor that ignores the lock): the write returns "changed on disk", and the file holds the
   other content (no roll back).
   *Excludes:* an in-process-only mutex; a timing heuristic in place of the lock; a lock not held
   across the read-back; reporting success for an overwritten write; rolling back over another
   writer's content.
10. **D28 clarifications hold** — a write that adds a link to a missing stem succeeds and is not rolled
    back; an update of a folder note with mode 0600 leaves it 0600; a folder note that is a symlink to a
    file outside `notes/` stays a symlink after an update and its target holds the new content.
    *Excludes:* treating dangling-link warnings as read-back failures; resetting modes on update;
    replacing a symlinked folder note with a regular file.

## Environment
- Built in a clean-room copy of the committed baseline (no `.git`); `docs/spec/` there holds only the
  two governing specs.
- No live LLM is needed for this unit's tests. The QA gate's smoke suite uses the `.env` endpoint with
  throwaway state dirs (about 7 minutes). Never print `.env` or any API key into output.
- `golangci-lint --new-from-rev` needs git and cannot run in the room; lint the package and the delivery
  lead runs the real check.

## Notes for the builder
- Reuse the encoder (`Encoder.EncodeNew` / `EncodeChange`) and `Load`; the write layer adds
  filesystem behaviour only.
- Test seams for failures (rename, edit race, read-back) must not change production behaviour.
- The file-writing tool in this environment has turned `﻿` / ` ` escapes in Go strings into
  raw characters before — check your files.
- Each fixture should give each outcome a sole cause; new lines must be lint-clean.

## Rework history
- **Build 1 (from baseline `7f2e37e`) — rejected 2026-11-15.** Verification passed all 8 criteria as
  then written, but found that writes from separate processes overwrite each other while every write
  reports success (in one run 7 writes reported success, 5 of their IDs absent): the read-back checked
  only that the node loads, not that the write's own bytes were on disk. D28 revised (operator ruling):
  a write never reports success for bytes not on disk, and does not roll back over another writer.
  Also ratified: dangling-link exemption, update keeps mode, symlinked folder notes written through,
  temp-file guarantee scoped to survived failures, one writer per `notes/`, the case-collision error,
  the known limits. Criteria 1 and 2 tightened (umask 077; `ghost` under a different parent;
  case-collision message); criteria 9 and 10 added.
- **Build 2 (from baseline `b812afe`) — not verified; superseded 2026-11-16.** The builder found D28's
  bytes-on-disk read-back insufficient across processes: writer B passes its checks, writer A renames,
  reads back its own bytes and reports success, then B's rename lands — A reported success for bytes
  no longer on disk (about 1 run in 5). It built an unspecified 5 s "wait for other writers' temporary
  files" heuristic to pass criterion 9. The operator ruled instead for an OS lock: D28 now serializes
  every write across processes with `flock` on `notes/.write.lock` (bounded 10 s wait, then "busy —
  retry"), keeps the edit-race check and read-back for editors that ignore the lock, and corrects the
  known limits. Criterion 9 rewritten (all 20 processes succeed; busy; lock released on death); timing
  heuristics put out of scope.
- **Build 3 (from baseline `3afef8e`) — accepted 2026-11-16.**

## Post-verification notes (banked 2026-11-16)
- **Verdict:** accepted at the third build. Independent verification passed all 10 criteria with no
  defects: 100/100 and 400/400 cross-process writes on disk; an external Python `flock` holder made a
  write wait the real 10 s and return "busy"; after `kill -9` on the holder the next write succeeded in
  under a second; 37 code mutations (every one that should be caught, caught); 9 inverted assertions
  all failed; smoke 28/28; cross-compiles for Linux and Windows. Tests were run on macOS only; CI runs
  Linux.
- **Ratified (D28):** the lock file is created on first use and exempt from "unchanged"; case
  collisions refused store-wide on every platform; a no-op write writes nothing; no-`flock` platforms
  refuse writes; the lock file's mode is requested 0644, umask applying.
- **Criteria weaker than intended:** "unchanged" (no file added) could not hold literally once the lock
  file existed — the spec now exempts it; releasing the lock before the read-back slipped past the
  20-process test (the window is too narrow), so a direct lock probe was needed — race criteria
  should require a direct probe of the invariant, not only a load test.
- **Not done:** the builder's spec gaps on polling fairness under heavy contention and a corrective
  error for a missing `notes/` are recorded here, not acted on; neither matters at folder scale.
