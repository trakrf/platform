# TRA-429 — Unify Asset/Location Write-Handler Responses Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `POST /api/v1/assets`, `PUT /api/v1/assets/{identifier}`, `PUT /api/v1/assets/by-id/{id}`, and the location-write twins return `PublicAssetView` / `PublicLocationView` — the same shape already emitted by the read endpoints. Remove the `id` / `org_id` leakage and project via the natural-key-resolving helpers. Update the frontend to normalize the response at the API boundary so `.id` continues to resolve.

**Architecture:** Widen four storage methods (`CreateAssetWithIdentifiers`, `UpdateAsset`, `CreateLocationWithIdentifiers`, `UpdateLocation`) to return the rich view types (`*asset.AssetWithLocation` / `*location.LocationWithParent`). Add internal `getAssetWithLocationByID` / `getLocationWithParentByID` helpers that perform the LEFT-JOIN against the parent and fetch identifiers. Unify the create path by deleting the `createAssetWithoutIdentifiers` / `createLocationWithoutIdentifiers` handler helpers — always call `storage.CreateAssetWithIdentifiers` / `storage.CreateLocationWithIdentifiers`. Handlers project through `ToPublicAssetView` / `ToPublicLocationView`. Frontend: introduce `normalizeLocation` (mirror of `normalizeAsset`); in both form modals, normalize once immediately after the API call and flow the normalized object through validation, cache writes, and toast messages.

**Tech Stack:** Go 1.21+ with pgx/v5, chi router, swag for OpenAPI; React 18 + TypeScript + Zustand + Vitest on the frontend. Root-level `just backend <cmd>` / `just frontend <cmd>` task runner.

**Spec:** `docs/superpowers/specs/2026-04-21-tra429-unify-write-handler-responses-design.md`

**Branch:** `miks2u/tra-429-unify-write-handler-responses` (already checked out; spec commit is HEAD).

**File map:**

*Backend — create / modify:*
- Modify `backend/internal/storage/assets.go` — widen `CreateAssetWithIdentifiers` / `UpdateAsset` signatures, add internal `getAssetWithLocationByID`.
- Modify `backend/internal/storage/locations.go` — widen `CreateLocationWithIdentifiers` / `UpdateLocation` signatures, add internal `getLocationWithParentByID`.
- Modify `backend/internal/storage/assets_test.go` — assertion tweaks + new parent-identifier coverage.
- Modify `backend/internal/storage/locations_test.go` — same.
- Modify `backend/internal/handlers/assets/assets.go` — remove `createAssetWithoutIdentifiers`, simplify `Create`, project in `doUpdateAsset`, add typed response structs, update Swagger.
- Modify `backend/internal/handlers/locations/locations.go` — remove `createLocationWithoutIdentifiers`, simplify `Create`, project in `doUpdate`, add typed response structs, update Swagger.
- Modify `backend/internal/handlers/assets/assets_integration_test.go` + `backend/internal/handlers/assets/by_id_integration_test.go` — shape updates + negative regression.
- Modify `backend/internal/handlers/locations/locations_integration_test.go` + `backend/internal/handlers/locations/by_id_integration_test.go` (create if missing) — shape updates + negative regression.
- Modify `docs/api/openapi.public.json`, `docs/api/openapi.public.yaml` — regenerated.

*Frontend — create / modify:*
- Create `frontend/src/lib/location/normalize.ts`.
- Create `frontend/src/lib/location/normalize.test.ts`.
- Modify `frontend/src/components/assets/AssetFormModal.tsx`.
- Modify `frontend/src/components/locations/LocationFormModal.tsx`.
- Modify `frontend/src/hooks/locations/useLocationMutations.ts` and any other seam discovered in the audit step.

---

## Task 1: Storage helper `getAssetWithLocationByID`

**Files:**
- Modify: `backend/internal/storage/assets.go` (add private helper near line 461, i.e. near `GetAssetViewByID`)
- Test: `backend/internal/storage/assets_integration_test.go` (add new test)

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/storage/assets_integration_test.go`:

```go
// TestGetAssetWithLocationByID_ResolvesParent verifies that the private
// helper returns AssetWithLocation with CurrentLocationIdentifier populated
// when the asset has a live parent location, and nil when unset.
// Guards against regression to the bare Asset/AssetView shape on write paths.
func TestGetAssetWithLocationByID_ResolvesParent(t *testing.T) {
	store := setupTestStorageWithFixtures(t)
	orgID := 1

	// fixture "widget-42" points at location "wh-1" — see fixtures.
	view, err := store.GetAssetByIdentifier(context.Background(), orgID, "widget-42")
	if err != nil || view == nil {
		t.Fatalf("precondition: GetAssetByIdentifier(widget-42) returned (%v, %v)", view, err)
	}

	got, err := store.GetAssetWithLocationByIDForTest(context.Background(), view.ID)
	if err != nil {
		t.Fatalf("GetAssetWithLocationByIDForTest: %v", err)
	}
	if got == nil {
		t.Fatal("want non-nil AssetWithLocation, got nil")
	}
	if got.CurrentLocationIdentifier == nil {
		t.Fatal("want CurrentLocationIdentifier non-nil, got nil")
	}
	if *got.CurrentLocationIdentifier == "" {
		t.Fatal("want CurrentLocationIdentifier non-empty")
	}
	if got.Identifier != "widget-42" {
		t.Errorf("want Identifier=widget-42, got %q", got.Identifier)
	}

	// Negative: an asset with no current_location_id should produce a nil
	// identifier pointer — use a fixture asset known to have null FK, else
	// create one inline.
	unplaced, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID:      orgID,
		Identifier: "tra429-unplaced",
		Name:       "unplaced",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}

	got2, err := store.GetAssetWithLocationByIDForTest(context.Background(), unplaced.ID)
	if err != nil {
		t.Fatalf("GetAssetWithLocationByIDForTest (no-parent): %v", err)
	}
	if got2 == nil {
		t.Fatal("want non-nil AssetWithLocation on no-parent asset, got nil")
	}
	if got2.CurrentLocationIdentifier != nil {
		t.Errorf("want nil CurrentLocationIdentifier, got %q", *got2.CurrentLocationIdentifier)
	}
}
```

Also, since the helper is unexported, add a test-only export shim at the bottom of `backend/internal/storage/assets.go`:

```go
// GetAssetWithLocationByIDForTest exposes getAssetWithLocationByID to integration
// tests in the same package. Production code must use GetAssetByIdentifier or
// the CreateAssetWithIdentifiers / UpdateAsset return values.
func (s *Storage) GetAssetWithLocationByIDForTest(ctx context.Context, id int) (*asset.AssetWithLocation, error) {
	return s.getAssetWithLocationByID(ctx, id)
}
```

*(Tests in the same package can call an unexported function directly, so the shim is only needed if any test lives in a `storage_test` package. Check test file's `package` declaration — if `storage` (not `storage_test`), drop the shim and call `store.getAssetWithLocationByID(...)` directly.)*

Ensure `time` and `asset` are imported in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run from project root:
```bash
just backend test ./internal/storage/... -run TestGetAssetWithLocationByID_ResolvesParent
```
Expected: FAIL — `getAssetWithLocationByID` does not exist yet (compile error).

- [ ] **Step 3: Add the helper**

In `backend/internal/storage/assets.go`, add immediately below `GetAssetViewByID` (after line ~479):

```go
// getAssetWithLocationByID returns an AssetWithLocation by surrogate id,
// performing the LEFT JOIN on parent location and fetching identifiers.
// Used by CreateAssetWithIdentifiers and UpdateAsset to emit the public
// write-response shape. Returns (nil, nil) if the asset doesn't exist
// or is soft-deleted.
func (s *Storage) getAssetWithLocationByID(ctx context.Context, id int) (*asset.AssetWithLocation, error) {
	query := `
		SELECT
			a.id, a.org_id, a.identifier, a.name, a.type, a.description,
			a.current_location_id, a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.identifier
		FROM trakrf.assets a
		LEFT JOIN trakrf.locations l ON l.id = a.current_location_id AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE a.id = $1 AND a.deleted_at IS NULL
		LIMIT 1
	`
	var (
		a      asset.Asset
		locIdt *string
	)
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.OrgID, &a.Identifier, &a.Name, &a.Type, &a.Description,
		&a.CurrentLocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata,
		&a.IsActive, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		&locIdt,
	)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset with location by id: %w", err)
	}

	identifiers, err := s.GetIdentifiersByAssetID(ctx, a.ID)
	if err != nil {
		return nil, err
	}

	return &asset.AssetWithLocation{
		AssetView: asset.AssetView{
			Asset:       a,
			Identifiers: identifiers,
		},
		CurrentLocationIdentifier: locIdt,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
just backend test ./internal/storage/... -run TestGetAssetWithLocationByID_ResolvesParent
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/assets.go backend/internal/storage/assets_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tra-429): add getAssetWithLocationByID storage helper

