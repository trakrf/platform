# Implementation Plan: Invite Signup Flow Fix (TRA-274)
Generated: 2026-01-14
Specification: spec.md

## Understanding

When a new user clicks an org invitation link, they currently see a confusing "Organization Name" field during signup. The fix:
1. Add `GET /auth/invitation-info` endpoint to fetch invite details (org name, role, email, user_exists)
2. AcceptInviteScreen fetches this and shows contextual UI with one button (Login or Signup based on user_exists)
3. SignupScreen hides org field, pre-fills email (read-only), and calls signup with `invitation_token`
4. Backend signup with token creates user WITHOUT personal org, adds to invited org atomically

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/handlers/auth/auth.go` (lines 43-76) - Handler pattern: decode → validate → service → error handling
- `backend/internal/services/auth/auth.go` (lines 278-349) - AcceptInvitation flow with token validation
- `backend/internal/storage/invitations.go` (lines 182-200) - GetInvitationByTokenHash query pattern
- `frontend/src/lib/api/auth.ts` - API client patterns
- `frontend/src/components/SignupScreen.tsx` - Form structure and validation

**Files to Create**:
- None (all modifications to existing files)

**Files to Modify**:
- `backend/internal/models/auth/auth.go` - Add InvitationInfoResponse, modify SignupRequest
- `backend/internal/handlers/auth/auth.go` - Add GetInvitationInfo handler, modify Signup handler
- `backend/internal/services/auth/auth.go` - Add GetInvitationInfo method, modify Signup method
- `backend/internal/storage/invitations.go` - Add GetInvitationInfoByTokenHash query
- `backend/internal/storage/users.go` - Add UserExistsByEmail query
- `frontend/src/lib/api/auth.ts` - Add getInvitationInfo method, modify signup signature
- `frontend/src/components/AcceptInviteScreen.tsx` - Fetch invite info, show contextual UI
- `frontend/src/components/SignupScreen.tsx` - Detect invite context, hide org field, pre-fill email

## Architecture Impact
- **Subsystems affected**: Backend (handlers, services, storage), Frontend (API, components)
- **New dependencies**: None
- **Breaking changes**: None - existing flows preserved

## Task Breakdown

### Task 1: Add InvitationInfo storage query
**File**: `backend/internal/storage/invitations.go`
**Action**: MODIFY
**Pattern**: Reference GetInvitationByTokenHash (lines 182-200)

**Implementation**:
```go
// Add new struct for invitation info response
type InvitationInfo struct {
    OrgID         int
    OrgName       string
    OrgIdentifier string
    Role          string
    Email         string
    InviterName   *string
    ExpiresAt     time.Time
    CancelledAt   *time.Time
    AcceptedAt    *time.Time
}

