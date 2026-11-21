This repo is agent-memory: a long-term memory store for an AI agent, kept as a tree of markdown notes with YAML frontmatter. It is used as a standalone CLI, `memory`. An agent uses it through shell tools, and the operator decides where its index goes (a tool, a livecontext entry, the system prompt). The agent does not know memory exists; it sees only tool files.

Its precedent is `adulting` (`github.com/riazarbi/adulting`): commands exposed as argv shell tools with rich descriptions. Everything an operator copies into an agent ships in this repo's `agent/` folder (tool files, the guidance, a live-context entry, an optional starting tree) and is installed by hand; see `agent/README.md`.

## Where this came from
The library `internal/memory` was built inside the agent repo (`github.com/riazarbi/agent`, branch `memory`, commit `476f7da`), where memory was a set of agent built-ins. It was moved here unchanged. The agent-side adapters (`memory_read`, `memory_write`, `memory_search`, the `<memory_index>` renderer, the memory smokes) were left behind on that branch. Read them there as reference; do not copy them in, because they import agent packages.

`docs/spec/agent-memory.md` Part III (D32) describes the CLI and overrides Part II's agent-specific parts (tool gate, system-prompt injection, `memory.*` events, seeding in `agent.New`, built-in markers).

## Workflow
- Familiarise yourself with the codebase. Be able to answer the question, "what is this, and how does it work?"
- TDD: break work into small steps, write tests first, iterate.
- When a piece of work is done: run the QA gate, write the CHANGELOG entry, and report results.
- **The QA gate** is: `go test -race ./...`, `go build ./...` (compiles everything, writes no binary into the tree), `go mod tidy -diff`, `scripts/release/release_test.sh`, `golangci-lint run ./...`, and `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`.
- The whole tree is lint-clean. Keep it that way: lint runs on everything, with no backlog exception.
- I commit. **You never commit.**

## Docs and agent files travel with the code
`agent/` and the READMEs are part of the product: operators copy `agent/` into agents as it is, and the model reads its texts on every turn. A change to what a verb accepts, prints, creates, or how it fails (verbs, flags, output, error texts, exit codes, store layout) updates all of these in the same change:
- `docs/spec/agent-memory.md` first. Part III is authoritative, and the code conforms to it.
- The usage text and package comment in `cmd/memory/main.go`.
- `README.md`: the Usage section, the store paragraphs and the wiring guide.
- `agent/README.md`.
- `agent/tools/*.json` (descriptions and `prepend_args`) and `agent/livecontext/memory_index.json`.
- `agent/prompt/memory_purpose.md` and `agent/skills/*/SKILL.md`, which stay byte-equal to the spec's III.8 texts.
- `agent/memory/notes/`, which stays byte-equal to what `memory write <stem> --parent / --description <D>` creates.
- The CHANGELOG entry, which names the docs and agent files checked.

Before handing over, search all of these for the old behaviour (verb names, flags, error texts, exit codes). Tests pin some of it (the guidance and tool descriptions against the spec, the README install commands, the starting tree), not all, so a green gate does not prove the docs are current.

Text the model reads (the tool descriptions, the live-context description, the guidance) never names a node or presumes a typology: it sends the model to the index for the structure (spec III.6).

## Changelog
Every commit carries a `CHANGELOG.md` entry, written before the work is handed over for commit.
- Newest first. Header: `## YYYY-MM-DD - <what is now true>`, stated as an outcome, not an activity.
- One bullet per file or concern: `` - `path`: what changed and why ``. Record the why, the trap a future reader would fall into, and alternatives rejected. Mark new files `(new)`. Say when something was deliberately not done and why.
- One entry per unit of work. If the work grows before it is committed, extend its entry rather than adding a second.

## CI
`.github/workflows/ci.yml` runs `lint`, `test`, `tidy`, `vuln` and `build` (cross-compiles every release target) on every PR into `main` and every push to `main`. A push to `main` that passes all of them is released: tagged `YYYYMMDD` (`.2`, `.3` for later same-day releases), with the release archives (`memory_<tag>_<os>_<arch>.tar.gz`, each carrying the `memory` binary and the repo's `agent/` folder beside it), `checksums.txt` and the CHANGELOG lines added since the previous release published as a GitHub Release. There is no Docker image.
- The Go toolchain comes from the `toolchain` line in `go.mod`.

## Code rules
- `internal/memory` stays dependency-light. Its only third-party dependency is `gopkg.in/yaml.v3`; adding another needs a reason in the CHANGELOG.
- The library does not log. It returns errors, and warnings as values, and its caller decides how to show them.
