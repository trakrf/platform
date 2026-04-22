# TRA-448: Inventory Save Accepts Natural Identifiers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `POST /api/v1/inventory/save` accepts `location_identifier` (string) and `asset_identifiers` ([]string) so external API consumers stop being forced to use internal numeric surrogate IDs. Numeric `location_id` / `asset_ids` stay accepted for the existing UI flow.

**Architecture:** Mirror the TRA-447 `parent_identifier` pattern from `handlers/locations`: add new identifier fields with `validate:"omitempty,..."`, hide the legacy numeric fields from the public OpenAPI via `swaggerignore:"true"`, resolve identifiers → internal IDs in the handler before the storage call, and add a single batch SQL lookup for the asset case so we don't N+1.

**Tech Stack:** Go 1.24, chi/v5, go-playground/validator, pgx/v5, swaggo (`swag init`).

---

## File Map

| File | Status | Responsibility |
| --- | --- | --- |
| `backend/internal/storage/assets.go` | modify | Add `GetAssetIDsByIdentifiers` batch resolver. |
| `backend/internal/storage/assets_test.go` | modify | Integration test for batch resolver (happy path, missing identifier, cross-org filter). |
| `backend/internal/handlers/inventory/save.go` | modify | Add `LocationIdentifier`, `AssetIdentifiers` to `SaveRequest`; loosen numeric validators to `omitempty`; resolve identifiers to numeric IDs; cross-field "at least one of each" check. |
| `backend/internal/handlers/inventory/save_test.go` | modify | Unit tests for new validation rules and resolution paths (with `mockInventoryStorage` extended). |
| `backend/internal/handlers/inventory/public_write_integration_test.go` | modify | End-to-end integration tests: identifier happy path, missing identifier 400, mixed-and-disagree 400. |
| `backend/internal/apierrors/messages.go` | modify | Add error message constant for missing identifier resolution if needed. |
| `backend/docs/swagger.json` / `backend/docs/swagger.yaml` | regen | Swag-generated. |
| `backend/internal/handlers/swaggerspec/openapi.public.{json,yaml}` | regen | Embedded public spec. |
| `backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}` | regen | Embedded internal spec. |
| `docs/api/openapi.public.{json,yaml}` | regen | Committed public spec used by docs site. |

No frontend changes — UI keeps using numeric `location_id` / `asset_ids` (out of scope per DoD).

---

## Design Notes

**Cross-field rules (matches TRA-447 spirit, simplified for the asset list):**

- `location_identifier` and `location_id`:
  - At least one must be provided.
  - If both provided: resolve `location_identifier`, error 400 if the resolved ID does not match `location_id` (`"location_identifier and location_id disagree"`). This mirrors TRA-447 exactly.
- `asset_identifiers` and `asset_ids`:
  - At least one must be non-empty.
  - If both non-empty: error 400 (`"specify either asset_identifiers or asset_ids, not both"`). Set-comparison after resolution is messy and the UI never sends both.
- `location_identifier` is resolved with the existing `GetLocationByIdentifier`; not-found → 400 (`parent_identifier %q not found`-style message but for `location_identifier`).
- `asset_identifiers` is resolved with a new `GetAssetIDsByIdentifiers` batch lookup. If any input identifier is absent from the result map → 400 listing the first missing identifier (deterministic for tests).

**`SaveRequest` final shape:**

```go
type SaveRequest struct {
    LocationID         int      `json:"location_id,omitempty" swaggerignore:"true" validate:"omitempty,min=1"`
    LocationIdentifier *string  `json:"location_identifier,omitempty" validate:"omitempty,min=1,max=255" example:"WH-01"`
    AssetIDs           []int    `json:"asset_ids,omitempty" swaggerignore:"true" validate:"omitempty,min=1,dive,min=1"`
    AssetIdentifiers   []string `json:"asset_identifiers,omitempty" validate:"omitempty,min=1,dive,min=1,max=255" example:"ASSET-0001"`
}
```

The struct-tag validator can't express "exactly one of A/B"; the cross-field check lives in the handler immediately after `validate.Struct`. Keep the existing `DecodeJSON` (not strict) — strict decode is out of scope and would be a behavior change for any existing client sending extra fields.

**Storage interface change:** `InventoryStorage` in `save.go` gains one method, `GetAssetIDsByIdentifiers`. Update `mockInventoryStorage` accordingly.

**OpenAPI docs:** With `swaggerignore:"true"` on the legacy numeric fields, the regenerated public OpenAPI spec will only show the identifier-based shape. The internal spec keeps both. This matches what TRA-447 did for `parent_location_id`.

