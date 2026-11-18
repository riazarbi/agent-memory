//go:build !unix

package memory

import (
	"errors"
	"time"
)

// lockFile has no flock to take on this platform (release targets are
// Linux and macOS), so every write is refused rather than run unlocked.
func lockFile(string, time.Time) (func(), error) {
	return nil, errors.New("memory writes need flock, which this platform does not have")
}
