# TRA-533 — Inventory/Save FK Cleanup + Alias Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the undocumented `_id`/`_ids` request aliases on `POST /api/v1/inventory/save`, lock in regression tests so the next embedded-SQL miss on this endpoint fails CI, and propagate the resulting backend shape change through the React frontend.

**Architecture:** Single PR with coordinated backend + frontend changes. Backend `SaveRequest` collapses to two required fields (`location_identifier`, `asset_identifiers`); cross-field branches that existed only to support the dual shape delete with the fields they guarded. Frontend `SaveInventoryRequest` type renames its fields to natural keys; `InventoryScreen` constructs an array of asset-identifier strings from the tag store (which already carries `assetIdentifier` alongside `assetId`) and passes natural keys at the API boundary. Three new integration test cases lock in the post-change behavior.

**Tech Stack:** Go (chi, validator/v10, pgx, testify), TypeScript/React (zustand, axios, vitest), TimescaleDB. Just task runner. swag for OpenAPI generation.

**Branch:** `fix/tra-533-inventory-save-fk-cleanup` (already created in worktree).

**Spec:** `docs/superpowers/specs/2026-04-27-tra-533-inventory-save-fk-cleanup-design.md`

---

## File Map

**Backend (modify):**
- `backend/internal/handlers/inventory/save.go` — struct + handler simplification.
- `backend/internal/handlers/inventory/save_test.go` — delete obsolete dual-shape tests; rework remaining tests' body shapes.
- `backend/internal/handlers/inventory/public_write_integration_test.go` — add 3 new cases; rework happy paths to identifier shape; delete cross-field-disagree test.

**Backend (regenerate):**
- `backend/docs/api/openapi.public.{json,yaml}` — committed public OpenAPI snapshot.
- `backend/internal/handlers/swaggerspec/*` — gitignored embedded specs.

**Backend (no changes — verified):**
- `backend/internal/storage/inventory.go` — already correct post-c678465.

**Frontend (modify):**
- `frontend/src/lib/api/inventory.ts` — request type rename.
- `frontend/src/components/InventoryScreen.tsx` — construction site (lines 241-243) + call site (lines 248-250).
- `frontend/src/hooks/inventory/useInventorySave.ts` — 403 warning log fields (lines 54-58).
- `frontend/src/hooks/inventory/useInventorySave.test.ts` — fixture data.
- `frontend/src/components/__tests__/InventoryScreen.test.tsx` — mocked save() call assertions.
- `frontend/src/components/__tests__/InventoryScreen.authgate.test.tsx` — verify, likely no change.

---

## Task 1: Backend — collapse `SaveRequest` and refactor unit tests

**Why this is one atomic task instead of TDD-split:** the existing unit tests reference `SaveRequest{LocationID:..., AssetIDs:...}` directly. Removing those struct fields breaks compilation of those tests. Struct + handler + unit-test surgery must be one commit. The TDD discipline applies to the *new* behavior tests added in Task 2, where "watch it fail before you make it pass" still holds.

**Files:**
- Modify: `backend/internal/handlers/inventory/save.go`
- Modify: `backend/internal/handlers/inventory/save_test.go`

### Steps

- [ ] **Step 1.1: Update `SaveRequest` struct in `save.go`**

In `backend/internal/handlers/inventory/save.go`, replace the existing struct (lines 45-59) with:

```go
// SaveRequest is the request body for POST /api/v1/inventory/save.
//
// Both fields are required; the public surface has a single canonical shape
// (TRA-533). Use natural identifiers — surrogate IDs were removed to collapse
// the C2-class spelling proliferation flagged in TRA-532 finding F10.
type SaveRequest struct {
	LocationIdentifier *string  `json:"location_identifier" validate:"required,min=1,max=255" example:"WH-01"`
	AssetIdentifiers   []string `json:"asset_identifiers" validate:"required,min=1,dive,min=1,max=255" example:"ASSET-0001"`
}
```

- [ ] **Step 1.2: Simplify the `Save` handler body in `save.go`**

In `backend/internal/handlers/inventory/save.go`, replace the cross-field block (lines 104-143) and the conditional resolution gates (lines 144-210) with the simplified flow below. The validator now enforces required-ness, so the hand-rolled "neither field" / "both fields" / "disagree" branches all delete with the fields they guarded.

