# Changelog

Dated entries, newest first. Each header is a unit of work; bullets capture the
detail.

## 2026-11-18 - An `agent-memory` binary finds the agent's store, seeds it with `init`, and reads it with `read`

- `cmd/agent-memory/main.go` (new): verb dispatch (`init`, `read`). No verb or an unknown verb prints usage to stderr and exits 1. A verb's result reaches stdout only on success; an error is one line on stderr (line breaks inside it become spaces) with exit 1 (Part III.3). Exit 75 is not defined yet: only `write` can be busy, and it arrives in CLI2. The binary embeds `time/tzdata`, so a `TZ` name resolves even in a container with no zone files; without it Go silently falls back to UTC and dates would be a day off for half the world.
- `cmd/agent-memory/store.go` (new): the store is `<state dir>/memory`. The state dir is `AGENT_STATE_DIR` from the environment if non-empty, else from `./.env` if non-empty, else `.agent`; a relative one stays relative, so it is taken against the working directory (Part III.1). The `.env` reader follows Part III.1's pinned grammar (the agent's `loadDotEnv` / `parseDotEnvValue`): lines trimmed, blank and `#` lines skipped, only `export ` with a space stripped, split at the first `=`, the value left-trimmed, a terminated quote giving the text between, otherwise (an unterminated quote included) cut at the first ` #` or `\t#` and trimmed, so a value starting with `#` is kept. The first occurrence of the key wins, even when it is empty (it then falls through to `.agent`). An unreadable `.env` (other than missing) is an error, not a fall-through to `.agent`: guessing could land on a store the agent does not use.
- `cmd/agent-memory/args.go` (new): flags and positionals in any order (`read STEM --depth 2` and `read --depth 2 STEM`). Go's `FlagSet.Parse` stops at the first positional, so the arguments are walked by hand and the per-verb `flag.FlagSet` is used only for definitions and `Set`. `--` ends flags; a non-boolean flag takes the next argument whatever it looks like (as `flag` does, so `--depth -1` is a value, then refused). Errors name the flag as typed (`--depth`, `-depth`), never `flag`'s rewritten `-name`. Rejected: calling `Parse` repeatedly on the remainder, which cannot tell a `--` terminator from a flag value `--` and still reports names in `flag`'s form.
- `cmd/agent-memory/initverb.go` (new): `init` calls `memory.SeedDefaults` only after its own `Lstat` finds no `memory/`, because `SeedDefaults` returns nil either way and the result must say whether anything was seeded. Once `memory/` exists, even empty, nothing is touched: a deleted `usage.md` or first-level node stays deleted (D19). The result is XML like every other success (Part III.3): `<init store="<abs path>" seeded="true">` with one `<created>` per file written, or `seeded="false"` and the text `memory/ already exists; nothing was needed`.
- `cmd/agent-memory/read.go` (new): D29's `memory_read` result built with `encoding/xml` from `memory.Load`'s store, never re-parsing files. A node gets `stem`, `path`, `parent` (omitted for a first-level node), `tags` (omitted when empty) and `modified` (`YYYY-MM-DD` in the local zone), `<description>`, its `<block id>`s, `<children>` entries to `--depth` (default 1; grandchildren nest inside their parent's entry, as in D30's index), `<links>` (every edge occurrence, `dangling="true"` on a missing target) and `<backlinks>`. Empty `<children>`, `<links>` and `<backlinks>` are omitted. `--block ID` gives the node's attributes, description and that one block with the block's own links and backlinks, and no children. No `STEM` gives `<memory>` with the first-level nodes (depth applies to them too) and `<tags>` listing every tag with its node count, most used first, ties by name, with no top-50 cap (D29 ruling). Argument errors come before the store is touched; a missing or non-directory `notes/` gives `memory has no notes/ folder; run agent-memory init` and creates nothing (no `notes/`, no lock file: `read` never takes the writer's lock). Load problems are not reported: Part III has no channel for them besides the result, and none was specified.
- `cmd/agent-memory/near.go` (new): an unknown stem's error lists up to five near matches, closest first: stems containing it or contained in it, or within an edit distance of half the longer stem. The spec says "near matches" without a metric; this is the build's choice.
- `cmd/agent-memory/*_test.go` (new): every acceptance case runs the built binary (built once in `TestMain`) in a fresh temp directory with `AGENT_STATE_DIR` and `TZ` stripped from the inherited environment, and parses results with test-local XML types so a wrong production tag fails. Unit tests cover the `.env` reader, argument walking, near matches and tag ordering.
- `internal/memory`: unchanged, and no helper added.
- `README.md`: a "Usage so far" section for `init` and `read`, and the status line.
- Not done, by scope: `write`, `search`, `index`, tool files and a release job (CLI2, CLI3). No smoke test: this repo has no smoke suite, and the binary tests already run the real binary against real directories.

- `docs/spec/agent-memory.md` (at CLI1): the `.env` grammar is pinned to
  the agent's exactly, first occurrence winning, so the CLI and the agent
  cannot resolve different stores. Where D29–D31 say `AGENT_TIMEZONE`, read
  the local zone. Three known gaps are recorded: a `memory/` without
  `notes/` loops between `read` and `init`; concurrent `init`s can each
  report seeding; skipped nodes are silent.
- `docs/delivery/done/cli/CLI1-skeleton-init-read.md` (moved from
  `doing/`): independent verification passed all six criteria, caught 43
  of 46 mutants, and all 10 inverted assertions failed. Production is 660
  lines.
- `docs/delivery/doing/CLI2-write-search.md` (new; skeleton removed):
  seven criteria, with a target of about 250 production lines.
## 2026-11-18 - The spec says how memory works as the agent-memory CLI, and its first milestone is planned

- `docs/spec/agent-memory.md`: Part III and D32 (the operator's rulings).
  Memory is the `agent-memory` binary, with the verbs `init`, `read`,
  `write`, `search` and `index`.
  - The store is `<agent state dir>/memory`, found exactly as the agent
    finds its state dir (environment, then `./.env`, then `.agent`). There
    is no `--store` flag.
  - `write` takes flags, not JSON.
  - `index` prints only `<memory_index>`, and the operator places it.
  - Tool files ship in `tools/`.
  - Results are XML on stdout, errors go to stderr, and the exit codes are
    0, 1 and 75 (retry). There is no logging and no capability line.
  - A banner tells readers that Part III overrides Part II's
    agent-specific parts.
- `docs/delivery/doing/CLI1-skeleton-init-read.md` (new) and
  `docs/delivery/todo/cli/` (new): the `cli` milestone in three units:
  binary, store, `init` and `read`; then `write` and `search`; then
  `index`, tool files and releases.

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
