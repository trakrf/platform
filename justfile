# TrakRF Platform - Task Runner
# https://just.systems/

# List all available recipes
default:
    @just --list

# Backend validation commands
backend-lint:
    cd backend && go fmt ./...
    cd backend && go vet ./...

backend-test:
    cd backend && go test -v ./...

backend-build:
    cd backend && go build -ldflags "-X main.version=0.1.0-dev" -o server .

backend-run:
    cd backend && go run .

# Docker development commands
backend-dev:
    docker compose up -d backend
    @echo "🚀 Backend running at http://localhost:8080"
    @echo "📊 Health check: curl localhost:8080/health"
    @echo "📋 View logs: just dev-logs"

backend-stop:
    docker compose stop backend

backend-restart:
    docker compose restart backend

backend-shell:
    docker compose exec backend sh

# Run all backend checks
backend: backend-lint backend-test backend-build

# Frontend validation commands
frontend-lint:
    cd frontend && pnpm run lint --fix

frontend-typecheck:
    cd frontend && pnpm run typecheck

frontend-test:
    cd frontend && pnpm test

frontend-build:
    cd frontend && pnpm run build

# Run all frontend checks
frontend: frontend-lint frontend-typecheck frontend-test frontend-build

# Combined validation commands
lint: backend-lint frontend-lint

test: backend-test frontend-test

build: backend-build frontend-build

# Full validation (used by CSW /check command)
validate: lint test build

# Alias for CSW integration
check: validate

# Docker Compose orchestration
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

# Database migrations
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

# Full stack development
dev: db-up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🔄 Running migrations..."
    @just db-migrate-up
    @echo "🚀 Starting backend..."
    @just backend-dev
    @echo "✅ Development environment ready"

dev-stop: backend-stop db-down

dev-logs:
    docker compose logs -f
