# Implementation Plan: Phase 5A - Password & JWT Utilities

**Generated**: 2025-10-18
**Specification**: spec.md (Phase 5A subset)
**Status**: Active

## Understanding

Phase 5A establishes the cryptographic foundation for authentication by implementing password hashing (bcrypt) and JWT token utilities. This phase focuses ONLY on the utility functions - no HTTP handlers, no middleware, no route protection. Those come in Phase 5B.

**Scope:**
- Password hashing with bcrypt (cost factor 10)
- JWT generation and validation
- Environment variable configuration (JWT_SECRET, JWT_EXPIRATION)
- Unit tests for both utilities
- Configuration files (.env.example, docker-compose.yml)

**Out of Scope (Phase 5B):**
- AuthService (Login/Signup logic)
- HTTP handlers (/api/v1/auth/login, /api/v1/auth/signup)
- Auth middleware (route protection)
- Integration with users repository

## Clarifying Questions - Resolved

1. **JWT Secret Management**: Option C - Full env config in Phase 5A (getJWTSecret() helper + .env.example + docker-compose.yml)
2. **JWT Expiration**: Option B - Env var (JWT_EXPIRATION) with 1-hour default, no parameter override yet
3. **Test Coverage**: Option A - Happy path tests only (YAGNI edge cases)
4. **Error Handling**: Option B - Wrap errors with context using `fmt.Errorf("...: %w", err)`
5. **Dependencies**: Option A - Latest via `go get`, trust semver + go.mod auto-pinning

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/health_test.go` (lines 11-54) - Table-driven test pattern with t.Run()
- `backend/errors.go` (line 18) - ErrUnauthorized already defined
- `backend/middleware.go` (lines 88-93) - Context helper pattern (getRequestID)

**Files to Create**:
- `backend/password.go` - bcrypt password hashing utilities
- `backend/password_test.go` - Unit tests for password utilities
- `backend/jwt.go` - JWT generation and validation utilities
- `backend/jwt_test.go` - Unit tests for JWT utilities

**Files to Modify**:
- `backend/go.mod` - Add jwt/v5 and bcrypt dependencies
- `backend/go.sum` - Auto-generated checksums
- `.env.example` - Add JWT_SECRET and JWT_EXPIRATION
- `docker-compose.yml` - Pass JWT env vars to backend container

## Architecture Impact

- **Subsystems affected**: Utilities layer (new)
- **New dependencies**:
  - `github.com/golang-jwt/jwt/v5` (v5.2.1 or latest)
  - `golang.org/x/crypto` (v0.31.0 or latest - includes bcrypt)
- **Breaking changes**: None (additive only)

## Task Breakdown

### Task 1: Install Dependencies

**Files**: `backend/go.mod`, `backend/go.sum`
**Action**: MODIFY

**Implementation**:
```bash
cd backend
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto/bcrypt
go mod tidy
```

**Validation**:
```bash
# Verify dependencies added
grep "github.com/golang-jwt/jwt/v5" backend/go.mod
grep "golang.org/x/crypto" backend/go.mod

# Build succeeds
cd backend && go build ./...
```

**Expected go.mod additions**:
```go
require (
    github.com/golang-jwt/jwt/v5 v5.2.1
    golang.org/x/crypto v0.31.0
)
```

---

### Task 2: Create Password Utilities

**File**: `backend/password.go`
**Action**: CREATE
**Pattern**: Follow simple utility pattern (no state, just functions)

**Implementation**:
```go
package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10 // Match Next.js implementation (trakrf-web)

// HashPassword generates bcrypt hash from plain text password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// ComparePassword checks if password matches hash
func ComparePassword(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("password comparison failed: %w", err)
	}
	return nil
}
```

**Validation**:
```bash
cd backend
go build ./...  # Must compile without errors
```

---

### Task 3: Create Password Tests

**File**: `backend/password_test.go`
**Action**: CREATE
**Pattern**: Reference `backend/health_test.go` for test structure

**Implementation**:
```go
package main

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"
	hash, err := HashPassword(password)

	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Error("hash should not be empty")
	}

	if hash == password {
		t.Error("hash should not equal plain password")
	}

	// bcrypt hash is always 60 characters
	if len(hash) != 60 {
		t.Errorf("expected hash length 60, got %d", len(hash))
	}

	// Should start with bcrypt identifier ($2a$ or $2b$)
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Errorf("hash should start with bcrypt identifier, got: %s", hash[:4])
	}
}

