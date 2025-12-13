# Implementation Plan: Fix Asset Storage Tests (TRA-212)

Generated: 2025-12-13
Specification: spec.md

## Understanding

The asset storage tests fail because mock rows define 13 columns but production Scan() expects 14. The missing column is `current_location_id`. This is a mechanical fix: add the column to all 12 mock definitions, remove skip directives, verify tests pass.

## Relevant Files

**Reference Pattern** (production code to match):
- `backend/internal/storage/assets.go` (lines 24-26) - Scan order to match

**Files to Modify**:
- `backend/internal/storage/assets_test.go` - 12 mock updates, remove build tag, remove 6 t.Skip calls

## Architecture Impact

- **Subsystems affected**: Tests only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Remove Build Tag

**File**: `backend/internal/storage/assets_test.go`
**Action**: MODIFY

Remove lines 1-6:
```go
//go:build skip_asset_tests
// +build skip_asset_tests

// TRA-212: Asset storage tests skipped - schema mismatch
// Multiple tests fail with "Incorrect argument number" or empty results
// Fix the Asset model/queries to match, then remove this build tag
```

**Validation**: File should start with `package storage`

---

### Task 2: Update All Mock Rows

**File**: `backend/internal/storage/assets_test.go`
**Action**: MODIFY

For all 12 `pgxmock.NewRows()` calls, add `"current_location_id"` after `"description"`:

**Before**:
```go
pgxmock.NewRows([]string{
    "id", "org_id", "identifier", "name", "type", "description",
    "valid_from", "valid_to", "metadata", "is_active",
    "created_at", "updated_at", "deleted_at",
})
```

**After**:
```go
pgxmock.NewRows([]string{
    "id", "org_id", "identifier", "name", "type", "description",
    "current_location_id", "valid_from", "valid_to", "metadata", "is_active",
    "created_at", "updated_at", "deleted_at",
})
```

And for all corresponding `AddRow()` calls, add `nil` after description value:

**Before**:
```go
.AddRow(
    1, 1, "TEST-001", "Test Asset", "equipment", "Description",
    now, timePtr(now.Add(24*time.Hour)), []byte(`{}`), true,
    now, now, nil,
)
```

**After**:
```go
.AddRow(
    1, 1, "TEST-001", "Test Asset", "equipment", "Description",
    nil, now, timePtr(now.Add(24*time.Hour)), []byte(`{}`), true,
    now, now, nil,
)
```

**Validation**: `go build ./internal/storage/...` compiles

---

### Task 3: Remove t.Skip Calls

**File**: `backend/internal/storage/assets_test.go`
**Action**: MODIFY

Remove all 6 lines containing `t.Skip("TRA-212`:
- TestUpdateAsset (line ~422)
- TestUpdateAsset_PartialUpdate (line ~506)
- TestGetAssetByID (line ~545)
- TestGetAssetByID_WithNullMetadata (line ~601)
- TestListAllAssets (line ~656)
- TestListAllAssets_WithPagination (line ~727)

**Validation**: `grep -c "t.Skip.*TRA-212" assets_test.go` returns 0

---

### Task 4: Run Tests and Verify

**Action**: VALIDATE

```bash
cd backend && go test ./internal/storage/... -v -run "Asset"
```

**Validation**: All asset tests pass (0 failures, 0 skips)

---

### Task 5: Full Backend Validation

**Action**: VALIDATE

```bash
just backend test
```

**Validation**: All backend tests pass

## Risk Assessment

- **Risk**: Typo in column order causes different test failures
  **Mitigation**: Match exactly against production Scan() in assets.go:24-26

## VALIDATION GATES (MANDATORY)

After each task, run:
```bash
cd backend && go build ./internal/storage/...   # Compiles
cd backend && go test ./internal/storage/... -v -run "Asset"  # Tests pass
```

Final validation:
```bash
just backend test
```

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 10/10 (HIGH)

**Confidence Factors**:
✅ Root cause confirmed via code analysis
✅ Exact column order known from production code
✅ Mechanical fix - no logic changes
✅ Clear validation criteria

**Estimated one-pass success probability**: 95%

**Reasoning**: This is a straightforward data alignment fix with no ambiguity.
