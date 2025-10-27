# Feature: Bulk Asset Upload via CSV - Phase 2 (CSV Upload & Processing)

## Metadata
**Workspace**: backend
**Type**: feature
**Phase**: 2 of 2
**Dependencies**: Phase 1 (job tracking infrastructure) - PR #32

## Outcome
Users can upload CSV files to create multiple assets asynchronously with real-time progress tracking.

## Phase 2 Scope

**What Phase 1 Built** (Complete):
- ✅ Database table `bulk_import_jobs` for job tracking
- ✅ Model layer (`BulkImportJob`, `ErrorDetail`, response types)
- ✅ Storage CRUD operations (Create, Get, UpdateProgress, UpdateStatus)
- ✅ `GET /api/v1/assets/bulk/{jobId}` endpoint for status checking

**What Phase 2 Builds** (This Spec):
- 🎯 `POST /api/v1/assets/bulk` endpoint for CSV upload
- 🎯 CSV parsing with `encoding/csv`
- 🎯 Goroutine-based async processing
- 🎯 Row-by-row validation and insertion
- 🎯 Progress tracking during processing
- 🎯 Comprehensive integration tests
- 🎯 Manual E2E testing

## Technical Requirements

### CSV Upload Endpoint

#### Handler: `POST /api/v1/assets/bulk`

**Synchronous Phase** (must complete quickly, <200ms):
1. Parse multipart form, extract `file` field
2. Validate file:
   - Check file exists
   - Check MIME type: `text/csv` or `application/vnd.ms-excel`
   - Check extension: `.csv`
   - Check size: ≤ 5MB
3. Read and parse CSV headers
4. Validate headers match expected columns (case-insensitive):
   - Required: `identifier`, `name`, `type`, `valid_from`, `valid_to`, `is_active`
   - Optional: `description`
5. Count total rows (for progress tracking)
6. Validate row limit: ≤ 1000 rows
7. Extract account_id from JWT claims
8. Create job record in database with status `pending`
9. **Return immediately** with `202 Accepted` and job ID
10. Launch goroutine to process rows asynchronously

**Response (202 Accepted)**:
```json
{
  "status": "accepted",
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status_url": "/api/v1/assets/bulk/550e8400-e29b-41d4-a716-446655440000",
  "message": "CSV upload accepted. Processing asynchronously."
}
```

**Error Responses**:
- `400 Bad Request`: Missing file, invalid MIME type, wrong extension, invalid headers, too many rows
- `413 Payload Too Large`: File exceeds 5MB
- `401 Unauthorized`: Missing or invalid JWT
- `500 Internal Server Error`: Failed to create job record

### Async Processing Logic

**Goroutine Function**: `processCSVAsync(ctx context.Context, jobID uuid.UUID, accountID int, csvContent []byte)`

**Processing Steps**:
1. Parse CSV content with `encoding/csv`
2. Read all rows (skip header)
3. Update job status to `processing`
4. For each row (with error recovery):
   - Map CSV columns to `asset.Asset` struct
   - Parse dates: `YYYY-MM-DD` → `time.Time`
   - Parse boolean: `true/false/1/0/yes/no` (case-insensitive) → `bool`
   - Inject `account_id` from job context
   - Validate using existing storage method
   - Attempt to create asset via `storage.CreateAsset()`
   - On success: Increment `processed_rows`
   - On error: Record error detail (row number, field, message), increment `failed_rows`
   - Every 10 rows: Update job progress in database
5. After all rows processed:
   - Calculate final counts
   - Update job status to `completed` (if any succeeded) or `failed` (if all failed)
   - Set `completed_at` timestamp
6. Handle panics gracefully: Catch with `recover()`, mark job as `failed`, log error

**Error Recovery Strategy**:
- Individual row failures don't stop processing
- Database connection errors abort processing, mark job as `failed`
- Panics are caught and logged, job marked as `failed`

### CSV Parsing Utilities

