//go:build integration
// +build integration

// TRA-664 / BB26 D7: locations counterpart to the asset rename tests.
// external_key is immutable on PATCH; the dedicated POST /rename operation
// regenerates tree_path for the row and every descendant, and returns
// descendant_count_affected so an integrator can decide whether to
// re-fetch the subtree.

package locations

import (
	"bytes"
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

func setupRenameLocationRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Patch("/api/v1/locations/{location_id}", handler.Update)
	r.Post("/api/v1/locations/{location_id}/rename", handler.Rename)
	return r
}

// seedLocationRoundTripWithParent matches seedLocationRoundTrip but accepts
// an optional parent so the rename cascade tests can construct a tree.
func seedLocationRoundTripWithParent(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name string, parent *int) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, parent_location_id, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, $5, true) RETURNING id
	`, orgID, extKey, name, parent, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

// PATCH must reject any body containing external_key with code=immutable_field
// and a detail pointing at the rename operation.
func TestPatchLocation_ExternalKeyImmutable_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-IMMUT", "ImmutLoc")

	handler := NewHandler(store)
	r := setupRenameLocationRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"explicit value", `{"external_key":"LOC-RENAMED"}`},
		{"explicit null", `{"external_key":null}`},
		{"with other fields", `{"name":"x","external_key":"LOC-OTHER"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch,
				fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"PATCH with external_key must be 400 (got %d): %s", rec.Code, rec.Body.String())

			var resp struct {
				Error struct {
					Type   string `json:"type"`
					Detail string `json:"detail"`
					Fields []struct {
						Field   string `json:"field"`
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"fields"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "validation_error", resp.Error.Type)
			require.Len(t, resp.Error.Fields, 1)
			assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
			assert.Equal(t, "immutable_field", resp.Error.Fields[0].Code)
			assert.Contains(t, resp.Error.Detail, "rename")
			assert.Contains(t, resp.Error.Fields[0].Message, "rename")
		})
	}
}

// POST /rename returns 200 with the updated LocationView and a 0 descendant
// count for a leaf rename. tree_path on the renamed row reflects the new
// canonical form.
func TestRenameLocation_Leaf_Success(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-LEAF-OLD", "Leaf")

	handler := NewHandler(store)
	r := setupRenameLocationRouter(handler)

	body := []byte(`{"external_key":"LOC-LEAF-NEW"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/locations/%d/rename", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "rename must be 200: %s", rec.Body.String())

	var resp struct {
		Data                    map[string]any `json:"data"`
		DescendantCountAffected int            `json:"descendant_count_affected"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "LOC-LEAF-NEW", resp.Data["external_key"])
	assert.Equal(t, 0, resp.DescendantCountAffected, "leaf rename has zero descendants")

	// tree_path on the row reflects the canonical lowercase + underscore form.
	var dbPath string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT path::text FROM trakrf.locations WHERE id = $1`, id).Scan(&dbPath))
	assert.Equal(t, "loc_leaf_new", dbPath)
}

// POST /rename on a parent regenerates tree_path for every descendant in
// one transaction and returns the count. With root R + 2 children + 1
// grandchild, renaming R must surface descendant_count_affected = 3.
func TestRenameLocation_Cascade_CountsDescendants(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	root := seedLocationRoundTripWithParent(t, pool, orgID, "ROOT-OLD", "Root", nil)
	child1 := seedLocationRoundTripWithParent(t, pool, orgID, "CHILD1", "Child1", &root)
	_ = seedLocationRoundTripWithParent(t, pool, orgID, "CHILD2", "Child2", &root)
	_ = seedLocationRoundTripWithParent(t, pool, orgID, "GCHILD1", "Grandchild1", &child1)

	handler := NewHandler(store)
	r := setupRenameLocationRouter(handler)

	body := []byte(`{"external_key":"ROOT-NEW"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/locations/%d/rename", root), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "rename must be 200: %s", rec.Body.String())

	var resp struct {
		Data                    map[string]any `json:"data"`
		DescendantCountAffected int            `json:"descendant_count_affected"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ROOT-NEW", resp.Data["external_key"])
	assert.Equal(t, 3, resp.DescendantCountAffected,
		"root has 3 descendants (2 children + 1 grandchild)")

	// Every descendant's tree_path now starts with the new canonical root segment.
	rows, err := pool.Query(context.Background(),
		`SELECT external_key, path::text FROM trakrf.locations WHERE org_id = $1 ORDER BY external_key`, orgID)
	require.NoError(t, err)
	defer rows.Close()
	paths := map[string]string{}
	for rows.Next() {
		var k, p string
		require.NoError(t, rows.Scan(&k, &p))
		paths[k] = p
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, "root_new", paths["ROOT-NEW"])
	assert.Equal(t, "root_new.child1", paths["CHILD1"])
	assert.Equal(t, "root_new.child2", paths["CHILD2"])
	assert.Equal(t, "root_new.child1.gchild1", paths["GCHILD1"])
}

// Duplicate external_key within the org → 409 conflict.
func TestRenameLocation_Duplicate_Conflict409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	_ = seedLocationRoundTrip(t, pool, orgID, "LOC-EXISTS", "Existing")
	otherID := seedLocationRoundTrip(t, pool, orgID, "LOC-OTHER", "Other")

	handler := NewHandler(store)
	r := setupRenameLocationRouter(handler)

	body := []byte(`{"external_key":"LOC-EXISTS"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/locations/%d/rename", otherID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code, "duplicate external_key must be 409: %s", rec.Body.String())

	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
}

// POST /rename with the same value: idempotent, returns descendant_count_affected=0
// without firing the trigger.
func TestRenameLocation_SameValue_NoOp200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	root := seedLocationRoundTripWithParent(t, pool, orgID, "ROOT-SAME", "Root", nil)
	_ = seedLocationRoundTripWithParent(t, pool, orgID, "CHILD-SAME", "Child", &root)

	handler := NewHandler(store)
	r := setupRenameLocationRouter(handler)

	body := []byte(`{"external_key":"ROOT-SAME"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/locations/%d/rename", root), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "same-value rename must be 200: %s", rec.Body.String())

	var resp struct {
		DescendantCountAffected int `json:"descendant_count_affected"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.DescendantCountAffected,
		"same-value rename reports 0 affected descendants even when descendants exist")
}