---

## Task 1: Branch + worktree setup

- [ ] **Step 1: Create worktree on feature branch**

Run from project root:

```bash
git -C /home/mike/platform fetch origin
git -C /home/mike/platform worktree add .worktrees/tra-448 -b miks2u/tra-448-inventorysave-requires-numeric-ids-contradicts-public-api origin/main
cd /home/mike/platform/.worktrees/tra-448
```

Expected: worktree created, branch checked out tracking origin/main.

- [ ] **Step 2: Verify clean tree**

Run: `git status --short`
Expected: empty output.

---

## Task 2: Add `GetAssetIDsByIdentifiers` batch resolver in storage

**Files:**
- Modify: `backend/internal/storage/assets.go` (add function near `GetAssetByIdentifier` around line 577)
- Modify: `backend/internal/storage/assets_test.go` (add integration test in the `_test.go` file — check whether existing assets_test.go uses the `//go:build integration` tag; if not, place the test in a new file `backend/internal/storage/assets_identifier_resolve_integration_test.go` with the build tag)

- [ ] **Step 1: Confirm correct test file**

Run: `head -3 backend/internal/storage/assets_test.go`
If file starts with `//go:build integration`, add the test to it.
Otherwise create `backend/internal/storage/assets_identifier_resolve_integration_test.go` with:

```go
//go:build integration
// +build integration

package storage_test
```

For the rest of this task, the test file is referred to as `<TEST_FILE>`.

- [ ] **Step 2: Write failing integration test for batch resolver**

Add to `<TEST_FILE>`:

```go
func TestGetAssetIDsByIdentifiers_HappyPath(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID := testutil.CreateTestAccount(t, pool)

    a1, err := store.CreateAsset(context.Background(), assetmodel.Asset{
        OrgID: orgID, Identifier: "tra448-a1", Name: "A1", Type: "asset",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)
    a2, err := store.CreateAsset(context.Background(), assetmodel.Asset{
        OrgID: orgID, Identifier: "tra448-a2", Name: "A2", Type: "asset",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    got, err := store.GetAssetIDsByIdentifiers(context.Background(), orgID,
        []string{"tra448-a1", "tra448-a2"})
    require.NoError(t, err)
    require.Equal(t, map[string]int{
        "tra448-a1": a1.ID,
        "tra448-a2": a2.ID,
    }, got)
}

func TestGetAssetIDsByIdentifiers_MissingIdentifierAbsent(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID := testutil.CreateTestAccount(t, pool)
    a, err := store.CreateAsset(context.Background(), assetmodel.Asset{
        OrgID: orgID, Identifier: "tra448-present", Name: "A", Type: "asset",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    got, err := store.GetAssetIDsByIdentifiers(context.Background(), orgID,
        []string{"tra448-present", "tra448-ghost"})
    require.NoError(t, err)
    require.Equal(t, map[string]int{"tra448-present": a.ID}, got)
}

func TestGetAssetIDsByIdentifiers_OtherOrgFilteredOut(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgA := testutil.CreateTestAccount(t, pool)
    var orgB int
    require.NoError(t, pool.QueryRow(context.Background(),
        `INSERT INTO trakrf.organizations (name, identifier, is_active)
         VALUES ('tra448-orgB','tra448-orgB',true) RETURNING id`).Scan(&orgB))

    _, err := store.CreateAsset(context.Background(), assetmodel.Asset{
        OrgID: orgB, Identifier: "tra448-shared", Name: "A", Type: "asset",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    got, err := store.GetAssetIDsByIdentifiers(context.Background(), orgA,
        []string{"tra448-shared"})
    require.NoError(t, err)
    require.Empty(t, got, "asset belonging to orgB must not surface for orgA")
}

func TestGetAssetIDsByIdentifiers_EmptyInput(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()

    got, err := store.GetAssetIDsByIdentifiers(context.Background(), 1, nil)
    require.NoError(t, err)
    require.Empty(t, got)

    got, err = store.GetAssetIDsByIdentifiers(context.Background(), 1, []string{})
    require.NoError(t, err)
    require.Empty(t, got)
}
```

If new file, also add the imports:

```go
import (
    "context"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/require"
    assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
    "github.com/trakrf/platform/backend/internal/testutil"
)
```

- [ ] **Step 3: Run tests, expect FAIL**

Run: `just backend test-integration -run TestGetAssetIDsByIdentifiers`
Expected: compile error (`store.GetAssetIDsByIdentifiers undefined`).

- [ ] **Step 4: Implement the resolver**