Internal helper that mirrors GetAssetByIdentifier but keyed on surrogate id.
Used in subsequent commits by CreateAssetWithIdentifiers / UpdateAsset to
return the rich AssetWithLocation shape needed for PublicAssetView projection.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Storage helper `getLocationWithParentByID`

**Files:**
- Modify: `backend/internal/storage/locations.go` (add private helper near line 473, i.e. near `GetLocationViewByID`)
- Test: `backend/internal/storage/locations_integration_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/storage/locations_integration_test.go`:

```go
// TestGetLocationWithParentByID_ResolvesParent verifies that the private
// helper returns LocationWithParent with ParentIdentifier populated when
// the location has a live parent, and nil for a root-level location.
func TestGetLocationWithParentByID_ResolvesParent(t *testing.T) {
	store := setupTestStorageWithFixtures(t)
	orgID := 1

	// fixture "wh-1.bay-3" has parent "wh-1".
	child, err := store.GetLocationByIdentifier(context.Background(), orgID, "wh-1.bay-3")
	if err != nil || child == nil {
		t.Fatalf("precondition: GetLocationByIdentifier(wh-1.bay-3) returned (%v, %v)", child, err)
	}

	got, err := store.GetLocationWithParentByIDForTest(context.Background(), child.ID)
	if err != nil {
		t.Fatalf("GetLocationWithParentByIDForTest: %v", err)
	}
	if got == nil {
		t.Fatal("want non-nil LocationWithParent, got nil")
	}
	if got.ParentIdentifier == nil {
		t.Fatal("want ParentIdentifier non-nil on child location, got nil")
	}
	if *got.ParentIdentifier != "wh-1" {
		t.Errorf("want ParentIdentifier=wh-1, got %q", *got.ParentIdentifier)
	}

	// Root-level fixture "wh-1".
	root, err := store.GetLocationByIdentifier(context.Background(), orgID, "wh-1")
	if err != nil || root == nil {
		t.Fatalf("precondition: GetLocationByIdentifier(wh-1) returned (%v, %v)", root, err)
	}

	gotRoot, err := store.GetLocationWithParentByIDForTest(context.Background(), root.ID)
	if err != nil {
		t.Fatalf("GetLocationWithParentByIDForTest (root): %v", err)
	}
	if gotRoot == nil {
		t.Fatal("want non-nil LocationWithParent on root, got nil")
	}
	if gotRoot.ParentIdentifier != nil {
		t.Errorf("want nil ParentIdentifier on root, got %q", *gotRoot.ParentIdentifier)
	}
}
```

Add the test-only export shim at the bottom of `backend/internal/storage/locations.go`:

```go
// GetLocationWithParentByIDForTest exposes getLocationWithParentByID to
// integration tests in the same package.
func (s *Storage) GetLocationWithParentByIDForTest(ctx context.Context, id int) (*location.LocationWithParent, error) {
	return s.getLocationWithParentByID(ctx, id)
}
```

*(Same caveat as Task 1 — if the test file is in package `storage`, the shim is unnecessary.)*

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test ./internal/storage/... -run TestGetLocationWithParentByID_ResolvesParent
```
Expected: FAIL — `getLocationWithParentByID` does not exist.

- [ ] **Step 3: Add the helper**

In `backend/internal/storage/locations.go`, add directly below `GetLocationViewByID`:

```go
// getLocationWithParentByID returns a LocationWithParent by surrogate id,
// performing the self-join on parent location and fetching identifiers.
// Used by CreateLocationWithIdentifiers and UpdateLocation to emit the
// public write-response shape. Returns (nil, nil) if the location doesn't
// exist or is soft-deleted.
func (s *Storage) getLocationWithParentByID(ctx context.Context, id int) (*location.LocationWithParent, error) {
	query := `
		SELECT
			l.id, l.org_id, l.name, l.identifier, l.parent_location_id,
			l.path, l.depth, l.description, l.valid_from, l.valid_to,
			l.is_active, l.created_at, l.updated_at, l.deleted_at,
			p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.id = $1 AND l.deleted_at IS NULL
		LIMIT 1
	`
	var (
		loc    location.Location
		parIdt *string
	)
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&loc.ID, &loc.OrgID, &loc.Name, &loc.Identifier, &loc.ParentLocationID,
		&loc.Path, &loc.Depth, &loc.Description, &loc.ValidFrom, &loc.ValidTo,
		&loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
		&parIdt,
	)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get location with parent by id: %w", err)
	}

	identifiers, err := s.GetIdentifiersByLocationID(ctx, loc.ID)
	if err != nil {
		return nil, err
	}

	return &location.LocationWithParent{
		LocationView: location.LocationView{
			Location:    loc,
			Identifiers: identifiers,
		},
		ParentIdentifier: parIdt,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test ./internal/storage/... -run TestGetLocationWithParentByID_ResolvesParent
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/locations.go backend/internal/storage/locations_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tra-429): add getLocationWithParentByID storage helper

