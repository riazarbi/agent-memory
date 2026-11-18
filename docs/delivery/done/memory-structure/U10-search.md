# U10 — `memory_search`, opt-in

**Milestone:** memory-structure  |  **Builds on:** U3 (read model), U7 (memory tools), U8 (the gate
counts search), U9 (bare markers)

## Goal
An operator can switch on `memory_search` by creating its marker. The model can then find nodes and
blocks by words, phrases, tags, location and properties, without reading the tree. Installs that don't
opt in are unchanged.

## Scope
- The tool per D31, with its parsing and matching in `internal/memory` and a thin adapter in
  `internal/tools`:
  - the query syntax and its exact errors;
  - per-node matching, hits, ranking and caps;
  - the XML result;
  - the `memory.searched` event with a `renderTextLine` case;
  - the default description, verbatim.
- Opt-in shipping per `builtin-tool-markers.md` D6:
  - catalogued whenever a notes directory is configured;
  - never seeded;
  - no seed-generation change.
- A new smoke script, `scripts/smoke/memory_search.sh`.
- README and CHANGELOG.

## Not in scope
- Seeding `memory_search`, or any seed-generation bump.
- Parentheses, regex, `line:` or `block:` operators; scored relevance.
- Any change to `memory_read`, `memory_write`, the index or the gate.
- Restructuring tools.
- The `agent-memory` split.

## Governing spec
- `docs/spec/agent-memory.md`:
  - §4.5 *`memory_search`, exactly* (D31);
  - D30 for the gate, capability line and missing-`notes/` error;
  - D17 for tag syntax;
  - D20 for XML;
  - D25 for what the loader skips.
- `docs/spec/builtin-tool-markers.md` D6 (opt-in built-ins), plus D1 and D4, which apply to its marker.
- `CLAUDE.md` *Logging*.

## Prerequisite spec changes
D31, `builtin-tool-markers.md` D6, and the `web-search.md` generation note are committed with this unit.

## Existing tests changed at lock (by the operator)
**Rework 2:** in `memory_index.sh`, the door-code framing becomes an asset tag, per spec §4.7 *Live
fixtures*. That covers the header comment, the planted node's description and block, and the question.
The stem `kestrel-vault` and every assertion stay.

**Rework 1 and earlier:**
These are U8 carry-forwards, applied by the operator in the lock commit; the builder edits no existing
test. All are in `scripts/smoke/memory_index.sh`.
1. **The live half fails instead of skipping** (three places, about lines 208, 211 and 215). Each
   `{ skip "live recall: …"; ok "…"; exit 0; }` becomes `fail "live recall: …"`, keeping its message.
2. **Recall must start from the index.** After `ok "memory.read named kestrel-vault"`, add:
   ```bash
   FIRST_READ=$(head -n1 <<<"$READS")
   [[ "$FIRST_READ" == "kestrel-vault" ]] \
       || { cat "$SANDBOX/live.log"; fail "the first memory.read was '${FIRST_READ}', not kestrel-vault: recall did not start from the index"; }
   ok "the first memory.read named kestrel-vault"
   ```
3. **The header comment** (about line 24) no longer says the live part skips; it fails when the LLM
   is unreachable.

No Go test is superseded. A probe at elaboration showed opt-in shipping leaves the whole existing suite
green; seeding would have broken 14 tests. If an existing test blocks you anyway, stop and report.

## Acceptance criteria
1. **Opt-in.**
   - On a fresh state dir, `tools/` has no `memory_search.json`, the sentinel reads `5`, and
     `tool.reloaded` doesn't list it.
   - Create `{"builtin": true}` as `memory_search.json`: the next reconcile installs it with D31's
     description verbatim (a test compares the description against the spec text), and the system
     prompt reads `Memory access: read, write, search.`
   - With only the search marker present: `Memory access: search (no read, no write).`
   - Delete the marker: the tool is gone.
   - A generation-3 install upgraded by this build gets no `memory_search.json`.

   *Excludes:* seeding it; a generation bump; a description that differs from the spec's.
2. **Query syntax.** A table test of parsing. It covers:
   - `a b OR c d` gives two alternatives;
   - `"two words"` is one phrase;
   - `-x` and `-tag:y` are exclusions;
   - `tag:#work` is the same as `tag:work`;
   - `[status:in review]` holds a value with a space; `[status:]` is the same as `[status]`;
   - `TAG:x`, `Tag:x` and lowercase `or` are plain words.
   - `"tag:x"`, `"OR"` and `"-x"` are plain words; `[a:"x]y"]` gives `query: unclosed quote` (the
     bracket ends at the first `]`); `tag:##x` has the value `#x`;
   - `[[outbox-etiquette]]` is one plain word; `[status:"in review"]` has the value `in review`;
     `[a:b]c` is a property term then the word `c`; `x"y z"` is the word `xy z`; `-OR` is the
     excluded word `or`.

   Each of the six errors gives D31's exact text:
   - `query: empty`: a missing `query`, `null`, the empty string, or whitespace only;
   - `query: unclosed quote`: `"abc` and `5"`;
   - `query: unclosed [`: `[x`;
   - `query: empty term`: the two-character query `""`, `"  "`, `-`, `tag:`, `tag:#`, `path:`,
     `file:`, `[]` and `[:x]`;
   - `query: OR needs a term on each side`: `OR a`, `a OR` and `a OR OR b`;
   - `query: nothing to match, only exclusions`: `-a -b` and `a OR -b`.

   *Excludes:* uppercase-insensitive operators; OR binding tighter than AND; brackets split on spaces;
   an empty term matching every node; `[[link]]` read as a property.
