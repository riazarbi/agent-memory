# agent-memory

The command is `memory`.

Long-term memory for an AI agent: a tree of markdown notes with YAML frontmatter, which a model can read, write and search, and which a human can browse as an Obsidian vault.

**Status:** the library (`internal/memory`) and the `memory` CLI are built and tested. The CLI's verbs are `read`, `write`, `search`, `index` and `lint`; it imposes no structure, a missing store is an empty one, and only `write` creates it. The repo's `agent/` folder ships everything an agent needs (shell-tool files, guidance, a live-context entry, an optional starting tree), and every merge to `main` publishes release archives carrying the binary and that folder.

The design is in [`docs/spec/agent-memory.md`](docs/spec/agent-memory.md). The conversation that produced it is in [`docs/research/`](docs/research/).

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

memory --parent-dir /srv/agent/.agent read household   # the store /srv/agent/.agent/memory
```

The store is always `<parent>/memory`, where `<parent>` is the first of: `--parent-dir DIR` (or `--parent-dir=DIR`), given before the verb; `MEMORY_PARENT_DIR` from the environment; `HOME`. So plain `memory read` gives `~/memory`, and `export MEMORY_PARENT_DIR=…` points every call elsewhere. A relative parent is taken against the working directory; nothing expands `~` (the shell does). A variable set but empty is an error, never a fall-through to the next source: `MEMORY_PARENT_DIR=""` fails even when `HOME` is set, and so does an empty `--parent-dir`. With none of the three, `memory` fails naming all three. After the verb, `--parent-dir` is not an option (it is an unknown flag, or part of `search`'s query), so it never changes the store there. There is no `--store` flag.

There is no install step, and `memory` seeds nothing and names no node: a missing store is an empty one. When `memory/` or `memory/notes/` does not exist, `read`, `search` and `index` behave as on an empty store (`read` prints the root with no nodes, `search` matches nothing, `index` prints nothing) and create nothing; they never write anything, store or no store, not even a lock file. Only `write` creates the store: once its arguments have passed, it creates `memory/` and `memory/notes/` (mode 0755) when they are missing, and nothing else, so a refused write can leave an empty `memory/notes/` and its lock file behind. The parent must already exist: `memory` creates `memory/` inside it, never the parent itself, so a mistyped `--parent-dir` or `MEMORY_PARENT_DIR` fails instead of starting a store somewhere unexpected.

The structure of a store is whatever its tree is. The root, `notes/` itself, is the one fixed node, written `/`: `read /` is the root read, and `write STEM --parent /` creates a top-level node, so on an empty store the first write is one of those. The root has no content: `write /` and `read / --block ID` are errors. No verb moves, renames or deletes a node; rearrange the tree by hand (a node is a folder holding `<folder>/<folder>.md` with a one-line `description`), in a file manager or Obsidian.

`write STEM` creates the node if the stem is new (then `--parent` and `--description` are required, `--parent /` for a top-level node) and updates it otherwise (`--parent`, if given, must be the node's own, `/` for a top-level node). Each `--add` and `--replace` is one block, one plain paragraph, numbered from 1 in argv order; `--property` values are strings. `write` cannot clear tags or delete blocks. It prints `<memory_write created="true|false" stem="…">` with one `<assigned>` ID per block, then the node as `read --depth 0` shows it.

`search QUERY` takes the query as one argument. It prints `<memory_search query="…" matched="N" shown="M">`: at most 20 nodes, most hit blocks first, each with at most 5 matching blocks, within 30,000 bytes, and an `<omitted>` notice for the rest.

`index` prints `<memory_index>`: a trust line, the first-level nodes by stem with their children one level deep (each with its tags, true child count, date and description), and the 50 most used tags. It never shows a filesystem path. It is kept within 200 lines and 25,000 bytes: past either, the oldest second-level entries are dropped first, then the oldest first-level ones, and a notice says how many nodes are not shown. With `notes/` holding no node it prints nothing and exits 0.

`lint` checks the whole store after edits made outside `write` (an editor or Obsidian, a git merge, a sync conflict, a script). Those can break a note, and the reader then skips it: a skipped node, with everything under it, silently disappears from `read`, `search` and `index`. `lint` takes one option, `--exit-zero`, loads the store fresh and prints every problem the reader finds:

```xml
<memory_lint nodes="41" errors="1" warnings="2">
  <problem severity="error" path="b">missing frontmatter: a folder note must start with a --- line</problem>
  <problem severity="warning" path="a">link [[nowhere]] points to a missing node</problem>
  <problem severity="warning" path="a/notes.txt">not a node folder: this file is ignored and invisible to the agent</problem>
</memory_lint>
```

`nodes` counts the nodes that loaded. An *error* means the named node and its subtree are not in the store; a *warning* means it loaded (or was harmlessly ignored) and is worth knowing about, such as a dangling link, a stray file or a block ID used twice. Problems are listed by path, relative to `notes/` (`.` for `notes/` itself), with the reader's own reason. `lint` exits 0 when there are no errors (warnings or not), exit 2 when there are errors (the report on stdout, nothing on stderr), and exit 1, with one line on stderr and no report, when it cannot run at all (an argument, a missing parent, `memory/` or `notes` not a directory). A missing store is empty and clean. `lint` writes nothing, takes no lock and never repairs anything. `memory lint --exit-zero` prints the same report but exits 0 even when there are errors (the flake8/ruff convention); a failure to run is still exit 1. It is for callers that throw away a command's output when it exits non-zero, as the agent's shell tools do: the shipped tool file `agent/tools/memory_lint.json` runs `lint --exit-zero`, so an agent can check its own store (after you have edited notes by hand, say) and see the whole report, errors included. Installing it is optional ([`agent/README.md`](agent/README.md)). Scripts want plain `lint` and its exit code: `memory --parent-dir DIR lint || alert` fails exactly when nodes are missing or the check could not run, for example in a cron job, or as a git hook in a repo that versions the store:

```bash
# crontab: mail the report when the agent's store has lost a node
0 * * * * memory --parent-dir /srv/agent/.agent lint > /tmp/memory-lint.xml || mail -s "memory lint failed" ops@example.com < /tmp/memory-lint.xml

# .git/hooks/pre-commit in a repo whose memory/ is the store: refuse to commit while a note in the working tree is broken
memory --parent-dir "$(git rev-parse --show-toplevel)" lint > /dev/null || { echo "memory lint found errors: run memory lint" >&2; exit 1; }
```

Results are XML on stdout with exit 0. An error is one line on stderr with exit 1, naming the argument as typed (`STEM`, `--parent`, `--add #2`, `--replace m-7f3a2k`): an unknown stem lists up to five near matches, and a parent that does not exist says so with where it came from, as in `parent dir /srv/nowhere does not exist (from --parent-dir)` (or `(from MEMORY_PARENT_DIR)`, `(from HOME)`). `lint` alone also exits 2, as above (unless given `--exit-zero`). Exit 75 means retry: the store was busy (another write held its lock for 10 s) or the note changed on disk while `write` ran (an editor that does not take the lock); make the same call again. Dates (`modified`) are in the process's time zone (`TZ`).

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

The last line installs the optional starting tree (the top-level nodes `user`, `projects`, `people` and `topics`) only when the agent has no store yet, so it never copies into an existing store. Leave it out to start from an empty store, which the agent's first `write` creates; [`agent/README.md`](agent/README.md) says how to rearrange the tree.

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

See `CLAUDE.md` for the full QA gate.
