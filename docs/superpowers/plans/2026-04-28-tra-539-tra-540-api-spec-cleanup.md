# TRA-539 + TRA-540 — API Spec Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the combined v1 OpenAPI hygiene fixes (TRA-539 §2.x) and vocabulary renames (TRA-540 §3.x) as one breaking-change PR, then regenerate the spec and sweep the hand-written frontend API client to consume the new names.

**Architecture:** Backend Go struct tags + handler swagger annotations are the source of truth. The OpenAPI yaml is generated via `just backend api-spec` and committed. Frontend has no codegen — `frontend/src/lib/api/*.ts` is hand-written and updated via TS-compiler-driven sweep. One commit per resource so colliding §2/§3 fields are touched once. Final commits regen the spec and sweep the frontend.

**Tech Stack:** Go 1.x backend (chi router, swag for OpenAPI generation), TypeScript/React frontend (pnpm), TimescaleDB, justfile task runner.

**Spec:** `docs/superpowers/specs/2026-04-28-tra-539-tra-540-api-spec-cleanup-design.md`

---

## Conventions used by every task

- **Run from project root** (`/home/mike/platform/.worktrees/tra-539-540-api-spec-cleanup`). Use `just backend <cmd>` / `just frontend <cmd>` delegation rather than `cd`.
- **Branch:** already on `fix/tra-539-540-api-spec-cleanup`. Do not switch.
- **Per-rename grep discipline:** before claiming any rename complete, grep the entire platform repo for every occurrence of the old identifier (struct field, JSON tag, swagger annotation, test fixture, comment, frontend literal). The "rename sweep enumeration" rule is non-negotiable.
- **Commits:** every task ends with one commit. Use HEREDOC commit messages with `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- **Tests:** each task runs `just backend test` for the affected package(s); the final task runs `just validate` (combined lint + test).
- **Database:** column names stay as they are. Rename happens at the API layer only — `surrogate_id` in the JSON, `id` in the column.

---

## Task 0: Pre-flight baseline

**Files:**
- Read: `backend/justfile`, `frontend/justfile`, root `justfile`

- [ ] **Step 1: Confirm worktree state**

```bash
git rev-parse --abbrev-ref HEAD
# Expected: fix/tra-539-540-api-spec-cleanup

