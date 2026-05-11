//go:build integration
// +build integration

// TRA-674 / BB27 F1 (Schemathesis Class B): locations parallel surface for
// the NULL-description scan fix. See the assets-side test for the full
// rationale. Adjacent read paths covered: list, get-by-id, ancestors,
// children, descendants — all share the same COALESCE'd row scanner.

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
)

func seedLocationWithNullDescription(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name string) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, NULL, $4, true) RETURNING id
	`, orgID, extKey, name, time.Now().UTC()).Scan(&id))
	return id
}

// GET /api/v1/locations?... returns 200 when the result set includes a
// row with description IS NULL.
func TestListLocations_NullDescription_NoCrash(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	_ = seedLocationWithNullDescription(t, pool, orgID, "LOC-NULL-DESC", "NullDescLoc")

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations", handler.ListLocations)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/locations?limit=50&offset=0&is_active=true&include_deleted=true", nil)
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"list with NULL description row must be 200 (got %d): %s", rec.Code, rec.Body.String())

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data, "expected at least one location")
	for _, l := range resp.Data {
		_, present := l["description"]
		assert.True(t, present, "every location row must include a description field")
	}
}

// GET /api/v1/locations/{id} returns 200 when description IS NULL.
func TestGetLocation_NullDescription_NoCrash(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationWithNullDescription(t, pool, orgID, "LOC-NULL-DESC-GET", "NullDescGet")

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations/{location_id}", handler.GetLocation)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"get with NULL description must be 200 (got %d): %s", rec.Code, rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	_, present := resp.Data["description"]
	assert.True(t, present, "description must be present on the response")
}

// GET /api/v1/locations/{id}/ancestors,children,descendants iterate the
// same nullable column; verifying one parent + one NULL-desc child covers
// every relation-read path in a single hop.
func TestLocationRelations_NullDescription_NoCrash(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationWithNullDescription(t, pool, orgID, "LOC-REL-PARENT", "RelParent")

	// Child: explicit parent_location_id + NULL description.
	var childID int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, parent_location_id, valid_from, is_active)
		VALUES ($1, 'LOC-REL-CHILD', 'RelChild', NULL, $2, $3, true) RETURNING id
	`, orgID, parentID, time.Now().UTC()).Scan(&childID))

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations/{location_id}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{location_id}/children", handler.GetChildren)
	r.Get("/api/v1/locations/{location_id}/descendants", handler.GetDescendants)

	paths := []struct {
		name string
		path string
	}{
		{"ancestors of child", fmt.Sprintf("/api/v1/locations/%d/ancestors", childID)},
		{"children of parent", fmt.Sprintf("/api/v1/locations/%d/children", parentID)},
		{"descendants of parent", fmt.Sprintf("/api/v1/locations/%d/descendants", parentID)},
	}
	for _, tc := range paths {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req = withLocationRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code,
				"%s with NULL description must be 200 (got %d): %s", tc.name, rec.Code, rec.Body.String())
		})
	}
}