func TestComparePassword_Valid(t *testing.T) {
	password := "testpassword123"
	hash, _ := HashPassword(password)

	err := ComparePassword(password, hash)
	if err != nil {
		t.Errorf("ComparePassword should succeed for valid password: %v", err)
	}
}

func TestComparePassword_Invalid(t *testing.T) {
	password := "testpassword123"
	hash, _ := HashPassword(password)

	err := ComparePassword("wrongpassword", hash)
	if err == nil {
		t.Error("ComparePassword should fail for invalid password")
	}
}
```

**Validation**:
```bash
cd backend
go test -v -run TestHashPassword
go test -v -run TestComparePassword
```

**Expected output**:
```
=== RUN   TestHashPassword
--- PASS: TestHashPassword (0.10s)
=== RUN   TestComparePassword_Valid
--- PASS: TestComparePassword_Valid (0.10s)
=== RUN   TestComparePassword_Invalid
--- PASS: TestComparePassword_Invalid (0.10s)
PASS
```

---

### Task 4: Create JWT Utilities

**File**: `backend/jwt.go`
**Action**: CREATE
**Pattern**: Follow password.go structure (utility functions + helpers)

**Implementation**:
```go
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims represents the JWT payload structure
type JWTClaims struct {
	UserID           int    `json:"user_id"`
	Email            string `json:"email"`
	CurrentAccountID *int   `json:"current_account_id,omitempty"`
	jwt.RegisteredClaims
}

// GenerateJWT creates a signed JWT token for authenticated user
func GenerateJWT(userID int, email string, accountID *int) (string, error) {
	expiration := getJWTExpiration()
	expirationTime := time.Now().Add(time.Duration(expiration) * time.Second)

	claims := &JWTClaims{
		UserID:           userID,
		Email:            email,
		CurrentAccountID: accountID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString([]byte(getJWTSecret()))
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return tokenString, nil
}

// ValidateJWT parses and validates a JWT token
func ValidateJWT(tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJWTSecret()), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid JWT token")
	}

	return claims, nil
}

// getJWTSecret retrieves JWT signing secret from environment
func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Development fallback - MUST be overridden in production
		secret = "dev-secret-change-in-production"
	}
	return secret
}

// getJWTExpiration retrieves JWT expiration duration from environment (in seconds)
func getJWTExpiration() int {
	exp := os.Getenv("JWT_EXPIRATION")
	if exp == "" {
		return 3600 // 1 hour default
	}

	seconds, err := strconv.Atoi(exp)
	if err != nil || seconds <= 0 {
		return 3600 // fallback on invalid value
	}

	return seconds
}
```

**Validation**:
```bash
cd backend
go build ./...  # Must compile without errors
```

---

### Task 5: Create JWT Tests

**File**: `backend/jwt_test.go`
**Action**: CREATE
**Pattern**: Happy path tests only (YAGNI)

**Implementation**:
```go
package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGenerateJWT(t *testing.T) {
	// Set test environment
	os.Setenv("JWT_SECRET", "test-secret-key")
	os.Setenv("JWT_EXPIRATION", "3600")

	userID := 1
	email := "test@example.com"
	accountID := 5

	token, err := GenerateJWT(userID, email, &accountID)

	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty")
	}

	// JWT format: header.payload.signature (3 parts separated by dots)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestValidateJWT_Valid(t *testing.T) {
	// Set test environment
	os.Setenv("JWT_SECRET", "test-secret-key")
	os.Setenv("JWT_EXPIRATION", "3600")

	userID := 1
	email := "test@example.com"
	accountID := 5

	// Generate token
	token, err := GenerateJWT(userID, email, &accountID)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	// Validate token
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatalf("ValidateJWT failed: %v", err)
	}

	// Verify claims
	if claims.UserID != userID {
		t.Errorf("expected UserID %d, got %d", userID, claims.UserID)
	}

	if claims.Email != email {
		t.Errorf("expected Email %s, got %s", email, claims.Email)
	}

	if claims.CurrentAccountID == nil || *claims.CurrentAccountID != accountID {
		t.Errorf("expected AccountID %d, got %v", accountID, claims.CurrentAccountID)
	}

	// Verify expiration is ~1 hour from now
	expectedExpiry := time.Now().Add(3600 * time.Second)
	expiryDiff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if expiryDiff > 5*time.Second || expiryDiff < -5*time.Second {
		t.Errorf("expiration time off by %v", expiryDiff)
	}
}

