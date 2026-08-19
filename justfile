# TrakRF Platform - Task Runner
# https://just.systems/

# Make a recipe's arguments available to its body as "$@" with word boundaries
# intact. `{{ ARGS }}` substitutes textually, so the shell re-splits a quoted
# argument on whitespace — which silently broke `just psql prod "SELECT ..."`
# (TRA-1105). The workspace delegation and infra passthrough recipes below both
# rely on this.
set positional-arguments

# List all available recipes
default:
    @just --list

# ============================================================================
# Workspace Delegation
# ============================================================================
# Delegate commands to workspace justfiles
# Usage: just <workspace> <command> [args...]
# Example: just frontend dev, just backend test
#
# "$@" rather than {{ args }}, for the reason given at the top of this file: a
# quoted argument such as `just backend test -run "TestFoo Bar"` must reach the
# workspace as one word (TRA-1105).

frontend *args:
    cd frontend && just "$@"

backend *args:
    cd backend && just "$@"

cli *args:
    cd cli && just "$@"

database *args:
    cd database && just "$@"

# ============================================================================
# Lazy Dev Aliases
# ============================================================================

alias db := database
alias fe := frontend
alias be := backend

# ============================================================================
# Combined Validation Commands
# ============================================================================
# Run checks across all workspaces

lint: (frontend "lint") (backend "lint") (cli "lint")

test: test-ops test-release-guards test-db-init (frontend "test") (backend "test") (cli "test")

build: (frontend "build") (backend "build") (cli "build")

validate: lint test build

# Alias for CSW integration
check: validate

# TRA-671: Run Schemathesis contract tests (see backend/justfile for details)
test-contract: (backend "test-contract")

# ============================================================================
# Infra Ops Passthrough (TRA-1053)
# ============================================================================
# Cluster, namespace, pod and CNPG knowledge lives in trakrf/infra. These
# recipes only forward to its justfile — they never restate any of it, and they
# deliberately do not mirror infra's recipe names, so infra can add, rename or
# re-signature a recipe without platform going stale.
#
# `just --justfile <path>` runs the delegated recipe with its working directory
# set to the infra checkout, so infra's own relative paths (`source
# scripts/ops-lib.sh`) keep working with no cd or path rewriting here.
#
# Nothing below is evaluated until a recipe is invoked: `just dev` and
# `just test` never need gcloud, kubectl or an infra checkout.

# Run an infra ops recipe (`just ops logs prod 1h`); bare `just ops` lists them
ops *ARGS:
    #!/usr/bin/env bash
    set -euo pipefail
    infra_dir="${TRAKRF_INFRA_DIR:-}"
    if [ -z "$infra_dir" ]; then
        # Resolve against the MAIN worktree, not this one: platform worktrees
        # live in .claude/worktrees/<branch>/, where ../infra would resolve to
        # .claude/worktrees/infra.
        main_dir=$(git worktree list --porcelain 2>/dev/null \
            | awk '/^worktree /{path=$2} /^branch refs\/heads\/main$/{print path; exit}')
        [ -n "$main_dir" ] || main_dir="{{ justfile_directory() }}"
        infra_dir="$(dirname "$main_dir")/infra"
    fi
    if [ ! -f "$infra_dir/justfile" ]; then
        echo "ERROR: no infra checkout at $infra_dir" >&2
        echo "       Set TRAKRF_INFRA_DIR to your trakrf/infra checkout." >&2
        echo "       See .env.local.example — .envrc loads .env.local." >&2
        exit 1
    fi
    # "$@" rather than {{ ARGS }}: infra's `psql ENV QUERY=""` takes a whole SQL
    # statement as one argument, and textual interpolation would re-split it on
    # whitespace before infra ever sees it (TRA-1105).
    just --justfile "$infra_dir/justfile" "$@"

# Authenticate to GCP and point kubectl at the cluster (no-op if already valid)
gcp-auth *ARGS:
    @just ops gcp-auth "$@"

# psql on a CNPG primary as the non-superuser `trakrf-migrate` role (TRA-1105):
# `just psql preview` for a shell, `just psql prod "SELECT 1;"` for a one-off.
# A superuser session is a deliberate opt-in: `just ops psql-super ENV [QUERY]`.
psql *ARGS:
    @just ops psql "$@"

