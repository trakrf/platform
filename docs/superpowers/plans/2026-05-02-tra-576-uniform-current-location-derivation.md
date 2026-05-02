# TRA-576 — Uniform `current_location_*` derivation across asset read paths

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `GET /api/v1/assets`, `GET /api/v1/assets/{id}`, and `GET /api/v1/assets/lookup` return identical `(current_location_id, current_location_external_key)` pairs — both populated together or both null — by aligning the SQL precedence and source expression in storage.

**Architecture:** All three read paths join `assets` against a CTE of latest scans + a LEFT JOIN to `locations`. Today the list path uses `COALESCE(ls.location_id, a.current_location_id)` (scan-first) and writes the coalesced int back to `CurrentLocationID`. Detail (`getAssetWithLocationByID`) and lookup (`GetAssetByExternalKey`) select bare `a.current_location_id` while joining via `COALESCE(a.current_location_id, ls.location_id)` — different precedence, plus the SELECT and JOIN read different sources. Fix is to align both functions to list-path semantics: scan-first precedence, both fields derived from the same expression.

**Tech Stack:** Go 1.22+, pgx/v5, Postgres, integration tests behind `//go:build integration`.

---

## File Structure

- Modify: `backend/internal/storage/assets.go` — `getAssetWithLocationByID` (lines 497–557) and `GetAssetByExternalKey` (lines 597–659)
- Create: `backend/internal/handlers/assets/current_location_consistency_integration_test.go` — new integration test asserting FK-pair invariant across list/detail/lookup

No other files require changes. `PublicAssetView` is correct, `ListAssetsFiltered` is correct, OpenAPI spec is correct.

---

### Task 1: Failing integration test for FK-pair invariant

**Files:**
- Create: `backend/internal/handlers/assets/current_location_consistency_integration_test.go`

- [ ] **Step 1: Write the failing test**

