# Feature: Asset & Location Scanner Integration with Workflow Enhancements

## Origin
**Feature Request**: Enable scanner-based data entry for Assets and Locations, with streamlined workflows from inventory tags to asset management.

**Branch**: `feature/asset-location-scanner-integration`

## Outcome
- Asset and Location forms support RFID/Barcode scanning for identifier fields
- Inventory tags can directly create or edit assets with pre-filled identifiers
- Assets can be linked to locations through form selectors
- Quick location creation from asset forms
- Reusable scanning logic with zero duplication

## User Story

**As a user creating an asset**
I want to scan an RFID tag or barcode instead of typing the identifier
So that I can quickly and accurately register assets without manual data entry

**As a user viewing inventory**
I want to create or edit assets directly from scanned tags
So that I can immediately register unidentified items or update existing asset details

**As a user managing assets**
I want to assign assets to locations and create new locations if needed
So that I can maintain accurate asset tracking and location hierarchies

## Context

### Current State
- Assets and Locations use manual text input for identifiers
- Inventory tags show asset details but have no direct asset management actions
- No relationship between assets and locations
- Scanning infrastructure exists (useTagStore, useBarcodeStore, DeviceManager)

### Desired State
- Forms have scanner buttons for RFID and barcode input
- Inventory rows have "Create Asset" and "Edit Asset" actions
- Asset forms include location selector and quick location creation
- All scanning reuses existing infrastructure through useScanToInput hook

## Technical Requirements

### 1. Scanner Input Hook (Foundation - COMPLETED ✅)
**File**: `frontend/src/hooks/useScanToInput.ts`

**Purpose**: Reusable hook that captures RFID/Barcode scans into form inputs

**Capabilities**:
- Temporarily switches reader mode to INVENTORY or BARCODE
- Listens to existing stores (useTagStore, useBarcodeStore) for new scans
- Auto-captures latest scan and returns to IDLE mode
- Supports manual stop and custom return modes
- No duplication - leverages existing DeviceManager and stores

**Exports**:
- `startRfidScan()` - Start RFID scanning
- `startBarcodeScan()` - Start barcode scanning
- `stopScan()` - Cancel current scan
- `isScanning` - Boolean scanning state
- `scanType` - 'rfid' | 'barcode' | null

**Hook Options**:
- `onScan: (value: string) => void` - Callback when scan captured
- `autoStop?: boolean` - Auto-stop after first scan (default: true)
- `returnMode?: ReaderModeType` - Mode to return to after scan (default: IDLE)

---

### 2. Asset Form Scanner Integration
**File**: `frontend/src/components/assets/AssetForm.tsx`

**Changes Required**:
- Import useScanToInput hook and useDeviceStore
- Add scanner hook with onScan callback updating identifier field
- Add RFID and Barcode scanner buttons next to identifier input
- Show scanning state feedback ("Scanning for RFID tag...")
- Only show scanner buttons when device is connected and in create mode

**UI Requirements**:
- Scanner buttons appear inline with identifier input
- RFID button: Blue background, scan icon
- Barcode button: Green background, scan icon
- Cancel button: Red background, X icon (appears during scanning)
- Scanning state shows which type is active
- Placeholder text updates during scanning
- Input disabled while scanning

**Behavior**:
- Scanner buttons only visible when device connected
- Only enabled in create mode (not edit mode)
- One scan auto-populates identifier and stops
- Manual cancel returns to IDLE mode
- Form validation applies to scanned values

---

### 3. Location Form Scanner Integration
**File**: `frontend/src/components/locations/LocationForm.tsx`

**Changes Required**:
- Same implementation as AssetForm
- Import useScanToInput hook and useDeviceStore
- Add scanner hook with onScan callback updating identifier field
- Add RFID and Barcode scanner buttons
- Show scanning state feedback

**UI Requirements**:
- Identical button styling to AssetForm for consistency
- Same behavior and constraints
- Scanner buttons inline with identifier input

---

### 4. Inventory Row Asset Actions
**File**: `frontend/src/components/inventory/InventoryTableRow.tsx`

**Changes Required for Tags WITHOUT assetId**:
- Add "Create Asset" button in actions column
- Button opens AssetFormModal in create mode
- Pre-fill identifier with tag EPC
- Button styling: Primary blue with Plus icon

