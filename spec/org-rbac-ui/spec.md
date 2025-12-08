# Feature: Organization RBAC - Phase 3: Frontend UI

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: frontend
**Type**: feature
**Priority**: Urgent
**Phase**: 3 of 3 (Frontend Components)

## Outcome
Users can switch organizations, manage members, send invitations, and accept invitations through the UI.

## User Story
As a TrakRF user
I want to manage my organizations and team members through the UI
So that I can collaborate with my team and control access to assets

## Context
**Current**: No org switcher, no member management UI, no invitation flow.
**Desired**: Full org management UI with switcher, settings, members, and invitations.
**Depends On**: org-rbac-api (Phase 2 must be complete)

## Builds On
- **org-rbac-db** (Phase 1) - Database schema
- **org-rbac-api** (Phase 2) - API endpoints

---

## Technical Requirements

### 1. Components

#### OrgSwitcher.tsx
Header dropdown showing current org and allowing org switching.

```typescript
// Features:
// - Shows current org name in header
// - Dropdown lists all user's orgs with role badges
// - "Create Organization" option at bottom
// - Selecting org calls POST /users/me/current-org and refreshes data
```

#### RoleBadge.tsx
Visual indicator for user roles.

```typescript
interface RoleBadgeProps {
  role: OrgRole;
  size?: 'sm' | 'md';
}

// Colors:
// - admin: red/rose
// - manager: blue
// - operator: green
// - viewer: gray
```

#### OrgSettingsScreen.tsx
Organization settings page (admin only).

```typescript
// Features:
// - Editable org name
// - Delete org button (admin only)
// - Delete requires typing org name exactly (modal)
```

#### MembersScreen.tsx
Member management page.

```typescript
// Features:
// - Table: Name, Email, Role, Actions
// - Role dropdown (admin only) to change roles
// - Remove button (admin only)
// - Shows "You" badge on current user's row
// - Pending invitations tab/section
```

#### InvitationsSection.tsx
Pending invitations within MembersScreen.

```typescript
// Features:
// - Table: Email, Role, Invited By, Expires, Actions
// - Cancel button per row
// - Resend button per row
// - Invite button opens InviteModal
```

#### InviteModal.tsx
Modal for sending invitations.

```typescript
// Features:
// - Email input (validated)
// - Role dropdown (viewer, operator, manager, admin)
// - Send button
// - Loading state
// - Error display (already member, pending invite)
```

#### AcceptInviteScreen.tsx
Invitation acceptance page.

```typescript
// Route: #accept-invite?token=xxx

// States:
// 1. Loading: Validating token...
// 2. Valid + logged in: "You've been invited to join {org} as {role}. [Accept] [Decline]"
// 3. Valid + not logged in: "Please log in to accept this invitation" [Go to Login]
// 4. Invalid/expired: "This invitation is invalid or has expired" [Request new invite]
// 5. Success: Redirect to org with toast
```

#### DeleteOrgModal.tsx
Confirmation modal for org deletion.

```typescript
// Features:
// - Warning text explaining consequences
// - Input field: "Type {org_name} to confirm"
// - Delete button (disabled until name matches)
// - Cancel button
```

---

### 2. Routes

```
#org-settings        → OrgSettingsScreen
#org-members         → MembersScreen
#accept-invite       → AcceptInviteScreen (with ?token=xxx)
```

---

### 3. State Management (Zustand)

#### orgStore.ts
```typescript
interface Organization {
  id: number;
  name: string;
}

interface OrgMembership {
  org: Organization;
  role: OrgRole;
}

interface OrgState {
  // Current org context
  currentOrg: Organization | null;
  currentRole: OrgRole | null;

  // All user's orgs
  userOrgs: OrgMembership[];

  // Actions
  setCurrentOrg: (orgId: number) => Promise<void>;
  fetchUserOrgs: () => Promise<void>;
  createOrg: (name: string) => Promise<Organization>;

  // Helpers
  isAdmin: () => boolean;
  canManageAssets: () => boolean;
  canScan: () => boolean;
}
```

---

### 4. API Client

#### lib/api/orgs.ts
```typescript
export const orgsApi = {
  // Orgs
  list: () => Promise<OrgMembership[]>,
  create: (name: string) => Promise<Organization>,
  get: (orgId: number) => Promise<Organization>,
  update: (orgId: number, data: { name: string }) => Promise<Organization>,
  delete: (orgId: number, confirmName: string) => Promise<void>,

  // Members
  listMembers: (orgId: number) => Promise<Member[]>,
  updateMemberRole: (orgId: number, userId: number, role: OrgRole) => Promise<void>,
  removeMember: (orgId: number, userId: number) => Promise<void>,

  // Invitations
  listInvitations: (orgId: number) => Promise<Invitation[]>,
  createInvitation: (orgId: number, email: string, role: OrgRole) => Promise<Invitation>,
  cancelInvitation: (orgId: number, inviteId: number) => Promise<void>,
  resendInvitation: (orgId: number, inviteId: number) => Promise<void>,
  acceptInvitation: (token: string) => Promise<{ org_id: number }>,
};
```

