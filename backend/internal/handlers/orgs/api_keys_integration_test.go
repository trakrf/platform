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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/handlers/orgs"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// seedAdminUser inserts a user with admin role in the org and returns (userID, sessionJWT).
func seedAdminUser(t *testing.T, pool *pgxpool.Pool, orgID int) (int, string) {
	t.Helper()
	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('admin', 'admin@example.com', 'stub') RETURNING id`,
	).Scan(&userID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, 'admin')`, orgID, userID)
	require.NoError(t, err)

	token, err := jwt.Generate(userID, "admin@example.com", &orgID)
	require.NoError(t, err)
	return userID, token
}

func newAdminRouter(t *testing.T, store *storage.Storage) *chi.Mux {
	t.Helper()
	pool := store.Pool().(*pgxpool.Pool)
	service := orgsservice.NewService(pool, store, nil)
	handler := orgs.NewHandler(store, service)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		handler.RegisterRoutes(r, store)
	})
	return r
}

func TestCreateAPIKey_Admin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	body := map[string]any{
		"name":   "TeamCentral sync",
		"scopes": []string{"assets:read", "locations:read"},
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var envelope struct {
		Data apikey.APIKeyCreateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	// Response must be wrapped in {"data": {...}}; a top-level "key" field must NOT exist.
	var flat map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &flat))
	assert.NotContains(t, flat, "key", "response must be wrapped in data envelope")
	resp := envelope.Data
	assert.NotEmpty(t, resp.Key)
	assert.Equal(t, "TeamCentral sync", resp.Name)
	assert.Equal(t, []string{"assets:read", "locations:read"}, resp.Scopes)

	// Key must validate as an api-key JWT
	claims, err := jwt.ValidateAPIKey(resp.Key)
	require.NoError(t, err)
	assert.Equal(t, orgID, claims.OrgID)
}

func TestCreateAPIKey_NonAdminForbidden(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('viewer', 'v@example.com', 'stub') RETURNING id`,
	).Scan(&userID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
        INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'operator')`,
		orgID, userID)
	require.NoError(t, err)

	token, err := jwt.Generate(userID, "v@example.com", &orgID)
	require.NoError(t, err)

	r := newAdminRouter(t, store)

	body := map[string]any{"name": "x", "scopes": []string{"assets:read"}}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListAPIKeys_ExcludesRevoked(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, sessionToken := seedAdminUser(t, pool, orgID)

	active, err := store.CreateAPIKey(context.Background(), orgID, "active",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)
	revoked, err := store.CreateAPIKey(context.Background(), orgID, "revoked",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)
	require.NoError(t, store.RevokeAPIKey(context.Background(), orgID, revoked.ID))

	r := newAdminRouter(t, store)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var out struct {
		Data []apikey.APIKeyListItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Data, 1)
	assert.Equal(t, active.ID, out.Data[0].ID)
	assert.NotEmpty(t, out.Data[0].JTI, "list response must include jti for disambiguation")
	assert.Equal(t, active.JTI, out.Data[0].JTI, "jti in response should match the stored row")
	assert.NotContains(t, w.Body.String(), "eyJ")
}

func TestCreateAPIKey_SoftCap(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, sessionToken := seedAdminUser(t, pool, orgID)

	for i := 0; i < apikey.ActiveKeyCap; i++ {
		_, err := store.CreateAPIKey(context.Background(), orgID, "k",
			[]string{"assets:read"}, userID, nil)
		require.NoError(t, err)
	}

	r := newAdminRouter(t, store)

	body := map[string]any{"name": "over-cap", "scopes": []string{"assets:read"}}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "10")
}

func TestRevokeAPIKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, sessionToken := seedAdminUser(t, pool, orgID)

	key, err := store.CreateAPIKey(context.Background(), orgID, "to-revoke",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)

	r := newAdminRouter(t, store)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%d", orgID, key.ID), nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	// Second delete on same id → 404
	req2 := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%d", orgID, key.ID), nil)
	req2.Header.Set("Authorization", "Bearer "+sessionToken)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code)
}

// TestRevokeAPIKey_CrossOrgReturns404 verifies that admin of org1 cannot revoke
// a key belonging to org2, even if they know its numeric id. The explicit
// WHERE org_id filter in storage.RevokeAPIKey enforces isolation.
func TestRevokeAPIKey_CrossOrgReturns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	org1 := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, org1)

	// Second org via raw SQL (CreateTestAccount uses a unique identifier, so one call max)
	var org2 int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Org 2', 'org-2', true) RETURNING id`,
	).Scan(&org2)
	require.NoError(t, err)

	// Create a creator user for org2's key (any user will do for FK)
	var creatorID int
	err = pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (name, email, password_hash)
		VALUES ('creator2', 'creator2@example.com', 'stub') RETURNING id`,
	).Scan(&creatorID)
	require.NoError(t, err)

	// Key belonging to org2
	victimKey, err := store.CreateAPIKey(context.Background(), org2, "victim",
		[]string{"assets:read"}, creatorID, nil)
	require.NoError(t, err)

	r := newAdminRouter(t, store)

	// Admin of org1 tries to delete via /orgs/{org1}/api-keys/{victimKey.ID} — RBAC passes,
	// but the handler's RevokeAPIKey call filters WHERE org_id=org1 AND id=victimKey.ID → 0 rows.
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%d", org1, victimKey.ID), nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	// Verify the victim key is still active in org2
	list, err := store.ListActiveAPIKeys(context.Background(), org2)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, victimKey.ID, list[0].ID)
}

func TestCreateAPIKey_EmptyBody_CleanMessage(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	body := w.Body.String()
	assert.NotContains(t, body, "EOF", "raw decoder error must not leak to clients")
	assert.NotContains(t, body, "unexpected end", "raw decoder error must not leak to clients")

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "bad_request", resp.Error.Type)
	assert.Equal(t, "Invalid JSON body", resp.Error.Title)
	assert.Empty(t, resp.Error.Detail, "detail must not carry runtime error text")
}

func TestCreateAPIKey_ValidationFailed_JSONFieldNames(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	// Valid JSON, missing required name and scopes fields.
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID),
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	body := w.Body.String()
	assert.NotContains(t, body, "CreateAPIKeyRequest", "raw validator output must not leak Go struct name")
	assert.NotContains(t, body, "'Name'", "field names must be JSON names, not Go struct names")
	assert.NotContains(t, body, "'Scopes'", "field names must be JSON names, not Go struct names")

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)

	fieldNames := make([]string, 0, len(resp.Error.Fields))
	for _, f := range resp.Error.Fields {
		fieldNames = append(fieldNames, f.Field)
	}
	assert.Contains(t, fieldNames, "name")
	assert.Contains(t, fieldNames, "scopes")
}
