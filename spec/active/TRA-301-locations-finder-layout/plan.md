# TRA-301 Phase 1: Core Components + Desktop Split Pane

## Implementation Plan

**Scope**: Core split-pane components, tree panel, details panel, store additions, desktop integration
**Phase**: 1 of 3
**Estimated Complexity**: 5/10 (manageable)

---

## Decisions Made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Resizable divider | `react-split-pane` library | Well-tested, handles edge cases |
| localStorage prefix | `locations_` | e.g., `locations_treePanelWidth` |
| Edit/Move/Delete | Keep modals | Reuse existing components, less disruption |

---

## Implementation Order

### Step 1: Add Dependencies

**File**: `frontend/package.json`

```bash
pnpm add react-split-pane
pnpm add -D @types/react-split-pane
```

---

### Step 2: Extend locationStore with UI State

**File**: `frontend/src/stores/locations/locationStore.ts`

**Add to interface** (after existing `selectedLocationId`):
```typescript
// UI State for split pane
expandedNodeIds: Set<number>;
treePanelWidth: number;
```

**Add to actions**:
```typescript
toggleNodeExpanded: (id: number) => void;
setTreePanelWidth: (width: number) => void;
expandToLocation: (id: number) => void;
```

**Implementation**:
- `expandedNodeIds` defaults to `new Set()`
- `treePanelWidth` defaults to `280` (px), persists to `locations_treePanelWidth`
- `toggleNodeExpanded` adds/removes from Set, persists to `locations_expandedNodes`
- `expandToLocation` uses existing `getAncestors()` to expand all parents

**Persistence**: Load from localStorage in store initialization, save on change.

---

### Step 3: Create LocationTreePanel Component

**File**: `frontend/src/components/locations/LocationTreePanel.tsx` (~150 lines)

**Purpose**: Left panel tree navigation (navigation only, no edit/delete buttons)

**Props**:
```typescript
interface LocationTreePanelProps {
  onSelect: (locationId: number) => void;
  selectedId: number | null;
  searchTerm?: string;
}
```

**Features**:
- Renders root locations from `getRootLocations()`
- Recursive tree nodes with chevron icons (right=collapsed, down=expanded)
- Building2 icon for root locations, MapPin for children
- Click chevron → toggle expand (does NOT select)
- Click node text → select (does NOT toggle expand)
- Blue background highlight on selected node
- Keyboard navigation: ArrowUp/Down (move selection), ArrowLeft/Right (collapse/expand), Enter (select)
- Filters by search term (shows matching + ancestors)

**Extract from**: `LocationTreeView.tsx` (reuse rendering logic, remove edit buttons)

---

### Step 4: Create LocationDetailsPanel Component

**File**: `frontend/src/components/locations/LocationDetailsPanel.tsx` (~200 lines)

**Purpose**: Right panel showing selected location details

**Props**:
```typescript
interface LocationDetailsPanelProps {
  locationId: number | null;
  onEdit: (id: number) => void;
  onMove: (id: number) => void;
  onDelete: (id: number) => void;
}
```

**Sections**:
1. **Empty state** (no selection): "Select a location" message + stats summary
2. **Header**: Location name + status badge (Active/Inactive)
3. **Info grid**: Identifier, Name, Description, Path breadcrumb
4. **Hierarchy info**: Children count, descendants count
5. **Children list**: Clickable links to navigate to children
6. **Tag identifiers**: Display/manage tag identifiers
7. **Action buttons**: Edit, Move, Delete (styled per existing patterns)

**Extract from**: `LocationDetailsModal.tsx` (reuse content sections, remove modal wrapper)

**Button styling** (from existing patterns):
```tsx
// Edit - blue
className="px-4 py-2 text-sm font-medium text-blue-700 bg-blue-50 hover:bg-blue-100
           dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40
           border border-blue-200 dark:border-blue-800 rounded-lg"

// Move - purple
className="px-4 py-2 text-sm font-medium text-purple-700 bg-purple-50 hover:bg-purple-100
           dark:text-purple-400 dark:bg-purple-900/20 dark:hover:bg-purple-900/40
           border border-purple-200 dark:border-purple-800 rounded-lg"

// Delete - red
className="px-4 py-2 text-sm font-medium text-red-700 bg-red-50 hover:bg-red-100
           dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40
           border border-red-200 dark:border-blue-800 rounded-lg"
```

