# Implementation Plan: TRA-301 Extra - Simplify Locations UI

Generated: 2026-01-23
Specification: spec.md

## Understanding

Simplify the Locations UI based on stakeholder feedback:
1. **Remove "Hierarchy Information" section** from details panel and mobile cards
2. **Context-aware Add button**: Tree selection determines parent (no dropdown in create mode)
3. **Keep parent selector in Edit mode** (per user decision)
4. **Add inline "+" button on mobile cards** for creating children (FAB creates root)

---

## Relevant Files

**Reference Patterns**:
- `LocationDetailsPanel.tsx` (lines 176-234) - Hierarchy section to remove
- `LocationExpandableCard.tsx` (lines 184-198) - Info grid to remove
- `LocationForm.tsx` (lines 356-369) - Parent selector pattern

**Files to Modify**:
| File | Changes |
|------|---------|
| `LocationDetailsPanel.tsx` | Remove Hierarchy Information section, rename "Direct Children" to "Sub-locations" |
| `LocationExpandableCard.tsx` | Remove Type/Children grid, add inline "+" button |
| `LocationForm.tsx` | Conditionally hide parent selector in create mode, add context message |
| `LocationFormModal.tsx` | Accept `parentLocationId` prop, pass to form |
| `LocationsScreen.tsx` | Pass `selectedLocationId` to create modal |
| `LocationDetailsPanel.test.tsx` | Update tests for new structure |
| `LocationExpandableCard.test.tsx` | Update tests, add tests for "+" button |

---

## Architecture Impact

- **Subsystems affected**: UI only
- **New dependencies**: None
- **Breaking changes**: None (UI simplification only)

---

## Task Breakdown

### Task 1: Remove Hierarchy Information from LocationDetailsPanel

**File**: `frontend/src/components/locations/LocationDetailsPanel.tsx`
**Action**: MODIFY

**Implementation**:
1. Remove lines 176-199 (entire "Hierarchy Information" section with grid)
2. Keep lines 201-233 (children list) but update the label
3. Change "Direct Children:" to "Sub-locations ({count}):"

**Before** (lines 176-234):
```tsx
{/* Hierarchy Information */}
<div className="border-t border-gray-200 dark:border-gray-700 pt-4 space-y-4">
  <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
    Hierarchy Information
  </h3>
  <div className="grid grid-cols-2 gap-4">
    {/* Type, Children count, Descendants - REMOVE ALL */}
  </div>
  {/* Children list - KEEP but simplify label */}
</div>
```

**After**:
```tsx
{/* Sub-locations */}
{children.length > 0 && (
  <div className="border-t border-gray-200 dark:border-gray-700 pt-4">
    <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-3">
      Sub-locations ({children.length})
    </h3>
    <div className="space-y-2 max-h-48 overflow-y-auto">
      {children.map((child) => (
        // ... existing child rendering
      ))}
    </div>
  </div>
)}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 2: Remove Type/Children Grid from LocationExpandableCard

**File**: `frontend/src/components/locations/LocationExpandableCard.tsx`
**Action**: MODIFY

**Implementation**:
1. Remove lines 184-198 (Info grid with Type and Children)
2. Remove unused variables: `descendants`, `getDescendants`
3. Keep the `isRoot` variable for the Icon selection

**Before** (lines 184-198):
```tsx
{/* Info grid */}
<div className="grid grid-cols-2 gap-3">
  <div className="p-2 bg-gray-50 dark:bg-gray-800 rounded">
    <p className="text-xs text-gray-500 dark:text-gray-400">Type</p>
    <p className="text-sm font-medium text-gray-900 dark:text-white">
      {isRoot ? 'Root Location' : 'Subsidiary'}
    </p>
  </div>
  <div className="p-2 bg-gray-50 dark:bg-gray-800 rounded">
    <p className="text-xs text-gray-500 dark:text-gray-400">Children</p>
    <p className="text-sm font-medium text-gray-900 dark:text-white">
      {children.length} direct / {descendants.length} total
    </p>
  </div>
