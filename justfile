# TrakRF Platform - Task Runner
# https://just.systems/

# List all available recipes
default:
    @just --list

# ============================================================================
# Workspace Delegation
# ============================================================================
# Delegate commands to workspace justfiles
# Usage: just <workspace> <command> [args...]
# Example: just frontend dev, just backend test

frontend *args:
    cd frontend && just {{args}}

backend *args:
    cd backend && just {{args}}

database *args:
    cd database && just {{args}}

ingester *args:
    cd ingester && just {{args}}

# ============================================================================
# Lazy Dev Aliases
# ============================================================================

alias db := database
alias fe := frontend
alias be := backend
alias ing := ingester

# ============================================================================
# Combined Validation Commands
# ============================================================================
# Run checks across all workspaces

lint: (frontend "lint") (backend "lint")

test: (frontend "test") (backend "test")

build: (frontend "build") (backend "build")

validate: lint test build

# Alias for CSW integration
check: validate

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
