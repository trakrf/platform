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
    @echo "🔄 Running migrations..."
    @just backend migrate
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

dev-stop:
    docker compose stop backend
    docker compose down

dev-logs:
    docker compose logs -f
