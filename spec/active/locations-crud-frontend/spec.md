# Feature: Locations Management - Frontend Complete

## Metadata
**Workspace**: frontend
**Type**: feature

## Outcome
A complete mobile-first frontend implementation for hierarchical location management, providing intuitive CRUD operations, parent/subsidiary navigation, and location tree visualization - fully integrated with the production-ready backend API.

**UI Design**: Identical/very similar to Assets screen for consistency and familiarity.

## User Story

**As a warehouse manager**
I want to view, create, edit, and delete locations through an intuitive interface
So that I can organize my facilities in a hierarchical structure and easily navigate between parent and subsidiary locations

**As a facility operator**
I want to see the full path of any location (e.g., USA → California → Warehouse 1 → Section A)
So that I understand exactly where a location sits in the organizational hierarchy

**As a system administrator**
I want to visualize the entire location tree
So that I can understand the complete structure of all locations at a glance

## Context

**Current**: Backend API is fully implemented and production-ready with ltree-based hierarchical storage. No frontend UI exists for location management.

**Desired**: Complete frontend implementation with:
- Full CRUD operations (Create, Read, Update, Delete)
- Parent/subsidiary location navigation with clear, simple naming
- Location path breadcrumbs showing full hierarchy
- Optional tree view for visualizing entire structure
- Type-safe API integration with existing backend
- Responsive design for mobile and desktop
- Real-time cache management with Zustand

**Examples**:
- Assets screen pattern: `frontend/src/components/AssetsScreen.tsx`
- Asset hooks pattern: `frontend/src/hooks/assets/useAssets.ts`
- Asset store pattern: `frontend/src/stores/assets/assetStore.ts`
- Asset API pattern: `frontend/src/lib/api/assets/index.ts`

## Technical Requirements

### Component Design Principles

**CRITICAL: Keep Components Dumb and Reusable**

1. **No Logic in TSX Components**
   - Components should ONLY render UI and call functions
   - Move ALL logic to hooks, utils, or store actions
   - Components are presentation-only, "dumb" wrappers
   - Maximum reusability through separation of concerns

2. **Extract All Helper Functions**
   - Never define helper functions inside component files
   - Move to `lib/location/` utilities or custom hooks
   - Components should only call pre-defined functions

3. **Reuse Existing Shared Components**
   - **ALWAYS check** `components/shared/` before creating new components
   - **Available shared components:**
     - Buttons: `FloatingActionButton`
     - Empty States: `EmptyState`, `NoResults`
     - Modals: `ConfirmModal`
     - Pagination: `PaginationButtons`, `PaginationControls`, `PaginationMobile`
     - Loaders: `SkeletonBase`, `SkeletonCard`, `SkeletonForm`, `SkeletonTableRow`
     - Banners: `ErrorBanner`
     - Layout: `Container`
     - Alerts: `ErrorAlert`, `SuccessAlert`, `ProcessingAlert`, `GlobalUploadAlert`
   - **DO NOT duplicate** components that already exist
   - Extend existing components via props, not by copying code

4. **Code Style**
   - Avoid unnecessary comments (code should be self-documenting)
   - Avoid overly complex code (keep it simple)
   - Use descriptive variable/function names instead of comments
   - Remove any commented-out code before committing

**Example of GOOD vs BAD component design:**

```typescript
// ❌ BAD - Logic mixed into component
function LocationTable({ locations }) {
  const filterActive = (locs) => locs.filter(l => l.is_active);
  const sortByName = (locs) => [...locs].sort((a, b) => a.name.localeCompare(b.name));

  const processed = sortByName(filterActive(locations));

  return <table>...</table>;
}

// ✅ GOOD - Component calls pre-defined functions
function LocationTable({ locations }) {
  const activeLocations = useLocationStore(state => state.getActiveLocations());
  const sortedLocations = useLocationStore(state => state.getFilteredLocations());

  return <table>...</table>;
}
```

**Component Responsibilities:**
- ✅ Render UI elements
- ✅ Call hooks for data
- ✅ Pass data to child components
- ✅ Handle user events (onClick, onChange) by calling store actions or callbacks
- ❌ NO data transformation logic
- ❌ NO filtering/sorting logic
- ❌ NO validation logic
- ❌ NO helper functions

### Mobile-First Design (CRITICAL)

**Priority: Mobile users first (phones + tablets), then optimize for desktop**

**Mobile = phones + tablets** (anything below md breakpoint: <768px)

Based on Assets screen implementation patterns:

**1. Responsive Layout Pattern**
```tsx
{/* Desktop Table - hidden on mobile/tablet */}
<LocationTable className="hidden md:block" {...props} />

{/* Mobile/Tablet Cards - hidden on desktop */}
<div className="md:hidden space-y-3">
  {locations.map(location => (
    <LocationCard key={location.id} location={location} variant="card" />
  ))}
</div>
```

**Breakpoints:**
- Mobile/Tablet: `<768px` (default, md:hidden)
- Desktop: `≥768px` (md:block)
- Cards for phones AND tablets
- Table only for desktop screens

**2. Mobile/Tablet Card Design (PRIMARY - optimize this first)**
- Clean border with rounded corners: `rounded-lg border`
- Touch-friendly padding: `p-4` (16px all around)
- Icon + title header section
- Key info displayed vertically (easy to scan on phone/tablet)
- Status badge clearly visible
- **Full-width action buttons** at bottom (not icons)
- Touch-friendly tap targets: 44x44px minimum (for phones AND tablets)
- Hover effects for feedback (tablets support hover)
- Works great on both portrait phones and landscape tablets

**Mobile Card Structure:**
```tsx
<div className="border rounded-lg p-4 hover:border-blue-500 hover:bg-blue-50">
  {/* Header: Icon + Identifier */}
  <div className="flex items-start gap-3 mb-3">
    <Icon className="h-6 w-6" />
    <div className="flex-1">
      <h3 className="text-base font-semibold">{identifier}</h3>
      <p className="text-sm text-gray-700">{name}</p>
    </div>
  </div>

  {/* Parent location (if applicable) */}
  {parent && (
    <div className="mb-3">
      <p className="text-sm">
        <span className="font-medium">Parent:</span> {parent.name}
      </p>
    </div>
  )}

  {/* Status badge */}
  <div className="mb-4">
    <span className="inline-flex px-3 py-1 rounded-full text-xs">
      Active ✓
    </span>
  </div>

  {/* Actions - FULL WIDTH buttons */}
  <div className="flex gap-2 pt-3 border-t">
    <button className="flex-1 flex items-center justify-center gap-2">
      <Pencil className="h-4 w-4" />
      Edit
    </button>
    <button className="flex-1 flex items-center justify-center gap-2">
      <Trash2 className="h-4 w-4" />
      Delete
    </button>
  </div>
</div>
```

**3. Desktop Table Design (SECONDARY)**
- `overflow-x-auto` for horizontal scroll
- Sortable column headers with icons
- Row hover effects: `hover:bg-blue-50`
- Compact inline action buttons (icon only)
- Hidden on mobile: `hidden md:block`

**4. Screen Layout Structure**
```tsx
<div className="h-full flex flex-col p-2">
  {/* Search/Sort controls - always visible */}
  <LocationSearchSort />

  {/* Content area - scrollable */}
  <div className="flex-1 flex flex-col gap-4 min-w-0">
    {/* Empty state */}
    {!loading && locations.length === 0 && <EmptyState />}

    {/* Desktop table */}
    <LocationTable className="hidden md:block" />

    {/* Mobile cards */}
    <div className="md:hidden space-y-3">
      {locations.map(loc => <LocationCard key={loc.id} />)}
    </div>
  </div>

  {/* Stats at bottom */}
  <LocationStats className="mt-6" />

  {/* Floating Action Button for create */}
  <FloatingActionButton icon={Plus} onClick={handleCreate} />
</div>
```

**5. Component Variants Pattern**
LocationCard should support dual variants (same as AssetCard):
- `variant="card"` - Mobile card view (vertical layout, full-width buttons)
- `variant="row"` - Table row view (horizontal layout, inline icons)

**6. Touch-Friendly Design (Phones + Tablets)**
- Minimum tap target: 44x44px
- Clear visual feedback on tap (hover states work on tablets too)
- No tiny icon-only buttons on mobile/tablet
- Text labels on all mobile/tablet buttons
- Adequate spacing between interactive elements
- Optimized for both finger touch (phones) and larger touch areas (tablets)

**7. Performance on Mobile/Tablet**
- Debounced search (300ms)
- Virtual scrolling if >100 items
- Lazy load images/icons
- Optimize re-renders with memoization
- Fast initial load (code splitting)
- Test on both phones and tablets for performance

**8. Mobile Patterns to Follow from Assets:**
- ✅ Same layout structure
- ✅ Same spacing and padding
- ✅ Same color scheme and hover effects
- ✅ Same modal/dialog behavior
- ✅ Same empty states
- ✅ Same loading skeletons
- ✅ Same floating action button position/style

### File Structure

