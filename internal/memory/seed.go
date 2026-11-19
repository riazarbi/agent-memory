package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// seedFirstLevel is the default protocol's starting ontology (D19, D29):
// the first-level nodes, seeded with a description and no blocks.
var seedFirstLevel = []struct{ stem, description string }{
	{"user", "Who the operator is and how they want work done"},
	{"projects", "Ongoing efforts, goals, incidents and commitments"},
	{"people", "Who is who, and how to deal with them"},
	{"topics", "Everything else worth knowing"},
}

// SeedDefaults seeds memoryDir (memory/) with notes/ holding the default
// first level, and nothing else, on the directory-level contract skills/
// and livecontext/ use: only when memoryDir does not exist. Once it exists
// — even empty, even a symlink — it is never touched, so a deleted
// first-level node stays deleted and a shipped memory/ is never
// overwritten. The guidance is not seeded: it ships as the prompt/
// fragment agent/prompt/memory_usage.md, installed by the operator
// (docs/spec/agent-memory.md Part III.4), and the protocol bundle (D24)
// has no seeded files for now.
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

// seedTree writes notes/ and the default first level into root, which
// becomes memory/.
func seedTree(root string) error {
	if err := os.Chmod(root, folderMode); err != nil {
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
