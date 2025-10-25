# Implementation Plan: Justfile Database Delegation Pattern
Generated: 2025-10-25
Specification: spec.md

## Understanding

This refactoring establishes consistent delegation patterns across all workspaces (frontend, backend, database) and separates infrastructure concerns (database/) from application concerns (backend/migrations).

**Key Design Decision**: Migrations belong in backend/justfile because they're versioned with application code in `backend/database/migrations/`, following industry conventions (Rails, Django, Prisma).

**Commands Identified from Current Root Justfile**:
- Infrastructure (→ database/justfile): `db-up`, `db-down`, `db-logs`, `psql`, `db-shell`, `db-reset`, `db-status` (7 commands)
- Application (→ backend/justfile): `db-migrate-up`, `db-migrate-down`, `db-migrate-status`, `db-migrate-create`, `db-migrate-force` (5 commands)

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/justfile` (lines 1-4) - Delegation pattern with `set fallback := true`
- `backend/justfile` (lines 1-4) - Delegation pattern with `set fallback := true`
- `justfile` (lines 15-19) - Workspace delegation pattern

**Files to Create**:
- `database/justfile` - Infrastructure commands (up, down, logs, psql, reset, status)

**Files to Modify**:
- `backend/justfile` - Add migration commands (migrate, migrate-down, migrate-status, migrate-create, migrate-force)
- `justfile` (root) - Add database delegation, add lazy aliases, update dev orchestration, remove old db-* recipes

## Architecture Impact
- **Subsystems affected**: Build/dev workflow only
- **New dependencies**: None
- **Breaking changes**: None - all existing command paths preserved via delegation and aliases
- **Directory structure**: New `database/` directory at project root

## Task Breakdown

### Task 1: Create database/ Directory Structure
**Action**: CREATE
**Files**: `database/justfile`

**Implementation**:
```bash
mkdir -p database
```

**Validation**:
```bash
test -d database && echo "✅ database/ directory created"
```

---

### Task 2: Create database/justfile with Infrastructure Commands
**File**: `database/justfile`
**Action**: CREATE
**Pattern**: Reference `frontend/justfile` and `backend/justfile` for fallback pattern

**Implementation**:
```just
# Database Infrastructure Task Runner
set fallback := true

# Start TimescaleDB container
up:
    docker compose up -d timescaledb
    @echo "⏳ Waiting for database to be ready..."
    @for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
        if docker compose exec timescaledb pg_isready -U postgres > /dev/null 2>&1; then \
            echo "✅ Database is ready"; \
            exit 0; \
        fi; \
        sleep 2; \
    done; \
    echo "⚠️  Database is starting but not ready yet. Run 'just database status' to check."

# Stop database container
down:
    docker compose down

# Show database logs
logs:
    docker compose logs -f timescaledb

# Interactive psql shell
psql:
    docker compose exec -it timescaledb psql -U postgres

# Alias for psql
alias shell := psql

# Reset database (WARNING: deletes all data)
reset:
    @echo "⚠️  This will delete all data. Press Ctrl+C to cancel."
    @sleep 3
    docker compose down -v
    docker compose up -d timescaledb
    @echo "⏳ Waiting for database to initialize..."
    @sleep 10
    @echo "✅ Database reset complete"

# Check database status
status:
    @docker compose ps timescaledb
    @docker compose exec timescaledb pg_isready -U postgres && echo "✅ Database is ready" || echo "❌ Database not ready"
```

**Source**: Copy logic from root justfile:
- `up` from `db-up` (lines 71-81)
- `down` from `db-down` (lines 83-84)
- `logs` from `db-logs` (lines 86-87)
- `psql` from `psql` (lines 93-94)
- `reset` from `db-reset` (lines 96-103)
- `status` from `db-status` (lines 105-107)

**Validation**:
```bash
test -f database/justfile && echo "✅ database/justfile created"
just database up      # Should start database
just database status  # Should show status
just database down    # Should stop database
just db up            # Should work (alias, after Task 4)
```

---

### Task 3: Add Migration Commands to backend/justfile
**File**: `backend/justfile`
**Action**: MODIFY
**Pattern**: Append after line 88 (after `shell` command)

**Implementation**:
Add to end of `backend/justfile`:
```just

