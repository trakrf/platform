# Implementation Plan: Backend Auth - Align with Organizations Schema

**Generated**: 2025-10-27T00:20:00Z
**Specification**: spec.md
**Linear Issue**: TRA-94
**Complexity**: 8/10 (HIGH - atomic refactor)

## Understanding

This is a critical fix for backend auth endpoints that currently fail because they reference deleted database tables (`accounts`, `account_users`). The database schema refactor (TRA-88) renamed these to `organizations` and `org_users`, but the Go code wasn't updated in the same PR.

**Core Issue**: Mechanical rename + SQL query updates
- Most changes are straightforward `account` → `organization` renames
- Critical non-mechanical parts: SQL queries need field changes (remove old fields, add new fields)
- JWT claims reference `current_account_id` - needs updating for go-forward correctness

**Validation Strategy**: Build frequently, test signup/login endpoints with curl, verify database writes

## Relevant Files

### Reference Patterns

**Migration schemas** (source of truth):
- `database/migrations/000002_organizations.up.sql` - Organizations table schema
- `database/migrations/000004_org_users.up.sql` - Org_users table schema
- `docs/schema-refactor-reference.md` - Complete refactor documentation

**Existing model to reference**:
- `backend/internal/models/account/account.go` (lines 10-24) - Old Account struct (shows what to remove)

### Files to Create

- `backend/internal/models/organization/organization.go` - New Organization model matching new schema

### Files to Rename

**Storage layer**:
- `backend/internal/storage/accounts.go` → `organizations.go`
- `backend/internal/storage/account_users.go` → `org_users.go`

**Handler packages**:
- `backend/internal/handlers/accounts/` → `organizations/`
  - `accounts.go` → `organizations.go`
  - `accounts_test.go` → `organizations_test.go`
- `backend/internal/handlers/account_users/` → `org_users/`
  - `account_users.go` → `org_users.go`
  - `account_users_test.go` → `org_users_test.go`

### Files to Modify

**Models**:
- `backend/internal/models/auth/auth.go` - Update SignupRequest (AccountName → OrgName)

**Services**:
- `backend/internal/services/auth/auth.go` - Update imports, SQL queries, variable names

**Handlers**:
- `backend/internal/handlers/auth/auth.go` - Update error messages

**JWT utilities**:
- `backend/internal/util/jwt/jwt.go` - Update Claims struct (CurrentAccountID → CurrentOrgID)
- `backend/internal/util/jwt/jwt_test.go` - Update test claims

**Main**:
- `backend/main.go` - Update imports if handler packages renamed

## Architecture Impact

- **Subsystems affected**: Models, Services, Storage, Handlers, JWT utilities, Main
- **New dependencies**: None
- **Breaking changes**: API contract changes `account_name` → `org_name` (pre-launch, acceptable)

## Task Breakdown

### Task 1: Create Organization Model

**File**: `backend/internal/models/organization/organization.go`
**Action**: CREATE
**Pattern**: Reference `database/migrations/000002_organizations.up.sql` for exact schema

**Implementation**:
```go
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

**Key differences from old Account model**:
- Removed: Status, SubscriptionTier, MaxUsers, MaxStorageGB, BillingEmail, TechnicalEmail, Settings
- Added: ValidFrom, ValidTo, IsActive
- Kept: Metadata (JSONB for flexibility)

**Validation**:
```bash
cd backend && go build ./internal/models/organization
```

---

### Task 2: Update Auth Request Model

**File**: `backend/internal/models/auth/auth.go`
**Action**: MODIFY
**Pattern**: Simple field rename

**Changes**:
- Line 9: `AccountName string` → `OrgName string`
- Line 9: JSON tag `account_name` → `org_name`

**Before**:
```go
type SignupRequest struct {
    Email       string `json:"email" validate:"required,email"`
    Password    string `json:"password" validate:"required,min=8"`
    AccountName string `json:"account_name" validate:"required,min=2"`
}
```

**After**:
```go
type SignupRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
    OrgName  string `json:"org_name" validate:"required,min=2"`
}
```

**Validation**:
```bash
cd backend && go build ./internal/models/auth
```

---

### Task 3: Update Auth Service

**File**: `backend/internal/services/auth/auth.go`
**Action**: MODIFY
**Pattern**: Update imports, variable names, SQL queries

**3a. Update imports** (line 10):
```go
// Remove
import "github.com/trakrf/platform/backend/internal/models/account"

