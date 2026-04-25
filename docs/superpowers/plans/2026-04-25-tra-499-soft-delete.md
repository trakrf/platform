# TRA-499 Soft-Delete Documentation & Lifecycle Lock-In Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `DELETE /api/v1/assets/{identifier}` and `DELETE /api/v1/locations/{identifier}` honest in their public docs, and lock in the soft-delete visibility contract with regression tests. No runtime behavior changes — the runtime is already correct.

**Architecture:** Two parts. (1) Append targeted regression tests to existing `public_write_integration_test.go` files, asserting that soft-deleted records are unreachable via GET-by-identifier and via list endpoints (default *and* `?is_active=false`). (2) Update Swagger annotations on six handler functions (DELETE, GET-by-identifier, and list `is_active` param for each of assets and locations) and regenerate the embedded OpenAPI spec. The runtime is *already* a soft-delete via `UPDATE … SET deleted_at = now()`; we are documenting reality and locking it down, not changing it.

**Tech Stack:** Go 1.x, chi router, pgx, swag (Swagger annotation generator), testify, real Postgres via `testutil.SetupTestDB`. Integration tests require the `integration` build tag and a live Postgres reachable via the test harness.

**Spec:** `docs/superpowers/specs/2026-04-25-tra-499-soft-delete-design.md`

---

## Pre-flight

Before starting any task, confirm the worktree environment is wired up:

```bash
cd /home/mike/platform/.worktrees/tra-499-delete-soft-delete
git status
# expect: clean working tree on miks2u/tra-499-delete-soft-delete
```

All `just` commands in this plan must be run from the project root (`/home/mike/platform/.worktrees/tra-499-delete-soft-delete`), not from inside `backend/`. The repo's CLAUDE.md is explicit on this: use `just backend <cmd>` delegation.

The integration tests in this plan need a live Postgres. The existing test harness (`testutil.SetupTestDB`) handles DB provisioning; if `just backend test-integration` already works for you on this branch, you're set. If it doesn't, fix the local Postgres setup before proceeding — do not skip integration tests.

---

## Task 1: Add asset soft-delete visibility regression test

**Why this comes first:** Existing tests already cover (a) basic DELETE → 204, (b) re-DELETE → 404, (c) re-POST after delete → 201. The gap is **read-side visibility after delete**: GET-by-identifier returning 404, list endpoints excluding the soft-deleted row, and `?is_active=false` *not* surfacing it (the bug the ticket originally feared, now confirmed absent and getting locked in).

**Files:**
- Modify: `backend/internal/handlers/assets/public_write_integration_test.go` (append a new test function at end of file)

- [ ] **Step 1: Append the new test function**

Open `backend/internal/handlers/assets/public_write_integration_test.go` and append at end of file:

```go
// TRA-499: lock in the read-side visibility contract for soft-deleted assets.
// After DELETE, the record must be unreachable via:
//   - GET /api/v1/assets/{identifier}     → 404
//   - GET /api/v1/assets                  → not in default list
//   - GET /api/v1/assets?is_active=false  → not surfaced (is_active is an
//                                            independent business-state flag,
//                                            not a soft-delete view)
func TestSoftDeleteVisibility_Asset(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-soft-delete-visibility")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:read", "assets:write"})
	writeRouter := buildAssetsPublicWriteRouter(store)
	readRouter := buildAssetsPublicRouter(store)

	auth := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
	}

	// 1. Create asset.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/assets",
		bytes.NewBufferString(`{"identifier":"tra499-vis-1","name":"v","type":"asset"}`))
	auth(createReq)
	createW := httptest.NewRecorder()
	writeRouter.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code, createW.Body.String())

	// 2. Sanity: live record is visible by identifier.
	getLiveReq := httptest.NewRequest(http.MethodGet, "/api/v1/assets/tra499-vis-1", nil)
	auth(getLiveReq)
	getLiveW := httptest.NewRecorder()
	readRouter.ServeHTTP(getLiveW, getLiveReq)
	require.Equal(t, http.StatusOK, getLiveW.Code, getLiveW.Body.String())

	// 3. Soft-delete the asset.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/tra499-vis-1", nil)
	auth(delReq)
	delW := httptest.NewRecorder()
	writeRouter.ServeHTTP(delW, delReq)
	require.Equal(t, http.StatusNoContent, delW.Code, delW.Body.String())

	// 4. GET-by-identifier of soft-deleted record → 404.
	getDelReq := httptest.NewRequest(http.MethodGet, "/api/v1/assets/tra499-vis-1", nil)
	auth(getDelReq)
	getDelW := httptest.NewRecorder()
	readRouter.ServeHTTP(getDelW, getDelReq)
	assert.Equal(t, http.StatusNotFound, getDelW.Code, getDelW.Body.String())

	// 5. Default list excludes soft-deleted record.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	auth(listReq)
	listW := httptest.NewRecorder()
	readRouter.ServeHTTP(listW, listReq)
	require.Equal(t, http.StatusOK, listW.Code, listW.Body.String())
	var listBody map[string]any
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listBody))
	assert.EqualValues(t, 0, listBody["total_count"], "default list must exclude soft-deleted asset")

	// 6. ?is_active=false also excludes soft-deleted record (is_active is independent of soft-delete).
	listInactiveReq := httptest.NewRequest(http.MethodGet, "/api/v1/assets?is_active=false", nil)
	auth(listInactiveReq)
	listInactiveW := httptest.NewRecorder()
	readRouter.ServeHTTP(listInactiveW, listInactiveReq)
	require.Equal(t, http.StatusOK, listInactiveW.Code, listInactiveW.Body.String())
	var listInactiveBody map[string]any
	require.NoError(t, json.Unmarshal(listInactiveW.Body.Bytes(), &listInactiveBody))
	assert.EqualValues(t, 0, listInactiveBody["total_count"],
		"is_active=false must NOT surface soft-deleted records — is_active is a business-state flag, not a soft-delete view")
}
```

This reuses the existing `seedOrgAndKey`, `buildAssetsPublicWriteRouter`, and `buildAssetsPublicRouter` helpers already in the package. No new imports are needed beyond what `public_write_integration_test.go` already has (`bytes`, `encoding/json`, `net/http`, `net/http/httptest`, `testing`, `chi`, `pgxpool`, `testify/assert`, `testify/require`, `testutil`).

- [ ] **Step 2: Run the test**

```bash
just backend test-integration ./internal/handlers/assets/... -run TestSoftDeleteVisibility_Asset
```

Expected: PASS. The runtime already implements the contract; this test locks it in. If it fails, **stop** — that means the runtime is broken (contradicting the spec premise). Investigate before proceeding.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/assets/public_write_integration_test.go
git commit -m "test(tra-499): lock in soft-delete read-side visibility for assets