**Required Helper Functions**:
```go
// ParseCSVDate converts "YYYY-MM-DD" to time.Time
func ParseCSVDate(dateStr string) (time.Time, error)

// ParseCSVBool converts "true/false/1/0/yes/no" to bool (case-insensitive)
func ParseCSVBool(boolStr string) (bool, error)

// ValidateCSVHeaders checks if headers match expected columns (case-insensitive)
func ValidateCSVHeaders(headers []string) error

// MapCSVRowToAsset converts CSV row to asset.Asset struct
func MapCSVRowToAsset(row []string, headers []string, accountID int) (*asset.Asset, error)
```

### Validation Rules

**File Validation** (synchronous):
- MIME type: `text/csv` or `application/vnd.ms-excel`
- Extension: `.csv`
- Size: ≤ 5MB (5 * 1024 * 1024 bytes)
- Row count: ≤ 1000 rows (configurable constant)

**Header Validation** (synchronous):
- Must contain all required columns (case-insensitive matching)
- Required: `identifier`, `name`, `type`, `valid_from`, `valid_to`, `is_active`
- Optional: `description`
- Extra columns are ignored

**Row Validation** (async, per row):
- Reuse existing validation from `asset.Asset` struct tags
- Additional checks:
  - `identifier`: Must be unique (check against existing DB records)
  - `type`: Must be one of: `person`, `device`, `asset`, `inventory`, `other`
  - `valid_from`, `valid_to`: Must be valid dates in `YYYY-MM-DD` format
  - `valid_to` must be after `valid_from`
  - `is_active`: Must be valid boolean
  - `description`: Optional, max 1024 chars

**Duplicate Detection**:
- Within CSV: Track identifiers in a `map[string]int` (identifier → row number)
- Against DB: Use existing `CreateAsset` method (DB constraint will catch duplicates)

### Request/Response Models

**Upload Request** (multipart form):
- Field name: `file`
- Content-Type: `multipart/form-data`
- Max file size: 5MB

**Upload Response Model**:
```go
type UploadResponse struct {
    Status     string `json:"status"`      // "accepted"
    JobID      string `json:"job_id"`      // UUID string
    StatusURL  string `json:"status_url"`  // Full path to status endpoint
    Message    string `json:"message"`     // User-friendly message
}
```

**Error Response Model** (reuse existing):
```go
// Use modelerrors.ErrorResponse from internal/models/errors
```

### Integration with Existing Code

**Storage Layer** (reuse existing):
- `CreateBulkImportJob(ctx, accountID, totalRows)` - Create job record
- `UpdateBulkImportJobStatus(ctx, jobID, status)` - Update status to "processing", "completed", "failed"
- `UpdateBulkImportJobProgress(ctx, jobID, processedRows, failedRows, errors)` - Update progress

**Asset Creation** (reuse existing):
- `storage.CreateAsset(ctx, asset)` - Create individual asset
- Uses existing validation and database constraints
- Returns error if identifier is duplicate (capture in job errors)

**JWT Claims** (reuse existing):
- `middleware.GetUserClaims(r)` - Extract account_id for tenant isolation
- Use `claims.CurrentAccountID` to get account context

### Configuration

**Constants** (define in handler or config):
```go
const (
    MaxFileSize  = 5 * 1024 * 1024  // 5MB
    MaxRows      = 1000              // Maximum rows per CSV
    ProgressStep = 10                // Update progress every N rows
)
```

## Implementation Tasks

### Task 1: Add CSV Helper Functions
- File: `backend/internal/handlers/bulkimport/csv_helpers.go`
- Functions:
  - `ParseCSVDate(dateStr string) (time.Time, error)`
  - `ParseCSVBool(boolStr string) (bool, error)`
  - `ValidateCSVHeaders(headers []string) error`
  - `MapCSVRowToAsset(row []string, headers map[string]int, accountID int) (*asset.Asset, error)`
- Unit tests for all helper functions

### Task 2: Add CSV Upload Handler
- File: `backend/internal/handlers/bulkimport/bulkimport.go`
- Function: `UploadCSV(w http.ResponseWriter, r *http.Request)`
- Synchronous validation logic (file, headers, row count)
- Job creation
- Goroutine launch
- Swagger/OpenAPI annotations
- Error handling with proper HTTP status codes

