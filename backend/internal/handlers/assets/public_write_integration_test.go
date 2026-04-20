//go:build integration
// +build integration

package assets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
)

func buildAssetsPublicWriteRouter(store *storage.Storage) *chi.Mux {
	handler := assets.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets", handler.Create)
		r.With(middleware.RequireScope("assets:write")).Put("/api/v1/assets/{id}", handler.UpdateAsset)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{id}", handler.DeleteAsset)
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets/{id}/identifiers", handler.AddIdentifier)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	})
	return r
}

func TestCreateAsset_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-create-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"identifier":"api-create-1","name":"Via API","type":"asset"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotEmpty(t, w.Header().Get("Location"))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "api-create-1", data["identifier"])
}

func TestCreateAsset_WrongScope_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-wrong-scope")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, readOnlyToken := seedOrgAndKey(t, pool, store, "", []string{"assets:read"})
	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets",
		bytes.NewBufferString(`{"identifier":"x","name":"y","type":"asset"}`))
	req.Header.Set("Authorization", "Bearer "+readOnlyToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestUpdateAsset_CrossOrg_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// orgA owns the asset; orgB's API key attempts to update.
	orgA, _ := seedOrgAndKey(t, pool, store, "orgA-assets-write", []string{"assets:write"})
	_, tokenB := seedOrgAndKey(t, pool, store, "orgB-assets-write", []string{"assets:write"})

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgA, Identifier: "orgA-asset", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	body := `{"name":"hijacked"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/"+strconv.Itoa(asset.ID), bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm asset untouched
	fetched, err := store.GetAssetByID(context.Background(), &asset.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "A", fetched.Name)
}

func TestDeleteAsset_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-delete-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "to-delete", Name: "Bye", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+strconv.Itoa(asset.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	var resp map[string]bool
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp["deleted"])
}

func TestAddIdentifier_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-add-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "wh", Name: "WH", Path: "wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	_ = loc

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ident-host", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	// TagIdentifierRequest.Type accepts only rfid/ble/barcode; use rfid.
	body := `{"type":"rfid","value":"EPC-ABC-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/"+strconv.Itoa(asset.ID)+"/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "rfid", data["type"])
	assert.Equal(t, "EPC-ABC-123", data["value"])
}
