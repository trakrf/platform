# Implementation Plan: Backend Internal Structure Refactoring

Generated: 2025-10-23
Specification: spec.md
Branch: refactor/internal-structure
Phase: Complete restructure (11 phases)

## Understanding

This plan migrates backend code from flat structure to layered `internal/` package architecture. The refactoring will:

1. **Organize by layer** - Separate models, storage, services, handlers, middleware, and utilities
2. **Maintain functionality** - Zero behavior changes, pure code reorganization
3. **Prevent circular imports** - Strict dependency order (models → storage → services → handlers)
4. **Keep tests passing** - Tests colocated with source, verified after each phase
5. **Enable scalability** - Clear boundaries for future growth (MQTT, webhooks, etc.)

**Key design decisions:**
- Incremental migration: Create new packages alongside old files, switch at end
- Tests move WITH their source files in the SAME operation (Go convention)
- Interfaces defined where used, not in central package
- No behavior changes - pure refactoring

**Critical: Test Colocation**
- ✅ Tests are created alongside source code in the SAME phase
- ✅ `file.go` + `file_test.go` in the same directory
- ✅ Source and tests committed together
- ❌ NO separate `tests/` directory (anti-Go pattern)
- When we say "migrate models," we mean create `account.go` AND `account_test.go` together

**Safety strategy:**
- Old files remain until new structure proven working
- Compile verification after each package created
- Test verification after each major phase
- main.go updated only when all packages ready
- Git commits at each phase for rollback safety

## Relevant Files

**Current Structure** (flat, 22 files):
```
backend/
├── main.go (153 lines)
├── accounts.go (398 lines)
├── users.go (377 lines)
├── account_users.go (379 lines)
├── auth.go (80 lines)
├── auth_service.go (199 lines)
├── jwt.go (78 lines)
├── password.go (24 lines)
├── middleware.go (172 lines)
├── errors.go (66 lines)
├── database.go (39 lines)
├── health.go (82 lines)
├── frontend.go (84 lines)
└── *_test.go (10 test files)
```

**Target Structure** (layered, 6 packages with colocated tests):
```
backend/
├── main.go (~120 lines)
├── internal/
│   ├── models/
│   │   ├── account.go + account_test.go
│   │   ├── user.go + user_test.go
│   │   ├── auth.go + auth_test.go
│   │   └── errors.go + errors_test.go
│   ├── storage/
│   │   ├── storage.go + storage_test.go
│   │   ├── accounts.go + accounts_test.go
│   │   ├── users.go + users_test.go
│   │   └── account_users.go + account_users_test.go
│   ├── services/
│   │   ├── auth.go + auth_test.go
│   │   ├── accounts.go + accounts_test.go
│   │   └── users.go + users_test.go
│   ├── handlers/
│   │   ├── auth.go + auth_test.go
│   │   ├── accounts.go + accounts_test.go
│   │   ├── users.go + users_test.go
│   │   ├── account_users.go + account_users_test.go
│   │   ├── health.go + health_test.go
│   │   └── frontend.go + frontend_test.go
│   ├── middleware/
│   │   ├── auth.go + auth_test.go
│   │   ├── cors.go + cors_test.go
│   │   ├── logging.go + logging_test.go
│   │   └── recovery.go + recovery_test.go
│   └── util/
│       ├── jwt.go + jwt_test.go
│       └── password.go + password_test.go
└── database/migrations/
```

## Architecture Impact

- **Subsystems affected**: All backend code (HTTP, business logic, database, auth)
- **New dependencies**: None (pure reorganization)
- **Breaking changes**: None (internal refactor only, API unchanged)
- **Import paths**: Change from flat to internal/ packages
- **Module path**: Remains `github.com/trakrf/platform/backend`

## Task Breakdown

---

### Task 1: Create Directory Structure
**Phase**: 1
**Risk**: None
**Estimated time**: 1 minute

**Action**: Create all internal/ subdirectories

**Commands**:
```bash
cd backend
mkdir -p internal/{models,storage,services,handlers,middleware,util}
```

**Verification**:
```bash
ls -la internal/
# Should show: models/ storage/ services/ handlers/ middleware/ util/
```

**Commit checkpoint**:
```bash
git add backend/internal
git commit -m "refactor: create internal package structure"
```

---

### Task 2: Migrate Models Layer
**Phase**: 2
**Risk**: Low (no dependencies)
**Estimated time**: 20 minutes

