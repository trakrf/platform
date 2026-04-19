# TRA-393 API Key Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the authentication infrastructure, admin UI, and a canary endpoint so external customers can hold an API key against the TrakRF public API, with TRA-396 later applying these middlewares to real business endpoints.

**Architecture:** JWT-based API keys (HS256, shared `JWT_SECRET`, discriminated by `iss` claim) with a DB-backed `api_keys` table for revocation via `jti` lookup. Two middlewares (`APIKeyAuth`, `RequireScope`) sit alongside the existing `middleware.Auth`. Admin CRUD reuses `middleware.RequireOrgAdmin` at `/api/v1/orgs/{id}/api-keys`. Canary endpoint `GET /api/v1/orgs/me` lets customers verify their key works end-to-end without TRA-396's endpoints shipped.

**Tech Stack:** Go (chi, pgx, golang-jwt), PostgreSQL (Timescale) with RLS, React + Vite + Zustand + TanStack Query, Tailwind, Vitest, `//go:build integration` for DB-backed tests.

**Spec:** [docs/superpowers/specs/2026-04-19-tra393-api-key-management-design.md](../specs/2026-04-19-tra393-api-key-management-design.md)

---

## File structure

### Backend (created)

- `backend/migrations/000027_api_keys.up.sql` / `.down.sql` — schema + partial index + RLS policy
- `backend/internal/util/jwt/apikey.go` — `APIKeyClaims`, `GenerateAPIKey`, `ValidateAPIKey`
- `backend/internal/util/jwt/apikey_test.go` — unit tests (no DB)
- `backend/internal/models/apikey/apikey.go` — `APIKey`, `CreateAPIKeyRequest`, `APIKeyResponse`, `APIKeyCreateResponse`, valid scope constants
- `backend/internal/storage/apikeys.go` — `CreateAPIKey`, `ListActiveAPIKeys`, `GetAPIKeyByJTI`, `RevokeAPIKey`, `CountActiveAPIKeys`, `UpdateAPIKeyLastUsed`
- `backend/internal/storage/apikeys_integration_test.go` — tagged integration
- `backend/internal/middleware/apikey.go` — `APIKeyAuth`, `RequireScope`, `APIKeyPrincipal`, `GetAPIKeyPrincipal`
- `backend/internal/middleware/apikey_test.go` — tagged integration
- `backend/internal/handlers/orgs/public.go` — `GetOrgMe` (the canary handler; name disambiguates from existing `me.go` which serves `/users/me`)
- `backend/internal/handlers/orgs/public_integration_test.go`
- `backend/internal/handlers/orgs/api_keys.go` — `CreateAPIKey`, `ListAPIKeys`, `RevokeAPIKey` admin handlers + `RegisterAPIKeyRoutes`
- `backend/internal/handlers/orgs/api_keys_integration_test.go`

### Backend (modified)

- `backend/internal/models/errors/errors.go` — add `ErrRateLimited` constant
- `backend/internal/handlers/orgs/orgs.go` — register `/api/v1/orgs/{id}/api-keys` subroutes in `RegisterRoutes`
- `backend/internal/cmd/serve/router.go` — wire the new public canary route (outside the session-auth group) and confirm session-auth group picks up the admin CRUD via `orgsHandler.RegisterRoutes`

### Frontend (created)

- `frontend/src/types/apiKey.ts` — `APIKey`, `CreateAPIKeyRequest`, `APIKeyCreateResponse`, `Scope` enums
- `frontend/src/lib/api/apiKeys.ts` — `apiKeysApi` wrapper
- `frontend/src/lib/api/apiKeys.test.ts` — request-shape tests
- `frontend/src/components/apikeys/ScopeSelector.tsx` — three dropdowns, stateful
- `frontend/src/components/apikeys/ScopeSelector.test.tsx`
- `frontend/src/components/apikeys/ExpirySelector.tsx` — radio presets + custom date
- `frontend/src/components/apikeys/ExpirySelector.test.tsx`
- `frontend/src/components/apikeys/CreateKeyModal.tsx`
- `frontend/src/components/apikeys/CreateKeyModal.test.tsx`
- `frontend/src/components/apikeys/ShowOnceModal.tsx`
- `frontend/src/components/apikeys/ShowOnceModal.test.tsx`
- `frontend/src/components/apikeys/RevokeConfirmModal.tsx`
- `frontend/src/components/APIKeysScreen.tsx`
- `frontend/src/components/APIKeysScreen.test.tsx`

### Frontend (modified)

- `frontend/src/App.tsx` — lazy-load `APIKeysScreen`, extend `VALID_TABS`, `tabComponents`, `loadingScreens`
- `frontend/src/stores/uiStore.ts` — add `'api-keys'` to `TabType`
- `frontend/src/components/OrgSettingsScreen.tsx` — add link to `#api-keys` (admin-only section)

---

## Conventions and gotchas

- Tables live in the `trakrf` schema; migrations start with `SET search_path=trakrf,public;`
- `id` columns are **INT** (not BIGINT) with a dedicated sequence and `generate_permuted_id('<seq_name>')` trigger — match this pattern exactly to stay consistent with assets/locations/etc.
- Integration tests carry `//go:build integration` and run with `just backend test-integration` (or `go test -tags=integration`); `testutil.SetupTestDB(t)` handles schema + cleanup
- `middleware.RequireOrgAdmin(store)` expects `{id}` or `{orgId}` in the URL — the route `/api/v1/orgs/{id}/api-keys` satisfies this
- Handler registration goes through `orgsHandler.RegisterRoutes(r, store)` which already uses `r.Route("/api/v1/orgs/{id}", ...)` — we extend that same route group
- The canary endpoint `/api/v1/orgs/me` is registered **outside** the session-auth `r.Group` in `router.go` because it uses `APIKeyAuth`, not session `Auth`
- Frontend routes are hash-based; `TabType` union in `stores/uiStore.ts` must include every new route name
- `apiClient` from `frontend/src/lib/api/client.ts` already handles `/api/v1/` prefix and auth header — pass the bare resource path (`/orgs/42/api-keys`, not `/api/v1/orgs/42/api-keys`)
- Commits follow `feat(tra-393): …` / `test(tra-393): …` / `refactor(tra-393): …` on branch `feature/tra-393-api-key-management`; **merge commit on PR, never squash** (per user feedback memory)

---

## Task 1: Migration — `api_keys` table

**Files:**
- Create: `backend/migrations/000027_api_keys.up.sql`
- Create: `backend/migrations/000027_api_keys.down.sql`

- [ ] **Step 1: Write the up migration**

Create `backend/migrations/000027_api_keys.up.sql`:

```sql
SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE api_key_seq;

CREATE TABLE api_keys (
    id           INT PRIMARY KEY,
    jti          UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    org_id       INT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL,
    scopes       TEXT[] NOT NULL,
    created_by   INT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

-- Permuted id trigger (mirrors assets / locations convention)
CREATE TRIGGER generate_api_key_id_trigger
    BEFORE INSERT ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('api_key_seq');

-- Partial index for the dominant UI query ("active keys for this org")
CREATE INDEX idx_api_keys_active_by_org
    ON api_keys(org_id)
    WHERE revoked_at IS NULL;

-- Lookup by jti (UNIQUE constraint already creates an index; this is explicit)
CREATE INDEX idx_api_keys_jti ON api_keys(jti);

-- No RLS on api_keys. See migration 000020 (users / org_users) for precedent:
-- the middleware must read this table BEFORE app.current_org_id is set, and
-- our DB user lacks BYPASSRLS on TimescaleDB Cloud. Org isolation is enforced
-- at the application layer — every storage method in storage/apikeys.go takes
-- orgID explicitly and WHERE-clauses on it.

COMMENT ON TABLE  api_keys IS 'API keys for public API authentication (TRA-393)';
COMMENT ON COLUMN api_keys.jti IS 'JWT ID — revocation handle referenced by api_key JWTs';
COMMENT ON COLUMN api_keys.scopes IS 'Subset of: assets:read, assets:write, locations:read, locations:write, scans:read';
COMMENT ON COLUMN api_keys.expires_at IS 'NULL means never expires';
COMMENT ON COLUMN api_keys.revoked_at IS 'NULL means active';
```

- [ ] **Step 2: Write the down migration**

Create `backend/migrations/000027_api_keys.down.sql`:

```sql
SET search_path=trakrf,public;

DROP TABLE IF EXISTS api_keys;
DROP SEQUENCE IF EXISTS api_key_seq;
```

- [ ] **Step 3: Run the migration to verify it applies cleanly**

```bash
just backend migrate
```

Expected: `migrations applied successfully` with no errors. If the migration fails, fix the SQL before proceeding.

- [ ] **Step 4: Verify rollback works**

```bash
just backend migrate-down 1
just backend migrate
```

Expected: down succeeds, re-up succeeds.

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000027_api_keys.up.sql backend/migrations/000027_api_keys.down.sql
git commit -m "feat(tra-393): api_keys migration with RLS and partial index"
```

---

## Task 2: `ErrRateLimited` error type

**Files:**
- Modify: `backend/internal/models/errors/errors.go`

- [ ] **Step 1: Add the constant**

Edit `backend/internal/models/errors/errors.go` — add `ErrRateLimited` alongside existing constants:

```go
const (
    ErrValidation   ErrorType = "validation_error"
    ErrNotFound     ErrorType = "not_found"
    ErrConflict     ErrorType = "conflict"
    ErrInternal     ErrorType = "internal_error"
    ErrBadRequest   ErrorType = "bad_request"
    ErrUnauthorized ErrorType = "unauthorized"
    ErrForbidden    ErrorType = "forbidden"
    ErrRateLimited  ErrorType = "rate_limited"
)
```

- [ ] **Step 2: Verify compile**

```bash
just backend build
```

Expected: success, no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/models/errors/errors.go
git commit -m "feat(tra-393): add rate_limited error type for future rate-limit sub-issue"
```

---

## Task 3: API-key JWT utilities

**Files:**
- Create: `backend/internal/util/jwt/apikey.go`
- Create: `backend/internal/util/jwt/apikey_test.go`

- [ ] **Step 1: Write failing unit tests**

Create `backend/internal/util/jwt/apikey_test.go`:

```go
package jwt

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestGenerateAndValidateAPIKey(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    jti := "11111111-2222-3333-4444-555555555555"
    orgID := 42
    scopes := []string{"assets:read", "locations:read"}

    token, err := GenerateAPIKey(jti, orgID, scopes, nil)
    require.NoError(t, err)
    require.NotEmpty(t, token)

    claims, err := ValidateAPIKey(token)
    require.NoError(t, err)
    assert.Equal(t, jti, claims.Subject)
    assert.Equal(t, orgID, claims.OrgID)
    assert.Equal(t, scopes, claims.Scopes)
    assert.Equal(t, "trakrf-api-key", claims.Issuer)
    assert.Contains(t, claims.Audience, "trakrf-api")
    assert.Nil(t, claims.ExpiresAt)
}

func TestGenerateAPIKeyWithExpiry(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    exp := time.Now().Add(24 * time.Hour)
    token, err := GenerateAPIKey("jti", 1, []string{"assets:read"}, &exp)
    require.NoError(t, err)

    claims, err := ValidateAPIKey(token)
    require.NoError(t, err)
    require.NotNil(t, claims.ExpiresAt)
    assert.WithinDuration(t, exp, claims.ExpiresAt.Time, time.Second)
}

func TestValidateAPIKeyRejectsSessionToken(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    // Use the existing session Generate (different iss/aud)
    sessionToken, err := Generate(1, "user@example.com", intPtr(42))
    require.NoError(t, err)

    _, err = ValidateAPIKey(sessionToken)
    assert.Error(t, err, "session token must not validate as an api-key token")
}

func TestValidateAPIKeyRejectsExpired(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    past := time.Now().Add(-1 * time.Hour)
    token, err := GenerateAPIKey("jti", 1, []string{"assets:read"}, &past)
    require.NoError(t, err)

    _, err = ValidateAPIKey(token)
    assert.Error(t, err)
}

func TestValidateAPIKeyRejectsBadSignature(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-abc123")

    token, err := GenerateAPIKey("jti", 1, []string{"assets:read"}, nil)
    require.NoError(t, err)

    // Swap the secret before validating
    t.Setenv("JWT_SECRET", "different-secret")
    _, err = ValidateAPIKey(token)
    assert.Error(t, err)
}

func intPtr(i int) *int { return &i }
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./backend/internal/util/jwt/ -run TestGenerateAndValidateAPIKey -v
```

Expected: FAIL — `GenerateAPIKey` / `ValidateAPIKey` / `APIKeyClaims` undefined.

- [ ] **Step 3: Implement**

Create `backend/internal/util/jwt/apikey.go`:

```go
package jwt

import (
    "fmt"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

const (
    apiKeyIssuer   = "trakrf-api-key"
    apiKeyAudience = "trakrf-api"
)

// APIKeyClaims carries the authorization context encoded into an API-key JWT.
type APIKeyClaims struct {
    OrgID  int      `json:"org_id"`
    Scopes []string `json:"scopes"`
    jwt.RegisteredClaims
}

// GenerateAPIKey mints a signed JWT for a newly-created api_keys row.
// sub is the row's jti (UUID string). exp is optional; nil means no expiry claim.
func GenerateAPIKey(jti string, orgID int, scopes []string, exp *time.Time) (string, error) {
    registered := jwt.RegisteredClaims{
        Issuer:   apiKeyIssuer,
        Subject:  jti,
        Audience: jwt.ClaimStrings{apiKeyAudience},
        IssuedAt: jwt.NewNumericDate(time.Now()),
    }
    if exp != nil {
        registered.ExpiresAt = jwt.NewNumericDate(*exp)
    }

    claims := &APIKeyClaims{
        OrgID:            orgID,
        Scopes:           scopes,
        RegisteredClaims: registered,
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    signed, err := token.SignedString([]byte(getSecret()))
    if err != nil {
        return "", fmt.Errorf("sign api-key jwt: %w", err)
    }
    return signed, nil
}

// ValidateAPIKey verifies signature, iss, aud, and exp. Does not consult the DB.
func ValidateAPIKey(tokenString string) (*APIKeyClaims, error) {
    claims := &APIKeyClaims{}

    parser := jwt.NewParser(
        jwt.WithIssuer(apiKeyIssuer),
        jwt.WithAudience(apiKeyAudience),
    )

    token, err := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
        if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
        }
        return []byte(getSecret()), nil
    })
    if err != nil {
        return nil, fmt.Errorf("parse api-key jwt: %w", err)
    }
    if !token.Valid {
        return nil, fmt.Errorf("invalid api-key jwt")
    }
    return claims, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./backend/internal/util/jwt/ -v
```

Expected: all tests PASS, including the five new `TestGenerateAndValidateAPIKey*` variants.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/jwt/apikey.go backend/internal/util/jwt/apikey_test.go
git commit -m "feat(tra-393): APIKeyClaims, GenerateAPIKey, ValidateAPIKey"
```

---

## Task 4: API-key storage layer

**Files:**
- Create: `backend/internal/models/apikey/apikey.go`
- Create: `backend/internal/storage/apikeys.go`
- Create: `backend/internal/storage/apikeys_integration_test.go`

- [ ] **Step 1: Write the model**

Create `backend/internal/models/apikey/apikey.go`:

```go
package apikey

import "time"

// ValidScopes is the canonical set of scope strings accepted by the public API.
var ValidScopes = map[string]bool{
    "assets:read":     true,
    "assets:write":    true,
    "locations:read":  true,
    "locations:write": true,
    "scans:read":      true,
}

