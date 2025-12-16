# Implementation Plan: TRA-214 Step 2

Generated: 2025-12-16
Specification: spec.md

## Step 1 (COMPLETED - PR #89)

- ✅ Database migration (000024) - PostgreSQL functions
- ✅ `TagIdentifier`, `TagIdentifierRequest` in `models/shared/identifier.go`
- ✅ `AssetView`, `CreateAssetWithIdentifiersRequest` in `models/asset/asset.go`
- ✅ `storage/identifiers.go` - all identifier CRUD functions
- ✅ `storage/assets.go` - CreateAssetWithIdentifiers, GetAssetViewByID, ListAssetViews
- ✅ 21 unit tests

---

## Step 2: Location Storage + Handlers

### Understanding

Complete TRA-214 by:
1. Adding LocationView model (mirrors AssetView)
2. Adding location storage functions (mirrors asset storage)
3. Adding LookupByTagValue for tag-based entity lookup
4. Updating asset/location handlers to return views with identifiers
5. Adding identifier sub-routes for add/remove operations
6. Adding lookup endpoint returning full entity

### Clarifications Applied
- Lookup returns full entity: `{entity_type, entity_id, asset: {...}}`
- Update existing tests to verify `identifiers: []` + add new tests

---

## Relevant Files

**Reference Patterns** (from Step 1):
- `storage/assets.go:301-402` - CreateAssetWithIdentifiers, GetAssetViewByID, ListAssetViews
- `storage/identifiers.go` - all identifier functions
- `models/asset/asset.go:59-72` - AssetView, CreateAssetWithIdentifiersRequest
- `handlers/assets/assets.go:45-80` - Create handler pattern
- `handlers/assets/assets.go:142-168` - GetAsset handler pattern

**Files to Create**:
- `handlers/assets/identifiers.go` - AddIdentifier, RemoveIdentifier for assets
- `handlers/locations/identifiers.go` - AddIdentifier, RemoveIdentifier for locations
- `handlers/lookup/lookup.go` - LookupByTag handler

**Files to Modify**:
- `models/location/location.go` - add LocationView, CreateLocationWithIdentifiersRequest
- `storage/locations.go` - add CreateLocationWithIdentifiers, GetLocationViewByID, ListLocationViews
- `storage/identifiers.go` - add LookupByTagValue
- `handlers/assets/assets.go` - update Create, GetAsset, ListAssets to use views
- `handlers/locations/locations.go` - update Create, GetLocation, ListLocations to use views
- `cmd/api/main.go` or router file - register new routes

---

## Task Breakdown

### Task 1: Add LocationView Model

**File**: `backend/internal/models/location/location.go`
**Action**: MODIFY
**Pattern**: Reference `models/asset/asset.go:59-72`

```go
type LocationView struct {
	Location
	Identifiers []shared.TagIdentifier `json:"identifiers"`
}

type CreateLocationWithIdentifiersRequest struct {
	CreateLocationRequest
	Identifiers []shared.TagIdentifierRequest `json:"identifiers,omitempty" validate:"omitempty,dive"`
}

type LocationViewListResponse struct {
	Data       []LocationView    `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
```

**Validation**: `go build ./...`

---

### Task 2: Add Location Storage Functions

**File**: `backend/internal/storage/locations.go`
**Action**: MODIFY
**Pattern**: Reference `storage/assets.go:301-402`

```go
func (s *Storage) CreateLocationWithIdentifiers(ctx context.Context, request location.CreateLocationWithIdentifiersRequest) (*location.LocationView, error)

func (s *Storage) GetLocationViewByID(ctx context.Context, id int) (*location.LocationView, error)

func (s *Storage) ListLocationViews(ctx context.Context, orgID, limit, offset int) ([]location.LocationView, error)
```

Implementation mirrors asset functions exactly:
- CreateLocationWithIdentifiers calls `trakrf.create_location_with_identifiers`
- GetLocationViewByID fetches location + identifiers
- ListLocationViews batch fetches identifiers for all locations

**Validation**: `go build ./...` && `go test ./internal/storage/...`

---

### Task 3: Add LookupByTagValue Storage Function

**File**: `backend/internal/storage/identifiers.go`
**Action**: MODIFY

```go
type LookupResult struct {
	EntityType string      `json:"entity_type"` // "asset" or "location"
	EntityID   int         `json:"entity_id"`
	Asset      *asset.Asset    `json:"asset,omitempty"`
	Location   *location.Location `json:"location,omitempty"`
}

func (s *Storage) LookupByTagValue(ctx context.Context, orgID int, tagType, value string) (*LookupResult, error) {
	query := `
		SELECT asset_id, location_id
		FROM trakrf.identifiers
		WHERE org_id = $1 AND type = $2 AND value = $3 AND deleted_at IS NULL
	`
	// Return asset or location based on which ID is non-null
}
```

**Validation**: `go build ./...` && `go test ./internal/storage/...`

---

### Task 4: Update Asset Handlers

**File**: `backend/internal/handlers/assets/assets.go`
**Action**: MODIFY

**Changes**:

1. **Create** (line ~45): Check for identifiers, call appropriate storage method
```go
// If request has identifiers, use transactional method
if len(request.Identifiers) > 0 {
    result, err := handler.storage.CreateAssetWithIdentifiers(ctx, request)
} else {
    result, err := handler.storage.CreateAsset(ctx, request)
}
```

2. **GetAsset** (line ~142): Use GetAssetViewByID
```go
result, err := handler.storage.GetAssetViewByID(req.Context(), id)
// Returns AssetView with identifiers
```

3. **ListAssets** (line ~221): Use ListAssetViews
```go
assets, err := handler.storage.ListAssetViews(req.Context(), orgID, limit, offset)
// Returns []AssetView with identifiers
```

**Validation**: `go build ./...`

---

### Task 5: Add Asset Identifier Sub-routes

**File**: `backend/internal/handlers/assets/identifiers.go` (NEW)
**Action**: CREATE
**Pattern**: Reference existing handler patterns

```go
package assets

