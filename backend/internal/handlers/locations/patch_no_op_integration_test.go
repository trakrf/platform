//go:build integration
// +build integration

// TRA-732 R1: PATCH /api/v1/locations/{id} with a body whose writable fields
// all match current values must not advance updated_at. Locations parallel
// to the assets contract pinned in patch_no_op_integration_test.go.

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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// PATCH with every writable field set to its current value returns 200 with
// the unchanged LocationView AND leaves updated_at untouched.
func TestPatchLocation_SameValueBody_PreservesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTripWithParent(t, pool, orgID, "LOC-NOOP", "NoOpLocation", nil)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	patchBody := []byte(`{"name":"NoOpLocation","is_active":true}`)
	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code, "same-value PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.Equal(beforeUpdatedAt),
		"same-value PATCH must not advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// Verbatim GET → PATCH round-trip (the cached-body PATCH retry pattern) must
// be fully idempotent: 200 with the same LocationView and stable updated_at.
//
// The wire format emits timestamps at millisecond precision; the seed below
// matches that precision so the test represents the real integrator path
// (rows authored via wire are already millisecond-precise).
func TestPatchLocation_VerbatimGETRoundTrip_PreservesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, "LOC-RT-NOOP", "RoundTripNoOpLoc", time.Now().UTC().Truncate(time.Millisecond)).Scan(&id))

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	getReq = withLocationRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	bodyBytes, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(bodyBytes))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code,
		"verbatim GET round-trip PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.Equal(beforeUpdatedAt),
		"verbatim GET round-trip PATCH must not advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH with at least one field that genuinely differs still advances
// updated_at — confirms the IS DISTINCT FROM guard fires only on full-match.
func TestPatchLocation_ActualChange_AdvancesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTripWithParent(t, pool, orgID, "LOC-CHANGE", "OriginalLocName", nil)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	time.Sleep(5 * time.Millisecond)

	patchBody := []byte(`{"name":"RenamedLocName"}`)
	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code, "real-change PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"real-change PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH against a nonexistent location id still returns 404 — the IS DISTINCT
// FROM disambiguation must not mask missing rows.
func TestPatchLocation_NonexistentID_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	patchBody := []byte(`{"name":"DoesNotMatter"}`)
	patchReq := httptest.NewRequest(http.MethodPatch,
		"/api/v1/locations/999999999", bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusNotFound, rec.Code,
		"PATCH on nonexistent id must be 404: %s", rec.Body.String())
}
