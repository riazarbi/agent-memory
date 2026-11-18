# agent-memory

Long-term memory for an AI agent: a tree of markdown notes with YAML frontmatter, which a model can read, write and search, and which a human can browse as an Obsidian vault.

**Status:** the library (`internal/memory`) is built and tested: read model, encoder, locked writer and search. The `agent-memory` CLI that exposes it as shell tools to an agent is being built: `init` and `read` work; `write`, `search`, `index` and the shipped tool files are still to come.

The design is in [`docs/spec/agent-memory.md`](docs/spec/agent-memory.md). The conversation that produced it is in [`docs/research/`](docs/research/).

## Usage so far

```bash
go build -o agent-memory ./cmd/agent-memory

agent-memory init                        # seed the store if it is missing
agent-memory read                        # the first-level nodes and every tag in use
agent-memory read topics                 # a node: blocks, children, links, backlinks
agent-memory read topics --depth 2       # children and grandchildren (--depth 0: none)
agent-memory read topics --block m-7f3a2k  # one block, with its own links and backlinks
```

The store is `<state dir>/memory`, where the state dir is found as the agent finds it: `AGENT_STATE_DIR` from the environment, else `AGENT_STATE_DIR` from `./.env`, else `.agent`, relative to the working directory. There is no `--store` flag; point it at another store by setting `AGENT_STATE_DIR`.

`init` creates `memory/notes/` with the first-level nodes `user`, `projects`, `people` and `topics`, and `memory/protocol/usage.md`, only when `memory/` does not exist; once it exists, `init` changes nothing. `read` never creates anything.

Results are XML on stdout with exit 0. An error is one line on stderr with exit 1: an unknown stem lists up to five near matches, and a store without `notes/` says `memory has no notes/ folder; run agent-memory init`. Dates (`modified`) are in the process's time zone (`TZ`).

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

See `CLAUDE.md` for the full QA gate.
