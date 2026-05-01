//go:build integration
// +build integration

// TRA-554: GET /api/v1/locations/lookup?external_key= returns a single live
// location matching the natural key, scoped to the caller's org.

package locations

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
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupLookupRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations/lookup", handler.Lookup)
	return r
}

func withLookupOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra554-lookup@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocation(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, externalKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestLookup_HappyPath_ReturnsLocation(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedLocation(t, pool, orgID, "wh-1", "Warehouse 1")

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/lookup?external_key=wh-1", nil)
	req = withLookupOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data locmodel.PublicLocationView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "wh-1", resp.Data.ExternalKey)
	assert.Equal(t, "Warehouse 1", resp.Data.Name)
}

func TestLookup_NotFound_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/lookup?external_key=nonexistent", nil)
	req = withLookupOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLookup_NoParams_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/lookup", nil)
	req = withLookupOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLookup_EmptyParam_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/lookup?external_key=", nil)
	req = withLookupOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLookup_LiveOnly_SoftDeletedReturns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocation(t, pool, orgID, "deleted-loc", "To delete")

	// Soft-delete it
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = now() WHERE id = $1`, id)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/lookup?external_key=deleted-loc", nil)
	req = withLookupOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLookup_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	var orgB int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ('Cross-org B', 'tra554-cross-orgB', true) RETURNING id`,
	).Scan(&orgB))

	seedLocation(t, pool, orgA, "secret-warehouse", "Secret")

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	// Caller is orgB, target lives in orgA
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/lookup?external_key=secret-warehouse", nil)
	req = withLookupOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
