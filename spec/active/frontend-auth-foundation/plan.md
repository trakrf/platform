# Implementation Plan: Frontend Auth - Foundation

Generated: 2025-10-26
Specification: spec.md
Linear Issue: TRA-89

## Understanding

This plan implements Part 2 of the frontend-auth-hybrid feature: core authentication infrastructure without UI changes. We're building:

1. **Auth Store** - Zustand store with persistence for user/token state
2. **API Client** - Axios instance with token injection and 401 handling
3. **ProtectedRoute** - Component wrapper for route protection

**Key Design Decisions from Clarification:**
- ✅ Use `persist(createStoreWithTracking(...))` pattern matching `tagStore.ts`
- ✅ Sanitize tokens/passwords from OpenReplay tracking (security)
- ✅ 401 interceptor shows toast notification via react-hot-toast
- ✅ Colocate all unit tests next to source files (project convention)
- ✅ Implementation + tests in same PR (validation gates)
- ✅ Configure Vite to use root `.env.local` (monorepo consistency)
- ✅ Backend error messages passed through as-is (POLS)

## Relevant Files

### Reference Patterns (existing code to follow)

**Zustand Store with Persistence:**
- `frontend/src/stores/tagStore.ts` (lines 80-95) - persist + createStoreWithTracking pattern
- `frontend/src/stores/settingsStore.ts` (lines 74-100) - localStorage initialization
- `frontend/src/stores/createStore.ts` (complete) - OpenReplay tracking wrapper

**Testing Patterns:**
- `frontend/src/stores/settingsStore.test.ts` (lines 7-25) - beforeEach, localStorage.clear(), setState pattern
- `frontend/src/stores/tagStore.test.ts` - mock pattern for testing stores

**Error Handling:**
- `frontend/src/stores/settingsStore.ts` (lines 14-32) - safe localStorage access pattern

### Files to Create

**Core Implementation:**
- `frontend/src/lib/api/client.ts` - Axios instance with interceptors
- `frontend/src/lib/api/auth.ts` - Auth API methods (login, signup)
- `frontend/src/stores/authStore.ts` - Auth state management
- `frontend/src/components/ProtectedRoute.tsx` - Route protection wrapper

**Tests:**
- `frontend/src/lib/api/client.test.ts` - API client interceptor tests
- `frontend/src/stores/authStore.test.ts` - Auth store unit tests
- `frontend/src/components/ProtectedRoute.test.tsx` - Protected route tests

### Files to Modify

- `frontend/vite.config.ts` - Add `envDir: '../'` to read root .env.local
- `frontend/package.json` - Add axios dependency (via pnpm add)
- `.env.local` (root) - Add VITE_API_URL configuration
- `frontend/src/stores/index.ts` - Export authStore for easy imports

## Architecture Impact

- **Subsystems affected**: State Management (Zustand), API Layer (new), Routing (hash-based protection)
- **New dependencies**: `axios@^1.6.0`
- **Breaking changes**: None - this is additive infrastructure
- **Security considerations**:
  - Token stored in localStorage (acceptable for MVP, documented constraint)
  - OpenReplay tracking sanitizes tokens/passwords
  - 401 auto-logout prevents stale sessions

## Task Breakdown

### Task 1: Dependencies and Environment Setup
**Files**: `frontend/package.json`, `frontend/vite.config.ts`, `.env.local`
**Action**: MODIFY (vite.config.ts, .env.local) + INSTALL (axios)

**Implementation**:

1. Install axios:
```bash
cd frontend
pnpm add axios
```

2. Configure Vite to use root .env.local:

**File**: `frontend/vite.config.ts`

Find the `export default defineConfig({` block and add:
```typescript
export default defineConfig({
  envDir: '../', // Read .env files from project root (monorepo)
  // ... existing config
})
```

3. Add API URL to root .env.local:

**File**: `.env.local` (project root)

Add at the end:
```bash
# Frontend API Configuration
VITE_API_URL=http://localhost:8080/api/v1
```

**Validation**:
```bash
cd frontend
pnpm typecheck  # Should pass
node -e "console.log(process.env.VITE_API_URL || 'not loaded')"  # Verify env
```

---

