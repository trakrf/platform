# Asset/Location Dimension–Fact Split (TRA-799, expanded) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `current_location` purely fact data — drop the denormalized `assets.current_location_id` column, remove `location_id`/`location_external_key` from the asset API request *and response* shapes, drop the `?location` filter from `GET /assets`, rewrite the location-delete guard against scans, and re-source the assets-screen location display from `/reports/asset-locations`.

**Architecture:** Assets are a dimension table; an asset's current location is fact data derived from the latest `asset_scans` row. The asset resource (`PublicAssetView`) carries only dimension attributes. Current location is read exclusively through the reporting layer (`GET /api/v1/reports/asset-locations`, `GET /api/v1/assets/{id}/history`). The frontend assets screen keeps its location display but sources it from the reports endpoint.

**Tech Stack:** Go backend (pgx, golang-migrate, TimescaleDB), React/TypeScript frontend (React Query + Zustand), `just` task runner.

**Scope note:** This expands TRA-799 beyond its written scope per the ticket author's 2026-05-22 direction (remove location from the GET response too — "stop commingling dimension and fact data"). Acceptance criterion #1's "still present read-only on PublicAssetView" is intentionally superseded. Backend + frontend ship in one PR (co-deploy required — embedded binary).

---

## File Structure

### Backend — created
- `backend/migrations/000043_drop_asset_current_location.up.sql` — backfill scans, redefine `create_asset_with_tags`, drop index + column.
- `backend/migrations/000043_drop_asset_current_location.down.sql` — re-add column/index/comment, restore 10-arg function.

### Backend — modified
- `backend/internal/models/asset/asset.go` — drop `Asset.LocationID`, `UpdateAssetRequest.LocationID/LocationExternalKey`, `ListFilter.LocationIDs/LocationExternalKeys`; collapse `AssetWithLocation` → alias of `AssetView`.
- `backend/internal/models/asset/public.go` — drop `PublicAssetView.LocationID/LocationExternalKey`; `ToPublicAssetView` takes `AssetView`.
- `backend/internal/storage/assets.go` — remove `current_location_id` from every query; simplify the 3 scan-join read queries; drop the `?location` filter from `buildAssetsWhere`; drop 3 dead `current_location_id_fkey` checks; 9-arg `create_asset_with_tags` call.
- `backend/internal/storage/locations.go` — rewrite `CountActiveAssetsAtLocation` against latest-scan location.
- `backend/internal/handlers/assets/assets.go` — drop PATCH location echo blocks; add `location_id`/`location_external_key` to `PublicRejectPatchFields`; update `assetLocationReadOnlyMessage`; drop `?location` filter handling from `ListAssets`.
- `backend/internal/tools/apispec/postprocess.go` — drop asset `location_id`/`location_external_key` schema metadata entries.

### Backend — tests
- DELETE: `current_location_consistency_integration_test.go`, `patch_natural_key_integration_test.go`, `error_detail_url_integration_test.go`.
- UPDATE: `delete_conflict_integration_test.go`, `fk_validation_integration_test.go`, `list_external_key_integration_test.go`, `patch_round_trip_integration_test.go`, `apispec/postprocess_test.go`, `database/seeds/contract_test_seed.sql`.

### Frontend — created
- `frontend/src/hooks/reports/useAssetLocations.ts` — `Map<assetId, CurrentLocationItem>` from `useCurrentLocations({fetchAll:true})`.

### Frontend — modified
- `frontend/src/types/assets/index.ts` — drop `location_id`/`location_external_key` from `Asset`, `CreateAssetRequest`, `UpdateAssetRequest`; drop the location filter type.
- `AssetForm.tsx` — remove location selector + payload field.
- `AssetCard.tsx`, `AssetTable.tsx`, `AssetDetailsModal.tsx` — source location from `useAssetLocations()`.
- `assetExport.ts` (+ its caller) — source location from the asset-location map.
- `AssetFilters.tsx`, `AssetSearchSort.tsx`, `AssetsScreen.tsx`, `stores/assets/assetStore.ts`, `stores/assets/assetActions.ts` — remove the location filter.
- Tests: `lookup.test.ts`, `AssetTable.test.tsx`, `AssetSearchSort.test.tsx`, `assetExport.test.ts`.

