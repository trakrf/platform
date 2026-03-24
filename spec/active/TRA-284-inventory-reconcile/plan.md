# Implementation Plan: TRA-284 Inventory Reconciliation with Multi-Tag Asset Support
Generated: 2026-03-23
Specification: spec.md

## Understanding

Update the reconciliation system to handle multi-tagged assets from CSV import. Currently, only the first "Tag ID" column is parsed and matching is tag-level. After this work: all Tag ID columns are parsed, an `assetIdentifier` links tags to their parent asset, stats show asset-level counts (Found/Missing/Unexpected), and inventory export matches the asset export column format for clean round-trip.

Also fixes a critical bug where RFID-scanned tags are never marked as "Found" due to incorrect source matching.

## Decisions

- **Asset stats replace tag stats** when reconciliation is active (not alongside)
- **Keep Description column** in inventory export for round-trip column parity
- **Both types**: `ReconciliationAsset` in store for asset-level aggregation + `ReconciliationItem` extended with `assetIdentifier` for tag-level detail
- **Unit tests only** — pure data transformations, no integration tests needed

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/utils/export/assetExport.ts` (lines 168-207) — multi-tag CSV column pattern (maxTags, repeated headers, rightward fill)
- `frontend/src/utils/export/assetExport.test.ts` — thorough CSV test patterns to mirror
- `frontend/src/stores/tagStore.ts` (lines 202-262) — merge logic pattern (Map-based, key normalization, pagination update)
- `frontend/src/stores/tagStore.test.ts` — Zustand store test patterns (act/state assertions)

**Files to Create**:
- `frontend/src/utils/reconciliationUtils.test.ts` — tests for CSV parsing, asset map building, stats
- `frontend/src/utils/excelExportUtils.test.ts` — tests for inventory CSV/Excel export

**Files to Modify**:
- `frontend/src/utils/reconciliationUtils.ts` — multi-tag parsing, `assetIdentifier`, `ReconciliationAsset` type, asset map builder
- `frontend/src/stores/tagStore.ts` (line 228) — source matching bug fix + pass `assetIdentifier` through merge
- `frontend/src/utils/excelExportUtils.ts` — realign inventory CSV/Excel columns to match asset export format
- `frontend/src/components/inventory/InventoryStats.tsx` — asset-level stat labels
- `frontend/src/components/InventoryScreen.tsx` (lines 155-170) — asset-level stats computation

## Architecture Impact
- **Subsystems affected**: Frontend only (utils, stores, components)
- **New dependencies**: None
- **Breaking changes**: Inventory CSV export column order changes (no production users per stack.md)

## Task Breakdown

### Task 1: Fix source matching bug in tagStore
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Single line fix

**Implementation**:
Line 228 — change:
```typescript
// FROM:
existing.reconciled = existing.source === 'scan' ? true : false;
// TO:
existing.reconciled = existing.source !== 'reconciliation';
```

**Why**: RFID-scanned tags have `source: 'rfid'`, not `'scan'`. The only tags that should NOT be marked Found are those with `source: 'reconciliation'` (stub entries from CSV import that haven't been scanned yet).

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 2: Add `assetIdentifier` to ReconciliationItem and add ReconciliationAsset type
**File**: `frontend/src/utils/reconciliationUtils.ts`
**Action**: MODIFY
**Pattern**: Extend existing interface (lines 7-16), add new type

**Implementation**:
```typescript
// Extend ReconciliationItem (line 7-16)
export interface ReconciliationItem {
  epc: string;
  originalEpc?: string;
  assetIdentifier?: string;  // NEW: "ASSET-0003" from CSV Asset ID column
  description?: string;
  location?: string;
  rssi?: number;
  count: number;
  found: boolean;
  lastSeen?: number;
}

// NEW type for asset-level aggregation
export interface ReconciliationAsset {
  assetIdentifier: string;
  tagIds: string[];
  name?: string;
  description?: string;
  location?: string;
  found: boolean;
  foundTagIds: string[];
}

