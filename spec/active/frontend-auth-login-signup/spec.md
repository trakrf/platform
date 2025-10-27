# Feature: Frontend Auth - Login & Signup Screens

**Linear Issue**: TRA-90
**Part**: 3 of 4 (Frontend Auth Implementation)
**Status**: In Progress
**Dependencies**:
- TRA-89 (Part 2: Auth Foundation - authStore, authApi, ProtectedRoute)
- Backend schema refactor (account → organization terminology)

## Origin

This specification is derived from TRA-90, part of a multi-phase frontend authentication implementation. The auth foundation (store, API client, protected routes) was completed in Part 2. Now we need user-facing screens to enable actual login and signup workflows.

## Outcome

Users will be able to:
- Create new accounts via a signup form
- Log in with existing credentials via a login form
- See clear validation errors before submitting forms
- See backend error messages (duplicate email, wrong password) in a user-friendly way
- Navigate seamlessly between login/signup flows

After implementation, the frontend auth system will have a complete user interface, preparing for Part 4 (integration with user menu and protected route wiring).

## User Stories

### Story 1: New User Signup
**As a** new user
**I want** to create an account with my email, password, and organization name
**So that** I can access the TrakRF platform

**Acceptance Criteria**:
- Signup form validates email format, password length (≥8 chars), organization name length (≥2 chars)
- Form submission calls `authStore.signup()` with correct payload
- Successful signup stores token and redirects to Home
- Duplicate email errors are shown inline
- Submit button shows loading state and is disabled during request

### Story 2: Returning User Login
**As a** returning user
**I want** to log in with my email and password
**So that** I can access my account

**Acceptance Criteria**:
- Login form validates email format and non-empty password
- Form submission calls `authStore.login()` with correct payload
- Successful login stores token and redirects to appropriate screen
- Wrong password/email errors are shown inline
- Submit button shows loading state and is disabled during request

### Story 3: Navigation Between Flows
**As a** user on the login screen
**I want** to navigate to signup if I don't have an account
**So that** I can create one without confusion

**Acceptance Criteria**:
- Login screen has "Don't have an account? Sign up" link → navigates to #signup
- Signup screen has "Already have an account? Log in" link → navigates to #login

## Context

**Discovery**: The auth foundation (authStore, authApi, ProtectedRoute) was built in Part 2 and is ready for integration. Backend endpoints for `/api/v1/auth/login` and `/api/v1/auth/signup` exist and return JWT tokens.

**Current State**:
- No UI for authentication exists
- Users cannot create accounts or log in
- Auth store and API client are functional but unused

**Desired State**:
- Complete login and signup user experience
- Form validation provides immediate feedback
- Error handling is user-friendly
- Design matches existing TrakRF dark theme

**Next Phase**: Part 4 will add user menu and wire protected routes to these authentication screens.

## Technical Requirements

### Component: LoginScreen.tsx

**Location**: `frontend/src/screens/LoginScreen.tsx` (or appropriate screens directory)

**Requirements**:
- Email input field (type="email")
- Password input field (type="password")
- Submit button with loading state
- Inline error message display area
- "Don't have an account? Sign up" link
- Client-side validation:
  - Email format validation
  - Password not empty
- Form submission:
  - Calls `authStore.login({ email, password })`
  - Disables submit during request
  - Shows loading state
  - Handles success: redirect to intended route or default
  - Handles error: display backend error message inline
- Design:
  - Dark theme (bg-gray-900, bg-gray-800)
  - Centered card layout
  - Tailwind classes consistent with Settings screen
  - Inline error messages (red text, clear positioning)

### Component: SignupScreen.tsx

**Location**: `frontend/src/screens/SignupScreen.tsx` (or appropriate screens directory)

**Requirements**:
- Email input field (type="email")
- Password input field (type="password")
- Organization name input field (type="text")
  - **Note**: Organization is the multi-tenant identifier for the application customer
  - Label as "Organization Name" or "Organization"
- Submit button with loading state
- Inline error message display area
- "Already have an account? Log in" link
- Client-side validation:
  - Email format validation
  - Password ≥8 characters
  - Organization name ≥2 characters
- Form submission:
  - Calls `authStore.signup({ email, password, org_name })`
  - Disables submit during request
  - Shows loading state
  - Handles success: redirect to Home
  - Handles error: display backend error message inline (e.g., "Email already exists")
