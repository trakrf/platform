# Implementation Plan: Asset & Location Scanner Integration (Phase 2-3)

**Feature Branch**: `feature/asset-location-scanner-integration`
**Spec**: `spec/active/asset-location-scanner-integration/spec.md`
**Coverage**: MR1-MR4 (Scanner Integration + Inventory Workflow)

---

## Overview

This plan covers Phase 2-3 of the scanner integration feature, implementing:
- **MR1**: AssetForm Scanner Integration
- **MR2**: LocationForm Scanner Integration
- **MR3**: Inventory Edit Button (TRA-131)
- **MR4**: Asset Enrichment Flow

**Estimated Effort**: 4-5 days
**Linear Tickets**: 1 existing (TRA-131) + 3 new tickets
**Unit Tests**: ~20 tests total
**Testing Strategy**: 80% unit, 15% integration, 5% E2E

---

## Prerequisites

✅ **Already Completed**:
- `useScanToInput` hook implemented and tested
- Backend supports `location_id` field for assets
- Asset enrichment on RFID reads (recent PR)
- Existing stores: useTagStore, useBarcodeStore, useDeviceStore, useAssetStore

🔧 **Validation Gates** (run before each MR):
```bash
pnpm lint          # ESLint checks
pnpm typecheck     # TypeScript validation
pnpm test          # Unit tests
pnpm build         # Production build
```

---

## MR1: AssetForm Scanner Integration

**Linear Ticket**: Create new ticket
- **Title**: "Add RFID/Barcode scanner buttons to Asset creation form"
- **Description**: "Enable scanner input for asset identifier field using useScanToInput hook. Scanner buttons appear when device connected, only in create mode."
- **Assignee**: nicholusmuwonge
- **Labels**: frontend, scanner-integration

### Files to Modify

**Primary**: `frontend/src/components/assets/AssetForm.tsx` (273 lines → ~350 lines)

### Implementation Steps

#### 1. Add Imports
```typescript
import { useScanToInput } from '@/hooks/useScanToInput';
import { useDeviceStore } from '@/stores';
import { ScanLine, QrCode, X } from 'lucide-react';
```

#### 2. Add Scanner Hook Integration
```typescript
// Inside AssetForm component, after formData state
const isConnected = useDeviceStore((s) => s.isConnected);

const { startRfidScan, startBarcodeScan, stopScan, isScanning, scanType } = useScanToInput({
  onScan: (value) => handleChange('identifier', value),
  autoStop: true,
});
```

#### 3. Add Scanner Buttons UI
Location: After the identifier input field (line 141), add:

```typescript
{mode === 'create' && isConnected && !isScanning && (
  <div className="flex gap-2 mt-2">
    <button
      type="button"
      onClick={startRfidScan}
      disabled={loading}
      className="flex items-center gap-2 px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-lg disabled:opacity-50 transition-colors"
    >
      <ScanLine className="w-4 h-4" />
      Scan RFID
    </button>
    <button
      type="button"
      onClick={startBarcodeScan}
      disabled={loading}
      className="flex items-center gap-2 px-3 py-1.5 text-xs font-medium text-white bg-green-600 hover:bg-green-700 rounded-lg disabled:opacity-50 transition-colors"
    >
      <QrCode className="w-4 h-4" />
      Scan Barcode
    </button>
  </div>
)}

{isScanning && (
  <div className="flex items-center gap-2 mt-2">
    <p className="text-sm text-blue-600 dark:text-blue-400">
      {scanType === 'rfid' ? 'Scanning for RFID tag...' : 'Scanning for barcode...'}
    </p>
    <button
      type="button"
      onClick={stopScan}
      className="flex items-center gap-1 px-2 py-1 text-xs font-medium text-white bg-red-600 hover:bg-red-700 rounded-lg transition-colors"
    >
      <X className="w-3 h-3" />
      Cancel
    </button>
  </div>
)}
```

#### 4. Update Identifier Input State
Modify the identifier input to show scanning state (line 129):

