# Implementation Plan: Bulk CSV Import - Tag Identifiers Column
Generated: 2024-12-20
Specification: spec.md

## Understanding

Add optional `tags` column to bulk CSV import that accepts comma-separated RFID tag values. Key behaviors:
- Column name `tags` in CSV (user-friendly), mapped internally to `identifiers`
- All tags are `type: rfid` (hardcoded per requirements)
- **Fail entire row** if any tag is duplicate (within CSV or in DB), with clear error message
- Rely on DB unique constraint for duplicate detection
- Backward compatible - CSV without `tags` column works unchanged

## Clarifications Applied

1. **Global uniqueness per org** - Same tag cannot exist on multiple assets
2. **Fail row on duplicate** - Asset not created if any tag is duplicate, with user alert
3. **DB constraint enforcement** - Rely on database unique constraint, handle errors gracefully
4. **Column naming** - CSV uses `tags` (UX), internally maps to `identifiers`

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/storage/assets.go:309-347` - `CreateAssetWithIdentifiers` using DB function
- `backend/migrations/000024_identifier_functions.up.sql:3-50` - `create_asset_with_identifiers` function
- `backend/internal/util/csv/helpers.go:134-224` - `MapCSVRowToAsset` pattern
- `backend/internal/util/csv/helpers_test.go` - Table-driven test pattern
- `backend/internal/services/bulkimport/service_integration_test.go` - Integration test patterns

**Files to Modify**:
- `backend/internal/util/csv/helpers.go` - Add `ParseCSVTags`, update `MapCSVRowToAsset`
- `backend/internal/util/csv/helpers_test.go` - Add tests for tags parsing
- `backend/internal/services/bulkimport/service.go` - Integrate tag handling
- `backend/internal/services/bulkimport/service_integration_test.go` - Add integration tests
- `backend/internal/models/bulkimport/bulkimport.go` - Add `TagsCreated` to response
- `backend/internal/testutil/factories.go` - Update `CSVFactory` to support tags
- `frontend/public/bulk_assets_sample.csv` - Add `tags` column
- `frontend/src/components/assets/BulkUploadModal.tsx` - Update help text

## Architecture Impact

- **Subsystems affected**: Backend (CSV parsing, bulk import service), Frontend (sample file, UI)
- **New dependencies**: None
- **Breaking changes**: None (backward compatible)

## Architecture Decision: Per-Row vs Batch Insert

**Current approach**: `BatchCreateAssets` uses all-or-nothing transaction for assets.

**New requirement**: Fail individual rows on duplicate tags, allow other rows to succeed.

**Decision**: Switch to per-row insert using existing `create_asset_with_identifiers` DB function.

**Rationale**:
- The DB function already handles asset + identifiers atomically
- Per-row semantics match user expectations (one bad row doesn't fail entire import)
- Existing function is battle-tested
- Slight performance trade-off acceptable for correctness

## Task Breakdown

### Task 1: Add ParseCSVTags function
**File**: `backend/internal/util/csv/helpers.go`
**Action**: MODIFY
**Pattern**: Reference existing `ParseCSVBool` pattern (lines 68-86)

**Implementation**:
```go
// ParseCSVTags splits a comma-separated tags string into individual values.
// Returns empty slice for empty input. Trims whitespace from each tag.
// Filters out empty values after trim.
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

**Validation**: `just backend test` - verify helpers_test.go passes

---

### Task 2: Add ParseCSVTags unit tests
**File**: `backend/internal/util/csv/helpers_test.go`
**Action**: MODIFY
**Pattern**: Reference existing table-driven tests (lines 9-60)

**Implementation**:
```go
func TestParseCSVTags(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected []string
    }{
        {"empty string", "", []string{}},
        {"single tag", "E280119020004F3D94E00C91", []string{"E280119020004F3D94E00C91"}},
        {"multiple tags", "TAG1,TAG2,TAG3", []string{"TAG1", "TAG2", "TAG3"}},
        {"with whitespace", " TAG1 , TAG2 ", []string{"TAG1", "TAG2"}},
        {"empty values filtered", "TAG1,,TAG2,", []string{"TAG1", "TAG2"}},
        {"only whitespace", "   ", []string{}},
        {"only commas", ",,,", []string{}},
        {"mixed empty and valid", ",TAG1,,TAG2,", []string{"TAG1", "TAG2"}},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := ParseCSVTags(tt.input)
            if len(result) != len(tt.expected) {
                t.Errorf("ParseCSVTags(%q) = %v, expected %v", tt.input, result, tt.expected)
                return
            }
            for i, v := range result {
                if v != tt.expected[i] {
                    t.Errorf("ParseCSVTags(%q)[%d] = %q, expected %q", tt.input, i, v, tt.expected[i])
                }
            }
        })
    }
}
```

