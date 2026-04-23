//go:build integration
// +build integration

package reports_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/reports"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

var reportsUserCounter int64

func seedReportsOrgAndKey(t *testing.T, pool *pgxpool.Pool, store *storage.Storage, name string, scopes []string) (int, string) {
	t.Helper()

	var orgID int
	if name == "" {
		orgID = testutil.CreateTestAccount(t, pool)
	} else {
		err := pool.QueryRow(context.Background(),
			`INSERT INTO trakrf.organizations (name, identifier) VALUES ($1, $2) RETURNING id`,
			name, name,
		).Scan(&orgID)
		require.NoError(t, err)
	}

	n := atomic.AddInt64(&reportsUserCounter, 1)
	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash)
         VALUES ('t', $1, 'stub') RETURNING id`,
		fmt.Sprintf("reports-user-%d@t.com", n),
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "k", scopes, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	tok, err := jwt.GenerateAPIKey(key.JTI, orgID, scopes, nil)
	require.NoError(t, err)

	return orgID, tok
}

func buildReportsPublicRouter(store *storage.Storage) *chi.Mux {
	handler := reports.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("scans:read")).Get("/api/v1/locations/current", handler.ListCurrentLocations)
		r.With(middleware.RequireScope("scans:read")).Get("/api/v1/assets/{identifier}/history", handler.GetAssetHistory)
	})
	return r
}

func insertAssetScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID, locationID int, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, identifier_scan_id)
         VALUES ($1, $2, $3, $4, NULL, NULL)`,
		ts, orgID, assetID, locationID,
	)
	require.NoError(t, err)
}

func TestListCurrentLocations_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-reports")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedReportsOrgAndKey(t, pool, store, "", []string{"scans:read"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "curr-loc-1",
		Name:       "Current Location 1",
		Path:       "curr-loc-1",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	a, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgID,
		Identifier: "curr-asset-1",
		Name:       "Current Asset 1",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	// Insert a tag identifier so the asset field in the response is populated
	_, err = pool.Exec(context.Background(),
		`INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
         VALUES ($1, 'barcode', $2, $3, true)`,
		orgID, "CURR-TAG-001", a.ID,
	)
	require.NoError(t, err)

	insertAssetScan(t, pool, orgID, a.ID, loc.ID, time.Now())

	r := buildReportsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/current", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 1, body["total_count"])

	data := body["data"].([]any)
	require.Len(t, data, 1)
	row := data[0].(map[string]any)
	// "asset" is the asset's natural-key identifier per the public API contract.
	assert.Equal(t, "curr-asset-1", row["asset"])
	assert.Equal(t, "curr-loc-1", row["location"])
	assert.Contains(t, row, "last_seen")
}

func TestGetAssetHistory_ByIdentifier(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-reports")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedReportsOrgAndKey(t, pool, store, "", []string{"scans:read"})

	loc1, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "hist-loc-1",
		Name:       "History Location 1",
		Path:       "hist-loc-1",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	loc2, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "hist-loc-2",
		Name:       "History Location 2",
		Path:       "hist-loc-2",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	a, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgID,
		Identifier: "hist-asset-1",
		Name:       "History Asset 1",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	now := time.Now()
	insertAssetScan(t, pool, orgID, a.ID, loc1.ID, now.Add(-2*time.Hour))
	insertAssetScan(t, pool, orgID, a.ID, loc2.ID, now.Add(-1*time.Hour))

	r := buildReportsPublicRouter(store)

	from := now.Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	url := fmt.Sprintf("/api/v1/assets/hist-asset-1/history?from=%s", from)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 2, body["total_count"])
	assert.Contains(t, body, "limit")
	assert.Contains(t, body, "offset")

	data := body["data"].([]any)
	require.Len(t, data, 2)

	// Verify location natural keys are present
	locations := make(map[string]bool)
	for _, item := range data {
		row := item.(map[string]any)
		assert.Contains(t, row, "timestamp")
		loc, ok := row["location"].(string)
		assert.True(t, ok)
		locations[loc] = true
	}
	assert.True(t, locations["hist-loc-1"] || locations["hist-loc-2"],
		"expected at least one of hist-loc-1 or hist-loc-2 in history")
}

// TestSessionJWT_PassesThroughRequireScope codifies the policy that a valid
// session JWT with no api-key scopes reaches both scan-class endpoints
// (/api/v1/locations/current, /api/v1/assets/{identifier}/history) through
// EitherAuth + RequireScope. RequireScope is intentionally an API-key-only
// gate; session access is governed elsewhere. A future refactor that tightens
// session checks inside RequireScope would silently regress the frontend.
func TestSessionJWT_PassesThroughRequireScope(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-reports")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// Seed minimum data so the handlers don't bail on empty results.
	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "sess-loc", Name: "SL", Path: "sess-loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	a, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "sess-asset", Name: "SA", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	insertAssetScan(t, pool, orgID, a.ID, loc.ID, time.Now())

	sessToken, err := jwt.Generate(1, "sess-passthrough@t.com", &orgID)
	require.NoError(t, err)

	r := buildReportsPublicRouter(store)

	// /locations/current — scans:read gate; session must pass through.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/current", nil)
	req.Header.Set("Authorization", "Bearer "+sessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code,
		"session JWT must pass through RequireScope on /locations/current; got %d %s",
		w.Code, w.Body.String())

	// /assets/{identifier}/history — scans:read gate; session must pass through.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/assets/sess-asset/history", nil)
	req.Header.Set("Authorization", "Bearer "+sessToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code,
		"session JWT must pass through RequireScope on /assets/{identifier}/history; got %d %s",
		w.Code, w.Body.String())
}