```typescript
<input
  type="text"
  id="identifier"
  value={formData.identifier}
  onChange={(e) => handleChange('identifier', e.target.value)}
  disabled={loading || mode === 'edit' || isScanning}  // Add isScanning
  className={`block w-full px-3 py-2 border rounded-lg ${
    fieldErrors.identifier
      ? 'border-red-500 focus:ring-red-500'
      : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
  } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 disabled:cursor-not-allowed`}
  placeholder={isScanning
    ? (scanType === 'rfid' ? 'Scanning RFID...' : 'Scanning barcode...')
    : 'e.g., LAP-001'
  }
/>
```

### Unit Tests (9 tests)

**File**: `frontend/src/components/assets/AssetForm.test.tsx` (new file)

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { AssetForm } from './AssetForm';
import { useDeviceStore } from '@/stores';
import * as useScanToInputModule from '@/hooks/useScanToInput';

describe('AssetForm - Scanner Integration', () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();
  const mockStartRfidScan = vi.fn();
  const mockStartBarcodeScan = vi.fn();
  const mockStopScan = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();

    // Mock useScanToInput
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: false,
      scanType: null,
    });
  });

  it('should show scanner buttons when device connected in create mode', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scan RFID')).toBeInTheDocument();
    expect(screen.getByText('Scan Barcode')).toBeInTheDocument();
  });

  it('should hide scanner buttons when device disconnected', () => {
    useDeviceStore.setState({ isConnected: false });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.queryByText('Scan RFID')).not.toBeInTheDocument();
    expect(screen.queryByText('Scan Barcode')).not.toBeInTheDocument();
  });

  it('should hide scanner buttons in edit mode', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <AssetForm
        mode="edit"
        asset={{ id: 1, identifier: 'TEST-001', name: 'Test Asset', type: 'device' } as any}
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.queryByText('Scan RFID')).not.toBeInTheDocument();
  });

  it('should call startRfidScan when RFID button clicked', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByText('Scan RFID'));
    expect(mockStartRfidScan).toHaveBeenCalledTimes(1);
  });

  it('should call startBarcodeScan when Barcode button clicked', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByText('Scan Barcode'));
    expect(mockStartBarcodeScan).toHaveBeenCalledTimes(1);
  });

  it('should show scanning state feedback for RFID', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'rfid',
    });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scanning for RFID tag...')).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
  });

  it('should show scanning state feedback for barcode', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'barcode',
    });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scanning for barcode...')).toBeInTheDocument();
  });

  it('should disable input while scanning', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'rfid',
    });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    const input = screen.getByPlaceholderText(/Scanning RFID/i);
    expect(input).toBeDisabled();
  });

  it('should populate identifier when onScan callback triggers', async () => {
    useDeviceStore.setState({ isConnected: true });
    let capturedOnScan: ((value: string) => void) | null = null;

    vi.spyOn(useScanToInputModule, 'useScanToInput').mockImplementation(({ onScan }) => {
      capturedOnScan = onScan;
      return {
        startRfidScan: mockStartRfidScan,
        startBarcodeScan: mockStartBarcodeScan,
        stopScan: mockStopScan,
        isScanning: false,
        scanType: null,
      };
    });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Simulate scan callback
    capturedOnScan?.('E280116060000020957C5876');

    await waitFor(() => {
      const input = screen.getByPlaceholderText(/e.g., LAP-001/i) as HTMLInputElement;
      expect(input.value).toBe('E280116060000020957C5876');
    });
  });
});
```

### Validation

```bash
# 1. Type check
pnpm typecheck

# 2. Run tests
pnpm test AssetForm.test.tsx

# Expected: 9/9 tests passing

# 3. Lint
pnpm lint

# 4. Build
pnpm build
```

### Post-MR Workflow

After creating the PR:

1. **Get PR URL**: `https://github.com/trakrf/platform/pull/XXX`
2. **Comment on Linear Ticket**:
   ```
   PR ready for review: https://github.com/trakrf/platform/pull/XXX

   Changes:
   - Added RFID/Barcode scanner buttons to AssetForm
   - Scanner buttons visible only when device connected and in create mode
   - Scanning state feedback with cancel option
   - 9/9 unit tests passing

   @mike please review
   ```

---

## MR2: LocationForm Scanner Integration

**Linear Ticket**: Create new ticket
- **Title**: "Add RFID/Barcode scanner buttons to Location creation form"
- **Description**: "Enable scanner input for location identifier field using useScanToInput hook. Consistent styling with AssetForm."
- **Assignee**: nicholusmuwonge
- **Labels**: frontend, scanner-integration

