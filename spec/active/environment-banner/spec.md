# Feature: Environment Banner

## Origin
Linear issue TRA-282 - Add visual indicator to distinguish non-production environments from production.

## Outcome
Users and developers can immediately identify which environment they're working in, preventing accidental operations on production data.

## User Story
As a TrakRF user or developer
I want to clearly see which environment I'm using
So that I don't accidentally perform destructive actions in production

## Context
**Problem**: Without clear visual differentiation, users may confuse dev/staging with production, leading to potential data issues or wasted effort on test environments.

**Current**: No environment indicator exists. All environments look identical.

**Desired**: Non-production environments display a prominent, color-coded banner. Production shows nothing (clean UI).

## Technical Requirements

### Environment Variable
- **Name**: `VITE_ENVIRONMENT`
- **Values**: `dev`, `staging`, `prod` (or empty)
- **Behavior**: If empty or `prod`, show nothing

### Visual Components

1. **Top Banner** (Primary indicator)
   - Thin persistent bar above the Header component
   - Full-width, always visible
   - Color-coded by environment:
     - `dev`: Orange (`bg-orange-500`)
     - `staging`: Purple (`bg-purple-600`)
   - Text: "Development Environment" / "Staging Environment"
   - Small, readable font; high contrast white text

2. **Favicon Badge** (Optional enhancement)
   - Different favicon per environment
   - Helps identify correct tab when multiple environments open

3. **Page Title Prefix** (Optional enhancement)
   - Prefix `<title>` with environment: "[DEV] TrakRF" / "[STG] TrakRF"

### Implementation Location
- New component: `frontend/src/components/EnvironmentBanner.tsx`
- Integration point: `frontend/src/App.tsx` (above Header)

### Color Palette (Fixed)
| Environment | Banner Color | Text Color |
|-------------|--------------|------------|
| dev         | orange-500   | white      |
| staging     | purple-600   | white      |
| prod        | (none)       | (none)     |

## Non-Goals
- Watermarks (too intrusive for MVP)
- Destructive action confirmations (separate feature)
- Email/notification environment labeling (separate feature)

## Code Example

```tsx
// EnvironmentBanner.tsx
const ENVIRONMENT = import.meta.env.VITE_ENVIRONMENT || 'prod';

const ENV_CONFIG: Record<string, { label: string; bgColor: string } | null> = {
  dev: { label: 'Development Environment', bgColor: 'bg-orange-500' },
  staging: { label: 'Staging Environment', bgColor: 'bg-purple-600' },
  prod: null,
};

export function EnvironmentBanner() {
  const config = ENV_CONFIG[ENVIRONMENT];
  if (!config) return null;

  return (
    <div className={`${config.bgColor} text-white text-center text-sm py-1 font-medium`}>
      {config.label}
    </div>
  );
}
```

## Validation Criteria
- [ ] Banner visible in dev environment with orange color
- [ ] Banner visible in staging environment with purple color
- [ ] No banner in production (empty or `prod` value)
- [ ] Banner persists across all pages/tabs
- [ ] Banner does not interfere with Header or navigation
- [ ] Responsive: readable on mobile and desktop
- [ ] Unit test for environment config logic

## Files to Modify
1. `frontend/src/components/EnvironmentBanner.tsx` (new)
2. `frontend/src/App.tsx` (add banner above Header)
3. `frontend/.env.example` (document VITE_ENVIRONMENT)

## References
- Linear: [TRA-282](https://linear.app/trakrf/issue/TRA-282)
- Related: TRA-270 (BLE issues - unrelated but mentioned in context)