Replace the section from `// Cross-field: at least one of (location_id, location_identifier).` (line 104) through `assetIDs = ids` (line 209) with:

```go
	// Resolve location_identifier → numeric.
	loc, err := h.storage.GetLocationByIdentifier(r.Context(), orgID, *request.LocationIdentifier)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}
	if loc == nil {
		msg := fmt.Sprintf("location_identifier %q not found", *request.LocationIdentifier)
		httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.InventorySaveFailed, msg, requestID,
			[]modelerrors.FieldError{{
				Field:   "location_identifier",
				Code:    "invalid_value",
				Message: msg,
			}})
		return
	}
	locationID := loc.ID

	// Resolve asset_identifiers → numeric IDs (one query).
	resolved, err := h.storage.GetAssetIDsByIdentifiers(r.Context(), orgID, request.AssetIdentifiers)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}
	assetIDs := make([]int, 0, len(request.AssetIdentifiers))
	var missing []string
	for _, ident := range request.AssetIdentifiers {
		if id, ok := resolved[ident]; ok {
			assetIDs = append(assetIDs, id)
		} else {
			missing = append(missing, ident)
		}
	}
	if len(missing) > 0 {
		msg := fmt.Sprintf("asset_identifier(s) not found: %s", strings.Join(missing, ", "))
		fields := make([]modelerrors.FieldError, 0, len(missing))
		for _, m := range missing {
			fields = append(fields, modelerrors.FieldError{
				Field:   "asset_identifiers",
				Code:    "invalid_value",
				Message: fmt.Sprintf("asset_identifier %q not found", m),
			})
		}
		httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.InventorySaveFailed, msg, requestID, fields)
		return
	}
```

The rest of the handler (the `result, err := h.storage.SaveInventoryScans(...)` block onward) stays unchanged.

- [ ] **Step 1.3: Delete obsolete unit tests in `save_test.go`**

Delete these three tests entirely (they test branches that no longer exist):

- `TestSave_BothAssetFieldsPresent_Rejected` (lines 562-575)
- `TestSave_LocationFieldsDisagree_Rejected` (lines 577-595)
- `TestSave_BothLocationFieldsAgree_Succeeds` (lines 669-690)

- [ ] **Step 1.4: Rework remaining unit tests in `save_test.go` to use the identifier shape**

These tests currently reference `SaveRequest{LocationID:..., AssetIDs:...}` and won't compile after Step 1.1. Update each as follows:

In `TestSave_MissingOrgContext` (lines 77-105), replace the body construction (lines 80-84) with:

```go
	body := SaveRequest{
		LocationIdentifier: ptr("WH-01"),
		AssetIdentifiers:   []string{"ASSET-0001", "ASSET-0002"},
	}
	bodyBytes, _ := json.Marshal(body)
```

In `TestSave_EmptyAssetIDs` (lines 150-190), rename the function to `TestSave_EmptyAssetIdentifiers` and replace the body (lines 157-161) with:

```go
	body := SaveRequest{
		LocationIdentifier: ptr("WH-01"),
		AssetIdentifiers:   []string{}, // empty array — fails validate:"required,min=1"
	}
	bodyBytes, _ := json.Marshal(body)
```

In `TestSave_NeitherLocationFieldProvided` (lines 128-148), update the body to use the identifier-shape key:

```go
	body := map[string]any{
		"asset_identifiers": []string{"ASSET-0001"},
	}
```

In `TestSave_LocationIdentifierNotFound_Rejected` (lines 597-612), the body already uses `location_identifier`. Update only `"asset_ids"` → `"asset_identifiers"` (string array):

```go
	body := map[string]any{
		"location_identifier": "ghost",
		"asset_identifiers":   []string{"ASSET-0001"},
	}
```

In `TestSave_LocationAccessDenied` (lines 460-476) and `TestSave_AssetAccessDenied` (lines 478-496) and `TestSave_InternalStorageError` (lines 498-511) and `TestSave_Success` (lines 513-540), update each `req := newTestRequest(t, SaveRequest{LocationID:..., AssetIDs:...}, 1)` to use the identifier shape via `map[string]any` body, and configure the mock's `locationByIdentifier` and `assetIDsByIdentifiers` so resolution succeeds before the storage call. Pattern, applied to `TestSave_LocationAccessDenied` (lines 460-476):

