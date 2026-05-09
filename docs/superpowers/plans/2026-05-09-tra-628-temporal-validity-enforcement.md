# TRA-628: Temporal-Validity Predicate Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce the temporal-validity predicate `(valid_from IS NULL OR valid_from <= NOW()) AND (valid_to IS NULL OR valid_to > NOW())` on all public read paths for assets, locations, embedded tags, and reports — except path-`{id}` GET, which overrides the predicate.

**Architecture:** A single SQL-fragment helper `temporallyEffective(alias string) string` is added to `backend/internal/storage`. Every list/filter/report query path appends the fragment to its WHERE clauses. Tag embed queries replace their hardcoded `is_active = true` clause with the temporal predicate. Single-resource path-`{id}` GET handlers are unchanged.

**Tech Stack:** Go 1.x, pgx/v5, raw SQL (no query builder), `testutil.SetupTestDB` integration harness, swag-based OpenAPI generation via `just api-spec`.

---

## Spec reference

- Design: [`docs/superpowers/specs/2026-05-09-tra-628-temporal-validity-enforcement-design.md`](../specs/2026-05-09-tra-628-temporal-validity-enforcement-design.md)
- Linear: [TRA-628](https://linear.app/trakrf/issue/TRA-628/bb20-c2-document-the-currently-effective-rule-is-active-valid)

## Site inventory (from audit)

| File | Function / line | Action |
|---|---|---|
| `backend/internal/storage/temporal.go` | new file | Create `temporallyEffective` helper |
| `backend/internal/storage/assets.go` | `buildAssetsWhere` line 848-889 | Append predicate; update Q-search tag subquery (line 884) |
| `backend/internal/storage/locations.go` | `buildLocationsWhere` line 890-921 | Append predicate; update Q-search tag subquery (line 916) |
| `backend/internal/storage/reports.go` | `CountCurrentLocations` line 97-132 | Apply predicate to assets+locations+tag subquery (line 116) |
| `backend/internal/storage/reports.go` | `buildCurrentLocationsQueryDistinctOn` line 134-168 | Same; tag subquery line 162 |
| `backend/internal/storage/reports.go` | `buildCurrentLocationsQueryTimescale` line 170-204 | Same; tag subquery line 198 |
| `backend/internal/storage/reports.go` | `ListAssetHistory` line 207-265 | Apply predicate to location join (line 217) |
| `backend/internal/storage/tags.go` | `GetTagsByAssetID` line 16-47 | Apply predicate to embedded tag query (line 20) |
| `backend/internal/storage/tags.go` | `GetTagsByLocationID` line 49-80 | Apply predicate to embedded tag query (line 53) |
| `backend/internal/storage/assets.go` | `getAssetWithLocationByID` line 510 | **No change.** Path-`{id}` override. |
| `backend/internal/storage/locations.go` | `GetLocationByID` line 99 | **No change.** Path-`{id}` override. |
| `backend/internal/handlers/assets/assets.go` | `ListAssets` swagger (line 412) | Update `@Description` to note default scope |
| `backend/internal/handlers/assets/assets.go` | Get-by-id swagger (line 521) | Update `@Description` to note path-`{id}` override |
| `backend/internal/handlers/locations/locations.go` | analogous swagger annotations | Same |
| `backend/internal/handlers/reports/*.go` | `ListCurrentLocations`, history endpoint annotations | Same |

---

## Task 1: Create the `temporallyEffective` helper

**Files:**
- Create: `backend/internal/storage/temporal.go`
- Test: `backend/internal/storage/temporal_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/storage/temporal_test.go`:

```go
package storage

import "testing"

func TestTemporallyEffective(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		want  string
	}{
		{
			name:  "asset alias",
			alias: "a",
			want:  "(a.valid_from IS NULL OR a.valid_from <= NOW()) AND (a.valid_to IS NULL OR a.valid_to > NOW())",
		},
		{
			name:  "location alias",
			alias: "l",
			want:  "(l.valid_from IS NULL OR l.valid_from <= NOW()) AND (l.valid_to IS NULL OR l.valid_to > NOW())",
		},
		{
			name:  "tag alias",
			alias: "i",
			want:  "(i.valid_from IS NULL OR i.valid_from <= NOW()) AND (i.valid_to IS NULL OR i.valid_to > NOW())",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := temporallyEffective(tc.alias)
			if got != tc.want {
				t.Fatalf("temporallyEffective(%q):\n  want: %s\n  got:  %s", tc.alias, tc.want, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just backend test ./internal/storage -run TestTemporallyEffective -v`
Expected: FAIL — `undefined: temporallyEffective`

- [ ] **Step 3: Write minimal implementation**

Create `backend/internal/storage/temporal.go`:

```go
package storage

import "fmt"

// temporallyEffective returns a SQL fragment matching rows that are currently
// effective per the bitemporal validity columns (valid_from, valid_to).
// Composes with deleted_at IS NULL and any other filters via AND.
//
// alias is the SQL alias the surrounding query uses for the table being filtered
// (e.g. "a" for assets, "l" for locations, "i" for tags).
//
// NULL valid_from is treated as "always-was" and NULL valid_to as "open-ended"
// so rows with unset windows remain visible by default.
func temporallyEffective(alias string) string {
	return fmt.Sprintf(
		"(%[1]s.valid_from IS NULL OR %[1]s.valid_from <= NOW()) AND (%[1]s.valid_to IS NULL OR %[1]s.valid_to > NOW())",
		alias,
	)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `just backend test ./internal/storage -run TestTemporallyEffective -v`
Expected: PASS — three subtests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/temporal.go backend/internal/storage/temporal_test.go
git commit -m "feat(storage): TRA-628 add temporallyEffective predicate helper

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Apply predicate to `buildAssetsWhere`

**Files:**
- Modify: `backend/internal/storage/assets.go:848-889`
- Test: `backend/internal/handlers/assets/temporal_validity_integration_test.go` (new)

- [ ] **Step 1: Write the failing integration test**

Create `backend/internal/handlers/assets/temporal_validity_integration_test.go`:

```go
//go:build integration
// +build integration

// TRA-628: Default list scope must apply temporal-validity predicate;
// path-{id} GET must override it.

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

func setupTemporalRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)
	r.Get("/api/v1/assets/{asset_id}", handler.GetAsset)
	return r
}

func withTemporalOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra628@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

// seedAssetWithWindow inserts an asset with explicit valid_from / valid_to.
// Pass nil to leave a column NULL.
func seedAssetWithWindow(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom, validTo *time.Time) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, externalKey, validFrom, validTo).Scan(&id)
	require.NoError(t, err)
	return id
}

type listResp struct {
	Data       []assetmodel.PublicAssetView `json:"data"`
	TotalCount int                          `json:"total_count"`
}

func doListReq(t *testing.T, router *chi.Mux, orgID int, query string) (int, listResp) {
	t.Helper()
	url := "/api/v1/assets"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Code, listResp{}
	}
	var resp listResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func doGetByIDReq(t *testing.T, router *chi.Mux, orgID, id int) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func externalKeysOf(items []assetmodel.PublicAssetView) []string {
	out := make([]string, 0, len(items))
	for _, a := range items {
		out = append(out, a.ExternalKey)
	}
	return out
}

func TestListAssets_TemporalValidity_DefaultScopeExcludesExpiredAndFuture(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	weekHence := now.Add(7 * 24 * time.Hour)

	effectiveID := seedAssetWithWindow(t, pool, orgID, "EFFECTIVE", &yesterday, nil)
	openEndedID := seedAssetWithWindow(t, pool, orgID, "OPENENDED", nil, nil)
	expiredID := seedAssetWithWindow(t, pool, orgID, "EXPIRED", &weekAgo, &yesterday)
	futureID := seedAssetWithWindow(t, pool, orgID, "FUTURE", &tomorrow, &weekHence)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	// List default scope — must include effective + open-ended, exclude expired + future
	code, resp := doListReq(t, router, orgID, "")
	require.Equal(t, http.StatusOK, code)
	keys := externalKeysOf(resp.Data)
	assert.Contains(t, keys, "EFFECTIVE")
	assert.Contains(t, keys, "OPENENDED")
	assert.NotContains(t, keys, "EXPIRED")
	assert.NotContains(t, keys, "FUTURE")

	// GET by id — overrides predicate, returns 200 for all four
	for _, id := range []int{effectiveID, openEndedID, expiredID, futureID} {
		assert.Equal(t, http.StatusOK, doGetByIDReq(t, router, orgID, id), "GET by id should ignore predicate (id=%d)", id)
	}

	// ?external_key= filter applies the predicate (list shape)
	_, resp = doListReq(t, router, orgID, "external_key=EXPIRED")
	assert.Empty(t, resp.Data, "?external_key=EXPIRED should return no rows")

	_, resp = doListReq(t, router, orgID, "external_key=EFFECTIVE")
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "EFFECTIVE", resp.Data[0].ExternalKey)
}

func TestListAssets_TemporalValidity_IsActiveIndependentOfPredicate(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)

	// Two effective rows: one is_active=true, one is_active=false
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, 'ACT-TRUE', 'ACT-TRUE', '', $2, true), ($1, 'ACT-FALSE', 'ACT-FALSE', '', $2, false)
	`, orgID, yesterday)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	// Default (no is_active filter): both visible
	_, resp := doListReq(t, router, orgID, "")
	keys := externalKeysOf(resp.Data)
	assert.Contains(t, keys, "ACT-TRUE")
	assert.Contains(t, keys, "ACT-FALSE")

	// ?is_active=true: only the active row
	_, resp = doListReq(t, router, orgID, "is_active=true")
	keys = externalKeysOf(resp.Data)
	assert.Contains(t, keys, "ACT-TRUE")
	assert.NotContains(t, keys, "ACT-FALSE")

	// ?is_active=false: only the inactive row (still temporally effective)
	_, resp = doListReq(t, router, orgID, "is_active=false")
	keys = externalKeysOf(resp.Data)
	assert.NotContains(t, keys, "ACT-TRUE")
	assert.Contains(t, keys, "ACT-FALSE")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just backend test-integration ./internal/handlers/assets -run TestListAssets_TemporalValidity -v`
Expected: FAIL — `EXPIRED` and `FUTURE` rows appear in default list (drift), or `?is_active=false` returns nothing if the predicate is applied wrongly.

- [ ] **Step 3: Implement — append predicate to `buildAssetsWhere` and update Q-search tag subquery**

Modify `backend/internal/storage/assets.go` lines 848-889. The function should look like:

```go
func buildAssetsWhere(orgID int, f asset.ListFilter) (string, []any) {
	clauses := []string{
		"a.org_id = $1",
		"a.deleted_at IS NULL",
		temporallyEffective("a"),
	}
	args := []any{orgID}

	// location_id and location_external_key combine with OR semantics — a row
	// matches if its current location appears in either set.
	hasIDs := len(f.LocationIDs) > 0
	hasExtKeys := len(f.LocationExternalKeys) > 0
	if hasIDs && hasExtKeys {
		args = append(args, f.LocationIDs)
		idIdx := len(args)
		args = append(args, f.LocationExternalKeys)
		ekIdx := len(args)
		clauses = append(clauses, fmt.Sprintf("(l.id = ANY($%d::int[]) OR l.external_key = ANY($%d::text[]))", idIdx, ekIdx))
	} else if hasIDs {
		args = append(args, f.LocationIDs)
		clauses = append(clauses, fmt.Sprintf("l.id = ANY($%d::int[])", len(args)))
	} else if hasExtKeys {
		args = append(args, f.LocationExternalKeys)
		clauses = append(clauses, fmt.Sprintf("l.external_key = ANY($%d::text[])", len(args)))
	}

	if len(f.ExternalKeys) > 0 {
		args = append(args, f.ExternalKeys)
		clauses = append(clauses, fmt.Sprintf("a.external_key = ANY($%d::text[])", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		clauses = append(clauses, fmt.Sprintf("a.is_active = $%d", len(args)))
	}
	if f.Q != nil {
		args = append(args, "%"+*f.Q+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(a.name ILIKE $%d OR a.external_key ILIKE $%d OR a.description ILIKE $%d "+
				"OR EXISTS (SELECT 1 FROM trakrf.tags i "+
				"WHERE i.asset_id = a.id AND i.deleted_at IS NULL AND "+temporallyEffective("i")+" "+
				"AND i.value ILIKE $%d))",
			idx, idx, idx, idx))
	}
	return strings.Join(clauses, " AND "), args
}
```

The two changes vs. existing code:
1. `temporallyEffective("a")` added as third clause.
2. Q-search tag subquery — `i.is_active = true` removed, replaced with `temporallyEffective("i")`.

- [ ] **Step 4: Run integration tests to verify pass**

Run: `just backend test-integration ./internal/handlers/assets -run TestListAssets_TemporalValidity -v`
Expected: PASS — both subtests pass.

- [ ] **Step 5: Run full assets handler integration suite to verify no regression**

Run: `just backend test-integration ./internal/handlers/assets -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/assets.go backend/internal/handlers/assets/temporal_validity_integration_test.go
git commit -m "feat(api): TRA-628 enforce temporal validity on assets list

Apply temporallyEffective predicate to buildAssetsWhere default scope and
to the embedded tag Q-search subquery. Path-{id} GET unchanged (overrides
predicate per design).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Apply predicate to `buildLocationsWhere`

**Files:**
- Modify: `backend/internal/storage/locations.go:890-921`
- Test: `backend/internal/handlers/locations/temporal_validity_integration_test.go` (new)

- [ ] **Step 1: Write the failing integration test**

Create `backend/internal/handlers/locations/temporal_validity_integration_test.go`:

```go
//go:build integration
// +build integration

// TRA-628: Default list scope must apply temporal-validity predicate;
// path-{id} GET must override it.

package locations

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
	locationmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTemporalRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations", handler.ListLocations)
	r.Get("/api/v1/locations/{location_id}", handler.GetLocation)
	return r
}

func withTemporalOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra628-loc@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocationWithWindow(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom, validTo *time.Time) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, externalKey, validFrom, validTo).Scan(&id)
	require.NoError(t, err)
	return id
}

type locListResp struct {
	Data       []locationmodel.PublicLocationView `json:"data"`
	TotalCount int                                `json:"total_count"`
}

func doLocListReq(t *testing.T, router *chi.Mux, orgID int, query string) (int, locListResp) {
	t.Helper()
	url := "/api/v1/locations"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Code, locListResp{}
	}
	var resp locListResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func doLocGetByIDReq(t *testing.T, router *chi.Mux, orgID, id int) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func locExternalKeysOf(items []locationmodel.PublicLocationView) []string {
	out := make([]string, 0, len(items))
	for _, a := range items {
		out = append(out, a.ExternalKey)
	}
	return out
}

