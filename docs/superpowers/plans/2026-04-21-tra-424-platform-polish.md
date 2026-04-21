# TRA-424 Platform Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship two PRs fixing the platform-side findings from the 2026-04-21 black-box evaluation: OpenAPI spec served from API host, HEAD request support, missing Header page titles, and API-key disambiguation.

**Architecture:** PR 1 (backend): embed `openapi.public.{json,yaml}` alongside the existing internal spec; serve at `/api/v1/openapi.{json,yaml}` with root-path redirects; add chi's `GetHead` middleware globally; expose `jti` on the API-key list response. PR 2 (frontend): fill in `Header.pageTitles` for all main-shell routes and render the JTI as a muted chip in the API Keys list.

**Tech Stack:** Go 1.25, chi v5, go-embed, Testify, React/TypeScript, Vitest, Testing Library.

**Spec:** [`docs/superpowers/specs/2026-04-21-tra-424-platform-polish-design.md`](../specs/2026-04-21-tra-424-platform-polish-design.md)

**Branches:**
- PR 1: `feature/tra-424-platform-polish` (already created; design doc committed)
- PR 2: create `feature/tra-424-ui-polish` after PR 1 opens, based on `feature/tra-424-platform-polish` so it picks up the `jti` field; rebase onto `main` after PR 1 merges.

**Run commands from repo root** using `just backend <cmd>` and `just frontend <cmd>`.

---

## PR 1 — Backend

### Task B1: Wire build/embed pipeline for the public spec

**Rationale:** `//go:embed openapi.public.{json,yaml}` only resolves if those files sit beside `swaggerspec.go` at build time. Get the pipeline producing them there — for local builds, Docker dev stage, and Docker prod stage — before writing any new handler code. This task has no test of its own; verification is that `just backend api-spec` produces the expected files and `just backend build` still succeeds.

**Files:**
- Modify: `.gitignore` (add public-spec embed artifacts)
- Modify: `backend/justfile` (api-spec recipe)
- Modify: `backend/Dockerfile` (development + builder stages)

- [ ] **Step 1: Add gitignore entry for the embedded public spec**

The committed public spec lives at `docs/api/openapi.public.{json,yaml}`. The embed-target copy in the backend package must be gitignored (mirrors existing internal treatment).

Open `.gitignore` and find the existing line (~139):

```
backend/internal/handlers/swaggerspec/openapi.internal.*
```

Add a second line immediately below it:

```
backend/internal/handlers/swaggerspec/openapi.public.*
```

- [ ] **Step 2: Update `backend/justfile` `api-spec` recipe to copy public spec into the embed dir**

Open `backend/justfile`. Replace the current `api-spec` recipe (lines ~67-77) with:

```justfile
# Generate OpenAPI 3.0 specs (public → docs/api/ + embedded; internal → embedded only)
api-spec:
    @echo "📚 Generating OpenAPI 3.0 specs..."
    swag init -g main.go --parseDependency --parseInternal -o docs
    @mkdir -p internal/handlers/swaggerspec ../docs/api
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out ../docs/api/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal
    @cp ../docs/api/openapi.public.json internal/handlers/swaggerspec/openapi.public.json
    @cp ../docs/api/openapi.public.yaml internal/handlers/swaggerspec/openapi.public.yaml
    @echo "✅ Public spec:   docs/api/openapi.public.{json,yaml}  (committed) + swaggerspec/ (gitignored, embedded)"
    @echo "✅ Internal spec: backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}  (gitignored, embedded)"
```

- [ ] **Step 3: Update `backend/Dockerfile` development stage to emit public spec into embed dir**

In `backend/Dockerfile`, find the development stage block (lines 18-24):

```dockerfile
# Generate OpenAPI specs — required by //go:embed in internal/handlers/swaggerspec
RUN swag init -g main.go --parseDependency --parseInternal && \
    mkdir -p internal/handlers/swaggerspec && \
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out /tmp/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal
```

Replace with:

```dockerfile
# Generate OpenAPI specs — required by //go:embed in internal/handlers/swaggerspec
RUN swag init -g main.go --parseDependency --parseInternal && \
    mkdir -p internal/handlers/swaggerspec && \
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out internal/handlers/swaggerspec/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal
```

- [ ] **Step 4: Update `backend/Dockerfile` production builder stage the same way**

