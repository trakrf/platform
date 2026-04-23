# TRA-465: Fix `/assets?location=X` Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `GET /api/v1/assets?location={identifier}` return assets whose most recent scan is at that location, matching the `/locations/current` semantics, with an integration test that guards the regression.

**Architecture:** Replace the stale `assets.current_location_id` join in `ListAssetsFiltered` and `CountAssetsFiltered` with a `LEFT JOIN` through a `latest_scans` CTE (`DISTINCT ON (asset_id) ... ORDER BY timestamp DESC` over `asset_scans`), mirroring the pattern already working in `storage/reports.go`. Both the surrogate `current_location_id` and the hydrated `current_location` returned by the endpoint are sourced from the CTE so filter and response cannot disagree.

**Tech Stack:** Go 1.x, `pgx/v5`, PostgreSQL + TimescaleDB, `chi` router, `testify`. Tests run with build tag `integration` against a real DB.

**Spec:** `docs/superpowers/specs/2026-04-23-tra-465-assets-location-filter-design.md`

---

## Pre-flight

- [ ] **Confirm branch.** The brainstorming step already created `miks2u/tra-465-assetslocationx-filter-returns-empty-despite-assets-present` and committed the spec there.

  Run: `git branch --show-current`
  Expected: `miks2u/tra-465-assetslocationx-filter-returns-empty-despite-assets-present`

- [ ] **Confirm DB is reachable for integration tests.**

  Run: `just backend test-integration -run TestCreateAsset ./internal/handlers/assets/...`
  Expected: PASS for existing `TestCreateAsset` (this proves the test harness + migrations + DB are ready).

---

## Task 1: Add test helpers for locations and scans in the assets handler tests

**Files:**
- Modify: `backend/internal/handlers/assets/assets_integration_test.go` (add private helpers alongside existing ones)

Reports tests already have local `createTestLocation` and `createTestScan` helpers (see `backend/internal/handlers/reports/asset_history_integration_test.go:70–97`). Mirror that pattern here — no shared testutil change needed.

- [ ] **Step 1: Add the two helpers near the top of the file (after the `withOrgContext` helper).**

```go
// createTestLocation inserts a location and returns its surrogate ID.
// identifier is LOC-<name>, matching the reports test helper pattern.
func createTestLocation(t *testing.T, pool *pgxpool.Pool, orgID int, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, identifier, name, is_active)
		VALUES ($1, $2, $3, true)
		RETURNING id
	`, orgID, "LOC-"+name, name).Scan(&id)
	require.NoError(t, err)
	return id
}

// createTestScan inserts an asset_scan row at the given timestamp.
func createTestScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, locationID *int, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (org_id, asset_id, location_id, timestamp)
		VALUES ($1, $2, $3, $4)
	`, orgID, assetID, locationID, ts)
	require.NoError(t, err)
}
```

- [ ] **Step 2: Verify the file still compiles (no test execution yet).**

  Run: `just backend build`
  Expected: success — the helpers are unused until Task 2 but the package still compiles.

- [ ] **Step 3: Commit.**

```bash
git add backend/internal/handlers/assets/assets_integration_test.go
git commit -m "test(tra-465): add location + scan test helpers in assets integration tests"
```

---

## Task 2: Write the failing regression test (the stale-column guard)

**Files:**
- Modify: `backend/internal/handlers/assets/assets_integration_test.go`

This is the test that would have caught TRA-465. It sets an asset's `current_location_id` to WHS-01 but creates its latest scan at WHS-02, then asserts the filter follows the scan, not the stale column.

- [ ] **Step 1: Add the test at the end of the file.**

```go
// TRA-465 regression: /assets?location filter must follow the asset's latest scan,
// not the denormalized assets.current_location_id column. The dead column is written
// only at create/update time; the real current location lives in asset_scans.
func TestListAssets_LocationFilter_FollowsLatestScanNotStaleColumn(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// Two locations.
	whs1 := createTestLocation(t, pool, orgID, "WHS-01")
	whs2 := createTestLocation(t, pool, orgID, "WHS-02")

	// Asset whose stale current_location_id points at WHS-01,
	// but whose latest scan is at WHS-02.
	a := testutil.NewAssetFactory(orgID).
		WithIdentifier("STALE-ASSET-001").
		WithName("Stale column asset").
		Build()
	a.CurrentLocationID = &whs1
	created, err := store.CreateAsset(context.Background(), a)
	require.NoError(t, err)

	now := time.Now().UTC()
	createTestScan(t, pool, orgID, created.ID, &whs1, now.Add(-2*time.Hour)) // older, at WHS-01
	createTestScan(t, pool, orgID, created.ID, &whs2, now.Add(-1*time.Hour)) // latest, at WHS-02

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	// ?location=LOC-WHS-01 must NOT return the asset (its latest scan is elsewhere).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	assert.Empty(t, data, "asset whose latest scan is at WHS-02 must not match ?location=LOC-WHS-01")

	// ?location=LOC-WHS-02 must return the asset.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-02", nil)
	req2 = withOrgContext(req2, orgID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusOK, w2.Code)
	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	data2, _ := resp2["data"].([]any)
	require.Len(t, data2, 1, "asset whose latest scan is at WHS-02 must match ?location=LOC-WHS-02")

	// Hydrated current_location must reflect the latest scan, not the stale column.
	item := data2[0].(map[string]any)
	assert.Equal(t, "LOC-WHS-02", item["current_location"])
}
```

