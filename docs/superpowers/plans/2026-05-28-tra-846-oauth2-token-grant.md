# TRA-846 OAuth2 Token Grant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `POST /api/v1/oauth/token` supporting OAuth2 `client_credentials` and `refresh_token` grants so integrators exchange a long-lived api-key JWT for short-lived (15 min) access tokens plus a rotating 30-day refresh token.

**Architecture:** Reuse TRA-843's refresh-token table + replay-detection. A migration makes `refresh_tokens.user_id` nullable so `token_type='api'` rows carry `api_key_id` instead of a user. New storage methods write/read the api discriminator; new service methods (`MintAPITokenPair`, `RefreshAPIToken`) mint API-issuer JWTs and rotate; a new handler authenticates the client_credentials request (validate the long-lived JWT, confirm `subject==client_id`, active-check) before minting.

**Tech Stack:** Go, chi router, pgx/pgxpool, golang-jwt/v5, swaggo annotations, schemathesis contract tests.

**Spec:** `docs/superpowers/specs/2026-05-28-tra-846-oauth2-token-grant-design.md`

**Working dir:** worktree at `.worktrees/feat+tra-846-oauth2-token-grant` on branch `feat/tra-846-oauth2-token-grant`. Run Go commands via `just backend <cmd>` from project root, or `cd backend && go ...`.

**Integration tests:** tagged `//go:build integration`, run with `go test -tags integration ./...` against a test DB (`testutil.SetupTestDB`). Plain unit tests run with `just backend test`.

---

## File Structure

- `backend/migrations/000012_refresh_tokens_api_grant.{up,down}.sql` — relax `user_id`, tighten type CHECK.
- `backend/internal/storage/refresh_tokens.go` — add `TokenType`/`APIKeyID` to struct + `GetRefreshTokenByHash`; add `CreateAPIRefreshToken`, `RotateAPIRefreshToken`; `UserID` → `*int`.
- `backend/internal/storage/apikeys.go` — add `GetAPIKeyByID`.
- `backend/internal/services/auth/refresh.go` — adapt to `*int` UserID (session path unchanged behaviorally).
- `backend/internal/services/auth/api_token.go` — NEW: `MintAPITokenPair`, `RefreshAPIToken`, `apiAccessTokenTTL`.
- `backend/internal/models/auth/auth.go` — add `TokenRequest`, `TokenResponse`.
- `backend/internal/handlers/auth/oauth.go` — NEW: `Token` handler + client_credentials auth.
- `backend/internal/handlers/auth/auth.go` — extend `authServicer` interface + register route.
- Tests alongside each.

---

## Task 1: Migration 000012 — relax user_id, tighten type CHECK

**Files:**
- Create: `backend/migrations/000012_refresh_tokens_api_grant.up.sql`
- Create: `backend/migrations/000012_refresh_tokens_api_grant.down.sql`
- Test: `backend/internal/storage/refresh_tokens_api_integration_test.go`

- [ ] **Step 1: Write the failing integration test (raw SQL, asserts new CHECK semantics)**

```go
//go:build integration
// +build integration

package storage_test

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

// An api-type refresh row: user_id NULL, api_key_id set — must be allowed
// after migration 000012.
func TestRefreshTokens_APIRowAllowsNullUser(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(context.Background(), orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	var id int64
	err = pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.refresh_tokens (token_type, user_id, org_id, api_key_id, token_hash, expires_at)
		VALUES ('api', NULL, $1, $2, $3, $4) RETURNING id`,
		orgID, key.ID, "hash_api_row_1", time.Now().Add(time.Hour),
	).Scan(&id)
	require.NoError(t, err)
	assert.NotZero(t, id)
}

