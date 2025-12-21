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

// TestFlexibleDatePointer_EmptyString verifies that when a *FlexibleDate field
// receives an empty string in JSON, the pointer is non-nil but IsZero() returns true.
// This is important because our handlers check both conditions to avoid inserting
// zero dates (0001-01-01) into the database.
func TestFlexibleDatePointer_EmptyString(t *testing.T) {
	type TestStruct struct {
		ValidTo *FlexibleDate `json:"valid_to,omitempty"`
	}

	tests := []struct {
		name       string
		input      string
		expectNil  bool
		expectZero bool
	}{
		{
			name:       "omitted field results in nil pointer",
			input:      `{}`,
			expectNil:  true,
			expectZero: false, // can't check IsZero on nil
		},
		{
			name:       "null value results in nil pointer",
			input:      `{"valid_to": null}`,
			expectNil:  true,
			expectZero: false,
		},
		{
			name:       "empty string results in non-nil pointer with zero time",
			input:      `{"valid_to": ""}`,
			expectNil:  false,
			expectZero: true,
		},
		{
			name:       "valid date results in non-nil pointer with non-zero time",
			input:      `{"valid_to": "2025-12-14"}`,
			expectNil:  false,
			expectZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ts TestStruct
			err := json.Unmarshal([]byte(tt.input), &ts)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, ts.ValidTo, "expected ValidTo to be nil")
			} else {
				require.NotNil(t, ts.ValidTo, "expected ValidTo to be non-nil")
				if tt.expectZero {
					assert.True(t, ts.ValidTo.IsZero(), "expected ValidTo.IsZero() to be true")
				} else {
					assert.False(t, ts.ValidTo.IsZero(), "expected ValidTo.IsZero() to be false")
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
