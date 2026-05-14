//go:build integration
// +build integration

// TRA-664 / BB26 D7: locations counterpart to the asset rename tests. The
// dedicated POST /api/v1/locations/{location_id}/rename operation mutates
// only this row's external_key (TRA-684 removed the tree_path materialised
// column and its descendant cascade), but the response still carries
// descendant_count_affected (the live descendant count reachable through
// parent_id) so integrators can decide whether to refresh derived
// natural-key joins.
//
// TRA-686 / BB29 F8: PATCH rejects an `external_key` body field with 400
// read_only naming the rename endpoint — silent-drop under TRA-674 hid
// bugs in read-modify-write integrations. Runtime reject coverage lives
// in TestPatchLocation_ExternalKeyRejected400; this file pins the
// happy-path rename endpoint behavior.
//
// TRA-719 / BB35 B2: parent_external_key, originally also rejected, is
// now writable on PATCH; see patch_natural_key_integration_test.go for
// dispatch coverage.

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

// POST /rename returns 200 with the updated LocationView and a 0 descendant
// count for a leaf rename.
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

	var dbKey string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT external_key FROM trakrf.locations WHERE id = $1`, id).Scan(&dbKey))
	assert.Equal(t, "LOC-LEAF-NEW", dbKey)
}

// POST /rename on a parent returns descendant_count_affected = the live
// descendant count reachable through parent_id (TRA-684: the materialised
// tree_path cascade is gone; only the renamed row is written, but
// integrators still need a signal that downstream natural-key joins for
// the subtree may need refreshing). With root R + 2 children + 1
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

	// Descendants' external_keys are untouched — only the renamed row
	// itself changes after TRA-684 dropped the tree_path cascade.
	rows, err := pool.Query(context.Background(),
		`SELECT external_key, parent_location_id FROM trakrf.locations WHERE org_id = $1 ORDER BY external_key`, orgID)
	require.NoError(t, err)
	defer rows.Close()
	parents := map[string]*int{}
	for rows.Next() {
		var k string
		var p *int
		require.NoError(t, rows.Scan(&k, &p))
		parents[k] = p
	}
	require.NoError(t, rows.Err())
	require.Contains(t, parents, "ROOT-NEW")
	require.Contains(t, parents, "CHILD1")
	require.Contains(t, parents, "CHILD2")
	require.Contains(t, parents, "GCHILD1")
	assert.Nil(t, parents["ROOT-NEW"])
	assert.Equal(t, root, *parents["CHILD1"])
	assert.Equal(t, root, *parents["CHILD2"])
	assert.Equal(t, child1, *parents["GCHILD1"])
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