### Files to Modify

**Primary**: `frontend/src/components/locations/LocationForm.tsx` (301 lines → ~370 lines)

### Implementation Steps

#### 1. Add Imports
```typescript
import { useScanToInput } from '@/hooks/useScanToInput';
import { useDeviceStore } from '@/stores';
import { ScanLine, QrCode, X } from 'lucide-react';
```

#### 2. Add Scanner Hook Integration
```typescript
// Inside LocationForm component, after formData state
const isConnected = useDeviceStore((s) => s.isConnected);

const { startRfidScan, startBarcodeScan, stopScan, isScanning, scanType } = useScanToInput({
  onScan: (value) => handleChange('identifier', value),
  autoStop: true,
});
```

#### 3. Add Scanner Buttons UI
**Location**: After the identifier input field (line 162), add same button structure as AssetForm:

```typescript
{mode === 'create' && isConnected && !isScanning && (
  <div className="flex gap-2 mt-2">
    <button
      type="button"
      onClick={startRfidScan}
      disabled={loading}
      className="flex items-center gap-2 px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-lg disabled:opacity-50 transition-colors"
    >
      <ScanLine className="w-4 h-4" />
      Scan RFID
    </button>
    <button
      type="button"
      onClick={startBarcodeScan}
      disabled={loading}
      className="flex items-center gap-2 px-3 py-1.5 text-xs font-medium text-white bg-green-600 hover:bg-green-700 rounded-lg disabled:opacity-50 transition-colors"
    >
      <QrCode className="w-4 h-4" />
      Scan Barcode
    </button>
  </div>
)}

{isScanning && (
  <div className="flex items-center gap-2 mt-2">
    <p className="text-sm text-blue-600 dark:text-blue-400">
      {scanType === 'rfid' ? 'Scanning for RFID tag...' : 'Scanning for barcode...'}
    </p>
    <button
      type="button"
      onClick={stopScan}
      className="flex items-center gap-1 px-2 py-1 text-xs font-medium text-white bg-red-600 hover:bg-red-700 rounded-lg transition-colors"
    >
      <X className="w-3 h-3" />
      Cancel
    </button>
  </div>
)}
```

#### 4. Update Identifier Input State
Modify the identifier input (line 147):

```typescript
<input
  type="text"
  id="identifier"
  value={formData.identifier}
  onChange={(e) => handleChange('identifier', e.target.value)}
  disabled={loading || mode === 'edit' || isScanning}  // Add isScanning
  className={`block w-full px-3 py-2 border rounded-lg ${
    fieldErrors.identifier
      ? 'border-red-500 focus:ring-red-500'
      : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
  } bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50`}
  placeholder={isScanning
    ? (scanType === 'rfid' ? 'Scanning RFID...' : 'Scanning barcode...')
    : 'e.g., warehouse_a'
  }
/>
```

### Unit Tests (4 tests)

**File**: `frontend/src/components/locations/LocationForm.test.tsx` (new file)

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { LocationForm } from './LocationForm';
import { useDeviceStore } from '@/stores';
import * as useScanToInputModule from '@/hooks/useScanToInput';

describe('LocationForm - Scanner Integration', () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();
  const mockStartRfidScan = vi.fn();
  const mockStartBarcodeScan = vi.fn();
  const mockStopScan = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();

    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: false,
      scanType: null,
    });
  });

  it('should show scanner buttons when device connected in create mode', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scan RFID')).toBeInTheDocument();
    expect(screen.getByText('Scan Barcode')).toBeInTheDocument();
  });

  it('should use consistent styling with AssetForm', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    const rfidButton = screen.getByText('Scan RFID').closest('button');
    expect(rfidButton).toHaveClass('bg-blue-600');

    const barcodeButton = screen.getByText('Scan Barcode').closest('button');
    expect(barcodeButton).toHaveClass('bg-green-600');
  });

  it('should show scanning state feedback', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'rfid',
    });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scanning for RFID tag...')).toBeInTheDocument();
  });

  it('should call scanner functions correctly', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByText('Scan RFID'));
    expect(mockStartRfidScan).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText('Scan Barcode'));
    expect(mockStartBarcodeScan).toHaveBeenCalledTimes(1);
  });
});
```

### Validation

```bash
pnpm typecheck
pnpm test LocationForm.test.tsx
# Expected: 4/4 tests passing
pnpm lint
pnpm build
```

### Post-MR Workflow

```
PR ready for review: [PR_URL]

