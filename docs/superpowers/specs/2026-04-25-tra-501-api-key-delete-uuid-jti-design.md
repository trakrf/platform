# TRA-501: API key DELETE accepts UUID `jti`

**Status:** Design approved 2026-04-25
**Linear:** [TRA-501](https://linear.app/trakrf/issue/TRA-501/api-key-delete-accept-uuid-jti-or-document-idjti-translation)
**Source:** BB9 FINDINGS.md finding #6 (eval run 2026-04-24)
**Related:** TRA-466 (key management public promotion), TRA-415 (internal/public classification), TRA-480 (OpenAPI follow-ups)

## Problem

`DELETE /api/v1/orgs/{orgId}/api-keys/{key_id}` accepts only the integer surrogate `id`, but the UI displays the UUID `jti` next to each key as the visible disambiguator. A customer who has noted "the key with UUID `12dc6ca3-…`" must re-list `GET /api/v1/orgs/{orgId}/api-keys` to translate the UUID to an integer before they can revoke it.

A second, related gap: `POST /api/v1/orgs/{orgId}/api-keys` does not return `jti` in its response body. The UUID is only available by decoding the JWT or by issuing a follow-up GET. Anyone who notes the UUID from the create flow today is implicitly decoding the JWT.

## Goals

1. Accept either an integer surrogate `id` or a UUID `jti` in the DELETE path param.
2. Make `jti` first-class on the create-flow response so customers do not need a follow-up GET or JWT decode to learn the UUID.
3. Document both forms in the OpenAPI spec.

## Non-goals

- Changing the surrogate `id` to UUID across the board. Existing scripts must keep working.
- Removing the integer `id` from any response.
- Updating the customer-facing docs site (trakrf-docs). That ships in a separate PR after this merges and deploys to prod, per the "ship docs behind backend reality" rule.

## External surface

### DELETE `/api/v1/orgs/{orgId}/api-keys/{key_id}`

`key_id` accepts either form:

- Integer surrogate, e.g. `1561818033` — existing behavior, preserved.
- UUID `jti`, e.g. `12dc6ca3-7b08-4548-ae18-d9f59eb033e1` — new.

Both forms revoke the same row, return the same `204 No Content`, and are scoped to `{orgId}`. A UUID belonging to a different org returns `404` (matches the integer-id behavior). Already-revoked or not-found returns `404`. Bad input (neither valid UUID nor integer) returns `400` with title `"Invalid key id"` (unchanged message).

### POST `/api/v1/orgs/{orgId}/api-keys`

Response body gains `jti`:

```json
{
  "data": {
    "id": 1561818033,
    "jti": "12dc6ca3-7b08-4548-ae18-d9f59eb033e1",
    "key": "<JWT>",
    "name": "...",
    "scopes": ["..."],
    "created_at": "...",
    "expires_at": null
  }
}
```

Purely additive — no field removals or renames.

### GET (list) — unchanged

`APIKeyListItem` already includes `jti`. No change.

## Components / files changed

### Handler — `backend/internal/handlers/orgs/api_keys.go`

`RevokeAPIKey()` (currently at line 208): replace the single `strconv.Atoi` with a dispatch:

1. `uuid.Parse(rawKeyID)` → if ok, call `storage.RevokeAPIKeyByJTI(ctx, orgID, jti)`.
2. Else `strconv.Atoi(rawKeyID)` → if ok, call existing `storage.RevokeAPIKey(ctx, orgID, intID)`.
3. Else `400 "Invalid key id"`.

UUID-first because `uuid.Parse` is stricter than `strconv.Atoi` and rejects bare integers cleanly. The two formats are syntactically disjoint (UUIDs always have hyphens at fixed positions; integers never do), so the dispatch is unambiguous.

`CreateAPIKey()` handler: include `jti` in the `APIKeyCreateResponse`. The row already has it — thread it through.

### Model — `backend/internal/models/apikey/apikey.go`

`APIKeyCreateResponse` (currently at line 40): add

```go
JTI string `json:"jti"`
```

### Storage — `backend/internal/storage/apikeys.go`

New function:

```go
func (s *Storage) RevokeAPIKeyByJTI(ctx context.Context, orgID int, jti uuid.UUID) error
```

Same shape as existing `RevokeAPIKey`, just `WHERE org_id = $1 AND jti = $2 AND revoked_at IS NULL`. Returns `ErrAPIKeyNotFound` on no rows.

The existing un-scoped `GetAPIKeyByJTI` (used by JWT middleware before org context exists) is **not** reused. We want the org filter for authorization, and a new function keeps the read-vs-revoke responsibilities cleanly split.

### OpenAPI — `backend/internal/handlers/swaggerspec/openapi.public.yaml`

- DELETE `key_id` path param: `type: integer` → `type: string`, with description `"Either the integer surrogate id or the UUID jti."`.
- `APIKeyCreateResponse` schema: add `jti` property (`type: string`).
- `APIKeyListItem` schema: no change (already documents `jti`).

## Data flow

```
DELETE /api/v1/orgs/123/api-keys/12dc6ca3-7b08-4548-ae18-d9f59eb033e1
       ↓
   chi router → RevokeAPIKey handler
       ↓
   rawKeyID := chi.URLParam(r, "key_id")
       ↓
   if jti, err := uuid.Parse(raw); err == nil →
       storage.RevokeAPIKeyByJTI(ctx, orgID, jti)
   else if id, err := strconv.Atoi(raw); err == nil →
       storage.RevokeAPIKey(ctx, orgID, id)
   else →
       400 "Invalid key id"
       ↓
   UPDATE api_keys
     SET revoked_at = now()
     WHERE org_id = $1 AND <id|jti> = $2 AND revoked_at IS NULL
       ↓
   0 rows → ErrAPIKeyNotFound → 404
   1 row  → 204 No Content
```

POST flow is unchanged in shape; the handler just maps `row.JTI` into the response struct.

## Authorization model

Unchanged. Both forms are scoped to `(orgID, identifier)`. A leaked or guessed `jti` belonging to a different org returns 404 — never 403, never leaks existence.

## Error handling

| Input | Result |
|---|---|
| Valid UUID, key in this org, active | `204 No Content` |
| Valid UUID, key in this org, already revoked | `404 Not Found` |
| Valid UUID, key in a different org | `404 Not Found` |
| Valid UUID, no such key | `404 Not Found` |
| Valid integer (any of the above) | Unchanged from today |
| Neither valid UUID nor integer | `400 Bad Request`, title `"Invalid key id"` |
| No `keys:admin` scope | `403` from middleware (unchanged) |
| Unauthenticated | `401` from middleware (unchanged) |

Error message text stays `"Invalid key id"` — broad enough to cover both forms; no need to leak which format was expected.

### Self-revocation edge case

A key revoking itself by its own `jti` behaves identically to revoking by integer `id`. Authentication middleware runs before the handler, so by the time we revoke, the request is already authenticated. Subsequent calls 401. No special handling — covered by adding a `_ByJTI` variant of the existing self-revoke test.

## Testing

### Handler tests — `backend/internal/handlers/orgs/api_keys_integration_test.go`

New:

- `TestRevokeAPIKey_ByJTI` — happy path: create key, revoke by UUID, verify 204 and that the key is now invalid.
- `TestRevokeAPIKey_ByJTI_CrossOrgReturns404` — UUID belongs to different org → 404 (mirrors `_CrossOrgReturns404`).
- `TestRevokeAPIKey_ByJTI_AlreadyRevoked` — second revoke by jti → 404.
- `TestRevokeAPIKey_InvalidFormat` — neither UUID nor integer (e.g. `"foo"`, `"12-not-a-uuid"`) → 400 `"Invalid key id"`.
- `TestRevokeAPIKey_KeyRevokesItself_ByJTI` — variant of existing self-revoke test using the JWT's own `jti`.
- `TestCreateAPIKey_ResponseIncludesJTI` — assert `jti` in POST response body and that it matches the JWT's `sub`/`jti` claim.

### Storage tests — `backend/internal/storage/apikeys_integration_test.go`

New:

- `TestAPIKeyStorage_RevokeByJTI` — basic happy path.
- `TestAPIKeyStorage_RevokeByJTIReturnsNotFoundForCrossOrg` — mirrors existing cross-org test.
- `TestAPIKeyStorage_RevokeByJTI_AlreadyRevoked` — returns `ErrAPIKeyNotFound`.

### OpenAPI

No automated test. Visual review of the rendered spec is sufficient.

## Acceptance criteria (from ticket)

- DELETE accepts either an integer surrogate or a UUID `jti`. ✓ (handler dispatch)
- Both forms revoke the same key with the same response. ✓ (storage layer; both 204)
- OpenAPI parameter type updated. ✓ (`integer` → `string` with description)
- Docs updated with a brief "key identifiers" explainer. **Deferred to follow-up trakrf-docs PR; tracked as a comment on TRA-501.**

## Follow-ups

- **trakrf-docs PR** — opened after this merges and deploys to prod. Adds a short "How to revoke a key — id vs jti" explainer to the customer-facing docs site. Reference: feedback memory `feedback_docs_behind_backend.md`. Leave a one-line comment on TRA-501 noting this follow-up.

## Out of scope

- Changing the surrogate `id` → UUID across the board. Would break existing scripts.
- Removing the integer `id` from any response.
- Wider deprecation of integer ids on public surfaces (post-v1 territory).
