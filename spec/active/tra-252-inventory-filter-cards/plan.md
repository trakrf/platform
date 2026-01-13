# Implementation Plan: Inventory Filter Cards (TRA-252)

Generated: 2026-01-13
Specification: spec.md

## Understanding

Convert the 4 static stat cards (Found, Missing, Not Listed, Total Scanned) at the bottom of the inventory page into clickable filter toggles. Multiple filters can be active simultaneously (multi-select). Remove the dropdown filter from the header. "Total Scanned" acts as a reset button to clear all filters.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/SortableHeader.tsx` (lines 39-58) - Keyboard accessibility pattern with `role="button"`, `tabIndex`, `onKeyDown`
- `frontend/src/components/inventory/InventoryHeader.tsx` (lines 55-70) - Conditional button styling with template literals

**Files to Modify**:
- `frontend/src/components/inventory/InventoryStats.tsx` - Add click handlers, selected state, accessibility
- `frontend/src/components/InventoryScreen.tsx` (lines 23, 61-73, 136-155, 213) - Update state to Set, modify filter logic, update props
- `frontend/src/components/inventory/InventoryHeader.tsx` - Remove dropdown component and related props

**Files to Delete**:
- `frontend/src/components/inventory/InventoryStatusFilter.tsx` - No longer needed

## Architecture Impact

- **Subsystems affected**: UI only
- **New dependencies**: None
- **Breaking changes**: None (internal refactor)

## Task Breakdown

### Task 1: Update InventoryStats.tsx - Add Props Interface

**File**: `frontend/src/components/inventory/InventoryStats.tsx`
**Action**: MODIFY

**Implementation**:
```typescript
interface InventoryStatsProps {
  stats: {
    found: number;
    missing: number;
    notListed: number;
    totalScanned: number;
    hasReconciliation: boolean;
  };
  activeFilters: Set<string>;           // NEW
  onToggleFilter: (filter: string) => void;  // NEW
  onClearFilters: () => void;           // NEW
}
```

**Validation**: `just frontend typecheck`

---

### Task 2: Update InventoryStats.tsx - Convert Cards to Buttons

**File**: `frontend/src/components/inventory/InventoryStats.tsx`
**Action**: MODIFY

**Implementation**:
Convert each `<div>` card to a `<button>` with:
- `onClick` handler calling `onToggleFilter('Found')` etc.
- Conditional ring class when filter is active
- `aria-pressed` attribute for accessibility
- `cursor-pointer` and hover states

Pattern for Found card:
```tsx
<button
  onClick={() => onToggleFilter('Found')}
  className={`bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg p-2 md:p-3 cursor-pointer transition-shadow text-left w-full ${
    activeFilters.has('Found') ? 'ring-2 ring-green-500 ring-offset-1 dark:ring-offset-gray-900' : ''
  }`}
  aria-pressed={activeFilters.has('Found')}
>
  {/* existing content unchanged */}
</button>
```

Ring colors per card:
- Found: `ring-green-500`
- Missing: `ring-red-500`
- Not Listed: `ring-gray-500`
- Total Scanned: `ring-blue-500` (clears filters via `onClearFilters`)

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 3: Update InventoryScreen.tsx - Change State to Set

**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY

**Changes at line 23**:
```typescript
// Before:
const [statusFilter, setStatusFilter] = useState('All Status');

// After:
const [statusFilters, setStatusFilters] = useState<Set<string>>(new Set());
```

**Add handlers after line 106**:
```typescript
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

**Validation**: `just frontend typecheck`

---

### Task 4: Update InventoryScreen.tsx - Update Filter Logic

**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY

**Changes at lines 61-73** (filteredTags useMemo):
```typescript
const filteredTags = useMemo(() => {
  return sortedTags.filter(tag => {
    const matchesSearch = !searchTerm ||
      (tag.displayEpc || tag.epc).toLowerCase().includes(searchTerm.toLowerCase());

    // Multi-select: empty set = show all, otherwise OR logic
    const matchesStatus = statusFilters.size === 0 ||
      (statusFilters.has('Found') && tag.reconciled === true) ||
      (statusFilters.has('Missing') && tag.reconciled === false) ||
      (statusFilters.has('Not Listed') && (tag.reconciled === null || tag.reconciled === undefined));

    return matchesSearch && matchesStatus;
  });
}, [sortedTags, searchTerm, statusFilters]);
```

