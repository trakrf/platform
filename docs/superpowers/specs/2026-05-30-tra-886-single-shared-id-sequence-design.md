# TRA-886 — Single shared sequence for surrogate ids

## Problem

Every Feistel-minted table has its own sequence (`asset_seq`, `tag_seq`, …) and a
`BEFORE INSERT` trigger that runs `nextval(<that_seq>)` through one shared-key
Feistel cipher. Because the key is shared across tables, ordinal *n* always
encrypts to the same id regardless of source sequence — so asset #n and tag #n
get the **same** `id`. Cross-type id equality is therefore structural.

It is benign today (resolution is type-scoped; same-type duplication is
impossible by injective Feistel + PK), but it is a recurring security-review and
data-audit question (CONVERGED_FINDINGS C1, round 2.5).

## Decision

Collapse to a single shared sequence `trakrf.id_seq` feeding the existing,
unchanged Feistel. One global counter ⇒ no two rows ever share an ordinal ⇒ no
two rows share an id. Globally unique by construction.

Rejected alternatives (per ticket): per-table distinct keys (only converts
structural equality into rare random collisions) and UUIDs (sacrifices the int53
SPA constraint TRA-720 deliberately holds).

## Scope

Authored **into the rebased clean schema stack** (TRA-720 foundation files,
`000001`–`000010`, up-only), edited in place — not an incremental `ALTER`. The
generator math, 52-bit block, 2^53 ceiling, and key are all untouched; only the
input source moves from N sequences to 1.

### Changes

1. **`backend/migrations/000002_id_generator.up.sql`**
   - Add `CREATE SEQUENCE trakrf.id_seq AS BIGINT;` (the one and only surrogate-id
     source).
   - Simplify `trakrf.generate_obfuscated_id()` to call
     `nextval('trakrf.id_seq')` directly and **drop the `TG_ARGV[0]` argument**.
     This makes `id_seq` the structurally-only source — a per-table sequence
     cannot be reintroduced by passing a different arg.

2. **Per-table files — remove the per-table `CREATE SEQUENCE` and drop the
   trigger arg** (trigger becomes `EXECUTE FUNCTION trakrf.generate_obfuscated_id()`):
   - `000003_organizations_and_users.up.sql` — `organization_seq`, `user_seq`
   - `000004_locations.up.sql` — `location_seq`
   - `000005_scan_devices_and_points.up.sql` — `scan_device_seq`, `scan_point_seq`
   - `000006_assets.up.sql` — `asset_seq`
   - `000007_tags.up.sql` — `tag_seq`
   - `000009_bulk_import_and_api_keys.up.sql` — `bulk_import_job_seq`, `api_key_seq`

   Nine sequences and nine trigger calls in total.

### Out of scope (confirmed, do not touch)

- **`external_key`** — derives its next value from `MAX(external_key …)` over
  existing rows in Go (`storage/assets.go`, `storage/locations.go`); reads no
  surrogate sequence. Leave it.
- **Non-Feistel ids** — `org_invitations`, `password_reset_tokens`,
  `refresh_tokens` (`BIGSERIAL`) and `tag_scans` (`GENERATED ALWAYS AS IDENTITY`)
  keep their own implicit sequences. The regression guard must **not** flag these.
- **TRA-810 cutover scripts** — FK remap is by natural key + old→new map,
  indifferent to how many sequences mint the new id. Run unchanged.

## Verification

Per ticket, skip migration-data validation (NADA is a single location; the rest
is test data; minor cutover-break risk is acceptable). The one guard worth
keeping:

- **SQL regression guard** in `backend/database/test/` (parallel to the existing
  `feistel_parity_test.sql` + `run_feistel_parity.sh` harness). Run against a
  freshly-migrated DB, it asserts:
  1. `trakrf.id_seq` exists.
  2. None of the nine per-table `*_seq` sequences exist.
  3. All nine surrogate-id `BEFORE INSERT` triggers exist and bind
     `trakrf.generate_obfuscated_id` with **zero** arguments.
  4. (Sanity) the four non-Feistel auto-id tables are explicitly allow-listed so
     their implicit sequences don't trip the "no stray sequence" check.

  A thin wrapper (`run_id_source_guard.sh`) runs it against `$PG_URL_LOCAL`,
  mirroring the parity harness (`\set ON_ERROR_STOP on`; `RAISE EXCEPTION` on
  failure, `RAISE NOTICE` on pass). Like `feistel_parity_test.sql`, this guard is
  a **manual/local** check — neither it nor the parity test is wired into CI or a
  `just` recipe today; the PR will note this so it's a deliberate, visible gap
  rather than an assumed-automated one.

- Existing `feistel_parity_test.sql` must still pass unchanged (generator math
  is untouched; same test vectors).

## Rollout / sequencing

1. Land in the schema stack (this PR).
2. **Preview re-migration is the rehearsal** — infra drops/recreates the preview
   CNPG DB and re-applies the foundation stack from scratch (golang-migrate has
   already marked `000002` etc. applied, so an in-place edit will not re-run on
   the existing DB — a full reset is required), then re-runs the TRA-810 cutover
   pull. Coordinated with infra over **cc2cc after this PR is green**, not before.
3. Prod cutover via TRA-810 propagates it (NADA comes across globally unique).

No surrogate id has escaped via API yet (NADA is UI-only, no live integrations),
so regeneration has zero external blast radius today.
