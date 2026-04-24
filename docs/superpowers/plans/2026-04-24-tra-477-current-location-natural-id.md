# TRA-477: `current_location` natural identifier on asset create/update

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Accept `current_location` (string natural identifier) on asset create/update requests, deprecate `current_location_id` (integer surrogate) for API use while keeping it functional, and make `current_location` always present (possibly `null`) on single-resource GET responses like it already is on list responses.

**Architecture:** Mirror the `parent_identifier` pattern already implemented for locations (TRA-447). Add a new string field `current_location` to `CreateAssetRequest` and `UpdateAssetRequest`; resolve it in the handler before calling storage, converting to the existing `current_location_id` surrogate. For the response-shape fix, swap `getAssetWithLocationByID` to use the same `latest_scans` CTE pattern the list query uses, so the single-asset endpoint returns the scan-inferred location when `current_location_id` is NULL. Remove `omitempty` on `PublicAssetView.CurrentLocation` so the field is always serialized (as `null` when empty).

**Tech Stack:** Go (chi router, validator/v10, pgx), TimescaleDB/Postgres, YAML OpenAPI spec.

---

## Scope & non-goals

**In scope**
- Add `current_location` string field to both write requests.
- Resolve identifier → surrogate in handlers, validate agreement with `current_location_id` when both set.
- Single-asset GET returns scan-inferred location consistently with list GET.
- `current_location` always rendered (null-when-empty) on public responses.
- OpenAPI updated; integration tests for happy/sad paths.

**Out of scope**
- Removing `current_location_id` (breaking change).
- Changing how scan ingestion updates (or doesn't update) `assets.current_location_id`.
- Bulk import JSON/CSV shape.

---

## File map

| File | Change |
|---|---|
| `backend/internal/models/asset/asset.go` | Add `CurrentLocation *string` to `CreateAssetRequest` + `UpdateAssetRequest`. |
| `backend/internal/models/asset/public.go` | Drop `omitempty` on `PublicAssetView.CurrentLocation`. |
| `backend/internal/handlers/assets/assets.go` | Resolve `current_location` → `current_location_id` in `Create` and `doUpdateAsset`. |
| `backend/internal/storage/assets.go` | Rewrite `GetAssetByIdentifier` query to use `latest_scans` CTE like `ListAssetsFiltered`. |
| `docs/api/openapi.public.yaml` | Add `current_location` to request schemas; mark `current_location_id` deprecated; document response field always-present. |
| `backend/internal/handlers/assets/assets_integration_test.go` | Happy path, not-found, disagree, update, GET-single-with-scan-only cases. |

---

## Task 1 — Extend request DTOs

**Files:**
- Modify: `backend/internal/models/asset/asset.go`

- [ ] **Step 1: Add `CurrentLocation` to `CreateAssetRequest`**

In `backend/internal/models/asset/asset.go`, inside `CreateAssetRequest`, just after `CurrentLocationID`:

```go
	CurrentLocationID *int                 `json:"current_location_id,omitempty" swaggerignore:"true" validate:"omitempty,min=1"`
	CurrentLocation   *string              `json:"current_location,omitempty" validate:"omitempty,min=1,max=255"`
```

Note: we add `swaggerignore:"true"` to `CurrentLocationID` so the public OpenAPI hides the surrogate (same treatment locations gave `parent_location_id`). Internal callers (handlers) still see it.

- [ ] **Step 2: Add `CurrentLocation` to `UpdateAssetRequest`**

In the same file, inside `UpdateAssetRequest`, just after `CurrentLocationID`:

```go
	CurrentLocationID *int                 `json:"current_location_id" swaggerignore:"true"`
	CurrentLocation   *string              `json:"current_location,omitempty" validate:"omitempty,min=1,max=255"`
```

- [ ] **Step 3: Build**

Run: `just backend build`
Expected: compiles clean. No handler wiring yet — handlers still compile because we only added fields.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/models/asset/asset.go
git commit -m "feat(tra-477): add current_location field to asset write requests"
```

---

## Task 2 — Resolve `current_location` in Create handler (TDD)

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (Create, ~lines 57-108)
- Test: `backend/internal/handlers/assets/assets_integration_test.go`

- [ ] **Step 1: Write failing test — happy path create with `current_location`**

Locate or create an API-key integration test file similar to `locations/public_write_integration_test.go`. If assets has one, append; otherwise add `backend/internal/handlers/assets/public_write_integration_test.go` using the same scaffolding pattern (reuse `seedAssetOrgAndKey` equivalent; if none exists, inline the `testutil.CreateTestAccount` + direct router pattern from `assets_integration_test.go`). Add:

```go
func TestCreateAsset_CurrentLocation_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: accountID, Identifier: "TRA477-WHS", Name: "Warehouse",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	body := `{"identifier":"TRA477-A1","name":"Asset","current_location":"TRA477-WHS"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			Identifier      string  `json:"identifier"`
			CurrentLocation *string `json:"current_location"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.CurrentLocation)
	assert.Equal(t, "TRA477-WHS", *resp.Data.CurrentLocation)
	_ = loc
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `just backend test ./internal/handlers/assets/... -run TestCreateAsset_CurrentLocation_HappyPath`
Expected: FAIL with `400 bad_request "unknown field \"current_location\""` or validation error.

