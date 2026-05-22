//go:build integration
// +build integration

// TRA-600: GET /api/v1/assets?external_key= replaces the retired
// /api/v1/assets/lookup endpoint. The filter scopes to the caller's org,
// excludes soft-deleted rows, returns 0 or 1 matches in the standard list
// envelope, and accepts repeat values for any-of semantics.

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

func setupExternalKeyListRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)
	return r
}

func withExternalKeyOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra600-filter@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedAssetForFilter(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, externalKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

type assetFilterResponse struct {
	Data       []assetmodel.PublicAssetView `json:"data"`
	Limit      int                          `json:"limit"`
	Offset     int                          `json:"offset"`
	TotalCount int                          `json:"total_count"`
}

func doFilterRequest(t *testing.T, router *chi.Mux, orgID int, query string) (int, assetFilterResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?"+query, nil)
	req = withExternalKeyOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Code, assetFilterResponse{}
	}
	var resp assetFilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestListAssets_ExternalKey_HappyPath_ReturnsSingleRow(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedAssetForFilter(t, pool, orgID, "WIDGET-7", "Widget 7")
	seedAssetForFilter(t, pool, orgID, "GADGET-3", "Gadget 3")

	router := setupExternalKeyListRouter(NewHandler(store))

	code, resp := doFilterRequest(t, router, orgID, "external_key=WIDGET-7")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "WIDGET-7", resp.Data[0].ExternalKey)
	assert.Equal(t, "Widget 7", resp.Data[0].Name)
	assert.Equal(t, 1, resp.TotalCount)
}

func TestListAssets_ExternalKey_NoMatch_ReturnsEmptyArray(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedAssetForFilter(t, pool, orgID, "WIDGET-7", "Widget 7")

	router := setupExternalKeyListRouter(NewHandler(store))

	code, resp := doFilterRequest(t, router, orgID, "external_key=nonexistent")
	require.Equal(t, http.StatusOK, code, "no match must be 200 with empty data, not 404")
	assert.Empty(t, resp.Data)
	assert.Equal(t, 0, resp.TotalCount)
}

func TestListAssets_ExternalKey_SoftDeleted_NotAddressable(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedAssetForFilter(t, pool, orgID, "DELETED-1", "Soon to die")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET deleted_at = now() WHERE id = $1`, id)
	require.NoError(t, err)

	router := setupExternalKeyListRouter(NewHandler(store))

	code, resp := doFilterRequest(t, router, orgID, "external_key=DELETED-1")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Data, "soft-deleted rows must not surface through external_key filter")
	assert.Equal(t, 0, resp.TotalCount)
}

func TestListAssets_ExternalKey_CrossOrg_NotAddressable(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	var orgB int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ('Cross-org B', 'tra600-cross-orgB', true) RETURNING id`,
	).Scan(&orgB))

	seedAssetForFilter(t, pool, orgA, "SECRET", "Org A only")

	router := setupExternalKeyListRouter(NewHandler(store))

	// Caller is orgB; orgA's asset must not surface.
	code, resp := doFilterRequest(t, router, orgB, "external_key=SECRET")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Data)
}

func TestListAssets_ExternalKey_RepeatedValues_AnyOf(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedAssetForFilter(t, pool, orgID, "A", "Asset A")
	seedAssetForFilter(t, pool, orgID, "B", "Asset B")
	seedAssetForFilter(t, pool, orgID, "C", "Asset C")

	router := setupExternalKeyListRouter(NewHandler(store))

	code, resp := doFilterRequest(t, router, orgID, "external_key=A&external_key=C")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 2)
	keys := []string{resp.Data[0].ExternalKey, resp.Data[1].ExternalKey}
	assert.Contains(t, keys, "A")
	assert.Contains(t, keys, "C")
	assert.Equal(t, 2, resp.TotalCount)
}

// TRA-713 / BB33 F5+C2: an external_key filter value that contains a
// slash (or any char outside the strict external_key_pattern) can never
// match a real row, because POST/PATCH reject the same input on the
// write side. The list filter must enforce the same regex at the
// boundary so the violation surfaces as 400 invalid_value rather than a
// silent 200 with empty data.
func TestListAssets_ExternalKey_SlashRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupExternalKeyListRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?external_key=abc%2Fdef", nil)
	req = withExternalKeyOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.NotEmpty(t, resp.Error.Fields)
	assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}
