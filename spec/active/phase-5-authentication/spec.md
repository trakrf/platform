# Phase 5: Authentication

**Workspace**: backend
**Status**: Active
**Created**: 2025-10-18
**Linear**: TRA-79
**Priority**: Urgent

## Outcome

Implement JWT-based authentication system for the Go backend, porting core patterns from the existing Next.js implementation (trakrf-web). Protect all Phase 4A REST API endpoints with authentication middleware, and provide login/signup endpoints for user credential management.

## User Story

As a backend developer
I want a JWT authentication system with email/password login
So that the frontend can authenticate users and access protected API endpoints

## Context

**Current State**:
- ✅ Phase 4A complete - REST API with 17 endpoints (accounts, users, account_users, health)
- ✅ Database schema has users table with password_hash, last_login_at fields
- ✅ Next.js app (trakrf-web) has working next-auth implementation with email/password + Google OAuth
- ❌ All API endpoints are currently PUBLIC (no authentication required)
- ❌ No login/signup endpoints
- ❌ No JWT generation or validation

**Desired State**:
- JWT-based authentication (stateless, scalable)
- Login and signup endpoints
- Protected routes with auth middleware
- Password hashing with bcrypt (matching Next.js implementation)
- Multi-tenant account support via JWT claims

**Why Now?**: Phase 4A gave us the REST API. Phase 5 adds authentication to protect it. Phase 6 (frontend integration) will consume both.

## Reference Implementation Analysis

**Existing Next.js Auth (trakrf-web):**

The Next.js app provides a working reference implementation we can port:

### 1. Password Hashing (`lib/password.ts`)
```typescript
// Uses bcryptjs with 10 salt rounds
export async function hashPassword(password: string): Promise<string> {
  const saltRounds = 10;
  return bcryptjs.hash(password, saltRounds);
}

export async function comparePassword(password: string, hash: string): Promise<boolean> {
  return bcryptjs.compare(password, hash);
}
```

**Go Equivalent:** `golang.org/x/crypto/bcrypt` (standard library)
- Same bcrypt algorithm
- Same cost factor (10)
- Drop-in conceptual replacement

### 2. User Verification Flow (`db/services/userService.ts`)
```typescript
async verifyCredentials(email: string, password: string) {
  // 1. Find user by email
  const userResults = await this.getUserByEmail(email);
  if (!userResults.length) return null;

  const user = userResults[0];

  // 2. Check password hash exists
  if (!user.passwordHash) return null;

  // 3. Verify password with bcrypt
  const isValid = await comparePassword(password, user.passwordHash);
  if (!isValid) return null;

  // 4. Record login timestamp
  await this.recordLogin(user.id);

  // 5. Get default account ID (multi-tenant)
  const defaultAccountId = await this.getDefaultAccountId(user.id);

  // 6. Return user without sensitive data
  return {
    id: user.id,
    name: user.name,
    email: user.email,
    currentAccountId: defaultAccountId,
  };
}
```

**Go Port:** Implement identical flow in `AuthService.Login()`
- Reuse existing `UserRepository.GetByEmail()` from Phase 4A
- Add bcrypt comparison
- Update `last_login_at` in users table
- Query `account_users` to get default account
- Return user + generate JWT

### 3. Signup Flow (`app/api/auth/signup/route.ts`)
```typescript
export async function POST(request: NextRequest) {
  // 1. Validate input (zod schema)
  const { name, email, password } = validationResult.data;

  // 2. Check for existing user (409 Conflict)
  const existingUser = await userService.getUserByEmail(email);
  if (existingUser.length > 0) {
    return NextResponse.json({ error: 'Email already registered' }, { status: 409 });
  }

  // 3. Hash password
  const passwordHash = await userService.hashPassword(password);

  // 4. Create user
  const newUser = await userService.createUser({
    name, email, passwordHash,
    settings: { preferredAccountId: trakrfAccountId },
    metadata: { signupDate: new Date().toISOString(), source: 'web' }
  });

  // 5. Add to default account (TrakRF account)
  await userService.addUserToAccount(newUser[0].id, trakrfAccountId, 'member', 'active');

  // 6. Return user (without password)
  return NextResponse.json({ success: true, user: { id, name, email } });
}
```

