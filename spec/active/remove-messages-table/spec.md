# Feature: Remove Unused Messages Table

## Linear Issue
TRA-101: https://linear.app/trakrf/issue/TRA-101/remove-unused-messages-table-duplicate-of-identifier-scans

## Outcome
**Clean up duplicate MQTT message processing by removing the unused `messages` table and consolidating to `identifier_scans` as the single source of truth.**

## Context

### Current State
- Two tables exist for MQTT message processing: `messages` and `identifier_scans`
- Both have similar triggers (`process_messages()` and `process_identifier_scans()`)
- `identifier_scans` is the active table being used
- `messages` table appears to be unused legacy code

### Problem
- Code duplication confuses developers
- Unnecessary database schema complexity
- Both tables reference `o.domain` (now `o.identifier`) needing updates
- Maintenance burden for unused code

### Discovery
Found during auth-personal-orgs implementation when updating database references from `domain` to `identifier`. The `messages` table has the same `o.domain` reference that needs updating, but investigation shows it's not actually being used.

## Technical Requirements

### Step 1: Verify Unused
- [x] Grep codebase for references to `messages` table
- [x] Check for any INSERT/SELECT/UPDATE queries using messages
- [x] Verify no application code depends on messages table
- [x] Confirm identifier_scans is the active pipeline

### Step 2: Remove Migration Files
- [ ] Delete `database/migrations/000012_messages.up.sql`
- [ ] Delete `database/migrations/000012_messages.down.sql`
- [ ] Leave gap in migration numbering (000012 will be skipped)
- [ ] No need to renumber subsequent migrations (greenfield project)

### Step 3: Validation
- [ ] Run `just db-reset` to verify clean database creation
- [ ] Verify no errors in migration sequence
- [ ] Confirm identifier_scans table and trigger still work correctly
- [ ] Run full test suite to ensure nothing breaks

## Implementation Details

### Files to Delete
```
database/migrations/000012_messages.up.sql
database/migrations/000012_messages.down.sql
```

### Migration Numbering Strategy
**Leave the gap** - Don't renumber migrations because:
1. Greenfield project (no production deployments)
2. Simpler to track what was removed (gap = removed migration)
3. Less risk of errors from renumbering multiple files
4. Migration tools handle gaps gracefully

### Verification Commands
```bash
# Verify no code references (should return empty)
grep -r "messages" backend/ frontend/ --include="*.go" --include="*.ts" --include="*.tsx"

# Check migration files exist before deletion
ls -la database/migrations/000012_*

# After deletion, reset database
just db-reset

# Verify migrations run cleanly
just database migrate-status
```

## Validation Criteria

### Functional Requirements
- [ ] messages table no longer exists in fresh database
- [ ] identifier_scans table and trigger work correctly
- [ ] No application code references messages table
- [ ] Migration sequence runs without errors

### Testing Requirements
- [ ] `just db-reset` succeeds
- [ ] Full backend test suite passes
- [ ] No errors in migration logs

## Out of Scope

- Migrating data (table is unused, no data to migrate)
- Renumbering subsequent migrations (keeping gap in numbering)
- Adding new features to identifier_scans (this is pure cleanup)

## Success Metrics

**Before:** 2 similar tables for MQTT message processing
**After:** 1 table (identifier_scans) as single source of truth

**Developer Experience:**
- Clearer data pipeline (no duplicate tables)
- Less maintenance burden
- Simpler schema to understand

## Notes

- **Greenfield advantage**: Zero production deployments means we can delete freely
- **No backward compatibility needed**: No users, no deployed systems, no legacy data
- **Simple cleanup**: Just delete the files and move on
- **Gap in numbering is intentional**: Makes it clear something was removed

## Estimated Effort
5-10 minutes (verification + deletion + validation)