// APIKey is the row as stored. Full JWT is NOT stored — only the jti for revocation.
type APIKey struct {
    ID         int        `json:"id"`
    JTI        string     `json:"jti"`
    OrgID      int        `json:"org_id"`
    Name       string     `json:"name"`
    Scopes     []string   `json:"scopes"`
    CreatedBy  int        `json:"created_by"`
    CreatedAt  time.Time  `json:"created_at"`
    ExpiresAt  *time.Time `json:"expires_at,omitempty"`
    LastUsedAt *time.Time `json:"last_used_at,omitempty"`
    RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// CreateAPIKeyRequest is the POST body from the admin UI.
type CreateAPIKeyRequest struct {
    Name      string     `json:"name"      validate:"required,min=1,max=255"`
    Scopes    []string   `json:"scopes"    validate:"required,min=1"`
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyCreateResponse is returned ONCE from POST; Key is the full JWT.
type APIKeyCreateResponse struct {
    Key       string     `json:"key"`
    ID        int        `json:"id"`
    Name      string     `json:"name"`
    Scopes    []string   `json:"scopes"`
    CreatedAt time.Time  `json:"created_at"`
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyListItem is what GET returns — never includes the JWT.
type APIKeyListItem struct {
    ID         int        `json:"id"`
    Name       string     `json:"name"`
    Scopes     []string   `json:"scopes"`
    CreatedAt  time.Time  `json:"created_at"`
    ExpiresAt  *time.Time `json:"expires_at,omitempty"`
    LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// ActiveKeyCap is the per-org soft cap enforced by the POST handler.
const ActiveKeyCap = 10
```

- [ ] **Step 2: Write failing integration tests**

Create `backend/internal/storage/apikeys_integration_test.go`:

```go
//go:build integration
// +build integration

package storage

import (
    "context"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/trakrf/platform/backend/internal/models/apikey"
    "github.com/trakrf/platform/backend/internal/testutil"
)

func createTestUser(t *testing.T, pool *pgxpool.Pool) int {
    t.Helper()
    var id int
    err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_active)
        VALUES ($1, $2, $3, true) RETURNING id`,
        "test user", "testuser@example.com", "stub",
    ).Scan(&id)
    require.NoError(t, err)
    return id
}

func TestAPIKeyStorage_CreateAndGetByJTI(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID := testutil.CreateTestAccount(t, pool)
    userID := createTestUser(t, pool)

    ctx := context.Background()
    scopes := []string{"assets:read", "locations:read"}
    key, err := store.CreateAPIKey(ctx, orgID, "test-key", scopes, userID, nil)
    require.NoError(t, err)
    assert.NotZero(t, key.ID)
    assert.NotEmpty(t, key.JTI)
    assert.Equal(t, orgID, key.OrgID)
    assert.Equal(t, "test-key", key.Name)

    got, err := store.GetAPIKeyByJTI(ctx, key.JTI)
    require.NoError(t, err)
    assert.Equal(t, key.ID, got.ID)
    assert.Equal(t, scopes, got.Scopes)
    assert.Nil(t, got.RevokedAt)
}

func TestAPIKeyStorage_ListExcludesRevoked(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID := testutil.CreateTestAccount(t, pool)
    userID := createTestUser(t, pool)
    ctx := context.Background()

    active, err := store.CreateAPIKey(ctx, orgID, "active", []string{"assets:read"}, userID, nil)
    require.NoError(t, err)
    revoked, err := store.CreateAPIKey(ctx, orgID, "revoked", []string{"assets:read"}, userID, nil)
    require.NoError(t, err)
    require.NoError(t, store.RevokeAPIKey(ctx, orgID, revoked.ID))

    list, err := store.ListActiveAPIKeys(ctx, orgID)
    require.NoError(t, err)
    require.Len(t, list, 1)
    assert.Equal(t, active.ID, list[0].ID)
}

func TestAPIKeyStorage_CountActive(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID := testutil.CreateTestAccount(t, pool)
    userID := createTestUser(t, pool)
    ctx := context.Background()

    for i := 0; i < 3; i++ {
        _, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, userID, nil)
        require.NoError(t, err)
    }
    n, err := store.CountActiveAPIKeys(ctx, orgID)
    require.NoError(t, err)
    assert.Equal(t, 3, n)
}

func TestAPIKeyStorage_RevokeReturnsNotFoundForCrossOrg(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    org1 := testutil.CreateTestAccount(t, pool)
    // Create a second org via raw SQL (testutil.CreateTestAccount truncates; we can't call twice safely)
    var org2 int
    err := pool.QueryRow(context.Background(),
        `INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Org 2', 'org-2', true) RETURNING id`,
    ).Scan(&org2)
    require.NoError(t, err)

    userID := createTestUser(t, pool)
    ctx := context.Background()

    key, err := store.CreateAPIKey(ctx, org1, "org1-key", []string{"assets:read"}, userID, nil)
    require.NoError(t, err)

    // Org 2 must NOT be able to revoke org 1's key
    err = store.RevokeAPIKey(ctx, org2, key.ID)
    assert.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestAPIKeyStorage_UpdateLastUsed(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID := testutil.CreateTestAccount(t, pool)
    userID := createTestUser(t, pool)
    ctx := context.Background()

    key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, userID, nil)
    require.NoError(t, err)
    assert.Nil(t, key.LastUsedAt)

    err = store.UpdateAPIKeyLastUsed(ctx, key.JTI)
    require.NoError(t, err)

    got, err := store.GetAPIKeyByJTI(ctx, key.JTI)
    require.NoError(t, err)
    require.NotNil(t, got.LastUsedAt)
    assert.WithinDuration(t, time.Now(), *got.LastUsedAt, 5*time.Second)
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test -tags=integration ./backend/internal/storage/ -run TestAPIKeyStorage -v
```

Expected: FAIL — storage methods undefined.

- [ ] **Step 4: Implement storage**

Create `backend/internal/storage/apikeys.go`. Because `api_keys` has no RLS (see migration 000020 precedent), every query filters on `org_id` explicitly and none uses `WithOrgTx`:

```go
package storage

import (
    "context"
    stderrors "errors"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/trakrf/platform/backend/internal/models/apikey"
)

// ErrAPIKeyNotFound indicates the caller lacks access or the key does not exist.
var ErrAPIKeyNotFound = stderrors.New("api key not found")

// CreateAPIKey inserts a new active key and returns it (populated id + jti).
func (s *Storage) CreateAPIKey(
    ctx context.Context,
    orgID int,
    name string,
    scopes []string,
    createdBy int,
    expiresAt *time.Time,
) (*apikey.APIKey, error) {
    var k apikey.APIKey
    err := s.pool.QueryRow(ctx, `
        INSERT INTO trakrf.api_keys
            (org_id, name, scopes, created_by, expires_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, jti, org_id, name, scopes, created_by, created_at, expires_at, last_used_at, revoked_at
    `, orgID, name, scopes, createdBy, expiresAt).Scan(
        &k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
        &k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
    )
    if err != nil {
        return nil, fmt.Errorf("insert api_keys: %w", err)
    }
    return &k, nil
}

// ListActiveAPIKeys returns non-revoked keys for the given org, newest first.
func (s *Storage) ListActiveAPIKeys(ctx context.Context, orgID int) ([]apikey.APIKey, error) {
    rows, err := s.pool.Query(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE org_id = $1 AND revoked_at IS NULL
        ORDER BY created_at DESC
    `, orgID)
    if err != nil {
        return nil, fmt.Errorf("list api_keys: %w", err)
    }
    defer rows.Close()

    var out []apikey.APIKey
    for rows.Next() {
        var k apikey.APIKey
        if err := rows.Scan(
            &k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
            &k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
        ); err != nil {
            return nil, fmt.Errorf("scan api_key row: %w", err)
        }
        out = append(out, k)
    }
    return out, rows.Err()
}

// CountActiveAPIKeys returns the active-key count for enforcing the per-org cap.
func (s *Storage) CountActiveAPIKeys(ctx context.Context, orgID int) (int, error) {
    var n int
    err := s.pool.QueryRow(ctx, `
        SELECT COUNT(*) FROM trakrf.api_keys
        WHERE org_id = $1 AND revoked_at IS NULL
    `, orgID).Scan(&n)
    if err != nil {
        return 0, fmt.Errorf("count api_keys: %w", err)
    }
    return n, nil
}

// GetAPIKeyByJTI fetches a key by its jti. The middleware uses this BEFORE
// org context exists (it must discover the org from the returned row).
// Returns ErrAPIKeyNotFound on no match.
func (s *Storage) GetAPIKeyByJTI(ctx context.Context, jti string) (*apikey.APIKey, error) {
    var k apikey.APIKey
    err := s.pool.QueryRow(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE jti = $1
    `, jti).Scan(
        &k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
        &k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
    )
    if err != nil {
        if stderrors.Is(err, pgx.ErrNoRows) {
            return nil, ErrAPIKeyNotFound
        }
        return nil, fmt.Errorf("get api_key by jti: %w", err)
    }
    return &k, nil
}

// RevokeAPIKey marks a key revoked. Returns ErrAPIKeyNotFound if the id is
// not in the given org or is already revoked (no rows updated).
func (s *Storage) RevokeAPIKey(ctx context.Context, orgID, id int) error {
    var revokedID int
    err := s.pool.QueryRow(ctx, `
        UPDATE trakrf.api_keys
        SET revoked_at = NOW()
        WHERE id = $1 AND org_id = $2 AND revoked_at IS NULL
        RETURNING id
    `, id, orgID).Scan(&revokedID)
    if err != nil {
        if stderrors.Is(err, pgx.ErrNoRows) {
            return ErrAPIKeyNotFound
        }
        return fmt.Errorf("revoke api_key: %w", err)
    }
    return nil
}

// UpdateAPIKeyLastUsed bumps last_used_at. Fire-and-forget semantics at the
// middleware layer — callers log but do not fail the request on error.
func (s *Storage) UpdateAPIKeyLastUsed(ctx context.Context, jti string) error {
    _, err := s.pool.Exec(ctx, `
        UPDATE trakrf.api_keys SET last_used_at = NOW() WHERE jti = $1
    `, jti)
    if err != nil {
        return fmt.Errorf("update api_key last_used_at: %w", err)
    }
    return nil
}
```

**Note:** If `s.pool` is not directly accessible from the storage package (e.g., because the field is `pool` vs `Pool`), check `backend/internal/storage/storage.go` and match the pattern used by `WithOrgTx` — that file already accesses the pool directly.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -tags=integration ./backend/internal/storage/ -run TestAPIKeyStorage -v
```

Expected: all five subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/apikey/apikey.go backend/internal/storage/apikeys.go backend/internal/storage/apikeys_integration_test.go
git commit -m "feat(tra-393): api_keys storage layer with RLS-aware CRUD"
```

---

## Task 5: `APIKeyAuth` and `RequireScope` middlewares

**Files:**
- Create: `backend/internal/middleware/apikey.go`
- Create: `backend/internal/middleware/apikey_test.go`

- [ ] **Step 1: Write failing integration tests**

Create `backend/internal/middleware/apikey_test.go`:

```go
//go:build integration
// +build integration

package middleware

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/trakrf/platform/backend/internal/storage"
    "github.com/trakrf/platform/backend/internal/testutil"
    "github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupAPIKey(t *testing.T) (*storage.Storage, func(), int, string) {
    t.Setenv("JWT_SECRET", "test-secret-mid")
    store, cleanup := testutil.SetupTestDB(t)
    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)

    var userID int
    err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_active)
        VALUES ('mw test', 'mwtest@example.com', 'stub', true) RETURNING id`,
    ).Scan(&userID)
    require.NoError(t, err)

    key, err := store.CreateAPIKey(context.Background(), orgID, "mw-key",
        []string{"assets:read"}, userID, nil)
    require.NoError(t, err)

    token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
    require.NoError(t, err)

    return store, cleanup, orgID, token
}

func protectedHandler(w http.ResponseWriter, r *http.Request) {
    p := GetAPIKeyPrincipal(r)
    if p == nil {
        http.Error(w, "no principal", http.StatusInternalServerError)
        return
    }
    _ = json.NewEncoder(w).Encode(map[string]any{"org_id": p.OrgID, "scopes": p.Scopes})
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
    store, cleanup, orgID, token := setupAPIKey(t)
    defer cleanup()

    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()

    APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    var body map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
    assert.Equal(t, float64(orgID), body["org_id"])
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
    store, cleanup, _, _ := setupAPIKey(t)
    defer cleanup()

    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    w := httptest.NewRecorder()
    APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_RejectsSessionToken(t *testing.T) {
    store, cleanup, _, _ := setupAPIKey(t)
    defer cleanup()

    sessionToken, err := jwt.Generate(1, "user@example.com", intPtr(42))
    require.NoError(t, err)

    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    req.Header.Set("Authorization", "Bearer "+sessionToken)
    w := httptest.NewRecorder()
    APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_RevokedKeyRejected(t *testing.T) {
    store, cleanup, orgID, token := setupAPIKey(t)
    defer cleanup()

    // Find key, revoke it
    list, err := store.ListActiveAPIKeys(context.Background(), orgID)
    require.NoError(t, err)
    require.Len(t, list, 1)
    require.NoError(t, store.RevokeAPIKey(context.Background(), orgID, list[0].ID))

    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_DBExpiredKeyRejected(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-mid")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()

    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)

    var userID int
    err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_active)
        VALUES ('exp', 'exp@example.com', 'stub', true) RETURNING id`,
    ).Scan(&userID)
    require.NoError(t, err)

    past := time.Now().Add(-1 * time.Hour)
    key, err := store.CreateAPIKey(context.Background(), orgID, "expired",
        []string{"assets:read"}, userID, &past)
    require.NoError(t, err)

    // Generate a token WITHOUT exp (JWT parser won't reject) — DB check must catch it
    token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
    require.NoError(t, err)

    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_LastUsedBumped(t *testing.T) {
    store, cleanup, orgID, token := setupAPIKey(t)
    defer cleanup()

    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    APIKeyAuth(store)(http.HandlerFunc(protectedHandler)).ServeHTTP(w, req)
    require.Equal(t, http.StatusOK, w.Code)

    list, err := store.ListActiveAPIKeys(context.Background(), orgID)
    require.NoError(t, err)
    require.Len(t, list, 1)
    require.NotNil(t, list[0].LastUsedAt)
    assert.WithinDuration(t, time.Now(), *list[0].LastUsedAt, 5*time.Second)
}

func TestRequireScope(t *testing.T) {
    store, cleanup, _, token := setupAPIKey(t)
    defer cleanup()

    // Key has only "assets:read"; require "assets:write" → 403
    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    chain := APIKeyAuth(store)(RequireScope("assets:write")(http.HandlerFunc(protectedHandler)))
    chain.ServeHTTP(w, req)
    assert.Equal(t, http.StatusForbidden, w.Code)

    // Required scope present → 200
    req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
    req2.Header.Set("Authorization", "Bearer "+token)
    w2 := httptest.NewRecorder()
    chain2 := APIKeyAuth(store)(RequireScope("assets:read")(http.HandlerFunc(protectedHandler)))
    chain2.ServeHTTP(w2, req2)
    assert.Equal(t, http.StatusOK, w2.Code)
}

func intPtr(i int) *int { return &i }
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -tags=integration ./backend/internal/middleware/ -run "TestAPIKeyAuth|TestRequireScope" -v
```

Expected: FAIL — middleware functions undefined.

- [ ] **Step 3: Implement middleware**

Create `backend/internal/middleware/apikey.go`:

```go
package middleware

