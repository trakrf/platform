# TRA-846 — API short-lived JWT grant flow (OAuth2 client_credentials + refresh_token)

**Status:** Design approved 2026-05-28
**Ticket:** TRA-846 (depends on TRA-843; blocks TRA-847, TRA-848)

## Problem

The public API today authenticates with a **long-lived, never-expiring** api-key JWT.
The JWT itself is the credential — there is no separate secret column; `api_keys`
stores only the `jti` (revocation handle) and `scopes`. A leaked key is valid until
someone manually revokes it.

TRA-843 shipped session refresh tokens with forward-compat schema
(`refresh_tokens.token_type` ENUM `('session','api')`, `refresh_tokens.api_key_id`
FK with `ON DELETE CASCADE`) explicitly to host this flow. This ticket adds the
OAuth2 `client_credentials` + `refresh_token` grant so integrators exchange their
long-lived key for **short-lived (15 min) access tokens** plus a rotating 30-day
refresh token.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Endpoint path | `POST /api/v1/oauth/token` | Standard OAuth2 convention; clearly separate from human-session `/api/v1/auth/*`. |
| `client_id` | `api_keys.jti` | The existing revocation handle / JWT subject. |
| `client_secret` | the long-lived api-key JWT ("the secret as today") | No secret column exists; the JWT is the credential. |
| `user_id` on api refresh rows | **migration → nullable** | An integration is not a user. Migration relaxes `user_id` to nullable and updates the type-consistency CHECK so `api` rows require `api_key_id` and forbid `user_id`. TRA-843 shipped 2026-05-27, low deploy risk. |
| Access TTL | 15 min | Per acceptance criteria; explicit `exp` on the api-issuer JWT. |
| Refresh TTL | 30 days | Reuses TRA-843 `refreshTokenTTL`. |
| Wire format | JSON request + platform `ErrorResponse` envelope | Uniform with every other endpoint and with schemathesis. |
| Service shape | new `MintAPITokenPair` / `RefreshAPIToken` methods | The existing `MintTokenPair`/`Refresh` are user-shaped (`userID`, `email`, `generateJWT(int,string,*int)`). Parallel API methods share the low-level helpers rather than contorting the user signatures. |

## Schema change (migration 000012)

```sql
SET search_path = trakrf, public;

ALTER TABLE refresh_tokens ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_type_consistent;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_type_consistent CHECK (
    (token_type = 'session' AND user_id IS NOT NULL AND api_key_id IS NULL) OR
    (token_type = 'api'     AND user_id IS NULL     AND api_key_id IS NOT NULL)
);
```

Down migration restores `NOT NULL` (safe only if no api rows exist) and the original CHECK.

`storage.RefreshToken.UserID` becomes `*int`; `CreateRefreshToken` /
`RotateRefreshToken` / `GetRefreshTokenByHash` adjust to nullable `user_id`.
New columns surfaced on the struct: `TokenType string`, `APIKeyID *int64`.

## Storage layer

Two new methods (parallel to the session ones, writing the api discriminator):

- `CreateAPIRefreshToken(ctx, apiKeyID int64, orgID *int, tokenHash, expiresAt, userAgent, ip)` — inserts `token_type='api'`, `user_id=NULL`, `api_key_id=$apiKeyID`.
- `RotateAPIRefreshToken(ctx, oldID, apiKeyID int64, orgID *int, newHash, expiresAt, userAgent, ip)` — same transactional rotate as `RotateRefreshToken` but with the api discriminator.

`GetRefreshTokenByHash` extended to also return `token_type` and `api_key_id` so the
service can branch and reject cross-type use (a session refresh presented at the
oauth endpoint, or vice versa, is rejected).

New `GetAPIKeyByID(ctx, id int64) (*apikey.APIKey, error)` — the refresh row stores
`api_key_id` (the `api_keys.id` int), not the jti, so refresh re-mint needs a
by-id lookup (existing storage only has `GetAPIKeyByJTI`). Returns jti, scopes, org,
and revoked/expired state for the active-check.

`RevokeRefreshTokenChain`, `RevokeRefreshToken` unchanged — reused as-is.

## Service layer (`internal/services/auth/`)

New file `api_token.go`:

