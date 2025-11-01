# Phase 4 UI - Phase 2: Complete Asset CRUD Interface

## Metadata
**Phase**: UI Phase 2 of 2
**Depends On**:
- UI Phase 1 (Shared Foundation Components) ✅
- Phase 1-4 Backend (API, Store, Hooks, Types) ✅
**Complexity**: 3/10
**Estimated Time**: 6-8 hours

## Outcome
Complete, working CRUD interface for asset management with list view, create/edit forms, delete confirmation, search/filter/sort, statistics dashboard, and responsive mobile/desktop layouts.

---

## Current State Analysis

### ✅ What's Already Complete (100% Backend)

**Store & State Management** (576 lines)
- `stores/assets/assetStore.ts` - Zustand store with multi-index cache (byId, byIdentifier, byType)
- `stores/assets/assetActions.ts` - Cache actions, UI actions, upload actions
- `stores/assets/assetPersistence.ts` - LocalStorage with TTL (1 hour)

**API Integration** (94 lines)
- `lib/api/assets/index.ts` - Full CRUD: list(), get(), create(), update(), delete(), uploadCSV(), getJobStatus()

**Types** (206 lines)
- `types/assets/index.ts` - Asset, CreateAssetRequest, UpdateAssetRequest, Filters, Pagination, Sort

**Utilities** (469 lines)
- `lib/asset/filters.ts` - filterAssets, sortAssets, searchAssets, paginateAssets
- `lib/asset/helpers.ts` - CSV validation, error extraction
- `lib/asset/transforms.ts` - Date formatting, serialization
- `lib/asset/validators.ts` - Input validation

**Tests** (1,391 lines - 47% coverage)
- Comprehensive test coverage for all backend logic

**UI Foundation** (Phase 1 - Complete)
- FloatingActionButton, Container, EmptyState, NoResults
- PaginationControls, SkeletonLoaders, ErrorBanner, ConfirmModal

**Current UI**: `AssetsScreen.tsx` (45 lines) - Placeholder showing "coming soon"

### ❌ What's Missing (0% UI)

**NO asset-specific UI components exist**. This phase creates them.

---

## What We're Building

**8 Asset-Specific Components** to complete the CRUD interface:

### Display Layer (5 components)
1. **AssetCard** - Single asset display (mobile card / desktop row)
2. **AssetTable** - Desktop table with sortable columns
3. **AssetFilters** - Filter panel (type, status, dates)
4. **AssetStats** - Statistics dashboard
5. **AssetSearchSort** - Search + sort controls

### Input Layer (2 components)
6. **AssetForm** - Create/edit form with validation
7. **AssetFormModal** - Modal wrapper for form

### Assembly Layer (1 component)
8. **AssetsScreen** - Main screen (replaces placeholder)

**Total**: ~1,290 lines across 8 files (avg 161 lines/file)

---

## Component Specifications

### 1. AssetCard (`components/assets/AssetCard.tsx`)

**Purpose**: Display a single asset (reusable in table rows and mobile cards).

**Props**:
```typescript
interface AssetCardProps {
  asset: Asset;
  onClick?: () => void;
  onEdit?: (asset: Asset) => void;
  onDelete?: (asset: Asset) => void;
  variant?: 'card' | 'row'; // card for mobile, row for table
  showActions?: boolean;
  className?: string;
}
```

**Features**:
- Displays: identifier, name, type, location (if exists), is_active status
- Status badge: Green (active) / Gray (inactive)
- Type icon: Device, Tool, Vehicle, Badge, Key (lucide-react)
- Conditional actions: edit/delete buttons
- Click to view details (onClick)
- Responsive: full card on mobile, table row on desktop

**Data Access**:
```typescript
// No API calls - receives asset as prop from parent
// Parent uses: const assets = useAssetStore(state => state.getFilteredAssets());
```

**Layout (Mobile Card)**:
```
┌───────────────────────────────────┐
│ [Device Icon] LAP-001             │
│               Engineering Laptop   │
│                                   │
│ Location: Building A - Floor 2    │
│ Status: [Active ✓]               │
│                                   │
│ [Edit] [Delete]                   │
└───────────────────────────────────┘
```

