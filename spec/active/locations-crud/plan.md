# Implementation Plan: Locations CRUD with ltree

## Overview

This plan breaks down the locations feature into 4 phases:
1. **Database & Migration** - Add ltree support, triggers, indexes
2. **Backend API** - CRUD endpoints, hierarchy queries
3. **Frontend Foundation** - Store, API client, basic components
4. **Frontend UI** - Tree visualization, move operations, polish

Each phase is independently testable and deployable.

---

## Phase 1: Database Layer (2-3 hours)

### Goal
Enable ltree extension, add path column, create triggers for automatic path maintenance.

### Tasks

#### 1.1 Create Migration File
**File**: `database/migrations/000018_locations_add_ltree.up.sql`

```sql
SET search_path=trakrf,public;

-- Enable ltree extension
CREATE EXTENSION IF NOT EXISTS ltree;

-- Add path column (nullable for now, will backfill)
ALTER TABLE locations ADD COLUMN path ltree;

-- Add depth as generated column
ALTER TABLE locations ADD COLUMN depth INT GENERATED ALWAYS AS (
  CASE
    WHEN path IS NULL THEN NULL
    ELSE nlevel(path)
  END
) STORED;

-- Create function to calculate path from parent
CREATE OR REPLACE FUNCTION calculate_location_path(location_id INT)
RETURNS ltree AS $$
DECLARE
  location_identifier TEXT;
  parent_id INT;
  parent_path ltree;
BEGIN
  SELECT identifier, parent_location_id
  INTO location_identifier, parent_id
  FROM locations
  WHERE id = location_id;

  IF parent_id IS NULL THEN
    -- Root location: path is just the identifier
    RETURN text2ltree(location_identifier);
  ELSE
    -- Child location: parent path + identifier
    SELECT path INTO parent_path
    FROM locations
    WHERE id = parent_id;

    IF parent_path IS NULL THEN
      RAISE EXCEPTION 'Parent location % has no path', parent_id;
    END IF;

    RETURN parent_path || text2ltree(location_identifier);
  END IF;
END;
$$ LANGUAGE plpgsql;

-- Create trigger function to maintain path
CREATE OR REPLACE FUNCTION update_location_path()
RETURNS TRIGGER AS $$
BEGIN
  -- Calculate path for current location
  NEW.path = calculate_location_path(NEW.id);

  -- If this is an UPDATE and parent changed, update all descendants
  IF TG_OP = 'UPDATE' AND OLD.parent_location_id IS DISTINCT FROM NEW.parent_location_id THEN
    UPDATE locations
    SET path = calculate_location_path(id)
    WHERE path <@ OLD.path AND id != NEW.id;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger
CREATE TRIGGER maintain_location_path
  BEFORE INSERT OR UPDATE ON locations
  FOR EACH ROW
  EXECUTE FUNCTION update_location_path();

-- Backfill existing locations (recursive, starting from roots)
WITH RECURSIVE location_tree AS (
  -- Start with root locations
  SELECT id, identifier, parent_location_id, text2ltree(identifier) as new_path
  FROM locations
  WHERE parent_location_id IS NULL

  UNION ALL

  -- Recursively add children
  SELECT l.id, l.identifier, l.parent_location_id,
         lt.new_path || text2ltree(l.identifier)
  FROM locations l
  JOIN location_tree lt ON l.parent_location_id = lt.id
)
UPDATE locations l
SET path = lt.new_path
FROM location_tree lt
WHERE l.id = lt.id;

-- Add constraint to prevent self-reference
ALTER TABLE locations ADD CONSTRAINT no_self_reference
  CHECK (id != parent_location_id);

-- Create indexes
CREATE INDEX locations_path_gist_idx ON locations USING GIST (path);
CREATE INDEX locations_depth_idx ON locations(depth);

-- Add index on parent for faster joins
CREATE INDEX IF NOT EXISTS locations_parent_idx ON locations(parent_location_id)
  WHERE parent_location_id IS NOT NULL;
```

#### 1.2 Create Down Migration
**File**: `database/migrations/000018_locations_add_ltree.down.sql`

