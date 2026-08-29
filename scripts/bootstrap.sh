#!/usr/bin/env bash
# Makes a fresh worktree buildable: the one command to run after `git worktree
# add`, before anything else (TRA-1172).
#
# Two Go files embed generated artifacts that are gitignored, so a fresh
# checkout does not compile at all:
#
#   backend/main.go                                    //go:embed frontend/dist
#   backend/internal/handlers/swaggerspec/*.go         //go:embed openapi.internal.json
#
# `just validate` in an unbootstrapped tree therefore fails with `pattern
# frontend/dist: no matching files found`, which names neither the step that is
# missing nor the command that supplies it. Before even that, an absent
# node_modules surfaces as `Cannot find package 'vite'` from a .vite-temp path.
#
# This script exists so that confusion happens once, here, with a message that
# says what to do — and so the same three commands are not retyped from memory,
# in the wrong order, in every new worktree.
#
# It is safe to re-run at any time and cheap when there is nothing to do, which
# is what makes it safe to fire reflexively (backgrounded at session start, say)
# without weighing the cost first.
#
# Run: just bootstrap  [--force]
set -uo pipefail

# Guarded rather than the usual one-liner: `cd ""` is a silent no-op in bash, so
# a failed resolution here would run every step against whatever directory the
# caller happened to be in, and report success.
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [ -z "$repo_root" ] || ! cd "$repo_root"; then
    echo "❌ bootstrap: cannot resolve the repo root from ${BASH_SOURCE[0]}" >&2
    exit 1
fi

force=0
for arg in "$@"; do
    case "$arg" in
        --force) force=1 ;;
        -h|--help)
            # The header comment is the help text. Matched structurally rather
            # than by line range so that editing the comment cannot silently
            # start printing code.
            awk 'NR>1 && /^#/ { sub(/^# ?/, ""); print; next } NR>1 { exit }' \
                "${BASH_SOURCE[0]}"
            exit 0
            ;;
        *)
            echo "❌ bootstrap: unknown argument '$arg' (expected --force)" >&2
            exit 2
            ;;
    esac
done

# The two go:embed targets, and the marker that distinguishes a real frontend
# build from the one-line stub `just backend api-spec` drops in to keep swag
# parsing (backend/justfile, TRA-505).
dist_dir="backend/frontend/dist"
dist_marker="$dist_dir/assets"
spec_dir="backend/internal/handlers/swaggerspec"
spec_files=(
    "$spec_dir/openapi.internal.json"
    "$spec_dir/openapi.internal.yaml"
    "$spec_dir/openapi.public.json"
    "$spec_dir/openapi.public.yaml"
)

# die <step> <message...> — every exit path out of this script goes through
# here. A step that fails quietly is the failure mode this whole ticket is
# about: bootstrap returning 0 having done nothing leaves `just validate` to
# report a missing embed, and leaves whatever ran in between having proved
# nothing.
die() {
    local step="$1"; shift
    echo "" >&2
    echo "❌ bootstrap: step '$step' failed" >&2
    for line in "$@"; do
        echo "   $line" >&2
    done
    echo "" >&2
    echo "   The tree is NOT bootstrapped. Do not trust a \`just validate\` run" >&2
    echo "   or any verification made against it until this is fixed." >&2
    exit 1
}

# run_step <step> <command...> — runs it, and turns a non-zero exit into die().
run_step() {
    local step="$1"; shift
    if ! "$@"; then
        die "$step" "command: $*"
    fi
}

# ---------------------------------------------------------------------------
# Preflight — name a missing tool here rather than letting a step die on
# `command not found` three minutes in.
# ---------------------------------------------------------------------------
require_tool() {
    local tool="$1" hint="$2"
    command -v "$tool" >/dev/null 2>&1 && return 0
    die "preflight" \
        "'$tool' is not on PATH." \
        "$hint"
}

require_tool pnpm "Install it: corepack enable && corepack prepare pnpm@9.12.3 --activate"
require_tool go   "Install Go 1.25+: https://go.dev/dl/"
# swag is not vendored — TRA-421 owns pinning it via tools.go. Until then the
# version is whatever is on PATH, and CI's is authoritative.
require_tool swag "Install it: go install github.com/swaggo/swag/cmd/swag@v1.16.6"

echo "🧰 Bootstrapping $(basename "$repo_root")"

# ---------------------------------------------------------------------------
# 0. Local environment files (TRA-1190)
# ---------------------------------------------------------------------------
# Two consumers, one file. docker compose reads `.env`; direnv reads
# `.env.local`. Keeping them as two real files means compose and the host-side
# tooling can disagree about which database is the database — and because the
# shell environment silently outranks `.env`, the disagreement does not even
# present the same way twice.
#
# A symlink makes them the same bytes by construction. That was already the
# arrangement on the one machine where it worked, created by hand in October
# 2025, documented nowhere and reproduced by nothing — so every other checkout
# had the split this ticket is about. Creating it here is the whole fix;
# `just test-env-drift` is what keeps it.
#
# Never overwrites a real file: if someone has a populated `.env`, that is
# content this script has not read and must not destroy. Say so and let them
# resolve it.
if [ -L "$repo_root/.env" ]; then
    target=$(readlink "$repo_root/.env")
    if [ "$target" = ".env.local" ]; then
        echo "✅ .env -> .env.local"
    else
        die "env" \
            ".env is a symlink to '$target', not to .env.local." \
            "Two env files can name two different databases (TRA-1190)." \
            "Fix: ln -sfn .env.local .env"
    fi
elif [ -e "$repo_root/.env" ]; then
    die "env" \
        ".env exists as a regular file." \
        "It must be a symlink to .env.local, so compose and direnv cannot" \
        "disagree about which database is the database (TRA-1190)." \
        "Merge anything it holds into .env.local, then:" \
        "  rm .env && ln -s .env.local .env" \
        "Left in place deliberately — this script will not delete a file whose" \
        "contents it has not read."
