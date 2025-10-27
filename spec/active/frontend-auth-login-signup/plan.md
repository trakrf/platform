# Implementation Plan: Frontend Auth - Login & Signup Screens
Generated: 2025-10-27
Specification: spec.md

## Understanding

This feature implements user-facing authentication screens (login and signup) to complete the frontend auth system. Users will be able to create accounts and log in through clean, dark-themed forms with validation, loading states, and error handling. The implementation builds on the auth foundation from Part 2 (authStore, authApi, ProtectedRoute) and prepares for Part 4 (user menu integration).

**Key Requirements:**
- Login screen with email/password inputs
- Signup screen with email/password/organization inputs
- Password visibility toggles on both screens
- Client-side validation (on blur)
- Inline error display from backend
- Loading states during API calls
- Navigation links between login/signup
- Dark theme matching existing design
- Hash-based routing integration (#login, #signup)
- Redirect to intended route after login

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/SettingsScreen.tsx` (lines 1-479) - Dark theme design, form inputs, button styles, card layout
- `frontend/src/components/ProtectedRoute.tsx` (lines 11-24) - Redirect pattern using sessionStorage
- `frontend/src/App.tsx` (lines 22-177) - Hash routing system, lazy loading, tab configuration
- `frontend/src/stores/authStore.ts` (lines 35-84) - Auth store methods: login() and signup()
- `frontend/src/components/__tests__/HomeScreen.test.tsx` - Test pattern with Vitest + RTL

**Files to Create:**
- `frontend/src/components/LoginScreen.tsx` - Login form component with email/password inputs
- `frontend/src/components/SignupScreen.tsx` - Signup form component with email/password/organization inputs
- `frontend/src/components/__tests__/LoginScreen.test.tsx` - Unit and integration tests for LoginScreen
- `frontend/src/components/__tests__/SignupScreen.test.tsx` - Unit and integration tests for SignupScreen

**Files to Modify:**
- `frontend/src/App.tsx` (lines ~22, ~168-177) - Add 'login' and 'signup' to VALID_TABS, tabComponents, loadingScreens; add lazy imports
- `frontend/src/stores/uiStore.ts` (if TabType is defined there) - Add 'login' | 'signup' to TabType union

## Architecture Impact

- **Subsystems affected**: Frontend UI only
- **New dependencies**: None (lucide-react Eye/EyeOff icons already available)
- **Breaking changes**: None
- **Pattern notes**:
  - **CRITICAL**: Spec uses `org_name` but authStore expects `accountName` parameter (internally sends `account_name` to API)
  - Auth store methods throw errors that must be caught
  - Auth store manages `isLoading` state internally
  - SessionStorage pattern for redirect: `sessionStorage.setItem('redirectAfterLogin', currentHash)`

## Task Breakdown

### Task 1: Create LoginScreen Component Structure
**File**: `frontend/src/components/LoginScreen.tsx`
**Action**: CREATE
**Pattern**: Reference `SettingsScreen.tsx` lines 136-206 for dark theme card layout

**Implementation:**
```typescript
import { useState } from 'react';
import { useAuthStore } from '@/stores';
import { Eye, EyeOff } from 'lucide-react';

export default function LoginScreen() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<{ email?: string; password?: string; general?: string }>({});

  const { login, isLoading } = useAuthStore();

  // Validation on blur
  const validateEmail = (email: string) => {
    if (!email) return 'Email is required';
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return 'Invalid email format';
    return null;
  };

  const validatePassword = (password: string) => {
    if (!password) return 'Password is required';
    return null;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Clear previous errors
    setErrors({});

    // Validate all fields
    const emailError = validateEmail(email);
    const passwordError = validatePassword(password);

    if (emailError || passwordError) {
      setErrors({
        email: emailError || undefined,
        password: passwordError || undefined,
      });
      return;
    }

    try {
      await login(email, password);

      // Handle redirect after successful login
      const redirect = sessionStorage.getItem('redirectAfterLogin');
      if (redirect) {
        window.location.hash = `#${redirect}`;
        sessionStorage.removeItem('redirectAfterLogin');
      } else {
        window.location.hash = '#home';
      }
    } catch (err: any) {
      // authStore throws with err.response?.data?.error
      const errorMessage = err.response?.data?.error || err.message || 'Login failed';
      setErrors({ general: errorMessage });
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <h1 className="text-2xl font-semibold text-white mb-6">Log In</h1>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Email input */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Email
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onBlur={() => {
                const error = validateEmail(email);
                if (error) setErrors(prev => ({ ...prev, email: error }));
              }}
              className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              disabled={isLoading}
            />
            {errors.email && (
              <p className="text-red-400 text-sm mt-1">{errors.email}</p>
            )}
          </div>

          {/* Password input with toggle */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Password
            </label>
            <div className="relative">
              <input
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onBlur={() => {
                  const error = validatePassword(password);
                  if (error) setErrors(prev => ({ ...prev, password: error }));
                }}
                className="w-full px-4 py-2 pr-10 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                disabled={isLoading}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-300"
                disabled={isLoading}
              >
                {showPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
              </button>
            </div>
            {errors.password && (
              <p className="text-red-400 text-sm mt-1">{errors.password}</p>
            )}
          </div>

          {/* General error */}
          {errors.general && (
            <div className="bg-red-900/20 border border-red-800 rounded-lg p-3">
              <p className="text-red-400 text-sm">{errors.general}</p>
            </div>
          )}

          {/* Submit button */}
          <button
            type="submit"
            disabled={isLoading}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isLoading ? 'Logging in...' : 'Log In'}
          </button>
        </form>

        {/* Navigation to signup */}
        <p className="text-gray-400 text-sm mt-6 text-center">
          Don't have an account?{' '}
          <a href="#signup" className="text-blue-400 hover:text-blue-300">
            Sign up
          </a>
        </p>
      </div>
    </div>
  );
}
```

**Validation:**
```bash
cd frontend
just lint
just typecheck
```

---

### Task 2: Create SignupScreen Component
**File**: `frontend/src/components/SignupScreen.tsx`
**Action**: CREATE
**Pattern**: Similar to LoginScreen.tsx with additional organization field

**Implementation:**
```typescript
import { useState } from 'react';
import { useAuthStore } from '@/stores';
import { Eye, EyeOff } from 'lucide-react';

export default function SignupScreen() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [organizationName, setOrganizationName] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<{
    email?: string;
    password?: string;
    organizationName?: string;
    general?: string;
  }>({});

  const { signup, isLoading } = useAuthStore();

  // Validation functions
  const validateEmail = (email: string) => {
    if (!email) return 'Email is required';
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return 'Invalid email format';
    return null;
  };

  const validatePassword = (password: string) => {
    if (!password) return 'Password is required';
    if (password.length < 8) return 'Password must be at least 8 characters';
    return null;
  };

  const validateOrganizationName = (name: string) => {
    if (!name) return 'Organization name is required';
    if (name.length < 2) return 'Organization name must be at least 2 characters';
    return null;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Clear previous errors
    setErrors({});

    // Validate all fields
    const emailError = validateEmail(email);
    const passwordError = validatePassword(password);
    const orgError = validateOrganizationName(organizationName);

    if (emailError || passwordError || orgError) {
      setErrors({
        email: emailError || undefined,
        password: passwordError || undefined,
        organizationName: orgError || undefined,
      });
      return;
    }

    try {
      // CRITICAL: authStore.signup expects accountName parameter
      // Spec says org_name but we use accountName here
      await signup(email, password, organizationName);

      // After successful signup, redirect to home
      window.location.hash = '#home';
    } catch (err: any) {
      const errorMessage = err.response?.data?.error || err.message || 'Signup failed';
      setErrors({ general: errorMessage });
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <h1 className="text-2xl font-semibold text-white mb-6">Sign Up</h1>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Email input */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Email
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onBlur={() => {
                const error = validateEmail(email);
                if (error) setErrors(prev => ({ ...prev, email: error }));
              }}
              className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              disabled={isLoading}
            />
            {errors.email && (
              <p className="text-red-400 text-sm mt-1">{errors.email}</p>
            )}
          </div>

          {/* Password input with toggle */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Password
            </label>
            <div className="relative">
              <input
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onBlur={() => {
                  const error = validatePassword(password);
                  if (error) setErrors(prev => ({ ...prev, password: error }));
                }}
                className="w-full px-4 py-2 pr-10 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                disabled={isLoading}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-300"
                disabled={isLoading}
              >
                {showPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
              </button>
            </div>
            {errors.password && (
              <p className="text-red-400 text-sm mt-1">{errors.password}</p>
            )}
          </div>

          {/* Organization name input */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Organization Name
            </label>
            <input
              type="text"
              value={organizationName}
              onChange={(e) => setOrganizationName(e.target.value)}
              onBlur={() => {
                const error = validateOrganizationName(organizationName);
                if (error) setErrors(prev => ({ ...prev, organizationName: error }));
              }}
              className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              disabled={isLoading}
            />
            {errors.organizationName && (
              <p className="text-red-400 text-sm mt-1">{errors.organizationName}</p>
            )}
          </div>

          {/* General error */}
          {errors.general && (
            <div className="bg-red-900/20 border border-red-800 rounded-lg p-3">
              <p className="text-red-400 text-sm">{errors.general}</p>
            </div>
          )}

          {/* Submit button */}
          <button
            type="submit"
            disabled={isLoading}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isLoading ? 'Signing up...' : 'Sign Up'}
          </button>
        </form>

        {/* Navigation to login */}
        <p className="text-gray-400 text-sm mt-6 text-center">
          Already have an account?{' '}
          <a href="#login" className="text-blue-400 hover:text-blue-300">
            Log in
          </a>
        </p>
      </div>
    </div>
  );
}
```

**Validation:**
```bash
cd frontend
just lint
just typecheck
```

---

### Task 3: Add Routing Integration
**File**: `frontend/src/App.tsx`
**Action**: MODIFY
**Pattern**: Follow existing tab registration pattern (lines 13-21, 168-177)

**Changes:**

1. Add lazy imports (after line 20):
```typescript
const LoginScreen = lazyWithRetry(() => import('@/components/LoginScreen'));
const SignupScreen = lazyWithRetry(() => import('@/components/SignupScreen'));
```

2. Update VALID_TABS array (line 22):
```typescript
const VALID_TABS: TabType[] = ['home', 'inventory', 'locate', 'barcode', 'assets', 'locations', 'settings', 'help', 'login', 'signup'];
```

3. Add to tabComponents (inside renderTabContent, ~line 168):
```typescript
const tabComponents = {
  home: HomeScreen,
  inventory: InventoryScreen,
  locate: LocateScreen,
  barcode: BarcodeScreen,
  assets: AssetsScreen,
  locations: LocationsScreen,
  settings: SettingsScreen,
  help: HelpScreen,
  login: LoginScreen,
  signup: SignupScreen,
};
```

4. Add to loadingScreens (inside renderTabContent, ~line 179):
```typescript
const loadingScreens = {
  home: LoadingScreen,
  inventory: InventoryLoadingScreen,
  locate: LocateLoadingScreen,
  barcode: BarcodeLoadingScreen,
  assets: LoadingScreen,
  locations: LoadingScreen,
  settings: SettingsLoadingScreen,
  help: HelpLoadingScreen,
  login: LoadingScreen,
  signup: LoadingScreen,
};
```

5. **Check uiStore.ts for TabType** and add 'login' | 'signup' if needed:
```bash
cd frontend
grep "type TabType" src/stores/uiStore.ts
```
If TabType is defined there, add 'login' and 'signup' to the union.

**Validation:**
```bash
cd frontend
just lint
just typecheck
just build
```

---

### Task 4: Create LoginScreen Tests
**File**: `frontend/src/components/__tests__/LoginScreen.test.tsx`
**Action**: CREATE
**Pattern**: Reference `HomeScreen.test.tsx` for Vitest + RTL pattern

**Implementation:**
```typescript
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import LoginScreen from '@/components/LoginScreen';
import { useAuthStore } from '@/stores';

describe('LoginScreen', () => {
  const mockLogin = vi.fn();

  beforeEach(() => {
    mockLogin.mockClear();
    useAuthStore.setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,
    });
    useAuthStore.getState().login = mockLogin;

    // Clear sessionStorage
    sessionStorage.clear();
  });

  afterEach(() => {
    cleanup();
  });

  describe('Rendering', () => {
    it('should render login form with all fields', () => {
      render(<LoginScreen />);

      expect(screen.getByText('Log In')).toBeInTheDocument();
      expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /log in/i })).toBeInTheDocument();
      expect(screen.getByText(/don't have an account/i)).toBeInTheDocument();
    });

    it('should render password visibility toggle', () => {
      render(<LoginScreen />);

      const toggleButton = screen.getByRole('button', { name: '' }); // Icon button
      expect(toggleButton).toBeInTheDocument();
    });
  });

  describe('Validation', () => {
    it('should show email error on blur with invalid email', async () => {
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      fireEvent.change(emailInput, { target: { value: 'invalid-email' } });
      fireEvent.blur(emailInput);

      await waitFor(() => {
        expect(screen.getByText(/invalid email format/i)).toBeInTheDocument();
      });
    });

    it('should show password error on blur with empty password', async () => {
      render(<LoginScreen />);

      const passwordInput = screen.getByLabelText(/password/i);
      fireEvent.blur(passwordInput);

      await waitFor(() => {
        expect(screen.getByText(/password is required/i)).toBeInTheDocument();
      });
    });

    it('should not submit with validation errors', async () => {
      render(<LoginScreen />);

      const submitButton = screen.getByRole('button', { name: /log in/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockLogin).not.toHaveBeenCalled();
      });
    });
  });

  describe('Password Visibility Toggle', () => {
    it('should toggle password visibility on icon click', () => {
      render(<LoginScreen />);

      const passwordInput = screen.getByLabelText(/password/i) as HTMLInputElement;
      const toggleButton = screen.getByRole('button', { name: '' });

      expect(passwordInput.type).toBe('password');

      fireEvent.click(toggleButton);
      expect(passwordInput.type).toBe('text');

      fireEvent.click(toggleButton);
      expect(passwordInput.type).toBe('password');
    });
  });

  describe('Form Submission', () => {
    it('should call login with correct credentials on valid submit', async () => {
      mockLogin.mockResolvedValue(undefined);
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockLogin).toHaveBeenCalledWith('test@example.com', 'password123');
      });
    });

    it('should redirect to home after successful login without redirect param', async () => {
      mockLogin.mockResolvedValue(undefined);
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(window.location.hash).toBe('#home');
      });
    });

    it('should redirect to intended route after successful login', async () => {
      sessionStorage.setItem('redirectAfterLogin', 'devices');
      mockLogin.mockResolvedValue(undefined);
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(window.location.hash).toBe('#devices');
        expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
      });
    });

    it('should display backend error message on failed login', async () => {
      const errorMessage = 'Invalid credentials';
      mockLogin.mockRejectedValue({
        response: { data: { error: errorMessage } }
      });

      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'wrongpassword' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText(errorMessage)).toBeInTheDocument();
      });
    });
  });

  describe('Loading State', () => {
    it('should disable form during loading', async () => {
      mockLogin.mockImplementation(() => new Promise(() => {})); // Never resolves
      useAuthStore.setState({ isLoading: true });

      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /logging in/i });

      expect(emailInput).toBeDisabled();
      expect(passwordInput).toBeDisabled();
      expect(submitButton).toBeDisabled();
    });

    it('should show loading text during submission', async () => {
      useAuthStore.setState({ isLoading: true });
      render(<LoginScreen />);

      expect(screen.getByText(/logging in/i)).toBeInTheDocument();
    });
  });

  describe('Navigation', () => {
    it('should have link to signup page', () => {
      render(<LoginScreen />);

      const signupLink = screen.getByText(/sign up/i);
      expect(signupLink).toHaveAttribute('href', '#signup');
    });
  });
});
```

**Validation:**
```bash
cd frontend
just test
```

---

### Task 5: Create SignupScreen Tests
**File**: `frontend/src/components/__tests__/SignupScreen.test.tsx`
**Action**: CREATE
**Pattern**: Similar to LoginScreen.test.tsx with organization field tests

**Implementation:**
```typescript
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import SignupScreen from '@/components/SignupScreen';
import { useAuthStore } from '@/stores';

