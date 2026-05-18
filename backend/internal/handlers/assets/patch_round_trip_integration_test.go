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

// TRA-608 / TRA-710 acceptance: GET /api/v1/assets/{id} → mutate one
// field → PATCH succeeds with 200. Under TRA-710 every read-only field —
// including `tags`, `external_key`, `id`, `created_at`, `updated_at`,
// `deleted_at`, and the location natural-key references — is accepted on
// the body when echoed verbatim from the GET (silent strip on the
// matching path). Integrators no longer need to manually scrub any
// field; full-document PATCH-back is the supported flow.
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

	// Mutate name and PATCH back with the entire GET body intact —
	// TRA-710 normalizes matching read-only fields out, no client-side
	// scrub required.
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
	// TRA-710: tags and external_key now follow the echo-or-reject rule;
	// the verbatim GET → PATCH succeeds without any client-side scrub.
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

// TRA-619 finding 1 / TRA-710 (BB33 F2): a PATCH body that contains only
// read-only fields whose values match the current resource must succeed
// with 200 (no-op write), preserving the GET → PATCH round-trip ergonomic.
// Pre-TRA-710 the four server-managed timestamps + surrogate id were
// silent-stripped regardless of value; now they follow the
// accept-if-matches, reject-if-differs rule alongside the natural-key
// fields. A body whose read-only fields differ from current is exercised
// in TestPatchAsset_ServerManagedReadOnly_Differs400.
func TestPutAsset_OnlyReadOnlyFields_MatchingCurrent_Returns200NoOp(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	// TRA-783: each accepted PATCH advances updated_at, so the cached-body
	// pattern (capture once at top, reuse across sub-tests) goes stale
	// after the first PATCH. Each sub-test seeds its own asset and re-GETs.
	type bodyFn func(t *testing.T, cur map[string]any) string
	cases := []struct {
		name string
		body bodyFn
	}{
		{"only id matches", func(t *testing.T, cur map[string]any) string {
			return fmt.Sprintf(`{"id":%v}`, cur["id"])
		}},
		{"only created_at matches", func(t *testing.T, cur map[string]any) string {
			return fmt.Sprintf(`{"created_at":%q}`, cur["created_at"])
		}},
		{"only updated_at matches", func(t *testing.T, cur map[string]any) string {
			return fmt.Sprintf(`{"updated_at":%q}`, cur["updated_at"])
		}},
		{"only deleted_at matches", func(t *testing.T, cur map[string]any) string {
			return `{"deleted_at":null}`
		}},
		{"only tags matches (empty)", func(t *testing.T, cur map[string]any) string {
			return `{"tags":[]}`
		}},
		{"empty object", func(t *testing.T, cur map[string]any) string {
			return `{}`
		}},
	}
	for i, tc := range cases {
		idx := i
		tcCopy := tc
		t.Run(tcCopy.name, func(t *testing.T) {
			id := seedRoundTripAsset(t, pool, orgID,
				fmt.Sprintf("ASSET-RO-NOOP-%d", idx), "ReadOnlyNoop")

			getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
			getReq = withRoundTripOrgContext(getReq, orgID)
			getRec := httptest.NewRecorder()
			router.ServeHTTP(getRec, getReq)
			require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
			var current struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &current))

			body := tcCopy.body(t, current.Data)
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
			putRec := httptest.NewRecorder()
			router.ServeHTTP(putRec, patchReq)

			require.Equal(t, http.StatusOK, putRec.Code,
				"matching read-only body must be 200 (got %d): %s", putRec.Code, putRec.Body.String())

			var resp struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &resp))
			assert.Equal(t, "ReadOnlyNoop", resp.Data["name"], "name unchanged")
		})
	}
}

