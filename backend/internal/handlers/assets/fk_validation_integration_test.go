//go:build integration
// +build integration

// TRA-734 (BB40 F3): asset location is scan/operational data, not master
// data. The public API no longer accepts location_id or location_external_key
// on POST /assets — both fields return 400 validation_error / read_only with
// an envelope that points at the consumption surfaces (GET /assets/{id},
// GET /assets/{id}/history, GET /reports/asset-locations).
//
// Pre-TRA-734 history kept for context: the surrogate path used to surface
// fk_not_found via resolveLocation (TRA-674 / BB27 F2 / TRA-681); the
// mutually-exclusive both-supplied path used to surface ambiguous_fields.
// Both paths are now unreachable on Create — the read_only pre-decode reject
// fires first regardless of value.

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

// TRA-734 (BB40 F3): POST /api/v1/assets with `location_id` is rejected
// pre-decode with 400 validation_error / read_only naming the consumption
// surfaces. The check fires whether the row exists or not — asset location
// is never settable on Create regardless of value.
func TestPostAsset_LocationID_Rejected400_ReadOnly(t *testing.T) {
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
		"external_key": "ASSET-LOC-ID-RO",
		"name":         "loc-id-readonly",
		"location_id":  42,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"location_id on POST must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message,
		"asset location is collected through scan event ingestion")
	assert.Contains(t, resp.Error.Fields[0].Message,
		"/api/v1/reports/asset-locations")
}

// TRA-734 (BB40 F3): POST /api/v1/assets with `location_external_key` is
// rejected pre-decode with 400 validation_error / read_only — same shape
// as the location_id case.
func TestPostAsset_LocationExternalKey_Rejected400_ReadOnly(t *testing.T) {
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
		"external_key":          "ASSET-LOC-EXT-RO",
		"name":                  "loc-ext-readonly",
		"location_external_key": "WHS-01",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"location_external_key on POST must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message,
		"asset location is collected through scan event ingestion")
}

// PATCH /api/v1/assets/{id} carrying `location_id` is pre-decode-rejected
// 400 validation_error / read_only (PublicRejectPatchFields). TRA-799:
// location is not part of the asset resource — any presence of the field,
// regardless of value, is rejected.
func TestPatchAsset_LocationID_Rejected400_ReadOnly(t *testing.T) {
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
		"differing location_id on PATCH must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
}

// TRA-734 (BB40 F3): POST /api/v1/assets with both location_id and
// location_external_key returns 400 validation_error / read_only with both
// fields named (RejectFields enumerates every offending key). The pre-
// TRA-734 ambiguous_fields shape is unreachable because the read_only
// pre-decode reject fires before any value-level reconciliation.
func TestPostAsset_BothLocationForms_Rejected400_ReadOnly(t *testing.T) {
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
		"external_key":          "ASSET-BOTH-FORMS-RO",
		"name":                  "both-forms-readonly",
		"location_id":           42,
		"location_external_key": "WHS-01",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"both forms must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	require.Len(t, resp.Error.Fields, 2)
	for _, fld := range resp.Error.Fields {
		assert.Equal(t, "read_only", fld.Code, "field %s should carry read_only", fld.Field)
		assert.Contains(t, []string{"location_id", "location_external_key"}, fld.Field)
	}
}
