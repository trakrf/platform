# Implementation Plan: Disable Asset Matching for Anonymous Users

Generated: 2026-01-23
Specification: spec.md

## Understanding

Anonymous users scanning RFID tags on the Inventory screen are unexpectedly redirected to login. The root cause is `tagStore._queueForLookup()` being called for every new tag, triggering a debounced API call that returns 401 for unauthenticated users. The global axios interceptor then redirects to `#login`.

**Solution**:
1. Guard `_flushLookupQueue` with an auth check (skip API call if not authenticated)
2. Keep tags queued so they can be enriched when user logs in
3. Subscribe to auth state changes and flush queue on login
4. Add E2E test for anonymous scanning behavior

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/stores/orgStore.ts` (lines 135-138) - Auth state subscription pattern
- `frontend/src/stores/orgStore.ts` (line 39) - `useAuthStore.getState()` usage in store
- `frontend/tests/e2e/inventory.spec.ts` - E2E test structure with hardware bridge

**Files to Modify**:
- `frontend/src/stores/tagStore.ts` (lines 358-400) - Add auth guard to `_flushLookupQueue` + subscription
- `frontend/tests/e2e/anonymous-access.spec.ts` - Add scanning test

## Architecture Impact
- **Subsystems affected**: Frontend stores only
- **New dependencies**: None
- **Breaking changes**: None (behavior improvement)

## Task Breakdown

### Task 1: Add useAuthStore Import to tagStore

**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Reference `frontend/src/stores/orgStore.ts` line 4

**Implementation**:
```typescript
// Add to imports (around line 10)
import { useAuthStore } from './authStore';
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 2: Guard _flushLookupQueue with Auth Check

**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Reference `frontend/src/stores/orgStore.ts` line 39

**Implementation**:
At the start of `_flushLookupQueue` (line 358), add auth check BEFORE the existing early-return checks:

```typescript
_flushLookupQueue: async () => {
  // Skip API call for anonymous users - keep queue intact for later
  const isAuthenticated = useAuthStore.getState().isAuthenticated;
  if (!isAuthenticated) {
    return;
  }

  const state = get();
  // ... rest of existing logic unchanged
```

**Key Detail**: Do NOT clear the queue when unauthenticated. The queue should persist so it can be flushed when user logs in.

**Validation**:
```bash
cd frontend && just typecheck && just test
```

---

### Task 3: Subscribe to Auth State Changes and Flush on Login

**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Reference `frontend/src/stores/orgStore.ts` lines 135-138

**Implementation**:
After the store creation (after line 412, after the closing `);`), add subscription:

```typescript
// Flush lookup queue when user logs in (for tags scanned while anonymous)
useAuthStore.subscribe((state, prevState) => {
  // Only react to login (false -> true transition)
  if (state.isAuthenticated && !prevState.isAuthenticated) {
    // User just logged in - flush any queued EPCs for asset enrichment
    useTagStore.getState()._flushLookupQueue();
  }
});
```

**Validation**:
```bash
cd frontend && just typecheck && just test
```

---

### Task 4: Add Unit Test for Auth Guard