Changes:
- Added RFID/Barcode scanner buttons to LocationForm
- Consistent styling with AssetForm
- 4/4 unit tests passing

@mike please review
```

---

## MR3: Inventory Edit Button (TRA-131)

**Linear Ticket**: TRA-131 (existing)
- **Title**: "Add support for adding an asset or location from inventory"
- **Assignee**: nicholusmuwonge

### Files to Modify

**Primary**: `frontend/src/components/inventory/InventoryTableRow.tsx` (127 lines → ~180 lines)

### Implementation Steps

#### 1. Add Imports
```typescript
import { Pencil } from 'lucide-react';
import { AssetFormModal } from '@/components/assets/AssetFormModal';
```

#### 2. Add Modal State
```typescript
// Inside InventoryTableRow component, after existing state
const [isAssetFormOpen, setIsAssetFormOpen] = useState(false);
```

#### 3. Add Edit Button
**Location**: Add new column in the row structure, before the Locate button (around line 105):

```typescript
<div className="w-24 text-center">
  <button
    onClick={(e) => {
      e.preventDefault();
      setIsAssetFormOpen(true);
    }}
    className="inline-flex items-center justify-center gap-1 px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 rounded-lg transition-colors"
    title={tag.assetId ? 'Edit Asset' : 'Create Asset'}
  >
    <Pencil className="w-3.5 h-3.5" />
    {tag.assetId ? 'Edit' : 'Create'}
  </button>
</div>
```

#### 4. Add AssetFormModal
**Location**: After the existing AssetDetailsModal (after line 124):

```typescript
{/* Asset Create/Edit Modal */}
<AssetFormModal
  isOpen={isAssetFormOpen}
  mode={tag.assetId ? 'edit' : 'create'}
  asset={tag.assetId ? asset : undefined}
  onClose={() => setIsAssetFormOpen(false)}
  initialIdentifier={!tag.assetId ? (tag.displayEpc || tag.epc) : undefined}
/>
```

#### 5. Update AssetFormModal to Support Initial Identifier
**File**: `frontend/src/components/assets/AssetFormModal.tsx`

Add prop:
```typescript
interface AssetFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  asset?: Asset;
  onClose: () => void;
  initialIdentifier?: string;  // NEW
}

export function AssetFormModal({
  isOpen,
  mode,
  asset,
  onClose,
  initialIdentifier  // NEW
}: AssetFormModalProps) {
  // ... existing code ...
}
```

Pass to AssetForm:
```typescript
<AssetForm
  mode={mode}
  asset={asset}
  onSubmit={handleSubmit}
  onCancel={onClose}
  loading={loading}
  error={error}
  initialIdentifier={initialIdentifier}  // NEW
/>
```

#### 6. Update AssetForm to Support Initial Identifier
**File**: `frontend/src/components/assets/AssetForm.tsx`

Add prop and use in initial state:
```typescript
interface AssetFormProps {
  mode: 'create' | 'edit';
  asset?: Asset;
  onSubmit: (data: CreateAssetRequest | UpdateAssetRequest) => Promise<void>;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
  initialIdentifier?: string;  // NEW
}

