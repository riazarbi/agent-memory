//go:build unix

package memory

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// lockPoll is how often a waiting write retries the lock. flock has no
// timeout, so a bounded wait polls a non-blocking attempt.
const lockPoll = 5 * time.Millisecond

// lockFile takes an exclusive flock on the lock file at path (created if
// missing), waiting until deadline, and returns its release. The OS drops
// the lock if the process dies.
func lockFile(path string, deadline time.Time) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, noteMode) //nolint:gosec // the lock file in notes/; D28 fixes its name
	if err != nil {
		return nil, fmt.Errorf("opening the memory write lock: %w", err)
	}
	fd := int(f.Fd()) //nolint:gosec // a file descriptor fits in an int
	for {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(fd, syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, fmt.Errorf("taking the memory write lock: %w", err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, ErrBusy
		}
		time.Sleep(lockPoll)
	}
}
