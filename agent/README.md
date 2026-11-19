# Agent files for `memory`

This folder holds everything an operator copies into an agent to give it memory. It is laid out like the agent's state dir, and it is installed by hand: `memory` itself never writes outside `<parent>/memory`. It ships in the repo and in every release archive, beside the binary.

Below, `<state dir>` is the agent's state dir (by default `.agent` in its working directory), written as an absolute path.

## What is here

| File | Copy to | What it is |
|---|---|---|
| `tools/memory_read.json` | `<state dir>/tools/` | Shell tool running `memory read`: reading nodes and blocks. |
| `tools/memory_write.json` | `<state dir>/tools/` | Shell tool running `memory write`: creating nodes and adding or replacing blocks. |
| `tools/memory_search.json` | `<state dir>/tools/` | Shell tool running `memory search`: searching. |
| `tools/memory_index.json` | `<state dir>/tools/` | Shell tool running `memory index`: the index on demand (takes no arguments). |
| `tools/memory_lint.json` | `<state dir>/tools/` | Shell tool running `memory lint --exit-zero`: the agent checks its own store, for example after you have edited notes by hand (takes no arguments). Optional. |
| `prompt/memory_usage.md` | `<state dir>/prompt/` | The guidance: when and what to save. A `prompt/` fragment, loaded every turn. |
| `livecontext/memory_index.json` | `<state dir>/livecontext/` | A live-context entry showing the index at the start of each input. |
| `memory/notes/` | `<state dir>/memory/notes/` | An optional starting tree: the top-level nodes `user`, `projects`, `people` and `topics`. Copied only into a store that does not exist yet. |
| `README.md` | nowhere | This file. |

## Install

From this folder (`agent/` in a checkout, or in an unpacked release archive), copy the files:

```bash
mkdir -p <state dir>/tools <state dir>/prompt <state dir>/livecontext
cp tools/memory_*.json <state dir>/tools/
cp prompt/memory_usage.md <state dir>/prompt/
cp livecontext/memory_index.json <state dir>/livecontext/
[ -e <state dir>/memory ] || cp -R memory <state dir>/
```

That installs everything. The last line copies the starting tree only when `<state dir>/memory` does not exist, so it never touches an existing store; without it the store starts empty, and the agent's first `memory` call of any kind creates it, empty (`memory` itself seeds nothing). That first call is usually the live-context index at the start of the agent's first input; after it, `<state dir>/memory` exists and the last line copies nothing, so copy the starting tree before the agent first runs. Copy less to grant less; the sections below say what each choice means.

## The tool files

Installing a tool file is the grant: leave one out and the agent lacks that capability. Without `memory_write.json` the agent's memory is read-only. If the index is in the live-context entry, `memory_index.json` is optional. Each description tells the model how to call its verb; the four verb tools' descriptions also say that exit 75 means "run the same command again" (`lint` cannot be busy, so `memory_lint.json`'s does not).

Each file's `prepend_args` are `["--parent-dir", "${AGENT_STATE_DIR}", "<verb>"]`, and `memory_lint.json`'s are `["--parent-dir", "${AGENT_STATE_DIR}", "lint", "--exit-zero"]`. The model's arguments come after the verb, where `--parent-dir` is not an option, so the model cannot move the store.

`memory_lint.json` is optional, like every tool file. It lets the agent check its own store, for example after you have edited notes by hand (an editor or Obsidian, a git merge, a sync conflict): a note the reader cannot load silently disappears from `read`, `search` and the index, and `lint` lists it. Its description tells the model that an error is for you to fix (its memory tools cannot) and which warnings it can fix itself. It runs `lint --exit-zero` because the agent's shell tools show the model only `exit status 2`, not the report, when a command exits non-zero; with `--exit-zero` the model always gets the report. It takes no model arguments. A cron job or git hook should run plain `memory lint` and use its exit code instead (see the repo's README).

## `${AGENT_STATE_DIR}` needs a recent agent

The tool files and the live-context entry need an agent that replaces `${AGENT_STATE_DIR}` in a tool's `prepend_args` and an entry's `args` with its absolute state dir (agent CHANGELOG 2026-11-19). An older agent passes the text literally, and every call then fails with `parent dir ${AGENT_STATE_DIR} does not exist (from --parent-dir)`, and creates nothing; the live-context entry shows that error at the start of every input.

## Pointing an agent at another store

The agent's store is `<state dir>/memory`, named explicitly with `--parent-dir`, so whatever the operator's shell exports (`MEMORY_PARENT_DIR`, `HOME`) cannot redirect it. To point an agent at another store (one shared by several agents, say), edit the path: replace `${AGENT_STATE_DIR}` with the store's parent dir (which must exist) in every tool file's `prepend_args` and in the live-context entry's `args`. Never delete the `--parent-dir` pair: the agent would then use `MEMORY_PARENT_DIR` or `$HOME/memory`, which may be a person's own store.

A store outside the agent's state dir is not covered by the agent's state-dir write deny, so the agent's file tools can edit its notes directly, bypassing `write`'s checks and lock.

## The guidance

`prompt/memory_usage.md` says when and what to save. It is a `prompt/` fragment, so the agent loads it every turn, whether or not the memory tools are installed. It is written for an agent that can read and write memory, so it should match the tools you installed: install it with the tools, edit it to match (for example, take out the saving advice if you leave out `memory_write.json`), or leave it out. Guidance that tells the model to save, with no tool to save with, sends it hunting for a way to write.

## The starting tree

`memory/notes/` is an example, not a requirement: `memory` imposes no structure, and the store's structure is whatever its tree is. It holds four top-level nodes, each a description and no blocks, exactly as `memory write <stem> --parent / --description <D>` would create them: `user` (who the operator is and how they want work done), `projects` (ongoing efforts, goals, incidents and commitments), `people` (who is who, and how to deal with them) and `topics` (everything else worth knowing). Install it, edit it, make your own, or leave it out.

To rearrange it, before or after installing, move folders around: a node is a folder holding `<folder>/<folder>.md`, whose frontmatter has a one-line `description`, so renaming, moving or deleting folders (in a file manager or Obsidian) reshapes the tree. Nothing the model reads names these nodes: the tool descriptions and the guidance send it to the index (or a root read) to learn the structure, so a different tree needs no other edit.

The model can add top-level nodes (`write` with `--parent /`), but no verb moves, renames or deletes a node, so it can never remove one. If the top level sprawls, prune it by hand.

## The live-context entry

`livecontext/memory_index.json` runs `memory --parent-dir ${AGENT_STATE_DIR} index` at the start of each input, so the agent sees the top of its memory without asking. It is optional: the index can instead be a tool (`tools/memory_index.json`), or nowhere, leaving the agent to use `read`.