### Regenerated
- `backend/internal/handlers/swaggerspec/openapi.{public,internal}.{json,yaml}` via `just backend api-spec`.

---

## Phase 1 — Migration

### Task 1: Create the migration

**Files:**
- Create: `backend/migrations/000043_drop_asset_current_location.up.sql`
- Create: `backend/migrations/000043_drop_asset_current_location.down.sql`

- [ ] **Step 1: Write `000043_drop_asset_current_location.up.sql`**

```sql
SET search_path = trakrf,public;

-- TRA-799: current_location becomes purely derived from the latest asset_scans
-- row. Drop the denormalized assets.current_location_id column.

-- 1. Backfill — preserve legacy create-time locations as scan rows before the
--    column is dropped. Idempotent via NOT EXISTS: no-ops on prod/preview
--    (already applied by hand), runs correctly on fresh/restored databases.
INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id)
SELECT a.updated_at, a.org_id, a.id, a.current_location_id
FROM assets a
WHERE a.current_location_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM asset_scans s WHERE s.asset_id = a.id);

-- 2. Redefine create_asset_with_tags() without p_current_location_id — asset
--    location is scan/operational data, never set on create (TRA-734).
DROP FUNCTION IF EXISTS create_asset_with_tags(
    INT, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

CREATE FUNCTION create_asset_with_tags(
    p_org_id INT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
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
        org_id, external_key, name, description,
        valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
        p_valid_from, p_valid_to, p_is_active, p_metadata
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

-- 3. Drop the denormalized column and its index.
DROP INDEX IF EXISTS idx_assets_current_location;
ALTER TABLE assets DROP COLUMN current_location_id;
```

- [ ] **Step 2: Write `000043_drop_asset_current_location.down.sql`**

```sql
SET search_path = trakrf,public;

-- Re-add the denormalized column, index, and comment. Data is NOT restored:
-- the up-migration's backfilled asset_scans rows remain (removing them is
-- unsafe — indistinguishable from genuine device reads). current_location_id
-- comes back NULL for all rows.
ALTER TABLE assets ADD COLUMN current_location_id INT REFERENCES locations(id);
CREATE INDEX idx_assets_current_location ON assets(current_location_id);
COMMENT ON COLUMN assets.current_location_id IS 'Denormalized current location for query performance';

-- Restore create_asset_with_tags() with the p_current_location_id parameter.
DROP FUNCTION IF EXISTS create_asset_with_tags(
    INT, VARCHAR, VARCHAR, TEXT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

CREATE FUNCTION create_asset_with_tags(
    p_org_id INT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
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
        org_id, external_key, name, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
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
```

- [ ] **Step 3: Commit** — `git add backend/migrations/000043_* && git commit -m "feat(db): drop assets.current_location_id, derive location from scans (TRA-799)"`

Note: the migration is verified end-to-end at Phase 7 (`just backend test` runs migrations against a real DB).

---

## Phase 2 — Backend models

### Task 2: Strip location from asset model structs

**Files:**
- Modify: `backend/internal/models/asset/asset.go`
- Modify: `backend/internal/models/asset/public.go`

- [ ] **Step 1: `asset.go` — `Asset` struct:** remove the `LocationID *int` field (line 18).
- [ ] **Step 2: `asset.go` — `UpdateAssetRequest`:** remove `LocationID *int` and `LocationExternalKey *string` (lines 113–115). Rewrite the doc comment: drop the TRA-699 location-echo-fields paragraph; state location is not part of the asset request shape (scan/operational data). Keep `ExternalKey *string` (still an echo field for rename).
- [ ] **Step 3: `asset.go` — `AssetWithLocation`:** replace the struct with `type AssetWithLocation = AssetView` (type alias) so existing storage/handler signatures keep compiling; add a comment that location is no longer carried and the alias is retained transitionally. (Full rename to `AssetView` is optional cleanup — alias keeps the diff bounded.)
- [ ] **Step 4: `asset.go` — `ListFilter`:** remove `LocationIDs []int` and `LocationExternalKeys []string` (lines 186–188) and their doc comment.
- [ ] **Step 5: `public.go` — `PublicAssetView`:** remove `LocationID *int` and `LocationExternalKey *string` (lines 27–28). Update the type doc comment (drop the TRA-555/TRA-580 location paragraph).
- [ ] **Step 6: `public.go` — `ToPublicAssetView`:** change signature to `func ToPublicAssetView(a AssetView) PublicAssetView`; remove the `LocationID`/`LocationExternalKey` projections.
- [ ] **Step 7:** Backend will not compile until Phase 3–5 land. Do not build yet. Commit at end of Phase 5.

