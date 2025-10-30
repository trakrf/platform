# Implementation Plan: Bulk Asset Upload - Phase 2B (Async Processing)
Generated: 2025-10-27
Specification: phase2-spec.md (lines 69-96, 212-220)
Phase: 2B of 4 (2A-1 ✅ → 2A-2 ✅ → **2B** → 2C)

## Understanding

This phase implements the **async CSV processing goroutine** that actually creates assets from CSV rows. Phase 2A created the upload endpoint that validates files and creates job records but left processing stubbed. Phase 2B completes the implementation by adding:

**What Phase 2A Built** (Complete):
- ✅ CSV helpers: ParseCSVDate, ParseCSVBool, ValidateCSVHeaders
- ✅ POST /api/v1/assets/bulk endpoint with file validation
- ✅ Job creation in database with "pending" status
- ✅ 202 Accepted response with job ID
- ✅ Service layer architecture (handler → service → validator)

**What Phase 2B Builds** (This Plan):
- 🎯 MapCSVRowToAsset helper function
- 🎯 processCSVAsync goroutine implementation
- 🎯 Row-by-row parsing and asset creation
- 🎯 Progress updates every 10 rows
- 🎯 Error collection with row numbers
- 🎯 Job status updates (pending → processing → completed/failed)
- 🎯 Panic recovery and error handling

**Out of Scope** (later phases):
- Integration tests (Phase 2C)
- E2E manual tests (Phase 2C)

**Why This Phase Critical**:
- Jobs currently stay in "pending" forever
- No actual asset creation happens
- Core value prop (bulk creation) not delivered
- Goroutine is already stubbed with TODO comment

## Relevant Files

### Files Created in Phase 2A (reference):
- `backend/internal/util/csv/helpers.go` - ParseCSVDate, ParseCSVBool, ValidateCSVHeaders
- `backend/internal/util/csv/helpers_test.go` - 47 tests for CSV helpers
- `backend/internal/services/bulkimport/service.go` - Business logic, ProcessUpload method
- `backend/internal/services/bulkimport/validator.go` - File & CSV validation
- `backend/internal/handlers/assets/bulkimport.go` - Thin HTTP layer

### Files to Modify (Phase 2B):
- `backend/internal/services/bulkimport/service.go` - Implement processCSVAsync, launch goroutine
- `backend/internal/util/csv/helpers.go` - Add MapCSVRowToAsset function
- `backend/internal/util/csv/helpers_test.go` - Add tests for MapCSVRowToAsset

### Reference Files (existing patterns):
- `backend/internal/storage/assets.go` - CreateAsset method
- `backend/internal/storage/bulk_import_jobs.go` - UpdateProgress, UpdateStatus methods
- `backend/internal/models/asset/asset.go` - Asset struct with validation tags

## Architecture Impact

**Subsystems affected**:
- Service layer: Goroutine management, async processing
- Storage layer: Multiple calls per job (progress updates)
- Database: Concurrent writes (job updates + asset inserts)

**New dependencies**: None (using stdlib)

**Concurrency considerations**:
- One goroutine per upload
- No shared state between goroutines
- Each goroutine has its own context
- Progress updates batched (every 10 rows)

**Breaking changes**: None (adding functionality only)

## Task Breakdown

### Task 1: Add MapCSVRowToAsset helper function

**File**: `backend/internal/util/csv/helpers.go`
**Action**: ADD (new function)

