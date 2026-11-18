# U3 — Store read model: load `memory/notes/` exactly as the spec reads it

**Milestone:** memory-structure  |  **Builds on:** nothing in this milestone (independent of U1–U2)

## Goal
A new package, `internal/memory`, loads a `memory/notes/` tree into an in-memory store — nodes,
blocks, properties, links and backlinks — and reports every problem it meets as a structured error or
warning, following the spec's *Reading the store* rules. Nothing in the runtime uses it yet.

## Scope
- The `internal/memory` package: load a store from a `notes/` directory.
- What the loaded store must let a caller do (later units build on these):
  - look up a node by stem;
  - get a node's parent (none for first-level nodes) and its children (ordered by stem);
  - list the first-level nodes;
  - for a node: its stem, its path relative to `notes/`, `description`, `tags`, the other
    properties (values as loaded), `modified`, and its blocks in file order (ID, possibly empty, and
    text);
  - resolve a block by (stem, block ID);
  - for a node or block: its outgoing edges, in order of occurrence, each marked dangling or not,
    and its backlinks as unique (source stem, source block ID) pairs;
  - the list of problems, each with severity (`error` or `warning`), the path relative to `notes/`,
    and a human-readable reason.
- Table-driven unit tests over fixture trees built in `t.TempDir()`.
- A `CHANGELOG.md` entry, per `CLAUDE.md`.

## Not in scope
- Writing anything: block-ID generation, create/update rules, serialization — unit U4.
- Rendering the index, the system-prompt section, seeding `memory/`, logging problems as events
  (`memory.load_error`) — unit U5.
- The memory tools — U6; search — U7.
- Any change outside `internal/memory` except `CHANGELOG.md`. No smoke test: nothing is wired into
  the runtime, so there is no operator-visible behaviour to smoke (say so in the CHANGELOG entry).

## Governing spec
- `docs/spec/agent-memory.md` §4.2 *Reading the store* (D25) — the authoritative rules for this unit.
- §4.2 *Blocks*, *Node frontmatter*, *Obsidian validity is enforced*; §4.3 (links are edges); D17,
  D18, D23 (the store is protocol-neutral: block text and extra properties are opaque).

## Prerequisite spec changes
D25 (*Reading the store*) is pinned and committed with this unit. No accepted test is affected.

## Acceptance criteria
All fixtures are trees under a temp `notes/` directory; "loads" means present in the store, "skipped"
means absent from it with an `error` problem naming its path. "Exactly" counts every problem the load
returns, not just those of one kind.

1. **A valid tree loads with its shape** — `user/user.md`, `topics/topics.md`,
   `topics/outbox-etiquette/outbox-etiquette.md`, plus a `.obsidian/app.json` and a `.DS_Store`:
   three nodes load and the load returns no problems; the first-level nodes are `topics` and `user`;
   `outbox-etiquette`'s parent is `topics` and `topics`' children are `[outbox-etiquette]`. A node with
   `tags: ["#escalation", outbox]` and `priority: 3` loads tags `[escalation, outbox]` and carries
   `priority` through with its value; `modified` equals the folder note's mtime (set it different
   from the folder's).
   *Excludes:* treating every `.md` file as a node; reporting or loading dot-entries.
2. **A skipped subtree yields exactly one problem, whatever is inside it** — each of these trees,
   loaded on its own, returns **exactly one** problem (an `error` naming the top skipped folder) and no
   node from inside that folder:
   (a) `projects/` with no `projects.md`, containing `a/a.md`;
   (b) `people/people.md` with no frontmatter, containing `kid/x.md` (a child with no folder note);
   (c) the same `people/`, containing `Bad_X/Bad_X.md` (a child with an invalid stem);
   (d) the same `people/`, containing `readme.txt` (a stray file);
   (e) the same `people/`, containing `kid/kid.md` with no frontmatter.
   `Bad_Name/Bad_Name.md` alone: skipped, exactly one problem.
   *Excludes:* walking (and reporting from) a subtree before deciding to skip it; loading orphaned
   children.
3. **Stem collisions: first loadable folder in component order** — `topics/dup/dup.md` and
   `user/dup/dup.md`, plus `user/dup/kid/other.md` (no folder note) and `user/dup/stray.txt`: the
   `topics/dup` node loads; `user/dup` is skipped with exactly one `error` and nothing from beneath it
   is reported. With `a/x/x.md` and `a-b/x/x.md` both valid, `a/x` wins (component order puts `a`
   before `a-b`; whole-string byte order would not). With `topics/dup/dup.md` malformed and
   `user/dup/dup.md` valid, `user/dup` loads (a skipped folder claims nothing). A stray
   `topics/notes.txt` and a `notes/notes.md` each give one `warning` and change nothing else.
   *Excludes:* last-one-wins; whole-string byte ordering; a malformed folder blocking a valid one.
