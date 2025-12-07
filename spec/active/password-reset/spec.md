# Feature: Password Reset - Forgot Password Flow

**Linear Issue**: [TRA-100](https://linear.app/trakrf/issue/TRA-100/password-reset-forgot-password-flow)
**Status**: Active Specification
**Priority**: Medium
**Complexity**: 6/10
**Labels**: frontend, backend, security

---

## Origin

This specification is based on Linear issue TRA-100, which defines a complete self-service password reset flow. This is a follow-on to TRA-91 (Frontend Auth - Integration & User Menu), building on the existing authentication system.

## Outcome

Users who have forgotten their password can securely reset it themselves via email verification, without requiring admin intervention. The system prevents abuse through rate limiting and secure token management while maintaining a smooth user experience.

## User Story

**As a** TrakRF user who has forgotten their password
**I want** to reset my password using my email address
**So that** I can regain access to my account without contacting support

## Context

### Current State
- Users can log in with email/password (TRA-91)
- No self-service password recovery exists
- Forgotten password = locked out → requires manual intervention

### Desired State
- Users can initiate password reset from login screen
- Secure token sent via email with time limit (15 minutes)
- User completes reset via link in email
- Password updated → user can immediately log in

### Why This Matters
1. **User Autonomy**: Users unblocked 24/7 without waiting for support
2. **Support Burden**: Reduces manual password reset tickets
3. **Security**: Standardized, auditable recovery process
4. **Urgency**: Password lockout is high-urgency for users
5. **Trust**: Professional auth flow builds product confidence

---

## User Flow

```
┌─────────────────┐
│  Login Screen   │
│                 │
│  [Forgot PW?]  │◄─── Step 1: User clicks link
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Forgot Password │
│                 │
│ Enter email:    │◄─── Step 2: User enters email
│ [___________]   │
│                 │
│ [Send Link]     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Success Page   │
│                 │
│ "Check email"   │◄─── Step 3: Confirmation shown
│                 │      (even if email not found - security)
└─────────────────┘
         │
         │ (User checks email)
         │
         ▼
┌─────────────────┐
│  Email Inbox    │
│                 │
│ [Reset Link]    │◄─── Step 4: Email with token link
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Reset Password  │
│                 │
│ New password:   │◄─── Step 5: User enters new password
│ [___________]   │
│ Confirm:        │
│ [___________]   │
│                 │
│ [Reset]         │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Success Page   │
│                 │
│ "Password reset"│◄─── Step 6: Success → redirect to login
│ [Go to Login]   │
└─────────────────┘
```

---

## Technical Requirements

### Frontend Components

#### 1. ForgotPasswordScreen (`/forgot-password`)

**Purpose**: Capture user email and initiate reset flow

**Elements**:
- Email input field with validation
- "Send Reset Link" button (disabled until valid email entered)
- Link back to login
- Success message after submission: "If an account exists with this email, we've sent a password reset link. Check your inbox."
- Error handling:
  - Network errors
  - Rate limiting (429 response)
  - Server errors (5xx)

**UX Requirements**:
- Loading state during API call
- Disable form during submission
- Clear, reassuring copy
- Mobile responsive
- Accessible labels and ARIA

**API Integration**:
```typescript
POST /api/v1/auth/forgot-password
Request: { email: string }
Response: { message: string } // Always same message for security
```

#### 2. ResetPasswordScreen (`/reset-password?token=xyz`)

**Purpose**: Validate token and capture new password

**Elements**:
- Token validation on mount (from URL query param `?token=`)
- New password input with strength indicator
- Confirm password input
- "Reset Password" button (disabled until valid matching passwords)
- Success message with auto-redirect to login (5 second countdown)
- Error handling:
  - Invalid token
  - Expired token
  - Used token
  - Network errors

**Password Requirements** (enforced client + server):
- Minimum 8 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one number
- Optional: Special character

**Password Strength Indicator**:
- Visual bar: Red → Yellow → Green
- Text hints: "Weak" / "Good" / "Strong"
- Real-time feedback as user types

**API Integration**:
```typescript
POST /api/v1/auth/reset-password
Request: {
  token: string,
  newPassword: string
}
Response: { success: boolean, message: string }
```

#### 3. LoginScreen Updates

**Changes Required**:
- Add "Forgot Password?" link below login form
- Link navigates to `/forgot-password`
- Styled as secondary/muted link

#### 4. Routing

**New Routes**:
```typescript
{
  path: '/forgot-password',
  component: ForgotPasswordScreen,
  // Public route - no auth required
}

{
  path: '/reset-password',
  component: ResetPasswordScreen,
  // Public route - no auth required
  // Token validation happens inside component
}
```

---

### Backend Implementation

#### 1. API Endpoints

##### POST `/api/v1/auth/forgot-password`

**Purpose**: Generate reset token and send email

**Request**:
```json
{
  "email": "user@example.com"
}
```

**Response** (always same message - security):
```json
{
  "message": "If an account exists with this email, we've sent a password reset link."
}
```

**Status Codes**:
- `200 OK`: Always (even if email not found)
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Server error

**Logic**:
1. Validate email format
2. Check rate limit (3 requests/hour per email)
3. Look up user by email (in constant time - prevent timing attacks)
4. If user exists:
   - Generate crypto-secure token (32 bytes)
   - Hash token with SHA-256
   - Store hash in `password_reset_tokens` table
   - Set expiration (15 minutes from now)
   - Send email with reset link
5. If user doesn't exist:
   - Still wait same amount of time (prevent timing attacks)
   - Return same success message
6. Log attempt (email, IP, timestamp, success/fail) for security monitoring

**Rate Limiting**:
- Key: Email address (normalized/lowercase)
- Limit: 3 requests per hour
- Storage: Redis or in-memory (depending on scale)
- Response: `429 Too Many Requests` with `Retry-After` header

##### POST `/api/v1/auth/reset-password`

**Purpose**: Validate token and update password

**Request**:
```json
{
  "token": "abc123...",
  "newPassword": "SecurePass123"
}
```

**Response (Success)**:
```json
{
  "success": true,
  "message": "Password successfully reset. You can now log in."
}
```

**Response (Error)**:
```json
{
  "success": false,
  "message": "Invalid or expired reset token."
}
```

**Status Codes**:
- `200 OK`: Password reset successful
- `400 Bad Request`: Invalid token, expired, or already used
- `422 Unprocessable Entity`: Password doesn't meet requirements
- `500 Internal Server Error`: Server error

**Logic**:
1. Validate password meets requirements
2. Hash provided token with SHA-256
3. Look up token in database by hash
4. Validate:
   - Token exists
   - Not expired (`expires_at > NOW()`)
   - Not used (`used_at IS NULL`)
5. If valid:
   - Hash new password (bcrypt/argon2)
   - Update user's password
   - Mark token as used (`used_at = NOW()`)
   - Invalidate any other pending tokens for this user (optional - security)
   - Log successful reset
6. If invalid:
   - Return generic error (don't leak which validation failed)
   - Log failed attempt

#### 2. Token Generation & Security

**Token Generation**:
```go
// Generate crypto-secure random token
func generateResetToken() (string, error) {
    bytes := make([]byte, 32) // 32 bytes = 256 bits
    if _, err := crypto/rand.Read(bytes); err != nil {
        return "", err
    }
    return base64.URLEncoding.EncodeToString(bytes), nil
}
```

**Why 32 bytes?**
- 256 bits of entropy
- Immune to brute force (2^256 possibilities)
- URL-safe when base64 encoded

**Token Hashing**:
```go
func hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}
```

**Why hash in database?**
- If database compromised, attacker can't use tokens
- Industry standard practice (like passwords)
- Minimal performance overhead

**Token Storage**:
- Store only hash, never plaintext token
- Token sent in email and URL is plaintext (one-time use)
- Database lookup via hash

#### 3. Database Schema

**New Table: `password_reset_tokens`**

```sql
CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL, -- SHA-256 hex = 64 chars
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    INDEX idx_token_hash (token_hash),
    INDEX idx_user_id (user_id),
    INDEX idx_expires_at (expires_at)
);
```

**Why these indexes?**
- `token_hash`: Primary lookup during reset validation (hot path)
- `user_id`: Invalidate all user's tokens, lookup user's pending tokens
- `expires_at`: Cleanup job to delete expired tokens

**Cleanup Strategy**:
```go
// Cron job: Run daily at 2 AM
DELETE FROM password_reset_tokens
WHERE expires_at < NOW() - INTERVAL '1 day';
```

#### 4. Email Integration

**Email Service Options** (choose one):
1. **SendGrid**: Recommended for MVP
   - Generous free tier (100 emails/day)
   - Excellent developer experience
   - Good deliverability
2. **Postmark**: Best deliverability
   - Focus on transactional email
   - Higher cost but excellent support
3. **AWS SES**: Lowest cost at scale
   - Requires more setup (DKIM, SPF, DMARC)
   - Good if already on AWS

**Email Template** (HTML):

```html
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Reset Your Password - TrakRF</title>
</head>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <h1 style="color: #333;">Reset Your Password</h1>

    <p>Hi there,</p>

    <p>We received a request to reset your password for your TrakRF account. Click the button below to reset it:</p>

    <p style="text-align: center; margin: 30px 0;">
        <a href="{{RESET_LINK}}"
           style="background-color: #007bff; color: white; padding: 12px 24px;
                  text-decoration: none; border-radius: 4px; display: inline-block;">
            Reset Password
        </a>
    </p>

    <p style="color: #666; font-size: 14px;">
        Or copy and paste this link into your browser:<br>
        <a href="{{RESET_LINK}}">{{RESET_LINK}}</a>
    </p>

    <p style="color: #666; font-size: 14px;">
        This link will expire in 15 minutes for security reasons.
    </p>

    <hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">

    <p style="color: #999; font-size: 12px;">
        If you didn't request this password reset, you can safely ignore this email.
        Your password will remain unchanged.
    </p>

    <p style="color: #999; font-size: 12px;">
        Request details:<br>
        IP Address: {{IP_ADDRESS}}<br>
        Time: {{TIMESTAMP}}<br>
        Location: {{APPROXIMATE_LOCATION}} (if available)
    </p>
</body>
</html>
```

**Reset Link Format**:
```
https://trakrf.com/reset-password?token={{TOKEN}}
```

**Email Error Handling**:
- Log all email send attempts (success/failure)
- If email fails, still return success to user (security)
- Alert ops team if email failure rate exceeds threshold (e.g., 5%)
- Consider retry logic for transient failures

#### 5. Security Measures

##### Information Disclosure Prevention

**Problem**: Attackers can enumerate valid emails by checking if reset was sent

**Solution**: Always return same message

```go
// ❌ BAD - Reveals if email exists
if !userExists {
    return errors.New("No account with that email")
}

// ✅ GOOD - Same response always
func (s *AuthService) RequestPasswordReset(email string) error {
    user := s.repo.FindUserByEmail(email)

    if user != nil {
        token := generateToken()
        s.sendResetEmail(user, token)
    }

    // Simulate email send delay even if user doesn't exist
    time.Sleep(randomDelay(50, 150)) // 50-150ms

    return nil // Always success
}
```

##### Timing Attack Prevention

**Problem**: Response time differs if email exists vs doesn't exist (DB lookup + email send)

**Solution**: Constant-time responses

```go
func (s *AuthService) RequestPasswordReset(email string) error {
    start := time.Now()

    user := s.repo.FindUserByEmail(email)

    if user != nil {
        token := generateToken()
        s.sendResetEmail(user, token)
    }

    // Ensure minimum response time
    elapsed := time.Since(start)
    minDuration := 100 * time.Millisecond
    if elapsed < minDuration {
        time.Sleep(minDuration - elapsed)
    }

    return nil
}
```

##### Rate Limiting

**Implementation**:
```go
type RateLimiter struct {
    store map[string][]time.Time // email -> request times
    mu    sync.Mutex
}

func (r *RateLimiter) Allow(email string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-1 * time.Hour)

    // Clean old entries
    requests := r.store[email]
    valid := []time.Time{}
    for _, t := range requests {
        if t.After(cutoff) {
            valid = append(valid, t)
        }
    }

    // Check limit
    if len(valid) >= 3 {
        return false // Rate limited
    }

    // Allow and record
    valid = append(valid, now)
    r.store[email] = valid
    return true
}
```

**Production**: Use Redis with sliding window:
```go
// Redis key: "rate:forgot-password:{email}"
// Value: sorted set of timestamps
// TTL: 1 hour
```

##### Token Security Checklist

- [x] Crypto-random generation (not `math/rand`)
- [x] Sufficient entropy (32 bytes minimum)
- [x] Hashed in database (SHA-256 or better)
- [x] Single-use (mark as used after successful reset)
- [x] Short expiration (15 minutes)
- [x] URL-safe encoding (base64url)

#### 6. Configuration

**Environment Variables**:
```bash
# Email Service
EMAIL_SERVICE_PROVIDER=sendgrid        # sendgrid|postmark|ses
EMAIL_API_KEY=sg_xxx...
EMAIL_FROM_ADDRESS=noreply@trakrf.com
EMAIL_FROM_NAME=TrakRF

# Password Reset Settings
PASSWORD_RESET_TOKEN_EXPIRY_MINUTES=15
PASSWORD_RESET_RATE_LIMIT_REQUESTS=3
PASSWORD_RESET_RATE_LIMIT_WINDOW_HOURS=1
PASSWORD_RESET_FRONTEND_URL=https://trakrf.com/reset-password

# Security
PASSWORD_MIN_LENGTH=8
PASSWORD_REQUIRE_UPPERCASE=true
PASSWORD_REQUIRE_LOWERCASE=true
PASSWORD_REQUIRE_NUMBER=true
PASSWORD_REQUIRE_SPECIAL=false         # Optional for MVP
```

---

## Testing Requirements

### Unit Tests

#### Frontend
- Email validation logic
- Password strength calculation
- Form submission handling
- Error state rendering
- Success state rendering

#### Backend
```go
// Token generation
TestGenerateResetToken_Uniqueness()          // 1000 tokens, all unique
TestGenerateResetToken_Entropy()             // Check randomness
TestHashToken_Deterministic()                // Same input = same hash

// Token validation
TestValidateToken_ValidToken()               // Happy path
TestValidateToken_ExpiredToken()             // Expired = rejected
TestValidateToken_UsedToken()                // Used = rejected
TestValidateToken_InvalidToken()             // Invalid = rejected

// Rate limiting
TestRateLimiter_AllowsUnderLimit()           // 3 requests in hour = OK
TestRateLimiter_BlocksOverLimit()            // 4th request = blocked
TestRateLimiter_ResetsAfterWindow()          // After 1 hour = allowed

// Password validation
TestPasswordValidation_MeetsRequirements()   // Valid password accepted
TestPasswordValidation_TooShort()            // < 8 chars rejected
TestPasswordValidation_NoUppercase()         // No uppercase rejected
TestPasswordValidation_NoNumber()            // No number rejected

// Timing attacks
TestRequestReset_ConstantTime()              // Response time similar for exist/not exist
```

### Integration Tests

```go
// Full flow
TestPasswordResetFlow_Success()
  1. Request reset for valid email
  2. Token created in database
  3. Email sent
  4. Reset password with token
  5. Token marked as used
  6. Login with new password succeeds

TestPasswordResetFlow_ExpiredToken()
  1. Request reset
  2. Manually set token expiration to past
  3. Attempt reset
  4. Should fail with "expired" error

TestPasswordResetFlow_TokenReuse()
  1. Request reset
  2. Use token successfully
  3. Attempt to reuse same token
  4. Should fail with "already used" error

TestPasswordResetFlow_NonexistentEmail()
  1. Request reset for email not in system
  2. Should return success message (security)
  3. No email should be sent
  4. No token created in database

TestRateLimiting_Integration()
  1. Request reset 3 times rapidly
  2. 4th request should return 429
  3. Wait 1 hour
  4. Next request should succeed
```

### E2E Tests (Playwright)

```typescript
test('User can reset forgotten password', async ({ page }) => {
  // Setup: Create test user
  const testUser = await createTestUser({
    email: 'test@example.com',
    password: 'OldPassword123'
  });

  // 1. Navigate to login
  await page.goto('/login');

  // 2. Click forgot password link
  await page.click('text=Forgot Password?');
  await expect(page).toHaveURL('/forgot-password');

  // 3. Enter email
  await page.fill('input[type=email]', 'test@example.com');
  await page.click('button:has-text("Send Reset Link")');

  // 4. Verify success message
  await expect(page.locator('text=Check your email')).toBeVisible();

  // 5. Get reset token from database (test helper)
  const token = await getLatestResetToken('test@example.com');

  // 6. Navigate to reset page with token
  await page.goto(`/reset-password?token=${token}`);

  // 7. Enter new password
  await page.fill('input[name=newPassword]', 'NewPassword123');
  await page.fill('input[name=confirmPassword]', 'NewPassword123');
  await page.click('button:has-text("Reset Password")');

  // 8. Verify success and redirect
  await expect(page.locator('text=Password successfully reset')).toBeVisible();
  await page.waitForURL('/login', { timeout: 6000 });

  // 9. Log in with new password
  await page.fill('input[name=email]', 'test@example.com');
  await page.fill('input[name=password]', 'NewPassword123');
  await page.click('button:has-text("Log In")');

  // 10. Verify successful login
  await expect(page).toHaveURL('/dashboard');
});

test('Expired token shows error', async ({ page }) => {
  const expiredToken = await createExpiredResetToken('test@example.com');

  await page.goto(`/reset-password?token=${expiredToken}`);

  await expect(page.locator('text=expired')).toBeVisible();
});

test('Used token cannot be reused', async ({ page }) => {
  const token = await createResetToken('test@example.com');

  // Use token once
  await page.goto(`/reset-password?token=${token}`);
  await page.fill('input[name=newPassword]', 'NewPassword123');
  await page.fill('input[name=confirmPassword]', 'NewPassword123');
  await page.click('button:has-text("Reset Password")');
  await expect(page.locator('text=successfully reset')).toBeVisible();

  // Try to use again
  await page.goto(`/reset-password?token=${token}`);
  await expect(page.locator('text=Invalid or expired')).toBeVisible();
});

test('Rate limiting prevents abuse', async ({ page }) => {
  // Request reset 3 times
  for (let i = 0; i < 3; i++) {
    await page.goto('/forgot-password');
    await page.fill('input[type=email]', 'test@example.com');
    await page.click('button:has-text("Send Reset Link")');
    await expect(page.locator('text=Check your email')).toBeVisible();
  }

  // 4th request should fail
  await page.goto('/forgot-password');
  await page.fill('input[type=email]', 'test@example.com');
  await page.click('button:has-text("Send Reset Link")');

  await expect(page.locator('text=Too many requests')).toBeVisible();
});
```

---

## Validation Criteria

**Before marking TRA-100 as complete, verify**:

### Functional Requirements
- [ ] User can request password reset from login screen
- [ ] Email with reset link is sent to valid addresses
- [ ] Reset link contains valid, unique token
- [ ] Reset form validates password requirements
- [ ] Valid token allows password update
- [ ] User can log in with new password immediately after reset
- [ ] Old password no longer works after reset

### Security Requirements
- [ ] Tokens are cryptographically random (32+ bytes entropy)
- [ ] Tokens are hashed in database (SHA-256 or better)
- [ ] Tokens expire after 15 minutes
- [ ] Tokens are single-use (cannot reuse after successful reset)
- [ ] Rate limiting prevents abuse (3 requests/hour per email)
- [ ] No information disclosure (same message for exist/non-exist email)
- [ ] Response time similar for valid/invalid email (timing attack prevention)
- [ ] Invalid/expired/used tokens show appropriate error
- [ ] Password requirements enforced on frontend and backend

### UX Requirements
- [ ] Mobile-responsive on all screen sizes
- [ ] Clear, reassuring messages at each step
- [ ] Loading states during API calls
- [ ] Error messages are helpful but secure
- [ ] Success flow feels smooth and trustworthy
- [ ] Password strength indicator works in real-time
- [ ] Forms are accessible (ARIA labels, keyboard navigation)

### Testing Requirements
- [ ] All unit tests pass (frontend + backend)
- [ ] All integration tests pass
- [ ] All E2E tests pass (Playwright)
- [ ] Manual testing on mobile devices
- [ ] Cross-browser testing (Chrome, Firefox, Safari)

### Operational Requirements
- [ ] Email service configured and tested
- [ ] Email delivery monitored (success/failure rates)
- [ ] Failed reset attempts logged for security monitoring
- [ ] Database migration applied successfully
- [ ] Cleanup job for expired tokens scheduled
- [ ] Environment variables documented
- [ ] Error tracking configured (Sentry/similar)

---

## Implementation Guidance

### Suggested Implementation Order

1. **Database First**
   - Create migration for `password_reset_tokens` table
   - Test migration up/down
   - Verify indexes created

2. **Backend Core**
   - Token generation + hashing functions
   - Password validation function
   - Unit tests for above

3. **Backend Endpoints (TDD)**
   - Write integration tests first
   - Implement `/forgot-password` endpoint
   - Implement `/reset-password` endpoint
   - Add rate limiting

4. **Email Integration**
   - Choose email service (SendGrid recommended for MVP)
   - Create HTML email template
   - Implement email sending
   - Test with real email

5. **Frontend Components**
   - Create ForgotPasswordScreen
   - Create ResetPasswordScreen
   - Update LoginScreen with link
   - Add routing

6. **E2E Tests**
   - Write Playwright tests
   - Run against full stack

7. **Security Hardening**
   - Add timing attack prevention
   - Verify information disclosure prevention
   - Penetration testing

8. **Documentation**
   - Update API docs
   - Update user documentation
   - Document runbook for ops team

### Gotchas & Common Mistakes

**Don't**:
- ❌ Store unhashed tokens in database
- ❌ Reveal if email exists in system
- ❌ Allow unlimited reset requests
- ❌ Forget to invalidate token after use
- ❌ Use `math/rand` for token generation
- ❌ Skip timing attack prevention
- ❌ Forget to test email delivery
- ❌ Hardcode reset URL (use env var)

**Do**:
- ✅ Use `crypto/rand` for tokens
- ✅ Hash tokens before database storage
- ✅ Implement rate limiting
- ✅ Return same message for all requests
- ✅ Test full flow end-to-end
- ✅ Monitor email delivery rates
- ✅ Log all reset attempts
- ✅ Make expiration time configurable

### Performance Considerations

- Token lookup is by hash (indexed) - should be fast
- Rate limiter should use Redis for production (in-memory for dev)
- Email sending should be async (don't block request)
- Database cleanup job should run during low-traffic hours

### Monitoring & Alerting

**Metrics to Track**:
- Reset requests per hour/day
- Email delivery success rate
- Token expiration rate (unused tokens)
- Reset completion rate
- Rate limit hits per hour

**Alerts**:
- Email delivery failure rate > 5%
- Unusual spike in reset requests (potential attack)
- High rate limit hit rate (abuse or UX issue)

---

## Decision Log

### Why 15 minutes expiration?

**Rationale**: Balance between security and UX
- **Security**: Limits window for token to be compromised
- **UX**: Most users check email within minutes; 15 min is sufficient
- **Alternative considered**: 30 minutes (rejected - too long for security)
- **Alternative considered**: 5 minutes (rejected - too short for slow email)

### Why 3 requests per hour rate limit?

**Rationale**: Prevent abuse while allowing legitimate retries
- User might not receive first email (spam folder)
- User might accidentally trigger twice
- 3 allows reasonable retries without enabling brute force
- **Alternative considered**: 5 requests (rejected - too permissive)
- **Alternative considered**: 1 request (rejected - too strict for legitimate use)

### Why SHA-256 for token hashing?

**Rationale**: Industry standard, sufficient security, good performance
- Tokens are random (no need for slow hash like bcrypt)
- SHA-256 is cryptographically secure for this use case
- Fast enough to not impact request latency
- **Alternative considered**: bcrypt (rejected - overkill for random tokens)

### Why single-use tokens?

**Rationale**: Prevents replay attacks
- If token intercepted, attacker gets one chance only
- Minimal UX impact (user rarely needs to reset twice)
- Industry best practice

### Why same response for exist/non-exist email?

**Rationale**: Prevent email enumeration
- Attacker can't discover valid emails in system
- Small UX cost (user can't confirm typo) for large security gain
- Industry standard practice (GitHub, Google, etc. do this)

---

## Dependencies

### Requires
- **TRA-91**: Frontend Auth - Integration & User Menu (completed)
  - Provides login screen to link from
  - Provides authentication infrastructure
- **Email Service**: SendGrid/Postmark/SES account and API key
- **Database**: Migration system ready

### Blocks
- None (standalone feature)

### Future Enhancements (Out of Scope)
- Password reset via SMS (TRA-XXX - future)
- Multi-factor authentication during reset (TRA-XXX - future)
- Password history (prevent reusing last N passwords)
- Account recovery questions
- Admin-initiated password resets (different flow)

---

## References

- **Linear Issue**: [TRA-100](https://linear.app/trakrf/issue/TRA-100/password-reset-forgot-password-flow)
- **Branch**: `miks2u/tra-100-password-reset-forgot-password-flow`
- **OWASP Forgot Password Cheat Sheet**: https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html
- **RFC 7519 (JWT)**: For token format inspiration (though we use random tokens, not JWTs)
- **NIST Password Guidelines**: https://pages.nist.gov/800-63-3/sp800-63b.html

---

## Success Metrics

**MVP Success** (before shipping):
- ✅ All validation criteria met
- ✅ All tests passing (unit + integration + E2E)
- ✅ Security review completed
- ✅ Email delivery tested with real addresses
- ✅ Rate limiting verified under load

**Post-Launch Success** (1 month after):
- Reset completion rate > 80%
- Email delivery rate > 95%
- Zero security incidents related to password reset
- < 5% of resets require support intervention
- User feedback positive (via support tickets/surveys)

---

## Notes for Implementers

1. **Start with security**: Get token generation, hashing, and validation right first
2. **Test email early**: Don't wait until end to discover email config issues
3. **Mobile first**: Many users will reset password on mobile device
4. **Clear copy**: Every message should reduce user anxiety, build trust
5. **Log everything**: Reset attempts are security-relevant events
6. **Monitor email**: Email delivery is single point of failure for this flow

**Questions?** Refer to Linear issue or ask in #engineering channel.