**Go Port:** Implement as `POST /api/v1/auth/signup`
- Reuse `go-playground/validator` from Phase 4A
- Reuse existing `UserRepository.Create()` and `AccountUserRepository.Create()`
- Return JWT token with user object

### 4. Next-Auth JWT Pattern (`auth.ts`)
```typescript
callbacks: {
  jwt({ token, user, trigger, session }) {
    if (user) {
      token.id = user.id;  // Add user ID to JWT
    }

    // Handle account switching
    if (trigger === "update" && session?.currentAccountId) {
      token.currentAccountId = session.currentAccountId;
    }

    return token;
  },
  session({ session, token }) {
    // Enrich session with JWT data
    session.user.id = token.id as string;
    session.user.currentAccountId = token.currentAccountId as number;
    return session;
  }
}
```

**Go Port:** JWT claims structure
```go
type JWTClaims struct {
    UserID           int    `json:"user_id"`
    Email            string `json:"email"`
    CurrentAccountID int    `json:"current_account_id"`
    jwt.RegisteredClaims
}
```

### 5. Protected Route Middleware (`middleware.ts`)
```typescript
export default auth((req) => {
  const isLoggedIn = !!req.auth;
  const isProtectedRoute = nextUrl.pathname.startsWith('/dashboard');

  if (!isLoggedIn && isProtectedRoute) {
    return NextResponse.redirect(new URL("/#signin", nextUrl));
  }
})
```

**Go Port:** Chi middleware
```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract JWT from Authorization header
        // Validate token
        // Set user context
        // Call next handler or return 401
    })
}
```

## What Can Be Reused vs Reference-Only

### ✅ Directly Reusable (Port to Go):

1. **Password hashing strategy**: bcrypt with cost factor 10
2. **Authentication flow**: email lookup → password compare → return user + token
3. **Signup flow**: validate → check duplicate → hash → create user → add to account
4. **JWT structure**: Store `user_id`, `email`, `current_account_id` in claims
5. **Error patterns**: 401 for bad credentials, 409 for duplicate email, 400 for validation
6. **Multi-tenant pattern**: Store preferred account in user.settings, default to first active account
7. **Last login tracking**: Update `last_login_at` on successful auth

### ⚠️ Reference Only (Conceptual Guidance):

1. **OAuth providers**: Next.js uses Google OAuth - defer to Phase 5B or Phase 6
2. **Session management**: Next-auth handles this - we'll use stateless JWT
3. **Client hooks**: `useSession()` is React-specific - frontend concern
4. **Refresh tokens**: Not in Next.js impl - defer to Phase 5B
5. **Email verification**: Not in Next.js impl - defer to Phase 5B

## Technical Requirements

### 1. Dependencies

**New Go packages:**
```go
github.com/golang-jwt/jwt/v5           // JWT generation and validation
golang.org/x/crypto/bcrypt             // Password hashing (stdlib)
```

**Already have:**
- `go-playground/validator` - Input validation (Phase 4A)
- `chi` - Router and middleware (Phase 4A)
- `pgx` - Database driver (Phase 4A)

### 2. Password Utilities

**File:** `backend/password.go`

```go
package main

import (
    "golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10  // Match Next.js implementation

// HashPassword generates bcrypt hash from plain text password
func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
    return string(bytes), err
}

// ComparePassword checks if password matches hash
func ComparePassword(password, hash string) error {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
```

### 3. JWT Utilities

**File:** `backend/jwt.go`

