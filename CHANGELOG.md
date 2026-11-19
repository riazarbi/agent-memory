# Changelog

Dated entries, newest first. Each header is a unit of work; bullets capture the
detail.

## 2026-11-19 - `MEMORY_DIR` points `memory` at any store, so it works without an agent

- `cmd/memory/store.go`: `MEMORY_DIR` (the memory folder itself; from
  the environment, then `./.env`) is checked first. Without it,
  resolution is as before: `<AGENT_STATE_DIR>/memory`, the state dir from
  the environment, then `./.env`, then `.agent`. Empty values count as
  unset. `.env` is read once.
  - The agent fallback stays, because the agent does not export its
    default state dir: without it, an agent install would need
    configuration.
  - Rejected by the operator: `MEMORY_DIR` alone, which would make an agent
    with a custom `AGENT_STATE_DIR` set both; and reading `MEMORY_DIR`
    from the environment only.
- `cmd/memory/resolve_bin_test.go`: four cases.
  - `MEMORY_DIR` in the environment is the store itself and beats
    `AGENT_STATE_DIR`.
  - `MEMORY_DIR` in `.env` beats `AGENT_STATE_DIR` in the environment.
  - The environment beats `.env`.
  - An empty `MEMORY_DIR` is unset.

  Mutations that ignore `MEMORY_DIR`, or treat it as a state dir, each fail
  them. `binary_test.go`'s `runBin` also strips an inherited `MEMORY_DIR`,
  so the developer's environment cannot leak in.
- `docs/spec/agent-memory.md` (III.1, D32) and `README.md`: the new order.
- Written directly by the delivery lead: a small change. The full gate
  passes.

## 2026-11-18 - The CLI's command is `memory`

- The binary is renamed from `agent-memory` to `memory`. It is usable
  without an agent, as a notes tool over an Obsidian-compatible vault, so a
  plain name fits. The repo and Go module stay `agent-memory`.
  - `cmd/agent-memory/` moves to `cmd/memory/`.
  - The four `tools/*.json` files run `"command": "memory"`.
  - `scripts/release/build-binaries.sh` publishes
    `memory_<tag>_<os>_<arch>.tar.gz`.
  - Two error messages and the usage text now name `memory`.
  - README, spec Part III and D32, `CLAUDE.md`, `.gitignore` and the `ci.yml`
    comment follow.
- The model never sees the binary name: it calls the tools by their tool
  names, and none of the descriptions mention it. Only the "run memory
  init" and "quote it: memory search …" errors reach it, and both read
  naturally.
- Considered and kept: `agent-memory`, for being unambiguous on PATH.
  Nothing called `memory` was found on the operator's machine.
- A mechanical rename, done by the delivery lead with no behaviour change.
  The full gate passed, and an end-to-end run with the real agent and the
  renamed tool files loaded all four tools and answered `memory_read` from
  the store.

## 2026-11-18 - `agent-memory index` prints the index, the repo ships the four shell-tool files and a wiring guide, and every merge to `main` publishes binaries

