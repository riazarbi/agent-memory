package main

import (
	"flag"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgsInterleaved(t *testing.T) {
	newFS := func() (*flag.FlagSet, *string, *string) {
		fs := flag.NewFlagSet("read", flag.ContinueOnError)
		a := fs.String("block", "", "")
		b := fs.String("depth", "1", "")
		return fs, a, b
	}
	cases := []struct {
		args       []string
		pos        []string
		block, dep string
	}{
		{[]string{"x", "--depth", "2"}, []string{"x"}, "", "2"},
		{[]string{"--depth", "2", "x"}, []string{"x"}, "", "2"},
		{[]string{"--depth=2", "x", "-block", "m-1"}, []string{"x"}, "m-1", "2"},
		{[]string{"--", "--depth"}, []string{"--depth"}, "", "1"},
		{[]string{"--depth", "-1"}, nil, "", "-1"},
		{[]string{"a", "b"}, []string{"a", "b"}, "", "1"},
		{[]string{"-"}, []string{"-"}, "", "1"},
	}
	for _, tc := range cases {
		fs, block, depth := newFS()
		got, _, err := parseArgs(fs, tc.args)
		check(t, fmt.Sprintf("%v: error", tc.args), err, nil)
		check(t, fmt.Sprintf("%v: parsed", tc.args), []any{got, *block, *depth}, []any{tc.pos, tc.block, tc.dep})
	}
	_, typed, _ := parseArgs(flagSet("depth", "block"), []string{"-depth", "1", "x", "--block=m"})
	check(t, "typed names", typed, map[string]string{"depth": "-depth", "block": "--block"})
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--nope"}, "--nope"},
		{[]string{"x", "-nope=3"}, "-nope"},
		{[]string{"--depth"}, "--depth"},
	} {
		fs, _, _ := newFS()
		_, _, err := parseArgs(fs, tc.args)
		if err == nil || !strings.HasPrefix(err.Error(), tc.want+":") {
			t.Errorf("%v: error %v, want it to start with %s:", tc.args, err, tc.want)
		}
	}
}

func flagSet(names ...string) *flag.FlagSet {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	for _, n := range names {
		fs.String(n, "", "")
	}
	return fs
}

func TestNearMatches(t *testing.T) {
	stems := []string{"topics", "user", "people", "projects", "topic-a", "topic-b", "topic-c", "topic-d", "topic-e", "outbox-etiquette"}
	got := nearMatches("topic", stems)
	want := []string{"topics", "topic-a", "topic-b", "topic-c", "topic-d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nearMatches(topic) = %v, want %v", got, want)
	}
	if got := nearMatches("outbox", stems); !reflect.DeepEqual(got, []string{"outbox-etiquette"}) {
		t.Errorf("nearMatches(outbox) = %v, want the containing stem", got)
	}
	if got := nearMatches("peeple", stems); !reflect.DeepEqual(got, []string{"people"}) {
		t.Errorf("nearMatches(peeple) = %v, want [people]", got)
	}
	if got := nearMatches("qqqqqqqq", stems); len(got) != 0 {
		t.Errorf("nearMatches(qqqqqqqq) = %v, want none", got)
	}
}

func TestTagCounts(t *testing.T) {
	got := tagVocabulary(map[string]int{"b": 2, "a": 2, "c": 5, "d": 1})
	if want := "c(5) a(2) b(2) d(1)"; got != want {
		t.Errorf("tagVocabulary = %q, want %q", got, want)
	}
}

// A flag that takes one value is refused when given a second time, under
// either spelling and whether or not the values agree; a flag the verb
// declares repeatable is not.
func TestParseArgsRefusesARepeatedFlag(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"x", "--depth", "1", "--depth", "2"}, "--depth: given more than once"},
		{[]string{"--depth=1", "x", "-depth", "1"}, "-depth: given more than once"},
		{[]string{"-block", "m", "--block=n"}, "--block: given more than once"},
	} {
		_, _, err := parseArgs(flagSet("depth", "block"), tc.args)
		if err == nil || err.Error() != tc.want {
			t.Errorf("%v: error %v, want %q", tc.args, err, tc.want)
		}
	}
	var on bool
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.BoolVar(&on, "exit-zero", false, "")
	if _, _, err := parseArgs(fs, []string{"--exit-zero", "--exit-zero"}); err == nil {
		t.Error("a boolean flag given twice: no error")
	}
	var got []string
	fs = flag.NewFlagSet("t", flag.ContinueOnError)
	fs.Func("tag", "", func(s string) error { got = append(got, s); return nil })
	_, _, err := parseArgs(fs, []string{"--tag", "a", "-tag=b"}, "tag")
	check(t, "repeatable: error", err, nil)
	check(t, "repeatable: values", got, []string{"a", "b"})
	// After --, a repeated flag is a positional and is not counted.
	_, _, err = parseArgs(flagSet("depth"), []string{"--depth", "1", "--", "--depth"})
	check(t, "after --: error", err, nil)
}
