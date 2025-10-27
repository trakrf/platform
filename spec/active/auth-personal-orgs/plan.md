# Implementation Plan: Auto-Create Personal Organizations on Signup
Generated: 2025-10-27
Specification: spec.md

## Understanding

This feature removes the organization name field from the signup form and auto-generates personal organizations from email addresses, following GitHub's pattern where personal accounts are created automatically.

**Key insight**: `mike@example.com` becomes org name `"mike-example-com"` (full email slugified), eliminating collision handling since emails are unique.

**Breaking changes (greenfield)**:
- Remove `org_name` from signup API request
- Rename database column `domain` → `org_slug` for clearer semantics (used in MQTT topics like `trakrf.id/<org_slug>/reads`)
- Add `is_personal` boolean flag to distinguish personal vs team organizations

**No backward compatibility needed** - project is in development with zero production deployments.

## Relevant Files

### Reference Patterns (existing code to follow):

**Database Migrations:**
- `database/migrations/000002_organizations.up.sql` - CREATE TABLE pattern for organizations
- `database/migrations/000003_users.up.sql` - Example of column comments and constraints

**Backend Auth Flow:**
- `backend/internal/services/auth/auth.go:29-99` - Signup transaction pattern (user + org + org_users)
- `backend/internal/services/auth/auth.go:144-151` - Current `slugifyOrgName()` function to update
- `backend/internal/models/auth/auth.go:6-10` - SignupRequest struct to modify

**Frontend Signup:**
- `frontend/src/components/SignupScreen.tsx:1-194` - Form with 3 fields (email, password, org name)
- `frontend/src/stores/authStore.ts:74-114` - Signup action with 3 parameters
- `frontend/src/components/__tests__/SignupScreen.test.tsx` - Test suite to update

**Test Patterns:**
- `frontend/src/stores/authStore.test.ts` - Store testing pattern
- `frontend/src/components/__tests__/SignupScreen.test.tsx` - Component testing pattern

### Files to Modify:

**Database:**
- `database/migrations/000002_organizations.up.sql` - Add `is_personal` column, rename `domain` → `org_slug`

**Backend Models:**
- `backend/internal/models/auth/auth.go` - Remove `OrgName` field from SignupRequest
- `backend/internal/models/organization/organization.go` - Rename `Domain` → `OrgSlug` field

**Backend Services:**
- `backend/internal/services/auth/auth.go` - Update `slugifyOrgName()` to handle full email, update Signup() service

**Backend Handlers:**
- `backend/internal/handlers/auth/auth.go` - Update swagger comments

**Frontend Components:**
- `frontend/src/components/SignupScreen.tsx` - Remove org name field and validation
- `frontend/src/components/__tests__/SignupScreen.test.tsx` - Update tests

**Frontend Store:**
- `frontend/src/stores/authStore.ts` - Remove 3rd parameter from signup() method
- `frontend/src/stores/authStore.test.ts` - Update tests

**Frontend API:**
- `frontend/src/lib/api/auth.ts` - Remove org_name from signup interface

### Files to Create:

**Backend Tests:**
- `backend/internal/services/auth/auth_test.go` - Unit tests for email slugification and signup flow

## Architecture Impact

- **Subsystems affected**: Database (schema change), Backend API (Go), Frontend UI (React)
- **New dependencies**: None
- **Breaking changes**: Yes - signup API contract changes (greenfield, acceptable)

## Task Breakdown

### Task 1: Update Database Migration - Add is_personal and Rename domain → org_slug
**File**: `database/migrations/000002_organizations.up.sql`
**Action**: MODIFY
**Pattern**: Reference existing CREATE TABLE structure

**Implementation**:
```sql
CREATE TABLE organizations (
    id INT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    org_slug VARCHAR(255) UNIQUE,  -- Renamed from 'domain'
    is_personal BOOLEAN NOT NULL DEFAULT false,  -- NEW COLUMN
    metadata JSONB DEFAULT '{}',
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- Update index name
CREATE INDEX idx_organizations_org_slug ON organizations(org_slug);

-- Update comments
COMMENT ON COLUMN organizations.org_slug IS 'URL-safe slug for MQTT topics and routing (e.g., mike-example-com for trakrf.id/mike-example-com/reads)';
COMMENT ON COLUMN organizations.is_personal IS 'True if auto-created personal organization (single-owner account), false for team organizations';
```

