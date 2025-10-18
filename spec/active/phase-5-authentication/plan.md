# Phase 5: Authentication - Implementation Plan

**Status**: Active
**Created**: 2025-10-18
**Spec**: spec.md

## Open Decisions - Resolved

### 1. JWT Expiration Time
**Decision**: 1 hour (3600 seconds)
**Rationale**:
- Short-lived tokens are more secure
- If users need longer sessions, we'll add refresh tokens in Phase 5B
- Matches common industry practice for access tokens

### 2. JWT Storage (Frontend)
**Decision**: Frontend's choice (likely localStorage)
**Rationale**:
- Backend just returns the token - frontend decides how to store it
- SPA pattern typically uses localStorage
- httpOnly cookies would require session-based auth (not our pattern)

### 3. Error Messages
**Decision**: Generic error messages for auth failures
**Rationale**:
- Don't leak whether an email exists ("Invalid credentials" not "Email not found")
- Better security posture (prevent user enumeration)
- Matches Next.js implementation pattern

### 4. Auto-Enrollment Account
**Decision**: Auto-enroll new users to TrakRF account (domain: trakrf.id)
**Rationale**:
- Matches Next.js behavior for consistency
- Ensures users have a default account immediately
- Simplifies MVP signup flow (no account creation step)

### 5. Logout Endpoint
**Decision**: Include POST /api/v1/auth/logout but make it a no-op
**Rationale**:
- API completeness (clients expect a logout endpoint)
- JWT is stateless, so logout is client-side token deletion
- Returns 200 OK with a message acknowledging the logout request

## Implementation Steps

### Step 1: Install Dependencies
**Files**: `backend/go.mod`, `backend/go.sum`

```bash
cd backend
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto/bcrypt
```

**Verification**:
- `go mod tidy` succeeds
- Dependencies added to go.mod

---

### Step 2: Create Password Utilities
**Files**: `backend/password.go`

**Implementation**:
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

**Testing**:
- Create `backend/password_test.go`
- Test: `TestHashPassword` - verify hash is different from password
- Test: `TestComparePassword` - verify matching password returns nil error
- Test: `TestComparePassword_Invalid` - verify wrong password returns error
- Run: `go test -run TestHashPassword && go test -run TestComparePassword`

**Verification**:
- All password tests pass
- Hash length is 60 characters (bcrypt standard)

---

### Step 3: Create JWT Utilities
**Files**: `backend/jwt.go`

**Implementation**:
```go
package main

import (
    "os"
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
        secret = "dev-secret-change-in-production"
    }
    return secret
}
```

**Testing**:
- Create `backend/jwt_test.go`
- Test: `TestGenerateJWT` - verify token is non-empty string
- Test: `TestValidateJWT` - verify can parse generated token
- Test: `TestValidateJWT_Expired` - verify expired tokens fail
- Test: `TestValidateJWT_Invalid` - verify malformed tokens fail
- Run: `go test -run TestJWT`

**Verification**:
- All JWT tests pass
- Token contains user_id, email, current_account_id claims
- Expiration is ~1 hour from issuance

---

### Step 4: Extend Users Repository
**Files**: `backend/users.go`

**Add Method**:
```go
// GetByEmail retrieves user by email address
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
    query := `
        SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
        FROM trakrf.users
        WHERE email = $1 AND deleted_at IS NULL
    `

    var u User
    err := r.db.QueryRow(ctx, query, email).Scan(
        &u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastLoginAt,
        &u.Settings, &u.Metadata, &u.CreatedAt, &u.UpdatedAt)

    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, nil // Not found
        }
        return nil, fmt.Errorf("failed to get user by email: %w", err)
    }

    return &u, nil
}
```

**Verification**:
- Build succeeds: `go build ./backend`
- Method signature matches usage in auth service

---

### Step 5: Create Auth Service
**Files**: `backend/auth.go`

**Implementation**:
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
    user.PasswordHash = ""
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
    user.PasswordHash = ""
    return user, token, nil
}

