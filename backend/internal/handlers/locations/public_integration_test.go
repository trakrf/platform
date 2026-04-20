//go:build integration
// +build integration

package locations_test

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

	"github.com/trakrf/platform/backend/internal/handlers/locations"
	"github.com/trakrf/platform/backend/internal/middleware"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

var locationsUserCounter int64

func seedLocOrgAndKey(t *testing.T, pool *pgxpool.Pool, store *storage.Storage, name string, scopes []string) (int, string) {
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

	n := atomic.AddInt64(&locationsUserCounter, 1)
	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash)
         VALUES ('t', $1, 'stub') RETURNING id`,
		fmt.Sprintf("locations-user-%d@t.com", n),
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "k", scopes, userID, nil)
	require.NoError(t, err)

	tok, err := jwt.GenerateAPIKey(key.JTI, orgID, scopes, nil)
	require.NoError(t, err)

	return orgID, tok
}

func buildLocationsPublicRouter(store *storage.Storage) *chi.Mux {
	handler := locations.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations", handler.ListLocations)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}", handler.GetLocationByIdentifier)
	})
	return r
}

func TestListLocations_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})

	// Create root location
	root, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "root-loc",
		Name:       "Root Location",
		Path:       "root-loc",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	// Create child location
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:            orgID,
		Identifier:       "child-loc",
		Name:             "Child Location",
		Path:             "root-loc.child-loc",
		ParentLocationID: &root.ID,
		ValidFrom:        time.Now(),
		IsActive:         true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 2, body["total_count"])

	data := body["data"].([]any)
	require.Len(t, data, 2)

	// Find child and verify parent field
	var foundChild bool
	for _, item := range data {
		row := item.(map[string]any)
		if row["identifier"] == "child-loc" {
			foundChild = true
			assert.Equal(t, "root-loc", row["parent"])
		}
	}
	assert.True(t, foundChild, "child-loc not found in response")
	assert.Contains(t, body, "limit")
	assert.Contains(t, body, "offset")
}

func TestListLocations_WrongScope(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"assets:read"})

	r := buildLocationsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestGetLocationByIdentifier_CrossOrgReturns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// orgA: create location
	orgAID, _ := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgAID,
		Identifier: "loc-in-orga",
		Name:       "OrgA Location",
		Path:       "loc-in-orga",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	// orgB: create key
	_, tokenB := seedLocOrgAndKey(t, pool, store, "loc-cross-org-b", []string{"locations:read"})

	r := buildLocationsPublicRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/loc-in-orga", nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}
