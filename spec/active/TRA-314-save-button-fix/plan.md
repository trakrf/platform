# Implementation Plan: TRA-314 Save Button Fix

Generated: 2026-01-24
Specification: spec.md

## Understanding

Fix the 403 error on inventory save caused by JWT `current_org_id` mismatch. The root cause is that Login uses an arbitrary first org while the frontend expects `last_org_id`. Additionally, the frontend silently swallows `setCurrentOrg` failures, leaving users with stale tokens.

**Three-part fix:**
1. Backend: Use `last_org_id` in Login (shared helper with GetUserProfile pattern)
2. Frontend: Retry once + throw on `setCurrentOrg` failure
3. Cleanup: Remove debug log spam, keep auth/error logs

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/services/orgs/service.go` (lines 117-130) - last_org_id resolution pattern
- `backend/internal/storage/users.go` (lines 197-208) - UpdateUserLastOrg storage pattern
- `frontend/src/stores/authStore.ts` (lines 79-81) - current error handling to improve

**Files to Modify**:
- `backend/internal/storage/users.go` - Add `GetUserPreferredOrgID` helper
- `backend/internal/services/auth/auth.go` (lines 230-240) - Use new helper instead of raw query
- `frontend/src/stores/authStore.ts` (lines 69-82) - Add retry + throw logic
- `frontend/src/hooks/useInventoryAudio.ts` - Remove debug logs
- `frontend/src/hooks/useDoubleTap.ts` - Remove debug logs
- `frontend/src/stores/tagStore.ts` - Remove verbose logs, keep auth subscription logs
- `frontend/src/stores/deviceStore.ts` - Remove debug logs (keep warning)
- `frontend/src/worker/cs108/system/parser.ts` - Remove battery debug log

## Architecture Impact
- **Subsystems affected**: Backend Auth, Frontend Auth Store
- **New dependencies**: None
- **Breaking changes**: None - Login now returns correct org in JWT from the start

## Task Breakdown

### Task 1: Add GetUserPreferredOrgID storage helper
**File**: `backend/internal/storage/users.go`
**Action**: MODIFY (add new function)
**Pattern**: Reference `UpdateUserLastOrg` at lines 197-208

**Implementation**:
```go
// GetUserPreferredOrgID returns the user's last_org_id if set and valid,
// otherwise returns their first org membership, or nil if no orgs.
func (s *Storage) GetUserPreferredOrgID(ctx context.Context, userID int) (*int, error) {
    query := `
        SELECT COALESCE(
            -- First try: user's last_org_id if they're still a member
            (SELECT u.last_org_id
             FROM trakrf.users u
             JOIN trakrf.org_users ou ON ou.org_id = u.last_org_id AND ou.user_id = u.id
             WHERE u.id = $1 AND u.last_org_id IS NOT NULL
               AND ou.deleted_at IS NULL AND u.deleted_at IS NULL),
            -- Fallback: first org by name (consistent with ListUserOrgs)
            (SELECT ou.org_id
             FROM trakrf.org_users ou
             JOIN trakrf.organizations o ON o.id = ou.org_id
             WHERE ou.user_id = $1 AND ou.deleted_at IS NULL AND o.deleted_at IS NULL
             ORDER BY o.name ASC
             LIMIT 1)
        ) as org_id
    `
    var orgID *int
    err := s.pool.QueryRow(ctx, query, userID).Scan(&orgID)
    if err == pgx.ErrNoRows || orgID == nil {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get user preferred org: %w", err)
    }
    return orgID, nil
}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 2: Update Login service to use GetUserPreferredOrgID
**File**: `backend/internal/services/auth/auth.go`
**Action**: MODIFY (lines 230-240)
**Pattern**: Replace inline query with storage call

**Implementation**:
Replace:
```go
orgUserQuery := `
    SELECT org_id
    FROM trakrf.org_users
    WHERE user_id = $1 AND deleted_at IS NULL
    LIMIT 1
`
var orgID int
err = s.db.QueryRow(ctx, orgUserQuery, usr.ID).Scan(&orgID)
if err != nil {
    orgID = 0
}
```

With:
```go
orgIDPtr, err := s.storage.GetUserPreferredOrgID(ctx, usr.ID)
if err != nil {
    // Log but don't fail login - user can still select org manually
    fmt.Printf("Warning: failed to get preferred org: %v\n", err)
}

var orgID int
if orgIDPtr != nil {
    orgID = *orgIDPtr
}
```

