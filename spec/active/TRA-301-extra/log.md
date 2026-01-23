# Build Log: TRA-301 Extra - Simplify Locations UI

## Session: 2026-01-23
Starting task: 1
Total tasks: 10

---

### Task 1: Remove Hierarchy Information from LocationDetailsPanel ✅
- Removed "Hierarchy Information" heading and grid (Type, Direct Children, Total Descendants)
- Renamed "Direct Children:" to "Sub-locations ({count}):"
- Kept `getDescendants` for empty state total count
- Validation: lint ✅, typecheck ✅

### Task 2: Remove Type/Children Grid from LocationExpandableCard ✅
- Removed info grid with Type and Children counts (lines 184-198)
- Kept `descendants` for search filtering
- Validation: typecheck ✅

### Task 3: Add Inline "+" Button to LocationExpandableCard ✅
- Added `onAddChild` prop to interface
- Added Plus icon import
- Added handleAddChild handler
- Added "Add Child" button in action buttons section
- Passed onAddChild to recursive children
- Validation: typecheck ✅

### Task 4: Update LocationMobileView to Pass onAddChild ✅
- Added `onAddChild` prop to interface
- Passed to LocationExpandableCard
- Validation: typecheck ✅

### Task 5: Add parentLocationId Prop to LocationFormModal ✅
- Added `parentLocationId?: number | null` to props
- Passed to LocationForm component
- Validation: typecheck ✅

### Task 6: Update LocationForm for Context-Aware Parent ✅
- Added `parentLocationId` prop to interface
- Added MapPin import and useLocationStore import
- Get parent location info for context message
- Updated create mode initialization to use parentLocationId
- Show context message in create mode ("Creating inside: X" or "Creating a top-level location")
- Keep LocationParentSelector in edit mode
- Validation: typecheck ✅

### Task 7: Update LocationsScreen to Pass selectedLocationId ✅
- Added `createParentId` state for mobile add child
- Get `selectedLocationId` from store
- Added `handleAddChild` callback
- Pass `parentLocationId` to create modal (desktop uses selectedLocationId, mobile uses createParentId)
- Pass `onAddChild` to mobile view
- Reset createParentId on modal close
- Validation: typecheck ✅

### Task 8: Update LocationDetailsPanel Tests ✅
- Updated test "should show hierarchy info with children count" → "should show Sub-locations section with count when children exist"
- Added test "should NOT show Hierarchy Information heading"
- Removed tests for "Root Location" and "Subsidiary Location" types
- Validation: all tests pass ✅

### Task 9: Update LocationExpandableCard Tests ✅
- Added test "should NOT show Type or Children info grid"
- Added test "should render Add Child button when onAddChild provided"
- Added test "should call onAddChild with location.id when Add Child clicked"
- Added test "should NOT render Add Child button when onAddChild not provided"
- Removed tests for "Root Location" and "Subsidiary" types
- Validation: all tests pass ✅

### Task 10: Add Tests for LocationForm Context Message ✅
- Added test "should show 'Creating a top-level location' when no parentLocationId"
- Added test "should show 'Creating inside: {identifier}' when parentLocationId provided"
- Added test "should NOT show LocationParentSelector dropdown in create mode"
- Added test "should show LocationParentSelector in edit mode"
- Validation: all tests pass ✅

---

## Initial Validation
- `just frontend lint` ✅
- `just frontend typecheck` ✅
- `just frontend test` ✅ (938 passing)
- `just frontend build` ✅

---

## Follow-up Session: User Feedback & Corrections

### Issue 1: Desktop Mode FAB Broken ✅
**Problem:** FAB was creating children of selected location instead of root locations.
**Fix:** Changed `parentLocationId={isDesktop ? selectedLocationId : createParentId}` to `parentLocationId={createParentId}` in LocationsScreen.tsx. FAB always creates root; inline "Add Sub-location" button creates children.

### Issue 2: Button Terminology ✅
**Problem:** "Add Child" terminology was unclear.
**Fix:** Renamed to "Add Sub-location" in LocationDetailsPanel and shortened to "Add" in LocationExpandableCard for mobile space constraints.

### Issue 3: Mobile Button Layout ✅
**Problem:** Action buttons didn't fit well on mobile screens.
**Fix:**
- LocationExpandableCard: Changed to 2-column grid layout (`grid grid-cols-2 gap-2`)
- LocationDetailsPanel: Added `flex-wrap`, responsive text (hidden/inline at sm breakpoint), icon-only on small screens

### Files Modified (Follow-up)
- `LocationDetailsPanel.tsx` - Added inline "Add Sub-location" button with responsive layout
- `LocationExpandableCard.tsx` - Changed to 2-column grid, shortened "Add" text
- `LocationDetailsPanel.test.tsx` - Updated button matcher to use regex `/Add/i`
- `LocationExpandableCard.test.tsx` - Updated button matcher to use regex `/Add/i`
- `locations/index.ts` - Removed phase comments from barrel export

---

## Final Validation
- `just frontend lint` ✅ (0 errors, 308 warnings)
- `just frontend typecheck` ✅ (0 errors)
- `just frontend test` ✅ (941 passing)
- `just frontend build` ✅

---

## Implementation Complete 🎉

**Branch:** `feature/TRA-301-extra-simplify-ui`
**PR:** https://github.com/trakrf/platform/pull/136
**Commits:** 16 commits ahead of main

### Summary of Changes

| Component | Change |
|-----------|--------|
| LocationDetailsPanel | Removed Hierarchy Information section; Added "Add Sub-location" button with responsive icons |
| LocationExpandableCard | Removed Type/Children info grid; Added "Add" button; 2-column action grid |
| LocationForm | Added parentLocationId prop; Context message for create mode |
| LocationFormModal | Added parentLocationId prop passthrough |
| LocationSplitPane | Added onAddChild prop |
| LocationMobileView | Added onAddChild prop |
| LocationsScreen | FAB creates root; handleAddChild for inline buttons |

### Behavior Summary

| Action | Result |
|--------|--------|
| Click FAB (floating + button) | Creates root-level location |
| Click "Add Sub-location" in details panel | Creates child of selected location |
| Click "Add" on mobile expanded card | Creates child of that card's location |

