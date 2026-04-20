//go:build integration
// +build integration

package assets_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/assets"
	"github.com/trakrf/platform/backend/internal/middleware"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

var assetsUserCounter int64

func seedOrgAndKey(t *testing.T, pool *pgxpool.Pool, store *storage.Storage, name string, scopes []string) (int, string) {
	t.Helper()

	var orgID int
	if name == "" {
		orgID = testutil.CreateTestAccount(t, pool)
	} else {
		err := pool.QueryRow(context.Background(),
			`INSERT INTO trakrf.organizations (name, identifier) VALUES ($1, $2) RETURNING id`,
			name, name,
		).Scan(&orgID)
		require.NoError(t, err)
	}

	n := atomic.AddInt64(&assetsUserCounter, 1)
	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash)
         VALUES ('t', $1, 'stub') RETURNING id`,
		fmt.Sprintf("assets-user-%d@t.com", n),
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "k", scopes, userID, nil)
	require.NoError(t, err)

	tok, err := jwt.GenerateAPIKey(key.JTI, orgID, scopes, nil)
	require.NoError(t, err)

	return orgID, tok
}

func buildAssetsPublicRouter(store *storage.Storage) *chi.Mux {
	handler := assets.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", handler.ListAssets)
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}", handler.GetAssetByIdentifier)
	})
	return r
}

func TestListAssets_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:read"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:     orgID,
		Identifier: "wh-1",
		Name:      "Warehouse 1",
		Path:      "wh-1",
		ValidFrom: time.Now(),
		IsActive:  true,
	})
	require.NoError(t, err)

	_, err = store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:             orgID,
		Identifier:        "widget-42",
		Name:              "Widget",
		Type:              "asset",
		CurrentLocationID: &loc.ID,
		ValidFrom:         time.Now(),
		IsActive:          true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(50), body["limit"])
	assert.EqualValues(t, 1, body["total_count"])

	data := body["data"].([]any)
	require.Len(t, data, 1)
	row := data[0].(map[string]any)
	assert.Equal(t, "widget-42", row["identifier"])
	assert.Equal(t, "wh-1", row["current_location"])
	assert.NotContains(t, row, "org_id")
	assert.Contains(t, row, "surrogate_id")
	assert.Contains(t, row, "valid_from")
}

func TestListAssets_WrongScope(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"locations:read"})

	r := buildAssetsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestListAssets_UnknownParam(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:read"})

	r := buildAssetsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?mystery=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "unknown parameter")
}

func TestGetAssetByIdentifier_CrossOrgReturns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// orgA: create asset
	orgAID, _ := seedOrgAndKey(t, pool, store, "", []string{"assets:read"})
	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgAID,
		Identifier: "asset-in-orga",
		Name:       "OrgA Asset",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	// orgB: create key
	_, tokenB := seedOrgAndKey(t, pool, store, "cross-org-b", []string{"assets:read"})

	r := buildAssetsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/asset-in-orga", nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}