git log --oneline -3
# Expected: top commit is "docs(spec): TRA-539 + TRA-540 combined API spec cleanup design"
```

- [ ] **Step 2: Confirm two openapi.public.yaml files diverge today (baseline)**

```bash
diff -q backend/internal/handlers/swaggerspec/openapi.public.yaml docs/api/openapi.public.yaml
# Expected: "Files ... differ" — they're out of sync at branch creation. Task 17 (spec regen) brings them back in sync.
```

- [ ] **Step 3: Run baseline backend tests**

```bash
just backend test
# Expected: all tests pass. If any fail on origin/main, surface them — do not proceed without flagging.
```

- [ ] **Step 4: Confirm frontend deps installed**

```bash
ls frontend/node_modules > /dev/null && echo "ok"
# Expected: "ok"
```

No commit — pre-flight only.

---

## Task 1: Asset resource (TRA-540 §3.1, §3.3, §3.6 + TRA-539 §2.2, §2.5)

**Goal:** Rename `type` → `asset_type` (with closed enum `[item, person, inventory]` on both request and response), rename `current_location` → `current_location_identifier`, and update spec annotations to reflect omit-when-unset for `description` and `valid_to`.

**Files (likely; verify via grep):**
- `backend/internal/models/asset/public.go` — `PublicAssetView` struct
- `backend/internal/models/asset/asset.go` — internal/storage struct
- `backend/internal/handlers/assets/assets.go` — handler annotations + write-side request/response shapes
- `backend/internal/handlers/assets/*_integration_test.go` — integration tests asserting field names
- `backend/internal/models/asset/public_test.go`
- `backend/internal/storage/assets.go` — projection/fixture helpers
- `backend/internal/storage/assets_integration_test.go`
- Any other test fixture files referencing the old field names

- [ ] **Step 1: Enumerate all old-name occurrences**

```bash
grep -rn '"type"' backend/internal/models/asset backend/internal/handlers/assets 2>/dev/null
grep -rn '"current_location"' backend/internal --include="*.go" 2>/dev/null
grep -rn '\\.Type\\b' backend/internal/models/asset backend/internal/handlers/assets 2>/dev/null | head -30
grep -rn 'CurrentLocation\\b' backend/internal --include="*.go" 2>/dev/null | head -30
```

Save the list. Every occurrence either gets renamed or is intentionally left (e.g., a field on a different struct).

- [ ] **Step 2: Rename the public schema fields**

Edit `backend/internal/models/asset/public.go`:
- `Type string `json:"type,omitempty"`` → `AssetType string `json:"asset_type,omitempty"`` with swag enum tag for `[item, person, inventory]`
- `CurrentLocation *string `json:"current_location"`` → `CurrentLocationIdentifier *string `json:"current_location_identifier"``
- Update `ToPublicAssetView` projection accordingly: `AssetType: a.Type` (or rename internal field too — see step 3)

The struct already has `Description string `json:"description,omitempty"`` and `ValidTo *time.Time `json:"valid_to,omitempty"`` — Go side is already omit-when-unset. Spec annotations are what need updating in step 4.

- [ ] **Step 3: Decide on internal type-name churn**

The internal `Asset` struct in `backend/internal/models/asset/asset.go` uses `.Type` heavily. Renaming the Go field to `AssetType` everywhere is a large blast radius. Two options:

**Option A (recommended):** rename only the public-API field name (`AssetType` on `PublicAssetView`), keep internal `Asset.Type` unchanged. Projection code maps `a.Type` → `view.AssetType`.

**Option B:** rename `Type` → `AssetType` everywhere internal too. Larger churn but more consistent.

Use Option A. Smaller blast radius, the internal name doesn't bleed into the API surface.

- [ ] **Step 4: Update swagger enum annotations on request shapes**

Locate the request-side struct (likely `CreateAssetRequest` / `UpdateAssetRequest` in `handlers/assets/assets.go`):
- Apply swag enum tag for `[item, person, inventory]` on the request `type` field (if it was previously `[asset, person, inventory]`)
- Rename request field from `type` to `asset_type` to match response; update validators

Search for the existing enum tag:
```bash
grep -rn 'enum.*asset.*person\\|enum.*person.*inventory' backend/internal --include="*.go"
```

- [ ] **Step 5: Update integration tests**

In `backend/internal/handlers/assets/*_integration_test.go`:
- Replace `"type":"asset"` with `"asset_type":"item"` in JSON fixtures
- Replace `"type":"person"` / `"type":"inventory"` with `"asset_type":"person"` / `"asset_type":"inventory"`
- Replace `"current_location":` with `"current_location_identifier":` in expected response bodies
- Update Go field accesses: `view.Type` → `view.AssetType`, `view.CurrentLocation` → `view.CurrentLocationIdentifier`

- [ ] **Step 6: Run asset-package tests**

```bash
just backend test ./internal/handlers/assets/... ./internal/models/asset/...
# Expected: PASS. Failures = test fixture not yet updated. Iterate until clean.
```

- [ ] **Step 7: Run full backend test suite**

```bash
just backend test
# Expected: PASS. Catches any test in another package that referenced old asset field names.
```

- [ ] **Step 8: Commit**

```bash
git add backend/
git commit -m "$(cat <<'EOF'
refactor(api): rename asset type/current_location fields (TRA-540 §3.1/§3.3/§3.6)

- PublicAssetView.type → asset_type with enum [item, person, inventory]
  (replaces tautological 'asset' value with neutral 'item'; pairs with
  person/inventory as orthogonal tracking modes)
- PublicAssetView.current_location → current_location_identifier
  (TRA-540 §3.6 standardization on natural-key naming)
- Closed enum applied on both request and response sides (TRA-539 §2.5)
- Description/valid_to spec annotations reflect omit-when-unset (TRA-539 §2.2;
  Go struct tags already correct)

BREAKING: clients must send/receive asset_type instead of type and
current_location_identifier instead of current_location.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Location resource (TRA-540 §3.4, §3.6 + TRA-539 §2.2)

**Goal:** Rename `parent` (response) and `parent_identifier` (request) to `parent_location_identifier`. Rename `storage.SaveInventoryResult.location_id` (int surrogate) → `location_identifier` (natural key). Confirm `valid_to` and `parent_location_identifier` are omit-when-unset.

**Files (likely; verify via grep):**
- `backend/internal/models/location/public.go` — `PublicLocationView`
- `backend/internal/models/location/location.go`
- `backend/internal/handlers/locations/locations.go` — `CreateLocationWithTagsRequest`, `UpdateLocationRequest`
- `backend/internal/handlers/locations/*_integration_test.go`
- `backend/internal/storage/inventory.go` — `SaveInventoryResult.LocationID` (note: confirm this is the int surrogate one)
- `backend/internal/storage/locations.go` and tests

- [ ] **Step 1: Enumerate**

```bash
grep -rn '"parent"' backend/internal/models/location backend/internal/handlers/locations 2>/dev/null
grep -rn '"parent_identifier"' backend/internal --include="*.go" 2>/dev/null
grep -rn 'json:"location_id"' backend/internal/storage/inventory.go backend/internal/handlers/inventory 2>/dev/null
grep -rn '\\.Parent\\b\\|\\.ParentIdentifier\\b' backend/internal --include="*.go" 2>/dev/null | head -30
```

- [ ] **Step 2: Rename `PublicLocationView.parent` → `parent_location_identifier`**

Edit `backend/internal/models/location/public.go`:
- `Parent *string `json:"parent"`` (or whatever the current shape is) → `ParentLocationIdentifier *string `json:"parent_location_identifier,omitempty"``
- Add `omitempty` so the field is omit-when-unset (TRA-539 §2.2)
- Confirm `ValidTo *time.Time `json:"valid_to,omitempty"`` already has omitempty; if not, add it

- [ ] **Step 3: Rename request-side `parent_identifier` → `parent_location_identifier`**

Edit `backend/internal/handlers/locations/locations.go`:
- On `CreateLocationWithTagsRequest`: `ParentIdentifier *string `json:"parent_identifier,omitempty"`` → `ParentLocationIdentifier *string `json:"parent_location_identifier,omitempty"``
- On `UpdateLocationRequest`: same rename
- Update validators that reference the old name

- [ ] **Step 4: Rename `storage.SaveInventoryResult.location_id` → `location_identifier`**

Edit `backend/internal/storage/inventory.go`:
- `LocationID int `json:"location_id"`` (int surrogate) → `LocationIdentifier string `json:"location_identifier"`` (natural key)
- Update the projection/fill logic: instead of returning `location.ID`, return `location.Identifier`
- This is a TYPE CHANGE (int → string). Verify no integer-arithmetic consumers downstream.

- [ ] **Step 5: Update all integration tests**

In `backend/internal/handlers/locations/*_integration_test.go` and `backend/internal/storage/locations_integration_test.go`:
- Replace `"parent":` with `"parent_location_identifier":` in expected response bodies
- Replace `"parent_identifier":` with `"parent_location_identifier":` in request bodies
- Update Go field accesses

In `backend/internal/handlers/inventory/save_test.go` and `backend/internal/handlers/inventory/public_write_integration_test.go`:
- Replace `"location_id":` (numeric) with `"location_identifier":` (string identifier) in expected response bodies for SaveInventoryResult
- Update assertions on the response shape

- [ ] **Step 6: Run package tests**

```bash
just backend test ./internal/handlers/locations/... ./internal/models/location/... ./internal/handlers/inventory/... ./internal/storage/...
# Expected: PASS.
```

- [ ] **Step 7: Run full backend test suite**

```bash
just backend test
# Expected: PASS.
```

- [ ] **Step 8: Commit**

```bash
git add backend/
git commit -m "$(cat <<'EOF'
refactor(api): rename location parent fields, drop int location_id (TRA-540 §3.4/§3.6, TRA-539 §2.2)

- PublicLocationView.parent → parent_location_identifier with omitempty
  (TRA-540 §3.4 + §3.6 merged: read/write parity AND consistent
  *_location_identifier naming across resources)
- CreateLocationWithTagsRequest/UpdateLocationRequest:
  parent_identifier → parent_location_identifier
- storage.SaveInventoryResult.location_id (int surrogate) →
  location_identifier (natural key string) per surrogate-IDs-are-internal
  policy
- valid_to and parent_location_identifier marked omit-when-unset (TRA-539 §2.2)

BREAKING: clients must use parent_location_identifier on both read/write;
SaveInventoryResult shape changes from int location_id to string
location_identifier.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Tag resource and tag-add endpoints (TRA-540 §3.1, §3.5, §2.8 + TRA-539 §2.4)

**Goal:** On `shared.TagIdentifier`: rename `id` → `surrogate_id` and `type` → `tag_type`. Type the tag-add 201 responses as `{"data": shared.TagIdentifier}`. Rename path param `{tagId}` → `{tagSurrogateId}`.

**Files (likely; verify via grep):**
- `backend/internal/models/shared/tag.go` — `TagIdentifier`
- `backend/internal/handlers/assets/assets.go` — tag-add handler for assets
- `backend/internal/handlers/locations/locations.go` — tag-add handler for locations
- `backend/internal/storage/tags.go`
- `backend/internal/handlers/assets/by_id_integration_test.go` — tag delete tests
- All locations referencing `TagIdentifier{Type: ...}` or `tag.ID`

- [ ] **Step 1: Enumerate**

```bash
grep -rn 'TagIdentifier\\b' backend/internal --include="*.go" 2>/dev/null
grep -rn '"tagId"\\|/tags/{tagId}\\|\\.TagID\\|tagID\\b' backend/internal --include="*.go" 2>/dev/null
grep -rn 'ID\\s*int.*json:"id"' backend/internal/models/shared/tag.go 2>/dev/null
```

- [ ] **Step 2: Rename `TagIdentifier.id` → `surrogate_id` and `type` → `tag_type`**

Edit `backend/internal/models/shared/tag.go`:
- `ID int `json:"id"`` → `SurrogateID int `json:"surrogate_id"``
- `Type string `json:"type"`` → `TagType string `json:"tag_type"`` with enum `[rfid, ble, barcode]`
- Update any `TagIdentifier{...}` literals across the repo (struct field rename — Go compiler will surface these)

- [ ] **Step 3: Type the tag-add 201 responses**

Tag-add lives on `POST /api/v1/assets/{identifier}/tags` and `POST /api/v1/locations/{identifier}/tags`. Find the handlers (likely in `handlers/assets/assets.go` and `handlers/locations/locations.go`):
- Confirm the response body is wrapped: `{"data": shared.TagIdentifier}`
- Update swagger response annotations to type the 201 response as `data` envelope wrapping `shared.TagIdentifier` (not `additionalProperties: true, type: object`)

The convention may already exist via a generic `respond.Data(...)` helper — check `backend/internal/respond/` for the response-envelope helper and confirm its swagger type matches.

- [ ] **Step 4: Rename path param `{tagId}` → `{tagSurrogateId}`**

Find the route registration (likely chi `r.Delete("/api/v1/assets/{identifier}/tags/{tagId}", ...)`):
- Rename path param to `{tagSurrogateId}`
- Update the handler that reads `chi.URLParam(r, "tagId")` → `chi.URLParam(r, "tagSurrogateId")`
- Update swagger annotation `@Param tagId path int true ...` → `@Param tagSurrogateId path int true ...`

- [ ] **Step 5: Update integration tests**

- Find DELETE-tag tests: replace URL `/tags/123` patterns are unaffected (numeric param, name doesn't appear in request URL — only in path template). Verify by reading test bodies.
- Find tag-add response assertions: any access to `tag.id` in JSON checks → `tag.surrogate_id`; `tag.type` → `tag.tag_type`
- Replace Go field accesses: `tagIdent.ID` → `tagIdent.SurrogateID`, `tagIdent.Type` → `tagIdent.TagType`

- [ ] **Step 6: Run package tests**

```bash
just backend test ./internal/models/shared/... ./internal/handlers/assets/... ./internal/handlers/locations/... ./internal/storage/...
# Expected: PASS.
```

- [ ] **Step 7: Run full backend test suite**

```bash
just backend test
# Expected: PASS.
```

- [ ] **Step 8: Commit**

```bash
git add backend/
git commit -m "$(cat <<'EOF'
refactor(api): tag schema cleanup — surrogate_id/tag_type/typed 201/{tagSurrogateId} (TRA-540 §3.1/§3.5/§2.8, TRA-539 §2.4)

- shared.TagIdentifier.id → surrogate_id (TRA-540 §3.5)
- shared.TagIdentifier.type → tag_type with enum [rfid, ble, barcode]
  (TRA-540 §3.1 — eliminates type=rfid vs type=item cross-resource collision)
- POST /assets/{identifier}/tags and POST /locations/{identifier}/tags
  201 responses typed as {"data": shared.TagIdentifier} instead of
  additionalProperties:true blob (TRA-539 §2.4)
- DELETE /api/v1/assets/{identifier}/tags/{tagId} path param renamed
  to {tagSurrogateId} (TRA-540 §2.8 — matches response field naming)

BREAKING: tag responses now expose surrogate_id and tag_type;
DELETE path param name changes (URL value unchanged).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: API key resource (TRA-540 §3.5 + TRA-539 §2.1)

**Goal:** Rename `id` → `surrogate_id` on both `APIKeyCreateResponse` and `APIKeyListItem`. Mark `expires_at` as omit-when-unset. Leave `jti` alone (it's a stable UUID, not a surrogate).

**Files:**
- `backend/internal/models/apikey/apikey.go`
- `backend/internal/handlers/orgs/api_keys.go`
- `backend/internal/handlers/orgs/api_keys_integration_test.go`

- [ ] **Step 1: Enumerate**

```bash
grep -rn 'APIKeyCreateResponse\\|APIKeyListItem' backend/internal --include="*.go" 2>/dev/null
grep -n 'json:"id"' backend/internal/models/apikey/apikey.go 2>/dev/null
grep -rn 'json:"expires_at"' backend/internal --include="*.go" 2>/dev/null
```

- [ ] **Step 2: Rename `id` → `surrogate_id`**

Edit `backend/internal/models/apikey/apikey.go` (or wherever the response shapes are defined):
- `APIKeyCreateResponse.ID int `json:"id"`` → `APIKeyCreateResponse.SurrogateID int `json:"surrogate_id"``
- `APIKeyListItem.ID int `json:"id"`` → `APIKeyListItem.SurrogateID int `json:"surrogate_id"``
- Confirm `jti` field (UUID string) stays untouched — different concept

- [ ] **Step 3: Mark `expires_at` as omit-when-unset**

- Confirm Go json tag is `json:"expires_at,omitempty"` and field is `*time.Time` (so nil is omitted). If not pointer, change to `*time.Time` so the field can be missing rather than zero-valued.
- Remove any `nullable: true` swagger annotation on `expires_at`
- Remove `expires_at` from the swag `required` list if listed

- [ ] **Step 4: Update integration tests**

In `backend/internal/handlers/orgs/api_keys_integration_test.go`:
- Replace assertions on `body["id"]` → `body["surrogate_id"]`
- Where `expires_at` was previously expected to be `null`, change assertion to "field absent"
- Where a key with no expiry is created, assert the `expires_at` key is missing (use `_, ok := body["expires_at"]; ok == false`)

- [ ] **Step 5: Run package tests**

```bash
just backend test ./internal/models/apikey/... ./internal/handlers/orgs/...
# Expected: PASS.
```

- [ ] **Step 6: Run full backend test suite**

```bash
just backend test
# Expected: PASS.
```

- [ ] **Step 7: Commit**

```bash
git add backend/
git commit -m "$(cat <<'EOF'
refactor(api): apikey responses use surrogate_id; expires_at omit-when-unset (TRA-540 §3.5, TRA-539 §2.1)

- APIKeyCreateResponse.id → surrogate_id
- APIKeyListItem.id → surrogate_id
- expires_at marked omit-when-unset in spec to match service behavior
  (jti, the stable UUID, is unchanged)

BREAKING: clients must read surrogate_id instead of id on apikey responses;
clients must test for key-presence on expires_at (per docs/api/date-fields).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Report resource (TRA-540 §3.6 + TRA-539 §2.3)

**Goal:** Rename `location` → `location_identifier` on both `PublicAssetHistoryItem` and `PublicCurrentLocationItem`. Mark `asset_deleted_at` as omit-when-unset on `PublicCurrentLocationItem`.

**Files:**
- `backend/internal/models/report/public.go`
- `backend/internal/models/report/public_test.go`
- `backend/internal/handlers/reports/asset_history.go`
- `backend/internal/handlers/reports/current_locations.go`
- Integration tests for reports endpoints

- [ ] **Step 1: Enumerate**

```bash
grep -rn 'PublicAssetHistoryItem\\|PublicCurrentLocationItem' backend/internal --include="*.go" 2>/dev/null
grep -rn 'json:"location"\\|json:"asset_deleted_at"' backend/internal --include="*.go" 2>/dev/null
```

- [ ] **Step 2: Rename `location` → `location_identifier`**

Edit `backend/internal/models/report/public.go`:
- `PublicAssetHistoryItem.Location string `json:"location"`` → `LocationIdentifier string `json:"location_identifier"``
- `PublicCurrentLocationItem.Location string `json:"location"`` → `LocationIdentifier string `json:"location_identifier"``
- Update projections / `To...View` helpers

- [ ] **Step 3: Mark `asset_deleted_at` omit-when-unset**

- `AssetDeletedAt *time.Time `json:"asset_deleted_at,omitempty"`` — confirm pointer + omitempty; add if missing
- Remove `nullable: true` from swag annotation on `asset_deleted_at`
- Remove from `required` list

- [ ] **Step 4: Update tests**

In `backend/internal/models/report/public_test.go` and report integration tests:
- Replace `"location":` with `"location_identifier":`
- Replace `view.Location` → `view.LocationIdentifier`
- Where tests assert `asset_deleted_at: null`, change to assert key absent
- Where tests check soft-deleted asset history (`include_deleted=true`), confirm the field IS present (it's only omitted for live assets)

- [ ] **Step 5: Run package tests**

```bash
just backend test ./internal/models/report/... ./internal/handlers/reports/...
# Expected: PASS.
```

- [ ] **Step 6: Run full backend test suite**

```bash
just backend test
# Expected: PASS.
```

- [ ] **Step 7: Commit**

```bash
git add backend/
git commit -m "$(cat <<'EOF'
refactor(api): report shapes — location → location_identifier, asset_deleted_at omit-when-unset (TRA-540 §3.6, TRA-539 §2.3)

- PublicAssetHistoryItem.location → location_identifier (TRA-540 §3.6)
- PublicCurrentLocationItem.location → location_identifier (TRA-540 §3.6)
- PublicCurrentLocationItem.asset_deleted_at marked omit-when-unset
  (TRA-539 §2.3 — service already omits for live assets unless
  include_deleted=true)

BREAKING: clients reading report endpoints must consume location_identifier.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Scans route + orgs surrogate_id + security block (TRA-540 §3.2, §3.5 + TRA-539 §2.6)

**Goal:** Move `POST /api/v1/inventory/save` to `POST /api/v1/scans`. Rename `orgs.OrgMeView.id` → `surrogate_id`. Add top-level `security: [{ APIKey: [] }]` to the OpenAPI document.

**Files:**
- `backend/internal/handlers/inventory/save.go` — handler + swagger annotations
- `backend/internal/handlers/inventory/save_test.go`
- `backend/internal/handlers/inventory/public_write_integration_test.go`
- Wherever `RegisterRoutes` for the inventory handler is wired (likely `backend/cmd/...` or a router setup file)
- `backend/internal/handlers/orgs/public.go` — `OrgMeView`
- `backend/internal/handlers/orgs/orgs_integration_test.go` (if exists)
- `backend/internal/tools/apispec/postprocess.go` — possibly where the top-level `security:` block is injected

- [ ] **Step 1: Enumerate**

```bash
grep -rn '/api/v1/inventory/save\\|/inventory/save' backend/internal --include="*.go" --include="*.md" 2>/dev/null
grep -rn 'OrgMeView' backend/internal --include="*.go" 2>/dev/null
grep -rn 'securitySchemes\\|security:' backend/internal/tools/apispec/ 2>/dev/null
```

- [ ] **Step 2: Move route from `/api/v1/inventory/save` to `/api/v1/scans`**

In `backend/internal/handlers/inventory/save.go`:
- Update swagger annotation: `// @Router /api/v1/inventory/save [post]` → `// @Router /api/v1/scans [post]`
- Update doc comments referencing the old path
- Leave the handler function name as-is (`Save`) — internal naming, no external impact. Optionally rename to `Scan` if the package is renamed; weigh churn vs clarity.

In the route registration site (find via `grep -rn 'handlers/inventory\\|inventory.Handler\\|inventoryHandler' backend/cmd backend/internal`):
- Update `r.With(middleware.RequireScope("scans:write")).Post("/api/v1/inventory/save", ...)` → `Post("/api/v1/scans", ...)`

In `backend/internal/handlers/inventory/public_write_integration_test.go` and `save_test.go`:
- Replace every `"/api/v1/inventory/save"` URL with `"/api/v1/scans"`
- Update test names / comments referencing the old path

The handler package can stay named `inventory` for now (renaming the directory adds churn) — note in the commit message that a follow-on may rename the package to `scans` for full consistency.

- [ ] **Step 3: Rename `orgs.OrgMeView.id` → `surrogate_id`**

Edit `backend/internal/handlers/orgs/public.go`:
- `OrgMeView.ID int `json:"id"`` → `OrgMeView.SurrogateID int `json:"surrogate_id"``
- Update integration tests asserting `body["id"]` → `body["surrogate_id"]`

- [ ] **Step 4: Add top-level `security` block to the OpenAPI document**

The spec is generated via `swag`. The top-level security block can be injected via either:
1. `backend/cmd/<server>/main.go` swag annotation (`// @security APIKey`) — applies globally
2. `backend/internal/tools/apispec/postprocess.go` — postprocess injection

Check `postprocess.go` first — TRA-505 already establishes a postprocess pattern. Add a step that injects:
```yaml
security:
  - APIKey: []
```
At the document root level (sibling of `paths`, `components`, `info`).

If postprocess is the right home, add a unit test in `postprocess_test.go` confirming the block is present after postprocess. If main.go's swag annotation is the right home, regenerate with `just backend api-spec` and confirm the block appears.

- [ ] **Step 5: Run package tests + apispec test**

```bash
just backend test ./internal/handlers/inventory/... ./internal/handlers/orgs/... ./internal/tools/apispec/...
# Expected: PASS.
```

- [ ] **Step 6: Run full backend test suite**

```bash
just backend test
# Expected: PASS.
```

- [ ] **Step 7: Commit**

```bash
git add backend/
git commit -m "$(cat <<'EOF'
refactor(api): /inventory/save → /scans, orgs surrogate_id, top-level security (TRA-540 §3.2/§3.5, TRA-539 §2.6)

- POST /api/v1/inventory/save → POST /api/v1/scans (TRA-540 §3.2;
  aligns with scans:write scope, eliminates models.Inventory /
  inventoryClient.save() SDK ambiguity with the inventory asset type)
- OrgMeView.id → surrogate_id (TRA-540 §3.5)
- Top-level security: [{ APIKey: [] }] declared (TRA-539 §2.6 —
  generated clients now ship every call authenticated by default)

BREAKING: clients must POST to /scans instead of /inventory/save;
OrgMeView.id renamed to surrogate_id.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Spec regen and sync

**Goal:** Regenerate `backend/internal/handlers/swaggerspec/openapi.public.{yaml,json}` from the updated annotations, sync the `docs/api/openapi.public.{yaml,json}` copies, and confirm both copies are byte-identical.

**Files:**
- `backend/internal/handlers/swaggerspec/openapi.public.yaml` — regenerated
- `backend/internal/handlers/swaggerspec/openapi.public.json` — regenerated
- `docs/api/openapi.public.yaml` — synced from regen
- `docs/api/openapi.public.json` — synced from regen

- [ ] **Step 1: Run the spec generator**

```bash
just backend api-spec
# Expected: regenerates backend/internal/handlers/swaggerspec/openapi.public.{yaml,json}
```

- [ ] **Step 2: Sync `docs/api/` copies**

Confirm whether the justfile already syncs `docs/api/openapi.public.{yaml,json}` from the backend output, or whether this is a manual step. Search:

```bash
grep -A 10 "api-spec" justfile docs/justfile 2>/dev/null
grep -rn "openapi.public" justfile **/justfile 2>/dev/null
```

If not auto-synced, copy:

```bash
cp backend/internal/handlers/swaggerspec/openapi.public.yaml docs/api/openapi.public.yaml
cp backend/internal/handlers/swaggerspec/openapi.public.json docs/api/openapi.public.json
```

- [ ] **Step 3: Confirm both yaml files are byte-identical**

```bash
diff -q backend/internal/handlers/swaggerspec/openapi.public.yaml docs/api/openapi.public.yaml
# Expected: no output (files identical)

