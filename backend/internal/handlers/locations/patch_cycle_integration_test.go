//go:build integration
// +build integration

// TRA-770 / BB58 F1+F2: PATCH /api/v1/locations/{id} must reject any
// parent_id assignment that would create a cycle in the parent_location_id
// chain — both 1-hop (self-parent) and N-hop (transitive) — with a 409
// conflict carrying a specific actionable detail. Valid reparenting and
// root-promotion must still succeed.

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
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupCycleRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Patch("/api/v1/locations/{location_id}", handler.Update)
	return r
}

func withCycleOrgCtx(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra770@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocationCycle(t *testing.T, pool *pgxpool.Pool, orgID int, key, name string, parentID *int) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active, parent_location_id)
		VALUES ($1, $2, $3, '', $4, true, $5) RETURNING id
	`, orgID, key, name, time.Now().UTC(), parentID).Scan(&id)
	require.NoError(t, err)
	return id
}

func patchLocationCycle(t *testing.T, router *chi.Mux, orgID, id int, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	req = withCycleOrgCtx(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// 1-hop: PATCH location X with parent_id=X. Rejected as a self-referential
// cycle with specific detail (folds in BB58 F2 — no more generic
// "Request violates a domain invariant").
func TestPatchLocation_SelfParent_Returns409Specific(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupCycleRouter(NewHandler(store))
	x := seedLocationCycle(t, pool, orgID, "tra770-self-x", "X", nil)

	rec := patchLocationCycle(t, router, orgID, x, map[string]any{"parent_id": x})
	require.Equal(t, http.StatusConflict, rec.Code, "1-hop self-parent must be 409: %s", rec.Body.String())

	var resp errResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
	assert.Contains(t, resp.Error.Detail, "self-referential cycle",
		"detail must name the 1-hop cycle case specifically; got %q", resp.Error.Detail)
	assert.NotContains(t, resp.Error.Detail, "domain invariant",
		"BB58 F2: the generic 'domain invariant' detail must be gone")
}

// N-hop: X has child Y. PATCH X with parent_id=Y → 409 specific detail
// naming both endpoints of the cycle.
func TestPatchLocation_TransitiveCycle_Returns409Specific(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupCycleRouter(NewHandler(store))
	x := seedLocationCycle(t, pool, orgID, "tra770-trans-x", "X", nil)
	y := seedLocationCycle(t, pool, orgID, "tra770-trans-y", "Y", &x)

	rec := patchLocationCycle(t, router, orgID, x, map[string]any{"parent_id": y})
	require.Equal(t, http.StatusConflict, rec.Code, "2-hop X↔Y cycle must be 409: %s", rec.Body.String())

	var resp errResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
	assert.Contains(t, resp.Error.Detail, "cycle through location",
		"N-hop detail must name the cycle target; got %q", resp.Error.Detail)
	assert.Contains(t, resp.Error.Detail, fmt.Sprintf("%d", y))
	assert.Contains(t, resp.Error.Detail, fmt.Sprintf("%d", x))
}

// 3-hop: X → Y → Z. PATCH X with parent_id=Z → 409.
func TestPatchLocation_ThreeHopCycle_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupCycleRouter(NewHandler(store))
	x := seedLocationCycle(t, pool, orgID, "tra770-3h-x", "X", nil)
	y := seedLocationCycle(t, pool, orgID, "tra770-3h-y", "Y", &x)
	z := seedLocationCycle(t, pool, orgID, "tra770-3h-z", "Z", &y)

	rec := patchLocationCycle(t, router, orgID, x, map[string]any{"parent_id": z})
	require.Equal(t, http.StatusConflict, rec.Code, "3-hop X↔Z cycle must be 409: %s", rec.Body.String())
}

// Regression: a valid non-descendant parent assignment must still succeed.
func TestPatchLocation_ValidReparent_Returns200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupCycleRouter(NewHandler(store))
	// Two independent roots; reparenting B under A is a no-cycle change.
	a := seedLocationCycle(t, pool, orgID, "tra770-ok-a", "A", nil)
	b := seedLocationCycle(t, pool, orgID, "tra770-ok-b", "B", nil)

	rec := patchLocationCycle(t, router, orgID, b, map[string]any{"parent_id": a})
	require.Equal(t, http.StatusOK, rec.Code, "valid reparent must be 200: %s", rec.Body.String())
}

// Regression: clearing parent_id (root promotion) must still succeed even
// when the location is part of an existing chain.
func TestPatchLocation_ClearParentToRoot_Returns200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupCycleRouter(NewHandler(store))
	a := seedLocationCycle(t, pool, orgID, "tra770-root-a", "A", nil)
	b := seedLocationCycle(t, pool, orgID, "tra770-root-b", "B", &a)

	rec := patchLocationCycle(t, router, orgID, b, map[string]any{"parent_id": nil})
	require.Equal(t, http.StatusOK, rec.Code, "root promotion must be 200: %s", rec.Body.String())
}

// No-op same-parent PATCH must still succeed (the cycle check skips when
// the resolved parent equals the current parent).
func TestPatchLocation_SameParent_NoOpOK(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupCycleRouter(NewHandler(store))
	a := seedLocationCycle(t, pool, orgID, "tra770-noop-a", "A", nil)
	b := seedLocationCycle(t, pool, orgID, "tra770-noop-b", "B", &a)

	rec := patchLocationCycle(t, router, orgID, b, map[string]any{"parent_id": a})
	require.Equal(t, http.StatusOK, rec.Code, "same-parent no-op must be 200: %s", rec.Body.String())
}
