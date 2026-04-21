# TRA-424 Platform Polish — Design

Linear: [TRA-424](https://linear.app/trakrf/issue/TRA-424/platform-write-endpoints-missing-from-public-spec-no-spec-discovery-ui)

Follow-ups from the 2026-04-21 black-box evaluation after [TRA-400](https://linear.app/trakrf/issue/TRA-400) merged. Platform-side fixes the docs ticket ([TRA-423](https://linear.app/trakrf/issue/TRA-423)) can't cover.

## Status of original findings

| # | Finding | Status |
|---|---|---|
| 1 | Write endpoints missing from public OpenAPI spec | **Already fixed** by TRA-407 (merged 2026-04-21). Confirmed `POST/PUT/DELETE /api/v1/assets` and `/api/v1/locations` are in `docs/api/openapi.public.json` and handlers are tagged `,public`. Dropped from scope. |
| 2 | No machine-readable spec served from the API host | In scope. |
| 3 | `HEAD /api/v1/assets` returns 404 | In scope. |
| 4 | API Keys page header reads "Inventory" | In scope. Root cause is broader than ticket states. |
| 5 | Create-key dialog doesn't show key fingerprint after close | In scope, but premise corrected: the API does **not** return `key_prefix` today. Alternative identifier chosen (JTI). |

## Delivery plan: two PRs

Backend (findings 2, 3, and the backend half of 5) ships first. Frontend (findings 4, and the UI half of 5) depends on the backend JTI field, so it rebases after backend merges + preview deploys.

---

## PR 1 — Backend

### #2: Serve OpenAPI spec from the API host (option C)

Embed the public spec in the Go binary and serve it from the API. Also redirect root-level probe paths to the versioned path so codegen tools find it either way.

**Changes:**

- `backend/internal/handlers/swaggerspec/swaggerspec.go`
  - Add `//go:embed openapi.public.json` → `publicJSON []byte`.
  - Add `//go:embed openapi.public.yaml` → `publicYAML []byte`.
  - Add `ServePublicJSON` and `ServePublicYAML` handlers mirroring the existing `ServeJSON`/`ServeYAML`.

- `backend/internal/cmd/serve/router.go`
  - Register (unauthenticated, outside the `middleware.Auth` group):
    - `GET /api/v1/openapi.json` → `swaggerspec.ServePublicJSON`
    - `GET /api/v1/openapi.yaml` → `swaggerspec.ServePublicYAML`
    - `GET /openapi.json` → 302 → `/api/v1/openapi.json`
    - `GET /openapi.yaml` → 302 → `/api/v1/openapi.yaml`
  - Root redirects must be registered **before** any SPA catchall that would otherwise serve `index.html` at these paths.

- Build/embed glue:
  - `backend/justfile` `api-spec` recipe: after generating `docs/api/openapi.public.{json,yaml}`, copy them into `backend/internal/handlers/swaggerspec/` so local builds embed the current spec.
  - `backend/Dockerfile`: copy `docs/api/openapi.public.{json,yaml}` into the `swaggerspec` package dir prior to `go build`, mirroring how `openapi.internal.*` is copied today.

- `.gitignore`: `openapi.internal.{json,yaml}` inside `backend/internal/handlers/swaggerspec/` are already gitignored (only `swaggerspec.go` is tracked). Add `openapi.public.{json,yaml}` in the same dir to `.gitignore` with the same treatment — embed-target only, never checked in.

**Rationale:** Embedding ties spec version to binary version (important for preview vs prod drift). Serving at `/api/v1/openapi.json` matches the versioned API namespace; the root alias matches what most codegen tools (OpenAPI Generator, Postman) probe by default. Content-types: `application/json` and `application/yaml` respectively.

### #3: HEAD method support globally

Use chi's built-in `GetHead` middleware to rewrite HEAD → GET before route matching. Go's `http.Server` strips the body from HEAD responses automatically.

**Changes:**

- `backend/internal/cmd/serve/router.go`
  - Add import: `chimiddleware "github.com/go-chi/chi/v5/middleware"` (aliased to avoid collision with the project's own `middleware` package).
  - Add `r.Use(chimiddleware.GetHead)` in `setupRouter`, placed before any route groups so it applies uniformly.

**Rationale:** One line, REST-correct for the entire API surface, no drift risk when someone adds a new GET route later.

### #5 backend half: Expose JTI on the list response

The ticket's "key_prefix" premise is incorrect — no such field exists and these are JWTs, not Stripe-style prefixed keys. Instead, expose the `jti` (already stored for revocation and already embedded in the JWT payload, so not a secret) so the UI can display a stable non-secret identifier.

**Changes:**

- `backend/internal/models/apikey/apikey.go`
  - Add `JTI string \`json:"jti"\`` to `APIKeyListItem`.

- `backend/internal/handlers/orgs/api_keys.go`
  - In `ListAPIKeys`, populate `JTI: k.JTI` when building the response.

### Backend tests

- `backend/internal/handlers/swaggerspec/*_test.go`
  - Unit test for `ServePublicJSON`: status 200, `Content-Type: application/json`, body parses as JSON, top-level `openapi` key present.
  - Unit test for `ServePublicYAML`: status 200, `Content-Type: application/yaml`, non-empty body.

- `backend/internal/cmd/serve/` integration test (or extend nearest existing router test):
  - `GET /api/v1/openapi.json` → 200, JSON.
  - `GET /openapi.json` → 302 with `Location: /api/v1/openapi.json`.
  - `HEAD /api/v1/assets` with a valid API key → 200, empty body, same headers as GET.

- `backend/internal/handlers/orgs/api_keys_integration_test.go`
  - Extend existing list test to assert `jti` is present and matches the stored row.

### Swagger annotations (backend)

The two new public-spec handlers are infrastructure, not part of the documented public surface. Tag them `,internal` only, so they appear in the internal Swagger UI (for debugging) but do **not** round-trip back into `openapi.public.{json,yaml}`.

---

## PR 2 — Frontend

### #4: Fix page title map for all main-shell routes

Root cause: `frontend/src/components/Header.tsx` `pageTitles` lacks entries for many routes and falls back to `pageTitles.inventory`. `api-keys` is one affected route among several.

**Changes:**

- `frontend/src/components/Header.tsx`
  - Add `pageTitles` entries for every route wired in `App.tsx` that renders inside the main shell:
    - `'api-keys'`: `{ title: "API Keys", subtitle: "Manage programmatic access tokens" }`
    - `'reports-history'`: `{ title: "Report History", subtitle: "Previously generated reports" }`
    - `'org-members'`: `{ title: "Members", subtitle: "Manage organization members" }`
    - `'org-settings'`: `{ title: "Organization Settings", subtitle: "Configure your organization" }`
    - `'create-org'`: `{ title: "Create Organization", subtitle: "Set up a new organization" }`
    - `'accept-invite'`: `{ title: "Accept Invite", subtitle: "Join an organization" }`
  - Change the fallback on line 158 from `pageTitles.inventory` to `{ title: "", subtitle: "" }`. Missing future routes will render blank rather than lying with "Inventory".

- Auth-flow routes (`login`, `signup`, `forgot-password`, `reset-password`) render full-screen without the header and intentionally stay out of the map.

### #5 frontend half: Display JTI chip

**Changes:**

- `frontend/src/types/apiKey.ts`
  - Add `jti: string` to the `APIKey` interface.

- `frontend/src/components/APIKeysScreen.tsx`
  - In the list row, render `{k.jti.slice(0, 8)}` as a muted monospace chip next to the key name (same line, tiny visual weight). Format: `<span className="font-mono text-xs text-gray-500 dark:text-gray-400">{k.jti.slice(0, 8)}</span>`.
  - Preserve existing name rendering; the chip is additive.

### Frontend tests

- `frontend/src/components/APIKeysScreen.test.tsx`
  - Extend the "renders key list" case with a fixture that includes `jti` and assert the truncated JTI is visible in the DOM.

- New or extended Header test:
  - For each of the added `pageTitles` keys (at minimum `api-keys`), render `Header` with that `activeTab` value and assert the corresponding title/subtitle appears.
  - Assert that an unknown `activeTab` value renders an empty title/subtitle (verifies the fallback change).

---

## Out of scope

- Renaming/restructuring the OpenAPI build pipeline (just this one embedded-public addition).
- Key rotation, key self-service beyond current screen, name uniqueness constraints.
- A standalone `last4` column or key-fingerprinting migration (can be reconsidered later if admins request it; JTI is sufficient for v1 disambiguation).
- Fixing title breadcrumbs for routes that render full-screen (auth flow).

## Risks / watch-outs

- **Embed copy step drift**: if the Dockerfile copy and the justfile copy diverge (e.g., someone updates one), local and prod builds embed different specs. Mitigation: keep both in lock-step with the existing `openapi.internal.*` handling — don't invent a new pattern.
- **Root `/openapi.json` vs SPA catchall**: the SPA static handler (`frontendHandler.ServeFrontend`) currently handles several specific root paths (`/assets/*`, `/favicon.ico`, etc.) explicitly but there may be a broader fallback. Verify the `/openapi.json` redirect registers before any wildcard that would capture it, and add a test for the 302.
- **JTI length & format**: UUIDs are 36 chars; first-8 yields 8 hex chars which is unique enough in practice for a per-org 10-key cap. No collision handling needed.
- **Chi `GetHead` interaction with rate limit/auth middleware**: `GetHead` only rewrites the method, so the HEAD request still passes through `EitherAuth`/`RequireScope`/`RateLimit`. That's correct behavior (HEAD should consume quota and respect auth just like GET). Add an auth-failure HEAD test case if coverage is light.