describe('SignupScreen', () => {
  const mockSignup = vi.fn();

  beforeEach(() => {
    mockSignup.mockClear();
    useAuthStore.setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,
    });
    useAuthStore.getState().signup = mockSignup;
  });

  afterEach(() => {
    cleanup();
  });

  describe('Rendering', () => {
    it('should render signup form with all fields', () => {
      render(<SignupScreen />);

      expect(screen.getByText('Sign Up')).toBeInTheDocument();
      expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/organization name/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /sign up/i })).toBeInTheDocument();
      expect(screen.getByText(/already have an account/i)).toBeInTheDocument();
    });

    it('should render password visibility toggle', () => {
      render(<SignupScreen />);

      const toggleButton = screen.getByRole('button', { name: '' });
      expect(toggleButton).toBeInTheDocument();
    });
  });

  describe('Validation', () => {
    it('should show email error on blur with invalid email', async () => {
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      fireEvent.change(emailInput, { target: { value: 'invalid-email' } });
      fireEvent.blur(emailInput);

      await waitFor(() => {
        expect(screen.getByText(/invalid email format/i)).toBeInTheDocument();
      });
    });

    it('should show password error on blur with short password', async () => {
      render(<SignupScreen />);

      const passwordInput = screen.getByLabelText(/password/i);
      fireEvent.change(passwordInput, { target: { value: 'short' } });
      fireEvent.blur(passwordInput);

      await waitFor(() => {
        expect(screen.getByText(/at least 8 characters/i)).toBeInTheDocument();
      });
    });

    it('should show organization error on blur with short name', async () => {
      render(<SignupScreen />);

      const orgInput = screen.getByLabelText(/organization name/i);
      fireEvent.change(orgInput, { target: { value: 'a' } });
      fireEvent.blur(orgInput);

      await waitFor(() => {
        expect(screen.getByText(/at least 2 characters/i)).toBeInTheDocument();
      });
    });

    it('should not submit with validation errors', async () => {
      render(<SignupScreen />);

      const submitButton = screen.getByRole('button', { name: /sign up/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockSignup).not.toHaveBeenCalled();
      });
    });
  });

  describe('Password Visibility Toggle', () => {
    it('should toggle password visibility on icon click', () => {
      render(<SignupScreen />);

      const passwordInput = screen.getByLabelText(/password/i) as HTMLInputElement;
      const toggleButton = screen.getByRole('button', { name: '' });

      expect(passwordInput.type).toBe('password');

      fireEvent.click(toggleButton);
      expect(passwordInput.type).toBe('text');

      fireEvent.click(toggleButton);
      expect(passwordInput.type).toBe('password');
    });
  });

  describe('Form Submission', () => {
    it('should call signup with correct data on valid submit', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.change(orgInput, { target: { value: 'Acme Corp' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockSignup).toHaveBeenCalledWith('test@example.com', 'password123', 'Acme Corp');
      });
    });

    it('should redirect to home after successful signup', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.change(orgInput, { target: { value: 'Acme Corp' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(window.location.hash).toBe('#home');
      });
    });

    it('should display backend error message on failed signup', async () => {
      const errorMessage = 'Email already exists';
      mockSignup.mockRejectedValue({
        response: { data: { error: errorMessage } }
      });

      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'existing@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.change(orgInput, { target: { value: 'Acme Corp' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText(errorMessage)).toBeInTheDocument();
      });
    });
  });

  describe('Loading State', () => {
    it('should disable form during loading', async () => {
      mockSignup.mockImplementation(() => new Promise(() => {}));
      useAuthStore.setState({ isLoading: true });

      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /signing up/i });

      expect(emailInput).toBeDisabled();
      expect(passwordInput).toBeDisabled();
      expect(orgInput).toBeDisabled();
      expect(submitButton).toBeDisabled();
    });

    it('should show loading text during submission', async () => {
      useAuthStore.setState({ isLoading: true });
      render(<SignupScreen />);

      expect(screen.getByText(/signing up/i)).toBeInTheDocument();
    });
  });

  describe('Navigation', () => {
    it('should have link to login page', () => {
      render(<SignupScreen />);

      const loginLink = screen.getByText(/log in/i);
      expect(loginLink).toHaveAttribute('href', '#login');
    });
  });
});
```

**Validation:**
```bash
cd frontend
just test
```

---

## Risk Assessment

- **Risk**: Field name mismatch (spec says `org_name`, authStore expects `accountName`)
  **Mitigation**: Documented in Task 2 pseudocode. AuthStore internally sends `account_name` to API. Use `accountName` parameter consistently.

- **Risk**: TabType definition location unknown
  **Mitigation**: Task 3 includes grep check for TabType location and conditional modification

- **Risk**: Auth errors may vary in structure
  **Mitigation**: Error handling catches multiple error formats: `err.response?.data?.error || err.message || 'Default message'`

- **Risk**: SessionStorage not cleared on logout
  **Mitigation**: ProtectedRoute only sets redirect for non-auth routes. After login, sessionStorage is cleared. No stale redirects possible.

- **Risk**: Password toggle accessibility
  **Mitigation**: Using button type="button" to prevent form submission, aria-label could be added if needed

## Integration Points

- **Auth Store**: Direct calls to `login()` and `signup()` methods
- **Route System**: Extends App.tsx tab routing with 'login' and 'signup' tabs
- **SessionStorage**: Uses 'redirectAfterLogin' key following ProtectedRoute pattern
- **Icon Library**: lucide-react Eye/EyeOff icons
- **Styles**: Tailwind classes matching SettingsScreen dark theme
- **Loading States**: Uses authStore.isLoading for form disable/button text

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change:
```bash
cd frontend
just lint        # Gate 1: Syntax & Style
just typecheck   # Gate 2: Type Safety
just test        # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**After each task:**
```bash
cd frontend
just lint
just typecheck
just test
```

