# TRA-538 + TRA-541 Envelope Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring all non-2xx HTTP responses into compliance with the documented error envelope contract (fixed `title` per `error.type`, variable explanation in `detail`); add envelope coverage for 405 and 415; resolve TRA-541's multipart sub-finding.

**Architecture:** Three new helpers in `backend/internal/util/httputil/` (`Respond404`, `Respond405`, `Respond415`) parallel the existing `Respond401`. Two new error-type constants land in `backend/internal/models/errors/errors.go`. Middleware (`EitherAuth`, `ContentType`) and the chi router wire through these helpers. ~37 handler call sites that emit 404 are mechanically swept to use `Respond404`. Targeted regression tests live alongside each touched file plus the four BB12 §1.2 reproductions in `cmd/serve/contract_smoke_test.go`.

**Tech Stack:** Go 1.23, chi v5 router, stretchr/testify, slog. Spec at `docs/superpowers/specs/2026-04-28-tra-538-tra-541-envelope-consistency-design.md`.

---

## File Structure

**New files:**
- `backend/internal/util/httputil/method_error.go` — `Respond405`, `Respond415` helpers
- `backend/internal/util/httputil/method_error_test.go` — unit tests for the two helpers

**Modified files:**
- `backend/internal/models/errors/errors.go` — add `ErrMethodNotAllowed`, `ErrUnsupportedMedia`; extend swagger enum annotation in `ErrorResponse`
- `backend/internal/util/httputil/auth_error.go` — add `Respond404`
- `backend/internal/util/httputil/auth_error_test.go` — add `Respond404` test
- `backend/internal/middleware/either_auth.go` — migrate 4 `WriteJSONError` 401 sites to `Respond401`
- `backend/internal/middleware/middleware.go` — `ContentType` calls `Respond415`
- `backend/internal/middleware/middleware_test.go` — `ContentType` test asserts new envelope
- `backend/internal/cmd/serve/router.go` — chi `MethodNotAllowed` wiring; catchall uses `Respond404`
- `backend/internal/cmd/serve/contract_smoke_test.go` — add BB12 §1.2 four 401 reproductions, 404 catchall, 404 handler, 405, 415 envelope tests
- ~37 handler files (assets, locations, orgs, users, lookup, reports, auth, bulkimport, api_keys, members, invitations) — replace `WriteJSONError(..., http.StatusNotFound, ...)` with `Respond404`

---

### Task 1: Add new error type constants

**Files:**
- Modify: `backend/internal/models/errors/errors.go:8-17, 53`
- Test: `backend/internal/models/errors/errors_test.go`

- [ ] **Step 1: Add a test that asserts the new constants exist with expected string values**

In `backend/internal/models/errors/errors_test.go`, append:

```go
func TestNewErrorTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  ErrorType
		want string
	}{
		{"method_not_allowed", ErrMethodNotAllowed, "method_not_allowed"},
		{"unsupported_media_type", ErrUnsupportedMedia, "unsupported_media_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `just backend test ./internal/models/errors/...`
Expected: FAIL — `undefined: ErrMethodNotAllowed`, `undefined: ErrUnsupportedMedia`.

- [ ] **Step 3: Add the constants**

In `backend/internal/models/errors/errors.go`, edit the const block:

```go
const (
	ErrValidation       ErrorType = "validation_error"
	ErrNotFound         ErrorType = "not_found"
	ErrConflict         ErrorType = "conflict"
	ErrInternal         ErrorType = "internal_error"
	ErrBadRequest       ErrorType = "bad_request"
	ErrUnauthorized     ErrorType = "unauthorized"
	ErrForbidden        ErrorType = "forbidden"
	ErrRateLimited      ErrorType = "rate_limited"
	ErrMethodNotAllowed ErrorType = "method_not_allowed"
	ErrUnsupportedMedia ErrorType = "unsupported_media_type"
)
```

- [ ] **Step 4: Extend the swagger enum annotation in `ErrorResponse`**

Same file, change line 53 from:

```go
		Type      string       `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error" extensions:"x-extensible-enum=true"`
```

to:

```go
		Type      string       `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error,method_not_allowed,unsupported_media_type" extensions:"x-extensible-enum=true"`
```

- [ ] **Step 5: Run the test again, confirm it passes**

Run: `just backend test ./internal/models/errors/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/errors/errors.go backend/internal/models/errors/errors_test.go
git commit -m "feat(api-errors): add method_not_allowed + unsupported_media_type types

