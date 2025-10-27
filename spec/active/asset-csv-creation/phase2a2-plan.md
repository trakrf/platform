# Implementation Plan: Bulk Asset Upload - Phase 2A-2 (Upload Endpoint)
Generated: 2025-10-27
Specification: phase2a2-outline.md
Phase: 2A-2 of 4 (2A-1 → 2A-2 → 2B → 2C)

## Understanding

This phase implements the **CSV upload endpoint** with synchronous file validation and job creation. The endpoint accepts CSV files, validates them, creates job records in the database, and returns immediately with a job ID. **Actual row processing is stubbed** - jobs remain in "pending" status until Phase 2B adds goroutine-based async processing.

**Scope**:
- POST /api/v1/assets/bulk endpoint
- Multipart form parsing
- File validation (size, MIME type, extension)
- CSV header validation using helpers from Phase 2A-1
- Row counting for progress tracking
- Job creation in database
- 202 Accepted response with job ID

**Out of Scope** (later phases):
- Goroutine launch and async processing (Phase 2B)
- Row-by-row CSV parsing and validation (Phase 2B)
- Asset creation from CSV rows (Phase 2B)
- Integration tests (Phase 2C)

**Why This Phase Second**:
- ✅ Builds on tested CSV helpers from Phase 2A-1
- ✅ HTTP layer can be tested independently
- ✅ Validates upload flow before adding async complexity
- ✅ Clear validation: upload works, creates jobs, status endpoint shows pending

## Relevant Files

### Reference Patterns (existing code to follow):

- `backend/internal/handlers/assets/assets.go` (lines 39-50) - Handler pattern with validation
- `backend/internal/handlers/assets/bulkimport.go` (lines 26-81) - GetJobStatus handler pattern
- `backend/internal/util/csv/helpers.go` - ValidateCSVHeaders from Phase 2A-1
- `backend/internal/storage/bulk_import_jobs.go` (lines 14-27) - CreateBulkImportJob method

### Files to Create:

None - adding to existing files

### Files to Modify:

- `backend/internal/handlers/assets/bulkimport.go` - Add UploadCSV handler function
- `backend/internal/handlers/assets/assets.go` - Add POST route registration
- `backend/internal/models/bulkimport/bulkimport.go` - Add UploadResponse type

## Architecture Impact

- **Subsystems affected**: HTTP (multipart forms), Storage (job creation), CSV parsing
- **New dependencies**: None (using stdlib `encoding/csv`, `mime/multipart`)
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add UploadResponse model

**File**: `backend/internal/models/bulkimport/bulkimport.go`
**Action**: MODIFY (add type)

**Implementation**:
```go
// UploadResponse is returned when a CSV file is successfully accepted
type UploadResponse struct {
	Status    string `json:"status"`      // "accepted"
	JobID     string `json:"job_id"`      // UUID string
	StatusURL string `json:"status_url"`  // "/api/v1/assets/bulk/{jobId}"
	Message   string `json:"message"`     // User-friendly message
}
```

**Validation**:
```bash
go fmt ./internal/models/bulkimport/
go vet ./internal/models/bulkimport/
go build ./internal/models/bulkimport/
```

**Expected**: No errors, type compiles successfully

---

### Task 2: Add constants for file validation

**File**: `backend/internal/handlers/assets/bulkimport.go`
**Action**: MODIFY (add constants at top of file)

**Implementation**:
```go
const (
	MaxFileSize = 5 * 1024 * 1024 // 5MB
	MaxRows     = 1000            // Maximum rows per CSV
)

// Allowed MIME types for CSV files
var allowedMIMETypes = map[string]bool{
	"text/csv":                     true,
	"application/vnd.ms-excel":     true,
	"application/csv":              true,
	"text/plain":                   true, // Some systems send CSV as text/plain
}
```

**Validation**:
```bash
go fmt ./internal/handlers/assets/bulkimport.go
go vet ./internal/handlers/assets/bulkimport.go
go build ./internal/handlers/assets/
```

**Expected**: No errors

---

### Task 3: Implement UploadCSV handler (part 1 - file extraction and validation)

**File**: `backend/internal/handlers/assets/bulkimport.go`
**Action**: MODIFY (add function)

