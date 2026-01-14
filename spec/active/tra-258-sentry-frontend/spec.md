# Feature: Add Sentry Error Tracking to React Frontend

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear Issue**: [TRA-258](https://linear.app/trakrf/issue/TRA-258/add-sentry-to-react-frontend)
**Parent**: [TRA-148](https://linear.app/trakrf/issue/TRA-148/add-production-monitoring-and-alerting) - Production monitoring initiative
**Related**: [TRA-257](https://linear.app/trakrf/issue/TRA-257/add-sentry-to-go-backend) - Backend Sentry (completed)

## Outcome
Frontend JavaScript errors and React component crashes will be automatically captured and reported to Sentry with full stack traces, component hierarchy, and user context, complementing the backend error tracking already in place.

## User Story
As a **developer/operator**
I want **frontend errors to be automatically captured and reported to Sentry**
So that **I can quickly identify and fix JavaScript errors, React crashes, and user-impacting issues before customers report them**

## Context
**Current**: The frontend has a custom `ErrorBoundary` component (`frontend/src/components/ErrorBoundary.tsx`) that catches React errors and displays a fallback UI with console logging, but errors are not aggregated or alerted.

**Desired**: Errors captured by Sentry with:
- Full JavaScript stack traces with source maps
- React component hierarchy
- User context (user ID, org ID if authenticated)
- Environment separation (preview vs production)
- Integration with existing ErrorBoundary

**Why Now**: Backend Sentry integration (TRA-257) just shipped. This completes the full-stack error monitoring needed before accepting payments (NADA verbal commitment).

## Technical Requirements

### 1. Sentry SDK Installation
- Add `@sentry/react` dependency via pnpm
- Initialize in `main.tsx` before React renders

### 2. Sentry Initialization
```typescript
// In main.tsx, before ReactDOM.createRoot
import * as Sentry from "@sentry/react";

Sentry.init({
  dsn: import.meta.env.VITE_SENTRY_DSN,
  environment: import.meta.env.MODE, // 'development', 'production'
  release: import.meta.env.VITE_APP_VERSION || '0.0.0',
  enabled: !!import.meta.env.VITE_SENTRY_DSN, // Disabled if DSN not set
});
```

### 3. ErrorBoundary Integration
Update existing `ErrorBoundary` component to report to Sentry:
```typescript
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

### 4. User Context (Optional Enhancement)
Set user context when authenticated:
```typescript
// In auth store or after login
Sentry.setUser({
  id: user.id,
  email: user.email,
});

// On logout
Sentry.setUser(null);
```

### 5. Environment Variables
- `VITE_SENTRY_DSN` - Sentry DSN (empty = disabled)
- Uses existing `MODE` for environment tag
- Add `VITE_APP_VERSION` for release tracking (optional)

### 6. Railway Configuration
- Add `VITE_SENTRY_DSN` to preview and production environments
- Use same DSN as backend (same Sentry project) for correlated errors
- Keep empty in local development

## Implementation Notes

**Entry Point** (`main.tsx`):
- Initialize Sentry BEFORE `ReactDOM.createRoot()`
- Sentry must capture errors during initial render

**Existing ErrorBoundary** (`components/ErrorBoundary.tsx`):
- Already wraps Header, TabNavigation, Tab Content in App.tsx
- Add Sentry reporting without changing the UI behavior

**Source Maps** (future enhancement):
- Vite generates source maps in production build
- Can upload to Sentry for readable stack traces (separate task)

## Out of Scope
- Source map upload to Sentry (can be added later)
- Performance/tracing (`enableTracing`)
- Session replay
- Custom error boundaries beyond the existing one
- Breadcrumbs customization

## Validation Criteria
- [ ] Sentry SDK compiles without errors
- [ ] `VITE_SENTRY_DSN` empty = no Sentry calls (safe for local dev)
- [ ] Unhandled JavaScript errors are captured
- [ ] React component errors (caught by ErrorBoundary) are captured
- [ ] Captured events include: stack trace, component name, environment
- [ ] Frontend tests still pass (`pnpm test`)
- [ ] Build succeeds (`pnpm build`)

## Success Metrics
- [ ] Test error appears in Sentry dashboard
- [ ] Error includes component context (which ErrorBoundary caught it)
- [ ] Environment shows correctly (preview vs production)
- [ ] Errors are grouped with backend errors in same Sentry project

## Verification Steps
1. Deploy to preview environment with `VITE_SENTRY_DSN` configured
2. Trigger a test error (e.g., temporary `throw new Error()` in a component)
3. Confirm error appears in Sentry with full context
4. Remove test error code
5. Verify existing ErrorBoundary still shows fallback UI

## References
- [Sentry React SDK](https://docs.sentry.io/platforms/javascript/guides/react/)
- [Sentry Error Boundary](https://docs.sentry.io/platforms/javascript/guides/react/features/error-boundary/)
- Existing ErrorBoundary: `frontend/src/components/ErrorBoundary.tsx`
- Entry point: `frontend/src/main.tsx`
- Backend Sentry (for reference): `backend/main.go` lines 160-176
