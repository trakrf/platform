# Feature: Organization RBAC UI - Phase 3c: Invitations

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: frontend
**Type**: feature
**Priority**: High
**Phase**: 3c of 3 (Invitations)

## Outcome
Admins can send invitations, manage pending invites, and users can accept invitations to join organizations.

## User Story
As an organization admin
I want to invite users by email
So that they can join my organization

As an invited user
I want to accept an invitation via link
So that I can join the organization

## Context
**Current**: Members screen exists (Phase 3b complete).
**Desired**: Full invitation flow with send, manage, and accept.
**Depends On**: org-rbac-ui-members (Phase 3b)

---

## Technical Requirements

### 1. API Client Additions (lib/api/orgs.ts)

```typescript
// Add to existing orgsApi:
export const orgsApi = {
  // ... existing from Phase 3a/3b ...

  // Invitations
  listInvitations: (orgId: number) => Promise<Invitation[]>,
  createInvitation: (orgId: number, email: string, role: OrgRole) => Promise<Invitation>,
  cancelInvitation: (orgId: number, inviteId: number) => Promise<void>,
  resendInvitation: (orgId: number, inviteId: number) => Promise<void>,

  // Accept (auth endpoint)
  acceptInvitation: (token: string) => Promise<AcceptInvitationResponse>,
};
```

### 2. Types Additions

```typescript
export interface Invitation {
  id: number;
  email: string;
  role: OrgRole;
  invited_by: { id: number; name: string } | null;
  expires_at: string;
  created_at: string;
}

export interface AcceptInvitationResponse {
  message: string;
  org_id: number;
  org_name: string;
  role: string;
}
```

### 3. Components

#### InvitationsSection.tsx
```typescript
// Section within MembersScreen (or tab)
// Features:
// - "Invite Member" button opens InviteModal
// - Table: Email, Role, Invited By, Expires, Actions
// - Cancel button per row
// - Resend button per row
```

#### InviteModal.tsx
```typescript
// Modal for sending invitations
// Features:
// - Email input (validated)
// - Role dropdown (viewer, operator, manager, admin)
// - Send button
// - Loading state
// - Error display (already member, pending invite)
// - Success closes modal and refreshes list
```

#### AcceptInviteScreen.tsx
```typescript
// Route: #accept-invite?token=xxx
// States:
// 1. Loading: "Validating invitation..."
// 2. Valid + logged in: Show org name, role, Accept/Decline buttons
// 3. Valid + not logged in: "Please log in" with login link (preserve token)
// 4. Invalid/expired: Error message
// 5. Already member: "You're already a member"
// 6. Success: Redirect to dashboard with toast
```

### 4. Routes

```
#accept-invite?token=xxx → AcceptInviteScreen
```

---

## File Structure

```
frontend/src/
├── lib/
│   ├── types/
│   │   └── org.ts              # MODIFY: Add Invitation types
│   └── api/
│       └── orgs.ts             # MODIFY: Add invitation methods
├── screens/
│   ├── MembersScreen.tsx       # MODIFY: Add InvitationsSection
│   └── AcceptInviteScreen.tsx  # NEW
├── components/
│   ├── InvitationsSection.tsx  # NEW
│   └── InviteModal.tsx         # NEW
└── App.tsx                     # MODIFY: Add accept-invite route
```

---

## Implementation Tasks

### Task 1: Types & API
- [ ] Add Invitation, AcceptInvitationResponse types
- [ ] Add invitation methods to API client

### Task 2: InvitationsSection
- [ ] Create section with invitations table
- [ ] Implement cancel button
- [ ] Implement resend button
- [ ] Add "Invite Member" button

### Task 3: InviteModal
- [ ] Create modal with email and role inputs
- [ ] Implement email validation
- [ ] Handle API errors (already member, pending)
- [ ] Success flow

### Task 4: AcceptInviteScreen
- [ ] Create screen with token extraction
- [ ] Implement all states (loading, valid, invalid, etc.)
- [ ] Handle logged-in vs not-logged-in
- [ ] Implement accept flow with redirect

### Task 5: Integration
- [ ] Add InvitationsSection to MembersScreen
- [ ] Add accept-invite route to App.tsx

---

## Validation Criteria

- [ ] Invite modal validates email
- [ ] Invite modal shows role options
- [ ] Invitation sent successfully
- [ ] "Already member" error displayed
- [ ] "Pending invite" error displayed
- [ ] Pending invitations table shows data
- [ ] Cancel invitation works
- [ ] Resend invitation works
- [ ] Accept screen extracts token from URL
- [ ] Accept works when logged in
- [ ] Redirect to login when not logged in
- [ ] Invalid/expired token shows error
- [ ] Success redirects to org
- [ ] `just frontend validate` passes

## Success Metrics
- [ ] Full invitation flow works end-to-end
- [ ] All error states handled gracefully
- [ ] No console errors
