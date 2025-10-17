# Implementation Plan: Phase 3 - Database Migrations

Generated: 2025-10-17
Specification: spec.md

## Understanding

Convert Docker entrypoint SQL initialization to golang-migrate migration system. This is a **pure infrastructure change** with zero schema modifications. All 12 SQL files in `database/init/` will be ported verbatim to versioned migration pairs (up/down), enabling production-ready schema versioning and rollback capabilities.

**Key Decisions from Clarification**:
- Delete `database/init/` directory (clean cutover, no dueling strategies)
- Remove `docker-entrypoint-initdb.d` volume mount from docker-compose.yaml
- Auto-migrate in `just dev` recipe (zero friction workflow)
- Skip checksum verification for migrate binary (matches Air pattern)
- Down migrations use CASCADE drops (reverse dependency order handles cleanup)
- Install migrate in both dev + production stages (tech debt: separate migration user later)

## Relevant Files

**Reference Patterns**:
- `backend/Dockerfile:6` - Air binary installation pattern (will mirror for migrate)
- `database/init/01-prereqs.sql` through `database/init/99-sample-data.sql` - Source SQL to port

**Files to Create**:
- `database/migrations/000001_prereqs.up.sql` - Schema, functions (from 01-prereqs.sql)
- `database/migrations/000001_prereqs.down.sql` - Drop schema CASCADE
- `database/migrations/000002_accounts.up.sql` - Accounts table (from 02-accounts.sql)
- `database/migrations/000002_accounts.down.sql` - Drop accounts CASCADE
- `database/migrations/000003_users.up.sql` - Users table (from 03-users.sql)
- `database/migrations/000003_users.down.sql` - Drop users CASCADE
- `database/migrations/000004_account_users.up.sql` - RBAC junction (from 04-account_users.sql)
- `database/migrations/000004_account_users.down.sql` - Drop account_users CASCADE
- `database/migrations/000005_locations.up.sql` - Locations table (from 05-locations.sql)
- `database/migrations/000005_locations.down.sql` - Drop locations CASCADE
- `database/migrations/000006_devices.up.sql` - Devices table (from 06-devices.sql)
- `database/migrations/000006_devices.down.sql` - Drop devices CASCADE
- `database/migrations/000007_antennas.up.sql` - Antennas table (from 07-antennas.sql)
- `database/migrations/000007_antennas.down.sql` - Drop antennas CASCADE
- `database/migrations/000008_assets.up.sql` - Assets table (from 08-assets.sql)
- `database/migrations/000008_assets.down.sql` - Drop assets CASCADE
- `database/migrations/000009_tags.up.sql` - Tags table (from 09-tags.sql)
- `database/migrations/000009_tags.down.sql` - Drop tags CASCADE
- `database/migrations/000010_events.up.sql` - Events hypertable (from 10-events.sql)
- `database/migrations/000010_events.down.sql` - Drop events hypertable
- `database/migrations/000011_messages.up.sql` - Messages hypertable + trigger (from 11-messages.sql)
- `database/migrations/000011_messages.down.sql` - Drop messages, trigger, function
- `database/migrations/000012_sample_data.up.sql` - Sample data (from 99-sample-data.sql)
- `database/migrations/000012_sample_data.down.sql` - No-op (CASCADE drops handle data)

**Files to Modify**:
- `backend/Dockerfile` - Add migrate CLI installation (lines ~6-7, after Air)
- `docker-compose.yaml` - Remove init volume mount (line ~15)
- `justfile` - Add 5 migration commands (db-migrate-up, down, status, create, force)
- `README.md` - Update Database Management section with migration workflow
- `backend/README.md` - Add Database Migrations section

**Files to Delete**:
- `database/init/01-prereqs.sql` through `database/init/99-sample-data.sql` (12 files)
- `database/init/` directory (after migration files created)

## Architecture Impact

- **Subsystems affected**: Database, Docker, Build (Just), Documentation
- **New dependencies**: golang-migrate/migrate v4.17.0 (binary, not Go package)
- **Breaking changes**: None (schema identical, delivery mechanism different)
- **Tech debt added**: Production image can self-migrate (should separate migration user from app user for security in future)

