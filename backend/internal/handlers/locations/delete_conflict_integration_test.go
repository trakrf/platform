//go:build integration
// +build integration

// TRA-644 / BB22 F2: DELETE /api/v1/locations/{id} returns 409 conflict when
// the location has descendant locations or assets placed directly at it.
// Bulk cascade is not supported in v1 — descendants must be reassigned and
// placed assets moved before the parent location can be deleted.

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
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupDeleteConflictRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Delete("/api/v1/locations/{location_id}", handler.Delete)
	return r
}

func withDeleteConflictOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra644@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocationDC(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string, parentID *int) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active, parent_location_id)
		VALUES ($1, $2, $3, '', $4, true, $5) RETURNING id
	`, orgID, externalKey, name, time.Now().UTC(), parentID).Scan(&id)
	require.NoError(t, err)
	return id
}

// seedAssetAtLocation seeds a live asset and, when locationID is non-nil, an
// asset_scans row placing it at that location. TRA-799: the location-delete
// guard counts assets by their latest scan location, not a denormalized
// column.
func seedAssetAtLocation(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, locationID *int) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets
			(org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $2, '', $3, true) RETURNING id
	`, orgID, externalKey, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	if locationID != nil {
		_, err = pool.Exec(context.Background(), `
			INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
			VALUES ($1, $2, $3, $4)
		`, time.Now().UTC(), orgID, id, *locationID)
		require.NoError(t, err)
	}
	return id
}

type errResp struct {
	Error struct {
		Type   string `json:"type"`
		Detail string `json:"detail"`
	} `json:"error"`
}

// Descendants present → 409 conflict, distinct detail referencing descendants.
func TestDeleteLocation_WithDescendants_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationDC(t, pool, orgID, "wh-parent-desc", "Parent", nil)
	_ = seedLocationDC(t, pool, orgID, "wh-child-desc", "Child", &parentID)

	router := setupDeleteConflictRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", parentID), nil)
	req = withDeleteConflictOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code,
		"DELETE on parent with active children must be 409 (got %d): %s", rec.Code, rec.Body.String())

	var resp errResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
	assert.Contains(t, resp.Error.Detail, "descendant",
		"detail should mention descendants so integrators know to reassign children")

	// Parent must still be present (not soft-deleted by the failed call).
	var deletedAt *time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT deleted_at FROM trakrf.locations WHERE id = $1`, parentID).Scan(&deletedAt))
	assert.Nil(t, deletedAt, "parent location must remain undeleted after 409")
}

// Placed assets present → 409 conflict, distinct detail referencing assets.
func TestDeleteLocation_WithPlacedAssets_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := seedLocationDC(t, pool, orgID, "wh-leaf-asset", "Leaf-with-asset", nil)
	_ = seedAssetAtLocation(t, pool, orgID, "asset-at-leaf-1", &locID)

	router := setupDeleteConflictRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", locID), nil)
	req = withDeleteConflictOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code,
		"DELETE on location with placed assets must be 409 (got %d): %s", rec.Code, rec.Body.String())

	var resp errResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
	assert.Contains(t, resp.Error.Detail, "assets",
		"detail should mention assets so integrators know to move them")

	var deletedAt *time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT deleted_at FROM trakrf.locations WHERE id = $1`, locID).Scan(&deletedAt))
	assert.Nil(t, deletedAt, "location must remain undeleted after 409")
}

// True leaf — no descendants, no placed assets → 204.
func TestDeleteLocation_TrueLeaf_Returns204(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := seedLocationDC(t, pool, orgID, "wh-true-leaf", "TrueLeaf", nil)

	router := setupDeleteConflictRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", locID), nil)
	req = withDeleteConflictOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code,
		"DELETE on true leaf must be 204 (got %d): %s", rec.Code, rec.Body.String())

	var deletedAt *time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT deleted_at FROM trakrf.locations WHERE id = $1`, locID).Scan(&deletedAt))
	assert.NotNil(t, deletedAt, "location must be soft-deleted on 204")
}

// Soft-deleted descendants do NOT count against the parent — only active
// children block delete.
func TestDeleteLocation_SoftDeletedDescendant_DoesNotBlock(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationDC(t, pool, orgID, "wh-parent-sd", "Parent-sd", nil)
	childID := seedLocationDC(t, pool, orgID, "wh-child-sd", "Child-sd", &parentID)
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = NOW() WHERE id = $1`, childID)
	require.NoError(t, err)

	router := setupDeleteConflictRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", parentID), nil)
	req = withDeleteConflictOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code,
		"DELETE on parent whose only descendant was soft-deleted must be 204 (got %d): %s",
		rec.Code, rec.Body.String())
}

// TRA-799: the guard follows the LATEST scan. An asset whose most recent scan
// moved it to another location no longer blocks deletion of the location it
// was first scanned into.
func TestDeleteLocation_AssetScannedAway_DoesNotBlock(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locA := seedLocationDC(t, pool, orgID, "wh-from", "From", nil)
	locB := seedLocationDC(t, pool, orgID, "wh-to", "To", nil)

	// Asset's first scan is at locA; a later scan moves it to locB.
	assetID := seedAssetAtLocation(t, pool, orgID, "asset-moved", &locA)
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
		VALUES ($1, $2, $3, $4)
	`, time.Now().UTC().Add(time.Hour), orgID, assetID, locB)
	require.NoError(t, err)

	router := setupDeleteConflictRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", locA), nil)
	req = withDeleteConflictOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code,
		"DELETE on the prior location of an asset since scanned elsewhere must be 204 (got %d): %s",
		rec.Code, rec.Body.String())
}

// Soft-deleted placed assets do NOT count against the location.
func TestDeleteLocation_SoftDeletedAsset_DoesNotBlock(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := seedLocationDC(t, pool, orgID, "wh-leaf-sda", "LeafSDA", nil)
	assetID := seedAssetAtLocation(t, pool, orgID, "asset-soft-deleted", &locID)
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET deleted_at = NOW() WHERE id = $1`, assetID)
	require.NoError(t, err)

	router := setupDeleteConflictRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", locID), nil)
	req = withDeleteConflictOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code,
		"DELETE on location whose only placed asset was soft-deleted must be 204 (got %d): %s",
		rec.Code, rec.Body.String())
}