```go
package main

import (
    "time"
    "github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
    UserID           int    `json:"user_id"`
    Email            string `json:"email"`
    CurrentAccountID *int   `json:"current_account_id,omitempty"`
    jwt.RegisteredClaims
}

// GenerateJWT creates a signed JWT token for authenticated user
func GenerateJWT(userID int, email string, accountID *int) (string, error) {
    // Token expires in 1 hour (configurable via JWT_EXPIRATION env var)
    expirationTime := time.Now().Add(1 * time.Hour)

    claims := &JWTClaims{
        UserID:           userID,
        Email:            email,
        CurrentAccountID: accountID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(expirationTime),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

    // Sign with secret from JWT_SECRET env var
    return token.SignedString([]byte(getJWTSecret()))
}

// ValidateJWT parses and validates a JWT token
func ValidateJWT(tokenString string) (*JWTClaims, error) {
    claims := &JWTClaims{}

    token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
        return []byte(getJWTSecret()), nil
    })

    if err != nil || !token.Valid {
        return nil, err
    }

    return claims, nil
}

func getJWTSecret() string {
    secret := os.Getenv("JWT_SECRET")
    if secret == "" {
        // Development default - MUST be overridden in production
        secret = "dev-secret-change-in-production"
    }
    return secret
}
```

### 4. Auth Middleware

**File:** `backend/middleware.go` (extend existing)

```go
// authMiddleware validates JWT and sets user context
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract token from Authorization header
        authHeader := r.Header.Get("Authorization")
        if authHeader == "" {
            writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized, "Missing authorization header", "")
            return
        }

        // Expected format: "Bearer <token>"
        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        if tokenString == authHeader {
            writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized, "Invalid authorization format", "")
            return
        }

        // Validate JWT
        claims, err := ValidateJWT(tokenString)
        if err != nil {
            writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized, "Invalid or expired token", "")
            return
        }

        // Set user context for downstream handlers
        ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
        ctx = context.WithValue(ctx, "user_email", claims.Email)
        if claims.CurrentAccountID != nil {
            ctx = context.WithValue(ctx, "current_account_id", *claims.CurrentAccountID)
        }

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Helper to get authenticated user ID from context
func getUserID(ctx context.Context) (int, bool) {
    userID, ok := ctx.Value("user_id").(int)
    return userID, ok
}

// Helper to get current account ID from context
func getCurrentAccountID(ctx context.Context) (*int, bool) {
    accountID, ok := ctx.Value("current_account_id").(int)
    if !ok {
        return nil, false
    }
    return &accountID, true
}
```

### 5. Auth Service

**File:** `backend/auth.go`

```go
package main

import (
    "context"
    "errors"
    "github.com/jackc/pgx/v5/pgxpool"
)

var (
    ErrInvalidCredentials = errors.New("invalid email or password")
    ErrEmailTaken         = errors.New("email already registered")
)

type AuthService struct {
    db *pgxpool.Pool
}

// Login authenticates user with email and password, returns user + JWT
func (s *AuthService) Login(ctx context.Context, email, password string) (*User, string, error) {
    // 1. Get user by email
    user, err := userRepo.GetByEmail(ctx, email)
    if err != nil || user == nil {
        return nil, "", ErrInvalidCredentials
    }

    // 2. Compare password hash
    if err := ComparePassword(password, user.PasswordHash); err != nil {
        return nil, "", ErrInvalidCredentials
    }

    // 3. Get user's default account ID
    accountID := s.getDefaultAccountID(ctx, user.ID)

    // 4. Update last_login_at
    s.recordLogin(ctx, user.ID)

    // 5. Generate JWT
    token, err := GenerateJWT(user.ID, user.Email, accountID)
    if err != nil {
        return nil, "", err
    }

    // 6. Return user (without password_hash)
    user.PasswordHash = "" // Clear sensitive data
    return user, token, nil
}

// Signup creates new user account and returns user + JWT
func (s *AuthService) Signup(ctx context.Context, req SignupRequest) (*User, string, error) {
    // 1. Check for existing user
    existing, _ := userRepo.GetByEmail(ctx, req.Email)
    if existing != nil {
        return nil, "", ErrEmailTaken
    }

    // 2. Hash password
    passwordHash, err := HashPassword(req.Password)
    if err != nil {
        return nil, "", err
    }

    // 3. Create user
    user, err := userRepo.Create(ctx, CreateUserRequest{
        Email:        req.Email,
        Name:         req.Name,
        PasswordHash: passwordHash,
    })
    if err != nil {
        return nil, "", err
    }

    // 4. Add user to TrakRF account (auto-enrollment for MVP)
    trakrfAccountID := s.getTrakRFAccountID(ctx)
    if trakrfAccountID != nil {
        accountUserRepo.Create(ctx, *trakrfAccountID, AddUserToAccountRequest{
            UserID: user.ID,
            Role:   "member",
            Status: "active",
        })
    }

    // 5. Generate JWT
    token, err := GenerateJWT(user.ID, user.Email, trakrfAccountID)
    if err != nil {
        return nil, "", err
    }

    // 6. Return user + token
    user.PasswordHash = "" // Clear sensitive data
    return user, token, nil
}

// Helper: Get user's default account ID
func (s *AuthService) getDefaultAccountID(ctx context.Context, userID int) *int {
    // Query account_users table for user's accounts
    query := `
        SELECT account_id FROM trakrf.account_users
        WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL
        ORDER BY created_at ASC
        LIMIT 1
    `
    var accountID int
    err := s.db.QueryRow(ctx, query, userID).Scan(&accountID)
    if err != nil {
        return nil
    }
    return &accountID
}

// Helper: Get TrakRF account ID for auto-enrollment
func (s *AuthService) getTrakRFAccountID(ctx context.Context) *int {
    query := `SELECT id FROM trakrf.accounts WHERE domain = 'trakrf.id' LIMIT 1`
    var accountID int
    err := s.db.QueryRow(ctx, query).Scan(&accountID)
    if err != nil {
        return nil
    }
    return &accountID
}

// Helper: Record user login timestamp
func (s *AuthService) recordLogin(ctx context.Context, userID int) {
    query := `UPDATE trakrf.users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`
    s.db.Exec(ctx, query, userID)
}
```

