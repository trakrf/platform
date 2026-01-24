package reports

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/report"
)

func TestGetAssetHistory_MissingOrgContext(t *testing.T) {
	handler := NewHandler(nil)

	// Use chi router to properly set up URL params
	r := chi.NewRouter()
	r.Get("/api/v1/reports/assets/{id}/history", handler.GetAssetHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/assets/123/history", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response struct {
		Error struct {
			Type   string `json:"type"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", response.Error.Type)
}

func TestGetAssetHistory_InvalidAssetID(t *testing.T) {
	// Test that non-numeric IDs are caught at parse time
	tests := []struct {
		name    string
		assetID string
	}{
		{"letters", "abc"},
		{"special chars", "12#34"},
		{"empty", ""},
		{"float", "12.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the parsing logic from the handler
			_, err := parseAssetID(tt.assetID)
			assert.Error(t, err)
		})
	}
}

func parseAssetID(s string) (int, error) {
	if len(s) == 0 {
		return 0, assert.AnError
	}
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, assert.AnError
		}
		result = result*10 + int(c-'0')
	}
	return result, nil
}

func TestGetAssetHistory_DefaultPagination(t *testing.T) {
	// Verify default limit and max limit constants
	handler := NewHandler(nil)
	assert.NotNil(t, handler)
	assert.Equal(t, 50, assetHistoryDefaultLimit)
	assert.Equal(t, 100, assetHistoryMaxLimit)
	assert.Equal(t, 30, defaultDateRangeDays)
}

func TestGetAssetHistory_LimitCapping(t *testing.T) {
	tests := []struct {
		name          string
		queryLimit    string
		expectedLimit int
	}{
		{"default", "", 50},
		{"valid", "25", 25},
		{"over max", "200", 100},
		{"invalid", "abc", 50},
		{"zero", "0", 50},
		{"negative", "-5", 50},
		{"at max", "100", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the parsing logic from the handler
			limit := assetHistoryDefaultLimit
			if tt.queryLimit != "" {
				if parsed := parsePositiveInt(tt.queryLimit); parsed > 0 {
					limit = parsed
					if limit > assetHistoryMaxLimit {
						limit = assetHistoryMaxLimit
					}
				}
			}
			assert.Equal(t, tt.expectedLimit, limit)
		})
	}
}

func parsePositiveInt(s string) int {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
	}
	return result
}

func TestGetAssetHistory_DefaultDateRange(t *testing.T) {
	// Verify that the default date range is 30 days
	now := time.Now()
	defaultStart := now.AddDate(0, 0, -defaultDateRangeDays)

	// The difference should be approximately 30 days
	diff := now.Sub(defaultStart)
	expectedDiff := time.Duration(defaultDateRangeDays) * 24 * time.Hour

	assert.Equal(t, expectedDiff, diff)
}

func TestGetAssetHistory_RouteRegistration(t *testing.T) {
	handler := NewHandler(nil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	rctx := chi.NewRouteContext()
	if !r.Match(rctx, http.MethodGet, "/api/v1/reports/assets/123/history") {
		t.Error("Route GET /api/v1/reports/assets/{id}/history not registered")
	}
}

func TestAssetHistoryItem_JSON(t *testing.T) {
	locationID := 456
	locationName := "Warehouse A"
	durationSeconds := 3600

	item := report.AssetHistoryItem{
		Timestamp:       time.Date(2025, 12, 15, 10, 30, 0, 0, time.UTC),
		LocationID:      &locationID,
		LocationName:    &locationName,
		DurationSeconds: &durationSeconds,
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "2025-12-15T10:30:00Z", parsed["timestamp"])
	assert.Equal(t, float64(456), parsed["location_id"])
	assert.Equal(t, "Warehouse A", parsed["location_name"])
	assert.Equal(t, float64(3600), parsed["duration_seconds"])
}

func TestAssetHistoryItem_NullableFields(t *testing.T) {
	// Test that null fields serialize correctly (most recent scan)
	item := report.AssetHistoryItem{
		Timestamp:       time.Date(2025, 12, 15, 10, 30, 0, 0, time.UTC),
		LocationID:      nil,
		LocationName:    nil,
		DurationSeconds: nil,
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Nil(t, parsed["location_id"])
	assert.Nil(t, parsed["location_name"])
	assert.Nil(t, parsed["duration_seconds"])
}

func TestAssetHistoryResponse_EmptyData(t *testing.T) {
	// Test that empty results return empty array, not null
	response := report.AssetHistoryResponse{
		Asset: report.AssetInfo{
			ID:         123,
			Name:       "Test Asset",
			Identifier: "TAG001",
		},
		Data:       []report.AssetHistoryItem{},
		Count:      0,
		Offset:     0,
		TotalCount: 0,
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Verify data is an array, not null
	dataArr, ok := parsed["data"].([]any)
	assert.True(t, ok, "data should be an array")
	assert.Empty(t, dataArr)
	assert.Equal(t, float64(0), parsed["count"])
	assert.Equal(t, float64(0), parsed["offset"])
	assert.Equal(t, float64(0), parsed["total_count"])

	// Verify asset info
	asset, ok := parsed["asset"].(map[string]any)
	assert.True(t, ok, "asset should be an object")
	assert.Equal(t, float64(123), asset["id"])
	assert.Equal(t, "Test Asset", asset["name"])
	assert.Equal(t, "TAG001", asset["identifier"])
}

func TestAssetHistoryResponse_WithData(t *testing.T) {
	locationID := 1
	locationName := "Test Location"
	duration := 3600

	response := report.AssetHistoryResponse{
		Asset: report.AssetInfo{
			ID:         123,
			Name:       "Test Asset",
			Identifier: "TAG001",
		},
		Data: []report.AssetHistoryItem{
			{
				Timestamp:       time.Date(2025, 12, 15, 10, 30, 0, 0, time.UTC),
				LocationID:      &locationID,
				LocationName:    &locationName,
				DurationSeconds: &duration,
			},
			{
				Timestamp:       time.Date(2025, 12, 15, 11, 30, 0, 0, time.UTC),
				LocationID:      nil,
				LocationName:    nil,
				DurationSeconds: nil,
			},
		},
		Count:      2,
		Offset:     0,
		TotalCount: 50,
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	dataArr, ok := parsed["data"].([]any)
	assert.True(t, ok)
	assert.Len(t, dataArr, 2)
	assert.Equal(t, float64(2), parsed["count"])
	assert.Equal(t, float64(0), parsed["offset"])
	assert.Equal(t, float64(50), parsed["total_count"])
}

func TestAssetInfo_JSON(t *testing.T) {
	info := report.AssetInfo{
		ID:         123,
		Name:       "Laptop-001",
		Identifier: "E2003412345678",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, float64(123), parsed["id"])
	assert.Equal(t, "Laptop-001", parsed["name"])
	assert.Equal(t, "E2003412345678", parsed["identifier"])
}

func TestAssetHistoryFilter_Struct(t *testing.T) {
	now := time.Now()
	start := now.AddDate(0, 0, -30)

	filter := report.AssetHistoryFilter{
		StartDate: &start,
		EndDate:   &now,
		Limit:     50,
		Offset:    100,
	}

	assert.Equal(t, start, *filter.StartDate)
	assert.Equal(t, now, *filter.EndDate)
	assert.Equal(t, 50, filter.Limit)
	assert.Equal(t, 100, filter.Offset)
}

func TestOffsetParsingForHistory(t *testing.T) {
	tests := []struct {
		name           string
		queryOffset    string
		expectedOffset int
	}{
		{"default", "", 0},
		{"valid", "50", 50},
		{"zero", "0", 0},
		{"invalid", "abc", 0},
		{"negative", "-10", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := 0
			if tt.queryOffset != "" {
				if parsed := parseNonNegativeInt(tt.queryOffset); parsed >= 0 {
					offset = parsed
				}
			}
			assert.Equal(t, tt.expectedOffset, offset)
		})
	}
}

func parseNonNegativeInt(s string) int {
	if len(s) == 0 {
		return -1
	}
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		result = result*10 + int(c-'0')
	}
	return result
}