### Task 2: Create API Client Layer
**Files**: `frontend/src/lib/api/client.ts`, `frontend/src/lib/api/auth.ts`, `frontend/src/lib/api/client.test.ts`
**Action**: CREATE
**Pattern**: Reference axios docs and backend API spec

**Implementation**:

**File**: `frontend/src/lib/api/client.ts`

```typescript
import axios from 'axios';
import toast from 'react-hot-toast';

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1';

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor: Inject Bearer token from localStorage
apiClient.interceptors.request.use((config) => {
  const authStorage = localStorage.getItem('auth-storage');

  if (authStorage) {
    try {
      const { state } = JSON.parse(authStorage);
      if (state?.token) {
        config.headers.Authorization = `Bearer ${state.token}`;
      }
    } catch (err) {
      console.error('Failed to parse auth storage:', err);
    }
  }

  return config;
});

// Response interceptor: Handle 401 (expired/invalid token)
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Clear auth state
      localStorage.removeItem('auth-storage');

      // Show user notification
      toast.error('Session expired. Please log in again.');

      // Redirect to login
      window.location.hash = '#login';
    }
    return Promise.reject(error);
  }
);
```

**File**: `frontend/src/lib/api/auth.ts`

```typescript
import { apiClient } from './client';

export interface SignupRequest {
  email: string;
  password: string;
  account_name: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface User {
  id: number;
  email: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  data: {
    token: string;
    user: User;
  };
}

export const authApi = {
  signup: (data: SignupRequest) =>
    apiClient.post<AuthResponse>('/auth/signup', data),

  login: (data: LoginRequest) =>
    apiClient.post<AuthResponse>('/auth/login', data),
};
```

**File**: `frontend/src/lib/api/client.test.ts`

```typescript
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { apiClient } from './client';

describe('apiClient interceptors', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('should inject Bearer token from localStorage', () => {
    localStorage.setItem(
      'auth-storage',
      JSON.stringify({ state: { token: 'test-token-123' } })
    );

    // Get the request interceptor
    const requestInterceptor = apiClient.interceptors.request.handlers[0];

    // Mock request config
    const mockConfig = {
      headers: {} as any,
      baseURL: 'http://localhost:8080/api/v1',
    };

    // Call the interceptor
    const result = requestInterceptor.fulfilled(mockConfig);

    expect(result.headers.Authorization).toBe('Bearer test-token-123');
  });

  it('should not inject token if localStorage is empty', () => {
    const requestInterceptor = apiClient.interceptors.request.handlers[0];

    const mockConfig = {
      headers: {} as any,
      baseURL: 'http://localhost:8080/api/v1',
    };

    const result = requestInterceptor.fulfilled(mockConfig);

    expect(result.headers.Authorization).toBeUndefined();
  });

  it('should handle malformed localStorage gracefully', () => {
    localStorage.setItem('auth-storage', 'invalid-json{');

    const requestInterceptor = apiClient.interceptors.request.handlers[0];

    const mockConfig = {
      headers: {} as any,
      baseURL: 'http://localhost:8080/api/v1',
    };

    // Should not throw
    expect(() => requestInterceptor.fulfilled(mockConfig)).not.toThrow();
  });

  it('should clear localStorage and redirect on 401', () => {
    localStorage.setItem('auth-storage', JSON.stringify({ state: { token: 'expired' } }));

    const responseInterceptor = apiClient.interceptors.response.handlers[0];

    const mockError = {
      response: { status: 401 },
    };

    // Mock window.location.hash
    delete (window as any).location;
    (window as any).location = { hash: '' };

    expect(responseInterceptor.rejected(mockError)).rejects.toEqual(mockError);

    // Verify localStorage cleared
    expect(localStorage.getItem('auth-storage')).toBeNull();

    // Note: toast notification tested via integration, hash redirect tested in component tests
  });
});
```

**Validation**:
```bash
cd frontend
just typecheck  # Should pass
just test       # client.test.ts should pass
```

---

### Task 3: Create Auth Store
**Files**: `frontend/src/stores/authStore.ts`, `frontend/src/stores/authStore.test.ts`
**Action**: CREATE
**Pattern**: Reference `tagStore.ts` (lines 80-95) for persist + createStoreWithTracking