</div>
```

**After**: Remove entire grid section.

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 3: Add Inline "+" Button to LocationExpandableCard

**File**: `frontend/src/components/locations/LocationExpandableCard.tsx`
**Action**: MODIFY

**Implementation**:
1. Add `onAddChild` prop to interface
2. Add "+" button in the action buttons section (when expanded)
3. Style consistent with other action buttons

**Props addition**:
```tsx
export interface LocationExpandableCardProps {
  // ... existing props
  onAddChild?: (parentId: number) => void;  // NEW
}
```

**Button addition** (after Delete button, lines ~216-222):
```tsx
{onAddChild && (
  <button
    onClick={(e) => {
      e.stopPropagation();
      onAddChild(location.id);
    }}
    className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-green-700 bg-green-50 hover:bg-green-100 dark:text-green-400 dark:bg-green-900/20 dark:hover:bg-green-900/40 border border-green-200 dark:border-green-800 rounded-lg transition-colors"
  >
    <Plus className="h-4 w-4" />
    Add Child
  </button>
)}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 4: Update LocationMobileView to Pass onAddChild

**File**: `frontend/src/components/locations/LocationMobileView.tsx`
**Action**: MODIFY

**Implementation**:
1. Add `onAddChild` prop to interface
2. Pass to LocationExpandableCard

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 5: Add parentLocationId Prop to LocationFormModal

**File**: `frontend/src/components/locations/LocationFormModal.tsx`
**Action**: MODIFY

**Implementation**:
1. Add `parentLocationId?: number | null` to props interface
2. Pass to LocationForm component

**Props change**:
```tsx
interface LocationFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  location?: Location;
  parentLocationId?: number | null;  // NEW
  onClose: () => void;
}
```

**Pass to form**:
```tsx
<LocationForm
  mode={mode}
  location={location}
  parentLocationId={parentLocationId}  // NEW
  onSubmit={handleSubmit}
  onCancel={onClose}
  loading={loading}
  error={error}
/>
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 6: Update LocationForm for Context-Aware Parent

**File**: `frontend/src/components/locations/LocationForm.tsx`
**Action**: MODIFY

**Implementation**:
1. Add `parentLocationId?: number | null` prop
2. Show context message in create mode instead of dropdown
3. Keep dropdown visible in edit mode
4. Initialize `parent_location_id` from prop in create mode

**Props addition**:
```tsx
interface LocationFormProps {
  mode: 'create' | 'edit';
  location?: Location;
  parentLocationId?: number | null;  // NEW
  onSubmit: (data: LocationFormData) => void;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}
```

**Context message** (replace lines 356-369 for create mode):
```tsx
{mode === 'create' ? (
  <div>
    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
      Parent Location
    </label>
    <div className="p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
      {parentLocationId ? (
        <div className="flex items-center gap-2">
          <MapPin className="h-4 w-4 text-blue-600 dark:text-blue-400" />
          <span className="text-sm text-blue-700 dark:text-blue-300">
            Creating inside: <span className="font-medium">{parentLocation?.identifier}</span>
          </span>
        </div>
      ) : (
        <span className="text-sm text-blue-700 dark:text-blue-300">
          Creating a top-level location
        </span>
      )}
    </div>
  </div>
) : (
  // Edit mode: keep existing LocationParentSelector
  <div>
    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
      Parent Location
    </label>
    <LocationParentSelector
      value={formData.parent_location_id}
      onChange={(value) => handleChange('parent_location_id', value)}
      currentLocationId={location?.id}
      disabled={loading}
    />
  </div>
)}
```

**Initialize from prop** (in useEffect for create mode):
```tsx
} else if (mode === 'create') {
  setFormData({
    identifier: '',
    name: '',
    description: '',
    parent_location_id: parentLocationId ?? null,  // Use prop
    valid_from: '',
    valid_to: '',
    is_active: true,
  });
  // ...
}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 7: Update LocationsScreen to Pass selectedLocationId