### Task 3: Add Async Processing Function
- File: `backend/internal/handlers/bulkimport/bulkimport.go`
- Function: `processCSVAsync(ctx context.Context, h *Handler, jobID uuid.UUID, accountID int, csvContent []byte)`
- Row-by-row parsing and validation
- Asset creation via storage layer
- Progress updates every 10 rows
- Error collection and recovery
- Job status updates (processing → completed/failed)

### Task 4: Update Route Registration
- File: `backend/internal/handlers/bulkimport/bulkimport.go`
- Add POST route to `RegisterRoutes()` method:
  ```go
  r.Post("/api/v1/assets/bulk", h.UploadCSV)
  ```

### Task 5: Add Integration Tests
- File: `backend/internal/handlers/bulkimport/bulkimport_integration_test.go`
- Test cases:
  - Valid CSV upload (all rows succeed)
  - Partial failures (some rows succeed, some fail)
  - Invalid file format (rejected before job creation)
  - Invalid headers (rejected before job creation)
  - Duplicate identifiers within CSV
  - Duplicate identifiers against existing DB records
  - File too large (rejected with 413)
  - Too many rows (rejected with 400)
  - Job status polling during processing
  - Tenant isolation (can't access other account's jobs)

### Task 6: Manual E2E Testing
- Create test CSV files (valid, invalid, partial)
- Test upload via curl or Postman
- Verify job status updates in real-time
- Verify assets created in database
- Test error scenarios (invalid formats, duplicates, etc.)

## Validation Criteria

- [ ] Upload endpoint returns `202 Accepted` within 200ms
- [ ] Goroutine processes CSV in background (non-blocking)
- [ ] Status endpoint shows real-time progress during processing
- [ ] Successfully processes valid CSV with 10+ assets
- [ ] Handles partial failures (some rows succeed, some fail)
- [ ] Returns detailed errors with row numbers, fields, and messages
- [ ] Rejects files over 5MB before creating job
- [ ] Rejects invalid headers before creating job
- [ ] Rejects files with >1000 rows before creating job
- [ ] Detects duplicate identifiers within CSV
- [ ] Detects duplicate identifiers against existing DB records
- [ ] Job status persists across server restarts
- [ ] Tenant isolation enforced (users can only access own jobs)
- [ ] All existing tests remain passing
- [ ] Integration tests cover all scenarios
- [ ] Manual E2E tests successful

## Success Metrics

- [ ] Upload endpoint responds within 200ms (fast validation)
- [ ] Small uploads (<100 rows) complete within 3 seconds
- [ ] Large uploads (500-1000 rows) complete within 10 seconds
- [ ] Progress updates every 10 rows
- [ ] Validation errors include row number, field, and message
- [ ] Multiple concurrent uploads work correctly
- [ ] No memory leaks or goroutine leaks

## Files to Create/Modify

**New Files**:
- `backend/internal/handlers/bulkimport/csv_helpers.go` - CSV parsing utilities
- `backend/internal/handlers/bulkimport/csv_helpers_test.go` - Unit tests for helpers
- `backend/internal/handlers/bulkimport/bulkimport_integration_test.go` - Integration tests

**Modified Files**:
- `backend/internal/handlers/bulkimport/bulkimport.go` - Add UploadCSV handler and async processing

**Test Files**:
- `spec/active/asset-csv-creation/test-data/valid.csv` - Valid test CSV
- `spec/active/asset-csv-creation/test-data/invalid-headers.csv` - Invalid headers
- `spec/active/asset-csv-creation/test-data/duplicate-ids.csv` - Duplicate identifiers
- `spec/active/asset-csv-creation/test-data/partial-fail.csv` - Some valid, some invalid

## References

- Phase 1 implementation: PR #32
- Existing patterns: `backend/internal/handlers/assets/assets.go`
- Storage methods: `backend/internal/storage/bulk_import_jobs.go`
- CSV parsing: Go standard library `encoding/csv`
- Validation: `github.com/go-playground/validator/v10`