else
    echo "🔗 .env -> .env.local"
    ln -s .env.local "$repo_root/.env" || die "env" "could not create the .env symlink"
fi

# A worktree gets no .env.local of its own — it is gitignored, so `git worktree
# add` never brings one across, and direnv in a fresh worktree therefore finds
# nothing. Point it at the main worktree's copy so credentials are edited in one
# place and a worktree cannot drift from the checkout it was cut from.
#
# The main worktree is resolved from git rather than from `..`, for the reason
# the `ops` recipe in the root justfile gives: worktrees live in
# .claude/worktrees/<branch>/, where a relative guess lands somewhere else
# entirely.
if [ ! -e "$repo_root/.env.local" ]; then
    main_dir=$(git worktree list --porcelain 2>/dev/null \
        | awk '/^worktree /{path=$2} /^branch refs\/heads\/main$/{print path; exit}')
    if [ -n "$main_dir" ] && [ "$main_dir" != "$repo_root" ] && [ -f "$main_dir/.env.local" ]; then
        echo "🔗 .env.local -> $main_dir/.env.local"
        ln -s "$main_dir/.env.local" "$repo_root/.env.local" \
            || die "env" "could not link .env.local to the main worktree's copy"
    else
        # Not fatal: every value has a working local default, so the stack comes
        # up on the canonical database regardless. Only the things with no
        # sensible default — MQTT credentials, a Resend key — actually need it.
        echo "ℹ️  no .env.local (defaults apply; cp .env.local.example .env.local to override)"
    fi
fi

# ---------------------------------------------------------------------------
# 1. Workspace dependencies
# ---------------------------------------------------------------------------
# Unconditional: a warm `pnpm install --frozen-lockfile` is sub-second, and pnpm
# already knows precisely when it has work to do. A staleness gate here would be
# a worse copy of a check pnpm does correctly, and getting it wrong means
# skipping an install that was needed.
echo "📦 pnpm install"
run_step "install" pnpm install --frozen-lockfile

# ---------------------------------------------------------------------------
# 2. Frontend build -> backend/frontend/dist
# ---------------------------------------------------------------------------
# Before api-spec, deliberately. `just backend api-spec` creates a placeholder
# backend/frontend/dist/index.html when none exists, so running it first would
# satisfy main.go's embed with an empty page: the build goes green and the
# binary serves no frontend.
#
# Staleness is keyed on assets/, not index.html, for the same reason — a tree
# holding only that placeholder must read as unbootstrapped.
frontend_stale=1
if [ "$force" -eq 0 ] && [ -d "$dist_marker" ] && [ -f "$dist_dir/index.html" ]; then
    if [ -z "$(find frontend -newer "$dist_dir/index.html" \
                    -not -path 'frontend/node_modules/*' \
                    -not -path 'frontend/dist/*' \
                    -not -path 'frontend/test-results/*' \
                    -not -path 'frontend/playwright-report/*' \
                    -print -quit 2>/dev/null)" ]; then
        frontend_stale=0
    fi
fi

if [ "$frontend_stale" -eq 0 ]; then
    echo "✅ frontend/dist up to date — skipping build"
else
    echo "🏗️  frontend build"
    run_step "build" pnpm --filter frontend build
    [ -f "frontend/dist/index.html" ] || die "build" \
        "'pnpm --filter frontend build' exited 0 but wrote no frontend/dist/index.html."
    # main.go's embed path resolves against backend/, not the repo root, so the
    # dist has to be copied there. rm -rf first: a merge would leave last
    # build's hashed chunks behind alongside this one's.
    mkdir -p "$(dirname "$dist_dir")"
    rm -rf "$dist_dir"
    run_step "build" cp -r frontend/dist "$dist_dir"
fi

# ---------------------------------------------------------------------------
# 3. OpenAPI specs -> swaggerspec/
# ---------------------------------------------------------------------------
specs_stale=1
if [ "$force" -eq 0 ]; then
    specs_stale=0
    for f in "${spec_files[@]}"; do
        [ -f "$f" ] || { specs_stale=1; break; }
    done
    if [ "$specs_stale" -eq 0 ] && [ -n "$(find backend -name '*.go' \
            -newer "$spec_dir/openapi.internal.json" -print -quit 2>/dev/null)" ]; then
        specs_stale=1
    fi
fi

if [ "$specs_stale" -eq 0 ]; then
    echo "✅ OpenAPI specs up to date — skipping generation"
else
    echo "📚 backend api-spec"
    run_step "api-spec" just backend api-spec
fi

# ---------------------------------------------------------------------------
# Postconditions
# ---------------------------------------------------------------------------
# A step can exit 0 and produce nothing — a stale cache, a silently skipped
# generator. Checking the exit code alone would report success and hand back a
# tree that does not compile, so assert the artifacts the compiler will actually
# look for.
missing=()
[ -f "$dist_dir/index.html" ] || missing+=("$dist_dir/index.html  (embedded by backend/main.go)")
[ -d "$dist_marker" ]         || missing+=("$dist_dir/assets/  (a real build, not the api-spec placeholder)")
for f in "${spec_files[@]}"; do
    [ -f "$f" ] || missing+=("$f  (embedded by $spec_dir)")
done

if [ "${#missing[@]}" -gt 0 ]; then
    step="build"
    case "${missing[0]}" in
        *swaggerspec*) step="api-spec" ;;
    esac
    die "$step" \
        "The step reported success but these artifacts are missing:" \
        "${missing[@]}"
fi

echo "✅ Bootstrap complete — \`just validate\` should now run clean"
