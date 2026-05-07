# TRA-588 — Emit Allow header on 405 method_not_allowed

## Goal

405 responses include the `Allow` header listing the methods supported on the route, plus an `error.detail` of the form `"Allowed methods: GET, POST"`. Closes the BB16 W6 doc-vs-service mismatch and aligns with RFC 7231 §6.5.5.

## Non-goals

- Bulk-attaching `405` response declarations to every operation in the public OpenAPI spec. Out of scope per the ticket; spec defaults to "method not in this operation list" implying 405. We declare the reusable response component but do not wire it into operations.
- Restructuring the 405 envelope shape (`type`, `title`, `status`, `instance`, `request_id` unchanged).
- Touching the `/api/*` catchall 404 path.

## Approach

chi v5 computes allowed methods internally but exposes them only to its default 405 handler. The platform registers a custom `MethodNotAllowed` handler (envelope-aware), so chi's allowed-method list is unreachable through the public RouteContext API.

At 405 time, probe the mux: iterate `[GET, POST, PUT, PATCH, DELETE, OPTIONS]` and call `mux.Match(chi.NewRouteContext(), method, r.URL.Path)`. Collect matching methods. Add `HEAD` to the result whenever `GET` matches — `chimiddleware.GetHead` rewrites HEAD→GET, so HEAD is implicitly supported wherever GET is.

The probe runs only on the cold 405 path (six chi tree lookups per 405); 405s are rare and chi tree lookups are cheap. No precomputed table, no shadow routing logic.

## Components

### 1. `internal/cmd/serve/method_allowed.go` (new)

```go
package serve

import (
    "net/http"
    "sort"

    "github.com/go-chi/chi/v5"
)

// methodProbeOrder lists the canonical HTTP methods we probe at 405 time.
// HEAD is omitted: chimiddleware.GetHead rewrites HEAD→GET upstream, so HEAD
// is implicitly accepted wherever GET is. We synthesize HEAD into the
// allowed set when GET matches.
var methodProbeOrder = []string{
    http.MethodGet,
    http.MethodPost,
    http.MethodPut,
    http.MethodPatch,
    http.MethodDelete,
    http.MethodOptions,
}

// computeAllowedMethods returns the HTTP methods the mux would accept for
// path, sorted in canonical order (GET, HEAD, POST, PUT, PATCH, DELETE,
// OPTIONS subset). Returns an empty slice if no method matches — at 405 time
// at least one method is guaranteed to match, so an empty result indicates a
// programming error in the caller.
func computeAllowedMethods(mux *chi.Mux, path string) []string {
    seen := make(map[string]bool, len(methodProbeOrder)+1)
    for _, m := range methodProbeOrder {
        if mux.Match(chi.NewRouteContext(), m, path) {
            seen[m] = true
        }
    }
    if seen[http.MethodGet] {
        seen[http.MethodHead] = true
    }
    out := make([]string, 0, len(seen))
    for m := range seen {
        out = append(out, m)
    }
    sort.Slice(out, func(i, j int) bool {
        return canonicalRank(out[i]) < canonicalRank(out[j])
    })
    return out
}

func canonicalRank(m string) int { /* GET=0, HEAD=1, POST=2, PUT=3, PATCH=4, DELETE=5, OPTIONS=6 */ }
```

### 2. `internal/cmd/serve/router.go` (modify)

Replace the existing `MethodNotAllowed` closure:

```go
r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
    allowed := computeAllowedMethods(r, req.URL.Path)
    httputil.Respond405(w, req, allowed, middleware.GetRequestID(req.Context()))
})
```

The closure captures `r` (the `*chi.Mux`) by reference. The router is fully constructed by the time the first request arrives, so the captured mux is safe to introspect.

### 3. `internal/util/httputil/method_error.go` (modify)

Extend `Respond405` signature:

```go
func Respond405(w http.ResponseWriter, r *http.Request, allowed []string, requestID string)
```

Behavior:
- If `allowed` is non-empty: set `Allow: <comma-joined>` header and `error.detail = "Allowed methods: <comma-joined>"`.
- If `allowed` is empty (defensive): emit envelope without the Allow header and with empty detail. Should not occur in production.

