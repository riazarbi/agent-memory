# U8 — Injection: the memory section, `<memory_index>`, and the U7 carry-forwards

**Milestone:** memory-structure  |  **Builds on:** U3 (read model), U7 (memory tools, seeding)

## Goal
When any memory tool is granted, the system prompt carries a memory section: a capability line, the
operator's `usage.md`, and an index of the store rendered by the harness. So the model knows it has
memory, what it may do with it, and where things are, without having to explore. When no memory tool
is granted, the section is absent and the operator is told why.

## Scope
- The memory section per §4.4 and D30:
  - the gate on registered memory tools;
  - the capability line;
  - `memory/protocol/usage.md`;
  - `<memory_index>` built with `encoding/xml`, with its budget and notice;
  - placement as the fourth system-message part;
  - computed once per chain.
- Events `memory.disabled`, `memory.index_truncated` and `memory.load_problem`, logged on change, each
  with a `renderTextLine` case.
- U7 carry-forwards:
  - `memory_write`'s default description gains the stem sentence now in D29, so the code matches the
    spec (`TestMemoryTools_DescriptionsAreTheSpecs` is red at lock and must go green unchanged);
  - the corrective error for a missing `notes/` (D30).
- A smoke script `scripts/smoke/memory_index.sh` (new); README and CHANGELOG.

## Not in scope
- `memory_search` (U10). The capability-line and notice rows that name it are pinned now so U10 doesn't
  change existing lines. Build the rule; don't build the tool.