**Files to create** (source + tests colocated):
- `internal/models/account.go` + `account_test.go` (~80 + 20 lines)
- `internal/models/user.go` + `user_test.go` (~70 + 15 lines)
- `internal/models/auth.go` + `auth_test.go` (~50 + 10 lines)
- `internal/models/token.go` + `token_test.go` (~30 + 10 lines)
- `internal/models/errors.go` + `errors_test.go` (~70 + 15 lines)

**Extract from**:
- `accounts.go` → Account struct, CreateAccountRequest, UpdateAccountRequest
- `accounts_test.go` → Validation tests for Account structs
- `users.go` → User struct, CreateUserRequest, UpdateUserRequest
- `users_test.go` → Validation tests for User structs
- `account_users.go` → AccountUser struct
- `auth_service.go` → SignupRequest, LoginRequest, LoginResponse
- `jwt.go` → TokenClaims struct
- `jwt_test.go` → Token validation tests
- `errors.go` → All error types (move entire file)

**Implementation pattern**:

**Example: internal/models/account.go**
```go
package models

import "time"

// Account represents a top-level organization
type Account struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// CreateAccountRequest is the input for creating accounts
type CreateAccountRequest struct {
    Name string `json:"name" validate:"required,min=1,max=255"`
}

// UpdateAccountRequest is the input for updating accounts
type UpdateAccountRequest struct {
    Name string `json:"name" validate:"required,min=1,max=255"`
}
```

**Import rules**:
- ✅ Standard library only (time, encoding/json, etc.)
- ✅ Validation tags (go-playground/validator)
- ❌ NO imports from internal/ packages
- ❌ NO business logic, NO database queries

**Test colocation**:
Create `*_test.go` files alongside source in the SAME operation:
```bash
# Create source and test together
vim internal/models/account.go       # Struct definitions
vim internal/models/account_test.go  # Struct validation tests
```

**Verification**:
```bash
go build ./internal/models
go test ./internal/models  # Tests run immediately
```

**Commit checkpoint** (source + tests together):
```bash
git add backend/internal/models
git commit -m "refactor: add models layer with colocated tests"
```

---

### Task 3: Migrate Storage Layer
**Phase**: 3
**Risk**: Low (only imports models)
**Estimated time**: 40 minutes

**Files to create** (source + tests colocated):
- `internal/storage/storage.go` + `storage_test.go` (~60 + 20 lines)
- `internal/storage/accounts.go` + `accounts_test.go` (~150 + 60 lines)
- `internal/storage/users.go` + `users_test.go` (~140 + 55 lines)
- `internal/storage/account_users.go` + `account_users_test.go` (~100 + 40 lines)

**Extract from**:
- `database.go` → Connection pool logic
- `accounts.go` → CreateAccount, GetAccount, ListAccounts, UpdateAccount, DeleteAccount
- `accounts_test.go` → Database integration tests for accounts
- `users.go` → CreateUser, GetUser, GetUserByEmail, ListUsers, UpdateUser, DeleteUser
- `users_test.go` → Database integration tests for users
- `account_users.go` → AddUserToAccount, RemoveUserFromAccount, GetAccountUsers
- `account_users_test.go` → Database integration tests for account-user relationships

**Implementation pattern**:

**Example: internal/storage/storage.go**
```go
package storage

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/trakrf/platform/backend/internal/models"
)

// Storage handles all database operations
type Storage struct {
    pool *pgxpool.Pool
}

// New creates a new Storage instance
func New(pool *pgxpool.Pool) *Storage {
    return &Storage{pool: pool}
}

// Interface can be defined here or in consuming packages
// (We'll define in services/ to avoid circular imports)
```

**Example: internal/storage/accounts.go**
```go
package storage

import (
    "context"
    "github.com/trakrf/platform/backend/internal/models"
)

// CreateAccount inserts a new account
func (s *Storage) CreateAccount(ctx context.Context, req *models.CreateAccountRequest) (*models.Account, error) {
    // SQL query implementation
    query := `INSERT INTO accounts (name) VALUES ($1) RETURNING id, name, created_at, updated_at`

    var account models.Account
    err := s.pool.QueryRow(ctx, query, req.Name).Scan(
        &account.ID,
        &account.Name,
        &account.CreatedAt,
        &account.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }

    return &account, nil
}

// GetAccount retrieves account by ID
func (s *Storage) GetAccount(ctx context.Context, id string) (*models.Account, error) {
    // SQL query implementation
}

// ... other account methods
```

