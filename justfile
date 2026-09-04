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

# TRA-1219 item 3 — CONTRIBUTING.md had no consumer and rotted silently: every
# command in its quickstart was dead. A dependency of `lint` rather than its own
# job, so it runs inside the existing `lint-test` required check instead of
# adding a context the branch-protection ruleset would have to be taught.
check-contributing:
    @./scripts/assert-contributing-paths.sh

lint: check-contributing (frontend "lint") (backend "lint") (cli "lint")

test: test-ops test-release-guards test-db-init test-env-drift test-bootstrap (frontend "test") (backend "test") (cli "test")

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

# TRA-1190: local env has exactly one declaration and every copy agrees with it
# — .env.local.example holds parts rather than DSNs (the shape the cluster has
# used since the GKE/CNPG migration), .env is a symlink to .env.local rather
# than a second file, and the database and role names come from
# database/justfile. Text assertions, no database needed.
test-env-drift:
    @./scripts/test-env-drift.sh

# TRA-1172: `just bootstrap` fails loudly and is a cheap no-op when warm. Runs
# the real script against a stub toolchain in a temp dir — no network, no vite.
test-bootstrap:
    @./scripts/test-bootstrap.sh

# ============================================================================
# Full Stack Development
# ============================================================================

# Docker-based development (database + backend container)
#
# Migrations run BEFORE the backend serves, which is the order the cluster uses:
# helm/trakrf-backend's migrate Job is a `pre-install,pre-upgrade` hook at
# weight -5, so no pod ever accepts traffic against a schema it is newer than.
#
# This recipe used to start the backend first and migrate second. That window is
# where TRA-1190 lived: the backend came up against a schema two migrations
# behind, /health returned 200 and signup returned 201 — because neither touched
# the new column — and only login 500'd. Every cheap check passed, so an e2e run
# launched into it produced 89 identical failures that read as test rot.
dev:
    @just database up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🔄 Running migrations (before the backend serves)..."
    @just backend migrate
    @echo "🔐 Re-applying grants (the migration ledger only exists now)..."
    @just database grants
    @echo "🚀 Starting backend..."
    @docker compose up -d backend
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

# TRA-1172. The first thing to run in a fresh worktree, before `just validate`.
# Installs deps and generates the two gitignored go:embed targets — without them
# the backend does not compile, and says so in terms that name neither the step
# nor the fix (`pattern frontend/dist: no matching files found`).
#
# Idempotent, and near-instant when there is nothing to do, so it is safe to run
# reflexively — including backgrounded at the start of a session, which is the
# intended use: it takes a couple of minutes cold and needs no supervision.
#
# `just bootstrap --force` rebuilds regardless of the staleness checks.
bootstrap *ARGS:
    @./scripts/bootstrap.sh "$@"

# Copy gitignored build artifacts (openapi.internal/public specs, frontend/dist)
# from the main worktree so `go run . migrate` and friends work without
# regenerating them. Safe to run repeatedly; no-op if already in the main
# worktree.
#
# Prefer `just bootstrap` (above). This one is the shortcut: it is faster
# because it builds nothing, but it copies MAIN's artifacts, so the specs and
# the frontend it leaves behind describe main's code rather than this branch's.
# Fine for `go run . migrate`; wrong for anything that reads what it embedded.
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