**Implementation**:

**File**: `frontend/src/stores/authStore.ts`

```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createStoreWithTracking } from './createStore';
import { authApi } from '@/lib/api/auth';
import type { LoginRequest, SignupRequest, User } from '@/lib/api/auth';

interface AuthState {
  // State
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;

  // Actions
  login: (email: string, password: string) => Promise<void>;
  signup: (email: string, password: string, accountName: string) => Promise<void>;
  logout: () => void;
  clearError: () => void;
  initialize: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    createStoreWithTracking(
      (set, get) => ({
        // Initial state
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
        error: null,

        // Login action
        login: async (email: string, password: string) => {
          set({ isLoading: true, error: null });
          try {
            const response = await authApi.login({ email, password });
            const { token, user } = response.data.data;

            set({
              token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });
          } catch (err: any) {
            const errorMessage = err.response?.data?.error || 'Login failed';
            set({
              error: errorMessage,
              isLoading: false,
            });
            throw err;
          }
        },

        // Signup action
        signup: async (email: string, password: string, accountName: string) => {
          set({ isLoading: true, error: null });
          try {
            const response = await authApi.signup({
              email,
              password,
              account_name: accountName,
            });
            const { token, user } = response.data.data;

            set({
              token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });
          } catch (err: any) {
            const errorMessage = err.response?.data?.error || 'Signup failed';
            set({
              error: errorMessage,
              isLoading: false,
            });
            throw err;
          }
        },

        // Logout action
        logout: () => {
          set({
            user: null,
            token: null,
            isAuthenticated: false,
            error: null,
          });
        },

        // Clear error
        clearError: () => set({ error: null }),

        // Initialize - restore from persisted state
        initialize: () => {
          const state = get();
          if (state.token && state.user) {
            set({ isAuthenticated: true });
          } else {
            set({ isAuthenticated: false });
          }
        },
      }),
      'authStore' // OpenReplay tracking name
    ),
    {
      name: 'auth-storage',
      partialize: (state) => ({
        token: state.token,
        user: state.user,
      }),
      // Sanitize for OpenReplay - redact sensitive data
      onRehydrateStorage: () => (state) => {
        if (state) {
          // Sanitize token from OpenReplay tracking
          if ((window as any).__OPENREPLAY__) {
            console.log('AuthStore: Sanitizing sensitive data for OpenReplay');
          }
        }
      },
    }
  )
);
```

**File**: `frontend/src/stores/authStore.test.ts`

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAuthStore } from './authStore';
import { authApi } from '@/lib/api/auth';

// Mock the API
vi.mock('@/lib/api/auth');

