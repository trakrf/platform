# TRA-285: Print and Export Asset List

## Context

Spun out from TRA-250 NADA launch requirements. Users need to generate physical/digital copies of asset lists for manifests, audits, and records.

The inventory screen (TRA-27) already implements share/export functionality. This ticket extends that capability to the Assets screen and centralizes the implementation for reuse across multiple screens.

## Current State Analysis

### Existing Implementation (Inventory Screen)

Location: `frontend/src/components/InventoryScreen.tsx:244-250`

```typescript
<ShareModal
  isOpen={isShareModalOpen}
  onClose={() => setIsShareModalOpen(false)}
  tags={filteredTags}
  reconciliationList={tags.some(t => t.reconciled !== undefined) ? tags.filter(t => t.reconciled !== null).map(t => t.displayEpc || t.epc) : null}
  selectedFormat={selectedExportFormat}
/>
```

**File Structure:**
```
frontend/src/
├── components/
│   ├── ShareButton.tsx              # Dropdown for format selection (lines 17-71)
│   ├── ShareModal.tsx               # Modal with share/download (lines 15-294)
│   └── inventory/
│       └── InventoryHeader.tsx      # ShareButton at lines 97-101, 170-174
├── utils/
│   ├── shareUtils.ts                # canShare, canShareFiles, shareFile, downloadBlob (300 lines)
│   ├── exportFormats.ts             # EXPORT_FORMATS config, getFormatOptions (63 lines)
│   ├── excelExportUtils.ts          # generateInventoryExcel, generateInventoryCSV (186 lines)
│   ├── pdfExportUtils.ts            # generateInventoryPDF (129 lines)
│   └── simpleExcelExport.ts         # Tab-separated fallback for sharing (101 lines)
└── types/
    └── export.ts                    # ExportFormat, ExportResult, ShareResult (41 lines)
```

**Dependencies (already installed):**
```json
{
  "jspdf": "^3.0.1",
  "jspdf-autotable": "^5.0.2",
  "xlsx": "^0.18.5"
}
```

### Component Analysis

**ShareButton** (`ShareButton.tsx:10-15`):
```typescript
interface ShareButtonProps {
  onFormatSelect: (format: ExportFormat) => void;
  disabled?: boolean;
  className?: string;
  iconOnly?: boolean;  // true for mobile, false for desktop
}
```
- Already reusable, no changes needed
- Uses `@headlessui/react` Menu component
- Gets options from `getFormatOptions()`

**ShareModal** (`ShareModal.tsx:15`):
```typescript
interface ShareModalProps {
  isOpen: boolean;
  onClose: () => void;
  tags: TagInfo[];              // Tightly coupled to TagInfo
  reconciliationList: string[] | null;
  selectedFormat: ExportFormat;
}
```
- Tightly coupled to `TagInfo[]` and inventory-specific reconciliation
- Calls `generateInventoryPDF`, `generateInventoryExcel`, `generateInventoryCSV`

**Export Generators** - All take `(tags: TagInfo[], reconciliationList: string[] | null)`:
- `generateInventoryPDF()` - Uses jsPDF with autoTable
- `generateInventoryExcel()` - Uses xlsx library
- `generateInventoryCSV()` - Plain text generation

### Data Types

**TagInfo** (`stores/tagStore.ts:13-36`):
```typescript
export interface TagInfo {
  epc: string;
  displayEpc?: string;
  rssi?: number;
  count: number;
  timestamp?: number;
  reconciled?: boolean | null;
  description?: string;
  location?: string;
  source: 'scan' | 'reconciliation' | 'rfid';
  assetId?: number;
  assetName?: string;
  assetIdentifier?: string;
}
```

**Asset** (`types/assets/index.ts:21-37`):
```typescript
export interface Asset {
  id: number;
  org_id: number;
  identifier: string;           // Customer ID (e.g., "LAP-001")
  name: string;
  type: AssetType;              // 'person' | 'device' | 'asset' | 'inventory' | 'other'
  description: string;
  current_location_id: number | null;
  valid_from: string;
  valid_to: string | null;
  metadata: Record<string, any>;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
  identifiers: TagIdentifier[]; // RFID tags linked to asset
}
```