### 6. Auth Handlers

**File:** `backend/auth.go` (extend)

```go
// LoginRequest for POST /api/v1/auth/login
type LoginRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
}

// SignupRequest for POST /api/v1/auth/signup
type SignupRequest struct {
    Name     string `json:"name" validate:"required,min=1,max=255"`
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
}

// AuthResponse for login/signup responses
type AuthResponse struct {
    Token string `json:"token"`
    User  *User  `json:"user"`
}

var authService *AuthService

func initAuthService() {
    authService = &AuthService{db: db}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    var req LoginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
        return
    }

    if err := validate.Struct(req); err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
        return
    }

    user, token, err := authService.Login(r.Context(), req.Email, req.Password)
    if err != nil {
        if errors.Is(err, ErrInvalidCredentials) {
            writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized, "Invalid email or password", "")
            return
        }
        writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Login failed", "")
        return
    }

    resp := AuthResponse{
        Token: token,
        User:  user,
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"data": resp})
}

func signupHandler(w http.ResponseWriter, r *http.Request) {
    var req SignupRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
        return
    }

    if err := validate.Struct(req); err != nil {
        writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
        return
    }

    user, token, err := authService.Signup(r.Context(), req)
    if err != nil {
        if errors.Is(err, ErrEmailTaken) {
            writeJSONError(w, r, http.StatusConflict, ErrConflict, "Email already registered", "")
            return
        }
        writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Signup failed", "")
        return
    }

    resp := AuthResponse{
        Token: token,
        User:  user,
    }
    writeJSON(w, http.StatusCreated, map[string]interface{}{"data": resp})
}

// registerAuthRoutes registers authentication endpoints
func registerAuthRoutes(r chi.Router) {
    r.Post("/api/v1/auth/login", loginHandler)
    r.Post("/api/v1/auth/signup", signupHandler)
}
```

### 7. Update main.go to Protect Routes

**File:** `backend/main.go` (modify)

