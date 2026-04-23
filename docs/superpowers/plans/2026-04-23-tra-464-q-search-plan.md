# TRA-464 — `q` Search: Substring Docs Fix + Identifier Match — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the `q` query parameter on `/assets`, `/locations`, and `/locations/current` to match active, non-deleted `identifiers.value`, and correct the swagger docs that call `q` "fuzzy" when it is case-insensitive substring.

**Architecture:** Mirror the existing `EXISTS (SELECT 1 FROM trakrf.identifiers ...)` pattern from `backend/internal/storage/reports.go` into `assets.go` and `locations.go`. Add `AND deleted_at IS NULL` on all three endpoints. Reword swagger comments in the three handlers; regenerate `docs/api/openapi.public.yaml` via `just backend api-spec`.

**Tech Stack:** Go, pgx, Postgres/Timescale, swag (swaggo) for OpenAPI generation, `stretchr/testify` for tests. Integration tests use `//go:build integration` and `testutil.SetupTestDB`.

**Spec:** `docs/superpowers/specs/2026-04-23-tra-464-q-search-design.md`

**Branch:** `miks2u/tra-464-fix-q-search-docs-substring-not-fuzzy-and-add-identifier`

---

## File Inventory

Modify:
- `backend/internal/storage/assets.go` (lines 808–813, `q` subclause in `buildAssetsWhere`)
- `backend/internal/storage/locations.go` (lines 791–796, `q` subclause in `buildLocationsWhere`)
- `backend/internal/storage/reports.go` (lines 153–157 DISTINCT ON variant, 186–190 Timescale variant)
- `backend/internal/storage/assets_integration_test.go` (extend `TestListAssetsFiltered_Q`, add identifier test)
- `backend/internal/storage/locations_integration_test.go` (add `TestListLocationsFiltered_Q`)
- `backend/internal/handlers/assets/assets.go` (lines 327, 337)
- `backend/internal/handlers/locations/locations.go` (line 325)
- `backend/internal/handlers/reports/current_locations.go` (line 36)
- `docs/api/openapi.public.yaml` (regenerated)
- `backend/internal/handlers/swaggerspec/openapi.public.json` / `openapi.public.yaml` (regenerated)

Create:
- `backend/internal/storage/reports_integration_test.go` (new file — no integration tests exist for `/locations/current` today)

---

## Task 1: Extend `/assets` `q` to match active identifier values

**Files:**
- Modify: `backend/internal/storage/assets.go:808-813`
- Test: `backend/internal/storage/assets_integration_test.go` (add new test after line 167)

### - [ ] Step 1.1: Write the failing test

Add this test function to `backend/internal/storage/assets_integration_test.go` immediately after `TestListAssetsFiltered_Q` (ends at line 167). The test covers three distinct behaviors: positive identifier match, `is_active = false` exclusion, and `deleted_at IS NOT NULL` exclusion.

