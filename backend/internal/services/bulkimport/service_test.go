package bulkimport

import "testing"

func TestIsEmptyRow(t *testing.T) {
	tests := []struct {
		name     string
		row      []string
		expected bool
	}{
		{
			name:     "completely empty row",
			row:      []string{},
			expected: true,
		},
		{
			name:     "row with empty strings",
			row:      []string{"", "", ""},
			expected: true,
		},
		{
			name:     "row with whitespace only",
			row:      []string{"   ", "\t", "  \t  "},
			expected: true,
		},
		{
			name:     "row with newlines only",
			row:      []string{"\n", "\r\n", ""},
			expected: true,
		},
		{
			name:     "row with one non-empty field",
			row:      []string{"", "value", ""},
			expected: false,
		},
		{
			name:     "row with all non-empty fields",
			row:      []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "row with value surrounded by empty",
			row:      []string{"", "", "data", "", ""},
			expected: false,
		},
		{
			name:     "row with whitespace and value",
			row:      []string{"  ", "value", "  "},
			expected: false,
		},
		{
			name:     "typical CSV empty row (commas only)",
			row:      []string{"", "", "", "", "", "", ""},
			expected: true,
		},
		{
			name:     "single empty field",
			row:      []string{""},
			expected: true,
		},
		{
			name:     "single whitespace field",
			row:      []string{"   "},
			expected: true,
		},
		{
			name:     "single non-empty field",
			row:      []string{"value"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmptyRow(tt.row)
			if result != tt.expected {
				t.Errorf("isEmptyRow(%v) = %v, expected %v", tt.row, result, tt.expected)
			}
		})
	}
}