Adds the two ErrorType constants needed for TRA-541 envelope coverage on
405 and 415 responses. Extends the swagger enum annotation so generated
clients see the new values; the existing x-extensible-enum extension makes
this additive for downstream consumers."
```

---

### Task 2: Implement `Respond404` helper

**Files:**
- Modify: `backend/internal/util/httputil/auth_error.go`
- Test: `backend/internal/util/httputil/auth_error_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/util/httputil/auth_error_test.go`:

```go
func TestRespond404_FixedTitleAndCallerDetail(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/assets/bogus", nil)
	httputil.Respond404(w, r, "Asset not found", "req-2")

	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Not found" {
		t.Errorf("title = %q, want Not found", resp.Error.Title)
	}
	if resp.Error.Type != string(apierrors.ErrNotFound) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrNotFound)
	}
	if resp.Error.Detail != "Asset not found" {
		t.Errorf("detail = %q, want caller-supplied string", resp.Error.Detail)
	}
	if resp.Error.RequestID != "req-2" {
		t.Errorf("request_id = %q, want req-2", resp.Error.RequestID)
	}
}
```

- [ ] **Step 2: Run, confirm it fails**

Run: `just backend test ./internal/util/httputil/...`
Expected: FAIL — `undefined: httputil.Respond404`.

- [ ] **Step 3: Implement the helper**

Append to `backend/internal/util/httputil/auth_error.go`:

```go
// Respond404 writes a normalized not-found response. All 404 call sites in
// public and internal handlers should route through this helper so the
// envelope and title are consistent and the variable explanation lives in
// detail.
//
// detail is caller-supplied, e.g. apierrors.AssetNotFound.
func Respond404(w http.ResponseWriter, r *http.Request, detail, requestID string) {
	WriteJSONError(w, r, http.StatusNotFound, apierrors.ErrNotFound,
		"Not found", detail, requestID)
}
```

- [ ] **Step 4: Run, confirm it passes**

Run: `just backend test ./internal/util/httputil/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/httputil/auth_error.go backend/internal/util/httputil/auth_error_test.go
git commit -m "feat(httputil): add Respond404 helper

Parallels Respond401: hardcodes title=\"Not found\" and type=not_found,
caller supplies the variable explanation in detail. Used by the
TRA-538 sweep that brings ~37 404 emission sites under the
title-fixed-per-type contract."
```

---

### Task 3: Implement `Respond405` helper

**Files:**
- Create: `backend/internal/util/httputil/method_error.go`
- Test: `backend/internal/util/httputil/method_error_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/util/httputil/method_error_test.go`:

```go
package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestRespond405_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/v1/assets", nil)
	httputil.Respond405(w, r, "req-3")

	if w.Code != 405 {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Method not allowed" {
		t.Errorf("title = %q, want Method not allowed", resp.Error.Title)
	}
	if resp.Error.Type != string(apierrors.ErrMethodNotAllowed) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrMethodNotAllowed)
	}
	if resp.Error.Status != 405 {
		t.Errorf("status field = %d, want 405", resp.Error.Status)
	}
	if resp.Error.Detail != "" {
		t.Errorf("detail = %q, want empty", resp.Error.Detail)
	}
}
```

- [ ] **Step 2: Run, confirm it fails**

Run: `just backend test ./internal/util/httputil/...`
Expected: FAIL — `undefined: httputil.Respond405`.

- [ ] **Step 3: Implement the helper**

Create `backend/internal/util/httputil/method_error.go`:

```go
package httputil

