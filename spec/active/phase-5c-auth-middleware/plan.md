# Implementation Plan: Phase 5C - Auth Middleware & Protected Routes
Generated: 2025-10-18
Specification: spec.md

## Understanding

Phase 5C completes the authentication system by adding JWT validation middleware to protect existing API endpoints. This is the final piece of TRA-79, making JWT tokens from Phase 5B actually useful.

**Core Requirements:**
1. Create `authMiddleware` that validates JWT tokens from Authorization header
2. Protect all Phase 4A REST endpoints (accounts, users, account_users)
3. Keep public endpoints accessible (health checks, auth routes)
4. Inject authenticated user claims into request context
5. Return generic 401 errors (security best practice)
6. Add 4 minimal integration tests with httptest + chi router

**Clarifying Decisions (from Q&A):**
- Auth middleware follows existing patterns with auth-specific adaptations
- `GetUserClaims()` returns nil (handlers validate defensively - idiomatic Go)
- Tests use httptest with real chi router (not mocks - integration-style)
- Error messages are generic for security, detailed logs server-side
- Trust ValidateJWT(), just check for nil (no redundant field validation)

## Relevant Files

### Reference Patterns (existing code to follow):

**Middleware Pattern:**
- `backend/middleware.go:16-27` - requestIDMiddleware shows context injection pattern
- `backend/middleware.go:88-93` - getRequestID helper shows context extraction pattern
- `backend/middleware.go:11-12` - contextKey type definition

**JWT Validation:**
- `backend/jwt.go:45-66` - ValidateJWT() function (Phase 5A)
- `backend/jwt.go:13-18` - JWTClaims struct definition

**Error Handling:**
- `backend/errors.go:34-64` - writeJSONError with RFC 7807 format
- `backend/errors.go:18` - ErrUnauthorized already defined
- `backend/auth.go:66-70` - Generic error pattern for security

**Router Grouping:**
- `backend/main.go:52-70` - Current flat route registration
- Chi router docs pattern: `r.Group(func(r chi.Router) { r.Use(middleware) })`

**Test Pattern:**
- `backend/health_test.go:11-54` - httptest.NewRequest/ResponseRecorder pattern
- `backend/health_test.go:7-36` - Table-driven test structure

### Files to Modify:

**backend/middleware.go** (add ~90 LOC):
- Add `UserClaimsKey` contextKey constant (after line 12)
- Add `authMiddleware()` function (~40 LOC)
- Add `GetUserClaims()` helper function (~10 LOC)

**backend/main.go** (modify ~10 LOC):
- Lines 67-70: Wrap protected routes in `r.Group()` with authMiddleware

### Files to Create:

**backend/auth_middleware_test.go** (~120 LOC):
- 4 test functions (missing token, invalid token, valid token, public endpoints)
- Table-driven test structure like health_test.go
- Use httptest + chi router

## Architecture Impact

**Subsystems affected:** Backend only (middleware, routing)
**New dependencies:** None (uses existing chi router, JWT utils)
**Breaking changes:** None - endpoints remain same, now require auth

**Pattern Introduction:**
- Auth middleware with context injection (extends existing middleware.go)
- Protected route groups using chi.Router.Group() (new to this codebase)

## Task Breakdown

### Task 1: Add auth middleware to middleware.go
**File:** `backend/middleware.go`
**Action:** MODIFY (extend existing file)
**Pattern:** Follow requestIDMiddleware pattern (lines 16-27)

**Implementation:**