func TestValidateJWT_Invalid(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-key")

	// Try to validate malformed token
	_, err := ValidateJWT("invalid.token.string")
	if err == nil {
		t.Error("ValidateJWT should fail for invalid token")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	// Generate with one secret
	os.Setenv("JWT_SECRET", "secret1")
	token, _ := GenerateJWT(1, "test@example.com", nil)

	// Validate with different secret
	os.Setenv("JWT_SECRET", "secret2")
	_, err := ValidateJWT(token)
	if err == nil {
		t.Error("ValidateJWT should fail when secret changes")
	}
}

func TestGetJWTSecret_Default(t *testing.T) {
	os.Unsetenv("JWT_SECRET")

	secret := getJWTSecret()
	if secret != "dev-secret-change-in-production" {
		t.Errorf("expected default secret, got %s", secret)
	}
}

func TestGetJWTExpiration_Default(t *testing.T) {
	os.Unsetenv("JWT_EXPIRATION")

	exp := getJWTExpiration()
	if exp != 3600 {
		t.Errorf("expected 3600 seconds default, got %d", exp)
	}
}

func TestGetJWTExpiration_Custom(t *testing.T) {
	os.Setenv("JWT_EXPIRATION", "7200")

	exp := getJWTExpiration()
	if exp != 7200 {
		t.Errorf("expected 7200 seconds, got %d", exp)
	}
}

func TestGetJWTExpiration_Invalid(t *testing.T) {
	os.Setenv("JWT_EXPIRATION", "invalid")

	exp := getJWTExpiration()
	if exp != 3600 {
		t.Errorf("expected fallback to 3600, got %d", exp)
	}
}
```

**Validation**:
```bash
cd backend
go test -v -run TestGenerateJWT
go test -v -run TestValidateJWT
go test -v -run TestGetJWT
```

**Expected output**:
```
=== RUN   TestGenerateJWT
--- PASS: TestGenerateJWT (0.00s)
=== RUN   TestValidateJWT_Valid
--- PASS: TestValidateJWT_Valid (0.00s)
=== RUN   TestValidateJWT_Invalid
--- PASS: TestValidateJWT_Invalid (0.00s)
=== RUN   TestValidateJWT_WrongSecret
--- PASS: TestValidateJWT_WrongSecret (0.00s)
=== RUN   TestGetJWTSecret_Default
--- PASS: TestGetJWTSecret_Default (0.00s)
=== RUN   TestGetJWTExpiration_Default
--- PASS: TestGetJWTExpiration_Default (0.00s)
=== RUN   TestGetJWTExpiration_Custom
--- PASS: TestGetJWTExpiration_Custom (0.00s)
=== RUN   TestGetJWTExpiration_Invalid
--- PASS: TestGetJWTExpiration_Invalid (0.00s)
PASS
```

---

### Task 6: Update .env.example

**File**: `.env.example`
**Action**: MODIFY

**Implementation**:
Add JWT configuration section at the end of the file:

```bash
# JWT Configuration
JWT_SECRET=your-secret-key-change-in-production
JWT_EXPIRATION=3600  # 1 hour (in seconds)
```

**Validation**:
```bash
# Verify file contains new variables
grep "JWT_SECRET" .env.example
grep "JWT_EXPIRATION" .env.example
```

---

### Task 7: Update docker-compose.yml

**File**: `docker-compose.yml`
**Action**: MODIFY

**Implementation**:
Add JWT environment variables to backend service (in the environment section):

```yaml
services:
  backend:
    environment:
      # ... existing vars (PG_URL, etc.) ...

      # JWT Configuration
      JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
      JWT_EXPIRATION: ${JWT_EXPIRATION:-3600}
```

**Validation**:
```bash
# Verify JWT vars in docker-compose.yml
grep "JWT_SECRET" docker-compose.yml
grep "JWT_EXPIRATION" docker-compose.yml

# Test docker-compose passes env vars correctly
docker-compose config | grep JWT
```

---

## Risk Assessment

**Low Risk Phase** - Utilities are isolated and well-tested patterns.

- **Risk**: bcrypt cost factor too low/high
  **Mitigation**: Using cost=10 (matches Next.js reference, industry standard)

- **Risk**: JWT secret leaked in development
  **Mitigation**: Dev default clearly marked "change-in-production", production requires env var

- **Risk**: Dependencies introduce vulnerabilities
  **Mitigation**: Both packages are Go standard/semi-standard (golang.org/x/crypto, mature jwt/v5)

## Integration Points

**None for Phase 5A** - Utilities are self-contained. Phase 5B will integrate:
- AuthService will call HashPassword() and ComparePassword()
- Auth handlers will call GenerateJWT()
- Auth middleware will call ValidateJWT()

## VALIDATION GATES (MANDATORY)

**After EVERY code change, run these commands from project root:**

### Gate 1: Syntax & Style
```bash
just backend-lint
# OR
cd backend && go fmt ./... && go vet ./...
```

### Gate 2: Unit Tests
```bash
just backend-test
# OR
cd backend && go test ./...
```

### Gate 3: Build
```bash
just backend-build
# OR
cd backend && go build ./...
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**After each task (1-7):**
```bash
just backend-lint
just backend-test
just backend-build
```

**Final validation (after Task 7 complete):**
```bash
# Full validation
just validate

# Verify all tests pass
cd backend && go test -v ./...

# Verify docker-compose starts with env vars
docker-compose up -d backend
docker-compose exec backend env | grep JWT
docker-compose down
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW-MEDIUM)
- 4 new files, 2 modified go.mod/sum, 2 config files
- 1 subsystem (utilities layer)
- 7 tasks total
- 2 new dependencies (standard packages)
- Well-known patterns (bcrypt, JWT)

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar test patterns found in codebase (health_test.go)
✅ ErrUnauthorized already defined in errors.go
✅ Reference implementation (Next.js) provides exact parameters
✅ Standard Go packages (bcrypt is golang.org/x/crypto)
✅ All clarifying questions answered
✅ No external dependencies or integration complexity

**Assessment**: Straightforward utility implementation with clear success criteria. Standard Go patterns, well-tested libraries, and isolated scope minimize risk.

**Estimated one-pass success probability**: 92%

**Reasoning**:
- Simple, well-understood utilities (bcrypt, JWT)
- Clear patterns from reference implementation
- Isolated from rest of system (no integration yet)
- Only risk is typos or test assertions, both caught by validation gates
- bcrypt and JWT are proven libraries with stable APIs

## Success Criteria (Phase 5A Only)

All must pass:
- ✅ `go build ./...` succeeds without errors
- ✅ All unit tests pass (password + JWT utilities)
- ✅ JWT tokens can be generated and validated
- ✅ Passwords can be hashed and compared
- ✅ Environment variables read correctly
- ✅ Docker-compose passes JWT env vars to backend
- ✅ No hardcoded secrets in code

## Next Steps (Phase 5B)

After Phase 5A merges:
- Extend UserRepository with GetByEmail() method
- Create AuthService (Login, Signup methods)
- Create auth handlers (loginHandler, signupHandler)
- Create authMiddleware for route protection
- Wire everything in main.go
- Protect all Phase 4A routes
- Integration testing with real endpoints
