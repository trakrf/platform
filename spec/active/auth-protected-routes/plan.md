# Implementation Plan: Protected Routes & Redirect Flow
Generated: 2025-10-28
Specification: spec.md

## Understanding

This feature completes the hybrid authentication system (Phase 3 of TRA-91) by:
1. Protecting Assets and Locations screens with authentication requirement
2. Implementing seamless redirect-after-login flow using sessionStorage
3. Preserving public access to Inventory, Locate, and Barcode screens
4. Ensuring comprehensive test coverage (unit + E2E)

**Key Discovery**: Much of the infrastructure already exists:
- ProtectedRoute component already saves hash to sessionStorage (line 14-19)
- LoginScreen already implements redirect logic (line 52-59)
- SignupScreen needs update to check sessionStorage before redirecting
- Assets and Locations screens exist but aren't wrapped with ProtectedRoute

**Architecture**: Hash-based routing (`#assets`, not `/assets`), matching existing pattern throughout the app.

## Relevant Files

### Reference Patterns (existing code to follow)

- `frontend/src/components/ProtectedRoute.tsx` (lines 14-19) - sessionStorage save pattern
- `frontend/src/components/LoginScreen.tsx` (lines 52-59) - redirect-after-login pattern
- `frontend/tests/e2e/auth.spec.ts` - E2E test structure and assertions
- `frontend/src/components/__tests__/LoginScreen.test.tsx` - Unit test pattern for auth screens

### Files to Create

- `frontend/src/utils/authRedirect.ts` - Shared redirect helper function
- `frontend/tests/e2e/protected-routes.spec.ts` - E2E tests for protected route flows

### Files to Modify

- `frontend/src/components/LoginScreen.tsx` - Refactor to use shared redirect helper
- `frontend/src/components/SignupScreen.tsx` - Add sessionStorage check using shared helper
- `frontend/src/components/AssetsScreen.tsx` - Wrap with ProtectedRoute
- `frontend/src/components/LocationsScreen.tsx` - Wrap with ProtectedRoute
- `frontend/src/components/ProtectedRoute.test.tsx` - Add sessionStorage unit tests
- `frontend/src/components/__tests__/LoginScreen.test.tsx` - Add redirect unit tests
- `frontend/src/components/__tests__/SignupScreen.test.tsx` - Add redirect unit tests

## Architecture Impact

- **Subsystems affected**: Frontend (UI, Routing, Auth)
- **New dependencies**: None (using native sessionStorage)
- **Breaking changes**: None (purely additive - protecting previously public routes)

## Task Breakdown

### Task 1: Create Shared Redirect Helper
**File**: `frontend/src/utils/authRedirect.ts`
**Action**: CREATE
**Pattern**: Reference LoginScreen.tsx lines 52-59

**Implementation**:
```typescript
/**
 * Handles post-authentication redirect logic
 * Checks sessionStorage for saved redirect path, falls back to home
 */
export function handleAuthRedirect(): void {
  const redirect = sessionStorage.getItem('redirectAfterLogin');

  if (redirect) {
    // Clear before redirecting to avoid loops
    sessionStorage.removeItem('redirectAfterLogin');
    window.location.hash = `#${redirect}`;
  } else {
    window.location.hash = '#home';
  }
}
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test` (will add unit tests in Task 2)

---

### Task 2: Add Unit Tests for Redirect Helper
**File**: `frontend/src/utils/__tests__/authRedirect.test.ts`
**Action**: CREATE
**Pattern**: Follow existing utils test patterns

**Implementation**:
```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { handleAuthRedirect } from '../authRedirect';

