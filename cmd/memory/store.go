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

// storeDirMode is the mode write gives memory/ and notes/ when it creates
// them, whatever the umask (Part III.1).
const storeDirMode fs.FileMode = 0o755

// paths resolves the store (Part III.1): the parent and its source, which
// must already exist as a directory (symlinks followed), then memory/ and
// memory/notes inside it. memory creates memory/ inside the parent, never
// the parent itself, so a mistyped parent fails loudly instead of starting
// a store somewhere unexpected.
func (l location) paths() (memoryDir, notes string, err error) {
	parent, source, err := l.parent()
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(parent) //nolint:gosec // the parent the caller named (III.1); reaching it is this command's job
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", "", fmt.Errorf("parent dir %s does not exist (from %s)", parent, source)
	case err != nil:
		return "", "", fmt.Errorf("parent dir %s (from %s): %w", parent, source, err)
	case !info.IsDir():
		return "", "", fmt.Errorf("parent dir %s is not a directory (from %s)", parent, source)
	}
	memoryDir = filepath.Join(parent, "memory")
	return memoryDir, filepath.Join(memoryDir, "notes"), nil
}

// notesDir is the store's notes/ for the read verbs, which create nothing
// (Part III.1): a missing memory/ or notes/ is returned as it is, and loads
// as an empty store. memory/ or notes existing as anything but a directory
// is an error naming that path.
func (l location) notesDir() (string, error) {
	memoryDir, notes, err := l.paths()
	if err != nil {
		return "", err
	}
	for _, dir := range []string{memoryDir, notes} {
		missing, err := missingDir(dir)
		if err != nil || missing {
			return notes, err
		}
	}
	return notes, nil
}

// makeNotesDir is the store's notes/ for write, which creates memory/ and
// memory/notes as needed, mode 0755 forced, and nothing else (Part III.1).
// write calls it after its argument checks and before its lock, so an
// argument error creates nothing. Another write creating the same
// directory first is not an error.
func (l location) makeNotesDir() (string, error) {
	memoryDir, notes, err := l.paths()
	if err != nil {
		return "", err
	}
	for _, dir := range []string{memoryDir, notes} {
		if err := makeStoreDir(dir); err != nil {
			return "", err
		}
	}
	return notes, nil
}

// makeStoreDir creates dir with storeDirMode if it is missing. Whatever is
// there already, or appears there first, must be a directory.
func makeStoreDir(dir string) error {
	missing, err := missingDir(dir)
	if err != nil || !missing {
		return err
	}
	if err := os.Mkdir(dir, storeDirMode); err != nil { //nolint:gosec // inside the store the caller named (III.1)
		if errors.Is(err, fs.ErrExist) {
			// Another write made it in between: judge what it made.
			_, err := missingDir(dir)
			return err
		}
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	// Forced, whatever the umask.
	if err := os.Chmod(dir, storeDirMode); err != nil { //nolint:gosec // inside the store the caller named (III.1)
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	return nil
}

// missingDir reports whether dir does not exist, judged without following
// a symlink. Anything at dir that is not a directory, or a symlink that
// does not lead to one (a dangling one included), is an error naming it.
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
