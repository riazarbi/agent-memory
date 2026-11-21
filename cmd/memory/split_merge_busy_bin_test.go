//go:build unix

package main

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Acceptance criterion 8: while another process's flock holds
// notes/.write.lock, split and merge each wait out D28's 10 s and then
// exit 75 with the "busy" line, having written nothing. The two calls wait
// side by side; once the lock is released the same calls succeed.
func TestSplitAndMergeBusyAreRetry(t *testing.T) {
	t.Parallel()
	f := newSMF(t)
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

	calls := [][]string{f.splitCall(), mergeCall()}
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
	noFolderNamed(t, f.notes, "atlas-cutover")

	// The control: the lock was all that stopped them.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil { //nolint:gosec // a file descriptor fits in an int
		t.Fatal(err)
	}
	for _, args := range calls {
		f.ok(t, args...)
	}
}
