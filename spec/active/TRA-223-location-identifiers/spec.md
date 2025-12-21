# Feature: Frontend Location View with Tag Identifiers (TRA-223)

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-223
**Parent**: TRA-193 (Asset/Location CRUD - separate customer identifier from tag identifiers)
**Depends On**: TRA-214 (Backend transactional identifiers - COMPLETED)
**Reference**: TRA-221 (Asset identifiers - COMPLETED)

## Outcome
Users can view, add, and manage RFID tag identifiers linked to locations, exactly as they can with assets.

## User Story
As a facility manager
I want to attach RFID tags to locations (rooms, zones, shelves)
So that I can scan a tag to identify which location I'm at during inventory

## Context

**Current State**:
- Backend already supports location identifiers via `CreateLocationWithIdentifiers` and `LocationView`
- Frontend `Location` type does NOT include identifiers
- Frontend location components do NOT show/manage identifiers
- Asset components have full identifier support that can be adapted

**Desired State**:
- Frontend `Location` type includes `identifiers: TagIdentifier[]`
- LocationCard shows identifier count badge (like AssetCard)
- LocationDetailsModal shows linked identifiers section
- LocationForm allows adding/removing identifiers during create/edit
- Locate popover works for location tags

**Backend API Reference** (TRA-214 completed):
```go
// backend/internal/models/location/location.go
type LocationView struct {
    Location
    Identifiers []shared.TagIdentifier `json:"identifiers"`
}

type CreateLocationWithIdentifiersRequest struct {
    CreateLocationRequest
    Identifiers []shared.TagIdentifier `json:"identifiers,omitempty"`
}
```

## Reusable Components from Assets

These components were created for TRA-221 and can be directly imported for locations:

| Component | Location | Description |
|-----------|----------|-------------|
| `TagCountBadge` | `components/assets/TagCountBadge.tsx` | Shows "X tags" badge |
| `TagIdentifierList` | `components/assets/TagIdentifierList.tsx` | Lists identifiers with icons/status |
| `TagIdentifiersModal` | `components/assets/TagIdentifiersModal.tsx` | Full modal with delete capability |
| `LocateTagPopover` | `components/assets/LocateTagPopover.tsx` | Popover to select tag for location mode |
| `TagIdentifier` type | `types/shared/identifier.ts` | Shared type definition |

## Technical Requirements

### 1. Type Updates

#### Update: `Location` type (add identifiers)
```typescript
// frontend/src/types/locations/index.ts
import type { TagIdentifier } from '@/types/shared';

export interface Location {
  // ... existing fields ...
  identifiers?: TagIdentifier[]; // NEW: Optional for backwards compatibility
}
```

### 2. LocationCard Changes

#### Location: `frontend/src/components/locations/LocationCard.tsx`

Add identifier count badge and locate button, following AssetCard pattern:

```
ROW VARIANT:
+------+------------+--------+-------------+---------+--------+----------+
| Type | Identifier | Name   | Description | [2 tags]| Status | Actions  |
+------+------------+--------+-------------+---------+--------+----------+
| Root | ZONE-A     | Zone A | Main floor  | 🏷️ 2   | Active | [📍][✏️][🗑️] |
+------+------------+--------+-------------+---------+--------+----------+

CARD VARIANT:
+-----------------------------------------------+
| [Building]  ZONE-A             [2 tags]       |
| Zone A                                        |
| Main floor warehouse                          |
| [Active ✓]                                    |
+-----------------------------------------------+
| [Locate] [Edit] [Delete]                      |
+-----------------------------------------------+
```

**Requirements**:
1. Import and use `TagCountBadge` component
2. Import and use `LocateTagPopover` component
3. Import and use `TagIdentifiersModal` component
4. Track `localIdentifiers` state (sync with location.identifiers)
5. Add `onIdentifierRemoved` callback support

### 3. LocationDetailsModal Changes

#### Location: `frontend/src/components/locations/LocationDetailsModal.tsx`

Add "Tag Identifiers" section after Status, before Hierarchy Information:

```
+---------------------------------------------------+
| Location Details                                   |
+---------------------------------------------------+
| Breadcrumb: HQ > Building A > Zone-1              |
|---------------------------------------------------|
| Identifier: ZONE-1    |  Name: Storage Zone 1    |
| Description: North side of building A              |
| Status: [Active ✓]                                 |
+---------------------------------------------------+
| Tag Identifiers                            [?]    |
| +-----------------------------------------------+ |
| | [RFID] E20000000000001234  [Active]  [Locate]| |
| | [RFID] E20000000000005678  [Active]  [Locate]| |
| +-----------------------------------------------+ |
| -- OR if empty --                                  |
| No tag identifiers linked                          |
+---------------------------------------------------+
| Hierarchy Information                              |
| Type: Subsidiary  |  Direct Children: 3           |
| Total Descendants: 12                              |
+---------------------------------------------------+
```

**Requirements**:
1. Import and use `TagIdentifierList` component with `showHeader={true}`
2. Show section between Status and Hierarchy Information
3. Handle empty state gracefully

### 4. LocationForm Changes

#### Location: `frontend/src/components/locations/LocationForm.tsx`

Add tag identifiers management section, following AssetForm pattern:

```
+---------------------------------------------------+
| Identifier* [_________] [Scan RFID] [Scan Barcode]|
| Name*       [_________________________]           |
| [x] Active                                        |
+---------------------------------------------------+
| Parent Location [Dropdown________________v]       |
+---------------------------------------------------+
| Description [______________________________]      |
|             [______________________________]      |
+---------------------------------------------------+
| Tag Identifiers                            [?]    |
| Add tag: [______________] [+ Add]                 |
| +-----------------------------------------------+ |
| | [RFID] E20000000000001234  [Active]  [Remove]| |
| +-----------------------------------------------+ |
| -- OR if no tags --                               |
| No tag identifiers added                          |
+---------------------------------------------------+
| Valid From [__/__/____]  Valid To [__/__/____]   |
+---------------------------------------------------+
|                          [Cancel] [Create/Update] |
+---------------------------------------------------+
```

**Requirements**:
1. Add `identifiers` to `LocationFormData` interface
2. Create tag input with validation (EPC format)
3. Allow adding new identifiers
4. Display existing identifiers with remove capability
5. Pass identifiers in form submission
6. Sync with scan-to-input for identifier field

### 5. LocationStore Updates

#### Location: `frontend/src/stores/locations/locationStore.ts`

Add method to update cached location identifiers:

```typescript
updateCachedLocation: (id: number, updates: Partial<Location>) => void;
```

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `frontend/src/types/locations/index.ts` | Modify | Add identifiers to Location type |
| `frontend/src/components/locations/LocationCard.tsx` | Modify | Add TagCountBadge, LocateTagPopover, TagIdentifiersModal |
| `frontend/src/components/locations/LocationDetailsModal.tsx` | Modify | Add TagIdentifierList section |
| `frontend/src/components/locations/LocationForm.tsx` | Modify | Add tag identifiers management |
| `frontend/src/stores/locations/locationStore.ts` | Modify | Add updateCachedLocation method |
| `frontend/src/components/assets/index.ts` | Modify | Export shared identifier components |

## Component Wireframes

### LocationCard Row Variant
```
┌────────────────────────────────────────────────────────────────────────────┐
│ [🏢] Root    │ ZONE-A │ Zone A │ Main floor... │ 🏷️2 │ Active │ [📍][✏️][🗑️] │
└────────────────────────────────────────────────────────────────────────────┘
```

### LocationCard Card Variant
```
┌─────────────────────────────────────┐
│ [🏢] ZONE-A              [🏷️ 2 tags] │
│ Zone A                              │
│ Main floor warehouse                │
│ ┌─────────────┐                     │
│ │ Active ✓    │                     │
│ └─────────────┘                     │
├─────────────────────────────────────┤
│ [📍 Locate] [✏️ Edit] [🗑️ Delete]    │
└─────────────────────────────────────┘
```