import (
    "context"
    "net/http"
    "strings"
    "time"

    "github.com/trakrf/platform/backend/internal/logger"
    "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/storage"
    "github.com/trakrf/platform/backend/internal/util/httputil"
    "github.com/trakrf/platform/backend/internal/util/jwt"
)

// APIKeyPrincipal is the authenticated-call identity for the public API.
type APIKeyPrincipal struct {
    OrgID  int
    Scopes []string
    JTI    string
}

const APIKeyPrincipalKey contextKey = "api_key_principal"

// GetAPIKeyPrincipal returns the principal if this request was authenticated via APIKeyAuth.
func GetAPIKeyPrincipal(r *http.Request) *APIKeyPrincipal {
    p, _ := r.Context().Value(APIKeyPrincipalKey).(*APIKeyPrincipal)
    return p
}

// APIKeyAuth validates an API-key JWT, looks up its DB record, and sets the principal on context.
func APIKeyAuth(store *storage.Storage) func(http.Handler) http.Handler {
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

            claims, err := jwt.ValidateAPIKey(parts[1])
            if err != nil {
                logger.Get().Warn().Err(err).Str("request_id", reqID).Msg("api key jwt validation failed")
                httputil.WriteJSONError(w, r, http.StatusUnauthorized,
                    errors.ErrUnauthorized, "Invalid or expired token", "", reqID)
                return
            }

            key, err := store.GetAPIKeyByJTI(r.Context(), claims.Subject)
            if err != nil {
                logger.Get().Warn().Err(err).Str("jti", claims.Subject).Str("request_id", reqID).
                    Msg("api key lookup failed")
                httputil.WriteJSONError(w, r, http.StatusUnauthorized,
                    errors.ErrUnauthorized, "Invalid or expired token", "", reqID)
                return
            }
            if key.RevokedAt != nil {
                logger.Get().Warn().Str("jti", key.JTI).Str("reason", "revoked").Str("request_id", reqID).
                    Msg("api key rejected")
                httputil.WriteJSONError(w, r, http.StatusUnauthorized,
                    errors.ErrUnauthorized, "Invalid or expired token", "", reqID)
                return
            }
            if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
                logger.Get().Warn().Str("jti", key.JTI).Str("reason", "expired").Str("request_id", reqID).
                    Msg("api key rejected")
                httputil.WriteJSONError(w, r, http.StatusUnauthorized,
                    errors.ErrUnauthorized, "Invalid or expired token", "", reqID)
                return
            }

            // Fire-and-forget last_used_at bump. Logged on error, does not fail the request.
            go func(jti string) {
                if err := store.UpdateAPIKeyLastUsed(context.Background(), jti); err != nil {
                    logger.Get().Error().Err(err).Str("jti", jti).Msg("last_used_at update failed")
                }
            }(key.JTI)

            principal := &APIKeyPrincipal{
                OrgID:  key.OrgID,
                Scopes: key.Scopes,
                JTI:    key.JTI,
            }
            ctx := context.WithValue(r.Context(), APIKeyPrincipalKey, principal)
            logger.Get().Info().
                Int("org_id", principal.OrgID).
                Str("jti", principal.JTI).
                Str("request_id", reqID).
                Msg("api key auth success")
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// RequireScope rejects any request whose principal lacks the given scope.
// Must be chained after APIKeyAuth.
func RequireScope(required string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            reqID := GetRequestID(r.Context())
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

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -tags=integration ./backend/internal/middleware/ -run "TestAPIKeyAuth|TestRequireScope" -v
```

Expected: all seven subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/middleware/apikey.go backend/internal/middleware/apikey_test.go
git commit -m "feat(tra-393): APIKeyAuth and RequireScope middleware"
```

---

## Task 6: Canary handler `GET /api/v1/orgs/me`

**Files:**
- Create: `backend/internal/handlers/orgs/public.go`
- Create: `backend/internal/handlers/orgs/public_integration_test.go`
- Modify: `backend/internal/cmd/serve/router.go`

- [ ] **Step 1: Write failing integration test**

Create `backend/internal/handlers/orgs/public_integration_test.go`:

```go
//go:build integration
// +build integration

package orgs

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/trakrf/platform/backend/internal/middleware"
    orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
    "github.com/trakrf/platform/backend/internal/testutil"
    "github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestGetOrgMe_ValidAPIKey(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-public")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)

    var userID int
    err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_active)
        VALUES ('pub', 'pub@example.com', 'stub', true) RETURNING id`,
    ).Scan(&userID)
    require.NoError(t, err)

    key, err := store.CreateAPIKey(context.Background(), orgID, "pub-key",
        []string{"assets:read"}, userID, nil)
    require.NoError(t, err)
    token, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"assets:read"}, nil)
    require.NoError(t, err)

    handler := NewHandler(store, orgsservice.NewService(store))
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", handler.GetOrgMe)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/me", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    var body map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
    assert.Equal(t, float64(orgID), body["id"])
    assert.Equal(t, "Test Organization", body["name"])
}

func TestGetOrgMe_SessionTokenRejected(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-public")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()

    sessionToken, err := jwt.Generate(1, "u@e.com", intPtr(42))
    require.NoError(t, err)

    handler := NewHandler(store, orgsservice.NewService(store))
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", handler.GetOrgMe)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/me", nil)
    req.Header.Set("Authorization", "Bearer "+sessionToken)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func intPtr(i int) *int { return &i }
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -tags=integration ./backend/internal/handlers/orgs/ -run TestGetOrgMe -v
```

Expected: FAIL — `GetOrgMe` and `GetOrgByID` service method undefined.

- [ ] **Step 3: Implement handler**

Create `backend/internal/handlers/orgs/public.go`:

```go
package orgs

import (
    "net/http"

    "github.com/trakrf/platform/backend/internal/middleware"
    modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

// GetOrgMe returns the org that the authenticated API key belongs to.
// Scoped to API-key auth (not session auth); serves as the canary endpoint
// customers hit to verify a key works end-to-end before TRA-396 lands.
func (h *Handler) GetOrgMe(w http.ResponseWriter, r *http.Request) {
    reqID := middleware.GetRequestID(r.Context())
    principal := middleware.GetAPIKeyPrincipal(r)
    if principal == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            "Unauthorized", "", reqID)
        return
    }

    org, err := h.storage.GetOrganizationByID(r.Context(), principal.OrgID)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            "Failed to get organization", "", reqID)
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{
        "id":   org.ID,
        "name": org.Name,
    })
}
```

**Note:** If `storage.GetOrganizationByID` does not exist with that exact name, grep for `Organization` fetch methods in `backend/internal/storage/organizations.go` and use whichever single-org lookup is available (e.g., `GetOrgByID`). Update the handler accordingly — do not invent a new storage method if one already serves this purpose.

- [ ] **Step 4: Wire the route (outside session-auth group)**

Edit `backend/internal/cmd/serve/router.go` — add the API-key-auth'd canary outside the session `r.Group`. After the session-auth group (around line 83), before the test/frontend catch-all:

```go
    // Public API — API-key auth (TRA-393 canary; TRA-396 adds the rest)
    r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", orgsHandler.GetOrgMe)
```

- [ ] **Step 5: Run tests + server builds**

```bash
go test -tags=integration ./backend/internal/handlers/orgs/ -run TestGetOrgMe -v
just backend build
```

Expected: tests PASS, build succeeds.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/orgs/public.go backend/internal/handlers/orgs/public_integration_test.go backend/internal/cmd/serve/router.go
git commit -m "feat(tra-393): GET /api/v1/orgs/me canary endpoint for public API key verification"
```

---

## Task 7: Admin CRUD handlers `/api/v1/orgs/{id}/api-keys`

**Files:**
- Create: `backend/internal/handlers/orgs/api_keys.go`
- Create: `backend/internal/handlers/orgs/api_keys_integration_test.go`
- Modify: `backend/internal/handlers/orgs/orgs.go` (extend `RegisterRoutes` with new subroutes)

- [ ] **Step 1: Write failing integration tests**

Create `backend/internal/handlers/orgs/api_keys_integration_test.go`:

```go
//go:build integration
// +build integration

package orgs

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/trakrf/platform/backend/internal/middleware"
    "github.com/trakrf/platform/backend/internal/models/apikey"
    orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
    "github.com/trakrf/platform/backend/internal/testutil"
    "github.com/trakrf/platform/backend/internal/util/jwt"
)

// seedAdminUser inserts a user with admin role in the org and returns (userID, sessionJWT).
func seedAdminUser(t *testing.T, pool *pgxpool.Pool, orgID int) (int, string) {
    t.Helper()
    var userID int
    err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_active)
        VALUES ('admin', 'admin@example.com', 'stub', true) RETURNING id`,
    ).Scan(&userID)
    require.NoError(t, err)
    _, err = pool.Exec(context.Background(), `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, 'admin')`, orgID, userID)
    require.NoError(t, err)

    token, err := jwt.Generate(userID, "admin@example.com", &orgID)
    require.NoError(t, err)
    return userID, token
}

func TestCreateAPIKey_Admin(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-crud")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)
    _, sessionToken := seedAdminUser(t, pool, orgID)

    handler := NewHandler(store, orgsservice.NewService(store))
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Group(func(r chi.Router) {
        r.Use(middleware.Auth)
        handler.RegisterRoutes(r, store)
    })

    body := map[string]any{
        "name":   "TeamCentral sync",
        "scopes": []string{"assets:read", "locations:read"},
    }
    buf, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost,
        fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+sessionToken)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
    var resp apikey.APIKeyCreateResponse
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    assert.NotEmpty(t, resp.Key)
    assert.Equal(t, "TeamCentral sync", resp.Name)
    assert.Equal(t, []string{"assets:read", "locations:read"}, resp.Scopes)

    // Key must validate as an api-key JWT
    claims, err := jwt.ValidateAPIKey(resp.Key)
    require.NoError(t, err)
    assert.Equal(t, orgID, claims.OrgID)
}

