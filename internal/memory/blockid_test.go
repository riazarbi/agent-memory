package memory

import (
	"bytes"
	"crypto/rand"
	"regexp"
	"strings"
	"testing"
)

var generatedIDRE = regexp.MustCompile(`^m-[a-z0-9]{6}$`)

// idDraws returns the random bytes that make the generator draw each of
// suffixes (6 base-36 characters each), in order.
func idDraws(t *testing.T, suffixes ...string) []byte {
	t.Helper()
	var out []byte
	for _, s := range suffixes {
		if len(s) != 6 {
			t.Fatalf("suffix %q is not 6 characters", s)
		}
		for _, r := range s {
			i := strings.IndexRune(idAlphabet, r)
			if i < 0 {
				t.Fatalf("suffix %q has a non-base-36 character", s)
			}
			out = append(out, byte(i)) //nolint:gosec // i indexes the 36-character alphabet
		}
	}
	return out
}

// U5 criterion 1: every generated ID has the D27 form.
func TestBlockID_Form(t *testing.T) {
	g := newIDGen(rand.Reader, nil)
	seen := map[string]bool{}
	for range 2000 {
		id, err := g.next()
		if err != nil {
			t.Fatal(err)
		}
		if !generatedIDRE.MatchString(id) {
			t.Fatalf("id %q does not match m-[a-z0-9]{6}", id)
		}
		if seen[id] {
			t.Fatalf("id %q generated twice", id)
		}
		seen[id] = true
	}
}

// Every base-36 character is reachable, and bytes that would bias the
// draw (>= 252) are discarded rather than folded in.
func TestBlockID_AlphabetAndRejection(t *testing.T) {
	src := append([]byte{252, 255, 253}, idDraws(t, "09azqz")...)
	g := newIDGen(bytes.NewReader(src), nil)
	id, err := g.next()
	if err != nil {
		t.Fatal(err)
	}
	if id != "m-09azqz" {
		t.Errorf("id = %q, want m-09azqz (bytes >= 252 skipped)", id)
	}
	if len(idAlphabet) != 36 || strings.ToLower(idAlphabet) != idAlphabet {
		t.Errorf("alphabet %q is not 36 lowercase base-36 characters", idAlphabet)
	}
}

// Criterion 1: a draw that collides with an ID already in the store, or
// one assigned earlier in the same encode, is drawn again.
func TestBlockID_RedrawsOnCollision(t *testing.T) {
	taken := map[string]bool{"m-aaaaaa": true}
	src := idDraws(t, "aaaaaa", "bbbbbb", "aaaaaa", "bbbbbb", "cccccc")
	g := newIDGen(bytes.NewReader(src), taken)
	first, err := g.next()
	if err != nil {
		t.Fatal(err)
	}
	if first != "m-bbbbbb" {
		t.Fatalf("first = %q, want m-bbbbbb (m-aaaaaa is in the store)", first)
	}
	second, err := g.next()
	if err != nil {
		t.Fatal(err)
	}
	if second != "m-cccccc" {
		t.Fatalf("second = %q, want m-cccccc (store ID and the earlier assignment both redrawn)", second)
	}
}

// A source that runs dry is an error, never a short or reused ID.
func TestBlockID_SourceFailure(t *testing.T) {
	g := newIDGen(bytes.NewReader(idDraws(t, "aaaaaa")[:3]), nil)
	if id, err := g.next(); err == nil {
		t.Fatalf("next() = %q, want an error from an exhausted source", id)
	}
	// A source that only ever collides gives up with an error.
	taken := map[string]bool{"m-aaaaaa": true}
	g = newIDGen(bytes.NewReader(bytes.Repeat(idDraws(t, "aaaaaa"), maxIDDraws+1)), taken)
	if id, err := g.next(); err == nil {
		t.Fatalf("next() = %q, want an error after %d collisions", id, maxIDDraws)
	}
}