```go
func TestListAssetsFiltered_QMatchesActiveIdentifier(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	activeAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-active", Name: "Active", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	inactiveIDAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-inactive-id", Name: "InactiveID", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	deletedIDAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-deleted-id", Name: "DeletedID", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Active identifier — should be found by q.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'EPC-ACTIVE-10023', $2, NOW(), true)
	`, orgID, activeAsset.ID)
	require.NoError(t, err)

	// Inactive identifier — should NOT be found by q.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'EPC-INACTIVE-10023', $2, NOW(), false)
	`, orgID, inactiveIDAsset.ID)
	require.NoError(t, err)

	// Soft-deleted identifier — should NOT be found by q.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, valid_from, is_active, deleted_at)
		VALUES ($1, 'rfid', 'EPC-DELETED-10023', $2, NOW(), true, NOW())
	`, orgID, deletedIDAsset.ID)
	require.NoError(t, err)

	t.Run("active identifier matches", func(t *testing.T) {
		q := "10023"
		items, err := store.ListAssetsFiltered(context.Background(), orgID, asset.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "asset-active", items[0].Identifier)
	})

	t.Run("inactive identifier does not match", func(t *testing.T) {
		q := "INACTIVE-10023"
		items, err := store.ListAssetsFiltered(context.Background(), orgID, asset.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("soft-deleted identifier does not match", func(t *testing.T) {
		q := "DELETED-10023"
		items, err := store.ListAssetsFiltered(context.Background(), orgID, asset.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})
}
```

### - [ ] Step 1.2: Run the test to verify it fails

Run:
```bash
just backend test-integration -run TestListAssetsFiltered_QMatchesActiveIdentifier
```
Expected: FAIL on the `active identifier matches` subtest with `len=0, want=1`. The other two subtests will pass for the wrong reason (no current match path exists) — that's fine; the red case is the positive match.

### - [ ] Step 1.3: Update the `q` clause in `buildAssetsWhere`

In `backend/internal/storage/assets.go`, find the block at lines 808–813:

```go
if f.Q != nil {
	args = append(args, "%"+*f.Q+"%")
	idx := len(args)
	clauses.append(fmt.Sprintf(
		"(a.name ILIKE $%d OR a.identifier ILIKE $%d OR a.description ILIKE $%d)",
		idx, idx, idx))
}
```

Replace with:

```go
if f.Q != nil {
	args = append(args, "%"+*f.Q+"%")
	idx := len(args)
	clauses.append(fmt.Sprintf(
		"(a.name ILIKE $%d OR a.identifier ILIKE $%d OR a.description ILIKE $%d "+
			"OR EXISTS (SELECT 1 FROM trakrf.identifiers i "+
			"WHERE i.asset_id = a.id AND i.is_active = true "+
			"AND i.deleted_at IS NULL AND i.value ILIKE $%d))",
		idx, idx, idx, idx))
}
```

Note: confirm via `grep -n "clauses.append" backend/internal/storage/assets.go` that the helper is `clauses.append` (lowercase `append` on a slice wrapper), not a Go built-in `append(clauses, ...)`. If the actual code uses `clauses = append(clauses, ...)`, keep that same pattern — do not change the slice-helper convention.

### - [ ] Step 1.4: Run the test to verify it passes

Run:
```bash
just backend test-integration -run TestListAssetsFiltered_QMatchesActiveIdentifier
```
Expected: all three subtests PASS.

### - [ ] Step 1.5: Run the existing `/assets` q test to confirm no regression

Run:
```bash
just backend test-integration -run TestListAssetsFiltered_Q
```
Expected: both `TestListAssetsFiltered_Q` and `TestListAssetsFiltered_QMatchesActiveIdentifier` PASS. The `-run` regex matches both.

### - [ ] Step 1.6: Commit

```bash
git add backend/internal/storage/assets.go backend/internal/storage/assets_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tra-464): extend /assets q to match active identifier values

q now also matches trakrf.identifiers.value via EXISTS subquery, filtered
by is_active = true AND deleted_at IS NULL. Mirrors the pattern already
in reports.go for /locations/current.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Extend `/locations` `q` to match active identifier values

**Files:**
- Modify: `backend/internal/storage/locations.go:791-796`
- Test: `backend/internal/storage/locations_integration_test.go` (add new test function)

### - [ ] Step 2.1: Write the failing test

There is currently no `q` test for locations. Add this new function to `backend/internal/storage/locations_integration_test.go`. It covers the basic name/identifier/description backfill plus the new identifier-value match plus the inactive/deleted exclusions.

```go
func TestListLocationsFiltered_Q(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	activeLoc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "loc-active", Name: "Warehouse Active", Path: "loc-active",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	inactiveIDLoc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "loc-inactive-id", Name: "InactiveID", Path: "loc-inactive-id",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	deletedIDLoc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "loc-deleted-id", Name: "DeletedID", Path: "loc-deleted-id",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, location_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'LOC-ACTIVE-20055', $2, NOW(), true)
	`, orgID, activeLoc.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, location_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'LOC-INACTIVE-20055', $2, NOW(), false)
	`, orgID, inactiveIDLoc.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, location_id, valid_from, is_active, deleted_at)
		VALUES ($1, 'rfid', 'LOC-DELETED-20055', $2, NOW(), true, NOW())
	`, orgID, deletedIDLoc.ID)
	require.NoError(t, err)

	t.Run("name substring matches", func(t *testing.T) {
		q := "Warehouse"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "loc-active", items[0].Identifier)
	})

	t.Run("active identifier value matches", func(t *testing.T) {
		q := "20055"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "loc-active", items[0].Identifier)
	})

	t.Run("inactive identifier value does not match", func(t *testing.T) {
		q := "INACTIVE-20055"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("soft-deleted identifier value does not match", func(t *testing.T) {
		q := "DELETED-20055"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})
}
```

### - [ ] Step 2.2: Run the test to verify it fails

Run:
```bash
just backend test-integration -run TestListLocationsFiltered_Q
```
Expected: `name substring matches` PASSES (existing behavior); `active identifier value matches` FAILS with `len=0, want=1`; the exclusion subtests will PASS vacuously until the identifier path exists.

### - [ ] Step 2.3: Update the `q` clause in `buildLocationsWhere`

In `backend/internal/storage/locations.go`, find the block at lines 791–796:

```go
if f.Q != nil {
	args = append(args, "%"+*f.Q+"%")
	idx := len(args)
	clauses.append(fmt.Sprintf(
		"(l.name ILIKE $%d OR l.identifier ILIKE $%d OR l.description ILIKE $%d)",
		idx, idx, idx))
}
```

Replace with:

```go
if f.Q != nil {
	args = append(args, "%"+*f.Q+"%")
	idx := len(args)
	clauses.append(fmt.Sprintf(
		"(l.name ILIKE $%d OR l.identifier ILIKE $%d OR l.description ILIKE $%d "+
			"OR EXISTS (SELECT 1 FROM trakrf.identifiers i "+
			"WHERE i.location_id = l.id AND i.is_active = true "+
			"AND i.deleted_at IS NULL AND i.value ILIKE $%d))",
		idx, idx, idx, idx))
}
```

Note the differences from Task 1:
- Alias is `l.` (locations) not `a.` (assets)
- Subquery joins on `i.location_id = l.id` not `i.asset_id = a.id`

If the actual slice pattern is `clauses = append(clauses, ...)`, match it (see Task 1 step 1.3 note).

### - [ ] Step 2.4: Run the test to verify it passes

Run:
```bash
just backend test-integration -run TestListLocationsFiltered_Q
```
Expected: all four subtests PASS.

### - [ ] Step 2.5: Run adjacent location tests to confirm no regression

Run:
```bash
just backend test-integration -run "TestListLocationsFiltered"
```
Expected: `TestListLocationsFiltered_Parent`, `TestListLocationsFiltered_Integration_IdentifiersNeverNil`, and the new `TestListLocationsFiltered_Q` all PASS.

### - [ ] Step 2.6: Commit

```bash
git add backend/internal/storage/locations.go backend/internal/storage/locations_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tra-464): extend /locations q to match active identifier values

