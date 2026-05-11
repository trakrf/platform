//go:build integration
// +build integration

// TRA-674 / BB27 F2: missing-reference on the surrogate `location_id` returns
// the same validation_error envelope shape as missing-reference on the
// natural-key `location_external_key` — both surface as 400 invalid_value
// keyed on the offending field. Previously the surrogate path reached the
// storage layer and tripped the FK constraint as 500 internal_error, while
// the natural-key path was already a clean 400. Two different envelopes for
// the same logical case forced integrators to write branch code per FK
// form; the fix collapses them onto one shape.

package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// POST /api/v1/assets with a surrogate location_id that does not exist
// returns 400 validation_error / invalid_value keyed on `location_id`.
// Mirrors the existing natural-key behavior for `location_external_key`.
func TestPostAsset_MissingLocationID_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body, err := json.Marshal(map[string]any{
		"external_key": "ASSET-MISSING-FK",
		"name":         "missing-fk",
		"location_id":  99999999,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"missing location_id must be 400 (got %d): %s", rec.Code, rec.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}

// PATCH /api/v1/assets/{id} with a surrogate location_id that does not
// exist returns the same shape as POST — 400 validation_error /
// invalid_value keyed on `location_id`.
func TestPatchAsset_MissingLocationID_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-PATCH-MISSING-FK", "patch-missing-fk")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{"location_id":99999999}`)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"missing location_id on PATCH must be 400 (got %d): %s", rec.Code, rec.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}
