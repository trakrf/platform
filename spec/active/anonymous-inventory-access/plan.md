# Implementation Plan: Anonymous Inventory Access

Generated: 2024-12-10
Specification: spec.md

## Understanding

Anonymous users should be able to navigate to Inventory, Locate, and Barcode screens
without being redirected to login. The fix requires making the `useAssets` hook
conditional on authentication status.

## Relevant Files

**Reference Patterns**:
- `frontend/tests/e2e/org-crud.spec.ts` - E2E test structure with clearAuthState
- `frontend/tests/e2e/fixtures/org.fixture.ts` - clearAuthState helper

**Files to Modify**:
- `frontend/src/components/InventoryScreen.tsx` (line ~56) - Add auth check

**Files to Create**:
- `frontend/tests/e2e/anonymous-access.spec.ts` - E2E test for anonymous screen access

## Architecture Impact

- **Subsystems affected**: Frontend only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Fix InventoryScreen to conditionally load assets

**File**: `frontend/src/components/InventoryScreen.tsx`
**Action**: MODIFY

**Implementation**:
```typescript
// Add import for useAuthStore
import { useDeviceStore, useTagStore, useSettingsStore, useAuthStore } from '@/stores';

// Change line ~56 from:
useAssets({ enabled: true });

// To:
const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
useAssets({ enabled: isAuthenticated });
```

**Validation**:
- `pnpm typecheck` - No type errors
- `pnpm lint` - No lint errors

### Task 2: Create anonymous access E2E test

**File**: `frontend/tests/e2e/anonymous-access.spec.ts`
**Action**: CREATE
**Pattern**: Reference `org-crud.spec.ts` for clearAuthState pattern

**Implementation**:
```typescript
/**
 * Anonymous Access E2E Tests
 *
 * Verifies that Inventory, Locate, and Barcode screens are accessible
 * without logging in. Regression test for TRA-177.
 */

import { test, expect } from '@playwright/test';
import { clearAuthState } from './fixtures/org.fixture';

test.describe('Anonymous Access', () => {
  test.beforeEach(async ({ page }) => {
    // Ensure no auth state
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
  });

  test('should access inventory screen without login', async ({ page }) => {
    await page.goto('/#inventory');

    // Should NOT redirect to login
    await expect(page).not.toHaveURL(/#login/);

    // Should see inventory heading
    await expect(page.getByRole('heading', { name: /inventory/i })).toBeVisible({ timeout: 5000 });
  });

  test('should access locate screen without login', async ({ page }) => {
    await page.goto('/#locate');

    await expect(page).not.toHaveURL(/#login/);
    await expect(page.getByRole('heading', { name: /locate/i })).toBeVisible({ timeout: 5000 });
  });

  test('should access barcode screen without login', async ({ page }) => {
    await page.goto('/#barcode');

    await expect(page).not.toHaveURL(/#login/);
    await expect(page.getByRole('heading', { name: /barcode/i })).toBeVisible({ timeout: 5000 });
  });
});
```

**Validation**:
- `pnpm test:e2e tests/e2e/anonymous-access.spec.ts` - All 3 tests pass

## Risk Assessment

- **Risk**: Test might flake if page loads slowly
  **Mitigation**: Using `timeout: 5000` on visibility checks

- **Risk**: Locate/Barcode might have similar issues
  **Mitigation**: Spec confirms they already work; tests will catch regression

## VALIDATION GATES (MANDATORY)

After EVERY code change:
1. `pnpm typecheck` - Must pass
2. `pnpm lint` - Must pass
3. `pnpm test:e2e tests/e2e/anonymous-access.spec.ts` - Must pass (after Task 2)

**Final validation**:
- `pnpm build` - Must succeed

## Validation Sequence

1. After Task 1: `pnpm typecheck && pnpm lint`
2. After Task 2: `pnpm test:e2e tests/e2e/anonymous-access.spec.ts`
3. Final: `pnpm build`

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 10/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Root cause already identified (useAssets unconditional)
✅ Exact code change specified in spec
✅ Existing test patterns to follow (org-crud.spec.ts)
✅ No new dependencies
✅ Single file code change

**Assessment**: Trivial fix with high confidence. The spec contains the exact code change needed.

**Estimated one-pass success probability**: 95%

**Reasoning**: Single-line fix in one file, clear test pattern to follow. Only uncertainty is whether page headings match the regex patterns in tests.