- Design:
  - Dark theme (bg-gray-900, bg-gray-800)
  - Centered card layout
  - Tailwind classes consistent with Settings screen
  - Inline error messages (red text, clear positioning)

### Routing Integration

**File**: `frontend/src/App.tsx` (or router configuration)

**Requirements**:
- Add route: `#login` → LoginScreen
- Add route: `#signup` → SignupScreen
- Routes should be unprotected (public access)

### Design Consistency

**Reference**: Existing Settings screen

**Requirements**:
- Use Tailwind classes consistently
- Dark theme palette:
  - Background: `bg-gray-900`
  - Cards: `bg-gray-800`
  - Text: `text-white`, `text-gray-300`
  - Errors: `text-red-400` or `text-red-500`
- Typography and spacing match Settings screen
- Button styles match existing patterns
- Input field styles match existing forms

### Password Visibility Toggle

**Required Feature**:
- Password visibility toggle (eye icon to show/hide password)
- Toggle between `type="password"` and `type="text"` on click
- Icon positioned inside input field (right side)
- Use consistent icon from existing icon set (eye/eye-slash pattern)
- Applies to both LoginScreen and SignupScreen password fields

## Code Examples

### LoginScreen Structure (Pseudocode)
```tsx
// frontend/src/screens/LoginScreen.tsx
import { useState } from 'react';
import { useAuthStore } from '@/store/authStore';

export function LoginScreen() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const login = useAuthStore(state => state.login);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Client-side validation
    if (!email || !password) {
      setError('All fields required');
      return;
    }

    setLoading(true);
    setError('');

    try {
      await login({ email, password });
      // Redirect on success (implementation detail TBD)
      window.location.hash = '#home';
    } catch (err) {
      setError(err.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center">
      <div className="bg-gray-800 p-8 rounded-lg w-96">
        <h1 className="text-2xl text-white mb-6">Log In</h1>
        <form onSubmit={handleSubmit}>
          {/* Form fields */}
          {error && <p className="text-red-400 text-sm">{error}</p>}
          <button
            type="submit"
            disabled={loading}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 rounded disabled:opacity-50"
          >
            {loading ? 'Logging in...' : 'Log In'}
          </button>
        </form>
        <p className="text-gray-400 text-sm mt-4">
          Don't have an account? <a href="#signup" className="text-blue-400">Sign up</a>
        </p>
      </div>
    </div>
  );
}
```

### SignupScreen Structure (Similar Pattern)
```tsx
// frontend/src/screens/SignupScreen.tsx
// Similar structure to LoginScreen
// Additional field: org_name
// Different validation: password ≥8, org_name ≥2
// Different authStore call: signup({ email, password, org_name })
// Different redirect target: window.location.hash = '#home'
```

## Validation Criteria

### Manual Testing Checklist

**Signup Flow**:
- [ ] Navigate to `#signup`
- [ ] Enter valid data (email, password ≥8 chars, org_name ≥2 chars)
- [ ] Submit → Account created, token stored, redirected to Home
- [ ] Check localStorage/sessionStorage for auth token
- [ ] Test validation errors:
  - [ ] Invalid email format shows error
  - [ ] Password <8 chars shows error
  - [ ] Organization name <2 chars shows error
  - [ ] Empty fields show error
- [ ] Test backend errors:
  - [ ] Duplicate email shows clear error message
- [ ] Test loading state:
  - [ ] Submit button disabled during request
  - [ ] Button text changes to "Signing up..." or similar
- [ ] Test password visibility toggle:
  - [ ] Click eye icon → password visible (type="text")
  - [ ] Click again → password hidden (type="password")
- [ ] Test navigation:
  - [ ] "Already have an account? Log in" link navigates to #login

**Login Flow**:
- [ ] Navigate to `#login`
- [ ] Enter valid credentials (previously created account)
- [ ] Submit → Logged in, token stored, redirected appropriately
- [ ] Check localStorage/sessionStorage for auth token
- [ ] Test validation errors:
  - [ ] Invalid email format shows error
  - [ ] Empty password shows error
- [ ] Test backend errors:
  - [ ] Wrong password shows clear error message
  - [ ] Non-existent email shows clear error message
- [ ] Test loading state:
  - [ ] Submit button disabled during request
  - [ ] Button text changes to "Logging in..." or similar