Internal helper that mirrors GetLocationByIdentifier but keyed on surrogate id.
Used in subsequent commits by CreateLocationWithIdentifiers / UpdateLocation
to return the rich LocationWithParent shape for PublicLocationView projection.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Storage + handler: asset write path returns AssetWithLocation

This task changes four files together because the storage signature change breaks the handler until updated. Unifies the create path (deletes `createAssetWithoutIdentifiers`).

**Files:**
- Modify: `backend/internal/storage/assets.go`
- Modify: `backend/internal/handlers/assets/assets.go`
- Modify: `backend/internal/storage/assets_test.go` (signature updates)
- Add test: `backend/internal/storage/assets_integration_test.go` (parent-identifier coverage on UpdateAsset)

- [ ] **Step 1: Write the failing test for UpdateAsset parent-identifier population**

Append to `backend/internal/storage/assets_integration_test.go`:

```go
// TestUpdateAsset_PopulatesCurrentLocationIdentifier verifies that an
// UpdateAsset call returns the AssetWithLocation shape with the parent
// location's natural key resolved — the contract write-handlers depend on
// to emit PublicAssetView (TRA-429).
func TestUpdateAsset_PopulatesCurrentLocationIdentifier(t *testing.T) {
	store := setupTestStorageWithFixtures(t)
	orgID := 1

	// Precondition: widget-42 points at wh-1 via fixtures.
	base, err := store.GetAssetByIdentifier(context.Background(), orgID, "widget-42")
	if err != nil || base == nil {
		t.Fatalf("precondition: GetAssetByIdentifier(widget-42) returned (%v, %v)", base, err)
	}

	newName := "updated for tra-429"
	result, err := store.UpdateAsset(context.Background(), orgID, base.ID, asset.UpdateAssetRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	if result == nil {
		t.Fatal("want non-nil AssetWithLocation, got nil")
	}
	if result.CurrentLocationIdentifier == nil {
		t.Fatal("want CurrentLocationIdentifier non-nil after update, got nil")
	}
	if result.Name != newName {
		t.Errorf("want Name=%q, got %q", newName, result.Name)
	}
	if result.Identifiers == nil {
		t.Error("want non-nil Identifiers slice, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test ./internal/storage/... -run TestUpdateAsset_PopulatesCurrentLocationIdentifier
```
Expected: FAIL — `result.CurrentLocationIdentifier` doesn't exist (compile error: `UpdateAsset` returns `*asset.Asset`, not `*asset.AssetWithLocation`).

- [ ] **Step 3: Widen `storage.UpdateAsset` signature**

In `backend/internal/storage/assets.go`, replace the `UpdateAsset` function (starts ~line 83). The core change: on success, delegate to `getAssetWithLocationByID` instead of returning the bare asset.

```go
func (s *Storage) UpdateAsset(ctx context.Context, orgID, id int, request asset.UpdateAssetRequest) (*asset.AssetWithLocation, error) {
	updates := []string{}
	args := []any{id, orgID}
	argPos := 3
	fields, err := mapReqToFields(request)

	if err != nil {
		return nil, err
	}

	for key, value := range fields {
		if value != nil {
			updates = append(updates, fmt.Sprintf("%s = $%d", key, argPos))
			args = append(args, value)
			argPos++
		}
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		update trakrf.assets
		set %s, updated_at = now()
		where id = $1 and org_id = $2 and deleted_at is null
		returning id
	`, strings.Join(updates, ", "))

	var updatedID int
	err = s.pool.QueryRow(ctx, query, args...).Scan(&updatedID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			identifier := "unknown"
			if request.Identifier != nil {
				identifier = *request.Identifier
			}
			return nil, fmt.Errorf("asset with identifier %s already exists", identifier)
		}
		if strings.Contains(err.Error(), "current_location_id_fkey") {
			return nil, fmt.Errorf("invalid current_location_id: location does not exist")
		}
		return nil, fmt.Errorf("failed to update asset: %w", err)
	}

	return s.getAssetWithLocationByID(ctx, updatedID)
}
```

- [ ] **Step 4: Widen `storage.CreateAssetWithIdentifiers` signature**

In `backend/internal/storage/assets.go`, update the return type and tail call (line ~412):

```go
func (s *Storage) CreateAssetWithIdentifiers(ctx context.Context, request asset.CreateAssetWithIdentifiersRequest) (*asset.AssetWithLocation, error) {
	// ... body unchanged until the final return ...

	// (keep all existing logic through the query execution and error handling)

	return s.getAssetWithLocationByID(ctx, assetID)
}
```

The only line-level changes:
- Return type: `*asset.AssetView` → `*asset.AssetWithLocation`.
- Final line: `return s.GetAssetViewByID(ctx, assetID)` → `return s.getAssetWithLocationByID(ctx, assetID)`.

- [ ] **Step 5: Update handler `Create` — unify and project**

In `backend/internal/handlers/assets/assets.go`, **delete** `createAssetWithoutIdentifiers` (lines 41-67 — the whole helper function).

Replace the body of `Create` (lines 86-131) with:

```go
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}

	var request asset.CreateAssetWithIdentifiersRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	request.OrgID = orgID

	result, err := handler.storage.CreateAssetWithIdentifiers(r.Context(), request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/assets/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": asset.ToPublicAssetView(*result)})
}
```

- [ ] **Step 6: Update `doUpdateAsset` — project to PublicAssetView**

In `backend/internal/handlers/assets/assets.go`, replace line 213 (`httputil.WriteJSON(w, http.StatusOK, map[string]*asset.Asset{"data": result})`) with:

```go
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": asset.ToPublicAssetView(*result)})
```

The `result` variable is now `*asset.AssetWithLocation` (from the widened `storage.UpdateAsset`); the nil check on line 207 still works.

- [ ] **Step 7: Adjust pre-existing storage tests for the new signature**

In `backend/internal/storage/assets_test.go`, the tests at lines 432-546 (`TestUpdateAsset`, `TestUpdateAsset_NoFields`, `TestUpdateAsset_NotFound`, `TestUpdateAsset_PartialUpdate`) assign `result` from `UpdateAsset`. Due to Go field promotion, `result.ID`, `result.Name`, `result.Identifier`, etc. still resolve — no changes needed for field access.

However, if any test does something like:
```go
var _ *asset.Asset = result
```
or compares against a `*asset.Asset` literal, change the target type to `*asset.AssetWithLocation`.

Run a targeted grep:
```bash
rg -n 'var.*\*asset\.Asset' backend/internal/storage/assets_test.go backend/internal/storage/assets_crossorg_test.go
```
If nothing turns up, no edits needed.

- [ ] **Step 8: Run the full asset storage & handler tests**

```bash
just backend test ./internal/storage/... -run 'TestUpdateAsset|TestCreateAssetWithIdentifiers|TestGetAssetWithLocationByID'
just backend test ./internal/handlers/assets/...
```
Expected: PASS.

If compile errors from the test file's `*asset.Asset` assignments, fix them per Step 7 and re-run.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/storage/assets.go \
        backend/internal/handlers/assets/assets.go \
        backend/internal/storage/assets_test.go \
        backend/internal/storage/assets_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tra-429): asset write path returns PublicAssetView shape

Widen storage.UpdateAsset and storage.CreateAssetWithIdentifiers to return
*asset.AssetWithLocation (rich view with parent natural key + identifiers).
Handlers project through ToPublicAssetView before responding. Drops the
private createAssetWithoutIdentifiers helper — CreateAssetWithIdentifiers
handles the empty-identifiers case correctly.

Public write responses now match the PublicAssetView shape already emitted
by read endpoints (surrogate_id, current_location natural key, no id/org_id).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Asset handler typed response envelopes + Swagger

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go`