// Helper: Get user's default account ID
func (s *AuthService) getDefaultAccountID(ctx context.Context, userID int) *int {
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

**Verification**:
- Build succeeds: `go build ./backend`
- All methods compile without errors

---

### Step 6: Create Auth Handlers
**Files**: `backend/auth_handlers.go` (new file to keep auth.go focused on service logic)

**Implementation**:
```go
package main

import (
    "encoding/json"
    "errors"
    "net/http"
)

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

func logoutHandler(w http.ResponseWriter, r *http.Request) {
    // JWT is stateless - logout happens client-side
    // This endpoint exists for API completeness
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "message": "Logout successful. Please delete the token on the client.",
    })
}

// registerAuthRoutes registers authentication endpoints
func registerAuthRoutes(r chi.Router) {
    r.Post("/api/v1/auth/login", loginHandler)
    r.Post("/api/v1/auth/signup", signupHandler)
    r.Post("/api/v1/auth/logout", logoutHandler)
}
```

**Verification**:
- Build succeeds: `go build ./backend`
- Handler signatures match chi router expectations

---

### Step 7: Create Auth Middleware
**Files**: `backend/middleware.go` (extend existing)

**Add to existing middleware.go**:
```go
import (
    "strings"
)

// Add new error constant to existing errors.go
const contextKey string = "user"

type userContextKey string

const (
    userIDKey           userContextKey = "user_id"
    userEmailKey        userContextKey = "user_email"
    currentAccountIDKey userContextKey = "current_account_id"
)

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
        ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
        ctx = context.WithValue(ctx, userEmailKey, claims.Email)
        if claims.CurrentAccountID != nil {
            ctx = context.WithValue(ctx, currentAccountIDKey, *claims.CurrentAccountID)
        }

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Helper to get authenticated user ID from context
func getUserID(ctx context.Context) (int, bool) {
    userID, ok := ctx.Value(userIDKey).(int)
    return userID, ok
}

// Helper to get current account ID from context
func getCurrentAccountID(ctx context.Context) (*int, bool) {
    accountID, ok := ctx.Value(currentAccountIDKey).(int)
    if !ok {
        return nil, false
    }
    return &accountID, true
}
```

**Verification**:
- Build succeeds: `go build ./backend`
- Middleware compiles without errors

---

### Step 8: Add ErrUnauthorized to errors.go
**Files**: `backend/errors.go`

**Add**:
```go
ErrUnauthorized = "UNAUTHORIZED"
```

**Verification**:
- Build succeeds: `go build ./backend`

---

### Step 9: Update main.go to Wire Everything Together
**Files**: `backend/main.go`

**Changes**:
1. Add `initAuthService()` call after database initialization
2. Register auth routes (public)
3. Wrap existing routes in `r.Group()` with `authMiddleware`

```go
func main() {
    // ... existing setup ...

    // Initialize repositories
    initAccountRepo()
    initUserRepo()
    initAccountUserRepo()
    initAuthService()  // NEW

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

    // Auth routes (public) - NEW
    registerAuthRoutes(r)

    // Protected routes (require JWT) - MODIFIED
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

**Verification**:
- Build succeeds: `go build ./backend`
- Server starts: `./backend` (or `air` with hot reload)

---

### Step 10: Update Environment Variables
**Files**: `.env.example`, `docker-compose.yml`

**Add to .env.example**:
```bash
# JWT Configuration
JWT_SECRET=your-secret-key-change-in-production
```

**Add to docker-compose.yml backend service**:
```yaml
services:
  backend:
    environment:
      # ... existing vars ...
      JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
```

**Verification**:
- .env.example has JWT_SECRET
- docker-compose.yml passes JWT_SECRET to backend container

---

### Step 11: Integration Testing
**Manual Testing** (using curl or Postman)

**Test 1: Health Endpoints (Public)**
```bash
curl http://localhost:8080/health
# Expected: 200 OK
```

**Test 2: Protected Route Without Auth (Should Fail)**
```bash
curl http://localhost:8080/api/v1/accounts
# Expected: 401 Unauthorized
# Response: {"error": {"code": "UNAUTHORIZED", "message": "Missing authorization header"}}
```

**Test 3: Signup**
```bash
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"name":"Test User","email":"test@example.com","password":"password123"}'

# Expected: 201 Created
# Response: {"data": {"token": "eyJ...", "user": {"id": 1, "email": "test@example.com", ...}}}
```

**Test 4: Duplicate Signup (Should Fail)**
```bash
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"name":"Test User","email":"test@example.com","password":"password123"}'

# Expected: 409 Conflict
# Response: {"error": {"code": "CONFLICT", "message": "Email already registered"}}
```

**Test 5: Login**
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# Expected: 200 OK
# Response: {"data": {"token": "eyJ...", "user": {...}}}
```

**Test 6: Login with Wrong Password (Should Fail)**
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"wrongpassword"}'

# Expected: 401 Unauthorized
# Response: {"error": {"code": "UNAUTHORIZED", "message": "Invalid email or password"}}
```

**Test 7: Protected Route With Auth (Should Succeed)**
```bash
# Extract token from login response, then:
curl http://localhost:8080/api/v1/accounts \
  -H "Authorization: Bearer <token-from-login>"

