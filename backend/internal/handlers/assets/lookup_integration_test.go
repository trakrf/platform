//go:build integration
// +build integration

// TRA-555: GET /api/v1/assets/lookup?external_key= returns a single live
// asset matching the natural key, scoped to the caller's org.

package assets

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
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupLookupRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets/lookup", handler.Lookup)
	return r
}

func withLookupOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra555-lookup@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedAsset(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, externalKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestLookup_HappyPath_ReturnsAsset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedAsset(t, pool, orgID, "WIDGET-7", "Widget 7")

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/lookup?external_key=WIDGET-7", nil)
	req = withLookupOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data assetmodel.PublicAssetView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "WIDGET-7", resp.Data.ExternalKey)
	assert.Equal(t, "Widget 7", resp.Data.Name)
}

func TestLookup_NotFound_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/lookup?external_key=nonexistent", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/lookup", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/lookup?external_key=", nil)
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

	id := seedAsset(t, pool, orgID, "deleted-asset", "To delete")

	// Soft-delete it
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET deleted_at = now() WHERE id = $1`, id)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/lookup?external_key=deleted-asset", nil)
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
		 VALUES ('Cross-org B', 'tra555-cross-orgB', true) RETURNING id`,
	).Scan(&orgB))

	seedAsset(t, pool, orgA, "secret-asset", "Secret")

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	// Caller is orgB, target lives in orgA
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/lookup?external_key=secret-asset", nil)
	req = withLookupOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TRA-579 D-4: duplicate external_key parameters must be rejected with 400
// rather than silently first-wins. The "looks correct, isn't" behavior is
// the worst kind of failure for an LLM-driven integration.
func TestLookup_DuplicateExternalKey_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedAsset(t, pool, orgID, "WIDGET-7", "Widget 7")

	handler := NewHandler(store)
	router := setupLookupRouter(handler)

	cases := []struct {
		name string
		url  string
	}{
		{"two distinct values", "/api/v1/assets/lookup?external_key=WIDGET-7&external_key=NOPE"},
		{"two identical values", "/api/v1/assets/lookup?external_key=WIDGET-7&external_key=WIDGET-7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			req = withLookupOrgContext(req, orgID)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

			var resp struct {
				Error struct {
					Type   string `json:"type"`
					Title  string `json:"title"`
					Detail string `json:"detail"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "bad_request", resp.Error.Type)
			assert.Equal(t, "Bad request", resp.Error.Title)
			assert.Contains(t, resp.Error.Detail, "exactly one of: external_key")
		})
	}
}
