# Implementation Plan: Frontend Location View with Tag Identifiers (TRA-223)
Generated: 2025-12-21
Specification: spec.md

## Understanding

Add RFID tag identifier support to location components by reusing existing asset components. The backend already supports location identifiers via `CreateLocationWithIdentifiers` and identifier endpoints. This is a frontend-only feature that follows patterns established in TRA-221 (asset identifiers).

Key changes:
1. Make shared tag components generic (entityId vs assetId)
2. Add identifier API methods to locationsApi
3. Update Location type with identifiers
4. Integrate tag components into LocationCard, LocationDetailsModal, and LocationForm

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/assets/AssetCard.tsx` (lines 42-68) - identifier state management pattern
- `frontend/src/components/assets/AssetForm.tsx` (lines 353-401) - tag identifiers form section
- `frontend/src/lib/api/assets/index.ts` (lines 99-110) - identifier API methods

**Files to Create**:
- None - all components exist, just need modification

**Files to Modify**:
| File | Change |
|------|--------|
| `frontend/src/components/assets/index.ts` | Export shared tag components |
| `frontend/src/lib/api/locations/index.ts` | Add addIdentifier, removeIdentifier methods |
| `frontend/src/types/locations/index.ts` | Add identifiers to Location type |
| `frontend/src/components/assets/TagIdentifiersModal.tsx` | Make generic (entityId/entityName) |
| `frontend/src/components/assets/LocateTagPopover.tsx` | Make generic (entityIdentifier) |
| `frontend/src/stores/locations/locationStore.ts` | Add updateCachedLocation method |
| `frontend/src/components/locations/LocationCard.tsx` | Add tag badge, modal, popover |
| `frontend/src/components/locations/LocationDetailsModal.tsx` | Add TagIdentifierList section |
| `frontend/src/components/locations/LocationForm.tsx` | Add tag identifiers management |

## Architecture Impact
- **Subsystems affected**: Frontend only (UI components, API layer, store)
- **New dependencies**: None
- **Breaking changes**: TagIdentifiersModal and LocateTagPopover props renamed (but AssetCard will be updated to match)

## Task Breakdown

### Task 1: Export Shared Tag Components
**File**: `frontend/src/components/assets/index.ts`
**Action**: MODIFY

**Implementation**:
Add exports for reusable tag components:
```typescript
export { TagCountBadge } from './TagCountBadge';
export { TagIdentifierList } from './TagIdentifierList';
export { TagIdentifiersModal } from './TagIdentifiersModal';
export { TagIdentifierInputRow } from './TagIdentifierInputRow';
// LocateTagPopover already exported
```

**Validation**:
```bash
just frontend typecheck
```

---

### Task 2: Add Identifier Methods to Locations API
**File**: `frontend/src/lib/api/locations/index.ts`
**Action**: MODIFY
**Pattern**: Reference `frontend/src/lib/api/assets/index.ts` lines 99-110

**Implementation**:
Add two methods matching assetsApi pattern:
```typescript
/**
 * Add a tag identifier to a location
 * POST /api/v1/locations/:locationId/identifiers
 */
addIdentifier: (locationId: number, identifier: { type: string; value: string }) =>
  apiClient.post<{ data: { id: number; type: string; value: string; is_active: boolean } }>(
    `/locations/${locationId}/identifiers`,
    identifier
  ),

/**
 * Remove a tag identifier from a location
 * DELETE /api/v1/locations/:locationId/identifiers/:identifierId
 */
removeIdentifier: (locationId: number, identifierId: number) =>
  apiClient.delete<DeleteResponse>(`/locations/${locationId}/identifiers/${identifierId}`),
```

**Validation**:
```bash
just frontend typecheck
```

---

### Task 3: Update Location Type with Identifiers
**File**: `frontend/src/types/locations/index.ts`
**Action**: MODIFY

**Implementation**:
1. Add import for TagIdentifier:
```typescript
import type { TagIdentifier } from '@/types/shared';
```

2. Add identifiers field to Location interface (after line 27):
```typescript
export interface Location {
  // ... existing fields ...
  identifiers?: TagIdentifier[]; // Tag identifiers linked to this location
}
```

3. Add TagIdentifierInput type for forms:
```typescript
export interface TagIdentifierInput {
  id?: number;
  type: 'rfid';
  value: string;
}
```

**Validation**:
```bash
just frontend typecheck
```

---

### Task 4: Make TagIdentifiersModal Generic
**File**: `frontend/src/components/assets/TagIdentifiersModal.tsx`
**Action**: MODIFY

**Implementation**:
1. Change props interface:
```typescript
interface TagIdentifiersModalProps {
  identifiers: TagIdentifier[];
  entityId?: number;      // Renamed from assetId
  entityName?: string;    // Renamed from assetName
  entityType?: 'asset' | 'location';  // New: determines which API to call
  isOpen: boolean;
  onClose: () => void;
  onIdentifierRemoved?: (identifierId: number) => void;
}
```

2. Update handleConfirmRemove to use entityType:
```typescript
import { assetsApi } from '@/lib/api/assets';
import { locationsApi } from '@/lib/api/locations';

