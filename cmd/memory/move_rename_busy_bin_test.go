//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// wantBusy fails unless a call exited 75 with nothing on stdout and one
// stderr line saying busy, after waiting out D28's 10 s.
func wantBusy(t *testing.T, args []string, r result, waited time.Duration) {
	t.Helper()
	if r.code != exitRetry || r.stdout != "" {
		t.Errorf("%v: exit %d, stdout %q, stderr %q; want exit 75 and empty stdout", args, r.code, r.stdout, r.stderr)
	}
	line := strings.TrimSuffix(r.stderr, "\n")
	if !strings.Contains(line, "busy") || strings.Contains(line, "\n") {
		t.Errorf("%v: stderr %q, want one line saying busy", args, r.stderr)
	}
	if waited < 9*time.Second {
		t.Errorf("%v gave up after %v; D28 waits 10 s", args, waited)
	}
}

// Acceptance criterion 8: while another process's flock holds
// notes/.write.lock, move and rename each wait out D28's 10 s and then
// exit 75 with the "busy" line, having written nothing. The two calls wait
// side by side; once the lock is released the same calls succeed.
func TestMoveAndRenameBusyAreRetry(t *testing.T) {
	t.Parallel()
	f := newAtlasF(t)
	lock, err := os.OpenFile(filepath.Join(f.notes, ".write.lock"), os.O_RDWR, 0o644) //nolint:gosec // a test temp dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // a file descriptor fits in an int
		t.Fatal(err)
	}
	ageTree(t, f.state)
	before := snapshot(t, f.state)

	calls := [][]string{{"move", "atlas-migration", "--parent", "projects"}, {"rename", "atlas-migration", "atlas-go-live"}}
	results := make([]result, len(calls))
	waited := make([]time.Duration, len(calls))
	var wg sync.WaitGroup
	for i, args := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			results[i] = runBin(t, f.dir, nil, args...)
			waited[i] = time.Since(start)
		}()
	}
	wg.Wait()
	for i, args := range calls {
		wantBusy(t, args, results[i], waited[i])
	}
	sameSnapshot(t, before, snapshot(t, f.state))

	// The control: the lock was all that stopped them.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil { //nolint:gosec // a file descriptor fits in an int
		t.Fatal(err)
	}
	for _, args := range calls {
		f.ok(t, args...)
	}
}
