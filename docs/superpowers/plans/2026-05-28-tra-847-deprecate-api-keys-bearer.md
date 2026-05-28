# TRA-847 Deprecate api_keys-as-bearer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the long-lived api-key JWT (returned by `POST /orgs/{id}/api-keys` and used directly as a Bearer credential) with an opaque `{client_id, client_secret}` pair that is exchanged at `POST /oauth/token` for a short-lived access JWT.

**Architecture:** The key-create endpoint stops minting JWTs; it stores a SHA-256 hash of a fresh opaque secret and returns the secret once. The `client_credentials` grant verifies the presented secret against that hash instead of validating a JWT. Grant-minted short-lived access JWTs (issuer `trakrf-api-key`) remain the only Bearer credential and validate exactly as TRA-846 produces them. No backwards compatibility: pre-existing api_keys rows are deleted by the migration.

**Tech Stack:** Go, chi router, pgx/TimescaleDB, golang-jwt/v5, golang-migrate migrations, Spectral (OpenAPI lint), schemathesis (contract tests). Task runner: `just` (`just backend <cmd>` from repo root).

**Spec:** `docs/superpowers/specs/2026-05-28-tra-847-deprecate-api-keys-bearer-design.md`

---

### Task 1: `apisecret` package (opaque secret generate/hash/verify)

**Files:**
- Create: `backend/internal/util/apisecret/apisecret.go`
- Test: `backend/internal/util/apisecret/apisecret_test.go`

- [ ] **Step 1: Write the failing test**

```go
package apisecret

import (
	"strings"
	"testing"
)

func TestGenerateProducesPrefixedUniqueSecrets(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(a, "trakrf_") {
		t.Errorf("missing trakrf_ prefix: %q", a)
	}
	if a == b {
		t.Error("two Generate() calls returned identical secrets")
	}
	// trakrf_ (7) + 64 hex chars
	if len(a) != 7+64 {
		t.Errorf("unexpected length %d: %q", len(a), a)
	}
}

func TestHashIsStableAndVerifies(t *testing.T) {
	secret, _ := Generate()
	h := Hash(secret)
	if len(h) != 64 {
		t.Errorf("hash not 64 hex chars: %q", h)
	}
	if Hash(secret) != h {
		t.Error("Hash not deterministic")
	}
	if !Verify(secret, h) {
		t.Error("Verify rejected the correct secret")
	}
	if Verify(secret+"x", h) {
		t.Error("Verify accepted a wrong secret")
	}
	if Verify("", h) {
		t.Error("Verify accepted empty secret")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/util/apisecret/ -v`
Expected: FAIL — `undefined: Generate` / package has no Go files.

- [ ] **Step 3: Write minimal implementation**

```go
// Package apisecret generates and verifies the opaque client_secret returned
// by API-key creation. The secret is high-entropy random, so a single SHA-256
// is sufficient (matching the refresh_tokens.token_hash precedent); bcrypt is
// reserved for low-entropy human passwords.
package apisecret

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// secretBytes is the entropy of the opaque secret before hex-encoding.
const secretBytes = 32

// Generate returns a fresh opaque secret: "trakrf_" + 64 hex chars.
// The prefix aids secret scanning and log greppability.
func Generate() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api secret: %w", err)
	}
	return "trakrf_" + hex.EncodeToString(b), nil
}

// Hash returns the SHA-256 hex digest of the secret (64 chars).
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Verify reports whether presented hashes to storedHash, in constant time.
func Verify(presented, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(Hash(presented)), []byte(storedHash)) == 1
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/util/apisecret/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/apisecret/
git commit -m "feat(apisecret): opaque secret generate/hash/verify (TRA-847)"
```

---

### Task 2: Rename JWT funcs to access-token semantics (pure rename, no behavior change)

`GenerateAPIKey`/`ValidateAPIKey` only ever handle short-lived grant access tokens now; the names mislead. This task is a mechanical rename across all call sites so the tree still compiles and all existing tests pass. The require-exp invariant is added later (Task 8), after no-exp minting is removed.

