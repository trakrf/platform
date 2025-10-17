# Troubleshooting Guide

## 🚨 THE FIRST RULE OF DEBUGGING

**ALWAYS run the hardware test first:**

```bash
pnpm test:hardware
```

This test takes **1 second** and will save you hours of debugging.

## Understanding the Hardware Test

The hardware test (`tests/integration/ble-mcp-test/connection.spec.ts`) is special because it:
- **Bypasses ALL application code** - no worker, no DeviceManager, no stores
- **Goes straight to the bridge server** - direct WebSocket connection
- **Sends a real CS108 command** - `0xA7B3...` (actual protocol bytes)
- **Receives a real response** - from the actual CS108 hardware

### What "Test Passed" Means

When you see:
```
✅ Bridge server connection test passed!
```

This definitively proves:
1. **Bridge server is running** ✅
2. **Network path is clear** ✅
3. **CS108 is connected via BLE** ✅
4. **CS108 is powered on** ✅
5. **CS108 is responding to commands** ✅

**Translation: Hardware is fine. The problem is in your code.**

### What "Test Failed" Means

If the test fails, check these IN ORDER:
1. Is the bridge server running? (`node-ble-host` or similar)
2. Is the CS108 powered on?
3. Is the CS108 paired/connected via Bluetooth?
4. Is the network path clear? (firewall, VPN, etc.)
5. Are the bridge server logs showing errors?

**DO NOT debug your application code if this test fails.**

## The Complete Debugging Flow

### Step 1: Verify Hardware
```bash
pnpm test:hardware
```

### Step 2: See What Happened
```bash
# Check bridge logs to see the actual communication
mcp__ble-mcp-test__get_logs --since=30s
```

You should see:
```
RX: A7 B3 02 D9 82 37 00 00 A0 01     ← Command sent to CS108
TX: A7 B3 03 D9 82 9E 74 37 A0 01 00  ← CS108 responded
```

### Step 3: Debug Your Code (ONLY if Step 1 passed)

Now you can confidently debug knowing:
- Hardware ✅ Working
- Bridge ✅ Working  
- Connection ✅ Working
- Problem → In your implementation

## Common Misconceptions

### ❌ "Maybe the hardware isn't working"
**Reality:** Run `pnpm test:hardware`. If it passes, hardware IS working.

### ❌ "The CS108 isn't responding"
**Reality:** Run `pnpm test:hardware`. If it passes, CS108 IS responding.

### ❌ "The bridge might be down"
**Reality:** Run `pnpm test:hardware`. If it passes, bridge IS up.

### ❌ "It could be a BLE connection issue"
**Reality:** Run `pnpm test:hardware`. If it passes, BLE IS connected.

## Integration Test Failures

If `pnpm test:hardware` passes but integration tests fail:

1. **Commands not reaching hardware**
   - Check worker → transport wiring
   - Verify MessagePort connections
   - Check command building logic

2. **Responses not reaching worker**
   - Check transport → worker message flow
   - Verify packet parsing logic
   - Check event routing

3. **State management issues**
   - Verify CommandManager state
   - Check Reader state transitions
   - Validate mode sequences

## The Golden Rules

1. **ALWAYS run `pnpm test:hardware` first**
2. **If it passes, hardware is fine**
3. **If it fails, fix hardware/bridge first**
4. **Never guess - verify with logs**
5. **Trust the test - it talks to real hardware**

## Known Limitation: MCP Connection After Bridge Restart

**If the bridge server restarts, the MCP connection to Claude Code is lost.**

You'll see errors like:
```
Error: MCP server ble-mcp-test is not connected
```

### The Workaround

Manually reconnect using the Claude Code command:
```
/mcp reconnect ble-mcp-test
```

Then resume using MCP tools normally:
```bash
mcp__ble-mcp-test__get_logs --since=30s
```

### When This Happens
- After bridge server restart
- After bridge server crash
- After changing bridge configuration
- After network interruption

### Future Enhancement
This will be fixed by separating the MCP service from the bridge process, allowing MCP to stay connected even when the bridge restarts. Until then, use the manual reconnect workaround.

## Quick Reference

```bash
# The only command you need to remember
pnpm test:hardware

# If you want proof of what happened
mcp__ble-mcp-test__get_logs --since=30s

# If MCP connection is lost (after bridge restart)
/mcp reconnect ble-mcp-test

# That's it. Hardware verified in 1 second.
```

Remember: This test has prevented countless hours of "maybe it's the hardware" wild goose chases. Use it religiously.