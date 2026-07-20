//go:build integration
// +build integration

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// The kits routes (/api/v1/kits, /api/v1/kits/verify) carry no org URL param;
// the operator gate must resolve the org from JWT claims (TRA-1033 — wiring
// RequireOrgOperator there 400'd every call with "Organization ID required").
func newCurrentOrgOperatorRouter(store *storage.Storage) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.RequireCurrentOrgRole(store, models.RoleOperator))
		r.Post("/api/v1/kits/verify", okHandler)
	})
	return r
}

func postVerify(t *testing.T, store *storage.Storage, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kits/verify", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	newCurrentOrgOperatorRouter(store).ServeHTTP(w, req)
	return w
}

func TestRequireCurrentOrgRole_AllowsOperatorOnOrglessRoute(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-current-org-role")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	_, token := seedUserWithRole(t, pool, orgID, "operator", "op@current-org")

	w := postVerify(t, store, token)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRequireCurrentOrgRole_RejectsViewer(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-current-org-role")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	_, token := seedUserWithRole(t, pool, orgID, "viewer", "viewer@current-org")

	w := postVerify(t, store, token)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestRequireCurrentOrgRole_RejectsNonMember(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-current-org-role")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// Member of orgA, but the JWT claims org is orgB.
	// (CreateTestAccount hardcodes its identifier, so seed orgB directly.)
	orgA := testutil.CreateTestAccount(t, pool)
	var orgB int
	err := pool.QueryRow(t.Context(), `
        INSERT INTO trakrf.organizations (name, identifier, is_active)
        VALUES ('Other Org', 'other-org', true) RETURNING id`).Scan(&orgB)
	require.NoError(t, err)
	userID, _ := seedUserWithRole(t, pool, orgA, "operator", "stranger@current-org")
	token, tokErr := jwt.Generate(userID, "stranger@current-org", &orgB)
	require.NoError(t, tokErr)

	w := postVerify(t, store, token)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestRequireCurrentOrgRole_RejectsMissingOrgContext(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-current-org-role")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID, _ := seedUserWithRole(t, pool, orgID, "operator", "no-org@current-org")
	// JWT without a current org.
	token, err := jwt.Generate(userID, "no-org@current-org", nil)
	require.NoError(t, err)

	w := postVerify(t, store, token)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

func TestRequireCurrentOrgRole_RejectsAnonymous(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-current-org-role")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	w := postVerify(t, store, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