Add to `backend/internal/storage/assets.go` directly after `GetAssetByIdentifier` (around line 620):

```go
// GetAssetIDsByIdentifiers resolves a batch of natural identifiers to internal
// surrogate IDs for one org. Returns a map keyed by identifier; identifiers not
// found in the org are absent from the map. Empty/nil input returns an empty
// map without querying.
//
// Used by inventory/save (TRA-448) to convert public-API identifier lists to
// the numeric IDs the storage layer expects.
func (s *Storage) GetAssetIDsByIdentifiers(
    ctx context.Context, orgID int, identifiers []string,
) (map[string]int, error) {
    if len(identifiers) == 0 {
        return map[string]int{}, nil
    }

    const query = `
        SELECT identifier, id
        FROM trakrf.assets
        WHERE org_id = $1 AND identifier = ANY($2) AND deleted_at IS NULL
    `
    rows, err := s.pool.Query(ctx, query, orgID, identifiers)
    if err != nil {
        return nil, fmt.Errorf("get asset ids by identifiers: %w", err)
    }
    defer rows.Close()

    out := make(map[string]int, len(identifiers))
    for rows.Next() {
        var (
            ident string
            id    int
        )
        if err := rows.Scan(&ident, &id); err != nil {
            return nil, fmt.Errorf("scan asset identifier row: %w", err)
        }
        out[ident] = id
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("iterate asset identifier rows: %w", err)
    }
    return out, nil
}
```

- [ ] **Step 5: Run tests, expect PASS**

Run: `just backend test-integration -run TestGetAssetIDsByIdentifiers`
Expected: 4 PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/assets.go backend/internal/storage/assets_test.go backend/internal/storage/assets_identifier_resolve_integration_test.go 2>/dev/null
git commit -m "feat(tra-448): add GetAssetIDsByIdentifiers batch resolver

Resolves natural asset identifiers to internal surrogate IDs in a single
query, scoped to one org. Missing identifiers are absent from the result
map; caller decides how to surface that to the user.

Used by POST /api/v1/inventory/save to accept asset_identifiers."
```

---

## Task 3: Extend `SaveRequest`, loosen validators, update mock

**Files:**
- Modify: `backend/internal/handlers/inventory/save.go` (`SaveRequest` struct around line 42, `InventoryStorage` interface around line 25)
- Modify: `backend/internal/handlers/inventory/save_test.go` (`mockInventoryStorage` around line 22)

- [ ] **Step 1: Write failing unit test for new struct shape and validator behavior**

Add to `save_test.go` (in the `TestSaveRequest_Validation` table, after the existing cases):

```go
{
    name: "identifier-only request validates",
    request: SaveRequest{
        LocationIdentifier: ptr("WH-01"),
        AssetIdentifiers:   []string{"ASSET-0001"},
    },
    wantErr: false,
},
{
    name: "all empty fails (legacy required-style still triggers somewhere)",
    request: SaveRequest{},
    wantErr: false, // struct-level validator no longer rejects; cross-field check is in handler
},
{
    name: "asset_identifiers with empty string element fails",
    request: SaveRequest{
        LocationIdentifier: ptr("WH-01"),
        AssetIdentifiers:   []string{""},
    },
    wantErr: true,
},
```

And add a small helper at the bottom of the file (skip if already present):

```go
func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 2: Run tests, expect FAIL**

Run: `just backend test -run TestSaveRequest_Validation`
Expected: compile error (`LocationIdentifier`, `AssetIdentifiers` undefined).

- [ ] **Step 3: Modify `SaveRequest` and `InventoryStorage` interface**

In `backend/internal/handlers/inventory/save.go`, replace the existing `SaveRequest` (lines 41–45) with:

```go
// SaveRequest is the request body for POST /api/v1/inventory/save.
//
// External API consumers should use location_identifier and asset_identifiers.
// The numeric location_id / asset_ids are accepted for backward compatibility
// with the UI (which already has surrogate IDs in client state) and are hidden
// from the public OpenAPI spec via swaggerignore.
//
// At least one of (location_id, location_identifier) and one of (asset_ids,
// asset_identifiers) must be provided. See Save handler for cross-field rules.
type SaveRequest struct {
    LocationID         int      `json:"location_id,omitempty" swaggerignore:"true" validate:"omitempty,min=1"`
    LocationIdentifier *string  `json:"location_identifier,omitempty" validate:"omitempty,min=1,max=255" example:"WH-01"`
    AssetIDs           []int    `json:"asset_ids,omitempty" swaggerignore:"true" validate:"omitempty,min=1,dive,min=1"`
    AssetIdentifiers   []string `json:"asset_identifiers,omitempty" validate:"omitempty,min=1,dive,min=1,max=255" example:"ASSET-0001"`
}
```

