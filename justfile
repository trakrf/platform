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
dev: db-up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🔄 Running migrations..."
    @just db-migrate-up
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

# ============================================================================
# Docker Compose Orchestration
# ============================================================================

db-up:
    docker compose up -d timescaledb
    @echo "⏳ Waiting for database to be ready..."
    @for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
        if docker compose exec timescaledb pg_isready -U postgres > /dev/null 2>&1; then \
            echo "✅ Database is ready"; \
            exit 0; \
        fi; \
        sleep 2; \
    done; \
    echo "⚠️  Database is starting but not ready yet. Run 'just db-status' to check."

db-down:
    docker compose down

db-logs:
    docker compose logs -f timescaledb

db-shell:
    docker compose exec timescaledb psql -U postgres -d postgres

# Interactive psql shell
psql:
    docker compose exec -it timescaledb psql -U postgres

db-reset:
    @echo "⚠️  This will delete all data. Press Ctrl+C to cancel."
    @sleep 3
    docker compose down -v
    docker compose up -d timescaledb
    @echo "⏳ Waiting for database to initialize..."
    @sleep 10
    @echo "✅ Database reset complete"

db-status:
    @docker compose ps timescaledb
    @docker compose exec timescaledb pg_isready -U postgres && echo "✅ Database is ready" || echo "❌ Database not ready"

# ============================================================================
# Database Migrations
# ============================================================================

db-migrate-up:
    @echo "🔄 Running database migrations..."
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" up'
    @echo "✅ Migrations complete"

db-migrate-down:
    @echo "⏪ Rolling back last migration..."
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" down 1'
    @echo "✅ Rollback complete"

db-migrate-status:
    @echo "📊 Migration status:"
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" version'

db-migrate-create name:
    @echo "📝 Creating new migration: {{name}}"
    docker compose exec backend sh -c 'migrate create -ext sql -dir /app/database/migrations -seq {{name}}'

db-migrate-force version:
    @echo "⚠️  Forcing migration version to {{version}}"
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" force {{version}}'