---

### Step 5: Create LocationSplitPane Component

**File**: `frontend/src/components/locations/LocationSplitPane.tsx` (~100 lines)

**Purpose**: Desktop split layout container using react-split-pane

**Props**:
```typescript
interface LocationSplitPaneProps {
  searchTerm?: string;
  onEdit: (id: number) => void;
  onMove: (id: number) => void;
  onDelete: (id: number) => void;
}
```

**Implementation**:
```tsx
import SplitPane from 'react-split-pane';

<SplitPane
  split="vertical"
  minSize={200}
  maxSize={400}
  defaultSize={store.treePanelWidth}
  onChange={(size) => store.setTreePanelWidth(size)}
  resizerStyle={{ /* divider styling */ }}
>
  <LocationTreePanel
    onSelect={store.setSelectedLocation}
    selectedId={store.selectedLocationId}
    searchTerm={searchTerm}
  />
  <LocationDetailsPanel
    locationId={store.selectedLocationId}
    onEdit={onEdit}
    onMove={onMove}
    onDelete={onDelete}
  />
</SplitPane>
```

**Constraints**:
- Min width: 200px
- Max width: 400px
- Default: 280px (or from localStorage)

---

### Step 6: Update LocationsScreen for Desktop

**File**: `frontend/src/components/LocationsScreen.tsx`

**Changes**:
1. Import `LocationSplitPane`
2. Add responsive breakpoint detection (`useMediaQuery` or window width check)
3. On desktop (≥1024px): Render `LocationSplitPane` instead of List/Tree toggle
4. On mobile (<1024px): Keep existing behavior (Phase 2 will add expandable cards)
5. Remove List as default - Tree is now default for non-desktop

**Conditional rendering**:
```tsx
const isDesktop = useMediaQuery('(min-width: 1024px)');

return (
  <div>
    {/* Header with search, sort, add button */}
    {isDesktop ? (
      <LocationSplitPane
        searchTerm={searchTerm}
        onEdit={handleEdit}
        onMove={handleMove}
        onDelete={handleDelete}
      />
    ) : (
      // Existing tree/list view for now (Phase 2 will replace with expandable cards)
      <LocationTreeView ... />
    )}
    {/* Footer stats */}
  </div>
);
```

---

### Step 7: Export New Components

**File**: `frontend/src/components/locations/index.ts`

Add exports:
```typescript
export { LocationTreePanel } from './LocationTreePanel';
export { LocationDetailsPanel } from './LocationDetailsPanel';
export { LocationSplitPane } from './LocationSplitPane';
```

---

## Unit Tests (Vitest + React Testing Library)

### Step 8: LocationTreePanel.test.tsx

**File**: `frontend/src/components/locations/LocationTreePanel.test.tsx`

Tests:
- [ ] Renders root locations at top level
- [ ] Renders children indented under parents
- [ ] Shows Building2 icon for root, MapPin for children
- [ ] Shows chevron-right for collapsed, chevron-down for expanded
- [ ] Toggles expansion on chevron click
- [ ] Does NOT toggle expansion on node text click (selects instead)
- [ ] Highlights selected location with blue background
- [ ] Calls onSelect when location clicked
- [ ] Has no edit/delete buttons (navigation only)
- [ ] Keyboard: ArrowDown moves selection down
- [ ] Keyboard: ArrowUp moves selection up
- [ ] Keyboard: ArrowRight expands node
- [ ] Keyboard: ArrowLeft collapses node
- [ ] Keyboard: Enter selects node
- [ ] Filters locations by search term
- [ ] Shows matching locations and ancestors

---

### Step 9: LocationDetailsPanel.test.tsx

**File**: `frontend/src/components/locations/LocationDetailsPanel.test.tsx`

Tests:
- [ ] Shows "Select a location" when none selected
- [ ] Shows stats summary in empty state
- [ ] Shows location identifier, name, description
- [ ] Shows active/inactive status badge
- [ ] Shows breadcrumb path
- [ ] Shows hierarchy info (children count, descendants)
- [ ] Lists direct children with click navigation
- [ ] Shows Edit, Move, Delete buttons
- [ ] Calls onEdit when Edit clicked
- [ ] Calls onMove when Move clicked
- [ ] Calls onDelete when Delete clicked
- [ ] Shows and manages tag identifiers

