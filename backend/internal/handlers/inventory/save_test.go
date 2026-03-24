package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// mockInventoryStorage implements InventoryStorage for testing.
type mockInventoryStorage struct {
	saveResult *storage.SaveInventoryResult
	saveError  error
}

func (m *mockInventoryStorage) SaveInventoryScans(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error) {
	return m.saveResult, m.saveError
}

// newTestRequest creates a POST request with JSON body and org claims set.
func newTestRequest(t *testing.T, body any, orgID int) *http.Request {
	t.Helper()
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	claims := &jwt.Claims{
		UserID:       1,
		Email:        "test@example.com",
		CurrentOrgID: &orgID,
	}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
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

func TestSaveInventoryRequest_Struct(t *testing.T) {
	req := storage.SaveInventoryRequest{
		LocationID: 123,
		AssetIDs:   []int{1, 2, 3},
	}

	assert.Equal(t, 123, req.LocationID)
	assert.Equal(t, []int{1, 2, 3}, req.AssetIDs)
	assert.Len(t, req.AssetIDs, 3)
}

func TestAccessDeniedErrorDetection(t *testing.T) {
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
			containsAccessDenied := strings.Contains(errStr, "not found or access denied")
			assert.Equal(t, tt.expectForbidden, containsAccessDenied)
		})
	}
}

func TestSave_AccessErrorDetection(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		expectForbidden bool
		expectOrgInMsg  bool
	}{
		{
			name: "typed location error includes org context",
			err: &storage.InventoryAccessError{
				Reason:     "location",
				OrgID:      123,
				LocationID: 456,
			},
			expectForbidden: true,
			expectOrgInMsg:  true,
		},
		{
			name: "typed asset error includes org context",
			err: &storage.InventoryAccessError{
				Reason:     "assets",
				OrgID:      123,
				AssetIDs:   []int{1, 2, 3},
				ValidCount: 2,
				TotalCount: 3,
			},
			expectForbidden: true,
			expectOrgInMsg:  true,
		},
		{
			name:            "internal error is not forbidden",
			err:             errors.New("database connection failed"),
			expectForbidden: false,
			expectOrgInMsg:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			isForbidden := strings.Contains(errStr, "not found or access denied")
			assert.Equal(t, tt.expectForbidden, isForbidden)
			if tt.expectOrgInMsg {
				assert.Contains(t, errStr, "org_id=123")
			}
		})
	}
}

// --- Handler-level tests using mockInventoryStorage ---

func TestSave_LocationAccessDenied(t *testing.T) {
	mock := &mockInventoryStorage{
		saveError: &storage.InventoryAccessError{
			Reason:     "location",
			OrgID:      1,
			LocationID: 999,
		},
	}
	handler := NewHandler(mock)

	req := newTestRequest(t, SaveRequest{LocationID: 999, AssetIDs: []int{100, 101}}, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "not found or access denied")
}

func TestSave_AssetAccessDenied(t *testing.T) {
	mock := &mockInventoryStorage{
		saveError: &storage.InventoryAccessError{
			Reason:     "assets",
			OrgID:      1,
			AssetIDs:   []int{1, 2, 3},
			ValidCount: 2,
			TotalCount: 3,
		},
	}
	handler := NewHandler(mock)

	req := newTestRequest(t, SaveRequest{LocationID: 1, AssetIDs: []int{1, 2, 3}}, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "not found or access denied")
}

func TestSave_InternalStorageError(t *testing.T) {
	mock := &mockInventoryStorage{
		saveError: errors.New("database connection failed"),
	}
	handler := NewHandler(mock)

	req := newTestRequest(t, SaveRequest{LocationID: 1, AssetIDs: []int{100}}, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "database connection failed")
}

func TestSave_Success(t *testing.T) {
	ts := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	mock := &mockInventoryStorage{
		saveResult: &storage.SaveInventoryResult{
			Count:        3,
			LocationID:   42,
			LocationName: "Warehouse B",
			Timestamp:    ts,
		},
	}
	handler := NewHandler(mock)

	req := newTestRequest(t, SaveRequest{LocationID: 42, AssetIDs: []int{1, 2, 3}}, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response struct {
		Data storage.SaveInventoryResult `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 3, response.Data.Count)
	assert.Equal(t, 42, response.Data.LocationID)
	assert.Equal(t, "Warehouse B", response.Data.LocationName)
	assert.Equal(t, ts, response.Data.Timestamp)
}