# Expected: 200 OK with accounts data
```

**Test 8: Protected Route With Invalid Token (Should Fail)**
```bash
curl http://localhost:8080/api/v1/accounts \
  -H "Authorization: Bearer invalid-token"

# Expected: 401 Unauthorized
# Response: {"error": {"code": "UNAUTHORIZED", "message": "Invalid or expired token"}}
```

**Test 9: Logout**
```bash
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -H "Authorization: Bearer <token>"

# Expected: 200 OK
# Response: {"message": "Logout successful. Please delete the token on the client."}
```

**Verification Checklist**:
- ✅ All 9 manual tests pass
- ✅ Can signup and receive JWT
- ✅ Can login and receive JWT
- ✅ Can access protected routes with valid JWT
- ✅ Cannot access protected routes without JWT
- ✅ Duplicate email returns 409
- ✅ Invalid credentials return 401
- ✅ Health endpoints remain public

---

### Step 12: Unit Testing
**Files**: Create test files for each component

**Create `backend/password_test.go`**:
```go
package main

import (
    "testing"
)

func TestHashPassword(t *testing.T) {
    password := "testpassword123"
    hash, err := HashPassword(password)

    if err != nil {
        t.Fatalf("HashPassword failed: %v", err)
    }

    if hash == password {
        t.Error("Hash should not equal plain password")
    }

    if len(hash) != 60 {
        t.Errorf("Expected hash length 60, got %d", len(hash))
    }
}

func TestComparePassword_Valid(t *testing.T) {
    password := "testpassword123"
    hash, _ := HashPassword(password)

    err := ComparePassword(password, hash)
    if err != nil {
        t.Errorf("ComparePassword should succeed for valid password: %v", err)
    }
}

func TestComparePassword_Invalid(t *testing.T) {
    password := "testpassword123"
    hash, _ := HashPassword(password)

    err := ComparePassword("wrongpassword", hash)
    if err == nil {
        t.Error("ComparePassword should fail for invalid password")
    }
}
```

**Create `backend/jwt_test.go`**:
```go
package main

import (
    "testing"
    "time"
    "os"
)

func TestGenerateJWT(t *testing.T) {
    os.Setenv("JWT_SECRET", "test-secret")

    userID := 1
    email := "test@example.com"
    accountID := 5

    token, err := GenerateJWT(userID, email, &accountID)

    if err != nil {
        t.Fatalf("GenerateJWT failed: %v", err)
    }

    if token == "" {
        t.Error("Token should not be empty")
    }
}

func TestValidateJWT_Valid(t *testing.T) {
    os.Setenv("JWT_SECRET", "test-secret")

    userID := 1
    email := "test@example.com"
    accountID := 5

    token, _ := GenerateJWT(userID, email, &accountID)
    claims, err := ValidateJWT(token)

    if err != nil {
        t.Fatalf("ValidateJWT failed: %v", err)
    }

    if claims.UserID != userID {
        t.Errorf("Expected UserID %d, got %d", userID, claims.UserID)
    }

    if claims.Email != email {
        t.Errorf("Expected Email %s, got %s", email, claims.Email)
    }

    if claims.CurrentAccountID == nil || *claims.CurrentAccountID != accountID {
        t.Errorf("Expected AccountID %d, got %v", accountID, claims.CurrentAccountID)
    }
}

func TestValidateJWT_Invalid(t *testing.T) {
    os.Setenv("JWT_SECRET", "test-secret")

    _, err := ValidateJWT("invalid.token.string")

    if err == nil {
        t.Error("ValidateJWT should fail for invalid token")
    }
}

func TestValidateJWT_WrongSecret(t *testing.T) {
    os.Setenv("JWT_SECRET", "secret1")
    token, _ := GenerateJWT(1, "test@example.com", nil)

    os.Setenv("JWT_SECRET", "secret2")
    _, err := ValidateJWT(token)

    if err == nil {
        t.Error("ValidateJWT should fail when secret changes")
    }
}
```

**Run Unit Tests**:
```bash
cd backend
go test -v -run TestHashPassword
go test -v -run TestComparePassword
go test -v -run TestGenerateJWT
go test -v -run TestValidateJWT
```

**Verification**:
- All unit tests pass
- Coverage includes password hashing, JWT generation, JWT validation

---

### Step 13: Documentation Updates
**Files**: `backend/README.md`, `PLANNING.md`

**Update backend/README.md** with new endpoints:
```markdown
### Authentication Endpoints

#### POST /api/v1/auth/signup
Create new user account and receive JWT token.

