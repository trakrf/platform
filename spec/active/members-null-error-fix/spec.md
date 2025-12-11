# Feature: Fix MembersScreen Null Reference Error (TRA-181)

## Origin
This specification addresses runtime error TRA-181 discovered in production on the organization members screen.

## Outcome
The Members API will return `[]` instead of `null` for empty collections (REST best practice), and the frontend will defensively handle null responses.

## User Story
As a user viewing organization members
I want the members screen to load without crashing
So that I can manage my organization's membership

## Context

**Discovery**: Production error reported - users see a blank screen with console error when navigating to Members tab.

**Error**:
```
TypeError: Cannot read properties of null (reading 'length')
```

**Root Cause Chain**:
1. `backend/internal/storage/org_users.go:98` declares `var members []organization.OrgMember` (nil slice)
2. If no rows returned, slice stays nil
3. Go's `json.Marshal(nil)` produces `null`, not `[]`
4. Frontend receives `{ "data": null }` instead of `{ "data": [] }`
5. `MembersScreen.tsx:41` does `setMembers(response.data.data)` - sets state to null
6. Lines 173, 269 call `members.length` on null → TypeError

**REST Best Practice**: Collections should return `[]`, not `null`. Null means "absence of value" (for scalars), while `[]` means "list with zero items".

## Technical Requirements

### Backend Fix (Primary)
Initialize slice to empty instead of nil in storage layer.

**File**: `backend/internal/storage/org_users.go:98`

**Before**:
```go
var members []organization.OrgMember
```

**After**:
```go
members := []organization.OrgMember{}
```

This ensures JSON serialization produces `[]` not `null`.

### Frontend Fix (Defense in Depth)
Add null coalescing when setting members state.

**File**: `frontend/src/components/MembersScreen.tsx:41`

**Before**:
```typescript
setMembers(response.data.data);
```

**After**:
```typescript
setMembers(response.data.data ?? []);
```

## Validation Criteria

- [ ] Backend: `GET /api/v1/orgs/:id/members` returns `{ "data": [] }` for org with no members
- [ ] Backend: Existing members endpoint tests pass
- [ ] Frontend: Members screen loads without error when API returns `{ "data": null }`
- [ ] Frontend: Members screen loads without error when API returns `{ "data": [] }`
- [ ] Frontend: Members screen displays correctly with actual member data
- [ ] TypeScript compiles without errors
- [ ] All existing tests pass

## Files to Modify

1. `backend/internal/storage/org_users.go` - Line 98
2. `frontend/src/components/MembersScreen.tsx` - Line 41

## Risk Assessment

**Low risk** - Two single-line changes with defensive handling. No behavior change for happy path (orgs with members).

## References

- [JSON:API spec on empty collections](https://github.com/json-api/json-api/issues/101)
- [CERT Java: Return empty array instead of null](https://wiki.sei.cmu.edu/confluence/display/java/MET55-J.+Return+an+empty+array+or+collection+instead+of+a+null+value+for+methods+that+return+an+array+or+collection)
