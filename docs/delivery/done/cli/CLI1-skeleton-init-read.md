# CLI1 — The `agent-memory` binary: finding the store, `init`, `read`

**Milestone:** cli  |  **Builds on:** the library in `internal/memory` (as ported)

## Goal
An `agent-memory` binary exists. It finds the agent's memory store the way the agent finds its state
dir, it can seed a store (`init`), and it can read one (`read`), with the result XML, errors and exit
codes of spec Part III.

## Scope
- `cmd/agent-memory/` with verb dispatch, and usage text when no verb or an unknown verb is given.
- Store resolution (Part III.1).
- `init` (Part III.2, D19 seeding), using `memory.SeedDefaults`.
- `read` (Part III.2, D29's `memory_read` results and rulings).
- The error and exit-code conventions (Part III.3), for these two verbs.
- Tests that run the built binary, plus unit tests where they help. Update `CHANGELOG.md`, and
  `README.md` with a short "Usage so far" section.

## Not in scope
- `write`, `search`, `index` (CLI2, CLI3), tool files, release jobs.
- Any change to `internal/memory`'s behaviour. The CLI only calls it. Adding a small exported
  helper there is acceptable if the CLI needs one.

## Governing spec
- `docs/spec/agent-memory.md` **Part III** (III.1–III.3).
- D29 (results XML and its rulings), D19 and the seeding paragraph after D29, D20, D22, D25.

## Existing tests this unit supersedes
None. `internal/memory`'s tests must stay green unchanged.

## Acceptance criteria
Every case runs the built binary in a fresh temp directory as its working directory. "Store" is
the resolved `<state dir>/memory`.

1. **Finding the store.**
   - With `AGENT_STATE_DIR=/abs/X` in the environment, the store is `/abs/X/memory`.
   - With it unset and `./.env` holding `AGENT_STATE_DIR="Y"` (quoted, with a trailing ` # comment`
     line elsewhere), the store is `./Y/memory`.
   - With neither, the store is `./.agent/memory`.
   - With both set, the environment wins.
   - A relative value is taken against the working directory.

   Check each case by running `init` and seeing where `memory/` appears.
   *Excludes:* a hardcoded `.agent`; ignoring `.env`; letting `.env` override the environment; a
   `--store` flag.
2. **`init` seeds once.**
   - On a missing store, it creates `notes/` with the four first-level nodes and their D29 seeding
     descriptions, and `protocol/usage.md` with the D19 text verbatim. It exits 0 and prints what
     it created.
   - A second run changes nothing (compare bytes and mtimes), exits 0, and says nothing was needed.
   - If `memory/` exists but `usage.md` was deleted, it is not recreated.

   *Excludes:* re-seeding an existing store; partial top-ups.
3. **`read` with no stem reads the root.** On a seeded store with two tagged child nodes,
   `agent-memory read` prints `<memory>` holding the first-level nodes as child entries, and a
   `<tags>` element listing every tag with its node count. It exits 0.
   *Excludes:* reusing the index's top-50 tag cap (D29 ruling: no cap here).
4. **`read STEM` renders a node with its edges.** A node with two blocks, a child, an outgoing link
   to an existing node and one to a missing node, and a backlink from another node:
   - the output has the attributes `stem`, `path`, `parent`, `tags` and `modified` (`YYYY-MM-DD`);
   - it has `<description>` and each `<block id>`;
   - `<children>` lists the child;
   - `<links>` marks the missing target `dangling="true"`;
   - `<backlinks>` lists the linking node.

   `--depth 0` omits children, and `--depth 2` includes grandchildren. `--block ID` returns the
   node's attributes, its description and that one block with *its* links and backlinks, and no
   children.
   *Excludes:* omitting backlinks; ignoring `--depth`; a block read that still lists every block.
5. **Errors and exit codes.**
   - An unknown stem exits 1 with stdout empty, and stderr listing up to five near matches.
   - An unknown flag, or a missing or invalid `--depth`, exits 1, and stderr names the flag as typed.
   - A store without `notes/` gives, for `read`, `memory has no notes/ folder; run agent-memory
     init` on stderr with exit 1, and creates nothing (no `notes/`, no lock file).
   - No verb, or an unknown verb, prints usage to stderr and exits 1.

   *Excludes:* errors on stdout; exit 0 on failure; writer-internal field names in messages.
6. **Dates follow the local zone.** A node whose folder note's mtime is 2026-11-14T23:30:00Z shows
   `modified="2026-11-14"` under `TZ=UTC`, and `modified="2026-11-15"` under `TZ=Asia/Tokyo`.
   *Excludes:* hardcoded UTC.

## Environment
- The build happens in a clean-room copy of the committed baseline, with no `.git`. No live LLM and
  no `.env` from the repo are needed. Tests make their own `.env` files in temp dirs.
- The QA gate is `CLAUDE.md`'s. The tree is lint-clean, and must stay at `golangci-lint run ./...`
  → 0 issues.

## Notes for the builder
- Keep it lean: aim for a few hundred lines of production code. Use the standard library's `flag`
  package (a `flag.FlagSet` per verb), not a CLI framework. Dependencies stay `yaml.v3` only.
- The library is the source of truth. Render from `memory.Load`'s `Store`. Do not re-parse files in
  the CLI.
- Interleaved flags and positionals: Go's `flag` stops at the first positional. Decide how `read
  STEM --depth 2` is handled, make both orders work, and say how.

## Post-verification notes (banked 2026-11-18)
- **Verdict:** accepted. The independent verifier passed all six criteria live through the binary.
  - It caught 43 of 46 mutants, and 10 inverted assertions each failed.
  - The gate is green, with lint at 0 issues. There was no tampering, and scope is clean.
- **Size:** 660 production lines against a target of "a few hundred". The extra is the hand-rolled
  argument walker (flags and positionals in any order), the `.env` reader, and near matches. CLI2
  and CLI3 reuse them.
- **Rework in place:** the builder's spec gap 1 showed the `.env` grammar diverging from the agent's
  in four cases (first versus last occurrence wins, `export<TAB>`, a leading `#`, an unterminated
  quote). The spec pinned the agent's grammar, and the same builder fixed the four, with a test
  each. It was cheaper than a fresh build.
- **Surviving mutants, behaviour correct but untested:**
  - an unreadable `.env` falling through instead of erroring;
  - dropping `time/tzdata`;
  - dropping the per-node tag dedupe in the root count.
- **Recorded as known gaps in Part III:** the `memory/`-without-`notes/` loop; concurrent `init`
  misreporting; skipped nodes being silent.
