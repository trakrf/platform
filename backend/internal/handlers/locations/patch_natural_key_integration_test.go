//go:build integration
// +build integration

// TRA-699 (BB31 §2): natural-key reference fields on PATCH follow the
// accept-if-matches, reject-if-differs rule. This file covers PATCH
// /locations:
//
//   - external_key         (own natural key, managed-via-rename)
//   - parent_external_key  (parent reference, managed-via-rename on parent)
//
// parent_id is NOT in scope — it remains writable (PATCH may re-parent via
// surrogate). Only the natural-key form locks down to accept-if-matches.
//
// Pre-TRA-699: both fields were pre-decode 400 rejected under TRA-686.

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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

func seedLocationWithOptionalParent(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name, parentExtKey string) (int, int, string) {
	t.Helper()
	var parentID *int
	if parentExtKey != "" {
		var id int
		err := pool.QueryRow(context.Background(), `
			INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
			VALUES ($1, $2, $2, '', $3, true) RETURNING id
		`, orgID, parentExtKey, time.Now().UTC()).Scan(&id)
		require.NoError(t, err)
		parentID = &id
	}
	var locID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations
		  (org_id, external_key, name, description, parent_location_id, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, $5, true) RETURNING id
	`, orgID, extKey, name, parentID, time.Now().UTC()).Scan(&locID)
	require.NoError(t, err)
	var pid int
	if parentID != nil {
		pid = *parentID
	}
	return locID, pid, parentExtKey
}

type locPatchErrorResp struct {
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

func patchLoc(t *testing.T, router http.Handler, orgID, locID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", locID), bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = withLocationRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TRA-699 §2.A: external_key echoed back → 200.
func TestPatchLocation_NaturalKey_ExternalKey_Matches200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-EK-MATCH", "EkMatch", "")
	router := setupLocationRoundTripRouter(NewHandler(store))

	rec := patchLoc(t, router, orgID, id, `{"external_key":"LOC-EK-MATCH","name":"renamed via patch"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "renamed via patch", resp.Data["name"])
	assert.Equal(t, "LOC-EK-MATCH", resp.Data["external_key"])
}

// TRA-699 §2.B: external_key differing → 400 read_only naming /rename.
func TestPatchLocation_NaturalKey_ExternalKey_Differs400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-EK-DIFF", "EkDiff", "")
	router := setupLocationRoundTripRouter(NewHandler(store))

	rec := patchLoc(t, router, orgID, id, `{"external_key":"LOC-NEW-NAME"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp locPatchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message, "/rename")
}

// TRA-699 §2.C: parent_external_key echoed back (non-null) → 200.
func TestPatchLocation_NaturalKey_ParentExternalKey_Matches200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, parentEK := seedLocationWithOptionalParent(t, pool, orgID, "LOC-PEK-MATCH", "PekMatch", "LOC-PARENT-MATCH")
	router := setupLocationRoundTripRouter(NewHandler(store))

	rec := patchLoc(t, router, orgID, id, fmt.Sprintf(`{"parent_external_key":%q,"name":"n2"}`, parentEK))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "n2", resp.Data["name"])
	assert.Equal(t, parentEK, resp.Data["parent_external_key"])
}

// TRA-699 §2.D: parent_external_key echoed back as null when current is null → 200.
func TestPatchLocation_NaturalKey_ParentExternalKey_NullNullMatches200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-PEK-NN", "PekNullNull", "")
	router := setupLocationRoundTripRouter(NewHandler(store))

	rec := patchLoc(t, router, orgID, id, `{"parent_external_key":null,"name":"n2"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TRA-699 §2.E: parent_external_key differing → 400 read_only.
func TestPatchLocation_NaturalKey_ParentExternalKey_Differs400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-PEK-DIFF", "PekDiff", "LOC-PARENT-DIFF")
	router := setupLocationRoundTripRouter(NewHandler(store))

	rec := patchLoc(t, router, orgID, id, `{"parent_external_key":"LOC-OTHER-PARENT"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp locPatchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "parent_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	// TRA-713 / BB33 F3: hint must direct integrators at parent_id (the
	// surrogate that actually re-parents); /rename only renames external_key.
	assert.NotContains(t, resp.Error.Fields[0].Message, "/rename",
		"hint must not point at /rename — that endpoint can't re-parent")
	assert.Contains(t, resp.Error.Fields[0].Message, "parent_id",
		"hint must name parent_id (the surrogate that re-parents)")
}

// TRA-699 §2.F: parent_id remains writable on PATCH (the natural-key form
// is what's locked down, not the surrogate). Regression test against any
// over-reach of the new rule.
func TestPatchLocation_NaturalKey_ParentIDStillWritable(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-PID-MOVE", "PidMove", "LOC-PID-SRC")
	router := setupLocationRoundTripRouter(NewHandler(store))

	var destParent int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, 'LOC-PID-DEST', 'dest', '', $2, true) RETURNING id
	`, orgID, time.Now().UTC()).Scan(&destParent))

	rec := patchLoc(t, router, orgID, id, fmt.Sprintf(`{"parent_id":%d}`, destParent))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var dbParent *int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT parent_location_id FROM trakrf.locations WHERE id = $1`, id).Scan(&dbParent))
	require.NotNil(t, dbParent)
	assert.Equal(t, destParent, *dbParent)
}

// TRA-702 / BB32 D2: a single differing read_only field must echo
// fields[0].message verbatim in detail. Pre-TRA-702 the inline emit-site
// wrote the literal "validation failed", which buried the redirect-to-/rename
// message inside fields[0] where AI integrators were less likely to read it.
func TestPatchLocation_NaturalKey_ReadOnly_DetailEchoesFieldMessage(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-D2-ECHO", "D2Echo", "")
	router := setupLocationRoundTripRouter(NewHandler(store))

	rec := patchLoc(t, router, orgID, id, `{"external_key":"LOC-DIFFERENT"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp locPatchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, resp.Error.Fields[0].Message, resp.Error.Detail,
		"detail must echo fields[0].message verbatim (BB32 D2)")
	assert.Contains(t, resp.Error.Detail, "/rename",
		"detail must name the rename endpoint")
}

