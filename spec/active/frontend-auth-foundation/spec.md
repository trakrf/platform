# Feature: Frontend Auth - Foundation (Store & API Client)

## Origin
This specification is Part 2 of the larger `frontend-auth-hybrid` implementation. It emerged from the need to build authentication infrastructure before adding UI components.

**Parent Spec**: `spec/active/frontend-auth-hybrid/spec.md`
**Linear Issue**: TRA-89

## Outcome
Core authentication infrastructure (Zustand store, API client, ProtectedRoute component) that enables Parts 3 & 4 to build login/signup UI and integration. No visible UI changes - purely foundational.

## User Story
**As a frontend developer**
I want a complete authentication infrastructure (store, API client, route protection)
So that I can build login/signup screens and protected routes without worrying about state management or API communication

## Context

### Discovery
- Backend has complete JWT authentication (signup, login, protected routes)
- Frontend has NO authentication yet
- Need foundation before building UI (Part 3) or integration (Part 4)
- Using Zustand for state (already used for device stores)
- Using Axios for API calls (or need to add it)

### Current State
**Frontend:**
- No auth state management
- No API client with token injection
- No route protection mechanism
- All screens are public

**Backend (Already Complete):**
- ✅ `POST /api/v1/auth/signup` - Working
- ✅ `POST /api/v1/auth/login` - Working
- ✅ JWT middleware - Working
- ✅ Protected routes - Working