Replace `InventoryStorage` (lines 25–27) with:

```go
// InventoryStorage defines the storage operations needed by the inventory handler.
type InventoryStorage interface {
    SaveInventoryScans(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error)
    GetLocationByIdentifier(ctx context.Context, orgID int, identifier string) (*location.Location, error)
    GetAssetIDsByIdentifiers(ctx context.Context, orgID int, identifiers []string) (map[string]int, error)
}
```

Add the `location` model import at the top of the file:

```go
"github.com/trakrf/platform/backend/internal/models/location"
```

- [ ] **Step 4: Extend `mockInventoryStorage`**

In `save_test.go`, replace the `mockInventoryStorage` struct + method block (around lines 22–30) with:

```go
type mockInventoryStorage struct {
    saveResult *storage.SaveInventoryResult
    saveError  error

    // Identifier resolution stubs.
    locationByIdentifier      map[string]*location.Location
    locationByIdentifierError error

    assetIDsByIdentifiers      map[string]int
    assetIDsByIdentifiersError error
}

func (m *mockInventoryStorage) SaveInventoryScans(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error) {
    return m.saveResult, m.saveError
}

func (m *mockInventoryStorage) GetLocationByIdentifier(ctx context.Context, orgID int, identifier string) (*location.Location, error) {
    if m.locationByIdentifierError != nil {
        return nil, m.locationByIdentifierError
    }
    return m.locationByIdentifier[identifier], nil
}

func (m *mockInventoryStorage) GetAssetIDsByIdentifiers(ctx context.Context, orgID int, identifiers []string) (map[string]int, error) {
    if m.assetIDsByIdentifiersError != nil {
        return nil, m.assetIDsByIdentifiersError
    }
    out := make(map[string]int, len(identifiers))
    for _, id := range identifiers {
        if v, ok := m.assetIDsByIdentifiers[id]; ok {
            out[id] = v
        }
    }
    return out, nil
}
```

Add to the imports in `save_test.go`:

```go
"github.com/trakrf/platform/backend/internal/models/location"
```

- [ ] **Step 5: Run tests, expect PASS for the new validation cases (existing ones may now FAIL)**

Run: `just backend test ./internal/handlers/inventory/...`
Expected: `TestSaveRequest_Validation` cases pass; the existing `TestSave_MissingLocationID` and `TestInventorySave_BadBody_FieldsEnvelope` will FAIL because `location_id`/`asset_ids` are no longer `required`. That's expected — we fix them in Task 4.

- [ ] **Step 6: Commit (red on the cross-field tests is intentional)**

```bash
git add backend/internal/handlers/inventory/save.go backend/internal/handlers/inventory/save_test.go
git commit -m "refactor(tra-448): broaden SaveRequest schema and storage interface

Add LocationIdentifier and AssetIdentifiers fields. Demote numeric
LocationID/AssetIDs validators to omitempty (legacy fields stay accepted
but are hidden from the public OpenAPI spec). Extend InventoryStorage
with the two identifier-resolution methods used by the handler.

Cross-field 'one-of' enforcement and resolution land in the next commit."
```

---

## Task 4: Resolve identifiers and enforce cross-field rules in handler

**Files:**
- Modify: `backend/internal/handlers/inventory/save.go` (`Save` function lines 66–115)
- Modify: `backend/internal/handlers/inventory/save_test.go` (rewrite the legacy "missing/invalid" tests + add new resolution tests)

- [ ] **Step 1: Write failing unit tests for resolution + cross-field rules**

Append to `save_test.go`:

