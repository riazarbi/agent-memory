# agent-memory

The command is `memory`.

Long-term memory for an AI agent: a tree of markdown notes with YAML frontmatter, which a model can read, write and search, and which a human can browse as an Obsidian vault.

**Status:** the library (`internal/memory`) and the `memory` CLI are built and tested. The CLI's verbs are `read`, `write`, `search`, `index`, `lint`, `delete`, `retag`, `move`, `rename` and `move-block`; it imposes no structure, and a missing store is an empty one, created empty by the first call of any verb. The repo's `agent/` folder ships everything an agent needs (shell-tool files, guidance, a live-context entry, an optional starting tree), and every merge to `main` publishes release archives carrying the binary and that folder.

The design is in [`docs/spec/agent-memory.md`](docs/spec/agent-memory.md).

## Usage

```bash
go build -o memory ./cmd/memory

memory read                        # the root (also memory read /): the top-level nodes and every tag in use

# --parent / is the root: this creates a top-level node, which an empty store needs first
memory write household --parent / --description "Running the house"
# create a node under an existing one: blocks in argv order, IDs assigned and returned
memory write kettle-descaling --parent household --description "How to descale the kettle" \
  --tag home --tag cleaning --property status=active --add "Use citric acid." --add "Rinse twice."

memory read household              # a node: blocks, children, links, backlinks
memory read household --depth 2    # children and grandchildren (--depth 0: none)
memory read kettle-descaling --block m-7f3a2k  # one block, with its own links and backlinks

# update it: --add appends, --replace ID=TEXT rewrites a block, any --tag replaces the tags,
# --property merges
memory write kettle-descaling --replace m-7f3a2k="Use citric acid, 2 tbsp." --tag home

memory search 'kettle -coffee'     # one quoted argument, Obsidian search syntax
memory search 'tag:home OR [status:active]'

memory index                       # <memory_index>: the top of the tree and the top tags

memory lint                        # <memory_lint>: every problem in the store; exit 2 if any node is missing
memory lint --exit-zero            # the same report, but exit 0 even when a node is missing

memory delete kettle-descaling --block m-7f3a2k   # delete one block; the node stays, even with no blocks left
memory delete kettle-descaling     # delete a node; refused if its folder holds children or other files
memory delete household --recursive   # delete a node and everything under it
memory retag cleaning chores       # rename a tag on every node: frontmatter tags and inline #tags
memory retag home                  # no new tag: remove the tag everywhere
memory move kettle-descaling --parent garden   # move a node, with everything under it, under the node garden
memory move kettle-descaling --parent /        # ... or to the top level
memory rename kettle-descaling kettle-care     # change a node's stem and rewrite every link to it
memory move-block m-7f3a2k --to kettle-care    # move one block to the end of another node, keeping its ID

memory --parent-dir /srv/agent/.agent read household   # the store /srv/agent/.agent/memory
```