**Update useEffect dependency at line 75-77**:
```typescript
useEffect(() => {
  setCurrentPage(1);
}, [searchTerm, statusFilters, setCurrentPage]);  // statusFilter -> statusFilters
```

**Validation**: `just frontend typecheck`

---

### Task 5: Update InventoryScreen.tsx - Update Component Props

**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY

**Remove props from InventoryHeader (around line 141-142)**:
```typescript
// Remove these lines:
statusFilter={statusFilter}
onStatusFilterChange={setStatusFilter}
```

**Update InventoryStats props (around line 213)**:
```typescript
<InventoryStats
  stats={stats}
  activeFilters={statusFilters}
  onToggleFilter={handleToggleFilter}
  onClearFilters={handleClearFilters}
/>
```

**Validation**: `just frontend typecheck`

---

### Task 6: Update InventoryHeader.tsx - Remove Dropdown

**File**: `frontend/src/components/inventory/InventoryHeader.tsx`
**Action**: MODIFY

**Remove import (line 5)**:
```typescript
// Delete this line:
import { InventoryStatusFilter } from './InventoryStatusFilter';
```

**Remove from interface (lines 13-14)**:
```typescript
// Delete these lines:
statusFilter: string;
onStatusFilterChange: (value: string) => void;
```

**Remove from destructuring (lines 31-32)**:
```typescript
// Delete these lines:
statusFilter,
onStatusFilterChange,
```

**Remove component usages**:
- Line 111: Delete `<InventoryStatusFilter value={statusFilter} onChange={onStatusFilterChange} />`
- Line 127: Delete `<InventoryStatusFilter value={statusFilter} onChange={onStatusFilterChange} />`

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 7: Delete InventoryStatusFilter.tsx

**File**: `frontend/src/components/inventory/InventoryStatusFilter.tsx`
**Action**: DELETE

```bash
rm frontend/src/components/inventory/InventoryStatusFilter.tsx
```

**Validation**: `just frontend typecheck && just frontend build`

---

### Task 8: Final Validation

**Action**: Full validation suite

```bash
just frontend validate
```

Verify:
- [ ] Clicking cards toggles filters
- [ ] Multiple cards can be selected
- [ ] "Total Scanned" clears all filters
- [ ] No TypeScript errors
- [ ] No lint errors
- [ ] Build succeeds

## Risk Assessment

- **Risk**: Stats calculated from filteredTags may show 0s when filters active
  **Mitigation**: This is expected per user decision (counts reflect filtered results). If UX is confusing, can adjust stats to use `sortedTags` (pre-status-filter) in follow-up.

- **Risk**: Set state doesn't trigger re-render if mutated in place
  **Mitigation**: Always create new Set instance: `new Set(prev)` then modify

## Integration Points

- Store updates: None (local component state only)
- Route changes: None
- Config updates: None

## VALIDATION GATES (MANDATORY)

After EVERY task:
```bash
just frontend typecheck   # Gate 1: Type Safety
just frontend lint        # Gate 2: Style
```

After Task 7 & 8:
```bash
just frontend validate    # Full validation
```

**If ANY gate fails → Fix immediately → Re-run → Repeat until pass**

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- Clear requirements from spec
- Similar accessibility pattern found in `SortableHeader.tsx`
- Simple state change (string → Set)
- No external dependencies
- No backend changes
- Existing button styling patterns to follow

**Assessment**: Straightforward UI refactor with clear patterns to follow.

**Estimated one-pass success probability**: 90%

**Reasoning**: All code patterns exist in codebase, no architectural changes, single subsystem (UI), clear task boundaries. Main risk is typos or missed prop removals, which typecheck will catch.
