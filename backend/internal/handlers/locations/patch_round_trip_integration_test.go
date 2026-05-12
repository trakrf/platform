//go:build integration
// +build integration

// TRA-608 / BB18 §1.7 + TRA-610 / BB18 §1.8: locations counterpart to the
// assets PUT round-trip + always-emit tests.
//
// TRA-674 / BB27 F3: `tags` and `external_key` are now silently stripped on
// PATCH along with id / created_at / updated_at / deleted_at so a verbatim
// GET → PATCH round-trip succeeds. Tag mutation still goes through
// /locations/{id}/tags and rename still goes through /locations/{id}/rename.

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

	for _, field := range []string{"id", "created_at", "updated_at", "tags", "description", "valid_to"} {
		_, present := getResp.Data[field]
		assert.True(t, present, "GET response must include %q (TRA-608/610)", field)
	}

	// Mutate name and PATCH the full body back. All read-only fields
	// (id, created_at, updated_at, deleted_at, external_key, tags) are
	// silently stripped server-side per TRA-674 / BB27 F3 — the integrator
	// does not need to scrub the body first.
	getResp.Data["name"] = "Warehouse 1 (renamed)"
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

// TRA-681 supersedes TRA-614: parent_external_key is read-only on PATCH
// and stripped before validation, so
// `{"parent_id": null, "parent_external_key": "X-99"}` is processed as
// `{"parent_id": null}` — clear the FK. The previous "disagree → 400" rule
// no longer applies.
func TestPutLocation_ParentNullStripsExternalKey_ClearsFK(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationRoundTrip(t, pool, orgID, "PARENT-STRIP", "parent")
	var childID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations
		  (org_id, external_key, name, description, parent_location_id, valid_from, is_active)
		VALUES ($1, 'CHILD-STRIP', 'child-strip', '', $2, $3, true) RETURNING id
	`, orgID, parentID, time.Now().UTC()).Scan(&childID)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"parent_id": null, "parent_external_key": "X-99"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", childID), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "PATCH must strip natural-key + clear FK via null id: %s", putRec.Body.String())

	var dbParent *int
	err = pool.QueryRow(context.Background(),
		`SELECT parent_location_id FROM trakrf.locations WHERE id = $1`, childID).Scan(&dbParent)
	require.NoError(t, err)
	assert.Nil(t, dbParent, "parent FK must be cleared (parent_id: null wins after strip)")
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

// TRA-665 / BB26 D3: POST /api/v1/locations must reject an explicit empty
// external_key with 400 too_short. Location external_key is now optional by
// *omission* (auto-mints LOC-NNNN), so the explicit-empty case is enforced
// by the handler's presentKeys-gated validator (mirrors POST /assets).
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

// TRA-665 / BB26 D3: when external_key is omitted from the body, the server
// auto-mints a LOC-NNNN value — the legitimate "omit means auto-mint" path
// (parallels POST /assets ASSET-NNNN behavior).
func TestPostLocation_OmittedExternalKey_AutoMints(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body, err := json.Marshal(map[string]any{"name": "auto-mint-me"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "omitted external_key must auto-mint: %s", rec.Body.String())

	var resp struct {
		Data struct {
			ExternalKey string `json:"external_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Regexp(t, `^LOC-\d+$`, resp.Data.ExternalKey)

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

// TRA-674 / BB27 F3: `tags` in a PATCH body is silently stripped. Tag
// mutation still goes through POST/DELETE /locations/{id}/tags; the PATCH
// body just tolerates the read-only field so a verbatim GET → PATCH
// round-trip succeeds. Previously (TRA-643 / BB22 F1) a `tags` key
// surfaced as 400 invalid_value, but the strip-vs-reject rule was reversed
// pre-launch — see PublicReadOnlyFields.
func TestPutLocation_TagsStripped200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-TAGS-STRIP", "TagsStrip")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"empty array", `{"tags":[]}`},
		{"tags with values", `{"tags":[{"key":"foo","value":"bar"}]}`},
		{"tags alongside name", `{"name":"TagsStrip renamed","tags":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
			putRec := httptest.NewRecorder()
			router.ServeHTTP(putRec, patchReq)

			require.Equal(t, http.StatusOK, putRec.Code,
				"tags in PATCH body must be 200 silent-strip (got %d): %s", putRec.Code, putRec.Body.String())

			var tagCount int
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT count(*) FROM trakrf.tags WHERE location_id = $1 AND deleted_at IS NULL`, id).Scan(&tagCount))
			assert.Equal(t, 0, tagCount, "PATCH must not mutate tag subresource")
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