## Task Breakdown

### Task 1: Install migrate CLI in Dockerfile

**File**: `backend/Dockerfile`
**Action**: MODIFY
**Pattern**: Mirror Air installation at line 6

**Implementation**:
```dockerfile
# After line 6 (Air installation), add:

# Install migrate CLI (pinned version for reproducibility)
RUN wget -qO- https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | \
    tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate
```

**Also add to production stage** (after line 31):
```dockerfile
# Production stage needs migrate for self-migration capability
FROM alpine:3.20 AS production
RUN apk --no-cache add ca-certificates wget
WORKDIR /root/

# Install migrate CLI
RUN wget -qO- https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | \
    tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate

COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
```

**Validation**:
- `docker compose build backend` succeeds
- `docker compose run --rm backend migrate --version` shows v4.17.0

### Task 2: Create database/migrations directory

**File**: `database/migrations/`
**Action**: CREATE

**Implementation**:
```bash
mkdir -p database/migrations
```

**Validation**:
- Directory exists: `ls -la database/migrations`

### Task 3: Create migration 000001 (prereqs)

**Files**:
- `database/migrations/000001_prereqs.up.sql`
- `database/migrations/000001_prereqs.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/01-prereqs.sql` verbatim):
```sql
SET search_path=trakrf,public;

CREATE SCHEMA IF NOT EXISTS trakrf;

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Function for permuted ID generation
CREATE OR REPLACE FUNCTION generate_permuted_id(sequence_name TEXT)
RETURNS TRIGGER AS $$
DECLARE
    next_id BIGINT;
    permuted_id BIGINT;
BEGIN
    -- Get next sequence value
    EXECUTE format('SELECT nextval(%L)', sequence_name) INTO next_id;

    -- Simple permutation: multiply by large prime and modulo
    permuted_id := (next_id * 1103515245 + 12345) % 2147483647;

    -- Ensure it's positive
    IF permuted_id < 0 THEN
        permuted_id := -permuted_id;
    END IF;

    NEW.id := permuted_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

**Down migration** (reverse order, CASCADE handles dependencies):
```sql
SET search_path=trakrf,public;

DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;
DROP FUNCTION IF EXISTS generate_permuted_id(TEXT) CASCADE;
DROP SCHEMA IF EXISTS trakrf CASCADE;
```

**Validation**:
- Files created: `ls -la database/migrations/000001*`
- Syntax check: `cat database/migrations/000001_prereqs.up.sql` (visual inspection)

### Task 4: Create migration 000002 (accounts)

**Files**:
- `database/migrations/000002_accounts.up.sql`
- `database/migrations/000002_accounts.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/02-accounts.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS accounts CASCADE;
DROP SEQUENCE IF EXISTS account_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 5: Create migration 000003 (users)

**Files**:
- `database/migrations/000003_users.up.sql`
- `database/migrations/000003_users.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/03-users.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS users CASCADE;
DROP SEQUENCE IF EXISTS user_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 6: Create migration 000004 (account_users)

**Files**:
- `database/migrations/000004_account_users.up.sql`
- `database/migrations/000004_account_users.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/04-account_users.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS account_users CASCADE;
DROP SEQUENCE IF EXISTS account_user_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 7: Create migration 000005 (locations)

**Files**:
- `database/migrations/000005_locations.up.sql`
- `database/migrations/000005_locations.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/05-locations.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS locations CASCADE;
DROP SEQUENCE IF EXISTS location_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 8: Create migration 000006 (devices)

**Files**:
- `database/migrations/000006_devices.up.sql`
- `database/migrations/000006_devices.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/06-devices.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS devices CASCADE;
DROP SEQUENCE IF EXISTS device_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 9: Create migration 000007 (antennas)

**Files**:
- `database/migrations/000007_antennas.up.sql`
- `database/migrations/000007_antennas.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/07-antennas.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS antennas CASCADE;
DROP SEQUENCE IF EXISTS antenna_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 10: Create migration 000008 (assets)

**Files**:
- `database/migrations/000008_assets.up.sql`
- `database/migrations/000008_assets.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/08-assets.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS assets CASCADE;
DROP SEQUENCE IF EXISTS asset_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 11: Create migration 000009 (tags)