func TestListLocations_TemporalValidity_DefaultScopeExcludesExpiredAndFuture(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	weekHence := now.Add(7 * 24 * time.Hour)

	effectiveID := seedLocationWithWindow(t, pool, orgID, "L-EFFECTIVE", &yesterday, nil)
	openEndedID := seedLocationWithWindow(t, pool, orgID, "L-OPENENDED", nil, nil)
	expiredID := seedLocationWithWindow(t, pool, orgID, "L-EXPIRED", &weekAgo, &yesterday)
	futureID := seedLocationWithWindow(t, pool, orgID, "L-FUTURE", &tomorrow, &weekHence)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	code, resp := doLocListReq(t, router, orgID, "")
	require.Equal(t, http.StatusOK, code)
	keys := locExternalKeysOf(resp.Data)
	assert.Contains(t, keys, "L-EFFECTIVE")
	assert.Contains(t, keys, "L-OPENENDED")
	assert.NotContains(t, keys, "L-EXPIRED")
	assert.NotContains(t, keys, "L-FUTURE")

	for _, id := range []int{effectiveID, openEndedID, expiredID, futureID} {
		assert.Equal(t, http.StatusOK, doLocGetByIDReq(t, router, orgID, id), "GET by id should ignore predicate (id=%d)", id)
	}

	_, resp = doLocListReq(t, router, orgID, "external_key=L-EXPIRED")
	assert.Empty(t, resp.Data, "?external_key=L-EXPIRED should return no rows")

	_, resp = doLocListReq(t, router, orgID, "external_key=L-EFFECTIVE")
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "L-EFFECTIVE", resp.Data[0].ExternalKey)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just backend test-integration ./internal/handlers/locations -run TestListLocations_TemporalValidity -v`
Expected: FAIL — expired and future rows appear.

- [ ] **Step 3: Implement — `buildLocationsWhere` update**

Modify `backend/internal/storage/locations.go` lines 890-921. The two changes are: prepend `temporallyEffective("l")` to default clauses; replace `i.is_active = true` in the Q-search tag subquery with `temporallyEffective("i")`.

```go
func buildLocationsWhere(orgID int, f location.ListFilter) (string, []any) {
	clauses := []string{
		"l.org_id = $1",
		"l.deleted_at IS NULL",
		temporallyEffective("l"),
	}
	args := []any{orgID}

	if len(f.ParentIDs) > 0 {
		args = append(args, f.ParentIDs)
		clauses = append(clauses, fmt.Sprintf("p.id = ANY($%d::int[])", len(args)))
	}
	if len(f.ParentExternalKeys) > 0 {
		args = append(args, f.ParentExternalKeys)
		clauses = append(clauses, fmt.Sprintf("p.external_key = ANY($%d::text[])", len(args)))
	}
	if len(f.ExternalKeys) > 0 {
		args = append(args, f.ExternalKeys)
		clauses = append(clauses, fmt.Sprintf("l.external_key = ANY($%d::text[])", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		clauses = append(clauses, fmt.Sprintf("l.is_active = $%d", len(args)))
	}
	if f.Q != nil {
		args = append(args, "%"+*f.Q+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(l.name ILIKE $%d OR l.external_key ILIKE $%d OR l.description ILIKE $%d "+
				"OR EXISTS (SELECT 1 FROM trakrf.tags i "+
				"WHERE i.location_id = l.id AND i.deleted_at IS NULL AND "+temporallyEffective("i")+" "+
				"AND i.value ILIKE $%d))",
			idx, idx, idx, idx))
	}
	return strings.Join(clauses, " AND "), args
}
```

- [ ] **Step 4: Run integration test to verify pass**

Run: `just backend test-integration ./internal/handlers/locations -run TestListLocations_TemporalValidity -v`
Expected: PASS.

- [ ] **Step 5: Run full locations handler integration suite**

Run: `just backend test-integration ./internal/handlers/locations -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/locations.go backend/internal/handlers/locations/temporal_validity_integration_test.go
git commit -m "feat(api): TRA-628 enforce temporal validity on locations list

Apply temporallyEffective predicate to buildLocationsWhere default scope
and to the embedded tag Q-search subquery. Path-{id} GET unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Apply predicate to reports — `ListCurrentLocations`, `CountCurrentLocations`

**Files:**
- Modify: `backend/internal/storage/reports.go:97-204`
- Test: `backend/internal/handlers/reports/temporal_validity_integration_test.go` (new — current_locations only here; history added in Task 5)

- [ ] **Step 1: Write the failing integration test**

Create `backend/internal/handlers/reports/temporal_validity_integration_test.go`:

```go
//go:build integration
// +build integration

// TRA-628: /locations/current must apply temporal-validity predicate to both
// the assets join and the locations join.

package reports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

type currLocResp struct {
	Data       []report.PublicCurrentLocationItem `json:"data"`
	TotalCount int                                `json:"total_count"`
}

func setupCurrLocTemporalRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations/current", handler.ListCurrentLocations)
	return r
}

func withCurrLocOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra628-rep@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedAssetCurrLoc(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom, validTo *time.Time) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, externalKey, validFrom, validTo).Scan(&id))
	return id
}

func seedLocCurrLoc(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom, validTo *time.Time) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, externalKey, validFrom, validTo).Scan(&id))
	return id
}

func seedScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID, locationID int, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (org_id, asset_id, location_id, timestamp)
		VALUES ($1, $2, $3, $4)
	`, orgID, assetID, locationID, ts)
	require.NoError(t, err)
}

