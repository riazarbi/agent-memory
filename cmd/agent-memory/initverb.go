package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/riazarbi/agent-memory/internal/memory"
)

// initXML is init's result: the store, whether it was seeded, and the files
// created, relative to the store.
type initXML struct {
	XMLName xml.Name `xml:"init"`
	Store   string   `xml:"store,attr"`
	Seeded  bool     `xml:"seeded,attr"`
	Created []string `xml:"created"`
	Note    string   `xml:",chardata"`
}

// initVerb seeds memory/ with the default protocol bundle if it is missing
// (Part III.2, D19). Once memory/ exists — even empty, even without
// usage.md — it is never touched.
func initVerb(args []string) ([]byte, error) {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pos, _, err := parseArgs(flags, args)
	if err != nil {
		return nil, err
	}
	if len(pos) > 0 {
		return nil, fmt.Errorf("init takes no arguments, got %q", pos[0])
	}
	dir, err := memoryDir()
	if err != nil {
		return nil, err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(dir); err == nil {
		return render(initXML{Store: dir, Note: "memory/ already exists; nothing was needed"})
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	if err := memory.SeedDefaults(dir); err != nil {
		return nil, err
	}
	created, err := filesUnder(dir)
	if err != nil {
		return nil, err
	}
	return render(initXML{Store: dir, Seeded: true, Created: created})
}

// filesUnder lists the regular files under root, slash-separated and
// relative to it, in walk (lexical) order.
func filesUnder(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		out = append(out, filepath.ToSlash(rel))
		return err
	})
	return out, err
}
