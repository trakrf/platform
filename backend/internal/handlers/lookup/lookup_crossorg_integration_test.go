//go:build integration
// +build integration

package lookup_test

import (
	"bytes"
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

	"github.com/trakrf/platform/backend/internal/handlers/lookup"
	"github.com/trakrf/platform/backend/internal/middleware"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

var lookupUserCounter int64

// seedLookupOrgAndSession creates an org (optionally with a caller-supplied
// identifier so multiple orgs can coexist in the same test) and a session
// JWT for a newly-minted admin user in that org.
func seedLookupOrgAndSession(t *testing.T, pool *pgxpool.Pool, orgIdentifier string) (int, string) {
	t.Helper()

	var orgID int
	if orgIdentifier == "" {
		orgID = testutil.CreateTestAccount(t, pool)
	} else {
		err := pool.QueryRow(context.Background(),
			`INSERT INTO trakrf.organizations (name, identifier, is_active)
			 VALUES ($1, $2, true) RETURNING id`,
			orgIdentifier, orgIdentifier,
		).Scan(&orgID)
		require.NoError(t, err)
	}

	n := atomic.AddInt64(&lookupUserCounter, 1)
	email := fmt.Sprintf("tra431-lookup-%d@t.com", n)
	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash)
		 VALUES ('t', $1, 'stub') RETURNING id`,
		email,
	).Scan(&userID))

	_, err := pool.Exec(context.Background(),
		`INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`,
		orgID, userID,
	)
	require.NoError(t, err)

	token, err := jwt.Generate(userID, email, &orgID)
	require.NoError(t, err)
	return orgID, token
}

func buildLookupRouter(store *storage.Storage) *chi.Mux {
	handler := lookup.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		handler.RegisterRoutes(r)
	})
	return r
}

// Forge the invariant break the ticket describes: an identifier row tenanted
// to orgA but whose asset_id/location_id FK points at an entity owned by
// orgB. Simulates a buggy INSERT path, sample-data seed, or admin psql
// session. The production code path never writes rows like this, but the
// public API must still refuse to dereference them.
func forgeCrossOrgAssetIdentifier(t *testing.T, pool *pgxpool.Pool, orgID int, value string, assetID int) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
		 VALUES ($1, 'rfid', $2, $3, true)`,
		orgID, value, assetID,
	)
	require.NoError(t, err)
}

func forgeCrossOrgLocationIdentifier(t *testing.T, pool *pgxpool.Pool, orgID int, value string, locationID int) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO trakrf.identifiers (org_id, type, value, location_id, is_active)
		 VALUES ($1, 'rfid', $2, $3, true)`,
		orgID, value, locationID,
	)
	require.NoError(t, err)
}

// TestLookupByTags_CrossOrg_AssetNoLeak: TRA-431 regression. An identifier
// row in orgA points to an asset owned by orgB (a broken invariant the FK
// does not prevent). The /lookup/tags endpoint must NOT surface orgB's
// asset to orgA's session.
func TestLookupByTags_CrossOrg_AssetNoLeak(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra431-lookup-asset-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA, tokenA := seedLookupOrgAndSession(t, pool, "")
	orgB, _ := seedLookupOrgAndSession(t, pool, "tra431-orgB-asset")

	orgBAsset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgB,
		Identifier: "orgB-secret-asset",
		Name:       "OrgB Secret Asset",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	const leakValue = "EPC-TRA431-ASSET-LEAK"
	forgeCrossOrgAssetIdentifier(t, pool, orgA, leakValue, orgBAsset.ID)

	r := buildLookupRouter(store)

	body, _ := json.Marshal(map[string]any{"type": "rfid", "values": []string{leakValue}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lookup/tags", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenA)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "response payload must carry a map under data")

	assert.Empty(t, data,
		"orgA /lookup/tags must not return orgB's asset via a cross-org identifier; got %v", data)
}

// TestLookupByTags_CrossOrg_LocationNoLeak: TRA-431 regression for the
// location arm of /lookup/tags. Mirrors the asset test above.
func TestLookupByTags_CrossOrg_LocationNoLeak(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra431-lookup-location-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA, tokenA := seedLookupOrgAndSession(t, pool, "")
	orgB, _ := seedLookupOrgAndSession(t, pool, "tra431-orgB-location")

	orgBLoc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgB,
		Identifier: "orgB-secret-location",
		Name:       "OrgB Secret Location",
		Path:       "orgB-secret-location",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	const leakValue = "EPC-TRA431-LOC-LEAK"
	forgeCrossOrgLocationIdentifier(t, pool, orgA, leakValue, orgBLoc.ID)

	r := buildLookupRouter(store)

	body, _ := json.Marshal(map[string]any{"type": "rfid", "values": []string{leakValue}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lookup/tags", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenA)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "response payload must carry a map under data")

	assert.Empty(t, data,
		"orgA /lookup/tags must not return orgB's location via a cross-org identifier; got %v", data)
}
