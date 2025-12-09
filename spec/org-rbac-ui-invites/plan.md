# Implementation Plan: Organization RBAC UI - Phase 3c (Invitations)

Generated: 2025-12-09
Specification: spec.md

## Understanding

Add invitation functionality to the org RBAC UI:
1. InvitationsSection - table of pending invites with cancel/resend actions
2. InviteModal - form to send new invitations (email + role)
3. AcceptInviteScreen - handle invitation acceptance via token URL

Key decisions from planning:
- Stacked layout: InvitationsSection below members table
- Show invitation details to unauthenticated users with login/signup buttons
- Exclude "owner" from invite roles (must be promoted after joining)
- Basic email validation client-side, backend handles member/invite conflicts

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/MembersScreen.tsx` - Table pattern, error handling, admin checks
- `frontend/src/components/DeleteOrgModal.tsx` - Modal pattern with form
- `frontend/src/App.tsx` (lines 48-60, 224-236) - URL token extraction pattern

**Files to Create**:
- `frontend/src/components/InvitationsSection.tsx` - Pending invites table
- `frontend/src/components/InviteModal.tsx` - Send invitation modal
- `frontend/src/components/AcceptInviteScreen.tsx` - Accept invitation flow

**Files to Modify**:
- `frontend/src/components/MembersScreen.tsx` (lines 274-280) - Replace placeholder with InvitationsSection
- `frontend/src/App.tsx` - Add accept-invite route
- `frontend/src/stores/uiStore.ts` - Add 'accept-invite' to TabType

## Architecture Impact
- **Subsystems affected**: UI only (API/types already exist)
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add Route for AcceptInviteScreen

**Files**: `frontend/src/stores/uiStore.ts`, `frontend/src/App.tsx`

**Action**: MODIFY

**Implementation**:

1. Update TabType in `uiStore.ts`:
```typescript
export type TabType = '...' | 'accept-invite';
```

2. Add lazy import in `App.tsx`:
```typescript
const AcceptInviteScreen = lazyWithRetry(() => import('@/components/AcceptInviteScreen'));
```

3. Add to `VALID_TABS`:
```typescript
const VALID_TABS: TabType[] = [..., 'accept-invite'];
```

4. Add to `tabComponents`:
```typescript
'accept-invite': AcceptInviteScreen,
```

5. Add to `loadingScreens`:
```typescript
'accept-invite': LoadingScreen,
```

6. Modify `renderTabContent()` to pass token to AcceptInviteScreen (similar to reset-password):
```typescript
{activeTab === 'reset-password' ? (
  <Component token={token} />
) : activeTab === 'accept-invite' ? (
  <Component token={token} />
) : (
  <Component />
)}
```

**Validation**: `just frontend typecheck`

---

### Task 2: Create InviteModal

**File**: `frontend/src/components/InviteModal.tsx`

**Action**: CREATE

**Pattern**: Reference `DeleteOrgModal.tsx` for modal structure

**Implementation**:
```typescript
interface InviteModalProps {
  onClose: () => void;
  onSuccess: () => void;
  orgId: number;
}

// Features:
// - Email input with basic format validation
// - Role dropdown (admin, manager, operator, viewer - NO owner)
// - Send button with loading state
// - Error display for API errors
// - Success calls onSuccess and closes

const INVITE_ROLES: OrgRole[] = ['admin', 'manager', 'operator', 'viewer'];

// Basic email validation
const isValidEmail = (email: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 3: Create InvitationsSection

**File**: `frontend/src/components/InvitationsSection.tsx`

**Action**: CREATE

**Pattern**: Reference `MembersScreen.tsx` table pattern

**Implementation**:
```typescript
interface InvitationsSectionProps {
  orgId: number;
  isAdmin: boolean;
}

// Features:
// - Fetch invitations on mount
// - Table: Email, Role, Invited By, Expires, Actions
// - Cancel button per row (with confirmation or immediate)
// - Resend button per row
// - "Invite Member" button opens InviteModal
// - Empty state when no pending invitations

// Format expiry date with "Expires in X days" or "Expired"
const formatExpiry = (expiresAt: string) => {
  const now = new Date();
  const expiry = new Date(expiresAt);
  const diffDays = Math.ceil((expiry.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
  if (diffDays < 0) return 'Expired';
  if (diffDays === 0) return 'Expires today';
  if (diffDays === 1) return 'Expires tomorrow';
  return `Expires in ${diffDays} days`;
};
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 4: Create AcceptInviteScreen

**File**: `frontend/src/components/AcceptInviteScreen.tsx`

**Action**: CREATE

**Pattern**: Reference `ResetPasswordScreen.tsx` for token handling

**Implementation**:
```typescript
interface AcceptInviteScreenProps {
  token: string | null;
}

// States to handle:
// 1. No token: Show error "Invalid invitation link"
// 2. Loading: "Validating invitation..." (call API to validate)
// 3. Valid + logged in: Show org name, role, Accept/Decline buttons
// 4. Valid + not logged in: Show org name, role, Login/Signup buttons
// 5. Invalid/expired: Error message
// 6. Already member: "You're already a member of this organization"
// 7. Success: Redirect to home with toast

// For not-logged-in state, preserve token in URL:
// Login link: #login?returnTo=accept-invite&token=xxx
// Signup link: #signup?returnTo=accept-invite&token=xxx

// Note: We need to validate the token without accepting it first
// Check if backend has a GET endpoint to validate, or just show org/role from token
// If no validation endpoint, show generic "You've been invited" and reveal details after accept
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 5: Integrate InvitationsSection into MembersScreen

**File**: `frontend/src/components/MembersScreen.tsx`

**Action**: MODIFY

**Changes**:
1. Import InvitationsSection
2. Replace placeholder (lines 274-280) with InvitationsSection component

```typescript
// Replace:
{isAdmin && (
  <div className="mt-6 pt-6 border-t border-gray-700 text-center">
    <p className="text-gray-500 text-sm">
      Invite new members coming in Phase 3c
    </p>
  </div>
)}

// With:
{isAdmin && currentOrg && (
  <InvitationsSection orgId={currentOrg.id} isAdmin={isAdmin} />
)}
```

**Validation**: `just frontend typecheck && just frontend lint`

---

## Risk Assessment

- **Risk**: Token validation without dedicated endpoint
  **Mitigation**: Show generic invite message, reveal details after accept attempt. Backend returns org info on accept.

- **Risk**: Return URL handling for login/signup
  **Mitigation**: Use hash params (returnTo, token) - verify login/signup screens handle returnTo

## Integration Points

- Store updates: None (using existing orgsApi)
- Route changes: Add 'accept-invite' route
- Config updates: None

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
- Gate 1: `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `just frontend test` (pre-existing failures acceptable)

**Do not proceed to next task until current task passes gates 1 and 2.**

## Validation Sequence

After each task: `just frontend typecheck && just frontend lint`

Final validation: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase (MembersScreen, DeleteOrgModal)
✅ All clarifying questions answered
✅ API methods already exist
✅ Types already defined
⚠️ Token validation flow may need adjustment based on backend behavior

**Assessment**: Straightforward UI implementation following established patterns. API layer complete.

**Estimated one-pass success probability**: 85%

**Reasoning**: All infrastructure exists. Main uncertainty is the accept-invite flow for unauthenticated users - may need minor adjustments based on backend behavior.
