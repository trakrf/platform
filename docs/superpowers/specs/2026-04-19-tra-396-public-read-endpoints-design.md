# TRA-396 — Read-only public API endpoints design

**Linear:** [TRA-396](https://linear.app/trakrf/issue/TRA-396) — parent epic [TRA-210](https://linear.app/trakrf/issue/TRA-210)
**Date:** 2026-04-19
**Status:** Design — pending implementation plan
**Author:** Mike Stankavich
**Supersedes parts of:** [2026-04-19-tra392-public-api-design.md](./2026-04-19-tra392-public-api-design.md) — see "Amendments" below

---

## Context

TRA-393 (API key management + `APIKeyAuth` / `RequireScope` middleware + `GET /api/v1/orgs/me` canary) merged to main. TRA-392 (public API design) merged to main. TRA-394 (OpenAPI/Redoc docs pipeline) is in parallel review.

This card wires the TRA-393 auth chain to the first tranche of read endpoints — assets, locations, and the two existing scan-report surfaces — and normalizes response shapes, path parameters, and request conventions per TRA-392. Writes are deferred to TRA-397; rate limiting is deferred to TRA-395; a flat `/api/v1/scans` stream is deferred to a v1.1 bundle alongside webhooks.

## Scope summary

| In | Out |
|---|---|
| API-key auth + `*:read` scope checks on read routes | Write endpoints (TRA-397) |
| Single public response shape for all callers | Rate limiting + `X-RateLimit-*` (TRA-395) |
| Natural-key path params on public routes | Flat `/api/v1/scans` stream (v1.1 bundle) |
| Internal surrogate-path routes for FE convenience | Full OpenAPI tag split + Redoc delivery (TRA-394) |
| Request-convention enforcement (pagination cap, unknown-param 400, sort, filter allowlist) | Asset tag-identifier add/remove (stays internal) |
| Report-route renames: `reports/current-locations` → `locations/current`, `reports/assets/{id}/history` → `assets/{identifier}/history` | |
| List envelope normalized to `{data, limit, offset, total_count}` | |

---

## Architecture

Public API = same chi router, same handler functions, new middleware chain + new storage paths + new response structs. No separate mux, no separate URL prefix.

### Route-group shape

```go
// existing — session-auth group, unchanged
r.Group(func(r chi.Router) {
    r.Use(middleware.Auth)
    r.Use(middleware.SentryContext)
    ... existing handler registrations, with path adjustments below ...
})

// existing — TRA-393 canary
r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", orgsHandler.GetOrgMe)

// new — TRA-396 public read surface (dual auth: API-key OR session)
r.Group(func(r chi.Router) {
    r.Use(middleware.EitherAuth(store))  // falls through to APIKeyAuth or session.Auth
    r.Use(middleware.SentryContext)

    r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets",                        assetsHandler.ListAssets)
    r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}",           assetsHandler.GetAssetByIdentifier)
    r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}/history",   reportsHandler.GetAssetHistory)

    r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations",                  locationsHandler.ListLocations)
    r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}",     locationsHandler.GetLocationByIdentifier)
    r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/current",          reportsHandler.ListCurrentLocations)
})

// new — internal-only surrogate paths for frontend convenience
r.Group(func(r chi.Router) {
    r.Use(middleware.Auth)
    r.Use(middleware.SentryContext)

    r.Get("/api/v1/assets/by-id/{id}",                 assetsHandler.GetAssetByID)
    r.Get("/api/v1/assets/by-id/{id}/history",         reportsHandler.GetAssetHistoryByID)
    r.Get("/api/v1/locations/by-id/{id}",              locationsHandler.GetLocationByID)
})
```

Two notable pieces above:

1. **`EitherAuth` middleware** — a thin wrapper that inspects the JWT's `iss` claim and dispatches to `APIKeyAuth` or session `Auth` accordingly. Required because the public read routes accept both auth types (session callers — e.g., frontend — reach `/api/v1/assets/{identifier}` too once code is migrated; API-key callers reach the same routes with appropriate scopes). `RequireScope` must behave correctly under both principal types: if an API-key principal lacks scope → 403; if a session principal is present → pass through (session scope checks are governed by existing RBAC, not `api_keys.scopes`).

2. **Principal-agnostic org resolver.** One helper `getRequestOrgID(r *http.Request) (int, error)` returns the effective `org_id` — from `APIKeyPrincipal.OrgID` if present, else from session claims' `CurrentOrgID`. Handlers drop their hard `GetUserClaims` assumption and call this helper.

### Handlers share with internal callers

- Handlers return `PublicAssetView` / `PublicLocationView` — the canonical HTTP shape. Internal Go callers that need internal struct fields (`OrgID`, temporal validity beyond what the public shape exposes) call storage methods directly.
- `AssetView` / `LocationView` internal structs remain; HTTP layer just doesn't emit them.

### `GET /api/v1/orgs/me` — no change

TRA-393's canary route stays as-is. The scope model treats it as "any valid key" — no `RequireScope` decorator. Documented in the public API as a connectivity/key-verification probe.

### Public vs internal path decision

**Internal (session auth) keeps surrogate paths under `/by-id/{id}`.** `/api/v1/assets/{id}` (GET) is removed; the PUT/DELETE at `/api/v1/assets/{id}` stay untouched and are TRA-397's problem. The frontend migrates its detail-fetch URLs from `/api/v1/assets/${id}` to `/api/v1/assets/by-id/${id}`; same for locations. Report-page URLs migrate to `/api/v1/locations/current` and `/api/v1/assets/by-id/${id}/history`.

Rationale: path-pattern collision between `{id}` (numeric) and `{identifier}` (string) in chi's trie, resolved by giving internal surrogates their own explicit path. Frontend diff stays surgical (~5–10 files) instead of migrating to natural keys wholesale. Public API surface stays exactly as TRA-392 designed it (`/api/v1/assets/{identifier}`), undiluted by an internal-use regex discriminator.

---

## Route inventory

### Public (API-key auth or session auth)

| Method | Path | Scope | Status |
|---|---|---|---|
| GET | `/api/v1/assets` | `assets:read` | New public exposure of existing list handler |
| GET | `/api/v1/assets/{identifier}` | `assets:read` | New route; replaces `/api/v1/assets/{id}` for GET |
| GET | `/api/v1/assets/{identifier}/history` | `assets:read` | Moved from `/api/v1/reports/assets/{id}/history` |
| GET | `/api/v1/locations` | `locations:read` | New public exposure of existing list handler |
| GET | `/api/v1/locations/{identifier}` | `locations:read` | New route; replaces `/api/v1/locations/{id}` for GET |
| GET | `/api/v1/locations/current` | `locations:read` | Moved from `/api/v1/reports/current-locations` |
| GET | `/api/v1/orgs/me` | any valid key | Already shipped in TRA-393 — no change |

### Internal-only (session auth only) — new FE-convenience paths

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/assets/by-id/{id}` | Surrogate-based detail lookup for FE |
| GET | `/api/v1/assets/by-id/{id}/history` | Surrogate-based history lookup for FE |
| GET | `/api/v1/locations/by-id/{id}` | Surrogate-based detail lookup for FE |

### Unchanged (session auth only, TRA-397 will revisit)

- `POST /api/v1/assets`, `PUT /api/v1/assets/{id}`, `DELETE /api/v1/assets/{id}`
- `POST /api/v1/locations`, `PUT /api/v1/locations/{id}`, `DELETE /api/v1/locations/{id}`
- `POST /api/v1/assets/{id}/identifiers`, `DELETE /api/v1/assets/{id}/identifiers/{identifierId}`
- `POST /api/v1/assets/bulk`, `GET /api/v1/assets/bulk/{jobId}`

### Removed (no back-compat shim)

- `GET /api/v1/assets/{id}` — replaced by `/api/v1/assets/{identifier}` (public) and `/api/v1/assets/by-id/{id}` (internal)
- `GET /api/v1/locations/{id}` — replaced by `/api/v1/locations/{identifier}` (public) and `/api/v1/locations/by-id/{id}` (internal)
- `GET /api/v1/reports/current-locations` — replaced by `/api/v1/locations/current`
- `GET /api/v1/reports/assets/{id}/history` — replaced by `/api/v1/assets/{identifier}/history` (public) and `/api/v1/assets/by-id/{id}/history` (internal)

No external consumers exist, so no alias shim is provided.

---

## Response shape

### Transforms applied to all read endpoints

1. `id` → `surrogate_id` (renamed `json` tag, same underlying value)
2. `org_id` → dropped from JSON (`json:"-"`)
3. `current_location_id` (int) → `current_location` (string identifier), resolved via SQL join
4. `parent_location_id` (int) → `parent` (string identifier), resolved via SQL join
5. `deleted_at` → dropped from JSON (`json:"-"`); soft-deleted rows are never returned
6. `valid_from` / `valid_to` → **kept** in JSON (deliberate deviation from TRA-392 design doc — we retain optionality here, UI just doesn't feature them)
7. `identifiers[]` (tag identifier list) → **kept** on asset list and detail; both frontend and public consumers benefit

### Example — asset detail

```json
{
  "identifier": "widget-42",
  "name": "Widget #42",
  "type": "asset",
  "description": "Forklift 3 attachment",
  "current_location": "warehouse-1.bay-3",
  "metadata": { "any": "customer-supplied JSON" },
  "is_active": true,
  "valid_from": "2026-03-01T12:00:00Z",
  "valid_to": null,
  "created_at": "2026-03-01T12:00:00Z",
  "updated_at": "2026-04-10T09:15:00Z",
  "surrogate_id": 58273649,
  "identifiers": [
    { "kind": "rfid", "value": "EPC-ABC123" }
  ]
}
```

### Example — location detail

```json
{
  "identifier": "warehouse-1.bay-3",
  "name": "Bay 3",
  "description": "North bay",
  "parent": "warehouse-1",
  "path": "warehouse-1.bay-3",
  "metadata": {},
  "is_active": true,
  "valid_from": "2025-12-14T00:00:00Z",
  "valid_to": null,
  "created_at": "2025-12-14T00:00:00Z",
  "updated_at": "2026-02-02T00:00:00Z",
  "surrogate_id": 12345678,
  "identifiers": []
}
```

### List envelope — all list endpoints

```json
{
  "data": [ /* resource objects */ ],
  "limit": 50,
  "offset": 100,
  "total_count": 1234
}
```

Renames `count` → `limit` on today's handlers; `total_count` stays.

### Current-locations snapshot

```json
{
  "data": [
    {
      "asset": "widget-42",
      "location": "warehouse-1.bay-3",
      "last_seen": "2026-04-19T14:23:11Z"
    }
  ],
  "limit": 50,
  "offset": 0,
  "total_count": 1234
}
```

Replaces `asset_id` / `location_id` ints with natural keys.

### Asset history

```json
{
  "data": [
    {
      "timestamp": "2026-04-19T14:23:11Z",
      "location": "warehouse-1.bay-3",
      "duration_seconds": 3720
    }
  ],
  "limit": 50,
  "offset": 0,
  "total_count": 1234
}
```

### Error envelope — unchanged

Continues to use `httputil.WriteJSONError` as implemented pre-TRA-396. Error `type` values per TRA-392 §D; `rate_limited` already exists (added by TRA-393, used by TRA-395 later).

### Acknowledged join cost

List endpoints gain one `LEFT JOIN` each — `assets LEFT JOIN locations ON locations.id = assets.current_location_id`, and similarly `locations LEFT JOIN locations parent ON parent.id = locations.parent_location_id`. Both joins are on indexed primary keys; cost is acceptable at current scale. A realistic-data `EXPLAIN ANALYZE` check is part of the implementation plan's verification step. Fallback (in-memory resolver) documented in §Risks.

### `current_location` referencing a soft-deleted location

The join returns the identifier as recorded at scan time. If the referenced location is later soft-deleted, the identifier remains in the response but subsequent `GET /api/v1/locations/{identifier}` returns 404. Documented behavior matching the "eventual consistency" contract of a scanning system; no null-coalescing required.

---

## Request conventions

### Pagination

- `limit` default 50, hard-capped at 200. Exceeding returns `400 bad_request` (`detail: "limit must be ≤ 200"`).
- `offset` default 0, min 0.
- Envelope always returns `{data, limit, offset, total_count}`.

Tightens current caps (assets list default 10, current-locations max 100). Both callers (session and API-key) share the 200 cap.

### Unknown query parameter → 400

Each list handler declares an allowlist; anything else returns `400 bad_request` (`detail: "unknown parameter: foo"`).

### Per-resource filter allowlists

| Resource | Allowed filters (in addition to `limit`/`offset`/`sort`) |
|---|---|
| `GET /api/v1/assets` | `location` (natural key), `is_active`, `type`, `q` |
| `GET /api/v1/locations` | `parent` (natural key), `is_active`, `q` |
| `GET /api/v1/locations/current` | `location` (natural key), `q` |
| `GET /api/v1/assets/{identifier}/history` | `from` (ISO 8601), `to` (ISO 8601) |

Multi-value filters (e.g., `?location=wh-1&location=wh-2`) produce `IN (...)` semantics.

Current-locations previously accepted `location_id=<int>` and `search=<string>`; these rename to `location=<identifier>` and `q=<string>`. Frontend migrates its calls.

### Sort convention

`sort=field1,-field2`. Leading `-` = DESC. Allowlisted per resource; unknown field returns 400.

| Resource | Allowed sort fields | Default |
|---|---|---|
| Assets list | `identifier`, `name`, `created_at`, `updated_at` | `identifier` ASC |
| Locations list | `path`, `identifier`, `name`, `created_at` | `path` ASC |
| Current locations | `last_seen`, `asset`, `location` | `last_seen` DESC |
| Asset history | `timestamp` | `timestamp` DESC |

### Shared parser

`httputil.ParseListParams(r, Allowlist{Filters, Sorts}) (ListParams, error)` — single call per handler. Returns typed struct (with parsed `Limit`, `Offset`, filter map, sort slice) or a pre-formatted 400 error. Keeps validation uniform across handlers and under test in one place.

---

## Authentication and scope

### `EitherAuth` middleware

```go
// EitherAuth dispatches to APIKeyAuth or session Auth based on JWT iss claim.
// Public read routes use this so the frontend (session auth) and external
// API key callers share the same handler without duplicate registration.
func EitherAuth(store *storage.Storage) func(http.Handler) http.Handler
```

Behavior:
- No `Authorization` header → 401.
- Malformed `Bearer <token>` header → 401.
- **Peek at `iss` without signature verification** via a new helper `jwt.PeekIssuer(tokenString) (string, error)` in `backend/internal/util/jwt/`. Parses the JWT body without checking signature or expiry so we can route the request; the dispatched middleware then does full validation.
- `iss == "trakrf-api-key"` → delegate to `APIKeyAuth` chain (which runs full signature + expiry + revocation checks).
- `iss == ""` (absent) → delegate to session `Auth` chain. Session JWTs today carry no `iss`; absence is the session signal.
- Any other `iss` → 401.
- `PeekIssuer` parse error → 401.

**Why unverified peek is safe here:** we're only using `iss` to pick which chain does full validation. A forged token with a spoofed `iss` still has to pass signature verification at the delegated chain before any claims are trusted. The peek can't authorize anything by itself.

### Scope checks

`RequireScope(scope)` behavior updated to be principal-aware:
- If request has `APIKeyPrincipal`: existing check against `principal.Scopes`; missing → 403.
- If request has `UserClaims` only (session auth): pass through. Reads are gated only by authenticated session + current org (enforced by the session `Auth` middleware in the chain). No per-read RBAC check exists today; if read-level RBAC is introduced later, it applies uniformly at its own middleware step, not here.

This keeps `RequireScope` meaningful for API-key callers without double-gating session callers.

### Session-only routes

The `/by-id/{id}` internal paths use plain session `Auth`, not `EitherAuth`. They don't accept API-key tokens at all — if a customer's token arrives there, return 401. No `RequireScope` on these routes; session RBAC continues to govern.

---

## Frontend migration

Surgical FE changes; no URL-hash or route-params touched.

1. `apiClient.getAsset(id)` → `GET /api/v1/assets/by-id/${id}`
2. `apiClient.getLocation(id)` → `GET /api/v1/locations/by-id/${id}`
3. `apiClient.getAssetHistory(id, ...)` → `GET /api/v1/assets/by-id/${id}/history`
4. `apiClient.getCurrentLocations(...)` → `GET /api/v1/locations/current`; rename any `location_id=` query params to `location=`, `search=` to `q=`.
5. Shape adaptation:
   - `response.id` → `response.surrogate_id` (for the handful of places FE reads `id` from an API response rather than from the list row it already has)
   - `response.count` → `response.limit` on list pagination UI bindings
   - `response.current_location_id` → `response.current_location` where read directly
   - `response.parent_location_id` → `response.parent` where read directly
6. Types updated to reflect new shape; TypeScript compiler surfaces most miss-spots.

Estimated diff: ~5–10 files, ~50 lines, plus corresponding test-fixture updates.

Frontend continues to use surrogate IDs internally as the primary key for React Query caches, hash routes, and URL params. No changes to `#assets/:id` or `#locations/:id` patterns.

---

## Testing strategy

### Backend integration tests (real Postgres via testutil.SetupTestDB)

For each new public route:

- **Happy path — API key:** valid key with correct scope returns 200 + expected shape (JSON asserts on `current_location` = natural key string, `surrogate_id` present, `org_id` absent, `valid_from` present).
- **Happy path — session:** valid session JWT returns the same shape. Same handler, same emission.
- **Scope enforcement:** valid API key with wrong scope → 403 (e.g., `locations:read` key hitting `/api/v1/assets` → 403).
- **Auth rejection:** revoked key → 401; expired key → 401; session token passed as "api key" → 401 via `EitherAuth` dispatch logic.
- **Cross-org isolation:** API key for org A gets 404 for asset created by org B (identifier matches but org filter excludes).
- **List-param enforcement:** unknown param → 400; `limit=500` → 400; unknown sort field → 400; `sort=-updated_at` reverses correctly.
- **Internal `/by-id/{id}` routes:** session happy path; API-key token → 401 (never accepted on these).
- **Removed routes:** `GET /api/v1/reports/current-locations` → 404; `GET /api/v1/assets/{id}` → 404.

### Backend unit tests

- `httputil.ParseListParams`: table-driven; allowlist, cap, sort, multi-value filter, unknown param.
- `EitherAuth`: table-driven; api-key iss, session iss/absent, malformed header, unrecognized iss.
- `getRequestOrgID`: principal types return correct org; neither present returns error.

### New storage methods

Integration tests for `GetAssetByIdentifier`, `GetLocationByIdentifier`, list-with-join, against live DB. Cover soft-deleted rows excluded; `valid_to IS NULL` filter; cross-org isolation.

### Frontend — Vitest

- `apiClient` method URLs updated; tests updated alongside.
- Components that read `id` / `current_location_id` get shape-adapted fixtures.
- Shape types updated; compilation catches most regressions.

No new Playwright tests required. Existing smoke tests catch regressions on the handful of touched pages (asset detail, location detail, current-locations report, asset history).

### OpenAPI

- Keep existing `@Router` annotations accurate after path moves.
- Full tag split (`public` vs `internal`) + Redoc delivery remains TRA-394's scope; this card does not touch `backend/docs/` output.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Asset-list `LEFT JOIN locations` regresses latency | `EXPLAIN ANALYZE` on a realistic-size dataset as part of the plan's verification step. If >20% regression vs pre-join baseline, fall back to in-memory resolver (fetch referenced location IDs in one bulk query, map in Go). |
| Frontend drops `response.count` quietly | Grep for `.count` on API responses during FE migration; TypeScript catches explicit type usages; Vitest covers remaining UI bindings. |
| Report-route removal surprises internal callers | No external consumers; internal callers (frontend) get migrated in the same PR. No aliasing. |
| `RequireScope` wired out of order (before auth) | Tests assert exact response: unauthenticated on a scope-gated route → 401, not 403. |
| `EitherAuth` peek-at-`iss` mis-dispatches a token | `PeekIssuer` runs unverified — safe because it only selects a chain; the chain then does full signature + expiry validation. Tested via table: session JWT (no iss), API-key JWT (iss=trakrf-api-key), forged JWT (unknown iss), garbage string. |
| Customer identifier referencing a since-deleted location | Documented as-is: `current_location` reflects last-known; follow-up GET may 404. Noted in API docs (TRA-394 prose). |

---

## Sequencing — commit order

Each step compiles and passes tests on its own.

1. **Storage layer.** New methods: `GetAssetByIdentifier`, `GetLocationByIdentifier`, `ListAssets` / `ListLocations` with natural-key joins. Integration tests added alongside. Pure additive.
2. **Public response structs.** `asset.PublicAssetView`, `location.PublicLocationView`, report response types for current-locations and asset-history. JSON-tag audit.
3. **Shared request parser.** `httputil.ParseListParams` + table-driven tests.
4. **Org resolver.** `middleware.getRequestOrgID(r)` helper; unit-tested.
5. **`EitherAuth` middleware.** Unit-tested for all iss cases.
6. **Handler refactor.** Consume new storage + helpers; emit public structs. Internal assertion sites in existing tests adjust.
7. **Router surgery.** Register new public routes (`EitherAuth + RequireScope`); register new `/by-id/{id}` session routes; remove old `GET /assets/{id}`, `GET /locations/{id}`, and `/reports/*` routes.
8. **Frontend.** `apiClient` + shape-read updates, component fixtures, tests.
9. **Black-box integration pass.** Drive through each public route with a real API-key token against a running backend; verify scope enforcement, shape, auth rejection, cross-org isolation. Ends with `EXPLAIN ANALYZE` latency check on the list joins.

---

## Amendments to TRA-392 public API design

This card makes two small deviations from the TRA-392 design doc, documented here so the design record remains coherent:

1. **Path shape for internal/session-auth GET.** TRA-392 §A-3 states "Public URL paths use natural-key `{identifier}` on assets and locations. Frontend keeps using surrogate `{id}`." This is updated to: frontend uses `/api/v1/assets/by-id/{id}` and `/api/v1/locations/by-id/{id}` for GET (surrogate-based internal paths); public natural-key GET paths own `/api/v1/assets/{identifier}` and `/api/v1/locations/{identifier}`. Driven by chi path-pattern collision; no effect on the public API surface.

2. **Temporal fields in response shape.** TRA-392 §4 transform #1 states "Strip temporal fields — `valid_from`, `valid_to`, `deleted_at` are hidden." This is updated to: `deleted_at` is dropped from JSON; `valid_from` / `valid_to` are kept in JSON. Frontend does not feature them; no UI cost. Retains optionality in case future customer or internal use wants them without a shape rework.

3. **`/api/v1/scans` endpoint placement.** TRA-392 places plain `/api/v1/scans` (flat scan event stream) in the v1 read inventory. This card defers that endpoint to a v1.1 bundle alongside webhooks, scan aggregation, and other deferred enhancements. At v1 ship time, the v1 inventory table is updated to remove `/api/v1/scans`; the v1.x roadmap adds it alongside "Scan aggregation."

The TRA-392 doc is not edited by this card — these amendments stand here as the operative record until a consolidated edit lands at v1 ship time.

---

## References

- [TRA-392 public API design](./2026-04-19-tra392-public-api-design.md) — source of truth for shapes, scopes, errors, versioning, rate limiting
- [TRA-393 API key management design](./2026-04-19-tra393-api-key-management-design.md) — `APIKeyAuth`, `RequireScope`, `api_keys` table
- TRA-394 — OpenAPI + Redoc docs pipeline (in parallel review)
- TRA-395 — Rate limiting middleware (next after this card)
- TRA-397 — Write endpoints (next after TRA-395)
