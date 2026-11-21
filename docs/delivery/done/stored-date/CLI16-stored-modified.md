# CLI16 — a note stores the date its content last changed

**Milestone:** stored-date  |  **Builds on:** CLI1–CLI15

## Goal
A node's date is the time its content last changed, kept in the note as the `modified` property,
and no longer the file's time. A link rewrite, a `retag` or a copy of the store no longer makes
an old note look new.

## Scope
- `internal/memory`: reading `modified` into the node's date, with the mtime as fallback; setting
  it on a content change; keeping it (or first writing it from the mtime) on a mechanical
  rewrite; refusing it as a caller's property; the one-line frontmatter edit; the warning for a
  value that cannot be read, which `lint` reports as it reports every library problem.
- `agent/tools/memory_lint.json`: its description becomes III.7's text, which now names that
  warning. Generate it from the spec's quoted text; do not retype it.
- `cmd/memory`: `read`, `search` and `index` show and order by the node's date; `write` refuses
  `--property modified=…`.
- `README.md`: where it describes a note's frontmatter and the dates shown, and the copy advice
  in III.9's known gaps.
- Tests and the CHANGELOG entry.

## Not in scope
- Any other change to `lint`: no new flag, no other problem, no change to an existing reason.
- Anything else under `agent/` (the other tool descriptions, the purpose text and the skills do
  not change).
- A migration verb, a review-by date, any other frontmatter key.
- Any change to what the restructuring verbs do to blocks, links, tags or folders.