**Implementation**:
```go
// MapCSVRowToAsset converts a CSV row to an asset.Asset struct
// Returns error if required fields are missing or invalid
func MapCSVRowToAsset(row []string, headers []string, orgID int) (*asset.Asset, error) {
	// Build header index map
	headerIdx := make(map[string]int)
	for i, h := range headers {
		headerIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Helper to get column value safely
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

	// Extract required fields
	identifier, err := getCol("identifier")
	if err != nil {
		return nil, err
	}

	name, err := getCol("name")
	if err != nil {
		return nil, err
	}

	assetType, err := getCol("type")
	if err != nil {
		return nil, err
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

	// Parse dates
	validFrom, err := ParseCSVDate(validFromStr)
	if err != nil {
		return nil, fmt.Errorf("invalid valid_from: %w", err)
	}

	validTo, err := ParseCSVDate(validToStr)
	if err != nil {
		return nil, fmt.Errorf("invalid valid_to: %w", err)
	}

	// Parse boolean
	isActive, err := ParseCSVBool(isActiveStr)
	if err != nil {
		return nil, fmt.Errorf("invalid is_active: %w", err)
	}

	// Extract optional description
	description := ""
	if descIdx, ok := headerIdx["description"]; ok && descIdx < len(row) {
		description = strings.TrimSpace(row[descIdx])
	}

	// Validate date logic
	if validTo.Before(validFrom) {
		return nil, fmt.Errorf("valid_to must be after valid_from")
	}

	// Build asset
	return &asset.Asset{
		OrgID:       orgID,
		Identifier:  identifier,
		Name:        name,
		Type:        assetType,
		Description: &description,
		ValidFrom:   validFrom,
		ValidTo:     validTo,
		IsActive:    isActive,
	}, nil
}
```

**Add required import**:
```go
import (
	// ... existing imports ...
	"github.com/trakrf/platform/backend/internal/models/asset"
)
```

**Validation**:
```bash
go fmt ./internal/util/csv/
go vet ./internal/util/csv/
go build ./internal/util/csv/
```

**Expected**: No errors, function compiles

---

### Task 2: Add unit tests for MapCSVRowToAsset

**File**: `backend/internal/util/csv/helpers_test.go`
**Action**: ADD (new test function)

**Implementation**:
```go
func TestMapCSVRowToAsset_ValidRow(t *testing.T) {
	headers := []string{"identifier", "name", "type", "description", "valid_from", "valid_to", "is_active"}
	row := []string{"ASSET-001", "Test Asset", "device", "Test description", "2024-01-01", "2024-12-31", "true"}
	orgID := 1

	asset, err := MapCSVRowToAsset(row, headers, orgID)
	if err != nil {
		t.Fatalf("MapCSVRowToAsset failed: %v", err)
	}

	if asset.Identifier != "ASSET-001" {
		t.Errorf("Expected identifier ASSET-001, got %s", asset.Identifier)
	}
	if asset.Name != "Test Asset" {
		t.Errorf("Expected name 'Test Asset', got %s", asset.Name)
	}
	if asset.Type != "device" {
		t.Errorf("Expected type 'device', got %s", asset.Type)
	}
	if !asset.IsActive {
		t.Errorf("Expected is_active true, got false")
	}
	if asset.OrgID != orgID {
		t.Errorf("Expected org_id %d, got %d", orgID, asset.OrgID)
	}
}

func TestMapCSVRowToAsset_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		row     []string
	}{
		{
			name:    "missing identifier",
			headers: []string{"name", "type", "valid_from", "valid_to", "is_active"},
			row:     []string{"Test", "device", "2024-01-01", "2024-12-31", "true"},
		},
		{
			name:    "missing name",
			headers: []string{"identifier", "type", "valid_from", "valid_to", "is_active"},
			row:     []string{"ASSET-001", "device", "2024-01-01", "2024-12-31", "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MapCSVRowToAsset(tt.row, tt.headers, 1)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestMapCSVRowToAsset_InvalidDates(t *testing.T) {
	headers := []string{"identifier", "name", "type", "valid_from", "valid_to", "is_active"}
	row := []string{"ASSET-001", "Test", "device", "2024-12-31", "2024-01-01", "true"}

	_, err := MapCSVRowToAsset(row, headers, 1)
	if err == nil {
		t.Error("Expected error for valid_to before valid_from")
	}
	if !strings.Contains(err.Error(), "valid_to must be after valid_from") {
		t.Errorf("Expected date validation error, got: %v", err)
	}
}
```

**Validation**:
```bash
go test ./internal/util/csv/
```

**Expected**: All tests pass (new + existing 47 tests)

---

### Task 3: Implement processCSVAsync in service

**File**: `backend/internal/services/bulkimport/service.go`
**Action**: MODIFY (implement stubbed function)