describe('handleAuthRedirect', () => {
  beforeEach(() => {
    // Clear sessionStorage before each test
    sessionStorage.clear();
    // Reset location.hash
    window.location.hash = '';
  });

  it('should redirect to saved path from sessionStorage', () => {
    sessionStorage.setItem('redirectAfterLogin', 'assets');
    handleAuthRedirect();

    expect(window.location.hash).toBe('#assets');
    expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
  });

  it('should redirect to home if no saved path', () => {
    handleAuthRedirect();
    expect(window.location.hash).toBe('#home');
  });

  it('should clear sessionStorage after redirect', () => {
    sessionStorage.setItem('redirectAfterLogin', 'locations');
    handleAuthRedirect();

    expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
  });
});
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test` (verify new tests pass)

---

### Task 3: Update LoginScreen to Use Shared Helper
**File**: `frontend/src/components/LoginScreen.tsx`
**Action**: MODIFY (lines 52-59)
**Pattern**: Replace inline redirect with helper call

**Changes**:
1. Import helper: `import { handleAuthRedirect } from '@/utils/authRedirect';`
2. Replace lines 52-59 with: `handleAuthRedirect();`

**Before**:
```typescript
// Handle redirect after successful login
const redirect = sessionStorage.getItem('redirectAfterLogin');
if (redirect) {
  window.location.hash = `#${redirect}`;
  sessionStorage.removeItem('redirectAfterLogin');
} else {
  window.location.hash = '#home';
}
```

**After**:
```typescript
handleAuthRedirect();
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test`

---

### Task 4: Update SignupScreen to Use Shared Helper
**File**: `frontend/src/components/SignupScreen.tsx`
**Action**: MODIFY (line 58)
**Pattern**: Replace hardcoded redirect with helper call

**Changes**:
1. Import helper: `import { handleAuthRedirect } from '@/utils/authRedirect';`
2. Replace line 58 `window.location.hash = '#home';` with: `handleAuthRedirect();`

**Before**:
```typescript
// After successful signup, redirect to home
window.location.hash = '#home';
```

**After**:
```typescript
handleAuthRedirect();
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test`

---

### Task 5: Wrap AssetsScreen with ProtectedRoute
**File**: `frontend/src/components/AssetsScreen.tsx`
**Action**: MODIFY (entire component)
**Pattern**: Follow spec example (spec.md lines 52-64)

**Changes**:
1. Import ProtectedRoute: `import { ProtectedRoute } from '@/components/ProtectedRoute';`
2. Wrap return statement with `<ProtectedRoute>{...existing content...}</ProtectedRoute>`

**Before**:
```typescript
export default function AssetsScreen() {
  const { setActiveTab } = useUIStore();
  // ...
  return (
    <div className="max-w-4xl mx-auto">
      {/* existing content */}
    </div>
  );
}
```

**After**:
```typescript
import { ProtectedRoute } from '@/components/ProtectedRoute';

export default function AssetsScreen() {
  const { setActiveTab } = useUIStore();
  // ...
  return (
    <ProtectedRoute>
      <div className="max-w-4xl mx-auto">
        {/* existing content */}
      </div>
    </ProtectedRoute>
  );
}
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test`
- Manual: Navigate to `#assets` while logged out → Should redirect to `#login`

---

### Task 6: Wrap LocationsScreen with ProtectedRoute
**File**: `frontend/src/components/LocationsScreen.tsx`
**Action**: MODIFY (entire component)
**Pattern**: Follow AssetsScreen pattern (Task 5)

**Changes**:
1. Import ProtectedRoute: `import { ProtectedRoute } from '@/components/ProtectedRoute';`
2. Wrap return statement with `<ProtectedRoute>{...existing content...}</ProtectedRoute>`

**Implementation**: Same pattern as Task 5, applied to LocationsScreen

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test`
- Manual: Navigate to `#locations` while logged out → Should redirect to `#login`

---

### Task 7: Add Unit Tests for ProtectedRoute sessionStorage
**File**: `frontend/src/components/ProtectedRoute.test.tsx`
**Action**: MODIFY (add new test cases)
**Pattern**: Follow existing test structure in file

**Add tests**:
```typescript
describe('sessionStorage redirect path', () => {
  it('should save current hash to sessionStorage when not authenticated', () => {
    window.location.hash = '#assets';
    const { useAuthStore } = require('@/stores');
    useAuthStore.setState({ isAuthenticated: false });

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(sessionStorage.getItem('redirectAfterLogin')).toBe('assets');
  });

  it('should not save login/signup paths to sessionStorage', () => {
    window.location.hash = '#login';
    const { useAuthStore } = require('@/stores');
    useAuthStore.setState({ isAuthenticated: false });

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
  });

  it('should redirect to #login when not authenticated', () => {
    window.location.hash = '#assets';
    const { useAuthStore } = require('@/stores');
    useAuthStore.setState({ isAuthenticated: false });

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(window.location.hash).toBe('#login');
  });
});
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test` (verify new tests pass)

---

### Task 8: Add Unit Tests for LoginScreen Redirect
**File**: `frontend/src/components/__tests__/LoginScreen.test.tsx`
**Action**: MODIFY (add new test cases)
**Pattern**: Follow existing test structure