// Add
import "github.com/trakrf/platform/backend/internal/models/organization"
```

**3b. Signup method - Update variable references**:
- Line 36: `request.AccountName` → `request.OrgName`
- Line 60: `var acct account.Account` → `var org organization.Organization`

**3c. Signup method - Update organization INSERT query** (lines 61-76):

**Before**:
```go
accountQuery := `
    INSERT INTO trakrf.accounts (name, domain, billing_email, subscription_tier, max_users, max_storage_gb)
    VALUES ($1, $2, $3, 'free', 5, 1)
    RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
              settings, metadata, billing_email, technical_email, created_at, updated_at
`
err = tx.QueryRow(ctx, accountQuery, request.AccountName, domain, request.Email).Scan(
    &acct.ID, &acct.Name, &acct.Domain, &acct.Status, &acct.SubscriptionTier,
    &acct.MaxUsers, &acct.MaxStorageGB, &acct.Settings, &acct.Metadata,
    &acct.BillingEmail, &acct.TechnicalEmail, &acct.CreatedAt, &acct.UpdatedAt)
```

**After**:
```go
orgQuery := `
    INSERT INTO trakrf.organizations (name, domain)
    VALUES ($1, $2)
    RETURNING id, name, domain, metadata, valid_from, valid_to, is_active, created_at, updated_at
`
err = tx.QueryRow(ctx, orgQuery, request.OrgName, domain).Scan(
    &org.ID, &org.Name, &org.Domain, &org.Metadata,
    &org.ValidFrom, &org.ValidTo, &org.IsActive,
    &org.CreatedAt, &org.UpdatedAt)
```

**Key changes**:
- Table: `trakrf.accounts` → `trakrf.organizations`
- Remove INSERT columns: `billing_email`, `subscription_tier`, `max_users`, `max_storage_gb`
- Remove RETURNING: `status`, `subscription_tier`, `max_users`, `max_storage_gb`, `settings`, `billing_email`, `technical_email`
- Add RETURNING: `valid_from`, `valid_to`, `is_active`
- VALUES: Only `$1, $2` (name, domain) - removed billing_email and tier defaults

**3d. Signup method - Update org_users INSERT query** (lines 78-85):

**Before**:
```go
accountUserQuery := `
    INSERT INTO trakrf.account_users (account_id, user_id, role, status)
    VALUES ($1, $2, 'owner', 'active')
`
_, err = tx.Exec(ctx, accountUserQuery, acct.ID, usr.ID)
```

**After**:
```go
orgUserQuery := `
    INSERT INTO trakrf.org_users (org_id, user_id, role)
    VALUES ($1, $2, 'owner')
`
_, err = tx.Exec(ctx, orgUserQuery, org.ID, usr.ID)
```

**Key changes**:
- Table: `trakrf.account_users` → `trakrf.org_users`
- Column: `account_id` → `org_id`
- Remove `status` column (will use default 'active' from schema)
- Variable: `acct.ID` → `org.ID`

**3e. Signup method - Update JWT generation** (line 91):
```go
// Before
token, err := generateJWT(usr.ID, usr.Email, &acct.ID)

// After
token, err := generateJWT(usr.ID, usr.Email, &org.ID)
```

**3f. Signup method - Update error messages**:
- Line 73: `"account name already taken"` → `"organization name already taken"`
- Line 84: `"failed to link user to account"` → `"failed to link user to organization"`

**3g. Login method - Update org_users query** (lines 118-128):

**Before**:
```go
accountUserQuery := `
    SELECT account_id
    FROM trakrf.account_users
    WHERE user_id = $1 AND deleted_at IS NULL
    LIMIT 1
`
var accountID int
err = s.db.QueryRow(ctx, accountUserQuery, usr.ID).Scan(&accountID)
```

**After**:
```go
orgUserQuery := `
    SELECT org_id
    FROM trakrf.org_users
    WHERE user_id = $1 AND deleted_at IS NULL
    LIMIT 1
`
var orgID int
err = s.db.QueryRow(ctx, orgUserQuery, usr.ID).Scan(&orgID)
```

**3h. Login method - Update JWT generation** (lines 130-134):

**Before**:
```go
var accountIDPtr *int
if accountID != 0 {
    accountIDPtr = &accountID
}
token, err := generateJWT(usr.ID, usr.Email, accountIDPtr)
```

**After**:
```go
var orgIDPtr *int
if orgID != 0 {
    orgIDPtr = &orgID
}
token, err := generateJWT(usr.ID, usr.Email, orgIDPtr)
```

**3i. Rename function** (line 145):
```go
// Before
func slugifyAccountName(name string) string { ... }

