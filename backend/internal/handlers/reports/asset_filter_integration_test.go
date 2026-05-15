//go:build integration
// +build integration

// TRA-735 / BB40 F4: /reports/asset-locations must accept asset_id and
// asset_external_key as repeatable filters and enforce
// asset_id ⊕ asset_external_key mutual exclusivity. These are the natural
// inverse of the existing location filters: with the master-data /
// scan-data bifurcation landed in TRA-734, the canonical integrator
// query for "I have a list of asset external_keys from my ERP, where is
// each one?" needs single-request expressibility.

package reports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/testutil"
)

type errorFieldsResp struct {
	Error struct {
		Type   string `json:"type"`
		Fields []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"fields"`
	} `json:"error"`
}

func seedAssetActive(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string) int {
	t.Helper()
	now := time.Now().UTC().Add(-24 * time.Hour)
	return seedAssetForReports(t, pool, orgID, externalKey, now, nil)
}

func seedLocationActive(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string) int {
	t.Helper()
	now := time.Now().UTC().Add(-24 * time.Hour)
	return seedLocationForReports(t, pool, orgID, externalKey, now, nil)
}

func doListCurrent(t *testing.T, handler *Handler, orgID int, query string) (int, currLocResp) {
	t.Helper()
	router := setupTemporalReportsRouter(handler)
	url := "/api/v1/reports/asset-locations"
	if query != "" {
		url = url + "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp currLocResp
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	}
	return w.Code, resp
}

func seedThreeAssetsAtTwoLocations(t *testing.T, pool *pgxpool.Pool, orgID int) (assets map[string]int, locations map[string]int) {
	t.Helper()
	assets = map[string]int{
		"AST-0001": seedAssetActive(t, pool, orgID, "AST-0001"),
		"AST-0002": seedAssetActive(t, pool, orgID, "AST-0002"),
		"AST-0003": seedAssetActive(t, pool, orgID, "AST-0003"),
	}
	locations = map[string]int{
		"LOC-A": seedLocationActive(t, pool, orgID, "LOC-A"),
		"LOC-B": seedLocationActive(t, pool, orgID, "LOC-B"),
	}
	now := time.Now().UTC()
	seedScan(t, pool, orgID, assets["AST-0001"], locations["LOC-A"], now.Add(-3*time.Minute))
	seedScan(t, pool, orgID, assets["AST-0002"], locations["LOC-A"], now.Add(-2*time.Minute))
	seedScan(t, pool, orgID, assets["AST-0003"], locations["LOC-B"], now.Add(-1*time.Minute))
	return assets, locations
}

func collectAssetKeys(items []report.PublicCurrentLocationItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.AssetExternalKey)
	}
	return out
}

func TestListCurrentLocations_AssetExternalKey_FiltersSingle(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedThreeAssetsAtTwoLocations(t, pool, orgID)
	handler := NewHandler(store)

	code, resp := doListCurrent(t, handler, orgID, "asset_external_key=AST-0002")
	require.Equal(t, http.StatusOK, code)
	require.ElementsMatch(t, []string{"AST-0002"}, collectAssetKeys(resp.Data))
	assert.Equal(t, 1, resp.TotalCount)
}

func TestListCurrentLocations_AssetExternalKey_FiltersBatch(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedThreeAssetsAtTwoLocations(t, pool, orgID)
	handler := NewHandler(store)

	// The canonical batch-lookup integrator pattern: master system hands
	// us a list of external_keys, we resolve current locations in one
	// round-trip.
	code, resp := doListCurrent(t, handler, orgID, "asset_external_key=AST-0001&asset_external_key=AST-0003")
	require.Equal(t, http.StatusOK, code)
	require.ElementsMatch(t, []string{"AST-0001", "AST-0003"}, collectAssetKeys(resp.Data))
	assert.Equal(t, 2, resp.TotalCount)
}

func TestListCurrentLocations_AssetID_FiltersBatch(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assets, _ := seedThreeAssetsAtTwoLocations(t, pool, orgID)
	handler := NewHandler(store)

	query := fmt.Sprintf("asset_id=%d&asset_id=%d", assets["AST-0001"], assets["AST-0002"])
	code, resp := doListCurrent(t, handler, orgID, query)
	require.Equal(t, http.StatusOK, code)
	require.ElementsMatch(t, []string{"AST-0001", "AST-0002"}, collectAssetKeys(resp.Data))
	assert.Equal(t, 2, resp.TotalCount)
}

func TestListCurrentLocations_AssetExternalKey_IntersectWithLocation(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedThreeAssetsAtTwoLocations(t, pool, orgID)
	handler := NewHandler(store)

	// Two assets are at LOC-A (AST-0001, AST-0002); AST-0003 is at LOC-B.
	// Asking for AST-0001 + AST-0003 intersected with LOC-A must return
	// just AST-0001.
	code, resp := doListCurrent(t, handler, orgID,
		"asset_external_key=AST-0001&asset_external_key=AST-0003&location_external_key=LOC-A")
	require.Equal(t, http.StatusOK, code)
	require.ElementsMatch(t, []string{"AST-0001"}, collectAssetKeys(resp.Data))
	assert.Equal(t, 1, resp.TotalCount)
}

func TestListCurrentLocations_AssetExternalKey_NoMatchReturnsEmpty(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedThreeAssetsAtTwoLocations(t, pool, orgID)
	handler := NewHandler(store)

	code, resp := doListCurrent(t, handler, orgID, "asset_external_key=NONEXISTENT-Z")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Data)
	assert.Equal(t, 0, resp.TotalCount)
}

func TestListCurrentLocations_BothAssetForms_Rejected400AmbiguousFields(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTemporalReportsRouter(handler)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/asset-locations?asset_id=42&asset_external_key=AST-0001", nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	var resp errorFieldsResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 2)
	for _, fld := range resp.Error.Fields {
		assert.Equal(t, "ambiguous_fields", fld.Code)
		assert.Contains(t, []string{"asset_id", "asset_external_key"}, fld.Field)
	}
}

func TestListCurrentLocations_AssetExternalKey_SlashRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupTemporalReportsRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/asset-locations?asset_external_key=abc%2Fdef", nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp errorFieldsResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.NotEmpty(t, resp.Error.Fields)
	assert.Equal(t, "asset_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}

func TestListCurrentLocations_AssetID_InvalidValue400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupTemporalReportsRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/asset-locations?asset_id=not-a-number", nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp errorFieldsResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.NotEmpty(t, resp.Error.Fields)
	assert.Equal(t, "asset_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}

// Guard rail: a query with no filters still returns all live assets —
// confirms the new SQL predicates are no-ops when their args are NULL.
func TestListCurrentLocations_NoFilters_Unaffected(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedThreeAssetsAtTwoLocations(t, pool, orgID)
	handler := NewHandler(store)

	code, resp := doListCurrent(t, handler, orgID, "")
	require.Equal(t, http.StatusOK, code)
	require.ElementsMatch(t,
		[]string{"AST-0001", "AST-0002", "AST-0003"},
		collectAssetKeys(resp.Data))
	assert.Equal(t, 3, resp.TotalCount)
}