- [ ] **Step 1: Add typed response structs**

In `backend/internal/handlers/assets/assets.go`, alongside the existing `ListAssetsResponse` / `GetAssetResponse` declarations (around line 326-336), add:

```go
// CreateAssetResponse is the typed envelope returned by POST /api/v1/assets.
type CreateAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// UpdateAssetResponse is the typed envelope returned by PUT /api/v1/assets/{identifier}
// and PUT /api/v1/assets/by-id/{id}.
type UpdateAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}
```

- [ ] **Step 2: Update Swagger `@Success` annotation on `Create`**

In `backend/internal/handlers/assets/assets.go`, change line 77 from:
```go
// @Success      201  {object}  map[string]any                "data: asset.AssetView"
```
to:
```go
// @Success      201  {object}  assets.CreateAssetResponse
```

- [ ] **Step 3: Update Swagger `@Success` annotation on `UpdateAsset`**

Change line 141 from:
```go
// @Success      200  {object}  map[string]any                "data: asset.Asset"
```
to:
```go
// @Success      200  {object}  assets.UpdateAssetResponse
```

- [ ] **Step 4: Verify backend still builds & lints**

```bash
just backend lint
just backend test ./internal/handlers/assets/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/assets/assets.go
git commit -m "$(cat <<'EOF'
docs(tra-429): typed OpenAPI envelopes for asset create/update responses

Replace map[string]any @Success annotations with concrete
CreateAssetResponse / UpdateAssetResponse structs wrapping PublicAssetView,
matching the existing pattern for List/Get responses.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Asset integration tests — shape updates + negative regression

**Files:**
- Modify: `backend/internal/handlers/assets/assets_integration_test.go`
- Modify: `backend/internal/handlers/assets/by_id_integration_test.go`

- [ ] **Step 1: Discover existing assertions that read the internal shape**

```bash
rg -n '"id"|"org_id"|"current_location_id"' backend/internal/handlers/assets/assets_integration_test.go backend/internal/handlers/assets/by_id_integration_test.go
```
Note every line that asserts `id`, `org_id`, or `current_location_id` on a POST or PUT response body. These are the targets for Step 2.

- [ ] **Step 2: Rewrite the shape assertions**

For each create/update response assertion:
- `"id"` → `"surrogate_id"`.
- `"current_location_id"` → `"current_location"` (a string natural key, or absent/null if unset).
- Drop `"org_id"` checks entirely — the public shape does not expose it.
- Any `json.Unmarshal` into `asset.Asset` or `asset.AssetView` → `asset.PublicAssetView`.

Example shape:
```go
type createResp struct {
	Data asset.PublicAssetView `json:"data"`
}
var got createResp
if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
	t.Fatalf("unmarshal: %v", err)
}
if got.Data.Identifier != "widget-tra429" {
	t.Errorf("want Identifier=widget-tra429, got %q", got.Data.Identifier)
}
if got.Data.SurrogateID == 0 {
	t.Error("want SurrogateID non-zero")
}
```

Apply this pattern across every POST / PUT test in both files.

- [ ] **Step 3: Add the negative regression test**

Append to `backend/internal/handlers/assets/assets_integration_test.go`:

```go
// TestAssetWriteResponses_OmitInternalFields defends the public contract:
// POST and PUT responses MUST NOT contain "id" or "org_id" keys (TRA-429).
func TestAssetWriteResponses_OmitInternalFields(t *testing.T) {
	handler, _ := setupIntegrationHandler(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "POST /api/v1/assets",
			method: http.MethodPost,
			path:   "/api/v1/assets",
			body:   `{"identifier":"tra429-neg","name":"neg","type":"asset","valid_from":"2026-01-01","is_active":true}`,
		},
		// NOTE: PUT requires an existing asset — adapt using a created fixture.
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = withAuthedOrg(req, 1)
			rec := httptest.NewRecorder()

			handler.Create(rec, req) // switch to the router if the suite uses one

			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			data, ok := raw["data"].(map[string]any)
			if !ok {
				t.Fatalf("response has no data object: %s", rec.Body.String())
			}
			if _, has := data["id"]; has {
				t.Errorf("response data contains forbidden field %q: %s", "id", rec.Body.String())
			}
			if _, has := data["org_id"]; has {
				t.Errorf("response data contains forbidden field %q: %s", "org_id", rec.Body.String())
			}
			if _, has := data["surrogate_id"]; !has {
				t.Errorf("response data missing expected field %q", "surrogate_id")
			}
		})
	}
}
```

*(Adjust `setupIntegrationHandler` and `withAuthedOrg` to match the helpers already in use in the file — they vary by test suite. Look at neighboring tests for the correct pattern.)*

- [ ] **Step 4: Run the full handler test suite**

```bash
just backend test ./internal/handlers/assets/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/assets/assets_integration_test.go \
        backend/internal/handlers/assets/by_id_integration_test.go
