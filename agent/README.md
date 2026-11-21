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
| `tools/memory_move.json` | `<state dir>/tools/` | Shell tool running `memory move`: putting a node, with everything under it, somewhere else in the tree. Optional, like every restructuring tool. |
| `tools/memory_rename.json` | `<state dir>/tools/` | Shell tool running `memory rename`: changing a node's stem and rewriting every link to it. |
| `tools/memory_move_block.json` | `<state dir>/tools/` | Shell tool running `memory move-block`: moving one block to another node. |
| `tools/memory_split.json` | `<state dir>/tools/` | Shell tool running `memory split`: moving some of a node's blocks into a new node. |
| `tools/memory_merge.json` | `<state dir>/tools/` | Shell tool running `memory merge`: folding one node into another. |
| `tools/memory_delete.json` | `<state dir>/tools/` | Shell tool running `memory delete`: deleting a block, a node, or with `--recursive` a node and everything under it. |
| `tools/memory_retag.json` | `<state dir>/tools/` | Shell tool running `memory retag`: renaming or removing a tag across the store. |
| `prompt/memory_purpose.md` | `<state dir>/prompt/` | The purpose text: what memory is and what it is for, and nothing else. A `prompt/` fragment, loaded every turn. |
| `skills/memory-usage/` | `<state dir>/skills/` | A skill (the folder holds `SKILL.md`): how to consult memory before answering. |
| `skills/memory-capture/` | `<state dir>/skills/` | A skill: what to save and how to write it. |
| `skills/memory-restructure/` | `<state dir>/skills/` | A skill: how to reorganise memory with the restructuring tools. |
| `livecontext/memory_index.json` | `<state dir>/livecontext/` | A live-context entry showing the index at the start of each input. |
| `memory/notes/` | `<state dir>/memory/notes/` | An optional starting tree: the top-level nodes `user`, `projects`, `people` and `topics`. Copied only into a store that does not exist yet. |
| `README.md` | nowhere | This file. |

## Install

From this folder (`agent/` in a checkout, or in an unpacked release archive), copy the files:

```bash
mkdir -p <state dir>/tools <state dir>/prompt <state dir>/skills <state dir>/livecontext
cp tools/memory_*.json <state dir>/tools/
cp prompt/memory_purpose.md <state dir>/prompt/
rm -f <state dir>/prompt/memory_usage.md
cp -R skills/memory-usage skills/memory-capture skills/memory-restructure <state dir>/skills/
cp livecontext/memory_index.json <state dir>/livecontext/
[ -e <state dir>/memory ] || cp -R memory <state dir>/
```

That installs everything: all twelve tool files, the restructuring ones included, the purpose text, the three skills, the live-context entry and the starting tree. The `rm -f` line is for an install made before the skills existed: delete its `<state dir>/prompt/memory_usage.md`, the file `memory_purpose.md` replaces, or the model reads both on every turn (see "The purpose text and the skills"). The last line copies the starting tree only when `<state dir>/memory` does not exist, so it never touches an existing store; without it the store starts empty, and the agent's first `memory` call of any kind creates it, empty (`memory` itself seeds nothing). That first call is usually the live-context index at the start of the agent's first input; after it, `<state dir>/memory` exists and the last line copies nothing, so copy the starting tree before the agent first runs. Copy less to grant less; the sections below say what each choice means.

## The tool files

Installing a tool file is the grant: leave one out and the agent lacks that capability. Without `memory_write.json` the agent's memory is read-only. If the index is in the live-context entry, `memory_index.json` is optional. Each description tells the model how to call its verb; every description except `memory_lint.json`'s (the read, write, search and index tools and the seven restructuring tools) also says that exit 75 means "run the same command again" (`lint` cannot be busy, so `memory_lint.json`'s does not).

Each file's `prepend_args` are `["--parent-dir", "${AGENT_STATE_DIR}", "<verb>"]` (the verb is `move-block`, with a hyphen, in `memory_move_block.json`), and `memory_lint.json`'s are `["--parent-dir", "${AGENT_STATE_DIR}", "lint", "--exit-zero"]`. The model's arguments come after the verb, where `--parent-dir` is not an option, so the model cannot move the store.

`memory_lint.json` is optional, like every tool file. It lets the agent check its own store, for example after you have edited notes by hand (an editor or Obsidian, a git merge, a sync conflict): a note the reader cannot load silently disappears from `read`, `search` and the index, and `lint` lists it. Its description tells the model that an error is for you to fix (its memory tools cannot) and which warnings it can fix itself. It runs `lint --exit-zero` because the agent's shell tools show the model only `exit status 2`, not the report, when a command exits non-zero; with `--exit-zero` the model always gets the report. It takes no model arguments. A cron job or git hook should run plain `memory lint` and use its exit code instead (see the repo's README).

## The restructuring tools

`write` only adds: it creates nodes and adds or replaces blocks. The seven restructuring tools remove and relocate, one verb each: `memory_move.json` (move a node), `memory_rename.json` (rename a node), `memory_move_block.json` (move one block), `memory_split.json` (divide a node), `memory_merge.json` (fold one node into another), `memory_delete.json` (delete a block or a node) and `memory_retag.json` (rename or remove a tag). None is read-only. They can reshape any node but the root, top-level nodes included, and a delete cannot be undone.