- `cmd/agent-memory/index.go` (new): `index` prints D30's `<memory_index>` with `encoding/xml` `MarshalIndent`: the hardcoded trust line as leading character data, the first-level nodes sorted by stem, each with its children (store order) as `read` renders child entries (`tags`, the true `children` count, `modified` in the local zone), then `<omitted>` when the budget dropped entries, then `<tags>`: the top 50 by node count, ties by name (read's root list stays uncapped). No path appears. A `notes/` holding no loadable node prints nothing and exits 0 (no first-level node means no node). A missing `notes/` is Part III.3's error, exit 1, as for `read`: D30's "section without an index" has no CLI equivalent, and an operator wiring a live-context entry to an uninitialised store should hear about it. Arguments are refused before the store is looked at.
- The budget: the element, `<memory_index>` to `</memory_index>`, holds at most 200 lines and 25,000 bytes. Entries are dropped second-level first, oldest first (folder-note mtime at full precision, ties by stem), then first-level the same way. The notice is Part III.2's, `N nodes not shown: read a node to list all its children.` (`node` for one), and counts towards the budget. The fewest drops are found by binary search, not one render per drop: zero drops is tried alone (one drop adds the notice, which can outweigh a short entry), and from one drop on, each further drop removes a whole line and at least ~50 bytes while the notice grows by at most two, so the size only falls. The trap: a linear loop is quadratic, and a store big enough to need truncation is exactly the one where that hurts, on every input if the index is a live-context entry. If even dropping every entry does not fit (50 very long tags), every entry is dropped and the result is returned over budget, since the trust line, notice and `<tags>` are never dropped.
- `cmd/agent-memory/read.go`: `tagCounts`, `rankedTags` and `childEntry` are extracted so `index` shares `read`'s child entries and tag ordering, and `readFlags` so a test can check the tool file against read's flags. Behaviour unchanged.
- `cmd/agent-memory/write.go`: `writeFlags` extracted for the same test. Behaviour unchanged.
- `cmd/agent-memory/initverb.go`: `noArgs` extracted, shared by `init` and `index`; `init`'s message is unchanged.
- `cmd/agent-memory/search.go`: the `node`/`nodes` noun is the helper `nodeNoun`, shared with `index` (lint: goconst). Behaviour unchanged.
- `cmd/agent-memory/main.go`: `index` is dispatched and listed in the usage text.
- `tools/memory_read.json`, `memory_write.json`, `memory_search.json`, `memory_index.json` (new): argv shell tools in the agent's schema, every file carrying all six keys (`command: agent-memory`, `prepend_args: [verb]`, `forbidden_args: []`, `read_only` true except for write, `allow_user_args` false only for index). The descriptions are D29's and D31's rewritten for flags: the stem as the first argument, each flag and its value as separate arguments, a JSON-array example, and a closing sentence that exit 75 ("busy" or "changed on disk") means run the same command again. That sentence is in all four, as the unit asks, though only `write` can exit 75 today: `read`, `search` and `index` take no lock. `memory_index.json` describes the index and says it takes no arguments. `forbidden_args` is empty because nothing a verb accepts should be blocked; its semantics are the agent's and are not visible in this repo.
- `README.md`: a "Wiring it to an agent" guide in six steps, in order: install (release archive and checksum, or `go build`), the agent's `PATH`, `init` in the agent's working directory, `cp tools/*.json <state dir>/tools/` (with which file grants what), placing `usage.md` as a `prompt/` fragment, and a live-context entry for `index` as JSON. "Usage so far" is now "Usage" and covers `index`; the status line is updated.
- `.github/workflows/ci.yml`: a `build` job cross-compiles every target with `scripts/release/build-binaries.sh` on every PR and push, so a broken target fails before release. A `release` job runs only on a push to `main` (`if: github.event_name == 'push' && github.ref == 'refs/heads/main'`), needs `lint`, `test`, `tidy`, `vuln` and `build`, and alone holds `contents: write`. It checks out full history, picks the tag with `next-tag.sh` from `git tag --list` and today's UTC date, diffs `CHANGELOG.md` against the newest release tag behind `HEAD` (`git describe --match` on the `YYYYMMDD` pattern; the empty tree before the first release, so the first notes are the whole file) through `notes.sh`, builds with the tag, and runs `gh release create TAG dist/*.tar.gz dist/checksums.txt --target $GITHUB_SHA`, which creates the tag with the release. Step outputs reach scripts through `env`, not `${{ }}` inside `run`. No Docker image (Part III.5). Not exercised in this build: the room has no git and no GitHub, so the release steps are checked only structurally; the first merge is their real test. Re-running a release job for an already-released commit would publish a second release (`.2`) with empty notes; not guarded.
- `cmd/agent-memory/index_bin_test.go` (new): the layout is a byte-for-byte golden in `Pacific/Auckland`, where the fixture's UTC dates are a day behind, with a third-level node that must not appear, a description needing escapes, and an extra property and inline field whose removal changes nothing. The tag cap uses 60 tags with tied counts. The budget tests compare the binary's output with a model built by plain string formatting (no `encoding/xml`), which drops in D30's order and finds the fewest drops by trying each count: over by lines only (250 children with tied mtimes; 59 dropped, by formula), over by bytes only, every child then first-level nodes with tied mtimes, a singular notice, and edges at exactly 200 lines and exactly 25,000 bytes, each untruncated, with one line or byte more truncated.
- `cmd/agent-memory/tools_test.go` (new): each file is parsed strictly (exact key spelling, since `encoding/json` matches keys case-insensitively; nothing after the object). The flags a description names (every `--word`) must equal the flag set built by `readFlags`/`writeFlags`, in both directions; `search` and `index` have none. Each file is then run as the agent would (its `prepend_args` plus model arguments) against an `init`-seeded store: write creates a node, read returns it, search finds it, index lists it under `topics`.
- `cmd/agent-memory/readme_test.go` (new): the wiring section's `###` steps must be the six above in order. The first `json` block in the live-context step must be one JSON object with only `command`, `description` and `args`, running `agent-memory index`.
- `cmd/agent-memory/release_ci_test.go` (new): `ci.yml` parsed with `yaml.v3` (already a dependency): the jobs exist, the build job runs `build-binaries.sh`, and the release job's condition, `needs`, permissions and use of the three release scripts and `gh release create` are as above. No `docker` or `ghcr.io` anywhere. `build-binaries.sh` is run into a temp dir: exactly four archives and `checksums.txt`, each checksum matching, each archive holding `agent-memory_<tag>_<os>_<arch>/agent-memory` whose ELF machine or Mach-O CPU matches its name.
- `internal/memory`: unchanged. No dependency added.
- Not done, by scope: restructuring verbs, dreaming, an install command and Docker (Part III.5). No smoke test: this repo has no smoke suite, and the binary tests run the real binary against real directories. `CLAUDE.md`'s CI section, which still says there is no release job, is left for the delivery lead, as the unit asks.

