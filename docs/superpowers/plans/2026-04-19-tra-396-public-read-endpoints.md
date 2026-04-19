# TRA-396 Read-only public API endpoints — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire API-key authentication + scope enforcement to the first tranche of public read endpoints (assets, locations, current-locations, asset history) with normalized response shapes, natural-key path parameters, and enforced request conventions.

**Architecture:** Same chi router, shared handler functions, new middleware chain (`EitherAuth` dispatches API-key vs session auth based on JWT `iss`). Internal surrogate lookups move to `/api/v1/{resource}/by-id/{id}` paths; public uses `/api/v1/{resource}/{identifier}`. Single public response shape for both auth types — `org_id`/`deleted_at` dropped, temporal fields kept, `current_location`/`parent` resolved via one indexed LEFT JOIN each.

**Tech Stack:** Go 1.22 backend (chi, pgx, JWT-via-golang-jwt/v5), React + TypeScript frontend (Vite, Vitest, axios, Zustand, React Query), Postgres with TimescaleDB.

**Reference spec:** `docs/superpowers/specs/2026-04-19-tra-396-public-read-endpoints-design.md`

**Context for executors:**
- Always run commands from the project root. Use `just backend <cmd>` / `just frontend <cmd>` delegation rather than `cd`.
- Backend integration tests require `INTEGRATION_TESTS=1` env and a live Postgres (see `backend/internal/testutil/database.go`).
- Conventional commits. `feat(tra-396):`, `test(tra-396):`, `refactor(tra-396):`.
- Frequent incremental commits. Do NOT squash or amend during execution.

---

## File Structure

**Backend — new files:**
- `backend/internal/util/jwt/peek.go` — `PeekIssuer` helper
- `backend/internal/util/jwt/peek_test.go` — peek tests
- `backend/internal/middleware/either_auth.go` — `EitherAuth` middleware
- `backend/internal/middleware/either_auth_test.go` — EitherAuth tests
- `backend/internal/middleware/orgresolver.go` — `GetRequestOrgID` helper
- `backend/internal/middleware/orgresolver_test.go` — helper tests
- `backend/internal/util/httputil/listparams.go` — `ParseListParams` helper
- `backend/internal/util/httputil/listparams_test.go` — parser tests
- `backend/internal/models/asset/public.go` — `PublicAssetView` shape
- `backend/internal/models/location/public.go` — `PublicLocationView` shape
- `backend/internal/models/report/public.go` — public shapes for current-locations and asset-history
- `backend/internal/handlers/assets/public_integration_test.go` — API-key flow tests
- `backend/internal/handlers/locations/public_integration_test.go` — API-key flow tests
- `backend/internal/handlers/reports/public_integration_test.go` — API-key flow tests

**Backend — modified files:**
- `backend/internal/middleware/apikey.go` — `RequireScope` becomes principal-aware
- `backend/internal/middleware/middleware.go` — no changes to `Auth`, but re-export types
- `backend/internal/storage/assets.go` — add `GetAssetByIdentifier`, update `ListAllAssets` / `CountAllAssets` to take filter struct with join
- `backend/internal/storage/locations.go` — add `GetLocationByIdentifier`, update list to take filter struct
- `backend/internal/storage/reports.go` — accept natural-key filter, rename `Search`→`Q`, `StartDate`→`From`, `EndDate`→`To`, move identifier/location name to joins
- `backend/internal/models/asset/asset.go` — filter struct additions
- `backend/internal/models/location/location.go` — filter struct additions
- `backend/internal/models/report/report.go` — filter/response rename, natural-key fields
- `backend/internal/handlers/assets/assets.go` — refactor to use helpers + emit `PublicAssetView`
- `backend/internal/handlers/locations/locations.go` — refactor parallel to assets
- `backend/internal/handlers/reports/current_locations.go` — consume new filter + emit public shape
- `backend/internal/handlers/reports/asset_history.go` — consume new filter + emit public shape; add by-id variant
- `backend/internal/cmd/serve/router.go` — register new public and by-id routes; remove old routes

**Frontend — modified files:**
- `frontend/src/lib/api/` — new file `assets.ts`, `locations.ts`, `reports.ts` containing typed API-client wrappers (if not already present) OR update existing call sites
- `frontend/src/types/` — update `Asset`, `Location`, list-envelope types to match new shape
- `frontend/src/components/AssetsScreen.tsx` (and related) — adapt field reads
- `frontend/src/components/LocationsScreen.tsx` (and related) — adapt field reads
- `frontend/src/components/reports/CurrentLocations.tsx` — new URL + filter rename
- `frontend/src/components/reports/AssetHistory.tsx` — new URL + filter rename

**Removed endpoints (no back-compat shim):**
- `GET /api/v1/assets/{id}` (replaced by `{identifier}` public + `by-id/{id}` internal)
- `GET /api/v1/locations/{id}` (same pattern)
- `GET /api/v1/reports/current-locations` → `/api/v1/locations/current`
- `GET /api/v1/reports/assets/{id}/history` → `/api/v1/assets/{identifier}/history` + `/api/v1/assets/by-id/{id}/history`

---

## Task 1: `jwt.PeekIssuer` helper

**Files:**
- Create: `backend/internal/util/jwt/peek.go`
- Create: `backend/internal/util/jwt/peek_test.go`

- [ ] **Step 1.1: Write the failing test**

File: `backend/internal/util/jwt/peek_test.go`

```go
package jwt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestPeekIssuer_APIKeyToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "peek-test")
	tok, err := jwt.GenerateAPIKey("jti-1", 42, []string{"assets:read"}, nil)
	require.NoError(t, err)

	iss, err := jwt.PeekIssuer(tok)
	require.NoError(t, err)
	assert.Equal(t, "trakrf-api-key", iss)
}

func TestPeekIssuer_SessionToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "peek-test")
	orgID := 7
	tok, err := jwt.Generate(1, "u@e.com", &orgID)
	require.NoError(t, err)

	iss, err := jwt.PeekIssuer(tok)
	require.NoError(t, err)
	assert.Equal(t, "", iss, "session JWTs carry no iss claim")
}

func TestPeekIssuer_Garbage(t *testing.T) {
	_, err := jwt.PeekIssuer("not-a-jwt")
	assert.Error(t, err)
}

func TestPeekIssuer_ExpiredTokenStillPeeks(t *testing.T) {
	t.Setenv("JWT_SECRET", "peek-test")
	past := time.Now().Add(-1 * time.Hour)
	tok, err := jwt.GenerateAPIKey("jti-exp", 42, []string{"x"}, &past)
	require.NoError(t, err)

	// Full validation would reject expired; peek should still return iss.
	iss, err := jwt.PeekIssuer(tok)
	require.NoError(t, err)
	assert.Equal(t, "trakrf-api-key", iss)
}
```

- [ ] **Step 1.2: Run test to verify it fails**

```bash
just backend test ./internal/util/jwt/... -run PeekIssuer
```

Expected: compile failure — `jwt.PeekIssuer` undefined.

- [ ] **Step 1.3: Implement PeekIssuer**

File: `backend/internal/util/jwt/peek.go`

```go
package jwt

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// PeekIssuer parses the JWT body without verifying signature or expiry,
// returning the "iss" claim. Used by middleware.EitherAuth to pick between
// session and API-key validation chains; full validation runs downstream.
// Safe because peek authorizes nothing on its own.
func PeekIssuer(tokenString string) (string, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	tok, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("peek jwt: %w", err)
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("peek jwt: unexpected claims type")
	}
	iss, _ := claims["iss"].(string)
	return iss, nil
}
```

- [ ] **Step 1.4: Run tests to verify pass**

```bash
just backend test ./internal/util/jwt/... -run PeekIssuer
```

Expected: 4 PASS.

- [ ] **Step 1.5: Commit**

```bash
git add backend/internal/util/jwt/peek.go backend/internal/util/jwt/peek_test.go
git commit -m "feat(tra-396): jwt.PeekIssuer for unverified iss read"
```

---

## Task 2: `middleware.EitherAuth`

**Files:**
- Create: `backend/internal/middleware/either_auth.go`
- Create: `backend/internal/middleware/either_auth_test.go`

- [ ] **Step 2.1: Write the failing test**

File: `backend/internal/middleware/either_auth_test.go`

```go
//go:build integration
// +build integration

package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupEitherAuth(t *testing.T) (*storage.Storage, func(), int, int, string, string) {
	t.Setenv("JWT_SECRET", "either-test")
	store, cleanup := testutil.SetupTestDB(t)
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('ea', 'ea@example.com', 'stub') RETURNING id`,
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "ea-key",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)

	apiTok, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
	require.NoError(t, err)

	sessTok, err := jwt.Generate(userID, "ea@example.com", &orgID)
	require.NoError(t, err)

	return store, cleanup, orgID, userID, apiTok, sessTok
}

func echoPrincipalHandler(w http.ResponseWriter, r *http.Request) {
	if p := middleware.GetAPIKeyPrincipal(r); p != nil {
		w.Header().Set("X-Principal", "api-key")
		return
	}
	if c := middleware.GetUserClaims(r); c != nil {
		w.Header().Set("X-Principal", "session")
		return
	}
	http.Error(w, "no principal", http.StatusInternalServerError)
}

func TestEitherAuth_DispatchesAPIKey(t *testing.T) {
	store, cleanup, _, _, apiTok, _ := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+apiTok)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "api-key", w.Header().Get("X-Principal"))
}

func TestEitherAuth_DispatchesSession(t *testing.T) {
	store, cleanup, _, _, _, sessTok := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+sessTok)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "session", w.Header().Get("X-Principal"))
}

