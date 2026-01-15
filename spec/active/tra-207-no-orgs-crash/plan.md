# Implementation Plan: Fix Header Crash When User Has No Organizations
Generated: 2026-01-15
Specification: spec.md

## Understanding
Fix a crash caused by Go's nil slice serializing to JSON `null` instead of `[]`. When a user has no organization memberships, the backend returns `{"orgs": null}` which crashes the frontend's `orgs.map()` call. Fix at source (backend) with defensive frontend check.

## Relevant Files

**Files to Modify**:
- `backend/internal/storage/organizations.go` (line 30) - Initialize slice to prevent nil
- `frontend/src/components/OrgSwitcher.tsx` (line 90) - Add null-coalescing defensive check

## Architecture Impact
- **Subsystems affected**: Backend API, Frontend UI
- **New dependencies**: None
- **Breaking changes**: None (fixes a bug)

## Task Breakdown

### Task 1: Fix Backend Nil Slice
**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY
**Line**: 30

**Implementation**:
Change from:
```go
var orgs []organization.UserOrg
```
to:
```go
orgs := []organization.UserOrg{}
```

This ensures the slice is initialized as empty (not nil), so JSON serialization produces `[]` instead of `null`.

**Validation**:
```bash
just backend lint
just backend test
```

---

### Task 2: Add Frontend Defensive Check
**File**: `frontend/src/components/OrgSwitcher.tsx`
**Action**: MODIFY
**Line**: 90

**Implementation**:
Change from:
```tsx
{orgs.map(org => (
```
to:
```tsx
{(orgs ?? []).map(org => (
```

This provides belt-and-suspenders protection in case any code path sets orgs to null.

**Validation**:
```bash
just frontend lint
just frontend typecheck
just frontend test
```

---

### Task 3: Full Stack Validation
**Action**: VALIDATE

Run full validation to ensure no regressions:
```bash
just validate
```

## Risk Assessment
- **Risk**: Minimal - both changes are single-line, well-understood fixes
  **Mitigation**: Standard validation gates

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just backend lint` / `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `just backend test` / `just frontend test`

Final: `just validate`

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 10/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Root cause fully understood
✅ Single-line fixes in both locations
✅ No new patterns or dependencies
✅ Standard validation coverage

**Assessment**: Straightforward bug fix with well-understood root cause.

**Estimated one-pass success probability**: 98%

**Reasoning**: Both fixes are single-line changes to well-understood code. The only risk is typos.
