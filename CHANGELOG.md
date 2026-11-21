# Changelog

Dated entries, newest first. Each header is a unit of work; bullets capture the
detail.

## 2026-11-21 - The capture skill asks for a claim's source, and the spec records why OKF was not adopted and what sharing memory would need

From the review of the Open Knowledge Format (`docs/research/okf-recommendations.md`). Text and
documentation only: no Go changed.

- `agent/skills/memory-capture/SKILL.md`, `docs/spec/agent-memory.md` (III.8, the skill's text):
  one line added to "How to write it": when a claim rests on a file, a link, a message or a
  person, say which. Why: a claim that names its source can be re-checked; one that does not can
  only be believed or dropped. Plain prose, not an inline field, because nothing reads it but the
  model. **An existing install must copy the skill again** to get the line.
- `docs/spec/agent-memory.md` (§6, out of scope): the gaps a shared or replicated store would
  have to close (no record of which agent wrote what; stem collisions, unserialised writes and
  unrewritten links across stores; no merge strategy for replicas), and the OKF decision: not
  adopted as the store format, with the conditions for revisiting. Recorded there too: a per-claim
  review-by date is deferred to the audit's design, and needs no store change.
- Not done: no smoke or binary test beyond the existing one that pins the skill file to the
  spec's text byte for byte, which covers this line.

## 2026-11-21 - A flag given twice is refused instead of silently taking the last value

Closes the known gap recorded at CLI14's acceptance (spec III.8, "A flag given twice"). Before,
`merge x --into a --into b` merged into `b` and said nothing; a model that assembles a call badly
got a restructuring it did not ask for. A direct fix, not a delivery unit: one check in the shared
parser.

- `cmd/memory/args.go`: `parseArgs` refuses a flag set a second time with `<flag as typed>: given
  more than once` (exit 1, nothing changed), under either spelling (`--into` then `-into`) and
  even when the two values agree. It takes the names of the flags that may repeat. Booleans are
  refused too (`--recursive --recursive`), so there is one rule and no exception to remember.
  Trap: the flag package has already stored the second value when the check fires; that is
  harmless only because the call stops there. Rejected: refusing only when the values differ (a
  second rule, for no caller's benefit).
- `cmd/memory/write.go`, `cmd/memory/split.go`: declare what repeats: `write`'s `--tag`,
  `--property`, `--add`, `--replace`, and `split`'s `--block`. **Behaviour change for `write` and
  `read`**, which have taken the last `--parent`, `--description`, `--block` or `--depth` since
  they shipped. A repeated `--property` key still takes the last value.
- `cmd/memory/unit_test.go`, `cmd/memory/split_merge_bin_test.go`: the parser's rule, and through
  the built binary one refusal per verb that takes a flag, each checked to change nothing.
- `docs/spec/agent-memory.md`: III.8's known gap becomes the rule. The same section records the
  operator's decisions of this date: `agent/tools/` is a library and the documented install
  copies all of it (the operator removes what an agent should not have); `split`'s new node
  getting no tags or properties, and `merge` refusing a carried-entry name collision, are
  accepted as built.
- Not done: no change to the READMEs or the tool descriptions. None of them describes repeating
  a single-value flag, and the tool descriptions are pinned to the spec byte for byte.

## 2026-11-21 - An agent can be given the restructuring verbs, is told on every turn only what memory is for, and learns how to use it from three skills

CLI15, the last restructuring unit (spec III.8, "Tool files", "Tool descriptions" and "The prompt
and the skills"; III.4). Everything in this entry is under `agent/`, the READMEs and tests: no
production Go changed, no verb accepts, prints or refuses anything it did not before. **An existing
install must delete `<state dir>/prompt/memory_usage.md`** when it copies in `memory_purpose.md`,
or the model reads both on every turn; both READMEs' install commands do it.

- `agent/tools/memory_move.json`, `memory_rename.json`, `memory_move_block.json`,
  `memory_split.json`, `memory_merge.json`, `memory_delete.json`, `memory_retag.json` (new): one
  shell-tool file per restructuring verb, each `{"command": "memory", "prepend_args":
  ["--parent-dir", "${AGENT_STATE_DIR}", "<verb>"], "read_only": false, "allow_user_args": true,
  "forbidden_args": []}` with III.8's description, generated from the spec's quoted text, not
  retyped. Trap: the file is `memory_move_block.json` but its verb is `move-block`; `move_block`
  is an unknown verb. They ship together so that no release offers the model part of the set the
  `memory-restructure` skill describes. Nothing pins `--recursive` on or off: the operator
  withholds it by editing the installed file (see `agent/README.md` below).
- `agent/prompt/memory_purpose.md` (new), `agent/prompt/memory_usage.md` (deleted): the every-turn
  prompt file is now III.8's purpose text, two paragraphs on what memory is and what it is for.
  "What to save", "How to write it" and "Before you answer" left it for the skills. The cost,
  accepted in the spec: that advice is no longer in front of the model on every turn, only each
  skill's one-line description is.
- `agent/skills/memory-usage/SKILL.md`, `agent/skills/memory-capture/SKILL.md`,
  `agent/skills/memory-restructure/SKILL.md` (new): III.8's three skill texts, byte for byte, in
  the agent's skill format (frontmatter of exactly `name` and `description`, the folder named as
  `name`). No body names a node, and each refers to tools by what they do, since an operator may
  install only some.
- `agent/README.md`: the table lists the seven tool files, the purpose text and the three skills,
  with the new destination `<state dir>/skills/`; the install commands make `skills/`, copy the
  three skill folders, copy `memory_purpose.md` and `rm -f` the old `memory_usage.md`. New section
  "The restructuring tools": each is granted by installing its file and withheld by leaving it
  out, and `--recursive` is withheld from an installed `memory_delete.json` with
  `"forbidden_args": ["--recursive", "-recursive"]`, both spellings because the agent refuses a
  forbidden argument by exact match and `memory` takes a flag with one dash or two
  (`--recursive=true` needs no entry: `memory` refuses a value on the flag). "The guidance" became
  "The purpose text and the skills", which says the skill bodies are read only when the model
  loads them and which skills to leave out with which tools. The exit-75 sentence now covers every
  description but `memory_lint.json`'s. Removed: "no verb moves, renames or deletes a node, so it
  can never remove one", which the tool files make false. Decided where the spec is silent: the
  `rm -f` is in the install commands rather than only in prose, so that following the README
  cannot leave both prompt files in place; `rm -f` because a fresh install has no such file.
- `README.md`: the status line, the wiring guide's opening and step 3 name `memory_purpose.md`,
  the three skills and the twelve tool files; step 3's commands install the skills and remove the
  old prompt file, as `agent/README.md`'s do. The Usage section and the store paragraphs were
  checked and needed no change (no verb changed).
- `cmd/memory/main.go`: checked, not changed. Neither the usage text nor the package comment names
  a shipped file, so there was nothing to rename; a test now fails if either names
  `memory_usage.md`.
- `cmd/memory/restructure_tools_test.go` (new): each of the seven files against the spec (exactly
  the six keys, the pinned verb, not read-only, `forbidden_args` `[]`, the description read from
  III.8); no description says "area", names a starting-tree stem or mentions `--parent-dir`; and
  every example in every description is read out of the description (its JSON arrays) and run
  through its tool file as the agent runs it, on a fixture built with the shipped
  `memory_write.json`, checking what the verb did. The fixture uses the example's own stems and
  tags, and replaces only block IDs and `<parent stem>`. Every `["` in a description must open an
  example, so the extractor cannot silently miss one.
- `cmd/memory/skills_test.go` (new): the purpose text (byte-equal to the spec, no `##` heading,
  none of the old sections, alone in `agent/prompt/`) and the skills (byte-equal to the spec, the
  only file in a folder named as `name`, loadable by the format rules of III.8, written out as
  `skillFormatProblems`). `TestSkillFormatCheckRefusesBadSkills` runs that check over ten broken
  variants of a shipped skill, so the check is known to refuse what the agent's loader refuses.
  Not done: loading the skills with the agent's own loader, which lives in the agent repo.
- `cmd/memory/agent_docs_test.go` (new): both READMEs' install commands, run on a state dir that
  has the old prompt file, an operator's own prompt file and an operator's own skill, remove only
  the old file and install everything; `memory_usage.md` is named nowhere except in a line that
  says to delete it; the README's `forbidden_args` are parsed out of it and shown to be both
  needed (`delete X -recursive` deletes a subtree) and enough (`--recursive=true`, `-recursive=true`
  and the `=false` forms are refused by the binary and delete nothing). Some checks there pin
  README wording by pattern ("exact match", "one dash or two", "only when the model loads"):
  prose cannot be tested any other way, and a reworded README must keep saying those things.
- Existing tests: none edited. The nine that were red at the lock (the eight the unit lists and
  `TestToolFilesSchema`) are green. Comments in older test files that say "the four tool files" or
  "placing usage.md" now read stale and were left, as the unit says.
- Docs and agent files checked for the old behaviour: `README.md`, `agent/README.md`, every file
  in `agent/tools/`, `agent/livecontext/memory_index.json`, `agent/prompt/`, `agent/skills/`,
  `agent/memory/notes/` (unchanged), the usage text and package comment. The five existing tool
  files and the live-context entry are byte-for-byte unchanged; `memory_write.json` keeps "Tags
  cannot be cleared and blocks cannot be deleted.", which is true of `write`.
- Not done, by scope: a dream or audit recipe, a `memory-lint` skill, any agent-side change. No
  dependency added (`gopkg.in/yaml.v3`, already required, is now also imported by a test in
  `cmd/memory`).

## 2026-11-20 - A node that covers two subjects can be divided and two nodes about one subject joined, each in one call: `split` and `merge`

CLI14, the fourth restructuring unit (spec III.8, "`split`, exactly" and "`merge`, exactly").
With it all seven restructuring verbs exist in the binary. No tool file, skill or prompt text ships
for them yet (CLI15): `agent/` is byte-for-byte unchanged, so no agent can call them until then,
and `agent/README.md` and the shipped tool descriptions still say nothing can be deleted or
cleared.

- `internal/memory/split.go` (new): `Writer.Split(SplitRequest)`. Under the write lock and from a
  fresh load it resolves the blocks, checks the new node as `write` checks a create (`checkCreate`,
  unchanged, with its refusals renamed to the field `into`), works out every note, then makes the
  new node (`makeNode`), replaces the changed notes (`replaceAll`), reloads and checks the store
  with `unsound`. The new note is laid out by the encoder's own new-note code (`newNote`), so it
  is byte-equal to what `write` creates, each block carrying the ID it already has; it gets no
  tags and no properties. The old note loses the blocks by the same byte surgery `delete --block`
  uses. Only edges whose stem is the split node *and* whose block is one of those moved are
  retargeted, the moved blocks' own text included; `[[STEM]]` stays. Order of work, and why: the
  new node is made *before* the old note is rewritten, so a crash part-way leaves the blocks in
  both notes (a duplicate-ID lint warning), never in neither. Refusals, each a `FieldError`
  naming `stem`, `into`, `parent`, `description` or `block`, or a `NoNodeError` for an unknown
  stem or parent: what the arguments alone show (the root, an invalid new stem, a missing parent,
  a bad description, no block, an empty ID, an ID given twice) is refused before the lock is
  taken; then an unknown stem, a block not in the node, an ID more than one block carries (as
  `move-block`), a new stem in use, an unknown parent, and whatever `checkCreate` refuses.
  Decided where the spec is silent: an unknown `--parent` is a `NoNodeError` (near matches at the
  CLI) rather than `write`'s "no node exists to create under" text; a loaded node's stem as `NEW`
  gets its own message ("in use by the node at …"), because `checkCreate` would call it "a folder
  the reader skipped (no problem reported)"; a moved block keeps the reader warnings its text
  already had and a block that is only an ID arrives as `^ID`, as `move-block` does (the new note
  is not run through `write`'s block-text validation: the text is already in the store).
- `internal/memory/merge.go` (new): `Writer.Merge(from, to)`. One replace of the kept note can
  carry three kinds of edit: the folded-in node's blocks appended (as `move-block` appends one),
  its own links to that node retargeted, and the tag list re-emitted, only when a tag is added.
  Then every entry of the folded-in folder except its note and dot-entries is renamed into the
  kept node's folder (loaded children, folders that did not load, stray files), the folder is
  renamed aside to `.<stem>.deleting`, the store reloaded and checked, and only then is the hidden
  folder removed, as `delete` does. Notes are replaced *before* any folder moves, so their paths
  are the ones loaded; the read-back after the reload looks for them where the move put them.
  Refused: the root (before the lock), an unknown node, `TO` equal to `FROM` or under it, and an
  entry that would move having the name of one already in `TO`'s folder (`lstat`, so it also
  catches a name differing only by case on a case-insensitive filesystem). Decided where the spec
  is silent: the collision names the field `into`; the folded-in note must still hold the bytes
  that were read (`delete`'s edit-race check, exit 75) because it is discarded; a kept note that
  gains nothing (no block, no tag, no link) is not written; a hand-written paragraph with no ID
  in the folded-in node arrives as text with no ID ("IDs and text kept"); when the reload finds a
  replaced note edited by something that does not take the lock, the call exits 75, rolls nothing
  back and still removes the hidden folder, so the merge is complete on disk, as `rename`'s is.
  Known gaps, not fixed: `delete`'s (the hidden folder that cannot be removed, the crash between
  rename-aside and removal) and `rename`'s (restored notes carry new mtimes); a merge that would
  give the kept note two blocks with one ID, or carry a hand-written paragraph ending in a stray
  `^token`, is refused by the note check with the encoder's "reader warns about" text rather than
  a message of its own; a crash between replacing notes and moving entries leaves links rewritten
  and the folded-in node still there.
- `internal/memory/restructure.go`: `noteChange` now takes several `drops`, several `adds` and
  an optional tag list, where it took one block to drop and one to add; `rewritten` applies them.
  Several drops are applied as deleting the blocks one at a time (`withoutBlocks`, built on the
  unchanged `dropRange`), which is what "each block leaves the note as `delete --block` removes
  one" needs: taking the union of single-drop ranges instead leaves a trailing blank run when the
  last two blocks go. A test compares a split with one-by-one deletes for every subset of a
  five-paragraph note, in both orders. `withLastBlock` writes a block with no ID as its text
  alone (`paragraph`). `retargeting` and `retaggedHead` are lines lifted out of `moveblock.go`
  and `retag.go`. No behaviour of `delete`, `retag`, `rename` or `move-block` changes: their
  tests pass untouched.
- `internal/memory/delete.go`, `moveblock.go`, `rename.go`, `retag.go`: call sites follow
  `noteChange`'s new fields and the two lifted helpers. No behaviour change.
- `internal/memory/encode.go`: `EncodeNew`'s layout is lifted into `newNote`, which `Split`
  shares. `internal/memory/write.go`: `create`'s folder-and-note step is lifted into `makeNode`,
  which `Split` shares, with its roll back. No behaviour change to `write`.
- `cmd/memory/split.go`, `cmd/memory/merge.go` (new): the verbs. Argument errors name the
  argument as typed (`STEM`, `FROM`, `--into`, `--parent`, `--description`, `--block`, or `-into`
  when typed so) and come before the store is looked at: a missing positional, `/` as `STEM`,
  `FROM`, `NEW` or `TO`, a missing flag (the first missing in usage order), an invalid `NEW`.
  Results are `<memory_split from stem parent path blocks left links_rewritten>` and
  `<memory_merge from stem blocks children tags_added links_rewritten>`. `merge`'s first
  argument is named `FROM` in errors, as the spec's table names it, not `STEM`.
- `cmd/memory/main.go`: both verbs registered; the usage text and package comment list them, and
  the lock sentence now names every writing verb once (`write`, `delete`, `retag`, `move`,
  `rename`, `move-block`, `split` and `merge`), replacing "… and so does move-block".
- `README.md`: the Usage section documents both verbs with their results and refusals; the
  status line, the lock-file sentences, the root paragraph and the exit-75 sentence list them.
- Tests: `internal/memory/split_merge_test.go` (new) pins the bytes of every note, injects a
  failure at each step of each verb (the new note, each replace, each entry move, the
  rename-aside, the reload, an outside edit) and checks the lock; `cmd/memory/
  split_merge_bin_test.go` and `split_merge_busy_bin_test.go` (new) run the unit's eight
  acceptance criteria through the built binary.
- Docs checked: spec III.8 (already amended in this unit's lock), the usage text and package
  comment, `README.md`. Deliberately not touched, being CLI15's: `agent/README.md`,
  `agent/tools/`, `agent/prompt/`, `agent/livecontext/`. Not changed and now stale: spec III.1's
  list of verbs that take a lock file names five restructuring verbs, not `split` and `merge`
  (the builder does not edit the spec).

## 2026-11-20 - A single block can change node: `move-block` moves it with its ID and rewrites every link to it, and `rename` refuses a new stem that `write` would refuse to create

CLI13, the third restructuring unit (spec III.8, "`move-block`, exactly", and the paragraph of
"`rename`, exactly" that begins "`NEW` is checked as strictly as `write` checks a new stem").
`split` and `merge` are not built (CLI14), and no tool file, skill or prompt text ships for the
restructuring verbs yet (CLI15): `agent/` is byte-for-byte unchanged, so no agent can call them
until then. This is the second build of the unit (the doing file's "Rework 1"): the target note is
appended to by byte surgery, not through `write`'s encoder.

- `internal/memory/moveblock.go` (new): `Writer.MoveBlock(id, to)`. Under the write lock and from
  a fresh load it finds the one block carrying the ID, works out every note that changes, replaces
  them (`replaceAll`), reloads, and checks the store with `unsound`. Three kinds of edit, and one
  note can need two of them in the same replace: the old note loses the block (and has its other
  links to the block retargeted), the target gains it (and has its own links retargeted), and
  every other note holding a link to the block has that link retargeted. The moved block's links
  to itself are rewritten in its text as it moves, and counted. Only edges whose stem is the old
  node *and* whose block is the ID are touched: `[[old]]` stays, a `[[to#^ID]]` that dangled
  before now resolves uncounted, and `[[other#^ID]]` is left dangling. Refusals, in the spec's
  order, each a `FieldError` naming `id` or `to` (or a `NoNodeError` for an unknown target): the
  root as target (before the lock is taken, so nothing is created), no block with the ID, the ID
  on more than one block (across nodes or twice in one), the target unknown, the block already
  there. The trap: the "more than one block" check must come from a scan of every loaded block,
  not from `Store.Block`, which answers with the first holder and would silently move it.
  Decided where the spec is silent: an empty ID is refused naming `id` before the lock; the
  duplicate-ID message lists the nodes that hold it.
- `internal/memory/restructure.go`: `rewritten` and `noteChange` (new) are the one place a
  restructuring call edits a note by byte surgery: per-block cuts on link targets, one block
  removed, one block appended, then the output is loaded through the folder-note parser
  (`verifyOutput`) before anything is written. It is the bodies of `withoutBlock` (`delete
  --block`) and `relinked` (`rename`) lifted into one function; both now call it, with no
  behaviour change (their tests pass unchanged). `dropRange` is `withoutBlock`'s line arithmetic,
  lifted. `keptBytes` is the "replaced note holds other bytes at the reload" loop of
  `replaceNotes`, lifted so `MoveBlock` uses the same check and the same `ErrChangedOnDisk`
  wording. Rejected: giving `MoveBlock` its own copy of the removal and relinking code (a second
  rule for what a removed block takes with it).
- `internal/memory/restructure.go`, `withLastBlock` (new): how the block is appended. The note's
  bytes are kept; its last line gets a line ending if it had none; one blank line is added; then
  the text, a space, `^ID` and one line ending, all in the note's own line ending (the opening
  delimiter line's, as the encoder uses). So an ID-less hand-written paragraph in the target is
  not given an ID, a run of blank lines is not collapsed, a CRLF note stays CRLF and a byte-order
  mark stays. This is deliberately not `write --add`'s encoder, which does adopt and collapse:
  that was the first build's defect. Decided where the spec is silent or pulls two ways: a target
  that already ends in one or more blank lines keeps them and gets none added ("every byte stays"
  wins over "exactly one blank line", which then means at least one); a target with no blocks gets
  the frontmatter, one blank line and the block, dropping any blank lines that followed the
  frontmatter, because the spec says such a note is "laid out as `write` lays out a first block";
  a block whose ID was written on a line of its own arrives as text, space, `^ID` (the spec's
  form), and a block that is only an ID arrives as `^ID` alone, since a leading space before it
  would be a stray byte. The moved text keeps the reader warnings it already had (an unsupported
  `[[x#Heading]]`, an embed): they are added to what `verifyOutput` allows for the target,
  otherwise such a block could never be moved.
- `internal/memory/rename.go`: `refusal` now also calls `findFolders`, the walk `write`'s create
  check uses, so `NEW` is refused when any folder in the tree carries it, loaded or not (the
  message quotes the reader's reason for skipping it, as `write`'s does), and when it differs from
  a folder's name only by letter case. No second collision rule: it is the same walk, comparing
  names, so the case check does not depend on the filesystem. It runs after the own-stem and
  loaded-stem checks and before the two `Lstat` checks, so on a case-insensitive filesystem a
  case-different folder beside the node is reported as a case collision, not as "an entry already
  there". `write`'s own messages are untouched; `rename`'s are worded for `NEW`. `retargetCuts`
  (new) is `relinkCuts` with the edge test as a parameter, so `move-block` matches on stem and
  block ID with the same scanner positions; `relinkCuts` calls it, unchanged in behaviour.
- `internal/memory/delete.go`: `withoutBlock` is now one call of `rewritten`. No behaviour change.
- `cmd/memory/moveblock.go` (new), `main.go`: the verb `move-block ID --to STEM`, its result
  `<memory_move_block id from stem links_rewritten>`, and its errors: `ID:` or the flag as typed
  (`--to`, `-to`), an unknown `--to` with near matches, a surplus argument quoted as `rename`
  quotes one. Missing arguments, an empty `--to` and `--to /` fail before the store is created.
  `ID` is passed as typed (`^m-…` with a caret is an unknown ID, as in `read` and `delete`). The
  usage text and the package comment list the verb and `rename`'s stricter check. The trap in the
  usage text: `TestUsageListsMoveAndRename` (CLI12) pins the sentence "write, delete, retag, move
  and rename take the store's write lock", so the verb is added after it ("and so does
  move-block") rather than inside the list; the test is unchanged.
- `README.md`: the Usage section gains the `move-block` command line, a paragraph on what it
  does, refuses and prints, and the stricter `rename` check; the status line, the lock and
  retry sentences and the root paragraph list the verb.
- Tests: `internal/memory/move_block_test.go` (new) covers the bytes of both notes (line endings,
  byte-order mark, missing final newline, blank-line runs, ID-less paragraphs, an empty target),
  link counting, the refusals and their order, a failure injected at each note replacement and at
  the reload, an outside edit (`ErrChangedOnDisk`), the lock, and `rename`'s new checks.
  `cmd/memory/move_block_bin_test.go` and `move_block_busy_bin_test.go` (new) run the unit's eight
  acceptance criteria through the built binary on the fixture store the unit describes.
- Docs and agent files checked: `docs/spec/agent-memory.md` III.8 already holds "`move-block`,
  exactly" and the `rename` ruling (committed with the unit's lock; not edited here).
  `agent/README.md`, `agent/tools/*.json`, `agent/prompt/memory_usage.md`,
  `agent/livecontext/memory_index.json` and `agent/memory/notes/` are deliberately unchanged:
  they are CLI15's, and `agent/README.md` still says no verb moves, renames or deletes a node
  until then (a recorded gap, III.8). No new dependency.

## 2026-11-20 - A node can change place and name: `move` relocates it with its subtree, and `rename` gives it a new stem and rewrites every link to it

CLI12, the second restructuring unit (spec III.8, "`move`, exactly" and "`rename`, exactly").
`move-block`, `split` and `merge` are not built (CLI13, CLI14), and no tool file, skill or prompt
text ships for these verbs yet (CLI15): `agent/` is byte-for-byte unchanged, so no agent can call
them until then.

- `internal/memory/move.go` (new): `Writer.Move`. One `rename(2)` of the node's folder into the
  parent's folder (`notes/` itself for `/`), under the write lock and from a fresh load. No note is
  read or written, so every file keeps its bytes and mtime and whatever else is in the folder
  (unloaded children, stray files) goes along. Refused: an unknown stem or parent (`NoNodeError`,
  whose `Stem` says which), the node itself or its own descendant as parent, and an entry of the
  node's name already in the parent's folder (checked with `Lstat`, so a plain file or a folder
  that did not load both count). A move to the parent the node already has returns its result
  before touching anything. The store is reloaded after the rename and the folder renamed back if
  the check fails. Not done: D28's "changed on disk" check. A move replaces no bytes, so an edit
  made by something that does not take the lock is carried along intact; there is nothing to
  clobber and nothing to refuse.
- `internal/memory/move.go`, `unsound`: the postcondition check for both verbs, and the trap in
  this unit. CLI11's `newErrors` compares whole problems (path and reason). After a move or rename
  that is wrong twice over: a folder inside the node that did not load is the same error at a new
  path, and reasons quote paths the call changes (`stem "x" is already used by <path>`, `cannot
  read folder: open <path>`). Compared that way, every move carrying an unloaded folder was rolled
  back, against "whatever else is inside the folder goes with it" (the first run of the library
  test showed it). So here an error is identified by the folder it skips: the paths of the errors
  before the call are passed through the relocation (old folder to new), and the call fails only
  when the store afterwards skips a folder it did not skip before, or the node did not load at
  its new path. `newErrors` itself is unchanged and still serves `delete` and `retag`.
- `internal/memory/rename.go` (new): `Writer.Rename` and `CheckStem`. Order: every rewritten note
  is worked out first; the notes are replaced where they are (the renamed node's own included);
  the folder note is renamed to `NEW.md`; the folder is renamed; the store is reloaded and
  checked. A failure at any step undoes the steps before it, so the store is as it was: tested
  for a note that cannot be replaced, each of the two renames, and the reload. Links are found
  with `scanLinkSpans`, the reader's own scanner (no second parser): for each edge whose stem is
  the old one, the bytes right after `[[` are replaced, so aliases and block IDs are untouched and
  a `[[STEM]]` in inline code or in an unsupported form, which is not an edge, is left alone. The
  replacement is made in the block's source lines (`sourceCuts`, as `retag` does), not by
  re-encoding the note: line endings, a byte-order mark, ID-less paragraphs, blank-line runs and
  a missing final newline all stay. The output is still loaded through the folder-note parser
  (`verifyOutput`) before anything is written. Refused, naming `new`: a stem that is not valid,
  the node's own stem, a loaded node's stem, an entry named `NEW` beside the folder. Decided where
  the spec is silent: a file `NEW.md` already inside the node's folder is refused too, because
  renaming the folder note onto it would silently overwrite it; a folder named `NEW` elsewhere in
  the tree that did not load is not refused (the spec lists only loaded stems and the entry beside
  the folder; `write` is stricter for creates); the stem syntax is checked before the lock is
  taken, so `rename nowhere Bad_Stem` names `NEW`, not `STEM`. A replaced note found holding other
  bytes at the reload is `ErrChangedOnDisk` and is not rolled back over, as D28 rules and
  `replaceNotes` does; the rename is then complete on disk and the retry will say the old stem is
  unknown (recorded as a gap, not fixed).
- `internal/memory/restructure.go`: `replaceNotes` is split, behaviour unchanged. `replaceAll`
  (new) is its first half, replacing several notes and restoring those already replaced when one
  fails; `rename` needs the list it returns to restore the notes after a later step fails.
- `internal/memory/retag.go`: `cutBlock` (new) is the three lines of `retagBody` that apply cuts
  to a block's source lines, lifted out so `rename` uses the same code. No behaviour change.
- `cmd/memory/move.go`, `rename.go` (new), `main.go`, `read.go`: the verbs, their results
  (`<memory_move stem from parent path>`, `<memory_rename from stem links_rewritten>`, via
  `encoding/xml`), the usage text and the package comment. Argument errors come before the store
  is created and name the argument as typed: `STEM`, `--parent` (or `-parent`, if typed so), `NEW`,
  or the surplus argument itself (`rename a b c` names `"c"`). `/` is refused as `STEM` and as
  `NEW`, and accepted only as `--parent`. An unknown `--parent` lists near matches like an unknown
  stem (`unknownNodeAmong`, which `unknownStemAmong` now calls; `read`'s message is unchanged).
  Trap: the library's `NoNodeError` does not say which argument it is about. `move` looks the
  stem up first, so an unknown stem equal to `STEM` is `STEM`'s and any other is `--parent`'s.
  `writerAt` (new) is the "create the missing store, get its writer" pair both verbs share.
- `README.md`: the Usage section documents both verbs (the examples, a paragraph each with the
  result element, the refusals, the no-op move, all-or-nothing rename), and the status line lists
  them. Also corrected there, having gone stale: "no verb moves or renames one yet", the list of
  verbs that take the lock and create its file, and exit 75's list of verbs.
- Tests: `internal/memory/move_rename_test.go` (new) covers the library: byte-exact rewrites
  (CRLF, BOM, ID-less paragraphs), roll back at each step with injected failures, a real stem
  collision caused by a move, an unloaded folder carried along, the lock. `cmd/memory/
  move_rename_bin_test.go` and `move_rename_busy_bin_test.go` (new) cover the unit's eight
  acceptance criteria through the built binary on the fixture store F. No existing test was
  changed; none pinned the usage text or the verb list exactly.
- Docs and agent files checked: `docs/spec/agent-memory.md` III.8 already holds both verbs'
  rules and was not edited. `agent/README.md`, `agent/tools/*.json`, `agent/prompt/memory_usage.md`,
  `agent/livecontext/memory_index.json` and `agent/memory/notes/` are deliberately unchanged
  (CLI15 ships the tool files and rewrites the texts). Left stale on purpose, for the operator:
  spec III.1 still lists only `write`, `delete` and `retag` as the verbs that take the lock file,
  and `agent/README.md` still says no verb moves, renames or deletes a node (CLI11's known gap,
  closed by CLI15).

## 2026-11-20 - Memory can lose things: `delete` removes a block, a node or a subtree and reports the links it broke, and `retag` renames or removes a tag store-wide

CLI11, the first of the restructuring verbs (spec III.8). `move`, `rename`, `move-block`, `split`
and `merge` are not built (CLI12-CLI14), and no tool file, skill or prompt text ships for these
verbs yet (CLI15): `agent/` is byte-for-byte unchanged, so no agent can call them until then.

- `internal/memory/restructure.go` (new): what the restructuring calls share. `locked` takes the
  write lock and loads fresh; `replaceNotes` replaces several folder notes atomically (the
  existing `commit`, with its edit-race check), reloads, and restores every note already replaced
  if a replace fails or the reload shows an error the store did not have (`newErrors`, a multiset
  diff of error-severity problems: III.8's postcondition). A note found holding other bytes at the
  reload is `ErrChangedOnDisk` and is not rolled back over (D28). `NoNodeError` carries the loaded
  stems so the CLI can list near matches without a second, unlocked load. The check is on errors
  only, not warnings: deleting a block can legitimately reword another node's "block ID also used
  in" warning, so a warnings diff would roll back good deletes.
- `internal/memory/delete.go` (new): `Writer.Delete`. A block delete is byte surgery on the source
  lines (the block's lines plus the blank lines up to the next block, or, for the last block, back
  to the previous one), not a pass through the encoder: the encoder would also give IDs to ID-less
  operator paragraphs and collapse blank-line runs, and the rule is "changes nothing else". The
  result is still loaded through the folder-note parser (`verifyOutput`) before it is written. A
  node delete refuses, without `--recursive`, a folder holding anything but the folder note and
  dot-entries, judged from the directory listing, not from the loaded children, so a folder that
  failed to load or a stray file blocks it too. The folder is renamed to `.<stem>.deleting`
  beside itself, the store reloaded and checked, and only then removed. Trap: the spec's sentence
  order reads "renamed, then removed, then reloaded, then renamed back on error", which cannot be
  done; the reload comes before the removal. Every loaded note in the subtree is compared with
  the bytes read before the rename (the edit-race check). `Dangling` is every link in a surviving
  block that resolved before and does not resolve after, so links from inside the deleted
  subtree, links already dangling, and links a duplicated block ID's second holder still answers
  are left out without special cases. Not done: if removing the hidden folder fails after the
  check passed, the node is gone from the store and the call returns an error naming the
  leftover; it is not renamed back, because it may be half removed.
- `internal/memory/retag.go` (new): `Writer.Retag` and `CleanTag`. Every note is worked out
  before any is written, so a block that would be left empty fails the call with nothing
  changed. Block text is edited in the block's source lines (text offsets map to source offsets
  line by line), so line endings, the way the ID is written and ID-less paragraphs stay as they
  were; the frontmatter tag list goes through the existing `rewriteFrontmatter` (D26), so a
  changed list is re-emitted as any tag change is, and an unchanged one is copied. Removing an
  inline tag takes the space before it, or, when the tag starts the text or a line, the space
  after it; a tag alone on a line goes with its line break (leaving a blank line would split the
  block in two). A removal that would leave a blank line or expose a list, quote or heading
  marker is refused, naming the block. Decided where the spec is silent: matching is
  case-sensitive (search folds case, but the index counts `Misc` and `misc` as two tags);
  duplicates are collapsed only in a tag list the retag touches; `NEW` equal to `OLD` returns
  before taking the lock.
- `internal/memory/links.go`, `search.go`: `scanLinks` and `inlineTags` are now thin wrappers over
  `scanLinkSpans` and `inlineTagSpans`, which also say where each link or tag is written. That is
  how `delete` reports a link "as written" (alias included) and `retag` rewrites only the tag
  token, without a second parser. Both wrappers still return nil for no matches, which existing
  tests compare against.
- `cmd/memory/delete.go`, `retag.go` (new), `main.go`, `read.go`: the verbs, their results
  (`<memory_delete>` with `<dangling>`, `<memory_retag>`, via `encoding/xml`), the usage text and
  the package comment. Argument errors come before the store is created and name the argument as
  typed: `STEM`, `--block`, `--recursive`, `OLD`, `NEW`. `delete /` is refused naming `STEM`. A
  `<dangling>` whose holding block has no ID has no `block` attribute. `read`'s unknown-stem
  message was split so `delete` gives the same line, near matches included.
- `README.md`: the status line lists both verbs; the Usage section has examples and a paragraph
  for each, with the result XML. "No verb ... deletes a node" and "no lock file except `write`'s"
  are corrected (a refused `delete` or `retag` on a missing store leaves the empty store and its
  lock file, as a refused `write` does), and exit 75 names all three writing verbs.
- Tests: `cmd/memory/restructure_bin_test.go` and `restructure_busy_bin_test.go` run the unit's
  eight acceptance criteria through the built binary on the fixture store made with `write`;
  every successful call is also checked to add no lint error. `internal/memory/delete_test.go`
  and `retag_test.go` cover the byte-level rules (CRLF, byte-order mark, ID on its own line,
  ID-less paragraphs), the roll backs (a failed rename, a new error at the reload, a note edited
  under the call) and the lock.
- Docs and agent files checked: `docs/spec/agent-memory.md` III.8 already describes both verbs
  and was not edited. `agent/` is unchanged by design (CLI15 ships the tool files and skills), so
  `agent/README.md`, the tool descriptions and the guidance still describe only the five earlier
  verbs, and `memory_write.json`'s "Tags cannot be cleared and blocks cannot be deleted" stays
  true of `write`. Spec III.1 still says "no lock file except `write`'s"; the README has the
  current rule.
- No new dependency. The library still does not log.

## 2026-11-19 - The README no longer links to a design transcript that is not in the repo

- `README.md`: dropped "The conversation that produced it is in `docs/research/`". That folder is
  empty locally and git keeps no empty folders, so the link was dead on GitHub. The spec link
  stays; the spec is the design record. No code change.

## 2026-11-19 - CLAUDE.md requires the docs and agent/ files to change with the code

- `CLAUDE.md`: new section "Docs and agent files travel with the code". A change to what a verb
  accepts, prints, creates, or how it fails updates, in the same change, the spec (first), the
  usage text, `README.md`, `agent/README.md`, the tool and live-context files, the guidance
  (byte-equal to spec §4.5) and the starting tree (byte-equal to `write`'s output), and the
  CHANGELOG names what was checked. Why: `agent/` is copied into agents as it is and its texts are
  read by the model every turn, and tests pin only part of it, so a green gate can hide stale
  docs. It also restates III.6's rule that model-facing text never names a node. The intro no
  longer says tool files are "seeded into the agent's `tools/`" (pre-CLI5 wording): everything
  ships in `agent/` and is installed by hand.
- `.github/workflows/ci.yml`: the header comment says releases carry the archives (binary plus
  `agent/`) and `checksums.txt`, as the job already did. Comment only; no job changed.
- Audit, no change needed: `README.md` and `agent/README.md` match CLI10's behaviour;
  `agent/memory/notes/` lints clean and is byte-equal to fresh `write --parent /` output. The root
  `notes/` needs no `.md` file: a file directly in `notes/` is a stray (a `lint` warning), and the
  root has no content by design (spec III.6).

## 2026-11-19 - An empty memory says so: every verb creates a missing store, and `index` on it prints `<empty>No nodes yet.</empty>`

- `cmd/memory/store.go`: one path to the store for every verb (spec
  III.1, CLI10). `notesDir` now does what `makeNotesDir` did for `write`
  alone: after resolution and the parent check, it creates `memory/` and
  `memory/notes/` when missing (mode 0755 forced on what it creates; an
  existing directory keeps its mode; a concurrent creator's `EEXIST` is
  judged, not failed on) and nothing else. `makeNotesDir` is gone, and
  `read`, `search`, `index`, `lint` and `write` all call `notesDir` after
  their own argument checks, so an argument error, a resolution error, a
  missing parent and a non-directory `memory/` or `notes` (a dangling
  symlink included) still create nothing. No lock file: only `write`'s
  writer makes `notes/.write.lock`. Why: a live-context `index` on a fresh
  agent printed nothing, so the model was never told its memory exists.
  Trap: the install guard `[ -e <state dir>/memory ] || cp -R memory …`
  now skips the starting tree once the agent has made any call (the
  live-context index at its first input), so the tree must be copied
  before the agent first runs; both READMEs say so.
- `cmd/memory/index.go`: when no node loads (an empty store, or one
  whose every node is skipped) `index` renders `<memory_index>` with the
  trust line and `<empty>No nodes yet.</empty>` (a new `Empty` field on
  `indexXML`, `omitempty`, so stores with nodes render exactly as before)
  instead of returning nothing. No `<tags>` (tags come only from loaded
  nodes) and no budget pass (nothing to drop). Rejected: a bare
  `<memory_index/>` without the trust line (D8's line must frame anything
  shown as memory).
- `cmd/memory/main.go`, `read.go`, `lint.go`, `search.go`: the package
  comment, the usage text (`index` "on a store with no nodes, says so";
  "every verb … creates memory/ and memory/notes/ when they are missing")
  and the verbs' comments no longer say the read verbs create nothing.
- `cmd/memory/search.go`: deliberately unchanged in order. The query is
  parsed after the store is created (D31's order: arguments, then
  `notes/`, then the query), so a malformed query (`search '"'`) on a
  missing store fails with `query: unclosed quote` and leaves the empty
  `memory/notes/`, like `write`'s writer-checked refusals (III.4 known
  gap). Only `search`'s argument count is checked before the store, as
  III.2's argument rule. Not pinned by a test: the spec is silent on
  whether a query error is an "argument error" under III.1.
- `cmd/memory/empty_store_bin_test.go` (new): the CLI10 acceptance
  tests. Exact empty-index bytes on a missing store, an empty `notes/` and
  a `notes/` holding only a broken (tagged) node, and no `<empty>` on the
  starting tree; each of `read`, `read /`, `index`, `search anything`,
  `lint`, `lint --exit-zero` as a first call, with and without an existing
  `memory/`, under umask 077 and 002 (forced down as well as up): its
  exact empty-store result, exactly `memory/` and `memory/notes/` at
  0755, no lock file, and a second call
  changing nothing (snapshot); `read kettle` on a missing store; an
  existing 0700 `memory/` keeping its mode; argument errors (with and
  without `memory/`) and resolution errors, dangling symlinks included,
  creating nothing for every read verb; eight concurrent mixed first calls
  over 25 fresh parents (checked to fail at once when the `EEXIST` case is
  removed).
- `README.md`, `agent/README.md`: the sentences saying the read verbs
  create nothing, that only `write` creates the store, and that `index`
  prints nothing on an empty store are corrected; both say to install the
  starting tree before the agent's first call.
- No smoke test: this repo has no `scripts/smoke/` (no live environment);
  the binary tests run the built `memory` end to end.

## 2026-11-19 - An agent can lint its own store and see the whole report: `lint --exit-zero` and `memory_lint.json`

- `cmd/memory/lint.go`: `lint` takes one option, `--exit-zero` (spec
  III.7, CLI9), from its own flag set (`lintFlags`) instead of `noArgs`.
  With it, `lintVerb` returns the report with no error, so `run` prints it
  and exits 0 whatever the error count; the report itself is unchanged
  (byte-equal to plain `lint`'s). Argument, resolution and
  missing-parent errors still come first and still exit 1: `--exit-zero`
  drops only the verdict, never a failure to run. A stray argument now
  says `lint takes no arguments other than --exit-zero, got "…"`.
  Why the flag at all: the agent's shell tools discard stdout when a
  command exits non-zero, so a tool running plain `lint` would show the
  model `exit status 2` and never the report. Rejected: making the tool
  run something else (a wrapper script, or `lint; true`, which argv
  tools cannot express).
- `cmd/memory/args.go`: `setFlag` understands boolean flags (the first
  the CLI has): given, the flag is on and consumes nothing, so
  `lint --exit-zero extra` refuses `extra` instead of taking it as the
  flag's value. `--exit-zero=true` / `=false` is refused (`takes no
  value`), following flake8 and ruff, whose `--exit-zero` takes none,
  and III.7's "its one option is `--exit-zero`"; Go's `-flag=false`
  convention was rejected as a second spelling of plain `lint`. No other
  verb has a boolean flag, so their parsing is unchanged.
- `cmd/memory/main.go`: the usage text shows `lint [--exit-zero]` and
  what it does; the package comment says lint exits 2 unless given it.
- `agent/tools/memory_lint.json` (new): III.7's file exactly: `memory`,
  `prepend_args` `["--parent-dir", "${AGENT_STATE_DIR}", "lint",
  "--exit-zero"]`, `read_only` true, `allow_user_args` false,
  `forbidden_args` empty, and the pinned description, which names no flag
  and no exit 75 (`lint` cannot be busy). The existing install commands
  (`cp tools/memory_*.json`) already copy it.
- `agent/README.md`: the table lists `tools/memory_lint.json` (optional);
  a paragraph says what it is for (the agent checking its store after the
  operator's hand edits), why it runs `--exit-zero`, and that cron jobs
  and hooks use plain `lint`. The tool-file paragraph no longer claims
  every description mentions exit 75, and gives the lint file's
  `prepend_args`, which do not end with the verb.
- `README.md`: the Usage section shows `memory lint --exit-zero`,
  explains it beside `lint`, and points at the tool file; the sentence
  "no tool file ships for it, because it is the operator's check" is gone
  (CLI9 reverses it), and the exit-code paragraph notes the exception.
- `cmd/memory/lint_tool_test.go` (new): binary tests for each acceptance
  criterion: plain vs `--exit-zero` stdout compared byte for byte on a
  store with one error and one warning, on the clean starting tree and on
  a missing store; failures to run (`extra` on either side of the flag,
  `=value`, an unknown flag, an empty `MEMORY_PARENT_DIR`, a missing
  parent, `memory/` a file) stay
  exit 1 with nothing created; the tool file parsed with the strict
  loader and its description compared with III.7's blockquote read from
  the spec; the tool run as the agent runs it (substituted prepend,
  existing empty decoy `MEMORY_PARENT_DIR` that must stay empty), its
  report compared problem by problem with the library's; usage and README
  checks. The tests superseded by this unit (`TestToolFilesSchema`'s file
  list, `agentFiles`) were edited in the lock commit and are untouched
  here. No smoke script: this repo has none, and the behaviour needs no
  live agent (the agent-side discard of non-zero output is out of scope).

## 2026-11-19 - `memory lint` reports every problem in a store, and exits 2 when a node is missing from it

- `cmd/memory/lint.go` (new): the `lint` verb (spec III.7). No arguments
  (`noArgs`, as `index`); the store is found with `notesDir()`, the read
  verbs' lookup, so a missing store is the empty, clean report and nothing
  is created (no store, no `.write.lock`: `lint` never touches the
  writer). It prints `<memory_lint nodes errors warnings>` with one
  `<problem severity path>reason</problem>` per `Store.Problems()` entry,
  in the library's order, through `encoding/xml` (D20) so a reason holding
  `<`, `>` or `&` escapes and decodes back exactly. `nodes` is
  `len(Store.Nodes())`, so nodes in a skipped subtree are not counted. The
  library's checks and reason texts are untouched: `lint` is a renderer
  over what `memory.Load` already reports. With errors it returns its
  report together with the sentinel `errLintFound`.
- `cmd/memory/main.go`: `run` prints the verb's output and exits 2
  (`exitLintErrors`) with nothing on stderr when a verb returns
  `errLintFound`; every other error keeps its one stderr line and exit 1 or
  75 exactly as before. Rejected: changing the `verb` signature to return
  an exit code (it would touch every verb for one verb's need), and
  printing the report on stderr (III.7 puts it on stdout). Trap: the check
  for `errLintFound` must come before the generic error branch, or the
  report is lost and the verdict reads as an error line. The usage text
  and the package comment list `lint` and its exit 2.
- `README.md`: the status line lists `lint`; the Usage section has a
  `memory lint` example and a paragraph on what it reports (errors vs
  warnings, `.` for `notes/`), its three exit codes, and a cron job and a
  git pre-commit hook using `memory --parent-dir DIR lint || …`. The exit
  code paragraph notes that `lint` alone also exits 2.
- `cmd/memory/lint_bin_test.go` (new): binary tests for each acceptance
  criterion. Reports are compared problem by problem with
  `memory.Load(notes).Problems()` run in the test, so a re-sorted,
  regrouped or reworded report fails; a `notes/` made unreadable
  (mode 000, skipped as root) gives the one problem at `.`; a
  snapshot of the aged state dir proves `lint` writes nothing. Also a
  README check for the documentation above.
- Deliberately not done: no `--fix` or `--strict`, no failing on
  warnings, and no tool file or live-context entry in `agent/` (III.7: it
  is the operator's check; `TestAgentFolderIsComplete` is unchanged). The
  spec (III.7, III.2's row, III.3's exit 2, III.4's known-gap wording,
  D32) was already locked before this build and is not changed here.

## 2026-11-19 - `memory` imposes no structure: nothing is seeded, a missing store is empty, the root is `/`, and the starting tree ships as optional files

- `internal/memory/seed.go`: deleted, with `SeedDefaults` and the seed list
  (spec III.6, D19). The library names no node; no production `.go` file
  holds the string literal `"user"`, `"projects"`, `"people"` or `"topics"`.
- `internal/memory/write.go`: `RootParent` (`"/"`) is the root as a
  `WriteRequest.Parent`. `checkCreate` now returns the parent's path
  relative to `notes/` (`""` for the root) instead of the parent node, so a
  create under `/` makes `notes/<stem>/` through the same folder, commit and
  read-back path as any create. `checkUpdateParent` accepts `/` only for a
  node with no parent; for a nested node it is the usual "already exists
  under X" refusal naming `parent`. `/` is never a stem: `validStem` already
  refuses it. The no-parent and unknown-parent errors now say `/` creates at
  the top level (the existing test's "top level" wording is kept).
- `cmd/memory/store.go`: `notesDir()` (read, search, index) resolves and
  checks the parent and refuses a non-directory `memory/` or `notes` as
  before, but creates nothing: a missing `memory/` or `notes/` is returned
  as is and `memory.Load` reads it as an empty store (III.1). The new
  `makeNotesDir()` (write only) creates `memory/` and `memory/notes/` with
  `os.Mkdir`, mode 0755 forced by `chmod` (only on a directory it made, so
  an existing `memory/` keeps its mode), and nothing else. `EEXIST` from a
  concurrent writer is not an error: what is there is re-judged and must be
  a directory. Trap: dropping that tolerance loses most of eight
  simultaneous first writes (the concurrency test catches it). Rejected:
  `MkdirAll` (it would create a missing parent, which III.1 forbids), and
  creating the directories inside the library writer (the writer serves an
  existing `notes/`; making the store is the CLI's addressing job).
- `cmd/memory/write.go`: `write` calls `makeNotesDir()` after its argument
  checks, before `WriterFor` and its lock. `write /` is refused while
  parsing, `STEM: / is the root, …`, so it creates nothing even on a missing
  store; other invalid stems are still the writer's refusal after the store
  is made (III.1's known gap covers refused writes leaving an empty
  `notes/`).
- `cmd/memory/read.go`: `read /` is the root read, the same bytes as `read`
  (with `--depth` too); `read / --block ID` is refused while parsing with an
  error naming `STEM`. `rootArg` is `memory.RootParent`, shared with write.
- `cmd/memory/main.go`: the usage text says `read` with no STEM or `/` reads
  the root, `--parent /` makes a top-level node, and a missing store is
  empty with only `write` creating it; the package comment says the same.
- `agent/memory/notes/{user,projects,people,topics}/<stem>.md` (new): the
  optional starting tree, generated by running `memory write <stem>
  --parent / --description <D>` on an empty store with III.6's descriptions,
  so each file is byte-equal to what `write` makes (and to what CLI6
  seeded). Release archives carry them inside `agent/` unchanged
  (`build-binaries.sh` already copies the folder whole).
- `agent/tools/memory_write.json`: the description is III.6's text exactly.
  `memory_read.json`, `memory_index.json`, `agent/livecontext/memory_index.json`:
  III.6's replacement sentences ("top-level nodes", never "areas").
  `agent/prompt/memory_usage.md`: §4.5's revised text, byte for byte.
- `README.md`: the Usage examples are typology-free (a top-level
  `household` via `--parent /`, then a child under it); new paragraphs say a
  missing store is empty and only `write` creates it, and that the root `/`
  is the one fixed node; the wiring guide's step 3 adds
  `[ -e <state dir>/memory ] || cp -R agent/memory <state dir>/`.
- `agent/README.md`: the table's `memory/notes/` row; the install commands
  add `[ -e <state dir>/memory ] || cp -R memory <state dir>/`; a new
  section, *The starting tree*: optional, how to rearrange it, the model
  learns the structure from the index, and the model can add top-level nodes
  but never remove one. No bash block was added there, because every bash
  block in that file is run by the install tests.
- `internal/memory/root_test.go` (new): the library creates under `/` (modes
  forced under umask 077, on a populated and on an empty `notes/`, bytes
  equal to `EncodeNew`), accepts `/` on a first-level update and refuses it
  on a nested one naming `parent`, refuses `/` as a stem; and an AST scan of
  `internal/memory` and `cmd/memory` for a `SeedDefaults` declaration and
  the four stem literals.
- `cmd/memory/typology_bin_test.go` (new): the binary tests for every CLI7
  criterion — a missing store (no `memory/`, or `memory/` without `notes/`)
  reads empty and stays untouched; `read /` equals `read`; the `/` create,
  child, update and refusals (modes checked under umask 077); the shipped
  write tool creating at the root; a refused write leaving exactly
  `memory/`, `memory/notes/`, `.write.lock`, and argument errors (including
  `write /`) leaving nothing; eight concurrent first writes, five rounds;
  the shipped tree byte-equal to `write`'s output and loading clean; both
  READMEs' install commands leaving an existing `memory/` exactly as it was;
  the shipped texts against III.6 and §4.5 (read from the spec, not
  restated); every verb's output on the shipped tree equal to its output on
  a tree made by four `write --parent /` calls.
- Deliberately not done: an exemption the unit did not foresee. Criterion 5
  says no tool description contains the word `people`, but III.6 pins
  `memory_write.json`'s description verbatim, and its example quotes the
  description "How and when to cc people on outbox replies". The pinned
  text wins; the word check skips that one quoted value, and the test fails
  if the spec text changes so the exemption can go. No change to `search`,
  the index or any agent-side file.

## 2026-11-19 - The store is created on first use: any verb seeds a missing `notes/`, the parent must already exist, and `memory init` is gone

- `cmd/memory/store.go`: `location.notesDir()`, which every verb already
  called after checking its arguments, is now the one place that checks the
  parent and seeds (spec III.1, CLI6). Order: resolve the parent and its
  source (`location.parent()`, replacing `memoryDir()`, which threw the
  source away); `os.Stat` the parent (symlinks followed), failing with
  `parent dir <p> does not exist (from <source>)` or `… is not a directory
  (from <source>)`, `<p>` exactly as given (a relative one stays relative);
  then `memory/` and `memory/notes`, each judged with `Lstat` (missing →
  `memory.SeedDefaults`) and then `Stat` (anything that is not a directory,
  or a symlink that does not lead to one, → `<path> is not a directory`,
  the path as resolved from the parent). So argument errors, resolution
  errors and a bad parent all create nothing, and the read verbs write
  nothing once `notes/` exists. Seeding is silent: no notice, nothing on
  stderr. `errNoNotes` is gone. Rejected: `MkdirAll` of the parent (a
  mistyped `--parent-dir`, or an older agent's literal `${AGENT_STATE_DIR}`,
  would quietly start a store in the working directory), and making the path
  absolute in the message (the operator should see what they typed).
- `cmd/memory/initverb.go`: deleted, with `init`'s `verbs()` entry, its
  usage line and `filesUnder`. `noArgs`, still used by `index`, moved to
  `args.go`. `memory init` is now `unknown verb "init"` plus the usage; no
  hidden alias.
- `cmd/memory/main.go`: the usage text no longer mentions `init` and says the
  parent must exist and any verb creates `memory/notes/` when `notes/` is
  missing; the package comment says the same.
- `internal/memory/seed.go`: `SeedDefaults(memoryDir)`'s trigger is now
  `memoryDir/notes` (`Lstat`), not `memoryDir`: an existing `notes/` — even
  empty, even missing a first-level node — is never touched, entries already
  in `memoryDir` are left alone, and deleting `notes/` resets the store. It
  creates `memoryDir` with `os.Mkdir` (mode forced to 0755, as `memory/`
  always was) and never its parent: a missing parent is an error and
  creates nothing. `notes/` is built in `memoryDir/.notes-seed-*` (now
  inside `memoryDir`, not beside it, so nothing is ever written next to
  `memory/`), chmod 0755 (it starts as `MkdirTemp`'s 0700), and renamed into
  place; when the rename fails because another process's `notes/` now
  exists, that is success and the temporary directory is removed. Node
  folders and files keep their modes. Trap: renaming a directory onto an
  existing *empty* directory succeeds on POSIX, so an empty `notes/` created
  by hand in the instant between the `Lstat` and the rename is replaced by
  the seed; harmless, and not worth a lock.
- `cmd/memory/lazy_bin_test.go` (new): the binary tests for every CLI6
  criterion — each verb as the first call seeds silently and leaves exactly
  the seeds (plus `kettle` and `.write.lock` after a `write`); a seeding
  `read` is byte-equal to a read of a store seeded earlier; seeds are
  encoded as `EncodeNew` encodes them, with modes 0755/0644; the missing and
  non-directory parent from each source, exact lines, nothing created; the
  literal `${AGENT_STATE_DIR}`; a symlinked parent; the `notes/` trigger
  (`keep.txt` untouched, a deleted first-level node stays deleted, an empty
  `notes/` stays empty, a deleted `notes/` is reseeded); argument errors
  before seeding; `memory`/`notes` as regular files; eight concurrent first
  `read`s, ten rounds (an in-place seed fails this with `file exists`); `init`
  gone from the verbs and usage; neither README mentions `init`.
- `internal/memory/seed_lazy_test.go` (new): `SeedDefaults` on an existing
  empty `memoryDir`, beside other entries, with an existing (empty or
  partial) `notes/`, with its parent missing, with `memoryDir` a file, in a
  read-only `memoryDir` (no partial `notes/`, no temporary directory left),
  modes, and eight concurrent calls, twenty rounds.
- `README.md`: the Usage block loses `memory init`; plain `memory read` gives
  `~/memory`; a paragraph says there is no install step, what the first call
  creates, that `notes/` is the trigger, and that the parent must already
  exist; the error paragraph gives `parent dir … does not exist (from …)`
  instead of the "run memory init" error; `index` no longer calls a missing
  `notes/` an error. The wiring guide is three steps (install, PATH, copy the
  agent files), the third pointing at `agent/README.md` and saying the store
  appears on the agent's first memory call.
- `agent/README.md`: no seeding step (install is the copy commands; the store
  appears on the agent's first memory call); an older agent's error is
  `parent dir ${AGENT_STATE_DIR} does not exist (from --parent-dir)`; the
  live-context paragraph drops "Before `init` it shows an error…"; a shared
  store's parent dir must exist.
- Not done: no change to any verb's output, to Part II, or to the agent;
  no notice when a store is seeded (seeding is silent by rule). A crash
  mid-seed can leave an inert `.notes-seed-*` in `memory/` (spec III.4 known
  gap); nothing cleans it up, since a live sibling seed may own it.

## 2026-11-19 - Everything an agent needs ships in `agent/`, installed by hand and carried in every release archive; `memory init` seeds notes only

- `agent/` (new): everything an operator copies into an agent, laid out like
  the agent's state dir (spec III.4). `memory` never writes outside
  `<parent>/memory`; installing is a copy. Future agent material (skills)
  goes here the same way.
  - `agent/tools/*.json`: the four tool files, moved from the top-level
    `tools/`, content unchanged. No top-level copy is left, so there is one
    source of truth (`TestAgentFolderIsComplete` fails if `tools/` comes
    back).
  - `agent/prompt/memory_usage.md` (new): the §4.5 guidance, byte for byte
    (generated from the spec's fenced block, and checked against it by a
    test that reads the spec, so the two cannot drift). It is a `prompt/`
    fragment, so it loads every turn whether or not the tools are installed:
    this deliberately reverses D15's gating, which needed agent code. Its
    first paragraph says "Its index, when you are shown one", no longer
    "Its index is in your system prompt", which stopped being true when
    the index moved to a live-context entry or a tool.
  - `agent/livecontext/memory_index.json` (new): the live-context entry the
    README's old step 6 printed inline, as a file with `args`
    `["--parent-dir", "${AGENT_STATE_DIR}", "index"]`. Same description.
  - `agent/README.md` (new): each file, the `<state dir>` subfolder it goes
    to, and the copy commands; `memory --parent-dir <state dir> init`
    first; the `${AGENT_STATE_DIR}` substitution an agent must do (agent
    CHANGELOG 2026-11-19) and the `memory has no notes/ folder; run memory
    init` an older agent shows; editing the path, never deleting the
    `--parent-dir` pair, to share a store (with CLI4's write-deny gap for a
    store outside the state dir); and that the guidance loads every turn and
    should match the tools installed. The copy commands assume the working
    directory is `agent/` itself and create the three destinations with
    `mkdir -p`.
- `internal/memory/seed.go`: `SeedDefaults` creates `memory/notes/` and the
  four first-level nodes, and no `protocol/`; `UsageText` is gone. The
  protocol bundle (D24) has no seeded files for now. Existing stores that
  already hold `memory/protocol/usage.md` are untouched (the directory-level
  contract), and nothing reads that file any more.
- `cmd/memory/initverb.go`, `cmd/memory/main.go`: `init`'s `created` list is
  now exactly the four `notes/…` paths (it lists what was seeded, so no code
  change beyond the seed); the usage line says `init` creates
  `memory/notes/` with the first-level nodes and nothing else.
- `scripts/release/build-binaries.sh`: each archive holds
  `memory_<tag>_<os>_<arch>/memory` and the whole repo `agent/` folder at
  `memory_<tag>_<os>_<arch>/agent/`, so an install works from a download
  alone. `cp -R agent "$out/$name/agent"` keeps the layout (a `cp -R agent/.`
  would flatten it; the new test catches that). It copies the working tree,
  so run it from a clean checkout: a stray `.DS_Store` in `agent/` would be
  archived (CI checkouts are clean).
  - `COPYFILE_DISABLE=1`: found by the new archive test. On macOS, bsdtar
    added AppleDouble `._*` entries for every file with extended attributes
    (`com.apple.provenance` marks nearly everything), so locally built
    archives already held `._memory` beside the binary. CI builds on Linux,
    where GNU tar ignores the variable, so released archives were never
    affected.
- `README.md`: the wiring guide is four steps (install, PATH, init, agent
  files). Step 1 says an archive holds `memory_<tag>_<os>_<arch>/memory`
  and `memory_<tag>_<os>_<arch>/agent/`; step 3 no longer mentions
  `usage.md`; step 4 points at `agent/README.md` and gives only the copy
  commands, run from the folder holding `agent/`. The tool table, the
  substitution note and the store-sharing rules moved to `agent/README.md`
  rather than being said twice. The Usage section's `init` paragraph and the
  Status line say the same.
- `CLAUDE.md`: the CI paragraph says the release archives carry `agent/`
  beside the binary.
- `cmd/memory/agent_folder_test.go` (new): `init` on a fresh parent leaves
  exactly `memory/notes/` and its four nodes, in the parent and the working
  dir; the usage's `init` line; the guidance byte-equal to the spec block
  after "shipped verbatim as `agent/prompt/memory_usage.md`" and free of
  "system prompt"; `agent/` holding exactly its seven files and no
  top-level `tools/`; `agent/README.md` naming all six shipped files
  (four tools, the guidance, the entry), the three destinations, the `init` command, the placeholder
  and the older-agent error; the `bash` blocks of `agent/README.md` (run
  from `agent/`) and of the README's step 4 (run from the repo root),
  executed with `<state dir>` filled in and the test binary on `PATH`,
  seeding the store and installing every file byte-equal; and one archive
  (`linux_amd64`) listed in full and compared byte for byte, beside the
  untouched `TestBuildBinaries`. Mutations checked: a drifted guidance byte,
  a top-level `tools/` copy, a README destination renamed, a flattened
  archive, and the macOS `._*` entries all fail.
- Superseded tests (`seed_test.go`, `init_bin_test.go`,
  `addressing_bin_test.go`, `tools_test.go`, `readme_test.go`) were edited
  in the lock commit and are untouched here. Fixture stores that plant a
  stray `memory/protocol/usage.md` stay: the loader still ignores it.
- Deliberately not done: no install command (III.5), no change to the tool
  files' content or the verbs, no skills. The `.github/workflows/ci.yml`
  header comment still says the release publishes "the memory binaries";
  it is a comment, and the workflow itself needed no change because it
  publishes whatever `build-binaries.sh` produces.

## 2026-11-19 - `memory` is addressed, not agent-aware: the store is `<parent>/memory` from `--parent-dir`, `MEMORY_PARENT_DIR` or `HOME`, and the tool files pin an agent's store

- `cmd/memory/store.go`: the store is `<parent>/memory`, the parent being
  `--parent-dir` (before the verb), else `MEMORY_PARENT_DIR`, else `HOME`
  (spec III.1). This replaces the unreleased `MEMORY_DIR` change below,
  which never shipped.
  - The `.env` reader and every `AGENT_STATE_DIR` / `MEMORY_DIR` lookup are
    gone, and the `.agent` fallback with them. Why: an environment variable
    cannot say where it came from, so an agent's store must not depend on
    what the operator's shell exports. The tool files name the agent's
    store instead.
  - A variable set but empty is an error (`MEMORY_PARENT_DIR is set but
    empty`, `HOME is set but empty`), never a fall-through: an empty
    `MEMORY_PARENT_DIR` falling to `HOME` would put an agent's or a
    script's notes in a person's `~/memory`. With no source at all the
    error names all three. `os.LookupEnv`, not `Getenv`, is what tells
    set-but-empty from unset.
  - `parseLocation` takes `--parent-dir DIR` or `--parent-dir=DIR` before
    the verb, at most once, never empty, and stops at the first other
    argument. Only the double-dash spelling is recognised there (the verbs'
    own flags also take `-name`): `-parent-dir` is an unknown verb, which
    fails safe.
  - The verbs receive a `location` value instead of calling a global
    resolver, so the store has one source of truth per call. Argument
    errors still come before the store is looked at; an empty
    `--parent-dir` is refused before the verb, as malformed syntax, while
    the environment is read only when a verb needs the store.
  - The trap: after the verb, `--parent-dir` is deliberately not an
    option. Each verb's parser refuses it as an unknown flag (`write`'s
    `--parent` does not match it by prefix: `flag.Lookup` is exact), or
    `search` counts it as a second argument. Scanning all of argv for it
    would let a model's argument, which always comes after a tool file's
    verb, move the store.
- `cmd/memory/main.go`: `run` parses the location before looking up the
  verb; `memory --parent-dir P` with no verb prints the usage, as `memory`
  alone does. The usage line is `memory [--parent-dir DIR] <verb>`, and the
  usage text and package comment state the three sources and the
  empty-value rule.
- `cmd/memory/{init,read,write,search,index}`: each verb takes the
  `location`; behaviour otherwise unchanged.
- `tools/*.json`: `prepend_args` are `["--parent-dir", "${AGENT_STATE_DIR}",
  "<verb>"]`, which the agent replaces with its absolute state dir (agent
  CHANGELOG 2026-11-19). An exported `MEMORY_PARENT_DIR` can then never
  redirect an agent's memory. Descriptions unchanged: they never mention
  `--parent-dir`, and `TestToolFilesSchema` fails if one does.
- `cmd/memory/addressing_bin_test.go` (new): the flag beating both
  variables in both spellings and relative to the working dir; the
  environment order; every empty or missing source failing with one line
  and creating nothing (checked for `init`, `read` and `write`); a missing
  value and a repeated flag (with both stores seeded, so a last-wins
  implementation would succeed); `--parent-dir` after the verb never
  changing the store, including after a tool file's pinned one; no `.env`,
  `AGENT_STATE_DIR` or `MEMORY_DIR` consulted, and none of them named in
  the usage or in any production file; the README's store paragraph.
  Mutations checked: an empty value falling through, `HOME` before
  `MEMORY_PARENT_DIR`, and the last `--parent-dir` winning each fail them.
  The superseded tests (`runBin`, `TestToolFilesSchema`, `toolCaller`,
  `TestReadmeWiringGuide`; `TestStoreResolution` and `TestDotenvValue`
  deleted) were edited in the lock commit, not here.
- `README.md`: the usage section's store paragraph states the order and the
  empty-value rule. The wiring guide's `init` step runs `memory
  --parent-dir <state dir> init`; the tool-file step explains the pinned
  `prepend_args` and how to point an agent at a shared store (edit the
  path; never delete the pair, which falls back to `$HOME/memory`); the
  live-context entry's `args` are `["--parent-dir", "${AGENT_STATE_DIR}",
  "index"]`; and a note says the files need an agent that substitutes
  `${AGENT_STATE_DIR}`, and how an older one fails.
- `CLAUDE.md`: the CI section names the released binaries `memory`, not
  `agent-memory` (a line the rename missed).
- Not done, as out of scope: `~` expansion, a `--store` flag, any
  agent-side change, `internal/memory`, and the other III.4 known gaps.
- `CHANGELOG.md`: the `MEMORY_DIR` entry of the same day is removed. That change was committed but
  never released, and this one replaces it; leaving both would put a contradiction into the first
  release's notes.
- `cmd/memory/initverb.go`, `cmd/memory/store.go`: three `//nolint:gosec` (G703, path from argv).
  The store path now comes from the command line by design (III.1), so the taint finding is the
  feature.

## 2026-11-18 - The CLI's command is `memory`

- The binary is renamed from `agent-memory` to `memory`. It is usable
  without an agent, as a notes tool over an Obsidian-compatible vault, so a
  plain name fits. The repo and Go module stay `agent-memory`.
  - `cmd/agent-memory/` moves to `cmd/memory/`.
  - The four `tools/*.json` files run `"command": "memory"`.
  - `scripts/release/build-binaries.sh` publishes
    `memory_<tag>_<os>_<arch>.tar.gz`.
  - Two error messages and the usage text now name `memory`.
  - README, spec Part III and D32, `CLAUDE.md`, `.gitignore` and the `ci.yml`
    comment follow.
- The model never sees the binary name: it calls the tools by their tool
  names, and none of the descriptions mention it. Only the "run memory
  init" and "quote it: memory search …" errors reach it, and both read
  naturally.
- Considered and kept: `agent-memory`, for being unambiguous on PATH.
  Nothing called `memory` was found on the operator's machine.
- A mechanical rename, done by the delivery lead with no behaviour change.
  The full gate passed, and an end-to-end run with the real agent and the
  renamed tool files loaded all four tools and answered `memory_read` from
  the store.

## 2026-11-18 - `agent-memory index` prints the index, the repo ships the four shell-tool files and a wiring guide, and every merge to `main` publishes binaries

- `cmd/agent-memory/index.go` (new): `index` prints D30's `<memory_index>` with `encoding/xml` `MarshalIndent`: the hardcoded trust line as leading character data, the first-level nodes sorted by stem, each with its children (store order) as `read` renders child entries (`tags`, the true `children` count, `modified` in the local zone), then `<omitted>` when the budget dropped entries, then `<tags>`: the top 50 by node count, ties by name (read's root list stays uncapped). No path appears. A `notes/` holding no loadable node prints nothing and exits 0 (no first-level node means no node). A missing `notes/` is Part III.3's error, exit 1, as for `read`: D30's "section without an index" has no CLI equivalent, and an operator wiring a live-context entry to an uninitialised store should hear about it. Arguments are refused before the store is looked at.
- The budget: the element, `<memory_index>` to `</memory_index>`, holds at most 200 lines and 25,000 bytes. Entries are dropped second-level first, oldest first (folder-note mtime at full precision, ties by stem), then first-level the same way. The notice is Part III.2's, `N nodes not shown: read a node to list all its children.` (`node` for one), and counts towards the budget. The fewest drops are found by binary search, not one render per drop: zero drops is tried alone (one drop adds the notice, which can outweigh a short entry), and from one drop on, each further drop removes a whole line and at least ~50 bytes while the notice grows by at most two, so the size only falls. The trap: a linear loop is quadratic, and a store big enough to need truncation is exactly the one where that hurts, on every input if the index is a live-context entry. If even dropping every entry does not fit (50 very long tags), every entry is dropped and the result is returned over budget, since the trust line, notice and `<tags>` are never dropped.
- `cmd/agent-memory/read.go`: `tagCounts`, `rankedTags` and `childEntry` are extracted so `index` shares `read`'s child entries and tag ordering, and `readFlags` so a test can check the tool file against read's flags. Behaviour unchanged.
- `cmd/agent-memory/write.go`: `writeFlags` extracted for the same test. Behaviour unchanged.
- `cmd/agent-memory/initverb.go`: `noArgs` extracted, shared by `init` and `index`; `init`'s message is unchanged.
- `cmd/agent-memory/search.go`: the `node`/`nodes` noun is the helper `nodeNoun`, shared with `index` (lint: goconst). Behaviour unchanged.
- `cmd/agent-memory/main.go`: `index` is dispatched and listed in the usage text.
- `tools/memory_read.json`, `memory_write.json`, `memory_search.json`, `memory_index.json` (new): argv shell tools in the agent's schema, every file carrying all six keys (`command: agent-memory`, `prepend_args: [verb]`, `forbidden_args: []`, `read_only` true except for write, `allow_user_args` false only for index). The descriptions are D29's and D31's rewritten for flags: the stem as the first argument, each flag and its value as separate arguments, a JSON-array example, and a closing sentence that exit 75 ("busy" or "changed on disk") means run the same command again. That sentence is in all four, as the unit asks, though only `write` can exit 75 today: `read`, `search` and `index` take no lock. `memory_index.json` describes the index and says it takes no arguments. `forbidden_args` is empty because nothing a verb accepts should be blocked; its semantics are the agent's and are not visible in this repo.
- `README.md`: a "Wiring it to an agent" guide in six steps, in order: install (release archive and checksum, or `go build`), the agent's `PATH`, `init` in the agent's working directory, `cp tools/*.json <state dir>/tools/` (with which file grants what), placing `usage.md` as a `prompt/` fragment, and a live-context entry for `index` as JSON. "Usage so far" is now "Usage" and covers `index`; the status line is updated.
- `.github/workflows/ci.yml`: a `build` job cross-compiles every target with `scripts/release/build-binaries.sh` on every PR and push, so a broken target fails before release. A `release` job runs only on a push to `main` (`if: github.event_name == 'push' && github.ref == 'refs/heads/main'`), needs `lint`, `test`, `tidy`, `vuln` and `build`, and alone holds `contents: write`. It checks out full history, picks the tag with `next-tag.sh` from `git tag --list` and today's UTC date, diffs `CHANGELOG.md` against the newest release tag behind `HEAD` (`git describe --match` on the `YYYYMMDD` pattern; the empty tree before the first release, so the first notes are the whole file) through `notes.sh`, builds with the tag, and runs `gh release create TAG dist/*.tar.gz dist/checksums.txt --target $GITHUB_SHA`, which creates the tag with the release. Step outputs reach scripts through `env`, not `${{ }}` inside `run`. No Docker image (Part III.5). Not exercised in this build: the room has no git and no GitHub, so the release steps are checked only structurally; the first merge is their real test. Re-running a release job for an already-released commit would publish a second release (`.2`) with empty notes; not guarded.
- `cmd/agent-memory/index_bin_test.go` (new): the layout is a byte-for-byte golden in `Pacific/Auckland`, where the fixture's UTC dates are a day behind, with a third-level node that must not appear, a description needing escapes, and an extra property and inline field whose removal changes nothing. The tag cap uses 60 tags with tied counts. The budget tests compare the binary's output with a model built by plain string formatting (no `encoding/xml`), which drops in D30's order and finds the fewest drops by trying each count: over by lines only (250 children with tied mtimes; 59 dropped, by formula), over by bytes only, every child then first-level nodes with tied mtimes, a singular notice, and edges at exactly 200 lines and exactly 25,000 bytes, each untruncated, with one line or byte more truncated.
- `cmd/agent-memory/tools_test.go` (new): each file is parsed strictly (exact key spelling, since `encoding/json` matches keys case-insensitively; nothing after the object). The flags a description names (every `--word`) must equal the flag set built by `readFlags`/`writeFlags`, in both directions; `search` and `index` have none. Each file is then run as the agent would (its `prepend_args` plus model arguments) against an `init`-seeded store: write creates a node, read returns it, search finds it, index lists it under `topics`.
- `cmd/agent-memory/readme_test.go` (new): the wiring section's `###` steps must be the six above in order. The first `json` block in the live-context step must be one JSON object with only `command`, `description` and `args`, running `agent-memory index`.
- `cmd/agent-memory/release_ci_test.go` (new): `ci.yml` parsed with `yaml.v3` (already a dependency): the jobs exist, the build job runs `build-binaries.sh`, and the release job's condition, `needs`, permissions and use of the three release scripts and `gh release create` are as above. No `docker` or `ghcr.io` anywhere. `build-binaries.sh` is run into a temp dir: exactly four archives and `checksums.txt`, each checksum matching, each archive holding `agent-memory_<tag>_<os>_<arch>/agent-memory` whose ELF machine or Mach-O CPU matches its name.
- `internal/memory`: unchanged. No dependency added.
- Not done, by scope: restructuring verbs, dreaming, an install command and Docker (Part III.5). No smoke test: this repo has no smoke suite, and the binary tests run the real binary against real directories. `CLAUDE.md`'s CI section, which still says there is no release job, is left for the delivery lead, as the unit asks.

- `docs/delivery/done/cli/CLI3-index-tools-release.md` (moved from
  `doing/`): independent verification passed all five criteria.
  - It caught 17 of 17 `index` mutants and 29 of 29 tool-file, README, CI
    and script mutants.
  - An independent oracle matched the budget logic on 120 random stores.
  - The delivery lead also ran an end-to-end check against the real agent
    binary and a stub backend. The agent loaded all four shipped tool
    files; the live-context `index` entry reached the model; a model
    `memory_read` call returned the node from the store.
- `docs/spec/agent-memory.md`: Part III.4 lists the four tool files. Four
  CLI3 behaviours are recorded as known: `index` before `init` errors on
  every input; the budget can be unsatisfiable with huge tags; all four
  descriptions mention exit 75; a re-run release job publishes a `.2`.
- `CLAUDE.md`: the CI section describes the `build` and `release` jobs, and
  the spec note points at Part III.
## 2026-11-18 - `agent-memory write` saves to the store and `agent-memory search` finds things in it, and exit 75 means retry

- `cmd/agent-memory/write.go` (new): `write STEM [--parent P] [--description D] [--tag T]… [--property K=V]… [--add TEXT]… [--replace ID=TEXT]…` maps onto one `memory.WriterFor(notes).Write(WriteRequest)` (D29, Part III.2). `--add` and `--replace` are recorded as CLI1's walker meets them, so the blocks keep argv order: `--add A --replace X=… --add B` assigns a new ID, `X`, then a new ID. Any `--tag` replaces the tag list; with none, the tags are untouched. `--property` values are strings and are merged; a repeated key keeps the last value, as a repeated `--description` does. `--replace` with no `=` or an empty ID is refused by the walker: an empty ID would make the library append instead of replace. The result is `<memory_write created stem>` with one `<assigned>` per block, `<adopted>`, and the node from CLI1's `nodeOf` at depth 0 against the write's own read-back store, so backlinks are as of the write. Writer field errors (`*memory.FieldError`, matched by type, not by message text) are renamed to the arguments as typed: `stem` → `STEM`, `parent`/`description` → the flag, `tags[i]` → `--tag #i+1`, `properties["k"]` → `--property "k"`, `blocks[i]` → `--add #i+1` (numbered from 1 across every `--add` and `--replace`) or, for a replacement, `--replace <ID>` (for its text as well as its ID, as Part III.2's example shows). The reason text after the name is the writer's own, so a few still use the JSON tool's wording ("omit the id", "give it as the description field"). Order of checks: arguments, then `notes/` (Part III.3's message; nothing created, no lock file), then the writer.
- `cmd/agent-memory/search.go` (new): `search QUERY` runs `memory.ParseQuery` and `memory.Search` and keeps the library's ranking. The CLI applies D31's caps: 20 nodes and 5 hit blocks each, `hits` keeping the true count. Then the 30,000-byte budget on the rendered `<memory_search>` element (the final newline excluded): drop the lowest-ranked node one at a time, and with one node left drop its blocks from the last, never below its description. `<omitted nodes="N">` appears whenever `matched > shown`, `node` for one. Search takes no flags. The query is one argument, verbatim and untrimmed in the `query` attribute, so `search '-draft kettle'` is a query, not an unknown flag. A leading `--` is dropped as the end of flags, for tool files that want it. More than one argument is refused with `QUERY: …; quote it`, never joined, because joining would lose the user's quoting. Order of checks as in D31: arguments, then `notes/`, then the query, whose errors are the library's exact D31 text (`query: empty`, …), a missing argument included.
- `cmd/agent-memory/main.go`: `memory.ErrBusy` and `memory.ErrChangedOnDisk`, matched with `errors.Is`, exit 75 (`EX_TEMPFAIL`) with their one stderr line. Every other error stays exit 1. The usage text lists `write` and `search`.
- `cmd/agent-memory/args.go`: the `--` terminator is a named constant shared with `search` (lint: goconst). Behaviour unchanged.
- `cmd/agent-memory/write_bin_test.go`, `search_bin_test.go` (new): every criterion runs the built binary on an `init`-seeded store in a fresh temp directory. Results are parsed with test-local XML types. Expected search ranking comes from the library itself (`memory.Search`), so the test fails if the CLI re-ranks. Fixtures are shaped so ranking needs hits, mtime and stem. The budget tests check that the element fits, that the top-ranked nodes or the first blocks are kept, and that no more was dropped than needed (equal-sized nodes and blocks, so one more would not fit). Error cases check the exact argument prefix, that no writer field name appears, and that the whole state dir is byte-, mtime- and mode-identical afterwards.
- `cmd/agent-memory/retry_bin_test.go` (new, `unix` only): busy holds a real `flock` on `notes/.write.lock` from the test process and waits out the writer's 10 s. The library's `lockWait` seam is unexported, and an `internal/memory` export just for tests was out of scope ("no change to `internal/memory`"). The test runs in parallel, so it overlaps the other parallel tests only. Changed-on-disk has no deterministic trigger from outside the process. A goroutine replaces the folder note atomically and continuously (an editor that does not take the lock) while `write` runs up to 30 times. The test requires at least one exit 75 with "changed on disk" and fails on any exit 1. It saw a refusal on the first few attempts in 10 of 10 runs.
- `README.md`: "Usage so far" covers `write`, `search` and exit 75, and the status line is updated.
- `internal/memory`: unchanged.
- Not done, by scope: `index`, tool files and releases (CLI3), and clearing tags or deleting blocks (restructuring). No smoke test: this repo has no smoke suite, and the binary tests run the real binary against real directories. `govulncheck` was not run in this build: the environment refused `go run golang.org/x/vuln/cmd/govulncheck@latest`. No dependency was added.

- `docs/delivery/done/cli/CLI2-write-search.md` (moved from `doing/`):
  independent verification passed all seven criteria, and caught 21 of 22
  mutants. Production grew by about 326 lines. My criterion 1 wrongly
  asked `read` to show a property (D29 renders none); the builder checked
  it through the library. The spec records the JSON-tool wording left in
  writer reasons, and `search`'s verbatim query.
- `docs/delivery/doing/CLI3-index-tools-release.md` (new; skeleton removed): five criteria covering `index`, the four shipped tool files (including `memory_index.json`), the README wiring guide and binary releases.
## 2026-11-18 - An `agent-memory` binary finds the agent's store, seeds it with `init`, and reads it with `read`

- `cmd/agent-memory/main.go` (new): verb dispatch (`init`, `read`). No verb or an unknown verb prints usage to stderr and exits 1. A verb's result reaches stdout only on success; an error is one line on stderr (line breaks inside it become spaces) with exit 1 (Part III.3). Exit 75 is not defined yet: only `write` can be busy, and it arrives in CLI2. The binary embeds `time/tzdata`, so a `TZ` name resolves even in a container with no zone files; without it Go silently falls back to UTC and dates would be a day off for half the world.
- `cmd/agent-memory/store.go` (new): the store is `<state dir>/memory`. The state dir is `AGENT_STATE_DIR` from the environment if non-empty, else from `./.env` if non-empty, else `.agent`; a relative one stays relative, so it is taken against the working directory (Part III.1). The `.env` reader follows Part III.1's pinned grammar (the agent's `loadDotEnv` / `parseDotEnvValue`): lines trimmed, blank and `#` lines skipped, only `export ` with a space stripped, split at the first `=`, the value left-trimmed, a terminated quote giving the text between, otherwise (an unterminated quote included) cut at the first ` #` or `\t#` and trimmed, so a value starting with `#` is kept. The first occurrence of the key wins, even when it is empty (it then falls through to `.agent`). An unreadable `.env` (other than missing) is an error, not a fall-through to `.agent`: guessing could land on a store the agent does not use.
- `cmd/agent-memory/args.go` (new): flags and positionals in any order (`read STEM --depth 2` and `read --depth 2 STEM`). Go's `FlagSet.Parse` stops at the first positional, so the arguments are walked by hand and the per-verb `flag.FlagSet` is used only for definitions and `Set`. `--` ends flags; a non-boolean flag takes the next argument whatever it looks like (as `flag` does, so `--depth -1` is a value, then refused). Errors name the flag as typed (`--depth`, `-depth`), never `flag`'s rewritten `-name`. Rejected: calling `Parse` repeatedly on the remainder, which cannot tell a `--` terminator from a flag value `--` and still reports names in `flag`'s form.
- `cmd/agent-memory/initverb.go` (new): `init` calls `memory.SeedDefaults` only after its own `Lstat` finds no `memory/`, because `SeedDefaults` returns nil either way and the result must say whether anything was seeded. Once `memory/` exists, even empty, nothing is touched: a deleted `usage.md` or first-level node stays deleted (D19). The result is XML like every other success (Part III.3): `<init store="<abs path>" seeded="true">` with one `<created>` per file written, or `seeded="false"` and the text `memory/ already exists; nothing was needed`.
- `cmd/agent-memory/read.go` (new): D29's `memory_read` result built with `encoding/xml` from `memory.Load`'s store, never re-parsing files. A node gets `stem`, `path`, `parent` (omitted for a first-level node), `tags` (omitted when empty) and `modified` (`YYYY-MM-DD` in the local zone), `<description>`, its `<block id>`s, `<children>` entries to `--depth` (default 1; grandchildren nest inside their parent's entry, as in D30's index), `<links>` (every edge occurrence, `dangling="true"` on a missing target) and `<backlinks>`. Empty `<children>`, `<links>` and `<backlinks>` are omitted. `--block ID` gives the node's attributes, description and that one block with the block's own links and backlinks, and no children. No `STEM` gives `<memory>` with the first-level nodes (depth applies to them too) and `<tags>` listing every tag with its node count, most used first, ties by name, with no top-50 cap (D29 ruling). Argument errors come before the store is touched; a missing or non-directory `notes/` gives `memory has no notes/ folder; run agent-memory init` and creates nothing (no `notes/`, no lock file: `read` never takes the writer's lock). Load problems are not reported: Part III has no channel for them besides the result, and none was specified.
- `cmd/agent-memory/near.go` (new): an unknown stem's error lists up to five near matches, closest first: stems containing it or contained in it, or within an edit distance of half the longer stem. The spec says "near matches" without a metric; this is the build's choice.
- `cmd/agent-memory/*_test.go` (new): every acceptance case runs the built binary (built once in `TestMain`) in a fresh temp directory with `AGENT_STATE_DIR` and `TZ` stripped from the inherited environment, and parses results with test-local XML types so a wrong production tag fails. Unit tests cover the `.env` reader, argument walking, near matches and tag ordering.
- `internal/memory`: unchanged, and no helper added.
- `README.md`: a "Usage so far" section for `init` and `read`, and the status line.
- Not done, by scope: `write`, `search`, `index`, tool files and a release job (CLI2, CLI3). No smoke test: this repo has no smoke suite, and the binary tests already run the real binary against real directories.

- `docs/spec/agent-memory.md` (at CLI1): the `.env` grammar is pinned to
  the agent's exactly, first occurrence winning, so the CLI and the agent
  cannot resolve different stores. Where D29–D31 say `AGENT_TIMEZONE`, read
  the local zone. Three known gaps are recorded: a `memory/` without
  `notes/` loops between `read` and `init`; concurrent `init`s can each
  report seeding; skipped nodes are silent.
- `docs/delivery/done/cli/CLI1-skeleton-init-read.md` (moved from
  `doing/`): independent verification passed all six criteria, caught 43
  of 46 mutants, and all 10 inverted assertions failed. Production is 660
  lines.
- `docs/delivery/doing/CLI2-write-search.md` (new; skeleton removed):
  seven criteria, with a target of about 250 production lines.
## 2026-11-18 - The spec says how memory works as the agent-memory CLI, and its first milestone is planned

- `docs/spec/agent-memory.md`: Part III and D32 (the operator's rulings).
  Memory is the `agent-memory` binary, with the verbs `init`, `read`,
  `write`, `search` and `index`.
  - The store is `<agent state dir>/memory`, found exactly as the agent
    finds its state dir (environment, then `./.env`, then `.agent`). There
    is no `--store` flag.
  - `write` takes flags, not JSON.
  - `index` prints only `<memory_index>`, and the operator places it.
  - Tool files ship in `tools/`.
  - Results are XML on stdout, errors go to stderr, and the exit codes are
    0, 1 and 75 (retry). There is no logging and no capability line.
  - A banner tells readers that Part III overrides Part II's
    agent-specific parts.
- `docs/delivery/doing/CLI1-skeleton-init-read.md` (new) and
  `docs/delivery/todo/cli/` (new): the `cli` milestone in three units:
  binary, store, `init` and `read`; then `write` and `search`; then
  `index`, tool files and releases.

## 2026-11-18 - agent-memory is its own repo, holding the memory library and its design, with nothing from the agent

- Ported from `github.com/riazarbi/agent`, branch `memory`, commit `476f7da`. The whole tree was copied, then everything that is not memory was deleted as whole files and directories. No shared file was cut apart, so every file here is either unchanged or rewritten for this repo. The agent's full history, including the memory work's, stays on that branch.
- `internal/memory/` (unchanged): read model, block IDs, frontmatter, encoder, locked writer, seeding, search. It imported nothing from the agent before the move; its only third-party dependency is `gopkg.in/yaml.v3`.
- `go.mod`, `go.sum`: the module is `github.com/riazarbi/agent-memory` (was `agent`). `go mod tidy` drops every dependency except `yaml.v3`. Nothing imported the old module path, so no imports changed.
- Not carried over: the agent-side adapters (`internal/tools/memory*.go`, `internal/agent/memory_section*.go`, the `memory.*` render cases in `logx`) and the memory smokes (`scripts/smoke/memory*.sh`). They import agent packages and assert on agent events, so they would not compile or run here. The CLI is to be built from spec, not ported from them. They remain readable on the agent's `memory` branch.
- `docs/spec/agent-memory.md` (unchanged): it still describes memory as agent built-ins. The storage, encoding, writing and search sections hold. The integration sections (tool gate, system-prompt injection, agent events, state-dir seeding) are to be revised for the CLI first. `internal/memory/seed_test.go` reads this file, so it must stay at this path.
- `docs/delivery/`: the fixed builder and verifier prompts, and the banked records of units U3–U8 and U10. U1 (state-dir write deny), U2 (self-mutating overlap) and U9 (built-in descriptions) were changes to the agent core, so they stay with the agent.
- `docs/research/Implementing Agent Memory System From Research.md` (moved from the repo root): the design conversation, kept as the reasoning behind the spec.
- Deleted: `cmd/agent`, every `internal/` package except `memory`, `docker/`, `.dockerignore`, `.claude/`, `scripts/smoke/`, `scripts/validate-tests.sh`, and the agent's other specs (built-in tool markers, load-skill, seeded tool arguments, state-dir write deny, timers and reminders, web fetch, web search).
- `.github/workflows/ci.yml`: `lint`, `test`, `tidy` and `vuln` only. The `build` and `release` jobs are removed until there is a binary to build. Lint covers the whole tree, because `internal/memory` reports 0 issues under the agent's `.golangci.yml`. That drops the agent's new-issues-only diff and its full-history checkout.
- `scripts/release/build-binaries.sh`: builds `agent-memory` from `./cmd/agent-memory`, ready for the CLI. `next-tag.sh`, `notes.sh` and `release_test.sh` are unchanged.
- `CLAUDE.md`, `README.md`, `.gitignore`: rewritten for this repo. `CLAUDE.md` records where the code came from, that the spec's integration sections are out of date, and that the library does not log. The QA gate has no smoke suite until the CLI exists.
- QA gate on go1.27.4: `go build ./...`, `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` and `scripts/release/release_test.sh` pass. `golangci-lint run ./...` reports 0 issues. govulncheck finds no vulnerabilities.
