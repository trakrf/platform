# TRA-501: API key DELETE accepts UUID `jti` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow `DELETE /api/v1/orgs/{orgId}/api-keys/{key_id}` to accept either an integer surrogate `id` or a UUID `jti`, and include `jti` in the `POST /api/v1/orgs/{orgId}/api-keys` response body.

**Architecture:** Storage gains a `RevokeAPIKeyByJTI` function. Handler `RevokeAPIKey` uses `uuid.Parse` then `strconv.Atoi` to dispatch. Handler `CreateAPIKey` threads `key.JTI` into the response. OpenAPI is regenerated from updated swag annotations.

**Tech Stack:** Go 1.23, chi router, pgx/v5, testify, swag (swaggo) for OpenAPI generation, `github.com/google/uuid`.

**Spec:** `docs/superpowers/specs/2026-04-25-tra-501-api-key-delete-uuid-jti-design.md`

**Worktree:** `/home/mike/platform/.worktrees/tra-501-api-key-delete-uuid` on branch `miks2u/tra-501-api-key-delete-uuid-jti`. All commands run from the worktree root.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `backend/internal/storage/apikeys.go` | Modify | Add `RevokeAPIKeyByJTI` (org-scoped revoke by jti string) |
| `backend/internal/storage/apikeys_integration_test.go` | Modify | Storage tests for new function |
| `backend/internal/models/apikey/apikey.go` | Modify | Add `JTI` field to `APIKeyCreateResponse` |
| `backend/internal/handlers/orgs/api_keys.go` | Modify | Handler dispatch in `RevokeAPIKey`; thread `jti` in `CreateAPIKey`; update swag `@Param key_id` annotation |
| `backend/internal/handlers/orgs/api_keys_integration_test.go` | Modify | Handler integration tests for dispatch + create-returns-jti |
| `docs/api/openapi.public.json` | Regenerated | Output of `just backend api-spec` |
| `docs/api/openapi.public.yaml` | Regenerated | Output of `just backend api-spec` |

The `internal/handlers/swaggerspec/openapi.public.{json,yaml}` files are gitignored copies — they are written by `api-spec` but not committed.

---

## Task 1: Storage layer — `RevokeAPIKeyByJTI`

**Files:**
- Modify: `backend/internal/storage/apikeys.go`
- Test: `backend/internal/storage/apikeys_integration_test.go`

- [ ] **Step 1: Write the first failing test (happy path)**

Append to `backend/internal/storage/apikeys_integration_test.go`:

```go
func TestAPIKeyStorage_RevokeByJTI(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	ctx := context.Background()

	key, err := store.CreateAPIKey(ctx, orgID, "to-revoke",
		[]string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, key.JTI)

	err = store.RevokeAPIKeyByJTI(ctx, orgID, key.JTI)
	require.NoError(t, err)

	// Verify revoked: GetAPIKeyByJTI returns the row but RevokedAt is set.
	got, err := store.GetAPIKeyByJTI(ctx, key.JTI)
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test-integration -run TestAPIKeyStorage_RevokeByJTI ./internal/storage/...
```

Expected: compile error or test failure: `store.RevokeAPIKeyByJTI undefined`.

- [ ] **Step 3: Implement `RevokeAPIKeyByJTI`**

In `backend/internal/storage/apikeys.go`, append after the existing `RevokeAPIKey` function (around line 130):

```go
// RevokeAPIKeyByJTI marks a key revoked, looked up by its UUID jti.
// Returns ErrAPIKeyNotFound if the jti is not in the given org or the key is
// already revoked (no rows updated). Mirrors RevokeAPIKey's semantics so the
// handler dispatch is symmetric.
func (s *Storage) RevokeAPIKeyByJTI(ctx context.Context, orgID int, jti string) error {
	var revokedID int
	err := s.pool.QueryRow(ctx, `
        UPDATE trakrf.api_keys
        SET revoked_at = NOW()
        WHERE jti = $1 AND org_id = $2 AND revoked_at IS NULL
        RETURNING id
    `, jti, orgID).Scan(&revokedID)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return ErrAPIKeyNotFound
		}
		return fmt.Errorf("revoke api_key by jti: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test-integration -run TestAPIKeyStorage_RevokeByJTI ./internal/storage/...
```

Expected: PASS.

- [ ] **Step 5: Add cross-org test**

Append to `backend/internal/storage/apikeys_integration_test.go`:

