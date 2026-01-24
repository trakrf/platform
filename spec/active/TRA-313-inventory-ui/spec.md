# Feature: TRA-313 - Inventory UI: Location Bar and Save Button

## Origin
This specification is derived from Linear issue TRA-313, a sub-issue of TRA-137 (Add tracking save to Inventory screen). This is Phase 2 of TRA-137, following the completion of TRA-312 (Tag classification infrastructure).

## Outcome
The Inventory screen gains a Location Bar showing detected/selected location and a prominent Save button. Location tags are filtered from display, and users see a count breakdown of saveable items.

## User Story
As an inventory operator
I want to see my detected location and save my scan session
So that I can complete the inventory workflow efficiently

## Context

### Discovery
TRA-312 (now in PR review) added the tag classification infrastructure:
- `TagInfo.type` field: `'asset' | 'location' | 'unknown'`
- `locationStore.getLocationByTagEpc()` for O(1) EPC lookup
- Post-login enrichment to classify tags after authentication

This UI layer consumes that classification to:
1. Filter location tags from the table display
2. Auto-detect location from scanned location tags (strongest RSSI wins)
3. Show save controls with proper disabled states

### Current State
- `InventoryHeader.tsx` has toolbar with: Start/Stop, Sample/Reconcile, Clear, Audio, Share buttons
- `InventoryScreen.tsx` displays all tags including location tags (no filtering)
- `InventoryStats.tsx` shows reconciliation stats (found/missing/not listed)
- `tagStore` now has `type` field on tags (TRA-312)
- `locationStore` has `getLocationByTagEpc()` and cache populated on auth

### Desired State
- LocationBar component shows detected location between header and table
- Save button in toolbar (after Share) - green filled, disabled until location resolved
- Location tags hidden from table display
- Count breakdown: "8 scanned, 5 saveable"
- Anonymous users see Save button but get redirected to login on click

## Technical Requirements

### 1. Create LocationBar Component
**File**: `frontend/src/components/inventory/LocationBar.tsx`

```typescript
interface LocationBarProps {
  detectedLocation: { id: number; name: string } | null;
  detectionMethod: 'tag' | 'manual' | null;
  onLocationChange: (locationId: number) => void;
  locations: Location[];  // For dropdown options
  isLoading?: boolean;
}
```

**Layout**:
```
[Pin Icon] Warehouse A - Rack 12        [Change v]
           via location tag (strongest signal)
```

**When no location detected**:
```
[Pin Icon] No location tag detected     [Select v]
```

**Reference patterns**:
- ShareButton for dropdown: `frontend/src/components/ShareButton.tsx`
- Toast: `import toast from 'react-hot-toast'`

### 2. Add Save Button to InventoryHeader
**File**: `frontend/src/components/inventory/InventoryHeader.tsx`

**Position**: After ShareButton (line 97-101 mobile, line 170-174 desktop)

**Styling**:
- Desktop: `px-3 py-2 bg-green-500 hover:bg-green-600 text-white rounded-lg font-medium flex items-center text-sm`
- Mobile: `p-1.5 sm:p-2 bg-green-500 hover:bg-green-600 text-white rounded-lg`
- Disabled: `opacity-50 cursor-not-allowed` when no location resolved
- Icon: `Save` from lucide-react

**Props to add**:
```typescript
interface InventoryHeaderProps {
  // ... existing props
  onSave: () => void;
  isSaveDisabled: boolean;
  saveableCount: number;  // For tooltip: "Save 5 assets"
}
```

### 3. Filter Location Tags from Display
**File**: `frontend/src/components/InventoryScreen.tsx`

Modify `filteredTags` to exclude location tags:

```typescript
const displayableTags = useMemo(() => {
  // Filter out location tags - they're used for detection, not display
  return sortedTags.filter(tag => tag.type !== 'location');
}, [sortedTags]);

const filteredTags = useMemo(() => {
  return displayableTags.filter(tag => {
    // ... existing search and status filter logic
  });
}, [displayableTags, searchTerm, statusFilters]);
```

### 4. Derive Location from Scanned Tags
**File**: `frontend/src/components/InventoryScreen.tsx`

```typescript
const detectedLocation = useMemo(() => {
  const locationTags = tags.filter(t => t.type === 'location');
  if (locationTags.length === 0) return null;

  // Strongest RSSI wins
  const strongest = locationTags.reduce((best, current) =>
    (current.rssi ?? -120) > (best.rssi ?? -120) ? current : best
  );

  return {
    id: strongest.locationId!,
    name: strongest.locationName!,
  };
}, [tags]);
```

### 5. Add Count Breakdown
**File**: `frontend/src/components/inventory/InventoryStats.tsx` OR inline in header

Show saveable count:
```typescript
const saveableCount = tags.filter(t => t.type === 'asset').length;
const totalScanned = tags.filter(t => t.type !== 'location').length;

// Display: "8 scanned, 5 saveable"
```

### 6. Manual Location Selection State
**File**: `frontend/src/components/InventoryScreen.tsx`

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
```

### 7. Anonymous User Flow
When Save clicked and user is not authenticated:
```typescript
const handleSave = useCallback(() => {
  if (!isAuthenticated) {
    // Redirect to login with return URL
    const returnUrl = encodeURIComponent(window.location.pathname);
    window.location.href = `/login?returnUrl=${returnUrl}`;
    return;
  }

  // ... actual save logic (TRA-314)
  toast.success(`${saveableCount} assets saved to ${resolvedLocation.name}`);
}, [isAuthenticated, saveableCount, resolvedLocation]);
```

### 8. Load Locations for Dropdown
Use existing `useLocations` hook:
```typescript
import { useLocations } from '@/hooks/locations';

// In InventoryScreen
const { locations } = useLocations({ enabled: isAuthenticated });
```

## Component Hierarchy

```
InventoryScreen
├── InventoryHeader (+ Save button)
├── LocationBar (NEW)
│   ├── Location display (icon + name + method)
│   └── Location dropdown (Change/Select)
├── InventoryTableContent (receives filtered tags)
└── InventoryStats (+ saveable count)
```

## Validation Criteria

- [ ] LocationBar shows detected location from scanned tags
- [ ] Strongest RSSI wins when multiple location tags scanned
- [ ] Save button visible in toolbar, after Share button
- [ ] Save button disabled when no location resolved
- [ ] Save button enabled once location detected or selected
- [ ] Location tags hidden from inventory table
- [ ] Count breakdown shows saveable vs total scanned
- [ ] Anonymous user clicking Save redirects to `/login?returnUrl=...`
- [ ] Location dropdown populates with org's locations
- [ ] Manual location selection overrides auto-detection

## Dependencies

- **TRA-312** (Blocked by - In Review): Tag classification infrastructure
  - `TagInfo.type` field
  - `locationStore.getLocationByTagEpc()`
  - Post-login location enrichment

## Blocks

- **TRA-314**: Inventory save flow and persistence
  - Depends on this UI being in place

## Out of Scope

- Actual save API call (TRA-314)
- Post-save toast and Clear button animation (TRA-314)
- Inline location creation
- Auto-clear on save preference (TRA-138)

## Implementation Notes

### Styling Consistency
- Save button follows Start button pattern (green filled)
- LocationBar uses same spacing as existing inventory sections
- Dropdown uses Headless UI Menu like ShareButton

### Mobile Considerations
- LocationBar collapses gracefully on small screens
- Save button gets icon-only variant on mobile
- Touch-friendly tap targets (minimum 44px)

### Performance
- Location detection memo'd on `tags` array
- Dropdown options memo'd on `locations` array
- No additional API calls needed (data already in stores)