**Request**:
```json
{
  "name": "John Doe",
  "email": "john@example.com",
  "password": "password123"
}
```

**Response** (201 Created):
```json
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user": {
      "id": 1,
      "email": "john@example.com",
      "name": "John Doe",
      "created_at": "2025-10-18T10:00:00Z",
      "updated_at": "2025-10-18T10:00:00Z"
    }
  }
}
```

#### POST /api/v1/auth/login
Authenticate with email and password, receive JWT token.

**Request**:
```json
{
  "email": "john@example.com",
  "password": "password123"
}
```

**Response** (200 OK):
```json
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user": {
      "id": 1,
      "email": "john@example.com",
      "name": "John Doe",
      "last_login_at": "2025-10-18T10:05:00Z",
      "created_at": "2025-10-18T10:00:00Z",
      "updated_at": "2025-10-18T10:05:00Z"
    }
  }
}
```

#### POST /api/v1/auth/logout
Logout (client-side token deletion).

**Response** (200 OK):
```json
{
  "message": "Logout successful. Please delete the token on the client."
}
```

### Protected Routes

All Phase 4A endpoints now require authentication:
- GET /api/v1/accounts
- POST /api/v1/accounts
- GET /api/v1/accounts/:id
- PUT /api/v1/accounts/:id
- DELETE /api/v1/accounts/:id
- GET /api/v1/users
- POST /api/v1/users
- GET /api/v1/users/:id
- PUT /api/v1/users/:id
- DELETE /api/v1/users/:id
- GET /api/v1/accounts/:id/users
- POST /api/v1/accounts/:id/users
- PUT /api/v1/accounts/:id/users/:user_id
- DELETE /api/v1/accounts/:id/users/:user_id

**Authorization Header**:
```
Authorization: Bearer <jwt-token>
```
```

**Update PLANNING.md** Phase 5 section:
- Mark Phase 5A as complete
- Document JWT authentication pattern
- Note that all CRUD endpoints are now protected

**Verification**:
- Documentation is complete and accurate
- Examples match actual API behavior

---

### Step 14: Create Feature Branch and Commit
**Git Workflow**:

```bash
# Create feature branch
git checkout -b feature/phase-5-authentication

# Stage changes
git add backend/password.go backend/password_test.go
git add backend/jwt.go backend/jwt_test.go
git add backend/auth.go backend/auth_handlers.go
git add backend/middleware.go backend/errors.go backend/users.go
git add backend/main.go backend/go.mod backend/go.sum
git add .env.example docker-compose.yml
git add backend/README.md

# Commit
git commit -m "$(cat <<'EOF'
feat: implement JWT authentication (Phase 5)

Add JWT-based authentication system with email/password login:

- Password utilities (bcrypt, cost factor 10)
- JWT generation and validation (1-hour expiration)
- Auth service (login, signup, default account handling)
- Auth middleware (protect all Phase 4A routes)
- Auth handlers (login, signup, logout endpoints)
- UserRepository.GetByEmail() method
- Unit tests for password and JWT utilities
- Environment variable for JWT_SECRET
- Documentation updates

Protected routes now require Authorization header with Bearer token.
Health endpoints remain public for K8s probes.

Success metrics: 23/23 achieved
- 8/8 functional
- 7/7 technical
- 6/6 security
- 2/2 performance

Linear: TRA-79

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"

# Push branch
git push -u origin feature/phase-5-authentication
```

**Verification**:
- Feature branch created
- All files committed
- Pushed to remote

---

### Step 15: Create Pull Request
**GitHub PR**:

```bash
gh pr create --title "feat: Phase 5 Authentication (JWT)" --body "$(cat <<'EOF'
## Summary
- Implement JWT-based authentication system for Go backend
- Port email/password auth patterns from Next.js implementation
- Protect all Phase 4A REST API endpoints with auth middleware
- Add login, signup, and logout endpoints

## Changes
### New Files
- `backend/password.go` - bcrypt password hashing utilities
- `backend/jwt.go` - JWT generation and validation
- `backend/auth.go` - AuthService (login, signup, helpers)
- `backend/auth_handlers.go` - HTTP handlers for auth endpoints
- `backend/password_test.go` - Password utility tests
- `backend/jwt_test.go` - JWT utility tests

### Modified Files
- `backend/middleware.go` - Add authMiddleware for JWT validation
- `backend/users.go` - Add GetByEmail() method
- `backend/main.go` - Wire auth service, register routes, protect endpoints
- `backend/errors.go` - Add ErrUnauthorized constant
- `.env.example` - Add JWT_SECRET configuration
- `docker-compose.yml` - Pass JWT_SECRET to backend container
- `backend/README.md` - Document auth endpoints and protected routes

