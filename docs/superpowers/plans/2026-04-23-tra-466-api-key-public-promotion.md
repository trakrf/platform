# TRA-466: Promote API Key Management Endpoints to Public API Surface — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Promote `POST/GET/DELETE /api/v1/orgs/{id}/api-keys` from internal to public API, gated by a new `keys:admin` scope, with full provenance tracking of key-minted-by-key creation and matching SPA support.

**Architecture:** Add `keys:admin` to `ValidScopes`; make `api_keys.created_by` nullable and add `created_by_key_id` with a `CHECK` constraint enforcing exactly-one-non-null; introduce `RequireOrgAdminOrKeysAdmin` middleware that accepts session-admin OR api-key-with-keys:admin; swap the three api-keys routes to use it; flip swaggo `@Tags` to public; extend SPA `ScopeSelector` with a "Key management" row. Docs ship in a separate trakrf-docs PR against merged-main artifacts.

**Tech Stack:** Go (chi, pgx/v5), PostgreSQL migrations (golang-migrate), swaggo, testify, React/TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-04-23-tra-466-api-key-public-promotion-design.md`

---

## Phase 1 — Schema & model foundation

### Task 1: Add `keys:admin` to ValidScopes

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go:5-13`

- [ ] **Step 1: Edit ValidScopes map**

```go
// ValidScopes is the canonical set of scope strings accepted by the public API.
var ValidScopes = map[string]bool{
	"assets:read":     true,
	"assets:write":    true,
	"locations:read":  true,
	"locations:write": true,
	"scans:read":      true,
	"scans:write":     true,
	"keys:admin":      true,
}
```

- [ ] **Step 2: Build**

Run: `just backend build`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add backend/internal/models/apikey/apikey.go
git commit -m "feat(tra-466): add keys:admin to ValidScopes"
```

---

### Task 2: Migration — nullable `created_by` + new `created_by_key_id`

**Files:**
- Create: `backend/migrations/000029_api_keys_created_by_nullable.up.sql`
- Create: `backend/migrations/000029_api_keys_created_by_nullable.down.sql`

- [ ] **Step 1: Write up migration**

```sql
SET search_path=trakrf,public;

ALTER TABLE api_keys ALTER COLUMN created_by DROP NOT NULL;

ALTER TABLE api_keys
    ADD COLUMN created_by_key_id INT REFERENCES api_keys(id);

ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_creator_exactly_one
    CHECK ((created_by IS NOT NULL) <> (created_by_key_id IS NOT NULL));

COMMENT ON COLUMN api_keys.created_by IS
    'User who minted this key via session auth. Mutually exclusive with created_by_key_id.';
COMMENT ON COLUMN api_keys.created_by_key_id IS
    'Parent API key that minted this key via keys:admin scope. Mutually exclusive with created_by.';

-- Refresh stale scope enumeration (existing comment predates scans:write and keys:admin).
COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, scans:read, scans:write, keys:admin';
```

- [ ] **Step 2: Write down migration**

```sql
SET search_path=trakrf,public;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM api_keys WHERE created_by IS NULL) THEN
    RAISE EXCEPTION 'cannot downgrade: % api_keys rows have NULL created_by',
      (SELECT COUNT(*) FROM api_keys WHERE created_by IS NULL);
  END IF;
END$$;

ALTER TABLE api_keys DROP CONSTRAINT api_keys_creator_exactly_one;
ALTER TABLE api_keys DROP COLUMN created_by_key_id;
ALTER TABLE api_keys ALTER COLUMN created_by SET NOT NULL;
```

- [ ] **Step 3: Apply + verify round-trip**

Run:
```bash
just backend migrate-up
just backend migrate-down
just backend migrate-up
```
Expected: all three succeed with no errors. Last command leaves schema at the new state.

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000029_api_keys_created_by_nullable.up.sql \
        backend/migrations/000029_api_keys_created_by_nullable.down.sql
git commit -m "feat(tra-466): migration — nullable created_by + created_by_key_id"
```

---

### Task 3: Update `apikey.APIKey` struct + add `Creator` type

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go:15-27`

- [ ] **Step 1: Update APIKey struct — make CreatedBy a pointer, add CreatedByKeyID**

```go
// APIKey is the row as stored. Full JWT is NOT stored — only the jti for revocation.
// Exactly one of CreatedBy / CreatedByKeyID is non-nil (DB CHECK enforced).
type APIKey struct {
	ID             int        `json:"id"`
	JTI            string     `json:"jti"`
	OrgID          int        `json:"org_id"`
	Name           string     `json:"name"`
	Scopes         []string   `json:"scopes"`
	CreatedBy      *int       `json:"created_by"`
	CreatedByKeyID *int       `json:"created_by_key_id"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}
```

- [ ] **Step 2: Add Creator struct**

Append near the bottom of the file, after `ActiveKeyCap`:

```go
// Creator identifies who minted an API key. Exactly one field must be non-nil.
// UserID populated when a session admin created the key; KeyID populated when a
// parent API key with keys:admin scope created the key.
type Creator struct {
	UserID *int
	KeyID  *int
}
```

- [ ] **Step 3: Update `APIKeyListItem` to expose creator fields**

Modify `APIKeyListItem` at lines 47-55:

```go
// APIKeyListItem is what GET returns — never includes the JWT.
type APIKeyListItem struct {
	ID             int        `json:"id"`
	JTI            string     `json:"jti"`
	Name           string     `json:"name"`
	Scopes         []string   `json:"scopes"`
	CreatedBy      *int       `json:"created_by"`
	CreatedByKeyID *int       `json:"created_by_key_id"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}