git commit -m "$(cat <<'EOF'
test(tra-429): asset write handler tests expect PublicAssetView shape

Swap id/org_id/current_location_id assertions for surrogate_id/current_location
across POST and PUT tests. Add TestAssetWriteResponses_OmitInternalFields as
a regression guard against leaking internal fields in write responses.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Storage + handler: location write path returns LocationWithParent

Mirror of Task 3 for locations.

**Files:**
- Modify: `backend/internal/storage/locations.go`
- Modify: `backend/internal/handlers/locations/locations.go`
- Modify: `backend/internal/storage/locations_test.go`
- Add test: `backend/internal/storage/locations_integration_test.go`

- [ ] **Step 1: Write the failing test for UpdateLocation parent-identifier population**

Append to `backend/internal/storage/locations_integration_test.go`:

```go
// TestUpdateLocation_PopulatesParentIdentifier verifies UpdateLocation
// returns the LocationWithParent shape with ParentIdentifier populated
// when the location has a live parent.
func TestUpdateLocation_PopulatesParentIdentifier(t *testing.T) {
	store := setupTestStorageWithFixtures(t)
	orgID := 1

	child, err := store.GetLocationByIdentifier(context.Background(), orgID, "wh-1.bay-3")
	if err != nil || child == nil {
		t.Fatalf("precondition: GetLocationByIdentifier(wh-1.bay-3) returned (%v, %v)", child, err)
	}

	newName := "updated for tra-429"
	result, err := store.UpdateLocation(context.Background(), orgID, child.ID, location.UpdateLocationRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateLocation: %v", err)
	}
	if result == nil {
		t.Fatal("want non-nil LocationWithParent, got nil")
	}
	if result.ParentIdentifier == nil {
		t.Fatal("want ParentIdentifier non-nil, got nil")
	}
	if *result.ParentIdentifier != "wh-1" {
		t.Errorf("want ParentIdentifier=wh-1, got %q", *result.ParentIdentifier)
	}
	if result.Name != newName {
		t.Errorf("want Name=%q, got %q", newName, result.Name)
	}
	if result.Identifiers == nil {
		t.Error("want non-nil Identifiers slice")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test ./internal/storage/... -run TestUpdateLocation_PopulatesParentIdentifier
```
Expected: FAIL — `result.ParentIdentifier` doesn't exist.

- [ ] **Step 3: Widen `storage.UpdateLocation` signature**

In `backend/internal/storage/locations.go`, replace the `UpdateLocation` function (line 44). Change the query to return only `id`, then delegate to `getLocationWithParentByID`:

```go
func (s *Storage) UpdateLocation(ctx context.Context, orgID, id int, request location.UpdateLocationRequest) (*location.LocationWithParent, error) {
	updates := []string{}
	args := []any{id, orgID}
	argPos := 3
	fields, err := mapLocationReqToFields(request)

	if err != nil {
		return nil, err
	}

	for key, value := range fields {
		if value != nil {
			updates = append(updates, fmt.Sprintf("%s = $%d", key, argPos))
			args = append(args, value)
			argPos++
		}
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.locations
		SET %s, updated_at = NOW()
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING id
	`, strings.Join(updates, ", "))

	var updatedID int
	err = s.pool.QueryRow(ctx, query, args...).Scan(&updatedID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			identifier := "unknown"
			if request.Identifier != nil {
				identifier = *request.Identifier
			}
			return nil, fmt.Errorf("location with identifier %s already exists", identifier)
		}
		if strings.Contains(err.Error(), "parent_location_id_fkey") {
			return nil, fmt.Errorf("invalid parent_location_id: parent location does not exist")
		}
		return nil, fmt.Errorf("failed to update location: %w", err)
	}

	return s.getLocationWithParentByID(ctx, updatedID)
}
```

- [ ] **Step 4: Widen `storage.CreateLocationWithIdentifiers` signature**

In `backend/internal/storage/locations.go` (line 432), change the return type and the final return:

```go
func (s *Storage) CreateLocationWithIdentifiers(ctx context.Context, orgID int, request location.CreateLocationWithIdentifiersRequest) (*location.LocationWithParent, error) {
	// ... body unchanged ...

	return s.getLocationWithParentByID(ctx, locationID)
}
```

Only line-level changes:
- Return type: `*location.LocationView` → `*location.LocationWithParent`.
- Final line: `return s.GetLocationViewByID(ctx, locationID)` → `return s.getLocationWithParentByID(ctx, locationID)`.

- [ ] **Step 5: Update handler `Create` — unify and project**

In `backend/internal/handlers/locations/locations.go`, **delete** `createLocationWithoutIdentifiers` (lines 38-62).

Replace the body of `Create` (lines 81-124) with:

```go
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}

	var request location.CreateLocationWithIdentifiersRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	result, err := handler.storage.CreateLocationWithIdentifiers(r.Context(), orgID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/locations/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": location.ToPublicLocationView(*result)})
}
```

- [ ] **Step 6: Update `doUpdate` — project to PublicLocationView**

In `backend/internal/handlers/locations/locations.go`, replace line 204 (`httputil.WriteJSON(w, http.StatusOK, map[string]*location.Location{"data": result})`) with:

```go
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": location.ToPublicLocationView(*result)})
```

`result` is now `*location.LocationWithParent`; the nil check on line 198 still works.

- [ ] **Step 7: Adjust pre-existing storage tests**

Same field-promotion story as Task 3. Check:
```bash
rg -n 'var.*\*location\.Location' backend/internal/storage/locations_test.go backend/internal/storage/locations_crossorg_test.go
```
If nothing type-assigns the result to `*location.Location`, no edits required.

- [ ] **Step 8: Run location storage & handler tests**

```bash
just backend test ./internal/storage/... -run 'TestUpdateLocation|TestCreateLocationWithIdentifiers|TestGetLocationWithParentByID'
just backend test ./internal/handlers/locations/...
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/storage/locations.go \
        backend/internal/handlers/locations/locations.go \
        backend/internal/storage/locations_test.go \
        backend/internal/storage/locations_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tra-429): location write path returns PublicLocationView shape

