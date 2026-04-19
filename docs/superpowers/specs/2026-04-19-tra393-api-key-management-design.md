# API key management — JWT-based keys with scopes, rotation, and admin UI

**Linear:** [TRA-393](https://linear.app/trakrf/issue/TRA-393) — parent epic [TRA-210](https://linear.app/trakrf/issue/TRA-210), builds on [TRA-392](https://linear.app/trakrf/issue/TRA-392)
**Date:** 2026-04-19
**Status:** Design — pending implementation plan
**Author:** Mike Stankavich

---

## Context and goals

TRA-392 designed the TrakRF public REST API; this spec implements the authentication layer that public callers use. The output of TRA-393 is the server-side infrastructure for issuing, validating, and revoking API keys — plus the admin UI org owners and admins use to manage them — and one trivial protected endpoint (`GET /api/v1/orgs/me`) that customers can hit to verify a key works end-to-end.

Public business endpoints (assets, locations, scans) are [TRA-396](https://linear.app/trakrf/issue/TRA-396)'s concern, not this one. With TRA-393 merged, TRA-396 can apply `APIKeyAuth` + `RequireScope` to its handlers and ship.

### Goals

- A working `APIKeyAuth` middleware that enforces TRA-392's JWT contract
- A working `RequireScope(string)` middleware for per-endpoint authorization
- A schema migration for `api_keys` that matches TRA-392's table exactly
- An admin UI to create, list, and revoke keys
- One canary endpoint (`GET /api/v1/orgs/me`) so customers can verify key health before TRA-396 lands

### Non-goals

- Public business endpoints (TRA-396)
- Rate limiting and `tier` column (separate sub-issue — must land before TeamCentral load-tests us, but out of TRA-393)
- OAuth 2.0 client-credentials grant (v1.x)
- Pre-expiry warning emails (v1.x)
- Split signing keys for session vs API-key JWTs (v1.x)
- In-place key rotation (deliberately not supported; rotation is create-new-revoke-old)
- Language SDKs or Postman collections (TRA-394)

---

## Architecture

Public-key infrastructure lives alongside session auth, not layered over it. Two middlewares share the same `JWT_SECRET` and HS256 signing; the `iss` claim discriminates. Three backend artifacts and one frontend screen:

1. **Migration** — `000027_api_keys.up.sql` / `.down.sql`, table + partial index + RLS policy
2. **Middleware** — `APIKeyAuth` (parse → lookup → principal) and `RequireScope(scope)` (authorize)
3. **Handlers** — CRUD at `/api/v1/orgs/{id}/api-keys` (session-auth'd, admin-only) + canary `GET /api/v1/orgs/me` (API-key-auth'd)
4. **Frontend** — `APIKeysScreen` at route `#api-keys`, linked from `OrgSettingsScreen`

Non-public frontend routes keep using `middleware.Auth` unchanged. Each middleware puts its own principal type on request context (`UserClaimsKey` vs. `APIKeyPrincipalKey`); handlers extract `OrgID` from whichever is present and pass it into the existing storage transaction helper, which sets `app.current_org_id` for RLS. The storage layer stays principal-agnostic.

---

## Data model

### Migration: `backend/migrations/000027_api_keys.{up,down}.sql`

```sql
-- up
CREATE TABLE api_keys (
    id           BIGINT PRIMARY KEY,
    jti          UUID NOT NULL UNIQUE,
    org_id       INT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL,
    scopes       TEXT[] NOT NULL,
    created_by   INT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

-- partial index for "list active keys for this org" — the dominant UI query
CREATE INDEX api_keys_active_by_org_idx
    ON api_keys(org_id)
    WHERE revoked_at IS NULL;

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;

CREATE POLICY api_keys_org_isolation ON api_keys
    USING      (org_id = current_setting('app.current_org_id')::INT)
    WITH CHECK (org_id = current_setting('app.current_org_id')::INT);

-- id is populated by application code via the existing generate_permuted_id machinery;
-- mirrors the approach used for assets, locations, etc.
```

```sql
-- down
DROP TABLE api_keys;  -- cascades index + policy
```

### Schema rationale

- `id` is a permuted BIGINT (consistent with the rest of the schema). Used as the path param on `DELETE`.
- `jti` is the join key from JWT claim → row. UUID keeps tokens opaque and lets us regenerate IDs without collisions if we ever need to.
- `scopes TEXT[]` stores TRA-392's five strings: `assets:read`, `assets:write`, `locations:read`, `locations:write`, `scans:read`. A custom enum type was considered and rejected — adding scopes (e.g., `webhooks:write`) would require a migration.
- No `key_hash`. No `key_prefix`. JWTs are self-validating; the signature *is* the proof. Storing a hash would be dead weight. The original Linear ticket called for both; TRA-392 supersedes that.
- `revoked_at IS NULL` partial index keeps the list query hot without carrying the full table in memory as revoked keys accumulate.

### Soft cap — 10 active keys per org

Enforced in the `POST` handler, not as a DB constraint, so the 11th request returns a structured `409 conflict`:

```json
{
  "error": {
    "type": "conflict",
    "title": "Key limit reached",
    "status": 409,
    "detail": "Organization has reached the 10 active API key limit. Revoke an unused key first.",
    "instance": "/api/v1/orgs/{id}/api-keys",
    "request_id": "01J..."
  }
}
```

---

## JWT shape

New file `backend/internal/util/jwt/apikey.go` alongside the existing `jwt.go` to keep session and API-key tokens cleanly separated in code, even though they share a signing secret.

```go
type APIKeyClaims struct {
    OrgID  int      `json:"org_id"`
    Scopes []string `json:"scopes"`
    jwt.RegisteredClaims
}

// GenerateAPIKey mints a signed token for a newly-created api_keys row.
// sub is the jti (UUID); exp is optional (nil → no expiry claim).
func GenerateAPIKey(jti string, orgID int, scopes []string, exp *time.Time) (string, error)

// ValidateAPIKey verifies signature, iss, aud, and exp (if present).
// Returns claims or an error. Does NOT check the DB — that's the middleware's job.
func ValidateAPIKey(token string) (*APIKeyClaims, error)
```

**Claim values:**

| Claim | Value |
|---|---|
| `iss` | `"trakrf-api-key"` (discriminator vs. session tokens, which use default empty iss) |
| `sub` | the `jti` (UUID string) |
| `aud` | `"trakrf-api"` |
| `iat` | issuance time |
| `exp` | optional; omitted if `api_keys.expires_at` is NULL |
| `org_id` | int, org this key belongs to |
| `scopes` | string array, e.g., `["assets:read","locations:read","scans:read"]` |

Signed HS256 with `JWT_SECRET` (same as session tokens). The `iss` claim is the lockout: session middleware rejects `trakrf-api-key`, API-key middleware rejects anything else.

---

## Middleware

### New file: `backend/internal/middleware/apikey.go`

```go
type APIKeyPrincipal struct {
    OrgID  int
    Scopes []string
    JTI    string
}

const APIKeyPrincipalKey contextKey = "api_key_principal"

// APIKeyAuth validates an API-key JWT and sets principal + RLS context.
func APIKeyAuth(next http.Handler) http.Handler
```

**Per-request flow:**

1. Extract `Authorization: Bearer <jwt>`; 401 on missing or malformed
2. `jwt.ValidateAPIKey(token)` — verifies signature, `iss=trakrf-api-key`, `aud=trakrf-api`, `exp` if present; 401 on any failure
3. `SELECT id, org_id, scopes, revoked_at, expires_at FROM api_keys WHERE jti = $1`
   - 401 if not found
   - 401 if `revoked_at IS NOT NULL`
   - 401 if `expires_at IS NOT NULL AND expires_at < NOW()` (defense in depth vs. JWT `exp`)
4. `UPDATE api_keys SET last_used_at = NOW() WHERE jti = $1` — fire-and-forget; logs at error level on failure, does **not** fail the request
5. `ctx.WithValue(APIKeyPrincipalKey, &APIKeyPrincipal{...})`, pass to `next`

The RLS session variable `app.current_org_id` is **not** set by the middleware. The storage/transaction layer (`backend/internal/storage/transactions.go`) already calls `SET LOCAL app.current_org_id = <orgID>` at tx start; handlers pull `OrgID` from whichever principal is on context (`UserClaimsKey` or `APIKeyPrincipalKey`) and pass it in. This matches how session-authenticated requests already work.

Step 3 is one indexed (`jti UNIQUE`) lookup — O(1). The `last_used_at` write is also O(1) on the same index. At the rate-limited tier (60 req/min/key, placeholder), write pressure is negligible.

### `RequireScope`

```go
// RequireScope rejects any request whose principal lacks the given scope.
// Must be chained AFTER APIKeyAuth.
func RequireScope(scope string) func(http.Handler) http.Handler
```

Returns 403 `forbidden` with `detail: "Missing required scope: <scope>"` if the principal's `Scopes` slice doesn't contain the required string. `/orgs/me` gets `APIKeyAuth` but *not* `RequireScope` — any valid key can call it.

### Session `Auth` middleware

Unchanged. Any refactoring of the existing `Auth` to share code with `APIKeyAuth` is explicitly out of scope — the two paths are different enough (session tokens have `user_id`, API keys don't; session tokens can hit refresh logic, API keys can't) that fighting for DRY would cost more than it saved.

### Error type addition

`backend/internal/models/errors/errors.go` gains `ErrRateLimited` alongside the existing constants. **Unused in TRA-393** — the catalog is being aligned with TRA-392's public-error contract so the future rate-limit sub-issue doesn't have to edit the error registry. If this feels like scope creep, cut it; the rate-limit sub-issue can add it when it needs it.

---

## Handlers

### `GET /api/v1/orgs/me` — canary

`backend/internal/handlers/orgs/me.go`. Registered at `/api/v1/orgs/me` behind `middleware.APIKeyAuth`. No `RequireScope`.

**Response:**

```json
{
  "id": 12345,
  "name": "Acme Forklifts, Inc."
}
```

Derives from the `APIKeyPrincipal.OrgID` on context; single `SELECT id, name FROM organizations WHERE id = $1`. Exists purely so customers can verify their key is working without needing TRA-396's endpoints to have shipped.

### `/api/v1/orgs/{id}/api-keys` — admin CRUD

`backend/internal/handlers/orgs/apikeys.go`. Registered behind the existing session `Auth` middleware plus an admin-role RBAC check (`owner` or `admin`). Non-admins get 403.

| Method | Path | Behavior |
|---|---|---|
| `POST` | `/api/v1/orgs/{id}/api-keys` | Create row, mint JWT, return `{key, id, name, scopes, created_at, expires_at}` — `key` appears **only** here |
| `GET` | `/api/v1/orgs/{id}/api-keys` | List active keys for caller's org (excludes revoked via partial index); never returns `key` |
| `DELETE` | `/api/v1/orgs/{id}/api-keys/{id}` | Set `revoked_at = NOW()`; 204 on success, 404 on unknown/cross-org id |

**`POST` request body:**

```json
{
  "name": "TeamCentral sync",
  "scopes": ["assets:read","assets:write","locations:read","locations:write","scans:read"],
  "expires_at": null
}
```

- `name` required, 1–255 chars
- `scopes` required, non-empty, subset of the five valid strings; rejected otherwise (validation error)
- `expires_at` optional ISO 8601; must be in the future if present

**`POST` response body (success):**

```json
{
  "key": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "id": 58273649,
  "name": "TeamCentral sync",
  "scopes": ["assets:read","assets:write","locations:read","locations:write","scans:read"],
  "created_at": "2026-04-19T14:23:11Z",
  "expires_at": null
}
```

Handler order: begin tx → count active keys for org (409 if ≥ 10) → insert row (generates `id` via `generate_permuted_id`, `jti` via `gen_random_uuid()`) → commit → mint JWT with that row's `jti` → return.

### RBAC

Reused: `middleware.RequireOrgAdmin(store)` — already exists in `backend/internal/middleware/rbac.go`, expects `{id}` in the URL, returns 403 for non-admins. Superadmins are bypassed and granted admin role (audit-logged). The route shape `/api/v1/orgs/{id}/api-keys` matches the existing `/api/v1/orgs/{id}/members` and `/api/v1/orgs/{id}/invitations` conventions and plugs into this helper directly.

---

## Admin UI

### Entry point

- `App.tsx` gains a lazy-loaded `APIKeysScreen` at hash route `#api-keys`
- `OrgSettingsScreen` gains a section: "API Keys — Create and manage tokens for external integrations" with a link-style button to `#api-keys`
- Both are admin-gated; non-admins see neither the link nor the route

### Component tree

```
APIKeysScreen
├── Header: "API Keys" + [New key] button
├── APIKeysList
│   ├── (empty state: "No API keys yet. Create one to let an external system talk to TrakRF.")
│   └── Row per key: name • scopes-as-chips • created • last used • expires • [Revoke]
├── CreateKeyModal                (opened by New key)
│   ├── NameInput                 required, defaults to "API key — 2026-04-19"
│   ├── ScopeSelector             three per-resource dropdowns (Q3 choice B)
│   │   ├── Assets:    None | Read | Read + Write
│   │   ├── Locations: None | Read | Read + Write
│   │   └── Scans:     None | Read
│   ├── ExpirySelector            radio: Never | 30 days | 90 days | 1 year | Custom date
│   └── [Create key] [Cancel]
├── ShowOnceModal                 (replaces CreateKeyModal after successful POST)
│   ├── Warning banner: "This is the only time you'll see the full key. Copy it now."
│   ├── Key displayed in monospace
│   ├── [Copy] button
│   └── [I've saved it] (only enabled after copy, or after 3-second dwell)
└── RevokeConfirmModal
    ├── "Revoke key 'TeamCentral sync'? Applications using it stop working immediately."
    └── [Revoke] [Cancel]
```

### ScopeSelector mapping

The three dropdowns map to the five stored scopes:

| UI dropdown value | Stored scopes |
|---|---|
| Assets: None | (nothing) |
| Assets: Read | `assets:read` |
| Assets: Read + Write | `assets:read`, `assets:write` |
| Locations: None | (nothing) |
| Locations: Read | `locations:read` |
| Locations: Read + Write | `locations:read`, `locations:write` |
| Scans: None | (nothing) |
| Scans: Read | `scans:read` |

The UI prevents the pathological "write without read" combinations automatically. At least one scope must be selected overall; submitting the form with all three on "None" surfaces a validation error before POST.

### Scope chip rendering

List-row scopes collapse into compact chips matching the dropdown language: `Assets R/W`, `Locations R`, `Scans R`. Tooltip on hover shows the raw scope strings for admins who care.

### API client

New file `frontend/src/lib/api/apiKeys.ts`, following the `orgsApi` pattern:

```ts
export const apiKeysApi = {
  list: (): Promise<APIKey[]>,
  create: (req: CreateKeyRequest): Promise<APIKeyCreateResponse>,
  revoke: (id: number): Promise<void>,
}
```

No Zustand store — the screen is small enough to use React Query directly (TanStack Query is already in the project per `OrgSettingsScreen`).

---

## Testing

### Backend integration (real DB per project convention)

`backend/internal/middleware/apikey_test.go`
- Valid key → 200, `APIKeyPrincipal` on context with expected `OrgID` / `Scopes` / `JTI`
- Missing header → 401 `unauthorized`
- Malformed header → 401
- Invalid signature → 401
- Wrong `iss` (session JWT) → 401
- Key not in DB → 401
- Revoked key → 401
- Expired key (`expires_at < NOW()`) → 401
- Expired key via JWT `exp` claim but not DB → 401 (defense in depth)
- `last_used_at` bumped on success
- `RequireScope("assets:read")` with missing scope → 403
- `RequireScope("assets:read")` with matching scope → 200

`backend/internal/handlers/orgs/apikeys_test.go`
- Admin creates key, response contains `key` field (JWT)
- Non-admin gets 403
- List excludes revoked keys
- List never includes `key` field
- 11th create returns 409
- Delete sets `revoked_at`; second delete on same id returns 404
- Cross-org delete attempt returns 404 (RLS-enforced invisibility)
- Scope validation rejects unknown strings with 400 + structured `fields[]`

`backend/internal/handlers/orgs/me_test.go`
- API-key JWT → 200, org body
- Session JWT → 401 (iss mismatch)
- Revoked key → 401

### Frontend unit (Vitest)

`APIKeysScreen.test.tsx`
- Admin renders; non-admin redirected
- Create flow: fills modal → POSTs expected payload → show-once modal appears with returned key
- Copy button calls navigator.clipboard.writeText
- "I've saved it" enabled only after copy or 3s dwell
- Revoke confirm → DELETE → row disappears from list
- Empty state rendered when list returns `[]`

`apiKeysApi.test.ts` — straightforward request-shape assertions.

### Manual verification before marking done

On the PR preview deploy:

1. Log in as admin → create key `test-key` with "Assets: Read + Write, Locations: Read, Scans: Read"
2. Copy JWT from show-once modal
3. `curl -H "Authorization: Bearer <jwt>" https://<preview>/api/v1/orgs/me` → 200, correct org
4. Revoke the key in UI
5. Same curl → 401
6. Copy a valid session JWT from dev tools, repeat curl → 401 (iss mismatch)
7. Create 10 keys rapidly; 11th attempt → UI surfaces the 409 as a user-friendly message

---

## Migration safety

- Additive-only: new table, new index, new policy. Zero changes to existing schema.
- Zero-downtime: no locks on existing tables, no data rewrites.
- Clean rollback: `DROP TABLE api_keys` cascades the index and RLS policy. No dependent FKs exist — the table's references all point *outward* (`organizations`, `users`).

---

## Observability

Every auth event is logged via the existing `zerolog` logger at `info` (success) or `warn` (rejection):

```
{"level":"info","request_id":"01J...","jti":"<uuid>","org_id":42,"path":"/api/v1/orgs/me","msg":"api key auth success"}
{"level":"warn","request_id":"01J...","reject_reason":"revoked","path":"/api/v1/orgs/me","msg":"api key auth rejected"}
```

Never logged: the raw JWT, `JWT_SECRET`, or any field that could be combined to reconstruct a token. `jti` is safe to log (it's the revocation handle, useless without signature).

The fire-and-forget `last_used_at` update logs at `error` level on failure so we notice if writes to `api_keys` start failing, but does not propagate the error to the caller.

---

## PR sequencing

One PR, four commits, on branch `feature/tra-393-api-key-management`:

1. `feat(tra-393): api_keys migration, RLS, partial index`
2. `feat(tra-393): APIKeyAuth + RequireScope middleware, GET /orgs/me`
3. `feat(tra-393): admin CRUD handlers for org API keys`
4. `feat(tra-393): APIKeysScreen, OrgSettingsScreen link, apiKeysApi client`

Each commit compiles and tests green on its own; the PR preview exercises the full stack. Merge via merge commit (per project convention — never squash).

---

## Open questions

*(None open — RBAC helper identified as `middleware.RequireOrgAdmin`; route shape aligned with existing `/api/v1/orgs/{id}/...` conventions.)*

---

## Decisions log

### A. Scope

- **A-1.** TRA-393 ships middleware + migration + admin UI + `GET /api/v1/orgs/me` canary. Public business endpoints are TRA-396.
- **A-2.** Rate limiting deferred to a separate sub-issue, not TRA-393.
- **A-3.** `ErrRateLimited` error type stub added to align the catalog with TRA-392.

### B. Data model

- **B-1.** Table schema verbatim from TRA-392 §Authentication. No `key_hash`, no `key_prefix` — JWTs are self-validating.
- **B-2.** Partial index `api_keys(org_id) WHERE revoked_at IS NULL` for the list query.
- **B-3.** Soft cap at 10 active keys per org, enforced in handler, returned as 409.

### C. Authentication

- **C-1.** Two separate middlewares (`Auth`, `APIKeyAuth`); shared `JWT_SECRET`; discriminated by `iss` claim.
- **C-2.** `last_used_at` updated on every authenticated request; fire-and-forget.
- **C-3.** JWT exp and DB `expires_at` are both checked (defense in depth).
- **C-4.** Rotation is create-new-revoke-old; no in-place rotation.

### D. UI

- **D-1.** New top-level screen at `#api-keys`, linked from `OrgSettingsScreen`. No tab refactor of the existing settings screen.
- **D-2.** Scope selector is three per-resource dropdowns (None / Read / Read + Write), mapping to the five stored scope strings.
- **D-3.** Expiry picker offers Never / 30d / 90d / 1y / Custom date.
- **D-4.** Show-once modal enforces acknowledgement (copy-or-dwell) before dismissal.
- **D-5.** React Query for server state; no dedicated Zustand store.

### E. Delivery

- **E-1.** Single PR, four commits, merge-commit not squash.
- **E-2.** Integration tests hit a real test DB; no mocks for auth or DB layers.
