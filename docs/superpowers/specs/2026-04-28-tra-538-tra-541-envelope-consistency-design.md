# Error envelope contract enforcement (TRA-538 + TRA-541)

**Linear**:
- [TRA-538](https://linear.app/trakrf/issue/TRA-538/fix-error-envelope-titledetail-contract-violation-in-auth-rejection) — Fix error envelope title/detail contract violation in auth-rejection layer
- [TRA-541](https://linear.app/trakrf/issue/TRA-541/http-envelope-consistency-for-405415-investigate-undocumented) — HTTP envelope consistency for 405/415 + investigate undocumented multipart claim
- Parent: TRA-537 (BB12 audit findings)

**Status**: brainstorm complete, awaiting user review.

## Why combined

Both tickets are sibling sub-issues of the BB12 audit (TRA-537), both Urgent, both modify the same error-envelope plumbing under `backend/internal/util/httputil` and `backend/internal/middleware`. They share a single regression-test surface ("every non-2xx returns a contract-shaped envelope") and a single docs-catalog update. Splitting them produces two reviews of the same code paths with mostly overlapping diffs.

## Contract reaffirmed

After this work, the error envelope contract is:

- `error.type` — machine-readable, stable. Generated clients branch on this.
- `error.title` — **fixed string per `error.type`**. Does not vary between calls. Safe to log.
- `error.detail` — variable, per-call human-readable explanation. May name the offending field, value, or sub-cause.

This is what `errors.go:41-50` already documents. The work brings the production code paths (auth, 404 catchall, 405 handler, 415 middleware) into compliance and updates the public docs to remove the conflicting "title may vary" wording.

## Scope by finding

### TRA-538 §1.2 — auth-rejection title/detail violation

**Finding**: 401 paths emitted variable strings in `title` with empty `detail`.

**Resolution**: Most 401 paths already funnel through `httputil.Respond401`, which hardcodes `title:"Authentication required"`. The remaining offender is `backend/internal/middleware/either_auth.go`, which emits four 401 responses via `httputil.WriteJSONError` directly with the variable string passed as the `title` argument:

- `either_auth.go:31` — missing Authorization header (this is the exact source of the BB12 §1.2 reproduction with `X-API-Key` set; the `EitherAuth` wrapper is on `/api/v1/assets`)
- `either_auth.go:38` — malformed Authorization header
- `either_auth.go:45` — JWT classification failure
- `either_auth.go:56` — unknown token kind

All four migrate to `httputil.Respond401(w, r, <variable string>, reqID)`. The variable string moves from the `title` argument to the `detail` argument.

The other 401-emitting sites — `apikey.go`, `middleware.go` (`Auth`), `org_admin_or_keys_admin.go` — already use `Respond401` correctly. No change.

Acceptance is met by:

1. The `either_auth.go` migration above.
2. Regression tests that assert `title` does not contain the variable substring (e.g., `"Bearer"`, the offending token, the resource name).
3. The four BB-style 401 reproductions named in BB12 §1.2 exercised end-to-end through the wired router.

### TRA-538 §1.2 — 404 title/detail violation

**Finding**: 404 paths emitted resource-specific strings in `title` (e.g., `"Asset not found"`) with empty `detail`.

**Resolution**: New helper `httputil.Respond404` paralleling `Respond401`, hardcoding `title:"Not found"`, `type:"not_found"`, `status:404`. All `http.StatusNotFound` emission sites in `backend/internal/handlers/...` and the router catchall (`router.go:225`) move to this helper. The variable string moves into `detail`.

The router catchall today emits `title:"Unknown API route: <path>"` (variable in title — same violation in a different spot). Switches to `detail:"Unknown API route: " + req.URL.Path` with fixed title.

### TRA-541 §1.10 — 405 missing envelope

**Finding**: chi's default 405 handler returns an empty body with no envelope.

**Resolution**: New helper `httputil.Respond405(w, r, requestID)` registered as chi's `r.MethodNotAllowed(handler)` on the root mux. Emits envelope with `type:"method_not_allowed"`, `status:405`, `title:"Method not allowed"`, empty `detail`. The path/method are in the access log; no useful per-call detail.

### TRA-541 §1.11 — 415 wrong type + multipart wording

**Finding**: 415 emitted `error.type:"bad_request"` (collides with real 400s) and the title named multipart, an unspecified content type.

**Resolution**: New helper `httputil.Respond415(w, r, requestID)` replaces the inline `WriteJSONError` call in `middleware.ContentType`. Emits envelope with `type:"unsupported_media_type"`, `status:415`, `title:"Unsupported media type"`, `detail:"Content-Type must be application/json"`. The multipart wording is dropped from the user-facing message — this is the POLS resolution for AI integrators working from `openapi.public.yaml` (which contains zero multipart endpoints).

The middleware's underlying `strings.HasPrefix(ct, "multipart/form-data")` allowance stays in place; the bulk CSV upload route depends on it.

### TRA-541 §1.11 hidden sub-finding — multipart investigation

**Resolution**: Confirmed during exploration. `/api/v1/assets/bulk` is registered in `assets.go:841`, uses multipart, is session-auth only, and is intentionally omitted from `openapi.public.yaml`. There is no undocumented public surface. No security finding to escalate.

The PR description records the conclusion so the audit chain has a written answer.

## Code structure

### New error types

`backend/internal/models/errors/errors.go`:

```go
ErrMethodNotAllowed  ErrorType = "method_not_allowed"
ErrUnsupportedMedia  ErrorType = "unsupported_media_type"
```

The `ErrorResponse` swagger annotation `enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error"` grows to include the two new types. The existing `x-extensible-enum=true` tells generated clients to tolerate unknown values, so this is additive for consumers.

### New helpers

All in `backend/internal/util/httputil/`:

- `Respond404(w, r, detail, requestID)` — fixed `title:"Not found"`.
- `Respond405(w, r, requestID)` — fixed `title:"Method not allowed"`, empty `detail`.
- `Respond415(w, r, requestID)` — fixed `title:"Unsupported media type"`, fixed `detail:"Content-Type must be application/json"`.

### Router wiring

`backend/internal/cmd/serve/router.go`:

- `r.MethodNotAllowed(http.HandlerFunc(httputil.Respond405Handler))` registered on the root mux. (Helper signature adapted to a chi-compatible handler that pulls request ID from context.)
- The `r.With(...).Get("/api/*", ...)` catchall switches to `Respond404`.

### Sweep

**401 sweep (`either_auth.go`)**: four `WriteJSONError` calls migrate to `Respond401`. Variable string moves from the `title` arg to the `detail` arg.

**404 sweep**: all `httputil.WriteJSONError(..., http.StatusNotFound, ...)` calls in `backend/internal/handlers/...` move to `Respond404`. ~20 sites:

- `handlers/assets/assets.go` — 9 sites
- `handlers/locations/locations.go` — 6 sites
- `handlers/orgs/orgs.go`, `members.go`, `invitations.go`, `api_keys.go` — multiple sites
- `handlers/users/users.go`, `handlers/lookup/lookup.go`, `handlers/reports/asset_history.go`, `handlers/assets/bulkimport.go` — remaining sites

Mechanical edit: caller-supplied variable string moves from the `title` arg to the `detail` arg.

The `ContentType` middleware in `middleware/middleware.go:111` calls `Respond415` instead of inline `WriteJSONError`.

### Out of scope

- `backend/internal/handlers/health/health.go` 405s use `http.Error` (no envelope). Not under `/api/`, not in the public spec — leave alone.
- `backend/internal/handlers/testhandler/invitations.go:49` 404 uses `http.Error`. Gated to non-production by `APP_ENV` check in `router.go:215` — leave alone.
- 403 and `validation_error` 400s already follow the contract per the TRA-538 description — no global sweep.

## Test plan

Targeted table tests, no router-walking. New tests live next to the code they cover.

**`backend/internal/util/httputil/auth_error_test.go`** — extend with a `Respond404` table test: shape, fixed title, request ID echo, JSON content-type header.

**`backend/internal/util/httputil/respond_405_test.go`** (new) — unit test asserting envelope shape, type, status, fixed title.

**`backend/internal/util/httputil/respond_415_test.go`** (new) — unit test asserting envelope shape, type, status, fixed title, fixed detail (asserts the multipart string is gone).

**`backend/internal/middleware/middleware_test.go`** — extend the existing `ContentType` test to assert the new envelope shape (`type:"unsupported_media_type"`, no multipart in `detail`).

**`backend/internal/cmd/serve/router_test.go`** (new or extend an existing router-level test) — integration tests through the wired router:

- BB12 §1.2 four 401 reproductions: missing header, wrong scheme, garbage token, missing header with `X-API-Key` set. Assert `title == "Authentication required"` (fixed), variable string is in `detail`. Assert `title` does not contain `"Bearer"`.
- 404 catchall (`/api/v1/nonexistent`) and 404 handler (`/api/v1/assets/{bogus-identifier}` with auth). Assert fixed `title:"Not found"`, variable string in `detail`. Assert `title` does not contain the bogus identifier.
- 405: `PATCH /api/v1/assets` with valid API-key auth. Assert envelope present (currently empty body), `type:"method_not_allowed"`, `status:405`.
- 415: `POST /api/v1/assets` with `Content-Type: text/plain`. Assert `type:"unsupported_media_type"`, `status:415`, no `"multipart"` substring in body.

Existing `apikey_test.go`, `either_auth_test.go`, and `middleware_test.go` exercise paths whose assertions reference the old shape; they update in lockstep, not as new tests.

## Trakrf-docs follow-up (separate PR, sequenced after backend merges)

Per the project rule "ship docs behind backend reality," the docs PR runs from a separate `trakrf-docs` checkout (user will `/add-dir` it) after the backend PR merges to main and deploys to preview.

`docs/api/errors.md`:

- The "Field" table row for `title` today reads: "A short human-readable summary safe to log. May vary between instances of the same `type` (for example, 401 responses carry different titles for missing-header vs expired-token vs revoked-key)." Replace with: "A short human-readable summary safe to log. **Fixed per `type`** — the variable explanation lives in `detail`."
- The catalog row for `unauthorized`: today says "`title` varies by cause — match on `type`." Replace with: "Cause is in `detail`. Branch on `type`."
- Add catalog rows: `method_not_allowed` (405) and `unsupported_media_type` (415).

`docs/getting-started/api.mdx`:

- Line 94 ("Write handlers against `body.error.type` and `body.error.detail`") is correct under the new contract. No change.

## Acceptance mapping

| Ticket criterion | Where covered |
|---|---|
| TRA-538: All 401 variants emit fixed title with detail variable | `either_auth.go` migration + integration tests in `router_test.go` (4 BB12 reproductions) |
| TRA-538: 404 emits fixed title with detail variable | `Respond404` helper + sweep + integration test |
| TRA-538: All non-2xx envelopes match the contract documented in `docs/api/errors` | Backend code change + docs follow-up PR |
| TRA-538: Test fails if `title` contains the variable string | Explicit `assert.NotContains` in router_test.go |
| TRA-538: BB-style spot check the four §1.2 reproductions | Integration tests in `router_test.go` |
| TRA-541 §1.10: 405 emits envelope with `method_not_allowed` | `Respond405` helper + chi `MethodNotAllowed` registration + test |
| TRA-541 §1.11: 415 emits `unsupported_media_type` | `Respond415` helper + middleware change + test |
| TRA-541: Multipart investigation resolved | Confirmed during exploration; recorded in PR description |
| TRA-541: Error catalog updated | `docs/api/errors.md` follow-up PR |
| TRA-541: Test that every non-2xx response carries the envelope | Targeted table tests per status code (option A from brainstorm Q3) |

## Risks and notes

- **Sweep correctness**: ~20 `Respond404` callsites and 4 `either_auth.go` 401 sites across handlers/middleware. Risk is low (mechanical) but the integration tests guard against shape drift.
- **Generated client compatibility**: adding `method_not_allowed` and `unsupported_media_type` to the swagger enum is additive thanks to `x-extensible-enum`. Existing generated clients tolerate unknown values.
- **Docs lag window**: between backend merge and docs PR merge, `docs/api/errors.md` is briefly behind reality. Acceptable — the "title may vary" line was always loosely truthful and now becomes overstrict; no integrator breaks during the gap. The "ship docs behind backend reality" rule is satisfied because we never claim a stricter contract before the code enforces it.
- **Test handler**: `testhandler/invitations.go` continues to emit a non-envelope 404 in dev/preview. Acceptable — it is dev-only, not in the public spec, and its `http.Error` usage is contained.

## Plan structure (for the implementation pass)

Single backend branch (`fix/tra-538-541-envelope-contract`). Two logical commits if you want bisectability, otherwise one rolled-up commit:

1. `fix(api): TRA-538 enforce fixed title for 401/404 envelopes` — `either_auth.go` migration, `Respond404` helper, sweep, router catchall, tests.
2. `fix(api): TRA-541 add 405 + 415 envelope helpers, drop multipart wording` — error types, helpers, chi `MethodNotAllowed`, middleware change, tests.

Followed by a separate trakrf-docs PR after backend merges and the preview deploy is verified.