4. **Frontmatter: Obsidian-lenient, strict where undisplayable** — each of these nodes is skipped with
   an `error`, while a valid sibling still loads: no frontmatter; no `description`; a two-line
   `description`; `description: 123`; tag `"two words"`; tag `"123"`; a property holding a nested
   mapping; a property whose list contains a mapping; the same key twice; a `!!binary` value; a
   `description: |` literal block of two lines. These load: a `description` written as a plain scalar
   wrapped onto a second, indented source line (decodes to one line); `tags: solo` (tags `[solo]`); `tags: []` and `tags:` (no tags); a property
   `dates: [2026-11-14, true, 3]` (items read as the text `"2026-11-14"`, `"true"`, `"3"`); extra
   properties that are a string, a list of strings, an integer, a float, a boolean, a date, a
   datetime with offset and an empty value, each value preserved — dates and datetimes both quoted
   and unquoted.
   *Excludes:* accepting any parseable YAML; rejecting legitimate Obsidian values; rejecting legacy
   bare-string tags.
5. **Bodies are paragraphs only** — a node is skipped with an `error` if any block contains a line
   that is: `## Heading`, `- item`, `1. item`, a ```` ``` ```` fence, `> quote`, `| a |`, `---`,
   `<div>`, `<https://e.com> is a link` (an autolink counts as HTML, by ruling), a line indented
   four spaces, `$$`, `%% comment %%`, `[^1]: note`, `[x]: https://e.com`, or `===` under a first
   line. A node whose body is a three-line paragraph, a blank line, then a
   paragraph with `**bold**`, `` `code` ``, an inline `#tag`, inline `$x$` and a mid-line `[[b]]`,
   loads with exactly 2 blocks, the first holding all three lines.
   *Excludes:* silently accepting content Obsidian renders as non-paragraph; splitting a multi-line
   paragraph into one block per line.
6. **Block IDs** — a block ending ` ^m-7f3a2k` loads with ID `m-7f3a2k` and the ID removed from its
   text; so does a block whose last line is `^m-7f3a2k` alone; a block with no ID loads with an empty
   ID and no problem; a block ending ` ^a1b2c3` loads with no ID, keeps `^a1b2c3` in its text, and
   gives a `warning`; so does ` ^m-ABCDEF`. The same ID twice in one node gives a `warning` and
   resolving (stem, ID) returns the first; the same ID in two different nodes gives one `warning`
   naming the later node, and each resolves within its own node.
   *Excludes:* accepting any `^token` as an ID; failing a node over a duplicate ID.
7. **Links and backlinks** — block `m-aaaaa1` of node `a` contains `[[b]]`, `[[b|Bee]]`,
   `[[c#^m-cccccc]]`, `` `[[b]]` `` (inside inline code), `[[missing]]`, `[[c#^m-zzzzzz]]`,
   `[[c#Heading]]`, `[[topics/c]]`, `[[c.md]]` and `![[c]]`; node `a`'s frontmatter has
   `related: "[[b]]"`. Then: `a`'s outgoing edges, in order, are b, b, c#^m-cccccc, missing (dangling),
   c#^m-zzzzzz (dangling); the inline-code link is not an edge and gives no problem; the heading link,
   the path link, the `.md` link and the embed are not edges, stay as plain text, and each gives a
   `warning`; each dangling edge gives a `warning`. Node `b`'s backlinks are exactly
   `{(a, m-aaaaa1)}` (deduplicated); node `c`'s backlinks are `{(a, m-aaaaa1)}` (a block link counts
   for the node); block `c#^m-cccccc`'s backlinks are `{(a, m-aaaaa1)}`; nothing is derived from the
   frontmatter `related`.
   *Excludes:* every `[[…]]` as an edge; links inside inline code as edges; node backlinks that miss
   block-targeted links; backlinks counted per occurrence; edges read from frontmatter.
8. **Symlinks: real locations win, the walk stays linear** — `notes/shared` is a symlink to a
   directory outside `notes/` containing `shared.md`: node `shared` loads as a first-level node, at path
   `shared`. A directory inside a node that is a symlink back to an ancestor: exactly one `error`
   naming the symlink, the load completes, no node appears twice. **Converging links:** 24 nodes
   `n01`…`n24`, each a real first-level directory with a valid folder note, where each `nI`
   additionally contains symlinks named `nI-a` → `n(I+1)` and `nI-b` → `n(I+2)`: every `nXX` loads
   **once, as a first-level node at its real path** (`n24`'s path is `n24`, not a route through
   `n01`); each of the 45 symlinks gives exactly one `error` naming the symlink; no real directory is
   reported; the load completes in under 2 seconds.
   *Excludes:* refusing symlinked subtrees; infinite recursion on a cycle; ancestor-only cycle checks
   that re-walk a directory by different paths; loading a node at a symlinked route when its real
   location is inside `notes/`.
