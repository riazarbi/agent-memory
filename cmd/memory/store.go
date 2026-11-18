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
	stateDirKey     = "AGENT_STATE_DIR"
	defaultStateDir = ".agent"
	dotenvFile      = ".env"
)

// errNoNotes is Part III.3's corrective error for a store without notes/.
var errNoNotes = errors.New("memory has no notes/ folder; run memory init")

// memoryDir is the store, <state dir>/memory (Part III.1). The state dir is
// AGENT_STATE_DIR from the environment if set and non-empty; otherwise
// AGENT_STATE_DIR from ./.env if non-empty; otherwise .agent. A relative one
// stays relative, so it is taken against the working directory.
func memoryDir() (string, error) {
	dir := os.Getenv(stateDirKey)
	if dir == "" {
		data, err := os.ReadFile(dotenvFile)
		switch {
		case err == nil:
			dir, _ = dotenvValue(string(data), stateDirKey)
		case !errors.Is(err, fs.ErrNotExist):
			// Refusing beats guessing: falling back to .agent could land on
			// a store the agent does not use.
			return "", fmt.Errorf("reading %s: %w", dotenvFile, err)
		}
	}
	if dir == "" {
		dir = defaultStateDir
	}
	return filepath.Join(dir, "memory"), nil
}

// notesDir is the store's notes/, which must exist as a directory: the
// read verbs never create it.
func notesDir() (string, error) {
	dir, err := memoryDir()
	if err != nil {
		return "", err
	}
	notes := filepath.Join(dir, "notes")
	info, err := os.Stat(notes)
	switch {
	case errors.Is(err, fs.ErrNotExist), err == nil && !info.IsDir():
		return "", errNoNotes
	case err != nil:
		return "", fmt.Errorf("reading notes/: %w", err)
	}
	return notes, nil
}

// dotenvValue returns key's value in a .env file's text, parsed exactly as
// the agent parses .env (Part III.1): each line trimmed; blank lines and
// lines starting with # skipped; a leading "export " (with a space)
// removed; the line split at the first =, the key trimmed. The first
// occurrence of the key wins. found reports whether it is assigned at all.
func dotenvValue(data, key string) (value string, found bool) {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return dotenvParseValue(v), true
		}
	}
	return "", false
}

// dotenvParseValue: the value is left-trimmed of spaces and tabs. If it then
// starts with a quote that appears again, it is the text between them.
// Otherwise (an unterminated quote included) it is cut at the first " #" or
// "\t#", then trimmed.
func dotenvParseValue(v string) string {
	v = strings.TrimLeft(v, " \t")
	if v != "" && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			return v[1 : end+1]
		}
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, "\t#"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}
