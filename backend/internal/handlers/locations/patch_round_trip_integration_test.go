//go:build integration
// +build integration

// TRA-608 / BB18 §1.7 + TRA-610 / BB18 §1.8: locations counterpart to the
// assets PUT round-trip + always-emit tests.
//
// TRA-643 / BB22 F1: `tags` is managed via /locations/{id}/tags. The PUT
// validator rejects a `tags` body field with 400 invalid_value rather than
// silently dropping it.

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

func setupLocationRoundTripRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations/{location_id}", handler.GetLocation)
	r.Patch("/api/v1/locations/{location_id}", handler.Update)
	return r
}

func withLocationRoundTripOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra608-loc@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocationRoundTrip(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, extKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

// TRA-608 acceptance: GET /api/v1/locations/{id} → unmodified body →
// PUT /api/v1/locations/{id} succeeds with 200.
func TestPutLocation_GETBodyRoundTrip_Succeeds(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "WHS-01", "Warehouse 1")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	getReq = withLocationRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var getResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))

	for _, field := range []string{"id", "created_at", "updated_at", "tags", "tree_path", "depth", "description", "valid_to"} {
		_, present := getResp.Data[field]
		assert.True(t, present, "GET response must include %q (TRA-608/610)", field)
	}

	// Mutate name and PUT back. `tags` is managed via /locations/{id}/tags
	// and must be stripped (TRA-643); `external_key` is immutable and must
	// be stripped (TRA-664 / BB26 D7 — POST /locations/{id}/rename is the
	// dedicated path). Other read-only fields stay on the body to exercise
	// the round-trip-safe drop list (id, created_at, updated_at, tree_path, depth).
	getResp.Data["name"] = "Warehouse 1 (renamed)"
	delete(getResp.Data, "tags")
	delete(getResp.Data, "external_key")
	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "PUT round-trip must succeed: %s", putRec.Body.String())

	var putResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	assert.Equal(t, "Warehouse 1 (renamed)", putResp.Data["name"])
}

func TestPutLocation_TypoFieldStillRejected(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "WHS-02", "Warehouse 2")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"name":"x","nme":"oops"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusBadRequest, putRec.Code, "typo'd field must still be rejected")

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "nme", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}

func TestGetLocation_OptionalFieldsAlwaysEmittedNullWhenUnset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "WHS-03", "Warehouse 3")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	descRaw, present := resp.Data["description"]
	assert.True(t, present, "description must always be present (TRA-610)")
	assert.Nil(t, descRaw, "description must be JSON null when empty (TRA-610)")

	vtRaw, present := resp.Data["valid_to"]
	assert.True(t, present, "valid_to must always be present (TRA-610)")
	assert.Nil(t, vtRaw, "valid_to must be JSON null when nil (TRA-610)")
}

