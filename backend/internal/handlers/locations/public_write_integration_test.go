//go:build integration
// +build integration

package locations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
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
	fetched, err := store.GetLocationByID(context.Background(), orgA, loc.ID)
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
	fetched, err := store.GetLocationByID(context.Background(), orgA, loc.ID)
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

	fetched, err := store.GetIdentifierByID(context.Background(), orgID, ident.ID)
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

	fetched, err := store.GetIdentifierByID(context.Background(), orgID, ident.ID)
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

	r := buildLocationsPublicReadRouter(store)
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

	r := buildLocationsPublicReadRouter(store)
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

	r := buildLocationsPublicReadRouter(store)
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

	r := buildLocationsPublicReadRouter(store)
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

	r := buildLocationsPublicReadRouter(store)
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

	r := buildLocationsPublicReadRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/no-such-root/descendants", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestCreateLocation_APIKey_Defaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-defaults")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	before := time.Now().Add(-2 * time.Second)
	body := `{"identifier":"tra447-loc-def","name":"Defaults"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["is_active"])
	vf, err := time.Parse(time.RFC3339, data["valid_from"].(string))
	require.NoError(t, err)
	after := time.Now().Add(2 * time.Second)
	assert.Truef(t, vf.After(before) && vf.Before(after),
		"valid_from %s must fall within [%s, %s]", vf, before, after)
}

func TestCreateLocation_APIKey_ParentIdentifier_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-parent-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-parent", Name: "Parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"identifier":"tra447-child","name":"Child","parent_identifier":"tra447-parent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "tra447-parent", data["parent"])
	depth, _ := data["depth"].(float64)
	assert.Equal(t, float64(parent.Depth+1), depth)
}

func TestCreateLocation_APIKey_ParentIdentifier_NotFound(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-parent-404")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"tra447-orphan","name":"x","parent_identifier":"ghost"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not found")
}

func TestCreateLocation_APIKey_UnknownField_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	for _, field := range []string{"parent_path", "path", "parent"} {
		t.Run(field, func(t *testing.T) {
			body := fmt.Sprintf(`{"identifier":"tra447-u-%s","name":"x","%s":"anything"}`, field, field)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), field)
		})
	}
}

func TestUpdateLocation_APIKey_ParentIdentifier_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-update-parent")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-u-parent", Name: "Parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-u-child", Name: "Child",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"parent_identifier":"tra447-u-parent"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra447-u-child", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "tra447-u-parent", data["parent"])
	// Reparented from root → child of tra447-u-parent. Depth advances.
	depth, _ := data["depth"].(float64)
	assert.Equal(t, float64(parent.Depth+1), depth, "child should now sit at parent.depth+1")
}

func TestUpdateLocation_APIKey_UnknownField_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-update-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-u-unknown", Name: "x",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"name":"x","parent_path":"nope"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra447-u-unknown", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "parent_path")
}

func TestCreateLocation_APIKey_ParentDisagree_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-parent-disagree")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-disagree-parent", Name: "Parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	bogusID := parent.ID + 9999
	body := fmt.Sprintf(`{"identifier":"tra447-d-child","name":"x","parent_identifier":"tra447-disagree-parent","parent_location_id":%d}`, bogusID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "disagree")
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

	r := buildLocationsPublicReadRouter(store)
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

	r := buildLocationsPublicReadRouter(store)
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

	r := buildLocationsPublicReadRouter(store)
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

// TRA-468: PUT without valid_from/valid_to must not clobber existing values.
func TestUpdateLocation_DoesNotClobberValidDates(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-loc-write-clobber-valid-dates")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	// Create with explicit valid_to.
	createBody := `{"identifier":"tra468-clobber","name":"clobber-test",` +
		`"valid_from":"2026-01-01","valid_to":"2027-01-01","is_active":true}`
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/locations",
		bytes.NewBufferString(createBody))
	reqC.Header.Set("Authorization", "Bearer "+token)
	reqC.Header.Set("Content-Type", "application/json")
	wC := httptest.NewRecorder()
	r.ServeHTTP(wC, reqC)
	require.Equal(t, http.StatusCreated, wC.Code, wC.Body.String())

	var createdResp map[string]any
	require.NoError(t, json.Unmarshal(wC.Body.Bytes(), &createdResp))
	createdData := createdResp["data"].(map[string]any)
	origValidFrom := createdData["valid_from"]
	origValidTo := createdData["valid_to"]
	require.NotNil(t, origValidFrom)
	require.NotNil(t, origValidTo, "seed create did not return valid_to")

	// PUT only the name — nothing else.
	updateBody := `{"name":"renamed-loc"}`
	reqU := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra468-clobber",
		bytes.NewBufferString(updateBody))
	reqU.Header.Set("Authorization", "Bearer "+token)
	reqU.Header.Set("Content-Type", "application/json")
	wU := httptest.NewRecorder()
	r.ServeHTTP(wU, reqU)
	require.Equal(t, http.StatusOK, wU.Code, wU.Body.String())

	var updatedResp map[string]any
	require.NoError(t, json.Unmarshal(wU.Body.Bytes(), &updatedResp))
	updatedData := updatedResp["data"].(map[string]any)

	assert.Equal(t, origValidFrom, updatedData["valid_from"],
		"valid_from clobbered on PUT")
	assert.Equal(t, origValidTo, updatedData["valid_to"],
		"valid_to clobbered on PUT")
}

// TRA-468: POST with no valid_to must omit the `valid_to` key from the response JSON.
func TestCreateLocation_OmitsValidToWhenNull(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-loc-write-omit-valid-to")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"tra468-loc-omit","name":"no-expiry","valid_from":"2026-01-01","is_active":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)

	_, hasValidTo := data["valid_to"]
	assert.False(t, hasValidTo, "response contained valid_to key when none was set: %#v", data["valid_to"])
	_, hasValidFrom := data["valid_from"]
	assert.True(t, hasValidFrom, "response missing valid_from (should always be present)")
}

// TRA-468: POST with explicit valid_to must return it as RFC3339.
func TestCreateLocation_IncludesValidToWhenSet(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-loc-write-include-valid-to")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"tra468-loc-keep","name":"with-expiry","valid_from":"2026-01-01","valid_to":"2027-06-15","is_active":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)
	vt, ok := data["valid_to"].(string)
	require.True(t, ok, "valid_to missing or wrong type: %#v", data["valid_to"])
	_, err := time.Parse(time.RFC3339, vt)
	assert.NoError(t, err, "valid_to not RFC3339: %q", vt)
}

func TestCreateLocation_DuplicateIdentifier_Returns409(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-dup-409")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"dup-loc-1","name":"first"}`

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusCreated, w1.Code, w1.Body.String())

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusConflict, w2.Code, w2.Body.String())

	var errResp modelerrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &errResp))
	assert.Equal(t, "conflict", errResp.Error.Type)
	assert.Contains(t, errResp.Error.Detail, "dup-loc-1")
}