The store is always `<parent>/memory`, where `<parent>` is the first of: `--parent-dir DIR` (or `--parent-dir=DIR`), given before the verb; `MEMORY_PARENT_DIR` from the environment; `HOME`. So plain `memory read` gives `~/memory`, and `export MEMORY_PARENT_DIR=…` points every call elsewhere. A relative parent is taken against the working directory; nothing expands `~` (the shell does). A variable set but empty is an error, never a fall-through to the next source: `MEMORY_PARENT_DIR=""` fails even when `HOME` is set, and so does an empty `--parent-dir`. With none of the three, `memory` fails naming all three. After the verb, `--parent-dir` is not an option (it is an unknown flag, or part of `search`'s query), so it never changes the store there. There is no `--store` flag.

There is no install step, and `memory` seeds nothing and names no node: a missing store is an empty one. Every verb, once its arguments have passed, creates `memory/` and `memory/notes/` (mode 0755) when they are missing, and nothing else: no node, and no lock file except the one `write`, `delete`, `retag`, `move`, `rename` and `move-block` take. It then reports on the empty store: `read` prints the root with no nodes, `read STEM` is the unknown-stem error, `search` matches nothing, `index` prints its trust line and `<empty>No nodes yet.</empty>`, and `lint` reports `nodes="0" errors="0" warnings="0"`. Once `notes/` exists, `read`, `search`, `index` and `lint` write nothing, not even a lock file. An argument error creates nothing; a refused `write`, `delete`, `retag`, `move`, `rename` or `move-block` (an unknown parent, stem or block ID, say) still leaves the empty `memory/notes/` and its lock file. Because every verb creates a missing store, plain `memory read` in a fresh home directory leaves an empty `~/memory/notes/`. The parent must already exist: `memory` creates `memory/` inside it, never the parent itself, so a mistyped `--parent-dir` or `MEMORY_PARENT_DIR` fails instead of starting a store somewhere unexpected.

The structure of a store is whatever its tree is. The root, `notes/` itself, is the one fixed node, written `/`: `read /` is the root read, and `write STEM --parent /` creates a top-level node, so on an empty store the first write is one of those. The root has no content and is never deleted, moved or renamed: `write /`, `read / --block ID`, `delete /`, `move /` and `rename /` are errors. Every other node, top-level ones included, can be deleted (`delete`), put somewhere else (`move`) and given a new stem (`rename`), and any block can be moved to another node (`move-block`); the root holds no blocks, so `move-block ID --to /` is an error too. The tree can also be rearranged by hand (a node is a folder holding `<folder>/<folder>.md` with a one-line `description`), in a file manager or Obsidian; a rename by hand does not rewrite links, `rename` does.

`write STEM` creates the node if the stem is new (then `--parent` and `--description` are required, `--parent /` for a top-level node) and updates it otherwise (`--parent`, if given, must be the node's own, `/` for a top-level node). Each `--add` and `--replace` is one block, one plain paragraph, numbered from 1 in argv order; `--property` values are strings. `write` cannot clear tags or delete blocks: `retag` and `delete` do. It prints `<memory_write created="true|false" stem="…">` with one `<assigned>` ID per block, then the node as `read --depth 0` shows it.

`delete STEM --block ID` removes that one block from the node's note and changes nothing else in it; the node stays, even when its last block goes. `delete STEM` removes the node, which is its folder. It is refused when the folder holds anything besides the node's own note: child nodes, a folder that did not load as a node (`lint` shows why), or any other file. The error names the node, says what is in the way (`1 child`, `2 children`, or the first such entry by name) and says to add `--recursive`, so a plain delete never removes something `read` did not show. Dot-entries (`.DS_Store`) are not content and go with the folder. `delete STEM --recursive` removes the folder and everything in it, loaded or not. `--block` with `--recursive` is an error. Deleting cannot be undone, and it never edits other notes: links to what was deleted are left as they are, and the result lists each one, by the stem of the node that holds it, then block order, then position in the block:

```xml
<memory_delete stem="old-plan" nodes="3" blocks="11">
  <dangling stem="projects" block="m-4k2p9x">[[old-plan]]</dangling>
  <dangling stem="priya" block="m-9d1c0e">[[old-task#^m-2b7q8w]]</dangling>
</memory_delete>
```

`nodes` and `blocks` count what was removed (a block delete is `nodes="0" blocks="1"`, with a `block` attribute). Each `<dangling>` is one link, as written, with the node and block that hold it; links inside what was deleted are not listed, nor links that were already dangling. `lint` reports the same links afterwards as warnings; repair them with `write --replace`.

`retag OLD NEW` renames a tag on every node that has it, in both places a tag lives: the frontmatter `tags`, and inline `#tag` words in block text (found as `search tag:` finds them). Tags are given with or without a leading `#`. Nested tags follow their parent, by whole path segment: `retag project proj` turns `project/atlas` into `proj/atlas` and leaves `projects` alone. A node that would end with the same tag twice keeps it once. `retag OLD` with no new tag removes it, and every `OLD/…` tag, from the frontmatter and from block text, where the `#tag` word goes together with one adjacent space. A block that is nothing but the tag cannot be emptied: the call fails naming the block and writes nothing (delete the block instead). Matching is exact, case included. In block text only the tag itself changes; only the notes that change are written. It prints `<memory_retag from="escalations" tag="escalation" nodes="6">` (no `tag` attribute for a removal), where `nodes` is the number of notes changed: a tag nobody has, or a new tag equal to the old, is `nodes="0"` and writes nothing.

`move STEM --parent P` puts the node under the node `P`, or under the root with `--parent /`, by renaming its folder into `P`'s folder. It is one directory rename: the node's children and whatever else is in its folder (a folder that did not load, a stray file) go with it, no note is rewritten, and every file keeps its bytes and its date. No link changes, because links go by stem and not by place. It is refused when `P` is unknown (near matches are listed), when `P` is the node itself or one of its own descendants, and when `P`'s folder already holds an entry of the node's name (it can only be something that did not load; `lint` shows why). Moving a node to the parent it already has is not an error: it prints the same result and writes nothing. It prints the node, the parent it left, the parent it has now (`/` for the root) and its path under `notes/`, as `read` gives it:

```xml
<memory_move stem="atlas-cutover" from="people" parent="projects" path="projects/atlas-cutover"></memory_move>
```

`rename STEM NEW` changes the node's stem: its folder becomes `NEW` and its note `NEW.md`, where they are, and the target of every link to the node, in any note, its own included, is rewritten: `[[STEM]]`, `[[STEM|alias]]`, `[[STEM#^id]]` and `[[STEM#^id|alias]]` become the same with `NEW`, aliases and block IDs kept. The links are the ones `read` shows; a `[[STEM]]` inside inline code, or in a form that is not a link here (`[[STEM#Heading]]`), is text and is left alone. In a rewritten note only the link targets change; notes with no link to the node are not written and keep their date. The node's description, tags and properties are not changed (update the description with `write` if it no longer fits). It is refused when `NEW` is not a valid stem (lowercase letters, digits and single hyphens, at most 64 characters), is a stem in use (the node's own included), or is the name of an entry already beside the node's folder (or `NEW.md` is a file already inside it). `NEW` is checked as strictly as `write` checks the stem of a node it creates: it is also refused when any folder anywhere in the store already has that name, even one that did not load (`lint` shows why it did not), and when it differs from such a folder's name only by letter case, on every filesystem. It prints the old stem, the new one, and the number of links rewritten (links, not blocks or notes):

```xml
<memory_rename from="atlas-migration" stem="atlas-cutover" links_rewritten="5"></memory_rename>
```

A link `[[NEW]]` that was dangling before the call resolves to the renamed node afterwards; it is not counted, because it was not rewritten. `rename` is all or nothing: if any note cannot be replaced, every note already rewritten is restored, the folder and note keep their old names, and the call fails.

`move-block ID --to STEM` moves one block to another node that already exists. `ID` is the block's ID as `read` prints it (`m-7f3a2k`, no caret). The block is taken out of the node that holds it, as `delete --block` removes one (that node may be left with no blocks; it is not deleted), and added as the last block of `STEM` with the same ID and the same text: one new paragraph after one blank line. Nothing else in `STEM`'s note changes, unlike a `write --add`: a hand-written paragraph with no ID is not given one, and blank lines are not tidied. Every link to the block, `[[old#^ID]]` and `[[old#^ID|alias]]`, becomes the same with `STEM`, wherever it is: in other notes, in the old node, in `STEM`, and in the moved block itself. Links to the old node (`[[old]]`) are not touched, nor is a link inside inline code. Only the notes that change are written. It is refused when no block has that ID, when more than one block carries it (`lint` warns about an ID used twice; the call cannot tell which is meant), when `--to` is missing, unknown (near matches are listed) or `/`, and when the block is already in `STEM`. It prints the block, the node it left, the node it is in now, and the number of links rewritten:

```xml
<memory_move_block id="m-7f3a2k" from="priya" stem="atlas-cutover" links_rewritten="2"></memory_move_block>
```

A link `[[STEM#^ID]]` that was dangling before the call resolves afterwards and is not counted. `move-block` is all or nothing, as `rename` is: if any note cannot be replaced, every note already rewritten is restored and the call fails.

`delete`, `retag`, `move`, `rename` and `move-block`, like `write`, take the store's lock, start from a fresh load, replace files atomically and check the store afterwards: a call that would leave the store with an error `lint` did not report before is rolled back and fails. They act only on nodes that loaded; a node `lint` reports as an error is invisible to them.

`search QUERY` takes the query as one argument. It prints `<memory_search query="…" matched="N" shown="M">`: at most 20 nodes, most hit blocks first, each with at most 5 matching blocks, within 30,000 bytes, and an `<omitted>` notice for the rest.

`index` prints `<memory_index>`: a trust line, the first-level nodes by stem with their children one level deep (each with its tags, true child count, date and description), and the 50 most used tags. It never shows a filesystem path. It is kept within 200 lines and 25,000 bytes: past either, the oldest second-level entries are dropped first, then the oldest first-level ones, and a notice says how many nodes are not shown. When no node loads (an empty store, or one whose every node is skipped) it prints the trust line and `<empty>No nodes yet.</empty>`, with no `<tags>`, so an index in an agent's live context says that memory exists and is empty.

`lint` checks the whole store after edits made outside `write` (an editor or Obsidian, a git merge, a sync conflict, a script). Those can break a note, and the reader then skips it: a skipped node, with everything under it, silently disappears from `read`, `search` and `index`. `lint` takes one option, `--exit-zero`, loads the store fresh and prints every problem the reader finds:

```xml
<memory_lint nodes="41" errors="1" warnings="2">
  <problem severity="error" path="b">missing frontmatter: a folder note must start with a --- line</problem>
  <problem severity="warning" path="a">link [[nowhere]] points to a missing node</problem>
  <problem severity="warning" path="a/notes.txt">not a node folder: this file is ignored and invisible to the agent</problem>
</memory_lint>
```

`nodes` counts the nodes that loaded. An *error* means the named node and its subtree are not in the store; a *warning* means it loaded (or was harmlessly ignored) and is worth knowing about, such as a dangling link, a stray file or a block ID used twice. Problems are listed by path, relative to `notes/` (`.` for `notes/` itself), with the reader's own reason. `lint` exits 0 when there are no errors (warnings or not), exit 2 when there are errors (the report on stdout, nothing on stderr), and exit 1, with one line on stderr and no report, when it cannot run at all (an argument, a missing parent, `memory/` or `notes` not a directory). A missing store is created empty first, and is clean. Beyond that, `lint` writes nothing, takes no lock and never repairs anything. `memory lint --exit-zero` prints the same report but exits 0 even when there are errors (the flake8/ruff convention); a failure to run is still exit 1. It is for callers that throw away a command's output when it exits non-zero, as the agent's shell tools do: the shipped tool file `agent/tools/memory_lint.json` runs `lint --exit-zero`, so an agent can check its own store (after you have edited notes by hand, say) and see the whole report, errors included. Installing it is optional ([`agent/README.md`](agent/README.md)). Scripts want plain `lint` and its exit code: `memory --parent-dir DIR lint || alert` fails exactly when nodes are missing or the check could not run, for example in a cron job, or as a git hook in a repo that versions the store:

```bash
# crontab: mail the report when the agent's store has lost a node
0 * * * * memory --parent-dir /srv/agent/.agent lint > /tmp/memory-lint.xml || mail -s "memory lint failed" ops@example.com < /tmp/memory-lint.xml

# .git/hooks/pre-commit in a repo whose memory/ is the store: refuse to commit while a note in the working tree is broken
memory --parent-dir "$(git rev-parse --show-toplevel)" lint > /dev/null || { echo "memory lint found errors: run memory lint" >&2; exit 1; }
```

Results are XML on stdout with exit 0. An error is one line on stderr with exit 1, naming the argument as typed (`STEM`, `--parent`, `--add #2`, `--replace m-7f3a2k`): an unknown stem lists up to five near matches, and a parent that does not exist says so with where it came from, as in `parent dir /srv/nowhere does not exist (from --parent-dir)` (or `(from MEMORY_PARENT_DIR)`, `(from HOME)`). `lint` alone also exits 2, as above (unless given `--exit-zero`). Exit 75 means retry: the store was busy (another `write`, `delete`, `retag`, `move`, `rename` or `move-block` held its lock for 10 s) or a note changed on disk while `write`, `delete`, `retag`, `rename` or `move-block` ran (an editor that does not take the lock); make the same call again. Dates (`modified`) are in the process's time zone (`TZ`).

## Wiring it to an agent

The agent knows nothing about memory. It sees shell tools, which run `memory` as a command, and whatever context the operator gives it. Below, `<state dir>` is the agent's state dir (by default `.agent` in its working directory), written as an absolute path. The agent's store is `<state dir>/memory`.

### 1. Install the binary

Download the archive for your platform from the repo's GitHub Releases. Each release has `memory_<tag>_<os>_<arch>.tar.gz` for `linux` and `darwin` on `amd64` and `arm64`, plus `checksums.txt`. An archive holds the binary, `memory_<tag>_<os>_<arch>/memory`, and the agent files, `memory_<tag>_<os>_<arch>/agent/` (the repo's `agent/` folder as it is):

```bash
tag=20261118   # a release tag
curl -fsSLO "https://github.com/riazarbi/agent-memory/releases/download/$tag/memory_${tag}_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/riazarbi/agent-memory/releases/download/$tag/checksums.txt"
shasum -a 256 -c --ignore-missing checksums.txt
tar -xzf "memory_${tag}_linux_amd64.tar.gz"   # gives memory_<tag>_linux_amd64/memory and .../agent/
```

Or build it from a checkout with Go: `go build -o memory ./cmd/memory`. The checkout has the same `agent/` folder.

### 2. Put it on the agent's PATH

Shell tools and live-context entries run `memory` by name, so it must be on the `PATH` of the agent process (they inherit its environment and working directory), for example `install -m 0755 memory /usr/local/bin/`. Check with `command -v memory` in the agent's environment.

### 3. Copy the agent files

[`agent/README.md`](agent/README.md) says what each file is, which ones to leave out, which agent versions can use them (the agent must substitute `${AGENT_STATE_DIR}`), and how to point an agent at a shared store. From the directory holding `agent/` (the unpacked archive's folder, or a checkout):

```bash
mkdir -p <state dir>/tools <state dir>/prompt <state dir>/livecontext
cp agent/tools/memory_*.json <state dir>/tools/
cp agent/prompt/memory_usage.md <state dir>/prompt/
cp agent/livecontext/memory_index.json <state dir>/livecontext/
[ -e <state dir>/memory ] || cp -R agent/memory <state dir>/
```

The last line installs the optional starting tree (the top-level nodes `user`, `projects`, `people` and `topics`) only when the agent has no store yet, so it never copies into an existing store. Leave it out to start from an empty store, which the agent's first `memory` call creates (the live-context index, at the start of its first input, if you installed that entry). Once that has happened, `<state dir>/memory` exists and the last line copies nothing, so install the starting tree before the agent first runs; [`agent/README.md`](agent/README.md) says how to rearrange the tree.

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

See `CLAUDE.md` for the full QA gate.
