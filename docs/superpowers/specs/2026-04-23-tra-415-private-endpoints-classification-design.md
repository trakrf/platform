# TRA-415 — Classify private-endpoints.md entries (public vs internal)

**Linear:** [TRA-415](https://linear.app/trakrf/issue/TRA-415/docsplatform-classify-private-endpointsmd-entries-public-vs-internal)
**Related:** TRA-408 (docs PR #14 that created `private-endpoints.md`), TRA-407 (contract bugs — done)
**Date:** 2026-04-23
**Status:** Approved — ready for implementation plan

## Problem

`docs/api/private-endpoints.md` on the docs site lists 11 endpoints the first-party SPA uses but that aren't published in the OpenAPI spec. Every row currently shows `Status: Undocumented`, `Classification: Pending`. This spec closes that — every row gets a real classification, the platform's swaggo annotations match, and the partition tooling produces accurate `openapi.public.*` / `openapi.internal.*` specs.

Secondary: the orgs handlers have no swaggo annotations at all today, so large chunks of the internal API (members, invitations, org create/update/delete) are silently omitted from the internal spec. This spec closes that gap too — cheap side effect of touching the same files.

## Classification decisions

The split was brainstormed against the constraint that every operation must be tagged exactly `public` or `internal` (the partition tool rejects anything else). "Public-with-caveats" is a docs concept, not a spec concept — it's expressed as a `public` tag plus prose in the docs page.

| Endpoint | Method | Classification | Rationale |
|---|---|---|---|
| `/api/v1/auth/login` | POST | Internal | SPA login flow; integrators use API keys |
| `/api/v1/auth/signup` | POST | Internal | SPA registration |
| `/api/v1/auth/forgot-password` | POST | Internal | SPA password recovery |
| `/api/v1/auth/reset-password` | POST | Internal | SPA password recovery |
| `/api/v1/auth/accept-invite` | POST | Internal | SPA invite acceptance |
| `/api/v1/users/me` | GET | Internal | User + org-memberships context for SPA; API keys are scoped to one org |
| `/api/v1/users/me/current-org` | POST | Internal | SPA org-switcher; API keys have a fixed org |
| `/api/v1/orgs` | GET | Internal | SPA org picker |
| `/api/v1/orgs/{id}` | GET | Internal | SPA org detail |
| `/api/v1/orgs/{id}/api-keys` | GET/POST/DELETE | Internal | Session-JWT-only; API keys cannot mint or revoke other keys. Ticket suggested Public for CI rotation, but publishing an endpoint integrators can't call is worse than being honest. Follow-up ticket to add an API-key-authenticated rotation primitive if demand appears. |
| `/api/v1/orgs/me` | GET | Public (with shape normalization) | API-key health-check endpoint. Normalize from bare `{id,name}` to `{"data":{...}}` for envelope uniformity. |

Method correction: the docs row says `GET /api/v1/users/me/current-org`; the actual route is `POST`. Docs row gets fixed.

### Scope extension — adjacent orgs handlers

The following routes share the `handlers/orgs/` package, are all SPA-only, and currently lack swaggo annotations. They weren't in the docs table (so no doc-row changes needed), but are annotated in the same pass for a complete internal spec:

- `POST /api/v1/orgs`, `PUT /api/v1/orgs/{id}`, `DELETE /api/v1/orgs/{id}`
- `GET /api/v1/orgs/{id}/members`, `PUT /api/v1/orgs/{id}/members/{userId}`, `DELETE /api/v1/orgs/{id}/members/{userId}`
- `GET /api/v1/orgs/{id}/invitations`, `POST /api/v1/orgs/{id}/invitations`, `DELETE /api/v1/orgs/{id}/invitations/{inviteId}`, `POST /api/v1/orgs/{id}/invitations/{inviteId}/resend`
- `GET /api/v1/auth/invitation-info` (already tagged `internal`, confirmed no-op)

All classified **Internal**.

## Out of scope (explicit)

- Any new API-key-authenticated rotation primitive. Separate ticket if/when demand appears.
- Building a runtime `X-Internal: true` response-header marker. The docs site already publishes both a positive list (`/api` reference from `openapi.public.*`) and a negative list (`/docs/api/private-endpoints`); both are crawlable before a request is ever sent. A runtime header fires only after the call and duplicates signal.
- Expanding the OpenAPI partition tool's tag vocabulary. `public`/`internal` remain the only two categories.
- Reclassifying any existing public endpoint.

## Approach

Two PRs, landed in order:

1. **Platform PR** — swaggo annotations + `/orgs/me` shape normalization + OpenAPI regen.
2. **Docs PR** — rewrite `private-endpoints.md` table to match reality. Lands only after platform PR is deployed to preview, per the project-wide rule that docs must not ship ahead of the backend.

### Platform PR — changes

#### 1. Swaggo annotations on orgs handlers

Files: `backend/internal/handlers/orgs/{orgs,me,api_keys,members,invitations,public}.go`.

Add `// @Summary`, `// @Tags <resource>,internal` (or `,public` for `public.go`), `// @ID <resource>.<verb>`, `// @Security SessionJWT`, `// @Router` annotations to every handler method. Request/response schema annotations follow the pattern already established in `handlers/auth/auth.go` and `handlers/users/users.go`.

Resource tag names: `orgs`, `org-members`, `org-invitations`, `api-keys` (keeps the Redoc grouping legible). Verb suffixes: `list`, `get`, `create`, `update`, `delete`, `cancel`, `resend`.

#### 2. `/orgs/me` shape normalization

File: `backend/internal/handlers/orgs/public.go:36-39`.

Change:

```go
httputil.WriteJSON(w, http.StatusOK, map[string]any{
    "id":   org.ID,
    "name": org.Name,
})
```

to:

```go
httputil.WriteJSON(w, http.StatusOK, map[string]any{
    "data": map[string]any{
        "id":   org.ID,
        "name": org.Name,
    },
})
```

The swaggo annotation declares this as the response schema; the partition tool picks it up. Rate-limit exclusion (already in place) is independent of shape and stays.

#### 3. Regenerate OpenAPI + commit

Run the existing `backend/internal/tools/apispec` flow. Two files change:

- `backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}` — gains 17 new internal operations across ~11 paths (users/me ×2, orgs/* ×5, api-keys ×3, members ×3, invitations ×4).
- `docs/api/openapi.public.{json,yaml}` — gains one path (`/api/v1/orgs/me`) in its new envelope shape.

#### 4. Tests

- **Shape test on `GET /orgs/me`** — added to the existing `backend/internal/handlers/orgs/public_integration_test.go`. Asserts the top-level `data` object with `id` and `name` fields. One test function.
- **Partition smoke** — the existing `apispec` test suite already guards the public/internal partition. Run it; expect pass.
- **No other new tests** — swaggo annotations are compile-time metadata, not runtime behavior. The `/orgs/me` shape change is the only runtime change.

No new migrations. No service-layer changes.

### Docs PR — changes

File: `docs/api/private-endpoints.md` in the `trakrf-docs` repo.

#### 1. Rewrite the table

| Endpoint | Method(s) | Used by | Status | Classification |
|---|---|---|---|---|
| `/api/v1/auth/login` | POST | SPA login form | Internal | Internal |
| `/api/v1/auth/signup` | POST | SPA signup form | Internal | Internal |
| `/api/v1/auth/forgot-password` | POST | SPA password recovery | Internal | Internal |
| `/api/v1/auth/reset-password` | POST | SPA password recovery | Internal | Internal |
| `/api/v1/auth/accept-invite` | POST | SPA invite acceptance | Internal | Internal |
| `/api/v1/users/me` | GET | SPA user context | Internal | Internal |
| `/api/v1/users/me/current-org` | **POST** *(was listed as GET — error)* | SPA org context | Internal | Internal |
| `/api/v1/orgs` | GET | SPA org picker | Internal | Internal |
| `/api/v1/orgs/{id}` | GET | SPA org detail | Internal | Internal |
| `/api/v1/orgs/{id}/api-keys` | GET, POST, DELETE | Settings → API Keys UI | Internal | Internal — session-JWT-only (see note below) |
| `/api/v1/orgs/me` | GET | API-key health check | **Published in `/api`** | Public |

#### 2. Rewrite "response-shape note" section for `/orgs/me`

Current section warns that the response is bare-object and may migrate. Replace with a short note: the response is now `{"data": {"id": ..., "name": ...}}` to match the rest of the public API, and the endpoint is in the OpenAPI spec at `/api`. Keep the "if you're using `/orgs/me` as a health check, prefer to also verify the standard envelope on a 'real' endpoint" guidance — it's still good practice even with shape parity. Remove the "if it migrates to the standard envelope, clients keyed on the bare-object shape will break" line — that migration just happened.

#### 3. Trim the "API-key management: session-JWT-only today" section

Keep the explanation of session-JWT-only auth (it's still true and is the reason the row is Internal). Drop the "if and when these endpoints are reclassified as public" paragraph — that decision has been made: they're Internal, with a separate-ticket follow-up if we add an API-key rotation primitive later.

#### 4. Replace "Classification decisions to come" with "Classification policy"

Short section: "Each row above is one of Public (in `/api` reference) or Internal (listed here and nowhere else). Public-with-caveats is documented inline in the `/api` reference via `x-stability` / deprecation annotations — no separate classification." Drop the speculative third bucket.

## Rollout

1. Open platform PR. CI runs `apispec` tool as part of `just validate`; OpenAPI diff is visible in the PR.
2. Platform PR merges → auto-deploys to preview → `/api/v1/orgs/me` returns new shape on preview.
3. Open docs PR in `trakrf-docs` separate worktree (per project convention). PR body links to merged platform PR.
4. Docs PR merges → preview sync → production.

No migration. No feature flag. Shape change on `/orgs/me` is the only runtime break and it's documented as "we're pre-GA, no external integrators depend on this yet."

## Testing plan

Platform PR:

- `just backend test` — existing suites stay green.
- New handler test: `TestGetOrgMe_ReturnsEnveloped` in `backend/internal/handlers/orgs/public_integration_test.go`.
- `just validate` — `apispec` partition rejects any handler that's tagged neither public nor internal. Serves as a guard against future annotation drift.
- Manual smoke: `curl -H "Authorization: Bearer <api-key>" https://app.preview.trakrf.id/api/v1/orgs/me` returns `{"data": {...}}`.

Docs PR:

- Docusaurus link check (CI in `trakrf-docs`).
- Manual: docs preview renders, the table reads correctly, no "Pending" cells remain.

## Success criteria

- Every row in `docs/api/private-endpoints.md` has a non-Pending classification.
- `openapi.public.json` gains `/api/v1/orgs/me`.
- `openapi.internal.json` gains 17 previously-unannotated orgs/user operations.
- No public-surface endpoint is reclassified or regressed.
- Docs site shows accurate Status + Classification for every row.
