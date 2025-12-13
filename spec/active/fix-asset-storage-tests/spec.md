# Feature: Fix Asset Storage Tests (TRA-212)

Workspace: backend
Linear: TRA-212

## Origin

This specification emerged from investigating why 44 backend integration tests were failing with "Incorrect argument number" errors.

## Outcome

All asset storage unit tests pass and the `skip_asset_tests` build tag is removed.

## User Story

As a developer
I want the asset storage tests to run and pass
So that I have confidence in the storage layer when making changes

## Context

**Discovery**: The test mocks define 13 columns but the production Scan() expects 14 fields. The missing column is `current_location_id`, added when location tracking was implemented for assets.

**Current**: Tests are skipped via `//go:build skip_asset_tests` build tag. Multiple tests also have individual `t.Skip()` calls.

**Desired**: All tests pass without build tags or skips.

## Technical Requirements

1. Add `current_location_id` column to all `pgxmock.NewRows()` calls
2. Add corresponding value (typically `nil`) to all `AddRow()` calls
3. Remove `//go:build skip_asset_tests` build tag
4. Remove individual `t.Skip("TRA-212...")` calls from tests

## Code Pattern

**Before (13 columns):**
```go
pgxmock.NewRows([]string{
    "id", "org_id", "identifier", "name", "type", "description",
    "valid_from", "valid_to", "metadata", "is_active",
    "created_at", "updated_at", "deleted_at",
}).AddRow(
    1, 1, "TEST-001", "Test Asset", "equipment", "Description",
    now, timePtr(now.Add(24*time.Hour)), []byte(`{}`), true,
    now, now, nil,
)
```

**After (14 columns):**
```go
pgxmock.NewRows([]string{
    "id", "org_id", "identifier", "name", "type", "description",
    "current_location_id", "valid_from", "valid_to", "metadata", "is_active",
    "created_at", "updated_at", "deleted_at",
}).AddRow(
    1, 1, "TEST-001", "Test Asset", "equipment", "Description",
    nil, now, timePtr(now.Add(24*time.Hour)), []byte(`{}`), true,
    now, now, nil,
)
```

## Validation Criteria

- [ ] `go test ./internal/storage/... -v` passes all asset tests
- [ ] No `skip_asset_tests` build tag in assets_test.go
- [ ] No `t.Skip("TRA-212...")` calls remain
- [ ] `just backend test` passes

## Files to Modify

- `backend/internal/storage/assets_test.go`

## Conversation References

- Root cause: "current_location_id missing from test mocks, not deleted_at"
- Error message: "14 for columns 13" - Scan expects 14, mock provides 13
