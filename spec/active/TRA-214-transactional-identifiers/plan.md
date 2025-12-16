# Implementation Plan: TRA-214 Step 2 (Consolidated)

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

## Step 2: Consolidated (4 Tasks)

### Understanding

Complete TRA-214 by implementing the location-side mirror of Step 1 and updating handlers to use View models. Consolidated from 11 tasks to 4 based on proven patterns.

### Clarifications Applied
- Consolidate into fewer, larger tasks
- Add identifier methods to existing handler files (not separate files)
- Minimal tests - rely on patterns proven in Step 1

---

## Relevant Files

**Reference Patterns** (from Step 1):
- `models/asset/asset.go:59-72` - AssetView, CreateAssetWithIdentifiersRequest, AssetViewListResponse
- `storage/assets.go:302-387` - CreateAssetWithIdentifiers, GetAssetViewByID, ListAssetViews
- `storage/identifiers.go:189-222` - getIdentifiersForAssets (batch fetch pattern)
- `handlers/assets/assets.go:45-80` - Create handler pattern

**Files to Modify**:
- `models/location/location.go` - add LocationView, CreateLocationWithIdentifiersRequest
- `storage/locations.go` - add CreateLocationWithIdentifiers, GetLocationViewByID, ListLocationViews
- `storage/identifiers.go` - add LookupByTagValue
- `handlers/assets/assets.go` - update Create/Get/List to use Views, add AddIdentifier/RemoveIdentifier
- `handlers/locations/locations.go` - update Create/Get/List to use Views, add AddIdentifier/RemoveIdentifier

**Files to Create**:
- `handlers/lookup/lookup.go` - LookupByTag handler

---

## Task Breakdown

### Task 1: Location Models + Storage

**Files**: `models/location/location.go`, `storage/locations.go`
**Action**: MODIFY
**Pattern**: Mirror `models/asset/asset.go:59-72` and `storage/assets.go:302-387`

**1a. Add to `models/location/location.go`**:

```go
// After LocationListResponse (line ~60)

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

**1b. Add to `storage/locations.go`**:

```go
func (s *Storage) CreateLocationWithIdentifiers(ctx context.Context, orgID int, request location.CreateLocationWithIdentifiersRequest) (*location.LocationView, error) {
	// Mirror storage/assets.go:302-332
	// Call trakrf.create_location_with_identifiers stored proc
}

func (s *Storage) GetLocationViewByID(ctx context.Context, id int) (*location.LocationView, error) {
	// Mirror storage/assets.go:334-352
	// Fetch location + identifiers
}

func (s *Storage) ListLocationViews(ctx context.Context, orgID, limit, offset int) ([]location.LocationView, error) {
	// Mirror storage/assets.go:354-387
	// Use getIdentifiersForLocations for batch fetch
}
```

**Validation**: `go build ./...`

---

### Task 2: Lookup Storage

**File**: `storage/identifiers.go`
**Action**: MODIFY

**Add LookupByTagValue**:

```go
type LookupResult struct {
	EntityType string           `json:"entity_type"` // "asset" or "location"
	EntityID   int              `json:"entity_id"`
	Asset      *asset.Asset     `json:"asset,omitempty"`
	Location   *location.Location `json:"location,omitempty"`
}

func (s *Storage) LookupByTagValue(ctx context.Context, orgID int, tagType, value string) (*LookupResult, error) {
	query := `
		SELECT asset_id, location_id
		FROM trakrf.identifiers
		WHERE org_id = $1 AND type = $2 AND value = $3 AND deleted_at IS NULL
	`

	var assetID, locationID *int
	err := s.pool.QueryRow(ctx, query, orgID, tagType, value).Scan(&assetID, &locationID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lookup tag: %w", err)
	}

	if assetID != nil {
		asset, err := s.GetAssetByID(ctx, assetID)
		if err != nil {
			return nil, err
		}
		return &LookupResult{EntityType: "asset", EntityID: *assetID, Asset: asset}, nil
	}

	if locationID != nil {
		loc, err := s.GetLocationByID(ctx, *locationID)
		if err != nil {
			return nil, err
		}
		return &LookupResult{EntityType: "location", EntityID: *locationID, Location: loc}, nil
	}

	return nil, nil
}
```

**Validation**: `go build ./...`

---

### Task 3: Update All Handlers

**Files**: `handlers/assets/assets.go`, `handlers/locations/locations.go`
**Action**: MODIFY

**3a. Update `handlers/assets/assets.go`**:

1. **Create** (line ~45): Accept identifiers, route to appropriate storage method
```go
// Change request type to handle both cases
var request asset.CreateAssetWithIdentifiersRequest
// ... decode and validate ...

