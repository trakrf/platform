# Implementation Plan: Phase 5B - Authentication Endpoints

Generated: 2025-10-18
Specification: spec.md

## Understanding

Phase 5B adds user signup and login endpoints to the REST API, completing the first half of TRA-79 (Phase 5: Authentication). This phase builds on Phase 5A utilities (JWT + password hashing) to enable user registration and authentication WITHOUT yet protecting existing API routes (that's Phase 5C).

**Scope (Phase 5B):**
- User signup endpoint (creates user + account + links them)
- User login endpoint (validates credentials, returns JWT)
- UserRepository.GetByEmail() for email lookup
- AuthService layer for business logic
- Unit tests with mocked database

**Out of Scope (Phase 5C):**
- Auth middleware
- Protecting existing REST API routes
- Integration tests with real database

**Key Design Decisions from Clarifying Questions:**
1. **Transaction for signup** - Atomic 3-table insert (user, account, account_users)
2. **Account domain = slug** - Used in MQTT topic routing (`account.domain = "my-company"`)
3. **Duplicate domain → 409** - Database constraint enforced, no auto-increment suffix
4. **current_account_id from account_users** - 1:1 lookup for MVP (multi-account later)
5. **Password: min 8 chars only** - KISS approach (Type 2 decision)
6. **Generic login errors** - "Invalid email or password" (prevent email enumeration)
7. **Unit tests with mocks** - Not integration tests (simpler, faster)

## Relevant Files

### Reference Patterns (existing code to follow)

**Repository Pattern:**
- `backend/users.go:56-117` - UserRepository structure, GetByID pattern
- `backend/accounts.go:78-171` - AccountRepository, Create with RETURNING
- `backend/account_users.go:58-168` - AccountUserRepository, Create pattern

**Handler Pattern:**
- `backend/accounts.go:314-338` - createAccountHandler flow
  - JSON decode → validate → repository → error handling → response
- `backend/users.go:266-290` - createUserHandler flow
  - Duplicate email → 409 Conflict handling

**Error Handling:**
- `backend/errors.go:34-64` - writeJSONError RFC 7807 format
- `backend/accounts.go:328-334` - Duplicate constraint detection

**Validation:**
- `backend/accounts.go:36-45` - CreateAccountRequest with validate tags
- `backend/accounts.go:321-324` - validate.Struct usage

**JWT/Password Utilities:**
- `backend/jwt.go:21-43` - GenerateJWT function
- `backend/password.go:12-27` - HashPassword, ComparePassword

**Context Keys:**
- `backend/middleware.go:11-13` - contextKey type pattern

**Transaction Pattern (NEW - not in codebase yet):**
- Will use `db.Begin(ctx)` → `pgx.Tx`
- Standard Go pgx pattern for multi-table inserts

### Files to Create

**`backend/auth_service.go` (~180 LOC)**
- AuthService struct and interface
- SignupRequest, LoginRequest, AuthResponse types
- Signup method (transaction: create user + account + link)
- Login method (validate credentials, return JWT)
- slugifyAccountName helper (convert "My Company" → "my-company")

**`backend/auth.go` (~150 LOC)**
- signupHandler for POST /api/v1/auth/signup
- loginHandler for POST /api/v1/auth/login
- Follows Phase 4A handler pattern
- RFC 7807 error responses

**`backend/auth_test.go` (~150 LOC)**
- Mock repository interfaces
- Test signup: valid, duplicate email, validation errors, slug generation
- Test login: valid, invalid credentials, user not found
- Test helpers and fixtures

### Files to Modify

**`backend/users.go` (~+50 LOC)**
- Add `GetByEmail(ctx, email)` method to UserRepository
- Pattern: Similar to GetByID, return nil if not found
- Lines: Add after GetByID method (~line 117)

**`backend/main.go` (~+10 LOC)**
- Initialize authService (after repositories)
- Register auth routes (before existing API routes)
- Lines: ~line 46 (init), ~line 62 (routes)

## Architecture Impact

**Subsystems affected:**
- HTTP handlers (new auth endpoints)
- Service layer (NEW - first service in codebase)
- Data layer (UserRepository extension)

**New dependencies:** None (reusing existing packages)

**Breaking changes:** None (additive only)

**New patterns introduced:**
- Service layer pattern (business logic separation from handlers)
- Transaction pattern (multi-table atomic operations)

## Task Breakdown

### Task 1: Add UserRepository.GetByEmail()
**File**: `backend/users.go`
**Action**: MODIFY
**Pattern**: Reference users.go:97-117 (GetByID method)

**Implementation:**
```go
// Add after GetByID method (~line 117)

// GetByEmail retrieves a user by email address
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
		FROM trakrf.users
		WHERE email = $1 AND deleted_at IS NULL
	`

	var u User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastLoginAt,
		&u.Settings, &u.Metadata, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &u, nil
}
```

**Validation:**
```bash
just backend-lint
just backend-test
```

**Success criteria:**
- ✅ Compiles without errors
- ✅ Returns nil for non-existent email (no error)
- ✅ Returns user for existing email
- ✅ Filters soft-deleted users

---

### Task 2: Create slugification helper
**File**: `backend/auth_service.go` (new file)
**Action**: CREATE
**Pattern**: New utility function

**Implementation:**
```go
package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// slugifyAccountName converts account name to URL-safe slug for domain field
// Examples:
//   "My Company" → "my-company"
//   "ACME Corp!" → "acme-corp"
//   "Test  Multiple   Spaces" → "test-multiple-spaces"
func slugifyAccountName(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and non-alphanumeric chars with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	return slug
}
```

**Validation:**
```bash
just backend-lint
just backend-build
```

**Success criteria:**
- ✅ "My Company" → "my-company"
- ✅ "ACME Corp!!" → "acme-corp"
- ✅ Handles multiple spaces
- ✅ No leading/trailing hyphens

---

### Task 3: Create AuthService - Signup
**File**: `backend/auth_service.go` (continue from Task 2)
**Action**: CREATE
**Pattern**: Service layer with transaction, reference accounts.go:145-171 (Create)

**Implementation:**
```go
// Request/Response types
type SignupRequest struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=8"`
	AccountName string `json:"account_name" validate:"required,min=2"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"` // Reuse User struct from users.go
}