---

## Phase 3 — Backend storage

### Task 3: Strip `current_location_id` from `storage/assets.go`

**Files:** Modify `backend/internal/storage/assets.go`

- [ ] **Step 1: `CreateAsset`** (lines 16–58): remove `current_location_id` from the INSERT column list and the `RETURNING` list; renumber placeholders ($1–$8); drop `request.LocationID` from the arg list and `&asset.LocationID` from the `Scan`; delete the `current_location_id_fkey` error branch (lines 51–53).
- [ ] **Step 2: `UpdateAsset`** (lines 90–151): delete the `current_location_id_fkey` error branch (lines 144–146).
- [ ] **Step 3: `GetAssetByID`** (lines 200–223): remove `current_location_id` from `SELECT` and `&asset.LocationID` from `Scan`.
- [ ] **Step 4: `GetAssetsByIDs`** (lines 229–267): same removal.
- [ ] **Step 5: `ListAllAssets`** (lines 269–304): same removal.
- [ ] **Step 6: `getAssetWithLocationByID`** (lines 578–637): collapse to a plain single-asset select — drop the `latest_scan` CTE and both `LEFT JOIN`s, drop `l.external_key` from the select and `locExtKey` from the scan. Return `&asset.AssetView{Asset: a, Tags: tags}`. Rename the method to `getAssetViewWithTagsByID` and update `GetAssetWithLocationByID` (exported wrapper, lines 1015–1021) to `GetAssetViewWithTagsByID`; update the 3 internal call sites (`UpdateAsset`, `RenameAsset`, `CreateAssetWithTags`).
- [ ] **Step 7: `GetAssetByExternalKey`** (lines 677–741): same collapse — plain select from `assets`, no scan join, no `locExtKey`. Returns `*asset.AssetView`.
- [ ] **Step 8: `ListAssetsFiltered`** (lines 793–881): drop the `latest_scans` CTE and both `LEFT JOIN`s; select straight `FROM trakrf.assets a`; drop `l.external_key`/`locExtKey`. Result element type `asset.AssetView`.
- [ ] **Step 9: `CountAssetsFiltered`** (lines 884–914): drop the `latest_scans` CTE and joins; `SELECT COUNT(*) FROM trakrf.assets a WHERE %s`.
- [ ] **Step 10: `buildAssetsWhere`** (lines 916–968): remove the `LocationIDs`/`LocationExternalKeys` blocks (lines 930–946). The `q` clause and others reference only `a.*`/`tags` — unaffected.
- [ ] **Step 11: `CreateAssetWithTags`** (lines 485–552): change the SQL to `create_asset_with_tags($1..$9)`; drop the `(*int)(nil)` arg; update the TRA-734 comment.
- [ ] **Step 12: `parseAssetWithTagsError`** (lines 995–1013): delete the `current_location_id_fkey` branch (lines 1008–1010).

### Task 4: Rewrite the location-delete guard

**Files:** Modify `backend/internal/storage/locations.go`

- [ ] **Step 1:** Replace the `CountActiveAssetsAtLocation` query (lines 444–449) with:

```go
func (s *Storage) CountActiveAssetsAtLocation(ctx context.Context, orgID, locationID int) (int, error) {
	// TRA-799: count non-deleted assets whose LATEST scan places them at this
	// location. Replaces the dropped assets.current_location_id column. This
	// also restores BB22 F2 intent — since TRA-734 nulled current_location_id
	// for new assets, the column-based guard fired for zero modern assets.
	query := `
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id) s.asset_id, s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT COUNT(*)
		FROM trakrf.assets a
		JOIN latest_scans ls ON ls.asset_id = a.id
		WHERE a.org_id = $1 AND ls.location_id = $2 AND a.deleted_at IS NULL
	`
	var count int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, locationID).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count assets at location: %w", err)
	}
	return count, nil
}
```

