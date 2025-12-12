# Implementation Plan: Required Organization Name at Signup (TRA-190)
Generated: 2025-12-12
Specification: spec.md

## Understanding

Users currently sign up with just email/password, and a "personal org" is auto-created using their email as the org name. This causes confusion (seeing email addresses in the org switcher). We're changing this to:

1. Require an "Organization Name" field at signup
2. Remove the `is_personal` concept entirely (column dropped)
3. Add helper text guiding users to request invites if their company already uses TrakRF

## Relevant Files

**Reference Patterns**:
- `frontend/src/components/SignupScreen.tsx` - Current form structure to extend
- `frontend/src/components/__tests__/SignupScreen.test.tsx` - Test patterns to follow
- `backend/internal/services/auth/auth.go:37-109` - Signup function to modify
- `backend/internal/models/auth/auth.go` - Request struct pattern

**Files to Modify**:

| File | Change |
|------|--------|
| `backend/internal/models/auth/auth.go` | Add `OrgName` to SignupRequest |
| `backend/internal/services/auth/auth.go` | Use provided org name, remove `is_personal=true` |
| `backend/internal/services/auth/auth_test.go` | Update test comment |
| `backend/internal/models/organization/organization.go` | Remove `IsPersonal` field |
| `backend/internal/storage/organizations.go` | Remove `is_personal` from queries |
| `backend/internal/services/orgs/service.go` | Remove `is_personal` from queries |
| `backend/internal/testutil/database.go` | Remove `is_personal` from test data |
| `frontend/src/types/org/index.ts` | Remove `is_personal` from Organization interface |
| `frontend/src/components/SignupScreen.tsx` | Add org name field + helper text |
| `frontend/src/lib/api/auth.ts` | Add `org_name` to SignupRequest |
| `frontend/src/stores/authStore.ts` | Update signup action signature |
| `frontend/src/components/__tests__/SignupScreen.test.tsx` | Update tests for new field |

**Files to Create**:
- `backend/migrations/000023_remove_is_personal.up.sql`
- `backend/migrations/000023_remove_is_personal.down.sql`

## Architecture Impact
- **Subsystems affected**: Frontend (signup form), Backend (auth service), Database (schema)
- **New dependencies**: None
- **Breaking changes**: API change (signup now requires `org_name`), but no production users

## Task Breakdown

### Task 1: Create database migration to drop is_personal column
**Files**: `backend/migrations/000023_remove_is_personal.{up,down}.sql`
**Action**: CREATE

**Implementation**:
```sql
-- up.sql
ALTER TABLE trakrf.organizations DROP COLUMN is_personal;

-- down.sql
ALTER TABLE trakrf.organizations ADD COLUMN is_personal BOOLEAN NOT NULL DEFAULT false;
```

**Validation**: `just backend validate`

---

### Task 2: Update backend Organization model
**File**: `backend/internal/models/organization/organization.go`
**Action**: MODIFY (line 11)

**Implementation**:
Remove the `IsPersonal` field from the Organization struct.

**Validation**: `just backend validate`

---

### Task 3: Update backend auth models - add OrgName to SignupRequest
**File**: `backend/internal/models/auth/auth.go`
**Action**: MODIFY (lines 5-10)

**Implementation**:
```go
type SignupRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	OrgName  string `json:"org_name" validate:"required,min=2,max=100"`
}
```

**Validation**: `just backend validate`

---

### Task 4: Update auth service Signup function
**File**: `backend/internal/services/auth/auth.go`
**Action**: MODIFY (lines 43-78)

**Implementation**:
- Use `request.OrgName` instead of `request.Email` for org name (line 44)
- Remove `is_personal` from INSERT query (lines 72-74)
- Update RETURNING clause to exclude `is_personal`
- Update Scan to exclude `&org.IsPersonal`

**Validation**: `just backend validate`

---

### Task 5: Update storage/organizations.go queries
**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY (lines 43-100)

**Implementation**:
Remove `is_personal` from all SELECT, INSERT queries and Scan calls.

**Validation**: `just backend validate`

---

### Task 6: Update orgs service queries
**File**: `backend/internal/services/orgs/service.go`
**Action**: MODIFY (lines 39-45)

**Implementation**:
Remove `is_personal` from INSERT query and Scan.

**Validation**: `just backend validate`

---

### Task 7: Update test utilities
**File**: `backend/internal/testutil/database.go`
**Action**: MODIFY (line 256)

**Implementation**:
Remove `is_personal` from test INSERT statement.

**Validation**: `just backend validate`

---

### Task 8: Update frontend Organization type
**File**: `frontend/src/types/org/index.ts`
**Action**: MODIFY (line 46)

**Implementation**:
Remove `is_personal: boolean;` from Organization interface.

**Validation**: `just frontend typecheck`

---

### Task 9: Update frontend auth API types
**File**: `frontend/src/lib/api/auth.ts`
**Action**: MODIFY (lines 3-7)

**Implementation**:
```typescript
export interface SignupRequest {
  email: string;
  password: string;
  org_name: string;
}
```

**Validation**: `just frontend typecheck`

---

### Task 10: Update authStore signup action
**File**: `frontend/src/stores/authStore.ts`
**Action**: MODIFY (lines 22, 85-91)

**Implementation**:
- Update type: `signup: (email: string, password: string, orgName: string) => Promise<void>;`
- Update function signature and API call to include `org_name`

**Validation**: `just frontend typecheck`

---

### Task 11: Update SignupScreen with org name field
**File**: `frontend/src/components/SignupScreen.tsx`
**Action**: MODIFY

**Implementation**:
- Add `orgName` state variable
- Add `validateOrgName` function (2-100 chars, trimmed)
- Add org name input field after email, before password
- Add helper text: "If your company is already using TrakRF, ask your admin for an invite instead of creating a new organization."
- Update `handleSubmit` to pass orgName to signup

**Validation**: `just frontend validate`

---

### Task 12: Update SignupScreen tests
**File**: `frontend/src/components/__tests__/SignupScreen.test.tsx`
**Action**: MODIFY

**Implementation**:
- Update render tests to expect org name field
- Add validation tests for org name (too short, empty)
- Update form submission test to include org name
- Update mock expectations: `mockSignup.toHaveBeenCalledWith(email, password, orgName)`

**Validation**: `just frontend test`

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Migration fails on existing data | Test migration on dev DB first; no production data exists |
| Missed `is_personal` reference | Comprehensive grep search performed; all instances documented |
| API breaking change | No production users; frontend/backend deployed together |

## Integration Points
- **Store updates**: authStore.signup signature changes
- **API contract**: POST /auth/signup now requires `org_name` field
- **Database**: Migration must run before new code deploys

## VALIDATION GATES (MANDATORY)

After EVERY task, run validation commands from `spec/stack.md`:

**Backend tasks (1-7)**:
```bash
just backend lint && just backend test && just backend build
```

**Frontend tasks (8-12)**:
```bash
just frontend lint && just frontend typecheck && just frontend test
```

**Final validation**:
```bash
just validate
```

## Validation Sequence

1. After Tasks 1-7: `just backend validate`
2. After Tasks 8-12: `just frontend validate`
3. Final: `just validate`

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and user answers
- ✅ Similar patterns found: existing form fields, existing model structs
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow at `SignupScreen.test.tsx`
- ✅ No new dependencies
- ✅ No novel patterns - just adding a form field

**Assessment**: Straightforward form field addition with column removal. All patterns exist in codebase.

**Estimated one-pass success probability**: 90%

**Reasoning**: Well-understood changes following existing patterns. Main risk is missing an `is_personal` reference, but comprehensive grep was performed.
