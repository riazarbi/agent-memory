# CLI7 — No built-in typology: the root is the one fixed node, the starting tree ships as files

**Milestone:** cli  |  **Builds on:** CLI6

## Goal
`memory` imposes no structure. The binary seeds nothing and names no node; a missing store is an
empty one. The root (`notes/`, written `/`) is the one fixed node, and `write --parent /` creates a
first-level node, so an empty store can be written to. The default four-node tree ships as optional
files in `agent/memory/notes/`. Nothing the model reads (tool descriptions, the live-context
description, the guidance) presumes a typology: it is sent to the index for the structure.

## Scope
- `docs/spec/agent-memory.md` **Part III.6** (new, in this unit's lock commit), III.1 *A missing
  store is an empty store*, III.2's `read` and `write` rows, III.4's `agent/memory/` row and known
  gaps, §4.5's guidance text, D19, D14, D32, and the Part II passages revised in place (D28's create
  rule, §4.2 *Seeding and the ontology*).
- `internal/memory`: remove `SeedDefaults` and the seed list (`seed.go`). The writer accepts the
  root parent `/` on create (a first-level node, folder `notes/<stem>/`) and on update of a
  first-level node.
- `cmd/memory`: no seeding. `read`, `search`, `index` on a missing `memory/` or `notes/` behave as
  on an empty `notes/` and create nothing. `write` creates `memory/` and `memory/notes/` (0755,
  forced) when missing, after its argument checks and before its lock. `read /` is the root read;
  `read / --block …` and `write / …` are errors naming `STEM`. The usage text says `--parent /`
  makes a top-level node.
- `agent/memory/notes/{user,projects,people,topics}/<stem>.md` (new): the starting tree, each file
  byte-equal to what `write <stem> --parent / --description <D>` creates (III.6 gives the
  descriptions).
- `agent/tools/memory_write.json`, `memory_read.json`, `memory_index.json`,
  `agent/livecontext/memory_index.json`: descriptions as pinned in III.6.
  `agent/prompt/memory_usage.md`: §4.5's revised text, byte for byte.
- `README.md`: Usage examples typology-free (create a first-level node with `--parent /`, then a
  child under it; no `topics`); the store paragraphs say a missing store is empty and only `write`
  creates it; the wiring guide's step 3 copy commands also install the starting tree, only when no
  store exists (`[ -e <state dir>/memory ] || cp -R agent/memory <state dir>/`).
- `agent/README.md`: the table gets the `memory/notes/` row; the install commands add
  `[ -e <state dir>/memory ] || cp -R memory <state dir>/`; a section on the starting tree: optional,
  how to rearrange it (a node is a folder holding `<folder>/<folder>.md` with a `description`), that
  the model learns the structure from the index, and that the model can add top-level nodes but not
  remove them.
- Tests and the CHANGELOG entry.

## Not in scope
- Verbs that move, rename, merge, split or delete nodes.
- Any agent-side change; any change to `search`, the index's layout or budget.
- Part II text beyond what the lock commit revised.

## Governing spec
- `docs/spec/agent-memory.md`: III.6 (whole), III.1, III.2, III.4, §4.5 (guidance), §4.2 (D28 create
  rule, frontmatter rules), D19.

## Existing tests this unit supersedes
Edited in the lock commit; builders do not touch them:
- `internal/memory/seed_test.go` and `seed_lazy_test.go` deleted (no seeding).
- `seeded()` (`read_bin_test.go`) copies the shipped `agent/memory/` into `.agent/memory/` (new
  helper `copyTree`).
- `agent_folder_test.go`: `agentFiles` lists the four `memory/notes/…` files, so the README install
  tests expect the starting tree installed.
- `lazy_bin_test.go`: the seeding tests deleted (`TestFirstCallOfAnyVerbSeeds`,
  `TestSeedingReadMatchesSeededRead`, `TestSeedsAreWriteEncoded`,
  `TestSeedsBesideExistingMemoryEntries`, `TestDeletedNotesIsSeededAgain`,
  `TestConcurrentFirstCalls`); `TestSymlinkedParent` creates its store with a write.
- `addressing_bin_test.go`: stores are created by `firstWrite` (`write kettle --parent / --description
  d`); `isStore` and `readsRoot` expect that one node.

At the lock commit many binary tests fail (they need the shipped starting tree or `--parent /`); the
lead checked the edited tests against a throwaway prototype, where all pass once the READMEs carry
the copy line.

