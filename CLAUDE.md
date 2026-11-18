This repo is agent-memory: a long-term memory store for an AI agent, kept as a tree of markdown notes with YAML frontmatter. It is being turned into a standalone `agent-memory` CLI. An agent uses it through shell tools, and the operator decides where its index goes (a tool, a livecontext entry, the system prompt). The agent does not know memory exists; it sees only tool files.

Its precedent is `adulting` (`github.com/riazarbi/adulting`): commands exposed as argv shell tools with rich descriptions, seeded into the agent's `tools/` by whoever deploys it.

## Where this came from
The library `internal/memory` was built inside the agent repo (`github.com/riazarbi/agent`, branch `memory`, commit `476f7da`), where memory was a set of agent built-ins. It was moved here unchanged. The agent-side adapters (`memory_read`, `memory_write`, `memory_search`, the `<memory_index>` renderer, the memory smokes) were left behind on that branch. Read them there as reference; do not copy them in, because they import agent packages.

`docs/spec/agent-memory.md` still describes memory as agent built-ins: the tool gate, system-prompt injection, `memory.*` agent events, seeding into the state dir. The file format, read model, encoder, writer and search rules in it hold unchanged. The integration parts get revised for the CLI before the CLI is built.

## Workflow
- Familiarise yourself with the codebase. Be able to answer the question, "what is this, and how does it work?"
- TDD: break work into small steps, write tests first, iterate.
- When a piece of work is done: run the QA gate, write the CHANGELOG entry, and report results.
- **The QA gate** is: `go test -race ./...`, `go build ./...` (compiles everything, writes no binary into the tree), `go mod tidy -diff`, `scripts/release/release_test.sh`, `golangci-lint run ./...`, and `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`.
- The whole tree is lint-clean. Keep it that way: lint runs on everything, with no backlog exception.
- I commit. **You never commit.**

## Changelog
Every commit carries a `CHANGELOG.md` entry, written before the work is handed over for commit.
- Newest first. Header: `## YYYY-MM-DD - <what is now true>`, stated as an outcome, not an activity.
- One bullet per file or concern: `` - `path`: what changed and why ``. Record the why, the trap a future reader would fall into, and alternatives rejected. Mark new files `(new)`. Say when something was deliberately not done and why.
- One entry per unit of work. If the work grows before it is committed, extend its entry rather than adding a second.

## CI
`.github/workflows/ci.yml` runs `lint`, `test`, `tidy` and `vuln` on every PR into `main` and every push to `main`. There is no release job yet: it arrives with the CLI binary (`scripts/release/` is ready for it).
- The Go toolchain comes from the `toolchain` line in `go.mod`.

## Code rules
- `internal/memory` stays dependency-light. Its only third-party dependency is `gopkg.in/yaml.v3`; adding another needs a reason in the CHANGELOG.
- The library does not log. It returns errors, and warnings as values, and its caller decides how to show them.
