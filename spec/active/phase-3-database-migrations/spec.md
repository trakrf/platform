# Phase 3: Database Migrations

**Workspace**: backend
**Status**: Active
**Created**: 2025-10-17
**Linear**: TRA-77

## Outcome

Replace Docker entrypoint SQL initialization with go-migrate migration system. Pure infrastructure change - **zero schema modifications**.

## User Story

As a backend developer
I want proper database migration tooling
So that schema changes are versioned, repeatable, and safely reversible

## Context

**Current State**: 12 SQL files in `database/init/` loaded via Docker entrypoint
- Works for initial setup
- No versioning or rollback capability
- No migration history tracking
- Cannot handle schema changes in production

**Desired State**: golang-migrate/migrate with versioned migrations
- Each SQL file becomes a versioned migration pair (up/down)
- Migration state tracked in database
- Reversible changes with down migrations
- Production-ready migration workflow

**Success Criteria**: Same schema deployed, different mechanism

## Technical Requirements

### 1. Migration Tool Setup

- [ ] Add `golang-migrate/migrate` CLI to backend development environment
- [ ] Install method: Direct binary download (consistent with Air approach)
- [ ] Version: Pin to latest stable (v4.17.0 as of Oct 2024)
- [ ] Location: Install to backend container in Dockerfile

### 2. Migration File Structure

Convert existing SQL files to migration pairs:

```
database/migrations/
├── 000001_prereqs.up.sql           # from 01-prereqs.sql
├── 000001_prereqs.down.sql
├── 000002_accounts.up.sql          # from 02-accounts.sql
├── 000002_accounts.down.sql
├── 000003_users.up.sql             # from 03-users.sql
├── 000003_users.down.sql
├── 000004_account_users.up.sql     # from 04-account_users.sql
├── 000004_account_users.down.sql
├── 000005_locations.up.sql         # from 05-locations.sql
├── 000005_locations.down.sql
├── 000006_devices.up.sql           # from 06-devices.sql
├── 000006_devices.down.sql
├── 000007_antennas.up.sql          # from 07-antennas.sql
├── 000007_antennas.down.sql
├── 000008_assets.up.sql            # from 08-assets.sql
├── 000008_assets.down.sql
├── 000009_tags.up.sql              # from 09-tags.sql
├── 000009_tags.down.sql
├── 000010_events.up.sql            # from 10-events.sql
├── 000010_events.down.sql
├── 000011_messages.up.sql          # from 11-messages.sql
├── 000011_messages.down.sql
├── 000012_sample_data.up.sql       # from 99-sample-data.sql
├── 000012_sample_data.down.sql
```

**Migration Naming Convention**: `{version}_{description}.{up|down}.sql`

### 3. Migration Content

**Up Migrations**:
- Copy content from `database/init/NN-*.sql` files **verbatim**
- No schema changes
- No table renames
- Exact port of existing SQL

**Down Migrations**:
- Reverse the up migration
- Drop tables in reverse dependency order
- Drop sequences, functions, triggers
- Safe rollback to pre-migration state

**Example** (000001_prereqs.down.sql):
```sql
SET search_path=trakrf,public;

DROP FUNCTION IF EXISTS update_updated_at_column();
DROP FUNCTION IF EXISTS generate_permuted_id(TEXT);
DROP SCHEMA IF EXISTS trakrf CASCADE;
```

### 4. Just Commands

Add migration workflow commands to justfile:

```makefile
# Database migrations (from project root)
db-migrate-up:
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" up

db-migrate-down:
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" down 1

db-migrate-status:
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" version

db-migrate-create name:
    docker compose exec backend migrate create -ext sql -dir /app/database/migrations -seq {{name}}

db-migrate-force version:
    docker compose exec backend migrate -path /app/database/migrations -database "${PG_URL}" force {{version}}
```

### 5. Docker Integration

**Update backend/Dockerfile**:
```dockerfile
# Development stage
FROM golang:1.25-alpine AS development
WORKDIR /app

# Install Air for hot-reload
RUN go install github.com/air-verse/air@v1.63.0

# Install migrate CLI
RUN wget -qO- https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz -C /usr/local/bin migrate

# Copy and build
COPY go.mod ./
RUN go mod download
COPY . .

CMD ["air", "-c", ".air.toml"]
```

**Update docker-compose.yaml**:
- Remove `./database/init:/docker-entrypoint-initdb.d` volume mount
- Database starts empty (migrations handle schema)
- Consider adding `db-migrate-up` to justfile `dev` recipe

### 6. Documentation Updates

**README.md**:
- Update Database Management section with migration commands
- Document migration workflow (create, apply, rollback)

**backend/README.md**:
- Add Database Migrations section
- Document PG_URL format requirements
- Add troubleshooting for common migration issues

### 7. Validation

Before shipping, verify:
- [ ] `just db-reset` → fresh database
- [ ] `just db-migrate-up` → all 12 migrations apply successfully
- [ ] `psql` inspection shows identical schema to current init scripts
- [ ] `just db-migrate-down` → successful rollback (at least 1 step)
- [ ] `just db-migrate-status` → shows current version
- [ ] `just db-migrate-create test` → creates numbered migration files

### 8. Migration Dependencies

Migrations must preserve dependency order:
1. prereqs (schema, functions)
2. accounts (base multi-tenant)
3. users (identity)
4. account_users (RBAC junction)
5. locations (physical places)
6. devices (scan hardware)
7. antennas (scan points)
8. assets (things to track)
9. tags (identifiers)
10. events (time-series location data)
11. messages (MQTT raw data)
12. sample_data (dev/test data)

## Out of Scope (Deferred to Future Phases)

- ❌ Schema changes or table renames
- ❌ New tables (checkout, notifications, etc.)
- ❌ Identifier model refactoring (asset vs location tags)
- ❌ Production migration automation (CI/CD)
- ❌ Migration testing framework

## Success Metrics

- ✅ All 12 migrations created from existing SQL files
- ✅ `just db-migrate-up` produces identical schema to current `database/init/` approach
- ✅ Down migrations successfully drop all tables/functions/sequences
- ✅ Migration version tracked in `schema_migrations` table
- ✅ Documentation complete with migration workflow examples
- ✅ Zero schema drift between old and new approach

## Technical Constraints

- **PG_URL Format**: Must include `?x-migrations-table=schema_migrations` for custom table name (optional)
- **Search Path**: Must set `search_path=trakrf,public` in each migration
- **Hypertables**: TimescaleDB `create_hypertable()` must run after table creation
- **RLS Policies**: Depend on `app.current_account_id` session variable pattern
- **Permuted IDs**: Depend on `generate_permuted_id()` function from 000001

## References

- **Linear Issue**: https://linear.app/trakrf/issue/TRA-77/phase-3-database-migrations
- **golang-migrate**: https://github.com/golang-migrate/migrate
- **Current SQL**: `database/init/*.sql` (12 files, 826 lines total)
- **Migration Guide**: https://github.com/golang-migrate/migrate/blob/master/database/postgres/TUTORIAL.md

## Notes

**Why This Approach?**
- De-risks migration tooling setup
- No schema changes = no data migration complexity
- Establishes foundation for future schema evolution
- Can add checkout/refactoring in Phase 4 using this infrastructure

**Future Schema Changes** (Phase 4+):
- devices → scan_device
- antennas → scan_point
- messages → identifier_scan
- events → location_scan
- Add checkout table
- Refactor tags → identifiers (dual-purpose: asset + location identification)
