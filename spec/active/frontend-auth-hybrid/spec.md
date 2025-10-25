# Feature: Frontend Authentication - Hybrid Mode

## Origin
This specification emerged from the need to add authenticated platform features (Assets, Locations) while keeping existing RFID device features (Inventory, Locate, Barcode) publicly accessible without login requirements.

## Outcome
Frontend supports both authenticated and unauthenticated modes, with seamless navigation between public device features and protected platform features.

## User Story
**As a user**
I want to use RFID device features without logging in, but access asset management features when authenticated
So that I can quickly scan tags while also managing my asset inventory when needed

## Context

### Discovery
- Frontend is currently a standalone RFID device app with **NO authentication**
- Backend has **complete authentication** (signup, login, JWT, protected routes)
- **NO integration** between frontend and backend yet
- Cofounder is building Asset CRUD screens separately
- Need auth scaffolding + navigation stubs to unblock parallel work

### Current State
**Frontend:**
- 6 public screens: Home, Inventory, Locate, Barcode, Settings, Help
- Hash-based navigation (`#home`, `#inventory`, etc.)
- No routing library (React Router)
- No auth state management
- No API integration
- Zustand stores for device state only

**Backend:**
- ✅ `POST /api/v1/auth/signup` - Working
- ✅ `POST /api/v1/auth/login` - Working
- ✅ JWT middleware - Working
- ✅ Protected routes - Working
- ✅ Multi-tenant accounts - Ready

### Desired State
**Public Features** (No login required):
- Home screen
- Inventory screen (RFID tag scanning)
- Locate screen (find tags)
- Barcode screen
- Settings screen
- Help screen

**Protected Features** (Require authentication):
- **Assets screen** (stub page, cofounder building CRUD)
- **Locations screen** (stub page, cofounder building CRUD)
- Future: More entity CRUD screens

**Navigation Behavior:**
- Clicking Assets/Locations when **not logged in** → Redirect to Login
- After login → Redirect to originally requested page
- Login/Signup screens accessible even when logged in (redirect to Home)
- User menu in header shows avatar + logout option

**Future Integration** (Out of scope now):
- When authenticated, Inventory screen shows asset associations
- When authenticated, Locate screen shows asset associations
- Backend API calls for asset/location data (cofounder handles this)

## Technical Requirements

### 1. Auth State Management (Zustand Store)

**File**: `frontend/src/stores/authStore.ts`

```typescript
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface User {
  id: number
  email: string
  name: string
  created_at: string
  updated_at: string
}

interface AuthState {
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
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,

      login: async (email, password) => {
        // Call backend API, store token + user
      },

      signup: async (email, password, accountName) => {
        // Call backend API, store token + user
      },

      logout: () => {
        set({ user: null, token: null, isAuthenticated: false })
      },

      clearError: () => set({ error: null }),

      initialize: () => {
        // Load token from localStorage, validate if exists
      }
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({ token: state.token, user: state.user })
    }
  )
)
```

**Requirements:**
- Persist token and user to localStorage
- Initialize on app load
- Clear state on logout
- Handle loading and error states
- Type-safe with TypeScript

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

// Request interceptor: Inject token
apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('auth-storage')
    ? JSON.parse(localStorage.getItem('auth-storage')!).state.token
    : null

  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }

  return config
})

// Response interceptor: Handle 401 (token expired)
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Clear auth state, redirect to login
      localStorage.removeItem('auth-storage')
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

