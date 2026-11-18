# U7 — The memory tools: `memory_read`, `memory_write`, and seeding `memory/`

**Milestone:** memory-structure  |  **Builds on:** U1 (write deny, D16), U3–U6 (store, banked)

## Goal
The model can save to and read from durable memory through two built-in tools, and every install gets
a seeded `memory/` to save into — so "remember X" lands on disk and can be recalled later. Memory is
not yet in the system prompt (that is U8): this unit works through the tool descriptions alone.

## Scope
- `memory_read` and `memory_write` as built-in tools per D29: argument schemas, the default
  descriptions verbatim, XML results built with `encoding/xml`, field-named errors, `memory_read`
  marked read-only.
- Toggle markers (`memory_read.json`, `memory_write.json`) seeded on fresh installs and backfilled once
  into existing ones through a seed-generation bump (`seedGeneration`, `builtinSinceGeneration` in
  `internal/tools/shell_tool_seeder.go`); a deleted marker stays deleted.
- A `memory/` location in config (`$AGENT_STATE_DIR/memory`) and seeding per D29 in `agent.New`, on the
  directory-level contract `skills/` and `livecontext/` use (do not pre-create it in
  `ensureStateSkeleton`).
- Both tools use the one shared writer (`WriterFor`) for `memory/notes/`.
- Events `memory.saved`, `memory.updated`, `memory.read`, each with a `renderTextLine` case.
- `scripts/smoke/memory.sh` (live LLM, throwaway state); README and CHANGELOG.

## Not in scope
- The system-prompt section — capability line, `usage.md` injection, `<memory_index>`,
  `memory.disabled`, `memory.index_truncated`, per-input rendering — U8.
- `memory_search` — U9. Restructuring tools — later.

## Governing spec
- `docs/spec/agent-memory.md` §4.5 *The memory tools, exactly* (D29), the default `usage.md` text, and
  *Events*; §4.2 *Writing to disk* (D28) and *Encoding nodes* (D27) for the semantics the tools expose;
  D13, D16, D20, D22.

## Prerequisite spec changes
D29 and the pinned `usage.md` text are committed with this unit.

## Acceptance criteria
1. **Seeding** — on a fresh state dir, `agent.New` creates `memory/protocol/usage.md` (byte-equal to the
   spec's text) and `memory/notes/{user,projects,people,topics}/<stem>.md`, each loading clean with the
   spec's description and no blocks. Run again: nothing changes. Delete `usage.md` and run again: it
   stays deleted. With `memory/` already present (even empty), nothing is seeded.
   *Excludes:* seeding per file (resurrecting deletions); pre-creating `memory/` in the skeleton.
2. **Markers** — a fresh install has both markers; an existing install from before this change (a
   `.toggle-seeded` sentinel at the previous generation, no memory markers) gets both exactly once;
   deleting `memory_write.json` removes the tool from the registry on the next reconcile, and the next
   start does not restore it.
   *Excludes:* re-seeding a deleted marker; adding the tools without a generation bump.
3. **`memory_write` through the real handler** — create `outbox-etiquette` under `topics` with one block;
   then update it appending a block and replacing the first by ID. Each call's result is well-formed
   XML (parsed by `encoding/xml` in the test) holding `created`, one `<assigned>` per requested block in
   order, and the node as read back; the files on disk match. Errors: a bad stem, a missing parent, a
   two-paragraph block (`blocks[0].text`), a block with a heading — each a tool error naming the field,
   the store unchanged.
   *Excludes:* hand-built XML; errors without the field; results echoing the request instead of the
   node as read back.
4. **`memory_read` through the real handler** — on a store where `a` links `[[b]]` and `b` has a child:
   `stem: b` returns its description, tags, blocks with IDs, children at depth 1, links and the
   backlink from `a`; `depth: 0` omits children; `block: <id>` returns that block only; no `stem` returns
   `<memory>` with the four first-level nodes and the tag list; an unknown stem `topic` lists `topics`
   among its near matches. Block text containing `<`, `&`, `"` and `]]>` comes back escaped and the
   result still parses.
   *Excludes:* string-concatenated XML; an enum of stems in the schema.
5. **Events and rendering** — each successful write emits `memory.saved` (create) or `memory.updated`
   (update) with the stem and assigned IDs and no block text; each read emits `memory.read` with stem,
   block and depth. Each has its own text-mode line via `renderTextLine` (not the generic fallback).
   *Excludes:* logging block text; a generic fallback rendering.
6. **One writer, parallel calls** — 10 `memory_write` calls appending to one node, dispatched as
   parallel tool calls in one turn through the real tool dispatch, all land, with distinct IDs.
   *Excludes:* a writer per call (breaks the one-writer rule).
7. **End to end (live LLM)** — `scripts/smoke/memory.sh`, in a throwaway state dir: (a) ask the agent
   to remember an unforgeable sentinel fact about the operator; assert `memory.saved` or
   `memory.updated`, and that the sentinel is now in a node under `memory/notes/`. (b) In a fresh run
   with `-no-memory` (no conversation carries over), ask about that fact, hinting to check memory;
   assert `memory.read` and that the reply contains the sentinel. (c) Delete `memory_read.json` and
   `memory_write.json`; assert the agent starts and neither tool is registered.
   *Excludes:* wiring the tools without the shared writer, the real registry or the real seeding.

## Environment
- Built in a clean-room copy of the committed baseline (no `.git`); `docs/spec/` holds only the two
  governing specs.
- Criterion 7 needs the live LLM in `.env` (throwaway `AGENT_STATE_DIR` only; never print `.env` or any
  key). A full smoke run takes about 7 minutes.
- `golangci-lint --new-from-rev` needs git and cannot run in the room; lint the touched packages; the
  delivery lead runs the real check.

## Notes for the builder
- Follow `load_skill`'s pattern (`internal/tools/skill.go`) for a harness-backed built-in; but no enum
  in `memory_read`'s schema (D29: it would grow with the store and change the schema on every write).
- The shared writer is `memory.WriterFor(notesDir)`; never construct a second.
- Smoke tests assert on event names, not prose (`CLAUDE.md`).
- The file-writing tool here has turned ` `-style escapes into raw characters before — check.
- Each fixture should give each outcome a sole cause; new lines must be lint-clean.

## Post-verification notes (banked 2026-11-16)
- **Verdict:** accepted at the first build. Independent verification passed all 7 criteria through the
  real binary (scripted stub and live LLM): a genuine upgrade from a generation-3 install added both
  markers without restoring an operator-deleted one; every live result parsed under a strict XML
  parser with hostile text round-tripping; ~40 mutations, all killed but one; 10 inverted assertions
  all failed; smoke 29/29.
- **Criterion weaker than intended:** C6's "a writer per call" cannot be told apart by behaviour — U6's
  OS lock serializes separate writers too — so the surviving mutant is harmless and only code review
  shows the build is right. Criteria about sharing must probe identity, not outcome.
- **Live model behaviour (memory not yet in the system prompt):** facts filed as new child nodes under
  `user` rather than blocks; paths passed as stems; the near-match error recovered every time.
- **Ratified (D29):** model-facing field names belong to the tools; `memory_read` details as built;
  `tool.invoked` logging accepted; T8 open. **Carried to U8:** the path-stem description wording, a
  corrective error for a missing `notes/`, and a smoke check that the sentinel came from `memory_read`.
- **Stale text left for a later pass:** the README still says the file tools can write `reminders/`
  (predates the state-dir deny); `state_skeleton.sh`'s comment lists eight subdirectories.