**Implementation**:
```go
// @Summary Upload CSV for bulk asset creation
// @Description Accepts CSV file and creates async job. Returns immediately with job ID.
// @Tags bulk-import
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "CSV file with assets"
// @Success 202 {object} bulkimport.UploadResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid file or headers"
// @Failure 413 {object} modelerrors.ErrorResponse "File too large"
// @Security BearerAuth
// @Router /api/v1/assets/bulk [post]
func (h *Handler) UploadCSV(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	// Extract org_id from JWT claims
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Missing org context", "", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	// Parse multipart form (max 6MB to account for overhead beyond 5MB file)
	err := r.ParseMultipartForm(6 * 1024 * 1024)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Failed to parse multipart form", err.Error(), requestID)
		return
	}

	// Get the file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Missing or invalid 'file' field", err.Error(), requestID)
		return
	}
	defer file.Close()

	// Validate file size
	if header.Size > MaxFileSize {
		httputil.WriteJSONError(w, r, http.StatusRequestEntityTooLarge, modelerrors.ErrBadRequest,
			fmt.Sprintf("File too large: %d bytes (max %d bytes / 5MB)", header.Size, MaxFileSize),
			"", requestID)
		return
	}

	// Validate file extension
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid file extension: must be .csv", "", requestID)
		return
	}

	// Validate MIME type (use Content-Type from header as fallback)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		// Try to detect from file
		buffer := make([]byte, 512)
		_, err := file.Read(buffer)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Failed to read file for type detection", err.Error(), requestID)
			return
		}
		contentType = http.DetectContentType(buffer)
		// Reset file pointer
		file.Seek(0, 0)
	}

	if !allowedMIMETypes[contentType] {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf("Invalid MIME type: %s (expected text/csv or application/vnd.ms-excel)", contentType),
			"", requestID)
		return
	}

	// Continue in Task 4...
}
```

**Validation**:
```bash
go fmt ./internal/handlers/assets/bulkimport.go
go vet ./internal/handlers/assets/bulkimport.go
go build ./internal/handlers/assets/
```

**Expected**: No errors (function compiles but incomplete)

---

### Task 4: Implement UploadCSV handler (part 2 - CSV parsing and validation)

**File**: `backend/internal/handlers/assets/bulkimport.go`
**Action**: MODIFY (continue UploadCSV function)

**Implementation** (add to end of UploadCSV function):
```go
	// Read all CSV content into memory (safe due to 5MB limit)
	csvContent, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to read CSV file", err.Error(), requestID)
		return
	}

	// Parse CSV
	csvReader := csv.NewReader(bytes.NewReader(csvContent))
	records, err := csvReader.ReadAll()
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid CSV format", err.Error(), requestID)
		return
	}

	// Validate minimum content
	if len(records) < 1 {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"CSV file is empty", "", requestID)
		return
	}

	// Validate headers (using csvutil.ValidateCSVHeaders from util/csv package)
	headers := records[0]
	if err := csvutil.ValidateCSVHeaders(headers); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid CSV headers", err.Error(), requestID)
		return
	}

	// Count data rows (exclude header)
	totalRows := len(records) - 1
	if totalRows == 0 {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"CSV has headers but no data rows", "", requestID)
		return
	}

	// Validate row limit
	if totalRows > MaxRows {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf("Too many rows: %d (max %d)", totalRows, MaxRows),
			"", requestID)
		return
	}

	// Continue in Task 5...
}
```

**Validation**:
```bash
go fmt ./internal/handlers/assets/bulkimport.go
go vet ./internal/handlers/assets/bulkimport.go
go build ./internal/handlers/assets/
```

**Expected**: No errors

---

### Task 5: Implement UploadCSV handler (part 3 - job creation and response)

**File**: `backend/internal/handlers/assets/bulkimport.go`
**Action**: MODIFY (complete UploadCSV function)

**Implementation** (add to end of UploadCSV function):
```go
	// Create job in database
	job, err := h.storage.CreateBulkImportJob(r.Context(), orgID, totalRows)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to create import job", err.Error(), requestID)
		return
	}

	// Build response
	response := bulkimport.UploadResponse{
		Status:    "accepted",
		JobID:     job.ID.String(),
		StatusURL: fmt.Sprintf("/api/v1/assets/bulk/%s", job.ID.String()),
		Message:   fmt.Sprintf("CSV upload accepted. Processing %d rows asynchronously.", totalRows),
	}

	httputil.WriteJSON(w, http.StatusAccepted, response)

	// TODO Phase 2B: Launch goroutine here to process CSV rows
	// go h.processCSVAsync(context.Background(), job.ID, orgID, csvContent, headers)
}
```

**Add required imports at top of file**:
```go
import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	csvutil "github.com/trakrf/platform/backend/internal/util/csv"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)
```

**Validation**:
```bash
go fmt ./internal/handlers/assets/bulkimport.go
go vet ./internal/handlers/assets/bulkimport.go
go build ./internal/handlers/assets/
```

**Expected**: No errors, function complete and compiles

---

### Task 6: Add POST route registration

**File**: `backend/internal/handlers/assets/assets.go`
**Action**: MODIFY (update RegisterRoutes function)

**Implementation**:
Find the `RegisterRoutes` function and add the POST route:

```go
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/assets", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)

		// Bulk import routes
		r.Post("/bulk", h.UploadCSV)
		r.Get("/bulk/{jobId}", h.GetJobStatus)
	})
}
```

**Validation**:
```bash
go fmt ./internal/handlers/assets/assets.go
go vet ./internal/handlers/assets/
go build ./internal/handlers/assets/
```

**Expected**: No errors, routes registered

---

## VALIDATION GATES (MANDATORY)

After EVERY task, run these commands from the `backend` directory:

### Gate 1: Code Formatting
```bash
go fmt ./internal/handlers/assets/ ./internal/models/bulkimport/
```
**Expected**: No changes needed (code already formatted)

### Gate 2: Code Quality
```bash
go vet ./internal/handlers/assets/ ./internal/models/bulkimport/
```
**Expected**: No issues reported

### Gate 3: Build
```bash
go build ./internal/handlers/assets/ ./internal/models/bulkimport/
```
**Expected**: Successful compilation

### Gate 4: Full Test Suite
```bash
go test ./...
```
**Expected**: All tests passing (no regressions)

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
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

**Expected**:
- ✅ All files formatted correctly
- ✅ No vet issues
- ✅ All tests passing (including Phase 2A-1 tests)
- ✅ Successful build

## Risk Assessment

### Risks:

1. **Risk**: Multipart form parsing edge cases (empty files, missing boundaries)
   **Mitigation**: Comprehensive error handling with clear error messages. File size check prevents large payloads.

2. **Risk**: MIME type detection unreliable across different clients
   **Mitigation**: Check both Content-Type header and file extension. Allow multiple MIME types.

3. **Risk**: CSV parsing memory usage with large files
   **Mitigation**: 5MB file size limit + 1000 row limit keeps memory bounded (<10MB).

4. **Risk**: Jobs created but never processed (stuck in "pending")
   **Mitigation**: Acceptable for Phase 2A-2 (documented behavior). Phase 2B will add processing.

## Integration Points

**Phase 2A-1 Dependencies**:
- ✅ `ValidateCSVHeaders()` - Used for CSV header validation
- ✅ CSV helper tests provide confidence

**Storage Layer** (reuse existing):
- `CreateBulkImportJob(ctx, orgID, totalRows)` - Creates job with "pending" status

**HTTP Layer**:
- Multipart form parsing (stdlib)
- JWT claims extraction (existing middleware)

## Manual Testing Plan

After implementation, test manually:

```bash
# 1. Create test CSV file
cat > test-valid.csv << 'EOF'
identifier,name,type,valid_from,valid_to,is_active
TEST-001,Test Asset,device,2024-01-01,2025-01-01,true
TEST-002,Another Asset,device,2024-01-01,2025-01-01,false
EOF

# 2. Get JWT token (use existing auth flow)
TOKEN="your-jwt-token"

# 3. Upload valid CSV
curl -X POST http://localhost:8080/api/v1/assets/bulk \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@test-valid.csv" \
  -v

# Expected: 202 Accepted
# {
#   "status": "accepted",
#   "job_id": "uuid-here",
#   "status_url": "/api/v1/assets/bulk/uuid-here",
#   "message": "CSV upload accepted. Processing 2 rows asynchronously."
# }

# 4. Check job status
curl http://localhost:8080/api/v1/assets/bulk/{job_id} \
  -H "Authorization: Bearer $TOKEN"

# Expected: 200 OK
# {
#   "job_id": "uuid-here",
#   "status": "pending",
#   "total_rows": 2,
#   "processed_rows": 0,
#   "failed_rows": 0,
#   "created_at": "2024-..."
# }

# 5. Test error cases
# File too large (create >5MB file)
# Invalid MIME type (upload .txt file)
# Invalid headers (wrong column names)
# Too many rows (>1000 rows)
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW-MEDIUM)

**Breakdown**:
- 📁 File Impact: Modifying 3 files (bulkimport.go, assets.go, bulkimport model) = 1pt
- 🔗 Subsystems: 3 subsystems (HTTP, Storage, CSV) = 2pts
- 🔢 Task Estimate: 6 subtasks = 1pt
- 📦 Dependencies: 0 new packages (stdlib csv, multipart) = 0pts
- 🆕 Pattern Novelty: Existing patterns (handler structure from assets.go) = 0pts
- **Total**: 4pts

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Building on tested CSV helpers from Phase 2A-1
- ✅ Similar handler patterns exist in assets.go
- ✅ Storage layer already has CreateBulkImportJob method
- ✅ Multipart form handling is standard Go pattern
- ⚠️ MIME type detection may need adjustment based on client behavior
- ✅ Clear validation gates and manual testing plan

**Assessment**: High confidence in successful implementation. The main complexity is multipart form handling, but this is well-documented and standard in Go. CSV parsing is already validated from Phase 2A-1.

**Estimated one-pass success probability**: 85%

**Reasoning**: Straightforward HTTP layer implementation building on validated components. Main risks are edge cases in file upload handling, mitigated by comprehensive error handling and clear limits.

## Next Steps (After Phase 2A-2 Ships)

**Phase 2B: Async Processing (Goroutines)**
- Add goroutine launch at end of UploadCSV
- Implement `processCSVAsync()` function
- Use `ParseCSVDate()` and `ParseCSVBool()` from Phase 2A-1
- Row-by-row asset creation
- Progress updates every 10 rows
- Job status updates (pending → processing → completed/failed)

**Complexity**: 7/10 (MEDIUM-HIGH)
**Estimated subtasks**: 8