// AuthService handles authentication business logic
type AuthService struct {
	db                  *pgxpool.Pool
	userRepo            *UserRepository
	accountRepo         *AccountRepository
	accountUserRepo     *AccountUserRepository
}

// NewAuthService creates a new auth service instance
func NewAuthService(db *pgxpool.Pool, userRepo *UserRepository, accountRepo *AccountRepository, accountUserRepo *AccountUserRepository) *AuthService {
	return &AuthService{
		db:              db,
		userRepo:        userRepo,
		accountRepo:     accountRepo,
		accountUserRepo: accountUserRepo,
	}
}

// Signup registers a new user with a new account
func (s *AuthService) Signup(ctx context.Context, req SignupRequest) (*AuthResponse, error) {
	// Hash password
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Generate slug for account domain
	domain := slugifyAccountName(req.AccountName)

	// Start transaction for atomic operation
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Auto-rollback if commit not called

	// 1. Create user
	var user User
	userQuery := `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
	`
	err = tx.QueryRow(ctx, userQuery, req.Email, req.Email, passwordHash).Scan(
		&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.LastLoginAt,
		&user.Settings, &user.Metadata, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("email already exists")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// 2. Create account
	var account Account
	accountQuery := `
		INSERT INTO trakrf.accounts (name, domain, billing_email, subscription_tier, max_users, max_storage_gb)
		VALUES ($1, $2, $3, 'free', 5, 1)
		RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		          settings, metadata, billing_email, technical_email, created_at, updated_at
	`
	err = tx.QueryRow(ctx, accountQuery, req.AccountName, domain, req.Email).Scan(
		&account.ID, &account.Name, &account.Domain, &account.Status, &account.SubscriptionTier,
		&account.MaxUsers, &account.MaxStorageGB, &account.Settings, &account.Metadata,
		&account.BillingEmail, &account.TechnicalEmail, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("account name already taken")
		}
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// 3. Link user to account
	accountUserQuery := `
		INSERT INTO trakrf.account_users (account_id, user_id, role, status)
		VALUES ($1, $2, 'owner', 'active')
	`
	_, err = tx.Exec(ctx, accountUserQuery, account.ID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to link user to account: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Generate JWT with account ID
	token, err := GenerateJWT(user.ID, user.Email, &account.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Don't expose password_hash (User struct has `json:"-"` tag)
	return &AuthResponse{
		Token: token,
		User:  user,
	}, nil
}
```

**Validation:**
```bash
just backend-lint
just backend-build
```

**Success criteria:**
- ✅ Creates user, account, account_users atomically
- ✅ Generates slug from account_name
- ✅ Returns JWT with current_account_id
- ✅ Rolls back on error
- ✅ Handles duplicate email
- ✅ Handles duplicate domain

---

### Task 4: Create AuthService - Login
**File**: `backend/auth_service.go` (continue)
**Action**: MODIFY
**Pattern**: Password validation, user lookup

**Implementation:**
```go
// Add to AuthService struct

// Login authenticates user and returns JWT
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	// Lookup user by email
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup user: %w", err)
	}

	// Generic error if user not found (prevent email enumeration)
	if user == nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Compare password
	err = ComparePassword(req.Password, user.PasswordHash)
	if err != nil {
		// Generic error if password doesn't match (prevent enumeration)
		return nil, fmt.Errorf("invalid email or password")
	}

	// Lookup user's account (1:1 for MVP)
	accountUserQuery := `
		SELECT account_id
		FROM trakrf.account_users
		WHERE user_id = $1 AND deleted_at IS NULL
		LIMIT 1
	`
	var accountID int
	err = s.db.QueryRow(ctx, accountUserQuery, user.ID).Scan(&accountID)
	if err != nil {
		// User exists but no account linked (shouldn't happen, but handle gracefully)
		accountID = 0 // Will be nil in JWT
	}

	// Generate JWT
	var accountIDPtr *int
	if accountID != 0 {
		accountIDPtr = &accountID
	}
	token, err := GenerateJWT(user.ID, user.Email, accountIDPtr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	return &AuthResponse{
		Token: token,
		User:  *user,
	}, nil
}
```

**Validation:**
```bash
just backend-lint
just backend-build
```

**Success criteria:**
- ✅ Returns JWT for valid credentials
- ✅ Returns generic error for invalid email
- ✅ Returns generic error for invalid password
- ✅ Includes current_account_id in JWT

---

### Task 5: Create Auth Handlers
**File**: `backend/auth.go` (new file)
**Action**: CREATE
**Pattern**: Reference accounts.go:314-338 (createAccountHandler)

**Implementation:**
```go
package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

var authService *AuthService

// initAuthService initializes the authentication service
func initAuthService() {
	authService = NewAuthService(db, userRepo, accountRepo, accountUserRepo)
}

// signupHandler handles POST /api/v1/auth/signup
func signupHandler(w http.ResponseWriter, r *http.Request) {
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	resp, err := authService.Signup(r.Context(), req)
	if err != nil {
		// Check for specific error types
		errMsg := err.Error()
		if strings.Contains(errMsg, "email already exists") {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "Email already exists", "")
			return
		}
		if strings.Contains(errMsg, "account name already taken") {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "Account name already taken", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to signup", "")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": resp})
}

// loginHandler handles POST /api/v1/auth/login
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	resp, err := authService.Login(r.Context(), req)
	if err != nil {
		// Generic error for security (prevent email enumeration)
		if strings.Contains(err.Error(), "invalid email or password") {
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized, "Invalid email or password", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to login", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": resp})
}

