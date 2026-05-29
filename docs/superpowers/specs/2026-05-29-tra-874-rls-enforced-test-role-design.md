# TRA-874 — RLS-enforced integration test role

**Date:** 2026-05-29
**Branch:** `chore/tra-874-rls-enforced-test-role`
**Ticket:** [TRA-874](https://linear.app/trakrf/issue/TRA-874) (origin: TRA-865)

## Problem

The integration test harness connects to the test database as the `postgres`
**superuser**. Postgres exempts superusers (and table owners) from row-level
security, so every RLS policy is unevaluated in CI. Any storage method that
JOINs or reads an RLS-protected table without a `WithOrgTx` wrapper — which sets
the `app.current_org_id` GUC the policies depend on — passes CI green and only
fails in production / black-box smoke. That is exactly how TRA-865 (`/history`
500) shipped to preview.

The only existing protection is `check-rls-guard`, a grep over a hardcoded list
of six storage files for raw `s.pool.*` access. It is a backstop, not coverage:
it missed `internal/storage/reports.go` because that file wasn't in its list,
and it cannot see a missing wrapper in any file outside the list (e.g. a method
on `organizations.go` that JOINs an RLS table).

## Goal

Run storage methods under integration test against a **non-superuser,
RLS-enforced** Postgres role that mirrors the production `trakrf-app-<env>`
posture. A missing `WithOrgTx` then fails an integration test, not just the
grep. The grep stays as defense-in-depth.

## Key constraints discovered

1. **`Pool()` is production code.** `internal/cmd/serve/serve.go:80-92` calls
   `store.Pool()` to wire the auth/orgs/health services. Its production meaning
   (the real connection pool) must not change.

2. **Fixtures legitimately need superuser.** 53 of 56 integration test files
   call `store.Pool()`, and 27 seed RLS-protected tables (`locations`, `assets`,
   `tags`, `asset_scans`, …) via **raw inserts with no org context**. These are
   intentional test setup, not the bug class. Cleanup `TRUNCATE … RESTART
   IDENTITY CASCADE` also needs privilege the app role won't have. Forcing these
   through RLS would break dozens of tests for no coverage gain.

3. **Grants live in infra, not migrations.** The production app/migrate role
   split and its GRANTs are applied by the `trakrf-infra` Helm
   `init-grants-job`, *not* by a platform migration. A role-creation migration
   would wrongly run against production. The test role must therefore be created
   harness-side (superuser), never as a migration.

4. **Tables are not `FORCE ROW LEVEL SECURITY`.** The table owner bypasses RLS.
   In the test DB tables are owned by `postgres` (the migration runner), so a
   separate non-owner role gets RLS enforced — the asymmetry we need, with no
   schema change.

## Design

Two connection pools in the harness, cleanly split by purpose:

| Pool | Role | Used for |
|---|---|---|
| **admin pool** | `postgres` (superuser) | DB create/drop, migrations, fixture seeding, `TRUNCATE` cleanup |
| **app pool** | `trakrf_test_app` (non-superuser, RLS enforced) | the `*Storage` under test — all storage queries + `WithOrgTx` |

### The seam: `store.Pool()` returns the admin pool

`Storage` gains an optional second pool used *only* as the `Pool()` accessor
return value. Production is unchanged (the field is nil → `Pool()` returns the
real pool). In tests:

- `store`'s internal query pool (`s.pool`, what every storage method and
  `WithOrgTx` execute against) = **app pool** → RLS enforced.
- `store.Pool()` = **admin pool** → the "superuser escape hatch" the ticket
  asks for. Every existing `pool := store.Pool()` fixture/seed/cleanup call
  keeps working **unchanged**.

