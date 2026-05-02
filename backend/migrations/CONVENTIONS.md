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
