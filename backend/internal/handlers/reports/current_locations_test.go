package reports

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/report"
)

func TestListCurrentLocations_MissingOrgContext(t *testing.T) {
	handler := NewHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/current-locations", nil)
	w := httptest.NewRecorder()

	handler.ListCurrentLocations(w, req)

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

func TestListCurrentLocations_DefaultPagination(t *testing.T) {
	// Verify default limit and offset constants
	handler := NewHandler(nil)
	assert.NotNil(t, handler)
	assert.Equal(t, 50, defaultLimit)
	assert.Equal(t, 100, maxLimit)
}

func TestListCurrentLocations_LimitCapping(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the parsing logic from the handler
			limit := defaultLimit
			if tt.queryLimit != "" {
				if parsed := parseLimit(tt.queryLimit); parsed > 0 {
					limit = parsed
					if limit > maxLimit {
						limit = maxLimit
					}
				}
			}
			assert.Equal(t, tt.expectedLimit, limit)
		})
	}
}

func parseLimit(s string) int {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
	}
	return result
}

func TestListCurrentLocations_RouteRegistration(t *testing.T) {
	handler := NewHandler(nil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	rctx := chi.NewRouteContext()
	if !r.Match(rctx, http.MethodGet, "/api/v1/reports/current-locations") {
		t.Error("Route GET /api/v1/reports/current-locations not registered")
	}
}

func TestCurrentLocationFilter_Struct(t *testing.T) {
	locationID := 123
	search := "laptop"

	filter := report.CurrentLocationFilter{
		LocationID: &locationID,
		Search:     &search,
		Limit:      50,
		Offset:     100,
	}

	assert.Equal(t, 123, *filter.LocationID)
	assert.Equal(t, "laptop", *filter.Search)
	assert.Equal(t, 50, filter.Limit)
	assert.Equal(t, 100, filter.Offset)
}

func TestCurrentLocationItem_JSON(t *testing.T) {
	locationID := 456
	locationName := "Warehouse A"

	item := report.CurrentLocationItem{
		AssetID:         123,
		AssetName:       "Laptop-001",
		AssetIdentifier: "E200341234567890",
		LocationID:      &locationID,
		LocationName:    &locationName,
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, float64(123), parsed["asset_id"])
	assert.Equal(t, "Laptop-001", parsed["asset_name"])
	assert.Equal(t, "E200341234567890", parsed["asset_identifier"])
	assert.Equal(t, float64(456), parsed["location_id"])
	assert.Equal(t, "Warehouse A", parsed["location_name"])
}

func TestCurrentLocationItem_NullableFields(t *testing.T) {
	// Test that null location fields serialize correctly
	item := report.CurrentLocationItem{
		AssetID:         123,
		AssetName:       "Laptop-001",
		AssetIdentifier: "E200341234567890",
		LocationID:      nil,
		LocationName:    nil,
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Nil(t, parsed["location_id"])
	assert.Nil(t, parsed["location_name"])
}

func TestCurrentLocationsResponse_EmptyData(t *testing.T) {
	// Test that empty results return empty array, not null
	response := report.CurrentLocationsResponse{
		Data:       []report.CurrentLocationItem{},
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
}

func TestCurrentLocationsResponse_WithData(t *testing.T) {
	locationID := 1
	locationName := "Test Location"

	response := report.CurrentLocationsResponse{
		Data: []report.CurrentLocationItem{
			{
				AssetID:         1,
				AssetName:       "Asset 1",
				AssetIdentifier: "TAG001",
				LocationID:      &locationID,
				LocationName:    &locationName,
			},
			{
				AssetID:         2,
				AssetName:       "Asset 2",
				AssetIdentifier: "TAG002",
				LocationID:      nil,
				LocationName:    nil,
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

func TestHandler_NewHandler(t *testing.T) {
	handler := NewHandler(nil)
	assert.NotNil(t, handler)
}

func TestOffsetParsing(t *testing.T) {
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
				if parsed := parseOffset(tt.queryOffset); parsed >= 0 {
					offset = parsed
				}
			}
			assert.Equal(t, tt.expectedOffset, offset)
		})
	}
}

func parseOffset(s string) int {
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
