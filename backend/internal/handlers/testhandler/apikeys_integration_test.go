//go:build integration
// +build integration

package testhandler_test

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
	"github.com/trakrf/platform/backend/internal/handlers/testhandler"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

const (
	bbOrgIdentifier = "bb-test-org"
	bbOrgName       = "BB Test Org"
	bbUserEmail     = "bb-test@trakrf.invalid"
	bbUserName      = "BB Test User"
)

// seedBBTestOrg inserts the bb-test-org organization that the handler expects.
func seedBBTestOrg(t *testing.T, store *storage.Storage) int {
	t.Helper()
	org, err := store.CreateOrganization(context.Background(), bbOrgName, bbOrgIdentifier)
	require.NoError(t, err)
	return org.ID
}

// seedBBTestUser inserts the bb-test user via raw SQL (mirrors the seed fixture).
func seedBBTestUser(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var userID int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, 'stub') RETURNING id`,
		bbUserEmail, bbUserName,
	).Scan(&userID)
	require.NoError(t, err)
	return userID
}

func newTestRouter(t *testing.T, store *storage.Storage) *chi.Mux {
	t.Helper()
	h := testhandler.NewHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func TestMintAPIKey_Success(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-tra671-success")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_ = seedBBTestOrg(t, store)
	_ = seedBBTestUser(t, pool)

	r := newTestRouter(t, store)

	scopes := []string{"assets:read", "assets:write", "locations:read", "locations:write", "history:read"}
	body, _ := json.Marshal(map[string]any{"scopes": scopes})
	req := httptest.NewRequest(http.MethodPost, "/test/apikeys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		Token string `json:"token"`
		JTI   string `json:"jti"`
		Name  string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Token, "token must be present")
	assert.NotEmpty(t, resp.JTI, "jti must be present")
	assert.Equal(t, "schemathesis-mint", resp.Name)

	claims, err := jwt.ValidateAPIKey(resp.Token)
	require.NoError(t, err)
	assert.Equal(t, scopes, claims.Scopes)
	assert.Equal(t, resp.JTI, claims.Subject)
}

func TestMintAPIKey_RejectsInvalidScope(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-tra671-invalid-scope")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_ = seedBBTestOrg(t, store)
	_ = seedBBTestUser(t, pool)

	r := newTestRouter(t, store)

	body := []byte(`{"scopes":["assets:read","not-a-real-scope"]}`)
	req := httptest.NewRequest(http.MethodPost, "/test/apikeys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not-a-real-scope")
}

func TestMintAPIKey_RejectsEmptyScopes(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-tra671-empty-scopes")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_ = seedBBTestOrg(t, store)
	_ = seedBBTestUser(t, pool)

	r := newTestRouter(t, store)

	body := []byte(`{"scopes":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/test/apikeys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestMintAPIKey_MissingOrg(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-tra671-missing-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	// Intentionally DO NOT seed the org.

	r := newTestRouter(t, store)

	body := []byte(`{"scopes":["assets:read"]}`)
	req := httptest.NewRequest(http.MethodPost, "/test/apikeys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFailedDependency, w.Code, w.Body.String())
}

func TestMintAPIKey_MissingUser(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-tra671-missing-user")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	_ = seedBBTestOrg(t, store)
	// Intentionally DO NOT seed the user.

	r := newTestRouter(t, store)

	body := []byte(`{"scopes":["assets:read"]}`)
	req := httptest.NewRequest(http.MethodPost, "/test/apikeys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFailedDependency, w.Code, w.Body.String())
}
