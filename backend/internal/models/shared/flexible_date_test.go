package shared

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
			name:        "invalid format shows user-friendly error",
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
				assert.Contains(t, err.Error(), "Supported formats")
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
