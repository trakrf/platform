# Feature: Fix Header Crash When User Has No Organizations

## Origin
This specification emerged from investigating Linear ticket TRA-207 and tracing the root cause through the codebase.

## Outcome
Users with no organization memberships can view the app without JavaScript errors. The OrgSwitcher component handles empty org lists gracefully.

## User Story
As a user who has been removed from all organizations
I want to log in without the app crashing
So that I can create or join a new organization

## Context
**Discovery**: The root cause is a Go/JSON serialization quirk - nil slices serialize to `null`, not `[]`.

**Current**:
- `storage/organizations.go:30` declares `var orgs []organization.UserOrg` (nil slice)
- When user has no orgs, nil is returned and JSON-serialized as `null`
- Frontend receives `{"orgs": null}` and stores it
- `OrgSwitcher.tsx:90` crashes: `orgs.map(...)` → TypeError on null

**Desired**:
- Backend always returns `[]` for empty org lists
- Frontend handles edge case defensively (belt and suspenders)

## Technical Requirements

### Backend (Primary Fix)
- Change `storage/organizations.go:30` from:
  ```go
  var orgs []organization.UserOrg
  ```
  to:
  ```go
  orgs := []organization.UserOrg{}
  ```
- This ensures JSON serialization produces `[]` not `null`

### Frontend (Defensive)
- Add null-coalescing in `OrgSwitcher.tsx:90`:
  ```tsx
  {(orgs ?? []).map(org => (
  ```
- Consider similar defensive checks in other components using `orgs`

## Validation Criteria
- [ ] Backend `/api/v1/users/me` returns `"orgs": []` (not `null`) for user with no memberships
- [ ] Frontend renders without crash when orgs is empty array
- [ ] OrgSwitcher shows "No organization" state with create option
- [ ] Existing users with orgs unaffected (regression check)

## Test Scenarios
1. **User removed from only org**: Admin removes user → user logs in → sees "No organization" UI
2. **New user via invite signup**: Signs up without personal org → joins invited org → works normally
3. **API response format**: `GET /users/me` returns `[]` not `null` for orgs field

## Out of Scope
- UX improvements for "no org" state (separate ticket)
- Automatic redirect to "create org" flow (separate ticket)

## Conversation References
- Linear ticket: TRA-207
- Root cause: Go nil slice → JSON null serialization
- Decision: Fix at source (backend) + defensive frontend check
