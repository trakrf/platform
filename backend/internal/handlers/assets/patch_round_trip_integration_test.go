//go:build integration
// +build integration

// TRA-608 / BB18 §1.7: GET → PUT round-trip must succeed. The PUT handler
// strips the round-trip-safe read-only fields on PublicAssetView (id,
// created_at, updated_at) from the request body before strict-decoding so
// a naive read-mutate-write client doesn't trip over schema asymmetry.
// Typo'd fields not in that drop set still produce a 400 validation_error.
//
// TRA-643 / BB22 F1: `tags` is managed via the /assets/{id}/tags subresource,
// not the parent PUT. The validator rejects a `tags` body field with 400
// invalid_value rather than silently dropping it — the silent-drop pattern
// hid bugs in read-modify-write integrations.
//
// TRA-610 / BB18 §1.8: description and valid_to are always emitted (null
// when unset) on the response.

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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupRoundTripRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets/{asset_id}", handler.GetAsset)
	r.Patch("/api/v1/assets/{asset_id}", handler.Update)
	return r
}

func withRoundTripOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra608@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedRoundTripAsset(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, extKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

// TRA-608 / TRA-643 / TRA-674 acceptance: GET /api/v1/assets/{id} →
// mutate one field → PATCH the full body back succeeds with 200. The
// server silently strips every read-only field (id, created_at, updated_at,
// asset_deleted_at, external_key, tags) so the naive read-mutate-write
// integrator flow works without per-field client-side scrubbing.
func TestPutAsset_GETBodyRoundTrip_Succeeds(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "FORK-007", "Forklift 7")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	getReq = withRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var getResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))

	// Sanity: read-only and always-emit fields must be present on the GET.
	for _, field := range []string{"id", "created_at", "updated_at", "tags", "description", "valid_to"} {
		_, present := getResp.Data[field]
		assert.True(t, present, "GET response must include %q (TRA-608/610)", field)
	}

	// Mutate name and PATCH the full body back. Every read-only field
	// (id, created_at, updated_at, asset_deleted_at, external_key, tags)
	// is silently stripped server-side (TRA-674 / BB27 F3); the client
	// does not need to scrub them out first.
	getResp.Data["name"] = "Forklift 7 (renamed)"
	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "PUT round-trip must succeed: %s", putRec.Body.String())

	var putResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	assert.Equal(t, "Forklift 7 (renamed)", putResp.Data["name"])
}

// Strict-unknown-field still applies for fields that aren't readOnly in
// the spec — a typo'd field name still returns 400 validation_error.
func TestPutAsset_TypoFieldStillRejected(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "FORK-008", "Forklift 8")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{"name":"x","nme":"oops"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusBadRequest, putRec.Code, "typo'd field must still be rejected")

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Detail string `json:"detail"`
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

// TRA-610 acceptance: GET response always emits description and valid_to
// (null when unset). Verifies the wire shape on a freshly-created asset
// with no description / no valid_to.
func TestGetAsset_OptionalFieldsAlwaysEmittedNullWhenUnset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "FORK-009", "Forklift 9")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	req = withRoundTripOrgContext(req, orgID)
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

// TRA-614 / BB19 §S1: PUT with explicit `null` on read-side-nullable fields
// must succeed and clear the column. valid_to was already correct via TRA-468;
// description, location_id, location_external_key were the new additions.
func TestPutAsset_NullClearsReadSideNullableFields(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	// Seed an asset with a populated description and a location, then PUT
	// every nullable field as null — round-trip should succeed and clear.
	var locID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, 'LOC-FOR-NULL', 'loc-for-null', '', $2, true) RETURNING id
	`, orgID, time.Now().UTC()).Scan(&locID)
	require.NoError(t, err)

	var assetID int
	vt := time.Now().UTC().Add(24 * time.Hour)
	err = pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets
		  (org_id, external_key, name, description, current_location_id, valid_from, valid_to, is_active)
		VALUES ($1, 'ASSET-NULL-PUT', 'NullPut', 'has description', $2, $3, $4, true) RETURNING id
	`, orgID, locID, time.Now().UTC(), vt).Scan(&assetID)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{
		"description": null,
		"location_id": null,
		"location_external_key": null,
		"valid_to": null
	}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", assetID), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "PUT null on nullable fields must succeed: %s", putRec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
	assert.Nil(t, resp.Data["description"], "description cleared")
	assert.Nil(t, resp.Data["location_id"], "location_id cleared")
	assert.Nil(t, resp.Data["location_external_key"], "location_external_key cleared")
	assert.Nil(t, resp.Data["valid_to"], "valid_to cleared")

	// Verify storage: current_location_id is NULL, valid_to is NULL,
	// description is empty (read-side projects "" → null per TRA-610).
	var dbLoc, dbValidTo *string
	var dbDesc string
	err = pool.QueryRow(context.Background(),
		`SELECT description, current_location_id::text, valid_to::text FROM trakrf.assets WHERE id = $1`,
		assetID).Scan(&dbDesc, &dbLoc, &dbValidTo)
	require.NoError(t, err)
	assert.Equal(t, "", dbDesc)
	assert.Nil(t, dbLoc)
	assert.Nil(t, dbValidTo)
}

