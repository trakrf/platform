# Mock Usage Guide

This guide consolidates all information about using BLE mocks in the TrakRF Handheld project.

## Overview

The project uses the `ble-mcp-test` package to mock Web Bluetooth API for:
- Development without hardware
- E2E testing (required due to browser security)
- Debugging BLE communication

## When Mocks Are Required

### E2E Tests - ALWAYS Required
E2E tests **must always use mocks** because:
- Web Bluetooth API requires user gestures (click/tap) for device selection
- Automated test tools (Playwright) cannot simulate these gestures
- This is a browser security feature, not a limitation

### Development - Optional
Mocks are optional for development:
- Use `pnpm dev` for standard development (real BLE)
- Use `pnpm dev:mock` for development without hardware

## Mock Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────────┐
│   Browser   │────▶│ Mock WebBLE  │────▶│ Bridge Server   │
│ (Your App)  │◀────│   Client     │◀────│ (ble-mcp-test) │
└─────────────┘     └──────────────┘     └─────────────────┘
                         WebSocket              │
                                               ▼
                                        ┌─────────────┐
                                        │ Real CS108  │
                                        │  (Optional) │
                                        └─────────────┘
```

## Configuration

### Environment Variables

After recent consolidation, the project uses simplified environment variables:

```bash
# Bridge server configuration (simplified naming)
BLE_MCP_HOST=localhost          # Bridge server host
BLE_MCP_WS_PORT=8080           # Bridge server WebSocket port
BLE_MCP_HTTP_PORT=8081         # Bridge server HTTP/MCP port
BLE_MCP_HTTP_TOKEN=            # Optional auth token
BLE_MCP_LOG_LEVEL=info         # Bridge server log level

# Device configuration
BLE_DEVICE_NAME=CS108          # Device name/MAC address
BLE_SERVICE_UUID=9800          # BLE service UUID
BLE_WRITE_UUID=9900            # BLE write characteristic UUID
BLE_NOTIFY_UUID=9901           # BLE notify characteristic UUID
BLE_SESSION_ID=                # Optional: Override auto-generated session ID

# Mock control
VITE_BLE_MOCK_ENABLED=true     # Enable mock in development
```

The bridge URL is now built automatically from components:
- WebSocket URL: `ws://${BLE_MCP_HOST}:${BLE_MCP_WS_PORT}`
- No need to manually set `VITE_BLE_BRIDGE_URL` anymore

### Configuration Files

1. **Development**: `.env.local`
   ```bash
   # For local mock server
   BLE_MCP_HOST=localhost
   BLE_MCP_WS_PORT=8080
   
   # For external bridge (e.g., hardware on another machine)
   BLE_MCP_HOST=192.168.50.77
   BLE_MCP_WS_PORT=8080
   ```

2. **Shared Configuration**: `tests/config/ble-bridge.config.ts`
   - Central configuration for all test contexts
   - Ensures consistent session IDs for connection pool reuse
   - Used by Vite, E2E tests, and integration tests

## Usage Patterns

### 1. Development with Mock

```bash
# Start dev server with automatic mock injection
pnpm dev:mock

# This automatically:
# 1. Checks if bridge server is available
# 2. Injects mock into browser via Vite
# 3. Routes all BLE calls through mock
```

### 2. E2E Testing (Simplified Approach)

**IMPORTANT**: E2E tests now require a dev server to be running first.

```bash
# Terminal 1: Start mock server
pnpm dev:mock

# Terminal 2: Run tests
pnpm test:e2e:mock              # All tests with mock
pnpm test:e2e:mock <file>       # Specific test file

# For real device testing:
# Terminal 1: pnpm dev
# Terminal 2: pnpm test:e2e
```

**Why this approach?**
- Tests focus on UI behavior, not infrastructure
- No mock injection complexity in tests
- Faster test execution (no setup overhead)
- Clear separation of concerns

**CI/CD Setup:**
```yaml
# Start server in CI mode (auto-starts on port 5173)
- run: CI=true USE_MOCK=true pnpm test:e2e
```

### 3. Manual Mock Server

```bash
# Start mock server manually
pnpm dlx ble-mcp-test serve

# With custom port
PORT=9090 pnpm dlx ble-mcp-test serve

# Check server status
curl http://localhost:8081/status
```