export interface AuthResponse {
  data: {
    token: string
    user: {
      id: number
      email: string
      name: string
      created_at: string
      updated_at: string
    }
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
- Axios instance with base URL from env var
- Request interceptor injects Bearer token
- Response interceptor handles 401 (expired token)
- Type-safe API methods
- Error handling with proper types

### 3. Auth UI Components

**File**: `frontend/src/components/LoginScreen.tsx`

**Requirements:**
- Email + password form
- Submit button with loading state
- Error message display
- "Don't have an account? Sign up" link
- Redirect to originally requested page after login
- Validation: email format, password not empty

**File**: `frontend/src/components/SignupScreen.tsx`

**Requirements:**
- Email + password + account_name form
- Submit button with loading state
- Error message display
- "Already have an account? Log in" link
- Password requirements: min 8 characters
- Account name requirements: min 2 characters
- Redirect to Home after signup

**Design**:
- Use existing Tailwind classes for consistency
- Match existing app design (dark theme, similar layout to Settings screen)
- Center card layout on empty background
- Form validation with inline error messages

### 4. Protected Route Wrapper

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
      const currentHash = window.location.hash.slice(1)
      if (currentHash && currentHash !== 'login' && currentHash !== 'signup') {
        sessionStorage.setItem('redirectAfterLogin', currentHash)
      }
      window.location.hash = '#login'
    }
  }, [isAuthenticated])

  if (!isAuthenticated) {
    return null // Or loading spinner
  }

  return <>{children}</>
}
```

**Requirements:**
- Check `isAuthenticated` from auth store
- If not authenticated → Save current route, redirect to login
- If authenticated → Render children
- After login → Redirect to saved route (or Home)

### 5. Stub Pages for Assets and Locations

**File**: `frontend/src/components/AssetsScreen.tsx`

```typescript
export function AssetsScreen() {
  return (
    <div className="flex flex-col items-center justify-center h-full p-8 text-center">
      <h1 className="text-3xl font-bold mb-4">Assets</h1>
      <p className="text-gray-400 mb-8">
        Asset management CRUD coming soon.
      </p>
      <div className="text-sm text-gray-500">
        This page will show:
        <ul className="list-disc list-inside mt-2">
          <li>Asset list with search and filters</li>
          <li>Create new asset</li>
          <li>Edit asset details</li>
          <li>Delete assets</li>
          <li>Associate assets with locations</li>
        </ul>
      </div>
    </div>
  )
}
```

**File**: `frontend/src/components/LocationsScreen.tsx`

```typescript
export function LocationsScreen() {
  return (
    <div className="flex flex-col items-center justify-center h-full p-8 text-center">
      <h1 className="text-3xl font-bold mb-4">Locations</h1>
      <p className="text-gray-400 mb-8">
        Location management CRUD coming soon.
      </p>
      <div className="text-sm text-gray-500">
        This page will show:
        <ul className="list-disc list-inside mt-2">
          <li>Location list with hierarchy</li>
          <li>Create new location</li>
          <li>Edit location details</li>
          <li>Delete locations</li>
          <li>View assets at location</li>
        </ul>
      </div>
    </div>
  )
}
```

**Requirements:**
- Simple placeholder UI
- Explains what will be built
- Matches app design
- Protected by ProtectedRoute wrapper

### 6. Update Navigation (TabNavigation.tsx)

**Current tabs**:
- Home, Inventory, Locate, Barcode, Settings, Help

**Add new tabs**:
- Assets (protected)
- Locations (protected)

**Tab order**:
- Home
- Inventory
- Locate
- Barcode
- **Assets** (new, protected)
- **Locations** (new, protected)
- Settings
- Help

**Requirements:**
- Add Assets and Locations to tab list
- Use appropriate icons (box/package for Assets, map-pin for Locations)
- Visual indicator for protected tabs? (optional)
- Clicking protected tab when not logged in → Redirect to login

### 7. Update Header Component

**File**: `frontend/src/components/Header.tsx`

**Add**:
- User menu (avatar + email + logout) when authenticated
- "Log In" button when not authenticated
- Position: Top right corner

**Requirements:**
- Show user email from auth store
- Avatar component (initials or icon)
- Dropdown menu with:
  - User email (non-clickable)
  - Logout button
- "Log In" button redirects to `#login`
- Matches existing header design

### 8. Update App.tsx Router

**Current**: Hash-based routing with manual parsing

**Update**:
- Add `#login` route → LoginScreen
- Add `#signup` route → SignupScreen
- Add `#assets` route → ProtectedRoute(AssetsScreen)
- Add `#locations` route → ProtectedRoute(LocationsScreen)
- Initialize auth store on mount
- Handle redirect after login

