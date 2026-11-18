package memory

import (
	"errors"
	"fmt"
	"io"
)

// idAlphabet is the base-36 digits a block ID is drawn from.
const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

const (
	// idLen is the number of base-36 characters after "m-".
	idLen = 6
	// idByteLimit is the largest multiple of 36 that fits in a byte: bytes
	// at or above it are discarded so every character is equally likely.
	idByteLimit = 252
	// maxIDDraws bounds the redraws on collision. With a cryptographic
	// source a single collision is already unlikely; the bound only stops
	// a broken source from looping forever.
	maxIDDraws = 100
)

// idGen draws block IDs (D27): "m-" plus 6 base-36 characters from a
// random source, never one already taken - by the store, or by an earlier
// draw from the same generator.
type idGen struct {
	r     io.Reader
	taken map[string]bool
}

// newIDGen returns a generator over r that avoids every ID in taken. taken
// is copied; draws are added to the copy.
func newIDGen(r io.Reader, taken map[string]bool) *idGen {
	g := &idGen{r: r, taken: make(map[string]bool, len(taken))}
	for id := range taken {
		g.taken[id] = true
	}
	return g
}

// next returns a fresh ID and marks it taken.
func (g *idGen) next() (string, error) {
	for range maxIDDraws {
		id, err := g.draw()
		if err != nil {
			return "", fmt.Errorf("drawing a block ID: %w", err)
		}
		if !g.taken[id] {
			g.taken[id] = true
			return id, nil
		}
	}
	return "", errors.New("drawing a block ID: every draw collided with an ID already in use")
}

// draw reads one candidate ID by rejection sampling, one byte per character.
func (g *idGen) draw() (string, error) {
	out := make([]byte, 0, len("m-")+idLen)
	out = append(out, "m-"...)
	var b [1]byte
	for len(out) < cap(out) {
		if _, err := io.ReadFull(g.r, b[:]); err != nil {
			return "", err
		}
		if b[0] >= idByteLimit {
			continue
		}
		out = append(out, idAlphabet[int(b[0])%len(idAlphabet)])
	}
	return string(out), nil
}
