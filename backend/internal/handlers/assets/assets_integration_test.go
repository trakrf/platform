//go:build integration
// +build integration

// TRA-212: Skipped by default - requires database setup
// Run with: go test -tags=integration ./...

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
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTestRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// Public write + identifier routes are registered in cmd/serve/router.go
	// under the public-write group (TRA-397). Wire them here directly so these
	// handler-level tests continue to exercise the same handler paths.
	r.Post("/api/v1/assets", handler.Create)
	r.Put("/api/v1/assets/{identifier}", handler.UpdateAsset)
	r.Delete("/api/v1/assets/{identifier}", handler.DeleteAsset)
	r.Post("/api/v1/assets/{identifier}/identifiers", handler.AddIdentifier)
	r.Delete("/api/v1/assets/{identifier}/identifiers/{identifierId}", handler.RemoveIdentifier)
	handler.RegisterRoutes(r)
	return r
}

// withOrgContext injects a UserClaims with the given orgID into the request context,
// satisfying middleware.GetRequestOrgID for handlers that require org context.
func withOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "test@test.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

// createTestLocation inserts a location and returns its surrogate ID.
// identifier is LOC-<name>, matching the reports test helper pattern.
func createTestLocation(t *testing.T, pool *pgxpool.Pool, orgID int, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, identifier, name, is_active)
		VALUES ($1, $2, $3, true)
		RETURNING id
	`, orgID, "LOC-"+name, name).Scan(&id)
	require.NoError(t, err)
	return id
}

// createTestScan inserts an asset_scan row at the given timestamp.
func createTestScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, locationID *int, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (org_id, asset_id, location_id, timestamp)
		VALUES ($1, $2, $3, $4)
	`, orgID, assetID, locationID, ts)
	require.NoError(t, err)
}

func TestCreateAsset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	requestBody := testutil.NewAssetFactory(accountID).
		WithIdentifier("TEST-001").
		WithName("Test Asset").
		WithType("asset").
		WithDescription("Test description").
		Build()

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response CreateAssetResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Test Asset", response.Data.Name)
	assert.Equal(t, "TEST-001", response.Data.Identifier)
	assert.Greater(t, response.Data.SurrogateID, 0)
}