// TRA-702 / BB32 D3: a PATCH body with multiple differing read_only fields
// must surface one entry per field in fields[] AND a "(and N more
// validation errors)" suffix in detail.
func TestPatchLocation_NaturalKey_ReadOnly_MultiField_AllReportedWithSuffix(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-D3-MULTI", "D3Multi", "LOC-D3-PARENT")
	router := setupLocationRoundTripRouter(NewHandler(store))

	// Both natural-key fields differ from current resource state.
	rec := patchLoc(t, router, orgID, id,
		`{"external_key":"LOC-OTHER","parent_external_key":"LOC-OTHER-PARENT"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp locPatchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 2, "both differing read_only fields must surface")

	fields := map[string]string{}
	for _, f := range resp.Error.Fields {
		fields[f.Field] = f.Code
	}
	assert.Equal(t, "read_only", fields["external_key"])
	assert.Equal(t, "read_only", fields["parent_external_key"])

	assert.Contains(t, resp.Error.Detail, "(and 1 more validation error)",
		"detail must include the singular multi-field suffix when N=1")
	assert.Equal(t, resp.Error.Fields[0].Message+" (and 1 more validation error)",
		resp.Error.Detail,
		"detail must echo fields[0].message + suffix verbatim")
}

// TRA-702 / BB32 D3: a PATCH with multiple explicit-null violations on
// non-nullable fields must report every offending field, not just the first.
func TestPatchLocation_ExplicitNullOnNonNullable_MultiField_AllReported(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-NULL-MULTI", "NullMulti", "")
	router := setupLocationRoundTripRouter(NewHandler(store))

	rec := patchLoc(t, router, orgID, id, `{"valid_from":null,"name":null,"is_active":null}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var resp locPatchErrorResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 3, "every null-on-non-nullable violation must surface")
	for _, f := range resp.Error.Fields {
		assert.Equal(t, "invalid_value", f.Code)
	}
	assert.Contains(t, resp.Error.Detail, "(and 2 more validation errors)",
		"detail must carry the multi-field suffix")
}

// TRA-699 §2.G: full GET → PATCH round-trip with both natural-key fields
// populated must 200. The integrator-facing contract the rule unlocks.
func TestPatchLocation_NaturalKey_FullGETRoundTrip_200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id, _, _ := seedLocationWithOptionalParent(t, pool, orgID, "LOC-RT-NK", "RtNk", "LOC-RT-PARENT")
	router := setupLocationRoundTripRouter(NewHandler(store))

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	getReq = withLocationRoundTripOrgContext(getReq, orgID)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var getResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	getResp.Data["name"] = "renamed via round-trip"
	// `tags` remains pre-decode rejected (out of scope for TRA-699).
	delete(getResp.Data, "tags")

	body, err := json.Marshal(getResp.Data)
	require.NoError(t, err)

	rec := patchLoc(t, router, orgID, id, string(body))
	require.Equal(t, http.StatusOK, rec.Code,
		"full GET → PATCH round-trip with natural keys included must 200: %s", rec.Body.String())
}
