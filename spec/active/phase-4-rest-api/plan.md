# Phase 4A Implementation Plan: REST Foundation + User Management

**Complexity**: 5/10 (Well-scoped, reduced from 11/10)
**Estimated Files**: 9 new, 3 modified
**Estimated Subtasks**: 18

## Context

This plan implements Phase 4A: REST API foundation with user management endpoints only (accounts, users, account_users). Business entity APIs (locations, devices, assets, tags, etc.) are deferred until post-deployment (Phase 4B, after Phase 7).

**Decisions Locked**:
- Framework: chi (minimal, stdlib-compatible)
- Database: pgx/v5 (native PostgreSQL support)
- Validation: go-playground/validator/v10 (declarative)
- Structure: Flat files with domain grouping
- Deletes: Soft delete (set deleted_at timestamp)
- Testing: Unit tests (validation/errors) + Integration tests (happy paths)
- Pagination: Offset/limit (simple, optimize later if needed)

**Existing Patterns Identified**:
- Logging: slog with JSON handler
- HTTP: stdlib http.Server with timeouts
- Middleware: Simple wrapper functions
- Testing: Table-driven with httptest
- Config: 12-factor environment variables
- Database: Hashed IDs via triggers, RLS enabled, JSONB fields

---

## Task Breakdown

### Task 1: Add Dependencies (go.mod)

**Files Modified**: `backend/go.mod`

Add three core dependencies to currently stdlib-only backend:

```bash
go get github.com/go-chi/chi/v5@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/go-playground/validator/v10@latest
```

**Validation**:
```bash
cd backend && go mod tidy
cd backend && go mod verify
```

**Estimated Complexity**: 1/10 (simple dependency addition)

---

### Task 2: Database Connection Pool (database.go)

**Files Created**: `backend/database.go`

Set up pgx connection pool with proper configuration and health checks.

**Key Requirements**:
- Use PG_URL environment variable (already set in docker-compose.yaml)
- Configure pool: max connections, idle connections, max lifetime
- Implement Ping() for readiness checks
- Handle graceful shutdown
- Add database logging integration

**Code Template**:
```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

// initDB initializes the database connection pool
func initDB(ctx context.Context) error {
    pgURL := os.Getenv("PG_URL")
    if pgURL == "" {
        return fmt.Errorf("PG_URL environment variable not set")
    }

    config, err := pgxpool.ParseConfig(pgURL)
    if err != nil {
        return fmt.Errorf("failed to parse PG_URL: %w", err)
    }

    // Configure pool
    config.MaxConns = 25
    config.MinConns = 5
    config.MaxConnLifetime = time.Hour
    config.MaxConnIdleTime = 30 * time.Minute
    config.HealthCheckPeriod = time.Minute

    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        return fmt.Errorf("failed to create connection pool: %w", err)
    }

    // Verify connection
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return fmt.Errorf("failed to ping database: %w", err)
    }

    db = pool
    slog.Info("Database connection pool initialized",
        "max_conns", config.MaxConns,
        "min_conns", config.MinConns)

    return nil
}

// closeDB gracefully closes the database connection pool
func closeDB() {
    if db != nil {
        db.Close()
        slog.Info("Database connection pool closed")
    }
}
```

**Integration Points**:
- Call `initDB()` in `main()` before starting HTTP server
- Call `closeDB()` in shutdown sequence
- Use `db.Ping()` in `readyzHandler()` (update health.go)

**Validation**:
- Server starts without errors
- `curl localhost:8080/readyz` returns 200 when DB is up
- `curl localhost:8080/readyz` returns 503 when DB is down
- Logs show pool initialization

**Estimated Complexity**: 2/10 (straightforward pool setup)

---

### Task 3: Update Health Checks (health.go)

**Files Modified**: `backend/health.go`

Update `readyzHandler()` to include database connectivity check (currently has TODO comment).

**Changes**:
```go
func readyzHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Database connectivity check
    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    if err := db.Ping(ctx); err != nil {
        slog.Error("Readiness check failed", "error", err)
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("database unavailable"))
        return
    }

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}
```

Also update `HealthResponse` struct to include database status:
```go
type HealthResponse struct {
    Status    string    `json:"status"`
    Version   string    `json:"version"`
    Timestamp time.Time `json:"timestamp"`
    Uptime    string    `json:"uptime"`
    Database  string    `json:"database"` // NEW
}
```

**Validation**:
- Existing health tests still pass
- New database check works (test with DB up/down)

**Estimated Complexity**: 1/10 (minor update)

---

### Task 4: Common Middleware (middleware.go)

**Files Created**: `backend/middleware.go`

Implement middleware stack: Request ID, CORS, Recovery, Content-Type enforcement.

**Required Middleware**:

1. **Request ID**: Generate or extract X-Request-ID header
2. **Recovery**: Catch panics, return 500 with request ID
3. **CORS**: Allow frontend origin (configure via env var)
4. **Content-Type**: Enforce application/json for POST/PUT
5. **Logging**: Enhance existing loggingMiddleware with request ID

**Code Template**:
```go
package main

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "log/slog"
    "net/http"
    "time"
)

type contextKey string

const requestIDKey contextKey = "requestID"

// requestIDMiddleware generates or extracts request ID
func requestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = generateRequestID()
        }

        w.Header().Set("X-Request-ID", requestID)
        ctx := context.WithValue(r.Context(), requestIDKey, requestID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func generateRequestID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return hex.EncodeToString(b)
}

// recoveryMiddleware catches panics and returns 500
func recoveryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                requestID := getRequestID(r.Context())
                slog.Error("Panic recovered",
                    "error", err,
                    "request_id", requestID,
                    "path", r.URL.Path,
                    "method", r.Method)

                writeJSONError(w, r, http.StatusInternalServerError,
                    "internal_error", "Internal server error", "")
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// corsMiddleware handles CORS headers
func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // TODO: Make origin configurable via BACKEND_CORS_ORIGIN env var
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
        w.Header().Set("Access-Control-Max-Age", "3600")

        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusNoContent)
            return
        }

        next.ServeHTTP(w, r)
    })
}

// contentTypeMiddleware enforces JSON for write operations
func contentTypeMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == "POST" || r.Method == "PUT" {
            ct := r.Header.Get("Content-Type")
            if ct != "application/json" && ct != "application/json; charset=utf-8" {
                writeJSONError(w, r, http.StatusUnsupportedMediaType,
                    "invalid_content_type", "Content-Type must be application/json", "")
                return
            }
        }
        next.ServeHTTP(w, r)
    })
}

func getRequestID(ctx context.Context) string {
    if reqID, ok := ctx.Value(requestIDKey).(string); ok {
        return reqID
    }
    return ""
}
```

Update existing `loggingMiddleware` to include request ID:
```go
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)

        requestID := getRequestID(r.Context())
        slog.Info("Request",
            "method", r.Method,
            "path", r.URL.Path,
            "duration", time.Since(start),
            "request_id", requestID,
        )
    })
}
```

**Validation**:
- OPTIONS requests return 204
- Panics return 500 with request ID
- POST without Content-Type returns 415
- Response includes X-Request-ID header

**Estimated Complexity**: 3/10 (multiple middleware, error handling)

---

### Task 5: Error Response Helper (errors.go)

**Files Created**: `backend/errors.go`

Implement RFC 7807 Problem Details error response format.

**Code Template**:
```go
package main

import (
    "encoding/json"
    "log/slog"
    "net/http"
)

// ErrorType represents the type of error
type ErrorType string

const (
    ErrValidation   ErrorType = "validation_error"
    ErrNotFound     ErrorType = "not_found"
    ErrConflict     ErrorType = "conflict"
    ErrInternal     ErrorType = "internal_error"
    ErrBadRequest   ErrorType = "bad_request"
    ErrUnauthorized ErrorType = "unauthorized" // Phase 5
)

// ErrorResponse implements RFC 7807 Problem Details
type ErrorResponse struct {
    Error struct {
        Type      string `json:"type"`
        Title     string `json:"title"`
        Status    int    `json:"status"`
        Detail    string `json:"detail"`
        Instance  string `json:"instance"`
        RequestID string `json:"request_id"`
    } `json:"error"`
}

// writeJSONError writes a standardized error response
func writeJSONError(w http.ResponseWriter, r *http.Request, status int, errType ErrorType, title, detail string) {
    requestID := getRequestID(r.Context())

    resp := ErrorResponse{}
    resp.Error.Type = string(errType)
    resp.Error.Title = title
    resp.Error.Status = status
    resp.Error.Detail = detail
    resp.Error.Instance = r.URL.Path
    resp.Error.RequestID = requestID

    // Log errors based on severity
    if status >= 500 {
        slog.Error("Error response",
            "status", status,
            "type", errType,
            "detail", detail,
            "request_id", requestID,
            "path", r.URL.Path)
    } else {
        slog.Info("Client error",
            "status", status,
            "type", errType,
            "request_id", requestID,
            "path", r.URL.Path)
    }

    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(resp)
}

// writeJSON writes a successful JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) error {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)
    return json.NewEncoder(w).Encode(data)
}
```

**Validation**:
- Error responses match RFC 7807 format
- Request ID included in all errors
- Appropriate logging based on status code

**Estimated Complexity**: 2/10 (straightforward helpers)

---

### Task 6: Accounts API (accounts.go)

