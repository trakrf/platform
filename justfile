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
    docker-compose up -d timescaledb
    @echo "⏳ Waiting for database to be ready..."
    @for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
        if docker-compose exec timescaledb pg_isready -U postgres > /dev/null 2>&1; then \
            echo "✅ Database is ready"; \
            exit 0; \
        fi; \
        sleep 2; \
    done; \
    echo "⚠️  Database is starting but not ready yet. Run 'just db-status' to check."

db-down:
    docker-compose down

db-logs:
    docker-compose logs -f timescaledb

db-shell:
    docker-compose exec timescaledb psql -U postgres -d postgres

db-reset:
    @echo "⚠️  This will delete all data. Press Ctrl+C to cancel."
    @sleep 3
    docker-compose down -v
    docker-compose up -d timescaledb
    @echo "⏳ Waiting for database to initialize..."
    @sleep 10
    @echo "✅ Database reset complete"

db-status:
    @docker-compose ps timescaledb
    @docker-compose exec timescaledb pg_isready -U postgres && echo "✅ Database is ready" || echo "❌ Database not ready"