Asserts that GET-by-identifier returns 404 and list endpoints (default and
?is_active=false) exclude soft-deleted records. is_active is a business-state
flag independent of deleted_at, so ?is_active=false must not surface deletes."
```

---

## Task 2: Add location soft-delete visibility regression test

Mirror of Task 1 for locations. Same shape, different routes/router builders/identifier prefix.

**Files:**
- Modify: `backend/internal/handlers/locations/public_write_integration_test.go` (append a new test function at end of file)

- [ ] **Step 1: Append the new test function**

Open `backend/internal/handlers/locations/public_write_integration_test.go` and append at end of file:

```go
// TRA-499: lock in the read-side visibility contract for soft-deleted locations.
// After DELETE, the record must be unreachable via:
//   - GET /api/v1/locations/{identifier}     → 404
//   - GET /api/v1/locations                  → not in default list
//   - GET /api/v1/locations?is_active=false  → not surfaced (is_active is an
//                                               independent business-state flag,
//                                               not a soft-delete view)
func TestSoftDeleteVisibility_Location(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-soft-delete-visibility")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"locations:read", "locations:write"})
	writeRouter := buildLocationsPublicWriteRouter(store)
	readRouter := buildLocationsPublicRouter(store)

	auth := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
	}

	// 1. Create location.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/locations",
		bytes.NewBufferString(`{"identifier":"tra499-vis-1","name":"v"}`))
	auth(createReq)
	createW := httptest.NewRecorder()
	writeRouter.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code, createW.Body.String())

	// 2. Sanity: live record is visible by identifier.
	getLiveReq := httptest.NewRequest(http.MethodGet, "/api/v1/locations/tra499-vis-1", nil)
	auth(getLiveReq)
	getLiveW := httptest.NewRecorder()
	readRouter.ServeHTTP(getLiveW, getLiveReq)
	require.Equal(t, http.StatusOK, getLiveW.Code, getLiveW.Body.String())

	// 3. Soft-delete the location.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/locations/tra499-vis-1", nil)
	auth(delReq)
	delW := httptest.NewRecorder()
	writeRouter.ServeHTTP(delW, delReq)
	require.Equal(t, http.StatusNoContent, delW.Code, delW.Body.String())

	// 4. GET-by-identifier of soft-deleted record → 404.
	getDelReq := httptest.NewRequest(http.MethodGet, "/api/v1/locations/tra499-vis-1", nil)
	auth(getDelReq)
	getDelW := httptest.NewRecorder()
	readRouter.ServeHTTP(getDelW, getDelReq)
	assert.Equal(t, http.StatusNotFound, getDelW.Code, getDelW.Body.String())

	// 5. Default list excludes soft-deleted record.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/locations", nil)
	auth(listReq)
	listW := httptest.NewRecorder()
	readRouter.ServeHTTP(listW, listReq)
	require.Equal(t, http.StatusOK, listW.Code, listW.Body.String())
	var listBody map[string]any
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listBody))
	assert.EqualValues(t, 0, listBody["total_count"], "default list must exclude soft-deleted location")

	// 6. ?is_active=false also excludes soft-deleted record.
	listInactiveReq := httptest.NewRequest(http.MethodGet, "/api/v1/locations?is_active=false", nil)
	auth(listInactiveReq)
	listInactiveW := httptest.NewRecorder()
	readRouter.ServeHTTP(listInactiveW, listInactiveReq)
	require.Equal(t, http.StatusOK, listInactiveW.Code, listInactiveW.Body.String())
	var listInactiveBody map[string]any
	require.NoError(t, json.Unmarshal(listInactiveW.Body.Bytes(), &listInactiveBody))
	assert.EqualValues(t, 0, listInactiveBody["total_count"],
		"is_active=false must NOT surface soft-deleted records — is_active is a business-state flag, not a soft-delete view")
}
```

Caveat: this test references `buildLocationsPublicWriteRouter` and `buildLocationsPublicRouter` and `seedOrgAndKey`. These helpers already exist in the locations test package — confirm by grepping:

```bash
grep -n "func buildLocationsPublicWriteRouter\|func buildLocationsPublicRouter\|func seedOrgAndKey" backend/internal/handlers/locations/*.go
```

Expected: all three exist (in `public_write_integration_test.go` and `public_integration_test.go`). If any helper is named differently in the locations package, adjust the call site to match — do **not** rename the helper.

If imports differ from the assets test (e.g., the locations file already imports everything needed), no import edits are required since all the symbols used (`bytes`, `json`, `http`, `httptest`, `testing`, `pgxpool`, `assert`, `require`, `testutil`) are universally present in this file.

- [ ] **Step 2: Run the test**

```bash
just backend test-integration ./internal/handlers/locations/... -run TestSoftDeleteVisibility_Location
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "test(tra-499): lock in soft-delete read-side visibility for locations

Mirror of the assets visibility test — asserts GET-by-identifier returns 404
and list endpoints exclude soft-deleted records, including under
?is_active=false."
```

---

## Task 3: Update Swagger annotations on assets handler

Replace the misleading "marks the asset inactive" copy with descriptions that accurately describe `deleted_at`-based soft-delete and clarify the `is_active` filter's independence.

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (three swag annotation blocks: DELETE, list `is_active` param, GET-by-identifier)

- [ ] **Step 1: Update the DELETE annotation**

Find this line (currently around line 324):

```go
// @Description  Soft-delete an asset by its natural identifier. The asset is marked inactive and removed from future list results.
```

Replace with:

```go
// @Description  Soft-delete an asset by its natural identifier. The record is removed from all subsequent queries and its identifier becomes immediately available for reuse. Soft-deleted records are retained internally for audit purposes but are not retrievable via this API. Returns 204 on success, 404 if the asset does not exist or has already been deleted.
```

- [ ] **Step 2: Update the list `is_active` param annotation**

Find this line (currently around line 419, inside the `ListAssets` swag block):

```go
// @Param is_active query bool  false "filter by active flag"
```

Replace with:

```go
// @Param is_active query bool  false "filter by the active business-state flag. Independent of soft-delete: soft-deleted records are excluded regardless of is_active value."
```

- [ ] **Step 3: Update the GET-by-identifier annotation**

Find the swag block above `func (handler *Handler) GetAssetByIdentifier` (currently around line 505). The block has a `@Summary` and a `@Description`. The current `@Description` (look at the file to find it; it precedes the `@Tags` line) needs to mention the 404 behavior on soft-delete. If the existing `@Description` is missing or uninformative, add or replace it with:

```go
// @Description Retrieve an asset by its natural identifier. Returns 404 if the asset does not exist or has been soft-deleted.
```

If the block already has a `@Description` line, replace just that line. If it does not, insert this line immediately after the existing `@Summary` line.

- [ ] **Step 4: Build the backend to verify the file still compiles**

```bash
just backend lint
```

Expected: clean (no `go vet` errors). The swag comments are just comments — they shouldn't break compilation, but `lint` also runs `go fmt` and will catch any accidental syntax breakage.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/assets/assets.go
git commit -m "docs(tra-499): drop soft-delete terminology from public asset annotations

DELETE description rewritten to describe the customer-observable contract
only (record removed from all subsequent queries; identifier reusable; 204
on success, 404 if not found or already deleted). GET-by-identifier notes
404 for non-existent records. is_active param left in its original terse
form. Stripe-style: public docs describe what the caller can observe and
do not leak implementation detail. OpenAPI regeneration follows in a later
commit."
```

---

## Task 4: Update Swagger annotations on locations handler

Mirror of Task 3 for locations.

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (three swag annotation blocks: DELETE, list `is_active` param, GET-by-identifier)

- [ ] **Step 1: Update the DELETE annotation**

Find this line (currently around line 269):

```go
// @Description Soft-delete a location by its natural identifier. The location is marked inactive and removed from future list results.
```

Replace with:

```go
// @Description Delete a location by its natural identifier. The location is removed from all subsequent queries and its identifier becomes immediately available for reuse. Returns 204 on success, 404 if the location does not exist or has already been deleted.
```

- [ ] **Step 2: Leave the list `is_active` param annotation as-is**

The param description (currently `// @Param is_active query bool  false "filter by active flag"`) does NOT need to change. Earlier versions of this plan proposed adding a "soft-delete is independent" disambiguation, but since we are dropping soft-delete terminology from public docs entirely, there is nothing to disambiguate from. Keep the existing terse form.

- [ ] **Step 3: Update the GET-by-identifier annotation**

Find the swag block above `func (handler *Handler) GetLocationByIdentifier` (currently around line 461). If an existing `@Description` line is present, replace it; if none, insert immediately after `@Summary`. Use:

```go
// @Description Retrieve a location by its natural identifier. Returns 404 if the location does not exist.
```

- [ ] **Step 4: Build the backend to verify the file still compiles**

```bash
just backend lint
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/locations/locations.go
git commit -m "docs(tra-499): drop soft-delete terminology from public location annotations

Mirror of the assets cleanup. DELETE description rewritten to describe the
customer-observable contract only (record removed from queries; identifier
reusable; 404 on re-delete). GET-by-identifier description added with the
same Stripe-style framing. is_active param left unchanged. OpenAPI
regeneration follows in a later commit."
```

---

## Task 5: Regenerate OpenAPI spec and verify diff scope

The annotation edits in Tasks 3 and 4 are not user-visible until `swag init` regenerates `docs/swagger.json` and the embedded `swaggerspec` package, and `apispec` rebuilds the public OpenAPI 3.0 file under `docs/api/`.

**Files:**
- Modify: `backend/docs/swagger.json` (regenerated)
- Modify: `backend/docs/swagger.yaml` (regenerated, if `swag` produces it)
- Modify: `backend/internal/handlers/swaggerspec/*` (regenerated)
- Modify: `docs/api/*` (regenerated public OpenAPI; relative to repo root)

- [ ] **Step 1: Regenerate**

```bash
just backend api-spec
```

Expected output: messages from `swag init` and the `apispec` Go tool, no errors.

- [ ] **Step 2: Inspect the diff**

```bash
git status
git diff --stat
```

Expected files in the diff: `backend/docs/swagger.json`, `backend/docs/swagger.yaml` (if produced), `backend/internal/handlers/swaggerspec/*`, and `docs/api/*`.

```bash
git diff -- backend/docs/swagger.json | grep -E '^[+-]' | grep -iE 'description|is_active' | head -40
```

Expected: only changes to the three asset descriptions and three location descriptions defined in Tasks 3–4. If the diff includes path/operation/parameter additions, deletions, or unrelated description churn, **stop** — that means the local `swag` version differs from CI's, which is a separate problem (see comment in `backend/justfile` around line 139–142). Resolve by aligning the local `swag` version with what CI uses, then re-run.

- [ ] **Step 3: Commit**

```bash
git add backend/docs/swagger.json backend/docs/swagger.yaml backend/internal/handlers/swaggerspec docs/api
git commit -m "docs(tra-499): regenerate OpenAPI spec with corrected swagger copy

Pulls the description edits from Tasks 3 and 4 through swag and apispec
into docs/swagger.json, the embedded swaggerspec package, and the public
docs/api/ bundle."
```

If `git status` shows additional regenerated files outside this list (e.g., a `docs.go` or `swagger.yaml` that doesn't already exist in repo history), include them in the `git add` to keep the working tree clean.

---

## Task 6: Final verification

A short pass that exercises all moving parts together.

- [ ] **Step 1: Run the full backend integration test suite**

```bash
just backend test-integration ./internal/handlers/assets/... ./internal/handlers/locations/...
```

Expected: ALL PASS, including the two new `TestSoftDeleteVisibility_*` tests and all pre-existing `TestCreate*_AfterSoftDelete_ReusesIdentifier` and `TestDelete*_SecondDeleteReturns404` tests.

- [ ] **Step 2: Confirm no untracked/modified files remain**

```bash
git status
```

Expected: clean working tree on `miks2u/tra-499-delete-soft-delete`. If anything is dirty, decide whether it's part of this work (then commit) or pre-existing noise (then `git stash` or revert as appropriate, but **do not** silently leave it dirty).

- [ ] **Step 3: Smoke-check the regenerated OpenAPI**

```bash
grep -A1 '"/api/v1/assets/{identifier}"' backend/docs/swagger.json | head -20
grep -A1 'is_active' backend/docs/swagger.json | head -20
```

Expected: descriptions reflect the new copy from Tasks 3–4. If the old "marked inactive" wording still appears anywhere in `backend/docs/swagger.json`, regeneration didn't fully take — revisit Task 5.

- [ ] **Step 4: Done**

Branch is ready for PR. Push and open a PR per the project's standard flow (see CLAUDE.md "Git Workflow"). Reference TRA-499 in the PR description and link the spec at `docs/superpowers/specs/2026-04-25-tra-499-soft-delete-design.md`.

---

## Self-Review Notes (post-write)

- **Spec coverage:** Every spec section is covered by exactly one task or is explicitly out-of-scope.
  - Doc string changes (DELETE / GET-by-id / `is_active` param × 2 resources) → Tasks 3 & 4
  - OpenAPI regeneration → Task 5
  - Lifecycle integration tests (gap analysis: existing tests cover most; visibility is the gap) → Tasks 1 & 2
  - "No handler/storage/schema changes" → no task; absence confirmed by Task 6 final verification
  - Re-DELETE → 404 (Stripe convention) → already covered by existing `TestDelete*_SecondDeleteReturns404`; not duplicated
- **Placeholders:** none. Every step has either exact code or an exact command with expected output.
- **Type consistency:** test helper names (`seedOrgAndKey`, `buildAssetsPublicRouter`, `buildAssetsPublicWriteRouter`, `buildLocationsPublicRouter`, `buildLocationsPublicWriteRouter`) match the existing codebase. Task 2 includes a `grep` verification step in case the locations helpers diverged in naming.