**File**: `frontend/src/stores/tagStore.test.ts` (create if doesn't exist, or add to existing)
**Action**: CREATE or MODIFY
**Pattern**: Reference `frontend/src/stores/authStore.test.ts`

**Implementation**:
```typescript
describe('_flushLookupQueue auth guard', () => {
  beforeEach(() => {
    // Reset stores
    useTagStore.setState({
      _lookupQueue: new Set(['EPC001', 'EPC002']),
      _isLookupInProgress: false,
      _lookupTimer: null
    });
  });

  it('should skip API call when not authenticated', async () => {
    // Ensure not authenticated
    useAuthStore.setState({ isAuthenticated: false });

    // Mock the API to verify it's NOT called
    const lookupSpy = vi.spyOn(lookupApi, 'byTags');

    await useTagStore.getState()._flushLookupQueue();

    // API should NOT be called
    expect(lookupSpy).not.toHaveBeenCalled();

    // Queue should still have items (not cleared)
    expect(useTagStore.getState()._lookupQueue.size).toBe(2);
  });

  it('should call API when authenticated', async () => {
    useAuthStore.setState({ isAuthenticated: true });

    const lookupSpy = vi.spyOn(lookupApi, 'byTags').mockResolvedValue({
      data: { data: {} }
    } as any);

    await useTagStore.getState()._flushLookupQueue();

    expect(lookupSpy).toHaveBeenCalled();
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

---

### Task 5: Add E2E Test for Anonymous Scanning

**File**: `frontend/tests/e2e/anonymous-access.spec.ts`
**Action**: MODIFY

**Implementation**:
Add a new test that:
1. Clears auth state
2. Connects to device (via bridge)
3. Scans tags
4. Verifies no redirect to login
5. Verifies tags appear in inventory

```typescript
test('should scan tags without login redirect when anonymous', async ({ page }) => {
  // Start fresh - no auth
  await page.goto('/');
  await clearAuthState(page);
  await page.reload({ waitUntil: 'networkidle' });

  // Navigate to inventory
  await page.goto('/#inventory');
  await page.waitForTimeout(500);

  // Verify we're still on inventory (not redirected)
  expect(page.url()).toContain('#inventory');

  // Try to connect to device (will skip if bridge unavailable)
  try {
    await connectToDevice(page);
  } catch (e) {
    test.skip(true, 'Bridge server not available - skipping hardware test');
    return;
  }

  // Trigger a scan
  await simulateTriggerPress(page);
  await page.waitForTimeout(2000); // Allow tags to be read
  await simulateTriggerRelease(page);

  // Wait for any potential redirect (the bug would redirect here)
  await page.waitForTimeout(1500);

  // Should STILL be on inventory, NOT login
  const finalUrl = page.url();
  expect(finalUrl).not.toContain('#login');
  expect(finalUrl).toContain('#inventory');

  // Verify no 401 errors in console (optional - check console logs)
  // Tags should be visible (basic check)
  const tagCount = await page.evaluate(() => {
    const stores = (window as any).__ZUSTAND_STORES__;
    return stores?.tagStore?.getState().tags.length ?? 0;
  });

  // We should have scanned at least one tag
  expect(tagCount).toBeGreaterThan(0);

  // Cleanup
  await disconnectDevice(page);
});
```

**Note**: Import `connectToDevice`, `disconnectDevice`, `simulateTriggerPress`, `simulateTriggerRelease` from helpers.

**Validation**:
```bash
cd frontend && pnpm test:e2e tests/e2e/anonymous-access.spec.ts
```

---

## Risk Assessment

- **Risk**: Auth subscription might fire before tagStore is fully initialized
  **Mitigation**: Subscription is added after store creation, so this is safe. Same pattern used by orgStore.

- **Risk**: Queue might grow unbounded for long anonymous sessions
  **Mitigation**: Queue uses Set (dedupes EPCs). Worst case is memory grows with unique tags scanned, which is acceptable.

## Integration Points
- Store updates: `tagStore` now imports and subscribes to `useAuthStore`
- No route changes
- No config updates

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd frontend && just lint      # Gate 1: Syntax & Style
cd frontend && just typecheck # Gate 2: Type Safety
cd frontend && just test      # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass

## Validation Sequence

After each task:
```bash
cd frontend && just validate
```

Final validation:
```bash
just validate  # Full stack validation
```

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec - root cause identified with exact line numbers
- ✅ Similar patterns found in codebase at `orgStore.ts` lines 135-138
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow at `authStore.test.ts`
- ✅ Single subsystem (frontend stores) - no cross-cutting concerns

**Assessment**: Straightforward bug fix with well-established patterns in the codebase.

**Estimated one-pass success probability**: 95%

**Reasoning**: The fix is minimal (3-line auth check + 5-line subscription), follows exact patterns already used in `orgStore.ts`, and has clear validation criteria. The only uncertainty is E2E test flakiness with hardware bridge.
