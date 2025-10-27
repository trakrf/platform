# Feature: Bulk Asset Upload via CSV

## Metadata
**Workspace**: backend
**Type**: feature

## Outcome
Users can upload multiple assets at once via CSV file instead of creating them one-by-one through the API.

## User Story
As a system administrator
I want to upload multiple assets from a CSV file
So that I can quickly onboard devices, hardware, or licenses without manually creating each asset

## Context
**Current**: Assets can only be created individually via `POST /api/v1/assets`, requiring one API call per asset. This is inefficient for bulk data entry (e.g., importing 50+ laptops, TVs, or licenses).

**Desired**: Asynchronous bulk upload endpoint `POST /api/v1/assets/bulk` that:
- Accepts a CSV file and validates format synchronously (fast)
- Returns immediately with a job ID (202 Accepted)
- Processes rows asynchronously in a goroutine (non-blocking)
- Tracks progress in a database table
- Provides status endpoint to check progress and errors

**Rationale**:
- **Performance**: Bulk imports can take significant time (100s-1000s of rows). Processing on main thread would block the HTTP handler and cause timeouts.
- **User Experience**: Immediate response with progress tracking is better than long-running request that might timeout
- **Scalability**: Goroutines are lightweight and can handle multiple concurrent bulk uploads efficiently
- **State Persistence**: Job status survives server restarts by storing in PostgreSQL

**Examples**:
- Similar to existing single asset creation at `backend/internal/handlers/assets/assets.go:39`
- Uses existing validation patterns with `go-playground/validator`
- Follows transaction patterns from storage layer

## Technical Requirements

### CSV Format
- **Columns** (user-editable fields only):
  - `identifier` (required, 1-255 chars, unique)
  - `name` (required, 1-255 chars)
  - `type` (required, one of: person, device, asset, inventory, other)
  - `description` (optional, max 1024 chars)
  - `valid_from` (required, format: YYYY-MM-DD)
  - `valid_to` (required, format: YYYY-MM-DD)
  - `is_active` (required, boolean: true/false)

- **Excluded** (system-managed): `id`, `account_id`, `account`, `metadata`, `created_at`, `updated_at`, `deleted_at`

- **Example CSV**:
  ```csv
  identifier,name,type,description,valid_from,valid_to,is_active
  LAPTOP-001,MacBook Air,device,Employee machine,2024-01-01,2026-01-01,true
  TV-023,LG TV 55",device,Meeting room screen,2023-06-01,2027-06-01,true
  ```

### API Endpoints

#### 1. Upload CSV (Async)
- **Method**: `POST /api/v1/assets/bulk`
- **Content-Type**: `multipart/form-data`
- **Form Field**: `file` (CSV file)
- **File Size Limit**: 5MB
- **Response**: `202 Accepted` with job ID

#### 2. Check Job Status
- **Method**: `GET /api/v1/assets/bulk/{jobId}`
- **Response**: Job status with progress, errors, and results

### Request Handling

#### Upload Endpoint (Synchronous Part)
1. Parse multipart form and extract CSV file
2. Validate file format (extension, MIME type, size)
3. Parse CSV headers using `encoding/csv` package
4. Validate headers match expected columns
5. Generate job UUID
6. Create job record in `bulk_import_jobs` table with status `pending`
7. **Return immediately** with `202 Accepted` and job ID
8. Launch goroutine to process CSV rows asynchronously

#### Background Processing (Asynchronous Goroutine)
1. Read and parse all CSV rows
2. Map each row to `asset.CreateAssetRequest` struct
3. Inject `account_id` from job record (captured from user session)
4. Update job status to `processing`
5. For each row:
   - Validate using `validator.Struct()`
   - Attempt to insert into database
   - On error: Record in job errors JSONB field, increment `failed_rows`
   - On success: Increment `processed_rows`
   - Every 10 rows: Update job progress in database
6. After all rows processed:
   - Update job status to `completed` or `failed`
   - Set `completed_at` timestamp

#### Status Endpoint
1. Retrieve job by ID from `bulk_import_jobs` table
2. Verify job belongs to user's account (tenant isolation)
3. Return current status, progress, and errors

### Response Formats

#### Upload Endpoint Responses

**Upload Accepted (202 Accepted)**:
```json
{
  "status": "accepted",
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status_url": "/api/v1/assets/bulk/550e8400-e29b-41d4-a716-446655440000",
  "message": "CSV upload accepted. Processing asynchronously."
}
```

**File Errors (400 Bad Request)**:
```json
{
  "status": "error",
  "message": "Missing or invalid CSV file"
}
```

**Invalid Headers (400 Bad Request)**:
```json
{
  "status": "error",
  "message": "Invalid CSV headers",
  "expected": ["identifier", "name", "type", "description", "valid_from", "valid_to", "is_active"],
  "received": ["id", "name", "type"]
}
```

#### Status Endpoint Responses