**Files:**
- Modify: `backend/internal/util/jwt/apikey.go` (rename `GenerateAPIKey`→`GenerateAccessToken`, `ValidateAPIKey`→`ValidateAccessToken`)
- Modify: `backend/internal/util/jwt/apikey_test.go` (rename references)
- Modify: `backend/internal/middleware/apikey.go:55` (`jwt.ValidateAPIKey`→`jwt.ValidateAccessToken`)
- Modify: `backend/internal/handlers/auth/oauth.go:63` (`jwt.ValidateAPIKey`→`jwt.ValidateAccessToken`)
- Modify: `backend/internal/handlers/orgs/api_keys.go:131` (`jwt.GenerateAPIKey`→`jwt.GenerateAccessToken`)
- Modify: `backend/internal/services/auth/api_token.go:28,93` (`jwt.GenerateAPIKey`→`jwt.GenerateAccessToken`)
- Modify: `backend/internal/handlers/testhandler/apikeys.go:85` (`jwt.GenerateAPIKey`→`jwt.GenerateAccessToken`)

- [ ] **Step 1: Find every reference**

Run: `cd backend && grep -rn "GenerateAPIKey\|ValidateAPIKey" --include=*.go`
Expected: the call sites listed above (plus any test files). Note them all.

- [ ] **Step 2: Rename the definitions** in `backend/internal/util/jwt/apikey.go`

Change line 24 `func GenerateAPIKey(` → `func GenerateAccessToken(` and line 50 `func ValidateAPIKey(` → `func ValidateAccessToken(`. Update the doc comments to say "access token" rather than "api-key JWT" where it reads naturally.

- [ ] **Step 3: Rename every call site**

Apply the rename at each location from Step 1 (use the grep output as the checklist). Pure textual rename; no signature or behavior change.

- [ ] **Step 4: Build + run the suite**

Run: `cd backend && go build ./... && go test ./internal/util/jwt/ ./internal/middleware/ -v`
Expected: builds clean; jwt + middleware tests PASS unchanged.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/
git commit -m "refactor(jwt): rename api-key JWT funcs to access-token semantics (TRA-847)"
```

---

### Task 3: Migration `000013` — add `secret_hash`, delete stale rows

**Files:**
- Create: `backend/migrations/000013_api_key_secret_hash.up.sql`
- Create: `backend/migrations/000013_api_key_secret_hash.down.sql`

- [ ] **Step 1: Write the up-migration**

`backend/migrations/000013_api_key_secret_hash.up.sql`:

```sql
-- TRA-847: api keys now authenticate via an opaque client_secret (hashed),
-- not a long-lived JWT. Pre-existing rows have no recoverable secret and are
-- unusable under the new model, so they are removed (0 live prod keys per the
-- DB audit). Refresh tokens referencing them are removed first (FK).

DELETE FROM trakrf.refresh_tokens
WHERE api_key_id IS NOT NULL;

DELETE FROM trakrf.api_keys;

ALTER TABLE trakrf.api_keys
    ADD COLUMN secret_hash VARCHAR(64) NOT NULL;
```

- [ ] **Step 2: Write the down-migration**

`backend/migrations/000013_api_key_secret_hash.down.sql`:

```sql
ALTER TABLE trakrf.api_keys
    DROP COLUMN secret_hash;
```

- [ ] **Step 3: Verify the FK column name**

Run: `cd backend && grep -rn "api_key_id" migrations/000012_refresh_tokens_api_grant.up.sql`
Expected: confirms `refresh_tokens.api_key_id` exists (added by TRA-846). If the column name differs, adjust the up-migration's first DELETE accordingly.

- [ ] **Step 4: Apply migrations against a scratch/test DB**

Run: `just backend migrate-up` (or the project's migration command; check `backend/justfile` if unsure).
Expected: migration `000013` applies cleanly; `\d trakrf.api_keys` shows `secret_hash` NOT NULL.

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000013_api_key_secret_hash.up.sql backend/migrations/000013_api_key_secret_hash.down.sql
git commit -m "feat(db): add api_keys.secret_hash, drop stale rows (TRA-847)"
```

