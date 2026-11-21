package memory

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"time"
)

// expectation is what an encoded note must load back as: the values a
// write supplied, and everything it left untouched.
type expectation struct {
	description string
	tags        []string
	props       []Property
	blocks      []Block // ID and Text only
	// modified is the text the modified property must read back as, and
	// hasModified whether the note must carry the key at all (III.9).
	modified    string
	hasModified bool
	// warnings are the reader warnings the note may still carry: those of
	// the untouched operator content it was rewritten from. Supplied
	// content may add none.
	warnings []string
}

// verifyOutput loads the complete encoded note - delimiter lines, BOM and
// all - through the read model's folder-note parser, and fails unless it
// loads, every value and block reads back exactly as expected, and it
// carries no warning the source did not already have (D27 "Guarantee").
func verifyOutput(out []byte, want expectation) error {
	got, warnings, err := parseNote(out)
	if err != nil {
		return fmt.Errorf("refusing to emit a folder note the reader rejects: %v", err)
	}
	if err := compareNode(got, want); err != nil {
		return fmt.Errorf("refusing to emit a folder note that does not read back as written: %v", err)
	}
	if extra := extraWarnings(warnings, want.warnings); extra != "" {
		return fmt.Errorf("refusing to emit a folder note the reader warns about: %s", extra)
	}
	return nil
}

func compareNode(got *Node, want expectation) error {
	if got.Description != want.description {
		return fmt.Errorf("description reads back as %q, want %q", got.Description, want.description)
	}
	if !sameStrings(got.Tags, want.tags) {
		return fmt.Errorf("tags read back as %q, want %q", got.Tags, want.tags)
	}
	if err := compareProperties(got.Properties, want.props); err != nil {
		return err
	}
	if text, has := modifiedField(got); has != want.hasModified || text != want.modified {
		return fmt.Errorf("modified reads back as %q (present: %v), want %q (present: %v)", text, has, want.modified, want.hasModified)
	}
	return compareBlocks(got.Blocks, want.blocks)
}

func compareProperties(got, want []Property) error {
	if len(got) != len(want) {
		return fmt.Errorf("%d properties read back, want %d", len(got), len(want))
	}
	for i, p := range want {
		if g := got[i]; g.Key != p.Key || !valuesEqual(g.Value, p.Value) {
			return fmt.Errorf("property %d reads back as %q = %#v, want %q = %#v", i, g.Key, g.Value, p.Key, p.Value)
		}
	}
	return nil
}

func compareBlocks(got, want []Block) error {
	if len(got) != len(want) {
		return fmt.Errorf("%d blocks read back, want %d", len(got), len(want))
	}
	for i, b := range want {
		if g := got[i]; g.ID != b.ID || g.Text != b.Text {
			return fmt.Errorf("block %d reads back as ^%s %q, want ^%s %q", i, g.ID, g.Text, b.ID, b.Text)
		}
	}
	return nil
}

// extraWarnings returns the first warning in got beyond those allowed
// (counted as a multiset), or "".
func extraWarnings(got, allowed []string) string {
	left := map[string]int{}
	for _, w := range allowed {
		left[w]++
	}
	for _, w := range got {
		if left[w] == 0 {
			return w
		}
		left[w]--
	}
	return ""
}

// sameStrings compares lists item by item; nil and empty are alike.
func sameStrings(a, b []string) bool { return slices.Equal(a, b) }

// valuesEqual compares property values as the reader decodes them: numbers
// by value (JSON cannot tell 3.0 from 3), dates and datetimes by instant
// and offset, lists item by item (nil and empty alike).
func valuesEqual(a, b any) bool {
	if x, ok := asNumber(a); ok {
		y, ok := asNumber(b)
		return ok && x.equal(y)
	}
	switch x := a.(type) {
	case time.Time:
		y, ok := b.(time.Time)
		return ok && sameInstant(x, y)
	case []string:
		y, ok := b.([]string)
		return ok && sameStrings(x, y)
	}
	return reflect.DeepEqual(a, b)
}

func sameInstant(x, y time.Time) bool {
	_, xo := x.Zone()
	_, yo := y.Zone()
	return x.Equal(y) && xo == yo
}

type number struct {
	i     int64
	f     float64
	isInt bool
}

func asNumber(v any) (number, bool) {
	switch x := v.(type) {
	case int:
		return number{i: int64(x), f: float64(x), isInt: true}, true
	case int64:
		return number{i: x, f: float64(x), isInt: true}, true
	case uint64:
		if x <= math.MaxInt64 {
			return number{i: int64(x), f: float64(x), isInt: true}, true
		}
		return number{f: float64(x)}, true
	case float64:
		return number{f: x}, true
	}
	return number{}, false
}

func (n number) equal(m number) bool {
	if n.isInt && m.isInt {
		return n.i == m.i
	}
	return n.f == m.f
}
