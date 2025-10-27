# Feature: UI Integration - Header & User Menu

## Origin
**Linear Issue**: [TRA-97 - UI Integration - Header & User Menu](https://linear.app/trakrf/issue/TRA-97/ui-integration-header-and-user-menu)

**Parent Issue**: TRA-91 (Phase 2 of 3)

User-facing auth feedback. This phase adds visible authentication state to the UI so users can see if they're logged in and access logout functionality.

## Outcome
- User menu visible in header when authenticated (avatar + email + logout)
- "Log In" button visible in header when not authenticated
- Logout clears auth state and redirects to Home
- UI matches existing design patterns

## User Story
**As an authenticated user**
I want to see my email and access a logout button in the header
So that I know I'm logged in and can easily log out when done

**As an unauthenticated user**
I want to see a "Log In" button in the header
So that I can quickly access the login screen

## Context

### Current State
- Header exists with navigation tabs
- No auth-aware UI elements
- User has no visual indication of auth state
- No way to logout (except clearing localStorage manually)

### Desired State
- Header dynamically shows auth state
- When authenticated: User menu with avatar, email, logout dropdown
- When not authenticated: "Log In" button
- Logout action works and redirects to Home

## Technical Requirements

### 1. Create UserMenu Component
**File**: Create `frontend/src/components/UserMenu.tsx`

**Component structure**:
```tsx
interface UserMenuProps {
  user: User;
  onLogout: () => void;
}

export function UserMenu({ user, onLogout }: UserMenuProps) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <div className="user-menu">
      <button
        className="user-menu-trigger"
        onClick={() => setIsOpen(!isOpen)}
      >
        <Avatar initials={getInitials(user.email)} />
        <span className="user-email">{user.email}</span>
      </button>

      {isOpen && (
        <div className="user-menu-dropdown">
          <button onClick={onLogout}>Logout</button>
        </div>
      )}
    </div>
  );
}

function getInitials(email: string): string {
  // e.g., "john.doe@example.com" → "JD"
  const name = email.split('@')[0];
  const parts = name.split(/[._-]/);
  return parts.map(p => p[0].toUpperCase()).slice(0, 2).join('');
}
```

**UI Requirements**:
- Avatar shows user initials (first 2 letters)
- Email text displayed next to avatar
- Dropdown opens on click (or hover, depending on existing patterns)
- Dropdown contains "Logout" button
- Match existing button/dropdown styles
- Responsive: On mobile, hide email text, show only avatar

### 2. Update Header for Authenticated State
**File**: `frontend/src/components/Header.tsx`

**Add auth-aware rendering**:
```tsx
import { useAuthStore } from '@/store/authStore';
import { UserMenu } from '@/components/UserMenu';
import { useNavigate } from 'react-router-dom';

export function Header() {
  const { isAuthenticated, user } = useAuthStore();
  const navigate = useNavigate();

  const handleLogout = () => {
    authStore.getState().logout();
    navigate('/');
  };

  return (
    <header>
      {/* Existing nav/logo */}

      <div className="header-right">
        {isAuthenticated && user ? (
          <UserMenu user={user} onLogout={handleLogout} />
        ) : (
          <Button onClick={() => navigate('/login')}>Log In</Button>
        )}
      </div>
    </header>
  );
}
```

**Implementation notes**:
- Subscribe to authStore state (useAuthStore hook)
- Conditionally render UserMenu or Login button
- handleLogout: Call authStore logout, navigate to Home
- Position in top-right corner of header

### 3. Styling
**File**: Create `frontend/src/components/UserMenu.module.css` (or use existing CSS approach)

**Style requirements**:
- Avatar: Circular, matches brand colors
- Dropdown: Positioned below avatar, matches existing dropdown patterns
- Responsive breakpoints for mobile
- Hover/focus states for accessibility

### 4. Unit/Integration Tests
**File**: Create `frontend/src/components/Header.test.tsx` and `UserMenu.test.tsx`

**Test scenarios**:
- Header renders "Log In" button when not authenticated
- Header renders UserMenu when authenticated
- UserMenu displays correct email
- UserMenu displays correct initials
- Logout button calls onLogout handler
- Clicking "Log In" navigates to /login

## Validation Criteria

### Unit Tests
- [ ] Header renders "Log In" button when isAuthenticated=false
- [ ] Header renders UserMenu when isAuthenticated=true
- [ ] UserMenu displays user.email correctly
- [ ] UserMenu avatar shows correct initials (first 2 letters of email username)
- [ ] Clicking logout calls authStore.logout() and navigates to /
- [ ] Clicking "Log In" navigates to /login

### Manual Testing
- [ ] Log in → See user menu in header
- [ ] User menu shows correct email
- [ ] Avatar shows correct initials
- [ ] Click user menu → Dropdown opens
- [ ] Click logout → Redirects to Home
- [ ] After logout → See "Log In" button
- [ ] Click "Log In" → Navigate to /login
- [ ] Reload page while logged in → User menu persists
- [ ] Mobile view: User menu is responsive

## Technical Constraints

### Dependencies
- Phase 1 complete (authStore with initialize, logout methods)
- Header.tsx exists
- authStore provides isAuthenticated, user state
- React Router for navigation

### Browser Support
- Modern CSS for dropdown positioning
- Mobile breakpoints for responsive design

## Implementation Notes

### UI/UX Considerations
- Match existing header styling (colors, fonts, spacing)
- Dropdown should feel responsive (no delay)
- Logout action should be one-click (no confirmation modal)
- Avatar initials algorithm should handle edge cases (no @, single letter names)

### Accessibility
- Proper ARIA labels for dropdown
- Keyboard navigation support
- Focus states on interactive elements

### Code Organization
- UserMenu is a separate component (reusable, testable)
- Header only handles routing/logout logic
- Styling matches existing patterns (CSS modules, Tailwind, or styled-components)

## Success Criteria

✅ **Functionality**:
- User menu visible when logged in
- "Log In" button visible when not logged in
- Logout works and redirects to Home
- Navigation to /login works

✅ **User Experience**:
- UI matches existing patterns
- Avatar shows initials correctly
- Responsive on mobile
- Logout is instant (no delay)

✅ **Testing**:
- All unit tests pass
- Manual testing scenarios verified

✅ **Code Quality**:
- Properly typed (TypeScript)
- Follows existing component patterns
- Accessible markup

## Future Enhancements (Out of Scope)
- User profile dropdown items (Settings, Profile, etc.)
- Avatar image upload
- Dark mode support for user menu
- Notification badges

## Definition of Done
- [ ] UserMenu component created
- [ ] Header updated with auth-aware rendering
- [ ] Styling matches existing patterns
- [ ] All unit tests pass
- [ ] Manual testing completed
- [ ] Mobile responsive
- [ ] Code reviewed
- [ ] Ready for Phase 3 (Protected Routes)
