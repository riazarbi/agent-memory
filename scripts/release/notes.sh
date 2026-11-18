#!/usr/bin/env bash
# Turn a unified diff of CHANGELOG.md (on stdin) into release notes: the lines
# the diff adds, with the leading '+' stripped. File headers, context and
# removed lines are dropped; surrounding blank lines are trimmed.
#
#   git diff "$PREV_TAG" HEAD -- CHANGELOG.md | scripts/release/notes.sh
set -euo pipefail

# Only lines after the first hunk header count, so the '+++ b/CHANGELOG.md'
# file header is skipped but an added line that itself starts with '++' is not.
notes=$(awk '/^@@/ { h = 1; next } h && /^\+/ { print substr($0, 2) }' | sed -e '/[^[:space:]]/,$!d')

if [[ -z "$notes" ]]; then
    echo "_No CHANGELOG changes in this release._"
else
    printf '%s\n' "$notes"
fi
