# Phase 4: Basic REST API

**Workspace**: backend
**Status**: Active
**Created**: 2025-10-18
**Linear**: TRA-78

## Outcome

Build a foundational REST API layer that exposes the database schema via HTTP endpoints. This establishes the core API infrastructure that will support authentication (Phase 5) and frontend integration (Phase 6).

## User Story

As a backend developer
I want a REST API framework with basic CRUD endpoints
So that the frontend can interact with the database programmatically

## Context

**Current State**:
- ✅ Phase 3 complete - Database migrations operational with 12 tables
- ✅ Backend is stdlib-only (main.go, health.go - zero dependencies)
- ✅ TimescaleDB schema: accounts, users, locations, devices, antennas, assets, tags, events, messages
- ✅ Health endpoints working (/healthz, /readyz, /health)

**Desired State**:
- REST API framework chosen and integrated
- Basic CRUD endpoints for core entities
- Consistent JSON request/response handling
- Structured error responses
- Database connection pooling
- Foundation ready for authentication layer

**Why Now?**: Phase 3 gave us the database layer. Phase 4 exposes it via HTTP so Phase 5 (auth) can protect it and Phase 6 (frontend integration) can consume it.

## Technical Requirements

### 1. API Framework Selection

**Open Decision**: Choose between chi / echo / gorilla / stdlib

**Selection Criteria**:
- **Lightweight**: Prefer minimal dependencies (current backend is stdlib-only)
- **Middleware support**: Need logging, CORS, request ID, panic recovery
- **Router performance**: Must handle parameterized routes efficiently
- **Community**: Active maintenance, good documentation
- **Testing**: Easy to write table-driven tests
- **Future-proof**: Supports middleware for JWT auth (Phase 5)

**Considerations**:
- stdlib is tempting (zero deps) but may lack middleware ecosystem
- chi is popular, minimal, stdlib-focused
- echo is feature-rich but heavier
- gorilla is mature but less active maintenance

**Decision deferred to /plan**: Research and compare based on above criteria

### 2. Core Endpoints (MVP Scope)

**Accounts API** (start small, expand later):
```
GET    /api/v1/accounts          # List all accounts (paginated)
GET    /api/v1/accounts/:id      # Get account by ID
POST   /api/v1/accounts          # Create account
PUT    /api/v1/accounts/:id      # Update account
DELETE /api/v1/accounts/:id      # Delete account (soft delete?)
```

**Why accounts first?**
- Simplest table (no complex relations for MVP)
- Multi-tenant foundation - everything else depends on account_id
- Good learning endpoint for establishing patterns

**Future endpoints** (Phase 5+):
- Users, locations, devices, assets, tags
- Nested resources (e.g., `/accounts/:id/users`)
- Search and filtering

### 3. JSON Response Format

**Success Response**:
```json
{
  "data": {
    "id": 123,
    "name": "Acme Corp",
    "domain": "acme.com",
    "created_at": "2025-10-18T12:00:00Z"
  }
}
```

**List Response** (with pagination):
```json
{
  "data": [
    { "id": 123, "name": "Acme Corp" },
    { "id": 456, "name": "TechStart Inc" }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 42
  }
}
```

**Error Response** (RFC 7807 Problem Details):
```json
{
  "error": {
    "type": "validation_error",
    "title": "Invalid account data",
    "status": 400,
    "detail": "Account name is required",
    "instance": "/api/v1/accounts",
    "request_id": "abc123"
  }
}
```

### 4. Database Integration

**Connection Pooling**:
- Use `database/sql` with pgx driver
- Configure pool size, idle connections, max lifetime
- Proper connection cleanup on shutdown

**Repository Pattern**:
```go
type AccountRepository interface {
    List(ctx context.Context, limit, offset int) ([]Account, error)
    Get(ctx context.Context, id int) (*Account, error)
    Create(ctx context.Context, account *Account) error
    Update(ctx context.Context, account *Account) error
    Delete(ctx context.Context, id int) error
}
```

**Context Propagation**:
- Pass `context.Context` through all layers
- Support request timeouts and cancellation
- Enable tracing/logging correlation

### 5. Middleware Stack

**Required Middleware** (in order):
1. **Request ID**: Generate/extract for tracing
2. **Logger**: Structured logging with request context
3. **Recovery**: Catch panics, return 500 with request ID
4. **CORS**: Configure for frontend origin
5. **Content-Type**: Enforce application/json for POST/PUT
6. **Request Size Limit**: Prevent DoS (e.g., 1MB)

**Future Middleware** (Phase 5):
- JWT authentication
- Authorization (RBAC)
- Rate limiting

### 6. Error Handling Strategy

**HTTP Status Codes**:
- `200 OK`: Successful GET, PUT
- `201 Created`: Successful POST with Location header
- `204 No Content`: Successful DELETE
- `400 Bad Request`: Validation errors, malformed JSON
- `404 Not Found`: Resource doesn't exist
- `409 Conflict`: Duplicate resource (e.g., domain already exists)
- `500 Internal Server Error`: Unexpected errors (log details, return generic message)

**Error Types**:
```go
type ErrorType string

const (
    ErrValidation     ErrorType = "validation_error"
    ErrNotFound       ErrorType = "not_found"
    ErrConflict       ErrorType = "conflict"
    ErrInternal       ErrorType = "internal_error"
    ErrUnauthorized   ErrorType = "unauthorized"  // Phase 5
)
```