---

### Task 4: Model + storage carry `secret_hash`

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go` (add `SecretHash` to `APIKey`)
- Modify: `backend/internal/storage/apikeys.go` (`CreateAPIKey` takes hash; `GetAPIKeyByJTI`/`GetAPIKeyByID` select it)
- Test: `backend/internal/storage/apikeys_integration_test.go`

- [ ] **Step 1: Update the failing test first**

In `backend/internal/storage/apikeys_integration_test.go`, find `TestCreateAPIKey_WithCreatedByKeyID` (≈ line 138) and any other `CreateAPIKey(` call. Add a hash argument and assert round-trip. Example assertion to add after a create+fetch:

```go
const testHash = "0000000000000000000000000000000000000000000000000000000000000000"
// ... in the create call, pass testHash as the new secretHash arg ...
got, err := store.GetAPIKeyByJTI(ctx, key.JTI)
if err != nil {
	t.Fatalf("GetAPIKeyByJTI: %v", err)
}
if got.SecretHash != testHash {
	t.Errorf("SecretHash round-trip: got %q want %q", got.SecretHash, testHash)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `just backend test` (storage integration)
Expected: FAIL — `CreateAPIKey` arity mismatch / `got.SecretHash` undefined.

- [ ] **Step 3: Add `SecretHash` to the model**

In `backend/internal/models/apikey/apikey.go` `APIKey` struct, add after `JTI`:

```go
	SecretHash string `json:"-"` // SHA-256 of the opaque client_secret; never serialized
```

- [ ] **Step 4: Thread it through storage**

In `backend/internal/storage/apikeys.go`:

`CreateAPIKey` — add `secretHash string` param (after `name`/before `scopes` is fine; pick one and use consistently) and write it:

```go
func (s *Storage) CreateAPIKey(
	ctx context.Context,
	orgID int,
	name string,
	secretHash string,
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
            (org_id, name, secret_hash, scopes, created_by, created_by_key_id, expires_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, jti, org_id, name, secret_hash, scopes, created_by, created_by_key_id,
                  created_at, expires_at, last_used_at, revoked_at
    `, orgID, name, secretHash, scopes, creator.UserID, creator.KeyID, expiresAt).Scan(
		&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.SecretHash, &k.Scopes,
		&k.CreatedBy, &k.CreatedByKeyID,
		&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert api_keys: %w", err)
	}
	return &k, nil
}
```

`GetAPIKeyByJTI` and `GetAPIKeyByID` — add `secret_hash` to the SELECT column list and to the `.Scan(...)` (insert `&k.SecretHash` right after `&k.Name`). The list-queries (`ListActiveAPIKeys`, `ListActiveAPIKeysPaginated`) do NOT need it — leave them unchanged (list never exposes the hash).

- [ ] **Step 5: Update other `CreateAPIKey` callers to compile**

Run: `cd backend && grep -rn "\.CreateAPIKey(" --include=*.go`
For each non-test caller, pass a hash argument. The two production callers (`handlers/orgs/api_keys.go`, `handlers/testhandler/apikeys.go`) are rewritten in later tasks — for now pass a placeholder `apisecret.Hash("placeholder")` only if needed to compile, but prefer to leave those two until their tasks and instead temporarily pass `""`. Simplest: in this task, update ONLY the storage signature + the storage test; update the two handlers within their own tasks (5 and 7). To keep the build green between tasks, temporarily pass `""` as the hash in those two handlers now and replace it in Tasks 5/7.

- [ ] **Step 6: Run to verify it passes**

Run: `cd backend && go build ./... && just backend test`
Expected: build clean; storage integration test PASSES with the round-trip assertion.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/models/apikey/ backend/internal/storage/ backend/internal/handlers/
git commit -m "feat(storage): persist api_keys.secret_hash (TRA-847)"
```

---

