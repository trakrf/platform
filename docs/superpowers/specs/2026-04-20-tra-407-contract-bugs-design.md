# TRA-407 — Public API Contract Bug Fixes (Design)

**Date:** 2026-04-20
**Linear issue:** [TRA-407](https://linear.app/trakrf/issue/TRA-407/public-api-contract-bugs-shared-error-envelope-write-path-conflicts)
**Parent epic:** TRA-210
**Blocks:** TRA-416 (error-codes.md rewrite)
**Related:** TRA-395, TRA-394, TRA-396, TRA-408, TRA-409

## Goal

Close the gap between what `/docs/api/error-codes`, `/docs/api/authentication`, and `/docs/api/rate-limits` promise and what the service emits. Every item below is a customer-facing contract violation where a client written against the shipped docs will misbehave.

## Scope

Six items from the 2026-04-20 black-box API evaluation on `app.preview.trakrf.id`. Split into two PRs:

**PR #1 — runtime contract fixes** (items 1–5): behavior changes in handlers and middleware, shared error-translation helpers, ULID request IDs, 401 hygiene.

**PR #2 — OpenAPI coherence** (item 6): swaggo annotation churn plus path-param unification to `{identifier}` (real behavior change on write/child routes).

TRA-416 (docs rewrite) unblocks after both PRs ship.

## Out of scope

- Internal `/api/v1/internal/*` handlers (frontend-only). Audit during PR #1 implementation; if any leak the same way, add a line item to the plan. Otherwise they stay as-is.
- DB schema changes. None required.
- Storage-layer signature changes. Storage continues accepting surrogate IDs; the identifier→id resolution is a handler-layer concern.
- Backward compatibility. The API is pre-integration (TeamCentral hasn't wired up). Hard flip, no dual-routing, no shim.

## Architecture

**Pattern:** thin translation helpers in `internal/util/httputil` sit between the caller and the existing `WriteJSONError` renderer. Handlers call the helper; the helper normalizes the input and delegates to the renderer. No handler signature changes. No middleware restructure.

The existing envelope (RFC 7807: `{type, title, status, detail, instance, request_id}`) stays. The bugs are all about what gets *fed* to it.

## PR #1 — Runtime contract fixes (items 1–5)

### New helpers in `internal/util/httputil`

```go
// DecodeJSON decodes the request body into dst. Returns a typed
// *JSONDecodeError on malformed JSON so the caller can render a
// stable response without leaking encoding/json internals.
func DecodeJSON(r *http.Request, dst any) error

// RespondDecodeError writes a 400 with detail="Request body is not
// valid JSON". Idempotent with DecodeJSON.
func RespondDecodeError(w http.ResponseWriter, r *http.Request, err error, requestID string)

// RespondValidationError walks validator.ValidationErrors, renders
// the documented fields[] envelope. Uses JSON field names (snake_case,
// not Go struct names); maps validator tags to a finite code set.
func RespondValidationError(w http.ResponseWriter, r *http.Request, err error, requestID string)

// RespondStorageError unwraps to *pgconn.PgError via errors.As and
// switches on SQLSTATE:
//   23505 unique_violation   -> 409 conflict
//   anything else or non-pgx -> 500 internal_error
// 23503 (foreign_key_violation) intentionally not mapped here —
// the right status depends on whether the violating op was an
// insert (400/404) or a delete (409), and TRA-407's scope only
// calls out the duplicate-identifier case.
func RespondStorageError(w http.ResponseWriter, r *http.Request, err error, requestID string)

// Respond401 writes a normalized unauthorized response with
// WWW-Authenticate: Bearer realm="trakrf-api" and title="Authentication
// required". detail is caller-supplied (short canonical set).
func Respond401(w http.ResponseWriter, r *http.Request, detail, requestID string)
```

### Validator tag → code mapping

Lives in `httputil`; extensible.

| Validator tag | Code |
|---|---|
| `required`, `required_without`, `required_with` | `required` |
| `email`, `oneof`, `url`, `uuid` | `invalid_value` |
| `min` (numeric), `gte`, `gt` | `too_small` |
| `max` (numeric), `lte`, `lt` | `too_large` |
| `min` (string/slice len) | `too_short` |
| `max` (string/slice len) | `too_long` |
| unknown tag | `invalid_value` (fallback) |

### Request-ID: hex → ULID (item 3)

`middleware.generateRequestID()` swaps from `crypto/rand` 32-char hex to `github.com/oklog/ulid/v2` monotonic ULID. Header echo already works per the partial-fix note — no change there.

- Add `github.com/oklog/ulid/v2` to `go.mod`.
- Use `ulid.Monotonic(rand.Reader, 0)` as package-level generator guarded by `sync.Mutex` (the library's documented pattern).

### 401 hygiene (item 4)

Three call-sites migrate to `Respond401`:

1. `middleware/middleware.go` (session auth) — missing header, bad format, invalid/expired token.
2. `middleware/apikey.go` (API-key auth) — same plus revoked/expired key.
3. `handlers/auth/auth.go` (login) — wrong email/password.

Canonical detail strings:

| Site | detail |
|---|---|
| Missing header | `Authorization header is required` |
| Malformed header | `Authorization header must be Bearer <token>` |
| Invalid/expired bearer token | `Bearer token is invalid or expired` |
| Revoked API key | `API key has been revoked` |
| Expired API key | `API key has expired` |
| Login wrong email/password | `Invalid email or password` |

Login deliberately keeps the vague "email or password" wording (credential-stuffing defense) — it just moves under the normalized title.

`either_auth` dispatcher classifies token type before delegating; no 401 emitted from there. Unchanged.

### Handler migration (items 1, 2, 5)

Each public handler swaps three call patterns.

**Before:**
```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    httputil.WriteJSONError(w, r, 400, InvalidJSON, "Bad request", err.Error(), requestID)
    return
}
if err := validate.Struct(req); err != nil {
    httputil.WriteJSONError(w, r, 400, ValidationFailed, "...", err.Error(), requestID)
    return
}
result, err := storage.Create(ctx, req)
if err != nil {
    if strings.Contains(err.Error(), "already exists") {
        httputil.WriteJSONError(w, r, 409, Conflict, ...)
        return
    }
    httputil.WriteJSONError(w, r, 500, InternalError, ...)
    return
}
```

**After:**
```go
if err := httputil.DecodeJSON(r, &req); err != nil {
    httputil.RespondDecodeError(w, r, err, requestID)
    return
}
if err := validate.Struct(req); err != nil {
    httputil.RespondValidationError(w, r, err, requestID)
    return
}
result, err := storage.Create(ctx, req)
if err != nil {
    httputil.RespondStorageError(w, r, err, requestID)
    return
}
```

**Handlers to migrate:**

Not every handler needs all three swaps — delete and remove-identifier have no request body:

| Handler | Decode | Validate | Storage |
|---|---|---|---|
| `auth.go`: signup, login | ✓ | ✓ | n/a (login errors are 401, signup uses storage) |
| `assets.go`: create, update | ✓ | ✓ | ✓ |
| `assets.go`: delete, remove-identifier | | | ✓ |
| `assets.go`: add-identifier | ✓ | ✓ | ✓ |
| `locations.go`: create, update | ✓ | ✓ | ✓ |
| `locations.go`: delete, remove-identifier | | | ✓ |
| `locations.go`: add-identifier | ✓ | ✓ | ✓ |

The two add-identifier handlers currently return 500 on duplicate (no `"already exists"` check at all) — migration fixes them as a side effect.

Storage layer stays as-is. The `strings.Contains(err, "already exists")` matches in handlers disappear — `RespondStorageError` classifies by SQLSTATE via `errors.As` to `*pgconn.PgError`.

## PR #2 — OpenAPI coherence (item 6)

### 6a — Add `@ID` annotations (`resource.verb` convention)

| Operation | `@ID` |
|---|---|
| `GET /api/v1/assets` | `assets.list` |
| `GET /api/v1/assets/{identifier}` | `assets.get` |
| `GET /api/v1/assets/{identifier}/history` | `assets.history` |
| `POST /api/v1/assets` | `assets.create` |
| `PUT /api/v1/assets/{identifier}` | `assets.update` |
| `DELETE /api/v1/assets/{identifier}` | `assets.delete` |
| `POST /api/v1/assets/{identifier}/identifiers` | `assets.identifiers.add` |
| `DELETE /api/v1/assets/{identifier}/identifiers/{value}` | `assets.identifiers.remove` |
| `GET /api/v1/locations` | `locations.list` |
| `GET /api/v1/locations/current` | `locations.current` |
| `GET /api/v1/locations/{identifier}` | `locations.get` |
| `GET /api/v1/locations/{identifier}/ancestors` | `locations.ancestors` |
| `GET /api/v1/locations/{identifier}/children` | `locations.children` |
| `GET /api/v1/locations/{identifier}/descendants` | `locations.descendants` |
| `POST /api/v1/locations` | `locations.create` |
| `PUT /api/v1/locations/{identifier}` | `locations.update` |
| `DELETE /api/v1/locations/{identifier}` | `locations.delete` |
| `POST /api/v1/locations/{identifier}/identifiers` | `locations.identifiers.add` |
| `DELETE /api/v1/locations/{identifier}/identifiers/{value}` | `locations.identifiers.remove` |

### 6b — Path-param unification to `{identifier}` (behavior change)

Writes and child routes flip `{id}` → `{identifier}`:

- Route registrations change (`chi.Route("/{id}", …)` → `"/{identifier}"`).
- Handlers read the URL param as `identifier`, resolve to the internal surrogate via the same lookup reads use, then call existing storage methods.
- Storage API signatures unchanged (they still accept the surrogate internally). Only the handler layer does identifier→id resolution.
- OpenAPI path params renamed to match.

**Identifier-child URL shape:** `DELETE /{resource}/{identifier}/identifiers/{identifierId}` becomes `DELETE /{resource}/{identifier}/identifiers/{value}`. Assumes `(parent_id, type, value)` or `(parent_id, value)` is the unique key on the child identifier row — confirmed during implementation. If `(parent_id, type, value)` only, disambiguate with `?type=…` query param.

### 6c — Success codes: `202` → `200`/`204`

Swaggo annotations flip; no server behavior change (these endpoints already respond synchronously, the annotations were wrong):

- `PUT /assets/{identifier}`, `PUT /locations/{identifier}` → `200` (returns updated entity)
- `DELETE /assets/{identifier}`, `DELETE /locations/{identifier}`, `DELETE /.../identifiers/…` → `204` (empty body)
- `POST /.../identifiers` (append) → `201` if returning the new identifier row, `204` if not
- `GET /locations/{identifier}/ancestors|children|descendants` → `200`

### Regen

After annotation edits, run the existing swaggo generator target (`just backend swag` or equivalent) to regenerate `backend/docs/docs.go` and `backend/internal/handlers/swaggerspec/openapi.internal.yaml`. Regen output is part of the PR.

## Testing

### PR #1 — helper unit tests in `httputil`

- `DecodeJSON`: valid body decodes; malformed body returns `*JSONDecodeError`; `RespondDecodeError` emits stable detail `"Request body is not valid JSON"`.
- `RespondValidationError`: table-driven over the struct tags in use. Verifies snake_case JSON field names, tag→code map, fallback behavior, multi-field output order.
- `RespondStorageError`: fake `*pgconn.PgError` with `SQLSTATE=23505` → 409; non-pgx → 500; wrapped pgx (via `errors.As`) still classifies.
- `Respond401`: asserts `WWW-Authenticate: Bearer realm="trakrf-api"` header, title, type.

### PR #1 — end-to-end, one test per originally-broken case

- `POST /api/v1/locations` with duplicate identifier → 409 conflict.
- `POST /api/v1/auth/signup` bad body → `type=validation_error` with populated `fields[]` (snake_case names, mapped codes).
- `POST /api/v1/auth/login` malformed body → 400, detail = `"Request body is not valid JSON"`, no `invalid character 'o' in literal null` leak.
- Any 401 path → response has `WWW-Authenticate` header and title = `Authentication required`.
- Server-generated `request_id` matches ULID regex (`^[0-9A-HJKMNP-TV-Z]{26}$`).

### PR #2 — route and spec tests

- Route-level: writes and children resolve when called with `{identifier}`; confirm they 404 (not 500) on unknown identifier.
- OpenAPI JSON snapshot: every public op has an `@ID`, names match the canonical table, no path uses `{id}`, no op declares `202` as the success response.
- Identifier-remove URL: `DELETE /.../identifiers/{value}` works; behavior under ambiguous-value-across-types documented (query-param disambiguation if needed).

### Out of scope for tests

- No DB migration tests — no schema change.
- No contract change tests for internal `/api/v1/internal/*` handlers — those keep current behavior for this change.

## Rollout

1. Fresh worktree off `main` → `feature/tra-407-contract-bugs` (PR #1).
2. PR #1 lands.
3. Second worktree off updated `main` → `feature/tra-407-openapi-coherence` (PR #2).
4. PR #2 lands.
5. Unblocks TRA-416.

No DB changes. No backward-compat shim. Per project convention: always PR, never merge locally; default to merge commit (not squash).

## Open questions (resolve during implementation)

- **Identifier-remove URL keying.** Proposed `DELETE /.../identifiers/{value}`. Confirm whether child-identifier uniqueness is `(parent_id, value)` or `(parent_id, type, value)`. If the latter, route shape becomes `DELETE /.../identifiers/{value}?type=…`.
- **`WWW-Authenticate` realm string.** Proposed `Bearer realm="trakrf-api"`. If prior auth docs or infrastructure already name a realm, match that instead.
- **swaggo `@ID` collisions.** The canonical table is unique by construction, but sanity-check the generator output before committing.
- **Internal-handler audit.** During PR #1 implementation, grep for `json.Decode` / `validate.Struct` / `"already exists"` patterns on internal routes. Extend the PR only if leaks appear.