```go
func TestSave_RequiresAtLeastOneLocationField(t *testing.T) {
    handler := NewHandler(&mockInventoryStorage{})
    req := newTestRequest(t, map[string]any{"asset_ids": []int{1}}, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "location_identifier")
}

func TestSave_RequiresAtLeastOneAssetField(t *testing.T) {
    handler := NewHandler(&mockInventoryStorage{})
    req := newTestRequest(t, map[string]any{"location_id": 1}, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "asset_identifiers")
}

func TestSave_BothAssetFieldsPresent_Rejected(t *testing.T) {
    handler := NewHandler(&mockInventoryStorage{})
    body := map[string]any{
        "location_id":       1,
        "asset_ids":         []int{1, 2},
        "asset_identifiers": []string{"ASSET-0001"},
    }
    req := newTestRequest(t, body, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "not both")
}

func TestSave_LocationFieldsDisagree_Rejected(t *testing.T) {
    mock := &mockInventoryStorage{
        locationByIdentifier: map[string]*location.Location{
            "WH-01": {ID: 42, Identifier: "WH-01"},
        },
    }
    handler := NewHandler(mock)
    body := map[string]any{
        "location_id":         99, // doesn't match resolved 42
        "location_identifier": "WH-01",
        "asset_ids":           []int{1},
    }
    req := newTestRequest(t, body, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "disagree")
}

func TestSave_LocationIdentifierNotFound_Rejected(t *testing.T) {
    mock := &mockInventoryStorage{
        locationByIdentifier: map[string]*location.Location{}, // ghost
    }
    handler := NewHandler(mock)
    body := map[string]any{
        "location_identifier": "ghost",
        "asset_ids":           []int{1},
    }
    req := newTestRequest(t, body, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "ghost")
}

func TestSave_AssetIdentifierNotFound_Rejected(t *testing.T) {
    mock := &mockInventoryStorage{
        locationByIdentifier: map[string]*location.Location{
            "WH-01": {ID: 42, Identifier: "WH-01"},
        },
        assetIDsByIdentifiers: map[string]int{
            "ASSET-1": 7,
            // "ASSET-GHOST" intentionally absent
        },
    }
    handler := NewHandler(mock)
    body := map[string]any{
        "location_identifier": "WH-01",
        "asset_identifiers":   []string{"ASSET-1", "ASSET-GHOST"},
    }
    req := newTestRequest(t, body, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "ASSET-GHOST")
}

func TestSave_IdentifierHappyPath_ResolvesAndSucceeds(t *testing.T) {
    ts := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
    mock := &mockInventoryStorage{
        saveResult: &storage.SaveInventoryResult{
            Count:        2, LocationID: 42, LocationName: "WH-01", Timestamp: ts,
        },
        locationByIdentifier: map[string]*location.Location{
            "WH-01": {ID: 42, Identifier: "WH-01", Name: "WH-01"},
        },
        assetIDsByIdentifiers: map[string]int{
            "ASSET-1": 7,
            "ASSET-2": 8,
        },
    }
    handler := NewHandler(mock)
    body := map[string]any{
        "location_identifier": "WH-01",
        "asset_identifiers":   []string{"ASSET-1", "ASSET-2"},
    }
    req := newTestRequest(t, body, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
    var resp struct {
        Data storage.SaveInventoryResult `json:"data"`
    }
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    assert.Equal(t, 2, resp.Data.Count)
    assert.Equal(t, 42, resp.Data.LocationID)
}
```

Also rewrite the existing legacy tests to match the new behavior:

Replace `TestSave_MissingLocationID` (lines 99–135) with:

```go
func TestSave_NeitherLocationFieldProvided(t *testing.T) {
    handler := NewHandler(&mockInventoryStorage{})

    body := map[string]any{
        "asset_ids": []int{100, 101},
    }
    req := newTestRequest(t, body, 1)
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code)
    var response struct {
        Error struct {
            Type   string `json:"type"`
            Detail string `json:"detail"`
        } `json:"error"`
    }
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
    assert.Equal(t, "bad_request", response.Error.Type)
    assert.Contains(t, response.Error.Detail, "location_identifier")
}
```

Replace `TestInventorySave_BadBody_FieldsEnvelope` (lines 391–435) with a version asserting the cross-field error replaces the previous `fields[]` validation envelope:

```go
func TestInventorySave_BadBody_CrossFieldEnvelope(t *testing.T) {
    orgID := 1
    claims := &jwt.Claims{UserID: 1, Email: "test@example.com", CurrentOrgID: &orgID}
    req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewReader([]byte("{}")))
    req.Header.Set("Content-Type", "application/json")
    ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
    req = req.WithContext(ctx)

    handler := NewHandler(&mockInventoryStorage{})
    w := httptest.NewRecorder()
    handler.Save(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    var body struct {
        Error struct {
            Type   string `json:"type"`
            Detail string `json:"detail"`
        } `json:"error"`
    }
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
    assert.Equal(t, "bad_request", body.Error.Type)
    assert.Contains(t, body.Error.Detail, "location_identifier")
}
```

Update `TestSaveRequest_Validation` table-test "missing location_id" and "empty asset_ids" cases to `wantErr: false` (struct validator no longer rejects these; the handler does).

- [ ] **Step 2: Run tests, expect FAIL**

Run: `just backend test ./internal/handlers/inventory/...`
Expected: new tests fail with the current handler still rejecting on validate-tag.

- [ ] **Step 3: Update `Save` handler**

