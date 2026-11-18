package main

import (
	"flag"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestDotenvValue(t *testing.T) {
	cases := []struct {
		name, data, want string
		found            bool
	}{
		{"plain", "AGENT_STATE_DIR=state\n", "state", true},
		{"double quotes", `AGENT_STATE_DIR="a b"`, "a b", true},
		{"single quotes", `AGENT_STATE_DIR='a b'`, "a b", true},
		{"quoted, then a comment", `AGENT_STATE_DIR="a # b" # comment`, "a # b", true},
		{"inline comment", "AGENT_STATE_DIR=state # comment", "state", true},
		{"hash inside a word is text", "AGENT_STATE_DIR=st#ate", "st#ate", true},
		{"export prefix", "export AGENT_STATE_DIR=state", "state", true},
		{"spaces around", "  AGENT_STATE_DIR = state  ", "state", true},
		{"CRLF", "A=1\r\nAGENT_STATE_DIR=state\r\n", "state", true},
		{"comment line", "# AGENT_STATE_DIR=no\n", "", false},
		{"other key", "AGENT_STATE_DIRX=no\nXAGENT_STATE_DIR=no\n", "", false},
		{"no equals", "AGENT_STATE_DIR\n", "", false},
		{"empty value", "AGENT_STATE_DIR=\n", "", true},
		{"first wins", "AGENT_STATE_DIR=one\nAGENT_STATE_DIR=two\n", "one", true},
		{"export with a tab is not a prefix", "export\tAGENT_STATE_DIR=no\n", "", false},
		{"a leading # is kept", "AGENT_STATE_DIR=#state", "#state", true},
		{"tab comment", "AGENT_STATE_DIR=state\t# comment", "state", true},
		{"unterminated quote: the unquoted rule", `AGENT_STATE_DIR="state # comment`, `"state`, true},
		{"unterminated quote, tab comment", "AGENT_STATE_DIR='st ate\t# comment", "'st ate", true},
	}
	for _, tc := range cases {
		got, found := dotenvValue(tc.data, stateDirKey)
		if got != tc.want || found != tc.found {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, found, tc.want, tc.found)
		}
	}
}

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
