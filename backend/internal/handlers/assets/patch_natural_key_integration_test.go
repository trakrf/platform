//go:build integration
// +build integration

// TRA-699 (BB31 §2): natural-key reference fields on PATCH follow the
// accept-if-matches, reject-if-differs rule. Five fields across two
// endpoints share the policy; this file covers PATCH /assets:
//
//   - external_key            (own natural key, managed-via-rename)
//   - location_external_key   (derived from scan events / current_location_id)
//   - location_id             (derived from scan events / current_location_id)
//
// Behavior:
//   - if the body value matches the current resource value, the field is
//     stripped from the update and the request succeeds (200);
//   - if the body value differs, the request fails with 400
//     code=read_only and detail naming the proper write path.
//
// Pre-TRA-699 behavior these tests supersede:
//   - external_key was always 400 read_only (pre-decode reject under TRA-686).
//   - location_external_key was silently stripped on any value (TRA-681).
//   - location_id was a writable field with null-clear semantics (TRA-614).
//     The writable / clear flow is gone; integrators that need to mutate
//     asset location now do so via scan events (record-of-origin posture;
//     see TRA-411 for the medium-term continuous-aggregate refactor).

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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// seedNaturalKeyAsset seeds an asset, optionally with a current_location_id,
// and returns (assetID, locationID-or-zero, locationExternalKey-or-empty).
func seedNaturalKeyAsset(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name, locExtKey string) (int, int, string) {
	t.Helper()
	var locID *int
	if locExtKey != "" {
		var id int
		err := pool.QueryRow(context.Background(), `
			INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
			VALUES ($1, $2, $2, '', $3, true) RETURNING id
		`, orgID, locExtKey, time.Now().UTC()).Scan(&id)
		require.NoError(t, err)
		locID = &id
	}
	var assetID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets
		  (org_id, external_key, name, description, current_location_id, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, $5, true) RETURNING id
	`, orgID, extKey, name, locID, time.Now().UTC()).Scan(&assetID)
	require.NoError(t, err)
	var lid int
	if locID != nil {
		lid = *locID
	}
	return assetID, lid, locExtKey
}

type patchErrorResp struct {
	Error struct {
		Type   string `json:"type"`
		Detail string `json:"detail"`
		Fields []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"fields"`
	} `json:"error"`
}

func patch(t *testing.T, router http.Handler, orgID, assetID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", assetID), bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TRA-699 §1.A: external_key echoed back → 200, value stripped from update.
func TestPatchAsset_NaturalKey_ExternalKey_Matches200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-EK-MATCH", "EkMatch", "")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, `{"external_key":"ASSET-EK-MATCH","name":"renamed via patch"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "renamed via patch", resp.Data["name"], "name must apply")
	assert.Equal(t, "ASSET-EK-MATCH", resp.Data["external_key"], "external_key unchanged (stripped from update)")
}

// TRA-699 §1.B: external_key differing → 400 read_only naming /rename.
func TestPatchAsset_NaturalKey_ExternalKey_Differs400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-EK-DIFF", "EkDiff", "")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, `{"external_key":"ASSET-NEW-NAME"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp patchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message, "/rename",
		"message must name the rename endpoint")

	// Storage unchanged
	var ek string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT external_key FROM trakrf.assets WHERE id = $1`, id).Scan(&ek))
	assert.Equal(t, "ASSET-EK-DIFF", ek)
}

// TRA-699 §1.C: location_external_key echoed back (non-null) → 200.
func TestPatchAsset_NaturalKey_LocationExternalKey_Matches200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, lek := seedNaturalKeyAsset(t, pool, orgID, "ASSET-LEK-MATCH", "LekMatch", "WHS-MATCH")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, fmt.Sprintf(`{"location_external_key":%q,"name":"name2"}`, lek))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "name2", resp.Data["name"])
	assert.Equal(t, lek, resp.Data["location_external_key"])
}

// TRA-699 §1.D: location_external_key echoed back as null when current is
// null → 200.
func TestPatchAsset_NaturalKey_LocationExternalKey_NullNullMatches200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-LEK-NN", "LekNullNull", "")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, `{"location_external_key":null,"name":"n2"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TRA-699 §1.E: location_external_key differing → 400 read_only.
func TestPatchAsset_NaturalKey_LocationExternalKey_Differs400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-LEK-DIFF", "LekDiff", "WHS-CUR")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, `{"location_external_key":"WHS-OTHER"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp patchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message, "scan event",
		"detail must mention the scan-event mutation path (record-of-origin posture)")
}

// TRA-699 §1.F: location_id echoed back → 200.
func TestPatchAsset_NaturalKey_LocationID_Matches200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, locID, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-LID-MATCH", "LidMatch", "WHS-LID-OK")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, fmt.Sprintf(`{"location_id":%d,"name":"n3"}`, locID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TRA-699 §1.G: location_id differing (non-null → other non-null) → 400.
func TestPatchAsset_NaturalKey_LocationID_Differs400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, locID, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-LID-DIFF", "LidDiff", "WHS-A")
	router := setupRoundTripRouter(NewHandler(store))

	// Seed a second location and try to point at it via PATCH.
	var otherLoc int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, 'WHS-B', 'whs-b', '', $2, true) RETURNING id
	`, orgID, time.Now().UTC()).Scan(&otherLoc))
	require.NotEqual(t, locID, otherLoc)

	rec := patch(t, router, orgID, id, fmt.Sprintf(`{"location_id":%d}`, otherLoc))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp patchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message, "scan event",
		"detail must mention the scan-event mutation path")

	// Storage location unchanged.
	var curLoc *int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT current_location_id FROM trakrf.assets WHERE id = $1`, id).Scan(&curLoc))
	require.NotNil(t, curLoc)
	assert.Equal(t, locID, *curLoc)
}

