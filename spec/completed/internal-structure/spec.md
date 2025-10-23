# Feature: Backend Internal Structure Refactoring

## Metadata
**Workspace**: backend
**Type**: refactor

## Outcome
Backend code is organized into clear, navigable layers (models, storage, services, handlers, middleware, util) within an `internal/` package structure, making it easier to find code and maintain clear separation of concerns.

## User Story
As a backend developer
I want the codebase organized by layer (models, storage, services, handlers)
So that I can quickly find related code and maintain clear boundaries between different concerns

## Context

**Current**: Flat structure in `backend/` with ~3,500 lines across 22 Go files. All code at root level with mixed concerns (handlers, services, database queries, models all in same files or adjacent files). Largest file is 398 lines (under 500-line limit but approaching it).

**Desired**: Layered structure using `internal/` package with clear separation:
```
backend/
├── main.go
├── internal/
│   ├── models/          # Pure data structures
│   ├── storage/         # Database operations
│   ├── services/        # Business logic
│   ├── handlers/        # HTTP endpoints
│   ├── middleware/      # HTTP middleware
│   └── util/            # Shared utilities
└── database/migrations/
```

**Examples**:
- Supabase Auth uses similar layered structure (internal/api, internal/models, internal/storage)
- Standard Go practice for projects 3,000+ lines
- Gitea, Buffalo, and other production Go projects use internal/ with layers

## Technical Requirements

### Layer Responsibilities

