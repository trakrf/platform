# Feature: Organization RBAC UI - Phase 3b: Members & Settings

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: frontend
**Type**: feature
**Priority**: High
**Phase**: 3b of 3 (Members & Settings)

## Outcome
Admins can view and manage organization members, change roles, remove members, and edit org settings.

## User Story
As an organization admin
I want to manage members and org settings
So that I can control who has access and what permissions they have

## Context
**Current**: Org switcher works (Phase 3a complete).
**Desired**: Member management and org settings screens.
**Depends On**: org-rbac-ui-switcher (Phase 3a)

---

## Technical Requirements

### 1. API Client Additions (lib/api/orgs.ts)

```typescript
// Add to existing orgsApi:
export const orgsApi = {
  // ... existing from Phase 3a ...

  // Org CRUD
  get: (orgId: number) => Promise<Organization>,
  update: (orgId: number, data: { name: string }) => Promise<Organization>,
  delete: (orgId: number, confirmName: string) => Promise<void>,

  // Members
  listMembers: (orgId: number) => Promise<OrgMember[]>,
  updateMemberRole: (orgId: number, userId: number, role: OrgRole) => Promise<void>,
  removeMember: (orgId: number, userId: number) => Promise<void>,
};
```

### 2. Types Additions

```typescript
export interface OrgMember {
  user_id: number;
  name: string;
  email: string;
  role: OrgRole;
  joined_at: string;
}
```

### 3. Components

#### MembersScreen.tsx
```typescript
// Route: #org-members
// Features:
// - Table: Name, Email, Role, Joined, Actions
// - Role dropdown (admin only) to change roles
// - Remove button (admin only)
// - Shows "You" badge on current user's row
// - Cannot demote/remove last admin (disabled)
// - Link to invite members (Phase 3c)
```

#### OrgSettingsScreen.tsx
```typescript
// Route: #org-settings
// Features:
// - Org name input (editable by admin)
// - Save button for name changes
// - Danger zone: Delete org button (admin only)
// - Delete opens DeleteOrgModal
```

#### DeleteOrgModal.tsx
```typescript
// Confirmation modal for org deletion
// - Warning text about consequences
// - Input: "Type {org_name} to confirm"
// - Delete button disabled until name matches exactly
// - Cancel button
```

### 4. Routes

```
#org-settings  → OrgSettingsScreen
#org-members   → MembersScreen
```

---

## File Structure

```
frontend/src/
├── lib/
│   ├── types/
│   │   └── org.ts           # MODIFY: Add OrgMember type
│   └── api/
│       └── orgs.ts          # MODIFY: Add member/org methods
├── screens/
│   ├── MembersScreen.tsx    # NEW
│   └── OrgSettingsScreen.tsx # NEW
├── components/
│   └── DeleteOrgModal.tsx   # NEW
└── App.tsx                  # MODIFY: Add routes
```

---

## Implementation Tasks

### Task 1: Types & API
- [ ] Add OrgMember type
- [ ] Add org CRUD methods to API client
- [ ] Add member methods to API client

### Task 2: MembersScreen
- [ ] Create screen with members table
- [ ] Implement role change dropdown
- [ ] Implement remove member
- [ ] Add "You" indicator for current user
- [ ] Disable last-admin protection

### Task 3: OrgSettingsScreen
- [ ] Create settings screen
- [ ] Implement name editing with save
- [ ] Add delete button (admin only)

### Task 4: DeleteOrgModal
- [ ] Create confirmation modal
- [ ] Implement name-match validation
- [ ] Handle deletion and redirect

### Task 5: Route Integration
- [ ] Add routes to App.tsx
- [ ] Add navigation links

---

## Validation Criteria

- [ ] Members table shows all org members
- [ ] Admin can change member roles
- [ ] Admin can remove members (except last admin)
- [ ] Current user row shows "You" badge
- [ ] Org name can be edited and saved
- [ ] Delete requires typing org name exactly
- [ ] Non-admin cannot see admin controls
- [ ] `just frontend validate` passes

## Success Metrics
- [ ] Member management works end-to-end
- [ ] Org settings work end-to-end
- [ ] Permission gating works correctly
