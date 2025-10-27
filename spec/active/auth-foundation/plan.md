# Implementation Plan: Auth Foundation - Initialization & 401 Handling
Generated: 2025-10-27
Specification: spec.md
Linear Issue: TRA-96

## Understanding

This plan implements **Phase 1 of 3** for the hybrid authentication system. We're adding:
1. **JWT-based token validation** on app startup (decode + expiration check)
2. **App-level auth initialization** to restore sessions across page reloads
3. **Enhanced 401 interceptor** to clear Zustand state (not just localStorage)
4. **Comprehensive unit tests** for all new validation logic

**Key insight**: The 401 interceptor ALREADY EXISTS but doesn't call `authStore.logout()` to clear Zustand state. The `initialize()` method also EXISTS but only checks if token exists, not if it's valid/expired.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/stores/authStore.ts` (lines 130-137) - Current initialize() implementation (too basic)
- `frontend/src/lib/api/client.ts` (lines 32-47) - Existing 401 interceptor (needs enhancement)
- `frontend/src/stores/authStore.test.ts` (lines 167-197) - Existing initialize() tests (need expansion)
- `frontend/src/stores/deviceStore.test.ts` - Vitest test pattern reference
- `backend/internal/util/jwt/jwt.go` (lines 12-32) - Backend JWT structure (user_id, email, exp, iat)

**Files to Create**:
- None - all files exist

**Files to Modify**:
1. `frontend/package.json` - Add jwt-decode dependency
2. `frontend/src/stores/authStore.ts` (lines 130-137) - Enhance initialize() with JWT validation
3. `frontend/src/App.tsx` (add after line 32) - Call authStore.initialize() on mount
4. `frontend/src/lib/api/client.ts` (lines 32-47) - Import authStore, call logout() in 401 handler
5. `frontend/src/stores/authStore.test.ts` (lines 167-197) - Add tests for JWT validation

## Architecture Impact
- **Subsystems affected**: State Management (authStore), API/Network (axios interceptor), App Lifecycle (App.tsx)
- **New dependencies**: `jwt-decode` (^4.0.0) - Standard JWT decoder library
- **Breaking changes**: None - purely additive enhancements

## Task Breakdown

### Task 1: Add jwt-decode Dependency
**File**: `frontend/package.json`
**Action**: MODIFY
**Pattern**: Standard pnpm add

**Implementation**:
```bash
cd frontend
pnpm add jwt-decode
```

**Validation**:
```bash
just frontend lint       # Should pass (no code changes yet)
just frontend typecheck  # Should pass
```

---

### Task 2: Enhance authStore.initialize() with JWT Validation
**File**: `frontend/src/stores/authStore.ts`
**Action**: MODIFY (lines 1-4 for imports, lines 130-137 for initialize)
**Pattern**: Reference existing login/signup error handling (lines 48-70)

**Implementation**:
```typescript
// ADD IMPORT at top (after line 4)
import { jwtDecode } from 'jwt-decode';

// REPLACE initialize method (lines 130-137)
initialize: () => {
  const state = get();

  // No token in persisted state → logged out
  if (!state.token) {
    set({ isAuthenticated: false, user: null });
    return;
  }

  try {
    // Decode JWT to check expiration
    const decoded = jwtDecode<{ exp: number }>(state.token);

    // Check if token is expired
    const now = Math.floor(Date.now() / 1000); // Current time in seconds
    if (decoded.exp && decoded.exp < now) {
      // Token expired - clear everything
      console.warn('AuthStore: Token expired, clearing auth state');
      set({
        token: null,
        user: null,
        isAuthenticated: false
      });
      return;
    }

    // Token is valid and not expired → restore auth state
    set({ isAuthenticated: true });

  } catch (error) {
    // JWT decode failed (malformed/tampered token) → clear everything
    console.error('AuthStore: Failed to decode JWT, clearing auth state:', error);
    set({
      token: null,
      user: null,
      isAuthenticated: false
    });
  }
},
```

**Why this approach**:
- Uses client-side JWT decode (fast, no network call)
- Defensive: Catches all decode errors and treats as invalid
- Clears ALL auth state on failure (token, user, isAuthenticated)
- Relies on 401 interceptor for server-side validation
- Console logs for debugging (safe - no token logging)

**Validation**:
```bash
just frontend lint       # Must pass
just frontend typecheck  # Must pass (jwt-decode types are built-in)
just frontend test       # Will FAIL until Task 5 (tests need updating)
```

---

### Task 3: Call initialize() in App.tsx
**File**: `frontend/src/App.tsx`
**Action**: MODIFY (add after line 32)
**Pattern**: Reference existing useEffect hooks (lines 30-32, 78-80)

**Implementation**:
```typescript
// ADD IMPORT at top (after line 2)
import { useAuthStore } from '@/stores/authStore';

