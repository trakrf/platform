# Feature: Add Sentry Error Tracking to Go Backend

## Metadata
**Workspace**: backend
**Type**: feature
**Linear Issue**: [TRA-257](https://linear.app/trakrf/issue/TRA-257/add-sentry-to-go-backend)
**Parent**: [TRA-148](https://linear.app/trakrf/issue/TRA-148/add-production-monitoring-and-alerting) - Production monitoring initiative

## Outcome
Production errors and panics will be automatically captured and reported to Sentry with full stack traces and request context, enabling rapid debugging and monitoring.

## User Story
As a **developer/operator**
I want **application errors to be automatically captured and reported to Sentry**
So that **I can quickly identify, diagnose, and fix production issues before they impact customers**

## Context
**Current**: The backend has a Recovery middleware (`backend/internal/middleware/middleware.go:37-57`) that catches panics and logs them via zerolog, but errors are only visible in Railway logs with no aggregation, alerting, or tracking.

**Desired**: Errors and panics are captured by Sentry with:
- Full stack traces
- Request context (path, method, request ID, user ID if authenticated)
- Error grouping and deduplication
- Slack/email alerts on new errors

**Why Now**: Moving toward production with paying customers (NADA verbal commitment). Need production-grade observability before accepting payments.

**Examples**:
- Existing middleware chain in `backend/main.go:107-111`
- Structured logging already in place via zerolog

## Prerequisites
- [x] Sentry account created
- [x] Sentry project created (Go NET/HTTP)
- [x] `SENTRY_DSN` configured in Railway for platform-preview
- [x] `SENTRY_DSN` configured in Railway for platform-prod

## Technical Requirements

### 1. Sentry SDK Integration
- Add `github.com/getsentry/sentry-go` dependency
- Initialize Sentry client in `main()` before other services
- Flush Sentry on graceful shutdown

### 2. Chi HTTP Middleware
- Add `sentryhttp.New()` middleware to capture HTTP panics
- Place BEFORE the Recovery middleware in chain (so Sentry captures first, then Recovery returns 500)
- Configure `Repanic: true` so existing Recovery middleware still works

### 3. Request Context Enrichment
- Capture request ID from existing middleware
- Capture user ID from JWT claims when authenticated
- Include environment (dev/staging/prod) and version tags

### 4. Configuration
- `SENTRY_DSN` environment variable (empty = disabled)
- `APP_ENV` for environment tag (already exists)
- `version` variable for release tracking (already exists)

### 5. Railway Configuration
- Add `SENTRY_DSN` to preview and production environments
- Keep empty/unset in development to avoid noise

## Implementation Notes

```go
// Initialization in main.go
import (
    "github.com/getsentry/sentry-go"
    sentryhttp "github.com/getsentry/sentry-go/http"
)

func main() {
    // Initialize Sentry early
    if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
        err := sentry.Init(sentry.ClientOptions{
            Dsn:              dsn,
            Environment:      os.Getenv("APP_ENV"),
            Release:          version,
            EnableTracing:    false, // Can enable later for performance monitoring
        })
        if err != nil {
            log.Error().Err(err).Msg("Sentry initialization failed")
        }
    }
    defer sentry.Flush(2 * time.Second)

    // ... rest of main
}

// Middleware chain in setupRouter
r.Use(middleware.RequestID)
r.Use(logger.Middleware)
r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle) // NEW - before Recovery
r.Use(middleware.Recovery)
r.Use(middleware.CORS)
r.Use(middleware.ContentType)
```

## Out of Scope
- Frontend Sentry integration (separate ticket)
- Performance/tracing (can be added later via `EnableTracing`)
- Custom error boundaries in handlers (can capture manually later)
- Sentry crons monitoring

## Validation Criteria
- [ ] Sentry SDK compiles and initializes without errors
- [ ] `SENTRY_DSN` empty = no Sentry calls (safe for local dev)
- [ ] Panics in HTTP handlers are captured in Sentry
- [ ] Captured events include: stack trace, request path, method, request ID
- [ ] Graceful shutdown flushes pending events
- [ ] Backend tests still pass
- [ ] No Sentry errors in Railway deployment logs

## Success Metrics
- [ ] Test panic appears in Sentry dashboard within 30 seconds
- [ ] Error includes identifiable context (request path, request ID, environment)
- [ ] Sentry dashboard shows correct environment (preview vs production)
- [ ] Zero increase in request latency (Sentry is async)

## Verification Steps
1. Deploy to preview environment with `SENTRY_DSN` configured
2. Trigger a test panic via a temporary endpoint or manual code path
3. Confirm error appears in Sentry with full context
4. Remove test panic code
5. Monitor for any legitimate errors

## References
- [Sentry Go SDK](https://docs.sentry.io/platforms/go/)
- [Sentry HTTP Integration](https://docs.sentry.io/platforms/go/guides/http/)
- [Sentry Chi Middleware](https://docs.sentry.io/platforms/go/guides/http/net-http/) (works with Chi)
- Existing Recovery middleware: `backend/internal/middleware/middleware.go:37-57`
- Existing middleware chain: `backend/main.go:107-111`
