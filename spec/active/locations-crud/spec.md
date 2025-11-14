# Feature: Locations CRUD with Hierarchical Navigation

## Metadata
**Workspace**: Full-stack (database, backend, frontend)
**Type**: Feature

## Outcome
Users can create, read, update, and delete locations in a hierarchical structure (Country → City → Warehouse → Section → etc.) with unlimited nesting depth, efficient querying, and intuitive navigation.

## User Story

**As a warehouse manager**
I want to organize locations in a nested hierarchy (e.g., USA → California → Warehouse 1 → Section A → Shelf 3)
So that I can accurately track where assets are located and navigate the physical structure of my facilities

**As a system administrator**
I want locations to support unlimited nesting depth
So that I can model any organizational structure from simple (Country → Building) to complex (Country → Region → City → Campus → Building → Floor → Room → Zone → Shelf)

**As an API consumer**
I want efficient queries for ancestor/descendant relationships
So that I can quickly find "all locations under Warehouse 1" or "the full path to Shelf 3" without performance issues

## Context

### Current State
- `locations` table exists with `parent_location_id` (adjacency list)
- No ltree extension enabled
- No hierarchy-specific queries or indexes
- No frontend UI for locations management
- No backend API endpoints for locations CRUD

### Desired State
- ltree extension enabled for efficient hierarchy queries
- `path` column added to locations table (e.g., `usa.california.warehouse_1.section_a`)
- Full CRUD API endpoints with hierarchy operations
- Frontend UI for managing locations with tree visualization
- Support for unlimited nesting depth
- Fast queries for ancestors, descendants, siblings, depth

### Database Schema Enhancement
**Migration**: Add ltree support while preserving existing parent_location_id
```sql
-- Enable ltree extension
CREATE EXTENSION IF NOT EXISTS ltree;

-- Add path column
ALTER TABLE locations ADD COLUMN path ltree;

-- Add depth as generated column
ALTER TABLE locations ADD COLUMN depth INT GENERATED ALWAYS AS (
  nlevel(path)
) STORED;

-- Index for hierarchy queries
CREATE INDEX locations_path_gist_idx ON locations USING GIST (path);
CREATE INDEX locations_depth_idx ON locations(depth);
```

**Why keep parent_location_id?**
- Simple parent lookup (one JOIN vs path parsing)
- Backward compatibility if needed
- Easier to understand for new developers

**Why add ltree path?**
- O(1) depth calculation: `nlevel(path)`
- Instant ancestor/descendant queries: `path <@ 'usa.california'`
- Pattern matching: `path ~ '*.warehouse_1.*'`
- Subtree queries: `path <@ 'usa.california.warehouse_1'`
- No recursive CTEs needed

## Technical Requirements

### Database Layer

**Migration 000018_locations_add_ltree.up.sql**:
- Enable ltree extension
- Add `path ltree` column
- Add `depth INT GENERATED` column
- Create GiST index on path
- Create trigger to maintain path on INSERT/UPDATE
- Backfill existing locations with paths

**Path Maintenance Trigger**:
```sql
CREATE OR REPLACE FUNCTION update_location_path()
RETURNS TRIGGER AS $$
BEGIN
  IF NEW.parent_location_id IS NULL THEN
    -- Root location
    NEW.path = text2ltree(NEW.identifier);
  ELSE
    -- Child location: append to parent's path
    SELECT path || text2ltree(NEW.identifier)
    INTO NEW.path
    FROM locations
    WHERE id = NEW.parent_location_id;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER maintain_location_path
  BEFORE INSERT OR UPDATE ON locations
  FOR EACH ROW
  EXECUTE FUNCTION update_location_path();
```

**Constraint**: Prevent cycles
```sql
ALTER TABLE locations ADD CONSTRAINT no_self_reference
  CHECK (id != parent_location_id);
```

### Backend API Layer

