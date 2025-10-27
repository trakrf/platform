# Feature: Backend Auth - Align with Organizations Schema

**Linear Issue**: TRA-94
**Priority**: Urgent (Blocks TRA-90)
**Status**: Ready for Planning

## Origin

This specification addresses a critical gap discovered during frontend auth development: the backend auth service still references the old `accounts` and `account_users` tables, which were renamed to `organizations` and `org_users` in the database schema refactor (merged via TRA-88/PR #27).

## Problem Statement

**Current State**: Backend auth endpoints are **broken**
- Signup endpoint tries to INSERT into `trakrf.accounts` (table no longer exists)
- Signup endpoint tries to INSERT into `trakrf.account_users` (table no longer exists)
- Login endpoint tries to SELECT from `trakrf.account_users` (table no longer exists)
- API contract uses `account_name` field instead of `org_name`

**Impact**:
- All signup attempts fail with database error
- All login attempts fail with database error
- Frontend cannot test auth flows
- TRA-90 (Frontend Auth Screens) is blocked

## Outcome

After implementation:
- Backend auth service uses correct schema (`organizations`, `org_users`)
- Signup endpoint creates records in correct tables
- Login endpoint queries correct tables
- API contract uses `org_name` field (aligned with schema refactor)
- Frontend can successfully test signup/login flows
- TRA-90 unblocked

## User Story

**As a** frontend developer working on auth screens
**I want** the backend signup/login endpoints to work correctly
**So that** I can test and verify the complete authentication flow

**Acceptance Criteria**:
- Signup request with `org_name` creates organization in `organizations` table
- Signup request creates user-org link in `org_users` table
- Login request queries `org_users` table successfully
- No references to `accounts` or `account_users` remain in auth code
- Manual API testing passes for both endpoints

## Context

### Discovery
While preparing the frontend auth spec (TRA-90), we discovered the backend code was not updated during the database schema refactor. The migrations renamed tables, but the application code still references the old names.

### Root Cause
The schema refactor (TRA-88) focused on database migrations and created comprehensive documentation (`docs/schema-refactor-reference.md`), but the backend Go code was not updated in the same PR. This created a disconnect between schema and application code.

### Affected Files (9 files)
**Models** (2 files):
- `backend/internal/models/organization/organization.go` - doesn't exist, need to create
- `backend/internal/models/auth/auth.go` - uses `AccountName` / `account_name`

**Services** (1 file):
- `backend/internal/services/auth/auth.go` - queries old tables

**Storage** (2 files):
- `backend/internal/storage/accounts.go` - queries `accounts` table
- `backend/internal/storage/account_users.go` - queries `account_users` table

**Handlers** (4 files):
- `backend/internal/handlers/auth/auth.go` - error messages reference "account"
- `backend/internal/handlers/accounts/accounts.go` - entire package needs renaming
- `backend/internal/handlers/account_users/account_users.go` - entire package needs renaming
- `backend/main.go` - may have imports to update

## Technical Requirements

### 1. Create Organization Model

**File**: `backend/internal/models/organization/organization.go` (new file)

**Requirements**:
- Struct matches `organizations` table schema (migration 000002)
- Fields: `ID`, `Name`, `Domain`, `Metadata`, `ValidFrom`, `ValidTo`, `IsActive`, `CreatedAt`, `UpdatedAt`, `DeletedAt`
- Remove fields from old model: `Status`, `SubscriptionTier`, `MaxUsers`, `MaxStorageGB`, `BillingEmail`, `TechnicalEmail`, `Settings`
- Use appropriate Go types: `int` for ID, `string` for Name/Domain, `map[string]any` for Metadata
- Include JSON tags matching database column names

**Reference**: See `database/migrations/000002_organizations.up.sql` for exact schema

### 2. Update Auth Request Model

**File**: `backend/internal/models/auth/auth.go`

**Current**:
```go
type SignupRequest struct {
    Email       string `json:"email" validate:"required,email"`
    Password    string `json:"password" validate:"required,min=8"`
    AccountName string `json:"account_name" validate:"required,min=2"`
}
```

**Required**:
```go
type SignupRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
    OrgName  string `json:"org_name" validate:"required,min=2"`
}
```

**Changes**:
- Rename struct field: `AccountName` → `OrgName`
- Update JSON tag: `account_name` → `org_name`
- Validation rules unchanged (still `required,min=2`)

### 3. Update Auth Service

**File**: `backend/internal/services/auth/auth.go`

**Import Changes**:
```go
// Remove
import "github.com/trakrf/platform/backend/internal/models/account"

// Add
import "github.com/trakrf/platform/backend/internal/models/organization"
```

**Signup Method Updates** (lines 29-100):

**Variable naming**:
- Line 36: `request.AccountName` → `request.OrgName`
- Line 60: `var acct account.Account` → `var org organization.Organization`

**INSERT query** (lines 61-76):
```go
// Current (broken)
accountQuery := `
    INSERT INTO trakrf.accounts (name, domain, billing_email, subscription_tier, max_users, max_storage_gb)
    VALUES ($1, $2, $3, 'free', 5, 1)
    RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
              settings, metadata, billing_email, technical_email, created_at, updated_at
`
err = tx.QueryRow(ctx, accountQuery, request.AccountName, domain, request.Email).Scan(...)

// Required (working)
orgQuery := `
    INSERT INTO trakrf.organizations (name, domain)
    VALUES ($1, $2)
    RETURNING id, name, domain, metadata, valid_from, valid_to, is_active, created_at, updated_at
`
err = tx.QueryRow(ctx, orgQuery, request.OrgName, domain).Scan(
    &org.ID, &org.Name, &org.Domain, &org.Metadata, &org.ValidFrom, &org.ValidTo,
    &org.IsActive, &org.CreatedAt, &org.UpdatedAt)
```

**Key changes**:
- Table name: `trakrf.accounts` → `trakrf.organizations`
- Remove columns: `billing_email`, `subscription_tier`, `max_users`, `max_storage_gb`
- Remove RETURNING: `status`, `subscription_tier`, `max_users`, `max_storage_gb`, `settings`, `billing_email`, `technical_email`
- Add RETURNING: `valid_from`, `valid_to`, `is_active`
- Scan into new struct fields

**Link user to org** (lines 78-85):
```go
// Current (broken)
accountUserQuery := `
    INSERT INTO trakrf.account_users (account_id, user_id, role, status)
    VALUES ($1, $2, 'owner', 'active')
`
_, err = tx.Exec(ctx, accountUserQuery, acct.ID, usr.ID)

// Required (working)
orgUserQuery := `
    INSERT INTO trakrf.org_users (org_id, user_id, role)
    VALUES ($1, $2, 'owner')
`
_, err = tx.Exec(ctx, orgUserQuery, org.ID, usr.ID)
```

**Key changes**:
- Table name: `trakrf.account_users` → `trakrf.org_users`
- Column name: `account_id` → `org_id`
- Remove column: `status` (not in new schema)
- Variable name: `acct.ID` → `org.ID`

**JWT generation** (line 91):
```go
// Current
token, err := generateJWT(usr.ID, usr.Email, &acct.ID)

// Required
token, err := generateJWT(usr.ID, usr.Email, &org.ID)
```

**Login Method Updates** (lines 102-143):

**Query org_users** (lines 118-128):
```go
// Current (broken)
accountUserQuery := `
    SELECT account_id
    FROM trakrf.account_users
    WHERE user_id = $1 AND deleted_at IS NULL
    LIMIT 1
`
var accountID int
err = s.db.QueryRow(ctx, accountUserQuery, usr.ID).Scan(&accountID)

// Required (working)
orgUserQuery := `
    SELECT org_id
    FROM trakrf.org_users
    WHERE user_id = $1 AND deleted_at IS NULL
    LIMIT 1
`
var orgID int
err = s.db.QueryRow(ctx, orgUserQuery, usr.ID).Scan(&orgID)
```

**JWT generation** (lines 130-134):
```go
// Current
var accountIDPtr *int
if accountID != 0 {
    accountIDPtr = &accountID
}
token, err := generateJWT(usr.ID, usr.Email, accountIDPtr)

// Required
var orgIDPtr *int
if orgID != 0 {
    orgIDPtr = &orgID
}
token, err := generateJWT(usr.ID, usr.Email, orgIDPtr)
```

**Function rename** (line 145):
```go
// Current
func slugifyAccountName(name string) string { ... }

// Required
func slugifyOrgName(name string) string { ... }
```

**Error messages** (lines 73, 84):
```go
// Current
return nil, fmt.Errorf("account name already taken")
return nil, fmt.Errorf("failed to link user to account: %w", err)

// Required
return nil, fmt.Errorf("organization name already taken")
return nil, fmt.Errorf("failed to link user to organization: %w", err)
```

### 4. Update Auth Handler

**File**: `backend/internal/handlers/auth/auth.go`

**Error message** (lines 62-64):
```go
// Current
if strings.Contains(errMsg, "account name already taken") {
    httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
        "Account name already taken", "", middleware.GetRequestID(r.Context()))
    return
}

// Required
if strings.Contains(errMsg, "organization name already taken") {
    httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
        "Organization name already taken", "", middleware.GetRequestID(r.Context()))
    return
}
```

**Swagger comment** (line 37):
```go
// Current
// @Failure 409 {object} errors.ErrorResponse "Email or account name already exists"

// Required
// @Failure 409 {object} errors.ErrorResponse "Email or organization name already exists"
```

### 5. Update Storage Layer

**File**: `backend/internal/storage/accounts.go` → **Rename to**: `organizations.go`

**Requirements**:
- Rename file
- Update all queries to use `trakrf.organizations` table
- Update struct references: `account.Account` → `organization.Organization`
- Update function names: `GetAccount*` → `GetOrganization*`, `CreateAccount` → `CreateOrganization`, etc.
- Update error messages

**File**: `backend/internal/storage/account_users.go` → **Rename to**: `org_users.go`

**Requirements**:
- Rename file
- Update all queries to use `trakrf.org_users` table
- Update column references: `account_id` → `org_id`
- Update function names: `GetAccountUser*` → `GetOrgUser*`, etc.
- Update error messages

### 6. Update Handler Packages (If Exist)

**Directory**: `backend/internal/handlers/accounts/` → **Rename to**: `organizations/`

**Requirements**:
- Rename directory
- Update all queries to use `trakrf.organizations` table
- Update imports throughout codebase
- Update route registration in `main.go`

**Directory**: `backend/internal/handlers/account_users/` → **Rename to**: `org_users/`

**Requirements**:
- Rename directory
- Update all queries to use `trakrf.org_users` table
- Update column references: `account_id` → `org_id`
- Update imports throughout codebase
- Update route registration in `main.go`

### 7. Update Main Registration (If Needed)

**File**: `backend/main.go`

**Requirements**:
- Check for handler registrations referencing old package names
- Update imports if packages were renamed
- Verify all routes still registered correctly

## Code Examples

### Organization Model Structure

```go
// backend/internal/models/organization/organization.go
package organization

import "time"

type Organization struct {
    ID        int                    `json:"id"`
    Name      string                 `json:"name"`
    Domain    string                 `json:"domain"`
    Metadata  map[string]interface{} `json:"metadata"`
    ValidFrom time.Time              `json:"valid_from"`
    ValidTo   *time.Time             `json:"valid_to,omitempty"`
    IsActive  bool                   `json:"is_active"`
    CreatedAt time.Time              `json:"created_at"`
    UpdatedAt time.Time              `json:"updated_at"`
    DeletedAt *time.Time             `json:"deleted_at,omitempty"`
}
```

### Signup Flow (Corrected)

```go
// Simplified signup flow with correct table names
func (s *Service) Signup(ctx context.Context, request auth.SignupRequest, ...) (*auth.AuthResponse, error) {
    // 1. Hash password
    passwordHash, err := hashPassword(request.Password)

    // 2. Generate domain slug
    domain := slugifyOrgName(request.OrgName)

    // 3. Begin transaction
    tx, err := s.db.Begin(ctx)
    defer tx.Rollback(ctx)

    // 4. Create user
    userQuery := `INSERT INTO trakrf.users (...) VALUES (...) RETURNING ...`
    err = tx.QueryRow(ctx, userQuery, request.Email, ...).Scan(&usr.ID, ...)

    // 5. Create organization (CORRECTED)
    orgQuery := `
        INSERT INTO trakrf.organizations (name, domain)
        VALUES ($1, $2)
        RETURNING id, name, domain, metadata, valid_from, valid_to, is_active, created_at, updated_at
    `
    var org organization.Organization
    err = tx.QueryRow(ctx, orgQuery, request.OrgName, domain).Scan(
        &org.ID, &org.Name, &org.Domain, &org.Metadata,
        &org.ValidFrom, &org.ValidTo, &org.IsActive,
        &org.CreatedAt, &org.UpdatedAt)

    // 6. Link user to organization (CORRECTED)
    orgUserQuery := `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, 'owner')
    `
    _, err = tx.Exec(ctx, orgUserQuery, org.ID, usr.ID)

    // 7. Commit transaction
    err = tx.Commit(ctx)

    // 8. Generate JWT
    token, err := generateJWT(usr.ID, usr.Email, &org.ID)

    return &auth.AuthResponse{Token: token, User: usr}, nil
}
```