- [ ] Test password visibility toggle:
  - [ ] Click eye icon → password visible (type="text")
  - [ ] Click again → password hidden (type="password")
- [ ] Test navigation:
  - [ ] "Don't have an account? Sign up" link navigates to #signup

**Design Validation**:
- [ ] Dark theme matches Settings screen
- [ ] Centered card layout looks polished
- [ ] Error messages are clearly visible (red text, good positioning)
- [ ] Spacing and typography match existing patterns
- [ ] Mobile responsive (if app supports mobile)

**Edge Cases**:
- [ ] Network errors show user-friendly message (not raw error)
- [ ] Double-submit prevented by disabled button
- [ ] Forms reset appropriately after error or success
- [ ] User already logged in can still access #login or #signup (allows account switching)
- [ ] Password visibility toggle works correctly on both screens

## Implementation Notes

### Form Validation Timing
- **Client-side validation** happens on submit (before API call)
- **Purpose**: Improve UX by catching obvious errors immediately
- **Scope**: Format checks only (email format, length requirements)
- **Backend validation** is authoritative
- **Purpose**: Security - never trust client

### Error Handling Philosophy
- Display backend errors verbatim if they're user-friendly (e.g., "Email already exists")
- Transform technical errors into friendly messages (e.g., network timeout → "Connection error. Please try again.")
- Error message placement should match existing app patterns (check Settings screen and other forms for consistency)
- Generally prefer inline errors near the form (not alerts/toasts)

### Loading States
- Submit button disabled during request prevents double-submission
- Button text changes to indicate progress ("Logging in...", "Signing up...")
- Spinner icon optional but recommended

### Redirect Logic
- **Signup success** → Redirect to `#home` (or dashboard)
- **Login success** → Redirect to intended route (the route user was trying to access before being redirected to login)
  - If no intended route exists, default to `#home`
  - Implementation: Track redirect parameter or use navigation history
  - Example: User tries to access `#devices` → redirected to `#login` → after successful login → redirect to `#devices`

### Already Logged In Users
- **Behavior**: Users already authenticated CAN still access `#login` and `#signup` screens
- **Rationale**: Allows users to switch accounts without explicitly logging out first
- **No automatic redirect**: Let user remain on auth screens even if token exists

## Dependencies

**Required (from Part 2)**:
- `authStore` with `login()` and `signup()` methods
- `authApi` (used internally by authStore)
- Store should handle token persistence automatically

**Assumed Available**:
- Tailwind CSS configured
- React Router or hash routing setup
- Icon library if implementing password visibility toggle

## Conversation References

**Key Context**:
- "Part 3 of 4 (Frontend Auth Implementation)" - Sequential build, depends on Part 2
- "Match existing app design" - Must reference Settings screen for consistency
- "Form validation before API call (UX optimization)" - Client validation is about UX, not security
- "Loading states prevent double-submit" - Critical for data integrity

**Design Decisions**:
- Hash routing (#login, #signup) - Matches existing app routing pattern
- Inline error messages - Better UX than alerts
- Dark theme (bg-gray-900, bg-gray-800) - Established design system
- Password visibility toggle - Required feature for better UX
- Allow authenticated users to access auth screens - Enables account switching

**Testing Approach**:
- Manual testing emphasized - UI components benefit from visual verification
- Specific test scenarios provided - Validation, backend errors, loading states
- Next steps clearly defined - Part 4 will integrate with user menu

## Design Decisions Summary

**Clarified Requirements**:
1. ✅ **Redirect Behavior**: Login redirects to intended route (where user was trying to go), fallback to `#home`
2. ✅ **Already Logged In**: Users CAN access login/signup even when authenticated (allows account switching)
3. ✅ **Password Visibility Toggle**: Required feature, implement now (not optional)
4. ✅ **Error Message Styling**: Match existing app patterns (check Settings screen for consistency)
5. ✅ **Organization Field**: Organization is the multi-tenant identifier (application customer identity)
6. ✅ **Terminology**: Using "organization" instead of "account" for clarity (organization = tenant = customer)

---

## Next Steps

1. Review and confirm/adjust this specification
2. When ready, run: `/plan spec/active/frontend-auth-login-signup/spec.md` to create implementation plan
3. Implement LoginScreen.tsx
4. Implement SignupScreen.tsx
5. Add routing
6. Manual testing per validation checklist
7. Proceed to Part 4 (Integration)