The existing godoc comment about "detail intentionally empty" is replaced with rationale for populating it (TRA-588: doc claim parity, useful for clients without header inspection).

### 4. `internal/tools/apispec/postprocess.go` (modify)

New post-processing step `injectMethodNotAllowedResponse(doc)` called from `postprocessPublic`. Adds a single reusable response under `components.responses`:

```yaml
components:
  responses:
    MethodNotAllowed:
      description: Method not allowed
      headers:
        Allow:
          description: Comma-separated list of HTTP methods supported on this resource.
          schema:
            type: string
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/errors.ErrorResponse'
```

Does not modify any operation's `responses` map. Operations may $ref this in a future change.

## Test plan

### Unit tests

- **`method_allowed_test.go` (new)** — table-driven: synthetic mux with mixed routes, assert `computeAllowedMethods` returns the right canonical-order set. Cases:
  - Single GET route → `[GET, HEAD]` (HEAD inferred).
  - GET + POST → `[GET, HEAD, POST]`.
  - PUT + DELETE only (no GET) → `[PUT, DELETE]` (no HEAD).
  - Path with no matches → `[]`.

- **`method_error_test.go` (modify)** — `TestRespond405_EnvelopeShape`:
  - Replace `detail == ""` assertion with `detail == "Allowed methods: GET, POST"`.
  - Add `w.Header().Get("Allow") == "GET, POST"` assertion.
  - Existing title/type/status/request_id assertions unchanged.

- **`apispec/postprocess_test.go` (modify or add)** — assert `doc.Components.Responses["MethodNotAllowed"]` is present, has 405 description, has Allow header schema, and content references the error envelope.

### Contract tests

- **`contract_smoke_test.go` — `TestContract_MethodNotAllowed_EmitsEnvelope`** — extend with:
  - `require.Equal(t, "GET", rec.Header().Get("Allow"))` for the GET-only test route, after HEAD synthesis the value is `"GET, HEAD"`. Test uses a route with multiple methods to make assertion meaningful.
  - `require.Contains(t, resp.Error.Detail, "Allowed methods:")`.

### Integration verification

After implementation, exercise against the local dev server:

```bash
just backend test ./internal/cmd/serve/...
just backend test ./internal/util/httputil/...
just backend test ./internal/tools/apispec/...
```

Then `curl -i -X PATCH http://localhost:8080/api/v1/assets` against a running backend (or the preview deployment after PR merge) and verify both the `Allow:` header and `error.detail` content.

## Acceptance mapping

- [x] 405 responses include `Allow` header listing the methods actually supported on that path.
- [x] Verified across at least three different paths — covered by `computeAllowedMethods` table test (single-method, GET+POST, PUT+DELETE).
- [x] `error.detail` populated with allowed methods.
- [x] Spec: `405` response declarations declare the `Allow` header (single pattern reusable across endpoints) — added to `components.responses.MethodNotAllowed`.
- [x] Errors page doc claim now matches service behavior — covered by service emitting the header and detail. No docs change in this PR (docs handled separately).
- [ ] Removes corresponding finding from TRA-581 — note in PR description; ticket-comment update happens at merge.

## Risks

- **`mux.Match` mutation behavior.** chi's `Match` updates the passed RouteContext. Using a fresh `chi.NewRouteContext()` per probe per chi's documented requirement avoids state leak.
- **Subroute mounts.** chi propagates `MethodNotAllowed` to mounted subroutes. The platform's only sub-mount is via `r.Group(...)`, which shares the parent mux's `Match`. Probing the top-level mux walks into subroutes correctly.
- **`OPTIONS` preflight.** The platform doesn't currently register OPTIONS handlers; CORS middleware handles preflight. `mux.Match` will return false for OPTIONS, keeping it out of Allow. CORS preflight responses don't go through the 405 path.

## Out of scope

- Adding `405` response declarations to individual operations.
- OPTIONS handler implementation.
- Documenting the `Allow`-header behavior on the public docs site (handled in a separate trakrf-docs session).