**Add tests**:
```typescript
describe('redirect after login', () => {
  it('should redirect to saved path from sessionStorage', async () => {
    sessionStorage.setItem('redirectAfterLogin', 'assets');

    const { useAuthStore } = await import('@/stores');
    vi.mocked(useAuthStore).mockReturnValue({
      login: vi.fn().mockResolvedValue(undefined),
      isLoading: false,
    });

    render(<LoginScreen />);

    // Fill and submit form
    await userEvent.type(screen.getByLabelText(/email/i), 'test@example.com');
    await userEvent.type(screen.getByLabelText(/password/i), 'password123');
    await userEvent.click(screen.getByRole('button', { name: /log in/i }));

    await waitFor(() => {
      expect(window.location.hash).toBe('#assets');
      expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
    });
  });

  it('should redirect to home if no saved path', async () => {
    const { useAuthStore } = await import('@/stores');
    vi.mocked(useAuthStore).mockReturnValue({
      login: vi.fn().mockResolvedValue(undefined),
      isLoading: false,
    });

    render(<LoginScreen />);

    // Fill and submit form
    await userEvent.type(screen.getByLabelText(/email/i), 'test@example.com');
    await userEvent.type(screen.getByLabelText(/password/i), 'password123');
    await userEvent.click(screen.getByRole('button', { name: /log in/i }));

    await waitFor(() => {
      expect(window.location.hash).toBe('#home');
    });
  });
});
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test` (verify new tests pass)

---

### Task 9: Add Unit Tests for SignupScreen Redirect
**File**: `frontend/src/components/__tests__/SignupScreen.test.tsx`
**Action**: MODIFY (add new test cases)
**Pattern**: Follow LoginScreen test pattern (Task 8)

**Add tests**: Same structure as Task 8, but for SignupScreen component

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- Test: `cd frontend && just test` (verify new tests pass)

---

### Task 10: Create E2E Tests for Protected Routes
**File**: `frontend/tests/e2e/protected-routes.spec.ts`
**Action**: CREATE
**Pattern**: Reference auth.spec.ts structure and assertions

**Implementation**:
```typescript
import { test, expect } from '@playwright/test';

/**
 * Protected Routes & Redirect Flow E2E Tests
 *
 * Tests the complete authentication flow with protected routes.
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 *
 * Run with: pnpm test:e2e tests/e2e/protected-routes.spec.ts
 */

test.describe('Protected Routes & Redirect Flow', () => {
  test.beforeEach(async ({ page }) => {
    // Clear auth state
    await page.goto('/');
    await page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });
    await page.reload({ waitUntil: 'networkidle' });
  });

  test('public features accessible without login', async ({ page }) => {
    await page.goto('/');

    // Navigate to public tabs - should work without login
    await page.goto('/#inventory');
    await expect(page).toHaveURL(/#inventory/);

    await page.goto('/#locate');
    await expect(page).toHaveURL(/#locate/);

    await page.goto('/#barcode');
    await expect(page).toHaveURL(/#barcode/);
  });

  test('protected features require login with redirect to assets', async ({ page }) => {
    await page.goto('/');

    // Navigate to protected route (assets)
    await page.goto('/#assets');

    // Should redirect to login
    await expect(page).toHaveURL(/#login/);

    // Sign up with new account
    await page.locator('input#email').fill(`test-${Date.now()}@example.com`);
    await page.locator('input#password').fill('password123');
    await page.locator('button[type="submit"]').click();

    // Should redirect back to /assets
    await expect(page).toHaveURL(/#assets/, { timeout: 10000 });
    await expect(page.locator('text=Assets Management')).toBeVisible();
  });

  test('protected features require login with redirect to locations', async ({ page }) => {
    await page.goto('/');

    // Navigate to protected route (locations)
    await page.goto('/#locations');

    // Should redirect to login
    await expect(page).toHaveURL(/#login/);

    // Sign up with new account
    await page.locator('input#email').fill(`test-${Date.now()}@example.com`);
    await page.locator('input#password').fill('password123');
    await page.locator('button[type="submit"]').click();

    // Should redirect back to /locations
    await expect(page).toHaveURL(/#locations/, { timeout: 10000 });
    await expect(page.locator('text=Locations Management')).toBeVisible();
  });

  test('logout flow works correctly', async ({ page }) => {
    // Sign up first
    await page.goto('/#signup');
    await page.locator('input#email').fill(`test-${Date.now()}@example.com`);
    await page.locator('input#password').fill('password123');
    await page.locator('button[type="submit"]').click();

    // Wait for redirect to home
    await expect(page).toHaveURL(/#home/, { timeout: 10000 });

    // Navigate to protected route
    await page.goto('/#assets');
    await expect(page).toHaveURL(/#assets/);

    // Click user menu and logout
    await page.click('[data-testid="user-menu-trigger"]');
    await page.click('text=Logout');

    // Should redirect to home
    await expect(page).toHaveURL(/#home/);

    // Try to access protected route again
    await page.goto('/#assets');

    // Should redirect to login
    await expect(page).toHaveURL(/#login/);
  });

  test('token persists across reload', async ({ page }) => {
    // Sign up
    await page.goto('/#signup');
    await page.locator('input#email').fill(`test-${Date.now()}@example.com`);
    await page.locator('input#password').fill('password123');
    await page.locator('button[type="submit"]').click();

    // Navigate to assets
    await page.goto('/#assets');
    await expect(page).toHaveURL(/#assets/);

    // Reload page
    await page.reload();

    // Should still be logged in and on assets page
    await expect(page).toHaveURL(/#assets/);
    await expect(page.locator('[data-testid="user-menu-trigger"]')).toBeVisible();
  });

  test('direct URL to protected route redirects and returns', async ({ page }) => {
    // Go directly to /assets
    await page.goto('/#assets');

    // Should redirect to login
    await expect(page).toHaveURL(/#login/);

    // Sign up
    await page.locator('input#email').fill(`test-${Date.now()}@example.com`);
    await page.locator('input#password').fill('password123');
    await page.locator('button[type="submit"]').click();

    // Should redirect back to /assets
    await expect(page).toHaveURL(/#assets/, { timeout: 10000 });
  });

  test('default redirect to home when no path saved', async ({ page }) => {
    // Go directly to /login
    await page.goto('/#login');

    // Sign up (signup link)
    await page.locator('a[href="#signup"]').click();
    await page.locator('input#email').fill(`test-${Date.now()}@example.com`);
    await page.locator('input#password').fill('password123');
    await page.locator('button[type="submit"]').click();

    // Should redirect to home (/)
    await expect(page).toHaveURL(/#home/, { timeout: 10000 });
  });
});
```