export function AssetForm({
  mode,
  asset,
  onSubmit,
  onCancel,
  loading = false,
  error,
  initialIdentifier  // NEW
}: AssetFormProps) {
  const [formData, setFormData] = useState({
    identifier: asset?.identifier || initialIdentifier || '',  // UPDATED
    name: asset?.name || '',
    // ... rest of fields
  });
}
```

### Unit Tests (6 tests)

**File**: `frontend/src/components/inventory/InventoryTableRow.test.tsx` (new file)

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { InventoryTableRow } from './InventoryTableRow';
import { useAssetStore } from '@/stores';
import type { TagInfo } from '@/stores/tagStore';

describe('InventoryTableRow - Asset Actions', () => {
  const mockTag: TagInfo = {
    epc: 'E280116060000020957C5876',
    displayEpc: '10018',
    count: 5,
    timestamp: Date.now(),
    rssi: -45,
    reconciled: true,
    assetId: null,
    assetName: null,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render Create button for tags without assetId', () => {
    render(<InventoryTableRow tag={mockTag} />);

    expect(screen.getByText('Create')).toBeInTheDocument();
    expect(screen.getByTitle('Create Asset')).toBeInTheDocument();
  });

  it('should render Edit button for tags with assetId', () => {
    const tagWithAsset: TagInfo = {
      ...mockTag,
      assetId: 1,
      assetName: 'Test Asset',
    };

    useAssetStore.setState({
      cache: {
        byId: new Map([[1, { id: 1, identifier: 'TEST-001', name: 'Test Asset', type: 'device' } as any]]),
      },
    });

    render(<InventoryTableRow tag={tagWithAsset} />);

    expect(screen.getByText('Edit')).toBeInTheDocument();
    expect(screen.getByTitle('Edit Asset')).toBeInTheDocument();
  });

  it('should open AssetFormModal in create mode when Create clicked', () => {
    render(<InventoryTableRow tag={mockTag} />);

    fireEvent.click(screen.getByText('Create'));

    // Modal should be rendered (test modal visibility)
    expect(screen.getByText('Create New Asset')).toBeInTheDocument();
  });

  it('should open AssetFormModal in edit mode when Edit clicked', () => {
    const tagWithAsset: TagInfo = {
      ...mockTag,
      assetId: 1,
      assetName: 'Test Asset',
    };

    const mockAsset = {
      id: 1,
      identifier: 'TEST-001',
      name: 'Test Asset',
      type: 'device' as const
    };

    useAssetStore.setState({
      cache: {
        byId: new Map([[1, mockAsset]]),
      },
      getAssetById: vi.fn(() => mockAsset),
    });

    render(<InventoryTableRow tag={tagWithAsset} />);

    fireEvent.click(screen.getByText('Edit'));

    expect(screen.getByText(/Edit Asset:/)).toBeInTheDocument();
  });

  it('should pre-fill identifier from EPC in create mode', () => {
    render(<InventoryTableRow tag={mockTag} />);

    fireEvent.click(screen.getByText('Create'));

    const identifierInput = screen.getByPlaceholderText(/e.g., LAP-001/i) as HTMLInputElement;
    expect(identifierInput.value).toBe('10018');  // displayEpc
  });

  it('should load asset data in edit mode', () => {
    const mockAsset = {
      id: 1,
      identifier: 'TEST-001',
      name: 'Test Asset',
      type: 'device' as const,
      description: 'Test description',
      is_active: true,
    };

    const tagWithAsset: TagInfo = {
      ...mockTag,
      assetId: 1,
      assetName: 'Test Asset',
    };

    useAssetStore.setState({
      cache: {
        byId: new Map([[1, mockAsset]]),
      },
      getAssetById: vi.fn(() => mockAsset),
    });

    render(<InventoryTableRow tag={tagWithAsset} />);

    fireEvent.click(screen.getByText('Edit'));

    const identifierInput = screen.getByDisplayValue('TEST-001');
    expect(identifierInput).toBeInTheDocument();
  });
});
```

### Validation

```bash
pnpm typecheck
pnpm test InventoryTableRow.test.tsx
# Expected: 6/6 tests passing
pnpm lint
pnpm build
```

### Post-MR Workflow

```
PR ready for review: [PR_URL]

Closes TRA-131

Changes:
- Added pencil/edit button to inventory rows
- Opens AssetFormModal in create mode for unlinked tags
- Opens AssetFormModal in edit mode for linked tags
- Pre-fills identifier from tag EPC in create mode
- 6/6 unit tests passing

@mike please review
```

---

## MR4: Asset Enrichment Flow

**Linear Ticket**: Create new ticket
- **Title**: "Inventory refresh after asset create/edit from inventory row"
- **Description**: "Ensure inventory table updates with new asset data after creating or editing assets from inventory rows. Asset enrichment should re-run automatically."
- **Assignee**: nicholusmuwonge
- **Labels**: frontend, inventory, asset-enrichment

### Files to Modify

**Primary**:
- `frontend/src/components/inventory/InventoryTableRow.tsx` (add refresh callback)
- `frontend/src/components/inventory/InventoryScreen.tsx` (coordinate refresh)

