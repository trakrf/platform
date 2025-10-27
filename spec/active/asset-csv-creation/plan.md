# Implementation Plan: Bulk Asset Upload - Phase 1 (Job Tracking Infrastructure)

Generated: 2025-10-27
Specification: spec.md

## Understanding

**Phase 1 Goal**: Build the foundational job tracking infrastructure for async bulk operations, without the complexity of CSV processing or goroutines.

This phase establishes:
- Database table for tracking async job status
- Model layer for `BulkImportJob` with validation
- Storage layer for job CRUD operations
- HTTP handler for job status retrieval
- Integration tests for job lifecycle and tenant isolation

**Why Phase 1 First**: Validates the database schema and job tracking patterns in isolation before adding CSV parsing and async processing complexity. If the schema needs adjustment, we haven't built processing logic on top yet.

**Shippable**: No - This is infrastructure only. Jobs can be created manually in DB for testing, but there's no API endpoint to create jobs yet (that comes in Phase 2).

## Docker-First Development

**All commands in this plan use Docker** to ensure consistency with CI/production environment.

**Quick Start**:
```bash
# Start the full development environment (database + backend)
just dev

# View logs
just dev-logs

# Stop everything
just dev-stop
```

**Common Docker Commands**:
```bash
# Run Go commands inside backend container
docker compose exec backend go fmt ./...
docker compose exec backend go build ./...
docker compose exec backend go test -v ./...

# Access PostgreSQL
just psql

# Run migrations
just db-migrate-up
just db-migrate-status

# Access backend container shell
docker compose exec backend sh
```

**Why Docker?**
- ✅ Matches CI/production environment exactly
- ✅ No Go version mismatches
- ✅ Integrated with database (no manual connection setup)
- ✅ Consistent behavior across all developers
- ✅ Migrations auto-run on startup

## Relevant Files

### Reference Patterns (existing code to follow)

**Storage Layer**:
- `backend/internal/storage/assets.go` (lines 12-154) - Storage CRUD pattern
  - Methods on `Storage` struct
  - `pgx.QueryRow`, `pgx.Query`, `pgx.Exec` usage
  - Error handling with `pgx.ErrNoRows` check
  - Parameterized queries
  - Returns pointers to models

**Models**:
- `backend/internal/models/asset/asset.go` (lines 10-54) - Model struct pattern
  - Separate structs for entity vs requests
  - `json` tags for serialization
  - `validate` tags for validation rules

**Handlers**:
- `backend/internal/handlers/assets/assets.go` (lines 20-147) - Handler pattern
  - Handler struct with storage dependency
  - `middleware.GetRequestID(r.Context())` for request ID
  - `httputil.WriteJSON` and `httputil.WriteJSONError`
  - `chi.URLParam(req, "id")` for path parameters
  - `validator.Struct()` for validation

**Auth/Tenant Isolation**:
- `backend/internal/middleware/middleware.go` (lines 152-158) - `GetUserClaims(r)` function
- `backend/internal/util/jwt/jwt.go` (lines 12-16) - Claims struct with `CurrentAccountID *int`

**Migrations**:
- `database/migrations/000008_assets.up.sql` - Migration pattern
  - Numbered format: `XXXXXX_name.up.sql` / `XXXXXX_name.down.sql`
  - `SET search_path=trakrf,public;`
  - TIMESTAMPTZ for timestamps
  - CREATE INDEX for performance
  - Row Level Security policies
  - COMMENT ON for documentation

**Testing**:
- `backend/internal/handlers/assets/assets_integration_test.go` - Integration test pattern
  - Uses `testify/assert` and `testify/require`
  - `httptest.NewRecorder()` and `httptest.NewRequest()`
  - Manual DB setup for integration tests

### Files to Create

1. `database/migrations/000013_bulk_import_jobs.up.sql` - Migration to create bulk_import_jobs table
2. `database/migrations/000013_bulk_import_jobs.down.sql` - Rollback migration
3. `backend/internal/models/bulkimport/bulkimport.go` - BulkImportJob model
4. `backend/internal/storage/bulk_import_jobs.go` - Storage layer for jobs (CreateJob, GetJobByID, UpdateJobProgress, UpdateJobStatus)
5. `backend/internal/handlers/bulkimport/bulkimport.go` - Handler with GetJobStatus endpoint
6. `backend/internal/handlers/bulkimport/bulkimport_test.go` - Integration tests