```go
//go:build integration
// +build integration

// TRA-576: GET /assets, /assets/{id}, /assets/lookup must return the same
// (current_location_id, current_location_external_key) pair — both
// populated or both null — for any given asset.

package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupConsistencyRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)
	r.Get("/api/v1/assets/lookup", handler.Lookup)
	r.Get("/api/v1/assets/{id}", handler.GetAsset)
	return r
}

func withConsistencyOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra576@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocation(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, valid_from, is_active)
		VALUES ($1, $2, $2, $3, true) RETURNING id
	`, orgID, externalKey, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedAssetWithFK(t *testing.T, pool *pgxpool.Pool, orgID int, extKey string, locID *int) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description,
		                            current_location_id, valid_from, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, extKey, locID, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID, locID int, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
		VALUES ($1, $2, $3, $4)
	`, ts, orgID, assetID, locID)
	require.NoError(t, err)
}

type fkPair struct {
	id          *int
	externalKey *string
}

func (p fkPair) String() string {
	idStr := "<nil>"
	if p.id != nil {
		idStr = fmt.Sprintf("%d", *p.id)
	}
	keyStr := "<nil>"
	if p.externalKey != nil {
		keyStr = *p.externalKey
	}
	return fmt.Sprintf("(id=%s, key=%s)", idStr, keyStr)
}

func fetchListPair(t *testing.T, router *chi.Mux, orgID, assetID int) fkPair {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?limit=100", nil)
	req = withConsistencyOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data []assetmodel.PublicAssetView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, v := range resp.Data {
		if v.ID == assetID {
			return fkPair{id: v.CurrentLocationID, externalKey: v.CurrentLocationExternalKey}
		}
	}
	t.Fatalf("asset id %d not found in list response", assetID)
	return fkPair{}
}

func fetchDetailPair(t *testing.T, router *chi.Mux, orgID, assetID int) fkPair {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", assetID), nil)
	req = withConsistencyOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data assetmodel.PublicAssetView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return fkPair{id: resp.Data.CurrentLocationID, externalKey: resp.Data.CurrentLocationExternalKey}
}

func fetchLookupPair(t *testing.T, router *chi.Mux, orgID int, externalKey string) fkPair {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/lookup?external_key="+externalKey, nil)
	req = withConsistencyOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data assetmodel.PublicAssetView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return fkPair{id: resp.Data.CurrentLocationID, externalKey: resp.Data.CurrentLocationExternalKey}
}

// BB15 reproduction: asset has no explicit FK but has a scan pointing at a
// location. List populates the FK pair from the scan; detail and lookup
// must do the same.
func TestCurrentLocation_ScanInferred_ConsistentAcrossReads(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := seedLocation(t, pool, orgID, "WHS-01")
	assetID := seedAssetWithFK(t, pool, orgID, "ASSET-0001", nil)
	seedScan(t, pool, orgID, assetID, locID, time.Now().UTC())

	handler := NewHandler(store)
	router := setupConsistencyRouter(handler)

	listPair := fetchListPair(t, router, orgID, assetID)
	detailPair := fetchDetailPair(t, router, orgID, assetID)
	lookupPair := fetchLookupPair(t, router, orgID, "ASSET-0001")

	require.NotNil(t, listPair.id, "list FK should be populated; got %s", listPair)
	require.NotNil(t, listPair.externalKey)
	assert.Equal(t, locID, *listPair.id)
	assert.Equal(t, "WHS-01", *listPair.externalKey)

	assert.Equal(t, listPair.String(), detailPair.String(), "detail FK pair must match list")
	assert.Equal(t, listPair.String(), lookupPair.String(), "lookup FK pair must match list")
}

// No FK and no scan: all three endpoints return (null, null).
func TestCurrentLocation_NoLocation_AllNullAcrossReads(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetID := seedAssetWithFK(t, pool, orgID, "ASSET-NOLOC", nil)

	handler := NewHandler(store)
	router := setupConsistencyRouter(handler)

	listPair := fetchListPair(t, router, orgID, assetID)
	detailPair := fetchDetailPair(t, router, orgID, assetID)
	lookupPair := fetchLookupPair(t, router, orgID, "ASSET-NOLOC")

	assert.Nil(t, listPair.id)
	assert.Nil(t, listPair.externalKey)
	assert.Nil(t, detailPair.id)
	assert.Nil(t, detailPair.externalKey)
	assert.Nil(t, lookupPair.id)
	assert.Nil(t, lookupPair.externalKey)
}

// FK set but no scan: TRA-495 fallback. All three return the FK location.
func TestCurrentLocation_FKOnlyFallback_ConsistentAcrossReads(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := seedLocation(t, pool, orgID, "WHS-FK")
	assetID := seedAssetWithFK(t, pool, orgID, "ASSET-FKONLY", &locID)

	handler := NewHandler(store)
	router := setupConsistencyRouter(handler)

	listPair := fetchListPair(t, router, orgID, assetID)
	detailPair := fetchDetailPair(t, router, orgID, assetID)
	lookupPair := fetchLookupPair(t, router, orgID, "ASSET-FKONLY")

	require.NotNil(t, listPair.id)
	assert.Equal(t, locID, *listPair.id)
	assert.Equal(t, "WHS-FK", *listPair.externalKey)

	assert.Equal(t, listPair.String(), detailPair.String())
	assert.Equal(t, listPair.String(), lookupPair.String())
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `just backend test-integration -run TestCurrentLocation_ -v`
(or `cd backend && go test -tags=integration ./internal/handlers/assets/ -run TestCurrentLocation_ -v`)

Expected: `TestCurrentLocation_ScanInferred_ConsistentAcrossReads` FAILS — detail/lookup return `id=<nil>` while list returns the populated id. The `assert.Equal(... "detail FK pair must match list")` fires.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/assets/current_location_consistency_integration_test.go
git commit -m "test(assets): TRA-576 add failing FK-pair consistency tests across read paths"
```

---

### Task 2: Align detail and lookup SQL to list-path semantics

**Files:**
- Modify: `backend/internal/storage/assets.go` (`getAssetWithLocationByID`, `GetAssetByExternalKey`)

