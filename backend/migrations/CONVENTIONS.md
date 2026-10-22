# Migration Conventions

## Path-derived columns

When a migration changes `update_location_path()`, `cascade_location_path()`,
or anything else that derives `locations.path` from `external_key` / parent,
call `SELECT trakrf.recompute_location_paths();` from the same migration.

The triggers only re-fire on column-scoped INSERT/UPDATE events, so existing
rows do not pick up new derivation rules without an explicit recompute.
`recompute_location_paths()` is idempotent — safe to call against an
already-canonical table.

History: TRA-577 introduced this convention after BB15 found 704 of 713
preview-DB rows held non-canonical paths because the canonical rule changed
in 000036 without a recompute.

## Idempotent where practical

Write DDL so that running a migration a second time is a no-op:
`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`,
`ADD COLUMN IF NOT EXISTS`, `CREATE OR REPLACE FUNCTION` / `VIEW`,
`DROP … IF EXISTS`. Where Postgres has no guarded form (constraints, policies,
some `ALTER TYPE`s), wrap the statement in a `DO` block that checks the catalog
first.

This is a backstop, not the guard. The guard is the file record below; this
only decides what a re-run costs once something has gone wrong. On preview a
renumbered migration re-ran a plain `CREATE TABLE subscription_notices` against
a database that already had it from the PR's earlier number. That was `42P07`
and a dirty ledger that wedged every deploy for days, where `IF NOT EXISTS`
would have been a no-op.

## An open PR keeps its migration numbers

Preview composes main with every open, non-draft PR based on main, and its
migrate Job applies all of their migrations to one shared database. Once a PR's
migration has run there, its number is spent on that database:
golang-migrate stores only the number, so a different file later given the same
number reads as already applied.

So:

- **Pick a number above main's highest and above every open PR's.** The
  `sync-preview` workflow leaves out a PR whose new number is at or below
  main's highest or the highest preview has applied, or is already claimed by
  another PR's file — the one preview applied, else the earlier-opened one —
  and comments on the PR saying which number to move above. Left out, its migrations never reach
  preview's database, so renumbering then costs nothing.
- **Once a PR is in preview, merge it with its numbers unchanged.** If main
  takes the number first, preview's database must be reconciled alongside the
  renumber; that is an infra operator action, not something to discover from a
  failed deploy. A PR that leaves the preview build while carrying migrations,
  or rewrites one already applied there, fails the `sync-preview` run.
- **The migrator refuses what slips through.** `server migrate` records the
  file name and SHA-256 applied at each version in
  `trakrf.schema_migration_files`, and refuses, before writing anything, when
  the image's file at an applied version is missing or different. Its message
  names both files and the statement that re-points the record once the schema
  has been checked against the image's file.

History: preview applied an open PR's provisional 000042/000043; main
later gave 42 and 43 to other files and preview skipped them for over two
weeks, then the PR's renumbered 000050 re-ran its `CREATE TABLE` and wedged.
