package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// errLintFound is lint's verdict that the store has error-severity
// problems: run prints the report the verb returned with it on stdout,
// writes nothing to stderr, and exits exitLintErrors (Part III.7).
var errLintFound = errors.New("lint found errors")

// lintXML is <memory_lint> (Part III.7): the loaded node count, the
// problem counts by severity, and one <problem> per Store.Problems()
// entry, in the library's order.
type lintXML struct {
	XMLName  xml.Name     `xml:"memory_lint"`
	Nodes    int          `xml:"nodes,attr"`
	Errors   int          `xml:"errors,attr"`
	Warnings int          `xml:"warnings,attr"`
	Problems []problemXML `xml:"problem"`
}

type problemXML struct {
	Severity string `xml:"severity,attr"`
	Path     string `xml:"path,attr"`
	Reason   string `xml:",chardata"`
}

// lintFlags is lint's one option, --exit-zero (CLI9): the same report,
// but exit 0 whenever it was produced, for callers that discard a
// command's output when it exits non-zero (the agent's shell tools).
func lintFlags(exitZero *bool) *flag.FlagSet {
	flags := flag.NewFlagSet("lint", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(exitZero, "exit-zero", false, "")
	return flags
}

// lintVerb loads the store, creating only a missing store's empty
// memory/notes/ (Part III.7), and reports every problem the reader finds.
// It returns the report alone when the store has no errors, or when
// --exit-zero is given, and the report with errLintFound when it has
// errors. An argument error comes first, --exit-zero or not:
// a failure to run is never exit 0.
func lintVerb(loc location, args []string) ([]byte, error) {
	var exitZero bool
	pos, _, err := parseArgs(lintFlags(&exitZero), args)
	if err != nil {
		return nil, err
	}
	if len(pos) > 0 {
		return nil, fmt.Errorf("lint takes no arguments other than --exit-zero, got %q", pos[0])
	}
	notes, err := loc.notesDir()
	if err != nil {
		return nil, err
	}
	st := memory.Load(notes)
	x := lintXML{Nodes: len(st.Nodes())}
	for _, p := range st.Problems() {
		switch p.Severity {
		case memory.SeverityError:
			x.Errors++
		case memory.SeverityWarning:
			x.Warnings++
		}
		x.Problems = append(x.Problems, problemXML{Severity: string(p.Severity), Path: p.Path, Reason: p.Reason})
	}
	out, err := render(x)
	if err != nil {
		return nil, err
	}
	if x.Errors > 0 && !exitZero {
		return out, errLintFound
	}
	return out, nil
}
