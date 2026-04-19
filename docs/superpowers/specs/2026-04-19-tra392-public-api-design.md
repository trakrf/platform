# Public API design — endpoint inventory, resource shapes, auth model

**Linear:** [TRA-392](https://linear.app/trakrf/issue/TRA-392) — parent epic [TRA-210](https://linear.app/trakrf/issue/TRA-210)
**Date:** 2026-04-19
**Status:** Design — pending implementation plan
**Author:** Mike Stankavich

---

## Context and goals

TrakRF needs a customer-facing public REST API so integrators (starting with TeamCentral) can bidirectionally sync their asset and location records with TrakRF, and read logical scan events back into their systems (ERP, fixed-asset, maintenance, inventory).

An internal `/api/v1/` surface already exists, serving the frontend via session JWT auth. This document designs the public API surface as a documented subset of that existing API, distinguished by authentication mechanism rather than URL prefix. It defines endpoint inventory, resource shapes, authentication, errors, pagination/filtering/sorting, versioning policy, rate limiting, and documentation delivery.

This is a design document. Implementation happens across sub-issues: TRA-393 (API key management), TRA-394 (OpenAPI spec + rendered docs), TRA-396 (read-only endpoints), and two new sub-issues proposed in "Related work" below.

### Goals

- A v1 contract we can commit to publicly and not break for at least one year
- A surface small enough that TeamCentral can integrate against it without a months-long back-and-forth
- A code footprint small enough to ship in weeks, not quarters, given existing schedule pressure
- Clean, stable semantics that will still feel right once we have ten customers, not just one

### Non-goals for v1

- Webhooks / push delivery (TRA-210 mentions these as phase 3; out of scope here)
- Bulk import via public API (phase 2)
- OAuth 2.0 client-credentials grant (roadmapped for v1.x; API key auth is the v1 mechanism)
- GraphQL, gRPC, or anything non-REST
- Per-action scopes finer than `read` / `write` (see v1.x roadmap)
- Server-Sent Events or long-polling scan streams (customers poll in v1)

---

## Architecture

The public API is the existing `/api/v1/` router with API-key middleware applied to a documented subset of routes. Handlers, services, storage, and models are shared with the frontend. The public surface differs from the internal surface in three ways only:

1. **Authentication middleware** — API-key JWT instead of session JWT
2. **URL path parameters** — natural keys (`identifier`) instead of surrogate INT IDs
3. **Response shape transforms** — consistent stripping of internal fields and substitution of natural keys for cross-references (see "Resource shapes")

The public API does **not** get a separate `/api/public/v1/` prefix, a separate service, or a gateway layer. Option A from the TRA-210 epic.

Row-level security (PostgreSQL RLS) already enforces org scoping via `app.current_org_id`. The API-key middleware sets that session variable from the JWT's `org_id` claim, same as session auth does for authenticated users.

### Data exposure principle — logical only

TrakRF's value proposition is cleanly mapping physical scans (reader-generated `identifier_scans` of tag EPCs at physical antennas) into logical asset-at-location events. The public API exposes the logical layer only:

- **Publicly exposed:** `assets`, `locations`, `asset_scans` (surfaced via reports and a filtered scan list)
- **Internal only:** `scan_devices`, `scan_points`, `identifiers`, `identifier_scans`, `lookup` (autocomplete), `inventory`, `bulkimport`, user / invitation / org-admin management

Customers think in terms of business entities (this asset, this location, when was it last seen). They do not think in terms of RFID hardware topology. Exposing only the logical layer lets us evolve physical ingestion (new reader types, different tag standards) without breaking customer integrations.

---

## Endpoint inventory (public v1)

All endpoints live under `/api/v1/` and require a valid API key with appropriate scope.

### Assets (read + write)

Scopes: `assets:read`, `assets:write`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/assets` | List with filter, sort, pagination |
| GET | `/api/v1/assets/{identifier}` | Retrieve by customer-supplied natural key |
| POST | `/api/v1/assets` | Create (identifier optional; auto-assigned if omitted) |
| PUT | `/api/v1/assets/{identifier}` | Update |
| DELETE | `/api/v1/assets/{identifier}` | Soft delete |

### Locations (read + write)

Scopes: `locations:read`, `locations:write`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/locations` | List with filter, sort, pagination |
| GET | `/api/v1/locations/{identifier}` | Retrieve by natural key |
| POST | `/api/v1/locations` | Create |
| PUT | `/api/v1/locations/{identifier}` | Update |
| DELETE | `/api/v1/locations/{identifier}` | Soft delete |

### Logical scans and reports (read-only)

Scope: `scans:read`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/scans` | Logical scan stream, filterable by asset/location/time range |
| GET | `/api/v1/locations/current` | Snapshot of current asset-at-location map |
| GET | `/api/v1/assets/{identifier}/history` | Movement history for a single asset |

### Context (no scope required beyond valid key)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/orgs/me` | Org this key belongs to; used for connectivity checks and key-verification UX |

### Explicitly excluded from public v1

`users`, `invitations`, `orgs` write endpoints, `inventory`, `lookup`, `bulkimport`, any `scan_devices` / `scan_points` / `identifiers` / `identifier_scans` routes. YAGNI for v1; future customer demand drives additions.

---

## Resource shapes

Three transforms apply consistently to every public response derived from internal models:

1. **Strip temporal fields** — `valid_from`, `valid_to`, `deleted_at` are hidden. Public API always returns the currently-valid row.
2. **Strip internal linkage** — `org_id`, embedded `Org`, and the surrogate INT `id` are replaced by a single trailing `surrogate_id` for audit handling.
3. **Cross-references use natural keys** — `current_location_id` (int) becomes `current_location` (string identifier). Customers never handle trakrf surrogates in their own data model.

### Asset (public)

```json
{
  "identifier": "widget-42",
  "name": "Widget #42",
  "type": "asset",
  "description": "Forklift 3 attachment",
  "current_location": "warehouse-1.bay-3",
  "metadata": { "any": "customer-supplied JSON" },
  "is_active": true,
  "created_at": "2026-03-01T12:00:00Z",
  "updated_at": "2026-04-10T09:15:00Z",
  "surrogate_id": 58273649
}
```

- `identifier` is optional on `POST` (auto-assigned if omitted via the existing `generate_permuted_id` machinery); required on `PUT`.
- `current_location` is the parent location's `identifier`, not its surrogate.
- `metadata` is freeform JSON, an escape hatch for customer-specific fields.
- `surrogate_id` is documented as "stable, opaque; never interpret" — a join handle for customers whose identifier changes.

### Location (public)

```json
{
  "identifier": "warehouse-1.bay-3",
  "name": "Bay 3",
  "description": "North bay",
  "parent": "warehouse-1",
  "path": "warehouse-1.bay-3",
  "metadata": {},
  "is_active": true,
  "created_at": "2025-12-14T00:00:00Z",
  "updated_at": "2026-02-02T00:00:00Z",
  "surrogate_id": 12345678
}
```

- `parent` is the parent location's identifier; nullable for roots.
- `path` is the computed ltree path; read-only; included because hierarchical integrators want it.

### Scan (public, list element)

```json
{
  "timestamp": "2026-04-19T14:23:11Z",
  "asset": "widget-42",
  "location": "warehouse-1.bay-3",
  "duration_seconds": 3720
}
```

Flat event shape; no surrogate because scans aren't entities customers own.

### Current-locations snapshot

Array of `{asset, location, last_seen}` objects with natural keys. Mirrors `reports.CurrentLocationItem` with natural keys substituted for IDs.

### Asset history

Mirrors `reports.AssetHistoryResponse`; IDs replaced with identifiers; pagination envelope normalized (see below).

### List envelope (all list endpoints)

```json
{
  "data": [ /* resource objects */ ],
  "limit": 50,
  "offset": 100,
  "total_count": 1234
}
```

### Error envelope (all error responses)

```json
{
  "error": {
    "type": "validation_error",
    "title": "Invalid request",
    "status": 400,
    "detail": "identifier must be 1-255 characters",
    "instance": "/api/v1/assets",
    "request_id": "01J..."
  }
}
```

---

## Authentication and authorization

### Transport

`Authorization: Bearer <jwt>` on every request. Same header as session auth; different claims distinguish the two.

### API key token — JWT claims

```
iss:      "trakrf-api-key"
sub:      <api_key_jti>
aud:      "trakrf-api"
org_id:   <int>
scopes:   ["assets:read", "assets:write", ...]
iat:      <issued-at>
exp:      <optional expiry, omitted by default>
```

Signed HS256 with the same signing key as session JWTs for v1. The `iss` claim is the discriminator; middleware rejects a session JWT on a public route and vice versa. Splitting signing keys (distinct `kid`) is a v1.x improvement, not a launch blocker.

### Server-side state — new `api_keys` table

```sql
CREATE TABLE api_keys (
    id           BIGINT PRIMARY KEY,       -- permuted
    jti          UUID NOT NULL UNIQUE,
    org_id       INT NOT NULL REFERENCES organizations(id),
    name         VARCHAR(255) NOT NULL,    -- customer label, e.g. "TeamCentral sync"
    scopes       TEXT[] NOT NULL,
    created_by   INT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   TIMESTAMPTZ,              -- NULL = never expires
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
```

RLS policy mirrors other tables: `org_id = current_setting('app.current_org_id')::INT`.

Every authenticated request:

1. Parses the JWT and verifies the signature
2. Looks up `jti` in `api_keys`
3. Rejects with 401 if `revoked_at IS NOT NULL` or `expires_at` has passed
4. Bumps `last_used_at`
5. Sets `APIKeyPrincipal{OrgID, Scopes}` on the request context
6. Sets `app.current_org_id` session variable for RLS

Indexed on `jti`; one extra O(1) read per request.

### Scopes

Per-resource, coarse — five strings:

- `assets:read`, `assets:write`
- `locations:read`, `locations:write`
- `scans:read`

Finer granularity (e.g., `assets:create` vs `assets:update`) deferred to v1.x unless a customer requests it.

### Middleware

- **New:** `middleware.APIKeyAuth` — handles everything above for public routes.
- **Existing:** `middleware.Auth` continues to handle session JWTs for frontend routes.
- **Helper:** `middleware.RequireScope("assets:read")` — decorator on individual routes; missing scope → 403.

### Key lifecycle (drives TRA-393 UX shape)

- **Create:** org-admin opens Settings → API Keys → New Key. Names it, picks scopes, optionally sets an expiration. Full JWT displayed once, then only the `jti` prefix and metadata.
- **List / revoke:** admin sees name, scopes, created_at, last_used_at, expires_at (if set), revoke button.
- **Rotation:** customer creates a new key, updates their integration, revokes the old one. No in-place rotation.

### Expiry

`expires_at` is NULL by default (keys never expire). Customers or admins who want compliance-driven expiry set a date on creation. No in-UI edit after creation; rotate via create-new-revoke-old. No pre-expiry warning emails in v1 (customer sees expiration date in UI).

---

## Errors, validation, idempotency

### Error type catalog (the public contract)

| `type` | HTTP status | When |
|---|---|---|
| `validation_error` | 400 | Request body failed schema validation |
| `bad_request` | 400 | Malformed request — bad JSON, unknown query param, invalid sort field |
| `unauthorized` | 401 | Missing, malformed, revoked, or expired API key |
| `forbidden` | 403 | Valid key but insufficient scope |
| `not_found` | 404 | Natural-key lookup failed |
| `conflict` | 409 | Unique constraint violation (typically `(org_id, identifier)` collision) |
| `rate_limited` | 429 | Rate limit exceeded; see `Retry-After` header |
| `internal_error` | 500 | Unhandled server failure; retry advisable |

All existing `ErrorType` constants in `backend/internal/models/errors/errors.go` are kept; `rate_limited` is added.

### Validation errors carry structured field details

When `type: "validation_error"`, the error object carries an additional `fields` array:

```json
{
  "error": {
    "type": "validation_error",
    "title": "Invalid request",
    "status": 400,
    "detail": "Request validation failed",
    "instance": "/api/v1/assets",
    "request_id": "01J...",
    "fields": [
      { "field": "identifier", "code": "too_long",      "message": "must be ≤255 characters" },
      { "field": "type",       "code": "invalid_value", "message": "must be one of: asset" }
    ]
  }
}
```

`code` values: `required`, `invalid_value`, `too_short`, `too_long`, `invalid_format`, `out_of_range`. `field` uses JSON-pointer paths for nested errors (e.g., `metadata.foo.bar`). This is a backwards-compatible extension to the envelope — consumers that only read `detail` keep working.

### Idempotency strategy

No `Idempotency-Key` header in v1. Retry safety relies on existing properties:

- **`POST /assets`, `POST /locations`** — retry with the same `identifier` hits the `UNIQUE(org_id, identifier, valid_from)` constraint and returns `409 conflict`. Customer detects the pre-existing record and reconciles with `GET` / `PUT`. Customers who omit `identifier` (auto-assign) receive duplicates on retry; documentation will recommend always supplying an identifier for retry-critical workflows.
- **`PUT`** — HTTP-semantically idempotent; retries are safe.
- **`DELETE`** — idempotent; a second delete returns `404 not_found`, not `204`, so customers can detect state drift.

Full `Idempotency-Key` support is a v1.x enhancement if customer pain materializes.

### Request ID propagation

- Every response includes `X-Request-ID` (middleware.RequestID already sets this).
- Inbound `X-Request-ID` is accepted and echoed; otherwise a new ULID is generated.
- `request_id` inside error envelopes matches the response header.

### Deprecation / Sunset

When endpoints are retired (typically at a v2 cutover), responses carry:

- `Deprecation: true`
- `Sunset: <RFC 1123 date>`

per RFC 8594. Minimum six-month gap between announcement and removal. After sunset: `410 Gone`. No active deprecations at v1 launch; committing to the mechanism so future use doesn't surprise customers.

---

## Pagination, filtering, sorting

### Pagination — `limit` / `offset` / `total_count`

```
GET /api/v1/assets?limit=50&offset=100

{
  "data": [...],
  "limit": 50,
  "offset": 100,
  "total_count": 1234
}
```

- `limit` default 50, **maximum 200** (exceeding returns `400 bad_request` with detail `"limit must be ≤ 200"`)
- `offset` default 0
- `total_count` always included; may become approximate in v1.x for very large datasets

Cursor pagination deferred; will coexist as `?cursor=...` if added later (non-breaking).

### Filtering — flat query params

| Pattern | Example | Semantics |
|---|---|---|
| Equality | `?is_active=true` | exact match |
| Reference by natural key | `?location=warehouse-1.bay-3` | join on natural key |
| Multi-value (OR) | `?location=wh-1&location=wh-2` | `IN (...)` |
| Range (timestamps) | `?from=2026-04-01&to=2026-04-19` | inclusive; ISO 8601 |
| Full-text | `?q=widget` | fuzzy match on name + identifier + description |

No nested operator syntax (`filter[price][gt]`). Unknown query params return `400 bad_request`.

### Per-resource filter allowlists

- **Assets:** `location`, `is_active`, `type`, `q`
- **Locations:** `parent`, `is_active`, `q`
- **Scans:** `asset`, `location`, `from`, `to`

### Sorting — `sort=field1,-field2`

Leading `-` indicates descending order; multiple fields comma-separated.

| Resource | Allowed sort fields | Default |
|---|---|---|
| Assets | `identifier`, `name`, `created_at`, `updated_at` | `identifier` ASC |
| Locations | `path`, `identifier`, `name`, `created_at` | `path` ASC |
| Scans | `timestamp` | `timestamp` DESC |

Unknown sort field returns `400 bad_request`.

---

## Versioning

- **Path-based:** `/api/v1/*`. No header-based versioning.
- **v1 lifetime commitment — additive only:** new optional request fields, new response fields, new query params, new endpoints, new enum values (in documented open sets) are all in-bounds.
- **Breaking changes require v2:** removing/renaming fields, changing types, tightening validation, removing endpoints. v2 runs in parallel with v1 for the deprecation window.
- **Deprecation lifecycle:**
  1. Announcement in release notes + docs
  2. Deprecated responses carry `Deprecation: true` + `Sunset: <date>` headers
  3. Minimum 6 months before removal
  4. `410 Gone` after sunset
- **Enum openness labeled in OpenAPI:** `error.code` is open (may grow); HTTP status codes are closed. Customers must handle unknown values for open enums.

---

## Rate limiting

### Mechanism

Per-key token bucket, in-memory. Keyed by `api_keys.jti`.

- **Refill rate** = steady-state limit
- **Bucket capacity** = burst

Single-instance Railway deploy makes in-memory viable for launch. Horizontal scaling will require Redis or sticky routing (v1.x problem).

### Default tier (placeholder pending TRA-337 pricing)

- 60 requests/minute steady
- 120 request burst

A `tier` column on `api_keys` lets TRA-337 swap in pricing-driven limits without a schema migration.

### Response headers (on every API call)

- `X-RateLimit-Limit: 60` — steady-state limit
- `X-RateLimit-Remaining: 42` — tokens left in bucket
- `X-RateLimit-Reset: 1713547800` — Unix timestamp when bucket fully refills

### 429 response

```json
{
  "error": {
    "type": "rate_limited",
    "title": "Rate limit exceeded",
    "status": 429,
    "detail": "Retry after 30 seconds",
    "instance": "/api/v1/assets",
    "request_id": "01J..."
  }
}
```

Plus `Retry-After: 30`.

### Excluded from rate limiting

- `GET /api/v1/orgs/me` — used for health checks and key-verification; don't trip customers' connectivity probes.

### Per-key, not per-org

An org with three keys has 3× the combined limit. Simple and matches Stripe semantics; reconsider only if gamed.

---

## OpenAPI and documentation delivery

### One spec, two audiences, tag-based filtering

- **`@Tags public`** — all endpoints in this design's inventory
- **`@Tags internal`** — everything else (users, invitations, lookup, etc.)

Single source of truth: annotated handlers via existing swaggo toolchain. Generated `backend/docs/swagger.json` and `backend/docs/swagger.yaml` include both tag groups. Renderers filter:

- **Internal-facing:** `/swagger/*` (current route), auth-gated, shows all tags
- **Customer-facing:** `docs.trakrf.id/api` via Redoc filtered to the `public` tag

### Published customer-facing artifacts

- `https://docs.trakrf.id/api` — Redoc (preferred over Swagger UI for polish)
- `https://docs.trakrf.id/api/openapi.json` and `.yaml` — raw spec for customer codegen
- Postman collection generated from the spec

### Auth in OpenAPI

```yaml
securitySchemes:
  APIKey:
    type: http
    scheme: bearer
    bearerFormat: JWT
    description: TrakRF API key (JWT). Create in Settings → API Keys.
```

Each public operation declares `security: [APIKey: [<scope>]]`.

### Code examples in v1

- **Curl** example per endpoint via OpenAPI `x-code-samples`; rendered natively by Redoc.
- **Python, JavaScript:** deferred. Hand-written SDK examples rot fast; wait for customer signal before investing.

### Changelog

`docs/api/CHANGELOG.md` tracks public-API-affecting changes per release, labeled `added` / `deprecated` / `removed`. Feeds the deprecation lifecycle above.

### Prose documentation lives in the repo

See "Related work" — a new sub-issue owns the `docs/api/` prose (quickstart, auth guide, pagination guide, errors guide, versioning policy, changelog content). OpenAPI reference + prose guides are combined into the `docs.trakrf.id/api` site.

---

## Deployment contexts and BSL

The TrakRF platform is published under the Business Source License (BSL). The public API design above is the reference specification for the hosted `trakrf.id` deployment; any BSL-compliant deployment may expose the same `/api/v1/` contract.

Nothing in this design assumes hosted-only infrastructure:

- Authentication is local JWT (no third-party identity provider required)
- Rate limiting is in-process (no Redis required at launch)
- Documentation generation happens in CI against the repo (no proprietary tooling)
- The `api_keys` table and all middleware are part of the open platform

Implementation should keep this portability intact. Proprietary services (e.g., Resend for email, Sentry for error tracking) remain orthogonal and optional — the public API surface does not depend on them.

---

## Open questions

- **Locations — customer-facing identifier story:** schema already has `Location.Identifier` (required, 1-255 chars) with `UNIQUE(org_id, identifier, valid_from)`. Appears aligned with TRA-193's asset identifier pattern, but has not been independently audited for public-API consumption the way assets were. Sub-issue TRA-396 (read endpoints) should validate this before publication.
- **Top-level `/api/v1/scans` shape:** specified here as a flat event list, filterable by asset/location/time. TeamCentral's concrete integration may reveal that "give me everything since timestamp X" needs cursor pagination sooner than assets or locations do. Revisit after TeamCentral's first integration iteration.
- **Open vs closed enum inventory:** `error.fields[].code` on validation errors is open (may grow with new validation rules); exact set of HTTP status codes returned per endpoint is closed. All other enums (e.g., `Asset.type` — currently just `"asset"`) need an openness designation in OpenAPI before publication. Assigned to TRA-394.

---

## Planned v1.x capabilities (shape pre-committed, implementation deferred)

Additive features consciously held back from v1 to keep the first release small and ship-to-TeamCentral focused. Shapes are documented here so v1 clients aren't painted into a corner and future customers see the roadmap.

### Scan aggregation — `GET /api/v1/scans/aggregate`

TrakRF runs on TimescaleDB specifically to be good at time-series aggregation; v1.1 surfaces that capability to customers doing long-window analytics (monthly reconciliation, weekly reports) who would otherwise pull 100k+ raw scans to compute bucketed counts client-side.

Scope: `scans:read`.

```
GET /api/v1/scans/aggregate?interval=1h&from=2026-04-01&to=2026-04-19
                           &asset=widget-42         # optional filter
                           &location=warehouse-1    # optional filter
```

Response:

```json
{
  "interval": "1h",
  "from": "2026-04-01T00:00:00Z",
  "to":   "2026-04-19T00:00:00Z",
  "buckets": [
    {
      "start":       "2026-04-19T14:00:00Z",
      "asset":       "widget-42",
      "location":    "warehouse-1.bay-3",
      "scan_count":  42,
      "first_seen":  "2026-04-19T14:03:11Z",
      "last_seen":   "2026-04-19T14:58:47Z"
    }
  ]
}
```

**Allowed `interval` values (closed enum):** `1m`, `5m`, `15m`, `1h`, `6h`, `1d`, `1w`. No arbitrary duration parsing — keeps the OpenAPI enum finite and rules out pathological bucket counts.

**Grouping:** implicitly by `(asset, location)` within each time bucket. A `group_by` parameter is deliberately omitted from v1.1 to avoid cardinality-explosion footguns; add if customer demand is specific.

**Implementation notes for when v1.1 arrives:** TimescaleDB's `time_bucket(interval, timestamp)` plus a `GROUP BY (time_bucket, asset_id, location_id)` query. ~1 day of backend work.

### Other v1.x roadmap items

Consolidated from elsewhere in this document for easy scanning:

- **OAuth 2.0 client-credentials grant** (parallel auth path, coexists with API keys) — see §C-6
- **Per-resource action scopes** (e.g. `assets:create` vs `assets:update`) — see §C-2
- **Cursor pagination** on `/scans` (additive `?cursor=` param) — see §Pagination
- **`Idempotency-Key` header** for POST — see §D-4
- **Bulk import endpoints** (public-API-accessible) — see §Non-goals
- **Webhooks / push delivery** — see §Non-goals
- **Horizontally-scalable rate limiting** (Redis-backed) — see §G-1
- **Signing-key split** for session vs API-key JWTs — see §C-1
- **Language SDK examples** (Python, JavaScript) in OpenAPI — see §Code examples

None of these require breaking changes to reach. All are additive over the v1 contract.

---

## Related work

### Sub-issues this design spawns

- **Customer-facing API prose docs** — new sub-issue under TRA-210 (to be created). Writes `docs/api/README.md`, `quickstart.md`, `authentication.md`, `pagination-filtering-sorting.md`, `errors.md`, `versioning.md`, `CHANGELOG.md`. Blocked by TRA-392. Parallel to TRA-394.
- **Platform semver + release versioning discipline** — new sub-issue, independent of TRA-392. Covers: `version` string in `main.go`, CI tag format, release workflow, platform semver vs API contract version relationship. My strong recommendation: bump platform to v1.0.0 at public-API launch. Platform semver and `/api/v1/` are independent axes — platform can go v1.0.0 → v1.2.0 without touching the API.

### Existing sub-issues under TRA-210 — shape set by this design

- **TRA-393 — API key management** — data model and UX flow specified here in Authentication section. Implementation owns the `api_keys` migration, the CRUD UI, and the `APIKeyAuth` middleware.
- **TRA-394 — OpenAPI 3.0 spec + interactive docs** — tag strategy, renderer choice (Redoc), auth documentation, code-example scope specified here. Implementation owns swaggo annotations, CI regeneration, hosting.
- **TRA-396 — Read-only public API endpoints** — endpoint inventory (read portion), resource shapes, pagination/filtering/sorting conventions specified here. Implementation owns handler route wiring, natural-key resolvers, response-shape transforms.

### Dependencies

- **TRA-193 (done)** — customer identifier separation for assets. This design extends the same pattern to locations.
- **TRA-337** — pricing tiers. This design uses a placeholder default tier; TRA-337 defines the actual tier table.
- **TRA-83** — docs portal (Docusaurus). Hosts `docs.trakrf.id/api` alongside the rest of customer docs.

---

## Decisions log

### A. Architecture

- **A-1.** Extend existing `/api/v1/`; no separate public URL prefix or service. Shared handlers, services, storage, models.
- **A-2.** Logical-data-only exposure principle. Physical tables stay internal.
- **A-3.** Public URL paths use natural-key `{identifier}` on assets and locations. Frontend keeps using surrogate `{id}`.
- **A-4.** Surrogate INT IDs surfaced as `surrogate_id` in response bodies only, for audit-handle purposes.
- **A-5.** Temporal versioning (`valid_from`/`valid_to`) hidden from public responses. Public API always returns the currently-valid row.

### B. Scope

- **B-1.** Read + write for assets and locations; read-only for scans/reports; bulk deferred to v1.x.
- **B-2.** Excluded from v1: users, invitations, orgs write, inventory, lookup, bulkimport, physical tables.

### C. Authentication

- **C-1.** JWT-based API keys, HS256-signed with the same key as session JWTs; distinct `iss` claim for discrimination. Split signing keys is a v1.x improvement.
- **C-2.** Per-resource scopes: `assets:read`, `assets:write`, `locations:read`, `locations:write`, `scans:read`.
- **C-3.** DB-backed revocation via `jti` lookup on every request; O(1) indexed.
- **C-4.** Optional `expires_at` column on `api_keys`; default NULL (never expires). No pre-expiry warning emails in v1.
- **C-5.** Key rotation is create-new-revoke-old. No in-place rotation.
- **C-6.** OAuth 2.0 client-credentials deferred to v1.x as a second auth path coexisting with API keys.

### D. Errors

- **D-1.** Existing RFC 7807-inspired envelope (`httputil.WriteJSONError`) is the public contract.
- **D-2.** Validation errors carry a `fields[]` array alongside `detail`.
- **D-3.** `rate_limited` added to the error-type catalog.
- **D-4.** No `Idempotency-Key` header in v1. De-facto idempotency via natural-key constraint on POST; HTTP semantics on PUT/DELETE.
- **D-5.** `DELETE` on already-deleted returns `404 not_found`, not `204`.

### E. Pagination / filtering / sorting

- **E-1.** Pagination is `limit`/`offset`/`total_count`, matching current handler emission. Reverses an earlier draft decision based on unused `shared.Pagination` struct.
- **E-2.** `limit` hard-capped at 200.
- **E-3.** Flat filter query params; no nested operator syntax; unknown params return 400.
- **E-4.** Sort uses leading `-` for DESC, comma-separated for multi-field; unknown fields return 400.

### F. Versioning

- **F-1.** Path-based `/api/v1/`. Additive changes within v1.
- **F-2.** Breaking changes require v2 + parallel run; 6-month deprecation window; `Deprecation` + `Sunset` headers; `410 Gone` after sunset.

### G. Rate limits

- **G-1.** Per-key token bucket, in-memory for v1. Redis deferred until horizontal scaling.
- **G-2.** Default tier 60 req/min / 120 burst pending TRA-337 pricing.
- **G-3.** Per-key counting (not per-org).
- **G-4.** `GET /api/v1/orgs/me` excluded from rate limiting.

### H. Documentation

- **H-1.** Single OpenAPI source (annotated handlers) with `public` vs `internal` tag filtering for two audiences.
- **H-2.** Redoc at `docs.trakrf.id/api`; Swagger UI remains for internal `/swagger/*`.
- **H-3.** Curl examples only in v1; Python / JS SDK examples deferred until customer demand.
- **H-4.** Prose docs in `docs/api/` tracked by a separate sub-issue; repo is the canonical source, site renders from it.

### I. Platform

- **I-1.** BSL-compliant by construction — no hosted-only dependencies in the public API surface.
- **I-2.** Platform semver discipline tracked by a separate sub-issue; recommendation to bump to v1.0.0 at public-API launch.

### J. v1.x roadmap

- **J-1.** Scan aggregation endpoint shape pre-committed; implementation deferred to v1.1. Uses TimescaleDB `time_bucket`; enumerated interval set `{1m, 5m, 15m, 1h, 6h, 1d, 1w}`; grouping implicitly by `(asset, location)`. No `group_by` param in v1.1 to avoid cardinality footguns.
