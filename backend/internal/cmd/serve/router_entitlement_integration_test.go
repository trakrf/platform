//go:build integration

package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/alarm"
	"github.com/trakrf/platform/backend/internal/alarm/shelly"
	"github.com/trakrf/platform/backend/internal/buildinfo"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	outputdeviceshandler "github.com/trakrf/platform/backend/internal/handlers/outputdevices"
	readstreamhandler "github.com/trakrf/platform/backend/internal/handlers/readstream"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	scandeviceshandler "github.com/trakrf/platform/backend/internal/handlers/scandevices"
	scanpointshandler "github.com/trakrf/platform/backend/internal/handlers/scanpoints"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	readstreamsvc "github.com/trakrf/platform/backend/internal/services/readstream"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// setupRealRouter builds the full production router (setupRouter) wired against
// a real DB-backed *storage.Storage so the entitlement gate runs its real
// OrgIsEntitled query end-to-end. Mirrors setupTestRouter in serve_test.go but
// swaps the empty stub store for the live one.
func setupRealRouter(t *testing.T, store *storage.Storage) *chi.Mux {
	t.Helper()

	authSvc := authservice.NewService(nil, store, nil)
	orgsSvc := orgsservice.NewService(nil, store, nil)

	authHandler := authhandler.NewHandler(authSvc, store)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc, authSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	inventoryHandler := inventoryhandler.NewHandler(store)
	reportsHandler := reportshandler.NewHandler(store)
	scanDevicesHandler := scandeviceshandler.NewHandler(store, nil)
	scanPointsHandler := scanpointshandler.NewHandler(store)
	outputDevicesHandler := outputdeviceshandler.NewHandler(store, alarm.NewDispatcher(shelly.New(0), nil), 0)
	lookupHandler := lookuphandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(nil, buildinfo.Info{Version: "test"}, time.Now())
	frontendHandler := frontendhandler.NewHandler(fstest.MapFS{}, "frontend/dist", "")
	readstreamHandler := readstreamhandler.NewHandler(readstreamsvc.New())
	testHandler := testhandler.NewHandler(store)

	return setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, inventoryHandler, reportsHandler, scanDevicesHandler, scanPointsHandler, outputDevicesHandler, lookupHandler, healthHandler, frontendHandler, readstreamHandler, testHandler, store)
}

// sessionToken mints a real session JWT (passes middleware.Auth / EitherAuth and
// RequireScope's session pass-through) scoped to orgID.
func sessionToken(t *testing.T, orgID int) string {
	t.Helper()
	tok, err := jwt.Generate(1, "entitlement@test.com", &orgID)
	require.NoError(t, err)
	return tok
}

// TestEntitlementGate_Enforcement is the security-sensitive end-to-end check for
// TRA-947: a lapsed org gets 402 on paid mutations (public + internal) while
// reads and must-stay-open writes pass; an entitled org is never 402'd.
func TestEntitlementGate_Enforcement(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-key")

	db := testutil.SetupTestDBFull(t)
	r := setupRealRouter(t, db.Store)

	// Two orgs: one entitled (default), one lapsed (subscription disabled).
	entitledOrg := testutil.CreateTestAccount(t, db.AdminPool)
	var lapsedOrg int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Lapsed Co', 'lapsed-co', true) RETURNING id`,
	).Scan(&lapsedOrg))
	// Lapse it: disable the subscription. org_is_entitled() must now return false.
	_, err := db.AdminPool.Exec(context.Background(),
		`UPDATE trakrf.organizations SET subscription_enabled = false WHERE id = $1`, lapsedOrg)
	require.NoError(t, err)

	// Sanity: the SECURITY DEFINER entitlement fn agrees with our fixture setup.
	pool := db.Store.Pool().(*pgxpool.Pool)
	var lapsedEntitled, entitledEntitled bool
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT trakrf.org_is_entitled($1)`, lapsedOrg).Scan(&lapsedEntitled))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT trakrf.org_is_entitled($1)`, entitledOrg).Scan(&entitledEntitled))
	require.False(t, lapsedEntitled, "fixture: lapsed org must not be entitled")
	require.True(t, entitledEntitled, "fixture: entitled org must be entitled")

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

	// is402 asserts the response is a 402 carrying error.type == payment_required.
	is402 := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusPaymentRequired, rec.Code, "body: %s", rec.Body.String())
		var env struct {
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), "body: %s", rec.Body.String())
		require.Equal(t, "payment_required", env.Error.Type, "body: %s", rec.Body.String())
	}

	assetBody := map[string]any{"name": "Widget", "external_key": "WIDGET-1"}

	t.Run("lapsed org", func(t *testing.T) {
		// (1) Public write route — POST /api/v1/assets → 402 payment_required.
		is402(t, do(lapsedOrg, http.MethodPost, "/api/v1/assets", assetBody))

		// (2) Reads stay open — GET /api/v1/assets must NOT be 402.
		recGet := do(lapsedOrg, http.MethodGet, "/api/v1/assets", nil)
		require.NotEqual(t, http.StatusPaymentRequired, recGet.Code, "GET must not be gated; body: %s", recGet.Body.String())

		// (3) Internal paid route — POST /api/v1/scan-devices → 402.
		is402(t, do(lapsedOrg, http.MethodPost, "/api/v1/scan-devices", map[string]any{
			"name": "Dock", "type": "csl_cs463",
		}))

		// (4) Must-stay-open write — POST /api/v1/users/me/current-org must NOT be 402.
		recCurrentOrg := do(lapsedOrg, http.MethodPost, "/api/v1/users/me/current-org", map[string]any{
			"org_id": lapsedOrg,
		})
		require.NotEqual(t, http.StatusPaymentRequired, recCurrentOrg.Code,
			"current-org switch must stay open; body: %s", recCurrentOrg.Body.String())

		// (5) Operational output test action — POST .../test must NOT be 402.
		// Seed a real output device so the route resolves past the {id} lookup
		// and we observe the gate decision (it must let the request through to
		// the handler, which then 502s on the unreachable fake device — that
		// proves it was NOT 402'd at the gate).
		var outID int
		require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
			INSERT INTO trakrf.output_devices (org_id, name, transport, base_url)
			VALUES ($1, 'Buzzer', 'http', 'http://127.0.0.1:1/relay')
			RETURNING id`, lapsedOrg).Scan(&outID))
		recTest := do(lapsedOrg, http.MethodPost, "/api/v1/output-devices/"+strconv.Itoa(outID)+"/test", nil)
		require.NotEqual(t, http.StatusPaymentRequired, recTest.Code,
			"output test action must stay open; body: %s", recTest.Body.String())
	})

	t.Run("entitled org", func(t *testing.T) {
		// POST /api/v1/assets must NOT be 402 (proceeds to handler: 201 or a
		// validation 4xx, never 402).
		rec := do(entitledOrg, http.MethodPost, "/api/v1/assets", assetBody)
		require.NotEqual(t, http.StatusPaymentRequired, rec.Code,
			"entitled org must never be 402'd; got %d body: %s", rec.Code, rec.Body.String())
	})
}