- [ ] **Step 3: Add resolution logic in Create handler**

In `backend/internal/handlers/assets/assets.go` `Create`, insert **before** `validate.Struct(request)` (i.e. between the defaults block ending at the `IsActive`/`ValidFrom` defaults and the validate call around line 88):

```go
	// Resolve current_location → current_location_id (TRA-477). Empty string
	// is treated as nil. Parallels the parent_identifier handling on locations.
	if request.CurrentLocation != nil && *request.CurrentLocation != "" {
		loc, err := handler.storage.GetLocationByIdentifier(r.Context(), orgID, *request.CurrentLocation)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				apierrors.AssetCreateFailed, err.Error(), requestID)
			return
		}
		if loc == nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.AssetCreateFailed,
				fmt.Sprintf("current_location %q not found", *request.CurrentLocation), requestID)
			return
		}
		if request.CurrentLocationID != nil && *request.CurrentLocationID != loc.ID {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.AssetCreateFailed,
				"current_location and current_location_id disagree", requestID)
			return
		}
		request.CurrentLocationID = &loc.ID
	}
```

(Imports: `fmt` is already imported; confirm.)

- [ ] **Step 4: Run happy path test — verify it passes**

Run: `just backend test ./internal/handlers/assets/... -run TestCreateAsset_CurrentLocation_HappyPath`
Expected: PASS.

- [ ] **Step 5: Add sad-path tests**

Append:

```go
func TestCreateAsset_CurrentLocation_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupTestRouter(NewHandler(store))
	body := `{"identifier":"TRA477-A2","name":"Asset","current_location":"DOES-NOT-EXIST"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not found")
}

func TestCreateAsset_CurrentLocation_Disagree(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: accountID, Identifier: "TRA477-WHS2", Name: "Warehouse 2",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	router := setupTestRouter(NewHandler(store))
	body := fmt.Sprintf(
		`{"identifier":"TRA477-A3","name":"Asset","current_location":"TRA477-WHS2","current_location_id":%d}`,
		loc.ID+99999,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "disagree")
}
```

- [ ] **Step 6: Run the new tests**

Run: `just backend test ./internal/handlers/assets/... -run TestCreateAsset_CurrentLocation`
Expected: all 3 PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/assets/assets_integration_test.go
git commit -m "feat(tra-477): resolve current_location on asset create"
```

---