// TRA-699 §1.H: location_id null vs non-null current is a "differs" case (a
// clear request that PATCH no longer honors).
func TestPatchAsset_NaturalKey_LocationID_NullVsNonNullDiffers400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, locID, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-LID-CLEAR", "LidClear", "WHS-X")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, `{"location_id":null}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp patchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)

	// Storage location unchanged.
	var curLoc *int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT current_location_id FROM trakrf.assets WHERE id = $1`, id).Scan(&curLoc))
	require.NotNil(t, curLoc)
	assert.Equal(t, locID, *curLoc)
}

// TRA-702 / BB32 D2: a single differing read_only field must echo
// fields[0].message verbatim in detail — pre-TRA-702 the inline emit-site
// wrote the literal "validation failed", burying the redirect-to-/rename
// message in fields[0].
func TestPatchAsset_NaturalKey_ReadOnly_DetailEchoesFieldMessage(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-D2-ECHO", "D2Echo", "")
	router := setupRoundTripRouter(NewHandler(store))

	rec := patch(t, router, orgID, id, `{"external_key":"ASSET-DIFFERENT"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp patchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, resp.Error.Fields[0].Message, resp.Error.Detail,
		"detail must echo fields[0].message verbatim (BB32 D2)")
	assert.Contains(t, resp.Error.Detail, "/rename",
		"detail must name the rename endpoint so AI integrators can self-redirect")
}

// TRA-702 / BB32 D3: a PATCH body with multiple differing read_only fields
// must surface one entry per field in fields[] AND a "(and N more
// validation errors)" suffix in detail — not just the first violation.
func TestPatchAsset_NaturalKey_ReadOnly_MultiField_AllReportedWithSuffix(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-D3-MULTI", "D3Multi", "WHS-OWNED")
	router := setupRoundTripRouter(NewHandler(store))

	// All three natural-key fields differ from the current resource state.
	rec := patch(t, router, orgID, id,
		`{"external_key":"ASSET-OTHER","location_external_key":"WHS-OTHER","location_id":99999}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp patchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 3, "all three differing read_only fields must surface")

	fields := map[string]string{}
	for _, f := range resp.Error.Fields {
		fields[f.Field] = f.Code
	}
	assert.Equal(t, "read_only", fields["external_key"])
	assert.Equal(t, "read_only", fields["location_external_key"])
	assert.Equal(t, "read_only", fields["location_id"])

	assert.Contains(t, resp.Error.Detail, "(and 2 more validation errors)",
		"detail must include the plural multi-field suffix")
	assert.Equal(t, resp.Error.Fields[0].Message+" (and 2 more validation errors)",
		resp.Error.Detail,
		"detail must echo fields[0].message + suffix verbatim")
}

// TRA-702 / BB32 D3: a PATCH with multiple explicit-null violations on
// non-nullable fields must report every offending field, not just the first.
// Pre-TRA-702 the loop short-circuited on the first match.
func TestPatchAsset_ExplicitNullOnNonNullable_MultiField_AllReported(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-NULL-MULTI", "NullMulti", "")
	router := setupRoundTripRouter(NewHandler(store))

	// Two non-nullable PATCH fields set to null in the same body.
	rec := patch(t, router, orgID, id, `{"valid_from":null,"name":null,"is_active":null}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp patchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 3, "every null-on-non-nullable violation must surface")
	for _, f := range resp.Error.Fields {
		assert.Equal(t, "invalid_value", f.Code, "code stays invalid_value for null-on-non-nullable")
	}
	assert.Contains(t, resp.Error.Detail, "(and 2 more validation errors)",
		"detail must carry the multi-field suffix")
}

// TRA-699 §1.I: full GET → PATCH back round-trip with all three natural-key
// fields populated must 200 (each field is a verbatim echo). This is the
// integrator-facing contract the rule unlocks.
func TestPatchAsset_NaturalKey_FullGETRoundTrip_200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedNaturalKeyAsset(t, pool, orgID, "ASSET-RT-NK", "RtNk", "WHS-RT")
	router := setupRoundTripRouter(NewHandler(store))

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	getReq = withRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var getResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	getResp.Data["name"] = "renamed via round-trip"
	// TRA-710 brought `tags` under the same echo-or-reject rule, so it can
	// stay on the body — the verbatim GET echo of `[]` (or any current
	// shape) is normalized out.

	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	rec := patch(t, router, orgID, id, string(body))
	require.Equal(t, http.StatusOK, rec.Code,
		"full GET → PATCH round-trip with natural keys included must 200: %s", rec.Body.String())

	var putResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &putResp))
	assert.Equal(t, "renamed via round-trip", putResp.Data["name"])
}