```
frontend/src/
├── types/
│   └── locations/
│       └── index.ts                      # All TypeScript types and interfaces
├── lib/
│   ├── api/
│   │   └── locations/
│   │       └── index.ts                  # API client (8 endpoints)
│   └── location/
│       ├── validators.ts                 # Validation functions
│       ├── transforms.ts                 # Data transformations and formatting
│       └── filters.ts                    # Filter/sort logic
├── stores/
│   └── locations/
│       ├── locationStore.ts              # Zustand store with hierarchical cache
│       ├── locationActions.ts            # Store actions (cache, UI, hierarchy)
│       └── locationPersistence.ts        # LocalStorage persistence
├── hooks/
│   └── locations/
│       ├── useLocations.ts               # List locations with React Query
│       ├── useLocation.ts                # Single location query
│       ├── useLocationMutations.ts       # Create/Update/Delete mutations
│       └── useLocationHierarchy.ts       # Hierarchy navigation (parents, subsidiaries)
├── components/
│   ├── LocationsScreen.tsx               # Main locations page
│   └── locations/
│       ├── LocationTable.tsx             # Desktop table view
│       ├── LocationCard.tsx              # Mobile card view
│       ├── LocationFormModal.tsx         # Create/Edit modal wrapper
│       ├── LocationForm.tsx              # Form component
│       ├── LocationDetailsModal.tsx      # View location details
│       ├── LocationBreadcrumb.tsx        # Hierarchical path navigation
│       ├── LocationParentSelector.tsx    # Parent location picker
│       ├── LocationHierarchyPanel.tsx    # Parent/subsidiary navigation panel
│       ├── LocationStats.tsx             # Location statistics
│       ├── LocationSearchSort.tsx        # Search and sort controls
│       └── LocationTreeView.tsx          # Tree visualization (Phase 2)
└── pages/
    └── LocationsPage.tsx                 # Route entry point

```

### Layer 1: Types & Interfaces (`types/locations/index.ts`)

**Core Types** (matching backend):
```typescript
export interface Location {
  id: number;
  org_id: number;
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null;
  path: string;                    // ltree path (e.g., "usa.california.warehouse_1")
  depth: number;                   // Generated column from path
  valid_from: string;              // ISO date
  valid_to: string | null;         // ISO date
  is_active: boolean;
  metadata: Record<string, any>;
  created_at: string;
  updated_at: string;
}

export interface LocationTreeNode {
  location: Location;
  children: LocationTreeNode[];
}

export interface CreateLocationRequest {
  identifier: string;
  name: string;
  description?: string;
  parent_location_id?: number | null;
  metadata?: Record<string, any>;
}

export interface UpdateLocationRequest {
  name?: string;
  description?: string;
  metadata?: Record<string, any>;
}

export interface MoveLocationRequest {
  new_parent_id: number | null;
}

export interface LocationResponse {
  data: Location;
}

export interface ListLocationsResponse {
  data: Location[];
  total_count: number;
}

export interface DeleteResponse {
  message: string;
}
```

**UI State Types**:
```typescript
export interface LocationFilters {
  search: string;              // Search by identifier or name
  identifier?: string;         // Filter by specific identifier
  created_after?: string;      // Filter by created date (ISO)
  created_before?: string;     // Filter by created date (ISO)
  is_active: 'all' | 'active' | 'inactive';
}

export interface LocationSort {
  field: 'name' | 'identifier' | 'created_at';
  direction: 'asc' | 'desc';
}

export interface PaginationState {
  currentPage: number;
  pageSize: number;
  totalCount: number;
  totalPages: number;
}
```

**Cache Types**:
```typescript
export interface LocationCache {
  byId: Map<number, Location>;
  byIdentifier: Map<string, Location>;
  byParentId: Map<number | null, Set<number>>; // Parent → Children mapping
  rootIds: Set<number>;                        // Root locations (no parent)
  activeIds: Set<number>;                      // Active locations only
  allIds: number[];                            // Ordered for iteration
  allIdentifiers: string[];                    // Cached list for filters (avoid recalculation)
  lastFetched: number;                         // TTL tracking
  ttl: number;                                 // Cache duration
}
```

### Layer 2: API Client (`lib/api/locations/index.ts`)