In `backend/internal/handlers/inventory/save.go`, replace the body of `Save` (lines 66–115) with:

```go
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
    requestID := middleware.GetRequestID(r.Context())

    orgID, err := middleware.GetRequestOrgID(r)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            apierrors.InventorySaveFailed, "missing organization context", requestID)
        return
    }

    var request SaveRequest
    if err := httputil.DecodeJSON(r, &request); err != nil {
        httputil.RespondDecodeError(w, r, err, requestID)
        return
    }

    if err := validate.Struct(request); err != nil {
        httputil.RespondValidationError(w, r, err, requestID)
        return
    }

    // Cross-field: at least one of (location_id, location_identifier).
    hasLocID := request.LocationID > 0
    hasLocIdent := request.LocationIdentifier != nil && *request.LocationIdentifier != ""
    if !hasLocID && !hasLocIdent {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.InventorySaveFailed,
            "location_identifier or location_id is required", requestID)
        return
    }

    // Cross-field: at least one of (asset_ids, asset_identifiers).
    hasAssetIDs := len(request.AssetIDs) > 0
    hasAssetIdents := len(request.AssetIdentifiers) > 0
    if !hasAssetIDs && !hasAssetIdents {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.InventorySaveFailed,
            "asset_identifiers or asset_ids is required", requestID)
        return
    }
    if hasAssetIDs && hasAssetIdents {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.InventorySaveFailed,
            "specify either asset_identifiers or asset_ids, not both", requestID)
        return
    }

    // Resolve location_identifier → numeric.
    locationID := request.LocationID
    if hasLocIdent {
        loc, err := h.storage.GetLocationByIdentifier(r.Context(), orgID, *request.LocationIdentifier)
        if err != nil {
            httputil.RespondStorageError(w, r, err, requestID)
            return
        }
        if loc == nil {
            httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
                apierrors.InventorySaveFailed,
                fmt.Sprintf("location_identifier %q not found", *request.LocationIdentifier), requestID)
            return
        }
        if hasLocID && request.LocationID != loc.ID {
            httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
                apierrors.InventorySaveFailed,
                "location_identifier and location_id disagree", requestID)
            return
        }
        locationID = loc.ID
    }

    // Resolve asset_identifiers → numeric IDs (one query).
    assetIDs := request.AssetIDs
    if hasAssetIdents {
        resolved, err := h.storage.GetAssetIDsByIdentifiers(r.Context(), orgID, request.AssetIdentifiers)
        if err != nil {
            httputil.RespondStorageError(w, r, err, requestID)
            return
        }
        ids := make([]int, 0, len(request.AssetIdentifiers))
        var missing []string
        for _, ident := range request.AssetIdentifiers {
            if id, ok := resolved[ident]; ok {
                ids = append(ids, id)
            } else {
                missing = append(missing, ident)
            }
        }
        if len(missing) > 0 {
            httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
                apierrors.InventorySaveFailed,
                fmt.Sprintf("asset_identifier(s) not found: %s", strings.Join(missing, ", ")), requestID)
            return
        }
        assetIDs = ids
    }

    result, err := h.storage.SaveInventoryScans(r.Context(), orgID, storage.SaveInventoryRequest{
        LocationID: locationID,
        AssetIDs:   assetIDs,
    })

    if err != nil {
        errStr := err.Error()
        if strings.Contains(errStr, "not found or access denied") {
            logger.Get().Warn().
                Int("org_id", orgID).
                Int("location_id", locationID).
                Ints("asset_ids", assetIDs).
                Str("request_id", requestID).
                Str("error", errStr).
                Msg("Inventory save denied: org context mismatch")

            httputil.WriteJSONError(w, r, http.StatusForbidden, modelerrors.ErrForbidden,
                apierrors.InventorySaveForbidden, errStr, requestID)
            return
        }
        httputil.RespondStorageError(w, r, err, requestID)
        return
    }

    httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}
```

