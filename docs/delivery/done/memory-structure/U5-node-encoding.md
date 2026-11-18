# U5 — Node encoding: block IDs, validation, and faithful serialization

**Milestone:** memory-structure  |  **Builds on:** U3, U4 (read model, banked)

## Goal
`internal/memory` can turn a node — new, or loaded and changed — into the exact bytes of its folder
note, following D27 and D26: crypto-random block IDs, validation before output, byte-identical output
for an unchanged node, faithful rewrites. Pure functions: nothing is written to disk yet.

## Scope
- A block-ID generator per D27 (the random source injectable for tests).
- An encoder for a **new** node (description, tags, properties, block texts → bytes + assigned IDs).
- An encoder for a **loaded** node plus requested changes (description, tags, property merges, block
  appends, block replacements by ID) → bytes + assigned IDs, per D27/D26, including alias expansion
  and keeping the BOM and line ending.
- Validation on write per D27, with corrective errors naming the field.
- The read-side additions D27 needs: the read model records whether a file began with a byte-order
  mark, its line ending (that of the opening delimiter line), and each block's exact source lines.
- Tests; a `CHANGELOG.md` entry.

## Not in scope
- Anything touching the filesystem for writing: create-vs-update decisions, parent and stem rules,
  atomic writes, locking, read-back — unit U6.
- Tools, injection, events, seeding — U7/U8. No smoke test: nothing is wired into the runtime.

## Governing spec
- `docs/spec/agent-memory.md` §4.2 *Encoding nodes* (D27), *Round-trip of frontmatter* (D26),
  *Reading the store* (D25) — D25 defines what "loads clean" means and which block texts are legal.

## Prerequisite spec changes
D27 is pinned and committed with this unit.

## Acceptance criteria
"Loads clean" means: loading the encoder's output through the read model gives no errors and no
warnings except dangling-link warnings.

1. **Block IDs** — every generated ID matches `m-[a-z0-9]{6}`; with an injected source whose first
   draws collide with an ID already in the store, and with an ID already assigned earlier in the same
   encode, the generator draws again and returns a fresh ID; 3 appended blocks in one encode get 3
   distinct IDs.
   *Excludes:* a counter or timestamp ID; uniqueness checked against the node only, or not within one
   write.
2. **A new node** — description `"How to cc"`, tags `["#escalation", "outbox"]`, properties
   `{"zeta": "2026-11-14", "alpha": "007", "flag": "true", "n": 3, "b": false, "list": ["a","b"],
   "none": null}`, blocks `["First claim.", "Second claim, see [[people]]."]` → output whose
   frontmatter lists `description`, `tags`, then `alpha, b, flag, list, n, none, zeta` (sorted); body
   is `First claim. ^<id1>`, a blank line, `Second claim, see [[people]]. ^<id2>`, final newline; LF,
   no BOM. It loads clean, with tags `[escalation, outbox]`, `zeta` = the string `"2026-11-14"` (not a
   date), `alpha` = `"007"`, `flag` = `"true"` (strings), `n` = 3, `b` = false, `none` empty, `list`
   = `[a, b]`, and the two blocks with the returned IDs.
   *Excludes:* emitting strings unquoted so they decode as dates, numbers or booleans; map-order
   (random) property order.
3. **Validation before output** — each of these is rejected with an error naming the offending
   field, and no bytes are produced: description `""`; description `"a\nb"`; tag `"two words"`; tag
   `"123"`; property `{"x": {"y": 1}}`; property `{"x": [1, 2]}`; block `""`; block
   `"one\n\ntwo"`; block `"- item"`; block `"## heading"`; block `"text ^m-abc123"`; block
   `"text ^foo"`; block `"see [[c#Heading]]"`; block `"see ![[c]]"`; block `"see [[path/c]]"`. These
   are accepted: block `"see [[missing]]"` (dangling); block ``"code `[[x]]` here"``; block
   `"two\nlines"` (one paragraph).
   Also rejected, naming the field: a property string `"a\n\u2028"`, a property string containing
   U+2029, a list item containing NEL (U+0085), a description containing `\r`, a tag containing
   U+2028; a property key `""`; `description` passed as a property.
   *Excludes:* validating only some fields; producing output and relying on the reader to reject it;
   treating only `\n` as a line break.
4. **Unchanged means byte-identical** — for each folder note in a corpus of valid, fully-IDed nodes —
   LF; CRLF with a BOM; comments and irregular spacing in frontmatter; a delimiter line with trailing
   spaces; an alias; a block with a tab before its ID; a block whose ID stands alone on its last line;
   two blank lines between blocks; trailing spaces in a block — loading it and encoding it with no
   changes returns exactly the file's bytes.
   *Excludes:* re-emitting from the parsed form (whitespace, ID placement or delimiter normalised).
5. **Body changes keep untouched blocks exactly** — on a node whose frontmatter has a comment and whose
   blocks include a tab-separated ID and a standalone-ID line, plus one ID-less operator paragraph:
   appending one block → the frontmatter bytes are unchanged; every untouched block's source lines are
   unchanged; the operator paragraph and the appended block get new distinct IDs; blocks are separated
   by exactly one blank line; it loads clean. Replacing block `m-…` by ID → that block has the new text
   and keeps its ID, others unchanged. Replacing an ID not in the node → error, no output.
   *Excludes:* re-emitting every block from its decoded text (ID placement normalised); re-emitting
   the frontmatter when only blocks changed.