// Add query method - joins with organizations and users tables
func (s *Storage) GetInvitationInfoByTokenHash(ctx context.Context, tokenHash string) (*InvitationInfo, error)
// Query: SELECT i.*, o.name, o.identifier, u.name as inviter_name
//        FROM org_invitations i
//        JOIN organizations o ON i.org_id = o.id
//        LEFT JOIN users u ON i.invited_by = u.id
//        WHERE i.token = $1
```

**Validation**: `just backend test`

---

### Task 2: Add UserExistsByEmail storage query
**File**: `backend/internal/storage/users.go`
**Action**: MODIFY
**Pattern**: Reference existing user queries in same file

**Implementation**:
```go
func (s *Storage) UserExistsByEmail(ctx context.Context, email string) (bool, error)
// Query: SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1))
```

**Validation**: `just backend test`

---

### Task 3: Add InvitationInfoResponse model
**File**: `backend/internal/models/auth/auth.go`
**Action**: MODIFY

**Implementation**:
```go
type InvitationInfoResponse struct {
    OrgName       string  `json:"org_name"`
    OrgIdentifier string  `json:"org_identifier"`
    Role          string  `json:"role"`
    Email         string  `json:"email"`
    UserExists    bool    `json:"user_exists"`
    InviterName   *string `json:"inviter_name,omitempty"`
}
```

**Validation**: `just backend build`

---

### Task 4: Modify SignupRequest model
**File**: `backend/internal/models/auth/auth.go`
**Action**: MODIFY

**Implementation**:
```go
type SignupRequest struct {
    Email           string  `json:"email" validate:"required,email"`
    Password        string  `json:"password" validate:"required,min=8"`
    OrgName         string  `json:"org_name" validate:"required_without=InvitationToken,omitempty,min=2,max=100"`
    InvitationToken *string `json:"invitation_token,omitempty" validate:"omitempty,len=64"`
}
// OrgName required only when InvitationToken is not provided
```

**Validation**: `just backend build`

---

### Task 5: Add GetInvitationInfo service method
**File**: `backend/internal/services/auth/auth.go`
**Action**: MODIFY
**Pattern**: Reference AcceptInvitation method (lines 278-349)

**Implementation**:
```go
func (s *Service) GetInvitationInfo(ctx context.Context, token string) (*auth.InvitationInfoResponse, error) {
    // 1. Hash token with SHA256
    // 2. Call storage.GetInvitationInfoByTokenHash(tokenHash)
    // 3. Check if expired/cancelled/accepted → return specific errors
    // 4. Call storage.UserExistsByEmail(info.Email)
    // 5. Return InvitationInfoResponse with all fields
}
```

**Validation**: `just backend test`

---

### Task 6: Modify Signup service method
**File**: `backend/internal/services/auth/auth.go`
**Action**: MODIFY
**Pattern**: Reference existing Signup (lines 37-109) and AcceptInvitation (lines 278-349)

**Implementation**:
```go
func (s *Service) Signup(ctx context.Context, request auth.SignupRequest, ...) (*auth.AuthResponse, error) {
    // If InvitationToken provided:
    //   1. Hash token, get invitation info
    //   2. Validate token (not expired/cancelled/accepted)
    //   3. Verify request.Email matches invitation email (case-insensitive)
    //   4. Begin transaction
    //   5. Create user (email, name=email, password_hash) - NO org creation
    //   6. Add user to invited org with invitation role
    //   7. Mark invitation as accepted
    //   8. Commit transaction
    //   9. Generate JWT with invited org_id
    //   10. Return AuthResponse
    // Else (no token):
    //   Keep existing behavior (create user + personal org)
}
```

**Validation**: `just backend test`

---

### Task 7: Add GetInvitationInfo handler
**File**: `backend/internal/handlers/auth/auth.go`
**Action**: MODIFY
**Pattern**: Reference existing handlers (decode → service → error handling → respond)

**Implementation**:
```go
// GET /api/v1/auth/invitation-info?token={token}
func (h *Handler) GetInvitationInfo(w http.ResponseWriter, r *http.Request) {
    // 1. Get token from query params
    // 2. Validate token is 64 chars hex
    // 3. Call service.GetInvitationInfo(ctx, token)
    // 4. Handle errors:
    //    - "invalid_token" → 404 Not Found
    //    - "expired" → 404 Not Found (don't reveal it existed)
    //    - "cancelled" → 404 Not Found
    //    - "already_accepted" → 404 Not Found
    //    - Other → 500 Internal Server Error
    // 5. Return 200 OK with { data: InvitationInfoResponse }
}
```

**Validation**: `just backend test`

---

### Task 8: Modify Signup handler for invitation_token
**File**: `backend/internal/handlers/auth/auth.go`
**Action**: MODIFY
**Pattern**: Reference existing Signup handler (lines 43-76)

**Implementation**:
```go
// Modify existing Signup handler to handle invitation_token errors
// Add error cases:
//   - "email_mismatch:{email}" → 400 Bad Request with detail
//   - "invalid_token" → 400 Bad Request
//   - "expired" → 400 Bad Request
//   - "cancelled" → 400 Bad Request
//   - "already_accepted" → 400 Bad Request
```

**Validation**: `just backend test`

---

### Task 9: Register GetInvitationInfo route
**File**: `backend/internal/handlers/auth/auth.go` or router file
**Action**: MODIFY

**Implementation**:
```go
// Add route (no auth middleware required)
router.HandleFunc("/api/v1/auth/invitation-info", authHandler.GetInvitationInfo).Methods("GET")
```

**Validation**: `just backend build` then manual curl test

---

### Task 10: Add getInvitationInfo to frontend API
**File**: `frontend/src/lib/api/auth.ts`
**Action**: MODIFY

**Implementation**:
```typescript
export interface InvitationInfo {
  org_name: string;
  org_identifier: string;
  role: string;
  email: string;
  user_exists: boolean;
  inviter_name?: string;
}