### Desired State
**After this PR:**
- ✅ Auth state persisted in localStorage
- ✅ API client auto-injects Bearer tokens
- ✅ 401 responses auto-clear state and redirect
- ✅ ProtectedRoute component ready for use
- ✅ Unit tests covering all auth logic
- ❌ NO UI changes (that's Part 3)
- ❌ NO integration with existing screens (that's Part 4)

## Technical Requirements

### 1. Auth State Management (Zustand Store)

**File**: `frontend/src/stores/authStore.ts`

```typescript
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { authApi } from '@/lib/api/auth'
import type { LoginRequest, SignupRequest } from '@/lib/api/auth'

interface User {
  id: number
  email: string
  name: string
  created_at: string
  updated_at: string
}

interface AuthState {
  // State
  user: User | null
  token: string | null
  isAuthenticated: boolean
  isLoading: boolean
  error: string | null

  // Actions
  login: (email: string, password: string) => Promise<void>
  signup: (email: string, password: string, accountName: string) => Promise<void>
  logout: () => void
  clearError: () => void
  initialize: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      // Initial state
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,

      // Login action
      login: async (email: string, password: string) => {
        set({ isLoading: true, error: null })
        try {
          const response = await authApi.login({ email, password })
          const { token, user } = response.data.data
          set({
            token,
            user,
            isAuthenticated: true,
            isLoading: false
          })
        } catch (err: any) {
          const errorMessage = err.response?.data?.error || 'Login failed'
          set({
            error: errorMessage,
            isLoading: false
          })
          throw err
        }
      },

      // Signup action
      signup: async (email: string, password: string, accountName: string) => {
        set({ isLoading: true, error: null })
        try {
          const response = await authApi.signup({
            email,
            password,
            account_name: accountName
          })
          const { token, user } = response.data.data
          set({
            token,
            user,
            isAuthenticated: true,
            isLoading: false
          })
        } catch (err: any) {
          const errorMessage = err.response?.data?.error || 'Signup failed'
          set({
            error: errorMessage,
            isLoading: false
          })
          throw err
        }
      },

      // Logout action
      logout: () => {
        set({
          user: null,
          token: null,
          isAuthenticated: false,
          error: null
        })
      },

      // Clear error
      clearError: () => set({ error: null }),

      // Initialize - validate token on app load
      initialize: () => {
        const state = get()
        if (state.token && state.user) {
          set({ isAuthenticated: true })
        } else {
          set({ isAuthenticated: false })
        }
      }
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({
        token: state.token,
        user: state.user
      })
    }
  )
)
```

**Requirements:**
- ✅ Persist `token` and `user` to localStorage
- ✅ Initialize on app load (restore from localStorage)
- ✅ Clear state on logout
- ✅ Handle loading and error states
- ✅ Type-safe with TypeScript
- ✅ Throw errors on login/signup failure (let caller handle)

### 2. API Client Setup

**File**: `frontend/src/lib/api/client.ts`

```typescript
import axios from 'axios'

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1'

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json'
  }
})

// Request interceptor: Inject Bearer token
apiClient.interceptors.request.use((config) => {
  const authStorage = localStorage.getItem('auth-storage')

  if (authStorage) {
    try {
      const { state } = JSON.parse(authStorage)
      if (state?.token) {
        config.headers.Authorization = `Bearer ${state.token}`
      }
    } catch (err) {
      console.error('Failed to parse auth storage:', err)
    }
  }

  return config
})

// Response interceptor: Handle 401 (expired token)
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Clear auth state
      localStorage.removeItem('auth-storage')
      // Redirect to login
      window.location.hash = '#login'
    }
    return Promise.reject(error)
  }
)
```

**File**: `frontend/src/lib/api/auth.ts`

```typescript
import { apiClient } from './client'

export interface SignupRequest {
  email: string
  password: string
  account_name: string
}

export interface LoginRequest {
  email: string
  password: string
}

export interface User {
  id: number
  email: string
  name: string
  created_at: string
  updated_at: string
}

export interface AuthResponse {
  data: {
    token: string
    user: User
  }
}

export const authApi = {
  signup: (data: SignupRequest) =>
    apiClient.post<AuthResponse>('/auth/signup', data),

  login: (data: LoginRequest) =>
    apiClient.post<AuthResponse>('/auth/login', data)
}
```

**Requirements:**
- ✅ Axios instance with base URL from `VITE_API_URL` env var
- ✅ Request interceptor injects `Bearer {token}` header
- ✅ Response interceptor handles 401 (clear state, redirect to login)
- ✅ Type-safe API methods with TypeScript interfaces
- ✅ Error handling preserves backend error messages

**Environment Setup:**
```bash
# Add to frontend/.env.local
VITE_API_URL=http://localhost:8080/api/v1
```

### 3. Protected Route Wrapper

**File**: `frontend/src/components/ProtectedRoute.tsx`

```typescript
import { useAuthStore } from '@/stores/authStore'
import { useEffect } from 'react'

interface ProtectedRouteProps {
  children: React.ReactNode
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { isAuthenticated } = useAuthStore()

  useEffect(() => {
    if (!isAuthenticated) {
      // Save current hash for redirect after login
      const currentHash = window.location.hash.slice(1) // Remove '#'
      if (currentHash && currentHash !== 'login' && currentHash !== 'signup') {
        sessionStorage.setItem('redirectAfterLogin', currentHash)
      }
      // Redirect to login
      window.location.hash = '#login'
    }
  }, [isAuthenticated])

  // Don't render children if not authenticated
  if (!isAuthenticated) {
    return null
  }

  return <>{children}</>
}
```

**Requirements:**
- ✅ Check `isAuthenticated` from auth store
- ✅ If not authenticated → Save current route to sessionStorage, redirect to login
- ✅ If authenticated → Render children
- ✅ Redirect happens in useEffect (avoids flash of content)

### 4. Package Dependencies

**Add to `frontend/package.json`:**
```json
{
  "dependencies": {
    "axios": "^1.6.0",
    "zustand": "^4.4.0" // Already installed
  }
}
```

**Install:**
```bash
cd frontend
pnpm add axios
```

## Testing Strategy

### Unit Tests

**File**: `frontend/src/stores/authStore.test.ts`

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useAuthStore } from './authStore'
import { authApi } from '@/lib/api/auth'

// Mock the API
vi.mock('@/lib/api/auth')

describe('authStore', () => {
  beforeEach(() => {
    // Clear store state before each test
    useAuthStore.setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null
    })
    vi.clearAllMocks()
  })

  it('should login successfully', async () => {
    const mockResponse = {
      data: {
        data: {
          token: 'test-token',
          user: { id: 1, email: 'test@example.com', name: 'Test User' }
        }
      }
    }
    vi.mocked(authApi.login).mockResolvedValue(mockResponse)

    await useAuthStore.getState().login('test@example.com', 'password')

    const state = useAuthStore.getState()
    expect(state.token).toBe('test-token')
    expect(state.user?.email).toBe('test@example.com')
    expect(state.isAuthenticated).toBe(true)
    expect(state.error).toBeNull()
  })

  it('should handle login failure', async () => {
    const mockError = {
      response: { data: { error: 'Invalid credentials' } }
    }
    vi.mocked(authApi.login).mockRejectedValue(mockError)

    await expect(
      useAuthStore.getState().login('test@example.com', 'wrong')
    ).rejects.toThrow()

    const state = useAuthStore.getState()
    expect(state.error).toBe('Invalid credentials')
    expect(state.isAuthenticated).toBe(false)
  })

  it('should logout', () => {
    useAuthStore.setState({
      token: 'test-token',
      user: { id: 1, email: 'test@example.com', name: 'Test' },
      isAuthenticated: true
    })

    useAuthStore.getState().logout()

    const state = useAuthStore.getState()
    expect(state.token).toBeNull()
    expect(state.user).toBeNull()
    expect(state.isAuthenticated).toBe(false)
  })

  it('should initialize from persisted state', () => {
    useAuthStore.setState({
      token: 'persisted-token',
      user: { id: 1, email: 'test@example.com', name: 'Test' },
      isAuthenticated: false // Simulate after reload
    })

    useAuthStore.getState().initialize()

    expect(useAuthStore.getState().isAuthenticated).toBe(true)
  })
})
```

**File**: `frontend/src/lib/api/client.test.ts`

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { apiClient } from './client'

describe('apiClient interceptors', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('should inject Bearer token from localStorage', async () => {
    localStorage.setItem(
      'auth-storage',
      JSON.stringify({ state: { token: 'test-token' } })
    )

    // Mock a request
    const mockRequest = { headers: {} }
    const interceptor = apiClient.interceptors.request.handlers[0]
    const result = interceptor.fulfilled(mockRequest)

    expect(result.headers.Authorization).toBe('Bearer test-token')
  })

  it('should handle 401 response', async () => {
    const mockError = {
      response: { status: 401 }
    }

    const interceptor = apiClient.interceptors.response.handlers[0]

    expect(() => {
      interceptor.rejected(mockError)
    }).rejects.toEqual(mockError)

    // Verify localStorage cleared
    expect(localStorage.getItem('auth-storage')).toBeNull()
  })
})
```