Also update the `orgIDPtr` usage at line 256-259 (no longer need to create pointer):
```go
// Remove lines 256-259, use orgIDPtr directly:
token, err := generateJWT(usr.ID, usr.Email, orgIDPtr)
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 3: Add retry + throw for setCurrentOrg in authStore
**File**: `frontend/src/stores/authStore.ts`
**Action**: MODIFY (lines 69-82)

**Implementation**:
Replace the current try/catch block:
```typescript
// Ensure token has org_id claim for org-scoped API calls
const profile = get().profile;
if (profile?.current_org?.id) {
  try {
    console.log('[AuthStore] Refreshing token with org_id:', profile.current_org.id);
    const orgResponse = await orgsApi.setCurrentOrg({ org_id: profile.current_org.id });
    set({ token: orgResponse.data.token });

    // INVALIDATE: After setCurrentOrg() returns with org_id token
    const { invalidateAllOrgScopedData } = await import('@/lib/cache/orgScopedCache');
    const { queryClient } = await import('@/lib/queryClient');
    await invalidateAllOrgScopedData(queryClient);
  } catch (err) {
    console.error('[AuthStore] Failed to refresh token with org_id:', err);
  }
}
```

With:
```typescript
// Ensure token has org_id claim for org-scoped API calls
const profile = get().profile;
if (profile?.current_org?.id) {
  const refreshTokenWithOrg = async (attempt: number): Promise<void> => {
    try {
      console.log('[AuthStore] Refreshing token with org_id:', profile.current_org!.id, attempt > 1 ? `(attempt ${attempt})` : '');
      const orgResponse = await orgsApi.setCurrentOrg({ org_id: profile.current_org!.id });
      set({ token: orgResponse.data.token });

      // INVALIDATE: After setCurrentOrg() returns with org_id token
      const { invalidateAllOrgScopedData } = await import('@/lib/cache/orgScopedCache');
      const { queryClient } = await import('@/lib/queryClient');
      await invalidateAllOrgScopedData(queryClient);
    } catch (err) {
      if (attempt < 2) {
        console.warn('[AuthStore] setCurrentOrg failed, retrying...', err);
        await refreshTokenWithOrg(attempt + 1);
      } else {
        console.error('[AuthStore] Failed to refresh token with org_id after retry:', err);
        throw new Error('Login failed: could not set organization context');
      }
    }
  };
  await refreshTokenWithOrg(1);
}
```

**Note**: Same pattern needs to be applied to `signup` action (lines 140-152).

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 4: Remove debug log spam from useInventoryAudio
**File**: `frontend/src/hooks/useInventoryAudio.ts`
**Action**: MODIFY

**Remove these lines**:
- Line 46: `console.debug('[useInventoryAudio] Stopping all sounds...')`
- Line 55: `console.debug('[useInventoryAudio] Reading tags at...')`
- Line 61: `console.debug('[useInventoryAudio] Scanning without tags...')`

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 5: Remove debug log spam from useDoubleTap
**File**: `frontend/src/hooks/useDoubleTap.ts`
**Action**: MODIFY

**Remove these lines**:
- Line 18: `console.debug('[useDoubleTap] Starting double-tap...')`
- Line 26: `console.debug('[useDoubleTap] Playing initial double-tap')`
- Line 32: `console.debug('[useDoubleTap] Playing interval double-tap')`

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 6: Clean up tagStore logs (keep auth, remove queue spam)
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY

**Remove these lines**:
- Line 355: `console.log('[TagStore] refreshAssetEnrichment:...')`
- Line 389: `console.log('[TagStore] _flushLookupQueue: skipping (not authenticated)')`
- Line 397: `console.log('[TagStore] _flushLookupQueue: skipping...')`
- Line 404: `console.log('[TagStore] _flushLookupQueue: starting...')`
- Line 422: `console.log('[TagStore] _flushLookupQueue: API response...')`

**Keep these (auth flow)**:
- Line 502: `console.log('[TagStore] Auth subscription: login detected...')`
- Line 511: `console.log('[TagStore] Auth subscription: logout detected...')`

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 7: Remove debug logs from deviceStore
**File**: `frontend/src/stores/deviceStore.ts`
**Action**: MODIFY

**Remove these lines**:
- Line 60: `console.debug('[DeviceStore] Reader state change...')`
- Line 75: `console.debug('[DeviceStore] Scanning stopped...')`
- Line 80: `console.debug('[DeviceStore] Reader error...')`
- Line 88: `console.debug('[DeviceStore] Reader mode change...')`
- Line 104: `console.debug('[DeviceStore] Scan button toggled...')`
- Line 107: `console.debug('[DeviceStore] Scan button toggled for mode...')`

**Keep this (warning for bug detection)**:
- Line 63: `console.warn('[DeviceStore] WARNING: Setting READY after DISCONNECTED...')`

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 8: Remove battery parser debug log
**File**: `frontend/src/worker/cs108/system/parser.ts`
**Action**: MODIFY

**Remove this line**:
- Line 54: `console.debug('[BatteryParser] Raw voltage...')`

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 9: Run full validation and test
**Action**: VALIDATE

```bash
just validate
```

Verify:
- All 902+ frontend tests pass
- Backend tests pass
- Build succeeds

---

## Risk Assessment

- **Risk**: Storage helper query is complex (COALESCE with subqueries)
  **Mitigation**: Test with user who has last_org_id set, user without, and user with no orgs

- **Risk**: Retry logic could mask real auth issues
  **Mitigation**: Only retry once, throw with clear message on second failure

- **Risk**: Removing logs could make future debugging harder
  **Mitigation**: Keeping auth flow logs ([AuthStore], [TagStore] auth subscription)

## Integration Points
- Store updates: authStore retry logic
- Route changes: None
- Config updates: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just lint` - Must pass
- Gate 2: `just typecheck` (frontend) - Must pass
- Gate 3: `just test` - Must pass

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task:
```bash
# Backend tasks (1-2)
cd backend && just lint && just test

# Frontend tasks (3-8)
cd frontend && just lint && just typecheck && just test
```

Final validation:
```bash
just validate
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear root cause identified and verified
✅ Similar pattern exists in codebase (GetUserProfile lines 117-130)
✅ All clarifying questions answered
✅ Existing test patterns to follow
✅ Debug log locations precisely identified with line numbers
✅ No new dependencies

**Assessment**: Straightforward fix with well-understood patterns. Backend change follows existing last_org_id resolution. Frontend change adds standard retry logic.

**Estimated one-pass success probability**: 90%

**Reasoning**: Clear root cause, existing patterns to follow, no architectural changes. Main risk is the SQL query complexity which can be validated with tests.
