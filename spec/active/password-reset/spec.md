# Feature: Password Reset - Forgot Password Flow (MVP)

**Linear Issue**: [TRA-100](https://linear.app/trakrf/issue/TRA-100/password-reset-forgot-password-flow)
**Security Hardening**: [TRA-149](https://linear.app/trakrf/issue/TRA-149/password-reset-security-hardening) (Backlog)
**Labels**: frontend, backend

---

## Outcome

Users who forget their password can reset it via email without admin intervention.

## User Flow

1. Click "Forgot Password?" on login screen
2. Enter email address
3. See "Check your email" message (always - don't leak account existence)
4. Receive email with reset link
5. Click link -> new password form (password + confirm)
6. Submit -> password updated
7. Redirect to login with success message

---

## Backend Implementation

### Database Migration

Create `000021_password_reset_tokens.up.sql`:

```sql
SET search_path = trakrf, public;

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

Down migration:

```sql
SET search_path = trakrf, public;
DROP TABLE IF EXISTS password_reset_tokens;
```

### Token Generation

```go
import (
    "crypto/rand"
    "encoding/hex"
)

func generateResetToken() (string, error) {
    bytes := make([]byte, 32)
    if _, err := rand.Read(bytes); err != nil {
        return "", err
    }
    return hex.EncodeToString(bytes), nil // 64 char hex string
}
```

### Email Service: Resend

**Setup:**
1. Create account at https://resend.com
2. Verify domain `trakrf.id` (add DNS records)
3. Create API key
4. Add `RESEND_API_KEY` to Railway environment

**Install:**
```bash
go get github.com/resend/resend-go/v2
```

**Send email:**
```go
import "github.com/resend/resend-go/v2"

func sendPasswordResetEmail(toEmail, token string) error {
    client := resend.NewClient(os.Getenv("RESEND_API_KEY"))

    resetURL := fmt.Sprintf("https://app.trakrf.id/#reset-password?token=%s", token)

    _, err := client.Emails.Send(&resend.SendEmailRequest{
        From:    "TrakRF <noreply@trakrf.id>",
        To:      []string{toEmail},
        Subject: "Reset your TrakRF password",
        Html:    fmt.Sprintf(`
            <h2>Reset your password</h2>
            <p>Click the link below to reset your TrakRF password. This link expires in 24 hours.</p>
            <p><a href="%s">Reset Password</a></p>
            <p>If you didn't request this, you can safely ignore this email.</p>
        `, resetURL),
    })
    return err
}
```

### API Endpoints

#### POST /api/v1/auth/forgot-password

Request:
```json
{ "email": "user@example.com" }
```

Response (always 200, don't leak account existence):
```json
{ "message": "If an account exists, a reset email has been sent" }
```

Logic:
1. Look up user by email
2. If not found -> return success anyway (don't leak)
3. If found -> delete any existing tokens for this user
4. Generate new token, store with 24h expiry
5. Send email via Resend
6. Return success

#### POST /api/v1/auth/reset-password

Request:
```json
{
  "token": "abc123...",
  "password": "newpassword123"
}
```

Response (success):
```json
{ "message": "Password updated successfully" }
```

Response (error - expired/invalid):
```json
{ "error": "Invalid or expired reset link" }
```

Logic:
1. Look up token in database
2. Check not expired (expires_at > NOW())
3. If invalid/expired -> return error
4. Validate password (min 8 chars)
5. Hash new password (bcrypt)
6. Update user's password_hash
7. DELETE the token (single-use)
8. Return success

### File Structure

```
backend/
├── internal/
│   ├── handlers/auth/
│   │   └── auth.go          # Add ForgotPassword, ResetPassword handlers
│   ├── services/auth/
│   │   └── auth.go          # Add password reset business logic
│   ├── services/email/
│   │   └── resend.go        # NEW: Resend email client wrapper
│   └── storage/
│       └── password_reset.go # NEW: Token CRUD operations
└── database/migrations/
    ├── 000021_password_reset_tokens.up.sql
    └── 000021_password_reset_tokens.down.sql
```

---

## Frontend Implementation

### New Components

#### ForgotPasswordScreen

- Email input field
- "Send Reset Link" button
- Success message: "Check your email for a reset link"
- Link back to login
- Loading state on submit

#### ResetPasswordScreen

- Read token from URL: `#reset-password?token=xxx`
- Password input
- Confirm password input
- "Reset Password" button
- Validation: passwords match, min 8 chars
- Success -> redirect to login with toast
- Error -> show "Invalid or expired link" with link to request new one

### Routes

Add to App.tsx hash router:

```tsx
case 'forgot-password':
  return <ForgotPasswordScreen />;
case 'reset-password':
  return <ResetPasswordScreen token={getQueryParam('token')} />;
```

### LoginScreen Update

Add below password field:

```tsx
<a href="#forgot-password" className="text-sm text-blue-400 hover:underline">
  Forgot password?
</a>
```

### API Client

Add to `lib/api/auth.ts`:

```typescript
export const authApi = {
  // ... existing methods

  forgotPassword: async (email: string): Promise<void> => {
    await client.post('/auth/forgot-password', { email });
  },

  resetPassword: async (token: string, password: string): Promise<void> => {
    await client.post('/auth/reset-password', { token, password });
  },
};
```

---

## Environment Variables

Add to Railway:

```
RESEND_API_KEY=re_xxxxxxxxxxxxx
```

---

## Testing Checklist

- [ ] Forgot password link visible on login screen
- [ ] Submitting email shows success message (even for non-existent email)
- [ ] Email received with valid reset link
- [ ] Clicking link opens reset form with token pre-filled
- [ ] Can set new password (min 8 chars validated)
- [ ] Password confirmation must match
- [ ] After reset, can login with new password
- [ ] Used token cannot be reused (shows error)
- [ ] Expired token (24h+) shows error
- [ ] Requesting new reset invalidates old token

---

## Explicitly Deferred (TRA-149)

* Rate limiting
* Timing attack prevention
* Password strength meter/rules
* Token hashing (short-lived single-use is sufficient)
* 2FA
* Security audit logging