- `docs/delivery/done/cli/CLI3-index-tools-release.md` (moved from
  `doing/`): independent verification passed all five criteria.
  - It caught 17 of 17 `index` mutants and 29 of 29 tool-file, README, CI
    and script mutants.
  - An independent oracle matched the budget logic on 120 random stores.
  - The delivery lead also ran an end-to-end check against the real agent
    binary and a stub backend. The agent loaded all four shipped tool
    files; the live-context `index` entry reached the model; a model
    `memory_read` call returned the node from the store.
- `docs/spec/agent-memory.md`: Part III.4 lists the four tool files. Four
  CLI3 behaviours are recorded as known: `index` before `init` errors on
  every input; the budget can be unsatisfiable with huge tags; all four
  descriptions mention exit 75; a re-run release job publishes a `.2`.
- `CLAUDE.md`: the CI section describes the `build` and `release` jobs, and
  the spec note points at Part III.
## 2026-11-18 - `agent-memory write` saves to the store and `agent-memory search` finds things in it, and exit 75 means retry

- `cmd/agent-memory/write.go` (new): `write STEM [--parent P] [--description D] [--tag T]… [--property K=V]… [--add TEXT]… [--replace ID=TEXT]…` maps onto one `memory.WriterFor(notes).Write(WriteRequest)` (D29, Part III.2). `--add` and `--replace` are recorded as CLI1's walker meets them, so the blocks keep argv order: `--add A --replace X=… --add B` assigns a new ID, `X`, then a new ID. Any `--tag` replaces the tag list; with none, the tags are untouched. `--property` values are strings and are merged; a repeated key keeps the last value, as a repeated `--description` does. `--replace` with no `=` or an empty ID is refused by the walker: an empty ID would make the library append instead of replace. The result is `<memory_write created stem>` with one `<assigned>` per block, `<adopted>`, and the node from CLI1's `nodeOf` at depth 0 against the write's own read-back store, so backlinks are as of the write. Writer field errors (`*memory.FieldError`, matched by type, not by message text) are renamed to the arguments as typed: `stem` → `STEM`, `parent`/`description` → the flag, `tags[i]` → `--tag #i+1`, `properties["k"]` → `--property "k"`, `blocks[i]` → `--add #i+1` (numbered from 1 across every `--add` and `--replace`) or, for a replacement, `--replace <ID>` (for its text as well as its ID, as Part III.2's example shows). The reason text after the name is the writer's own, so a few still use the JSON tool's wording ("omit the id", "give it as the description field"). Order of checks: arguments, then `notes/` (Part III.3's message; nothing created, no lock file), then the writer.
- `cmd/agent-memory/search.go` (new): `search QUERY` runs `memory.ParseQuery` and `memory.Search` and keeps the library's ranking. The CLI applies D31's caps: 20 nodes and 5 hit blocks each, `hits` keeping the true count. Then the 30,000-byte budget on the rendered `<memory_search>` element (the final newline excluded): drop the lowest-ranked node one at a time, and with one node left drop its blocks from the last, never below its description. `<omitted nodes="N">` appears whenever `matched > shown`, `node` for one. Search takes no flags. The query is one argument, verbatim and untrimmed in the `query` attribute, so `search '-draft kettle'` is a query, not an unknown flag. A leading `--` is dropped as the end of flags, for tool files that want it. More than one argument is refused with `QUERY: …; quote it`, never joined, because joining would lose the user's quoting. Order of checks as in D31: arguments, then `notes/`, then the query, whose errors are the library's exact D31 text (`query: empty`, …), a missing argument included.
- `cmd/agent-memory/main.go`: `memory.ErrBusy` and `memory.ErrChangedOnDisk`, matched with `errors.Is`, exit 75 (`EX_TEMPFAIL`) with their one stderr line. Every other error stays exit 1. The usage text lists `write` and `search`.
- `cmd/agent-memory/args.go`: the `--` terminator is a named constant shared with `search` (lint: goconst). Behaviour unchanged.
- `cmd/agent-memory/write_bin_test.go`, `search_bin_test.go` (new): every criterion runs the built binary on an `init`-seeded store in a fresh temp directory. Results are parsed with test-local XML types. Expected search ranking comes from the library itself (`memory.Search`), so the test fails if the CLI re-ranks. Fixtures are shaped so ranking needs hits, mtime and stem. The budget tests check that the element fits, that the top-ranked nodes or the first blocks are kept, and that no more was dropped than needed (equal-sized nodes and blocks, so one more would not fit). Error cases check the exact argument prefix, that no writer field name appears, and that the whole state dir is byte-, mtime- and mode-identical afterwards.
- `cmd/agent-memory/retry_bin_test.go` (new, `unix` only): busy holds a real `flock` on `notes/.write.lock` from the test process and waits out the writer's 10 s. The library's `lockWait` seam is unexported, and an `internal/memory` export just for tests was out of scope ("no change to `internal/memory`"). The test runs in parallel, so it overlaps the other parallel tests only. Changed-on-disk has no deterministic trigger from outside the process. A goroutine replaces the folder note atomically and continuously (an editor that does not take the lock) while `write` runs up to 30 times. The test requires at least one exit 75 with "changed on disk" and fails on any exit 1. It saw a refusal on the first few attempts in 10 of 10 runs.
- `README.md`: "Usage so far" covers `write`, `search` and exit 75, and the status line is updated.
- `internal/memory`: unchanged.
- Not done, by scope: `index`, tool files and releases (CLI3), and clearing tags or deleting blocks (restructuring). No smoke test: this repo has no smoke suite, and the binary tests run the real binary against real directories. `govulncheck` was not run in this build: the environment refused `go run golang.org/x/vuln/cmd/govulncheck@latest`. No dependency was added.

- `docs/delivery/done/cli/CLI2-write-search.md` (moved from `doing/`):
  independent verification passed all seven criteria, and caught 21 of 22
  mutants. Production grew by about 326 lines. My criterion 1 wrongly
  asked `read` to show a property (D29 renders none); the builder checked
  it through the library. The spec records the JSON-tool wording left in
  writer reasons, and `search`'s verbatim query.
- `docs/delivery/doing/CLI3-index-tools-release.md` (new; skeleton removed): five criteria covering `index`, the four shipped tool files (including `memory_index.json`), the README wiring guide and binary releases.
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