# ============================================================================
# Database Migrations
# ============================================================================

# Run database migrations
migrate:
    @echo "🔄 Running database migrations..."
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" up'
    @echo "✅ Migrations complete"

# Alias for consistency
migrate-up: migrate

# Roll back last migration
migrate-down:
    @echo "⏪ Rolling back last migration..."
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" down 1'
    @echo "✅ Rollback complete"

# Show current migration version
migrate-status:
    @echo "📊 Migration status:"
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" version'

# Create new migration file
migrate-create name:
    @echo "📝 Creating new migration: {{name}}"
    docker compose exec backend sh -c 'migrate create -ext sql -dir /app/database/migrations -seq {{name}}'

# Force migration to specific version (dangerous)
migrate-force version:
    @echo "⚠️  Forcing migration version to {{version}}"
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" force {{version}}'
```

**Source**: Copy logic from root justfile:
- `migrate` from `db-migrate-up` (lines 113-116)
- `migrate-down` from `db-migrate-down` (lines 118-121)
- `migrate-status` from `db-migrate-status` (lines 123-125)
- `migrate-create` from `db-migrate-create` (lines 127-129)
- `migrate-force` from `db-migrate-force` (lines 131-133)

**Validation**:
```bash
grep -q "migrate:" backend/justfile && echo "✅ migrate command added"
just backend migrate-status  # Should show migration version
just be migrate-status       # Should work (alias, after Task 4)
```

---

### Task 4: Add Database Delegation and Aliases to Root Justfile
**File**: `justfile` (root)
**Action**: MODIFY
**Pattern**: Reference existing `frontend *args` and `backend *args` patterns (lines 15-19)

**Implementation**:

**Step 4a**: Add database delegation after line 19 (after `backend *args`):
```just
database *args:
    cd database && just {{args}}
```

**Step 4b**: Add lazy dev aliases after line 19 (create new section):
```just

# ============================================================================
# Lazy Dev Aliases
# ============================================================================

alias db := database
alias fe := frontend
alias be := backend
```

**Validation**:
```bash
grep -q "database \*args:" justfile && echo "✅ database delegation added"
grep -q "alias db := database" justfile && echo "✅ lazy aliases added"
just db up          # Should work
just fe --list      # Should work
just be --list      # Should work
```

---

### Task 5: Update dev Orchestration Command
**File**: `justfile` (root)
**Action**: MODIFY
**Lines**: ~42-49 (the `dev` recipe)

**Current**:
```just
dev: db-up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🔄 Running migrations..."
    @just db-migrate-up
    @echo "🚀 Starting backend..."
    @docker compose up -d backend
    @echo "✅ Development environment ready"
```

**New**:
```just
dev:
    @just database up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🔄 Running migrations..."
    @just backend migrate
    @echo "🚀 Starting backend..."
    @docker compose up -d backend
    @echo "✅ Development environment ready"
```

**Changes**:
- Remove `: db-up` dependency (line 42)
- Change to explicit `@just database up` (new line 43)
- Change `@just db-migrate-up` → `@just backend migrate` (line 47)

**Validation**:
```bash
grep -q "just database up" justfile && echo "✅ dev uses database delegation"
grep -q "just backend migrate" justfile && echo "✅ dev uses backend migration"
just dev            # Should start full stack
```

---

### Task 6: Remove Old db-* Recipes from Root Justfile
**File**: `justfile` (root)
**Action**: MODIFY
**Lines**: 71-133

**Remove these entire recipe blocks**:
- `db-up` (lines 71-81)
- `db-down` (lines 83-84)
- `db-logs` (lines 86-87)
- `db-shell` (lines 89-90)
- `psql` (lines 93-94) - NOTE: Now available as `just database psql`
- `db-reset` (lines 96-103)
- `db-status` (lines 105-107)
- `db-migrate-up` (lines 113-116)
- `db-migrate-down` (lines 118-121)
- `db-migrate-status` (lines 123-125)
- `db-migrate-create` (lines 127-129)
- `db-migrate-force` (lines 131-133)

**Remove sections** (lines 68-133):
- `# Docker Compose Orchestration` section
- `# Database Migrations` section