**Layout (Desktop Row)**:
```
| [Icon] | LAP-001 | Engineering Laptop | Device | Building A | [Active] | [Edit] [Delete] |
```

**Design**:
- Uses `Container` from shared foundation
- Flat design: `border border-gray-200 rounded-lg` (no shadow)
- Status colors: `bg-green-50 border-green-200` (active), `bg-gray-50 border-gray-200` (inactive)
- Hover: `hover:border-blue-500 hover:bg-blue-50`

**File Size**: ~120 lines

---

### 2. AssetTable (`components/assets/AssetTable.tsx`)

**Purpose**: Desktop table view with sortable columns.

**Props**:
```typescript
interface AssetTableProps {
  loading?: boolean;
  onAssetClick?: (asset: Asset) => void;
  onEdit?: (asset: Asset) => void;
  onDelete?: (asset: Asset) => void;
  className?: string;
}
```

**Features**:
- Desktop-only: `className="hidden md:block"`
- Reads from store: `useAssetStore(state => state.getFilteredAssets())`
- Sortable columns (clicks update store sort state)
- Loading skeleton: Uses `SkeletonTableRow` from shared
- Empty state: Uses `EmptyState` from shared
- Row click to view details

**Columns**:
1. Icon/Type
2. Identifier (sortable)
3. Name (sortable)
4. Type (sortable)
5. Location (if exists)
6. Status (sortable)
7. Actions (edit, delete)

**Data Access**:
```typescript
const assets = useAssetStore((state) => state.getFilteredAssets());
const { sortBy, sortDirection } = useAssetStore((state) => state.sort);
const setSortState = useAssetStore((state) => state.setSortState);
```

**Design**:
- Sticky header: `sticky top-0 bg-gray-50 dark:bg-gray-700 z-20`
- Striped rows: `even:bg-gray-50 dark:even:bg-gray-800/50`
- Row hover: `hover:bg-blue-50 dark:hover:bg-blue-900/20 cursor-pointer`
- Sort indicators: Arrow icons (ArrowUp, ArrowDown from lucide-react)

**File Size**: ~150 lines

---

### 3. AssetFilters (`components/assets/AssetFilters.tsx`)

**Purpose**: Collapsible filter panel.

**Props**:
```typescript
interface AssetFiltersProps {
  isOpen?: boolean;
  onToggle?: () => void;
  className?: string;
}
```

**Features**:
- Reads/updates store filters directly
- Filter by: type (checkboxes), is_active (radio), location (dropdown)
- Clear all filters button
- Active filter count badge
- Collapsible: mobile drawer, desktop sidebar

**Data Access**:
```typescript
const filters = useAssetStore((state) => state.filters);
const setFilters = useAssetStore((state) => state.setFilters);
const clearFilters = useAssetStore((state) => state.clearFilters);
```

**Filter Controls**:
1. **Type** (multi-select checkboxes)
   - Device, Tool, Vehicle, Badge, Key

2. **Status** (radio buttons)
   - All, Active, Inactive

3. **Location** (text input - searches location field)

**Layout** (Desktop):
```
┌─────────────────────────────┐
│ Filters          [Clear All]│
├─────────────────────────────┤
│ Asset Type                  │
│ ☑ Device (45)               │
│ ☐ Tool (23)                 │
│ ☐ Vehicle (12)              │
│ ☐ Badge (8)                 │
│ ☐ Key (3)                   │
├─────────────────────────────┤
│ Status                      │
│ ● Active                    │
│ ○ Inactive                  │
│ ○ All                       │
├─────────────────────────────┤
│ Location (optional)         │
│ [Search location...]        │
└─────────────────────────────┘
```

**Design**:
- Uses `Container` with gray variant
- Mobile: Full-screen drawer that slides in from right
- Desktop: Persistent 280px sidebar
- Filter badges show active filters with × to remove

**File Size**: ~180 lines

---

### 4. AssetStats (`components/assets/AssetStats.tsx`)

**Purpose**: Statistics dashboard.

**Props**:
```typescript
interface AssetStatsProps {
  className?: string;
}
```

