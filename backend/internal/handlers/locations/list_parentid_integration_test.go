//go:build integration
// +build integration

// TRA-579 D-10: GET /api/v1/locations accepts parent_id (canonical) as a
// filter, mutually exclusive with parent_external_key.

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

func setupListRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations", handler.ListLocations)
	return r
}

func withListOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra579-list@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocationWithParent(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string, parentID *int) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active, parent_location_id)
		VALUES ($1, $2, $3, '', $4, true, $5) RETURNING id
	`, orgID, externalKey, name, time.Now().UTC(), parentID).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestListLocations_ParentID_FiltersChildren(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationWithParent(t, pool, orgID, "wh-1", "Warehouse 1", nil)
	seedLocationWithParent(t, pool, orgID, "wh-1-aisle-a", "Aisle A", &parentID)
	seedLocationWithParent(t, pool, orgID, "wh-1-aisle-b", "Aisle B", &parentID)
	seedLocationWithParent(t, pool, orgID, "wh-2", "Warehouse 2", nil)

	handler := NewHandler(store)
	router := setupListRouter(handler)

	url := fmt.Sprintf("/api/v1/locations?parent_id=%d", parentID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withListOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data       []map[string]any `json:"data"`
		TotalCount int              `json:"total_count"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalCount)
	got := map[string]bool{}
	for _, item := range resp.Data {
		got[item["external_key"].(string)] = true
	}
	assert.True(t, got["wh-1-aisle-a"])
	assert.True(t, got["wh-1-aisle-b"])
}

func TestListLocations_ParentIDAndParentExternalKey_Mutex(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupListRouter(handler)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/locations?parent_id=1&parent_external_key=wh-1", nil)
	req = withListOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
			Fields []struct {
				Field   string `json:"field"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	assert.Equal(t, "Validation failed", resp.Error.Title)
	assert.Contains(t, resp.Error.Detail, "mutually exclusive")
}

func TestListLocations_ParentID_NonInteger_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupListRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations?parent_id=abc", nil)
	req = withListOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field   string `json:"field"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.NotEmpty(t, resp.Error.Fields)
	assert.Equal(t, "parent_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}