// TRA-614 / BB19 §S1: PUT with explicit `null` on read-side-nullable
// location fields must succeed and clear the column.
func TestPutLocation_NullClearsReadSideNullableFields(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	// Seed a parent + child so the child has a non-null parent_location_id
	// and a non-null description; nulls on PUT must clear both.
	parentID := seedLocationRoundTrip(t, pool, orgID, "PARENT-NULL", "parent")
	var childID int
	vt := time.Now().UTC().Add(24 * time.Hour)
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations
		  (org_id, external_key, name, description, parent_location_id, valid_from, valid_to, is_active)
		VALUES ($1, 'CHILD-NULL', 'child', 'has description', $2, $3, $4, true) RETURNING id
	`, orgID, parentID, time.Now().UTC(), vt).Scan(&childID)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{
		"description": null,
		"parent_id": null,
		"parent_external_key": null,
		"valid_to": null
	}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", childID), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "PUT null on nullable fields must succeed: %s", putRec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
	assert.Nil(t, resp.Data["description"], "description cleared")
	assert.Nil(t, resp.Data["parent_id"], "parent_id cleared")
	assert.Nil(t, resp.Data["parent_external_key"], "parent_external_key cleared")
	assert.Nil(t, resp.Data["valid_to"], "valid_to cleared")

	var dbDesc string
	var dbParent, dbValidTo *string
	err = pool.QueryRow(context.Background(),
		`SELECT description, parent_location_id::text, valid_to::text FROM trakrf.locations WHERE id = $1`,
		childID).Scan(&dbDesc, &dbParent, &dbValidTo)
	require.NoError(t, err)
	assert.Equal(t, "", dbDesc)
	assert.Nil(t, dbParent)
	assert.Nil(t, dbValidTo)
}

// TRA-614: parent_id null + parent_external_key value is a conflict.
func TestPutLocation_ParentNullVsValueIsConflict(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "PARENT-CONFLICT", "p")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"parent_id": null, "parent_external_key": "X-99"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusBadRequest, putRec.Code, "null/value conflict on parent pair must be 400: %s", putRec.Body.String())
}

// TRA-615 / BB19 §S5+§C2: external_key with reserved punctuation (space,
// slash, colon, period, underscore) is rejected at the validator boundary
// with 400 invalid_value rather than reaching storage and triggering 500.
func TestPostLocation_BadExternalKeyPattern_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	cases := []struct {
		name string
		key  string
	}{
		{"space", "BB With Spaces"},
		{"slash", "BB/slash"},
		{"colon", "BB:colon"},
		{"period", "BB.Dots.Hi"},
		{"underscore", "BB-EVAL_L1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"external_key": tc.key,
				"name":         "loc",
			})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"external_key %q must be rejected with 400 (got %d): %s", tc.key, rec.Code, rec.Body.String())

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
			require.NotEmpty(t, resp.Error.Fields)
			assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
			assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
		})
	}
	_ = pool
}

// TRA-650 / BB23 F3 (audit): POST /api/v1/locations must reject an explicit
// empty external_key with 400 too_short. Locations have always declared
// `required,min=1,max=255,external_key_pattern` on the field, so this test
// pins the symmetric behavior alongside the assets fix and guards against
// regressions if the validator chain is ever reordered.
func TestPostLocation_EmptyExternalKey_Rejected400(t *testing.T) {
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
		"external_key": "",
		"name":         "loc",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "explicit empty external_key must be 400: %s", rec.Body.String())

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
	require.NotEmpty(t, resp.Error.Fields)
	assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "too_short", resp.Error.Fields[0].Code)

	_ = pool
}

// TRA-615: well-formed external_keys (alphanumerics + hyphens) still succeed.
func TestPostLocation_GoodExternalKeyPattern_Accepted(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	for _, k := range []string{"WHS-01", "BB-EVAL-L1", "abc123", "A1"} {
		t.Run(k, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"external_key": k,
				"name":         "loc",
			})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusCreated, rec.Code, "external_key %q must be accepted: %s", k, rec.Body.String())
		})
	}
	_ = pool
}

// TRA-619 finding 1 (locations parallel surface): a PUT body that contains
// only read-only fields decodes to an empty UpdateLocationRequest after the
// readOnly drop. Previous behavior was a "no fields to update" error
// surfaced as 500 internal_error. Expected: 200 with the unchanged record.
func TestPutLocation_OnlyReadOnlyFields_Returns200NoOp(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-RO-NOOP", "RoNoop")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"only id", `{"id":999}`},
		{"only created_at", `{"created_at":"2020-01-01T00:00:00Z"}`},
		{"only updated_at", `{"updated_at":"2020-01-01T00:00:00Z"}`},
		{"only tree_path", `{"tree_path":"x"}`},
		{"only depth", `{"depth":42}`},
		{"empty object", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
			putRec := httptest.NewRecorder()
			router.ServeHTTP(putRec, patchReq)

			require.Equal(t, http.StatusOK, putRec.Code,
				"empty effective body must be no-op 200 (got %d): %s", putRec.Code, putRec.Body.String())

			var resp struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
			assert.Equal(t, "RoNoop", resp.Data["name"], "name unchanged")
			assert.Equal(t, "LOC-RO-NOOP", resp.Data["external_key"], "external_key unchanged")
		})
	}
}

// TRA-643 / BB22 F1: `tags` is managed via the /locations/{id}/tags
// subresource. A `tags` key in the PUT body must be rejected with 400
// invalid_value (matching the unknown-field response shape) so a
// read-modify-write integrator gets a clear signal instead of a silent
// no-op.
func TestPutLocation_TagsRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-TAGS-REJ", "TagsRej")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"empty array", `{"tags":[]}`},
		{"tags with values", `{"tags":[{"key":"foo","value":"bar"}]}`},
		{"tags alongside name", `{"name":"x","tags":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
			putRec := httptest.NewRecorder()
			router.ServeHTTP(putRec, patchReq)

			require.Equal(t, http.StatusBadRequest, putRec.Code,
				"tags in PUT body must be 400 (got %d): %s", putRec.Code, putRec.Body.String())

			var resp struct {
				Error struct {
					Type   string `json:"type"`
					Fields []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"fields"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
			assert.Equal(t, "validation_error", resp.Error.Type)
			require.Len(t, resp.Error.Fields, 1)
			assert.Equal(t, "tags", resp.Error.Fields[0].Field)
			assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
		})
	}
}

// TRA-649 / BB23 F2 (locations parallel surface): POST /api/v1/locations
// must reject loose date forms on valid_from / valid_to. assets and
// locations share a single FlexibleDate parser; this test pins the
// behavior at the locations seam so the audit residue can't regress
// independently.
func TestPostLocation_LooseDateForms_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	cases := []struct {
		name      string
		field     string
		bodyValue string
	}{
		{"valid_from date-only", "valid_from", `"2026-05-10"`},
		{"valid_from slashes", "valid_from", `"2026/05/10"`},
		{"valid_from empty string", "valid_from", `""`},
		{"valid_from Go zero-time", "valid_from", `"0001-01-01T00:00:00Z"`},
		{"valid_to date-only", "valid_to", `"2027-05-10"`},
		{"valid_to slashes", "valid_to", `"2027/05/10"`},
		{"valid_to empty string", "valid_to", `""`},
		{"valid_to Go zero-time", "valid_to", `"0001-01-01T00:00:00Z"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"external_key":"LOC-LOOSE-%s","name":"loose","%s":%s}`,
				tc.name, tc.field, tc.bodyValue)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"%s body %q must be 400: %s", tc.field, tc.bodyValue, rec.Body.String())

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
			require.NotEmpty(t, resp.Error.Fields)
			assert.Equal(t, tc.field, resp.Error.Fields[0].Field)
			assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
		})
	}

	_ = pool
}
