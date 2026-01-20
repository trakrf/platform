# TRA-301: Locations Tab Mac Finder-Style Layout

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: [TRA-301](https://linear.app/trakrf/issue/TRA-301/featlocations-redesign-locations-tab-with-mac-finder-style-split-pane)
**Branch**: `feature/TRA-301-locations-finder-layout`

## Outcome
Users can navigate location hierarchies intuitively using a persistent tree view on desktop and expandable cards on mobile, without modal interruptions.

## User Story
As a warehouse manager
I want to browse my location hierarchy with a persistent navigation tree
So that I can quickly drill down and view location details without losing context

## Context

**Current**:
- Default view is **List** (table format) - requires toggle to see Tree
- Details shown in **modal** overlay - interrupts workflow
- Single-pane layout - context lost when switching items
- Location: `frontend/src/components/LocationsScreen.tsx`

**Desired**:
- Desktop: Split-pane with tree nav (left) + details panel (right)
- Mobile: Expandable cards (no second drawer conflict)
- Tree view as default - hierarchy visible immediately
- Details inline - no modal interruption

**Examples**:
- `frontend/src/components/locations/LocationTreeView.tsx` - Existing tree renderer
- `frontend/src/components/locations/LocationDetailsModal.tsx` - Details content to reuse
- `frontend/tests/e2e/hamburger-menu.spec.ts` - Mobile responsive test patterns

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

### Styling Patterns (from existing components)

```tsx
// Button styles from LocationDetailsModal.tsx
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

// Active status badge
className="bg-green-50 text-green-700 border border-green-200
           dark:bg-green-900/20 dark:text-green-400 dark:border-green-800"

// Selected item
className="bg-blue-100 dark:bg-blue-900/40 border border-blue-300 dark:border-blue-700"
```

## Technical Requirements

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
│   │  └─ Section B  │   Identifier: floor-1                               │
│   └─ 📍 Floor 2    │   Name: Floor 1                                     │
│ ▶ 🏢 Warehouse B   │   Status: ● Active                                  │
│                    │   Path: warehouse-a > floor-1                       │
│                    │                                                     │
│                    │   [Edit]  [Move]  [Delete]                          │
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
│ ▶ 🏢 Warehouse B   ● Active │
├─────────────────────────────┤
│ Total: 6 │ Active: 6 │ ...  │
└─────────────────────────────┘
```

### Component Architecture

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

### State Management (locationStore additions)

```typescript
interface LocationUIState {
  selectedLocationId: number | null;
  expandedNodeIds: Set<number>;      // Desktop tree
  expandedCardIds: Set<number>;      // Mobile cards
  treePanelWidth: number;            // Resizable panel
}

interface LocationUIActions {
  setSelectedLocation: (id: number | null) => void;
  toggleNodeExpanded: (id: number) => void;
  toggleCardExpanded: (id: number) => void;
  setTreePanelWidth: (width: number) => void;
  expandToLocation: (id: number) => void;
}
```

### Responsive Breakpoints

| Breakpoint | Width | Layout |
|------------|-------|--------|
| Mobile | <768px | Expandable cards |
| Tablet | 768-1023px | Narrow split or cards |
| Desktop | ≥1024px | Full split-pane |

## Validation Criteria

### Unit Tests (Vitest + React Testing Library)

#### LocationTreePanel.test.tsx
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

#### LocationDetailsPanel.test.tsx
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

#### LocationExpandableCard.test.tsx
- [ ] Shows identifier, name, status when collapsed
- [ ] Shows expand chevron
- [ ] Shows full details when expanded
- [ ] Shows action buttons when expanded
- [ ] Shows children cards when expanded
- [ ] Toggles expanded state on header click
- [ ] Does NOT collapse when clicking action buttons
- [ ] Nests children with proper indentation

#### LocationSplitPane.test.tsx
- [ ] Renders tree panel on left
- [ ] Renders details panel on right
- [ ] Shows resizable divider
- [ ] Resizes panels on divider drag
- [ ] Respects minimum panel width (200px)
- [ ] Respects maximum panel width (400px)
- [ ] Persists panel width to localStorage
- [ ] Hides on mobile viewport (<768px)
- [ ] Shows on desktop viewport (≥1024px)

#### locationStore.test.ts (UI state additions)
- [ ] Sets selected location
- [ ] Clears selection
- [ ] Toggles node expansion
- [ ] Expands all ancestors when expandToLocation called
- [ ] Persists expanded nodes to localStorage
- [ ] Restores expanded nodes from localStorage
- [ ] Toggles card expansion
- [ ] Sets tree panel width

### E2E Tests (Playwright)

#### tests/e2e/locations-desktop.spec.ts
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

#### tests/e2e/locations-mobile.spec.ts
- [ ] Shows all root locations as collapsed cards
- [ ] Expands card to show details on tap
- [ ] Shows nested children when parent expanded
- [ ] Collapses card on header tap
- [ ] Shows action buttons in expanded card
- [ ] Does NOT show split pane layout
- [ ] Does NOT show separate tree panel
- [ ] Shows search bar
- [ ] Filters cards when searching
- [ ] Opens edit modal from expanded card
- [ ] Opens move modal from expanded card
- [ ] Opens delete confirmation from expanded card
- [ ] Creates new location via FAB
- [ ] Switches to split pane when viewport increases
- [ ] Preserves selection when switching layouts

#### tests/e2e/locations-accessibility.spec.ts
- [ ] Has proper ARIA tree role on tree panel
- [ ] Has aria-expanded on expandable nodes
- [ ] Has aria-selected on selected node
- [ ] Announces selection changes to screen readers
- [ ] Supports full keyboard navigation
- [ ] Has visible focus indicators
- [ ] Has sufficient color contrast
- [ ] Has proper heading hierarchy

## Success Metrics

- [ ] All existing location-related unit tests pass
- [ ] All existing location-related E2E tests pass
- [ ] 45+ new unit tests written and passing
- [ ] 40+ new E2E tests written and passing
- [ ] Desktop split-pane renders correctly at 1280x800
- [ ] Mobile cards render correctly at 375x667
- [ ] Tree keyboard navigation works (arrow keys)
- [ ] Dark mode verified for all new components
- [ ] No console errors during E2E tests
- [ ] Panel resize persists across page reloads

## Implementation Plan

### Phase 1: Core Components
1. Create `LocationSplitPane.tsx`
2. Create `LocationTreePanel.tsx` (extract from LocationTreeView)
3. Create `LocationDetailsPanel.tsx` (extract from LocationDetailsModal)
4. Add UI state to locationStore
5. Write unit tests for new components

### Phase 2: Desktop Integration
1. Update `LocationsScreen.tsx` to use split pane on desktop
2. Implement resizable divider
3. Add keyboard navigation
4. Add localStorage persistence
5. Write desktop E2E tests

### Phase 3: Mobile Implementation
1. Create `LocationExpandableCard.tsx`
2. Create `LocationExpandableTree.tsx`
3. Update `LocationsScreen.tsx` with responsive breakpoints
4. Test touch interactions
5. Write mobile E2E tests

### Phase 4: Polish & Testing
1. Accessibility audit and fixes
2. Write accessibility E2E tests
3. Performance optimization (virtualization if needed)
4. Dark mode verification
5. Final test pass

## File Changes Summary

### New Files
| File | Description |
|------|-------------|
| `src/components/locations/LocationSplitPane.tsx` | Desktop split layout (~100 lines) |
| `src/components/locations/LocationTreePanel.tsx` | Tree navigation panel (~150 lines) |
| `src/components/locations/LocationDetailsPanel.tsx` | Details display panel (~200 lines) |
| `src/components/locations/LocationExpandableCard.tsx` | Mobile expandable card (~120 lines) |
| `src/components/locations/LocationExpandableTree.tsx` | Mobile nested cards (~80 lines) |
| `src/components/locations/LocationSplitPane.test.tsx` | Unit tests |
| `src/components/locations/LocationTreePanel.test.tsx` | Unit tests |
| `src/components/locations/LocationDetailsPanel.test.tsx` | Unit tests |
| `src/components/locations/LocationExpandableCard.test.tsx` | Unit tests |
| `tests/e2e/locations-desktop.spec.ts` | E2E tests |
| `tests/e2e/locations-mobile.spec.ts` | E2E tests |
| `tests/e2e/locations-accessibility.spec.ts` | E2E tests |

### Modified Files
| File | Changes |
|------|---------|
| `src/components/LocationsScreen.tsx` | Major refactor - responsive layout |
| `src/stores/locations/locationStore.ts` | Add UI state |
| `src/stores/locations/locationStore.test.ts` | Add UI state tests |
| `src/components/locations/index.ts` | Export new components |

### Test Data
Create hierarchy for E2E tests:
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

## References

- [Apple HIG - Designing for macOS](https://developer.apple.com/design/human-interface-guidelines/designing-for-macos)
- [PatternFly Wizard Component](https://www.patternfly.org/components/wizard)
- [react-arborist](https://github.com/brimdata/react-arborist) - Mac Finder-style tree
- [react-split-pane](https://github.com/tomkp/react-split-pane) - Split pane component
- [MUI X Tree View](https://mui.com/x/react-tree-view/) - Tree component reference
- Existing: `frontend/src/components/locations/LocationTreeView.tsx`
- Existing: `frontend/src/components/locations/LocationDetailsModal.tsx`
- Existing: `frontend/tests/e2e/hamburger-menu.spec.ts` - Mobile test patterns
