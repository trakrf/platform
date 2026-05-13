//go:build integration
// +build integration

// TRA-674 / BB27 F2 / TRA-681: missing-reference on the surrogate
// `location_id` returns the same envelope shape as missing-reference on the
// natural-key `location_external_key` — both surface as 400 validation_error
// keyed on the offending field with code=fk_not_found.
//
// Pre-TRA-674 the surrogate path reached the storage layer and tripped the
// FK constraint as 500 internal_error. TRA-674 collapsed both onto 400
// invalid_value; TRA-678 then moved both to 409 conflict / fk_not_found to
// silence Schemathesis's positive_data_acceptance check; TRA-681 reverts to
// 400 validation_error / fk_not_found per design review. The typed code
// stays so generated clients can branch precisely.

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
// returns 400 validation_error / fk_not_found keyed on `location_id`.
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
	assert.Equal(t, "fk_not_found", resp.Error.Fields[0].Code)
}

// POST /api/v1/assets with a natural-key location_external_key that does
// not exist returns 400 validation_error / fk_not_found keyed on
// `location_external_key` — same envelope as the surrogate-id path.
func TestPostAsset_MissingLocationExternalKey_Rejected400(t *testing.T) {
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
		"external_key":          "ASSET-MISSING-EXTFK",
		"name":                  "missing-extfk",
		"location_external_key": "NOPE-XYZ",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"missing location_external_key must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	assert.Equal(t, "location_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "fk_not_found", resp.Error.Fields[0].Code)
}

// PATCH /api/v1/assets/{id} with a `location_id` that differs from the
// current resource state returns 400 validation_error / read_only — the
// field is no longer writable via PATCH (TRA-699 supersedes the prior
// fk_not_found path, which is no longer reachable on PATCH because the
// FK is never resolved). Existence of the target row is irrelevant; any
// value that differs from current state is rejected.
func TestPatchAsset_DifferingLocationID_Rejected400(t *testing.T) {
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

// POST /api/v1/assets with both location_id and location_external_key
// returns 400 validation_error / ambiguous_fields. TRA-681 oneOf rule —
// no silent winner on a request the integrator explicitly constructed.
func TestPostAsset_BothLocationForms_Rejected400(t *testing.T) {
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
		"external_key":          "ASSET-BOTH-FORMS",
		"name":                  "both-forms",
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
		assert.Equal(t, "ambiguous_fields", fld.Code, "field %s should carry ambiguous_fields", fld.Field)
		assert.Contains(t, []string{"location_id", "location_external_key"}, fld.Field)
	}
}

// GET /api/v1/assets?location_id=N&location_external_key=K is 400
// validation_error / ambiguous_fields — same oneOf rule as POST body.
func TestListAssets_BothLocationForms_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location_id=42&location_external_key=WHS-01", nil)
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"both filter forms must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
		assert.Equal(t, "ambiguous_fields", fld.Code, "field %s should carry ambiguous_fields", fld.Field)
		assert.Contains(t, []string{"location_id", "location_external_key"}, fld.Field)
	}
}

// TRA-699 supersedes the prior silent-strip behavior for
// location_external_key on PATCH. A *differing* natural key is now 400
// read_only; a *matching* echo (or null/null) is 200 with the field
// stripped from the update. See patch_natural_key_integration_test.go
// for the new accept-if-matches / reject-if-differs coverage.
