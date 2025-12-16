# Feature: Transactional Asset/Location + Identifiers Creation (TRA-214)

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-214
**Parent**: TRA-193 (Asset CRUD - separate customer identifier from tag identifiers)

## Outcome
Assets and locations can be created with their tag identifiers (RFID, BLE, barcode) in a single atomic transaction, ensuring data consistency.

## User Story
As a developer
I want to create assets/locations with their tag identifiers atomically
So that partial data is never persisted if any part of the creation fails

## Context
**Current**:
- `identifiers` table exists in DB with XOR constraint (asset_id OR location_id, never both)
- No Go code interacts with `identifiers` table
- Assets/locations created without tag identifier support

**Desired**:
- Create asset/location with embedded tag identifiers in single transaction
- Add/remove identifiers from existing assets/locations
- Lookup asset/location by tag value

**Examples**:
- Existing patterns: `backend/internal/storage/assets.go` (`BatchCreateAssets` uses transactions)
- Schema: `backend/migrations/000009_identifiers.up.sql`

## Technical Requirements

### Schema Reminder
```sql
-- identifiers table XOR constraint
CONSTRAINT identifier_target CHECK (
    (asset_id IS NOT NULL AND location_id IS NULL) OR
    (asset_id IS NULL AND location_id IS NOT NULL)
)
UNIQUE(org_id, type, value, valid_from)
```

### Model Changes

#### TagIdentifier (shared struct)

```go
// backend/internal/models/shared/identifier.go
type TagIdentifier struct {
    ID       int    `json:"id,omitempty"`
    Type     string `json:"type" validate:"required,oneof=rfid ble barcode"`
    Value    string `json:"value" validate:"required,min=1,max=255"`
    IsActive bool   `json:"is_active"`
}
```

#### Asset Models (internal vs view)

```go
// backend/internal/models/asset/asset.go

// Asset - internal model (no identifiers, matches DB row)
type Asset struct {
    ID                int        `json:"id"`
    OrgID             int        `json:"org_id"`
    Identifier        string     `json:"identifier"`       // Customer ID
    Name              string     `json:"name"`
    Type              string     `json:"type"`
    Description       string     `json:"description"`
    CurrentLocationID *int       `json:"current_location_id"`
    // ... other fields
}

// AssetView - API response model (includes identifiers)
type AssetView struct {
    Asset
    Identifiers []shared.TagIdentifier `json:"identifiers"`
}

// CreateAssetRequest - accepts optional identifiers
type CreateAssetRequest struct {
    Identifier        string                 `json:"identifier" validate:"required,min=1,max=255"`
    Name              string                 `json:"name" validate:"required,min=1,max=255"`
    Type              string                 `json:"type" validate:"oneof=asset"`
    Description       string                 `json:"description"`
    CurrentLocationID *int                   `json:"current_location_id"`
    Identifiers       []shared.TagIdentifier `json:"identifiers,omitempty"`
    // ... other fields
}
```

#### Location Models (same pattern)

```go
// backend/internal/models/location/location.go

// Location - internal model
type Location struct {
    ID         int    `json:"id"`
    OrgID      int    `json:"org_id"`
    Identifier string `json:"identifier"`
    Name       string `json:"name"`
    // ... other fields
}

// LocationView - API response model (includes identifiers)
type LocationView struct {
    Location
    Identifiers []shared.TagIdentifier `json:"identifiers"`
}

// CreateLocationRequest - accepts optional identifiers
type CreateLocationRequest struct {
    Identifier  string                 `json:"identifier"`
    Name        string                 `json:"name" validate:"required"`
    Identifiers []shared.TagIdentifier `json:"identifiers,omitempty"`
    // ... other fields
}
```

### API Changes

**Create Asset** - `POST /api/v1/assets`:
```json
{
  "identifier": "AV-001234",
  "name": "Engineering Laptop",
  "type": "asset",
  "identifiers": [
    { "type": "rfid", "value": "E20000000000001234" },
    { "type": "ble", "value": "AA:BB:CC:DD:EE:FF" }
  ]
}
```

**Create Location** - `POST /api/v1/locations`:
```json
{
  "identifier": "ROOM-101",
  "name": "Conference Room A",
  "identifiers": [
    { "type": "rfid", "value": "E20000000000005678" }
  ]
}
```

**Response** (both endpoints):
```json
{
  "data": {
    "id": 12345,
    "identifier": "AV-001234",
    "name": "Engineering Laptop",
    "identifiers": [
      { "id": 1, "type": "rfid", "value": "E20000000000001234", "is_active": true },
      { "id": 2, "type": "ble", "value": "AA:BB:CC:DD:EE:FF", "is_active": true }
    ]
  }
}
```