func TestCreateAPIKey_NonAdminForbidden(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-crud")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)

    var userID int
    err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_active)
        VALUES ('viewer', 'v@example.com', 'stub', true) RETURNING id`,
    ).Scan(&userID)
    require.NoError(t, err)
    _, err = pool.Exec(context.Background(), `
        INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'operator')`,
        orgID, userID)
    require.NoError(t, err)

    token, err := jwt.Generate(userID, "v@example.com", &orgID)
    require.NoError(t, err)

    handler := NewHandler(store, orgsservice.NewService(store))
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Group(func(r chi.Router) { r.Use(middleware.Auth); handler.RegisterRoutes(r, store) })

    body := map[string]any{"name": "x", "scopes": []string{"assets:read"}}
    buf, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost,
        fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListAPIKeys_ExcludesRevoked(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-crud")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)
    userID, sessionToken := seedAdminUser(t, pool, orgID)

    // Create two keys, revoke one
    active, err := store.CreateAPIKey(context.Background(), orgID, "active",
        []string{"assets:read"}, userID, nil)
    require.NoError(t, err)
    revoked, err := store.CreateAPIKey(context.Background(), orgID, "revoked",
        []string{"assets:read"}, userID, nil)
    require.NoError(t, err)
    require.NoError(t, store.RevokeAPIKey(context.Background(), orgID, revoked.ID))

    handler := NewHandler(store, orgsservice.NewService(store))
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Group(func(r chi.Router) { r.Use(middleware.Auth); handler.RegisterRoutes(r, store) })

    req := httptest.NewRequest(http.MethodGet,
        fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), nil)
    req.Header.Set("Authorization", "Bearer "+sessionToken)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    var out struct{ Data []apikey.APIKeyListItem `json:"data"` }
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
    require.Len(t, out.Data, 1)
    assert.Equal(t, active.ID, out.Data[0].ID)
    // JWT must NOT appear in list responses
    assert.NotContains(t, w.Body.String(), "eyJ")
}

func TestCreateAPIKey_SoftCap(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-crud")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)
    userID, sessionToken := seedAdminUser(t, pool, orgID)

    // Pre-seed 10 active keys
    for i := 0; i < apikey.ActiveKeyCap; i++ {
        _, err := store.CreateAPIKey(context.Background(), orgID, "k",
            []string{"assets:read"}, userID, nil)
        require.NoError(t, err)
    }

    handler := NewHandler(store, orgsservice.NewService(store))
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Group(func(r chi.Router) { r.Use(middleware.Auth); handler.RegisterRoutes(r, store) })

    body := map[string]any{"name": "over-cap", "scopes": []string{"assets:read"}}
    buf, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost,
        fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+sessionToken)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusConflict, w.Code)
    assert.Contains(t, w.Body.String(), "10")
}

func TestRevokeAPIKey(t *testing.T) {
    t.Setenv("JWT_SECRET", "test-secret-crud")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)
    orgID := testutil.CreateTestAccount(t, pool)
    userID, sessionToken := seedAdminUser(t, pool, orgID)

    key, err := store.CreateAPIKey(context.Background(), orgID, "to-revoke",
        []string{"assets:read"}, userID, nil)
    require.NoError(t, err)

    handler := NewHandler(store, orgsservice.NewService(store))
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Group(func(r chi.Router) { r.Use(middleware.Auth); handler.RegisterRoutes(r, store) })

    req := httptest.NewRequest(http.MethodDelete,
        fmt.Sprintf("/api/v1/orgs/%d/api-keys/%d", orgID, key.ID), nil)
    req.Header.Set("Authorization", "Bearer "+sessionToken)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusNoContent, w.Code)

    // Second delete on same id → 404
    req2 := httptest.NewRequest(http.MethodDelete,
        fmt.Sprintf("/api/v1/orgs/%d/api-keys/%d", orgID, key.ID), nil)
    req2.Header.Set("Authorization", "Bearer "+sessionToken)
    w2 := httptest.NewRecorder()
    r.ServeHTTP(w2, req2)
    assert.Equal(t, http.StatusNotFound, w2.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -tags=integration ./backend/internal/handlers/orgs/ -run "TestCreateAPIKey|TestListAPIKeys|TestRevokeAPIKey" -v
```

Expected: FAIL — admin handlers and route registration missing.

- [ ] **Step 3: Implement handler**

Create `backend/internal/handlers/orgs/api_keys.go`:

```go
package orgs

import (
    "encoding/json"
    stderrors "errors"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/trakrf/platform/backend/internal/middleware"
    "github.com/trakrf/platform/backend/internal/models/apikey"
    modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/storage"
    "github.com/trakrf/platform/backend/internal/util/httputil"
    "github.com/trakrf/platform/backend/internal/util/jwt"
)

// CreateAPIKey handles POST /api/v1/orgs/{id}/api-keys.
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
    reqID := middleware.GetRequestID(r.Context())
    claims := middleware.GetUserClaims(r)
    if claims == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            "Unauthorized", "", reqID)
        return
    }

    orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            "Invalid org id", "", reqID)
        return
    }

    var req apikey.CreateAPIKeyRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            "Invalid JSON", err.Error(), reqID)
        return
    }
    if err := validate.Struct(req); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
            "Validation failed", err.Error(), reqID)
        return
    }
    for _, s := range req.Scopes {
        if !apikey.ValidScopes[s] {
            httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
                "Invalid scope", "Unknown scope: "+s, reqID)
            return
        }
    }

    // Soft cap
    count, err := h.storage.CountActiveAPIKeys(r.Context(), orgID)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            "Failed to check key count", "", reqID)
        return
    }
    if count >= apikey.ActiveKeyCap {
        httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
            "Key limit reached",
            "Organization has reached the 10 active API key limit. Revoke an unused key first.",
            reqID)
        return
    }

    key, err := h.storage.CreateAPIKey(r.Context(), orgID, req.Name, req.Scopes, claims.UserID, req.ExpiresAt)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            "Failed to create api key", "", reqID)
        return
    }

    signed, err := jwt.GenerateAPIKey(key.JTI, orgID, req.Scopes, req.ExpiresAt)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            "Failed to sign api key", "", reqID)
        return
    }

    resp := apikey.APIKeyCreateResponse{
        Key:       signed,
        ID:        key.ID,
        Name:      key.Name,
        Scopes:    key.Scopes,
        CreatedAt: key.CreatedAt,
        ExpiresAt: key.ExpiresAt,
    }
    httputil.WriteJSON(w, http.StatusCreated, resp)
}

// ListAPIKeys handles GET /api/v1/orgs/{id}/api-keys.
func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
    reqID := middleware.GetRequestID(r.Context())
    orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            "Invalid org id", "", reqID)
        return
    }

    keys, err := h.storage.ListActiveAPIKeys(r.Context(), orgID)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            "Failed to list api keys", "", reqID)
        return
    }

    items := make([]apikey.APIKeyListItem, 0, len(keys))
    for _, k := range keys {
        items = append(items, apikey.APIKeyListItem{
            ID:         k.ID,
            Name:       k.Name,
            Scopes:     k.Scopes,
            CreatedAt:  k.CreatedAt,
            ExpiresAt:  k.ExpiresAt,
            LastUsedAt: k.LastUsedAt,
        })
    }
    httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// RevokeAPIKey handles DELETE /api/v1/orgs/{id}/api-keys/{keyID}.
func (h *Handler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
    reqID := middleware.GetRequestID(r.Context())
    orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            "Invalid org id", "", reqID)
        return
    }
    keyID, err := strconv.Atoi(chi.URLParam(r, "keyID"))
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            "Invalid key id", "", reqID)
        return
    }

    if err := h.storage.RevokeAPIKey(r.Context(), orgID, keyID); err != nil {
        if stderrors.Is(err, storage.ErrAPIKeyNotFound) {
            httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
                "Not found", "API key not found", reqID)
            return
        }
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            "Failed to revoke api key", "", reqID)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Register the subroutes**

Edit `backend/internal/handlers/orgs/orgs.go` — inside `RegisterRoutes`, extend the existing `r.Route("/api/v1/orgs/{id}", ...)` block to mount the api-keys subroutes guarded by `RequireOrgAdmin`:

Locate the existing route setup (around `orgs.go:193`):

```go
r.Route("/api/v1/orgs/{id}", func(r chi.Router) {
    r.Use(middleware.RequireOrgMember(store))
    r.Get("/", h.Get)
    r.Put("/", h.Update)
    r.Delete("/", h.Delete)
    // ... members, invitations, etc.
})
```

Add a nested admin-only block for api-keys (inside the `{id}` route):

```go
r.Route("/api/v1/orgs/{id}", func(r chi.Router) {
    r.Use(middleware.RequireOrgMember(store))
    r.Get("/", h.Get)
    r.Put("/", h.Update)
    r.Delete("/", h.Delete)
    // ... existing members / invitations routes unchanged ...

    r.Route("/api-keys", func(r chi.Router) {
        r.Use(middleware.RequireOrgAdmin(store))
        r.Post("/", h.CreateAPIKey)
        r.Get("/", h.ListAPIKeys)
        r.Delete("/{keyID}", h.RevokeAPIKey)
    })
})
```

Note: read `orgs.go` in full first — the exact structure of the existing `r.Route` block may differ slightly. Preserve every existing route; only add the new `r.Route("/api-keys", ...)` inside.

- [ ] **Step 5: Run tests + build**

```bash
go test -tags=integration ./backend/internal/handlers/orgs/ -v
just backend build
```

Expected: all new tests PASS plus existing orgs tests still PASS; build succeeds.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go backend/internal/handlers/orgs/api_keys_integration_test.go backend/internal/handlers/orgs/orgs.go
git commit -m "feat(tra-393): admin CRUD handlers for /api/v1/orgs/{id}/api-keys"
```

---

## Task 8: Frontend types and API client

**Files:**
- Create: `frontend/src/types/apiKey.ts`
- Create: `frontend/src/lib/api/apiKeys.ts`
- Create: `frontend/src/lib/api/apiKeys.test.ts`

- [ ] **Step 1: Write the types**

Create `frontend/src/types/apiKey.ts`:

```ts
export type Scope =
  | 'assets:read'
  | 'assets:write'
  | 'locations:read'
  | 'locations:write'
  | 'scans:read';

export interface APIKey {
  id: number;
  name: string;
  scopes: Scope[];
  created_at: string;
  expires_at: string | null;
  last_used_at: string | null;
}

export interface CreateAPIKeyRequest {
  name: string;
  scopes: Scope[];
  expires_at?: string | null;
}

export interface APIKeyCreateResponse {
  key: string; // full JWT — shown once
  id: number;
  name: string;
  scopes: Scope[];
  created_at: string;
  expires_at: string | null;
}

export const ALL_SCOPES: Scope[] = [
  'assets:read',
  'assets:write',
  'locations:read',
  'locations:write',
  'scans:read',
];

export const ACTIVE_KEY_CAP = 10;
```

