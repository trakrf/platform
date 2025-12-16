# Implementation Plan: TRA-214

## Step 1 (COMPLETED - PR #89)

- Database migration (000024) with PostgreSQL functions
- TagIdentifier shared model
- AssetView model
- Asset storage functions (CreateAssetWithIdentifiers, GetAssetViewByID, ListAssetViews)
- Identifier storage functions (GetIdentifiersByAssetID, AddIdentifierToAsset, etc.)
- 21 unit tests

---

# Step 2: Location Storage + Handlers + Tests

## 2.1 Location Model

**File**: `backend/internal/models/location/location.go`

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

---

## 2.2 Location Storage Functions

**File**: `backend/internal/storage/locations.go`

```go
func (s *Storage) CreateLocationWithIdentifiers(ctx context.Context, request location.CreateLocationWithIdentifiersRequest) (*location.LocationView, error)

func (s *Storage) GetLocationViewByID(ctx context.Context, id int) (*location.LocationView, error)

func (s *Storage) ListLocationViews(ctx context.Context, orgID, limit, offset int) ([]location.LocationView, error)
```

---

## 2.3 Lookup Storage Function

**File**: `backend/internal/storage/identifiers.go`

```go
type LookupResult struct {
	EntityType string `json:"entity_type"` // "asset" or "location"
	EntityID   int    `json:"entity_id"`
}

func (s *Storage) LookupByTagValue(ctx context.Context, orgID int, tagType, value string) (*LookupResult, error)
```

---

## 2.4 Update Asset Handlers

**File**: `backend/internal/handlers/assets/assets.go`

| Handler | Change |
|---------|--------|
| `Create` | If `request.Identifiers` present, call `CreateAssetWithIdentifiers` |
| `GetAsset` | Return `AssetView` instead of `Asset` |
| `ListAssets` | Return `[]AssetView` instead of `[]Asset` |

---

## 2.5 Asset Identifier Sub-routes

**File**: `backend/internal/handlers/assets/identifiers.go` (NEW)

```go
// POST /api/v1/assets/{id}/identifiers
func (h *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request)

// DELETE /api/v1/assets/{id}/identifiers/{identifierId}
func (h *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request)
```

Register in `assets.go`:
```go
r.Post("/api/v1/assets/{id}/identifiers", handler.AddIdentifier)
r.Delete("/api/v1/assets/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
```

---

## 2.6 Update Location Handlers

**File**: `backend/internal/handlers/locations/locations.go`

Same pattern as assets:
- `Create` → call `CreateLocationWithIdentifiers` when identifiers present
- `GetLocation` → return `LocationView`
- `ListLocations` → return `[]LocationView`

---

## 2.7 Location Identifier Sub-routes

**File**: `backend/internal/handlers/locations/identifiers.go` (NEW)

```go
// POST /api/v1/locations/{id}/identifiers
func (h *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request)

// DELETE /api/v1/locations/{id}/identifiers/{identifierId}
func (h *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request)
```

---

## 2.8 Lookup Handler

**File**: `backend/internal/handlers/lookup/lookup.go` (NEW)

```go
// GET /api/v1/lookup/tag?type=rfid&value=E200...
func (h *Handler) LookupByTag(w http.ResponseWriter, r *http.Request)
```

Response:
```json
{
  "entity_type": "asset",
  "entity_id": 12345,
  "asset": { ... }
}
```

---

## 2.9 Integration Tests

**File**: `backend/tests/integration/identifiers_test.go` (NEW)

| Test | Description |
|------|-------------|
| `TestCreateAssetWithIdentifiers` | Asset + identifiers created atomically |
| `TestCreateAssetWithIdentifiers_Rollback` | Duplicate tag causes full rollback |
| `TestAddIdentifierToAsset` | Add identifier to existing asset |
| `TestRemoveIdentifier` | Remove identifier from asset |
| `TestGetAssetView` | GET returns asset with identifiers |
| `TestListAssetViews` | List returns assets with identifiers |
| `TestLookupByTag` | Lookup returns correct entity |
| Repeat for locations | Same tests for location endpoints |

---

## Verification Checklist

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] Manual test: Create asset with identifiers via API
- [ ] Manual test: Add/remove identifier via sub-routes
- [ ] Manual test: Lookup by tag value
- [ ] GET endpoints return `identifiers: []` (not null) when empty