q now also matches trakrf.identifiers.value via EXISTS subquery, filtered
by is_active = true AND deleted_at IS NULL. Backfills previously-missing
q coverage on the locations endpoint.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `deleted_at` guard to `/locations/current` identifier subquery

**Files:**
- Modify: `backend/internal/storage/reports.go:153-157` (DISTINCT ON variant) and `186-190` (Timescale variant)
- Create: `backend/internal/storage/reports_integration_test.go` (no integration test file exists for this package-level function today)

### - [ ] Step 3.1: Check which variant is active and confirm both locations

Run:
```bash
grep -n "identifiers ai" backend/internal/storage/reports.go
```
Expected: two hits, one around line 155, one around line 188 (DISTINCT ON and Timescale `last()` variants). Both get the same edit.

### - [ ] Step 3.2: Create the integration test file with a failing test

Create `backend/internal/storage/reports_integration_test.go` with this content. Check an existing integration test file (e.g. `assets_integration_test.go` lines 1–17) for the exact import block shape — copy the imports that are actually needed.

```go
//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestCurrentLocations_QMatchesActiveIdentifierOnly(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-current", Name: "Current WH", Path: "wh-current",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	activeAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-current-active", Name: "ActiveCur", Type: "asset",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	deletedIDAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-current-deleted", Name: "DeletedCur", Type: "asset",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'CUR-ACTIVE-30077', $2, NOW(), true)
	`, orgID, activeAsset.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, valid_from, is_active, deleted_at)
		VALUES ($1, 'rfid', 'CUR-DELETED-30077', $2, NOW(), true, NOW())
	`, orgID, deletedIDAsset.ID)
	require.NoError(t, err)

	t.Run("active identifier matches", func(t *testing.T) {
		q := "ACTIVE-30077"
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "asset-current-active", items[0].AssetIdentifier)
	})

	t.Run("soft-deleted identifier does not match", func(t *testing.T) {
		q := "DELETED-30077"
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})
}
```

Confirmed type references used above (pinned during plan authoring):
- Storage fn: `Storage.ListCurrentLocations` at `backend/internal/storage/reports.go:30`
- Filter: `report.CurrentLocationFilter` at `backend/internal/models/report/current_location.go:25`
- Item field: `CurrentLocationItem.AssetIdentifier` at `backend/internal/models/report/current_location.go:9`

Package is `report` (singular), not `reports`.