**Job Processing (200 OK)**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "total_rows": 100,
  "processed_rows": 45,
  "failed_rows": 2,
  "created_at": "2024-01-15T10:30:00Z",
  "errors": [
    { "row": 12, "field": "identifier", "error": "must be unique" },
    { "row": 23, "field": "valid_from", "error": "invalid date format" }
  ]
}
```

**Job Completed (200 OK)**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "total_rows": 100,
  "processed_rows": 100,
  "failed_rows": 3,
  "successful_rows": 97,
  "created_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:30:12Z",
  "errors": [
    { "row": 12, "field": "identifier", "error": "must be unique" },
    { "row": 23, "field": "valid_from", "error": "invalid date format" },
    { "row": 67, "field": "type", "error": "must be one of: person, device, asset, inventory, other" }
  ]
}
```

**Job Failed (200 OK)**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "failed",
  "total_rows": 100,
  "processed_rows": 15,
  "failed_rows": 15,
  "created_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:30:05Z",
  "error": "Database connection lost during processing"
}
```

**Job Not Found (404 Not Found)**:
```json
{
  "status": "error",
  "message": "Job not found or does not belong to your account"
}
```

### Database Schema

**New Table: `bulk_import_jobs`**
```sql
CREATE TABLE bulk_import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    total_rows INT NOT NULL DEFAULT 0,
    processed_rows INT NOT NULL DEFAULT 0,
    failed_rows INT NOT NULL DEFAULT 0,
    errors JSONB, -- Array of {row: int, field: string, error: string}
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT valid_row_counts CHECK (processed_rows <= total_rows AND failed_rows <= processed_rows)
);

CREATE INDEX idx_bulk_import_jobs_account_id ON bulk_import_jobs(account_id);
CREATE INDEX idx_bulk_import_jobs_status ON bulk_import_jobs(status);
CREATE INDEX idx_bulk_import_jobs_created_at ON bulk_import_jobs(created_at DESC);
```

### Transaction Handling
- **No all-or-nothing transaction**: Rows are processed individually
- Failed rows are recorded in job errors, but successful rows are committed
- This allows partial success (e.g., 97 out of 100 assets created)
- Rationale: For large uploads, rolling back 1000 inserts due to 1 bad row is poor UX
- Each asset insert uses standard `CreateAsset` method (individual transactions)

### Validation Rules
- Reuse existing validation tags from `asset.CreateAssetRequest`
- Additional checks:
  - CSV headers must match expected column names (case-insensitive)
  - `identifier` must be unique within CSV and against existing DB records
  - Date parsing: accept `YYYY-MM-DD` format
  - Boolean parsing: accept `true/false`, `1/0`, `yes/no` (case-insensitive)
  - Empty values map to appropriate defaults or fail validation if required

### Security & Constraints
- Validate MIME type is `text/csv` or `application/vnd.ms-excel`
- Check file extension is `.csv`
- Limit rows to 1000 per upload (configurable constant)
- Sanitize all text inputs before database write
- Use parameterized queries (already handled by pgx)

## Validation Criteria
- [ ] Upload endpoint returns `202 Accepted` immediately with job ID
- [ ] Status endpoint shows real-time progress during processing
- [ ] Successfully processes valid CSV with 10+ assets (all rows succeed)
- [ ] Handles partial failures gracefully (some rows succeed, some fail with errors)
- [ ] Returns detailed validation errors for invalid rows (with row numbers)
- [ ] Rejects files over 5MB with clear error message before creating job
- [ ] Duplicate identifiers within CSV are detected and reported in job errors
- [ ] Job status persists in database (survives server restart)
- [ ] All validation rules from single asset creation apply to bulk upload
- [ ] Tenant isolation: Users can only view their own jobs

## Success Metrics
- [ ] Upload endpoint responds within 200ms (fast synchronous validation)
- [ ] Background processing completes small uploads (<100 rows) within 3 seconds
- [ ] Background processing completes large uploads (500-1000 rows) within 10 seconds
- [ ] Job progress updates every 10 rows during processing
- [ ] Validation errors include row number, field name, and specific error message
- [ ] Partial success works correctly (successful rows inserted, failed rows reported)
- [ ] Multiple concurrent uploads can be processed simultaneously
- [ ] Integration tests cover:
  - Valid upload (all rows succeed)
  - Partial failures (some rows succeed, some fail)
  - Invalid file formats (rejected before job creation)
  - Duplicate identifiers (within CSV and against existing DB records)
  - Job status polling during processing
  - Tenant isolation (users can't access other account's jobs)
- [ ] All existing asset tests remain passing

## References
- Existing asset handler: `backend/internal/handlers/assets/assets.go`
- Asset model: `backend/internal/models/asset/asset.go`
- Storage patterns: `backend/internal/storage/assets.go`
- Validation pattern: Uses `github.com/go-playground/validator/v10`
- CSV parsing: Go standard library `encoding/csv`
