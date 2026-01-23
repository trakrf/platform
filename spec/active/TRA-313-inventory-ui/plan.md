# Implementation Plan: TRA-313 - Inventory UI: Location Bar and Save Button

Generated: 2026-01-23
Specification: spec.md (Phase 2 of TRA-137)
Linear Issue: [TRA-313](https://linear.app/trakrf/issue/TRA-313)

## Understanding

Add UI components to the Inventory screen for location detection and save functionality:
1. LocationBar integrated into InventoryHeader showing detected/selected location
2. Save button (green, after Share) - disabled until location resolved
3. Filter location tags from display table
4. Add "saveable" count to InventoryStats
5. Hierarchical location dropdown with indentation
6. Anonymous user redirect to login on Save click

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/ShareButton.tsx` - Headless UI Menu dropdown pattern
- `frontend/src/components/inventory/InventoryHeader.tsx` - Toolbar button styling, mobile/desktop variants
- `frontend/src/components/inventory/InventoryStats.tsx` - Stats card pattern
- `frontend/src/components/locations/LocationTreeView.tsx` (lines 60-69) - Depth-based indentation
- `frontend/src/hooks/locations/useLocations.ts` - Location data fetching pattern

**Files to Create**:
- `frontend/src/components/inventory/LocationBar.tsx` - Location display and dropdown

**Files to Modify**:
- `frontend/src/components/inventory/InventoryHeader.tsx` - Add Save button, integrate LocationBar
- `frontend/src/components/InventoryScreen.tsx` - Filter location tags, derive detected location, location state
- `frontend/src/components/inventory/InventoryStats.tsx` - Add saveable count stat

## Architecture Impact
- **Subsystems affected**: Frontend UI only
- **New dependencies**: None (uses existing Headless UI, lucide-react)
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create LocationBar component
**File**: `frontend/src/components/inventory/LocationBar.tsx`
**Action**: CREATE
**Pattern**: Reference `ShareButton.tsx` for dropdown, `LocationTreeView.tsx` lines 60-69 for indentation

**Implementation**:
```typescript
interface LocationBarProps {
  detectedLocation: { id: number; name: string } | null;
  detectionMethod: 'tag' | 'manual' | null;
  selectedLocationId: number | null;
  onLocationChange: (locationId: number) => void;
  locations: Location[];
  isAuthenticated: boolean;
}

// Use Menu from @headlessui/react
// Pin icon (MapPin from lucide-react)
// Show location name + detection method subtitle
// Dropdown with hierarchical locations using depth-based padding
// Locations sorted by path for proper hierarchy ordering
```

Key details:
- Use `location.depth` field for indentation: `paddingLeft: ${location.depth * 1}rem`
- Sort locations by `path` field to maintain hierarchy order
- Show "via location tag" or "via strongest signal" as subtitle
- When no location: show "No location detected" with "Select" dropdown

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 2: Add Save button to InventoryHeader
**File**: `frontend/src/components/inventory/InventoryHeader.tsx`
**Action**: MODIFY
**Pattern**: Reference existing button styles at lines 121-137 (Start button), lines 170-174 (ShareButton placement)

**Implementation**:
Add props to interface:
```typescript
interface InventoryHeaderProps {
  // ... existing props
  onSave: () => void;
  isSaveDisabled: boolean;
  saveableCount: number;
}
```

Add Save button after ShareButton in both mobile (after line 101) and desktop (after line 174):

Mobile (icon-only):
```typescript
<button
  onClick={onSave}
  disabled={isSaveDisabled}
  className="p-1.5 sm:p-2 bg-green-500 hover:bg-green-600 text-white rounded-lg disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
  title={`Save ${saveableCount} assets`}
>
  <Save className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
</button>
```

Desktop (with label):
```typescript
<button
  onClick={onSave}
  disabled={isSaveDisabled}
  className="px-3 py-2 bg-green-500 hover:bg-green-600 text-white rounded-lg font-medium disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center text-sm"
  title={isSaveDisabled ? 'Select a location first' : `Save ${saveableCount} assets`}
>
  <Save className="w-4 h-4 mr-1.5" />
  Save
</button>
```

Import `Save` from lucide-react.

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 3: Integrate LocationBar into InventoryHeader
**File**: `frontend/src/components/inventory/InventoryHeader.tsx`
**Action**: MODIFY
**Pattern**: Component composition within header

**Implementation**:
Add props for LocationBar data:
```typescript
interface InventoryHeaderProps {
  // ... existing + Task 2 props
  detectedLocation: { id: number; name: string } | null;
  detectionMethod: 'tag' | 'manual' | null;
  selectedLocationId: number | null;
  onLocationChange: (locationId: number) => void;
  locations: Location[];
  isAuthenticated: boolean;
}
```

Add LocationBar below the toolbar row (after line 175 for desktop, after line 104 for mobile):
```typescript
<LocationBar
  detectedLocation={detectedLocation}
  detectionMethod={detectionMethod}
  selectedLocationId={selectedLocationId}
  onLocationChange={onLocationChange}
  locations={locations}
  isAuthenticated={isAuthenticated}
/>
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 4: Filter location tags from display in InventoryScreen
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Existing `filteredTags` memo at lines 61-74

**Implementation**:
Add intermediate memo to filter out location tags before search/status filtering:

```typescript
// Filter out location tags - they're used for detection, not display
const displayableTags = useMemo(() => {
  return sortedTags.filter(tag => tag.type !== 'location');
}, [sortedTags]);

// Update filteredTags to use displayableTags instead of sortedTags
const filteredTags = useMemo(() => {
  return displayableTags.filter(tag => {
    const matchesSearch = !searchTerm ||
      (tag.displayEpc || tag.epc).toLowerCase().includes(searchTerm.toLowerCase());
    // ... rest of existing logic
  });
}, [displayableTags, searchTerm, statusFilters]);
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 5: Derive detected location from scanned tags
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Existing memos for derived state

**Implementation**:
Add memo for location detection:
```typescript
const detectedLocation = useMemo(() => {
  const locationTags = tags.filter(t => t.type === 'location');
  if (locationTags.length === 0) return null;

  // Strongest RSSI wins
  const strongest = locationTags.reduce((best, current) =>
    (current.rssi ?? -120) > (best.rssi ?? -120) ? current : best
  );

  if (!strongest.locationId || !strongest.locationName) return null;

  return {
    id: strongest.locationId,
    name: strongest.locationName,
  };
}, [tags]);

const detectionMethod = useMemo(() => {
  if (!detectedLocation) return null;
  const locationTags = tags.filter(t => t.type === 'location');
  return locationTags.length > 1 ? 'tag' : 'tag'; // Always 'tag' for auto-detected
}, [detectedLocation, tags]);
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 6: Add manual location selection state
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Existing useState patterns at lines 22-27

**Implementation**:
Add state and resolved location:
```typescript
const [manualLocationId, setManualLocationId] = useState<number | null>(null);

// Resolved location = manual override OR detected
const resolvedLocation = useMemo(() => {
  if (manualLocationId) {
    const location = locations.find(l => l.id === manualLocationId);
    return location ? { id: location.id, name: location.name } : null;
  }
  return detectedLocation;
}, [manualLocationId, detectedLocation, locations]);

// Detection method for display
const displayDetectionMethod = useMemo(() => {
  if (manualLocationId) return 'manual';
  return detectionMethod;
}, [manualLocationId, detectionMethod]);
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 7: Add useLocations hook and saveable count
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Existing `useAssets` hook usage at lines 56-57

**Implementation**:
Import and use locations hook:
```typescript
import { useLocations } from '@/hooks/locations';

// Inside component
const { locations } = useLocations({ enabled: isAuthenticated });

// Saveable count memo
const saveableCount = useMemo(() => {
  return tags.filter(t => t.type === 'asset').length;
}, [tags]);
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 8: Add Save handler with anonymous redirect
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Existing handlers like `handleClearInventory` at lines 108-111

**Implementation**:
```typescript
const handleSave = useCallback(() => {
  if (!isAuthenticated) {
    // Redirect to login with return URL
    const returnUrl = encodeURIComponent(window.location.pathname);
    window.location.href = `/login?returnUrl=${returnUrl}`;
    return;
  }

  // Actual save will be implemented in TRA-314
  // For now, just show a placeholder toast
  if (resolvedLocation && saveableCount > 0) {
    toast.success(`Ready to save ${saveableCount} assets to ${resolvedLocation.name}`);
  }
}, [isAuthenticated, resolvedLocation, saveableCount]);
```

Import toast: `import toast from 'react-hot-toast';`

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 9: Wire up InventoryHeader with new props
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Existing InventoryHeader usage at lines 153-170

**Implementation**:
Update InventoryHeader props:
```typescript
<InventoryHeader
  // ... existing props
  onSave={handleSave}
  isSaveDisabled={!resolvedLocation || saveableCount === 0}
  saveableCount={saveableCount}
  detectedLocation={resolvedLocation}
  detectionMethod={displayDetectionMethod}
  selectedLocationId={manualLocationId}
  onLocationChange={setManualLocationId}
  locations={locations}
  isAuthenticated={isAuthenticated}
/>
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 10: Add saveable count to InventoryStats
**File**: `frontend/src/components/inventory/InventoryStats.tsx`
**Action**: MODIFY
**Pattern**: Existing stats cards at lines 19-99

**Implementation**:
Update interface:
```typescript
interface InventoryStatsProps {
  stats: {
    // ... existing
    saveable: number;  // NEW
  };
  // ... existing
}
```

Add new stat card (modify grid to 5 columns on lg):
```typescript
// Change grid-cols-4 to grid-cols-5 on lg
<div className="grid grid-cols-2 lg:grid-cols-5 gap-2 md:gap-3">

// Add new card after Total Scanned:
<div className="bg-purple-50 dark:bg-purple-900/20 border border-purple-200 dark:border-purple-800 rounded-lg p-2 md:p-3 text-left w-full">
  <div className="flex items-center justify-between">
    <div className="w-full">
      <div className="flex items-center mb-0.5 sm:mb-1">
        <Save className="w-3.5 h-3.5 sm:w-4 sm:h-4 lg:w-5 lg:h-5 text-purple-600 mr-1 sm:mr-1.5 md:mr-2 flex-shrink-0" />
        <span className="text-purple-800 dark:text-purple-200 font-semibold text-[10px] xs:text-xs sm:text-sm lg:text-base truncate">Saveable</span>
      </div>
      <div className="text-base sm:text-lg md:text-xl lg:text-2xl font-bold text-purple-800 dark:text-purple-200">{stats.saveable}</div>
      <div className="text-purple-600 dark:text-purple-400 text-[10px] xs:text-xs lg:text-sm truncate">Recognized assets</div>
    </div>
  </div>
</div>
```

Import `Save` from lucide-react.

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 11: Update stats calculation in InventoryScreen
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Existing stats memo at lines 82-96

**Implementation**:
Add saveable to stats:
```typescript
const stats = useMemo(() => {
  const foundTags = filteredTags.filter(tag => tag.reconciled === true).length;
  const missingTags = filteredTags.filter(tag => tag.reconciled === false).length;
  const notListedTags = filteredTags.filter(tag => tag.reconciled === null || tag.reconciled === undefined).length;
  const hasReconciliation = filteredTags.some(tag => tag.reconciled !== null && tag.reconciled !== undefined);

  return {
    total: filteredTags.length,
    totalScanned: filteredTags.length,
    found: foundTags,
    missing: missingTags,
    notListed: notListedTags,
    hasReconciliation,
    saveable: saveableCount,  // NEW: Add saveable count
  };
}, [filteredTags, saveableCount]);
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 12: Add unit test for location detection logic
**File**: `frontend/src/components/__tests__/InventoryScreen.test.tsx`
**Action**: MODIFY
**Pattern**: Existing test patterns

**Implementation**:
Add test cases for:
- Location tags filtered from display
- Strongest RSSI location wins detection
- Manual selection overrides auto-detection
- Save button disabled when no location

**Validation**:
```bash
cd frontend && just test
```

---

## Risk Assessment

- **Risk**: LocationBar dropdown may look cramped on mobile
  **Mitigation**: Test on mobile viewport, use full-width dropdown panel

- **Risk**: Location cache may not be populated when accessing `locations` from hook
  **Mitigation**: Hook handles loading state; LocationBar shows loading indicator

- **Risk**: Tags array changes frequently during scanning
  **Mitigation**: All derived values are memoized to prevent excessive re-renders

## Integration Points
- **Store access**: Uses `useTagStore` (existing), `useLocationStore` (via useLocations hook)
- **Route changes**: None
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd frontend && just lint      # Gate 1: Syntax & Style
cd frontend && just typecheck # Gate 2: Type Safety
cd frontend && just test      # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task:
```bash
cd frontend && just validate
```

Final validation:
```bash
just validate  # Full stack validation
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and clarifying questions
✅ Similar patterns found: ShareButton dropdown, InventoryHeader buttons
✅ Existing LocationTreeView shows indentation pattern
✅ useLocations hook already exists
✅ No new dependencies needed
✅ Single subsystem (frontend UI)

**Assessment**: Straightforward UI extension following established patterns. All building blocks exist.

**Estimated one-pass success probability**: 90%

**Reasoning**: All patterns exist in codebase. Task is primarily UI composition with well-defined props and state flow. Only complexity is ensuring the hierarchical dropdown renders correctly.
