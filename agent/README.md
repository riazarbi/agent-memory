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
| `prompt/memory_usage.md` | `<state dir>/prompt/` | The guidance: when and what to save. A `prompt/` fragment, loaded every turn. |
| `livecontext/memory_index.json` | `<state dir>/livecontext/` | A live-context entry showing the index at the start of each input. |
| `README.md` | nowhere | This file. |

## Install

From this folder (`agent/` in a checkout, or in an unpacked release archive), copy the files:

```bash
mkdir -p <state dir>/tools <state dir>/prompt <state dir>/livecontext
cp tools/memory_*.json <state dir>/tools/
cp prompt/memory_usage.md <state dir>/prompt/
cp livecontext/memory_index.json <state dir>/livecontext/
```

That installs everything. There is nothing to seed: the store, `<state dir>/memory`, appears on the agent's first memory call, which creates `memory/notes/` with the first-level nodes `user`, `projects`, `people` and `topics`. Copy less to grant less; the sections below say what each choice means.

## The tool files

Installing a tool file is the grant: leave one out and the agent lacks that capability. Without `memory_write.json` the agent's memory is read-only. If the index is in the live-context entry, `memory_index.json` is optional. Each description tells the model how to call its verb, including that exit 75 means "run the same command again".

Each file's `prepend_args` are `["--parent-dir", "${AGENT_STATE_DIR}", "<verb>"]`. The model's arguments come after the verb, where `--parent-dir` is not an option, so the model cannot move the store.

## `${AGENT_STATE_DIR}` needs a recent agent

The tool files and the live-context entry need an agent that replaces `${AGENT_STATE_DIR}` in a tool's `prepend_args` and an entry's `args` with its absolute state dir (agent CHANGELOG 2026-11-19). An older agent passes the text literally, and every call then fails with `parent dir ${AGENT_STATE_DIR} does not exist (from --parent-dir)`, and creates nothing; the live-context entry shows that error at the start of every input.

## Pointing an agent at another store

The agent's store is `<state dir>/memory`, named explicitly with `--parent-dir`, so whatever the operator's shell exports (`MEMORY_PARENT_DIR`, `HOME`) cannot redirect it. To point an agent at another store (one shared by several agents, say), edit the path: replace `${AGENT_STATE_DIR}` with the store's parent dir (which must exist) in every tool file's `prepend_args` and in the live-context entry's `args`. Never delete the `--parent-dir` pair: the agent would then use `MEMORY_PARENT_DIR` or `$HOME/memory`, which may be a person's own store.

A store outside the agent's state dir is not covered by the agent's state-dir write deny, so the agent's file tools can edit its notes directly, bypassing `write`'s checks and lock.

## The guidance

`prompt/memory_usage.md` says when and what to save. It is a `prompt/` fragment, so the agent loads it every turn, whether or not the memory tools are installed. It is written for an agent that can read and write memory, so it should match the tools you installed: install it with the tools, edit it to match (for example, take out the saving advice if you leave out `memory_write.json`), or leave it out. Guidance that tells the model to save, with no tool to save with, sends it hunting for a way to write.

## The live-context entry

`livecontext/memory_index.json` runs `memory --parent-dir ${AGENT_STATE_DIR} index` at the start of each input, so the agent sees the top of its memory without asking. It is optional: the index can instead be a tool (`tools/memory_index.json`), or nowhere, leaving the agent to use `read`.