Widen storage.UpdateLocation and storage.CreateLocationWithIdentifiers to
return *location.LocationWithParent (rich view with parent natural key
+ identifiers). Handlers project through ToPublicLocationView before
responding. Drops createLocationWithoutIdentifiers helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Location handler typed response envelopes + Swagger

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go`

- [ ] **Step 1: Add typed response structs**

Alongside existing `ListLocationsResponse` / `GetLocationResponse` (~line 270-281), add:

```go
// CreateLocationResponse is the typed envelope returned by POST /api/v1/locations.
type CreateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

// UpdateLocationResponse is the typed envelope returned by PUT /api/v1/locations/{identifier}
// and PUT /api/v1/locations/by-id/{id}.
type UpdateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}
```

- [ ] **Step 2: Update `Create` Swagger annotation**

Change line 72 from:
```go
// @Success      201  {object}  map[string]any                "data: location.LocationView"
```
to:
```go
// @Success      201  {object}  locations.CreateLocationResponse
```

- [ ] **Step 3: Update `Update` Swagger annotation**

Change line 134 from:
```go
// @Success      200  {object}  map[string]any                "data: location.Location"
```
to:
```go
// @Success      200  {object}  locations.UpdateLocationResponse
```

- [ ] **Step 4: Verify backend lints**

```bash
just backend lint
just backend test ./internal/handlers/locations/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/locations/locations.go
git commit -m "$(cat <<'EOF'
docs(tra-429): typed OpenAPI envelopes for location create/update responses

Replace map[string]any @Success annotations with CreateLocationResponse /
UpdateLocationResponse structs wrapping PublicLocationView.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Location integration tests — shape updates + negative regression

**Files:**
- Modify: `backend/internal/handlers/locations/locations_integration_test.go`
- Modify: `backend/internal/handlers/locations/by_id_integration_test.go` (check if exists — create only if missing)

- [ ] **Step 1: Survey both test files**

```bash
ls backend/internal/handlers/locations/
rg -n '"id"|"org_id"|"parent_location_id"' backend/internal/handlers/locations/locations_integration_test.go
```
If `by_id_integration_test.go` doesn't exist, the by-id route is either untested or covered by another file — grep for `UpdateByID` / `by-id/` coverage and update wherever it lives.

- [ ] **Step 2: Rewrite shape assertions**

For each create/update response assertion:
- `"id"` → `"surrogate_id"`.
- `"parent_location_id"` → `"parent"` (string natural key, may be absent / null).
- Drop `"org_id"` assertions.
- Unmarshal target types `location.Location` / `location.LocationView` → `location.PublicLocationView`.

- [ ] **Step 3: Add the negative regression test**

Append to `backend/internal/handlers/locations/locations_integration_test.go`:

```go
// TestLocationWriteResponses_OmitInternalFields defends the public contract:
// POST and PUT responses MUST NOT contain "id", "org_id", or "parent_location_id"
// keys (TRA-429).
func TestLocationWriteResponses_OmitInternalFields(t *testing.T) {
	handler, _ := setupIntegrationHandler(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "POST /api/v1/locations",
			method: http.MethodPost,
			path:   "/api/v1/locations",
			body:   `{"identifier":"tra429-neg","name":"neg","valid_from":"2026-01-01","is_active":true}`,
		},
		// NOTE: add a PUT case using a fixture location — pattern matches neighboring tests.
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = withAuthedOrg(req, 1)
			rec := httptest.NewRecorder()

			handler.Create(rec, req) // switch to router if suite uses one

			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			data, ok := raw["data"].(map[string]any)
			if !ok {
				t.Fatalf("response has no data object: %s", rec.Body.String())
			}
			for _, forbidden := range []string{"id", "org_id", "parent_location_id"} {
				if _, has := data[forbidden]; has {
					t.Errorf("response data contains forbidden field %q: %s", forbidden, rec.Body.String())
				}
			}
			if _, has := data["surrogate_id"]; !has {
				t.Errorf("response data missing expected field %q", "surrogate_id")
			}
		})
	}
}
```

*(Same caveat as Task 5 — match the helper names used by neighboring tests.)*

- [ ] **Step 4: Run the location handler tests**

```bash
just backend test ./internal/handlers/locations/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/locations/locations_integration_test.go \
        backend/internal/handlers/locations/by_id_integration_test.go
git commit -m "$(cat <<'EOF'
test(tra-429): location write handler tests expect PublicLocationView shape

Swap id/org_id/parent_location_id assertions for surrogate_id/parent across
POST and PUT tests. Add TestLocationWriteResponses_OmitInternalFields as
a regression guard.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

*(If `by_id_integration_test.go` didn't exist and nothing needed editing, drop it from the `git add`.)*

---

## Task 9: Regenerate OpenAPI spec

**Files:**
- Modify: `docs/api/openapi.public.json`
- Modify: `docs/api/openapi.public.yaml`

- [ ] **Step 1: Identify the spec regeneration recipe**

```bash
just --list --unsorted 2>/dev/null | rg -i 'openapi|swagger|swag'
```
Typical command on this repo: `just backend openapi` or `just backend swag`. Verify from the listing.

- [ ] **Step 2: Run the regen**

```bash
just backend openapi   # or whatever the recipe is — confirm from Step 1
```
Expected: updates `docs/api/openapi.public.json` and `.yaml`. No errors.

- [ ] **Step 3: Inspect the diff**

```bash
git diff docs/api/openapi.public.yaml | head -200
```
Spot-check:
- `POST /api/v1/assets` `201` response references `assets.CreateAssetResponse` → `asset.PublicAssetView`.
- `PUT /api/v1/assets/{identifier}` `200` references `assets.UpdateAssetResponse`.
- Location equivalents.
- No stray `asset.Asset` / `asset.AssetView` / `location.Location` / `location.LocationView` references in write-response schemas.

- [ ] **Step 4: Commit**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "$(cat <<'EOF'
chore(tra-429): regenerate OpenAPI spec for PublicAssetView/PublicLocationView write responses

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Frontend — `normalizeLocation` helper

**Files:**
- Create: `frontend/src/lib/location/normalize.ts`
- Create: `frontend/src/lib/location/normalize.test.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/location/normalize.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { normalizeLocation } from './normalize';