```
const apiAccessTokenTTL = 15 * time.Minute

// MintAPITokenPair authenticates a client_credentials request and issues a pair.
func (s *Service) MintAPITokenPair(ctx, jti string, scopes []string, orgID int,
        apiKeyID int64, userAgent, ip string) (accessToken, refreshSecret string, expiresIn int, err error)
    - exp := now + apiAccessTokenTTL
    - accessToken = jwt.GenerateAPIKey(jti, orgID, scopes, &exp)
    - refreshSecret = generateRefreshSecret()
    - storage.CreateAPIRefreshToken(... hashRefreshSecret(refreshSecret) ...)
    - expiresIn = int(apiAccessTokenTTL.Seconds())  // 900

// RefreshAPIToken exchanges an api refresh token for a new pair.
func (s *Service) RefreshAPIToken(ctx, presentedSecret, userAgent, ip string) (*APITokenResponse, error)
    - row = GetRefreshTokenByHash(hash); reuse the TRA-843 reject ladder
      (nil / revoked / used→revoke-chain / expired)
    - require row.TokenType == "api" && row.APIKeyID != nil  (else invalid)
    - key = GetAPIKeyByID(row.APIKeyID); reject if nil / revoked / expired
    - mint new api JWT with key.Scopes (current scopes, single source of truth)
    - RotateAPIRefreshToken(...)
```

Shared with TRA-843: `generateRefreshSecret`, `hashRefreshSecret`, `refreshTokenTTL`,
replay-detection semantics.

Note: `MintTokenPair` authenticates *nothing* — it is called post-auth. The
client_credentials authentication (validate JWT, subject==client_id, look up + check
active) lives in the **handler**, mirroring how `Login` authenticates before calling
the service.

## Handler layer (`internal/handlers/auth/`)

New file `oauth.go`, registered as `r.Post("/api/v1/oauth/token", handler.Token)`
in the existing `RegisterRoutes`.

```
type TokenRequest struct {
    GrantType    string `json:"grant_type" validate:"required,oneof=client_credentials refresh_token"`
    ClientID     string `json:"client_id"`     // required for client_credentials
    ClientSecret string `json:"client_secret"` // required for client_credentials
    RefreshToken string `json:"refresh_token"` // required for refresh_token
}

type TokenResponse struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    TokenType    string `json:"token_type"`  // "Bearer"
    ExpiresIn    int    `json:"expires_in"`  // 900
}
```

`Token` handler:
- decode + validate; branch on `grant_type`.
- **client_credentials:** require `client_id` + `client_secret`. `jwt.ValidateAPIKey(client_secret)`; reject if `claims.Subject != client_id`. `GetAPIKeyByJTI(client_id)`; reject if nil/revoked/expired. Call `MintAPITokenPair`.
- **refresh_token:** require `refresh_token`. Call `RefreshAPIToken`. On `invalid_refresh_token` → 401.
- All failures use platform `ErrorResponse` (401 for bad creds / bad refresh, 400 for malformed body, 415 for wrong content-type — same helpers as the other auth handlers).

The handler interface (`authServicer`) gains `MintAPITokenPair` and `RefreshAPIToken`.

## Acceptance mapping

| Acceptance criterion | Covered by |
|---|---|
| client_credentials → 15-min access JWT + 30-day refresh | handler client_credentials branch + `MintAPITokenPair` |
| refresh_token rotates; replay revokes chain | `RefreshAPIToken` + reused `RevokeRefreshTokenChain` |
| revoking api_keys row cascades to refresh tokens | existing `ON DELETE CASCADE` FK (no code) + GetAPIKeyByID active-check on refresh |
| schemathesis contract tests pass both grants | OpenAPI annotations on `Token` + request/response models |
| public OpenAPI spec has endpoint + worked example | swaggo annotations, `@Tags` public, worked example in description |

## Testing

- **Service unit tests** (`api_token_test.go`): mint produces decodable api JWT with `exp≈15m`, correct scopes/org; refresh rotates + marks old used; replay of used api token revokes chain; cross-type rejection (session row at api refresh).
- **Storage integration tests**: `CreateAPIRefreshToken` writes `token_type='api'`, `user_id NULL`, `api_key_id` set; CHECK rejects malformed rows; CASCADE on api_keys delete removes refresh rows.
- **Handler tests**: client_credentials happy path; bad secret → 401; subject/client_id mismatch → 401; refresh happy path; refresh of revoked key → 401; unsupported grant_type → 400.
- **Schemathesis**: both grants exercised against preview via the existing contract gate.

## Out of scope (per ticket)

Deprecating api-keys-as-bearer (TRA-847), multi-secret rotation, PKCE,
authorization_code grant.
