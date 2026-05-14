//go:build integration
// +build integration

// TRA-600: GET /api/v1/locations?external_key= replaces the retired
// /api/v1/locations/lookup endpoint. The filter scopes to the caller's org,
// excludes soft-deleted rows, returns 0 or 1 matches in the standard list
// envelope, and accepts repeat values for any-of semantics.

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

func setupLocFilterRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations", handler.ListLocations)
	return r
}

func withLocFilterOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra600-locfilter@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocationForFilter(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, externalKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

type locFilterResponse struct {
	Data       []locmodel.PublicLocationView `json:"data"`
	TotalCount int                           `json:"total_count"`
}

func doLocFilterRequest(t *testing.T, router *chi.Mux, orgID int, query string) (int, locFilterResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations?"+query, nil)
	req = withLocFilterOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Code, locFilterResponse{}
	}
	var resp locFilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestListLocations_ExternalKey_HappyPath_ReturnsSingleRow(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedLocationForFilter(t, pool, orgID, "wh-1", "Warehouse 1")
	seedLocationForFilter(t, pool, orgID, "wh-2", "Warehouse 2")

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgID, "external_key=wh-1")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "wh-1", resp.Data[0].ExternalKey)
	assert.Equal(t, 1, resp.TotalCount)
}

func TestListLocations_ExternalKey_NoMatch_ReturnsEmptyArray(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedLocationForFilter(t, pool, orgID, "wh-1", "Warehouse 1")

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgID, "external_key=does-not-exist")
	require.Equal(t, http.StatusOK, code, "no match is 200 with empty data, not 404")
	assert.Empty(t, resp.Data)
	assert.Equal(t, 0, resp.TotalCount)
}

func TestListLocations_ExternalKey_SoftDeleted_NotAddressable(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationForFilter(t, pool, orgID, "wh-deleted", "Soon to die")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = now() WHERE id = $1`, id)
	require.NoError(t, err)

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgID, "external_key=wh-deleted")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Data, "soft-deleted rows must not surface through external_key filter")
}

func TestListLocations_ExternalKey_CrossOrg_NotAddressable(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	var orgB int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ('Cross-org B', 'tra600-loc-cross-orgB', true) RETURNING id`,
	).Scan(&orgB))

	seedLocationForFilter(t, pool, orgA, "wh-secret", "Org A only")

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgB, "external_key=wh-secret")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Data)
}

func TestListLocations_ExternalKey_RepeatedValues_AnyOf(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedLocationForFilter(t, pool, orgID, "wh-A", "A")
	seedLocationForFilter(t, pool, orgID, "wh-B", "B")
	seedLocationForFilter(t, pool, orgID, "wh-C", "C")

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgID, "external_key=wh-A&external_key=wh-C")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 2)
	keys := []string{resp.Data[0].ExternalKey, resp.Data[1].ExternalKey}
	assert.Contains(t, keys, "wh-A")
	assert.Contains(t, keys, "wh-C")
}

// TRA-713 / BB33 F5+C2: an external_key filter value that contains a
// slash (or any char outside the strict external_key_pattern) can never
// match a real row, because POST/PATCH reject the same input on the
// write side. The list filter must enforce the same regex at the
// boundary so the violation surfaces as 400 invalid_value rather than a
// silent 200 with empty data.
func TestListLocations_ExternalKey_SlashRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupLocFilterRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations?external_key=abc%2Fdef", nil)
	req = withLocFilterOrgContext(req, orgID)
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

// Sibling check for the parent_external_key filter — same regex, same
// boundary rule.
func TestListLocations_ParentExternalKey_SlashRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupLocFilterRouter(NewHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations?parent_external_key=abc%2Fdef", nil)
	req = withLocFilterOrgContext(req, orgID)
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
	assert.Equal(t, "parent_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}