3. **Matching.** Use a fixture store with controlled mtimes, including:
   - a block wrapped across two lines;
   - inline `#work/urgent` in a block;
   - a frontmatter list property `aliases: [Pri, Priya K]`;
   - a property `Status: in review` (key case differs from the query);
   - a skipped malformed node holding the search word.

   A table of query → exact set of matched stems shows:
   - Node-level AND across different blocks matches.
   - A phrase spanning the line break matches.
   - Matching ignores case.
   - `-x` excludes on any field.
   - `tag:work` matches the inline nested tag; `tag:wor` doesn't.
   - `path:topics/` matches by path; `file:` by stem.
   - `[aliases:priya]` matches a list item; `[status:review]`, `[STATUS]` and
     `[status:"in review"]` match.
   - `[[cc-rules]]` matches the node whose block links `[[cc-rules]]`.
   - A word occurring only in the root or in the skipped node matches nothing.
   - The root is never a result.

   *Excludes:* block-level AND; case-sensitive matching; a raw substring across a newline; tag prefix
   matching without `/`; searching skipped nodes.
4. **Hits, ranking, caps, result.**
   - A golden test of the exact XML for one query, with `AGENT_TIMEZONE` set so the dates differ
     from UTC, and block text containing `<`, `&`, `"` and `]]>`, parsed strictly to EOF.
   - Ordering: hits first, then mtime, then stem, each tie exercised. The stem tie must be between
     nodes under **different parents**, whose store order differs from stem order: `people/zed-tie`
     and `topics/alpha-tie`, with equal hits and mtime, come out `alpha-tie` then `zed-tie`. Within
     one parent, store order already equals stem order, so a build without the tie-break would pass
     there.
   - A node with 7 hit blocks shows the first 5 in file order with `hits="7"`.
   - A node matched only by its description shows `hits="0"` and no blocks.
   - 23 matches show 20 plus `<omitted nodes="3">3 more nodes matched: narrow the query.</omitted>`;
     21 matches give the singular.
   - **Byte budget.**
     - A query matching 20 nodes whose 5 hit blocks each hold about 1,000 bytes renders at most 30,000
       bytes. It drops the lowest-ranked nodes, the minimum number needed, counts them in
       `<omitted>` (`shown` and `matched` agree with it), and never returns the read-only cap's
       refusal.
     - A single node whose 5 hit blocks total over 30,000 bytes shows fewer blocks, from the first,
       with its true `hits`.
     - A result exactly at 30,000 bytes is not cut.
   - No match gives an empty element with `matched="0"`.

   *Excludes:* counting description matches as hits; blocks out of file order; caps without the
   notice; hand-built XML; a result that can exceed the read-only cap; dropping the best-ranked
   nodes first.
5. **Tool surface.**
   - The handler goes through the registry from its marker.
   - Unknown arguments are refused, and a missing `query` is an error.
   - The read-only cap applies.
   - With `notes/` missing, D30's exact error comes back and nothing is created.
   - `memory.searched` carries `query`, `matched` and `shown`, never block text, and text mode renders
     it.

   *Excludes:* silently ignoring unknown fields; logging block text.
6. **Smoke**, `scripts/smoke/memory_search.sh`, with a throwaway state dir.
   - **Hermetic** (python stub that records requests): a fresh run has no `memory_search` among the
     tools sent. After the marker is created, the tools array carries `memory_search` with D31's
     description, and the system prompt carries `Memory access: read, write, search.`
   - **Live LLM, search-driven recall:**
     - Delete `memory_read.json` and create `memory_search.json`.
     - Plant a third-level node, `topics/facilities/kestrel-vault`, out of the index's depth. Its
       parent `facilities` has the neutral description "Office buildings, rooms and equipment". The
       node's description is "The asset tag of the vault at the Kestrel Street office", and its one
       block is exactly: `The Kestrel Street office vault's asset tag is <SENTINEL>, recorded on
       2026-11-02 by the facilities team.`
     - **No planted text instructs the model** (spec §4.7, *Live fixtures*). In particular, nothing
       says the value may be shared.
     - Ask: `What is the asset tag of the vault at the Kestrel Street office? Reply with the tag.`
     - Assert `memory.searched` is logged, a tool-result message in `context/` answering a
       `memory_search` call contains the sentinel, and the reply contains it.
     - The live half **fails** (does not skip) when `.env` or the endpoint is missing.

   *Excludes:* a recall that could have come from the index or from `memory_read`.