**Requirements:**
- Keep existing hash-based routing (don't migrate to React Router yet)
- Add route handlers for login, signup, assets, locations
- Initialize auth store: `useAuthStore.getState().initialize()` on mount
- After login success → Redirect to `sessionStorage.getItem('redirectAfterLogin')` or `#home`

## Code Examples

### Auth Store Integration in App.tsx

```typescript
// In App.tsx
useEffect(() => {
  // Initialize auth on app load
  useAuthStore.getState().initialize()
}, [])

const renderScreen = () => {
  switch (tab) {
    case 'home': return <HomeScreen />
    case 'inventory': return <InventoryScreen />
    case 'locate': return <LocateScreen />
    case 'barcode': return <BarcodeScreen />
    case 'settings': return <SettingsScreen />
    case 'help': return <HelpScreen />
    case 'login': return <LoginScreen />
    case 'signup': return <SignupScreen />
    case 'assets': return <ProtectedRoute><AssetsScreen /></ProtectedRoute>
    case 'locations': return <ProtectedRoute><LocationsScreen /></ProtectedRoute>
    default: return <HomeScreen />
  }
}
```

### Login Screen Example

```typescript
export function LoginScreen() {
  const { login, isLoading, error, clearError } = useAuthStore()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    clearError()

    try {
      await login(email, password)

      // Redirect after successful login
      const redirectTo = sessionStorage.getItem('redirectAfterLogin') || 'home'
      sessionStorage.removeItem('redirectAfterLogin')
      window.location.hash = `#${redirectTo}`
    } catch (err) {
      // Error handled by store
    }
  }

  return (
    <div className="flex items-center justify-center min-h-screen bg-gray-900">
      <div className="w-full max-w-md p-8 bg-gray-800 rounded-lg shadow-xl">
        <h1 className="text-3xl font-bold text-center mb-6">Log In</h1>

        {error && (
          <div className="mb-4 p-3 bg-red-900/50 border border-red-500 rounded text-red-200 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className="mb-4">
            <label className="block text-sm font-medium mb-2">Email</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          <div className="mb-6">
            <label className="block text-sm font-medium mb-2">Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          <button
            type="submit"
            disabled={isLoading}
            className="w-full py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 rounded font-medium transition-colors"
          >
            {isLoading ? 'Logging in...' : 'Log In'}
          </button>
        </form>

        <p className="mt-6 text-center text-sm text-gray-400">
          Don't have an account?{' '}
          <a href="#signup" className="text-blue-400 hover:text-blue-300">
            Sign up
          </a>
        </p>
      </div>
    </div>
  )
}
```

## Testing Strategy

### Manual Testing
```bash
# 1. Start full stack
just dev

# 2. Start frontend
cd frontend && just dev

# 3. Test public features (no login required)
- Navigate to Home, Inventory, Locate, Barcode, Settings, Help
- All should work without authentication

# 4. Test protected features redirect
- Click "Assets" tab → Should redirect to login
- Click "Locations" tab → Should redirect to login

# 5. Test signup flow
- Click "Sign Up"
- Fill form: email, password, account name
- Submit → Should create account and redirect to Home
- Check Assets and Locations tabs now work

# 6. Test login flow
- Log out
- Click "Log In"
- Fill form: email, password
- Submit → Should log in and redirect to Home

# 7. Test protected redirect after login
- Log out
- Click "Assets" tab → Redirects to login
- Log in → Should redirect back to Assets tab

# 8. Test logout
- Click user menu in header
- Click "Logout" → Should clear state and return to Home
- Assets/Locations tabs should redirect to login again
```

### Unit Tests
- `authStore.test.ts` - Test login, signup, logout, initialize
- `apiClient.test.ts` - Test token injection, 401 handling
- `ProtectedRoute.test.tsx` - Test redirect logic

### Integration Tests (Playwright)
- `auth-flow.spec.ts` - Full signup → login → logout flow
- `protected-routes.spec.ts` - Test redirect to login when not authenticated

## Validation Criteria

### Must Have
- [ ] Login screen renders and accepts email/password
- [ ] Signup screen renders and accepts email/password/account_name
- [ ] Login success stores token and user in auth store
- [ ] Signup success stores token and user in auth store
- [ ] Protected routes redirect to login when not authenticated
- [ ] Assets and Locations tabs redirect to login when not authenticated
- [ ] After login, redirect to originally requested page works
- [ ] Logout clears auth state and redirects to Home
- [ ] User menu shows in header when authenticated
- [ ] "Log In" button shows in header when not authenticated
- [ ] Token persists across page reloads
- [ ] API client injects Bearer token on requests
- [ ] 401 responses clear auth state and redirect to login

### Should Have
- [ ] Form validation with inline error messages
- [ ] Loading states on buttons
- [ ] Error messages from backend displayed properly
- [ ] Password visibility toggle
- [ ] "Remember me" checkbox (optional)

### Nice to Have
- [ ] Smooth transitions between screens
- [ ] Toast notifications for login/logout
- [ ] Password strength indicator on signup
- [ ] Email format validation before submit

## Edge Cases & Constraints

### Edge Cases
1. **Token expired during session**: 401 interceptor clears state, redirects to login
2. **Page reload while on protected route**: Initialize checks token, redirects if invalid
3. **Navigate to login while already logged in**: Allow (show login form, can switch accounts)
4. **Backend down**: Show error message, don't crash app
5. **Invalid credentials**: Display backend error message
6. **Duplicate email on signup**: Display backend error message
7. **Network timeout**: Show "Request timed out" message

### Constraints
- **No React Router yet**: Keep hash-based routing for now (don't over-engineer)
- **Backend API base URL**: Configurable via `VITE_API_URL` env var
- **Token storage**: localStorage (not secure for sensitive data, but acceptable for MVP)
- **Session timeout**: 1 hour (backend JWT expiration, no refresh token yet)

## Related Documents
- Backend auth implementation: `/backend/internal/handlers/auth/`
- Existing frontend: `/frontend/src/App.tsx`
- Zustand stores: `/frontend/src/stores/`

## Open Questions

1. **User menu placement**: Top right corner of header? (Recommendation: Yes)
2. **Avatar style**: Initials circle or icon? (Recommendation: Initials)
3. **Protected tab indicators**: Visual indicator that Assets/Locations require login? (Recommendation: No, discover on click)
4. **Token refresh**: Implement now or later? (Recommendation: Later, not MVP)

## Success Metrics
- User can sign up without errors
- User can log in and access protected features
- Public features remain accessible without login
- Navigation redirects work as expected
- Cofounder can replace stub pages with real CRUD without auth changes

## Future Enhancements (Post-MVP)
1. **Token Refresh** - Auto-refresh before expiration
2. **Password Reset** - Email-based password reset flow
3. **Email Verification** - Verify email before account activation
4. **Social Login** - OAuth with Google/GitHub
5. **Session Management** - View/revoke active sessions
6. **Remember Me** - Extended token expiration
7. **Account Switching** - Switch between multiple accounts
8. **Profile Settings** - Edit user profile (name, email, password)
