//go:build unix

package memory

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The cross-process tests (U6 criterion 9) re-execute this test binary as
// a small write driver: TestWriteHelperProcess, selected with -test.run
// and switched on by environment variables. Each driver runs in notes/,
// opens its own writer on it and makes one write.
const (
	envHelper = "MEMORY_WRITE_HELPER" // "append" or "hold"
	envText   = "MEMORY_WRITE_TEXT"
	envWait   = "MEMORY_WRITE_WAIT" // the lock wait, a time.Duration
)

// helperResult is a driver's one line of output.
type helperResult struct {
	IDs     []string `json:"ids"`
	Err     string   `json:"err"`
	Busy    bool     `json:"busy"`
	Changed bool     `json:"changed"`
}

// TestWriteHelperProcess is the driver, not a test: it does nothing unless
// started by the tests below.
func TestWriteHelperProcess(t *testing.T) {
	mode := os.Getenv(envHelper)
	if mode == "" {
		t.Skip("write driver for the cross-process tests")
	}
	os.Exit(runWriteHelper(mode))
}

func runWriteHelper(mode string) int {
	if d, err := time.ParseDuration(os.Getenv(envWait)); err == nil {
		lockWait = d
	}
	// Every driver waits on stdin before writing, so the test can start
	// them all and release them at once. A "hold" driver waits again after
	// it has the lock: it holds it until stdin closes or it is killed.
	release := func() { _, _ = io.Copy(io.Discard, os.Stdin) }
	if mode == "hold" {
		testHookLocked = func() {
			_, _ = os.Stdout.WriteString("locked\n")
			release()
		}
	} else {
		release()
	}
	w, err := WriterFor(".") // the driver runs in notes/
	var res WriteResult
	if err == nil {
		res, err = w.Write(appendTo("user", os.Getenv(envText)))
	}
	out := helperResult{IDs: res.BlockIDs}
	if err != nil {
		out = helperResult{Err: err.Error(), Busy: errors.Is(err, ErrBusy), Changed: errors.Is(err, ErrChangedOnDisk)}
	}
	b, _ := json.Marshal(out)
	_, _ = os.Stdout.Write(append(b, '\n'))
	return 0
}

// driver is one started write driver.
type driver struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   *bufio.Reader
}

// startDriver starts a write driver; it writes nothing until released.
func startDriver(t *testing.T, mode, dir, text string, wait time.Duration) *driver {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWriteHelperProcess$") //nolint:gosec // re-executes this test binary
	cmd.Env = append(os.Environ(), envHelper+"="+mode, envText+"="+text, envWait+"="+wait.String())
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	d := &driver{cmd: cmd, stdin: stdin, out: bufio.NewReader(stdout)}
	t.Cleanup(func() {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	})
	return d
}

func (d *driver) release() { _ = d.stdin.Close() }

// line reads the driver's next output line, failing after limit.
func (d *driver) line(t *testing.T, limit time.Duration) string {
	t.Helper()
	return within(t, limit, func() string {
		s, err := d.out.ReadString('\n')
		if err != nil {
			return "read error: " + err.Error()
		}
		return strings.TrimSpace(s)
	})
}

// result reads the driver's result line.
func (d *driver) result(t *testing.T) helperResult {
	t.Helper()
	line := d.line(t, 60*time.Second)
	var r helperResult
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("driver output %q: %v", line, err)
	}
	return r
}

// U6 criterion 9: 20 separate processes, each with its own writer on the
// same notes/, append one block to user at once, over 5 rounds. Every
// write reports success, and every reported block is in user.md.
func TestWrite_ProcessesSerialize(t *testing.T) {
	dir := writerTree(t, nil)
	const processes, rounds = 20, 5
	want := map[string]string{"m-u6aaa1": "Prefers short answers."}
	for round := range rounds {
		drivers := make([]*driver, processes)
		for i := range drivers {
			drivers[i] = startDriver(t, "append", dir, fmt.Sprintf("Round %d process %d.", round, i), 10*time.Second)
		}
		for _, d := range drivers {
			d.release()
		}
		for i, d := range drivers {
			r := d.result(t)
			if r.Err != "" || len(r.IDs) != 1 {
				t.Fatalf("round %d process %d: %+v", round, i, r)
			}
			if _, dup := want[r.IDs[0]]; dup {
				t.Fatalf("round %d process %d: ID %s reported twice", round, i, r.IDs[0])
			}
			want[r.IDs[0]] = fmt.Sprintf("Round %d process %d.", round, i)
		}
		n := mustNode(t, loadClean(t, dir), "user")
		if len(n.Blocks) != len(want) || !reflect.DeepEqual(blockTexts(n), want) {
			t.Fatalf("round %d: user.md has %d blocks, want %d; missing or wrong: %v", round, len(n.Blocks), len(want), diffTexts(blockTexts(n), want))
		}
	}
}

func diffTexts(got, want map[string]string) []string {
	var out []string
	for id, text := range want {
		if got[id] != text {
			out = append(out, id+"="+text)
		}
	}
	return out
}