// The tightened CHECK must reject an api row that still carries a user_id.
func TestRefreshTokens_APIRowRejectsUser(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(context.Background(), orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.refresh_tokens (token_type, user_id, org_id, api_key_id, token_hash, expires_at)
		VALUES ('api', $1, $2, $3, $4, $5)`,
		userID, orgID, key.ID, "hash_api_bad", time.Now().Add(time.Hour),
	)
	require.Error(t, err) // violates refresh_tokens_type_consistent
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd backend && go test -tags integration ./internal/storage/ -run TestRefreshTokens_APIRow -v`
Expected: FAIL — `TestRefreshTokens_APIRowAllowsNullUser` errors on the NULL user_id insert (column still NOT NULL / old CHECK).

- [ ] **Step 3: Write the up migration**

`backend/migrations/000012_refresh_tokens_api_grant.up.sql`:

```sql
-- TRA-846 — enable the OAuth2 client_credentials + refresh_token grant on the
-- TRA-843 refresh_tokens table. An API integration is not a user, so api-type
-- rows carry api_key_id and no user_id. Relax user_id to nullable and tighten
-- the type-consistency CHECK so session rows require user_id (no api_key_id)
-- and api rows require api_key_id (no user_id).

SET search_path = trakrf, public;

ALTER TABLE refresh_tokens ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_type_consistent;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_type_consistent CHECK (
    (token_type = 'session' AND user_id IS NOT NULL AND api_key_id IS NULL) OR
    (token_type = 'api'     AND user_id IS NULL     AND api_key_id IS NOT NULL)
);

COMMENT ON COLUMN refresh_tokens.user_id IS 'Owning user for session tokens. NULL for token_type=api (TRA-846): an integration is authenticated by its api_keys row, not a user.';
```

- [ ] **Step 4: Write the down migration**

`backend/migrations/000012_refresh_tokens_api_grant.down.sql`:

```sql
-- Reverts TRA-846. Restoring NOT NULL only succeeds if no api rows exist
-- (api rows have NULL user_id); intended for dev rollback before any api
-- tokens are minted.

SET search_path = trakrf, public;

ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_type_consistent;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_type_consistent CHECK (
    (token_type = 'session' AND api_key_id IS NULL) OR
    (token_type = 'api'     AND api_key_id IS NOT NULL)
);

ALTER TABLE refresh_tokens ALTER COLUMN user_id SET NOT NULL;
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd backend && go test -tags integration ./internal/storage/ -run TestRefreshTokens_APIRow -v`
Expected: PASS — `SetupTestDB` applies migrations through 000012; null-user api insert succeeds, api-with-user insert rejected.

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/000012_refresh_tokens_api_grant.up.sql \
        backend/migrations/000012_refresh_tokens_api_grant.down.sql \
        backend/internal/storage/refresh_tokens_api_integration_test.go
git commit -m "feat(auth): migration 000012 — nullable user_id for api refresh tokens (TRA-846)"
```

---

## Task 2: Storage — api refresh-token methods + GetAPIKeyByID + nullable UserID

**Files:**
- Modify: `backend/internal/storage/refresh_tokens.go`
- Modify: `backend/internal/storage/apikeys.go`
- Modify: `backend/internal/services/auth/refresh.go` (adapt to `*int` UserID)
- Test: `backend/internal/storage/refresh_tokens_api_integration_test.go` (append)

- [ ] **Step 1: Write failing tests for the new storage methods**

Append to `backend/internal/storage/refresh_tokens_api_integration_test.go`:

```go
func TestStorage_CreateAndGetAPIRefreshToken(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	id, err := store.CreateAPIRefreshToken(ctx, int64(key.ID), &orgID, "hash_get_1", time.Now().Add(time.Hour), "ua", "1.2.3.4")
	require.NoError(t, err)
	assert.NotZero(t, id)

	row, err := store.GetRefreshTokenByHash(ctx, "hash_get_1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "api", row.TokenType)
	assert.Nil(t, row.UserID)
	require.NotNil(t, row.APIKeyID)
	assert.Equal(t, int64(key.ID), *row.APIKeyID)
}

func TestStorage_RotateAPIRefreshToken(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	oldID, err := store.CreateAPIRefreshToken(ctx, int64(key.ID), &orgID, "hash_rot_old", time.Now().Add(time.Hour), "", "")
	require.NoError(t, err)

	newID, err := store.RotateAPIRefreshToken(ctx, oldID, int64(key.ID), &orgID, "hash_rot_new", time.Now().Add(time.Hour), "", "")
	require.NoError(t, err)
	assert.NotEqual(t, oldID, newID)

	old, err := store.GetRefreshTokenByHash(ctx, "hash_rot_old")
	require.NoError(t, err)
	assert.NotNil(t, old.UsedAt)
	require.NotNil(t, old.ReplacedBy)
	assert.Equal(t, newID, *old.ReplacedBy)
}

func TestStorage_GetAPIKeyByID(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read", "locations:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	got, err := store.GetAPIKeyByID(ctx, int64(key.ID))
	require.NoError(t, err)
	assert.Equal(t, key.JTI, got.JTI)
	assert.Equal(t, []string{"assets:read", "locations:read"}, got.Scopes)

	_, err = store.GetAPIKeyByID(ctx, 999999999)
	require.ErrorIs(t, err, storage.ErrAPIKeyNotFound)
}

func TestStorage_APIKeyDeleteCascadesRefreshTokens(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	_, err = store.CreateAPIRefreshToken(ctx, int64(key.ID), &orgID, "hash_cascade", time.Now().Add(time.Hour), "", "")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM trakrf.api_keys WHERE id = $1`, key.ID)
	require.NoError(t, err)

	row, err := store.GetRefreshTokenByHash(ctx, "hash_cascade")
	require.NoError(t, err)
	assert.Nil(t, row, "refresh token should be CASCADE-deleted with its api_keys row")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd backend && go test -tags integration ./internal/storage/ -run 'TestStorage_(CreateAndGetAPIRefreshToken|RotateAPIRefreshToken|GetAPIKeyByID|APIKeyDeleteCascades)' -v`
Expected: FAIL to compile — `CreateAPIRefreshToken`, `RotateAPIRefreshToken`, `GetAPIKeyByID`, and `RefreshToken.TokenType`/`.APIKeyID` undefined; `row.UserID` is not a pointer.

- [ ] **Step 3: Update the `RefreshToken` struct + `GetRefreshTokenByHash`**

In `backend/internal/storage/refresh_tokens.go`, change the struct (UserID → `*int`, add fields):

```go
// RefreshToken represents a row in trakrf.refresh_tokens.
type RefreshToken struct {
	ID         int64
	TokenType  string
	UserID     *int
	OrgID      *int
	APIKeyID   *int64
	TokenHash  string
	UserAgent  *string
	IP         *net.IP
	CreatedAt  time.Time
	ExpiresAt  time.Time
	UsedAt     *time.Time
	ReplacedBy *int64
	RevokedAt  *time.Time
}
```

Replace `GetRefreshTokenByHash`'s query + scan to include `token_type` and `api_key_id`:

```go
func (s *Storage) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var t RefreshToken
	var ipStr *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, token_type, user_id, org_id, api_key_id, token_hash, user_agent, host(ip), created_at, expires_at, used_at, replaced_by, revoked_at
		FROM trakrf.refresh_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(
		&t.ID, &t.TokenType, &t.UserID, &t.OrgID, &t.APIKeyID, &t.TokenHash, &t.UserAgent, &ipStr,
		&t.CreatedAt, &t.ExpiresAt, &t.UsedAt, &t.ReplacedBy, &t.RevokedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}
	if ipStr != nil {
		parsed := net.ParseIP(*ipStr)
		if parsed != nil {
			t.IP = &parsed
		}
	}
	return &t, nil
}
```

- [ ] **Step 4: Add `CreateAPIRefreshToken` and `RotateAPIRefreshToken`**

Append to `backend/internal/storage/refresh_tokens.go`:

```go
// CreateAPIRefreshToken inserts a token_type='api' refresh row (user_id NULL,
// api_key_id set) and returns its ID. Mirrors CreateRefreshToken for the
// OAuth2 client_credentials grant (TRA-846).
func (s *Storage) CreateAPIRefreshToken(ctx context.Context, apiKeyID int64, orgID *int, tokenHash string, expiresAt time.Time, userAgent, ipStr string) (int64, error) {
	var ua, ip any
	if userAgent != "" {
		ua = userAgent
	}
	if parsed := net.ParseIP(ipStr); parsed != nil {
		ip = parsed.String()
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO trakrf.refresh_tokens (token_type, api_key_id, org_id, token_hash, expires_at, user_agent, ip)
		VALUES ('api', $1, $2, $3, $4, $5, $6)
		RETURNING id
	`, apiKeyID, orgID, tokenHash, expiresAt, ua, ip).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create api refresh token: %w", err)
	}
	return id, nil
}

// RotateAPIRefreshToken atomically marks the old api token used and inserts a
// new api row, linking old.replaced_by → new.id. Returns the new row's ID.
func (s *Storage) RotateAPIRefreshToken(ctx context.Context, oldID, apiKeyID int64, orgID *int, newHash string, expiresAt time.Time, userAgent, ipStr string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin rotate tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var ua, ip any
	if userAgent != "" {
		ua = userAgent
	}
	if parsed := net.ParseIP(ipStr); parsed != nil {
		ip = parsed.String()
	}

	var newID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO trakrf.refresh_tokens (token_type, api_key_id, org_id, token_hash, expires_at, user_agent, ip)
		VALUES ('api', $1, $2, $3, $4, $5, $6)
		RETURNING id
	`, apiKeyID, orgID, newHash, expiresAt, ua, ip).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("insert new api refresh row: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE trakrf.refresh_tokens
		SET used_at = NOW(), replaced_by = $2
		WHERE id = $1
	`, oldID, newID)
	if err != nil {
		return 0, fmt.Errorf("mark old api refresh row used: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit api rotate tx: %w", err)
	}
	return newID, nil
}
```

- [ ] **Step 5: Add `GetAPIKeyByID`**

Append to `backend/internal/storage/apikeys.go` (after `GetAPIKeyByJTI`):

```go
// GetAPIKeyByID fetches a key by its obfuscated id. Used by the refresh-token
// grant: api refresh rows reference api_keys.id, and re-mint reads the current
// scopes/jti/active-state from the row (TRA-846).
func (s *Storage) GetAPIKeyByID(ctx context.Context, id int64) (*apikey.APIKey, error) {
	var k apikey.APIKey
	err := s.pool.QueryRow(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_by_key_id,
               created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE id = $1
    `, id).Scan(
		&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
		&k.CreatedBy, &k.CreatedByKeyID,
		&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
	)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("get api_key by id: %w", err)
	}
	return &k, nil
}
```

- [ ] **Step 6: Adapt session refresh path to `*int` UserID**

In `backend/internal/services/auth/refresh.go`, the session `Refresh` reads `row.UserID` (now `*int`). Update the user lookup + rotate call (session rows always have non-null user_id):

```go
	if row.UserID == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	usr, err := s.storage.GetUserByID(ctx, *row.UserID)
	if err != nil || usr == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	accessToken, err := generateJWT(usr.ID, usr.Email, row.OrgID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access JWT: %w", err)
	}

	newSecret, err := generateRefreshSecret()
	if err != nil {
		return nil, err
	}

	_, err = s.storage.RotateRefreshToken(
		ctx, row.ID, *row.UserID, row.OrgID, hashRefreshSecret(newSecret),
		time.Now().Add(refreshTokenTTL), userAgent, ip,
	)
```

(The `Logout` path uses `row.ID` only — unaffected. `MintTokenPair` still takes `userID int` and calls `CreateRefreshToken` — unaffected.)

- [ ] **Step 7: Run the storage tests + full build**

Run: `cd backend && go test -tags integration ./internal/storage/ -run 'TestStorage_(CreateAndGetAPIRefreshToken|RotateAPIRefreshToken|GetAPIKeyByID|APIKeyDeleteCascades)' -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/storage/refresh_tokens.go backend/internal/storage/apikeys.go \
        backend/internal/services/auth/refresh.go \
        backend/internal/storage/refresh_tokens_api_integration_test.go
git commit -m "feat(auth): storage methods for api refresh tokens + GetAPIKeyByID (TRA-846)"
```

---

## Task 3: Service — MintAPITokenPair + RefreshAPIToken

**Files:**
- Create: `backend/internal/services/auth/api_token.go`
- Test: `backend/internal/services/auth/api_token_integration_test.go`

Note: these methods touch storage, so they are exercised via integration tests (real DB) rather than mocks, matching the storage-test style.

- [ ] **Step 1: Write the failing tests**

`backend/internal/services/auth/api_token_integration_test.go`:

```go
//go:build integration
// +build integration

package auth_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func newAPITokenService(t *testing.T) (*authservice.Service, *pgxpool.Pool, func()) {
	t.Helper()
	store, cleanup := testutil.SetupTestDB(t)
	pool := store.Pool().(*pgxpool.Pool)
	svc := authservice.NewService(pool, store, nil)
	return svc, pool, cleanup
}

func TestMintAPITokenPair_IssuesShortLivedJWT(t *testing.T) {
	svc, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	var userID int
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u','apitok@example.com','x') RETURNING id`).Scan(&userID))
	store := testutil.StorageFromPool(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read", "locations:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	access, refresh, expiresIn, err := svc.MintAPITokenPair(ctx, key.JTI, key.Scopes, orgID, int64(key.ID), "ua", "1.2.3.4")
	require.NoError(t, err)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)
	assert.Equal(t, 900, expiresIn) // 15 min

	claims, err := jwt.ValidateAPIKey(access)
	require.NoError(t, err)
	assert.Equal(t, key.JTI, claims.Subject)
	assert.Equal(t, orgID, claims.OrgID)
	assert.ElementsMatch(t, []string{"assets:read", "locations:read"}, claims.Scopes)
	require.NotNil(t, claims.ExpiresAt) // short-lived: exp is set
}