### Task 5: Create endpoint returns `{client_id, client_secret}`

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go` (`APIKeyCreateResponse`)
- Modify: `backend/internal/handlers/orgs/api_keys.go` (`CreateAPIKey` handler)
- Test: `backend/internal/handlers/orgs/api_keys_integration_test.go`

- [ ] **Step 1: Update the response model**

Replace `APIKeyCreateResponse.Token` field. New struct:

```go
// APIKeyCreateResponse is returned ONCE from POST. client_secret is the opaque
// secret shown exactly once and never persisted in plaintext; client_id is the
// row's jti, used as the client_credentials client_id at POST /oauth/token.
type APIKeyCreateResponse struct {
	ClientID     string     `json:"client_id"`
	ClientSecret string     `json:"client_secret"`
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}
```

- [ ] **Step 2: Update the handler test (failing)**

In `api_keys_integration_test.go` `TestCreateAPIKey_Admin` (≈ line 77): assert the new shape instead of `token`. Replace the token assertion with:

```go
var body struct {
	Data apikey.APIKeyCreateResponse `json:"data"`
}
mustDecode(t, rec.Body, &body)
if body.Data.ClientSecret == "" || !strings.HasPrefix(body.Data.ClientSecret, "trakrf_") {
	t.Errorf("expected opaque client_secret, got %q", body.Data.ClientSecret)
}
if body.Data.ClientID != body.Data.ClientID || body.Data.ClientID == "" {
	t.Error("expected non-empty client_id (jti)")
}
// The secret must be the opaque secret, never a JWT (no dots).
if strings.Count(body.Data.ClientSecret, ".") == 2 {
	t.Error("client_secret looks like a JWT; must be opaque")
}
```

(Adjust to the file's existing decode helper if one exists.)

- [ ] **Step 3: Run to verify it fails**

Run: `just backend test`
Expected: FAIL — `body.Data.Token` removed / `ClientSecret` empty.

- [ ] **Step 4: Rewrite the handler mint block**

In `backend/internal/handlers/orgs/api_keys.go` `CreateAPIKey`, replace the `jwt.GenerateAccessToken(...)` block (≈ lines 131-147) with generate-secret-then-store-hash. The secret must be generated BEFORE the row insert so the hash can be persisted:

```go
	secret, err := apisecret.Generate()
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to generate client secret", reqID)
		return
	}

	key, err := h.storage.CreateAPIKey(r.Context(), orgID, req.Name, apisecret.Hash(secret),
		req.Scopes, creator, req.ExpiresAt)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to create api key", reqID)
		return
	}

	resp := apikey.APIKeyCreateResponse{
		ClientID:     key.JTI,
		ClientSecret: secret,
		ID:           key.ID,
		Name:         key.Name,
		Scopes:       key.Scopes,
		CreatedAt:    key.CreatedAt,
		ExpiresAt:    key.ExpiresAt,
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
```

Notes: remove the now-orphaned earlier `CreateAPIKey` call (the original ≈ lines 122-129 block) — there must be exactly one create call, the one above that passes the hash. Add the `apisecret` import; drop the `jwt` import if no longer used in this file. Update the handler's `@Description` swagger comment to say it returns `{client_id, client_secret}` and the secret is shown once.

- [ ] **Step 5: Run to verify it passes**

Run: `cd backend && go build ./... && just backend test`
Expected: build clean; create-endpoint test PASSES.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/apikey/ backend/internal/handlers/orgs/
git commit -m "feat(api-keys): return opaque client_id/client_secret on create (TRA-847)"
```

---

### Task 6: `client_credentials` grant verifies the opaque secret

**Files:**
- Modify: `backend/internal/handlers/auth/oauth.go` (`tokenClientCredentials`)
- Modify: `backend/internal/handlers/auth/oauth_integration_test.go`

- [ ] **Step 1: Update the integration test (failing)**

In `oauth_integration_test.go` `TestOAuthToken_ClientCredentialsThenRefresh` (≈ line 33): the client_secret must now be the opaque secret returned by key creation, not a minted JWT. Mint the key through the storage/create path with a known secret and present it. Minimal shape:

```go
secret, err := apisecret.Generate()
if err != nil { t.Fatal(err) }
key, err := store.CreateAPIKey(ctx, org.ID, "cc-test", apisecret.Hash(secret),
	[]string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
if err != nil { t.Fatal(err) }

reqBody := fmt.Sprintf(`{"grant_type":"client_credentials","client_id":%q,"client_secret":%q}`,
	key.JTI, secret)
// POST /api/v1/oauth/token with reqBody, expect 200 + access_token + refresh_token
```

In `TestOAuthToken_BadSecretIs401` (≈ line 95): present a wrong opaque secret (`secret+"x"` or a random string) against a real `client_id`, expect 401.

- [ ] **Step 2: Run to verify it fails**

Run: `just backend test`
Expected: FAIL — the grant still tries `jwt.ValidateAccessToken` on an opaque (non-JWT) secret.

- [ ] **Step 3: Rewrite the verification block**

In `backend/internal/handlers/auth/oauth.go` `tokenClientCredentials`, replace the JWT-validation block (≈ lines 62-67) with a row lookup + hash verify. Lookup by `client_id` (jti) first, then verify the secret:

```go
	key, err := handler.store.GetAPIKeyByJTI(r.Context(), request.ClientID)
	if err != nil || key == nil {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}
	if !apisecret.Verify(request.ClientSecret, key.SecretHash) {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}
	if key.RevokedAt != nil || (key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now())) {
		httputil.Respond401(w, r, "Invalid client credentials", reqID)
		return
	}
```

This replaces both the old `jwt.ValidateAccessToken` block AND the now-duplicated `GetAPIKeyByJTI` block (≈ lines 69-77) — there must be exactly one lookup. Add the `apisecret` import; drop the `jwt` import from this file if unused. Update the `@Description` swagger comment on `Token` (≈ line 17): `client_secret` is "the opaque secret returned once at key creation", not "the long-lived API key token".

- [ ] **Step 4: Run to verify it passes**

Run: `cd backend && go build ./... && just backend test`
Expected: build clean; oauth integration tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/auth/
git commit -m "feat(oauth): verify opaque client_secret against stored hash (TRA-847)"
```

---

### Task 7: testhandler mint returns an access token; delete old long-lived-bearer fixtures

**Files:**
- Modify: `backend/internal/handlers/testhandler/apikeys.go`
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go` (`mintKeysAdminAPIKey` + api-key-principal tests)
- Delete: any test/helper that mints a no-exp JWT and uses it directly as Bearer

- [ ] **Step 1: Rewrite the schemathesis mint helper**

In `backend/internal/handlers/testhandler/apikeys.go` `MintAPIKey`: the contract loop needs a usable Bearer = a short-lived access token. Create the row with a (discarded) secret hash, then mint an access token with a generous test TTL so a full schemathesis run doesn't expire mid-flight:

```go
	secret, err := apisecret.Generate()
	if err != nil {
		http.Error(w, "Failed to generate secret", http.StatusInternalServerError)
		return
	}
	creator := apikey.Creator{UserID: &user.ID}
	key, err := h.storage.CreateAPIKey(ctx, org.ID, mintedKeyName, apisecret.Hash(secret),
		req.Scopes, creator, nil)
	if err != nil {
		http.Error(w, "Failed to create api key", http.StatusInternalServerError)
		return
	}

	exp := time.Now().Add(1 * time.Hour) // generous: covers a full contract-test run
	token, err := jwt.GenerateAccessToken(key.JTI, org.ID, req.Scopes, &exp)
	if err != nil {
		http.Error(w, "Failed to sign access token", http.StatusInternalServerError)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, MintAPIKeyResponse{
		Token: token,
		JTI:   key.JTI,
		Name:  key.Name,
	})
```

Add the `time` import. (`MintAPIKeyResponse.Token` stays — it is now a short-lived access token, not a long-lived key; the schemathesis client uses it identically as `Authorization: Bearer`.)

- [ ] **Step 2: Update the orgs integration-test bearer helper**

In `api_keys_integration_test.go`, find `mintKeysAdminAPIKey` (≈ lines 66-75) and any helper that builds a Bearer from a directly-minted long-lived JWT. Replace its body so the Bearer is a short-lived access token minted via `jwt.GenerateAccessToken(jti, orgID, scopes, &exp)` AFTER creating the row with a hashed secret (use `apisecret.Generate()`/`Hash()` for the row). Example helper:

```go
func mintKeysAdminAPIKey(t *testing.T, store *storage.Storage, orgID, userID int) (jti, bearer string) {
	t.Helper()
	secret, err := apisecret.Generate()
	if err != nil { t.Fatal(err) }
	key, err := store.CreateAPIKey(context.Background(), orgID, "test-admin",
		apisecret.Hash(secret), []string{"keys:admin"}, apikey.Creator{UserID: &userID}, nil)
	if err != nil { t.Fatal(err) }
	exp := time.Now().Add(15 * time.Minute)
	bearer, err = jwt.GenerateAccessToken(key.JTI, orgID, []string{"keys:admin"}, &exp)
	if err != nil { t.Fatal(err) }
	return key.JTI, bearer
}
```

Update each caller to the helper's signature. For the api-key-principal tests (`TestCreateAPIKey_ByAPIKeyPrincipal`, `TestListAPIKeys_ByAPIKeyPrincipal`, `TestRevokeAPIKey_ByAPIKeyPrincipal`, `TestRevokeAPIKey_KeyRevokesItself`), the Bearer they pass must likewise be a short-lived access token for the parent key (same pattern).

- [ ] **Step 3: Delete dead fixtures**

Run: `cd backend && grep -rn "GenerateAccessToken(.*nil)" --include=*_test.go internal/`
Any remaining test that mints a NO-exp access token and uses it as a Bearer is the deprecated pattern — convert it to the create→short-lived-token pattern above, or delete the test if it exists solely to assert the old long-lived behavior.

- [ ] **Step 4: Build + run the affected suites**

Run: `cd backend && go build ./... && just backend test`
Expected: build clean; orgs + testhandler + oauth suites PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/
git commit -m "test(api-keys): bearer via grant access token; drop long-lived JWT fixtures (TRA-847)"
```

---

### Task 8: Require `exp` on access-token validation (invariant)

Now that nothing mints a no-exp api-key JWT, assert the invariant that an access token always carries an expiry.

**Files:**
- Modify: `backend/internal/util/jwt/apikey.go` (`ValidateAccessToken`)
- Test: `backend/internal/util/jwt/apikey_test.go`

- [ ] **Step 1: Write the failing test**

In `apikey_test.go` add:

```go
func TestValidateAccessTokenRejectsMissingExp(t *testing.T) {
	// A token minted with no expiry (the deprecated long-lived shape) must be rejected.
	tok, err := GenerateAccessToken("some-jti", 1, []string{"assets:read"}, nil)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if _, err := ValidateAccessToken(tok); err == nil {
		t.Error("expected ValidateAccessToken to reject a token with no exp")
	}
}

func TestValidateAccessTokenAcceptsBoundedExp(t *testing.T) {
	exp := time.Now().Add(15 * time.Minute)
	tok, err := GenerateAccessToken("some-jti", 1, []string{"assets:read"}, &exp)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if _, err := ValidateAccessToken(tok); err != nil {
		t.Errorf("expected valid token to pass, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd backend && go test ./internal/util/jwt/ -run TestValidateAccessToken -v`
Expected: `TestValidateAccessTokenRejectsMissingExp` FAILS (no-exp token currently accepted).

- [ ] **Step 3: Add the parser option**

In `ValidateAccessToken`, add `jwt.WithExpirationRequired()` to the `jwt.NewParser(...)` options:

```go
	parser := jwt.NewParser(
		jwt.WithIssuer(apiKeyIssuer),
		jwt.WithAudience(apiKeyAudience),
		jwt.WithExpirationRequired(),
	)
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd backend && go test ./internal/util/jwt/ -v`
Expected: all jwt tests PASS, including both new cases.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/jwt/
git commit -m "feat(jwt): require exp on access-token validation (TRA-847)"
```

---

### Task 9: Public OpenAPI auth model

**Files:**
- Modify: the public OpenAPI spec source (find it in Step 1)

- [ ] **Step 1: Locate the spec + the security scheme**

Run: `cd /home/mike/platform && grep -rln "ApiKeyAuth\|securitySchemes" --include=*.yaml --include=*.yml --include=*.json | grep -i public`
Then: `grep -n "ApiKeyAuth\|Bearer token\|/orgs/{id}/api-keys\|token:" <that-file>`
Expected: identifies the `securitySchemes.ApiKeyAuth` block, the create-endpoint response schema, and any "use this as a Bearer token" prose.

- [ ] **Step 2: Update the security scheme description**

Edit `securitySchemes.ApiKeyAuth.description` to: exchange `{client_id, client_secret}` (from key creation) at `POST /oauth/token` for a short-lived Bearer access token; present that access token as `Authorization: Bearer`. Remove any wording that says the key-creation `token` is itself usable as a Bearer credential.

- [ ] **Step 3: Update the create-endpoint response schema**

Where `POST /orgs/{id}/api-keys` documents its 201 body, replace the `token` property with `client_id` (string) and `client_secret` (string, "shown once"). Keep `id`, `name`, `scopes`, `created_at`, `expires_at`. If the spec is generated from swag annotations rather than hand-authored, regenerate it (`just backend <swagger-gen-recipe>` — check `backend/justfile`) so the `APIKeyCreateResponse` change from Task 5 propagates; otherwise hand-edit.

- [ ] **Step 4: Lint**

Run: `just lint` (or the project's spectral recipe — check the justfile for `spectral`/`lint-openapi`).
Expected: PASS, no new violations on the public spec.

- [ ] **Step 5: Commit**

```bash
git add <spec-file(s)>
git commit -m "docs(openapi): point api-key auth at /oauth/token (TRA-847)"
```

---

### Task 10: Full validation + PR

- [ ] **Step 1: Full validate**

Run: `just validate` (lint + test, both workspaces).
Expected: PASS. Capture the actual output.

- [ ] **Step 2: Confirm acceptance criteria**

- `grep -rn "GenerateAccessToken\|ValidateAccessToken" backend/internal` — no production caller mints a no-exp token; create endpoint mints none.
- Create endpoint returns `{client_id, client_secret}` (Task 5 test green).
- Bad opaque secret → 401 (Task 6 test green).
- No-exp access token rejected (Task 8 test green).
- OpenAPI lints clean (Task 9).

- [ ] **Step 3: Push + open PR**

```bash
git push -u origin feat/tra-847-deprecate-api-keys-bearer
gh pr create --title "feat(api): deprecate api-keys-as-bearer; opaque client_secret (TRA-847)" --body "$(cat <<'EOF'
## Summary
- `POST /orgs/{id}/api-keys` now returns an opaque `{client_id, client_secret}` (secret shown once, stored SHA-256 hashed); it no longer mints a long-lived JWT.
- `client_credentials` grant verifies the opaque secret against the stored hash instead of validating a JWT.
- Grant-minted short-lived access JWTs remain the only Bearer credential; `ValidateAccessToken` now requires `exp`.
- Migration 000013 adds `api_keys.secret_hash` and deletes stale rows (0 live prod keys).
- Public OpenAPI auth model points at `/oauth/token`.

Closes TRA-847.

## Test plan
- [ ] `just validate` green
- [ ] create returns opaque client_secret, never a JWT
- [ ] client_credentials with correct secret → 200; wrong secret → 401
- [ ] no-exp access token rejected 401
- [ ] schemathesis contract tests pass via grant-based mint helper
- [ ] OpenAPI lints clean

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR opened; preview deploy to `https://app.preview.trakrf.id` kicks off.