// NEW: Build asset map from reconciliation items
export type TagToAssetMap = Map<string, ReconciliationAsset>;
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 3: Update parseReconciliationCSV for multi-tag columns and Asset ID
**File**: `frontend/src/utils/reconciliationUtils.ts`
**Action**: MODIFY
**Pattern**: Reference `assetExport.ts` lines 168-176 for multi-tag column detection

**Implementation**:

Update the column detection (lines 104-128) to:
1. Find ALL Tag ID column indices (not just first)
2. Find Asset ID column index (pattern: `/^asset\s*id$/i`)
3. For each data row, emit one `ReconciliationItem` per non-empty tag, all sharing the same `assetIdentifier`

```typescript
// Find ALL Tag ID columns
const tagIdColumnIndices: number[] = headers
  .map((h, i) => /^(epc|tag\s*id|rfid\s*(tag)?)$/i.test(h.trim()) ? i : -1)
  .filter(i => i !== -1);

// Find Asset ID column
const assetIdColumnIndex = headers
  .findIndex(h => /^asset\s*id$/i.test(h.trim()));

// For each row, create one ReconciliationItem per tag
for (const colIdx of tagIdColumnIndices) {
  const rawEpc = values[colIdx]?.trim();
  if (!rawEpc) continue;
  // ... normalize, validate, create ReconciliationItem with shared assetIdentifier
}
```

**Backward compatibility**: If no "Asset ID" column exists (old-format CSV), `assetIdentifier` is undefined — falls back to tag-level behavior. If only one Tag ID column, works exactly as before.

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 4: Add buildAssetMap and getAssetReconciliationStats functions
**File**: `frontend/src/utils/reconciliationUtils.ts`
**Action**: MODIFY
**Pattern**: Pure functions, similar to existing `getReconciliationStats()` (lines 274-284)

**Implementation**:
```typescript
// Build lookup: tag EPC → ReconciliationAsset (shared references)
export function buildAssetMap(items: ReconciliationItem[]): TagToAssetMap {
  const assetMap = new Map<string, ReconciliationAsset>();
  const tagToAsset = new Map<string, ReconciliationAsset>();

  for (const item of items) {
    if (!item.assetIdentifier) continue;

    let asset = assetMap.get(item.assetIdentifier);
    if (!asset) {
      asset = {
        assetIdentifier: item.assetIdentifier,
        tagIds: [],
        name: item.description,
        description: item.description,
        location: item.location,
        found: false,
        foundTagIds: [],
      };
      assetMap.set(item.assetIdentifier, asset);
    }
    asset.tagIds.push(item.epc);
    tagToAsset.set(item.epc, asset);
  }
  return tagToAsset;
}

// Asset-level stats for display
export function getAssetReconciliationStats(items: ReconciliationItem[]): {
  totalAssets: number;
  foundAssets: number;
  missingAssets: number;
} {
  // Group by assetIdentifier, asset is Found if ANY of its tags are found
  const assetStatus = new Map<string, boolean>();
  for (const item of items) {
    const key = item.assetIdentifier ?? item.epc; // fallback for no-asset items
    const current = assetStatus.get(key) ?? false;
    assetStatus.set(key, current || item.found);
  }
  const totalAssets = assetStatus.size;
  const foundAssets = [...assetStatus.values()].filter(Boolean).length;
  return { totalAssets, foundAssets, missingAssets: totalAssets - foundAssets };
}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 5: Update mergeReconciliationTags to pass assetIdentifier through
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Follow existing merge pattern (lines 226-250)

**Implementation**:

In the existing tag update block (line ~230 area), add:
```typescript
existing.assetIdentifier = item.assetIdentifier;
```

In the new tag creation block (line ~240 area), add:
```typescript
assetIdentifier: item.assetIdentifier,
```

**Note**: `TagInfo` already has `assetIdentifier?: string` (line 48) — no type change needed.

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 6: Update InventoryScreen stats computation for asset-level
**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY
**Pattern**: Replace tag-level counting (lines 155-170) with asset-level aggregation

**Implementation**:

Import `getAssetReconciliationStats` from reconciliationUtils. In the `useMemo` stats block:

```typescript
const stats = useMemo(() => {
  const hasReconciliation = filteredTags.some(tag =>
    tag.reconciled !== null && tag.reconciled !== undefined
  );

  if (hasReconciliation) {
    // Asset-level: group by assetIdentifier, Found if ANY tag found
    const reconItems: ReconciliationItem[] = filteredTags
      .filter(t => t.reconciled !== null && t.reconciled !== undefined)
      .map(t => ({
        epc: t.epc,
        assetIdentifier: t.assetIdentifier,
        found: t.reconciled === true,
        count: t.count,
      }));
    const assetStats = getAssetReconciliationStats(reconItems);
    const notListed = filteredTags.filter(t =>
      t.reconciled === null || t.reconciled === undefined
    ).length;

    return {
      found: assetStats.foundAssets,
      missing: assetStats.missingAssets,
      notListed,
      totalScanned: filteredTags.filter(t => t.source !== 'reconciliation').length,
      hasReconciliation,
      saveable: saveableCount,
    };
  }

  // No reconciliation — tag-level (unchanged)
  return {
    found: 0,
    missing: 0,
    notListed: filteredTags.length,
    totalScanned: filteredTags.length,
    hasReconciliation: false,
    saveable: saveableCount,
  };
}, [filteredTags, saveableCount]);
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 7: Update InventoryStats labels for asset-level display
**File**: `frontend/src/components/inventory/InventoryStats.tsx`
**Action**: MODIFY