export const authApi = {
  // ... existing methods ...

  getInvitationInfo: (token: string) =>
    apiClient.get<{ data: InvitationInfo }>(`/auth/invitation-info?token=${encodeURIComponent(token)}`),

  // Modify signup to accept optional invitation_token
  signup: (data: { email: string; password: string; org_name?: string; invitation_token?: string }) =>
    apiClient.post<AuthResponse>('/auth/signup', data),
};
```

**Validation**: `just frontend typecheck`

---

### Task 11: Update AcceptInviteScreen for unauthenticated state
**File**: `frontend/src/components/AcceptInviteScreen.tsx`
**Action**: MODIFY
**Pattern**: Reference existing component structure

**Implementation**:
```typescript
// Add state for invitation info
const [inviteInfo, setInviteInfo] = useState<InvitationInfo | null>(null);
const [loading, setLoading] = useState(true);
const [fetchError, setFetchError] = useState<string | null>(null);

// Fetch on mount when not authenticated
useEffect(() => {
  if (!isAuthenticated && token) {
    authApi.getInvitationInfo(token)
      .then(res => setInviteInfo(res.data.data))
      .catch(err => setFetchError(extractErrorMessage(err)))
      .finally(() => setLoading(false));
  }
}, [isAuthenticated, token]);

// Update unauthenticated UI (lines 108-144):
// - Show loading spinner while fetching
// - If fetchError: show error message (expired/invalid)
// - If inviteInfo: show "You've been invited to join {org_name} as {role}"
// - If inviteInfo.inviter_name: show "Invited by {inviter_name}"
// - Single button based on user_exists:
//   - user_exists=true: "Sign In" button
//   - user_exists=false: "Create Account" button
// - Preserve token in URL params for redirect
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 12: Update SignupScreen for invite context
**File**: `frontend/src/components/SignupScreen.tsx`
**Action**: MODIFY
**Pattern**: Reference existing form structure

**Implementation**:
```typescript
// Detect invite context from URL
const searchParams = new URLSearchParams(window.location.hash.split('?')[1] || '');
const returnTo = searchParams.get('returnTo');
const inviteToken = searchParams.get('token');
const isInviteFlow = returnTo === 'accept-invite' && inviteToken;

// Add state for invite info
const [inviteInfo, setInviteInfo] = useState<InvitationInfo | null>(null);
const [inviteLoading, setInviteLoading] = useState(isInviteFlow);

// Fetch invite info on mount if in invite flow
useEffect(() => {
  if (isInviteFlow && inviteToken) {
    authApi.getInvitationInfo(inviteToken)
      .then(res => {
        setInviteInfo(res.data.data);
        setEmail(res.data.data.email); // Pre-fill email
      })
      .catch(() => {
        // Redirect back to accept-invite with error
        window.location.hash = `#accept-invite?token=${inviteToken}&error=invalid`;
      })
      .finally(() => setInviteLoading(false));
  }
}, []);

