//go:build integration

// TRA-1025 / ADR 0002: end-to-end enforcement of the per-org capability gate
// through the REAL production router, so route attachment, middleware ordering,
// and the SQL read path are all exercised together. The middleware's own
// behaviour is unit-tested in internal/middleware; what can only be checked
// here is that the gate is actually attached where it should be and absent
// where it should not.

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

// grantCap gives orgID the named capability.
func grantCap(t *testing.T, db *testutil.TestDB, orgID int, cap string) {
	t.Helper()
	_, err := db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.org_capabilities (org_id, capability) VALUES ($1, $2)`, orgID, cap)
	require.NoError(t, err)
}

// newOrg creates a bare org (CreateTestAccount hardcodes one identifier, so
// multi-org fixtures seed directly).
func newOrg(t *testing.T, db *testutil.TestDB, name, identifier string) int {
	t.Helper()
	var id int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ($1, $2, true) RETURNING id`, name, identifier).Scan(&id))
	return id
}

// requireCapabilityRequired asserts the uniform denial contract: 403, top-level
// type capability_required, matching title. Clients branch on type/title only.
func requireCapabilityRequired(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	var env struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), "body: %s", rec.Body.String())
	require.Equal(t, "capability_required", env.Error.Type, "body: %s", rec.Body.String())
	require.Equal(t, "Capability required", env.Error.Title)
	require.Equal(t, http.StatusForbidden, env.Error.Status)
}

// requireNotCapabilityGated asserts the response is anything BUT the capability
// denial. It deliberately checks error.type rather than the status code: these
// routes legitimately answer 403 forbidden (the fixture session user is not a
// member of the org), 404, or 415, and only capability_required would mean the
// gate is attached where it should not be.
func requireNotCapabilityGated(t *testing.T, rec *httptest.ResponseRecorder, path string) {
	t.Helper()
	var env struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	// A success body has no error object; Unmarshal leaves Type empty.
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	require.NotEqual(t, "capability_required", env.Error.Type,
		"%s must not be capability-gated; body: %s", path, rec.Body.String())
}

// TestCapabilityGate_Enforcement covers the acceptance criteria: an ungranted
// org is denied on every gated surface with a type distinguishable from
// forbidden and payment_required; a granted org is unaffected; the ungated base
// is never touched.
func TestCapabilityGate_Enforcement(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")

	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)

	// Zero grants is the default for every org (ADR 0002: no backfill, no
	// signup default), so the ungranted fixture needs no setup at all.
	ungrantedOrg := testutil.CreateTestAccount(t, db.AdminPool)
	grantedOrg := newOrg(t, db, "Granted Co", "granted-co")
	grantCap(t, db, grantedOrg, "mustering")
	grantCap(t, db, grantedOrg, "geofence")

	do := func(orgID int, method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+sessionToken(t, orgID))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	t.Run("ungranted org is denied on mustering, reads included", func(t *testing.T) {
		// Mustering is granted to nobody at all after this release, so every
		// mustering route denies for every org until someone grants it.
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodGet, "/api/v1/mustering/status", nil))
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodGet, "/api/v1/mustering/events", nil))
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodGet, "/api/v1/mustering/floor-plan", nil))
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodPost, "/api/v1/mustering/events",
			map[string]any{"name": "Drill"}))
		// Bodyless POST still needs a Content-Type: the group-level ContentType
		// middleware is a transport concern that runs ahead of every per-route
		// gate, so an empty object is what reaches the capability check.
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodPost, "/api/v1/mustering/seed",
			map[string]any{}))
	})

	t.Run("ungranted org is denied on the geofence surface", func(t *testing.T) {
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodGet, "/api/v1/output-devices", nil))
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodPost, "/api/v1/output-devices",
			map[string]any{"name": "Buzzer", "transport": "http", "base_url": "http://127.0.0.1:1/relay"}))
		requireCapabilityRequired(t, do(ungrantedOrg, http.MethodGet,
			"/api/v1/orgs/"+strconv.Itoa(ungrantedOrg)+"/geofence-defaults", nil))
	})

	t.Run("granted org behaves as before", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/mustering/status",
			"/api/v1/mustering/events",
			"/api/v1/output-devices",
		} {
			requireNotCapabilityGated(t, do(grantedOrg, http.MethodGet, path, nil), path)
		}
	})

	t.Run("ungated base surface is untouched", func(t *testing.T) {
		// Asset management is the always-on base an org gets with zero grants.
		// Auth, org/user management, reports, and webhooks likewise.
		for _, path := range []string{
			"/api/v1/assets",
			"/api/v1/locations",
			"/api/v1/scan-devices",
			"/api/v1/webhooks",
			"/api/v1/reports/asset-locations",
			"/api/v1/users/me",
		} {
			requireNotCapabilityGated(t, do(ungrantedOrg, http.MethodGet, path, nil), path)
		}
	})
}

