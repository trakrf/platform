# TRA-537 Handler-Level 401 Sweep — `missing_org_context` + Respond401 Migration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sweep the ~45 handler-level 401 emission sites identified in the TRA-537 audit-follow-up comment so every non-2xx envelope conforms to the contract enforced by TRA-538/541. Auth-state-passed-but-org-missing emissions become 422 + new `missing_org_context` type via a new helper; remaining `claims==nil` and API-key-resolution-failure sites migrate to `Respond401`.

**Architecture:** One new error type (`ErrMissingOrgContext` = `"missing_org_context"`), one new helper (`RespondMissingOrgContext` paralleling `Respond401`/`Respond404`/`Respond405`/`Respond415` from PR #243), then mechanical sweep across handler files. One compound check in `bulkimport.go` splits into two separate checks (one per condition).

**Tech Stack:** Go 1.23, chi v5 router, stretchr/testify. Design rationale lives in the TRA-537 audit-follow-up comment on Linear; PR #243 establishes the helper/sweep pattern this plan follows.

---

## Site classification

Per audit done at base SHA `2e50c0a` (PR #243 merge):

- **Pattern A** — `GetRequestOrgID(r)` fails (auth passed but org missing) → `RespondMissingOrgContext` (new, 422). 33 sites.
- **Pattern B** — `claims == nil` (session principal expected but missing) → `Respond401` with descriptive detail. 6 sites.
- **Pattern C** — `claims == nil || claims.CurrentOrgID == nil` (compound) → split into two checks: claims-nil → `Respond401`; current-org-nil → `RespondMissingOrgContext`. 2 sites → 4 emission sites after split.
- **Pattern D** — API-key principal nil → `Respond401`. 1 site (orgs/public.go:42).
- **Pattern E** — org deleted after key issuance → `Respond401("Organization no longer exists", ...)`. 1 site (orgs/public.go:55).
- **Pattern F** — API-key creator-resolution failure → `Respond401` with specific detail. 2 sites (orgs/api_keys.go:65, :76).

---

## File structure

**Modified files:**
- `backend/internal/models/errors/errors.go` — add `ErrMissingOrgContext`; extend swagger enum annotation
- `backend/internal/models/errors/errors_test.go` — assert new constant
- `backend/internal/util/httputil/method_error.go` — add `RespondMissingOrgContext` helper (sits next to Respond405/Respond415; both are non-auth envelope helpers)
- `backend/internal/util/httputil/method_error_test.go` — test the new helper
- `backend/internal/cmd/serve/contract_smoke_test.go` — add 422 contract test
- 14 handler files (assets, locations, orgs, lookup, reports, inventory, auth, bulkimport)

---

### Task 1: Add `ErrMissingOrgContext` constant + extend swagger enum

**Files:**
- Modify: `backend/internal/models/errors/errors.go`
- Modify: `backend/internal/models/errors/errors_test.go`

- [ ] **Step 1: Append a failing test asserting the constant**

In `backend/internal/models/errors/errors_test.go`, extend the existing `TestNewErrorTypeConstants` (added in PR #243) or append a sibling test:

```go
func TestErrMissingOrgContext(t *testing.T) {
	if string(ErrMissingOrgContext) != "missing_org_context" {
		t.Errorf("got %q, want missing_org_context", ErrMissingOrgContext)
	}
}
```

- [ ] **Step 2: Run, confirm it fails**

```
cd backend && go test ./internal/models/errors/...
```
Expected: FAIL — `undefined: ErrMissingOrgContext`.

- [ ] **Step 3: Add the constant**

In `backend/internal/models/errors/errors.go`, extend the const block:

```go
const (
	ErrValidation         ErrorType = "validation_error"
	ErrNotFound           ErrorType = "not_found"
	ErrConflict           ErrorType = "conflict"
	ErrInternal           ErrorType = "internal_error"
	ErrBadRequest         ErrorType = "bad_request"
	ErrUnauthorized       ErrorType = "unauthorized"
	ErrForbidden          ErrorType = "forbidden"
	ErrRateLimited        ErrorType = "rate_limited"
	ErrMethodNotAllowed   ErrorType = "method_not_allowed"
	ErrUnsupportedMedia   ErrorType = "unsupported_media_type"
	ErrMissingOrgContext  ErrorType = "missing_org_context"
)
```

- [ ] **Step 4: Extend the swagger enum annotation in `ErrorResponse`**

Same file. Find the line with the `enums:"..."` annotation and append `,missing_org_context`:

```go
		Type      string       `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error,method_not_allowed,unsupported_media_type,missing_org_context" extensions:"x-extensible-enum=true"`
```

- [ ] **Step 5: Run, confirm it passes**

```
cd backend && go test ./internal/models/errors/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/errors/errors.go backend/internal/models/errors/errors_test.go
git commit -m "feat(api-errors): add missing_org_context error type

Adds the ErrorType constant for the TRA-537 follow-up sweep. The new
type covers handler-level 'auth ok but org context not resolvable'
emissions that today emit 401 with misleading titles. Backward-compatible
via the existing x-extensible-enum=true annotation."
```

---

### Task 2: Add `RespondMissingOrgContext` helper + test

**Files:**
- Modify: `backend/internal/util/httputil/method_error.go`
- Modify: `backend/internal/util/httputil/method_error_test.go`

- [ ] **Step 1: Append the failing test**

In `backend/internal/util/httputil/method_error_test.go`:

```go
func TestRespondMissingOrgContext_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/assets", nil)
	httputil.RespondMissingOrgContext(w, r, "req-mo")

	if w.Code != 422 {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Type != string(apierrors.ErrMissingOrgContext) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrMissingOrgContext)
	}
	if resp.Error.Title != "Organization context required" {
		t.Errorf("title = %q, want Organization context required", resp.Error.Title)
	}
	if resp.Error.Status != 422 {
		t.Errorf("status field = %d, want 422", resp.Error.Status)
	}
	if resp.Error.Detail != "This request requires an active organization context. Select an organization or re-authenticate." {
		t.Errorf("detail = %q, want canonical message", resp.Error.Detail)
	}
	if resp.Error.RequestID != "req-mo" {
		t.Errorf("request_id = %q, want req-mo", resp.Error.RequestID)
	}
}
```

- [ ] **Step 2: Run, confirm it fails**

```
cd backend && go test ./internal/util/httputil/...
```
Expected: FAIL — `undefined: httputil.RespondMissingOrgContext`.

- [ ] **Step 3: Implement the helper**

Append to `backend/internal/util/httputil/method_error.go`:

```go
// RespondMissingOrgContext writes the canonical 422 envelope used when
// auth has succeeded but the request lacks an active organization context.
//
// Two real-world causes:
//   - Session-authenticated user has no current org (just signed up,
//     deleted last org, cleared client state). Frontend should route to
//     the org picker.
//   - API-key request with no org bound (shouldn't happen in production:
//     keys are minted per-org). Integrator should re-mint with the
//     correct org.
//
// Title and detail are both fixed; the variable cause does not surface
// per-call to keep the contract clean for client-side branching.
func RespondMissingOrgContext(w http.ResponseWriter, r *http.Request, requestID string) {
	WriteJSONError(w, r, http.StatusUnprocessableEntity, apierrors.ErrMissingOrgContext,
		"Organization context required",
		"This request requires an active organization context. Select an organization or re-authenticate.",
		requestID)
}
```

- [ ] **Step 4: Run, confirm it passes**

```
cd backend && go test ./internal/util/httputil/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/httputil/method_error.go backend/internal/util/httputil/method_error_test.go
git commit -m "feat(httputil): add RespondMissingOrgContext helper

Parallels Respond401/Respond404/Respond405/Respond415: fixed title
\"Organization context required\", fixed detail describing recovery,
type=missing_org_context, status 422.

422 (not 401) because auth genuinely succeeded and the state is
recoverable client-side; using 5xx would noise up Sentry on what is
effectively a frontend bug or an integrator with the wrong key."
```

---

### Task 3: Sweep `handlers/assets`

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (12 sites — Pattern A)
- Modify: `backend/internal/handlers/assets/bulkimport.go` (2 sites — Pattern C, each splits into 2 checks)

- [ ] **Step 1: Branch safety check**

```
git branch --show-current
```
Expected: `fix/tra-537-missing-org-context`. STOP if anything else.

- [ ] **Step 2: Migrate the 12 Pattern A sites in `assets.go`**

Each of the 12 sites in `assets.go` matches:

```go
orgID, err := middleware.GetRequestOrgID(r)  // or req
if err != nil {
    httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
        apierrors.<SomeAssetOp>Failed, "missing organization context", requestID)
    return
}
```

Replace the `WriteJSONError` call with:

```go
httputil.RespondMissingOrgContext(w, r, requestID)
```

Preserve the existing `w`/`r`/`req` and `requestID`/`reqID`/`ctx` variable names per site. The misleading title (e.g., `apierrors.AssetCreateFailed = "Failed to create asset"`) is dropped — it never belonged on a 401/422 anyway. The detail "missing organization context" is also dropped because the helper provides a canonical, more informative one.

12 sites: lines 68, 188, 310, 361, 456, 539, 590, 666, 725, 746, 767, 788 (line numbers may have drifted; locate by pattern).

- [ ] **Step 3: Migrate the 2 Pattern C sites in `bulkimport.go` — each splits into two checks**

Current shape at `bulkimport.go:41-44` and `bulkimport.go:97-99`:

```go
claims := middleware.GetUserClaims(r)
if claims == nil || claims.CurrentOrgID == nil {
    httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
        apierrors.BulkImport<...>MissingOrg, "", requestID)
    return
}
orgID := *claims.CurrentOrgID
```

Replace with two distinct checks:

```go
claims := middleware.GetUserClaims(r)
if claims == nil {
    httputil.Respond401(w, r, "Session authentication required", requestID)
    return
}
if claims.CurrentOrgID == nil {
    httputil.RespondMissingOrgContext(w, r, requestID)
    return
}
orgID := *claims.CurrentOrgID
```

Apply this transform at both bulkimport.go sites.

- [ ] **Step 4: Confirm no `http.StatusUnauthorized` remains in either file**

```
grep -n "http\.StatusUnauthorized" backend/internal/handlers/assets/assets.go backend/internal/handlers/assets/bulkimport.go
```
Expected: zero matches.

The `modelerrors` import in both files is still used by non-401 paths (400, 409, 500). Keep it.

- [ ] **Step 5: Compile + test**

```
cd backend
go build ./internal/handlers/assets/...
go test ./internal/handlers/assets/... -count=1
```
Expected: clean. Integration tests are DB-dependent and may time out — acceptable.

- [ ] **Step 6: Branch safety + commit**

```
git branch --show-current
```

```bash
git add backend/internal/handlers/assets/
git commit -m "refactor(handlers): TRA-537 assets handler 401 sites use canonical helpers

