# TRA-810 FDW Pull Cutover — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a natural-key FDW pull script that copies live entity rows from the legacy TimescaleDB Cloud preview database into a fresh CNPG preview database under the TRA-720 clean schema, regenerating all surrogate IDs via the in-database Feistel triggers. The output is a merged, validated script that the production cutover (out of scope) will execute against prod CNPG.

**Bonus reusability (called out by user 2026-05-26):** Because the script joins on natural keys and lets the target regenerate surrogate IDs, it works against *any* source schema whose entity tables have the same logical column names — including pre-TRA-720 (legacy v44) AND post-TRA-720 sources. That makes the same scripts reusable as a generic "snapshot preview into local dev" or "copy prod into a staging fixture" tool after the TRA-810 cutover ships. The justfile is structured so `cutover-preview` is a thin TRA-810-flavored alias on top of a parameterized `db-pull-nk` recipe.

**Architecture:**
- One-direction pull: target CNPG (under new 10-migration schema) opens a `postgres_fdw` connection to source Cloud Postgres preview and `INSERT INTO trakrf.X SELECT ... FROM cloud_source.X` per table in dependency order.
- Surrogate IDs are NOT preserved. Triggers stay enabled; `trakrf.generate_obfuscated_id` mints fresh Feistel IDs for every row. Cross-table FKs are resolved by joining on natural keys (`identifier`, `email`, `external_key`, `jti`, etc.).
- `WHERE deleted_at IS NULL` filter on every entity table — drops the API-testing tombstones (pre-launch there is no real customer history to preserve).
- Two-phase load for self-referential FKs (`locations.parent_location_id`, `api_keys.created_by_key_id`): phase 1 inserts with NULL self-FK, phase 2 UPDATEs by joining target-natural-key → source-natural-key → source-self-FK-id.
- Skipped tables: `bulk_import_jobs` (no natural key), `tag_scans` (no prod ingestion yet), `password_reset_tokens` (short-lived), `asset_scans` (expected empty pre-launch — verify, then copy if rows exist by remapping `(org_id, asset_id)` natural keys).

**Tech Stack:** PostgreSQL 16 + TimescaleDB, `postgres_fdw`, plain SQL scripts orchestrated by a `just` recipe, psql client.

**Spec sources:**
- TRA-720 design doc: `docs/superpowers/specs/2026-05-26-tra-720-clean-schema-stack-design.md`
- Strategy detail: TRA-810 Linear comment (`51750b2d-6a1a-4ef3-8c73-3f12c8048799`)
- Memory pointer: `project_tra810_fdw_pull_strategy.md`

**Test bench:** GKE preview CNPG (target, freshly migrated via TRA-720 stack) ← TimescaleDB Cloud preview (source, legacy 44-migration schema). Source connection: env var `$PG_URL_PREVIEW`. Target connection: env var `$PG_URL_GKE_PREVIEW` (already used in helm preview environment).

**Out of scope:**
- Production maintenance window, read-only flip, Cloud decommission (separate execution session).
- Dropping `trakrf_old.events` / `trakrf_old.messages` on Cloud (separate ticket scope, will be its own task).
- Provisioning the empty CNPG database (handled by infra, TRA-351).
- Setting `app.obfuscation_key` GUC on CNPG (handled by infra at provisioning time).
- `bulk_import_jobs`, `tag_scans`, `password_reset_tokens` — explicitly skipped.

---

## File Structure

All artifacts live under `backend/database/cutover/` in the platform repo. Each SQL file is independently runnable via `psql -v ON_ERROR_STOP=1 -f <file>` against the target CNPG. The orchestrator `just backend cutover-preview` runs them in order with verification gates between phases.

```
backend/database/cutover/
  README.md                         -- Runbook + scope + safety notes
  00_truncate_target.sql            -- (optional, --truncate flag) clear target entity tables
  01_fdw_setup.sql                  -- CREATE EXTENSION, SERVER, USER MAPPING, IMPORT FOREIGN SCHEMA
  02_organizations.sql              -- Pull organizations (root, no FKs)
  03_users_and_org_users.sql        -- Pull users (with last_org_id) then org_users
  04_locations.sql                  -- Two-phase: roots first, then children
  05_scan_devices_and_points.sql    -- scan_devices, then scan_points (FKs to devices + locations)
  06_assets.sql                     -- Pull assets
  07_tags.sql                       -- Pull tags (mutually-exclusive FK to assets or locations)
  08_api_keys.sql                   -- Two-phase: NULL created_by_key_id first, then UPDATE
  09_org_invitations.sql            -- Pull org_invitations
  10_asset_scans.sql                -- Skip if source empty; otherwise remap (org_id, asset_id)
  20_verify.sql                     -- Row counts, FK orphan checks, natural-key parity
  99_teardown.sql                   -- DROP USER MAPPING, SERVER, EXTENSION (no DROP DATABASE)
```

Plus two justfile entries in `backend/justfile`:

```
# Parameterized: pull live rows from any source-URL into any target-URL via natural keys.
# Optional --truncate flag clears the target's entity tables first (for local-dev-copy use).
db-pull-nk source-url target-url *flags: ...

# TRA-810 alias: pull from Cloud preview into GKE preview CNPG.
cutover-preview: ...
```

---

## Task 1: Scaffold the cutover directory and runbook

**Files:**
- Create: `backend/database/cutover/README.md`
- Create: `backend/database/cutover/.gitkeep` (placeholder; removed when first SQL file lands)

- [ ] **Step 1: Create the directory and runbook**

```bash
mkdir -p backend/database/cutover
```

Write `backend/database/cutover/README.md` with this exact content:

```markdown
# Natural-Key FDW Pull (TRA-810 cutover + general-purpose snapshot tool)

Natural-key FDW pull from one Postgres database to another. Originally built for TRA-810 (Cloud → CNPG cutover), but reusable: because surrogate IDs are regenerated by the target's triggers, the same scripts work against *any* source whose entity tables expose the canonical column names — pre-TRA-720 (legacy v44) or post-TRA-720. Use cases beyond cutover include snapshotting preview into local dev and seeding staging fixtures.

## What this does

Copies live entity rows (`WHERE deleted_at IS NULL`) from a source database to a target database under the TRA-720 schema. Source surrogate IDs are NOT preserved — target triggers regenerate fresh Feistel IDs. Cross-table FKs are resolved by natural-key joins.

## Prerequisites

1. Target database is fresh CNPG with the 10-file TRA-720 migration stack applied.
2. Target has `ALTER DATABASE trakrf SET app.obfuscation_key = '<64-hex>'` set.
3. Source database is reachable from the target (network path + credentials).
4. The role running these scripts on target is the table owner (so RLS policies don't filter rows — they're not FORCE).
5. Env vars set:
   - `PG_URL_GKE_PREVIEW` — target CNPG connection string
   - `PG_URL_PREVIEW` — source Cloud Postgres connection string

## Run

From repo root:

    # TRA-810 cutover (Cloud preview → GKE preview CNPG):
    just backend cutover-preview

    # General-purpose pull (any source → any target):
    just backend db-pull-nk "$SOURCE_URL" "$TARGET_URL"

    # Local-dev-copy use case (non-empty target → must truncate first):
    just backend db-pull-nk "$PG_URL_PREVIEW" "$PG_URL_LOCAL" --truncate

This applies 01..10 in order, then 20_verify.sql, then 99_teardown.sql. Each step
stops on first error (`-v ON_ERROR_STOP=1`). The optional `00_truncate_target.sql`
runs only with `--truncate` and is NEVER invoked by `cutover-preview` (production
cutover targets are always fresh by definition — truncating prod by accident is
the failure mode this guard prevents).

## Scope

In:  organizations, users, org_users, locations, scan_devices, scan_points,
     assets, tags, api_keys, org_invitations, asset_scans (if non-empty).
Out: bulk_import_jobs (no natural key), tag_scans (no prod ingestion),
     password_reset_tokens (short-lived).

All entity tables are filtered `WHERE deleted_at IS NULL`. Soft-deleted rows are
abandoned by design — pre-launch the source's deletes are API-test churn, not
real customer history.

## Production cutover

This script is the development + dry-run bench. Production cutover (maintenance
window, read-only flip, soak, Cloud decommission) is executed separately.
```

- [ ] **Step 2: Commit**

```bash
git add backend/database/cutover/README.md
git commit -m "docs(cutover): scaffold TRA-810 FDW pull directory and runbook"
```

---

## Task 1.5: Optional truncate-target script (for non-cutover reuse)

**Files:**
- Create: `backend/database/cutover/00_truncate_target.sql`

This is opt-in (`--truncate` flag on the justfile recipe). Not part of TRA-810 cutover itself — there the target is fresh by definition — but essential for the "snapshot into local dev" reuse path where you re-run against a non-empty target.

- [ ] **Step 1: Write the truncate script**

```sql
-- Optional pre-pull cleanup. ONLY runs when invoked explicitly via --truncate.
-- DO NOT run against production. Drops all entity-table rows on the target.
-- Sequences are restarted so Feistel inputs start fresh.
\set ON_ERROR_STOP on

TRUNCATE TABLE
    trakrf.asset_scans,
    trakrf.tags,
    trakrf.api_keys,
    trakrf.org_invitations,
    trakrf.assets,
    trakrf.scan_points,
    trakrf.scan_devices,
    trakrf.locations,
    trakrf.org_users,
    trakrf.users,
    trakrf.organizations
RESTART IDENTITY CASCADE;

DO $$ BEGIN RAISE NOTICE 'target entity tables truncated'; END $$;
```

- [ ] **Step 2: Commit**

```bash
git add backend/database/cutover/00_truncate_target.sql
git commit -m "feat(cutover): optional --truncate target reset for non-cutover reuse"
```

---

## Task 2: FDW setup and connectivity check

**Files:**
- Create: `backend/database/cutover/01_fdw_setup.sql`

- [ ] **Step 1: Write the FDW setup script**

The script must be idempotent (drop-if-exists then create). It parses source connection params from psql variables passed in by the justfile (`-v src_host=... -v src_port=... -v src_db=... -v src_user=... -v src_pw=...`).

```sql
-- TRA-810 — postgres_fdw setup pointing at legacy Cloud source.
-- Idempotent. Foreign objects live in schema `cloud_src`.

\set ON_ERROR_STOP on

DROP SCHEMA IF EXISTS cloud_src CASCADE;
DROP USER MAPPING IF EXISTS FOR CURRENT_USER SERVER cloud_src_srv;
DROP SERVER IF EXISTS cloud_src_srv CASCADE;
CREATE EXTENSION IF NOT EXISTS postgres_fdw;

CREATE SERVER cloud_src_srv
    FOREIGN DATA WRAPPER postgres_fdw
    OPTIONS (host :'src_host', port :'src_port', dbname :'src_db',
             fetch_size '5000');

CREATE USER MAPPING FOR CURRENT_USER SERVER cloud_src_srv
    OPTIONS (user :'src_user', password :'src_pw');

CREATE SCHEMA cloud_src;

-- Import only the tables we will read. The source `trakrf` schema is the live
-- v44 schema; foreign-table column types follow the source. BIGINT vs INT4 at
-- the source is fine — we re-mint IDs on the target side regardless.
IMPORT FOREIGN SCHEMA trakrf
    LIMIT TO (organizations, users, org_users, org_invitations, locations,
              scan_devices, scan_points, assets, tags, api_keys, asset_scans)
    FROM SERVER cloud_src_srv INTO cloud_src;

-- Smoke: must return >0 organizations or fail loudly.
DO $$
DECLARE n INT;
BEGIN
    SELECT count(*) INTO n FROM cloud_src.organizations WHERE deleted_at IS NULL;
    IF n = 0 THEN
        RAISE EXCEPTION 'FDW smoke failed: 0 live organizations visible at source';
    END IF;
    RAISE NOTICE 'FDW smoke OK: % live organizations visible at source', n;
END $$;
```

- [ ] **Step 2: Add justfile recipes (parameterized core + TRA-810 alias)**

Append to `backend/justfile` (working directory is `backend/` when this runs):

