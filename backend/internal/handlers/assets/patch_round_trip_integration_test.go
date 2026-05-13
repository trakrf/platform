//go:build integration
// +build integration

// TRA-608 / BB18 §1.7: GET → PATCH round-trip must succeed. The PATCH
// handler strips the round-trip-safe read-only fields on PublicAssetView
// (id, created_at, updated_at, deleted_at, location_external_key) from
// the request body before strict-decoding so a naive read-mutate-write
// client doesn't trip over schema asymmetry. Typo'd fields not in that
// drop set still produce a 400 validation_error.
//
// TRA-686 / BB29 F7+F8: `tags` (managed via /assets/{id}/tags) is
// rejected with 400 invalid_value, and `external_key` (managed via
// /assets/{id}/rename) is rejected with 400 read_only. The strip-on-PATCH
// rule from TRA-674 was reversed because silent-drop hid
// read-modify-write bugs.
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

// TRA-608 / TRA-686 acceptance: GET /api/v1/assets/{id} → strip the
// pre-decode-rejected fields (`tags`, `external_key`) → mutate one field
// → PATCH succeeds with 200. The round-trip-safe read-only fields
// (id, created_at, updated_at, deleted_at, location_external_key) remain
// on the body to exercise the silent-strip drop list.
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

	// Mutate name and PATCH back. `tags` and `external_key` are
	// pre-decode rejected (TRA-686) and must be stripped client-side
	// before re-sending. The round-trip-safe read-only fields stay on
	// the body to exercise the silent-drop list.
	getResp.Data["name"] = "Forklift 7 (renamed)"
	delete(getResp.Data, "tags")
	delete(getResp.Data, "external_key")
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
	assert.Equal(t, "unknown_field", resp.Error.Fields[0].Code)
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

