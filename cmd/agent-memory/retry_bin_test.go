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

// Acceptance criterion 4, busy: while another process holds the D28 flock
// on notes/.write.lock, write exits 75 with "busy" on stderr and writes
// nothing. The writer waits its full 10 s first; the library's wait seam is
// unexported, so this test waits it out.
func TestWriteBusyIsRetry(t *testing.T) {
	t.Parallel()
	dir, notes := seeded(t)
	lock, err := os.OpenFile(filepath.Join(notes, ".write.lock"), os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // a test temp dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // a file descriptor fits in an int
		t.Fatal(err)
	}
	state := filepath.Join(dir, ".agent")
	ageTree(t, state)
	before := snapshot(t, state)

	start := time.Now()
	r := runBin(t, dir, nil, createArgs...)
	if r.code != exitRetry || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want exit 75 and empty stdout", r.code, r.stdout, r.stderr)
	}
	line := strings.TrimSuffix(r.stderr, "\n")
	if !strings.Contains(line, "busy") || strings.Contains(line, "\n") {
		t.Errorf("stderr %q, want one line saying busy", r.stderr)
	}
	if waited := time.Since(start); waited < 9*time.Second {
		t.Errorf("write gave up after %v; D28 waits 10 s", waited)
	}
	sameSnapshot(t, before, snapshot(t, state))
}

// Acceptance criterion 4, changed on disk: an editor that does not take the
// lock rewrites the folder note continuously, so a write sees it change
// between its load and its rename (or its read-back). Such a refusal exits
// 75, and a run never exits 1 for it.
func TestWriteChangedOnDiskIsRetry(t *testing.T) {
	dir, notes := seeded(t)
	note := writeNode(t, notes, "topics/flapping", "---\ndescription: A\n---\n")
	contents := []string{"---\ndescription: A\n---\n", "---\ndescription: B\n---\n\nAn edit. ^m-edit01\n"}

	defer flap(note, filepath.Join(dir, ".agent", "memory"), contents)()

	var retried int
	for range 30 {
		r := runBin(t, dir, nil, "write", "flapping", "--add", "A claim.")
		switch r.code {
		case exitOK:
		case exitRetry:
			if r.stdout != "" || !strings.Contains(r.stderr, "changed on disk") {
				t.Fatalf("exit 75 with stdout %q, stderr %q; want empty stdout and a changed-on-disk line", r.stdout, r.stderr)
			}
			retried++
		default:
			t.Fatalf("exit %d (stderr %q); a changed-on-disk refusal must exit 75", r.code, r.stderr)
		}
		if retried >= 3 {
			return
		}
	}
	if retried == 0 {
		t.Fatal("no write was refused as changed on disk while the note was being rewritten")
	}
}

// flap replaces note with each of contents in turn, atomically (so a
// reader never sees a torn file), staging in scratch, until the returned
// stop is called.
func flap(note, scratch string, contents []string) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tmp := filepath.Join(scratch, ".edit.tmp")
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			if os.WriteFile(tmp, []byte(contents[i%2]), 0o600) == nil {
				_ = os.Rename(tmp, note)
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}
