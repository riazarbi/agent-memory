package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	parentDirFlag = "--parent-dir"
	parentDirKey  = "MEMORY_PARENT_DIR"
	homeKey       = "HOME"
)

var (
	// errNoNotes is Part III.3's corrective error for a store without notes/.
	errNoNotes = errors.New("memory has no notes/ folder; run memory init")
	// errNoStore is Part III.1's error when no source names a parent.
	errNoStore = errors.New("no store: give " + parentDirFlag + ", or set " + parentDirKey + " or " + homeKey)
)

// location is where the store is looked for: parentDir is the
// --parent-dir given before the verb, or "" when none was.
type location struct {
	parentDir string
}

// parseLocation takes the options before the verb (Part III.1): only
// --parent-dir DIR or --parent-dir=DIR, at most once, never empty. It stops
// at the first other argument, the verb, and returns the rest from there.
func parseLocation(args []string) (location, []string, error) {
	var loc location
	for len(args) > 0 {
		name, value, hasValue := strings.Cut(args[0], "=")
		if name != parentDirFlag {
			break
		}
		if loc.parentDir != "" {
			return loc, nil, fmt.Errorf("%s: given twice; give it once, before the verb", parentDirFlag)
		}
		args = args[1:]
		if !hasValue {
			if len(args) == 0 {
				return loc, nil, fmt.Errorf("%s: needs a directory", parentDirFlag)
			}
			value, args = args[0], args[1:]
		}
		if value == "" {
			return loc, nil, fmt.Errorf("%s is given but empty", parentDirFlag)
		}
		loc.parentDir = value
	}
	return loc, args, nil
}

// memoryDir is the store, <parent>/memory (Part III.1). The parent is
// --parent-dir, else MEMORY_PARENT_DIR, else HOME. A variable set but empty
// is an error, never a fall-through. A relative parent stays relative, so it
// is taken against the working directory.
func (l location) memoryDir() (string, error) {
	parent := l.parentDir
	if parent == "" {
		var err error
		if parent, err = parentFromEnv(); err != nil {
			return "", err
		}
	}
	return filepath.Join(parent, "memory"), nil
}

// parentFromEnv is the first of MEMORY_PARENT_DIR and HOME that is set.
func parentFromEnv() (string, error) {
	for _, key := range []string{parentDirKey, homeKey} {
		v, set := os.LookupEnv(key)
		switch {
		case !set:
			continue
		case v == "":
			return "", fmt.Errorf("%s is set but empty", key)
		default:
			return v, nil
		}
	}
	return "", errNoStore
}

// notesDir is the store's notes/, which must exist as a directory: the
// read verbs never create it.
func (l location) notesDir() (string, error) {
	dir, err := l.memoryDir()
	if err != nil {
		return "", err
	}
	notes := filepath.Join(dir, "notes")
	info, err := os.Stat(notes) //nolint:gosec // the store the caller named (III.1); reaching it is this command's job
	switch {
	case errors.Is(err, fs.ErrNotExist), err == nil && !info.IsDir():
		return "", errNoNotes
	case err != nil:
		return "", fmt.Errorf("reading notes/: %w", err)
	}
	return notes, nil
}