---

### 5. Integration Points

#### Header Integration
- Add OrgSwitcher to app header/navigation
- Show current org name
- Dropdown on click

#### Auth Store Update
- On login success, fetch user orgs
- Set currentOrg to last_org_id or first org
- On org switch, update last_org_id via API

#### Permission Gating
- Use orgStore.isAdmin() to show/hide admin-only UI
- Use orgStore.canManageAssets() for asset CRUD buttons
- Use orgStore.canScan() for scan buttons

---

## File Structure

```
frontend/
├── src/
│   ├── components/
│   │   ├── OrgSwitcher.tsx       # NEW
│   │   ├── InviteModal.tsx       # NEW
│   │   ├── DeleteOrgModal.tsx    # NEW
│   │   └── RoleBadge.tsx         # NEW
│   ├── screens/
│   │   ├── OrgSettingsScreen.tsx # NEW
│   │   ├── MembersScreen.tsx     # NEW
│   │   └── AcceptInviteScreen.tsx # NEW
│   ├── stores/
│   │   └── orgStore.ts           # NEW
│   ├── lib/api/
│   │   └── orgs.ts               # NEW
│   └── App.tsx                   # MODIFY: Add routes
```

---

## Implementation Tasks

### Task 1: API Client & Types
- [ ] Create `lib/api/orgs.ts` with all API methods
- [ ] Define TypeScript types for Org, Member, Invitation, OrgRole

### Task 2: Org Store
- [ ] Create `stores/orgStore.ts` with Zustand
- [ ] Implement currentOrg, userOrgs state
- [ ] Implement permission helper methods

### Task 3: OrgSwitcher Component
- [ ] Create dropdown component
- [ ] Integrate with orgStore
- [ ] Add to app header

### Task 4: RoleBadge Component
- [ ] Create badge with role colors
- [ ] Support sm/md sizes

### Task 5: OrgSettingsScreen
- [ ] Create settings page
- [ ] Implement name editing
- [ ] Implement delete with confirmation modal

### Task 6: MembersScreen
- [ ] Create members table
- [ ] Implement role change dropdown
- [ ] Implement member removal
- [ ] Add invitations section

### Task 7: Invitation Flow
- [ ] Create InviteModal
- [ ] Create AcceptInviteScreen
- [ ] Handle all acceptance states

### Task 8: Route Integration
- [ ] Add routes to App.tsx
- [ ] Gate admin routes by role

---

## Validation Criteria

### Org Switching
- [ ] Header shows current org name
- [ ] Dropdown shows all user orgs with role badges
- [ ] Clicking org switches context
- [ ] Data refreshes after org switch
- [ ] "Create Organization" opens creation flow

### Org Settings
- [ ] Admin can edit org name
- [ ] Admin can delete org by typing name
- [ ] Non-admin cannot see delete button
- [ ] Non-admin sees read-only settings

### Members
- [ ] All members visible in table
- [ ] Admin can change member roles via dropdown
- [ ] Admin can remove members
- [ ] Cannot remove/demote last admin (button disabled or error)
- [ ] Current user row shows "You" indicator

### Invitations
- [ ] Admin can open invite modal
- [ ] Email validation on input
- [ ] Role selection works
- [ ] Success shows toast, closes modal
- [ ] "Already member" error displayed
- [ ] "Pending invite" error displayed
- [ ] Pending invitations table shows correct data
- [ ] Cancel/resend buttons work

### Accept Invite Flow
- [ ] Token extracted from URL
- [ ] Loading state while validating
- [ ] Accept button works when logged in
- [ ] Redirect to login when not logged in
- [ ] Expired token shows error
- [ ] Success redirects to org

---

## Success Metrics
- [ ] All UI components render without errors
- [ ] Org switching works end-to-end
- [ ] Member management works end-to-end
- [ ] Invitation flow works end-to-end
- [ ] No console errors during flows
- [ ] `just frontend validate` passes
- [ ] Responsive on mobile viewports

## References
- [TRA-136 Linear Issue](https://linear.app/trakrf/issue/TRA-136)
- [org-rbac-db](../org-rbac-db/spec.md) - Phase 1
- [org-rbac-api](../org-rbac-api/spec.md) - Phase 2 (prerequisite)
- GitHub organization UI for UX patterns