**Files Created**: `backend/accounts.go`, `backend/accounts_test.go`

Implement full CRUD for accounts table.

**Endpoints**:
```
GET    /api/v1/accounts          # List accounts (paginated)
GET    /api/v1/accounts/:id      # Get account by ID
POST   /api/v1/accounts          # Create account
PUT    /api/v1/accounts/:id      # Update account
DELETE /api/v1/accounts/:id      # Soft delete account
```

**Data Structures**:
```go
package main

import (
    "context"
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-playground/validator/v10"
)

// Account represents an account entity
type Account struct {
    ID               int       `json:"id"`
    Name             string    `json:"name"`
    Domain           string    `json:"domain"`
    Status           string    `json:"status"`
    SubscriptionTier string    `json:"subscription_tier"`
    MaxUsers         int       `json:"max_users"`
    MaxStorageGB     int       `json:"max_storage_gb"`
    Settings         any       `json:"settings"`         // JSONB
    Metadata         any       `json:"metadata"`         // JSONB
    BillingEmail     string    `json:"billing_email"`
    TechnicalEmail   *string   `json:"technical_email"`
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}

// CreateAccountRequest for POST /api/v1/accounts
type CreateAccountRequest struct {
    Name             string  `json:"name" validate:"required,min=1,max=255"`
    Domain           string  `json:"domain" validate:"required,hostname"`
    BillingEmail     string  `json:"billing_email" validate:"required,email"`
    TechnicalEmail   *string `json:"technical_email" validate:"omitempty,email"`
    SubscriptionTier string  `json:"subscription_tier" validate:"omitempty,oneof=free basic premium god-mode"`
    MaxUsers         *int    `json:"max_users" validate:"omitempty,min=1"`
    MaxStorageGB     *int    `json:"max_storage_gb" validate:"omitempty,min=1"`
}

// UpdateAccountRequest for PUT /api/v1/accounts/:id
type UpdateAccountRequest struct {
    Name           *string `json:"name" validate:"omitempty,min=1,max=255"`
    BillingEmail   *string `json:"billing_email" validate:"omitempty,email"`
    TechnicalEmail *string `json:"technical_email" validate:"omitempty,email"`
    Status         *string `json:"status" validate:"omitempty,oneof=active inactive suspended"`
    MaxUsers       *int    `json:"max_users" validate:"omitempty,min=1"`
    MaxStorageGB   *int    `json:"max_storage_gb" validate:"omitempty,min=1"`
}

// AccountListResponse for GET /api/v1/accounts
type AccountListResponse struct {
    Data       []Account  `json:"data"`
    Pagination Pagination `json:"pagination"`
}

// Pagination metadata
type Pagination struct {
    Page    int `json:"page"`
    PerPage int `json:"per_page"`
    Total   int `json:"total"`
}

var validate = validator.New()

// registerAccountRoutes registers all account endpoints
func registerAccountRoutes(r chi.Router) {
    r.Get("/api/v1/accounts", listAccountsHandler)
    r.Get("/api/v1/accounts/{id}", getAccountHandler)
    r.Post("/api/v1/accounts", createAccountHandler)
    r.Put("/api/v1/accounts/{id}", updateAccountHandler)
    r.Delete("/api/v1/accounts/{id}", deleteAccountHandler)
}
```

