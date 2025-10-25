# Feature: Justfile Database Delegation Pattern

## Origin
This spec emerged from recognizing that database commands (`db-*`) live in root justfile, breaking the delegation pattern used by frontend/ and backend/.

## Outcome
Consistent justfile delegation pattern across all workspaces (frontend, backend, database) with lazy dev aliases for ergonomic CLI usage.

## User Story
**As a developer**
I want consistent justfile patterns across all workspaces
So that I can run commands predictably from any directory

## Context

### Discovery
- Frontend and backend already use delegation pattern successfully
- Database commands (`db-*`) live in root justfile, breaking pattern
- Migration commands belong with backend (application concern, not infrastructure)
- No `database/` directory exists yet

### Current State
- Root justfile: Contains `db-up`, `db-down`, `db-reset`, `db-migrate-*`, etc.
- No `database/justfile` exists
- Migration commands in root justfile (should be in backend)
- Inconsistent with frontend/backend delegation pattern

### Desired State
- `database/justfile` with infrastructure commands: `up`, `down`, `reset`, `logs`, `psql`, `status`
- `backend/justfile` with migration commands: `migrate`, `migrate-down`, `migrate-status`, `migrate-create`, `migrate-force`
- Root justfile delegates: `database *args`, `frontend *args`, `backend *args`
- **Lazy dev aliases**: `db` → `database`, `fe` → `frontend`, `be` → `backend`

## Technical Requirements

### 1. Create database/ Directory
- New directory at project root: `database/`
- Contains only `justfile` (infrastructure orchestration)

### 2. Create database/justfile
**Commands** (infrastructure concern):
- `up` - Start TimescaleDB container (from root `db-up`)
- `down` - Stop TimescaleDB container (from root `db-down`)
- `logs` - Show database logs (from root `db-logs`)
- `psql` - Interactive psql shell (from root `psql`)
- `shell` - Alias to `psql`
- `reset` - Reset database (from root `db-reset`)
- `status` - Check database status (from root `db-status`)

**Pattern**: Set `fallback := true` for access to root recipes

### 3. Update backend/justfile
**Add migration commands** (application concern):
- `migrate` (or `migrate-up`) - Run migrations up (from root `db-migrate-up`)
- `migrate-down` - Roll back last migration (from root `db-migrate-down`)
- `migrate-status` - Show migration version (from root `db-migrate-status`)
- `migrate-create name` - Create new migration (from root `db-migrate-create`)
- `migrate-force version` - Force migration version (from root `db-migrate-force`)

**Rationale**: Migrations are versioned with backend code in `backend/database/migrations/`

### 4. Update Root justfile
**Add delegation**:
- `database *args: cd database && just {{args}}`
- Ensure `frontend *args` and `backend *args` exist

**Add lazy dev aliases**:
- `alias db := database`
- `alias fe := frontend`
- `alias be := backend`

**Remove old recipes**:
- `db-up`, `db-down`, `db-logs`, `db-shell`, `psql`, `db-reset`, `db-status`
- `db-migrate-up`, `db-migrate-down`, `db-migrate-status`, `db-migrate-create`, `db-migrate-force`

**Update orchestration commands**:
- `dev` command: Change `db-up` → `just database up`, `db-migrate-up` → `just backend migrate`
- `dev-stop` command: Update if needed

## Architecture Decision: Infrastructure vs Application

**Principle**: Separate infrastructure concerns from application concerns

**database/justfile** (infrastructure):
- Start/stop database containers
- View logs
- Connect via psql
- Reset database

**backend/justfile** (application):
- Run schema migrations
- Application's view of database structure
- Versioned with application code

**Analogy**: Like Rails `rails db:migrate` or Django `manage.py migrate` - migrations are application commands.

## Code Examples

### database/justfile (NEW FILE)
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

### backend/justfile (ADD MIGRATION COMMANDS)
```just
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

### Root justfile (CHANGES)
```just
# ============================================================================
# Workspace Delegation
# ============================================================================

database *args:
    cd database && just {{args}}

frontend *args:
    cd frontend && just {{args}}

backend *args:
    cd backend && just {{args}}

# Lazy dev aliases
alias db := database
alias fe := frontend
alias be := backend

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
```

## Usage Examples

### New delegation syntax (verbose)
```bash
just database up          # Start database
just database logs        # View logs
just database psql        # Connect to DB
just backend migrate      # Run migrations
just backend migrate-down # Rollback
just frontend dev         # Start frontend
```

### Lazy dev aliases (abbreviated)
```bash
just db up         # Start database
just db logs       # View logs
just db psql       # Connect to DB
just be migrate    # Run migrations
just fe dev        # Start frontend
```

### From workspace directories (fallback)
```bash
cd database
just up            # Works (local command)
just dev           # Works (falls back to root)

cd backend
just migrate       # Works (local command)
just dev           # Works (local command)
just db up         # Works (falls back to root alias)
```

## Validation Criteria
- [ ] `database/` directory created with justfile
- [ ] `database/justfile` has all infrastructure commands (up, down, logs, psql, reset, status)
- [ ] `backend/justfile` has all migration commands (migrate, migrate-down, migrate-status, migrate-create, migrate-force)
- [ ] Root justfile has delegation: `database *args`, `frontend *args`, `backend *args`
- [ ] Root justfile has aliases: `db`, `fe`, `be`
- [ ] Old `db-*` recipes removed from root justfile
- [ ] `dev` command updated to use new delegation syntax
- [ ] Commands work from root: `just database up`, `just backend migrate`
- [ ] Commands work from workspace dirs: `cd database && just up`
- [ ] Aliases work: `just db up`, `just fe dev`, `just be migrate`
- [ ] Fallback works: `cd database && just dev` calls root recipe
- [ ] All existing functionality preserved (no breaking changes)

## Success Metrics
- Consistent delegation pattern across frontend, backend, database
- Clear separation: infrastructure (database/) vs application (backend/)
- Lazy dev aliases improve ergonomics
- No breaking changes to existing workflows
- Pattern follows industry conventions (migrations as application concern)