- Moving the index to the end of the request (D22's open door), `-immutable` write refusal (T8), and
  any change to `-no-memory`.
- Changing `usage.md`'s text, the seed, or the memory tools' behaviour, apart from the two
  carry-forwards above.
- Restructuring operations, dreaming, audit.

## Governing spec
- `docs/spec/agent-memory.md`:
  - §4.4 *Injection* and *Injection, exactly* (D30);
  - §4.5 *The memory tools, exactly* (D29), for the description and seeding;
  - §4.2 *Reading the store* (D25), for what `Store.Problems()` reports;
  - decisions D3, D8, D13, D15, D20, D22 and D23;
  - traps T1, T3, T9, T10 and T13.
- `CLAUDE.md` *Logging*.

## Prerequisite spec changes
D30, the D29 description sentence, and the path-free trust line in the §4.4 example are committed with
this unit.

## Acceptance criteria
1. **Gate, capability line, composition.** Drive the agent with a stub backend that records every
   request, with a `prompt/` fragment and one skill present.
   - Both memory markers present: the system message is nudge, fragment, skills catalogue, then the
     memory section, joined by `\n\n`. The section is `Memory access: read, write.`, then `usage.md`
     (trailing whitespace trimmed), then `<memory_index>…</memory_index>`, joined by `\n\n`.
   - Only `memory_read.json`: the line is `Memory access: read (no write).`
   - Only `memory_write.json`: the line is `Memory access: write (no read).`
   - Neither marker: no `Memory access`, no `usage.md` text and no `<memory_index>` anywhere in the
     system message, though `memory/` is fully populated.
   - Markers present but `usage.md` deleted: the capability line is followed directly by the index.

   *Excludes:* gating on `memory/` existing or on markers present but unresolved; a capability line
   built from prose; `usage.md` inside the envelope; the section placed anywhere but last.
2. **The index, exactly.** Set up a fixture store with controlled mtimes and `AGENT_TIMEZONE` set to a
   zone where the UTC date differs. The fixture holds:
   - the four first-level nodes;
   - `topics/outbox-etiquette` (tags `escalation,outbox`), with a child `cc-rules` (a third-level
     node);
   - a description containing `<`, `&` and `"`;
   - an extra property and an inline `[status:: draft]` field.

   The rendered index equals a golden byte for byte:
   - the trust line is the leading character data;
   - first-level nodes are ordered by stem, each with children one level deep;
   - `outbox-etiquette` shows `children="1"` and `cc-rules` is not rendered;
   - dates are in the zone;
   - the description is escaped, and the index parses under strict `encoding/xml` to EOF;
   - `<tags>` holds the counts.

   Further cases:
   - Removing the extra property and the inline field changes nothing.
   - With 60 distinct tags, exactly 50 appear: most used first, ties by name.
   - With no tags, there is no `<tags>` element.
   - With `notes/` empty, there is no index.
   - The section contains no filesystem path.

   *Excludes:* hand-built XML; depth 1 or 3; relative dates; local-vs-UTC date slips; tags capped at
   anything but 50; the path in the trust line; anything reading protocol conventions.
3. **The budget.** Use stores generated over the line limit only and over the byte limit only (long
   descriptions, few nodes).
   - The result fits within 200 lines and 25,000 bytes.
   - Second-level entries go oldest first, ties by stem, before any first-level entry.
   - First-level entries go only once no second-level entry remains.
   - The minimum number of entries is dropped in that order.
   - Each parent's `children` is unchanged.
   - The notice text is exact for `memory_read` registered, for neither tool registered, and for
     singular `1 node not shown…`.
   - A store exactly at the limit is not truncated.
   - Second-order case:
     1. Render twice: `memory.index_truncated` is logged once.
     2. Add a node: it is logged again with the new `omitted`.
     3. Shrink the store until it fits: nothing is logged.

   *Excludes:* head-order truncation; dropping newest first; recounting `children`; a notice left out
   of the budget; logging every input.
4. **Once per chain.** Use a stub backend that makes the model call `memory_write` (creating
   `kestrel-notes` under `topics`) and then reply, plus the cap-final path with the cap set to 1.
   - Every request in the first chain carries a byte-identical memory section with no
     `kestrel-notes`.
   - The next input's requests carry it.
   - Editing `usage.md`, or removing `memory_write.json` and letting the reconciler run, mid-chain
     changes nothing until the next input.
   - This holds in single-shot and in the interactive loop, where the catch-up chain counts as an
     input.

   *Excludes:* rendering the section per call (T10); the cap-final request rebuilding it; the section
   frozen at startup.
5. **`memory.disabled`.**
   - Start with no memory markers: exactly one `memory.disabled reason=tools_absent`. With either
     marker present: none.
   - Mid-session, with both markers present, delete `memory_write.json` and then `memory_read.json`:
     no event after the first delete, exactly one after the second.
   - Restore one marker, then delete it again: one more.
   - Other tools' reconciles never log it.
   - Text mode renders it on its own line.

   *Excludes:* logging per input or per reconcile; logging while memory is still partly on.
6. **Store problems at render.**
   - A malformed node (a heading in its body) and a stray file under `notes/`: the malformed node is
     absent from the index, and its parent's `children` excludes it.
   - Across three inputs, each problem is logged once as `memory.load_problem` with severity, path and
     reason.
   - Add a second malformed node: all problems are logged again, once.
   - Fix every problem (both malformed nodes and the stray file): nothing is logged.
   - *(Corrected after build 1, 2026-11-16: the fixture and "fix both" wording contradicted D30's
     depth 2 and its re-log-on-any-change rule; the build followed D30.)*

   *Excludes:* no warning; a warning per input; a malformed node rendered from partial data.
7. **Carry-forwards.**
   - `memory_write`'s default description equals D29's new text, so the existing spec-comparison test
     passes unmodified.
   - With `memory/` present but `notes/` absent, and in a second case with `notes` as a regular file,
     the D30 error text comes back from all three calls:
     - `memory_read` with no stem;
     - `memory_read` with `stem: user`;
     - `memory_write` creating a node.

     Afterwards, no `notes/` and no `.write.lock` exist.
   - In that state, the section has the capability line and `usage.md` but no index.

   *Excludes:* the raw lock-file error; creating `notes/` or the lock as a side effect; an empty
   `<memory>` passed off as success.
8. **Smoke** — `scripts/smoke/memory_index.sh`, with a throwaway state dir:
   - **Hermetic** (python stub backend recording requests, as in `builtin_toggle.sh`):
     - The system message carries `Memory access: read, write.` and `<memory_index>` with a planted
       stem.
     - Delete `memory_read.json`: `Memory access: write (no read).`
     - Delete `memory_write.json` too: neither appears, and `memory.disabled` is logged.
     - A planted store over budget: `<omitted` in the prompt and `memory.index_truncated` logged.
   - **Live LLM, index-driven recall:**
     - Plant `topics/kestrel-vault`, described as the Kestrel Street office vault door code, with the
       code as a random sentinel in a block, never in the description.
     - In a fresh state with no conversation, ask for the door code.
     - Assert `memory.read` names `kestrel-vault`, a tool-result message in `context/` answering a
       `memory_read` call contains the sentinel, and the reply contains it.

   *Excludes:* a smoke that passes on a sentinel the model guessed or found elsewhere (U7's weak spot).

## Environment
- Live LLM per `.env`; never print it.
- Smoke scripts sandbox their own state.
- Everything else is hermetic: stub backends in Go tests and the python stub in the smoke.

## Notes for the builder
- **Where the code lives today:**
  - The system message is built per call in `buildSystemMessage` (`internal/agent/agent.go`).
  - A chain starts in `Run`'s `runChain`.
  - `runCapFinalInference` builds its own system message.
  - The reconciler's `reconcile` (`internal/tools/shell_tool_reconciler.go`) is where markers take
    effect.
  - `memory_read`'s child-entry rendering (`internal/tools/memory.go`, `entries` and `xmlEntry`) is
    the form the index uses.
  - Dates use `AGENT_TIMEZONE`, as the tools do.
- **Dedupe "on change" against the previous render in this process**, as `reportSkills` does with a
  fingerprint. The first render counts as a change.
- **Avoid apostrophes in pinned text:** `encoding/xml` escapes `'` as `&#39;` in character data.
- **U7 lessons:**
  - The smoke's recall must prove the source of the sentinel, not just its presence.
  - Criteria about sharing or identity must probe identity, not outcome.
- **Observed live behaviour before injection:** the model filed facts as new child nodes under `user`
  and passed paths as stems. That isn't a criterion here; report what the live smoke shows after
  injection under SPEC GAPS.

## Post-verification notes (banked 2026-11-16)
- **Verdict:** accepted at the first build. The independent verifier passed all 8 criteria through the
  real binary against a recording stub backend, with an independently written golden for the index.
  - Every wrong-but-plausible implementation the criteria name failed: 36 mutants killed, 9 assertion
    inversions each failed. One survivor, equivalent: removing `memory_write`'s pre-check alone, where
    the post-write check gives the same answer.
  - The gate: `go test -race`, build, tidy and release passed; lint found 0 new issues; smoke passed
    30/30 with the live LLM.
- **Build 1 surfaced two criterion errors of mine,** corrected before verification:
  - C2's fixture put `cc-rules` at the third level.
  - C6's "fix both" contradicted D30's re-log rule.

  The build followed D30. Lesson: check a criterion's fixture against the spec's own rules (depth, set
  semantics) before lock.
- **Ratified:** the gate counts `memory_search` too (D13 over D30's first wording); D30 amended.
- **Carried to U10 (search):**
  - The live smoke can't tell index-driven recall from exploration. A no-index mutant passed by
    reading the root, then `topics`, then `kestrel-vault`. Assert that the first `memory.read` names
    the planted stem, with no root read.
  - The live half skips, exit 0, when the LLM is unreachable. It should fail, as the other live smokes
    do.
- **New unit before U10:** U9, seeded built-in markers stop carrying their description. Today a seeded
  description counts as an operator override, so no built-in's description change reaches an existing
  install. U7-seeded installs keep the old `memory_write` text.
- **Live behaviour after injection:**
  - The model passed a proper stem with `parent: user`, not a path.
  - It still files a fact as a new child node under `user` rather than as a block on `user`.
  - Recall went straight to `memory_read kestrel-vault` in one call.
- **Accepted by design (D30):** a marker removed mid-chain leaves the section saying `read, write`
  until the next input.
- **Raised, deferred by the operator:** splitting memory into a separate `agent-memory` binary.
  - Blast radius measured at banking: `internal/memory` imports nothing from the agent; about 1,000
    adapter lines in three files; about 150 lines across six shared core files.
  - The operator will return to it after this milestone.
  - Until then, new memory logic goes in `internal/memory` and adapters stay thin.
