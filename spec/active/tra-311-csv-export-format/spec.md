# Feature: TRA-311 - Align Asset Export CSV Format for Inventory Reconciliation Round-Trip

## Origin
This specification addresses the need to align the asset list CSV export format with the inventory reconciliation import workflow, enabling clean round-trip data flow.

## Outcome
The asset CSV export will produce a format optimized for inventory reconciliation:
- Tag IDs in rightmost columns, extending horizontally for multi-tag assets
- No embedded commas within Tag ID values
- Round-trip compatible: Export → Inventory scan → Reconciliation

## User Story
As an inventory manager
I want the asset export CSV to have Tag IDs in separate rightmost columns
So that I can import the exported list into the reconciliation workflow and match scanned tags against known assets

## Context
**Discovery**: TRA-285 delivered asset list export (PR #124). Users need the CSV format tuned for inventory reconciliation workflows.

**Current State**:
- CSV export uses a single "Tag ID(s)" column with semicolon-separated values
- Format: `"10018; 10019"` - embedded separator causes parsing complexity
- Column order: Asset ID, Name, Type, Tag ID(s), Location, Status, Description, Created

**Desired State**:
- Tag IDs in rightmost columns, one per column
- Header repeats "Tag ID" for each tag column
- Multi-tag assets extend rightward with additional columns
- Import reads rightward until empty cell to collect all tags

## Target Format

| Asset ID | Name | Description | Status | Created | Location | Tag ID | Tag ID |
|----------|------|-------------|--------|---------|----------|--------|--------|
| ASSET-0020 | sss | | Active | 1/16/26 | | 10018 | 10019 |
| ASSET-0003 | bb | | Active | 1/16/26 | | DEADBEEF | CAFE7731 |
| ASSET-0001 | asdf | | Active | 1/15/26 | | asdf | |

**Key design decisions:**
- Column order: Identity (ID, Name, Description) → State (Status, Created, Location) → Tags
- Tag IDs in rightmost columns, extending rightward for multi-tag assets
- No embedded commas/semicolons in CSV values
- Header repeats "Tag ID" for each tag column
- Import reads rightward until empty to collect all tags
- "Type" column removed (not needed for reconciliation)

## Technical Requirements

### Export Function Changes (`assetExport.ts`)
1. Calculate max tag count across all assets to determine column count
2. Generate header with repeated "Tag ID" columns at the end
3. Output each asset's tags in separate columns (rightmost, extending right)
4. Reorder columns: Asset ID, Name, Description, Status, Created, Location, Tag ID...

### Column Specification
| Column | Source | Notes |
|--------|--------|-------|
| Asset ID | `asset.identifier` | e.g., "ASSET-0020" |
| Name | `asset.name` | May be empty |
| Description | `asset.description` | May be empty |
| Status | `asset.is_active` | "Active" or "Inactive" |
| Created | `asset.created_at` | Format: M/D/YY |
| Location | Location lookup | May be empty |
| Tag ID (×N) | `asset.identifiers[n].value` | One column per tag |

### Constraints
- Max 2 tags per asset in practice (Tim confirmed)
- Format should support more if needed in future
- No commas within any cell value
- Standard CSV quoting for special characters in other fields

## Validation Criteria
- [ ] Export produces correct column order: Asset ID, Name, Description, Status, Created, Location, Tag ID...
- [ ] Multi-tag assets have tags in separate columns
- [ ] Header has repeated "Tag ID" for each tag column
- [ ] Single-tag assets have one Tag ID column populated, rest empty
- [ ] No-tag assets have all Tag ID columns empty
- [ ] CSV can be imported into reconciliation workflow successfully
- [ ] Round-trip test: export → import → verify all tags matched

## Files to Modify
- `frontend/src/utils/export/assetExport.ts` - `generateAssetCSV()` function

## Out of Scope
- Excel export format changes (keep as-is)
- PDF export format changes (keep as-is)
- Reconciliation import changes (already handles rightward reading pattern)
