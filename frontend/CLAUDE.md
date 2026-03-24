# Frontend CLAUDE.md

## Overview
Standalone React app for handheld RFID readers with CS108 protocol support, BLE communication, and Zustand state management.

## Module System
- **ES Modules only** — no CommonJS, no `require()`
- Vite for bundling, Vitest for unit tests, Playwright for E2E

## Key Directories
- `components/` — React UI components
- `stores/` — Zustand state management
- `lib/rfid/` — RFID protocol implementations
- `lib/ble/` — BLE transport layer
- `hooks/` — Custom React hooks
- `types/` — TypeScript type definitions

## Testing
```bash
pnpm test              # Unit tests (Vitest)
pnpm test:e2e          # E2E tests (Playwright, headless only — NO headed mode)
pnpm typecheck         # Type checking
pnpm lint              # Linting
pnpm validate          # All checks
pnpm test:hardware     # Hardware smoke test (bypasses app stack)
```

## Hardware Testing
- `pnpm test:hardware` — **always run first** when debugging hardware issues
- If it passes: hardware/bridge/BLE are fine, problem is in your code
- If it fails: fix hardware/bridge before debugging code
- MCP tools available as `mcp__ble-mcp-test__*` for real-time BLE monitoring

## Style
- TypeScript strict mode, React 18+, Tailwind CSS
- Prettier + ESLint configured in project
- Colocate unit tests with source files (`.test.ts` / `.spec.ts`)
- E2E tests in `tests/e2e/`
