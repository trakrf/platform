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
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// mockInventoryStorage implements InventoryStorage for testing.
type mockInventoryStorage struct {
	saveResult *storage.SaveInventoryResult
	saveError  error

	// Identifier resolution stubs.
	locationByIdentifier      map[string]*location.LocationWithParent
	locationByIdentifierError error

	assetIDsByIdentifiers      map[string]int
	assetIDsByIdentifiersError error
}

func (m *mockInventoryStorage) SaveInventoryScans(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error) {
	return m.saveResult, m.saveError
}

func (m *mockInventoryStorage) GetLocationByIdentifier(ctx context.Context, orgID int, identifier string) (*location.LocationWithParent, error) {
	if m.locationByIdentifierError != nil {
		return nil, m.locationByIdentifierError
	}
	return m.locationByIdentifier[identifier], nil
}

func (m *mockInventoryStorage) GetAssetIDsByIdentifiers(ctx context.Context, orgID int, identifiers []string) (map[string]int, error) {
	if m.assetIDsByIdentifiersError != nil {
		return nil, m.assetIDsByIdentifiersError
	}
	out := make(map[string]int, len(identifiers))
	for _, id := range identifiers {
		if v, ok := m.assetIDsByIdentifiers[id]; ok {
			out[id] = v
		}
	}
	return out, nil
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

func TestSave_NeitherLocationFieldProvided(t *testing.T) {
	handler := NewHandler(&mockInventoryStorage{})

	body := map[string]any{
		"asset_ids": []int{100, 101},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var response struct {
		Error struct {
			Type   string `json:"type"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "bad_request", response.Error.Type)
	assert.Contains(t, response.Error.Detail, "location_identifier")
}

func TestSave_EmptyAssetIDs(t *testing.T) {
	// JSON-marshalling SaveRequest with omitempty drops the empty slice, so the
	// decoded request has nil AssetIDs. The cross-field check fires before the
	// struct validator can reject the empty slice, returning bad_request.
	handler := NewHandler(&mockInventoryStorage{})

	body := SaveRequest{
		LocationID: 1,
		AssetIDs:   []int{}, // empty array — omitted by omitempty, arrives as nil
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
	assert.Equal(t, "bad_request", response.Error.Type)
}

func TestSave_RouteRegistration(t *testing.T) {
	// POST /api/v1/inventory/save is now wired in cmd/serve/router.go under the
	// public-write group (TRA-397); Handler.RegisterRoutes is intentionally empty.
	// Wire the route directly here to verify handler-level plumbing.
	handler := NewHandler(nil)

	r := chi.NewRouter()
	r.Post("/api/v1/inventory/save", handler.Save)

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
			name: "missing location_id passes struct validator (handler enforces)",
			request: SaveRequest{
				LocationID: 0,
				AssetIDs:   []int{100},
			},
			wantErr: false,
		},
		{
			name: "empty asset_ids",
			request: SaveRequest{
				LocationID: 1,
				AssetIDs:   []int{},
			},
			wantErr: true, // direct struct validation catches empty non-nil slice via min=1
		},
		{
			name: "nil asset_ids passes struct validator (handler enforces)",
			request: SaveRequest{
				LocationID: 1,
				AssetIDs:   nil,
			},
			wantErr: false,
		},
		{
			name: "identifier-only request validates",
			request: SaveRequest{
				LocationIdentifier: ptr("WH-01"),
				AssetIdentifiers:   []string{"ASSET-0001"},
			},
			wantErr: false,
		},
		{
			name:    "all-empty passes struct validator; cross-field lives in handler",
			request: SaveRequest{},
			wantErr: false,
		},
		{
			name: "asset_identifiers with empty string element fails",
			request: SaveRequest{
				LocationIdentifier: ptr("WH-01"),
				AssetIdentifiers:   []string{""},
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

// Bug reproduction: TRA-407 item 5 — malformed body must not leak encoding/json internals.
func TestInventorySave_MalformedBody_StableDetail(t *testing.T) {
	orgID := 1
	claims := &jwt.Claims{
		UserID:       1,
		Email:        "test@example.com",
		CurrentOrgID: &orgID,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	req = req.WithContext(ctx)

	handler := NewHandler(nil)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body struct {
		Error struct {
			Detail string `json:"detail"`
		} `json:"error"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "Request body is not valid JSON", body.Error.Detail,
		"detail must be the stable string, not a raw Go error")
	assert.NotContains(t, body.Error.Detail, "invalid character",
		"must not leak encoding/json internals")
	assert.NotContains(t, body.Error.Detail, "literal null",
		"must not leak encoding/json internals")
}

// Bug reproduction: TRA-407 item 2 — cross-field validation replaces old fields[] envelope.
func TestInventorySave_BadBody_CrossFieldEnvelope(t *testing.T) {
	orgID := 1
	claims := &jwt.Claims{UserID: 1, Email: "test@example.com", CurrentOrgID: &orgID}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	req = req.WithContext(ctx)

	handler := NewHandler(&mockInventoryStorage{})
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var body struct {
		Error struct {
			Type   string `json:"type"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "bad_request", body.Error.Type)
	assert.Contains(t, body.Error.Detail, "location_identifier")
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
	// Post-migration: RespondStorageError returns a stable message, not the raw error detail.
	assert.Contains(t, w.Body.String(), "An unexpected error occurred")
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

func TestSave_RequiresAtLeastOneLocationField(t *testing.T) {
	handler := NewHandler(&mockInventoryStorage{})
	req := newTestRequest(t, map[string]any{"asset_ids": []int{1}}, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "location_identifier")
}

func TestSave_RequiresAtLeastOneAssetField(t *testing.T) {
	handler := NewHandler(&mockInventoryStorage{})
	req := newTestRequest(t, map[string]any{"location_id": 1}, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "asset_identifiers")
}

func TestSave_BothAssetFieldsPresent_Rejected(t *testing.T) {
	handler := NewHandler(&mockInventoryStorage{})
	body := map[string]any{
		"location_id":       1,
		"asset_ids":         []int{1, 2},
		"asset_identifiers": []string{"ASSET-0001"},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not both")
}

func TestSave_LocationFieldsDisagree_Rejected(t *testing.T) {
	mock := &mockInventoryStorage{
		locationByIdentifier: map[string]*location.LocationWithParent{
			"WH-01": {LocationView: location.LocationView{Location: location.Location{ID: 42, Identifier: "WH-01"}}},
		},
	}
	handler := NewHandler(mock)
	body := map[string]any{
		"location_id":         99, // doesn't match resolved 42
		"location_identifier": "WH-01",
		"asset_ids":           []int{1},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "disagree")
}

func TestSave_LocationIdentifierNotFound_Rejected(t *testing.T) {
	mock := &mockInventoryStorage{
		locationByIdentifier: map[string]*location.LocationWithParent{}, // ghost
	}
	handler := NewHandler(mock)
	body := map[string]any{
		"location_identifier": "ghost",
		"asset_ids":           []int{1},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "ghost")
}

func TestSave_AssetIdentifierNotFound_Rejected(t *testing.T) {
	mock := &mockInventoryStorage{
		locationByIdentifier: map[string]*location.LocationWithParent{
			"WH-01": {LocationView: location.LocationView{Location: location.Location{ID: 42, Identifier: "WH-01"}}},
		},
		assetIDsByIdentifiers: map[string]int{
			"ASSET-1": 7,
			// "ASSET-GHOST" intentionally absent
		},
	}
	handler := NewHandler(mock)
	body := map[string]any{
		"location_identifier": "WH-01",
		"asset_identifiers":   []string{"ASSET-1", "ASSET-GHOST"},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "ASSET-GHOST")
}

func TestSave_IdentifierHappyPath_ResolvesAndSucceeds(t *testing.T) {
	ts := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	mock := &mockInventoryStorage{
		saveResult: &storage.SaveInventoryResult{
			Count:        2, LocationID: 42, LocationName: "WH-01", Timestamp: ts,
		},
		locationByIdentifier: map[string]*location.LocationWithParent{
			"WH-01": {LocationView: location.LocationView{Location: location.Location{ID: 42, Identifier: "WH-01", Name: "WH-01"}}},
		},
		assetIDsByIdentifiers: map[string]int{
			"ASSET-1": 7,
			"ASSET-2": 8,
		},
	}
	handler := NewHandler(mock)
	body := map[string]any{
		"location_identifier": "WH-01",
		"asset_identifiers":   []string{"ASSET-1", "ASSET-2"},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Data storage.SaveInventoryResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Data.Count)
	assert.Equal(t, 42, resp.Data.LocationID)
}

func TestSave_BothLocationFieldsAgree_Succeeds(t *testing.T) {
	ts := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	mock := &mockInventoryStorage{
		saveResult: &storage.SaveInventoryResult{
			Count: 1, LocationID: 42, LocationName: "WH-01", Timestamp: ts,
		},
		locationByIdentifier: map[string]*location.LocationWithParent{
			"WH-01": {LocationView: location.LocationView{Location: location.Location{ID: 42, Identifier: "WH-01", Name: "WH-01"}}},
		},
	}
	handler := NewHandler(mock)
	body := map[string]any{
		"location_id":         42, // matches resolved
		"location_identifier": "WH-01",
		"asset_ids":           []int{7},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func ptr[T any](v T) *T { return &v }