**Changes Required for Tags WITH assetId**:
- Add "Edit Asset" button in actions column
- Button opens AssetFormModal in edit mode
- Load existing asset data by assetId
- Button styling: Secondary gray with Pencil icon

**Layout Requirements**:
- Actions column expands to fit new buttons
- Locate button remains (existing functionality)
- Buttons stack vertically on mobile
- Hover states and tooltips for clarity

**Data Flow**:
- Tag EPC → AssetFormModal identifier (for create)
- Tag assetId → Asset lookup → AssetFormModal (for edit)
- Modal closes → Inventory updates with new/edited asset data
- Asset enrichment re-runs to update tag.assetName

---

### 5. Asset-Location Relationship (Backend Schema Update Required)
**Files**:
- `backend/internal/models/asset/asset.go` (add location_id field)
- `frontend/src/types/assets/index.ts` (add location_id to Asset interface)

**Schema Changes**:
- Add `location_id` (nullable integer) to Asset struct
- Foreign key constraint to locations table
- Migration file for schema update

**Frontend Type Changes**:
- Add `location_id?: number | null` to Asset interface
- Add `location_id?: number | null` to CreateAssetRequest
- Add `location_id?: number | null` to UpdateAssetRequest

---

### 6. Location Selector in Asset Form
**File**: `frontend/src/components/assets/AssetForm.tsx`

**Changes Required**:
- Import useLocations hook to fetch available locations
- Add location_id to formData state
- Add Location dropdown select field in form
- Dropdown shows location path for hierarchy clarity
- "None" option for unassigned assets
- "Create New Location" quick action button

**UI Requirements**:
- Location selector appears after Type field
- Dropdown shows location.path (e.g., "Building A > Floor 2 > Room 201")
- Loading state while fetching locations
- Empty state with "Create New Location" CTA
- Create button opens LocationFormModal inline

**Data Flow**:
- useLocations fetches all active locations
- Form submit includes location_id in request
- New location created → Dropdown refreshes → Auto-selects new location

---

### 7. Quick Location Creation from Asset Form
**File**: `frontend/src/components/assets/AssetFormModal.tsx`

**Changes Required**:
- Add state for nested LocationFormModal
- "Create New Location" button next to location selector
- Opens LocationFormModal in inline mode
- After location created, auto-select in asset form dropdown

**UX Flow**:
1. User clicks "Create New Location" in AssetForm
2. LocationFormModal opens as nested modal
3. User fills location details (can use scanner!)
4. Location created successfully
5. LocationFormModal closes
6. Asset form dropdown refreshes
7. Newly created location auto-selected

**Modal Behavior**:
- Nested modal appears above asset modal (higher z-index)
- Asset modal remains visible but dimmed
- Cancel location creation returns to asset form unchanged
- Success returns with new location selected

---

## Validation Criteria

### Unit Tests
- [ ] useScanToInput hook correctly switches modes
- [ ] useScanToInput auto-stops after scan when autoStop=true
- [ ] useScanToInput continues scanning when autoStop=false
- [ ] useScanToInput returns to custom returnMode
- [ ] useScanToInput cleans up on unmount
- [ ] AssetForm scanner buttons only show when connected
- [ ] LocationForm scanner buttons only show when connected

### Integration Tests
- [ ] Scan RFID tag → Identifier field populated in AssetForm
- [ ] Scan barcode → Identifier field populated in LocationForm
- [ ] Cancel scan → Returns to IDLE, no value populated
- [ ] Create asset from inventory tag → Modal opens with EPC pre-filled
- [ ] Edit asset from inventory tag → Modal opens with asset data loaded
- [ ] Select location in asset form → location_id saved correctly
- [ ] Create location from asset form → New location appears in dropdown

### E2E Tests (Playwright)
- [ ] Full workflow: Scan tag → Create asset → Assign location → Verify
- [ ] Scanner buttons appear when device connected
- [ ] Scanner buttons hidden when device disconnected
- [ ] RFID scan populates identifier correctly
- [ ] Barcode scan populates identifier correctly
- [ ] Create asset from inventory → Asset appears in assets list
- [ ] Edit asset from inventory → Changes persist
- [ ] Nested modal flow: Create location while creating asset

