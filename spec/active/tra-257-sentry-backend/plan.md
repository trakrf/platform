# Implementation Plan: Add Sentry Error Tracking to Go Backend

Generated: 2026-01-14
Specification: spec.md

## Understanding

Add Sentry error tracking to the Go backend with:
1. SDK initialization in `main()` with graceful degradation (log warning if fails)
2. HTTP middleware to capture panics with request context
3. Custom middleware to enrich Sentry scope with request ID and user ID
4. Test endpoint in existing `testhandler` package for verification

## Relevant Files

**Reference Patterns**:
- `backend/main.go` (lines 107-111) - Existing middleware chain order
- `backend/main.go` (lines 149-175) - Initialization pattern with error handling
- `backend/internal/middleware/middleware.go` (lines 37-57) - Recovery middleware pattern
- `backend/internal/handlers/testhandler/invitations.go` (lines 75-79) - Test route registration

**Files to Modify**:
- `backend/go.mod` - Add sentry-go dependency
- `backend/main.go` - Initialize Sentry, add middleware to chain
- `backend/internal/middleware/middleware.go` - Add Sentry context enrichment middleware
- `backend/internal/handlers/testhandler/invitations.go` - Add Sentry test endpoint

## Architecture Impact
- **Subsystems affected**: Backend initialization, middleware
- **New dependencies**: `github.com/getsentry/sentry-go`
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add Sentry Dependency
**File**: `backend/go.mod`
**Action**: MODIFY (via go get)

**Implementation**:
```bash
cd backend && go get github.com/getsentry/sentry-go
```

**Validation**:
```bash
cd backend && just lint
```

---

### Task 2: Initialize Sentry in main()
**File**: `backend/main.go`
**Action**: MODIFY
**Pattern**: Reference initialization section (lines 149-175)

**Implementation**:
```go
// Add imports
import (
    "github.com/getsentry/sentry-go"
    sentryhttp "github.com/getsentry/sentry-go/http"
)

// In main(), after logger initialization, before storage:
if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
    err := sentry.Init(sentry.ClientOptions{
        Dsn:           dsn,
        Environment:   os.Getenv("APP_ENV"),
        Release:       version,
        EnableTracing: false,
    })
    if err != nil {
        log.Warn().Err(err).Msg("Sentry initialization failed")
    } else {
        log.Info().Msg("Sentry initialized")
    }
}
defer sentry.Flush(2 * time.Second)
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 3: Add Sentry HTTP Middleware to Router
**File**: `backend/main.go`
**Action**: MODIFY
**Pattern**: Reference setupRouter middleware chain (lines 107-111)

**Implementation**:
```go
// In setupRouter(), add sentryhttp middleware BEFORE Recovery:
r.Use(middleware.RequestID)
r.Use(logger.Middleware)
r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle) // NEW
r.Use(middleware.Recovery)
r.Use(middleware.CORS)
r.Use(middleware.ContentType)
```

**Note**: `Repanic: true` ensures the panic propagates to Recovery middleware after Sentry captures it.

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 4: Add Sentry Context Enrichment Middleware
**File**: `backend/internal/middleware/middleware.go`
**Action**: MODIFY
**Pattern**: Reference Recovery middleware (lines 37-57)

**Implementation**:
```go
import "github.com/getsentry/sentry-go"

// SentryContext enriches Sentry scope with request ID and user info.
// Should be placed AFTER RequestID and Auth middlewares in the chain.
func SentryContext(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
            hub.Scope().SetTag("request_id", GetRequestID(r.Context()))

            if claims := GetUserClaims(r); claims != nil {
                hub.Scope().SetUser(sentry.User{
                    ID:    claims.UserID,
                    Email: claims.Email,
                })
            }
        }
        next.ServeHTTP(w, r)
    })
}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 5: Wire Context Middleware in Router
**File**: `backend/main.go`
**Action**: MODIFY

**Implementation**:
Add `middleware.SentryContext` inside the authenticated route group to capture user context:

```go
r.Group(func(r chi.Router) {
    r.Use(middleware.Auth)
    r.Use(middleware.SentryContext) // NEW - after Auth so user claims are available

    orgsHandler.RegisterRoutes(r, store)
    // ... rest of routes
})
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 6: Add Sentry Test Endpoint
**File**: `backend/internal/handlers/testhandler/invitations.go`
**Action**: MODIFY
**Pattern**: Reference existing test endpoint (lines 28-71)

**Implementation**:
```go
// SentryTest triggers a test panic to verify Sentry integration.
// GET /test/sentry
func (h *Handler) SentryTest(w http.ResponseWriter, r *http.Request) {
    panic("Sentry test panic - this should appear in Sentry dashboard")
}

// In RegisterRoutes:
r.Route("/test", func(r chi.Router) {
    r.Get("/invitations/{id}/token", h.GetInvitationToken)
    r.Get("/sentry", h.SentryTest) // NEW
})
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 7: Final Validation
**Action**: Full test suite and build

**Validation**:
```bash
cd backend && just validate
```

## Risk Assessment

- **Risk**: Sentry middleware slows down requests
  **Mitigation**: Sentry SDK is async by default, minimal overhead. Monitor latency after deploy.

- **Risk**: Sensitive data leaked to Sentry
  **Mitigation**: Only capturing user ID/email (already in JWT), request ID, and standard HTTP context. No request bodies or headers beyond defaults.

- **Risk**: Sentry unavailable blocks app
  **Mitigation**: Init failure only logs warning, app continues. `defer sentry.Flush()` has 2s timeout.

## Integration Points

- **Middleware chain**: Sentry HTTP middleware added before Recovery
- **Auth middleware**: SentryContext depends on Auth middleware running first
- **Environment variables**: `SENTRY_DSN`, `APP_ENV` (existing)
- **Test routes**: New `/test/sentry` endpoint (non-production only)

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd backend && just lint    # Gate 1: Syntax & Style
cd backend && just test    # Gate 2: Tests
cd backend && just build   # Gate 3: Build
```

**Final validation**:
```bash
just backend validate
```

## Verification After Deploy

1. Deploy to preview environment
2. Call `GET /test/sentry` to trigger test panic
3. Verify error appears in Sentry dashboard with:
   - Stack trace
   - Environment = preview
   - Request path = /test/sentry
4. Confirm app recovered (didn't crash)

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Well-documented Sentry Go SDK
- ✅ Existing middleware patterns to follow
- ✅ Test infrastructure already exists
- ✅ All clarifying questions answered

**Assessment**: Straightforward SDK integration following existing patterns.

**Estimated one-pass success probability**: 95%

**Reasoning**: Single dependency, clear integration points, existing test infrastructure, well-documented SDK.
