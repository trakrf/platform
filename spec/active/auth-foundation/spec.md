# Feature: Auth Foundation - Initialization & 401 Handling

## Origin
**Linear Issue**: [TRA-96 - Auth Foundation - Initialization & 401 Handling](https://linear.app/trakrf/issue/TRA-96/auth-foundation-initialization-and-401-handling)

**Parent Issue**: TRA-91 (Phase 1 of 3)

Foundation for all auth functionality. This phase establishes the core auth lifecycle: app startup initialization and automatic logout on session expiry.

## Outcome
- Auth state correctly initialized on app load from localStorage
- Valid tokens persist across page reloads
- Expired/invalid tokens are cleared automatically on app startup
- 401 responses from any API call trigger automatic logout and redirect to login

## User Story
**As a returning user**
I want my authentication session to persist across page reloads
So that I don't have to log in repeatedly during my work session

**As a user with an expired token**
I want the app to detect this and log me out automatically
So that I don't encounter confusing errors or security issues

## Context

### Current State
- authStore exists with login/logout/signup methods (from Part 2)
- Token is stored in localStorage on successful login
- No initialization logic to read token on app startup
- No 401 response handling
- Page reloads lose auth state (even though token exists in localStorage)

### Desired State
- Auth state is initialized on app load by reading and validating localStorage token
- If token is valid: User is automatically logged in
- If token is expired/invalid: Token is cleared, user remains logged out
- Any 401 response from the API triggers automatic logout and redirect

## Technical Requirements

### 1. Auth Store Initialization Method
**File**: `frontend/src/store/authStore.ts` (or wherever authStore is defined)

**Add `initialize()` method**:
```tsx
initialize: async () => {
  const token = localStorage.getItem('authToken');
  if (!token) {
    set({ isAuthenticated: false, user: null });
    return;
  }

  try {
    // Option 1: Decode JWT and check expiration
    const decoded = decodeJWT(token);
    if (isExpired(decoded)) {
      localStorage.removeItem('authToken');
      set({ isAuthenticated: false, user: null });
      return;
    }

    // Option 2: Make test API call to validate
    const user = await api.getCurrentUser();
    set({ isAuthenticated: true, user, token });
  } catch (error) {
    // Token invalid or network error
    localStorage.removeItem('authToken');
    set({ isAuthenticated: false, user: null });
  }
}
```

**Implementation notes**:
- Should be idempotent (safe to call multiple times)
- Should not throw errors (handle all errors internally)
- Should clear token if ANY validation fails
- Consider which validation approach: JWT decode vs API call vs both

### 2. App-Level Initialization
**File**: `frontend/src/App.tsx`

**Add useEffect to call initialize on mount**:
```tsx
import { authStore } from '@/store/authStore';

function App() {
  useEffect(() => {
    authStore.getState().initialize();
  }, []);

  // ... rest of App component
}
```

**Implementation notes**:
- Should run on mount only (empty dependency array)
- Should not block rendering
- Initialize can be async but don't await it (let it run in background)

### 3. 401 Response Interceptor
**File**: Create `frontend/src/lib/apiClient.ts` (or add to existing API client)

**Add response interceptor for 401**:
```tsx
// If using Axios
axios.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      authStore.getState().logout();
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

// If using fetch wrapper
export async function fetchWithAuth(url: string, options: RequestInit = {}) {
  const response = await fetch(url, options);

  if (response.status === 401) {
    authStore.getState().logout();
    window.location.href = '/login';
  }

  return response;
}
```

**Implementation notes**:
- Must apply to ALL authenticated API calls
- Should call authStore logout to clear state
- Should redirect to /login (hard redirect with window.location)
- Optional: Show toast notification "Session expired"

### 4. Unit Tests
**File**: Create `frontend/src/store/authStore.test.ts` (or add to existing test file)

**Test scenarios**:
- `initialize()` with no token: Sets isAuthenticated=false
- `initialize()` with valid token: Sets isAuthenticated=true, loads user
- `initialize()` with expired token: Clears localStorage, sets isAuthenticated=false
- `initialize()` with invalid token: Clears localStorage, sets isAuthenticated=false
- 401 response: Calls logout, redirects to /login

## Validation Criteria

### Unit Tests
- [ ] Auth initialization with no token sets isAuthenticated=false
- [ ] Auth initialization with valid token sets isAuthenticated=true
- [ ] Auth initialization with expired token clears localStorage
- [ ] Auth initialization with invalid token clears localStorage
- [ ] 401 response triggers logout
- [ ] 401 response redirects to /login

### Manual Testing
- [ ] Start app with no token → Not logged in
- [ ] Start app with valid token → Logged in automatically
- [ ] Start app with expired token → Token cleared, not logged in
- [ ] Corrupt token in localStorage, reload → Token cleared, not logged in
- [ ] Make API call that returns 401 → Logged out, redirected to login

## Technical Constraints

### Dependencies
- authStore exists (from Part 2)
- localStorage available in browser
- JWT decoding library (if using JWT decode approach)

### Browser Support
- localStorage must be available
- Modern fetch or Axios for API calls

### Security
- Never log tokens
- Clear all auth artifacts on validation failure
- Defensive validation (assume localStorage can be tampered with)

## Implementation Notes

### Token Validation Approach
**Option A: JWT Decode + Expiration Check**
- Pros: Fast, no network call
- Cons: Doesn't verify token is still valid server-side
- Best for: Quick startup, assume server 401 will catch invalid tokens

**Option B: API Call to getCurrentUser()**
- Pros: Validates token with server, gets fresh user data
- Cons: Network latency on every app startup
- Best for: Security-critical apps, need fresh user data

**Option C: Both**
- Check expiration first (fast fail)
- If not expired, call API to validate
- Best of both worlds

**Recommendation**: Option A for now (JWT decode), rely on 401 interceptor for server-side validation

### Error Handling
- All errors in `initialize()` should result in logout state
- Don't show error messages (fail silently to logout)
- 401 interceptor can optionally show toast

## Code Organization
- Auth logic stays in authStore
- API interceptor in separate apiClient module
- App.tsx only calls initialize, doesn't contain logic

## Success Criteria

✅ **Functionality**:
- Auth state initialized on app load
- Valid tokens persist across reloads
- Invalid tokens cleared automatically
- 401 responses trigger logout

✅ **Testing**:
- All unit tests pass
- Manual testing scenarios verified

✅ **Code Quality**:
- Properly typed (TypeScript)
- Error handling is defensive
- No token logging

## Future Enhancements (Out of Scope)
- Token refresh before expiration
- Remember Me (extend expiration)
- Multiple concurrent sessions handling

## Definition of Done
- [ ] `initialize()` method added to authStore
- [ ] App.tsx calls initialize on mount
- [ ] 401 interceptor added to API client
- [ ] All unit tests pass
- [ ] Manual testing completed
- [ ] Code reviewed
- [ ] Ready for Phase 2 (UI Integration)