func TestEitherAuth_MissingHeader(t *testing.T) {
	store, cleanup, _, _, _, _ := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestEitherAuth_UnknownIssuer(t *testing.T) {
	store, cleanup, _, _, _, _ := setupEitherAuth(t)
	defer cleanup()

	// Hand-forged JWT with iss="attacker". Signature won't verify, but EitherAuth
	// must reject at dispatch time based on iss (no chain accepts this iss).
	forged := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJhdHRhY2tlciJ9.sig"
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestEitherAuth_GarbageToken(t *testing.T) {
	store, cleanup, _, _, _, _ := setupEitherAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()
	middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
```

- [ ] **Step 2.2: Run test to verify it fails**

```bash
just backend test-integration ./internal/middleware/... -run EitherAuth
```

Expected: compile failure.

- [ ] **Step 2.3: Implement EitherAuth**

File: `backend/internal/middleware/either_auth.go`

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

const apiKeyIssuer = "trakrf-api-key"

// EitherAuth dispatches a request to APIKeyAuth or session Auth based on the
// JWT's "iss" claim. Public read routes use this so the frontend (session) and
// external API-key callers share one handler registration.
//
// The peek at iss is unverified; the delegated chain runs full signature +
// expiry + revocation validation. Peek authorizes nothing on its own.
func EitherAuth(store *storage.Storage) func(http.Handler) http.Handler {
	apiChain := APIKeyAuth(store)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Missing authorization header", "", reqID)
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Invalid authorization header format", "", reqID)
				return
			}

			iss, err := jwt.PeekIssuer(parts[1])
			if err != nil {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Invalid or malformed token", "", reqID)
				return
			}

			switch iss {
			case apiKeyIssuer:
				apiChain(next).ServeHTTP(w, r)
			case "":
				Auth(next).ServeHTTP(w, r)
			default:
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Invalid or expired token", "", reqID)
			}
		})
	}
}
```

- [ ] **Step 2.4: Run tests to verify pass**

```bash
just backend test-integration ./internal/middleware/... -run EitherAuth
```

Expected: 5 PASS.

- [ ] **Step 2.5: Commit**

```bash
git add backend/internal/middleware/either_auth.go backend/internal/middleware/either_auth_test.go
git commit -m "feat(tra-396): EitherAuth middleware dispatches on JWT iss claim"
```

---

## Task 3: `RequireScope` becomes principal-aware

Session principals pass through without scope check; API-key principals keep existing behavior.

**Files:**
- Modify: `backend/internal/middleware/apikey.go:106-127`
- Modify: `backend/internal/middleware/apikey_test.go` (add session pass-through test)

- [ ] **Step 3.1: Write the failing test**

Add to `backend/internal/middleware/apikey_test.go`:

```go
func TestRequireScope_SessionPassthrough(t *testing.T) {
	t.Setenv("JWT_SECRET", "rs-pass")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	orgID := 1
	sessionToken, err := jwt.Generate(1, "u@e.com", &orgID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()

	// Use session Auth, then RequireScope("assets:read") — session principal
	// has no scopes, but RequireScope must pass through.
	chain := middleware.Auth(middleware.RequireScope("assets:read")(http.HandlerFunc(protectedHandler)))
	chain.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"protectedHandler writes 500 for session principal because it only looks for APIKeyPrincipal — scope check itself should have passed")
}
```

Note: `protectedHandler` in the existing test file only inspects `APIKeyPrincipal`; a session passthrough will reach it but 500 because no api-key principal. That proves scope did not reject. For clarity, write a helper that echoes "ok" for session too:

```go
func echoAnyPrincipalHandler(w http.ResponseWriter, r *http.Request) {
	if p := middleware.GetAPIKeyPrincipal(r); p != nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`api-key`))
		return
	}
	if c := middleware.GetUserClaims(r); c != nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`session`))
		return
	}
	http.Error(w, "no principal", http.StatusInternalServerError)
}
```

Replace the assertion in `TestRequireScope_SessionPassthrough` with:

```go
chain := middleware.Auth(middleware.RequireScope("assets:read")(http.HandlerFunc(echoAnyPrincipalHandler)))
chain.ServeHTTP(w, req)

require.Equal(t, http.StatusOK, w.Code)
assert.Equal(t, "session", w.Body.String())
```

- [ ] **Step 3.2: Run test to verify it fails**

```bash
just backend test-integration ./internal/middleware/... -run TestRequireScope_SessionPassthrough
```

Expected: FAIL — current `RequireScope` rejects session principals with 401 ("Authentication required").

- [ ] **Step 3.3: Update RequireScope**

Replace the `RequireScope` function in `backend/internal/middleware/apikey.go`:

```go
// RequireScope rejects API-key requests whose principal lacks the given scope.
// Session-auth requests (UserClaims present) pass through; their access is
// governed elsewhere. Must be chained after EitherAuth or APIKeyAuth.
func RequireScope(required string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			// Session principal → pass through.
			if GetUserClaims(r) != nil {
				next.ServeHTTP(w, r)
				return
			}

			p := GetAPIKeyPrincipal(r)
			if p == nil {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Authentication required", "", reqID)
				return
			}
			for _, s := range p.Scopes {
				if s == required {
					next.ServeHTTP(w, r)
					return
				}
			}
			httputil.WriteJSONError(w, r, http.StatusForbidden,
				errors.ErrForbidden, "Forbidden",
				"Missing required scope: "+required, reqID)
		})
	}
}
```

- [ ] **Step 3.4: Run tests**

```bash
just backend test-integration ./internal/middleware/... -run TestRequireScope
```

Expected: All `TestRequireScope*` PASS (existing API-key 403/200 tests still pass; new session passthrough test passes).

- [ ] **Step 3.5: Commit**

```bash
git add backend/internal/middleware/apikey.go backend/internal/middleware/apikey_test.go
git commit -m "feat(tra-396): RequireScope passes session-auth principals through"
```

---

## Task 4: `GetRequestOrgID` principal-agnostic helper

**Files:**
- Create: `backend/internal/middleware/orgresolver.go`
- Create: `backend/internal/middleware/orgresolver_test.go`

- [ ] **Step 4.1: Write the failing test**

File: `backend/internal/middleware/orgresolver_test.go`

```go
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestGetRequestOrgID_APIKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), APIKeyPrincipalKey, &APIKeyPrincipal{OrgID: 99, Scopes: []string{"x"}})
	req = req.WithContext(ctx)

	org, err := GetRequestOrgID(req)
	assert.NoError(t, err)
	assert.Equal(t, 99, org)
}

func TestGetRequestOrgID_Session(t *testing.T) {
	orgID := 42
	claims := &jwt.Claims{UserID: 1, Email: "u@e.com", CurrentOrgID: &orgID}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserClaimsKey, claims)
	req = req.WithContext(ctx)

	org, err := GetRequestOrgID(req)
	assert.NoError(t, err)
	assert.Equal(t, 42, org)
}

func TestGetRequestOrgID_SessionWithoutOrg(t *testing.T) {
	claims := &jwt.Claims{UserID: 1, Email: "u@e.com", CurrentOrgID: nil}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserClaimsKey, claims)
	req = req.WithContext(ctx)

	_, err := GetRequestOrgID(req)
	assert.Error(t, err)
}

func TestGetRequestOrgID_NoPrincipal(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := GetRequestOrgID(req)
	assert.Error(t, err)
}
```

- [ ] **Step 4.2: Run test to verify it fails**

```bash
just backend test ./internal/middleware/... -run GetRequestOrgID
```

Expected: compile failure.

- [ ] **Step 4.3: Implement GetRequestOrgID**

File: `backend/internal/middleware/orgresolver.go`

```go
package middleware

import (
	stderrors "errors"
	"net/http"
)

// ErrNoOrgContext signals that neither an API-key principal nor a session
// with a current org was found on the request.
var ErrNoOrgContext = stderrors.New("no organization context on request")

// GetRequestOrgID returns the effective org_id for a request, sourcing it from
// either an APIKeyPrincipal (public/API-key callers) or UserClaims.CurrentOrgID
// (session callers).
//
// Handlers should call this instead of accessing claims directly so they work
// uniformly under either auth chain.
func GetRequestOrgID(r *http.Request) (int, error) {
	if p := GetAPIKeyPrincipal(r); p != nil {
		return p.OrgID, nil
	}
	if c := GetUserClaims(r); c != nil {
		if c.CurrentOrgID != nil {
			return *c.CurrentOrgID, nil
		}
	}
	return 0, ErrNoOrgContext
}
```

- [ ] **Step 4.4: Run tests**

```bash
just backend test ./internal/middleware/... -run GetRequestOrgID
```

Expected: 4 PASS.

- [ ] **Step 4.5: Commit**

```bash
git add backend/internal/middleware/orgresolver.go backend/internal/middleware/orgresolver_test.go
git commit -m "feat(tra-396): GetRequestOrgID helper resolves org from either principal"
```

---

## Task 5: `httputil.ParseListParams` shared request parser

**Files:**
- Create: `backend/internal/util/httputil/listparams.go`
- Create: `backend/internal/util/httputil/listparams_test.go`

- [ ] **Step 5.1: Write the failing test**

File: `backend/internal/util/httputil/listparams_test.go`

```go
package httputil_test

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestParseListParams_Defaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters: []string{"location"},
		Sorts:   []string{"identifier", "name"},
	})
	require.NoError(t, err)
	assert.Equal(t, 50, p.Limit)
	assert.Equal(t, 0, p.Offset)
	assert.Empty(t, p.Filters)
	assert.Empty(t, p.Sorts)
}

func TestParseListParams_LimitCap(t *testing.T) {
	req := httptest.NewRequest("GET", "/?limit=500", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "200")
}

func TestParseListParams_UnknownParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/?mystery=1", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{Filters: []string{"location"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mystery")
}

func TestParseListParams_Filters(t *testing.T) {
	req := httptest.NewRequest("GET", "/?location=wh-1&location=wh-2&is_active=true", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters: []string{"location", "is_active"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"wh-1", "wh-2"}, p.Filters["location"])
	assert.Equal(t, []string{"true"}, p.Filters["is_active"])
}

func TestParseListParams_Sort(t *testing.T) {
	req := httptest.NewRequest("GET", "/?sort=name,-created_at", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Sorts: []string{"name", "created_at"},
	})
	require.NoError(t, err)
	require.Len(t, p.Sorts, 2)
	assert.Equal(t, "name", p.Sorts[0].Field)
	assert.False(t, p.Sorts[0].Desc)
	assert.Equal(t, "created_at", p.Sorts[1].Field)
	assert.True(t, p.Sorts[1].Desc)
}

func TestParseListParams_UnknownSortField(t *testing.T) {
	req := httptest.NewRequest("GET", "/?sort=banana", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{Sorts: []string{"name"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "banana")
}

func TestParseListParams_InvalidOffsetNegative(t *testing.T) {
	req := httptest.NewRequest("GET", "/?offset=-1", nil)
	_, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.Error(t, err)
}

func TestParseListParams_LimitAndOffsetExempt(t *testing.T) {
	// limit/offset/sort are always allowed without declaration.
	req := httptest.NewRequest("GET", "/?limit=25&offset=5", nil)
	p, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	require.NoError(t, err)
	assert.Equal(t, 25, p.Limit)
	assert.Equal(t, 5, p.Offset)
}
```

- [ ] **Step 5.2: Run test to verify it fails**

```bash
just backend test ./internal/util/httputil/... -run ParseListParams
```

Expected: compile failure.

- [ ] **Step 5.3: Implement ParseListParams**

File: `backend/internal/util/httputil/listparams.go`

```go
package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// ListAllowlist declares which filter and sort fields the endpoint accepts.
// limit, offset, and sort are always allowed.
type ListAllowlist struct {
	Filters []string
	Sorts   []string
}

// SortField represents one entry in a sort list.
type SortField struct {
	Field string
	Desc  bool
}

// ListParams is the parsed result of a list-endpoint request.
type ListParams struct {
	Limit   int
	Offset  int
	Filters map[string][]string
	Sorts   []SortField
}

// ParseListParams validates and parses pagination, filters, and sort from
// the request query string. Returns an error whose message is safe to surface
// in a 400 "detail" field.
func ParseListParams(r *http.Request, allow ListAllowlist) (ListParams, error) {
	out := ListParams{
		Limit:   defaultListLimit,
		Offset:  0,
		Filters: map[string][]string{},
	}

	q := r.URL.Query()
	filterAllow := toSet(allow.Filters)
	sortAllow := toSet(allow.Sorts)

	for key, values := range q {
		switch key {
		case "limit":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 1 {
				return out, fmt.Errorf("limit must be a positive integer")
			}
			if n > maxListLimit {
				return out, fmt.Errorf("limit must be ≤ %d", maxListLimit)
			}
			out.Limit = n
		case "offset":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 0 {
				return out, fmt.Errorf("offset must be a non-negative integer")
			}
			out.Offset = n
		case "sort":
			parsed, err := parseSort(values[0], sortAllow)
			if err != nil {
				return out, err
			}
			out.Sorts = parsed
		default:
			if _, ok := filterAllow[key]; !ok {
				return out, fmt.Errorf("unknown parameter: %s", key)
			}
			out.Filters[key] = values
		}
	}

	return out, nil
}

func parseSort(raw string, allow map[string]struct{}) ([]SortField, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]SortField, 0, len(parts))
	for _, p := range parts {
		desc := false
		field := strings.TrimSpace(p)
		if strings.HasPrefix(field, "-") {
			desc = true
			field = field[1:]
		}
		if _, ok := allow[field]; !ok {
			return nil, fmt.Errorf("unknown sort field: %s", field)
		}
		out = append(out, SortField{Field: field, Desc: desc})
	}
	return out, nil
}

func toSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}
```

- [ ] **Step 5.4: Run tests**

```bash
just backend test ./internal/util/httputil/... -run ParseListParams
```

Expected: 8 PASS.

- [ ] **Step 5.5: Commit**

```bash
git add backend/internal/util/httputil/listparams.go backend/internal/util/httputil/listparams_test.go
git commit -m "feat(tra-396): ParseListParams shared list request parser"
```

---

## Task 6: Storage `GetAssetByIdentifier` with natural-key join

**Files:**
- Modify: `backend/internal/storage/assets.go` (append new method)
- Modify: `backend/internal/storage/assets_test.go` (append test)

- [ ] **Step 6.1: Write the failing integration test**

Append to `backend/internal/storage/assets_test.go`:

```go
func TestGetAssetByIdentifier_Found(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "widget-42", Name: "Widget", Type: "asset",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	view, err := store.GetAssetByIdentifier(context.Background(), orgID, "widget-42")
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.Equal(t, "widget-42", view.Identifier)
	assert.Equal(t, "wh-1", view.CurrentLocationIdentifier, "must resolve natural key via join")
}

func TestGetAssetByIdentifier_WrongOrg(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)
	orgB := testutil.CreateTestAccount(t, pool)

	_, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgA, Identifier: "a-only", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Look up as orgB — must not find orgA's asset.
	view, err := store.GetAssetByIdentifier(context.Background(), orgB, "a-only")
	require.NoError(t, err)
	assert.Nil(t, view)
}

func TestGetAssetByIdentifier_SoftDeletedNotReturned(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "gone", Name: "Gone",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.DeleteAsset(context.Background(), &created.ID)
	require.NoError(t, err)

	view, err := store.GetAssetByIdentifier(context.Background(), orgID, "gone")
	require.NoError(t, err)
	assert.Nil(t, view)
}
```

Note: `CurrentLocationIdentifier` is a new field on `asset.AssetView` — added in the next step along with the query. If `asset.AssetView` is not the right struct for this (it bundles `TagIdentifier` list), introduce a new `asset.AssetWithLocation` struct and reference it in the test.

- [ ] **Step 6.2: Add the CurrentLocationIdentifier field**

Append to `backend/internal/models/asset/asset.go` (below the existing `AssetView`):

```go
// AssetWithLocation is AssetView plus the resolved parent-location natural key.
// Populated by GetAssetByIdentifier / list-with-join storage methods; returned
// to HTTP handlers which then project it to PublicAssetView.
type AssetWithLocation struct {
	AssetView
	CurrentLocationIdentifier *string `json:"current_location_identifier,omitempty"`
}
```

- [ ] **Step 6.3: Run test to verify it fails**

```bash
just backend test-integration ./internal/storage/... -run TestGetAssetByIdentifier
```

Expected: compile failure — `GetAssetByIdentifier` undefined.

- [ ] **Step 6.4: Implement GetAssetByIdentifier**

Append to `backend/internal/storage/assets.go`:

```go
// GetAssetByIdentifier returns the live (non-deleted) asset with the given
// natural identifier for the given org, plus the parent location's identifier.
// Returns (nil, nil) if no match.
func (s *Storage) GetAssetByIdentifier(
	ctx context.Context, orgID int, identifier string,
) (*asset.AssetWithLocation, error) {
	query := `
		SELECT
			a.id, a.org_id, a.identifier, a.name, a.type, a.description,
			a.current_location_id, a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.identifier
		FROM trakrf.assets a
		LEFT JOIN trakrf.locations l ON l.id = a.current_location_id AND l.deleted_at IS NULL
		WHERE a.org_id = $1 AND a.identifier = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
	var (
		a      asset.Asset
		locIdt *string
	)
	err := s.pool.QueryRow(ctx, query, orgID, identifier).Scan(
		&a.ID, &a.OrgID, &a.Identifier, &a.Name, &a.Type, &a.Description,
		&a.CurrentLocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata,
		&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		&locIdt,
	)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset by identifier: %w", err)
	}

	identifiers, err := s.GetIdentifiersByAssetID(ctx, a.ID)
	if err != nil {
		return nil, err
	}

	return &asset.AssetWithLocation{
		AssetView: asset.AssetView{
			Asset:       a,
			Identifiers: identifiers,
		},
		CurrentLocationIdentifier: locIdt,
	}, nil
}
```

Add `stderrors "errors"` to the imports if not present.

- [ ] **Step 6.5: Run tests**

```bash
just backend test-integration ./internal/storage/... -run TestGetAssetByIdentifier
```

Expected: 3 PASS.

- [ ] **Step 6.6: Commit**

```bash
git add backend/internal/storage/assets.go backend/internal/storage/assets_test.go backend/internal/models/asset/asset.go
git commit -m "feat(tra-396): storage.GetAssetByIdentifier with current_location join"
```

---

## Task 7: Storage `GetLocationByIdentifier` with parent join

**Files:**
- Modify: `backend/internal/storage/locations.go` (append method)
- Modify: `backend/internal/models/location/location.go` (add `LocationWithParent`)
- Modify: `backend/internal/storage/locations_test.go` (append test)

- [ ] **Step 7.1: Add LocationWithParent**

Append to `backend/internal/models/location/location.go`:

```go
// LocationWithParent is LocationView plus the resolved parent's natural key.
type LocationWithParent struct {
	LocationView
	ParentIdentifier *string `json:"parent_identifier,omitempty"`
}
```

- [ ] **Step 7.2: Write the failing test**

Append to `backend/internal/storage/locations_test.go`:

```go
func TestGetLocationByIdentifier_Found(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	parent, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-1.bay-3", Name: "Bay 3",
		ParentLocationID: &parent.ID,
		Path:             "wh-1.bay-3", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	view, err := store.GetLocationByIdentifier(context.Background(), orgID, "wh-1.bay-3")
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.Equal(t, "wh-1.bay-3", view.Identifier)
	require.NotNil(t, view.ParentIdentifier)
	assert.Equal(t, "wh-1", *view.ParentIdentifier)
}

func TestGetLocationByIdentifier_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	view, err := store.GetLocationByIdentifier(context.Background(), orgID, "missing")
	require.NoError(t, err)
	assert.Nil(t, view)
}
```

- [ ] **Step 7.3: Implement GetLocationByIdentifier**

Append to `backend/internal/storage/locations.go`:

```go
// GetLocationByIdentifier returns the live location with the given natural key
// for the given org, plus the parent location's natural key. Returns (nil, nil)
// if no match.
func (s *Storage) GetLocationByIdentifier(
	ctx context.Context, orgID int, identifier string,
) (*location.LocationWithParent, error) {
	query := `
		SELECT
			l.id, l.org_id, l.identifier, l.name, l.description,
			l.parent_location_id, l.path, l.metadata, l.valid_from, l.valid_to,
			l.is_active, l.created_at, l.updated_at, l.deleted_at,
			p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p ON p.id = l.parent_location_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1 AND l.identifier = $2 AND l.deleted_at IS NULL
		LIMIT 1
	`
	var (
		loc    location.Location
		parIdt *string
	)
	err := s.pool.QueryRow(ctx, query, orgID, identifier).Scan(
		&loc.ID, &loc.OrgID, &loc.Identifier, &loc.Name, &loc.Description,
		&loc.ParentLocationID, &loc.Path, &loc.Metadata, &loc.ValidFrom, &loc.ValidTo,
		&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
		&parIdt,
	)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get location by identifier: %w", err)
	}

	identifiers, err := s.GetIdentifiersByLocationID(ctx, loc.ID)
	if err != nil {
		return nil, err
	}

	return &location.LocationWithParent{
		LocationView: location.LocationView{
			Location:    loc,
			Identifiers: identifiers,
		},
		ParentIdentifier: parIdt,
	}, nil
}
```

Add imports as needed (`stderrors "errors"`, `pgx`). If `locations.go` model field names differ from the above (e.g., `Path` or `Metadata` presence), adjust to match the actual struct — inspect `backend/internal/models/location/location.go` first.

- [ ] **Step 7.4: Run tests**

```bash
just backend test-integration ./internal/storage/... -run TestGetLocationByIdentifier
```

Expected: 2 PASS.

- [ ] **Step 7.5: Commit**

```bash
git add backend/internal/storage/locations.go backend/internal/storage/locations_test.go backend/internal/models/location/location.go
git commit -m "feat(tra-396): storage.GetLocationByIdentifier with parent join"
```

---

## Task 8: Storage `ListAssetsFiltered` with join + filters

Replaces the use of `ListAssetViews` + `ListAllAssets` on the list path. Keeps those as-is for bulk-import code that still wants the unfiltered view.

**Files:**
- Modify: `backend/internal/storage/assets.go` (append `ListAssetsFiltered`, `CountAssetsFiltered`)
- Modify: `backend/internal/models/asset/asset.go` (add `ListFilter`)
- Modify: `backend/internal/storage/assets_test.go` (append tests)

- [ ] **Step 8.1: Add ListFilter model**

Append to `backend/internal/models/asset/asset.go`:

```go
// ListFilter carries the optional filters the assets list endpoint supports.
type ListFilter struct {
	LocationIdentifiers []string // OR semantics when multi-valued
	IsActive            *bool
	Type                *string
	Q                   *string // fuzzy match on name, identifier, description
	Sorts               []ListSort
	Limit               int
	Offset              int
}