**Keep**:
- Everything before line 68 (delegation, validation, dev commands)
- Everything after line 133 (if any)

**Validation**:
```bash
! grep -q "^db-up:" justfile && echo "✅ db-up removed"
! grep -q "^db-migrate-up:" justfile && echo "✅ db-migrate-up removed"
just database up    # Should still work via delegation
just backend migrate # Should still work via delegation
```

---

## Risk Assessment

**Risk**: Breaking existing developer workflows
**Mitigation**:
- All old command paths preserved via delegation (`just database up` = old `just db-up`)
- Lazy aliases provide backward compatibility (`just db up`)
- Test all commands before committing

**Risk**: Confusion about where migrations live
**Mitigation**:
- Clear documentation in spec about infrastructure vs application
- Follows industry conventions (Rails, Django, Prisma)
- Migration files remain in `backend/database/migrations/`

**Risk**: Fallback pattern might not work as expected
**Mitigation**:
- Pattern already proven in frontend/justfile and backend/justfile
- Test from workspace directories before committing

## Integration Points
- Build system: Just task runner (no changes to Just itself)
- Docker Compose: No changes (same commands, different invocation path)
- Backend migrations: No changes to migration files or logic
- Dev workflow: Updated `dev` command to use delegation syntax

## VALIDATION GATES (MANDATORY)

This is a **configuration change** with no code modifications. Validation gates apply to justfile syntax only.

**After EVERY task, run**:
```bash
# Gate 1: Justfile Syntax Check
just --evaluate   # Must succeed (verifies syntax)

# Gate 2: Command Functionality
just database up     # Must start database
just database status # Must show status
just backend migrate-status  # Must show migration version
just db up           # Must work (alias)
just be migrate-status       # Must work (alias)

# Gate 3: Orchestration
just dev             # Must start full stack
```

**Enforcement Rules**:
- If syntax check fails → Fix justfile syntax immediately
- If any command fails → Fix and re-test
- Loop until ALL gates pass

## Validation Sequence

**After each task**:
```bash
just --evaluate    # Syntax check
```

**Final validation (after Task 6)**:
```bash
# Test delegation
just database up
just database logs    # Ctrl+C after verifying logs appear
just database status
just database down

# Test backend migrations
just backend migrate-status

# Test aliases
just db up
just db status
just be migrate-status

# Test fallback
cd database && just up && just status && just down

# Test full orchestration
just dev
docker compose ps
just dev-stop

# Test from workspace directories
cd backend && just migrate-status
cd ../database && just logs    # Ctrl+C after verifying

echo "✅ All validation passed"
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Existing delegation pattern to follow (frontend/justfile, backend/justfile)
✅ All old commands identified from current justfile
✅ Simple refactoring (move code, no logic changes)
✅ Proven pattern (fallback already works in frontend/backend)
✅ Zero external dependencies
✅ Configuration-only change (no code modifications)
⚠️ Need to ensure all 12 commands migrate correctly

**Assessment**: High confidence - straightforward refactoring following proven patterns already in use.

**Estimated one-pass success probability**: 95%

**Reasoning**: This is primarily moving existing code blocks to new files with delegation. The pattern is already proven in frontend/ and backend/. Main risk is missing a command during migration, easily caught by manual testing. No external dependencies or complex logic changes.