Find the production builder stage block (lines 46-52) with the identical `RUN swag init` command and apply the same change — replace `--public-out /tmp/openapi.public` with `--public-out internal/handlers/swaggerspec/openapi.public`.

- [ ] **Step 5: Regenerate specs locally to populate the embed target**

Run: `just backend api-spec`

Expected: no errors. Two new files should appear at `backend/internal/handlers/swaggerspec/openapi.public.{json,yaml}` (gitignored) and `docs/api/openapi.public.{json,yaml}` should still exist (tracked).

Verify with `git status` — only the modified `.gitignore`, `justfile`, and `Dockerfile` should be staged candidates; no new untracked `openapi.public.*` files.

- [ ] **Step 6: Confirm build still works**

Run: `just backend build`

Expected: successful build producing `backend/bin/trakrf`. The existing `swaggerspec` package still only embeds the internal spec, so the public files sit on disk unused until Task B2.

- [ ] **Step 7: Commit**

```bash
git add .gitignore backend/justfile backend/Dockerfile
git commit -m "chore(tra-424): emit public OpenAPI spec into swaggerspec embed dir

Prepares for embedding openapi.public.{json,yaml} in the Go binary. The
justfile api-spec recipe now copies the public spec into the swaggerspec
package alongside the internal spec; Docker dev + prod stages emit
directly into the embed dir; .gitignore mirrors the existing internal
treatment (embed target is gitignored; canonical copy stays in docs/api/).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B2: Embed and serve the public OpenAPI spec (JSON)

**Files:**
- Modify: `backend/internal/handlers/swaggerspec/swaggerspec.go`
- Create: `backend/internal/handlers/swaggerspec/swaggerspec_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/handlers/swaggerspec/swaggerspec_test.go` with:

```go
package swaggerspec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServePublicJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()

	ServePublicJSON(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var spec map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &spec), "body must be valid JSON")
	require.Contains(t, spec, "openapi", "spec must contain top-level openapi field")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/handlers/swaggerspec/ -run TestServePublicJSON -v`

Expected: FAIL — `ServePublicJSON` is undefined.

- [ ] **Step 3: Implement embed + handler**

Open `backend/internal/handlers/swaggerspec/swaggerspec.go`. Replace the file contents with:

```go
// Package swaggerspec embeds the OpenAPI 3.0 specs generated by the apispec
// tool and serves them over HTTP. Two specs live here:
//   - openapi.internal.{json,yaml}: full surface including session-only routes,
//     used by the internal Swagger UI.
//   - openapi.public.{json,yaml}: the API-key-authenticated public surface,
//     served at /api/v1/openapi.{json,yaml} for integrators and codegen tools.
//
// Both specs are regenerated on every build; see just backend api-spec.
package swaggerspec

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.internal.json
var internalJSON []byte

//go:embed openapi.internal.yaml
var internalYAML []byte

//go:embed openapi.public.json
var publicJSON []byte

//go:embed openapi.public.yaml
var publicYAML []byte

// ServeJSON writes the embedded internal OpenAPI spec as JSON.
func ServeJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(internalJSON)
}

// ServeYAML writes the embedded internal OpenAPI spec as YAML.
func ServeYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(internalYAML)
}

// ServePublicJSON writes the embedded public OpenAPI spec as JSON.
func ServePublicJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(publicJSON)
}

// ServePublicYAML writes the embedded public OpenAPI spec as YAML.
func ServePublicYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(publicYAML)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/handlers/swaggerspec/ -run TestServePublicJSON -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/swaggerspec/swaggerspec.go backend/internal/handlers/swaggerspec/swaggerspec_test.go
git commit -m "feat(tra-424): embed public OpenAPI spec and serve as JSON

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B3: Serve the public OpenAPI spec (YAML)

**Files:**
- Modify: `backend/internal/handlers/swaggerspec/swaggerspec_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/handlers/swaggerspec/swaggerspec_test.go`:

```go
func TestServePublicYAML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec := httptest.NewRecorder()

	ServePublicYAML(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/yaml", rec.Header().Get("Content-Type"))
	require.NotEmpty(t, rec.Body.Bytes(), "body must be non-empty")
	require.Contains(t, rec.Body.String(), "openapi:", "body should contain YAML key 'openapi:'")
}
```

- [ ] **Step 2: Run test to verify it passes**