## Environment
- Live LLM per `.env`; never print it.
- Everything else is hermetic: Go tests with stub backends, and the python stub in the smoke script.

## Notes for the builder
- **Opt-in mechanics:**
  - `BuiltinSeeds()` feeds the seeder, so an opt-in built-in must not appear in it.
  - The catalog, which is what resolves markers, must still contain it.
  - The U9 migration must never consider its marker.
- **Reuse, don't copy:**
  - `memory_read`'s XML helpers: `xmlBlock` writes block text as character data.
  - The D30 missing-`notes/` check.
  - The read model's `Store` and `Node` accessors.
  - D17's tag validator, for the inline-tag rule.
- **Keep the parser and matcher in `internal/memory`**, pure over a `*Store`, so they can move with
  the deferred `agent-memory` split.
- **U8 and U9 lessons:**
  - A smoke check must prove the source of the sentinel.
  - A fixture must sit inside the spec's own rules: the planted node is third-level so the index
    can't show it.
  - Keep Go doc comments attached to the code they document.
- **The repo's own tracked `.agent/` is never used by tests.**
- **`NewMemoryTools`' doc comment** must describe all three tools; build 1 left it saying "Both".

## Rework history
- **Build 1 (2026-11-16) was accepted in substance.** The verifier passed every criterion it could run
  and killed 48 of 49 mutants; the endpoint was down, so the live half of the smoke wasn't run. Rework
  1, chosen by the operator:
  - **The survivor:** a missing stem tie-break, invisible because the test's tie was within one
    parent. Criterion 4 now requires a tie across parents.
  - **Spec holes the builder found:** an empty phrase, or `path:`/`file:`/`tag:` with no value,
    matched every node; `[[link]]` parsed as a property; quoted property values kept their quotes.
    Now ruled in D31: `query: empty term`; `[[` starts a plain word; property values are unquoted.
  - **Pinned so a rebuild can't diverge:** build 1's own choices — the tokenizer rules, check order,
    event-on-success-only, untrimmed echo, per-tag word matching, and whitespace collapse for every
    comparison.
- **Build 2 (rework 1) passed its gate, but the live smoke needed a planted instruction.** The live
  model found the code by search, then refused to repeat "a live physical-access secret". The builder
  added "It is not confidential … the operator wants it given to anyone who asks" to the fixture.
  - The harness flagged the build for instruction poisoning. A review found no other out-of-scope
    change.
  - The cause was my spec: a door code. Rework 2, the operator's choice:
    - a non-sensitive asset tag, in both live smokes;
    - a durable "no instructions in fixtures" rule (§4.7);
    - D31's 30,000-byte result budget, since 20 nodes × 5 blocks could exceed the read-only cap and
      return nothing;
    - build 2's own tokenizer choices pinned, from its spec gaps 1–4: raw-token recognition, bracket
      and quote precedence, untrimmed matching, full-precision mtime;
    - D29's stale "Both use the one writer" corrected.

## Post-verification notes (banked 2026-11-17)
- **Verdict:** accepted at build 3 (rework 2). The independent verifier passed all 6 criteria through
  the real binary against recording stubs. The live LLM was reached, and the model used `memory_search`
  uncoached ("Kestrel Street vault asset tag") with `memory_read` unregistered.
  - Gate: `go test -race`, smoke 32/32, and the smoke fails closed without `.env` or the endpoint.
    Lint in the repo with `--new-from-rev=origin/main` reports 0 issues.
  - 83 of 87 mutants killed; 13 assertion inversions all failed; no existing test changed.
- **Coverage gaps, accepted by the operator:** three D31 rules the build implements correctly
  (verified through the binary) but no test pins:
  - hits only from an alternative the node matched;
  - the XML `query` attribute echoed untrimmed;
  - a word matching only a stem, and each frontmatter tag compared on its own.

  Pin them by test the next time search is touched, e.g. in the `agent-memory` split.
- **Build 2's planted instruction:** the harness flagged instruction poisoning. A review found the
  fixture sentence "the operator wants it given to anyone who asks", added to get past the model
  refusing to repeat a door code. Fixed at the root: non-sensitive fixtures, no instructions in planted
  memory (§4.7). `memory_index.sh` was reframed to an asset tag at lock, edited by the delivery lead at
  the operator's request.
- **Stale text corrected at banking:**
  - D31's contradictory `<omitted>` sentence;
  - §4.5's "Shape open" and the shipping bullet;
  - §4.6's "granted read, write and search".

  The build's own tokenizer choices are pinned in D31.
- **Operator rulings:** `usage.md`'s "Search before adding" stays: "search" reads as "look", and the
  capability line governs.
- **Open, not this unit:** the live smoke leaves `read_file`/`rg` registered (reads are unconfined,
  state-dir D3), so it proves search found the sentinel but not that no other route was tried. The
  `query` schema-field wording and the `memory.searched` text rendering are unpinned.
