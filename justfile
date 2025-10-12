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
    cd backend && go test ./...

backend-build:
    cd backend && go build ./...

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