### Implementation Steps

#### 1. Update InventoryTableRow
Add success callback to AssetFormModal:

```typescript
// Inside InventoryTableRow component
const handleAssetFormClose = useCallback(() => {
  setIsAssetFormOpen(false);
  // Trigger inventory refresh to update enrichment
  // This will be passed down as a prop from InventoryScreen
  onAssetUpdated?.();
}, [onAssetUpdated]);

// Update AssetFormModal
<AssetFormModal
  isOpen={isAssetFormOpen}
  mode={tag.assetId ? 'edit' : 'create'}
  asset={tag.assetId ? asset : undefined}
  onClose={handleAssetFormClose}  // UPDATED
  initialIdentifier={!tag.assetId ? (tag.displayEpc || tag.epc) : undefined}
/>
```

#### 2. Update InventoryTableRow Props
```typescript
interface InventoryTableRowProps {
  tag: TagInfo;
  onAssetUpdated?: () => void;  // NEW
}

export function InventoryTableRow({ tag, onAssetUpdated }: InventoryTableRowProps) {
  // ... implementation
}
```

#### 3. Update InventoryScreen
**File**: `frontend/src/components/inventory/InventoryScreen.tsx`

Add refresh handler and pass to rows:

```typescript
// Inside InventoryScreen component
const handleAssetUpdated = useCallback(() => {
  // Asset enrichment runs automatically via worker/inventory subsystem
  // Just trigger a re-render by refreshing tag list
  console.log('[InventoryScreen] Asset updated, enrichment will refresh');
}, []);

// Pass to InventoryTableRow
<InventoryTableRow
  key={tag.epc}
  tag={tag}
  onAssetUpdated={handleAssetUpdated}  // NEW
/>
```

### Integration Test (1 test)

**File**: `frontend/src/components/inventory/InventoryScreen.integration.test.tsx` (new file)

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { InventoryScreen } from './InventoryScreen';
import { useTagStore, useAssetStore } from '@/stores';
import { assetsApi } from '@/lib/api/assets';

vi.mock('@/lib/api/assets', () => ({
  assetsApi: {
    create: vi.fn(),
  },
}));

describe('InventoryScreen - Asset Enrichment Flow', () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Setup initial tag without asset
    useTagStore.setState({
      tags: [{
        epc: 'E280116060000020957C5876',
        displayEpc: '10018',
        count: 1,
        timestamp: Date.now(),
        assetId: null,
        assetName: null,
      }],
    });

    useAssetStore.setState({
      cache: {
        byId: new Map(),
        allIds: [],
      },
    });
  });

  it('should update inventory row after asset created from tag', async () => {
    vi.mocked(assetsApi.create).mockResolvedValue({
      data: {
        data: {
          id: 1,
          identifier: '10018',
          name: 'New Asset',
          type: 'device',
          is_active: true,
        },
      },
    } as any);

    render(<InventoryScreen />);

    // Click Create button
    fireEvent.click(screen.getByText('Create'));

    // Fill form
    const nameInput = screen.getByLabelText(/Name/i);
    fireEvent.change(nameInput, { target: { value: 'New Asset' } });

    // Submit
    fireEvent.click(screen.getByText('Create Asset'));

    // Wait for modal to close
    await waitFor(() => {
      expect(screen.queryByText('Create New Asset')).not.toBeInTheDocument();
    });

    // Asset enrichment will run in background
    // Update tag store to simulate enrichment
    useTagStore.setState({
      tags: [{
        epc: 'E280116060000020957C5876',
        displayEpc: '10018',
        count: 1,
        timestamp: Date.now(),
        assetId: 1,
        assetName: 'New Asset',
      }],
    });

    // Verify row shows asset name
    await waitFor(() => {
      expect(screen.getByText('New Asset')).toBeInTheDocument();
    });
  });
});
```

### Validation

```bash
pnpm typecheck
pnpm test InventoryScreen.integration.test.tsx
# Expected: 1/1 test passing
pnpm lint
pnpm build
```

### Post-MR Workflow

```
PR ready for review: [PR_URL]

Changes:
- Added onAssetUpdated callback to InventoryTableRow
- InventoryScreen coordinates refresh after asset changes
- Asset enrichment re-runs automatically via existing subsystem
- 1/1 integration test passing

