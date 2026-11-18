package memory

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// fileState is one entry of a fixture tree, as the encoder must leave it.
type fileState struct {
	mode    fs.FileMode
	modTime time.Time
	content string
}

// snapshotTree records every file and directory under root.
func snapshotTree(t *testing.T, root string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		st := fileState{mode: info.Mode(), modTime: info.ModTime()}
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(p) //nolint:gosec // test fixture path
			if err != nil {
				return err
			}
			st.content = string(b)
		}
		out[p] = st
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// sameStore fails unless got - a store encodes have run against - is
// indistinguishable from a fresh load of the same files.
func sameStore(t *testing.T, got, fresh *Store) {
	t.Helper()
	if !reflect.DeepEqual(stemsOf(got.Nodes()), stemsOf(fresh.Nodes())) {
		t.Fatalf("nodes %q, fresh %q", stemsOf(got.Nodes()), stemsOf(fresh.Nodes()))
	}
	for _, f := range fresh.Nodes() {
		g := mustNode(t, got, f.Stem)
		checks := []struct {
			what      string
			got, want any
		}{
			{"Frontmatter.Raw", g.Frontmatter.Raw, f.Frontmatter.Raw},
			{"Frontmatter.Doc", g.Frontmatter.Doc, f.Frontmatter.Doc},
			{"Frontmatter.Fields", g.Frontmatter.Fields, f.Frontmatter.Fields},
			{"Description", g.Description, f.Description},
			{"Tags", g.Tags, f.Tags},
			{"Properties", g.Properties, f.Properties},
			{"Blocks", g.Blocks, f.Blocks},
			{"Source", g.Source, f.Source},
			{"BOM", g.BOM, f.BOM},
			{"LineEnding", g.LineEnding, f.LineEnding},
		}
		for _, c := range checks {
			if !reflect.DeepEqual(c.got, c.want) {
				t.Errorf("node %s: %s changed by an encode\n got %#v\nwant %#v", f.Stem, c.what, c.got, c.want)
			}
		}
		// Every field node must still be the document's own node.
		checkFieldsAreDocNodes(t, g)
	}
	if !reflect.DeepEqual(got.Problems(), fresh.Problems()) {
		t.Errorf("problems changed: %s vs %s", dump(got.Problems()), dump(fresh.Problems()))
	}
}

// U5 criterion 8: the encodes of criteria 4-7 leave the loaded store
// exactly as a fresh load of the same files, and touch no file.
func TestEncode_Pure(t *testing.T) {
	files := corpusFiles(unchangedCorpus)
	files["c5-body/c5-body.md"] = bodyFixture
	files["c6-fields/c6-fields.md"] = fieldsFixture
	files["c7-alias/c7-alias.md"] = aliasFixture
	files["c7-crlf/c7-crlf.md"] = crlfFixture
	st, dir := storeOf(t, files)
	root := filepath.Dir(dir)
	before := snapshotTree(t, root)

	type job struct {
		stem string
		c    Change
	}
	jobs := make([]job, 0, len(unchangedCorpus)+8)
	jobs = append(jobs, []job{
		{"c5-body", Change{Blocks: []BlockEdit{{Text: "Appended."}}}},
		{"c5-body", Change{Blocks: []BlockEdit{{ID: "m-b5aaa2", Text: "Replaced."}}}},
		{"c6-fields", Change{Description: strp("new"), Properties: map[string]any{"keep": 2, "new": "v"}}},
		{"c6-fields", Change{Tags: tagsp("x"), Properties: map[string]any{"l": []string{"z"}, "d": "text"}}},
		{"c7-alias", Change{Properties: map[string]any{"a": "new"}}},
		{"c7-alias", Change{Properties: map[string]any{"b": "mine"}}},
		{"c7-crlf", Change{Properties: map[string]any{"x": 2}, Blocks: []BlockEdit{{Text: "New."}}}},
		{"comments", Change{Description: strp("re-emitted"), Tags: tagsp()}},
	}...)
	for stem := range unchangedCorpus {
		jobs = append(jobs, job{stem: stem})
	}
	for _, j := range jobs {
		out, err := Encoder{Store: st}.EncodeChange(j.stem, j.c)
		if err != nil {
			t.Fatalf("%s: %v", j.stem, err)
		}
		loadsClean(t, j.stem, out.Bytes)
	}
	if _, err := (Encoder{Store: st}).EncodeNew(NewNode{Description: "d", Blocks: []string{"x"}}); err != nil {
		t.Fatal(err)
	}

	sameStore(t, st, Load(dir))
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Errorf("fixture tree changed by encoding:\nbefore %v\nafter  %v", before, after)
	}
}