**Validation**:
```bash
# Reset database with new schema
just db-reset
# OR: psql -c "DROP SCHEMA trakrf CASCADE;" then just db-migrate-up

# Verify columns exist
psql -d trakrf -c "\d trakrf.organizations"
# Should show: org_slug VARCHAR(255) and is_personal BOOLEAN
```

---

### Task 2: Update Organization Model - Rename Domain → OrgSlug
**File**: `backend/internal/models/organization/organization.go`
**Action**: MODIFY
**Pattern**: Existing struct definition

**Implementation**:
```go
type Organization struct {
    ID         int                    `json:"id"`
    Name       string                 `json:"name"`
    OrgSlug    string                 `json:"org_slug"`  // Renamed from Domain
    IsPersonal bool                   `json:"is_personal"` // NEW FIELD
    Metadata   map[string]interface{} `json:"metadata"`
    ValidFrom  time.Time              `json:"valid_from"`
    ValidTo    *time.Time             `json:"valid_to,omitempty"`
    IsActive   bool                   `json:"is_active"`
    CreatedAt  time.Time              `json:"created_at"`
    UpdatedAt  time.Time              `json:"updated_at"`
    DeletedAt  *time.Time             `json:"deleted_at,omitempty"`
}
```

**Validation**:
```bash
cd backend
just lint
just typecheck  # Go doesn't have separate typecheck, but compilation catches errors
```

---

### Task 3: Update slugifyOrgName() to Handle Full Email
**File**: `backend/internal/services/auth/auth.go` (lines 144-151)
**Action**: MODIFY
**Pattern**: Existing slugify logic

**Implementation**:
```go
// slugifyOrgName converts organization name or email to URL-safe slug for org_slug field.
// For emails, the entire email is slugified to guarantee uniqueness.
// Examples:
//   "My Company"           -> "my-company"
//   "mike@example.com"     -> "mike-example-com"
//   "alice.smith@acme.io"  -> "alice-smith-acme-io"
func slugifyOrgName(name string) string {
    slug := strings.ToLower(name)
    // Replace @ and . with hyphens (for email addresses)
    slug = strings.ReplaceAll(slug, "@", "-")
    slug = strings.ReplaceAll(slug, ".", "-")
    // Replace any other non-alphanumeric characters with hyphens
    reg := regexp.MustCompile(`[^a-z0-9-]+`)
    slug = reg.ReplaceAllString(slug, "-")
    // Trim leading/trailing hyphens
    slug = strings.Trim(slug, "-")
    return slug
}
```

**Validation**:
```bash
cd backend
just lint
go build ./...
```

---

### Task 4: Remove OrgName from SignupRequest and Update Signup Service
**File**: `backend/internal/models/auth/auth.go` and `backend/internal/services/auth/auth.go`
**Action**: MODIFY
**Pattern**: Reference existing Signup() transaction flow

**Implementation**:

**A. Update SignupRequest (models/auth/auth.go:6-10):**
```go
// SignupRequest for POST /api/v1/auth/signup
type SignupRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
    // OrgName removed - auto-generated from email
}
```

**B. Update Signup() service (services/auth/auth.go:29-99):**
```go
func (s *Service) Signup(ctx context.Context, request auth.SignupRequest, hashPassword func(string) (string, error), generateJWT func(int, string, *int) (string, error)) (*auth.AuthResponse, error) {
    passwordHash, err := hashPassword(request.Password)
    if err != nil {
        return nil, fmt.Errorf("failed to hash password: %w", err)
    }

    // Auto-generate org name from email
    orgName := request.Email
    orgSlug := slugifyOrgName(orgName)

    tx, err := s.db.Begin(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback(ctx)

    // Create user (same as before)
    var usr user.User
    userQuery := `
        INSERT INTO trakrf.users (email, name, password_hash)
        VALUES ($1, $2, $3)
        RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
    `
    err = tx.QueryRow(ctx, userQuery, request.Email, request.Email, passwordHash).Scan(
        &usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
        &usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt)
    if err != nil {
        if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
            return nil, fmt.Errorf("email already exists")
        }
        return nil, fmt.Errorf("failed to create user: %w", err)
    }

    // Create personal organization with is_personal=true
    var org organization.Organization
    orgQuery := `
        INSERT INTO trakrf.organizations (name, org_slug, is_personal)
        VALUES ($1, $2, true)
        RETURNING id, name, org_slug, is_personal, metadata, valid_from, valid_to, is_active, created_at, updated_at
    `
    err = tx.QueryRow(ctx, orgQuery, orgName, orgSlug).Scan(
        &org.ID, &org.Name, &org.OrgSlug, &org.IsPersonal, &org.Metadata,
        &org.ValidFrom, &org.ValidTo, &org.IsActive,
        &org.CreatedAt, &org.UpdatedAt)
    if err != nil {
        if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
            return nil, fmt.Errorf("organization name already taken")
        }
        return nil, fmt.Errorf("failed to create organization: %w", err)
    }

    // Link user to organization as owner (same as before)
    orgUserQuery := `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, 'owner')
    `
    _, err = tx.Exec(ctx, orgUserQuery, org.ID, usr.ID)
    if err != nil {
        return nil, fmt.Errorf("failed to link user to organization: %w", err)
    }

    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("failed to commit transaction: %w", err)
    }

    token, err := generateJWT(usr.ID, usr.Email, &org.ID)
    if err != nil {
        return nil, fmt.Errorf("failed to generate JWT: %w", err)
    }

    return &auth.AuthResponse{
        Token: token,
        User:  usr,
    }, nil
}
```