func TestRefreshAPIToken_RotatesWithCurrentScopes(t *testing.T) {
	svc, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	var userID int
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u','apitok2@example.com','x') RETURNING id`).Scan(&userID))
	store := testutil.StorageFromPool(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	_, refresh, _, err := svc.MintAPITokenPair(ctx, key.JTI, key.Scopes, orgID, int64(key.ID), "", "")
	require.NoError(t, err)

	resp, err := svc.RefreshAPIToken(ctx, refresh, "", "")
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEqual(t, refresh, resp.RefreshToken)
	assert.Equal(t, 900, resp.ExpiresIn)

	// Old refresh is now used → replay revokes the chain.
	_, err = svc.RefreshAPIToken(ctx, refresh, "", "")
	require.Error(t, err)
	// The freshly rotated token is also revoked by the chain-revoke.
	_, err = svc.RefreshAPIToken(ctx, resp.RefreshToken, "", "")
	require.Error(t, err)
}

func TestRefreshAPIToken_RejectsSessionToken(t *testing.T) {
	svc, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	var userID int
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u','apitok3@example.com','x') RETURNING id`).Scan(&userID))
	store := testutil.StorageFromPool(t, pool)

	// Mint a SESSION token pair, then present it at the API refresh endpoint.
	_, sessionRefresh, _, err := svc.MintTokenPair(ctx, userID, "apitok3@example.com", &orgID, "", "", jwt.Generate)
	require.NoError(t, err)
	_ = store

	_, err = svc.RefreshAPIToken(ctx, sessionRefresh, "", "")
	require.Error(t, err) // cross-type rejection
}

func TestRefreshAPIToken_RejectsRevokedKey(t *testing.T) {
	svc, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	var userID int
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u','apitok4@example.com','x') RETURNING id`).Scan(&userID))
	store := testutil.StorageFromPool(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	_, refresh, _, err := svc.MintAPITokenPair(ctx, key.JTI, key.Scopes, orgID, int64(key.ID), "", "")
	require.NoError(t, err)

	require.NoError(t, store.RevokeAPIKey(ctx, orgID, key.ID))
	_, err = svc.RefreshAPIToken(ctx, refresh, "", "")
	require.Error(t, err) // key revoked → refresh rejected
}
```

Note: `testutil.StorageFromPool` is a tiny helper added in Step 2 if it doesn't already exist; if `testutil` already exposes the `*storage.Storage` from `SetupTestDB`, use that directly and drop the helper. **Check `internal/testutil` first** — `SetupTestDB` returns `(store, cleanup)`, so prefer threading that `store` through instead of adding a helper.

- [ ] **Step 2: Adjust the test to use the store returned by SetupTestDB**

`SetupTestDB` already returns `store`. Refactor `newAPITokenService` to also return it so tests don't need `StorageFromPool`:

```go
func newAPITokenService(t *testing.T) (*authservice.Service, *storage.Storage, *pgxpool.Pool, func()) {
	t.Helper()
	store, cleanup := testutil.SetupTestDB(t)
	pool := store.Pool().(*pgxpool.Pool)
	svc := authservice.NewService(pool, store, nil)
	return svc, store, pool, cleanup
}
```

Update each test to `svc, store, pool, cleanup := newAPITokenService(t)` and delete the `StorageFromPool` calls. Add `"github.com/trakrf/platform/backend/internal/storage"` to imports.

- [ ] **Step 3: Run to verify it fails**

Run: `cd backend && go test -tags integration ./internal/services/auth/ -run 'TestMintAPITokenPair|TestRefreshAPIToken' -v`
Expected: FAIL to compile — `MintAPITokenPair`, `RefreshAPIToken`, `APITokenResponse` undefined.

- [ ] **Step 4: Implement the service**

`backend/internal/services/auth/api_token.go`:

```go
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// apiAccessTokenTTL is the lifetime of an API access JWT. Short by design: a
// leaked access token self-expires quickly; the integrator silently refreshes.
const apiAccessTokenTTL = 15 * time.Minute

// APITokenResponse is the result of an OAuth2 token grant (client_credentials
// or refresh_token) for the public API.
type APITokenResponse struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// MintAPITokenPair issues a short-lived API access JWT + a rotating refresh
// token for an authenticated client_credentials request. The caller
// (handler) is responsible for authenticating the client first.
func (s *Service) MintAPITokenPair(ctx context.Context, jti string, scopes []string, orgID int, apiKeyID int64, userAgent, ip string) (accessToken, refreshSecret string, expiresIn int, err error) {
	exp := time.Now().Add(apiAccessTokenTTL)
	accessToken, err = jwt.GenerateAPIKey(jti, orgID, scopes, &exp)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to generate api access JWT: %w", err)
	}

	refreshSecret, err = generateRefreshSecret()
	if err != nil {
		return "", "", 0, err
	}

	orgIDPtr := orgID
	_, err = s.storage.CreateAPIRefreshToken(
		ctx, apiKeyID, &orgIDPtr, hashRefreshSecret(refreshSecret),
		time.Now().Add(refreshTokenTTL), userAgent, ip,
	)
	if err != nil {
		return "", "", 0, err
	}

	return accessToken, refreshSecret, int(apiAccessTokenTTL.Seconds()), nil
}

// RefreshAPIToken exchanges a current api refresh token for a new pair. Reuses
// TRA-843 rotation + replay-detection. Scopes are re-read from the api_keys
// row at mint time (single source of truth), so a scope change on the key
// takes effect on the next refresh.
func (s *Service) RefreshAPIToken(ctx context.Context, presentedSecret, userAgent, ip string) (*APITokenResponse, error) {
	hash := hashRefreshSecret(presentedSecret)
	row, err := s.storage.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("lookup refresh token: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	// Only api-type tokens may be exchanged here; a session token is invalid.
	if row.TokenType != "api" || row.APIKeyID == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if row.RevokedAt != nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if row.UsedAt != nil {
		// Replay of an already-rotated token → chain compromise.
		if revokeErr := s.storage.RevokeRefreshTokenChain(ctx, row.ID); revokeErr != nil {
			fmt.Printf("Warning: failed to revoke api refresh chain after replay: %v\n", revokeErr)
		}
		fmt.Printf("WARN api refresh-token replay detected api_key_id=%d token_id=%d\n", *row.APIKeyID, row.ID)
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	key, err := s.storage.GetAPIKeyByID(ctx, *row.APIKeyID)
	if err != nil || key == nil {
		return nil, fmt.Errorf("invalid_refresh_token")
	}
	if key.RevokedAt != nil || (key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now())) {
		return nil, fmt.Errorf("invalid_refresh_token")
	}

	exp := time.Now().Add(apiAccessTokenTTL)
	accessToken, err := jwt.GenerateAPIKey(key.JTI, key.OrgID, key.Scopes, &exp)
	if err != nil {
		return nil, fmt.Errorf("failed to generate api access JWT: %w", err)
	}

	newSecret, err := generateRefreshSecret()
	if err != nil {
		return nil, err
	}

	_, err = s.storage.RotateAPIRefreshToken(
		ctx, row.ID, int64(key.ID), row.OrgID, hashRefreshSecret(newSecret),
		time.Now().Add(refreshTokenTTL), userAgent, ip,
	)
	if err != nil {
		return nil, fmt.Errorf("rotate api refresh token: %w", err)
	}

	return &APITokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newSecret,
		ExpiresIn:    int(apiAccessTokenTTL.Seconds()),
	}, nil
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd backend && go test -tags integration ./internal/services/auth/ -run 'TestMintAPITokenPair|TestRefreshAPIToken' -v`
Expected: PASS (all four refresh cases + mint).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/auth/api_token.go \
        backend/internal/services/auth/api_token_integration_test.go
git commit -m "feat(auth): MintAPITokenPair + RefreshAPIToken service methods (TRA-846)"
```

---

## Task 4: Models — TokenRequest / TokenResponse

**Files:**
- Modify: `backend/internal/models/auth/auth.go`

- [ ] **Step 1: Add the request/response models**

Append to `backend/internal/models/auth/auth.go`:

```go
// TokenRequest is the body for POST /api/v1/oauth/token. The OAuth2 grant is
// selected by grant_type; the remaining fields are conditionally required.
type TokenRequest struct {
	GrantType    string `json:"grant_type" validate:"required,oneof=client_credentials refresh_token" example:"client_credentials"`
	ClientID     string `json:"client_id,omitempty" example:"6f1c2a8e-7d3b-4e90-9a11-2c4d5e6f7a8b"`
	ClientSecret string `json:"client_secret,omitempty" example:"eyJhbGciOiJIUzI1Ni␣...long-lived api key JWT"`
	RefreshToken string `json:"refresh_token,omitempty" example:"f3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
}

// TokenResponse is the OAuth2 token grant result for the public API.
type TokenResponse struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1Ni␣..."`
	RefreshToken string `json:"refresh_token" example:"f3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
	TokenType    string `json:"token_type" example:"Bearer"`
	ExpiresIn    int    `json:"expires_in" example:"900"`
}
```

(Replace the `␣` placeholder above with nothing — it marks where a real example value continues; keep the example strings short and valid.)

- [ ] **Step 2: Build**

Run: `cd backend && go build ./internal/models/...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/models/auth/auth.go
git commit -m "feat(auth): TokenRequest/TokenResponse models for oauth/token (TRA-846)"
```

---

## Task 5: Handler — oauth.go + interface + route

**Files:**
- Create: `backend/internal/handlers/auth/oauth.go`
- Modify: `backend/internal/handlers/auth/auth.go` (extend `authServicer`, register route)
- Test: `backend/internal/handlers/auth/oauth_test.go`

- [ ] **Step 1: Write the failing handler tests (stub service)**

`backend/internal/handlers/auth/oauth_test.go`:

```go
package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
)

// The handler authenticates client_credentials itself (it validates the
// long-lived api-key JWT and looks up the key); that requires a real DB, so
// the happy-path client_credentials test lives in the integration suite
// (Task 7). These unit tests cover request-shape validation + the refresh
// delegation via a stub, which need no DB.

type stubTokenService struct {
	authservice.Service // not used; we exercise refresh via interface
	refreshResp *authservice.APITokenResponse
	refreshErr  error
}

func TestToken_RejectsUnknownGrantType(t *testing.T) {
	h := authhandler.NewHandler(nil)
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	body, _ := json.Marshal(map[string]string{"grant_type": "password"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestToken_ClientCredentialsRequiresClientFields(t *testing.T) {
	h := authhandler.NewHandler(nil)
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	body, _ := json.Marshal(map[string]string{"grant_type": "client_credentials"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

var _ = context.Background // keep import if unused after edits
```

Note: `NewHandler(nil)` is used only for the no-service validation paths that return before touching the service. If `NewHandler` requires a non-nil service to construct, pass a zero stub that satisfies `authServicer`. **Confirm the existing `auth_test.go` stub pattern (`stubAuthService`) and reuse it** — extend that stub with `MintAPITokenPair`/`RefreshAPIToken` no-op methods rather than introducing a new one. Replace `stubTokenService` accordingly.

- [ ] **Step 2: Run to verify it fails**

Run: `cd backend && go test ./internal/handlers/auth/ -run TestToken -v`
Expected: FAIL to compile — `Token` handler + route not registered; `authServicer` lacks the new methods.

- [ ] **Step 3: Extend the `authServicer` interface**

In `backend/internal/handlers/auth/auth.go`, add to the `authServicer` interface:

```go
	MintAPITokenPair(ctx context.Context, jti string, scopes []string, orgID int, apiKeyID int64, userAgent, ip string) (accessToken, refreshSecret string, expiresIn int, err error)
	RefreshAPIToken(ctx context.Context, presentedSecret, userAgent, ip string) (*authservice.APITokenResponse, error)
```

(`authservice` is already imported in auth.go.)

- [ ] **Step 4: Implement the handler**

`backend/internal/handlers/auth/oauth.go`:

```go
package auth

import (
	"net/http"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// Token is the OAuth2 token endpoint for the public API.
//
// @Summary OAuth2 token grant
// @Description Exchange API credentials for a short-lived (15 min) access token + a rotating 30-day refresh token. Two grants are supported. `client_credentials`: supply `client_id` (the API key's id/jti) and `client_secret` (the long-lived API key token from key creation). `refresh_token`: supply a current `refresh_token` to rotate the pair; presenting an already-used refresh token revokes the whole chain and returns 401.
// @Tags oauth,public
// @Accept json
// @Produce json
// @Param request body auth.TokenRequest true "Token grant request"
// @Success 200 {object} auth.TokenResponse
// @Failure 400 {object} errors.ErrorResponse "Validation error / unsupported grant_type"
// @Failure 401 {object} errors.ErrorResponse "Invalid client credentials or refresh token"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Router /api/v1/oauth/token [post]
func (handler *Handler) Token(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var request auth.TokenRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}

	switch request.GrantType {
	case "client_credentials":
		handler.tokenClientCredentials(w, r, request)
	case "refresh_token":
		handler.tokenRefresh(w, r, request)
	default:
		// validator already constrains grant_type, but be explicit.
		httputil.RespondValidationError(w, r, nil, reqID)
	}
}

func (handler *Handler) tokenClientCredentials(w http.ResponseWriter, r *http.Request, request auth.TokenRequest) {
	reqID := middleware.GetRequestID(r.Context())

	if request.ClientID == "" || request.ClientSecret == "" {
		httputil.Respond401(w, r, "client_id and client_secret are required for client_credentials", reqID)
		return
	}

	// Authenticate the client: the client_secret is the long-lived api-key JWT.
	claims, err := jwt.ValidateAPIKey(request.ClientSecret)
	if err != nil || claims.Subject != request.ClientID {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}

	key, err := handler.store.GetAPIKeyByJTI(r.Context(), request.ClientID)
	if err != nil || key == nil {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}
	if key.RevokedAt != nil || (key.ExpiresAt != nil && key.ExpiresAt.Before(timeNow())) {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}

	access, refresh, expiresIn, err := handler.service.MintAPITokenPair(
		r.Context(), key.JTI, key.Scopes, key.OrgID, int64(key.ID), r.UserAgent(), clientIP(r),
	)
	if err != nil {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, auth.TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
	})
}

func (handler *Handler) tokenRefresh(w http.ResponseWriter, r *http.Request, request auth.TokenRequest) {
	reqID := middleware.GetRequestID(r.Context())

	if request.RefreshToken == "" {
		httputil.Respond401(w, r, "refresh_token is required for refresh_token grant", reqID)
		return
	}

	resp, err := handler.service.RefreshAPIToken(r.Context(), request.RefreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		httputil.Respond401(w, r, "Invalid or expired refresh token", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, auth.TokenResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    resp.ExpiresIn,
	})
}

// storage import retained for the handler's GetAPIKeyByJTI call type.
var _ = storage.ErrAPIKeyNotFound
```

**Important wiring:** the handler now needs a `*storage.Storage`. The current `Handler` holds only `service`. Add a `store` field:

In `auth.go`, change the struct + constructor:

```go
type Handler struct {
	service authServicer
	store   *storage.Storage
}

func NewHandler(service *authservice.Service, store *storage.Storage) *Handler {
	return &Handler{service: service, store: store}
}
```

Update the call site in `internal/cmd/serve/router.go` where `authhandler.NewHandler(...)` is constructed to pass the existing `store`. Add `timeNow` helper or replace `timeNow()` with `time.Now()` (import `time`) in oauth.go. Remove the `var _ = storage.ErrAPIKeyNotFound` line once `store` is wired (it exists only to keep the import while drafting).

- [ ] **Step 5: Register the route**

In `auth.go` `RegisterRoutes`, add:

```go
	r.Post("/api/v1/oauth/token", handler.Token)
```

- [ ] **Step 6: Fix the test stub + run**

Extend the existing `stubAuthService` in `auth_test.go` with:

```go
func (s *stubAuthService) MintAPITokenPair(_ context.Context, _ string, _ []string, _ int, _ int64, _, _ string) (string, string, int, error) {
	return "", "", 0, nil
}
func (s *stubAuthService) RefreshAPIToken(_ context.Context, _, _, _ string) (*authservice.APITokenResponse, error) {
	return nil, nil
}
```

Update `oauth_test.go` to construct the handler the same way the other handler tests do (with the stub service + a nil/test store, since the two unit tests return before touching `store` or `service`). Run:

Run: `cd backend && go test ./internal/handlers/auth/ -run TestToken -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handlers/auth/oauth.go backend/internal/handlers/auth/auth.go \
        backend/internal/handlers/auth/auth_test.go backend/internal/handlers/auth/oauth_test.go \
        backend/internal/cmd/serve/router.go
git commit -m "feat(auth): POST /api/v1/oauth/token handler with client_credentials + refresh grants (TRA-846)"
```

---

## Task 6: OpenAPI spec regeneration + worked example

**Files:**
- Generated: `backend/docs/swagger.{json,yaml}`, `docs/api/openapi.public.{json,yaml}` (committed artifacts)

- [ ] **Step 1: Regenerate the spec**

Run: `just backend build` (runs swaggo + apispec; see baseline output)
Expected: regenerates and the partition step succeeds (the new op has exactly one of public/internal → `oauth,public`).

- [ ] **Step 2: Verify the endpoint is in the public spec**

Run: `grep -c "/api/v1/oauth/token" docs/api/openapi.public.json`
Expected: `1` (present in public spec). Also confirm it is NOT only internal: `grep "oauth/token" backend/internal/handlers/swaggerspec/openapi.internal.json` should be absent or, if both partition, ensure public has it.

- [ ] **Step 3: Confirm the worked example renders**

Open `docs/api/openapi.public.yaml`, find `/api/v1/oauth/token`, confirm the request body example shows `grant_type`, `client_id`, `client_secret` and the response shows `access_token`/`expires_in: 900`. Adjust the `example:` struct tags in `models/auth/auth.go` if any render poorly, then re-run Step 1.

- [ ] **Step 4: Commit**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml backend/docs/swagger.json backend/docs/swagger.yaml
git commit -m "docs(api): regenerate public spec with oauth/token endpoint (TRA-846)"
```

---

## Task 7: End-to-end handler integration test + final validation

**Files:**
- Test: `backend/internal/handlers/auth/oauth_integration_test.go`

- [ ] **Step 1: Write the client_credentials happy-path integration test**

`backend/internal/handlers/auth/oauth_integration_test.go`:

```go
//go:build integration
// +build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestOAuthToken_ClientCredentialsThenRefresh(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	var userID int
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u','oauth-e2e@example.com','x') RETURNING id`).Scan(&userID))
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	// The long-lived JWT is the client_secret. Mint one the same way key
	// creation does (no exp).
	clientSecret, err := jwt.GenerateAPIKey(key.JTI, key.OrgID, key.Scopes, nil)
	require.NoError(t, err)

	svc := authservice.NewService(pool, store, nil)
	h := authhandler.NewHandler(svc, store)
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	// client_credentials
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     key.JTI,
		"client_secret": clientSecret,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Bearer", resp["token_type"])
	assert.EqualValues(t, 900, resp["expires_in"])
	assert.NotEmpty(t, resp["access_token"])
	refresh, _ := resp["refresh_token"].(string)
	require.NotEmpty(t, refresh)

	// refresh_token
	body2, _ := json.Marshal(map[string]string{"grant_type": "refresh_token", "refresh_token": refresh})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
}

