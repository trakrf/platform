# TRA-524 Rename `Identifier` Entity → `Tag` (Cutover)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the physical-tag entity from `identifier` → `tag` consistently across DB, Go internals, URL paths, MQTT, frontend, and docs — without renaming the natural-key column convention also called `identifier` (which lives on five entity tables and stays as-is).

**Architecture:** Atomic single-PR cutover. One migration file (000033) handles every DB rename + PL/pgSQL function body rewrite. Go side renames storage methods, DTOs, struct fields, JSON keys, and `@Router`/`@Description` annotations. OpenAPI YAML regenerates from those annotations (it's auto-generated, not hand-edited). Frontend mirrors all backend renames in lockstep — API client, types, field accessors, e2e fixtures. Ingester `connect.yaml` updates to write to `tag_scans`. Docs prose lands in the same PR.

**Tech Stack:** Go 1.22+ / chi router / pgx, PostgreSQL 17 + TimescaleDB 2.26, swag (Swaggo) for OpenAPI generation, Vite + React + TypeScript, Playwright e2e, Redpanda Connect (`sql_raw` output) for MQTT ingestion.

**Reference:** `docs/superpowers/plans/2026-04-26-tra-523-identifier-overload-analysis.md` has the full impact analysis and rename rationale.

---

## Out-of-scope (Nick's sibling ticket)

These are **NOT** in this plan. Do not touch them in this PR:

- UI label English text: "Identifier" / "Tag identifier(s)" labels in modals, forms, tables — stays as-is until Nick's UI cleanup PR
- Component filenames: `TagIdentifiersModal.tsx` etc. — keep current filenames
- Test filenames mirroring those components

The split rule: **JSON keys, TypeScript field names, and URL paths change here. Visible English text and component filenames change in Nick's PR.**

---

## Phase 0: Pre-flight (already complete)

- ✅ Worktree at `.worktrees/miks2u+tra-524-rename-identifier-tag` on `miks2u/tra-524-rename-identifier-tag`
- ✅ Rebased onto `origin/main` (currently `9db7314`)
- ✅ Hypertable rename smoke test on preview (PG 17.9 + Timescale 2.26.3) — `ALTER TABLE` atomic, retention policy auto-tracks, trigger stays attached
- ✅ TRA-524 AC §1–§7 fleshed out and signed off

No tasks here. Proceed to Phase 1.

---

## File Structure Overview

### New files
- `backend/migrations/000033_rename_identifier_to_tag.up.sql`
- `backend/migrations/000033_rename_identifier_to_tag.down.sql`

### Renamed files
- `backend/internal/storage/identifiers.go` → `backend/internal/storage/tags.go`
- `backend/internal/storage/identifiers_test.go` → `backend/internal/storage/tags_test.go`
- `backend/internal/storage/identifiers_crossorg_test.go` → `backend/internal/storage/tags_crossorg_test.go`
- `backend/internal/models/shared/identifier.go` → `backend/internal/models/shared/tag.go` (file rename only — type names `TagIdentifier`/`TagIdentifierRequest` stay per AC §2)
- `frontend/src/types/shared/identifier.ts` → `frontend/src/types/shared/tag.ts`

### Modified files (illustrative — full grep run during execution)

**Backend Go:**
- `backend/internal/storage/assets.go` — SQL string for `create_asset_with_tags`
- `backend/internal/storage/locations.go` — SQL string for `create_location_with_tags`
- `backend/internal/models/asset/asset.go` — field + DTO renames
- `backend/internal/models/asset/public.go` — `Identifiers` field on `PublicAssetView`
- `backend/internal/models/location/location.go` — same pattern
- `backend/internal/models/location/public.go` — same pattern
- `backend/internal/handlers/assets/assets.go` — handler methods + annotations
- `backend/internal/handlers/locations/locations.go` — same
- `backend/internal/cmd/serve/router.go` — 8 route path segments

**Auto-generated (do NOT hand-edit; run `just backend api-spec`):**
- `backend/docs/docs.go`
- `backend/docs/swagger.json`
- `docs/api/openapi.public.yaml`
- `docs/api/openapi.public.json`
- `backend/internal/handlers/swaggerspec/openapi.public.{json,yaml}`
- `backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}`

**Frontend:**
- `frontend/src/lib/api/assets/index.ts`
- `frontend/src/lib/api/locations/index.ts`
- `frontend/src/types/shared/index.ts` (re-export rename)
- `frontend/src/types/assets/index.ts` — `TagIdentifierInput`
- `frontend/src/types/locations/index.ts` — `TagIdentifierInput`
- `frontend/src/components/assets/AssetForm.tsx`
- `frontend/src/components/assets/AssetCard.tsx`
- `frontend/src/components/assets/AssetDetailsModal.tsx`
- `frontend/src/components/locations/LocationForm.tsx`
- `frontend/src/components/locations/LocationCard.tsx`
- `frontend/src/components/locations/LocationDetailsModal.tsx`
- `frontend/src/components/locations/LocationDetailsPanel.tsx`
- `frontend/src/stores/locations/locationActions.ts`
- `frontend/src/lib/asset/filters.ts`
- `frontend/src/utils/export/assetExport.ts`
- `frontend/src/lib/api/assets/assets.test.ts` (and any other unit tests touching field names)

**E2E:**
- `frontend/tests/e2e/inventory-save.spec.ts` — payload `identifiers: [...]` → `tags: [...]`
- Any fixture/helper that constructs the asset/location create payloads

**Ingester:**
- `ingester/connect.yaml`

**Docs prose:**
- `docs/schema-naming-conventions.md`
- `docs/logical-schema.md`

---

## Verification Discipline

This is a refactor, not a feature — there's no new behavior to test. The discipline is:

1. **Existing tests are the regression net.** Run them after every phase. They MUST stay green.
2. **TypeScript compilation is the second net.** `just frontend typecheck` catches missed field accessors.
3. **Migration round-trip.** Up, then down, then up — verifies the migration is reversible and idempotent.
4. **OpenAPI alignment.** After regen, `git diff docs/api/openapi.public.yaml` should reflect ONLY the rename (no unrelated drift).
5. **Frontend e2e against the running backend** — last-mile verification that URL routing actually works after the rename.

If a step's test fails for a reason unrelated to the step's change, **stop and investigate** before continuing. Don't paper over surprise failures.

---

## Phase 1: Database migration (up + down)

**Goal:** Land one migration that renames every DB object atomically and rewrites the three PL/pgSQL function bodies.

### Task 1.1: Write the up migration

**Files:**
- Create: `backend/migrations/000033_rename_identifier_to_tag.up.sql`

- [ ] **Step 1: Create the up migration file**

```sql
SET search_path = trakrf,public;

-- ============================================================================
-- TRA-524: Rename Identifier entity → Tag
-- See docs/superpowers/plans/2026-04-26-tra-524-rename-identifier-tag.md
-- Concept-#1 column convention `identifier` on entity tables stays unchanged.
-- ============================================================================

-- 1. Rename the regular table + sequence
ALTER TABLE identifiers RENAME TO tags;
ALTER SEQUENCE identifier_seq RENAME TO tag_seq;

-- 2. Rename indexes on the renamed table (Postgres does NOT auto-rename them)
ALTER INDEX idx_identifiers_org             RENAME TO idx_tags_org;
ALTER INDEX idx_identifiers_asset           RENAME TO idx_tags_asset;
ALTER INDEX idx_identifiers_location        RENAME TO idx_tags_location;
ALTER INDEX idx_identifiers_value           RENAME TO idx_tags_value;
ALTER INDEX idx_identifiers_valid           RENAME TO idx_tags_valid;
ALTER INDEX idx_identifiers_type            RENAME TO idx_tags_type;
ALTER INDEX idx_identifiers_active          RENAME TO idx_tags_active;
ALTER INDEX identifiers_org_id_type_value_unique
                                            RENAME TO tags_org_id_type_value_unique;

-- 3. Rename CHECK constraint (named `identifier_target` in 000009)
ALTER TABLE tags RENAME CONSTRAINT identifier_target TO tag_target;

-- 4. Rename triggers attached to the renamed table
ALTER TRIGGER generate_identifier_id_trigger ON tags RENAME TO generate_tag_id_trigger;
ALTER TRIGGER update_identifiers_updated_at  ON tags RENAME TO update_tags_updated_at;

-- 5. Rename the RLS policy
ALTER POLICY org_isolation_identifiers ON tags RENAME TO org_isolation_tags;

-- 6. Rename the hypertable (Timescale catalog tracks rename automatically;
--    retention policy job_id 1004 follows; verified on preview 2026-04-26)
ALTER TABLE identifier_scans RENAME TO tag_scans;

-- 7. Rename hypertable's child indexes (NOT auto-renamed)
ALTER INDEX idx_identifier_scans_topic       RENAME TO idx_tag_scans_topic;
ALTER INDEX identifier_scans_created_at_idx  RENAME TO tag_scans_created_at_idx;
ALTER INDEX identifier_scans_pkey            RENAME TO tag_scans_pkey;

-- 8. Rename the FK column on asset_scans
ALTER TABLE asset_scans RENAME COLUMN identifier_scan_id TO tag_scan_id;

-- 9. Rewrite PL/pgSQL functions: rename function name + param `p_tags` +
--    OUT col `tag_ids` + body inserts INTO `tags`. Go callers invoke with
--    `SELECT *` positionally (see backend/internal/storage/assets.go:461 +
--    locations.go:584), so renaming param + OUT column names is safe.

CREATE OR REPLACE FUNCTION create_asset_with_tags(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_type VARCHAR(50),
    p_description TEXT,
    p_current_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (asset_id INT, tag_ids INT[]) AS $$
DECLARE
    v_asset_id INT;
    v_tag_ids INT[] := '{}';
    v_tag JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.assets (
        org_id, identifier, name, type, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_type, p_description,
        p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags)
        LOOP
            INSERT INTO trakrf.tags (org_id, type, value, asset_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;

-- Drop old function (signature differs by name only; CREATE OR REPLACE on the
-- new name + DROP on the old name keeps things tidy)
DROP FUNCTION IF EXISTS create_asset_with_identifiers(
    INT, VARCHAR, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

CREATE OR REPLACE FUNCTION create_location_with_tags(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (location_id INT, tag_ids INT[]) AS $$
DECLARE
    v_location_id INT;
    v_tag_ids INT[] := '{}';
    v_tag JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.locations (
        org_id, identifier, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags)
        LOOP
            INSERT INTO trakrf.tags (org_id, type, value, location_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;

DROP FUNCTION IF EXISTS create_location_with_identifiers(
    INT, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

-- 10. Rewrite the MQTT ingestion processor function. Body inserts INTO `tags`
--     instead of `identifiers`. Drop the trigger first since it references the
--     old function by name; recreate at the end pointing at the new function.

DROP TRIGGER IF EXISTS trigger_process_identifier_scans ON tag_scans;
DROP FUNCTION IF EXISTS process_identifier_scans();

CREATE OR REPLACE FUNCTION process_tag_scans() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    topic_org_id INT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %. Topic should match organization identifier', NEW.message_topic;
        RETURN NEW;
    END IF;

    INSERT INTO locations (org_id, identifier, name)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM locations l
        WHERE l.org_id = topic_org_id
          AND l.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO scan_devices (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        NEW.message_data ->> 'rfidReaderName',
        NEW.message_data ->> 'rfidReaderName' || ' (auto-created from scan)',
        'rfid_reader'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_devices d
        WHERE d.org_id = topic_org_id
          AND d.identifier = NEW.message_data ->> 'rfidReaderName'
    );

    INSERT INTO scan_points (org_id, scan_device_id, location_id, identifier, name, antenna_port)
    SELECT DISTINCT
        topic_org_id,
        (SELECT id FROM scan_devices
         WHERE org_id = topic_org_id
           AND identifier = NEW.message_data ->> 'rfidReaderName'),
        l.id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)',
        (t.tag ->> 'antennaPort')::INT
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN locations l ON l.org_id = topic_org_id
                     AND l.identifier = t.tag ->> 'capturePointName'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_points sp
        WHERE sp.org_id = topic_org_id
          AND sp.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO assets (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'epc',
        t.tag ->> 'epc' || ' (auto-created from scan)',
        'unknown'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM assets a
        WHERE a.org_id = topic_org_id
          AND a.identifier = t.tag ->> 'epc'
    )
    AND NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO tags (org_id, asset_id, type, value)
    SELECT DISTINCT
        topic_org_id,
        a.id,
        'rfid',
        t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id
                  AND a.identifier = t.tag ->> 'epc'
    WHERE NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0) AS timestamp,
        topic_org_id,
        a.id AS asset_id,
        sp.location_id,
        sp.id AS scan_point_id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN scan_points sp ON sp.org_id = topic_org_id
                        AND sp.identifier = t.tag ->> 'capturePointName'
    JOIN assets a ON a.org_id = topic_org_id
                  AND a.identifier = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing tag_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;

-- 11. Recreate the trigger pointing at the renamed function
CREATE TRIGGER trigger_process_tag_scans
    AFTER INSERT ON tag_scans
    FOR EACH ROW
    EXECUTE FUNCTION process_tag_scans();

-- 12. Update comments
COMMENT ON FUNCTION process_tag_scans() IS 'Auto-create entities from MQTT messages and populate asset_scans';
COMMENT ON TABLE tags IS 'Stores physical/logical tags (RFID, BLE, NFC, barcode, serial, etc.) with temporal validity';
COMMENT ON COLUMN tags.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN tags.type IS 'Tag type: rfid, ble, nfc, barcode, serial, mac, qr, etc.';
COMMENT ON COLUMN tags.value IS 'The actual tag value (EPC, MAC address, NFC UID, barcode digits, etc.)';
COMMENT ON COLUMN tags.asset_id IS 'Optional FK to asset - identifies one asset (mutually exclusive with location_id)';
COMMENT ON COLUMN tags.location_id IS 'Optional FK to location - identifies one location (mutually exclusive with asset_id)';
COMMENT ON COLUMN tags.valid_from IS 'Start of validity period for this tag version';
COMMENT ON COLUMN tags.valid_to IS 'End of validity period for this tag version';

COMMENT ON TABLE tag_scans IS 'Raw MQTT message capture from RFID readers - pure data lake for tag scans';
COMMENT ON COLUMN tag_scans.created_at IS 'Timestamp when message was received';
COMMENT ON COLUMN tag_scans.message_topic IS 'MQTT topic (e.g., trakrf.id/cs463-214/scan)';
COMMENT ON COLUMN tag_scans.message_data IS 'Raw MQTT message payload as JSON';

COMMENT ON COLUMN asset_scans.tag_scan_id IS 'Link to the source raw tag scan for audit trail';
```

- [ ] **Step 2: Sanity-check the SQL parses (no DB run yet)**

Run: `psql --no-psqlrc --dry-run < backend/migrations/000033_rename_identifier_to_tag.up.sql 2>&1 | head -5` — psql doesn't have a true dry-run, so instead use a syntax-only parser:

Run: `pgsanity backend/migrations/000033_rename_identifier_to_tag.up.sql 2>/dev/null || python3 -c "import re; sql = open('backend/migrations/000033_rename_identifier_to_tag.up.sql').read(); print('balanced \$\$:', sql.count('\$\$') % 2 == 0); print('total length:', len(sql))"`

Expected: dollar-quote count is even (BEGIN/END `$$ LANGUAGE plpgsql` blocks balanced).

### Task 1.2: Write the down migration

**Files:**
- Create: `backend/migrations/000033_rename_identifier_to_tag.down.sql`

- [ ] **Step 1: Create the down migration file**

```sql
SET search_path = trakrf,public;

-- ============================================================================
-- TRA-524: Reverse the Identifier → Tag rename. Restores the prior schema.
-- Functions are recreated with their original bodies (referencing `identifiers`).
-- ============================================================================

-- 1. Drop the new trigger + function before recreating the old
DROP TRIGGER IF EXISTS trigger_process_tag_scans ON tag_scans;
DROP FUNCTION IF EXISTS process_tag_scans();
DROP FUNCTION IF EXISTS create_asset_with_tags(
    INT, VARCHAR, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);
DROP FUNCTION IF EXISTS create_location_with_tags(
    INT, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

-- 2. Reverse the FK column rename on asset_scans
ALTER TABLE asset_scans RENAME COLUMN tag_scan_id TO identifier_scan_id;

-- 3. Reverse the hypertable + child index renames
ALTER INDEX idx_tag_scans_topic       RENAME TO idx_identifier_scans_topic;
ALTER INDEX tag_scans_created_at_idx  RENAME TO identifier_scans_created_at_idx;
ALTER INDEX tag_scans_pkey            RENAME TO identifier_scans_pkey;
ALTER TABLE tag_scans RENAME TO identifier_scans;

-- 4. Reverse the regular table renames
ALTER POLICY org_isolation_tags ON tags RENAME TO org_isolation_identifiers;
ALTER TRIGGER update_tags_updated_at  ON tags RENAME TO update_identifiers_updated_at;
ALTER TRIGGER generate_tag_id_trigger ON tags RENAME TO generate_identifier_id_trigger;
ALTER TABLE tags RENAME CONSTRAINT tag_target TO identifier_target;
ALTER INDEX tags_org_id_type_value_unique RENAME TO identifiers_org_id_type_value_unique;
ALTER INDEX idx_tags_active   RENAME TO idx_identifiers_active;
ALTER INDEX idx_tags_type     RENAME TO idx_identifiers_type;
ALTER INDEX idx_tags_valid    RENAME TO idx_identifiers_valid;
ALTER INDEX idx_tags_value    RENAME TO idx_identifiers_value;
ALTER INDEX idx_tags_location RENAME TO idx_identifiers_location;
ALTER INDEX idx_tags_asset    RENAME TO idx_identifiers_asset;
ALTER INDEX idx_tags_org      RENAME TO idx_identifiers_org;
ALTER SEQUENCE tag_seq RENAME TO identifier_seq;
ALTER TABLE tags RENAME TO identifiers;

-- 5. Recreate original functions with bodies pointing at `identifiers`
CREATE OR REPLACE FUNCTION create_asset_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_type VARCHAR(50),
    p_description TEXT,
    p_current_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB
) RETURNS TABLE (asset_id INT, identifier_ids INT[]) AS $$
DECLARE
    v_asset_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.assets (
        org_id, identifier, name, type, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_type, p_description,
        p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_identifier->>'type', 'rfid'),
                v_identifier->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION create_location_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB
) RETURNS TABLE (location_id INT, identifier_ids INT[]) AS $$
DECLARE
    v_location_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.locations (
        org_id, identifier, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (org_id, type, value, location_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_identifier->>'type', 'rfid'),
                v_identifier->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

-- 6. Recreate original process_identifier_scans function with body pointing at
--    `identifiers` and trigger pointing at it
CREATE OR REPLACE FUNCTION process_identifier_scans() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    topic_org_id INT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %. Topic should match organization identifier', NEW.message_topic;
        RETURN NEW;
    END IF;

    INSERT INTO locations (org_id, identifier, name)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM locations l
        WHERE l.org_id = topic_org_id
          AND l.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO scan_devices (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        NEW.message_data ->> 'rfidReaderName',
        NEW.message_data ->> 'rfidReaderName' || ' (auto-created from scan)',
        'rfid_reader'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_devices d
        WHERE d.org_id = topic_org_id
          AND d.identifier = NEW.message_data ->> 'rfidReaderName'
    );

    INSERT INTO scan_points (org_id, scan_device_id, location_id, identifier, name, antenna_port)
    SELECT DISTINCT
        topic_org_id,
        (SELECT id FROM scan_devices
         WHERE org_id = topic_org_id
           AND identifier = NEW.message_data ->> 'rfidReaderName'),
        l.id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)',
        (t.tag ->> 'antennaPort')::INT
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN locations l ON l.org_id = topic_org_id
                     AND l.identifier = t.tag ->> 'capturePointName'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_points sp
        WHERE sp.org_id = topic_org_id
          AND sp.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO assets (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'epc',
        t.tag ->> 'epc' || ' (auto-created from scan)',
        'unknown'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM assets a
        WHERE a.org_id = topic_org_id
          AND a.identifier = t.tag ->> 'epc'
    )
    AND NOT EXISTS (
        SELECT 1 FROM identifiers i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO identifiers (org_id, asset_id, type, value)
    SELECT DISTINCT
        topic_org_id,
        a.id,
        'rfid',
        t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id
                  AND a.identifier = t.tag ->> 'epc'
    WHERE NOT EXISTS (
        SELECT 1 FROM identifiers i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0) AS timestamp,
        topic_org_id,
        a.id AS asset_id,
        sp.location_id,
        sp.id AS scan_point_id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN scan_points sp ON sp.org_id = topic_org_id
                        AND sp.identifier = t.tag ->> 'capturePointName'
    JOIN assets a ON a.org_id = topic_org_id
                  AND a.identifier = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing identifier_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_process_identifier_scans
    AFTER INSERT ON identifier_scans
    FOR EACH ROW
    EXECUTE FUNCTION process_identifier_scans();

-- 7. Restore comments
COMMENT ON FUNCTION process_identifier_scans() IS 'Auto-create entities from MQTT messages and populate asset_scans';
COMMENT ON TABLE identifiers IS 'Stores physical/logical identifiers (RFID, BLE, barcode, serial, etc.) with temporal validity';
COMMENT ON COLUMN identifiers.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN identifiers.type IS 'Identifier type: rfid, ble, barcode, serial, mac, qr, nfc, etc.';
COMMENT ON COLUMN identifiers.value IS 'The actual identifier value (EPC, MAC address, serial number, etc.)';
COMMENT ON COLUMN identifiers.asset_id IS 'Optional FK to asset - identifies one asset (mutually exclusive with location_id)';
COMMENT ON COLUMN identifiers.location_id IS 'Optional FK to location - identifies one location (mutually exclusive with asset_id)';
COMMENT ON COLUMN identifiers.valid_from IS 'Start of validity period for this identifier version';
COMMENT ON COLUMN identifiers.valid_to IS 'End of validity period for this identifier version';

COMMENT ON TABLE identifier_scans IS 'Raw MQTT message capture from RFID readers - pure data lake for identifier scans';
COMMENT ON COLUMN identifier_scans.created_at IS 'Timestamp when message was received';
COMMENT ON COLUMN identifier_scans.message_topic IS 'MQTT topic (e.g., trakrf.id/cs463-214/scan)';
COMMENT ON COLUMN identifier_scans.message_data IS 'Raw MQTT message payload as JSON';

COMMENT ON COLUMN asset_scans.identifier_scan_id IS 'Link to the source raw identifier scan for audit trail';
```

- [ ] **Step 2: Verify dollar-quote balance**

Run the same parser check as Task 1.1 Step 2 against the down file.
Expected: balanced `$$`.

### Task 1.3: Round-trip the migration on local DB

**Files:** none (DB-only)

- [ ] **Step 1: Apply, rollback, re-apply**

Run from project root:
```bash
just backend migrate                       # apply 000033 up
just backend migrate-status                # confirm version is 33
just backend migrate-down                  # roll back to 32
just backend migrate-status                # confirm version is 32
just backend migrate                       # apply again
just backend migrate-status                # confirm 33
```

Expected: each command exits 0, status reports the expected version, no errors about missing functions/triggers/indexes.

- [ ] **Step 2: Spot-check the catalog after up migration**

Requires `$PG_URL_LOCAL` exported (direnv loads it from `.env.local`). Run:

```bash
psql "$PG_URL_LOCAL" -c "SELECT 'tags exists' WHERE EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='trakrf' AND table_name='tags');"
psql "$PG_URL_LOCAL" -c "SELECT 'tag_scans hypertable' WHERE EXISTS(SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name='tag_scans');"
psql "$PG_URL_LOCAL" -c "\df trakrf.process_tag_scans"
psql "$PG_URL_LOCAL" -c "\df trakrf.create_asset_with_tags"
psql "$PG_URL_LOCAL" -c "\df trakrf.create_location_with_tags"
```

Expected: each query returns the renamed object and no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000033_rename_identifier_to_tag.up.sql \
        backend/migrations/000033_rename_identifier_to_tag.down.sql
git commit -m "$(cat <<'EOF'
chore(tra-524): db migration — rename identifier entity to tag

Renames identifiers→tags, identifier_scans→tag_scans, plus indexes,
sequence, triggers, RLS policy, FK column on asset_scans, and rewrites
the three PL/pgSQL function bodies. Down migration is symmetric.

TimescaleDB hypertable rename verified on preview env per TRA-524 AC §1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 2: Backend Go internals

**Goal:** Update Go to call the renamed PL/pgSQL functions, rename storage methods + DTOs + struct fields + JSON keys + handler methods + URL paths.

**Note:** Backend cannot compile + test against the migrated DB until this phase is complete; the migration ran in Phase 1 and the Go code currently expects the old names.

### Task 2.1: Move shared identifier model file

**Files:**
- Rename: `backend/internal/models/shared/identifier.go` → `backend/internal/models/shared/tag.go`

- [ ] **Step 1: Rename the file via git**

```bash
git mv backend/internal/models/shared/identifier.go \
       backend/internal/models/shared/tag.go
```

- [ ] **Step 2: Verify the file content is unchanged but tracked at the new path**

Run: `git status -s`
Expected: `R  backend/internal/models/shared/identifier.go -> backend/internal/models/shared/tag.go`

(Type names `TagIdentifier` / `TagIdentifierRequest` / `DefaultIdentifierType` stay — per TRA-524 AC §2.)

### Task 2.2: Rename storage layer file + tests

**Files:**
- Rename: `backend/internal/storage/identifiers.go` → `backend/internal/storage/tags.go`
- Rename: `backend/internal/storage/identifiers_test.go` → `backend/internal/storage/tags_test.go`
- Rename: `backend/internal/storage/identifiers_crossorg_test.go` → `backend/internal/storage/tags_crossorg_test.go`

- [ ] **Step 1: Git-mv the three files**

```bash
git mv backend/internal/storage/identifiers.go              backend/internal/storage/tags.go
git mv backend/internal/storage/identifiers_test.go         backend/internal/storage/tags_test.go
git mv backend/internal/storage/identifiers_crossorg_test.go backend/internal/storage/tags_crossorg_test.go
```

### Task 2.3: Rename storage method names + SQL strings in `tags.go`

**Files:**
- Modify: `backend/internal/storage/tags.go`

The file currently has:
- `GetIdentifiersByAssetID` (line 16) → `GetTagsByAssetID`
- `GetIdentifiersByLocationID` (line 49) → `GetTagsByLocationID`
- `AddIdentifierToAsset` (line 82) → `AddTagToAsset`
- `AddIdentifierToLocation` (line 105) → `AddTagToLocation`
- `RemoveAssetIdentifier` (line 133) → `RemoveAssetTag`
- `RemoveLocationIdentifier` (line 163) → `RemoveLocationTag`
- `GetIdentifierByID` (line 189) → `GetTagByID`
- Internal helper `parseIdentifierError` → `parseTagError`
- All SQL: `FROM trakrf.identifiers` → `FROM trakrf.tags`, `INSERT INTO trakrf.identifiers` → `INSERT INTO trakrf.tags`, `UPDATE trakrf.identifiers` → `UPDATE trakrf.tags`

- [ ] **Step 1: sed-rename method names + SQL strings**

The pattern is mechanical. Use a Python helper rather than sed to handle all variants safely:

```bash
python3 - <<'PY'
import re, pathlib
p = pathlib.Path("backend/internal/storage/tags.go")
src = p.read_text()
# Method-name replacements (exact)
replacements = [
    ("GetIdentifiersByAssetID",     "GetTagsByAssetID"),
    ("GetIdentifiersByLocationID",  "GetTagsByLocationID"),
    ("AddIdentifierToAsset",        "AddTagToAsset"),
    ("AddIdentifierToLocation",     "AddTagToLocation"),
    ("RemoveAssetIdentifier",       "RemoveAssetTag"),
    ("RemoveLocationIdentifier",    "RemoveLocationTag"),
    ("GetIdentifierByID",           "GetTagByID"),
    ("parseIdentifierError",        "parseTagError"),
    ("trakrf.identifiers",          "trakrf.tags"),
    ("get identifiers for asset",   "get tags for asset"),
    ("get identifiers for location","get tags for location"),
    ("failed to scan identifier",   "failed to scan tag"),
    ("remove asset identifier",     "remove asset tag"),
    ("remove location identifier",  "remove location tag"),
]
for old, new in replacements:
    src = src.replace(old, new)
p.write_text(src)
PY
```

- [ ] **Step 2: Inspect the diff manually**

Run: `git diff backend/internal/storage/tags.go | head -120`

Expected: the listed renames applied. **Variable** names like `identifier shared.TagIdentifier` and `identifierType` are local — review whether to leave or rename for readability. Recommended: rename `identifier` → `tag`, `identifierType` → `tagType`, `identifierID` → `tagID`. Apply with another small sed pass:

```bash
python3 - <<'PY'
import pathlib
p = pathlib.Path("backend/internal/storage/tags.go")
src = p.read_text()
# Local-var renames — be precise to avoid clobbering 'identifier' in
# concept-#1 SQL strings (e.g. "WHERE assets.identifier = $1"). The variable
# uses are word-boundary qualified; the SQL strings have surrounding chars.
import re
src = re.sub(r'\bidentifier\.', 'tag.', src)         # field access
src = re.sub(r'\bidentifier shared\.', 'tag shared.', src)  # type decl
src = re.sub(r'\bidentifierType\b', 'tagType', src)
src = re.sub(r'\bidentifierID\b', 'tagID', src)
src = re.sub(r'\bidentifierIDs\b', 'tagIDs', src)
src = re.sub(r'&identifier\b', '&tag', src)
p.write_text(src)
PY
```

- [ ] **Step 3: Compile-check the storage layer**

Run: `just backend lint`
Expected: `go fmt` and `go vet` clean. (Build will not yet pass — callers in handlers/storage still reference old names. That's fixed in 2.4.)

### Task 2.4: Update test files in storage to call renamed methods

**Files:**
- Modify: `backend/internal/storage/tags_test.go`
- Modify: `backend/internal/storage/tags_crossorg_test.go`

- [ ] **Step 1: Apply the same name replacements**

```bash
python3 - <<'PY'
import re, pathlib
for fp in ["backend/internal/storage/tags_test.go",
           "backend/internal/storage/tags_crossorg_test.go"]:
    p = pathlib.Path(fp)
    src = p.read_text()
    replacements = [
        ("GetIdentifiersByAssetID",     "GetTagsByAssetID"),
        ("GetIdentifiersByLocationID",  "GetTagsByLocationID"),
        ("AddIdentifierToAsset",        "AddTagToAsset"),
        ("AddIdentifierToLocation",     "AddTagToLocation"),
        ("RemoveAssetIdentifier",       "RemoveAssetTag"),
        ("RemoveLocationIdentifier",    "RemoveLocationTag"),
        ("GetIdentifierByID",           "GetTagByID"),
        ("trakrf.identifiers",          "trakrf.tags"),
        ("parseIdentifierError",        "parseTagError"),
    ]
    for old, new in replacements:
        src = src.replace(old, new)
    p.write_text(src)
PY
```

- [ ] **Step 2: Verify no references to old names remain**

Run: `grep -nE "(GetIdentifier|AddIdentifierTo|RemoveAssetIdentifier|RemoveLocationIdentifier|trakrf\\.identifiers)" backend/internal/storage/tags*.go || echo "clean"`
Expected: `clean`

### Task 2.5: Update SQL function calls in `assets.go` and `locations.go`

**Files:**
- Modify: `backend/internal/storage/assets.go` (line 461)
- Modify: `backend/internal/storage/locations.go` (line 584)

- [ ] **Step 1: Update the function-name SQL strings**

```bash
sed -i 's/create_asset_with_identifiers/create_asset_with_tags/g'    backend/internal/storage/assets.go
sed -i 's/create_location_with_identifiers/create_location_with_tags/g' backend/internal/storage/locations.go
```

- [ ] **Step 2: Verify the change is exactly two lines**

Run: `git diff --shortstat backend/internal/storage/assets.go backend/internal/storage/locations.go`
Expected: 2 files changed, 2 insertions(+), 2 deletions(-).

### Task 2.6: Rename Go DTOs and struct fields

**Files:**
- Modify: `backend/internal/models/asset/asset.go`
- Modify: `backend/internal/models/asset/public.go`
- Modify: `backend/internal/models/location/location.go`
- Modify: `backend/internal/models/location/public.go`

The renames:
- `CreateAssetWithIdentifiersRequest` → `CreateAssetWithTagsRequest`
- `CreateLocationWithIdentifiersRequest` → `CreateLocationWithTagsRequest`
- Struct field `Identifiers []shared.TagIdentifier` → `Tags []shared.TagIdentifier` on `AssetView`, `PublicAssetView`, `LocationView`, `PublicLocationView`, and the embedded request DTOs
- JSON tag `json:"identifiers"` → `json:"tags"` (this is the wire-contract change)

- [ ] **Step 1: Apply the renames**

```bash
python3 - <<'PY'
import pathlib, re
files = [
    "backend/internal/models/asset/asset.go",
    "backend/internal/models/asset/public.go",
    "backend/internal/models/location/location.go",
    "backend/internal/models/location/public.go",
]
for fp in files:
    p = pathlib.Path(fp)
    src = p.read_text()
    src = src.replace("CreateAssetWithIdentifiersRequest",   "CreateAssetWithTagsRequest")
    src = src.replace("CreateLocationWithIdentifiersRequest", "CreateLocationWithTagsRequest")
    # Struct field: `Identifiers []shared.TagIdentifier` (any whitespace before/after)
    src = re.sub(
        r'\bIdentifiers(\s+)\[\]shared\.TagIdentifier(\s*`json:")identifiers("`)',
        r'Tags\1[]shared.TagIdentifier\2tags\3',
        src,
    )
    src = re.sub(
        r'\bIdentifiers(\s+)\[\]shared\.TagIdentifierRequest(\s*`json:")identifiers(",omitempty[^`]*`)',
        r'Tags\1[]shared.TagIdentifierRequest\2tags\3',
        src,
    )
    # ToPublic*View constructor body: `Identifiers: a.Identifiers`
    src = re.sub(r'\bIdentifiers:\s+([al])\.Identifiers\b', r'Tags:        \1.Tags', src)
    p.write_text(src)
PY
```

- [ ] **Step 2: Inspect diffs**

Run: `git diff backend/internal/models/`
Expected: 4 files changed. The struct field, JSON key, and type-name renames are visible. No drift in unrelated lines.

- [ ] **Step 3: Verify no orphan references**

Run: `grep -nE "Identifiers\\s+\\[\\]shared\\.|json:\"identifiers\"|CreateAssetWithIdentifiersRequest|CreateLocationWithIdentifiersRequest" backend/internal/models/ -r || echo "clean"`
Expected: `clean`

### Task 2.7: Update handler methods + annotations + URL routes

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go`
- Modify: `backend/internal/handlers/locations/locations.go`
- Modify: `backend/internal/cmd/serve/router.go`

The renames:
- Method `AddIdentifier` → `AddTag` (on both Handlers)
- Method `RemoveIdentifier` → `RemoveTag`
- Method `AddIdentifierByID` → `AddTagByID`
- Method `RemoveIdentifierByID` → `RemoveTagByID`
- Internal helper `doAddAssetIdentifier` → `doAddAssetTag`, same for location
- DTO field `Identifiers` → `Tags` (where handlers reference the JSON body type)
- `@Router /api/v1/assets/{identifier}/identifiers[/...]` → `/tags[/...]`
- `@Router /api/v1/assets/by-id/{id}/identifiers[/...]` → `/tags[/...]`
- Same for locations
- `@Description`, `@Param identifierId` → `@Param tagId`, `@Tags Identifiers` → `@Tags Tags` (if present)
- Storage method calls: `AddIdentifierToAsset` → `AddTagToAsset`, etc. (matches Task 2.3)
- Variable names: `tagIdent` → `tag`, `identifierID` → `tagID` (locals)
- 8 router lines:

```go
// Old (router.go:164–172):
r.With(middleware.RequireScope("assets:write")).Post(  "/api/v1/assets/{identifier}/identifiers",                  assetsHandler.AddIdentifier)
r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{identifier}/identifiers/{identifierId}",  assetsHandler.RemoveIdentifier)
r.With(middleware.RequireScope("locations:write")).Post(  "/api/v1/locations/{identifier}/identifiers",                 locationsHandler.AddIdentifier)
r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{identifier}/identifiers/{identifierId}", locationsHandler.RemoveIdentifier)

// New:
r.With(middleware.RequireScope("assets:write")).Post(  "/api/v1/assets/{identifier}/tags",                  assetsHandler.AddTag)
r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{identifier}/tags/{tagId}",         assetsHandler.RemoveTag)
r.With(middleware.RequireScope("locations:write")).Post(  "/api/v1/locations/{identifier}/tags",                 locationsHandler.AddTag)
r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{identifier}/tags/{tagId}",         locationsHandler.RemoveTag)
```

Plus the same pattern on lines 189–196 (the `by-id/{id}/identifiers` routes → `by-id/{id}/tags`).

- [ ] **Step 1: Apply the renames in handlers**

```bash
python3 - <<'PY'
import pathlib, re
files = [
    "backend/internal/handlers/assets/assets.go",
    "backend/internal/handlers/locations/locations.go",
]
for fp in files:
    p = pathlib.Path(fp)
    src = p.read_text()
    # Method names
    repls = [
        ("AddIdentifierByID",         "AddTagByID"),
        ("RemoveIdentifierByID",      "RemoveTagByID"),
        ("AddIdentifier",             "AddTag"),
        ("RemoveIdentifier",          "RemoveTag"),
        ("doAddAssetIdentifier",      "doAddAssetTag"),
        ("doAddLocationIdentifier",   "doAddLocationTag"),
        # Storage method calls
        ("AddIdentifierToAsset",      "AddTagToAsset"),
        ("AddIdentifierToLocation",   "AddTagToLocation"),
        ("RemoveAssetIdentifier",     "RemoveAssetTag"),
        ("RemoveLocationIdentifier",  "RemoveLocationTag"),
        ("GetIdentifierByID",         "GetTagByID"),
        ("GetIdentifiersByAssetID",   "GetTagsByAssetID"),
        ("GetIdentifiersByLocationID","GetTagsByLocationID"),
        # DTO field accessor
        ("CreateAssetWithIdentifiersRequest",   "CreateAssetWithTagsRequest"),
        ("CreateLocationWithIdentifiersRequest","CreateLocationWithTagsRequest"),
        # Router annotations: URL path segment
        ("/identifiers/{identifierId}", "/tags/{tagId}"),
        ("/identifiers ",                "/tags "),   # trailing space in @Router lines
        ("/identifiers\n",               "/tags\n"),  # end-of-line variants
        # @Param
        ("@Param        identifierId", "@Param        tagId"),
        ("@Param identifierId",        "@Param tagId"),
        # @Description and English prose - common phrasings
        ("tag identifier(s)",          "tag(s)"),
        ("a tag identifier",           "a tag"),
        ("Add a tag identifier",       "Add a tag"),
        ("Remove a tag identifier",    "Remove a tag"),
        ("the tag identifier",         "the tag"),
        ("tag identifier ID",          "tag ID"),
        ("the identifier",             "the tag"),         # narrow — in @Description prose
        ("identifier(s)",              "tag(s)"),
    ]
    for old, new in repls:
        src = src.replace(old, new)
    # Local variable names — be word-boundary careful
    src = re.sub(r'\btagIdent\b', 'tag', src)
    src = re.sub(r'\bidentifierID\b', 'tagID', src)
    p.write_text(src)
PY
```

- [ ] **Step 2: Apply renames in `router.go`**

```bash
python3 - <<'PY'
import pathlib, re
p = pathlib.Path("backend/internal/cmd/serve/router.go")
src = p.read_text()
# URL path segment + path parameter renames in chi route literals
src = src.replace("/identifiers/{identifierId}", "/tags/{tagId}")
src = re.sub(r'(/api/v1/(?:assets|locations)/[^"]*?)/identifiers"', r'\1/tags"', src)
# Handler method references
src = src.replace("assetsHandler.AddIdentifierByID",       "assetsHandler.AddTagByID")
src = src.replace("assetsHandler.RemoveIdentifierByID",    "assetsHandler.RemoveTagByID")
src = src.replace("assetsHandler.AddIdentifier",           "assetsHandler.AddTag")
src = src.replace("assetsHandler.RemoveIdentifier",        "assetsHandler.RemoveTag")
src = src.replace("locationsHandler.AddIdentifierByID",    "locationsHandler.AddTagByID")
src = src.replace("locationsHandler.RemoveIdentifierByID", "locationsHandler.RemoveTagByID")
src = src.replace("locationsHandler.AddIdentifier",        "locationsHandler.AddTag")
src = src.replace("locationsHandler.RemoveIdentifier",     "locationsHandler.RemoveTag")
p.write_text(src)
PY
```

- [ ] **Step 3: Verify routes**

Run: `grep -n "/identifiers\\|/tags\\b" backend/internal/cmd/serve/router.go`
Expected: 8 lines all matching `/tags`, none matching `/identifiers`.

- [ ] **Step 4: Compile**

Run: `just backend lint`
Expected: `go fmt` and `go vet` clean.

Run: `cd backend && go build ./... && cd ..`
Expected: clean build, no errors.

If `go build` reports unresolved references, those are likely places where the regex-based renames missed an edge case (e.g., a backtick literal inside an annotation, an inline test string). Fix by hand and re-run.

### Task 2.8: Run backend unit tests

- [ ] **Step 1: Run unit tests**

Run: `just backend test`
Expected: all tests pass.

If integration tests reference the renamed function or table by SQL string, those need updating too. Search for stragglers:

```bash
grep -rn "create_asset_with_identifiers\|create_location_with_identifiers\|process_identifier_scans\|trakrf\\.identifiers\\b\|trakrf\\.identifier_scans\\b" backend/internal/ --include="*.go" || echo "clean"
```
Expected: `clean`

- [ ] **Step 2: Commit**

```bash
git add -A backend/internal/ backend/migrations/
git commit -m "$(cat <<'EOF'
chore(tra-524): rename Identifier → Tag in Go internals

Storage methods, DTOs, struct fields, JSON keys, handler methods, route
paths, @Description/@Param/@Router annotations all updated. SQL strings
point at renamed PL/pgSQL functions. URL path segment /identifiers → /tags.
JSON wire field .identifiers → .tags on PublicAssetView, PublicLocationView,
AssetView, LocationView, and create-request DTOs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3: Regenerate OpenAPI / swagger artifacts

**Goal:** Run `just backend api-spec` to regenerate `backend/docs/docs.go`, `docs/api/openapi.public.{json,yaml}`, and the embedded `swaggerspec/` files. These all flow from the Go annotations updated in Phase 2.

### Task 3.1: Run the spec regeneration

- [ ] **Step 1: Regenerate**

Run: `just backend api-spec`
Expected: command exits 0; final lines say "Public spec: docs/api/openapi.public.{json,yaml} (committed) + swaggerspec/ (gitignored, embedded)" and "Internal spec: backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml} (gitignored, embedded)".

- [ ] **Step 2: Check the public-spec diff is exactly the rename**

Run: `git diff docs/api/openapi.public.yaml | head -200`
Expected: changes confined to:
- Path entries `/api/v1/assets/{identifier}/identifiers[*]` → `/api/v1/assets/{identifier}/tags[*]`
- Path parameter `identifierId` → `tagId`
- Operation IDs `assets.identifiers.add` → `assets.tags.add` etc.
- Schema rename `asset.CreateAssetWithIdentifiersRequest` → `asset.CreateAssetWithTagsRequest`, same for location
- Field rename `identifiers` → `tags` in `PublicAssetView`/`PublicLocationView`/`AssetView`/`LocationView` and their request counterparts
- `@Description` prose updates from "tag identifier" to "tag"

If the diff includes anything else (unrelated reordering, accidental schema reshape), stop and investigate before committing.

- [ ] **Step 3: Stage the regenerated files**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml backend/docs/
```

(The `swaggerspec/` files are gitignored and live only at runtime — `git status` should not show them.)

- [ ] **Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
chore(tra-524): regenerate OpenAPI + swagger from updated annotations

Reflects URL-path rename /identifiers → /tags, path param identifierId → tagId,
field rename .identifiers → .tags, schema rename CreateAssetWithIdentifiersRequest
→ CreateAssetWithTagsRequest, and prose updates in @Description.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 4: Frontend cutover companion

**Goal:** Mirror every backend rename in TypeScript so the frontend compiles and stays runtime-compatible with the new API surface.

### Task 4.1: Rename + widen TagType

**Files:**
- Rename: `frontend/src/types/shared/identifier.ts` → `frontend/src/types/shared/tag.ts`
- Modify: `frontend/src/types/shared/index.ts`

- [ ] **Step 1: Move and rewrite the type file**

```bash
git mv frontend/src/types/shared/identifier.ts frontend/src/types/shared/tag.ts
```

Now overwrite the file's content:

```ts
/**
 * Tag Types
 *
 * Types for physical-tag entities (RFID, BLE, NFC, barcode) linked to assets and locations.
 * Matches backend: backend/internal/models/shared/tag.go
 */

/**
 * Tag type — supported physical-tag technologies
 */
export type TagType = 'rfid' | 'ble' | 'nfc' | 'barcode';

/**
 * Tag entity — returned from API
 * Reference: backend/internal/models/shared/tag.go TagIdentifier (Go type name kept)
 */
export interface TagIdentifier {
  id: number;
  type: TagType;
  value: string;
  is_active: boolean;
}
```

(Note: the Go type name `TagIdentifier` is preserved per AC §2; the TS interface keeps the same name for parity.)

- [ ] **Step 2: Update the re-export in shared/index.ts**

Edit `frontend/src/types/shared/index.ts`:

```ts
export * from './tag';
```

(Replace the line `export * from './identifier';`.)

### Task 4.2: Rename TS field on Asset / Location types

**Files:**
- Modify: `frontend/src/types/assets/index.ts`
- Modify: `frontend/src/types/locations/index.ts`

- [ ] **Step 1: Find current field declarations**

Run:
```bash
grep -nE "(IdentifierType|identifiers\\?\\?|identifiers:\\s|TagIdentifierInput)" \
  frontend/src/types/assets/index.ts frontend/src/types/locations/index.ts
```

Expected output: shows `identifiers` field declarations on `Asset`, `AssetWithLocation`, `Location`, and the `*WithIdentifiersInput` interfaces, plus `TagIdentifierInput` (currently named per qualifier-prefix).

- [ ] **Step 2: Apply renames**

```bash
python3 - <<'PY'
import pathlib, re
for fp in ["frontend/src/types/assets/index.ts",
           "frontend/src/types/locations/index.ts"]:
    p = pathlib.Path(fp)
    src = p.read_text()
    # Field rename on interfaces
    src = re.sub(r'\bidentifiers\?: TagIdentifier\[\]', 'tags?: TagIdentifier[]', src)
    src = re.sub(r'\bidentifiers: TagIdentifier\[\]',  'tags: TagIdentifier[]',  src)
    src = re.sub(r'\bidentifiers\?: TagIdentifierInput\[\]', 'tags?: TagIdentifierInput[]', src)
    src = re.sub(r'\bidentifiers: TagIdentifierInput\[\]',  'tags: TagIdentifierInput[]',  src)
    # CreateAssetWithIdentifiersInput → CreateAssetWithTagsInput
    src = src.replace("CreateAssetWithIdentifiersInput",   "CreateAssetWithTagsInput")
    src = src.replace("CreateLocationWithIdentifiersInput","CreateLocationWithTagsInput")
    # IdentifierType → TagType
    src = src.replace("IdentifierType", "TagType")
    p.write_text(src)
PY
```

- [ ] **Step 3: Inspect**

Run: `git diff frontend/src/types/`
Expected: clean field/type renames; no unrelated drift.

### Task 4.3: Update frontend API client

**Files:**
- Modify: `frontend/src/lib/api/assets/index.ts`
- Modify: `frontend/src/lib/api/locations/index.ts`

The renames:
- Methods `addIdentifier` → `addTag`, `removeIdentifier` → `removeTag`
- URL paths in fetch calls: `/identifiers` → `/tags`, `/identifiers/${identifierId}` → `/tags/${tagId}`
- JSDoc comments: "tag identifier" → "tag"
- Local parameter names: `identifierId` → `tagId`, `identifier` (the body type) → `tag`

- [ ] **Step 1: Apply renames**

```bash
python3 - <<'PY'
import pathlib, re
for fp in ["frontend/src/lib/api/assets/index.ts",
           "frontend/src/lib/api/locations/index.ts"]:
    p = pathlib.Path(fp)
    src = p.read_text()
    # Method names
    src = re.sub(r'\baddIdentifier\b',    'addTag',    src)
    src = re.sub(r'\bremoveIdentifier\b', 'removeTag', src)
    # URL path segments
    src = src.replace("/identifiers/${identifierId}", "/tags/${tagId}")
    src = re.sub(r'/identifiers([`"])', r'/tags\1', src)
    # Param names
    src = re.sub(r'\bidentifierId\b', 'tagId', src)
    # JSDoc prose
    src = src.replace("tag identifier", "tag")
    src = src.replace("Tag identifier", "Tag")
    src = src.replace("identifier - ", "tag - ")  # JSDoc @param prose
    p.write_text(src)
PY
```

- [ ] **Step 2: Inspect**

Run: `git diff frontend/src/lib/api/`
Expected: about 20 changed lines across two files. Method names, URL paths, JSDoc prose, parameter names all updated consistently.

### Task 4.4: Update component / store / utility callsites

**Files (callsites that read `.identifiers` from typed Asset/Location objects):**
- Modify: `frontend/src/components/assets/AssetForm.tsx`
- Modify: `frontend/src/components/assets/AssetCard.tsx`
- Modify: `frontend/src/components/assets/AssetDetailsModal.tsx`
- Modify: `frontend/src/components/locations/LocationForm.tsx`
- Modify: `frontend/src/components/locations/LocationCard.tsx`
- Modify: `frontend/src/components/locations/LocationDetailsModal.tsx`
- Modify: `frontend/src/components/locations/LocationDetailsPanel.tsx`
- Modify: `frontend/src/stores/locations/locationActions.ts`
- Modify: `frontend/src/lib/asset/filters.ts`
- Modify: `frontend/src/utils/export/assetExport.ts`

- [ ] **Step 1: Run a TypeScript-typecheck-driven find**

Run: `just frontend typecheck 2>&1 | head -50`
Expected: errors at every callsite that still uses `.identifiers`. The compiler is now the source of truth — fix the listed locations.

- [ ] **Step 2: Apply the field-access rename**

```bash
python3 - <<'PY'
import pathlib, re

# Targeted access pattern: object.identifiers (concept #2 array). Avoid renaming
# `obj.identifier` (singular — concept #1, the natural-key field, NOT renamed).
files = [
    "frontend/src/components/assets/AssetForm.tsx",
    "frontend/src/components/assets/AssetCard.tsx",
    "frontend/src/components/assets/AssetDetailsModal.tsx",
    "frontend/src/components/locations/LocationForm.tsx",
    "frontend/src/components/locations/LocationCard.tsx",
    "frontend/src/components/locations/LocationDetailsModal.tsx",
    "frontend/src/components/locations/LocationDetailsPanel.tsx",
    "frontend/src/stores/locations/locationActions.ts",
    "frontend/src/lib/asset/filters.ts",
    "frontend/src/utils/export/assetExport.ts",
]
# Only match `.identifiers` (with the trailing 's') — concept #2 array field.
# Concept #1 is `.identifier` (singular).
pattern = re.compile(r'\.identifiers\b')
for fp in files:
    p = pathlib.Path(fp)
    src = p.read_text()
    new = pattern.sub('.tags', src)
    if new != src:
        p.write_text(new)
        print(f"updated {fp}")
PY
```

- [ ] **Step 3: Re-run typecheck**

Run: `just frontend typecheck`
Expected: clean. If new errors appear, they likely involve local-var names or destructuring patterns the regex missed (e.g. `const { identifiers } = asset`). Fix by hand:

```bash
grep -rnE "(\\bidentifiers\\b|TagIdentifierInput\\b|CreateAssetWithIdentifiersInput|CreateLocationWithIdentifiersInput)" frontend/src/ --include="*.ts" --include="*.tsx" | grep -v "tag identifier" | head -30
```

Treat each remaining hit case-by-case: if it's a concept-#2 reference, rename to `tags`; if it's an unrelated local var, leave it.

- [ ] **Step 4: Run frontend unit tests**

Run: `just frontend test`
Expected: pass. If a test in `frontend/src/lib/api/assets/assets.test.ts` (or similar) asserts on URL paths or method names, update it the same way as the source files.

- [ ] **Step 5: Lint**

Run: `just frontend lint`
Expected: clean.

### Task 4.5: Update e2e fixtures and specs

**Files:**
- Modify: `frontend/tests/e2e/inventory-save.spec.ts` (lines 59, 71, 110)
- Modify: any other e2e test that POSTs `identifiers: [...]` payloads

- [ ] **Step 1: Find e2e tests with `identifiers:` payloads**

Run: `grep -rnE "identifiers\\s*[:=]\\s*\\[" frontend/tests/e2e/ --include="*.ts"`
Expected: list of files (at minimum `inventory-save.spec.ts`).

- [ ] **Step 2: Apply the rename**

```bash
python3 - <<'PY'
import pathlib, re
import subprocess
out = subprocess.run(
    ["grep", "-rlE", r"identifiers\s*[:=]\s*\[", "frontend/tests/e2e/", "--include=*.ts"],
    capture_output=True, text=True
).stdout.splitlines()
for fp in out:
    p = pathlib.Path(fp)
    src = p.read_text()
    # `identifiers: [...]` → `tags: [...]` in object literals
    src = re.sub(r'\bidentifiers(\s*:\s*)\[', r'tags\1[', src)
    # const identifiers = ... → const tags = ... when followed by use as `tags` in payload
    src = re.sub(r'\bconst identifiers\b', 'const tags', src)
    # Then the `identifiers,` shorthand in payload object → `tags,`
    src = re.sub(r'^(\s*)identifiers,', r'\1tags,', src, flags=re.M)
    p.write_text(src)
PY
```

- [ ] **Step 3: Inspect**

Run: `git diff frontend/tests/e2e/`
Expected: just the payload-key + variable rename. No semantic changes.

- [ ] **Step 4: Commit Phase 4**

```bash
git add -A frontend/
git commit -m "$(cat <<'EOF'
chore(tra-524): rename frontend mirror — types, API client, callsites, e2e

TypeScript field accessors `.identifiers` → `.tags` to match the renamed
backend JSON wire shape. API client methods addIdentifier/removeIdentifier
→ addTag/removeTag with new URL paths /tags. Type rename IdentifierType
→ TagType (widened to 'rfid' | 'ble' | 'nfc' | 'barcode'). E2E inventory
fixture payloads updated to send `tags: [...]`.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 5: Ingester (Redpanda Connect)

**Goal:** Update `connect.yaml` to write to `tag_scans` instead of `identifier_scans`.

### Task 5.1: Update connect.yaml

**Files:**
- Modify: `ingester/connect.yaml` (line 21)

- [ ] **Step 1: Apply the rename**

```bash
sed -i 's/INSERT INTO trakrf\.identifier_scans/INSERT INTO trakrf.tag_scans/' ingester/connect.yaml
```

- [ ] **Step 2: Verify**

Run: `git diff ingester/connect.yaml`
Expected: exactly one line changed:

```diff
-    query: "INSERT INTO trakrf.identifier_scans (message_topic, message_data) VALUES ($1, $2)"
+    query: "INSERT INTO trakrf.tag_scans (message_topic, message_data) VALUES ($1, $2)"
```

- [ ] **Step 3: Commit**

```bash
git add ingester/connect.yaml
git commit -m "$(cat <<'EOF'
chore(tra-524): ingester writes to tag_scans

Updates Redpanda Connect SQL output to point at the renamed hypertable.
Deploy AFTER backend cutover lands per TRA-524 AC §4 — failure mode is
loud (relation does not exist) until ingester redeploys.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 6: Documentation prose

**Goal:** Update markdown docs that describe the data model so they reflect the new naming. These don't break anything if they ship before or after — but for review legibility they belong in this PR.

### Task 6.1: Update schema-naming-conventions.md

**Files:**
- Modify: `docs/schema-naming-conventions.md`

- [ ] **Step 1: Read current state**

Run: `grep -n "identifier" docs/schema-naming-conventions.md | head -20`
Expected: paragraphs referencing the dual-meaning of `identifier`.

- [ ] **Step 2: Add a section explicitly defining `tag` vs `identifier`**

Append (or replace the existing identifier paragraph with) a section that draws the line:

```markdown
## `identifier` vs `tag`

The codebase uses two distinct concepts that previously shared the name `identifier`. As of TRA-524 they are differentiated:

- **`identifier`** — the natural-key column convention applied to entity tables (`assets`, `locations`, `scan_devices`, `scan_points`, `organizations`). It is the human-meaningful key the customer assigns to a record (e.g. `asset.identifier = "WIDGET-001"`). Universally typed `VARCHAR(255)`. Always present on the row's owning entity.

- **`tag`** — a physical or logical identification device associated with an asset or location. RFID tag, BLE beacon, NFC tag, barcode sticker. Lives in its own table (`tags`) with a temporal validity window. An asset or location can have zero or more tags; a tag belongs to exactly one asset OR one location.

In prose, prefer "tag" for the physical-device concept and "identifier" for the natural-key column. Avoid "tag identifier" as a phrase — it conflates the two.
```

(Use Edit/Write to apply — exact placement depends on the existing structure.)

### Task 6.2: Update logical-schema.md

**Files:**
- Modify: `docs/logical-schema.md`

- [ ] **Step 1: Find existing entity descriptions for the identifiers entity**

Run: `grep -nB2 -A8 "identifiers\\|Identifier" docs/logical-schema.md | head -60`

- [ ] **Step 2: Rewrite the entity section**

Wherever the doc describes the `identifiers` entity, rename it to `tags` and update prose. The description should mention all four supported tag types: RFID, BLE, NFC, barcode. Apply via Edit on the specific lines.

- [ ] **Step 3: Commit Phase 6**

```bash
git add docs/schema-naming-conventions.md docs/logical-schema.md
git commit -m "$(cat <<'EOF'
docs(tra-524): describe tag entity + identifier/tag distinction

Adds explicit prose distinguishing the natural-key column convention
(`identifier`) from the physical-device entity (`tag`). Updates the
logical-schema entity description.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 7: Verification (full pass)

**Goal:** Make sure the whole stack runs cleanly end-to-end before opening the PR.

### Task 7.1: Backend full test run

- [ ] **Step 1: Lint + unit + integration tests**

Run: `just backend test`
Expected: all pass against the migrated DB.

- [ ] **Step 2: Build**

Run: `cd backend && go build ./... && cd ..`
Expected: clean.

### Task 7.2: Frontend full test run

- [ ] **Step 1: Lint + typecheck + unit + build**

Run: `just frontend lint && just frontend typecheck && just frontend test && just frontend build`
Expected: each step exits 0.

### Task 7.3: E2E against local backend

- [ ] **Step 1: Start backend**

In one terminal:
```bash
just backend dev
```

In a second terminal:
```bash
just frontend dev
```

Wait for both to come up.

- [ ] **Step 2: Run e2e suite (headless)**

Run: `just frontend test-e2e`
Expected: all e2e tests pass against the renamed API.

(If you can't start both servers locally, push the branch and run the e2e suite against the preview deploy with `just frontend test-e2e-remote https://app.preview.trakrf.id` after the PR is open.)

### Task 7.4: Migration round-trip on preview

- [ ] **Step 1: Apply against preview DB**

Connect via `$PG_URL_PREVIEW` and run:
```bash
PGURL="$PG_URL_PREVIEW" go run ./backend migrate
```

Or use the embedded migration via the backend's `serve migrate` subcommand if direnv is set up.

Expected: migration applies cleanly. (Preview DB has very little data; rollback safety is high.)

- [ ] **Step 2: Spot-check the catalog**

```bash
psql "$PG_URL_PREVIEW" -c "\dt trakrf.tags*; \dt trakrf.tag_scans"
psql "$PG_URL_PREVIEW" -c "SELECT job_id, hypertable_name FROM timescaledb_information.jobs WHERE hypertable_schema='trakrf';"
```

Expected: `tags` and `tag_scans` exist; retention job points at `tag_scans`.

### Task 7.5: Final sanity sweep

- [ ] **Step 1: Search for stragglers across the whole repo**

Run:
```bash
grep -rnE "(create_asset_with_identifiers|create_location_with_identifiers|process_identifier_scans|trakrf\\.identifiers\\b|trakrf\\.identifier_scans\\b|TagIdentifierInput\\b|/identifiers/\\{|\\.identifiers\\b)" \
  --include="*.go" --include="*.ts" --include="*.tsx" --include="*.sql" --include="*.yaml" --include="*.yml" --include="*.md" \
  | grep -v "node_modules\\|.git/\\|tra-523\\|tra-524\\|2026-04-26-tra-523-identifier-overload\\|migrations/000009\\|migrations/000010\\|migrations/000015\\|migrations/000024\\|migrations/000032\\|migrations/000033"
```

Expected: empty (no remaining stragglers). The `grep -v` excludes prior migrations (immutable history), the analysis doc (intentional historical reference), and this plan doc. If anything else shows, investigate and fix.

- [ ] **Step 2: Push the branch**

```bash
git push -u origin miks2u/tra-524-rename-identifier-tag
```

### Task 7.6: Open the PR

- [ ] **Step 1: Open the PR via gh**

```bash
gh pr create --title "chore(tra-524): rename Identifier entity → Tag (cutover)" \
  --body "$(cat <<'EOF'
## Summary
- Renames the physical-tag entity from `identifier` → `tag` across DB, Go internals, URL paths, MQTT ingester, frontend mirror, and docs prose
- Natural-key column convention also called `identifier` (on assets/locations/scan_devices/scan_points/organizations) **stays unchanged**
- Single atomic cutover; UI labels and component filenames are out of scope (sibling ticket)

## Linear
TRA-524 (parent: TRA-522). Background: TRA-523 analysis at `docs/superpowers/plans/2026-04-26-tra-523-identifier-overload-analysis.md`. Plan: `docs/superpowers/plans/2026-04-26-tra-524-rename-identifier-tag.md`.

## Deploy ordering
Backend (with migration) deploys FIRST. Ingester deploys SECOND once `connect.yaml` ships. Failure mode during the gap is a loud `relation "trakrf.identifier_scans" does not exist` error in ingester logs — acceptable because fixed-reader ingestion is not customer-facing yet.

## Test plan
- [ ] `just backend test` passes
- [ ] `just frontend lint && just frontend typecheck && just frontend test && just frontend build` all pass
- [ ] E2E pass locally OR against preview after deploy
- [ ] Spot-check renamed catalog on preview after deploy: `tags`, `tag_scans` exist; retention job intact
- [ ] After merge: redeploy ingester service immediately

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review Checklist (run before marking the plan complete)

- [x] **Spec coverage:** Every TRA-524 AC bullet (§1 DB, §2 Go, §3 API, §3a frontend, §4 MQTT, §5 type widening, §6 docs, §7 verification) maps to a task above.
- [x] **No placeholders:** every step has either an exact command, exact file path, or actual code/SQL.
- [x] **Type consistency:** Go method names, TS field names, URL paths, and DB names line up across phases. e.g. `AddTag` (Go), `addTag` (TS), `/tags` (URL), `tags` (DB) — all consistent.
- [x] **Atomicity preserved:** the cutover lands as one PR; commits are granular within the PR for review legibility.
- [x] **Out-of-scope is explicit:** UI labels and component filenames listed as Nick's territory; field accessors `.identifiers` listed as in-scope.