**Import rules**:
- ✅ `internal/models`
- ✅ pgx/pgxpool (database driver)
- ✅ Standard library
- ❌ NO imports from services/, handlers/, middleware/

**Test colocation**:
Create integration tests alongside storage code:
```bash
# Create source and test together
vim internal/storage/accounts.go       # SQL queries
vim internal/storage/accounts_test.go  # Database integration tests
```

**Verification**:
```bash
go build ./internal/storage
go test ./internal/storage  # Integration tests (may need test DB)
```

**Commit checkpoint** (source + tests together):
```bash
git add backend/internal/storage
git commit -m "refactor: add storage layer with integration tests"
```

---

### Task 4: Migrate Services Layer
**Phase**: 4
**Risk**: Medium (depends on models + storage)
**Estimated time**: 30 minutes

**Files to create** (source + tests colocated):
- `internal/services/auth.go` + `auth_test.go` (~150 + 60 lines)
- `internal/services/accounts.go` + `accounts_test.go` (~80 + 35 lines)
- `internal/services/users.go` + `users_test.go` (~80 + 35 lines)

**Extract from**:
- `auth_service.go` → Signup, Login, validation logic
- `auth_test.go` / `auth_service_test.go` → Business logic tests (can use mocked storage)
- `accounts.go` → Business validation, orchestration
- `accounts_test.go` → Service-level tests
- `users.go` → Business validation, orchestration
- `users_test.go` → Service-level tests

**Implementation pattern**:

**Example: internal/services/auth.go**
```go
package services

import (
    "context"
    "errors"
    "github.com/trakrf/platform/backend/internal/models"
    "github.com/trakrf/platform/backend/internal/storage"
    // JWT and password utilities will come from util/ later
)

// AuthStorage defines storage operations needed by AuthService
// (Interface defined in consumer, implemented by storage package)
type AuthStorage interface {
    CreateUser(ctx context.Context, req *models.CreateUserRequest) (*models.User, error)
    GetUserByEmail(ctx context.Context, email string) (*models.User, error)
}

// AuthService handles authentication business logic
type AuthService struct {
    storage AuthStorage
}

// NewAuthService creates an auth service
func NewAuthService(store *storage.Storage) *AuthService {
    return &AuthService{storage: store}
}

// Signup validates input and creates a new user
func (s *AuthService) Signup(ctx context.Context, req *models.SignupRequest) (*models.LoginResponse, error) {
    // Validation logic
    if err := validateSignupRequest(req); err != nil {
        return nil, err
    }

    // Check duplicate email
    existing, _ := s.storage.GetUserByEmail(ctx, req.Email)
    if existing != nil {
        return nil, errors.New("email already exists")
    }

    // Hash password (will use util/password.go)
    // Create user via storage
    // Generate JWT (will use util/jwt.go)
    // Return response

    return &models.LoginResponse{}, nil
}

// Login validates credentials and returns JWT
func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest) (*models.LoginResponse, error) {
    // Business logic implementation
}
```

**Import rules**:
- ✅ `internal/models`
- ✅ `internal/storage`
- ✅ `internal/util` (JWT, password helpers)
- ❌ NO imports from handlers/, middleware/
- ❌ NO imports from other services/ files

**Test colocation**:
Create unit tests with mocked dependencies alongside services:
```bash
# Create source and test together
vim internal/services/auth.go       # Business logic
vim internal/services/auth_test.go  # Unit tests (mock storage)
```

**Verification**:
```bash
go build ./internal/services
go test ./internal/services  # Unit tests with mocks
```

**Commit checkpoint** (source + tests together):
```bash
git add backend/internal/services
git commit -m "refactor: add services layer with unit tests"
```

---

### Task 5: Migrate Handlers Layer
**Phase**: 5
**Risk**: Medium (depends on services)
**Estimated time**: 40 minutes

**Files to create** (source + tests colocated):
- `internal/handlers/auth.go` + `auth_test.go` (~80 + 50 lines)
- `internal/handlers/accounts.go` + `accounts_test.go` (~150 + 80 lines)
- `internal/handlers/users.go` + `users_test.go` (~140 + 75 lines)
- `internal/handlers/account_users.go` + `account_users_test.go` (~190 + 65 lines)
- `internal/handlers/health.go` + `health_test.go` (~80 + 100 lines)
- `internal/handlers/frontend.go` + `frontend_test.go` (~85 + 70 lines)

