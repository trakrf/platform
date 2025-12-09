# Feature: Organization Member Management API

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: backend
**Type**: feature
**Priority**: Urgent
**Phase**: 2b of RBAC API (Member Management)

## Outcome
API endpoints for listing, updating roles, and removing organization members with RBAC enforcement.

## User Story
As an organization admin
I want to manage member roles and remove members via API
So that the frontend can build the member management UI

## Context
**Current**: Phase 2a complete - org CRUD and user context endpoints exist.
**Desired**: Member management endpoints with last-admin protection.
**Depends On**: org-rbac-api Phase 2a (complete)

---

## Technical Requirements

### API Endpoints

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs/:id/members` | List members with roles | RequireOrgMember |
| PUT | `/api/v1/orgs/:id/members/:userId` | Update member role | RequireOrgAdmin |
| DELETE | `/api/v1/orgs/:id/members/:userId` | Remove member | RequireOrgAdmin |

### List Members Response
```json
// GET /api/v1/orgs/:id/members
{
  "data": [
    {
      "user_id": 1,
      "name": "Mike Stankavich",
      "email": "mike@example.com",
      "role": "admin",
      "joined_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Update Role Request/Response
```json
// PUT /api/v1/orgs/:id/members/:userId
// Request:
{ "role": "manager" }

// Response (success):
{ "message": "Role updated" }

// Response (last admin):
{ "error": "Cannot demote the last admin" }
```

### Remove Member Response
```json
// DELETE /api/v1/orgs/:id/members/:userId
// Response (success):
{ "message": "Member removed" }

// Response (last admin):
{ "error": "Cannot remove the last admin" }

// Response (self-removal):
{ "error": "Cannot remove yourself" }
```

---

## Business Rules

1. **Last Admin Protection**
   - Cannot remove the last admin from an org
   - Cannot demote the last admin to a non-admin role
   - Check: `SELECT COUNT(*) FROM org_users WHERE org_id = ? AND role = 'admin'`

2. **Self-Removal Prevention**
   - Users cannot remove themselves (must have another admin do it)
   - Prevents accidental lockout

3. **Data Retention**
   - Removing a member only deletes from `org_users`
   - Assets/locations they created retain `created_by` reference
   - User keeps their personal org and other org memberships

4. **Valid Roles**
   - viewer, operator, manager, admin
   - Validate role in request body

---

## File Structure

```
backend/
├── internal/
│   ├── handlers/orgs/
│   │   └── members.go       # NEW: Member management handlers
│   ├── services/orgs/
│   │   └── service.go       # MODIFY: Add member business logic
│   └── storage/
│       └── org_users.go     # MODIFY: Add member queries
```

---

## Implementation Tasks

### Task 1: Storage Layer
- [ ] Add `ListOrgMembers(ctx, orgID)` to storage
- [ ] Add `UpdateMemberRole(ctx, orgID, userID, role)` to storage
- [ ] Add `RemoveMember(ctx, orgID, userID)` to storage
- [ ] Add `CountOrgAdmins(ctx, orgID)` to storage

### Task 2: Service Layer
- [ ] Add `ListMembers(ctx, orgID)` with member details
- [ ] Add `UpdateMemberRole(ctx, orgID, userID, role, actorID)` with last-admin check
- [ ] Add `RemoveMember(ctx, orgID, userID, actorID)` with last-admin and self-removal checks

### Task 3: Handlers
- [ ] Create `handlers/orgs/members.go`
- [ ] Implement GET /orgs/:id/members
- [ ] Implement PUT /orgs/:id/members/:userId
- [ ] Implement DELETE /orgs/:id/members/:userId

### Task 4: Route Registration
- [ ] Register member routes with RBAC middleware
- [ ] GET requires RequireOrgMember
- [ ] PUT/DELETE require RequireOrgAdmin

---

## Validation Criteria

- [ ] Any member can view member list
- [ ] Admin can change member roles
- [ ] Admin can remove members
- [ ] Cannot remove last admin (returns 400)
- [ ] Cannot demote last admin (returns 400)
- [ ] Cannot remove yourself (returns 400)
- [ ] Non-admin cannot update roles (returns 403)
- [ ] Non-admin cannot remove members (returns 403)
- [ ] `just backend validate` passes

## Success Metrics
- [ ] All 3 endpoints return correct responses
- [ ] RBAC middleware enforces access
- [ ] Last-admin protection working
- [ ] Edge cases handled with clear error messages
