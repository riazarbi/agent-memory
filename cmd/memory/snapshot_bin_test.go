package main

// Snapshot helpers: what a call changed under a directory.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fileState struct {
	data  []byte
	mtime time.Time
	mode  fs.FileMode
}

// snapshot records every entry under root: bytes, mtime and mode.
func snapshot(t *testing.T, root string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		st := fileState{mtime: info.ModTime(), mode: info.Mode()}
		if d.Type().IsRegular() {
			if st.data, err = os.ReadFile(p); err != nil { //nolint:gosec // a test temp dir
				return err
			}
		}
		out[p] = st
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ageTree sets every mtime under root into the past, so any rewrite shows.
func ageTree(t *testing.T, root string) {
	t.Helper()
	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, past, past) //nolint:gosec // a test temp dir
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sameSnapshot(t *testing.T, before, after map[string]fileState) {
	t.Helper()
	for p, b := range before {
		a, ok := after[p]
		switch {
		case !ok:
			t.Errorf("%s disappeared", p)
		case !bytes.Equal(a.data, b.data):
			t.Errorf("%s changed content", p)
		case !a.mtime.Equal(b.mtime):
			t.Errorf("%s mtime changed %v -> %v", p, b.mtime, a.mtime)
		case a.mode != b.mode:
			t.Errorf("%s mode changed %v -> %v", p, b.mode, a.mode)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s appeared", p)
		}
	}
}
