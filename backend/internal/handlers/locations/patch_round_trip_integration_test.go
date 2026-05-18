//go:build integration
// +build integration

// TRA-608 / BB18 §1.7 + TRA-610 / BB18 §1.8: locations counterpart to the
// assets PATCH round-trip + always-emit tests.
//
// TRA-710 (BB33 F2): all read-only fields on PATCH follow the uniform
// accept-if-matches / reject-if-differs rule — server-managed (id,
// created_at, updated_at, deleted_at), tags, and the natural-key
// references (external_key, parent_external_key). Pre-TRA-710 the four
// timestamps+id were silent-stripped regardless of value (TRA-608) and
// tags was pre-decode rejected as invalid_value (TRA-686).

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

	// Mutate name and PATCH back with the entire GET body intact — TRA-710
	// normalizes matching read-only fields out (tags, external_key,
	// parent_external_key, id, created_at, updated_at, deleted_at), no
	// client-side scrub required.
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
	assert.Equal(t, "unknown_field", resp.Error.Fields[0].Code)
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

	// TRA-686: parent FK clears originally only landed via `parent_id: null`.
	// TRA-719 / BB35 B2 restored writability to parent_external_key, so
	// either form now clears the FK (see TestPatchLocation_NaturalKey_*
	// in patch_natural_key_integration_test.go for the natural-key path).
	// This test pins the surrogate form's null-clear behavior.
	body := []byte(`{
		"description": null,
		"parent_id": null,
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
	assert.Nil(t, resp.Data["parent_external_key"], "parent_external_key derives from cleared parent_id")
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

// TRA-686 supersedes TRA-681: parent_external_key in a PATCH body is no
// longer silently stripped — it's rejected with 400 read_only pointing
// at the rename endpoint. Re-parenting on PATCH is exclusively via
// `parent_id`; the natural-key form has no write semantic on this verb.
// PATCH with just `parent_id: null` still clears the FK (sole signal).
func TestPatchLocation_ParentIDNull_ClearsFK(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationRoundTrip(t, pool, orgID, "PARENT-CLR", "parent")
	var childID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations
		  (org_id, external_key, name, description, parent_location_id, valid_from, is_active)
		VALUES ($1, 'CHILD-CLR', 'child-clr', '', $2, $3, true) RETURNING id
	`, orgID, parentID, time.Now().UTC()).Scan(&childID)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	body := []byte(`{"parent_id": null}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", childID), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "PATCH must clear FK via parent_id null: %s", putRec.Body.String())

	var dbParent *int
	err = pool.QueryRow(context.Background(),
		`SELECT parent_location_id FROM trakrf.locations WHERE id = $1`, childID).Scan(&dbParent)
	require.NoError(t, err)
	assert.Nil(t, dbParent, "parent FK must be cleared")
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

// TRA-619 finding 1 / TRA-710 (BB33 F2): a PATCH body that contains only
// read-only fields whose values match the current resource must succeed
// with 200. Pre-TRA-710 the four server-managed timestamps + surrogate id
// were silent-stripped regardless of value; now they follow the
// accept-if-matches, reject-if-differs rule.
func TestPutLocation_OnlyReadOnlyFields_MatchingCurrent_Returns200NoOp(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-RO-NOOP", "RoNoop")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	// GET the current resource so we can echo back its server-managed
	// timestamps verbatim — the matching path requires real values.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	getReq = withLocationRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
	var current struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &current))
	cur := current.Data

	cases := []struct {
		name string
		body string
	}{
		{"only id matches", fmt.Sprintf(`{"id":%d}`, id)},
		{"only created_at matches", fmt.Sprintf(`{"created_at":%q}`, cur["created_at"])},
		{"only updated_at matches", fmt.Sprintf(`{"updated_at":%q}`, cur["updated_at"])},
		{"only deleted_at matches", `{"deleted_at":null}`},
		{"only tags matches (empty)", `{"tags":[]}`},
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
				"matching read-only body must be 200 (got %d): %s", putRec.Code, putRec.Body.String())

			var resp struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
			assert.Equal(t, "RoNoop", resp.Data["name"], "name unchanged")
			assert.Equal(t, "LOC-RO-NOOP", resp.Data["external_key"], "external_key unchanged")
		})
	}
}

// TRA-710 (BB33 F2): differing values for the server-managed read-only
// fields (id, created_at, updated_at, deleted_at) return 400 read_only.
// Pre-TRA-710 all four were silent-stripped regardless of value.
func TestPatchLocation_ServerManagedReadOnly_Differs400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-RO-DIFF", "ReadOnlyDiff")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	cases := []struct {
		name  string
		body  string
		field string
	}{
		{"id differs", fmt.Sprintf(`{"id":%d}`, id+99999), "id"},
		{"created_at differs", `{"created_at":"2020-01-01T00:00:00Z"}`, "created_at"},
		{"updated_at differs", `{"updated_at":"2020-01-01T00:00:00Z"}`, "updated_at"},
		{"deleted_at differs (non-null vs null current)", `{"deleted_at":"2020-01-01T00:00:00Z"}`, "deleted_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, patchReq)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"differing read-only field must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
			assert.Equal(t, tc.field, resp.Error.Fields[0].Field)
			assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
		})
	}
}

// TRA-721: read-only datetime fields (created_at, updated_at, deleted_at)
// must accept any RFC 3339 wire form of the current instant, not just a
// byte-equal echo. Mirrors TestPatchAsset_DatetimeEncodingVariants — the
// fix lives in the shared httputil comparator so both resources are
// exercised end-to-end.
func TestPatchLocation_DatetimeEncodingVariants_InstantEquality_200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-RO-INST", "ReadOnlyInstant")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	getReq = withLocationRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
	var current struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &current))

	parseWireTime := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339Nano, s)
		require.NoError(t, err)
		return v
	}
	createdAt := parseWireTime(current.Data["created_at"].(string))
	updatedAt := parseWireTime(current.Data["updated_at"].(string))

	variants := func(label string, t0 time.Time) []struct {
		name, value string
	} {
		return []struct{ name, value string }{
			{label + " literal Z (millis)", t0.UTC().Format("2006-01-02T15:04:05.000Z")},
			{label + " +00:00 offset form", t0.UTC().Format("2006-01-02T15:04:05.000-07:00")},
			{label + " microsecond +00:00", t0.UTC().Format("2006-01-02T15:04:05.000000-07:00")},
		}
	}

	cases := []struct {
		field    string
		variants []struct{ name, value string }
	}{
		{"created_at", variants("created_at", createdAt)},
		{"updated_at", variants("updated_at", updatedAt)},
	}
	for _, c := range cases {
		for _, v := range c.variants {
			t.Run(v.name, func(t *testing.T) {
				body := fmt.Sprintf(`{%q:%q}`, c.field, v.value)
				patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(body)))
				patchReq.Header.Set("Content-Type", "application/json")
				patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, patchReq)
				require.Equal(t, http.StatusOK, rec.Code,
					"%s = %s must be 200 (same instant as server state): %s",
					c.field, v.value, rec.Body.String())
			})
		}
	}
}

// TRA-710 (BB33 F2): `tags` follows the uniform accept-if-matches /
// reject-if-differs rule on locations. Pre-TRA-710 any `tags` presence
// was rejected with 400 invalid_value regardless of value.
func TestPatchLocation_TagsDiffersFromCurrent_Rejected400(t *testing.T) {
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
		{"tags null vs current []", `{"tags":null}`},
		{"tags with values vs current []", `{"tags":[{"tag_type":"rfid","value":"bar"}]}`},
		{"tags with values alongside name", `{"name":"TagsRej renamed","tags":[{"tag_type":"rfid","value":"bar"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, patchReq)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"differing tags in PATCH body must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
			assert.Equal(t, "tags", resp.Error.Fields[0].Field)
			assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
			assert.Contains(t, resp.Error.Fields[0].Message,
				"POST /api/v1/locations/{location_id}/tags")

			var tagCount int
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT count(*) FROM trakrf.tags WHERE location_id = $1 AND deleted_at IS NULL`, id).Scan(&tagCount))
			assert.Equal(t, 0, tagCount, "rejected PATCH must not have mutated tags")
		})
	}
}

// TRA-710 (BB33 F2): matching tags echo silently strips and returns 200.
// Mirrors TestPatchAsset_TagsMatchesCurrent_200.
func TestPatchLocation_TagsMatchesCurrent_200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	t.Run("empty-tags echo", func(t *testing.T) {
		id := seedLocationRoundTrip(t, pool, orgID, "LOC-TAGS-MATCH-EMPTY", "TagsMatchEmpty")
		patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(`{"tags":[]}`)))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, patchReq)
		require.Equal(t, http.StatusOK, rec.Code,
			"matching empty tags must be 200: %s", rec.Body.String())
	})

	t.Run("populated-tags echo", func(t *testing.T) {
		id := seedLocationRoundTrip(t, pool, orgID, "LOC-TAGS-MATCH-FULL", "TagsMatchFull")
		var tagID int
		require.NoError(t, pool.QueryRow(context.Background(), `
			INSERT INTO trakrf.tags (org_id, type, value, location_id, is_active)
			VALUES ($1, 'rfid', 'V-LOC-MATCH', $2, true) RETURNING id
		`, orgID, id).Scan(&tagID))

		getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
		getReq = withLocationRoundTripOrgContext(getReq, orgID)
		getRec := httptest.NewRecorder()
		router.ServeHTTP(getRec, getReq)
		require.Equal(t, http.StatusOK, getRec.Code)
		var getResp struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
		tagsRaw, err := json.Marshal(getResp.Data["tags"])
		require.NoError(t, err)

		patchBody := []byte(fmt.Sprintf(`{"tags":%s}`, string(tagsRaw)))
		patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(patchBody))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, patchReq)
		require.Equal(t, http.StatusOK, rec.Code,
			"verbatim GET tags echo must be 200: %s", rec.Body.String())
	})
}

// TRA-686 / BB29 F8: `external_key` in a PATCH body is rejected with 400
// read_only pointing at /locations/{id}/rename.
func TestPatchLocation_ExternalKeyRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-EK-REJ", "EKRej")

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"only external_key", `{"external_key":"LOC-9999"}`},
		{"external_key alongside name", `{"name":"x","external_key":"LOC-9999"}`},
		{"external_key null", `{"external_key":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, patchReq)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"external_key in PATCH body must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
			assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
			assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
			assert.Contains(t, resp.Error.Fields[0].Message,
				"POST /api/v1/locations/{location_id}/rename")

			var ek string
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT external_key FROM trakrf.locations WHERE id = $1`, id).Scan(&ek))
			assert.Equal(t, "LOC-EK-REJ", ek, "rejected PATCH must not have mutated external_key")
		})
	}
}

// TRA-719 / BB35 B2: parent_external_key is writable on PATCH —
// superseding the TRA-686 / TRA-713 read-only behavior tested here
// previously. New surface lives in patch_natural_key_integration_test.go:
//
//   - TestPatchLocation_NaturalKey_ParentExternalKey_ReParents200
//   - TestPatchLocation_NaturalKey_ParentExternalKey_NotFound400
//   - TestPatchLocation_NaturalKey_ParentExternalKey_NullClears200
//   - TestPatchLocation_NaturalKey_ParentBoth400_Ambiguous

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
		{"valid_from Unix epoch", "valid_from", `"1970-01-01T00:00:00Z"`},
		{"valid_to date-only", "valid_to", `"2027-05-10"`},
		{"valid_to slashes", "valid_to", `"2027/05/10"`},
		{"valid_to empty string", "valid_to", `""`},
		{"valid_to Go zero-time", "valid_to", `"0001-01-01T00:00:00Z"`},
		{"valid_to Unix epoch", "valid_to", `"1970-01-01T00:00:00Z"`},
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

// TRA-692 §1.2: POST with empty body must report missing `name` as
// code=required (no min_length param). Length-bearing required fields
// that are absent from the body are presence-class violations, not
// length-class — the presence overlay promotes the validator's collapsed
// too_short back to `required`. Empty strings on the same field stay as
// too_short. Supersedes TRA-675's "always too_short" framing.
func TestPostLocation_MissingNameEmitsRequired(t *testing.T) {
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
	assert.Equal(t, "required", resp.Error.Fields[0].Code,
		"omitted length-bearing required field must be `required`, not `too_short` (TRA-692 §1.2)")
	assert.Nil(t, resp.Error.Fields[0].Params,
		"promoted `required` carries no params")

	_ = pool
}

// TRA-705 (BB32 §C6): POST with explicit `valid_from: null` is rejected
// with 400 validation_error. Supersedes TRA-675 — the Create-only
// nullable carve-out is gone; omit valid_from to use the server default.
func TestPostLocation_NullValidFrom_Rejected400(t *testing.T) {
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

	require.Equal(t, http.StatusBadRequest, rec.Code, "POST valid_from null must be 400: %s", rec.Body.String())

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
	assert.Equal(t, "valid_from", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)

	_ = pool
}

// TRA-705 (BB32 §C6): POST with explicit `is_active: null` is rejected
// with 400 validation_error — omit is_active to use the server default.
func TestPostLocation_NullIsActive_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body := []byte(`{"external_key":"LOC-NULL-IA","name":"n","is_active":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "POST is_active null must be 400: %s", rec.Body.String())

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
	assert.Equal(t, "is_active", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}

// TRA-705 (BB32 §C6 / D3): multiple null-on-non-nullable violations on
// the same POST body must all be reported in one round trip.
func TestPostLocation_NullMultiField_AllReported(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	body := []byte(`{"name":"n","valid_from":null,"is_active":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp struct {
		Error struct {
			Detail string `json:"detail"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 2)
	for _, f := range resp.Error.Fields {
		assert.Equal(t, "invalid_value", f.Code)
	}
	assert.Contains(t, resp.Error.Detail, "(and 1 more validation error",
		"detail must carry the multi-field suffix")
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

// TRA-775 (BB61-3 F1): `tags` PATCH echo on locations is now compared as a
// set on full tag content, mirroring the asset behavior. A submitted array
// with the same tag content as the current state matches regardless of
// order; differing set membership or differing field values on a matching
// id still returns 400 read_only.
func TestPatchLocation_TagsSetEqualityEcho(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedLocationRoundTrip(t, pool, orgID, "LOC-TAGS-SETEQ", "TagsSetEq")

	type seedTag struct {
		tagType string
		value   string
	}
	seeded := []seedTag{
		{"rfid", "V-LOC-ALPHA"},
		{"rfid", "V-LOC-BRAVO"},
		{"ble", "V-LOC-CHARLIE"},
	}
	ids := make([]int, 0, len(seeded))
	for _, s := range seeded {
		var tagID int
		require.NoError(t, pool.QueryRow(context.Background(), `
			INSERT INTO trakrf.tags (org_id, type, value, location_id, is_active)
			VALUES ($1, $2, $3, $4, true) RETURNING id
		`, orgID, s.tagType, s.value, id).Scan(&tagID))
		ids = append(ids, tagID)
	}

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	getReq = withLocationRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, "GET location failed: %s", getRec.Body.String())

	var getResp struct {
		Data struct {
			Tags []map[string]any `json:"tags"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	require.Len(t, getResp.Data.Tags, 3, "location must report all three seeded tags")

	current := getResp.Data.Tags
	currentJSON, err := json.Marshal(current)
	require.NoError(t, err)

	reversed := []map[string]any{current[2], current[1], current[0]}
	reversedJSON, err := json.Marshal(reversed)
	require.NoError(t, err)

	rotated := []map[string]any{current[1], current[2], current[0]}
	rotatedJSON, err := json.Marshal(rotated)
	require.NoError(t, err)

	extraTag := map[string]any{"id": ids[2] + 1000, "tag_type": "rfid", "value": "V-LOC-EXTRA"}
	tooMany := append([]map[string]any{}, current...)
	tooMany = append(tooMany, extraTag)
	tooManyJSON, err := json.Marshal(tooMany)
	require.NoError(t, err)

	swappedID := []map[string]any{
		current[0],
		current[1],
		{"id": ids[2] + 999, "tag_type": current[2]["tag_type"], "value": current[2]["value"]},
	}
	swappedIDJSON, err := json.Marshal(swappedID)
	require.NoError(t, err)

	wrongTagType := []map[string]any{
		current[0],
		{"id": current[1]["id"], "tag_type": "barcode", "value": current[1]["value"]},
		current[2],
	}
	wrongTagTypeJSON, err := json.Marshal(wrongTagType)
	require.NoError(t, err)

	wrongValue := []map[string]any{
		current[0],
		{"id": current[1]["id"], "tag_type": current[1]["tag_type"], "value": "MUTATED"},
		current[2],
	}
	wrongValueJSON, err := json.Marshal(wrongValue)
	require.NoError(t, err)

	patchTags := func(t *testing.T, tagsJSON []byte) *httptest.ResponseRecorder {
		t.Helper()
		body := []byte(fmt.Sprintf(`{"tags":%s}`, string(tagsJSON)))
		patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader(body))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq = withLocationRoundTripOrgContext(patchReq, orgID)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, patchReq)
		return rec
	}

	requireReadOnlyTagsRejection := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 read_only, got %d: %s", rec.Code, rec.Body.String())
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
		assert.Equal(t, "tags", resp.Error.Fields[0].Field)
		assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
		assert.Contains(t, resp.Error.Fields[0].Message, "POST /api/v1/locations/{location_id}/tags",
			"message must name the subresource endpoint")
	}

	t.Run("in-order echo 200 (regression)", func(t *testing.T) {
		rec := patchTags(t, currentJSON)
		require.Equal(t, http.StatusOK, rec.Code, "in-order tags echo must remain 200: %s", rec.Body.String())
	})
	t.Run("reverse-order echo 200 (new)", func(t *testing.T) {
		rec := patchTags(t, reversedJSON)
		require.Equal(t, http.StatusOK, rec.Code, "reverse-order tags echo must be 200 under set-equality: %s", rec.Body.String())
	})
	t.Run("rotated-order echo 200 (new)", func(t *testing.T) {
		rec := patchTags(t, rotatedJSON)
		require.Equal(t, http.StatusOK, rec.Code, "rotated tags echo must be 200 under set-equality: %s", rec.Body.String())
	})
	t.Run("length mismatch (4 vs 3) 400 read_only", func(t *testing.T) {
		requireReadOnlyTagsRejection(t, patchTags(t, tooManyJSON))
	})
	t.Run("same length different id set 400 read_only", func(t *testing.T) {
		requireReadOnlyTagsRejection(t, patchTags(t, swappedIDJSON))
	})
	t.Run("same ids wrong tag_type 400 read_only", func(t *testing.T) {
		requireReadOnlyTagsRejection(t, patchTags(t, wrongTagTypeJSON))
	})
	t.Run("same ids wrong value 400 read_only", func(t *testing.T) {
		requireReadOnlyTagsRejection(t, patchTags(t, wrongValueJSON))
	})

	rows, err := pool.Query(context.Background(), `
		SELECT id, type, value FROM trakrf.tags
		WHERE location_id = $1 AND deleted_at IS NULL
		ORDER BY id
	`, id)
	require.NoError(t, err)
	defer rows.Close()
	var persisted []struct {
		ID    int
		Type  string
		Value string
	}
	for rows.Next() {
		var r struct {
			ID    int
			Type  string
			Value string
		}
		require.NoError(t, rows.Scan(&r.ID, &r.Type, &r.Value))
		persisted = append(persisted, r)
	}
	require.NoError(t, rows.Err())
	require.Len(t, persisted, 3, "tag set must be unchanged after echo PATCHes")
	for i, want := range seeded {
		assert.Equal(t, ids[i], persisted[i].ID)
		assert.Equal(t, want.tagType, persisted[i].Type)
		assert.Equal(t, want.value, persisted[i].Value)
	}
}