// After
func slugifyOrgName(name string) string { ... }
```

**Validation**:
```bash
cd backend && go build ./internal/services/auth
```

---

### Task 4: Update Auth Handler

**File**: `backend/internal/handlers/auth/auth.go`
**Action**: MODIFY
**Pattern**: Update error messages and swagger comments

**4a. Update error message** (lines 62-64):
```go
// Before
if strings.Contains(errMsg, "account name already taken") {
    httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
        "Account name already taken", "", middleware.GetRequestID(r.Context()))
    return
}

// After
if strings.Contains(errMsg, "organization name already taken") {
    httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
        "Organization name already taken", "", middleware.GetRequestID(r.Context()))
    return
}
```

**4b. Update swagger comment** (line 37):
```go
// Before
// @Failure 409 {object} errors.ErrorResponse "Email or account name already exists"

// After
// @Failure 409 {object} errors.ErrorResponse "Email or organization name already exists"
```

**Validation**:
```bash
cd backend && go build ./internal/handlers/auth
```

---

### Task 5: Update JWT Utilities

**File**: `backend/internal/util/jwt/jwt.go`
**Action**: MODIFY
**Pattern**: Update Claims struct field name and JSON tag

**5a. Update Claims struct** (line 15):
```go
// Before
type Claims struct {
    UserID           int    `json:"user_id"`
    Email            string `json:"email"`
    CurrentAccountID *int   `json:"current_account_id,omitempty"`
    jwt.RegisteredClaims
}

// After
type Claims struct {
    UserID       int    `json:"user_id"`
    Email        string `json:"email"`
    CurrentOrgID *int   `json:"current_org_id,omitempty"`
    jwt.RegisteredClaims
}
```

**5b. Update Generate function** (lines 20, 27):
```go
// Before
func Generate(userID int, email string, accountID *int) (string, error) {
    // ...
    claims := &Claims{
        UserID:           userID,
        Email:            email,
        CurrentAccountID: accountID,
        // ...
    }
}

// After
func Generate(userID int, email string, orgID *int) (string, error) {
    // ...
    claims := &Claims{
        UserID:       userID,
        Email:        email,
        CurrentOrgID: orgID,
        // ...
    }
}
```

**File**: `backend/internal/util/jwt/jwt_test.go`
**Action**: MODIFY
**Pattern**: Update test claims to use CurrentOrgID

Update any test cases that reference `CurrentAccountID` to use `CurrentOrgID`.

**Validation**:
```bash
cd backend && go test ./internal/util/jwt
```

---

### Task 6: Rename and Update Storage Layer

**6a. Rename file**: `backend/internal/storage/accounts.go` → `organizations.go`

**6b. Update organizations.go**:
- Update package comment if exists
- Replace import: `"github.com/trakrf/platform/backend/internal/models/account"` → `"github.com/trakrf/platform/backend/internal/models/organization"`
- Global replace: `account.Account` → `organization.Organization`
- Global replace: `trakrf.accounts` → `trakrf.organizations`
- Rename functions: `ListAccounts` → `ListOrganizations`, `GetAccount` → `GetOrganization`, etc.
- Update all variable names: `acct` → `org`, `accounts` → `organizations`
- Update SQL queries to match new schema (remove old fields, add valid_from/valid_to/is_active)
- Update error messages: "account" → "organization"

**6c. Rename file**: `backend/internal/storage/account_users.go` → `org_users.go`

**6d. Update org_users.go**:
- Update package comment if exists
- Global replace: `trakrf.account_users` → `trakrf.org_users`
- Global replace: `account_id` → `org_id`
- Rename functions: `GetAccountUser*` → `GetOrgUser*`, etc.
- Update error messages: "account user" → "org user"

**Validation**:
```bash
cd backend && go build ./internal/storage
```

---

### Task 7: Rename and Update Handler Packages

**7a. Rename directory**: `backend/internal/handlers/accounts/` → `organizations/`

**7b. Rename file**: `organizations/accounts.go` → `organizations.go`

**7c. Update organizations.go**:
- Line 1: `package accounts` → `package organizations`
- Update import: `"github.com/trakrf/platform/backend/internal/models/account"` → `"github.com/trakrf/platform/backend/internal/models/organization"`
- Global replace: `account.Account` → `organization.Organization`
- Update swagger tags: `@Tags accounts` → `@Tags organizations`
- Update swagger router: `@Router /api/v1/accounts` → `@Router /api/v1/organizations`
- Update function comments: "accounts" → "organizations"

**7d. Rename file**: `organizations/accounts_test.go` → `organizations_test.go`

**7e. Update organizations_test.go**:
- Line 1: `package accounts` → `package organizations`
- Update test names and assertions to reference organizations

**7f. Rename directory**: `backend/internal/handlers/account_users/` → `org_users/`

**7g. Rename file**: `org_users/account_users.go` → `org_users.go`

**7h. Update org_users.go**:
- Line 1: `package account_users` → `package org_users`
- Update swagger tags: `@Tags account_users` → `@Tags org_users`
- Update swagger router: `@Router /api/v1/account_users` → `@Router /api/v1/org_users`
- Global replace: `account_id` → `org_id`
- Update function comments

**7i. Rename file**: `org_users/account_users_test.go` → `org_users_test.go`

**7j. Update org_users_test.go**:
- Line 1: `package account_users` → `package org_users`
- Update test names and assertions

**Validation**:
```bash
cd backend && go build ./internal/handlers/organizations
cd backend && go build ./internal/handlers/org_users
```

---

### Task 8: Update Main Registration

**File**: `backend/main.go`
**Action**: MODIFY
**Pattern**: Update imports and route registrations

**8a. Update imports**:
```go
// Before
import (
    accountshandler "github.com/trakrf/platform/backend/internal/handlers/accounts"
    accountusershandler "github.com/trakrf/platform/backend/internal/handlers/account_users"
)