**Implementation**:
```go
const ProgressUpdateInterval = 10 // Update progress every N rows

func (s *Service) processCSVAsync(
	ctx context.Context,
	jobID uuid.UUID,
	orgID int,
	records [][]string,
	headers []string,
) {
	defer func() {
		if r := recover(); r != nil {
			s.storage.UpdateBulkImportJobStatus(ctx, jobID, "failed")
		}
	}()

	s.storage.UpdateBulkImportJobStatus(ctx, jobID, "processing")

	var processedRows int
	var failedRows int
	var errors []bulkimport.ErrorDetail

	// Skip header row (index 0)
	dataRows := records[1:]

	for rowIdx, row := range dataRows {
		rowNumber := rowIdx + 2 // +2 because: +1 for 1-indexed, +1 for header

		asset, err := csvutil.MapCSVRowToAsset(row, headers, orgID)
		if err != nil {
			failedRows++
			errors = append(errors, bulkimport.ErrorDetail{
				Row:   rowNumber,
				Field: "",
				Error: err.Error(),
			})
			continue
		}

		_, err = s.storage.CreateAsset(ctx, *asset)
		if err != nil {
			failedRows++
			errorMsg := err.Error()
			field := ""

			// Parse field from error if possible (e.g., "duplicate identifier")
			if strings.Contains(errorMsg, "identifier") {
				field = "identifier"
			}

			errors = append(errors, bulkimport.ErrorDetail{
				Row:   rowNumber,
				Field: field,
				Error: errorMsg,
			})
		} else {
			processedRows++
		}

		// Update progress every N rows
		if (rowIdx+1)%ProgressUpdateInterval == 0 {
			s.storage.UpdateBulkImportJobProgress(ctx, jobID, processedRows, failedRows, errors)
		}
	}

	// Final progress update
	s.storage.UpdateBulkImportJobProgress(ctx, jobID, processedRows, failedRows, errors)

	// Determine final status
	finalStatus := "completed"
	if processedRows == 0 {
		finalStatus = "failed"
	}

	s.storage.UpdateBulkImportJobStatus(ctx, jobID, finalStatus)
}
```

**Add required import**:
```go
import (
	// ... existing imports ...
	csvutil "github.com/trakrf/platform/backend/internal/util/csv"
)
```

**Validation**:
```bash
go fmt ./internal/services/bulkimport/
go vet ./internal/services/bulkimport/
go build ./internal/services/bulkimport/
```

**Expected**: No errors, compiles successfully

---

### Task 4: Launch goroutine from ProcessUpload

**File**: `backend/internal/services/bulkimport/service.go`
**Action**: MODIFY (uncomment goroutine launch)

**Find this section** in ProcessUpload method:
```go
// TODO Phase 2B: Launch goroutine to process CSV rows
// go s.processCSVAsync(context.Background(), job.ID, orgID, records, headers)
_ = records
_ = headers
```

**Replace with**:
```go
go s.processCSVAsync(context.Background(), job.ID, orgID, records, headers)
```

**Validation**:
```bash
go fmt ./internal/services/bulkimport/
go vet ./internal/services/bulkimport/
go build ./internal/services/bulkimport/
```

**Expected**: No errors, no unused variable warnings

---

### Task 5: Add error detail to storage method

**File**: `backend/internal/storage/bulk_import_jobs.go`
**Action**: VERIFY (check if UpdateBulkImportJobProgress handles errors slice)

**Expected signature**:
```go
func (s *Storage) UpdateBulkImportJobProgress(
	ctx context.Context,
	jobID uuid.UUID,
	processedRows int,
	failedRows int,
	errors []bulkimport.ErrorDetail,
) error
```

**If method doesn't exist or has different signature**, we need to update it in a separate task.

**Validation**: Check the file and verify

---

### Task 6: Validation gate - Full build and test

**Commands**:
```bash
# Format all code
go fmt ./...

# Vet all code
go vet ./...

# Build entire backend
go build ./...

# Run all tests
go test ./...

# Check for race conditions (optional but recommended)
go test -race ./internal/services/bulkimport/
```

**Expected**:
- ✅ All code formatted
- ✅ No vet issues
- ✅ Successful compilation
- ✅ All existing tests still pass
- ✅ New MapCSVRowToAsset tests pass
- ✅ No race conditions detected

---

## VALIDATION GATES (MANDATORY)

Run after EVERY task:

### Gate 1: Code Formatting
```bash
go fmt ./internal/services/bulkimport/ ./internal/util/csv/
```
**Expected**: No changes needed (or only your new code formatted)

