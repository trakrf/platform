# Implementation Plan: UI Integration - Header & User Menu
Generated: 2025-10-28
Specification: spec/active/auth-ui/spec.md

## Understanding

This feature adds visible authentication state to the Header component. When authenticated, users see a UserMenu with their avatar (initials), email, and logout dropdown. When not authenticated, users see a "Log In" button. The logout action clears auth state and navigates to the Home screen.

**Key Requirements:**
- Avatar shows user initials (first 2 letters from email username)
- Dropdown opens on click, closes on outside click
- Uses @headlessui/react Menu for accessibility
- Logout navigates to Home (/#home)
- Mobile: Hide email text below 640px (show avatar only)
- Auth UI always visible in top-right corner
- Existing Connect Device button shown conditionally beside auth UI

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/ShareButton.tsx` (lines 1-71) - @headlessui/react Menu pattern, lucide-react icons, Tailwind dropdown styling
- `frontend/src/components/Header.tsx` (lines 1-324) - Component structure, Tailwind responsive design, conditional rendering patterns
- `frontend/src/stores/authStore.ts` (lines 1-190) - Auth state structure (user, token, isAuthenticated, logout method)
- `frontend/src/stores/authStore.test.ts` (lines 1-367) - Vitest testing patterns for Zustand stores
- `frontend/src/App.tsx` (lines 1-100) - Navigation pattern: `useUIStore.getState().setActiveTab()`

**Files to Create:**
- `frontend/src/components/Avatar.tsx` - Reusable avatar component with initials generation
- `frontend/src/components/Avatar.test.tsx` - Unit tests for Avatar component
- `frontend/src/components/UserMenu.tsx` - Dropdown menu with user info and logout
- `frontend/src/components/UserMenu.test.tsx` - Unit tests for UserMenu component
- `frontend/src/components/Header.test.tsx` - Integration tests for Header auth rendering

**Files to Modify:**
- `frontend/src/components/Header.tsx` (lines 52-217) - Add auth-aware rendering in button controls section

## Architecture Impact

**Subsystems affected:**
- UI layer (Header, new Avatar/UserMenu components)
- Auth layer (useAuthStore integration)
- Navigation (logout navigation to Home)

**New dependencies:**
- None (uses existing @headlessui/react, lucide-react, zustand)

**Breaking changes:**
- None (additive feature)

## Task Breakdown

### Task 1: Create Avatar Component
**File**: `frontend/src/components/Avatar.tsx`
**Action**: CREATE
**Pattern**: Reference ShareButton.tsx (lines 1-25) for component structure and prop typing

**Implementation:**
```tsx
// Avatar component - displays user initials in a circular badge
interface AvatarProps {
  email: string;
  className?: string;
}

export function Avatar({ email, className = '' }: AvatarProps) {
  const initials = getInitials(email);

  return (
    <div className={`flex items-center justify-center w-8 h-8 rounded-full bg-blue-600 text-white font-semibold text-sm ${className}`}>
      {initials}
    </div>
  );
}

function getInitials(email: string): string {
  // Extract username before @
  const username = email.split('@')[0];

  // Split by common separators (., _, -)
  const parts = username.split(/[._-]/);

  // Take first 2 parts, get first letter of each, uppercase
  const initials = parts
    .slice(0, 2)
    .map(part => part[0]?.toUpperCase() || '')
    .join('');

  // Fallback to first 2 chars if no separators
  return initials || username.slice(0, 2).toUpperCase();
}
```

**Validation:**
```bash
# From project root
just frontend lint
just frontend typecheck
```

---

### Task 2: Create UserMenu Component
**File**: `frontend/src/components/UserMenu.tsx`
**Action**: CREATE
**Pattern**: Reference ShareButton.tsx (lines 24-71) for @headlessui/react Menu usage

**Implementation:**
```tsx
import { Menu } from '@headlessui/react';
import { ChevronDown } from 'lucide-react';
import { Avatar } from './Avatar';
import type { User } from '@/lib/api/auth';

interface UserMenuProps {
  user: User;
  onLogout: () => void;
}

export function UserMenu({ user, onLogout }: UserMenuProps) {
  return (
    <Menu as="div" className="relative inline-block text-left">
      <Menu.Button className="flex items-center gap-2 px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
        <Avatar email={user.email} />
        <span className="hidden sm:inline-block text-sm font-medium text-gray-900 dark:text-gray-100">
          {user.email}
        </span>
        <ChevronDown className="w-4 h-4 text-gray-500 dark:text-gray-400" />
      </Menu.Button>

      <Menu.Items className="absolute right-0 mt-2 w-48 origin-top-right divide-y divide-gray-100 dark:divide-gray-700 rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
        <div className="p-1">
          <Menu.Item>
            {({ active }) => (
              <button
                onClick={onLogout}
                className={`${
                  active ? 'bg-gray-100 dark:bg-gray-700' : ''
                } group flex w-full items-center rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
              >
                Logout
              </button>
            )}
          </Menu.Item>
        </div>
      </Menu.Items>
    </Menu>
  );
}
```

**Validation:**
```bash
# From project root
just frontend lint
just frontend typecheck
```

---

### Task 3: Update Header for Auth-Aware Rendering
**File**: `frontend/src/components/Header.tsx`
**Action**: MODIFY
**Pattern**: Reference existing Header structure (lines 52-217)

**Changes:**
1. Import auth components and store:
```tsx
import { useAuthStore } from '@/stores';
import { UserMenu } from './UserMenu';
```

2. Add auth state subscription (after line 63):
```tsx
const { isAuthenticated, user } = useAuthStore();
```

3. Add logout handler (after line 96):
```tsx
const handleLogout = () => {
  useAuthStore.getState().logout();
  useUIStore.getState().setActiveTab('home');
};
```

4. Replace button controls section (lines 218-311) with:
```tsx
{/* Button Controls */}
<div className="flex items-center gap-2 md:gap-3">
  {/* Auth UI - Always visible */}
  {isAuthenticated && user ? (
    <UserMenu user={user} onLogout={handleLogout} />
  ) : (
    <button
      onClick={() => useUIStore.getState().setActiveTab('login')}
      className="px-3 md:px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-semibold text-sm md:text-base transition-colors"
    >
      Log In
    </button>
  )}

  {/* Connect Device Button - Conditionally shown */}
  {shouldShowConnectButton && (
    <>
      {/* Battery, Trigger, Connect button - existing code */}
      {/* ... (keep existing battery/trigger indicators and connect button) */}
    </>
  )}
</div>
```

**Validation:**
```bash
# From project root
just frontend lint
just frontend typecheck
```

---

### Task 4: Add Tests for Avatar Component
**File**: `frontend/src/components/Avatar.test.tsx`
**Action**: CREATE
**Pattern**: Reference authStore.test.ts (lines 1-50) for Vitest test structure

**Implementation:**
```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Avatar } from './Avatar';

describe('Avatar', () => {
  it('should display correct initials for standard email (first.last@domain)', () => {
    render(<Avatar email="john.doe@example.com" />);
    expect(screen.getByText('JD')).toBeInTheDocument();
  });

  it('should display correct initials for underscore separator', () => {
    render(<Avatar email="jane_smith@example.com" />);
    expect(screen.getByText('JS')).toBeInTheDocument();
  });

  it('should display correct initials for hyphen separator', () => {
    render(<Avatar email="bob-jones@example.com" />);
    expect(screen.getByText('BJ')).toBeInTheDocument();
  });

  it('should fallback to first 2 chars if no separators', () => {
    render(<Avatar email="alice@example.com" />);
    expect(screen.getByText('AL')).toBeInTheDocument();
  });

  it('should handle single letter email', () => {
    render(<Avatar email="a@example.com" />);
    expect(screen.getByText('A')).toBeInTheDocument();
  });

  it('should handle email with multiple parts (take first 2)', () => {
    render(<Avatar email="john.paul.smith@example.com" />);
    expect(screen.getByText('JP')).toBeInTheDocument();
  });

  it('should apply custom className', () => {
    const { container } = render(<Avatar email="test@example.com" className="custom-class" />);
    const avatar = container.querySelector('.custom-class');
    expect(avatar).toBeInTheDocument();
  });
});
```

**Validation:**
```bash
# From project root
just frontend test
```

---

### Task 5: Add Tests for UserMenu Component
**File**: `frontend/src/components/UserMenu.test.tsx`
**Action**: CREATE
**Pattern**: Reference authStore.test.ts (lines 145-170) for event handler testing

**Implementation:**
```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { UserMenu } from './UserMenu';
import type { User } from '@/lib/api/auth';

const mockUser: User = {
  id: 1,
  email: 'test@example.com',
  name: 'Test User',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

describe('UserMenu', () => {
  it('should render user email', () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);
    expect(screen.getByText('test@example.com')).toBeInTheDocument();
  });

  it('should render avatar with correct initials', () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);
    expect(screen.getByText('TE')).toBeInTheDocument(); // "test" -> "TE"
  });

  it('should open dropdown on button click', () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    expect(screen.getByText('Logout')).toBeInTheDocument();
  });

  it('should call onLogout when logout button clicked', () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);

    // Open dropdown
    const menuButton = screen.getByRole('button');
    fireEvent.click(menuButton);

    // Click logout
    const logoutButton = screen.getByText('Logout');
    fireEvent.click(logoutButton);

    expect(onLogout).toHaveBeenCalledTimes(1);
  });

  it('should hide email text on mobile (< 640px)', () => {
    const onLogout = vi.fn();
    const { container } = render(<UserMenu user={mockUser} onLogout={onLogout} />);

    const emailSpan = container.querySelector('.hidden.sm\\:inline-block');
    expect(emailSpan).toBeInTheDocument();
    expect(emailSpan).toHaveTextContent('test@example.com');
  });
});
```

**Validation:**
```bash
# From project root
just frontend test
```

---

### Task 6: Add Tests for Header Auth Integration
**File**: `frontend/src/components/Header.test.tsx`
**Action**: CREATE
**Pattern**: Reference authStore.test.ts (lines 29-57) for store mocking patterns

**Implementation:**
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import Header from './Header';
import { useAuthStore, useUIStore } from '@/stores';

// Mock stores
vi.mock('@/stores', () => ({
  useAuthStore: vi.fn(),
  useUIStore: vi.fn(),
  useDeviceStore: vi.fn(() => ({})),
}));

describe('Header - Auth Integration', () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Default UIStore mock
    vi.mocked(useUIStore).mockReturnValue({
      activeTab: 'inventory',
    } as any);
  });

  it('should render "Log In" button when not authenticated', () => {
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      user: null,
    } as any);

    render(<Header />);
    expect(screen.getByText('Log In')).toBeInTheDocument();
  });

  it('should render UserMenu when authenticated', () => {
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: true,
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test User',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      },
    } as any);

    render(<Header />);
    expect(screen.getByText('test@example.com')).toBeInTheDocument();
    expect(screen.getByText('TE')).toBeInTheDocument(); // Avatar initials
  });

  it('should navigate to login screen when "Log In" button clicked', () => {
    const setActiveTab = vi.fn();
    vi.mocked(useUIStore).mockReturnValue({
      activeTab: 'inventory',
    } as any);
    vi.mocked(useUIStore).getState = vi.fn(() => ({
      setActiveTab,
    })) as any;

    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      user: null,
    } as any);

    render(<Header />);

    const loginButton = screen.getByText('Log In');
    fireEvent.click(loginButton);

    expect(setActiveTab).toHaveBeenCalledWith('login');
  });

  it('should call logout and navigate to home when logout clicked', () => {
    const logout = vi.fn();
    const setActiveTab = vi.fn();

    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: true,
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test User',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      },
    } as any);
    vi.mocked(useAuthStore).getState = vi.fn(() => ({
      logout,
    })) as any;

    vi.mocked(useUIStore).mockReturnValue({
      activeTab: 'inventory',
    } as any);
    vi.mocked(useUIStore).getState = vi.fn(() => ({
      setActiveTab,
    })) as any;

    render(<Header />);

    // Open dropdown
    const menuButton = screen.getByRole('button', { name: /test@example.com/i });
    fireEvent.click(menuButton);

    // Click logout
    const logoutButton = screen.getByText('Logout');
    fireEvent.click(logoutButton);

    expect(logout).toHaveBeenCalledTimes(1);
    expect(setActiveTab).toHaveBeenCalledWith('home');
  });

  it('should not render auth UI on home screen', () => {
    vi.mocked(useUIStore).mockReturnValue({
      activeTab: 'home',
    } as any);

    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      user: null,
    } as any);

    render(<Header />);

    // Button controls section should not render on home
    expect(screen.queryByText('Log In')).not.toBeInTheDocument();
  });
});
```

**Validation:**
```bash
# From project root
just frontend test
```

---

### Task 7: Final Validation - All Gates
**Action**: Run full frontend validation suite
**Pattern**: Reference spec/stack.md validation commands

**Commands:**
```bash
# From project root
just frontend lint        # Gate 1: Syntax & Style
just frontend typecheck   # Gate 2: Type Safety
just frontend test        # Gate 3: Unit Tests
just frontend build       # Gate 4: Build Success
```

**Expected Results:**
- Lint: 0 errors, 0 warnings
- Typecheck: No type errors
- Tests: All tests passing (Avatar: 7 tests, UserMenu: 5 tests, Header: 6 tests = 18+ new tests)
- Build: Successful production build

**If any gate fails:**
1. Fix the issue immediately
2. Re-run the failed gate
3. Repeat until all gates pass
4. Do NOT proceed if any gate fails

---

## Risk Assessment

**Risk**: Header layout complexity with multiple conditional elements
**Mitigation**: Use flexbox gap utilities, test on multiple screen sizes, reference existing Header responsive patterns

**Risk**: UserMenu dropdown z-index conflicts
**Mitigation**: Use z-50 (matches ShareButton pattern), test with existing modals/dropdowns

**Risk**: Auth state initialization race condition
**Mitigation**: App.tsx already calls useAuthStore.getState().initialize() on mount (line 36)

**Risk**: getInitials edge cases (empty email, no @, special chars)
**Mitigation**: Comprehensive test coverage (7 test cases), defensive fallback logic

**Risk**: Mobile responsive breakpoints inconsistent
**Mitigation**: Follow existing Header patterns (md:, sm: breakpoints), test on mobile viewport

---

## Integration Points

**Store updates:**
- useAuthStore: Subscribe to isAuthenticated, user state in Header
- useUIStore: Call setActiveTab('home') on logout, setActiveTab('login') on Login button click

**Route changes:**
- Logout: Navigate to home (/#home) via useUIStore.getState().setActiveTab('home')
- Login button: Navigate to login (/#login) via useUIStore.getState().setActiveTab('login')

**Component integration:**
- Header.tsx: Import and render UserMenu (authenticated) or Login button (not authenticated)
- UserMenu: Import and render Avatar component

---

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change, run commands from `spec/stack.md`:
- **Gate 1: Syntax & Style** → `just frontend lint`
- **Gate 2: Type Safety** → `just frontend typecheck`
- **Gate 3: Unit Tests** → `just frontend test`

**Enforcement Rules:**
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts on same gate → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

---

## Validation Sequence

**After each task (1-6):**
```bash
just frontend lint       # Must pass
just frontend typecheck  # Must pass
```

**After test tasks (4-6):**
```bash
just frontend test       # Must pass - verify new tests work
```

**Final validation (Task 7):**
```bash
just frontend lint       # Full codebase lint
just frontend typecheck  # Full type checking
just frontend test       # All tests (existing + new)
just frontend build      # Production build
```

---

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)

**Complexity Factors**:
- 📁 File Impact: Creating 5 files, modifying 1 file (6 files total)
- 🔗 Subsystems: Touching 3 subsystems (UI, Auth, Navigation)
- 🔢 Task Estimate: 7 subtasks (well-scoped)
- 📦 Dependencies: 0 new packages
- 🆕 Pattern Novelty: Using existing patterns (@headlessui/react, Tailwind, Zustand)

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar patterns found in codebase:
  - ShareButton.tsx (Menu component usage)
  - HamburgerMenu.tsx (dropdown patterns)
  - Header.tsx (layout and styling)
  - authStore.ts (auth state management)
  - authStore.test.ts (testing patterns)
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow
- ✅ No external dependencies required
- ✅ No breaking changes

**Assessment**: High confidence implementation. All patterns exist in codebase, requirements are clear, and scope is well-defined. The main complexity is integrating auth UI into existing Header layout, but reference patterns provide clear guidance.

**Estimated one-pass success probability**: 85%

**Reasoning**: Well-scoped feature with clear requirements and existing patterns to follow. Risk areas are minor (layout integration, mobile responsiveness, edge cases) and mitigated by comprehensive tests and validation gates. The 15% risk accounts for potential Header layout adjustments and mobile styling iterations.
