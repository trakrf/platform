package assets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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

	requestBody := testutil.NewAssetFactory(accountID).
		WithIdentifier("TEST-001").
		WithName("Test Asset").
		WithType("equipment").
		WithDescription("Test description").
		Build()

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]asset.Asset
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Test Asset", response["data"].Name)
	assert.Equal(t, "TEST-001", response["data"].Identifier)
	assert.Greater(t, response["data"].ID, 0)
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

	createRequest := testutil.NewAssetFactory(accountID).
		WithIdentifier("TEST-002").
		WithName("Test Asset Get").
		Build()

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

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

	assert.Equal(t, http.StatusAccepted, w.Code)

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

	assert.Equal(t, http.StatusAccepted, w.Code)

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

	createRequest := testutil.NewAssetFactory(accountID).
		WithIdentifier("TEST-004").
		WithName("Test Asset Delete").
		Build()

	created, err := store.CreateAsset(context.Background(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, created)

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

	assert.Equal(t, http.StatusAccepted, w.Code)

	var response map[string]bool
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["deleted"])

	deleted, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
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

	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var response map[string][]asset.Asset
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(response["data"]), 3)
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
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]asset.Asset
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Workflow Test Asset", response["data"].Name)
		createdID = response["data"].ID
	})

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

		assert.Equal(t, http.StatusAccepted, w.Code)

		var response map[string]*asset.Asset
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

		assert.Equal(t, http.StatusAccepted, w.Code)

		var response map[string]*asset.Asset
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, newName, response["data"].Name)
	})

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

		assert.Equal(t, http.StatusAccepted, w.Code)

		var response map[string]bool
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["deleted"])
	})
}
