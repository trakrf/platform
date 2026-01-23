# Feature: TRA-284 Inventory Reconciliation with Multi-Tag Asset Support

## Origin

This specification emerges from completing TRA-311 (CSV export format alignment) which now enables the round-trip workflow: **Asset List → Export CSV → Inventory Scan → Reconciliation**

## Outcome

Update reconciliation matching logic to handle multi-tagged assets. When a CSV with multiple "Tag ID" columns is imported, ALL tag IDs for each asset are mapped to that asset, enabling reconciliation to work correctly when:
- Only one of multiple tags is read
- All tags are read (no double-counting)
- Assets have varying numbers of tags

## User Story

As a **warehouse manager**
I want **to import my asset list CSV and reconcile against scanned tags**
So that **I can identify which assets are found, missing, or unexpected**

## Context

**Discovery (TRA-311 completed)**:
- Asset export now produces CSV with Tag IDs in rightmost columns
- Format: `Asset ID, Name, Description, Status, Created, Location, Tag ID, Tag ID, ...`
- Header repeats "Tag ID" for each tag column
- Multi-tag assets extend rightward (e.g., ASSET-0003 → DEADBEEF, CAFE7731)

**Current State**:
- `parseReconciliationCSV()` in `reconciliationUtils.ts` only reads the **first** Tag ID column
- Each CSV row creates one `ReconciliationItem` with one EPC
- Multi-tag assets are only partially imported (first tag only)
- Matching works at tag level, not asset level

**Desired State**:
- Import reads ALL Tag ID columns (rightward from first "Tag ID" until empty/end)
- Build lookup: `{tagId → assetId}` for ALL tags on each asset
- Asset marked "Found" if ANY of its tags are scanned
- No double-counting if multiple tags from same asset are read

## Technical Requirements

### 1. Update CSV Parsing (`reconciliationUtils.ts`)

**Current**: `parseReconciliationCSV()` finds first "Tag ID" column and reads only that value

**Required**:
```typescript
// Find ALL Tag ID columns (indices where header matches /tag\s*id/i)
const tagIdColumnIndices: number[] = headers
  .map((h, i) => /tag\s*id/i.test(h) ? i : -1)
  .filter(i => i !== -1);

// For each data row, collect ALL tag IDs from those columns
const tagIds = tagIdColumnIndices
  .map(i => values[i]?.trim())
  .filter(Boolean);
```

### 2. New Data Structure for Asset-Level Reconciliation

**New type** (or extend existing):
```typescript
interface ReconciliationAsset {
  assetId: string;           // "ASSET-0003"
  tagIds: string[];          // ["DEADBEEF", "CAFE7731"]
  name?: string;
  description?: string;
  location?: string;
  found: boolean;            // ANY tag scanned
  foundTagIds: string[];     // Which specific tags were read
}
```

**Lookup map**:
```typescript
type TagToAssetMap = Map<string, ReconciliationAsset>;
// Example:
// "DEADBEEF" → ReconciliationAsset for ASSET-0003
// "CAFE7731" → ReconciliationAsset for ASSET-0003 (same reference)
```

### 3. Update Matching Logic

**Current** (`mergeReconciliationTags` in `tagStore.ts`): Matches at EPC/tag level

**Required**:
1. When importing CSV, build `TagToAssetMap`
2. On scan, lookup tag → if found in map, mark asset as found
3. Track which specific tags were read (`foundTagIds`)
4. Display shows asset-level status (Found/Missing)

### 4. Align Inventory Export with Asset Export Format

**Current inventory CSV** (`excelExportUtils.ts`):
```
Tag ID, RSSI (dBm), Count, Last Seen, [Status], [Description]
```

**Target format** (matching columns + inventory-specific data):
```
Asset ID, Name, Description, Location, Tag ID, RSSI (dBm), Count, Last Seen
```

**Key matching columns** (must match asset export for round-trip):
- `Asset ID` - Links tag to asset
- `Tag ID` - For scan matching

**Inventory-specific columns** (kept for operational value):
- `RSSI (dBm)` - Signal strength
- `Count` - Read count
- `Last Seen` - Timestamp

**Omitted from inventory** (asset-only metadata):
- `Status` (Active/Inactive) - Not relevant to scan results
- `Created` - Asset creation date not needed

