# Implementation Plan: Password Reset (TRA-100)

**Complexity**: Medium
**Estimated Files**: 10-12 new/modified files
**Branch**: `feature/tra-100-password-reset`

---

## Phase 1: Database Migration

### 1.1 Create migration files

**Files:**
- `database/migrations/000021_password_reset_tokens.up.sql`
- `database/migrations/000021_password_reset_tokens.down.sql`

**Schema:**
```sql
CREATE TABLE password_reset_tokens (
  id SERIAL PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token VARCHAR(64) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_password_reset_tokens_token ON password_reset_tokens(token);
CREATE INDEX idx_password_reset_tokens_expires ON password_reset_tokens(expires_at);
```

---

## Phase 2: Backend Implementation

### 2.1 Add Resend email dependency

```bash
cd backend && go get github.com/resend/resend-go/v2
```

### 2.2 Create email service

**File:** `backend/internal/services/email/resend.go`

- `NewClient(apiKey string) *Client`
- `SendPasswordResetEmail(toEmail, token string) error`
- Reset URL format: `https://app.trakrf.id/#reset-password?token={token}`
- Email template with 24-hour expiry notice

### 2.3 Create password reset storage

**File:** `backend/internal/storage/password_reset.go`

Methods:
- `CreatePasswordResetToken(ctx, userID int, token string, expiresAt time.Time) error`
- `GetPasswordResetToken(ctx, token string) (*PasswordResetToken, error)`
- `DeletePasswordResetToken(ctx, token string) error`
- `DeleteUserPasswordResetTokens(ctx, userID int) error`

### 2.4 Add request/response models

**File:** `backend/internal/models/auth/auth.go` (extend existing)

```go
type ForgotPasswordRequest struct {
    Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
    Token    string `json:"token" validate:"required"`
    Password string `json:"password" validate:"required,min=8"`
}

type MessageResponse struct {
    Message string `json:"message"`
}
```

### 2.5 Extend auth service

**File:** `backend/internal/services/auth/auth.go` (extend existing)