## Governing spec
`docs/spec/agent-memory.md`: **III.9** (all of it); III.7 (the report, and the tool file's
description); D25 (*Reading the store*, the `modified`
line and the frontmatter rules); D26 (the round-trip rule and its `modified` exception); D27
(validation on write; the edit-locked node; a new node's layout); D28 ("a write that changes
nothing writes nothing"; roll back); §4.4 (the index's dropping order); §4.5 (search's ranking);
III.8 (which notes each restructuring verb rewrites).

## Prerequisite spec changes
III.9, D34, the in-place revisions that point to III.9, and III.7's amended tool description,
committed with this unit's lock. From that commit the existing test that pins
`agent/tools/memory_lint.json` to the spec fails until this unit updates the file: that is the
unit's work, not a superseded test.

## Existing tests this unit supersedes
Any existing test that pins a note's whole bytes or its frontmatter after a content write, that
expects "frontmatter bytes copied exactly" on a body-only write, or that expects a date taken
from a file's mtime on a note `memory` has written. They cannot be listed before a build exists.
**Do not edit them.** Bring the build far enough to run the whole suite once, then stop and
report every such test in one list (file, test name, the assertion, and what III.9 makes the new
expectation). The delivery lead edits them and you are run again. A test that fails for any
other reason is a defect in the build, not a superseded test.

## Acceptance criteria
All through the real binary, with `TZ=UTC` unless a criterion sets another. `<NOW>` is a value of
the form `YYYY-MM-DDTHH:MM:SSZ` not before the second the call started and not after the second
it ended. "Dated D" below means the note's frontmatter holds `modified: D` and the test has set
the file's mtime to 2020-01-01T00:00:00Z, so the two can be told apart. "Keyless" means the test
has removed the `modified` line from the note by hand. The fixture store `F` is the shipped
starting tree plus, made with `write` and then dated 2026-01-15T10:00:00Z:

- `priya` under `people`, tags `platform`, `atlas`; blocks `<P1>` "Priya leads platform.",
  `<P2>` "Atlas cutover is 2026-12-01."
- `atlas` under `projects`, tag `atlas`; blocks `<A1>` "Owner is [[priya]].", `<A2>` "Date:
  [[priya#^<P2>]]."
- `kettle` under `topics`, no tags; block `<K1>` "Use citric acid."

1. **A new node carries the date, and the date shown comes from it.** `write descaling --parent
   topics --description "How to descale" --add "Rinse twice."` → the note's frontmatter is
   exactly `description: How to descale` then `modified: <NOW>`. Set the file's mtime to
   2020-01-01; `read descaling`, `search rinse` and the index all show `modified` as `<NOW>`'s
   day, not 2020-01-01.
   *Excludes:* still reading the mtime; writing local time or an offset; showing the stored value
   in `read` but not in `search` or the index.

2. **A content change rewrites only that line.** Give `priya` this frontmatter by hand:
   ```
   description: "Priya, who leads platform"   # keep
   tags: [platform,   atlas]
   modified: "2026-01-15 10:00"
   owner: "007"
   ```
   `write priya --add "Prefers async."` → the frontmatter is byte for byte the same except the
   third line, now `modified: <NOW>`. Make `kettle` keyless: `write kettle --add "Descale
   monthly."` → its frontmatter gains `modified: <NOW>` as its last line and is otherwise
   unchanged. Give `atlas` a property D27 cannot re-emit (a literal block holding a raw U+2028):
   `write atlas --add "Go."` succeeds and sets `modified` the same way; `write atlas --tag x`
   is still refused as before.
   *Excludes:* re-emitting the frontmatter (the comment spacing and the flow list would change);
   adding a second `modified` key; moving the key to the end; locking the body of a note whose
   frontmatter cannot be re-emitted.

3. **A call that changes nothing leaves the date.** On `F`: `write priya --description` with its
   current description, `write priya --tag platform --tag atlas`, and `write priya --replace
   <P1>="Priya leads platform."` each exit 0 and leave `priya.md`'s bytes and mtime exactly as
   they were.
   *Excludes:* stamping the date before deciding whether anything changed.

4. **A mechanical rewrite keeps the node's date.** On `F`, make `kettle` keyless, add to it by
   `write` a block "See [[priya]].", make it keyless again and set its mtime to
   2026-03-02T08:30:45.5Z. `rename priya priya-lead` → `atlas.md` still holds `modified:
   2026-01-15T10:00:00Z` and `read atlas` shows 2026-01-15; `kettle.md` has gained `modified:
   2026-03-02T08:30:45Z` as the last frontmatter line and `read kettle` shows 2026-03-02;
   `priya-lead.md`'s `modified` line is unchanged. Then `retag atlas atlas-2026` → `priya-lead`
   and `atlas` still hold 2026-01-15T10:00:00Z.
   *Excludes:* stamping the time of the call on a link rewrite or a `retag`; restoring the mtime
   without writing the key (lost at the next copy); stamping the renamed node.

5. **Each restructuring verb dates the notes whose content it changes, and no others.** Each on
   a fresh `F` with `kettle` holding one more block `<K2>` "Cutover: [[priya#^<P2>]].":
   - `move-block <P2> --to kettle` → `priya` and `kettle` hold `<NOW>`; `atlas` (a link
     rewritten) holds 2026-01-15T10:00:00Z.
   - `split priya --into priya-atlas --parent priya --description "Atlas facts" --block <P2>` →
     `priya` and `priya-atlas` hold `<NOW>`; `atlas` and `kettle` hold 2026-01-15T10:00:00Z.
   - `delete priya --block <P1>` → `priya` holds `<NOW>`; no other note changes.
   - `merge kettle --into atlas` → `atlas` holds `<NOW>`. Then, with a new node `empty` under
     `topics` that has no blocks and no tags, date `atlas` 2026-01-15T10:00:00Z again and
     `merge empty --into atlas` → `atlas.md`'s `modified` line still holds 2026-01-15T10:00:00Z.
   - `move kettle --parent people` → no note's bytes change.
   *Excludes:* dating only the receiving note; dating every note a verb writes; dating `TO` on
   every merge.

6. **Order follows the stored date, not the file's time.** Two nodes under `topics`, each with
   one block containing "zebra": `old-note` holding `modified: 2026-01-15T10:00:00Z` with an
   mtime of now, and `new-note` holding `modified: 2026-06-01T10:00:00Z` with an mtime of
   2020-01-01. `search zebra` lists `new-note` before `old-note`. With the index over its budget
   by one second-level entry, and these two the oldest by either measure, `old-note` is the
   entry dropped.
   *Excludes:* changing the displayed date but leaving ranking and dropping on the mtime.

7. **Values an operator or a plugin wrote are read; others fall back.** With the file's mtime at
   2020-01-01, `read` shows: `modified: 2026-11-21T23:30:00Z` under `TZ=Asia/Tokyo` →
   2026-11-22; `modified: 2026-11-21T23:30:00-05:00` → 2026-11-22; `modified:
   2026-11-21T14:03:00` under `TZ=Asia/Tokyo` → 2026-11-21; `modified: "2026-11-21 14:03"` →
   2026-11-21; `modified: 2026-11-21` → 2026-11-21. For each of `modified: banana`, `modified:
   12`, `modified: [a, b]` and `modified:` with no value, the node loads, `read` shows
   2020-01-01, and `lint` exits 0 with `errors="0" warnings="1"` and one `<problem
   severity="warning">` at the node's path reading `modified: cannot be read as a date or
   date-time; the file's time is used`; none of the readable values above produces a problem.
   Then `write --add` on that note replaces the key, whatever lines its old value took, with the
   one line `modified: <NOW>`, and `lint` reports `warnings="0"`. On another such note, a
   `rename` that rewrites a link in it replaces the value with the file's prior mtime and clears
   the warning too.
   *Excludes:* taking an offset-less value as UTC (the YAML library does); failing the node, or
   exiting 2, on a value that is not a date; warning on a readable value; leaving a list value's
   lines behind under the new line.

8. **Only `memory` sets it, and a failed call does not.** `write priya --property
   modified=2020-01-01` and `write newnode --parent topics --description d --property
   modified=x` → exit 1, stderr holds `--property: "modified" is set by memory`, nothing under
   the state dir changes. `write priya --property Modified=x` is accepted as an ordinary
   property. A `rename priya priya-lead` that fails part-way and rolls back (as the existing
   all-or-nothing tests force it) leaves every note's bytes, `modified` lines included, as they
   were.
   *Excludes:* letting the caller back-date a note; matching the key without regard to case;
   leaving a stamped `modified` behind after a roll back.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- The QA gate is in `CLAUDE.md`. The whole tree stays lint-clean.
- The node's date is read in one place today (`internal/memory/load.go`, `n.Modified`) and used
  in `cmd/memory/read.go`, `search.go`, `index.go` and `internal/memory/search.go`. Keep one
  field carrying the node's date so those users need no second rule.
- Every restructuring verb replaces notes through the helpers in
  `internal/memory/restructure.go`; `write` goes through the encoder (`encode.go`). The
  one-line edit must give the same bytes on both routes: write it once and call it from both.
- The time of the call must be replaceable in library tests. The library has no clock today;
  add the smallest seam that works, unexported.
- A mechanical rewrite of a note with an unreadable `modified` value replaces that value with
  the file's prior mtime, as for a keyless note (III.9).
- yaml.v3 decodes an unquoted timestamp without an offset as UTC. Read the scalar's text.
- The library does not log, and takes no new dependency.

## Post-verification notes (2026-11-21)
- One build. The builder finished the whole unit instead of stopping at the first superseded
  test, and listed 37 of them (30 failing always, 7 only when two calls fell in different
  seconds). The delivery lead edited them against the finished build (15 files, listed in the
  CHANGELOG entry); there was no second builder run. Operator accepted this.
- Verifier: all eight criteria PASS through the real binary; 26 of 27 mutants killed, the
  survivor equivalent (`.UTC()` applied twice). Gate green, vulnerability scan included.
- Production: `internal/memory/modified.go` (new: reading, the stamp rule, the one-line edit,
  the unexported clock seam `now`); `frontmatter.go`, `load.go`, `store.go`, `encode.go`,
  `rewrite.go`, `verify.go`, `validate.go`, `restructure.go` and each restructuring verb's file;
  `cmd/memory/write.go` (the refusal). Tests: `internal/memory/modified_test.go`,
  `cmd/memory/modified_bin_test.go`.
- Rulings and known gaps are in III.9, "Settled and open at CLI16's banking". Criterion wording
  found loose by the verifier: criterion 1 alone cannot tell "today" from the stored date
  (criteria 6 and 7 do); criterion 6's "oldest by either measure" is not literally possible, and
  "over budget by one entry" is reachable only through the byte budget; criterion 2's
  edit-locked fixture is `note: |+` followed by a line holding only U+2028.
- Lesson: when a unit changes what every write emits, expect the superseded-test list to be
  long and partly time-dependent; a single suite run does not find the flaky ones.
