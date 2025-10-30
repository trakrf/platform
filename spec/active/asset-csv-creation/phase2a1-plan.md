# Implementation Plan: Bulk Asset Upload - Phase 2A-1 (CSV Helpers)
Generated: 2025-10-27
Specification: phase2-spec.md
Phase: 2A-1 of 4 (2A-1 → 2A-2 → 2B → 2C)

## Understanding

This phase implements **pure utility functions** for CSV parsing without any HTTP, database, or goroutine complexity. These are foundational helper functions that Phase 2A-2 (upload endpoint) will consume.

**Scope**:
- Date parsing with multiple format support (YYYY-MM-DD, MM/DD/YYYY, DD-MM-YYYY)
- Boolean parsing with case-insensitive + whitespace trimming (true/false/1/0/yes/no)
- CSV header validation with flexible column ordering
- Comprehensive unit tests for all functions
- Detailed error messages with suggestions for better UX

**Out of Scope** (later phases):
- HTTP multipart form handling (Phase 2A-2)
- File validation (Phase 2A-2)
- Goroutine processing (Phase 2B)
- Integration tests (Phase 2C)

**Why This Phase First**:
- ✅ Zero dependencies on HTTP/DB/async - pure functions
- ✅ Can be fully unit tested in isolation
- ✅ Validates parsing logic before adding complexity
- ✅ Foundation for upload endpoint (Phase 2A-2)

## Relevant Files

### Reference Patterns (existing code to follow):

- `backend/internal/handlers/assets/assets.go` (lines 1-26) - Handler structure pattern
- `backend/internal/util/password/password_test.go` (lines 8-51) - Testing pattern (simple assertions)
- `backend/internal/models/auth/auth_test.go` (lines 7-31) - Test structure pattern
- `backend/internal/handlers/assets/bulkimport.go` (lines 26-81) - Error handling with httputil pattern

### Files to Create:

- `backend/internal/handlers/assets/csv_helpers.go` - CSV parsing utility functions
  - `ParseCSVDate(dateStr string) (time.Time, error)` - Multi-format date parsing
  - `ParseCSVBool(boolStr string) (bool, error)` - Flexible boolean parsing
  - `ValidateCSVHeaders(headers []string) error` - Column validation
  - Helper function for building detailed error messages

- `backend/internal/handlers/assets/csv_helpers_test.go` - Comprehensive unit tests
  - Test valid inputs for all formats
  - Test invalid inputs with error message validation
  - Test edge cases (whitespace, case variations, empty strings)

### Files to Modify:

None - Phase 2A-1 is purely additive

## Architecture Impact

- **Subsystems affected**: None (pure utility functions)
- **New dependencies**: None (using stdlib `time` and `strings` packages)
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create csv_helpers.go with ParseCSVDate

**File**: `backend/internal/handlers/assets/csv_helpers.go`
**Action**: CREATE

**Implementation**:
```go
package assets

import (
	"fmt"
	"strings"
	"time"
)

// Supported date formats for CSV import
const (
	DateFormatISO     = "2006-01-02"      // YYYY-MM-DD
	DateFormatUSA     = "01/02/2006"      // MM/DD/YYYY
	DateFormatEuropean = "02-01-2006"     // DD-MM-YYYY
)

// ParseCSVDate converts a date string to time.Time, supporting multiple formats:
// - YYYY-MM-DD (ISO 8601)
// - MM/DD/YYYY (US format)
// - DD-MM-YYYY (European format)
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
		"invalid date format '%s': could not parse as %s. Expected formats: YYYY-MM-DD, MM/DD/YYYY, or DD-MM-YYYY",
		dateStr,
		strings.Join(parseErrs, ", "),
	)
}
```

**Validation**:
```bash
# From backend directory
go fmt ./internal/handlers/assets/csv_helpers.go
go vet ./internal/handlers/assets/csv_helpers.go
go build ./internal/handlers/assets/
```

**Expected**: No errors, file compiles successfully

---

### Task 2: Create ParseCSVBool function

**File**: `backend/internal/handlers/assets/csv_helpers.go`
**Action**: MODIFY (add function)

**Implementation**:
```go
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
```

**Validation**:
```bash
go fmt ./internal/handlers/assets/csv_helpers.go
go vet ./internal/handlers/assets/csv_helpers.go
go build ./internal/handlers/assets/
```

**Expected**: No errors, file compiles successfully

---

### Task 3: Create ValidateCSVHeaders function

**File**: `backend/internal/handlers/assets/csv_helpers.go`
**Action**: MODIFY (add function)

**Implementation**:
```go
// Required CSV columns for asset bulk import
var requiredCSVHeaders = []string{
	"identifier",
	"name",
	"type",
	"valid_from",
	"valid_to",
	"is_active",
}

// Optional CSV columns
var optionalCSVHeaders = []string{
	"description",
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
```

**Validation**:
```bash
go fmt ./internal/handlers/assets/csv_helpers.go
go vet ./internal/handlers/assets/csv_helpers.go
go build ./internal/handlers/assets/
```

**Expected**: No errors, all functions compile successfully

