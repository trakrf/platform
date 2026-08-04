#!/usr/bin/env bash
# Tests for the release guard scripts (TRA-1085).
#
# Both guards are pure — no registry, no network, no cluster, no docker. The
# resolution guard runs against a throwaway git repo built here, so nothing
# depends on the state of the checkout the tests happen to run in.
#
# Run: just test-release-guards
set -uo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

pass=0
fail=0

# expect <name> <expected-substring> <actual-output>
expect() {
    local name="$1" want="$2" got="$3"
    if [[ "$got" == *"$want"* ]]; then
        echo "  ✓ $name"
        pass=$((pass + 1))
    else
        echo "  ✗ $name"
        echo "      want substring: $want"
        echo "      got:            ${got//$'\n'/\\n}"
        fail=$((fail + 1))
    fi
}

# expect_status <name> <expected-status> <actual-status>
expect_status() {
    local name="$1" want="$2" got="$3"
    if [ "$want" = "$got" ]; then
        echo "  ✓ $name"
        pass=$((pass + 1))
    else
        echo "  ✗ $name (want exit $want, got exit $got)"
        fail=$((fail + 1))
    fi
}

# ---------------------------------------------------------------------------
echo "assert-release-version.sh"
# ---------------------------------------------------------------------------
assert="$repo_root/scripts/assert-release-version.sh"

"$assert" v1.3.0 >/dev/null 2>&1
expect_status "accepts v1.3.0" 0 $?

"$assert" v10.20.30 >/dev/null 2>&1
expect_status "accepts multi-digit components" 0 $?

# The exact string that would have shipped as v1.3.0 — merge-then-tag bakes a
# `git describe` dev version into the image permanently (TRA-1085 item 1).
"$assert" v1.2.0-559-gaa9822bb >/dev/null 2>&1
expect_status "refuses a git-describe dev version" 1 $?

"$assert" v1.1.1-3-gdeadbee-preview+419+420 >/dev/null 2>&1
expect_status "refuses a preview composition version" 1 $?

"$assert" dev >/dev/null 2>&1
expect_status "refuses the Dockerfile's 'dev' fallback" 1 $?

"$assert" sha-aa9822b >/dev/null 2>&1
expect_status "refuses an image tag mistaken for a version" 1 $?

"$assert" 1.3.0 >/dev/null 2>&1
expect_status "refuses a version with no leading v" 1 $?

"$assert" v1.3 >/dev/null 2>&1
expect_status "refuses a two-component version" 1 $?

"$assert" v1.3.0-rc1 >/dev/null 2>&1
expect_status "refuses a pre-release suffix" 1 $?

"$assert" v1.3.0-dirty >/dev/null 2>&1
expect_status "refuses a dirty-tree version" 1 $?

# A missing label reads back as an empty string. Fail closed.
"$assert" "" >/dev/null 2>&1
expect_status "refuses an empty version (missing label fails closed)" 1 $?

"$assert" >/dev/null 2>&1
expect_status "refuses a missing argument" 1 $?

out=$("$assert" v1.2.0-559-gaa9822bb 2>&1)
expect "names the offending version" "v1.2.0-559-gaa9822bb" "$out"
expect "explains the likely cause" "before the release tag existed" "$out"

out=$("$assert" v1.3.0 2>&1)
expect "echoes the accepted version" "v1.3.0" "$out"

# ---------------------------------------------------------------------------
echo
echo "resolve-promote-source.sh"
# ---------------------------------------------------------------------------
resolve="$repo_root/scripts/resolve-promote-source.sh"

# A throwaway repo: main with two commits, a release tag on the tip, and a side
# branch that is NOT an ancestor of main (standing in for `preview`, which is
# force-rewritten and must never be promotable).
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
(
    cd "$fixture" || exit 1
    git init -q -b main .
    git config user.email t@example.com
    git config user.name Test
    git commit -q --allow-empty -m first
    git commit -q --allow-empty -m second
    git tag v1.3.0
    git checkout -q -b sidebranch
    git commit -q --allow-empty -m "not on main"
    git checkout -q main
    # promote-prod runs against a real remote; emulate origin/* locally.
    git update-ref refs/remotes/origin/main refs/heads/main
    git update-ref refs/remotes/origin/sidebranch refs/heads/sidebranch
) >/dev/null 2>&1

main_sha=$(git -C "$fixture" rev-parse main)
main_short="sha-${main_sha:0:7}"

out=$(cd "$fixture" && "$resolve" 2>&1) && status=0 || status=$?
expect_status "defaults to origin/main HEAD" 0 "$status"
expect "default resolves to the 7-char sha tag" "$main_short" "$out"

out=$(cd "$fixture" && "$resolve" "" 2>&1) && status=0 || status=$?
expect_status "treats an empty argument as the default" 0 "$status"
expect "empty argument resolves to origin/main" "$main_short" "$out"

out=$(cd "$fixture" && "$resolve" v1.3.0 2>&1) && status=0 || status=$?
expect_status "accepts a release tag" 0 "$status"
expect "release tag resolves to its commit's image tag" "$main_short" "$out"

out=$(cd "$fixture" && "$resolve" main 2>&1) && status=0 || status=$?
expect_status "accepts a branch name" 0 "$status"
expect "branch resolves to the 7-char sha tag" "$main_short" "$out"

# The v1.3.0 failure in one assertion: `git rev-parse --short` returned 8
# characters, docker/metadata-action publishes 7.
out=$(cd "$fixture" && "$resolve" "$main_sha" 2>&1) && status=0 || status=$?
expect_status "accepts a full sha" 0 "$status"
expect "full sha is truncated to 7, not 8" "$main_short" "$out"
if [ "$out" = "sha-${main_sha:0:8}" ]; then
    echo "  ✗ truncates to 7 characters, not 8"
    fail=$((fail + 1))
else
    echo "  ✓ truncates to 7 characters, not 8"
    pass=$((pass + 1))
fi

# Backwards compatibility: the old input contract passed the image tag directly.
out=$(cd "$fixture" && "$resolve" sha-abc1234 2>&1) && status=0 || status=$?
expect_status "passes an existing sha- image tag through" 0 "$status"
expect "sha- tag unchanged" "sha-abc1234" "$out"

out=$(cd "$fixture" && "$resolve" latest 2>&1) && status=0 || status=$?
expect_status "passes latest through" 0 "$status"
expect "latest unchanged" "latest" "$out"

# The guarantee the old input regex provided has to survive now that arbitrary
# refs are accepted: only main-derived images may reach :prod.
out=$(cd "$fixture" && "$resolve" sidebranch 2>&1) && status=0 || status=$?
expect_status "refuses a ref that is not an ancestor of origin/main" 1 "$status"
expect "says why it refused" "not an ancestor" "$out"

out=$(cd "$fixture" && "$resolve" no-such-ref 2>&1) && status=0 || status=$?
expect_status "refuses an unresolvable ref" 1 "$status"
expect "names the unresolvable ref" "no-such-ref" "$out"

echo
echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
