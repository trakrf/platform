//go:build integration
// +build integration

package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupAPIKey(t *testing.T) (*storage.Storage, func(), int, string) {
	t.Setenv("JWT_SECRET", "test-secret-mid")
	store, cleanup := testutil.SetupTestDB(t)
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('mw test', 'mwtest@example.com', 'stub') RETURNING id`,
	).Scan(&userID)
	require.NoError(t, err)

	key, err := store.CreateAPIKey(context.Background(), orgID, "mw-key",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)

	token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
	require.NoError(t, err)

	return store, cleanup, orgID, token
}

func protectedHandler(w http.ResponseWriter, r *http.Request) {
	p := middleware.GetAPIKeyPrincipal(r)
	if p == nil {
		http.Error(w, "no principal", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"org_id": p.OrgID, "scopes": p.Scopes})
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	store, cleanup, orgID, token := setupAPIKey(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	middleware.APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(orgID), body["org_id"])
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	store, cleanup, _, _ := setupAPIKey(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	middleware.APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, `Bearer realm="trakrf-api"`, w.Header().Get("WWW-Authenticate"))
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, _ := resp["error"].(map[string]any)
	assert.Equal(t, "Authorization header is required", errObj["detail"])
}

func TestAPIKeyAuth_RejectsSessionToken(t *testing.T) {
	store, cleanup, _, _ := setupAPIKey(t)
	defer cleanup()

	sessionToken, err := jwt.Generate(1, "user@example.com", intPtr(42))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	middleware.APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_RevokedKeyRejected(t *testing.T) {
	store, cleanup, orgID, token := setupAPIKey(t)
	defer cleanup()

	list, err := store.ListActiveAPIKeys(context.Background(), orgID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NoError(t, store.RevokeAPIKey(context.Background(), orgID, list[0].ID))

	// Give the fire-and-forget last_used_at goroutine time to finish, then revoke
	// and issue a fresh request — the second request should see the revoked flag.
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	middleware.APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, `Bearer realm="trakrf-api"`, w.Header().Get("WWW-Authenticate"))
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, _ := resp["error"].(map[string]any)
	assert.Equal(t, "API key has been revoked", errObj["detail"])
}

func TestAPIKeyAuth_DBExpiredKeyRejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-mid")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('exp', 'exp@example.com', 'stub') RETURNING id`,
	).Scan(&userID)
	require.NoError(t, err)

	past := time.Now().Add(-1 * time.Hour)
	key, err := store.CreateAPIKey(context.Background(), orgID, "expired",
		[]string{"assets:read"}, userID, &past)
	require.NoError(t, err)

	// Generate a token WITHOUT exp claim so JWT parser doesn't reject;
	// the DB check must catch the expiry.
	token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	middleware.APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, `Bearer realm="trakrf-api"`, w.Header().Get("WWW-Authenticate"))
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, _ := resp["error"].(map[string]any)
	assert.Equal(t, "API key has expired", errObj["detail"])
}

func TestAPIKeyAuth_LastUsedBumped(t *testing.T) {
	store, cleanup, orgID, token := setupAPIKey(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	middleware.APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Wait for the fire-and-forget goroutine; generous margin.
	require.Eventually(t, func() bool {
		list, err := store.ListActiveAPIKeys(context.Background(), orgID)
		if err != nil || len(list) != 1 {
			return false
		}
		return list[0].LastUsedAt != nil
	}, 2*time.Second, 50*time.Millisecond)
}

func TestRequireScope(t *testing.T) {
	store, cleanup, _, token := setupAPIKey(t)
	defer cleanup()

	// Key has only "assets:read"; require "assets:write" → 403
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	chain := middleware.APIKeyAuth(store)(middleware.RequireScope("assets:write")(http.HandlerFunc(protectedHandler)))
	chain.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Required scope present → 200
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	chain2 := middleware.APIKeyAuth(store)(middleware.RequireScope("assets:read")(http.HandlerFunc(protectedHandler)))
	chain2.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func echoAnyPrincipalHandler(w http.ResponseWriter, r *http.Request) {
	if p := middleware.GetAPIKeyPrincipal(r); p != nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`api-key`))
		return
	}
	if c := middleware.GetUserClaims(r); c != nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`session`))
		return
	}
	http.Error(w, "no principal", http.StatusInternalServerError)
}

func TestRequireScope_SessionPassthrough(t *testing.T) {
	t.Setenv("JWT_SECRET", "rs-pass")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	_ = store

	orgID := 1
	sessionToken, err := jwt.Generate(1, "u@e.com", &orgID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()

	chain := middleware.Auth(middleware.RequireScope("assets:read")(http.HandlerFunc(echoAnyPrincipalHandler)))
	chain.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "session", w.Body.String())
}

func intPtr(i int) *int { return &i }