### 4. External Bridge Server

For testing with real hardware on another machine:

```bash
# On machine with CS108 connected
cd path/to/ble-mcp-test
pnpm serve

# On development machine (.env.local)
BLE_MCP_HOST=192.168.50.77
BLE_MCP_WS_PORT=8080

# Then run
pnpm dev:mock
```

## Mock Behavior

### Device Availability Modes

Configure via URL parameter: `?availability=<mode>`

- `available` (default) - Device found and connectable
- `none` - No devices found
- `timeout` - Connection attempt times out
- `mock` - Special mock-only behaviors

### Mock vs Real Device Differences

1. **Trigger Behavior**: Mock may not perfectly replicate trigger press/release timing
2. **Response Timing**: Mock responses are instant, real devices have latency
3. **Error States**: Some hardware-specific errors may not be simulated

## Debugging

### Enable Debug Logging

```bash
# Browser console
localStorage.setItem('debug', 'ble:*')

# Server logs
DEBUG=* pnpm dlx ble-mcp-test serve
```

### Monitor BLE Traffic

```bash
# Real-time log streaming
wscat -c "ws://localhost:8080?command=log-stream"

# Use MCP tools (if configured)
mcp__ble-mcp-test__get_logs
mcp__ble-mcp-test__get_connection_state
```

### Common Issues

1. **"Device not found"**
   - Check mock server is running: `curl http://localhost:8081/status`
   - Verify environment variables are set
   - Check browser console for injection confirmation

2. **"WebSocket connection failed"**
   - Ensure no firewall blocking WebSocket
   - Try different port if 8080 is in use
   - Check server logs for errors

3. **"Mock not injected"**
   - Must inject before page navigation
   - Check for `[WebBLE Adapter] Injected` in console
   - Verify Vite config includes mock plugin

## Best Practices

1. **Always use mocks for E2E tests** - No exceptions
2. **Use real devices for final testing** - Mocks can't catch all issues
3. **Log bridge traffic when debugging** - Essential for troubleshooting
4. **Keep mock server updated** - Sync with latest CS108 behavior
5. **Document mock limitations** - Note any behavioral differences

## Current Limitations

1. **Static Device Configuration**: Device parameters must be known at injection time
2. **No Dynamic Discovery**: Can't change device without restart
3. **Trigger Release Issue**: Mock doesn't properly stop inventory on trigger release
4. **Single Device Only**: Can't simulate multiple devices

## Testing Philosophy

### What Changed (January 2025)

We simplified E2E testing by removing mock injection from test code:

**Before**: Tests manually injected mocks, managed console tracking, complex setup
**After**: Tests assume server provides appropriate BLE interface (mock or real)

**Benefits**:
- No more 2-minute timeouts from complex test setup
- Tests focus on UI behavior only
- Same tests work with mock or real devices
- Cleaner, more maintainable test code

### Writing E2E Tests

```typescript
// Simple, clean tests - no mock awareness needed
test('connect to device', async ({ page }) => {
  await page.goto('/');
  await page.click('[data-testid="connect-button"]');
  
  // Mock or real device will handle BLE communication
  await expect(page.locator('[data-testid="device-connected"]')).toBeVisible();
});
```

### Test Categories and Complexity

#### 1. Simple UI Tests (Easy)
Tests that only interact with UI elements:
```typescript
test('hamburger menu opens', async ({ page }) => {
  await page.goto('/');
  await page.click('[data-testid="hamburger-button"]');
  await expect(page.locator('[data-testid="hamburger-dropdown"]')).toBeVisible();
});
```

#### 2. Device Connection Tests (Medium)
Tests that require BLE communication but have predictable outcomes:
```typescript
test('shows battery level after connection', async ({ page }) => {
  await page.goto('/');
  await page.click('[data-testid="connect-button"]');
  
  // Wait for connection and battery query
  await expect(page.locator('[data-testid="battery-level"]')).toBeVisible({ timeout: 10000 });
  await expect(page.locator('[data-testid="battery-level"]')).toContainText('%');
});
```

