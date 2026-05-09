//go:build integration
// +build integration

// TRA-628: /locations/current and /assets/{id}/history must apply
// temporal-validity predicate on entity joins.

package reports

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
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

type currLocResp struct {
	Data       []report.PublicCurrentLocationItem `json:"data"`
	TotalCount int                                `json:"total_count"`
}

type historyResp struct {
	Data       []report.PublicAssetHistoryItem `json:"data"`
	TotalCount int                             `json:"total_count"`
}

func setupTemporalReportsRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations/current", handler.ListCurrentLocations)
	r.Get("/api/v1/assets/{asset_id}/history", handler.GetAssetHistory)
	return r
}

func withReportsOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra628-rep@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedAssetForReports(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom time.Time, validTo *time.Time) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, externalKey, validFrom, validTo).Scan(&id))
	return id
}

func seedLocationForReports(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom time.Time, validTo *time.Time) int {
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

	effectiveAsset := seedAssetForReports(t, pool, orgID, "CL-A-EFF", yesterday, nil)
	effectiveAsset2 := seedAssetForReports(t, pool, orgID, "CL-A-EFF2", yesterday, nil)
	expiredAsset := seedAssetForReports(t, pool, orgID, "CL-A-EXP", weekAgo, &yesterday)

	effectiveLoc := seedLocationForReports(t, pool, orgID, "CL-L-EFF", yesterday, nil)
	expiredLoc := seedLocationForReports(t, pool, orgID, "CL-L-EXP", weekAgo, &yesterday)

	seedScan(t, pool, orgID, effectiveAsset, effectiveLoc, now)
	seedScan(t, pool, orgID, effectiveAsset2, expiredLoc, now)
	seedScan(t, pool, orgID, expiredAsset, effectiveLoc, now)

	handler := NewHandler(store)
	router := setupTemporalReportsRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/current", nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp currLocResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	byKey := make(map[string]report.PublicCurrentLocationItem)
	for _, item := range resp.Data {
		if item.AssetExternalKey != nil {
			byKey[*item.AssetExternalKey] = item
		}
	}

	assert.Contains(t, byKey, "CL-A-EFF", "effective asset with effective location must appear")
	assert.Contains(t, byKey, "CL-A-EFF2", "effective asset with expired location must still appear (LEFT JOIN)")
	assert.NotContains(t, byKey, "CL-A-EXP", "expired asset must be excluded entirely")

	if eff, ok := byKey["CL-A-EFF"]; ok {
		require.NotNil(t, eff.LocationExternalKey)
		assert.Equal(t, "CL-L-EFF", *eff.LocationExternalKey)
	}

	// Expired location: predicate filters the LEFT JOIN, so location_external_key is nil.
	if eff2, ok := byKey["CL-A-EFF2"]; ok {
		assert.Nil(t, eff2.LocationExternalKey, "expired location must not surface external_key")
	}
}

func TestListAssetHistory_TemporalValidity_LocationJoinAppliesPredicate(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	// Use an expired asset to confirm the path-{id} override on the asset side.
	expiredAsset := seedAssetForReports(t, pool, orgID, "H-A-EXP", weekAgo, &yesterday)
	effectiveLoc := seedLocationForReports(t, pool, orgID, "H-L-EFF", yesterday, nil)
	expiredLoc := seedLocationForReports(t, pool, orgID, "H-L-EXP", weekAgo, &yesterday)

	seedScan(t, pool, orgID, expiredAsset, effectiveLoc, now.Add(-2*time.Hour))
	seedScan(t, pool, orgID, expiredAsset, expiredLoc, now.Add(-1*time.Hour))

	handler := NewHandler(store)
	router := setupTemporalReportsRouter(handler)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d/history", expiredAsset), nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "expired asset must be addressable by id (path-{id} override): body=%s", w.Body.String())

	var resp historyResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2, "both scans must surface")

	for _, item := range resp.Data {
		if item.LocationID != nil && *item.LocationID == effectiveLoc {
			require.NotNil(t, item.LocationExternalKey)
			assert.Equal(t, "H-L-EFF", *item.LocationExternalKey)
		}
		if item.LocationID != nil && *item.LocationID == expiredLoc {
			assert.Nil(t, item.LocationExternalKey, "expired location must not surface external_key")
		}
	}
}
