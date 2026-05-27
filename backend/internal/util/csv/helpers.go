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

// ParseCSVTags splits a comma-separated tags string into individual values.
// Returns empty slice for empty input. Trims whitespace from each tag.
// Filters out empty values after trim.
func ParseCSVTags(tagsStr string) []string {
	tagsStr = strings.TrimSpace(tagsStr)
	if tagsStr == "" {
		return []string{}
	}

	parts := strings.Split(tagsStr, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// Required CSV columns for asset bulk import. Only `name` is strictly required;
// `external_key` auto-mints to ASSET-NNN, `valid_from` defaults to NOW,
// `valid_to` to NULL, and `is_active` to TRUE in the storage layer.
var requiredCSVHeaders = []string{
	"name",
}

// normalizeHeader trims whitespace, strips a leading UTF-8 BOM (Excel adds one
// when saving a CSV), and lowercases for case-insensitive matching.
func normalizeHeader(h string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
}

// ValidateCSVHeaders checks if all required columns are present in the CSV header row.
// Column order is flexible - all required columns must be present but can be in any order.
// Extra columns are allowed and will be ignored.
// Matching is case-insensitive and tolerates a leading UTF-8 BOM.
//
// Returns detailed error listing missing columns if validation fails.
func ValidateCSVHeaders(headers []string) error {
	if len(headers) == 0 {
		return fmt.Errorf("CSV headers cannot be empty")
	}

	normalizedHeaders := make(map[string]bool)
	for _, h := range headers {
		normalizedHeaders[normalizeHeader(h)] = true
	}

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
		headerIdx[normalizeHeader(h)] = i
	}

	// getOpt returns the trimmed cell value if the header exists and the row
	// is long enough, otherwise the empty string. Use for optional columns.
	getOpt := func(name string) string {
		idx, ok := headerIdx[name]
		if !ok {
			return ""
		}
		if idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	name := getOpt("name")
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}

	// external_key is optional; storage auto-mints ASSET-NNN when empty.
	externalKey := getOpt("external_key")

	// valid_from / valid_to / is_active are optional; storage applies defaults
	// (NOW, NULL, TRUE respectively) when the parsed values are zero/empty.
	var validFrom time.Time
	if s := getOpt("valid_from"); s != "" {
		t, err := ParseCSVDate(s)
		if err != nil {
			return nil, fmt.Errorf("invalid valid_from: %w", err)
		}
		validFrom = t
	}

	var validToPtr *time.Time
	if s := getOpt("valid_to"); s != "" {
		t, err := ParseCSVDate(s)
		if err != nil {
			return nil, fmt.Errorf("invalid valid_to: %w", err)
		}
		validToPtr = &t
	}

	isActive := true
	if s := getOpt("is_active"); s != "" {
		b, err := ParseCSVBool(s)
		if err != nil {
			return nil, fmt.Errorf("invalid is_active: %w", err)
		}
		isActive = b
	}

	description := getOpt("description")

	if validToPtr != nil && !validFrom.IsZero() && validToPtr.Before(validFrom) {
		return nil, fmt.Errorf("valid_to must be after valid_from")
	}

	return &asset.Asset{
		OrgID:       orgID,
		ExternalKey: externalKey,
		Name:        name,
		Description: description,
		ValidFrom:   validFrom,
		ValidTo:     validToPtr,
		IsActive:    isActive,
	}, nil
}

// AssetWithTags holds a parsed asset and its associated tag values from CSV.
// TagValues contains raw tag values from the CSV "tags" column.
type AssetWithTags struct {
	Asset     *asset.Asset
	TagValues []string
}

// MapCSVRowToAssetWithTags parses a CSV row into an asset with optional tags.
// The "tags" column is optional - if missing, TagValues will be empty.
func MapCSVRowToAssetWithTags(row []string, headers []string, orgID int) (*AssetWithTags, error) {
	// Reuse existing MapCSVRowToAsset logic
	parsedAsset, err := MapCSVRowToAsset(row, headers, orgID)
	if err != nil {
		return nil, err
	}

	// Extract tags if column exists
	headerIdx := make(map[string]int)
	for i, h := range headers {
		headerIdx[normalizeHeader(h)] = i
	}

	var tagValues []string
	if tagsIdx, ok := headerIdx["tags"]; ok && tagsIdx < len(row) {
		tagValues = ParseCSVTags(row[tagsIdx])
	}

	return &AssetWithTags{
		Asset:     parsedAsset,
		TagValues: tagValues,
	}, nil
}