### Files to Modify

1. `backend/internal/storage/storage.go` - No changes needed (uses existing Storage struct)
2. `backend/cmd/api/main.go` or router config - Wire up bulk import routes (likely in main or routes file)

## Architecture Impact

- **Subsystems affected**: Database, Storage Layer, API Layer
- **New dependencies**: None (uses existing `pgx`, `validator`, `chi`)
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create Database Migration for bulk_import_jobs Table

**File**: `database/migrations/000013_bulk_import_jobs.up.sql`
**Action**: CREATE
**Pattern**: Reference `database/migrations/000008_assets.up.sql` (migration structure)

**Implementation**:
```sql
SET search_path=trakrf,public;

CREATE TABLE bulk_import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id INT NOT NULL REFERENCES accounts(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    total_rows INT NOT NULL DEFAULT 0,
    processed_rows INT NOT NULL DEFAULT 0,
    failed_rows INT NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb,  -- Empty array by default
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    CONSTRAINT valid_row_counts CHECK (processed_rows <= total_rows AND failed_rows <= processed_rows)
);

-- Indexes for common access patterns
CREATE INDEX idx_bulk_import_jobs_account_id ON bulk_import_jobs(account_id);
CREATE INDEX idx_bulk_import_jobs_status ON bulk_import_jobs(status);
CREATE INDEX idx_bulk_import_jobs_created_at ON bulk_import_jobs(created_at DESC);

-- Row Level Security
ALTER TABLE bulk_import_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY account_isolation_bulk_import_jobs ON bulk_import_jobs
    USING (account_id = current_setting('app.current_account_id')::INT);

-- Documentation
COMMENT ON TABLE bulk_import_jobs IS 'Tracks async bulk import operations for assets';
COMMENT ON COLUMN bulk_import_jobs.status IS 'Job status: pending, processing, completed, failed';
COMMENT ON COLUMN bulk_import_jobs.errors IS 'Array of error objects: [{row: int, field: string, error: string}]';
```

**Also create**: `database/migrations/000013_bulk_import_jobs.down.sql`
```sql
SET search_path=trakrf,public;
DROP TABLE IF EXISTS bulk_import_jobs CASCADE;
```

**Validation**:
```bash
# Start database (if not running)
just db-up

# Run migration
just db-migrate-up

# Verify table exists
just psql
# Then in psql:
\d bulk_import_jobs
\di  # List indexes
\q   # Exit psql
```

---

### Task 2: Create BulkImportJob Model

**File**: `backend/internal/models/bulkimport/bulkimport.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/models/asset/asset.go` (model structure)

**Implementation**:
```go
package bulkimport

import (
	"time"

	"github.com/google/uuid"
)

// ErrorDetail represents a single row error during bulk import
type ErrorDetail struct {
	Row   int    `json:"row"`
	Field string `json:"field,omitempty"`
	Error string `json:"error"`
}

// BulkImportJob represents an async bulk import operation
type BulkImportJob struct {
	ID            uuid.UUID      `json:"job_id"`
	AccountID     int            `json:"account_id"`
	Status        string         `json:"status"` // pending, processing, completed, failed
	TotalRows     int            `json:"total_rows"`
	ProcessedRows int            `json:"processed_rows"`
	FailedRows    int            `json:"failed_rows"`
	Errors        []ErrorDetail  `json:"errors,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
}

// CreateJobRequest is used when creating a new job (Phase 2 will use this)
type CreateJobRequest struct {
	AccountID int `json:"account_id" validate:"required,min=1"`
	TotalRows int `json:"total_rows" validate:"required,min=1,max=1000"`
}

// UpdateJobProgressRequest is used to update job progress
type UpdateJobProgressRequest struct {
	ProcessedRows int           `json:"processed_rows" validate:"required,min=0"`
	FailedRows    int           `json:"failed_rows" validate:"min=0"`
	Errors        []ErrorDetail `json:"errors,omitempty"`
}

// JobStatusResponse is returned by the status endpoint
type JobStatusResponse struct {
	JobID          string        `json:"job_id"`
	Status         string        `json:"status"`
	TotalRows      int           `json:"total_rows"`
	ProcessedRows  int           `json:"processed_rows"`
	FailedRows     int           `json:"failed_rows"`
	SuccessfulRows int           `json:"successful_rows,omitempty"` // Calculated: processed - failed
	CreatedAt      string        `json:"created_at"`
	CompletedAt    string        `json:"completed_at,omitempty"`
	Errors         []ErrorDetail `json:"errors,omitempty"`
}
```

**Validation**:
```bash
# Ensure backend container is running
just dev  # Starts database + backend in Docker

