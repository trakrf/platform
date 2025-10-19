# Stack: Go + React + TimescaleDB (Monorepo)

> **Package Manager**: pnpm (frontend only)
> **Task Runner**: Just (https://just.systems/)
> **Backend**: Go 1.25+
> **Frontend**: React + TypeScript + Vite
> **Database**: TimescaleDB (PostgreSQL extension)

## ⚠️ DEVELOPMENT MODE - NO BACKWARD COMPATIBILITY

**This project is in active development with ZERO production deployments.**

**Critical rules for AI agents and developers:**
- ❌ **NO backward compatibility required** - We have no users, no deployments, no legacy data
- ❌ **NO migration paths needed** - Breaking changes are acceptable and encouraged if they improve the codebase
- ❌ **NO deprecation warnings** - Just delete old code and move forward
- ✅ **DO break things to make them better** - Refactor aggressively, change APIs freely
- ✅ **DO simplify without guilt** - If a feature is speculative or unused, delete it
- ✅ **DO focus on getting it right** - Not on maintaining compatibility with nothing

**When this changes:**
- First production deployment → Update this section
- Real users → Start thinking about migrations
- Until then → Move fast and break things

## Quick Validation

From project root:
```bash
just validate
```

## Backend (Go)

### From backend/ directory:
```bash
# Lint
just lint      # or: go fmt ./... && go vet ./...

# Test
just test      # or: go test ./...

# Build
just build     # or: go build ./...

# All checks
just validate
```

### From project root (via Just delegation):
```bash
just backend lint
just backend test
just backend build
just backend validate  # All backend checks
```

## Frontend (React + TypeScript)

**IMPORTANT**: This project uses pnpm EXCLUSIVELY. Never use npm or npx.

### From frontend/ directory:
```bash
# Lint
just lint       # or: pnpm run lint --fix

# Typecheck
just typecheck  # or: pnpm run typecheck

# Test
just test       # or: pnpm test

# Build
just build      # or: pnpm run build

# All checks
just validate
```

### From project root (via Just delegation):
```bash
just frontend lint
just frontend typecheck
just frontend test
just frontend build
just frontend validate  # All frontend checks
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