```go
func TestSave_LocationAccessDenied(t *testing.T) {
	mock := &mockInventoryStorage{
		saveError: &storage.InventoryAccessError{
			Reason:     "location",
			OrgID:      1,
			LocationID: 999,
		},
		locationByIdentifier: map[string]*location.LocationWithParent{
			"WH-99": {LocationView: location.LocationView{Location: location.Location{ID: 999, Identifier: "WH-99"}}},
		},
		assetIDsByIdentifiers: map[string]int{"ASSET-0001": 100, "ASSET-0002": 101},
	}
	handler := NewHandler(mock)

	body := map[string]any{
		"location_identifier": "WH-99",
		"asset_identifiers":   []string{"ASSET-0001", "ASSET-0002"},
	}
	req := newTestRequest(t, body, 1)
	w := httptest.NewRecorder()
	handler.Save(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "not found or access denied")
}
```

Apply the same pattern (mock with `locationByIdentifier` + `assetIDsByIdentifiers`, `map[string]any` body) to the other three. `TestSave_AssetAccessDenied` and `TestSave_InternalStorageError` and `TestSave_Success` only differ in their `mock.saveError` / `mock.saveResult` setup — keep those unchanged.

In `TestSave_RequiresAtLeastOneLocationField` (lines 542-550), update the body key:

```go
	req := newTestRequest(t, map[string]any{"asset_identifiers": []string{"ASSET-0001"}}, 1)
```

In `TestSave_RequiresAtLeastOneAssetField` (lines 552-560), update the body key:

```go
	req := newTestRequest(t, map[string]any{"location_identifier": "WH-01"}, 1)
```

- [ ] **Step 1.5: Rework `TestSaveRequest_Validation` table-driven test (lines 208-279)**

This whole test asserts validator behavior on the struct. The cases that reference `LocationID`/`AssetIDs` must drop. Replace the test body with:

```go
func TestSaveRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request SaveRequest
		wantErr bool
	}{
		{
			name: "valid identifier request",
			request: SaveRequest{
				LocationIdentifier: ptr("WH-01"),
				AssetIdentifiers:   []string{"ASSET-0001", "ASSET-0002"},
			},
			wantErr: false,
		},
		{
			name:    "all-empty fails: location_identifier and asset_identifiers required",
			request: SaveRequest{},
			wantErr: true,
		},
		{
			name: "missing location_identifier fails",
			request: SaveRequest{
				AssetIdentifiers: []string{"ASSET-0001"},
			},
			wantErr: true,
		},
		{
			name: "missing asset_identifiers fails",
			request: SaveRequest{
				LocationIdentifier: ptr("WH-01"),
			},
			wantErr: true,
		},
		{
			name: "empty asset_identifiers fails (min=1)",
			request: SaveRequest{
				LocationIdentifier: ptr("WH-01"),
				AssetIdentifiers:   []string{},
			},
			wantErr: true,
		},
		{
			name: "asset_identifiers with empty string element fails (dive,min=1)",
			request: SaveRequest{
				LocationIdentifier: ptr("WH-01"),
				AssetIdentifiers:   []string{""},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.request)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

- [ ] **Step 1.6: Run unit tests to confirm green**

Run: `just backend test ./internal/handlers/inventory/... -run "^TestSave|^TestInventory|^TestSaveRequest|^TestSaveInventory|^TestAccessDenied"`

Expected: all tests pass. Specifically the renamed `TestSave_EmptyAssetIdentifiers` and reworked `TestSaveRequest_Validation` cases should be in the output and passing.

- [ ] **Step 1.7: Run lint to catch any unused imports/vars**

Run: `just backend lint`

Expected: clean. If `errors` or `strings` imports become unused after the handler simplification, drop them. (Note: `strings` is still used by the asset-not-found path; `errors` was used by `TestSave_LocationFieldsDisagree_Rejected` which we deleted — likely still used elsewhere, verify.)

- [ ] **Step 1.8: Commit**

```bash
git add backend/internal/handlers/inventory/save.go backend/internal/handlers/inventory/save_test.go
git commit -m "$(cat <<'EOF'
refactor(tra-533): collapse inventory/save to identifier-only shape

SaveRequest drops location_id and asset_ids surrogate aliases. Required-ness
moves into struct tags; cross-field branches that existed only to support
the dual shape delete with the fields they guarded.