# Run validation inside Docker container
docker compose exec backend go fmt ./...
docker compose exec backend go vet ./...
docker compose exec backend go build ./...

# Or use backend justfile (runs locally, not in Docker)
just backend lint
just backend build
```

---

### Task 3: Create Storage Layer for Bulk Import Jobs

**File**: `backend/internal/storage/bulk_import_jobs.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/storage/assets.go` (storage CRUD methods)

**Implementation**:
```go
package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
)

// CreateBulkImportJob creates a new job record
func (s *Storage) CreateBulkImportJob(ctx context.Context, accountID int, totalRows int) (*bulkimport.BulkImportJob, error) {
	query := `
		INSERT INTO trakrf.bulk_import_jobs (account_id, status, total_rows)
		VALUES ($1, 'pending', $2)
		RETURNING id, account_id, status, total_rows, processed_rows, failed_rows, errors, created_at, completed_at
	`

	var job bulkimport.BulkImportJob
	var errorsJSON []byte

	err := s.pool.QueryRow(ctx, query, accountID, totalRows).Scan(
		&job.ID, &job.AccountID, &job.Status, &job.TotalRows,
		&job.ProcessedRows, &job.FailedRows, &errorsJSON,
		&job.CreatedAt, &job.CompletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create bulk import job: %w", err)
	}

	// Parse errors JSONB
	if err := json.Unmarshal(errorsJSON, &job.Errors); err != nil {
		return nil, fmt.Errorf("failed to parse job errors: %w", err)
	}

	return &job, nil
}

// GetBulkImportJobByID retrieves a job by ID and account_id (tenant isolation)
func (s *Storage) GetBulkImportJobByID(ctx context.Context, jobID uuid.UUID, accountID int) (*bulkimport.BulkImportJob, error) {
	query := `
		SELECT id, account_id, status, total_rows, processed_rows, failed_rows, errors, created_at, completed_at
		FROM trakrf.bulk_import_jobs
		WHERE id = $1 AND account_id = $2
	`

	var job bulkimport.BulkImportJob
	var errorsJSON []byte

	err := s.pool.QueryRow(ctx, query, jobID, accountID).Scan(
		&job.ID, &job.AccountID, &job.Status, &job.TotalRows,
		&job.ProcessedRows, &job.FailedRows, &errorsJSON,
		&job.CreatedAt, &job.CompletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Job not found or doesn't belong to account
		}
		return nil, fmt.Errorf("failed to get bulk import job: %w", err)
	}

	// Parse errors JSONB
	if err := json.Unmarshal(errorsJSON, &job.Errors); err != nil {
		return nil, fmt.Errorf("failed to parse job errors: %w", err)
	}

	return &job, nil
}

// UpdateBulkImportJobProgress updates job progress and errors
func (s *Storage) UpdateBulkImportJobProgress(ctx context.Context, jobID uuid.UUID, processedRows, failedRows int, errors []bulkimport.ErrorDetail) error {
	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return fmt.Errorf("failed to marshal errors: %w", err)
	}

	query := `
		UPDATE trakrf.bulk_import_jobs
		SET processed_rows = $2, failed_rows = $3, errors = $4
		WHERE id = $1
	`

	result, err := s.pool.Exec(ctx, query, jobID, processedRows, failedRows, errorsJSON)
	if err != nil {
		return fmt.Errorf("failed to update job progress: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}

	return nil
}

// UpdateBulkImportJobStatus updates job status and optionally sets completed_at
func (s *Storage) UpdateBulkImportJobStatus(ctx context.Context, jobID uuid.UUID, status string) error {
	query := `
		UPDATE trakrf.bulk_import_jobs
		SET status = $2, completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
		WHERE id = $1
	`

	result, err := s.pool.Exec(ctx, query, jobID, status)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}

	return nil
}
```

**Validation**:
```bash
# Inside Docker container
docker compose exec backend go fmt ./...
docker compose exec backend go vet ./...
docker compose exec backend go build ./...