// ListSort is one (field, direction) entry.
type ListSort struct {
	Field string
	Desc  bool
}
```

- [ ] **Step 8.2: Write the failing test**

Append to `backend/internal/storage/assets_test.go`:

```go
func TestListAssetsFiltered_LocationAndSort(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	locA, _ := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-A", Name: "A", Path: "wh-A",
		ValidFrom: time.Now(), IsActive: true,
	})
	locB, _ := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-B", Name: "B", Path: "wh-B",
		ValidFrom: time.Now(), IsActive: true,
	})

	for _, spec := range []struct {
		id   string
		name string
		loc  *int
	}{
		{"aaa", "A Asset", &locA.ID},
		{"bbb", "B Asset", &locB.ID},
		{"ccc", "C Asset", &locA.ID},
	} {
		_, err := store.CreateAsset(context.Background(), asset.Asset{
			OrgID: orgID, Identifier: spec.id, Name: spec.name,
			CurrentLocationID: spec.loc, ValidFrom: time.Now(), IsActive: true,
		})
		require.NoError(t, err)
	}

	// Filter by location wh-A, sort by identifier ASC
	items, err := store.ListAssetsFiltered(context.Background(), orgID, asset.ListFilter{
		LocationIdentifiers: []string{"wh-A"},
		Sorts:               []asset.ListSort{{Field: "identifier", Desc: false}},
		Limit:               50, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "aaa", items[0].Identifier)
	assert.Equal(t, "ccc", items[1].Identifier)
	require.NotNil(t, items[0].CurrentLocationIdentifier)
	assert.Equal(t, "wh-A", *items[0].CurrentLocationIdentifier)

	count, err := store.CountAssetsFiltered(context.Background(), orgID, asset.ListFilter{
		LocationIdentifiers: []string{"wh-A"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestListAssetsFiltered_Q(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	_, _ = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "forklift-1", Name: "Forklift One",
		ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "widget-1", Name: "Widget",
		ValidFrom: time.Now(), IsActive: true,
	})

	q := "fork"
	items, err := store.ListAssetsFiltered(context.Background(), orgID, asset.ListFilter{
		Q: &q, Limit: 50,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "forklift-1", items[0].Identifier)
}
```

- [ ] **Step 8.3: Implement ListAssetsFiltered and CountAssetsFiltered**

Append to `backend/internal/storage/assets.go`. Use parameterized SQL; build WHERE dynamically based on filter fields present.

```go
// ListAssetsFiltered returns assets matching the filter, joined with their
// current location's natural key. Sort fields allowlisted by handler.
func (s *Storage) ListAssetsFiltered(
	ctx context.Context, orgID int, f asset.ListFilter,
) ([]asset.AssetWithLocation, error) {
	where, args := buildAssetsWhere(orgID, f)
	orderBy := buildAssetsOrderBy(f.Sorts)

	query := fmt.Sprintf(`
		SELECT
			a.id, a.org_id, a.identifier, a.name, a.type, a.description,
			a.current_location_id, a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.identifier
		FROM trakrf.assets a
		LEFT JOIN trakrf.locations l
			ON l.id = a.current_location_id AND l.deleted_at IS NULL
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, where, orderBy, len(args)+1, len(args)+2)

	args = append(args, clampLimit(f.Limit), f.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list assets filtered: %w", err)
	}
	defer rows.Close()

	out := []asset.AssetWithLocation{}
	for rows.Next() {
		var (
			a      asset.Asset
			locIdt *string
		)
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.Identifier, &a.Name, &a.Type, &a.Description,
			&a.CurrentLocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata,
			&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
			&locIdt,
		); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		out = append(out, asset.AssetWithLocation{
			AssetView:                 asset.AssetView{Asset: a, Identifiers: nil},
			CurrentLocationIdentifier: locIdt,
		})
	}

	// Bulk-fetch identifiers for the returned assets (existing helper).
	if len(out) > 0 {
		ids := make([]int, len(out))
		for i, a := range out {
			ids[i] = a.ID
		}
		idMap, err := s.getIdentifiersForAssets(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Identifiers = idMap[out[i].ID]
			if out[i].Identifiers == nil {
				out[i].Identifiers = []shared.TagIdentifier{}
			}
		}
	}

	return out, rows.Err()
}

// CountAssetsFiltered returns total count matching the filter (ignores limit/offset/sort).
func (s *Storage) CountAssetsFiltered(
	ctx context.Context, orgID int, f asset.ListFilter,
) (int, error) {
	where, args := buildAssetsWhere(orgID, f)
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM trakrf.assets a
		LEFT JOIN trakrf.locations l
			ON l.id = a.current_location_id AND l.deleted_at IS NULL
		WHERE %s
	`, where)

	var n int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count assets filtered: %w", err)
	}
	return n, nil
}

func buildAssetsWhere(orgID int, f asset.ListFilter) (string, []any) {
	clauses := []string{"a.org_id = $1", "a.deleted_at IS NULL"}
	args := []any{orgID}

	if len(f.LocationIdentifiers) > 0 {
		args = append(args, f.LocationIdentifiers)
		clauses = append(clauses, fmt.Sprintf("l.identifier = ANY($%d::text[])", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		clauses = append(clauses, fmt.Sprintf("a.is_active = $%d", len(args)))
	}
	if f.Type != nil {
		args = append(args, *f.Type)
		clauses = append(clauses, fmt.Sprintf("a.type = $%d", len(args)))
	}
	if f.Q != nil {
		args = append(args, "%"+*f.Q+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(a.name ILIKE $%d OR a.identifier ILIKE $%d OR a.description ILIKE $%d)", idx, idx, idx))
	}
	return strings.Join(clauses, " AND "), args
}

func buildAssetsOrderBy(sorts []asset.ListSort) string {
	if len(sorts) == 0 {
		return "a.identifier ASC"
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		// Field allowlist is enforced at the handler layer; map to SQL col:
		col := "a." + s.Field
		out = append(out, col+" "+dir)
	}
	return strings.Join(out, ", ")
}

func clampLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}
```

Ensure `strings` and `shared` are imported; both are in-package already based on existing `assets.go`.

- [ ] **Step 8.4: Run tests**

```bash
just backend test-integration ./internal/storage/... -run TestListAssetsFiltered
```

Expected: 2 PASS.

- [ ] **Step 8.5: Commit**

```bash
git add backend/internal/storage/assets.go backend/internal/storage/assets_test.go backend/internal/models/asset/asset.go
git commit -m "feat(tra-396): ListAssetsFiltered with join, filters, sort"
```

---

## Task 9: Storage `ListLocationsFiltered` with parent join + filters

Same pattern as Task 8 for locations.

**Files:**
- Modify: `backend/internal/storage/locations.go`
- Modify: `backend/internal/models/location/location.go`
- Modify: `backend/internal/storage/locations_test.go`

- [ ] **Step 9.1: Add ListFilter**

Append to `backend/internal/models/location/location.go`:

```go
// ListFilter carries the optional filters the locations list endpoint supports.
type ListFilter struct {
	ParentIdentifiers []string
	IsActive          *bool
	Q                 *string
	Sorts             []ListSort
	Limit             int
	Offset            int
}

type ListSort struct {
	Field string
	Desc  bool
}
```

- [ ] **Step 9.2: Write the failing test**

Append to `backend/internal/storage/locations_test.go`:

```go
func TestListLocationsFiltered_Parent(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	root, _ := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "root", Name: "R", Path: "root",
		ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "root.a", Name: "A", ParentLocationID: &root.ID,
		Path: "root.a", ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "root.b", Name: "B", ParentLocationID: &root.ID,
		Path: "root.b", ValidFrom: time.Now(), IsActive: true,
	})

	items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
		ParentIdentifiers: []string{"root"},
		Sorts:             []location.ListSort{{Field: "identifier"}},
		Limit:             50,
	})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "root.a", items[0].Identifier)
	assert.Equal(t, "root.b", items[1].Identifier)
	require.NotNil(t, items[0].ParentIdentifier)
	assert.Equal(t, "root", *items[0].ParentIdentifier)
}
```

- [ ] **Step 9.3: Implement ListLocationsFiltered and CountLocationsFiltered**

Append to `backend/internal/storage/locations.go`:

```go
// ListLocationsFiltered returns locations matching the filter with parent's
// natural key resolved via self-join.
func (s *Storage) ListLocationsFiltered(
	ctx context.Context, orgID int, f location.ListFilter,
) ([]location.LocationWithParent, error) {
	where, args := buildLocationsWhere(orgID, f)
	orderBy := buildLocationsOrderBy(f.Sorts)

	query := fmt.Sprintf(`
		SELECT
			l.id, l.org_id, l.identifier, l.name, l.description,
			l.parent_location_id, l.path, l.metadata, l.valid_from, l.valid_to,
			l.is_active, l.created_at, l.updated_at, l.deleted_at,
			p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.deleted_at IS NULL
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, where, orderBy, len(args)+1, len(args)+2)

	args = append(args, clampLimit(f.Limit), f.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list locations filtered: %w", err)
	}
	defer rows.Close()

	out := []location.LocationWithParent{}
	for rows.Next() {
		var (
			loc    location.Location
			parIdt *string
		)
		if err := rows.Scan(
			&loc.ID, &loc.OrgID, &loc.Identifier, &loc.Name, &loc.Description,
			&loc.ParentLocationID, &loc.Path, &loc.Metadata, &loc.ValidFrom, &loc.ValidTo,
			&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
			&parIdt,
		); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		out = append(out, location.LocationWithParent{
			LocationView:     location.LocationView{Location: loc},
			ParentIdentifier: parIdt,
		})
	}
	return out, rows.Err()
}

// CountLocationsFiltered returns total count matching the filter.
func (s *Storage) CountLocationsFiltered(
	ctx context.Context, orgID int, f location.ListFilter,
) (int, error) {
	where, args := buildLocationsWhere(orgID, f)
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.deleted_at IS NULL
		WHERE %s
	`, where)

	var n int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count locations filtered: %w", err)
	}
	return n, nil
}

