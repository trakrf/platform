//go:build integration
// +build integration

package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupEitherAuth(t *testing.T) (*storage.Storage, func(), int, int, string, string) {
	t.Setenv("JWT_SECRET", "either-test")
	store, cleanup := testutil.SetupTestDB(t)
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('ea', 'ea@example.com', 'stub') RETURNING id`,
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "ea-key",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)

	apiTok, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
	require.NoError(t, err)

	sessTok, err := jwt.Generate(userID, "ea@example.com", &orgID)
	require.NoError(t, err)

	return store, cleanup, orgID, userID, apiTok, sessTok
}

func echoPrincipalHandler(w http.ResponseWriter, r *http.Request) {
	if p := middleware.GetAPIKeyPrincipal(r); p != nil {
		w.Header().Set("X-Principal", "api-key")
		return
	}
	if c := middleware.GetUserClaims(r); c != nil {
		w.Header().Set("X-Principal", "session")
		return
	}
	http.Error(w, "no principal", http.StatusInternalServerError)
}

func TestEitherAuth_DispatchesAPIKey(t *testing.T) {
	store, cleanup, _, _, apiTok, _ := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+apiTok)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "api-key", w.Header().Get("X-Principal"))
}

func TestEitherAuth_DispatchesSession(t *testing.T) {
	store, cleanup, _, _, _, sessTok := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+sessTok)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "session", w.Header().Get("X-Principal"))
}

func TestEitherAuth_MissingHeader(t *testing.T) {
	store, cleanup, _, _, _, _ := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TRA-449 D10: X-API-Key with no Authorization header should get a hint
// pointing at the correct header format rather than the generic
// missing-header 401.
func TestEitherAuth_XAPIKeyWithoutAuthorization_HintsBearer(t *testing.T) {
	store, cleanup, _, _, _, _ := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", "some-token-value")
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Authorization: Bearer",
		"detail should hint at the correct header format when X-API-Key is sent")
}

func TestEitherAuth_UnknownIssuer(t *testing.T) {
	store, cleanup, _, _, _, _ := setupEitherAuth(t)
	defer cleanup()

	// Hand-forged JWT with iss="attacker". Signature won't verify, but EitherAuth
	// must reject at dispatch time based on iss (no chain accepts this iss).
	forged := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJhdHRhY2tlciJ9.sig"
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestEitherAuth_GarbageToken(t *testing.T) {
	store, cleanup, _, _, _, _ := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestEitherAuth_BearerSchemeCaseInsensitive(t *testing.T) {
	store, cleanup, _, _, _, sessTok := setupEitherAuth(t)
	defer cleanup()

	cases := []string{"Bearer", "bearer", "BEARER", "BeArEr"}
	for _, scheme := range cases {
		t.Run(scheme, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", scheme+" "+sessTok)
			w := httptest.NewRecorder()
			middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, "session", w.Header().Get("X-Principal"))
		})
	}
}
