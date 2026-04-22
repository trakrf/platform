//go:build integration
// +build integration

package locations_test

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

	"github.com/trakrf/platform/backend/internal/handlers/locations"
	"github.com/trakrf/platform/backend/internal/middleware"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func buildLocationsPublicWriteRouter(store *storage.Storage) *chi.Mux {
	handler := locations.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations", handler.Create)
		r.With(middleware.RequireScope("locations:write")).Put("/api/v1/locations/{identifier}", handler.Update)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{identifier}", handler.Delete)
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations/{identifier}/identifiers", handler.AddIdentifier)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{identifier}/identifiers/{identifierId}", handler.RemoveIdentifier)
	})
	return r
}

func buildLocationsHierarchyRouter(store *storage.Storage) *chi.Mux {
	handler := locations.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/ancestors", handler.GetAncestors)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/children", handler.GetChildren)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/descendants", handler.GetDescendants)
	})
	return r
}

func TestCreateLocation_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-create-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"wh-1","name":"Warehouse 1","valid_from":"2026-01-01","is_active":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotEmpty(t, w.Header().Get("Location"))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "wh-1", data["identifier"])
}

func TestCreateLocation_WrongScope_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-wrong-scope")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, readOnlyToken := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	r := buildLocationsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations",
		bytes.NewBufferString(`{"identifier":"wh-1","name":"Warehouse 1","valid_from":"2026-01-01","is_active":true}`))
	req.Header.Set("Authorization", "Bearer "+readOnlyToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestUpdateLocation_CrossOrg_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// orgA owns the location; orgB's API key attempts to update.
	orgA, _ := seedLocOrgAndKey(t, pool, store, "orgA-locations-write", []string{"locations:write"})
	_, tokenB := seedLocOrgAndKey(t, pool, store, "orgB-locations-write", []string{"locations:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "orgA-loc", Name: "A", Path: "orgA-loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	body := `{"name":"hijacked"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/orgA-loc", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm location untouched
	fetched, err := store.GetLocationByID(context.Background(), loc.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "A", fetched.Name)
}

func TestDeleteLocation_CrossOrg_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-delete-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// orgA owns the location; orgB's API key attempts to delete.
	orgA, _ := seedLocOrgAndKey(t, pool, store, "orgA-locations-write-delete", []string{"locations:write"})
	_, tokenB := seedLocOrgAndKey(t, pool, store, "orgB-locations-write-delete", []string{"locations:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "orgA-loc-delete", Name: "Survivor", Path: "orgA-loc-delete",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/orgA-loc-delete", nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// orgB's token sees orgA's location as not found (different org).
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm the location survives.
	fetched, err := store.GetLocationByID(context.Background(), loc.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "location must survive cross-org DELETE attempt")
	assert.Equal(t, "Survivor", fetched.Name)
}

func TestDeleteLocation_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-delete-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "to-delete", Name: "Bye", Path: "to-delete",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/to-delete", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped DeleteLocation from 202+body to 204 no-body.
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")
}

func TestAddIdentifier_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-add-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "ident-host-loc", Name: "WH", Path: "ident-host-loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	// TagIdentifierRequest.Type accepts only rfid/ble/barcode; use rfid.
	body := `{"type":"rfid","value":"EPC-LOC-ABC-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations/ident-host-loc/identifiers",
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
	assert.Equal(t, "EPC-LOC-ABC-123", data["value"])
}

func TestRemoveLocationIdentifier_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-remove-ident-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "ident-host", Name: "Host", Path: "ident-host",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToLocation(context.Background(), orgID, loc.ID, shared.TagIdentifierRequest{
		Type:  "rfid",
		Value: "EPC-LOC-HAPPY-1",
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	url := "/api/v1/locations/ident-host/identifiers/" + strconv.Itoa(ident.ID)
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

func TestRemoveLocationIdentifier_WrongLocationID_DoesNotDelete(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-remove-ident-wrong-owner")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	owningLoc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "owning-loc", Name: "Owner", Path: "owning-loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "other-loc", Name: "Other", Path: "other-loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToLocation(context.Background(), orgID, owningLoc.ID, shared.TagIdentifierRequest{
		Type:  "rfid",
		Value: "EPC-LOC-WRONG-1",
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	// DELETE via otherLoc's identifier targeting ident (which belongs to owningLoc).
	// Storage cross-location check: identifierID's location_id won't match other-loc's ID → no row
	// is soft-deleted, but TRA-407 changed the response to an unconditional 204. The invariant
	// being verified here is that the identifier itself survives — not the (now gone) "deleted"
	// flag in the response body.
	url := "/api/v1/locations/other-loc/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	fetched, err := store.GetIdentifierByID(context.Background(), ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must still exist since the path identifier didn't match its owner")
	assert.Equal(t, "EPC-LOC-WRONG-1", fetched.Value)
}

// TRA-407 Task 2: locations write/child/hierarchy routes accept {identifier}

func TestLocationsUpdate_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-update-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "update-target", Name: "Old Name", Path: "update-target",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"name":"New Name"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/update-target", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped UpdateLocation from 202 to 200.
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "New Name", data["name"])
}

func TestLocationsUpdate_UnknownIdentifier_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-update-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	r := buildLocationsPublicWriteRouter(store)
	body := `{"name":"Ghost"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/does-not-exist", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestLocationsDelete_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-delete-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "delete-target", Name: "Bye", Path: "delete-target",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/delete-target", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped DeleteLocation from 202+body to 204 no-body.
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")
}

func TestLocationsDelete_UnknownIdentifier_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-delete-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	r := buildLocationsPublicWriteRouter(store)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/ghost-loc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestLocationsAddIdentifier_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-addident-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "addident-target", Name: "Host", Path: "addident-target",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"type":"rfid","value":"EPC-TRA407-ADD-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations/addident-target/identifiers",
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
	assert.Equal(t, "EPC-TRA407-ADD-1", data["value"])
}

func TestLocationsAddIdentifier_UnknownParent_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-addident-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	r := buildLocationsPublicWriteRouter(store)
	body := `{"type":"rfid","value":"EPC-GHOST"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations/ghost-parent/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestLocationsRemoveIdentifier_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-removeident-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "removeident-target", Name: "Host", Path: "removeident-target",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToLocation(context.Background(), orgID, loc.ID, shared.TagIdentifierRequest{
		Type: "rfid", Value: "EPC-TRA407-REMOVE-1",
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	url := "/api/v1/locations/removeident-target/identifiers/" + strconv.Itoa(ident.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// TRA-407 flipped RemoveIdentifier from 202+body to 204 no-body.
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")
}

func TestLocationsRemoveIdentifier_UnknownParent_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-removeident-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	r := buildLocationsPublicWriteRouter(store)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/ghost-loc/identifiers/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestLocationsGetAncestors_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-ancestors-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	root, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "anc-root", Name: "Root", Path: "anc-root",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "anc-parent", Name: "Parent", Path: "anc-root.anc-parent",
		ParentLocationID: &root.ID,
		ValidFrom:        time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "anc-child", Name: "Child", Path: "anc-root.anc-parent.anc-child",
		ParentLocationID: &parent.ID,
		ValidFrom:        time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/anc-child/ancestors", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 2)

	rootNode := data[0].(map[string]any)
	assert.Equal(t, "anc-root", rootNode["identifier"])
	_, rootHasParent := rootNode["parent"]
	assert.False(t, rootHasParent, "root ancestor must omit parent")

	parentNode := data[1].(map[string]any)
	assert.Equal(t, "anc-parent", parentNode["identifier"])
	assert.Equal(t, "anc-root", parentNode["parent"], "non-root ancestor must carry parent identifier")
}

func TestLocationsGetAncestors_UnknownIdentifier_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-ancestors-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/no-such-loc/ancestors", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestLocationsGetChildren_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-children-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "children-parent", Name: "Parent", Path: "children-parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "children-child", Name: "Child", Path: "children-parent.children-child",
		ParentLocationID: &parent.ID,
		ValidFrom:        time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/children-parent/children", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 1)
	child := data[0].(map[string]any)
	assert.Equal(t, "children-child", child["identifier"])
	assert.Equal(t, "children-parent", child["parent"], "child must carry parent identifier")
}

func TestLocationsGetChildren_UnknownIdentifier_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-children-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/no-such-parent/children", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestLocationsGetDescendants_ByIdentifier_Works(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-descendants-by-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	root, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "desc-root", Name: "Root", Path: "desc-root",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	child, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "desc-child", Name: "Child", Path: "desc-root.desc-child",
		ParentLocationID: &root.ID,
		ValidFrom:        time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "desc-grandchild", Name: "GrandChild", Path: "desc-root.desc-child.desc-grandchild",
		ParentLocationID: &child.ID,
		ValidFrom:        time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/desc-root/descendants", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 2)

	byIdentifier := map[string]map[string]any{}
	for _, entry := range data {
		m := entry.(map[string]any)
		byIdentifier[m["identifier"].(string)] = m
	}
	require.Contains(t, byIdentifier, "desc-child")
	require.Contains(t, byIdentifier, "desc-grandchild")
	assert.Equal(t, "desc-root", byIdentifier["desc-child"]["parent"], "direct descendant must carry root as parent")
	assert.Equal(t, "desc-child", byIdentifier["desc-grandchild"]["parent"], "grandchild must carry intermediate parent")
}

func TestLocationsGetDescendants_UnknownIdentifier_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra407-loc-descendants-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/no-such-root/descendants", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// seedTra428HierarchyPair seeds two orgs with the SAME whs-01 → dock-01 identifiers.
// OrgB additionally has dock-01 → leak-b (a descendant that MUST NOT surface to orgA).
// Returns (orgA id, orgA token, orgB id) so callers can assert scoping from orgA's side.
func seedTra428HierarchyPair(t *testing.T, pool *pgxpool.Pool, store *storage.Storage, prefix string) (int, string, int) {
	t.Helper()

	orgA, tokenA := seedLocOrgAndKey(t, pool, store, prefix+"-orgA", []string{"locations:read"})
	orgB, _ := seedLocOrgAndKey(t, pool, store, prefix+"-orgB", []string{"locations:read"})

	now := time.Now()
	whsA, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "whs-01", Name: "OrgA Warehouse",
		ValidFrom: now, IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "dock-01", Name: "OrgA Dock",
		ParentLocationID: &whsA.ID,
		ValidFrom:        now, IsActive: true,
	})
	require.NoError(t, err)

	whsB, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgB, Identifier: "whs-01", Name: "OrgB Warehouse",
		ValidFrom: now, IsActive: true,
	})
	require.NoError(t, err)
	dockB, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgB, Identifier: "dock-01", Name: "OrgB Dock",
		ParentLocationID: &whsB.ID,
		ValidFrom:        now, IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgB, Identifier: "leak-b", Name: "OrgB Only",
		ParentLocationID: &dockB.ID,
		ValidFrom:        now, IsActive: true,
	})
	require.NoError(t, err)

	return orgA, tokenA, orgB
}

