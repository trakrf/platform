//go:build integration
// +build integration

package inventory_test

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

	"github.com/trakrf/platform/backend/internal/handlers/inventory"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

var inventoryUserCounter int64

// seedInventoryOrgAndKey creates a test org (via testutil.CreateTestAccount, which uses
// the default `identifier='test-org'`), a user, and an API key with the given scopes.
// Returns the orgID and a signed API-key JWT.
func seedInventoryOrgAndKey(t *testing.T, pool *pgxpool.Pool, store *storage.Storage, scopes []string) (int, string) {
	t.Helper()
	orgID := testutil.CreateTestAccount(t, pool)
	return seedInventoryKeyForOrg(t, pool, store, orgID, scopes)
}

// createInventoryOrg inserts an org with a caller-supplied name/identifier so that
// cross-org tests can seed multiple orgs in one test without colliding on the
// UNIQUE(organizations.identifier) constraint.
func createInventoryOrg(t *testing.T, pool *pgxpool.Pool, name string) int {
	t.Helper()
	var orgID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ($1, $2, true) RETURNING id`,
		name, name,
	).Scan(&orgID))
	return orgID
}

// seedInventoryKeyForOrg creates a user + API key for an already-existing org.
func seedInventoryKeyForOrg(t *testing.T, pool *pgxpool.Pool, store *storage.Storage, orgID int, scopes []string) (int, string) {
	t.Helper()
	n := atomic.AddInt64(&inventoryUserCounter, 1)
	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash)
		 VALUES ('t', $1, 'stub') RETURNING id`,
		fmt.Sprintf("inv-user-%d@t.com", n),
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "k", scopes, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	tok, err := jwt.GenerateAPIKey(key.JTI, orgID, scopes, nil)
	require.NoError(t, err)
	return orgID, tok
}

func buildInventoryPublicWriteRouter(store *storage.Storage) *chi.Mux {
	handler := inventory.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.With(middleware.RequireScope("scans:write")).Post("/api/v1/inventory/save", handler.Save)
	})
	return r
}

func TestInventorySave_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "inv-wh", Name: "WH", Path: "inv-wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "inv-asset", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)

	body := fmt.Sprintf(`{"location_identifier":%q,"asset_identifiers":[%q]}`, loc.Identifier, asset.Identifier)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp["data"])
}

func TestInventorySave_APIKey_MultiAsset_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-multi-asset")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "ma-wh", Name: "WH", Path: "ma-wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	asset1, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ma-asset-1", Name: "A1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	asset2, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ma-asset-2", Name: "A2",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)
	body := fmt.Sprintf(
		`{"location_identifier":%q,"asset_identifiers":[%q,%q]}`,
		loc.Identifier, asset1.Identifier, asset2.Identifier,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, float64(2), data["count"])

	// Verify both rows landed in asset_scans.
	var rowCount int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM trakrf.asset_scans WHERE org_id = $1`, orgID).Scan(&rowCount))
	assert.Equal(t, 2, rowCount)
}

func TestInventorySave_EmptyAssetIdentifiers_Returns400(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-empty-assets")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	r := buildInventoryPublicWriteRouter(store)
	body := `{"location_identifier":"any-wh","asset_identifiers":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "asset_identifiers")
}

func TestInventorySave_LegacyShape_Returns400(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-legacy-shape")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	r := buildInventoryPublicWriteRouter(store)
	// Pre-TRA-533 shape — must be rejected post-AC2.
	body := `{"location_id":1,"asset_ids":[1]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "location_identifier")
}

func TestInventorySave_WrongScope_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-wrong-scope")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:read"})

	r := buildInventoryPublicWriteRouter(store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save",
		bytes.NewBufferString(`{"location_identifier":"any","asset_identifiers":["any"]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

// TRA-426: session-auth regression coverage for POST /inventory/save.
// TRA-397 moved this route under EitherAuth but the session-auth path
// (session JWT, not API key) was never exercised at the HTTP boundary.
// Keep this as a permanent regression guard.
func TestInventorySave_SessionAuth_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-session")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "sess-inv-wh", Name: "WH", Path: "sess-inv-wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "sess-inv-asset", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	sessToken, err := jwt.Generate(1, "sess-inv@t.com", &orgID)
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)

	body := fmt.Sprintf(`{"location_identifier":%q,"asset_identifiers":[%q]}`, loc.Identifier, asset.Identifier)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+sessToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp["data"])
}

func TestInventorySave_CrossOrg_Returns400(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// Two orgs: orgA owns the location+asset; orgB's API key tries to reference them.
	// testutil.CreateTestAccount uses a fixed identifier='test-org', so it can't be
	// called twice in the same test. Use createInventoryOrg for both to keep
	// identifiers distinct.
	orgA := createInventoryOrg(t, pool, "inv-orgA")
	orgB := createInventoryOrg(t, pool, "inv-orgB")
	_, _ = seedInventoryKeyForOrg(t, pool, store, orgA, []string{"scans:write"})
	_, tokenB := seedInventoryKeyForOrg(t, pool, store, orgB, []string{"scans:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "xo-wh", Name: "WH", Path: "xo-wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgA, Identifier: "xo-asset", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)
	body := fmt.Sprintf(`{"location_identifier":%q,"asset_identifiers":[%q]}`, loc.Identifier, asset.Identifier)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// With the identifier-only shape, cross-org references fail at handler-side
	// resolution (RLS filters orgA's location out of orgB's identifier lookup),
	// returning 400. Tenant isolation is preserved; the failure mode is just
	// reported at the validation layer instead of the storage layer.
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "xo-wh")
	assert.Contains(t, w.Body.String(), "not found")
}

func TestInventorySave_APIKey_LocationIdentifierNotFound(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-ident-loc-404")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})
	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "tra448-asset-2", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)
	body := `{"location_identifier":"ghost-wh","asset_identifiers":["tra448-asset-2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "ghost-wh")
}

func TestInventorySave_APIKey_AssetIdentifierNotFound(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-ident-asset-404")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra448-wh-2", Name: "WH", Path: "tra448-wh-2",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)
	body := `{"location_identifier":"tra448-wh-2","asset_identifiers":["ghost-asset"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "ghost-asset")
}
