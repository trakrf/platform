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
	"github.com/trakrf/platform/backend/internal/models/shared"
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
		r.With(middleware.RequireScope("assets:write")).Put("/api/v1/assets/{identifier}", handler.UpdateAsset)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{identifier}", handler.DeleteAsset)
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets/{identifier}/identifiers", handler.AddIdentifier)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{identifier}/identifiers/{identifierId}", handler.RemoveIdentifier)
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

	seededAsset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgA, Identifier: "orgA-asset", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	body := `{"name":"hijacked"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/orgA-asset", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm asset untouched
	fetched, err := store.GetAssetByID(context.Background(), &seededAsset.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "A", fetched.Name)
}

func TestDeleteAsset_CrossOrg_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-delete-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// orgA owns the asset; orgB's API key attempts to delete.
	orgA, _ := seedOrgAndKey(t, pool, store, "orgA-assets-write-delete", []string{"assets:write"})
	_, tokenB := seedOrgAndKey(t, pool, store, "orgB-assets-write-delete", []string{"assets:write"})

	seededAsset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgA, Identifier: "orgA-delete-target", Name: "Survivor", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/orgA-delete-target", nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Cross-org: GetAssetByIdentifier returns nil for orgB (identifier belongs to orgA),
	// so handler returns 404 before reaching storage delete.
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm the asset survives.
	fetched, err := store.GetAssetByID(context.Background(), &seededAsset.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "asset must survive cross-org DELETE attempt")
	assert.Equal(t, "Survivor", fetched.Name)
}

func TestDeleteAsset_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-delete-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "to-delete", Name: "Bye", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/to-delete", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped DeleteAsset from 202+body to 204 no-body.
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")
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

	_, err = store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ident-host", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	// TagIdentifierRequest.Type accepts only rfid/ble/barcode; use rfid.
	body := `{"type":"rfid","value":"EPC-ABC-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/ident-host/identifiers",
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

func TestRemoveAssetIdentifier_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-remove-ident-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	seededAsset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ident-host", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgID, seededAsset.ID, shared.TagIdentifierRequest{
		Type:  "rfid",
		Value: "EPC-HAPPY-1",
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	url := "/api/v1/assets/ident-host/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped RemoveIdentifier from 202+body to 204 no-body.
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")

	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched, "identifier row must be soft-deleted (GetIdentifierByID hides deleted rows)")
}

func TestRemoveAssetIdentifier_WrongAssetIdentifier_DoesNotDelete(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write-remove-ident-wrong-owner")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	owningAsset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "owning-asset", Name: "Owner", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "other-asset", Name: "Other", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgID, owningAsset.ID, shared.TagIdentifierRequest{
		Type:  "rfid",
		Value: "EPC-WRONG-1",
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	// DELETE via other-asset's {identifier} targeting ident (which belongs to owning-asset).
	// Storage cross-asset check: identifierID's asset_id won't match other-asset's ID → no row
	// is soft-deleted, but TRA-407 changed the response to an unconditional 204. The invariant
	// being verified here is that the identifier itself survives — not the (now gone) "deleted"
	// flag in the response body.
	url := "/api/v1/assets/other-asset/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must still exist since the path identifier didn't match its owner")
	assert.Equal(t, "EPC-WRONG-1", fetched.Value)
}

func TestAssetsUpdate_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-update-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "TRA-407B-UPDATE-1", Name: "Original", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	body := `{"name":"Updated"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/TRA-407B-UPDATE-1", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped UpdateAsset from 202 to 200.
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "Updated", data["name"])
}

func TestAssetsUpdate_UnknownIdentifier_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-update-unknown-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"name":"ghost"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/DOES-NOT-EXIST", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj := resp["error"].(map[string]any)
	assert.Equal(t, "not_found", errObj["type"])
}

func TestAssetsDelete_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-delete-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "TRA-407B-DELETE-1", Name: "ToDelete", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/TRA-407B-DELETE-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped DeleteAsset from 202+body to 204 no-body.
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")
}

func TestAssetsDelete_UnknownIdentifier_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-delete-unknown-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/DOES-NOT-EXIST", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj := resp["error"].(map[string]any)
	assert.Equal(t, "not_found", errObj["type"])
}

func TestAssetsAddIdentifier_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-addident-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "TRA-407B-ADDIDENT-1", Name: "Host", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	body := `{"type":"rfid","value":"EPC-407B-NEW"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/TRA-407B-ADDIDENT-1/identifiers",
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
	assert.Equal(t, "EPC-407B-NEW", data["value"])
}

func TestAssetsAddIdentifier_UnknownParent_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-addident-unknown-parent")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"type":"rfid","value":"EPC-GHOST"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/DOES-NOT-EXIST/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj := resp["error"].(map[string]any)
	assert.Equal(t, "not_found", errObj["type"])
}

func TestAssetsRemoveIdentifier_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-removeident-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	seededAsset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "TRA-407B-REMOVEIDENT-1", Name: "Host", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgID, seededAsset.ID, shared.TagIdentifierRequest{
		Type:  "rfid",
		Value: "EPC-407B-REMOVE",
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	url := "/api/v1/assets/TRA-407B-REMOVEIDENT-1/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped RemoveIdentifier from 202+body to 204 no-body.
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")

	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched, "identifier must be soft-deleted")
}

func TestAssetsRemoveIdentifier_UnknownParent_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-removeident-unknown-parent")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/DOES-NOT-EXIST/identifiers/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj := resp["error"].(map[string]any)
	assert.Equal(t, "not_found", errObj["type"])
}
