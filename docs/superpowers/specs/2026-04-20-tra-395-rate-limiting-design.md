# TRA-395 — Rate limiting middleware

Per-key rate limiting middleware for the public API. Matches the already-published docs at `docs.preview.trakrf.id/docs/api/rate-limits/` exactly.

## Problem

The public API is open to API-key-authenticated consumers (TRA-393) with no protection against runaway clients. Docs site already advertises specific rate-limit behavior — 60/min steady, 120 burst, `X-RateLimit-*` headers, 429 with `Retry-After`. Backend must match. No tier differentiation yet; TRA-337 owns pricing tiers and will swap the constants for a lookup when it lands.

## Scope

In:
- Token-bucket rate limiter keyed by `api_keys.jti` (JWT ID).
- HTTP middleware that emits `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` on API-key-authenticated responses and returns 429 + `Retry-After` when buckets are exhausted.
- TTL eviction of idle buckets.
- Clock-injected unit tests and httptest-level middleware tests.

Out (deferred):
- Redis / multi-instance backend (single-instance Railway holds).
- Tier-based limits — TRA-337.
- Per-route overrides, env-var overrides, Prometheus metrics — no current driver.

## Architecture

Two packages:

```
backend/internal/
├── ratelimit/
│   ├── clock.go            # Clock interface + RealClock + FakeClock
│   ├── limiter.go          # Limiter + bucket, sweep goroutine
│   └── limiter_test.go     # Unit tests, synthetic clock
└── middleware/
    ├── ratelimit.go        # HTTP adapter; reads APIKeyPrincipal, emits headers
    └── ratelimit_test.go   # httptest tests
```

The `ratelimit` package owns the limiter, bucket state, and sweep loop — decoupled from HTTP so it is testable without a server and reusable if a CLI or different transport ever needs it. The `middleware` package is a thin HTTP skin.

## Data structures

```go
// package ratelimit

type Clock interface {
    Now() time.Time
}

type Decision struct {
    Allowed    bool
    Limit      int           // steady-state requests per minute
    Remaining  int           // tokens left, floored at 0
    ResetAt    time.Time     // when bucket fully refills
    RetryAfter time.Duration // 0 when Allowed; else time until next token
}

type Config struct {
    RatePerMinute int
    Burst         int
    IdleTTL       time.Duration
    SweepInterval time.Duration
    Clock         Clock
}

type Limiter struct {
    cfg     Config
    buckets sync.Map // key string -> *bucket
    stop    chan struct{}
}

type bucket struct {
    lim      *rate.Limiter
    lastSeen atomic.Int64 // unix nanos
}

func DefaultConfig() Config // returns 60, 120, 1h, 10m, RealClock
func NewLimiter(cfg Config) *Limiter
func (l *Limiter) Allow(key string) Decision
func (l *Limiter) Close()
```

Thread safety: `sync.Map` for Load/Store, `rate.Limiter` is internally locked, `lastSeen` is atomic. No external mutex.

## Allow logic

1. Load or create bucket. First-time keys get a full burst (`rate.NewLimiter(rate.Limit(rpm/60), burst)`).
2. Update `lastSeen` with `clock.Now().UnixNano()`.
3. Call `bucket.lim.AllowN(clock.Now(), 1)`.
4. `Remaining = max(0, int(bucket.lim.TokensAt(clock.Now())))`.
5. `ResetAt = clock.Now() + time.Duration((burst - Remaining) / rate_per_sec * float64(time.Second))`.
6. On reject, `RetryAfter = ceil(1s / rate_per_sec)` — time until next single token.

## Sweeper

Ticker fires every `SweepInterval`. Walks `buckets`, deletes entries where `now - lastSeen > IdleTTL`. Swept keys that return re-materialize with a full burst — acceptable because an hour-idle client's bucket would already have refilled to full, so the eviction is invisible in practice.

Started by `NewLimiter`, stopped by `Close` via `stop` channel.

