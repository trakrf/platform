//go:build integration
// +build integration

// TRA-732 R1: PATCH /api/v1/assets/{id} with a body whose writable fields
// all match current values must not advance updated_at. Companion to the
// rename same-value contract pinned by TRA-731 — same integrator concern
// (cached-body PATCH after a defensive retry).

package assets

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
// the unchanged AssetView AND leaves updated_at untouched.
func TestPatchAsset_SameValueBody_PreservesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-NOOP", "NoOpAsset")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	// GET current state, then PATCH with the same name + is_active back.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	getReq = withRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	patchBody := []byte(`{"name":"NoOpAsset","is_active":true}`)
	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code, "same-value PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.Equal(beforeUpdatedAt),
		"same-value PATCH must not advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// Verbatim GET → PATCH round-trip (the cached-body PATCH retry pattern) must
// be fully idempotent: 200 with the same AssetView and stable updated_at.
//
// The wire format emits timestamps at millisecond precision; the seed below
// matches that precision so the test represents the real integrator path
// (rows authored via wire are already millisecond-precise). The raw-Go
// time.Now() default of seedRoundTripAsset includes nanoseconds that drop
// below wire precision and would cause the verbatim PATCH-back to look
// "different" only because the wire format truncated.
func TestPatchAsset_VerbatimGETRoundTrip_PreservesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, "ASSET-RT-NOOP", "RoundTripNoOp", time.Now().UTC().Truncate(time.Millisecond)).Scan(&id))

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	// GET the asset and capture full body for verbatim PATCH-back.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	getReq = withRoundTripOrgContext(getReq, orgID)
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
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(bodyBytes))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code,
		"verbatim GET round-trip PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.Equal(beforeUpdatedAt),
		"verbatim GET round-trip PATCH must not advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH with at least one field that genuinely differs still advances
// updated_at — confirms the IS DISTINCT FROM guard fires only on full-match.
func TestPatchAsset_ActualChange_AdvancesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-CHANGE", "OriginalName")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	// Sleep a hair so the clock resolution can't hide a real bump.
	time.Sleep(5 * time.Millisecond)

	patchBody := []byte(`{"name":"RenamedName"}`)
	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code, "real-change PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"real-change PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH against a nonexistent asset id still returns 404 — the IS DISTINCT
// FROM disambiguation must not mask missing rows.
func TestPatchAsset_NonexistentID_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	patchBody := []byte(`{"name":"DoesNotMatter"}`)
	patchReq := httptest.NewRequest(http.MethodPatch,
		"/api/v1/assets/999999999", bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusNotFound, rec.Code,
		"PATCH on nonexistent id must be 404: %s", rec.Body.String())
}
