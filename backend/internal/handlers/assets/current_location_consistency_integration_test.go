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

func seedConsistencyLocation(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, valid_from, is_active)
		VALUES ($1, $2, $2, $3, true) RETURNING id
	`, orgID, externalKey, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedConsistencyAsset(t *testing.T, pool *pgxpool.Pool, orgID int, extKey string, locID *int) int {
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

func seedConsistencyScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID, locID int, ts time.Time) {
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

	locID := seedConsistencyLocation(t, pool, orgID, "WHS-01")
	assetID := seedConsistencyAsset(t, pool, orgID, "ASSET-0001", nil)
	seedConsistencyScan(t, pool, orgID, assetID, locID, time.Now().UTC())

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

	assetID := seedConsistencyAsset(t, pool, orgID, "ASSET-NOLOC", nil)

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

	locID := seedConsistencyLocation(t, pool, orgID, "WHS-FK")
	assetID := seedConsistencyAsset(t, pool, orgID, "ASSET-FKONLY", &locID)

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
