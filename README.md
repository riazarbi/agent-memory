# agent-memory

Long-term memory for an AI agent: a tree of markdown notes with YAML frontmatter, which a model can read, write and search, and which a human can browse as an Obsidian vault.

**Status:** the library (`internal/memory`) is built and tested: read model, encoder, locked writer and search. The `agent-memory` CLI that exposes it as shell tools to an agent is being built: `init`, `read`, `write` and `search` work; `index` and the shipped tool files are still to come.

The design is in [`docs/spec/agent-memory.md`](docs/spec/agent-memory.md). The conversation that produced it is in [`docs/research/`](docs/research/).

## Usage so far

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
```

The store is `<state dir>/memory`, where the state dir is found as the agent finds it: `AGENT_STATE_DIR` from the environment, else `AGENT_STATE_DIR` from `./.env`, else `.agent`, relative to the working directory. There is no `--store` flag; point it at another store by setting `AGENT_STATE_DIR`.

`init` creates `memory/notes/` with the first-level nodes `user`, `projects`, `people` and `topics`, and `memory/protocol/usage.md`, only when `memory/` does not exist; once it exists, `init` changes nothing. `read` and `search` never create anything.

`write STEM` creates the node if the stem is new (then `--parent` and `--description` are required) and updates it otherwise. Each `--add` and `--replace` is one block, one plain paragraph, numbered from 1 in argv order; `--property` values are strings. `write` cannot clear tags or delete blocks. It prints `<memory_write created="true|false" stem="…">` with one `<assigned>` ID per block, then the node as `read --depth 0` shows it.

`search QUERY` takes the query as one argument. It prints `<memory_search query="…" matched="N" shown="M">`: at most 20 nodes, most hit blocks first, each with at most 5 matching blocks, within 30,000 bytes, and an `<omitted>` notice for the rest.

Results are XML on stdout with exit 0. An error is one line on stderr with exit 1, naming the argument as typed (`STEM`, `--parent`, `--add #2`, `--replace m-7f3a2k`): an unknown stem lists up to five near matches, and a store without `notes/` says `memory has no notes/ folder; run agent-memory init`. Exit 75 means retry: the store was busy (another write held its lock for 10 s) or the note changed on disk while `write` ran (an editor that does not take the lock); make the same call again. Dates (`modified`) are in the process's time zone (`TZ`).

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

See `CLAUDE.md` for the full QA gate.