# Follow backend logs: `just logs preview`, `just logs prod 1h`
logs *ARGS:
    @just ops logs "$@"

# Test the passthrough against a stub infra justfile (no cluster access needed)
test-ops:
    @./scripts/test-ops-passthrough.sh

# TRA-1085 / TRA-1126: the release guards — VERSION derivation, source-ref
# resolution, the clean-vX.Y.Z check, the tag-commit binding and the release
# tag decision. Pure bash, no registry or cluster needed.
test-release-guards:
    @./scripts/test-release-guards.sh

# TRA-1085 item 4 — a clean VERSION requires its CHANGELOG.md section. Inert
# during development (VERSION carries -dev). Run in CI inside `lint-test` so it
# needs no new required-check context.
check-changelog:
    @./scripts/assert-changelog-section.sh

# TRA-1075: the local + edge database bootstrap keeps its non-superuser posture.
# Text assertions over database/sql/ and the env templates — no database needed.
test-db-init:
    @./scripts/test-db-init.sh

# ============================================================================
# Full Stack Development
# ============================================================================

# Docker-based development (database + backend container)
dev:
    @just database up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🚀 Starting backend..."
    @docker compose up -d backend
    @sleep 2
    @echo "🔄 Running migrations..."
    @just backend migrate
    @echo "✅ Development environment ready"

# Local development (parallel frontend + backend)
dev-local:
    @echo "🚀 Starting local development servers..."
    @echo "📱 Frontend: http://localhost:5173"
    @echo "🔧 Backend: http://localhost:8080"
    @echo ""
    @echo "Press Ctrl+C to stop both servers"
    @just frontend dev & just backend dev & wait

# Local development with BLE bridge (db + backend + frontend via bridge server)
dev-bridge:
    @just database up
    @echo ""
    @echo "🚀 Starting local development (BLE bridge mode)..."
    @echo "📱 Frontend: http://localhost:5173 (BLE via bridge server)"
    @echo "🔧 Backend:  http://localhost:8080"
    @echo ""
    @echo "Press Ctrl+C to stop both servers"
    @just frontend dev-bridge & just backend dev & wait

dev-stop:
    docker compose stop backend
    docker compose down

dev-logs:
    docker compose logs -f

# ============================================================================
# Worktree Support
# ============================================================================

# Copy gitignored build artifacts (openapi.internal/public specs, frontend/dist)
# from the main worktree so `go run . migrate` and friends work without
# regenerating them. Safe to run repeatedly; no-op if already in the main
# worktree.
worktree-bootstrap:
    #!/usr/bin/env bash
    set -euo pipefail
    main_dir=$(git worktree list --porcelain | awk '/^worktree /{path=$2} /^branch refs\/heads\/main$/{print path; exit}')
    if [ -z "$main_dir" ]; then
        echo "❌ Cannot locate main worktree (no branch refs/heads/main in git worktree list)" >&2
        exit 1
    fi
    here=$(git rev-parse --show-toplevel)
    if [ "$main_dir" = "$here" ]; then
        echo "ℹ️  Already in main worktree — nothing to bootstrap"
        exit 0
    fi
    echo "📋 Source: $main_dir"
    echo "📋 Target: $here"
    specs_dir="backend/internal/handlers/swaggerspec"
    for f in openapi.internal.json openapi.internal.yaml openapi.public.json openapi.public.yaml; do
        src="$main_dir/$specs_dir/$f"
        if [ -f "$src" ]; then
            cp "$src" "$here/$specs_dir/$f"
            echo "  ✓ $specs_dir/$f"
        else
            echo "  ⚠ $specs_dir/$f not found in main — run \`just backend api-spec\` there first" >&2
        fi
    done
    dist_src="$main_dir/backend/frontend/dist"
    dist_dst="$here/backend/frontend/dist"
    if [ -d "$dist_src" ]; then
        mkdir -p "$here/backend/frontend"
        rm -rf "$dist_dst"
        cp -r "$dist_src" "$dist_dst"
        echo "  ✓ backend/frontend/dist/"
    else
        echo "  ⚠ backend/frontend/dist not found in main — run \`just frontend build\` there first" >&2
    fi
    echo "✅ Worktree bootstrap complete"
