# E2E Test Strategy

## Key Learnings from Connection Test Refactoring

### 1. Mode Switching is Expensive
- Navigating to Inventory/Locate/Barcode tabs triggers heavyweight mode changes
- Each mode switch involves multiple CS108 commands and state transitions
- Mode switches during connection setup can cause timeouts

### 2. Proper Connection Sequence
**DO THIS:**
1. Navigate to Home tab (`/` or `/?tab=home`)
2. Connect to device (establishes IDLE mode)
3. Navigate to target tab if needed (triggers mode switch)

**NOT THIS:**
1. Navigate to target tab (`/?tab=inventory`)
2. Connect to device (mode switch conflicts with connection)

### 3. Tab-to-Mode Mapping
- **Home/Settings/Help tabs** → IDLE mode (lightweight, no scanning)
- **Inventory tab** → INVENTORY mode (RFID scanning)
- **Locate tab** → LOCATE mode (RFID location with RSSI)
- **Barcode tab** → BARCODE mode (barcode scanning)

### 4. Test Suite Organization
Each test suite should:
- Connect ONCE in `beforeAll` from Home tab
- Navigate to target tab AFTER connection established
- Stay in that mode for all tests in the suite
- Disconnect ONCE in `afterAll`

### 5. Connection Test Focus
Connection tests should ONLY verify:
- Connection establishment
- Battery level reporting
- Trigger state changes
- Basic navigation between IDLE-mode tabs (Home/Settings)

They should NOT:
- Navigate to scanning tabs (Inventory/Locate/Barcode)
- Test mode-specific functionality
- Trigger actual scanning operations

### 6. Performance Impact
- Simplified connection tests: ~14.5 seconds
- Tests with mode switching during connect: timeout after 30+ seconds
- Bridge stability degrades with repeated full test runs

## Implementation Checklist

### ✅ Fixed: connection.spec.ts
- Removed navigation to scanning tabs
- Focused on core connection concerns
- Tests run reliably in ~14.5 seconds

### ⚠️ Need Fixing: inventory.spec.ts
- Currently navigates to inventory tab before connecting
- Should connect from Home, then navigate to inventory

### ⚠️ Need Fixing: locate.spec.ts
- Currently navigates to locate tab before connecting
- Should connect from Home, then navigate to locate

### ✅ Already Good: barcode.spec.ts
- Appears to follow correct pattern (needs verification)

## Test Isolation Strategy

To avoid killing the bridge server:
1. Fix and test ONE spec file at a time
2. Run individual tests: `pnpm test:e2e tests/e2e/[specific].spec.ts`
3. Only run full suite after all individual tests pass
4. Consider adding delays between test suites if bridge instability persists