@mike please review
```

---

## Linear Ticket Creation

Create 3 new Linear tickets manually via Linear UI or API:

### MR1 Ticket: AssetForm Scanner
- **Title**: Add RFID/Barcode scanner buttons to Asset creation form
- **Description**: Enable scanner input for asset identifier field using useScanToInput hook. Scanner buttons appear when device connected, only in create mode.
- **Assignee**: nicholusmuwonge
- **Labels**: frontend, scanner-integration
- **Priority**: 2

### MR2 Ticket: LocationForm Scanner
- **Title**: Add RFID/Barcode scanner buttons to Location creation form
- **Description**: Enable scanner input for location identifier field using useScanToInput hook. Consistent styling with AssetForm.
- **Assignee**: nicholusmuwonge
- **Labels**: frontend, scanner-integration
- **Priority**: 2

### MR4 Ticket: Asset Enrichment
- **Title**: Inventory refresh after asset create/edit from inventory row
- **Description**: Ensure inventory table updates with new asset data after creating or editing assets from inventory rows. Asset enrichment should re-run automatically.
- **Assignee**: nicholusmuwonge
- **Labels**: frontend, inventory, asset-enrichment
- **Priority**: 2

**Note**: MR3 uses existing ticket TRA-131. Create tickets via Linear UI or use Linear API with your API key from `~/.env.local`.

---

## Summary Checklist

### Before Starting
- [ ] Read spec: `spec/active/asset-location-scanner-integration/spec.md`
- [ ] Create 3 new Linear tickets (MR1, MR2, MR4)
- [ ] Ensure feature branch exists: `feature/asset-location-scanner-integration`

### MR1: AssetForm Scanner
- [ ] Implement scanner buttons in AssetForm
- [ ] Add scanning state feedback
- [ ] Write 9 unit tests
- [ ] Run validation gates
- [ ] Create PR
- [ ] Comment on Linear ticket + tag @mike

### MR2: LocationForm Scanner
- [ ] Implement scanner buttons in LocationForm
- [ ] Ensure consistent styling with AssetForm
- [ ] Write 4 unit tests
- [ ] Run validation gates
- [ ] Create PR
- [ ] Comment on Linear ticket + tag @mike

### MR3: Inventory Edit Button (TRA-131)
- [ ] Add pencil button to InventoryTableRow
- [ ] Implement AssetFormModal integration
- [ ] Add initialIdentifier prop support
- [ ] Write 6 unit tests
- [ ] Run validation gates
- [ ] Create PR
- [ ] Comment on TRA-131 + tag @mike

### MR4: Asset Enrichment
- [ ] Add onAssetUpdated callback
- [ ] Update InventoryScreen coordination
- [ ] Write 1 integration test
- [ ] Run validation gates
- [ ] Create PR
- [ ] Comment on Linear ticket + tag @mike

### Final Validation
- [ ] All 20 tests passing (9+4+6+1)
- [ ] pnpm validate passes
- [ ] No TypeScript errors
- [ ] No ESLint warnings
- [ ] Production build successful

---

## Expected Outcomes

**Total Tests**: 20 (9+4+6+1)
**Total MRs**: 4
**Total PRs**: 4
**Linear Tickets**: 3 new + 1 existing (TRA-131)

**Files Modified**:
- `frontend/src/components/assets/AssetForm.tsx` (MR1, MR3)
- `frontend/src/components/assets/AssetFormModal.tsx` (MR3)
- `frontend/src/components/locations/LocationForm.tsx` (MR2)
- `frontend/src/components/inventory/InventoryTableRow.tsx` (MR3, MR4)
- `frontend/src/components/inventory/InventoryScreen.tsx` (MR4)

**Files Created**:
- `frontend/src/components/assets/AssetForm.test.tsx` (9 tests)
- `frontend/src/components/locations/LocationForm.test.tsx` (4 tests)
- `frontend/src/components/inventory/InventoryTableRow.test.tsx` (6 tests)
- `frontend/src/components/inventory/InventoryScreen.integration.test.tsx` (1 test)

**User Value**:
- ✅ Faster asset/location creation via scanner
- ✅ Seamless inventory → asset workflow
- ✅ Real-time enrichment updates
- ✅ Consistent UX across forms