```go
// Add after line 12 (after requestIDKey constant):
const UserClaimsKey contextKey = "user_claims"

// Add after line 93 (after getRequestID function):
// authMiddleware validates JWT token and injects claims into context
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			slog.Info("Missing authorization header",
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Missing authorization header", "")
			return
		}

		// 2. Parse "Bearer {token}" format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			slog.Info("Invalid authorization header format",
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Invalid authorization header format", "")
			return
		}
		token := parts[1]

		// 3. Validate JWT using Phase 5A ValidateJWT()
		claims, err := ValidateJWT(token)
		if err != nil {
			slog.Info("JWT validation failed",
				"error", err,
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Invalid or expired token", "")
			return
		}

		// 4. Defensive nil check (idiomatic Go)
		if claims == nil {
			slog.Error("ValidateJWT returned nil claims without error",
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Invalid or expired token", "")
			return
		}

		// 5. Inject claims into context and proceed
		ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserClaims extracts JWT claims from request context
// Returns nil if claims not found (handlers should validate defensively)
func GetUserClaims(r *http.Request) *JWTClaims {
	if claims, ok := r.Context().Value(UserClaimsKey).(*JWTClaims); ok {
		return claims
	}
	return nil
}
```

**Key Points:**
- Follow requestIDMiddleware pattern for consistency
- Use slog.Info for 401s (not slog.Error - client errors, not server errors)
- Log errors with request_id for debugging (follows existing pattern)
- Generic error messages to client ("Invalid or expired token")
- Detailed logs server-side (includes actual error for debugging)
- Add import for "strings" at top of file

**Validation:**
```bash
cd backend && go fmt ./... && go vet ./...
```

### Task 2: Add helper for extracting claims from context
**File:** `backend/middleware.go`
**Action:** MODIFY (add helper function)
**Pattern:** Follow getRequestID pattern (lines 88-93)

**Implementation:**
Already included in Task 1 above (GetUserClaims function).

**Key Points:**
- Returns nil if claims not in context (defensive - handlers should check)
- Type assertion with ok check (idiomatic Go)
- Simple helper, no logging needed

**Validation:**
```bash
cd backend && go fmt ./... && go vet ./...
```

### Task 3: Update route registration to protect endpoints
**File:** `backend/main.go`
**Action:** MODIFY
**Pattern:** Chi router Group pattern (wraps routes with middleware)

**Implementation:**

Replace lines 67-70:
```go
// OLD (flat registration):
registerAuthRoutes(r)
registerAccountRoutes(r)
registerUserRoutes(r)
registerAccountUserRoutes(r)
```

With protected group pattern:
```go
// NEW (grouped with middleware):
r.Route("/api/v1", func(r chi.Router) {
	// Public endpoints (no auth required)
	registerAuthRoutes(r)  // POST /api/v1/auth/signup, /api/v1/auth/login

	// Protected endpoints (require valid JWT)
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)  // Apply auth middleware to this group

		registerAccountRoutes(r)      // All /api/v1/accounts/* routes
		registerUserRoutes(r)          // All /api/v1/users/* routes
		registerAccountUserRoutes(r)   // All /api/v1/account_users/* routes
	})
})
```

**Key Points:**
- Health checks (lines 62-64) stay outside /api/v1 - remain public
- Auth routes registered first - public (no middleware)
- All Phase 4A routes wrapped in `r.Group()` with `r.Use(authMiddleware)`
- Note: registerAuthRoutes, registerAccountRoutes, etc. already include the "/api/v1" prefix, so we need to wrap them properly

**IMPORTANT FIX:** The route registration functions (registerAuthRoutes, etc.) already include the `/api/v1` prefix. We need to update them to work with the Group pattern.

Actually, looking at auth.go:78-81:
```go
func registerAuthRoutes(r chi.Router) {
	r.Post("/api/v1/auth/signup", signupHandler)
	r.Post("/api/v1/auth/login", loginHandler)
}
```

These functions define full paths. So we need to keep the current flat structure but use r.Group() without r.Route():

**CORRECTED Implementation:**

Replace lines 67-70:
```go
// Public endpoints (no auth required)
registerAuthRoutes(r)  // /api/v1/auth/*

// Protected endpoints (require valid JWT)
r.Group(func(r chi.Router) {
	r.Use(authMiddleware)  // Apply auth middleware to this group

	registerAccountRoutes(r)      // All /api/v1/accounts/* routes
	registerUserRoutes(r)          // All /api/v1/users/* routes
	registerAccountUserRoutes(r)   // All /api/v1/account_users/* routes
})
```

