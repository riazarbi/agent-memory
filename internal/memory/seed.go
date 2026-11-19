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
// first level, and nothing else, when memoryDir/notes does not exist
// (docs/spec/agent-memory.md Part III.1). The trigger is notes/, judged
// without following a symlink: once notes/ exists — even empty, even
// missing a first-level node — it is never touched, so a deleted
// first-level node stays deleted, and deleting notes/ resets the store.
// memoryDir is created if it is missing, but never its parent: a missing
// parent is an error and nothing is created. Entries already in memoryDir
// are left as they are. The guidance is not seeded: it ships as the
// prompt/ fragment agent/prompt/memory_usage.md, installed by the operator
// (Part III.4), and the protocol bundle (D24) has no seeded files for now.
//
// notes/ is built in a hidden temporary directory inside memoryDir
// (.notes-seed-*) and renamed into place, so a seed that fails part-way
// leaves no partial notes/ that would then never be completed. When the
// rename fails because another process's notes/ now exists, theirs stands
// and this is success.
func SeedDefaults(memoryDir string) error {
	notes := filepath.Join(memoryDir, "notes")
	if _, err := os.Lstat(notes); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", notes, err)
	}
	if err := os.Mkdir(memoryDir, folderMode); err == nil {
		// Forced, whatever the umask, as memory/ always was.
		if err := os.Chmod(memoryDir, folderMode); err != nil {
			return fmt.Errorf("create %s: %w", memoryDir, err)
		}
	} else if !errors.Is(err, fs.ErrExist) {
		// Never MkdirAll: a missing parent is the caller's mistake.
		return fmt.Errorf("create %s: %w", memoryDir, err)
	}
	tmp, err := os.MkdirTemp(memoryDir, ".notes-seed-*")
	if err != nil {
		return fmt.Errorf("seed %s: %w", notes, err)
	}
	if err := seedTree(tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("seed %s: %w", notes, err)
	}
	if err := os.Rename(tmp, notes); err != nil {
		_ = os.RemoveAll(tmp)
		if _, statErr := os.Lstat(notes); statErr == nil {
			return nil // another process seeded it first; theirs stands
		}
		return fmt.Errorf("seed %s: %w", notes, err)
	}
	return nil
}

// seedTree writes the default first level into notes, the temporary
// directory that becomes notes/.
func seedTree(notes string) error {
	// MkdirTemp made it 0700; notes/ gets the folder mode.
	if err := os.Chmod(notes, folderMode); err != nil {
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