func buildLocationsWhere(orgID int, f location.ListFilter) (string, []any) {
	clauses := []string{"l.org_id = $1", "l.deleted_at IS NULL"}
	args := []any{orgID}

	if len(f.ParentIdentifiers) > 0 {
		args = append(args, f.ParentIdentifiers)
		clauses = append(clauses, fmt.Sprintf("p.identifier = ANY($%d::text[])", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		clauses = append(clauses, fmt.Sprintf("l.is_active = $%d", len(args)))
	}
	if f.Q != nil {
		args = append(args, "%"+*f.Q+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(l.name ILIKE $%d OR l.identifier ILIKE $%d OR l.description ILIKE $%d)", idx, idx, idx))
	}
	return strings.Join(clauses, " AND "), args
}

func buildLocationsOrderBy(sorts []location.ListSort) string {
	if len(sorts) == 0 {
		return "l.path ASC"
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		col := "l." + s.Field
		out = append(out, col+" "+dir)
	}
	return strings.Join(out, ", ")
}
```

- [ ] **Step 9.4: Run tests**

```bash
just backend test-integration ./internal/storage/... -run TestListLocationsFiltered
```

Expected: 1 PASS.

- [ ] **Step 9.5: Commit**

```bash
git add backend/internal/storage/locations.go backend/internal/storage/locations_test.go backend/internal/models/location/location.go
git commit -m "feat(tra-396): ListLocationsFiltered with parent join, filters, sort"
```

---

## Task 10: Update reports storage for natural-key filters

Change `ListCurrentLocations` and `ListAssetHistory` to accept new filter shape and return joined natural keys.

**Files:**
- Modify: `backend/internal/storage/reports.go`
- Modify: `backend/internal/models/report/report.go`
- Modify: `backend/internal/storage/reports_test.go` (if present) or add new tests

- [ ] **Step 10.1: Update model**

Edit `backend/internal/models/report/report.go`:

1. On `CurrentLocationFilter`, rename `LocationID *int` → `LocationIdentifiers []string`, rename `Search *string` → `Q *string`.
2. On `CurrentLocationItem`, rename `AssetID` → keep but add `AssetIdentifier string` and `LocationIdentifier string` (if not already present). Adjust `CurrentLocationsResponse` to use `Limit` instead of `Count` if that field exists.
3. On `AssetHistoryFilter`, rename `StartDate` → `From *time.Time`, `EndDate` → `To *time.Time`.
4. On `AssetHistoryItem`, add `LocationIdentifier string` if not present.

Example (edit around `backend/internal/models/report/report.go`):

```go
type CurrentLocationFilter struct {
	LocationIdentifiers []string
	Q                   *string
	Limit               int
	Offset              int
}

type AssetHistoryFilter struct {
	From   *time.Time
	To     *time.Time
	Limit  int
	Offset int
}
```

- [ ] **Step 10.2: Update storage ListCurrentLocations signature**

Replace the existing `ListCurrentLocations` query builders in `backend/internal/storage/reports.go` to accept natural-key filter and join:

```go
func (s *Storage) ListCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) ([]report.CurrentLocationItem, error) {
	// Existing TimescaleDB / DistinctOn dispatch stays; both variants must:
	// - JOIN trakrf.locations ON l.id = scan's location_id (return l.identifier)
	// - JOIN trakrf.assets ON a.id = scan's asset_id (return a.identifier)
	// - WHERE l.identifier = ANY($N::text[]) when filter.LocationIdentifiers non-empty
	// - WHERE (a.name ILIKE $N OR a.identifier ILIKE $N) when filter.Q non-nil
	//
	// The query builders are in buildCurrentLocationsQueryDistinctOn() and
	// buildCurrentLocationsQueryTimescale(). Update both.
	...
}
```

Concrete rewrite for `buildCurrentLocationsQueryDistinctOn`:

```go
func buildCurrentLocationsQueryDistinctOn() string {
	return `
		SELECT DISTINCT ON (s.asset_id)
			s.asset_id,
			a.name         AS asset_name,
			a.identifier   AS asset_identifier,
			s.location_id,
			l.name         AS location_name,
			l.identifier   AS location_identifier,
			s.timestamp    AS last_seen
		FROM trakrf.asset_scans s
		JOIN trakrf.assets    a ON a.id = s.asset_id    AND a.deleted_at IS NULL
		JOIN trakrf.locations l ON l.id = s.location_id AND l.deleted_at IS NULL
		WHERE s.org_id = $1
		  AND ($2::text[] IS NULL OR l.identifier = ANY($2::text[]))
		  AND ($3::text IS NULL OR a.name ILIKE $3 OR a.identifier ILIKE $3)
		ORDER BY s.asset_id, s.timestamp DESC
		LIMIT $4 OFFSET $5
	`
}
```

Query invocation and row scan updates in `ListCurrentLocations`:

```go
var locFilterArg any
if len(filter.LocationIdentifiers) > 0 {
	locFilterArg = filter.LocationIdentifiers
}
var qArg any
if filter.Q != nil {
	q := "%" + *filter.Q + "%"
	qArg = q
}

rows, err := s.pool.Query(ctx, query, orgID, locFilterArg, qArg, filter.Limit, filter.Offset)
...
rows.Scan(
	&item.AssetID, &item.AssetName, &item.AssetIdentifier,
	&item.LocationID, &item.LocationName, &item.LocationIdentifier,
	&item.LastSeen,
)
```

Apply equivalent changes to `buildCurrentLocationsQueryTimescale`.

Equivalent change to `CountCurrentLocations`:

```go
func (s *Storage) CountCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) (int, error) {
	query := `
		SELECT COUNT(DISTINCT s.asset_id)
		FROM trakrf.asset_scans s
		JOIN trakrf.assets    a ON a.id = s.asset_id    AND a.deleted_at IS NULL
		JOIN trakrf.locations l ON l.id = s.location_id AND l.deleted_at IS NULL
		WHERE s.org_id = $1
		  AND ($2::text[] IS NULL OR l.identifier = ANY($2::text[]))
		  AND ($3::text IS NULL OR a.name ILIKE $3 OR a.identifier ILIKE $3)
	`
	... (args mirror the list builder)
}
```

- [ ] **Step 10.3: Update ListAssetHistory / CountAssetHistory**

Open `backend/internal/storage/reports.go` (search for `ListAssetHistory`). Swap `StartDate` / `EndDate` usage to `From` / `To` and ensure the result items populate `LocationIdentifier`. If the existing SQL doesn't join locations, add `JOIN trakrf.locations l ON l.id = s.location_id`.

- [ ] **Step 10.4: Run tests**

```bash
just backend test-integration ./internal/storage/... -run "TestListCurrentLocations|TestListAssetHistory|TestCountCurrentLocations|TestCountAssetHistory"
```

Expected: all existing report tests PASS after updating them to use the renamed fields. Expect some test edits.

- [ ] **Step 10.5: Commit**

```bash
git add backend/internal/storage/reports.go backend/internal/models/report/report.go backend/internal/storage/reports_test.go
git commit -m "refactor(tra-396): reports storage takes natural-key filters, joins for names"
```

---

## Task 11: Public response structs

**Files:**
- Create: `backend/internal/models/asset/public.go`
- Create: `backend/internal/models/location/public.go`
- Create: `backend/internal/models/report/public.go`

- [ ] **Step 11.1: Asset public view**

File: `backend/internal/models/asset/public.go`

```go
package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// PublicAssetView is the HTTP shape emitted by read endpoints. It drops
// org_id and deleted_at, renames the surrogate id, and carries the parent
// location's natural key instead of the INT FK.
type PublicAssetView struct {
	Identifier        string                  `json:"identifier"`
	Name              string                  `json:"name"`
	Type              *string                 `json:"type,omitempty"`
	Description       *string                 `json:"description,omitempty"`
	CurrentLocation   *string                 `json:"current_location,omitempty"`
	Metadata          map[string]any          `json:"metadata,omitempty"`
	IsActive          bool                    `json:"is_active"`
	ValidFrom         time.Time               `json:"valid_from"`
	ValidTo           *time.Time              `json:"valid_to,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
	SurrogateID       int                     `json:"surrogate_id"`
	Identifiers       []shared.TagIdentifier  `json:"identifiers"`
}

// ToPublicAssetView projects an AssetWithLocation to the public HTTP shape.
func ToPublicAssetView(a AssetWithLocation) PublicAssetView {
	return PublicAssetView{
		Identifier:      a.Identifier,
		Name:            a.Name,
		Type:            a.Type,
		Description:     a.Description,
		CurrentLocation: a.CurrentLocationIdentifier,
		Metadata:        a.Metadata,
		IsActive:        a.IsActive,
		ValidFrom:       a.ValidFrom,
		ValidTo:         a.ValidTo,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
		SurrogateID:     a.ID,
		Identifiers:     a.Identifiers,
	}
}
```

- [ ] **Step 11.2: Location public view**

File: `backend/internal/models/location/public.go`

```go
package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

type PublicLocationView struct {
	Identifier  string                 `json:"identifier"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Parent      *string                `json:"parent,omitempty"`
	Path        string                 `json:"path"`
	Metadata    map[string]any         `json:"metadata,omitempty"`
	IsActive    bool                   `json:"is_active"`
	ValidFrom   time.Time              `json:"valid_from"`
	ValidTo     *time.Time             `json:"valid_to,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	SurrogateID int                    `json:"surrogate_id"`
	Identifiers []shared.TagIdentifier `json:"identifiers"`
}

func ToPublicLocationView(l LocationWithParent) PublicLocationView {
	return PublicLocationView{
		Identifier:  l.Identifier,
		Name:        l.Name,
		Description: l.Description,
		Parent:      l.ParentIdentifier,
		Path:        l.Path,
		Metadata:    l.Metadata,
		IsActive:    l.IsActive,
		ValidFrom:   l.ValidFrom,
		ValidTo:     l.ValidTo,
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
		SurrogateID: l.ID,
		Identifiers: l.Identifiers,
	}
}
```

- [ ] **Step 11.3: Report public shapes**

File: `backend/internal/models/report/public.go`

```go
package report

import "time"

// PublicCurrentLocationItem is the public shape for /api/v1/locations/current items.
type PublicCurrentLocationItem struct {
	Asset    string    `json:"asset"`
	Location string    `json:"location"`
	LastSeen time.Time `json:"last_seen"`
}

func ToPublicCurrentLocationItem(it CurrentLocationItem) PublicCurrentLocationItem {
	return PublicCurrentLocationItem{
		Asset:    it.AssetIdentifier,
		Location: it.LocationIdentifier,
		LastSeen: it.LastSeen,
	}
}

// PublicAssetHistoryItem is the public shape for asset-history list items.
type PublicAssetHistoryItem struct {
	Timestamp       time.Time `json:"timestamp"`
	Location        string    `json:"location"`
	DurationSeconds *int64    `json:"duration_seconds,omitempty"`
}

func ToPublicAssetHistoryItem(it AssetHistoryItem) PublicAssetHistoryItem {
	return PublicAssetHistoryItem{
		Timestamp:       it.Timestamp,
		Location:        it.LocationIdentifier,
		DurationSeconds: it.DurationSeconds, // may be *int64 in source; adapt to actual type
	}
}

// PublicListEnvelope is the response shape used by all read list endpoints.
type PublicListEnvelope[T any] struct {
	Data       []T `json:"data"`
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
	TotalCount int `json:"total_count"`
}
```

If the Go version in `go.mod` doesn't support generics (1.18+), drop `PublicListEnvelope` and have each handler build the envelope inline.

- [ ] **Step 11.4: Compile check**

```bash
just backend build
```

Expected: builds cleanly.

- [ ] **Step 11.5: Commit**

```bash
git add backend/internal/models/asset/public.go backend/internal/models/location/public.go backend/internal/models/report/public.go
git commit -m "feat(tra-396): public response structs for asset, location, reports"
```

---

## Task 12: Refactor assets handlers

Replace `ListAssets` and add `GetAssetByIdentifier` / `GetAssetByID` (internal-only); all emit `PublicAssetView`. Keep existing `Create` / `UpdateAsset` / `DeleteAsset` / identifier handlers unchanged — TRA-397 revisits them.

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go`

- [ ] **Step 12.1: Replace ListAssets**

Edit `backend/internal/handlers/assets/assets.go`. Replace the existing `ListAssets` function body with:

```go
// @Summary List assets
// @Description Paginated assets list with natural-key filters, sort, and fuzzy search
// @Tags assets
// @Accept json
// @Produce json
// @Param limit    query int    false "max 200"   default(50)
// @Param offset   query int    false "min 0"    default(0)
// @Param location query string false "filter by location natural key (may repeat)"
// @Param is_active query bool  false
// @Param type     query string false
// @Param q        query string false "fuzzy search"
// @Param sort     query string false "comma-separated; prefix '-' for DESC"
// @Success 200 {object} report.PublicListEnvelope[asset.PublicAssetView]
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/assets [get]
func (handler *Handler) ListAssets(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetListFailed, "missing organization context", reqID)
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters: []string{"location", "is_active", "type", "q"},
		Sorts:   []string{"identifier", "name", "created_at", "updated_at"},
	})
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid list parameters", err.Error(), reqID)
		return
	}

	f := asset.ListFilter{
		LocationIdentifiers: params.Filters["location"],
		Limit:               params.Limit,
		Offset:              params.Offset,
	}
	if vs, ok := params.Filters["is_active"]; ok && len(vs) > 0 {
		b := vs[0] == "true"
		f.IsActive = &b
	}
	if vs, ok := params.Filters["type"]; ok && len(vs) > 0 {
		f.Type = &vs[0]
	}
	if vs, ok := params.Filters["q"]; ok && len(vs) > 0 {
		f.Q = &vs[0]
	}
	for _, s := range params.Sorts {
		f.Sorts = append(f.Sorts, asset.ListSort{Field: s.Field, Desc: s.Desc})
	}

	items, err := handler.storage.ListAssetsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetListFailed, err.Error(), reqID)
		return
	}

	total, err := handler.storage.CountAssetsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetCountFailed, err.Error(), reqID)
		return
	}

	out := make([]asset.PublicAssetView, 0, len(items))
	for _, a := range items {
		out = append(out, asset.ToPublicAssetView(a))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}