---

### Step 10: LocationSplitPane.test.tsx

**File**: `frontend/src/components/locations/LocationSplitPane.test.tsx`

Tests:
- [ ] Renders tree panel on left
- [ ] Renders details panel on right
- [ ] Shows resizable divider
- [ ] Respects minimum panel width (200px)
- [ ] Respects maximum panel width (400px)

---

### Step 11: locationStore.test.ts (UI state additions)

**File**: `frontend/src/stores/locations/locationStore.test.ts` (add to existing)

Tests:
- [ ] Sets selected location
- [ ] Clears selection
- [ ] Toggles node expansion
- [ ] Expands all ancestors when expandToLocation called
- [ ] Persists expanded nodes to localStorage
- [ ] Restores expanded nodes from localStorage
- [ ] Sets tree panel width
- [ ] Persists tree panel width to localStorage

---

## E2E Tests (Playwright)

### Step 12: tests/e2e/locations-desktop.spec.ts

**File**: `frontend/tests/e2e/locations-desktop.spec.ts`

**Setup**: Create test hierarchy via API:
```
Warehouse A (root)
├── Floor 1
│   ├── Section A
│   └── Section B
└── Floor 2
    └── Section C
Warehouse B (root)
└── Storage Area
```

Tests (viewport: 1280x800):
- [ ] Shows tree panel on left side
- [ ] Shows details panel on right side
- [ ] Shows resizable divider between panels
- [ ] Resizes panels when dragging divider
- [ ] Persists panel width across page reloads
- [ ] Shows all root locations in tree
- [ ] Expands location on chevron click
- [ ] Collapses expanded location on chevron click
- [ ] Highlights selected location
- [ ] Updates details panel when location selected
- [ ] Supports keyboard navigation (arrow keys)
- [ ] Filters tree when searching
- [ ] Shows empty state when no location selected
- [ ] Shows location details when selected
- [ ] Shows breadcrumb path in details
- [ ] Shows children list in details
- [ ] Navigates to child when clicked in details
- [ ] Opens edit modal when Edit clicked
- [ ] Opens move modal when Move clicked
- [ ] Opens delete confirmation when Delete clicked
- [ ] Creates new location and shows in tree
- [ ] Updates location and reflects in tree/details
- [ ] Deletes location and removes from tree
- [ ] Moves location and updates hierarchy

---

## Validation Checklist

Before marking Phase 1 complete:

```bash
# Run all checks
just frontend validate

# Specific checks
just frontend lint
just frontend typecheck
just frontend test
just frontend test:e2e tests/e2e/locations-desktop.spec.ts
```

- [ ] All existing location tests pass
- [ ] All new unit tests pass (20+ tests)
- [ ] All new E2E tests pass (25+ tests)
- [ ] Desktop split-pane renders at 1280x800
- [ ] Dark mode works for all new components
- [ ] No console errors during E2E tests
- [ ] Panel resize persists across reloads

---

## Files Summary

### New Files (7)
| File | Lines | Purpose |
|------|-------|---------|
| `src/components/locations/LocationTreePanel.tsx` | ~150 | Tree navigation panel |
| `src/components/locations/LocationDetailsPanel.tsx` | ~200 | Details display panel |
| `src/components/locations/LocationSplitPane.tsx` | ~100 | Split pane container |
| `src/components/locations/LocationTreePanel.test.tsx` | ~200 | Unit tests |
| `src/components/locations/LocationDetailsPanel.test.tsx` | ~180 | Unit tests |
| `src/components/locations/LocationSplitPane.test.tsx` | ~80 | Unit tests |
| `tests/e2e/locations-desktop.spec.ts` | ~300 | E2E tests |

### Modified Files (4)
| File | Changes |
|------|---------|
| `package.json` | Add react-split-pane dependency |
| `src/stores/locations/locationStore.ts` | Add UI state + actions |
| `src/stores/locations/locationStore.test.ts` | Add UI state tests |
| `src/components/LocationsScreen.tsx` | Responsive layout |
| `src/components/locations/index.ts` | Export new components |

---

## Out of Scope (Phase 2+)

- Mobile expandable cards (Phase 2)
- Mobile E2E tests (Phase 2)
- Accessibility audit (Phase 3)
- Performance optimization (Phase 3)
