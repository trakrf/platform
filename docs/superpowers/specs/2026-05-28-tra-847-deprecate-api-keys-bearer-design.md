# TRA-847 — Deprecate api_keys-as-bearer pattern (rip the long-lived JWT path)

**Date:** 2026-05-28
**Depends on:** TRA-846 (OAuth2 client_credentials + refresh_token grant — merged in #424)
**Blocks:** TRA-848 (integrator docs cutover, separate trakrf-docs repo)

## Problem

Today `POST /api/v1/orgs/{id}/api-keys` mints a long-lived api-key JWT and returns it as
`token`. Integrators use that JWT directly as a `Bearer` credential indefinitely. With
TRA-846 landing the short-lived grant flow, the long-lived path is dead weight and a worse
security posture. Removing it before any integrator ships against it is far cheaper than
after.

No backwards compatibility is required: there are zero live api_keys in prod and no
customer-facing integrations yet. Existing rows (preview e2e cruft) are disposable.

## Target model

1. `POST /orgs/{id}/api-keys` returns an opaque `{client_id, client_secret}`. It no longer
   mints or returns a JWT. `client_id` is the row's `jti`; `client_secret` is a fresh
   high-entropy opaque secret, stored **hashed** on the row and shown exactly once.
2. The integrator exchanges `{client_id, client_secret}` at `POST /oauth/token`
   (`grant_type=client_credentials`) for a short-lived (15 min) access JWT + rotating
   refresh token. The access JWT is the **only** thing usable as a `Bearer` credential.
3. The long-lived-JWT-as-Bearer pattern is *deleted*, not gated — nothing in the codebase
   mints such a token anymore, so it cannot exist.

## Design

### 1. Schema — migration `000013_api_key_secret_hash`

- Add `secret_hash VARCHAR(64) NOT NULL` to `trakrf.api_keys`.
- Up-migration first removes pre-existing data that is unusable under the new model:
  delete `refresh_tokens` rows referencing `api_keys` (FK), then delete `api_keys` rows.
  This avoids a nullable column and dead, non-authenticatable rows. (Safe: 0 live prod
  keys per the DB audit; preview is disposable e2e churn.)
- Down-migration drops the column.

### 2. JWT layer (`backend/internal/util/jwt/apikey.go`)

- Rename `GenerateAPIKey` → `GenerateAccessToken`, `ValidateAPIKey` → `ValidateAccessToken`.
  The old names are now misleading: these functions only ever handle short-lived grant
  access tokens, never long-lived keys.
- `ValidateAccessToken` adds `jwt.WithExpirationRequired()`. Access tokens always carry a
  15-min `exp`; requiring it asserts what an access token *is* (not a back-compat hedge).
- Issuer `trakrf-api-key` and audience `trakrf-api` are unchanged — grant-minted access
  tokens keep validating exactly as TRA-846 produces them (ticket §2).
- Update `classify.go`, `middleware/apikey.go`, `middleware/either_auth.go`, and the
  `services/auth` callers to the renamed functions.

### 3. Opaque secret hashing — SHA-256 (`backend/internal/util/apisecret`)

New small package:
- `Generate() (string, error)` — 32 random bytes, returned as `trakrf_` + hex (the prefix
  aids secret scanning / log greppability).
- `Hash(secret string) string` — SHA-256 → 64 hex chars.
- `Verify(presented, storedHash string) bool` — `Hash(presented)` compared in constant time.

SHA-256 matches the established `refresh_tokens.token_hash` precedent for high-entropy
opaque secrets. bcrypt is reserved for low-entropy human passwords and would diverge from
that pattern. (Approved over bcrypt.)

### 4. Create endpoint + storage + model

- `apikey.APIKeyCreateResponse`: replace `Token string` with `ClientID string` and
  `ClientSecret string`.
- `storage.CreateAPIKey(...)`: accept a `secretHash string` argument and persist it into
  the new column.
- `apikey.APIKey` model + `storage.GetAPIKeyByJTI`: carry `SecretHash` so the grant flow
  can verify it.
- `handlers/orgs/api_keys.go` `CreateAPIKey`: `apisecret.Generate()` → `apisecret.Hash()`
  → store hash → respond
  `{ data: { client_id: jti, client_secret: <plaintext, once>, id, name, scopes,
  created_at, expires_at } }`. No `jwt.GenerateAccessToken` call here.

### 5. Grant flow (`backend/internal/handlers/auth/oauth.go` `tokenClientCredentials`)

- `TokenRequest.ClientSecret` is now the opaque secret, not a JWT.
- Replace `jwt.ValidateAPIKey(request.ClientSecret)` with: `GetAPIKeyByJTI(client_id)` →
  `apisecret.Verify(request.ClientSecret, key.SecretHash)` → existing revoked/expired
  checks → `MintAPITokenPair` (unchanged). All failure modes still return a uniform 401.

### 6. Public OpenAPI

- `securitySchemes.ApiKeyAuth` description points at `/oauth/token`; remove the
  "use this as a Bearer token" wording.
- Create-endpoint response schema → `{client_id, client_secret}`.
- Must lint clean (spectral) under the new auth model.

### 7. Tests / fixtures — delete the old path

- `handlers/auth/oauth_integration_test.go`: `client_secret` is the opaque secret returned
  by key creation; bad-secret-is-401 test presents a wrong opaque secret.
- `handlers/orgs/api_keys_integration_test.go`: `mintKeysAdminAPIKey` and every
  api-key-principal test obtain their Bearer via create → grant → access token, not a
  directly-minted long-lived JWT.
- `handlers/testhandler/apikeys.go` (schemathesis mint helper): return a short-lived
  access token (mint via the grant path internally) so contract tests still receive a
  usable Bearer.
- Delete any test/fixture that mints a long-lived JWT and uses it directly as Bearer.

## Acceptance

- `POST /orgs/{id}/api-keys` returns `{client_id, client_secret}` and never a JWT.
- The long-lived-JWT-as-Bearer pattern no longer exists (nothing mints it).
- Schemathesis contract tests pass with the grant-based mint helper.
- Public OpenAPI lints cleanly.

## Out of scope

- Integrator documentation rewrite (TRA-848, trakrf-docs repo).
- Migration of existing api_keys rows (none live; rows are deleted, not migrated).