```

- [ ] **Step 12.2: Add GetAssetByIdentifier handler**

```go
// GetAssetByIdentifier serves /api/v1/assets/{identifier}.
func (handler *Handler) GetAssetByIdentifier(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetGetFailed, "missing organization context", reqID)
		return
	}

	identifier := chi.URLParam(req, "identifier")
	if identifier == "" {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Missing identifier", "", reqID)
		return
	}

	a, err := handler.storage.GetAssetByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), reqID)
		return
	}
	if a == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": asset.ToPublicAssetView(*a),
	})
}
```

- [ ] **Step 12.3: Rewrite GetAsset → GetAssetByID (internal only, surrogate path)**

Replace the existing `GetAsset` function:

```go
// GetAssetByID serves /api/v1/assets/by-id/{id} for session-auth FE callers.
func (handler *Handler) GetAssetByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetGetFailed, "missing organization context", reqID)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetGetInvalidID, idParam), err.Error(), reqID)
		return
	}

	view, err := handler.storage.GetAssetViewByID(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), reqID)
		return
	}
	if view == nil || view.OrgID != orgID {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", reqID)
		return
	}

	// For FE symmetry, wrap in an AssetWithLocation with a nil location identifier
	// (caller either already has it from a list row or can fetch it separately).
	public := asset.ToPublicAssetView(asset.AssetWithLocation{
		AssetView: *view,
	})

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": public})
}
```

- [ ] **Step 12.4: Remove old RegisterRoutes entry for GetAsset and wire new methods**

Replace the `RegisterRoutes` function with stripped-down form; routing is now registered in `router.go`:

```go
// RegisterRoutes keeps write + identifier sub-routes that stay on session auth
// with surrogate path params (TRA-397 will revisit). Read routes (list, detail,
// by-id) are registered directly in internal/cmd/serve/router.go to split them
// across the API-key and session-only groups.
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/assets", handler.Create)
	r.Put("/api/v1/assets/{id}", handler.UpdateAsset)
	r.Delete("/api/v1/assets/{id}", handler.DeleteAsset)
	r.Post("/api/v1/assets/{id}/identifiers", handler.AddIdentifier)
	r.Delete("/api/v1/assets/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	r.Post("/api/v1/assets/bulk", handler.UploadCSV)
	r.Get("/api/v1/assets/bulk/{jobId}", handler.GetJobStatus)
}
```

- [ ] **Step 12.5: Build + run existing unit tests**

```bash
just backend build && just backend test ./internal/handlers/assets/...
```

Expected: compiles; existing non-integration tests either pass or fail because shape changed (they assert on `.count` / `.id`). Fix those inline — replace `response.count` assertions with `response.limit`, `response.id` with `response.surrogate_id`. Keep it minimal.

- [ ] **Step 12.6: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/assets/*_test.go
git commit -m "refactor(tra-396): assets handlers emit PublicAssetView; add GetAssetByIdentifier + GetAssetByID"
```