// startHolder starts a driver that takes the lock and holds it, and waits
// until it has it.
func startHolder(t *testing.T, dir string) *driver {
	t.Helper()
	// Hold mode writes at once, and holds the lock until released.
	h := startDriver(t, "hold", dir, "Held.", 10*time.Second)
	if got := h.line(t, 30*time.Second); got != "locked" {
		t.Fatalf("holder said %q, want locked", got)
	}
	return h
}

// U6 criterion 9: while another process holds the lock past the wait, a
// write is refused as busy and nothing changes.
func TestWrite_BusyAcrossProcesses(t *testing.T) {
	dir := writerTree(t, nil)
	before := treeState(t, dir)
	h := startHolder(t, dir)
	d := startDriver(t, "append", dir, "Refused.", 300*time.Millisecond)
	d.release()
	r := d.result(t)
	if !r.Busy || r.Changed || len(r.IDs) != 0 || !strings.Contains(r.Err, "busy") {
		t.Fatalf("write while locked: %+v, want the busy error", r)
	}
	wantTree(t, dir, before)
	// The holder was mid-write: released, it completes.
	h.release()
	if hr := h.result(t); hr.Err != "" {
		t.Fatalf("holder: %+v", hr)
	}
}

// U6 criterion 9: a process killed while holding the lock does not block
// the next write.
func TestWrite_KilledHolderReleasesLock(t *testing.T) {
	dir := writerTree(t, nil)
	h := startHolder(t, dir)
	// Control: the lock really is held.
	withHook(t, &lockWait, 200*time.Millisecond)
	if _, err := writerFor(t, dir).Write(appendTo("user", "Busy.")); !errors.Is(err, ErrBusy) {
		t.Fatalf("write while held: %v, want ErrBusy", err)
	}
	if err := h.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = h.cmd.Wait()
	withHook(t, &lockWait, 10*time.Second)
	res, err := within(t, 5*time.Second, func() writeOutcome {
		r, err := writerFor(t, dir).Write(appendTo("user", "After the kill."))
		return writeOutcome{r, err}
	}).split()
	if err != nil {
		t.Fatalf("write after the holder died: %v", err)
	}
	if blockTexts(mustNode(t, loadClean(t, dir), "user"))[res.BlockIDs[0]] != "After the kill." {
		t.Fatal("block not on disk")
	}
}

type writeOutcome struct {
	res WriteResult
	err error
}

func (o writeOutcome) split() (WriteResult, error) { return o.res, o.err }

// In-process: the wait is bounded both ways - a lock released within the
// wait is acquired, one held past it is refused as busy.
func TestWrite_LockWaitIsBounded(t *testing.T) {
	if lockWait != 10*time.Second {
		t.Fatalf("lock wait %v, want D28's 10 s", lockWait)
	}
	dir := writerTree(t, nil)
	f, err := os.OpenFile(dir+"/"+lockName, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	fd := int(f.Fd()) //nolint:gosec // a file descriptor fits in an int
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	withHook(t, &lockWait, 300*time.Millisecond)
	before := treeState(t, dir)
	start := time.Now()
	_, err = writerFor(t, dir).Write(appendTo("user", "Busy."))
	if !errors.Is(err, ErrBusy) || errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if waited := time.Since(start); waited < 300*time.Millisecond {
		t.Errorf("refused after %v, before the wait was up", waited)
	}
	wantTree(t, dir, before)

	withHook(t, &lockWait, 5*time.Second)
	time.AfterFunc(200*time.Millisecond, func() { _ = syscall.Flock(fd, syscall.LOCK_UN) })
	if _, err := writerFor(t, dir).Write(appendTo("user", "Waited.")); err != nil {
		t.Fatalf("write after the lock was released within the wait: %v", err)
	}
}

// lockHeld reports whether another open file description holds the lock
// on notes/: a non-blocking flock of our own fails.
func lockHeld(t *testing.T, dir string) bool {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, lockName)) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	fd := int(f.Fd()) //nolint:gosec // a file descriptor fits in an int
	err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		return false
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatal(err)
	}
	return true
}

// U6 criterion 9: the OS lock is held across the read-back - after the
// rename, and while the store is loaded again - so no other process can
// write between this write's rename and its check.
func TestWrite_LockHeldThroughReadBack(t *testing.T) {
	dir := writerTree(t, nil)
	var afterRename, atReadBack bool
	withHook(t, &testHookAfterRename, func(string) { afterRename = lockHeld(t, dir) })
	withHook(t, &testHookReadBack, func(*Store, string) { atReadBack = lockHeld(t, dir) })
	if _, err := writerFor(t, dir).Write(appendTo("user", "Locked throughout.")); err != nil {
		t.Fatal(err)
	}
	if !afterRename || !atReadBack {
		t.Fatalf("lock held after the rename: %v; at the read-back: %v", afterRename, atReadBack)
	}
	if lockHeld(t, dir) {
		t.Fatal("lock still held after the write returned")
	}
}