**Validation:**
```bash
cd backend && go fmt ./... && go vet ./...
cd backend && go build ./...
```

### Task 4: Write integration tests
**File:** `backend/auth_middleware_test.go` (NEW)
**Action:** CREATE
**Pattern:** Follow health_test.go structure (table-driven tests with httptest)

**Implementation:**

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestAuthMiddleware_MissingToken tests 401 without Authorization header
func TestAuthMiddleware_MissingToken(t *testing.T) {
	// Setup: Create chi router with auth middleware protecting test endpoint
	r := chi.NewRouter()
	r.Use(requestIDMiddleware) // Need request_id for error responses
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			// This should never run - middleware should block
			w.WriteHeader(http.StatusOK)
		})
	})

	// Test: Request without Authorization header
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Assert: 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Assert: Response body contains error message
	body := w.Body.String()
	if !contains(body, "Missing authorization header") {
		t.Errorf("body should contain 'Missing authorization header', got: %s", body)
	}
}

// TestAuthMiddleware_InvalidToken tests 401 with malformed token
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	tests := []struct {
		name      string
		authHeader string
		wantError  string
	}{
		{
			name:      "malformed bearer format",
			authHeader: "InvalidFormat",
			wantError:  "Invalid authorization header format",
		},
		{
			name:      "invalid JWT token",
			authHeader: "Bearer invalid-token-string",
			wantError:  "Invalid or expired token",
		},
		{
			name:      "missing bearer prefix",
			authHeader: "just-a-token",
			wantError:  "Invalid authorization header format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup router
			r := chi.NewRouter()
			r.Use(requestIDMiddleware)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware)
				r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			})

			// Test with invalid token
			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			req.Header.Set("Authorization", tt.authHeader)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			// Assert 401
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
			}

			// Assert error message
			body := w.Body.String()
			if !contains(body, tt.wantError) {
				t.Errorf("body should contain %q, got: %s", tt.wantError, body)
			}
		})
	}
}

// TestAuthMiddleware_ValidToken tests 200 with valid JWT
func TestAuthMiddleware_ValidToken(t *testing.T) {
	// Generate valid JWT token
	token, err := GenerateJWT(1, "test@example.com", nil)
	if err != nil {
		t.Fatalf("GenerateJWT() failed: %v", err)
	}

	// Setup router with test endpoint that extracts claims
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			// Verify claims are in context
			claims := GetUserClaims(r)
			if claims == nil {
				t.Error("GetUserClaims() returned nil - claims not injected")
			}
			if claims != nil && claims.UserID != 1 {
				t.Errorf("claims.UserID = %d, want 1", claims.UserID)
			}
			if claims != nil && claims.Email != "test@example.com" {
				t.Errorf("claims.Email = %q, want %q", claims.Email, "test@example.com")
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	// Test with valid token
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Assert: Middleware passed, handler ran successfully
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (middleware should pass with valid token)", w.Code, http.StatusOK)
	}
}

