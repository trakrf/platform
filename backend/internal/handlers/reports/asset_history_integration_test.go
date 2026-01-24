//go:build integration
// +build integration

// TRA-218: Skipped by default - requires database setup
// Run with: go test -tags=integration ./internal/handlers/reports/...

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

func setupIntegrationRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	handler.RegisterRoutes(r)
	return r
}

// withClaims adds user claims to the request context for testing
func withClaims(r *http.Request, claims *jwt.Claims) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserClaimsKey, claims)
	return r.WithContext(ctx)
}

// testClaims creates test claims with the given org ID
func testClaims(orgID int) *jwt.Claims {
	return &jwt.Claims{
		UserID:       1,
		Email:        "test@example.com",
		CurrentOrgID: &orgID,
	}
}

// createTestAsset creates a test asset and returns its ID
func createTestAsset(t *testing.T, pool *pgxpool.Pool, orgID int, name string) int {
	t.Helper()
	ctx := context.Background()

	var assetID int
	err := pool.QueryRow(ctx, `
		INSERT INTO trakrf.assets (org_id, identifier, name, type, is_active)
		VALUES ($1, $2, $3, 'equipment', true)
		RETURNING id
	`, orgID, "TEST-"+name, name).Scan(&assetID)

	require.NoError(t, err)
	return assetID
}

// createTestLocation creates a test location and returns its ID
func createTestLocation(t *testing.T, pool *pgxpool.Pool, orgID int, name string) int {
	t.Helper()
	ctx := context.Background()

	var locationID int
	err := pool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, identifier, name, is_active)
		VALUES ($1, $2, $3, true)
		RETURNING id
	`, orgID, "LOC-"+name, name).Scan(&locationID)

	require.NoError(t, err)
	return locationID
}

// createTestScan creates a test asset scan
func createTestScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, locationID *int, timestamp time.Time) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO trakrf.asset_scans (org_id, asset_id, location_id, timestamp)
		VALUES ($1, $2, $3, $4)
	`, orgID, assetID, locationID, timestamp)

	require.NoError(t, err)
}

func TestGetAssetHistory_Integration_Success(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)

	// Create test org
	orgID := testutil.CreateTestAccount(t, pool)

	// Create test asset and location
	assetID := createTestAsset(t, pool, orgID, "Test Asset")
	locationID := createTestLocation(t, pool, orgID, "Warehouse A")

	// Create test scans at known timestamps
	now := time.Now().UTC()
	scan1Time := now.Add(-2 * time.Hour)
	scan2Time := now.Add(-1 * time.Hour)

	createTestScan(t, pool, orgID, assetID, &locationID, scan1Time)
	createTestScan(t, pool, orgID, assetID, &locationID, scan2Time)

	handler := NewHandler(store)
	router := setupIntegrationRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/"+itoa(assetID)+"/history", nil)
	req = withClaims(req, testClaims(orgID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response report.AssetHistoryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, assetID, response.Asset.ID)
	assert.Equal(t, "Test Asset", response.Asset.Name)
	assert.Equal(t, 2, len(response.Data))
	assert.Equal(t, 2, response.TotalCount)
}

func TestGetAssetHistory_Integration_AssetNotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	handler := NewHandler(store)
	router := setupIntegrationRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/99999/history", nil)
	req = withClaims(req, testClaims(orgID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	errorObj := response["error"].(map[string]any)
	assert.Equal(t, "not_found", errorObj["type"])
}

func TestGetAssetHistory_Integration_WrongOrg(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)

	// Create two orgs
	org1ID := testutil.CreateTestAccount(t, pool)

	// Create second org manually
	var org2ID int
	ctx := context.Background()
	err := pool.QueryRow(ctx, `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Other Organization", "other-org", true).Scan(&org2ID)
	require.NoError(t, err)

	// Create asset in org1
	assetID := createTestAsset(t, pool, org1ID, "Org1 Asset")

	handler := NewHandler(store)
	router := setupIntegrationRouter(handler)

	// Request with org2 claims should not see org1's asset
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/"+itoa(assetID)+"/history", nil)
	req = withClaims(req, testClaims(org2ID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetAssetHistory_Integration_NoScans(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// Create asset with no scans
	assetID := createTestAsset(t, pool, orgID, "Empty Asset")

	handler := NewHandler(store)
	router := setupIntegrationRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/"+itoa(assetID)+"/history", nil)
	req = withClaims(req, testClaims(orgID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response report.AssetHistoryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, assetID, response.Asset.ID)
	assert.Equal(t, 0, len(response.Data))
	assert.Equal(t, 0, response.TotalCount)
}

func TestGetAssetHistory_Integration_DateRangeFilter(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	assetID := createTestAsset(t, pool, orgID, "Filtered Asset")
	locationID := createTestLocation(t, pool, orgID, "Test Location")

	now := time.Now().UTC()

	// Create scans: one 60 days ago, one 5 days ago
	oldScan := now.Add(-60 * 24 * time.Hour)
	recentScan := now.Add(-5 * 24 * time.Hour)

	createTestScan(t, pool, orgID, assetID, &locationID, oldScan)
	createTestScan(t, pool, orgID, assetID, &locationID, recentScan)

	handler := NewHandler(store)
	router := setupIntegrationRouter(handler)

	// Default 30-day range should only return the recent scan
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/"+itoa(assetID)+"/history", nil)
	req = withClaims(req, testClaims(orgID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response report.AssetHistoryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, 1, response.TotalCount)
}

func TestGetAssetHistory_Integration_Pagination(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	assetID := createTestAsset(t, pool, orgID, "Paginated Asset")
	locationID := createTestLocation(t, pool, orgID, "Test Location")

	now := time.Now().UTC()

	// Create 5 scans
	for i := 0; i < 5; i++ {
		scanTime := now.Add(-time.Duration(i) * time.Hour)
		createTestScan(t, pool, orgID, assetID, &locationID, scanTime)
	}

	handler := NewHandler(store)
	router := setupIntegrationRouter(handler)

	// Get first 2
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/"+itoa(assetID)+"/history?limit=2&offset=0", nil)
	req = withClaims(req, testClaims(orgID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response report.AssetHistoryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, len(response.Data))
	assert.Equal(t, 5, response.TotalCount)
	assert.Equal(t, 0, response.Offset)
}

func TestGetAssetHistory_Integration_DurationCalculation(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	assetID := createTestAsset(t, pool, orgID, "Duration Asset")
	locationID := createTestLocation(t, pool, orgID, "Test Location")

	now := time.Now().UTC()

	// Create scans exactly 1 hour apart
	scan1 := now.Add(-2 * time.Hour)
	scan2 := now.Add(-1 * time.Hour)

	createTestScan(t, pool, orgID, assetID, &locationID, scan1)
	createTestScan(t, pool, orgID, assetID, &locationID, scan2)

	handler := NewHandler(store)
	router := setupIntegrationRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/"+itoa(assetID)+"/history", nil)
	req = withClaims(req, testClaims(orgID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response report.AssetHistoryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, len(response.Data))

	// Most recent scan (first in DESC order) should have nil duration
	assert.Nil(t, response.Data[0].DurationSeconds)

	// Older scan should have duration of ~3600 seconds (1 hour)
	assert.NotNil(t, response.Data[1].DurationSeconds)
	// Allow for small timing variations
	assert.InDelta(t, 3600, *response.Data[1].DurationSeconds, 10)
}

func itoa(i int) string {
	return string(rune('0'+i/10000%10)) + string(rune('0'+i/1000%10)) + string(rune('0'+i/100%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10))
}