Unit tests updated: dropped tests for deleted branches (BothAssetFields,
LocationFieldsDisagree, BothLocationFieldsAgree); reworked remaining tests
to use map[string]any{"location_identifier", "asset_identifiers"} bodies
with mock identifier resolution stubbed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Backend — add regression integration test cases

**TDD posture:** the three new cases (multi-asset → 201, empty asset_identifiers → 400, legacy-shape rejection → 400) all pass on the post-Task-1 implementation. They are added as lock-ins, not as drivers of new code. We still verify they fail on the pre-Task-1 baseline by running them against the original code if curious — but that's optional and not required to advance.

**Files:**
- Modify: `backend/internal/handlers/inventory/public_write_integration_test.go`

**Existing test landscape (from reading the file):** Tests are in `package inventory_test` with build tag `integration`. Helpers in this file:

- `testutil.SetupTestDB(t) → (store *storage.Storage, cleanup func())`
- `seedInventoryOrgAndKey(t, pool, store, scopes []string) → (orgID int, token string)` — new org via `testutil.CreateTestAccount`, then key.
- `seedInventoryKeyForOrg(t, pool, store, orgID, scopes []string) → (orgID, token)` — key for existing org.
- `createInventoryOrg(t, pool, name string) → orgID` — for cross-org tests where multiple orgs are needed.
- `buildInventoryPublicWriteRouter(store) → *chi.Mux`
- Setup boilerplate: `t.Setenv("JWT_SECRET", "<unique>")`, `store, cleanup := testutil.SetupTestDB(t)`, `defer cleanup()`, `pool := store.Pool().(*pgxpool.Pool)`.

Use these exact helpers — do not introduce new ones.

Existing tests:
- `TestInventorySave_APIKey_HappyPath` (line 88) — surrogate-ID body. **Rework** to identifier shape (Step 2.2).
- `TestInventorySave_WrongScope_Returns403` (line 124) — body shape is incidental (middleware rejects). **Update body** to identifier shape for cleanliness (Step 2.2).
- `TestInventorySave_SessionAuth_HappyPath` (line 147) — surrogate-ID body. **Rework** to identifier shape (Step 2.2).
- `TestInventorySave_CrossOrg_Returns403` (line 186) — surrogate-ID body, asserts 403. **Behavior change**: with identifier shape, RLS filters orgA's location out of orgB's identifier lookup, so the resolution-not-found path fires a **400** instead of 403. Test gets renamed and asserts 400 (Step 2.3).
- `TestInventorySave_APIKey_Identifiers_HappyPath` (line 225) — redundant with reworked APIKey_HappyPath. **Delete** (Step 2.4).
- `TestInventorySave_APIKey_LocationIdentifierNotFound` (line 263) — already identifier shape. **Keep unchanged**.
- `TestInventorySave_APIKey_AssetIdentifierNotFound` (line 288) — already identifier shape. **Keep unchanged**.
- `TestInventorySave_APIKey_LocationFieldsDisagree` (line 313) — branch gone. **Delete** (Step 2.4).
- `TestInventorySave_APIKey_BothAssetFields_Rejected` (line 344) — branch gone. **Delete** (Step 2.4).

### Steps

- [ ] **Step 2.1: Open `backend/internal/handlers/inventory/public_write_integration_test.go`**

Just have the file open in the editor. No commands.

- [ ] **Step 2.2: Rework the three identifier-incidental tests to use natural-key bodies**

In `TestInventorySave_APIKey_HappyPath` (lines 88-122), replace the body construction at line 110 with:

```go
	body := fmt.Sprintf(`{"location_identifier":%q,"asset_identifiers":[%q]}`, loc.Identifier, asset.Identifier)
```

The fixtures already use `Identifier: "inv-wh"` and `Identifier: "inv-asset"` (lines 97 and 103) — those values feed `loc.Identifier` and `asset.Identifier`.

In `TestInventorySave_WrongScope_Returns403` (lines 124-141), replace the body literal at line 134 with:

```go
		bytes.NewBufferString(`{"location_identifier":"any","asset_identifiers":["any"]}`))
```

(The middleware rejects on scope before the handler runs; body shape is irrelevant to the test outcome but the canonical shape is cleaner.)

In `TestInventorySave_SessionAuth_HappyPath` (lines 147-184), replace the body construction at line 172 with:

```go
	body := fmt.Sprintf(`{"location_identifier":%q,"asset_identifiers":[%q]}`, loc.Identifier, asset.Identifier)
```

The fixtures use `sess-inv-wh` and `sess-inv-asset` — those flow through `loc.Identifier` / `asset.Identifier`.

- [ ] **Step 2.3: Convert `TestInventorySave_CrossOrg_Returns403` to assert 400**

The existing test relies on the surrogate-ID path: orgB submits orgA's numeric IDs, storage RLS rejects, handler returns 403. With identifier shape, RLS filters orgA's location out of orgB's `GetLocationByIdentifier` lookup, returning nil — handler returns **400** with `location_identifier 'xo-wh' not found`. The cross-org isolation still works; the failure mode just shifts from 403 (storage layer) to 400 (handler validation). Both are correct and both prove tenant isolation.

Rename the function to `TestInventorySave_CrossOrg_Returns400` and update the body at line 213:

```go
	body := fmt.Sprintf(`{"location_identifier":%q,"asset_identifiers":[%q]}`, loc.Identifier, asset.Identifier)
```

Update the assertion at lines 220-222:

```go
	// With the identifier-only shape, cross-org references fail at handler-side
	// resolution (RLS filters orgA's location out of orgB's identifier lookup),
	// returning 400. Tenant isolation is preserved; the failure mode is just
	// reported at the validation layer instead of the storage layer.
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "xo-wh")
	assert.Contains(t, w.Body.String(), "not found")
```

- [ ] **Step 2.4: Delete three obsolete tests**

Delete entirely:
- `TestInventorySave_APIKey_Identifiers_HappyPath` (lines 225-261) — redundant with reworked happy path.
- `TestInventorySave_APIKey_LocationFieldsDisagree` (lines 313-342) — disagree branch is gone.
- `TestInventorySave_APIKey_BothAssetFields_Rejected` (lines 344-362) — both-fields branch is gone.

- [ ] **Step 2.5: Add multi-asset happy-path test**

Append after the existing `TestInventorySave_APIKey_HappyPath`:

```go
func TestInventorySave_APIKey_MultiAsset_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-multi-asset")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "ma-wh", Name: "WH", Path: "ma-wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	asset1, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ma-asset-1", Name: "A1", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	asset2, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ma-asset-2", Name: "A2", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)
	body := fmt.Sprintf(
		`{"location_identifier":%q,"asset_identifiers":[%q,%q]}`,
		loc.Identifier, asset1.Identifier, asset2.Identifier,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, float64(2), data["count"])

	// Verify both rows landed in asset_scans.
	var rowCount int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM trakrf.asset_scans WHERE org_id = $1`, orgID).Scan(&rowCount))
	assert.Equal(t, 2, rowCount)
}
```

- [ ] **Step 2.6: Add empty-asset_identifiers 400 test**

```go
func TestInventorySave_EmptyAssetIdentifiers_Returns400(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-empty-assets")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	r := buildInventoryPublicWriteRouter(store)
	body := `{"location_identifier":"any-wh","asset_identifiers":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "asset_identifiers")
}
```

(No location seeding needed — the validator rejects on the empty array before resolution runs.)

- [ ] **Step 2.7: Add legacy-shape rejection 400 test (AC2 canary)**

```go
func TestInventorySave_LegacyShape_Returns400(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv-legacy-shape")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	r := buildInventoryPublicWriteRouter(store)
	// Pre-TRA-533 shape — must be rejected post-AC2.
	body := `{"location_id":1,"asset_ids":[1]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "location_identifier")
}
```

- [ ] **Step 2.8: Run integration tests**

Run: `just backend test-integration ./internal/handlers/inventory/...`

Expected: all integration tests pass, including the three new ones (multi-asset 201, empty-asset_identifiers 400, legacy-shape 400).

- [ ] **Step 2.9: Run full backend test suite**

Run: `just backend test ./...`

Expected: full suite green. Any unexpected fallout (e.g., another package consuming `inventory.SaveRequest` with the old field names) surfaces here.

- [ ] **Step 2.10: Commit**

```bash
git add backend/internal/handlers/inventory/public_write_integration_test.go
git commit -m "$(cat <<'EOF'
test(tra-533): lock in inventory/save regression coverage