**Repository Pattern** (in same file for flat structure):
```go
// AccountRepository handles database operations for accounts
type AccountRepository struct {
    db *pgxpool.Pool
}

func (r *AccountRepository) List(ctx context.Context, limit, offset int) ([]Account, int, error) {
    // Query with soft delete filter: WHERE deleted_at IS NULL
    query := `
        SELECT id, name, domain, status, subscription_tier, max_users, max_storage_gb,
               settings, metadata, billing_email, technical_email, created_at, updated_at
        FROM trakrf.accounts
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT $1 OFFSET $2
    `

    rows, err := r.db.Query(ctx, query, limit, offset)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to query accounts: %w", err)
    }
    defer rows.Close()

    var accounts []Account
    for rows.Next() {
        var a Account
        err := rows.Scan(&a.ID, &a.Name, &a.Domain, &a.Status, &a.SubscriptionTier,
            &a.MaxUsers, &a.MaxStorageGB, &a.Settings, &a.Metadata,
            &a.BillingEmail, &a.TechnicalEmail, &a.CreatedAt, &a.UpdatedAt)
        if err != nil {
            return nil, 0, fmt.Errorf("failed to scan account: %w", err)
        }
        accounts = append(accounts, a)
    }

    // Get total count
    var total int
    err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM trakrf.accounts WHERE deleted_at IS NULL").Scan(&total)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to count accounts: %w", err)
    }

    return accounts, total, nil
}

func (r *AccountRepository) GetByID(ctx context.Context, id int) (*Account, error) {
    query := `
        SELECT id, name, domain, status, subscription_tier, max_users, max_storage_gb,
               settings, metadata, billing_email, technical_email, created_at, updated_at
        FROM trakrf.accounts
        WHERE id = $1 AND deleted_at IS NULL
    `

    var a Account
    err := r.db.QueryRow(ctx, query, id).Scan(
        &a.ID, &a.Name, &a.Domain, &a.Status, &a.SubscriptionTier,
        &a.MaxUsers, &a.MaxStorageGB, &a.Settings, &a.Metadata,
        &a.BillingEmail, &a.TechnicalEmail, &a.CreatedAt, &a.UpdatedAt)

    if err != nil {
        if err == pgx.ErrNoRows {
            return nil, nil // Not found
        }
        return nil, fmt.Errorf("failed to get account: %w", err)
    }

    return &a, nil
}

func (r *AccountRepository) Create(ctx context.Context, req CreateAccountRequest) (*Account, error) {
    // Database generates ID via trigger, use RETURNING
    query := `
        INSERT INTO trakrf.accounts (name, domain, billing_email, technical_email, subscription_tier, max_users, max_storage_gb)
        VALUES ($1, $2, $3, $4, COALESCE($5, 'free'), COALESCE($6, 5), COALESCE($7, 1))
        RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
                  settings, metadata, billing_email, technical_email, created_at, updated_at
    `

    var a Account
    err := r.db.QueryRow(ctx, query,
        req.Name, req.Domain, req.BillingEmail, req.TechnicalEmail,
        req.SubscriptionTier, req.MaxUsers, req.MaxStorageGB,
    ).Scan(&a.ID, &a.Name, &a.Domain, &a.Status, &a.SubscriptionTier,
        &a.MaxUsers, &a.MaxStorageGB, &a.Settings, &a.Metadata,
        &a.BillingEmail, &a.TechnicalEmail, &a.CreatedAt, &a.UpdatedAt)

    if err != nil {
        // Check for unique constraint violation (duplicate domain)
        if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
            return nil, ErrDuplicateDomain
        }
        return nil, fmt.Errorf("failed to create account: %w", err)
    }

    return &a, nil
}

func (r *AccountRepository) Update(ctx context.Context, id int, req UpdateAccountRequest) (*Account, error) {
    // Build dynamic UPDATE query based on non-nil fields
    // (Implementation details for partial updates)
    // ...
}

func (r *AccountRepository) SoftDelete(ctx context.Context, id int) error {
    query := `UPDATE trakrf.accounts SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
    result, err := r.db.Exec(ctx, query, id)
    if err != nil {
        return fmt.Errorf("failed to delete account: %w", err)
    }

    if result.RowsAffected() == 0 {
        return ErrNotFound
    }

    return nil
}

var (
    ErrNotFound        = fmt.Errorf("account not found")
    ErrDuplicateDomain = fmt.Errorf("domain already exists")
)
```

**Handlers**:
```go
var accountRepo = &AccountRepository{db: db} // Initialize after db setup

func listAccountsHandler(w http.ResponseWriter, r *http.Request) {
    // Parse pagination params
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

    if page < 1 {
        page = 1
    }
    if perPage < 1 || perPage > 100 {
        perPage = 20
    }

    offset := (page - 1) * perPage

    accounts, total, err := accountRepo.List(r.Context(), perPage, offset)
    if err != nil {
        writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to list accounts", "")
        return
    }

    resp := AccountListResponse{
        Data: accounts,
        Pagination: Pagination{
            Page:    page,
            PerPage: perPage,
            Total:   total,
        },
    }

    writeJSON(w, http.StatusOK, resp)
}

func getAccountHandler(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
        return
    }

    account, err := accountRepo.GetByID(r.Context(), id)
    if err != nil {
        writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to get account", "")
        return
    }

    if account == nil {
        writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "Account not found", "")
        return
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{"data": account})
}

func createAccountHandler(w http.ResponseWriter, r *http.Request) {
    var req CreateAccountRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
        return
    }

    if err := validate.Struct(req); err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
        return
    }

    account, err := accountRepo.Create(r.Context(), req)
    if err != nil {
        if err == ErrDuplicateDomain {
            writeJSONError(w, r, http.StatusConflict, ErrConflict, "Domain already exists", "")
            return
        }
        writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to create account", "")
        return
    }

    w.Header().Set("Location", fmt.Sprintf("/api/v1/accounts/%d", account.ID))
    writeJSON(w, http.StatusCreated, map[string]interface{}{"data": account})
}

func updateAccountHandler(w http.ResponseWriter, r *http.Request) {
    // Similar pattern to create
}