// TRA-614 / BB19 §S1 + TRA-699: PATCH with explicit `null` on
// read-side-nullable fields. `description` and `valid_to` clear the
// underlying column when sent as null. `location_id` and
// `location_external_key` are no longer writable on PATCH (TRA-699 supersedes
// the TRA-614 clear-via-null path for location); a `null` body on those
// fields is allowed only when the current resource state is already null
// (matched echo). Mutations to asset location move to the scan-event path.
func TestPutAsset_NullClearsReadSideNullableFields(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	// Seed an asset with a populated description, populated valid_to, and
	// no location (so the natural-key null/null echo case is exercised
	// alongside the description / valid_to clear paths).
	var assetID int
	vt := time.Now().UTC().Add(24 * time.Hour)
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets
		  (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, 'ASSET-NULL-PUT', 'NullPut', 'has description', $2, $3, true) RETURNING id
	`, orgID, time.Now().UTC(), vt).Scan(&assetID)
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

	require.Equal(t, http.StatusOK, putRec.Code, "PATCH null on nullable fields must succeed: %s", putRec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
	assert.Nil(t, resp.Data["description"], "description cleared")
	assert.Nil(t, resp.Data["location_id"], "location_id stays null (null/null echo)")
	assert.Nil(t, resp.Data["location_external_key"], "location_external_key stays null (null/null echo)")
	assert.Nil(t, resp.Data["valid_to"], "valid_to cleared")

	// Verify storage: valid_to is NULL, description is empty (read-side
	// projects "" → null per TRA-610). current_location_id was already
	// null and remains null.
	var dbValidTo *string
	var dbDesc string
	err = pool.QueryRow(context.Background(),
		`SELECT description, valid_to::text FROM trakrf.assets WHERE id = $1`,
		assetID).Scan(&dbDesc, &dbValidTo)
	require.NoError(t, err)
	assert.Equal(t, "", dbDesc)
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

	// PATCH-back of the GET body — the connector flow from §S2.
	// TRA-686 / BB29 F7+F8: tags and external_key are pre-decode
	// rejected with 400, so a verbatim PATCH-back requires stripping
	// them client-side first.
	delete(getResp.Data, "tags")
	delete(getResp.Data, "external_key")
	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusOK, putRec.Code, "GET → PUT round-trip with explicit nulls must succeed: %s", putRec.Body.String())
}

// TRA-699 supersedes TRA-681 / TRA-614: location_id and
// location_external_key are no longer writable on PATCH (record-of-origin
// posture; mutate via scan events). A body like
// `{"location_id": null, "location_external_key": "WHS-99"}` on an asset
// with a current location now returns 400 read_only on BOTH fields:
// location_id null ≠ non-null current, and location_external_key
// "WHS-99" ≠ matching current. The FK is not cleared. See
// patch_natural_key_integration_test.go for the matched-echo 200 path
// and the differs 400 path.
func TestPatchAsset_LocationFieldsOnPATCH_NonMatchingDiffers400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	// Seed an asset that starts with a real location.
	var locID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, 'LOC-FOR-STRIP', 'loc-for-strip', '', $2, true) RETURNING id
	`, orgID, time.Now().UTC()).Scan(&locID)
	require.NoError(t, err)

	var assetID int
	err = pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets
		  (org_id, external_key, name, description, current_location_id, valid_from, is_active)
		VALUES ($1, 'ASSET-STRIP-CLEAR', 'StripClear', '', $2, $3, true) RETURNING id
	`, orgID, locID, time.Now().UTC()).Scan(&assetID)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{"location_id": null, "location_external_key": "WHS-99"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", assetID), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, patchReq)

	require.Equal(t, http.StatusBadRequest, putRec.Code,
		"PATCH with differing location_id+external_key must 400 read_only on both: %s", putRec.Body.String())

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
	require.Len(t, resp.Error.Fields, 2)
	for _, f := range resp.Error.Fields {
		assert.Equal(t, "read_only", f.Code, "field %s must carry read_only", f.Field)
		assert.Contains(t, []string{"location_id", "location_external_key"}, f.Field)
	}

	// FK unchanged on disk.
	var dbLoc *int
	err = pool.QueryRow(context.Background(),
		`SELECT current_location_id FROM trakrf.assets WHERE id = $1`, assetID).Scan(&dbLoc)
	require.NoError(t, err)
	require.NotNil(t, dbLoc)
	assert.Equal(t, locID, *dbLoc, "FK must not have changed on rejected PATCH")
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

// TRA-686 / BB29 F7: `tags` in a PATCH body is rejected with 400
// invalid_value pointing at the /assets/{id}/tags subresource. The
// silent-drop default (TRA-674) hid bugs in read-modify-write integrations
// where the caller believed the tag write took effect.
func TestPatchAsset_TagsRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-TAGS-REJ", "TagsRej")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"empty array", `{"tags":[]}`},
		{"tags with values", `{"tags":[{"key":"foo","value":"bar"}]}`},
		{"tags alongside name", `{"name":"TagsRej renamed","tags":[]}`},
		{"tags null", `{"tags":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, patchReq)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"tags in PATCH body must be 400 (got %d): %s", rec.Code, rec.Body.String())

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
			assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
			assert.Contains(t, resp.Error.Fields[0].Message,
				"POST /api/v1/assets/{asset_id}/tags",
				"message must name the subresource endpoint")

			// The asset's persisted tag set must not have been mutated.
			var tagCount int
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT count(*) FROM trakrf.tags WHERE asset_id = $1 AND deleted_at IS NULL`, id).Scan(&tagCount))
			assert.Equal(t, 0, tagCount, "rejected PATCH must not have mutated tags")
		})
	}
}

// TRA-686 / BB29 F8: `external_key` in a PATCH body is rejected with 400
// read_only pointing at the rename endpoint (POST /assets/{id}/rename).
// Silent-drop would let an integrator believe a rename PATCH succeeded
// while the natural key — the join key downstream systems rely on —
// stayed unchanged.
func TestPatchAsset_ExternalKeyRejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-EK-REJ", "ExtKeyRej")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"only external_key", `{"external_key":"ASSET-9999"}`},
		{"external_key alongside name", `{"name":"x","external_key":"ASSET-9999"}`},
		{"external_key null", `{"external_key":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
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
				"POST /api/v1/assets/{asset_id}/rename",
				"message must name the rename endpoint")

			// The asset's external_key must not have been mutated.
			var ek string
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT external_key FROM trakrf.assets WHERE id = $1`, id).Scan(&ek))
			assert.Equal(t, "ASSET-EK-REJ", ek, "rejected PATCH must not have mutated external_key")
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
		{"valid_from Unix epoch", "valid_from", `"1970-01-01T00:00:00Z"`},
		{"valid_to date-only", "valid_to", `"2027-05-10"`},
		{"valid_to slashes", "valid_to", `"2027/05/10"`},
		{"valid_to empty string", "valid_to", `""`},
		{"valid_to Go zero-time", "valid_to", `"0001-01-01T00:00:00Z"`},
		{"valid_to Unix epoch", "valid_to", `"1970-01-01T00:00:00Z"`},
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
		{"valid_from Unix epoch", "valid_from", `"1970-01-01T00:00:00Z"`},
		{"valid_to date-only", "valid_to", `"2027-05-10"`},
		{"valid_to slashes", "valid_to", `"2027/05/10"`},
		{"valid_to empty string", "valid_to", `""`},
		{"valid_to Go zero-time", "valid_to", `"0001-01-01T00:00:00Z"`},
		{"valid_to Unix epoch", "valid_to", `"1970-01-01T00:00:00Z"`},
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

// TRA-675 / BB27 F4: PATCH `{"description":""}` must be rejected with
// 400 too_short / min_length=1 instead of silently coercing to null in
// the response. Adjacent string fields (name, external_key) already
// reject empty string cleanly; description now matches.
func TestPutAsset_EmptyDescription_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-DESC-EMPTY", "DescEmpty")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{"description":""}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
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

// TRA-675 / BB27 F4: POST mirrors PATCH — explicit empty description is
// rejected with 400 too_short.
func TestPostAsset_EmptyDescription_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body := []byte(`{"external_key":"ASSET-DESC-POST-EMPTY","name":"n","description":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
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

// TRA-675 / BB27 F5: POST with no body (and thus no `name`) must report
// the length-bearing required field as code=too_short with min_length,
// not code=required. errors.mdx is authoritative: missing length-bearing
// fields are too_short whether sent as empty or omitted. This fixes the
// previously inconsistent envelope where /assets POST emitted `required`
// but /assets/{id}/tags POST and /assets/{id}/rename POST emitted
// `too_short` for the same condition.
func TestPostAsset_MissingNameEmitsTooShort(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
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

// TRA-705 (BB32 §C6): POST with explicit `valid_from: null` is rejected
// with 400 validation_error. Supersedes TRA-675: the prior carve-out
// (nullable on Create only, "null-as-now" alias for omission) created a
// documented Create/Update asymmetry that integrators tripped on. Pre-
// launch we tighten — omit valid_from to use the server default.
func TestPostAsset_NullValidFrom_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body := []byte(`{"external_key":"ASSET-NULL-VF","name":"n","valid_from":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
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
// with 400 validation_error — omit is_active to use the server default
// (true), or send a boolean.
func TestPostAsset_NullIsActive_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body := []byte(`{"external_key":"ASSET-NULL-IA","name":"n","is_active":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
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

// TRA-705 (BB32 §C6): POST with explicit `metadata: null` is rejected
// with 400 validation_error — omit metadata to use the server default
// (no metadata), or send an object.
func TestPostAsset_NullMetadata_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body := []byte(`{"external_key":"ASSET-NULL-MD","name":"n","metadata":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "POST metadata null must be 400: %s", rec.Body.String())

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
	assert.Equal(t, "metadata", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
}

// TRA-705 (BB32 §C6 / D3): multiple null-on-non-nullable violations on
// the same POST body must all be reported in one round trip.
func TestPostAsset_NullMultiField_AllReported(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	body := []byte(`{"name":"n","valid_from":null,"is_active":null,"metadata":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
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
	require.Len(t, resp.Error.Fields, 3)
	for _, f := range resp.Error.Fields {
		assert.Equal(t, "invalid_value", f.Code)
	}
	assert.Contains(t, resp.Error.Detail, "(and 2 more validation errors)",
		"detail must carry the multi-field suffix")
}

// TRA-675: PATCH keeps rejecting valid_from null. On update there is no
// "use server default" semantic — explicit null would mean "reset
// temporal validity to now()", which the handler treats as a malformed
// request. Documented spec asymmetry: nullable on Create, non-nullable
// on Update.
func TestPutAsset_NullValidFrom_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-NULL-VF-PUT", "NullVfPut")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	body := []byte(`{"valid_from":null}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withRoundTripOrgContext(patchReq, orgID)
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