**Validation**:
```bash
cd backend
just lint
just build
```

---

### Task 5: Create Backend Unit Tests
**File**: `backend/internal/services/auth/auth_test.go`
**Action**: CREATE
**Pattern**: Go testing conventions with table-driven tests

**Implementation**:
```go
package auth

import (
    "testing"
)

func TestSlugifyOrgName(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "simple email",
            input:    "mike@example.com",
            expected: "mike-example-com",
        },
        {
            name:     "email with dots",
            input:    "alice.smith@company.io",
            expected: "alice-smith-company-io",
        },
        {
            name:     "email with plus",
            input:    "bob+test@gmail.com",
            expected: "bob+test-gmail-com",
        },
        {
            name:     "email with hyphens",
            input:    "john-doe@example.com",
            expected: "john-doe-example-com",
        },
        {
            name:     "uppercase email",
            input:    "MIKE@EXAMPLE.COM",
            expected: "mike-example-com",
        },
        {
            name:     "regular org name",
            input:    "My Company Inc",
            expected: "my-company-inc",
        },
        {
            name:     "org name with special chars",
            input:    "Acme & Co.",
            expected: "acme-co",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := slugifyOrgName(tt.input)
            if result != tt.expected {
                t.Errorf("slugifyOrgName(%q) = %q, want %q", tt.input, result, tt.expected)
            }
        })
    }
}

// TODO: Add integration test for Signup() that:
// 1. Creates user + personal org in transaction
// 2. Verifies is_personal = true
// 3. Verifies org_slug matches expected format
// 4. Verifies user is owner in org_users table
```

**Validation**:
```bash
cd backend
just test
# Should show: TestSlugifyOrgName passing with 7 test cases
```

---

### Task 6: Update Backend Handler Swagger Comments
**File**: `backend/internal/handlers/auth/auth.go` (lines 29-39)
**Action**: MODIFY
**Pattern**: Existing swagger comment format

**Implementation**:
```go
// @Summary User signup
// @Description Register new user with auto-created personal organization
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.SignupRequest true "Signup request (email and password only)"
// @Success 201 {object} map[string]any "data: auth.SignupResponse"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 409 {object} errors.ErrorResponse "Email already exists"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/signup [post]
```

**Validation**:
```bash
cd backend
just lint
```

---

### Task 7: Remove Organization Name Field from SignupScreen
**File**: `frontend/src/components/SignupScreen.tsx`
**Action**: MODIFY
**Pattern**: Reference existing form structure (lines 1-194)

**Implementation**:
Remove:
- Line 8: `const [organizationName, setOrganizationName] = useState('');`
- Lines 10-14: Remove `organizationName?: string;` from errors interface
- Lines 32-36: Remove `validateOrganizationName()` function
- Line 47: Remove `const orgError = validateOrganizationName(organizationName);`
- Lines 50-53: Remove org error from validation block
- Line 59: Change to `await signup(email, password);` (remove 3rd arg)
- Lines 144-164: Remove organization name input field JSX

Keep only email and password fields.

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 8: Update AuthStore - Remove 3rd Parameter from signup()
**File**: `frontend/src/stores/authStore.ts` (lines 17, 74-114)
**Action**: MODIFY
**Pattern**: Existing store action pattern

