# Phase 2A-2 Outline: Upload Endpoint (Stub Processing)

**Status**: OUTLINE ONLY - Full plan will be generated after Phase 2A-1 ships
**Complexity**: 4/10 (LOW-MEDIUM)
**Estimated subtasks**: 5

## Scope

Add CSV upload endpoint that accepts files, validates them, creates job records, but does NOT yet process rows asynchronously (goroutine launch comes in Phase 2B).

## What Gets Built

### Synchronous Upload Handler
- `POST /api/v1/assets/bulk` endpoint
- Multipart form parsing (`r.FormFile("file")`)
- File validation:
  - Size ≤ 5MB
  - MIME type: `text/csv` or `application/vnd.ms-excel`
  - Extension: `.csv`
- CSV parsing with `encoding/csv`
- Header validation using `ValidateCSVHeaders()` from Phase 2A-1
- Row counting (for job.total_rows)
- Job creation in database (status: "pending")
- Return 202 Accepted with job ID

### Stub Behavior
Jobs created by this endpoint will remain in "pending" status. Phase 2B will add the goroutine that actually processes rows and updates status to "processing" → "completed"/"failed".

## Files to Create/Modify

**Create**:
- None (adding to existing `backend/internal/handlers/assets/bulkimport.go`)

**Modify**:
- `backend/internal/handlers/assets/bulkimport.go` - Add `UploadCSV(w, r)` handler
- `backend/internal/handlers/assets/assets.go` - Add route registration for POST endpoint

## Key Implementation Details

### Handler Signature
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
func (handler *Handler) UploadCSV(w http.ResponseWriter, r *http.Request) {
    // Implementation
}
```

### Response Model (add to models/bulkimport/bulkimport.go)
```go
type UploadResponse struct {
    Status    string `json:"status"`      // "accepted"
    JobID     string `json:"job_id"`      // UUID string
    StatusURL string `json:"status_url"`  // "/api/v1/assets/bulk/{jobId}"
    Message   string `json:"message"`     // User-friendly message
}
```

### Route Registration
Add to `RegisterRoutes()` in assets handler:
```go
r.Post("/api/v1/assets/bulk", h.UploadCSV)
r.Get("/api/v1/assets/bulk/{jobId}", h.GetJobStatus) // Already exists
```

## Validation Approach

### Unit Tests (add to bulkimport_test.go or new file)
- Valid file upload (creates job with correct row count)
- File too large (returns 413)
- Invalid MIME type (returns 400)
- Invalid extension (returns 400)
- Invalid headers (returns 400 with helpful message)
- Missing file (returns 400)

### Manual Testing
```bash
# Valid CSV
curl -X POST http://localhost:8080/api/v1/assets/bulk \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@test-data/valid.csv"

# Expected: 202 Accepted with job_id

# Check status (should be "pending")
curl http://localhost:8080/api/v1/assets/bulk/{job_id} \
  -H "Authorization: Bearer $TOKEN"

# Expected: 200 OK, status="pending", total_rows=N, processed_rows=0
```

## Dependencies on Phase 2A-1

- ✅ `ValidateCSVHeaders()` - Used to validate CSV columns
- ✅ Unit tests provide confidence in CSV parsing
- ⏭️ `ParseCSVDate()` and `ParseCSVBool()` - NOT used yet (Phase 2B)

## Success Criteria

- [ ] Upload endpoint returns 202 Accepted within 200ms
- [ ] Job created in database with status "pending"
- [ ] Job.total_rows matches CSV row count
- [ ] File validation errors return 400 with clear messages
- [ ] File too large returns 413
- [ ] Status endpoint shows job in "pending" state
- [ ] All existing tests still pass
- [ ] Can upload multiple files simultaneously (creates separate jobs)

## Risks

1. **Risk**: Multipart form parsing complexity
   **Mitigation**: Reference existing file upload patterns in Go documentation

2. **Risk**: Large CSV files blocking HTTP handler during counting
   **Mitigation**: 1000 row limit + 5MB limit keeps it fast (<200ms target)

3. **Risk**: MIME type detection unreliable
   **Mitigation**: Also validate file extension as backup check

## Next Phase Dependency

**Phase 2B** will:
- Add goroutine launch at end of UploadCSV handler
- Implement `processCSVAsync()` function
- Use `ParseCSVDate()` and `ParseCSVBool()` from Phase 2A-1
- Update job status from "pending" → "processing" → "completed"/"failed"
