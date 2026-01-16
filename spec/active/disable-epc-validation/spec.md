# Feature: Disable EPC Validation

## Origin
This specification emerged from TRA-271 testing feedback. Tim reported that the EPC validation rules are too restrictive for real-world use. The feature needs to be disabled while we rethink what validations would actually be helpful.

## Outcome
EPC validation warnings will be disabled in the UI without removing the code structure, making it easy to re-enable once proper validation rules are determined.

## User Story
As an **RFID operator**
I want **to scan tags without restrictive validation warnings**
So that **I can complete my workflow without false-positive warnings blocking my work**

## Context
**Discovery**: TRA-271 added 3 validation rules for scanned EPC values:
1. Hex characters only - catches corruption
2. Minimum 24 chars - catches BLE truncation
3. Divisible by 8 - catches word boundary issues

**Problem**: Real-world testing revealed these rules are too restrictive. Users are seeing warnings on valid use cases that we didn't anticipate.

**Solution**: Disable validation via a constant flag. Code structure remains for future re-enablement.

## Technical Requirements

### Files to Modify
1. **`frontend/src/components/BarcodeScreen.tsx`**
   - Add `const ENABLE_EPC_VALIDATION = false;` constant
   - Wrap validation call with flag check

2. **`frontend/src/components/assets/TagIdentifierInputRow.tsx`**
   - Add same `ENABLE_EPC_VALIDATION = false;` constant
   - Wrap validation call with flag check

3. **`frontend/src/components/BarcodeScreen.test.tsx`**
   - Update 6 validation tests to expect no warnings when disabled
   - OR skip validation tests while flag is false

### Implementation Pattern
```typescript
// Add constant at top of file
const ENABLE_EPC_VALIDATION = false;

// Modify validation call
const warning = ENABLE_EPC_VALIDATION ? validateEPC(data) : undefined;
```

### What NOT to Change
- `settingsValidation.ts` - Already has relaxed validation, used for hardware filtering
- Keep `validateEPC` function code intact - we want to re-enable later

## Validation Criteria
- [ ] No EPC warnings appear in BarcodeScreen
- [ ] No EPC warnings appear in TagIdentifierInputRow
- [ ] Locate button remains functional
- [ ] All existing tests pass (or validation tests are skipped)
- [ ] `pnpm validate` passes

## Conversation References
- **Original issue**: TRA-271 - Warn on non-standard EPC length scan
- **Feedback**: "Rules are too restrictive" - Tim's testing feedback
- **Decision**: Disable via constant, rethink validations later
- **PR reference**: PR #116 (merged) contains the original implementation

## Future Considerations
When re-enabling validation, consider:
- What real-world EPC formats are users actually scanning?
- Are there formats shorter than 24 chars that are valid?
- Should validation be configurable per-organization?
- Should warnings be less alarming (info vs warning)?
