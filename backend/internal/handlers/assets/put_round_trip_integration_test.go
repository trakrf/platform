//go:build integration
// +build integration

// TRA-608 / BB18 §1.7: GET → PUT round-trip must succeed. The PUT handler
// strips the read-only fields on PublicAssetView (id, created_at,
// updated_at, tags) from the request body before strict-decoding so a
// naive read-mutate-write client doesn't trip over schema asymmetry. Typo'd
// fields not in that drop set still produce a 400 validation_error.
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
	r.Put("/api/v1/assets/{asset_id}", handler.Update)
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

// TRA-608 acceptance: GET /api/v1/assets/{id} → unmodified body →
// PUT /api/v1/assets/{id} succeeds with 200.
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

	// Naive mutate-and-PUT: take the entire GET body verbatim, change name.
	getResp.Data["name"] = "Forklift 7 (renamed)"
	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = withRoundTripOrgContext(putReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

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
	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = withRoundTripOrgContext(putReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

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
	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/assets/%d", assetID), bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = withRoundTripOrgContext(putReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

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

	// Verbatim PUT-back of the GET body — the connector flow from §S2.
	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = withRoundTripOrgContext(putReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

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
	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putReq = withRoundTripOrgContext(putReq, orgID)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

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