**Endpoints**:
```
GET    /api/v1/locations                 # List all locations (with hierarchy)
GET    /api/v1/locations/:id             # Get single location
GET    /api/v1/locations/:id/ancestors   # Get full path from root
GET    /api/v1/locations/:id/descendants # Get all children/grandchildren
GET    /api/v1/locations/:id/children    # Get direct children only
POST   /api/v1/locations                 # Create location
PUT    /api/v1/locations/:id             # Update location
DELETE /api/v1/locations/:id             # Soft delete location
POST   /api/v1/locations/:id/move        # Move location to new parent
```

**Models** (`backend/internal/models/location/location.go`):
```go
type Location struct {
    ID               int        `json:"id"`
    OrgID            int        `json:"org_id"`
    Identifier       string     `json:"identifier"`
    Name             string     `json:"name"`
    Description      string     `json:"description"`
    ParentLocationID *int       `json:"parent_location_id"`
    Path             string     `json:"path"`          // ltree as string
    Depth            int        `json:"depth"`
    ValidFrom        time.Time  `json:"valid_from"`
    ValidTo          *time.Time `json:"valid_to"`
    IsActive         bool       `json:"is_active"`
    Metadata         JSONB      `json:"metadata"`
    CreatedAt        time.Time  `json:"created_at"`
    UpdatedAt        time.Time  `json:"updated_at"`
}

type CreateLocationRequest struct {
    Identifier       string     `json:"identifier" validate:"required,max=255"`
    Name             string     `json:"name" validate:"required,max=255"`
    Description      string     `json:"description" validate:"max=1024"`
    ParentLocationID *int       `json:"parent_location_id" validate:"omitempty"`
    Metadata         JSONB      `json:"metadata"`
}

type UpdateLocationRequest struct {
    Name             *string    `json:"name" validate:"omitempty,max=255"`
    Description      *string    `json:"description" validate:"omitempty,max=1024"`
    Metadata         *JSONB     `json:"metadata"`
}

type MoveLocationRequest struct {
    NewParentID *int `json:"new_parent_id" validate:"omitempty"`
}

type LocationTreeNode struct {
    Location
    Children []LocationTreeNode `json:"children,omitempty"`
}
```

**Storage** (`backend/internal/storage/locations.go`):
```go
func (s *Storage) ListLocations(ctx context.Context, orgID int) ([]Location, error)
func (s *Storage) GetLocation(ctx context.Context, id int, orgID int) (*Location, error)
func (s *Storage) GetLocationAncestors(ctx context.Context, id int, orgID int) ([]Location, error)
func (s *Storage) GetLocationDescendants(ctx context.Context, id int, orgID int) ([]Location, error)
func (s *Storage) GetLocationChildren(ctx context.Context, id int, orgID int) ([]Location, error)
func (s *Storage) CreateLocation(ctx context.Context, loc Location) (*Location, error)
func (s *Storage) UpdateLocation(ctx context.Context, id int, orgID int, updates map[string]any) (*Location, error)
func (s *Storage) DeleteLocation(ctx context.Context, id int, orgID int) error
func (s *Storage) MoveLocation(ctx context.Context, id int, newParentID *int, orgID int) (*Location, error)
func (s *Storage) BuildLocationTree(ctx context.Context, orgID int) ([]LocationTreeNode, error)
```

**Key Queries**:
```sql
-- Get ancestors (full path from root to location)
SELECT * FROM locations
WHERE path @> (SELECT path FROM locations WHERE id = $1)
  AND org_id = $2
ORDER BY depth ASC;

-- Get all descendants
SELECT * FROM locations
WHERE path <@ (SELECT path FROM locations WHERE id = $1)
  AND org_id = $2
ORDER BY path;

-- Get direct children only
SELECT * FROM locations
WHERE path ~ ((SELECT path FROM locations WHERE id = $1)::text || '.*{1}')::lquery
  AND org_id = $2;

-- Move location (update parent_location_id, trigger recalculates path)
UPDATE locations SET parent_location_id = $1 WHERE id = $2;
```

### Frontend Layer

**Pages**:
- `frontend/src/pages/LocationsPage.tsx` - Main locations management page

