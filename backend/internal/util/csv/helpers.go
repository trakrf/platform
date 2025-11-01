package csv

import (
	"fmt"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/models/asset"
)

// Supported date formats for CSV import
const (
	DateFormatISO             = "2006-01-02" // YYYY-MM-DD
	DateFormatUSA             = "01/02/2006" // MM/DD/YYYY
	DateFormatEuropean        = "02-01-2006" // DD-MM-YYYY
	DateFormatEuropeanSlashes = "02/01/2006" // DD/MM/YYYY
)

// ParseCSVDate converts a date string to time.Time, supporting multiple formats:
// - YYYY-MM-DD (ISO 8601)
// - MM/DD/YYYY (US format)
// - DD-MM-YYYY (European format with hyphens)
// - DD/MM/YYYY (European format with slashes)
//
// Returns detailed error with format suggestions if parsing fails.
func ParseCSVDate(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)

	if dateStr == "" {
		return time.Time{}, fmt.Errorf("date cannot be empty")
	}

	// Try each supported format
	formats := []struct {
		layout string
		name   string
	}{
		{DateFormatISO, "YYYY-MM-DD"},
		{DateFormatUSA, "MM/DD/YYYY"},
		{DateFormatEuropeanSlashes, "DD/MM/YYYY"},
		{DateFormatEuropean, "DD-MM-YYYY"},
	}

	var parseErrs []string
	for _, f := range formats {
		t, err := time.Parse(f.layout, dateStr)
		if err == nil {
			return t, nil
		}
		parseErrs = append(parseErrs, f.name)
	}

	// Build detailed error message with suggestions
	return time.Time{}, fmt.Errorf(
		"invalid date format '%s': could not parse as %s. Expected formats: YYYY-MM-DD, MM/DD/YYYY, DD/MM/YYYY, or DD-MM-YYYY",
		dateStr,
		strings.Join(parseErrs, ", "),
	)
}

// ParseCSVBool converts a boolean string to bool, supporting multiple representations:
// - true/false (case-insensitive)
// - 1/0
// - yes/no (case-insensitive)
//
// Whitespace is trimmed before parsing.
// Returns detailed error with suggestions if parsing fails.
func ParseCSVBool(boolStr string) (bool, error) {
	boolStr = strings.TrimSpace(strings.ToLower(boolStr))

	if boolStr == "" {
		return false, fmt.Errorf("boolean value cannot be empty")
	}

	switch boolStr {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf(
			"invalid boolean value '%s': expected 'true', 'false', '1', '0', 'yes', or 'no' (case-insensitive)",
			boolStr,
		)
	}
}

// Required CSV columns for asset bulk import
var requiredCSVHeaders = []string{
	"identifier",
	"name",
	"type",
	"valid_from",
	"valid_to",
	"is_active",
}

// ValidateCSVHeaders checks if all required columns are present in the CSV header row.
// Column order is flexible - all required columns must be present but can be in any order.
// Extra columns are allowed and will be ignored.
// Matching is case-insensitive.
//
// Returns detailed error listing missing columns if validation fails.
func ValidateCSVHeaders(headers []string) error {
	if len(headers) == 0 {
		return fmt.Errorf("CSV headers cannot be empty")
	}

	// Normalize headers to lowercase for case-insensitive matching
	normalizedHeaders := make(map[string]bool)
	for _, h := range headers {
		normalizedHeaders[strings.ToLower(strings.TrimSpace(h))] = true
	}

	// Check for missing required columns
	var missing []string
	for _, required := range requiredCSVHeaders {
		if !normalizedHeaders[required] {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"CSV is missing required columns: %s. Required columns are: %s (order doesn't matter, case-insensitive)",
			strings.Join(missing, ", "),
			strings.Join(requiredCSVHeaders, ", "),
		)
	}

	return nil
}

func MapCSVRowToAsset(row []string, headers []string, orgID int) (*asset.Asset, error) {
	headerIdx := make(map[string]int)
	for i, h := range headers {
		headerIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	getCol := func(name string) (string, error) {
		idx, ok := headerIdx[name]
		if !ok {
			return "", fmt.Errorf("missing required column: %s", name)
		}
		if idx >= len(row) {
			return "", fmt.Errorf("row too short for column: %s", name)
		}
		return strings.TrimSpace(row[idx]), nil
	}

	identifier, err := getCol("identifier")
	if err != nil {
		return nil, err
	}
	if identifier == "" {
		return nil, fmt.Errorf("identifier cannot be empty")
	}

	name, err := getCol("name")
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}

	assetType, err := getCol("type")
	if err != nil {
		return nil, err
	}
	if assetType == "" {
		return nil, fmt.Errorf("type cannot be empty")
	}

	validFromStr, err := getCol("valid_from")
	if err != nil {
		return nil, err
	}

	validToStr, err := getCol("valid_to")
	if err != nil {
		return nil, err
	}

	isActiveStr, err := getCol("is_active")
	if err != nil {
		return nil, err
	}

	validFrom, err := ParseCSVDate(validFromStr)
	if err != nil {
		return nil, fmt.Errorf("invalid valid_from: %w", err)
	}

	validTo, err := ParseCSVDate(validToStr)
	if err != nil {
		return nil, fmt.Errorf("invalid valid_to: %w", err)
	}

	isActive, err := ParseCSVBool(isActiveStr)
	if err != nil {
		return nil, fmt.Errorf("invalid is_active: %w", err)
	}

	description := ""
	if descIdx, ok := headerIdx["description"]; ok && descIdx < len(row) {
		description = strings.TrimSpace(row[descIdx])
	}

	if validTo.Before(validFrom) {
		return nil, fmt.Errorf("valid_to must be after valid_from")
	}

	return &asset.Asset{
		OrgID:       orgID,
		Identifier:  identifier,
		Name:        name,
		Type:        assetType,
		Description: description,
		ValidFrom:   validFrom,
		ValidTo:     &validTo,
		IsActive:    isActive,
	}, nil
}