// Conditional form rendering:
// - If isInviteFlow && inviteInfo:
//   - Show banner: "Joining {org_name}" with role badge
//   - Email field: read-only, pre-filled from inviteInfo.email
//   - Hide org_name field entirely
//   - Password field: normal
// - Else: show existing form (org_name required)

// Modify handleSubmit:
// - If isInviteFlow:
//   - Call signup({ email, password, invitation_token: inviteToken })
//   - On success: redirect to #home with toast "Welcome to {org_name}!"
//   - On error: redirect to #accept-invite?token=...&error=...
// - Else: existing behavior
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 13: Write backend unit tests
**File**: `backend/internal/services/auth/auth_test.go` (or create if needed)
**Action**: CREATE or MODIFY

**Test cases**:
1. `TestGetInvitationInfo_ValidToken` - returns org info and user_exists
2. `TestGetInvitationInfo_ExpiredToken` - returns error
3. `TestGetInvitationInfo_CancelledToken` - returns error
4. `TestGetInvitationInfo_InvalidToken` - returns error
5. `TestSignup_WithInvitationToken_Success` - creates user, adds to org, no personal org
6. `TestSignup_WithInvitationToken_EmailMismatch` - returns error
7. `TestSignup_WithInvitationToken_ExpiredToken` - returns error
8. `TestSignup_WithoutInvitationToken` - existing behavior unchanged

**Validation**: `just backend test`

---

### Task 14: Write frontend component tests
**File**: `frontend/src/components/AcceptInviteScreen.test.tsx` and `SignupScreen.test.tsx`
**Action**: CREATE or MODIFY

**Test cases**:
1. `AcceptInviteScreen` - shows org name and role when fetched
2. `AcceptInviteScreen` - shows "Sign In" when user_exists=true
3. `AcceptInviteScreen` - shows "Create Account" when user_exists=false
4. `AcceptInviteScreen` - shows error for invalid token
5. `SignupScreen` - hides org field in invite flow
6. `SignupScreen` - pre-fills email in invite flow (read-only)
7. `SignupScreen` - shows org name banner in invite flow
8. `SignupScreen` - submits with invitation_token

**Validation**: `just frontend test`

---

## Risk Assessment

- **Risk**: Token validation logic duplicated between GetInvitationInfo and Signup
  **Mitigation**: Extract shared validation into helper function in service layer

- **Risk**: Race condition if invitation expires between info fetch and signup submit
  **Mitigation**: Already handled - signup validates token again, returns clear error

- **Risk**: Email case sensitivity mismatch
  **Mitigation**: Use case-insensitive comparison (strings.EqualFold) - already pattern in AcceptInvitation

## Integration Points
- Store updates: authStore.signup() signature unchanged, just different payload
- Route changes: Add GET /api/v1/auth/invitation-info (public, no auth middleware)
- Config updates: None required

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just backend lint` or `just frontend lint`
- Gate 2: `just backend build` or `just frontend typecheck`
- Gate 3: `just backend test` or `just frontend test`

**Final validation**: `just validate`

## Validation Sequence
1. Tasks 1-4: Backend models/storage → `just backend build && just backend test`
2. Tasks 5-9: Backend service/handlers → `just backend validate`
3. Tasks 10-12: Frontend API/components → `just frontend validate`
4. Tasks 13-14: Tests → `just validate`

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and clarifying questions
✅ Similar patterns found: AcceptInvitation flow at `backend/internal/services/auth/auth.go:278-349`
✅ Token validation pattern established at `backend/internal/storage/invitations.go:182-200`
✅ All clarifying questions answered (4/4)
✅ Existing test patterns in both backend and frontend
⚠️ Signup service modification is largest change - needs careful transaction handling

**Assessment**: Well-defined feature extending established patterns. Token validation and user creation logic already exist - this combines them into a streamlined flow.

**Estimated one-pass success probability**: 85%

**Reasoning**: All patterns exist in codebase. Main complexity is atomic transaction in signup (user creation + org membership + invitation acceptance). Error handling patterns are well-established.
