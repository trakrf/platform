# Feature: Soft Delete Organization Name Collision Fix (TRA-196)

## Origin
When an organization is soft-deleted, its name remains in the database, blocking creation of a new org with the same name. This creates a poor UX when users want to reuse org names.

## Outcome
Soft-deleted organizations will have their names mangled to free up the original name for reuse, while preserving an audit trail.

## User Story
As an organization owner
I want to be able to reuse an organization name after deleting the old one
So that I can recreate my organization with the same branding

## Context

**Current Behavior**:
1. User creates org "Acme Corp"
2. User deletes org "Acme Corp" (soft delete - row remains with `deleted_at` timestamp)
3. User tries to create new org "Acme Corp"
4. **Error**: Name already exists (unique constraint violation)

**Desired Behavior**:
1. User creates org "Acme Corp"
2. User deletes org "Acme Corp"
3. Backend mangles name to `*** DELETED 2025-12-12T21:30:00Z *** Acme Corp`
4. User creates new org "Acme Corp" - **Success!**

## Technical Requirements

### Backend Changes

**File**: `backend/internal/services/orgs/service.go` (or storage layer)

On soft delete, mangle the org name:

```go
func (s *Service) Delete(ctx context.Context, orgID int64, confirmName string) error {
    // ... existing validation ...

    // Mangle the name to free it up for reuse
    deletedAt := time.Now().UTC()
    mangledName := fmt.Sprintf("*** DELETED %s *** %s",
        deletedAt.Format(time.RFC3339),
        org.Name)

    // Update name AND set deleted_at in same transaction
    return s.store.SoftDeleteWithNameMangle(ctx, orgID, mangledName, deletedAt)
}
```

### Database Considerations

- The `organizations` table likely has a unique constraint on `name`
- Constraint should exclude soft-deleted rows OR we mangle names (mangling is simpler)
- Mangled name format: `*** DELETED {ISO8601} *** {original_name}`
- Max name length may need consideration (100 char limit mentioned in validation)

### Edge Cases

- **Very long names**: If original name is 80+ chars, mangled name exceeds 100. Options:
  - Truncate original name in mangled version
  - Allow longer mangled names (they're not user-visible)
  - Use shorter prefix like `[DEL]`
- **Double deletion**: Can't happen (already deleted)
- **Name with special chars**: No issue, just prepend the prefix

## Validation Criteria
- [ ] Create org "Test Org"
- [ ] Delete org "Test Org"
- [ ] Verify in DB: name is `*** DELETED 2025-... *** Test Org`
- [ ] Create new org "Test Org" - succeeds
- [ ] Existing org deletion still works normally
- [ ] Existing tests still pass

## Related
- TRA-196: Parent issue (Organization bugs)
- Also see: `spec/active/invite-accept-redirect-fix/spec.md` for the other TRA-196 bug
