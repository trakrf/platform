#!/usr/bin/env bash
# Tests for scripts/bootstrap.sh — the fresh-worktree setup recipe (TRA-1172).
#
# The failure this file exists to catch is bootstrap failing QUIETLY. If a step
# dies and bootstrap still exits 0, the next thing anyone runs is `just
# validate`, which fails with `pattern frontend/dist: no matching files found`
# and no hint that setup is why. Worse, on 2026-08-23 an unbootstrapped tree
# turned a deliberate-break verification into a false pass: the check recorded
# the failure it expected, but would have recorded it for any input, because
# nothing was installed. A bootstrap that fails in the direction of success
# invalidates whatever ran after it.
#
# So the assertions here are mostly about the failure paths, not the happy one.
#
# Everything runs against a throwaway fake repo with stub pnpm/just/swag/go on
# PATH — no network, no Go toolchain, no real vite build. That keeps it in `just
# test` and therefore in CI, where the real thing cannot run cheaply.
#
# Run: just test-bootstrap
set -uo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
bootstrap="$repo_root/scripts/bootstrap.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

pass=0
fail=0

# ok <name> <status>
ok() {
    local name="$1" status="$2"
    if [ "$status" -eq 0 ]; then
        echo "  ✓ $name"
        pass=$((pass + 1))
    else
        echo "  ✗ $name"
        fail=$((fail + 1))
    fi
}

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

# expect_not <name> <unwanted-substring> <actual-output>
expect_not() {
    local name="$1" unwanted="$2" got="$3"
    if [[ "$got" != *"$unwanted"* ]]; then
        echo "  ✓ $name"
        pass=$((pass + 1))
    else
        echo "  ✗ $name"
        echo "      unwanted substring: $unwanted"
        echo "      got:                ${got//$'\n'/\\n}"
        fail=$((fail + 1))
    fi
}

# ---------------------------------------------------------------------------
# Fake repo + stub toolchain
# ---------------------------------------------------------------------------
# Each stub appends its argv to $TRACE and then does what the real command does
# to the tree, so the assertions can check both what ran and in what order.
#
# STUB_FAIL names a step that should exit non-zero; STUB_NOOP names one that
# should exit 0 while producing nothing, which is how the postcondition check
# gets exercised.