**Validation**: `just backend test`

---

### Task 3: Create AssetWithTags type and update MapCSVRowToAsset
**File**: `backend/internal/util/csv/helpers.go`
**Action**: MODIFY

**Implementation**:
```go
// AssetWithTags holds a parsed asset and its associated tag values from CSV
type AssetWithTags struct {
    Asset    *asset.Asset
    TagValues []string // Raw tag values from CSV "tags" column
}

// MapCSVRowToAssetWithTags parses a CSV row into an asset with optional tags.
// The "tags" column is optional - if missing, TagValues will be empty.
func MapCSVRowToAssetWithTags(row []string, headers []string, orgID int) (*AssetWithTags, error) {
    // Reuse existing MapCSVRowToAsset logic
    asset, err := MapCSVRowToAsset(row, headers, orgID)
    if err != nil {
        return nil, err
    }

    // Extract tags if column exists
    headerIdx := make(map[string]int)
    for i, h := range headers {
        headerIdx[strings.ToLower(strings.TrimSpace(h))] = i
    }

    var tagValues []string
    if tagsIdx, ok := headerIdx["tags"]; ok && tagsIdx < len(row) {
        tagValues = ParseCSVTags(row[tagsIdx])
    }

    return &AssetWithTags{
        Asset:     asset,
        TagValues: tagValues,
    }, nil
}
```

**Validation**: `just backend test`

---

### Task 4: Add tests for MapCSVRowToAssetWithTags
**File**: `backend/internal/util/csv/helpers_test.go`
**Action**: MODIFY

**Implementation**:
```go
func TestMapCSVRowToAssetWithTags_WithTags(t *testing.T) {
    headers := []string{"identifier", "name", "type", "description", "valid_from", "valid_to", "is_active", "tags"}
    row := []string{"ASSET-001", "Test Asset", "device", "Desc", "2024-01-01", "2024-12-31", "true", "TAG1,TAG2"}

    result, err := MapCSVRowToAssetWithTags(row, headers, 1)
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }

    if len(result.TagValues) != 2 {
        t.Errorf("Expected 2 tags, got %d", len(result.TagValues))
    }
    if result.TagValues[0] != "TAG1" || result.TagValues[1] != "TAG2" {
        t.Errorf("Unexpected tags: %v", result.TagValues)
    }
}

func TestMapCSVRowToAssetWithTags_NoTagsColumn(t *testing.T) {
    headers := []string{"identifier", "name", "type", "description", "valid_from", "valid_to", "is_active"}
    row := []string{"ASSET-001", "Test Asset", "device", "Desc", "2024-01-01", "2024-12-31", "true"}

    result, err := MapCSVRowToAssetWithTags(row, headers, 1)
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }

    if len(result.TagValues) != 0 {
        t.Errorf("Expected 0 tags, got %d", len(result.TagValues))
    }
}

func TestMapCSVRowToAssetWithTags_EmptyTags(t *testing.T) {
    headers := []string{"identifier", "name", "type", "description", "valid_from", "valid_to", "is_active", "tags"}
    row := []string{"ASSET-001", "Test Asset", "device", "Desc", "2024-01-01", "2024-12-31", "true", ""}

    result, err := MapCSVRowToAssetWithTags(row, headers, 1)
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }

    if len(result.TagValues) != 0 {
        t.Errorf("Expected 0 tags, got %d", len(result.TagValues))
    }
}
```

**Validation**: `just backend test`

---

### Task 5: Update bulk import models
**File**: `backend/internal/models/bulkimport/bulkimport.go`
**Action**: MODIFY

**Implementation**:
Add `TagsCreated` field to `JobStatusResponse`:
```go
type JobStatusResponse struct {
    JobID          string        `json:"job_id"`
    Status         string        `json:"status"`
    TotalRows      int           `json:"total_rows"`
    ProcessedRows  int           `json:"processed_rows"`
    FailedRows     int           `json:"failed_rows"`
    SuccessfulRows int           `json:"successful_rows,omitempty"`
    TagsCreated    int           `json:"tags_created,omitempty"` // NEW
    CreatedAt      string        `json:"created_at"`
    CompletedAt    string        `json:"completed_at,omitempty"`
    Errors         []ErrorDetail `json:"errors,omitempty"`
}
```

Also add to `BulkImportJob`:
```go
type BulkImportJob struct {
    // ... existing fields ...
    TagsCreated   int           `json:"tags_created"`  // NEW
}
```