// TRA-614: GET → null-mutate → PUT round-trip is a wire-level
// regression test for the §S2 type-error scenario from the ticket.
func TestPutAsset_GETToPUTRoundTripWithNulls(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-RT-NULL", "round-trip null")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	getReq = withRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var getResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	// description, location_id, location_external_key, valid_to should be
	// JSON null (asset was seeded with empty description and no location).
	assert.Nil(t, getResp.Data["description"])
	assert.Nil(t, getResp.Data["location_id"])
	assert.Nil(t, getResp.Data["location_external_key"])
	assert.Nil(t, getResp.Data["valid_to"])

	// PATCH-back of the verbatim GET body — the connector flow from §S2.
	// TRA-674 / BB27 F3: tags and external_key are now silently stripped
	// server-side along with id/created_at/updated_at/asset_deleted_at.
	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "GET → PUT round-trip with explicit nulls must succeed: %s", putRec.Body.String())
}

// TRA-614: location_id null + location_external_key value (or vice versa)
// is a 400 conflict, not a silent clear.
func TestPutAsset_LocationNullVsValueIsConflict(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-CONFLICT", "conflict")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{"location_id": null, "location_external_key": "WHS-99"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusBadRequest, putRec.Code, "null/value conflict on location pair must be 400: %s", putRec.Body.String())
}

// TRA-615 / BB19 §S5: external_key with reserved punctuation (space, slash,
// colon, period, underscore) is rejected at the validator boundary with 400
// invalid_value rather than reaching storage and triggering 500.
func TestPostAsset_BadExternalKeyPattern_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	cases := []struct {
		name string
		key  string
	}{
		{"space", "BB With Spaces"},
		{"slash", "BB/slash"},
		{"colon", "BB:colon"},
		{"period", "BB.period"},
		{"underscore", "BB_underscore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"external_key": tc.key,
				"name":         "n",
			})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
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

// TRA-650 / BB23 F3: POST /api/v1/assets must reject an explicit empty
// external_key with 400 too_short, mirroring the PUT validator's min=1 on
// UpdateAssetRequest.external_key. Absence of the key still triggers the
// auto-mint of ASSET-NNNN.
func TestPostAsset_EmptyExternalKey_Rejected400(t *testing.T) {
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
		"external_key": "",
		"name":         "n",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
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

// TRA-650 / BB23 F3: when external_key is omitted from the body, the server
// continues to auto-mint an ASSET-NNNN value — the legitimate "omit means
// auto-mint" path is preserved.
func TestPostAsset_OmittedExternalKey_AutoMints(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body, err := json.Marshal(map[string]any{"name": "auto-mint-me"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "omitted external_key must auto-mint: %s", rec.Body.String())

	var resp struct {
		Data struct {
			ExternalKey string `json:"external_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Regexp(t, `^ASSET-\d+$`, resp.Data.ExternalKey)

	_ = pool
}

// TRA-619 finding 1: a PUT body that contains only read-only fields decodes
// to an empty UpdateAssetRequest after the readOnly drop. The previous
// behavior was a "no fields to update" error surfaced as 500 internal_error.
// Expected behavior: 200 with the unchanged record (no-op write), matching
// the GET → PUT round-trip ergonomic.
//
// TRA-674: tags and external_key are now in the strip set too — see
// TestPutAsset_TagsStripped200 and TestPatchAsset_ExternalKey_Stripped200.
func TestPutAsset_OnlyReadOnlyFields_Returns200NoOp(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-RO-NOOP", "ReadOnlyNoop")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

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
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
			putRec := httptest.NewRecorder()
			router.ServeHTTP(putRec, patchReq)

			require.Equal(t, http.StatusOK, putRec.Code,
				"empty effective body must be no-op 200 (got %d): %s", putRec.Code, putRec.Body.String())

			var resp struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
			assert.Equal(t, "ReadOnlyNoop", resp.Data["name"], "name unchanged")
			assert.Equal(t, "ASSET-RO-NOOP", resp.Data["external_key"], "external_key unchanged")
		})
	}
}

// TRA-674 / BB27 F3: `tags` in a PATCH body is silently stripped, mirroring
// id / created_at / updated_at / external_key. Tag mutation still goes
// through POST/DELETE /assets/{id}/tags; the PATCH body just tolerates the
// read-only field so a verbatim GET → PATCH round-trip succeeds. Previously
// (TRA-643 / BB22 F1) a `tags` key surfaced as 400 invalid_value, but the
// strip-vs-reject rule was reversed pre-launch in favor of the more
// generator-friendly shape — see PublicReadOnlyFields.
func TestPutAsset_TagsStripped200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-TAGS-STRIP", "TagsStrip")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

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
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
			putRec := httptest.NewRecorder()
			router.ServeHTTP(putRec, patchReq)

			require.Equal(t, http.StatusOK, putRec.Code,
				"tags in PATCH body must be 200 silent-strip (got %d): %s", putRec.Code, putRec.Body.String())

			// Tag set on the persisted row never changed — the seed inserts
			// none, and the PATCH did not touch the tag subresource.
			var tagCount int
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT count(*) FROM trakrf.tags WHERE asset_id = $1 AND deleted_at IS NULL`, id).Scan(&tagCount))
			assert.Equal(t, 0, tagCount, "PATCH must not mutate tag subresource")
		})
	}
}