**Final validation:**
```bash
cd frontend
just build
just validate  # Runs all checks
```

**Manual testing checklist** (from spec.md lines 243-295):
- Signup flow: navigation, form validation, submission, errors, loading, password toggle
- Login flow: navigation, form validation, submission, errors, loading, password toggle
- Navigation between screens
- Design consistency with dark theme
- Edge cases: network errors, double-submit prevention, already logged in users

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase at SettingsScreen.tsx, ProtectedRoute.tsx
✅ All clarifying questions answered (follow existing patterns)
✅ Existing test patterns to follow at HomeScreen.test.tsx
✅ Auth store exists and is well-defined (authStore.ts)
✅ Icon library available (lucide-react)
⚠️ TabType location needs verification (grep check in Task 3)
⚠️ Field name inconsistency documented (org_name vs accountName)

**Assessment**: High confidence implementation following established patterns with clear validation gates.

**Estimated one-pass success probability**: 85%

**Reasoning**: The implementation follows well-established patterns in the codebase (SettingsScreen for design, ProtectedRoute for redirects, HomeScreen for testing). The auth store is fully implemented from Part 2. The main complexity is in the routing integration (extending tab system) and ensuring proper error handling. The field name mismatch (org_name vs accountName) is documented and straightforward to handle. Test patterns are clear and well-established. The 15% risk accounts for potential routing integration issues and unknown TabType location.
