//go:build integration
// +build integration

package orgs_test

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
	"github.com/trakrf/platform/backend/internal/handlers/orgs"
	"github.com/trakrf/platform/backend/internal/middleware"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// seedSessionUser inserts a user (optionally superadmin) and returns a session JWT.
func seedSessionUser(t *testing.T, pool *pgxpool.Pool, email string, superadmin bool) string {
	t.Helper()
	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_superadmin)
        VALUES ($1, $2, 'stub', $3) RETURNING id`,
		email, email, superadmin,
	).Scan(&userID)
	require.NoError(t, err)
	token, err := jwt.Generate(userID, email, nil)
	require.NoError(t, err)
	return token
}

// newAdminOrgRouter wires the superadmin org routes the way production does:
// session auth + RequireSuperadmin, registered via the orgs handler.
func newAdminOrgRouter(t *testing.T, store *storage.Storage) *chi.Mux {
	t.Helper()
	pool := store.Pool().(*pgxpool.Pool)
	service := orgsservice.NewService(pool, store, nil)
	handler := orgs.NewHandler(store, service, nil)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.ContentType)
		handler.RegisterRoutes(r, store)
	})
	return r
}

func TestListAllOrgs_Superadmin200(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-admin-orgs")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// An org the superadmin is NOT a member of.
	var orgID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Foreign Org', 'foreign-org', true) RETURNING id`,
	).Scan(&orgID))

	token := seedSessionUser(t, pool, "super@x", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newAdminOrgRouter(t, store).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	found := false
	for _, o := range body.Data {
		if o["name"] == "Foreign Org" {
			found = true
			assert.Equal(t, true, o["subscription_enabled"])
			assert.Equal(t, float64(0), o["member_count"])
		}
	}
	assert.True(t, found, "superadmin all-orgs list must include the non-member org")
}

func TestListAllOrgs_NonSuperadmin403(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-admin-orgs")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	token := seedSessionUser(t, pool, "regular@x", false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newAdminOrgRouter(t, store).ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func patchEntitlement(t *testing.T, store *storage.Storage, token string, orgID int, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/orgs/%d/entitlement", orgID), bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newAdminOrgRouter(t, store).ServeHTTP(w, req)
	return w
}

func TestUpdateEntitlement_SuperadminPersists(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-admin-orgs")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	var orgID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Target', 'target-org', true) RETURNING id`,
	).Scan(&orgID))
	token := seedSessionUser(t, pool, "super@x", true)

	// Disable entitlement on a non-member org.
	w := patchEntitlement(t, store, token, orgID, `{"subscription_enabled": false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, false, body.Data["subscription_enabled"])

	entitled, err := store.OrgIsEntitled(context.Background(), orgID)
	require.NoError(t, err)
	assert.False(t, entitled, "entitlement must reflect the disable immediately")

	// Re-enable with a future expiry.
	w = patchEntitlement(t, store, token, orgID,
		`{"subscription_enabled": true, "subscription_expires_at": "2999-01-01T00:00:00Z"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body.Data["subscription_enabled"])
	// timestamptz serializes in the connection's local tz, so compare the
	// instant, not the string representation.
	gotExpiry, perr := time.Parse(time.RFC3339, body.Data["subscription_expires_at"].(string))
	require.NoError(t, perr)
	wantExpiry, _ := time.Parse(time.RFC3339, "2999-01-01T00:00:00Z")
	assert.True(t, gotExpiry.Equal(wantExpiry), "got %v want %v", gotExpiry, wantExpiry)

	entitled, err = store.OrgIsEntitled(context.Background(), orgID)
	require.NoError(t, err)
	assert.True(t, entitled)
}

func TestUpdateEntitlement_ClearsExpiry(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-admin-orgs")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	var orgID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active, subscription_expires_at)
		 VALUES ('Expiry', 'expiry-org', true, now() + interval '1 day') RETURNING id`,
	).Scan(&orgID))
	token := seedSessionUser(t, pool, "super@x", true)

	// Omitting subscription_expires_at clears it (NULL = never expires).
	w := patchEntitlement(t, store, token, orgID, `{"subscription_enabled": true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	_, present := body.Data["subscription_expires_at"]
	assert.False(t, present, "expiry must be cleared (omitted from response) after clear")
}

func TestUpdateEntitlement_NonSuperadmin403(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-admin-orgs")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	var orgID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Theirs', 'theirs-org', true) RETURNING id`,
	).Scan(&orgID))
	// A regular org admin must not be able to extend entitlement.
	token := seedSessionUser(t, pool, "admin@x", false)

	w := patchEntitlement(t, store, token, orgID, `{"subscription_enabled": true}`)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestUpdateEntitlement_MissingEnabled400(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-admin-orgs")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	var orgID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Bad', 'bad-org', true) RETURNING id`,
	).Scan(&orgID))
	token := seedSessionUser(t, pool, "super@x", true)

	// A bare PATCH must be rejected, not silently flip the kill switch off.
	w := patchEntitlement(t, store, token, orgID, `{}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}