const handleConfirmRemove = async () => {
  if (!entityId || !confirmingId) return;
  setRemovingId(confirmingId);
  try {
    if (entityType === 'location') {
      await locationsApi.removeIdentifier(entityId, confirmingId);
    } else {
      await assetsApi.removeIdentifier(entityId, confirmingId);
    }
    toast.success('Tag identifier removed');
    onIdentifierRemoved?.(confirmingId);
  } catch (err) {
    toast.error('Failed to remove tag identifier');
  } finally {
    setRemovingId(null);
    setConfirmingId(null);
  }
};
```

3. Update empty state message:
```typescript
<p>No tag identifiers linked to this {entityType || 'asset'}.</p>
```

4. Update AssetCard.tsx to use new prop names:
```typescript
<TagIdentifiersModal
  identifiers={localIdentifiers}
  entityId={asset.id}           // was: assetId
  entityName={asset.identifier} // was: assetName
  entityType="asset"            // NEW
  isOpen={tagsModalOpen}
  onClose={() => setTagsModalOpen(false)}
  onIdentifierRemoved={handleIdentifierRemoved}
/>
```

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 5: Make LocateTagPopover Generic
**File**: `frontend/src/components/assets/LocateTagPopover.tsx`
**Action**: MODIFY

**Implementation**:
1. Rename prop in interface:
```typescript
interface LocateTagPopoverProps {
  identifiers: TagIdentifier[];
  entityIdentifier: string;  // Renamed from assetIdentifier
  isActive: boolean;
  triggerClassName?: string;
  variant?: 'icon' | 'button';
}
```

2. Update aria-label:
```typescript
aria-label={`Locate ${entityIdentifier}`}
```

3. Update AssetCard.tsx to use new prop name:
```typescript
<LocateTagPopover
  identifiers={localIdentifiers}
  entityIdentifier={asset.identifier}  // was: assetIdentifier
  isActive={asset.is_active}
  variant="icon"
/>
```

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 6: Add updateCachedLocation to Location Store
**File**: `frontend/src/stores/locations/locationStore.ts`
**Action**: MODIFY

**Implementation**:
1. Add method to interface (around line 24):
```typescript
updateCachedLocation: (id: number, updates: Partial<Location>) => void;
```

2. Add implementation in createCacheActions or directly in store:
The updateLocation method already exists - verify it updates cache properly.
If not, add:
```typescript
updateCachedLocation: (id, updates) => {
  const current = get().cache.byId.get(id);
  if (current) {
    const updated = { ...current, ...updates };
    set((state) => {
      const newById = new Map(state.cache.byId);
      newById.set(id, updated);
      return { cache: { ...state.cache, byId: newById } };
    });
  }
},
```

**Validation**:
```bash
just frontend typecheck
```

---

### Task 7: Update LocationCard with Identifier Support
**File**: `frontend/src/components/locations/LocationCard.tsx`
**Action**: MODIFY
**Pattern**: Reference `frontend/src/components/assets/AssetCard.tsx` lines 42-68

**Implementation**:
1. Add imports:
```typescript
import { useState, useEffect } from 'react';
import { TagCountBadge } from '@/components/assets/TagCountBadge';
import { TagIdentifiersModal } from '@/components/assets/TagIdentifiersModal';
import { LocateTagPopover } from '@/components/assets/LocateTagPopover';
import type { TagIdentifier } from '@/types/shared';
import { useLocationStore } from '@/stores/locations/locationStore';
```

2. Add state management (after line 24):
```typescript
const [tagsModalOpen, setTagsModalOpen] = useState(false);
const [localIdentifiers, setLocalIdentifiers] = useState<TagIdentifier[]>(
  location.identifiers || []
);
const updateCachedLocation = useLocationStore((state) => state.updateCachedLocation);

useEffect(() => {
  setLocalIdentifiers(location.identifiers || []);
}, [location.identifiers]);

const handleOpenTagsModal = (e: React.MouseEvent) => {
  e.stopPropagation();
  if (localIdentifiers.length > 0) {
    setTagsModalOpen(true);
  }
};

const handleIdentifierRemoved = (identifierId: number) => {
  const updated = localIdentifiers.filter((i) => i.id !== identifierId);
  setLocalIdentifiers(updated);
  updateCachedLocation(location.id, { ...location, identifiers: updated });
};

