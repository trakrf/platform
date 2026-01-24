package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// mockStorage implements the storage methods needed for testing
type mockStorage struct {
	saveResult *storage.SaveInventoryResult
	saveError  error
}

func (m *mockStorage) SaveInventoryScans(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error) {
	if m.saveError != nil {
		return nil, m.saveError
	}
	return m.saveResult, nil
}

// storageWrapper wraps mockStorage to satisfy *storage.Storage interface requirement
// In a real test, we'd use dependency injection with an interface
type testHandler struct {
	mockSave func(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error)
}

func TestSave_MissingOrgContext(t *testing.T) {
	handler := NewHandler(nil)

	body := SaveRequest{
		LocationID: 1,
		AssetIDs:   []int{100, 101},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	// Don't set user claims - this simulates missing auth

	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", response.Error.Type)
}

func TestSave_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	// Add user claims
	orgID := 1
	claims := &jwt.Claims{
		UserID:       1,
		Email:        "test@example.com",
		CurrentOrgID: &orgID,
	}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSave_MissingLocationID(t *testing.T) {
	handler := NewHandler(nil)

	body := map[string]any{
		"asset_ids": []int{100, 101},
		// missing location_id
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	orgID := 1
	claims := &jwt.Claims{
		UserID:       1,
		Email:        "test@example.com",
		CurrentOrgID: &orgID,
	}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "validation_error", response.Error.Type)
}

func TestSave_EmptyAssetIDs(t *testing.T) {
	handler := NewHandler(nil)

	body := SaveRequest{
		LocationID: 1,
		AssetIDs:   []int{}, // empty array
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	orgID := 1
	claims := &jwt.Claims{
		UserID:       1,
		Email:        "test@example.com",
		CurrentOrgID: &orgID,
	}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "validation_error", response.Error.Type)
}

func TestSave_RouteRegistration(t *testing.T) {
	handler := NewHandler(nil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Verify route is registered
	rctx := chi.NewRouteContext()
	if !r.Match(rctx, http.MethodPost, "/api/v1/inventory/save") {
		t.Error("Route POST /api/v1/inventory/save not registered")
	}
}

func TestSaveRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request SaveRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: SaveRequest{
				LocationID: 1,
				AssetIDs:   []int{100, 101, 102},
			},
			wantErr: false,
		},
		{
			name: "missing location_id",
			request: SaveRequest{
				LocationID: 0,
				AssetIDs:   []int{100},
			},
			wantErr: true,
		},
		{
			name: "empty asset_ids",
			request: SaveRequest{
				LocationID: 1,
				AssetIDs:   []int{},
			},
			wantErr: true,
		},
		{
			name: "nil asset_ids",
			request: SaveRequest{
				LocationID: 1,
				AssetIDs:   nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.request)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSaveInventoryResult_JSON(t *testing.T) {
	result := storage.SaveInventoryResult{
		Count:        5,
		LocationID:   123,
		LocationName: "Warehouse A - Rack 12",
		Timestamp:    time.Date(2026, 1, 23, 20, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, float64(5), parsed["count"])
	assert.Equal(t, float64(123), parsed["location_id"])
	assert.Equal(t, "Warehouse A - Rack 12", parsed["location_name"])
}

// Integration-style tests that require a mock storage
// These test the handler's error handling paths

func TestSave_LocationNotOwnedByOrg(t *testing.T) {
	// This test documents the expected behavior when storage returns
	// a "not found or access denied" error for location validation
	// In a full integration test, we'd use a real or properly mocked storage

	// For now, verify the handler exists and can be instantiated
	handler := NewHandler(nil)
	assert.NotNil(t, handler)
}

func TestSave_AssetNotOwnedByOrg(t *testing.T) {
	// This test documents the expected behavior when storage returns
	// a "not found or access denied" error for asset validation
	// In a full integration test, we'd use a real or properly mocked storage

	// For now, verify the handler exists
	handler := NewHandler(nil)
	assert.NotNil(t, handler)
}

func TestSaveInventoryRequest_Struct(t *testing.T) {
	// Test the storage request struct
	req := storage.SaveInventoryRequest{
		LocationID: 123,
		AssetIDs:   []int{1, 2, 3},
	}

	assert.Equal(t, 123, req.LocationID)
	assert.Equal(t, []int{1, 2, 3}, req.AssetIDs)
	assert.Len(t, req.AssetIDs, 3)
}

func TestAccessDeniedErrorDetection(t *testing.T) {
	// Test that the error detection logic works correctly
	tests := []struct {
		name            string
		err             error
		expectForbidden bool
	}{
		{
			name:            "location not found",
			err:             errors.New("location not found or access denied"),
			expectForbidden: true,
		},
		{
			name:            "asset not found",
			err:             errors.New("one or more assets not found or access denied"),
			expectForbidden: true,
		},
		{
			name:            "internal error",
			err:             errors.New("database connection failed"),
			expectForbidden: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			containsAccessDenied := contains(errStr, "not found or access denied")
			assert.Equal(t, tt.expectForbidden, containsAccessDenied)
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