```sql
SET search_path=trakrf,public;

DROP INDEX IF EXISTS locations_parent_idx;
DROP INDEX IF EXISTS locations_depth_idx;
DROP INDEX IF EXISTS locations_path_gist_idx;

ALTER TABLE locations DROP CONSTRAINT IF EXISTS no_self_reference;

DROP TRIGGER IF EXISTS maintain_location_path ON locations;
DROP FUNCTION IF EXISTS update_location_path();
DROP FUNCTION IF EXISTS calculate_location_path(INT);

ALTER TABLE locations DROP COLUMN IF EXISTS depth;
ALTER TABLE locations DROP COLUMN IF EXISTS path;

DROP EXTENSION IF EXISTS ltree CASCADE;
```

#### 1.3 Test Migration
```bash
# Apply migration
docker exec backend migrate -path=/app/database/migrations -database="${PG_URL}" up

# Verify
docker exec timescaledb psql -U postgres -d postgres -c "
  SELECT id, identifier, path, depth, parent_location_id
  FROM trakrf.locations
  ORDER BY path
  LIMIT 10;
"

# Test trigger: Insert root location
docker exec timescaledb psql -U postgres -d postgres -c "
  SET search_path=trakrf,public;
  INSERT INTO locations (org_id, identifier, name)
  VALUES (1, 'test_root', 'Test Root')
  RETURNING id, path, depth;
"

# Test trigger: Insert child location
# (use returned ID from above as parent_location_id)
```

#### 1.4 Write Tests
**File**: `backend/internal/storage/locations_ltree_test.go`
- Test path generation for root locations
- Test path generation for nested locations
- Test path updates when moving locations
- Test descendant queries with ltree
- Test ancestor queries with ltree

### Validation
- [ ] Migration applies cleanly
- [ ] Down migration works
- [ ] All existing locations have paths
- [ ] Trigger updates path on INSERT
- [ ] Trigger updates descendant paths on parent change
- [ ] GiST index created successfully
- [ ] ltree tests passing

---

## Phase 2: Backend API (4-5 hours)

### Goal
Create full CRUD API with hierarchy-aware endpoints.

### Tasks

#### 2.1 Create Models
**File**: `backend/internal/models/location/location.go`

```go
package location

import "time"

type Location struct {
    ID               int       `json:"id"`
    OrgID            int       `json:"org_id"`
    Identifier       string    `json:"identifier"`
    Name             string    `json:"name"`
    Description      string    `json:"description"`
    ParentLocationID *int      `json:"parent_location_id"`
    Path             string    `json:"path"`
    Depth            int       `json:"depth"`
    ValidFrom        time.Time `json:"valid_from"`
    ValidTo          *time.Time `json:"valid_to"`
    IsActive         bool      `json:"is_active"`
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}

type CreateLocationRequest struct {
    Identifier       string `json:"identifier" validate:"required,max=255,alphanum"`
    Name             string `json:"name" validate:"required,max=255"`
    Description      string `json:"description" validate:"omitempty,max=1024"`
    ParentLocationID *int   `json:"parent_location_id" validate:"omitempty"`
}

type UpdateLocationRequest struct {
    Name        *string `json:"name" validate:"omitempty,max=255"`
    Description *string `json:"description" validate:"omitempty,max=1024"`
}

type MoveLocationRequest struct {
    NewParentID *int `json:"new_parent_id" validate:"omitempty"`
}

type LocationTreeNode struct {
    Location
    Children []LocationTreeNode `json:"children,omitempty"`
}
```

#### 2.2 Implement Storage Layer
**File**: `backend/internal/storage/locations.go`

Key functions:
- `ListLocations(ctx, orgID)` - All locations flat list
- `GetLocation(ctx, id, orgID)` - Single location
- `GetLocationAncestors(ctx, id, orgID)` - Full path to root
- `GetLocationDescendants(ctx, id, orgID)` - All children recursively
- `GetLocationChildren(ctx, id, orgID)` - Direct children only
- `CreateLocation(ctx, loc)` - Create new location
- `UpdateLocation(ctx, id, orgID, updates)` - Update location
- `DeleteLocation(ctx, id, orgID)` - Soft delete
- `MoveLocation(ctx, id, newParentID, orgID)` - Move to new parent
- `BuildLocationTree(ctx, orgID)` - Nested tree structure

**Key Queries**:
```go
// Ancestors (from root to location)
const queryAncestors = `
  SELECT l2.* FROM locations l1
  CROSS JOIN locations l2
  WHERE l1.id = $1
    AND l2.path @> l1.path
    AND l2.org_id = $2
  ORDER BY l2.depth ASC