// After
import (
    organizationshandler "github.com/trakrf/platform/backend/internal/handlers/organizations"
    orgusershandler "github.com/trakrf/platform/backend/internal/handlers/org_users"
)
```

**8b. Update handler initialization and registration**:
```go
// Before
accountsHandler := accountshandler.NewHandler(storage)
accountUsersHandler := accountusershandler.NewHandler(storage)
accountsHandler.RegisterRoutes(r)
accountUsersHandler.RegisterRoutes(r)

// After
organizationsHandler := organizationshandler.NewHandler(storage)
orgUsersHandler := orgusershandler.NewHandler(storage)
organizationsHandler.RegisterRoutes(r)
orgUsersHandler.RegisterRoutes(r)
```

**Validation**:
```bash
cd backend && go build .
```

---

### Task 9: Global Code Search Validation

**Action**: VERIFY no remaining "account" references

**9a. Search for old table references**:
```bash
cd backend
grep -r "trakrf\.accounts" --include="*.go"
# Should return: NO RESULTS

grep -r "trakrf\.account_users" --include="*.go"
# Should return: NO RESULTS
```

**9b. Search for old model imports**:
```bash
cd backend
grep -r "models/account\"" --include="*.go"
# Should return: NO RESULTS (except in spec files)
```

**9c. Search for old JSON tags**:
```bash
cd backend
grep -r "account_name" --include="*.go"
# Should return: NO RESULTS
```

**9d. Search for old variable patterns**:
```bash
cd backend
grep -r "accountID\|account_id" --include="*.go"
# Should return: NO RESULTS (expect some in comments is OK)
```

**If any results found**: Fix manually and re-run validation

---

### Task 10: Build Validation

**Action**: Ensure entire backend builds without errors

```bash
cd backend && go build ./...
```

**Expected**: No compilation errors, all imports resolve

**If build fails**: Review error messages, fix import paths or missing renames, retry

---

### Task 11: Test Suite Validation

**Action**: Run all backend tests

```bash
cd backend && go test ./...
```

**Expected**: All tests pass

**If tests fail**:
- Review test failures
- Update test assertions to match new schema
- Update test data to use `org_name` instead of `account_name`
- Retry until all pass

---

### Task 12: Manual API Testing

**Prerequisites**:
- Database must be running with migrations applied
- Backend server running: `cd backend && go run .`

**12a. Test Signup Endpoint**:
```bash
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "org_name": "Test Organization"
  }'
```

**Expected response (201 Created)**:
```json
{
  "data": {
    "token": "eyJhbGc...",
    "user": {
      "id": 123,
      "email": "test@example.com",
      "name": "test@example.com",
      "created_at": "2025-10-27T00:00:00Z",
      "updated_at": "2025-10-27T00:00:00Z"
    }
  }
}
```

**12b. Verify Database - Organizations**:
```bash
just db-shell
SELECT id, name, domain, is_active, created_at FROM trakrf.organizations;
```

**Expected output**:
```
 id  |       name        |      domain       | is_active |         created_at
-----+-------------------+-------------------+-----------+----------------------------
 123 | Test Organization | test-organization | t         | 2025-10-27 00:00:00+00