## Task 3 — Resolve `current_location` in Update handler (TDD)

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (doUpdateAsset, ~lines 159-208)
- Test: `backend/internal/handlers/assets/assets_integration_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestUpdateAsset_CurrentLocation_HappyPath(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	_, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: accountID, Identifier: "TRA477-UPD", Name: "Updated Loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	createBody := `{"identifier":"TRA477-UA1","name":"Asset"}`
	router := setupTestRouter(NewHandler(store))
	w := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodPost, "/api/v1/assets", strings.NewReader(createBody))
	creq.Header.Set("Content-Type", "application/json")
	creq = withOrgContext(creq, accountID)
	router.ServeHTTP(w, creq)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	updateBody := `{"current_location":"TRA477-UPD"}`
	w2 := httptest.NewRecorder()
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/assets/TRA477-UA1", strings.NewReader(updateBody))
	ureq.Header.Set("Content-Type", "application/json")
	ureq = withOrgContext(ureq, accountID)
	router.ServeHTTP(w2, ureq)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	var resp struct {
		Data struct {
			CurrentLocation *string `json:"current_location"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.CurrentLocation)
	assert.Equal(t, "TRA477-UPD", *resp.Data.CurrentLocation)
}
```

- [ ] **Step 2: Run — verify FAIL**

Run: `just backend test ./internal/handlers/assets/... -run TestUpdateAsset_CurrentLocation_HappyPath`
Expected: FAIL (unknown field).

- [ ] **Step 3: Implement resolution in `doUpdateAsset`**

In `doUpdateAsset` (around line 185, after `validate.Struct`), insert:

```go
	// Resolve current_location → current_location_id (TRA-477).
	if request.CurrentLocation != nil && *request.CurrentLocation != "" {
		loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, *request.CurrentLocation)
		if err != nil {
			httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
				apierrors.AssetUpdateFailed, err.Error(), reqID)
			return
		}
		if loc == nil {
			httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.AssetUpdateFailed,
				fmt.Sprintf("current_location %q not found", *request.CurrentLocation), reqID)
			return
		}
		if request.CurrentLocationID != nil && *request.CurrentLocationID != loc.ID {
			httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.AssetUpdateFailed,
				"current_location and current_location_id disagree", reqID)
			return
		}
		request.CurrentLocationID = &loc.ID
	}
```

- [ ] **Step 4: Run — verify PASS**

Run: `just backend test ./internal/handlers/assets/... -run TestUpdateAsset_CurrentLocation_HappyPath`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/assets/assets_integration_test.go
git commit -m "feat(tra-477): resolve current_location on asset update"
```

---

## Task 4 — Always-present `current_location` on response

**Files:**
- Modify: `backend/internal/models/asset/public.go`

- [ ] **Step 1: Drop `omitempty`**

Change line 17 from:

```go
	CurrentLocation *string                `json:"current_location,omitempty"`
```

to:

```go
	CurrentLocation *string                `json:"current_location"`
```

- [ ] **Step 2: Verify existing tests still pass**