6. **Frontmatter changes re-emit faithfully** — on a node with
   `d: 2026-11-14`, `f: 1.50`, `q: "007"`, `l: [a, b]`, `c: x  # note`, `keep: 1` (in that order):
   merging `{"keep": 2, "new": "v"}` and replacing `description` → `keep` = 2 in its original
   position, `new` appended last, and `d`, `f`, `q`, `l`, `c` keep their source form (date not
   datetime, `1.50`, double-quoted, flow list, `# note`); replacing `tags` with `["x"]` replaces the
   whole list; output loads clean with the expected decoded values.
   *Excludes:* re-emitting properties from decoded values; appending updated keys at the end; merging
   tags instead of replacing them.
7. **Aliases and file conventions** — `a: &v old` / `b: *v`: changing `a` to `"new"` → output loads
   clean with `a` = `new`, `b` = `old`, and is valid YAML. A CRLF file with a BOM, with a block appended
   and a property changed → output starts with the BOM and uses CRLF on every line; an LF file stays
   LF throughout.
   *Excludes:* leaving `*v` pointing at a changed anchor (silently changing `b`) or dangling; dropping
   the BOM or mixing line endings.
8. **Pure** — after any encode in criteria 4–7, the loaded store is unchanged (its `Frontmatter.Raw`,
   `Doc` contents, field nodes, properties and blocks compare equal to a fresh load of the same files),
   and no file in the fixture tree was modified or created.
   *Excludes:* editing the store's YAML nodes in place; writing files from the encoder.

9. **The encoder refuses to emit what does not load** — the encoder's final check loads the complete
   output file (delimiter lines included) through the read model. Test the check directly: when the
   emitted bytes would not load (e.g. via a test seam that corrupts the emitted frontmatter), the
   encoder returns an error and no bytes. Separately, a fuzz test over string property values,
   descriptions, tags and block texts drawn from a corpus including every Unicode line-break character,
   YAML-special words and quote characters: every input is either rejected by validation or produces
   output that loads clean and decodes back exactly — never a file the reader rejects.
   *Excludes:* a guard that re-parses only the YAML, not the whole file; relying on validation alone.
10. **Rulings of 2026-11-15 hold** — (a) updating `f: 1.50` with the value `1.5` changes nothing (bytes
    identical); (b) on a node with no tags, setting `tags: ["x"]` places `tags` directly after
    `description`; setting `tags: []` where tags exist emits an empty list; (c) on a node where
    `m-aaaaaa` appears twice, replacing it changes the first occurrence; replacing the same ID twice in
    one write is an error; (d) a mixed-ending file (CRLF opening line, LF body) with a block appended
    keeps its copied regions' endings and writes the new block with CRLF.
    *Excludes:* treating equal values as changes; tags appended after properties; replacing every
    occurrence of a duplicate ID.

## Environment
- Built in a clean-room copy of the committed baseline (no `.git`); `docs/spec/` there holds only the
  two governing specs.
- No live LLM is needed for this unit's tests. The QA gate's smoke suite may use the `.env` endpoint
  with throwaway state dirs (a full run takes about 7 minutes when the endpoint is healthy).
- `golangci-lint --new-from-rev` needs git and cannot run in the room; the delivery lead runs it.

## Notes for the builder
- Use the YAML nodes U4 retains (`Node.Frontmatter`) — deep-copy before editing.
- The reader is the oracle: every positive criterion's output must load clean through `memory.Load`
  on a temp tree, and tests should assert that.
- Each fixture should give each expected outcome a sole cause.
- New lines must be lint-clean on the package.

## Rework history
- **Build 1 (from baseline `d3bd44e`) — rejected 2026-11-15.** Verification passed all 8 criteria as
  then written, but fuzzing found a property string ending in U+2028/U+2029 (with an earlier newline)
  emitted as a block scalar whose frontmatter the reader rejects — the node and its subtree vanish.
  The build's guard re-parsed only the YAML. D26/D27 revised (operator rulings 2026-11-15): non-LF line
  breaks rejected everywhere; the encoder loads its complete output before returning bytes; the
  builder's choices where the spec was silent ratified (equal value is no change, tags placement,
  duplicate-ID replacement, extra rejections, comments kept / anchors not, mixed line endings); the
  overclaims corrected (layout not guaranteed; full source kept; the guarantee covers supplied
  content). Criterion 3 extended; criteria 9 and 10 added.
- **Build 2 (from baseline `0f04e02`) — accepted 2026-11-15** (resumed once after an API rate limit;
  no change to inputs).

## Post-verification notes (banked 2026-11-15)
- **Verdict:** accepted. Independent verification passed all 10 criteria with no code defect: every
  named wrong reading killed by mutation (including "guard re-parses only the YAML"); ~528k native fuzz
  executions plus a 90 s update fuzz and a 20k wide sweep, all clean; smoke 28/28; no tampering.
- **Carried to U6** (test strength, not defects): the criterion-9 fuzz cannot rediscover build 1's
  U+2028 bug if guard and validation are removed (the regression is still pinned by an explicit
  criterion-3 case); criterion 6's fixture put `keep` last, so "updated keys appended at the end"
  passes that test (caught only by the guard); the guard's warnings check has no isolating test.
- **Ratified (D27):** block text rejects non-LF line breaks; the guard uses the folder-note parser, not
  a full `Load`; new strings are verified per value; an un-re-emittable operator value edit-locks that
  node's frontmatter.
- **For future fuzz criteria:** require an acceptance floor and a mutation proof that the fuzz finds
  the class it targets — build 2's first generator passed 98 of 1,500 inputs through validation.
- **Process:** the verifier printed `.env` (including `AGENT_API_KEY`) into its own transcript; nothing
  left the machine. The builder's file-writing tool turned `﻿`/` ` escapes into raw
  characters; expect the same in future rooms.
