package location

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlexibleDate_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		checkFunc   func(t *testing.T, fd FlexibleDate)
	}{
		{
			name:        "RFC3339 format",
			input:       `"2025-12-14T10:30:00Z"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
			},
		},
		{
			name:        "ISO 8601 date only",
			input:       `"2025-12-14"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
			},
		},
		{
			name:        "US format MM/DD/YYYY",
			input:       `"12/14/2025"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
			},
		},
		{
			name:        "European format DD.MM.YYYY",
			input:       `"14.12.2025"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
			},
		},
		{
			name:        "UK format DD/MM/YYYY",
			input:       `"14/12/2025"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
			},
		},
		{
			name:        "ISO with slashes YYYY/MM/DD",
			input:       `"2025/12/14"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
			},
		},
		{
			name:        "ISO with time",
			input:       `"2025-12-14 10:30:00"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
				assert.Equal(t, 10, fd.Hour())
				assert.Equal(t, 30, fd.Minute())
			},
		},
		{
			name:        "null value",
			input:       `null`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.True(t, fd.IsZero())
			},
		},
		{
			name:        "empty string",
			input:       `""`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.True(t, fd.IsZero())
			},
		},
		{
			name:        "invalid format",
			input:       `"not-a-date"`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fd FlexibleDate
			err := json.Unmarshal([]byte(tt.input), &fd)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkFunc != nil {
					tt.checkFunc(t, fd)
				}
			}
		})
	}
}

func TestFlexibleDate_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		date     FlexibleDate
		expected string
	}{
		{
			name:     "valid date",
			date:     FlexibleDate{Time: time.Date(2025, 12, 14, 10, 30, 0, 0, time.UTC)},
			expected: `"2025-12-14T10:30:00Z"`,
		},
		{
			name:     "zero date",
			date:     FlexibleDate{},
			expected: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := json.Marshal(tt.date)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestFlexibleDate_InCreateLocationRequest(t *testing.T) {
	jsonData := `{
		"name": "Warehouse 1",
		"identifier": "wh1",
		"description": "Main warehouse",
		"valid_from": "12/14/2025",
		"is_active": true
	}`

	var req CreateLocationRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	assert.Equal(t, "Warehouse 1", req.Name)
	assert.Equal(t, "wh1", req.Identifier)
	assert.Equal(t, 2025, req.ValidFrom.Year())
	assert.Equal(t, time.Month(12), req.ValidFrom.Month())
	assert.Equal(t, 14, req.ValidFrom.Day())
}

func TestFlexibleDate_InCreateLocationRequest_EuropeanFormat(t *testing.T) {
	jsonData := `{
		"name": "Warehouse 1",
		"identifier": "wh1",
		"description": "Main warehouse",
		"valid_from": "14.12.2025",
		"is_active": true
	}`

	var req CreateLocationRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	assert.Equal(t, "Warehouse 1", req.Name)
	assert.Equal(t, "wh1", req.Identifier)
	assert.Equal(t, 2025, req.ValidFrom.Year())
	assert.Equal(t, time.Month(12), req.ValidFrom.Month())
	assert.Equal(t, 14, req.ValidFrom.Day())
}