## Validation Criteria

### Build Validation
- [ ] Backend builds without errors: `cd backend && go build`
- [ ] No compile errors from missing types or packages
- [ ] All imports resolve correctly

### Code Search Validation
- [ ] No references to `trakrf.accounts` in `backend/` directory
- [ ] No references to `trakrf.account_users` in `backend/` directory
- [ ] No references to `models/account` import in auth code
- [ ] No `account_name` JSON tags in auth models

### Manual API Testing

**Signup Test**:
```bash
# Start backend
cd backend && go run .

# Test signup with org_name field
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "org_name": "Test Organization"
  }'

# Expected response (201 Created):
{
  "data": {
    "token": "eyJhbGc...",
    "user": {
      "id": 1,
      "email": "test@example.com",
      "name": "test@example.com",
      "created_at": "2025-10-27T00:00:00Z",
      "updated_at": "2025-10-27T00:00:00Z"
    }
  }
}
```

**Login Test**:
```bash
# Test login with previously created account
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'

# Expected response (200 OK):
{
  "data": {
    "token": "eyJhbGc...",
    "user": {
      "id": 1,
      "email": "test@example.com",
      "name": "test@example.com",
      ...
    }
  }
}
```

**Database Verification**:
```bash
# Connect to database
just db-shell

# Verify organization created
SELECT id, name, domain, is_active, created_at FROM trakrf.organizations;

# Expected output:
#  id  |       name        |      domain       | is_active |         created_at
# -----+-------------------+-------------------+-----------+----------------------------
#  123 | Test Organization | test-organization | t         | 2025-10-27 00:00:00+00

# Verify user-org link created
SELECT org_id, user_id, role FROM trakrf.org_users;

# Expected output:
#  org_id | user_id | role
# --------+---------+-------
#     123 |       1 | owner
```

