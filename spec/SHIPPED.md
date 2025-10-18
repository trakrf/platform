# Shipped Features

This file tracks all features that have been completed and shipped via Pull Request.

## Phase 3: Database Migrations
- **Date**: 2025-10-17
- **Branch**: feature/active-phase-3-database-migrations
- **Commit**: 7455398
- **PR**: https://github.com/trakrf/platform/pull/9
- **Summary**: Replace Docker entrypoint SQL with golang-migrate versioned migrations
- **Key Changes**:
  - Installed golang-migrate v4.17.0 CLI in backend Docker image
  - Created 24 migration files (12 up/down pairs) from database/init/
  - Added 5 Just commands for migration workflow (up, down, status, create, force)
  - Removed docker-entrypoint-initdb.d volume mount, added migrations volume
  - Auto-migration on `just dev` startup
  - Added .env.example for developer onboarding
  - Updated README.md and backend/README.md with migration documentation
- **Validation**: ✅ All checks passed

### Success Metrics
(From spec.md - all metrics achieved)
- ✅ All 12 migrations created from existing SQL files - **Result**: 24 files created (12 up/down pairs), verbatim SQL copy
- ✅ `just db-migrate-up` produces identical schema to current `database/init/` approach - **Result**: Schema verified identical, zero drift
- ✅ Down migrations successfully drop all tables/functions/sequences - **Result**: CASCADE drops tested, all objects cleaned up
- ✅ Migration version tracked in `schema_migrations` table - **Result**: Version 12 confirmed operational
- ✅ Documentation complete with migration workflow examples - **Result**: README.md + backend/README.md updated with commands and workflows
- ✅ Zero schema drift between old and new approach - **Result**: Pure infrastructure change, no schema modifications

**Overall Success**: 100% of metrics achieved

### Technical Highlights
- Migration timing: ~400-600ms per migration (TimescaleDB hypertables take longer)
- Full lifecycle tested: fresh → up → down → cascade → re-up
- Sample data down migration is no-op (cleanup via table CASCADE drops)
- sh -c wrapper in justfile for proper environment variable expansion
- Migrations mounted as Docker volume for development workflow

### Migration Structure
1. 000001_prereqs - TimescaleDB extensions, schema, functions
2. 000002_accounts through 000011_messages - Multi-tenant schema
3. 000012_sample_data - Development fixtures

## Phase 4A: REST API Foundation + User Management
- **Date**: 2025-10-18
- **Branch**: feature/phase-4a-rest-api
- **Commit**: a16d632
- **PR**: https://github.com/trakrf/platform/pull/10
- **Summary**: Implement foundational REST API with Accounts, Users, and AccountUsers CRUD endpoints
- **Key Changes**:
  - Chi router integration with middleware stack (request ID, recovery, CORS, content-type)
  - Database connection pool (pgx driver) with health checks and graceful shutdown
  - RFC 7807 error response format with request ID tracing
  - Accounts API: 5 endpoints (list, get, create, update, soft delete)
  - Users API: 5 endpoints with email uniqueness and password_hash security
  - AccountUsers API: 4 endpoints with nested routes and JOIN queries
  - Repository pattern for clean separation of concerns
  - go-playground/validator for declarative input validation
  - Comprehensive unit tests (32 passing, 10 intentionally skipped for integration)
  - Updated README.md with REST API documentation
- **Validation**: ✅ All checks passed (lint, test, build)

### Success Metrics
(From spec.md - Phase 4A expanded beyond original "Accounts only" scope)

✅ **Functional** (8/8 achieved):
- ✅ Can create account via POST /api/v1/accounts - **Result**: Implemented with validation
- ✅ Can retrieve account by ID via GET /api/v1/accounts/:id - **Result**: Implemented with 404 handling
- ✅ Can list all accounts via GET /api/v1/accounts - **Result**: Implemented with pagination
- ✅ Can update account via PUT /api/v1/accounts/:id - **Result**: Implemented with dynamic updates
- ✅ Can delete account via DELETE /api/v1/accounts/:id - **Result**: Implemented as soft delete
- ✅ Validation errors return 400 with field details - **Result**: RFC 7807 format with validation messages
- ✅ Non-existent resources return 404 - **Result**: All endpoints return proper 404 responses
- ✅ Duplicate domains return 409 - **Result**: Database constraint violations handled

✅ **Technical** (7/7 achieved):
- ✅ All endpoints return proper JSON with consistent format - **Result**: `{"data": ...}` pattern with pagination
- ✅ Error responses include request_id for tracing - **Result**: All errors include request ID from middleware
- ✅ Database connection pool configured and monitored - **Result**: pgx pool with 25 max, 5 min connections
- ✅ Middleware stack operational - **Result**: requestID, recovery, CORS, contentType all implemented
- ✅ All tests pass - **Result**: 32/32 unit tests passing, 10 skipped for integration
- ✅ `just backend` validates successfully - **Result**: Lint, test, build all passing
- ✅ Air hot-reload works with new routes - **Result**: Docker dev workflow unchanged

⏳ **Performance** (to be measured in production):
- ⏳ List endpoint handles 1000+ accounts without pagination issues - **Result**: Efficient SQL queries implemented, production testing pending
- ⏳ Response times < 100ms for simple queries - **Result**: Local testing shows < 10ms, production monitoring pending
- ⏳ No N+1 query issues - **Result**: All queries reviewed, no N+1 patterns detected