- [ ] **Step 2: Write failing tests**

Create `frontend/src/lib/api/apiKeys.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { apiKeysApi } from './apiKeys';
import { apiClient } from './client';

vi.mock('./client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('apiKeysApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('list GETs /orgs/{orgId}/api-keys', async () => {
    (apiClient.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] });
    await apiKeysApi.list(42);
    expect(apiClient.get).toHaveBeenCalledWith('/orgs/42/api-keys');
  });

  it('create POSTs request body to /orgs/{orgId}/api-keys', async () => {
    (apiClient.post as ReturnType<typeof vi.fn>).mockResolvedValue({
      key: 'eyJ...',
      id: 1,
      name: 'x',
      scopes: ['assets:read'],
      created_at: '2026-04-19T00:00:00Z',
      expires_at: null,
    });
    const req = { name: 'x', scopes: ['assets:read' as const] };
    await apiKeysApi.create(42, req);
    expect(apiClient.post).toHaveBeenCalledWith('/orgs/42/api-keys', req);
  });

  it('revoke DELETEs /orgs/{orgId}/api-keys/{id}', async () => {
    (apiClient.delete as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
    await apiKeysApi.revoke(42, 99);
    expect(apiClient.delete).toHaveBeenCalledWith('/orgs/42/api-keys/99');
  });
});
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
just frontend test run -- src/lib/api/apiKeys.test.ts
```

Expected: FAIL — `apiKeysApi` not found.

- [ ] **Step 4: Implement**

Create `frontend/src/lib/api/apiKeys.ts`:

```ts
import { apiClient } from './client';
import type { APIKey, CreateAPIKeyRequest, APIKeyCreateResponse } from '../../types/apiKey';

export const apiKeysApi = {
  list: (orgId: number) =>
    apiClient.get<{ data: APIKey[] }>(`/orgs/${orgId}/api-keys`),

  create: (orgId: number, req: CreateAPIKeyRequest) =>
    apiClient.post<APIKeyCreateResponse>(`/orgs/${orgId}/api-keys`, req),

  revoke: (orgId: number, keyId: number) =>
    apiClient.delete<void>(`/orgs/${orgId}/api-keys/${keyId}`),
};
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
just frontend test run -- src/lib/api/apiKeys.test.ts
just frontend typecheck
```

Expected: tests PASS, typecheck clean.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types/apiKey.ts frontend/src/lib/api/apiKeys.ts frontend/src/lib/api/apiKeys.test.ts
git commit -m "feat(tra-393): frontend types and apiKeysApi client"
```

---

## Task 9: ScopeSelector component

**Files:**
- Create: `frontend/src/components/apikeys/ScopeSelector.tsx`
- Create: `frontend/src/components/apikeys/ScopeSelector.test.tsx`

- [ ] **Step 1: Write failing tests**

Create `frontend/src/components/apikeys/ScopeSelector.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ScopeSelector } from './ScopeSelector';

describe('ScopeSelector', () => {
  it('emits assets:read for "Read" on Assets', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    expect(onChange).toHaveBeenCalledWith(['assets:read']);
  });

  it('emits assets:read + assets:write for "Read + Write"', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'readwrite' } });
    expect(onChange).toHaveBeenCalledWith(['assets:read', 'assets:write']);
  });

  it('preserves other resources when changing one dropdown', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={['locations:read']} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining(['assets:read', 'locations:read']));
  });

  it('shows initial value correctly', () => {
    render(<ScopeSelector value={['assets:read', 'assets:write']} onChange={() => {}} />);
    expect(screen.getByLabelText(/assets/i)).toHaveValue('readwrite');
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
just frontend test run -- src/components/apikeys/ScopeSelector.test.tsx
```

Expected: FAIL — component not found.

- [ ] **Step 3: Implement**

Create `frontend/src/components/apikeys/ScopeSelector.tsx`:

```tsx
import type { Scope } from '@/types/apiKey';

type ResourceLevel = 'none' | 'read' | 'readwrite';

interface Props {
  value: Scope[];
  onChange: (next: Scope[]) => void;
}

type ResourceKey = 'assets' | 'locations' | 'scans';

const RESOURCES: { key: ResourceKey; label: string; hasWrite: boolean }[] = [
  { key: 'assets',    label: 'Assets',    hasWrite: true },
  { key: 'locations', label: 'Locations', hasWrite: true },
  { key: 'scans',     label: 'Scans',     hasWrite: false },
];

function levelFor(resource: ResourceKey, scopes: Scope[]): ResourceLevel {
  const read = scopes.includes(`${resource}:read` as Scope);
  const write = scopes.includes(`${resource}:write` as Scope);
  if (read && write) return 'readwrite';
  if (read) return 'read';
  return 'none';
}

function scopesFor(resource: ResourceKey, level: ResourceLevel): Scope[] {
  if (level === 'none') return [];
  if (level === 'read') return [`${resource}:read` as Scope];
  return [`${resource}:read` as Scope, `${resource}:write` as Scope];
}

export function ScopeSelector({ value, onChange }: Props) {
  const setLevel = (resource: ResourceKey, level: ResourceLevel) => {
    const without = value.filter(
      (s) => !s.startsWith(`${resource}:`),
    );
    onChange([...without, ...scopesFor(resource, level)]);
  };

  return (
    <fieldset className="space-y-3">
      <legend className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
        Permissions
      </legend>
      {RESOURCES.map((r) => {
        const current = levelFor(r.key, value);
        return (
          <div key={r.key} className="flex items-center gap-3">
            <label
              htmlFor={`scope-${r.key}`}
              className="w-24 text-sm text-gray-800 dark:text-gray-200"
            >
              {r.label}
            </label>
            <select
              id={`scope-${r.key}`}
              aria-label={r.label}
              value={current}
              onChange={(e) => setLevel(r.key, e.target.value as ResourceLevel)}
              className="border rounded px-2 py-1 text-sm bg-white dark:bg-gray-800"
            >
              <option value="none">None</option>
              <option value="read">Read</option>
              {r.hasWrite && <option value="readwrite">Read + Write</option>}
            </select>
          </div>
        );
      })}
    </fieldset>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
just frontend test run -- src/components/apikeys/ScopeSelector.test.tsx
just frontend typecheck
```

Expected: tests PASS, typecheck clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/apikeys/ScopeSelector.tsx frontend/src/components/apikeys/ScopeSelector.test.tsx
git commit -m "feat(tra-393): ScopeSelector component with per-resource dropdowns"
```

---

## Task 10: ExpirySelector component

**Files:**
- Create: `frontend/src/components/apikeys/ExpirySelector.tsx`
- Create: `frontend/src/components/apikeys/ExpirySelector.test.tsx`

- [ ] **Step 1: Write failing tests**

Create `frontend/src/components/apikeys/ExpirySelector.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ExpirySelector } from './ExpirySelector';

describe('ExpirySelector', () => {
  it('defaults to "never" and emits null', () => {
    const onChange = vi.fn();
    render(<ExpirySelector value={null} onChange={onChange} />);
    expect(screen.getByLabelText(/never/i)).toBeChecked();
  });

  it('emits an ISO date ~30 days out when 30 days is selected', () => {
    const onChange = vi.fn();
    render(<ExpirySelector value={null} onChange={onChange} />);
    fireEvent.click(screen.getByLabelText(/30 days/i));
    expect(onChange).toHaveBeenCalled();
    const arg = onChange.mock.calls[0][0] as string;
    const diffDays = (new Date(arg).getTime() - Date.now()) / 86_400_000;
    expect(diffDays).toBeGreaterThan(29);
    expect(diffDays).toBeLessThan(31);
  });

  it('shows custom date input when "Custom" is selected', () => {
    render(<ExpirySelector value={null} onChange={() => {}} />);
    fireEvent.click(screen.getByLabelText(/custom/i));
    expect(screen.getByLabelText(/expiry date/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
just frontend test run -- src/components/apikeys/ExpirySelector.test.tsx
```

Expected: FAIL.

- [ ] **Step 3: Implement**

Create `frontend/src/components/apikeys/ExpirySelector.tsx`:

```tsx
import { useState } from 'react';

type Preset = 'never' | '30d' | '90d' | '1y' | 'custom';

interface Props {
  value: string | null; // ISO 8601 or null
  onChange: (next: string | null) => void;
}

function daysFromNow(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString();
}

export function ExpirySelector({ value, onChange }: Props) {
  const [preset, setPreset] = useState<Preset>(value ? 'custom' : 'never');
  const [customDate, setCustomDate] = useState(value ?? '');

  const pick = (p: Preset) => {
    setPreset(p);
    switch (p) {
      case 'never':
        onChange(null);
        break;
      case '30d':
        onChange(daysFromNow(30));
        break;
      case '90d':
        onChange(daysFromNow(90));
        break;
      case '1y':
        onChange(daysFromNow(365));
        break;
      case 'custom':
        onChange(customDate || null);
        break;
    }
  };

  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
        Expiration
      </legend>
      {(
        [
          ['never', 'Never'],
          ['30d', '30 days'],
          ['90d', '90 days'],
          ['1y', '1 year'],
          ['custom', 'Custom date'],
        ] as const
      ).map(([key, label]) => (
        <label key={key} className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name="expiry"
            value={key}
            checked={preset === key}
            onChange={() => pick(key)}
          />
          {label}
        </label>
      ))}
      {preset === 'custom' && (
        <div className="mt-2">
          <label htmlFor="custom-date" className="block text-xs mb-1">
            Expiry date
          </label>
          <input
            id="custom-date"
            type="date"
            aria-label="Expiry date"
            value={customDate.slice(0, 10)}
            onChange={(e) => {
              const iso = new Date(e.target.value).toISOString();
              setCustomDate(iso);
              onChange(iso);
            }}
            className="border rounded px-2 py-1 text-sm bg-white dark:bg-gray-800"
          />
        </div>
      )}
    </fieldset>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
just frontend test run -- src/components/apikeys/ExpirySelector.test.tsx
just frontend typecheck
```

Expected: PASS, typecheck clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/apikeys/ExpirySelector.tsx frontend/src/components/apikeys/ExpirySelector.test.tsx
git commit -m "feat(tra-393): ExpirySelector with preset durations and custom date"
```

---

## Task 11: CreateKeyModal, ShowOnceModal, RevokeConfirmModal

**Files:**
- Create: `frontend/src/components/apikeys/CreateKeyModal.tsx`
- Create: `frontend/src/components/apikeys/CreateKeyModal.test.tsx`
- Create: `frontend/src/components/apikeys/ShowOnceModal.tsx`
- Create: `frontend/src/components/apikeys/ShowOnceModal.test.tsx`
- Create: `frontend/src/components/apikeys/RevokeConfirmModal.tsx`

- [ ] **Step 1: Write failing tests for CreateKeyModal**

Create `frontend/src/components/apikeys/CreateKeyModal.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { CreateKeyModal } from './CreateKeyModal';

describe('CreateKeyModal', () => {
  it('defaults name to "API key — <today>"', () => {
    render(<CreateKeyModal onCreate={() => {}} onCancel={() => {}} />);
    const name = screen.getByLabelText(/name/i) as HTMLInputElement;
    expect(name.value).toMatch(/^API key — \d{4}-\d{2}-\d{2}$/);
  });

  it('blocks submit when no scopes selected', () => {
    const onCreate = vi.fn();
    render(<CreateKeyModal onCreate={onCreate} onCancel={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /create key/i }));
    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByText(/at least one permission/i)).toBeInTheDocument();
  });

  it('calls onCreate with request when form is valid', () => {
    const onCreate = vi.fn();
    render(<CreateKeyModal onCreate={onCreate} onCancel={() => {}} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    fireEvent.click(screen.getByRole('button', { name: /create key/i }));
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        name: expect.stringMatching(/API key/),
        scopes: ['assets:read'],
        expires_at: null,
      }),
    );
  });
});
```

- [ ] **Step 2: Write failing tests for ShowOnceModal**

Create `frontend/src/components/apikeys/ShowOnceModal.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ShowOnceModal } from './ShowOnceModal';

