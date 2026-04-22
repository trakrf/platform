//go:build integration
// +build integration

// TRA-425: internal /by-id/{id} write routes used by the frontend after
// TRA-407 flipped the public write surface to {identifier}. These tests
// cover the new UpdateByID / DeleteByID / AddIdentifierByID /
// RemoveIdentifierByID handlers registered under session-auth.

package locations

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
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func setupByIDRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Put("/api/v1/locations/by-id/{id}", handler.UpdateByID)
	r.Delete("/api/v1/locations/by-id/{id}", handler.DeleteByID)
	r.Post("/api/v1/locations/by-id/{id}/identifiers", handler.AddIdentifierByID)
	r.Delete("/api/v1/locations/by-id/{id}/identifiers/{identifierId}", handler.RemoveIdentifierByID)
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

func TestUpdateLocationByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "byid-loc-update", Name: "Before",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"name":"After"}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/locations/by-id/"+strconv.Itoa(created.ID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp UpdateLocationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "After", resp.Data.Name)
}

func TestUpdateLocationByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "byid-loc-cross", Name: "OrgA Loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"name":"hijacked"}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/locations/by-id/"+strconv.Itoa(created.ID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	fetched, err := store.GetLocationByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "OrgA Loc", fetched.Name)
}

func TestUpdateLocationByID_UnknownID_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/by-id/9999999",
		bytes.NewBufferString(`{"name":"ghost"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateLocationByID_InvalidID_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/by-id/abc",
		bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestUpdateLocationByID_NoSession_Returns401(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/by-id/1",
		bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestDeleteLocationByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "byid-loc-delete", Name: "Doomed",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/locations/by-id/"+strconv.Itoa(created.ID), nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	fetched, err := store.GetLocationByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched, "soft-deleted location should be hidden by GetLocationByID")
}

func TestDeleteLocationByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "byid-loc-del-cross", Name: "Survivor",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/locations/by-id/"+strconv.Itoa(created.ID), nil)
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	fetched, err := store.GetLocationByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "location must survive cross-org DELETE attempt")
	assert.Equal(t, "Survivor", fetched.Name)
}

func TestAddLocationIdentifierByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "byid-loc-add-ident", Name: "Host",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"type":"rfid","value":"EPC-LOC-BYID-1"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/locations/by-id/"+strconv.Itoa(created.ID)+"/identifiers",
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
	assert.Equal(t, "EPC-LOC-BYID-1", data["value"])
}

func TestAddLocationIdentifierByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "byid-loc-addident-cross", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"type":"rfid","value":"EPC-LOC-CROSS"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/locations/by-id/"+strconv.Itoa(created.ID)+"/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestAddLocationIdentifierByID_InvalidBody_Returns400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "byid-loc-addident-bad", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"type":"rfid"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/locations/by-id/"+strconv.Itoa(created.ID)+"/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestRemoveLocationIdentifierByID_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgID := testutil.CreateTestAccount(t, pool)
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "byid-loc-rm-ident", Name: "Host",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToLocation(context.Background(), orgID, created.ID, shared.TagIdentifierRequest{
		Type: "rfid", Value: "EPC-LOC-RM",
	})
	require.NoError(t, err)

	url := "/api/v1/locations/by-id/" + strconv.Itoa(created.ID) + "/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched, "identifier must be soft-deleted")
}

func TestRemoveLocationIdentifierByID_CrossOrg_Returns404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupTestAccounts(t, pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrgByID(t, pool, "byid-org-b-"+t.Name())
	handler := NewHandler(store)
	router := setupByIDRouter(handler)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "byid-loc-rm-ident-cross", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToLocation(context.Background(), orgA, created.ID, shared.TagIdentifierRequest{
		Type: "rfid", Value: "EPC-LOC-CROSS-RM",
	})
	require.NoError(t, err)

	url := "/api/v1/locations/by-id/" + strconv.Itoa(created.ID) + "/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req = withOrgContext(req, orgB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must survive cross-org DELETE attempt")
}
