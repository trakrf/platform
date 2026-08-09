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

# ---------------------------------------------------------------------------
echo
echo "extract-image-version.sh"
# ---------------------------------------------------------------------------
extract="$repo_root/scripts/extract-image-version.sh"

# Shapes below are trimmed from real `docker buildx imagetools inspect
# --format '{{ json .Image }}'` output, verified 2026-08-04.

# What our published images actually look like: amd64 + arm64 merged (TRA-909).
multiarch='{
  "linux/amd64": {"created":"2026-08-04T00:00:00Z","architecture":"amd64","os":"linux",
    "config":{"Labels":{"id.trakrf.app-version":"v1.3.0","org.opencontainers.image.version":"sha-aa9822b"}}},
  "linux/arm64": {"created":"2026-08-04T00:00:00Z","architecture":"arm64","os":"linux",
    "config":{"Labels":{"id.trakrf.app-version":"v1.3.0","org.opencontainers.image.version":"sha-aa9822b"}}}
}'
out=$(printf '%s' "$multiarch" | "$extract" 2>&1)
expect "reads the label from a multi-arch manifest" "v1.3.0" "$out"
if [ "$out" = "v1.3.0" ]; then
    echo "  ✓ multi-arch result is exactly the version"
    pass=$((pass + 1))
else
    echo "  ✗ multi-arch result is exactly the version (got '$out')"
    fail=$((fail + 1))
fi

# Does not confuse itself with metadata-action's key, which holds the image tag.
if [ "$out" = "sha-aa9822b" ]; then
    echo "  ✗ must not read org.opencontainers.image.version"
    fail=$((fail + 1))
else
    echo "  ✓ does not read org.opencontainers.image.version"
    pass=$((pass + 1))
fi

# The single-platform shape, for robustness.
single='{"created":"2026-08-04T00:00:00Z","architecture":"amd64","os":"linux",
  "config":{"Labels":{"id.trakrf.app-version":"v1.3.0"}}}'
out=$(printf '%s' "$single" | "$extract" 2>&1)
expect "reads the label from a single-platform image" "v1.3.0" "$out"

# An image built before this change has no such label — must come back empty so
# assert-release-version.sh fails closed rather than promoting it.
nolabel='{
  "linux/amd64": {"config":{"Labels":{"org.opencontainers.image.version":"sha-aa9822b"}}},
  "linux/arm64": {"config":{"Labels":{"org.opencontainers.image.version":"sha-aa9822b"}}}
}'
out=$(printf '%s' "$nolabel" | "$extract" 2>&1)
if [ -z "$out" ]; then
    echo "  ✓ returns empty when the label is absent"
    pass=$((pass + 1))
else
    echo "  ✗ returns empty when the label is absent (got '$out')"
    fail=$((fail + 1))
fi

# No Labels map at all.
nolabels='{"linux/amd64": {"config":{}}, "linux/arm64": {"config":{}}}'
out=$(printf '%s' "$nolabels" | "$extract" 2>&1)
if [ -z "$out" ]; then
    echo "  ✓ returns empty when there are no labels at all"
    pass=$((pass + 1))
else
    echo "  ✗ returns empty when there are no labels at all (got '$out')"
    fail=$((fail + 1))
fi

# Arches that disagree mean the version is not a fact about the image.
disagree='{
  "linux/amd64": {"config":{"Labels":{"id.trakrf.app-version":"v1.3.0"}}},
  "linux/arm64": {"config":{"Labels":{"id.trakrf.app-version":"v1.2.0"}}}
}'
out=$(printf '%s' "$disagree" | "$extract" 2>&1)
if [ -z "$out" ]; then
    echo "  ✓ returns empty when platforms disagree"
    pass=$((pass + 1))
else
    echo "  ✗ returns empty when platforms disagree (got '$out')"
    fail=$((fail + 1))
fi

# One arch labelled and one not is also a disagreement.
partial='{
  "linux/amd64": {"config":{"Labels":{"id.trakrf.app-version":"v1.3.0"}}},
  "linux/arm64": {"config":{"Labels":{}}}
}'
out=$(printf '%s' "$partial" | "$extract" 2>&1)
if [ -z "$out" ]; then
    echo "  ✓ returns empty when only one platform is labelled"
    pass=$((pass + 1))
else
    echo "  ✗ returns empty when only one platform is labelled (got '$out')"
    fail=$((fail + 1))
fi

# End to end, mirroring exactly what promote-prod.yml does: capture into a
# variable, then pass it quoted so an empty value stays one empty argument.
version=$(printf '%s' "$multiarch" | "$extract")
"$assert" "$version" >/dev/null 2>&1 && status=0 || status=$?
expect_status "chained extract -> assert accepts a released image" 0 "$status"

version=$(printf '%s' "$nolabel" | "$extract")
"$assert" "$version" >/dev/null 2>&1 && status=0 || status=$?
expect_status "chained extract -> assert refuses an unlabelled image" 1 "$status"

version=$(printf '%s' "$disagree" | "$extract")
"$assert" "$version" >/dev/null 2>&1 && status=0 || status=$?
expect_status "chained extract -> assert refuses a disagreeing manifest" 1 "$status"