---

### Task 4: Create comprehensive unit tests for ParseCSVDate

**File**: `backend/internal/handlers/assets/csv_helpers_test.go`
**Action**: CREATE

**Pattern**: Reference `backend/internal/util/password/password_test.go` for test structure

**Implementation**:
```go
package assets

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
```

**Validation**:
```bash
go test ./internal/handlers/assets/ -v -run TestParseCSVDate
```

**Expected**: All tests pass

---

### Task 5: Create unit tests for ParseCSVBool

**File**: `backend/internal/handlers/assets/csv_helpers_test.go`
**Action**: MODIFY (add tests)

**Implementation**:
```go
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
```

**Validation**:
```bash
go test ./internal/handlers/assets/ -v -run TestParseCSVBool
```

**Expected**: All tests pass

---

### Task 6: Create unit tests for ValidateCSVHeaders

**File**: `backend/internal/handlers/assets/csv_helpers_test.go`
**Action**: MODIFY (add tests)

**Implementation**:
```go
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
```

**Validation**:
```bash
go test ./internal/handlers/assets/ -v -run TestValidateCSVHeaders
```

**Expected**: All tests pass

---

## VALIDATION GATES (MANDATORY)

After EVERY task, run these commands from the `backend` directory:

### Gate 1: Code Formatting
```bash
just backend lint
# or: go fmt ./internal/handlers/assets/csv_helpers*
```
**Expected**: No changes needed (code already formatted)

### Gate 2: Code Quality
```bash
go vet ./internal/handlers/assets/
```
**Expected**: No issues reported

### Gate 3: Unit Tests
```bash
just backend test
# or: go test ./internal/handlers/assets/ -v
```
**Expected**: All tests passing (0 failures)

### Gate 4: Build
```bash
just backend build
# or: go build ./...
```
**Expected**: Successful compilation

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Final Validation Sequence

After all tasks complete:

```bash
# From backend directory
go fmt ./internal/handlers/assets/csv_helpers*
go vet ./internal/handlers/assets/
go test ./internal/handlers/assets/ -v -run "TestParseCSV|TestValidateCSV"
go build ./...
```

**Expected**:
- ✅ All files formatted correctly
- ✅ No vet issues
- ✅ All 3 test functions pass (ParseCSVDate, ParseCSVBool, ValidateCSVHeaders)
- ✅ Successful build

## Risk Assessment

### Risks:

1. **Risk**: Date parsing ambiguity (01/02/2024 could be Jan 2 or Feb 1)
   **Mitigation**: Document clearly in error messages which formats are tried. Users should use ISO format (YYYY-MM-DD) for clarity.

2. **Risk**: Boolean parsing too permissive (accepting many variations)
   **Mitigation**: Comprehensive tests cover all valid combinations. Error messages list all accepted values.

3. **Risk**: Case-insensitive header matching could match wrong columns
   **Mitigation**: Using exact string matching after normalization. Headers are explicit and unlikely to collide.

## Integration Points

**None for Phase 2A-1** - Pure utility functions with no external dependencies.

**Phase 2A-2 will consume these functions**:
- UploadCSV handler will call `ValidateCSVHeaders()` during file validation
- Background processing (Phase 2B) will call `ParseCSVDate()` and `ParseCSVBool()` for each row

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)

**Breakdown**:
- 📁 File Impact: Creating 2 files (csv_helpers.go, csv_helpers_test.go) = 1pt
- 🔗 Subsystems: 0 subsystems (pure functions) = 0pts
- 🔢 Task Estimate: 6 subtasks = 1pt
- 📦 Dependencies: 0 new packages (stdlib only) = 0pts
- 🆕 Pattern Novelty: Existing patterns (test structure from password_test.go) = 0pts
- **Total**: 2pts (rounded to 3/10 for conservative estimate)

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from user (multiple date formats, case-insensitive bool, flexible headers)
- ✅ Similar test patterns found in codebase (password_test.go, auth_test.go)
- ✅ All clarifying questions answered
- ✅ Pure functions - easy to test and validate
- ✅ No external dependencies or integration complexity
- ✅ Standard library packages (time, strings) - well-documented

**Assessment**: Very high confidence in successful implementation. Pure utility functions with comprehensive test coverage are straightforward and low-risk.

**Estimated one-pass success probability**: 95%

**Reasoning**: This is purely algorithmic logic with no I/O, concurrency, or external dependencies. The main risk is edge case handling in date parsing, which is mitigated by comprehensive tests. The test patterns from the codebase are clear and easy to follow.

## Next Steps (After Phase 2A-1 Ships)

**Phase 2A-2: Upload Endpoint (Stub Processing)**
- Branch from: `feature/assets-crud` (or `feature/bulk-import-phase2a1` after merge)
- Add: `UploadCSV(w, r)` handler
- Multipart form parsing
- File validation (size, MIME type, extension)
- Use CSV helpers from Phase 2A-1
- Create job in DB with status "pending"
- Return 202 Accepted (processing stubbed)

**Complexity**: 4/10 (LOW-MEDIUM)
**Estimated subtasks**: 5
