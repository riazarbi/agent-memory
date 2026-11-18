# agent-memory

Long-term memory for an AI agent: a tree of markdown notes with YAML frontmatter, which a model can read, write and search, and which a human can browse as an Obsidian vault.

**Status:** the library (`internal/memory`) is built and tested: read model, encoder, locked writer and search. The `agent-memory` CLI that exposes it as shell tools to an agent has not been built yet.

The design is in [`docs/spec/agent-memory.md`](docs/spec/agent-memory.md). The conversation that produced it is in [`docs/research/`](docs/research/).

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

See `CLAUDE.md` for the full QA gate.