func deleteAccountHandler(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
        return
    }

    if err := accountRepo.SoftDelete(r.Context(), id); err != nil {
        if err == ErrNotFound {
            writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "Account not found", "")
            return
        }
        writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to delete account", "")
        return
    }

    w.WriteHeader(http.StatusNoContent)
}
```

**Unit Tests** (`accounts_test.go`):
- Test validation errors (missing name, invalid email, etc.)
- Test malformed JSON
- Test invalid account IDs

**Validation**:
- All 5 endpoints work correctly
- Validation errors return 400 with field details
- Duplicate domains return 409
- Soft delete sets deleted_at timestamp
- Pagination works correctly

**Estimated Complexity**: 4/10 (first full CRUD, establishes patterns)

---

### Task 7: Users API (users.go)

**Files Created**: `backend/users.go`, `backend/users_test.go`

Implement full CRUD for users table (similar pattern to accounts).

**Endpoints**:
```
GET    /api/v1/users          # List users (paginated)
GET    /api/v1/users/:id      # Get user by ID
POST   /api/v1/users          # Create user
PUT    /api/v1/users/:id      # Update user
DELETE /api/v1/users/:id      # Soft delete user
```

**Key Differences from Accounts**:
- Email field has unique constraint (handle duplicate errors)
- password_hash field (Phase 5 will handle hashing, for now accept plain string)
- RLS policy: `user_isolation_users` (app.current_user_id session variable)

**Data Structures**:
```go
type User struct {
    ID           int       `json:"id"`
    Email        string    `json:"email"`
    Name         string    `json:"name"`
    PasswordHash string    `json:"-"` // Never expose in JSON
    LastLoginAt  *time.Time `json:"last_login_at"`
    Settings     any       `json:"settings"`
    Metadata     any       `json:"metadata"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

type CreateUserRequest struct {
    Email        string `json:"email" validate:"required,email"`
    Name         string `json:"name" validate:"required,min=1,max=255"`
    PasswordHash string `json:"password_hash" validate:"required,min=8"` // Temporary, Phase 5 will hash
}

type UpdateUserRequest struct {
    Name  *string `json:"name" validate:"omitempty,min=1,max=255"`
    Email *string `json:"email" validate:"omitempty,email"`
}
```

**Validation**:
- Email uniqueness enforced (return 409 on duplicate)
- Soft delete works
- RLS policy verified (Phase 5 will test isolation)

**Estimated Complexity**: 3/10 (follows accounts pattern)

---

### Task 8: AccountUsers API (account_users.go)

**Files Created**: `backend/account_users.go`, `backend/account_users_test.go`

Implement CRUD for account_users junction table (RBAC foundation).

**Endpoints** (nested under accounts):
```
GET    /api/v1/accounts/:account_id/users           # List account members
POST   /api/v1/accounts/:account_id/users           # Add user to account
PUT    /api/v1/accounts/:account_id/users/:user_id  # Update role/status
DELETE /api/v1/accounts/:account_id/users/:user_id  # Remove user from account (soft delete)
```

**Key Differences**:
- Composite primary key (account_id, user_id)
- CHECK constraints on role and status (validate against allowed values)
- Soft delete via deleted_at

**Data Structures**:
```go
type AccountUser struct {
    AccountID   int        `json:"account_id"`
    UserID      int        `json:"user_id"`
    Role        string     `json:"role"`
    Status      string     `json:"status"`
    LastLoginAt *time.Time `json:"last_login_at"`
    Settings    any        `json:"settings"`
    Metadata    any        `json:"metadata"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
}

type AddUserToAccountRequest struct {
    UserID int    `json:"user_id" validate:"required"`
    Role   string `json:"role" validate:"required,oneof=owner admin member readonly"`
    Status string `json:"status" validate:"omitempty,oneof=active inactive suspended invited"`
}

type UpdateAccountUserRequest struct {
    Role   *string `json:"role" validate:"omitempty,oneof=owner admin member readonly"`
    Status *string `json:"status" validate:"omitempty,oneof=active inactive suspended invited"`
}
```

**Special Handling**:
- Validate that account_id and user_id exist before INSERT
- Handle duplicate membership (return 409)
- Return user details with role (JOIN query)

**Validation**:
- Can add user to account
- Can update role/status
- Can list account members
- Can remove user from account (soft delete)
- Validation enforces role/status constraints

**Estimated Complexity**: 3/10 (similar to users, with JOIN complexity)

---

### Task 9: Integration Tests (integration_test.go)

**Files Created**: `backend/integration_test.go`

Write integration tests for happy paths with real database.

**Test Setup**:
```go
// +build integration

package main

import (
    "context"
    "os"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
)