const hasIdentifiers = localIdentifiers.length > 0;
```

3. In ROW variant, add Tags column (after Description, before Status):
```typescript
{/* Tags */}
<td className="px-4 py-3">
  <TagCountBadge
    identifiers={localIdentifiers}
    onClick={localIdentifiers.length ? handleOpenTagsModal : undefined}
  />
</td>
```

4. In ROW variant actions, add locate button:
```typescript
{hasIdentifiers && (
  <LocateTagPopover
    identifiers={localIdentifiers}
    entityIdentifier={location.identifier}
    isActive={location.is_active}
    variant="icon"
  />
)}
```

5. In CARD variant, add badge after identifier (line ~134):
```typescript
<h3 className="...">
  {location.identifier}
</h3>
{localIdentifiers.length > 0 && (
  <TagCountBadge
    identifiers={localIdentifiers}
    onClick={handleOpenTagsModal}
  />
)}
```

6. In CARD variant actions, add Locate button:
```typescript
{hasIdentifiers && (
  <LocateTagPopover
    identifiers={localIdentifiers}
    entityIdentifier={location.identifier}
    isActive={location.is_active}
    variant="button"
  />
)}
```

7. Add modal at end of both variants (use Fragment wrapper):
```typescript
<TagIdentifiersModal
  identifiers={localIdentifiers}
  entityId={location.id}
  entityName={location.identifier}
  entityType="location"
  isOpen={tagsModalOpen}
  onClose={() => setTagsModalOpen(false)}
  onIdentifierRemoved={handleIdentifierRemoved}
/>
```

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 8: Update LocationDetailsModal with TagIdentifierList
**File**: `frontend/src/components/locations/LocationDetailsModal.tsx`
**Action**: MODIFY

**Implementation**:
1. Add import:
```typescript
import { TagIdentifierList } from '@/components/assets/TagIdentifierList';
```

2. Add section after Status (around line 116), before Hierarchy Information:
```typescript
{/* Tag Identifiers Section */}
<div className="border-t border-gray-200 dark:border-gray-700 pt-4">
  <TagIdentifierList
    identifiers={location.identifiers || []}
    showHeader={true}
    expanded={true}
    size="md"
  />
</div>
```

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 9: Update LocationForm with Tag Identifiers Management
**File**: `frontend/src/components/locations/LocationForm.tsx`
**Action**: MODIFY
**Pattern**: Reference `frontend/src/components/assets/AssetForm.tsx` lines 353-401

**Implementation**:
1. Add imports:
```typescript
import { Plus, HelpCircle } from 'lucide-react';
import { TagIdentifierInputRow } from '@/components/assets/TagIdentifierInputRow';
import type { TagIdentifierInput } from '@/types/locations';
```

2. Update LocationFormData interface:
```typescript
interface LocationFormData {
  // ... existing fields ...
  identifiers: TagIdentifierInput[];  // NEW
}
```

3. Add initial state for identifiers:
```typescript
const [formData, setFormData] = useState<LocationFormData>({
  // ... existing ...
  identifiers: [],
});
```

4. In edit mode useEffect, initialize identifiers:
```typescript
if (mode === 'edit' && location) {
  setFormData({
    // ... existing ...
    identifiers: (location.identifiers || []).map((id) => ({
      id: id.id,
      type: 'rfid' as const,
      value: id.value,
    })),
  });
}
```

5. Add Tag Identifiers section (after Description, before Valid From):
```typescript
{/* Tag Identifiers Section */}
<div className="border-t border-gray-200 dark:border-gray-700 pt-6">
  <div className="flex items-center justify-between mb-4">
    <div className="flex items-center gap-2">
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
        Tag Identifiers
      </label>
      <div className="group relative">
        <HelpCircle className="w-4 h-4 text-gray-400 cursor-help" />
        <div className="absolute left-0 bottom-full mb-2 hidden group-hover:block w-64 p-2 bg-gray-900 text-white text-xs rounded-lg shadow-lg z-10">
          RFID tags physically attached to this location for scanning.
        </div>
      </div>
    </div>
    <button
      type="button"
      onClick={() =>
        handleChange('identifiers', [...formData.identifiers, { type: 'rfid', value: '' }])
      }
      disabled={loading}
      className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded-lg transition-colors disabled:opacity-50"
    >
      <Plus className="w-4 h-4" />
      Add Tag
    </button>
  </div>

  {formData.identifiers.length === 0 ? (
    <p className="text-sm text-gray-500 dark:text-gray-400 italic">
      No tag identifiers added. Click "Add Tag" to link RFID tags.
    </p>
  ) : (
    <div className="space-y-3">
      {formData.identifiers.map((identifier, index) => (
        <TagIdentifierInputRow
          key={identifier.id ?? `new-${index}`}
          type={identifier.type}
          value={identifier.value}
          onTypeChange={(type) => {
            const updated = [...formData.identifiers];
            updated[index] = { ...updated[index], type };
            handleChange('identifiers', updated);
          }}
          onValueChange={(value) => {
            const updated = [...formData.identifiers];
            updated[index] = { ...updated[index], value };
            handleChange('identifiers', updated);
          }}
          onRemove={() => {
            handleChange('identifiers', formData.identifiers.filter((_, i) => i !== index));
          }}
          disabled={loading}
        />
      ))}
    </div>
  )}