**Behavior**:
- When tag has `assetId` (from reconciliation): use asset data
- When tag has no `assetId` (unexpected/scanned-only): leave Asset ID empty or use tag as fallback

**Export columns**:
| Column | Source (reconciled) | Source (unexpected) |
|--------|---------------------|---------------------|
| Asset ID | `tag.assetId` | (empty) |
| Name | `tag.description` | - |
| Description | - | - |
| Location | `tag.location` | - |
| Tag ID | `tag.epc` | `tag.epc` |
| RSSI (dBm) | `tag.rssi` | `tag.rssi` |
| Count | `tag.count` | `tag.count` |
| Last Seen | `tag.timestamp` | `tag.timestamp` |

### 5. Edge Cases (from TRA-284 description)

| Scenario | Expected Result |
|----------|-----------------|
| Double-tagged asset, one tag read | Asset shows "Found" |
| Double-tagged asset, both tags read | Asset shows "Found" (counted once) |
| Scanned tag not in lookup | Shows as "Unexpected" |
| Asset with no tags scanned | Shows as "Missing" |

## Implementation Approach

Extend existing `ReconciliationItem` with `assetId` field:
- Parse creates one ReconciliationItem per tag, with shared `assetId`
- Tag-level detail preserved in UI (see which specific tags were read)
- Asset-level aggregation for summary stats (Found/Missing counts)

**Pros**: Minimal refactor, backward compatible, tag-level visibility retained

Extend existing `ReconciliationItem`:
```typescript
interface ReconciliationItem {
  epc: string;              // Normalized EPC for matching
  originalEpc?: string;     // Original EPC from CSV
  assetId?: string;         // NEW: Asset ID from CSV row
  description?: string;     // Asset name/description
  location?: string;
  rssi?: number;
  count: number;
  found: boolean;
  lastSeen?: number;
}
```

Then add aggregation in display layer to show asset-level stats.

## Files to Modify

1. **`frontend/src/utils/reconciliationUtils.ts`**
   - `parseReconciliationCSV()`: Read ALL Tag ID columns
   - Extract Asset ID from first column
   - Create one ReconciliationItem per tag, with shared assetId

2. **`frontend/src/stores/tagStore.ts`**
   - Update `mergeReconciliationTags()` to store assetId on TagInfo
   - Add `assetId?: string` field to TagInfo type

3. **`frontend/src/utils/excelExportUtils.ts`**
   - Update `generateInventoryCSV()` to match asset export format
   - Column order: `Asset ID, Name, Description, Status, Location, Tag ID`
   - Include assetId from reconciliation data when available
   - For scanned tags without assetId, use tag as row identifier

4. **`frontend/src/components/InventoryStats.tsx`** (or similar)
   - Display asset-level counts (Found/Missing/Unexpected)
   - Group by assetId for display

5. **Tests**: Update/add tests for multi-tag CSV parsing, matching, and export format

## Validation Criteria

- [ ] CSV with single Tag ID column still works (backward compatible)
- [ ] CSV with 2+ Tag ID columns imports all tags
- [ ] Each tag maps to its parent asset ID
- [ ] Scanning any tag marks asset as "Found"
- [ ] Scanning multiple tags from same asset = 1 found asset (no double-count)
- [ ] Unexpected tags (not in CSV) shown separately
- [ ] Asset with no tags scanned shows as "Missing"
- [ ] Inventory CSV export matches asset export column format
- [ ] Exported inventory includes Asset ID when reconciliation data present
- [ ] Round-trip: Export assets → Import CSV → Scan → Export inventory → formats align

## Design Decisions

- **Display format**: Tag-level detail preserved (see which specific tags were read per asset)
- **Aggregation**: Asset-level summary stats derived by grouping on `assetId`

## References

- **TRA-311** (Done): CSV format alignment with multi-tag columns
- **TRA-285** (Done): Asset list export
- **TRA-284**: This ticket (blocked by TRA-311, now unblocked)
- **Code**:
  - `frontend/src/utils/reconciliationUtils.ts` - CSV parsing
  - `frontend/src/utils/export/assetExport.ts` - Asset CSV export format (TRA-311)
  - `frontend/src/utils/excelExportUtils.ts` - Inventory CSV export (to be aligned)
  - `frontend/src/stores/tagStore.ts:185-244` - Reconciliation merge logic