`

// Descendants (all children recursively)
const queryDescendants = `
  SELECT l2.* FROM locations l1
  CROSS JOIN locations l2
  WHERE l1.id = $1
    AND l2.path <@ l1.path
    AND l2.org_id = $2
  ORDER BY l2.path
`

// Direct children only
const queryChildren = `
  SELECT * FROM locations
  WHERE parent_location_id = $1 AND org_id = $2
  ORDER BY name
`
```

#### 2.3 Create Service Layer (if needed)
**File**: `backend/internal/services/locations/service.go`

Add business logic:
- Validate parent exists before creating child
- Prevent circular references
- Prevent deleting location with children (or cascade)
- Validate identifier uniqueness within org

#### 2.4 Implement Handlers
**File**: `backend/internal/handlers/locations/locations.go`

```go
type Handler struct {
    storage *storage.Storage
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request)
func (h *Handler) Get(w http.ResponseWriter, r *http.Request)
func (h *Handler) GetAncestors(w http.ResponseWriter, r *http.Request)
func (h *Handler) GetDescendants(w http.ResponseWriter, r *http.Request)
func (h *Handler) GetChildren(w http.ResponseWriter, r *http.Request)
func (h *Handler) Create(w http.ResponseWriter, r *http.Request)
func (h *Handler) Update(w http.ResponseWriter, r *http.Request)
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request)
func (h *Handler) Move(w http.ResponseWriter, r *http.Request)

func (h *Handler) RegisterRoutes(r chi.Router)
```

#### 2.5 Register Routes
**File**: `backend/main.go`

```go
locationHandler := locations.NewHandler(store)
locationHandler.RegisterRoutes(r)
```

#### 2.6 Write Tests
**Files**:
- `backend/internal/storage/locations_test.go` - Unit tests
- `backend/internal/handlers/locations/locations_integration_test.go` - Integration tests

Test coverage:
- CRUD operations
- Hierarchy queries (ancestors, descendants, children)
- Move operations
- Org isolation
- Error cases (invalid parent, circular refs, etc.)

#### 2.7 Update Swagger Docs
Add location endpoints to OpenAPI spec.

### Validation
- [ ] All endpoints return correct data
- [ ] Org isolation working (can't see other org's locations)
- [ ] Hierarchy queries return correct results
- [ ] Move operation updates paths correctly
- [ ] Tests passing (unit + integration)
- [ ] Swagger docs updated

---

## Phase 3: Frontend Foundation (3-4 hours)

### Goal
API client, Zustand store, TypeScript types, basic hooks.

### Tasks

#### 3.1 Create TypeScript Types
**File**: `frontend/src/types/locations.ts`

```typescript
export interface Location {
  id: number;
  org_id: number;
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null;
  path: string;
  depth: number;
  valid_from: string;
  valid_to: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateLocationRequest {
  identifier: string;
  name: string;
  description?: string;
  parent_location_id?: number;
}

export interface UpdateLocationRequest {
  name?: string;
  description?: string;
}

export interface MoveLocationRequest {
  new_parent_id: number | null;
}

export interface LocationTreeNode extends Location {
  children?: LocationTreeNode[];
}
```

#### 3.2 Create API Client
**File**: `frontend/src/api/locations.ts`

```typescript
import { apiClient } from './client';
import type { Location, CreateLocationRequest, UpdateLocationRequest, LocationTreeNode } from '@/types/locations';

export const locationsApi = {
  list: () => apiClient.get<Location[]>('/locations'),

  get: (id: number) => apiClient.get<Location>(`/locations/${id}`),

  getAncestors: (id: number) =>
    apiClient.get<Location[]>(`/locations/${id}/ancestors`),

  getDescendants: (id: number) =>
    apiClient.get<Location[]>(`/locations/${id}/descendants`),

  getChildren: (id: number) =>
    apiClient.get<Location[]>(`/locations/${id}/children`),

  create: (data: CreateLocationRequest) =>
    apiClient.post<Location>('/locations', data),

  update: (id: number, data: UpdateLocationRequest) =>
    apiClient.put<Location>(`/locations/${id}`, data),

  delete: (id: number) =>
    apiClient.delete(`/locations/${id}`),

  move: (id: number, newParentId: number | null) =>
    apiClient.post<Location>(`/locations/${id}/move`, { new_parent_id: newParentId }),
};
```