**8 API Methods** (matching backend):
```typescript
export const locationsApi = {
  // Core CRUD
  list: (options?: ListLocationsOptions) => Promise<AxiosResponse<ListLocationsResponse>>
  get: (id: number) => Promise<AxiosResponse<LocationResponse>>
  create: (data: CreateLocationRequest) => Promise<AxiosResponse<LocationResponse>>
  update: (id: number, data: UpdateLocationRequest) => Promise<AxiosResponse<LocationResponse>>
  delete: (id: number) => Promise<AxiosResponse<DeleteResponse>>

  // Hierarchy operations
  getParents: (id: number) => Promise<AxiosResponse<ListLocationsResponse>>      // Get all ancestors
  getSubsidiaries: (id: number) => Promise<AxiosResponse<ListLocationsResponse>> // Get all descendants
  move: (id: number, data: MoveLocationRequest) => Promise<AxiosResponse<LocationResponse>>
}
```

**Requirements**:
- Use existing `apiClient` from `lib/api/client.ts`
- JWT auto-injected via interceptor
- Type-safe request/response
- Proper error propagation (RFC 7807)

### Layer 3: Zustand Store (`stores/locations/locationStore.ts`)

**Hierarchical Cache** (optimized for tree operations):
```typescript
interface LocationStore {
  // ============ Cache State ============
  cache: LocationCache;

  // ============ UI State ============
  selectedLocationId: number | null;
  filters: LocationFilters;
  pagination: PaginationState;
  sort: LocationSort;
  expandedNodeIds: Set<number>;    // For tree view

  // ============ Cache Actions ============
  addLocations: (locations: Location[]) => void;
  addLocation: (location: Location) => void;
  updateCachedLocation: (id: number, updates: Partial<Location>) => void;
  removeLocation: (id: number) => void;
  invalidateCache: () => void;

  // ============ Cache Queries ============
  getLocationById: (id: number) => Location | undefined;
  getLocationByIdentifier: (identifier: string) => Location | undefined;
  getRootLocations: () => Location[];
  getSubsidiaries: (parentId: number) => Location[];      // Direct children only
  getAllSubsidiaries: (parentId: number) => Location[];   // All descendants
  getParentLocation: (id: number) => Location | undefined;
  getLocationPath: (id: number) => Location[];            // Full path from root
  getAllIdentifiers: () => string[];                       // Cached, sorted list
  getActiveLocations: () => Location[];
  getFilteredLocations: () => Location[];

  // ============ UI State Actions ============
  setFilters: (filters: Partial<LocationFilters>) => void;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
  setSort: (field: LocationSort['field'], direction: LocationSort['direction']) => void;
  setSearchTerm: (term: string) => void;
  resetPagination: () => void;
  selectLocation: (id: number | null) => void;
  getSelectedLocation: () => Location | undefined;
  toggleNodeExpansion: (id: number) => void;
  expandNode: (id: number) => void;
  collapseNode: (id: number) => void;
}
```

**LocalStorage Persistence**:
- Persist cache with Map/Set serialization
- Persist filters, pagination, sort, expandedNodeIds
- Persist cached identifiers list to avoid recalculation
- 1-hour TTL on cache

### Layer 4: Business Logic (Pure Functions)

**Validators** (`lib/location/validators.ts`):
```typescript
// Identifier validation (ltree-safe)
validateIdentifier(identifier: string): string | null

// Name validation
validateName(name: string): string | null

// Parent relationship validation
validateParentRelationship(locationId: number, newParentId: number | null, locations: Location[]): string | null

// Prevent circular references
detectCircularReference(locationId: number, newParentId: number, locations: Location[]): boolean

// RFC 7807 error extraction
extractErrorMessage(err: any): string
```

**Transforms** (`lib/location/transforms.ts`):
```typescript
// Format path for display (usa.california → USA → California)
formatPath(path: string): string[]

// Format path as breadcrumb string
formatPathBreadcrumb(path: string): string

// Build tree structure from flat list
buildLocationTree(locations: Location[]): LocationTreeNode[]

// Flatten tree to array
flattenLocationTree(nodes: LocationTreeNode[]): Location[]

// Cache serialization
serializeCache(cache: LocationCache): SerializedCache
deserializeCache(data: SerializedCache): LocationCache
```

**Filters** (`lib/location/filters.ts`):
```typescript
// Filter by search term (identifier or name only - simple)
searchLocations(locations: Location[], searchTerm: string): Location[]

// Filter by specific identifier
filterByIdentifier(locations: Location[], identifier: string): Location[]

// Filter by created date range
filterByCreatedDate(locations: Location[], after?: string, before?: string): Location[]

// Filter by active status
filterByActiveStatus(locations: Location[], status: 'all' | 'active' | 'inactive'): Location[]

// Apply all filters (simple combination)
filterLocations(locations: Location[], filters: LocationFilters): Location[]

// Sort locations (simple fields only)
sortLocations(locations: Location[], sort: LocationSort): Location[]

// Paginate locations
paginateLocations(locations: Location[], pagination: PaginationState): Location[]

// Get unique identifiers (memoized in cache)
extractUniqueIdentifiers(locations: Location[]): string[]
```