Update the doc comment above it (lines 440–443) to say "latest scan location" instead of "placed directly at".

---

## Phase 4 — Backend handlers

### Task 5: Asset handler — drop location request handling

**Files:** Modify `backend/internal/handlers/assets/assets.go`

- [ ] **Step 1: `assetLocationReadOnlyMessage`** (line 54): update — remove the now-false `GET /api/v1/assets/{id}` mention. New value:

```go
const assetLocationReadOnlyMessage = "asset location is collected through scan event ingestion (fixed-reader MQTT pipeline or handheld UI submission) and is not part of the asset resource. Read current asset location through GET /api/v1/reports/asset-locations or GET /api/v1/assets/{id}/history."
```

- [ ] **Step 2: `PublicRejectCreateFields`** (lines 62–65): keep both `location_id` and `location_external_key` entries unchanged (still a friendly pre-decode `read_only` rejection on POST).
- [ ] **Step 3: `PublicRejectPatchFields`** in `models/asset/asset.go` (line 143): populate it so PATCH pre-decode-rejects the same two fields symmetrically:

```go
var PublicRejectPatchFields = map[string]httputil.FieldRejectPolicy{
	"location_id":           {Code: "read_only", Message: assetLocationReadOnlyMessage},
	"location_external_key": {Code: "read_only", Message: assetLocationReadOnlyMessage},
}
```

Note: `assetLocationReadOnlyMessage` lives in the handler package. Either move the const to `models/asset` or define the message string in `asset.go`. **Decision:** define the canonical message string in `models/asset/asset.go` (exported `LocationReadOnlyMessage`) and have the handler const reference it, so both reject maps share one source of truth.

- [ ] **Step 4: PATCH echo blocks** (lines 431–461): delete the `location_id` and `location_external_key` `if present` blocks entirely. They are now pre-decode-rejected by `PublicRejectPatchFields`, so a present field never reaches the echo stage.
- [ ] **Step 5: PATCH post-echo cleanup** (lines 473–480): delete `request.LocationID = nil`, `request.LocationExternalKey = nil`, and the `delete(presentKeys, "location_id"/"location_external_key")` + `delete(explicitNulls, ...)` lines. Keep the `external_key` equivalents.
- [ ] **Step 6: `ListAssets` filter allowlist** (line 733): remove `"location_id"` and `"location_external_key"` from `Filters`.
- [ ] **Step 7: `ListAssets` mutual-exclusivity block** (lines 747–757): delete the `hasLocID && hasLocExt` block.
- [ ] **Step 8: `ListAssets` location validation** (lines 766–769 area): delete the `ValidateExternalKeyFilterValues("location_external_key", ...)` call.
- [ ] **Step 9: `ListAssets` ListFilter build** (lines 770–791 area): remove `LocationExternalKeys` from the `asset.ListFilter` literal and delete the entire `location_id` parsing loop.
- [ ] **Step 10: `GetAssetByID` handler callers** (lines ~1005, ~1060): confirm neither reads `.LocationID`; no change expected (verified — only `current.LocationID` in the deleted echo block referenced it).

---

## Phase 5 — Backend API spec metadata

### Task 6: Update `apispec/postprocess.go`

**Files:** Modify `backend/internal/tools/apispec/postprocess.go`

- [ ] **Step 1:** Line 1577 — remove `"location_id", "location_external_key"` from the `asset.PublicAssetView` entry (keep `description`, `valid_to`, `deleted_at`). Leave the `report.*` entries (1579, 1583) untouched.
- [ ] **Step 2:** Line 1609 — change `"asset.UpdateAssetRequest": {"description", "location_id", "valid_to"}` to `{"description", "valid_to"}`.
- [ ] **Step 3:** Lines 1598–1606 — update the comment block that documents `location_id` on `UpdateAssetRequest` to reflect that location is no longer on the request shape at all.
- [ ] **Step 4:** Line 1648 — remove `"location_id", "location_external_key"` from the `asset.PublicAssetView` property list.
- [ ] **Step 5:** Line 1725 — remove `"location_id", "location_external_key"` from the `asset.PublicAssetView` read-only list (keep `id, created_at, updated_at, deleted_at, tags`).
- [ ] **Step 6:** Inspect line 1130's array-typed-id-filter handling and the query-parameter section — remove any asset-list `location_id`/`location_external_key` query-parameter postprocessing. Leave location query params on other endpoints (`/reports/asset-locations`, `/locations`) untouched.
- [ ] **Step 7: Build the backend.** Run: `just backend build`. Expected: compiles clean. Fix any remaining `LocationID`/`LocationExternalKey`/`AssetWithLocation` references the compiler flags.
- [ ] **Step 8: Commit** — `git commit -am "feat(api): remove current_location from asset request/response shapes (TRA-799)"`

