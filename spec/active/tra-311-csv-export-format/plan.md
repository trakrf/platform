# Implementation Plan: TRA-311 - Align Asset Export CSV Format

Generated: 2026-01-22
Specification: spec.md

## Understanding

Modify `generateAssetCSV()` to output Tag IDs in separate rightmost columns instead of a single semicolon-delimited cell. Column order changes to: Asset ID, Name, Description, Status, Created, Location, Tag ID... (with "Type" column removed). Always include at least 1 Tag ID column, extending rightward based on max tags across all assets.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/utils/export/assetExport.ts` (lines 163-187) - current CSV generation pattern
- `frontend/src/utils/export/assetExport.test.ts` - existing test structure to update

**Files to Modify**:
- `frontend/src/utils/export/assetExport.ts` - `generateAssetCSV()` function
- `frontend/src/utils/export/assetExport.test.ts` - update tests for new format

## Architecture Impact
- **Subsystems affected**: Frontend export utilities only
- **New dependencies**: None
- **Breaking changes**: CSV format changes (intentional - enables reconciliation workflow)

## Task Breakdown

### Task 1: Update generateAssetCSV() column order and tag handling
**File**: `frontend/src/utils/export/assetExport.ts`
**Action**: MODIFY
**Pattern**: Existing CSV generation at lines 163-187

**Implementation**:
```typescript
export function generateAssetCSV(assets: Asset[]): ExportResult {
  // Calculate max tag count (minimum 1)
  const maxTags = Math.max(1, ...assets.map(a => a.identifiers?.length || 0));

  // Build headers: fixed columns + repeated "Tag ID" columns
  const fixedHeaders = ['Asset ID', 'Name', 'Description', 'Status', 'Created', 'Location'];
  const tagHeaders = Array(maxTags).fill('Tag ID');
  const headers = [...fixedHeaders, ...tagHeaders];

  let content = headers.join(',') + '\n';

  assets.forEach((asset) => {
    // Fixed columns in new order
    const fixedCols = [
      `"${asset.identifier}"`,
      `"${(asset.name || '').replace(/"/g, '""')}"`,
      `"${(asset.description || '').replace(/"/g, '""')}"`,
      asset.is_active ? 'Active' : 'Inactive',
      asset.created_at ? new Date(asset.created_at).toLocaleDateString() : '',
      `"${getLocationName(asset.current_location_id).replace(/"/g, '""')}"`,
    ];

    // Tag columns - one per column, pad with empty if fewer tags
    const tagCols = Array(maxTags).fill('').map((_, i) => {
      const tag = asset.identifiers?.[i]?.value || '';
      return tag ? `"${tag.replace(/"/g, '""')}"` : '';
    });

    content += [...fixedCols, ...tagCols].join(',') + '\n';
  });

  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  return {
    blob,
    filename: `assets_${getDateString()}.csv`,
    mimeType: 'text/csv',
  };
}
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

### Task 2: Update CSV tests for new format
**File**: `frontend/src/utils/export/assetExport.test.ts`
**Action**: MODIFY

**Changes needed**:
1. Update `includes correct headers` test - remove 'Type', add column order check
2. Update `includes asset data in rows` test - remove 'device' type check
3. Add new test: `separates multiple tags into columns`
4. Add new test: `includes minimum one Tag ID column even with no tags`
5. Add new test: `correct column order`

**Validation**:
```bash
cd frontend && just test
```

### Task 3: Run full validation
**Action**: VALIDATE

```bash
cd frontend && just validate
```

### Task 4: Manual verification
**Action**: VERIFY

Export a test CSV and verify:
- Column order: Asset ID, Name, Description, Status, Created, Location, Tag ID...
- Multi-tag assets have tags in separate columns
- Header repeats "Tag ID" for each tag column
- Empty Tag ID columns for assets with fewer tags

## Risk Assessment

- **Risk**: Test mocks may need adjustment for new column expectations
  **Mitigation**: Tests use actual function output, just need assertion updates

- **Risk**: Edge case with all assets having 0 tags
  **Mitigation**: `Math.max(1, ...)` ensures minimum 1 Tag ID column

## Integration Points
- Store updates: None (read-only from location store)
- Route changes: None
- Config updates: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd frontend && just lint      # Gate 1: Syntax & Style
cd frontend && just typecheck # Gate 2: Type Safety
cd frontend && just test      # Gate 3: Unit Tests
```

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task:
```bash
cd frontend && just validate
```

Final validation:
```bash
just validate  # Full stack from project root
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar pattern already exists in codebase (lines 163-187)
✅ All clarifying questions answered
✅ Existing test file to update
✅ No external dependencies
✅ Single file modification

**Assessment**: Straightforward refactor of existing CSV generation logic with clear target format.

**Estimated one-pass success probability**: 95%

**Reasoning**: Well-defined scope, existing patterns to follow, comprehensive test coverage already in place. Only risk is minor test assertion updates.