### Layer 5: React Hooks

**useLocations** (`hooks/locations/useLocations.ts`):
```typescript
// Fetches and caches all locations using React Query
export function useLocations(options?: UseLocationsOptions) {
  // React Query integration
  // Auto-updates cache via store
  return {
    locations: Location[];
    totalCount: number;
    isLoading: boolean;
    isRefetching: boolean;
    error: any;
    refetch: () => void;
  };
}
```

**useLocation** (`hooks/locations/useLocation.ts`):
```typescript
// Fetches single location by ID
export function useLocation(id: number | null, options?: UseLocationOptions) {
  return {
    location: Location | undefined;
    isLoading: boolean;
    error: any;
    refetch: () => void;
  };
}
```

**useLocationMutations** (`hooks/locations/useLocationMutations.ts`):
```typescript
// Create, update, delete mutations
export function useLocationMutations() {
  return {
    create: useMutation(createFn),
    update: useMutation(updateFn),
    delete: useMutation(deleteFn),
    move: useMutation(moveFn),
  };
}
```

**useLocationHierarchy** (`hooks/locations/useLocationHierarchy.ts`):
```typescript
// Hierarchy navigation helpers
export function useLocationHierarchy(locationId: number | null) {
  return {
    parentLocation: Location | undefined;
    subsidiaries: Location[];           // Direct children
    allSubsidiaries: Location[];        // All descendants
    locationPath: Location[];           // Full path from root
    isRoot: boolean;
    hasSubsidiaries: boolean;
    fetchParents: () => Promise<void>;
    fetchSubsidiaries: () => Promise<void>;
  };
}
```

### Layer 6: UI Components

#### Main Screen (`components/LocationsScreen.tsx`)
**Features**:
- Responsive table (desktop) and card (mobile) views
- Search and sort controls
- Pagination
- Create/Edit/Delete modals
- View details modal with hierarchy navigation
- Empty state when no locations
- No results state when search returns empty
- Location statistics

**Pattern**: Follow `AssetsScreen.tsx` structure

#### Table & Card Views (MOBILE FIRST - Cards are PRIMARY)

**LocationCard.tsx** (PRIORITY - Mobile/Tablet First):
- **Dual variants**: `variant="card"` (mobile/tablet) and `variant="row"` (table)
- **Mobile/Tablet card layout** (variant="card"):
  - Clean border with rounded corners: `border rounded-lg p-4`
  - Touch-friendly 44x44px tap targets (phones + tablets)
  - Icon + identifier + name header
  - Parent location shown (if applicable)
  - Status badge (Active/Inactive)
  - **Full-width action buttons** (Edit + Delete) at bottom
  - Text labels on buttons, not icon-only
  - Hover effect: `hover:border-blue-500 hover:bg-blue-50` (works on tablets)
  - Click/tap card to view details
  - Works on phones (portrait) and tablets (portrait/landscape)
- **Table row layout** (variant="row"):
  - Used inside LocationTable
  - Horizontal layout with columns
  - Compact inline action buttons (icon only)
  - Desktop only
- **Pattern**: Follow `AssetCard.tsx` exactly (same structure, same styling)

**LocationTable.tsx** (SECONDARY - Desktop Only):
- Hidden on mobile/tablet: `hidden md:block`
- **Desktop table with columns**: Identifier, Name, Parent, Created At, Active, Actions
- Click row to view details
- Sortable columns with icons: identifier, name, created_at
- Edit/Delete icon buttons inline
- Row hover effect: `hover:bg-blue-50`
- Uses LocationCard with `variant="row"`
- Simple, fast rendering for 500+ locations
- No complex tree rendering in table
- `overflow-x-auto` for horizontal scroll
- **Desktop only** (≥768px)
- **Pattern**: Follow `AssetTable.tsx` exactly

#### Forms
**LocationFormModal.tsx**:
- Wrapper for create/edit modes
- Handles API calls and cache updates
- Error handling and loading states

**LocationForm.tsx**:
- Identifier input (validates ltree-safe characters)
- Name input (required)
- Description textarea (optional)
- Parent location selector (dropdown with search)
- Metadata JSON editor (optional)
- Client-side validation

**LocationParentSelector.tsx**:
- Searchable dropdown of all locations
- Shows location paths for clarity
- Option for "No parent" (root location)
- Prevents selecting self or descendants as parent

#### Details & Navigation
**LocationDetailsModal.tsx**:
- Full location details
- Breadcrumb path to root
- Parent location (clickable)
- List of subsidiaries (clickable)
- Edit/Delete actions