**Implementation**:
```typescript
interface AuthState {
  // ... other fields
  signup: (email: string, password: string) => Promise<void>;  // Remove 3rd parameter
  // ... other actions
}

// Update signup action
signup: async (email: string, password: string) => {
  set({ isLoading: true, error: null });
  try {
    const response = await authApi.signup({
      email,
      password,
      // org_name removed - backend auto-generates
    });
    const { token, user } = response.data.data;

    set({
      token,
      user,
      isAuthenticated: true,
      isLoading: false,
      error: null,
    });
  } catch (err: any) {
    // Error handling unchanged...
  }
},
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 9: Update Auth API Client - Remove org_name from Signup Interface
**File**: `frontend/src/lib/api/auth.ts`
**Action**: MODIFY
**Pattern**: Existing API interface

**Implementation**:
```typescript
export interface SignupRequest {
  email: string;
  password: string;
  // org_name removed
}

export const authApi = {
  signup: (data: SignupRequest) => api.post<AuthResponse>('/auth/signup', data),
  // ... other methods
};
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 10: Update SignupScreen Tests
**File**: `frontend/src/components/__tests__/SignupScreen.test.tsx`
**Action**: MODIFY
**Pattern**: Existing test structure

**Implementation**:
Remove/update tests:
- Line 33: Remove `expect(screen.getByLabelText(/organization name/i))` assertion
- Lines 72-82: Remove organization validation test
- Lines 84-93: Update to only test email/password validation
- Lines 117-134: Update signup call test - remove orgInput, change to 2 parameters
- Lines 136-153: Update redirect test - remove orgInput
- Lines 155-176: Update error test - remove orgInput
- Lines 180-195: Update loading state test - remove orgInput

Add new test:
```typescript
it('should call signup with only email and password', async () => {
  mockSignup.mockResolvedValue(undefined);
  render(<SignupScreen />);

  const emailInput = screen.getByLabelText(/email/i);
  const passwordInput = screen.getByLabelText(/password/i);
  const submitButton = screen.getByRole('button', { name: /sign up/i });

  fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
  fireEvent.change(passwordInput, { target: { value: 'password123' } });
  fireEvent.click(submitButton);

  await waitFor(() => {
    expect(mockSignup).toHaveBeenCalledWith('test@example.com', 'password123');
    expect(mockSignup).toHaveBeenCalledTimes(1);
  });
});
```

**Validation**:
```bash
cd frontend
just test
# All SignupScreen tests should pass
```

---

### Task 11: Update AuthStore Tests
**File**: `frontend/src/stores/authStore.test.ts`
**Action**: MODIFY (if exists) or CREATE
**Pattern**: Zustand store testing pattern

**Implementation**:
```typescript
describe('authStore signup', () => {
  it('should call API with email and password only', async () => {
    const mockSignup = vi.spyOn(authApi, 'signup').mockResolvedValue({
      data: {
        data: {
          token: 'test-token',
          user: { id: 1, email: 'test@example.com', name: 'test@example.com' }
        }
      }
    } as any);

    await useAuthStore.getState().signup('test@example.com', 'password123');

    expect(mockSignup).toHaveBeenCalledWith({
      email: 'test@example.com',
      password: 'password123',
    });
    expect(useAuthStore.getState().isAuthenticated).toBe(true);
  });
});
```

**Validation**:
```bash
cd frontend
just test
```

---

### Task 12: Integration Test - Full Signup Flow
**File**: `backend/internal/services/auth/auth_test.go` (add to existing file from Task 5)
**Action**: MODIFY
**Pattern**: Transaction-based testing with test database

**Implementation**:
```go
// Integration test - requires test database
func TestSignupIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // TODO: Set up test database connection
    // TODO: Test full signup flow:
    // 1. Call Signup() with email and password
    // 2. Verify user created in users table
    // 3. Verify personal org created with is_personal=true
    // 4. Verify org_slug matches email slugification
    // 5. Verify user linked as owner in org_users
    // 6. Verify JWT contains org_id
}
```

**Validation**:
```bash
cd backend
just test
```

---

### Task 13: Manual E2E Testing
**Action**: MANUAL TESTING
**Pattern**: Full user signup flow