if len(request.Identifiers) > 0 {
	result, err := handler.storage.CreateAssetWithIdentifiers(ctx, request)
} else {
	// Create without identifiers, return as AssetView with empty identifiers
	baseAsset, err := handler.storage.CreateAsset(ctx, request.CreateAssetRequest)
	result = &asset.AssetView{Asset: *baseAsset, Identifiers: []shared.TagIdentifier{}}
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
```

4. **Add identifier methods**:
```go
// POST /api/v1/assets/{id}/identifiers
func (handler *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request) {
	// Parse asset ID, decode TagIdentifierRequest
	// Call storage.AddIdentifierToAsset
	// Return created identifier
}

// DELETE /api/v1/assets/{id}/identifiers/{identifierId}
func (handler *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	// Parse asset ID and identifier ID
	// Call storage.RemoveIdentifier
	// Return success
}
```

5. **Update RegisterRoutes**:
```go
r.Post("/api/v1/assets/{id}/identifiers", handler.AddIdentifier)
r.Delete("/api/v1/assets/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
```

**3b. Update `handlers/locations/locations.go`** (same pattern):

1. **Create**: Accept identifiers, route appropriately
2. **Get**: Use GetLocationViewByID
3. **List**: Use ListLocationViews
4. **Add**: AddIdentifier, RemoveIdentifier methods
5. **Routes**: Register identifier sub-routes

**Validation**: `go build ./...`

---

### Task 4: Lookup Handler

**File**: `handlers/lookup/lookup.go` (NEW)
**Action**: CREATE

```go
package lookup

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// GET /api/v1/lookup/tag?type=rfid&value=E200...
func (h *Handler) LookupByTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		// Return unauthorized error
		return
	}
	orgID := *claims.CurrentOrgID

	tagType := r.URL.Query().Get("type")
	value := r.URL.Query().Get("value")

	if tagType == "" || value == "" {
		// Return bad request error
		return
	}

	result, err := h.storage.LookupByTagValue(r.Context(), orgID, tagType, value)
	if err != nil {
		// Return internal error
		return
	}

	if result == nil {
		// Return not found
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/lookup/tag", h.LookupByTag)
}
```

**Register in main.go or router**:
```go
lookupHandler := lookup.NewHandler(store)
lookupHandler.RegisterRoutes(r)
```

**Validation**: `go build ./...` && `go test ./...`

---

## Risk Assessment

- **Risk**: Handler changes break existing API responses
  **Mitigation**: AssetView/LocationView embed base types, all existing fields preserved. Only `identifiers` field added.

- **Risk**: CreateLocationWithIdentifiers stored proc doesn't exist
  **Mitigation**: Verify migration 000024 includes location proc. If missing, add migration.

---

## VALIDATION GATES (MANDATORY)

After EVERY task:
```bash
go build ./...
```

After Task 3 & 4:
```bash
go test ./internal/storage/...
go test ./internal/handlers/...
```

Final validation:
```bash
go build ./... && go test ./...
```

---

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ All patterns established in Step 1 (storage/assets.go:302-387)
- ✅ Location mirrors asset exactly
- ✅ Handler patterns exist in codebase
- ✅ Storage layer proven with 21 tests
- ✅ No new dependencies
- ✅ Consolidated to 4 manageable tasks

**Assessment**: High confidence - this is pattern replication, not new design.

**Estimated one-pass success probability**: 90%

**Reasoning**: All patterns proven in Step 1. Main risk is typos when copying patterns to locations. Consolidation reduces context-switching overhead.
