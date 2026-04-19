//go:build integration
// +build integration

package orgs_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/handlers/orgs"
	"github.com/trakrf/platform/backend/internal/middleware"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func newTestHandler(t *testing.T, store interface{ Pool() any }) *orgs.Handler {
	t.Helper()
	return nil // replaced inline below
}

func TestGetOrgMe_ValidAPIKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-public")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (name, email, password_hash)
		VALUES ('pub', 'pub@example.com', 'stub') RETURNING id`,
	).Scan(&userID)
	require.NoError(t, err)

	key, err := store.CreateAPIKey(context.Background(), orgID, "pub-key",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)
	token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
	require.NoError(t, err)

	service := orgsservice.NewService(pool, store, nil)
	handler := orgs.NewHandler(store, service)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", handler.GetOrgMe)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(orgID), body["id"])
	assert.Equal(t, "Test Organization", body["name"])
}

func TestGetOrgMe_SessionTokenRejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-public")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	sessionToken, err := jwt.Generate(1, "u@e.com", intPtr(42))
	require.NoError(t, err)

	service := orgsservice.NewService(pool, store, nil)
	handler := orgs.NewHandler(store, service)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", handler.GetOrgMe)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/me", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Unused stub — remove if linter complains.
var _ = newTestHandler

func intPtr(i int) *int { return &i }
