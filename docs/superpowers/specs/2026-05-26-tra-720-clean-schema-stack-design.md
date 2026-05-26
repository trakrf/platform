# TRA-720 design — clean schema stack with bigint surrogate IDs, unified Feistel ID generator, and tag_scans surrogate key

**Linear:** [TRA-720](https://linear.app/trakrf/issue/TRA-720) (folds in [TRA-836](https://linear.app/trakrf/issue/TRA-836))
**Date:** 2026-05-26
**Status:** Design — pending implementation plan
**Author:** Mike Stankavich

---

## Context and goals

The current schema on TimescaleDB Cloud has accumulated 44 migrations of schema evolution: tenant model pivots, column renames, denormalization removals, entity renames (`identifier` → `tag`, `identifier_scans` → `tag_scans`, `assets.identifier` → `external_key`, etc.). Multiple structural issues remain unresolved:

- **Surrogate PKs and FKs are `INT4`**, with hash-generated IDs scattering across the 31-bit namespace. Organizations and users sit ~60k below the int32 ceiling with <1k rows. This is the BB35 finding B7 that TRA-719 wire-only-int64-promoted; this ticket retires the wire/storage divergence.
- **Two parallel ID-generation functions** (`public.generate_hashed_id` for tenant-root tables, `trakrf.generate_permuted_id` for operational tables) with separate correctness bugs (MD5 collisions, bit-15 dropping rotate), incomplete keying (the per-org seed is hardcoded), and schema-hygiene asymmetry (one in `public`, one in `trakrf`).
- **`tag_scans` PK collides under burst insert rate** (TRA-836). The composite `(created_at, message_topic)` PK drops messages when replay/catchup clusters multiple same-topic messages into the same microsecond.
- **44 numbered migration files representing schema evolution** rather than schema design make the current truth hard to read. A reader must mentally apply 44 chained changes to know what each table looks like today.

Parallel to this, the platform is moving from TimescaleDB Cloud (on Railway) to CNPG operator on GKE (TRA-351 epic, TRA-810 data cutover). The cutover involves provisioning fresh CNPG databases and moving data via FDW. This creates a natural moment to land schema cleanup as part of the cutover rather than as a separate `ALTER COLUMN` migration on the live Cloud database.

This design captures TRA-720's contribution: a new, clean 10-file migration stack that the empty CNPG databases will be initialized with. TRA-810 owns the data transport (FDW pull from Cloud → CNPG) and the cutover orchestration; TRA-720 just produces the schema.

### Goals

- Surrogate PK/FK columns are `BIGINT` throughout. Wire format matches storage; no runtime cap, no `markSurrogateIDsInt64` postprocess, no int32 ceiling anywhere in the system.
- A single, correct, keyed-Feistel ID generator that replaces both legacy functions. Bijective by construction (no PK retry path), keyed by a real secret (Kerckhoffs-respecting).
- `tag_scans` gains a surrogate `id` to eliminate burst-rate PK collisions (TRA-836 fold-in).
- Migration stack reads as schema design (10 files organized by concern) rather than schema chronology (44 files preserving every dead end).
- All necessary Go-side cleanup (drop `SurrogateIDMax`, validate caps, swag annotations, `markSurrogateIDsInt64`) lands in the same PR; the new stack and the cleaned Go binary deploy together against CNPG.

### Non-goals (boundary with TRA-810)

- The FDW pull script that moves data Cloud → CNPG.
- The cutover orchestration (maintenance window, connect-string flip, Cloud decommission).
- Provisioning of CNPG databases (handled by infra, tracked under TRA-351).
- Setting `app.obfuscation_key` on the CNPG databases (handled at provisioning time by infra).
- Re-minting existing IDs through the new Feistel. TRA-810 adopted a natural-key FDW pull strategy: surrogate IDs are regenerated on the target side rather than preserved, so the source/target ID spaces are independent by construction.
- Ingester architecture redesign. Trigger-driven `process_tag_scans()` is a known interim approach, deferred until customer traffic justifies redesign.

---

## Decisions captured during brainstorming

| Decision | Choice |
|---|---|
| Generator function shape | Collapse the two legacy functions into a single `trakrf.generate_obfuscated_id` (keyed Feistel) used by all 9 obfuscated-PK tables. |
| Feistel block width | 52 bits (2 × 26-bit halves). Pure Feistel output range `[0, 2^52)`. |
| Feistel rounds | 6. |
| Round function | `pgcrypto.hmac(data, round_key, 'sha256')` truncated to 26 bits. |
| Key plumbing | Database-level GUC: `ALTER DATABASE trakrf SET app.obfuscation_key = '<64-hex-char secret>'`. Function reads via `current_setting('app.obfuscation_key')`. |
| Tier 2 SERIAL PKs | Widen `password_reset_tokens.id` and `org_invitations.id` to `BIGSERIAL` (BIGINT IDENTITY) for schema uniformity. |
| Migration stack numbering | Restart at `000001`. |
| Seed data | Drop the `sample_data` migration; move dev seeds to a separate recipe outside the migration directory. |
| Down migrations | None for the foundational 10 files (up-only). Post-cutover migrations (`000011_…`) follow the existing up+down convention. |
| TRA-836 fold-in | `tag_scans` gains `id BIGINT GENERATED ALWAYS AS IDENTITY`; PK changes from `(created_at, message_topic)` to `(created_at, id)`. |
| Collision handling | No retry path needed; bijection guaranteed by Feistel construction. |
| Ticket scope split | TRA-720 owns clean schema stack + Go cleanup. TRA-810 owns data transport, cutover, decommission. |
| Old migrations | Deleted from the directory. Git tag `pre-tra-720` preserves history. New `backend/migrations/README.md` references the tag and documents the up-only foundation convention. |

---

## Architecture and scope

TRA-720 produces:

1. **A new 10-file migration stack** under `backend/migrations/`, replacing the existing 44 files. Applies cleanly to an empty Postgres+TimescaleDB instance and produces the canonical end-state schema.
2. **A Go reference implementation** of the Feistel ID generator at `backend/internal/obfuscatedid/`, used for test-vector generation and as a parity oracle against the PL/pgSQL implementation.
3. **Go-side cleanup** removing all the `int32`-ceiling plumbing (cap constants, validate caps, swag annotations, postprocess function, error messages, spec description paragraph). Shipped in the same PR.
4. **A new `backend/migrations/README.md`** documenting the design choice (clean schema, not chronological), pointing at the `pre-tra-720` git tag for historical context, and codifying the up-only foundation convention.
5. **A new `just db:diff-old-vs-new` recipe** that verifies the new stack is schema-equivalent to the old stack (modulo the intended bigint/Feistel/tag_scans-PK diffs).

TRA-720 does not include the FDW pull script, the maintenance window, the connect-string cutover, or any infra plumbing. Those belong to TRA-810.

### Verification gate before TRA-810 can consume

1. Apply the new stack to a fresh CNPG instance (GKE preview).
2. `pg_dump --schema-only` diff against a fresh apply of the current 44-migration stack matches the expected-diff allowlist (defined in the "Test plan" section).
3. The existing contract test suite, Go integration tests, and frontend test suite all pass against the new stack.
4. Manual smoke (signup → login → asset/location/tag CRUD → bulk import → tag scan) passes against the new stack.
5. Feistel parity test (Go ↔ PL/pgSQL) byte-equal on shared vectors.

---

## Migration stack file layout

```
backend/migrations/
  000001_extensions_and_schema.up.sql
  000002_id_generator.up.sql
  000003_organizations_and_users.up.sql
  000004_locations.up.sql
  000005_scan_devices_and_points.up.sql
  000006_assets.up.sql
  000007_tags.up.sql
  000008_scan_hypertables.up.sql
  000009_bulk_import_and_api_keys.up.sql
  000010_stored_procedures.up.sql
  README.md
```

### What's in each file

| # | File | Contents |
|---|---|---|
| 1 | `extensions_and_schema` | `CREATE EXTENSION timescaledb`, `CREATE EXTENSION pgcrypto` (explicit — needed for `hmac()` in the Feistel function; Cloud had it implicitly via TimescaleDB Cloud defaults but CNPG won't), `CREATE SCHEMA trakrf`. No `ltree` (was used by dropped `locations.path`, never readded elsewhere). |
| 2 | `id_generator` | `trakrf.generate_obfuscated_id()` (keyed Feistel — full design in next section). `trakrf.update_updated_at_column()` (unchanged from current — no IDs). Defined before any table so `CREATE TRIGGER` can reference them. |
| 3 | `organizations_and_users` | `org_role` enum. `organizations` (bigint id, no RLS — matches current state per 000022). `users` (bigint id with `is_superadmin`, `last_org_id`, no RLS per 000020). `org_users` (composite PK `(org_id, user_id)` both bigint, no RLS per 000020, `role org_role`). `org_invitations` (`id BIGSERIAL`, bigint `org_id` + `invited_by`). `password_reset_tokens` (`id BIGSERIAL`, bigint `user_id`). All update triggers attached. |
| 4 | `locations` | `locations` table with bigint `id`, `org_id`, `parent_location_id`. Column is `external_key` (post-000036). No `path`, no `depth` (post-000042). Indexes including partial unique `(org_id, external_key) WHERE deleted_at IS NULL`. Insert trigger calling `generate_obfuscated_id('location_seq')`. `no_self_reference CHECK (id != parent_location_id)`. RLS policy with `::BIGINT` cast. |
| 5 | `scan_devices_and_points` | `scan_devices` (column name `identifier` — never renamed, per current state). `scan_points` with bigint FKs to scan_devices and locations. Indexes, triggers, RLS. |
| 6 | `assets` | `assets` table — column is `external_key` (post-000037), no `current_location_id` (post-000043), no `type` column (post-000035). Indexes, partial unique `(org_id, external_key) WHERE deleted_at IS NULL`, trigger, RLS. |
| 7 | `tags` | `tags` table (entity formerly known as `identifiers`, renamed in 000033). `tag_target` CHECK constraint (mutually exclusive asset_id / location_id). Partial unique `(org_id, type, value) WHERE deleted_at IS NULL`. Trigger calling `generate_obfuscated_id('tag_seq')`. RLS. |
| 8 | `scan_hypertables` | `tag_scans` with new `id BIGINT GENERATED ALWAYS AS IDENTITY` column; PK is `(created_at, id)` per TRA-836. `create_hypertable`, 1-day chunks, 30-day retention. `asset_scans` (bigint FKs, `tag_scan_id BIGINT`, composite PK `(timestamp, org_id, asset_id)` unchanged — intentional dedup-by-content). `create_hypertable`, 1-day chunks, 365-day retention. No RLS on hypertables. |
| 9 | `bulk_import_and_api_keys` | `bulk_import_jobs` with bigint id+org_id, the corrected `valid_row_counts` CHECK constraint (post-000026), `tags_created` counter (post-000025). RLS. `api_keys` with bigint id+org_id+created_by+created_by_key_id (post-000029), `api_keys_creator_exactly_one` CHECK constraint. No RLS on api_keys (intentional per 000027 comment — middleware reads before session GUC is set). |
| 10 | `stored_procedures` | `process_tag_scans()` (current effective body from 000037, all `INT` declarations widened to `BIGINT`, leading comment flagging trigger-driven ingestion as known interim). `create_asset_with_tags()` (current body from 000043, bigint throughout). `create_location_with_tags()` (current body from 000036, bigint throughout). Trigger on `tag_scans` attaching `process_tag_scans`. |

### Sequences

Nine obfuscated-ID sequences for the Feistel-keyed tables: `organization_seq`, `user_seq`, `location_seq`, `scan_device_seq`, `scan_point_seq`, `asset_seq`, `tag_seq`, `bulk_import_job_seq`, `api_key_seq`. All created with explicit `AS BIGINT`.

Two implicit `BIGSERIAL`-backed sequences for `org_invitations` and `password_reset_tokens`.

One implicit IDENTITY sequence for `tag_scans.id`.

### Three surrogate-ID strategies

The new schema uses three distinct ID strategies, each fit to its use case:

| Strategy | Used by | Why |
|---|---|---|
| **Keyed Feistel (obfuscated, output range `[0, 2^52)`)** | `organizations`, `users`, `locations`, `scan_devices`, `scan_points`, `assets`, `tags`, `bulk_import_jobs`, `api_keys` | Wire-exposed entities. Need pseudo-random spread (no enumeration via incrementing) AND bijective output (no PK retry). |
| **`BIGINT GENERATED ALWAYS AS IDENTITY` (monotonic)** | `tag_scans` | Internal append-only hypertable, high insert rate, never wire-exposed. Sequence-backed is cheapest. |
| **No surrogate (composite content PK)** | `asset_scans` | Derived hypertable; dedup-by-content is intentional. Multiple raw tag reads of the same asset in the same µs collapse to one logical asset-scan event. PK is `(timestamp, org_id, asset_id)`. |

### `BIGSERIAL` for `org_invitations` and `password_reset_tokens`

These tables don't need obfuscation (internal, short-lived, never URL-routed). Monotonic IDENTITY is cheaper and clearer. Widening from int4-backed `SERIAL` to int8-backed `BIGSERIAL` is for schema uniformity — no operational requirement, just consistency so every surrogate PK in the schema is bigint.

---

## The Feistel ID generator

### Cryptographic parameters

| Parameter | Value | Rationale |
|---|---|---|
| Block size | 52 bits (2 × 26-bit halves) | Output range `[0, 2^52)`. Stays below SPA's 2^53 f64 mantissa limit. |
| Rounds | 6 | Provably indistinguishable from a random permutation after 4 rounds (Luby–Rackoff); 6 is conventional safety margin for format-preserving encryption. |
| Round function | `pgcrypto.hmac(data, round_key, 'sha256')` truncated to 26 bits | Standard PRF, C-implemented in pgcrypto. |
| Round keys | `HMAC-SHA256(master_key, 'round-' \|\| round_index)` | Derived per-round from the master key. No explicit KDF needed; pgcrypto's HMAC is sufficient. |
| Master key | 32-byte secret in hex, read from `current_setting('app.obfuscation_key')` | Set once per environment via `ALTER DATABASE`. Decoded to bytea via `decode(…, 'hex')`. |
| Output transform | `(L << 26) \| R` | Pure Feistel: combine halves. Probability of id=0 is 1/2^52 ≈ 2e-16 per insert (not handled; bigint 0 is a valid PG value). |
| Overflow guard | `RAISE EXCEPTION` if `nextval()` returns ≥ 2^52 | Defensive. Unreachable in practice (4.5 quadrillion per table). |

### Why HMAC-SHA256, not AES

`pgcrypto.hmac()` is C-implemented and fast enough at our insert rates (~microseconds per call, ~12 calls per Feistel run ≈ tens of µs per insert). AES-based FPE constructions (FF1/FF3) would be marginally lighter but require a dedicated extension or careful PL/pgSQL implementation. HMAC-Feistel is the lower-risk choice for our scale.

### PL/pgSQL sketch

```sql
CREATE OR REPLACE FUNCTION trakrf.generate_obfuscated_id()
RETURNS TRIGGER AS $$
DECLARE
    seq_name TEXT := TG_ARGV[0];
    seq_id   BIGINT;
    master_key BYTEA;
    L BIGINT;
    R BIGINT;
    L_new BIGINT;
    round_idx INT;
    round_key BYTEA;
    f_out BIGINT;
    MASK26 CONSTANT BIGINT := (1::bigint << 26) - 1;
BEGIN
    seq_id := nextval(seq_name);
    IF seq_id >= (1::bigint << 52) THEN
        RAISE EXCEPTION 'Sequence % overflow: % >= 2^52', seq_name, seq_id;
    END IF;

    master_key := decode(current_setting('app.obfuscation_key'), 'hex');

    L := (seq_id >> 26) & MASK26;
    R := seq_id & MASK26;

    FOR round_idx IN 1..6 LOOP
        round_key := hmac(('round-' || round_idx)::bytea, master_key, 'sha256');
        f_out := ('x' || encode(substring(
                    hmac(int8send(R), round_key, 'sha256')
                    FROM 1 FOR 4), 'hex'))::bit(32)::bigint & MASK26;
        L_new := R;
        R := L # f_out;
        L := L_new;
    END LOOP;

    NEW.id := (L << 26) | R;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

Function is `STABLE` (reads GUC, advances sequence — not `IMMUTABLE`).

### Go reference and parity test

`backend/internal/obfuscatedid/` package implements the same Feistel in Go:

```go
package obfuscatedid

const (
    blockBits = 52
    halfBits  = 26
    mask26    = (uint64(1) << halfBits) - 1
    rounds    = 6
)

func Encrypt(masterKey []byte, seqValue uint64) uint64 {
    if seqValue >= (1 << blockBits) {
        panic("sequence overflow")
    }
    L := (seqValue >> halfBits) & mask26
    R := seqValue & mask26
    for i := 1; i <= rounds; i++ {
        rk := roundKey(masterKey, i)
        L, R = R, L ^ f(rk, R)
    }
    return (L << halfBits) | R
}
```

Purpose:
1. **Test-vector oracle.** Shared `testdata/vectors.json` records `(master_key, seq_value, expected_id)` tuples. Loaded by both Go unit tests and an SQL integration test. Drift between Go and PL/pgSQL outputs breaks the test.
2. **Standalone reference for documentation.** Future readers see the Feistel structure in typed Go code instead of having to reverse-engineer the PL/pgSQL.

### Key provisioning (TRA-810 / infra concern)

```sql
ALTER DATABASE trakrf SET app.obfuscation_key = '<64-hex-char secret>';
```

Run once per environment during CNPG provisioning. Secret managed in whatever secret store the infra team already uses (1Password / Vault / GCP Secret Manager). Rotation = `ALTER DATABASE … SET app.obfuscation_key = '<new>'` + drain sessions. Critical: rotating the key changes the Feistel output for subsequent inserts; existing rows unaffected.

---

## RLS policies, stored procedures, constraints, indexes

### RLS policies (6 total, identical shape)

```sql
CREATE POLICY org_isolation_<table> ON trakrf.<table>
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
```

Applied to: `locations`, `scan_devices`, `scan_points`, `assets`, `tags`, `bulk_import_jobs`.

No RLS on: `organizations`, `users`, `org_users`, `org_invitations`, `password_reset_tokens`, `api_keys`, `tag_scans`, `asset_scans`. Each is intentional (auth pre-session, hypertable, etc.); see the respective migration in the old stack for rationale.

The only difference from the current Cloud policies: cast is `::BIGINT` instead of `::INT`.

### Stored procedures (file 000010)

| Function | Source | Changes |
|---|---|---|
| `trakrf.process_tag_scans()` | Current body from 000037 | `topic_org_id INT` → `topic_org_id BIGINT`. Leading comment flagging trigger-driven ingestion as known interim. |
| `trakrf.create_asset_with_tags()` | Current body from 000043 | Signature `p_org_id INT` → `BIGINT`, returns `(asset_id BIGINT, tag_ids BIGINT[])`, locals all bigint. |
| `trakrf.create_location_with_tags()` | Current body from 000036 | Same widening pattern as `create_asset_with_tags`. |

Go callers use positional `SELECT *` binding (verified in `storage/assets.go`, `storage/locations.go`), so param-name changes are safe.

### Constraints preserved

| Table | Constraint |
|---|---|
| `org_users` | `valid_status CHECK (status IN ('active', 'inactive', 'suspended', 'invited'))`. `valid_role` is replaced by the `org_role` enum (no longer a check constraint). |
| `tags` | `tag_target CHECK ((asset_id IS NOT NULL AND location_id IS NULL) OR (asset_id IS NULL AND location_id IS NOT NULL))` |
| `bulk_import_jobs` | `valid_row_counts CHECK (processed_rows >= 0 AND failed_rows >= 0 AND processed_rows + failed_rows <= total_rows)` (corrected version from 000026, not the broken original from 000013) |
| `api_keys` | `api_keys_creator_exactly_one CHECK ((created_by IS NOT NULL) <> (created_by_key_id IS NOT NULL))` |
| `locations` | `no_self_reference CHECK (id != parent_location_id)` |

### Update triggers

`update_updated_at_column()` attached BEFORE UPDATE on every table with an `updated_at` column: organizations, users, org_users, locations, scan_devices, scan_points, assets, tags. NOT on org_invitations or password_reset_tokens (no `updated_at`).

### Indexes

All current indexes ported forward with their current names. Notable: every entity table has its appropriate partial-unique index `(org_id, external_key|identifier|…) WHERE deleted_at IS NULL`, time-DESC indexes on hypertables, partial WHERE-active indexes where they exist today. No gratuitous renames.

---

## Test plan

### Level 1 — Feistel correctness

Hardcoded test vectors in `backend/internal/obfuscatedid/testdata/vectors.json`, byte-equal verification between Go and PL/pgSQL. Bijection sample over `[1, 10_000]` (assert no collisions). Output-range check (every output in `[0, 2^52)`). Key sensitivity (avalanche check on neighbouring keys).

### Level 2 — Schema-equivalence to current Cloud

`just db:diff-old-vs-new` recipe:

1. Spins up two Postgres+TimescaleDB containers.
2. Applies all 44 current migrations to one (`old`).
3. Applies the new 10-file stack to the other (`new`).
4. `pg_dump --schema-only --no-owner --no-privileges` both.
5. Compares against an expected-diff allowlist.

Expected diffs:

| Diff | Why |
|---|---|
| `integer` → `bigint` on every surrogate PK/FK column | The whole point |
| `public.generate_hashed_id` absent on `new` | Collapsed |
| `trakrf.generate_permuted_id` absent on `new` | Collapsed |
| `trakrf.generate_obfuscated_id` present on `new` only | New unified function |
| `::INT` → `::BIGINT` in 6 RLS policy quals | Cast widened |
| `tag_scans.id` column on `new` only; PK shape change | TRA-836 fold-in |
| `pgcrypto` extension explicit on `new` | Was implicit on Cloud |
| `ltree` extension on `old` only | Dropped |
| Function bodies have `BIGINT` locals/params | Widening |
| `password_reset_tokens.id` / `org_invitations.id` int4 → int8 | Tier 2 widening |

Any diff outside the allowlist = regression to investigate.

### Level 3 — Application integration

Existing test surfaces, all against local docker-compose'd new-stack DB:

- `just backend test`
- `just frontend test`
- `just contract-tests`
- Manual smoke: signup, login, asset/location/tag CRUD, bulk import
- `check-spec-sync.sh` (or equivalent) — OpenAPI ↔ runtime consistency
- New: Feistel parity test, integration-level, exercises real DB

### Level 4 — Data-shape (TRA-810 territory, listed for handoff)

Post-FDW move:

- Row count parity per table (Cloud → CNPG).
- Sample ID-keyed lookups byte-identical between source and target.
- Two `BIGSERIAL` sequences (`password_reset_tokens_id_seq`, `org_invitations_id_seq`) advanced past max migrated ID via `setval()`. These are monotonic, so collision-avoidance requires explicit advancement.
- The 9 Feistel sequences do **not** need `setval()` advancement. TRA-810's natural-key FDW pull regenerates surrogate IDs on the target, so source/target ID spaces are independent — fresh sequences from 1 are correct.
- `app.obfuscation_key` GUC verified set on target DB before any new insert hits a Feistel trigger.
- Every entity row on the target has `id` in `[0, 2^52)` — confirms IDs were minted by the new Feistel rather than copied across or otherwise corrupted.

---

## Operational sequencing

### What lands in the TRA-720 PR

**Adds:**
- `backend/migrations/000001_extensions_and_schema.up.sql` through `000010_stored_procedures.up.sql`
- `backend/migrations/README.md`
- `backend/internal/obfuscatedid/` package (Go reference, parity test, shared test vectors)
- `just db:diff-old-vs-new` recipe in the appropriate justfile

**Deletes:**
- `backend/migrations/000001_prereqs.{up,down}.sql` through `000044_cascade_soft_delete_tags.{up,down}.sql`

**Edits (Go cleanup):**
- `httputil/parseid.go`: remove `SurrogateIDMax` constant
- `httputil/listparams.go`: remove offset cap branch
- `models/location/*.go`: drop `validate:"omitempty,min=1,max=2147483647"` tags (and any other affected models)
- All handlers carrying `@Param ... maximum(2147483647) format(int32)` swag annotations: drop the qualifiers
- `apispec/postprocess.go`: remove `markSurrogateIDsInt64`, `isSurrogateIDName`, and the call site
- The three "must be a positive integer ≤ %d" runtime error messages: revert to plain wording
- "Surrogate ID width" paragraph in `info.description` source: remove

**At merge time:**
- Git tag `pre-tra-720` created on the last pre-merge commit, pushed to origin
- Cloud envs (preview, prod) pinned to that tag (or otherwise prevented from auto-deploying from main) so embedded migrations don't break them

### Exit criteria

TRA-720 marked **Done** when:

1. New 10-file stack applies cleanly to an empty Postgres+TimescaleDB.
2. `just db:diff-old-vs-new` passes against the allowlist.
3. `just backend test`, `just frontend test`, `just contract-tests` all green against local docker-compose'd new-stack DB.
4. Same suite green against GKE preview CNPG (when psql access lands).
5. Feistel parity test green (Go ↔ PL/pgSQL byte-equal).
6. Manual smoke green on GKE preview CNPG.
7. PR merged to `main`.
8. Cloud envs pinned to `pre-tra-720` tag (or `MIGRATIONS_MODE=disabled` equivalent deployed).
9. TRA-836 closed as folded-into-TRA-720.

### Handoff to TRA-810

TRA-720's PR description publishes the operational steps TRA-810 needs to execute (also commented on TRA-810 itself):

> Schema stack ready in `backend/migrations/`. To execute the cutover:
>
> 1. Provision empty `trakrf` database on target CNPG (preview, then prod).
> 2. Set the obfuscation key: `ALTER DATABASE trakrf SET app.obfuscation_key = '<secret>'`.
> 3. Apply the new migration stack (happens automatically on first app boot via golang-migrate, or run explicitly).
> 4. Develop and prove the FDW pull script using GKE preview ← Cloud preview as the test bench.
> 5. Execute against prod CNPG: maintenance window, FDW pull from Cloud, verify row counts and sample lookups, advance the two `BIGSERIAL` sequences past their migrated max IDs.
> 6. Cutover connect strings.
> 7. Soak with Cloud in read-only fallback (24–48h).
> 8. Decommission Cloud.

### Rough sequencing (informational)

| Phase | What | Where |
|---|---|---|
| 0 | Design (this doc) | Conversation |
| 1 | Implementation in worktree | `.worktrees/tra-720-bigint-migration`, local docker compose |
| 2 | Apply + test on GKE preview CNPG | When psql access lands |
| 3 | PR review, merge to main | |
| 4–8 | TRA-810 picks up | Cutover, decommission |

Disposable GKE preview (no data) makes phase 2 trivially reversible: drop schema, re-apply, re-test as needed.

---

## Risks and open considerations

### Risks

- **PL/pgSQL Feistel implementation bugs.** Mitigated by the Go reference + parity test. Any deviation between Go and PL/pgSQL outputs surfaces in CI.
- **TimescaleDB extension version parity between Cloud and CNPG.** CNPG ships its own TimescaleDB build; subtle behavior differences in hypertable creation, retention policies, or chunk management could surface. Mitigated by running the full test suite against GKE preview CNPG before merge.
- **`pgcrypto` extension availability on CNPG.** Standard extension, ships with most Postgres builds, but worth confirming explicitly on the CNPG image during phase 2.
- **Coordination with TRA-810 for Cloud-env pinning.** If Cloud envs auto-deploy from main and we forget to pin, the post-TRA-720 build's embedded migrations will fail on Cloud startup. Mitigation: confirm pinning before merging the PR.
- **`session_replication_role = replica` during FDW pull (TRA-810 concern).** Disables triggers, but RLS still applies unless explicitly bypassed. The pull executor needs `BYPASSRLS` or `SET row_security = OFF`. Not TRA-720's problem to solve, but worth flagging in the handoff.

### Open considerations

- **OBfuscation key length.** 32 bytes (64 hex chars) is the natural choice for HMAC-SHA256 master key. Confirm with infra that this length fits whatever the secret-storage convention is.
- **Sequence cache size.** PG sequences default to cache=1. At our entity-insert rates (low), no tuning needed. If `tag_scans` ever moves to a Feistel-keyed surrogate (not in this plan), the IDENTITY sequence cache would want bumping.
- **`process_tag_scans()` performance.** The trigger does N inserts per incoming MQTT message. At fixed-reader scale (1000+ msg/sec sustained), this becomes a bottleneck. Out of scope for TRA-720 but flagged in the migration comment as known-interim.

---

## Cross-references

- [TRA-720](https://linear.app/trakrf/issue/TRA-720) — this ticket
- [TRA-836](https://linear.app/trakrf/issue/TRA-836) — `tag_scans` PK collision, folded into TRA-720
- [TRA-810](https://linear.app/trakrf/issue/TRA-810) — M3 data cutover (consumes this design)
- [TRA-351](https://linear.app/trakrf/issue/TRA-351) — GKE migration epic
- [TRA-719](https://linear.app/trakrf/issue/TRA-719) — BB35 omnibus including B7 wire-only int64 promotion
- [TRA-718](https://linear.app/trakrf/issue/TRA-718) — BB35 findings parent (B7 origin)