```

- [ ] **Step 4: Build — will fail in storage and handlers**

Run: `just backend build`
Expected: compile errors in `backend/internal/storage/apikeys.go` (scan into `&k.CreatedBy` — type mismatch `*int` vs `int`) and possibly handlers. These are fixed in Task 4.

Do NOT commit yet — this is a broken intermediate state. Continue to Task 4.

---

### Task 4: Update storage layer for new creator shape

**Files:**
- Modify: `backend/internal/storage/apikeys.go` (all of it)

- [ ] **Step 1: Replace `CreateAPIKey` signature + body**

Replace the function at lines 16-39:

```go
// CreateAPIKey inserts a new active key and returns it (populated id + jti).
// creator must have exactly one non-nil field (enforced at call site AND by DB CHECK).
func (s *Storage) CreateAPIKey(
	ctx context.Context,
	orgID int,
	name string,
	scopes []string,
	creator apikey.Creator,
	expiresAt *time.Time,
) (*apikey.APIKey, error) {
	if (creator.UserID == nil) == (creator.KeyID == nil) {
		return nil, fmt.Errorf("creator must have exactly one of UserID/KeyID set")
	}
	var k apikey.APIKey
	err := s.pool.QueryRow(ctx, `
        INSERT INTO trakrf.api_keys
            (org_id, name, scopes, created_by, created_by_key_id, expires_at)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, jti, org_id, name, scopes, created_by, created_by_key_id,
                  created_at, expires_at, last_used_at, revoked_at
    `, orgID, name, scopes, creator.UserID, creator.KeyID, expiresAt).Scan(
		&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
		&k.CreatedBy, &k.CreatedByKeyID,
		&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert api_keys: %w", err)
	}
	return &k, nil
}
```

- [ ] **Step 2: Update `ListActiveAPIKeys` to select + scan both columns**

Replace the function at lines 41-66:

```go
// ListActiveAPIKeys returns non-revoked keys for the given org, newest first.
func (s *Storage) ListActiveAPIKeys(ctx context.Context, orgID int) ([]apikey.APIKey, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_by_key_id,
               created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE org_id = $1 AND revoked_at IS NULL
        ORDER BY created_at DESC
    `, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys: %w", err)
	}
	defer rows.Close()

	out := []apikey.APIKey{}
	for rows.Next() {
		var k apikey.APIKey
		if err := rows.Scan(
			&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
			&k.CreatedBy, &k.CreatedByKeyID,
			&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api_key row: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Update `GetAPIKeyByJTI` to select + scan both columns**

Replace the function at lines 81-101:

```go
// GetAPIKeyByJTI fetches a key by its jti. The middleware uses this BEFORE
// org context exists (it must discover the org from the returned row).
// Returns ErrAPIKeyNotFound on no match.
func (s *Storage) GetAPIKeyByJTI(ctx context.Context, jti string) (*apikey.APIKey, error) {
	var k apikey.APIKey
	err := s.pool.QueryRow(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_by_key_id,
               created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE jti = $1
    `, jti).Scan(
		&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
		&k.CreatedBy, &k.CreatedByKeyID,
		&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
	)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("get api_key by jti: %w", err)
	}
	return &k, nil
}
```

- [ ] **Step 4: Build — will still fail in handlers + tests**

Run: `just backend build`
Expected: compile errors remaining only in the handler call site and integration tests. Fixed next tasks.

---

### Task 5: Update call sites — `CreateAPIKey` handler + all integration tests

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go:84`
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go:146-150, 183-184, 211-212, 262-263`

- [ ] **Step 1: Update handler creator resolution**

Replace the `CreateAPIKey` call in `api_keys.go` at line 84 (the `claims.UserID` invocation). The full section from line 83 onward:

```go
	key, err := h.storage.CreateAPIKey(r.Context(), orgID, req.Name, req.Scopes,
		apikey.Creator{UserID: &claims.UserID}, req.ExpiresAt)
```

This is a minimal change for this task. The api-key-principal creator path is added in Task 7 (handler rewiring). Session-only behavior is preserved here.

- [ ] **Step 2: Update integration test helper calls**

In `backend/internal/handlers/orgs/api_keys_integration_test.go`, search and replace all `store.CreateAPIKey` calls. Each currently passes `userID` (an `int`); replace with `apikey.Creator{UserID: &userID}`.

The four call sites are:
- Line 146-147: `store.CreateAPIKey(context.Background(), orgID, "active", []string{"assets:read"}, userID, nil)` → `..., apikey.Creator{UserID: &userID}, nil)`
- Line 149-150: same pattern, replace `userID` with `apikey.Creator{UserID: &userID}`
- Line 183-184: same pattern
- Line 211-212: same pattern
- Line 262-263: `creatorID` (not `userID`) — replace with `apikey.Creator{UserID: &creatorID}`

Make them line up cleanly; the helper already imports `"github.com/trakrf/platform/backend/internal/models/apikey"`.

- [ ] **Step 3: Update storage integration tests**

```bash
grep -rn "store.CreateAPIKey\|storage.CreateAPIKey\|\.CreateAPIKey(" backend/internal/storage/
```

For each file found with a call site, replace the `createdBy int` argument with `apikey.Creator{UserID: &userID}`. The known file is `backend/internal/storage/apikeys_integration_test.go` — update all call sites there the same way.

- [ ] **Step 4: Update middleware test call sites**

```bash
grep -rn "CreateAPIKey(" backend/internal/middleware/
```

Same substitution.

- [ ] **Step 5: Full build**

Run: `just backend build`
Expected: clean build.

- [ ] **Step 6: Run existing integration tests to confirm no regressions**

Run: `just backend test-integration ./internal/handlers/orgs/... ./internal/storage/... ./internal/middleware/...`
Expected: all existing tests pass (they were exercising session-admin flow, which is unchanged).

NOTE: If `api_keys_integration_test.go` tests fail because routes 404, that's because Task 7 will move api-keys routes to a new method. That's fixed in Task 7 Step 5, and `newAdminRouter` is updated in Task 7 Step 7 below. For now, `just backend build` should succeed; integration test pass is re-verified at the end of Task 7.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/models/apikey/apikey.go \
        backend/internal/storage/apikeys.go \
        backend/internal/handlers/orgs/api_keys.go \
        backend/internal/handlers/orgs/api_keys_integration_test.go \
        backend/internal/storage/apikeys_integration_test.go \
        backend/internal/middleware/apikey_test.go
git commit -m "refactor(tra-466): creator struct, nullable created_by + created_by_key_id columns

Storage/handler/tests updated to pass apikey.Creator{UserID, KeyID} instead
of a raw int. Existing behavior preserved — all new rows carry UserID only."
```

---

## Phase 2 — Middleware + handler wiring

### Task 6: Write `RequireOrgAdminOrKeysAdmin` middleware (TDD)

**Files:**
- Create: `backend/internal/middleware/org_admin_or_keys_admin.go`
- Create: `backend/internal/middleware/org_admin_or_keys_admin_test.go`

- [ ] **Step 1: Write failing test file**

```go
//go:build integration
// +build integration

package middleware_test

import (
	"context"
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
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// okHandler writes 200; used to confirm the request passed the middleware.
func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func seedUserWithRole(t *testing.T, pool *pgxpool.Pool, orgID int, role, email string) (int, string) {
	t.Helper()
	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ($1, $2, 'stub') RETURNING id`,
		email, email,
	).Scan(&userID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, $3)`, orgID, userID, role)
	require.NoError(t, err)
	token, err := jwt.Generate(userID, email, &orgID)
	require.NoError(t, err)
	return userID, token
}

// mintAPIKeyJWT creates a DB row + signed JWT for the given org/scopes.
func mintAPIKeyJWT(t *testing.T, store *testutil.Store, orgID int, scopes []string) string {
	t.Helper()
	var seederID int
	err := store.Pool().(*pgxpool.Pool).QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('seed-`+fmt.Sprint(orgID)+`', 'seed-`+fmt.Sprint(orgID)+`@ex', 'stub') RETURNING id`,
	).Scan(&seederID)
	require.NoError(t, err)
	key, err := store.CreateAPIKey(context.Background(), orgID, "t", scopes,
		apikey.Creator{UserID: &seederID}, nil)
	require.NoError(t, err)
	signed, err := jwt.GenerateAPIKey(key.JTI, orgID, scopes, nil)
	require.NoError(t, err)
	return signed
}

func newRouter(store *testutil.Store) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Route("/orgs/{id}/api-keys", func(r chi.Router) {
		r.Use(middleware.EitherAuth(store.Inner()))
		r.Use(middleware.RequireOrgAdminOrKeysAdmin(store.Inner()))
		r.Get("/", okHandler)
		r.Post("/", okHandler)
	})
	return r
}

func TestRequireOrgAdminOrKeysAdmin_SessionAdmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDBWrapped(t)
	defer cleanup()
	orgID := testutil.CreateTestAccount(t, store.Pool().(*pgxpool.Pool))
	_, token := seedUserWithRole(t, store.Pool().(*pgxpool.Pool), orgID, "admin", "admin@x")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRequireOrgAdminOrKeysAdmin_SessionMemberForbidden(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDBWrapped(t)
	defer cleanup()
	orgID := testutil.CreateTestAccount(t, store.Pool().(*pgxpool.Pool))
	_, token := seedUserWithRole(t, store.Pool().(*pgxpool.Pool), orgID, "operator", "op@x")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireOrgAdminOrKeysAdmin_APIKeyWithKeysAdmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDBWrapped(t)
	defer cleanup()
	orgID := testutil.CreateTestAccount(t, store.Pool().(*pgxpool.Pool))
	token := mintAPIKeyJWT(t, store, orgID, []string{"keys:admin"})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRequireOrgAdminOrKeysAdmin_APIKeyWithoutKeysAdmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDBWrapped(t)
	defer cleanup()
	orgID := testutil.CreateTestAccount(t, store.Pool().(*pgxpool.Pool))
	token := mintAPIKeyJWT(t, store, orgID, []string{"assets:read"})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireOrgAdminOrKeysAdmin_APIKeyWrongOrg(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-combined")
	store, cleanup := testutil.SetupTestDBWrapped(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	org1 := testutil.CreateTestAccount(t, pool)
	var org2 int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('O2', 'o2', true) RETURNING id`,
	).Scan(&org2))
	token := mintAPIKeyJWT(t, store, org2, []string{"keys:admin"}) // bound to org2

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orgs/%d/api-keys/", org1), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	newRouter(store).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
```

Note: this test uses `testutil.SetupTestDBWrapped` returning a `*testutil.Store` that exposes `.Inner() *storage.Storage` for middleware consumption. If that wrapper doesn't exist (grep `testutil.SetupTestDBWrapped`), adapt to whatever pattern `testutil.SetupTestDB` already returns — the test just needs a live `*storage.Storage`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `just backend test-integration ./internal/middleware/... -run RequireOrgAdminOrKeysAdmin`
Expected: FAIL with "undefined: middleware.RequireOrgAdminOrKeysAdmin"

- [ ] **Step 3: Implement the middleware**

Create `backend/internal/middleware/org_admin_or_keys_admin.go`:

```go
package middleware

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// RequireOrgAdminOrKeysAdmin accepts either a session admin of the target org
// OR an API-key principal with the "keys:admin" scope whose principal.OrgID
// matches {id} in the URL. Must be chained AFTER EitherAuth.
func RequireOrgAdminOrKeysAdmin(store OrgRoleStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			// 1. Session principal → delegate to existing org-admin check.
			if GetUserClaims(r) != nil {
				RequireOrgAdmin(store)(next).ServeHTTP(w, r)
				return
			}

			// 2. API-key principal → require keys:admin + matching org.
			if p := GetAPIKeyPrincipal(r); p != nil {
				orgIDStr := chi.URLParam(r, "id")
				orgID, err := strconv.Atoi(orgIDStr)
				if err != nil {
					httputil.WriteJSONError(w, r, http.StatusBadRequest,
						errors.ErrBadRequest, "Invalid org id", "", reqID)
					return
				}
				if p.OrgID != orgID {
					httputil.WriteJSONError(w, r, http.StatusForbidden,
						errors.ErrForbidden, "Forbidden",
						"API key is not authorized for this organization", reqID)
					return
				}
				for _, s := range p.Scopes {
					if s == "keys:admin" {
						next.ServeHTTP(w, r)
						return
					}
				}
				httputil.WriteJSONError(w, r, http.StatusForbidden,
					errors.ErrForbidden, "Forbidden",
					"Missing required scope: keys:admin", reqID)
				return
			}

			// 3. No principal (defensive — EitherAuth should have rejected).
			httputil.Respond401(w, r, "Authorization required", reqID)
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `just backend test-integration ./internal/middleware/... -run RequireOrgAdminOrKeysAdmin`
Expected: all 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/middleware/org_admin_or_keys_admin.go \
        backend/internal/middleware/org_admin_or_keys_admin_test.go
git commit -m "feat(tra-466): RequireOrgAdminOrKeysAdmin middleware

Accepts session admin OR api-key principal with keys:admin scope bound to
the target org. Delegates session path to existing RequireOrgAdmin."
```

---

### Task 7: Wire handlers to support api-key principal + swap router

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go:35-107`
- Modify: `backend/internal/handlers/orgs/orgs.go:283-285`

- [ ] **Step 1: Update `CreateAPIKey` handler to resolve creator from either principal**

Replace the principal check at lines 36-42 and the creator arg at line 84:

```go
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	// Resolve creator — exactly one of session-user or api-key-principal must be present.
	var creator apikey.Creator
	if claims := middleware.GetUserClaims(r); claims != nil {
		userID := claims.UserID
		creator = apikey.Creator{UserID: &userID}
	} else if p := middleware.GetAPIKeyPrincipal(r); p != nil {
		parent, err := h.storage.GetAPIKeyByJTI(r.Context(), p.JTI)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				"Failed to resolve parent key", "", reqID)
			return
		}
		parentID := parent.ID
		creator = apikey.Creator{KeyID: &parentID}
	} else {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", reqID)
		return
	}

	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	// ...remainder unchanged through the scope validation loop...
```

Then replace the `h.storage.CreateAPIKey(...)` call at line 84:

```go
	key, err := h.storage.CreateAPIKey(r.Context(), orgID, req.Name, req.Scopes,
		creator, req.ExpiresAt)
```

Delete the old `claims := middleware.GetUserClaims(r)` / `if claims == nil` block at the top (lines 37-42) — it's now covered by the resolver above. Do NOT reintroduce it; the resolver is the single source of auth.

- [ ] **Step 2: Update swaggo Description on CreateAPIKey**

Line 19 in `api_keys.go` currently says "Session-JWT-only — API-key tokens are rejected with 401." Replace with:

```go
// @Description Mints an API-key JWT scoped to the target org. Accepts either session-admin or an API key with the keys:admin scope.
```

- [ ] **Step 3: Flip @Tags from internal to public on all three handlers**

Change lines 20, 110, 155 from:

```go
// @Tags api-keys,internal
```

to:

```go
// @Tags api-keys,public
```

- [ ] **Step 4: Update `ListAPIKeys` handler to populate new list item fields**

Replace the struct literal at lines 141-149:

```go
		items = append(items, apikey.APIKeyListItem{
			ID:             k.ID,
			JTI:            k.JTI,
			Name:           k.Name,
			Scopes:         k.Scopes,
			CreatedBy:      k.CreatedBy,
			CreatedByKeyID: k.CreatedByKeyID,
			CreatedAt:      k.CreatedAt,
			ExpiresAt:      k.ExpiresAt,
			LastUsedAt:     k.LastUsedAt,
		})
```

- [ ] **Step 5: Carve api-keys routes out of `RegisterRoutes` into a new method**

Context: the orgs subtree in `router.go:89-93` runs under session-only `middleware.Auth`. Leaving the api-keys routes under `RegisterRoutes` means api-key JWTs never reach them (the upstream `Auth` middleware would 401). The fix is to move the three api-keys registrations into a sibling method the router can register under an `EitherAuth` group, without widening api-key acceptance to other org endpoints.

In `backend/internal/handlers/orgs/orgs.go`:

1. DELETE lines 282-285 (the three `api-keys` registrations plus their `// API keys (admin only)` comment).

2. ADD a new method `RegisterAPIKeyRoutes` below `RegisterRoutes`:

```go
// RegisterAPIKeyRoutes registers the /api/v1/orgs/{id}/api-keys endpoints.
// Registered SEPARATELY from RegisterRoutes because these routes accept api-key
// auth via keys:admin scope — they must live under an EitherAuth group, not
// the session-only middleware.Auth group used by the rest of the org subtree.
func (h *Handler) RegisterAPIKeyRoutes(r chi.Router, store middleware.OrgRoleStore) {
	r.Route("/api/v1/orgs/{id}/api-keys", func(r chi.Router) {
		r.Use(middleware.RequireOrgAdminOrKeysAdmin(store))
		r.Post("/", h.CreateAPIKey)
		r.Get("/", h.ListAPIKeys)
		r.Delete("/{keyID}", h.RevokeAPIKey)
	})
}
```

- [ ] **Step 6: Register api-keys routes under `EitherAuth` in `router.go`**

In `backend/internal/cmd/serve/router.go`, BELOW the `// Public API — API-key auth (TRA-393 canary)` block (around line 114) and BEFORE the `// TRA-396 public read surface` comment (around line 116), add a new group:

```go
	// TRA-466 API-key management — accepts session admin OR api-key with keys:admin scope.
	// Lives outside the session-only orgs subtree so api-key JWTs are accepted.
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.RateLimit(rl))
		r.Use(middleware.SentryContext)
		orgsHandler.RegisterAPIKeyRoutes(r, store)
	})
```

The `rl` variable is declared at line 111 (`rl := ratelimit.NewLimiter(...)`). This new group must appear AFTER `rl` is declared — placing it at ~line 115 satisfies that ordering.

- [ ] **Step 7: Update `newAdminRouter` test helper to match new route shape**

In `backend/internal/handlers/orgs/api_keys_integration_test.go`, replace the `newAdminRouter` function (lines 47-59) with:

```go
// newAdminRouter wires the api-keys routes under EitherAuth so tests can
// exercise both session and api-key auth paths. This mirrors the production
// layout in cmd/serve/router.go: api-keys routes live in their own EitherAuth
// group separate from the session-only org subtree.
func newAdminRouter(t *testing.T, store *storage.Storage) *chi.Mux {
	t.Helper()
	pool := store.Pool().(*pgxpool.Pool)
	service := orgsservice.NewService(pool, store, nil)
	handler := orgs.NewHandler(store, service)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		handler.RegisterAPIKeyRoutes(r, store)
	})
	return r
}
```

This single helper handles both auth paths — existing session-JWT tests pass through `EitherAuth` → `Auth` branch → `UserClaims` on ctx → `RequireOrgAdminOrKeysAdmin` delegates to session admin check. Same behavior as before for them. And the new api-key tests in Task 8 can reuse this same helper with no second variant needed.

- [ ] **Step 8: Build**

Run: `just backend build`
Expected: clean build.

- [ ] **Step 9: Run all previously-passing tests**

Run: `just backend test-integration ./internal/handlers/orgs/... ./internal/middleware/...`
Expected: all existing tests still pass. `TestCreateAPIKey_NonAdminForbidden` in particular must still 403 (operator role via session JWT → admin delegation fails).

- [ ] **Step 10: Regenerate OpenAPI specs**

Run: `just backend api-spec`
Expected: `docs/api/openapi.public.json` is rewritten; `grep -A2 '/api/v1/orgs/{id}/api-keys' docs/api/openapi.public.json` now returns results (previously absent when endpoints were tagged internal).

- [ ] **Step 11: Verify endpoints are in public spec**

Run:
```bash
jq '.paths | keys[] | select(test("api-keys"))' docs/api/openapi.public.json
```
Expected: three paths listed — `/api/v1/orgs/{id}/api-keys`, `/api/v1/orgs/{id}/api-keys/{keyID}`.

Also verify they are NOT still in the internal spec:
```bash
jq '.paths | keys[] | select(test("api-keys"))' backend/internal/handlers/swaggerspec/openapi.internal.json
```
Expected: empty (no api-keys paths).

- [ ] **Step 12: Also verify the missing path in the internal spec**

Run:
```bash
jq '.paths | keys[] | select(test("api-keys"))' backend/internal/handlers/swaggerspec/openapi.internal.json
```
Expected: empty (no api-keys paths left in internal).

- [ ] **Step 13: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go \
        backend/internal/handlers/orgs/orgs.go \
        backend/internal/handlers/orgs/api_keys_integration_test.go \
        backend/internal/cmd/serve/router.go \
        docs/api/openapi.public.json \
        docs/api/openapi.public.yaml \
        backend/internal/handlers/swaggerspec/openapi.public.json \
        backend/internal/handlers/swaggerspec/openapi.public.yaml \
        backend/internal/handlers/swaggerspec/openapi.internal.json \
        backend/internal/handlers/swaggerspec/openapi.internal.yaml \
        backend/docs/swagger.json \
        backend/docs/swagger.yaml \
        backend/docs/docs.go
git commit -m "feat(tra-466): promote api-keys endpoints to public; resolve creator from either principal"
```

(Adjust the file list — only commit what `git status` actually reports as changed.)

---

## Phase 3 — Integration test coverage for new auth paths

### Task 8: API-key-authenticated POST/GET/DELETE + self-rotation

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go` (append to end)

- [ ] **Step 1: Add helper `mintKeysAdminAPIKey`**

`newAdminRouter` was already updated in Task 7 Step 7 to use `EitherAuth` + `RegisterAPIKeyRoutes` — no new router helper needed. Only add the api-key JWT minter. Below `newAdminRouter`, append:

```go
// mintKeysAdminAPIKey inserts a DB row, returns the signed JWT and the row id.
func mintKeysAdminAPIKey(t *testing.T, store *storage.Storage, orgID, userID int) (string, int) {
	t.Helper()
	key, err := store.CreateAPIKey(context.Background(), orgID, "bootstrap admin",
		[]string{"keys:admin"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	signed, err := jwt.GenerateAPIKey(key.JTI, orgID, []string{"keys:admin"}, nil)
	require.NoError(t, err)
	return signed, key.ID
}
```

- [ ] **Step 2: Add failing test — api-key principal creates a data key**

Append to end of file:

```go
func TestCreateAPIKey_ByAPIKeyPrincipal(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-keys-admin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, _ := seedAdminUser(t, pool, orgID)
	adminKeyJWT, parentID := mintKeysAdminAPIKey(t, store, orgID, userID)

	r := newAdminRouter(t, store)

	body := map[string]any{
		"name":   "minted-by-key",
		"scopes": []string{"assets:read"},
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKeyJWT)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// The new row must have created_by NULL and created_by_key_id = parentID.
	var createdBy *int
	var createdByKeyID *int
	err := pool.QueryRow(context.Background(),
		`SELECT created_by, created_by_key_id FROM trakrf.api_keys WHERE name = 'minted-by-key'`,
	).Scan(&createdBy, &createdByKeyID)
	require.NoError(t, err)
	assert.Nil(t, createdBy, "session user must be NULL when minted by api-key")
	require.NotNil(t, createdByKeyID)
	assert.Equal(t, parentID, *createdByKeyID)
}
```

- [ ] **Step 3: Run — should pass (handler + middleware already implemented)**

Run: `just backend test-integration ./internal/handlers/orgs/... -run TestCreateAPIKey_ByAPIKeyPrincipal`
Expected: PASS

- [ ] **Step 4: Add self-rotation test**

```go
func TestCreateAPIKey_KeysAdminMintsKeysAdmin(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-keys-admin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, _ := seedAdminUser(t, pool, orgID)
	adminKeyJWT, _ := mintKeysAdminAPIKey(t, store, orgID, userID)

	r := newAdminRouter(t, store)

	body := map[string]any{
		"name":   "rotated-admin",
		"scopes": []string{"keys:admin"},
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKeyJWT)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var scopes []string
	err := pool.QueryRow(context.Background(),
		`SELECT scopes FROM trakrf.api_keys WHERE name = 'rotated-admin'`,
	).Scan(&scopes)
	require.NoError(t, err)
	assert.Contains(t, scopes, "keys:admin")
}
```

- [ ] **Step 5: Add list + revoke via api-key tests**

```go
func TestListAPIKeys_ByAPIKeyPrincipal(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-keys-admin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, _ := seedAdminUser(t, pool, orgID)
	adminKeyJWT, _ := mintKeysAdminAPIKey(t, store, orgID, userID)

	r := newAdminRouter(t, store)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), nil)
	req.Header.Set("Authorization", "Bearer "+adminKeyJWT)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Data []apikey.APIKeyListItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Data, 1, "should see the admin key itself in the list")
	// The admin key was minted by a user, so CreatedBy is non-nil, CreatedByKeyID is nil.
	assert.NotNil(t, out.Data[0].CreatedBy)
	assert.Nil(t, out.Data[0].CreatedByKeyID)
}

func TestRevokeAPIKey_ByAPIKeyPrincipal(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-keys-admin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, _ := seedAdminUser(t, pool, orgID)
	adminKeyJWT, _ := mintKeysAdminAPIKey(t, store, orgID, userID)

	// Create a separate data key to revoke.
	dataKey, err := store.CreateAPIKey(context.Background(), orgID, "target",
		[]string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	r := newAdminRouter(t, store)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%d", orgID, dataKey.ID), nil)
	req.Header.Set("Authorization", "Bearer "+adminKeyJWT)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

// A keys:admin key is allowed to revoke its own JTI; the subsequent request with
// that key should 401 because the token is now revoked.
func TestRevokeAPIKey_KeyRevokesItself(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-keys-admin")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, _ := seedAdminUser(t, pool, orgID)
	adminKeyJWT, adminKeyID := mintKeysAdminAPIKey(t, store, orgID, userID)

	r := newAdminRouter(t, store)

	// 1. Revoke self — should succeed.
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%d", orgID, adminKeyID), nil)
	req.Header.Set("Authorization", "Bearer "+adminKeyJWT)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// 2. Next call with the same (now-revoked) token — should 401.
	req2 := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), nil)
	req2.Header.Set("Authorization", "Bearer "+adminKeyJWT)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}
```

- [ ] **Step 6: Run all new tests**

Run: `just backend test-integration ./internal/handlers/orgs/... -run "TestCreateAPIKey_ByAPIKey|TestCreateAPIKey_KeysAdmin|TestListAPIKeys_ByAPIKey|TestRevokeAPIKey_ByAPIKey|TestRevokeAPIKey_KeyRevokes"`
Expected: all 5 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "test(tra-466): api-key-authenticated create/list/revoke + self-rotation"
```

---

### Task 9: Storage roundtrip + CHECK constraint test

**Files:**
- Modify: `backend/internal/storage/apikeys_integration_test.go` (append)

- [ ] **Step 1: Add roundtrip test**

Find the bottom of the test file (after existing tests) and append:

```go
func TestCreateAPIKey_WithCreatedByKeyID(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var seedUserID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash) VALUES ('seed', 'seed@x', 'stub') RETURNING id`,
	).Scan(&seedUserID))

	parent, err := store.CreateAPIKey(context.Background(), orgID, "parent",
		[]string{"keys:admin"}, apikey.Creator{UserID: &seedUserID}, nil)
	require.NoError(t, err)

	child, err := store.CreateAPIKey(context.Background(), orgID, "child",
		[]string{"assets:read"}, apikey.Creator{KeyID: &parent.ID}, nil)
	require.NoError(t, err)
	require.Nil(t, child.CreatedBy)
	require.NotNil(t, child.CreatedByKeyID)
	assert.Equal(t, parent.ID, *child.CreatedByKeyID)

	// Roundtrip via List — creator fields survive scan.
	list, err := store.ListActiveAPIKeys(context.Background(), orgID)
	require.NoError(t, err)
	var roundtripped *apikey.APIKey
	for i := range list {
		if list[i].ID == child.ID {
			roundtripped = &list[i]
			break
		}
	}
	require.NotNil(t, roundtripped)
	assert.Nil(t, roundtripped.CreatedBy)
	require.NotNil(t, roundtripped.CreatedByKeyID)
	assert.Equal(t, parent.ID, *roundtripped.CreatedByKeyID)
}

// Direct SQL insert with both creator columns must violate the CHECK constraint.
func TestAPIKeys_CreatorExactlyOneCheck(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash) VALUES ('u', 'u@x', 'stub') RETURNING id`,
	).Scan(&userID))

	parent, err := store.CreateAPIKey(context.Background(), orgID, "p",
		[]string{"keys:admin"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	// Bypass storage helper — raw INSERT with BOTH creator columns set → CHECK fails.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.api_keys (org_id, name, scopes, created_by, created_by_key_id)
		VALUES ($1, 'both', ARRAY['assets:read'], $2, $3)`,
		orgID, userID, parent.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_keys_creator_exactly_one")

	// And with NEITHER set → also CHECK fails.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.api_keys (org_id, name, scopes)
		VALUES ($1, 'neither', ARRAY['assets:read'])`, orgID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_keys_creator_exactly_one")
}
```

- [ ] **Step 2: Run**

Run: `just backend test-integration ./internal/storage/... -run "TestCreateAPIKey_WithCreatedByKeyID|TestAPIKeys_CreatorExactlyOneCheck"`
Expected: both PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/storage/apikeys_integration_test.go
git commit -m "test(tra-466): storage roundtrip + CHECK constraint for created_by_key_id"
```

---

## Phase 4 — SPA support

### Task 10: Extend Scope type + list response interface

**Files:**
- Modify: `frontend/src/types/apiKey.ts`

- [ ] **Step 1: Extend Scope union and ALL_SCOPES; add creator fields to APIKey**

Replace the file contents:

```ts
export type Scope =
  | 'assets:read'
  | 'assets:write'
  | 'locations:read'
  | 'locations:write'
  | 'scans:read'
  | 'scans:write'
  | 'keys:admin';

export interface APIKey {
  id: number;
  jti: string;
  name: string;
  scopes: Scope[];
  created_by: number | null;
  created_by_key_id: number | null;
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
  'scans:write',
  'keys:admin',
];

export const ACTIVE_KEY_CAP = 10;
```

- [ ] **Step 2: Typecheck — will fail if any existing code assumed Scope was narrower**

Run: `just frontend typecheck`
Expected: either clean, or narrow errors like "'scans:write' not assignable to Scope" in a test fixture. Fix each one by accepting the extended union (no inline suppressions).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types/apiKey.ts
git commit -m "feat(tra-466): extend Scope union with keys:admin (and scans:write); add creator fields to APIKey"
```

---

### Task 11: Extend `ScopeSelector` with Key management row

**Files:**
- Modify: `frontend/src/components/apikeys/ScopeSelector.tsx`
- Modify: `frontend/src/components/apikeys/ScopeSelector.test.tsx` (append)

- [ ] **Step 1: Write failing test**

Append to `ScopeSelector.test.tsx`:

```tsx
  it('renders Key management row with None/Admin options', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    const select = screen.getByLabelText(/key management/i);
    expect(within(select).getByRole('option', { name: /none/i })).toBeInTheDocument();
    expect(within(select).getByRole('option', { name: /admin/i })).toBeInTheDocument();
    // No "Read" or "Read + Write" on this row — it's binary.
    expect(within(select).queryByRole('option', { name: /^read$/i })).not.toBeInTheDocument();
  });

  it('emits keys:admin when Key management is set to Admin', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/key management/i), { target: { value: 'admin' } });
    expect(onChange).toHaveBeenCalledWith(['keys:admin']);
  });

  it('shows initial value correctly for keys:admin', () => {
    render(<ScopeSelector value={['keys:admin']} onChange={() => {}} />);
    expect(screen.getByLabelText(/key management/i)).toHaveValue('admin');
  });

  it('preserves data scopes when toggling key management', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={['assets:read']} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/key management/i), { target: { value: 'admin' } });
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining(['assets:read', 'keys:admin']));
  });