**Files**:
- `database/migrations/000009_tags.up.sql`
- `database/migrations/000009_tags.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/09-tags.sql` verbatim)

**Down migration**:
```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS tags CASCADE;
DROP SEQUENCE IF EXISTS tag_seq CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 12: Create migration 000010 (events)

**Files**:
- `database/migrations/000010_events.up.sql`
- `database/migrations/000010_events.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/10-events.sql` verbatim)

**Down migration** (note: hypertable cleanup):
```sql
SET search_path=trakrf,public;

-- Drop hypertable (automatically drops chunks)
DROP TABLE IF EXISTS events CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 13: Create migration 000011 (messages)

**Files**:
- `database/migrations/000011_messages.up.sql`
- `database/migrations/000011_messages.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/11-messages.sql` verbatim)

**Down migration** (note: trigger + function + hypertable):
```sql
SET search_path=trakrf,public;

-- Drop trigger, then function, then table
DROP TRIGGER IF EXISTS messages_insert_trigger ON messages;
DROP FUNCTION IF EXISTS process_messages() CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
```

**Validation**:
- Files created with correct naming

### Task 14: Create migration 000012 (sample_data)

**Files**:
- `database/migrations/000012_sample_data.up.sql`
- `database/migrations/000012_sample_data.down.sql`

**Action**: CREATE

**Up migration** (copy from `database/init/99-sample-data.sql` verbatim)

**Down migration** (no-op, CASCADE drops handle data):
```sql
SET search_path=trakrf,public;

-- No-op: sample data cleanup handled by table CASCADE drops
-- This migration is reversible via down migrations 000011 -> 000001
```

**Validation**:
- Files created with correct naming
- Total: 24 migration files (12 up + 12 down)

### Task 15: Add migration commands to justfile

**File**: `justfile`
**Action**: MODIFY
**Pattern**: Add after existing `db-*` commands

**Implementation**:
```makefile
# Database migrations (from project root)
db-migrate-up:
    @echo "🔄 Running database migrations..."
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" up
    @echo "✅ Migrations complete"

db-migrate-down:
    @echo "⏪ Rolling back last migration..."
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" down 1
    @echo "✅ Rollback complete"

db-migrate-status:
    @echo "📊 Migration status:"
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" version

db-migrate-create name:
    @echo "📝 Creating new migration: {{name}}"
    docker compose exec backend migrate create -ext sql -dir /app/database/migrations -seq {{name}}

db-migrate-force version:
    @echo "⚠️  Forcing migration version to {{version}}"
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" force {{version}}
```

**Update `dev` recipe** to auto-migrate:
```makefile
# Find existing dev recipe and modify:
dev: db-up backend-dev
    @echo "⏳ Waiting for backend to be ready..."
    @sleep 3
    @echo "🔄 Running migrations..."
    @just db-migrate-up
    @echo "✅ Development environment ready"
```

**Validation**:
- `just --list` shows new migration commands
- Commands are alphabetically sorted with other db-* commands

### Task 16: Update docker-compose.yaml

**File**: `docker-compose.yaml`
**Action**: MODIFY
**Pattern**: Remove docker-entrypoint-initdb.d volume mount

**Implementation**:
```yaml
# Find timescaledb service volumes section (lines 13-15)
# BEFORE:
    volumes:
      - timescale_data:/var/lib/postgresql/data
      - ./database/init:/docker-entrypoint-initdb.d

# AFTER:
    volumes:
      - timescale_data:/var/lib/postgresql/data
      # Removed docker-entrypoint-initdb.d - using golang-migrate instead
```

**Validation**:
- `docker compose config` validates YAML syntax
- No volume mount for ./database/init

### Task 17: Delete database/init directory

**Files**: `database/init/` directory (12 SQL files)
**Action**: DELETE