### Error Handling Tests

**Duplicate email**:
```bash
# Attempt to signup with existing email
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "newpassword",
    "org_name": "Another Org"
  }'

# Expected response (409 Conflict):
{
  "error": "Email already exists"
}
```

**Duplicate organization name**:
```bash
# Attempt to signup with existing org name
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "another@example.com",
    "password": "password123",
    "org_name": "Test Organization"
  }'

# Expected response (409 Conflict):
{
  "error": "Organization name already taken"
}
```

**Wrong password**:
```bash
# Attempt login with wrong password
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "wrongpassword"
  }'

# Expected response (401 Unauthorized):
{
  "error": "Invalid email or password"
}
```

## Implementation Notes

### Schema Reference

**Always consult**: `docs/schema-refactor-reference.md` for exact schema changes
- Organizations table definition: migration `000002_organizations.up.sql`
- Org_users table definition: migration `000004_org_users.up.sql`

### Field Mapping

**Removed fields** (do not include):
- `status` - Not in new schema (use `is_active` instead)
- `subscription_tier` - Not in new schema (future: metadata)
- `max_users` - Not in new schema (future: metadata)
- `max_storage_gb` - Not in new schema (future: metadata)
- `billing_email` - Not in new schema (future: metadata)
- `technical_email` - Not in new schema (future: metadata)
- `settings` - Not in new schema (use `metadata` for flexible fields)

