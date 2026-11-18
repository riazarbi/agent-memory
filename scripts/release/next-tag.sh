#!/usr/bin/env bash
# Print the release tag for DATE (YYYYMMDD), given existing tags on stdin.
# The first release of a day is DATE; later ones are DATE.2, DATE.3, ...
#
#   git tag --list | scripts/release/next-tag.sh "$(date -u +%Y%m%d)"
set -euo pipefail

date=${1:-}
if [[ ! "$date" =~ ^[0-9]{8}$ ]]; then
    echo "usage: $0 YYYYMMDD < existing-tags" >&2
    exit 2
fi

awk -v d="$date" '
    $0 == d { if (n < 1) n = 1; next }
    index($0, d ".") == 1 {
        s = substr($0, length(d) + 2)
        if (s ~ /^[0-9]+$/ && s + 0 > n) n = s + 0
    }
    END { if (n == 0) print d; else print d "." (n + 1) }
'
