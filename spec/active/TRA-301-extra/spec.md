# TRA-301 Follow-up: Simplify Locations UI

**Ticket:** TRA-301 (follow-up)
**Author:** Claude Code
**Date:** 2026-01-23
**Status:** Complete
**PR:** https://github.com/trakrf/platform/pull/136
**Workspace:** frontend

---

## Summary

Simplify the Locations tab UI based on stakeholder feedback from miks2u:

1. **Remove "Hierarchy Information" section** from the details panel
2. **Context-aware Add button**: Use tree selection to determine parent of new locations
3. **Future: Drag-and-drop** to adjust hierarchy (out of scope for this spec)

---

## Problem Statement

The current implementation has UX friction:

1. **Redundant information**: "Hierarchy Information" section shows Parent, Children count, and Descendants count - information that's already visible in the tree structure
2. **Manual parent selection**: When creating a location, users must manually select a parent from a dropdown, even when they've already selected a location in the tree
3. **Cognitive overhead**: Users have to think "where does this go?" instead of the system inferring from context

---

## Proposed Solution

### Change 1: Remove Hierarchy Information Section

**Current Details Panel:**
```
┌──────────────────────────────────┐
│  📍 Floor 1                      │
│  ─────────────────────────────   │
│                                  │
│  Identifier: floor-1             │
│  Name: Floor 1                   │
│  Status: ● Active                │
│                                  │
│  ┌────────────────────────────┐  │
│  │ HIERARCHY INFORMATION      │  │  ← REMOVE THIS
│  │ Type: Subsidiary Location  │  │
│  │ Parent: Warehouse A        │  │
│  │ Direct Children: 2         │  │
│  │ Total Descendants: 2       │  │
│  └────────────────────────────┘  │
│                                  │
│  Children:                       │
│    → Section A                   │
│    → Section B                   │
│                                  │
│  [Edit] [Move] [Delete]          │
└──────────────────────────────────┘
```

**New Details Panel:**
```
┌──────────────────────────────────┐
│  📍 Floor 1                      │
│  ─────────────────────────────   │
│                                  │
│  Identifier: floor-1             │
│  Name: Floor 1                   │
│  Status: ● Active                │
│  Description: Main floor area    │
│                                  │
│  Sub-locations (2):              │  ← Simplified, just show children
│    → Section A                   │
│    → Section B                   │
│                                  │
│  [Edit] [Move] [Delete]          │
└──────────────────────────────────┘
```

