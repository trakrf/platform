# Feature: EPC Validation Warning (TRA-271)

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: [TRA-271](https://linear.app/trakrf/issue/TRA-271/warn-on-non-standard-epc-length-scan-24-chars)

## Outcome
Users see a warning when a scanned EPC value is invalid (too short, wrong length alignment, or non-hex characters), making BLE truncation and corruption failures visible instead of silently storing bad data.

## User Story
As a warehouse operator scanning RFID tags
I want to be warned when a scan appears incomplete
So that I can retry the scan or manually verify/enter the value before it corrupts my inventory data

## Context

**Current**: BLE MTU limitations (20-byte MTU with ~10 bytes CS108 header overhead) cause intermittent truncation of 24-character (96-bit) EPC values when scanning via CS108. When truncation occurs, incomplete data is silently stored without any user awareness. This leads to inventory data quality issues.

**Desired**: Display a visual warning on any scanned barcode/EPC where `data.length < 24`. The warning should:
- Be immediately visible in the scan results list
- Not block saving (user can verify and proceed)
- Suggest "Scan may be incomplete - try again or enter manually"

**Related**: TRA-270 tracks the root cause (BLE packet fragmentation) which requires deeper investigation. This warning is a quick defensive measure until proper BLE packet reassembly is implemented.

**Examples**: Barcode list rendering in `frontend/src/components/BarcodeScreen.tsx:229-257`

## Technical Requirements

### Core Implementation
- Add EPC validation check in BarcodeScreen component
- Display warning indicator (icon + text) for invalid EPCs
- Warning should be visually distinct (yellow/amber styling)

### Validation Rules
| Rule | Check | Warning Text |
|------|-------|--------------|
| Minimum length | `length < 24` | "Scan may be incomplete - try again or enter manually" |
| Word boundary | `length % 8 !== 0` | "Invalid EPC length - must be divisible by 8" |
| Hex characters | `!/^[0-9A-Fa-f]+$/.test(data)` | "Invalid characters detected - try again or enter manually" |

**Validation logic** (pseudocode):
```typescript
function validateEPC(data: string): { valid: boolean; warning?: string } {
  // Check hex characters first (catches corruption)
  if (!/^[0-9A-Fa-f]+$/.test(data)) {
    return { valid: false, warning: "Invalid characters detected - try again or enter manually" };
  }
  // Check minimum length (96-bit standard)
  if (data.length < 24) {
    return { valid: false, warning: "Scan may be incomplete - try again or enter manually" };
  }
  // Check 32-bit word boundary alignment
  if (data.length % 8 !== 0) {
    return { valid: false, warning: "Invalid EPC length - must be divisible by 8" };
  }
  return { valid: true };
}
```

### Data Flow
```
BarcodeScreen renders barcode list
  → For each barcode, run validateEPC(barcode.data)
  → If invalid, render warning indicator with appropriate message
```

### Files to Modify
| File | Change |
|------|--------|
| `frontend/src/components/BarcodeScreen.tsx` | Add warning display in barcode list rendering |

### UI Behavior
- Warning appears inline with the affected scan entry
- Does NOT prevent saving or using the value
- Does NOT prevent "Locate" button functionality
- User can still export truncated values to CSV (their choice)

### Non-Requirements (Explicit)
- Do NOT block saves or prevent usage of short values
- Do NOT add validation at the worker/handler level (keep it UI-only for now)
- Do NOT modify the BarcodeStore or BarcodeData interface
- Do NOT add toast notifications (keep warning inline with the scan entry)

## Validation Criteria
- [ ] Valid 24-char hex EPC shows no warning (e.g., `E20034120000000022440401`)
- [ ] Valid 32-char hex EPC shows no warning (128-bit)
- [ ] Truncated EPC (< 24 chars) shows "incomplete" warning
- [ ] Non-aligned length (e.g., 25 chars) shows "divisible by 8" warning
- [ ] Non-hex characters show "invalid characters" warning
- [ ] Warning does not block "Locate" button
- [ ] Warning does not block CSV export
- [ ] Warning displays immediately when scan appears in list

## Success Metrics
- [ ] 100% of invalid EPCs display appropriate warning
- [ ] Zero false positives on valid hex EPCs with correct length
- [ ] No functional regression in barcode scanning flow
- [ ] All existing barcode tests pass
- [ ] Manual verification with CS108 hardware confirms warning appears on truncated reads

## References
- Linear issue: [TRA-271](https://linear.app/trakrf/issue/TRA-271/warn-on-non-standard-epc-length-scan-24-chars)
- Related root cause: [TRA-270](https://linear.app/trakrf/issue/TRA-270/barcodeqr-reads-occasionally-drop-characters-from-the-end-of-the-value)
- BarcodeScreen component: `frontend/src/components/BarcodeScreen.tsx`
- Barcode store: `frontend/src/stores/barcodeStore.ts`
- Barcode scan handler: `frontend/src/worker/cs108/barcode/scan-handler.ts`