**Features**:
- Reads from store: calculates stats from cached assets
- Total count, active count, inactive count
- Breakdown by type (Device, Tool, Vehicle, Badge, Key)
- Loading skeleton: Uses `SkeletonStatsCard` from shared

**Data Access**:
```typescript
const assets = useAssetStore((state) => state.cache.byId);
const loading = useAssetStore((state) => state.ui.loading);

// Calculate stats client-side
const total = assets.size;
const active = Array.from(assets.values()).filter(a => a.is_active).length;
const byType = {
  device: Array.from(assets.values()).filter(a => a.type === 'device').length,
  // ... etc
};
```

**Layout**:
```
┌──────────┬──────────┬──────────┐
│  Total   │  Active  │ Inactive │
│   245    │   198    │    47    │
└──────────┴──────────┴──────────┘

┌────────────────────────────────┐
│ By Type                        │
│ Devices:  120 ████████░░░ 49% │
│ Tools:     65 ████░░░░░░ 27%  │
│ Vehicles:  40 ███░░░░░░░ 16%  │
│ Badges:    15 █░░░░░░░░░  6%  │
│ Keys:       5 █░░░░░░░░░  2%  │
└────────────────────────────────┘
```

**Design**:
- Grid layout: 3 columns on desktop, 1 on mobile
- Uses `Container` for each stat card
- Progress bars for type breakdown
- Type icons from lucide-react

**File Size**: ~140 lines

---

### 5. AssetSearchSort (`components/assets/AssetSearchSort.tsx`)

**Purpose**: Search input and sort controls.

**Props**:
```typescript
interface AssetSearchSortProps {
  className?: string;
}
```

**Features**:
- Search input with debounce (300ms)
- Reads/updates store search and sort state
- Clear search button (× icon)
- Sort dropdown (name, identifier, type, is_active)
- Sort direction toggle (asc/desc)
- Results count display

**Data Access**:
```typescript
const searchTerm = useAssetStore((state) => state.filters.searchTerm);
const setSearchTerm = useAssetStore((state) => state.setSearchTerm);
const { sortBy, sortDirection } = useAssetStore((state) => state.sort);
const setSortState = useAssetStore((state) => state.setSortState);
const totalCount = useAssetStore((state) => state.pagination.totalCount);
```

**Layout**:
```
┌──────────────────────────────────────────┐
│ [🔍 Search assets...           [×] ]     │
│                                          │
│ Sort: [Name ▾]  [↑↓]        245 results │
└──────────────────────────────────────────┘
```

**Design**:
- Search: Full width on mobile, 60% on desktop
- Debounced input (useDebounce hook or setTimeout)
- Sort controls inline on desktop, stacked on mobile

**File Size**: ~100 lines

---

### 6. AssetForm (`components/assets/AssetForm.tsx`)

**Purpose**: Reusable form for creating and editing assets.

**Props**:
```typescript
interface AssetFormProps {
  mode: 'create' | 'edit';
  asset?: Asset; // Required for edit mode
  onSubmit: (data: CreateAssetRequest | UpdateAssetRequest) => Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}
```

**Features**:
- Form validation using validators from `lib/asset/validators.ts`
- Required fields: identifier, name, type
- Optional fields: location, notes, valid_from, valid_to, metadata
- Real-time validation feedback
- Submit/Cancel buttons
- Error display using `ErrorBanner` from shared

**Fields**:
```typescript
1. identifier (text input, required, unique)
2. name (text input, required)
3. type (dropdown, required: device | tool | vehicle | badge | key)
4. location (text input, optional)
5. notes (textarea, optional)
6. is_active (checkbox, default: true)
7. valid_from (date picker, optional)
8. valid_to (date picker, optional)
9. metadata (JSON text area, optional)
```

**Validation**:
- Uses `validateAssetType()` from lib/asset/validators.ts
- Uses `validateDateRange()` for valid_from/valid_to
- Identifier: Required, alphanumeric + hyphens
- Name: Required, min 2 chars
- Type: Must be one of the 5 valid types

**Design**:
- Two-column layout on desktop, single column on mobile
- Field errors shown below each input in red
- Disabled state during submission
- Uses Tailwind form classes

**File Size**: ~200 lines

---

### 7. AssetFormModal (`components/assets/AssetFormModal.tsx`)

