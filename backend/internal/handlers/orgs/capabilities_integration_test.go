//go:build integration
// +build integration

// TRA-1027: superadmin capability grant management, HTTP surface. Extends the
// TRA-949 superadmin org surface exercised by admin_integration_test.go, whose
// seedSessionUser / newAdminOrgRouter helpers these tests reuse.

package orgs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func seedOrg(t *testing.T, pool *pgxpool.Pool, name, identifier string) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ($1, $2, true) RETURNING id`, name, identifier).Scan(&id))
	return id
}

func getCapabilities(t *testing.T, store *storage.Storage, token string, orgID int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%d/capabilities", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newAdminOrgRouter(t, store).ServeHTTP(w, req)
	return w
}

func putCapabilities(t *testing.T, store *storage.Storage, token string, orgID int, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/orgs/%d/capabilities", orgID), bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newAdminOrgRouter(t, store).ServeHTTP(w, req)
	return w
}

// capabilityView decodes {"data": {"capabilities": [...], "available": [...]}}.
func capabilityView(t *testing.T, w *httptest.ResponseRecorder) (granted, available []string) {
	t.Helper()
	var body struct {
		Data struct {
			Capabilities []string `json:"capabilities"`
			Available    []string `json:"available"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Data.Capabilities, body.Data.Available
}