```just
# Natural-key FDW pull from source-url → target-url. Reusable beyond TRA-810
# (e.g., snapshot preview into local dev). Source must have the entity
# tables under `trakrf.*` with the canonical column names; target must be
# the TRA-720 v10 schema with `app.obfuscation_key` set. Surrogate IDs are
# regenerated by target triggers — works against pre- or post-TRA-720 sources.
#
# Flags:
#   --truncate    TRUNCATE target entity tables before pull (for local-dev-copy)
#
# Example:
#   just backend db-pull-nk "$PG_URL_PREVIEW" "$PG_URL_LOCAL" --truncate
db-pull-nk source-url target-url *flags='':
    #!/usr/bin/env bash
    set -euo pipefail
    SRC_URL="{{source-url}}"
    TGT_URL="{{target-url}}"
    FLAGS="{{flags}}"

    SRC_HOST=$(python3 -c "import urllib.parse,sys;u=urllib.parse.urlparse(sys.argv[1]);print(u.hostname)" "$SRC_URL")
    SRC_PORT=$(python3 -c "import urllib.parse,sys;u=urllib.parse.urlparse(sys.argv[1]);print(u.port or 5432)" "$SRC_URL")
    SRC_DB=$(python3 -c   "import urllib.parse,sys;u=urllib.parse.urlparse(sys.argv[1]);print(u.path.lstrip('/'))" "$SRC_URL")
    SRC_USER=$(python3 -c "import urllib.parse,sys;u=urllib.parse.urlparse(sys.argv[1]);print(u.username)" "$SRC_URL")
    SRC_PW=$(python3 -c   "import urllib.parse,sys;u=urllib.parse.urlparse(sys.argv[1]);print(u.password)" "$SRC_URL")

    PSQL="psql $TGT_URL -v ON_ERROR_STOP=1 \
        -v src_host=$SRC_HOST -v src_port=$SRC_PORT -v src_db=$SRC_DB \
        -v src_user=$SRC_USER -v src_pw=$SRC_PW"

    if [[ "$FLAGS" == *--truncate* ]]; then
        echo "==> TRUNCATE target entity tables (--truncate flag)"
        $PSQL -f database/cutover/00_truncate_target.sql
    fi

    for f in database/cutover/[0-9][0-9]_*.sql; do
        # Skip the optional truncate file in the normal sequence — handled above.
        [[ "$f" == *00_truncate_target.sql ]] && continue
        echo "==> $f"
        $PSQL -f "$f"
    done

# TRA-810 alias — Cloud preview source, GKE preview CNPG target.
# Requires PG_URL_PREVIEW (source) and PG_URL_GKE_PREVIEW (target) in env.
cutover-preview:
    just db-pull-nk "$PG_URL_PREVIEW" "$PG_URL_GKE_PREVIEW"
```

- [ ] **Step 3: Run the FDW setup against preview**

```bash
just backend cutover-preview
```

Expected: only `01_fdw_setup.sql` exists in the cutover directory, so it runs and emits `NOTICE: FDW smoke OK: <N> live organizations visible at source`. Exit code 0.

If the FDW server can't reach the source from inside the CNPG cluster, abort and resolve network/IP-allowlist with infra (related: trakrf/infra#116) before continuing.

- [ ] **Step 4: Commit**

```bash
git add backend/database/cutover/01_fdw_setup.sql backend/justfile
git commit -m "feat(cutover): TRA-810 FDW setup + just backend cutover-preview recipe"
```

---

## Task 3: Pull organizations

**Files:**
- Create: `backend/database/cutover/02_organizations.sql`

Organizations are the root of the dependency graph — no FK lookups needed. The trigger mints a fresh `id`; we just project the natural key plus content columns.

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull live organizations from Cloud source.
-- Natural key: identifier. Surrogate id regenerated by trigger.
\set ON_ERROR_STOP on

INSERT INTO trakrf.organizations
    (name, identifier, metadata, valid_from, valid_to, is_active,
     created_at, updated_at)
SELECT
    name, identifier, metadata, valid_from, valid_to, is_active,
    created_at, updated_at
FROM cloud_src.organizations
WHERE deleted_at IS NULL;

DO $$
DECLARE src_n INT; tgt_n INT;
BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.organizations WHERE deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.organizations;
    IF src_n <> tgt_n THEN
        RAISE EXCEPTION 'organizations count mismatch: src=% tgt=%', src_n, tgt_n;
    END IF;
    RAISE NOTICE 'organizations OK: % rows', tgt_n;
END $$;
```

- [ ] **Step 2: Run end-to-end**

```bash
just backend cutover-preview
```

Expected output includes `NOTICE: organizations OK: <N> rows` where N matches source live count (~873 per the 2026-05-26 snapshot).

- [ ] **Step 3: Spot-check a row**

```bash
psql "$PG_URL_GKE_PREVIEW" -c "SELECT id, identifier, name FROM trakrf.organizations ORDER BY created_at LIMIT 3;"
```

Expected: 3 rows with non-zero `id` values in `[0, 2^52)` and `identifier`/`name` matching source.

- [ ] **Step 4: Commit**

```bash
git add backend/database/cutover/02_organizations.sql
git commit -m "feat(cutover): TRA-810 pull organizations by identifier"
```

---

## Task 4: Pull users and org_users

**Files:**
- Create: `backend/database/cutover/03_users_and_org_users.sql`

Users carry `last_org_id` which resolves via `organizations.identifier`. `org_users` is a junction whose composite PK is `(org_id, user_id)` — both FKs resolve via natural keys.

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull users and org_users.
-- users.last_org_id resolved via organizations.identifier.
-- org_users FKs resolved via org.identifier + user.email.
\set ON_ERROR_STOP on

INSERT INTO trakrf.users
    (email, name, last_login_at, password_hash, settings, metadata,
     is_superadmin, last_org_id, created_at, updated_at)
SELECT
    s.email, s.name, s.last_login_at, s.password_hash, s.settings, s.metadata,
    s.is_superadmin,
    t_org.id,                 -- map source.last_org_id → target.organizations.id via identifier
    s.created_at, s.updated_at
FROM cloud_src.users s
LEFT JOIN cloud_src.organizations src_org ON src_org.id = s.last_org_id
LEFT JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL;

DO $$
DECLARE src_n INT; tgt_n INT;
BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.users WHERE deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.users;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'users count mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'users OK: % rows', tgt_n;
END $$;

INSERT INTO trakrf.org_users
    (org_id, user_id, role, status, last_login_at, settings, metadata,
     created_at, updated_at)
SELECT
    t_org.id, t_usr.id, s.role, s.status, s.last_login_at, s.settings, s.metadata,
    s.created_at, s.updated_at
FROM cloud_src.org_users s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN cloud_src.users src_usr ON src_usr.id = s.user_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
JOIN trakrf.users t_usr ON t_usr.email = src_usr.email
WHERE s.deleted_at IS NULL
  AND src_org.deleted_at IS NULL
  AND src_usr.deleted_at IS NULL;

DO $$
DECLARE src_n INT; tgt_n INT;
BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.org_users s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.users u ON u.id = s.user_id AND u.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.org_users;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'org_users count mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'org_users OK: % rows', tgt_n;
END $$;
```

- [ ] **Step 2: Run end-to-end and verify counts**

```bash
just backend cutover-preview
```

Expected: `users OK: <N>`, `org_users OK: <N>`. ~694 users at last snapshot.

- [ ] **Step 3: Spot-check FK resolution**

