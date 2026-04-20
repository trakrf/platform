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
		r.With(middleware.RequireScope("locations:write")).Put("/api/v1/locations/{id}", handler.Update)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{id}", handler.Delete)
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations/{id}/identifiers", handler.AddIdentifier)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
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
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/"+strconv.Itoa(loc.ID), bytes.NewBufferString(body))
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

func TestDeleteLocation_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-delete-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "to-delete", Name: "Bye", Path: "to-delete",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/"+strconv.Itoa(loc.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	var resp map[string]bool
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp["deleted"])
}

func TestAddIdentifier_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-write-add-ident")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "ident-host-loc", Name: "WH", Path: "ident-host-loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)

	// TagIdentifierRequest.Type accepts only rfid/ble/barcode; use rfid.
	body := `{"type":"rfid","value":"EPC-LOC-ABC-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations/"+strconv.Itoa(loc.ID)+"/identifiers",
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