```go
func TestAPIKeyStorage_RevokeByJTIReturnsNotFoundForCrossOrg(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	org1 := testutil.CreateTestAccount(t, pool)
	var org2 int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Org 2', 'org-2-jti', true) RETURNING id`,
	).Scan(&org2)
	require.NoError(t, err)

	userID := createTestUser(t, pool)
	ctx := context.Background()

	key, err := store.CreateAPIKey(ctx, org1, "org1-key", []string{"assets:read"},
		apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	err = store.RevokeAPIKeyByJTI(ctx, org2, key.JTI)
	assert.ErrorIs(t, err, storage.ErrAPIKeyNotFound)
}
```

- [ ] **Step 6: Add already-revoked test**

Append:

```go
func TestAPIKeyStorage_RevokeByJTIAlreadyRevoked(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	ctx := context.Background()

	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"},
		apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	require.NoError(t, store.RevokeAPIKeyByJTI(ctx, orgID, key.JTI))

	err = store.RevokeAPIKeyByJTI(ctx, orgID, key.JTI)
	assert.ErrorIs(t, err, storage.ErrAPIKeyNotFound)
}
```

- [ ] **Step 7: Run all three new tests**

```bash
just backend test-integration -run TestAPIKeyStorage_RevokeByJTI ./internal/storage/...
```

Expected: 3 tests PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/storage/apikeys.go backend/internal/storage/apikeys_integration_test.go
git commit -m "feat(tra-501): add RevokeAPIKeyByJTI storage function

Org-scoped revocation by UUID jti. Mirrors RevokeAPIKey semantics:
returns ErrAPIKeyNotFound on cross-org or already-revoked.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Model — add `JTI` to `APIKeyCreateResponse`

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go`

This task has no test of its own — it is exercised by Task 3's handler test. Keep this commit small and focused so a reader can see exactly which response field was added.

- [ ] **Step 1: Add `JTI` field**

In `backend/internal/models/apikey/apikey.go`, modify `APIKeyCreateResponse` (currently lines 40-47). Insert `JTI` between `ID` and `Name`:

```go
// APIKeyCreateResponse is returned ONCE from POST; Key is the full JWT.
type APIKeyCreateResponse struct {
	Key       string     `json:"key"`
	ID        int        `json:"id"`
	JTI       string     `json:"jti"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
```

- [ ] **Step 2: Verify the package still compiles**

```bash
just backend test ./internal/models/apikey/...
```

Expected: PASS (no tests in this package, but it must build).

- [ ] **Step 3: Commit**

```bash
git add backend/internal/models/apikey/apikey.go
git commit -m "feat(tra-501): add JTI to APIKeyCreateResponse