**Implementation**:
```bash
# After migrations validated, delete old init scripts
rm -rf database/init/
```

**Validation**:
- Directory gone: `ls database/init` returns "No such file"
- Migrations directory exists: `ls database/migrations` shows 24 files

### Task 18: Test migration workflow

**Action**: VALIDATE
**Pattern**: End-to-end migration test

**Implementation**:
```bash
# Clean slate
just db-reset

# Start services (auto-migrates)
just dev

# Verify schema
docker compose exec timescaledb psql -U postgres -d postgres -c "\dt trakrf.*"

# Should show all 11 tables (accounts, users, account_users, locations,
# devices, antennas, assets, tags, events, messages)

# Test rollback
just db-migrate-down
just db-migrate-status  # Should show version 11

# Test re-apply
just db-migrate-up
just db-migrate-status  # Should show version 12

# Test create command
just db-migrate-create test_migration
# Should create 000013_test_migration.up.sql and .down.sql
```

**Validation**:
- All migrations apply without errors
- Schema matches original init script result
- Rollback works (at least 1 step)
- Migration version tracking works
- `schema_migrations` table exists in database

### Task 19: Update README.md

**File**: `README.md`
**Action**: MODIFY
**Pattern**: Update Database Management section

**Implementation**:
```markdown
# Find "Database Management" section and replace with:

## Database Management

### Quick Start
```bash
# Start database and run migrations
just dev

# Check migration status
just db-migrate-status
```

### Migration Commands
```bash
# Apply all pending migrations
just db-migrate-up

# Rollback last migration
just db-migrate-down

# Check current migration version
just db-migrate-status

# Create new migration
just db-migrate-create add_checkout_table

# Force specific version (recovery only)
just db-migrate-force 12
```

### Database Operations
```bash
# Reset database (⚠️  destroys all data)
just db-reset

# Connect to database shell
just db-shell

# View database logs
just db-logs
```

### Migration Files
Migrations are in `database/migrations/` with versioned pairs:
- `000001_prereqs.up.sql` / `000001_prereqs.down.sql`
- `000002_accounts.up.sql` / `000002_accounts.down.sql`
- ... (12 migrations total)

Each migration creates/drops one table or major schema component.
```

**Validation**:
- README renders correctly in GitHub preview
- Migration commands documented
- Examples are accurate

### Task 20: Update backend/README.md

**File**: `backend/README.md`
**Action**: MODIFY
**Pattern**: Add Database Migrations section after Environment Variables

**Implementation**:
```markdown
# Add new section after Environment Variables:

## Database Migrations

This project uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema versioning.

### Migration Workflow

**Normal development** (auto-migrates):
```bash
just dev  # Starts database and applies pending migrations
```

**Manual migration control**:
```bash
just db-migrate-up      # Apply all pending migrations
just db-migrate-down    # Rollback last migration
just db-migrate-status  # Show current version
```

### Creating New Migrations

```bash
# Create migration pair (up/down)
just db-migrate-create add_checkout_table

# Edit generated files:
# - database/migrations/000013_add_checkout_table.up.sql
# - database/migrations/000013_add_checkout_table.down.sql

# Apply new migration
just db-migrate-up
```

### Migration Conventions

**File naming**: `{version}_{description}.{up|down}.sql`
- Version: 6-digit zero-padded sequence (000001, 000002, ...)
- Description: snake_case, descriptive
- Direction: `up` (apply) or `down` (rollback)

**SQL structure**:
```sql
SET search_path=trakrf,public;

-- Up migration: CREATE TABLE ...
-- Down migration: DROP TABLE ... CASCADE
```

**Dependency order**: Migrations run sequentially, so order matters for foreign keys.

### Troubleshooting

**"Dirty database" error**:
```bash
# Check current version
just db-migrate-status

# Force to last known good version
just db-migrate-force 12

# Re-apply if needed
just db-migrate-up
```

**PG_URL format**:
```bash
# Standard format
postgresql://postgres:password@timescaledb:5432/postgres?search_path=trakrf,public

