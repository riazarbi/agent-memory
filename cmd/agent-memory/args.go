package main

import (
	"flag"
	"fmt"
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
// Errors name the flag as typed (--depth, -depth), never the flag package's
// rewritten form. typed maps each flag set to the name it was typed as.
func parseArgs(fs *flag.FlagSet, args []string) (positional []string, typed map[string]string, err error) {
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
			typed[strings.TrimLeft(token, "-")] = token
			i += consumed
		}
	}
	return positional, typed, nil
}

// setFlag sets the flag arg names, from its =value or else from the first of
// rest. It returns the flag as typed and how many of rest it consumed.
func setFlag(fs *flag.FlagSet, arg string, rest []string) (token string, consumed int, err error) {
	token, value, hasValue := strings.Cut(arg, "=")
	name := strings.TrimPrefix(token[1:], "-")
	if name == "" || name[0] == '-' || fs.Lookup(name) == nil {
		return token, 0, fmt.Errorf("%s: unknown flag", token)
	}
	if !hasValue {
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