Run: `just backend test ./internal/handlers/assets/...`
Expected: PASS. If any test asserted field absence, tighten it to assert `null`. Update accordingly.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/models/asset/public.go
git commit -m "fix(tra-477): always emit current_location on asset responses"
```

---

## Task 5 — Single-asset GET reads scan-inferred location (TDD)

**Context:** `ListAssetsFiltered` (storage/assets.go:694) uses a `latest_scans` CTE to infer location. `GetAssetByIdentifier` (storage/assets.go:597) LEFT JOINs only on `a.current_location_id`. Swap the single-asset query to match.

**Files:**
- Modify: `backend/internal/storage/assets.go` — `GetAssetByIdentifier` and `getAssetWithLocationByID`
- Test: `backend/internal/storage/assets_integration_test.go` or `backend/internal/handlers/assets/assets_integration_test.go` (pick whichever houses scan-driven tests; default to handler-level end-to-end).

- [ ] **Step 1: Write failing test**

```go
func TestGetAsset_LocationInferredFromLatestScan(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: accountID, Identifier: "TRA477-SCAN", Name: "Scan Loc",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Create asset WITHOUT current_location_id.
	router := setupTestRouter(NewHandler(store))
	createBody := `{"identifier":"TRA477-SA1","name":"Scan Asset"}`
	creq := httptest.NewRequest(http.MethodPost, "/api/v1/assets", strings.NewReader(createBody))
	creq.Header.Set("Content-Type", "application/json")
	creq = withOrgContext(creq, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, creq)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created struct {
		Data struct {
			SurrogateID int `json:"surrogate_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// Insert a scan directly so assets.current_location_id stays NULL but
	// asset_scans has a row pointing at loc.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (org_id, asset_id, location_id, timestamp)
		VALUES ($1, $2, $3, NOW())
	`, accountID, created.Data.SurrogateID, loc.ID)
	require.NoError(t, err)

	// GET single-asset should now return current_location = "TRA477-SCAN".
	greq := httptest.NewRequest(http.MethodGet, "/api/v1/assets/TRA477-SA1", nil)
	greq = withOrgContext(greq, accountID)
	gw := httptest.NewRecorder()
	router.ServeHTTP(gw, greq)
	require.Equal(t, http.StatusOK, gw.Code, gw.Body.String())

	var resp struct {
		Data struct {
			CurrentLocation *string `json:"current_location"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.CurrentLocation, "current_location should be inferred from latest scan")
	assert.Equal(t, "TRA477-SCAN", *resp.Data.CurrentLocation)
}
```

Verify `asset_scans` columns (`org_id, asset_id, location_id, timestamp`) against the schema before running — adjust INSERT if the column set differs.

- [ ] **Step 2: Run — verify FAIL**

Run: `just backend test ./internal/handlers/assets/... -run TestGetAsset_LocationInferredFromLatestScan`
Expected: FAIL (`current_location` is nil because query ignores scans).

- [ ] **Step 3: Rewrite `GetAssetByIdentifier` to use `latest_scans`**

Replace the query in `GetAssetByIdentifier` (storage/assets.go:600-610) with the same CTE pattern used in `ListAssetsFiltered`:

```go
	query := `
		WITH latest_scans AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT
			a.id, a.org_id, a.identifier, a.name, a.type, a.description,
			COALESCE(a.current_location_id, ls.location_id),
			a.valid_from, a.valid_to, a.metadata,
			a.is_active, a.created_at, a.updated_at, a.deleted_at,
			l.identifier
		FROM trakrf.assets a
		LEFT JOIN latest_scans ls ON ls.asset_id = a.id
		LEFT JOIN trakrf.locations l
			ON l.id = COALESCE(a.current_location_id, ls.location_id)
			AND l.org_id = a.org_id AND l.deleted_at IS NULL
		WHERE a.org_id = $1 AND a.identifier = $2 AND a.deleted_at IS NULL
		LIMIT 1
	`
```

Keep the scan targets the same. Note the single-asset version uses `COALESCE(current_location_id, ls.location_id)` so explicit FK wins over scan-inferred when both exist — matching list semantics is the goal but `ListAssetsFiltered` currently uses scan-only. Decide: for this ticket, prefer COALESCE to avoid regressions when `current_location_id` was set explicitly.

- [ ] **Step 4: Apply the same treatment to `getAssetWithLocationByID` (storage/assets.go:514)**

This helper is called by Create and Update to fetch the enriched response. Mirror the CTE + COALESCE rewrite so the response immediately after create/update reflects a scan-inferred location.

Replace the query body (keep function signature, scan targets, identifier lookup) with the same CTE/COALESCE pattern, keyed on `a.id`.

- [ ] **Step 5: Run the new test — verify PASS**

Run: `just backend test ./internal/handlers/assets/... -run TestGetAsset_LocationInferredFromLatestScan`
Expected: PASS.

- [ ] **Step 6: Run all asset tests — verify no regressions**

Run: `just backend test ./internal/handlers/assets/... ./internal/storage/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/storage/assets.go backend/internal/handlers/assets/assets_integration_test.go
git commit -m "fix(tra-477): infer current_location from latest scan on single-asset GET"
```

---

## Task 6 — OpenAPI spec

**Files:**
- Modify: `docs/api/openapi.public.yaml`

- [ ] **Step 1: Update request schemas**

Find `asset.CreateAssetWithIdentifiersRequest` (lines ~20-58) and `asset.UpdateAssetRequest` (lines ~89-115) under `components.schemas`.

- Remove `current_location_id` from each schema's `properties` block (it's now `swaggerignore:"true"`).
- Add `current_location` property to each:

```yaml
        current_location:
          type: string
          description: Natural identifier of the asset's current location. Preferred over current_location_id.
          minLength: 1
          maxLength: 255
          example: WHS-01
```

- [ ] **Step 2: Update response schema note**

On `asset.PublicAssetView.properties.current_location`, drop any `nullable: false` / required-adjacent wording. Ensure description states the field is always present and may be `null`:

```yaml
        current_location:
          type: string
          nullable: true
          description: Natural identifier of the asset's current location (from explicit assignment or latest scan). Always present; null when the asset has never been scanned and no location was set.
          example: WHS-01
```

Remove `current_location` from the schema's `required:` array if it's listed there (the field is nullable, not mandatory).

- [ ] **Step 3: Regenerate swagger if the repo uses swag**

Run: `just backend docs` (or `just backend swagger` — check the justfile; skip if the yaml is hand-authored).

Check: `grep -n '"\?swag"\?' backend/justfile` — if absent, the YAML is hand-authored and no regeneration is needed.

- [ ] **Step 4: Validate spec**

Run: `just backend validate` (or the OpenAPI validation target if there is one).
Expected: spec valid.

- [ ] **Step 5: Commit**

```bash
git add docs/api/openapi.public.yaml
git commit -m "docs(tra-477): document current_location request field, deprecate current_location_id"
```

---

## Task 7 — Cross-cutting verification

- [ ] **Step 1: Run the full backend test suite**

Run: `just backend test`
Expected: all green.

- [ ] **Step 2: Run lint**

Run: `just lint`
Expected: clean.

- [ ] **Step 3: Smoke the original evidence**

Start the backend locally (`just backend dev` or equivalent), then:

```bash
# Get an API key / session per existing dev docs, then:
curl -s -X POST http://localhost:8080/api/v1/assets \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"identifier":"BB8-LOC-TEST3","name":"test","current_location":"WHS-01"}' | jq .
```

Expected: `201` with `"current_location": "WHS-01"` in the response. Re-GET the single asset and confirm `current_location` still present.

- [ ] **Step 4: Push and open PR**

```bash
git push -u origin miks2u/tra-477-accept-current_location-natural-identifier-on-asset
gh pr create --title "feat(tra-477): accept current_location natural identifier on asset create/update" \
  --body "$(cat <<'EOF'
## Summary
- Accept `current_location` (string natural identifier) on POST/PUT /api/v1/assets
- Deprecate `current_location_id` in the public OpenAPI (still works, hidden from docs)
- Single-asset GET now infers location from latest scan (consistent with list)
- `current_location` always present on public responses (null when unknown)

Closes TRA-477.

## Test plan
- [x] Added integration tests for happy-path / not-found / disagree on create
- [x] Added integration tests for update
- [x] Added regression test for scan-inferred location on GET single asset
- [x] `just backend test` green
- [x] `just lint` clean

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review notes

- Spec coverage: accept `current_location` (T1–T3), deprecate `current_location_id` in docs (T6), always-present response (T4), always present on single GET via scan inference (T5), OpenAPI (T6), integration tests (T2/T3/T5). All DoD items covered.
- No placeholders.
- Field name `current_location` used consistently across request/response/tests/docs.
- Type consistency: `CurrentLocation *string` in both request structs; response struct already had `CurrentLocation *string`.
