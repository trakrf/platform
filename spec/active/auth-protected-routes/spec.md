# Feature: Protected Routes & Redirect Flow

## Origin
**Linear Issue**: [TRA-98 - Protected Routes & Redirect Flow](https://linear.app/trakrf/issue/TRA-98/protected-routes-and-redirect-flow)

**Parent Issue**: TRA-91 (Phase 3 of 3 - Final)

Completes the hybrid authentication system. This phase protects Assets and Locations screens while implementing seamless redirect-after-login flow.

## Outcome
- Assets and Locations screens require authentication
- Public features (Inventory, Locate, Barcode) remain accessible without login
- Seamless redirect flow: Click protected route → Login → Return to original destination
- Token persistence across page reloads (validated by E2E tests)

## User Story
**As an unauthenticated user clicking a protected route**
I want to be redirected to login and then returned to my intended destination
So that I don't lose context or have to navigate back manually

**As an authenticated user**
I want to access protected features without interruption
So that I can manage assets and locations efficiently

**As a user accessing public features**
I want to use Inventory, Locate, and Barcode screens without any login requirement
So that I can quickly scan barcodes and check inventory

## Context

### Current State
- ProtectedRoute component exists (from Part 2)
- Assets and Locations screens are accessible without authentication
- No redirect-after-login logic
- Public screens are accessible (already working)

### Desired State
- Assets and Locations wrapped with ProtectedRoute
- ProtectedRoute saves requested path to sessionStorage before redirect
- LoginScreen and SignupScreen read sessionStorage and redirect after auth
- Public screens remain unchanged (no authentication barriers)
- Full E2E test coverage validates all flows

## Technical Requirements

### 1. Wrap Protected Screens
**Files**:
- `frontend/src/screens/AssetsScreen.tsx`
- `frontend/src/screens/LocationsScreen.tsx`

**Wrap both screens with ProtectedRoute**:
```tsx
import { ProtectedRoute } from '@/components/ProtectedRoute';

export function AssetsScreen() {
  return (
    <ProtectedRoute>
      <div className="assets-screen">
        {/* Existing content */}
      </div>
    </ProtectedRoute>
  );
}
```

**Implementation notes**:
- Minimal changes to existing screen components
- ProtectedRoute handles all auth logic
- Children render only when authenticated

### 2. Update ProtectedRoute to Save Redirect Path
**File**: `frontend/src/components/ProtectedRoute.tsx`

**Add sessionStorage logic**:
```tsx
export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuthStore();
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => {
    if (!isAuthenticated) {
      // Save current path before redirecting
      sessionStorage.setItem('redirectAfterLogin', location.pathname);
      navigate('/login');
    }
  }, [isAuthenticated, location, navigate]);

  if (!isAuthenticated) {
    return null; // Or loading spinner
  }

  return <>{children}</>;
}
```

**Implementation notes**:
- Save location.pathname to sessionStorage key 'redirectAfterLogin'
- Only save when redirecting (not on every render)
- Use sessionStorage (not localStorage) for security

### 3. Add Redirect-After-Login Logic
**Files**:
- `frontend/src/screens/LoginScreen.tsx`
- `frontend/src/screens/SignupScreen.tsx`

**After successful authentication, read and redirect**:
```tsx
// In LoginScreen after successful login
const handleLoginSuccess = () => {
  const redirectTo = sessionStorage.getItem('redirectAfterLogin');
  sessionStorage.removeItem('redirectAfterLogin');
  navigate(redirectTo || '/');
};

// In SignupScreen after successful signup
const handleSignupSuccess = () => {
  const redirectTo = sessionStorage.getItem('redirectAfterLogin');
  sessionStorage.removeItem('redirectAfterLogin');
  navigate(redirectTo || '/');
};
```

**Implementation notes**:
- Check sessionStorage for 'redirectAfterLogin'
- If exists: Navigate to saved path
- If not exists: Navigate to Home ('/')
- Always clear sessionStorage after reading
- Apply to both LoginScreen and SignupScreen

### 4. E2E Tests (Playwright)
**File**: Create `frontend/tests/e2e/auth-flow.spec.ts`

**Test scenarios**:

**Test 1: Public features accessible without login**
```typescript
test('public features work without login', async ({ page }) => {
  await page.goto('/');

  // Navigate to public tabs
  await page.click('text=Inventory');
  await expect(page).toHaveURL(/\/inventory/);

  await page.click('text=Locate');
  await expect(page).toHaveURL(/\/locate/);

  await page.click('text=Barcode');
  await expect(page).toHaveURL(/\/barcode/);
});
```

**Test 2: Protected features require login with redirect**
```typescript
test('protected features require login with redirect', async ({ page }) => {
  await page.goto('/');

  // Click Assets tab (protected)
  await page.click('text=Assets');

  // Should redirect to login
  await expect(page).toHaveURL(/\/login/);

  // Sign up
  await page.fill('input[name="email"]', 'test@example.com');
  await page.fill('input[name="password"]', 'password123');
  await page.click('button:has-text("Sign Up")');

  // Should redirect back to /assets
  await expect(page).toHaveURL(/\/assets/);
  await expect(page).toHaveText(/Assets/);
});
```

**Test 3: Logout flow**
```typescript
test('logout flow works correctly', async ({ page }) => {
  // Log in first
  await page.goto('/login');
  await page.fill('input[name="email"]', 'test@example.com');
  await page.fill('input[name="password"]', 'password123');
  await page.click('button:has-text("Log In")');

  // Click user menu
  await page.click('.user-menu-trigger');

  // Click logout
  await page.click('text=Logout');

  // Should redirect to Home
  await expect(page).toHaveURL('/');

  // Try to access Assets again
  await page.click('text=Assets');

  // Should redirect to login
  await expect(page).toHaveURL(/\/login/);
});
```

**Test 4: Token persistence across reload**
```typescript
test('token persists across reload', async ({ page }) => {
  // Log in
  await page.goto('/login');
  await page.fill('input[name="email"]', 'test@example.com');
  await page.fill('input[name="password"]', 'password123');
  await page.click('button:has-text("Log In")');

  // Navigate to Assets
  await page.click('text=Assets');
  await expect(page).toHaveURL(/\/assets/);

  // Reload page
  await page.reload();

  // Should still be logged in and on Assets
  await expect(page).toHaveURL(/\/assets/);
  await expect(page.locator('.user-menu')).toBeVisible();
});
```

**Test 5: Direct URL navigation to protected route**
```typescript
test('direct URL to protected route redirects and returns', async ({ page }) => {
  // Go directly to /assets
  await page.goto('/assets');

  // Should redirect to login
  await expect(page).toHaveURL(/\/login/);

  // Log in
  await page.fill('input[name="email"]', 'test@example.com');
  await page.fill('input[name="password"]', 'password123');
  await page.click('button:has-text("Log In")');

  // Should redirect back to /assets
  await expect(page).toHaveURL(/\/assets/);
});
```

**Test 6: Default redirect when no path saved**
```typescript
test('default redirect to Home when no path saved', async ({ page }) => {
  // Go directly to /login
  await page.goto('/login');

  // Log in
  await page.fill('input[name="email"]', 'test@example.com');
  await page.fill('input[name="password"]', 'password123');
  await page.click('button:has-text("Log In")');

  // Should redirect to Home (/)
  await expect(page).toHaveURL('/');
});
```

## Validation Criteria

### Unit Tests
- [ ] ProtectedRoute saves path to sessionStorage when redirecting
- [ ] LoginScreen reads sessionStorage and redirects correctly
- [ ] SignupScreen reads sessionStorage and redirects correctly
- [ ] sessionStorage is cleared after redirect

### E2E Tests (Playwright)
- [ ] Test 1: Public features accessible without login
- [ ] Test 2: Protected features require login with redirect
- [ ] Test 3: Logout flow works correctly
- [ ] Test 4: Token persistence across reload
- [ ] Test 5: Direct URL navigation to protected route
- [ ] Test 6: Default redirect when no path saved

### Manual Testing
- [ ] Navigate to /assets while logged out → Redirects to login → After login returns to /assets
- [ ] Navigate to /locations while logged out → Redirects to login → After signup returns to /locations
- [ ] Open /login directly → After login goes to Home (/)
- [ ] Click Inventory/Locate/Barcode while logged out → Works without login
- [ ] Reload page on /assets while logged in → Still on /assets, still logged in
- [ ] Browser back button through auth flow → Works correctly (no broken states)

## Technical Constraints

### Dependencies
- Phase 1 complete (auth initialization, 401 handling)
- Phase 2 complete (Header with user menu)
- ProtectedRoute component exists
- Assets and Locations screens exist
- Login and Signup screens exist

### Browser Support
- sessionStorage must be available
- React Router for navigation

### Security
- sessionStorage is cleared after redirect (no leftover data)
- sessionStorage is session-scoped (not persistent across tabs/windows)

## Implementation Notes

### sessionStorage vs localStorage
- Use sessionStorage (not localStorage) for redirect path
- Reason: Security - redirect path should not persist across sessions
- sessionStorage clears when tab/window closes

### Edge Cases
- User opens /assets in new tab → Should redirect to login
- User has multiple tabs → Each tab has independent sessionStorage
- User manually clears sessionStorage → Defaults to Home (/)

### Testing Strategy
- E2E tests are critical for this phase (tests integration of all 3 phases)
- Use Playwright for browser-based testing
- Mock API responses for consistent test results

## Success Criteria

✅ **Functionality**:
- Public features (Inventory, Locate, Barcode) accessible without login
- Protected features (Assets, Locations) require authentication
- Redirect-after-login flow works correctly
- Default redirect to Home when no path saved
- Token persists across page reloads

✅ **User Experience**:
- Seamless transitions (no jarring redirects)
- Context preservation (users return to intended page)
- No broken states or dead ends
- Browser back button works correctly

✅ **Testing**:
- All E2E tests pass
- All unit tests pass
- Manual testing scenarios verified

✅ **Code Quality**:
- Properly typed (TypeScript)
- Follows existing patterns
- Clear separation of concerns

## Completion

This phase completes the hybrid authentication system (TRA-91) and enables:
- Cofounder to build Assets/Locations CRUD on protected pages
- Future entity CRUD screens to follow same protected pattern
- Authenticated context for Inventory/Locate (future work)

## Definition of Done
- [ ] AssetsScreen wrapped with ProtectedRoute
- [ ] LocationsScreen wrapped with ProtectedRoute
- [ ] ProtectedRoute saves redirect path to sessionStorage
- [ ] LoginScreen implements redirect-after-login
- [ ] SignupScreen implements redirect-after-login
- [ ] All E2E tests pass
- [ ] All unit tests pass
- [ ] Manual testing completed
- [ ] Browser back button tested
- [ ] Code reviewed
- [ ] TRA-91 closed as complete