As with every tool file, installing a file is the grant, and a tool is withheld by leaving its file out: each of the seven is granted or withheld on its own. To let an agent rearrange its memory but never delete from it, install the others and leave `memory_delete.json` out. An agent with none of them can add to its memory, top-level nodes included, but cannot remove, move or rename anything; you then prune by hand. The install commands above copy all seven, so delete from `<state dir>/tools/` the ones you do not want to grant.

To grant `delete` without its `--recursive` flag (the agent can then delete a block, or a node that has no children, but never a whole subtree in one call), edit the installed `<state dir>/tools/memory_delete.json`:

```json
"forbidden_args": ["--recursive", "-recursive"]
```

Both spellings are listed because the agent refuses a forbidden argument by exact match, and `memory` accepts a flag written with one dash or two: with only `--recursive` forbidden, `-recursive` would still get through. `--recursive=true` needs no entry, because `memory` rejects a value on that flag (`--recursive: takes no value`) and deletes nothing.

The `memory-restructure` skill tells the model when to use which of these tools. It refers to them by what they do and tells the model to skip a step whose tool is missing, so it needs no edit when you install only some.

## `${AGENT_STATE_DIR}` needs a recent agent

The tool files and the live-context entry need an agent that replaces `${AGENT_STATE_DIR}` in a tool's `prepend_args` and an entry's `args` with its absolute state dir (agent CHANGELOG 2026-11-19). An older agent passes the text literally, and every call then fails with `parent dir ${AGENT_STATE_DIR} does not exist (from --parent-dir)`, and creates nothing; the live-context entry shows that error at the start of every input.

## Pointing an agent at another store

The agent's store is `<state dir>/memory`, named explicitly with `--parent-dir`, so whatever the operator's shell exports (`MEMORY_PARENT_DIR`, `HOME`) cannot redirect it. To point an agent at another store (one shared by several agents, say), edit the path: replace `${AGENT_STATE_DIR}` with the store's parent dir (which must exist) in every tool file's `prepend_args` and in the live-context entry's `args`. Never delete the `--parent-dir` pair: the agent would then use `MEMORY_PARENT_DIR` or `$HOME/memory`, which may be a person's own store.

A store outside the agent's state dir is not covered by the agent's state-dir write deny, so the agent's file tools can edit its notes directly, bypassing `write`'s checks and lock.

## The purpose text and the skills

`prompt/memory_purpose.md` says what memory is and what it is for, and nothing else. It is a `prompt/` fragment, so the agent loads it on every turn, whether or not the memory tools are installed. It tells the model that what it can do with memory is exactly what its memory tools allow, so it fits any set of tools; leave it out if you install none.

Until the skills existed this file was `prompt/memory_usage.md`, which also held the how-to that is now in the skills. An install that has it: delete `<state dir>/prompt/memory_usage.md` when you copy in `memory_purpose.md` (the install commands above do), or the model reads both on every turn.

How to use memory ships as three skills, each a folder holding `SKILL.md`, copied as a folder into `<state dir>/skills/`:

- `skills/memory-usage/`: how to consult memory before answering a question or starting a task.
- `skills/memory-capture/`: what is worth saving, and how to write it so that it is still useful later.
- `skills/memory-restructure/`: when and how to split, merge, move, rename, retag and delete.

The agent lists each skill's name and description on every turn, and the body is read only when the model loads the skill. So the advice on what to save and how to consult memory is no longer in front of the model on every turn: each skill's description is what prompts the model to load it. The skills need an agent that loads skills from `<state dir>/skills/`. They should match the tools you installed: leave out `memory-capture` if you leave out `memory_write.json`, and `memory-restructure` if you install none of the restructuring tools. A skill that tells the model to save, with no tool to save with, sends it hunting for a way to write.

## The starting tree

`memory/notes/` is an example, not a requirement: `memory` imposes no structure, and the store's structure is whatever its tree is. It holds four top-level nodes, each a description and no blocks, as `memory write <stem> --parent / --description <D>` would create them, without the `modified` date a create writes: `user` (who the operator is and how they want work done), `projects` (ongoing efforts, goals, incidents and commitments), `people` (who is who, and how to deal with them) and `topics` (everything else worth knowing). Install it, edit it, make your own, or leave it out.

To rearrange it, before or after installing, move folders around: a node is a folder holding `<folder>/<folder>.md`, whose frontmatter has a one-line `description`, so renaming, moving or deleting folders (in a file manager or Obsidian) reshapes the tree. Nothing the model reads names these nodes: the tool descriptions, the purpose text and the skills send it to the index (or a root read) to learn the structure, so a different tree needs no other edit.

The model can add top-level nodes (`write` with `--parent /`). Whether it can also move, rename, merge, split or delete them is up to you: an agent given the restructuring tools can reshape any node but the root, and an agent without them can add to the top level but not take away from it, so a top level that sprawls is then yours to tidy, by moving folders or with `memory move`, `memory merge` and `memory delete` from a shell.

## The live-context entry

`livecontext/memory_index.json` runs `memory --parent-dir ${AGENT_STATE_DIR} index` at the start of each input, so the agent sees the top of its memory without asking. It is optional: the index can instead be a tool (`tools/memory_index.json`), or nowhere, leaving the agent to use `read`.