# Or locally
just backend lint
just backend build
```

---

### Task 4: Create Bulk Import Handler with Status Endpoint

**File**: `backend/internal/handlers/bulkimport/bulkimport.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/handlers/assets/assets.go` (handler structure)

**Implementation**:
```go
package bulkimport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// @Summary Get bulk import job status
// @Description Retrieve the status of a bulk import job by ID
// @Tags bulk-import
// @Accept json
// @Produce json
// @Param jobId path string true "Job ID (UUID)"
// @Success 200 {object} bulkimport.JobStatusResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid job ID"
// @Failure 404 {object} modelerrors.ErrorResponse "Job not found or access denied"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets/bulk/{jobId} [get]
func (h *Handler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())
	jobIDParam := chi.URLParam(r, "jobId")

	// Parse job ID as UUID
	jobID, err := uuid.Parse(jobIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid job ID format", err.Error(), requestID)
		return
	}

	// Extract account_id from JWT claims for tenant isolation
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentAccountID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Missing account context", "", requestID)
		return
	}
	accountID := *claims.CurrentAccountID

	// Retrieve job from storage (with tenant isolation)
	job, err := h.storage.GetBulkImportJobByID(r.Context(), jobID, accountID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to retrieve job", err.Error(), requestID)
		return
	}

	if job == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			"Job not found or does not belong to your account", "", requestID)
		return
	}

	// Build response
	response := bulkimport.JobStatusResponse{
		JobID:         job.ID.String(),
		Status:        job.Status,
		TotalRows:     job.TotalRows,
		ProcessedRows: job.ProcessedRows,
		FailedRows:    job.FailedRows,
		CreatedAt:     job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Errors:        job.Errors,
	}

	if job.Status == "completed" {
		response.SuccessfulRows = job.ProcessedRows - job.FailedRows
	}

	if job.CompletedAt != nil {
		response.CompletedAt = job.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}

// RegisterRoutes registers all bulk import routes
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/assets/bulk/{jobId}", h.GetJobStatus)
}
```

**Validation**:
```bash
# Inside Docker container
docker compose exec backend go fmt ./...
docker compose exec backend go vet ./...
docker compose exec backend go build ./...

# Or locally
just backend lint
just backend build
```

---

### Task 5: Wire Up Routes in Main Router

**File**: Find and modify the main router configuration (likely `backend/cmd/api/main.go` or a routes file)
**Action**: MODIFY
**Pattern**: Reference how `assets.RegisterRoutes(r)` is called

**Implementation**:
```go
// In main.go or routes.go, add:
import "github.com/trakrf/platform/backend/internal/handlers/bulkimport"

// In the setup function:
bulkImportHandler := bulkimport.NewHandler(store)
bulkImportHandler.RegisterRoutes(r) // Register under protected routes with Auth middleware
```

**Validation**:
```bash
# Rebuild backend container to include new routes
docker compose build backend

# Restart backend service
docker compose restart backend

# Verify server starts without errors
docker compose logs backend

# Check if server is responding
curl http://localhost:8080/healthz
```

---

### Task 6: Create Integration Tests

**File**: `backend/internal/handlers/bulkimport/bulkimport_test.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/handlers/assets/assets_integration_test.go`

**Implementation**:
```go
package bulkimport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/bulkimport"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// setupTestRouter creates a test router with minimal middleware
func setupTestRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	handler.RegisterRoutes(r)
	return r
}

func TestGetJobStatus_Success(t *testing.T) {
	// TODO: Replace with actual test DB setup when testutil is available
	// For now, use manual setup or skip
	t.Skip("Requires test database setup - implement with testutil")

	// Expected implementation:
	// 1. Setup test DB
	// 2. Create test account
	// 3. Manually insert a job record into bulk_import_jobs
	// 4. Call GetJobStatus with matching account_id
	// 5. Assert response matches inserted job
}

func TestGetJobStatus_NotFound(t *testing.T) {
	t.Skip("Requires test database setup - implement with testutil")

	// Expected implementation:
	// 1. Setup test DB
	// 2. Create test account
	// 3. Call GetJobStatus with non-existent UUID
	// 4. Assert 404 response
}

func TestGetJobStatus_TenantIsolation(t *testing.T) {
	t.Skip("Requires test database setup - implement with testutil")

	// Expected implementation:
	// 1. Setup test DB
	// 2. Create two test accounts (account1, account2)
	// 3. Insert job for account1
	// 4. Attempt to retrieve job with account2's credentials
	// 5. Assert 404 response (tenant isolation working)
}

