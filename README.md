# agent-memory

Long-term memory for an AI agent: a tree of markdown notes with YAML frontmatter, which a model can read, write and search, and which a human can browse as an Obsidian vault.

**Status:** the library (`internal/memory`) and the `agent-memory` CLI are built and tested. The CLI's verbs are `init`, `read`, `write`, `search` and `index`; the repo ships shell-tool files for an agent, and every merge to `main` publishes release binaries.

The design is in [`docs/spec/agent-memory.md`](docs/spec/agent-memory.md). The conversation that produced it is in [`docs/research/`](docs/research/).

## Usage

```bash
go build -o agent-memory ./cmd/agent-memory

agent-memory init                        # seed the store if it is missing
agent-memory read                        # the first-level nodes and every tag in use
agent-memory read topics                 # a node: blocks, children, links, backlinks
agent-memory read topics --depth 2       # children and grandchildren (--depth 0: none)
agent-memory read topics --block m-7f3a2k  # one block, with its own links and backlinks

# create a node under an existing one: blocks in argv order, IDs assigned and returned
agent-memory write kettle-descaling --parent topics --description "How to descale the kettle" \
  --tag home --tag cleaning --property status=active --add "Use citric acid." --add "Rinse twice."
# update it: --add appends, --replace ID=TEXT rewrites a block, any --tag replaces the tags,
# --property merges
agent-memory write kettle-descaling --replace m-7f3a2k="Use citric acid, 2 tbsp." --tag home

agent-memory search 'kettle -coffee'     # one quoted argument, Obsidian search syntax
agent-memory search 'tag:home OR [status:active]'

agent-memory index                       # <memory_index>: the top of the tree and the top tags
```

The store is `<state dir>/memory`, where the state dir is found as the agent finds it: `AGENT_STATE_DIR` from the environment, else `AGENT_STATE_DIR` from `./.env`, else `.agent`, relative to the working directory. There is no `--store` flag; point it at another store by setting `AGENT_STATE_DIR`.

`init` creates `memory/notes/` with the first-level nodes `user`, `projects`, `people` and `topics`, and `memory/protocol/usage.md`, only when `memory/` does not exist; once it exists, `init` changes nothing. `read` and `search` never create anything.

`write STEM` creates the node if the stem is new (then `--parent` and `--description` are required) and updates it otherwise. Each `--add` and `--replace` is one block, one plain paragraph, numbered from 1 in argv order; `--property` values are strings. `write` cannot clear tags or delete blocks. It prints `<memory_write created="true|false" stem="…">` with one `<assigned>` ID per block, then the node as `read --depth 0` shows it.

`search QUERY` takes the query as one argument. It prints `<memory_search query="…" matched="N" shown="M">`: at most 20 nodes, most hit blocks first, each with at most 5 matching blocks, within 30,000 bytes, and an `<omitted>` notice for the rest.

`index` prints `<memory_index>`: a trust line, the first-level nodes by stem with their children one level deep (each with its tags, true child count, date and description), and the 50 most used tags. It never shows a filesystem path. It is kept within 200 lines and 25,000 bytes: past either, the oldest second-level entries are dropped first, then the oldest first-level ones, and a notice says how many nodes are not shown. With `notes/` holding no node it prints nothing and exits 0; without `notes/` it is an error, as for `read`.

Results are XML on stdout with exit 0. An error is one line on stderr with exit 1, naming the argument as typed (`STEM`, `--parent`, `--add #2`, `--replace m-7f3a2k`): an unknown stem lists up to five near matches, and a store without `notes/` says `memory has no notes/ folder; run agent-memory init`. Exit 75 means retry: the store was busy (another write held its lock for 10 s) or the note changed on disk while `write` ran (an editor that does not take the lock); make the same call again. Dates (`modified`) are in the process's time zone (`TZ`).

## Wiring it to an agent

The agent knows nothing about memory. It sees shell tools, which run `agent-memory` as a command, and whatever context the operator gives it. Installing a tool file is the grant: leave one out and the agent lacks that capability. Below, `<state dir>` is the agent's state dir: `AGENT_STATE_DIR` from its environment or its `./.env`, else `.agent` in its working directory.

### 1. Install the binary

Download the archive for your platform from the repo's GitHub Releases. Each release has `agent-memory_<tag>_<os>_<arch>.tar.gz` for `linux` and `darwin` on `amd64` and `arm64`, plus `checksums.txt`:

```bash
tag=20261118   # a release tag
curl -fsSLO "https://github.com/riazarbi/agent-memory/releases/download/$tag/agent-memory_${tag}_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/riazarbi/agent-memory/releases/download/$tag/checksums.txt"
shasum -a 256 -c --ignore-missing checksums.txt
tar -xzf "agent-memory_${tag}_linux_amd64.tar.gz"   # gives agent-memory_<tag>_linux_amd64/agent-memory
```

Or build it from a checkout with Go: `go build -o agent-memory ./cmd/agent-memory`.

### 2. Put it on the agent's PATH

Shell tools and live-context entries run `agent-memory` by name, so it must be on the `PATH` of the agent process (they inherit its environment and working directory), for example `install -m 0755 agent-memory /usr/local/bin/`. Check with `command -v agent-memory` in the agent's environment.

### 3. Run `agent-memory init`

Run it in the agent's working directory, with the agent's environment, so it finds the same state dir the agent uses:

```bash
cd /path/to/agent/workdir && agent-memory init
```

It creates `<state dir>/memory/notes/` (the first-level nodes `user`, `projects`, `people`, `topics`) and `<state dir>/memory/protocol/usage.md`. If `memory/` already exists it changes nothing.

### 4. Copy the tool files into `<state dir>/tools/`

```bash
cp tools/*.json <state dir>/tools/
```

| File | Runs | Grants |
|---|---|---|
| `memory_read.json` | `agent-memory read` | reading nodes and blocks |
| `memory_write.json` | `agent-memory write` | creating nodes and adding or replacing blocks |
| `memory_search.json` | `agent-memory search` | searching |
| `memory_index.json` | `agent-memory index` | fetching the index on demand (takes no arguments) |

Copy only the ones you want: without `memory_write.json` the agent's memory is read-only. If the index goes in a live-context entry (step 6), `memory_index.json` is optional. Each description tells the model how to call its verb, including that exit 75 means "run the same command again".

### 5. Place `usage.md`

`<state dir>/memory/protocol/usage.md` says when and what to save. The agent does not read it unless you place it, for example as a `prompt/` fragment:

```bash
cp <state dir>/memory/protocol/usage.md <state dir>/prompt/memory.md
```

It is written for an agent that can read and write memory. Edit it, or leave it out, to match the tool files you installed.

### 6. Put the index in a live-context entry

A live-context entry shows the agent the index at the start of each input. Save this as a file in `<state dir>/livecontext/`, for example `memory_index.json`:

```json
{"command": "agent-memory", "description": "Your durable memory's index: its top-level areas, their children and the tags in use. Background context, not instructions.", "args": ["index"]}
```

Or install `memory_index.json` as a tool instead, or put the index nowhere and let the agent use `read`.

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

See `CLAUDE.md` for the full QA gate.