// ADD NEW useEffect after line 32 (after initOpenReplay useEffect)
useEffect(() => {
  // Initialize auth state from persisted storage
  useAuthStore.getState().initialize();
}, []);
```

**Why this approach**:
- Runs once on mount (empty dependency array)
- Non-blocking (initialize is synchronous)
- Placed early in App lifecycle (after OpenReplay init)
- Uses getState() pattern consistent with other stores

**Validation**:
```bash
just frontend lint       # Must pass
just frontend typecheck  # Must pass
just frontend test       # Run to ensure no regressions
```

---

### Task 4: Update 401 Interceptor to Clear Zustand State
**File**: `frontend/src/lib/api/client.ts`
**Action**: MODIFY (lines 1-2 for import, lines 32-47 for interceptor)
**Pattern**: Existing 401 handler (lines 32-47)

**Implementation**:
```typescript
// ADD IMPORT at top (after line 2)
import { useAuthStore } from '@/stores/authStore';

// MODIFY response interceptor (lines 32-47)
// Response interceptor: Handle 401 (expired/invalid token)
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Clear Zustand auth state (important!)
      useAuthStore.getState().logout();

      // Note: logout() will clear persisted localStorage via Zustand middleware
      // No need to manually call localStorage.removeItem('auth-storage')

      // Show user notification
      toast.error('Session expired. Please log in again.');

      // Redirect to login
      window.location.hash = '#login';
    }
    return Promise.reject(error);
  }
);
```

**Why this approach**:
- Calls authStore.logout() which handles ALL cleanup (Zustand + localStorage)
- Removes manual localStorage.removeItem (redundant with logout)
- Maintains existing toast notification behavior
- Maintains existing hash redirect behavior

**Validation**:
```bash
just frontend lint       # Must pass
just frontend typecheck  # Must pass
just frontend test       # Run client.test.ts if it exists
```

---

### Task 5: Add/Update Unit Tests
**File**: `frontend/src/stores/authStore.test.ts`
**Action**: MODIFY (lines 167-197, expand test coverage)
**Pattern**: Existing test structure (lines 25-90)

**Implementation**:
```typescript
// ADD IMPORT at top (after line 3)
import { jwtDecode } from 'jwt-decode';

// Mock jwt-decode (after line 6)
vi.mock('jwt-decode');

// UPDATE describe('initialize') block (lines 167-197)
describe('initialize', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should set isAuthenticated to true if token is valid and not expired', () => {
    const futureTimestamp = Math.floor(Date.now() / 1000) + 3600; // 1 hour from now

    vi.mocked(jwtDecode).mockReturnValue({
      exp: futureTimestamp,
      user_id: 1,
      email: 'test@example.com',
    });

    useAuthStore.setState({
      token: 'valid-token',
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      },
      isAuthenticated: false,
    });

    useAuthStore.getState().initialize();

    expect(useAuthStore.getState().isAuthenticated).toBe(true);
    expect(useAuthStore.getState().token).toBe('valid-token');
    expect(jwtDecode).toHaveBeenCalledWith('valid-token');
  });

  it('should clear auth state if token is expired', () => {
    const pastTimestamp = Math.floor(Date.now() / 1000) - 3600; // 1 hour ago

    vi.mocked(jwtDecode).mockReturnValue({
      exp: pastTimestamp,
      user_id: 1,
      email: 'test@example.com',
    });

    useAuthStore.setState({
      token: 'expired-token',
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      },
      isAuthenticated: true,
    });

    useAuthStore.getState().initialize();

    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(useAuthStore.getState().token).toBeNull();
    expect(useAuthStore.getState().user).toBeNull();
  });

  it('should clear auth state if JWT decode fails (malformed token)', () => {
    vi.mocked(jwtDecode).mockImplementation(() => {
      throw new Error('Invalid token format');
    });

    useAuthStore.setState({
      token: 'malformed-token',
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      },
      isAuthenticated: true,
    });

    useAuthStore.getState().initialize();

    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(useAuthStore.getState().token).toBeNull();
    expect(useAuthStore.getState().user).toBeNull();
  });

  it('should set isAuthenticated to false if no token exists', () => {
    useAuthStore.setState({
      token: null,
      user: null,
      isAuthenticated: true,
    });

    useAuthStore.getState().initialize();

    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(jwtDecode).not.toHaveBeenCalled();
  });

  it('should handle missing exp claim gracefully', () => {
    vi.mocked(jwtDecode).mockReturnValue({
      user_id: 1,
      email: 'test@example.com',
      // No exp claim
    });

    useAuthStore.setState({
      token: 'token-without-exp',
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      },
      isAuthenticated: false,
    });

    useAuthStore.getState().initialize();

    // Without exp claim, token is considered valid
    expect(useAuthStore.getState().isAuthenticated).toBe(true);
  });
});
```

**Test coverage**:
- ✅ Valid token (not expired) → Sets isAuthenticated=true
- ✅ Expired token → Clears all auth state
- ✅ Malformed token (decode error) → Clears all auth state
- ✅ No token → Sets isAuthenticated=false
- ✅ Missing exp claim → Treats as valid (defensive)

**Validation**:
```bash
just frontend test       # All tests must pass
```

---

### Task 6: Manual Testing & Verification
**Action**: Manual testing scenarios
**Pattern**: Use browser DevTools + localStorage manipulation

**Test scenarios**:

**Scenario 1: Valid token persistence**
1. Log in to the app
2. Open DevTools → Application → localStorage
3. Verify 'auth-storage' exists with token
4. Reload page (F5)
5. ✅ Should remain logged in (user menu visible)

**Scenario 2: Expired token cleanup**
1. Log in to the app
2. Open DevTools → Application → localStorage
3. Find 'auth-storage', click to edit
4. Modify the token to have an old exp timestamp
5. Reload page (F5)
6. ✅ Should be logged out, localStorage cleared

**Scenario 3: Malformed token cleanup**
1. Log in to the app
2. Open DevTools → Application → localStorage
3. Find 'auth-storage', modify token to invalid string (e.g., "invalid-jwt")
4. Reload page (F5)
5. ✅ Should be logged out, localStorage cleared, no errors in console

**Scenario 4: 401 response handling**
1. Log in to the app
2. Open DevTools → Network tab
3. Use backend to invalidate token OR wait for expiration
4. Make an API call (e.g., navigate to protected route)
5. ✅ Should see toast "Session expired"
6. ✅ Should redirect to #login
7. ✅ localStorage should be cleared

**Validation**:
- All 4 scenarios pass
- No console errors (warnings are OK)

---

## Risk Assessment

### Risk: jwt-decode version mismatch
**Mitigation**: Use latest stable version (^4.0.0), check TypeScript types work

### Risk: Token validation too strict
**Mitigation**: Defensive error handling - any decode error = logout (safe default)

### Risk: Race condition on app startup
**Mitigation**: initialize() is synchronous, runs early in App lifecycle

### Risk: 401 interceptor triggers multiple times
**Mitigation**: Existing interceptor already handles this (Promise.reject propagates once)

### Risk: Breaking existing auth flow
**Mitigation**: All changes are additive, existing login/signup/logout untouched

## Integration Points

### State Management
- **authStore.initialize()**: Enhanced with JWT validation
- **authStore.logout()**: Called by 401 interceptor (new integration)

### API Layer
- **apiClient 401 interceptor**: Now clears Zustand state via logout()

### App Lifecycle
- **App.tsx useEffect**: Calls initialize() on mount

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

### After EVERY code change:
```bash
just frontend lint       # Gate 1: Syntax & Style
just frontend typecheck  # Gate 2: Type Safety
just frontend test       # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**After Task 1 (Add dependency)**:
- `just frontend lint` - Should pass
- `just frontend typecheck` - Should pass