**Location** (`types/locations/index.ts:15-31`):
```typescript
export interface Location {
  id: number;
  org_id: number;
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null;
  path: string;                 // Hierarchical path
  depth: number;
  valid_from: string;
  valid_to: string | null;
  is_active: boolean;
  metadata: Record<string, any>;
  created_at: string;
  updated_at: string;
  identifiers?: TagIdentifier[];
}
```

**TagIdentifier** (`types/shared/identifier.ts:17-22`):
```typescript
export interface TagIdentifier {
  id: number;
  type: IdentifierType;  // Currently only 'rfid'
  value: string;
  is_active: boolean;
}
```

### Screen UI Patterns

**AssetSearchSort** (`components/assets/AssetSearchSort.tsx:80-158`):
- Layout: `flex flex-col md:flex-row gap-3 md:items-center md:justify-between`
- Contains: Search input, Location filter dropdown, Sort controls, Results count
- No export button currently

**LocationSearchSort** (`components/locations/LocationSearchSort.tsx:58-112`):
- Layout: `flex flex-col md:flex-row gap-3 md:items-center md:justify-between`
- Contains: Search input, Sort controls, Results count
- No export button currently

### Store Access Patterns

```typescript
// Assets - from AssetsScreen.tsx:35-37
const filteredAssets = useMemo(() => {
  return useAssetStore.getState().getFilteredAssets();
}, [cache.byId.size, cache.lastFetched, filters, sort]);

// Locations - from LocationsScreen.tsx:46-48
const filteredLocations = useMemo(() => {
  return useLocationStore.getState().getFilteredLocations();
}, [cache.byId.size, filters, sort]);
```

## Requirements

### Functional Requirements

1. **Assets Screen Export**
   - Export filtered asset list (respects current search/filters)
   - Formats: CSV, XLSX, PDF
   - Columns based on actual Asset fields:
     - Identifier, Name, Type, Tag ID(s), Location, Status, Description, Created

2. **Locations Screen Export** (if in scope)
   - Export filtered location list
   - Columns: Identifier, Name, Path, Description, Status, Created

3. **Centralized Implementation**
   - Generic export system reusable across screens
   - Consistent UI/UX with existing inventory export

### Non-Functional Requirements

- No regression to existing inventory export
- Web Share API support maintained
- Disabled state when no data to export

## Technical Design

### Architecture: Generic Export System

```
┌─────────────────────────────────────────────────────────────┐
│                     Screen Components                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │ Inventory    │  │ Assets       │  │ Locations        │  │
│  │ Screen       │  │ Screen       │  │ Screen           │  │
│  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘  │
│         │                 │                    │            │
│         ▼                 ▼                    ▼            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              useExport Hook                         │   │
│  │  - isModalOpen, selectedFormat                      │   │
│  │  - openExport(format), closeExport()               │   │
│  └─────────────────────────────────────────────────────┘   │
│         │                 │                    │            │
│         ▼                 ▼                    ▼            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │           ExportModal<T> (Generic)                  │   │
│  │  Props: data, config, selectedFormat, title         │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                 │
│                           ▼                                 │
│  ┌─────────────────────────────────────────────────────┐   │
│  │           Export Generators (per entity)            │   │
│  │  generateAssetPDF, generateAssetExcel, etc.         │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### New Files to Create

```
frontend/src/
├── hooks/
│   └── useExport.ts                    # Export state hook
├── components/
│   └── export/
│       ├── ExportModal.tsx             # Generic export modal
│       └── index.ts                    # Barrel export
└── utils/
    └── export/
        ├── assetExport.ts              # generateAssetPDF, generateAssetExcel, generateAssetCSV
        ├── locationExport.ts           # generateLocationPDF, etc. (if in scope)
        └── index.ts                    # Barrel export