// POST /api/v1/assets/{id}/identifiers
func (h *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request) {
	// Parse asset ID from URL
	// Decode TagIdentifierRequest from body
	// Call storage.AddIdentifierToAsset
	// Return created identifier
}

// DELETE /api/v1/assets/{id}/identifiers/{identifierId}
func (h *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	// Parse asset ID and identifier ID from URL
	// Call storage.RemoveIdentifier
	// Return success
}
```

**Register routes** in router:
```go
r.Post("/api/v1/assets/{id}/identifiers", handler.AddIdentifier)
r.Delete("/api/v1/assets/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
```

**Validation**: `go build ./...`

---

### Task 6: Update Location Handlers

**File**: `backend/internal/handlers/locations/locations.go`
**Action**: MODIFY
**Pattern**: Same as Task 4 for assets

**Changes**:
1. **Create**: Check for identifiers, call CreateLocationWithIdentifiers
2. **GetLocation**: Use GetLocationViewByID
3. **ListLocations**: Use ListLocationViews

**Validation**: `go build ./...`

---

### Task 7: Add Location Identifier Sub-routes

**File**: `backend/internal/handlers/locations/identifiers.go` (NEW)
**Action**: CREATE
**Pattern**: Same as Task 5 for assets

```go
package locations

// POST /api/v1/locations/{id}/identifiers
func (h *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request)

// DELETE /api/v1/locations/{id}/identifiers/{identifierId}
func (h *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request)
```

**Validation**: `go build ./...`

---

### Task 8: Add Lookup Handler

**File**: `backend/internal/handlers/lookup/lookup.go` (NEW)
**Action**: CREATE

```go
package lookup

type Handler struct {
	storage *storage.Storage
}

// GET /api/v1/lookup/tag?type=rfid&value=E200...
func (h *Handler) LookupByTag(w http.ResponseWriter, r *http.Request) {
	tagType := r.URL.Query().Get("type")
	value := r.URL.Query().Get("value")

	// Validate inputs
	// Call storage.LookupByTagValue
	// Return full entity
}
```

Response format:
```json
{
  "entity_type": "asset",
  "entity_id": 12345,
  "asset": { ... full asset view ... }
}
```

**Register route**:
```go
r.Get("/api/v1/lookup/tag", lookupHandler.LookupByTag)
```

**Validation**: `go build ./...`

---

### Task 9: Register All New Routes

**File**: Router registration file (find via `grep -r "chi.NewRouter"`)
**Action**: MODIFY

Add routes:
- `POST /api/v1/assets/{id}/identifiers`
- `DELETE /api/v1/assets/{id}/identifiers/{identifierId}`
- `POST /api/v1/locations/{id}/identifiers`
- `DELETE /api/v1/locations/{id}/identifiers/{identifierId}`
- `GET /api/v1/lookup/tag`

**Validation**: `go build ./...`

---

### Task 10: Add Unit Tests for Location Storage

**File**: `backend/internal/storage/locations_test.go`
**Action**: MODIFY
**Pattern**: Reference `storage/identifiers_test.go`

Tests to add:
- TestCreateLocationWithIdentifiers
- TestCreateLocationWithIdentifiers_Rollback
- TestGetLocationViewByID
- TestListLocationViews

**Validation**: `go test ./internal/storage/...`

---

### Task 11: Add Unit Tests for Lookup

**File**: `backend/internal/storage/identifiers_test.go`
**Action**: MODIFY

Tests to add:
- TestLookupByTagValue_Asset
- TestLookupByTagValue_Location
- TestLookupByTagValue_NotFound

**Validation**: `go test ./internal/storage/...`

---

## Risk Assessment

- **Risk**: Handler changes break existing API responses
  **Mitigation**: AssetView embeds Asset, so all existing fields preserved. Only `identifiers` field added.

- **Risk**: Location storage doesn't follow asset pattern exactly
  **Mitigation**: Copy-paste from asset functions, adapt table/column names only.

- **Risk**: Route registration order conflicts
  **Mitigation**: Register specific routes (`/assets/{id}/identifiers`) before generic (`/assets/{id}`).

---

## VALIDATION GATES (MANDATORY)

After EVERY task:
```bash
go build ./...
go test ./internal/storage/...
go test ./internal/handlers/...
```

Final validation:
```bash
go build ./...
go test ./...
```

---

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ All patterns established in Step 1
- ✅ Location mirrors asset exactly
- ✅ Handler patterns exist in codebase
- ✅ Storage patterns proven with 21 tests
- ✅ No new dependencies
- ⚠️ Multiple handler files to modify

**Assessment**: High confidence - this is pattern replication, not new design.

**Estimated one-pass success probability**: 85%

**Reasoning**: All patterns proven in Step 1. Main risk is typos/oversights when copying patterns to locations.
