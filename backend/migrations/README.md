# Migrations

This directory contains the canonical schema definition as a set of
versioned SQL files applied in numeric order by golang-migrate.

## Layout

The 10 foundational files (`000001`–`000010`) define the schema by concern,
not chronology. Each file is up-only. Future incremental changes
(`000011`+) follow the conventional up+down pattern.

## History

The pre-TRA-720 stack contained 44 migration files representing schema
evolution: tenant model pivots, column renames, denormalization removals.
Those files were collapsed into this clean stack as part of TRA-720 / the
GKE/CNPG cutover (TRA-810).

To inspect the pre-TRA-720 stack:

    git checkout pre-tra-720 -- backend/migrations
    ls backend/migrations          # see the 82 legacy files

Or browse via the tag on GitHub: <https://github.com/trakrf/platform/releases/tag/pre-tra-720>

## Conventions

- **Up-only foundation.** Files `000001`–`000010` have no down-migration.
  They are the schema baseline; rolling them back means dropping the
  schema entirely.
- **Up+down for increments.** Any migration added after `000010` follows
  the conventional pattern (`000011_<topic>.up.sql` and
  `000011_<topic>.down.sql`).
- **Idempotent where possible.** `CREATE EXTENSION IF NOT EXISTS`,
  `CREATE SCHEMA IF NOT EXISTS`, etc. — guards against double-apply on
  recovery scenarios.

## The schema and its ledger are owned by `./server migrate` (TRA-1069)

`internal/cmd/migrate` is the only thing that applies migrations — the integration
harness calls into it rather than shelling out to the `migrate` CLI. Before
golang-migrate runs it:

1. creates the `trakrf` schema if absent, because the driver resolves its
   `schema_migrations` location *before* migration `000001` could create it;
2. sets its own `search_path`, so unqualified DDL never depends on the caller's
   DSN or role;
3. pins the ledger to `trakrf.schema_migrations`, so `DROP SCHEMA trakrf CASCADE`
   takes the ledger with it and leaves a genuinely empty database;
4. refuses to run if a `schema_migrations` exists in another schema — a split
   history that would otherwise report a clean version over a mismatched schema.

Do not add another path that applies migrations. See `docs/adr/0003`.

## Applied migrations are immutable (TRA-1077)

`checksums.txt` records the SHA-256 of every file in this directory, and
`TestMigrationChecksums` re-derives and diffs it on every CI run. No database
is involved; it runs in milliseconds inside the existing backend test job.

This exists because golang-migrate cannot catch the mistake itself. Its ledger
is one `(version, dirty)` row with no file hashes, and `Up()` only opens files
*after* the recorded version — so editing a migration that has already been
applied is undetectable at runtime, and the new DDL silently never reaches any
database that recorded that version. That is exactly how TRA-1069 happened.

So:

- **Adding a migration** — regenerate the manifest and commit it with the
  migration:

      just backend migrate-checksums

  The diff should be new lines only.
- **Changing something already applied** — you can't. Add a forward migration.
  A changed hash on an existing line in a `checksums.txt` diff is a red flag,
  not a formality: it means either an applied migration was edited, or the
  regeneration was used to paper over one.
- **Deleting a migration** — same answer. Removing a file desynchronizes every
  database that already recorded that version; the test fails on the missing
  entry.

The guard is source-level. It stops the edit from reaching main; it cannot
repair a database that already applied the pre-edit version.

## Required GUC

`trakrf.generate_obfuscated_id()` reads `app.obfuscation_key` via
`current_setting()`. The key must be set on the target database before
any insert hits a Feistel trigger:

    ALTER DATABASE <db> SET app.obfuscation_key = '<64-hex-char-secret>';

This is normally handled at CNPG provisioning time; see TRA-810 for the
data cutover sequence.

## Role separation (TRA-85)

Production environments use **two distinct database roles** for least-privilege
defense in depth. The platform binary respects this split:

| Role | DDL | DML | Used by |
|---|---|---|---|
| `trakrf-migrate-<env>` | yes (owns all schema objects) | yes | `./server migrate` (helm migrate-job) |
| `trakrf-app-<env>` | no (USAGE on schema, EXECUTE on functions, CRUD on tables only) | yes | `./server serve` (helm backend deployment) |

The bare `./server` invocation defaults to `serve` (no DDL needed at runtime).
Migrations must be run explicitly via `./server migrate` under the migrate role.

GRANT flow lives in `trakrf-infra` chart `helm/trakrf-db/templates/init-grants-job.yaml`
(`post-install,post-upgrade` Helm hook, hook-weight 5). It:
1. Re-applies grants on existing objects (recovers from `DROP SCHEMA CASCADE`).
2. Sets `ALTER DEFAULT PRIVILEGES FOR ROLE <migrate-role> IN SCHEMA trakrf` so
   migrate-created tables/sequences/functions inherit app-role grants.

The default-privileges policy is per-schema and gets dropped along with the
schema. If you rebuild the schema (e.g., during M3 cutover), re-run the
init-grants Job — or manually issue the GRANT block from a CNPG superuser
session.