// TRA-619 finding 2: metadata is `*any` on the Go struct but the public spec
// declares it `type: object`. JSON-decoded scalars/arrays previously flowed
// through to the jsonb column and got rejected by Postgres as 500. Expected:
// 400 validation_error / invalid_value at the validator boundary.
func TestPutAsset_MetadataNonObject_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-META-TYPE", "MetaType")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"string", `{"metadata":"not-an-object"}`},
		{"number", `{"metadata":42}`},
		{"bool", `{"metadata":true}`},
		{"array", `{"metadata":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
			putRec := httptest.NewRecorder()
			router.ServeHTTP(putRec, patchReq)

			require.Equal(t, http.StatusBadRequest, putRec.Code,
				"non-object metadata must be 400 (got %d): %s", putRec.Code, putRec.Body.String())

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
			assert.Equal(t, "metadata", resp.Error.Fields[0].Field)
			assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
		})
	}
}

// TRA-649 / BB23 F2: POST /api/v1/assets must reject loose date forms on
// valid_from / valid_to. The body validator now matches the strict
// query-param validator on /assets/{id}/history and the spec's
// `format: date-time` declaration. Slash-separated dates, date-only
// strings, empty strings, and the Go zero-time literal previously
// round-tripped silently (slash forms got normalised; empty / zero values
// got substituted with the request creation timestamp). Each must now
// surface as 400 validation_error keyed on the offending field.
func TestPostAsset_LooseDateForms_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	cases := []struct {
		name      string
		field     string
		bodyValue string
	}{
		{"valid_from date-only", "valid_from", `"2026-05-10"`},
		{"valid_from US slashes", "valid_from", `"05/10/2026"`},
		{"valid_from ISO slashes", "valid_from", `"2026/05/10"`},
		{"valid_from empty string", "valid_from", `""`},
		{"valid_from Go zero-time", "valid_from", `"0001-01-01T00:00:00Z"`},
		{"valid_to date-only", "valid_to", `"2027-05-10"`},
		{"valid_to slashes", "valid_to", `"2027/05/10"`},
		{"valid_to empty string", "valid_to", `""`},
		{"valid_to Go zero-time", "valid_to", `"0001-01-01T00:00:00Z"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"external_key":"ASSET-LOOSE-%s","name":"loose","%s":%s}`,
				tc.name, tc.field, tc.bodyValue)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
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

// TRA-649 / BB23 F2: PUT /api/v1/assets/{id} mirrors POST — body validator
// rejects loose date forms with 400 validation_error.
func TestPutAsset_LooseDateForms_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-PUT-LOOSE", "PutLoose")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

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
			body := fmt.Sprintf(`{"%s":%s}`, tc.field, tc.bodyValue)
			req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

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
}

// TRA-649: omitting valid_from on POST /api/v1/assets continues to default
// to the request creation timestamp. The legitimate "absent means
// server-defaults" path is preserved — only silent coercion of *explicit*
// values went away.
func TestPostAsset_OmittedValidFrom_DefaultsToNow(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body := []byte(`{"external_key":"ASSET-DEFAULT-VF","name":"defaultvf"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "omitted valid_from must default-to-now: %s", rec.Body.String())

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

// TRA-619: an object metadata is still accepted (regression guard for the
// type-check above — it must not reject the legitimate happy path).
func TestPutAsset_MetadataObject_Accepted(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-META-OK", "MetaOk")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{"metadata":{"foo":"bar","n":1}}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code,
		"object metadata must be accepted: %s", putRec.Body.String())
}
