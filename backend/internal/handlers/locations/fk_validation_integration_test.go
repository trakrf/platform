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

// TRA-757 (BB50/51/52 F1): POST /api/v1/locations now reconciles
// parent_id + parent_external_key the same way PATCH does — a payload
// that names the same parent through both forms is silently normalized
// to a single re-parent operation. Pre-TRA-757 this returned 400
// ambiguous_fields; the strictness was accidental spec evolution rather
// than deliberate design (BB50/51/52 surfaced this three cycles in a
// row), and matching docs already framed POST as symmetric with PATCH.
func TestPostLocation_BothParentForms_Matching_Accepted201(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationRoundTripWithParent(t, pool, orgID, "LOC-BOTH-PARENT", "BothParent", nil)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body, err := json.Marshal(map[string]any{
		"external_key":        "LOC-BOTH-CHILD",
		"name":                "both-child",
		"parent_id":           parentID,
		"parent_external_key": "LOC-BOTH-PARENT",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code,
		"matching parent pair must be 201 (got %d): %s", rec.Code, rec.Body.String())

	var dbParent *int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT parent_location_id FROM trakrf.locations WHERE org_id = $1 AND external_key = $2`,
		orgID, "LOC-BOTH-CHILD").Scan(&dbParent))
	require.NotNil(t, dbParent, "matching pair must persist parent_location_id")
	assert.Equal(t, parentID, *dbParent)
}

// TRA-757 (BB50/51/52 F1): POST with differing parent_id and
// parent_external_key returns 400 validation_error / ambiguous_fields
// using the PATCH-shaped "both supplied and disagree" message — BB51 F2
// alignment falls out of moving to shared validation logic.
func TestPostLocation_BothParentForms_Differing_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentA := seedLocationRoundTripWithParent(t, pool, orgID, "LOC-DIFF-A", "DiffA", nil)
	seedLocationRoundTripWithParent(t, pool, orgID, "LOC-DIFF-B", "DiffB", nil)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body, err := json.Marshal(map[string]any{
		"external_key":        "LOC-DIFF-CHILD",
		"name":                "diff-child",
		"parent_id":           parentA,
		"parent_external_key": "LOC-DIFF-B",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"differing parent pair must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	require.Len(t, resp.Error.Fields, 2)
	fields := map[string]string{}
	for _, fld := range resp.Error.Fields {
		assert.Equal(t, "ambiguous_fields", fld.Code, "field %s should carry ambiguous_fields", fld.Field)
		assert.Contains(t, []string{"parent_id", "parent_external_key"}, fld.Field)
		assert.Contains(t, fld.Message, "both supplied and disagree",
			"message must use PATCH-shaped 'both supplied and disagree' wording")
		fields[fld.Field] = fld.Message
	}
	assert.Contains(t, fields, "parent_id")
	assert.Contains(t, fields, "parent_external_key")
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

// TRA-719 / BB35 B2: PATCH /api/v1/locations/{id} with an unresolvable
// parent_external_key returns 400 validation_error / fk_not_found —
// supersedes the TRA-686 read_only behavior (now reflected in
// TestPatchLocation_NaturalKey_ParentExternalKey_NotFound400 under
// patch_natural_key_integration_test.go). The natural-key form is now
// writable on PATCH and dispatches through the same FK resolution as
// CreateLocationRequest.
func TestPatchLocation_ParentExternalKey_NotFound400(t *testing.T) {
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
		"PATCH with non-existent parent_external_key must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
	assert.Equal(t, "fk_not_found", resp.Error.Fields[0].Code)

	// And the location row remains unchanged.
	var dbName string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT name FROM trakrf.locations WHERE id = $1`, id).Scan(&dbName))
	assert.Equal(t, "rej-extfk", dbName, "rejected PATCH must not have mutated name")
}