**LocationBreadcrumb.tsx**:
- Shows full path: USA → California → Warehouse 1 → Section A
- Each segment clickable to navigate
- Compact on mobile

**LocationHierarchyPanel.tsx**:
- Shows parent location (if any)
- Shows subsidiaries list (if any)
- Click to navigate to parent or subsidiary
- Expandable/collapsible

#### Statistics
**LocationStats.tsx**:
- Total locations count
- Active/Inactive counts
- Recent locations count (created in last 7 days)
- Root locations count

#### Tree View (SECONDARY - Phase 6)
**LocationTreeView.tsx**:
- **Use existing library** instead of building from scratch
- **Recommended libraries** (evaluate and choose best fit):
  1. **react-complex-tree** - Full-featured, accessible, keyboard support
  2. **react-accessible-treeview** - Lightweight, ARIA-compliant
  3. **@headlessui/react** with custom tree logic - Full control, minimal bundle
- Hierarchical tree visualization
- Expandable/collapsible nodes
- Search/filter in tree
- Click to select location
- Visual depth indicators
- Handles large trees (500+ locations)
- Lazy loading for performance

## Implementation Phases

### Phase 1: Foundation (Data Layer) - 2 days
**Goal**: Complete data layer with no UI

**Tasks**:
1. Create types (`types/locations/index.ts`)
2. Create API client (`lib/api/locations/index.ts`)
3. Create validators (`lib/location/validators.ts`)
4. Create transforms (`lib/location/transforms.ts`)
5. Create filters (`lib/location/filters.ts`)
6. Unit tests for all pure functions

**Validation**:
- [ ] All types compile without errors
- [ ] API client methods properly typed
- [ ] All unit tests passing

### Phase 2: State Management - 2 days
**Goal**: Zustand store with hierarchical cache

**Tasks**:
1. Create location store (`stores/locations/locationStore.ts`)
2. Create store actions (`stores/locations/locationActions.ts`)
3. Create persistence layer (`stores/locations/locationPersistence.ts`)
4. Implement all cache operations
5. Implement hierarchy queries
6. Unit tests for store and cache

**Validation**:
- [ ] Cache operations maintain index consistency
- [ ] Hierarchy queries return correct results
- [ ] LocalStorage persistence works
- [ ] All store tests passing

### Phase 3: React Hooks - 1 day
**Goal**: React Query integration and custom hooks

**Tasks**:
1. Create `useLocations` hook
2. Create `useLocation` hook
3. Create `useLocationMutations` hook
4. Create `useLocationHierarchy` hook
5. Unit tests for hooks

**Validation**:
- [ ] Hooks properly integrate with React Query
- [ ] Cache updates on mutations
- [ ] All hook tests passing

### Phase 4: Core UI - Table Views First - 3 days
**Goal**: Main screen with basic table CRUD (NO tree view yet)

**Tasks**:
1. Create `LocationsScreen.tsx` (table/card views only)
2. Create `LocationTable.tsx` (simple columns, no tree)
3. Create `LocationCard.tsx` (mobile view)
4. Create `LocationFormModal.tsx`
5. Create `LocationForm.tsx`
6. Create `LocationParentSelector.tsx` (simple dropdown)
7. Create `LocationStats.tsx`
8. Create `LocationSearchSort.tsx` (simple filters)
9. Component tests

**Validation**:
- [ ] Can create root locations
- [ ] Can create subsidiary locations
- [ ] Can edit locations
- [ ] Can delete locations
- [ ] Search by identifier/name works
- [ ] Filter by identifier works
- [ ] Filter by created date works
- [ ] Sort works (identifier, name, created_at)
- [ ] Pagination works
- [ ] Table renders 500+ locations smoothly
- [ ] Mobile responsive
- [ ] All component tests passing

### Phase 5: Details & Navigation - 2 days
**Goal**: View details with parent/subsidiary links

**Tasks**:
1. Create `LocationDetailsModal.tsx`
2. Create `LocationBreadcrumb.tsx`
3. Create `LocationHierarchyPanel.tsx`
4. Integrate hierarchy panel into details modal
5. Implement parent/subsidiary links (clickable, opens details)
6. Component tests

**Validation**:
- [ ] Can view location details
- [ ] Can see parent location (clickable)
- [ ] Can see subsidiaries list (clickable)
- [ ] Breadcrumb shows full path
- [ ] Clicking breadcrumb segment opens that location's details
- [ ] All navigation tests passing

