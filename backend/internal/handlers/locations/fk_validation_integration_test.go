//go:build integration
// +build integration

// TRA-674 / BB27 F2 / TRA-681: missing-reference on the surrogate
// `parent_id` returns the same envelope shape as missing-reference on the
// natural-key `parent_external_key` — both surface as 400 validation_error
// keyed on the offending field with code=fk_not_found. Mirrors the fix on
// the assets surface for location_id ↔ location_external_key.
//
// History: TRA-678 routed this to 409 conflict / fk_not_found to silence
// Schemathesis; TRA-681 reverts to 400 validation_error / fk_not_found per
// design review (industry precedent, conceptual cleanliness — see
// assets/fk_validation_integration_test.go for full notes).

package locations

import (
	"bytes"
	"context"
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

// POST /api/v1/locations with a surrogate parent_id that does not exist
// returns 400 validation_error / fk_not_found keyed on `parent_id`.
func TestPostLocation_MissingParentID_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body, err := json.Marshal(map[string]any{
		"external_key": "LOC-MISSING-FK",
		"name":         "missing-fk",
		"parent_id":    99999999,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"missing parent_id must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	assert.Equal(t, "parent_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "fk_not_found", resp.Error.Fields[0].Code)
}

// POST /api/v1/locations with a natural-key parent_external_key that does
// not exist returns 400 validation_error / fk_not_found keyed on
// `parent_external_key` — same envelope as the surrogate-id path.
func TestPostLocation_MissingParentExternalKey_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body, err := json.Marshal(map[string]any{
		"external_key":        "LOC-MISSING-EXTFK",
		"name":                "missing-extfk",
		"parent_external_key": "NOPE-XYZ",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"missing parent_external_key must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	assert.Equal(t, "parent_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "fk_not_found", resp.Error.Fields[0].Code)
}

// PATCH /api/v1/locations/{id} with a surrogate parent_id that does not
// exist returns 400 validation_error / fk_not_found keyed on `parent_id`.
func TestPatchLocation_MissingParentID_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-PATCH-MISSING-FK", "patch-missing-fk")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"parent_id":99999999}`)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"missing parent_id on PATCH must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	assert.Equal(t, "parent_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "fk_not_found", resp.Error.Fields[0].Code)
}

// POST /api/v1/locations with both parent_id and parent_external_key
// returns 400 validation_error / ambiguous_fields. TRA-681 oneOf rule.
func TestPostLocation_BothParentForms_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body, err := json.Marshal(map[string]any{
		"external_key":        "LOC-BOTH-FORMS",
		"name":                "both-forms",
		"parent_id":           42,
		"parent_external_key": "WHS-01",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
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
		assert.Contains(t, []string{"parent_id", "parent_external_key"}, fld.Field)
	}
}

// GET /api/v1/locations?parent_id=N&parent_external_key=K is 400
// validation_error / ambiguous_fields — same oneOf rule as POST body.
func TestListLocations_BothParentForms_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations", handler.ListLocations)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations?parent_id=42&parent_external_key=WHS-01", nil)
	req = withLocationRoundTripOrgContext(req, orgID)
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
		assert.Contains(t, []string{"parent_id", "parent_external_key"}, fld.Field)
	}
}

// PATCH /api/v1/locations/{id} with parent_external_key in the body is
// rejected with 400 read_only pointing at the rename endpoint
// (TRA-686 / BB29 F8). Replaces the silent-strip behavior from TRA-681 —
// silent-drop hid bugs in read-modify-write integrations.
func TestPatchLocation_ParentExternalKey_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-REJ-EXTFK", "rej-extfk")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"name":"renamed","parent_external_key":"DOES-NOT-EXIST"}`)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"PATCH with parent_external_key must be 400 read_only (got %d): %s", rec.Code, rec.Body.String())

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
	assert.Equal(t, "parent_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)

	// TRA-713 / BB33 F3: the hint must direct integrators at parent_id
	// (the surrogate that actually re-parents). The /rename endpoint only
	// renames external_key (RenameLocationRequest carries no parent
	// field), so a hint mentioning /rename for re-parenting is wrong and
	// sends integrators down a dead end.
	msg := resp.Error.Fields[0].Message
	assert.NotContains(t, msg, "/rename",
		"parent_external_key hint must not point at /rename — that endpoint can't re-parent")
	assert.Contains(t, msg, "parent_id",
		"parent_external_key hint must direct integrators at parent_id")

	// And the location row remains unchanged.
	var dbName string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT name FROM trakrf.locations WHERE id = $1`, id).Scan(&dbName))
	assert.Equal(t, "rej-extfk", dbName, "rejected PATCH must not have mutated name")
}