**Implementation**:

Update subtitle text for reconciliation mode:
- Found: "Matched" → "Assets matched"
- Missing: "From CSV" → "Assets missing"
- Not Listed: "Not in CSV" → "Tags not in CSV"

These are string-only changes in the JSX (lines ~30, ~50, ~70).

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 8: Realign inventory export columns
**File**: `frontend/src/utils/excelExportUtils.ts`
**Action**: MODIFY
**Pattern**: Reference `assetExport.ts` column ordering

**Implementation**:

Update `generateInventoryCSV()` (lines 140-186) and `generateInventoryExcel()` (lines 13-135):

**New column order**:
```
Asset ID, Name, Description, Location, Tag ID, RSSI (dBm), Count, Last Seen
```

**Data mapping**:
```typescript
const row = [
  tag.assetIdentifier || '',          // Asset ID
  tag.assetName || tag.description || '', // Name
  '',                                   // Description (kept for parity)
  tag.locationName || tag.location || '', // Location
  tag.displayEpc || tag.epc,           // Tag ID
  tag.rssi != null ? String(tag.rssi) : '', // RSSI
  String(tag.count),                   // Count
  timestamp,                           // Last Seen
];
```

Remove the old Status/Source columns — reconciliation status is now asset-level in the stats, not per-row. The Summary sheet in Excel export still shows Found/Missing/NotListed counts.

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 9: Write unit tests
**Files**: Create `frontend/src/utils/reconciliationUtils.test.ts` and `frontend/src/utils/excelExportUtils.test.ts`
**Action**: CREATE
**Pattern**: Mirror `assetExport.test.ts` structure

**reconciliationUtils.test.ts coverage**:

1. **parseReconciliationCSV — backward compat**:
   - Single Tag ID column CSV → one item per row, no assetIdentifier
   - Old-format CSV (EPC header) → works as before

2. **parseReconciliationCSV — multi-tag**:
   - Two Tag ID columns → two ReconciliationItems per row, shared assetIdentifier
   - Asset with 1 tag + asset with 2 tags in same CSV → correct item counts
   - Empty second tag column → only one item (no empty-epc items)