```bash
psql "$PG_URL_GKE_PREVIEW" -c "
SELECT u.email, o.identifier
  FROM trakrf.users u
  LEFT JOIN trakrf.organizations o ON o.id = u.last_org_id
  WHERE u.last_org_id IS NOT NULL
  LIMIT 5;"
```

Expected: 5 rows; every email + identifier matches what source has.

- [ ] **Step 4: Commit**

```bash
git add backend/database/cutover/03_users_and_org_users.sql
git commit -m "feat(cutover): TRA-810 pull users + org_users by email/identifier"
```

---

## Task 5: Pull locations (two-phase parent_location_id)

**Files:**
- Create: `backend/database/cutover/04_locations.sql`

Locations have a self-referential FK `parent_location_id`. Two-phase load: insert with NULL parent first, then UPDATE by joining target.external_key → source.external_key → source.parent_location_id → source.external_key (of parent) → target.external_key (of parent) → target.id.

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull locations.
-- Natural key: external_key (unique per org for live rows).
-- Two-phase for parent_location_id self-FK.
\set ON_ERROR_STOP on

-- Phase 1: insert all live locations with parent_location_id = NULL.
INSERT INTO trakrf.locations
    (org_id, external_key, name, description, parent_location_id,
     valid_from, valid_to, is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.external_key, s.name, s.description, NULL,
    s.valid_from, s.valid_to, s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.locations s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL AND src_org.deleted_at IS NULL;

-- Phase 2: set parent_location_id for rows whose parent is also live.
UPDATE trakrf.locations t_child
SET parent_location_id = t_parent.id
FROM cloud_src.locations s_child
JOIN cloud_src.organizations src_org ON src_org.id = s_child.org_id
JOIN cloud_src.locations s_parent ON s_parent.id = s_child.parent_location_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
JOIN trakrf.locations t_parent
       ON t_parent.org_id = t_org.id AND t_parent.external_key = s_parent.external_key
WHERE t_child.org_id = t_org.id
  AND t_child.external_key = s_child.external_key
  AND s_child.deleted_at IS NULL
  AND s_parent.deleted_at IS NULL
  AND src_org.deleted_at IS NULL;

DO $$
DECLARE src_n INT; tgt_n INT; src_parents INT; tgt_parents INT;
BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.locations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.locations;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'locations count mismatch: src=% tgt=%', src_n, tgt_n; END IF;

    -- Parent linkage: count source live rows whose parent is also live.
    SELECT count(*) INTO src_parents FROM cloud_src.locations s
        JOIN cloud_src.locations p ON p.id = s.parent_location_id AND p.deleted_at IS NULL
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_parents FROM trakrf.locations WHERE parent_location_id IS NOT NULL;
    IF src_parents <> tgt_parents THEN
        RAISE EXCEPTION 'locations parent_location_id link mismatch: src=% tgt=%', src_parents, tgt_parents;
    END IF;
    RAISE NOTICE 'locations OK: % rows (% with live parent)', tgt_n, tgt_parents;
END $$;
```

- [ ] **Step 2: Run end-to-end**

```bash
just backend cutover-preview
```

Expected: `locations OK: ~954 rows (M with live parent)`.

- [ ] **Step 3: Spot-check a parent/child pair**

```bash
psql "$PG_URL_GKE_PREVIEW" -c "
SELECT child.external_key AS child_ek, parent.external_key AS parent_ek
  FROM trakrf.locations child
  JOIN trakrf.locations parent ON parent.id = child.parent_location_id
  LIMIT 3;"
```

Expected: 3 rows where each `(child_ek, parent_ek)` pair exists in source with the same parent relationship.

- [ ] **Step 4: Commit**

```bash
git add backend/database/cutover/04_locations.sql
git commit -m "feat(cutover): TRA-810 pull locations with two-phase parent_location_id"
```

---

## Task 6: Pull scan_devices and scan_points

**Files:**
- Create: `backend/database/cutover/05_scan_devices_and_points.sql`

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull scan_devices then scan_points.
-- scan_devices natural key: (org_id, identifier).
-- scan_points natural key: (org_id, identifier); FKs via device.identifier + location.external_key.
\set ON_ERROR_STOP on

INSERT INTO trakrf.scan_devices
    (org_id, identifier, name, type, serial_number, model, description,
     valid_from, valid_to, is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.identifier, s.name, s.type, s.serial_number, s.model, s.description,
    s.valid_from, s.valid_to, s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.scan_devices s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL AND src_org.deleted_at IS NULL;

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.scan_devices s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.scan_devices;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'scan_devices mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'scan_devices OK: % rows', tgt_n;
END $$;

INSERT INTO trakrf.scan_points
    (org_id, scan_device_id, location_id, identifier, name, antenna_port,
     description, valid_from, valid_to, is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, t_dev.id, t_loc.id, s.identifier, s.name, s.antenna_port,
    s.description, s.valid_from, s.valid_to, s.is_active, s.metadata,
    s.created_at, s.updated_at
FROM cloud_src.scan_points s
JOIN cloud_src.organizations src_org   ON src_org.id = s.org_id
JOIN cloud_src.scan_devices src_dev    ON src_dev.id = s.scan_device_id
LEFT JOIN cloud_src.locations src_loc  ON src_loc.id = s.location_id
JOIN trakrf.organizations t_org        ON t_org.identifier = src_org.identifier
JOIN trakrf.scan_devices t_dev
        ON t_dev.org_id = t_org.id AND t_dev.identifier = src_dev.identifier
LEFT JOIN trakrf.locations t_loc
        ON t_loc.org_id = t_org.id AND t_loc.external_key = src_loc.external_key
WHERE s.deleted_at IS NULL
  AND src_org.deleted_at IS NULL
  AND src_dev.deleted_at IS NULL
  AND (src_loc.id IS NULL OR src_loc.deleted_at IS NULL);

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.scan_points s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.scan_devices d  ON d.id = s.scan_device_id AND d.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (s.location_id IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.locations l
                            WHERE l.id = s.location_id AND l.deleted_at IS NULL));
    SELECT count(*) INTO tgt_n FROM trakrf.scan_points;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'scan_points mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'scan_points OK: % rows', tgt_n;
END $$;
```

- [ ] **Step 2: Run and verify**

```bash
just backend cutover-preview
```

Expected: `scan_devices OK: <N> rows`, `scan_points OK: <N> rows`.

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/05_scan_devices_and_points.sql
git commit -m "feat(cutover): TRA-810 pull scan_devices + scan_points"
```

---

## Task 7: Pull assets

**Files:**
- Create: `backend/database/cutover/06_assets.sql`

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull assets. Natural key: external_key (unique per org for live rows).
\set ON_ERROR_STOP on

INSERT INTO trakrf.assets
    (org_id, external_key, name, description, valid_from, valid_to,
     is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.external_key, s.name, s.description, s.valid_from, s.valid_to,
    s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.assets s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL AND src_org.deleted_at IS NULL;

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.assets s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.assets;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'assets mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'assets OK: % rows', tgt_n;
END $$;
```

- [ ] **Step 2: Run and verify**

```bash
just backend cutover-preview
```

Expected: `assets OK: ~151 rows`.

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/06_assets.sql
git commit -m "feat(cutover): TRA-810 pull assets by external_key"
```

---

## Task 8: Pull tags

**Files:**
- Create: `backend/database/cutover/07_tags.sql`

Tags have a mutually-exclusive FK to either `asset_id` or `location_id` (CHECK constraint `tag_target`). Both resolve via their natural keys.

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull tags. Natural key: (org_id, type, value).
-- FKs: asset_id via assets.external_key OR location_id via locations.external_key (mutually exclusive).
\set ON_ERROR_STOP on

INSERT INTO trakrf.tags
    (org_id, type, value, asset_id, location_id, valid_from, valid_to,
     is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.type, s.value, t_asset.id, t_loc.id,
    s.valid_from, s.valid_to, s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.tags s
JOIN cloud_src.organizations src_org   ON src_org.id = s.org_id
LEFT JOIN cloud_src.assets src_asset   ON src_asset.id = s.asset_id
LEFT JOIN cloud_src.locations src_loc  ON src_loc.id = s.location_id
JOIN trakrf.organizations t_org        ON t_org.identifier = src_org.identifier
LEFT JOIN trakrf.assets t_asset
        ON t_asset.org_id = t_org.id AND t_asset.external_key = src_asset.external_key
LEFT JOIN trakrf.locations t_loc
        ON t_loc.org_id = t_org.id AND t_loc.external_key = src_loc.external_key
WHERE s.deleted_at IS NULL
  AND src_org.deleted_at IS NULL
  AND (src_asset.id IS NULL OR src_asset.deleted_at IS NULL)
  AND (src_loc.id   IS NULL OR src_loc.deleted_at IS NULL)
  -- Skip tags whose only FK target was soft-deleted (would now violate tag_target CHECK).
  AND (
        (s.asset_id IS NOT NULL AND t_asset.id IS NOT NULL)
     OR (s.location_id IS NOT NULL AND t_loc.id IS NOT NULL)
  );

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    -- Source count must exclude tags whose targeted asset/location was soft-deleted.
    SELECT count(*) INTO src_n FROM cloud_src.tags s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (
                (s.asset_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.assets a
                      WHERE a.id = s.asset_id AND a.deleted_at IS NULL))
             OR (s.location_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.locations l
                      WHERE l.id = s.location_id AND l.deleted_at IS NULL))
          );
    SELECT count(*) INTO tgt_n FROM trakrf.tags;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'tags mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'tags OK: % rows', tgt_n;
END $$;
```

- [ ] **Step 2: Run and verify**

```bash
just backend cutover-preview
```

Expected: `tags OK: ~60 rows` (live ones).

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/07_tags.sql
git commit -m "feat(cutover): TRA-810 pull tags by (org,type,value), drop orphaned target rows"
```

---

## Task 9: Pull api_keys (two-phase self-FK)

**Files:**
- Create: `backend/database/cutover/08_api_keys.sql`

`api_keys` has two-phase requirements:
1. Self-FK `created_by_key_id` references `api_keys(id)`.
2. CHECK `api_keys_creator_exactly_one`: exactly one of `created_by` (user) or `created_by_key_id` must be set.

A row that originated from a key (not a user) cannot be inserted with both NULL — the CHECK forbids it. Workarounds:

- **Phase 1a (user-rooted keys, simple):** Insert all source rows that have `created_by IS NOT NULL` directly, resolving `created_by` via `users.email`. Leave `created_by_key_id = NULL`.
- **Phase 1b (key-rooted keys, deferred):** For source rows where `created_by IS NULL AND created_by_key_id IS NOT NULL`, the CHECK constraint prevents direct insertion with NULL self-FK. Strategy: temporarily DROP and re-ADD the CHECK at the end of this script, or skip the keys entirely if none exist (pre-launch reality per `prelaunch-no-prod-keys`).

The plan defaults to **SKIP** key-rooted children if none exist on source. If any are present, the script aborts with a clear error and the engineer decides whether to extend this task or defer.

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull api_keys. Natural key: jti (UUID).
-- Self-FK created_by_key_id requires two-phase, complicated by the
-- api_keys_creator_exactly_one CHECK which forbids both NULL.
-- Phase 1: insert user-rooted keys only (created_by IS NOT NULL).
-- Phase 2: assert no key-rooted children on source; if present, abort.
\set ON_ERROR_STOP on

-- Phase 1: user-rooted keys.
INSERT INTO trakrf.api_keys
    (jti, org_id, name, scopes, created_by, created_by_key_id,
     created_at, expires_at, last_used_at, revoked_at)
SELECT
    s.jti, t_org.id, s.name, s.scopes, t_usr.id, NULL,
    s.created_at, s.expires_at, s.last_used_at, s.revoked_at
FROM cloud_src.api_keys s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN cloud_src.users src_usr         ON src_usr.id = s.created_by
JOIN trakrf.organizations t_org      ON t_org.identifier = src_org.identifier
JOIN trakrf.users t_usr              ON t_usr.email = src_usr.email
WHERE src_org.deleted_at IS NULL
  AND src_usr.deleted_at IS NULL
  AND s.created_by IS NOT NULL;

-- Phase 2: assert no key-rooted children on source. If any exist we punt.
DO $$
DECLARE child_n INT;
BEGIN
    SELECT count(*) INTO child_n FROM cloud_src.api_keys
        WHERE created_by IS NULL AND created_by_key_id IS NOT NULL;
    IF child_n > 0 THEN
        RAISE EXCEPTION 'api_keys: % key-rooted children on source — extend 08_api_keys.sql to handle them (drop+re-add CHECK, or two-phase)', child_n;
    END IF;
END $$;

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.api_keys s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.users u ON u.id = s.created_by AND u.deleted_at IS NULL
        WHERE s.created_by IS NOT NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.api_keys;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'api_keys mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'api_keys OK: % rows (user-rooted only)', tgt_n;
END $$;
```

- [ ] **Step 2: Run and verify**

```bash
just backend cutover-preview
```

Expected (pre-launch): `api_keys OK: <small N> rows (user-rooted only)`, no exception about key-rooted children.

If the script aborts due to key-rooted children, that's a real signal — extend this task with the drop-CHECK/re-add-CHECK two-phase before continuing. Do not silently skip them.

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/08_api_keys.sql
git commit -m "feat(cutover): TRA-810 pull api_keys (user-rooted only, abort on key-rooted children)"
```

---

## Task 10: Pull org_invitations

**Files:**
- Create: `backend/database/cutover/09_org_invitations.sql`

`org_invitations.id` is `BIGSERIAL` (no Feistel trigger) — we just let it auto-assign. Natural key is `token`. FKs: `org_id` via `organizations.identifier`, optional `invited_by` via `users.email`. No `deleted_at` column on this table — all rows are live; filter expired/cancelled per business judgement, but for parity we copy everything.

- [ ] **Step 1: Write the pull script**

```sql
-- TRA-810 — pull org_invitations. Natural key: token. id is BIGSERIAL (auto-assigned).
\set ON_ERROR_STOP on

INSERT INTO trakrf.org_invitations
    (org_id, email, role, token, invited_by, expires_at, accepted_at,
     cancelled_at, created_at)
SELECT
    t_org.id, s.email, s.role, s.token, t_usr.id, s.expires_at, s.accepted_at,
    s.cancelled_at, s.created_at
FROM cloud_src.org_invitations s
JOIN cloud_src.organizations src_org   ON src_org.id = s.org_id
LEFT JOIN cloud_src.users src_usr      ON src_usr.id = s.invited_by
JOIN trakrf.organizations t_org        ON t_org.identifier = src_org.identifier
LEFT JOIN trakrf.users t_usr           ON t_usr.email = src_usr.email
WHERE src_org.deleted_at IS NULL
  AND (src_usr.id IS NULL OR src_usr.deleted_at IS NULL);

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.org_invitations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE (s.invited_by IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.users u
                            WHERE u.id = s.invited_by AND u.deleted_at IS NULL));
    SELECT count(*) INTO tgt_n FROM trakrf.org_invitations;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'org_invitations mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'org_invitations OK: % rows', tgt_n;
