//go:build integration
// +build integration

// TRA-783 supersedes TRA-732 R1: every accepted PATCH advances updated_at,
// including empty body (`{}`), verbatim writable echoes, and partial
// mutations. Locations parallel to the assets contract pinned in
// patch_no_op_integration_test.go.

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
// the unchanged LocationView AND advances updated_at (TRA-783 supersedes
// the pre-TRA-783 "no-op preserves updated_at" contract).
func TestPatchLocation_SameValueBody_AdvancesUpdatedAt(t *testing.T) {
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

	time.Sleep(5 * time.Millisecond)

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
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"same-value PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// Verbatim GET → PATCH round-trip succeeds (200) and advances updated_at
// per TRA-783.
func TestPatchLocation_VerbatimGETRoundTrip_AdvancesUpdatedAt(t *testing.T) {
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

	time.Sleep(5 * time.Millisecond)

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
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"verbatim GET round-trip PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH with at least one field that genuinely differs advances updated_at.
// Regression check.
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

// TRA-783: PATCH with an empty body (`{}`) — the filesystem `touch`
// case — advances updated_at.
func TestPatchLocation_EmptyBody_AdvancesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTripWithParent(t, pool, orgID, "LOC-EMPTY", "EmptyBodyLocation", nil)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	time.Sleep(5 * time.Millisecond)

	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(`{}`)))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusOK, rec.Code, "empty-body PATCH must be 200: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt),
		"empty-body PATCH must advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// TRA-783 regression: when a PATCH is REJECTED (e.g. read-only field with a
// differing value), updated_at must NOT advance — the operation was rejected
// before reaching the storage write.
func TestPatchLocation_RejectedReadOnlyMismatch_PreservesUpdatedAt(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTripWithParent(t, pool, orgID, "LOC-REJECT", "RejectedLocation", nil)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	var beforeUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&beforeUpdatedAt))

	time.Sleep(5 * time.Millisecond)

	patchBody := []byte(fmt.Sprintf(`{"id":%d}`, id+99999))
	patchReq := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"differing read-only field PATCH must be 400: %s", rec.Body.String())

	var afterUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT updated_at FROM trakrf.locations WHERE id = $1`, id).Scan(&afterUpdatedAt))
	assert.True(t, afterUpdatedAt.Equal(beforeUpdatedAt),
		"rejected PATCH must NOT advance updated_at (before=%s after=%s)",
		beforeUpdatedAt, afterUpdatedAt)
}

// PATCH against a nonexistent location id still returns 404 — the always-touch
// UPDATE must not mask missing rows.
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
