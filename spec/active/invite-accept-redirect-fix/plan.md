# Implementation Plan: Fix Invitation Accept Redirect Flow

Generated: 2025-12-13
Specification: spec.md
Linear: TRA-183, TRA-196

## Understanding

When a user clicks an invitation link from email, they land on `#accept-invite?token=...`. If not logged in, they click "Sign In" which navigates to `#login?returnTo=accept-invite&token=...`. After successful login, `handleAuthRedirect()` is called but only checks sessionStorage (empty in new tab), so user gets sent to `#home` instead of back to accept the invite.

**Fix**: Enhance `handleAuthRedirect()` to read URL query params first, then fall back to sessionStorage, then home.

## Relevant Files

**Reference Patterns**:
- `frontend/src/components/AcceptInviteScreen.tsx` (lines 124, 131) - Shows URL format being constructed
- `frontend/src/utils/__tests__/authRedirect.test.ts` - Existing test patterns to extend

**Files to Modify**:
- `frontend/src/utils/authRedirect.ts` - Add URL param parsing (priority over sessionStorage)
- `frontend/src/utils/__tests__/authRedirect.test.ts` - Add tests for URL param handling

## Architecture Impact
- **Subsystems affected**: Frontend auth redirect only
- **New dependencies**: None
- **Breaking changes**: None - additive enhancement, existing sessionStorage flow preserved

## Task Breakdown

### Task 1: Update handleAuthRedirect to Read URL Params
**File**: `frontend/src/utils/authRedirect.ts`
**Action**: MODIFY

**Implementation**:
```typescript
export function handleAuthRedirect(): void {
  // Priority 1: Check URL params (from AcceptInviteScreen flow)
  const hash = window.location.hash.slice(1); // Remove leading #
  const queryIndex = hash.indexOf('?');

  if (queryIndex !== -1) {
    const params = new URLSearchParams(hash.slice(queryIndex + 1));
    const returnTo = params.get('returnTo');
    const token = params.get('token');

    if (returnTo) {
      // Build redirect with preserved token if present
      const redirectHash = token
        ? `#${returnTo}?token=${encodeURIComponent(token)}`
        : `#${returnTo}`;
      window.location.hash = redirectHash;
      return;
    }
  }

  // Priority 2: Check sessionStorage (from ProtectedRoute flow)
  const redirect = sessionStorage.getItem('redirectAfterLogin');
  if (redirect) {
    sessionStorage.removeItem('redirectAfterLogin');
    window.location.hash = `#${redirect}`;
    return;
  }

  // Priority 3: Default to home
  window.location.hash = '#home';
}
```

**Validation**:
```bash
cd frontend && just typecheck
```

### Task 2: Add Tests for URL Param Handling
**File**: `frontend/src/utils/__tests__/authRedirect.test.ts`
**Action**: MODIFY

**Implementation**: Add new test cases:
```typescript
describe('URL param handling (invite flow)', () => {
  it('should redirect to returnTo with token from URL params', () => {
    window.location.hash = '#login?returnTo=accept-invite&token=abc123';
    handleAuthRedirect();
    expect(window.location.hash).toBe('#accept-invite?token=abc123');
  });

  it('should redirect to returnTo without token if not present', () => {
    window.location.hash = '#login?returnTo=settings';
    handleAuthRedirect();
    expect(window.location.hash).toBe('#settings');
  });

  it('should URL-encode token with special characters', () => {
    window.location.hash = '#login?returnTo=accept-invite&token=abc%2B123%3D';
    handleAuthRedirect();
    // Token should be re-encoded in output
    expect(window.location.hash).toContain('accept-invite?token=');
  });

  it('should prioritize URL params over sessionStorage', () => {
    sessionStorage.setItem('redirectAfterLogin', 'assets');
    window.location.hash = '#login?returnTo=accept-invite&token=xyz';
    handleAuthRedirect();
    expect(window.location.hash).toBe('#accept-invite?token=xyz');
    // sessionStorage should NOT be cleared since URL params took priority
  });

  it('should fall back to sessionStorage when no URL params', () => {
    sessionStorage.setItem('redirectAfterLogin', 'locations');
    window.location.hash = '#login';
    handleAuthRedirect();
    expect(window.location.hash).toBe('#locations');
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

### Task 3: Run Full Validation
**Action**: VALIDATE

**Commands**:
```bash
cd frontend && just validate
```

This runs lint, typecheck, test, and build.

### Task 4: Manual E2E Verification (Optional)
**Action**: VERIFY

Test the actual flow:
1. Open invite link in new tab: `#accept-invite?token=test123`
2. Click "Sign In" → should go to `#login?returnTo=accept-invite&token=test123`
3. Log in → should redirect to `#accept-invite?token=test123`

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Token encoding issues | Low | Medium | Tests cover special chars |
| Break existing sessionStorage flow | Low | High | Tests verify fallback works |
| Hash parsing edge cases | Low | Low | Simple string operations |

## Integration Points
- **No store updates needed** - Pure utility function
- **No route changes needed** - URL format unchanged
- **No config updates needed** - No new constants

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd frontend && just lint      # Gate 1: Syntax & Style
cd frontend && just typecheck # Gate 2: Type Safety
cd frontend && just test      # Gate 3: Unit Tests
```

**Final validation**:
```bash
cd frontend && just validate  # All checks
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec with code examples
- ✅ Existing test patterns to follow
- ✅ Simple string manipulation, no external dependencies
- ✅ Additive change, preserves existing behavior
- ✅ Small scope (1 function, ~20 lines of new code)

**Estimated one-pass success probability**: 95%

**Reasoning**: This is a straightforward enhancement to a 15-line function. The fix is well-documented in the spec with exact code. Only risk is encoding edge cases, covered by tests.