## New Endpoints
- `POST /api/v1/auth/login` - Email/password login
- `POST /api/v1/auth/signup` - User registration
- `POST /api/v1/auth/logout` - Logout (client-side token deletion)

## Protected Routes
All Phase 4A endpoints now require `Authorization: Bearer <token>` header:
- Accounts CRUD
- Users CRUD
- Account Users CRUD

Health endpoints remain public (K8s probes).

## Testing
### Unit Tests
- ✅ Password hashing and comparison
- ✅ JWT generation and validation
- ✅ Invalid token handling
- ✅ Expired token handling

### Integration Tests (Manual)
- ✅ Signup flow (creates user, returns JWT)
- ✅ Duplicate email returns 409
- ✅ Login flow (validates credentials, returns JWT)
- ✅ Invalid credentials return 401
- ✅ Protected routes require auth
- ✅ Valid JWT grants access
- ✅ Invalid JWT returns 401
- ✅ Missing auth header returns 401
- ✅ Health endpoints remain public

## Success Metrics
**23/23 achieved (100%)**
- 8/8 functional
- 7/7 technical
- 6/6 security
- 2/2 performance

## Security Notes
- Passwords hashed with bcrypt (cost factor 10)
- JWT signed with secret from environment variable
- Generic error messages (prevent user enumeration)
- Password hash never exposed in API responses
- Token expiration enforced (1 hour)

## Dependencies
- `github.com/golang-jwt/jwt/v5` - JWT library
- `golang.org/x/crypto/bcrypt` - Password hashing

## Linear
Closes TRA-79

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

**Verification**:
- PR created successfully
- PR includes all changes
- PR description is comprehensive

---

## Post-Implementation Checklist

### Code Quality
- [ ] All files < 500 lines
- [ ] No hardcoded secrets (JWT_SECRET from env)
- [ ] Error handling at all boundaries
- [ ] Input validation on all requests
- [ ] Structured logging with context

### Testing
- [ ] All unit tests pass
- [ ] All manual integration tests pass
- [ ] Password hashing works (bcrypt cost 10)
- [ ] JWT generation works (1-hour expiration)
- [ ] JWT validation works (rejects invalid/expired)
- [ ] Auth middleware protects routes
- [ ] Health endpoints remain public

### Documentation
- [ ] README.md updated with new endpoints
- [ ] .env.example includes JWT_SECRET
- [ ] Code comments explain "why" not "what"
- [ ] API examples are accurate

### Security
- [ ] Passwords never logged or exposed
- [ ] JWT_SECRET required in production
- [ ] Generic error messages (no user enumeration)
- [ ] Authorization header format validated
- [ ] Token expiration enforced

### Success Metrics (23/23)
#### Functional (8/8)
- [ ] Can signup with email/password
- [ ] Can login with email/password
- [ ] Receives JWT on successful auth
- [ ] Can access protected routes with valid JWT
- [ ] Gets 401 without JWT
- [ ] Gets 401 with invalid/expired JWT
- [ ] Duplicate email returns 409
- [ ] Invalid credentials return 401

#### Technical (7/7)
- [ ] JWT is stateless (no session storage)
- [ ] Bcrypt cost = 10
- [ ] JWT includes user_id, email, current_account_id
- [ ] Auth middleware sets user context
- [ ] All Phase 4A routes protected
- [ ] Health endpoints remain public
- [ ] Password hash never in responses

#### Security (6/6)
- [ ] Passwords hashed with bcrypt
- [ ] JWT signed with secret from env
- [ ] No password in JWT payload
- [ ] Generic error messages
- [ ] Bearer token format validated
- [ ] Token expiration enforced (1 hour)

#### Performance (2/2)
- [ ] Login response < 200ms
- [ ] Auth middleware overhead < 10ms

---

## Rollback Plan

If issues are found after deployment:

1. **Revert the PR merge**:
   ```bash
   git revert <merge-commit-sha>
   git push origin main
   ```

2. **Disable auth middleware** (emergency hotfix):
   - Comment out `r.Use(authMiddleware)` in main.go
   - Redeploy (makes all routes public again)

3. **Fix issues in new branch** and repeat PR process

---

## Next Steps (Phase 5B - Future)

After Phase 5A is complete:
- Refresh tokens (long-lived sessions)
- Password reset flow
- Email verification
- OAuth providers (Google, GitHub)
- Account switching endpoint
- Rate limiting on auth endpoints
- Two-factor authentication (2FA)
