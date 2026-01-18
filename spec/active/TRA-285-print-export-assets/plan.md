# Implementation Plan: Print and Export Asset List (TRA-285)
Generated: 2025-01-18
Specification: spec.md

## Understanding

Add export functionality (CSV, XLSX, PDF) to the Assets screen by creating a reusable export system. The implementation adapts the existing inventory export pattern (`ShareButton` + `ShareModal`) into a generic `ExportModal` that can be reused across screens. Location names will be resolved from the location store cache for better UX.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/ShareModal.tsx` (lines 15-294) - Modal structure, share/download logic, Web Share API handling
- `frontend/src/components/ShareButton.tsx` (lines 17-71) - Already reusable, no changes needed
- `frontend/src/utils/excelExportUtils.ts` (lines 13-135) - Excel generation pattern with xlsx
- `frontend/src/utils/pdfExportUtils.ts` (lines 14-129) - PDF generation with jsPDF + autoTable
- `frontend/src/utils/shareUtils.ts` - `shareFile`, `downloadBlob`, `canShareFormat` utilities
- `frontend/src/components/__tests__/ShareModal.test.tsx` - Test pattern for export modal

**Files to Create**:
- `frontend/src/hooks/useExport.ts` - Generic export state hook
- `frontend/src/hooks/useExport.test.ts` - Hook unit tests
- `frontend/src/components/export/ExportModal.tsx` - Generic export modal component
- `frontend/src/components/export/ExportModal.test.tsx` - Modal component tests
- `frontend/src/components/export/index.ts` - Barrel export
- `frontend/src/utils/export/assetExport.ts` - Asset PDF/Excel/CSV generators
- `frontend/src/utils/export/assetExport.test.ts` - Export generator tests
- `frontend/src/utils/export/index.ts` - Barrel export

**Files to Modify**:
- `frontend/src/components/AssetsScreen.tsx` - Add ShareButton and ExportModal integration

## Architecture Impact
- **Subsystems affected**: Frontend UI only
- **New dependencies**: None (xlsx, jspdf, jspdf-autotable already installed)
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create useExport Hook
**File**: `frontend/src/hooks/useExport.ts`
**Action**: CREATE
**Pattern**: Simple state hook pattern

**Implementation**:
```typescript
import { useState, useCallback } from 'react';
import type { ExportFormat } from '@/types/export';

export function useExport(defaultFormat: ExportFormat = 'csv') {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [selectedFormat, setSelectedFormat] = useState<ExportFormat>(defaultFormat);

  const openExport = useCallback((format: ExportFormat) => {
    setSelectedFormat(format);
    setIsModalOpen(true);
  }, []);

  const closeExport = useCallback(() => {
    setIsModalOpen(false);
  }, []);

  return { isModalOpen, selectedFormat, openExport, closeExport };
}
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 2: Create useExport Hook Tests
**File**: `frontend/src/hooks/useExport.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/hooks/assets/useAsset.test.ts`

**Implementation**:
```typescript
import { renderHook, act } from '@testing-library/react';
import { useExport } from './useExport';

describe('useExport', () => {
  it('initializes with default format csv', () => {
    const { result } = renderHook(() => useExport());
    expect(result.current.selectedFormat).toBe('csv');
    expect(result.current.isModalOpen).toBe(false);
  });

  it('opens modal with selected format', () => {
    const { result } = renderHook(() => useExport());
    act(() => result.current.openExport('pdf'));
    expect(result.current.isModalOpen).toBe(true);
    expect(result.current.selectedFormat).toBe('pdf');
  });

  it('closes modal', () => {
    const { result } = renderHook(() => useExport());
    act(() => result.current.openExport('xlsx'));
    act(() => result.current.closeExport());
    expect(result.current.isModalOpen).toBe(false);
  });
});
```

**Validation**:
```bash
cd frontend && just test -- useExport
```

---

### Task 3: Create Asset Export Generators
**File**: `frontend/src/utils/export/assetExport.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/utils/excelExportUtils.ts` and `frontend/src/utils/pdfExportUtils.ts`

**Key Implementation Details**:
- Import `useLocationStore` to resolve location names from `current_location_id`
- Create helper function `getLocationName(id: number | null): string`
- Export functions: `generateAssetPDF`, `generateAssetExcel`, `generateAssetCSV`
- Columns: Asset ID, Name, Type, Tag ID(s), Location, Status, Description, Created