**Rationale:**
- Parent is visible in tree (selected node's position shows hierarchy)
- Children count is implicit from the list
- Descendants count rarely needed for day-to-day use
- "Root Location" vs "Subsidiary" terminology was confusing (per earlier feedback)

---

### Change 2: Context-Aware Add Button

**New Behavior:**

| Tree Selection State | Click [+ Add] | Result |
|---------------------|---------------|--------|
| Nothing selected | Opens form | Creates **root-level** location |
| Location X selected | Opens form | Creates **child of X** |

**Form Changes:**

**When nothing selected (creating root):**
```
┌─────────────────────────────────┐
│  Create Location                │
│  ─────────────────────────────  │
│                                 │
│  ℹ️  Creating a top-level       │
│     location                    │
│                                 │
│  Identifier: [_______________]  │
│  Name:       [_______________]  │
│  Description: [______________]  │
│                                 │
│  [Cancel]           [Create]    │
└─────────────────────────────────┘
```

**When "Warehouse A" selected (creating child):**
```
┌─────────────────────────────────┐
│  Create Location                │
│  ─────────────────────────────  │
│                                 │
│  ℹ️  Creating inside:           │
│     📍 Warehouse A              │
│                                 │
│  Identifier: [_______________]  │
│  Name:       [_______________]  │
│  Description: [______________]  │
│                                 │
│  [Cancel]           [Create]    │
└─────────────────────────────────┘
```

**Key Points:**
- Remove `parent_location_id` dropdown from create form
- Parent is determined by `selectedLocationId` from store
- Show clear context message so user knows where location will be created
- User can still change parent via "Move" action after creation if needed

---

### Change 3: Mobile Expandable Cards

Apply same simplification to mobile view:

**Current Mobile Card (expanded):**
```
┌─────────────────────────────────┐
│ ▼ 📍 Warehouse A        ● Active│
│ ┌─────────────────────────────┐ │
│ │ Identifier: warehouse-a     │ │
│ │ Type: Root Location         │ │  ← REMOVE
│ │ Direct Children: 2          │ │  ← REMOVE
│ │ Total Descendants: 4        │ │  ← REMOVE
│ │                             │ │
│ │ [Edit] [Move] [Delete]      │ │
│ │                             │ │
│ │ ▶ 📍 Floor 1                │ │
│ │ ▶ 📍 Floor 2                │ │
│ └─────────────────────────────┘ │
└─────────────────────────────────┘
```

**New Mobile Card (expanded):**
```
┌─────────────────────────────────┐
│ ▼ 📍 Warehouse A        ● Active│
│ ┌─────────────────────────────┐ │
│ │ Identifier: warehouse-a     │ │
│ │ Description: Main warehouse │ │
│ │                             │ │
│ │ [Edit] [Move] [Delete]      │ │
│ │                             │ │
│ │ Sub-locations:              │ │
│ │ ▶ 📍 Floor 1                │ │
│ │ ▶ 📍 Floor 2                │ │
│ └─────────────────────────────┘ │
└─────────────────────────────────┘
```

---

## Out of Scope (Future Enhancement)

### Drag-and-Drop Hierarchy Adjustment

Per miks2u: "ideally we can add drag-and-drop to adjust the hierarchy"

This is a larger feature requiring:
- Drag source/target indicators
- Drop zone validation (prevent circular references)
- Optimistic UI updates
- Mobile touch support (long-press to drag)

**Recommendation:** Create separate ticket TRA-XXX for drag-and-drop after core simplification ships.

---

## Technical Implementation

### Files to Modify

| File | Changes |
|------|---------|
| `LocationDetailsPanel.tsx` | Remove Hierarchy Information section |
| `LocationExpandableCard.tsx` | Remove Type, Children count, Descendants from expanded view |
| `LocationFormModal.tsx` | Remove parent selector dropdown, add context message |
| `LocationsScreen.tsx` | Pass `selectedLocationId` to form modal for parent context |
| `LocationMobileView.tsx` | Update add behavior for mobile |

### Component Changes

**LocationDetailsPanel.tsx:**
```tsx
// REMOVE this entire section:
{/* Hierarchy Information */}
<div className="bg-gray-50 dark:bg-gray-800 rounded-lg p-4">
  <h4>Hierarchy Information</h4>
  <div>Type: {isRoot ? 'Root Location' : 'Subsidiary Location'}</div>
  <div>Parent: {parent?.identifier}</div>
  <div>Direct Children: {children.length}</div>
  <div>Total Descendants: {descendants.length}</div>
</div>

// KEEP children list but simplify label:
<div>
  <h4>Sub-locations ({children.length})</h4>
  {children.map(child => ...)}
</div>
```

**LocationFormModal.tsx:**
```tsx
interface LocationFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  location?: Location;          // For edit mode
  parentLocationId?: number;    // NEW: For create mode context
  onClose: () => void;
}

// In create mode, show context instead of dropdown:
{mode === 'create' && (
  <div className="bg-blue-50 dark:bg-blue-900/20 p-3 rounded-lg">
    {parentLocationId ? (
      <>
        <span className="text-sm text-blue-700 dark:text-blue-300">
          Creating inside:
        </span>
        <span className="font-medium ml-2">
          📍 {parentLocation?.identifier}
        </span>
      </>
    ) : (
      <span className="text-sm text-blue-700 dark:text-blue-300">
        Creating a top-level location
      </span>
    )}
  </div>
)}

// Remove the LocationParentSelector component from create form
```

**LocationsScreen.tsx:**
```tsx
// Pass selected location as parent context
<LocationFormModal
  isOpen={isCreateModalOpen}
  mode="create"
  parentLocationId={selectedLocationId}  // NEW
  onClose={() => setIsCreateModalOpen(false)}
/>
```

---

## Test Plan

### Unit Tests

- [ ] LocationDetailsPanel renders without Hierarchy Information section
- [ ] LocationDetailsPanel shows "Sub-locations" with count
- [ ] LocationExpandableCard renders without Type/Children/Descendants
- [ ] LocationFormModal shows "Creating inside: X" when parent provided
- [ ] LocationFormModal shows "Creating a top-level location" when no parent
- [ ] LocationFormModal does NOT render parent selector dropdown in create mode
- [ ] Create form submits with correct `parent_location_id` from context

### E2E Tests

- [ ] Desktop: Select location → Click Add → Form shows "Creating inside: X"
- [ ] Desktop: Deselect all → Click Add → Form shows "Creating top-level"
- [ ] Desktop: Create child location → Appears under selected parent in tree
- [ ] Desktop: Create root location → Appears at root level in tree
- [ ] Mobile: Expand card → No "Type", "Children count", "Descendants" visible
- [ ] Mobile: Add button creates root when no card expanded
- [ ] Mobile: Add button on expanded card creates child (if we add per-card add button)

### Manual Testing

- [ ] Verify details panel is cleaner without hierarchy info
- [ ] Verify create flow is intuitive with context message
- [ ] Verify no confusion about where new location will appear
- [ ] Dark mode renders correctly for new context message

---

## Acceptance Criteria

- [x] Hierarchy Information section removed from desktop details panel
- [x] Hierarchy Information section removed from mobile expanded cards
- [x] Create form shows parent context based on tree selection
- [x] Create form does NOT show parent dropdown (in create mode)
- [x] Creating with no selection → root-level location (FAB creates root)
- [x] Creating with selection → child of selected location (inline button)
- [x] All existing tests pass (941 tests)
- [x] New unit tests for changed behavior
- [ ] E2E tests for create flow (existing E2E tests cover mobile/accessibility)

---

## Rollout Plan

1. **Phase 1:** Remove Hierarchy Information (low risk, UI cleanup)
2. **Phase 2:** Context-aware Add button (medium risk, behavior change)
3. **Phase 3:** (Future ticket) Drag-and-drop hierarchy adjustment

---

## Open Questions (Resolved)

1. **Should Edit mode keep the parent selector?**
   - ✅ **Decision:** Yes, edit mode keeps the LocationParentSelector dropdown for explicit parent changes.

2. **Mobile: Where does Add button go?**
   - ✅ **Decision:** FAB creates root-level locations. Each expanded card has an inline "Add" button to create children of that location.

3. **What if user wants to create location at different level?**
   - ✅ **Decision:** Select desired parent in tree/card, then use inline "Add Sub-location" button. Or create anywhere and use "Move" to relocate.

---

## References

- Original TRA-301 spec: `spec/active/TRA-301-locations-finder-layout/spec.md`
- Feedback from miks2u: Linear comment on TRA-301 (2026-01-20)
- Feedback from tim.buckley: "Will have to review it with you next meeting"
