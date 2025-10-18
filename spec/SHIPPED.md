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

## Phase 5B: Authentication Endpoints
- **Date**: 2025-10-18
- **Branch**: feature/active-phase-5b-auth-endpoints
- **Commit**: 8ba074e
- **PR**: https://github.com/trakrf/platform/pull/12
- **Summary**: Add signup and login endpoints using Phase 5A utilities
- **Key Changes**:
  - Auth service (auth_service.go) - Business logic for signup and login
  - Auth handlers (auth.go) - HTTP endpoints for POST /api/v1/auth/signup and /api/v1/auth/login
  - Auth tests (auth_test.go) - Unit tests for validation and slugification
  - UserRepository.GetByEmail() method for login authentication
  - Service layer pattern (first service in codebase)
  - pgx transaction pattern for atomic multi-table operations
  - Account domain slugification for MQTT topic routing
- **Validation**: ✅ All checks passed (35 tests passing, 10 skipped integration)

### Success Metrics
(From spec.md - Phase 5B endpoints only, middleware deferred to Phase 5C)

✅ **Functional** (7/7 achieved):
- ✅ Users can signup via POST /api/v1/auth/signup - **Result**: Implemented with atomic transaction
- ✅ Users can login via POST /api/v1/auth/login - **Result**: Implemented with credential validation
- ✅ Both endpoints return JWT tokens - **Result**: Using Phase 5A GenerateJWT() with account context
- ✅ Duplicate email returns 409 Conflict - **Result**: Database constraint handling implemented
- ✅ Invalid credentials return 401 Unauthorized - **Result**: Generic error messages prevent enumeration
- ✅ Account created atomically with user - **Result**: pgx transaction across 3 tables (users, accounts, account_users)
- ✅ Account domain slug generated from name - **Result**: "My Company" → "my-company" for MQTT routing

✅ **Technical** (6/6 achieved):
- ✅ Password hashed with bcrypt - **Result**: Using Phase 5A HashPassword (cost 10)
- ✅ Password hash never exposed in JSON - **Result**: User struct has `json:"-"` tag verified
- ✅ JWT includes current_account_id - **Result**: Lookup from account_users table (1:1 for MVP)
- ✅ Transaction rollback safety - **Result**: defer tx.Rollback() pattern implemented
- ✅ Generic error messages for security - **Result**: "Invalid email or password" prevents enumeration
- ✅ All validation gates passing - **Result**: go fmt, go vet, go test, go build all clean

✅ **Testing** (4/4 achieved):
- ✅ Slug generation tested (6 edge cases) - **Result**: TestSlugifyAccountName covers special chars, spaces, etc.
- ✅ Signup validation tested (5 scenarios) - **Result**: TestSignup_ValidationErrors covers all failure modes
- ✅ Login validation tested (4 scenarios) - **Result**: TestLogin_ValidationErrors covers all failure modes
- ✅ Password hashing integration verified - **Result**: TestPasswordHashing verifies correct/incorrect passwords

**Overall Success**: 100% of Phase 5B metrics achieved (17/17)

### Technical Highlights
- Service layer pattern introduced for business logic separation
- pgx transactions for atomic 3-table insert (user + account + account_users)
- Account domain slugification uses regex to sanitize names for MQTT topics
- Generic error messages prevent email enumeration attacks
- Table-driven tests follow Go best practices
- No breaking changes (endpoints are additive)
- Existing API still unprotected (by design - Phase 5C adds middleware)

### New Patterns Introduced
- **Service Layer**: First service in codebase (AuthService separates business logic from handlers)
- **pgx Transactions**: `db.Begin(ctx)` → `tx.Commit()` with `defer tx.Rollback()` safety
- **Account Slugification**: Converts display names to URL-safe slugs for MQTT routing

### Security Features
- bcrypt password hashing (cost 10, matches Next.js implementation)
- Password hash excluded from JSON responses (`json:"-"` tag on User.PasswordHash)
- Generic login errors ("Invalid email or password") prevent email enumeration
- Input validation on all request fields (go-playground/validator)
- JWT tokens signed with HS256 (Phase 5A utilities)

### API Endpoints Added
**Total: 2 new endpoints**
- POST /api/v1/auth/signup - Create user + account, return JWT
- POST /api/v1/auth/login - Authenticate user, return JWT

### Files Created
- backend/auth_service.go (200 lines) - Service layer with Signup/Login business logic
- backend/auth.go (82 lines) - HTTP handlers for signup/login endpoints
- backend/auth_test.go (108 lines) - Unit tests for validation and slugification

### Files Modified
- backend/users.go (+22 lines) - Added GetByEmail() method for login
- backend/main.go (+5 lines) - Wired auth service and routes

### Next Phase
Phase 5C will complete authentication system:
- Auth middleware (extract and validate JWT from Authorization header)
- Protected routes (apply middleware to all Phase 4A endpoints)
- Integration tests (full signup → login → access protected endpoint flow)
- Context injection (make authenticated user available to handlers)