func TestCreateLocation_AfterSoftDelete_ReusesIdentifier(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-reuse-after-delete")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	createBody := `{"identifier":"reuse-loc-1","name":"v1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/reuse-loc-1", nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)
	require.Equal(t, http.StatusNoContent, delW.Code)

	recreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/locations",
		bytes.NewBufferString(`{"identifier":"reuse-loc-1","name":"v2"}`))
	recreateReq.Header.Set("Authorization", "Bearer "+token)
	recreateReq.Header.Set("Content-Type", "application/json")
	rcW := httptest.NewRecorder()
	r.ServeHTTP(rcW, recreateReq)
	require.Equal(t, http.StatusCreated, rcW.Code, rcW.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rcW.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "reuse-loc-1", data["identifier"])
	assert.Equal(t, "v2", data["name"])
}

func TestDeleteLocation_SecondDeleteReturns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-delete-idempotent")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"idem-loc-1","name":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	d1 := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/idem-loc-1", nil)
	d1.Header.Set("Authorization", "Bearer "+token)
	d1w := httptest.NewRecorder()
	r.ServeHTTP(d1w, d1)
	require.Equal(t, http.StatusNoContent, d1w.Code)

	d2 := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/idem-loc-1", nil)
	d2.Header.Set("Authorization", "Bearer "+token)
	d2w := httptest.NewRecorder()
	r.ServeHTTP(d2w, d2)
	require.Equal(t, http.StatusNotFound, d2w.Code, d2w.Body.String())
}

func TestUpdateLocation_RenameToExistingIdentifier_Returns409(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-rename-conflict")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	mkPost := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusCreated, mkPost(`{"identifier":"rn-loc-a","name":"a"}`).Code)
	require.Equal(t, http.StatusCreated, mkPost(`{"identifier":"rn-loc-b","name":"b"}`).Code)

	upBody := `{"identifier":"rn-loc-a"}`
	upReq := httptest.NewRequest(http.MethodPut, "/api/v1/locations/rn-loc-b", bytes.NewBufferString(upBody))
	upReq.Header.Set("Authorization", "Bearer "+token)
	upReq.Header.Set("Content-Type", "application/json")
	upW := httptest.NewRecorder()
	r.ServeHTTP(upW, upReq)

	require.Equal(t, http.StatusConflict, upW.Code, upW.Body.String())
}

// TRA-476: PUT with explicit `valid_to: null` must clear the field (SQL NULL),
// matching the TRA-468 convention that null = "no constraint". Regression
// coverage for the same bug pattern seen on the assets endpoint in black-box
// eval #6.
func TestUpdateLocation_ValidToNull_ClearsField(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra476-loc-null-valid-to")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	validTo := time.Now().UTC().AddDate(1, 0, 0)
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra476-loc-null-vt", Name: "expires",
		Path: "tra476-loc-null-vt", ValidFrom: time.Now().UTC(), ValidTo: &validTo, IsActive: true,
	})
	require.NoError(t, err)

	body := `{"valid_to": null}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra476-loc-null-vt",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	_, hasValidTo := data["valid_to"]
	assert.False(t, hasValidTo, "valid_to key must be absent after clear: %#v", data["valid_to"])
}

// TRA-476: PUT with explicit `valid_from: null` must return 400. valid_from is
// NOT NULL in the database (TRA-468 convention).
func TestUpdateLocation_ValidFromNull_Returns400(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra476-loc-null-valid-from")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra476-loc-null-vf", Name: "has-valid-from",
		Path: "tra476-loc-null-vf", ValidFrom: time.Now().UTC(), IsActive: true,
	})
	require.NoError(t, err)

	body := `{"valid_from": null}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra476-loc-null-vf",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	errObj, ok := envelope["error"].(map[string]any)
	require.True(t, ok, "expected error envelope: %s", w.Body.String())
	assert.Equal(t, string(modelerrors.ErrValidation), errObj["type"],
		"expected validation_error type, got %v", errObj["type"])

	fields, ok := errObj["fields"].([]any)
	require.True(t, ok, "expected fields[] in error envelope: %s", w.Body.String())
	require.Len(t, fields, 1)
	first := fields[0].(map[string]any)
	assert.Equal(t, "valid_from", first["field"])
}