describe('ShowOnceModal', () => {
  it('renders the key value in monospace', () => {
    render(<ShowOnceModal apiKey="eyJTESTtoken" onClose={() => {}} />);
    expect(screen.getByText('eyJTESTtoken')).toBeInTheDocument();
  });

  it('shows the warning banner about one-time display', () => {
    render(<ShowOnceModal apiKey="eyJx" onClose={() => {}} />);
    expect(screen.getByText(/only time you'?ll see the full key/i)).toBeInTheDocument();
  });

  it('disables the Close button until key is copied', () => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
    render(<ShowOnceModal apiKey="eyJx" onClose={() => {}} />);
    expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: /copy/i }));
    // After clipboard write, Close becomes enabled
    return Promise.resolve().then(() => {
      expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeEnabled();
    });
  });
});
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
just frontend test run -- src/components/apikeys/CreateKeyModal.test.tsx src/components/apikeys/ShowOnceModal.test.tsx
```

Expected: FAIL — components not found.

- [ ] **Step 4: Implement CreateKeyModal**

Create `frontend/src/components/apikeys/CreateKeyModal.tsx`:

```tsx
import { useState } from 'react';
import type { CreateAPIKeyRequest, Scope } from '@/types/apiKey';
import { ScopeSelector } from './ScopeSelector';
import { ExpirySelector } from './ExpirySelector';

interface Props {
  onCreate: (req: CreateAPIKeyRequest) => void;
  onCancel: () => void;
  busy?: boolean;
}

function defaultName(): string {
  const today = new Date().toISOString().slice(0, 10);
  return `API key — ${today}`;
}

export function CreateKeyModal({ onCreate, onCancel, busy }: Props) {
  const [name, setName] = useState(defaultName());
  const [scopes, setScopes] = useState<Scope[]>([]);
  const [expires, setExpires] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (scopes.length === 0) {
      setError('Select at least one permission.');
      return;
    }
    setError(null);
    onCreate({ name, scopes, expires_at: expires });
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <form
        onSubmit={submit}
        className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-md space-y-4"
      >
        <h2 className="text-lg font-semibold">Create API key</h2>
        <div>
          <label htmlFor="key-name" className="block text-sm font-medium mb-1">
            Name
          </label>
          <input
            id="key-name"
            aria-label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            maxLength={255}
            className="w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-900"
          />
        </div>
        <ScopeSelector value={scopes} onChange={setScopes} />
        <ExpirySelector value={expires} onChange={setExpires} />
        {error && <p className="text-sm text-red-600">{error}</p>}
        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 text-sm border rounded"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={busy}
            className="px-4 py-2 text-sm bg-blue-600 text-white rounded disabled:opacity-50"
          >
            Create key
          </button>
        </div>
      </form>
    </div>
  );
}
```

- [ ] **Step 5: Implement ShowOnceModal**

Create `frontend/src/components/apikeys/ShowOnceModal.tsx`:

```tsx
import { useState } from 'react';

interface Props {
  apiKey: string;
  onClose: () => void;
}

export function ShowOnceModal({ apiKey, onClose }: Props) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    await navigator.clipboard.writeText(apiKey);
    setCopied(true);
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-lg space-y-4">
        <h2 className="text-lg font-semibold">API key created</h2>
        <div className="bg-amber-100 dark:bg-amber-900/30 border border-amber-300 text-amber-900 dark:text-amber-200 rounded px-3 py-2 text-sm">
          <strong>This is the only time you'll see the full key.</strong> Copy
          it now. If you lose it, revoke this key and create a new one.
        </div>
        <div className="bg-gray-100 dark:bg-gray-900 rounded p-3 break-all font-mono text-xs">
          {apiKey}
        </div>
        <div className="flex justify-between gap-2">
          <button
            type="button"
            onClick={copy}
            className="px-4 py-2 text-sm bg-blue-600 text-white rounded"
          >
            {copied ? 'Copied' : 'Copy'}
          </button>
          <button
            type="button"
            onClick={onClose}
            disabled={!copied}
            className="px-4 py-2 text-sm border rounded disabled:opacity-50"
          >
            I've saved it
          </button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Implement RevokeConfirmModal**

Create `frontend/src/components/apikeys/RevokeConfirmModal.tsx`:

```tsx
interface Props {
  keyName: string;
  onConfirm: () => void;
  onCancel: () => void;
  busy?: boolean;
}

export function RevokeConfirmModal({ keyName, onConfirm, onCancel, busy }: Props) {
  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-md space-y-4">
        <h2 className="text-lg font-semibold">Revoke API key?</h2>
        <p className="text-sm">
          Revoke key <strong>{keyName}</strong>? Applications using this key
          stop working immediately. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 text-sm border rounded"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={busy}
            className="px-4 py-2 text-sm bg-red-600 text-white rounded disabled:opacity-50"
          >
            Revoke
          </button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
just frontend test run -- src/components/apikeys/
just frontend typecheck
```

Expected: all component tests PASS, typecheck clean.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/apikeys/CreateKeyModal.tsx frontend/src/components/apikeys/CreateKeyModal.test.tsx frontend/src/components/apikeys/ShowOnceModal.tsx frontend/src/components/apikeys/ShowOnceModal.test.tsx frontend/src/components/apikeys/RevokeConfirmModal.tsx
git commit -m "feat(tra-393): CreateKeyModal, ShowOnceModal, RevokeConfirmModal"
```

---

## Task 12: APIKeysScreen

**Files:**
- Create: `frontend/src/components/APIKeysScreen.tsx`
- Create: `frontend/src/components/APIKeysScreen.test.tsx`

- [ ] **Step 1: Write failing tests**

Create `frontend/src/components/APIKeysScreen.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import APIKeysScreen from './APIKeysScreen';
import { apiKeysApi } from '@/lib/api/apiKeys';
import { useOrgStore } from '@/stores';

vi.mock('@/lib/api/apiKeys');
vi.mock('@/stores', async () => {
  const actual = await vi.importActual<typeof import('@/stores')>('@/stores');
  return {
    ...actual,
    useOrgStore: vi.fn(),
  };
});

const wrap = (ui: React.ReactElement) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
};

describe('APIKeysScreen', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (useOrgStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      currentOrg: { id: 42, name: 'Acme' },
      currentRole: 'admin',
    });
  });

  it('renders the empty state when no keys exist', async () => {
    (apiKeysApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] });
    wrap(<APIKeysScreen />);
    await waitFor(() =>
      expect(screen.getByText(/no api keys yet/i)).toBeInTheDocument(),
    );
  });

  it('lists existing keys with name and scopes', async () => {
    (apiKeysApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: [
        {
          id: 1,
          name: 'TeamCentral',
          scopes: ['assets:read', 'assets:write', 'locations:read'],
          created_at: '2026-04-01T00:00:00Z',
          expires_at: null,
          last_used_at: null,
        },
      ],
    });
    wrap(<APIKeysScreen />);
    await waitFor(() => expect(screen.getByText('TeamCentral')).toBeInTheDocument());
    expect(screen.getByText(/Assets R\/W/)).toBeInTheDocument();
  });

  it('non-admin sees a forbidden state', () => {
    (useOrgStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      currentOrg: { id: 42, name: 'Acme' },
      currentRole: 'operator',
    });
    wrap(<APIKeysScreen />);
    expect(screen.getByText(/admin/i)).toBeInTheDocument();
  });

  it('create flow: POSTs and shows the key in show-once modal', async () => {
    (apiKeysApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] });
    (apiKeysApi.create as ReturnType<typeof vi.fn>).mockResolvedValue({
      key: 'eyJNEWtoken',
      id: 99,
      name: 'x',
      scopes: ['assets:read'],
      created_at: '2026-04-19T00:00:00Z',
      expires_at: null,
    });
    wrap(<APIKeysScreen />);
    await waitFor(() => screen.getByRole('button', { name: /new key/i }));
    fireEvent.click(screen.getByRole('button', { name: /new key/i }));
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    fireEvent.click(screen.getByRole('button', { name: /create key/i }));

    await waitFor(() => expect(screen.getByText('eyJNEWtoken')).toBeInTheDocument());
    expect(apiKeysApi.create).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ scopes: ['assets:read'] }),
    );
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
just frontend test run -- src/components/APIKeysScreen.test.tsx
```

Expected: FAIL — screen not found.

- [ ] **Step 3: Implement**

Create `frontend/src/components/APIKeysScreen.tsx`:

```tsx
import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import { useOrgStore } from '@/stores';
import { apiKeysApi } from '@/lib/api/apiKeys';
import type { APIKey, CreateAPIKeyRequest, APIKeyCreateResponse, Scope } from '@/types/apiKey';
import { CreateKeyModal } from './apikeys/CreateKeyModal';
import { ShowOnceModal } from './apikeys/ShowOnceModal';
import { RevokeConfirmModal } from './apikeys/RevokeConfirmModal';