9. **Loading is total** — no input makes loading panic or return early: a missing `notes/` directory
   yields an empty store and no problems; a `notes/` that is a regular file yields one `error` at
   `.`; a `notes/` whose every node is malformed yields an empty store and exactly one problem per
   skipped top-level subtree; an unreadable folder note (permission denied) gives an `error` for that
   node only, while its siblings load. **Only regular files are read:** a folder note that is a named
   pipe (`mkfifo f/f.md`, beside a valid `a/a.md`), and one that is a symlink to `/dev/zero`, each give
   one `error` for that node, while `a` loads — and the whole load completes within 2 seconds (it
   never opens them). A folder note that is a symlink to a regular file elsewhere loads normally.
   *Excludes:* returning an error for the whole store on the first bad node; opening a non-regular
   file and blocking on it.

## Environment
- Built in a clean-room copy of the committed baseline (no `.git`); `docs/spec/` there holds only the
  two governing specs.
- No live LLM is needed for this unit's tests. The QA gate in `CLAUDE.md` still includes the smoke
  suite, which may use the `.env` endpoint with throwaway state dirs only.
- `golangci-lint --new-from-rev` needs git and cannot run in the room; the delivery lead runs it.

## Notes for the builder
- `internal/skills` already splits YAML frontmatter from a body (`splitDocument`) without mistaking a
  body `---` for the closing delimiter, using `gopkg.in/yaml.v3` (already a dependency). Reuse the
  approach rather than adding a dependency; don't change `internal/skills`.
- yaml.v3 decodes unquoted dates and datetimes into `time.Time`, and both quoted and unquoted forms
  are legitimate Obsidian values — test both.
- Criterion 3's tie-break is path order compared component by component; make it deterministic
  regardless of directory-read order. Decide whether a folder loads before letting it claim a stem,
  and decide whether a folder is skipped before walking anything beneath it.
- Test fixtures must give each problem a sole cause: a fixture meant to exercise one rule must not
  also trip another (a build-1 cycle fixture passed only because the link also lacked a folder note).
- **New lines must be lint-clean**, including tests: run `golangci-lint run --max-same-issues=0
  --max-issues-per-linter=0` on the new package (repo config: `goconst` min-occurrences 2, `gocyclo`
  above 10, `mnd`).

## Rework history
- **Build 1 (from baseline `7b78bd3`) — rejected 2026-11-15.** Verification FAILED criteria 2 and 8 as
  then written: the walker reported problems from inside subtrees before the loader decided to skip
  them, so a skipped parent with a problematic child produced extra problems (the build's own test
  only used children with broken content). Verification also found an exponential walk on
  symlinks that converge on the same directories (24 cross-linked nodes: 16 s, 35,136 problems),
  because cycle detection compared ancestors only. Spec D25 revised (operator rulings 2026-11-15):
  exactly one problem per skipped subtree; each real directory walked once; collisions in component
  order, first *loadable* folder wins; Obsidian-lenient frontmatter; the extended non-paragraph list;
  no edges inside inline code; Obsidian backlink semantics. Criteria rewritten accordingly (now 9).
- **Build 2 (from baseline `756a015`) — rejected 2026-11-15.** Verification PASSED all 9 criteria as
  then written (44/44 mutants caught), but found `Load` hangs forever when a folder note is a named
  pipe (outside the criteria; it would hang the agent once U5 renders from this package). Review also
  ruled that real locations win over symlinked routes (build 2 loaded `n02`…`n24` nested under `n01`
  via symlinks, as path order then required). Spec D25 revised (operator rulings 2026-11-15): only
  regular files are read; real locations win; a description is single-line by decoded value; an
  autolink at line start is an HTML block. Criteria 4, 5, 8 and 9 amended.
- **Build 3 (from baseline `0e7ca02`) — accepted 2026-11-15.**

## Post-verification notes (banked 2026-11-15)
- **Verdict:** accepted at the third build (the skill's stop-rebuilding point). Independent
  verification passed all 9 criteria through the public API; 42 of 45 seeded wrong readings caught;
  12 inverted assertions all failed; FIFO and `/dev/zero` folder notes never hang; 9.5M fuzz runs, no
  panic; smoke 28/28 against the live endpoint; no tampering, scope clean.
- **Carried to U4 as acceptance criteria** (see its skeleton): a custom YAML tag on a whole list loads
  silently instead of erroring (spec violation outside these criteria); on macOS a folder note whose
  name differs only in case loads and warns at once (spec now pins exact-name matching); and two
  untested behaviours (dangling block link as node backlink; one-line `description: |`).
- **Criteria weaker than intended** (for future unit-writing): criterion 7's fixture hides a wrong
  "dangling block link is a node backlink" reading because `a` also links `c` validly; criterion 8's
  symlinks named `nI-a`/`nI-b` also fail for a missing folder note, so the fixture alone cannot
  distinguish "real locations win" — links named after their targets are needed (the builder and
  verifier both added such fixtures); criterion 4 named only a two-line `|` description.
- **Process:** the full smoke suite takes over 30 minutes against the live endpoint, beyond a
  background subagent's limit; builders may need to run it in parts.
