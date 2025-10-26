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
	"github.com/trakrf/platform/backend/internal/testutil"
)

func setupTestRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	handler.RegisterRoutes(r)
	return r
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

	now := time.Now()
	requestBody := asset.Asset{
		Name:        "Test Asset",
		Identifier:  "TEST-001",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    map[string]any{"key": "value"},
		IsActive:    true,
		AccountID:   accountID,
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "Response body: %s", w.Body.String())

	var response map[string]asset.Asset
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Test Asset", response["data"].Name)
	assert.Equal(t, "TEST-001", response["data"].Identifier)
	assert.Greater(t, response["data"].ID, 0, "Should have a valid ID")
}

func TestCreateAsset_InvalidJSON(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAsset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	// First create an asset
	now := time.Now()
	createRequest := asset.Asset{
		Name:        "Test Asset Get",
		Identifier:  "TEST-002",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    map[string]any{"key": "value"},
		IsActive:    true,
		AccountID:   accountID,
	}

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Now get the asset
	idStr := strconv.Itoa(created.ID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+idStr, nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{idStr},
		},
	}))
	w := httptest.NewRecorder()

	handler.GetAsset(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "Response body: %s", w.Body.String())

	var response map[string]*asset.Asset
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Test Asset Get", response["data"].Name)
	assert.Equal(t, "TEST-002", response["data"].Identifier)
}

func TestGetAsset_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/999999", nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{"999999"},
		},
	}))
	w := httptest.NewRecorder()

	handler.GetAsset(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetAsset_InvalidID(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/invalid", nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{"invalid"},
		},
	}))
	w := httptest.NewRecorder()

	handler.GetAsset(w, req)

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

	// First create an asset
	now := time.Now()
	createRequest := asset.Asset{
		Name:        "Test Asset Update",
		Identifier:  "TEST-003",
		Type:        "equipment",
		Description: "Original description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    map[string]any{"key": "value"},
		IsActive:    true,
		AccountID:   accountID,
	}

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Now update it
	newName := "Updated Asset"
	newDescription := "Updated description"
	updateRequest := asset.UpdateAccountRequest{
		Name:        &newName,
		Description: &newDescription,
	}

	body, err := json.Marshal(updateRequest)
	require.NoError(t, err)

	idStr := strconv.Itoa(created.ID)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/"+idStr, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{idStr},
		},
	}))
	w := httptest.NewRecorder()

	handler.UpdateAsset(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "Response body: %s", w.Body.String())

	var response map[string]*asset.Asset
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, newName, response["data"].Name)
	assert.Equal(t, newDescription, response["data"].Description)
}

func TestDeleteAsset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	// First create an asset
	now := time.Now()
	createRequest := asset.Asset{
		Name:        "Test Asset Delete",
		Identifier:  "TEST-004",
		Type:        "equipment",
		Description: "Test description",
		ValidFrom:   now,
		ValidTo:     now.Add(24 * time.Hour),
		Metadata:    map[string]any{"key": "value"},
		IsActive:    true,
		AccountID:   accountID,
	}

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Now delete it
	idStr := strconv.Itoa(created.ID)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+idStr, nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{idStr},
		},
	}))
	w := httptest.NewRecorder()

	handler.DeleteAsset(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "Response body: %s", w.Body.String())

	var response map[string]bool
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["deleted"])

	// Verify it's soft deleted
	deleted, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted, "Asset should be soft deleted")
}

func TestDeleteAsset_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/999999", nil)
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"id"},
			Values: []string{"999999"},
		},
	}))
	w := httptest.NewRecorder()

	handler.DeleteAsset(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var response map[string]bool
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response["deleted"])
}

func TestListAssets(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)

	// Clean up any existing test data first
	testutil.CleanupAssets(t, pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	// Create multiple assets
	now := time.Now()
	for i := 1; i <= 3; i++ {
		createRequest := asset.Asset{
			Name:        fmt.Sprintf("Test List Asset %d", i),
			Identifier:  fmt.Sprintf("TEST-LIST-%d", i),
			Type:        "equipment",
			Description: "Test description",
			ValidFrom:   now,
			ValidTo:     now.Add(24 * time.Hour),
			Metadata:    map[string]any{},
			IsActive:    true,
			AccountID:   accountID,
		}
		_, err := store.CreateAsset(context.Background(), createRequest)
		require.NoError(t, err)
	}

	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "Response body: %s", w.Body.String())

	var response map[string][]asset.Asset
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(response["data"]), 3, "Should have at least 3 test assets")
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
	now := time.Now()

	var createdID int

	// Step 1: Create an asset
	t.Run("Create", func(t *testing.T) {
		requestBody := asset.Asset{
			Name:        "Workflow Test Asset",
			Identifier:  "WF-001",
			Type:        "equipment",
			Description: "Test workflow",
			ValidFrom:   now,
			ValidTo:     now.Add(24 * time.Hour),
			Metadata:    map[string]any{"test": true},
			IsActive:    true,
			AccountID:   accountID,
		}

		body, err := json.Marshal(requestBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code, "Response body: %s", w.Body.String())

		var response map[string]asset.Asset
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Workflow Test Asset", response["data"].Name)
		createdID = response["data"].ID
	})

	// Step 2: Read the asset
	t.Run("Read", func(t *testing.T) {
		idStr := strconv.Itoa(createdID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+idStr, nil)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"id"},
				Values: []string{idStr},
			},
		}))
		w := httptest.NewRecorder()

		handler.GetAsset(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code, "Response body: %s", w.Body.String())

		var response map[string]*asset.Asset
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Workflow Test Asset", response["data"].Name)
	})

	// Step 3: Update the asset
	t.Run("Update", func(t *testing.T) {
		newName := "Updated Workflow Asset"
		updateRequest := asset.UpdateAccountRequest{
			Name: &newName,
		}

		body, err := json.Marshal(updateRequest)
		require.NoError(t, err)

		idStr := strconv.Itoa(createdID)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/"+idStr, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"id"},
				Values: []string{idStr},
			},
		}))
		w := httptest.NewRecorder()

		handler.UpdateAsset(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code, "Response body: %s", w.Body.String())

		var response map[string]*asset.Asset
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, newName, response["data"].Name)
	})

	// Step 4: Delete the asset
	t.Run("Delete", func(t *testing.T) {
		idStr := strconv.Itoa(createdID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+idStr, nil)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"id"},
				Values: []string{idStr},
			},
		}))
		w := httptest.NewRecorder()

		handler.DeleteAsset(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code, "Response body: %s", w.Body.String())

		var response map[string]bool
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["deleted"])
	})
}
