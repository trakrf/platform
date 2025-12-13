# Feature: Fix Invitation Accept Redirect Flow (TRA-183, TRA-196)

## Origin
User clicked invitation email link `https://app.trakrf.id/#accept-invite?token=...` and was redirected to home page instead of seeing the invite acceptance screen. This is Bug #1 in TRA-196.

## Outcome
Users clicking invitation links will see the AcceptInviteScreen, and after logging in (if needed) will return to complete the invitation acceptance.

## User Story
As an invited user
I want to click the invitation link and accept the invitation
So that I can join the organization I was invited to

## Context

**Discovery**: The AcceptInviteScreen component exists and is correctly wired in App.tsx routing. The component properly handles unauthenticated users by showing login/signup buttons that link to `#login?returnTo=accept-invite&token=xyz`.

**Root Cause**: There's a disconnect between two redirect mechanisms:
1. AcceptInviteScreen passes `returnTo` and `token` as **URL query params**
2. LoginScreen/handleAuthRedirect() only checks **sessionStorage.redirectAfterLogin**
3. URL query params are **never read** - redirect falls back to `#home`

**Current Flow (Broken)**:
1. User clicks `#accept-invite?token=abc123`
2. AcceptInviteScreen shows (not logged in) with "Sign In" button
3. User clicks → navigates to `#login?returnTo=accept-invite&token=abc123`
4. User logs in successfully
5. `handleAuthRedirect()` checks `sessionStorage.redirectAfterLogin` → **empty!**
6. Falls back to `#home` (token lost, invite not processed)

**Desired Flow**:
1. User clicks `#accept-invite?token=abc123`
2. AcceptInviteScreen shows with "Sign In" button
3. User clicks → navigates to `#login?returnTo=accept-invite&token=abc123`
4. User logs in successfully
5. `handleAuthRedirect()` reads `returnTo` and `token` from URL params
6. Redirects to `#accept-invite?token=abc123`
7. AcceptInviteScreen shows "Accept" button (now authenticated)
8. User accepts → joins org

## Technical Requirements

### Option A: Enhance handleAuthRedirect (Recommended)
Modify `handleAuthRedirect()` to check URL query params first:

```typescript
// src/utils/authRedirect.ts
export function handleAuthRedirect(): void {
  // Check URL params first (from AcceptInviteScreen)
  const hash = window.location.hash.slice(1);
  const queryIndex = hash.indexOf('?');
  if (queryIndex !== -1) {
    const params = new URLSearchParams(hash.slice(queryIndex + 1));
    const returnTo = params.get('returnTo');
    const token = params.get('token');

    if (returnTo) {
      // Build redirect with preserved params
      const redirectHash = token
        ? `#${returnTo}?token=${encodeURIComponent(token)}`
        : `#${returnTo}`;
      window.location.hash = redirectHash;
      return;
    }
  }

  // Fall back to sessionStorage (from ProtectedRoute)
  const redirect = sessionStorage.getItem('redirectAfterLogin');
  if (redirect) {
    sessionStorage.removeItem('redirectAfterLogin');
    window.location.hash = `#${redirect}`;
  } else {
    window.location.hash = '#home';
  }
}
```

### Option B: Store in sessionStorage before redirect
Have AcceptInviteScreen save to sessionStorage before redirecting to login:

```typescript
// In AcceptInviteScreen, before rendering login link
sessionStorage.setItem('redirectAfterLogin', `accept-invite?token=${token}`);
```

**Recommendation**: Option A is cleaner - keeps state in URL (shareable, bookmarkable) rather than hidden in sessionStorage.

### Files to Modify
1. `frontend/src/utils/authRedirect.ts` - Read URL query params
2. `frontend/src/utils/__tests__/authRedirect.test.ts` - Add tests for URL param handling
3. `frontend/src/components/SignupScreen.tsx` - Verify it also uses handleAuthRedirect

### Edge Cases
- Token is URL-encoded (contains special chars) - must decode/re-encode properly
- User refreshes login page - params should persist in URL
- User navigates away then back - sessionStorage fallback still works
- Invalid/expired token - AcceptInviteScreen already handles this with error message

## Validation Criteria
- [ ] Click invite link → see AcceptInviteScreen (not logged in)
- [ ] Click "Sign In" → login page shows
- [ ] Log in → redirected back to `#accept-invite?token=...`
- [ ] AcceptInviteScreen shows "Accept" button (authenticated)
- [ ] Click "Accept" → join org successfully
- [ ] Same flow works for "Create Account" (signup)
- [ ] Unit tests pass for handleAuthRedirect with URL params

## Conversation References
- Bug discovered: "I sent an invitation, received the email, clicked the link... but just went to home page"
- URL format from email: `https://app.trakrf.id/#accept-invite?token=8a89c23d...`
- Linear issue: TRA-183

## Related
- TRA-196: Parent issue (Organization bugs) - this spec addresses Bug #1
- TRA-181: Members screen null crash (fixed) - same area of code
- Backend: Invitation system, email sending already working
- AcceptInviteScreen.tsx: Component exists and handles auth states correctly
- Also see: `spec/active/org-soft-delete-name/spec.md` for Bug #3 in TRA-196
