# Feature: Inventory Count Panels as Clickable Filters

## Linear Issue
[TRA-252](https://linear.app/trakrf/issue/TRA-252/turn-the-inventory-count-panels-into-filters) - Turn the inventory count panels into filters

## Origin
This specification captures an enhancement to the inventory page UX. Currently, stat cards display counts (Found, Missing, Not Listed, Total Scanned) but require a separate dropdown to filter. Users expect to click directly on these cards to filter.

## Outcome
Clicking an inventory stat card immediately filters the table to show only items in that category, with visual feedback indicating the active filter state.

## User Story
As a **warehouse operator doing inventory reconciliation**
I want **to click on the "Missing" count card to immediately see only missing items**
So that **I can quickly focus on items that need attention without using a separate dropdown**

## Context

**Current State:**
- `InventoryStats.tsx` renders 4 static display cards at bottom of inventory page
- `InventoryStatusFilter.tsx` provides a separate dropdown for filtering (in header)
- Filtering logic exists in `InventoryScreen.tsx` lines 61-73
- `statusFilter` state: `'All Status' | 'Found' | 'Missing' | 'Not Listed'` (single-select)

**Desired State:**
- Cards are clickable and act as filter toggle buttons
- Multiple cards can be selected simultaneously (multi-select)
- Active cards show selected state (ring/border highlight)
- "Total Scanned" clears all filters (shows everything)
- Dropdown filter removed entirely

## Technical Requirements

### State Changes

**Before (single-select):**
```typescript
const [statusFilter, setStatusFilter] = useState('All Status');
// Filter: statusFilter === 'Found' || statusFilter === 'All Status'
```

**After (multi-select):**
```typescript
const [statusFilters, setStatusFilters] = useState<Set<string>>(new Set());
// Empty set = show all (no filters active)
// Filter: statusFilters.size === 0 || statusFilters.has(tag.status)
```

### Component Changes

1. **InventoryStats.tsx** - Add interactivity
   - Accept `activeFilters: Set<string>` prop (set of active filter values)
   - Accept `onToggleFilter: (filter: string) => void` callback
   - Make each card clickable (button element)
   - Add visual selected state (ring/border) when filter is in activeFilters
   - Support keyboard navigation (Tab, Enter, Space)
   - Toggle behavior: clicking adds/removes that filter from the set
   - Card mappings:
     - Found card → toggles `'Found'`
     - Missing card → toggles `'Missing'`
     - Not Listed card → toggles `'Not Listed'`
     - Total Scanned card → clears all filters (resets to empty set)

2. **InventoryScreen.tsx** - Update state and filtering
   - Change `statusFilter: string` to `statusFilters: Set<string>`
   - Update filtering logic:
     ```typescript
     const matchesStatus = statusFilters.size === 0 ||
       (statusFilters.has('Found') && tag.reconciled === true) ||
       (statusFilters.has('Missing') && tag.reconciled === false) ||
       (statusFilters.has('Not Listed') && (tag.reconciled === null || tag.reconciled === undefined));
     ```
   - Pass `statusFilters` and toggle handler to InventoryStats
   - Remove dropdown filter props from InventoryHeader

3. **InventoryHeader.tsx** - Remove dropdown
   - Remove `InventoryStatusFilter` component and related props
   - Reclaim horizontal space for search bar on mobile
   - Delete `statusFilter` and `onStatusFilterChange` props

4. **InventoryStatusFilter.tsx** - Delete file
   - No longer needed after cards become filters

### Visual Design

**Default State (unselected):**
- Current card styling with standard border

**Selected State:**
- Ring highlight matching card color
- Slightly elevated shadow (optional)
- Example: `ring-2 ring-green-500` for active Found filter

**Hover State:**
- Cursor pointer
- Subtle hover effect (brightness or scale)

### Accessibility

- Cards must be focusable (`tabIndex={0}` or use `<button>`)
- Announce filter changes to screen readers
- Support keyboard activation (Enter/Space)
- Use `aria-pressed` or `aria-selected` for toggle state

## Code Examples

Current card (static):
```tsx
<div className="bg-green-50 ...">
  <span>Found</span>
  <div>{stats.found}</div>
</div>
```

Proposed card (interactive, multi-select):
```tsx
<button
  onClick={() => onToggleFilter('Found')}
  className={cn(
    "bg-green-50 ... cursor-pointer transition-shadow",
    activeFilters.has('Found') && "ring-2 ring-green-500 ring-offset-1"
  )}
  aria-pressed={activeFilters.has('Found')}
>
  <span>Found</span>
  <div>{stats.found}</div>
</button>
```

Toggle handler in InventoryScreen:
```tsx
const handleToggleFilter = useCallback((filter: string) => {
  setStatusFilters(prev => {
    const next = new Set(prev);
    if (next.has(filter)) {
      next.delete(filter);
    } else {
      next.add(filter);
    }
    return next;
  });
}, []);

const handleClearFilters = useCallback(() => {
  setStatusFilters(new Set());
}, []);
```

## Validation Criteria

- [ ] Clicking "Found" card toggles found items filter
- [ ] Clicking "Missing" card toggles missing items filter
- [ ] Clicking "Not Listed" card toggles not-listed items filter
- [ ] Multiple filters can be active simultaneously (e.g., Found + Missing shows both)
- [ ] Clicking "Total Scanned" card clears all filters (shows all)
- [ ] Clicking active card again removes that filter (toggle off)
- [ ] No filters active = show all items (same as "All Status" previously)
- [ ] Selected cards show visual highlight (ring/border)
- [ ] Multiple cards can be highlighted at once
- [ ] Keyboard navigation works (Tab to card, Enter/Space to activate)
- [ ] Screen reader announces filter state changes
- [ ] Dropdown filter removed from header
- [ ] Mobile header has more space for search bar

## Out of Scope

- Changing card colors or base styling
- Persisting filter state to URL or storage
- Animation/transition effects (keep simple for now)
- Mobile card size changes (will test viability separately)

## Files to Modify

1. `frontend/src/components/inventory/InventoryStats.tsx` - Add click handlers and selected state
2. `frontend/src/components/InventoryScreen.tsx` - Pass filter state to InventoryStats, remove dropdown props
3. `frontend/src/components/inventory/InventoryHeader.tsx` - Remove dropdown component and props
4. `frontend/src/components/inventory/InventoryStatusFilter.tsx` - Delete file

## Related Issues

- Parent: [TRA-250](https://linear.app/trakrf/issue/TRA-250/nada-launch-requirements) - NADA launch requirements