**Extract from**:
- `auth.go` → Signup, Login HTTP handlers
- `auth_test.go` → HTTP handler tests
- `accounts.go` → Create, Get, List, Update, Delete account handlers
- `accounts_test.go` → HTTP handler tests
- `users.go` → Create, Get, List, Update, Delete user handlers
- `users_test.go` → HTTP handler tests
- `account_users.go` → Add, Remove, List user-account handlers
- `account_users_test.go` → HTTP handler tests
- `health.go` + `health_test.go` → Move both files
- `frontend.go` + `frontend_test.go` → Move both files

**Implementation pattern**:

**Example: internal/handlers/auth.go**
```go
package handlers

import (
    "encoding/json"
    "net/http"
    "github.com/trakrf/platform/backend/internal/models"
    "github.com/trakrf/platform/backend/internal/services"
)

// AuthHandler handles authentication HTTP endpoints
type AuthHandler struct {
    authService *services.AuthService
}

// NewAuthHandler creates an auth handler
func NewAuthHandler(authSvc *services.AuthService) *AuthHandler {
    return &AuthHandler{authService: authSvc}
}

// Signup handles POST /api/v1/auth/signup
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
    var req models.SignupRequest

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    resp, err := h.authService.Signup(r.Context(), &req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(resp)
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
    // Similar pattern
}
```

**Import rules**:
- ✅ `internal/models`
- ✅ `internal/services`
- ✅ Standard library (net/http, encoding/json)
- ✅ chi router
- ❌ NO imports from storage/ (use services instead)
- ❌ NO imports from other handlers/ files

**Test colocation**:
Create HTTP tests using httptest alongside handlers:
```bash
# Create source and test together
vim internal/handlers/auth.go       # HTTP handlers
vim internal/handlers/auth_test.go  # HTTP request/response tests
```

**Verification**:
```bash
go build ./internal/handlers
go test ./internal/handlers  # HTTP tests with httptest
```

**Commit checkpoint** (source + tests together):
```bash
git add backend/internal/handlers
git commit -m "refactor: add handlers layer with HTTP tests"
```

---

### Task 6: Migrate Middleware and Utilities
**Phase**: 6
**Risk**: Low (leaf packages)
**Estimated time**: 25 minutes

**Middleware files to create** (source + tests colocated):
- `internal/middleware/auth.go` + `auth_test.go` (~70 + 30 lines)
- `internal/middleware/cors.go` + `cors_test.go` (~35 + 15 lines)
- `internal/middleware/logging.go` + `logging_test.go` (~45 + 20 lines)
- `internal/middleware/recovery.go` + `recovery_test.go` (~45 + 20 lines)

**Utility files to create** (source + tests colocated):
- `internal/util/jwt.go` + `jwt_test.go` (~80 + 55 lines)
- `internal/util/password.go` + `password_test.go` (~25 + 15 lines)

**Extract from**:
- `middleware.go` → Split into 4 separate files by concern
- `auth_middleware_test.go` → Split tests to match middleware files
- `jwt.go` + `jwt_test.go` → Move both to util/
- `password.go` + `password_test.go` → Move both to util/

**Implementation pattern**:

**Example: internal/middleware/auth.go**
```go
package middleware

import (
    "context"
    "net/http"
    "strings"
    "github.com/trakrf/platform/backend/internal/models"
    "github.com/trakrf/platform/backend/internal/storage"
    "github.com/trakrf/platform/backend/internal/util"
)

type contextKey string

const userContextKey contextKey = "user"

// RequireAuth validates JWT and loads user from database
func RequireAuth(store *storage.Storage) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            token := strings.TrimPrefix(authHeader, "Bearer ")
            claims, err := util.ValidateJWT(token)
            if err != nil {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            user, err := store.GetUserByID(r.Context(), claims.UserID)
            if err != nil {
                http.Error(w, "User not found", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), userContextKey, user)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// GetUserFromContext extracts authenticated user from context
func GetUserFromContext(ctx context.Context) (*models.User, bool) {
    user, ok := ctx.Value(userContextKey).(*models.User)
    return user, ok
}
```