- [ ] **Step 2: Run the test and confirm it FAILS in the expected way.**

  Run: `just backend test-integration -run TestListAssets_LocationFilter_FollowsLatestScanNotStaleColumn ./internal/handlers/assets/...`

  Expected: FAIL. The first assertion (`data` should be empty for `?location=LOC-WHS-01`) will fail because today the filter matches against `current_location_id` which still points at WHS-01. Confirm the failure message names that assertion — this proves the test actually exercises the bug path.

- [ ] **Step 3: Commit the failing test.**

```bash
git add backend/internal/handlers/assets/assets_integration_test.go
git commit -m "test(tra-465): add failing regression test for stale current_location_id"
```

---

## Task 3: Fix `ListAssetsFiltered` to source location from `latest_scans` CTE

**Files:**
- Modify: `backend/internal/storage/assets.go:693–767` (the `ListAssetsFiltered` function body)

- [ ] **Step 1: Replace the query construction in `ListAssetsFiltered`.**

  In `backend/internal/storage/assets.go`, find the `ListAssetsFiltered` function. Replace the `query := fmt.Sprintf(...)` block (currently lines 700–714) with the CTE version below. Everything else in the function (scan loop, identifier hydration) stays identical.

```go
	query := fmt.Sprintf(`
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT
			a.id, a.org_id, a.identifier, a.name, a.type, a.description,
			ls.location_id,
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.identifier
		FROM trakrf.assets a
		LEFT JOIN latest_scans ls ON ls.asset_id = a.id
		LEFT JOIN trakrf.locations l
			ON l.id = ls.location_id AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, where, orderBy, len(args)+1, len(args)+2)
```

  Notes on what changed vs. the old query:
  - Added `WITH latest_scans AS (...)` CTE.
  - Replaced `a.current_location_id` in the SELECT list with `ls.location_id` so the surrogate `CurrentLocationID` field in the returned `Asset` reflects the scan-derived value.
  - Replaced the direct `LEFT JOIN trakrf.locations l ON l.id = a.current_location_id ...` with `LEFT JOIN latest_scans ls ON ls.asset_id = a.id` + `LEFT JOIN trakrf.locations l ON l.id = ls.location_id ...`.
  - `buildAssetsWhere` is unchanged: it still emits `l.identifier = ANY($n::text[])`, which now references the scan-derived `l` join.

- [ ] **Step 2: Run the regression test. Expect it still to fail on the count assertion.**

  Run: `just backend test-integration -run TestListAssets_LocationFilter_FollowsLatestScanNotStaleColumn ./internal/handlers/assets/...`

  Expected: the `data` assertions pass, but the response may show the wrong `total_count` because `CountAssetsFiltered` still uses the old query. The test above doesn't check `total_count` directly, so it should actually pass. Confirm it does. If it fully passes, proceed to Task 4 anyway — `CountAssetsFiltered` must still be fixed for correctness (pagination).

- [ ] **Step 3: Commit.**

```bash
git add backend/internal/storage/assets.go
git commit -m "fix(tra-465): source /assets location filter from latest_scans CTE"
```

---

## Task 4: Fix `CountAssetsFiltered` to use the same CTE

**Files:**
- Modify: `backend/internal/storage/assets.go:770–780` (`CountAssetsFiltered`)

Pagination's `total_count` must agree with the rows returned. Same CTE shape, reduced to `SELECT COUNT(*)`.

- [ ] **Step 1: Replace the query in `CountAssetsFiltered`.**

  In `backend/internal/storage/assets.go`, find `CountAssetsFiltered`. Replace the `query := fmt.Sprintf(...)` block (currently lines 774–780) with:

```go
	query := fmt.Sprintf(`
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT COUNT(*)
		FROM trakrf.assets a
		LEFT JOIN latest_scans ls ON ls.asset_id = a.id
		LEFT JOIN trakrf.locations l
			ON l.id = ls.location_id AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE %s
	`, where)