func TestGetJobStatus_InvalidUUID(t *testing.T) {
	// This test doesn't need DB - just tests UUID parsing
	store := &storage.Storage{} // Mock or nil storage
	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/bulk/invalid-uuid", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"jobId"},
			Values: []string{"invalid-uuid"},
		},
	}))

	// Add mock JWT claims to context
	mockClaims := &jwt.Claims{
		UserID:           1,
		Email:            "test@example.com",
		CurrentAccountID: func() *int { i := 1; return &i }(),
	}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, mockClaims))

	w := httptest.NewRecorder()
	handler.GetJobStatus(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should reject invalid UUID")
}
```

**Note**: Integration tests reference `testutil` which doesn't exist yet. Options:
- **Option A**: Create minimal test utilities in this phase
- **Option B**: Use manual DB setup in tests (more verbose but works)
- **Option C**: Skip integration tests for Phase 1, add in Phase 2

**Recommended**: Option C - Skip full integration tests for Phase 1, add comprehensive tests in Phase 2 when we have actual CSV upload to test end-to-end.

**Validation**:
```bash
# Run tests inside Docker container
docker compose exec backend go test -v ./internal/handlers/bulkimport/

# Or use backend justfile's Docker test command
just backend test-docker ./internal/handlers/bulkimport

# Also validate linting and build
docker compose exec backend go fmt ./...
docker compose exec backend go vet ./...
docker compose exec backend go build ./...
```

---

### Task 7: Manual End-to-End Testing

**Action**: Manual testing to verify the full stack works
**Pattern**: Use `psql` to insert test job, then call API

**Steps**:

1. **Start Docker development environment**:
   ```bash
   just dev
   # This will:
   # - Start timescaledb container
   # - Run migrations
   # - Start backend container
   ```

2. **Verify containers are running**:
   ```bash
   docker compose ps
   # Should show timescaledb and backend running
   ```

3. **Insert test job via psql**:
   ```bash
   # Open psql shell in Docker
   just psql
   ```

   Then in psql:
   ```sql
   -- Switch to trakrf schema
   SET search_path=trakrf,public;

   -- Insert test job
   INSERT INTO bulk_import_jobs (id, account_id, status, total_rows, processed_rows, failed_rows, errors)
   VALUES (
       'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'::uuid,
       1,  -- Replace with valid account_id from your dev DB
       'processing',
       100,
       45,
       2,
       '[{"row": 12, "field": "identifier", "error": "must be unique"}]'::jsonb
   );

   -- Verify insertion
   SELECT * FROM bulk_import_jobs WHERE id = 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'::uuid;

   \q  -- Exit psql
   ```

4. **Get JWT token** (use existing auth endpoint or generate manually)
   ```bash
   # Example: Login to get token
   curl -X POST http://localhost:8080/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email":"user@example.com","password":"password"}'

   # Extract the token from response
   ```

5. **Call status endpoint**:
   ```bash
   curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
        http://localhost:8080/api/v1/assets/bulk/a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11
   ```

6. **Verify response matches inserted data**:
   - Should return 200 OK
   - Status should be "processing"
   - total_rows: 100, processed_rows: 45, failed_rows: 2
   - errors array should contain the identifier error

7. **Test tenant isolation**:
   - Get JWT for different account_id (or create second test job for different account)
   - Try accessing job with wrong account's JWT
   - Should get 404 Not Found

8. **Test invalid UUID**:
   ```bash
   curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
        http://localhost:8080/api/v1/assets/bulk/invalid-uuid
   # Should return 400 Bad Request
   ```

9. **Test non-existent job**:
   ```bash
   curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
        http://localhost:8080/api/v1/assets/bulk/00000000-0000-0000-0000-000000000000
   # Should return 404 Not Found
   ```

**Validation**:
- Status endpoint returns correct job data
- Tenant isolation works (can't access other account's jobs)
- Invalid UUID returns 400
- Non-existent job returns 404

---

## Risk Assessment

### Risk 1: testutil Package Missing
**Description**: Integration tests in `assets_integration_test.go` reference `testutil` package that doesn't exist.

**Mitigation**:
- Phase 1: Use manual DB testing as documented in Task 7
- Phase 2: Create testutil package or use alternative test setup
- This doesn't block Phase 1 since manual testing validates the functionality

### Risk 2: Router Configuration Unknown
**Description**: Not clear where routes are registered (main.go or separate routes file).

**Mitigation**:
- Search for `assets.RegisterRoutes` in codebase to find pattern
- If not found, add to `main.go` directly
- Document location in implementation

### Risk 3: JWT Claims Might Not Always Have CurrentAccountID
**Description**: `Claims.CurrentAccountID` is `*int` (nullable). If nil, handler will reject request.

**Mitigation**:
- This is correct behavior - require valid account context
- Ensure auth flow always sets CurrentAccountID
- Return 401 if missing (already handled in handler)

## Integration Points

- **Storage**: New methods on existing `Storage` struct (no breaking changes)
- **Routes**: New route group `/api/v1/assets/bulk/{jobId}` (no conflicts)
- **Auth**: Uses existing `middleware.GetUserClaims()` pattern
- **Database**: New table, no changes to existing tables

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change, run validation **inside Docker container** to ensure consistency:

**Gate 1: Syntax & Style**
```bash
# Ensure backend container is running
just dev

