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
			name:        "RFC3339 with offset",
			input:       `"2025-12-14T10:30:00-08:00"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
			},
		},
		{
			name:        "RFC3339 nano truncates to microseconds",
			input:       `"2025-12-14T10:30:00.123456789Z"`,
			expectError: false,
			checkFunc: func(t *testing.T, fd FlexibleDate) {
				assert.Equal(t, 2025, fd.Year())
				assert.Equal(t, time.Month(12), fd.Month())
				assert.Equal(t, 14, fd.Day())
				// Storage layer truncates to microseconds; the parser
				// itself preserves nanos. The truncation contract is
				// asserted at the storage seam.
			},
		},
		// TRA-649 / BB23 F2: loose forms that previously round-tripped
		// silently must reject as validation_error. The query-param
		// validator on /assets/{id}/history already enforces strict
		// RFC 3339; the body validator now matches.
		{
			name:        "ISO 8601 date only — rejected",
			input:       `"2025-12-14"`,
			expectError: true,
		},
		{
			name:        "ISO with space separator — rejected",
			input:       `"2025-12-14 10:30:00"`,
			expectError: true,
		},
		{
			name:        "US slashes — rejected",
			input:       `"12/14/2025"`,
			expectError: true,
		},
		{
			name:        "ISO slashes — rejected",
			input:       `"2025/12/14"`,
			expectError: true,
		},
		{
			name:        "European dots — rejected",
			input:       `"14.12.2025"`,
			expectError: true,
		},
		{
			name:        "empty string — rejected",
			input:       `""`,
			expectError: true,
		},
		{
			name:        "Go zero-time — rejected",
			input:       `"0001-01-01T00:00:00Z"`,
			expectError: true,
		},
		{
			name:        "null value still permitted",
			input:       `null`,
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
				require.Error(t, err)
				// TRA-641 / BB21 §2.1: format failures surface as
				// *json.UnmarshalTypeError so the decoder fills in the field
				// path and RespondDecodeError can route to validation_error.
				var typeErr *json.UnmarshalTypeError
				require.ErrorAs(t, err, &typeErr, "expected *json.UnmarshalTypeError")
				assert.Equal(t, "time.Time", typeErr.Type.String())
			} else {
				require.NoError(t, err)
				if tt.checkFunc != nil {
					tt.checkFunc(t, fd)
				}
			}
		})
	}
}

// TestFlexibleDatePointer_FieldShapes verifies pointer-field behavior across
// the JSON shapes a `*FlexibleDate` may receive on a request body. After
// TRA-649 the parser rejects empty string; null and absence still map to a
// nil pointer, which handlers interpret as "field not provided".
func TestFlexibleDatePointer_FieldShapes(t *testing.T) {
	type TestStruct struct {
		ValidTo *FlexibleDate `json:"valid_to,omitempty"`
	}

	t.Run("omitted field results in nil pointer", func(t *testing.T) {
		var ts TestStruct
		require.NoError(t, json.Unmarshal([]byte(`{}`), &ts))
		assert.Nil(t, ts.ValidTo)
	})

	t.Run("null value results in nil pointer", func(t *testing.T) {
		var ts TestStruct
		require.NoError(t, json.Unmarshal([]byte(`{"valid_to": null}`), &ts))
		assert.Nil(t, ts.ValidTo)
	})

	t.Run("empty string is rejected", func(t *testing.T) {
		var ts TestStruct
		err := json.Unmarshal([]byte(`{"valid_to": ""}`), &ts)
		require.Error(t, err)
		var typeErr *json.UnmarshalTypeError
		require.ErrorAs(t, err, &typeErr, "expected *json.UnmarshalTypeError")
	})

	t.Run("valid date results in non-nil pointer with non-zero time", func(t *testing.T) {
		var ts TestStruct
		require.NoError(t, json.Unmarshal([]byte(`{"valid_to": "2025-12-14T00:00:00Z"}`), &ts))
		require.NotNil(t, ts.ValidTo)
		assert.False(t, ts.ValidTo.IsZero())
	})
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
