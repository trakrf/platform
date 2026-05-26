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

## Required GUC

`trakrf.generate_obfuscated_id()` reads `app.obfuscation_key` via
`current_setting()`. The key must be set on the target database before
any insert hits a Feistel trigger:

    ALTER DATABASE <db> SET app.obfuscation_key = '<64-hex-char-secret>';

This is normally handled at CNPG provisioning time; see TRA-810 for the
data cutover sequence.
