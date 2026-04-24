//go:build integration
// +build integration

// TRA-212: Skipped by default - requires database setup
// Run with: go test -tags=integration ./...

package assets

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

// createTestLocationWithDesc is like createTestLocation but writes an empty
// description so pgx scans succeed in code paths that route the row through
// GetLocationByIdentifier (which targets a non-nullable string).
func createTestLocationWithDesc(t *testing.T, pool *pgxpool.Pool, orgID int, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, identifier, name, description, is_active)
		VALUES ($1, $2, $3, '', true)
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
		BuildCreateRequest()

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

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

// TRA-482: POST /api/v1/assets/{identifier}/identifiers with a tag value
// already attached to a different asset must return 409 Conflict.
//
// Before the 000032 migration this path silently created duplicate rows:
// the legacy UNIQUE(org_id, type, value, valid_from) constraint never fired
// because every insert used DEFAULT CURRENT_TIMESTAMP. The partial unique
// index UNIQUE(org_id, type, value) WHERE deleted_at IS NULL fixes that.
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
	_, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: accountID, Identifier: "SN-A-dup-host", Name: "Asset A", Type: "asset",
		ValidFrom: time.Time{}, IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: accountID, Identifier: "SN-B-dup-host", Name: "Asset B", Type: "asset",
		ValidFrom: time.Time{}, IsActive: true,
	})
	require.NoError(t, err)

	// Attach an identifier to asset B via the handler.
	body := `{"type":"rfid","value":"TRA-482-IDENT-DUP"}`
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/assets/SN-B-dup-host/identifiers", bytes.NewBufferString(body))
	reqB.Header.Set("Content-Type", "application/json")
	reqB = withOrgContext(reqB, accountID)
	wB := httptest.NewRecorder()
	router.ServeHTTP(wB, reqB)
	require.Equal(t, http.StatusCreated, wB.Code, wB.Body.String())

	// Act: attach the same value to asset A.
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/assets/SN-A-dup-host/identifiers", bytes.NewBufferString(body))
	reqA.Header.Set("Content-Type", "application/json")
	reqA = withOrgContext(reqA, accountID)
	wA := httptest.NewRecorder()
	router.ServeHTTP(wA, reqA)

	// Assert: 409 Conflict, body.error.type == "conflict".
	require.Equal(t, http.StatusConflict, wA.Code, wA.Body.String())
	var resp map[string]map[string]any
	require.NoError(t, json.Unmarshal(wA.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp["error"]["type"])
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
			BuildCreateRequest()

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
			BuildCreateRequest()

		body, err := json.Marshal(requestBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withOrgContext(req, accountID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		data := assertNoLeaks(t, w.Body.Bytes())
		// TRA-477: current_location is always present in the response; null when
		// the asset has no explicit parent and no scan-inferred location.
		assert.Contains(t, data, "current_location", "current_location must always be present")
		assert.Nil(t, data["current_location"], "current_location should be null when no parent and no scans")
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

	// Hydrated current_location must reflect the latest scan, not the stale column.
	item := data2[0].(map[string]any)
	assert.Equal(t, "LOC-WHS-02", item["current_location"])
}

// TRA-465: single-value ?location= happy path.
func TestListAssets_LocationFilter_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	loc := createTestLocation(t, pool, orgID, "WHS-01")

	a := testutil.NewAssetFactory(orgID).WithIdentifier("HP-ASSET-001").Build()
	created, err := store.CreateAsset(context.Background(), a)
	require.NoError(t, err)
	createTestScan(t, pool, orgID, created.ID, &loc, time.Now().UTC().Add(-1*time.Hour))

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	require.Len(t, data, 1)
	assert.Equal(t, "HP-ASSET-001", data[0].(map[string]any)["identifier"])
	assert.Equal(t, "LOC-WHS-01", data[0].(map[string]any)["current_location"])
	assert.EqualValues(t, 1, resp["total_count"])
}

// TRA-465: multi-value ?location=A&location=B has OR semantics.
func TestListAssets_LocationFilter_MultiValueOR(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	whs1 := createTestLocation(t, pool, orgID, "WHS-01")
	whs2 := createTestLocation(t, pool, orgID, "WHS-02")
	whs3 := createTestLocation(t, pool, orgID, "WHS-03")

	a1 := testutil.NewAssetFactory(orgID).WithIdentifier("OR-A-001").Build()
	c1, err := store.CreateAsset(context.Background(), a1)
	require.NoError(t, err)
	a2 := testutil.NewAssetFactory(orgID).WithIdentifier("OR-A-002").Build()
	c2, err := store.CreateAsset(context.Background(), a2)
	require.NoError(t, err)
	a3 := testutil.NewAssetFactory(orgID).WithIdentifier("OR-A-003").Build()
	c3, err := store.CreateAsset(context.Background(), a3)
	require.NoError(t, err)

	now := time.Now().UTC()
	createTestScan(t, pool, orgID, c1.ID, &whs1, now.Add(-1*time.Hour))
	createTestScan(t, pool, orgID, c2.ID, &whs2, now.Add(-1*time.Hour))
	createTestScan(t, pool, orgID, c3.ID, &whs3, now.Add(-1*time.Hour))

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01&location=LOC-WHS-02", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	require.Len(t, data, 2, "expected OR of WHS-01 and WHS-02 to include both assets but not the one at WHS-03")
	assert.EqualValues(t, 2, resp["total_count"])

	got := map[string]bool{}
	for _, row := range data {
		got[row.(map[string]any)["identifier"].(string)] = true
	}
	assert.True(t, got["OR-A-001"])
	assert.True(t, got["OR-A-002"])
	assert.False(t, got["OR-A-003"])
}

// TRA-468: PATCH/PUT without valid_from/valid_to must not zero or clobber existing values.
func TestUpdateAsset_DoesNotClobberValidDates(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	// Create with explicit valid_from and valid_to.
	vf := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	vt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	createReq := asset.Asset{
		OrgID:      accountID,
		Identifier: "TRA468-UPD-001",
		Name:       "update-test",
		Type:       "asset",
		ValidFrom:  vf,
		ValidTo:    &vt,
		IsActive:   true,
	}
	created, err := store.CreateAsset(context.Background(), createReq)
	require.NoError(t, err)
	require.NotNil(t, created)

	// PUT only the name — no valid_from/valid_to in body.
	newName := "renamed"
	updateReq := asset.UpdateAssetRequest{Name: &newName}
	body, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/TRA468-UPD-001", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"identifier"},
			Values: []string{"TRA468-UPD-001"},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	handler.UpdateAsset(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp UpdateAssetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, newName, resp.Data.Name)
	assert.True(t, resp.Data.ValidFrom.Equal(vf),
		"valid_from clobbered: got %v, want %v", resp.Data.ValidFrom, vf)
	require.NotNil(t, resp.Data.ValidTo, "valid_to clobbered to nil")
	assert.True(t, resp.Data.ValidTo.Equal(vt),
		"valid_to clobbered: got %v, want %v", resp.Data.ValidTo, vt)
}

// TRA-465: an asset with no scans is excluded from every ?location=X filter.
func TestListAssets_LocationFilter_ExcludesAssetsWithNoScans(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	createTestLocation(t, pool, orgID, "WHS-01") // location exists, but nothing scanned here

	a := testutil.NewAssetFactory(orgID).WithIdentifier("NO-SCAN-001").Build()
	_, err := store.CreateAsset(context.Background(), a)
	require.NoError(t, err)
	// Intentionally: no scan inserted.

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?location=LOC-WHS-01", nil)
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].([]any)
	assert.Empty(t, data, "asset with no scans must not match any location filter")

	// Sanity: unfiltered list should include the asset with current_location = null.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req2 = withOrgContext(req2, orgID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	data2, _ := resp2["data"].([]any)
	require.Len(t, data2, 1)
	assert.Nil(t, data2[0].(map[string]any)["current_location"])
}

