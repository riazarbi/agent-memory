//go:build unix

package memory

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// CLI7 criterion 2 (library): a WriteRequest whose Parent is the root, "/",
// creates a first-level node in notes/<stem>/, modes forced, under umask 077.
func TestWrite_CreateUnderRoot(t *testing.T) {
	dir := writerTree(t, nil)
	withUmask(t, 0o077)
	res, err := writerFor(t, dir).Write(WriteRequest{
		Stem: "kettle", Parent: RootParent, Description: strp("d"),
		Blocks: []BlockEdit{{Text: "A claim."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created {
		t.Error("create under the root not reported as created")
	}
	folder := filepath.Join(dir, "kettle")
	if m := fileMode(t, folder); m != 0o755 {
		t.Errorf("folder mode %v, want 0755", m)
	}
	if m := fileMode(t, filepath.Join(folder, "kettle.md")); m != 0o644 {
		t.Errorf("folder note mode %v, want 0644", m)
	}
	s := loadClean(t, dir)
	n := mustNode(t, s, "kettle")
	if n.Path != "kettle" || n.Parent() != nil {
		t.Errorf("kettle: path %q parent %v, want a first-level node at kettle", n.Path, n.Parent())
	}
	wantStems(t, s.FirstLevel(), "kettle", "topics", "user")
	if res.Node == nil || res.Node.Path != "kettle" {
		t.Errorf("returned node %+v, want the node at kettle", res.Node)
	}
}

// CLI7 criterion 2 (library): an empty notes/ - no node at all - takes a
// create under the root, which is the only create that can succeed there.
func TestWrite_CreateUnderRootOnEmptyNotes(t *testing.T) {
	dir := t.TempDir()
	w := writerFor(t, dir)
	_, err := w.Write(WriteRequest{Stem: "kettle", Parent: "nope", Description: strp("d")})
	wantFieldRefusal(t, err, "parent", "nope")
	if _, err := w.Write(WriteRequest{Stem: "kettle", Parent: RootParent, Description: strp("d")}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(WriteRequest{Stem: "descale", Parent: "kettle", Description: strp("e")}); err != nil {
		t.Fatal(err)
	}
	s := loadClean(t, dir)
	wantStems(t, s.FirstLevel(), "kettle")
	if n := mustNode(t, s, "descale"); n.Path != "kettle/descale" {
		t.Errorf("descale at %q, want kettle/descale", n.Path)
	}
	// What a create under the root writes is EncodeNew's output for it.
	enc, err := Encoder{}.EncodeNew(NewNode{Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fileBytes(t, dir, "kettle/kettle.md")); got != string(enc.Bytes) {
		t.Errorf("kettle.md %q, want %q", got, enc.Bytes)
	}
}

// CLI7 criterion 2 (library): an update of a first-level node may name the
// root as its parent; an update of a nested node may not, and is refused
// naming parent with nothing written. The root is never a node's own stem.
func TestWrite_UpdateWithRootParent(t *testing.T) {
	dir := writerTree(t, nil)
	w := writerFor(t, dir)
	res, err := w.Write(WriteRequest{Stem: "user", Parent: RootParent, Blocks: []BlockEdit{{Text: "Likes tables."}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created {
		t.Error("an update reported as a create")
	}
	if got := blockTexts(mustNode(t, loadClean(t, dir), "user"))[res.BlockIDs[0]]; got != "Likes tables." {
		t.Errorf("user's new block %q", got)
	}

	if _, err := w.Write(WriteRequest{Stem: "child", Parent: "topics", Description: strp("d")}); err != nil {
		t.Fatal(err)
	}
	before := treeState(t, dir)
	_, err = w.Write(WriteRequest{Stem: "child", Parent: RootParent, Blocks: []BlockEdit{{Text: "c"}}})
	wantFieldRefusal(t, err, "parent", "topics")
	wantTree(t, dir, before)

	_, err = w.Write(WriteRequest{Stem: RootParent, Description: strp("x")})
	wantFieldRefusal(t, err, "stem")
	wantTree(t, dir, before)
}

// CLI7 criterion 1: the library seeds nothing and names no node. No
// production file declares SeedDefaults, and none holds the string literal
// "user", "projects", "people" or "topics".
func TestNoBuiltInTypology(t *testing.T) {
	files := productionFiles(t, ".", filepath.Join("..", "..", "cmd", "memory"))
	if len(files) < 10 {
		t.Fatalf("found only %d production files: %v", len(files), files)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "SeedDefaults" {
				t.Errorf("%s declares SeedDefaults", f)
			}
		}
		for _, lit := range typologyLiterals(file) {
			t.Errorf("%s holds the string literal %s", fset.Position(lit.Pos()), lit.Value)
		}
	}
}

// productionFiles lists the non-test .go files in each package dir.
func productionFiles(t *testing.T, dirs ...string) []string {
	t.Helper()
	var files []string
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range matches {
			if !strings.HasSuffix(m, "_test.go") {
				files = append(files, m)
			}
		}
	}
	return files
}

// typologyLiterals returns the string literals in file whose value is one
// of the starting tree's stems.
func typologyLiterals(file *ast.File) []*ast.BasicLit {
	banned := map[string]bool{"user": true, "projects": true, "people": true, "topics": true}
	var out []*ast.BasicLit
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil && banned[s] {
				out = append(out, lit)
			}
		}
		return true
	})
	return out
}
