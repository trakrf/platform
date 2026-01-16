# Implementation Plan: Disable EPC Validation

Generated: 2026-01-16
Specification: spec.md

## Understanding

Disable the EPC validation warnings added in TRA-271/PR #116 without removing the code structure. The validation rules (hex-only, min 24 chars, divisible by 8) are too restrictive for real-world use. We'll add a `ENABLE_EPC_VALIDATION = false` constant to disable warnings while preserving the logic for future re-enablement.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/BarcodeScreen.tsx` (lines 17-31) - Current `validateEPC` function
- `frontend/src/components/assets/TagIdentifierInputRow.tsx` (lines 9-24) - Duplicate `validateEPC` function
- `frontend/src/components/BarcodeScreen.test.tsx` (lines 43-121) - 6 validation tests to invert

**Files to Modify**:
- `frontend/src/components/BarcodeScreen.tsx` - Add constant, wrap validation call
- `frontend/src/components/assets/TagIdentifierInputRow.tsx` - Add constant, wrap validation call
- `frontend/src/components/BarcodeScreen.test.tsx` - Invert 6 tests to expect NO warnings

## Architecture Impact
- **Subsystems affected**: Frontend UI only
- **New dependencies**: None
- **Breaking changes**: None (warnings simply won't appear)

## Task Breakdown

### Task 1: Disable validation in BarcodeScreen.tsx

**File**: `frontend/src/components/BarcodeScreen.tsx`
**Action**: MODIFY

**Implementation**:
```typescript
// Add after imports, before validateEPC function (around line 11)

// DISABLED: TRA-271 feedback - rules too restrictive. Re-evaluate before enabling.
const ENABLE_EPC_VALIDATION = false;

// Modify line 250 (inside map callback):
// FROM:
const warning = validateEPC(barcode.data);
// TO:
const warning = ENABLE_EPC_VALIDATION ? validateEPC(barcode.data) : undefined;
```

**Validation**:
```bash
just frontend lint
just frontend typecheck
```

---

### Task 2: Disable validation in TagIdentifierInputRow.tsx

**File**: `frontend/src/components/assets/TagIdentifierInputRow.tsx`
**Action**: MODIFY

**Implementation**:
```typescript
// Add after imports, before validateEPC function (around line 3)

// DISABLED: TRA-271 feedback - rules too restrictive. Re-evaluate before enabling.
const ENABLE_EPC_VALIDATION = false;

// Modify line 58 (inside component):
// FROM:
const warning = validateEPC(value);
// TO:
const warning = ENABLE_EPC_VALIDATION ? validateEPC(value) : undefined;
```

**Validation**:
```bash
just frontend lint
just frontend typecheck
```

---

### Task 3: Invert test expectations

**File**: `frontend/src/components/BarcodeScreen.test.tsx`
**Action**: MODIFY

**Implementation**:
Update 6 tests to verify NO warnings appear (proving disable works):

1. **Line 53-61**: `shows no warning for valid 24-char hex EPC` - Keep as-is (already expects no warning)

2. **Line 63-75**: `shows no warning for valid 32-char hex EPC` - Keep as-is (already expects no warning)

3. **Line 77-87**: `shows warning for truncated EPC` → Rename and invert:
   ```typescript
   it('shows NO warning for truncated EPC (validation disabled)', () => {
     // same setup...
     expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
   });
   ```

4. **Line 89-99**: `shows warning for non-hex characters` → Rename and invert:
   ```typescript
   it('shows NO warning for non-hex characters (validation disabled)', () => {
     // same setup...
     expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
   });
   ```

5. **Line 101-111**: `shows warning for non-aligned length` → Rename and invert:
   ```typescript
   it('shows NO warning for non-aligned length (validation disabled)', () => {
     // same setup...
     expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
   });
   ```

6. **Line 113-120**: `does not block Locate button when warning is shown` → Update:
   ```typescript
   it('Locate button works when validation is disabled', () => {
     // same setup...
     expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
     expect(screen.getByTestId('locate-button')).toBeEnabled();
   });
   ```

**Validation**:
```bash
just frontend test
```

---

### Task 4: Final validation

**Action**: VALIDATE

Run full validation suite:
```bash
just frontend validate
```

Verify:
- All 6 tests pass with inverted expectations
- No lint errors
- No type errors
- Build succeeds

## Risk Assessment

- **Risk**: Tests may have additional assertions that need updating
  **Mitigation**: Read each test carefully, update all assertions to expect no warnings

- **Risk**: Other components might import validateEPC
  **Mitigation**: Already verified - only these 2 files use the inline function

## Integration Points

- No store updates needed
- No route changes needed
- No config updates needed

## VALIDATION GATES (MANDATORY)

After EVERY task:
1. `just frontend lint` - Must pass
2. `just frontend typecheck` - Must pass
3. `just frontend test` - Must pass (after Task 3)

**Final gate**: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Already read all files to modify
✅ Simple pattern (constant + conditional)
✅ No external dependencies
✅ No architectural changes
✅ Tests are straightforward to invert

**Assessment**: Straightforward disable-via-flag implementation with high confidence.

**Estimated one-pass success probability**: 95%

**Reasoning**: All code locations identified, pattern is simple, no complex interactions. Only risk is missing an assertion in tests.
