# TRA-454 — API Contract Polish (platform slice)

**Linear:** https://linear.app/trakrf/issue/TRA-454
**Date:** 2026-04-23
**Branch:** `miks2u/tra-454-api-contract-polish`
**Status:** Design approved, ready for plan

## Scope

Platform (backend + frontend) items from TRA-454. Docs-only items (C1, C6 narrative, C7) are out of scope for this session and will be handled in a separate trakrf-docs checkout.

**In scope:**

- **C3** — Case-insensitive `Bearer` scheme match in auth middleware.
- **C4** — Wrap `POST /api/v1/orgs/{id}/api-keys` response in `{"data": {...}}` and adapt the frontend.
- **C5** — Clean validation errors on the org api-keys handlers (and a collateral fix that benefits the whole `orgs` handler package).

**Out of scope:**

- C1 "RFC 7807" phrasing removal from `docs/api/errors`.
- C6 documenting `surrogate_id` / `path` / `depth` in the public docs site.
- C7 identifier case-sensitivity note in `docs/api/resource-identifiers`.
- Anything RFC 7807 body-shape compliant (breaking change, not in this ticket).
- Rate-limit header bug (TRA-453).

## C3 — Bearer scheme case-insensitive

The `Bearer` token scheme must match case-insensitively per RFC 6750 §2.1 / RFC 7235 §2.1. Three middlewares currently do a strict `!= "Bearer"` comparison; all three get fixed.

| File | Line | Function |
|---|---|---|
| `backend/internal/middleware/middleware.go` | 149 | `Auth` (session JWT) |
| `backend/internal/middleware/apikey.go` | 43 | `APIKeyAuth` |
| `backend/internal/middleware/either_auth.go` | 37 | `EitherAuth` |

### Change

Each comparison moves from:

```go
if len(parts) != 2 || parts[0] != "Bearer" {
```

to:

```go
if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
```

The token half (`parts[1]`) stays case-preserving — JWTs are case-sensitive.

`middleware.go:148` uses `strings.Split` where the other two use `strings.SplitN`. Leave `strings.Split` alone; the existing `len(parts) != 2` guard already rejects weird payloads, and unifying the split mechanism is out of scope for a contract-polish ticket.

### Tests

One table-driven subtest per middleware covering:

- `Bearer <token>` → passes
- `bearer <token>` → passes
- `BEARER <token>` → passes
- `BeArEr <token>` → passes
- `Basic <token>` → 401
- `Token <token>` → 401
- missing header → 401 (regression)
- `"Bearer"` alone (no token) → 401 (regression)

If per-middleware test files do not yet exist, add minimal ones scoped to header parsing. No need to exercise full token-validation paths for this ticket.

## C5 — Clean validation errors on org api-keys

The `CreateAPIKey` handler leaks two kinds of raw error text:

1. Raw validator output with Go struct field names on `validate.Struct` failure.
2. Raw decoder error strings (`"EOF"`, `"invalid character ..."`) on `json.Decode` failure.

Both get replaced. A collateral change registers `JSONTagNameFunc` on the orgs-package validator so any other handler in that package that later adopts `RespondValidationError` inherits the JSON-name behavior.

### 5a. Register JSON tag-name function on the orgs validator

`backend/internal/handlers/orgs/orgs.go:19`

From:

```go
var validate = validator.New()
```

To:

```go
var validate = func() *validator.Validate {
    v := validator.New()
    v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
    return v
}()
```

Pattern copied from `backend/internal/handlers/assets/assets.go:22-24`.

Effect: any future `RespondValidationError` call in the `orgs` package reports JSON field names (`name`, `scopes`) instead of Go struct names. No behavior change for handlers that currently don't call the translator — they still leak as before, and this ticket does not expand scope to fix the rest of the package. That belongs in a separate cleanup ticket.

### 5b. Use the shared translator on validator failure

`backend/internal/handlers/orgs/api_keys.go:57-60`

From:

```go
if err := validate.Struct(req); err != nil {
    httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
        "Validation failed", err.Error(), reqID)
    return
}
```

To:

```go
if err := validate.Struct(req); err != nil {
    httputil.RespondValidationError(w, r, err, reqID)
    return
}
```

Matches `backend/internal/handlers/assets/assets.go:88-91`. The response envelope now includes a `fields[]` array with per-field `code` / `message` / `params`.

### 5c. Strip decoder error text

`backend/internal/handlers/orgs/api_keys.go:52-55`

From:

```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
        "Invalid JSON", err.Error(), reqID)
    return
}
```

To:

```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
        "Invalid JSON body", "", reqID)
    return
}
```

