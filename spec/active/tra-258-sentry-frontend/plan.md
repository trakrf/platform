# Implementation Plan: Add Sentry Error Tracking to React Frontend

Generated: 2026-01-14
Specification: spec.md

## Understanding

Add Sentry error tracking to the React frontend with:
1. SDK initialization in `main.tsx` before React renders
2. Integration with existing `ErrorBoundary` component to report caught errors
3. User context integration with `authStore` (set on login, clear on logout)
4. Dev-only test button for verification

## Relevant Files

**Reference Patterns**:
- `frontend/src/lib/openreplay.ts` (lines 6-40) - Environment variable pattern for optional service
- `frontend/src/main.tsx` (lines 34-55) - Dev-only code block pattern
- `frontend/src/stores/authStore.ts` (lines 43-58, 130-138) - Login/logout action patterns
- `backend/main.go` (lines 160-176) - Backend Sentry init for reference

**Files to Modify**:
- `frontend/src/main.tsx` - Add Sentry initialization before React renders
- `frontend/src/components/ErrorBoundary.tsx` - Add Sentry reporting in componentDidCatch
- `frontend/src/stores/authStore.ts` - Set/clear Sentry user context on login/logout

**Files to Create**:
- `frontend/src/components/SentryTest.tsx` - Dev-only test button component

## Architecture Impact
- **Subsystems affected**: Frontend only
- **New dependencies**: `@sentry/react`
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add Sentry Dependency
**File**: `frontend/package.json`
**Action**: MODIFY (via pnpm)

**Implementation**:
```bash
cd frontend && pnpm add @sentry/react
```

**Validation**:
```bash
cd frontend && pnpm lint
```

---

### Task 2: Initialize Sentry in main.tsx
**File**: `frontend/src/main.tsx`
**Action**: MODIFY
**Pattern**: Reference openreplay.ts for env var pattern, backend main.go for Sentry init

**Implementation**:
Add at the TOP of main.tsx, before any other imports:
```typescript
import * as Sentry from '@sentry/react';

// Initialize Sentry for error tracking (disabled if DSN not set)
if (import.meta.env.VITE_SENTRY_DSN) {
  Sentry.init({
    dsn: import.meta.env.VITE_SENTRY_DSN,
    environment: import.meta.env.MODE,
    enabled: true,
  });
}
```

**Note**: Must be BEFORE React imports to capture errors during initial render.

**Validation**:
```bash
cd frontend && pnpm lint && pnpm typecheck
```

---

### Task 3: Update ErrorBoundary to Report to Sentry
**File**: `frontend/src/components/ErrorBoundary.tsx`
**Action**: MODIFY
**Pattern**: Reference existing componentDidCatch implementation

**Implementation**:
```typescript
// Add import at top
import * as Sentry from '@sentry/react';

// Update componentDidCatch method
public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
  console.error(`Error in ${this.props.name || 'component'}:`, error, errorInfo);

  // Report to Sentry with component context
  Sentry.withScope((scope) => {
    scope.setTag('component', this.props.name || 'unknown');
    scope.setExtra('componentStack', errorInfo.componentStack);
    Sentry.captureException(error);
  });
}
```

**Validation**:
```bash
cd frontend && pnpm lint && pnpm typecheck
```

---

### Task 4: Add User Context to Auth Store
**File**: `frontend/src/stores/authStore.ts`
**Action**: MODIFY
**Pattern**: Reference existing login/logout actions

**Implementation**:
```typescript
// Add import at top
import * as Sentry from '@sentry/react';

// In login action, after successful login (around line 55):
// Set Sentry user context
Sentry.setUser({
  id: String(user.id),
  email: user.email,
});

// In logout action (around line 137):
// Clear Sentry user context
Sentry.setUser(null);

// In initialize action, after confirming authenticated (around line 164):
// Restore Sentry user context from persisted state
if (state.user) {
  Sentry.setUser({
    id: String(state.user.id),
    email: state.user.email,
  });
}
```