```

### Type Definitions

```typescript
// types/export.ts (extend existing)

export interface ExportConfig {
  /** File name prefix (e.g., 'assets', 'inventory', 'locations') */
  filenamePrefix: string;
  /** Title for PDF report header */
  reportTitle: string;
}
```

### Asset Export Implementation

```typescript
// utils/export/assetExport.ts

import * as XLSX from 'xlsx';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { Asset } from '@/types/assets';
import type { ExportResult } from '@/types/export';
import { getDateString, getTimestamp } from '@/utils/shareUtils';

export function generateAssetPDF(assets: Asset[]): ExportResult {
  const doc = new jsPDF();

  // Header
  doc.setFontSize(20);
  doc.text('Asset Report', 14, 20);
  doc.setFontSize(10);
  doc.setTextColor(100);
  doc.text(`Generated: ${getTimestamp()}`, 14, 30);
  doc.text(`Total Assets: ${assets.length}`, 14, 36);
  doc.text(`Active: ${assets.filter(a => a.is_active).length}`, 14, 42);
  doc.setTextColor(0);

  // Table data
  const tableData = assets.map(asset => [
    asset.identifier,
    asset.name,
    asset.type,
    asset.identifiers?.map(t => t.value).join(', ') || '',
    // Location name would need to be resolved from store or passed in
    asset.is_active ? 'Active' : 'Inactive',
    asset.description || '',
  ]);

  autoTable(doc, {
    head: [['Asset ID', 'Name', 'Type', 'Tag ID(s)', 'Status', 'Description']],
    body: tableData,
    startY: 50,
    styles: { fontSize: 8, cellPadding: 2 },
    headStyles: { fillColor: [37, 99, 235], textColor: 255, fontStyle: 'bold' },
  });

  const blob = doc.output('blob');
  return {
    blob,
    filename: `assets_${getDateString()}.pdf`,
    mimeType: 'application/pdf'
  };
}

export function generateAssetExcel(assets: Asset[]): ExportResult {
  const wb = XLSX.utils.book_new();

  const data = assets.map(asset => ({
    'Asset ID': asset.identifier,
    'Name': asset.name,
    'Type': asset.type,
    'Tag ID(s)': asset.identifiers?.map(t => t.value).join(', ') || '',
    'Status': asset.is_active ? 'Active' : 'Inactive',
    'Description': asset.description || '',
    'Created': asset.created_at ? new Date(asset.created_at).toLocaleDateString() : '',
  }));

  const ws = XLSX.utils.json_to_sheet(data);
  ws['!cols'] = [
    { wch: 15 }, { wch: 25 }, { wch: 12 }, { wch: 40 },
    { wch: 10 }, { wch: 35 }, { wch: 12 }
  ];

  XLSX.utils.book_append_sheet(wb, ws, 'Assets');

  const wbout = XLSX.write(wb, { bookType: 'xlsx', type: 'array', compression: true });
  const blob = new Blob([wbout], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  });

  return {
    blob,
    filename: `assets_${getDateString()}.xlsx`,
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  };
}

export function generateAssetCSV(assets: Asset[]): ExportResult {
  const headers = ['Asset ID', 'Name', 'Type', 'Tag ID(s)', 'Status', 'Description', 'Created'];
  let content = headers.join(',') + '\n';

  assets.forEach(asset => {
    const row = [
      `"${asset.identifier}"`,
      `"${asset.name || ''}"`,
      asset.type,
      `"${asset.identifiers?.map(t => t.value).join('; ') || ''}"`,
      asset.is_active ? 'Active' : 'Inactive',
      `"${(asset.description || '').replace(/"/g, '""')}"`,
      asset.created_at ? new Date(asset.created_at).toLocaleDateString() : '',
    ];
    content += row.join(',') + '\n';
  });

  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  return {
    blob,
    filename: `assets_${getDateString()}.csv`,
    mimeType: 'text/csv'
  };
}
```

### Generic Export Hook

```typescript
// hooks/useExport.ts

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

  return {
    isModalOpen,
    selectedFormat,
    openExport,
    closeExport,
  };
}
```

### Generic ExportModal

```typescript
// components/export/ExportModal.tsx

