# CLI3 — `index`, the shipped tool files, and releases

**Milestone:** cli  |  **Builds on:** CLI2 (banked)

## Goal
The CLI is complete and installable:
- `agent-memory index` prints `<memory_index>`;
- the repo ships shell-tool files an operator copies into an agent's `tools/`;
- the README says how to wire it up;
- merges to `main` publish binaries.

## Scope
- `index` (Part III.2), using D30's layout and budget with Part III's generic notice.
- `tools/memory_read.json`, `tools/memory_write.json`, `tools/memory_search.json` and
  `tools/memory_index.json`, in the agent's shell-tool schema:
  - the keys `command`, `description`, `read_only`, `prepend_args`, `forbidden_args` and
    `allow_user_args`;
  - `"command": "agent-memory"`, and `prepend_args` holding the verb;
  - `read_only: true` for read, search and index;
  - `allow_user_args: false` for index.

  The descriptions carry all call mechanics, as D29 and D31's defaults do, rewritten for flags. They
  also say that exit 75 ("busy" or "changed on disk") means "run the same command again".
- `README.md`: install (download a release binary or `go build`), put it on the agent's `PATH`, run
  `init`, copy the tool files into the agent's `tools/`, place `usage.md`, and an example
  live-context entry (`{"command": "agent-memory", "description": "...", "args": ["index"]}`).
- `.github/workflows/ci.yml`:
  - a `build` job that cross-compiles via `scripts/release/build-binaries.sh`;
  - a `release` job on push to `main` that tags `YYYYMMDD` (`.2`, `.3` …) with
    `scripts/release/next-tag.sh`, and publishes a GitHub Release with the binaries and the
    CHANGELOG lines added since the previous release (`scripts/release/notes.sh`).

  No Docker image.
- A CHANGELOG entry.

## Not in scope
- Restructuring verbs, dreaming, an install command, Docker (Part III.5).
- Changing `internal/memory`, or the behaviour of the other verbs.

## Governing spec
- `docs/spec/agent-memory.md` Part III (III.2 `index` row and notice, III.4).
- D30's index layout and budget, with Part III's notice and local-zone dates. D29 and D31 for the
  description texts to adapt.

## Existing tests this unit supersedes
None.

## Acceptance criteria
1. **The index layout.** On a seeded store with children and tags, `index` prints exactly D30's
   `<memory_index>`:
   - the trust line;
   - the first-level nodes ordered by stem, each with its children nested one level;
   - `tags`, `children` (the true count) and `modified` (`YYYY-MM-DD`, local zone);
   - `<tags>` holding the top 50 tags by count, then name.

   No filesystem path appears. It exits 0. On a store whose `notes/` holds no loadable node, it
   prints nothing and exits 0.
   *Excludes:* depth beyond 2; a path in the output; an uncapped tag list (the index's cap is 50,
   unlike `read`'s).
2. **The index budget.** When the rendered element exceeds 200 lines or 25,000 bytes:
   - entries are dropped one at a time, second-level first, oldest first (ties by stem), then
     first-level in the same order, until both limits hold;
   - the notice `N nodes not shown: read a node to list all its children.` (`node` for one) appears,
     and counts towards the budget;
   - the trust line and `<tags>` stay.

   Check one case that goes over by lines and one that goes over by bytes.
   *Excludes:* head-order truncation; dropping first-level entries before second-level ones; a
   notice that names a tool.
3. **The tool files work as shipped.** For each file in `tools/`:
   - it parses with exactly the six schema keys above (unknown keys fail);
   - `command` is `agent-memory`;
   - the verb in `prepend_args`, run against a seeded store, behaves as that verb.

   `memory_index.json` has `allow_user_args: false`, and the other three `true` (or the key omitted,
   meaning true). Each description names every flag its verb accepts. A test checks this against
   the verb's flag set, so a description cannot drift from the code.
   *Excludes:* a description missing a flag; a `read_only` write tool.
4. **The README wiring guide.** The README documents, in order: install, `PATH`, `init`, copying
   `tools/*.json` into `<state dir>/tools/`, placing `usage.md`, and a live-context entry for
   `index` given as valid JSON in the agent's live-context schema. A test extracts that JSON block
   and checks it parses with `command`, `description` and `args`.
   *Excludes:* a wiring example that drifts from the schema.
5. **Releases.** `ci.yml` has the `build` and `release` jobs described in Scope:
   - the release job runs only on push to `main`;
   - it needs the `lint`, `test`, `tidy`, `vuln` and `build` jobs;
   - it holds `contents: write`;
   - it uses the repo's release scripts.

   `scripts/release/build-binaries.sh` produces `agent-memory_<tag>_<os>_<arch>.tar.gz` for
   linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64, plus `checksums.txt`, when run locally
   into a temp dir. `release_test.sh` still passes.
   *Excludes:* a release job that also runs on pull requests; a Docker step.

## Environment
- The build happens in a clean-room copy, with no `.git`. No LLM is needed. The gate is
  `CLAUDE.md`'s, and lint stays at 0 issues. `govulncheck` may be refused in the room; if so, say so,
  and the delivery lead runs it.

## Notes for the builder
- **Size target:** about 150 production Go lines for `index`, plus the JSON files and the YAML.
  Reuse CLI1's renderer pieces (child entries, local-zone dates).
- The agent's shell-tool loader requires a non-empty `description` and `command`. A tool with
  `allow_user_args: false` takes no arguments from the model.
- Do not edit `CLAUDE.md`. The delivery lead updates its CI section at banking.
