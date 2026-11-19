# agent-memory

The command is `memory`.

Long-term memory for an AI agent: a tree of markdown notes with YAML frontmatter, which a model can read, write and search, and which a human can browse as an Obsidian vault.

**Status:** the library (`internal/memory`) and the `memory` CLI are built and tested. The CLI's verbs are `init`, `read`, `write`, `search` and `index`; the repo's `agent/` folder ships everything an agent needs (shell-tool files, guidance, a live-context entry), and every merge to `main` publishes release archives carrying the binary and that folder.

The design is in [`docs/spec/agent-memory.md`](docs/spec/agent-memory.md). The conversation that produced it is in [`docs/research/`](docs/research/).

## Usage

```bash
go build -o memory ./cmd/memory

memory init                        # seed the store if it is missing
memory read                        # the first-level nodes and every tag in use
memory read topics                 # a node: blocks, children, links, backlinks
memory read topics --depth 2       # children and grandchildren (--depth 0: none)
memory read topics --block m-7f3a2k  # one block, with its own links and backlinks

# create a node under an existing one: blocks in argv order, IDs assigned and returned
memory write kettle-descaling --parent topics --description "How to descale the kettle" \
  --tag home --tag cleaning --property status=active --add "Use citric acid." --add "Rinse twice."
# update it: --add appends, --replace ID=TEXT rewrites a block, any --tag replaces the tags,
# --property merges
memory write kettle-descaling --replace m-7f3a2k="Use citric acid, 2 tbsp." --tag home

memory search 'kettle -coffee'     # one quoted argument, Obsidian search syntax
memory search 'tag:home OR [status:active]'

memory index                       # <memory_index>: the top of the tree and the top tags

memory --parent-dir /srv/agent/.agent read topics   # the store /srv/agent/.agent/memory
```

The store is always `<parent>/memory`, where `<parent>` is the first of: `--parent-dir DIR` (or `--parent-dir=DIR`), given before the verb; `MEMORY_PARENT_DIR` from the environment; `HOME`. So plain `memory init` gives `~/memory`, and `export MEMORY_PARENT_DIR=…` points every call elsewhere. A relative parent is taken against the working directory; nothing expands `~` (the shell does). A variable set but empty is an error, never a fall-through to the next source: `MEMORY_PARENT_DIR=""` fails even when `HOME` is set, and so does an empty `--parent-dir`. With none of the three, `memory` fails naming all three. After the verb, `--parent-dir` is not an option (it is an unknown flag, or part of `search`'s query), so it never changes the store there. There is no `--store` flag.

`init` creates `memory/notes/` with the first-level nodes `user`, `projects`, `people` and `topics`, and nothing else, only when `memory/` does not exist; once it exists, `init` changes nothing. `read` and `search` never create anything.

`write STEM` creates the node if the stem is new (then `--parent` and `--description` are required) and updates it otherwise. Each `--add` and `--replace` is one block, one plain paragraph, numbered from 1 in argv order; `--property` values are strings. `write` cannot clear tags or delete blocks. It prints `<memory_write created="true|false" stem="…">` with one `<assigned>` ID per block, then the node as `read --depth 0` shows it.

`search QUERY` takes the query as one argument. It prints `<memory_search query="…" matched="N" shown="M">`: at most 20 nodes, most hit blocks first, each with at most 5 matching blocks, within 30,000 bytes, and an `<omitted>` notice for the rest.

`index` prints `<memory_index>`: a trust line, the first-level nodes by stem with their children one level deep (each with its tags, true child count, date and description), and the 50 most used tags. It never shows a filesystem path. It is kept within 200 lines and 25,000 bytes: past either, the oldest second-level entries are dropped first, then the oldest first-level ones, and a notice says how many nodes are not shown. With `notes/` holding no node it prints nothing and exits 0; without `notes/` it is an error, as for `read`.

Results are XML on stdout with exit 0. An error is one line on stderr with exit 1, naming the argument as typed (`STEM`, `--parent`, `--add #2`, `--replace m-7f3a2k`): an unknown stem lists up to five near matches, and a store without `notes/` says `memory has no notes/ folder; run memory init`. Exit 75 means retry: the store was busy (another write held its lock for 10 s) or the note changed on disk while `write` ran (an editor that does not take the lock); make the same call again. Dates (`modified`) are in the process's time zone (`TZ`).

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

### 3. Run `memory init`

Give it the agent's state dir as the parent, so the store lands where the tool files point:

```bash
memory --parent-dir <state dir> init
```

It creates `<state dir>/memory/notes/` (the first-level nodes `user`, `projects`, `people`, `topics`) and nothing else. If `memory/` already exists it changes nothing.

### 4. Copy the agent files

[`agent/README.md`](agent/README.md) says what each file is, which ones to leave out, which agent versions can use them (the agent must substitute `${AGENT_STATE_DIR}`), and how to point an agent at a shared store. From the directory holding `agent/` (the unpacked archive's folder, or a checkout):

```bash
mkdir -p <state dir>/tools <state dir>/prompt <state dir>/livecontext
cp agent/tools/memory_*.json <state dir>/tools/
cp agent/prompt/memory_usage.md <state dir>/prompt/
cp agent/livecontext/memory_index.json <state dir>/livecontext/
```

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

See `CLAUDE.md` for the full QA gate.