</div>
```

6. Update handleSubmit to include identifiers:
```typescript
const validIdentifiers = formData.identifiers.filter((id) => id.value.trim() !== '');

const submitData = {
  ...formData,
  // ... existing transformations ...
  identifiers: validIdentifiers,
};

onSubmit(submitData);
```

7. Add scan-to-input for tags (optional enhancement):
```typescript
// Create second scanner for tag input
const { startRfidScan: startTagScan, isScanning: isTagScanning } = useScanToInput({
  onScan: (value) => {
    handleChange('identifiers', [...formData.identifiers, { type: 'rfid', value }]);
  },
  autoStop: true,
});
```

**Validation**:
```bash
just frontend typecheck
just frontend lint
just frontend test
```

---

### Task 10: E2E Testing with Playwright MCP
**Action**: MANUAL TESTING via Playwright MCP
**Credentials**: test1@test.com / password

**Test Scenarios**:

1. **Login and Navigate to Locations**:
   - Navigate to http://localhost:5173
   - Login with test1@test.com / password
   - Navigate to Locations page

2. **Create Location with Tag Identifiers**:
   - Click "Create Location" button
   - Fill in: identifier, name, description
   - Add 2 tag identifiers using "Add Tag" button
   - Submit form
   - Verify location appears in list with tag count badge

3. **View Location Details with Identifiers**:
   - Click on a location with identifiers
   - Verify Tag Identifiers section appears in modal
   - Verify identifiers are displayed with type, value, status

4. **Test Tag Count Badge**:
   - In location list, verify tag count badge shows correct count
   - Click badge to open TagIdentifiersModal
   - Verify modal shows all identifiers

5. **Test Locate Popover**:
   - For location with active identifiers, click Locate button
   - Verify popover shows list of active tags
   - Click a tag to trigger locate mode

6. **Edit Location - Add/Remove Identifiers**:
   - Edit existing location
   - Add new identifier
   - Remove existing identifier
   - Save and verify changes persist

7. **Remove Identifier from Modal**:
   - Open TagIdentifiersModal for a location
   - Click remove on an identifier
   - Confirm removal
   - Verify identifier is removed from list

**Playwright MCP Commands**:
```
# Login flow
playwright_navigate: http://localhost:5173
playwright_fill: [email field] test1@test.com
playwright_fill: [password field] password
playwright_click: [login button]

# Navigate to locations
playwright_click: [locations nav item]

# Create location with identifiers
playwright_click: [create location button]
playwright_fill: [identifier input] TEST-LOC-001
playwright_fill: [name input] Test Location
playwright_click: [Add Tag button]
playwright_fill: [tag value input] E200000001234567
playwright_click: [submit button]

# Verify in list
playwright_screenshot: verify location in list with tag badge
```

**Validation**:
- All test scenarios pass visually
- No console errors during interactions
- Tag identifiers persist after page refresh

---

## Risk Assessment

- **Risk**: TagIdentifiersModal prop rename breaks AssetCard
  **Mitigation**: Update AssetCard in same task (Task 4)

- **Risk**: Location type change causes type errors in existing code
  **Mitigation**: identifiers is optional (`identifiers?:`), backward compatible

- **Risk**: locationsApi identifier methods don't match backend
  **Mitigation**: Backend routes verified in handlers/locations/locations.go lines 532-533

## Integration Points

- **Store updates**: LocationStore needs updateCachedLocation method
- **API layer**: locationsApi needs addIdentifier/removeIdentifier methods
- **Component exports**: assets/index.ts exports shared tag components

## VALIDATION GATES (MANDATORY)

After EVERY code change, run from `frontend/` directory:
```bash
just lint      # Gate 1: Syntax & Style
just typecheck # Gate 2: Type Safety
just test      # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task: Run lint, typecheck from frontend/

Final validation:
```bash
cd frontend && just validate
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- Clear requirements from spec
- Similar patterns found in AssetCard.tsx, AssetForm.tsx
- All clarifying questions answered
- Existing test patterns to follow
- Backend API endpoints verified
- All components already exist, just need integration

**Assessment**: Straightforward feature reusing existing patterns. Main work is prop renaming and component integration.

**Estimated one-pass success probability**: 90%

**Reasoning**: High confidence because we're directly following proven AssetCard patterns. Only risks are prop rename coordination (handled in same task) and potential edge cases in form state management.