- [ ] **Step 1: Edit `getAssetWithLocationByID`**

Replace lines 502–525 (the block from the `// TRA-477:` comment through `LIMIT 1`) with:

```go
	// TRA-576: align with ListAssetsFiltered. Latest scan wins; explicit
	// current_location_id is the fallback (TRA-495). Selecting the
	// coalesced expression for both the int and the JOIN guarantees the
	// FK pair is always derived from the same row — they're populated or
	// null together.
	query := `
		WITH latest_scan AS (
			SELECT s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $2 AND s.asset_id = $1
			ORDER BY s.timestamp DESC
			LIMIT 1
		)
		SELECT
			a.id, a.org_id, a.external_key, a.name, a.description,
			COALESCE(ls.location_id, a.current_location_id),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.external_key
		FROM trakrf.assets a
		LEFT JOIN latest_scan ls ON true
		LEFT JOIN trakrf.locations l
			ON l.id = COALESCE(ls.location_id, a.current_location_id)
			AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE a.id = $1 AND a.org_id = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
```

- [ ] **Step 2: Edit `GetAssetByExternalKey`**

Replace lines 600–627 (the block from the `// TRA-477:` comment through `LIMIT 1`) with:

```go
	// TRA-576: same scan-first / FK-fallback expression as
	// ListAssetsFiltered and getAssetWithLocationByID, so all read paths
	// return identical (current_location_id, current_location_external_key)
	// pairs.
	query := `
		WITH latest_scan AS (
			SELECT s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1 AND s.asset_id = (
				SELECT id FROM trakrf.assets
				WHERE org_id = $1 AND external_key = $2 AND deleted_at IS NULL
				LIMIT 1
			)
			ORDER BY s.timestamp DESC
			LIMIT 1
		)
		SELECT
			a.id, a.org_id, a.external_key, a.name, a.description,
			COALESCE(ls.location_id, a.current_location_id),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.external_key
		FROM trakrf.assets a
		LEFT JOIN latest_scan ls ON true
		LEFT JOIN trakrf.locations l
			ON l.id = COALESCE(ls.location_id, a.current_location_id)
			AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE a.org_id = $1 AND a.external_key = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
```

- [ ] **Step 3: Run the new tests**

Run: `just backend test-integration -run TestCurrentLocation_ -v`

Expected: all three TestCurrentLocation_* tests PASS.

- [ ] **Step 4: Run the full backend test suite to check for regressions**

Run: `just backend test`

Expected: PASS. No prior tests assert FK-first precedence in a way that would break.

- [ ] **Step 5: Run the integration suite for assets to be sure**

Run: `cd backend && go test -tags=integration ./internal/handlers/assets/ ./internal/storage/ -v -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/assets.go
git commit -m "fix(assets): TRA-576 align detail/lookup current_location derivation with list path"
```

---

### Task 3: Lint, then push and open PR

- [ ] **Step 1: Run lint**

Run: `just lint`

Expected: PASS.

- [ ] **Step 2: Push branch**

```bash
git push -u origin fix/tra-576-uniform-current-location-derivation
```

- [ ] **Step 3: Open PR via gh**

```bash
gh pr create --title "fix(assets): TRA-576 uniform current_location derivation across asset read paths" --body "..."
```

PR body covers: problem, root cause, fix (align detail/lookup to list-path scan-first semantics, derive both FK fields from the same expression), test plan (three new integration cases), out-of-scope (bulk import, .tra555-needs-rewrite files), Linear ref.

---

## Self-review

- AC1 (consistent FK pair across `GET /assets`, `GET /assets/{id}`, `GET /assets/lookup`) — Task 1 tests it directly; Task 2 implements it.
- AC2 (BB15 reproduction case identical across reads) — `TestCurrentLocation_ScanInferred_ConsistentAcrossReads` is the BB15 setup (asset with no FK + scan at WHS-01).
- AC3 (integration test for invariant) — Task 1.
- AC4 (PublicAssetView schema unchanged) — no model edits.
- No placeholders. Type names, helper names, and SQL strings are consistent across tasks.
