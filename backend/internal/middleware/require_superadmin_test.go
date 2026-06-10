//go:build integration
// +build integration

package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// seedUserSession inserts a user (optionally superadmin) and returns a session JWT.
func seedUserSession(t *testing.T, pool *pgxpool.Pool, email string, superadmin bool) string {
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

func newSuperadminRouter(store *storage.Storage) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.RequireSuperadmin(store))
		r.Get("/api/v1/admin/orgs", okHandler)
	})
	return r
}

func TestRequireSuperadmin_AllowsSuperadmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-superadmin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	token := seedUserSession(t, pool, "super@x", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newSuperadminRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRequireSuperadmin_RejectsNonSuperadmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-superadmin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	token := seedUserSession(t, pool, "regular@x", false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newSuperadminRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestRequireSuperadmin_RejectsNoPrincipal(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-superadmin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orgs", nil)
	// No Authorization header.
	w := httptest.NewRecorder()
	newSuperadminRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