**Components**:
- `frontend/src/components/locations/LocationTree.tsx` - Tree visualization
- `frontend/src/components/locations/LocationCard.tsx` - Location details card
- `frontend/src/components/locations/LocationForm.tsx` - Create/edit form
- `frontend/src/components/locations/LocationBreadcrumb.tsx` - Path navigation
- `frontend/src/components/locations/LocationMoveDialog.tsx` - Move location dialog

**Store** (`frontend/src/stores/locationStore.ts`):
```typescript
interface LocationState {
  locations: Map<number, Location>;
  tree: LocationTreeNode[];
  selectedLocation: Location | null;
  isLoading: boolean;
  error: string | null;

  // Actions
  fetchLocations: () => Promise<void>;
  fetchLocationTree: () => Promise<void>;
  fetchLocationAncestors: (id: number) => Promise<Location[]>;
  fetchLocationDescendants: (id: number) => Promise<Location[]>;
  createLocation: (data: CreateLocationRequest) => Promise<Location>;
  updateLocation: (id: number, data: UpdateLocationRequest) => Promise<Location>;
  deleteLocation: (id: number) => Promise<void>;
  moveLocation: (id: number, newParentId: number | null) => Promise<Location>;
  selectLocation: (location: Location | null) => void;
}
```

**API Client** (`frontend/src/api/locations.ts`):
```typescript
export const locationsApi = {
  list: () => apiClient.get<Location[]>('/locations'),
  get: (id: number) => apiClient.get<Location>(`/locations/${id}`),
  getAncestors: (id: number) => apiClient.get<Location[]>(`/locations/${id}/ancestors`),
  getDescendants: (id: number) => apiClient.get<Location[]>(`/locations/${id}/descendants`),
  getChildren: (id: number) => apiClient.get<Location[]>(`/locations/${id}/children`),
  create: (data: CreateLocationRequest) => apiClient.post<Location>('/locations', data),
  update: (id: number, data: UpdateLocationRequest) => apiClient.put<Location>(`/locations/${id}`, data),
  delete: (id: number) => apiClient.delete(`/locations/${id}`),
  move: (id: number, newParentId: number | null) =>
    apiClient.post<Location>(`/locations/${id}/move`, { new_parent_id: newParentId }),
};
```

## Validation Criteria

- [ ] ltree extension enabled in database
- [ ] Path column populated for all locations
- [ ] Trigger maintains path consistency on INSERT/UPDATE/MOVE
- [ ] All CRUD endpoints working with proper org isolation
- [ ] Hierarchy queries return correct ancestors/descendants
- [ ] Frontend tree component renders nested locations
- [ ] Can create root locations (no parent)
- [ ] Can create nested locations (with parent)
- [ ] Can move locations to different parents
- [ ] Path updates cascade to all descendants when moving
- [ ] Cannot create circular references
- [ ] Cannot delete location with children (or cascade delete)
- [ ] Breadcrumb navigation shows full path
- [ ] Search works across all levels

## Success Metrics

- [ ] Query performance: Ancestor lookup < 50ms for depth 10
- [ ] Query performance: Descendant lookup < 100ms for 1000 locations
- [ ] All backend tests passing (unit + integration)
- [ ] All frontend tests passing (components + stores)
- [ ] E2E test: Create nested location hierarchy (depth 5)
- [ ] E2E test: Move location preserves descendant structure
- [ ] E2E test: Delete location fails with children present
- [ ] Zero N+1 queries in list endpoint
- [ ] Tree component handles 500+ locations without lag
- [ ] Mobile-responsive location management UI

## References

- [PostgreSQL ltree Documentation](https://www.postgresql.org/docs/current/ltree.html)
- [Existing locations migration](../../../database/migrations/000005_locations.up.sql)
- [Assets CRUD pattern](../../../backend/internal/handlers/assets/)
- [Zustand store pattern](../../../frontend/src/stores/assetStore.ts)
- [Tree component inspiration](https://ui.shadcn.com/docs/components/tree)

## Non-Goals (Out of Scope)

- Location templates or bulk creation
- Location images/photos
- Location-based access control (beyond org isolation)
- Geographic coordinates/mapping integration
- QR code generation for locations
- Location capacity/occupancy tracking

These can be added in future iterations if needed.
