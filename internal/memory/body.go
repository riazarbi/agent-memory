package memory

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// codeIndent is the indent width at which a line becomes indented code.
	codeIndent = 4
	tabWidth   = 4
)

// blockRules are the line starts (after up to three spaces of indent) that
// Obsidian renders as something other than paragraph text.
var blockRules = []struct {
	re   *regexp.Regexp
	what string
}{
	{regexp.MustCompile(`^#{1,6}([ \t]|$)`), "a heading"},
	{regexp.MustCompile(`^([-*+]|[0-9]+[.)])([ \t]|$)`), "a list item"},
	{regexp.MustCompile("^(```|~~~)"), "a code fence"},
	{regexp.MustCompile(`^>`), "a blockquote"},
	{regexp.MustCompile(`^\|`), "a table row"},
	{regexp.MustCompile(`^((-[ \t]*){3,}|(\*[ \t]*){3,}|(_[ \t]*){3,})$`), "a thematic break"},
	{regexp.MustCompile(`^<[A-Za-z/!]`), "an HTML block (an autolink at line start counts)"},
	{regexp.MustCompile(`^\$\$`), "a math block"},
	{regexp.MustCompile(`^%%`), "a comment block"},
	{regexp.MustCompile(`^\[\^[^\]]+\]:`), "a footnote definition"},
	{regexp.MustCompile(`^\[[^\[\]]+\]:([ \t]|$)`), "a link reference definition"},
}

var (
	setextRE = regexp.MustCompile(`^(=+|-+)[ \t]*$`)
	// blockIDRE matches a harness block ID ending a block's last line.
	blockIDRE = regexp.MustCompile(`(^|[ \t])\^(m-[a-z0-9]{6})[ \t]*$`)
	// caretTokenRE matches any other trailing ^token, e.g. Obsidian's own.
	caretTokenRE = regexp.MustCompile(`(^|[ \t])\^([A-Za-z0-9-]+)[ \t]*$`)
)

// parseBody splits body lines into blocks — runs of non-blank lines — and
// requires each to be a plain paragraph. src holds the same lines as in the
// source, each with its own line ending, so each block records its exact
// source lines.
func parseBody(lines, src []string) ([]Block, []string, error) {
	var (
		blocks   []Block
		warnings []string
		ids      = map[string]bool{}
	)
	for _, r := range splitRuns(lines) {
		run := lines[r.start:r.end]
		if err := checkParagraph(run); err != nil {
			return nil, nil, err
		}
		b, w := parseBlock(run)
		b.Source = append([]string(nil), src[r.start:r.end]...)
		warnings = append(warnings, w...)
		if b.ID != "" && ids[b.ID] {
			warnings = append(warnings, fmt.Sprintf("block ID ^%s appears more than once in this node; links resolve to the first", b.ID))
		}
		ids[b.ID] = true
		blocks = append(blocks, b)
	}
	return blocks, warnings, nil
}

// lineRun is a run of lines [start, end).
type lineRun struct{ start, end int }

// splitRuns returns the runs of non-blank lines.
func splitRuns(lines []string) []lineRun {
	var runs []lineRun
	start := -1
	for i, line := range lines {
		switch {
		case !blankLine(line) && start < 0:
			start = i
		case blankLine(line) && start >= 0:
			runs = append(runs, lineRun{start, i})
			start = -1
		}
	}
	if start >= 0 {
		runs = append(runs, lineRun{start, len(lines)})
	}
	return runs
}

// blankLine reports whether a body line separates blocks.
func blankLine(line string) bool { return strings.TrimSpace(line) == "" }

func checkParagraph(run []string) error {
	for i, line := range run {
		if what := nonParagraph(line, i == 0); what != "" {
			return fmt.Errorf("body line %q is %s; a node body holds plain paragraphs only", line, what)
		}
	}
	return nil
}

// nonParagraph names what Obsidian would render line as, or "" if it is
// paragraph text.
func nonParagraph(line string, first bool) string {
	width, rest := indent(line)
	if width >= codeIndent {
		return "indented code (four or more spaces)"
	}
	if !first && setextRE.MatchString(rest) {
		return "a heading underline"
	}
	for _, r := range blockRules {
		if r.re.MatchString(rest) {
			return r.what
		}
	}
	return ""
}

// indent returns a line's leading indent width (a tab counts as four) and
// the line after it.
func indent(line string) (int, string) {
	width := 0
	for i, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += tabWidth
		default:
			return width, line[i:]
		}
	}
	return width, ""
}

// parseBlock reads a paragraph's block ID and links.
func parseBlock(run []string) (Block, []string) {
	var warnings []string
	lines := append([]string(nil), run...)
	last := lines[len(lines)-1]
	var id string
	if m := blockIDRE.FindStringSubmatchIndex(last); m != nil {
		id = last[m[4]:m[5]]
		last = strings.TrimRight(last[:m[0]], " \t")
	} else if m := caretTokenRE.FindStringSubmatch(last); m != nil {
		warnings = append(warnings, fmt.Sprintf("trailing ^%s is not a harness block ID (^m- and 6 lowercase letters or digits); kept as text", m[2]))
	}
	if strings.TrimSpace(last) == "" {
		lines = lines[:len(lines)-1]
	} else {
		lines[len(lines)-1] = last
	}
	b := Block{ID: id, Text: strings.Join(lines, "\n")}
	var lw []string
	b.Edges, lw = scanLinks(b.Text)
	return b, append(warnings, lw...)
}
