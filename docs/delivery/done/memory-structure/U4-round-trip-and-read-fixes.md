# U4 — Frontmatter round-trip groundwork and read-model fixes

**Milestone:** memory-structure  |  **Builds on:** U3 (read model, banked)

## Goal
The read model keeps everything a writer needs to re-emit a node's frontmatter faithfully (D26), and
the three defects and two test gaps found when U3 was accepted are closed. Reading only — nothing is
written yet.

## Scope
- In `internal/memory`: retain, for every loaded node, the raw frontmatter text, and for every
  property (including `description` and `tags`) its YAML key and value nodes, exposed so a later writer
  can re-emit them. Decoded values and the existing API behave exactly as today.
- Fix: a non-sequence tag on a list is an error (D25).
- Fix: the folder note is matched by exact, case-sensitive name on every platform (D25).
- Add the two missing tests (criterion 5).
- A `CHANGELOG.md` entry, per `CLAUDE.md`.

## Not in scope
- Writing anything, re-emitting frontmatter, block-ID generation — unit U5. This unit only keeps the
  material; tests may encode a retained YAML node to check it, but no write path is built.
- Any change outside `internal/memory` except `CHANGELOG.md`. No smoke test: still nothing wired into
  the runtime.

## Governing spec
- `docs/spec/agent-memory.md` §4.2 *Round-trip of frontmatter* (D26) and *Reading the store* (D25) —
  in particular the folder-note exact-name rule and the list-tag rule, and the two "Built read model"
  notes they replace.

## Prerequisite spec changes
D26 and the D25 list-tag precision are pinned and committed with this unit. No accepted test encodes
a behaviour this unit changes, except tests that assert the macOS case behaviour or list-tag
acceptance, if any exist — report them rather than editing them.

## Acceptance criteria

1. **Raw frontmatter is kept byte-for-byte** — a folder note whose frontmatter has a comment line,
   irregular spacing (`key:    value`), a quoted value, a flow list and a block list loads; the node's
   raw frontmatter equals exactly the bytes between its opening and closing `---` lines (CRLF and a
   byte-order mark included where present in the source).
   *Excludes:* keeping a re-encoded or normalised copy instead of the source bytes.
2. **Each property keeps its source form** — for `d: 2026-11-14`, `t: 2026-11-14T00:00:00Z`,
   `f: 1.50`, `q: "007"`, `l: [a, b]`, a block list `m:` with items `a` and `b`, and `c: x  # note`:
   encoding each property's retained key and value nodes with yaml.v3 yields the same scalar text,
   tag, style and comment as the source — `d` stays the date `2026-11-14` (not a datetime), `f` stays
   `1.50`, `q` stays double-quoted `"007"`, `l` stays a flow list, `m` a block list, `c` keeps
   `# note` — while each decoded value is exactly what U3 returned. The same holds for `description`
   and `tags`.
   *Excludes:* retaining only decoded values (date and datetime become indistinguishable; `1.50`
   becomes `1.5`); dropping tags, styles or comments.
3. **A tag on a list is checked** — each of these nodes is skipped with one `error`, while a valid
   sibling loads: `k: !foo [a, b]`, `tags: !foo [a]`, `k: !!binary [a]`, `k: !!set [a]`,
   `k: !!str [a]`. `k: !!seq [a, b]` and an untagged `k: [a, b]` load.
   *Excludes:* checking tags only on scalars or list items.
4. **The folder note is matched by exact name** — a folder `a/` containing only `A.md`: `a` is skipped
   with exactly one `error` (missing folder note) and there is no warning about `A.md` — on a
   case-insensitive filesystem (macOS) as on a case-sensitive one. A folder `a/` containing `a.md`
   loads as before.
   *Excludes:* `os.Stat("a/a.md")`-style lookups that succeed case-insensitively; reporting the
   same file as both the folder note and an invisible stray.
5. **Two behaviours U3 left untested** — (a) node `a`'s only link to `c` is the dangling block link
   `[[c#^m-zzzzzz]]` (`c` exists, the block does not): `c`'s node backlinks are empty and the edge is
   dangling with one `warning`. (b) `description: |` followed by a single indented line: the node is
   skipped with an `error`.
   *Excludes:* counting dangling block links as node backlinks; accepting a literal block for a
   one-line description.

## Environment
- Built in a clean-room copy of the committed baseline (no `.git`); `docs/spec/` there holds only the
  two governing specs.
- No live LLM is needed for this unit's tests. The QA gate in `CLAUDE.md` includes the smoke suite,
  which may use the `.env` endpoint with throwaway state dirs; it takes over 30 minutes in full, so
  run it in parts if a background limit stops it, and say which parts ran.
- `golangci-lint --new-from-rev` needs git and cannot run in the room; the delivery lead runs it.

## Notes for the builder
- The read model already parses frontmatter into `yaml.Node`s; this unit is about keeping them (and
  the raw bytes), not re-parsing.
- Criterion 4 needs a filesystem test that runs on case-insensitive macOS; make it hold on both.
- Each test fixture should give each problem a sole cause.
- New lines must be lint-clean on the package (`golangci-lint run --max-same-issues=0
  --max-issues-per-linter=0 ./internal/memory/...`).

## Post-verification notes (banked 2026-11-15)
- **Verdict:** accepted at the first build. Independent verification passed all 5 criteria on both a
  case-insensitive (APFS) and a temporary case-sensitive volume; a baseline-vs-build dump of the full
  read-model API over a 92-node corpus differed only where intended; 21 of 22 mutants killed (the
  survivor is equivalent); 8 inverted assertions all failed; smoke 28/28 in one 6m50s run.
- **Criterion 1's wording was ambiguous** ("a byte-order mark included where present"): a file's BOM
  precedes the opening `---`, so it is never between the delimiters; the build follows "between", and
  D26 now says so.
- **Decisions for U5, made at review:** preserve BOM and line endings on rewrite; expand aliases that
  point at a changed field; write from deep copies (D26, U5 skeleton).
- **Smoke flakiness:** `timer.sh` failed once in the builder's run because the live model refused to
  make the requested tool call, so the protection was never exercised; it passed on rerun. The script
  treats a refusal as a failure — worth making it inconclusive instead, separately.