**Validation**: `just backend build`

---

### Task 6: Update CSVFactory to support tags column
**File**: `backend/internal/testutil/factories.go`
**Action**: MODIFY

**Implementation**:
```go
type CSVFactory struct {
    rows      [][]string
    withTags  bool
}

func NewCSVFactory() *CSVFactory {
    return &CSVFactory{
        rows: [][]string{
            {"identifier", "name", "type", "description", "valid_from", "valid_to", "is_active"},
        },
        withTags: false,
    }
}

func (f *CSVFactory) WithTags() *CSVFactory {
    if !f.withTags {
        f.withTags = true
        f.rows[0] = append(f.rows[0], "tags")
    }
    return f
}

func (f *CSVFactory) AddRow(identifier, name, assetType, description, validFrom, validTo, isActive string) *CSVFactory {
    row := []string{identifier, name, assetType, description, validFrom, validTo, isActive}
    if f.withTags {
        row = append(row, "") // Empty tags by default
    }
    f.rows = append(f.rows, row)
    return f
}

func (f *CSVFactory) AddRowWithTags(identifier, name, assetType, description, validFrom, validTo, isActive, tags string) *CSVFactory {
    if !f.withTags {
        f.WithTags()
    }
    f.rows = append(f.rows, []string{identifier, name, assetType, description, validFrom, validTo, isActive, tags})
    return f
}
```

**Validation**: `just backend build`

---

### Task 7: Update processCSVAsync to handle tags
**File**: `backend/internal/services/bulkimport/service.go`
**Action**: MODIFY
**Pattern**: Reference `CreateAssetWithIdentifiers` in storage/assets.go:309

**Implementation Strategy**:

1. Parse rows using `MapCSVRowToAssetWithTags` instead of `MapCSVRowToAsset`
2. Check for duplicate tags within CSV batch (new validation phase)
3. For rows with tags, use `CreateAssetWithIdentifiers` storage method
4. For rows without tags, continue using batch insert
5. Track tags created count

**Key Changes**:
```go
// In PHASE 1: Parse all rows - use new function
parsedAssetWithTags, err := csvutil.MapCSVRowToAssetWithTags(row, headers, orgID)

// NEW PHASE 2.5: Check for duplicate tags within CSV
tagToRows := make(map[string][]int) // tag -> list of row numbers
for _, pr := range validRows {
    for _, tag := range pr.TagValues {
        tagToRows[tag] = append(tagToRows[tag], pr.rowNumber)
    }
}
// Report duplicates within CSV batch
for tag, rowNumbers := range tagToRows {
    if len(rowNumbers) > 1 {
        for _, rowNum := range rowNumbers {
            allErrors = append(allErrors, bulkimport.ErrorDetail{
                Row:   rowNum,
                Field: "tags",
                Error: fmt.Sprintf("duplicate tag '%s' appears in rows %v", tag, rowNumbers),
            })
        }
    }
}

// In PHASE 5: Insert - use per-row insert for rows with tags
var tagsCreated int
for _, pr := range validRows {
    if len(pr.TagValues) > 0 {
        // Use CreateAssetWithIdentifiers
        identifiers := make([]shared.TagIdentifierRequest, len(pr.TagValues))
        for i, tag := range pr.TagValues {
            identifiers[i] = shared.TagIdentifierRequest{Type: "rfid", Value: tag}
        }
        req := asset.CreateAssetWithIdentifiersRequest{
            CreateAssetRequest: asset.CreateAssetRequest{...},
            Identifiers: identifiers,
        }
        _, err := s.storage.CreateAssetWithIdentifiers(ctx, req)
        if err != nil {
            // Check if it's a duplicate tag error
            allErrors = append(allErrors, bulkimport.ErrorDetail{
                Row: pr.rowNumber,
                Field: "tags",
                Error: err.Error(),
            })
            continue
        }
        tagsCreated += len(pr.TagValues)
    } else {
        // No tags - can use batch insert (collect and insert at end)
        assetsWithoutTags = append(assetsWithoutTags, pr.asset)
    }
}
```

**Validation**: `just backend test`

---

### Task 8: Update storage layer - add tags tracking to job
**File**: `backend/internal/storage/bulk_import_jobs.go`
**Action**: MODIFY

**Implementation**:
Add `tags_created` column handling to job progress updates.

**Validation**: `just backend build`

---

### Task 9: Add integration tests for tags
**File**: `backend/internal/services/bulkimport/service_integration_test.go`
**Action**: MODIFY