**File**: `frontend/src/components/LocationsScreen.tsx`
**Action**: MODIFY

**Implementation**:
1. Get `selectedLocationId` from store
2. Add handler for mobile "Add Child" action
3. Pass `parentLocationId` to create modal

**Get selected location**:
```tsx
const selectedLocationId = useLocationStore((state) => state.selectedLocationId);
```

**Handler for mobile add child**:
```tsx
const handleAddChild = useCallback((parentId: number) => {
  // Store the parent ID and open create modal
  setCreateParentId(parentId);
  setIsCreateModalOpen(true);
}, []);

// New state for create parent context
const [createParentId, setCreateParentId] = useState<number | null>(null);
```

**Update modal**:
```tsx
<LocationFormModal
  isOpen={isCreateModalOpen}
  mode="create"
  parentLocationId={isDesktop ? selectedLocationId : createParentId}
  onClose={() => {
    setIsCreateModalOpen(false);
    setCreateParentId(null);
  }}
/>
```

**Pass to mobile view**:
```tsx
<LocationMobileView
  searchTerm={filters.search || ''}
  onEdit={handleEditById}
  onMove={handleMoveById}
  onDelete={handleDeleteById}
  onAddChild={handleAddChild}  // NEW
/>
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 8: Update LocationDetailsPanel Tests

**File**: `frontend/src/components/locations/LocationDetailsPanel.test.tsx`
**Action**: MODIFY

**Implementation**:
1. Remove tests that check for "Hierarchy Information" section
2. Remove tests for "Root Location" / "Subsidiary" type display
3. Remove tests for "Direct Children" / "Total Descendants" counts
4. Add test: shows "Sub-locations" with count when children exist
5. Add test: does NOT show "Hierarchy Information" heading

**Validation**: `just frontend test -- LocationDetailsPanel`

---

### Task 9: Update LocationExpandableCard Tests

**File**: `frontend/src/components/locations/LocationExpandableCard.test.tsx`
**Action**: MODIFY

**Implementation**:
1. Remove tests for Type display
2. Remove tests for Children count display
3. Add test: renders "Add Child" button when onAddChild provided
4. Add test: calls onAddChild with location.id when clicked
5. Add test: does NOT render "Add Child" when onAddChild not provided

**Validation**: `just frontend test -- LocationExpandableCard`

---

### Task 10: Add Tests for LocationForm Context Message

**File**: `frontend/src/components/locations/LocationForm.test.tsx`
**Action**: MODIFY (or CREATE if doesn't exist)

**Implementation**:
1. Add test: shows "Creating a top-level location" when no parentLocationId
2. Add test: shows "Creating inside: {identifier}" when parentLocationId provided
3. Add test: does NOT show LocationParentSelector in create mode
4. Add test: shows LocationParentSelector in edit mode

**Validation**: `just frontend test -- LocationForm`

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing tests | Medium | Low | Run tests after each task, fix incrementally |
| Mobile layout issues | Low | Medium | Test on various viewport sizes |
| Parent context lost on modal close | Low | Medium | Reset createParentId state on close |

---

## VALIDATION GATES (MANDATORY)

After EVERY task, run:
```bash
just frontend lint
just frontend typecheck
just frontend test
```

**Do not proceed to next task until current task passes all gates.**

---

## Validation Sequence

After all tasks complete:
```bash
just frontend validate   # Full frontend validation
just validate            # Full stack validation
```

---

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and stakeholder feedback
- ✅ All files identified and read
- ✅ Existing patterns to follow for styling and testing
- ✅ No new dependencies required
- ✅ Straightforward UI changes (remove code, add props)

**Assessment**: High-confidence plan with well-scoped changes. Most tasks involve removing code or adding simple conditional logic.

**Estimated one-pass success probability**: 90%

**Reasoning**: Changes are isolated to UI components, no backend changes, clear patterns exist for all modifications.
