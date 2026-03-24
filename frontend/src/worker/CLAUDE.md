# Worker Component CLAUDE.md

Worker thread implementation: DeviceManager lifecycle, CS108 protocol, BLE transport, worker-main thread communication via Comlink.

## Scope
- DeviceManager, CS108Worker, BLE transport, state synchronization
- **Not in scope**: UI components, Zustand stores, E2E tests

## Critical Rules
- **One worker during connection** — zero at startup, zero after disconnect, fail-fast on duplicates
- **Hex values only** in protocol code — `0xA001` never `40961`
- **Constants over magic numbers** — use `CS108_COMMANDS` metadata, not hardcoded bytes
- **Pure message passing** — no direct store coupling in worker; structured events only

## Testing
```bash
pnpm test src/worker/                                          # Unit tests
pnpm test src/worker/ --config vitest.integration.config.ts    # Integration (mock hardware)
pnpm test:hardware                                             # Real CS108 device
```

## Debugging
1. **Always start with** `pnpm test:hardware` — proves hardware is working
2. Use `mcp__ble-mcp-test__get_logs` to monitor real-time packet flow
3. Use `mcp__ble-mcp-test__search_packets` for specific command patterns