// TRA-468: POST with no valid_to must omit the `valid_to` key from the response JSON.
func TestCreateAsset_OmitsValidToWhenNull(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	reqBody := `{"identifier":"TRA468-OMIT","name":"no-expiry","type":"asset","valid_from":"2026-01-01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)

	_, hasValidTo := data["valid_to"]
	assert.False(t, hasValidTo, "response contained valid_to key when none was set: %#v", data["valid_to"])
	_, hasValidFrom := data["valid_from"]
	assert.True(t, hasValidFrom, "response missing valid_from (should always be present)")
}

// TRA-468: POST with explicit valid_to must return it as RFC3339 on both POST and GET.
func TestCreateAsset_IncludesValidToWhenSet(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	reqBody := `{"identifier":"TRA468-KEEP","name":"with-expiry","type":"asset","valid_from":"2026-01-01","valid_to":"2027-06-15"}`
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(reqBody))
	reqC.Header.Set("Content-Type", "application/json")
	reqC = withOrgContext(reqC, accountID)
	wC := httptest.NewRecorder()
	router.ServeHTTP(wC, reqC)

	require.Equal(t, http.StatusCreated, wC.Code, wC.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(wC.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)
	vt, ok := data["valid_to"].(string)
	require.True(t, ok, "valid_to missing or wrong type on POST: %#v", data["valid_to"])
	_, err := time.Parse(time.RFC3339, vt)
	require.NoError(t, err, "valid_to not RFC3339 on POST: %q", vt)

	// GET the asset back via handler.GetAssetByID (direct call with chi context,
	// matching the pattern used by TestGetAssetByID) and verify the same shape round-trips.
	surrogateID := int(data["surrogate_id"].(float64))
	idStr := strconv.Itoa(surrogateID)
	reqG := httptest.NewRequest(http.MethodGet, "/api/v1/assets/by-id/"+idStr, nil)
	reqG = reqG.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{idStr},
		},
	}))
	reqG = withOrgContext(reqG, accountID)
	wG := httptest.NewRecorder()
	handler.GetAssetByID(wG, reqG)

	require.Equal(t, http.StatusOK, wG.Code, wG.Body.String())
	var getEnvelope map[string]any
	require.NoError(t, json.Unmarshal(wG.Body.Bytes(), &getEnvelope))
	getData := getEnvelope["data"].(map[string]any)
	assert.Equal(t, vt, getData["valid_to"], "GET valid_to differs from POST")
}

// TRA-482: After soft-deleting an identifier, the same tag value may be
// attached again (on the same or a different asset). Exercises the partial
// unique index's WHERE deleted_at IS NULL clause.
func TestAssetsAddIdentifier_AfterSoftDelete_ReusesValue(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	_, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: accountID, Identifier: "SN-reuse-host", Name: "Host", Type: "asset",
		ValidFrom: time.Time{}, IsActive: true,
	})
	require.NoError(t, err)

	// Attach.
	body := `{"type":"rfid","value":"TRA-482-REUSE"}`
	reqAdd := httptest.NewRequest(http.MethodPost, "/api/v1/assets/SN-reuse-host/identifiers", bytes.NewBufferString(body))
	reqAdd.Header.Set("Content-Type", "application/json")
	reqAdd = withOrgContext(reqAdd, accountID)
	wAdd := httptest.NewRecorder()
	router.ServeHTTP(wAdd, reqAdd)
	require.Equal(t, http.StatusCreated, wAdd.Code, wAdd.Body.String())

	// Extract the identifier id from the 201 response.
	var addResp map[string]any
	require.NoError(t, json.Unmarshal(wAdd.Body.Bytes(), &addResp))
	data, ok := addResp["data"].(map[string]any)
	require.True(t, ok, "response missing data object: %s", wAdd.Body.String())
	rawID, ok := data["id"]
	require.True(t, ok, "response data missing id: %s", wAdd.Body.String())
	// JSON numbers decode as float64; the DELETE route expects an int-shaped path segment.
	identifierID := int(rawID.(float64))

	// Soft-delete.
	delURL := fmt.Sprintf("/api/v1/assets/SN-reuse-host/identifiers/%d", identifierID)
	reqDel := httptest.NewRequest(http.MethodDelete, delURL, nil)
	reqDel = withOrgContext(reqDel, accountID)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)
	require.Equal(t, http.StatusNoContent, wDel.Code, wDel.Body.String())

	// Re-attach the same value.
	reqReadd := httptest.NewRequest(http.MethodPost, "/api/v1/assets/SN-reuse-host/identifiers", bytes.NewBufferString(body))
	reqReadd.Header.Set("Content-Type", "application/json")
	reqReadd = withOrgContext(reqReadd, accountID)
	wReadd := httptest.NewRecorder()
	router.ServeHTTP(wReadd, reqReadd)
	require.Equal(t, http.StatusCreated, wReadd.Code, wReadd.Body.String())
}

// --- TRA-477: current_location natural identifier ---

func TestCreateAsset_CurrentLocation_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	createTestLocationWithDesc(t, pool, accountID, "TRA477WHS")

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	body := `{"identifier":"TRA477-A1","name":"Asset","current_location":"LOC-TRA477WHS"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			Identifier      string  `json:"identifier"`
			CurrentLocation *string `json:"current_location"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.CurrentLocation, "current_location should be set from natural identifier")
	assert.Equal(t, "LOC-TRA477WHS", *resp.Data.CurrentLocation)
}

func TestCreateAsset_CurrentLocation_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	body := `{"identifier":"TRA477-A2","name":"Asset","current_location":"DOES-NOT-EXIST"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not found")
}

func TestUpdateAsset_CurrentLocation_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	createTestLocationWithDesc(t, pool, accountID, "TRA477UPD")

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Create without a location.
	createBody := `{"identifier":"TRA477-UA1","name":"Asset"}`
	creq := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(createBody))
	creq.Header.Set("Content-Type", "application/json")
	creq = withOrgContext(creq, accountID)
	cw := httptest.NewRecorder()
	router.ServeHTTP(cw, creq)
	require.Equal(t, http.StatusCreated, cw.Code, cw.Body.String())

	// PUT with current_location natural identifier.
	updateBody := `{"current_location":"LOC-TRA477UPD"}`
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/assets/TRA477-UA1", bytes.NewBufferString(updateBody))
	ureq.Header.Set("Content-Type", "application/json")
	ureq = withOrgContext(ureq, accountID)
	uw := httptest.NewRecorder()
	router.ServeHTTP(uw, ureq)
	require.Equal(t, http.StatusOK, uw.Code, uw.Body.String())

	var resp struct {
		Data struct {
			CurrentLocation *string `json:"current_location"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(uw.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.CurrentLocation)
	assert.Equal(t, "LOC-TRA477UPD", *resp.Data.CurrentLocation)
}

func TestGetAsset_LocationInferredFromLatestScan(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := createTestLocationWithDesc(t, pool, accountID, "TRA477SCAN")

	handler := NewHandler(store)
	router := setupTestRouter(handler)
	// Wire the identifier-keyed GET (not registered by setupTestRouter).
	router.Get("/api/v1/assets/{identifier}", handler.GetAssetByIdentifier)

	// Seed asset directly (no explicit current_location_id).
	a, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: accountID, Identifier: "TRA477-SA1", Name: "Scan Asset", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Insert a scan so asset_scans has a row pointing at the location; the
	// asset's own current_location_id remains NULL.
	createTestScan(t, pool, accountID, a.ID, &locID, time.Now())

	// GET single asset should infer current_location from the latest scan.
	greq := httptest.NewRequest(http.MethodGet, "/api/v1/assets/TRA477-SA1", nil)
	greq = withOrgContext(greq, accountID)
	gw := httptest.NewRecorder()
	router.ServeHTTP(gw, greq)
	require.Equal(t, http.StatusOK, gw.Code, gw.Body.String())

	var resp struct {
		Data struct {
			CurrentLocation *string `json:"current_location"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.CurrentLocation,
		"current_location must be inferred from the latest scan when current_location_id is NULL")
	assert.Equal(t, "LOC-TRA477SCAN", *resp.Data.CurrentLocation)
}

func TestCreateAsset_CurrentLocation_Disagree(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := createTestLocationWithDesc(t, pool, accountID, "TRA477WHS2")

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Send a current_location_id that deliberately disagrees with current_location.
	body := fmt.Sprintf(
		`{"identifier":"TRA477-A3","name":"Asset","current_location":"LOC-TRA477WHS2","current_location_id":%d}`,
		locID+99999,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "disagree")
}