**Purpose**: Modal wrapper for AssetForm with mutation handling.

**Props**:
```typescript
interface AssetFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  asset?: Asset; // Required for edit mode
  onClose: () => void;
}
```

**Features**:
- Lazy loaded: `const AssetFormModal = React.lazy(() => ...)`
- Calls store actions for create/update
- Success: Shows success message, closes modal, refreshes list
- Error: Shows error in form (doesn't close)
- Backdrop click to cancel

**Data Access**:
```typescript
const addAsset = useAssetStore((state) => state.addAsset);
const updateCachedAsset = useAssetStore((state) => state.updateCachedAsset);

// For API calls (uses existing API client)
import { assetsApi } from '@/lib/api/assets';
```

**Modal Flow**:
1. User clicks "Create" FAB or "Edit" button
2. Modal opens with AssetForm
3. User fills form
4. Submit → API call → Update cache → Close modal
5. List auto-updates (reactive to cache changes)

**Design**:
- Uses React Portal for modal
- Backdrop: `bg-black bg-opacity-50`
- Modal: `max-w-2xl` on desktop, full screen on mobile
- Shadow on modal: `shadow-xl` (exception to flat design - modals allowed)
- Close X button in top-right

**File Size**: ~150 lines

---

### 8. AssetsScreen (`components/AssetsScreen.tsx`)

**Purpose**: Main screen that composes all asset components.

**Features**:
- Replaces current 45-line placeholder
- Responsive layout: mobile cards, desktop table
- Floating Action Button to create assets
- Conditional rendering: loading, empty, error, data
- Tab navigation integration (already exists in app)

**Layout Structure**:
```tsx
<div className="h-full flex flex-col p-4">
  {/* Stats Dashboard */}
  <AssetStats />

  <div className="flex gap-4 mt-6">
    {/* Filters Sidebar (desktop) / Drawer (mobile) */}
    <AssetFilters />

    {/* Main Content */}
    <div className="flex-1 flex flex-col gap-4">
      {/* Search & Sort */}
      <AssetSearchSort />

      {/* Loading State */}
      {loading && <SkeletonInventoryTable />}

      {/* Empty State */}
      {!loading && assets.length === 0 && !hasActiveFilters && (
        <EmptyState
          icon={Package}
          title="No assets yet"
          description="Get started by adding your first asset"
          action={{
            label: "Create Asset",
            onClick: () => setIsCreateModalOpen(true)
          }}
        />
      )}

      {/* No Results (with filters) */}
      {!loading && assets.length === 0 && hasActiveFilters && (
        <NoResults
          searchTerm={searchTerm}
          filterCount={activeFilterCount}
          onClearFilters={clearFilters}
        />
      )}

      {/* Data Display */}
      {!loading && assets.length > 0 && (
        <>
          {/* Desktop Table */}
          <AssetTable
            onAssetClick={handleViewAsset}
            onEdit={handleEditAsset}
            onDelete={handleDeleteAsset}
          />

          {/* Mobile Cards */}
          <div className="md:hidden space-y-3">
            {assets.map(asset => (
              <AssetCard
                key={asset.id}
                asset={asset}
                variant="card"
                onClick={() => handleViewAsset(asset)}
                onEdit={handleEditAsset}
                onDelete={handleDeleteAsset}
                showActions
              />
            ))}
          </div>

          {/* Pagination */}
          <PaginationControls
            currentPage={currentPage}
            totalPages={totalPages}
            totalItems={totalItems}
            pageSize={pageSize}
            startIndex={startIndex}
            endIndex={endIndex}
            onPageChange={setCurrentPage}
            onPrevious={handlePrevious}
            onNext={handleNext}
            onFirstPage={handleFirstPage}
            onLastPage={handleLastPage}
            onPageSizeChange={setPageSize}
          />
        </>
      )}
    </div>
  </div>

  {/* Floating Action Button */}
  <FloatingActionButton
    icon={Plus}
    onClick={() => setIsCreateModalOpen(true)}
    ariaLabel="Create new asset"
    variant="primary"
  />

  {/* Modals (lazy loaded) */}
  <Suspense fallback={null}>
    {isCreateModalOpen && (
      <AssetFormModal
        isOpen={isCreateModalOpen}
        mode="create"
        onClose={() => setIsCreateModalOpen(false)}
      />
    )}

    {editingAsset && (
      <AssetFormModal
        isOpen={!!editingAsset}
        mode="edit"
        asset={editingAsset}
        onClose={() => setEditingAsset(null)}
      />
    )}

    {deletingAsset && (
      <ConfirmModal
        isOpen={!!deletingAsset}
        title="Delete Asset"
        message={`Are you sure you want to delete ${deletingAsset.name}? This action cannot be undone.`}
        onConfirm={confirmDelete}
        onCancel={() => setDeletingAsset(null)}
      />
    )}
  </Suspense>
</div>
```

**State Management**:
```typescript
// All data from store (reactive)
const assets = useAssetStore((state) => state.getFilteredAssets());
const loading = useAssetStore((state) => state.ui.loading);
const searchTerm = useAssetStore((state) => state.filters.searchTerm);
const { currentPage, pageSize, totalCount } = useAssetStore((state) => state.pagination);

// Local UI state
const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
const [editingAsset, setEditingAsset] = useState<Asset | null>(null);
const [deletingAsset, setDeletingAsset] = useState<Asset | null>(null);
```

**Data Flow**:
1. **On Mount**: Check if cache is empty/stale → fetch from API → update cache
2. **User Creates**: Modal → API → Cache update → List auto-refreshes
3. **User Edits**: Modal → API → Cache update → List auto-refreshes
4. **User Deletes**: Confirm → API → Cache remove → List auto-refreshes
5. **User Filters**: Update store filters → getFilteredAssets() recomputes → UI updates

**File Size**: ~200 lines

---

## File Structure

```
frontend/src/
├── components/
│   ├── assets/
│   │   ├── AssetCard.tsx              (~120 lines)
│   │   ├── AssetTable.tsx             (~150 lines)
│   │   ├── AssetFilters.tsx           (~180 lines)
│   │   ├── AssetStats.tsx             (~140 lines)
│   │   ├── AssetSearchSort.tsx        (~100 lines)
│   │   ├── AssetForm.tsx              (~200 lines)
│   │   ├── AssetFormModal.tsx         (~150 lines)
│   │   ├── index.ts                   (barrel export)
│   │   └── __tests__/
│   │       ├── AssetCard.test.tsx
│   │       ├── AssetTable.test.tsx
│   │       ├── AssetFilters.test.tsx
│   │       ├── AssetStats.test.tsx
│   │       ├── AssetSearchSort.test.tsx
│   │       ├── AssetForm.test.tsx
│   │       └── AssetFormModal.test.tsx
│   └── AssetsScreen.tsx               (~200 lines) [REPLACE PLACEHOLDER]
└── (All backend already exists - no new files needed)
```

**Total New/Modified Files**: 9 files (~1,240 lines of components + ~700 lines of tests)

---

## Design System Compliance

### Flat Design Pattern ✅
- All components: borders + rounded corners
- NO shadows (except modals - allowed per spec)
- Color pattern: `bg-{color}-50 dark:bg-{color}-900/20 border-{color}-200 dark:border-{color}-800`

### Status Colors
```typescript
const STATUS_STYLES = {
  active: 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-200',
  inactive: 'bg-gray-50 dark:bg-gray-900/20 border-gray-200 dark:border-gray-800 text-gray-800 dark:text-gray-200',
} as const;
```

### Type Colors & Icons
```typescript
import { Laptop, Wrench, Car, IdCard, Key } from 'lucide-react';

const TYPE_CONFIG = {
  device: { icon: Laptop, color: 'text-blue-600 dark:text-blue-400' },
  tool: { icon: Wrench, color: 'text-orange-600 dark:text-orange-400' },
  vehicle: { icon: Car, color: 'text-purple-600 dark:text-purple-400' },
  badge: { icon: IdCard, color: 'text-green-600 dark:text-green-400' },
  key: { icon: Key, color: 'text-red-600 dark:text-red-400' },
} as const;
```

### Icons (from lucide-react)
- Search, Filter, ArrowUpDown, Plus, Pencil, Trash2, X, Package

---

## Testing Requirements

### Unit Tests (7 files)

**AssetCard.test.tsx** (6 tests)
- Renders asset data correctly
- Shows/hides action buttons based on props
- Calls onClick/onEdit/onDelete handlers
- Displays correct status badge color
- Shows type icon
- Applies variant styling (card vs row)

**AssetTable.test.tsx** (8 tests)
- Renders list of assets in table
- Shows loading skeleton when loading
- Shows empty state when no assets
- Handles column sorting (updates store)
- Calls onAssetClick when row clicked
- Renders action buttons
- Hidden on mobile (has `md:block` class)
- Displays correct number of rows

**AssetFilters.test.tsx** (7 tests)
- Renders all filter controls
- Updates store when filters change
- Clears all filters
- Shows active filter count
- Toggles open/closed state
- Type checkboxes update filters
- Status radio updates filters

**AssetStats.test.tsx** (5 tests)
- Displays total/active/inactive counts
- Shows loading skeleton when loading
- Calculates type breakdown correctly
- Renders progress bars
- Responsive grid layout

**AssetSearchSort.test.tsx** (6 tests)
- Updates search term in store
- Debounces search input (300ms)
- Clears search when × clicked
- Updates sort in store
- Toggles sort direction
- Displays results count

**AssetForm.test.tsx** (10 tests)
- Renders all form fields
- Validates required fields
- Shows field errors
- Calls onSubmit with correct data
- Disables form during submission
- Shows error banner on error
- Populates fields in edit mode
- Validates date range
- Validates asset type
- Clears form after successful create

**AssetFormModal.test.tsx** (8 tests)
- Opens/closes modal
- Calls API on submit (create)
- Updates cache after create
- Calls API on submit (edit)
- Updates cache after edit
- Shows error in form on API error
- Closes on backdrop click
- Closes on cancel

### Integration Tests (1 file)

**assets-components.integration.test.tsx** (6 tests)
- All components import from barrel export
- AssetCard renders within AssetTable
- Filter updates affect displayed assets
- Search filters asset list
- Sort changes order
- Create/edit/delete flow works end-to-end

**Total Tests**: 56 tests

---

## Success Criteria

**Functionality**
- [ ] List all assets with pagination
- [ ] Search assets by identifier/name
- [ ] Filter by type, status, location
- [ ] Sort by any column
- [ ] Create new assets via form
- [ ] Edit existing assets via form
- [ ] Delete assets with confirmation
- [ ] View statistics dashboard
- [ ] All data reads from/writes to Zustand store
- [ ] Cache updates automatically trigger UI refresh

**Quality**
- [ ] All components under 200 lines
- [ ] Flat design pattern (borders, no shadows except modals)
- [ ] Dark mode support
- [ ] Mobile-first responsive
- [ ] TypeScript: 0 errors
- [ ] Lint: 0 new errors
- [ ] Tests: 56+ tests, 100% pass rate
- [ ] Loading states with skeletons
- [ ] Empty states with actions
- [ ] Error states with clear messages

**Performance**
- [ ] Lazy-loaded modals
- [ ] Debounced search (300ms)
- [ ] Zustand selectors prevent unnecessary re-renders
- [ ] Pagination limits rendered items

---

## Implementation Checklist

### Setup
- [ ] Verify UI Phase 1 (Shared Foundation) is merged
- [ ] Create new branch from `feature/assets-frontend-logic`
- [ ] Branch name: `feature/assets-ui-components`
- [ ] Create `frontend/src/components/assets/` directory
- [ ] Create barrel export `frontend/src/components/assets/index.ts`

### Component Implementation
- [ ] AssetCard.tsx (~120 lines)
- [ ] AssetTable.tsx (~150 lines)
- [ ] AssetFilters.tsx (~180 lines)
- [ ] AssetStats.tsx (~140 lines)
- [ ] AssetSearchSort.tsx (~100 lines)
- [ ] AssetForm.tsx (~200 lines)
- [ ] AssetFormModal.tsx (~150 lines)
- [ ] Replace AssetsScreen.tsx (~200 lines)

### Testing
- [ ] AssetCard.test.tsx (6 tests)
- [ ] AssetTable.test.tsx (8 tests)
- [ ] AssetFilters.test.tsx (7 tests)
- [ ] AssetStats.test.tsx (5 tests)
- [ ] AssetSearchSort.test.tsx (6 tests)
- [ ] AssetForm.test.tsx (10 tests)
- [ ] AssetFormModal.test.tsx (8 tests)
- [ ] Integration test (6 tests)

### Validation
- [ ] Run lint (0 new errors)
- [ ] Run typecheck (0 errors)
- [ ] Run tests (56+ tests, 100% pass)
- [ ] Verify all files under 200 lines
- [ ] Verify flat design compliance
- [ ] Test mobile responsive behavior
- [ ] Test dark mode
- [ ] Test create flow end-to-end
- [ ] Test edit flow end-to-end
- [ ] Test delete flow end-to-end

### PR & Merge
- [ ] Commit with conventional commit message
- [ ] Push to `feature/assets-ui-components`
- [ ] Create PR targeting `feature/assets-frontend-logic` (NOT main)
- [ ] PR passes all checks
- [ ] Merge to `feature/assets-frontend-logic`

---

## Branching Strategy

```
feature/assets-frontend-logic (base for all asset work)
├── feature/assets-ui-shared-foundation (Phase 1)
│   └── PR #44 → merge to feature/assets-frontend-logic ✅
└── feature/assets-ui-components (Phase 2)
    └── New PR → merge to feature/assets-frontend-logic ⏭️
```

**CRITICAL**:
- PR #44 is currently targeting `main` - should target `feature/assets-frontend-logic`
- Phase 2 PR MUST target `feature/assets-frontend-logic` (NOT main)
- Final PR merges `feature/assets-frontend-logic` → `main` after all phases complete

---

## Dependencies

### From Phase 1 (Shared Foundation)
```typescript
import {
  FloatingActionButton,
  Container,
  EmptyState,
  NoResults,
  PaginationControls,
  SkeletonTableRow,
  SkeletonInventoryTable,
  SkeletonStatsCard,
  ErrorBanner,
  ConfirmModal,
} from '@/components/shared';
```

### From Backend (Already Complete)
```typescript
// Store
import { useAssetStore } from '@/stores/assets/assetStore';

// Types
import type { Asset, CreateAssetRequest, UpdateAssetRequest } from '@/types/assets';

// API (for direct API calls in modals)
import { assetsApi } from '@/lib/api/assets';

// Validators
import { validateAssetType, validateDateRange } from '@/lib/asset/validators';

// Transforms
import { formatDateForInput } from '@/lib/asset/transforms';
```

### Icons
```typescript
import {
  Laptop,
  Wrench,
  Car,
  IdCard,
  Key,
  Search,
  Filter,
  ArrowUpDown,
  Plus,
  Pencil,
  Trash2,
  X,
  Package,
} from 'lucide-react';
```

---

## Notes

**Why Complexity is Only 3/10:**
1. All backend logic exists (API, store, cache, filters, validators)
2. Components just read from store (reactive)
3. Forms submit to API → API updates cache → UI auto-refreshes
4. Shared foundation handles all common UI (loaders, pagination, empty states)
5. No complex state management - store handles everything
6. No complex algorithms - just UI composition

**Why This Completes CRUD:**
- **CREATE**: AssetForm + AssetFormModal + assetsApi.create()
- **READ**: AssetTable + AssetCard + useAssetStore.getFilteredAssets()
- **UPDATE**: AssetForm (edit mode) + AssetFormModal + assetsApi.update()
- **DELETE**: ConfirmModal + assetsApi.delete()

**Why It's Fast to Implement:**
- Store already handles cache updates
- API client already exists
- Validators already exist
- Filters/search/sort already exist
- Just wiring existing pieces together

---

## References

- Shared Foundation: `frontend/src/components/shared/`
- Asset Store: `frontend/src/stores/assets/assetStore.ts`
- Asset Types: `frontend/src/types/assets/index.ts`
- Asset API: `frontend/src/lib/api/assets/index.ts`
- Asset Utilities: `frontend/src/lib/asset/`
- Phase 1 Plan: `spec/active/assets-frontend-logic/phase-4-phase1-plan.md`