#### 3.3 Create Zustand Store
**File**: `frontend/src/stores/locationStore.ts`

```typescript
import { create } from 'zustand';
import { locationsApi } from '@/api/locations';
import type { Location, CreateLocationRequest, UpdateLocationRequest, LocationTreeNode } from '@/types/locations';

interface LocationState {
  // Data
  locations: Map<number, Location>;
  selectedLocationId: number | null;

  // UI State
  isLoading: boolean;
  error: string | null;

  // Computed
  getLocation: (id: number) => Location | undefined;
  getLocationsByParent: (parentId: number | null) => Location[];
  getRootLocations: () => Location[];

  // Actions
  fetchLocations: () => Promise<void>;
  fetchLocation: (id: number) => Promise<void>;
  createLocation: (data: CreateLocationRequest) => Promise<Location>;
  updateLocation: (id: number, data: UpdateLocationRequest) => Promise<Location>;
  deleteLocation: (id: number) => Promise<void>;
  moveLocation: (id: number, newParentId: number | null) => Promise<Location>;
  selectLocation: (id: number | null) => void;
  clearError: () => void;
}

export const useLocationStore = create<LocationState>((set, get) => ({
  locations: new Map(),
  selectedLocationId: null,
  isLoading: false,
  error: null,

  getLocation: (id) => get().locations.get(id),

  getLocationsByParent: (parentId) => {
    return Array.from(get().locations.values())
      .filter(loc => loc.parent_location_id === parentId)
      .sort((a, b) => a.name.localeCompare(b.name));
  },

  getRootLocations: () => {
    return get().getLocationsByParent(null);
  },

  fetchLocations: async () => {
    set({ isLoading: true, error: null });
    try {
      const locations = await locationsApi.list();
      const map = new Map(locations.map(loc => [loc.id, loc]));
      set({ locations: map, isLoading: false });
    } catch (error) {
      set({ error: error.message, isLoading: false });
    }
  },

  fetchLocation: async (id) => {
    set({ isLoading: true, error: null });
    try {
      const location = await locationsApi.get(id);
      set(state => {
        const newMap = new Map(state.locations);
        newMap.set(id, location);
        return { locations: newMap, isLoading: false };
      });
    } catch (error) {
      set({ error: error.message, isLoading: false });
    }
  },

  createLocation: async (data) => {
    set({ isLoading: true, error: null });
    try {
      const location = await locationsApi.create(data);
      set(state => {
        const newMap = new Map(state.locations);
        newMap.set(location.id, location);
        return { locations: newMap, isLoading: false };
      });
      return location;
    } catch (error) {
      set({ error: error.message, isLoading: false });
      throw error;
    }
  },

  updateLocation: async (id, data) => {
    set({ isLoading: true, error: null });
    try {
      const location = await locationsApi.update(id, data);
      set(state => {
        const newMap = new Map(state.locations);
        newMap.set(id, location);
        return { locations: newMap, isLoading: false };
      });
      return location;
    } catch (error) {
      set({ error: error.message, isLoading: false });
      throw error;
    }
  },

  deleteLocation: async (id) => {
    set({ isLoading: true, error: null });
    try {
      await locationsApi.delete(id);
      set(state => {
        const newMap = new Map(state.locations);
        newMap.delete(id);
        return {
          locations: newMap,
          selectedLocationId: state.selectedLocationId === id ? null : state.selectedLocationId,
          isLoading: false
        };
      });
    } catch (error) {
      set({ error: error.message, isLoading: false });
      throw error;
    }
  },

  moveLocation: async (id, newParentId) => {
    set({ isLoading: true, error: null });
    try {
      const location = await locationsApi.move(id, newParentId);
      // Refetch all to get updated paths
      await get().fetchLocations();
      return location;
    } catch (error) {
      set({ error: error.message, isLoading: false });
      throw error;
    }
  },

  selectLocation: (id) => set({ selectedLocationId: id }),

  clearError: () => set({ error: null }),
}));
```

#### 3.4 Create Custom Hooks
**File**: `frontend/src/hooks/locations.ts`

