# TRA-407 PR #2 — OpenAPI Coherence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the OpenAPI coherence gaps from TRA-407 item 6: (a) add `@ID` annotations to all public operations, (b) unify write/child/hierarchy routes to `{identifier}` (real behavior change — writes previously accepted only surrogate `{id}`), (c) flip incorrect `@Success 202` to `200`/`204`.

**Architecture:** Handler-layer-only migration. Write/child/hierarchy handlers swap `chi.URLParam(r, "id")` for `chi.URLParam(r, "identifier")`, then resolve identifier → internal surrogate via the existing `storage.GetAssetByIdentifier` / `GetLocationByIdentifier` lookup (same code path read handlers use). Storage API signatures unchanged. Child identifier routes keep `{identifierId}` as the inner param — the identifier row uses a permuted surrogate because `(org_id, type, value)` is NOT unique (temporal schema: `UNIQUE(org_id, type, value, valid_from)`), so no sensible natural key exists for the child row.

**Tech Stack:** Go 1.22+, chi v5, swaggo `@ID`/`@Success` annotations, `swag v1.16.6` pinned via CI (`just backend api-spec` is the regen entry point; CI's `api-spec.yml` enforces drift).

**Spec:** `docs/superpowers/specs/2026-04-20-tra-407-contract-bugs-design.md` (item 6 section)

**Scope:** PR #2 of 2. Item 6 only. PR #1 (items 1–5 runtime contract fixes) already merged on main as commit `5edc07e`.

---

## Deviation from spec

**Child identifier URL shape** — the spec proposed `DELETE /{resource}/{identifier}/identifiers/{value}`. Scouting (`backend/migrations/000009_identifiers.up.sql:6-28`) revealed the row uniqueness is `UNIQUE(org_id, type, value, valid_from)` — TEMPORAL — so `{value}` is ambiguous (multiple rows can share the same `(org_id, type, value)` across time via soft-delete / re-activation). The identifier row's permuted surrogate `id` (from `generate_permuted_id_trigger`) is the only unambiguous child key. Keeping `{identifierId}` for the child preserves correctness; the outer parent param still flips to `{identifier}`. The permuted IDs are opaque (not sequential), so they don't leak cardinality the way a bare integer surrogate would.

---

## File structure

**Modify:**
- `backend/internal/cmd/serve/router.go` — flip chi route patterns `{id}` → `{identifier}` on 11 routes (4 asset write/child, 4 location write/child, 3 location hierarchy). Leave list/create/current/get-by-identifier routes untouched.
- `backend/internal/handlers/assets/assets.go` — 4 handlers (`UpdateAsset`, `DeleteAsset`, `AddIdentifier`, `RemoveIdentifier`): swap `chi.URLParam(req, "id")` → `"identifier"`, add `GetAssetByIdentifier` lookup step, feed the resolved `.ID` into existing storage calls. Also swaggo annotations (`@ID`, `@Success`, path param names).
- `backend/internal/handlers/locations/locations.go` — 7 handlers (`Update`, `Delete`, `AddIdentifier`, `RemoveIdentifier`, `GetAncestors`, `GetChildren`, `GetDescendants`): same pattern. Also swaggo.
- `backend/internal/handlers/assets/assets.go` — also the `ListAssets`, `GetAssetByIdentifier` read annotations to add `@ID`.
- `backend/internal/handlers/locations/locations.go` — also the read annotations: `ListLocations`, `GetLocationByIdentifier`, `GetCurrentLocation`.
- Test files per handler: migrate test setups to use `{identifier}` URLs + add 404-on-unknown-identifier tests.

**Committed regeneration outputs:**
- `docs/api/openapi.public.json`, `docs/api/openapi.public.yaml` — regenerated via `just backend api-spec`; CI enforces drift.

**Create:** none.

---

### Task 1: Migrate `assets` write and child routes to `{identifier}`

**Files:**
- Modify: `backend/internal/cmd/serve/router.go` — 4 routes
- Modify: `backend/internal/handlers/assets/assets.go` — 4 handlers
- Modify: `backend/internal/handlers/assets/assets_test.go` and/or existing integration tests

**Routes to flip** (in `router.go` around lines 105–115 per scouting):

| Before | After |
|---|---|
| `r.Put("/{id}", assetsHandler.UpdateAsset)` | `r.Put("/{identifier}", assetsHandler.UpdateAsset)` |
| `r.Delete("/{id}", assetsHandler.DeleteAsset)` | `r.Delete("/{identifier}", assetsHandler.DeleteAsset)` |
| `r.Post("/{id}/identifiers", assetsHandler.AddIdentifier)` | `r.Post("/{identifier}/identifiers", assetsHandler.AddIdentifier)` |
| `r.Delete("/{id}/identifiers/{identifierId}", assetsHandler.RemoveIdentifier)` | `r.Delete("/{identifier}/identifiers/{identifierId}", assetsHandler.RemoveIdentifier)` |

**Handler changes** — per handler, swap the param read and add a lookup step. The existing read handler already does this lookup; mirror the pattern:

- [ ] **Step 1: Write failing tests**

For each of the 4 handlers, add or update tests that exercise the `{identifier}` URL shape AND the 404-on-unknown-identifier path. Read the existing `assets_test.go` / `public_write_integration_test.go` to find the current test harness; reuse it.

Minimum new tests:
```go
// URL shape: writes now take {identifier}, not {id}.
func TestAssetsUpdate_ByIdentifier_Works(t *testing.T) {
    // Arrange: create asset with identifier "TRA-407B-1".
    // Act: PUT /api/v1/assets/TRA-407B-1 with a valid UpdateAssetRequest.
    // Assert: 200 OK, body reflects the update.
}

// 404 path: unknown identifier no longer returns an invalid-id-parse 400/500.
func TestAssetsUpdate_UnknownIdentifier_Returns404(t *testing.T) {
    // Act: PUT /api/v1/assets/DOES-NOT-EXIST with any body.
    // Assert: 404 not_found, body.error.type == "not_found".
}

// Same pair for DeleteAsset.
func TestAssetsDelete_ByIdentifier_Works(t *testing.T) { /* ... */ }
func TestAssetsDelete_UnknownIdentifier_Returns404(t *testing.T) { /* ... */ }

// AddIdentifier + RemoveIdentifier: outer param flips to {identifier}, inner {identifierId} stays.
func TestAssetsAddIdentifier_ByIdentifier_Works(t *testing.T) { /* ... */ }
func TestAssetsAddIdentifier_UnknownParent_Returns404(t *testing.T) { /* ... */ }
func TestAssetsRemoveIdentifier_ByIdentifier_Works(t *testing.T) { /* ... */ }
func TestAssetsRemoveIdentifier_UnknownParent_Returns404(t *testing.T) { /* ... */ }
```

If existing tests assert the `/api/v1/assets/{id}` URL shape, UPDATE them to use `{identifier}` — those assertions are stale after this migration.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/handlers/assets/...
```
Expected: new `ByIdentifier` tests FAIL (route not defined; or routed but handler parses `chi.URLParam(r, "id")` which is now empty string and returns a 400/500). Old tests using `/{id}` may also start failing — that's expected, they need to migrate.

- [ ] **Step 3: Update `router.go` route patterns**

In `backend/internal/cmd/serve/router.go`, in the assets route block (around lines 105–115), flip the 4 route patterns per the table above. Leave list/create/get-by-identifier alone.

- [ ] **Step 4: Migrate `UpdateAsset` handler**

In `backend/internal/handlers/assets/assets.go` around line 149:

Replace the param-read block (lines 149–170-ish):
```go
func (h *Handler) UpdateAsset(w http.ResponseWriter, req *http.Request) {
    requestID := middleware.GetRequestID(req.Context())
    idStr := chi.URLParam(req, "id")
    id, err := strconv.Atoi(idStr)
    if err != nil || id <= 0 {
        httputil.WriteJSONError(w, req, http.StatusBadRequest, errors.ErrBadRequest,
            "Bad Request", "Invalid asset ID", requestID)
        return
    }
    // ... rest of handler
```

with:
```go
func (h *Handler) UpdateAsset(w http.ResponseWriter, req *http.Request) {
    requestID := middleware.GetRequestID(req.Context())
    identifier := chi.URLParam(req, "identifier")
    orgID := middleware.GetOrgID(req.Context())

    asset, err := h.storage.GetAssetByIdentifier(req.Context(), orgID, identifier)
    if err != nil {
        httputil.RespondStorageError(w, req, err, requestID)
        return
    }
    if asset == nil {
        httputil.WriteJSONError(w, req, http.StatusNotFound, errors.ErrNotFound,
            "Not Found", "Asset not found", requestID)
        return
    }
    id := asset.ID
    // ... rest of handler unchanged, uses `id` as before
```

The rest of the handler (decode body, validate, call storage.UpdateAsset(ctx, id, ...)) is unchanged — the lookup produces the same `id` variable the old parse produced.

**Verify the orgID accessor.** If `middleware.GetOrgID` isn't the right call in this codebase, check how `GetAssetByIdentifier` is called from the existing `GetAssetByIdentifier` handler and match that exactly. The scouting report says reads already do this lookup; mimic it.

- [ ] **Step 5: Migrate `DeleteAsset` handler**

Same pattern. Lines ~262. After the lookup, call existing `storage.DeleteAsset(ctx, asset.ID, ...)`.

- [ ] **Step 6: Migrate `AddIdentifier` handler**

Lines ~458. Same parent-lookup pattern, outer param = `{identifier}`. The `identifierId` child param is not used in AddIdentifier (no inner param; body carries the new identifier). Keep the remaining body decode/validate/storage flow unchanged.

- [ ] **Step 7: Migrate `RemoveIdentifier` handler**

Lines ~531. Two params: outer `{identifier}` (parent asset lookup key) and inner `{identifierId}` (child identifier row surrogate). Read outer via `chi.URLParam(req, "identifier")`, resolve via `GetAssetByIdentifier`. Read inner via `chi.URLParam(req, "identifierId")` and keep the existing integer parse. Pass the resolved parent `asset.ID` and parsed `identifierId` to the existing storage call.

**Important:** preserve the existing cross-asset safety check — the storage layer validates that the identifier row's `asset_id` matches the parent `asset.ID`. Do not skip or weaken it.

- [ ] **Step 8: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/handlers/assets/... -v
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/handlers/assets/assets.go backend/internal/handlers/assets/*_test.go
git commit -m "feat(tra-407): assets write/child routes accept {identifier} param"
```

---

### Task 2: Migrate `locations` write, child, and hierarchy routes to `{identifier}`

**Files:**
- Modify: `backend/internal/cmd/serve/router.go` — 7 routes
- Modify: `backend/internal/handlers/locations/locations.go` — 7 handlers
- Modify: `backend/internal/handlers/locations/locations_test.go` and/or existing integration tests

**Routes to flip** (in `router.go` around lines 125–142):

| Before | After |
|---|---|
| `r.Put("/{id}", locationsHandler.Update)` | `r.Put("/{identifier}", locationsHandler.Update)` |
| `r.Delete("/{id}", locationsHandler.Delete)` | `r.Delete("/{identifier}", locationsHandler.Delete)` |
| `r.Post("/{id}/identifiers", locationsHandler.AddIdentifier)` | `r.Post("/{identifier}/identifiers", locationsHandler.AddIdentifier)` |
| `r.Delete("/{id}/identifiers/{identifierId}", locationsHandler.RemoveIdentifier)` | `r.Delete("/{identifier}/identifiers/{identifierId}", locationsHandler.RemoveIdentifier)` |
| `r.Get("/{id}/ancestors", locationsHandler.GetAncestors)` | `r.Get("/{identifier}/ancestors", locationsHandler.GetAncestors)` |
| `r.Get("/{id}/children", locationsHandler.GetChildren)` | `r.Get("/{identifier}/children", locationsHandler.GetChildren)` |
| `r.Get("/{id}/descendants", locationsHandler.GetDescendants)` | `r.Get("/{identifier}/descendants", locationsHandler.GetDescendants)` |

- [ ] **Step 1: Write failing tests**

Mirror Task 1 Step 1 for each of the 7 location handlers. Each gets a `_ByIdentifier_Works` happy path and `_UnknownIdentifier_Returns404` failure path. Reuse the existing locations test harness.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/handlers/locations/...
```
Expected: new tests FAIL. Stale `/{id}`-shape assertions in old tests may also fail — update them.

- [ ] **Step 3: Update `router.go`**

Flip the 7 route patterns per the table above.

- [ ] **Step 4: Migrate `Update` handler (line ~142)**

Pattern identical to Task 1 Step 4, using `storage.GetLocationByIdentifier(ctx, orgID, identifier)`.

- [ ] **Step 5: Migrate `Delete` handler (line ~206)**

Same.

- [ ] **Step 6: Migrate `AddIdentifier` handler (line ~536)**

Same outer-param pattern.

- [ ] **Step 7: Migrate `RemoveIdentifier` handler (line ~609)**

Outer `{identifier}` + inner `{identifierId}` preserved. Mirror Task 1 Step 7 safety check.

- [ ] **Step 8: Migrate hierarchy readers — `GetAncestors`, `GetChildren`, `GetDescendants` (lines ~428, ~498, ~463)**

Each reads `chi.URLParam(req, "id")` and parses it as integer. Replace with `identifier`-based lookup. The hierarchy queries take the location ID and return related rows — no behavior change to the queries, just the input resolution.

- [ ] **Step 9: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/handlers/locations/... -v
```
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/handlers/locations/locations.go backend/internal/handlers/locations/*_test.go
git commit -m "feat(tra-407): locations write/child/hierarchy routes accept {identifier} param"
```

---

### Task 3: Add `@ID` annotations and flip `@Success 202` → `200`/`204`

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` — swaggo comments above each public handler
- Modify: `backend/internal/handlers/locations/locations.go` — same
- Modify: `backend/internal/handlers/assets/bulkimport.go` — skip (internal)

**Scope:** annotation-only changes, no behavior. This is the kind of change a reviewer reads differently from Tasks 1–2.

**Canonical `@ID` table** (resource.verb convention):

| Handler | `@ID` | `@Success` target |
|---|---|---|
| `ListAssets` | `assets.list` | 200 |
| `GetAssetByIdentifier` | `assets.get` | 200 |
| `CreateAsset` | `assets.create` | 201 |
| `UpdateAsset` | `assets.update` | **200** (was 202) |
| `DeleteAsset` | `assets.delete` | **204** (was 202) |
| `AddIdentifier` (assets) | `assets.identifiers.add` | 201 |
| `RemoveIdentifier` (assets) | `assets.identifiers.remove` | **204** (was 202) |
| asset history (`GetAssetHistory`) | `assets.history` | 200 |
| `ListLocations` | `locations.list` | 200 |
| `GetLocationByIdentifier` | `locations.get` | 200 |
| `GetCurrentLocation` | `locations.current` | 200 |
| `CreateLocation` | `locations.create` | 201 |
| `Update` (locations) | `locations.update` | **200** (was 202) |
| `Delete` (locations) | `locations.delete` | **204** (was 202) |
| `AddIdentifier` (locations) | `locations.identifiers.add` | 201 |
| `RemoveIdentifier` (locations) | `locations.identifiers.remove` | **204** (was 202) |
| `GetAncestors` | `locations.ancestors` | **200** (was 202) |
| `GetChildren` | `locations.children` | **200** (was 202) |
| `GetDescendants` | `locations.descendants` | **200** (was 202) |

- [ ] **Step 1: Add `@ID` to every public handler's swaggo block**

For each entry in the table above, open the handler file and add an `@ID <value>` line inside the swaggo comment block. Conventional placement: right after `@Tags` or right before `@Produce`. Example:

```go
// @Summary Update an asset
// @Description ...
// @ID assets.update
// @Tags assets,public
// @Accept json
// @Produce json
// @Param identifier path string true "Asset identifier"
// @Success 200 {object} ...
// ...
```

Confirm uniqueness — all `@ID` values in the table are distinct by construction, but the swaggo generator will fail if it sees collisions. If it does, investigate.

- [ ] **Step 2: Flip `@Success 202` to `200`/`204` where applicable**

Ten annotations change (see the bolded rows in the table):
- `assets.go`: `UpdateAsset` 202→200, `DeleteAsset` 202→204, `RemoveIdentifier` 202→204
- `locations.go`: `Update` 202→200, `Delete` 202→204, `RemoveIdentifier` 202→204, `GetAncestors` 202→200, `GetChildren` 202→200, `GetDescendants` 202→200

**Important:** the handler's actual `w.WriteHeader(...)` call must match the annotation. If the handler currently writes `http.StatusAccepted` (202), change to `http.StatusOK` (200) for updates or `http.StatusNoContent` (204) for deletes. Look at each handler to confirm. The scouting report flagged these as "incorrectly 202" at the annotation level — the runtime behavior may already be 200/204, in which case only the annotation was wrong. Either way, verify and align.

- [ ] **Step 3: Also update `@Param` path declarations from `id` to `identifier`**

Every handler whose URL now takes `{identifier}` must have its `@Param` line renamed. For example, in `UpdateAsset`:

Before:
```go
// @Param id path integer true "Asset ID"
```

After:
```go
// @Param identifier path string true "Asset identifier"
```

Handlers needing this: all 11 migrated in Tasks 1 + 2. Hierarchy reads (Ancestors/Children/Descendants) — same update.

**RemoveIdentifier** has two path params: `@Param identifier path string true "Asset identifier"` (outer) and keep `@Param identifierId path integer true "Identifier row ID"` (inner, unchanged).

- [ ] **Step 4: Build and run existing tests**

Run:
```bash
cd backend && go build ./... && go test ./internal/handlers/...
```
Expected: PASS. No test changes needed — annotation-only change.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go
git commit -m "docs(tra-407): add @ID annotations and correct success codes on public handlers"
```

---

### Task 4: Regenerate OpenAPI spec and verify

**Files:**
- Modify (regen output): `docs/api/openapi.public.json`, `docs/api/openapi.public.yaml`

- [ ] **Step 1: Install pinned swag (once per environment)**

If local swag is not v1.16.6, install it:
```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.6
```
Note: the `--version` output reports `v1.16.4` even for v1.16.6 binaries (upstream hardcoded string not bumped). Use the install command as the source of truth.

- [ ] **Step 2: Regenerate the public spec**

Run:
```bash
cd /home/mike/platform/.worktrees/tra-407-openapi-coherence && just backend api-spec
```

- [ ] **Step 3: Inspect the diff**

```bash
git status
git diff --stat docs/api/
git diff docs/api/openapi.public.yaml | head -200
```

Expected additions:
- `operationId: assets.list`, `operationId: assets.get`, etc. on every public op
- Path keys change from `/api/v1/assets/{id}` to `/api/v1/assets/{identifier}` on 11 routes
- Success response codes flip 202→200/204 on the 10 annotations

If local swag produces output that does not match CI's output format (fully-qualified schema names vs short names, etc.), use the CI artifact approach:
1. Push the branch (step 5 below) to trigger CI
2. CI's `api-spec` check fails on drift, but uploads the correct spec as an artifact
3. Download via `gh run download <run-id> -n openapi-specs -R trakrf/platform`
4. Overwrite local files, commit, and push

- [ ] **Step 4: Run the drift-check equivalent locally**

Run:
```bash
cd backend && go build ./... && go test ./...
```

Full backend tests. Expected: PASS.

- [ ] **Step 5: Commit the regenerated spec**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "docs(tra-407): regenerate openapi.public spec with @ID and {identifier} routes"
```

- [ ] **Step 6: Run `just validate` from repo root**

Run:
```bash
cd /home/mike/platform/.worktrees/tra-407-openapi-coherence && just validate
```
Expected: PASS. Frontend may fail on node_modules-missing in a fresh worktree; if so, run `pnpm install` first and re-run. Only real failures block.

- [ ] **Step 7: Review commit history**

Run:
```bash
git log --oneline main..HEAD
```

Expected 4 commits (1 per task), clean atomic sequence.

---

## Testing strategy recap

**Task 1 and 2** each add happy-path + 404-on-unknown tests per migrated handler. 11 routes × 2 tests = 22 new tests, though the `AddIdentifier` + `RemoveIdentifier` happy paths may overlap with tests added in PR #1 and can be updated rather than duplicated.

**Task 3** is annotation-only — no test changes.

**Task 4** is regen — covered by CI's drift check (`api-spec.yml`), Redocly lint, and our full backend test run.

**Out of scope for tests:**
- No DB migration tests.
- No perf testing of the extra lookup step (it's the same lookup reads already do on every request).
- No load testing of the routing change.

## Spec coverage map

| Design section | Task(s) |
|---|---|
| 6a — `@ID` annotations | 3 |
| 6b — `{identifier}` on write/child/hierarchy | 1, 2 |
| 6c — `202 → 200/204` | 3 |
| Spec regen + commit | 4 |
| Drift verification | 4 |

## Out of scope (reiterated)

- Storage-layer `%w` wrapping on pgx errors (TRA-407 PR #1 carries a follow-up ticket for this; separate PR).
- Internal users/orgs handler migration (separate PR).
- `/api/v1/assets/{identifier}/history` endpoint (already on `{identifier}`, not touched here).
- Bulk operations (`POST /api/v1/assets/bulk-import`) — internal, not public-API scope.

## Open questions (resolve during implementation)

- **orgID accessor.** Task 1 Step 4 uses `middleware.GetOrgID(req.Context())`. Confirm the exact function name/signature by reading how the existing `GetAssetByIdentifier` handler fetches orgID. Adopt whatever the existing pattern is.
- **404 vs 400 for unknown identifier.** The plan specifies 404. If the existing `GetAssetByIdentifier` handler returns a different shape for unknown identifiers, match that for consistency — but flag the deviation.
- **Swag output drift.** If local `swag v1.16.6` produces output that differs from CI's output, fall back to the CI artifact approach in Task 4 Step 3. PR #1 resolved the same drift this way (commit d582f18).