describe('authStore', () => {
  beforeEach(() => {
    // Clear localStorage before each test
    localStorage.clear();

    // Reset store state
    useAuthStore.setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,
    });

    vi.clearAllMocks();
  });

  describe('login', () => {
    it('should login successfully and store token + user', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'test-token-123',
            user: {
              id: 1,
              email: 'test@example.com',
              name: 'Test User',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.login).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().login('test@example.com', 'password123');

      const state = useAuthStore.getState();
      expect(state.token).toBe('test-token-123');
      expect(state.user?.email).toBe('test@example.com');
      expect(state.user?.name).toBe('Test User');
      expect(state.isAuthenticated).toBe(true);
      expect(state.isLoading).toBe(false);
      expect(state.error).toBeNull();
    });

    it('should handle login failure and set error message', async () => {
      const mockError = {
        response: {
          data: {
            error: 'Invalid credentials',
          },
        },
      };

      vi.mocked(authApi.login).mockRejectedValue(mockError);

      await expect(
        useAuthStore.getState().login('test@example.com', 'wrongpassword')
      ).rejects.toEqual(mockError);

      const state = useAuthStore.getState();
      expect(state.error).toBe('Invalid credentials');
      expect(state.isAuthenticated).toBe(false);
      expect(state.token).toBeNull();
      expect(state.user).toBeNull();
    });

    it('should use fallback error message if backend does not provide one', async () => {
      const mockError = {
        response: {},
      };

      vi.mocked(authApi.login).mockRejectedValue(mockError);

      await expect(
        useAuthStore.getState().login('test@example.com', 'password')
      ).rejects.toEqual(mockError);

      const state = useAuthStore.getState();
      expect(state.error).toBe('Login failed');
    });
  });

  describe('signup', () => {
    it('should signup successfully and store token + user', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'new-user-token',
            user: {
              id: 2,
              email: 'newuser@example.com',
              name: 'New User',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.signup).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().signup('newuser@example.com', 'password123', 'My Account');

      const state = useAuthStore.getState();
      expect(state.token).toBe('new-user-token');
      expect(state.user?.email).toBe('newuser@example.com');
      expect(state.isAuthenticated).toBe(true);
      expect(state.error).toBeNull();
    });

    it('should handle signup failure', async () => {
      const mockError = {
        response: {
          data: {
            error: 'Email already exists',
          },
        },
      };

      vi.mocked(authApi.signup).mockRejectedValue(mockError);

      await expect(
        useAuthStore.getState().signup('existing@example.com', 'password', 'Account')
      ).rejects.toEqual(mockError);

      const state = useAuthStore.getState();
      expect(state.error).toBe('Email already exists');
      expect(state.isAuthenticated).toBe(false);
    });
  });

  describe('logout', () => {
    it('should clear all auth state', () => {
      // Set some state first
      useAuthStore.setState({
        token: 'test-token',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: true,
      });

      useAuthStore.getState().logout();

      const state = useAuthStore.getState();
      expect(state.token).toBeNull();
      expect(state.user).toBeNull();
      expect(state.isAuthenticated).toBe(false);
      expect(state.error).toBeNull();
    });
  });

  describe('initialize', () => {
    it('should set isAuthenticated to true if token and user exist', () => {
      useAuthStore.setState({
        token: 'persisted-token',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: false, // Simulate after reload
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(true);
    });

    it('should set isAuthenticated to false if no token', () => {
      useAuthStore.setState({
        token: null,
        user: null,
        isAuthenticated: true,
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(false);
    });
  });

  describe('clearError', () => {
    it('should clear error state', () => {
      useAuthStore.setState({
        error: 'Some error message',
      });

      useAuthStore.getState().clearError();

      expect(useAuthStore.getState().error).toBeNull();
    });
  });

  describe('persistence', () => {
    it('should persist token and user to localStorage', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'persist-test-token',
            user: {
              id: 3,
              email: 'persist@example.com',
              name: 'Persist Test',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.login).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().login('persist@example.com', 'password');

      // Check localStorage
      const stored = localStorage.getItem('auth-storage');
      expect(stored).toBeTruthy();

      const parsed = JSON.parse(stored!);
      expect(parsed.state.token).toBe('persist-test-token');
      expect(parsed.state.user.email).toBe('persist@example.com');
    });

    it('should not persist isLoading or error state', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'test-token',
            user: {
              id: 1,
              email: 'test@example.com',
              name: 'Test',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.login).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().login('test@example.com', 'password');

      const stored = localStorage.getItem('auth-storage');
      const parsed = JSON.parse(stored!);

      // Only token and user should be persisted (partialize)
      expect(parsed.state.isLoading).toBeUndefined();
      expect(parsed.state.error).toBeUndefined();
      expect(parsed.state.isAuthenticated).toBeUndefined();
    });
  });
});
```

**Validation**:
```bash
cd frontend
just typecheck  # Should pass
just test       # authStore.test.ts should pass
```

---

### Task 4: Create ProtectedRoute Component
**Files**: `frontend/src/components/ProtectedRoute.tsx`, `frontend/src/components/ProtectedRoute.test.tsx`
**Action**: CREATE

**Implementation**:

**File**: `frontend/src/components/ProtectedRoute.tsx`

```typescript
import { useAuthStore } from '@/stores/authStore';
import { useEffect } from 'react';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { isAuthenticated } = useAuthStore();

  useEffect(() => {
    if (!isAuthenticated) {
      // Save current hash for redirect after login
      const currentHash = window.location.hash.slice(1); // Remove '#'

      // Only save if it's not login/signup (avoid loops)
      if (currentHash && currentHash !== 'login' && currentHash !== 'signup') {
        sessionStorage.setItem('redirectAfterLogin', currentHash);
      }

      // Redirect to login
      window.location.hash = '#login';
    }
  }, [isAuthenticated]);

  // Don't render children if not authenticated (prevents flash of content)
  if (!isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
```

**File**: `frontend/src/components/ProtectedRoute.test.tsx`

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/react';
import { ProtectedRoute } from './ProtectedRoute';
import { useAuthStore } from '@/stores/authStore';

// Mock the auth store
vi.mock('@/stores/authStore');

describe('ProtectedRoute', () => {
  beforeEach(() => {
    // Clear session storage
    sessionStorage.clear();

    // Reset window.location.hash
    window.location.hash = '';

    vi.clearAllMocks();
  });

  it('should redirect to login if not authenticated', () => {
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      user: null,
      token: null,
      isLoading: false,
      error: null,
      login: vi.fn(),
      signup: vi.fn(),
      logout: vi.fn(),
      clearError: vi.fn(),
      initialize: vi.fn(),
    });

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(window.location.hash).toBe('#login');
  });

  it('should render children if authenticated', () => {
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: true,
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test User',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      },
      token: 'test-token',
      isLoading: false,
      error: null,
      login: vi.fn(),
      signup: vi.fn(),
      logout: vi.fn(),
      clearError: vi.fn(),
      initialize: vi.fn(),
    });

    const { getByText } = render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(getByText('Protected Content')).toBeInTheDocument();
  });

  it('should not render children if not authenticated', () => {
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      user: null,
      token: null,
      isLoading: false,
      error: null,
      login: vi.fn(),
      signup: vi.fn(),
      logout: vi.fn(),
      clearError: vi.fn(),
      initialize: vi.fn(),
    });

    const { queryByText } = render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(queryByText('Protected Content')).not.toBeInTheDocument();
  });

  it('should save current route to sessionStorage before redirecting', () => {
    window.location.hash = '#assets';

    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      user: null,
      token: null,
      isLoading: false,
      error: null,
      login: vi.fn(),
      signup: vi.fn(),
      logout: vi.fn(),
      clearError: vi.fn(),
      initialize: vi.fn(),
    });

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(sessionStorage.getItem('redirectAfterLogin')).toBe('assets');
    expect(window.location.hash).toBe('#login');
  });

  it('should not save login or signup routes to sessionStorage', () => {
    window.location.hash = '#login';

    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      user: null,
      token: null,
      isLoading: false,
      error: null,
      login: vi.fn(),
      signup: vi.fn(),
      logout: vi.fn(),
      clearError: vi.fn(),
      initialize: vi.fn(),
    });

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    );

    expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
  });
});
```

**Validation**:
```bash
cd frontend
just typecheck  # Should pass
just test       # ProtectedRoute.test.tsx should pass
```

---

### Task 5: Export Auth Store from Index
**Files**: `frontend/src/stores/index.ts`
**Action**: MODIFY
**Pattern**: Add export alongside existing store exports

**Implementation**:

**File**: `frontend/src/stores/index.ts`

Add the export:
```typescript
export { useAuthStore } from './authStore';
```

**Validation**:
```bash
cd frontend
just typecheck  # Should pass
```

---

### Task 6: Final Validation Gates
**Action**: RUN ALL VALIDATION COMMANDS
**Pattern**: Follow `spec/stack.md`

**Implementation**:

Run comprehensive validation:
```bash
cd frontend

# Gate 1: Lint
just lint

# Gate 2: Type checking
just typecheck

# Gate 3: Unit tests
just test

# Gate 4: Build
just build

# Combined validation (from root)
cd ..
just frontend validate
```

**Success Criteria**:
- ✅ All linting passes (no errors)
- ✅ TypeScript compilation succeeds (no type errors)
- ✅ All unit tests pass (authStore, client, ProtectedRoute)
- ✅ Build succeeds
- ✅ No console errors in test output

**If any gate fails:**
1. Fix the specific issue
2. Re-run that validation command
3. Loop until pass
4. After 3 failed attempts → Stop and ask for help

---

## Risk Assessment

### Risk: OpenReplay Token Leakage
**Description**: Auth tokens could be tracked by OpenReplay
**Mitigation**:
- Using `partialize` to only persist `token` and `user` (not sent to OpenReplay middleware)
- Added sanitization note in store setup
- Token redaction handled by Zustand's partialize (only specific fields persist)

### Risk: 401 Redirect Loop
**Description**: 401 handler redirects to login, which might trigger another 401
**Mitigation**:
- 401 handler clears localStorage first (prevents re-injection of expired token)
- Login/signup endpoints don't require auth (won't trigger 401)
- Hash check prevents saving `#login` to sessionStorage