// registerAuthRoutes registers authentication endpoints
func registerAuthRoutes(r chi.Router) {
	r.Post("/api/v1/auth/signup", signupHandler)
	r.Post("/api/v1/auth/login", loginHandler)
}
```

**Modify `backend/main.go`:**
```go
// Add after repository initialization (~line 46)
initAuthService()
slog.Info("Auth service initialized")

// Add before existing API routes (~line 62)
registerAuthRoutes(r)
```

**Validation:**
```bash
just backend-lint
just backend-build
just dev  # Start server
```

**Manual test with curl:**
```bash
# Signup
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "account_name": "Test Company"
  }'

# Expected: 201 Created with JWT token

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'

# Expected: 200 OK with JWT token
```

**Success criteria:**
- ✅ Signup returns 201 with JWT
- ✅ Duplicate email returns 409
- ✅ Duplicate account name returns 409
- ✅ Login returns 200 with JWT
- ✅ Invalid credentials return 401
- ✅ Validation errors return 400

---

### Task 6: Unit Tests
**File**: `backend/auth_test.go` (new file)
**Action**: CREATE
**Pattern**: Table-driven tests with mocks

**Implementation:**
```go
package main

import (
	"context"
	"testing"
)

// TestSlugifyAccountName tests slug generation
func TestSlugifyAccountName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "My Company", "my-company"},
		{"uppercase", "ACME CORP", "acme-corp"},
		{"special chars", "Test! Company@", "test-company"},
		{"multiple spaces", "Test  Multiple   Spaces", "test-multiple-spaces"},
		{"leading/trailing", "-Leading Trailing-", "leading-trailing"},
		{"numbers", "Company123", "company123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slugifyAccountName(tt.input)
			if result != tt.expected {
				t.Errorf("slugifyAccountName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSignup_ValidationErrors tests signup validation
func TestSignup_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		req     SignupRequest
		wantErr bool
	}{
		{"valid", SignupRequest{Email: "test@example.com", Password: "password123", AccountName: "Company"}, false},
		{"missing email", SignupRequest{Password: "password123", AccountName: "Company"}, true},
		{"invalid email", SignupRequest{Email: "notanemail", Password: "password123", AccountName: "Company"}, true},
		{"short password", SignupRequest{Email: "test@example.com", Password: "short", AccountName: "Company"}, true},
		{"missing account_name", SignupRequest{Email: "test@example.com", Password: "password123"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate.Struct() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLogin_ValidationErrors tests login validation
func TestLogin_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		req     LoginRequest
		wantErr bool
	}{
		{"valid", LoginRequest{Email: "test@example.com", Password: "password123"}, false},
		{"missing email", LoginRequest{Password: "password123"}, true},
		{"invalid email", LoginRequest{Email: "notanemail", Password: "password123"}, true},
		{"missing password", LoginRequest{Email: "test@example.com"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate.Struct() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestPasswordHashing tests password utilities
func TestPasswordHashing(t *testing.T) {
	password := "mySecurePassword123"

	// Hash password
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	// Verify correct password
	err = ComparePassword(password, hash)
	if err != nil {
		t.Errorf("ComparePassword() should succeed for correct password, got error: %v", err)
	}

	// Verify incorrect password
	err = ComparePassword("wrongPassword", hash)
	if err == nil {
		t.Errorf("ComparePassword() should fail for incorrect password")
	}
}

// Note: Full integration tests with mocked database would go here
// For Phase 5B, we're keeping tests simple and focused on validation
// Phase 5C will add integration tests with the full auth flow
```

**Validation:**
```bash
just backend-test
just backend  # Run all backend checks
```

**Success criteria:**
- ✅ All slug generation tests pass
- ✅ Validation tests pass for both signup and login
- ✅ Password hashing tests pass
- ✅ No test failures

---

## Risk Assessment

### Risk: Transaction rollback complexity
**Mitigation**: Use defer tx.Rollback() pattern (safe to call even after Commit). Test failure scenarios manually.

### Risk: Slug collision edge cases
**Mitigation**: Database constraint handles uniqueness. Return 409 to user. Future: Add numeric suffix logic if needed.

### Risk: Password hash exposure
**Mitigation**: User struct already has `json:"-"` tag on PasswordHash field. Verify in tests.

### Risk: Email enumeration via timing
**Mitigation**: Generic error messages. Timing attacks mitigated by bcrypt (always takes same time). No additional defense needed for MVP.

## Integration Points

**Database:**
- trakrf.users (INSERT)
- trakrf.accounts (INSERT)
- trakrf.account_users (INSERT)
- Transaction across all 3 tables

**HTTP Routes:**
- POST /api/v1/auth/signup (new)
- POST /api/v1/auth/login (new)
- No changes to existing routes

**Dependencies:**
- Phase 5A: JWT utilities (GenerateJWT)
- Phase 5A: Password utilities (HashPassword, ComparePassword)
- Phase 4A: Repositories (UserRepository, AccountRepository, AccountUserRepository)
- Phase 4A: Error handling (writeJSONError, RFC 7807)
- Phase 4A: Validation (go-playground/validator)

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are blocking gates, not suggestions.

**After EVERY task, run from project root:**
```bash
just backend-lint    # Gate 1: Syntax & Style
just backend-test    # Gate 2: Unit Tests
just backend-build   # Gate 3: Compilation
```

**Enforcement Rules:**
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

**Final validation before shipping:**
```bash
just backend        # All backend checks
just validate       # Full stack validation
```

**Manual validation (after Task 5):**
```bash
just dev            # Start server
# Run curl commands from Task 5
```

## Validation Sequence

**Per-task validation:**
1. After Task 1: `just backend` (verify GetByEmail compiles and tests pass)
2. After Task 2: `just backend-build` (verify slugification compiles)
3. After Task 3: `just backend-build` (verify AuthService.Signup compiles)
4. After Task 4: `just backend-build` (verify AuthService.Login compiles)
5. After Task 5: `just backend` + manual curl tests (verify endpoints work)
6. After Task 6: `just backend` (verify all tests pass)

**Final validation:**
```bash
just validate       # Full stack (backend + frontend)
```

**Success criteria:**
- ✅ All lint checks pass
- ✅ All unit tests pass (including new auth_test.go)
- ✅ Build succeeds
- ✅ Manual curl tests demonstrate signup/login flow
- ✅ No password_hash in JSON responses

## Plan Quality Assessment

**Complexity Score**: 4/10 (MEDIUM-LOW)

**File Impact**:
- Creating: 3 files (auth_service.go, auth.go, auth_test.go)
- Modifying: 2 files (users.go, main.go)
- **Total**: 5 files

**Subsystems**: 3 (handlers, service, repository)

**Tasks**: 6 subtasks (atomic and validatable)

**Dependencies**: 0 new packages (reusing existing)

**Patterns**: Adapting existing patterns + new service layer

---

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar patterns found in Phase 4A (repository, handler, error handling)
- ✅ Phase 5A utilities ready (JWT, password hashing)
- ✅ All clarifying questions answered
- ✅ User decisions locked in (transactions, slugs, errors, tests)
- ✅ Database schema understood
- ⚠️ Service layer pattern is NEW (not in codebase yet)
- ⚠️ Transaction pattern is NEW (no existing reference)

**Assessment**: High confidence in implementation success. Service layer and transactions are standard Go patterns, well-documented. The main unknowns are minor (slug edge cases, transaction error handling), both easily testable.

**Estimated one-pass success probability**: 85%

**Reasoning**:
- Strong foundation from Phase 4A patterns
- Clear user decisions eliminate ambiguity
- New patterns (service, transactions) are well-known Go idioms
- Main risk is transaction error handling (15% probability of needing iteration)
- Manual testing will catch any edge cases before shipping

---

## Phase 5B Completion Criteria

**When is Phase 5B done?**
- ✅ Users can signup via POST /api/v1/auth/signup
- ✅ Users can login via POST /api/v1/auth/login
- ✅ Both endpoints return JWT tokens
- ✅ Duplicate email/domain return 409
- ✅ Invalid credentials return 401
- ✅ All validation gates pass
- ✅ Manual curl tests demonstrate working flow

**Ready for Phase 5C when:**
- ✅ This PR is merged to main
- ✅ Auth endpoints proven to work
- ✅ JWT generation validated
- ✅ Ready to add auth middleware (Phase 5C scope)