**Error Logging**:
- Log all 5xx errors with full stack trace
- Log 4xx errors at info level (business logic, not bugs)
- Include request ID for correlation
- Never leak internal details in error responses

### 7. Validation

**Input Validation**:
- Validate all request bodies before database queries
- Use Go structs with validation tags (if using validator package)
- Return detailed validation errors with field names

**Example** (accounts):
```go
type CreateAccountRequest struct {
    Name         string `json:"name" validate:"required,min=1,max=255"`
    Domain       string `json:"domain" validate:"required,hostname"`
    BillingEmail string `json:"billing_email" validate:"required,email"`
    Tier         string `json:"subscription_tier" validate:"oneof=free basic premium god-mode"`
}
```

**Database Constraints**:
- Let database enforce uniqueness (e.g., domain)
- Handle constraint violations gracefully (return 409 Conflict)

### 8. Testing Strategy

**Unit Tests** (handlers):
- Table-driven tests for each endpoint
- Mock database layer
- Test all HTTP status codes
- Test validation errors
- Test malformed JSON

**Integration Tests** (with real database):
- Test full request/response cycle
- Use test database (migrations applied)
- Test actual SQL queries
- Test transaction rollback on errors

**Example structure**:
```go
func TestAccountsHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        path       string
        body       string
        wantStatus int
        wantBody   string
    }{
        {"get account", "GET", "/api/v1/accounts/1", "", 200, `{"data":{"id":1}}`},
        {"create invalid", "POST", "/api/v1/accounts", `{}`, 400, `{"error":{"type":"validation_error"}}`},
    }
    // ...
}
```

## Constraints

- **Minimal Dependencies**: Prefer stdlib where possible (current backend pattern)
- **Database Schema**: Use existing schema from Phase 3 migrations (no changes)
- **Backwards Compatibility**: Keep existing health endpoints (/healthz, /readyz, /health)
- **Port**: Continue using 8080 (configurable via BACKEND_PORT)
- **Hot Reload**: Air must continue to work with new framework
- **Go Version**: 1.25+ (current requirement)

## Out of Scope (Deferred to Later Phases)

- ❌ Authentication/Authorization (Phase 5)
- ❌ All CRUD endpoints (just accounts for MVP)
- ❌ Frontend integration (Phase 6)
- ❌ Rate limiting
- ❌ API versioning strategy (v1 is fine for now)
- ❌ OpenAPI/Swagger documentation
- ❌ GraphQL (REST only)
- ❌ Webhooks
- ❌ Background jobs/workers

## Success Metrics

**Functional**:
- ✅ Can create a new account via POST /api/v1/accounts
- ✅ Can retrieve account by ID via GET /api/v1/accounts/:id
- ✅ Can list all accounts via GET /api/v1/accounts
- ✅ Can update account via PUT /api/v1/accounts/:id
- ✅ Can delete account via DELETE /api/v1/accounts/:id
- ✅ Validation errors return 400 with field details
- ✅ Non-existent resources return 404
- ✅ Duplicate domains return 409

**Technical**:
- ✅ All endpoints return proper JSON with consistent format
- ✅ Error responses include request_id for tracing
- ✅ Database connection pool configured and monitored
- ✅ Middleware stack operational (logger, recovery, CORS)
- ✅ All tests pass (unit + integration)
- ✅ `just backend` validates successfully
- ✅ Air hot-reload works with new routes

**Performance**:
- ✅ List endpoint handles 1000+ accounts without pagination issues
- ✅ Response times < 100ms for simple queries (local dev)
- ✅ No N+1 query issues

## Open Decisions for /plan

1. **Framework Selection**: chi vs echo vs gorilla vs stdlib
   - Need: Research, comparison table, recommendation
   - Criteria: Lightweight, middleware support, community, testing

2. **Database Driver**: pgx vs lib/pq
   - Need: Performance comparison, feature comparison
   - Consideration: pgx is faster, better error messages, but less stdlib-like

3. **Validation Library**: go-playground/validator vs custom
   - Need: Evaluate if validation library is worth the dependency
   - Alternative: Hand-rolled validation (more control, no deps)

4. **File Structure**: Flat vs layered
   - Flat (current): main.go, health.go, accounts.go, database.go
   - Layered: internal/handlers, internal/repository, internal/models
   - Decision impacts testability and future growth

5. **Soft vs Hard Delete**: For DELETE /api/v1/accounts/:id
   - Hard delete: Actually remove from database
   - Soft delete: Set deleted_at timestamp (preserve history)
   - Current schema has deleted_at field - implies soft delete intent?

## References

- **Linear Issue**: https://linear.app/trakrf/issue/TRA-78/phase-4-basic-rest-api
- **Database Schema**: `database/migrations/000002_accounts.up.sql` (and others)
- **Current Backend**: `backend/main.go`, `backend/health.go`
- **Phase 3 Log**: `spec/SHIPPED.md` (migration system details)

## Notes

**Why Accounts First?**
- Simplest table to start with (fewer relations)
- Multi-tenant foundation (everything references account_id)
- Establishes patterns for other endpoints
- Good learning opportunity without complex joins

**Why Not All CRUD at Once?**
- Start small, establish patterns
- Get feedback on API design before scaling
- Each table has unique complexities (e.g., messages = hypertable)
- Better to iterate on one endpoint than redo all of them

**Progression Path**:
- Phase 4: Accounts CRUD (this phase)
- Phase 5: Add auth, then expand CRUD to users
- Phase 6: Add locations, devices, assets, tags
- Phase 7+: Complex queries, search, filtering, nested resources