1. **models/** - Pure data structures
   - Struct definitions (Account, User, Token, etc.)
   - Request/Response types
   - Validation tags
   - Custom error types
   - NO business logic, NO imports from other internal packages

2. **storage/** - Database operations
   - Connection pool management
   - SQL queries (CRUD operations)
   - Transaction helpers
   - Imports: `models/` only (plus external libs like pgx)

3. **services/** - Business logic
   - Validation logic
   - Business rules
   - Orchestration between storage calls
   - Imports: `models/`, `storage/`

4. **handlers/** - HTTP layer
   - Request parsing
   - Response formatting
   - HTTP status codes
   - Imports: `models/`, `services/`

5. **middleware/** - HTTP middleware
   - Authentication
   - CORS
   - Logging
   - Recovery
   - Imports: Can import `storage/`, `models/`, `util/` as needed

6. **util/** - Shared utilities
   - JWT operations
   - Password hashing
   - Validation helpers
   - Imports: `models/` only

### Import Dependency Flow (Critical - Prevents Circular Dependencies)

```
main.go
   ↓
handlers/
   ↓
services/
   ↓
storage/
   ↓
models/

(middleware/ and util/ can be imported by multiple layers)
```

**Rules:**
- `models/` imports NOTHING from internal/
- `storage/` imports ONLY `models/`
- `services/` imports `models/` + `storage/`
- `handlers/` imports `models/` + `services/`
- `middleware/` imports `storage/`, `models/`, `util/` as needed
- `util/` imports `models/` only
- NO domain cross-imports (services/users NEVER imports services/accounts)

### Test Colocation (Go Convention)

- Tests MUST stay with source code (e.g., `users.go` + `users_test.go`)
- No separate `tests/` directory
- This is non-negotiable for Go tooling compatibility

### Interface Definition Pattern

- Define interfaces in the package that USES them, not in a central package
- Example: `services/users.go` defines `Storage` interface, `storage/` implements it implicitly
- Avoids circular dependencies and follows Go best practices

## Migration Plan

### Phase 1: Create Directory Structure
```bash
mkdir -p backend/internal/{models,storage,services,handlers,middleware,util}
```

### Phase 2: Migrate Models Layer
Extract pure data structures from existing files:
- `accounts.go` → extract `Account` struct → `internal/models/account.go`
- `users.go` → extract `User` struct → `internal/models/user.go`
- `auth_service.go` → extract request/response types → `internal/models/auth.go`
- `errors.go` → move entirely → `internal/models/errors.go`

**Files to create:**
- `internal/models/account.go` (Account struct, CreateAccountRequest, etc.)
- `internal/models/user.go` (User struct, CreateUserRequest, etc.)
- `internal/models/token.go` (Token struct, JWT claims)
- `internal/models/errors.go` (Custom error types)

**Tests:** Move `*_test.go` for model validation alongside models

### Phase 3: Migrate Storage Layer
Extract database operations:
- `database.go` → `internal/storage/storage.go` (pool, connection)
- `accounts.go` → extract queries → `internal/storage/accounts.go`
- `users.go` → extract queries → `internal/storage/users.go`
- `account_users.go` → extract queries → `internal/storage/account_users.go`

**Files to create:**
- `internal/storage/storage.go` (Storage struct, connection pool, New() constructor)
- `internal/storage/accounts.go` (CreateAccount, GetAccount, ListAccounts, etc.)
- `internal/storage/users.go` (CreateUser, GetUser, GetUserByEmail, etc.)
- `internal/storage/account_users.go` (AddUserToAccount, RemoveUserFromAccount, etc.)

**Tests:** Move integration tests that hit database alongside storage files

### Phase 4: Migrate Services Layer
Extract business logic:
- `auth_service.go` → `internal/services/auth.go`
- `accounts.go` → extract logic → `internal/services/accounts.go`
- `users.go` → extract logic → `internal/services/users.go`

**Files to create:**
- `internal/services/auth.go` (Signup, Login, validation logic)
- `internal/services/accounts.go` (Account business logic)
- `internal/services/users.go` (User business logic)

**Tests:** Move service-level tests (mocked storage) alongside services

### Phase 5: Migrate Handlers Layer
Extract HTTP handlers:
- `auth.go` → `internal/handlers/auth.go`
- `accounts.go` → extract handlers → `internal/handlers/accounts.go`
- `users.go` → extract handlers → `internal/handlers/users.go`
- `account_users.go` → extract handlers → `internal/handlers/account_users.go`
- `health.go` → `internal/handlers/health.go`
- `frontend.go` → `internal/handlers/frontend.go`

**Files to create:**
- `internal/handlers/auth.go` (Signup, Login HTTP handlers)
- `internal/handlers/accounts.go` (Create, Get, List, Update, Delete accounts)
- `internal/handlers/users.go` (Create, Get, List, Update, Delete users)
- `internal/handlers/account_users.go` (Add, Remove, List users in accounts)
- `internal/handlers/health.go` (Health check endpoints)
- `internal/handlers/frontend.go` (Serve embedded frontend)

**Tests:** Move handler tests (HTTP request/response tests) alongside handlers

### Phase 6: Migrate Middleware and Utilities
- `middleware.go` → split into `internal/middleware/` files
  - Auth middleware → `internal/middleware/auth.go`
  - CORS → `internal/middleware/cors.go`
  - Logging → `internal/middleware/logging.go`
  - Recovery → `internal/middleware/recovery.go`

- `jwt.go` → `internal/util/jwt.go`
- `password.go` → `internal/util/password.go`

**Tests:** Move alongside respective files

### Phase 7: Update main.go
Rewrite `main.go` to:
1. Import all new internal packages
2. Initialize layers in dependency order:
   - Storage (with connection pool)
   - Services (inject storage)
   - Handlers (inject services)
3. Setup router with handlers
4. Start server

**Imports pattern:**
```go
import (
    "github.com/trakrf/platform/backend/internal/storage"
    "github.com/trakrf/platform/backend/internal/services"
    "github.com/trakrf/platform/backend/internal/handlers"
    "github.com/trakrf/platform/backend/internal/middleware"
)
```

### Phase 8: Update go.mod (if needed)
Verify module path is correct: `github.com/trakrf/platform/backend`

### Phase 9: Run Tests
```bash
cd backend
go test ./...
go test -race ./...
```

Verify:
- All tests pass
- No race conditions
- No circular import errors

### Phase 10: Update Documentation
- Update `backend/README.md` with new structure
- Update `CLAUDE.md` if it references backend structure
- Document import rules and layer responsibilities

### Phase 11: Clean Up Old Files
After verifying all tests pass:
```bash
rm backend/accounts.go backend/accounts_test.go
rm backend/users.go backend/users_test.go
rm backend/auth.go backend/auth_service.go backend/auth_test.go
# ... (remove all migrated files)
```

## File Migration Map

| Current File | New Location(s) | Split Into |
|--------------|-----------------|------------|
| `accounts.go` (398 lines) | `models/account.go` (80 lines)<br>`storage/accounts.go` (150 lines)<br>`services/accounts.go` (80 lines)<br>`handlers/accounts.go` (88 lines) | Structs → models<br>Queries → storage<br>Logic → services<br>HTTP → handlers |
| `users.go` (377 lines) | `models/user.go` (70 lines)<br>`storage/users.go` (140 lines)<br>`services/users.go` (80 lines)<br>`handlers/users.go` (87 lines) | Same pattern |
| `account_users.go` (379 lines) | `models/account_user.go` (40 lines)<br>`storage/account_users.go` (150 lines)<br>`handlers/account_users.go` (189 lines) | Same pattern |
| `auth.go` (80 lines) | `handlers/auth.go` (80 lines) | HTTP handlers only |
| `auth_service.go` (199 lines) | `services/auth.go` (150 lines)<br>`models/auth.go` (49 lines) | Logic → services<br>Types → models |
| `jwt.go` (78 lines) | `util/jwt.go` (78 lines) | Direct move |
| `password.go` (24 lines) | `util/password.go` (24 lines) | Direct move |
| `middleware.go` (172 lines) | `middleware/auth.go` (60 lines)<br>`middleware/cors.go` (30 lines)<br>`middleware/logging.go` (40 lines)<br>`middleware/recovery.go` (42 lines) | Split by concern |
| `errors.go` (66 lines) | `models/errors.go` (66 lines) | Direct move |
| `database.go` (39 lines) | `storage/storage.go` (50 lines) | Expand with interface |
| `health.go` (82 lines) | `handlers/health.go` (82 lines) | Direct move |
| `frontend.go` (84 lines) | `handlers/frontend.go` (84 lines) | Direct move |
| `main.go` (153 lines) | `main.go` (120 lines) | Rewrite with dependency injection |

## Validation Criteria
- [ ] All files moved to appropriate internal/ packages
- [ ] No circular import dependencies
- [ ] All tests pass with new structure
- [ ] No race conditions detected
- [ ] Import paths follow dependency flow rules
- [ ] Tests colocated with source files
- [ ] go.mod updated if necessary

## Success Metrics

- [ ] All existing tests pass (40+ tests currently passing)
- [ ] `go test ./...` completes successfully
- [ ] `go test -race ./...` shows no race conditions
- [ ] `go build` compiles without errors
- [ ] No circular import errors
- [ ] Code coverage remains at or above 40%
- [ ] Backend server starts and responds to health checks
- [ ] All API endpoints continue to function correctly
- [ ] Frontend can still be served from embedded build

## References
- Supabase Auth internal structure: https://github.com/supabase/auth/tree/master/internal
- Go project layout: https://github.com/golang-standards/project-layout
- CLAUDE.md: Project guidelines (never create files > 500 lines, Clean Architecture)
- Current backend README: `backend/README.md`
- REFACTORING_PROPOSAL.md: Initial analysis document (can be archived after migration)
- IMPORT_GUIDE.md: Import rules and patterns (can be archived after migration)