```go
func main() {
    // ... existing setup ...

    // Initialize auth service
    initAuthService()

    // Setup chi router
    r := chi.NewRouter()

    // Apply global middleware
    r.Use(requestIDMiddleware)
    r.Use(recoveryMiddleware)
    r.Use(corsMiddleware)
    r.Use(contentTypeMiddleware)

    // Public routes (no auth required)
    r.Get("/healthz", healthzHandler)
    r.Get("/readyz", readyzHandler)
    r.Get("/health", healthHandler)

    // Auth routes (public)
    registerAuthRoutes(r)

    // Protected routes (require JWT)
    r.Group(func(r chi.Router) {
        r.Use(authMiddleware)  // Validate JWT

        // All Phase 4A endpoints now protected
        registerAccountRoutes(r)
        registerUserRoutes(r)
        registerAccountUserRoutes(r)
    })

    // ... rest of setup ...
}
```

### 8. Environment Variables

Add to `.env.example`:
```bash
# JWT Configuration
JWT_SECRET=your-secret-key-change-in-production
JWT_EXPIRATION=3600  # seconds (1 hour)
```

### 9. Testing Strategy

**Unit Tests** (`backend/auth_test.go`):
```go
func TestHashPassword(t *testing.T)
func TestComparePassword(t *testing.T)
func TestGenerateJWT(t *testing.T)
func TestValidateJWT(t *testing.T)
func TestLoginHandler_Validation(t *testing.T)
func TestSignupHandler_Validation(t *testing.T)
```

**Integration Tests** (manual - with real database):
```bash
# Signup
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"name":"Test User","email":"test@example.com","password":"password123"}'

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# Access protected route
curl http://localhost:8080/api/v1/accounts \
  -H "Authorization: Bearer <token-from-login>"

# Access protected route without token (should fail with 401)
curl http://localhost:8080/api/v1/accounts
```

## Two-Tier Access Model

**From Linear TRA-79:**

### Pre-Auth (Public Demo):
- Frontend RFID reading features (handheld core)
- No account required
- **Backend impact:** None - these are client-side features

### Post-Auth (Registered Users):
- Asset/location CRUD screens
- Account management
- Data persistence
- **Backend impact:** All CRUD endpoints require authentication

**Implementation:**
- All `/api/v1/accounts`, `/api/v1/users`, `/api/v1/account_users` endpoints protected
- Frontend handles two-tier UX (RFID demo → login prompt → full CRUD access)

## Constraints

- **Password Security**: Use bcrypt with cost factor 10 (matches Next.js)
- **JWT Secret**: MUST be set via `JWT_SECRET` env var in production
- **Backwards Compatibility**: Keep health endpoints public (K8s liveness/readiness)
- **Existing Schema**: Use `users.password_hash`, `users.last_login_at` fields as-is
- **No Breaking Changes**: Existing Phase 4A endpoints just gain auth middleware
- **Stateless**: JWT tokens are self-contained (no server-side session storage)

## Out of Scope (Deferred to Later Phases)

- ❌ OAuth providers (Google, GitHub) - Phase 5B or Phase 6
- ❌ Refresh tokens - Phase 5B
- ❌ Password reset flow - Phase 5B
- ❌ Email verification - Phase 5B
- ❌ Account switching endpoint - Phase 5B
- ❌ Rate limiting on auth endpoints - Phase 7+
- ❌ Two-factor authentication (2FA) - Phase 7+
- ❌ Magic link login - Phase 7+

## Success Metrics

### Functional (8/8 required)
- ✅ Can signup with email/password via POST /api/v1/auth/signup
- ✅ Can login with email/password via POST /api/v1/auth/login
- ✅ Receives JWT token on successful auth
- ✅ Can access protected routes with valid JWT in Authorization header
- ✅ Gets 401 Unauthorized on protected routes without JWT
- ✅ Gets 401 Unauthorized with invalid/expired JWT
- ✅ Duplicate email on signup returns 409 Conflict
- ✅ Invalid credentials on login return 401 Unauthorized

### Technical (7/7 required)
- ✅ JWT tokens are stateless (no server-side session storage)
- ✅ Bcrypt cost factor = 10 (matches Next.js implementation)
- ✅ JWT includes user_id, email, and current_account_id in claims
- ✅ Auth middleware sets user context for downstream handlers
- ✅ All Phase 4A routes protected (accounts, users, account_users)
- ✅ Health endpoints remain public (healthz, readyz, health)
- ✅ Password hash never exposed in API responses