```

**12c. Verify Database - Org Users**:
```sql
SELECT org_id, user_id, role, status FROM trakrf.org_users;
```

**Expected output**:
```
 org_id | user_id | role  | status
--------+---------+-------+--------
    123 |     456 | owner | active
```

**12d. Test Login Endpoint**:
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

**Expected response (200 OK)**:
```json
{
  "data": {
    "token": "eyJhbGc...",
    "user": {
      "id": 456,
      "email": "test@example.com",
      ...
    }
  }
}
```

**12e. Verify JWT claims**:
Decode the JWT token (use jwt.io) and verify:
```json
{
  "user_id": 456,
  "email": "test@example.com",
  "current_org_id": 123,
  "exp": ...
}
```

Should use `current_org_id`, NOT `current_account_id`

**12f. Test Error Handling - Duplicate Email**:
```bash
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "newpassword",
    "org_name": "Another Org"
  }'
```

**Expected response (409 Conflict)**:
```json
{
  "error": "Email already exists"
}
```

**12g. Test Error Handling - Duplicate Org Name**:
```bash
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "another@example.com",
    "password": "password123",
    "org_name": "Test Organization"
  }'
```

**Expected response (409 Conflict)**:
```json
{
  "error": "Organization name already taken"
}
```

**12h. Test Error Handling - Wrong Password**:
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "wrongpassword"
  }'
```

**Expected response (401 Unauthorized)**:
```json
{
  "error": "Invalid email or password"
}
```

---

## Risk Assessment

### Risk: Import errors after directory renames
**Mitigation**: Build after each rename, fix import paths immediately

### Risk: SQL query field mismatches
**Mitigation**: Reference migration files directly, test with curl after each query change

### Risk: Missing renames in tests
**Mitigation**: Run tests after each major change (storage, handlers), fix failures immediately

### Risk: JWT claim breaking existing tokens
**Mitigation**: Pre-launch system - no existing tokens to break. Go-forward only.

## Integration Points

- **Database**: Organizations and org_users tables (already exist from migrations)
- **JWT**: Claims structure changed (pre-launch, no existing tokens)
- **API Routes**: May need route updates if handlers expose REST endpoints (check main.go)

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are blocking gates - do not proceed if any fail.

### After EVERY code change:
1. **Build Gate**: `cd backend && go build ./...`
   - Must pass before proceeding to next task
   - If fails: Fix imports/syntax, re-run
2. **Test Gate**: `cd backend && go test ./...`
   - Must pass before considering task complete
   - If fails: Update tests, re-run

### Before marking complete:
3. **Manual API Gate**: Signup and login curl tests must return expected responses
4. **Database Verification Gate**: Organizations and org_users tables must contain correct data

**Enforcement**: If ANY gate fails after 3 attempts, STOP and request help.

## Validation Sequence

**Per-task validation** (after each task 1-8):
```bash
cd backend && go build ./...
```

**After Task 9** (code search):
- Verify NO "account" references remain

**After Task 10** (build):
```bash
cd backend && go build ./...
```

**After Task 11** (tests):
```bash
cd backend && go test ./...
```

**After Task 12** (manual testing):
- All curl tests return expected responses
- Database contains correct data
- JWT claims use `current_org_id`

**Final validation** (before marking complete):
```bash
just backend validate
```

## Plan Quality Assessment

**Complexity Score**: 8/10 (HIGH)
- 10 files total (1 new, 9 modified/renamed)
- 5 subsystems touched
- 12 implementation tasks
- 0 new dependencies
- Existing patterns (refactor work)

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from comprehensive spec
- ✅ Migration files provide exact schema reference
- ✅ All affected files identified via grep
- ✅ Mechanical rename pattern for most work
- ✅ Non-mechanical parts clearly documented (SQL queries)
- ✅ Existing models provide template for new organization model
- ✅ Pre-launch system - no backward compatibility concerns
- ⚠️ Multiple subsystem integration (5 subsystems)
- ⚠️ File renames can cause import errors if missed

**Assessment**: High confidence due to comprehensive spec, clear schema reference, and mechanical nature of changes. Main risk is missing import updates after renames, mitigated by frequent build validation.

**Estimated one-pass success probability**: 75%

**Reasoning**: The mechanical nature of renames combined with clear SQL requirements from migrations provides strong foundation. The 25% risk comes from potential import path issues after directory renames and test assertion updates. Frequent validation gates will catch issues early. This is a well-scoped atomic refactor that must be done all at once.