### TagIdentifiersModal (reused from assets)
```
┌─────────────────────────────────────┐
│ Tag Identifiers for ZONE-A      [X] │
├─────────────────────────────────────┤
│ [RFID] E200000001234                │
│        [Active]         [🗑️ Remove] │
│ ─────────────────────────────────── │
│ [RFID] E200000005678                │
│        [Active]         [🗑️ Remove] │
├─────────────────────────────────────┤
│                            [Close]  │
└─────────────────────────────────────┘
```

### LocationForm Tag Section
```
┌─────────────────────────────────────┐
│ Tag Identifiers               [?]   │
│                                     │
│ Add tag: [________________] [+ Add] │
│                                     │
│ ┌─────────────────────────────────┐ │
│ │ [📻] RFID  E200001234  [Active] │ │
│ │                        [Remove] │ │
│ └─────────────────────────────────┘ │
│ ┌─────────────────────────────────┐ │
│ │ [📻] RFID  E200005678  [Active] │ │
│ │                        [Remove] │ │
│ └─────────────────────────────────┘ │
└─────────────────────────────────────┘
```

## Validation Criteria

- [ ] Location type includes `identifiers?: TagIdentifier[]`
- [ ] LocationCard shows tag count badge when identifiers exist
- [ ] LocationCard has locate button for locations with active identifiers
- [ ] LocationDetailsModal shows tag identifiers section
- [ ] LocationDetailsModal shows empty state when no identifiers
- [ ] LocationForm allows adding new tag identifiers
- [ ] LocationForm allows removing existing identifiers
- [ ] LocationForm submits identifiers with location data
- [ ] Clicking tag count badge opens TagIdentifiersModal
- [ ] LocateTagPopover works for selecting location tags
- [ ] No regression in existing location functionality

## Success Metrics

- [ ] Create location with 2 identifiers - all saved and displayed
- [ ] View location with identifiers - badge shows count, details show list
- [ ] Edit location - can add/remove identifiers
- [ ] Delete identifier from modal - updates UI immediately
- [ ] Locate button opens popover with correct tags
- [ ] No TypeScript errors
- [ ] All existing tests still pass

## Out of Scope (Future Work)

1. **Bulk identifier import** - CSV upload for location tags
2. **BLE/Barcode types** - Only RFID for this version
3. **Identifier validation rules** - Custom regex per org
4. **API integration** - Backend already supports this, frontend-only ticket

## Implementation Notes

### Code Patterns from AssetCard

```typescript
// State management pattern
const [tagsModalOpen, setTagsModalOpen] = useState(false);
const [localIdentifiers, setLocalIdentifiers] = useState<TagIdentifier[]>(
  location.identifiers || []
);

// Sync with prop changes
useEffect(() => {
  setLocalIdentifiers(location.identifiers || []);
}, [location.identifiers]);

// Handler for removed identifiers
const handleIdentifierRemoved = (identifierId: number) => {
  const updated = localIdentifiers.filter((i) => i.id !== identifierId);
  setLocalIdentifiers(updated);
  updateCachedLocation(location.id, { ...location, identifiers: updated });
};
```

### Component Imports

```typescript
// In LocationCard.tsx
import { TagCountBadge } from '@/components/assets/TagCountBadge';
import { TagIdentifiersModal } from '@/components/assets/TagIdentifiersModal';
import { LocateTagPopover } from '@/components/assets/LocateTagPopover';
import type { TagIdentifier } from '@/types/shared';
```

## References

- [TRA-221 Asset Identifiers Spec](../TRA-221-frontend-asset-identifiers/spec.md) - Asset implementation
- [TRA-214 Backend Spec](../TRA-214-transactional-identifiers/spec.md) - Backend implementation
- [AssetCard.tsx](frontend/src/components/assets/AssetCard.tsx) - Reference implementation
- [TagIdentifierList.tsx](frontend/src/components/assets/TagIdentifierList.tsx) - Reusable component
- [Backend CreateLocationWithIdentifiers](backend/internal/handlers/locations/locations.go) - API endpoint