---

## Task 13: Refactor locations handlers

Same pattern as Task 12. Add `ListLocations`, `GetLocationByIdentifier`, `GetLocationByID`; emit `PublicLocationView`.

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go`

- [ ] **Step 13.1: Replace list handler**

Replace `ListLocations` (or the current equivalent) with a version mirroring Task 12.1 but using `location.ListFilter`, allowlist `{"parent", "is_active", "q"}`, sorts `{"path", "identifier", "name", "created_at"}`, and emitting `location.PublicLocationView`. Follow the structure of the assets handler exactly.

- [ ] **Step 13.2: Add GetLocationByIdentifier handler**

Mirror Task 12.2 — `chi.URLParam(req, "identifier")`, call `storage.GetLocationByIdentifier`, emit `location.ToPublicLocationView`.

- [ ] **Step 13.3: Rewrite existing GetLocation → GetLocationByID**

Mirror Task 12.3 — keep `{id}` as the path param, emit `PublicLocationView` with nil parent identifier (caller fetches separately if needed).

- [ ] **Step 13.4: Strip GET routes from RegisterRoutes**

Leave POST/PUT/DELETE and any identifier sub-routes registered in the handler's `RegisterRoutes`; remove the list and detail-by-`{id}` GET registrations.

- [ ] **Step 13.5: Build + fix existing tests**

```bash
just backend build && just backend test ./internal/handlers/locations/...
```

Expected: fix test assertions for the shape change the same way as Task 12.5.

- [ ] **Step 13.6: Commit**

```bash
git add backend/internal/handlers/locations/
git commit -m "refactor(tra-396): locations handlers emit PublicLocationView; add by-identifier + by-id"
```

---

## Task 14: Refactor reports handlers

**Files:**
- Modify: `backend/internal/handlers/reports/current_locations.go`
- Modify: `backend/internal/handlers/reports/asset_history.go`

- [ ] **Step 14.1: Replace `ListCurrentLocations` handler**

Rewrite around the new filter model and the public response shape. Use `ParseListParams` with allowlist `{"location", "q"}`, no sorts exposed publicly (server sorts by last-seen DESC).

```go
func (h *Handler) ListCurrentLocations(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportCurrentLocationsFailed, "missing organization context", reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters: []string{"location", "q"},
		Sorts:   []string{"last_seen", "asset", "location"},
	})
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid list parameters", err.Error(), reqID)
		return
	}

	filter := report.CurrentLocationFilter{
		LocationIdentifiers: params.Filters["location"],
		Limit:               params.Limit,
		Offset:              params.Offset,
	}
	if vs, ok := params.Filters["q"]; ok && len(vs) > 0 {
		filter.Q = &vs[0]
	}

	items, err := h.storage.ListCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportCurrentLocationsFailed, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportCurrentLocationsCount, err.Error(), reqID)
		return
	}

	out := make([]report.PublicCurrentLocationItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicCurrentLocationItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}