# With URL-encoded password
postgresql://postgres:rfidCollect%231@timescaledb:5432/postgres
```

**Migration history**: Tracked in `schema_migrations` table:
```sql
SELECT * FROM schema_migrations;
```
```

**Validation**:
- README renders correctly
- Troubleshooting section is accurate
- Examples match actual workflow

## Risk Assessment

**Risk**: Migration fails mid-apply, database in "dirty" state
**Mitigation**:
- Test migrations against fresh database first (`just db-reset && just dev`)
- Use `just db-migrate-force` to recover from dirty state
- Each migration is atomic (single transaction)
- Down migrations allow rollback

**Risk**: Schema drift between old init/ and new migrations/
**Mitigation**:
- Copy SQL verbatim from init files
- Validate schema identity with `psql` inspection after migration
- Run `diff` on table structure before/after

**Risk**: Forgotten dependency in down migrations
**Mitigation**:
- Use CASCADE on all drops (handles implicit dependencies)
- Test down migrations: `just db-migrate-down && just db-migrate-up`

**Risk**: Production self-migration security concern (app user has DDL rights)
**Mitigation**:
- Document as tech debt in plan
- Phase 4+ will separate migration user from app user
- For now, simplicity > security (dev/staging environment)

## Integration Points

- **Docker**: Migrate binary installed in both dev + production stages
- **Just**: 5 new migration commands (up, down, status, create, force)
- **Database**: `schema_migrations` table tracks version
- **Dev workflow**: `just dev` auto-migrates on startup

## VALIDATION GATES (MANDATORY)

**After EVERY task**, run:

```bash
# Syntax validation (for modified Dockerfile, docker-compose, justfile)
docker compose config  # Validates YAML syntax
just --list            # Validates justfile syntax

# After creating migrations, validate SQL syntax
cat database/migrations/000001_prereqs.up.sql  # Visual inspection

# After Docker changes, rebuild
docker compose build backend

# After migrations created, test end-to-end
just db-reset && just dev && just db-migrate-status
```

**Do not proceed to next task until current task validation passes.**

## Validation Sequence

**After Task 1** (Dockerfile):
- `docker compose build backend` succeeds
- `docker compose run --rm backend migrate --version` shows v4.17.0

**After Tasks 3-14** (migrations created):
- Count: `ls database/migrations/*.sql | wc -l` = 24 files

**After Task 15** (justfile):
- `just --list | grep db-migrate` shows 5 commands

**After Task 16** (docker-compose):
- `docker compose config` validates
- `grep -r docker-entrypoint-initdb.d docker-compose.yaml` = no results

**After Task 18** (E2E test):
- `just db-reset && just dev` completes without errors
- `just db-migrate-status` shows "12"
- `docker compose exec timescaledb psql -U postgres -d postgres -c "\dt trakrf.*"` shows 11 tables

**Final validation** (before shipping):
- `just check` passes all gates
- README.md and backend/README.md render correctly
- Git status clean (all migration files committed)

## Plan Quality Assessment

**Complexity Score**: 10/10 (HIGH - but overridden by user expertise)
**Cognitive Complexity**: 3/10 (LOW - porting known-good SQL)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ SQL is battle-tested (runs successfully in production trakrf-web)
- ✅ Clear dependency order (numeric prefixes 01-12)
- ✅ Similar pattern exists (Air binary installation in Dockerfile)
- ✅ User has 20+ years DB schema design experience
- ✅ One table per file = clear boundaries
- ⚠️ 29 files to create/modify (mechanical complexity, not cognitive)
- ✅ Zero schema changes (pure infrastructure port)

**Assessment**: High confidence - this is a mechanical port of proven SQL to a migration framework. The risk is low because the schema is known-good, dependencies are clear, and the user has deep DB expertise to debug any migration issues.

**Estimated one-pass success probability**: 85%

**Reasoning**: The primary risk is mechanical errors during copy/paste (24 migration files), not logical errors in SQL. The schema has run successfully many times via docker-entrypoint-initdb.d. Down migrations are simple (DROP CASCADE). Migration tooling is well-documented (golang-migrate). User's DB expertise significantly reduces debugging time if issues arise.
