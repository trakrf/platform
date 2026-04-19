# OpenAPI 3.0 spec generation + interactive API docs (Redoc)

**Linear:** [TRA-394](https://linear.app/trakrf/issue/TRA-394) — parent epic [TRA-210](https://linear.app/trakrf/issue/TRA-210)
**Date:** 2026-04-19
**Status:** Design — pending implementation plan
**Author:** Mike Stankavich
**Depends on:** [TRA-392](https://linear.app/trakrf/issue/TRA-392) (public API design — merged 2026-04-19), [TRA-83](https://linear.app/trakrf/issue/TRA-83) (Docusaurus portal at `docs.trakrf.id` — in review)

---

## Context and goals

TRA-392 locked the public API contract (endpoints, shapes, auth, errors, pagination, versioning, rate limits) and committed to publishing Redoc-rendered API reference at `docs.trakrf.id/api` plus raw OpenAPI spec + Postman collection downloads. TRA-394 owns the tooling that turns swaggo annotations in `backend/` into those published artifacts.

The backend already uses swaggo — 322 `@Summary`/`@Tags`/`@Router`/etc. annotations across 10 handler packages — and CI runs `swag init` during lint. What's missing: (1) OpenAPI 3.0 output (swaggo emits 2.0), (2) separation of the customer-facing subset from the internal surface, (3) rendered customer docs, (4) drift detection between annotations and published spec, (5) enum-openness labeling per TRA-392 open question #3, (6) auth-gating of the currently-public internal `/swagger/*` UI.

### Goals

- Every public TrakRF API operation renders as a customer-grade entry in Redoc at `docs.trakrf.id/api` with consistent auth, error, and pagination descriptions.
- Raw OpenAPI 3.0 spec (JSON + YAML) and a Postman collection are fetchable directly from `docs.trakrf.id/api/openapi.{json,yaml}` and `docs.trakrf.id/api/trakrf-api.postman_collection.json`.
- Spec updates follow a reviewable cross-repo PR flow — spec changes are visible diffs before customers see them.
- CI fails when handler annotations drift from the committed public spec.
- The internal API surface (users, invitations, inventory, lookup, bulkimport, auth, health, test) is reachable only via an auth-gated `/swagger/*` UI on the platform, not published externally.

### Non-goals for v1

- Language SDK examples (Python, JavaScript, Go). Redoc auto-generates curl plus a few others; hand-crafted SDK snippets wait for customer demand (TRA-392 §H-3).
- Contract testing (Dredd, Schemathesis, Pact). Typed handlers + annotations are adequate for v1.
- Semver / diff automation (openapi-diff). v1 is additive-only per TRA-392 §F-1; automation arrives ahead of the first deprecation cycle.
- Docusaurus versioning (`/api/v1` vs `/api/v2` side-by-side). Single v1 surface at launch; opt in when v2 enters parallel-run.
- RBAC on `/swagger/*` (role-based filtering so non-admins see read-only operations). v1.x enhancement; v1 ships with a single `middleware.Auth` gate.
- `x-codeSamples` hand-crafted snippets. Redoc's native renderer handles curl for v1.
- Prose guides (`quickstart.md`, `authentication.md`, `pagination-filtering-sorting.md`, `errors.md`, `versioning.md`, `CHANGELOG.md`). TRA-392 §Related work assigns those to a sibling sub-issue.

---

## Architecture

One annotated source, two filtered specs, two renderers.

```
platform/backend (annotated handlers — source of truth)
         │
         │  just backend api-spec
         │
         ▼
  swag init → docs/swagger.json (OpenAPI 2.0)
         │
         ▼
  apispec tool (Go, in-repo)
  ├── convert 2.0 → 3.0                  (github.com/getkin/kin-openapi)
  ├── filter by @Tags                    → openapi.public.{json,yaml}
  │                                        openapi.internal.{json,yaml}
  └── post-process                       bearerFormat: JWT, servers, info
         │
         ▼
  pnpm dlx openapi-to-postmanv2          → trakrf-api.postman_collection.json

platform runtime
  /swagger/*  (auth-gated)               serves openapi.internal (embedded)

platform CI (publish-api-docs.yml on main merge)
  cross-repo PR → trakrf-docs            static/api/openapi.public.{json,yaml}
                                         static/api/trakrf-api.postman_collection.json

trakrf-docs
  redocusaurus plugin                    /api renders openapi.public.yaml
  static assets                          raw spec + Postman downloadable
```

**Two specs at runtime.** The internal spec lives only in the `platform` build artifact (embedded into the Go binary via `go:embed`) and backs the internal Swagger UI. The public spec is the customer contract — committed to `platform` at `docs/api/openapi.public.{json,yaml}`, mirrored into `trakrf-docs` via cross-repo PR, served as static assets under `docs.trakrf.id/api/`.

**Security posture.** Internal endpoint names (`/bulkimport`, `/lookup`, internal test routes) never appear in the spec served at `docs.trakrf.id`. The public spec is pre-filtered in CI, not hidden in-render, so the contract is explicit at the file level.

---

## Annotation strategy

### Tag discipline

Every operation receives exactly one of `@Tags public` or `@Tags internal`. Two tags on the same handler is illegal. The `apispec` tool (§Build pipeline) enforces this: an operation with neither tag, or with both, fails the build with a message naming the path and method.

Pass over the existing 10 handler packages with the following rubric, derived directly from TRA-392 §Endpoint inventory:

| Handler | Tag | Notes |
|---|---|---|
| `handlers/assets/assets.go` | `public` | CRUD on `/api/v1/assets`, full surface per TRA-392 |
| `handlers/assets/bulkimport.go` | `internal` | Bulk is deferred to v1.x |
| `handlers/locations/locations.go` | `public` | CRUD on `/api/v1/locations` |
| `handlers/reports/current_locations.go` | `public` | Exposed as `/api/v1/locations/current` |
| `handlers/reports/asset_history.go` | `public` | `/api/v1/assets/{identifier}/history` |
| (future) scans list endpoint | `public` | TRA-396 adds `/api/v1/scans`; annotation added there |
| `handlers/orgs/*` (`/orgs/me` only) | `public` | Key-verification endpoint only; all other org routes stay `internal` |
| `handlers/users/users.go` | `internal` | Excluded from v1 |
| `handlers/inventory/save.go` | `internal` | Excluded from v1 |
| `handlers/lookup/lookup.go` | `internal` | Excluded from v1 |
| `handlers/auth/auth.go` | `internal` | Session JWT flow; not customer surface |
| `handlers/health/health.go` | `internal` | Not part of v1 contract; may expose selectively in v1.x |
| `handlers/testhandler` | `internal` | Dev-only; already `APP_ENV != "production"` gated |

### Public-handler annotation requirements

Beyond tagging, every `public`-tagged operation must have:

- `@Summary` — customer-facing one-liner (active voice, imperative mood; e.g., "Retrieve an asset").
- `@Description` — one to three sentences; explains semantics, including any identifier resolution or soft-delete behavior relevant to the caller.
- `@Security APIKey` with the scope required by that operation, drawn from TRA-392 §Auth scopes (`assets:read`, `assets:write`, `locations:read`, `locations:write`, `scans:read`).
- `@Failure` lines for every error type in TRA-392's catalog that the handler can emit: `400 validation_error`, `400 bad_request`, `401 unauthorized`, `403 forbidden`, `404 not_found`, `409 conflict` (where applicable), `429 rate_limited`, `500 internal_error`.
- Enum-valued fields annotated per the audit in §Enum openness.

### Path annotation

Public handlers that expose natural-key URLs (assets, locations under TRA-392 §A-3) use `{identifier}` in their `@Router` line. Surrogate-INT-vs-identifier routing is TRA-396's responsibility; TRA-394 annotates against paths as they exist at merge time. If TRA-396 lands first, TRA-394 uses `{identifier}`. If TRA-394 lands first, annotations reference `{id}` and TRA-396 updates both code and annotations. This keeps the two tickets sequenceable in either order.

### Security scheme

Declared once in `backend/main.go` swag header comments:

```
@securityDefinitions.apikey APIKey
@in header
@name Authorization
@description TrakRF API key (JWT). Format: "Bearer <jwt>". Create in Settings → API Keys.
```

Swag 2.0 emits `type: apiKey`. The `apispec` tool's post-processing step maps this to the 3.0 HTTP-Bearer form expected by TRA-392 §Auth in OpenAPI:

```yaml
securitySchemes:
  APIKey:
    type: http
    scheme: bearer
    bearerFormat: JWT
    description: TrakRF API key (JWT). Create in Settings → API Keys.
```

---

## Build pipeline

### New recipe: `just backend api-spec`

```
# backend/justfile
api-spec:
    @echo "📚 Generating OpenAPI 3.0 specs..."
    swag init -g main.go --parseDependency --parseInternal -o docs
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out ../docs/api/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal
    @echo "✅ Public spec:   docs/api/openapi.public.{json,yaml}  (committed)"
    @echo "✅ Internal spec: backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}  (gitignored, embedded in binary)"
```

### The `apispec` tool

New Go binary at `backend/internal/tools/apispec/`. Keeps the pipeline in one language and avoids dragging Node deps into the backend build.

Dependencies:

- `github.com/getkin/kin-openapi` for 2.0 → 3.0 conversion and schema model
- Standard library for YAML emission via `gopkg.in/yaml.v3`

Responsibilities, one pass:

1. **Convert** `swagger.json` (2.0) to OpenAPI 3.0 using `kin-openapi`'s `openapi2conv.ToV3`.
2. **Filter** the operations map by tag. For each path/operation pair:
   - If `tags` contains `public` → emit into the public spec.
   - If `tags` contains `internal` → emit into the internal spec.
   - If neither (or both) → fail the build with a clear error (`handler X at path Y has neither public nor internal tag`).
3. **Prune** unreferenced schemas in the public spec (operations filtered out may reference schemas no longer used; `kin-openapi` exposes walks sufficient for this).
4. **Post-process** the security scheme: set `type: http`, `scheme: bearer`, `bearerFormat: JWT`, preserve description.
5. **Set `info`**: `title: "TrakRF API"`, `version: "v1"`, contact/licence fields from TRA-392 surface. For the internal spec, set `info.title: "TrakRF Internal API — not for customer use"`.
6. **Set `servers`**: public `[{url: "https://trakrf.id", description: "Production"}]`; internal `[{url: "http://localhost:8080", description: "Local development"}]`.
7. **Write** both JSON and YAML outputs for each spec.

### Postman collection

Separate step because `openapi-to-postmanv2` is Node:

```yaml
- run: pnpm dlx openapi-to-postmanv2 \
       -s docs/api/openapi.public.json \
       -o docs/api/trakrf-api.postman_collection.json \
       -p -O folderStrategy=Tags
```

### Workflow layout

Two new GitHub Actions workflows under `.github/workflows/`:

**`api-spec.yml`** — PR-time validation (runs on PRs touching `backend/**`):

- Check out branch
- Install Go, swag, pnpm
- `just backend api-spec`
- `pnpm dlx openapi-to-postmanv2 ...` (Postman collection)
- `git diff --exit-code docs/api/` — drift check; fails with "run `just backend api-spec` and commit the result" if the committed public spec is stale
- `pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml --extends=recommended` — spec validity
- Upload `docs/api/*` as a workflow artifact for preview inspection

**`publish-api-docs.yml`** — main-merge publication (runs on push to `main`):

- Check out `platform` main
- Regenerate spec (same pipeline as above) into a scratch path
- Check out `trakrf/docs` using secret `TRAKRF_DOCS_PAT`
- Copy `docs/api/openapi.public.{json,yaml}` + `trakrf-api.postman_collection.json` into `trakrf-docs/static/api/`
- Commit on a new branch `sync/platform-<short-sha>`
- Open PR titled `chore(api): sync spec from platform@<short-sha>`; body includes commit range and changed files
- Assign reviewer (initially Mike); auto-merge disabled by default so someone eyeballs the rendered Redoc preview before customers see it

### Local developer loop

- `just backend api-spec` regenerates on demand.
- `just backend validate` extends to include `api-spec` + `redocly lint` before its existing `lint test build smoke-test` chain, so local `just validate` catches drift uniformly.
- `just lint` (top level) picks this up via its existing delegation.

---

## Integration with `trakrf-docs`

### Dependencies added

```json
"dependencies": {
  "redocusaurus": "^2.x",
  "redoc": "^2.x"
}
```

Docusaurus 3.9 + React 19 is already on a modern `redocusaurus` release; install the latest compatible major.

### Docusaurus config additions

```ts
presets: [
  ["classic", { /* existing */ }],
  [
    "redocusaurus",
    {
      specs: [
        {
          id: "trakrf-api",
          spec: "static/api/openapi.public.yaml",
          route: "/api",
        },
      ],
      theme: {
        primaryColor: "var(--ifm-color-primary)",
      },
    },
  ],
],
```

### Navbar and sidebar

Keep the existing `apiSidebar`-based `API` entry in the navbar (sibling sub-issue owns its prose content: quickstart, auth guide, pagination, errors, versioning, changelog). Add a second navbar item `API Reference` linking to `/api`. The prose guides live in the sidebar; the Redoc reference is a single route.

Footer `/docs/api/authentication` link already exists in `docusaurus.config.ts` — leave untouched; sibling prose-docs ticket fills that page in.

### Static assets

Shipped by the cross-repo PR:

```
trakrf-docs/static/api/
  openapi.public.json
  openapi.public.yaml
  trakrf-api.postman_collection.json
```

Docusaurus serves `static/*` at site root, so these are reachable at `docs.trakrf.id/api/openapi.public.json` etc. To honor TRA-392's shorter promised URLs (`/api/openapi.json`, `/api/openapi.yaml`), add redirects:

```ts
plugins: [
  [
    "@docusaurus/plugin-client-redirects",
    {
      redirects: [
        { from: "/api/openapi.json", to: "/api/openapi.public.json" },
        { from: "/api/openapi.yaml", to: "/api/openapi.public.yaml" },
      ],
    },
  ],
],
```

### Postman download

Redoc itself doesn't render download-link buttons. Add a small MDX page `docs/api/postman.mdx` (inside the prose sidebar) with a download link to `/api/trakrf-api.postman_collection.json` and brief import instructions. An introduction paragraph on the same page can also link to the Redoc reference at `/api`.

### Build-time impact

Redocusaurus adds ~2s and ~5MB to the Docusaurus build. No runtime cost (static-site).

### PR-driven review

Every cross-repo PR from `platform` produces a `trakrf-docs` preview deploy showing the rendered Redoc with the new spec — this is the human review gate for spec changes. Merging the PR publishes.

---

## Internal `/swagger/*` changes

### Move behind auth

Current state in `backend/internal/cmd/serve/router.go`:

```go
r.Get("/swagger/*", httpSwagger.WrapHandler)     // line 63, unauthenticated
_ "github.com/trakrf/platform/backend/docs"      // imports swag's 2.0 output
```

Replace with, inside the existing authenticated group:

```go
r.Group(func(r chi.Router) {
    r.Use(middleware.Auth)
    r.Use(middleware.SentryContext)
    // ...existing registrations...
    r.Get("/swagger/*", httpSwagger.Handler(
        httpSwagger.URL("/swagger/openapi.internal.json"),
    ))
    r.Get("/swagger/openapi.internal.json", internalSpec.ServeJSON)
    r.Get("/swagger/openapi.internal.yaml", internalSpec.ServeYAML)
})
```

### Spec embedding

New package `backend/internal/handlers/swaggerspec` owns the embedded internal spec:

```go
package swaggerspec

import _ "embed"

//go:embed openapi.internal.json
var InternalJSON []byte

//go:embed openapi.internal.yaml
var InternalYAML []byte
```

The `apispec` tool writes the internal spec directly into `backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}` (gitignored — matches the existing `backend/docs/` policy for regenerated artifacts). The `//go:embed` directive reads from that same directory. Missing file = compile error, which means `just backend build` now implicitly depends on `just backend api-spec` having run. Add `api-spec` as a `build` prerequisite:

```
build: api-spec
    go build -ldflags "-X main.version=0.1.0-dev" -o bin/trakrf .
```

### Drop the `_ "backend/docs"` import

The current import pattern brings swaggo's generated `docs/swagger.json` into Swagger UI via a global registration. Replaced by the explicit embedded 3.0 internal spec above. One line removed from router imports.

### Swagger UI compatibility

`httpSwagger`'s bundled Swagger UI v5 reads OpenAPI 3.0 natively. The rendered `/swagger/*` page looks the same but reads 3.0 and shows both `public` and `internal` tag groups. Tag names are visible to internal viewers — that's intentional; devs need them to pick annotations correctly.

### Future RBAC

Per brainstorming note: future enhancement is a `middleware.RequireRole("admin")` on write operations surfaced in the internal Swagger UI, so non-admins see read-only ops only. v1.x ticket; not blocking TRA-394.

---

## Drift detection & validation

Three checks, all wired into existing workflows:

1. **Public-spec drift** — `git diff --exit-code docs/api/` after `just backend api-spec` in the `api-spec.yml` PR workflow. Failure surfaces "Annotations changed; run `just backend api-spec` and commit the result."
2. **Spec validity** — `pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml --extends=recommended`. Same workflow. Redocly CLI (free, MIT) validates against the OpenAPI 3.0 schema and catches missing `operationId`, empty `description`, orphan `$ref`, etc. Matches what `trakrf-docs`'s Redoc reads, so failures block before reaching the docs repo.
3. **Internal-spec build gate** — `just backend build` depends on `just backend api-spec`; the `go:embed` directive fails compilation if the internal spec is absent. No extra CI step.

**Extended `just backend validate`:** `api-spec + redocly lint + lint + test + build + smoke-test`. Local `just validate` behaves identically to CI.

**Not building:**

- Contract testing (Dredd, Schemathesis). Overkill for v1.
- Semver / diff automation. Additive-only v1 per TRA-392 §F-1; automate ahead of first deprecation.
- Spectral rule packs beyond Redocly `recommended`. Start simple; tighten when customer feedback warrants.

---

## Enum openness audit

TRA-392 open question #3 assigned enum-openness labeling to TRA-394. The table below covers every enum in the v1 public surface.

### Convention

- **Closed** enums → plain OpenAPI `enum: [...]`. Clients treat unknown values as a server bug.
- **Open** enums → `enum: [...]` plus `x-extensible-enum: true` (widely recognized vendor extension; Redoc renders a banner note). Clients must handle unknown values gracefully.

### Audit

| Location | Current values | Openness | Rationale |
|---|---|---|---|
| `Asset.type` | `"asset"` | **open** | Roadmap includes `tool`, `container`, etc.; clients shouldn't break when types expand |
| `sort` on `/assets` | `identifier`, `name`, `created_at`, `updated_at` | **closed** | Adding a sort field is additive; clients send a single string, don't enumerate |
| `sort` on `/locations` | `path`, `identifier`, `name`, `created_at` | **closed** | Same reasoning |
| `sort` on `/scans` | `timestamp` | **closed** | Same |
| `error.type` | `validation_error`, `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `rate_limited`, `internal_error` | **open** | TRA-392 §D-3 adds `rate_limited`; future categories (`locked`, `quota_exceeded`) inevitable |
| `error.fields[].code` | `required`, `invalid_value`, `too_short`, `too_long`, `invalid_format`, `out_of_range` | **open** | TRA-392 §D-2 already labeled open; new validation rules add new codes |
| API key `scopes` (in `/orgs/me` response) | `assets:read`, `assets:write`, `locations:read`, `locations:write`, `scans:read` | **open** | Scope set grows with endpoint surface |
| HTTP status codes per endpoint | per RFC 7231 + TRA-392 §Errors catalog | **closed** | TRA-392 §F-1 commitment |
| Filter allowlists per resource | per TRA-392 §Filtering | **closed** (allowlist itself) | Unknown query param returns 400; new filter names are additive |

### Description blurb on open enums

Every open enum field carries this sentence in its `description`:

> This enum is extensible. Clients should handle unknown values gracefully; TrakRF may add new values in any v1 release without a breaking-change bump.

### v1.1 pre-committed (noted here, annotated when v1.1 lands)

- Aggregate `interval` — `1m, 5m, 15m, 1h, 6h, 1d, 1w` — **closed** per TRA-392 §J-1. Finite set deliberately chosen to prevent pathological bucket counts.

---

## Acceptance criteria

- `just backend api-spec` produces committed `docs/api/openapi.public.{json,yaml}` and `docs/api/trakrf-api.postman_collection.json` from annotations alone.
- Every public-tagged handler has: `@Summary`, `@Description`, `@Security APIKey [<scope>]`, `@Failure` lines for applicable error types, and enum annotations per §Enum openness.
- `pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml --extends=recommended` passes.
- `/swagger/*` is reachable only with a valid session JWT; unauthenticated requests return 401.
- `publish-api-docs.yml` opens a cross-repo PR against `trakrf/docs` on main-merge; the PR's preview deploy renders Redoc at `/api`, spec is reachable at `/api/openapi.{json,yaml}`, Postman collection downloadable via the `postman.mdx` prose page.
- Drift check (`git diff --exit-code docs/api/` after `just backend api-spec`) passes in CI on PRs touching `backend/**`.
- Enum audit table from §Enum openness is reflected in the shipped spec (closed → `enum`, open → `enum` + `x-extensible-enum: true`).

---

## Non-goals and v1.x follow-ups

Out of scope for TRA-394 but shape-committed:

1. **Language SDK examples** (Python, JavaScript, Go) — customer demand drives.
2. **Contract testing** — second consumer drives.
3. **Semver / diff automation** — first deprecation cycle drives.
4. **Docusaurus versioning** (`/api/v1` and `/api/v2` side-by-side) — v2 parallel-run drives.
5. **RBAC on `/swagger/*`** — non-admins see read-only operations; admins see everything. Keeps the internal UI useful without handing write ops to every dev.
6. **`x-codeSamples` hand-crafted snippets** — Redoc's native generation is adequate for v1; curated samples wait for customer feedback.

Deferred to sibling tickets:

- **TRA-396** — read-only endpoint wiring and natural-key path conversions. TRA-394 annotates at-merge paths; whichever ships first drives the other's path parameters.
- **TRA-393** — API key management (table, middleware, UI). TRA-394 documents the `APIKey` security scheme; backing implementation is TRA-393's.
- **Customer-facing API prose docs sub-issue** (to be created per TRA-392 §Related work) — quickstart, auth guide, pagination guide, errors guide, versioning policy, CHANGELOG. Lives in `trakrf-docs/docs/api/*.md`, renders in the prose sidebar beside the Redoc reference.
- **Platform semver + release versioning discipline sub-issue** — independent axis from `/api/v1` contract version.

---

## Decisions log

### A. Architecture

- **A-1.** Single swaggo source → two filtered specs (public, internal). No dual-annotation system, no hand-maintained spec fragments.
- **A-2.** Public spec is committed to `platform`; internal spec is gitignored and regenerated per-build.
- **A-3.** Internal spec is embedded into the backend binary via `go:embed`; `/swagger/*` reads from the binary, not the filesystem.

### B. Toolchain

- **B-1.** Keep swaggo 2.0 as source; convert to OpenAPI 3.0 in CI via `github.com/getkin/kin-openapi`. swag's experimental `--v3.1` flag revisited later.
- **B-2.** Conversion + filtering + post-processing in a single in-repo Go binary (`backend/internal/tools/apispec`). Keeps backend build dependency-free of Node.
- **B-3.** Postman collection generated via `pnpm dlx openapi-to-postmanv2` as a separate CI step; Node only appears in CI, not in `go build`.

### C. Rendering

- **C-1.** Redoc (via `redocusaurus`) inside the `trakrf-docs` Docusaurus portal at `/api`. Not a standalone Redoc HTML, not embedded in `platform`.
- **C-2.** Raw spec served as Docusaurus `static/` assets at `/api/openapi.public.{json,yaml}`; redirects at `/api/openapi.{json,yaml}` honor the TRA-392-promised shorter URLs.
- **C-3.** Postman collection linked from an MDX page under the prose API sidebar; Redoc itself is not modified to host the link.
- **C-4.** Curl samples via Redoc's native auto-generation; no `x-codeSamples` annotations in v1.

### D. Delivery

- **D-1.** Cross-repo PR flow — `platform` main-merge opens a PR in `trakrf-docs` with updated spec and Postman files. Preview deploy of that PR is the human review gate.
- **D-2.** Cross-repo auth via `TRAKRF_DOCS_PAT` GitHub secret. GitHub App upgrade is a follow-up if PAT rotation becomes burdensome.
- **D-3.** Auto-merge disabled by default; reviewer eyeballs the rendered preview before spec changes reach customers.

### E. Tag filtering

- **E-1.** Pre-filter in CI into two distinct specs. No client-side tag hiding. Internal endpoint names never appear in the customer-facing spec file.
- **E-2.** Every operation must carry exactly one of `@Tags public` or `@Tags internal`. `apispec` fails with a clear error if an operation is missing or has both.

### F. Internal `/swagger/*`

- **F-1.** Move `/swagger/*` behind `middleware.Auth`. No unauthenticated access in any environment.
- **F-2.** Future RBAC (admin-vs-non-admin filtering) logged as v1.x follow-up; v1 is single-tier auth.
- **F-3.** Swagger UI reads the embedded internal 3.0 spec; the old `_ "backend/docs"` import is removed.

### G. Drift detection

- **G-1.** `git diff --exit-code docs/api/` after `just backend api-spec` on every backend-touching PR.
- **G-2.** `@redocly/cli lint --extends=recommended` on every backend-touching PR.
- **G-3.** `just backend validate` extended with `api-spec + redocly lint` steps; local and CI behavior identical.

### H. Enum openness

- **H-1.** Every public-surface enum labeled open or closed per the audit table; open enums annotated with `x-extensible-enum: true` and a standard description blurb.
- **H-2.** `Asset.type`, `error.type`, `error.fields[].code`, and API key `scopes` are open. Per-resource `sort`, HTTP status codes, and filter allowlists are closed.
- **H-3.** v1.1 aggregate `interval` pre-committed closed.

### I. Annotation policy

- **I-1.** Every public operation carries `@Summary`, `@Description`, `@Security APIKey [<scope>]`, and `@Failure` for each applicable error type.
- **I-2.** Public paths use `{identifier}` where TRA-392 §A-3 specifies; annotations track whichever state TRA-396 has reached at merge time and the later-merging ticket updates the other.
- **I-3.** Security scheme declared once in `main.go` swag header; `apispec` post-processes the 2.0 `apiKey` emission into 3.0 HTTP-Bearer with `bearerFormat: JWT`.