### Gate 2: Code Quality
```bash
go vet ./internal/services/bulkimport/ ./internal/util/csv/
```
**Expected**: No issues reported

### Gate 3: Build
```bash
go build ./internal/services/bulkimport/ ./internal/util/csv/
```
**Expected**: Successful compilation, no errors

### Gate 4: Tests
```bash
go test ./internal/util/csv/
```
**Expected**: All tests pass (47 existing + new MapCSVRowToAsset tests)

### Gate 5: Integration Check
```bash
go build ./...
```
**Expected**: Entire backend compiles successfully

---

## Manual Testing Checklist

After implementation, test manually:

### Test 1: Valid CSV upload
```bash
# Create valid.csv:
# identifier,name,type,description,valid_from,valid_to,is_active
# ASSET-001,Test Asset,device,Test description,2024-01-01,2024-12-31,true

curl -X POST http://localhost:8080/api/v1/assets/bulk \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@valid.csv"

# Expected: 202 Accepted with job_id

# Check status immediately
curl http://localhost:8080/api/v1/assets/bulk/{job_id} \
  -H "Authorization: Bearer $TOKEN"

# Expected: status "processing" or "completed"
```

### Test 2: Invalid row in CSV
```bash
# Create partial.csv:
# identifier,name,type,description,valid_from,valid_to,is_active
# ASSET-001,Test Asset,device,Test,2024-01-01,2024-12-31,true
# ASSET-002,Bad Asset,device,Test,2024-12-31,2024-01-01,true  # Bad dates
# ASSET-003,Good Asset,device,Test,2024-01-01,2024-12-31,true

# Upload and check status
# Expected: processed_rows: 2, failed_rows: 1, errors: [{row: 3, error: "valid_to must be after valid_from"}]
```

### Test 3: Duplicate identifier
```bash
# Create duplicate.csv with same identifier twice
# Expected: Second row fails with duplicate error
```

---

## Risk Assessment

**Complexity**: 5/10 (MEDIUM)
- Goroutine management: Familiar pattern
- Row-by-row processing: Straightforward loop
- Error collection: Simple slice append
- Progress updates: Batched, not complex

**Dependencies**:
- Existing storage methods (already tested)
- CSV helpers (already implemented and tested)
- Asset validation (existing struct tags)

**Risks**:
- **Goroutine leaks**: Mitigated by defer recover
- **Database connection in goroutine**: Use context passed to storage methods
- **Concurrent job updates**: Single goroutine per job, no contention
- **Memory usage**: 5MB file limit bounds memory, records loaded once

**Mitigation**:
- Panic recovery with defer
- Use background context for long-running operations
- Batch progress updates (every 10 rows)
- Comprehensive error handling per row

---

## Success Criteria

- [ ] MapCSVRowToAsset converts CSV rows to Asset structs correctly
- [ ] processCSVAsync processes all rows and creates assets
- [ ] Progress updates occur every 10 rows
- [ ] Errors collected with row numbers and messages
- [ ] Job status transitions: pending → processing → completed
- [ ] Job status transitions: pending → processing → failed (if all fail)
- [ ] Panic recovery marks job as failed
- [ ] Duplicate identifiers caught and reported
- [ ] Invalid dates/booleans caught and reported
- [ ] All existing tests still pass
- [ ] New MapCSVRowToAsset tests pass
- [ ] Manual upload test succeeds

---

## Notes

- This phase completes the core bulk upload feature
- Phase 2C will add comprehensive integration tests
- Goroutine uses background context (not request context) to survive request lifecycle
- Progress updates batched to reduce database load
- Individual row failures don't stop processing (collect errors, continue)
- Job survives server restarts (persisted in database)

---

## Estimated Effort

- Task 1 (MapCSVRowToAsset): 15 minutes
- Task 2 (Unit tests): 10 minutes
- Task 3 (processCSVAsync): 20 minutes
- Task 4 (Launch goroutine): 2 minutes
- Task 5 (Verification): 5 minutes
- Task 6 (Validation): 5 minutes
- Manual testing: 10 minutes

**Total**: ~70 minutes (1.2 hours)

**Complexity**: MEDIUM (familiar patterns, well-defined scope)
**Confidence**: 8/10 (HIGH - building on working Phase 2A foundation)
