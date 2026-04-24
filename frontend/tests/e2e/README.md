# E2E Tests

End-to-end test suite for the TrakRF Handheld application using Playwright.

## Quick Start

```bash
# Run all tests
pnpm test:e2e

# Run by category (tag-based execution)
pnpm test:ui-only    # UI tests with mocks (no hardware)
pnpm test:hardware   # Tests requiring CS108 device
pnpm test:smoke      # Quick validation (<2 min)
pnpm test:critical   # Production-critical tests

# Run specific test
pnpm exec playwright test inventory.spec.ts

# Debug with UI
pnpm exec playwright test --ui
```

## Test Categories

Tests are tagged for selective execution:

| Tag | Purpose | Hardware Required |
|-----|---------|-------------------|
| `@ui-only` | UI functionality with mocks | No |
| `@hardware` | Real device integration | Yes (CS108) |
| `@smoke` | Quick CI/CD validation | No |
| `@critical` | Production-blocking features | Varies |

## Prerequisites

### For UI-Only Tests
- Node.js 18+
- No additional requirements (uses mocks)

### For Hardware Tests
1. **CS108 RFID Reader** powered on
2. **Test Tags** positioned in front of reader (see test-utils/constants.ts for tag definitions)
3. **Bridge Server** running: `pnpm dlx ble-mcp-test`

## Project Structure

```
tests/e2e/
├── *.spec.ts              # Test specifications
├── helpers/               # Shared test utilities
│   ├── assertions.ts      # Custom matchers
│   ├── ble-integration.ts # BLE mock/bridge utilities
│   ├── commands.ts        # CS108 command helpers
│   ├── connection.ts      # Device connection helpers
│   ├── console-utils.ts   # Console monitoring
│   └── trigger-utils.ts   # Trigger simulation
├── e2e.config.ts         # Configuration constants
├── e2e.setup.ts          # Global test setup
└── test-setup.ts         # Playwright exports
```

## Writing Tests

### Basic Structure

```typescript
import { test, expect } from './test-setup';

test.describe('Feature Name', () => {
  test('@ui-only should do something', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.element')).toBeVisible();
  });
});
```

### Important Rules

1. **NEVER nest test.describe()** - Causes Playwright to hang
2. **Keep tests focused** - One feature per test file
3. **Use helpers** - Don't duplicate connection/assertion logic
4. **Tag appropriately** - Ensures tests run in correct environment

### Common Patterns

#### Device Connection
```typescript
import { connectToDevice } from './helpers/connection';

test('should connect', async ({ page }) => {
  await connectToDevice(page);
  // Device is now connected
});
```

#### Trigger Simulation
```typescript
import { pressTrigger, releaseTrigger } from './helpers/trigger-utils';

test('@ui-only trigger starts inventory', async ({ page }) => {
  await pressTrigger(page);
  // Inventory starts
  await releaseTrigger(page);
  // Inventory stops
});
```

#### Custom Assertions
```typescript
import { expectConnected } from './helpers/assertions';

test('verify connection', async ({ page }) => {
  await expectConnected(page);
});
```

## Environment Configuration

### .env.local
```bash
# For UI-only tests (default)
VITE_BLE_BRIDGE_ENABLED=true

# For hardware tests
BLE_MCP_HOST=localhost
BLE_MCP_WS_PORT=8080
```

## Running Against a Remote Deployment

The suite can be run against any deployed environment (preview, gke, staging,
etc.) without booting a local Vite server. Useful for smoke-testing a
deployment after a release or cluster change.

```bash
# From project root
just frontend test-e2e-remote https://gke.trakrf.app

# Or directly
cd frontend && PLAYWRIGHT_BASE_URL=https://gke.trakrf.app pnpm test:e2e

# Single spec
PLAYWRIGHT_BASE_URL=https://gke.trakrf.app pnpm exec playwright test auth.spec.ts
```

When `PLAYWRIGHT_BASE_URL` is set, the config skips the local `webServer`
launch — the remote deployment is treated as the server.

### Skipping hardware-dependent specs

Specs tagged `@hardware` require a physical CS108 reader on the same machine
as the test runner, which isn't available when pointing at a remote URL.
Exclude them:

```bash
PLAYWRIGHT_BASE_URL=https://gke.trakrf.app \
  pnpm exec playwright test --grep-invert "@hardware"
```

Or pass an explicit file list of non-hardware specs.

### Provisioning a remote test user

Fixtures perform signup per test with unique timestamped emails, so remote
runs don't require pre-seeded accounts. For a one-off check:

```bash
ts=$(date +%s)
curl -sS -X POST https://gke.trakrf.app/api/v1/auth/signup \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"mike-remotetest-${ts}@trakrf.id\",\"password\":\"Passw0rd!123\",\"org_name\":\"remote-test-${ts}\"}"
```

A 201 with a JWT confirms auth + DB + JWT signing are healthy.

### Verifying deployed commit vs local

Until `/api/v1/version` exists (see `TRA-481`), check the tag pinned in the
infra repo:

```bash
grep -E '^\s+tag:' /home/mike/trakrf-infra/helm/trakrf-backend/values-gke.yaml
# → tag: sha-<shortsha>
```

Compare against `git log --oneline <shortsha>..HEAD` in this repo to gauge
drift before attributing test failures to real bugs.

## Test Coverage Areas

### Core Functionality
- Device connection/disconnection
- Battery monitoring
- Trigger button behavior
- Error recovery

### Inventory Operations
- Start/stop inventory
- Tag accumulation
- Duplicate handling
- Export functionality

### Locate Mode
- Target tag selection
- RSSI-based proximity
- Visual/audio feedback

### Settings
- Power configuration
- Session management
- Filter settings
- Persistence

### Barcode
- Scanner activation
- Data capture
- Integration with inventory

## Debugging

### Console Monitoring
Tests automatically monitor console for errors. Any unexpected errors will fail the test.

### Viewing Mock Traffic
```bash
# Enable debug logging
DEBUG=ble:* pnpm test:e2e
```

### Using Playwright UI
```bash
pnpm exec playwright test --ui
```
This provides:
- Step-by-step execution
- DOM inspection
- Network monitoring
- Console output

## CI/CD Integration

Tests run automatically on push/PR:
- Smoke tests run first (fast feedback)
- Full suite runs if smoke passes
- Hardware tests skipped in CI (no device available)

## Common Issues

### "test.describe() called unexpectedly"
- Don't use `pnpm dlx playwright`, use `pnpm exec playwright`
- Ensure no nested test.describe() blocks

### "Element not found"
- Check data-testid attributes exist
- Verify page navigation completed
- Increase timeout for BLE operations

### "Bridge not injected"
- Ensure dev server running: `pnpm dev:bridge`
- Check VITE_BLE_BRIDGE_ENABLED=true
- Verify bridge injection in browser console

## Performance

- UI-only tests: ~30 seconds
- Hardware tests: ~2 minutes (depends on device)
- Smoke tests: <2 minutes
- Full suite: ~5 minutes

## Best Practices

1. **Use tags** - Enable selective test execution
2. **Keep tests independent** - No shared state between tests
3. **Use helpers** - Reduce duplication, improve maintainability
4. **Monitor console** - Catch JavaScript errors early
5. **Test user workflows** - Not implementation details