The YAML handler was already added in Task B2 — this test just locks in behavior.

Run: `cd backend && go test ./internal/handlers/swaggerspec/ -run TestServePublicYAML -v`

Expected: PASS.

- [ ] **Step 3: Run the full swaggerspec package tests**

Run: `cd backend && go test ./internal/handlers/swaggerspec/ -v`

Expected: both `TestServePublicJSON` and `TestServePublicYAML` pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/swaggerspec/swaggerspec_test.go
git commit -m "test(tra-424): cover public OpenAPI YAML handler

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B4: Register public spec routes in the router

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`
- Modify: `backend/internal/cmd/serve/serve_test.go`

- [ ] **Step 1: Write the failing test**

Open `backend/internal/cmd/serve/serve_test.go`. Find `TestRouterRegistration` (the table-driven test around line 63+). Add two entries to its `tests` slice — one for each path:

```go
{"GET", "/api/v1/openapi.json"},
{"GET", "/api/v1/openapi.yaml"},
```

Then add a new standalone test at the end of the file:

```go
func TestPublicOpenAPISpec_ServedAt_V1Path(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/openapi.json = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"openapi"`) {
		t.Fatalf("body does not contain \"openapi\" key: %s", rec.Body.String()[:min(200, len(rec.Body.String()))])
	}
}
```

(If `min` isn't already imported as a Go 1.21+ built-in in this package, the test can drop the slice and just use `rec.Body.String()`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/cmd/serve/ -run TestPublicOpenAPISpec_ServedAt_V1Path -v`

Expected: FAIL with 404 or similar (routes not yet registered).

- [ ] **Step 3: Register the routes in `setupRouter`**

Open `backend/internal/cmd/serve/router.go`. Find the existing `/metrics` registration around line 66:

```go
r.Handle("/metrics", promhttp.Handler())

healthHandler.RegisterRoutes(r)
```

Insert public-spec route registrations between them, and add the swaggerspec import at top if not already there (it is). The new block:

```go
r.Handle("/metrics", promhttp.Handler())

// Public OpenAPI spec — served unauthenticated so codegen tools and
// integrators can fetch it directly from the API host. Root-path aliases
// (/openapi.{json,yaml}) are added below.
r.Get("/api/v1/openapi.json", swaggerspec.ServePublicJSON)
r.Get("/api/v1/openapi.yaml", swaggerspec.ServePublicYAML)

healthHandler.RegisterRoutes(r)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/cmd/serve/ -run 'TestPublicOpenAPISpec_ServedAt_V1Path|TestRouterRegistration' -v`

Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/cmd/serve/serve_test.go
git commit -m "feat(tra-424): serve public OpenAPI spec at /api/v1/openapi.{json,yaml}

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B5: Root-path redirects for spec discovery

**Rationale:** Codegen tools and developers typically probe `/openapi.json` at the root. Redirect to the versioned path so the binary can serve them without ambiguity.

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`
- Modify: `backend/internal/cmd/serve/serve_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/cmd/serve/serve_test.go`:

```go
func TestOpenAPISpec_RootRedirect_JSON(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("GET /openapi.json = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/openapi.json" {
		t.Fatalf("Location = %q, want /api/v1/openapi.json", loc)
	}
}

func TestOpenAPISpec_RootRedirect_YAML(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("GET /openapi.yaml = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/openapi.yaml" {
		t.Fatalf("Location = %q, want /api/v1/openapi.yaml", loc)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/cmd/serve/ -run 'TestOpenAPISpec_RootRedirect' -v`

Expected: FAIL. Most likely the SPA frontend handler serves `index.html` at these paths with 200.

- [ ] **Step 3: Register the redirects**

In `backend/internal/cmd/serve/router.go`, below the versioned `/api/v1/openapi.*` registrations added in Task B4, add:

```go
// Root-path aliases for codegen tools that probe /openapi.{json,yaml}.
// Registered before any SPA catchall so the redirect wins.
r.Get("/openapi.json", func(w http.ResponseWriter, req *http.Request) {
    http.Redirect(w, req, "/api/v1/openapi.json", http.StatusFound)
})
r.Get("/openapi.yaml", func(w http.ResponseWriter, req *http.Request) {
    http.Redirect(w, req, "/api/v1/openapi.yaml", http.StatusFound)
})
```

The block should now look like:

```go
r.Handle("/metrics", promhttp.Handler())

// Public OpenAPI spec — served unauthenticated so codegen tools and
// integrators can fetch it directly from the API host. Root-path aliases
// (/openapi.{json,yaml}) are added below.
r.Get("/api/v1/openapi.json", swaggerspec.ServePublicJSON)
r.Get("/api/v1/openapi.yaml", swaggerspec.ServePublicYAML)

// Root-path aliases for codegen tools that probe /openapi.{json,yaml}.
// Registered before any SPA catchall so the redirect wins.
r.Get("/openapi.json", func(w http.ResponseWriter, req *http.Request) {
    http.Redirect(w, req, "/api/v1/openapi.json", http.StatusFound)
})
r.Get("/openapi.yaml", func(w http.ResponseWriter, req *http.Request) {
    http.Redirect(w, req, "/api/v1/openapi.yaml", http.StatusFound)
})

healthHandler.RegisterRoutes(r)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/cmd/serve/ -run 'TestOpenAPISpec_RootRedirect' -v`

Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/cmd/serve/serve_test.go
git commit -m "feat(tra-424): redirect /openapi.{json,yaml} to /api/v1/ path

Codegen tools (OpenAPI Generator, Postman) probe root-level paths by
convention. Registered before the SPA catchall so the redirect wins.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B6: Global HEAD support via chi's GetHead middleware

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`
- Modify: `backend/internal/cmd/serve/serve_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/cmd/serve/serve_test.go`:

```go
func TestHeadRequestMatches_OpenAPISpec(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodHead, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD /api/v1/openapi.json = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}
```

The openapi endpoint is used for the HEAD test because it's unauthenticated — no fixture setup needed to prove HEAD matches a GET route.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/cmd/serve/ -run TestHeadRequestMatches_OpenAPISpec -v`

Expected: FAIL with 404 or 405 — chi does not auto-match HEAD to GET.

- [ ] **Step 3: Add chi's GetHead middleware**

Open `backend/internal/cmd/serve/router.go`. Add the aliased import (chi's middleware package collides with the project's own `middleware` package). Find the import block at the top and add:

```go
chimiddleware "github.com/go-chi/chi/v5/middleware"
```

Place it alphabetically near the other chi import (line 12 area).

Then in `setupRouter`, find the middleware block (lines ~52-57):

```go
r.Use(middleware.RequestID)
r.Use(logger.Middleware)
r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)
r.Use(middleware.Recovery)
r.Use(middleware.CORS)
r.Use(middleware.ContentType)
```

Add `chimiddleware.GetHead` as the last `Use` in the block:

```go
r.Use(middleware.RequestID)
r.Use(logger.Middleware)
r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)
r.Use(middleware.Recovery)
r.Use(middleware.CORS)
r.Use(middleware.ContentType)
r.Use(chimiddleware.GetHead)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/cmd/serve/ -run TestHeadRequestMatches_OpenAPISpec -v`

Expected: PASS.

- [ ] **Step 5: Run the full serve package tests to confirm no regressions**

Run: `cd backend && go test ./internal/cmd/serve/ -v`

Expected: all existing tests still pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/cmd/serve/serve_test.go
git commit -m "feat(tra-424): add chi GetHead middleware for HEAD support

Rewrites HEAD to GET before route matching; Go's http.Server strips the
response body automatically. Applies uniformly to every GET route.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B7: Expose JTI on API-key list response

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go`
- Modify: `backend/internal/handlers/orgs/api_keys.go`
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go`

- [ ] **Step 1: Write the failing test**

Open `backend/internal/handlers/orgs/api_keys_integration_test.go`. Find the existing list test (around line 147, the test that asserts `active.ID, out.Data[0].ID`). Immediately after that assertion, add:

```go
	assert.NotEmpty(t, out.Data[0].JTI, "list response must include jti for disambiguation")
	assert.Equal(t, active.JTI, out.Data[0].JTI, "jti in response should match the stored row")
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/handlers/orgs/ -run TestListAPIKeys -v` (or the exact name of the list test — grep `func Test.*ListAPIKeys` in that file).

Expected: FAIL — `out.Data[0].JTI` is an undefined field on `APIKeyListItem`.

- [ ] **Step 3: Add JTI to APIKeyListItem**

Open `backend/internal/models/apikey/apikey.go`. Find `APIKeyListItem` (line ~47). Add `JTI` as the second field (right after `ID`, before `Name`):

```go
// APIKeyListItem is what GET returns — never includes the JWT.
type APIKeyListItem struct {
	ID         int        `json:"id"`
	JTI        string     `json:"jti"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}
```

- [ ] **Step 4: Populate JTI in the handler**

Open `backend/internal/handlers/orgs/api_keys.go`. Find `ListAPIKeys` (line ~94). Update the struct literal inside the loop (line ~113):

```go
items = append(items, apikey.APIKeyListItem{
    ID:         k.ID,
    JTI:        k.JTI,
    Name:       k.Name,
    Scopes:     k.Scopes,
    CreatedAt:  k.CreatedAt,
    ExpiresAt:  k.ExpiresAt,
    LastUsedAt: k.LastUsedAt,
})
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd backend && go test ./internal/handlers/orgs/ -run TestListAPIKeys -v`

Expected: PASS.

- [ ] **Step 6: Run full backend tests to confirm no regressions**

Run: `just backend test`

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/models/apikey/apikey.go backend/internal/handlers/orgs/api_keys.go backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "feat(tra-424): expose jti on API key list response

The jti (JWT ID) is already embedded in the issued token's payload, so
it is not a secret. Surfacing it on the list lets the UI show a stable
non-secret identifier for disambiguation without a schema migration.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B8: Regenerate public OpenAPI spec and verify drift is legitimate

**Rationale:** The swaggerspec handlers are not part of the public API surface — they have no `@Tags ,public` annotation, so they should not appear in `openapi.public.{json,yaml}`. The JTI field addition *does* change the list endpoint's response schema, but the API key management endpoints are session-auth only and are already tagged internal, so the public spec should still be unchanged. Regenerate and verify.

**Files:**
- (potentially) Modify: `docs/api/openapi.public.{json,yaml}` — only if generated output differs

- [ ] **Step 1: Regenerate specs**

Run: `just backend api-spec`

- [ ] **Step 2: Check for drift**

Run: `git status docs/api/openapi.public.*`

- If the files are unchanged: skip commit, proceed to Task B9.
- If they changed: inspect the diff with `git diff docs/api/openapi.public.yaml`. The diff should be limited to whatever the generator now naturally produces for unrelated changes (likely none). If the diff adds/removes routes related to this work, something is wrong — the new openapi.json handlers must not be in the public spec. Stop and investigate before committing.

- [ ] **Step 3: Commit (only if there is drift)**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "docs(tra-424): regenerate public OpenAPI spec

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task B9: Push PR 1 and open for review

- [ ] **Step 1: Run full validation**

Run: `just validate`

Expected: lint + test + build + smoke pass on both workspaces.

- [ ] **Step 2: Push branch**

```bash
git push -u origin feature/tra-424-platform-polish
```

- [ ] **Step 3: Open PR 1**

```bash
gh pr create --title "feat(tra-424): OpenAPI spec discoverability, HEAD support, API key JTI" --body "$(cat <<'EOF'
## Summary
- Embed `openapi.public.{json,yaml}` in the Go binary and serve at `/api/v1/openapi.{json,yaml}`; 302 redirect `/openapi.{json,yaml}` → versioned path for codegen tools.
- Add chi's `GetHead` middleware globally so HEAD requests match GET routes (response body is stripped by the stdlib server).
- Expose `jti` on the API key list response — enables UI disambiguation without a schema migration. The JTI is already in the JWT payload, so it is not secret.

Scope notes: Finding #1 from TRA-424 was already fixed by TRA-407 before this branch; dropped. Finding #5's "key_prefix" premise was incorrect — JTI chosen as the minimal-change alternative.

Spec: docs/superpowers/specs/2026-04-21-tra-424-platform-polish-design.md
Plan: docs/superpowers/plans/2026-04-21-tra-424-platform-polish.md

## Test plan
- [x] `just backend test` — full backend suite
- [x] `just backend build` — binary builds cleanly with new embeds
- [x] `just validate` — lint + test + build + smoke across both workspaces
- [ ] Preview deploy: `curl https://app.preview.trakrf.id/api/v1/openapi.json | jq .openapi` returns `"3.1.0"` (or current version)
- [ ] Preview deploy: `curl -I https://app.preview.trakrf.id/openapi.json` returns `302 Found` with `Location: /api/v1/openapi.json`
- [ ] Preview deploy: `curl -I -H "X-API-Key: ..." https://app.preview.trakrf.id/api/v1/assets` returns `200` with no body

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## PR 2 — Frontend

**Branch:** base on `feature/tra-424-platform-polish` initially (so the `jti` field is in the TypeScript type definitions' expected backend shape). After PR 1 merges, rebase onto `main`.

- [ ] **Step 0: Branch**

```bash
git checkout feature/tra-424-platform-polish
git checkout -b feature/tra-424-ui-polish
```

---

### Task F1: Display JTI chip in API Keys list (TDD)

**Files:**
- Modify: `frontend/src/types/apiKey.ts`
- Modify: `frontend/src/components/APIKeysScreen.tsx`
- Modify: `frontend/src/components/APIKeysScreen.test.tsx`

- [ ] **Step 1: Write the failing test**

Open `frontend/src/components/APIKeysScreen.test.tsx`. Find the test `'lists existing keys with name and scopes'` (around line 43). Replace the fixture `data` entry and add an assertion:

```tsx
  it('lists existing keys with name and scopes', async () => {
    (apiKeysApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: [
        {
          id: 1,
          jti: 'a1b2c3d4-5678-90ab-cdef-1234567890ab',
          name: 'TeamCentral',
          scopes: ['assets:read', 'assets:write', 'locations:read'],
          created_at: '2026-04-01T00:00:00Z',
          expires_at: null,
          last_used_at: null,
        },
      ],
    });
    wrap(<APIKeysScreen />);
    await waitFor(() => expect(screen.getByText('TeamCentral')).toBeInTheDocument());
    expect(screen.getByText(/Assets R\/W/)).toBeInTheDocument();
    expect(screen.getByText('a1b2c3d4')).toBeInTheDocument();
  });
```

The fixture's `jti` adds the new field; the new `expect(screen.getByText('a1b2c3d4'))` assertion locks in the first-8-char chip rendering.

- [ ] **Step 2: Run test to verify it fails**

Run: `just frontend test APIKeysScreen`

Expected: FAIL — `getByText('a1b2c3d4')` finds no element; TypeScript may also complain about the `jti` field.

- [ ] **Step 3: Add `jti` to the APIKey type**

Open `frontend/src/types/apiKey.ts`. Update the `APIKey` interface:

```ts
export interface APIKey {
  id: number;
  jti: string;
  name: string;
  scopes: Scope[];
  created_at: string;
  expires_at: string | null;
  last_used_at: string | null;
}
```

- [ ] **Step 4: Render the JTI chip in the list**

Open `frontend/src/components/APIKeysScreen.tsx`. Find the name cell in the keys table (around line 128):

```tsx
<td className="py-2 font-medium">{k.name}</td>
```

Replace with:

```tsx
<td className="py-2 font-medium">
  <div className="flex items-baseline gap-2">
    <span>{k.name}</span>
    <span className="font-mono text-xs text-gray-500 dark:text-gray-400" title={`Key ID: ${k.jti}`}>
      {k.jti.slice(0, 8)}
    </span>
  </div>
</td>
```

The `title` attribute gives hover-to-see-full-jti. The chip sits inline with the name so the existing row layout is unchanged.

- [ ] **Step 5: Run the test to verify it passes**

Run: `just frontend test APIKeysScreen`

Expected: PASS.

- [ ] **Step 6: Run frontend typecheck**

Run: `just frontend typecheck`

Expected: PASS. If any other code reads `APIKey` and a `jti`-less literal is used as a fixture somewhere, update those fixtures too.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/types/apiKey.ts frontend/src/components/APIKeysScreen.tsx frontend/src/components/APIKeysScreen.test.tsx
git commit -m "feat(tra-424): display key JTI chip in API Keys list

Shows the first 8 chars of the JWT ID as a muted monospace chip next to
the user-supplied name. Full JTI available via title tooltip. Lets admins
disambiguate keys with similar names without relying on the show-once dialog.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task F2: Fill in Header.pageTitles for all main-shell routes + blank fallback

**Files:**
- Modify: `frontend/src/components/Header.tsx`
- Create: `frontend/src/components/Header.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/Header.test.tsx`:

```tsx
import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Header } from './Header';
import { useUIStore } from '@/stores/uiStore';

describe('Header page titles', () => {
  beforeEach(() => {
    // Reset store between tests; tailor to uiStore's actual API.
    useUIStore.setState({ activeTab: 'home' });
  });

  it.each([
    ['home', 'Dashboard'],
    ['inventory', 'Inventory'],
    ['api-keys', 'API Keys'],
    ['reports-history', 'Report History'],
    ['org-members', 'Members'],
    ['org-settings', 'Organization Settings'],
    ['create-org', 'Create Organization'],
    ['accept-invite', 'Accept Invite'],
  ])('renders %s tab with title %s', (tab, expectedTitle) => {
    useUIStore.setState({ activeTab: tab });
    render(<Header />);
    expect(screen.getByText(expectedTitle)).toBeInTheDocument();
  });

  it('renders empty title for an unknown tab (fallback is blank, not "Inventory")', () => {
    useUIStore.setState({ activeTab: 'some-unknown-future-tab' });
    const { container } = render(<Header />);
    // Inventory must not appear as a lying fallback.
    expect(screen.queryByText('Inventory')).not.toBeInTheDocument();
    // Subtitle that belonged to inventory must not appear either.
    expect(screen.queryByText('View and manage your scanned items')).not.toBeInTheDocument();
    // Sanity check: header still renders structurally.
    expect(container.firstChild).not.toBeNull();
  });
});
```

**Note:** The `<Header>` component takes `onMenuToggle` and `isMobileMenuOpen` props. If rendering without them produces a runtime error, pass them as `onMenuToggle={() => {}} isMobileMenuOpen={false}`. Also check if `Header` needs stores beyond `useUIStore` to mount (auth, device) — inspect existing tests in the codebase (e.g., `frontend/src/components/OrgSettingsScreen.test.tsx`) for the setup pattern; mock or seed as needed. Keep mocks minimal — the test only needs the title text to render.

- [ ] **Step 2: Run test to verify it fails**

Run: `just frontend test Header`

Expected: FAIL on the `api-keys`, `reports-history`, `org-members`, `org-settings`, `create-org`, `accept-invite` cases (all show "Inventory" currently) and on the "empty title for unknown tab" case.

- [ ] **Step 3: Fix `pageTitles` map and fallback in `Header.tsx`**

Open `frontend/src/components/Header.tsx`. Find the `pageTitles` object (line ~71):

```tsx
const pageTitles = {
  home: { title: "Dashboard", subtitle: "Choose your scanning mode to get started" },
  inventory: { title: "Inventory", subtitle: "View and manage your scanned items" },
  locate: { title: "Locate", subtitle: "Search for a specific item" },
  barcode: { title: "Barcode Scanner", subtitle: "Scan barcodes to identify items" },
  settings: { title: "Device Setup", subtitle: "Configure your RFID reader" },
  help: { title: "Help", subtitle: "Quick answers to get you started" },
  assets: { title: "Assets", subtitle: "Manage your organization's assets" },
  locations: { title: "Locations", subtitle: "Manage your organization's locations" },
  reports: { title: "Reports", subtitle: "View asset locations and movement history" }
};
```

Replace with:

```tsx
const pageTitles = {
  home: { title: "Dashboard", subtitle: "Choose your scanning mode to get started" },
  inventory: { title: "Inventory", subtitle: "View and manage your scanned items" },
  locate: { title: "Locate", subtitle: "Search for a specific item" },
  barcode: { title: "Barcode Scanner", subtitle: "Scan barcodes to identify items" },
  settings: { title: "Device Setup", subtitle: "Configure your RFID reader" },
  help: { title: "Help", subtitle: "Quick answers to get you started" },
  assets: { title: "Assets", subtitle: "Manage your organization's assets" },
  locations: { title: "Locations", subtitle: "Manage your organization's locations" },
  reports: { title: "Reports", subtitle: "View asset locations and movement history" },
  'reports-history': { title: "Report History", subtitle: "Previously generated reports" },
  'api-keys': { title: "API Keys", subtitle: "Manage programmatic access tokens" },
  'org-members': { title: "Members", subtitle: "Manage organization members" },
  'org-settings': { title: "Organization Settings", subtitle: "Configure your organization" },
  'create-org': { title: "Create Organization", subtitle: "Set up a new organization" },
  'accept-invite': { title: "Accept Invite", subtitle: "Join an organization" }
};
```

Then find the fallback line (line ~158):

```tsx
const currentPage = pageTitles[activeTab as keyof typeof pageTitles] || pageTitles.inventory;
```

Replace with:

```tsx
const currentPage = pageTitles[activeTab as keyof typeof pageTitles] || { title: "", subtitle: "" };
```

- [ ] **Step 4: Run test to verify it passes**

Run: `just frontend test Header`

Expected: all cases PASS.

- [ ] **Step 5: Run frontend typecheck + existing tests**

Run: `just frontend typecheck && just frontend test`

Expected: PASS. If any existing test relied on "Inventory" leaking as a fallback title somewhere, update it.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/Header.tsx frontend/src/components/Header.test.tsx
git commit -m "fix(tra-424): complete Header pageTitles map; blank fallback for unknown routes

api-keys was one of several routes missing from the map — they all fell
back to the inventory title. Added entries for api-keys, reports-history,
org-members, org-settings, create-org, accept-invite. Changed the fallback
from pageTitles.inventory to an empty object so future missing routes go
blank instead of lying.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task F3: Full validation + visual check

- [ ] **Step 1: Run full frontend validation**

Run: `just frontend validate`

Expected: typecheck + lint + unit tests all pass.

- [ ] **Step 2: Manual visual check**

Start dev server: `just frontend dev`. In a browser:

1. Log in as an org admin.
2. Navigate to the API Keys screen. Verify the top-of-page header reads **"API Keys"** (not "Inventory").
3. If there are existing keys: verify the 8-char JTI chip renders next to each key name in muted monospace. Hover the chip to see the full JTI in the tooltip.
4. Create a new key, dismiss the show-once dialog, and confirm the new key now shows its JTI chip in the list.
5. Navigate to Settings → Members and verify the header reads **"Members"**. Repeat for Org Settings → **"Organization Settings"**.

If anything looks wrong, fix and re-commit before opening the PR.

- [ ] **Step 3: Push branch**

```bash
git push -u origin feature/tra-424-ui-polish
```

- [ ] **Step 4: Open PR 2**

```bash
gh pr create --title "fix(tra-424): Header page titles + API key JTI chip" --body "$(cat <<'EOF'
## Summary
- Fill in Header.pageTitles for all main-shell routes missing from the map (api-keys, reports-history, org-members, org-settings, create-org, accept-invite); change fallback from pageTitles.inventory to a blank object so missing future routes stop lying with "Inventory".
- Display the first 8 chars of each key's JTI as a muted monospace chip next to the name in the API Keys list, with full JTI as a tooltip. Resolves "can't distinguish keys with similar names" from the eval.

Depends on the backend \`jti\` field landing (PR #<PR1_NUMBER>). If merging this first, the \`jti\` field will be \`undefined\` at runtime until the backend ships — chip will still render from the slice on \`undefined.slice(0,8)\` which throws. Prefer to merge after PR #<PR1_NUMBER>.

Spec: docs/superpowers/specs/2026-04-21-tra-424-platform-polish-design.md
Plan: docs/superpowers/plans/2026-04-21-tra-424-platform-polish.md

## Test plan
- [x] `just frontend test` — unit tests
- [x] `just frontend typecheck` — type check
- [x] `just frontend lint` — lint
- [ ] Preview deploy: log in as admin, navigate to API Keys — header reads "API Keys", JTI chip visible next to each key name with full JTI on hover
- [ ] Preview deploy: Members, Org Settings headers correct

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Replace `<PR1_NUMBER>` with the actual PR 1 number once it's opened.

---

## Self-review notes

- **Spec coverage:** every finding in the spec (2, 3, 4, 5 plus the `jti` backend half) maps to a task. Finding #1 is explicitly dropped per the design; confirmed no task exists for it.
- **Placeholder scan:** no TBDs. The one `<PR1_NUMBER>` substitution is explicit and called out.
- **Type consistency:** `APIKeyListItem.JTI` (Go) and `APIKey.jti` (TS) match via the `json:"jti"` tag; `jti.slice(0, 8)` is used consistently in both tests and implementation; `pageTitles` additions are keyed by the exact same strings used in `App.tsx`'s `tabComponents` map (verified: `api-keys`, `reports-history`, `org-members`, `org-settings`, `create-org`, `accept-invite`).
- **TDD discipline:** each behavioral change starts with a failing test, then minimal implementation, then green.
