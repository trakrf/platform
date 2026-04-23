# TRA-466: Promote API key management endpoints to public API surface

**Linear:** [TRA-466](https://linear.app/trakrf/issue/TRA-466/promote-api-key-management-endpoints-to-public-api-surface)
**Parent:** TRA-210
**Related:** TRA-415 (private endpoints classification), TRA-393 (api-key management impl)

## Problem

`POST/GET/DELETE /api/v1/orgs/{id}/api-keys` exist and work, but they are tagged `internal` in swaggo and therefore excluded from `openapi.public.json`. The public docs site tells integrators to "request a feature" if they want programmatic key rotation.

This blocks TeamCentral — our first external public-API customer — from building a production-grade iPaaS connector, because their connector needs to rotate its own API key on the published 90-day cadence without human intervention. It also blocks CI/CD pipelines and IaC (Terraform, Pulumi) workflows that want to provision keys.

Black-box evaluation #5 (2026-04-23) additionally observed that session JWTs are accepted as Bearer tokens on public endpoints with no scope enforcement — effectively unscoped access for the JWT's 1h lifetime. This is the intended behavior for SPA traffic, but it's invisible to integrators reading the public docs and deserves a callout.

## Goal

Make programmatic API key rotation a supported, documented part of the public API, gated by a new `keys:admin` scope, without breaking the existing SPA flow.

## Non-goals

- OAuth2 client credentials flow
- Service account model
- Webhook-based key expiry notifications
- Automated OpenAPI contract testing in CI (separate effort)

## Design

### Authentication model

Two principal types can reach key management endpoints:

1. **Session JWT** (SPA) — carries `UserClaims`. Must be an org admin of `{id}`. No scope concept applies. Unchanged from today.
2. **API key JWT** (integrations) — carries `APIKeyPrincipal`. Must have `org_id == {id}` and `keys:admin` in `scopes`.

A new middleware `RequireOrgAdminOrKeysAdmin` handles both cases in one place. `EitherAuth` upstream already dispatches to the correct auth chain based on JWT issuer, so both principal types land on the context in a uniform way.

#### `keys:admin` scope semantics

- Pure key-management. Does NOT grant `assets:read`, `locations:write`, or any data access.
- A `keys:admin` key CAN mint another `keys:admin` key. This is deliberate: TeamCentral-class integrations must be able to self-rotate without a human in the loop. Leak containment relies on short `expires_at` + the existing revocation audit trail, not on crippling rotation.
- Added to `models/apikey.ValidScopes`.

#### Session JWT behavior on public endpoints

Documented, not changed. Session tokens (1h lifetime) bypass `RequireScope` on all public read/write endpoints — this is the SPA's auth model. The public docs will call this out so integrators don't try to build long-running automation around session tokens.

### Request flow

```
Request
 └─> EitherAuth                  (existing, internal/middleware/either_auth.go)
      ├─ iss=session  -> Auth       -> UserClaims on ctx
      └─ iss=api-key  -> APIKeyAuth -> APIKeyPrincipal on ctx
 └─> RequireOrgAdminOrKeysAdmin  (NEW, internal/middleware/org_admin_or_keys_admin.go)
      1. Parse {id} from URL.
      2. If UserClaims present: delegate to existing org-admin check. 403 if not admin.
      3. Else if APIKeyPrincipal present:
            require principal.OrgID == {id} AND "keys:admin" in principal.Scopes.
            403 otherwise.
      4. Else: 401 (defensive — EitherAuth should have short-circuited).
 └─> CreateAPIKey | ListAPIKeys | RevokeAPIKey
```

### Handler changes

`CreateAPIKey` resolves the creator from whichever principal is on the context:

- `UserClaims` → `creator = Creator{UserID: &claims.UserID}`
- `APIKeyPrincipal` → `creator = Creator{KeyID: &parentKeyID}` where `parentKeyID` is looked up from `principal.JTI`

Scope validation (all requested scopes appear in `ValidScopes`) is unchanged — `keys:admin` is simply an allowed value now.

`ListAPIKeys` / `RevokeAPIKey` have no principal-type-dependent logic; they just need to be reachable via both auth paths, which the middleware swap handles.

### Storage changes

`storage.APIKeys.Create` signature change:

```go
// Before
Create(ctx, orgID int, name string, scopes []string, createdBy int, expiresAt *time.Time) (*APIKey, error)

// After
Create(ctx, orgID int, name string, scopes []string, creator Creator, expiresAt *time.Time) (*APIKey, error)

type Creator struct {
    UserID *int
    KeyID  *int
}
```

Exactly one of `UserID` / `KeyID` must be non-nil; enforced at the storage layer with a precondition and again at the DB with a `CHECK` constraint. `List` / `GetByJTI` return both columns.

### Schema migration

`000034_api_keys_created_by_nullable.up.sql` (exact number confirmed at implementation time via `ls migrations/`):

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

Down migration refuses to run if any row has `created_by IS NULL`, surfacing the problem rather than silently discarding key-minted-key rows:

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

Existing rows all have `created_by NOT NULL`; they pass the new `CHECK` unchanged. No backfill.

### Response shape

```json
{
  "id": 123,
  "jti": "...",
  "org_id": 456,
  "name": "TeamCentral prod",
  "scopes": ["assets:read", "locations:read"],
  "created_by": 789,
  "created_by_key_id": null,
  "created_at": "...",
  "expires_at": "...",
  "last_used_at": null,
  "revoked_at": null
}
```

Exactly one of `created_by` / `created_by_key_id` is non-null. SPA resolves `created_by` to a user name via the existing users endpoint; when `created_by_key_id` is non-null, SPA renders "Key: {name}" using the name already present in the list response for that key row.

This is an **additive** response change — public API has no existing external consumers of this endpoint (that's what this ticket unlocks), so field additions are free. The SPA is internal and updated in the same PR.

### Frontend changes

`components/apikeys/ScopeSelector.tsx` — add a fourth row:

```
Key management:  [ None | Admin ]
```

Maps to `keys:admin` scope. Updates the `Scope` type union in `types/apiKey.ts`.

`components/APIKeysScreen.tsx` — renders the `keys:admin` scope label ("Key admin") consistent with existing scope rendering. Handles the new `created_by_key_id` column in the keys list (displays "Key: {name}" when user ID is null).

### Router changes

`internal/handlers/orgs/orgs.go` lines 282–285:

```go
// Before
r.With(middleware.RequireOrgAdmin(store)).Post("/api-keys", h.CreateAPIKey)
r.With(middleware.RequireOrgAdmin(store)).Get("/api-keys", h.ListAPIKeys)
r.With(middleware.RequireOrgAdmin(store)).Delete("/api-keys/{keyID}", h.RevokeAPIKey)

// After
r.With(middleware.RequireOrgAdminOrKeysAdmin(store)).Post("/api-keys", h.CreateAPIKey)
r.With(middleware.RequireOrgAdminOrKeysAdmin(store)).Get("/api-keys", h.ListAPIKeys)
r.With(middleware.RequireOrgAdminOrKeysAdmin(store)).Delete("/api-keys/{keyID}", h.RevokeAPIKey)
```

Other uses of `RequireOrgAdmin` remain unchanged; grep before implementing to confirm none of them also need the new combined middleware.

### Swaggo annotations

Three one-line flips:

```
// Before
// @Tags api-keys,internal

// After
// @Tags api-keys,public
```

On `CreateAPIKey` (line 20), `ListAPIKeys` (line 110), `RevokeAPIKey` (line 155). Regenerate with `just backend api-spec`.

Verify that `postprocess.go` renders the `keys:admin` scope cleanly in the public spec's security scheme block; if it doesn't enumerate scopes there, extend it to do so. (Check at implementation time — current state unknown.)

## PR sequencing

Per memory policy: docs ship *behind* backend reality, and docs changes happen in a separate checkout.

1. **PR 1** (platform repo, branch `miks2u/tra-466-...`): backend + SPA changes + integration tests. Merge to main.
2. **PR 2** (trakrf-docs repo, separate worktree/sibling checkout — NOT `/home/mike/trakrf-docs`): docs updates, using the `openapi.public.{json,yaml}` artifact from the merged-main build of PR 1. Opened only after PR 1 is merged.

## Documentation changes (PR 2)

`docs/api/authentication.md`:
- Add "Programmatic key rotation" section with cURL examples for the create/list/revoke flow using a `keys:admin` key.
- Add `keys:admin` row to the scopes table with description.
- Add "Session JWTs on public endpoints" subsection noting that session tokens bypass scope checks, are SPA-oriented, and have a 1h lifetime — not suitable for long-running automation.

`docs/api/private-endpoints.md`:
- Remove the three api-keys rows from the Internal section.
- Revise the "API-key management is Internal" explainer to describe the new public model (keys:admin scope, programmatic rotation supported).

`static/api/openapi.{json,yaml}`:
- Replace with the artifacts from the merged-main build.

## Testing

### Backend integration (`//go:build integration`, real PG via `testutil.SetupTestDB`)

`internal/middleware/api_keys_admin_test.go` (NEW):

- `TestRequireOrgAdminOrKeysAdmin_SessionAdmin` — 200
- `TestRequireOrgAdminOrKeysAdmin_SessionMember` — 403
- `TestRequireOrgAdminOrKeysAdmin_APIKeyWithKeysAdmin` — 200
- `TestRequireOrgAdminOrKeysAdmin_APIKeyWithoutKeysAdmin` — 403 (e.g. key has `assets:read` only)
- `TestRequireOrgAdminOrKeysAdmin_APIKeyWrongOrg` — 403
- `TestRequireOrgAdminOrKeysAdmin_NoPrincipal` — 401

`internal/handlers/orgs/api_keys_integration_test.go` (EXTEND):

- `TestCreateAPIKey_ByAPIKeyPrincipal` — POST with `keys:admin` key → 201; response has `created_by: null`, `created_by_key_id: <parent row id>`
- `TestCreateAPIKey_KeysAdminMintsKeysAdmin` — self-rotation path works
- `TestListAPIKeys_ByAPIKeyPrincipal` — 200
- `TestRevokeAPIKey_ByAPIKeyPrincipal` — 204
- `TestRevokeAPIKey_KeyRevokesItself` — a `keys:admin` key revoking its own JTI succeeds; next call with that key 401s

`internal/storage/apikeys_integration_test.go` (EXTEND):

- `TestCreate_WithCreatedByKeyID` — roundtrip via storage layer
- `TestCreate_ViolatesExclusivityCheck` — both creator columns set at once → PG `CHECK` violation surfaced as error

### Frontend unit

`components/apikeys/ScopeSelector.test.tsx` (EXTEND) — new "Key management" row with `none/admin` options; emits `keys:admin` when set to admin.

`components/APIKeysScreen.test.tsx` (EXTEND) — displays `keys:admin` scope label; displays "Key: {name}" when `created_by_key_id` is non-null.

### Not tested in this PR

- OpenAPI contract tests — not diff-tested in CI today; separate effort.
- Playwright E2E for the full mint-via-UI then rotate-via-API flow — manual pre-merge verification.

## Risks & open points

- **`RequireOrgAdmin` usage elsewhere:** grep before implementing. Any other route that should accept `keys:admin` stays on the old middleware for this ticket unless explicitly in scope. Default answer: other routes stay session-only.
- **Scope type generation:** if `types/apiKey.ts` is generated from the OpenAPI spec rather than hand-written, the fix is in the generator config. Confirm at implementation.
- **`postprocess.go` scope enumeration:** unknown whether the public spec's security scheme already lists scope names. If it does, `keys:admin` needs to be added there. If it doesn't enumerate scopes, no change needed.
- **Self-referencing FK in down migration:** the `created_by_key_id REFERENCES api_keys(id)` self-FK is harmless for inserts (parents pre-exist) and `ON DELETE CASCADE` is not used — deleting a parent key with children is not a supported path, and we soft-delete (`revoked_at`) anyway.

## Definition of done

- [ ] `keys:admin` added to `ValidScopes`
- [ ] Migration 00003X created, up and down tested
- [ ] `RequireOrgAdminOrKeysAdmin` middleware + tests landed
- [ ] Three `@Tags` flipped; `openapi.public.json` regenerated and contains all three routes
- [ ] `storage.APIKeys.Create` signature updated; handler resolves creator from principal type
- [ ] `CHECK` constraint enforces exactly-one creator
- [ ] Response exposes `created_by` and `created_by_key_id`
- [ ] SPA `ScopeSelector` offers "Key management" with `none/admin` levels
- [ ] SPA renders keys minted by other keys distinguishably from user-minted
- [ ] Integration tests listed above all pass
- [ ] PR 1 merged to main
- [ ] PR 2 (docs) opened against trakrf-docs from separate checkout, using openapi artifacts from merged-main build