func TestListCurrentLocations_TemporalValidity_FiltersAssetsAndLocations(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	effectiveAsset := seedAssetCurrLoc(t, pool, orgID, "CL-A-EFF", &yesterday, nil)
	expiredAsset := seedAssetCurrLoc(t, pool, orgID, "CL-A-EXP", &weekAgo, &yesterday)

	effectiveLoc := seedLocCurrLoc(t, pool, orgID, "CL-L-EFF", &yesterday, nil)
	expiredLoc := seedLocCurrLoc(t, pool, orgID, "CL-L-EXP", &weekAgo, &yesterday)

	// Effective asset → effective loc — should appear with location populated
	seedScan(t, pool, orgID, effectiveAsset, effectiveLoc, now)
	// Effective asset → expired loc — asset appears but location fields NULL/empty
	// (LEFT JOIN with predicate filters out the location row). Asset itself still listed.
	effectiveAsset2 := seedAssetCurrLoc(t, pool, orgID, "CL-A-EFF2", &yesterday, nil)
	seedScan(t, pool, orgID, effectiveAsset2, expiredLoc, now)
	// Expired asset → effective loc — must be excluded entirely
	seedScan(t, pool, orgID, expiredAsset, effectiveLoc, now)

	handler := NewHandler(store)
	router := setupCurrLocTemporalRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/current", nil)
	req = withCurrLocOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp currLocResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assetKeys := make(map[string]report.PublicCurrentLocationItem)
	for _, item := range resp.Data {
		assetKeys[item.AssetExternalKey] = item
	}

	assert.Contains(t, assetKeys, "CL-A-EFF", "effective asset with effective location must appear")
	assert.Contains(t, assetKeys, "CL-A-EFF2", "effective asset with expired location must still appear (LEFT JOIN)")
	assert.NotContains(t, assetKeys, "CL-A-EXP", "expired asset must be excluded entirely")

	// Effective asset with effective location — location fields populated
	if eff, ok := assetKeys["CL-A-EFF"]; ok {
		assert.Equal(t, "CL-L-EFF", eff.LocationExternalKey)
	}

	// Effective asset with expired location — location_id present (from scan) but name should be empty/NULL
	// because the LEFT JOIN's predicate filters out the location row.
	if eff2, ok := assetKeys["CL-A-EFF2"]; ok {
		assert.Empty(t, eff2.LocationExternalKey, "expired location must not surface name/external_key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just backend test-integration ./internal/handlers/reports -run TestListCurrentLocations_TemporalValidity -v`
Expected: FAIL — expired asset appears in output, expired location's name is populated.

- [ ] **Step 3: Implement — update three queries in `reports.go`**

Modify `backend/internal/storage/reports.go`:

For `CountCurrentLocations` (lines 97-132), replace the inline query string with this version (only changes are the new predicate clauses on `a` and `l`, and the tag subquery's `is_active=true` → `temporallyEffective("ai")`):

```go
	query := `
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT COUNT(*)
		FROM latest_scans ls
		JOIN trakrf.assets    a ON a.id = ls.asset_id AND a.org_id = $1 AND ` + temporallyEffective("a") + `
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.org_id = $1 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
		WHERE ($2::int[]  IS NULL OR l.id           = ANY($2::int[]))
		  AND ($3::text[] IS NULL OR l.external_key = ANY($3::text[]))
		  AND ($4::text IS NULL OR a.name ILIKE $4 OR a.external_key ILIKE $4
			   OR EXISTS (
				   SELECT 1 FROM trakrf.tags ai
				   WHERE ai.asset_id = a.id AND ai.deleted_at IS NULL AND ` + temporallyEffective("ai") + ` AND ai.value ILIKE $4
			   ))
		  AND (a.deleted_at IS NULL OR $5::bool)
	`
```

For `buildCurrentLocationsQueryDistinctOn` (lines 134-168), make the same edits to its returned string:

```go
func buildCurrentLocationsQueryDistinctOn() string {
	return `
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id,
				s.timestamp AS last_seen
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT
			a.id            AS asset_id,
			a.name          AS asset_name,
			a.external_key  AS asset_external_key,
			l.id            AS location_id,
			l.name          AS location_name,
			l.external_key  AS location_external_key,
			ls.last_seen,
			a.deleted_at    AS asset_deleted_at
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id AND a.org_id = $1 AND ` + temporallyEffective("a") + `
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.org_id = $1 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
		WHERE ($2::int[]  IS NULL OR l.id           = ANY($2::int[]))
		  AND ($3::text[] IS NULL OR l.external_key = ANY($3::text[]))
		  AND ($4::text IS NULL OR a.name ILIKE $4 OR a.external_key ILIKE $4
			   OR EXISTS (
				   SELECT 1 FROM trakrf.tags ai
				   WHERE ai.asset_id = a.id AND ai.deleted_at IS NULL AND ` + temporallyEffective("ai") + ` AND ai.value ILIKE $4
			   ))
		  AND (a.deleted_at IS NULL OR $7::bool)
		ORDER BY a.name
		LIMIT $5 OFFSET $6
	`
}
```

For `buildCurrentLocationsQueryTimescale` (lines 170-204), same shape:

```go
func buildCurrentLocationsQueryTimescale() string {
	return `
		WITH latest_scans AS (
			SELECT
				asset_id,
				last(location_id, timestamp) AS location_id,
				max(timestamp) AS last_seen
			FROM trakrf.asset_scans
			WHERE org_id = $1
			GROUP BY asset_id
		)
		SELECT
			a.id            AS asset_id,
			a.name          AS asset_name,
			a.external_key  AS asset_external_key,
			l.id            AS location_id,
			l.name          AS location_name,
			l.external_key  AS location_external_key,
			ls.last_seen,
			a.deleted_at    AS asset_deleted_at
		FROM latest_scans ls
		JOIN trakrf.assets a ON a.id = ls.asset_id AND a.org_id = $1 AND ` + temporallyEffective("a") + `
		LEFT JOIN trakrf.locations l ON l.id = ls.location_id AND l.org_id = $1 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
		WHERE ($2::int[]  IS NULL OR l.id           = ANY($2::int[]))
		  AND ($3::text[] IS NULL OR l.external_key = ANY($3::text[]))
		  AND ($4::text IS NULL OR a.name ILIKE $4 OR a.external_key ILIKE $4
			   OR EXISTS (
				   SELECT 1 FROM trakrf.tags ai
				   WHERE ai.asset_id = a.id AND ai.deleted_at IS NULL AND ` + temporallyEffective("ai") + ` AND ai.value ILIKE $4
			   ))
		  AND (a.deleted_at IS NULL OR $7::bool)
		ORDER BY a.name
		LIMIT $5 OFFSET $6
	`
}
```

- [ ] **Step 4: Run integration test to verify pass**

Run: `just backend test-integration ./internal/handlers/reports -run TestListCurrentLocations_TemporalValidity -v`
Expected: PASS.

- [ ] **Step 5: Run full reports handler integration suite**

Run: `just backend test-integration ./internal/handlers/reports -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/reports.go backend/internal/handlers/reports/temporal_validity_integration_test.go
git commit -m "feat(api): TRA-628 enforce temporal validity on /locations/current

Apply predicate to assets and locations joins in DistinctOn + Timescale
variants of ListCurrentLocations and in CountCurrentLocations. Tag
subqueries drop hardcoded is_active=true.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Apply predicate to `ListAssetHistory` location join

**Files:**
- Modify: `backend/internal/storage/reports.go:207-265`
- Test: `backend/internal/handlers/reports/temporal_validity_integration_test.go` (extend)

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/handlers/reports/temporal_validity_integration_test.go`:

```go
type historyResp struct {
	Data []report.PublicAssetHistoryItem `json:"data"`
}

func setupHistoryTemporalRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets/{asset_id}/history", handler.ListAssetHistory)
	return r
}

func TestListAssetHistory_TemporalValidity_LocationJoinAppliesPredicate(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	// Asset is path-addressed — predicate doesn't apply to its existence check.
	// Use an expired asset to confirm the override on the asset side.
	expiredAsset := seedAssetCurrLoc(t, pool, orgID, "H-A-EXP", &weekAgo, &yesterday)
	effectiveLoc := seedLocCurrLoc(t, pool, orgID, "H-L-EFF", &yesterday, nil)
	expiredLoc := seedLocCurrLoc(t, pool, orgID, "H-L-EXP", &weekAgo, &yesterday)

	seedScan(t, pool, orgID, expiredAsset, effectiveLoc, now.Add(-2*time.Hour))
	seedScan(t, pool, orgID, expiredAsset, expiredLoc, now.Add(-1*time.Hour))

	handler := NewHandler(store)
	router := setupHistoryTemporalRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+itoa(expiredAsset)+"/history", nil)
	req = withCurrLocOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "expired asset must be addressable by id (path-{id} override)")

	var resp historyResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2, "both scans must surface")

	// Find each row by location_id and verify the predicate filtered names
	for _, item := range resp.Data {
		if item.LocationID != nil && *item.LocationID == effectiveLoc {
			assert.Equal(t, "H-L-EFF", *item.LocationExternalKey)
		}
		if item.LocationID != nil && *item.LocationID == expiredLoc {
			// Expired location — predicate filters the join, name fields nil/empty
			assert.Nil(t, item.LocationExternalKey, "expired location must not surface name/external_key")
		}
	}
}

// itoa avoids a strconv import for one call site
func itoa(i int) string { return fmt.Sprintf("%d", i) }
```

(If `fmt` is not yet imported in this file, add it to the import block.)

- [ ] **Step 2: Run test to verify it fails**

Run: `just backend test-integration ./internal/handlers/reports -run TestListAssetHistory_TemporalValidity -v`
Expected: FAIL — expired location's name is still populated.

- [ ] **Step 3: Implement — update `ListAssetHistory` query**

Modify `backend/internal/storage/reports.go` lines 208-232. Add the predicate to the location join only:

```go
	query := `
		WITH scans AS (
			SELECT
				s.timestamp,
				s.location_id,
				l.name         AS location_name,
				l.external_key AS location_external_key,
				LEAD(s.timestamp) OVER (ORDER BY s.timestamp) AS next_timestamp
			FROM trakrf.asset_scans s
			LEFT JOIN trakrf.locations l ON l.id = s.location_id AND l.org_id = $2 AND l.deleted_at IS NULL AND ` + temporallyEffective("l") + `
			WHERE s.asset_id = $1
			  AND s.org_id = $2
			  AND ($3::timestamptz IS NULL OR s.timestamp >= $3)
			  AND ($4::timestamptz IS NULL OR s.timestamp <= $4)
		)
		SELECT
			timestamp,
			location_id,
			location_name,
			location_external_key,
			EXTRACT(EPOCH FROM (next_timestamp - timestamp))::INT AS duration_seconds
		FROM scans
		ORDER BY timestamp DESC
		LIMIT $5 OFFSET $6
	`
```

`CountAssetHistory` (lines 268-285) does not join `locations` and is unchanged.

- [ ] **Step 4: Run integration test to verify pass**

Run: `just backend test-integration ./internal/handlers/reports -run TestListAssetHistory_TemporalValidity -v`
Expected: PASS.

- [ ] **Step 5: Run full reports handler integration suite**

Run: `just backend test-integration ./internal/handlers/reports -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/reports.go backend/internal/handlers/reports/temporal_validity_integration_test.go
git commit -m "feat(api): TRA-628 enforce temporal validity on asset history join

Apply predicate to the location LEFT JOIN inside ListAssetHistory so a
history row referencing an expired location surfaces NULL location
metadata, matching deleted_at IS NULL behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Apply predicate to embedded tag queries

**Files:**
- Modify: `backend/internal/storage/tags.go:16-80`
- Test: extend `backend/internal/handlers/assets/temporal_validity_integration_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/handlers/assets/temporal_validity_integration_test.go`:

```go
func seedTagOnAsset(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, tagType, value string, validFrom, validTo *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, asset_id, type, value, is_active, valid_from, valid_to)
		VALUES ($1, $2, $3, $4, true, $5, $6)
	`, orgID, assetID, tagType, value, validFrom, validTo)
	require.NoError(t, err)
}

func TestGetAsset_TemporalValidity_EmbeddedTagsFilterPredicate(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	assetID := seedAssetWithWindow(t, pool, orgID, "TAG-HOST", &yesterday, nil)
	seedTagOnAsset(t, pool, orgID, assetID, "rfid", "EFFECTIVE-TAG", &yesterday, nil)
	seedTagOnAsset(t, pool, orgID, assetID, "rfid", "EXPIRED-TAG", &weekAgo, &yesterday)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", assetID), nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var view assetmodel.PublicAssetView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &view))

	tagValues := make([]string, 0, len(view.Tags))
	for _, tag := range view.Tags {
		tagValues = append(tagValues, tag.Value)
	}
	assert.Contains(t, tagValues, "EFFECTIVE-TAG")
	assert.NotContains(t, tagValues, "EXPIRED-TAG", "embedded tags must respect temporal predicate")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just backend test-integration ./internal/handlers/assets -run TestGetAsset_TemporalValidity_EmbeddedTags -v`
Expected: FAIL — `EXPIRED-TAG` appears.

- [ ] **Step 3: Implement — update `GetTagsByAssetID` and `GetTagsByLocationID`**

Modify `backend/internal/storage/tags.go`. Replace the two query strings:

```go
func (s *Storage) GetTagsByAssetID(ctx context.Context, orgID, assetID int) ([]shared.Tag, error) {
	query := `
		SELECT id, type, value, is_active
		FROM trakrf.tags
		WHERE asset_id = $1 AND org_id = $2 AND deleted_at IS NULL
		  AND ` + temporallyEffective("trakrf.tags") + `
		ORDER BY created_at ASC
	`
	// ... rest unchanged
```

Wait — the table has no alias here; the predicate must reference column names directly. Use a bare-column form by passing an empty alias or by inlining. Two options:

Option A (preferred): give the table an alias and reference it.

```go
func (s *Storage) GetTagsByAssetID(ctx context.Context, orgID, assetID int) ([]shared.Tag, error) {
	query := `
		SELECT i.id, i.type, i.value, i.is_active
		FROM trakrf.tags i
		WHERE i.asset_id = $1 AND i.org_id = $2 AND i.deleted_at IS NULL
		  AND ` + temporallyEffective("i") + `
		ORDER BY i.created_at ASC
	`

	var tags []shared.Tag
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, assetID, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		tags = []shared.Tag{}
		for rows.Next() {
			var tag shared.Tag
			if err := rows.Scan(&tag.ID, &tag.TagType, &tag.Value, &tag.IsActive); err != nil {
				return fmt.Errorf("failed to scan tag: %w", err)
			}
			tags = append(tags, tag)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for asset: %w", err)
	}

	return tags, nil
}

func (s *Storage) GetTagsByLocationID(ctx context.Context, orgID, locationID int) ([]shared.Tag, error) {
	query := `
		SELECT i.id, i.type, i.value, i.is_active
		FROM trakrf.tags i
		WHERE i.location_id = $1 AND i.org_id = $2 AND i.deleted_at IS NULL
		  AND ` + temporallyEffective("i") + `
		ORDER BY i.created_at ASC
	`

	var tags []shared.Tag
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, locationID, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		tags = []shared.Tag{}
		for rows.Next() {
			var tag shared.Tag
			if err := rows.Scan(&tag.ID, &tag.TagType, &tag.Value, &tag.IsActive); err != nil {
				return fmt.Errorf("failed to scan tag: %w", err)
			}
			tags = append(tags, tag)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for location: %w", err)
	}

	return tags, nil
}
```

The two changes per query: add alias `i` to the FROM clause, reference all columns via `i.`, append the predicate. Note the design decision drops the previously hardcoded `is_active = true` filter — embedded tags now follow the same "is_active is independent" rule as parents.

- [ ] **Step 4: Run integration test to verify pass**

Run: `just backend test-integration ./internal/handlers/assets -run TestGetAsset_TemporalValidity_EmbeddedTags -v`
Expected: PASS.

- [ ] **Step 5: Run full handler integration suite to verify nothing else broke**

Run: `just backend test-integration ./internal/handlers/assets ./internal/handlers/locations ./internal/handlers/reports -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/tags.go backend/internal/handlers/assets/temporal_validity_integration_test.go
git commit -m "feat(api): TRA-628 enforce temporal validity on embedded tags

Drop hardcoded is_active=true clause from GetTagsByAssetID and
GetTagsByLocationID; replace with temporallyEffective predicate. Aligns
embedded tags with the is_active-independent rule used on parents.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Update OpenAPI spec descriptions and regenerate

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (swagger annotations on `ListAssets` and Get-by-id)
- Modify: `backend/internal/handlers/locations/locations.go` (analogous)
- Modify: handlers in `backend/internal/handlers/reports/` for `/locations/current` and asset history endpoints
- Regenerate: `backend/internal/handlers/swaggerspec/openapi.public.yaml` (and any docs/api/ copy via `just api-spec`)

- [ ] **Step 1: Locate exact swagger annotation lines**

Run: `grep -nE '@Summary|@Description|@Param.*is_active|func.*ListAssets|func.*GetAsset|func.*ListLocations|func.*GetLocation|func.*ListCurrentLocations|func.*ListAssetHistory' backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go backend/internal/handlers/reports/*.go | head -60`

Expected: enumeration of every relevant swagger block.

- [ ] **Step 2: Update assets ListAssets `@Description`**

In `backend/internal/handlers/assets/assets.go`, around line 412-413, the existing single-line `@Description` is:

```go
// @Description Paginated assets list with natural-key filters, sort, and substring search
```

Replace with:

```go
// @Description Paginated assets list with natural-key filters, sort, and substring search.
// @Description
// @Description Default scope returns currently-effective assets only — rows whose `valid_from`
// @Description is null or in the past AND whose `valid_to` is null or in the future. The
// @Description `is_active` filter is independent of temporal validity; omit it to include both
// @Description active and inactive rows within the effective window, or pass `?is_active=true`/`false`
// @Description to filter further.
```

- [ ] **Step 3: Update assets Get-by-id `@Description`**

Around line 521-522, the existing `@Description` is:

```go
// @Description Retrieve an asset by its canonical id. Returns 404 if the asset does not exist.
```

Replace with:

```go
// @Description Retrieve an asset by its canonical id. Returns 404 if the asset does not exist.
// @Description
// @Description Path-addressed retrieval bypasses the temporal-validity filter applied on list
// @Description endpoints — any non-deleted asset is returned regardless of its `valid_from`/`valid_to`.
// @Description Use this endpoint when you have an id and need the row even if its effective window
// @Description has elapsed.
```

- [ ] **Step 4: Update locations annotations the same way**

Find the `ListLocations` and `GetLocation` swagger blocks in `backend/internal/handlers/locations/locations.go` (use the grep output from Step 1). Apply analogous updates:

- `ListLocations @Description`: append the same two-paragraph note about temporal-validity default scope and `is_active` independence (substituting "location" for "asset").
- `GetLocation @Description`: append the same path-`{id}`-override paragraph.

- [ ] **Step 5: Update reports annotations**

In `backend/internal/handlers/reports/`, find the swagger blocks for `ListCurrentLocations` and `ListAssetHistory`. Append:

- `ListCurrentLocations @Description`: a paragraph noting that temporally-invalid assets are excluded entirely, and that scans referencing temporally-invalid locations return the asset with empty location metadata (matching deleted-location behavior).
- `ListAssetHistory @Description`: a paragraph noting that the asset existence check follows GET-by-id override semantics (no temporal filter on the asset), but each history row's location reference applies the temporal predicate (expired locations surface as null location metadata).

Use clear, terse copy modeled on the existing TRA-499 / TRA-578 docs voice.

- [ ] **Step 6: Regenerate the OpenAPI spec**

Run: `just backend api-spec`
Expected: `backend/internal/handlers/swaggerspec/openapi.public.yaml` updated; if any docs/api/ copy exists, that is updated too.

- [ ] **Step 7: Verify the diff is meaningful only**

Run: `git diff -- backend/internal/handlers/swaggerspec/`
Expected: only the descriptions changed appear in the diff. Any unrelated formatting churn should be investigated and the regen pre-baselined per the design's risk note.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/handlers/assets/assets.go \
        backend/internal/handlers/locations/locations.go \
        backend/internal/handlers/reports/ \
        backend/internal/handlers/swaggerspec/
git commit -m "docs(api): TRA-628 document temporal-validity scope and path-{id} override

ListAssets, ListLocations, /locations/current, and /assets/{id}/history
default scopes now apply the temporal-validity predicate. Path-{id} GET
on assets and locations overrides it. Annotations updated and OpenAPI
regenerated.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Final validation

- [ ] **Step 1: Run the full unit test suite**

Run: `just backend test`
Expected: All unit tests pass.

- [ ] **Step 2: Run the full integration test suite**

Run: `just backend test-integration`
Expected: All integration tests pass. The Warehouse 1 preview anomaly is on the preview DB only and does not affect local integration.

- [ ] **Step 3: Run lint**

Run: `just backend lint`
Expected: No lint errors.

- [ ] **Step 4: Run combined validation**

Run: `just validate`
Expected: Both backend and frontend checks pass.

- [ ] **Step 5: Branch ready summary**

The branch should now contain:
- 1 spec commit (`docs(spec): TRA-628 …`)
- 6 implementation commits (helper, assets, locations, current_locations, asset_history, tags)
- 1 docs/spec commit (`docs(api): TRA-628 …`)

Confirm with `git log --oneline main..HEAD` — should show 8 commits.

- [ ] **Step 6: Push and open PR**

```bash
git push -u origin feat/tra-628-temporal-validity-enforcement
gh pr create --title "feat(api): TRA-628 enforce temporal-validity predicate on public read paths" --body "$(cat <<'EOF'
## Summary
- Adds `temporallyEffective(alias)` SQL helper and applies it to default scope on assets list, locations list, `/locations/current`, asset history's location join, and embedded tag queries.
- Path-`{id}` GET on assets and locations is intentionally unchanged — overrides the predicate so callers with a known id can read any non-deleted row.
- `is_active` remains an independent filter dimension. Embedded tag queries drop their hardcoded `is_active = true` clause to align with that rule.
- OpenAPI descriptions updated to describe the new contract.

## Why
TRA-628 was filed as a docs-only ticket to publish the "currently effective" rule. Audit found the rule was unenforced on every public read path. Publishing the doc as written would have misled ingestion partners. This change makes the rule true before the docs land.

## Test plan
- [x] Unit test for `temporallyEffective` covers asset/location/tag aliases
- [x] Integration: assets list default scope excludes expired + future rows
- [x] Integration: assets GET-by-id returns 200 for any non-deleted row (override)
- [x] Integration: `is_active` remains independently filterable across temporal categories
- [x] Integration: locations list mirrors asset list behavior
- [x] Integration: `/locations/current` excludes temporally-invalid assets and surfaces null location metadata for temporally-invalid joined locations
- [x] Integration: asset history allows expired-asset path lookup but predicate-filters location join
- [x] Integration: embedded tags excluded from asset payload when temporally invalid
- [x] `just validate` passes
- [ ] Black-box verification on preview deployment after merge

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review

Run after the plan is written:

1. **Spec coverage check.** Each decision in the spec maps to a task: helper (Task 1), assets list + Q-search tag subquery (Task 2), locations list + Q-search subquery (Task 3), reports current_locations all three queries (Task 4), asset history (Task 5), embedded tags (Task 6), spec docs (Task 7), validation (Task 8). Out-of-scope items (`?as_of=`, api_keys expiry, sentinel backfill, schema removal) correctly deferred.

2. **Placeholder scan.** No "TBD", "TODO", "implement later", or generic "add error handling" — every code block is real and copyable.

3. **Type consistency.** `temporallyEffective` signature consistent across all call sites. Test helper names (`seedAssetWithWindow`, `seedLocationWithWindow`, etc.) consistent within their packages.

4. **Ambiguity check.** Tag queries: clarified the alias change (FROM `trakrf.tags` → FROM `trakrf.tags i`). Reports queries: explicit which three places (DistinctOn, Timescale, Count) get updated.
