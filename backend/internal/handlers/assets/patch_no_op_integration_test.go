//go:build integration
// +build integration

// TRA-783 supersedes TRA-732 R1: every accepted PATCH advances updated_at,
// including empty body (`{}`), verbatim writable echoes, and partial
// mutations. Filesystem `touch` semantics — any successful write advances
// mtime. Pre-TRA-783 the storage layer applied an IS DISTINCT FROM gate
// and skipped the UPDATE on full-match no-ops; that model broke for
// valid_from/valid_to whenever storage precision (µs) exceeded wire
// precision (ms), and the new uniform-advance rule removes those edge
// cases entirely.

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
// the unchanged AssetView AND advances updated_at (TRA-783 supersedes the
// pre-TRA-783 "no-op preserves updated_at" contract).
func TestPatchAsset_SameValueBody_AdvancesUpdatedAt(t *testing.T) {
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

	// Sleep a hair so the clock resolution can't hide a real bump.
	time.Sleep(5 * time.Millisecond)

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
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"same-value PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// Verbatim GET → PATCH round-trip succeeds (200) and advances updated_at.
// Pre-TRA-783 this preserved updated_at; the wire-idempotency model failed
// for server-defaulted timestamps with sub-ms storage precision, and the
// new model removes the edge case by always advancing.
func TestPatchAsset_VerbatimGETRoundTrip_AdvancesUpdatedAt(t *testing.T) {
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

	time.Sleep(5 * time.Millisecond)

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
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"verbatim GET round-trip PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH with at least one field that genuinely differs advances updated_at.
// Regression check; matches the pre-TRA-783 behavior in the changed case.
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

// TRA-783: PATCH with an empty body (`{}`) — the filesystem `touch`
// case — advances updated_at. Pre-TRA-783 this short-circuited to a
// no-op and left updated_at stable; the new uniform-advance model
// treats every accepted PATCH the same.
func TestPatchAsset_EmptyBody_AdvancesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-EMPTY", "EmptyBodyAsset")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	time.Sleep(5 * time.Millisecond)

	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(`{}`)))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code, "empty-body PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"empty-body PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// TRA-783 regression: when a PATCH is REJECTED (e.g. read-only field with a
// differing value), updated_at must NOT advance — because the operation was
// rejected before reaching the storage write. The uniform-advance rule
// applies only to ACCEPTED PATCH.
func TestPatchAsset_RejectedReadOnlyMismatch_PreservesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-REJECT", "RejectedAsset")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	time.Sleep(5 * time.Millisecond)

	// Differing id field triggers a code: read_only rejection; storage
	// never sees the request.
	patchBody := []byte(fmt.Sprintf(`{"id":%d}`, id+99999))
	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"differing read-only field PATCH must be 400: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.assets WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.Equal(beforeUpdatedAt),
		"rejected PATCH must NOT advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH against a nonexistent asset id still returns 404 — the always-touch
// UPDATE must not mask missing rows.
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