```

- [ ] **Step 2: Run tests — confirm failure**

Run: `just frontend test -- ScopeSelector`
Expected: FAIL — no "Key management" label present.

- [ ] **Step 3: Implement — extend `ScopeSelector.tsx`**

Replace the whole file:

```tsx
import type { Scope } from '@/types/apiKey';

type ResourceLevel = 'none' | 'read' | 'readwrite';
type AdminLevel = 'none' | 'admin';

interface Props {
  value: Scope[];
  onChange: (next: Scope[]) => void;
}

type ResourceKey = 'assets' | 'locations' | 'scans';

const RESOURCES: { key: ResourceKey; label: string; hasWrite: boolean }[] = [
  { key: 'assets',    label: 'Assets',    hasWrite: true },
  { key: 'locations', label: 'Locations', hasWrite: true },
  { key: 'scans',     label: 'Scans',     hasWrite: true },
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

function adminLevelFor(scopes: Scope[]): AdminLevel {
  return scopes.includes('keys:admin') ? 'admin' : 'none';
}

export function ScopeSelector({ value, onChange }: Props) {
  const setLevel = (resource: ResourceKey, level: ResourceLevel) => {
    const without = value.filter((s) => !s.startsWith(`${resource}:`));
    onChange([...without, ...scopesFor(resource, level)]);
  };

  const setAdmin = (level: AdminLevel) => {
    const without = value.filter((s) => s !== 'keys:admin');
    onChange(level === 'admin' ? [...without, 'keys:admin'] : without);
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
              className="w-32 text-sm text-gray-800 dark:text-gray-200"
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
      <div className="flex items-center gap-3">
        <label
          htmlFor="scope-keys"
          className="w-32 text-sm text-gray-800 dark:text-gray-200"
        >
          Key management
        </label>
        <select
          id="scope-keys"
          aria-label="Key management"
          value={adminLevelFor(value)}
          onChange={(e) => setAdmin(e.target.value as AdminLevel)}
          className="border rounded px-2 py-1 text-sm bg-white dark:bg-gray-800"
        >
          <option value="none">None</option>
          <option value="admin">Admin</option>
        </select>
      </div>
    </fieldset>
  );
}
```

(The `w-24` → `w-32` widens the label column enough to fit "Key management".)

- [ ] **Step 4: Run tests**

Run: `just frontend test -- ScopeSelector`
Expected: all ScopeSelector tests PASS, including the 4 new ones.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/apikeys/ScopeSelector.tsx \
        frontend/src/components/apikeys/ScopeSelector.test.tsx
git commit -m "feat(tra-466): ScopeSelector adds Key management row with keys:admin"
```

---

### Task 12: Update `APIKeysScreen` to render key-minted-key provenance

**Files:**
- Modify: `frontend/src/components/APIKeysScreen.tsx`
- Modify: `frontend/src/components/APIKeysScreen.test.tsx` (if exists; otherwise skip test additions but hand-verify in the browser)

- [ ] **Step 1: Inspect current APIKeysScreen**

Run: `head -100 frontend/src/components/APIKeysScreen.tsx`
Look for the render section that displays each list item. Identify where `created_by` would be rendered (if anywhere — it may not be displayed today since the old type didn't include it).

- [ ] **Step 2: Decide based on inspection**

- If the screen currently does NOT render creator info, skip UI changes beyond the type extension. TRA-466 is delivering the data over the wire; presenting it nicely is a separate polish ticket.
- If the screen DOES render creator info (e.g., resolves user names), ALSO handle `created_by_key_id`: when non-null, render "Key: {name_of_parent_key}" by looking up the parent in the same list response. When both are null (defensive; shouldn't happen), render "—".

If you choose to render, add to the row display something like:

```tsx
{key.created_by_key_id != null
  ? `Key: ${list.find((k) => k.id === key.created_by_key_id)?.name ?? '(unknown)'}`
  : key.created_by != null
    ? `User: ${resolveUserName(key.created_by)}` // existing pattern
    : '—'}
```

- [ ] **Step 3: Run full frontend tests**

Run: `just frontend test`
Expected: all tests pass. If any break because they asserted on specific list-row text, update assertions to match.

- [ ] **Step 4: Typecheck + lint**

Run: `just frontend typecheck && just frontend lint`
Expected: clean.

- [ ] **Step 5: Commit (only if changes were made)**

```bash
git add frontend/src/components/APIKeysScreen.tsx frontend/src/components/APIKeysScreen.test.tsx
git commit -m "feat(tra-466): APIKeysScreen renders creator provenance for key-minted keys"
```

(If no changes, skip this step.)

---

## Phase 5 — Final validation + push

### Task 13: Repo-wide validation

- [ ] **Step 1: Run full validation**

Run: `just validate`
Expected: backend + frontend all green (lint, typecheck, unit tests).

- [ ] **Step 2: Run full integration test suite**

Run: `just backend test-integration`
Expected: all integration tests pass, including the new ones from Tasks 6, 8, 9.

- [ ] **Step 3: Manual check — OpenAPI public spec has the new endpoints**

Run:
```bash
jq '.paths."/api/v1/orgs/{id}/api-keys"' docs/api/openapi.public.json
jq '.paths."/api/v1/orgs/{id}/api-keys/{keyID}"' docs/api/openapi.public.json
```
Expected: both return non-null objects with the correct HTTP methods (post+get, delete respectively).

- [ ] **Step 4: Manual check — internal spec does NOT still have them**

Run:
```bash
jq '.paths | with_entries(select(.key | test("api-keys"))) | keys' backend/internal/handlers/swaggerspec/openapi.internal.json
```
Expected: empty array `[]`.

---

### Task 14: Push branch, open PR 1

- [ ] **Step 1: Push branch**

Run: `git push -u origin miks2u/tra-466-promote-api-key-management-endpoints-to-public-api-surface`

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "feat(tra-466): promote api-key management endpoints to public API" --body "$(cat <<'EOF'
## Summary
- Add `keys:admin` scope; make `api_keys.created_by` nullable and add `created_by_key_id` with a CHECK constraint
- New `RequireOrgAdminOrKeysAdmin` middleware: accepts session admin OR api-key with `keys:admin` scope
- Flip `@Tags api-keys,internal` → `api-keys,public` on the three handlers; regenerate `openapi.public.{json,yaml}`
- SPA `ScopeSelector` gains a "Key management" row with None/Admin

Design: `docs/superpowers/specs/2026-04-23-tra-466-api-key-public-promotion-design.md`

## Test plan
- [ ] `just validate` passes
- [ ] `just backend test-integration` passes (new middleware, handler, storage tests)
- [ ] `just frontend test` passes (new ScopeSelector tests)
- [ ] Preview deploy: log in, open API Keys screen, create a key with "Key management: Admin", copy the JWT, call `curl -H "Authorization: Bearer $K" https://app.preview.trakrf.id/api/v1/orgs/{id}/api-keys` and get 200
- [ ] Same call with a non-admin key returns 403 with `"Missing required scope: keys:admin"`

## Follow-up
Docs PR against `trakrf/docs` comes after this merges, using the `openapi.public.{json,yaml}` from the merged-main build.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Wait for CI + preview deploy**

Check `gh pr checks` until green. Then verify the PR URL's preview deploy (see CLAUDE.md — preview at `https://app.preview.trakrf.id`).

- [ ] **Step 4: Hand off to user for review/merge**

Stop. Do not merge; user merges.

---

## Out of scope / deferred

- Docs PR in `trakrf/docs` — opened after this PR merges, from a separate checkout (worktree or sibling dir — not `/home/mike/trakrf-docs` main). Updates `docs/api/authentication.md` (programmatic rotation section, `keys:admin` scope row, session-JWT-on-public-endpoints note) and `docs/api/private-endpoints.md` (remove api-keys rows); replaces `static/api/openapi.{json,yaml}` with the merged-main artifacts.
- OpenAPI contract tests in CI — separate effort.
- Playwright E2E for UI-mint → API-rotate flow — manual pre-merge verification only.