make_repo() {
    local dir="$1"
    rm -rf "$dir"
    mkdir -p "$dir/scripts" "$dir/frontend/src" "$dir/backend/internal/handlers/swaggerspec" "$dir/bin"
    cp "$bootstrap" "$dir/scripts/bootstrap.sh"
    chmod +x "$dir/scripts/bootstrap.sh"
    printf 'lockfile\n' > "$dir/pnpm-lock.yaml"
    printf 'package main\n' > "$dir/backend/main.go"
    printf 'export const x = 1\n' > "$dir/frontend/src/app.ts"

    cat > "$dir/bin/pnpm" <<'STUB'
#!/usr/bin/env bash
echo "pnpm $*" >> "$TRACE"
if [ "${STUB_FAIL:-}" = "${1:-}" ] || { [ "$1" = "--filter" ] && [ "${STUB_FAIL:-}" = "build" ]; }; then
    echo "stub pnpm: forced failure" >&2
    exit 3
fi
# `pnpm install`
if [ "$1" = "install" ]; then
    mkdir -p "$REPO/node_modules"
    : > "$REPO/node_modules/.modules.yaml"
    exit 0
fi
# `pnpm --filter frontend build`
if [ "$1" = "--filter" ]; then
    [ "${STUB_NOOP:-}" = "build" ] && exit 0
    mkdir -p "$REPO/frontend/dist/assets"
    : > "$REPO/frontend/dist/index.html"
    : > "$REPO/frontend/dist/assets/index.js"
fi
exit 0
STUB

    cat > "$dir/bin/just" <<'STUB'
#!/usr/bin/env bash
echo "just $*" >> "$TRACE"
if [ "${STUB_FAIL:-}" = "api-spec" ]; then
    echo "stub just: forced failure" >&2
    exit 4
fi
[ "${STUB_NOOP:-}" = "api-spec" ] && exit 0
spec="$REPO/backend/internal/handlers/swaggerspec"
mkdir -p "$spec"
for f in openapi.internal.json openapi.internal.yaml openapi.public.json openapi.public.yaml; do
    : > "$spec/$f"
done
exit 0
STUB

    printf '#!/usr/bin/env bash\necho "swag $*" >> "$TRACE"\nexit 0\n' > "$dir/bin/swag"
    printf '#!/usr/bin/env bash\necho "go $*" >> "$TRACE"\nexit 0\n' > "$dir/bin/go"
    chmod +x "$dir/bin"/*
}

# run <repo-dir> [args...] — bootstrap with the stub toolchain on PATH.
# Sets $status, $trace and $err (stderr only). Deliberately NOT a command
# substitution at the call site: that runs in a subshell, so $status would never
# make it back and every exit-code assertion would silently read a stale value.
run() {
    local dir="$1"; shift
    : > "$dir/trace.log"
    (
        cd "$dir" && REPO="$dir" TRACE="$dir/trace.log" \
            PATH="$dir/bin:/usr/bin:/bin" \
            ./scripts/bootstrap.sh "$@" >"$dir/out.log" 2>"$dir/err.log"
    )
    status=$?
    trace=$(cat "$dir/trace.log")
    # Loud failure means specifically on stderr: a script that complains on
    # stdout passes a combined-output check while any wrapper redirecting
    # stdout still swallows it.
    err=$(cat "$dir/err.log")
}

echo "bootstrap — a bare tree"
# ---------------------------------------------------------------------------
repo="$tmp/bare"
make_repo "$repo"
run "$repo"
ok "exits 0" $status
expect "installs workspace deps"       "pnpm install"            "$trace"
expect "builds the frontend"           "pnpm --filter frontend"  "$trace"
expect "generates the OpenAPI specs"   "just backend api-spec"   "$trace"

# Ordering is load-bearing, not cosmetic. `just backend api-spec` stubs
# backend/frontend/dist/index.html when the real build has not run yet
# (backend/justfile, TRA-505). Run it first and the tree compiles, embeds a
# one-line stub, and serves no frontend — green, and wrong.
frontend_line=$(grep -n 'pnpm --filter frontend' "$repo/trace.log" | head -1 | cut -d: -f1)
apispec_line=$(grep -n 'just backend api-spec' "$repo/trace.log" | head -1 | cut -d: -f1)
[ -n "$frontend_line" ] && [ -n "$apispec_line" ] && [ "$frontend_line" -lt "$apispec_line" ]
ok "frontend build runs BEFORE api-spec (else api-spec's stub dist is what ships)" $?

# main.go's //go:embed frontend/dist resolves against backend/, so a dist built
# at the repo root is not where the compiler looks. CI copies it explicitly;
# so must this.
[ -f "$repo/backend/frontend/dist/index.html" ]
ok "syncs frontend/dist -> backend/frontend/dist (the go:embed path)" $?
[ -d "$repo/backend/frontend/dist/assets" ]
ok "syncs the whole dist, not just index.html" $?
[ -f "$repo/backend/internal/handlers/swaggerspec/openapi.internal.json" ]
ok "leaves the swaggerspec embed target in place" $?

echo
echo "bootstrap — re-running on a warm tree"
# ---------------------------------------------------------------------------
# Cheap-when-satisfied is what makes this safe to run reflexively, including
# from a session-start hook.
run "$repo"
ok "exits 0" $status
expect "re-installs deps (warm pnpm install is sub-second)" "pnpm install" "$trace"
expect_not "skips the frontend build"  "pnpm --filter frontend" "$trace"
expect_not "skips api-spec"            "just backend api-spec"  "$trace"

echo
echo "bootstrap --force — ignores the staleness gates"
# ---------------------------------------------------------------------------
run "$repo" --force
ok "exits 0" $status
expect "rebuilds the frontend" "pnpm --filter frontend" "$trace"
expect "regenerates the specs" "just backend api-spec"  "$trace"

echo
echo "bootstrap — staleness"
# ---------------------------------------------------------------------------
touch "$repo/frontend/src/app.ts"
run "$repo"
expect "a newer frontend source triggers a rebuild" "pnpm --filter frontend" "$trace"

touch "$repo/backend/main.go"
run "$repo"
expect "a newer backend .go triggers a spec regen" "just backend api-spec" "$trace"

echo
echo "bootstrap — a stubbed dist reads as unbootstrapped"
# ---------------------------------------------------------------------------
# api-spec writes a one-line backend/frontend/dist/index.html so swag can parse
# main.go. Someone who ran `just backend api-spec` by hand has that file and
# nothing else. Gating on index.html alone would call that tree warm and skip
# the build — leaving an empty frontend embedded, which looks like success.
repo="$tmp/stubdist"
make_repo "$repo"
mkdir -p "$repo/backend/frontend/dist"
echo "stub" > "$repo/backend/frontend/dist/index.html"
run "$repo"
ok "exits 0" $status
expect "rebuilds over the stub" "pnpm --filter frontend" "$trace"
[ -d "$repo/backend/frontend/dist/assets" ]
ok "the stub is replaced by a real dist" $?

echo
echo "bootstrap — a failing step is loud and fatal"
# ---------------------------------------------------------------------------
for step in install build api-spec; do
    repo="$tmp/fail-$step"
    make_repo "$repo"
    STUB_FAIL="$step" run "$repo"
    [ "$status" -ne 0 ]
    ok "'$step' failing exits non-zero" $?
    expect "'$step' failing names the step on stderr" "$step" "$err"
    expect "'$step' failing says bootstrap is what failed" "bootstrap" "$err"
done

echo
echo "bootstrap — a step that exits 0 without its artifact still fails"
# ---------------------------------------------------------------------------
# The quietest failure of all: a command that returns success and writes
# nothing. Trusting the exit code alone would hand back a tree that cannot
# compile, having reported that setup worked.
for step in build api-spec; do
    repo="$tmp/noop-$step"
    make_repo "$repo"
    STUB_NOOP="$step" run "$repo"
    [ "$status" -ne 0 ]
    ok "'$step' producing nothing exits non-zero" $?
    expect "'$step' producing nothing names the missing artifact" "$step" "$err"
done

echo
echo "bootstrap — a missing tool is named before anything runs"
# ---------------------------------------------------------------------------
# `swag` is not vendored (TRA-421 owns pinning it). Without this preflight the
# failure surfaces deep inside api-spec as `swag: command not found`, which
# reads like a repo bug rather than a missing install.
repo="$tmp/noswag"
make_repo "$repo"
rm "$repo/bin/swag"
run "$repo"
[ "$status" -ne 0 ]
ok "missing swag exits non-zero" $?
expect "missing swag is named"            "swag"       "$err"
expect "missing swag says how to get it"  "go install" "$err"
[ ! -d "$repo/node_modules" ]
ok "preflight runs before any step does work" $?

echo
echo "justfile wiring"
# ---------------------------------------------------------------------------
# Against `just --summary`, not a grep for `^bootstrap:` — the recipe takes
# `*ARGS`, so a text match on the bare name would have to encode the signature
# and would go stale the moment it changed. This also fails if the justfile
# stops parsing.
recipes=$(cd "$repo_root" && just --summary 2>/dev/null)
[[ " $recipes " == *" bootstrap "* ]]
ok "root justfile exposes 'bootstrap'" $?
[[ " $recipes " == *" test-bootstrap "* ]]
ok "root justfile exposes 'test-bootstrap'" $?
grep -Eq '^test:.*test-bootstrap' "$repo_root/justfile"
ok "test-bootstrap is in the 'just test' chain" $?
[ -x "$repo_root/scripts/bootstrap.sh" ]
ok "scripts/bootstrap.sh is executable" $?

echo
echo "docs name it as the first step"
# ---------------------------------------------------------------------------
# Acceptance criterion: someone landing in a fresh worktree has to be able to
# find this without being told.
grep -q 'just bootstrap' "$repo_root/CLAUDE.md"
ok "CLAUDE.md names 'just bootstrap'" $?
grep -q 'just bootstrap' "$repo_root/README.md"
ok "README.md names 'just bootstrap'" $?

echo
echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
