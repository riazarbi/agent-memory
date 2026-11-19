package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/riazarbi/agent-memory/internal/memory"
)

const (
	parentDirFlag = "--parent-dir"
	parentDirKey  = "MEMORY_PARENT_DIR"
	homeKey       = "HOME"
)

// errNoStore is Part III.1's error when no source names a parent.
var errNoStore = errors.New("no store: give " + parentDirFlag + ", or set " + parentDirKey + " or " + homeKey)

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

// parent is the store's parent dir as given (Part III.1) and the source
// that named it: --parent-dir, else MEMORY_PARENT_DIR, else HOME. A variable
// set but empty is an error, never a fall-through. A relative parent stays
// relative, so it is taken against the working directory.
func (l location) parent() (dir, source string, err error) {
	if l.parentDir != "" {
		return l.parentDir, parentDirFlag, nil
	}
	return parentFromEnv()
}

// parentFromEnv is the first of MEMORY_PARENT_DIR and HOME that is set,
// and its name.
func parentFromEnv() (dir, source string, err error) {
	for _, key := range []string{parentDirKey, homeKey} {
		v, set := os.LookupEnv(key)
		switch {
		case !set:
			continue
		case v == "":
			return "", "", fmt.Errorf("%s is set but empty", key)
		default:
			return v, key, nil
		}
	}
	return "", "", errNoStore
}

// notesDir is the store's notes/, seeded first if it is missing (Part
// III.1): every verb calls it after checking its arguments, so the first
// call of any kind creates the store, and an argument error creates
// nothing. The parent must already exist as a directory (symlinks
// followed); memory creates memory/ inside it, never the parent itself, so
// a mistyped parent fails loudly instead of starting a store somewhere
// unexpected. memory/ or notes existing as anything but a directory is an
// error naming that path. Seeding prints nothing.
func (l location) notesDir() (string, error) {
	parent, source, err := l.parent()
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent) //nolint:gosec // the parent the caller named (III.1); reaching it is this command's job
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("parent dir %s does not exist (from %s)", parent, source)
	case err != nil:
		return "", fmt.Errorf("parent dir %s (from %s): %w", parent, source, err)
	case !info.IsDir():
		return "", fmt.Errorf("parent dir %s is not a directory (from %s)", parent, source)
	}
	memoryDir := filepath.Join(parent, "memory")
	notes := filepath.Join(memoryDir, "notes")
	for _, dir := range []string{memoryDir, notes} {
		missing, err := missingDir(dir)
		if err != nil {
			return "", err
		}
		if missing {
			return notes, memory.SeedDefaults(memoryDir)
		}
	}
	return notes, nil
}

// missingDir reports whether dir does not exist (judged without following
// a symlink, as SeedDefaults judges notes/). Anything at dir that is not a
// directory, or a symlink that does not lead to one, is an error naming
// it.
func missingDir(dir string) (bool, error) {
	if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) { //nolint:gosec // inside the store the caller named (III.1)
		return true, nil
	} else if err != nil {
		return false, fmt.Errorf("reading %s: %w", dir, err)
	}
	info, err := os.Stat(dir) //nolint:gosec // inside the store the caller named (III.1)
	if err == nil && info.IsDir() {
		return false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("reading %s: %w", dir, err)
	}
	return false, fmt.Errorf("%s is not a directory", dir)
}