import (
	"net/http"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// Respond405 writes a normalized method-not-allowed response. Registered as
// chi's MethodNotAllowed handler on the root mux so unknown method/path
// combinations return the standard envelope instead of an empty body.
//
// detail is intentionally empty — the path and method are already in the
// access log; no useful per-call variability remains.
func Respond405(w http.ResponseWriter, r *http.Request, requestID string) {
	WriteJSONError(w, r, http.StatusMethodNotAllowed, apierrors.ErrMethodNotAllowed,
		"Method not allowed", "", requestID)
}

// Respond415 writes a normalized unsupported-media-type response. Used by the
// ContentType middleware when a write request arrives with a content-type the
// public API spec does not declare.
//
// detail is fixed: the public OpenAPI spec only declares application/json on
// every endpoint, so the message names exactly that. The middleware still
// accepts multipart/form-data internally for the session-only bulk CSV
// endpoint, but that detail is not surfaced in the public-facing error.
func Respond415(w http.ResponseWriter, r *http.Request, requestID string) {
	WriteJSONError(w, r, http.StatusUnsupportedMediaType, apierrors.ErrUnsupportedMedia,
		"Unsupported media type", "Content-Type must be application/json", requestID)
}
```

- [ ] **Step 4: Run, confirm it passes**

Run: `just backend test ./internal/util/httputil/...`
Expected: PASS for `TestRespond405_EnvelopeShape`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/httputil/method_error.go backend/internal/util/httputil/method_error_test.go
git commit -m "feat(httputil): add Respond405 helper

Parallels Respond401/Respond404: fixed title \"Method not allowed\",
type=method_not_allowed, empty detail. To be wired as chi's
MethodNotAllowed handler in a follow-up task. Also drops in Respond415
in the same file (tested in the next task) so both 4xx-method/media
helpers live together."
```

---

### Task 4: Test `Respond415` and confirm the multipart wording is gone

**Files:**
- Modify: `backend/internal/util/httputil/method_error_test.go`

- [ ] **Step 1: Append the failing test**

In `backend/internal/util/httputil/method_error_test.go`:

```go
func TestRespond415_DropsMultipartWording(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)
	httputil.Respond415(w, r, "req-4")

	if w.Code != 415 {
		t.Fatalf("status = %d, want 415", w.Code)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Title != "Unsupported media type" {
		t.Errorf("title = %q, want Unsupported media type", resp.Error.Title)
	}
	if resp.Error.Type != string(apierrors.ErrUnsupportedMedia) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrUnsupportedMedia)
	}
	// POLS: AI integrators work from openapi.public.yaml, which never names
	// multipart/form-data. The public 415 message must not either.
	if strings.Contains(resp.Error.Detail, "multipart") {
		t.Errorf("detail = %q, must not mention multipart", resp.Error.Detail)
	}
	if resp.Error.Detail != "Content-Type must be application/json" {
		t.Errorf("detail = %q, want Content-Type must be application/json", resp.Error.Detail)
	}
}
```

Add `"strings"` to the import block of `method_error_test.go`.

- [ ] **Step 2: Run, confirm it passes (helper already implemented in Task 3)**

Run: `just backend test ./internal/util/httputil/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/util/httputil/method_error_test.go
git commit -m "test(httputil): assert Respond415 detail does not mention multipart

POLS for AI integrators working from openapi.public.yaml: the public
spec declares only application/json on every endpoint, so the 415 detail
must not name multipart. Multipart support remains in the ContentType
middleware for the session-only bulk CSV upload route."
```

---

### Task 5: Migrate `EitherAuth` 401 sites to `Respond401`

**Files:**
- Modify: `backend/internal/middleware/either_auth.go:31-58`

This is the source of the BB12 §1.2 reproduction (X-API-Key header → variable string in title). Existing `either_auth_test.go` tests assert only on status code and on body-substring containment, so they continue to pass after the title↔detail swap.

- [ ] **Step 1: Replace the four `WriteJSONError` calls with `Respond401`**

In `backend/internal/middleware/either_auth.go`, edit the function body:

```go
func EitherAuth(store *storage.Storage) func(http.Handler) http.Handler {
	apiChain := APIKeyAuth(store)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := GetRequestID(r.Context())

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httputil.Respond401(w, r, missingAuthDetail(r, "Missing authorization header"), reqID)
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httputil.Respond401(w, r, "Invalid authorization header format", reqID)
				return
			}

			kind, err := jwt.ClassifyToken(parts[1])
			if err != nil {
				httputil.Respond401(w, r, "Invalid or expired token", reqID)
				return
			}

			switch kind {
			case jwt.TokenKindAPIKey:
				apiChain(next).ServeHTTP(w, r)
			case jwt.TokenKindSession:
				Auth(next).ServeHTTP(w, r)
			default:
				httputil.Respond401(w, r, "Invalid or expired token", reqID)
			}
		})
	}
}
```

The `errors` import becomes unused. Remove it from the import block.

- [ ] **Step 2: Run the existing either_auth tests**

Run: `just backend test -tags=integration ./internal/middleware/...`
Expected: PASS (status-code and body-substring assertions still match).

- [ ] **Step 3: Run the package without integration tag (no-op for either_auth, sanity check for compile)**

Run: `just backend test ./internal/middleware/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/middleware/either_auth.go
git commit -m "fix(middleware): TRA-538 route EitherAuth 401s through Respond401

EitherAuth was the actual source of the BB12 §1.2 contract violation:
four WriteJSONError calls passed the variable explanation as the title
argument, leaving detail empty. Migrating to Respond401 hardcodes
title=\"Authentication required\" and moves the variable string into
detail as documented in docs/api/errors.

Existing either_auth_test.go assertions are body-substring checks that
continue to pass after the title<>detail swap."
```

---

### Task 6: Switch `ContentType` middleware to `Respond415`

**Files:**
- Modify: `backend/internal/middleware/middleware.go:111-117`
- Modify: `backend/internal/middleware/middleware_test.go`

- [ ] **Step 1: Update the `ContentType` middleware to call the helper**

In `backend/internal/middleware/middleware.go`, replace the `WriteJSONError` call inside `ContentType`:

```go
			if !isAllowed {
				httputil.Respond415(w, r, GetRequestID(r.Context()))
				return
			}
```

The `errors` import in this file may still be needed by other functions in `middleware.go` — keep it. Run `go build ./...` to confirm.

- [ ] **Step 2: Update the existing `TestContentType` cases that expected 415**

Find the cases in `middleware_test.go` where `expectedStatus: http.StatusUnsupportedMediaType` is set. Add envelope assertions inside the test loop. Modify the test loop to also unmarshal and verify type / fixed title / no-multipart-in-detail when the response is 415:

```go
if tc.expectedStatus == http.StatusUnsupportedMediaType {
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Type != string(apierrors.ErrUnsupportedMedia) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrUnsupportedMedia)
	}
	if resp.Error.Title != "Unsupported media type" {
		t.Errorf("title = %q, want Unsupported media type", resp.Error.Title)
	}
	if strings.Contains(resp.Error.Detail, "multipart") {
		t.Errorf("detail = %q, must not mention multipart", resp.Error.Detail)
	}
}
```

If `apierrors`, `json`, `strings` are not yet imported in `middleware_test.go`, add them. (`json` is `encoding/json`; `apierrors` is `github.com/trakrf/platform/backend/internal/models/errors`.)

- [ ] **Step 3: Run the middleware tests**

Run: `just backend test ./internal/middleware/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/middleware/middleware.go backend/internal/middleware/middleware_test.go
git commit -m "fix(middleware): TRA-541 ContentType emits Respond415 envelope

Replaces the inline WriteJSONError that emitted type=bad_request on a
415 status (catalog-status mismatch) with the new Respond415 helper.
Also drops the multipart wording from the user-visible detail since the
public OpenAPI spec contains zero multipart endpoints. The middleware's
underlying multipart allowance for the session-only bulk CSV route is
unchanged."
```

---

### Task 7: Wire chi `MethodNotAllowed` and add a Respond405 router test

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`
- Modify: `backend/internal/cmd/serve/contract_smoke_test.go`

- [ ] **Step 1: Write a failing router-level integration test**

Append to `backend/internal/cmd/serve/contract_smoke_test.go`:

```go
// TestContract_MethodNotAllowed_EmitsEnvelope asserts that an unknown method
// against an existing route returns the documented envelope (TRA-541 §1.10).
// Before the fix, chi's default 405 handler returned an empty body.
func TestContract_MethodNotAllowed_EmitsEnvelope(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond405(w, req, middleware.GetRequestID(req.Context()))
	})
	r.Get("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/assets", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.NotEmpty(t, rec.Body.String(), "405 must carry an envelope, not an empty body")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "method_not_allowed", resp.Error.Type)
	require.Equal(t, 405, resp.Error.Status)
	require.Equal(t, "Method not allowed", resp.Error.Title)
}
```

Add `"github.com/trakrf/platform/backend/internal/util/httputil"` to the imports if not already present.

- [ ] **Step 2: Run, confirm it passes (the helper already exists, the test is just exercising it)**

Run: `just backend test ./internal/cmd/serve/...`
Expected: PASS for `TestContract_MethodNotAllowed_EmitsEnvelope`.

- [ ] **Step 3: Wire `MethodNotAllowed` on the production router**

In `backend/internal/cmd/serve/router.go`, immediately after `r := chi.NewRouter()` (line 51):

```go
	r := chi.NewRouter()

	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond405(w, req, middleware.GetRequestID(req.Context()))
	})
