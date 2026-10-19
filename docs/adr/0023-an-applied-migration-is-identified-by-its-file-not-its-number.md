# ADR 0023 — An applied migration is identified by its file, not its number

Date: 2026-10-19
Status: Proposed
Tracking: TRA-1287 (this record), ADR 0003 (explicit schema placement), TRA-1077 (`checksums.txt`)

## Context

golang-migrate's whole ledger is one `(version, dirty)` row. A database at 43
is "done through 43", whichever file 43 was when it ran. `checksums.txt`
(TRA-1077) stops an applied file being edited inside one history, but nothing
stopped a number being applied as one file by one build and given to a
different file by the next.

Preview composes main with every open PR and applies all of their migrations
to one database, so it hit exactly that. A PR's provisional 42 and 43 were
applied there; main then gave both numbers to other files, and preview skipped
main's 42 and 43 without an error for over two weeks. A second PR's
provisional 49 did the same to main's 49.

## Decision

1. **`server migrate` records the file applied at each version** in
   `trakrf.schema_migration_files (version, filename, sha256, applied_at)`.
   The runner creates and owns it, as it does the ledger: it is not a
   migration, so it covers migrations older than itself, and it lives in
   `trakrf` so a development `DROP SCHEMA trakrf CASCADE` takes it along.
   Write grants other than the owner's are revoked, since the schema's default
   privileges would otherwise let the application rewrite it.
2. **It is seeded trust-on-first-use**, on every environment including prod,
   from the image that first runs with it. That is exact on prod, whose applied
   files are all guarded by `checksums.txt`.
3. **Migrate refuses, before writing anything, when an applied version's file
   is missing from the image or differs in name or SHA-256** — a third
   preflight beside the stray-ledger and ownership checks, with the same
   fail-safe property: nothing written, ledger clean, the old pod serving.
   The digest is `checksums.txt`'s, so a file has one identity everywhere.
4. **Repair is an operator action.** The refusal prints the statement that
   re-points the record, which is correct only once the schema has been checked
   against the image's file. The runner never repairs itself.
5. **Preview keeps provisional numbers out of its database** to begin with:
   `sync-preview` leaves out a PR whose migration number is not above main's or
   is claimed by an earlier PR, and fails when a rebuild drops a migration the
   previous build carried.

Timestamp-based versions were ruled out: golang-migrate will not apply a
version below the current one, so a PR merged after a newer-stamped one would
be skipped without an error — the same failure, made routine.

## Consequences

- A reused number now stops a deploy instead of silently leaving a schema
  behind. That is louder, deliberately: preview's River tables were missing for
  weeks because the failure was quiet.
- Rolling back to an image older than the database already failed in
  golang-migrate ("file does not exist"); it now fails with the reason.
- One permanent table on every database. Undoing this decision is
  `DROP TABLE trakrf.schema_migration_files` plus removing the preflight.