END $$;
```

- [ ] **Step 2: Run and verify**

```bash
just backend cutover-preview
```

Expected: `org_invitations OK: <N> rows`.

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/09_org_invitations.sql
git commit -m "feat(cutover): TRA-810 pull org_invitations by token"
```

---

## Task 11: Handle asset_scans hypertable (skip if empty, copy if not)

**Files:**
- Create: `backend/database/cutover/10_asset_scans.sql`

`asset_scans` is a TimescaleDB hypertable with composite PK `(timestamp, org_id, asset_id)`. Per memory and Linear comment, pre-launch this table is expected to be empty in source (no readers running in prod). The script verifies emptiness; if non-empty, it remaps `(org_id, asset_id)` via natural keys and copies.

- [ ] **Step 1: Write the script**

```sql
-- TRA-810 — asset_scans hypertable. Expected empty pre-launch. If non-empty,
-- copy with org_id/asset_id remapped via natural keys.
\set ON_ERROR_STOP on

DO $$
DECLARE n BIGINT;
BEGIN
    SELECT count(*) INTO n FROM cloud_src.asset_scans;
    IF n = 0 THEN
        RAISE NOTICE 'asset_scans OK: 0 rows on source — skipping';
        RETURN;
    END IF;
    RAISE NOTICE 'asset_scans: % rows on source — performing remapped copy', n;
END $$;

-- This INSERT is a no-op when source is empty. When non-empty it remaps.
-- tag_scan_id intentionally NOT carried: tag_scans is skipped from pull,
-- so the source IDs would not point to anything; leave NULL.
INSERT INTO trakrf.asset_scans
    (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id, created_at)
SELECT
    s.timestamp, t_org.id, t_asset.id, t_loc.id, t_sp.id, NULL, s.created_at
FROM cloud_src.asset_scans s
JOIN cloud_src.organizations src_org       ON src_org.id = s.org_id
JOIN cloud_src.assets src_asset            ON src_asset.id = s.asset_id
LEFT JOIN cloud_src.locations src_loc      ON src_loc.id = s.location_id
LEFT JOIN cloud_src.scan_points src_sp     ON src_sp.id = s.scan_point_id
JOIN trakrf.organizations t_org            ON t_org.identifier = src_org.identifier
JOIN trakrf.assets t_asset
        ON t_asset.org_id = t_org.id AND t_asset.external_key = src_asset.external_key
LEFT JOIN trakrf.locations t_loc
        ON t_loc.org_id = t_org.id AND t_loc.external_key = src_loc.external_key
LEFT JOIN trakrf.scan_points t_sp
        ON t_sp.org_id = t_org.id AND t_sp.identifier = src_sp.identifier
WHERE src_org.deleted_at   IS NULL
  AND src_asset.deleted_at IS NULL
  AND (src_loc.id IS NULL OR src_loc.deleted_at IS NULL)
  AND (src_sp.id  IS NULL OR src_sp.deleted_at IS NULL);

DO $$ DECLARE src_n BIGINT; tgt_n BIGINT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.asset_scans;
    SELECT count(*) INTO tgt_n FROM trakrf.asset_scans;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'asset_scans mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'asset_scans OK: % rows', tgt_n;
END $$;
```

