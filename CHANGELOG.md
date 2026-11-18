# Changelog

Dated entries, newest first. Each header is a unit of work; bullets capture the
detail.

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