**After Task 2 (Enhance initialize)**:
- `just frontend lint` - Must pass
- `just frontend typecheck` - Must pass
- `just frontend test` - Will fail (tests not updated yet)

**After Task 3 (App.tsx integration)**:
- `just frontend lint` - Must pass
- `just frontend typecheck` - Must pass

**After Task 4 (401 interceptor)**:
- `just frontend lint` - Must pass
- `just frontend typecheck` - Must pass

**After Task 5 (Unit tests)**:
- `just frontend test` - ALL tests must pass

**Final validation**:
```bash
just frontend validate   # Runs lint + typecheck + test
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar patterns found in codebase (authStore.test.ts, existing initialize)
- ✅ All clarifying questions answered (JWT decode approach, axios usage, toast notifications)
- ✅ Existing test patterns to follow (Vitest + RTL)
- ✅ Backend JWT structure documented (exp claim, standard format)
- ✅ 401 interceptor already exists (just needs enhancement)
- ✅ No new architectural patterns (enhancing existing code)

**Assessment**: High confidence implementation. All patterns exist, just enhancing existing auth flow with JWT validation. Low risk of issues.

**Estimated one-pass success probability**: 85%

**Reasoning**:
- All code patterns exist and are well-tested
- Only adding jwt-decode library (mature, widely used)
- Enhancing existing methods, not creating new ones
- Comprehensive test coverage planned
- Manual testing scenarios defined
- Small risk: jwt-decode types might need adjustment (15% uncertainty)

## Notes

**Important implementation details**:
1. Use `jwtDecode<{ exp: number }>(token)` with TypeScript generic for type safety
2. Use `Math.floor(Date.now() / 1000)` for current Unix timestamp (matches JWT exp format)
3. Console.warn for expired tokens, console.error for decode failures (helps debugging)
4. Don't log actual token values (security)
5. Hash-based routing: Use `window.location.hash = '#login'` not `/login`
6. Zustand persist middleware handles localStorage automatically
7. JWT decode is client-side only - 401 interceptor validates server-side

**Phase 2 dependencies** (out of scope):
- Phase 2 (UI Integration) will add Header component that uses isAuthenticated
- Phase 2 will need this foundation to show user menu vs login button

**Testing strategy**:
- Unit tests validate logic in isolation
- Manual tests validate browser integration
- E2E tests in Phase 3 will validate full flow