### Manual Testing
- [ ] Connect device → Scanner buttons appear
- [ ] Disconnect device → Scanner buttons disappear
- [ ] Click RFID button → Device switches to INVENTORY mode
- [ ] Click Barcode button → Device switches to BARCODE mode
- [ ] Scan tag → Identifier auto-fills and scan stops
- [ ] Click Cancel during scan → Returns to IDLE
- [ ] Create asset from unlinked inventory tag → Works correctly
- [ ] Edit asset from linked inventory tag → Loads correct data
- [ ] Location dropdown shows hierarchical paths clearly
- [ ] Create location from asset form → Inline modal works smoothly

---

## Technical Constraints

### Dependencies
- useScanToInput hook (COMPLETED ✅)
- useTagStore, useBarcodeStore, useDeviceStore (existing)
- DeviceManager with mode switching (existing)
- Asset and Location CRUD APIs (existing)
- Asset enrichment on RFID reads (existing from recent PR)

### Browser Support
- Device connection via Web Bluetooth or BLE MCP
- React state management with Zustand
- Modal z-index layering for nested modals

### Performance
- Location dropdown lazy-loads only when opened
- Asset lookup by ID is O(1) from assetStore cache
- Scanner mode switches should be debounced

---

## Implementation Notes

### Scanner Reusability
- useScanToInput hook is the single source of truth
- No scanning logic duplicated in components
- Same hook works for any form field
- Future forms (e.g., User, Category) can reuse immediately

### Asset-Location Relationship
- Optional relationship (location_id nullable)
- Assets can exist without locations
- Locations can have multiple assets
- Future: Location screen shows asset count

### Inventory → Asset Workflow
- Seamless transition from scanning to asset management
- Pre-filled identifier reduces data entry errors
- Edit flow allows immediate correction of asset details
- Asset enrichment updates inventory display in real-time

### Nested Modal Pattern
- LocationFormModal can be used standalone or nested
- Z-index layering: Base modal (1000) → Nested modal (1100)
- Backdrop click only closes topmost modal
- Success/cancel callbacks handle data flow between modals

---

## Success Criteria

✅ **Functionality**:
- Scanner buttons work in Asset and Location forms
- RFID and Barcode scanning populate identifier fields correctly
- Inventory tags have Create/Edit Asset actions
- Assets can be assigned to locations
- Location can be created from asset form

✅ **User Experience**:
- Scanning is faster than typing
- Clear visual feedback during scanning
- No duplicate code or logic
- Smooth nested modal transitions
- Intuitive button placement and styling

✅ **Testing**:
- All unit tests pass
- All integration tests pass
- E2E tests cover full workflows
- Manual testing scenarios verified

✅ **Code Quality**:
- Single scanner hook (no duplication)
- Type-safe TypeScript throughout
- Follows existing patterns (stores, hooks, modals)
- Clear separation of concerns

---

## Definition of Done

### Phase 1: Scanner Foundation (COMPLETED ✅)
- [x] useScanToInput hook created
- [x] useScanToInput tests written and passing
- [x] Hook committed to feature branch

### Phase 2: Form Scanner Integration
- [ ] AssetForm scanner integration complete
- [ ] LocationForm scanner integration complete
- [ ] Scanner buttons styled consistently
- [ ] Scanning state feedback implemented
- [ ] Unit tests for form scanner behavior

### Phase 3: Inventory → Asset Workflow
- [ ] InventoryTableRow "Create Asset" button added
- [ ] InventoryTableRow "Edit Asset" button added
- [ ] AssetFormModal opens with pre-filled data
- [ ] Asset enrichment updates after create/edit
- [ ] Integration tests for inventory workflows

### Phase 4: Asset-Location Linking
- [ ] Backend: location_id added to Asset schema
- [ ] Backend: Migration file created
- [ ] Frontend: location_id added to Asset types
- [ ] AssetForm: Location selector dropdown added
- [ ] AssetForm: "Create New Location" quick action
- [ ] LocationFormModal: Nested modal support
- [ ] Integration tests for location assignment

### Phase 5: Testing & Documentation
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] E2E tests written and passing
- [ ] Manual testing completed
- [ ] Code reviewed
- [ ] README updated (if needed)
- [ ] PR created and merged

---

## Future Enhancements (Not in Scope)

- Bulk asset creation from inventory (select multiple tags)
- Location screen shows asset count and list
- Asset search by location filter
- Location tree view in asset selector
- Scanner shortcuts (keyboard shortcuts to trigger scan)
- Audio feedback for successful scan
- Barcode camera scanning (in addition to hardware scanner)