Add `"fmt"` to the imports if not already present (it isn't — add it).

- [ ] **Step 4: Run tests**

Run: `just backend test ./internal/handlers/inventory/...`
Expected: all unit tests PASS, including the new identifier resolution tests and the rewritten "neither field provided" test.

- [ ] **Step 5: Run full backend unit suite to catch fallout**

Run: `just backend test`
Expected: PASS. If any other handler tests broke, they likely depended on the `mockInventoryStorage` shape — fix them.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/inventory/save.go backend/internal/handlers/inventory/save_test.go
git commit -m "feat(tra-448): resolve identifier-based inventory save

POST /api/v1/inventory/save now accepts location_identifier and
asset_identifiers. The handler resolves each to its internal surrogate
ID before delegating to the storage layer.

Cross-field rules:
  - at least one of (location_id, location_identifier) required
  - at least one of (asset_ids, asset_identifiers) required
  - asset_ids and asset_identifiers cannot both be set
  - if both location fields are set, the resolved IDs must agree
  - missing identifier(s) yield 400 with the offending value(s)

Numeric location_id/asset_ids stay accepted for the existing UI flow but
are now hidden from the public OpenAPI surface (regenerated in a later
commit) so external API consumers see only the identifier-based shape."
```

---

## Task 5: End-to-end integration tests

**Files:**
- Modify: `backend/internal/handlers/inventory/public_write_integration_test.go`

- [ ] **Step 1: Write failing identifier integration tests**

Append to `public_write_integration_test.go`:

```go
func TestInventorySave_APIKey_Identifiers_HappyPath(t *testing.T) {
    t.Setenv("JWT_SECRET", "pub-inv-ident-happy")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

    _, err := store.CreateLocation(context.Background(), locmodel.Location{
        OrgID: orgID, Identifier: "tra448-wh", Name: "WH", Path: "tra448-wh",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    _, err = store.CreateAsset(context.Background(), assetmodel.Asset{
        OrgID: orgID, Identifier: "tra448-asset", Name: "A", Type: "asset",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    r := buildInventoryPublicWriteRouter(store)

    body := `{"location_identifier":"tra448-wh","asset_identifiers":["tra448-asset"]}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

    var resp map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    data := resp["data"].(map[string]any)
    assert.Equal(t, float64(1), data["count"])
    assert.Equal(t, "WH", data["location_name"])
}

func TestInventorySave_APIKey_LocationIdentifierNotFound(t *testing.T) {
    t.Setenv("JWT_SECRET", "pub-inv-ident-loc-404")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})
    _, err := store.CreateAsset(context.Background(), assetmodel.Asset{
        OrgID: orgID, Identifier: "tra448-asset-2", Name: "A", Type: "asset",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    r := buildInventoryPublicWriteRouter(store)
    body := `{"location_identifier":"ghost-wh","asset_identifiers":["tra448-asset-2"]}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "ghost-wh")
}

func TestInventorySave_APIKey_AssetIdentifierNotFound(t *testing.T) {
    t.Setenv("JWT_SECRET", "pub-inv-ident-asset-404")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})
    _, err := store.CreateLocation(context.Background(), locmodel.Location{
        OrgID: orgID, Identifier: "tra448-wh-2", Name: "WH", Path: "tra448-wh-2",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    r := buildInventoryPublicWriteRouter(store)
    body := `{"location_identifier":"tra448-wh-2","asset_identifiers":["ghost-asset"]}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "ghost-asset")
}

func TestInventorySave_APIKey_LocationFieldsDisagree(t *testing.T) {
    t.Setenv("JWT_SECRET", "pub-inv-ident-disagree")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})
    loc, err := store.CreateLocation(context.Background(), locmodel.Location{
        OrgID: orgID, Identifier: "tra448-wh-d", Name: "WH", Path: "tra448-wh-d",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)
    asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
        OrgID: orgID, Identifier: "tra448-asset-d", Name: "A", Type: "asset",
        ValidFrom: time.Now(), IsActive: true,
    })
    require.NoError(t, err)

    r := buildInventoryPublicWriteRouter(store)
    bogus := loc.ID + 9999
    body := fmt.Sprintf(`{"location_identifier":"tra448-wh-d","location_id":%d,"asset_ids":[%d]}`, bogus, asset.ID)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "disagree")
}

func TestInventorySave_APIKey_BothAssetFields_Rejected(t *testing.T) {
    t.Setenv("JWT_SECRET", "pub-inv-ident-both-assets")
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    pool := store.Pool().(*pgxpool.Pool)

    _, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

    r := buildInventoryPublicWriteRouter(store)
    body := `{"location_id":1,"asset_ids":[1],"asset_identifiers":["x"]}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
    assert.Contains(t, w.Body.String(), "not both")
}
```

- [ ] **Step 2: Run, expect PASS** (handler is already implemented from Task 4)

Run: `just backend test-integration -run TestInventorySave_APIKey_Identifiers -run TestInventorySave_APIKey_LocationIdentifier -run TestInventorySave_APIKey_AssetIdentifier -run TestInventorySave_APIKey_LocationFieldsDisagree -run TestInventorySave_APIKey_BothAssetFields`
Expected: 5 PASS.

- [ ] **Step 3: Run the full inventory integration suite to confirm legacy numeric path still works**

Run: `just backend test-integration -run TestInventorySave_`
Expected: all original tests (`HappyPath`, `WrongScope_Returns403`, `SessionAuth_HappyPath`, `CrossOrg_Returns403`) plus the 5 new ones PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/inventory/public_write_integration_test.go
git commit -m "test(tra-448): inventory save identifier-based contract

Integration coverage for the new identifier-based POST /inventory/save:
happy path, missing location_identifier, missing asset_identifier,
location_id+identifier disagreement, and both-asset-field rejection.
Confirms the legacy numeric path is unaffected."
```

---

## Task 6: Regenerate OpenAPI specs

**Files:**
- Regen: `backend/docs/swagger.{json,yaml}`, `backend/internal/handlers/swaggerspec/openapi.{public,internal}.{json,yaml}`, `docs/api/openapi.public.{json,yaml}`

- [ ] **Step 1: Regenerate**

Run: `just backend api-spec`
Expected: success message, no errors.

- [ ] **Step 2: Verify the public OpenAPI now shows identifier fields and hides numeric fields**

Run: `grep -A 20 "inventory.SaveRequest" docs/api/openapi.public.yaml | head -40`
Expected output should contain:
- `location_identifier` (string, with `WH-01` example)
- `asset_identifiers` (array of strings)
- NO `location_id` or `asset_ids` (those should appear only in `swaggerspec/openapi.internal.yaml`)

Run: `grep -A 20 "inventory.SaveRequest" backend/internal/handlers/swaggerspec/openapi.internal.yaml | head -40`
Expected: includes both numeric and identifier fields.

- [ ] **Step 3: Lint the public spec**

Run: `just backend api-lint`
Expected: PASS (or only warnings unrelated to inventory).

- [ ] **Step 4: Commit**

```bash
git add backend/docs backend/internal/handlers/swaggerspec docs/api
git commit -m "docs(tra-448): regenerate OpenAPI for identifier-based inventory save

Public spec now documents location_identifier and asset_identifiers as
the inventory save inputs. Numeric location_id / asset_ids remain in
the internal spec only (swaggerignore on the public surface)."
```

---

## Task 7: Push branch + open PR

- [ ] **Step 1: Push branch**

Run: `git push -u origin miks2u/tra-448-inventorysave-requires-numeric-ids-contradicts-public-api`

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "feat(tra-448): inventory/save accepts natural identifiers" --body "$(cat <<'EOF'
## Summary
- POST /api/v1/inventory/save accepts `location_identifier` and `asset_identifiers`; handler resolves both to internal IDs.
- Numeric `location_id` / `asset_ids` stay accepted for the UI but are hidden from the public OpenAPI surface (swaggerignore).
- New `Storage.GetAssetIDsByIdentifiers` does the asset resolution in a single query.

Fixes TRA-448. Mirrors the TRA-447 `parent_identifier` pattern for `handlers/locations`.

## Test plan
- [ ] `just backend test ./internal/handlers/inventory/...`
- [ ] `just backend test-integration -run TestInventorySave_`
- [ ] `just backend test-integration -run TestGetAssetIDsByIdentifiers`
- [ ] `just backend api-lint`
- [ ] Preview deploy: POST `/api/v1/inventory/save` with identifier-based body succeeds; same with numeric body still succeeds.
EOF
)"
```

Expected: PR URL printed.

- [ ] **Step 3: Hand off**

Print PR URL to the user. No squash; merge commit when reviewed (per project feedback memory).

---

## Self-Review Checklist

- **Spec coverage:** All five DoD bullets addressed: identifier fields ✅ (Task 3), handler resolves to FKs ✅ (Task 4), numeric backward compat ✅ (Task 3 — `omitempty`, accepted), OpenAPI documents both ✅ (Task 6 — public shows new, internal shows both), integration test ✅ (Task 5). Docs quickstart: confirmed there is no Markdown quickstart referencing `inventory/save` outside historical plans, so no docs update needed beyond OpenAPI regen.
- **Placeholder scan:** No "TBD" / "implement later" / generic "add validation" steps. Every code block is complete.
- **Type consistency:** `GetAssetIDsByIdentifiers(ctx, orgID, identifiers []string) (map[string]int, error)` is referenced identically in storage (Task 2 step 4), interface (Task 3 step 3), mock (Task 3 step 4), and handler (Task 4 step 3). `LocationIdentifier *string` and `AssetIdentifiers []string` are consistent throughout. Error message strings (`"disagree"`, `"not both"`, `"not found"`, identifier echoes) match between handler implementation and test assertions.