```go
// internal/storage/storage.go
type Storage struct {
    pool         PgxPool // queries + WithOrgTx run here
    accessorPool PgxPool // returned by Pool(); nil in prod
}

func (s *Storage) Pool() PgxPool {
    if s.accessorPool != nil {
        return s.accessorPool
    }
    return s.pool
}

// NewForTest builds a Storage whose methods run on queryPool (RLS-enforced)
// while Pool() exposes accessorPool (superuser) for fixture setup/teardown.
func NewForTest(queryPool, accessorPool PgxPool) *Storage { ... }
```

This is semantically honest: `Pool()` is documented as "direct pool access for
advanced use cases"; in tests that advanced case is privileged fixture I/O.
Net production diff: one nil-guarded branch. Net test-file diff: **zero** — the
53 files are untouched.

### Harness changes (`internal/testutil/database.go`)

`SetupTestDatabase` (superuser, as today) additionally:

1. Creates the role once per cluster (idempotent `DO` block):
   `CREATE ROLE trakrf_test_app LOGIN PASSWORD '…' NOSUPERUSER NOBYPASSRLS;`
2. After migrations, applies grants mirroring the prod app role (idempotent,
   re-run each test because the DB is dropped/recreated):
   - `GRANT USAGE ON SCHEMA trakrf, public`
   - `GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA trakrf`
   - `GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA trakrf`
   - `GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA trakrf`
   - **No** `TRUNCATE`, **no** ownership, **no** `BYPASSRLS` → RLS stays enforced.
3. Builds both pools (app pool via the test DB URL with user/password overridden
   on the parsed `pgxpool.Config`).
4. `store := storage.NewForTest(appPool, adminPool)`.
5. `t.Cleanup` truncates via the **admin pool** and closes both pools.

`app.obfuscation_key` and `search_path` are set via `ALTER DATABASE` and are
inherited by every role, so the app role picks them up for free.

### Why missing `WithOrgTx` now fails

Under the app role, a query touching an RLS table without `WithOrgTx` leaves
`app.current_org_id` unset. The policy `org_id = current_setting('app.current_org_id')::BIGINT`
then either errors `42704` (unrecognized parameter) or `22P02` (cast of empty
string to bigint) — the precise TRA-865 SQLSTATEs — or returns zero rows. With
`WithOrgTx` the GUC is an integer and the policy matches. Fixtures are immune
because they run on the admin pool.

## Testing

1. **Keystone TDD test** (`internal/storage/rls_role_integration_test.go`):
   proves the harness is RLS-enforced. Seed a `locations` row via the admin
   pool, then via the app pool: a bare read with no org context errors / returns
   nothing; the same read inside `WithOrgTx(orgID)` returns the row. Written
   first — fails against today's superuser harness, passes once the role lands.
2. **Regression coverage of TRA-865**: the existing asset-history integration
   test runs under the new role and stays green (reports.go already wraps in
   `WithOrgTx`). Verification per ticket: temporarily revert the wrapper → test
   goes red with `22P02`/`42704`; restore → green. Done as a manual verification
   step, not committed.
3. **Audit pass**: run the full `just backend test-integration` suite under the
   new role. Triage failures:
   - Fixture/cleanup failures → design bug, fix the harness (should be none,
     since fixtures use the admin pool).
   - Genuine missing-`WithOrgTx` in a storage path → **spinoff ticket** per the
     ticket's instruction (fix inline only if trivial and in scope).
4. `check-rls-guard` continues to pass (unchanged backstop).

## Out of scope

- Production role/grant changes (infra-owned).
- Fixing latent bugs the audit uncovers beyond trivial ones — those become
  per-method spinoff tickets, cross-referenced to TRA-874.
- Converting fixture seeds to RLS-aware inserts (no coverage value).

## Files touched

- `internal/storage/storage.go` — `accessorPool` field, `NewForTest`, `Pool()` guard.
- `internal/testutil/database.go` — role creation, grants, dual pools, cleanup.
- `internal/storage/rls_role_integration_test.go` — new keystone test.
- Possibly per-audit spinoff fixes (only if trivial).
