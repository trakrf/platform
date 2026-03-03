# Implementation Plan: Barcode Scan Duration Control
Generated: 2026-03-03
Specification: spec.md

## Understanding

The barcode laser shuts off in under a second because the current start command (`BARCODE_ESC_TRIGGER = 0x1b 0x30`) is likely "trigger stop" per both Newland and CS108 specs. The CS108 spec (Appendix D.2) recommends using `0x1b 0x33` (continuous reading) to start and `0x1b 0x30` to stop. The fix is to use the correct commands and add a 3-second software timeout for button-initiated scans.

## Relevant Files

**Reference Patterns**:
- `frontend/src/worker/cs108/event.ts` (lines 193-199) - Current ESC command constants
- `frontend/src/worker/cs108/barcode/sequences.ts` (lines 38-57) - Current start/stop sequences
- `frontend/src/components/BarcodeScreen.tsx` (lines 108-152) - Current auto-stop and trigger handling

**Files to Modify**:
- `frontend/src/worker/cs108/event.ts` (lines 193-199) - Fix ESC command constants and comments
- `frontend/src/worker/cs108/barcode/sequences.ts` (lines 7-11, 38-44, 51-57) - Update start/stop to use correct commands
- `frontend/src/components/BarcodeScreen.tsx` (lines 108-120) - Add 3-second button timeout

**No files to create.**

## Architecture Impact
- **Subsystems affected**: Frontend worker (CS108 protocol), UI (BarcodeScreen)
- **New dependencies**: None
- **Breaking changes**: Changes the fundamental barcode start/stop commands. All barcode scanning flows will use the new commands (BarcodeScreen button, BarcodeScreen trigger, useScanToInput trigger). This is intentional - the old commands were wrong.

## Task Breakdown

### Task 1: Fix ESC Command Constants in event.ts
**File**: `frontend/src/worker/cs108/event.ts`
**Action**: MODIFY (lines 193-199)

Replace the incorrect/misleading ESC constants with correct ones per CS108 spec Appendix D:

```typescript
// Barcode ESC command constants
// Per CS108 BLE API Spec Appendix D.2 and Newland Serial Programming Manual V1.3.0-2:
//   0x1B 0x30 = Stop scanning (Newland: "Trigger Stop" / CS108: stop)
//   0x1B 0x31 = Single trigger (Newland: "Analog Trigger", default 3000ms timeout)
//   0x1B 0x33 = Continuous reading (Newland: "Continuous Reading" / CS108: start)
export const BARCODE_ESC_START = new Uint8Array([0x1b, 0x33]);    // Continuous reading - stays active until stop
export const BARCODE_ESC_STOP = new Uint8Array([0x1b, 0x30]);     // Trigger stop - halts scanning
export const BARCODE_ESC_TRIGGER = new Uint8Array([0x1b, 0x31]);  // Single-shot analog trigger (legacy, not used)
export const BARCODE_ESC_CONTINUOUS = new Uint8Array([0x1b, 0x33]); // Alias for BARCODE_ESC_START
```

Key changes:
- `BARCODE_ESC_STOP` changes from `[0x1b, 0x31]` to `[0x1b, 0x30]` (correct stop per both specs)
- New `BARCODE_ESC_START` = `[0x1b, 0x33]` (continuous mode, the CS108-recommended start command)
- `BARCODE_ESC_TRIGGER` redefined as `[0x1b, 0x31]` (single-shot, kept for reference but unused)
- `BARCODE_ESC_CONTINUOUS` kept as alias for backward compat with any other references
- Comments updated to cite both specs accurately

**Validation**: `just frontend typecheck`

### Task 2: Update Barcode Sequences to Use Correct Commands
**File**: `frontend/src/worker/cs108/barcode/sequences.ts`
**Action**: MODIFY

Update imports and sequences to use the new constants:

**Import change** (line 6-11):
```typescript
import {
  BARCODE_POWER_ON,
  BARCODE_SEND_COMMAND,
  BARCODE_ESC_STOP,
  BARCODE_ESC_START,
} from '../event.js';
```

**Config sequence** (line 26-27): Change stop command to new `BARCODE_ESC_STOP`:
```typescript
    payload: BARCODE_ESC_STOP,  // Ensure scanner is stopped before configuration
```
This uses the corrected stop constant (`0x1b 0x30`). Same variable name, new byte value from Task 1.

**Start sequence** (line 40-41): Change from `BARCODE_ESC_TRIGGER` to `BARCODE_ESC_START`:
```typescript
    payload: BARCODE_ESC_START,
```
This sends `0x1b 0x33` (continuous mode) instead of `0x1b 0x30`.

**Stop sequence** (line 53-54): Already uses `BARCODE_ESC_STOP` - just verify the bytes are now correct from Task 1.

**Validation**: `just frontend typecheck`

### Task 3: Add 3-Second Button Timeout in BarcodeScreen
**File**: `frontend/src/components/BarcodeScreen.tsx`
**Action**: MODIFY

Add a 3-second auto-stop timer that only applies to button-initiated scans. The hardware trigger should NOT have this timeout.

**Add a ref for the button timeout** (after line 54):
```typescript
const buttonTimeoutRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
```

**Add a helper to clear the timeout**:
```typescript
const clearButtonTimeout = useCallback(() => {
  if (buttonTimeoutRef.current) {
    clearTimeout(buttonTimeoutRef.current);
    buttonTimeoutRef.current = null;
  }
}, []);
```