- [ ] **Step 2: Run and verify**

```bash
just backend cutover-preview
```

Expected: `asset_scans OK: 0 rows on source — skipping` (pre-launch reality).

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/10_asset_scans.sql
git commit -m "feat(cutover): TRA-810 conditional asset_scans copy with natural-key remap"
```

---

## Task 12: Verification script — counts, orphan FKs, parity sampling

**Files:**
- Create: `backend/database/cutover/20_verify.sql`

End-of-pull integrity sweep. Independent of the per-file `RAISE NOTICE` checks: this script runs after all loads and is the authoritative "did we land what we expected" gate.

- [ ] **Step 1: Write the verification script**

```sql
-- TRA-810 — end-to-end verification after pull.
-- Cross-checks counts, asserts no orphan FKs, samples natural-key parity.
\set ON_ERROR_STOP on

-- ---------- A. Row count parity (live source vs target) ----------
DO $$
DECLARE
    src_org INT; src_usr INT; src_ou INT; src_loc INT; src_dev INT; src_sp INT;
    src_ast INT; src_tag INT; src_inv INT;
    tgt_org INT; tgt_usr INT; tgt_ou INT; tgt_loc INT; tgt_dev INT; tgt_sp INT;
    tgt_ast INT; tgt_tag INT; tgt_inv INT;
