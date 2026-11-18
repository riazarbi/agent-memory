package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Acceptance criterion 1: the store is <state dir>/memory, the state dir
// found as the agent finds it — environment, then ./.env, then .agent —
// checked by running init and seeing where memory/ appears.
func TestStoreResolution(t *testing.T) {
	const dotenv = "# agent settings\nAGENT_MODEL=some-model # trailing comment\nAGENT_STATE_DIR=\"from-dotenv\"\n"
	abs := t.TempDir()
	cases := []struct {
		name   string
		env    map[string]string
		dotenv string
		want   func(dir string) string // the memory/ that must appear
		absent []string                // relative to dir: must not appear
	}{
		{
			name:   "environment, absolute",
			env:    map[string]string{"AGENT_STATE_DIR": filepath.Join(abs, "env-abs")},
			want:   func(string) string { return filepath.Join(abs, "env-abs", "memory") },
			absent: []string{".agent"},
		},
		{
			name:   ".env, quoted, among comments",
			dotenv: dotenv,
			want:   func(dir string) string { return filepath.Join(dir, "from-dotenv", "memory") },
			absent: []string{".agent"},
		},
		{
			name:   ".env, export prefix, single quotes, inline comment",
			dotenv: "export AGENT_STATE_DIR='quoted state' # where the agent lives\n",
			want:   func(dir string) string { return filepath.Join(dir, "quoted state", "memory") },
			absent: []string{".agent"},
		},
		{
			name: "neither: .agent",
			want: func(dir string) string { return filepath.Join(dir, ".agent", "memory") },
		},
		{
			name:   ".env without the key: .agent",
			dotenv: "AGENT_MODEL=x\n# AGENT_STATE_DIR=commented-out\n",
			want:   func(dir string) string { return filepath.Join(dir, ".agent", "memory") },
			absent: []string{"commented-out"},
		},
		{
			name:   "both: the environment wins",
			env:    map[string]string{"AGENT_STATE_DIR": filepath.Join(abs, "env-wins")},
			dotenv: dotenv,
			want:   func(string) string { return filepath.Join(abs, "env-wins", "memory") },
			absent: []string{"from-dotenv", ".agent"},
		},
		{
			name:   "an empty environment value is unset",
			env:    map[string]string{"AGENT_STATE_DIR": ""},
			dotenv: dotenv,
			want:   func(dir string) string { return filepath.Join(dir, "from-dotenv", "memory") },
			absent: []string{".agent"},
		},
		{
			name:   "relative environment value: against the working directory",
			env:    map[string]string{"AGENT_STATE_DIR": "rel/state"},
			want:   func(dir string) string { return filepath.Join(dir, "rel", "state", "memory") },
			absent: []string{".agent"},
		},
		{
			name:   "relative .env value: against the working directory",
			dotenv: "AGENT_STATE_DIR=nested/dir\n",
			want:   func(dir string) string { return filepath.Join(dir, "nested", "dir", "memory") },
			absent: []string{".agent"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.dotenv != "" {
				writeFile(t, filepath.Join(dir, ".env"), tc.dotenv)
			}
			r := runBin(t, dir, tc.env, "init")
			mustSucceed(t, r)
			want := tc.want(dir)
			for _, sub := range []string{"notes/user/user.md", "protocol/usage.md"} {
				if !exists(filepath.Join(want, filepath.FromSlash(sub))) {
					t.Errorf("%s missing under %s", sub, want)
				}
			}
			for _, a := range tc.absent {
				if exists(filepath.Join(dir, a)) {
					t.Errorf("%s appeared in the working directory", a)
				}
			}
			var out xInit
			decode(t, r.stdout, &out)
			if got, _ := filepath.EvalSymlinks(out.Store); got != mustEval(t, want) {
				t.Errorf("init reported store %q, want %q", out.Store, want)
			}
		})
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

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