### Phase 6: Tree View (SECONDARY/OPTIONAL) - 2-3 days
**Goal**: Visual tree representation using library

**Tasks**:
1. **Evaluate tree libraries** (react-complex-tree, react-accessible-treeview)
2. **Choose best library** based on:
   - Bundle size
   - Accessibility (ARIA, keyboard nav)
   - Performance with 500+ nodes
   - Customization options
   - Active maintenance
3. Install chosen library
4. Create `LocationTreeView.tsx` wrapper
5. Implement data transformation (Location[] → tree format)
6. Integrate search/filter in tree
7. Add toggle to switch between table and tree view
8. Component tests

**Validation**:
- [ ] Tree library integrates cleanly
- [ ] Tree renders correctly
- [ ] Expand/collapse works
- [ ] Search in tree works
- [ ] Handles 500+ locations without lag
- [ ] Keyboard navigation works
- [ ] Screen reader compatible
- [ ] Toggle between table and tree view works
- [ ] All tree tests passing

**Total Estimated Time**: 10-12 days

## Validation Criteria

### Functional
- [ ] Can create root locations (no parent)
- [ ] Can create subsidiary locations (with parent)
- [ ] Can edit location name and description
- [ ] Can delete locations (with proper validation)
- [ ] Can view location details
- [ ] Can navigate to parent location
- [ ] Can view list of subsidiaries
- [ ] Can navigate to subsidiary locations
- [ ] Breadcrumb shows full path from root
- [ ] Search works (identifier and name)
- [ ] Filter by specific identifier works
- [ ] Filter by created date range works
- [ ] Filter by active status works
- [ ] Sort by identifier, name, created_at works
- [ ] Pagination works correctly
- [ ] Mobile/tablet responsive on all screens (test phones + tablets first, then desktop)
- [ ] Mobile/tablet cards work perfectly (44x44px tap targets, full-width buttons)
- [ ] Desktop table hidden on mobile/tablet (`hidden md:block`)
- [ ] Mobile/tablet cards hidden on desktop (`md:hidden`)
- [ ] Tested on phone screens (320px-480px)
- [ ] Tested on tablet screens (481px-767px)
- [ ] Tested on desktop screens (768px+)
- [ ] LocationCard supports both variants (card and row)
- [ ] UI matches Assets screen patterns (layout, spacing, colors)
- [ ] Identifier list cached and doesn't recalculate on every render

### Technical
- [ ] All types match backend API exactly
- [ ] API client uses existing `apiClient`
- [ ] Store follows Zustand patterns from assets
- [ ] Hooks use React Query properly
- [ ] Components are "dumb" (no logic, only render and call functions)
- [ ] No helper functions defined inside TSX components
- [ ] All logic extracted to hooks, utils, or store actions
- [ ] Reused existing shared components (no duplicates)
- [ ] No unnecessary comments (code is self-documenting)
- [ ] No overly complex code (KISS principle applied)
- [ ] Components follow existing UI patterns
- [ ] No console errors or warnings
- [ ] Proper error handling and user feedback
- [ ] Cache invalidation on mutations
- [ ] LocalStorage persistence works

### Testing
- [ ] All unit tests passing (target: 30+ tests)
- [ ] All component tests passing (target: 15+ tests)
- [ ] All integration tests passing
- [ ] Type safety: 100% TypeScript coverage
- [ ] Zero console errors during tests

### Performance
- [ ] Cache lookups O(1) for byId and byIdentifier
- [ ] Mobile/tablet card list handles 500+ locations smoothly
- [ ] Desktop table handles 500+ locations smoothly
- [ ] Tree view handles 500+ locations without lag
- [ ] Search results update in <100ms
- [ ] Search input debounced at 300ms
- [ ] No unnecessary re-renders
- [ ] Mobile/tablet loads fast (<2s initial load on 3G)

## Success Metrics

- [ ] All tests passing (45+ tests minimum)
- [ ] Type safety: 100% TypeScript coverage, no `any` types except metadata
- [ ] Zero console errors or warnings
- [ ] Mobile responsive on all breakpoints
- [ ] Accessibility: keyboard navigation and ARIA labels
- [ ] User can complete full CRUD workflow without errors
- [ ] Hierarchy navigation is intuitive and fast
- [ ] Performance: No lag with 500+ locations

## References

### Existing Patterns
- Assets screen: `frontend/src/components/AssetsScreen.tsx`
- Assets store: `frontend/src/stores/assets/assetStore.ts`
- Assets hooks: `frontend/src/hooks/assets/`
- Assets API: `frontend/src/lib/api/assets/index.ts`
- Auth store: `frontend/src/stores/authStore.ts`

