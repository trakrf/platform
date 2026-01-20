# TRA-301: Locations Tab Mac Finder-Style Layout

## Overview

Redesign the Locations tab to use a Mac Finder-style split-pane layout on desktop and expandable cards on mobile, replacing the current toggle-based List/Tree view system.

**Linear Ticket**: [TRA-301](https://linear.app/trakrf/issue/TRA-301/featlocations-redesign-locations-tab-with-mac-finder-style-split-pane)

**Branch**: `feature/TRA-301-locations-finder-layout`

---

## Current State Analysis

### Existing Components
| Component | Path | Purpose |
|-----------|------|---------|
| `LocationsScreen.tsx` | `src/components/LocationsScreen.tsx` | Main screen with List/Tree toggle |
| `LocationTreeView.tsx` | `src/components/locations/LocationTreeView.tsx` | Recursive tree renderer |
| `LocationTable.tsx` | `src/components/locations/LocationTable.tsx` | Paginated table view |
| `LocationDetailsModal.tsx` | `src/components/locations/LocationDetailsModal.tsx` | Modal for viewing details |
| `LocationCard.tsx` | `src/components/locations/LocationCard.tsx` | Mobile card component |
| `LocationBreadcrumb.tsx` | `src/components/locations/LocationBreadcrumb.tsx` | Breadcrumb navigation |
| `LocationStats.tsx` | `src/components/locations/LocationStats.tsx` | Statistics display |

### Current Behavior
- Default view: **List** (table format)
- Toggle required to switch to **Tree** view
- Details shown in **modal** overlay
- Single-pane layout for both views
- Mobile: Card-based list with same toggle

### Pain Points
1. Tree view hidden by default - users must click to see hierarchy
2. Modal interrupts workflow when viewing details
3. No persistent navigation when drilling down
4. Context lost when switching between items

---

## Proposed Design

### Desktop Layout (≥1024px)

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Locations                                              [Disconnected] T │
├────────────────────┬─────────────────────────────────────────────────────┤
│                    │  🔍 Search...          Sort: [Identifier ▼]   [+]   │
│   TREE PANEL       ├─────────────────────────────────────────────────────┤
│   (250-300px)      │                                                     │
│                    │   DETAILS PANEL                                     │
│ ▼ 🏢 Warehouse A   │                                                     │
│   ├─ 📍 Floor 1    │   Location: Floor 1                                 │
│   │  ├─ Section A  │   ─────────────────────────────────────────         │
│   │  └─ Section B  │                                                     │
│   └─ 📍 Floor 2    │   Identifier: floor-1                               │
│ ▶ 🏢 Warehouse B   │   Name: Floor 1                                     │
│                    │   Status: ● Active                                  │
│                    │   Parent: Warehouse A                               │
│                    │   Path: warehouse-a > floor-1                       │
│                    │                                                     │
│                    │   Description: Main production floor                │
│                    │                                                     │
│                    │   ──── Hierarchy ────                               │
│                    │   Direct Children: 2                                │
│                    │   Total Descendants: 2                              │
│                    │                                                     │
│                    │   ├─ 📍 Section A                                   │
│                    │   └─ 📍 Section B                                   │
│                    │                                                     │
│                    │   [Edit]  [Move]  [Delete]                          │
│                    │                                                     │
├────────────────────┴─────────────────────────────────────────────────────┤
│  Total: 6  │  Active: 6  │  Inactive: 0  │  Root: 2                      │
└──────────────────────────────────────────────────────────────────────────┘
```

### Mobile Layout (<768px) - Expandable Cards

```
┌─────────────────────────────┐
│ ☰  Locations         [+]    │
├─────────────────────────────┤
│ 🔍 Search...                │
├─────────────────────────────┤
│                             │
│ ▼ 🏢 Warehouse A   ● Active │  ← Tap header to expand
│ ┌─────────────────────────┐ │
│ │ Path: warehouse-a       │ │
│ │ Children: 2             │ │
│ │ [Edit] [Move] [Delete]  │ │
│ │                         │ │
│ │ ▼ 📍 Floor 1   ● Active │ │  ← Nested expandable
│ │ ┌───────────────────┐   │ │
│ │ │ Path: ...floor-1  │   │ │
│ │ │ Children: 2       │   │ │
│ │ │ [Edit] [Move] [Del│   │ │
│ │ │                   │   │ │
│ │ │ ▶ 📍 Section A    │   │ │
│ │ │ ▶ 📍 Section B    │   │ │
│ │ └───────────────────┘   │ │
│ │ ▶ 📍 Floor 2            │ │
│ └─────────────────────────┘ │
│                             │
│ ▶ 🏢 Warehouse B   ● Active │  ← Collapsed
│                             │
├─────────────────────────────┤
│ Total: 6 │ Active: 6 │ ...  │
└─────────────────────────────┘
```

### Tablet Layout (768-1023px)
- Narrower split-pane OR expandable cards
- Tree panel collapsible via toggle button
- Touch-optimized click targets (min 44px)

---

## Component Architecture

### New Components

```
src/components/locations/
├── LocationsScreen.tsx           # Updated - orchestrates layout
├── LocationSplitPane.tsx         # NEW - desktop split layout
├── LocationTreePanel.tsx         # NEW - left panel tree nav
├── LocationDetailsPanel.tsx      # NEW - right panel details
├── LocationExpandableCard.tsx    # NEW - mobile expandable card
├── LocationExpandableTree.tsx    # NEW - mobile nested cards
└── ... (existing components)
```

### Component Responsibilities

#### `LocationSplitPane.tsx`
- Wraps tree and details panels
- Handles resizable divider
- Manages panel widths (min/max constraints)
- Responsive breakpoint handling

#### `LocationTreePanel.tsx`
- Navigation-only tree (no inline actions)
- Expand/collapse state management
- Selected item highlighting
- Keyboard navigation (arrow keys)
- Search filtering

#### `LocationDetailsPanel.tsx`
- Displays selected location details
- Inline actions (Edit, Move, Delete)
- Shows children list
- Empty state when none selected

#### `LocationExpandableCard.tsx`
- Single location card with expand/collapse
- Shows summary when collapsed
- Shows full details + children when expanded
- Nested cards for children

---

## State Management

### New Store State (locationStore.ts)

```typescript
interface LocationUIState {
  // Selection
  selectedLocationId: number | null;

  // Tree expansion (desktop)
  expandedNodeIds: Set<number>;

  // Card expansion (mobile)
  expandedCardIds: Set<number>;

  // Panel width (desktop)
  treePanelWidth: number;
}

interface LocationUIActions {
  setSelectedLocation: (id: number | null) => void;
  toggleNodeExpanded: (id: number) => void;
  toggleCardExpanded: (id: number) => void;
  setTreePanelWidth: (width: number) => void;
  expandToLocation: (id: number) => void; // Expands all ancestors
}
```

### Persistence
- `expandedNodeIds` → localStorage (desktop)
- `expandedCardIds` → localStorage (mobile)
- `treePanelWidth` → localStorage
- `selectedLocationId` → session only (not persisted)

---

## Styling Guidelines

### Color Palette (from tailwind.config.js)

| Element | Light Mode | Dark Mode |
|---------|------------|-----------|
| Tree panel bg | `bg-white` | `bg-gray-900` |
| Details panel bg | `bg-gray-50` | `bg-gray-800` |
| Selected item | `bg-blue-100` | `bg-blue-900/40` |
| Hover item | `bg-blue-50` | `bg-blue-900/20` |
| Active status | `text-green-700 bg-green-50` | `text-green-400 bg-green-900/20` |
| Inactive status | `text-gray-700 bg-gray-50` | `text-gray-400 bg-gray-900/20` |
| Divider | `border-gray-200` | `border-gray-700` |
| Icons (root) | Building2 `text-gray-500` | `text-gray-400` |
| Icons (child) | MapPin `text-gray-500` | `text-gray-400` |

### Button Styles (from LocationDetailsModal.tsx)

```tsx
// Edit button
className="px-4 py-2 text-sm font-medium text-blue-700 bg-blue-50 hover:bg-blue-100
           dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40
           border border-blue-200 dark:border-blue-800 rounded-lg"

// Move button
className="px-4 py-2 text-sm font-medium text-purple-700 bg-purple-50 hover:bg-purple-100
           dark:text-purple-400 dark:bg-purple-900/20 dark:hover:bg-purple-900/40
           border border-purple-200 dark:border-purple-800 rounded-lg"

// Delete button
className="px-4 py-2 text-sm font-medium text-red-700 bg-red-50 hover:bg-red-100
           dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40
           border border-red-200 dark:border-red-800 rounded-lg"
```

### Responsive Breakpoints

| Breakpoint | Width | Layout |
|------------|-------|--------|
| Mobile | <768px | Expandable cards |
| Tablet | 768-1023px | Narrow split or cards |
| Desktop | ≥1024px | Full split-pane |

---

## Test Requirements

### Unit Tests (Vitest + React Testing Library)

#### LocationTreePanel.test.tsx
```typescript
describe('LocationTreePanel', () => {
  // Rendering
  it('should render root locations at top level');
  it('should render children indented under parents');
  it('should show Building2 icon for root locations');
  it('should show MapPin icon for child locations');

  // Expansion
  it('should show chevron-right for collapsed nodes with children');
  it('should show chevron-down for expanded nodes');
  it('should hide chevron for leaf nodes');
  it('should toggle expansion on chevron click');
  it('should NOT toggle expansion on node text click');

  // Selection
  it('should highlight selected location with blue background');
  it('should call onSelect when location clicked');
  it('should not have edit/delete buttons (navigation only)');

  // Keyboard Navigation
  it('should move selection down with ArrowDown');
  it('should move selection up with ArrowUp');
  it('should expand node with ArrowRight');
  it('should collapse node with ArrowLeft');
  it('should select node with Enter');

  // Filtering
  it('should filter locations by search term');
  it('should show matching locations and their ancestors');
  it('should highlight matching text');
});
```

#### LocationDetailsPanel.test.tsx
```typescript
describe('LocationDetailsPanel', () => {
  // Empty state
  it('should show "Select a location" when none selected');
  it('should show stats summary in empty state');

  // Details display
  it('should show location identifier');
  it('should show location name');
  it('should show location description');
  it('should show active/inactive status badge');
  it('should show breadcrumb path');
  it('should show hierarchy info (children count, descendants)');
  it('should list direct children with click navigation');

  // Actions
  it('should show Edit button');
  it('should show Move button');
  it('should show Delete button');
  it('should call onEdit when Edit clicked');
  it('should call onMove when Move clicked');
  it('should call onDelete when Delete clicked');

  // Tag identifiers
  it('should show tag identifiers list');
  it('should allow removing tag identifiers');
});
```

#### LocationExpandableCard.test.tsx
```typescript
describe('LocationExpandableCard', () => {
  // Collapsed state
  it('should show identifier and name when collapsed');
  it('should show status badge when collapsed');
  it('should show expand chevron');

  // Expanded state
  it('should show full details when expanded');
  it('should show action buttons when expanded');
  it('should show children cards when expanded');

  // Interaction
  it('should toggle expanded state on header click');
  it('should NOT collapse when clicking action buttons');
  it('should nest children with proper indentation');
});
```

#### LocationSplitPane.test.tsx
```typescript
describe('LocationSplitPane', () => {
  // Layout
  it('should render tree panel on left');
  it('should render details panel on right');
  it('should show resizable divider between panels');

  // Resize
  it('should resize panels on divider drag');
  it('should respect minimum panel width (200px)');
  it('should respect maximum panel width (400px)');
  it('should persist panel width to localStorage');

  // Responsive
  it('should hide on mobile viewport (<768px)');
  it('should show on desktop viewport (≥1024px)');
});
```

#### locationStore.test.ts (UI state additions)
```typescript
describe('locationStore - UI State', () => {
  // Selection
  it('should set selected location');
  it('should clear selection');

  // Tree expansion
  it('should toggle node expansion');
  it('should expand all ancestors when expandToLocation called');
  it('should persist expanded nodes to localStorage');
  it('should restore expanded nodes from localStorage');

  // Card expansion
  it('should toggle card expansion');
  it('should persist expanded cards to localStorage');

  // Panel width
  it('should set tree panel width');
  it('should persist panel width to localStorage');
});
```

### E2E Tests (Playwright)

#### tests/e2e/locations-desktop.spec.ts
```typescript
describe('Locations - Desktop Layout', () => {
  beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    // Login and navigate to locations
  });

  describe('Split Pane Layout', () => {
    test('should show tree panel on left side');
    test('should show details panel on right side');
    test('should show resizable divider between panels');
    test('should resize panels when dragging divider');
    test('should persist panel width across page reloads');
  });

  describe('Tree Navigation', () => {
    test('should show all root locations');
    test('should expand location to show children on chevron click');
    test('should collapse expanded location on chevron click');
    test('should highlight selected location');
    test('should update details panel when location selected');
    test('should support keyboard navigation (arrow keys)');
    test('should filter tree when searching');
  });

  describe('Details Panel', () => {
    test('should show empty state when no location selected');
    test('should show location details when selected');
    test('should show breadcrumb path');
    test('should show children list');
    test('should navigate to child when clicked in details');
    test('should open edit modal when Edit clicked');
    test('should open move modal when Move clicked');
    test('should open delete confirmation when Delete clicked');
  });

  describe('CRUD Operations', () => {
    test('should create new location and show in tree');
    test('should update location and reflect in tree/details');
    test('should delete location and remove from tree');
    test('should move location and update hierarchy');
  });
});
```

#### tests/e2e/locations-mobile.spec.ts
```typescript
describe('Locations - Mobile Layout', () => {
  beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    // Login and navigate to locations
  });

  describe('Expandable Cards', () => {
    test('should show all root locations as collapsed cards');
    test('should expand card to show details on tap');
    test('should show nested children when parent expanded');
    test('should collapse card on header tap');
    test('should show action buttons in expanded card');
  });

  describe('Navigation', () => {
    test('should NOT show split pane layout');
    test('should NOT show separate tree panel');
    test('should show search bar');
    test('should filter cards when searching');
  });

  describe('Actions', () => {
    test('should open edit modal from expanded card');
    test('should open move modal from expanded card');
    test('should open delete confirmation from expanded card');
    test('should create new location via FAB');
  });

  describe('Responsive Transition', () => {
    test('should switch to split pane when viewport increases');
    test('should preserve selection when switching layouts');
  });
});
```

#### tests/e2e/locations-accessibility.spec.ts
```typescript
describe('Locations - Accessibility', () => {
  test('should have proper ARIA tree role on tree panel');
  test('should have aria-expanded on expandable nodes');
  test('should have aria-selected on selected node');
  test('should announce selection changes to screen readers');
  test('should support full keyboard navigation');
  test('should have visible focus indicators');
  test('should have sufficient color contrast');
  test('should have proper heading hierarchy');
});
```

---

## Implementation Plan

### Phase 1: Core Components
1. Create `LocationSplitPane.tsx`
2. Create `LocationTreePanel.tsx` (extract from LocationTreeView)
3. Create `LocationDetailsPanel.tsx` (extract from LocationDetailsModal)
4. Add UI state to locationStore

### Phase 2: Desktop Integration
1. Update `LocationsScreen.tsx` to use split pane on desktop
2. Implement resizable divider
3. Add keyboard navigation
4. Add localStorage persistence

### Phase 3: Mobile Implementation
1. Create `LocationExpandableCard.tsx`
2. Create `LocationExpandableTree.tsx`
3. Update `LocationsScreen.tsx` with responsive breakpoints
4. Test touch interactions

### Phase 4: Polish & Testing
1. Write all unit tests
2. Write all E2E tests
3. Accessibility audit
4. Performance optimization (virtualization if needed)
5. Dark mode verification

---

## Acceptance Criteria

### Must Have
- [ ] Desktop: Split-pane layout with tree on left, details on right
- [ ] Desktop: Tree view is default (no toggle needed)
- [ ] Desktop: Clicking tree item shows details inline (no modal)
- [ ] Desktop: Resizable divider between panels
- [ ] Mobile: Expandable cards with nested hierarchy
- [ ] Mobile: No conflict with hamburger menu
- [ ] Actions (Edit/Move/Delete) work in new layout
- [ ] Search/filter functionality preserved
- [ ] All existing unit tests pass
- [ ] All existing E2E tests pass
- [ ] New unit tests written and passing
- [ ] New E2E tests written and passing

### Should Have
- [ ] Keyboard navigation in tree (arrow keys)
- [ ] Persist tree expansion state
- [ ] Persist panel width
- [ ] Smooth animations on expand/collapse

### Nice to Have
- [ ] Virtualized tree for large hierarchies
- [ ] Drag-and-drop reordering
- [ ] Multi-select in tree

---

## Test Data

For E2E tests, create the following location hierarchy:

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

---

## Files to Modify

### New Files
- `src/components/locations/LocationSplitPane.tsx`
- `src/components/locations/LocationTreePanel.tsx`
- `src/components/locations/LocationDetailsPanel.tsx`
- `src/components/locations/LocationExpandableCard.tsx`
- `src/components/locations/LocationExpandableTree.tsx`
- `src/components/locations/LocationSplitPane.test.tsx`
- `src/components/locations/LocationTreePanel.test.tsx`
- `src/components/locations/LocationDetailsPanel.test.tsx`
- `src/components/locations/LocationExpandableCard.test.tsx`
- `tests/e2e/locations-desktop.spec.ts`
- `tests/e2e/locations-mobile.spec.ts`
- `tests/e2e/locations-accessibility.spec.ts`

### Modified Files
- `src/components/LocationsScreen.tsx` - Major refactor
- `src/stores/locations/locationStore.ts` - Add UI state
- `src/stores/locations/locationStore.test.ts` - Add UI state tests
- `src/components/locations/index.ts` - Export new components

### Potentially Deprecated
- `src/components/locations/LocationTable.tsx` - May keep as optional view
- `src/components/locations/LocationDetailsModal.tsx` - Details now inline

---

## Notes

- Preserve existing `LocationTreeView.tsx` internally or extract reusable pieces
- The `LocationDetailsModal` content can be reused in `LocationDetailsPanel`
- Consider using `react-split-pane` or building custom divider
- Test on actual mobile devices, not just browser dev tools
- Verify dark mode works correctly for all new components