### - [ ] Step 3.3: Run the test to verify it fails

Run:
```bash
just backend test-integration -run TestCurrentLocations_QMatchesActiveIdentifierOnly
```
Expected: `active identifier matches` PASSES (existing behavior — the identifier subquery is already there); `soft-deleted identifier does not match` FAILS with a non-empty result, because the current subquery has `is_active = true` but no `deleted_at IS NULL`.

### - [ ] Step 3.4: Add the `deleted_at` guard to both variants in `reports.go`

In `backend/internal/storage/reports.go`, locate the two `EXISTS (SELECT 1 FROM trakrf.identifiers ai ...)` subqueries (one in each of the DISTINCT ON and Timescale branches). Each currently has:

```
AND ai.is_active = true AND ai.value ILIKE $3
```

Change each to:

```
AND ai.is_active = true AND ai.deleted_at IS NULL AND ai.value ILIKE $3
```

Keep the existing parameter placeholder (`$3` or whatever it is in context). Do not renumber. Apply the edit to **both** occurrences.

### - [ ] Step 3.5: Run the test to verify it passes

Run:
```bash
just backend test-integration -run TestCurrentLocations_QMatchesActiveIdentifierOnly
```
Expected: both subtests PASS.

### - [ ] Step 3.6: Run the broader reports + current_locations tests