BEGIN
    SELECT count(*) INTO src_org FROM cloud_src.organizations WHERE deleted_at IS NULL;
    SELECT count(*) INTO src_usr FROM cloud_src.users         WHERE deleted_at IS NULL;
    SELECT count(*) INTO src_ou  FROM cloud_src.org_users s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.users u ON u.id = s.user_id AND u.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_loc FROM cloud_src.locations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_dev FROM cloud_src.scan_devices s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_sp  FROM cloud_src.scan_points s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.scan_devices d  ON d.id = s.scan_device_id AND d.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (s.location_id IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.locations l
                            WHERE l.id = s.location_id AND l.deleted_at IS NULL));
    SELECT count(*) INTO src_ast FROM cloud_src.assets s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_tag FROM cloud_src.tags s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (
                (s.asset_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.assets a
                      WHERE a.id = s.asset_id AND a.deleted_at IS NULL))
             OR (s.location_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.locations l
                      WHERE l.id = s.location_id AND l.deleted_at IS NULL))
          );
    SELECT count(*) INTO src_inv FROM cloud_src.org_invitations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE (s.invited_by IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.users u
                            WHERE u.id = s.invited_by AND u.deleted_at IS NULL));

    SELECT count(*) INTO tgt_org FROM trakrf.organizations;
    SELECT count(*) INTO tgt_usr FROM trakrf.users;
    SELECT count(*) INTO tgt_ou  FROM trakrf.org_users;
    SELECT count(*) INTO tgt_loc FROM trakrf.locations;
    SELECT count(*) INTO tgt_dev FROM trakrf.scan_devices;
    SELECT count(*) INTO tgt_sp  FROM trakrf.scan_points;
    SELECT count(*) INTO tgt_ast FROM trakrf.assets;
    SELECT count(*) INTO tgt_tag FROM trakrf.tags;
    SELECT count(*) INTO tgt_inv FROM trakrf.org_invitations;

    IF (src_org, src_usr, src_ou, src_loc, src_dev, src_sp, src_ast, src_tag, src_inv)
       <> (tgt_org, tgt_usr, tgt_ou, tgt_loc, tgt_dev, tgt_sp, tgt_ast, tgt_tag, tgt_inv) THEN
        RAISE EXCEPTION 'verify A — count parity failed: src=(%,%,%,%,%,%,%,%,%) tgt=(%,%,%,%,%,%,%,%,%)',
            src_org, src_usr, src_ou, src_loc, src_dev, src_sp, src_ast, src_tag, src_inv,
            tgt_org, tgt_usr, tgt_ou, tgt_loc, tgt_dev, tgt_sp, tgt_ast, tgt_tag, tgt_inv;
    END IF;
    RAISE NOTICE 'verify A OK: row-count parity across 9 entity tables';
END $$;

-- ---------- B. FK integrity — every FK column resolves to a target row ----------
DO $$
DECLARE n INT;
BEGIN
    -- users.last_org_id (nullable)
    SELECT count(*) INTO n FROM trakrf.users u
        WHERE u.last_org_id IS NOT NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.organizations o WHERE o.id = u.last_org_id);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — users.last_org_id orphans: %', n; END IF;

    -- locations.parent_location_id (nullable)
    SELECT count(*) INTO n FROM trakrf.locations l
        WHERE l.parent_location_id IS NOT NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.locations p WHERE p.id = l.parent_location_id);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — locations.parent_location_id orphans: %', n; END IF;

    -- scan_points.location_id (nullable)
    SELECT count(*) INTO n FROM trakrf.scan_points sp
        WHERE sp.location_id IS NOT NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.locations l WHERE l.id = sp.location_id);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — scan_points.location_id orphans: %', n; END IF;

    -- tags: exactly one of asset_id/location_id NOT NULL (CHECK tag_target).
    SELECT count(*) INTO n FROM trakrf.tags t
        WHERE (t.asset_id IS NULL) = (t.location_id IS NULL);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — tags violating tag_target CHECK: %', n; END IF;

    RAISE NOTICE 'verify B OK: no FK orphans, tag_target CHECK respected';
END $$;

-- ---------- C. Natural-key parity — every source live row's natural key
-- has a matching target row. Sample-based for speed but exhaustive per entity. ----------
DO $$
DECLARE missing INT;
BEGIN
    SELECT count(*) INTO missing FROM cloud_src.organizations s
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.organizations t WHERE t.identifier = s.identifier);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live organizations missing on target', missing; END IF;

    SELECT count(*) INTO missing FROM cloud_src.users s
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.users t WHERE t.email = s.email);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live users missing on target', missing; END IF;

    SELECT count(*) INTO missing FROM cloud_src.locations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN trakrf.organizations t_org ON t_org.identifier = o.identifier
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.locations t
                            WHERE t.org_id = t_org.id AND t.external_key = s.external_key);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live locations missing on target', missing; END IF;

    SELECT count(*) INTO missing FROM cloud_src.assets s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN trakrf.organizations t_org ON t_org.identifier = o.identifier
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.assets t
                            WHERE t.org_id = t_org.id AND t.external_key = s.external_key);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live assets missing on target', missing; END IF;

    RAISE NOTICE 'verify C OK: natural-key parity for org/user/location/asset';
END $$;

-- ---------- D. Surrogate ID sanity — every target id is in [1, 2^52) ----------
DO $$
DECLARE bad INT;
BEGIN
    SELECT count(*) INTO bad FROM (
        SELECT id FROM trakrf.organizations
        UNION ALL SELECT id FROM trakrf.users
        UNION ALL SELECT id FROM trakrf.locations
        UNION ALL SELECT id FROM trakrf.scan_devices
        UNION ALL SELECT id FROM trakrf.scan_points
        UNION ALL SELECT id FROM trakrf.assets
        UNION ALL SELECT id FROM trakrf.tags
    ) t WHERE id < 0 OR id >= (1::BIGINT << 52);
    IF bad > 0 THEN RAISE EXCEPTION 'verify D — % rows with id outside Feistel [0, 2^52)', bad; END IF;
    RAISE NOTICE 'verify D OK: all surrogate ids in Feistel range';
END $$;

RAISE NOTICE 'TRA-810 verification: all gates passed';
```

- [ ] **Step 2: Run and verify**

```bash
just backend cutover-preview
```

Expected: each of `verify A OK`, `verify B OK`, `verify C OK`, `verify D OK`, and the final `TRA-810 verification: all gates passed`.

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/20_verify.sql
git commit -m "feat(cutover): TRA-810 end-to-end verification — counts, FK orphans, NK parity"
```

