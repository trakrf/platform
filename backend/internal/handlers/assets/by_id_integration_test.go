//go:build integration
// +build integration

// TRA-425: internal /by-id/{id} write routes used by the frontend after
// TRA-407 flipped the public write surface to {identifier}. These tests
// cover the new UpdateAssetByID / DeleteAssetByID / AddIdentifierByID /
// RemoveIdentifierByID handlers registered under session-auth.

package assets

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

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func setupByIDRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Put("/api/v1/assets/by-id/{id}", handler.UpdateAssetByID)
	r.Delete("/api/v1/assets/by-id/{id}", handler.DeleteAssetByID)
	r.Post("/api/v1/assets/by-id/{id}/identifiers", handler.AddIdentifierByID)
	r.Delete("/api/v1/assets/by-id/{id}/identifiers/{identifierId}", handler.RemoveIdentifierByID)
	return r
}

// createOrgByID is a local helper for cross-org tests since
// testutil.CreateTestAccount uses a fixed identifier that can't be inserted
// twice. Callers supply a distinct identifier.
func createOrgByID(t *testing.T, pool *pgxpool.Pool, identifier string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ($1, $2, true) RETURNING id`,
		identifier, identifier,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestUpdateAssetByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "byid-update-1", Name: "Before", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"name":"After"}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/assets/by-id/"+strconv.Itoa(created.ID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]*asset.Asset
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "After", resp["data"].Name)
}

func TestUpdateAssetByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgA, Identifier: "byid-cross-org", Name: "OrgA Asset", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"name":"hijacked"}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/assets/by-id/"+strconv.Itoa(created.ID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm asset untouched.
	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	assert.Equal(t, "OrgA Asset", fetched.Name)
}

func TestUpdateAssetByID_UnknownID_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/by-id/9999999",
		bytes.NewBufferString(`{"name":"ghost"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateAssetByID_InvalidID_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/by-id/abc",
		bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestUpdateAssetByID_NoSession_Returns401(t *testing.T) {
	// The router group that owns /by-id/{id} in production applies
	// middleware.Auth; here we exercise the handler without injecting a
	// session to prove the handler itself short-circuits with 401 when
	// GetRequestOrgID returns an error.
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/by-id/1",
		bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestDeleteAssetByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "byid-delete-1", Name: "Doomed", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/assets/by-id/"+strconv.Itoa(created.ID), nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// Confirm soft-delete actually took effect.
	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched, "soft-deleted asset should be hidden by GetAssetByID")
}

func TestDeleteAssetByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgA, Identifier: "byid-del-crossorg", Name: "Survivor", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/assets/by-id/"+strconv.Itoa(created.ID), nil)
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm asset not deleted.
	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "asset must survive cross-org DELETE attempt")
	assert.Equal(t, "Survivor", fetched.Name)
}

func TestAddIdentifierByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "byid-add-ident", Name: "Host", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"type":"rfid","value":"EPC-BYID-ADD-1"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/assets/by-id/"+strconv.Itoa(created.ID)+"/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "rfid", data["type"])
	assert.Equal(t, "EPC-BYID-ADD-1", data["value"])
}

func TestAddIdentifierByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgA, Identifier: "byid-add-ident-crossorg", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"type":"rfid","value":"EPC-CROSS"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/assets/by-id/"+strconv.Itoa(created.ID)+"/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestAddIdentifierByID_InvalidBody_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "byid-add-ident-badbody", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Missing required value field.
	body := `{"type":"rfid"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/assets/by-id/"+strconv.Itoa(created.ID)+"/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestRemoveIdentifierByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "byid-rm-ident", Name: "Host", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgID, created.ID, shared.TagIdentifierRequest{
		Type: "rfid", Value: "EPC-BYID-RM",
	})
	require.NoError(t, err)

	url := "/api/v1/assets/by-id/" + strconv.Itoa(created.ID) + "/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched, "identifier must be soft-deleted")
}

func TestRemoveIdentifierByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgA, Identifier: "byid-rm-ident-cross", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgA, created.ID, shared.TagIdentifierRequest{
		Type: "rfid", Value: "EPC-CROSS-RM",
	})
	require.NoError(t, err)

	url := "/api/v1/assets/by-id/" + strconv.Itoa(created.ID) + "/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Identifier must still exist.
	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must survive cross-org DELETE attempt")
}