var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
    // Setup: Connect to test database
    ctx := context.Background()
    var err error

    // Use test database URL
    testDB, err = pgxpool.New(ctx, os.Getenv("PG_URL"))
    if err != nil {
        panic("failed to connect to test database: " + err.Error())
    }

    // Run migrations (ensure test DB is up-to-date)
    // ...

    // Run tests
    code := m.Run()

    // Teardown: Close connection
    testDB.Close()

    os.Exit(code)
}

func TestAccountsIntegration(t *testing.T) {
    ctx := context.Background()
    repo := &AccountRepository{db: testDB}

    t.Run("Create and Get Account", func(t *testing.T) {
        // Create account
        req := CreateAccountRequest{
            Name:         "Test Corp",
            Domain:       "test.example.com",
            BillingEmail: "billing@test.example.com",
        }

        account, err := repo.Create(ctx, req)
        if err != nil {
            t.Fatalf("Create failed: %v", err)
        }

        if account.ID == 0 {
            t.Error("Expected non-zero ID")
        }

        // Get account
        fetched, err := repo.GetByID(ctx, account.ID)
        if err != nil {
            t.Fatalf("GetByID failed: %v", err)
        }

        if fetched.Name != req.Name {
            t.Errorf("Name = %q, want %q", fetched.Name, req.Name)
        }

        // Cleanup
        repo.SoftDelete(ctx, account.ID)
    })

    // Additional tests: List, Update, Delete, duplicate domain...
}

func TestUsersIntegration(t *testing.T) {
    // Similar pattern for users
}

func TestAccountUsersIntegration(t *testing.T) {
    // Similar pattern for account_users
}
```

**Run via**:
```bash
just db-up
just db-migrate-up
cd backend && go test -tags=integration -v ./...
```

**Validation**:
- All integration tests pass
- Database constraints enforced
- Hashed IDs generated correctly
- Soft deletes work

**Estimated Complexity**: 3/10 (straightforward CRUD tests)

---

### Task 10: Update main.go

**Files Modified**: `backend/main.go`

Integrate chi router, middleware stack, database, and route registration.

**Changes**:
```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/go-chi/chi/v5"
)

func main() {
    startTime = time.Now()

    // Setup logging
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: getLogLevel(),
    }))
    slog.SetDefault(logger)

    // Initialize database
    ctx := context.Background()
    if err := initDB(ctx); err != nil {
        slog.Error("Failed to initialize database", "error", err)
        os.Exit(1)
    }
    defer closeDB()

    // Config
    port := os.Getenv("BACKEND_PORT")
    if port == "" {
        port = "8080"
    }

    // Setup chi router
    r := chi.NewRouter()

    // Middleware stack (order matters!)
    r.Use(requestIDMiddleware)
    r.Use(loggingMiddleware)
    r.Use(recoveryMiddleware)
    r.Use(corsMiddleware)
    r.Use(contentTypeMiddleware)

    // Health endpoints (no middleware)
    r.Get("/healthz", healthzHandler)
    r.Get("/readyz", readyzHandler)
    r.Get("/health", healthHandler)

    // API routes
    registerAccountRoutes(r)
    registerUserRoutes(r)
    registerAccountUserRoutes(r)

    // HTTP server
    server := &http.Server{
        Addr:         ":" + port,
        Handler:      r,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    // Start server
    go func() {
        slog.Info("Server starting", "port", port, "version", version)
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("Server failed", "error", err)
            os.Exit(1)
        }
    }()

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    slog.Info("Shutting down gracefully...")
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(shutdownCtx); err != nil {
        slog.Error("Shutdown error", "error", err)
    }

    slog.Info("Server stopped")
}