```

- [ ] **Step 2: Re-run the regression test. Confirm it passes.**

  Run: `just backend test-integration -run TestListAssets_LocationFilter_FollowsLatestScanNotStaleColumn ./internal/handlers/assets/...`

  Expected: PASS.

- [ ] **Step 3: Commit.**

```bash
git add backend/internal/storage/assets.go
git commit -m "fix(tra-465): use latest_scans CTE in CountAssetsFiltered for consistent total_count"
```

---

## Task 5: Add the remaining three coverage tests

**Files:**
- Modify: `backend/internal/handlers/assets/assets_integration_test.go`

The regression test alone is the most valuable guard. These three round out the DoD: happy path (single value), OR semantics (multi-value), no-scans asset (NULL case).

- [ ] **Step 1: Add the three tests at the end of the file.**

```go
// TRA-465: single-value ?location= happy path.
func TestListAssets_LocationFilter_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	loc := createTestLocation(t, pool, orgID, "WHS-01")

	a := testutil.NewAssetFactory(orgID).WithIdentifier("HP-ASSET-001").Build()
	created, err := store.CreateAsset(context.Background(), a)
	require.NoError(t, err)
	createTestScan(t, pool, orgID, created.ID, &loc, time.Now().UTC().Add(-1*time.Hour))

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	require.Len(t, data, 1)
	assert.Equal(t, "HP-ASSET-001", data[0].(map[string]any)["identifier"])
	assert.Equal(t, "LOC-WHS-01", data[0].(map[string]any)["current_location"])
}

// TRA-465: multi-value ?location=A&location=B has OR semantics.
func TestListAssets_LocationFilter_MultiValueOR(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	whs1 := createTestLocation(t, pool, orgID, "WHS-01")
	whs2 := createTestLocation(t, pool, orgID, "WHS-02")
	whs3 := createTestLocation(t, pool, orgID, "WHS-03")

	a1 := testutil.NewAssetFactory(orgID).WithIdentifier("OR-A-001").Build()
	c1, err := store.CreateAsset(context.Background(), a1)
	require.NoError(t, err)
	a2 := testutil.NewAssetFactory(orgID).WithIdentifier("OR-A-002").Build()
	c2, err := store.CreateAsset(context.Background(), a2)
	require.NoError(t, err)
	a3 := testutil.NewAssetFactory(orgID).WithIdentifier("OR-A-003").Build()
	c3, err := store.CreateAsset(context.Background(), a3)
	require.NoError(t, err)

	now := time.Now().UTC()
	createTestScan(t, pool, orgID, c1.ID, &whs1, now.Add(-1*time.Hour))
	createTestScan(t, pool, orgID, c2.ID, &whs2, now.Add(-1*time.Hour))
	createTestScan(t, pool, orgID, c3.ID, &whs3, now.Add(-1*time.Hour))

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01&location=LOC-WHS-02", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	require.Len(t, data, 2, "expected OR of WHS-01 and WHS-02 to include both assets but not the one at WHS-03")

	got := map[string]bool{}
	for _, row := range data {
		got[row.(map[string]any)["identifier"].(string)] = true
	}
	assert.True(t, got["OR-A-001"])
	assert.True(t, got["OR-A-002"])
	assert.False(t, got["OR-A-003"])
}