**File**: `frontend/src/components/ProtectedRoute.test.tsx`

```typescript
import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { ProtectedRoute } from './ProtectedRoute'
import { useAuthStore } from '@/stores/authStore'

vi.mock('@/stores/authStore')

describe('ProtectedRoute', () => {
  it('should redirect to login if not authenticated', () => {
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      // ... other store values
    })

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    )

    expect(window.location.hash).toBe('#login')
  })

  it('should render children if authenticated', () => {
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: true,
      // ... other store values
    })

    const { getByText } = render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    )

    expect(getByText('Protected Content')).toBeInTheDocument()
  })

  it('should save current route to sessionStorage', () => {
    window.location.hash = '#assets'
    vi.mocked(useAuthStore).mockReturnValue({
      isAuthenticated: false,
      // ... other store values
    })

    render(
      <ProtectedRoute>
        <div>Protected Content</div>
      </ProtectedRoute>
    )

    expect(sessionStorage.getItem('redirectAfterLogin')).toBe('assets')
  })
})
```

### Test Commands

```bash
# Run unit tests
cd frontend
pnpm test

# Run specific test file
pnpm test authStore.test.ts

# Run with coverage
pnpm test --coverage
```

## Validation Criteria

### Must Have
- [ ] authStore created with all required state and actions
- [ ] login() stores token and user, sets isAuthenticated
- [ ] signup() stores token and user, sets isAuthenticated
- [ ] logout() clears all auth state
- [ ] initialize() restores state from localStorage
- [ ] API client injects Bearer token on all requests
- [ ] 401 responses clear auth state and redirect to login
- [ ] ProtectedRoute redirects unauthenticated users to login
- [ ] ProtectedRoute saves intended destination to sessionStorage
- [ ] All unit tests pass
- [ ] Token persists across page reload

### Should Have
- [ ] Error messages from backend preserved in store
- [ ] Loading states work correctly
- [ ] clearError() resets error state

### Nice to Have
- [ ] Test coverage > 80%
- [ ] TypeScript strict mode passes
- [ ] No console errors during tests

## Edge Cases & Constraints

### Edge Cases
1. **Malformed localStorage**: If auth-storage is corrupted, interceptor should handle gracefully
2. **Token expired mid-session**: 401 interceptor should clear state and redirect
3. **Multiple tabs**: Token changes in one tab should affect others (Zustand persistence handles this)
4. **API down**: Errors should be captured and displayed (not crash app)

### Constraints
- **No React Router yet**: Keep using hash-based routing (`window.location.hash`)
- **Backend API base URL**: Configurable via `VITE_API_URL` env var
- **Token storage**: localStorage (not secure for sensitive data, but acceptable for MVP)
- **No token refresh**: Backend JWT has 1-hour expiration, no refresh token yet

## Dependencies

**Required:**
- ✅ Backend auth API (TRA-79) - ALREADY COMPLETE
- ✅ Zustand already installed in frontend

**Blocked by:**
- ❌ None - can start immediately

**Blocks:**
- ⚠️ TRA-90 (Login/Signup Screens) - needs authStore and authApi
- ⚠️ TRA-91 (Integration) - needs everything from this PR

## Success Metrics

After this PR is merged:
1. ✅ `pnpm test` passes with new auth store tests
2. ✅ API client automatically injects tokens
3. ✅ 401 responses trigger logout and redirect
4. ✅ ProtectedRoute component ready for use in Part 3
5. ✅ No visible UI changes (foundation only)
6. ✅ Type checking passes: `pnpm typecheck`

## Next Steps

**After this PR:**
1. TRA-90 (Part 3) will create LoginScreen and SignupScreen using this authStore
2. TRA-91 (Part 4) will wire ProtectedRoute to Assets/Locations and add user menu

## References

- Parent spec: `spec/active/frontend-auth-hybrid/spec.md`
- Linear issue: TRA-89
- Backend auth implementation: `backend/internal/handlers/auth/`
- Existing Zustand stores: `frontend/src/stores/`