**Location Resolution Pattern**:
```typescript
import { useLocationStore } from '@/stores/locations/locationStore';

function getLocationName(locationId: number | null): string {
  if (!locationId) return '';
  const location = useLocationStore.getState().cache.byId.get(locationId);
  return location?.name || '';
}
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 4: Create Asset Export Tests
**File**: `frontend/src/utils/export/assetExport.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/utils/__tests__/exportUtils.test.ts`

**Test Cases**:
1. `generateAssetCSV` - returns blob with correct headers and data
2. `generateAssetExcel` - returns blob with correct MIME type
3. `generateAssetPDF` - returns blob with correct MIME type
4. Empty array handling - returns valid empty export
5. Location resolution - correctly resolves location names

**Validation**:
```bash
cd frontend && just test -- assetExport
```

---

### Task 5: Create Export Barrel Files
**Files**:
- `frontend/src/utils/export/index.ts`
- `frontend/src/components/export/index.ts`
**Action**: CREATE

**Implementation** (`utils/export/index.ts`):
```typescript
export { generateAssetPDF, generateAssetExcel, generateAssetCSV } from './assetExport';
```

**Implementation** (`components/export/index.ts`):
```typescript
export { ExportModal } from './ExportModal';
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 6: Create Generic ExportModal Component
**File**: `frontend/src/components/export/ExportModal.tsx`
**Action**: CREATE
**Pattern**: Adapt `frontend/src/components/ShareModal.tsx` (lines 15-294)

**Props Interface**:
```typescript
interface ExportModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedFormat: ExportFormat;
  itemCount: number;
  title: string;
  generateFile: (format: ExportFormat) => ExportResult;
  statsFooter?: React.ReactNode;  // Optional custom stats
}
```

**Key Differences from ShareModal**:
- Remove `tags: TagInfo[]` and `reconciliationList` props
- Add `generateFile` callback prop instead
- Add `title` prop for modal header
- Add optional `statsFooter` prop for custom stats per screen
- Keep all Web Share API logic, loading states, toast notifications

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 7: Create ExportModal Tests
**File**: `frontend/src/components/export/ExportModal.test.tsx`
**Action**: CREATE
**Pattern**: Reference `frontend/src/components/__tests__/ShareModal.test.tsx`

**Test Cases**:
1. Renders when isOpen=true
2. Does not render when isOpen=false
3. Shows correct title and item count
4. Calls generateFile on download click
5. Calls onClose after successful download
6. Shows loading state during export
7. Renders optional statsFooter when provided

**Validation**:
```bash
cd frontend && just test -- ExportModal
```

---

### Task 8: Integrate Export into AssetsScreen
**File**: `frontend/src/components/AssetsScreen.tsx`
**Action**: MODIFY
**Pattern**: Reference `frontend/src/components/InventoryScreen.tsx` (lines 164-170, 244-250)

**Changes**:
1. Add imports for `ShareButton`, `ExportModal`, `useExport`, asset export generators
2. Add `useExport()` hook call
3. Create `generateFile` callback that switches on format
4. Add `ShareButton` next to `AssetSearchSort` in JSX
5. Add `ExportModal` at end of component

**Integration Point** (~line 103, after `<AssetSearchSort />`):
```typescript
<div className="flex items-center justify-between gap-4">
  <AssetSearchSort className="flex-1" />
  <ShareButton
    onFormatSelect={exportState.openExport}
    disabled={filteredAssets.length === 0}
  />
</div>
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

## Risk Assessment

- **Risk**: Location store may not be populated when exporting
  **Mitigation**: `getLocationName()` returns empty string if location not in cache; `useLocations()` is already called in `AssetSearchSort` which loads locations

- **Risk**: Large asset lists may cause slow PDF generation
  **Mitigation**: Same as inventory - jsPDF handles pagination automatically; could add progress indicator later if needed

- **Risk**: ExportModal tests may need mocking for shareUtils
  **Mitigation**: Mock `shareFile` and `downloadBlob` in tests, pattern exists in ShareModal.test.tsx

## Integration Points

- **Store access**: Read-only access to `useLocationStore.getState().cache.byId` for location name resolution
- **Existing components**: Reuses `ShareButton` as-is
- **Existing utilities**: Reuses `shareUtils.ts` functions (`shareFile`, `downloadBlob`, `canShareFormat`, etc.)

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change:
```bash
cd frontend
just lint       # Gate 1: Syntax & Style
just typecheck  # Gate 2: Type Safety
just test       # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task: `cd frontend && just lint && just typecheck && just test`

Final validation: `just validate` (from project root)

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec with verified code references
✅ Similar pattern exists in codebase (`ShareModal`, `ShareButton`)
✅ All clarifying questions answered
✅ Existing test patterns to follow (`ShareModal.test.tsx`, `exportUtils.test.ts`)
✅ Dependencies already installed (xlsx, jspdf, jspdf-autotable)
✅ Single subsystem (frontend only) - no API/backend changes
⚠️ Location resolution adds coupling to location store (mitigated - store already loaded)

**Assessment**: High-confidence implementation adapting well-established patterns with comprehensive test coverage.

**Estimated one-pass success probability**: 90%

**Reasoning**: This is essentially "copy and adapt" from working inventory export code. The main patterns (ShareModal, export generators, shareUtils) are battle-tested. The only new element is the generic abstraction, which is straightforward. Location resolution is the only minor risk but is well-mitigated.