12 Pattern A sites in assets.go (GetRequestOrgID failure) migrated to
RespondMissingOrgContext (422 + missing_org_context). 2 Pattern C sites
in bulkimport.go split into separate claims-nil (Respond401) and
current-org-nil (RespondMissingOrgContext) checks."
```

---

### Task 4: Sweep `handlers/locations`

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (15 sites — Pattern A)

- [ ] **Step 1: Branch safety check**

- [ ] **Step 2: Migrate all 15 sites**

Same Pattern A transform as Task 3 Step 2. Each site is `GetRequestOrgID(req)` (or `(r)`) failure followed by `WriteJSONError(... StatusUnauthorized ...)`. Replace with `httputil.RespondMissingOrgContext(w, req, requestID)` (preserve existing variable names per site).

15 sites: lines 59, 159, 287, 403, 483, 519, 574, 643, 712, 791, 867, 926, 947, 968, 989.

- [ ] **Step 3: Confirm no `http.StatusUnauthorized` remains**

```
grep -n "http\.StatusUnauthorized" backend/internal/handlers/locations/locations.go
```
Expected: zero matches.

- [ ] **Step 4: Compile**

```
cd backend && go build ./internal/handlers/locations/...
```

- [ ] **Step 5: Branch safety + commit**

```bash
git add backend/internal/handlers/locations/
git commit -m "refactor(handlers): TRA-537 locations handler 401 sites use RespondMissingOrgContext