**Renamed fields**:
- `account_id` → `org_id` (in org_users table)
- `account_name` → `org_name` (in API contract)

**New fields** (include in queries):
- `valid_from` - Temporal versioning start
- `valid_to` - Temporal versioning end (nullable)
- `is_active` - Active status boolean

### Transaction Safety

The signup flow uses a transaction to ensure atomicity:
1. Insert user
2. Insert organization
3. Link user to organization (org_users)

If ANY step fails, ALL changes are rolled back. This prevents orphaned users or organizations.

### Domain Slugification

The `slugifyOrgName` function converts organization names to URL-safe slugs:
- Input: `"Test Organization"`
- Output: `"test-organization"`
- Used for `domain` field (e.g., `test-organization.trakrf.com`)

**Rules**:
- Lowercase
- Replace non-alphanumeric with hyphens
- Trim leading/trailing hyphens

## Dependencies

**Requires**:
- TRA-88 (Database Schema Refactor) - ✅ Complete (merged PR #27)
- Database migrations applied (`just db-migrate`)
- `organizations` and `org_users` tables exist

**Blocks**:
- TRA-90 (Frontend Auth - Login & Signup Screens)
- Any future frontend auth work

## Files Changed Summary

```
backend/
  internal/
    models/
      organization/
        organization.go                (new - create organization model)
      auth/
        auth.go                        (update - AccountName → OrgName)
    services/
      auth/
        auth.go                        (update - use organizations/org_users tables)
    storage/
      organizations.go                 (rename from accounts.go)
      org_users.go                     (rename from account_users.go)
    handlers/
      auth/
        auth.go                        (update - error messages)
      organizations/                   (rename from accounts/)
        organizations.go
      org_users/                       (rename from account_users/)
        org_users.go
  main.go                              (update - imports if handlers renamed)
```

**Estimated changes**: 9 files modified/renamed + 1 file created

## Next Steps

1. Review and confirm this specification
2. Run: `/plan spec/active/backend-auth-org-alignment/spec.md`
3. Implement changes
4. Manual API testing per validation criteria
5. Verify database records created correctly
6. Update TRA-90 status to unblocked
7. Proceed with frontend auth screens (TRA-90)