**Validation**:
```bash
cd frontend && pnpm lint && pnpm typecheck && pnpm test
```

---

### Task 5: Create Dev-Only Sentry Test Component
**File**: `frontend/src/components/SentryTest.tsx`
**Action**: CREATE
**Pattern**: Reference main.tsx dev-only block

**Implementation**:
```typescript
import * as Sentry from '@sentry/react';

/**
 * Dev-only component to test Sentry integration.
 * Renders a button that captures a test error.
 */
export function SentryTest() {
  if (!import.meta.env.DEV) {
    return null;
  }

  const handleTestError = () => {
    Sentry.captureException(new Error('Sentry test error from React frontend'));
    alert('Test error sent to Sentry! Check dashboard.');
  };

  const handleTestCrash = () => {
    throw new Error('Sentry test crash - this should be caught by ErrorBoundary');
  };

  return (
    <div style={{ padding: '10px', margin: '10px', border: '1px dashed orange' }}>
      <strong>Sentry Test (Dev Only)</strong>
      <div style={{ marginTop: '8px', display: 'flex', gap: '8px' }}>
        <button
          onClick={handleTestError}
          style={{ padding: '4px 8px', cursor: 'pointer' }}
        >
          Send Test Error
        </button>
        <button
          onClick={handleTestCrash}
          style={{ padding: '4px 8px', cursor: 'pointer', backgroundColor: '#ffcccc' }}
        >
          Trigger Crash
        </button>
      </div>
    </div>
  );
}
```

**Validation**:
```bash
cd frontend && pnpm lint && pnpm typecheck
```

---

### Task 6: Add SentryTest to App (Dev Only)
**File**: `frontend/src/App.tsx`
**Action**: MODIFY

**Implementation**:
Add import and render in dev mode only:
```typescript
// Add import
import { SentryTest } from '@/components/SentryTest';

// Add inside the main container, after Toaster (around line 255):
{import.meta.env.DEV && <SentryTest />}
```

**Validation**:
```bash
cd frontend && pnpm lint && pnpm typecheck
```

---

### Task 7: Final Validation
**Action**: Full test suite and build

**Validation**:
```bash
cd frontend && pnpm validate
```

## Risk Assessment

- **Risk**: Sentry import increases bundle size
  **Mitigation**: @sentry/react uses tree-shaking. Minimal impact (~30KB gzipped).

- **Risk**: Sentry calls fail silently in dev without DSN
  **Mitigation**: SDK is disabled when DSN not set. No errors thrown.

- **Risk**: User PII sent to Sentry
  **Mitigation**: Only sending user ID and email (already in JWT). No sensitive data.

## Integration Points

- **Auth Store**: Set/clear user context on login/logout/initialize
- **ErrorBoundary**: Existing component gains Sentry reporting
- **Environment Variables**: `VITE_SENTRY_DSN` (already configured in Railway)

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd frontend && pnpm lint      # Gate 1: Syntax & Style
cd frontend && pnpm typecheck # Gate 2: Type Safety
cd frontend && pnpm test      # Gate 3: Unit Tests
```

**Final validation**:
```bash
cd frontend && pnpm validate
```

## Verification After Deploy

1. Deploy to preview environment
2. Click "Send Test Error" button (dev mode) or use browser console
3. Verify error appears in Sentry dashboard with:
   - Stack trace
   - Environment = preview (or development)
   - User context (if logged in)
4. Test "Trigger Crash" to verify ErrorBoundary integration

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar pattern already implemented (backend Sentry)
- ✅ OpenReplay integration provides env var pattern reference
- ✅ Existing ErrorBoundary to integrate with
- ✅ All clarifying questions answered
- ✅ Railway env vars already configured

**Assessment**: Straightforward SDK integration following established patterns.

**Estimated one-pass success probability**: 95%

**Reasoning**: Single dependency, clear integration points, existing patterns to follow, Railway already configured.