15 Pattern A sites in locations.go migrated. Same mechanical sweep as
the assets handler in the prior commit."
```

---

### Task 5: Sweep `handlers/orgs` (mixed patterns)

**Files:**
- Modify: `backend/internal/handlers/orgs/orgs.go` (2 sites — Pattern B)
- Modify: `backend/internal/handlers/orgs/me.go` (2 sites — Pattern B)
- Modify: `backend/internal/handlers/orgs/members.go` (1 site — Pattern B)
- Modify: `backend/internal/handlers/orgs/invitations.go` (1 site — Pattern A)
- Modify: `backend/internal/handlers/orgs/api_keys.go` (2 sites — Pattern F)
- Modify: `backend/internal/handlers/orgs/public.go` (2 sites — Pattern D + Pattern E)

- [ ] **Step 1: Branch safety check**

- [ ] **Step 2: Pattern B migrations (orgs.go, me.go, members.go) → `Respond401`**

Each Pattern B site looks like:

```go
claims := middleware.GetUserClaims(r)
if claims == nil {
    httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
        <variable title>, <variable detail>, requestID)
    return
}
```

Replace with:

```go
claims := middleware.GetUserClaims(r)
if claims == nil {
    httputil.Respond401(w, r, "Session authentication required", requestID)
    return
}
```

Use `"Session authentication required"` as the canonical detail (matches the rbac.go fix in PR #243). If the existing call has a more specific detail you want to preserve (e.g., something operation-specific), use it instead — judgment call per site.

Sites: orgs.go:48, orgs.go:81, me.go:44, me.go:77, members.go:139.

- [ ] **Step 3: Pattern A migration (invitations.go:68) → `RespondMissingOrgContext`**

Same shape as Task 3/4 Pattern A. Replace with `httputil.RespondMissingOrgContext(w, r, requestID)`.

- [ ] **Step 4: Pattern F migrations (api_keys.go) → `Respond401`**

Two distinct detail strings. At `api_keys.go:65` (parent key not found in storage):

```go
if stderrors.Is(err, storage.ErrAPIKeyNotFound) {
    httputil.Respond401(w, r, "API key is no longer valid", reqID)
    return
}
```

At `api_keys.go:76` (no session claims AND no API-key principal — the `else` branch of an auth-method-resolution if/else):

```go
} else {
    httputil.Respond401(w, r, "Authentication required", reqID)
    return
}
```

(The `else` arm is defensive — both upstream auth paths are required to populate context. Use the generic detail.)

- [ ] **Step 5: Pattern D migration (public.go:42) → `Respond401`**

```go
principal := middleware.GetAPIKeyPrincipal(r)
if principal == nil {
    httputil.Respond401(w, r, "API key authentication required", reqID)
    return
}
```

- [ ] **Step 6: Pattern E migration (public.go:55) → `Respond401` with specific detail**

```go
if org == nil {
    // Org was deleted between key issuance and this request — treat the key as unauthorized.
    httputil.Respond401(w, r, "Organization no longer exists", reqID)
    return
}
```

- [ ] **Step 7: Confirm no `http.StatusUnauthorized` remains across all 6 files**

```
grep -n "http\.StatusUnauthorized" backend/internal/handlers/orgs/*.go
```
Expected: zero matches in source files (test files are OK).

- [ ] **Step 8: Compile + test**

```
cd backend
go build ./internal/handlers/orgs/...
go test ./internal/handlers/orgs/... -count=1
```

- [ ] **Step 9: Branch safety + commit**

```bash
git add backend/internal/handlers/orgs/
git commit -m "refactor(handlers): TRA-537 orgs handler 401 sites use canonical helpers

10 sites across orgs/{orgs,me,members,invitations,api_keys,public}.go
migrated:
- 5 Pattern B (claims==nil) sites → Respond401 with 'Session authentication required'
- 1 Pattern A (GetRequestOrgID fail) site in invitations.go → RespondMissingOrgContext
- 2 Pattern F (api_keys creator-resolution) sites → Respond401 with specific details
- 1 Pattern D (api-key principal nil) + 1 Pattern E (org deleted) site
  in public.go → Respond401 with case-specific details"
```

---

### Task 6: Sweep remaining handlers (`auth`, `lookup`, `reports`, `inventory`)

**Files:**
- Modify: `backend/internal/handlers/auth/auth.go` (1 site — Pattern B)
- Modify: `backend/internal/handlers/lookup/lookup.go` (2 sites — Pattern A)
- Modify: `backend/internal/handlers/reports/current_locations.go` (1 site — Pattern A)
- Modify: `backend/internal/handlers/reports/asset_history.go` (2 sites — Pattern A)
- Modify: `backend/internal/handlers/inventory/save.go` (1 site — Pattern A)

- [ ] **Step 1: Branch safety check**

- [ ] **Step 2: Pattern B migration in `auth/auth.go:247`**

```go
claims := middleware.GetUserClaims(r)
if claims == nil {
    httputil.Respond401(w, r, "Please log in to accept this invitation", middleware.GetRequestID(r.Context()))
    return
}
```

Preserve the existing operation-specific detail string `"Please log in to accept this invitation"` — it's already informative and doesn't conflict with the contract.

This file uses `errors` (alias for `models/errors`) rather than `modelerrors`. After the migration, check if the import is still used elsewhere; remove if unused (run `go build` to confirm).

- [ ] **Step 3: Pattern A migrations in lookup, reports, inventory**

Apply the standard Pattern A transform (`RespondMissingOrgContext`) to:

- `lookup/lookup.go:43, :101`
- `reports/current_locations.go:60`
- `reports/asset_history.go:70, :155`
- `inventory/save.go:82`

6 sites total.

- [ ] **Step 4: Confirm no `http.StatusUnauthorized` remains in any of the four packages**

```
grep -rn "http\.StatusUnauthorized" backend/internal/handlers/auth/ backend/internal/handlers/lookup/ backend/internal/handlers/reports/ backend/internal/handlers/inventory/
```
Expected: zero matches in source files.

- [ ] **Step 5: Compile + test**

```
cd backend
go build ./internal/handlers/auth/... ./internal/handlers/lookup/... ./internal/handlers/reports/... ./internal/handlers/inventory/...
go test ./internal/handlers/auth/... ./internal/handlers/lookup/... ./internal/handlers/reports/... ./internal/handlers/inventory/... -count=1
```

- [ ] **Step 6: Branch safety + commit**

```bash
git add backend/internal/handlers/auth/ backend/internal/handlers/lookup/ backend/internal/handlers/reports/ backend/internal/handlers/inventory/
git commit -m "refactor(handlers): TRA-537 remaining 401 sites use canonical helpers

7 sites across auth, lookup, reports, inventory packages migrated:
- 1 Pattern B (claims==nil) in auth/auth.go → Respond401 with operation-specific detail
- 6 Pattern A (GetRequestOrgID fail) sites → RespondMissingOrgContext

With this commit, every public 401-or-422-class emission site under
backend/internal/handlers/ routes through Respond401 or
RespondMissingOrgContext. The TRA-537 audit follow-up is complete."
```

---

### Task 7: Add 422 contract test

**Files:**
- Modify: `backend/internal/cmd/serve/contract_smoke_test.go`

- [ ] **Step 1: Branch safety check**

- [ ] **Step 2: Append the contract test**

```go
// TestContract_MissingOrgContext_EnvelopeAndType covers TRA-537 follow-up:
// 422 with type=missing_org_context for the "auth ok but org missing"
// state. The test wires RespondMissingOrgContext through chi to confirm
// the helper produces the documented envelope shape end-to-end.
func TestContract_MissingOrgContext_EnvelopeAndType(t *testing.T) {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Get("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondMissingOrgContext(w, req, middleware.GetRequestID(req.Context()))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "missing_org_context", resp.Error.Type)
	require.Equal(t, "Organization context required", resp.Error.Title)
	require.Equal(t, 422, resp.Error.Status)
	require.Contains(t, resp.Error.Detail, "active organization context")
	require.NotEmpty(t, resp.Error.RequestID, "request_id must propagate into envelope")
}
```

All required imports (`chi`, `middleware`, `httputil`, `apierrors`, `json`, `httptest`, `http`, `require`, `testing`) are already in scope from existing tests in the file.

- [ ] **Step 3: Run, confirm pass**

```
cd backend
go test ./internal/cmd/serve/... -v -run TestContract_MissingOrgContext
```
Expected: PASS.

- [ ] **Step 4: Branch safety + commit**

```bash
git add backend/internal/cmd/serve/contract_smoke_test.go
git commit -m "test(api): TRA-537 contract test for missing_org_context envelope

End-to-end contract test for the new 422 + missing_org_context envelope
shape. Exercises RespondMissingOrgContext through a chi router and
asserts: status 422, type=missing_org_context, fixed title, status field
422, detail contains the recovery hint, request_id propagates."
```

---

### Task 8: Validate, sync OpenAPI spec, push, PR

- [ ] **Step 1: Branch safety check**

- [ ] **Step 2: Run full validation**

```
just validate
```

Frontend may fail if `node_modules` isn't installed in the worktree — that's pre-existing infrastructure, not introduced here. Backend lint + tests are what matter:

```
just backend lint
just backend test
```

If any test outside this scope is broken because it asserted on the old 401 envelope shape, fix it in lockstep (likely candidates: integration tests that pinned `error.title` or `error.type` to the old strings).

- [ ] **Step 3: Sync the OpenAPI public spec**

The `docs/api/openapi.public.{json,yaml}` files mirror the swagger annotation in `errors.go`. After Task 1's enum change, these may need regeneration. Either run the swagger gen tool (likely `just backend swagger` or similar) or hand-edit the enum to add `missing_org_context`.

If hand-editing:
- `docs/api/openapi.public.yaml` — find the `error.type` enum block and append `- missing_org_context`
- `docs/api/openapi.public.json` — find the `error.type` `enum` array and append `"missing_org_context"`

Commit if changed:

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "chore(api): sync docs/api public spec with TRA-537 missing_org_context type"
```

- [ ] **Step 4: Final validate**

```
just backend lint && just backend test
```
Expected: clean.

- [ ] **Step 5: Push + open PR**

```
git push -u origin fix/tra-537-missing-org-context
```

```bash
gh pr create --title "fix(api): TRA-537 sweep handler 401 sites — missing_org_context + Respond401" --body "$(cat <<'EOF'
## Summary

Audit follow-up to TRA-538/541 (PR #243). Sweeps ~46 handler-level 401 emission sites identified during PR #243's final review.

- **Pattern A (~33 sites)** — `GetRequestOrgID(r)` failure: migrated to new **422 `missing_org_context`** type via new **`RespondMissingOrgContext`** helper. Auth genuinely succeeded; this is a recoverable client-side state (frontend → org picker; integrator → re-mint key with correct org). 422 (not 5xx) avoids Sentry noise on what is effectively a frontend bug or wrong-key scenario.
- **Pattern B (~6 sites)** — `claims == nil` failure: same class as the rbac.go fix in PR #243. Migrated to **`Respond401`** with descriptive detail.
- **Pattern C (2 sites)** — compound `claims == nil || claims.CurrentOrgID == nil` in bulkimport.go: split into two distinct checks (Respond401 + RespondMissingOrgContext).
- **Pattern D/E (2 sites)** — orgs/public.go API-key principal nil + org-deleted-after-key-issuance: migrated to `Respond401` with case-specific details.
- **Pattern F (2 sites)** — orgs/api_keys.go creator-resolution failures: migrated to `Respond401` with operation-specific details.

Design rationale: TRA-537 audit-follow-up comment (option B' = 422 + new type + helper).

## Why this matters

Same envelope-contract violation TRA-538 fixed in the auth-middleware layer — variable strings in title position with empty or misleading detail. The handler layer was deferred from TRA-538 because the fix needed a design call (401 vs 500 vs new type). With the design locked in (B'), this PR completes the sweep.

## Backward compatibility

Adding `missing_org_context` to the swagger enum is additive — `x-extensible-enum=true` already tells generated clients to tolerate unknown values.

## Test plan

- [ ] CI green (`just validate`)
- [ ] Preview spot check: hit a public-API endpoint with a session JWT that lacks `current_org_id` (e.g., a freshly-signed-up user); confirm 422 + `missing_org_context` instead of 401
- [ ] Preview spot check: confirm Pattern B sites (e.g., `/api/v1/auth/accept-invite` without a session cookie) emit `Respond401` envelope shape

## Docs follow-up

The trakrf-docs PR (sequenced after PR #243 + this PR merge) will:
- Reverse "title may vary" wording in `docs/api/errors.md`
- Add catalog rows for `method_not_allowed`, `unsupported_media_type`, AND `missing_org_context`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 6: Return the PR URL**

---

## Self-review

**Spec coverage:** All 6 patterns (A through F) have explicit task coverage. Pattern A is in T3/T4/T5/T6. Pattern B is in T5/T6. Pattern C is in T3. Pattern D/E is in T5. Pattern F is in T5.

**Placeholder scan:** none. All steps have concrete code or commands.

**Type/method consistency:** `RespondMissingOrgContext(w, r, requestID)` signature is defined in T2 and used unchanged across T3-T7. `ErrMissingOrgContext` is defined in T1 and referenced in T2's test.

**Sweep completeness check:** After T6, `grep -rn "http\.StatusUnauthorized" backend/internal/handlers/ backend/internal/cmd/serve/ backend/internal/middleware/` should return only `testhandler/invitations.go:49` (the dev-only `http.Error` site documented as out-of-scope in the TRA-538 plan).
