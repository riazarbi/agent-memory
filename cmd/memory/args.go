package main

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// endOfFlags ends the flags: every argument after it is positional.
const endOfFlags = "--"

// parseArgs parses args against fs, with flags and positionals in any order
// (read STEM --depth 2 and read --depth 2 STEM alike). Go's fs.Parse stops at
// the first positional, so this walks the arguments itself and uses fs only
// for the flag definitions and their values:
//
//   - an argument starting with - (other than - alone) is a flag, -name or
//     --name, its value either after = or in the next argument (taken
//     whatever it looks like, as fs.Parse does: --depth -1 is a value);
//   - -- ends the flags, and everything after it is positional;
//   - anything else is positional, in order.
//
// A flag given a second time is an error naming the second spelling, unless
// its name is in repeatable (write's --tag, split's --block): taking the last
// value silently would run a call the caller did not write.
//
// Errors name the flag as typed (--depth, -depth), never the flag package's
// rewritten form. typed maps each flag set to the name it was typed as.
func parseArgs(fs *flag.FlagSet, args []string, repeatable ...string) (positional []string, typed map[string]string, err error) {
	typed = map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == endOfFlags:
			return append(positional, args[i+1:]...), typed, nil
		case len(arg) < 2 || arg[0] != '-':
			positional = append(positional, arg)
		default:
			token, consumed, err := setFlag(fs, arg, args[i+1:])
			if err != nil {
				return nil, nil, err
			}
			name := strings.TrimLeft(token, "-")
			if _, again := typed[name]; again && !slices.Contains(repeatable, name) {
				return nil, nil, fmt.Errorf("%s: given more than once", token)
			}
			typed[name] = token
			i += consumed
		}
	}
	return positional, typed, nil
}

// setFlag sets the flag arg names, from its =value or else from the first of
// rest. A boolean flag (lint's --exit-zero) takes no value: given, it is
// on, and an =value is an error, so it never consumes the next argument.
// It returns the flag as typed and how many of rest it consumed.
func setFlag(fs *flag.FlagSet, arg string, rest []string) (token string, consumed int, err error) {
	token, value, hasValue := strings.Cut(arg, "=")
	name := strings.TrimPrefix(token[1:], "-")
	if name == "" || name[0] == '-' || fs.Lookup(name) == nil {
		return token, 0, fmt.Errorf("%s: unknown flag", token)
	}
	if b, ok := fs.Lookup(name).Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
		if hasValue {
			return token, 0, fmt.Errorf("%s: takes no value", token)
		}
		value = "true"
	} else if !hasValue {
		if len(rest) == 0 {
			return token, 0, fmt.Errorf("%s: needs a value", token)
		}
		value, consumed = rest[0], 1
	}
	if err := fs.Set(name, value); err != nil {
		return token, 0, fmt.Errorf("%s: invalid value %q: %w", token, value, err)
	}
	return token, consumed, nil
}

// noArgs refuses any argument to a verb that takes none; a lone -- (the
// end of flags) is allowed.
func noArgs(verb string, args []string) error {
	flags := flag.NewFlagSet(verb, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pos, _, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf("%s takes no arguments, got %q", verb, pos[0])
	}
	return nil
}