#### 3. Data Flow Tests (Complex)
Tests involving full command/response cycles with data parsing:
```typescript
test('inventory reads tags', async ({ page }) => {
  await page.goto('/inventory');
  
  // Start inventory
  await page.click('[data-testid="start-inventory"]');
  
  // Wait for tags to appear (mock provides consistent test tags)
  await expect(page.locator('[data-testid="tag-list"] >> nth=0')).toBeVisible({ timeout: 15000 });
  
  // Verify tag data is parsed correctly
  const firstTag = page.locator('[data-testid="tag-list"] >> nth=0');
  await expect(firstTag).toContainText('EPC'); // Has EPC data
  await expect(firstTag).toContainText('RSSI'); // Has signal strength
});
```

### Handling Timing and Async Operations

#### Use Appropriate Timeouts
```typescript
// Quick UI changes: default 5s timeout is fine
await expect(page.locator('.menu')).toBeVisible();

// BLE operations: may need longer
await expect(page.locator('.connected')).toBeVisible({ timeout: 10000 });

// Complex operations (inventory): even longer
await expect(page.locator('.tag-count')).toContainText('10', { timeout: 30000 });
```

#### Wait for Stable State
```typescript
// Bad: Fixed delay
await page.waitForTimeout(5000); // Avoid this!

// Good: Wait for specific condition
await page.waitForSelector('[data-testid="inventory-complete"]');
await expect(page.locator('.status')).toContainText('Ready');
```

### Mock vs Real Device Considerations

#### What the Mock Provides
- Consistent device discovery (always finds "CS108")
- Predictable connection success
- Test tags (EPCs 10018-10023) for inventory
- Stable battery level (90%)
- Consistent RSSI values

#### What to Test vs What to Skip

**DO Test:**
- UI responds correctly to device states
- Data is displayed properly when received
- User workflows complete successfully
- Error states are handled gracefully

**DON'T Test:**
- Exact packet formats (unit test this)
- Protocol implementation details
- Timing-sensitive operations
- Hardware-specific edge cases

### Debugging Failed Tests

#### 1. Check Server is Running
```bash
pnpm dev:check
# Should show: "🔌 Mock server is running"
```

#### 2. Watch Server Logs
In the terminal running `pnpm dev:mock`, look for:
- WebSocket connections
- Command/response traffic
- Any error messages

#### 3. Use Playwright UI Mode
```bash
pnpm test:e2e:ui
```
This lets you:
- See the browser
- Step through tests
- Inspect elements
- Check network traffic

#### 4. Common Issues

**"Element not found"**
- Check data-testid attributes exist
- Verify correct page navigation
- Ensure mock is providing expected data

**"Timeout waiting for element"**
- Increase timeout for BLE operations
- Check if mock server is responding
- Verify WebSocket connection in server logs

**"Unexpected test behavior"**
- Mock may be in different state than expected
- Previous test may have left connection open
- Check for JavaScript errors in browser console

### Future Improvements

#### Web Worker Architecture (In Progress)
Moving device communication to a Web Worker will enable:
- Better test isolation (mock at worker boundary)
- Cleaner separation of UI and device logic  
- More reliable E2E tests for complex operations
- Ability to unit test protocol logic independently

See `src/worker/docs/CS108-REFACTOR-SPECIFICATION.md` for current progress.

#### Enhanced Mock Capabilities
- Multiple device simulation
- Error injection for edge case testing
- Configurable latency simulation
- Recording/playback of real device sessions

## Key Takeaways

1. **Keep tests simple** - Test UI behavior, not implementation
2. **Trust the mock** - It provides consistent, predictable responses
3. **Use appropriate timeouts** - BLE operations take time
4. **Run server first** - `pnpm dev:mock` then `pnpm test:e2e:mock`
5. **Debug systematically** - Check server, logs, use UI mode

## Testing with simulateNotification

### Overview

The `simulateNotification` function is a testing utility provided by the ble-mcp-test mock that allows E2E tests to simulate BLE notifications from the CS108 hardware without requiring a physical device. This is essential for testing trigger-based functionality and other hardware events.

### How It Works

The ble-mcp-test mock replaces the Web Bluetooth API with a mock implementation that includes a `simulateNotification` method on the characteristic object. This method allows tests to inject data packets as if they were received from the actual CS108 device over BLE.

```javascript
// The mock characteristic includes this method:
characteristic.simulateNotification(data: Uint8Array)
```

