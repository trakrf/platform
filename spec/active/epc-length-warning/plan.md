# Implementation Plan: EPC Validation Warning (TRA-271)

Generated: 2026-01-15
Specification: spec.md

## Understanding

Add inline warnings to the BarcodeScreen component when scanned EPC values fail validation. Three validation rules apply in order:
1. Hex characters only (`/^[0-9A-Fa-f]+$/`)
2. Minimum length (24 characters for 96-bit EPC)
3. Word boundary alignment (length divisible by 8)

Warnings are non-blocking - users can still use "Locate" and export CSV with invalid data.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/locations/LocationMoveModal.tsx` (lines 104-111) - Warning banner styling pattern
- `frontend/src/components/shared/empty-states/EmptyState.tsx` (lines 30-35) - Yellow/warning color classes
- `frontend/src/components/AcceptInviteScreen.tsx` (lines 235-236) - AlertTriangle icon usage

**Files to Create**:
- `frontend/src/components/BarcodeScreen.test.tsx` - Unit tests for EPC validation and warning display

**Files to Modify**:
- `frontend/src/components/BarcodeScreen.tsx` - Add validation function and warning UI

## Architecture Impact
- **Subsystems affected**: UI only (BarcodeScreen component)
- **New dependencies**: None (AlertTriangle already available from lucide-react)
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add AlertTriangle import
**File**: `frontend/src/components/BarcodeScreen.tsx`
**Action**: MODIFY
**Pattern**: Reference `AcceptInviteScreen.tsx` line 236

**Implementation**:
```typescript
// Add to existing lucide-react import (line 7)
import { Target, AlertTriangle } from 'lucide-react';
```

**Validation**: `just frontend typecheck`

---

### Task 2: Add validateEPC helper function
**File**: `frontend/src/components/BarcodeScreen.tsx`
**Action**: MODIFY
**Location**: After imports, before component definition (~line 12)

**Implementation**:
```typescript
/**
 * Validates EPC data for common issues caused by BLE truncation or corruption.
 * Returns warning message if invalid, undefined if valid.
 */
function validateEPC(data: string): string | undefined {
  // Check hex characters first (catches corruption)
  if (!/^[0-9A-Fa-f]+$/.test(data)) {
    return "Invalid characters detected - try again or enter manually";
  }
  // Check minimum length (96-bit standard)
  if (data.length < 24) {
    return "Scan may be incomplete - try again or enter manually";
  }
  // Check 32-bit word boundary alignment
  if (data.length % 8 !== 0) {
    return "Invalid EPC length - must be divisible by 8";
  }
  return undefined;
}
```

**Validation**: `just frontend typecheck`

---

### Task 3: Add warning display in barcode list rendering
**File**: `frontend/src/components/BarcodeScreen.tsx`
**Action**: MODIFY
**Location**: Inside barcode map (~lines 229-255)
**Pattern**: Reference `LocationMoveModal.tsx` lines 104-111 for styling

**Implementation**:
Add warning display after the barcode data div (line 235), before the metadata div:

```typescript
{barcodes.map((barcode, index) => {
  const warning = validateEPC(barcode.data);
  return (
    <div
      key={`${barcode.timestamp}-${index}`}
      data-testid={`barcode-${barcode.data}`}
      className="p-3 hover:bg-gray-50 dark:hover:bg-gray-700"
    >
      <div className="font-medium break-all text-gray-900 dark:text-gray-100">{barcode.data}</div>
      {warning && (
        <div
          data-testid="epc-warning"
          className="flex items-center gap-1.5 mt-1 text-xs text-yellow-700 dark:text-yellow-400"
        >
          <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0" />
          <span>{warning}</span>
        </div>
      )}
      <div className="flex justify-between items-center text-sm text-gray-500 dark:text-gray-400 mt-1">
        {/* ... rest unchanged ... */}
      </div>
    </div>
  );
})}
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 4: Create unit tests
**File**: `frontend/src/components/BarcodeScreen.test.tsx`
**Action**: CREATE
**Pattern**: Reference existing test patterns in `frontend/src/components/__tests__/`

**Implementation**:
```typescript
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import BarcodeScreen from './BarcodeScreen';
import { useBarcodeStore } from '@/stores/barcodeStore';

// Test the validateEPC function behavior through the component
describe('BarcodeScreen EPC Validation', () => {
  beforeEach(() => {
    // Reset store state
    useBarcodeStore.setState({ barcodes: [], scanning: false });
  });

  it('shows no warning for valid 24-char hex EPC', () => {
    useBarcodeStore.setState({
      barcodes: [{ data: 'E20034120000000022440401', type: 'EPC', timestamp: Date.now() }]
    });
    render(<BarcodeScreen />);
    expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
  });

  it('shows no warning for valid 32-char hex EPC', () => {
    useBarcodeStore.setState({
      barcodes: [{ data: 'E2003412000000002244040112345678', type: 'EPC', timestamp: Date.now() }]
    });
    render(<BarcodeScreen />);
    expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
  });

  it('shows warning for truncated EPC (< 24 chars)', () => {
    useBarcodeStore.setState({
      barcodes: [{ data: 'E200341200000000', type: 'EPC', timestamp: Date.now() }]
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toHaveTextContent('Scan may be incomplete');
  });

  it('shows warning for non-hex characters', () => {
    useBarcodeStore.setState({
      barcodes: [{ data: 'E20034120000GHIJ22440401', type: 'EPC', timestamp: Date.now() }]
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toHaveTextContent('Invalid characters detected');
  });

  it('shows warning for non-aligned length (25 chars)', () => {
    useBarcodeStore.setState({
      barcodes: [{ data: 'E200341200000000224404012', type: 'EPC', timestamp: Date.now() }]
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toHaveTextContent('must be divisible by 8');
  });

  it('does not block Locate button when warning is shown', () => {
    useBarcodeStore.setState({
      barcodes: [{ data: 'E200341200', type: 'EPC', timestamp: Date.now() }]
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toBeInTheDocument();
    expect(screen.getByTestId('locate-button')).toBeEnabled();
  });
});
```

**Validation**: `just frontend test`

---

### Task 5: Final validation
**Action**: VERIFY
**Commands**:
```bash
just frontend lint
just frontend typecheck
just frontend test
just frontend build
```

## Risk Assessment

- **Risk**: Test mocking complexity for Zustand stores
  **Mitigation**: Use `useBarcodeStore.setState()` directly to set test state (pattern from existing tests)

- **Risk**: Dark mode styling inconsistency
  **Mitigation**: Follow existing pattern from LocationMoveModal with explicit dark: variants

## Integration Points
- Store updates: None (read-only from barcodeStore)
- Route changes: None
- Config updates: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `just frontend test`

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task: `just frontend lint && just frontend typecheck`
After Task 4: `just frontend test`
Final validation: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar warning patterns found at `LocationMoveModal.tsx:104-111`
✅ All clarifying questions answered
✅ Existing test patterns to follow
✅ Single file modification (low risk)
✅ No external dependencies

**Assessment**: Straightforward UI enhancement with clear patterns to follow.

**Estimated one-pass success probability**: 95%

**Reasoning**: Well-defined scope, existing patterns for warning styling, inline implementation keeps complexity low, comprehensive test coverage defined.