### Security (6/6 required)
- ✅ Passwords hashed with bcrypt (never stored plain text)
- ✅ JWT signed with secret key from environment variable
- ✅ No password in JWT payload
- ✅ No sensitive data in error messages (don't leak "user exists" vs "wrong password")
- ✅ Authorization header format validated (Bearer <token>)
- ✅ Token expiration enforced (default 1 hour)

### Performance (2/2 expected)
- ✅ Login response < 200ms (bcrypt comparison is the bottleneck)
- ✅ Auth middleware overhead < 10ms per request (JWT validation is fast)

**Overall Target**: 23/23 metrics achieved (100%)

## Open Decisions for /plan

1. **JWT Expiration Time**: 1 hour vs 24 hours vs 7 days
   - Next.js doesn't specify - need to decide
   - Recommendation: 1 hour (short-lived), add refresh tokens in Phase 5B if needed

2. **JWT Storage (Frontend)**: localStorage vs httpOnly cookie
   - Next.js uses server-side sessions
   - Recommendation: Let frontend decide (probably localStorage for SPA)

3. **Error Messages**: Specific ("Email not found" vs "Wrong password") or Generic ("Invalid credentials")
   - Security vs UX tradeoff
   - Recommendation: Generic (don't leak user existence)

4. **Auto-Enrollment Account**: TrakRF account vs let user create account
   - Next.js auto-adds to TrakRF account
   - Recommendation: Follow Next.js pattern (auto-enroll to trakrf.id account)

5. **Logout Endpoint**: Include POST /api/v1/auth/logout endpoint?
   - JWT is stateless, so logout is client-side token deletion
   - Recommendation: Include for API completeness, but it's a no-op (returns 200 OK)

## References

- **Linear Issue**: https://linear.app/trakrf/issue/TRA-79/phase-5-authentication
- **Next.js Auth Impl**: `../trakrf-web/auth.ts`, `../trakrf-web/app/api/auth/signup/route.ts`
- **Password Utils**: `../trakrf-web/lib/password.ts`
- **User Service**: `../trakrf-web/db/services/userService.ts`
- **Phase 4A Spec**: `spec/SHIPPED.md` (REST API foundation)
- **Database Schema**: `database/migrations/000003_users.up.sql`

## Notes

### Why Port from Next.js Instead of Starting Fresh?

**Proven Patterns:**
- Next.js implementation is already working in production (trakrf-web)
- Password hashing, verification flow, multi-tenant account handling all tested
- Reduces risk of auth implementation bugs

**Consistency:**
- Same bcrypt cost factor = users can share credentials between Next.js and Go APIs
- Same JWT claims structure = frontend can use same token format
- Same error codes = frontend can reuse error handling logic

**Speed:**
- Don't reinvent the wheel - port working code
- Focus on Go-specific concerns (JWT library, middleware) not auth logic

### Why JWT Instead of Sessions?

**Scalability:**
- Stateless = no session storage needed (Redis, database)
- Horizontal scaling without sticky sessions
- No session replication across servers

**API-First:**
- JWT is standard for REST APIs
- Works well with SPA frontends
- Easy to consume from mobile apps later

**Already Have It:**
- Next.js uses JWT (via next-auth)
- Consistent token format across stack

### Why Defer OAuth to Phase 5B?

**Complexity:**
- OAuth adds significant complexity (redirect flows, state management, token exchange)
- Google/GitHub provider integration requires testing, credentials, callbacks

**MVP Focus:**
- Email/password covers 80% of use cases for MVP
- Can add OAuth later without breaking existing auth

**Incremental:**
- Get core auth working first
- Add OAuth when there's demand

### Progression Path:
- **Phase 5A** (this phase): Email/password auth + JWT + protected routes
- **Phase 5B**: Refresh tokens, password reset, email verification, OAuth (Google)
- **Phase 6**: Frontend integration, serve static assets
- **Phase 7+**: Rate limiting, 2FA, magic links, account switching
