# Feature: Phase 5B - Authentication Endpoints & Protected Routes

## Origin
This specification emerged from analyzing Linear ticket TRA-79 (Phase 5: Authentication) and determining what remains after shipping Phase 5A (JWT + password utilities foundation).

**Context from conversation:**
- Phase 5A shipped: JWT generation/validation + bcrypt password hashing (utilities only, no endpoints)
- Phase 4A shipped: REST API with Accounts, Users, AccountUsers CRUD endpoints (currently UNPROTECTED)
- TRA-79 scope: Port next-auth to Go, session management, protected routes
- Decision: One-shot implementation (~800 LOC) rather than splitting into 5B + 5C

## Outcome
Complete the authentication system by building on Phase 5A utilities to add:
- User signup and login endpoints that issue JWTs
- Auth middleware to protect existing REST API routes
- Integration tests proving the full auth flow works

**What changes:**
- Users can register via `/api/v1/auth/signup`
- Users can login via `/api/v1/auth/login`
- All Phase 4A endpoints (Accounts, Users, AccountUsers) require valid JWT
- Invalid/expired/missing JWT returns 401 Unauthorized
- TRA-79 is completed and can be closed

## User Stories

**As an API consumer**
I want to register for an account
So that I can access protected platform features

**As a registered user**
I want to login with my credentials
So that I receive a JWT token for API access

**As a backend developer**
I want all REST API endpoints protected by default
So that unauthorized users cannot access sensitive data

## Context

### What We Have (Phase 5A)
```go
// JWT utilities (backend/jwt.go)
GenerateJWT(userID int, email string, currentAccountID *int) (string, error)
ValidateJWT(tokenString string) (*JWTClaims, error)

// Password utilities (backend/password.go)
HashPassword(password string) (string, error)
ComparePassword(hashedPassword, password string) error
```

### What We're Missing
1. **Auth Service Layer** - Business logic for Login/Signup
2. **Auth Handlers** - HTTP endpoints that use the service
3. **Auth Middleware** - Extract/validate JWT from Authorization header
4. **UserRepository.GetByEmail()** - Lookup user by email for login
5. **Protected Routes** - Apply middleware to Phase 4A endpoints
6. **Integration Tests** - Prove the full flow works

### Current State
- REST API is wide open (anyone can call any endpoint)
- JWT utilities exist but are unused
- No way to authenticate users

### Desired State
- Users signup → receive JWT
- Users login → receive JWT
- Protected endpoints validate JWT → 200 OK or 401 Unauthorized
- All Phase 4A endpoints require authentication

## Technical Requirements

### 1. Auth Service Layer (~150 LOC)
**File:** `backend/auth_service.go`

**Interface:**
```go
type AuthService interface {
    Signup(ctx context.Context, req SignupRequest) (*AuthResponse, error)
    Login(ctx context.Context, req LoginRequest) (*AuthResponse, error)
}

type SignupRequest struct {
    Email       string `json:"email" validate:"required,email"`
    Password    string `json:"password" validate:"required,min=8"`
    AccountName string `json:"account_name" validate:"required,min=2"`
}

type LoginRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required"`
}

type AuthResponse struct {
    Token string    `json:"token"`
    User  UserDTO   `json:"user"`
}
```

**Responsibilities:**
- Validate signup request (email uniqueness, password strength)
- Create new user with hashed password (using Phase 5A `HashPassword`)
- Create default account for new user
- Link user to account via AccountUsers junction
- Generate JWT (using Phase 5A `GenerateJWT`)
- Validate login credentials (using Phase 5A `ComparePassword`)
- Lookup user by email (using new `UserRepository.GetByEmail()`)
- Return JWT + user data on success

**Error Handling:**
- 400 Bad Request - Validation errors
- 401 Unauthorized - Invalid credentials (login)
- 409 Conflict - Email already exists (signup)
- 500 Internal Server Error - Database/unexpected errors

### 2. Auth Handlers (~150 LOC)
**File:** `backend/auth.go`

**Endpoints:**
```
POST /api/v1/auth/signup
POST /api/v1/auth/login
```

**Request/Response Examples:**

**Signup:**
```bash
# Request
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "securepass123",
    "account_name": "My Company"
  }'

# Response (201 Created)
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user": {
      "id": 1,
      "email": "user@example.com",
      "created_at": "2025-10-18T14:30:00Z"
    }
  }
}
```

**Login:**
```bash
# Request
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "securepass123"
  }'