```

- [ ] **Step 14.2: Replace `GetAssetHistory` with identifier-based**

Rewrite `GetAssetHistory` to accept `{identifier}` path param, resolve to surrogate via `GetAssetByIdentifier`, then query history. Support `from` / `to` query params (RFC 3339). Emit `PublicAssetHistoryItem` list + envelope.

```go
func (h *Handler) GetAssetHistory(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportAssetHistoryFailed, "missing organization context", reqID)
		return
	}

	identifier := chi.URLParam(r, "identifier")
	assetRow, err := h.storage.GetAssetByIdentifier(r.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), reqID)
		return
	}
	if assetRow == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.ReportAssetNotFound, "asset not found", reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters: []string{"from", "to"},
		Sorts:   []string{"timestamp"},
	})
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid list parameters", err.Error(), reqID)
		return
	}

	filter := report.AssetHistoryFilter{Limit: params.Limit, Offset: params.Offset}
	if vs, ok := params.Filters["from"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339, vs[0])
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Invalid 'from' timestamp; RFC3339 required", err.Error(), reqID)
			return
		}
		filter.From = &t
	}
	if vs, ok := params.Filters["to"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339, vs[0])
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Invalid 'to' timestamp; RFC3339 required", err.Error(), reqID)
			return
		}
		filter.To = &t
	}

	items, err := h.storage.ListAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryCount, err.Error(), reqID)
		return
	}

	out := make([]report.PublicAssetHistoryItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicAssetHistoryItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}
```

- [ ] **Step 14.3: Add GetAssetHistoryByID (internal `/by-id/{id}/history`)**

```go
// GetAssetHistoryByID is the session-auth surrogate variant of GetAssetHistory.
func (h *Handler) GetAssetHistoryByID(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportAssetHistoryFailed, "missing organization context", reqID)
		return
	}

	idParam := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.ReportInvalidAssetID, idParam), err.Error(), reqID)
		return
	}

	assetRow, err := h.storage.GetAssetByID(r.Context(), &id)
	if err != nil || assetRow == nil || assetRow.OrgID != orgID {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.ReportAssetNotFound, "asset not found or not accessible", reqID)
		return
	}

	// Reuse the same parse + storage-call shape as GetAssetHistory.
	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters: []string{"from", "to"},
		Sorts:   []string{"timestamp"},
	})
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid list parameters", err.Error(), reqID)
		return
	}

	filter := report.AssetHistoryFilter{Limit: params.Limit, Offset: params.Offset}
	if vs, ok := params.Filters["from"]; ok && len(vs) > 0 {
		t, _ := time.Parse(time.RFC3339, vs[0])
		filter.From = &t
	}
	if vs, ok := params.Filters["to"]; ok && len(vs) > 0 {
		t, _ := time.Parse(time.RFC3339, vs[0])
		filter.To = &t
	}

	items, err := h.storage.ListAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryCount, err.Error(), reqID)
		return
	}

	out := make([]report.PublicAssetHistoryItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicAssetHistoryItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}
```

- [ ] **Step 14.4: Remove old RegisterRoutes in reports handler**

```go
// RegisterRoutes is intentionally empty — reports routes are registered in
// internal/cmd/serve/router.go across the public and session-only groups.
func (h *Handler) RegisterRoutes(r chi.Router) {}
```

- [ ] **Step 14.5: Build + fix existing tests**

```bash
just backend build && just backend test ./internal/handlers/reports/...
```

Expected: fix test assertions (`search`→`q`, `location_id`→`location`, `start_date`→`from`, `end_date`→`to`, `count`→`limit`).

- [ ] **Step 14.6: Commit**

```bash
git add backend/internal/handlers/reports/
git commit -m "refactor(tra-396): reports handlers emit public shapes; by-identifier + by-id variants"
```

---

## Task 15: Router surgery

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`

- [ ] **Step 15.1: Replace the route registrations**

Replace the body of `setupRouter` (keeping signature, middleware stack, and non-API handler mounts unchanged) to register public routes under `EitherAuth + RequireScope` and internal `/by-id` routes under session `Auth`. Remove the default-group registrations of assets / locations / reports.

Relevant diff (show the new sections, not the whole file):

```go
	// Existing session-auth group keeps write + identifier + org + user routes,
	// but drops the read routes that moved to EitherAuth.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.SentryContext)

		orgsHandler.RegisterRoutes(r, store)
		orgsHandler.RegisterMeRoutes(r)
		usersHandler.RegisterRoutes(r)
		assetsHandler.RegisterRoutes(r)    // now write + identifier only
		locationsHandler.RegisterRoutes(r) // now write only
		inventoryHandler.RegisterRoutes(r)
		lookupHandler.RegisterRoutes(r)
		// reports routes moved to the public group below + internal by-id group
	})

	// TRA-393 canary (unchanged)
	r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", orgsHandler.GetOrgMe)

	// TRA-396 public read surface — API-key OR session auth via EitherAuth
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.SentryContext)

		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", assetsHandler.ListAssets)
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}", assetsHandler.GetAssetByIdentifier)
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}/history", reportsHandler.GetAssetHistory)

		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations", locationsHandler.ListLocations)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}", locationsHandler.GetLocationByIdentifier)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/current", reportsHandler.ListCurrentLocations)
	})

	// TRA-396 internal-only surrogate paths — session auth only
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.SentryContext)

		r.Get("/api/v1/assets/by-id/{id}", assetsHandler.GetAssetByID)
		r.Get("/api/v1/assets/by-id/{id}/history", reportsHandler.GetAssetHistoryByID)
		r.Get("/api/v1/locations/by-id/{id}", locationsHandler.GetLocationByID)
	})
```

- [ ] **Step 15.2: Build**

```bash
just backend build
```

Expected: clean build. Fix any remaining references to removed routes (e.g., swagger `@Router` annotations that still claim the old paths — update or remove them).

- [ ] **Step 15.3: Run full backend test suite**

```bash
just backend test
just backend test-integration ./...
```

Expected: all PASS. Fix any remaining test fallout (handler unit tests still using old path params, etc.).

- [ ] **Step 15.4: Commit**

```bash
git add backend/internal/cmd/serve/router.go
git commit -m "feat(tra-396): router surgery — public API-key routes + internal by-id routes"
```

---

## Task 16: Backend integration tests for public flow

End-to-end coverage through real router + real DB + real JWTs.

**Files:**
- Create: `backend/internal/handlers/assets/public_integration_test.go`
- Create: `backend/internal/handlers/locations/public_integration_test.go`
- Create: `backend/internal/handlers/reports/public_integration_test.go`

- [ ] **Step 16.1: Asset public integration test**

File: `backend/internal/handlers/assets/public_integration_test.go`

```go
//go:build integration

package assets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/assets"
	"github.com/trakrf/platform/backend/internal/middleware"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestListAssets_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "la-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('la', 'la@example.com', 'stub') RETURNING id`,
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "la-key",
		[]string{"assets:read"}, userID, nil)
	require.NoError(t, err)
	token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
	require.NoError(t, err)

	loc, _ := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	_, err = store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "widget-42", Name: "Widget",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	handler := assets.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", handler.ListAssets)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(50), body["limit"])
	assert.EqualValues(t, 1, body["total_count"])
	data := body["data"].([]any)
	require.Len(t, data, 1)
	row := data[0].(map[string]any)
	assert.Equal(t, "widget-42", row["identifier"])
	assert.Equal(t, "wh-1", row["current_location"])
	assert.NotContains(t, row, "org_id")
	assert.Contains(t, row, "surrogate_id")
	assert.Contains(t, row, "valid_from")
}

func TestListAssets_WrongScope(t *testing.T) {
	t.Setenv("JWT_SECRET", "la-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('ws', 'ws@example.com', 'stub') RETURNING id`,
	).Scan(&userID))

	// Key has only locations:read.
	key, err := store.CreateAPIKey(context.Background(), orgID, "wrong-scope",
		[]string{"locations:read"}, userID, nil)
	require.NoError(t, err)
	token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"locations:read"}, nil)
	require.NoError(t, err)

	handler := assets.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", handler.ListAssets)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListAssets_UnknownParam(t *testing.T) {
	t.Setenv("JWT_SECRET", "la-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('u', 'u@example.com', 'stub') RETURNING id`,
	).Scan(&userID))

	key, _ := store.CreateAPIKey(context.Background(), orgID, "k", []string{"assets:read"}, userID, nil)
	token, _ := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)

	handler := assets.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", handler.ListAssets)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?mystery=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unknown parameter")
}