Methods:
- `ForgotPassword(ctx, email string) error`
  - Look up user by email
  - If not found, return success (don't leak)
  - Delete existing tokens for user
  - Generate 64-char hex token via `crypto/rand`
  - Store token with 24h expiry
  - Send email via Resend

- `ResetPassword(ctx, token, newPassword string) error`
  - Look up token
  - Validate not expired
  - Hash new password with bcrypt
  - Update user's password_hash
  - Delete token (single-use)

### 2.6 Add API error messages

**File:** `backend/internal/apierrors/messages.go` (extend existing)

```go
const (
    AuthForgotPasswordInvalidJSON  = "Invalid JSON"
    AuthForgotPasswordValidation   = "Validation failed"
    AuthForgotPasswordFailed       = "Failed to process request"
    AuthResetPasswordInvalidJSON   = "Invalid JSON"
    AuthResetPasswordValidation    = "Validation failed"
    AuthResetPasswordInvalidToken  = "Invalid or expired reset link"
    AuthResetPasswordFailed        = "Failed to reset password"
)
```

### 2.7 Add auth handlers

**File:** `backend/internal/handlers/auth/auth.go` (extend existing)

Handlers:
- `ForgotPassword(w, r)` - POST /api/v1/auth/forgot-password
  - Always returns 200 with generic message
- `ResetPassword(w, r)` - POST /api/v1/auth/reset-password
  - Returns 200 on success, 400 on invalid/expired token

Register routes:
```go
r.Post("/api/v1/auth/forgot-password", handler.ForgotPassword)
r.Post("/api/v1/auth/reset-password", handler.ResetPassword)
```

---

## Phase 3: Frontend Implementation

### 3.1 Add API methods

**File:** `frontend/src/lib/api/auth.ts` (extend existing)

```typescript
export const authApi = {
  // ... existing methods

  forgotPassword: (email: string) =>
    apiClient.post('/auth/forgot-password', { email }),

  resetPassword: (token: string, password: string) =>
    apiClient.post('/auth/reset-password', { token, password }),
};
```

### 3.2 Create ForgotPasswordScreen

**File:** `frontend/src/components/ForgotPasswordScreen.tsx`

Features:
- Email input field
- "Send Reset Link" button
- Success state: "Check your email for a reset link"
- Loading state on submit
- Link back to login (`#login`)
- Matches existing dark theme styling

### 3.3 Create ResetPasswordScreen

**File:** `frontend/src/components/ResetPasswordScreen.tsx`

Props: `{ token: string | null }`

Features:
- Read token from URL query param
- Password input with show/hide toggle
- Confirm password input with show/hide toggle
- Client-side validation:
  - Passwords match
  - Min 8 characters
- "Reset Password" button
- Success → redirect to `#login` with toast
- Error → "Invalid or expired link" with link to `#forgot-password`
- Matches existing dark theme styling

### 3.4 Update routing

**File:** `frontend/src/App.tsx`

Add to `VALID_TABS`:
```typescript
const VALID_TABS: TabType[] = [...existing, 'forgot-password', 'reset-password'];
```

Add lazy imports:
```typescript
const ForgotPasswordScreen = lazyWithRetry(() => import('@/components/ForgotPasswordScreen'));
const ResetPasswordScreen = lazyWithRetry(() => import('@/components/ResetPasswordScreen'));
```

Add to `tabComponents`:
```typescript
'forgot-password': ForgotPasswordScreen,
'reset-password': ResetPasswordScreen,
```

Pass token to ResetPasswordScreen from URL params.

### 3.5 Update TabType

**File:** `frontend/src/stores/index.ts` (or wherever TabType is defined)

Add new tab types to union.

### 3.6 Update LoginScreen

**File:** `frontend/src/components/LoginScreen.tsx` (modify existing)

Add "Forgot password?" link below password field:
```tsx
<a href="#forgot-password" className="text-sm text-blue-400 hover:underline">
  Forgot password?
</a>
```

---

## Phase 4: Environment & Deployment

### 4.1 Add environment variable

Add to Railway (both preview and prod):
```
RESEND_API_KEY=re_xxxxxxxxxxxxx
```

### 4.2 Run migration

Apply migration to preview, then production after verification.

---

## Testing Checklist

### Backend
- [ ] Token generation produces 64-char hex string
- [ ] Token stored with correct expiry (24h)
- [ ] Email lookup returns consistent response regardless of existence
- [ ] Expired tokens are rejected
- [ ] Used tokens are deleted
- [ ] New password is properly hashed
- [ ] User can login with new password

### Frontend
- [ ] Forgot password link visible on login screen
- [ ] Email validation on forgot password form
- [ ] Success message shown after submission
- [ ] Reset form validates password match
- [ ] Reset form validates min 8 chars
- [ ] Error state shows for invalid/expired tokens
- [ ] Success redirects to login with toast

### E2E
- [ ] Complete flow: forgot → email → reset → login works
- [ ] Invalid token shows error
- [ ] Re-requesting reset invalidates old token

---

## File Summary

**New Files (8):**
1. `database/migrations/000021_password_reset_tokens.up.sql`
2. `database/migrations/000021_password_reset_tokens.down.sql`
3. `backend/internal/services/email/resend.go`
4. `backend/internal/storage/password_reset.go`
5. `frontend/src/components/ForgotPasswordScreen.tsx`
6. `frontend/src/components/ResetPasswordScreen.tsx`
7. `frontend/src/components/__tests__/ForgotPasswordScreen.test.tsx`
8. `frontend/src/components/__tests__/ResetPasswordScreen.test.tsx`

**Modified Files (5):**
1. `backend/internal/models/auth/auth.go` - Add request/response types
2. `backend/internal/apierrors/messages.go` - Add error constants
3. `backend/internal/services/auth/auth.go` - Add ForgotPassword, ResetPassword methods
4. `backend/internal/handlers/auth/auth.go` - Add handlers and routes
5. `frontend/src/lib/api/auth.ts` - Add API methods
6. `frontend/src/App.tsx` - Add routes
7. `frontend/src/components/LoginScreen.tsx` - Add forgot password link