// TestPublicEndpoints_NoAuth tests public routes work without token
func TestPublicEndpoints_NoAuth(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"health check", "GET", "/healthz"},
		{"readiness check", "GET", "/readyz"},
		{"OPTIONS for CORS", "OPTIONS", "/api/v1/auth/signup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup full router like in main.go
			r := chi.NewRouter()
			r.Use(requestIDMiddleware)
			r.Use(corsMiddleware)

			// Health checks (public)
			r.Get("/healthz", healthzHandler)
			r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
				// Simplified readyz for test (no DB)
				w.WriteHeader(http.StatusOK)
			})

			// Auth routes (public)
			r.Post("/api/v1/auth/signup", signupHandler)

			// Test without Authorization header
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			// Assert: NOT 401 (public endpoints should work)
			if w.Code == http.StatusUnauthorized {
				t.Errorf("public endpoint returned 401 - should be accessible without auth")
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

**Key Points:**
- Use httptest + real chi router (integration-style, not mocks)
- Table-driven tests where appropriate (TestAuthMiddleware_InvalidToken)
- Test actual HTTP flow (middleware → handler → response)
- Test both protected and public endpoints
- Verify claims injection in context (TestAuthMiddleware_ValidToken)

**Validation:**
```bash
cd backend && go test ./... -v -run TestAuthMiddleware
cd backend && go test ./...
```

### Task 5: Run validation gates
**File:** Multiple
**Action:** VALIDATE
**Pattern:** Use validation commands from spec/stack.md

**Implementation:**

Run validation gates in sequence:

**Gate 1: Lint & Format**
```bash
cd backend && go fmt ./...
cd backend && go vet ./...
```

**Gate 2: Type Safety**
```bash
cd backend && go build ./...
```

**Gate 3: Unit Tests**
```bash
cd backend && go test ./...
```

**Gate 4: Full Stack Validation**
```bash
just backend
```

**If any gate fails:**
1. Read error message carefully
2. Fix the specific issue
3. Re-run validation from Gate 1
4. Repeat until all gates pass

**Common issues to watch for:**
- Missing import for "strings" in middleware.go
- Typos in function names or variable names
- Router group syntax (chi v5 specific)
- Test helper functions (contains function in tests)

**Validation:**
All gates must pass before proceeding to Task 6.

### Task 6: Manual curl validation (optional but recommended)
**File:** N/A (manual testing)
**Action:** VALIDATE
**Pattern:** Quick smoke test with curl

**Prerequisites:**
```bash
# Start docker compose with database
docker-compose up -d

# Run backend server
cd backend && go run .
```

**Test 3 scenarios:**

```bash
# 1. Protected endpoint without token → 401
curl -v http://localhost:8080/api/v1/accounts
# Expected: 401 with "Missing authorization header"

# 2. Signup → get token → access protected endpoint → 200
TOKEN=$(curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123","account_name":"Test Co"}' \
  | jq -r '.data.token')

curl -v http://localhost:8080/api/v1/accounts \
  -H "Authorization: Bearer $TOKEN"
# Expected: 200 OK with accounts list

# 3. Public endpoint without token → 200
curl -v http://localhost:8080/healthz
# Expected: 200 OK
```

**If curl tests fail:**
- Check docker-compose is running (database)
- Check backend server is running
- Check error response format (should be RFC 7807)
- Check logs for detailed error messages

**Validation:**
This is optional but highly recommended to verify the auth flow works end-to-end before marking Task 6 complete.

## Risk Assessment

### Risk 1: Router group syntax
**Description:** Chi v5 Group pattern might differ from documentation examples
**Likelihood:** Low (well-documented pattern)
**Impact:** Medium (tests will catch routing issues)
**Mitigation:**
- Follow exact pattern from chi documentation
- Tests verify both protected and public routes work
- If issues arise, check chi v5 migration guide

### Risk 2: Context value type assertion
**Description:** Context value extraction could fail silently if types don't match
**Likelihood:** Low (strongly typed)
**Impact:** Low (GetUserClaims returns nil, handlers check defensively)
**Mitigation:**
- Use exact type `*JWTClaims` in type assertion
- Test verifies claims are correctly injected (TestAuthMiddleware_ValidToken)
- Defensive nil checks in both middleware and handlers

### Risk 3: Import cycles
**Description:** middleware.go might create import cycle with jwt.go or errors.go
**Likelihood:** Very Low (all in main package)
**Impact:** High (compilation failure)
**Mitigation:**
- All files are in `package main` - no import cycles possible
- Validation Gate 2 (go build) catches this immediately

### Risk 4: CORS preflight failures
**Description:** OPTIONS requests might hit auth middleware before CORS
**Likelihood:** Medium (middleware order matters)
**Impact:** High (breaks frontend auth)
**Mitigation:**
- CORS middleware is already applied globally (line 58 in main.go)
- CORS runs before auth middleware (order: requestID → recovery → CORS → contentType → auth)
- Test includes OPTIONS endpoint check (TestPublicEndpoints_NoAuth)

## Integration Points

**Middleware Stack Order (in main.go:56-59):**
1. requestIDMiddleware (required for error responses)
2. recoveryMiddleware (panic handling)
3. corsMiddleware (must run before auth for OPTIONS)
4. contentTypeMiddleware (JSON validation)
5. authMiddleware (NEW - only on protected routes via Group)

**Route Structure:**
- Health checks: Public (lines 62-64, no middleware changes)
- Auth routes: Public (registered first in Group)
- Phase 4A routes: Protected (wrapped in r.Group with authMiddleware)

**Context Flow:**
1. Request arrives → requestIDMiddleware injects request_id
2. Protected route → authMiddleware validates JWT → injects claims
3. Handler → GetUserClaims(r) → extracts claims → uses for business logic

**Error Handling:**
- Middleware uses writeJSONError (RFC 7807 format)
- Includes request_id automatically (from requestIDMiddleware)
- Logs detailed errors server-side (with request_id for correlation)
- Returns generic messages to client (security best practice)

## VALIDATION GATES (MANDATORY)

**CRITICAL:** These are blocking gates, not suggestions.

After EVERY code change (each task):

**Gate 1: Lint & Format**
```bash
cd backend && go fmt ./...
cd backend && go vet ./...
```

**Gate 2: Type Safety**
```bash
cd backend && go build ./...
```

**Gate 3: Unit Tests**
```bash
cd backend && go test ./...
```

**Enforcement Rules:**
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and report issue

**Do not proceed to next task until current task passes all gates.**

**Final validation (after all tasks):**
```bash
just backend
```

## Validation Sequence

### After Each Task:
```bash
cd backend && go fmt ./...
cd backend && go vet ./...
cd backend && go build ./...
cd backend && go test ./...
```

### Final Validation:
```bash
just backend  # Runs all backend checks
```

### Optional Manual Validation:
```bash
# See Task 6 for curl commands
docker-compose up -d
cd backend && go run .
# Run 3 curl commands from Task 6
```

## Plan Quality Assessment

**Complexity Score:** 2/10 (LOW - WELL-SCOPED)

**Confidence Score:** 9/10 (HIGH)

**Confidence Factors:**
✅ Clear requirements from spec
✅ Similar patterns found in codebase at middleware.go:16-27
✅ All clarifying questions answered with industry best practices
✅ Existing test patterns to follow at health_test.go:11-54
✅ JWT validation already implemented and tested (Phase 5A)
✅ No new dependencies - uses existing chi router and JWT utils
✅ Small scope - 3 files, ~200 LOC total
⚠️ Chi router Group pattern new to this codebase (but well-documented)

**Assessment:** High confidence implementation. Clear patterns to follow, small scope, well-tested foundation from Phase 5A/5B. The only minor uncertainty is chi Group syntax, but that's well-documented and tests will catch any issues.

**Estimated one-pass success probability:** 85%

**Reasoning:**
- Strong foundation from Phase 5A (ValidateJWT tested and working)
- Clear patterns in existing middleware.go to follow
- Comprehensive test coverage will catch integration issues
- Small scope reduces complexity
- Only risk is chi Group syntax, which is well-documented
- Go's strong typing and compilation checks catch most errors early

**Risk Mitigation:**
- Tests verify both protected and public routes work correctly
- Validation gates run after every task (early error detection)
- Defensive nil checks prevent runtime panics
- Generic error messages protect against info disclosure
- Server-side logging with request_id enables debugging
