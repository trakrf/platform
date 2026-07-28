//go:build integration

// TRA-1027 / ADR 0002: the superadmin grant surface end-to-end through the REAL
// production router, which is the only place the ticket's acceptance criteria
// can actually be checked — that a grant written through the UI is the same
// grant the enforcement middleware reads, and that a revocation lands on the
// org's very next request with no restart and no token reissue.
//
// The enforcement gate itself is covered by router_capability_integration_test.go;
// what is new here is the write path in front of it.

package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// seedUser inserts a user and returns its id.
func seedUser(t *testing.T, db *testutil.TestDB, email string, superadmin bool) int {
	t.Helper()
	var id int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (email, name, password_hash, is_superadmin)
		VALUES ($1, $1, 'stub', $2) RETURNING id`, email, superadmin).Scan(&id))
	return id
}

// TestCapabilityGrants_GrantAndRevokeThroughSuperadminSurface is the ticket's
// acceptance path: grant → the gated surface opens and /users/me lists the
// capability; revoke → capability_required returns and /users/me drops it. Every
// request reuses the same JWT throughout, which is what "no token reissue" means
// in practice.
func TestCapabilityGrants_GrantAndRevokeThroughSuperadminSurface(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")

	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)

	// The target org and a member of it — the principal whose experience the
	// acceptance criteria describe. It starts at zero grants, the default.
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	memberID := seedUser(t, db, "grantee-member@example.com", false)
	_, err := db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`,
		orgID, memberID)
	require.NoError(t, err)
	memberTok, err := jwt.Generate(memberID, "grantee-member@example.com", &orgID)
	require.NoError(t, err)

	// The operator: a superadmin who is NOT a member of the target org, which is
	// the real shape of a release-day grant.
	superID := seedUser(t, db, "operator@example.com", true)
	otherOrg := newOrg(t, db, "Operator Co", "operator-co")
	superTok, err := jwt.Generate(superID, "operator@example.com", &otherOrg)
	require.NoError(t, err)

	setCaps := func(tok string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut,
			"/api/v1/orgs/"+strconv.Itoa(orgID)+"/capabilities", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	asMember := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+memberTok)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	meCapabilities := func() []string {
		rec := asMember(http.MethodGet, "/api/v1/users/me")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var body struct {
			Data struct {
				CurrentOrg struct {
					Capabilities []string `json:"capabilities"`
				} `json:"current_org"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body: %s", rec.Body.String())
		return body.Data.CurrentOrg.Capabilities
	}

	// Before: zero grants, so the geofence surface is closed.
	requireCapabilityRequired(t, asMember(http.MethodGet, "/api/v1/output-devices"))
	require.Empty(t, meCapabilities())

	// Grant through the superadmin surface — the equivalence the ticket asks
	// for: this replaces the hand-written INSERT, it does not supplement it.
	rec := setCaps(superTok, `{"capabilities": ["geofence"]}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	requireNotCapabilityGated(t, asMember(http.MethodGet, "/api/v1/output-devices"),
		"/api/v1/output-devices")
	require.Equal(t, []string{"geofence"}, meCapabilities())

	// Revoke. Same JWT, no restart: the next request must already be denied.
	rec = setCaps(superTok, `{"capabilities": []}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	requireCapabilityRequired(t, asMember(http.MethodGet, "/api/v1/output-devices"))
	require.Empty(t, meCapabilities())
}

// A member — even an org admin — must not be able to grant their own org a
// capability. This is the whole security premise of the surface, so it is
// checked through the production router rather than only at the handler.
func TestCapabilityGrants_OrgAdminCannotGrantThemselves(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")

	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)

	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	adminID := seedUser(t, db, "self-grant@example.com", false)
	_, err := db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`,
		orgID, adminID)
	require.NoError(t, err)
	tok, err := jwt.Generate(adminID, "self-grant@example.com", &orgID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/orgs/"+strconv.Itoa(orgID)+"/capabilities",
		bytes.NewBufferString(`{"capabilities": ["geofence"]}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())

	caps, err := db.Store.OrgCapabilitySet(context.Background(), orgID)
	require.NoError(t, err)
	require.Empty(t, caps)

	// And the gate is still closed, which is the consequence that matters.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/output-devices", nil)
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	requireCapabilityRequired(t, getRec)
}

// The grant surface must never be capability-gated itself: a chicken-and-egg
// gate would make the zero-grant default unrecoverable through the UI.
func TestCapabilityGrants_SurfaceIsNotItselfCapabilityGated(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")

	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)

	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	superID := seedUser(t, db, "bootstrap@example.com", true)
	tok, err := jwt.Generate(superID, "bootstrap@example.com", &orgID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/orgs/"+strconv.Itoa(orgID)+"/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}