**Test Cases**:
1. **Basic signup**: `test@example.com` → verify org created as `test-example-com`
2. **Email with dots**: `alice.smith@company.io` → verify org `alice-smith-company-io`
3. **Email with plus**: `bob+tag@gmail.com` → verify org `bob+tag-gmail-com`
4. **Uppercase email**: `MIKE@EXAMPLE.COM` → verify org `mike-example-com` (lowercase)
5. **Duplicate email**: Try same email twice → expect error

**Validation Steps**:
```bash
# Start services
just dev-local

# Test signup via frontend
# Open http://localhost:5173/#signup
# Enter email and password only
# Submit form

# Verify in database
psql -d trakrf -c "SELECT id, name, org_slug, is_personal FROM trakrf.organizations;"
psql -d trakrf -c "SELECT org_id, user_id, role FROM trakrf.org_users;"
```

---

### Task 14: Update MQTT Topic Comments/Docs (Optional)
**File**: `database/migrations/000002_organizations.up.sql`
**Action**: Already done in Task 1
**Pattern**: Column comments

The column comment for `org_slug` already references MQTT topic usage:
```sql
COMMENT ON COLUMN organizations.org_slug IS 'URL-safe slug for MQTT topics and routing (e.g., mike-example-com for trakrf.id/mike-example-com/reads)';
```

**Validation**: None needed (documentation only)

---

## Risk Assessment

**Risk**: Breaking backend queries that reference `domain` column
**Mitigation**: Greenfield project with no production deployments. Search codebase for `domain` references and update to `org_slug`.

**Risk**: Frontend tests failing after removing org name field
**Mitigation**: Update all test assertions systematically. Run `just frontend test` after each change.

**Risk**: Existing data in development database
**Mitigation**: Use `DROP SCHEMA trakrf CASCADE;` or `just db-reset` to start clean.

**Risk**: slugifyOrgName() logic doesn't handle edge cases
**Mitigation**: Comprehensive unit tests cover dots, plus signs, hyphens, uppercase, special characters.

## Integration Points

**Database Schema:**
- Add `is_personal` BOOLEAN column
- Rename `domain` → `org_slug` column
- Update column comments

**Backend API:**
- Remove `OrgName` from SignupRequest
- Update Signup() service to auto-generate from email
- Update `slugifyOrgName()` function logic
- Rename Organization model field

**Frontend:**
- Remove organization name input from SignupScreen
- Remove 3rd parameter from authStore.signup()
- Update all tests

**JWT Claims:**
- No changes needed (already includes org_id)

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

**After EVERY code change:**

**Backend:**
```bash
cd backend
just lint      # Must pass - no linting errors
just build     # Must pass - Go compilation succeeds
just test      # Must pass - all tests passing
```

**Frontend:**
```bash
cd frontend
just lint       # Must pass - ESLint clean
just typecheck  # Must pass - TypeScript types correct
just test       # Must pass - all tests passing
```

**Enforcement Rules:**
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**After each backend task:**
```bash
cd backend
just lint && just build && just test
```

**After each frontend task:**
```bash
cd frontend
just lint && just typecheck && just test
```

**Final validation (all tasks complete):**
```bash
# From project root
just validate
# This runs lint + test + build for both backend and frontend
```

**Manual E2E validation:**
```bash
just dev-local
# Test signup flow in browser
# Verify database state
```

## Plan Quality Assessment

**Complexity Score**: 7/10 (MEDIUM-HIGH)
- 6 files modified, 1 file created
- 3 subsystems (Database, Backend, Frontend)
- ~14 subtasks
- 0 new dependencies
- Existing patterns to follow

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from detailed spec
✅ Similar patterns found in codebase (signup flow at auth.go:29-99, form at SignupScreen.tsx)
✅ All clarifying questions answered
✅ Existing test patterns to follow (SignupScreen.test.tsx, store testing patterns)
✅ Greenfield project - breaking changes acceptable
✅ No external dependencies or unknowns

**Assessment**: High confidence implementation. Changes are straightforward modifications to existing patterns. Signup flow already exists, we're just removing one field and auto-generating the value. Database change is simple column add/rename. Test patterns are established.

**Estimated one-pass success probability**: 85%

**Reasoning**:
- Clear spec with examples
- Existing code patterns to modify (not creating from scratch)
- Comprehensive validation gates catch issues early
- No complex new features or external integrations
- Main risk is systematic updating of all references (mitigated by good test coverage)
