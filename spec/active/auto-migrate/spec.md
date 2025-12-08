# Feature: Auto-Migrate on Startup

Linear: [TRA-151](https://linear.app/trakrf/issue/TRA-151/automate-database-migrations-on-application-startup)

## Outcome

Database migrations run automatically when the backend starts, eliminating manual migration steps in CI/CD and ensuring all environments stay in sync.

## User Story

As a **platform operator**
I want **migrations to run automatically on app startup**
So that **I never have to manually apply migrations or debug missing tables**

## Context

**Discovery**: Password reset feature (TRA-100) failed on preview because migration 000021 was never applied. Migrations are currently a manual step that's easy to forget.

**Current State**:
- Migrations managed via `golang-migrate` CLI
- Manual `migrate up` required after deployments
- Easy to forget, causing runtime errors (missing tables)

**Desired State**:
- Migrations embedded in Go binary
- Run automatically at startup before HTTP server starts
- Single artifact contains everything needed

## Technical Requirements

### Implementation
1. Use `//go:embed` to embed `database/migrations/*.sql` files in binary
2. Create `runMigrations()` function using:
   - `iofs` source (for embedded filesystem)
   - `postgres` driver (for database connection)
3. Call migrations after `storage.New()` but before HTTP server starts
4. Exit with error if migrations fail (fail fast)

### Dependencies
- `github.com/golang-migrate/migrate/v4/source/iofs`
- `github.com/jackc/pgx/v5/stdlib` (pool to sql.DB conversion)

### Why Programmatic Over CLI
- **Single artifact**: Migrations baked into binary
- **Omnibus-ready**: Aligns with future self-hosted container (app + TimescaleDB)
- **Zero operator knowledge**: No migration commands needed
- **Consistent**: Works identically across Railway, docker-compose, omnibus

### Simplifications (YAGNI)
- Single DB credential - no separate DDL/DML accounts
- migrate CLI can be removed from Dockerfile after this ships

## Validation Criteria

- [ ] Migrations run automatically on fresh database
- [ ] Migrations skip already-applied versions (idempotent)
- [ ] App exits with clear error if migration fails
- [ ] Works in docker-compose local dev
- [ ] Works on Railway preview/prod
- [ ] `migrate` CLI removed from Dockerfile production stage

## Files to Modify

- `backend/main.go` - Add embed directive and runMigrations()
- `backend/go.mod` - Add iofs dependency
- `Dockerfile` - Remove migrate CLI from production stage (optional cleanup)

## Out of Scope

- Rollback automation (manual rollbacks are rare, keep simple)
- Migration dry-run mode
- Separate DDL credentials
