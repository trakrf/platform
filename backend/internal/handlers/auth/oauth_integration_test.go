//go:build integration
// +build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/apisecret"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func mkUserH(t *testing.T, pool *pgxpool.Pool, email string) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u',$1,'x') RETURNING id`, email).Scan(&id))
	return id
}

func TestOAuthToken_ClientCredentialsThenRefresh(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := mkUserH(t, pool, "oauth-e2e@example.com")

	// client_secret is the opaque secret shown once at key creation; only its
	// hash is stored on the row.
	clientSecret, err := apisecret.Generate()
	require.NoError(t, err)
	key, err := store.CreateAPIKey(ctx, orgID, "k", apisecret.Hash(clientSecret), []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	svc := authservice.NewService(pool, store, nil)
	h := authhandler.NewHandler(svc, store)
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	// client_credentials
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     key.JTI,
		"client_secret": clientSecret,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Bearer", resp["token_type"])
	assert.EqualValues(t, 900, resp["expires_in"])
	assert.NotEmpty(t, resp["access_token"])
	refresh, _ := resp["refresh_token"].(string)
	require.NotEmpty(t, refresh)

	// The minted access token authenticates as an api-key JWT with the key's scopes.
	claims, err := jwt.ValidateAccessToken(resp["access_token"].(string))
	require.NoError(t, err)
	assert.Equal(t, key.JTI, claims.Subject)

	// refresh_token
	body2, _ := json.Marshal(map[string]string{"grant_type": "refresh_token", "refresh_token": refresh})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	// Replay of the now-used refresh token is rejected.
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body2))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusUnauthorized, w3.Code)
}

func TestOAuthToken_BadSecretIs401(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := mkUserH(t, pool, "oauth-bad@example.com")
	secret, err := apisecret.Generate()
	require.NoError(t, err)
	key, err := store.CreateAPIKey(ctx, orgID, "k", apisecret.Hash(secret), []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	svc := authservice.NewService(pool, store, nil)
	h := authhandler.NewHandler(svc, store)
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     key.JTI,
		"client_secret": "not-a-valid-jwt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