describe('normalizeLocation', () => {
  it('populates id from surrogate_id when response uses public shape', () => {
    const raw = { surrogate_id: 42, identifier: 'wh-1', name: 'Warehouse 1' };
    const normalized = normalizeLocation(raw);
    expect(normalized.id).toBe(42);
    expect(normalized.surrogate_id).toBe(42);
    expect(normalized.identifier).toBe('wh-1');
  });

  it('populates surrogate_id from id when response uses legacy shape', () => {
    const raw = { id: 7, identifier: 'wh-2', name: 'Warehouse 2' };
    const normalized = normalizeLocation(raw);
    expect(normalized.id).toBe(7);
    expect(normalized.surrogate_id).toBe(7);
  });

  it('is idempotent when both fields present', () => {
    const raw = { id: 3, surrogate_id: 3, identifier: 'wh-3', name: 'Warehouse 3' };
    const normalized = normalizeLocation(raw);
    expect(normalized.id).toBe(3);
    expect(normalized.surrogate_id).toBe(3);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just frontend test src/lib/location/normalize.test.ts
```
Expected: FAIL — module `./normalize` does not resolve.

- [ ] **Step 3: Create the helper**

Create `frontend/src/lib/location/normalize.ts`:

```ts
import type { Location } from '@/types/locations';

/**
 * Normalize a location response to the internal cache shape.
 *
 * The public API returns `surrogate_id` with no `id` field; legacy or
 * mocked responses may return `id` with no `surrogate_id`. The cache
 * keys locations by `id`, so populate it from whichever field the server
 * sent. Mirrors `lib/asset/normalize.ts`.
 */
export function normalizeLocation(raw: any): Location {
  const id = raw.id ?? raw.surrogate_id;
  return {
    ...raw,
    id,
    surrogate_id: raw.surrogate_id ?? raw.id,
  };
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
just frontend test src/lib/location/normalize.test.ts
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/location/normalize.ts frontend/src/lib/location/normalize.test.ts
git commit -m "$(cat <<'EOF'
feat(tra-429): add normalizeLocation helper mirroring normalizeAsset

Prepares the frontend to handle PublicLocationView responses (surrogate_id
only, no id) that the backend will emit from write endpoints post-TRA-429.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Refactor `AssetFormModal` to normalize once

**Files:**
- Modify: `frontend/src/components/assets/AssetFormModal.tsx`

- [ ] **Step 1: Replace the create branch**

In `frontend/src/components/assets/AssetFormModal.tsx`, replace lines 61-98 (the entire `if (mode === 'create')` branch body up to and including the success toast) with:

```tsx
        const response = await assetsApi.create(createData as CreateAssetRequest);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }
        const normalized = normalizeAsset(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        const newAssetId = normalized.id;

        const validIdentifiers = identifiers.filter(id => id.value.trim() !== '');
        for (const identifier of validIdentifiers) {
          try {
            await assetsApi.addIdentifier(newAssetId, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr: any) {
            console.error('Failed to add identifier:', idErr);
            toast.error(`Failed to add tag "${identifier.value}": ${idErr.message || 'Unknown error'}`);
          }
        }

        if (validIdentifiers.length > 0) {
          const freshResponse = await assetsApi.get(newAssetId);
          if (freshResponse.data?.data) {
            addAsset(normalizeAsset(freshResponse.data.data));
          } else {
            addAsset(normalized);
          }
        } else {
          addAsset(normalized);
        }

        toast.success(`Asset "${normalized.identifier}" created successfully`);
```

- [ ] **Step 2: Replace the edit branch**

Replace lines 99-137 (the `else if (mode === 'edit' && asset)` branch body, up to and including its success toast) with:

```tsx
      } else if (mode === 'edit' && asset) {
        const identifiers = (data as UpdateAssetRequest & { identifiers?: TagIdentifierInput[] }).identifiers || [];
        const newIdentifiers = identifiers.filter(id => !id.id);

        const { identifiers: _, ...updateData } = data as UpdateAssetRequest & { identifiers?: TagIdentifierInput[] };

        const response = await assetsApi.update(asset.id, updateData);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }
        const normalized = normalizeAsset(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        for (const identifier of newIdentifiers) {
          try {
            await assetsApi.addIdentifier(asset.id, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr: any) {
            console.error('Failed to add identifier:', idErr);
            toast.error(`Failed to add tag "${identifier.value}": ${idErr.message || 'Unknown error'}`);
          }
        }

        const freshResponse = await assetsApi.get(asset.id);
        if (freshResponse.data?.data) {
          updateCachedAsset(asset.id, normalizeAsset(freshResponse.data.data));
        } else {
          updateCachedAsset(asset.id, normalized);
        }

        toast.success(`Asset "${normalized.identifier}" updated successfully`);
```

- [ ] **Step 3: Type-check and test**

```bash
just frontend typecheck
just frontend test src/components/assets/
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/assets/AssetFormModal.tsx
git commit -m "$(cat <<'EOF'
refactor(tra-429): AssetFormModal normalizes response once at API boundary

After the backend switch, POST/PUT responses use PublicAssetView shape
(surrogate_id only). Normalize immediately after each API call so the
id-check, cache write, and toast all see a populated `id`. Behavior
unchanged under legacy shape because normalizeAsset is a no-op when
`id` is already present.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Refactor `LocationFormModal` to normalize once

**Files:**
- Modify: `frontend/src/components/locations/LocationFormModal.tsx`

- [ ] **Step 1: Add the import**

At the top of the file, alongside other `@/lib/...` imports, add:

```tsx
import { normalizeLocation } from '@/lib/location/normalize';
```

- [ ] **Step 2: Replace the create branch**

Replace lines 29-65 (the `if (mode === 'create')` branch up to and including the toast) with:

```tsx
      if (mode === 'create') {
        const identifiers = (data as CreateLocationRequest & { identifiers?: TagIdentifierInput[] }).identifiers || [];
        const { identifiers: _, ...createData } = data as CreateLocationRequest & { identifiers?: TagIdentifierInput[] };

        const response = await locationsApi.create(createData as CreateLocationRequest);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Location API may not be available.');
        }
        const normalized = normalizeLocation(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Location API may not be available.');
        }

        const newLocationId = normalized.id;
        const validIdentifiers = identifiers.filter(id => id.value.trim() !== '');
        for (const identifier of validIdentifiers) {
          try {
            await locationsApi.addIdentifier(newLocationId, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr) {
            console.error('Failed to add identifier:', idErr);
          }
        }

        if (validIdentifiers.length > 0) {
          const freshResponse = await locationsApi.get(newLocationId);
          if (freshResponse.data?.data) {
            addLocation(normalizeLocation(freshResponse.data.data));
          } else {
            addLocation(normalized);
          }
        } else {
          addLocation(normalized);
        }

        toast.success(`Location "${normalized.identifier}" created successfully`);
```

- [ ] **Step 3: Replace the edit branch**

Replace lines 66-98 (the `else if (mode === 'edit' && location)` branch) with:

```tsx
      } else if (mode === 'edit' && location) {
        const identifiers = (data as UpdateLocationRequest & { identifiers?: TagIdentifierInput[] }).identifiers || [];
        const newIdentifiers = identifiers.filter(id => !id.id);

        const { identifiers: _, ...updateData } = data as UpdateLocationRequest & { identifiers?: TagIdentifierInput[] };

        const response = await locationsApi.update(location.id, updateData);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Location API may not be available.');
        }
        const normalized = normalizeLocation(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Location API may not be available.');
        }

        for (const identifier of newIdentifiers) {
          try {
            await locationsApi.addIdentifier(location.id, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr) {
            console.error('Failed to add identifier:', idErr);
          }
        }

        const freshResponse = await locationsApi.get(location.id);
        if (freshResponse.data?.data) {
          updateLocation(location.id, normalizeLocation(freshResponse.data.data));
        } else {
          updateLocation(location.id, normalized);
        }

        toast.success(`Location "${normalized.identifier}" updated successfully`);
```

- [ ] **Step 4: Type-check and test**

```bash
just frontend typecheck
just frontend test src/components/locations/
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/locations/LocationFormModal.tsx
git commit -m "$(cat <<'EOF'
refactor(tra-429): LocationFormModal normalizes response once at API boundary

Same pattern as AssetFormModal. Uses the new normalizeLocation helper so
the id-check, cache write, and toast all operate on a populated `id` after
the backend emits PublicLocationView (surrogate_id only).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Audit location hooks for response shape drift

**Files:**
- Modify (maybe): `frontend/src/hooks/locations/useLocationMutations.ts`
- Modify (maybe): `frontend/src/hooks/locations/useLocations.ts`
- Modify (maybe): `frontend/src/hooks/locations/useLocation.ts`

- [ ] **Step 1: Audit each file for `.data.id` or similar reads on API responses**

```bash
rg -n '\.data\.data\.id|response\.data\.data|res\.data\.data' frontend/src/hooks/locations/
```
For each hit, check whether the reference feeds into a cache/store. If yes → wrap with `normalizeLocation` at the seam.

- [ ] **Step 2: For each flagged line, apply the normalize wrapper**

Pattern for each seam:
```ts
import { normalizeLocation } from '@/lib/location/normalize';

// before:
//   useLocationStore.getState().addLocation(response.data.data);
// after:
useLocationStore.getState().addLocation(normalizeLocation(response.data.data));
```

If a hook transforms the API response into an intermediate object, normalize at the point the raw API payload enters the hook.

- [ ] **Step 3: Type-check and test**

```bash
just frontend typecheck
just frontend test src/hooks/locations/
```
Expected: PASS.

- [ ] **Step 4: Commit (only if files changed)**

```bash
git status --short frontend/src/hooks/locations/
# only commit if there are changes
git add frontend/src/hooks/locations/
git commit -m "$(cat <<'EOF'
fix(tra-429): normalize location API responses at hook seams

Guards location store writes against the PublicLocationView shape
(surrogate_id only) that write endpoints will emit after the backend
changes land.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If nothing changed, skip the commit.

---

## Task 14: End-to-end verification + push + open PR

**Files:** None modified; this is a verification gate + ship step.

- [ ] **Step 1: Full backend test suite**

```bash
just backend test
```
Expected: all green.

- [ ] **Step 2: Backend lint**

```bash
just backend lint
```
Expected: no new findings.

- [ ] **Step 3: Full frontend test + typecheck + lint**

```bash
just frontend validate
```
Expected: all green.

- [ ] **Step 4: Push the branch**

```bash
git push -u origin miks2u/tra-429-unify-write-handler-responses
```

- [ ] **Step 5: Manual preview-deploy sanity check**

After the `sync-preview` workflow finishes (watch `gh run list` or the PR), open `https://app.preview.trakrf.id` and:
- Create a new asset. Open the browser's Network tab → the POST response body contains `surrogate_id`, NOT `id` / `org_id`. No console errors.
- Edit the same asset. PUT response same check.
- Repeat for one location (create + edit).

If any check fails, fix and push before marking the task complete.

- [ ] **Step 6: Open the PR**

```bash
gh pr create --title "fix(tra-429): unify asset/location write-handler responses to PublicAssetView/PublicLocationView" --body "$(cat <<'EOF'
## Summary
- Widen `storage.UpdateAsset` / `storage.CreateAssetWithIdentifiers` to return `*asset.AssetWithLocation`; mirror for locations.
- Unify create paths — drop `createAssetWithoutIdentifiers` / `createLocationWithoutIdentifiers` helpers, always go through `*WithIdentifiers`.
- Project write responses through `ToPublicAssetView` / `ToPublicLocationView`; add typed OpenAPI response envelopes.
- Add `normalizeLocation` helper (mirror of `normalizeAsset`); refactor both form modals to normalize once at the API boundary.
- Regenerate OpenAPI spec.

Closes [TRA-429](https://linear.app/trakrf/issue/TRA-429).

## Test plan
- [ ] `just backend test` green
- [ ] `just backend lint` green
- [ ] `just frontend validate` green
- [ ] Preview: POST `/api/v1/assets` response body contains `surrogate_id`, omits `id` / `org_id`
- [ ] Preview: PUT `/api/v1/assets/{identifier}` response body contains `surrogate_id`, omits `id`
- [ ] Same for `/api/v1/locations` POST and PUT
- [ ] Preview UI: create + edit asset round-trips through the form modal without console errors
- [ ] Preview UI: create + edit location round-trips without console errors

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 7: Confirm PR URL**

Capture the PR URL the command emits and report it. The task is complete when preview checks in Step 5 have passed.

---

## Self-review against spec

Run the following spec-coverage check before handing the plan off.

- [ ] Every spec "Storage changes" bullet maps to Task 1, 2, 3, or 6. ✓
- [ ] Every spec "Handler changes" bullet maps to Task 3, 4, 6, or 7. ✓
- [ ] Every spec "Frontend changes" bullet maps to Task 10, 11, 12, or 13. ✓
- [ ] Spec "Integration tests" → Task 5 (asset), Task 8 (location). ✓
- [ ] Spec "Negative regression test" → Steps in Task 5 and Task 8. ✓
- [ ] Spec "Rollout" → Task 14 (atomic PR with backend + frontend). ✓
- [ ] Spec "Verification gate" → Task 14 Steps 1-5. ✓
- [ ] OpenAPI regen → Task 9. ✓