# Run formatting and static analysis in Docker
docker compose exec backend go fmt ./...
docker compose exec backend go vet ./...

# Alternative: Run locally (may have version differences)
just backend lint
```
If fails → Fix formatting/vet errors → Re-run → Repeat until pass

**Gate 2: Build**
```bash
# Build inside Docker container
docker compose exec backend go build ./...

# Alternative: Build locally
just backend build
```
If fails → Fix compilation errors → Re-run → Repeat until pass

**Gate 3: Unit Tests** (when tests exist)
```bash
# Run tests inside Docker container (recommended - matches CI environment)
docker compose exec backend go test -v ./...

# Or run specific package tests
just backend test-docker ./internal/handlers/bulkimport

# Alternative: Run locally
just backend test
```
If fails → Fix test failures → Re-run → Repeat until pass

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help
- **Prefer Docker commands** - they match the CI/production environment

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**After each task**:
```bash
# Ensure backend container is running
just dev

# Gate 1: Syntax & Style
docker compose exec backend go fmt ./...
docker compose exec backend go vet ./...

# Gate 2: Build
docker compose exec backend go build ./...

# Gate 3: Unit Tests (if tests exist for that task)
docker compose exec backend go test -v ./internal/handlers/bulkimport
```

**After all tasks complete**:
```bash
# 1. Run full backend validation (inside Docker)
docker compose exec backend go fmt ./...
docker compose exec backend go vet ./...
docker compose exec backend go build ./...
docker compose exec backend go test -v ./...

# 2. Verify migration applied successfully
just db-migrate-status

# 3. Manual testing as per Task 7 (see Task 7 section for detailed steps)

# 4. Verify backend is healthy
curl http://localhost:8080/healthz

# Optional: Run full validation suite locally
just backend validate
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Strong reference patterns found in codebase:
   - Storage: `backend/internal/storage/assets.go`
   - Models: `backend/internal/models/asset/asset.go`
   - Handlers: `backend/internal/handlers/assets/assets.go`
   - Migrations: `database/migrations/000008_assets.up.sql`
✅ All clarifying questions answered
✅ Migration pattern is well-established
✅ No new external dependencies
⚠️ testutil package missing - using manual testing as workaround
⚠️ Router registration location unknown - will search during implementation

**Assessment**: High confidence for successful implementation. The patterns are well-established in the codebase, and Phase 1 scope is focused and testable. The testutil missing is a minor issue resolved by manual testing.

**Estimated one-pass success probability**: 85%

**Reasoning**: Strong existing patterns to follow, simple CRUD operations, clear validation gates. Main risk is minor integration points (router wiring, test setup) but these are discoverable during implementation. Phase 1 is intentionally scoped to avoid CSV/async complexity, reducing risk significantly.

## Next Steps After Phase 1

Once Phase 1 is complete and validated:
1. Ship this PR (job tracking infrastructure)
2. Plan Phase 2: CSV Upload + Async Processing
3. Phase 2 will build on this proven foundation:
   - Create upload handler (POST /api/v1/assets/bulk)
   - Add CSV parsing utilities
   - Implement goroutine background processor
   - Wire up job creation to upload endpoint
   - Comprehensive integration tests with full workflow