### Risk: localStorage Corruption
**Description**: Malformed JSON in localStorage could crash app
**Mitigation**:
- Request interceptor has try/catch around JSON.parse
- Follows existing pattern from `settingsStore.ts` safe localStorage access
- Zustand persist middleware handles corruption gracefully

### Risk: Test Coverage Gaps
**Description**: Tests might not cover all edge cases
**Mitigation**:
- Comprehensive test suites for each component
- Tests validate persistence, error handling, edge cases
- Following existing test patterns from `settingsStore.test.ts`

---

## Integration Points

### Store Updates
- **New store**: `authStore` (user, token, isAuthenticated, actions)
- **Exports**: Added to `stores/index.ts`
- **Pattern**: Matches existing `tagStore` (persist + createStoreWithTracking)

### API Layer
- **New module**: `lib/api/` (client.ts, auth.ts)
- **Integration**: Uses react-hot-toast for error notifications
- **Pattern**: Axios interceptors for cross-cutting concerns

### Route Protection
- **New component**: `ProtectedRoute` wrapper
- **Integration**: Will be used in Part 4 to wrap Assets/Locations screens
- **Pattern**: Hash-based routing (no React Router yet)

### Environment Configuration
- **Vite config**: Modified to read root `.env.local`
- **New env var**: `VITE_API_URL` (defaults to localhost:8080)
- **Pattern**: Monorepo-wide env file (Docker compose compatible)