### Usage in E2E Tests

There are three main approaches to access and use `simulateNotification` in our tests:

#### 1. Via Transport Manager's Notify Characteristic (Recommended)

```javascript
const tm = (window as any).__TRANSPORT_MANAGER__;
if (tm.notifyCharacteristic?.simulateNotification) {
  tm.notifyCharacteristic.simulateNotification(packet);
}
```

#### 2. Via Transport Manager's Event Emitter

```javascript
const tm = (window as any).__TRANSPORT_MANAGER__;
if (tm.emit) {
  tm.emit('notification', packet);
}
```

#### 3. Via Legacy BLE Mock (Deprecated)

```javascript
if ((window as any).__BLE_MOCK__?.simulateNotification) {
  (window as any).__BLE_MOCK__.simulateNotification(packet);
}
```

### Common Test Scenarios

#### Simulating Trigger Press/Release

The most common use case is simulating the CS108's physical trigger button:

```javascript
// Trigger PRESS packet
const TRIGGER_PRESS_PACKET = new Uint8Array([
  0xA7, 0xB3, 0x02, 0xD9, 0x82, 0x9E, 0xA7, 0x6F, 0xA1, 0x02
]);

// Trigger RELEASE packet  
const TRIGGER_RELEASE_PACKET = new Uint8Array([
  0xA7, 0xB3, 0x02, 0xD9, 0x82, 0x9E, 0xB6, 0xE6, 0xA1, 0x03
]);

// Simulate a trigger press
await page.evaluate((packet) => {
  const tm = (window as any).__TRANSPORT_MANAGER__;
  if (tm.notifyCharacteristic?.simulateNotification) {
    tm.notifyCharacteristic.simulateNotification(new Uint8Array(packet));
  }
}, Array.from(TRIGGER_PRESS_PACKET));
```

#### Test Helper Functions

We have helper functions in `tests/e2e/helpers/trigger-simulation.ts`:

```javascript
import { pressTrigger, releaseTrigger } from './helpers/trigger-simulation';

// In your test
await pressTrigger(page);
await page.waitForTimeout(200); // Let state propagate
await releaseTrigger(page);
```

### Understanding the Packet Format

CS108 packets follow a specific format:
- Bytes 0-1: Header (0xA7B3)
- Byte 2: Packet type
- Bytes 3-8: Device-specific data
- Bytes 9-10: Command/notification code

For trigger events:
- `0xA102`: Trigger pressed notification
- `0xA103`: Trigger released notification

### simulateNotification Debugging Tips

1. **Enable console logging** to see when notifications are sent:
   ```javascript
   page.on('console', msg => {
     if (msg.text().includes('[Trigger]')) {
       console.log(msg.text());
     }
   });
   ```

2. **Verify the mock is loaded**:
   ```javascript
   const hasMock = await page.evaluate(() => {
     return !!(window as any).__TRANSPORT_MANAGER__?.notifyCharacteristic?.simulateNotification;
   });
   expect(hasMock).toBe(true);
   ```

3. **Check state changes** after simulation:
   ```javascript
   await pressTrigger(page);
   const triggerState = await page.evaluate(() => {
     const store = (window as any).__ZUSTAND_STORES__?.deviceStore;
     return store?.getState()?.triggerState;
   });
   expect(triggerState).toBe(true);
   ```

### Important Notes

- The transport manager must be connected before using `simulateNotification`
- Notifications are processed asynchronously, so use appropriate waits
- The mock only works when `VITE_BLE_MOCK_ENABLED=true` or when using the `dev:mock` script
- Real hardware testing uses the ble-mcp-test bridge server, which tunnels to actual CS108 devices

### Related Files for simulateNotification

- `/tests/e2e/helpers/trigger-simulation.ts` - Helper functions for trigger testing
- `/tests/e2e/trigger-behavior.spec.ts` - Example trigger tests
- `/lib/rfid/cs108/transportManager.ts` - Where notifications are processed
- `/public/web-ble-mock.bundle.js` - The mock implementation (symlinked from node_modules)

## Related Documentation

- `/tests/e2e/README.md` - E2E test structure and patterns
- `/src/worker/docs/CS108-REFACTOR-SPECIFICATION.md` - Worker refactor in progress