Three new integration cases:
- Multi-asset → 201 (AC3)
- Empty asset_identifiers → 400 (AC3)
- Legacy {location_id, asset_ids} shape → 400 (AC2 canary)

Reworked numeric-ID happy paths to identifier shape; deleted the redundant
identifier-only happy path and the cross-field-disagree test (branch is gone).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Backend — regenerate OpenAPI specs

**Files:**
- Regenerate: `backend/docs/api/openapi.public.{json,yaml}` (committed).
- Regenerate: `backend/internal/handlers/swaggerspec/*` (gitignored).

### Steps

- [ ] **Step 3.1: Run the api-spec recipe**

Run: `just backend api-spec`

Expected: success message ending with `✅ Public spec: docs/api/openapi.public.{json,yaml} ...`. If the recipe fails complaining about a missing `frontend/dist` stub, follow whatever its error message says (this is a known requirement of `swag --parseDependency`).

- [ ] **Step 3.2: Inspect the OpenAPI diff**

Run: `git diff backend/docs/api/openapi.public.json backend/docs/api/openapi.public.yaml`

Expected diff:
- `inventory.SaveRequest` schema: `location_id` and `asset_ids` properties drop entirely.
- `inventory.SaveRequest` schema: `location_identifier` and `asset_identifiers` move into the `required` array (or get `required: true` on the property — depending on swag's output style).
- No other endpoints / schemas should change.

If anything else moves, investigate before committing — a stale annotation elsewhere in this PR's surface would be unexpected.

- [ ] **Step 3.3: Run validate to catch any spec drift CI checks**

Run: `just backend validate`

Expected: clean.

- [ ] **Step 3.4: Commit the regenerated spec**

```bash
git add backend/docs/api/openapi.public.json backend/docs/api/openapi.public.yaml
git commit -m "$(cat <<'EOF'
chore(tra-533): regenerate OpenAPI for inventory/save shape change

Public spec drops location_id/asset_ids properties from inventory.SaveRequest
and flips location_identifier/asset_identifiers to required.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Frontend — rename request type, update call site, update tests

**Files:**
- Modify: `frontend/src/lib/api/inventory.ts`
- Modify: `frontend/src/components/InventoryScreen.tsx`
- Modify: `frontend/src/hooks/inventory/useInventorySave.ts`
- Modify: `frontend/src/hooks/inventory/useInventorySave.test.ts`
- Modify: `frontend/src/components/__tests__/InventoryScreen.test.tsx`

### Steps

- [ ] **Step 4.1: Update `SaveInventoryRequest` type in `frontend/src/lib/api/inventory.ts`**

Replace the existing interface (lines 15-18):

```ts
export interface SaveInventoryRequest {
  location_identifier: string;
  asset_identifiers: string[];
}
```

`SaveInventoryResponse` (lines 23-28) stays unchanged — response-shape `location_id` is OOS per the spec.

- [ ] **Step 4.2: Run TypeScript compile to surface every break**

Run: `just frontend tsc` (or whatever the project's typecheck recipe is — check `just frontend --list`; `pnpm tsc --noEmit` is the fallback).

Expected: TypeScript errors at:
- `frontend/src/components/InventoryScreen.tsx` around line 248 (call site uses old `location_id`/`asset_ids` keys).
- `frontend/src/hooks/inventory/useInventorySave.ts` line 56-57 (warn log destructures old keys from `data`).
- `frontend/src/hooks/inventory/useInventorySave.test.ts` (fixture data uses old keys).
- `frontend/src/components/__tests__/InventoryScreen.test.tsx` (mock assertions on old keys).

- [ ] **Step 4.3: Update the construction site in `InventoryScreen.tsx` (lines 241-243)**

Replace:

```ts
    const saveableAssets = tags
      .filter(t => t.type === 'asset' && t.assetId)
      .map(t => t.assetId!);
```

with:

```ts
    const saveableAssetIdentifiers = tags
      .filter(t => t.type === 'asset' && t.assetIdentifier)
      .map(t => t.assetIdentifier!);
```

And update the early-return guard at line 245:

```ts
    if (saveableAssetIdentifiers.length === 0) return;
```

- [ ] **Step 4.4: Update the call site in `InventoryScreen.tsx` (lines 248-250)**

Replace:

```ts
      await save({
        location_id: resolvedLocation.id,
        asset_ids: saveableAssets,
      });
```

with:

```ts
      await save({
        location_identifier: resolvedLocation.identifier,
        asset_identifiers: saveableAssetIdentifiers,
      });
```

- [ ] **Step 4.5: Update the 403 warning log in `useInventorySave.ts` (lines 54-58)**

Replace:

```ts
          console.warn('[InventorySave] 403 from inventory/save', {
            detail,
            location_id: data.location_id,
            asset_ids_count: data.asset_ids.length,
          });
```

with:

```ts
          console.warn('[InventorySave] 403 from inventory/save', {
            detail,
            location_identifier: data.location_identifier,
            asset_identifiers_count: data.asset_identifiers.length,
          });
```

- [ ] **Step 4.6: Re-run typecheck to confirm clean**

Run: `just frontend tsc` (or `pnpm tsc --noEmit`).

Expected: clean. If any errors remain, they're in test files — proceed to Step 4.7.

- [ ] **Step 4.7: Update FE test fixtures**

In `frontend/src/hooks/inventory/useInventorySave.test.ts`, find each call to `save({...})` or fixture object typed as `SaveInventoryRequest` and replace `location_id: <number>` / `asset_ids: <number[]>` with `location_identifier: <string>` / `asset_identifiers: <string[]>`. Use placeholder values like `"WH-01"` and `["ASSET-0001"]` consistent with backend test fixtures.

In `frontend/src/components/__tests__/InventoryScreen.test.tsx`, find each `expect(saveMock).toHaveBeenCalledWith({...})` (or equivalent assertion) and update the expected payload shape. Also update any tag-store fixture data so `assetIdentifier` is populated alongside `assetId` for any tag that's intended to be saveable.

In `frontend/src/components/__tests__/InventoryScreen.authgate.test.tsx`, read the file. If it asserts payload shape, update; if it only checks the auth gate redirect, no change.

- [ ] **Step 4.8: Run frontend tests**

Run: `just frontend test`

Expected: all tests pass. If a test fails because it depends on a tag fixture without `assetIdentifier`, update the fixture. Do not weaken the test assertions to accommodate — the test was using surrogate IDs because the backend accepted them; now the test must use natural keys for the same reason.

- [ ] **Step 4.9: Run frontend lint**

Run: `just frontend lint`

Expected: clean.

- [ ] **Step 4.10: Commit**

```bash
git add frontend/src/lib/api/inventory.ts frontend/src/components/InventoryScreen.tsx frontend/src/hooks/inventory/useInventorySave.ts frontend/src/hooks/inventory/useInventorySave.test.ts frontend/src/components/__tests__/InventoryScreen.test.tsx
git commit -m "$(cat <<'EOF'
fix(tra-533): switch FE inventory/save to natural-key shape

SaveInventoryRequest renames location_id/asset_ids to location_identifier/
asset_identifiers. InventoryScreen builds asset_identifier strings from the
tag store (assetIdentifier is populated alongside assetId by enrichment).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If `InventoryScreen.authgate.test.tsx` was modified, include it in the `git add`.

---

## Task 5: Final verification, push, and PR

**Files:**
- No file changes; running verification commands and opening the PR.

### Steps

- [ ] **Step 5.1: Run `just validate` for combined lint + test across both workspaces**

Run: `just validate`

Expected: clean across both backend and frontend. Any failure here means earlier tasks left something broken — fix and recommit before continuing.

- [ ] **Step 5.2: Inspect the full diff**

Run: `git log --oneline main..HEAD` and `git diff main...HEAD --stat`

Expected: 4 commits (Task 1, Task 2, Task 3, Task 4) plus the design + plan commits already on the branch. Files touched should match the file map at the top of this plan.

- [ ] **Step 5.3: Push the branch**

Run: `git push -u origin fix/tra-533-inventory-save-fk-cleanup`

- [ ] **Step 5.4: Open the PR**

Run:

```bash
gh pr create --title "fix(tra-533): collapse inventory/save to natural-key shape, lock in regression tests" --body "$(cat <<'EOF'
## Summary

- Removes undocumented `_id`/`_ids` request aliases on `POST /api/v1/inventory/save` (AC2 — TRA-532 finding F10). Public surface now has one canonical request shape.
- Adds three regression test cases (AC3): multi-asset → 201, empty asset_identifiers → 400, and a legacy-shape rejection canary that locks in the alias removal.
- Documents that AC1 (find/fix all embedded SQL FK rename misses from TRA-524) was already complete in c678465; spec records the grep verification.
- Coordinated FE change: `SaveInventoryRequest` type renames its fields; `InventoryScreen` constructs asset-identifier strings from the tag store (which already carries `assetIdentifier` alongside `assetId`).

## BB11 repro outcomes

The TRA-533 ticket lists three BB11 payloads that returned 500. Post-this-PR:

1. `{location_identifier: "WHS-01", asset_identifiers: ["ASSET-0001"]}` → **201** ✓
2. `{location_identifier: "WHS-01", asset_identifiers: ["ASSET-0001", "ASSET-0002"]}` → **201** ✓
3. `{location_id: 542787020, asset_identifiers: ["ASSET-0001"]}` → **400** with `location_identifier required` field error ✓ (this inverts the original ticket's "should return 200" expectation — that text pre-dates the AC2 alias-removal decision; rejecting the legacy shape is the *correct* post-AC2 behavior).

## Out of scope (explicit)

- Response-shape `location_id` surrogate cleanup (C2 territory, separate ticket).
- Broader SQL-canary harness (skipped per brainstorm; no follow-up ticket).
- TRA-525 frontend identifier→tag UI work.

## Test plan

- [ ] `just validate` green locally.
- [ ] Preview deploy succeeds.
- [ ] Manual curl of the three BB11 payloads against `https://app.preview.trakrf.id` returns the outcomes listed above.
- [ ] Single Playwright e2e smoke against preview covering the inventory scan-and-save user flow passes.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: a PR URL is printed. Report it back.

- [ ] **Step 5.5: Wait for preview deploy and run the BB11 manual checks**

Once the preview deploy is green (watch GitHub Actions on the PR), curl the three payloads against the preview API. Use a valid API key bound to the org that has `WHS-01` and `ASSET-0001`/`ASSET-0002` fixtures.

```bash
PREVIEW_API="https://api.preview.trakrf.id/api/v1"
TOKEN="<api key with scans:write>"

# Repro 1: single-asset identifier shape — expect 201
curl -i -X POST "$PREVIEW_API/inventory/save" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"location_identifier": "WHS-01", "asset_identifiers": ["ASSET-0001"]}'

# Repro 2: multi-asset identifier shape — expect 201
curl -i -X POST "$PREVIEW_API/inventory/save" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"location_identifier": "WHS-01", "asset_identifiers": ["ASSET-0001", "ASSET-0002"]}'

# Repro 3: legacy surrogate-ID shape — expect 400 with location_identifier field error
curl -i -X POST "$PREVIEW_API/inventory/save" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"location_id": 542787020, "asset_identifiers": ["ASSET-0001"]}'
```

The exact preview API hostname may differ — check the project's actual preview URL (the user feedback memory notes `https://app.preview.trakrf.id` is the FE; the API host pattern is similar). If unsure, ask the user before guessing.

Report the actual outcomes back; do not claim success without the real curl output.

- [ ] **Step 5.6: Run a single Playwright smoke against preview**

If the project has a preview-targeted Playwright recipe (check `just frontend --list` for an `e2e:preview` or similar), run only the inventory scan-and-save spec. Avoid running the full e2e suite — this is a focused fix.

If no narrow recipe exists, ask the user before running the full suite.

Report the test outcome.

---

## Notes for the implementer

**Branch convention.** The worktree is already on `fix/tra-533-inventory-save-fk-cleanup`. Don't rename or rebase mid-task.

**Commit cadence.** One commit per task. If a task's verification fails, fix and recommit (don't amend) — project policy is new commits, not amends. See CLAUDE.md.

**No squash, no local merges.** PR uses merge commits. Don't merge to main locally.

**Honest test reports.** Project memory says "report actual test results — no false optimism." If a test is flaky, name it; don't paper over.

**Decode strictness.** The spec leaves whether to switch this handler to `DisallowUnknownFields` as an implementation-time judgment call. After Task 1 lands, run the legacy-shape rejection test (Task 2 Step 2.6) and inspect the actual response detail. If it's crisp ("location_identifier required"), no change needed. If the validator's auto-generated detail is awkward, consider switching this one handler to strict decoding for cleaner errors. Either way, the test passes — strictness only affects message clarity.