// TRA-465: an asset with no scans is excluded from every ?location=X filter.
func TestListAssets_LocationFilter_ExcludesAssetsWithNoScans(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	createTestLocation(t, pool, orgID, "WHS-01") // location exists, but nothing scanned here

	a := testutil.NewAssetFactory(orgID).WithIdentifier("NO-SCAN-001").Build()
	_, err := store.CreateAsset(context.Background(), a)
	require.NoError(t, err)
	// Intentionally: no scan inserted.

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	assert.Empty(t, data, "asset with no scans must not match any location filter")

	// Sanity: unfiltered list should include the asset with current_location = null.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req2 = withOrgContext(req2, orgID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	data2, _ := resp2["data"].([]any)
	require.Len(t, data2, 1)
	assert.Nil(t, data2[0].(map[string]any)["current_location"])
}
```

- [ ] **Step 2: Run all four TRA-465 tests. All must pass.**

  Run: `just backend test-integration -run 'TestListAssets_LocationFilter_' ./internal/handlers/assets/...`

  Expected: 4 tests, all PASS.

- [ ] **Step 3: Commit.**

```bash
git add backend/internal/handlers/assets/assets_integration_test.go
git commit -m "test(tra-465): add happy-path, OR-semantics, and no-scans coverage for /assets?location"
```

---

## Task 6: Full validation pass

- [ ] **Step 1: Run the full assets integration suite.** Catches any collateral regression from the query rewrite.

  Run: `just backend test-integration ./internal/handlers/assets/...`
  Expected: all tests PASS.

- [ ] **Step 2: Run the full storage integration suite.** The query changes are in storage; make sure no storage-level test broke.

  Run: `just backend test-integration ./internal/storage/...`
  Expected: all tests PASS.

- [ ] **Step 3: Run unit tests + lint.**

  Run: `just validate`
  Expected: lint clean, all unit tests PASS.

- [ ] **Step 4: If anything failed in steps 1–3, stop and debug. Do not push a broken branch.**

---

## Task 7: Push branch and open PR

- [ ] **Step 1: Push the branch.**

```bash
git push -u origin miks2u/tra-465-assetslocationx-filter-returns-empty-despite-assets-present
```

- [ ] **Step 2: Open the PR.**

```bash
gh pr create --title "fix(tra-465): /assets?location filter follows latest scan, not stale column" --body "$(cat <<'EOF'
## Summary
- `GET /api/v1/assets?location={identifier}` now returns assets whose **most recent scan** is at that location, matching the `/locations/current` semantics that iPaaS connectors (e.g. TeamCentral) rely on.
- Root cause was that `assets.current_location_id` is a dead denormalized column — written only at create/update and never synced from scans. The filter ran correctly against a source of truth that wasn't true.
- Both the surrogate `current_location_id` and the hydrated `current_location` in the response are now sourced from the same `latest_scans` CTE as the filter, so they cannot disagree.

## Behavior change for callers
Anyone who was reading `current_location` or `current_location_id` from `/api/v1/assets` was previously seeing the stale create-time value. They now see scan-derived current location. This is a correctness fix, but it is observable.

## Deferred follow-up (post-v1, not a TeamCentral launch blocker)
The `latest_scans` CTE is fine at MVP scan volume. A separate ticket (to be filed) will evaluate two Timescale-native replacements and remove the dead column + its write paths:
- Continuous aggregate over `asset_scans` materializing latest scan per asset, with a refresh policy.
- `last()` hyperfunction query — already wired for reports behind `QueryEngineTimescaleLast` in `storage/reports.go`, just needs to be extended to the assets query engine.

## Docs
Customer-facing docs for this filter live in the `trakrf-docs` repo. A small follow-up PR there (to be opened **after** this merges) clarifies that the `location` filter value is the location identifier (natural key). Per the "docs behind backend reality" convention.

## Test plan
- [ ] `just backend test-integration -run 'TestListAssets_LocationFilter_' ./internal/handlers/assets/...` — the four new tests (regression guard, happy path, OR semantics, no-scans exclusion).
- [ ] `just backend test-integration ./internal/handlers/assets/...` — no regressions in surrounding tests.
- [ ] `just backend test-integration ./internal/storage/...` — query changes don't break other storage-level tests.
- [ ] `just validate` — lint + unit tests clean.
- [ ] Preview deploy: `curl -H "Authorization: Bearer $KEY" 'https://app.preview.trakrf.id/api/v1/assets?location=WHS-01'` returns the expected assets and agrees with `/locations/current?location=WHS-01`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Report PR URL back to the user.**

---

## Self-review done by the plan author

- **Spec coverage.** Every goal from the spec has a task: filter correctness (Task 3), count correctness (Task 4), consistent hydration (Task 3 — surrogate `ls.location_id` + identifier from joined `l`), OR semantics (Task 5 test), regression guard (Task 2 test), docs deferred appropriately (called out in PR body, no platform-repo change).
- **Placeholder scan.** No TBDs, no "similar to above," no "add appropriate error handling." Every code block is complete and copy-pasteable.
- **Type consistency.** `createTestLocation` returns `int`, used as `*int` via `&loc`. `createTestScan` signature matches `(orgID, assetID int, locationID *int, ts time.Time)` in both definition and all call sites. `testutil.NewAssetFactory(orgID).WithIdentifier(...).Build()` returns `asset.Asset` (verified in `testutil/factories.go:64`). `store.CreateAsset` returns `(*asset.Asset, error)` (verified in `storage/assets.go:16`) — the tests use `created.ID` accordingly.
