#!/usr/bin/env bash
# Tests for the release helper scripts. No git, no network: each case feeds
# fixture input on stdin and compares stdout.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
fails=0

check() {
    local name=$1 want=$2 got=$3
    if [[ "$got" == "$want" ]]; then
        echo "PASS $name"
    else
        echo "FAIL $name"
        echo "  want: $(printf '%q' "$want")"
        echo "  got:  $(printf '%q' "$got")"
        fails=$((fails + 1))
    fi
}

# ── next-tag.sh ────────────────────────────────────────────────────────────

check "next-tag: no tags" "20261002" \
    "$(printf '' | "$DIR/next-tag.sh" 20261002)"

check "next-tag: only other days" "20261002" \
    "$(printf '20261001\n20260930.3\n' | "$DIR/next-tag.sh" 20261002)"

check "next-tag: second release of the day" "20261002.2" \
    "$(printf '20261001\n20261002\n' | "$DIR/next-tag.sh" 20261002)"

check "next-tag: numeric not lexical max" "20261002.11" \
    "$(printf '20261002\n20261002.2\n20261002.10\n20261002.9\n' | "$DIR/next-tag.sh" 20261002)"

check "next-tag: ignores lookalike tags" "20261002" \
    "$(printf 'v20261002\n20261002-rc\n20261002.x\n' | "$DIR/next-tag.sh" 20261002)"

if "$DIR/next-tag.sh" 2026-10-02 </dev/null >/dev/null 2>&1; then
    check "next-tag: rejects malformed date" "error" "accepted"
else
    check "next-tag: rejects malformed date" "error" "error"
fi

# ── notes.sh ───────────────────────────────────────────────────────────────

diff_added=$(cat <<'EOF'
diff --git a/CHANGELOG.md b/CHANGELOG.md
index 1111111..2222222 100644
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -3,6 +3,13 @@
 Dated entries, newest first. Each header is a unit of work; bullets capture the
 detail.

+## 2026-10-02 - CI gates merges to main
+
+- `.github/workflows/ci.yml`: lint, test, tidy, vuln, build.
+++ a bullet that happens to start with two plus signs
+- removed a typo
+
 ## 2026-09-16 - Older entry

-- old typo
+- old typo fixed
EOF
)
want_added=$(cat <<'EOF'
## 2026-10-02 - CI gates merges to main

- `.github/workflows/ci.yml`: lint, test, tidy, vuln, build.
++ a bullet that happens to start with two plus signs
- removed a typo

- old typo fixed
EOF
)
check "notes: added lines, header and removals dropped" "$want_added" \
    "$(printf '%s\n' "$diff_added" | "$DIR/notes.sh")"

diff_padded=$(cat <<'EOF'
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,2 +1,6 @@
+
+
+## Entry
+
 context
EOF
)
check "notes: leading and trailing blank lines trimmed" "## Entry" \
    "$(printf '%s\n' "$diff_padded" | "$DIR/notes.sh")"

check "notes: empty diff gives placeholder" "_No CHANGELOG changes in this release._" \
    "$(printf '' | "$DIR/notes.sh")"

diff_removal_only=$(cat <<'EOF'
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,2 +1 @@
-gone
 kept
EOF
)
check "notes: removal-only diff gives placeholder" "_No CHANGELOG changes in this release._" \
    "$(printf '%s\n' "$diff_removal_only" | "$DIR/notes.sh")"

(( fails == 0 ))