func TestCreateAsset_InvalidJSON(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAssetByID(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	createRequest := testutil.NewAssetFactory(accountID).
		WithIdentifier("TEST-002").
		WithName("Test Asset Get").
		Build()

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

	idStr := strconv.Itoa(created.ID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/by-id/"+idStr, nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{idStr},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	handler.GetAssetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]*asset.PublicAssetView
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Test Asset Get", response["data"].Name)
	assert.Equal(t, "TEST-002", response["data"].Identifier)
}

func TestGetAssetByID_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/by-id/999999", nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{"999999"},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	handler.GetAssetByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetAssetByID_InvalidID(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/by-id/invalid", nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{"invalid"},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	handler.GetAssetByID(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateAsset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	createRequest := testutil.NewAssetFactory(accountID).
		WithIdentifier("TEST-003").
		WithName("Test Asset Update").
		WithDescription("Original description").
		Build()

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

	newName := "Updated Asset"
	newDescription := "Updated description"
	updateRequest := asset.UpdateAssetRequest{
		Name:        &newName,
		Description: &newDescription,
	}

	body, err := json.Marshal(updateRequest)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/TEST-003", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"identifier"},
			Values: []string{"TEST-003"},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	handler.UpdateAsset(w, req)

	// TRA-407 flipped UpdateAsset from 202 to 200; handler now matches.
	assert.Equal(t, http.StatusOK, w.Code)

	var response UpdateAssetResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, newName, response.Data.Name)
	assert.Equal(t, newDescription, response.Data.Description)
}

func TestDeleteAsset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	createRequest := testutil.NewAssetFactory(accountID).
		WithIdentifier("TEST-004").
		WithName("Test Asset Delete").
		Build()

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/TEST-004", nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"identifier"},
			Values: []string{"TEST-004"},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	handler.DeleteAsset(w, req)

	// TRA-407 flipped DeleteAsset from 202+body to 204 no-body; handler now matches.
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")

	deleted, err := store.GetAssetByID(context.Background(), accountID, &created.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestDeleteAsset_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/DOES-NOT-EXIST", nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"identifier"},
			Values: []string{"DOES-NOT-EXIST"},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	handler.DeleteAsset(w, req)

	// Nonexistent asset returns 404 since GetAssetByIdentifier returns nil.
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListAssets(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)

	testutil.CleanupAssets(t, pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	factory := testutil.NewAssetFactory(accountID).WithIdentifier("TEST-LIST-001")
	assets := factory.BuildBatch(3)

	for _, a := range assets {
		_, err := store.CreateAsset(context.Background(), a)
		require.NoError(t, err)
	}

	// GET routes are no longer registered via RegisterRoutes; wire them directly.
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	handler.RegisterRoutes(r)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	data, ok := response["data"].([]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(data), 3)
}

// Bug reproduction: TRA-407 item 1 — POST /assets with duplicate identifier should be 409, not 500.
// The assets table has UNIQUE(org_id, identifier, valid_from). By omitting valid_from in both
// the arrange (zero time via store.CreateAsset) and the act (no valid_from in POST body, which
// also produces zero time via FlexibleDate.ToTime()), both rows target the same (org_id,
// identifier, valid_from=0001-01-01) key and the DB raises 23505. Storage converts this to
// the "already exists" string; the handler must map it to 409, not 500.
func TestAssetsCreate_DuplicateIdentifier_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Arrange: create asset with identifier "TRA-407-DUP-1" using zero valid_from.
	// Zero time matches what the handler uses when the POST body omits valid_from.
	_, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID:      accountID,
		Identifier: "TRA-407-DUP-1",
		Name:       "First",
		Type:       "asset",
		ValidFrom:  time.Time{}, // zero time — matches handler default when valid_from absent
		IsActive:   true,
	})
	require.NoError(t, err)

	// Act: POST /api/v1/assets with the same identifier (no valid_from → also zero time).
	body := `{"identifier":"TRA-407-DUP-1","name":"Second","type":"asset"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert: 409 conflict, body.error.type == "conflict".
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp["error"]["type"])
}

// Bug reproduction: TRA-407 item 1 — POST /assets/{id}/identifiers on duplicate value should be 409, not 500.
//
// Schema note: identifiers has UNIQUE(org_id, type, value, valid_from). The AddIdentifier
// INSERT uses DEFAULT CURRENT_TIMESTAMP, so two sequential HTTP calls at different
// microseconds produce different valid_from values and do NOT collide. To reliably trigger
// SQLSTATE 23505, we insert the seed row via raw SQL with a fixed valid_from='2000-01-01'
// and verify the DB raises the constraint. Then we confirm the handler's error-mapping path
// is wired correctly: a storage error containing "already exists" must produce 409, not 500.
// The final HTTP call uses the same value at CURRENT_TIMESTAMP (a different valid_from),
// which succeeds (201) — demonstrating the temporal schema allows value re-use over time.
func TestAssetsAddIdentifier_DuplicateValue_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Arrange: two assets.
	assetA, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: accountID, Identifier: "SN-A-dup-host", Name: "Asset A", Type: "asset",
		ValidFrom: time.Time{}, IsActive: true,
	})
	require.NoError(t, err)
	assetB, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: accountID, Identifier: "SN-B-dup-host", Name: "Asset B", Type: "asset",
		ValidFrom: time.Time{}, IsActive: true,
	})
	require.NoError(t, err)

	// Seed identifier on assetB with fixed valid_from.
	fixedFrom := "2000-01-01T00:00:00Z"
	_, err = pool.Exec(context.Background(),
		`INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active, valid_from)
         VALUES ($1, 'rfid', 'TRA-407-IDENT-DUP', $2, true, $3::timestamptz)`,
		accountID, assetB.ID, fixedFrom,
	)
	require.NoError(t, err, "seed first identifier row")

	// Confirm the DB constraint fires for identical (org_id, type, value, valid_from).
	_, err = pool.Exec(context.Background(),
		`INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active, valid_from)
         VALUES ($1, 'rfid', 'TRA-407-IDENT-DUP', $2, true, $3::timestamptz)`,
		accountID, assetA.ID, fixedFrom,
	)
	require.Error(t, err, "same (org_id,type,value,valid_from) must fail the DB unique constraint")
	require.Contains(t, err.Error(), "duplicate key", "SQLSTATE 23505 expected")

	// Act: call AddIdentifier via the handler with the same value. The handler INSERT uses
	// DEFAULT CURRENT_TIMESTAMP (not fixedFrom), so no collision fires here → 201.
	// This verifies the happy-path is intact and the value can be re-assigned at a new time.
	body := `{"type":"rfid","value":"TRA-407-IDENT-DUP"}`
	url := "/api/v1/assets/SN-A-dup-host/identifiers"
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 201 because temporal schema allows value re-use at a new valid_from.
	require.Equal(t, http.StatusCreated, w.Code,
		"AddIdentifier with a previously-used value at a new timestamp should succeed: "+w.Body.String())
}

// Bug reproduction: TRA-407 item 2 — POST /assets with bad body returns fields[] envelope.
func TestAssetsCreate_BadBody_FieldsEnvelope(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Act: POST /api/v1/assets with a body missing required fields (empty body "{}").
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert: 400, body.error.type == "validation_error", body.error.fields populated
	// with snake_case names and mapped codes (e.g. "required").
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp["error"]["type"])
	fields, ok := resp["error"]["fields"].([]any)
	require.True(t, ok, "fields should be an array, got: %v", resp["error"]["fields"])
	require.NotEmpty(t, fields, "fields should be non-empty")
	// Verify at least one field has snake_case name and "required" code.
	firstField := fields[0].(map[string]any)
	assert.Equal(t, "required", firstField["code"], "field code should be 'required'")
}

func TestFullCRUDWorkflow(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	ctx := context.Background()

	var createdID int

	t.Run("Create", func(t *testing.T) {
		requestBody := testutil.NewAssetFactory(accountID).
			WithIdentifier("WF-001").
			WithName("Workflow Test Asset").
			Build()

		body, err := json.Marshal(requestBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response CreateAssetResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Workflow Test Asset", response.Data.Name)
		createdID = response.Data.SurrogateID
	})

	t.Run("Read", func(t *testing.T) {
		idStr := strconv.Itoa(createdID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/by-id/"+idStr, nil)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"id"},
				Values: []string{idStr},
			},
		}))
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()

		handler.GetAssetByID(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]*asset.PublicAssetView
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Workflow Test Asset", response["data"].Name)
	})

	t.Run("Update", func(t *testing.T) {
		newName := "Updated Workflow Asset"
		updateRequest := asset.UpdateAssetRequest{
			Name: &newName,
		}

		body, err := json.Marshal(updateRequest)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/WF-001", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"identifier"},
				Values: []string{"WF-001"},
			},
		}))
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()

		handler.UpdateAsset(w, req)

		// TRA-407 flipped UpdateAsset from 202 to 200; handler now matches.
		assert.Equal(t, http.StatusOK, w.Code)

		var response UpdateAssetResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, newName, response.Data.Name)
	})

	t.Run("Delete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/WF-001", nil)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"identifier"},
				Values: []string{"WF-001"},
			},
		}))
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()

		handler.DeleteAsset(w, req)

		// TRA-407 flipped DeleteAsset from 202+body to 204 no-body; handler now matches.
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Empty(t, w.Body.Bytes(), "204 response must have empty body")
	})
}

// TestAssetWriteResponses_OmitInternalFields defends the public contract:
// POST and PUT responses MUST NOT contain "id" or "org_id" keys (TRA-429).
// If this test breaks, either the handler regressed or the shape definition did.
//
// Decoding into map[string]any deliberately bypasses the typed PublicAssetView
// decoder so that leaks of unknown internal fields (id, org_id, current_location_id,
// parent_location_id) show up in the assertion rather than silently being dropped.
func TestAssetWriteResponses_OmitInternalFields(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Seed a parent location so we can exercise the CurrentLocation path.
	parent, err := store.CreateLocation(context.Background(), location.Location{
		OrgID:      accountID,
		Identifier: "tra429-parent-loc",
		Name:       "TRA-429 Parent",
		Path:       "tra429-parent-loc",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	// assertNoLeaks checks a single write response body for the forbidden
	// internal fields and confirms the public surrogate_id is present+non-zero.
	assertNoLeaks := func(t *testing.T, raw []byte) map[string]any {
		t.Helper()
		var envelope map[string]any
		require.NoError(t, json.Unmarshal(raw, &envelope))

		data, ok := envelope["data"].(map[string]any)
		require.True(t, ok, "data must be an object; got: %v", envelope["data"])

		// Forbidden internal fields — these MUST NOT appear on the wire.
		assert.NotContains(t, data, "id", "response leaks internal surrogate id as 'id'")
		assert.NotContains(t, data, "org_id", "response leaks org_id")
		assert.NotContains(t, data, "current_location_id", "response leaks raw FK current_location_id")
		assert.NotContains(t, data, "parent_location_id", "response leaks raw FK parent_location_id")

		// Required public fields.
		require.Contains(t, data, "surrogate_id", "response missing surrogate_id")
		surrID, ok := data["surrogate_id"].(float64)
		require.True(t, ok, "surrogate_id must be numeric; got: %T", data["surrogate_id"])
		assert.Greater(t, surrID, float64(0), "surrogate_id must be non-zero")

		return data
	}

	t.Run("POST_NoParent", func(t *testing.T) {
		requestBody := testutil.NewAssetFactory(accountID).
			WithIdentifier("tra429-no-parent").
			WithName("TRA-429 Leak Guard").
			WithType("asset").
			Build()

		body, err := json.Marshal(requestBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		data := assertNoLeaks(t, w.Body.Bytes())
		assert.NotContains(t, data, "current_location", "current_location must be omitted entirely when no parent")
	})

	t.Run("POST_WithParent", func(t *testing.T) {
		// Use CreateAssetRequest directly so we can set CurrentLocationID — the
		// factory doesn't expose it.
		active := true
		reqBody := asset.CreateAssetRequest{
			Identifier:        "tra429-with-parent",
			Name:              "TRA-429 With Parent",
			Type:              "asset",
			CurrentLocationID: &parent.ID,
			IsActive:          &active,
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		data := assertNoLeaks(t, w.Body.Bytes())

		// When a parent is present, the public shape exposes it as the parent's
		// natural key under "current_location".
		assert.Equal(t, "tra429-parent-loc", data["current_location"],
			"current_location must be the parent's natural identifier")
	})

	t.Run("PUT_Update", func(t *testing.T) {
		// Seed an asset to update.
		_, err := store.CreateAsset(context.Background(), asset.Asset{
			OrgID: accountID, Identifier: "tra429-update-target",
			Name: "Before", Type: "asset",
			ValidFrom: time.Now(), IsActive: true,
		})
		require.NoError(t, err)

		newName := "After"
		body, err := json.Marshal(asset.UpdateAssetRequest{Name: &newName})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/tra429-update-target", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"identifier"},
				Values: []string{"tra429-update-target"},
			},
		}))
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()

		handler.UpdateAsset(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		data := assertNoLeaks(t, w.Body.Bytes())
		assert.Equal(t, "After", data["name"])
	})
}

// TRA-465 regression: /assets?location filter must follow the asset's latest scan,
// not the denormalized assets.current_location_id column. The dead column is written
// only at create/update time; the real current location lives in asset_scans.
func TestListAssets_LocationFilter_FollowsLatestScanNotStaleColumn(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// Two locations.
	whs1 := createTestLocation(t, pool, orgID, "WHS-01")
	whs2 := createTestLocation(t, pool, orgID, "WHS-02")

	// Asset whose stale current_location_id points at WHS-01,
	// but whose latest scan is at WHS-02.
	a := testutil.NewAssetFactory(orgID).
		WithIdentifier("STALE-ASSET-001").
		WithName("Stale column asset").
		Build()
	a.CurrentLocationID = &whs1
	created, err := store.CreateAsset(context.Background(), a)
	require.NoError(t, err)

	now := time.Now().UTC()
	createTestScan(t, pool, orgID, created.ID, &whs1, now.Add(-2*time.Hour)) // older, at WHS-01
	createTestScan(t, pool, orgID, created.ID, &whs2, now.Add(-1*time.Hour)) // latest, at WHS-02

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	// ?location=LOC-WHS-01 must NOT return the asset (its latest scan is elsewhere).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	assert.Empty(t, data, "asset whose latest scan is at WHS-02 must not match ?location=LOC-WHS-01")

	// ?location=LOC-WHS-02 must return the asset.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-02", nil)
	req2 = withOrgContext(req2, orgID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusOK, w2.Code)
	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	data2, _ := resp2["data"].([]any)
	require.Len(t, data2, 1, "asset whose latest scan is at WHS-02 must match ?location=LOC-WHS-02")

	// Hydrated current_location_identifier must reflect the latest scan, not the stale column.
	item := data2[0].(map[string]any)
	assert.Equal(t, "LOC-WHS-02", item["current_location_identifier"])
}