// TRA-675 / BB27 F4: PATCH `{"description":""}` must be rejected with
// 400 too_short / min_length=1 instead of silently coercing to null in
// the response. Matches asset behavior.
func TestPutLocation_EmptyDescription_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-DESC-EMPTY", "DescEmpty")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"description":""}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)

	require.Equal(t, http.StatusBadRequest, rec.Code, "empty description must be 400: %s", rec.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field  string         `json:"field"`
				Code   string         `json:"code"`
				Params map[string]any `json:"params"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "description", resp.Error.Fields[0].Field)
	assert.Equal(t, "too_short", resp.Error.Fields[0].Code)
	assert.EqualValues(t, 1, resp.Error.Fields[0].Params["min_length"])
}

// TRA-675 / BB27 F4: POST mirrors PATCH for description empty-string.
func TestPostLocation_EmptyDescription_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body := []byte(`{"external_key":"LOC-DESC-POST-EMPTY","name":"n","description":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "empty description must be 400: %s", rec.Body.String())

	var resp struct {
		Error struct {
			Fields []struct {
				Field  string         `json:"field"`
				Code   string         `json:"code"`
				Params map[string]any `json:"params"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "description", resp.Error.Fields[0].Field)
	assert.Equal(t, "too_short", resp.Error.Fields[0].Code)
	assert.EqualValues(t, 1, resp.Error.Fields[0].Params["min_length"])

	_ = pool
}

// TRA-675 / BB27 F5: POST with empty body must report missing `name` as
// code=too_short with min_length, not code=required (errors.mdx contract).
func TestPostLocation_MissingNameEmitsTooShort(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "missing name must be 400: %s", rec.Body.String())

	var resp struct {
		Error struct {
			Fields []struct {
				Field  string         `json:"field"`
				Code   string         `json:"code"`
				Params map[string]any `json:"params"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "name", resp.Error.Fields[0].Field)
	assert.Equal(t, "too_short", resp.Error.Fields[0].Code,
		"length-bearing required field missing from body must be too_short, not required")
	assert.EqualValues(t, 1, resp.Error.Fields[0].Params["min_length"])

	_ = pool
}

// TRA-675 / Schemathesis Class D: POST with explicit `valid_from: null`
// is accepted as "use server default" — Create schemas mark valid_from
// nullable:true to match handler behavior. PATCH still rejects null on
// valid_from (no "use default" semantic on update).
func TestPostLocation_NullValidFrom_AcceptedAsNow(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body := []byte(`{"external_key":"LOC-NULL-VF","name":"n","valid_from":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "valid_from null must be accepted as now: %s", rec.Body.String())

	var resp struct {
		Data struct {
			ValidFrom time.Time `json:"valid_from"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Data.ValidFrom.IsZero(), "valid_from must be populated")
	assert.WithinDuration(t, time.Now().UTC(), resp.Data.ValidFrom, 5*time.Minute)

	_ = pool
}

// TRA-675: PATCH keeps rejecting valid_from null on locations, mirroring
// assets. Update path has no "use server default" semantic.
func TestPutLocation_NullValidFrom_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-NULL-VF-PUT", "NullVfPut")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"valid_from":null}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, patchReq)

	require.Equal(t, http.StatusBadRequest, rec.Code, "PATCH valid_from null must be 400: %s", rec.Body.String())

	var resp struct {
		Error struct {
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "valid_from", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}