## Acceptance criteria
`W` is a fresh temp dir with an empty `W/.agent` (runBin's default parent).

1. **A missing store is empty, and reading it creates nothing.** On `W`: `read` → exit 0, the root
   element with no nodes and no tags; `read /` → byte-equal to `read`; `index` → exit 0, empty
   stdout; `search anything` → exit 0, `matched="0"`; `read kettle` → exit 1, the unknown-stem
   error. Afterwards `W/.agent` is still empty. The same holds with `W/.agent/memory/` existing but
   no `notes/`. `internal/memory` has no `SeedDefaults`, and no production `.go` file holds the
   string literal `"user"`, `"projects"`, `"people"` or `"topics"`.
   *Excludes:* keeping any seeding; a read verb creating `memory/` or `notes/`.
2. **The root is `/`.** On `W`: `write kettle --parent / --description d` → exit 0,
   `created="true"`; `W/.agent/memory/notes/kettle/kettle.md` exists, `memory/` and `notes/` are
   0755; `read` lists `kettle`. Then `write descale --parent kettle --description e --add a`
   creates a child; `write kettle --parent / --add b` is an update (`created="false"`);
   `write descale --parent / --add c` → exit 1 naming `--parent`; `write / --description x` →
   exit 1 naming `STEM`; `read / --block m-aaaaaa` → exit 1 naming `STEM`. The shipped
   `memory_write.json` called through the tool-file harness with
   `["kettle", "--parent", "/", "--description", "d"]` on an empty store creates `kettle`. Library:
   a `WriteRequest` with `Parent: "/"` creates a first-level node.
   *Excludes:* requiring an existing first-level node; treating `/` as a stem; letting `write /`
   write anything.
3. **Writing to a missing store.** On `W`, `write kettle --parent nope --description d` → exit 1
   naming `--parent`, and `W/.agent` then holds exactly `memory/`, `memory/notes/` and
   `memory/notes/.write.lock`. Eight `write k<i> --parent / --description d` started together on a
   fresh `W` all exit 0, leaving eight first-level nodes.
   *Excludes:* `write` failing when racing another to create `notes/`; creating the store before
   argument checks (`write` with no stem still leaves `W/.agent` empty).
4. **The starting tree is shipped, optional, and never overwrites.** `agent/memory/notes/` holds
   exactly the four files, each byte-equal to the file `memory write <stem> --parent /
   --description <D>` creates on an empty store, with III.6's descriptions; the four load with no
   problems. Running `agent/README.md`'s install commands on a fresh state dir installs them; on a
   state dir whose `memory/notes/` already holds `own/own.md`, the commands leave `memory/` exactly
   as it was. A release archive holds `memory_<tag>_<os>_<arch>/agent/memory/notes/user/user.md`.
   *Excludes:* copying the tree into an existing store; a tree that differs from what `write` makes.
5. **No shipped text presumes a typology.** `memory_write.json`'s description equals III.6's text
   exactly; `memory_read.json`, `memory_index.json` and `livecontext/memory_index.json` carry III.6's
   replacement sentences. None of the four descriptions contains `area` (any case) or the words
   `user`, `projects`, `people` or `topics`. `agent/prompt/memory_usage.md` equals §4.5's text and
   contains neither `area` nor the words `user`, `projects` or `topics`. `README.md`'s Usage section
   contains `--parent /` and not `topics`; neither README contains `--parent topics`.
   *Excludes:* leaving "areas" in a read or index description; an example that files under a seed
   stem.
6. **Existing behaviour holds.** The parent-must-exist errors, non-directory errors and argument
   ordering of III.1 are unchanged (the existing CLI6 tests pass), and every verb's output on the
   shipped starting tree is what it was on the CLI6-seeded tree.
   *Excludes:* regressions in resolution or rendering.

## Environment
Local only: Go toolchain, temp dirs. No network, no live agent.

## Notes for the builder
- `memory.Load` already treats a missing `notes/` as an empty store.
- The writer's create resolves its parent with `st.Node(req.Parent)` and joins `pn.Path`; the root's
  path is `""`. `checkUpdateParent` must accept `/` only for a node with no parent.
- `write`'s argument checks already run before the store lookup; create the directories after them.
- `seeded()` in the tests copies `agent/memory/`, so the shipped files must exist for most binary
  tests to pass.
- Keep the tree lint-clean (`golangci-lint run ./...`, full tree).