# Response (200 OK)
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user": {
      "id": 1,
      "email": "user@example.com",
      "current_account_id": 1
    }
  }
}
```

**Responsibilities:**
- Parse and validate JSON request bodies
- Call AuthService methods
- Return RFC 7807 error responses on failure
- Return 201 for signup, 200 for login
- Include request_id in all responses (from middleware)

### 3. Auth Middleware (~100 LOC)
**File:** `backend/middleware.go` (extend existing file)

**Function:**
```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract "Authorization: Bearer <token>" header
        // Validate JWT using Phase 5A ValidateJWT()
        // Inject claims into request context
        // Call next.ServeHTTP() on success
        // Return 401 on failure
    })
}
```

**Behavior:**
- Extract JWT from `Authorization: Bearer <token>` header
- Return 401 if header missing or malformed
- Validate JWT using `ValidateJWT()` from Phase 5A
- Return 401 if token invalid or expired
- Inject `JWTClaims` into request context (for use by handlers)
- Call next handler on success

**Context Key:**
```go
type contextKey string
const UserClaimsKey contextKey = "user_claims"

// In handlers, retrieve claims:
claims := r.Context().Value(UserClaimsKey).(*JWTClaims)
```

### 4. UserRepository Extension (~50 LOC)
**File:** `backend/users.go` (modify existing)

**Add Method:**
```go
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
    query := `SELECT id, email, password_hash, created_at, updated_at
              FROM users
              WHERE email = $1 AND deleted_at IS NULL`

    var user User
    err := r.db.QueryRow(ctx, query, email).Scan(...)
    if err == pgx.ErrNoRows {
        return nil, nil // Not found
    }
    return &user, err
}
```

**Used by:** AuthService.Login() to lookup user credentials

### 5. Protected Routes (~30 LOC)
**File:** `backend/main.go` (modify existing router setup)

**Current (Unprotected):**
```go
r.Route("/api/v1", func(r chi.Router) {
    r.Get("/accounts", listAccounts)
    r.Post("/accounts", createAccount)
    // ... all Phase 4A endpoints
})
```

**Updated (Protected):**
```go
r.Route("/api/v1", func(r chi.Router) {
    // Public auth endpoints (no middleware)
    r.Post("/auth/signup", signupHandler)
    r.Post("/auth/login", loginHandler)

    // Protected group (requires valid JWT)
    r.Group(func(r chi.Router) {
        r.Use(AuthMiddleware) // Apply to all routes in group

        // Accounts
        r.Get("/accounts", listAccounts)
        r.Get("/accounts/{id}", getAccount)
        r.Post("/accounts", createAccount)
        r.Put("/accounts/{id}", updateAccount)
        r.Delete("/accounts/{id}", deleteAccount)

        // Users
        r.Get("/users", listUsers)
        r.Get("/users/{id}", getUser)
        r.Post("/users", createUser)
        r.Put("/users/{id}", updateUser)
        r.Delete("/users/{id}", deleteUser)

        // AccountUsers
        r.Get("/accounts/{id}/users", listAccountUsers)
        r.Post("/accounts/{id}/users", addAccountUser)
        r.Put("/accounts/{account_id}/users/{user_id}", updateAccountUser)
        r.Delete("/accounts/{account_id}/users/{user_id}", removeAccountUser)
    })
})
```

### 6. Integration Tests (~300 LOC)
**File:** `backend/auth_integration_test.go`

**Test Cases:**

**Signup Flow:**
- ✅ Valid signup returns 201 + JWT + user data
- ✅ Duplicate email returns 409 Conflict
- ✅ Invalid email format returns 400
- ✅ Weak password (< 8 chars) returns 400
- ✅ Missing account_name returns 400
- ✅ Creates user record in database
- ✅ Creates account record in database
- ✅ Links user to account in account_users
- ✅ Password is hashed (not plaintext)

**Login Flow:**
- ✅ Valid credentials return 200 + JWT
- ✅ Invalid email returns 401
- ✅ Invalid password returns 401
- ✅ Non-existent user returns 401
- ✅ JWT contains correct user_id and email

**Protected Routes:**
- ✅ Request with valid JWT returns 200
- ✅ Request with missing JWT returns 401
- ✅ Request with malformed JWT returns 401
- ✅ Request with expired JWT returns 401
- ✅ Request with invalid signature returns 401

**Full Flow:**
- ✅ Signup → receive token → access protected endpoint → 200 OK
- ✅ Login → receive token → access protected endpoint → 200 OK
- ✅ No token → access protected endpoint → 401 Unauthorized

**Test Setup:**
- Use Docker test database (same as Phase 4A pattern)
- Run migrations before tests
- Clean up test data after each test
- Mock time for JWT expiration testing

## Architecture Patterns

### Follow Phase 4A Conventions
- **Repository Pattern** - Data access layer (UserRepository extension)
- **Service Layer** - Business logic (AuthService)
- **Handler Layer** - HTTP endpoints (auth.go)
- **RFC 7807 Errors** - Consistent error responses
- **Request ID Tracing** - Include in all responses
- **go-playground/validator** - Input validation

### New Patterns for Phase 5B
- **JWT Claims in Context** - Pass authenticated user to handlers
- **Middleware Composition** - Separate public vs protected routes
- **Stateless Sessions** - No server-side session storage

## Validation Criteria

### Functional Requirements
- [ ] Can signup with email, password, account name → receive JWT
- [ ] Can login with email, password → receive JWT
- [ ] Can access protected endpoint with valid JWT → 200 OK
- [ ] Cannot access protected endpoint without JWT → 401 Unauthorized
- [ ] Cannot access protected endpoint with expired JWT → 401 Unauthorized
- [ ] Cannot access protected endpoint with invalid JWT → 401 Unauthorized
- [ ] Duplicate email on signup → 409 Conflict
- [ ] Invalid credentials on login → 401 Unauthorized
- [ ] All Phase 4A endpoints require authentication

### Technical Requirements
- [ ] JWT generated using Phase 5A utilities
- [ ] Password hashed using Phase 5A utilities
- [ ] User lookup works via UserRepository.GetByEmail()
- [ ] Auth middleware extracts claims into context
- [ ] Handlers can access authenticated user from context
- [ ] All errors follow RFC 7807 format
- [ ] All responses include request_id

### Testing Requirements
- [ ] All integration tests pass (signup, login, protected routes)
- [ ] `just backend` validates successfully (lint, test, build)
- [ ] Manual testing via curl shows expected behavior
- [ ] No password_hash fields in JSON responses (security check)

### Code Quality
- [ ] No file exceeds 500 lines
- [ ] All public functions have comments
- [ ] Error messages are clear and actionable
- [ ] Consistent naming with Phase 4A patterns
- [ ] No hardcoded secrets (use environment variables)

## Success Metrics

**Completion Criteria:**
- ✅ TRA-79 can be closed (Phase 5: Authentication complete)
- ✅ Platform API is fully secured
- ✅ Users can register and login
- ✅ Integration tests prove end-to-end auth flow
- ✅ Ready for Phase 6 (Serve Frontend Assets)

**Files Changed (Estimated):**
- Created: `backend/auth_service.go` (~150 LOC)
- Created: `backend/auth.go` (~150 LOC)
- Created: `backend/auth_integration_test.go` (~300 LOC)
- Modified: `backend/middleware.go` (+100 LOC)
- Modified: `backend/users.go` (+50 LOC)
- Modified: `backend/main.go` (+30 LOC)

**Total: ~780 LOC** (within ~800 LOC target)

## Out of Scope (Phase 6+)

**Not included in Phase 5B:**
- Token refresh mechanism (`POST /api/v1/auth/refresh`)
- Logout endpoint (`POST /api/v1/auth/logout`)
- Token blacklist/revocation
- Password reset flow
- Email verification
- OAuth integration
- Pre-auth public demo routes (per epic two-tier model)
- Frontend integration (happens in Phase 6)

**Rationale:** Keep scope focused on core auth system. Additional features can be added incrementally after Phase 6.

## Conversation References

**Key Insight:**
> "we split linear tra-79 into 3 sub phases i should have made a note or updated the linear ticket with the split"

**Decision:**
> "yea ~800 loc is manageable we can one-shot."

**Analysis:**
> "Phase 5A shipped the utilities but the API is still wide open. Phase 5B completes TRA-79 by actually using those utilities to secure the platform."

**Options Considered:**
- Split into 5B (endpoints) + 5C (middleware) → Rejected as incomplete
- One-shot 5B (~800 LOC) → Accepted as cohesive and manageable