**Modify the SCAN_ONE auto-stop effect** (lines 108-120): When scanning stops (for any reason), clear the button timeout:
```typescript
useEffect(() => {
  if (scanMode === BarcodeScanMode.SCAN_ONE && barcodes.length > 0 && scanning) {
    console.debug('SCAN_ONE mode: Automatically stopping after successful scan');
    clearButtonTimeout();
    setTimeout(() => {
      stopBarcodeScan();
    }, 500);
  }
}, [barcodes.length, scanMode, scanning, stopBarcodeScan, clearButtonTimeout]);
```

**Start the 3-second timeout when button activates scanning**. Modify the button `onClick` handler (lines 211-215) to start a timer when the button is toggled ON:
```typescript
onClick={() => {
  const willActivate = !scanButtonActive;
  toggleScanButton();
  if (willActivate && scanMode === BarcodeScanMode.SCAN_ONE) {
    // Auto-stop after 3 seconds if no barcode read
    buttonTimeoutRef.current = setTimeout(() => {
      console.debug('[BarcodeScreen] Button scan timeout (3s) - auto-stopping');
      buttonTimeoutRef.current = null;
      // Only stop if still scanning
      if (useBarcodeStore.getState().scanning) {
        useBarcodeStore.getState().setScanning(false);
      }
      if (useDeviceStore.getState().scanButtonActive) {
        useDeviceStore.getState().toggleScanButton();
      }
    }, 3000);
  } else if (!willActivate) {
    clearButtonTimeout();
  }
}}
```

**Clear timeout on unmount** - update the existing cleanup effect (lines 57-65):
```typescript
useEffect(() => {
  return () => {
    if (useDeviceStore.getState().scanButtonActive) {
      console.debug('[BarcodeScreen] Unmounting - turning off scan button');
      useDeviceStore.setState({ scanButtonActive: false });
    }
    if (buttonTimeoutRef.current) {
      clearTimeout(buttonTimeoutRef.current);
      buttonTimeoutRef.current = null;
    }
  };
}, []);
```

**Trigger handler unchanged**: The trigger useEffect (lines 122-152) already starts on press and stops on release. With the corrected start command (continuous mode from Tasks 1-2), the laser will now stay on until trigger release. No timeout needed.

**Validation**: `just frontend typecheck && just frontend lint`

### Task 4: Verify No Other References to Old Constants
**Action**: SEARCH

Grep the codebase for any other imports of `BARCODE_ESC_TRIGGER` to ensure nothing else depends on the old byte values. Fix any found references.

Check for:
- `BARCODE_ESC_TRIGGER` - should only be in event.ts definition now (no longer imported in sequences.ts)
- `BARCODE_ESC_CONTINUOUS` - may be referenced elsewhere; it's now an alias for `BARCODE_ESC_START` so no breakage
- Direct references to `0x1b, 0x30` or `0x1b, 0x31` byte arrays

**Validation**: `just frontend typecheck && just frontend lint`

### Task 5: Full Validation
**Action**: VALIDATE

Run complete frontend validation:
```bash
just frontend validate
```

This runs lint + typecheck + test + build. All must pass.

## Risk Assessment

- **Risk**: Changing the fundamental start/stop byte values could break barcode scanning entirely if the spec interpretation is wrong.
  **Mitigation**: The CS108's own spec explicitly says to use `0x1b 0x33` to start and `0x1b 0x30` to stop (Appendix D.2). This matches Newland docs. The current behavior is already broken (under-1-second timeout). Test on hardware immediately after deploying.

- **Risk**: The `useScanToInput` hook calls `dm.setMode(ReaderMode.BARCODE)` which triggers the config+start sequence. It relies on the worker auto-stop and trigger release flow.
  **Mitigation**: useScanToInput doesn't directly reference ESC constants - it goes through DeviceManager → Worker → sequences. The sequence changes in Task 2 will automatically propagate. The 2-second cleanup timeout in useScanToInput (line 238) still applies as a safety net.

- **Risk**: The 3-second button timeout might conflict with the existing SCAN_ONE auto-stop (500ms after barcode read).
  **Mitigation**: The auto-stop effect (Task 3) clears the button timeout via `clearButtonTimeout()` before calling `stopBarcodeScan()`. Whichever fires first wins cleanly.

## Integration Points

- **Barcode store**: No changes needed. `setScanning(true/false)` API unchanged.
- **Device store**: No changes needed. `toggleScanButton()` API unchanged.
- **DeviceManager**: No changes needed. Routes through worker → sequences.
- **useScanToInput hook**: No changes needed. Uses DeviceManager.setMode() which triggers updated sequences.

## VALIDATION GATES (MANDATORY)

After EVERY task:
```bash
just frontend typecheck   # Gate 1: Types
just frontend lint         # Gate 2: Lint
```

After Task 5 (final):
```bash
just frontend validate    # All gates: lint + typecheck + test + build
```

If any gate fails: Fix → Re-run → Repeat until pass.

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- Clear requirements from spec and user clarification
- CS108 spec Appendix D explicitly documents the correct commands
- Changes are minimal: 3 files, ~30 lines of actual code changes
- No new dependencies, no new patterns
- Existing auto-stop and trigger handling largely unchanged
- `useScanToInput` integration is transparent (no direct constant references)

**Assessment**: Straightforward fix to incorrect ESC command bytes, plus a simple UI timer. High confidence in approach since it directly follows the hardware vendor's own documentation.

**Estimated one-pass success probability**: 90%

**Reasoning**: The only uncertainty is whether the CS108 hardware truly responds as its own spec documents (since the existing code comment claims "inversion"). Hardware testing will confirm immediately. The software changes are minimal and well-understood.
