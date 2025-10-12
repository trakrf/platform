# Stack: Go + React + TimescaleDB (Monorepo)

> **Package Manager**: pnpm (frontend only)
> **Task Runner**: Just (https://just.systems/)
> **Backend**: Go 1.21+
> **Frontend**: React + TypeScript + Vite
> **Database**: TimescaleDB (PostgreSQL extension)

## Quick Validation

From project root:
```bash
just validate
```

## Backend (Go)

### From backend/ directory:
```bash
# Lint
go fmt ./...
go vet ./...

# Test
go test ./...

# Build
go build ./...
```

### From project root (via Just):
```bash
just backend-lint
just backend-test
just backend-build
just backend  # All backend checks
```

## Frontend (React + TypeScript)

**IMPORTANT**: This project uses pnpm EXCLUSIVELY. Never use npm or npx.

### From frontend/ directory:
```bash
# Lint
pnpm run lint --fix

# Typecheck
pnpm run typecheck

# Test
pnpm test

# Build
pnpm run build
```

### From project root (via Just):
```bash
just frontend-lint
just frontend-typecheck
just frontend-test
just frontend-build
just frontend  # All frontend checks
```

## Full Stack Validation

From project root:
```bash
just lint        # Lint backend + frontend
just test        # Test backend + frontend
just build       # Build backend + frontend
just validate    # All checks (used by /check)
```

## CSW Integration

The `/check` command runs:
```bash
just check
```

This validates the entire stack is ready to ship.

## Database

TimescaleDB validation happens via backend tests. No separate validation commands needed.