**Overall Success**: 100% of functional and technical metrics achieved, performance metrics pending production deployment

### Technical Highlights
- Chi router chosen for minimal footprint and stdlib compatibility
- Soft delete pattern using deleted_at timestamp (preserves audit trail)
- Dynamic UPDATE queries build SQL from non-nil struct fields (partial updates)
- Password hash security: `json:"-"` tag ensures password_hash never serialized
- Request ID middleware generates UUID for distributed tracing
- Panic recovery middleware prevents server crashes from handler panics
- CORS middleware configured for local development (TODO: make configurable)
- Validation errors include field-level details from go-playground/validator

### API Endpoints Implemented
**Total: 17 endpoints**
- Accounts: 5 endpoints (GET list, GET by ID, POST, PUT, DELETE)
- Users: 5 endpoints (GET list, GET by ID, POST, PUT, DELETE)
- AccountUsers: 4 endpoints (GET list, POST, PUT, DELETE - nested under accounts)
- Health: 3 endpoints (/healthz, /readyz, /health)

### Files Created
- backend/database.go (75 lines) - Connection pool management
- backend/middleware.go (90 lines) - Request ID, recovery, CORS, content-type
- backend/errors.go (85 lines) - RFC 7807 error responses
- backend/accounts.go (398 lines) - Accounts CRUD API
- backend/accounts_test.go (160 lines) - Account validation tests
- backend/users.go (355 lines) - Users CRUD API
- backend/users_test.go (157 lines) - User validation tests
- backend/account_users.go (380 lines) - AccountUsers CRUD API
- backend/account_users_test.go (125 lines) - AccountUser validation tests

### Files Modified
- backend/go.mod - Added chi, pgx, validator dependencies
- backend/health.go - Added database connectivity checks
- backend/main.go - Integrated chi router, middleware, all APIs
- README.md - Added REST API documentation

### Next Phase
Phase 5 will add authentication layer:
- JWT token generation and validation
- Session management
- Password hashing (bcrypt)
- Protected routes with auth middleware
- User login/logout endpoints
- Token refresh mechanism

## Phase 5A: Authentication Utilities (Foundation)
- **Date**: 2025-10-18
- **Branch**: feature/active-phase-5-authentication
- **Commit**: 0ae42ea
- **PR**: https://github.com/trakrf/platform/pull/11
- **Summary**: Add foundational authentication utilities (JWT generation/validation and bcrypt password hashing)
- **Key Changes**:
  - JWT utilities (jwt.go) - Token generation with 1-hour expiration
  - JWT unit tests (jwt_test.go) - 8 test cases for generation/validation/expiration
  - Password utilities (password.go) - bcrypt hashing with cost factor 10
  - Password unit tests (password_test.go) - 3 test cases for hashing/comparison
  - Dependencies added: github.com/golang-jwt/jwt/v5 and golang.org/x/crypto/bcrypt
  - Environment configuration: JWT_SECRET in .env.example
  - Docker integration: JWT_SECRET passed to backend container
- **Validation**: ✅ All checks passed (lint, test, build)

### Success Metrics
(From spec.md - Phase 5A foundational utilities only)

✅ **Foundational Utilities** (7/7 achieved):
- ✅ Password hashing with bcrypt (cost factor 10) - **Result**: HashPassword() and ComparePassword() implemented
- ✅ JWT generation works - **Result**: GenerateJWT() creates signed tokens with user claims
- ✅ JWT validation works - **Result**: ValidateJWT() verifies signature and expiration
- ✅ JWT includes user_id, email, current_account_id in claims - **Result**: JWTClaims struct with all fields
- ✅ JWT expiration enforced (1 hour) - **Result**: RegisteredClaims with ExpiresAt set to 1 hour
- ✅ JWT signed with secret from environment - **Result**: getJWTSecret() reads JWT_SECRET env var
- ✅ All utility tests passing - **Result**: 11/11 tests passing (8 JWT + 3 password)

**Overall Success**: 100% of foundational metrics achieved

### Technical Highlights
- Pure utility functions with no side effects (safe to merge without runtime impact)
- JWT claims structure: UserID (int), Email (string), CurrentAccountID (*int)
- Password cost factor matches Next.js implementation (bcrypt cost 10)
- Development default for JWT_SECRET warns to change in production
- No breaking changes - no modifications to existing endpoints

### Files Created
- backend/jwt.go (59 lines) - JWT generation and validation
- backend/jwt_test.go (95 lines) - JWT unit tests
- backend/password.go (18 lines) - bcrypt password utilities
- backend/password_test.go (39 lines) - password unit tests

### Files Modified
- backend/go.mod, backend/go.sum - Added jwt and bcrypt dependencies
- .env.example - Added JWT_SECRET configuration
- docker-compose.yaml - Pass JWT_SECRET to backend container

### Next Phase
Phase 5B will build on these utilities:
- Auth service (Login and Signup business logic)
- Auth handlers (HTTP endpoints for /api/v1/auth/login and /api/v1/auth/signup)
- Auth middleware (Protect existing REST API routes with JWT validation)
- UserRepository.GetByEmail() method
- Integration testing of full auth flow
