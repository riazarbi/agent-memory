package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// There is no --store flag (Part III.1).
func TestNoStoreFlag(t *testing.T) {
	dir := t.TempDir()
	for _, verb := range []string{"init", "read"} {
		line := mustFail(t, runBin(t, dir, nil, verb, "--store", "elsewhere"))
		if !strings.Contains(line, "--store") {
			t.Errorf("%s: stderr %q does not name --store", verb, line)
		}
	}
	if exists(filepath.Join(dir, "elsewhere")) || exists(filepath.Join(dir, ".agent")) {
		t.Error("a refused call created a store")
	}
}