**Implementation**:
```go
func TestProcessCSVAsync_WithTags(t *testing.T) {
    store, cleanup := testutil.SetupTestDB(t)
    defer cleanup()
    // ... setup ...

    csvFactory := testutil.NewCSVFactory().WithTags().
        AddRowWithTags("ASSET-001", "Asset 1", "device", "Desc", "2024-01-01", "2024-12-31", "true", "TAG1,TAG2").
        AddRowWithTags("ASSET-002", "Asset 2", "device", "Desc", "2024-01-01", "2024-12-31", "true", "TAG3")

    // ... process and verify tags created in DB ...
}

func TestProcessCSVAsync_DuplicateTagsInCSV(t *testing.T) {
    // Test that duplicate tags within CSV fail the rows
}

func TestProcessCSVAsync_DuplicateTagsInDB(t *testing.T) {
    // Pre-create a tag, then import CSV with same tag - should fail that row
}

func TestProcessCSVAsync_BackwardCompatibility(t *testing.T) {
    // Test CSV without tags column still works
}
```

**Validation**: `just backend test -tags=integration`

---

### Task 10: Update sample CSV file
**File**: `frontend/public/bulk_assets_sample.csv`
**Action**: MODIFY

**Implementation**:
```csv
identifier,name,type,description,valid_from,valid_to,is_active,tags
LAPTOP-001,Dell XPS 15 - Engineering,device,Development laptop for software engineering team,2024-01-15,2026-12-31,true,"E280119020004F3D94E00C91,E280119020004F3D94E00C92"
LAPTOP-002,MacBook Pro 16 - Design,device,High-performance laptop for graphic design team,2024-02-01,2026-12-31,yes,E280119020004F3D94E00C93
RFID-TAG-1001,RFID Tag #1001,inventory,Passive RFID tag for asset tracking,2024-01-01,2027-12-31,1,
DESK-A-101,Standing Desk - Office A101,asset,Ergonomic standing desk,2024-03-15,2029-12-31,yes,E280119020004F3D94E00C94
CHAIR-A-101,Herman Miller Aeron,asset,Ergonomic office chair,2024-03-15,2029-12-31,1,"E280119020004F3D94E00C95,E280119020004F3D94E00C96"
```

**Validation**: Verify file is valid CSV

---

### Task 11: Update BulkUploadModal help text
**File**: `frontend/src/components/assets/BulkUploadModal.tsx`
**Action**: MODIFY

**Implementation**:
Update the format requirements list (around line 149-154):
```tsx
<ul className="text-sm text-blue-800 dark:text-blue-400 space-y-1 list-disc list-inside">
  <li>Required columns: identifier, name, type</li>
  <li>Optional columns: description, is_active, valid_from, valid_to, tags</li>
  <li>Type must be one of: person, device, asset, inventory, other</li>
  <li>Tags: comma-separated RFID values (e.g., "TAG1,TAG2")</li>
  <li>Maximum file size: 5MB</li>
</ul>
```

**Validation**: `just frontend build`

---

## Risk Assessment

- **Risk**: Per-row insert slower than batch for large imports
  **Mitigation**: Only use per-row for assets WITH tags; batch insert assets without tags

- **Risk**: Duplicate tag detection across CSV and DB could have race conditions
  **Mitigation**: Rely on DB constraint as source of truth; handle constraint violations gracefully

- **Risk**: Error messages may expose internal DB constraint names
  **Mitigation**: Parse errors in `parseAssetWithIdentifiersError` (already exists) to provide user-friendly messages

## Integration Points

- **Storage**: Uses existing `CreateAssetWithIdentifiers` method
- **Models**: Extends `BulkImportJob` and `JobStatusResponse` with `TagsCreated`
- **CSV Helpers**: New `MapCSVRowToAssetWithTags` wraps existing function

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just backend lint`
- Gate 2: `just backend build`
- Gate 3: `just backend test`

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task: `just backend lint && just backend build && just backend test`

Final validation: `just validate`

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and user clarifications
- ✅ `create_asset_with_identifiers` DB function already exists
- ✅ `CreateAssetWithIdentifiers` storage method already exists
- ✅ Strong test patterns to follow in existing codebase
- ✅ Backward compatible - no breaking changes
- ⚠️ Switching from batch to per-row for tagged assets (minor risk)

**Assessment**: Well-understood feature with existing patterns to leverage. Main complexity is integrating tag handling into the async processing loop.

**Estimated one-pass success probability**: 85%

**Reasoning**: All building blocks exist. The main work is wiring them together and adding the CSV parsing layer. Strong test coverage will catch integration issues early.