// TRA-710 (BB33 F2): differing values for the server-managed read-only
// fields (id, created_at, updated_at, deleted_at) return 400 read_only.
// Pre-TRA-710 all four were silent-stripped regardless of value.
func TestPatchAsset_ServerManagedReadOnly_Differs400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-RO-DIFF", "ReadOnlyDiff")

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

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
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
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
// byte-equal echo. A typed client (Go time.Time, Python Pydantic, etc.)
// deserializes the GET response into a datetime and re-serializes via the
// language's default representation; that default rarely matches the
// server's millisecond-Z emit shape. Pre-TRA-721 those bodies were
// rejected with 400 read_only, breaking the GET → typed deserialize →
// PATCH round-trip the operation description promises.
func TestPatchAsset_DatetimeEncodingVariants_InstantEquality_200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	parseWireTime := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339Nano, s)
		require.NoError(t, err)
		return v
	}

	// Three encoding variants per field: literal Z (server emit shape —
	// control), "+00:00" UTC offset form (Go time.Time MarshalJSON
	// default), and microsecond-fractional "+00:00" (Pydantic v2
	// model_dump default). All three represent the same instant and must
	// round-trip as 200.
	variantNames := []string{"literal Z millis", "+00:00 offset", "microsecond +00:00"}
	variantFmts := []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05.000-07:00",
		"2006-01-02T15:04:05.000000-07:00",
	}

	// TRA-783: every accepted PATCH advances updated_at, so each sub-test
	// re-fetches the current value rather than relying on a snapshot
	// captured at the top of the test (which would go stale as soon as
	// the first PATCH lands).
	for i, vname := range variantNames {
		variantFmt := variantFmts[i]
		for _, field := range []string{"created_at", "updated_at"} {
			fieldCopy, vnameCopy := field, vname
			t.Run(fieldCopy+"/"+vnameCopy, func(t *testing.T) {
				id := seedRoundTripAsset(t, pool, orgID,
					fmt.Sprintf("ASSET-RO-INST-%d-%s", i, fieldCopy),
					"ReadOnlyInstant")

				// GET the current state for THIS sub-test — updated_at
				// changes between sub-tests under the TRA-783 rule.
				getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
				getReq = withRoundTripOrgContext(getReq, orgID)
				getRec := httptest.NewRecorder()
				router.ServeHTTP(getRec, getReq)
				require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
				var current struct {
					Data map[string]any `json:"data"`
				}
				require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &current))
				ts := parseWireTime(current.Data[fieldCopy].(string))
				value := ts.UTC().Format(variantFmt)

				body := fmt.Sprintf(`{%q:%q}`, fieldCopy, value)
				patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(body)))
				patchReq.Header.Set("Content-Type", "application/json")
				patchReq = withRoundTripOrgContext(patchReq, orgID)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, patchReq)
				require.Equal(t, http.StatusOK, rec.Code,
					"%s = %s must be 200 (same instant as server state): %s",
					fieldCopy, value, rec.Body.String())
			})
		}
	}
}