// TestCapabilityGate_PrecedesSubscriptionGate pins the ADR 0002 enforcement
// order on a route that carries both gates: an org cannot be past-due on a
// surface it never bought, so capability_required wins over payment_required.
func TestCapabilityGate_PrecedesSubscriptionGate(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")

	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)

	lapsedNoGrant := newOrg(t, db, "Lapsed Ungranted", "lapsed-ungranted")
	lapsedGranted := newOrg(t, db, "Lapsed Granted", "lapsed-granted")
	grantCap(t, db, lapsedGranted, "geofence")

	_, err := db.AdminPool.Exec(context.Background(),
		`UPDATE trakrf.organizations SET subscription_enabled = false WHERE id = ANY($1)`,
		[]int{lapsedNoGrant, lapsedGranted})
	require.NoError(t, err)

	post := func(orgID int) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
			"name": "Buzzer", "transport": "http", "base_url": "http://127.0.0.1:1/relay",
		}))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/output-devices", &buf)
		req.Header.Set("Authorization", "Bearer "+sessionToken(t, orgID))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// Lapsed AND ungranted: capability_required, not 402.
	requireCapabilityRequired(t, post(lapsedNoGrant))

	// Lapsed but granted: 402 as before TRA-1025.
	rec := post(lapsedGranted)
	require.Equal(t, http.StatusPaymentRequired, rec.Code, "body: %s", rec.Body.String())
	var env struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, "payment_required", env.Error.Type)
}

// TestUsersMe_ExposesCapabilities covers the payload half of the ticket: the
// frontend gates nav and routes on this array, so it must be present, sorted,
// and [] rather than null for the zero-grant default.
func TestUsersMe_ExposesCapabilities(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")

	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)

	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	var userID int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ('caps-me@example.com', 'Caps Me', 'stub') RETURNING id`).Scan(&userID))
	_, err := db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`, orgID, userID)
	require.NoError(t, err)

	me := func() (raw []byte, caps []string) {
		tok, gerr := jwt.Generate(userID, "caps-me@example.com", &orgID)
		require.NoError(t, gerr)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		var body struct {
			Data struct {
				CurrentOrg struct {
					Capabilities []string `json:"capabilities"`
				} `json:"current_org"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body: %s", rec.Body.String())
		return rec.Body.Bytes(), body.Data.CurrentOrg.Capabilities
	}

	raw, caps := me()
	require.NotNil(t, caps, "capabilities must serialize as [] for a zero-grant org, never null")
	require.Empty(t, caps)
	require.Contains(t, string(raw), `"capabilities":[]`)

	grantCap(t, db, orgID, "mustering")
	grantCap(t, db, orgID, "geofence")

	// Revocation and granting take effect on the next request — grants are read
	// per-request, never baked into the token (the JWT above is unchanged).
	_, caps = me()
	require.Equal(t, []string{"geofence", "mustering"}, caps)
}