```

- [ ] **Step 4: Run the full serve package tests**

Run: `just backend test ./internal/cmd/serve/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/cmd/serve/contract_smoke_test.go
git commit -m "fix(api): TRA-541 wire chi MethodNotAllowed to Respond405

Before this change, chi's default 405 handler returned an empty body,
breaking the envelope contract for any client that always tries to
deserialize on non-2xx. Registering Respond405 as the root mux's
MethodNotAllowed handler closes that gap. New contract test exercises
the wiring end-to-end through a chi router."
```

---

### Task 8: Fix the router `/api/*` 404 catchall

**Files:**
- Modify: `backend/internal/cmd/serve/router.go:225-228`

- [ ] **Step 1: Update the catchall to use `Respond404`**

In `backend/internal/cmd/serve/router.go`, replace the catchall body:

```go
	r.With(middleware.DefaultRateLimitHeaders(rl)).Get("/api/*", func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond404(w, req, "Unknown API route: "+req.URL.Path, middleware.GetRequestID(req.Context()))
	})
```

The `errors` import in router.go may be referenced elsewhere — keep it if needed; remove only if `go build` flags it as unused.

- [ ] **Step 2: Run the package tests**

Run: `just backend test ./internal/cmd/serve/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/cmd/serve/router.go
git commit -m "fix(api): TRA-538 catchall 404 emits fixed title

The /api/* catchall previously emitted title=\"Unknown API route: <path>\"
(variable in title). Routes through Respond404 now: title=\"Not found\",
detail carries the unknown-path string."
```

---

### Task 9: Sweep 404 sites in `handlers/assets`

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` — 9 sites (lines 202, 290, 332, 378, 398, 563, 610, 687, 828)
- Modify: `backend/internal/handlers/assets/bulkimport.go:56`

- [ ] **Step 1: Replace each `WriteJSONError(..., http.StatusNotFound, modelerrors.ErrNotFound, <variable string>, "", reqID)` with `Respond404(w, req, <variable string>, reqID)`**

Each call site has the same shape:

```go
httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
    apierrors.AssetNotFound, "", requestID)
```

Replace with:

```go
httputil.Respond404(w, req, apierrors.AssetNotFound, requestID)
```

Apply this transformation to all 9 sites in `assets.go` and the 1 site in `bulkimport.go`. Watch for variations: some sites pass formatted strings (`fmt.Sprintf("Asset %s not found", id)`) — those move into the `detail` arg unchanged.

The `modelerrors` package may become unused in some files. Remove the import if `go build` flags it.

- [ ] **Step 2: Run package tests**

Run: `just backend test ./internal/handlers/assets/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/assets/
git commit -m "refactor(handlers): TRA-538 assets 404 sites use Respond404

Mechanical sweep: 10 call sites across assets.go and bulkimport.go that
emitted WriteJSONError(..., StatusNotFound, ...) now route through
Respond404. Variable explanation string moves from the title arg to the
detail arg; title becomes the fixed \"Not found\" string from the helper."
```

---

### Task 10: Sweep 404 sites in `handlers/locations`

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` — 12 sites (lines 173, 260, 303, 323, 506, 544, 592, 662, 732, 814, 891, 1032)

- [ ] **Step 1: Apply the same transformation as Task 9 to all 12 sites**

Same shape: `WriteJSONError(..., http.StatusNotFound, modelerrors.ErrNotFound, <var>, "", reqID)` → `Respond404(w, req, <var>, reqID)`.

- [ ] **Step 2: Run package tests**

Run: `just backend test ./internal/handlers/locations/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/locations/
git commit -m "refactor(handlers): TRA-538 locations 404 sites use Respond404

12 call sites in handlers/locations migrated to Respond404. Same
mechanical transformation as the assets sweep."
```

---

### Task 11: Sweep 404 sites in `handlers/orgs`

**Files:**
- Modify: `backend/internal/handlers/orgs/orgs.go` — 3 sites (lines 146, 199, 247)
- Modify: `backend/internal/handlers/orgs/members.go` — 2 sites (lines 104, 167)
- Modify: `backend/internal/handlers/orgs/invitations.go` — 2 sites (lines 145, 198)
- Modify: `backend/internal/handlers/orgs/api_keys.go:259`

- [ ] **Step 1: Apply the transformation to all 8 sites**

Same shape as Task 9.

- [ ] **Step 2: Run package tests**

Run: `just backend test ./internal/handlers/orgs/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/orgs/
git commit -m "refactor(handlers): TRA-538 orgs 404 sites use Respond404

8 call sites across orgs.go, members.go, invitations.go, and api_keys.go
migrated to Respond404."
```

---

### Task 12: Sweep 404 sites in remaining handlers

**Files:**
- Modify: `backend/internal/handlers/users/users.go` — 3 sites (lines 109, 209, 240)
- Modify: `backend/internal/handlers/lookup/lookup.go:72`
- Modify: `backend/internal/handlers/reports/asset_history.go` — 2 sites (lines 83, 171)
- Modify: `backend/internal/handlers/auth/auth.go:325`

- [ ] **Step 1: Apply the transformation to all 7 sites**

Same shape as Task 9.

- [ ] **Step 2: Run package tests**

Run: `just backend test ./internal/handlers/users/... ./internal/handlers/lookup/... ./internal/handlers/reports/... ./internal/handlers/auth/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/users/ backend/internal/handlers/lookup/ backend/internal/handlers/reports/ backend/internal/handlers/auth/
git commit -m "refactor(handlers): TRA-538 remaining 404 sites use Respond404

7 call sites across users, lookup, reports, and auth handlers migrated
to Respond404. With this sweep, every public 404 emission site under
backend/internal/handlers/ routes through the fixed-title helper.

Out of scope: testhandler/invitations.go (dev-only http.Error 404,
gated by APP_ENV != production) and health/health.go (not under /api/)."
```

---

### Task 13: Add BB12 §1.2 four 401 reproductions

**Files:**
- Modify: `backend/internal/cmd/serve/contract_smoke_test.go`

These integration tests exercise `EitherAuth` end-to-end through a chi router and assert that every 401 has `title:"Authentication required"` (fixed) and the variable string lives in `detail`. The four reproductions named in BB12 §1.2 are: missing-header, wrong-scheme, garbage-token, missing-header-with-X-API-Key.

- [ ] **Step 1: Add the test**

Append to `backend/internal/cmd/serve/contract_smoke_test.go`:

```go
// TestContract_BB12_401Reproductions covers the four 401 variants named in
// BB12 §1.2 (TRA-538). Each must emit title="Authentication required" with
// the variable explanation in detail. Title must NOT contain "Bearer" or
// the offending value — the contract violation the audit found.
func TestContract_BB12_401Reproductions(t *testing.T) {
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := middleware.GetRequestID(r.Context())
			h := r.Header.Get("Authorization")
			if h == "" {
				detail := "Missing authorization header"
				if r.Header.Get("X-API-Key") != "" {
					detail = "Use Authorization: Bearer <token>"
				}
				httputilRespond401(w, r, detail, reqID)
				return
			}
			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httputilRespond401(w, r, "Invalid authorization header format", reqID)
				return
			}
			httputilRespond401(w, r, "Invalid or expired token", reqID)
		})
	}

	mkRouter := func() *chi.Mux {
		mux := chi.NewRouter()
		mux.Use(middleware.RequestID)
		mux.Use(auth)
		mux.Get("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
			t.Fatal("auth should reject before handler")
		})
		return mux
	}

	cases := []struct {
		name           string
		setup          func(*http.Request)
		wantDetailHas  string
		titleMustNotBe []string
	}{
		{
			name:           "missing header",
			setup:          func(r *http.Request) {},
			wantDetailHas:  "Missing authorization header",
			titleMustNotBe: []string{"Bearer", "Missing authorization header"},
		},
		{
			name:           "wrong scheme",
			setup:          func(r *http.Request) { r.Header.Set("Authorization", "Basic abc") },
			wantDetailHas:  "Invalid authorization header format",
			titleMustNotBe: []string{"Basic", "Invalid authorization header format"},
		},
		{
			name:           "garbage token",
			setup:          func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-jwt") },
			wantDetailHas:  "Invalid or expired token",
			titleMustNotBe: []string{"Bearer not-a-jwt", "Invalid or expired token"},
		},
		{
			name:           "missing header with X-API-Key",
			setup:          func(r *http.Request) { r.Header.Set("X-API-Key", "some-token") },
			wantDetailHas:  "Use Authorization: Bearer <token>",
			titleMustNotBe: []string{"Bearer <token>", "Use Authorization: Bearer <token>"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
			tc.setup(req)
			rec := httptest.NewRecorder()
			mkRouter().ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)

			var resp apierrors.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			require.Equal(t, "Authentication required", resp.Error.Title,
				"title must be the fixed string per TRA-538 contract")
			require.Equal(t, "unauthorized", resp.Error.Type)
			require.Contains(t, resp.Error.Detail, tc.wantDetailHas,
				"variable explanation must live in detail")
			for _, forbidden := range tc.titleMustNotBe {
				require.NotContains(t, resp.Error.Title, forbidden,
					"title must not contain the variable substring %q", forbidden)
			}
		})
	}
}
```

The test uses a stand-in auth middleware (not the real `EitherAuth`, which is integration-tagged and pulls a DB). The stand-in is structurally identical to `EitherAuth`'s 401 paths and exercises the same `Respond401` helper end-to-end. This intentionally pairs as a contract test, not an `EitherAuth` test — `either_auth_test.go` (integration) covers the real middleware.

Add an alias for the helper to keep the test readable. At the top of the file:

```go
var httputilRespond401 = httputil.Respond401
```

Add `"github.com/trakrf/platform/backend/internal/util/httputil"` and `"strings"` imports if not present.

- [ ] **Step 2: Run, confirm it passes**

Run: `just backend test ./internal/cmd/serve/...`
Expected: PASS — all four subtests.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/cmd/serve/contract_smoke_test.go
git commit -m "test(api): TRA-538 BB12 §1.2 four 401 reproductions

End-to-end contract tests for the four 401 variants named in BB12 §1.2:
missing header, wrong scheme, garbage token, X-API-Key without
Authorization. Each asserts:
- title is the fixed \"Authentication required\" string
- type is unauthorized
- variable explanation lives in detail
- title does NOT contain the variable substring (the audited regression)

Uses a stand-in auth middleware that mirrors EitherAuth's 401 paths so
the test can run without a database; the real EitherAuth is exercised
in either_auth_test.go (integration tag)."
```

---

### Task 14: Add 404 + 415 contract tests at the router level

**Files:**
- Modify: `backend/internal/cmd/serve/contract_smoke_test.go`

- [ ] **Step 1: Append the 404 and 415 contract tests**

```go
// TestContract_NotFound_FixedTitleAndDetail covers TRA-538: 404 responses
// must emit title="Not found" with the resource-specific message in detail.
func TestContract_NotFound_FixedTitleAndDetail(t *testing.T) {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Get("/api/v1/assets/{id}", func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond404(w, req, "Asset not found",
			middleware.GetRequestID(req.Context()))
	})
	mux.Get("/api/*", func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond404(w, req, "Unknown API route: "+req.URL.Path,
			middleware.GetRequestID(req.Context()))
	})

	cases := []struct {
		name          string
		path          string
		wantDetailHas string
		titleMustNot  string
	}{
		{"handler 404", "/api/v1/assets/bogus", "Asset not found", "Asset not found"},
		{"catchall 404", "/api/v1/nonexistent", "Unknown API route", "Unknown API route"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)

			var resp apierrors.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, "Not found", resp.Error.Title,
				"title must be the fixed string per TRA-538 contract")
			require.Equal(t, "not_found", resp.Error.Type)
			require.Contains(t, resp.Error.Detail, tc.wantDetailHas)
			require.NotContains(t, resp.Error.Title, tc.titleMustNot,
				"title must not contain the variable string")
		})
	}
}

// TestContract_UnsupportedMediaType_EnvelopeAndType covers TRA-541 §1.11:
// 415 must emit type=unsupported_media_type and a detail that does not
// name multipart (since the public OpenAPI spec contains no multipart
// endpoints).
func TestContract_UnsupportedMediaType_EnvelopeAndType(t *testing.T) {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.ContentType)
	mux.Post("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "unsupported_media_type", resp.Error.Type)
	require.Equal(t, "Unsupported media type", resp.Error.Title)
	require.Equal(t, 415, resp.Error.Status)
	require.NotContains(t, resp.Error.Detail, "multipart",
		"public 415 detail must not name multipart per TRA-541 POLS resolution")
}
```

- [ ] **Step 2: Run, confirm both pass**

Run: `just backend test ./internal/cmd/serve/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/cmd/serve/contract_smoke_test.go
git commit -m "test(api): TRA-538/541 router-level 404 and 415 contract tests

Adds router-level contract tests for the remaining envelope cases:
- 404 with fixed title \"Not found\", variable explanation in detail
  (covers both handler-level Respond404 and the /api/* catchall)
- 415 with type=unsupported_media_type, no multipart wording in detail

Combined with the existing 405 test and the BB12 §1.2 401 reproductions,
this covers every status code touched by TRA-538 + TRA-541."
```

---

### Task 15: Run the full validation suite

- [ ] **Step 1: Run linters and tests across both workspaces**

Run: `just validate`
Expected: PASS (`just lint` and `just test` both green).

- [ ] **Step 2: If any test outside this scope is broken because it asserted on the old envelope shape, fix it in lockstep**

Likely candidates: any handler-level test that pinned `body.error.title` to the resource-specific string (e.g., `"Asset not found"`). Update those assertions to read from `resp.Error.Detail` instead.

If you find one, fix it, run the package test, then commit:

```bash
git add <files>
git commit -m "test: align <package>_test envelope assertions with TRA-538 contract

After Respond404 sweep, title is always \"Not found\" and the
resource-specific message lives in detail. Updates assertions that
referenced the old shape."
```

- [ ] **Step 3: Final clean validate**

Run: `just validate`
Expected: PASS, no diff in `git status`.

---

### Task 16: Push and open the PR

- [ ] **Step 1: Push the branch**

Run: `git push -u origin fix/tra-538-541-envelope-contract`

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "fix(api): TRA-538 + TRA-541 enforce error envelope contract" --body "$(cat <<'EOF'
## Summary

- TRA-538: every 401 and 404 response now carries fixed `title` per `error.type` with the variable explanation in `detail`. The actual source of the BB12 §1.2 violation was four `WriteJSONError` calls in `EitherAuth` that passed the variable string as the title argument; migrated to `Respond401`. ~37 handler 404 sites swept to a new `Respond404` helper.
- TRA-541 §1.10: 405 responses now carry the standard envelope (`type:method_not_allowed`) instead of an empty body. Wired via chi's `MethodNotAllowed` and a new `Respond405` helper.
- TRA-541 §1.11: 415 responses now emit `type:unsupported_media_type` (catalog-aligned) instead of `bad_request`. Public message no longer names multipart.
- TRA-541 multipart sub-finding: `/api/v1/assets/bulk` is the only multipart consumer, session-auth only, intentionally omitted from `openapi.public.yaml` — no undocumented public surface.

Spec: `docs/superpowers/specs/2026-04-28-tra-538-tra-541-envelope-consistency-design.md`

A follow-up trakrf-docs PR will reverse the "title may vary" wording in `docs/api/errors.md` and add catalog rows for the two new types.

## Test plan

- [ ] CI green (`just validate`)
- [ ] BB12 §1.2 spot check on preview: re-run the four 401 reproductions; confirm all four follow the contract
- [ ] BB12 §1.10 / §1.11 spot check on preview: re-run the 405 (`PATCH /api/v1/assets`) and 415 (`POST` with `Content-Type: text/plain`) reproductions

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Note the PR URL in your handoff**

Return the URL printed by `gh pr create` so it can be tracked.

---

## Self-review

**Spec coverage:**

- TRA-538 §1.2 401 contract: covered by Task 5 (`either_auth.go` migration) + Task 13 (BB12 reproductions test).
- TRA-538 §1.2 404 contract: covered by Task 2 (`Respond404` helper) + Tasks 8–12 (sweep) + Task 14 (router-level 404 test).
- TRA-538 acceptance — test that fails if title contains the variable string: explicit `NotContains` assertions in Tasks 13 and 14.
- TRA-541 §1.10 405 envelope: Task 3 (helper) + Task 7 (chi wiring + test).
- TRA-541 §1.11 415 envelope and type: Task 4 (test for no-multipart wording) + Task 6 (middleware migration).
- TRA-541 §1.11 multipart sub-finding: resolved during exploration; recorded in PR body in Task 16.
- Error catalog updates: Task 1 (constants + swagger enum). Docs catalog change is the trakrf-docs follow-up PR, not in scope here.

**Placeholder scan:** none. Every code step shows actual code; every command shows actual flags.

**Type/method consistency:** `Respond404`, `Respond405`, `Respond415` signatures appear in Tasks 2/3 and are referenced unchanged in 6/7/8/9–14. `ErrMethodNotAllowed` and `ErrUnsupportedMedia` defined in Task 1, referenced in Tasks 3/4/6/7/14 with the same names.