func TestGetAssetByIdentifier_CrossOrgReturns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "la-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)
	orgB := testutil.CreateTestAccount(t, pool)

	var userB int
	require.NoError(t, pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('b', 'b@example.com', 'stub') RETURNING id`,
	).Scan(&userB))

	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgA, Identifier: "a-asset", Name: "A",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	key, _ := store.CreateAPIKey(context.Background(), orgB, "b-key", []string{"assets:read"}, userB, nil)
	token, _ := jwt.GenerateAPIKey(key.JTI, orgB, []string{"assets:read"}, nil)

	handler := assets.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}", handler.GetAssetByIdentifier)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/a-asset", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

- [ ] **Step 16.2: Locations public integration test**

File: `backend/internal/handlers/locations/public_integration_test.go`

Mirror Step 16.1: happy path (with `parent` natural key present in the response), wrong-scope (use a key with `assets:read` scope hitting a `locations:read` route → 403), unknown-param 400, cross-org 404. Use `locations:read` scope and path `/api/v1/locations/{identifier}`.

- [ ] **Step 16.3: Reports public integration test**

File: `backend/internal/handlers/reports/public_integration_test.go`

Cover: `GET /api/v1/locations/current` with `location=` filter, scope enforcement (`locations:read`), response shape has `asset` and `location` as natural keys. Also cover `GET /api/v1/assets/{identifier}/history` with `from=`/`to=` date filter. You will need to insert asset_scans rows to exercise the report queries — use `testutil.CreateTestScan` if present; otherwise inline an INSERT INTO trakrf.asset_scans.

- [ ] **Step 16.4: Run**

```bash
just backend test-integration ./internal/handlers/...
```

Expected: all new tests PASS.

- [ ] **Step 16.5: Commit**

```bash
git add backend/internal/handlers/assets/public_integration_test.go backend/internal/handlers/locations/public_integration_test.go backend/internal/handlers/reports/public_integration_test.go
git commit -m "test(tra-396): public endpoint integration tests (scope, shape, cross-org)"
```

---

## Task 17: Frontend API client updates

**Files:**
- Modify: frontend apiClient call sites for assets, locations, reports

- [ ] **Step 17.1: Locate existing call sites**

```bash
grep -rn "api/v1/assets\|api/v1/locations\|api/v1/reports" frontend/src
```

Expected output: list of lines referencing old paths. Record each file + line for the edits below.

- [ ] **Step 17.2: Rewrite URL constructors**

For each call site found:

- `apiClient.get('/assets/${id}')` or similar → `apiClient.get(`/assets/by-id/${id}`)`
- `apiClient.get('/locations/${id}')` → `apiClient.get(`/locations/by-id/${id}`)`
- `apiClient.get('/reports/current-locations')` → `apiClient.get('/locations/current')`
- `apiClient.get('/reports/assets/${id}/history')` → `apiClient.get(`/assets/by-id/${id}/history`)`
- Query-param renames on the current-locations call: `location_id=${locationId}` → `location=${locationIdentifier}` (frontend switches to the natural-key value); `search=${q}` → `q=${q}`.
- Query-param renames on asset-history: `start_date=...` → `from=...`; `end_date=...` → `to=...`.

Note on the `location` filter: frontend needs the Location's `identifier` string (not surrogate) to filter current-locations. The list rows already expose it — pass through from the selected row.

- [ ] **Step 17.3: Shape reads**

Grep for `.current_location_id`, `.parent_location_id`, `.count` (on list responses), `.id` on asset/location API responses; rename:

- `response.data.id` → `response.data.surrogate_id`
- `response.data.current_location_id` → nothing; we now have `response.data.current_location` (a string)
- `response.data.parent_location_id` → `response.data.parent`
- `response.count` → `response.limit`

TypeScript types in `src/types/` need corresponding updates — add `surrogate_id: number`, `current_location: string | null`, drop `org_id`, `current_location_id`, `parent_location_id`.

- [ ] **Step 17.4: Run frontend validate**

```bash
just frontend validate
```

Expected: compiles; tests pass (tests get updated fixtures in the process).

- [ ] **Step 17.5: Commit**

```bash
git add frontend/
git commit -m "refactor(tra-396): frontend consumes new API paths, field names, list envelope"
```

---

## Task 18: EXPLAIN ANALYZE on list joins

**Files:**
- No code changes (verification only)

- [ ] **Step 18.1: Start the backend against a seeded DB**

Use the existing preview env or local docker-compose:

```bash
just backend up  # or equivalent local bring-up command in this repo
```

Seed enough data to make the join cost measurable — at minimum 10k assets spread across 100 locations.

- [ ] **Step 18.2: Run EXPLAIN ANALYZE on ListAssetsFiltered**

Connect via psql (URI matches local docker-compose):

```sql
SET search_path = trakrf,public;
SET app.current_org_id = '<test-org-id>';

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT a.id, a.identifier, a.name, a.current_location_id, l.identifier
FROM trakrf.assets a
LEFT JOIN trakrf.locations l
    ON l.id = a.current_location_id AND l.deleted_at IS NULL
WHERE a.org_id = <test-org-id> AND a.deleted_at IS NULL
ORDER BY a.identifier ASC
LIMIT 50 OFFSET 0;
```

Capture the reported execution time. Expected: single-digit milliseconds given the index on `assets.current_location_id` and the PK on `locations.id`. If >20ms, investigate.

- [ ] **Step 18.3: Run the same without the join as a baseline**

```sql
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT a.id, a.identifier, a.name, a.current_location_id
FROM trakrf.assets a
WHERE a.org_id = <test-org-id> AND a.deleted_at IS NULL
ORDER BY a.identifier ASC
LIMIT 50 OFFSET 0;
```

Compare: ratio of joined:baseline should be under 1.5x.

- [ ] **Step 18.4: Record in the PR description**

Put the two timings and the ratio in the PR body. No commit needed (verification step).

---

## Task 19: End-to-end smoke against the running app

**Files:**
- No code changes.

- [ ] **Step 19.1: Bring up backend + frontend**

```bash
just validate   # full lint + test pass before smoke
```

Local dev bring-up (use whatever this repo's README specifies; do not invent commands):

```bash
# Example if docker-compose is the norm:
just dev        # or: just up; adjust to repo conventions
```

- [ ] **Step 19.2: Create an API key via the UI**

Log in as an admin user, go to Org Settings → API Keys, create a key with `assets:read` and `locations:read` scopes. Copy the JWT.

- [ ] **Step 19.3: Exercise each public endpoint**

```bash
TOKEN="<paste the key JWT>"
BASE="http://localhost:8080/api/v1"

curl -fsS -H "Authorization: Bearer $TOKEN" $BASE/orgs/me | jq
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/assets?limit=5" | jq
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/assets/<some-identifier>" | jq
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/locations?limit=5" | jq
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/locations/<some-identifier>" | jq
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/locations/current?limit=5" | jq
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/assets/<some-identifier>/history?from=2026-03-01T00:00:00Z" | jq
```

Verify shape on every response: `surrogate_id` present, `org_id` absent, `current_location` / `parent` are strings, list envelope uses `limit` / `offset` / `total_count`.

- [ ] **Step 19.4: Verify scope rejection**

Create a second key with only `locations:read`. Confirm `GET /api/v1/assets` returns 403.

- [ ] **Step 19.5: Verify frontend still works**

Open http://localhost:8080 (or whatever the dev URL is). Log in. Navigate to Assets, Locations, and each report page. Confirm:
- Pages render without console errors.
- Detail pages load from `/by-id/${id}` URLs.
- Current-locations and asset-history reports render with correct data.

Take a screenshot or note failures.

- [ ] **Step 19.6: Push branch; open PR**

```bash
git push -u origin feature/tra-396-public-read-endpoints
gh pr create --title "feat(tra-396): read-only public API endpoints — assets, locations, scans, orgs via API key auth" --body "$(cat <<'EOF'
## Summary

- Wires TRA-393 APIKeyAuth + new RequireScope to the first read tranche (assets, locations, current-locations, asset history).
- Introduces EitherAuth middleware (dispatches on JWT iss) so public paths serve both API-key and session callers.
- Normalizes response shapes: public structs, natural-key cross-refs, surrogate_id renamed, org_id / deleted_at dropped, temporal fields kept.
- Route migration: `GET /assets/{identifier}` + `/assets/by-id/{id}` (session FE); same pattern for locations; reports rename `/reports/current-locations` → `/locations/current` and `/reports/assets/{id}/history` → `/assets/{identifier}/history` (public) + `/assets/by-id/{id}/history` (session).
- Request conventions enforced: limit cap 200, unknown param → 400, per-resource filter + sort allowlists via `httputil.ParseListParams`.
- Frontend adapts to new URLs and response shape; no URL-hash or route-params touched.

## Test plan

- [ ] `just validate` clean
- [ ] `just backend test-integration ./...` green
- [ ] EXPLAIN ANALYZE shows <1.5x baseline for assets list join
- [ ] Smoke: every public endpoint returns expected shape with valid key
- [ ] Smoke: wrong-scope → 403; revoked key → 401; session JWT → 401 at APIKeyAuth-only route; api-key JWT → 401 at session Auth route
- [ ] Smoke: frontend pages render without regressions

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Spec coverage check

- [x] Route inventory fully represented (Task 15 router surgery)
- [x] API-key auth on read routes with `RequireScope` (Tasks 2, 3, 15)
- [x] Single public response shape for both auth types (Tasks 11–14)
- [x] Natural-key path params + internal by-id split (Tasks 6, 7, 12, 13, 15)
- [x] Report route renames (Tasks 10, 14, 15)
- [x] Request convention enforcement: limit cap, unknown-param 400, filter allowlist, sort convention (Task 5; used in Tasks 12–14)
- [x] List envelope `{data, limit, offset, total_count}` (Tasks 11–14)
- [x] `LEFT JOIN` for natural-key resolution on lists (Tasks 8, 9)
- [x] `valid_from`/`valid_to` kept; `org_id`/`deleted_at` dropped (Task 11 shapes)
- [x] EitherAuth dispatch via unverified `PeekIssuer` (Tasks 1, 2)
- [x] `GetRequestOrgID` principal-agnostic helper (Task 4)
- [x] Frontend migration (Task 17)
- [x] Integration tests (Task 16)
- [x] EXPLAIN ANALYZE verification (Task 18)
- [x] End-to-end smoke + PR (Task 19)

No placeholders, TODOs, or ambiguous steps. All referenced types, methods, and paths are defined in a preceding task.
