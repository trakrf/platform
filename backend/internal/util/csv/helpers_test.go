package csv

import (
	"strings"
	"testing"
	"time"
)

func TestParseCSVDate_ValidFormats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Format: "2006-01-02"
	}{
		{
			name:     "ISO 8601 format (YYYY-MM-DD)",
			input:    "2024-01-15",
			expected: "2024-01-15",
		},
		{
			name:     "US format (MM/DD/YYYY)",
			input:    "01/15/2024",
			expected: "2024-01-15",
		},
		{
			name:     "European format (DD-MM-YYYY)",
			input:    "15-01-2024",
			expected: "2024-01-15",
		},
		{
			name:     "ISO format with leading spaces",
			input:    "  2024-01-15",
			expected: "2024-01-15",
		},
		{
			name:     "ISO format with trailing spaces",
			input:    "2024-01-15  ",
			expected: "2024-01-15",
		},
		{
			name:     "ISO format with surrounding spaces",
			input:    "  2024-01-15  ",
			expected: "2024-01-15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCSVDate(tt.input)
			if err != nil {
				t.Errorf("ParseCSVDate(%q) returned error: %v", tt.input, err)
				return
			}

			expected, _ := time.Parse("2006-01-02", tt.expected)
			if !result.Equal(expected) {
				t.Errorf("ParseCSVDate(%q) = %v, expected %v", tt.input, result, expected)
			}
		})
	}
}

func TestParseCSVDate_InvalidFormats(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		errorContains string // Error message should contain this substring
	}{
		{
			name:          "empty string",
			input:         "",
			errorContains: "date cannot be empty",
		},
		{
			name:          "invalid format",
			input:         "2024/01/15", // Slashes with YYYY first (not supported)
			errorContains: "invalid date format",
		},
		{
			name:          "invalid month",
			input:         "2024-13-15",
			errorContains: "invalid date format",
		},
		{
			name:          "invalid day",
			input:         "2024-01-32",
			errorContains: "invalid date format",
		},
		{
			name:          "plain text",
			input:         "January 15, 2024",
			errorContains: "invalid date format",
		},
		{
			name:          "partial date",
			input:         "2024-01",
			errorContains: "invalid date format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCSVDate(tt.input)
			if err == nil {
				t.Errorf("ParseCSVDate(%q) should have returned error", tt.input)
				return
			}

			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("ParseCSVDate(%q) error = %v, should contain %q", tt.input, err, tt.errorContains)
			}
		})
	}
}

func TestParseCSVBool_ValidValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// True values
		{"lowercase true", "true", true},
		{"uppercase TRUE", "TRUE", true},
		{"mixed case True", "True", true},
		{"numeric 1", "1", true},
		{"lowercase yes", "yes", true},
		{"uppercase YES", "YES", true},
		{"mixed case Yes", "Yes", true},
		{"true with spaces", "  true  ", true},
		{"1 with spaces", "  1  ", true},

		// False values
		{"lowercase false", "false", false},
		{"uppercase FALSE", "FALSE", false},
		{"mixed case False", "False", false},
		{"numeric 0", "0", false},
		{"lowercase no", "no", false},
		{"uppercase NO", "NO", false},
		{"mixed case No", "No", false},
		{"false with spaces", "  false  ", false},
		{"0 with spaces", "  0  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCSVBool(tt.input)
			if err != nil {
				t.Errorf("ParseCSVBool(%q) returned error: %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("ParseCSVBool(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseCSVBool_InvalidValues(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		errorContains string
	}{
		{
			name:          "empty string",
			input:         "",
			errorContains: "boolean value cannot be empty",
		},
		{
			name:          "invalid text",
			input:         "maybe",
			errorContains: "invalid boolean value",
		},
		{
			name:          "numeric 2",
			input:         "2",
			errorContains: "invalid boolean value",
		},
		{
			name:          "y (partial yes)",
			input:         "y",
			errorContains: "invalid boolean value",
		},
		{
			name:          "t (partial true)",
			input:         "t",
			errorContains: "invalid boolean value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCSVBool(tt.input)
			if err == nil {
				t.Errorf("ParseCSVBool(%q) should have returned error", tt.input)
				return
			}

			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("ParseCSVBool(%q) error = %v, should contain %q", tt.input, err, tt.errorContains)
			}
		})
	}
}

func TestValidateCSVHeaders_ValidHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
	}{
		{
			name:    "exact order",
			headers: []string{"identifier", "name", "type", "description", "valid_from", "valid_to", "is_active"},
		},
		{
			name:    "different order",
			headers: []string{"name", "identifier", "type", "valid_from", "valid_to", "is_active"},
		},
		{
			name:    "required only (no description)",
			headers: []string{"identifier", "name", "type", "valid_from", "valid_to", "is_active"},
		},
		{
			name:    "uppercase headers",
			headers: []string{"IDENTIFIER", "NAME", "TYPE", "VALID_FROM", "VALID_TO", "IS_ACTIVE"},
		},
		{
			name:    "mixed case headers",
			headers: []string{"Identifier", "Name", "Type", "Valid_From", "Valid_To", "Is_Active"},
		},
		{
			name:    "headers with spaces",
			headers: []string{"  identifier  ", "name", "type", "valid_from", "valid_to", "is_active"},
		},
		{
			name:    "extra columns (should be ignored)",
			headers: []string{"identifier", "name", "type", "valid_from", "valid_to", "is_active", "extra_column", "another_column"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCSVHeaders(tt.headers)
			if err != nil {
				t.Errorf("ValidateCSVHeaders(%v) returned error: %v", tt.headers, err)
			}
		})
	}
}

func TestValidateCSVHeaders_InvalidHeaders(t *testing.T) {
	tests := []struct {
		name          string
		headers       []string
		errorContains string
	}{
		{
			name:          "empty headers",
			headers:       []string{},
			errorContains: "CSV headers cannot be empty",
		},
		{
			name:          "missing identifier",
			headers:       []string{"name", "type", "valid_from", "valid_to", "is_active"},
			errorContains: "missing required columns: identifier",
		},
		{
			name:          "missing name",
			headers:       []string{"identifier", "type", "valid_from", "valid_to", "is_active"},
			errorContains: "missing required columns: name",
		},
		{
			name:          "missing multiple columns",
			headers:       []string{"identifier", "name"},
			errorContains: "missing required columns",
		},
		{
			name:          "completely wrong headers",
			headers:       []string{"id", "title", "category"},
			errorContains: "missing required columns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCSVHeaders(tt.headers)
			if err == nil {
				t.Errorf("ValidateCSVHeaders(%v) should have returned error", tt.headers)
				return
			}

			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("ValidateCSVHeaders(%v) error = %v, should contain %q", tt.headers, err, tt.errorContains)
			}
		})
	}
}
