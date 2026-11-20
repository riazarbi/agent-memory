//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Acceptance criterion 8: while another process's flock holds
// notes/.write.lock, move-block waits out D28's 10 s and then exits 75
// with the "busy" line, having written nothing; once the lock is released
// the same call succeeds.
func TestMoveBlockBusyIsRetry(t *testing.T) {
	t.Parallel()
	f := newBlockF(t)
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

	args := []string{"move-block", f.p2, "--to", "atlas-cutover"}
	start := time.Now()
	r := runBin(t, f.dir, nil, args...)
	wantBusy(t, args, r, time.Since(start))
	sameSnapshot(t, before, snapshot(t, f.state))

	// The control: the lock was all that stopped it.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil { //nolint:gosec // a file descriptor fits in an int
		t.Fatal(err)
	}
	f.ok(t, args...)
}