Run:
```bash
just backend test-integration -run "TestCurrentLocation"
```
Expected: new test plus any existing tests (e.g. `TestCurrentLocationFilter_Struct` in the handlers package — though that's in `handlers/reports/`, not `storage/`) all PASS. If no other matching tests exist in `storage`, only the new one runs.

### - [ ] Step 3.7: Commit

```bash
git add backend/internal/storage/reports.go backend/internal/storage/reports_integration_test.go
git commit -m "$(cat <<'EOF'
fix(tra-464): exclude soft-deleted identifiers from /locations/current q

The existing EXISTS subquery filtered only on is_active=true. Adds
deleted_at IS NULL so soft-deleted identifiers are never returned.
Applied to both DISTINCT ON and Timescale query variants.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Reword swagger comments and regenerate OpenAPI

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go:327,337`
- Modify: `backend/internal/handlers/locations/locations.go:325`
- Modify: `backend/internal/handlers/reports/current_locations.go:36`
- Regenerated: `docs/api/openapi.public.{json,yaml}`, `backend/internal/handlers/swaggerspec/openapi.public.{json,yaml}`

### - [ ] Step 4.1: Reword `assets.go` swagger comments

In `backend/internal/handlers/assets/assets.go`, change line 327:

```go
// @Description Paginated assets list with natural-key filters, sort, and fuzzy search
```

to:

```go
// @Description Paginated assets list with natural-key filters, sort, and substring search
```

And line 337:

```go
// @Param q        query string false "fuzzy search on name / identifier / description"
```

to:

```go
// @Param q        query string false "substring search (case-insensitive) on name, identifier, description, and active identifier values"
```

### - [ ] Step 4.2: Reword `locations.go` swagger comment

In `backend/internal/handlers/locations/locations.go`, change line 325:

```go
// @Param q        query string false "fuzzy search on name, identifier, description"
```

to:

```go
// @Param q        query string false "substring search (case-insensitive) on name, identifier, description, and active identifier values"
```

### - [ ] Step 4.3: Reword `current_locations.go` swagger comment

In `backend/internal/handlers/reports/current_locations.go`, change line 36:

```go
// @Param q query string false "fuzzy search on asset name / identifier"
```

to:

```go
// @Param q query string false "substring search (case-insensitive) on asset name, identifier, and active identifier values"
```

### - [ ] Step 4.4: Regenerate the OpenAPI spec

Run:
```bash
just backend api-spec
```
Expected output includes `✅ Public spec: docs/api/openapi.public.{json,yaml}` and `✅ Internal spec: ...`. If the recipe fails because `swag` is not installed, run `go install github.com/swaggo/swag/cmd/swag@latest` first, then retry.

### - [ ] Step 4.5: Verify the regenerated yaml reflects the new wording

Run:
```bash
grep -nE "fuzzy|substring" docs/api/openapi.public.yaml
```
Expected: zero hits for `fuzzy`; one or more hits for `substring` wording. If `fuzzy` still appears anywhere in the regenerated yaml, open that line and trace it back to a swagger comment that was missed, then fix the comment and rerun `just backend api-spec`.

### - [ ] Step 4.6: Lint the regenerated spec

Run:
```bash
just backend api-lint
```
Expected: no new warnings compared to `main`. If there are pre-existing warnings, confirm the count hasn't changed; the goal is no regression introduced by this change.

### - [ ] Step 4.7: Commit

```bash
git add backend/internal/handlers/assets/assets.go \
	backend/internal/handlers/locations/locations.go \
	backend/internal/handlers/reports/current_locations.go \
	docs/api/openapi.public.json docs/api/openapi.public.yaml \
	backend/internal/handlers/swaggerspec/openapi.public.json \
	backend/internal/handlers/swaggerspec/openapi.public.yaml
git commit -m "$(cat <<'EOF'
docs(tra-464): reword q as substring (case-insensitive) + list fields

Swagger comments on /assets, /locations, /locations/current no longer
claim "fuzzy search". They now accurately describe case-insensitive
ILIKE behavior and list the searched fields, including active
identifier values (new in this ticket).

Regenerated docs/api/openapi.public.{json,yaml} and the embedded
swaggerspec copies.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Full validation, push, PR

### - [ ] Step 5.1: Run the full integration suite

Run:
```bash
just backend test-integration
```
Expected: all tests pass. If any unrelated test fails, investigate whether it's a pre-existing flake or caused by our change. Our change only adds conditions to existing `q` clauses — it should not affect any non-`q` code path.

### - [ ] Step 5.2: Run `just validate`

Run:
```bash
just validate
```
Expected: lint + unit tests + build + smoke test all green across backend and frontend.

### - [ ] Step 5.3: Push the branch

Run:
```bash
git push -u origin miks2u/tra-464-fix-q-search-docs-substring-not-fuzzy-and-add-identifier
```
Expected: branch publishes; preview deployment auto-triggers (see `.github/workflows/sync-preview.yml`).

### - [ ] Step 5.4: Open the PR

Run:
```bash
gh pr create --title "fix(tra-464): q search — substring docs + active identifier match" --body "$(cat <<'EOF'
## Summary
- `/assets` and `/locations` `q` parameter now matches active, non-deleted `identifiers.value` via EXISTS subquery (mirrors the pattern already in `/locations/current`).
- `/locations/current` `q` subquery tightened to also require `deleted_at IS NULL` alongside `is_active = true`.
- Swagger comments on all three endpoints reworded from "fuzzy search" to "substring search (case-insensitive)" and now list the searched fields.
- OpenAPI spec regenerated.

## Behavior change callout
Clients that previously saw `q=<identifier-value>` return empty on `/assets` and `/locations` will now get matches. This is a fix (the docs promised "fuzzy" but the implementation didn't even substring-match identifier values), not a regression.

## Test plan
- [x] New integration test `TestListAssetsFiltered_QMatchesActiveIdentifier` covers positive match + inactive + deleted exclusions.
- [x] New integration test `TestListLocationsFiltered_Q` backfills basic name/identifier/description coverage plus identifier-value match and exclusions.
- [x] New integration test `TestCurrentLocations_QMatchesActiveIdentifierOnly` validates the `deleted_at IS NULL` guard on `/locations/current`.
- [x] Existing `TestListAssetsFiltered_Q` continues to pass (no regression on name-only path).
- [x] `just validate` green.
- [ ] Black-box check against preview: `curl .../api/v1/assets?q=<known-tag-value>` returns the expected asset.

## Out of scope
- Actual fuzzy / trigram / Levenshtein search implementation.
- Refactor of the `is_active` vs `deleted_at` lifecycle semantics on identifiers.

Linear: https://linear.app/trakrf/issue/TRA-464

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
Expected: PR URL returned. Report the URL to the user.

---

## Self-Review Notes

- **Spec coverage**: every section of the spec maps to a task. Storage changes → Tasks 1–3. Docs changes → Task 4. Tests → included in each of Tasks 1–3. Definition-of-done items → Task 5 validation.
- **Parameter reuse**: all three SQL edits reuse the existing `%term%` bind (`$N`), consistent with the spec's "no extra param" statement.
- **Slice-helper note**: Step 1.3 and 2.3 both flag that `clauses.append` vs `clauses = append(clauses, ...)` needs to match the actual code — avoids a cosmetic merge-conflict.
- **Task 3 type references**: `Storage.ListCurrentLocations`, `report.CurrentLocationFilter`, and `CurrentLocationItem.AssetIdentifier` were pinned from the actual model/storage files during plan authoring — no guesses remain.
- **No "TBD"**: no unresolved placeholders. Every step has full code and commands.
