# CLI2 — `write` and `search`

**Milestone:** cli  |  **Builds on:** CLI1 (banked)

## Goal
`agent-memory write` saves to the store, and `agent-memory search` finds things in it, with D29's
and D31's semantics and results. Errors name the flags as the user typed them, and exit code 75
means "retry".

## Scope
- `write STEM [--parent P] [--description D] [--tag T]… [--property K=V]… [--add TEXT]…
  [--replace ID=TEXT]…` (Part III.2), mapped onto `memory.WriterFor(...).Write(WriteRequest)`.
- `search QUERY` (Part III.2), on `memory.ParseQuery` and `memory.Search`. The CLI applies D31's
  caps (20 nodes, 5 blocks each), its byte budget and its result XML.
- Exit code 75 for `memory.ErrBusy` and `memory.ErrChangedOnDisk` (Part III.3).
- Tests that run the built binary. README "Usage so far" extended. CHANGELOG entry.

## Not in scope
- `index`, tool files, releases (CLI3).
- Clearing tags, deleting blocks, any restructuring (Part III.2).
- Changing `internal/memory` behaviour.

## Governing spec
- `docs/spec/agent-memory.md` Part III (III.2, III.3).
- D29's `memory_write` (create versus update, blocks, tags replace, properties merge, results, the
  field-named errors, ratified rulings).
- D31 in full (syntax, errors, matching, hits, ranking, caps, byte budget, result XML, the U10
  rulings), apart from its event.
- D28 for busy and changed-on-disk.

## Existing tests this unit supersedes
None. CLI1's tests and `internal/memory`'s must stay green unchanged.

## Acceptance criteria
Every case runs the built binary on a store seeded with `init`, in a fresh temp working directory.

1. **Create.** `write kettle-descaling --parent topics --description "How to descale the kettle"
   --tag home --tag cleaning --property status=active --add "Use citric acid." --add "Rinse twice."`
   exits 0. It prints D29's `<memory_write created="true" stem="kettle-descaling">`, with two
   `<assigned>` IDs in argv order, followed by the node rendered as `read` renders it at depth 0.
   `read kettle-descaling` then shows both blocks, both tags and `status`.
   *Excludes:* ignoring `--property`; assigned IDs out of order.
2. **Update, in argv order.** On that node, `write kettle-descaling --add "New A." --replace
   <id1>="Use citric acid, 2 tbsp." --add "New B." --tag home --description "Kettle descaling"`:
   - `created="false"`;
   - the `<assigned>` IDs are, in order, a new ID, `<id1>` and a new ID;
   - block 1's text is replaced, and the two new blocks are appended in that order;
   - the tags are now just `home`;
   - the description is replaced;
   - `status` is unchanged (properties merge).

   *Excludes:* processing all `--add`s before all `--replace`s; tags merged rather than replaced;
   properties replaced rather than merged.
3. **Errors name the arguments as typed, and write nothing.** Each of these exits 1, with stdout
   empty, one stderr line naming the argument, and the store byte-identical afterwards:
   - a create without `--parent` names `--parent`;
   - an unknown parent names `--parent`;
   - an invalid stem (`Bad_Stem`) names `STEM`;
   - a second block containing a blank line names `--add #2` (blocks are numbered from 1 across
     all `--add` and `--replace` arguments, in argv order);
   - `--replace` with no `=` names `--replace`;
   - `--replace` with an unknown ID names `--replace <that id>`;
   - `--property` with no `=`, or with an invalid key, names `--property`;
   - an invalid tag names `--tag #N`.

   *Excludes:* writer-internal names such as `blocks[1].text` or `properties["k"]`; partial writes.
4. **Retry is exit 75.** While another process holds the store's write lock (the D28 flock on
   `notes/.write.lock`), `write` exits **75** with `busy` in the stderr line, and writes nothing. A
   changed-on-disk refusal also exits 75. Any other failure exits 1.
   *Excludes:* exit 1 for retryable errors.
5. **Search results.** On a store with 25 nodes matching `kettle`, `search kettle` exits 0 and
   prints D31's `<memory_search query="kettle" matched="25" shown="20">`:
   - 20 `<node>`s, in the library's rank order;
   - each with at most 5 `<block>` hits, in file order, and `hits` giving the true count;
   - `<omitted nodes="5">5 more nodes matched: narrow the query.</omitted>`.

   A query that matches nothing prints `matched="0" shown="0"` and exits 0.
   *Excludes:* uncapped output; re-ranking in the CLI; a missing omitted notice.
6. **Search budget and errors.**
   - When the result would exceed D31's byte budget, nodes are dropped lowest-ranked first, as D31
     rules, and counted in `<omitted>`.
   - `search` with an empty or whitespace query exits 1 with `query: empty`. An unclosed quote
     gives `query: unclosed quote`, and every other D31 error has its exact text.
   - More than one positional argument exits 1 with a message telling the user to quote the query.

   *Excludes:* splitting a query over several arguments; trimming the echoed `query` attribute.
7. **No `notes/`.** `write` and `search` on a store without `notes/` print Part III.3's message and
   exit 1. `write` creates nothing: no `notes/`, no lock file.

## Environment
- The build happens in a clean-room copy of the committed baseline, with no `.git`. No LLM is
  needed. The gate is `CLAUDE.md`'s, and lint stays at 0 issues tree-wide.

## Notes for the builder
- **Size target: about 250 production lines.** Reuse CLI1's argument walker, store resolution, error
  helpers and node renderer. Do not duplicate them.
- `--add` and `--replace` are repeatable flags whose relative order matters. CLI1's walker sees
  arguments in order, so record them as they come.
- The writer's own errors name fields like `blocks[i].text`, `tags[i]` and `properties["k"]`.
  Translate them to the argument names above. Look at the error values or types the library
  exposes before matching on message text.
- D28's lock waits a bounded 10 s before "busy". A test that waits it out is acceptable. If the
  library offers a test seam for the wait, use it.