func getLogLevel() slog.Level {
    level := os.Getenv("BACKEND_LOG_LEVEL")
    switch level {
    case "debug":
        return slog.LevelDebug
    case "warn":
        return slog.LevelWarn
    case "error":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}
```

**Validation**:
- Server starts successfully
- All routes registered
- Middleware executes in correct order
- Database connection established

**Estimated Complexity**: 2/10 (integration work)

---

### Task 11: Update .env.example

**Files Modified**: `.env.example`

Add any new environment variables (if needed).

**Current variables** (already set):
- POSTGRES_PASSWORD
- POSTGRES_DB
- BACKEND_PORT
- BACKEND_LOG_LEVEL
- PG_URL

**Potential additions** (optional for Phase 4A):
```bash
# CORS Configuration (optional, defaults to * for now)
BACKEND_CORS_ORIGIN=*
```

**Validation**: N/A (documentation only)

**Estimated Complexity**: 0/10

---

### Task 12: Manual Testing & Validation

**Files Created**: None (testing only)

Manually test all endpoints using curl or HTTP client.

**Test Script** (create as `test-api.sh`):
```bash
#!/bin/bash
set -e

BASE_URL="http://localhost:8080"

echo "==> Testing Health Endpoints"
curl -s $BASE_URL/health | jq .
curl -s $BASE_URL/healthz
curl -s $BASE_URL/readyz

echo -e "\n==> Testing Accounts API"

# Create account
ACCOUNT=$(curl -s -X POST $BASE_URL/api/v1/accounts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Corp",
    "domain": "test.example.com",
    "billing_email": "billing@test.example.com",
    "subscription_tier": "basic"
  }')
echo "Created: $ACCOUNT"
ACCOUNT_ID=$(echo $ACCOUNT | jq -r '.data.id')

# Get account
curl -s $BASE_URL/api/v1/accounts/$ACCOUNT_ID | jq .

# List accounts
curl -s "$BASE_URL/api/v1/accounts?page=1&per_page=10" | jq .

# Update account
curl -s -X PUT $BASE_URL/api/v1/accounts/$ACCOUNT_ID \
  -H "Content-Type: application/json" \
  -d '{"name": "Test Corp Updated"}' | jq .

# Delete account
curl -s -X DELETE $BASE_URL/api/v1/accounts/$ACCOUNT_ID

echo -e "\n==> Testing Users API"
# Similar tests for users...

echo -e "\n==> Testing AccountUsers API"
# Similar tests for account_users...

echo -e "\n✅ All manual tests passed"
```

**Run via**:
```bash
just dev  # Start services
chmod +x test-api.sh
./test-api.sh
```

**Validation Checklist**:
- ✅ All endpoints return correct status codes
- ✅ Validation errors are clear and actionable
- ✅ Pagination works correctly
- ✅ Soft deletes work (verify in database)
- ✅ Duplicate domains/emails return 409
- ✅ Invalid IDs return 404
- ✅ Request IDs present in all responses

**Estimated Complexity**: 2/10 (manual testing)

---

### Task 13: Run Full Validation

**Files Modified**: None

Run all validation commands to ensure everything works.

**Commands**:
```bash
just backend-lint      # go fmt, go vet
just backend-test      # Unit tests
just backend-build     # Compile binary
just backend           # All of above

# Integration tests
just dev               # Start services
cd backend && go test -tags=integration -v ./...

# Full stack validation
just validate
```

**Success Criteria**:
- ✅ `go fmt` changes nothing (code formatted)
- ✅ `go vet` reports no issues
- ✅ All unit tests pass
- ✅ All integration tests pass
- ✅ Build succeeds without warnings
- ✅ Docker services healthy

**Estimated Complexity**: 1/10 (verification only)

---

## Risk Assessment

**Low Risk**:
- Dependency addition (chi, pgx, validator are mature)
- Database connection pool (well-documented pgx pattern)
- Middleware implementation (straightforward HTTP wrappers)
- Error handling (RFC 7807 is standard)

**Medium Risk**:
- Integration tests requiring test database setup
  - Mitigation: Use same docker-compose.yaml, run migrations before tests
- Soft delete queries (easy to miss WHERE deleted_at IS NULL)
  - Mitigation: Create helper function for common WHERE clauses
- Hashed ID handling (database-generated, not Go-controlled)
  - Mitigation: Always use RETURNING clause in INSERT queries

**High Risk**: None for Phase 4A scope

---

## Success Metrics

All metrics from spec.md, scoped to Phase 4A entities (accounts, users, account_users):

**Functional** (per entity × 3 = 9 checks):
- ✅ Can create via POST
- ✅ Can retrieve by ID via GET
- ✅ Can list all via GET (paginated)
- ✅ Can update via PUT
- ✅ Can soft delete via DELETE
- ✅ Validation errors return 400 with details
- ✅ Non-existent resources return 404
- ✅ Duplicate constraints return 409 (accounts.domain, users.email)
- ✅ AccountUsers enforces role/status CHECK constraints

**Technical**:
- ✅ All endpoints return proper JSON with consistent format
- ✅ Error responses include request_id for tracing
- ✅ Database connection pool configured and monitored
- ✅ Middleware stack operational (request ID, logger, recovery, CORS)
- ✅ All tests pass (unit + integration)
- ✅ `just backend` validates successfully
- ✅ Air hot-reload works with chi routes

**Performance**:
- ✅ List endpoints handle 1000+ records without pagination issues
- ✅ Response times < 100ms for simple queries (local dev)
- ✅ No N+1 query issues (verified via logging)

---

## File Summary

**New Files** (9):
1. `backend/database.go` - Database connection pool
2. `backend/middleware.go` - Request ID, CORS, recovery, content-type
3. `backend/errors.go` - RFC 7807 error helpers
4. `backend/accounts.go` - Accounts CRUD + repository
5. `backend/accounts_test.go` - Accounts unit tests
6. `backend/users.go` - Users CRUD + repository
7. `backend/users_test.go` - Users unit tests
8. `backend/account_users.go` - AccountUsers CRUD + repository
9. `backend/account_users_test.go` - AccountUsers unit tests
10. `backend/integration_test.go` - Integration tests (all entities)

**Modified Files** (3):
1. `backend/go.mod` - Add chi, pgx, validator dependencies
2. `backend/main.go` - Integrate chi, middleware, database, routes
3. `backend/health.go` - Add database connectivity check to readyzHandler

**Optional Files**:
- `test-api.sh` - Manual testing script
- `.env.example` - Add BACKEND_CORS_ORIGIN (optional)

**Total Lines Estimated**: ~1500 lines of new Go code

---

## Dependencies on Future Phases

**Phase 4A Enables**:
- **Phase 5 (Authentication)**: Users table ready for login, password hashing
- **Phase 6 (Serve Frontend)**: API endpoints ready for consumption
- **Phase 7 (Deploy)**: Health checks ready for Railway/K8s

**Phase 4A Requires**:
- ✅ Phase 3 complete (database migrations, schema exists)
- ✅ Docker environment working
- ✅ TimescaleDB running

**Phase 4B Deferred Until**:
- Post-Phase 7 deployment
- User feedback collected
- Business entity requirements validated

---

## Testing Strategy Summary

**Unit Tests** (~40-50 tests):
- Validation errors (missing fields, invalid emails, etc.)
- Malformed JSON handling
- Invalid ID formats
- Edge cases (empty strings, negative numbers, etc.)

**Integration Tests** (~14 tests):
- Create account → Verify in DB
- Get account → Verify response
- List accounts → Verify pagination
- Update account → Verify changes
- Delete account → Verify soft delete
- Duplicate domain → Verify 409
- (Same for users and account_users)

**Manual Tests** (via test-api.sh):
- Full CRUD workflow for each entity
- Error handling verification
- Performance checks (1000+ records)

**E2E Tests** (Phase 6+):
- Frontend integration tests will cover full stack
- Playwright tests for user workflows

---

## Open Questions (Resolved)

All questions answered during /plan clarifying phase:

1. ✅ Framework: chi
2. ✅ Database driver: pgx/v5
3. ✅ Validation: go-playground/validator/v10
4. ✅ File structure: Flat with domain grouping
5. ✅ Delete behavior: Soft delete
6. ✅ Testing: Unit + integration (reasonable scope)
7. ✅ Pagination: Offset/limit initially

No blocking questions remain.

---

## Implementation Order

**Week 1: Foundation**
1. Add dependencies (Task 1)
2. Database connection pool (Task 2)
3. Update health checks (Task 3)
4. Middleware stack (Task 4)
5. Error helpers (Task 5)

**Week 2: First Entity (Accounts)**
6. Accounts API (Task 6) - Full implementation + tests
7. Manual testing of accounts endpoints

**Week 3: Remaining Entities**
8. Users API (Task 7)
9. AccountUsers API (Task 8)
10. Integration tests (Task 9)

**Week 4: Integration & Validation**
11. Update main.go (Task 10)
12. Manual testing (Task 12)
13. Full validation (Task 13)
14. Documentation updates

**Total Estimated Time**: 3-4 weeks (assuming part-time development)

---

## Notes

**Why This Scope Works**:
- Establishes all patterns for future entity endpoints
- Enables Phase 5 (authentication) immediately
- Minimal surface area for debugging
- Fast iteration cycle
- Clear success metrics

**Why Business Entities Deferred**:
- Frontend works standalone (Web Bluetooth)
- Phase 5 (auth) only needs users/accounts
- Phase 6 (serve frontend) doesn't need backend RFID APIs yet
- Phase 7 (deploy) can ship with just user management
- Get real user feedback before building unused features
- Avoid premature optimization

**Alignment with CLAUDE.md**:
- ✅ Type safety: All structs strongly typed
- ✅ Clean architecture: Repository pattern separates data layer
- ✅ Testing first: Unit + integration tests required
- ✅ No magic numbers: Validation uses declarative tags
- ✅ Feature organization: Flat files grouped by domain
- ✅ Error handling: Explicit error propagation with context
- ✅ Dependency injection: Repositories take db pool as parameter

**Next Steps After Phase 4A**:
1. Phase 5: Add JWT auth, protect endpoints, hash passwords
2. Phase 6: Serve frontend via backend (static files or reverse proxy)
3. Phase 7: Deploy to Railway with health checks
4. Phase 8: Marketing (parallel with deployment)
5. Phase 4B: Add business entity APIs based on user feedback

---

**End of Plan**
