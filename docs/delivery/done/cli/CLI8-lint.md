# CLI8 — `memory lint` reports every problem in a store

**Milestone:** cli  |  **Builds on:** CLI7

## Goal
An operator can check a whole store after outside edits (Obsidian, git, sync, scripts) with one
command. `memory lint` prints every problem the reader finds, and its exit code says whether any
node is missing from the store because of an error.

## Scope
- `docs/spec/agent-memory.md` **III.7** (new, in this unit's lock commit), III.2's `lint` row,
  III.3's exit 2, the III.4 known-gap rewording, D32.
- `cmd/memory`: the `lint` verb (no arguments; III.7's XML; exits 0, 2 or 1); the usage text lists
  it. `run` must be able to print a verb's stdout and still exit 2.
- `README.md`: the Usage section documents `lint` (what it reports, the three exit codes, and an
  example of using it in a cron job or git hook); the status line lists the verb.
- Tests and the CHANGELOG entry.

## Not in scope
- Repairing anything (`--fix`), a `--strict` mode, failing on warnings.
- A tool file or live-context entry for `lint` in `agent/`.
- Any change to the library's checks or reason texts, or to `read`, `search`, `index`, `write`.

## Governing spec
- `docs/spec/agent-memory.md`: III.7 (whole), III.1 (resolution, missing store), III.3, D25 (what
  the reader checks), §4.2 *Reading the store*.

## Existing tests this unit supersedes
None.

## Acceptance criteria
`W` has `W/.agent/memory/` copied from the shipped starting tree (`seeded()`), unless stated.

1. **A clean store passes.** On the starting tree: `lint` → exit 0, stderr empty, stdout
   `<memory_lint nodes="4" errors="0" warnings="0"></memory_lint>` (or the self-closed form that
   `encoding/xml` emits; it parses with no `<problem>`). On a fresh `W` with an empty `W/.agent`:
   `nodes="0" errors="0" warnings="0"`, exit 0, and `W/.agent` stays empty.
   *Excludes:* treating a missing store as an error; creating the store.
2. **Errors fail with exit 2 and a full report.** Add `notes/b/b.md` with no frontmatter,
   `notes/topics/c/c.md` whose body has a `# heading` line, and a valid child
   `notes/topics/c/d/d.md` under the broken `c`. `lint` → exit **2**, stderr empty, `errors="2"`,
   `nodes="4"` (the four seeds: `d` is in `c`'s skipped subtree), and the `<problem>` elements equal,
   in order, `memory.Load(notes).Problems()` — severity, path and reason, exactly.
   *Excludes:* exit 1 for findings; the report on stderr; counting skipped nodes; rewording reasons.
3. **Warnings alone pass.** On the starting tree, add a dangling `[[nowhere]]` link in a valid node,
   a stray file `notes/topics/notes.txt`, and the same block ID in two nodes. `lint` → exit 0,
   `errors="0"`, `warnings` equal to the number of warning problems the library reports (at least
   3), each listed.
   *Excludes:* failing on warnings; dropping warnings from the report.
4. **The report is real XML.** A problem whose reason contains `<`, `>` or `&` (e.g. a body line
   `# a<b & c>`) decodes back to the library's exact reason text. Problems appear in the library's
   order (sorted by path), and a problem about `notes/` itself has `path="."`.
   *Excludes:* hand-built XML without escaping; re-sorting or grouping by severity.
5. **When lint cannot run, exit 1.** `lint extra` → exit 1, one line on stderr, empty stdout;
   `--parent-dir W/nope lint` → the missing-parent line of III.1, exit 1; `W/.agent/memory` a regular
   file → `<path> is not a directory`, exit 1. In every case nothing is created.
   *Excludes:* exit 2 or 0 for a failure to run; a report printed alongside an error.
6. **Lint writes nothing.** On a store with errors and warnings, with mtimes aged: after `lint`, the
   whole `W/.agent` tree is unchanged (bytes, mtimes, modes) and no `.write.lock` exists. The usage
   text lists `lint`; `agent/` ships no lint tool file (the existing `TestAgentFolderIsComplete`
   still passes unchanged).
   *Excludes:* taking the write lock; creating the store; shipping a tool file.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- `memory.Load(notes).Problems()` already holds everything; `Store.Nodes()` gives the loaded nodes.
- `run` currently writes stdout only when a verb returns no error. Exit 2 needs stdout written and
  a distinct exit; keep exit 75 and exit 1 behaviour exactly as they are.
- The read verbs' store lookup (`notesDir()`, which creates nothing) is the one to use.
- Keep the tree lint-clean (`golangci-lint run ./...`, full tree).

## Post-verification notes (2026-11-19)
- Built in one pass. Pre-filter green; no existing test touched. Production change:
  `cmd/memory/lint.go` (new, ~60 lines) and an `errLintFound` branch in `run` (exit 2, report on
  stdout). New tests `cmd/memory/lint_bin_test.go`.
- Verifier: all six criteria PASS through the real binary, results compared line by line with
  `memory.Load().Problems()`; `lint` returned in 17 ms while another process held the write lock;
  exit 75 unchanged; 18 of 19 mutations caught; 7/7 inverted assertions failed; tamper and scope
  clean; govulncheck clean.
- Survived: rendering a non-empty report unindented (`xml.Marshal`) — no criterion pins it. Low
  value; left.
- Builder spec gaps recorded as III.4 known gaps: unreadable `memory/` (exit 1) vs unreadable
  `notes/` (exit 2); XML's U+FFFD substitution in paths; `lint --` accepted. AC1's "self-closed
  form" wording was wrong (`encoding/xml` never self-closes); harmless.