### Storage Layer

```go
// backend/internal/storage/identifiers.go

// Transactional creation
CreateAssetWithIdentifiers(ctx, asset Asset) (*Asset, error)
CreateLocationWithIdentifiers(ctx, location Location) (*Location, error)

// Tag management for existing entities
AddIdentifierToAsset(ctx, assetID int, identifier TagIdentifier) (*TagIdentifier, error)
AddIdentifierToLocation(ctx, locationID int, identifier TagIdentifier) (*TagIdentifier, error)
RemoveIdentifier(ctx, identifierID int) error

// Lookup
LookupByTagValue(ctx, tagType string, value string) (*LookupResult, error)

// Get identifiers for entity
GetIdentifiersByAssetID(ctx, assetID int) ([]TagIdentifier, error)
GetIdentifiersByLocationID(ctx, locationID int) ([]TagIdentifier, error)
```

### Operation Modes

Asset/Location and Identifier operations are **not strictly coupled**. Users can:

1. **Create asset/location only** - no identifiers, works as today
2. **Add identifiers later** - `POST /api/v1/assets/{id}/identifiers`
3. **Create with identifiers** - single request, uses stored procedure for atomicity

### Stored Procedure for Atomic Creation

When creating asset/location WITH identifiers in one request, use a **stored procedure** to ensure atomicity:

```sql
-- backend/migrations/000010_identifier_procedures.up.sql

CREATE OR REPLACE FUNCTION create_asset_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_type VARCHAR(50),
    p_description TEXT,
    p_current_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB  -- array of {type, value}
) RETURNS TABLE (
    asset_id INT,
    identifier_ids INT[]
) AS $$
DECLARE
    v_asset_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    -- Insert asset
    INSERT INTO trakrf.assets (org_id, identifier, name, type, description,
                               current_location_id, valid_from, valid_to, is_active, metadata)
    VALUES (p_org_id, p_identifier, p_name, p_type, p_description,
            p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata)
    RETURNING id INTO v_asset_id;

    -- Insert identifiers
    FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
    LOOP
        INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
        VALUES (p_org_id,
                v_identifier->>'type',
                v_identifier->>'value',
                v_asset_id,
                TRUE)
        RETURNING id INTO v_new_id;

        v_identifier_ids := array_append(v_identifier_ids, v_new_id);
    END LOOP;

    RETURN QUERY SELECT v_asset_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;
```

Similar procedure for locations: `create_location_with_identifiers()`

### Transaction Behavior

1. **Stored procedure** handles atomicity - if any identifier insert fails, entire operation rolls back
2. **Duplicate check**: Stored procedure fails if tag value already exists in org (unique constraint)
3. **Standalone operations**: Adding identifier to existing asset is a simple INSERT, no procedure needed

### Handler Changes

```go
// backend/internal/handlers/assets/assets.go
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    // If request has identifiers, use transactional method
    if len(request.Identifiers) > 0 {
        result, err := h.storage.CreateAssetWithIdentifiers(ctx, request)
    } else {
        result, err := h.storage.CreateAsset(ctx, request)
    }
}
```

### Additional Endpoints

```go
// Add to existing asset
POST /api/v1/assets/{id}/identifiers
Body: { "type": "rfid", "value": "E200..." }

// Remove from asset
DELETE /api/v1/assets/{id}/identifiers/{identifierId}

// Lookup by tag
GET /api/v1/lookup/tag?type=rfid&value=E200...
Response: { "entity_type": "asset", "entity_id": 123, "asset": {...} }
```

### Files to Create/Modify

| File | Action |
|------|--------|
| `backend/migrations/000010_identifier_procedures.up.sql` | New - stored procedures for atomic creation |
| `backend/migrations/000010_identifier_procedures.down.sql` | New - drop stored procedures |
| `backend/internal/models/shared/identifier.go` | New - `TagIdentifier` struct (shared) |
| `backend/internal/models/asset/asset.go` | Add `AssetView`, update `CreateAssetRequest` |
| `backend/internal/models/location/location.go` | Add `LocationView`, update `CreateLocationRequest` |
| `backend/internal/storage/identifiers.go` | New - identifier storage functions |
| `backend/internal/storage/assets.go` | Add `CreateAssetWithIdentifiers` (calls stored proc), `GetAssetViewByID` |
| `backend/internal/storage/locations.go` | Add `CreateLocationWithIdentifiers` (calls stored proc), `GetLocationViewByID` |
| `backend/internal/handlers/assets/assets.go` | Return `AssetView`, call stored proc when identifiers present |
| `backend/internal/handlers/locations/locations.go` | Return `LocationView`, call stored proc when identifiers present |
| `backend/internal/handlers/lookup/lookup.go` | New - tag lookup endpoint |