**Example: internal/util/jwt.go**
```go
package util

import (
    "time"
    "github.com/golang-jwt/jwt/v5"
    "github.com/trakrf/platform/backend/internal/models"
)

// GenerateJWT creates a signed JWT token
func GenerateJWT(userID string, jwtSecret string) (string, error) {
    claims := &models.TokenClaims{
        UserID: userID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(jwtSecret))
}

// ValidateJWT verifies and parses a JWT token
func ValidateJWT(tokenString string) (*models.TokenClaims, error) {
    // Implementation
}
```

**Import rules**:
- ✅ `internal/models`
- ✅ `internal/storage` (middleware only)
- ✅ External libs (jwt, bcrypt)
- ❌ NO imports from services/, handlers/

**Test colocation**:
Create tests alongside middleware and utilities:
```bash
# Create source and test together for middleware
vim internal/middleware/auth.go       # Middleware implementation
vim internal/middleware/auth_test.go  # Middleware tests

# Create source and test together for utilities
vim internal/util/jwt.go       # JWT utilities
vim internal/util/jwt_test.go  # JWT tests
```

**Verification**:
```bash
go build ./internal/middleware
go build ./internal/util
go test ./internal/middleware  # Middleware tests
go test ./internal/util        # Utility tests
```

**Commit checkpoint** (source + tests together):
```bash
git add backend/internal/middleware backend/internal/util
git commit -m "refactor: add middleware and utilities with tests"
```

---

### Task 7: Update main.go (Critical Switchover)
**Phase**: 7
**Risk**: HIGH (this is where we switch from old to new)
**Estimated time**: 30 minutes

**File to modify**: `backend/main.go`

**Action**: Complete rewrite with dependency injection

**Implementation**:

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
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/trakrf/platform/backend/internal/handlers"
    "github.com/trakrf/platform/backend/internal/middleware"
    "github.com/trakrf/platform/backend/internal/services"
    "github.com/trakrf/platform/backend/internal/storage"
)

var (
    version   = "dev"
    startTime time.Time
)

func main() {
    startTime = time.Now()

    // Initialize database connection
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        slog.Error("DATABASE_URL not set")
        os.Exit(1)
    }

    pool, err := pgxpool.New(context.Background(), dbURL)
    if err != nil {
        slog.Error("Failed to connect to database", "error", err)
        os.Exit(1)
    }
    defer pool.Close()

    // Initialize layers
    store := storage.New(pool)

    authService := services.NewAuthService(store)
    accountsService := services.NewAccountsService(store)
    usersService := services.NewUsersService(store)

    authHandler := handlers.NewAuthHandler(authService)
    accountsHandler := handlers.NewAccountsHandler(accountsService)
    usersHandler := handlers.NewUsersHandler(usersService)
    healthHandler := handlers.NewHealthHandler(version, startTime)
    frontendHandler := handlers.NewFrontendHandler()

    // Setup router
    r := setupRouter(
        store,
        authHandler,
        accountsHandler,
        usersHandler,
        healthHandler,
        frontendHandler,
    )

    // Start server
    port := os.Getenv("BACKEND_PORT")
    if port == "" {
        port = "8080"
    }

    srv := &http.Server{
        Addr:         ":" + port,
        Handler:      r,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    // Graceful shutdown
    go func() {
        slog.Info("Server starting", "port", port, "version", version)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("Server failed", "error", err)
            os.Exit(1)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    slog.Info("Server shutting down")
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        slog.Error("Server forced to shutdown", "error", err)
    }

    slog.Info("Server stopped")
}