import { useState } from 'react';
import { Download, Share2, X, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import type { ExportFormat, ExportResult } from '@/types/export';
import { shareFile, downloadBlob, canShareFiles, canShareFormat } from '@/utils/shareUtils';
import { getFormatConfig } from '@/utils/exportFormats';

interface ExportModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedFormat: ExportFormat;
  itemCount: number;
  title: string;
  generateFile: (format: ExportFormat) => ExportResult;
}

export function ExportModal({
  isOpen,
  onClose,
  selectedFormat,
  itemCount,
  title,
  generateFile,
}: ExportModalProps) {
  // Reuse logic from ShareModal but with generic generateFile prop
  // ... (similar to ShareModal implementation)
}
```

### Integration: AssetsScreen

```typescript
// In AssetsScreen.tsx - add to imports
import { ShareButton } from '@/components/ShareButton';
import { ExportModal } from '@/components/export/ExportModal';
import { useExport } from '@/hooks/useExport';
import { generateAssetPDF, generateAssetExcel, generateAssetCSV } from '@/utils/export/assetExport';

// In component body
const exportState = useExport();

const generateFile = (format: ExportFormat) => {
  switch (format) {
    case 'pdf': return generateAssetPDF(filteredAssets);
    case 'xlsx': return generateAssetExcel(filteredAssets);
    case 'csv': return generateAssetCSV(filteredAssets);
  }
};

// In JSX - add ShareButton to AssetSearchSort area
<div className="flex items-center justify-between gap-4">
  <AssetSearchSort className="flex-1" />
  <ShareButton
    onFormatSelect={exportState.openExport}
    disabled={filteredAssets.length === 0}
  />
</div>

// Add ExportModal at end of component
<ExportModal
  isOpen={exportState.isModalOpen}
  onClose={exportState.closeExport}
  selectedFormat={exportState.selectedFormat}
  itemCount={filteredAssets.length}
  title="Export Assets"
  generateFile={generateFile}
/>
```

## File Changes Summary

| File | Action | Description |
|------|--------|-------------|
| `hooks/useExport.ts` | Create | Export state hook (~25 lines) |
| `components/export/ExportModal.tsx` | Create | Generic modal (~150 lines) |
| `components/export/index.ts` | Create | Barrel export |
| `utils/export/assetExport.ts` | Create | Asset PDF/Excel/CSV generators (~120 lines) |
| `utils/export/index.ts` | Create | Barrel export |
| `components/AssetsScreen.tsx` | Modify | Add ShareButton and ExportModal |

## Testing Plan

### Unit Tests

1. `hooks/useExport.test.ts` - Hook state transitions
2. `utils/export/assetExport.test.ts` - Export generation for each format

### Integration Tests

1. Export with filters applied - verify filtered data exported
2. Export with empty list - button disabled
3. All formats generate valid files

### Manual Testing

1. CSV opens in Excel/Google Sheets
2. XLSX opens in Excel with correct columns
3. PDF is readable with correct formatting
4. Web Share works on mobile (iOS Safari, Android Chrome)
5. Download works on desktop browsers

## Acceptance Criteria

- [ ] Assets screen has Share/Export button (matches inventory UI)
- [ ] Export to CSV with columns: Asset ID, Name, Type, Tag ID(s), Status, Description, Created
- [ ] Export to XLSX with same columns + formatting
- [ ] Export to PDF with report header and table
- [ ] Export respects current search/filter state
- [ ] Share button disabled when `filteredAssets.length === 0`
- [ ] Web Share API works on supported devices
- [ ] Download fallback works universally
- [ ] No regression to inventory export

## Out of Scope

- Print stylesheet (browser print via PDF is sufficient)
- Locations screen export (can be added with same pattern later)
- Custom field export from metadata
- Location name resolution (shows location_id for now, or blank)
