# Feature: Bulk CSV Import - Tag Identifiers Column

## Metadata
**Workspace**: monorepo (backend + frontend)
**Type**: feature
**Ticket**: [TRA-222](https://linear.app/trakrf/issue/TRA-222/bulk-csv-import-add-identifiers-column-for-rfid-tags)
**Parent**: [TRA-193](https://linear.app/trakrf/issue/TRA-193/asset-crud-separate-customer-identifier-from-tag-identifiers)

## Outcome
Users can associate RFID tag identifiers with assets during bulk CSV import, eliminating the need to manually add tags after import.

## User Story
As an asset manager
I want to include RFID tag identifiers in my bulk asset CSV import
So that I can fully provision assets with their associated tags in a single operation

## Context

**Current**:
- Bulk CSV import creates assets with columns: `identifier`, `name`, `type`, `description`, `valid_from`, `valid_to`, `is_active`
- Tag identifiers must be added manually after import via the asset edit form
- Sample file: `frontend/public/bulk_assets_sample.csv`

**Desired**:
- Add optional `tags` column to CSV format
- Parse comma-separated RFID tag values (e.g., `E280119020004F3D94E00C91,E280119020004F3D94E00C92`)
- Create `identifiers` records with `type: rfid` linked to imported assets
- All tags are RFID type (as specified in requirements)

**Examples**:
- Asset creation with identifiers: `backend/internal/storage/identifiers.go:84` (`AddIdentifierToAsset`)
- CSV parsing: `backend/internal/util/csv/helpers.go:134` (`MapCSVRowToAsset`)
- Bulk import service: `backend/internal/services/bulkimport/service.go`

## Technical Requirements

### CSV Format Changes

New optional column: `tags`

```csv
identifier,name,type,description,valid_from,valid_to,is_active,tags
LAPTOP-001,Dell XPS 15,device,Dev laptop,2024-01-15,2026-12-31,true,"E280119020004F3D94E00C91,E280119020004F3D94E00C92"
DESK-A-101,Standing Desk,asset,Ergonomic desk,2024-03-15,2029-12-31,yes,E280119020004F3D94E00C93
CHAIR-001,Office Chair,asset,Standard chair,2024-03-15,2029-12-31,1,
```

**Format Rules**:
- Column name: `tags` (lowercase, matches existing convention)
- Multiple tags: comma-separated within the cell
- Empty value: valid (asset created without tags)
- Tag type: always `rfid` (hardcoded per requirements)
- Whitespace: trimmed from each tag value

### Backend Changes

#### 1. CSV Helper (`backend/internal/util/csv/helpers.go`)

Add function to parse tags column:

```go
// ParseCSVTags splits a comma-separated tags string into individual values.
// Returns empty slice for empty input. Trims whitespace from each tag.
func ParseCSVTags(tagsStr string) []string {
    tagsStr = strings.TrimSpace(tagsStr)
    if tagsStr == "" {
        return []string{}
    }

    parts := strings.Split(tagsStr, ",")
    tags := make([]string, 0, len(parts))
    for _, part := range parts {
        tag := strings.TrimSpace(part)
        if tag != "" {
            tags = append(tags, tag)
        }
    }
    return tags
}
```

Update `MapCSVRowToAsset` to return tags:
- Create new return type or add tags to Asset struct temporarily
- Extract `tags` column if present (optional column)

#### 2. Bulk Import Models (`backend/internal/models/bulkimport/bulkimport.go`)

Add type to hold parsed asset with tags:

```go
type AssetWithTags struct {
    Asset      asset.Asset
    TagValues  []string  // Raw tag values from CSV
}
```

Update response to include tag count:

```go
type JobStatus struct {
    // existing fields...
    TagsCreated int `json:"tags_created,omitempty"`
}
```

#### 3. Bulk Import Service (`backend/internal/services/bulkimport/service.go`)

Update `processCSVAsync`:
1. Parse tags for each row alongside asset data
2. After successful asset batch insert, create identifier records
3. Track tag creation count in job progress
4. Handle tag creation errors (log but don't fail entire import)

#### 4. Storage Layer (`backend/internal/storage/identifiers.go`)

Add batch identifier creation:

```go
func (s *Storage) BatchAddIdentifiersToAssets(
    ctx context.Context,
    orgID int,
    assetTagPairs []struct{ AssetID int; Tags []string },
) (int, []error)
```

### Frontend Changes

#### 1. Sample CSV (`frontend/public/bulk_assets_sample.csv`)

Update to include `tags` column:

```csv
identifier,name,type,description,valid_from,valid_to,is_active,tags
LAPTOP-001,Dell XPS 15 - Engineering,device,Development laptop for software engineering team,2024-01-15,2026-12-31,true,"E280119020004F3D94E00C91,E280119020004F3D94E00C92"
LAPTOP-002,MacBook Pro 16 - Design,device,High-performance laptop for graphic design team,2024-02-01,2026-12-31,yes,E280119020004F3D94E00C93
RFID-TAG-1001,RFID Tag #1001,inventory,Passive RFID tag for asset tracking,2024-01-01,2027-12-31,1,
DESK-A-101,Standing Desk - Office A101,asset,Ergonomic standing desk in office suite A,2024-03-15,2029-12-31,yes,E280119020004F3D94E00C94
CHAIR-A-101,Herman Miller Aeron - A101,asset,Ergonomic office chair,2024-03-15,2029-12-31,1,"E280119020004F3D94E00C95,E280119020004F3D94E00C96"
MONITOR-LG-001,LG UltraWide 34 inch,device,Curved ultrawide monitor for development,2024-04-10,2027-06-30,true,
SCANNER-WH-001,Zebra MC3300 Handheld Scanner,device,Mobile barcode and RFID scanner for warehouse,2024-01-20,2028-01-20,yes,E280119020004F3D94E00C97
```

#### 2. BulkUploadModal (`frontend/src/components/assets/BulkUploadModal.tsx`)

Update format requirements section:
- Add `tags` to optional columns list
- Add note about comma-separated format for multiple tags

### Edge Cases & Validation

1. **Duplicate tags within same row**: Skip duplicates, create unique tags only
2. **Duplicate tags across rows**: Each asset gets its own identifier record (same tag value can exist on multiple assets per org policy)
3. **Invalid tag format**: Log warning, skip invalid tag, continue with valid ones
4. **Empty tags after trim**: Skip empty values
5. **Missing tags column**: Backward compatible - assets created without tags
6. **All tags invalid for a row**: Asset created successfully, warning logged

### Error Handling Strategy

- Tag creation failures should NOT fail the entire import
- Log warnings for skipped/invalid tags
- Include tag summary in job response: `"tags_created": 15, "tags_skipped": 2`
- Asset import success is independent of tag creation success

## Validation Criteria

- [ ] CSV with `tags` column creates assets with associated RFID identifiers
- [ ] Multiple tags per asset work via comma separation
- [ ] CSV without `tags` column still works (backward compatibility)
- [ ] Empty `tags` values create assets without identifiers
- [ ] Sample CSV file updated with `tags` column examples
- [ ] BulkUploadModal shows updated format requirements
- [ ] Job status includes tag creation count

## Success Metrics

- [ ] All existing bulk import tests pass (no regressions)
- [ ] New unit tests for `ParseCSVTags` function (5+ test cases)
- [ ] New unit tests for CSV parsing with tags column
- [ ] Integration test: bulk import with tags creates correct DB records
- [ ] Integration test: backward compatibility without tags column
- [ ] Frontend sample CSV includes tags examples

## Testing Plan

### Unit Tests (Backend)

**File**: `backend/internal/util/csv/helpers_test.go`
```go
func TestParseCSVTags(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected []string
    }{
        {"empty string", "", []string{}},
        {"single tag", "TAG001", []string{"TAG001"}},
        {"multiple tags", "TAG001,TAG002,TAG003", []string{"TAG001", "TAG002", "TAG003"}},
        {"with whitespace", " TAG001 , TAG002 ", []string{"TAG001", "TAG002"}},
        {"empty values filtered", "TAG001,,TAG002,", []string{"TAG001", "TAG002"}},
        {"only whitespace", "   ", []string{}},
    }
    // ...
}
```

**File**: `backend/internal/util/csv/helpers_test.go`
- Test `MapCSVRowToAsset` with tags column
- Test `MapCSVRowToAsset` without tags column (backward compat)

### Integration Tests (Backend)

**File**: `backend/internal/services/bulkimport/service_integration_test.go`
- Test full import flow with tags creates identifiers in DB
- Test import without tags column works
- Test duplicate tag handling
- Test partial tag failures (some valid, some invalid)

### Frontend Tests

- Verify sample CSV file contains expected columns and data
- Component test: BulkUploadModal displays updated requirements

## Files to Modify

### Backend
- `backend/internal/util/csv/helpers.go` - Add `ParseCSVTags`, update `MapCSVRowToAsset`
- `backend/internal/util/csv/helpers_test.go` - Add tests for tags parsing
- `backend/internal/models/bulkimport/bulkimport.go` - Add `AssetWithTags`, update response
- `backend/internal/services/bulkimport/service.go` - Integrate tag creation
- `backend/internal/services/bulkimport/service_test.go` - Add unit tests
- `backend/internal/services/bulkimport/service_integration_test.go` - Add integration tests
- `backend/internal/storage/identifiers.go` - Add batch creation method
- `backend/internal/storage/identifiers_test.go` - Add tests for batch creation

### Frontend
- `frontend/public/bulk_assets_sample.csv` - Add `tags` column with examples
- `frontend/src/components/assets/BulkUploadModal.tsx` - Update requirements text

## References

- [TRA-222 Linear Ticket](https://linear.app/trakrf/issue/TRA-222/bulk-csv-import-add-identifiers-column-for-rfid-tags)
- [TRA-193 Parent Ticket](https://linear.app/trakrf/issue/TRA-193/asset-crud-separate-customer-identifier-from-tag-identifiers)
- Identifiers storage: `backend/internal/storage/identifiers.go`
- Bulk import service: `backend/internal/services/bulkimport/service.go`
- CSV helpers: `backend/internal/util/csv/helpers.go`
- TagIdentifier model: `backend/internal/models/shared/identifier.go`
