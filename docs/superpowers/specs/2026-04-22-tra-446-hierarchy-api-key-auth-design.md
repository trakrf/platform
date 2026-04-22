# TRA-446 — Hierarchy endpoints accept API-key auth

**Linear:** [TRA-446](https://linear.app/trakrf/issue/TRA-446/hierarchy-endpoints-reject-api-key-auth-despite-docs-claiming-support)
**Related:** TRA-396 (public read wiring), TRA-407 (hierarchy routes), TRA-441 (hierarchy parent-shape fix)
**Date:** 2026-04-22
**Status:** Approved — ready for implementation plan

## Problem

`GET /api/v1/locations/{identifier}/ancestors`, `/children`, and `/descendants` return 401 to API-key callers that hold `locations:read`. The same calls succeed with a session JWT.

Root cause: TRA-396 wired the list and single-resource location reads into the `EitherAuth` public-read group in `router.go`. The three hierarchy routes, authored under TRA-407, were registered on the session-only router group via `locations.Handler.RegisterRoutes` and never moved. The OpenAPI annotations on the three handlers already declare `@Security APIKey[locations:read]`, so the published contract is ahead of the wiring.

For an API-savvy integrator, the first consequence is "my key works on `/locations` but not on `/locations/X/children`" — a contract inconsistency the docs actively deny. The second is that the 401 message (`"Bearer token is invalid or expired"`) sends them on a credential-rotation detour; that message is not fixed here (see *Out of scope*).

## Affected endpoints

| Route | Current auth surface | Target |
|-------|---------------------|--------|
| `GET /api/v1/locations/{identifier}/ancestors` | session only | API-key + session via `EitherAuth`, scope `locations:read` |
| `GET /api/v1/locations/{identifier}/children` | session only | API-key + session via `EitherAuth`, scope `locations:read` |
| `GET /api/v1/locations/{identifier}/descendants` | session only | API-key + session via `EitherAuth`, scope `locations:read` |

Post-fix the three routes also pick up the middleware the sibling public reads already have: `RateLimit(rl)` and `SentryContext`. That aligns runtime behavior with the OpenAPI `429` response already declared on each handler.

## Approach

**Decisions** (chosen during brainstorming):
1. **Single registration site, not duplication.** chi panics on duplicate route registration; dual-group registration isn't an option. Move the routes out of the session-only group entirely into the existing `EitherAuth` read group in `router.go`. `EitherAuth` already accepts both auth classes, so session callers continue to work.
2. **Delete `locations.Handler.RegisterRoutes` rather than leave it empty.** Every other location route is registered inline in `router.go`; keeping a one-method indirection just for the residue invites drift. Remove the method and its call site.
3. **Consolidate the two test router builders.** `public_integration_test.go` has `buildLocationsPublicRouter` (list + by-identifier) and `public_write_integration_test.go` has `buildLocationsHierarchyRouter` (the three hierarchy routes, wired correctly under `EitherAuth`). The original bug exists precisely *because* the hierarchy test helper diverged from production — it was specified correctly but production never caught up. Collapse into one helper (`buildLocationsPublicReadRouter`) in `public_integration_test.go` that mounts all five routes in one `EitherAuth` group, mirroring post-fix production. Update both callers. This kills the drift-class for the public-read surface.
4. **OpenAPI: no change.** Annotations already declare `@Security APIKey[locations:read]` and the 429 response on all three hierarchy handlers. The fix makes the code match the existing contract, not vice versa.
5. **Defer the 401 error-message taxonomy.** The ticket's wild-goose-chase path (API key on hierarchy → "Bearer token is invalid or expired") is cured by the route move. A general "unsupported auth method vs invalid credential" rework touches `middleware/apikey.go`, `middleware/either_auth.go`, and `httputil/auth_error.go`, and wants its own design pass on wording and `WWW-Authenticate` challenges. Out of scope for this ticket.

## Code changes

### `backend/internal/cmd/serve/router.go`
Inside the `EitherAuth` group at lines 118–134, add three lines next to `ListLocations` / `GetLocationByIdentifier`:

```go
r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/ancestors",   locationsHandler.GetAncestors)
r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/children",    locationsHandler.GetChildren)
r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/descendants", locationsHandler.GetDescendants)
```

Remove `locationsHandler.RegisterRoutes(r)` from the session-only group (line 97).

### `backend/internal/handlers/locations/locations.go`
Delete the `RegisterRoutes` method at lines 849–856 (and drop the now-unused `chi` import there if it was only for this method — verify).

### Test helper consolidation
In `backend/internal/handlers/locations/public_integration_test.go`:
- Replace `buildLocationsPublicRouter` with `buildLocationsPublicReadRouter` that registers all five public-read routes (list, by-identifier, ancestors, children, descendants) in one `EitherAuth` group.
- Update callers in `public_integration_test.go` (currently the list/by-identifier happy-path tests).

In `backend/internal/handlers/locations/public_write_integration_test.go`:
- Delete `buildLocationsHierarchyRouter` and re-point its callers at `buildLocationsPublicReadRouter`.

### New API-key-specific hierarchy tests
Add to `public_integration_test.go`, mirroring the style of `TestListLocations_APIKey_HappyPath`:
- `TestHierarchyAncestors_APIKey_HappyPath` — API key with `locations:read` → 200, expected parent shape
- `TestHierarchyChildren_APIKey_HappyPath` — same
- `TestHierarchyDescendants_APIKey_HappyPath` — same
- `TestHierarchy_APIKey_MissingScope_Returns403` — API key with some other scope (e.g. `assets:read`) hitting each of the three paths → 403
- `TestHierarchy_NoAuth_Returns401` — no Authorization header on each of the three paths → 401

These are distinct from the existing `TestLocationsGet{Ancestors,Children,Descendants}_ByIdentifier_Works` tests, which don't assert on auth class.

## Out of scope

- 401 message taxonomy rework (`middleware/apikey.go`, `either_auth.go`, `httputil/auth_error.go`). Separate ticket if we want it.
- Any change to hierarchy handler logic, storage calls, or response shape.
- Moving the existing hierarchy tests out of `public_write_integration_test.go` into a hierarchy-specific file. Cleanup opportunity but pure churn for this ticket.

## Definition of done

- `go test -tags=integration ./backend/internal/handlers/locations/...` green, including the new API-key-specific hierarchy tests.
- Manual smoke via the running preview or local server: an API key with `locations:read` returns 200 on each of `/ancestors`, `/children`, `/descendants`; without the scope returns 403; with no auth returns 401.
- `locations.Handler.RegisterRoutes` removed; no references remain.
- `just validate` (lint + test across both workspaces) green.
