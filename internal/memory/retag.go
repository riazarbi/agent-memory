package memory

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RetagResult is a successful retag: the tags as used (no leading '#'; New
// is empty for a removal) and the number of nodes whose folder note
// changed.
type RetagResult struct {
	Old   string
	New   string
	Nodes int
}

// CleanTag validates a tag as a caller gives it (D17), with or without a
// leading '#', and returns it without one.
func CleanTag(raw string) (string, error) { return tag(raw) }

// Retag replaces the tag old with new across the store (Part III.8
// "retag, exactly"), or removes it when new is empty: in each node's
// frontmatter tags, and in inline #tags in block text. Nested tags go with
// their parent, by whole path segment. Only notes that change are written.
// A bad tag is a *FieldError naming "old" or "new". Any failure leaves the
// store as it was.
func (w *Writer) Retag(old, new string) (RetagResult, error) {
	res, err := retagPair(old, new)
	if err != nil || res.New == res.Old {
		return res, err
	}
	from, to := res.Old, res.New
	st, unlock, err := w.locked()
	if err != nil {
		return RetagResult{}, err
	}
	defer unlock()
	// Every note is worked out before any is written, so a block that
	// cannot be rewritten fails the call with nothing changed.
	var edits []noteEdit
	for _, n := range st.order {
		data, changed, err := retagNote(n, from, to)
		if err != nil {
			return RetagResult{}, err
		}
		if changed {
			edits = append(edits, noteEdit{node: n, data: data})
		}
	}
	if len(edits) == 0 {
		return res, nil
	}
	if _, err := w.replaceNotes(st, edits, "retagging "+from); err != nil {
		return RetagResult{}, err
	}
	res.Nodes = len(edits)
	return res, nil
}

// retagPair validates a retag's two tags and strips their leading '#'.
func retagPair(old, new string) (RetagResult, error) {
	from, err := tag(old)
	if err != nil {
		return RetagResult{}, fieldErr("old", "%v", err)
	}
	if new == "" {
		return RetagResult{Old: from}, nil
	}
	to, err := tag(new)
	if err != nil {
		return RetagResult{}, fieldErr("new", "%v", err)
	}
	return RetagResult{Old: from, New: to}, nil
}

// retagged returns t with the tag from replaced by to, and whether t is
// from or nested under it (from/…). Matching is on whole path segments.
func retagged(t, from, to string) (string, bool) {
	if t == from || strings.HasPrefix(t, from+"/") {
		return to + t[len(from):], true
	}
	return t, false
}

// retagList applies a retag to a node's frontmatter tags: matching tags
// are renamed, or dropped when to is empty, and a tag the list then holds
// twice is kept once, at its first position. It reports whether the list
// changed.
func retagList(tags []string, from, to string) ([]string, bool) {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	touched := false
	for _, t := range tags {
		nt, match := retagged(t, from, to)
		if match {
			touched = true
			if to == "" {
				continue
			}
			t = nt
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if !touched {
		return tags, false
	}
	return out, !slices.Equal(out, tags)
}

// cut replaces text[start:end] with repl.
type cut struct {
	start, end int
	repl       string
}

func applyCuts(s string, cuts []cut) string {
	for i := len(cuts) - 1; i >= 0; i-- {
		c := cuts[i]
		s = s[:c.start] + c.repl + s[c.end:]
	}
	return s
}

// retagCuts returns the changes a retag makes to a block's text, in
// order: each inline tag (found as search finds them) that is from or
// nested under it is renamed, or removed when to is empty.
func retagCuts(text, from, to string) []cut {
	var hits []tagSpan
	for _, s := range inlineTagSpans(text) {
		if _, match := retagged(s.tag, from, to); match {
			hits = append(hits, s)
		}
	}
	if to == "" {
		return removalCuts(text, hits)
	}
	cuts := make([]cut, 0, len(hits))
	for _, s := range hits {
		nt, _ := retagged(s.tag, from, to)
		cuts = append(cuts, cut{start: s.start, end: s.end, repl: "#" + nt})
	}
	return cuts
}

// removalCuts removes each tag token and one adjacent whitespace
// character, so no double space is left: the one before it, or, for a tag
// that starts the text or a line, the one after it (a tag alone on a line
// goes with its line break).
func removalCuts(text string, hits []tagSpan) []cut {
	removed := make([]bool, len(text))
	mark := func(from, to int) {
		for i := from; i < to; i++ {
			removed[i] = true
		}
	}
	for _, s := range hits {
		mark(s.start, s.end)
		// What precedes the tag once earlier removals are applied.
		before := s.start
		for before > 0 && removed[before-1] {
			before--
		}
		mark(adjacentSpace(text, before, s.end))
	}
	return maskCuts(removed)
}

// adjacentSpace picks the one whitespace character that goes with a
// removed tag: text[:before] is what precedes it, text[after:] what
// follows. It returns an empty range when neither side has one.
func adjacentSpace(text string, before, after int) (from, to int) {
	prev, prevSize := utf8.DecodeLastRuneInString(text[:before])
	next, nextSize := utf8.DecodeRuneInString(text[after:])
	startsLine := before == 0 || prev == '\n'
	switch {
	case nextSize > 0 && unicode.IsSpace(next) && startsLine:
		return after, after + nextSize
	case before > 0 && unicode.IsSpace(prev):
		return before - prevSize, before
	}
	return 0, 0
}

// maskCuts turns a mask of removed bytes into cuts, one per run.
func maskCuts(removed []bool) []cut {
	var cuts []cut
	for i := 0; i < len(removed); {
		if !removed[i] {
			i++
			continue
		}
		j := i
		for j < len(removed) && removed[j] {
			j++
		}
		cuts = append(cuts, cut{start: i, end: j})
		i = j
	}
	return cuts
}

// retagNote returns n's folder note with the retag applied, and whether
// it differs from the note as read. In the body only the tag tokens
// change: each block is edited in its source lines, so line endings, the
// way its ID is written and every untouched block stay byte for byte. A
// changed tag list is re-emitted as any frontmatter change is (D26).
func retagNote(n *Node, from, to string) ([]byte, bool, error) {
	tags, tagsChanged := retagList(n.Tags, from, to)
	lines, bodyStart, runs, err := bodyLines(n)
	if err != nil {
		return nil, false, err
	}
	want, err := expectationOf(n)
	if err != nil {
		return nil, false, err
	}
	body := append([]string(nil), lines[bodyStart:]...)
	bodyChanged, err := retagBody(n, body, runs, want.blocks, from, to)
	if err != nil {
		return nil, false, err
	}
	if !tagsChanged && !bodyChanged {
		return nil, false, nil
	}
	var out strings.Builder
	if n.BOM {
		out.WriteString(bomText)
	}
	if tagsChanged {
		fm, _, err := rewriteFrontmatter(n, changeReq{tags: &tags})
		if err != nil {
			return nil, false, fmt.Errorf("node %s: %w", n.Stem, err)
		}
		eol := n.LineEnding
		out.WriteString(delimiter + eol + strings.ReplaceAll(fm, "\n", eol) + delimiter + eol)
		want.tags = tags
	} else {
		out.WriteString(strings.Join(lines[:bodyStart], ""))
	}
	out.WriteString(strings.Join(body, ""))
	data := []byte(out.String())
	if err := verifyOutput(data, want); err != nil {
		return nil, false, fmt.Errorf("node %s: %w", n.Stem, err)
	}
	return data, true, nil
}

// retagBody applies a retag to n's blocks in place: body holds the body's
// source lines and runs each block's lines in it; a changed block's lines
// are rewritten there (joined into its first line) and its new text
// recorded in want. It reports whether any block changed.
func retagBody(n *Node, body []string, runs []lineRun, want []Block, from, to string) (bool, error) {
	changed := false
	for i, b := range n.Blocks {
		cuts := retagCuts(b.Text, from, to)
		if len(cuts) == 0 {
			continue
		}
		text := applyCuts(b.Text, cuts)
		if err := retaggedTextErr(n, i, text); err != nil {
			return false, err
		}
		r := runs[i]
		body[r.start] = applyCuts(strings.Join(body[r.start:r.end], ""), sourceCuts(b, cuts))
		for k := r.start + 1; k < r.end; k++ {
			body[k] = ""
		}
		want[i].Text = text
		changed = true
	}
	return changed, nil
}

// retaggedTextErr refuses a block text a retag would leave empty, or no
// longer one plain paragraph (a line left blank, or a removed tag exposing
// a list marker, say). It names the block.
func retaggedTextErr(n *Node, i int, text string) error {
	name := fmt.Sprintf("block %d of node %s (it has no ID)", i+1, n.Stem)
	if id := n.Blocks[i].ID; id != "" {
		name = fmt.Sprintf("block %s of node %s", id, n.Stem)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%s holds nothing but the tag, and a block cannot be left with no text: delete the block instead; nothing was written", name)
	}
	for k, line := range strings.Split(text, "\n") {
		what := nonParagraph(line, k == 0)
		if blankLine(line) {
			what = "blank"
		}
		if what != "" {
			return fmt.Errorf("%s would no longer be one plain paragraph without the tag (line %q would be %s): rewrite the block instead; nothing was written", name, line, what)
		}
	}
	return nil
}

// sourceCuts maps cuts on a block's text to cuts on its source lines
// joined. Text line k is a prefix of source line k (the source line has
// its line ending, and the last one the block ID), so an offset keeps its
// line and column; a cut that takes the "\n" joining two text lines takes
// the source line's own ending, whatever it is.
func sourceCuts(b Block, cuts []cut) []cut {
	textLines := strings.Split(b.Text, "\n")
	textStarts, srcStarts := make([]int, 0, len(textLines)), make([]int, 0, len(textLines))
	textAt, srcAt := 0, 0
	for k, line := range textLines {
		textStarts, srcStarts = append(textStarts, textAt), append(srcStarts, srcAt)
		textAt += len(line) + 1
		srcAt += len(b.Source[k])
	}
	at := func(off int) int {
		k := len(textStarts) - 1
		for k > 0 && textStarts[k] > off {
			k--
		}
		return srcStarts[k] + off - textStarts[k]
	}
	out := make([]cut, 0, len(cuts))
	for _, c := range cuts {
		out = append(out, cut{start: at(c.start), end: at(c.end), repl: c.repl})
	}
	return out
}