func setupRouter(
    store *storage.Storage,
    authHandler *handlers.AuthHandler,
    accountsHandler *handlers.AccountsHandler,
    usersHandler *handlers.UsersHandler,
    healthHandler *handlers.HealthHandler,
    frontendHandler *handlers.FrontendHandler,
) *chi.Mux {
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.Recovery)
    r.Use(middleware.CORS)
    r.Use(middleware.Logging)

    // Frontend routes (must be before API routes)
    r.Handle("/assets/*", frontendHandler.ServeAssets())
    r.Handle("/favicon.ico", frontendHandler.ServeStatic())
    r.Handle("/icon-*", frontendHandler.ServeStatic())

    // Health checks
    r.Get("/healthz", healthHandler.Liveness)
    r.Get("/readyz", healthHandler.Readiness)
    r.Get("/health", healthHandler.Detailed)

    // API routes
    r.Route("/api/v1", func(r chi.Router) {
        // Public auth routes
        r.Post("/auth/signup", authHandler.Signup)
        r.Post("/auth/login", authHandler.Login)

        // Protected routes
        r.Group(func(r chi.Router) {
            r.Use(middleware.RequireAuth(store))

            // Accounts
            r.Get("/accounts", accountsHandler.List)
            r.Post("/accounts", accountsHandler.Create)
            r.Get("/accounts/{id}", accountsHandler.Get)
            r.Put("/accounts/{id}", accountsHandler.Update)
            r.Delete("/accounts/{id}", accountsHandler.Delete)

            // Users
            r.Get("/users", usersHandler.List)
            r.Post("/users", usersHandler.Create)
            r.Get("/users/{id}", usersHandler.Get)
            r.Put("/users/{id}", usersHandler.Update)
            r.Delete("/users/{id}", usersHandler.Delete)

            // Account-User relationships
            r.Post("/accounts/{accountID}/users/{userID}", accountsHandler.AddUser)
            r.Delete("/accounts/{accountID}/users/{userID}", accountsHandler.RemoveUser)
            r.Get("/accounts/{accountID}/users", accountsHandler.ListUsers)
        })
    })

    // SPA catch-all (must be last)
    r.Get("/*", frontendHandler.ServeSPA())

    return r
}
```

**Critical verification steps**:

1. **Verify compilation**:
```bash
cd backend
go build
```

2. **Run all tests**:
```bash
go test ./...
```

3. **Check for race conditions**:
```bash
go test -race ./...
```

4. **Test server startup**:
```bash
./bin/trakrf
# Should start without panic
```

5. **Smoke test endpoints**:
```bash
curl localhost:8080/healthz        # Should return "ok"
curl localhost:8080/health         # Should return JSON
curl localhost:8080/                # Should serve frontend
```

**If anything fails:**
```bash
git checkout main.go  # Rollback
# Debug issues in internal/ packages
```

**Commit checkpoint** (only if all tests pass):
```bash
git add backend/main.go
git commit -m "refactor: switch to internal package structure"
```

---

### Task 8: Run Full Test Suite
**Phase**: 8
**Risk**: Validation phase
**Estimated time**: 10 minutes

**Actions**:
```bash
cd backend

# 1. Run all tests
go test ./... -v

# 2. Check race conditions
go test -race ./...

# 3. Check coverage
go test -cover ./...

# 4. Build binary
go build -o bin/trakrf

# 5. Test binary startup
./bin/trakrf &
SERVER_PID=$!
sleep 2

# 6. Health check
curl http://localhost:8080/healthz
curl http://localhost:8080/health

# 7. Stop server
kill $SERVER_PID
```

**Success criteria**:
- [ ] All tests pass
- [ ] No race conditions
- [ ] Coverage ≥ 40%
- [ ] Binary compiles
- [ ] Server starts successfully
- [ ] Health endpoints respond

**If failures occur**: Debug and fix before proceeding

---

### Task 9: Update Documentation
**Phase**: 9
**Risk**: Low
**Estimated time**: 15 minutes

**Files to update**:
- `backend/README.md` - Update structure section
- `CLAUDE.md` - Update architecture guidance (if needed)

**backend/README.md changes**:

Update "Structure" section:
```markdown
## Structure

```
backend/
├── main.go              # Server entrypoint with dependency injection
├── go.mod               # Go module definition
├── Dockerfile           # Multi-stage build
│
├── internal/            # Private application code
│   ├── models/          # Domain entities (Account, User, Token)
│   ├── storage/         # Database operations (queries, connection pool)
│   ├── services/        # Business logic (validation, orchestration)
│   ├── handlers/        # HTTP endpoints (request/response handling)
│   ├── middleware/      # HTTP middleware (auth, CORS, logging)
│   └── util/            # Shared utilities (JWT, password hashing)
│
├── frontend/            # Embedded React build
│   └── dist/
│
└── database/            # SQL migrations
    └── migrations/
```

**Import Dependency Flow**:
```
main.go → handlers/ → services/ → storage/ → models/
          (middleware/ and util/ can be imported by multiple layers)
```

**Key principles**:
- Pure data structures in models/
- Database operations in storage/
- Business logic in services/
- HTTP concerns in handlers/
- Tests colocated with source
```

**Commit checkpoint**:
```bash
git add backend/README.md CLAUDE.md
git commit -m "docs: update for internal package structure"
```

---

### Task 10: Clean Up Old Files
**Phase**: 10
**Risk**: Medium (irreversible, but we have git)
**Estimated time**: 5 minutes

**ONLY proceed if**:
- [ ] All tests passing
- [ ] Server runs successfully
- [ ] Smoke tests completed
- [ ] Previous commits created

**Files to remove**:
```bash
cd backend

