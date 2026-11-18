// Command memory is an AI agent's long-term memory store, used through
// shell tools (docs/spec/agent-memory.md Part III). The store is
// <state dir>/memory, found as the agent finds its state dir. Results are
// XML on stdout with exit 0; an error is one line on stderr with exit 1, or
// exit 75 when retrying can succeed (busy, changed on disk). It never logs.
package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/riazarbi/agent-memory/internal/memory"

	// Embedded zone data, so TZ names resolve where the system has none
	// (a minimal container): dates follow the process's zone (Part III.2).
	_ "time/tzdata"
)

// Exit codes (Part III.3).
const (
	exitOK    = 0
	exitError = 1
	// exitRetry is EX_TEMPFAIL: the same call can succeed if made again.
	exitRetry = 75
)

const usage = `usage: memory <verb> [arguments]

verbs:
  init                                  seed the memory store if it is missing
  read [STEM] [--block ID] [--depth N]  read a node (depth defaults to 1), or
                                        with no STEM the first-level nodes and tags
  write STEM [--parent P] [--description D] [--tag T]... [--property K=V]...
        [--add TEXT]... [--replace ID=TEXT]...
                                        create or update a node; --add and
                                        --replace are its blocks, in order
  search QUERY                          search, in Obsidian search syntax (quote
                                        a query of several words)
  index                                 the index: first-level nodes, their
                                        children and the top tags

The store is <state dir>/memory. The state dir is AGENT_STATE_DIR from the
environment, else AGENT_STATE_DIR from ./.env, else .agent; a relative one is
taken against the working directory.
`

// verb runs one verb's arguments and returns its result for stdout.
type verb func(args []string) ([]byte, error)

func verbs() map[string]verb {
	return map[string]verb{
		"init":   initVerb,
		"read":   readVerb,
		"write":  writeVerb,
		"search": searchVerb,
		"index":  indexVerb,
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches args[0] as the verb. Nothing reaches stdout unless the
// verb succeeds.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return exitError
	}
	v, ok := verbs()[args[0]]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "unknown verb %q\n%s", args[0], usage)
		return exitError
	}
	out, err := v(args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, oneLine(err.Error()))
		if errors.Is(err, memory.ErrBusy) || errors.Is(err, memory.ErrChangedOnDisk) {
			return exitRetry
		}
		return exitError
	}
	_, _ = stdout.Write(out)
	return exitOK
}

// oneLine keeps an error to the one line Part III.3 promises: a line break
// inside it (from a path, say) becomes a space.
func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

// render marshals v with encoding/xml (D20), indented as D30's
// MarshalIndent, with a final newline.
func render(v any) ([]byte, error) {
	out, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
