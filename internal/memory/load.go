package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const maxStemLen = 64

var stemRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// validStem reports whether name can address a node.
func validStem(name string) bool {
	return len(name) <= maxStemLen && stemRE.MatchString(name)
}

// Load reads the notes/ directory at notesDir. It never fails: a missing
// notes/ is an empty store, and everything else that cannot be loaded is
// reported in Problems.
func Load(notesDir string) *Store {
	l := &loader{store: newStore(), firstIDs: map[string]string{}}
	l.load(notesDir)
	l.resolveLinks()
	sort.SliceStable(l.store.problems, func(i, j int) bool {
		return comparePaths(l.store.problems[i].Path, l.store.problems[j].Path) < 0
	})
	return l.store
}

// loader is the state of one Load.
type loader struct {
	store *Store
	// rootInfo is the identity of notes/.
	rootInfo os.FileInfo
	// seen holds the identity of every directory walked outside notes/
	// (plus notes/ itself), with the route that walked it.
	seen []seenDir
	// claimed maps a loaded stem to its node's path.
	claimed map[string]string
	// firstIDs maps a block ID to the path of the first node holding it.
	firstIDs map[string]string
}

type seenDir struct {
	info os.FileInfo
	rel  string
}

// folder is a directory being considered as a node.
type folder struct {
	abs     string // path to read it through
	rel     string // slash path relative to notes/
	parent  *Node
	outside bool        // reached through a symlink to outside notes/
	info    os.FileInfo // identity (followed), for outside folders
}

func (l *loader) problemf(sev Severity, rel, format string, args ...any) {
	l.store.problems = append(l.store.problems, Problem{Severity: sev, Path: rel, Reason: fmt.Sprintf(format, args...)})
}

func (l *loader) load(notesDir string) {
	l.claimed = map[string]string{}
	if _, err := os.Lstat(notesDir); errors.Is(err, fs.ErrNotExist) {
		return
	}
	info, err := os.Stat(notesDir)
	if err != nil {
		l.problemf(SeverityError, ".", "cannot read notes/: %v", err)
		return
	}
	if !info.IsDir() {
		l.problemf(SeverityError, ".", "notes/ is not a directory")
		return
	}
	resolved, err := filepath.EvalSymlinks(notesDir)
	if err != nil {
		l.problemf(SeverityError, ".", "cannot resolve notes/: %v", err)
		return
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		l.problemf(SeverityError, ".", "cannot read notes/: %v", err)
		return
	}
	l.rootInfo = info
	l.seen = append(l.seen, seenDir{info: info, rel: "."})
	l.walkEntries(resolved, "", nil, false, entries)
}

// walkEntries considers every entry of a directory whose node (parent, nil
// for notes/) has already loaded. Entries come sorted by name, so the walk
// visits folders in component path order.
func (l *loader) walkEntries(dirAbs, dirRel string, parent *Node, outside bool, entries []os.DirEntry) {
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if parent != nil && name == parent.Stem+".md" {
			continue // the folder note itself
		}
		rel := path.Join(dirRel, name)
		abs := filepath.Join(dirAbs, name)
		switch {
		case e.IsDir():
			l.visitRealDir(folder{abs: abs, rel: rel, parent: parent, outside: outside})
		case e.Type()&fs.ModeSymlink != 0:
			l.visitSymlink(folder{abs: abs, rel: rel, parent: parent})
		default:
			l.problemf(SeverityWarning, rel, "not a node folder: this file is ignored and invisible to the agent")
		}
	}
}

// visitRealDir handles a directory reached without following a symlink at
// this step. Inside notes/ that is its real location; inside a bound
// subtree it is still checked by identity, so a route out of notes/ that
// leads back into it (or around a cycle) is never walked twice.
func (l *loader) visitRealDir(f folder) {
	if f.outside {
		info, err := os.Stat(f.abs)
		if err != nil {
			l.problemf(SeverityError, f.rel, "cannot read folder: %v", err)
			return
		}
		if prev, ok := l.seenAt(info); ok {
			l.problemf(SeverityError, f.rel, "second route to a directory already walked at %s; not walked again", prev)
			return
		}
		f.info = info
	}
	l.visitFolder(f)
}

// visitSymlink handles a symlink entry: a symlink to a directory is a
// candidate folder under the symlink rules; anything else is a stray file.
func (l *loader) visitSymlink(f folder) {
	info, err := os.Stat(f.abs)
	if err != nil {
		l.problemf(SeverityWarning, f.rel, "broken symlink: ignored and invisible to the agent")
		return
	}
	if !info.IsDir() {
		l.problemf(SeverityWarning, f.rel, "not a node folder: this file is ignored and invisible to the agent")
		return
	}
	target, err := filepath.EvalSymlinks(f.abs)
	if err != nil {
		l.problemf(SeverityError, f.rel, "cannot resolve symlink: %v", err)
		return
	}
	if at, ok := l.insideRoot(target); ok {
		l.problemf(SeverityError, f.rel,
			"symlink to a directory inside notes/ (its real location is %s); only the real location is loaded", at)
		return
	}
	if prev, ok := l.seenAt(info); ok {
		l.problemf(SeverityError, f.rel, "second route to a directory already walked at %s; not walked again", prev)
		return
	}
	f.outside, f.info = true, info
	l.visitFolder(f)
}