func assertNoInternalLocationFields(t *testing.T, item map[string]any) {
	t.Helper()
	assert.NotContains(t, item, "id", "internal surrogate id must not leak")
	assert.NotContains(t, item, "org_id", "org_id must not leak")
	assert.NotContains(t, item, "parent_location_id", "parent_location_id must not leak")
}

func TestLocationsGetAncestors_CrossOrg_NoLeak(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra428-loc-ancestors-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, tokenA, _ := seedTra428HierarchyPair(t, pool, store, "tra428-anc")

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/dock-01/ancestors", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 1, "ancestors must not include records from other organizations")
	got := data[0].(map[string]any)
	assert.Equal(t, "whs-01", got["identifier"])
	assert.Equal(t, "OrgA Warehouse", got["name"])
	assertNoInternalLocationFields(t, got)
}

func TestLocationsGetChildren_CrossOrg_NoLeak(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra428-loc-children-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, tokenA, _ := seedTra428HierarchyPair(t, pool, store, "tra428-chd")

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/whs-01/children", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 1, "children must not include records from other organizations")
	got := data[0].(map[string]any)
	assert.Equal(t, "dock-01", got["identifier"])
	assert.Equal(t, "OrgA Dock", got["name"])
	assertNoInternalLocationFields(t, got)
}

func TestLocationsGetDescendants_CrossOrg_NoLeak(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra428-loc-descendants-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, tokenA, _ := seedTra428HierarchyPair(t, pool, store, "tra428-dsc")

	r := buildLocationsHierarchyRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/whs-01/descendants", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 1, "descendants must not include records from other organizations")
	got := data[0].(map[string]any)
	assert.Equal(t, "dock-01", got["identifier"])
	assert.Equal(t, "OrgA Dock", got["name"])
	assertNoInternalLocationFields(t, got)
}
