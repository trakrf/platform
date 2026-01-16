# Implementation Plan: Environment Banner

Generated: 2026-01-15
Specification: spec.md

## Understanding

Add a visual environment indicator to the TrakRF app that:
1. Shows a colored banner for non-production environments (dev=orange, staging=purple)
2. Prefixes the page title with `[DEV]` or `[STG]`
3. Shows nothing in production (clean UI)
4. Always visible, no dismiss option

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/Header.tsx` - Simple functional component pattern
- `frontend/src/components/__tests__/Header.test.tsx` - Test patterns with vitest/RTL
- `frontend/src/lib/openreplay.ts` (lines 6-7) - Environment variable access pattern
- `frontend/src/App.tsx` (line 258) - Conditional rendering: `{import.meta.env.DEV && <SentryTest />}`

**Files to Create**:
- `frontend/src/components/EnvironmentBanner.tsx` - Banner component with title effect
- `frontend/src/components/__tests__/EnvironmentBanner.test.tsx` - Unit tests

**Files to Modify**:
- `frontend/src/App.tsx` (line ~244) - Add banner above Toaster in layout
- `frontend/.env.local.example` - Document VITE_ENVIRONMENT variable

## Architecture Impact
- **Subsystems affected**: UI only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create EnvironmentBanner Component
**File**: `frontend/src/components/EnvironmentBanner.tsx`
**Action**: CREATE
**Pattern**: Reference `frontend/src/components/Header.tsx` for component structure

**Implementation**:
```tsx
import { useEffect } from 'react';

type Environment = 'dev' | 'staging' | 'prod';

interface EnvConfig {
  label: string;
  titlePrefix: string;
  bgColor: string;
}

const ENV_CONFIG: Record<Environment, EnvConfig | null> = {
  dev: {
    label: 'Development Environment',
    titlePrefix: '[DEV]',
    bgColor: 'bg-orange-500'
  },
  staging: {
    label: 'Staging Environment',
    titlePrefix: '[STG]',
    bgColor: 'bg-purple-600'
  },
  prod: null,
};

function getEnvironment(): Environment {
  const env = import.meta.env.VITE_ENVIRONMENT;
  if (env === 'dev' || env === 'staging') return env;
  return 'prod'; // Default to prod (shows nothing)
}

export function EnvironmentBanner() {
  const environment = getEnvironment();
  const config = ENV_CONFIG[environment];

  // Update page title with environment prefix
  useEffect(() => {
    if (!config) return;

    const baseTitle = 'TrakRF';
    document.title = `${config.titlePrefix} ${baseTitle}`;

    return () => {
      document.title = baseTitle;
    };
  }, [config]);

  if (!config) return null;

  return (
    <div
      className={`${config.bgColor} text-white text-center text-sm py-1 font-medium`}
      data-testid="environment-banner"
    >
      {config.label}
    </div>
  );
}
```

**Validation**:
- `just frontend lint`
- `just frontend typecheck`

---

### Task 2: Create Unit Tests
**File**: `frontend/src/components/__tests__/EnvironmentBanner.test.tsx`
**Action**: CREATE
**Pattern**: Reference `frontend/src/components/__tests__/Header.test.tsx`

**Implementation**:
```tsx
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { EnvironmentBanner } from '@/components/EnvironmentBanner';

describe('EnvironmentBanner', () => {
  const originalTitle = document.title;

  beforeEach(() => {
    document.title = 'TrakRF';
  });

  afterEach(() => {
    cleanup();
    document.title = originalTitle;
    vi.unstubAllEnvs();
  });

  it('should show orange banner for dev environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'dev');
    render(<EnvironmentBanner />);

    const banner = screen.getByTestId('environment-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('Development Environment');
    expect(banner).toHaveClass('bg-orange-500');
  });

  it('should show purple banner for staging environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'staging');
    render(<EnvironmentBanner />);

    const banner = screen.getByTestId('environment-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('Staging Environment');
    expect(banner).toHaveClass('bg-purple-600');
  });

  it('should render nothing for prod environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'prod');
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should render nothing when VITE_ENVIRONMENT is empty', () => {
    vi.stubEnv('VITE_ENVIRONMENT', '');
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should render nothing when VITE_ENVIRONMENT is undefined', () => {
    vi.stubEnv('VITE_ENVIRONMENT', undefined);
    render(<EnvironmentBanner />);

    expect(screen.queryByTestId('environment-banner')).not.toBeInTheDocument();
  });

  it('should set page title with [DEV] prefix for dev environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'dev');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('[DEV] TrakRF');
  });

  it('should set page title with [STG] prefix for staging environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'staging');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('[STG] TrakRF');
  });

  it('should not modify page title for prod environment', () => {
    vi.stubEnv('VITE_ENVIRONMENT', 'prod');
    render(<EnvironmentBanner />);

    expect(document.title).toBe('TrakRF');
  });
});
```

**Validation**:
- `just frontend test`

---

### Task 3: Integrate Banner in App.tsx
**File**: `frontend/src/App.tsx`
**Action**: MODIFY
**Location**: Around line 244, at the start of the main layout div

**Changes**:
1. Add import at top of file
2. Add `<EnvironmentBanner />` as first child inside the main container div

**Implementation**:
```tsx
// Add import (near other component imports, ~line 6)
import { EnvironmentBanner } from '@/components/EnvironmentBanner';

// Add banner at line ~244, right after opening div
return (
  <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex relative">
    <EnvironmentBanner />  {/* ADD THIS LINE */}
    <Toaster
      ...
```

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test`

---

### Task 4: Document Environment Variable
**File**: `frontend/.env.local.example`
**Action**: MODIFY
**Location**: Add new section after "Development Settings" (~line 53)

**Implementation**:
```bash
# -----------------------------------------------------------------------------
# Environment Identification
# -----------------------------------------------------------------------------
# Environment indicator shown in UI banner and page title
# Values: dev, staging, prod (or empty)
# - dev: Orange banner, [DEV] title prefix
# - staging: Purple banner, [STG] title prefix
# - prod or empty: No banner (clean production UI)
# VITE_ENVIRONMENT=dev
```

**Validation**:
- Visual inspection

---

## Risk Assessment

- **Risk**: `vi.stubEnv` may not work as expected in vitest
  **Mitigation**: Test locally; fallback to manual mock of `import.meta.env`

- **Risk**: Title effect cleanup may cause flicker
  **Mitigation**: Only set title once on mount, cleanup on unmount

## Integration Points
- Store updates: None required
- Route changes: None
- Config updates: New `VITE_ENVIRONMENT` variable

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
- Gate 1: `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `just frontend test`

**Final validation**: `just frontend validate`

## Validation Sequence

After each task:
1. `just frontend lint`
2. `just frontend typecheck`
3. `just frontend test`

Final: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar patterns found: `Header.tsx`, `openreplay.ts`
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow: `Header.test.tsx`
- ✅ No new dependencies
- ✅ Single subsystem (UI only)

**Assessment**: Straightforward feature with well-established patterns in the codebase.

**Estimated one-pass success probability**: 95%

**Reasoning**: Simple component with clear requirements, existing patterns to follow, and minimal integration points. Only minor risk is vitest env mocking behavior.