func TestGetOrgCapabilities_SuperadminSeesGrantsOfNonMemberOrg(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Caps Org", "caps-org")
	_, err := pool.Exec(context.Background(),
		`INSERT INTO trakrf.org_capabilities (org_id, capability) VALUES ($1, 'geofence')`, orgID)
	require.NoError(t, err)
	token := seedSessionUser(t, pool, "super@x", true)

	w := getCapabilities(t, store, token, orgID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	granted, available := capabilityView(t, w)
	assert.Equal(t, []string{"geofence"}, granted)
	// The vocabulary ships with the response so the grant UI renders checkboxes
	// from server truth rather than a hand-maintained frontend copy.
	assert.Equal(t, []string{"geofence", "inventory", "mustering"}, available)
}

// Zero grants is the norm, and it must arrive as [] — a null would render as
// "unknown" in the UI where the truth is "none granted".
func TestGetOrgCapabilities_UngrantedOrgReturnsEmptyArray(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Bare Org", "bare-org")
	token := seedSessionUser(t, pool, "super@x", true)

	w := getCapabilities(t, store, token, orgID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"capabilities":[]`)
}

func TestGetOrgCapabilities_NonSuperadmin403(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Theirs", "theirs-caps-org")
	token := seedSessionUser(t, pool, "regular@x", false)

	w := getCapabilities(t, store, token, orgID)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestGetOrgCapabilities_UnknownOrg404(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	token := seedSessionUser(t, pool, "super@x", true)

	w := getCapabilities(t, store, token, 999999999)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// The acceptance case: granting through this surface must be equivalent to the
// hand-written INSERT used at release time, not merely similar.
func TestSetOrgCapabilities_SuperadminGrantsAndPersists(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Grantee", "grantee-org")
	token := seedSessionUser(t, pool, "super@x", true)

	w := putCapabilities(t, store, token, orgID, `{"capabilities": ["geofence"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	granted, _ := capabilityView(t, w)
	assert.Equal(t, []string{"geofence"}, granted)

	// Same result the middleware reads on the org's next request.
	caps, err := store.OrgCapabilitySet(context.Background(), orgID)
	require.NoError(t, err)
	assert.Equal(t, []string{"geofence"}, caps)
}

func TestSetOrgCapabilities_RevokesOmittedNames(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Revokee", "revokee-org")
	token := seedSessionUser(t, pool, "super@x", true)

	require.Equal(t, http.StatusOK,
		putCapabilities(t, store, token, orgID, `{"capabilities": ["geofence", "mustering"]}`).Code)

	w := putCapabilities(t, store, token, orgID, `{"capabilities": []}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	granted, _ := capabilityView(t, w)
	assert.Empty(t, granted)

	caps, err := store.OrgCapabilitySet(context.Background(), orgID)
	require.NoError(t, err)
	assert.Empty(t, caps, "revocation must be visible to the next request immediately")
}

// A body with no capabilities key is rejected rather than read as "revoke
// everything" — same reasoning as the entitlement PATCH requiring its enabled
// flag: a malformed superadmin write must not silently strip an org's grants.
func TestSetOrgCapabilities_MissingField400(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Sloppy", "sloppy-org")
	token := seedSessionUser(t, pool, "super@x", true)

	w := putCapabilities(t, store, token, orgID, `{}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// The FK would reject this anyway; the registry check turns a 500-shaped
// database error into a 400 that names the bad value.
func TestSetOrgCapabilities_UnknownCapability400(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Typo", "typo-org")
	token := seedSessionUser(t, pool, "super@x", true)

	w := putCapabilities(t, store, token, orgID, `{"capabilities": ["wip_tracking"]}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "wip_tracking")
}

func TestSetOrgCapabilities_NonSuperadmin403(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := seedOrg(t, pool, "Not Yours", "not-yours-org")
	token := seedSessionUser(t, pool, "admin@x", false)

	w := putCapabilities(t, store, token, orgID, `{"capabilities": ["geofence"]}`)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	caps, err := store.OrgCapabilitySet(context.Background(), orgID)
	require.NoError(t, err)
	assert.Empty(t, caps, "a rejected caller must not have written a grant")
}

func TestSetOrgCapabilities_UnknownOrg404(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	token := seedSessionUser(t, pool, "super@x", true)

	w := putCapabilities(t, store, token, 999999999, `{"capabilities": ["geofence"]}`)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// The all-orgs list is where an operator scans grant state across every org, so
// it carries the set rather than making them open each org in turn.
func TestListAllOrgs_IncludesCapabilities(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-org-caps")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	grantedOrg := seedOrg(t, pool, "Granted Org", "granted-list-org")
	bareOrg := seedOrg(t, pool, "Bare List Org", "bare-list-org")
	_, err := pool.Exec(context.Background(),
		`INSERT INTO trakrf.org_capabilities (org_id, capability)
		 VALUES ($1, 'geofence'), ($1, 'mustering')`, grantedOrg)
	require.NoError(t, err)

	// Two members, so a capability join that fans out rows would inflate the
	// member count — the bug this assertion exists to catch.
	for _, email := range []string{"m1@x", "m2@x"} {
		var userID int
		require.NoError(t, pool.QueryRow(context.Background(),
			`INSERT INTO trakrf.users (name, email, password_hash) VALUES ($1, $1, 'stub') RETURNING id`,
			email).Scan(&userID))
		_, err := pool.Exec(context.Background(),
			`INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`,
			grantedOrg, userID)
		require.NoError(t, err)
	}

	token := seedSessionUser(t, pool, "super@x", true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newAdminOrgRouter(t, store).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Data []struct {
			ID           int      `json:"id"`
			Capabilities []string `json:"capabilities"`
			MemberCount  int      `json:"member_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	seen := map[int]struct {
		caps    []string
		members int
	}{}
	for _, o := range body.Data {
		seen[o.ID] = struct {
			caps    []string
			members int
		}{o.Capabilities, o.MemberCount}
	}

	require.Contains(t, seen, grantedOrg)
	assert.Equal(t, []string{"geofence", "mustering"}, seen[grantedOrg].caps)
	assert.Equal(t, 2, seen[grantedOrg].members, "capabilities must not fan out the member count")

	require.Contains(t, seen, bareOrg)
	assert.Empty(t, seen[bareOrg].caps)
	assert.NotNil(t, seen[bareOrg].caps, "an ungranted org must serialize [], not null")
}