---

## Task 13: Teardown script

**Files:**
- Create: `backend/database/cutover/99_teardown.sql`

Removes the FDW objects after the pull is verified. Does NOT drop data. Idempotent.

- [ ] **Step 1: Write the teardown script**

```sql
-- TRA-810 — drop FDW objects after pull. Does NOT touch trakrf.* data.
\set ON_ERROR_STOP on

DROP SCHEMA IF EXISTS cloud_src CASCADE;
DROP USER MAPPING IF EXISTS FOR CURRENT_USER SERVER cloud_src_srv;
DROP SERVER IF EXISTS cloud_src_srv CASCADE;
-- Leave postgres_fdw extension installed (might be needed by other tooling).

DO $$ BEGIN
    RAISE NOTICE 'TRA-810 teardown OK: FDW objects removed';
END $$;
```

- [ ] **Step 2: Run end-to-end and verify clean exit**

```bash
just backend cutover-preview
```

Expected: full run prints all per-table `OK` notices, all 4 verify gates, then `TRA-810 teardown OK: FDW objects removed`. Exit code 0.

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/99_teardown.sql
git commit -m "feat(cutover): TRA-810 teardown — drop FDW server/mapping/schema"
```

---

## Task 14: End-to-end dry-run on GKE preview ← Cloud preview

This task is execution + evidence, not new code. It validates that the full script runs clean against a fresh CNPG preview pulling from Cloud preview.

- [ ] **Step 1: Confirm target CNPG preview is on the TRA-720 schema and empty**

```bash
psql "$PG_URL_GKE_PREVIEW" -c "SELECT version, dirty FROM schema_migrations;"
psql "$PG_URL_GKE_PREVIEW" -c "SELECT count(*) AS n_orgs FROM trakrf.organizations;"
```

Expected: `schema_migrations.version` = 10, `dirty` = false, `n_orgs` = 0.

If target is not empty, either provision a fresh preview database (user has greenlit rebuilding GKE preview — recreate it via the standard infra flow) or run `just backend db-pull-nk "$PG_URL_PREVIEW" "$PG_URL_GKE_PREVIEW" --truncate` to clear and re-pull in one shot. Do not proceed against a non-empty target without an explicit cleanup choice.

- [ ] **Step 2: Confirm source connectivity and live-row baseline**

```bash
psql "$PG_URL_PREVIEW" -c "
SELECT 'organizations' AS tbl, count(*) FROM trakrf.organizations WHERE deleted_at IS NULL
UNION ALL SELECT 'users', count(*) FROM trakrf.users WHERE deleted_at IS NULL
UNION ALL SELECT 'locations', count(*) FROM trakrf.locations WHERE deleted_at IS NULL
UNION ALL SELECT 'assets', count(*) FROM trakrf.assets WHERE deleted_at IS NULL
UNION ALL SELECT 'tags', count(*) FROM trakrf.tags WHERE deleted_at IS NULL;"
```

Expected: numbers in the order of the 2026-05-26 snapshot (873 / 694 / 954 / 151 / 60), give or take recent churn.

- [ ] **Step 3: Execute the full pull**

```bash
just backend cutover-preview 2>&1 | tee /tmp/tra-810-dryrun.log
```

Expected: every `NOTICE` from each step is present in the log, no `ERROR`, exit code 0.

- [ ] **Step 4: Sanity-spot-check via the API path**

The pull is meaningless if the new schema isn't queryable by the app. Hit the GKE preview backend at the public endpoint with a known user's credentials:

```bash
# Substitute a real preview-known email/password from your secrets store.
curl -s -X POST https://gke.trakrf.app/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"email":"<known>","password":"<known>"}' | jq .
```

Expected: HTTP 200 with a session cookie set. Then `curl` `/api/orgs` and `/api/assets` (with the cookie) to confirm the API returns sensible data.

If login fails because password_hash is null (we filtered or skipped), or because last_org_id points to a stale id, capture the symptom and triage — don't paper over.

- [ ] **Step 5: Attach dry-run log to the Linear ticket and ship the PR**

Use the project's standard PR flow per `feedback_always_pr_never_merge_local`:

```bash
git push -u origin feat/tra-810-fdw-pull-cutover

gh pr create --title "feat(cutover): TRA-810 FDW pull script — dev + dry-run only" --body "$(cat <<'EOF'
## Summary
- Adds `backend/database/cutover/` with 10 SQL files + a `just backend cutover-preview` orchestrator.
- Natural-key FDW pull from Cloud Postgres → CNPG, regenerating surrogate IDs via TRA-720 Feistel triggers.
- `WHERE deleted_at IS NULL` filter on every entity table; skips `bulk_import_jobs`, `tag_scans`, `password_reset_tokens`.
- Verification gate (`20_verify.sql`) covers count parity, FK orphans, natural-key parity, and Feistel-range surrogate-id sanity.
- Dry-run validated against GKE preview ← Cloud preview, log attached.

## Scope
- In: dev + dry-run script merged and validated.
- Out: production cutover execution (separate session — maintenance window required); `trakrf_old` table drops (separate scope).

## Test plan
- [x] Fresh CNPG preview migrated to TRA-720 v10 schema, `trakrf.organizations` empty.
- [x] `just backend cutover-preview` runs end-to-end, exits 0, all verify gates green.
- [x] Smoke: API login against GKE preview returns 200, `/api/orgs` and `/api/assets` return sensible data.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 6: Update Linear ticket**

Add a comment to TRA-810 linking the PR and the dry-run log. Leave the ticket `In Progress` — it doesn't go Done until the production cutover (out of scope here) lands.

---

## Self-review notes (after writing — fix anything broken)

- Spec coverage: every entity in the TRA-810 Linear comment is covered (orgs, users, org_users, locations, scan_devices/points, assets, tags, api_keys, org_invitations, asset_scans). Skips are explicit (bulk_import_jobs, tag_scans, password_reset_tokens) with rationale in the README. ✓
- Two-phase entities are explicit: locations (Task 5) and api_keys (Task 9 — with abort-if-key-rooted-children safety). ✓
- The CHECK-constraint edge case on api_keys is surfaced rather than silently mishandled. ✓
- RLS is bypassed implicitly because the migration role is the table owner and policies are USING-only (not FORCE). README notes this. ✓
- Tags-orphaned-by-soft-delete edge case is handled both in the INSERT (skip) and the verify A count (subtract). ✓
- All file paths and column lists match the TRA-720 migration files inspected at plan-writing time. ✓
- No "TBD" / "implement later" / "add error handling" placeholders. ✓