function scopeChip(scopes: Scope[], resource: 'assets' | 'locations' | 'scans'): string | null {
  const read = scopes.includes(`${resource}:read` as Scope);
  const write = scopes.includes(`${resource}:write` as Scope);
  if (read && write) return `${resource.charAt(0).toUpperCase()}${resource.slice(1)} R/W`;
  if (read) return `${resource.charAt(0).toUpperCase()}${resource.slice(1)} R`;
  return null;
}

function formatDate(iso: string | null): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString();
}

function formatRelative(iso: string | null): string {
  if (!iso) return 'Never';
  const diffMs = Date.now() - new Date(iso).getTime();
  const hours = Math.floor(diffMs / 3_600_000);
  if (hours < 1) return 'just now';
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export default function APIKeysScreen() {
  const { currentOrg, currentRole } = useOrgStore();
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [newKey, setNewKey] = useState<APIKeyCreateResponse | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<APIKey | null>(null);

  const isAdmin = currentRole === 'owner' || currentRole === 'admin';
  const orgId = currentOrg?.id ?? 0;

  const { data: keys, isLoading } = useQuery({
    queryKey: ['apiKeys', orgId],
    queryFn: async () => {
      const resp = await apiKeysApi.list(orgId);
      return resp.data;
    },
    enabled: isAdmin && orgId > 0,
  });

  const createMutation = useMutation({
    mutationFn: (req: CreateAPIKeyRequest) => apiKeysApi.create(orgId, req),
    onSuccess: (resp) => {
      setCreating(false);
      setNewKey(resp);
      queryClient.invalidateQueries({ queryKey: ['apiKeys', orgId] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Failed to create key');
    },
  });

  const revokeMutation = useMutation({
    mutationFn: (id: number) => apiKeysApi.revoke(orgId, id),
    onSuccess: () => {
      toast.success('Key revoked');
      setRevokeTarget(null);
      queryClient.invalidateQueries({ queryKey: ['apiKeys', orgId] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Failed to revoke key');
    },
  });

  if (!isAdmin) {
    return (
      <div className="max-w-3xl mx-auto py-8">
        <h1 className="text-2xl font-semibold">API Keys</h1>
        <p className="mt-4 text-gray-600 dark:text-gray-400">
          Only organization admins can manage API keys.
        </p>
      </div>
    );
  }

  return (
    <div className="max-w-3xl mx-auto py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold">API Keys</h1>
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="px-4 py-2 text-sm bg-blue-600 text-white rounded"
        >
          New key
        </button>
      </div>

      {isLoading && <p className="text-sm text-gray-500">Loading…</p>}

      {!isLoading && keys?.length === 0 && (
        <p className="text-sm text-gray-600 dark:text-gray-400">
          No API keys yet. Create one to let an external system talk to TrakRF.
        </p>
      )}

      {!isLoading && keys && keys.length > 0 && (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left border-b">
              <th className="py-2">Name</th>
              <th>Scopes</th>
              <th>Created</th>
              <th>Last used</th>
              <th>Expires</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => {
              const chips = (['assets', 'locations', 'scans'] as const)
                .map((r) => scopeChip(k.scopes, r))
                .filter((x): x is string => !!x);
              return (
                <tr key={k.id} className="border-b">
                  <td className="py-2 font-medium">{k.name}</td>
                  <td className="space-x-1">
                    {chips.map((c) => (
                      <span
                        key={c}
                        className="inline-block bg-gray-100 dark:bg-gray-700 rounded px-2 py-0.5 text-xs"
                        title={k.scopes.join(', ')}
                      >
                        {c}
                      </span>
                    ))}
                  </td>
                  <td>{formatDate(k.created_at)}</td>
                  <td>{formatRelative(k.last_used_at)}</td>
                  <td>{k.expires_at ? formatDate(k.expires_at) : 'Never'}</td>
                  <td>
                    <button
                      type="button"
                      onClick={() => setRevokeTarget(k)}
                      className="text-red-600 text-xs hover:underline"
                    >
                      Revoke
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {creating && (
        <CreateKeyModal
          onCreate={(req) => createMutation.mutate(req)}
          onCancel={() => setCreating(false)}
          busy={createMutation.isPending}
        />
      )}

      {newKey && (
        <ShowOnceModal
          apiKey={newKey.key}
          onClose={() => setNewKey(null)}
        />
      )}

      {revokeTarget && (
        <RevokeConfirmModal
          keyName={revokeTarget.name}
          onConfirm={() => revokeMutation.mutate(revokeTarget.id)}
          onCancel={() => setRevokeTarget(null)}
          busy={revokeMutation.isPending}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
just frontend test run -- src/components/APIKeysScreen.test.tsx
just frontend typecheck
```

Expected: PASS, typecheck clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/APIKeysScreen.tsx frontend/src/components/APIKeysScreen.test.tsx
git commit -m "feat(tra-393): APIKeysScreen list/create/revoke with React Query"
```

---

## Task 13: Route registration + OrgSettingsScreen link

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/stores/uiStore.ts`
- Modify: `frontend/src/components/OrgSettingsScreen.tsx`

- [ ] **Step 1: Add `'api-keys'` to TabType**

Read `frontend/src/stores/uiStore.ts` first, then edit the `TabType` union to include `'api-keys'`:

```ts
export type TabType =
  | 'home'
  | 'inventory'
  | 'locate'
  | 'barcode'
  | 'assets'
  | 'locations'
  | 'reports'
  | 'reports-history'
  | 'settings'
  | 'help'
  | 'login'
  | 'signup'
  | 'forgot-password'
  | 'reset-password'
  | 'create-org'
  | 'org-members'
  | 'org-settings'
  | 'accept-invite'
  | 'api-keys';
```

If `TabType` is defined elsewhere (e.g., a types file imported by `uiStore.ts`), edit it there and leave `uiStore.ts` unchanged.

- [ ] **Step 2: Wire the route in App.tsx**

Edit `frontend/src/App.tsx` at three spots:

```tsx
// Near other lazy imports
const APIKeysScreen = lazyWithRetry(() => import('@/components/APIKeysScreen'));

// Extend VALID_TABS
const VALID_TABS: TabType[] = [
  // ...existing tabs...
  'api-keys',
];

// In tabComponents
const tabComponents: Record<string, React.ComponentType<any>> = {
  // ...existing entries...
  'api-keys': APIKeysScreen,
};

// In loadingScreens
const loadingScreens: Record<string, React.ComponentType> = {
  // ...existing entries...
  'api-keys': LoadingScreen,
};
```

- [ ] **Step 3: Add link from OrgSettingsScreen**

Read `frontend/src/components/OrgSettingsScreen.tsx`, then add an admin-only section below the org-name form and above the delete section:

```tsx
{isAdmin && (
  <section className="mt-8 border-t pt-6">
    <h2 className="text-lg font-semibold mb-2">API Keys</h2>
    <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
      Create and manage tokens for external integrations.
    </p>
    <button
      type="button"
      onClick={() => {
        window.location.hash = '#api-keys';
      }}
      className="px-4 py-2 text-sm border rounded"
    >
      Manage API keys →
    </button>
  </section>
)}
```

- [ ] **Step 4: Verify build + typecheck + existing tests**

```bash
just frontend typecheck
just frontend lint
just frontend test run
```

Expected: all pass. Existing `OrgSettingsScreen` and `App` tests still green.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/stores/uiStore.ts frontend/src/components/OrgSettingsScreen.tsx
git commit -m "feat(tra-393): register #api-keys route and OrgSettingsScreen link"
```

---

## Task 14: End-to-end preview deploy verification

**Files:** *(none — manual verification)*

- [ ] **Step 1: Push branch and open PR**

```bash
git push -u origin feature/tra-393-api-key-management
gh pr create \
  --title "feat(tra-393): API key management — JWT auth, scopes, admin UI" \
  --body "$(cat <<'EOF'
## Summary

- JWT-based API key auth with DB-backed revocation via `jti`
- `APIKeyAuth` + `RequireScope` middlewares alongside existing session `Auth`
- Admin CRUD at `/api/v1/orgs/{id}/api-keys` (session-auth + `RequireOrgAdmin`)
- Canary `GET /api/v1/orgs/me` behind API-key auth so customers can verify keys work
- `APIKeysScreen` with create/revoke flows, scope selector, show-once modal
- Soft cap of 10 active keys/org; fire-and-forget `last_used_at`

See [design spec](docs/superpowers/specs/2026-04-19-tra393-api-key-management-design.md).

## Test plan

- [ ] Log in as admin, create a key with Assets R/W + Locations R + Scans R
- [ ] Copy the JWT from the show-once modal
- [ ] `curl -H "Authorization: Bearer <jwt>" https://app.preview.trakrf.id/api/v1/orgs/me` returns 200 + org
- [ ] Revoke the key; same curl returns 401
- [ ] Copy a session JWT from dev tools; curl `/orgs/me` returns 401 (iss mismatch)
- [ ] Create 10 keys rapidly; 11th attempt surfaces a friendly cap message
- [ ] Non-admin user sees "Only organization admins..." message at `#api-keys`
EOF
)"
```

- [ ] **Step 2: Wait for preview deploy, execute the manual test plan above**

Preview URL: `https://app.preview.trakrf.id` (per `CLAUDE.md`; see `.github/workflows/sync-preview.yml`).

Record the outcome of each item in the PR thread. If any step fails, investigate before requesting review.

- [ ] **Step 3: Merge (merge commit, not squash)**

```bash
gh pr merge --merge
```

Per project feedback memory: **never squash**.

---

## Out-of-scope reminders (DO NOT implement)

Listed here so future engineers reading this plan don't get nerd-sniped:

- Rate limiting / `tier` column / `X-RateLimit-*` headers — separate sub-issue
- Public business endpoints (assets, locations, scans) — TRA-396
- OAuth 2.0 client-credentials — v1.x
- Pre-expiry warning emails — v1.x
- Split signing keys (session vs api-key) — v1.x
- In-place key rotation — deliberately not supported; users create-new-revoke-old

---