3. **parseReconciliationCSV — Asset ID column**:
   - Asset ID column present → assetIdentifier populated
   - Asset ID column missing → assetIdentifier undefined
   - Mixed: some rows with Asset ID, some without

4. **buildAssetMap**:
   - Two tags mapping to same asset → same ReconciliationAsset reference
   - Single-tag asset → one entry in map
   - Items without assetIdentifier → excluded from map

5. **getAssetReconciliationStats**:
   - All tags found → all assets found
   - One of two tags found → asset still counted as found (dedup)
   - Both tags found → 1 asset found (no double-count)
   - No tags found → asset missing
   - Mix of found/missing assets → correct counts

6. **Source matching bug (tagStore.test.ts addition)**:
   - Tag with `source: 'rfid'` merged with reconciliation → `reconciled: true`
   - Tag with `source: 'reconciliation'` (stub) → `reconciled: false`

**excelExportUtils.test.ts coverage**:

1. **generateInventoryCSV — column order**:
   - Headers match: Asset ID, Name, Description, Location, Tag ID, RSSI, Count, Last Seen
   - Tag with assetIdentifier → Asset ID column populated
   - Tag without assetIdentifier → Asset ID column empty

**Validation**: `just frontend test`

---

### Task 10: Final validation
**Action**: Run full validation suite

```bash
just frontend validate
```

All gates must pass: lint, typecheck, test, build.

## Risk Assessment

- **Risk**: `parseReconciliationCSV` column detection may conflict with existing EPC header patterns (e.g., `epc` pattern matching "Tag ID" columns)
  **Mitigation**: Multi-tag detection only activates when header literally matches `/^(tag\s*id)$/i` — won't affect `epc` or other patterns. Backward compat test covers this.

- **Risk**: Stats computation change (tag→asset level) could confuse the filter interaction (clicking Found/Missing filters by `tag.reconciled`, but counts show assets)
  **Mitigation**: Filters still work at tag level — you see the individual tags for that status. The count just reflects unique assets. This is actually the expected UX (filter to see which tags, count tells you how many assets).

- **Risk**: Export column reorder breaks any external tools consuming the CSV
  **Mitigation**: No production users (per stack.md). Format now matches asset export for round-trip consistency.

## Integration Points
- **Store updates**: tagStore `mergeReconciliationTags` passes `assetIdentifier` through to `TagInfo`
- **Stats flow**: `InventoryScreen` → `getAssetReconciliationStats()` → `InventoryStats` component
- **Export flow**: `excelExportUtils` reads `assetIdentifier` from tag data (already on `TagInfo`)
- **No route changes, no config changes, no new dependencies**

## VALIDATION GATES (MANDATORY)

After EVERY task, run from project root:
```bash
just frontend lint        # Gate 1: Syntax & Style
just frontend typecheck   # Gate 2: Type Safety
just frontend test        # Gate 3: Unit Tests (after Task 9)
```

After Task 10 (final):
```bash
just frontend validate    # All gates + build
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

Tasks 1-8: lint + typecheck after each
Task 9: lint + typecheck + test
Task 10: full `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec — all edge cases enumerated
- ✅ Similar patterns found: `assetExport.ts` multi-tag columns, `tagStore.ts` merge logic
- ✅ All clarifying questions answered (asset-level stats, column parity, both types, unit tests)
- ✅ Existing test patterns to follow at `assetExport.test.ts`
- ✅ Bug fix is single-line with clear rationale
- ✅ `TagInfo` already has `assetIdentifier` field — no type system changes needed
- ✅ No new dependencies, single subsystem (frontend)
- ⚠️ Stats computation refactor touches component logic — needs careful useMemo dependency review

**Assessment**: High confidence — well-scoped feature extending existing patterns with clear data flow. All types and fields already exist; work is connecting them correctly.

**Estimated one-pass success probability**: 90%

**Reasoning**: Pure frontend work extending existing patterns. The only uncertainty is getting the stats computation and filter interaction right on first pass, which is mitigated by thorough test coverage in Task 9.