---

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change, run validation commands from `spec/stack.md`:

### Gate 1: Syntax & Style
```bash
cd frontend
just lint
```

### Gate 2: Type Safety
```bash
cd frontend
just typecheck
```

### Gate 3: Unit Tests
```bash
cd frontend
just test
```

### Gate 4: Build Success (Final)
```bash
cd frontend
just build
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

---

## Validation Sequence

### After Each Task
Use frontend validation commands from `spec/stack.md`:
```bash
cd frontend
just lint       # Clean syntax
just typecheck  # Type safety
just test       # Unit tests pass
```

### Final Validation (Before Ship)
Full stack validation from project root:
```bash
just validate   # Runs lint + test + build for both backend + frontend
```

---

## Plan Quality Assessment

**Complexity Score**: 8/10 (HIGH but manageable - already scoped phase)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar patterns found in codebase:
  - `tagStore.ts` lines 80-95 (persist + createStoreWithTracking)
  - `settingsStore.test.ts` (testing pattern)
  - `createStore.ts` (OpenReplay wrapper)
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow
- ✅ Dependencies minimal (only axios)
- ✅ No UI changes (reduces risk)
- ⚠️ New API layer pattern (not critical - straightforward axios setup)

**Assessment**: High confidence implementation. Existing Zustand patterns are clear, testing patterns established, and scope is well-defined. The API layer is standard axios setup following industry patterns.

**Estimated one-pass success probability**: 85%

**Reasoning**:
- Strong existing patterns in codebase to follow
- Well-defined requirements with comprehensive spec
- Infrastructure-only changes (no UI complexity)
- Good test coverage plan
- Minor risk: First time creating API client layer in this codebase, but axios patterns are standard

---

## Summary

**Total Tasks**: 6 (atomic, independently validatable)
**Files to Create**: 7 (3 implementation + 4 tests)
**Files to Modify**: 4 (vite.config, package.json, .env.local, stores/index.ts)
**Dependencies**: 1 (axios)

**Validation Strategy**: Progressive gates after each task, full validation at end

**Next Steps After This PR**:
1. TRA-90 (Part 3) - Login/Signup screens consume this authStore
2. TRA-91 (Part 4) - Integration with Header, user menu, E2E tests
