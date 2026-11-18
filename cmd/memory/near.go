package main

import (
	"sort"
	"strings"
)

// maxNearMatches is how many near matches an unknown stem lists (D29).
const maxNearMatches = 5

// nearMatches returns up to five stems resembling stem, closest first: those
// containing it or contained in it, and those within an edit distance of
// half the longer stem's length. Ties go by stem.
func nearMatches(stem string, stems []string) []string {
	type scored struct {
		stem string
		dist int
	}
	var near []scored
	for _, s := range stems {
		d := editDistance(stem, s)
		contains := stem != "" && (strings.Contains(s, stem) || strings.Contains(stem, s))
		if contains || 2*d <= max(len(stem), len(s)) {
			near = append(near, scored{s, d})
		}
	}
	sort.Slice(near, func(i, j int) bool {
		if near[i].dist != near[j].dist {
			return near[i].dist < near[j].dist
		}
		return near[i].stem < near[j].stem
	})
	out := make([]string, 0, maxNearMatches)
	for i := 0; i < len(near) && i < maxNearMatches; i++ {
		out = append(out, near[i].stem)
	}
	return out
}

// editDistance is the Levenshtein distance between a and b, by bytes (stems
// are ASCII).
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