// TRA-710 (BB33 F2) / TRA-780 F4: `tags` follows the uniform
// accept-if-matches / reject-if-differs rule. A submitted `tags` value
// that does not match the asset's current tag set is rejected with 400
// invalid_context (pointing at the /assets/{id}/tags subresource). A
// submitted `tags` matching current state is silently stripped
// (round-trip ergonomic — see TestPatchAsset_TagsMatchesCurrent_200).
//
// Pre-TRA-710 behavior (TRA-686 / BB29 F7): any `tags` presence was
// rejected with 400 invalid_value regardless of value, including the
// verbatim GET → PATCH echo of `[]` against an empty current tag set.
// Pre-TRA-780 F4 the code was read_only; TRA-780 split read_only (truly
// server-managed) from invalid_context (sub-resource-mutable).
func TestPatchAsset_TagsDiffersFromCurrent_Rejected400(t *testing.T) {
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
		// current tag set is []; null differs from [].
		{"tags null vs current []", `{"tags":null}`},
		{"tags with values vs current []", `{"tags":[{"tag_type":"rfid","value":"bar"}]}`},
		{"tags with values alongside name", `{"name":"TagsRej renamed","tags":[{"tag_type":"rfid","value":"bar"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			patchReq.Header.Set("Content-Type", "application/json")
			patchReq = withRoundTripOrgContext(patchReq, orgID)
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
			assert.Equal(t, "invalid_context", resp.Error.Fields[0].Code)
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

// TRA-710 (BB33 F2): a PATCH submitting `tags` whose value matches the
// asset's current tag set is silently normalized out — 200 with no mutation.
// Two scenarios: an asset with no tags (echo `[]`) and an asset with one
// tag (echo the full tag object including server-assigned id).
func TestPatchAsset_TagsMatchesCurrent_200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	t.Run("empty-tags echo", func(t *testing.T) {
		id := seedRoundTripAsset(t, pool, orgID, "ASSET-TAGS-MATCH-EMPTY", "TagsMatchEmpty")
		patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(`{"tags":[]}`)))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq = withRoundTripOrgContext(patchReq, orgID)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, patchReq)
		require.Equal(t, http.StatusOK, rec.Code,
			"matching empty tags must be 200: %s", rec.Body.String())
	})

	t.Run("populated-tags echo", func(t *testing.T) {
		id := seedRoundTripAsset(t, pool, orgID, "ASSET-TAGS-MATCH-FULL", "TagsMatchFull")
		// Seed one tag via SQL.
		var tagID int
		require.NoError(t, pool.QueryRow(context.Background(), `
			INSERT INTO trakrf.tags (org_id, type, value, asset_id, is_active)
			VALUES ($1, 'rfid', 'V-MATCH', $2, true) RETURNING id
		`, orgID, id).Scan(&tagID))

		// GET the asset to capture the exact wire shape of `tags`.
		getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
		getReq = withRoundTripOrgContext(getReq, orgID)
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
		patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(patchBody))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq = withRoundTripOrgContext(patchReq, orgID)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, patchReq)
		require.Equal(t, http.StatusOK, rec.Code,
			"verbatim GET tags echo must be 200: %s", rec.Body.String())
	})
}

// TRA-780 F4: read_only / invalid_context semantic split on PATCH /assets.
// Server-managed fields (id, timestamps, location_id, location_external_key
// — derived from scan ingestion, no public mutation path) keep code: read_only.
// Sub-resource-mutable fields (external_key via /rename; tags via /tags
// POST/DELETE) emit code: invalid_context — same wire envelope, distinct
// code so strict-typed clients branching on code can route to the correct
// verb instead of treating both as "server-only."
func TestPatchAsset_ReadOnlyVsInvalidContext_Split_TRA780F4(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-TRA780-SPLIT", "Tra780Split")
	router := setupRoundTripRouter(NewHandler(store))

	cases := []struct {
		name     string
		body     string
		field    string
		wantCode string
	}{
		// invalid_context — sub-resource mutation paths exist
		{"external_key → invalid_context", `{"external_key":"ASSET-OTHER"}`, "external_key", "invalid_context"},
		{"tags → invalid_context", `{"tags":[{"tag_type":"rfid","value":"NEW"}]}`, "tags", "invalid_context"},
		// read_only — no public mutation path
		{"id → read_only", fmt.Sprintf(`{"id":%d}`, id+99999), "id", "read_only"},
		{"created_at → read_only", `{"created_at":"2020-01-01T00:00:00Z"}`, "created_at", "read_only"},
		{"location_id → read_only", `{"location_id":99999}`, "location_id", "read_only"},
		{"location_external_key → read_only", `{"location_external_key":"WHS-OTHER"}`, "location_external_key", "read_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id),
				bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
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
			assert.Equal(t, tc.field, resp.Error.Fields[0].Field)
			assert.Equal(t, tc.wantCode, resp.Error.Fields[0].Code)
		})
	}
}

// TRA-686 / BB29 F8 / TRA-780 F4: `external_key` in a PATCH body is rejected
// with 400 invalid_context pointing at the rename endpoint (POST
// /assets/{id}/rename). Silent-drop would let an integrator believe a rename
// PATCH succeeded while the natural key — the join key downstream systems
// rely on — stayed unchanged. Pre-TRA-780 F4 the code was read_only.
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
			assert.Equal(t, "invalid_context", resp.Error.Fields[0].Code)
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

// TRA-692 §1.2: POST with no body (and thus no `name`) must report the
// missing length-bearing field as code=required (no min_length param),
// not too_short. The presence overlay promotes the validator's collapsed
// too_short back to `required` when the JSON key was absent from the
// body — the absent case is presence-class, not length-class. Empty
// strings on the same field stay as too_short; that case is covered
// separately.
//
// Supersedes TRA-675's "always too_short" framing — the docs (errors.md)
// were updated under TRA-692 to match the presence-aware split.
func TestPostAsset_MissingNameEmitsRequired(t *testing.T) {
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
	assert.Equal(t, "required", resp.Error.Fields[0].Code,
		"omitted length-bearing required field must be `required`, not `too_short` (TRA-692 §1.2)")
	assert.Nil(t, resp.Error.Fields[0].Params,
		"promoted `required` carries no params — min_length applies only when the value was present and short")

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

// TRA-775 (BB61-3 F1) / TRA-780 F4: `tags` PATCH echo is now compared as a
// set on full tag content. A submitted array with the same tag content as
// the current state matches regardless of order — generated clients that
// deserialize tags into unordered collections (Python set, Go map, ORMs
// with hash-ordered associations) no longer trip on a verbatim GET → PATCH
// round-trip. Differing set membership or differing field values on a
// matching id returns 400 invalid_context (was read_only pre-TRA-780 F4).
func TestPatchAsset_TagsSetEqualityEcho(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "ASSET-TAGS-SETEQ", "TagsSetEq")

	// Seed three tags with mixed tag_type to exercise full-content checks.
	type seedTag struct {
		tagType string
		value   string
	}
	seeded := []seedTag{
		{"rfid", "V-ALPHA"},
		{"rfid", "V-BRAVO"},
		{"ble", "V-CHARLIE"},
	}
	ids := make([]int, 0, len(seeded))
	for _, s := range seeded {
		var tagID int
		require.NoError(t, pool.QueryRow(context.Background(), `
			INSERT INTO trakrf.tags (org_id, type, value, asset_id, is_active)
			VALUES ($1, $2, $3, $4, true) RETURNING id
		`, orgID, s.tagType, s.value, id).Scan(&tagID))
		ids = append(ids, tagID)
	}

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	// GET the current tags wire shape so we have an authoritative starting
	// point — the integration matters: server-emitted IDs, server-emitted
	// tag_type ordering, etc.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	getReq = withRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, "GET asset failed: %s", getRec.Body.String())

	var getResp struct {
		Data struct {
			Tags []map[string]any `json:"tags"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	require.Len(t, getResp.Data.Tags, 3, "asset must report all three seeded tags")

	current := getResp.Data.Tags
	currentJSON, err := json.Marshal(current)
	require.NoError(t, err)

	reversed := []map[string]any{current[2], current[1], current[0]}
	reversedJSON, err := json.Marshal(reversed)
	require.NoError(t, err)

	rotated := []map[string]any{current[1], current[2], current[0]}
	rotatedJSON, err := json.Marshal(rotated)
	require.NoError(t, err)

	// Length 4: extend with an extra tag object (any plausible id beyond
	// the seeded ones).
	extraTag := map[string]any{"id": ids[2] + 1000, "tag_type": "rfid", "value": "V-EXTRA"}
	tooMany := append([]map[string]any{}, current...)
	tooMany = append(tooMany, extraTag)
	tooManyJSON, err := json.Marshal(tooMany)
	require.NoError(t, err)

	// Same length, swap one id for a non-existent one.
	swappedID := []map[string]any{
		current[0],
		current[1],
		{"id": ids[2] + 999, "tag_type": current[2]["tag_type"], "value": current[2]["value"]},
	}
	swappedIDJSON, err := json.Marshal(swappedID)
	require.NoError(t, err)

	// Same length, same ids, but tweak one tag_type.
	wrongTagType := []map[string]any{
		current[0],
		{"id": current[1]["id"], "tag_type": "barcode", "value": current[1]["value"]},
		current[2],
	}
	wrongTagTypeJSON, err := json.Marshal(wrongTagType)
	require.NoError(t, err)

	// Same length, same ids, but tweak one value.
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
		patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq = withRoundTripOrgContext(patchReq, orgID)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, patchReq)
		return rec
	}

	requireTagsRejection := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 invalid_context, got %d: %s", rec.Code, rec.Body.String())
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
		assert.Equal(t, "invalid_context", resp.Error.Fields[0].Code)
		assert.Contains(t, resp.Error.Fields[0].Message, "POST /api/v1/assets/{asset_id}/tags",
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
	t.Run("length mismatch (4 vs 3) 400 invalid_context", func(t *testing.T) {
		requireTagsRejection(t, patchTags(t, tooManyJSON))
	})
	t.Run("same length different id set 400 invalid_context", func(t *testing.T) {
		requireTagsRejection(t, patchTags(t, swappedIDJSON))
	})
	t.Run("same ids wrong tag_type 400 invalid_context", func(t *testing.T) {
		requireTagsRejection(t, patchTags(t, wrongTagTypeJSON))
	})
	t.Run("same ids wrong value 400 invalid_context", func(t *testing.T) {
		requireTagsRejection(t, patchTags(t, wrongValueJSON))
	})

	// After all of the above (200 echoes + 400 rejections), the persisted
	// tag set must remain unchanged in ids and content.
	rows, err := pool.Query(context.Background(), `
		SELECT id, type, value FROM trakrf.tags
		WHERE asset_id = $1 AND deleted_at IS NULL
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
