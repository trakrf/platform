# TRA-503 — Pagination Envelope Consistency

**Status:** Design approved, plan pending
**Linear:** [TRA-503](https://linear.app/trakrf/issue/TRA-503/pagination-envelope-inconsistency-on-hierarchy-api-keys-and-asset)
**Branch:** `worktree-tra-503`
**Date:** 2026-04-25

## Problem

List endpoints across the public API are inconsistent about whether they return a pagination envelope. Two breaking effects:

1. **SDK codegen fragmentation.** Generators that detect pagination by envelope shape treat half the list endpoints as paginated and half as bare. The TeamCentral SDK (TrakRF's first public-API customer) consumes both groups.
2. **Schema/runtime drift on already-enveloped endpoints.** `ListAssetsResponse` and friends are named structs in swaggo annotations, but the handlers actually return `map[string]any{...}`. The OpenAPI spec promises a typed shape that the runtime doesn't enforce. SDK consumers get a clean type definition; the runtime can drift from it silently.

## Decision summary

Apply pattern A from the ticket — **envelope everywhere, on every list endpoint**. Add real pagination behavior (server-side `LIMIT`/`OFFSET`, separate `COUNT` for `total_count`) to four bare endpoints, and clean up the four already-enveloped endpoints so their handlers return typed structs instead of `map[string]any`.

After this change, every list endpoint on the public API returns `{data, limit, offset, total_count}` with `default_limit=50`, `max_limit=200`. Single rule, no per-endpoint overrides, documentable in one paragraph.

## Scope

### Endpoints touched

**Cleanup pass (already enveloped, swap `map[string]any` for typed struct):**

| Endpoint | File | Handler | Existing struct |
|---|---|---|---|
| `GET /api/v1/assets` | `backend/internal/handlers/assets/assets.go` | `ListAssets` | `ListAssetsResponse` |
| `GET /api/v1/locations` | `backend/internal/handlers/locations/locations.go` | `ListLocations` | `ListLocationsResponse` |
| `GET /api/v1/locations/current` | `backend/internal/handlers/reports/current_locations.go` | `ListCurrentLocations` | `ListCurrentLocationsResponse` |
| `GET /api/v1/assets/{id}/history` | `backend/internal/handlers/reports/asset_history.go` | `GetAssetHistory` | `AssetHistoryResponse` |

JSON output is byte-identical before/after; existing tests must pass unchanged.

**Envelope conversion (add real pagination):**

| Endpoint | File | Handler | New struct |
|---|---|---|---|
| `GET /api/v1/locations/{id}/ancestors` | `backend/internal/handlers/locations/locations.go` | `GetAncestors` | `ListAncestorsResponse` |
| `GET /api/v1/locations/{id}/children` | `backend/internal/handlers/locations/locations.go` | `GetChildren` | `ListChildrenResponse` |
| `GET /api/v1/locations/{id}/descendants` | `backend/internal/handlers/locations/locations.go` | `GetDescendants` | `ListDescendantsResponse` |
| `GET /api/v1/orgs/{id}/api-keys` | `backend/internal/handlers/orgs/api_keys.go` | `ListAPIKeys` | `ListAPIKeysResponse` (extend existing) |

The existing `LocationHierarchyResponse` (currently shared across all three hierarchy endpoints) is replaced by three distinct structs so each endpoint surfaces as a distinct OpenAPI schema and a distinct generated SDK type.

**Limit standardization (folded into this PR):**

- `backend/internal/handlers/reports/current_locations.go` — drop file-scoped `maxLimit=100`; use the global `httputil.maxListLimit=200`.
- `backend/internal/handlers/reports/asset_history.go` — drop package-scoped `assetHistoryMaxLimit=100`; use the global 200.

After this PR, every list endpoint on the public API uses `default=50, max=200` with no per-endpoint overrides.

### Out of scope

- Cursor-based pagination — offset/limit is fine for current scale.
- Changing the envelope shape itself.
- Adding pagination to non-list endpoints — none today.
- Customer-facing docs page on `docs/api/pagination` — separate ticket against `trakrf/docs`, lands after this PR merges (per "ship docs behind backend reality" rule).

## Architecture

### Storage layer

Add new paginated methods alongside the existing ones rather than mutating signatures of methods called from internal (non-handler) sites.

**`backend/internal/storage/locations.go`** — new methods:

- `ListAncestorsPaginated(ctx, orgID, id, limit, offset) ([]LocationWithParent, error)`
  — `ORDER BY nlevel(path) ASC, id ASC LIMIT $N OFFSET $M`.
- `CountAncestors(ctx, orgID, id) (int, error)`.
- `ListChildrenPaginated(ctx, orgID, id, limit, offset) ([]LocationWithParent, error)`
  — `ORDER BY name ASC, id ASC LIMIT $N OFFSET $M`.
- `CountChildren(ctx, orgID, id) (int, error)`.
- `ListDescendantsPaginated(ctx, orgID, id, limit, offset) ([]LocationWithParent, error)`
  — `ORDER BY path ASC, id ASC LIMIT $N OFFSET $M`.
- `CountDescendants(ctx, orgID, id) (int, error)`.

Existing `GetAncestors` / `GetChildren` / `GetDescendants` remain available for any internal caller that genuinely wants the full subtree.

**`backend/internal/storage/apikeys.go`** — new method:

- `ListActiveAPIKeysPaginated(ctx, orgID, limit, offset) ([]apikey.APIKey, error)`
  — `ORDER BY created_at DESC, id ASC LIMIT $N OFFSET $M`. Returns the storage row type; the handler maps to `apikey.APIKeyListItem` as it does today.

`CountActiveAPIKeys(ctx, orgID) (int, error)` already exists at `backend/internal/storage/apikeys.go:77` (used by the active-key cap check in `CreateAPIKey`); the new handler reuses it.

Existing `ListActiveAPIKeys` remains for non-handler callers.

### Handler layer

Each of the four bare endpoints follows the existing assets/locations pattern:

1. Parse query params via `httputil.ParseListParams(req, allowlist)` — gives `ListParams{Limit, Offset}`, returns 400 on invalid input.
2. Call paginated storage method.
3. Call count storage method.
4. Build typed response struct.
5. `httputil.WriteJSON(w, http.StatusOK, response)`.

The four already-enveloped endpoints get a one-line change: replace the `map[string]any{...}` literal with the corresponding `ListXResponse{Data: ..., Limit: ..., Offset: ..., TotalCount: ...}` value.

### Response struct shape

All eight envelope structs follow the same shape:

```go
type ListXResponse struct {
    Data       []SomeView `json:"data"`
    Limit      int        `json:"limit"       example:"50"`
    Offset     int        `json:"offset"      example:"0"`
    TotalCount int        `json:"total_count" example:"100"`
}
```

No shared `PaginatedResponse[T]` generic — per-endpoint named structs give clearer SDK-generated type names. The DRY tax is ~5 lines per endpoint.

### Ordering contract

| Endpoint | ORDER BY |
|---|---|
| `ancestors` | `nlevel(path) ASC, id ASC` (root → target) |
| `children` | `name ASC, id ASC` (alphabetical) |
| `descendants` | `path ASC, id ASC` (depth-first lexical) |
| `api-keys` | `created_at DESC, id ASC` (newest first, for rotation workflows) |

`id ASC` tiebreaker on every endpoint guarantees fully deterministic ordering across pages.

### OpenAPI

Falls out automatically. After the struct changes:

```
just backend api-spec
```

regenerates `backend/internal/openapi/openapi.json` from the new struct definitions and `@Success` annotations. The `api-spec` recipe self-heals the `frontend/dist` stub (per commit `63ef61e`), so this should just work.

## Data flow (paginated request)

For `GET /api/v1/locations/{id}/descendants?limit=20&offset=40`:

1. Router resolves handler → `GetDescendants`.
2. Auth middleware injects `orgID` into context.
3. Handler resolves `{id}` → internal `locationID` (existing logic).
4. `httputil.ParseListParams(req, nil)` → `ListParams{Limit: 20, Offset: 40}`. Returns 400 on bad input.
5. Storage call: `ListDescendantsPaginated(ctx, orgID, locationID, 20, 40)` → up to 20 rows ordered by `path ASC, id ASC`, skipping the first 40.
6. Storage call: `CountDescendants(ctx, orgID, locationID)` → total count of descendants for the requested root.
7. Map storage rows → `[]location.PublicLocationView`.
8. Build `ListDescendantsResponse{Data, Limit: 20, Offset: 40, TotalCount: N}`.
9. `httputil.WriteJSON(w, 200, response)`.

## Error handling

No new error paths. `ParseListParams` already enforces:

- Negative `limit` or `offset` → 400.
- Non-numeric values → 400.
- `limit > maxListLimit (200)` → 400.

Existing org-scoping (404 on cross-org access) and not-found handling are unchanged.

## Testing

### Newly-paginated endpoints (4)

For each (`ancestors`, `children`, `descendants`, `api-keys`) in the existing `*_integration_test.go` files:

- **Happy path** — response unmarshals into the typed envelope; all four fields populated; `data` is the expected list.
- **Pagination behavior** — seed N>2 rows, request `?limit=2&offset=2`, assert correct slice and `total_count == N`.
- **Validation wiring** — one bad-input test (e.g., `?limit=999`) returns 400. Confirms `ParseListParams` is wired in; not exhaustive (the parser has its own unit tests).

### Descendants ordering (1 dedicated test)

Build a small subtree with predictable path values (e.g., `a`, `a.1`, `a.1.x`, `a.2`), request page 1 (`limit=2`) and page 2 (`limit=2&offset=2`), assert the union covers all four nodes in `path ASC` order. Catches missing or non-deterministic `ORDER BY`.

### Cleanup-pass endpoints (4)

No new tests. Existing tests must pass unchanged — that *is* the regression check. JSON output is byte-identical because typed-struct marshal produces the same shape as the `map[string]any` it replaces.

### Limit standardization

If any existing test asserts `limit > 100` is rejected on `current_locations` or `asset_history`, update to `limit > 200`. Otherwise no change.

## Risks

- **Storage signature evolution.** Mitigated by adding new paginated methods (`ListXPaginated`, `CountX`) alongside the existing ones rather than changing existing signatures. Internal non-handler callers stay on the unbounded methods. Slight method proliferation (7 new methods: 3 paginated hierarchy lists, 3 hierarchy counts, 1 paginated api-keys list — `CountActiveAPIKeys` already exists) for zero risk to internal flows.
- **API keys ordering switch.** The current `ListActiveAPIKeys` has no explicit `ORDER BY`. Switching to `created_at DESC, id ASC` is a behavior change for any caller that happened to depend on the (unspecified) prior order. Mitigation: `git grep` during implementation; expectation is no internal caller cares.
- **swaggo regeneration noise.** `just backend api-spec` regenerates `openapi.json` — the diff will include the new `ListAncestorsResponse`/`ListChildrenResponse`/`ListDescendantsResponse` schemas plus the renamed `ListAPIKeysResponse` shape. Reviewers should expect a sizeable but mechanical OpenAPI diff.

## Acceptance

- All eight list endpoints return `{data, limit, offset, total_count}` at runtime, marshaled from typed structs (no `map[string]any`).
- All four newly-paginated endpoints accept `?limit=` and `?offset=` and honor them at the storage layer.
- All four newly-paginated endpoints have stable ordering with `id ASC` tiebreaker.
- `default=50, max=200` everywhere; per-endpoint limit overrides removed from `current_locations` and `asset_history`.
- OpenAPI spec reflects the new shape.
- `descendants` ordering test passes; per-endpoint pagination tests pass; existing tests on cleanup-pass endpoints pass unchanged.
- Follow-up Linear ticket created for the `docs/api/pagination` page (separate PR against `trakrf/docs`, sequenced after this PR merges).

## Sequencing

1. **This PR** (TRA-503, platform repo): backend handlers + storage methods + tests + regenerated OpenAPI spec.
2. **Follow-up PR** (separate Linear ticket against `trakrf/docs`, separate checkout per the docs-PR rule): `docs/api/pagination` reference page; cross-link from each list endpoint's docs.