func TestOAuthToken_BadSecretIs401(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	var userID int
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u','oauth-bad@example.com','x') RETURNING id`).Scan(&userID))
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	svc := authservice.NewService(pool, store, nil)
	h := authhandler.NewHandler(svc, store)
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     key.JTI,
		"client_secret": "not-a-valid-jwt",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
```

- [ ] **Step 2: Run the integration tests**

Run: `cd backend && go test -tags integration ./internal/handlers/auth/ -run TestOAuthToken -v`
Expected: PASS.

- [ ] **Step 3: Full validation sweep**

Run: `just validate` (lint + test both workspaces) and `cd backend && go test -tags integration ./internal/storage/ ./internal/services/auth/ ./internal/handlers/auth/`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/auth/oauth_integration_test.go
git commit -m "test(auth): e2e oauth/token client_credentials + refresh integration (TRA-846)"
```

- [ ] **Step 5: Schemathesis (contract gate)**

Schemathesis runs against the preview deploy after the PR opens (per the contract-tests gate). No local action beyond ensuring the public spec includes the endpoint (Task 6). Note in the PR description that both grants should be exercised; the gate looks back at main's status.

---

## Self-Review Notes

- **Spec coverage:** client_credentials (T3,T5,T7) · refresh rotation + replay (T3) · CASCADE on key revoke (T2) · schemathesis (T6,T7-S5) · public OpenAPI + example (T4,T6). All acceptance criteria mapped.
- **Type consistency:** `MintAPITokenPair(ctx, jti string, scopes []string, orgID int, apiKeyID int64, userAgent, ip string)` and `RefreshAPIToken(ctx, secret, ua, ip) (*APITokenResponse, error)` used identically in service, interface, handler, and tests. `RefreshToken.UserID *int`, `.APIKeyID *int64`, `.TokenType string` consistent across storage + service.
- **Open verification points for the implementer:** (1) confirm `testutil.SetupTestDB` returns `(*storage.Storage, func())` and applies all migrations; (2) confirm the existing `stubAuthService` is the stub to extend; (3) confirm `authhandler.NewHandler` call site in `router.go` and thread `store`; (4) confirm `httputil.Respond401` signature `(w, r, detail, reqID)`.