### Out of Scope

**Bulk import (`POST /api/v1/assets/bulk`) is NOT modified in TRA-214.**

Bulk CSV import remains unchanged - no tag identifier support. Tags for bulk-imported assets can be added later via `POST /api/v1/assets/{id}/identifiers`.

### GET Responses (View Models)

All GET endpoints return view models with embedded identifiers:

**GET /api/v1/assets/{id}** → `AssetView`
**GET /api/v1/assets** → `[]AssetView`
**GET /api/v1/locations/{id}** → `LocationView`
**GET /api/v1/locations** → `[]LocationView`

```json
{
  "data": {
    "id": 12345,
    "identifier": "AV-001234",
    "name": "Engineering Laptop",
    "type": "asset",
    "identifiers": [
      { "id": 1, "type": "rfid", "value": "E20000000000001234", "is_active": true }
    ]
  }
}
```

Storage layer uses JOIN or separate query to fetch identifiers:
```go
func (s *Storage) GetAssetViewByID(ctx, id int) (*AssetView, error) {
    asset, err := s.GetAssetByID(ctx, id)
    if err != nil { return nil, err }

    identifiers, err := s.GetIdentifiersByAssetID(ctx, id)
    if err != nil { return nil, err }

    return &AssetView{Asset: *asset, Identifiers: identifiers}, nil
}
```

## Validation Criteria
- [ ] Asset + identifiers created in single transaction
- [ ] Location + identifiers created in single transaction
- [ ] Rollback on any identifier failure (all or nothing)
- [ ] Duplicate tag value returns error (not silent ignore)
- [ ] Response includes created identifiers with IDs
- [ ] GET asset/location includes associated identifiers
- [ ] Add identifier to existing asset works
- [ ] Remove identifier from asset works
- [ ] Lookup by tag value returns correct entity

## Success Metrics
- [ ] Transaction rollback verified with failing identifier
- [ ] Duplicate check covers both assets AND locations
- [ ] API response time < 100ms for single asset + 3 identifiers
- [ ] All existing asset/location tests still pass
- [ ] New tests: 10+ covering transactional behavior
- [ ] Zero partial data on failure scenarios

## Implementation Order

### Phase 1: Database
1. Create migration `000010_identifier_procedures.up.sql` with stored procedures
2. Create migration `000010_identifier_procedures.down.sql` to drop procedures
3. Run migrations, verify procedures work

### Phase 2: Models
4. Create `models/shared/identifier.go` with `TagIdentifier` struct
5. Add `AssetView` to `models/asset/asset.go`, update `CreateAssetRequest`
6. Add `LocationView` to `models/location/location.go`, update `CreateLocationRequest`

### Phase 3: Storage Layer
7. Create `storage/identifiers.go` with basic functions:
   - `GetIdentifiersByAssetID`
   - `GetIdentifiersByLocationID`
   - `AddIdentifierToAsset`
   - `AddIdentifierToLocation`
   - `RemoveIdentifier`
   - `LookupByTagValue`
8. Add `CreateAssetWithIdentifiers` to `storage/assets.go` (calls stored proc)
9. Add `GetAssetViewByID`, `ListAssetViews` to `storage/assets.go`
10. Repeat for locations

### Phase 4: Handlers
11. Update asset handlers to return `AssetView`
12. Update asset Create handler to call stored proc when identifiers present
13. Add `POST /api/v1/assets/{id}/identifiers` endpoint
14. Add `DELETE /api/v1/assets/{id}/identifiers/{id}` endpoint
15. Repeat for locations
16. Create `handlers/lookup/lookup.go` with tag lookup endpoint

### Phase 5: Testing
17. Unit tests for stored procedure (via storage layer)
18. Integration tests for atomic creation
19. Tests for add/remove identifier operations
20. Tests for GET responses including identifiers

## References
- [Identifiers migration](backend/migrations/000009_identifiers.up.sql) - table schema
- [Assets migration](backend/migrations/000008_assets.up.sql) - asset schema
- [Asset storage](backend/internal/storage/assets.go) - existing patterns, `BatchCreateAssets` for transaction example
- [Asset model](backend/internal/models/asset/asset.go) - current model
- [Linear Issue](https://linear.app/trakrf/issue/TRA-214)
- [Parent Issue TRA-193](https://linear.app/trakrf/issue/TRA-193)