Status and error code unchanged; the `detail` field is no longer populated with Go-runtime text.

### 5d. Tests

Handler-level integration tests covering:

- Empty body → 400, `code: bad_request`, `message: "Invalid JSON body"`, `detail` empty (or absent), no `"EOF"` substring anywhere in the response.
- `{}` (valid JSON, missing required fields) → 400, `code: validation_failed`, `fields[].field` values drawn from JSON names (`name`, `scopes`, not `Name`, `Scopes`), `fields[].code` values like `required`.
- `{"name": "x", "scopes": ["not-a-real-scope"]}` → 400 via the existing custom-scope branch (`api_keys.go:62-68`). Kept intentionally — the ticket does not ask for a `fields[]` envelope on that branch, and its message is already clean.

## C4 — Wrap `POST /orgs/{id}/api-keys` response

Single-resource responses are inconsistent: `GET /api/v1/assets/{id}` returns `{"data": {...}}` but `POST /orgs/{id}/api-keys` returns an unwrapped object. Wrap the POST to match.

### 4a. Backend — wrap the response

`backend/internal/handlers/orgs/api_keys.go:107`

From:

```go
httputil.WriteJSON(w, http.StatusCreated, resp)
```

To:

```go
httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
```

This follows the pattern already in use at `api_keys.go:152` (`ListAPIKeys`).

### 4b. Swagger annotation

`backend/internal/handlers/orgs/api_keys.go:26`

From:

```
// @Success 201 {object} apikey.APIKeyCreateResponse
```

To:

```
// @Success 201 {object} map[string]any "data: apikey.APIKeyCreateResponse"
```

Matches the ListAPIKeys annotation style at line 116. Swagger output is regenerated per the project's existing build steps; the plan will confirm the exact command.

### 4c. Frontend — unwrap inside the API client module

`frontend/src/lib/api/apiKeys.ts:14-17`

From:

```ts
create: async (orgId: number, req: CreateAPIKeyRequest): Promise<APIKeyCreateResponse> => {
  const resp = await apiClient.post<APIKeyCreateResponse>(`/orgs/${orgId}/api-keys`, req);
  return resp.data;
},
```

To:

```ts
create: async (orgId: number, req: CreateAPIKeyRequest): Promise<APIKeyCreateResponse> => {
  const resp = await apiClient.post<{ data: APIKeyCreateResponse }>(`/orgs/${orgId}/api-keys`, req);
  return resp.data.data;
},
```

The public return type (`Promise<APIKeyCreateResponse>`) is unchanged. No downstream hook or component is affected.

### 4d. Frontend test fixtures

`frontend/src/lib/api/apiKeys.test.ts:24-36`

Update the mocked HTTP body for the `create` test(s) from the flat shape to `{ data: <existing fixture> }`. Verify the test still asserts against the unwrapped `APIKeyCreateResponse` (no changes to assertions, just the mocked payload).

### 4e. Backend test

A handler-level test on `CreateAPIKey` (new or existing) asserts the 201 response body is `{"data": {"key": "...", "id": ..., "name": ..., "scopes": [...], "created_at": "...", "expires_at": ...}}` — in particular, `key` must live at `body.data.key`, not `body.key`.

## Verification

Tests (run from project root via `just`):

- `just backend test ./internal/middleware/...` — Bearer casing matrix.
- `just backend test ./internal/handlers/orgs/...` — validation envelope + response wrap.
- `just frontend test` — updated apiKeys mocks pass.
- `just validate` — lint + full suite green.

Manual sanity: after backend changes, `curl -H 'Authorization: bearer <jwt>' ...` should authenticate.

## Definition of done (platform slice)

- Bearer scheme matches case-insensitively in all three middlewares.
- `POST /orgs/{id}/api-keys` returns `{"data": {...}}`.
- Key-creation empty body and validator failures return clean, JSON-named messages with no Go error text.
- Frontend continues to work unchanged from the caller's perspective; tests green.
- Integration tests added for Bearer casing, validator cleanup, and response wrapping.

## Out-of-scope follow-ups

- C1 remove "RFC 7807" phrasing from `docs/api/errors` — trakrf-docs session.
- C6 document `path` / `depth` + note on `surrogate_id` — trakrf-docs session.
- C7 identifier case-sensitivity note in `docs/api/resource-identifiers` — trakrf-docs session.
- Apply the same validator-translation cleanup to the rest of the `orgs/` handlers (`me.go`, `invitations.go`, `members.go`, `orgs.go`) — separate ticket.
- Unify decode-error handling across the backend (generic `httputil.DecodeJSONBody` helper) — separate ticket if we want it.