// insideRoot reports whether target (a symlink-free path) lies inside
// notes/, judged by identity on target and its ancestors, and if so its
// path relative to notes/.
func (l *loader) insideRoot(target string) (string, bool) {
	var tail []string
	for p := target; ; p = filepath.Dir(p) {
		if info, err := os.Stat(p); err == nil && os.SameFile(info, l.rootInfo) {
			if len(tail) == 0 {
				return ".", true
			}
			for i, j := 0, len(tail)-1; i < j; i, j = i+1, j-1 {
				tail[i], tail[j] = tail[j], tail[i]
			}
			return path.Join(tail...), true
		}
		if filepath.Dir(p) == p {
			return "", false
		}
		tail = append(tail, filepath.Base(p))
	}
}

func (l *loader) seenAt(info os.FileInfo) (string, bool) {
	for _, s := range l.seen {
		if os.SameFile(s.info, info) {
			return s.rel, true
		}
	}
	return "", false
}

// visitFolder decides whether a folder loads — before walking anything
// beneath it — and if it does, registers it and walks its entries.
func (l *loader) visitFolder(f folder) {
	stem := path.Base(f.rel)
	if !validStem(stem) {
		l.problemf(SeverityError, f.rel, "folder name %q is not a valid stem (lowercase letters, digits and single hyphens, at most %d characters)", stem, maxStemLen)
		return
	}
	if prev, ok := l.claimed[stem]; ok {
		l.problemf(SeverityError, f.rel, "stem %q is already used by %s", stem, prev)
		return
	}
	entries, err := os.ReadDir(f.abs)
	if err != nil {
		l.problemf(SeverityError, f.rel, "cannot read folder: %v", err)
		return
	}
	n, warnings, err := readFolderNote(f.abs, stem, entries)
	if err != nil {
		l.problemf(SeverityError, f.rel, "%v", err)
		return
	}
	n.Stem, n.Path, n.parent = stem, f.rel, f.parent
	l.register(n, warnings)
	if f.outside {
		l.seen = append(l.seen, seenDir{info: f.info, rel: f.rel})
	}
	l.walkEntries(f.abs, f.rel, n, f.outside, entries)
}

// register adds a node that has loaded, with its warnings.
func (l *loader) register(n *Node, warnings []string) {
	st := l.store
	l.claimed[n.Stem] = n.Path
	st.byStem[n.Stem] = n
	st.order = append(st.order, n)
	if n.parent == nil {
		st.firstLevel = append(st.firstLevel, n)
	} else {
		n.parent.children = append(n.parent.children, n)
	}
	for _, w := range warnings {
		l.problemf(SeverityWarning, n.Path, "%s", w)
	}
	l.checkCrossNodeIDs(n)
}

// checkCrossNodeIDs warns, on the later node in path order, about each
// block ID an earlier node already holds.
func (l *loader) checkCrossNodeIDs(n *Node) {
	reported := map[string]bool{}
	for _, b := range n.Blocks {
		if b.ID == "" || reported[b.ID] {
			continue
		}
		reported[b.ID] = true
		if first, ok := l.firstIDs[b.ID]; ok {
			l.problemf(SeverityWarning, n.Path, "block ID ^%s is also used in %s", b.ID, first)
			continue
		}
		l.firstIDs[b.ID] = n.Path
	}
}

// readFolderNote reads and parses the folder note of the folder at dir,
// whose listing is entries. The note is matched by exact, case-sensitive
// name among the entries — never by a lookup that a case-insensitive
// filesystem would satisfy with A.md for a.md. Anything that is not a
// regular file (judged through symlinks) is refused before it is opened,
// so a named pipe or device can never block the load.
func readFolderNote(dir, stem string, entries []os.DirEntry) (*Node, []string, error) {
	name := stem + ".md"
	missing := fmt.Errorf("missing folder note %s (it must be a regular file with exactly that name, case included)", name)
	if !hasEntry(entries, name) {
		return nil, nil, missing
	}
	notePath := filepath.Join(dir, name)
	info, err := os.Stat(notePath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, missing
	}
	data, err := os.ReadFile(notePath) //nolint:gosec // path is built from a walked directory under notes/
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read folder note %s: %v", name, err)
	}
	n, warnings, err := parseNote(data)
	if err != nil {
		return nil, nil, err
	}
	if !n.dated {
		n.Modified = info.ModTime()
	}
	return n, warnings, nil
}

// hasEntry reports whether a directory listing holds an entry named
// exactly name, byte for byte.
func hasEntry(entries []os.DirEntry, name string) bool {
	for _, e := range entries {
		if e.Name() == name {
			return true
		}
	}
	return false
}

// comparePaths orders slash paths component by component, as a directory
// walk visits them: "a/x" before "a-b/x".
func comparePaths(a, b string) int {
	ac, bc := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(ac) && i < len(bc); i++ {
		if c := strings.Compare(ac[i], bc[i]); c != 0 {
			return c
		}
	}
	return len(ac) - len(bc)
}
