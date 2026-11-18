package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// UsageText is the default protocol's memory/protocol/usage.md, seeded
// verbatim (docs/spec/agent-memory.md §4.5): when and what to save. How to
// call the tools lives in their descriptions, never here.
const UsageText = `# Memory

You have a durable memory: notes that outlive this conversation, kept as a
tree under four areas — user, projects, people, topics. Its index is in your
system prompt. What you can do with it is exactly what your memory tools allow.

## What to save
Save a claim only if it would still matter if this conversation were cleared
today, and you could not work it out again from files or a live lookup:
- a correction or stated preference — and quiet confirmations that an unusual
  approach was right, not only corrections;
- who is doing what, why, and by when;
- facts about people and things you deal with that you would otherwise have
  to ask about again.
Nobody may be watching (mailbox, watch): save what you worked out, not only
what you were told. If you are unsure whether something is durable, it isn't.

Never save: what you can re-read or re-derive; progress on the current task;
records of finished work; secrets.

## How to write it
- One claim per block, written for a stranger: turn "today", "Thursday",
  "this one", "the one we discussed" into dates and names.
- For a rule, say why it holds and when it applies.
- Mark your own interpretation as yours, not as fact.
- Search before adding: update the existing note rather than starting a second.
- File it under the area it belongs to, or a node you can name — don't hunt
  for a deep location.

## Before you answer
For questions about the operator's preferences, people, or past work, check
the index and read the relevant node first. A note can be stale: re-check
anything it says about files or the outside world before acting on it.
`

// seedFirstLevel is the default protocol's starting ontology (D19, D29):
// the first-level nodes, seeded with a description and no blocks.
var seedFirstLevel = []struct{ stem, description string }{
	{"user", "Who the operator is and how they want work done"},
	{"projects", "Ongoing efforts, goals, incidents and commitments"},
	{"people", "Who is who, and how to deal with them"},
	{"topics", "Everything else worth knowing"},
}

// SeedDefaults seeds memoryDir (memory/) with protocol/usage.md and the
// default first level under notes/, on the directory-level contract
// skills/ and livecontext/ use: only when memoryDir does not exist. Once
// it exists — even empty, even a symlink — it is never touched, so a
// deleted usage.md or first-level node stays deleted and a shipped
// memory/ is never overwritten.
//
// The tree is built in a hidden sibling directory and renamed into place,
// so a seed that fails part-way leaves no half-seeded memory/ that would
// then never be completed.
func SeedDefaults(memoryDir string) error {
	if _, err := os.Lstat(memoryDir); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat memory dir %s: %w", memoryDir, err)
	}
	parent := filepath.Dir(memoryDir)
	if err := os.MkdirAll(parent, folderMode); err != nil {
		return fmt.Errorf("create %s: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, ".memory-seed-*")
	if err != nil {
		return fmt.Errorf("seed memory dir %s: %w", memoryDir, err)
	}
	if err := seedTree(tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("seed memory dir %s: %w", memoryDir, err)
	}
	if err := os.Rename(tmp, memoryDir); err != nil {
		_ = os.RemoveAll(tmp)
		if _, statErr := os.Lstat(memoryDir); statErr == nil {
			return nil // another process seeded it first; theirs stands
		}
		return fmt.Errorf("seed memory dir %s: %w", memoryDir, err)
	}
	return nil
}

// seedTree writes the default bundle into root, which becomes memory/.
func seedTree(root string) error {
	if err := os.Chmod(root, folderMode); err != nil {
		return err
	}
	protocol := filepath.Join(root, "protocol")
	if err := os.Mkdir(protocol, folderMode); err != nil {
		return err
	}
	if err := writeSeedFile(filepath.Join(protocol, "usage.md"), []byte(UsageText)); err != nil {
		return err
	}
	notes := filepath.Join(root, "notes")
	if err := os.Mkdir(notes, folderMode); err != nil {
		return err
	}
	for _, n := range seedFirstLevel {
		// Encoded like any new node, so a seeded node is exactly what
		// memory_write would have written (and loads clean, D27).
		enc, err := Encoder{}.EncodeNew(NewNode{Description: n.description})
		if err != nil {
			return fmt.Errorf("encoding %s: %w", n.stem, err)
		}
		folder := filepath.Join(notes, n.stem)
		if err := os.Mkdir(folder, folderMode); err != nil {
			return err
		}
		if err := writeSeedFile(filepath.Join(folder, n.stem+".md"), enc.Bytes); err != nil {
			return err
		}
	}
	return nil
}

func writeSeedFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, noteMode); err != nil {
		return err
	}
	return os.Chmod(path, noteMode)
}