diff -q backend/internal/handlers/swaggerspec/openapi.public.json docs/api/openapi.public.json
# Expected: no output
```

- [ ] **Step 4: Eyeball-diff against the rename catalog**

Skim the diff of the regenerated yaml vs origin/main to spot-check key renames:

```bash
git diff origin/main -- backend/internal/handlers/swaggerspec/openapi.public.yaml | grep -E '^[+-].*(asset_type|tag_type|surrogate_id|location_identifier|/api/v1/scans|security:)' | head -40
```

Expected: appearances of `asset_type`, `tag_type`, `surrogate_id`, `*_location_identifier`, `/api/v1/scans`, and `security:` block. Disappearances of `"type"` (on response shapes), `"current_location"`, `"parent"`, `"location_id"`, `/inventory/save`.

- [ ] **Step 5: Confirm top-level security block is present**

```bash
grep -B 1 -A 3 '^security:' backend/internal/handlers/swaggerspec/openapi.public.yaml
# Expected:
# security:
#   - APIKey: []
```

- [ ] **Step 6: Run apispec postprocess test if any was added in Task 6**

```bash
just backend test ./internal/tools/apispec/...
# Expected: PASS.
```

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handlers/swaggerspec/ docs/api/
git commit -m "$(cat <<'EOF'
chore(api): regenerate OpenAPI spec for TRA-539+TRA-540 renames

Regenerates the backend-served and docs-served OpenAPI yaml/json after
the per-resource rename and hygiene commits in this branch. Brings the
two spec files (which had drifted ~6KB at branch creation) back into
byte-identical sync.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Frontend sweep — types and API modules

**Goal:** Update hand-written TS types in `frontend/src/types/*.ts` and `frontend/src/lib/api/*.ts` to consume the new field names and route. Let TS compiler enumerate consumer breakage.

**Files (likely; verify via grep):**
- `frontend/src/types/apiKey.ts`
- `frontend/src/lib/api/apiKeys.ts`
- `frontend/src/lib/api/orgs.ts`
- `frontend/src/lib/api/inventory.ts` — likely renames to `scans.ts`
- `frontend/src/lib/api/locations/index.ts`
- `frontend/src/lib/api/reports/index.ts`
- Wherever asset and tag types live (search)
- All `*.test.ts`/`*.test.tsx` mock fixtures

- [ ] **Step 1: Enumerate frontend usages**

```bash
grep -rn '"type"\\|"current_location"\\|"parent"\\|"parent_identifier"\\|"location_id"\\|"location"\\b' frontend/src --include="*.ts" --include="*.tsx" 2>/dev/null | head -50
grep -rn 'asset_type\\|tag_type\\|surrogate_id\\|location_identifier\\|parent_location_identifier' frontend/src --include="*.ts" --include="*.tsx" 2>/dev/null | head -30
grep -rn '/api/v1/inventory/save\\|inventory/save' frontend/src --include="*.ts" --include="*.tsx" 2>/dev/null
grep -rn '"id"\\b' frontend/src/lib/api frontend/src/types --include="*.ts" 2>/dev/null | head -30
```

- [ ] **Step 2: Update TS type definitions**

For each TS interface/type that mirrors a renamed Go shape:
- `Asset.type` → `Asset.asset_type`; constrain to `'item' | 'person' | 'inventory'`
- `Asset.current_location` → `Asset.current_location_identifier`
- `Location.parent` → `Location.parent_location_identifier`
- `Location.parent_identifier` (request types) → `Location.parent_location_identifier`
- `Tag.id` → `Tag.surrogate_id`
- `Tag.type` → `Tag.tag_type`
- `APIKey.id` → `APIKey.surrogate_id`
- `Org.id` → `Org.surrogate_id`
- `Report.location` → `Report.location_identifier`
- `SaveInventoryResult.location_id` (number) → `SaveInventoryResult.location_identifier` (string)
- `expires_at?: string` (already optional, just confirm)
- `description?`, `valid_to?`, `asset_deleted_at?` confirmed optional
- `parent_location_identifier?` confirmed optional

Tests that referenced the old enum value `'asset'` change to `'item'`.

- [ ] **Step 3: Update API modules — endpoint URL**

Edit `frontend/src/lib/api/inventory.ts` (or whatever module wraps the scan-save call):
- Change POST URL from `/api/v1/inventory/save` to `/api/v1/scans`
- Optionally rename file to `scans.ts` and update imports across the frontend to match. Use the TS compiler to find imports.

- [ ] **Step 4: Update API module field references**

For each module, walk through:
- Request bodies that built `{ type, current_location, parent_identifier, location_id }` — rename keys to new names
- Response handlers that read `response.id` for tags/orgs/apikeys → `response.surrogate_id`
- Any `body.type` for tag responses → `body.tag_type`; for asset responses → `body.asset_type`
- Tag-add response unwrapping: if previously typed as `Map<string, unknown>`, retype to `{ data: Tag }`

- [ ] **Step 5: Type-check and lint**

```bash
just frontend lint
# Expected: PASS. TS compiler will surface any consumer code still using old names.
```

- [ ] **Step 6: Walk and fix every TS error**

For each error, navigate to the call site and update the field/key reference. Common patterns:
- Component destructuring: `const { type, current_location } = asset` → `const { asset_type, current_location_identifier } = asset`
- Test fixtures: `{ id: 1, type: "rfid" }` → `{ surrogate_id: 1, tag_type: "rfid" }`
- Mock factories in `*.test.ts`/`*.test.tsx`

- [ ] **Step 7: Run frontend tests**

```bash
just frontend test
# Expected: PASS.
```

- [ ] **Step 8: Commit**

```bash
git add frontend/
git commit -m "$(cat <<'EOF'
refactor(frontend): consume renamed API fields and /scans route (TRA-539, TRA-540)

Sweeps frontend/src/lib/api and frontend/src/types to consume the new
v1 API vocabulary:
- asset_type, tag_type, surrogate_id, *_location_identifier,
  parent_location_identifier
- /api/v1/scans endpoint (renamed from /api/v1/inventory/save)
- Tag-add response typed as { data: Tag } envelope
- expires_at, description, valid_to, asset_deleted_at,
  parent_location_identifier confirmed optional in TS types

No production behavior change beyond the field/URL renames; TS lint
and tests pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Final verification and push

**Goal:** Run combined validation, confirm clean, push the branch, and surface the BB-style re-run + generated-client smoke checks for the user to perform against the preview deployment after PR open.

- [ ] **Step 1: Run combined validate**

```bash
just validate
# Expected: backend lint + tests PASS, frontend lint + tests PASS.
```

- [ ] **Step 2: Confirm spec yaml drift remains zero**

```bash
diff -q backend/internal/handlers/swaggerspec/openapi.public.yaml docs/api/openapi.public.yaml
# Expected: no output (files identical).
```

- [ ] **Step 3: Confirm no stragglers — final repo-wide grep**

```bash
# These old names should appear ZERO times in committed code (excepting
# the spec doc itself, the design plan, and CHANGELOG/docs intentionally
# documenting old vs new vocabulary):
grep -rn '"type"' backend/internal/models backend/internal/handlers --include="*.go" 2>/dev/null | grep -v "_test.go"
grep -rn '"current_location"' backend/internal frontend/src --include="*.go" --include="*.ts" --include="*.tsx" 2>/dev/null
grep -rn '/api/v1/inventory/save' backend/internal frontend/src --include="*.go" --include="*.ts" --include="*.tsx" 2>/dev/null
grep -rn 'json:"id"' backend/internal/models/apikey backend/internal/models/shared backend/internal/handlers/orgs --include="*.go" 2>/dev/null
# Each should return zero hits in production code paths. Test fixtures may
# legitimately reference some old names if asserting backward-compat behavior;
# review any hits surfaced.
```

- [ ] **Step 4: Push the branch**

```bash
git push -u origin fix/tra-539-540-api-spec-cleanup
# Expected: branch pushed, preview deployment kicks off via .github/workflows/sync-preview.yml
```

- [ ] **Step 5: Surface BB-run and generated-client smoke as user tasks**

The PR is ready for verification. Post-push, the following acceptance bars require running against `app.preview.trakrf.id` once the preview deployment completes — these are user-driven, not part of subagent build:

1. **BB-style re-run** of §2.x and §3.x reproduction calls from BB12 FINDINGS.md → expect zero §2.x findings, zero §3.x findings, zero §2.8 finding.
2. **Generated-client smoke** — `openapi-generator-cli generate -i https://app.preview.trakrf.id/redocusaurus/trakrf-api.yaml -g typescript-fetch -o /tmp/trakrf-client && cd /tmp/trakrf-client && tsc --noEmit` — confirm clean compile.
3. **Frontend browser smoke** — local dev server: inventory/scan submit, asset CRUD, apikey create/list, location create-with-tags, reports view.

These are gating for merge. Do not merge until all three are clean.

- [ ] **Step 6: Open the PR (user-driven)**

The user will open the PR once verification is clean. Suggested PR title: `fix: combined API spec hygiene + vocabulary cleanup (TRA-539 + TRA-540)`. Draft body should include the rename catalog and acceptance checklist from the spec doc.

---

## Dependencies and parallelization

Tasks 1–6 (per-resource backend changes) are sequential — they share a global test suite that must pass at every commit. Each task's tests pass before the next starts.

Task 7 (spec regen) depends on all of Tasks 1–6.

Task 8 (frontend sweep) depends on Task 7 (the spec is the contract; frontend mirrors it).

Task 9 (verification + push) depends on Task 8.

**Subagent execution:** dispatch one subagent per task in sequence. Each subagent inherits the worktree state from the previous commit. The reviewing main agent verifies the commit landed cleanly (matching the task's acceptance) before dispatching the next.

## Out-of-band considerations

- **Trakrf-docs:** the customer-facing docs at `/home/mike/trakrf-docs` are NOT touched in this PR. After this PR merges and the BB-style re-run is clean, a separate trakrf-docs PR follows from a separate trakrf-docs checkout — not from this worktree.
- **Linear:** mark TRA-539 and TRA-540 In Review when the PR opens; mark Done when merged.
- **Catalog addendums:** the plan extended TRA-540's `parent_identifier` → `parent_location_identifier` rename to align with §3.6. This is documented in the spec doc; if the BB re-run surfaces any new mismatch, record it as an addendum on the relevant ticket before silently fixing.

## Spec-coverage check

| Spec requirement | Implementing task |
| --- | --- |
| TRA-540 §3.1 (`type` collision) | Tasks 1, 3 |
| TRA-540 §3.2 (`/inventory/save` → `/scans`) | Task 6 |
| TRA-540 §3.3 (`asset` enum tautology → `item`) | Task 1 |
| TRA-540 §3.4 (`parent` read/write parity) | Task 2 |
| TRA-540 §3.5 (`id` polysemy → `surrogate_id`) | Tasks 3, 4, 6 |
| TRA-540 §3.6 (Location reference standardization) | Tasks 1, 2, 5 |
| TRA-540 §2.8 (path param `tagId` → `tagSurrogateId`) | Task 3 |
| TRA-539 §2.1 (`expires_at` omit-when-unset) | Task 4 |
| TRA-539 §2.2 (`description`, `valid_to`, `parent` omit-when-unset) | Tasks 1, 2 |
| TRA-539 §2.3 (`asset_deleted_at` omit-when-unset) | Task 5 |
| TRA-539 §2.4 (typed tag-add 201 response) | Task 3 |
| TRA-539 §2.5 (response-side enum on `asset_type`) | Task 1 |
| TRA-539 §2.6 (top-level `security` block) | Task 6 |
| Spec regen + sync | Task 7 |
| Frontend sweep | Task 8 |
| Combined validation | Task 9 |
| BB re-run, generated-client smoke, browser smoke | Task 9 (user-driven) |

Every spec acceptance item has a task. No gaps.