# A dev-shaped version reaching the label is still refused end to end — this is
# the v1.3.0 near-miss in full.
devbuild='{
  "linux/amd64": {"config":{"Labels":{"id.trakrf.app-version":"v1.2.0-559-gaa9822bb"}}},
  "linux/arm64": {"config":{"Labels":{"id.trakrf.app-version":"v1.2.0-559-gaa9822bb"}}}
}'
version=$(printf '%s' "$devbuild" | "$extract")
"$assert" "$version" >/dev/null 2>&1 && status=0 || status=$?
expect_status "chained extract -> assert refuses a merge-then-tag image" 1 "$status"

# ---------------------------------------------------------------------------
echo
echo "build-version.sh"
# ---------------------------------------------------------------------------
bv="$repo_root/scripts/build-version.sh"

out=$("$bv" 2>&1) && status=0 || status=$?
expect_status "reads the committed VERSION file" 0 "$status"
expect "prefixes the declared version with v" "v$(cat "$repo_root/VERSION")" "$out"

out=$("$bv" "-preview+419+420" 2>&1)
expect "appends the suffix verbatim" "v$(cat "$repo_root/VERSION")-preview+419+420" "$out"

# Run from elsewhere: CI and the local backend build both invoke it with a cwd
# that is not guaranteed to be the repo root.
out=$(cd / && "$bv" 2>&1)
expect "resolves VERSION relative to the script, not the cwd" "v$(cat "$repo_root/VERSION")" "$out"

# A VERSION file that already carries the v is the mistake this catches: the
# derivation layer owns the prefix, so it would produce `vv1.5.0`.
bvfix=$(mktemp -d)
mkdir -p "$bvfix/scripts"
cp "$bv" "$bvfix/scripts/build-version.sh"

printf 'v1.5.0\n' > "$bvfix/VERSION"
out=$("$bvfix/scripts/build-version.sh" 2>&1) && status=0 || status=$?
expect_status "refuses a VERSION carrying a leading v" 1 "$status"
expect "explains that VERSION is bare semver" "bare semver" "$out"

printf '1.5\n' > "$bvfix/VERSION"
out=$("$bvfix/scripts/build-version.sh" 2>&1) && status=0 || status=$?
expect_status "refuses a two-component VERSION" 1 "$status"

printf '\n' > "$bvfix/VERSION"
out=$("$bvfix/scripts/build-version.sh" 2>&1) && status=0 || status=$?
expect_status "refuses an empty VERSION" 1 "$status"

printf '1.5.0-dev\n' > "$bvfix/VERSION"
out=$("$bvfix/scripts/build-version.sh" 2>&1) && status=0 || status=$?
expect_status "accepts a -dev prerelease" 0 "$status"
expect "keeps the prerelease suffix" "v1.5.0-dev" "$out"

printf '1.5.0\n' > "$bvfix/VERSION"
out=$("$bvfix/scripts/build-version.sh" 2>&1)
expect "accepts a clean release version" "v1.5.0" "$out"
rm -rf "$bvfix"

# ---------------------------------------------------------------------------
echo
echo "resolve-release-action.sh"
# ---------------------------------------------------------------------------
action="$repo_root/scripts/resolve-release-action.sh"

# Reuse the fixture repo built for resolve-promote-source.sh: main has two
# commits and v1.3.0 sits on the tip.
tip=$(git -C "$fixture" rev-parse main)
prev=$(git -C "$fixture" rev-parse main~1)

out=$(cd "$fixture" && "$action" 1.5.0-dev "$tip" 2>&1) && status=0 || status=$?
expect_status "a -dev version is not a release" 0 "$status"
expect "reports no action for -dev" "none" "$out"

out=$(cd "$fixture" && "$action" 1.4.0 "$tip" 2>&1) && status=0 || status=$?
expect_status "an untagged clean version mints a tag" 0 "$status"
expect "reports create" "create" "$out"

out=$(cd "$fixture" && "$action" 1.3.0 "$tip" 2>&1) && status=0 || status=$?
expect_status "an existing tag at this commit is a no-op" 0 "$status"
expect "reports skip" "skip" "$out"

# The Q3 stale-window case: main keeps moving while VERSION is still clean, so
# a later commit re-runs the release job against a tag that already exists
# somewhere else. That is a version reuse and must be loud.
out=$(cd "$fixture" && "$action" 1.3.0 "$prev" 2>&1) && status=0 || status=$?
expect_status "refuses a version already tagged at a different commit" 1 "$status"
expect "names the version being reused" "v1.3.0" "$out"
expect "says the tag is elsewhere" "different commit" "$out"

out=$(cd "$fixture" && "$action" "not-a-version" "$tip" 2>&1) && status=0 || status=$?
expect_status "a malformed version is not a release" 0 "$status"
expect "reports none for a malformed version" "none" "$out"

out=$(cd "$fixture" && "$action" 1.4.0 "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" 2>&1) && status=0 || status=$?
expect_status "refuses an unresolvable commit" 1 "$status"

echo
echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