# Remove old source files
rm accounts.go accounts_test.go
rm users.go users_test.go
rm account_users.go account_users_test.go
rm auth.go auth_test.go
rm auth_service.go
rm auth_middleware_test.go
rm jwt.go jwt_test.go
rm password.go password_test.go
rm middleware.go
rm errors.go
rm database.go
rm health.go health_test.go
rm frontend.go frontend_test.go
```

**Keep**:
- `main.go` (updated)
- `main_test.go` (if exists)
- `go.mod`, `go.sum`
- `Dockerfile`, `.dockerignore`
- `justfile`
- `.air.toml`
- `README.md`
- `internal/` (entire directory)
- `database/` (migrations)
- `frontend/` (embedded dist)

**Verification after deletion**:
```bash
go build
go test ./...
./bin/trakrf  # Test startup
```

**Commit checkpoint**:
```bash
git add -A
git commit -m "refactor: remove old flat structure files"
```

---

### Task 11: Final Validation
**Phase**: 11
**Risk**: Low
**Estimated time**: 10 minutes

**Complete validation checklist**:

**1. Code Quality**:
```bash
cd backend
go fmt ./...
go vet ./...
```

**2. Tests**:
```bash
go test ./... -v
go test -race ./...
go test -cover ./...
```

**3. Build**:
```bash
go build -ldflags "-X main.version=0.1.0-refactored" -o bin/trakrf
```

**4. Runtime**:
```bash
DATABASE_URL="postgres://..." ./bin/trakrf &
sleep 2

# Test all endpoints
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/health
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123","name":"Test User"}'

# Stop server
pkill trakrf
```

**5. Docker Build** (if applicable):
```bash
docker build -t trakrf-backend:refactored .
docker run -p 8080:8080 -e DATABASE_URL="..." trakrf-backend:refactored
```

**6. Documentation Review**:
- [ ] README.md accurate
- [ ] CLAUDE.md updated (if needed)
- [ ] Spec validation criteria all checked
- [ ] All commit messages clear

**Final commit**:
```bash
git add -A
git commit -m "refactor: complete internal structure migration

- Organized code into internal/ package with 6 layers
- All tests passing (40+ tests)
- No circular dependencies
- Zero behavior changes
- Ready for PR"
```

---

## Rollback Plan

If critical issues discovered:

**Rollback entire refactor**:
```bash
git log --oneline  # Find commit before refactoring started
git reset --hard <commit-hash>
```

**Rollback specific phase**:
```bash
git log --oneline
git reset --hard HEAD~3  # Go back 3 commits
```

**Keep changes but unstage**:
```bash
git reset --soft HEAD^
```

---

## Success Criteria (From Spec)

At completion, verify:

- [x] All existing tests pass (40+ tests)
- [x] `go test ./...` completes successfully
- [x] `go test -race ./...` shows no race conditions
- [x] `go build` compiles without errors
- [x] No circular import errors
- [x] Code coverage remains at or above 40%
- [x] Backend server starts and responds to health checks
- [x] All API endpoints continue to function correctly
- [x] Frontend can still be served from embedded build

---

## Time Estimate

| Phase | Task | Time |
|-------|------|------|
| 1 | Create directories | 1 min |
| 2 | Models layer | 20 min |
| 3 | Storage layer | 40 min |
| 4 | Services layer | 30 min |
| 5 | Handlers layer | 40 min |
| 6 | Middleware/Utils | 25 min |
| 7 | Update main.go | 30 min |
| 8 | Full test suite | 10 min |
| 9 | Documentation | 15 min |
| 10 | Cleanup old files | 5 min |
| 11 | Final validation | 10 min |
| **Total** | | **~3.5 hours** |

Add buffer for debugging/issues: **4-5 hours total**

---

## Post-Migration

**After merge to main**:
1. Archive proposal docs:
   ```bash
   mv REFACTORING_PROPOSAL.md docs/archive/
   mv IMPORT_GUIDE.md docs/archive/
   ```

2. Move spec to shipped:
   ```bash
   mv spec/active/internal-structure spec/shipped/
   ```

3. Update SHIPPED.md with results

4. Continue with Phase 7 (deployment) or Phase 8 (MQTT) work
