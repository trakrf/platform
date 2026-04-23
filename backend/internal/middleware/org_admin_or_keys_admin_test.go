//go:build integration
// +build integration

package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func seedUserWithRole(t *testing.T, pool *pgxpool.Pool, orgID int, role, email string) (int, string) {
	t.Helper()
	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ($1, $2, 'stub') RETURNING id`,
		email, email,
	).Scan(&userID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, $3)`, orgID, userID, role)
	require.NoError(t, err)
	token, err := jwt.Generate(userID, email, &orgID)
	require.NoError(t, err)
	return userID, token
}

func mintAPIKeyJWT(t *testing.T, store *storage.Storage, orgID int, scopes []string) string {
	t.Helper()
	pool := store.Pool().(*pgxpool.Pool)
	var seederID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ($1, $2, 'stub') RETURNING id`,
		fmt.Sprintf("seed-%d", orgID), fmt.Sprintf("seed-%d@ex", orgID),
	).Scan(&seederID)
	require.NoError(t, err)
	key, err := store.CreateAPIKey(context.Background(), orgID, "t", scopes,
		apikey.Creator{UserID: &seederID}, nil)
	require.NoError(t, err)
	signed, err := jwt.GenerateAPIKey(key.JTI, orgID, scopes, nil)
	require.NoError(t, err)
	return signed
}

func newCombinedAuthRouter(store *storage.Storage) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Route("/orgs/{id}/api-keys", func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.RequireOrgAdminOrKeysAdmin(store))
		r.Get("/", okHandler)
		r.Post("/", okHandler)
	})
	return r
}

func TestRequireOrgAdminOrKeysAdmin_SessionAdmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, token := seedUserWithRole(t, pool, orgID, "admin", "admin@x")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newCombinedAuthRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRequireOrgAdminOrKeysAdmin_SessionMemberForbidden(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, token := seedUserWithRole(t, pool, orgID, "operator", "op@x")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newCombinedAuthRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireOrgAdminOrKeysAdmin_APIKeyWithKeysAdmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	token := mintAPIKeyJWT(t, store, orgID, []string{"keys:admin"})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newCombinedAuthRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRequireOrgAdminOrKeysAdmin_APIKeyWithoutKeysAdmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	token := mintAPIKeyJWT(t, store, orgID, []string{"assets:read"})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newCombinedAuthRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireOrgAdminOrKeysAdmin_APIKeyWrongOrg(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	org1 := testutil.CreateTestAccount(t, pool)
	var org2 int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('O2', 'o2', true) RETURNING id`,
	).Scan(&org2))
	token := mintAPIKeyJWT(t, store, org2, []string{"keys:admin"}) // bound to org2

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", org1), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newCombinedAuthRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