## Middleware

```go
func RateLimit(limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            p := GetAPIKeyPrincipal(r)
            if p == nil {
                next.ServeHTTP(w, r) // session auth — not rate limited
                return
            }

            d := limiter.Allow(p.JTI)
            w.Header().Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
            w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
            w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(d.ResetAt.Unix(), 10))

            if !d.Allowed {
                retrySec := int(math.Ceil(d.RetryAfter.Seconds()))
                w.Header().Set("Retry-After", strconv.Itoa(retrySec))
                httputil.WriteJSONError(w, r, http.StatusTooManyRequests,
                    errors.ErrRateLimited,
                    fmt.Sprintf("Retry after %d seconds", retrySec),
                    r.URL.Path, GetRequestID(r))
                logger.Get().Warn().
                    Str("request_id", GetRequestID(r)).
                    Str("jti", p.JTI).
                    Int("org_id", p.OrgID).
                    Str("path", r.URL.Path).
                    Msg("rate limit exceeded")
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

### 429 response body

```json
{
  "error": {
    "type": "rate_limited",
    "title": "Rate limit exceeded",
    "status": 429,
    "detail": "Retry after 30 seconds",
    "instance": "/api/v1/assets",
    "request_id": "01J..."
  }
}
```

Matches published docs verbatim. Uses existing `httputil.WriteJSONError` (same helper `APIKeyAuth` uses for 401s) with `errors.ErrRateLimited` — shape is identical to other error responses by construction.

## Mount points in `router.go`

```go
rl := ratelimit.NewLimiter(ratelimit.DefaultConfig())

// /orgs/me — excluded (health-check exemption). Structurally outside the rate-limited group.
r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", orgsHandler.GetOrgMe)

// Public API routes — EitherAuth accepts both API key and session; RateLimit no-ops on session.
r.Group(func(r chi.Router) {
    r.Use(middleware.EitherAuth(store))
    r.Use(middleware.RateLimit(rl))
    r.Use(middleware.SentryContext)
    // ... existing scoped routes
})
```

Write endpoints (TRA-397) will land inside this group and be rate-limited automatically.

## Testing

`ratelimit/limiter_test.go` — pure unit, synthetic `FakeClock`:
- Fresh key has burst tokens, first request returns `Remaining = burst - 1`.
- Exhaust burst → next request rejected with `RetryAfter ≈ 1s`.
- Advance clock 1s → one token refilled, next request allowed.
- Two distinct keys are independent.
- Sweep removes entries past `IdleTTL`; returning key gets a fresh full bucket.

`middleware/ratelimit_test.go` — httptest, no DB:
- With `APIKeyPrincipal` attached → headers set on 2xx, 429 on exhaustion, body matches published shape byte-for-byte.
- Without principal (session auth) → no headers, limiter not touched.

Integration smoke: one test in `apikey_test.go` style that boots the chi router and confirms the middleware chain wires up end-to-end.

## Observability

Log at `Warn` only on rejection — fields `request_id`, `jti`, `org_id`, `path`, `method`. No log on allowed requests (volume). No Prometheus metrics in this PR.

## OpenAPI

Verify `docs/api/openapi.public.yaml` already documents 429 responses (Explore survey said yes). Add `X-RateLimit-*` header definitions to 2xx responses if missing.

## Dependencies

- Add `golang.org/x/time/rate` to `go.mod`.
- No new runtime dependencies beyond that.

## Cross-references

- TRA-393 — API key auth (Done); provides `APIKeyPrincipal.JTI`.
- TRA-337 — pricing tiers; will replace `DefaultConfig()` constants with a per-key lookup. Comment posted.
- TRA-392 — public API design; this spec is a deeper cut of its rate-limit section.
- TRA-408 — docs coordination; not needed because this implementation already matches published docs.
- TRA-397 — write endpoints; lands in the same EitherAuth group and gains rate limiting automatically.
