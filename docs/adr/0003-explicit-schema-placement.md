# ADR 0003 — One thing runs migrations, and it owns the schema and its ledger

Date: 2026-07-30
Status: Accepted
Tracking: TRA-1069 (shipped), TRA-1075 (local + edge non-superuser roles), TRA-1077 (migration checksum guard)

## Context

Application objects live in a `trakrf` schema. golang-migrate's postgres driver
locates its `schema_migrations` ledger with `CURRENT_SCHEMA()` when
`Config.SchemaName` is unset — and on a fresh database `trakrf` does not exist
yet, because migration `000001` is what creates it. So `CURRENT_SCHEMA()` resolves
to `public` on the first run and `trakrf` on every run afterwards. The ledger
relocates, the new one starts at version 0, the stack replays onto a populated
schema and dies on "already exists". Forcing a version past that leaves a ledger
reporting clean over a schema that does not match it.

TRA-1069 is that: `trakrf.refresh_tokens` missing while the ledger read a clean
version 38, signup 500ing, nothing naming the cause.

It had been found four times and fixed zero times, because every fix addressed the
symptom — steering `CURRENT_SCHEMA()` — rather than the cause:

| Where | Mitigation | Steered `CURRENT_SCHEMA()` to |
|---|---|---|
| TRA-278 (2026-01-14, canceled) | Diagnosed it; specified `Config{SchemaName}` | — |
| infra `f52dd9d` / TRA-383 | Inverted the DSN to `public,trakrf` | `public` |
| `test-contract` | Pre-created the schema | `trakrf` |
| `deploy/edge` | Pre-created the schema | `trakrf` |
| a plain local database | nothing | drifts — TRA-1069 |

Two steered opposite ways, which is why the ledger's location differed by
environment. The deeper problem was that there were **two implementations** of
"run the migrations" — the `./server migrate` subcommand and a bare `migrate` CLI
invocation in the test harness — so anything learned had to be taught twice, and
wasn't.

## Decision

**There is one implementation.** `internal/cmd/migrate` exposes `Run` (reads
`PG_URL`) and `RunURL` (explicit URL). The integration harness calls `RunURL`
instead of shelling out to the `migrate` CLI, which deleted ~110 lines of
harness — `getMigrationsPath`, `findMigrateBinary`, and the "migrate binary not
found in PATH" failure mode along with them. Anything that migrates goes through
here and inherits everything below for free.

**It creates the schema itself**, `CREATE SCHEMA IF NOT EXISTS trakrf`, before
golang-migrate looks for its ledger. This is the actual fix: it is the one thing
that cannot be left to a migration, because the driver resolves the ledger
location and creates that table *before* migration `000001` runs. Migration
`000001` still declares the schema, for a hand-applied run.

**It sets its own `search_path`** via `ConnConfig.RuntimeParams`, so a migration's
unqualified DDL resolves to the application schema regardless of the caller's DSN
or role default. Nothing about placement depends on deployment config.

**The ledger lives in `trakrf`**, pinned with `Config{SchemaName}`, so the schema
and its bookkeeping are one unit. That matters for the documented rebuild path:
`DROP SCHEMA trakrf CASCADE` now takes the ledger with it, leaving a genuinely
empty database. With the ledger in `public` it would survive the drop still
claiming version 38, and the next migrate would report "no pending migrations"
against an empty schema — TRA-1069 reproduced by the reset procedure itself.

**Migrating is refused when a `schema_migrations` exists in any other schema**,
naming each with its version. A split history needs a human to decide which is
real; reporting success over one is how TRA-1069 stayed invisible. This also makes
the one-time ledger relocation below safe to forget: the Job fails loudly instead
of replaying.

## Consequences

* **Preview and prod need a one-time relocation** — their ledgers are in `public`
  (at 38 and 10). `ALTER TABLE public.schema_migrations SET SCHEMA trakrf;` before
  or with the deploy. If forgotten, migrate refuses and nothing is damaged.
* `deploy/edge` needs no change: its pre-create already steered to `trakrf`, which
  is now where the ledger belongs. The workaround is now redundant rather than
  wrong, and can be dropped whenever that file is next touched.
* **Already-split local databases must be rebuilt** (`just db reset` then
  migrate). Intended — a reconcile cannot be trusted when the foundation came from
  a pre-fold migration.
* infra's `public,trakrf` DSN inversion is no longer load-bearing, so the ordering
  is free to go back to `trakrf, public` whenever convenient. Not urgent.
* golang-migrate keeps no checksums — its ledger is one `(version, dirty)` row and
  `Up()` never opens files at or below the current version — so editing an applied
  migration is undetectable. That is how three migrations folded into `000009`
  silently never ran. TRA-1077 adds the guard Flyway would have given us.

## Notes on scope

The `trakrf` schema stays. Its original justifications (plugins, schema-per-tenant)
were never implemented, but what it earns now is operational:
`DROP SCHEMA trakrf CASCADE` as the rebuild primitive, and a clean boundary for
the two-role least-privilege posture (TRA-85). Flattening it into `public` would
mean ~810 `trakrf.` references across 44 Go files, 198 more in migration SQL, and
a live 28-table migration on prod and preview — to forfeit both of those. It would
not even achieve single-schema: TimescaleDB occupies nine schemas with 213 tables.

Schema-per-tenant is abandoned. Plugin or custom-data schemas stay plausible, and
explicit placement is what makes them possible rather than merely compatible: if
core resolved names through the path, a plugin schema could shadow a core table,
and a shadowed table means the real table's RLS policies are never consulted.
Terms if one arrives — its own ledger (a single `(version, dirty)` row cannot
represent two histories), explicit DDL placement, and no unpinned
`SECURITY DEFINER`. Nothing to build now.

TRA-278 (configurable schema name) stays canceled. Its driver — Timescale Cloud
blocking `CREATE DATABASE`, forcing schema-per-environment — died when preview
moved to CNPG. It also cannot cover `SECURITY DEFINER` functions, which must pin a
literal `SET search_path` or a caller who can create objects earlier on the path
can shadow a table and execute as the definer. So the schema name can never fully
leave the SQL, and treating it as an identifier rather than a configuration point
is the honest position. Its migration-bootstrapping half shipped as TRA-1069.

TRA-1075 (non-superuser roles for local dev and edge) survives this record but is
no longer justified by `search_path` — nothing here depends on the role. It stands
on its own argument: a superuser bypasses RLS, so every policy goes untested
locally until it reaches a deployed environment. The integration harness already
proved the posture with `trakrf_test_app` (TRA-874).