```typescript
import { useLocationStore } from '@/stores/locationStore';
import { useEffect } from 'react';

export function useLocations() {
  const store = useLocationStore();

  useEffect(() => {
    store.fetchLocations();
  }, []);

  return {
    locations: Array.from(store.locations.values()),
    isLoading: store.isLoading,
    error: store.error,
  };
}

export function useLocation(id: number) {
  const store = useLocationStore();

  useEffect(() => {
    if (!store.locations.has(id)) {
      store.fetchLocation(id);
    }
  }, [id]);

  return {
    location: store.getLocation(id),
    isLoading: store.isLoading,
    error: store.error,
  };
}

export function useLocationMutations() {
  const store = useLocationStore();

  return {
    create: store.createLocation,
    update: store.updateLocation,
    delete: store.deleteLocation,
    move: store.moveLocation,
  };
}
```

#### 3.5 Write Tests
**Files**:
- `frontend/src/stores/locationStore.test.ts`
- `frontend/src/hooks/locations.test.ts`

### Validation
- [ ] Store state management working
- [ ] API calls succeed
- [ ] Error handling working
- [ ] Tests passing

---

## Phase 4: Frontend UI (6-8 hours)

### Goal
Complete location management UI with tree visualization, forms, and move operations.

### Tasks

#### 4.1 Create LocationsPage
**File**: `frontend/src/pages/LocationsPage.tsx`

Main page layout with:
- Tree view (left/main)
- Details panel (right)
- Create/Edit modals
- Search/filter

#### 4.2 Create LocationTree Component
**File**: `frontend/src/components/locations/LocationTree.tsx`

Recursive tree component:
- Expand/collapse nodes
- Select location
- Context menu (create child, edit, delete, move)
- Drag-and-drop to move (optional)

#### 4.3 Create LocationForm Component
**File**: `frontend/src/components/locations/LocationForm.tsx`

Form for create/edit:
- Identifier field
- Name field
- Description textarea
- Parent selector (dropdown or tree picker)
- Validation

#### 4.4 Create LocationCard Component
**File**: `frontend/src/components/locations/LocationCard.tsx`

Details view:
- Location info
- Breadcrumb path
- Edit/Delete buttons
- Child locations list

#### 4.5 Create LocationBreadcrumb Component
**File**: `frontend/src/components/locations/LocationBreadcrumb.tsx`

Shows full path: `USA > California > Warehouse 1 > Section A`

#### 4.6 Create LocationMoveDialog
**File**: `frontend/src/components/locations/LocationMoveDialog.tsx`

Dialog for moving location:
- Select new parent
- Show preview of new path
- Warn if moving to descendant (circular ref)

#### 4.7 Add Route
**File**: `frontend/src/App.tsx`

```tsx
<Route path="/locations" element={<LocationsPage />} />
```

#### 4.8 Write Component Tests
Test all components with React Testing Library.

#### 4.9 Write E2E Tests
**File**: `frontend/tests/e2e/locations.spec.ts`

- Create root location
- Create nested location
- Edit location
- Delete location
- Move location
- Search locations
- Navigate tree

### Validation
- [ ] Can create root locations
- [ ] Can create nested locations
- [ ] Can edit locations
- [ ] Can delete locations (prevents if has children)
- [ ] Can move locations
- [ ] Tree expands/collapses correctly
- [ ] Breadcrumb shows full path
- [ ] Search works
- [ ] All E2E tests passing
- [ ] Mobile responsive

---

## Testing Strategy

### Unit Tests
- Database trigger functions
- Storage layer queries
- API handlers
- Frontend store logic

### Integration Tests
- Full API request/response cycle
- Database constraints
- Org isolation

### E2E Tests
- Complete user flows
- Tree interaction
- CRUD operations
- Move operations

---

## Deployment Checklist

### Database
- [ ] Migration applied to production
- [ ] ltree extension enabled
- [ ] Existing locations have paths
- [ ] Indexes created

### Backend
- [ ] New endpoints deployed
- [ ] Swagger docs updated
- [ ] Monitoring added for new endpoints

### Frontend
- [ ] New route accessible
- [ ] Components bundled
- [ ] No console errors
- [ ] Performance acceptable (< 100ms render for 500 locations)

---

## Future Enhancements (Not in Scope)

- Bulk operations (create multiple, bulk move)
- Location templates
- Import/export location hierarchy
- Location photos/images
- Geographic coordinates/mapping
- Capacity tracking
- Access control per location
- Audit log for location changes