### Backend API
- Locations handler: `backend/internal/handlers/locations/locations.go`
- Locations storage: `backend/internal/storage/locations.go`
- Location models: `backend/internal/models/location/location.go`

### Documentation
- Locations CRUD spec: `spec/active/locations-crud/spec.md`
- ltree usage: `spec/active/locations-crud/ltree-usage-example.md`
- Backend plan: `spec/active/locations-crud/plan.md`

### UI Libraries
- **React Hook Form**: form management
- **React Query**: data fetching and caching
- **Zustand**: state management
- **Lucide React**: icons
- **Tailwind CSS**: styling
- **Radix UI or Headless UI**: accessible components (dropdowns, modals)

### Tree View Libraries (Phase 6 - Evaluate & Choose)
- **react-complex-tree**: https://rct.lukasbach.com/
  - Pros: Full-featured, accessible, keyboard support, drag-and-drop
  - Cons: Larger bundle size
- **react-accessible-treeview**: https://dgreene1.github.io/react-accessible-treeview/
  - Pros: Lightweight, ARIA-compliant, good performance
  - Cons: Less features out-of-box
- **@headlessui/react** with custom logic:
  - Pros: Full control, minimal bundle, already in project
  - Cons: More implementation work

**Recommendation**: Start with `react-accessible-treeview` for best balance of accessibility, performance, and simplicity. Fallback to `react-complex-tree` if more features needed.

## Non-Goals (Out of Scope)

- Location templates or bulk creation (CSV upload)
- Location images/photos
- Location-based access control (beyond org isolation)
- Geographic coordinates/mapping integration
- QR code generation for locations
- Location capacity/occupancy tracking
- Move operations via drag-and-drop (may add later)
- Bulk move operations

These features can be added in future iterations if needed.

## Notes

### Naming Conventions
**User-Friendly Terms** (instead of technical jargon):
- ✅ "Parent location" instead of "ancestor"
- ✅ "Subsidiaries" instead of "children" or "descendants"
- ✅ "Sub-locations" as alternative to "subsidiaries"
- ✅ "Main location" for root/top-level locations

### Performance Optimizations
**Identifier Caching**:
- Cache all identifiers in `cache.allIdentifiers` array
- Update only when locations added/removed
- Use memoized list for filter dropdowns/autocomplete
- Avoid recalculation on every render
- Sort identifiers once when caching

**Table Performance**:
- Render simple table first (no tree complexity in table view)
- Virtual scrolling for 500+ rows (use `@tanstack/react-virtual` if needed)
- Debounce search input (300ms)
- Lazy load details only when modal opens
- Memoize filtered/sorted results

### Path Display
- Show paths as breadcrumbs: `USA → California → Warehouse 1`
- Make each segment clickable for navigation
- Truncate on mobile with ellipsis: `USA → ... → Warehouse 1`

### Parent Selection
- Prevent selecting self as parent
- Prevent selecting own subsidiaries as parent (circular reference)
- Warn when moving location with subsidiaries (affects entire subtree)

### Delete Behavior
- Cannot delete location with subsidiaries (prevent orphans)
- Soft delete: sets `is_active = false` instead of removing
- Show warning if location has subsidiaries

### Mobile/Tablet Considerations
- Stack form fields vertically (phones + tablets)
- Use bottom sheets for modals on mobile (phones)
- Full modals work well on tablets
- Simplify breadcrumbs on small screens (phones)
- Touch-friendly tap targets (44x44px minimum for phones + tablets)
- Test on both portrait and landscape orientations
- Phones: 320px-480px (portrait), 480px-768px (landscape)
- Tablets: 481px-768px (portrait), 768px-1024px (landscape, but shows desktop view)

### Accessibility
- Keyboard navigation for all interactive elements
- ARIA labels for screen readers
- Focus management in modals
- Clear error messages
- Loading states with announcements

## Open Questions

1. **Delete with subsidiaries**: Should we allow cascade delete or require manual deletion of subsidiaries first?
   - **Recommendation**: Prevent deletion, require manual cleanup

2. **Tree view placement**: Separate tab or integrated view?
   - **Recommendation**: Phase 2 feature, separate toggle view

3. **Move operation**: Allow moving locations with subsidiaries?
   - **Recommendation**: Yes, but show confirmation with count of affected subsidiaries

4. **Tree library choice**: Which tree component library to use?
   - **Recommendation**: Evaluate in Phase 6, prefer react-accessible-treeview

5. **Identifier filter**: Dropdown or autocomplete?
   - **Recommendation**: Autocomplete with cached identifier list for better UX with many locations