---

## Phase 6 — Backend tests

### Task 7: Delete obsolete tests, update the rest

**Files:** see below.

- [ ] **Step 1: Delete** `backend/internal/handlers/assets/current_location_consistency_integration_test.go` (tests COALESCE consistency of a removed field).
- [ ] **Step 2: Delete** `backend/internal/handlers/assets/patch_natural_key_integration_test.go` location tests. If the file also covers `external_key` echo, keep those and delete only the `Location*` tests + the `current_location_id` seeding; otherwise delete the file. Inspect first.
- [ ] **Step 3: Delete** `backend/internal/handlers/assets/error_detail_url_integration_test.go` location cases — inspect; if the file is entirely the 4 `Location*` error-detail tests, delete it; if it covers other fields, delete only the location cases.
- [ ] **Step 4: Update** `delete_conflict_integration_test.go` — change `seedAssetAtLocation` to insert an `asset_scans` row (timestamp, org_id, asset_id, location_id) instead of setting `current_location_id`. The 409-on-placed-assets and soft-deleted-asset tests then exercise the rewritten guard. A soft-deleted asset with a scan must still not block (the guard's `a.deleted_at IS NULL`).
- [ ] **Step 5: Update** `fk_validation_integration_test.go` — keep the POST `location_id`/`location_external_key` `read_only` rejection tests (behavior unchanged). `TestPatchAsset_DifferingLocationID_Rejected400` now rejects pre-decode for *any* value — adjust the assertion if it relied on echo-specific wording (code stays `read_only`). Delete `TestListAssets_BothLocationForms_Rejected400` (filter removed).
- [ ] **Step 6: Update** `list_external_key_integration_test.go` — delete `TestListAssets_LocationExternalKey_SlashRejected400` (filter removed).
- [ ] **Step 7: Update** `patch_round_trip_integration_test.go` — remove `location_external_key`/`location_id` from the expected read-only round-trip field set; a body still echoing them now 400s `read_only` (pre-decode) — adjust or drop those assertions.
- [ ] **Step 8: Update** `apispec/postprocess_test.go` — remove asset `location_id`/`location_external_key` assertions (read-only list, property list, schema presence). Keep `report.*` assertions.
- [ ] **Step 9: Update** `backend/database/seeds/contract_test_seed.sql` — remove `current_location_id` from the assets INSERT column list (line ~71).
- [ ] **Step 10: Grep sweep** — `grep -rn "current_location\|CurrentLocation\|LocationID\|LocationExternalKey" backend --include=*_test.go` and fix every remaining reference (seeds, assertions, helpers).
- [ ] **Step 11: Run backend tests.** Run: `just backend test`. Expected: PASS (migrations apply, including 000043; guard test green).
- [ ] **Step 12: Regenerate the OpenAPI spec.** Run: `just backend api-spec`. Verify `openapi.public.yaml` no longer lists `location_id`/`location_external_key` on the asset schemas or `?location_id` on `GET /assets`.
- [ ] **Step 13: Commit** — `git commit -am "test(assets): update test suite for current_location removal (TRA-799)"`

---

## Phase 7 — Frontend types & API

### Task 8: Strip location from asset types

**Files:** Modify `frontend/src/types/assets/index.ts`

- [ ] **Step 1:** `Asset` — remove `location_id` and `location_external_key` (lines 21–22).
- [ ] **Step 2:** `CreateAssetRequest` — remove `location_id`/`location_external_key` (lines 43–44).
- [ ] **Step 3:** `UpdateAssetRequest` — remove `location_id`/`location_external_key` (lines 61–62).
- [ ] **Step 4:** The asset-filter state type (line ~152, `location_id?: number | null | 'all'`) — remove it.

---

## Phase 8 — Frontend location re-sourcing

### Task 9: `useAssetLocations` hook

**Files:** Create `frontend/src/hooks/reports/useAssetLocations.ts`

- [ ] **Step 1:** Implement a hook that wraps the existing `useCurrentLocations({ fetchAll: true })` and exposes an asset-id-keyed map:

```ts
import { useMemo } from 'react';
import { useCurrentLocations } from './useCurrentLocations';
import type { CurrentLocationItem } from '@/types/reports';

/**
 * Current location for every asset in the org, keyed by asset id.
 *
 * TRA-799: asset location is fact data and is no longer on the asset
 * resource. The assets screen sources it from GET /reports/asset-locations.
 * React Query dedupes the underlying fetch across all consumers.
 */
export function useAssetLocations() {
  const { data, isLoading, error } = useCurrentLocations({ fetchAll: true });
  const byAssetId = useMemo(() => {
    const m = new Map<number, CurrentLocationItem>();
    for (const item of data) {
      if (item.asset_id != null) m.set(item.asset_id, item);
    }
    return m;
  }, [data]);
  return { byAssetId, isLoading, error };
}
```

### Task 10: Wire the display components

**Files:** Modify `AssetCard.tsx`, `AssetTable.tsx`, `AssetDetailsModal.tsx`

For each component, replace `asset.location_external_key` lookups with the report-sourced value. Pattern (AssetCard):

```ts
const { byAssetId } = useAssetLocations();
const locExtKey = byAssetId.get(asset.id)?.location_external_key ?? null;
const locationData = locExtKey ? getLocationByIdentifier(locExtKey) : null;
const locationName = locationData?.name ?? locExtKey ?? undefined;
```

- [ ] **Step 1:** `AssetCard.tsx` — apply the pattern; keep the existing render at lines 117–120 / 227–231.
- [ ] **Step 2:** `AssetDetailsModal.tsx` — apply the pattern; keep the "Location" field render.
- [ ] **Step 3:** `AssetTable.tsx` — the table renders a Location column; source each row's location via `useAssetLocations()` (call once in the table, look up per row).
- [ ] **Step 4:** `AssetsScreen.tsx` — confirm React Query dedupes; no orchestration needed beyond the hook being called by children. (Optionally hoist `useAssetLocations()` here and pass down — only if prop-drilling is already the screen's pattern.)

### Task 11: CSV export

**Files:** Modify `frontend/src/utils/export/assetExport.ts` and its caller.

- [ ] **Step 1:** Read `assetExport.ts` and identify its invocation site. Change `getLocationName(asset.location_external_key)` to take the location from an asset-id→`CurrentLocationItem` map passed in by the caller.
- [ ] **Step 2:** The caller (an assets-screen component) calls `useAssetLocations()` and passes `byAssetId` into the export function. `fetchAll` already loads every asset's location, so the full export set is covered.

---

## Phase 9 — Frontend form & filter removal

### Task 12: Remove the location selector from `AssetForm`

**Files:** Modify `frontend/src/components/assets/AssetForm.tsx`, `AssetFormModal.tsx`

- [ ] **Step 1:** Remove the `<select id="location">` block (lines 363–379) and its `<label>`.
- [ ] **Step 2:** Remove `location_id` from `formData` initial state (line 36), the edit-init (lines 91–92), and the submit payload (line 270).
- [ ] **Step 3:** Remove `resolvedLocationId` (lines 26–29), the `useLocations({ enabled: true })` call if only used for the dropdown (line 46), `locationCache`/`locations` (lines 47–48), and the `useLocationStore` import if now unused. Keep imports still used elsewhere in the file.
- [ ] **Step 4:** `AssetFormModal.tsx` — verify it only passes data through; no change expected.

### Task 13: Remove the assets-screen location filter

**Files:** `AssetFilters.tsx`, `AssetSearchSort.tsx`, `AssetsScreen.tsx`, `stores/assets/assetStore.ts`, `stores/assets/assetActions.ts`

- [ ] **Step 1:** `assetStore.ts` — remove `location_id` from the default `filters` (line 93).
- [ ] **Step 2:** `assetActions.ts` — remove `location_id: 'all'` from the filter reset (line 139).
- [ ] **Step 3:** `AssetSearchSort.tsx` — remove the location-filter `<select>` and `locationFilter` state (lines 22, 94–98).
- [ ] **Step 4:** `AssetsScreen.tsx` — remove `location_id` from the active-filter check (line 58) and the reset (line 86).
- [ ] **Step 5:** `AssetFilters.tsx` — remove any location filter UI present.
- [ ] **Step 6:** Confirm the assets list API call no longer sends a `location_id` query param.

---

## Phase 10 — Frontend tests & validation

### Task 14: Update frontend tests

- [ ] **Step 1:** `lookup.test.ts` — remove `location_id` from Asset mock objects (lines 29, 65, 179, 236, 254).
- [ ] **Step 2:** `AssetTable.test.tsx` — remove `location_id` from filter mocks (lines 67, 118, 151, 193); add `useAssetLocations`/`useCurrentLocations` mock so the Location column renders.
- [ ] **Step 3:** `AssetSearchSort.test.tsx` — remove `location_id` from filter mocks (lines 28, 94).
- [ ] **Step 4:** `assetExport.test.ts` — remove `location_external_key` from test assets; pass the asset-location map per the new export signature.
- [ ] **Step 5:** Grep sweep — `grep -rn "location_id\|location_external_key" frontend/src --include=*.test.ts*` and fix remaining references.
- [ ] **Step 6: Run** `just frontend lint` and `just frontend test`. Expected: PASS.
- [ ] **Step 7: Commit** — `git commit -am "feat(frontend): re-source asset location from reports endpoint; remove location form/filter (TRA-799)"`

---

## Phase 11 — Full validation, docs, PR

### Task 15: Validate & ship

- [ ] **Step 1: Run** `just validate` (both workspaces). Expected: all green.
- [ ] **Step 2:** Audit-sweep check — grep the whole repo for stragglers: `grep -rn "current_location" backend frontend --include=*.go --include=*.ts --include=*.tsx --include=*.sql` (excluding the `report`/`current-locations` reporting names, which are correct and stay).
- [ ] **Step 3:** Dimension/fact audit — confirm `PublicLocationView` and other dimension resources carry no fact columns; record the finding in the PR description.
- [ ] **Step 4:** Push the branch; open the PR (`feat/tra-799-current-location-read-only`), body summarizing the expanded scope, the four locked decisions, and the audit result.
- [ ] **Step 5:** Add a Linear comment to TRA-799 with the required docs changes (for a separate docs session): `current_location_*` removed from the asset resource; document current location as read via `/reports/asset-locations` and `/assets/{id}/history`; `?location` filter removed from `GET /assets`. Note the scope expansion vs. the original ticket text.
- [ ] **Step 6:** Leave TRA-799 open (docs work pending) per project convention.

---

## Self-Review

**Spec coverage:** migration (T1) ✓; read-query simplification incl. the queries the ticket undercounted (T3) ✓; guard rewrite (T4) ✓; request-struct removal — option 2 (T2,T5) ✓; **response** removal — the expansion (T2,T6) ✓; `?location` filter drop (T3,T5) ✓; frontend selector removal (T12) ✓; frontend re-sourcing (T9–T11) ✓; spec regen (T6.12) ✓; docs comment (T15.5) ✓; co-deploy single PR (T15.4) ✓.

**Type consistency:** `AssetWithLocation` becomes a `= AssetView` alias (T2.3), so storage methods returning `*asset.AssetWithLocation` and `ToPublicAssetView` callers stay compiling; `ToPublicAssetView` takes `AssetView` (T2.6) — consistent with the alias. `getAssetWithLocationByID` → `getAssetViewWithTagsByID` renamed with all call sites (T3.6). `LocationReadOnlyMessage` is the single source of truth shared by both reject maps (T5.3).

**Known investigate-then-edit steps** (acceptable — depend on file contents not yet read): T7.2/T7.3 (whether those test files are location-only), T11.1 (export caller site), T13.5 (AssetFilters location UI presence). Each step says what to inspect and the rule to apply.