**Validation**:
- Lint: `cd frontend && just lint`
- Typecheck: `cd frontend && just typecheck`
- E2E: `cd frontend && pnpm test:e2e tests/e2e/protected-routes.spec.ts`

---

## Risk Assessment

### Risk: sessionStorage not cleared causing redirect loops
**Mitigation**:
- Helper function clears sessionStorage BEFORE redirect (defensive)
- ProtectedRoute checks for login/signup paths (lines 16-18)
- Unit tests verify clearing behavior

### Risk: Hash routing edge cases with special characters
**Mitigation**:
- Following existing pattern (ProtectedRoute already handles this)
- E2E tests cover actual browser navigation
- No URL encoding needed (hash routing handles it)

### Risk: E2E tests flaky due to timing
**Mitigation**:
- Use Playwright's built-in waiting mechanisms (`expect(...).toHaveURL()`)
- Set reasonable timeouts (10s for auth operations)
- Follow existing auth.spec.ts patterns (proven stable)

### Risk: Unit tests for LoginScreen/SignupScreen complex due to store mocking
**Mitigation**:
- Reference existing test patterns in same files
- Mock only authStore.login/signup methods
- Test redirect behavior, not auth logic (separation of concerns)

## Integration Points

- **Store updates**: No changes to authStore (already handles auth state)
- **Route changes**: No changes to App.tsx routing logic (hash-based remains)
- **Config updates**: None required
- **Component composition**: ProtectedRoute wraps AssetsScreen and LocationsScreen

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change:
- **Gate 1: Lint** - `cd frontend && just lint` (must pass)
- **Gate 2: Typecheck** - `cd frontend && just typecheck` (must pass)
- **Gate 3: Unit Tests** - `cd frontend && just test` (must pass)

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task:
```bash
cd frontend
just lint
just typecheck
just test
```

After Task 10 (all tasks complete):
```bash
cd frontend
just validate  # Runs all checks
pnpm test:e2e tests/e2e/protected-routes.spec.ts  # Run new E2E tests
```

Final validation (from project root):
```bash
just validate  # Full stack validation
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)

**Breakdown**:
- File Impact: 2 files to create, 7 files to modify (9 total) = 2pts
- Subsystem Coupling: 1 subsystem (Frontend only) = 0pts
- Task Estimate: 10 subtasks = 2pts
- Dependencies: 0 new packages = 0pts
- Pattern Novelty: Existing patterns (ProtectedRoute, sessionStorage, hash routing) = 0pts

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ ProtectedRoute already implements core logic (lines 14-19)
✅ LoginScreen already has redirect pattern (lines 52-59)
✅ Hash routing pattern established and working
✅ Existing test patterns to follow (auth.spec.ts, unit test files)
✅ No new dependencies or external libraries
✅ All clarifying questions answered
⚠️ E2E tests for auth flows can be timing-sensitive (mitigated with proper waits)

**Assessment**: High confidence implementation. Most infrastructure already exists, primarily refactoring to shared helper and adding comprehensive test coverage. The main work is wrapping two screens and creating E2E tests following established patterns.

**Estimated one-pass success probability**: 85%

**Reasoning**:
- Core logic already implemented and working (ProtectedRoute, LoginScreen redirect)
- Following established patterns throughout (hash routing, test structure)
- Main risks are E2E test timing (mitigated) and unit test mocking (existing examples)
- No architectural decisions or novel patterns required
- Clear task breakdown with validation gates at each step