Surface the UUID jti in the create response so customers can revoke
by jti without a follow-up GET or JWT decode.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Handler — `CreateAPIKey` returns `jti`

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go`
- Test: `backend/internal/handlers/orgs/api_keys_integration_test.go`

- [ ] **Step 1: Write failing test**

Append to `backend/internal/handlers/orgs/api_keys_integration_test.go`:

```go
func TestCreateAPIKey_ResponseIncludesJTI(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-create-jti")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	body := map[string]any{
		"name":   "needs-jti",
		"scopes": []string{"assets:read"},
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var envelope struct {
		Data apikey.APIKeyCreateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	resp := envelope.Data

	require.NotEmpty(t, resp.JTI, "create response must include jti")

	// The UUID jti is encoded in the JWT's `sub` claim (see GenerateAPIKey
	// in backend/internal/util/jwt/apikey.go) — assert they match.
	claims, err := jwt.ValidateAPIKey(resp.Key)
	require.NoError(t, err)
	assert.Equal(t, claims.Subject, resp.JTI, "jti in response must match JWT sub claim")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test-integration -run TestCreateAPIKey_ResponseIncludesJTI ./internal/handlers/orgs/...
```

Expected: FAIL — `resp.JTI` will be the empty string because the handler does not populate it yet.

- [ ] **Step 3: Thread `key.JTI` into the response**

In `backend/internal/handlers/orgs/api_keys.go`, modify the `resp` literal in `CreateAPIKey` (currently lines 132-139). Add `JTI: key.JTI`:

```go
	resp := apikey.APIKeyCreateResponse{
		Key:       signed,
		ID:        key.ID,
		JTI:       key.JTI,
		Name:      key.Name,
		Scopes:    key.Scopes,
		CreatedAt: key.CreatedAt,
		ExpiresAt: key.ExpiresAt,
	}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test-integration -run TestCreateAPIKey_ResponseIncludesJTI ./internal/handlers/orgs/...
```

Expected: PASS.

- [ ] **Step 5: Run the full create test set to confirm no regressions**

```bash
just backend test-integration -run TestCreateAPIKey ./internal/handlers/orgs/...
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "feat(tra-501): include jti in CreateAPIKey response

Threads key.JTI into the response struct so the create flow returns
the UUID alongside the JWT and integer id.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Handler — `RevokeAPIKey` accepts UUID `jti`

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go`
- Test: `backend/internal/handlers/orgs/api_keys_integration_test.go`

- [ ] **Step 1: Write failing happy-path test (revoke by jti)**

Append to `backend/internal/handlers/orgs/api_keys_integration_test.go`:

```go
func TestRevokeAPIKey_ByJTI(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-revoke-jti")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, sessionToken := seedAdminUser(t, pool, orgID)

	key, err := store.CreateAPIKey(context.Background(), orgID, "to-revoke-jti",
		[]string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, key.JTI)

	r := newAdminRouter(t, store)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%s", orgID, key.JTI), nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// Second delete on same jti → 404
	req2 := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%s", orgID, key.JTI), nil)
	req2.Header.Set("Authorization", "Bearer "+sessionToken)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test-integration -run TestRevokeAPIKey_ByJTI$ ./internal/handlers/orgs/...
```

Expected: FAIL with status 400 and body `"Invalid key id"` — `strconv.Atoi` rejects the UUID.

- [ ] **Step 3: Add the dispatch in `RevokeAPIKey`**

In `backend/internal/handlers/orgs/api_keys.go`:

3a. Add the import. The `import` block currently lacks `github.com/google/uuid`. Insert it alphabetically (right after `github.com/go-chi/chi/v5`):

```go
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/trakrf/platform/backend/internal/middleware"
```

3b. Replace the integer-only parse + revoke block in `RevokeAPIKey` (currently lines 216-232). Find this block:

```go
	keyID, err := strconv.Atoi(chi.URLParam(r, "key_id"))
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
```

Replace with:

```go
	rawKeyID := chi.URLParam(r, "key_id")
	var revokeErr error
	if jti, parseErr := uuid.Parse(rawKeyID); parseErr == nil {
		revokeErr = h.storage.RevokeAPIKeyByJTI(r.Context(), orgID, jti.String())
	} else if intID, parseErr := strconv.Atoi(rawKeyID); parseErr == nil {
		revokeErr = h.storage.RevokeAPIKey(r.Context(), orgID, intID)
	} else {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid key id", "", reqID)
		return
	}

	if revokeErr != nil {
		if stderrors.Is(revokeErr, storage.ErrAPIKeyNotFound) {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				"Not found", "API key not found", reqID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to revoke api key", "", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
```

3c. Update the swag annotation for the path param. Find the line above the function:

```go
// @Param key_id path int true "API key id"
```

Replace with:

```go
// @Param key_id path string true "Either the integer surrogate id or the UUID jti"
```

- [ ] **Step 4: Run the happy-path test to verify it passes**

```bash
just backend test-integration -run TestRevokeAPIKey_ByJTI$ ./internal/handlers/orgs/...
```

Expected: PASS.

- [ ] **Step 5: Add cross-org test**

Append to the test file:

```go
func TestRevokeAPIKey_ByJTI_CrossOrgReturns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-revoke-jti-cross-org")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	org1 := testutil.CreateTestAccount(t, pool)
	var org2 int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Org 2', 'org-2-jti-handler', true) RETURNING id`,
	).Scan(&org2)
	require.NoError(t, err)

	userID, _ := seedAdminUser(t, pool, org1)
	_, sessionToken2 := seedAdminUser2(t, pool, org2)

	// Create the target key in org1.
	key, err := store.CreateAPIKey(context.Background(), org1, "org1-target",
		[]string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	r := newAdminRouter(t, store)

	// org2 admin tries to revoke an org1 key by jti.
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%s", org2, key.JTI), nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken2)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// seedAdminUser2 mirrors seedAdminUser but with a distinct email so two admins
// can coexist in the same test database.
func seedAdminUser2(t *testing.T, pool *pgxpool.Pool, orgID int) (int, string) {
	t.Helper()
	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ('admin2', 'admin2@example.com', 'stub') RETURNING id`,
	).Scan(&userID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, 'admin')`, orgID, userID)
	require.NoError(t, err)

	token, err := jwt.Generate(userID, "admin2@example.com", &orgID)
	require.NoError(t, err)
	return userID, token
}
```

If `seedAdminUser2` (or an equivalent helper for a second admin) already exists in the file, **do not redefine it** — reuse what's there. Run `grep -n 'func seedAdminUser2' backend/internal/handlers/orgs/api_keys_integration_test.go` first; only add the helper if grep finds nothing.

- [ ] **Step 6: Add invalid-format test**

Append:

```go
func TestRevokeAPIKey_InvalidFormat(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-revoke-bad-format")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	cases := []string{"foo", "12-not-a-uuid", "abc123", "123.456"}
	for _, badID := range cases {
		t.Run(badID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete,
				fmt.Sprintf("/api/v1/orgs/%d/api-keys/%s", orgID, badID), nil)
			req.Header.Set("Authorization", "Bearer "+sessionToken)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "Invalid key id")
		})
	}
}
```

- [ ] **Step 7: Add self-revoke-by-jti test**

Append:

```go
// Mirror of TestRevokeAPIKey_KeyRevokesItself but using the JWT's jti instead
// of the integer id. The middleware authenticates BEFORE the handler runs, so
// this should behave identically.
func TestRevokeAPIKey_KeyRevokesItself_ByJTI(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-self-revoke-jti")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, _ := seedAdminUser(t, pool, orgID)
	adminKeyJWT, _ := mintKeysAdminAPIKey(t, store, orgID, userID)

	// Pull the jti out of the JWT we just minted. The UUID is in `sub`.
	claims, err := jwt.ValidateAPIKey(adminKeyJWT)
	require.NoError(t, err)
	require.NotEmpty(t, claims.Subject)

	r := newAdminRouter(t, store)

	// 1. Revoke self by jti — should succeed.
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys/%s", orgID, claims.Subject), nil)
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

Note: `claims.Subject` is intentional — `GenerateAPIKey` encodes the UUID jti in the JWT `sub` claim, not the `jti` claim. See `backend/internal/util/jwt/apikey.go:27`.

- [ ] **Step 8: Run all four new revoke tests**

```bash
just backend test-integration -run 'TestRevokeAPIKey_ByJTI|TestRevokeAPIKey_InvalidFormat|TestRevokeAPIKey_KeyRevokesItself_ByJTI' ./internal/handlers/orgs/...
```

Expected: all PASS.

- [ ] **Step 9: Run the full revoke regression set**

```bash
just backend test-integration -run TestRevokeAPIKey ./internal/handlers/orgs/...
```

Expected: every revoke test (existing + new) PASSES. The existing integer-id tests must still pass — that is the backward-compatibility check.

- [ ] **Step 10: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "feat(tra-501): DELETE accepts UUID jti or integer id

RevokeAPIKey dispatches by format: uuid.Parse first, then
strconv.Atoi, else 400 'Invalid key id'. Backward compatible —
the integer form keeps working unchanged. Swag annotation for
key_id is now string with description noting both forms.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Regenerate the OpenAPI spec

**Files:**
- Regenerated: `docs/api/openapi.public.json`
- Regenerated: `docs/api/openapi.public.yaml`

The committed spec is generated from swag annotations. Since Tasks 2 + 4 changed the `JTI` struct field and the `@Param key_id` annotation, the generated spec must be regenerated and committed alongside the code change.

- [ ] **Step 1: Regenerate the spec**

```bash
just backend api-spec
```

Expected output ends with `✅ Public spec: docs/api/openapi.public.{json,yaml} (committed) + swaggerspec/ (gitignored, embedded)`.

- [ ] **Step 2: Inspect the diff**

```bash
git diff -- docs/api/openapi.public.yaml
```

Expected diff covers exactly two areas:

1. The DELETE `key_id` path parameter: `type: integer` → `type: string`, with the new description.
2. The `apikey.APIKeyCreateResponse` schema: a new `jti` property of type `string`.

If unrelated churn appears (line counts in the hundreds, unrelated schemas changing), stop and investigate — swag's behavior depends on local cache state (see TRA-505 note in `backend/justfile`). Resolve the noise before committing.

- [ ] **Step 3: Lint the spec**

```bash
just backend api-lint
```

Expected: no new errors. (Pre-existing warnings on unrelated paths are OK; do not fix them in this PR.)

- [ ] **Step 4: Commit**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "docs(tra-501): regenerate OpenAPI spec

Reflects the new key_id path param type (string with both forms)
and the jti field on APIKeyCreateResponse.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Validation pass and Linear follow-up note

- [ ] **Step 1: Run the full backend integration suite**

```bash
just backend test-integration ./internal/handlers/orgs/... ./internal/storage/...
```

Expected: every test PASSES. Confirms no regressions across the api-keys surface.

- [ ] **Step 2: Run lint**

```bash
just backend lint
```

Expected: clean.

- [ ] **Step 3: Confirm the worktree is clean and pushable**

```bash
git status
git log --oneline main..HEAD
```

Expected: `git status` reports a clean working tree. The log shows the spec commit plus 5 implementation commits (one per task).

- [ ] **Step 4: Add a Linear comment for the trakrf-docs follow-up**

After this branch's PR opens (or merges), add a comment to TRA-501 noting the docs follow-up. Use the `mcp__linear-server__save_comment` tool with the issue id `TRA-501`. Comment body:

```
Docs follow-up: open a separate PR in trakrf/docs (working in /home/mike/trakrf-docs or a sibling worktree) adding a "Key identifiers — id vs jti" explainer under the API key revocation section. Open it AFTER this PR merges and deploys to prod, per the "ship docs behind backend reality" rule.
```

This is the only piece of the ticket explicitly deferred.

---

## Out of scope for this plan

- Customer-facing trakrf-docs site update (separate PR, gated on this merging + deploying).
- Wider deprecation of the integer surrogate `id` on public surfaces (post-v1).
- Changing surrogate `id` → UUID across the board.
