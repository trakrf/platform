# CLAUDE.md

This file provides guidance to Claude when working with code in this repository.

## 🔄 Project Awareness & Context
- **Always read `PLANNING.md`** at the start of a new conversation to understand the project's architecture, goals, style, and constraints.
- **Use consistent naming conventions, file structure, and architecture patterns** as described in `PLANNING.md`.

## 📋 PRP (Product Requirements Prompt) Framework

For complex features or multi-step implementations, this project uses the PRP framework:

### When to Use PRPs
- **Complex features**: Multi-step implementations requiring research and planning
- **User requests PRP**: When explicitly asked to use the PRP process
- **Unclear requirements**: When feature needs discovery and specification

### PRP Workflow
1. **Read PRP documentation**: `prp/README.md` for complete framework details
2. **Create specification**: Write feature spec in `prp/spec/`
3. **Generate PRP**: Use `/generate-prp` command to research and create implementation plan
4. **Execute PRP**: Use `/execute-prp` command to implement with validation loops

### Key PRP Directories
- `prp/spec/` - Feature specifications (inputs)
- `prp/prompt/` - Generated PRPs (outputs)
- `prp/template/` - Templates for specs and PRPs
- `prp/COMPLETED.md` - Registry of completed PRPs
- **NEVER access `prp/archive/`** unless explicitly directed

### PRP Commands
- `/generate-prp prp/spec/feature.md` - Generate comprehensive PRP
- `/execute-prp prp/prompt/feature.md` - Execute implementation
- `/archive-prp prp/prompt/feature.md` - Archive completed work

## ⚠️ MANDATORY: Package Manager Rules

1. This project uses pnpm EXCLUSIVELY
2. Replace ALL instances of `npx` with `pnpm dlx`
3. Replace ALL instances of `npm` with `pnpm`
4. Any command suggesting `npx` or `npm` is INCORRECT and must be fixed
5. Examples:
  - ❌ WRONG: `npx prisma generate`
  - ✅ CORRECT: `pnpm dlx prisma generate`
  - ❌ WRONG: `npm test`
  - ✅ CORRECT: `pnpm test`

## 🚨 CRITICAL: Git Workflow Rules

**NEVER PUSH DIRECTLY TO MAIN BRANCH**
1. **ALL changes must go through a Pull Request** - no exceptions
2. **Never use `git push origin main`** - this is ALWAYS wrong
3. **Always create a feature/fix branch** for your work
4. **NEVER squash merge** - preserve individual commit history
5. **NEVER merge without explicit confirmation** - always ask before executing merge commands
6. **Proper workflow**:
   - Create branch: `git checkout -b feature/your-feature`
   - Commit changes: `git commit -m "your message"`
   - Push branch: `git push -u origin feature/your-feature`
   - Create PR: `gh pr create`
   - **Wait for user confirmation before any merge operations**
7. **Even for small changes** like documentation updates - use a PR
8. **Branch protection will be enabled** once GitHub org is upgraded

## Project Overview

This is a standalone React application for handheld RFID readers, extracted from a Next.js monolith. The project provides a clean implementation with CS108 protocol support, BLE communication, and comprehensive state management through Zustand stores.

## Module System Requirements

**🚨 CRITICAL: ES Modules ONLY - NO CommonJS**
- **Node.js 18+ or 20+** - Use modern Node.js versions with full ESM support
- **Pure ES Modules** - All packages use `"type": "module"`, no `require()` or `module.exports`
- **Use `import/export` exclusively** - Never mix module systems
- **Vite for bundling** - Modern ESM-first build tool

## 🧱 Code Structure & Modularity

- **Never create a file longer than 500 lines of code.** If a file approaches this limit, refactor by splitting it into modules or helper files.
- **Organize code into clearly separated modules**, grouped by feature or responsibility:
  - `components/` - React UI components
  - `stores/` - Zustand state management stores
  - `lib/rfid/` - RFID protocol implementations
  - `lib/ble/` - BLE transport layer
  - `hooks/` - Custom React hooks
  - `utils/` - Utility functions
  - `types/` - TypeScript type definitions
  - `tests/` - Integration and e2e tests
- **Use clear, consistent imports** (prefer relative imports within the same package, absolute imports for cross-package).

## Architecture

### Core Components

- **CS108 Protocol Implementation**: Complete command/response handling with metadata-driven architecture
- **BLE Transport Layer**: Web Bluetooth API abstraction with proper error handling
- **Zustand Stores**: State management for device connections, tag data, settings, and packets
- **React Components**: Clean, modular UI components extracted from monolith
- **Type Safety**: Comprehensive TypeScript types with shared enums and constants

### Essential Architecture Principles

1. **State Ownership Model**: Clear boundaries between transport states and device states
2. **Metadata-Driven Design**: Use CS108_COMMANDS and CS108_NOTIFICATIONS instead of hardcoded values
3. **Constants Over Magic Numbers**: Always import and use enums (e.g., `ReaderState.READY` not `4`)
4. **Hex Values Only**: ALL protocol values must be in hex format to match vendor specs

## 🧪 Testing & Reliability

### Test Organization
- **Unit tests use inline/colocated pattern** - Test files live next to the code they test
- **E2E tests go in `tests/e2e/`** - Integration and end-to-end tests using Playwright
- **NO centralized unit test folders** - Don't use `tests/unit/`, keep tests with their source files

### Unit Test Structure
- **Test files should be colocated** with the code they test using `.test.ts` or `.spec.ts` suffix:
  ```
  components/
    DeviceList.tsx
    DeviceList.test.tsx     ← Inline test file
  stores/
    deviceStore.ts
    deviceStore.test.ts     ← Inline test file
  utils/
    shareUtils.ts
    __tests__/              ← Alternative: __tests__ folder for multiple related tests
      exportUtils.test.ts
  ```
- **Always create Vitest unit tests for new features** (functions, classes, components, hooks)
- **After updating any logic**, check whether existing tests need updates. If so, do it
- **Use React Testing Library** for component tests
- **Mock external dependencies** but test the real implementation logic

### E2E Test Structure  
- **E2E tests stay in `tests/e2e/`** for clear separation:
  ```
  tests/
    e2e/
      inventory-page.spec.ts
      connection-flow.spec.ts
      helpers/              ← Shared test utilities
        assertions.ts
  ```
- **Use Playwright** for e2e tests
- Include at least:
  - 1 test for expected use
  - 1 edge case test
  - 1 failure case test

### Test Verification Protocol
1. **Unit tests**: `pnpm test`
2. **E2E tests**: `pnpm test:e2e` 
   - **🚨 CRITICAL: NO HEADED TESTS! NO X WINDOWS ON THIS SYSTEM!**
   - **🚨 NEVER use --headed flag with Playwright**
   - **🚨 ALWAYS run tests in headless mode**
   - **🚨 DO NOT add headed mode scripts to package.json**
3. **Type checking**: `pnpm typecheck`
4. **Linting**: `pnpm lint`
5. **Full validation**: `pnpm validate` (runs all checks)

### Test Standards
- **NO false confidence** - if tests fail, report it immediately
- **Always report EXACT numbers**: "X tests passing, Y tests failing"
- **Never claim "ready for testing"** until ALL tests pass
- **System is NOT ready** if ANY test fails

## Critical Implementation Rules

### 🚨 NEVER DO THESE:
1. **NO HARDCODED BYTE ARRAYS** - Always use packet builders and command constants
2. **NO HARDCODED CASE STATEMENTS** - Use metadata lookups: `Object.values(CS108_COMMANDS).find(cmd => cmd.responseCode === command)`
3. **NO DECIMAL VALUES IN PROTOCOL CODE** - Always hex: `0xA001` never `40961`
4. **NO MOCKING CORE LOGIC** - Only mock external dependencies (BLE, device)
5. **NO CommonJS** - Pure ES modules only

### ✅ ALWAYS DO THESE:
1. **Use constants from shared types** - Import enums and constants
2. **Hex format in all protocol work** - Logging, constants, comments
3. **Complete state contract testing** - Verify all Zustand store updates
4. **Clear error boundaries** - Transport vs device errors
5. **Proper WASM string handling** - Use `__getString()` for WASM returns

## Memory Rules

- We're building fresh - no need for backward compatibility
- Always use `pnpm`, never npm or yarn
- Node.js 18+ for modern ESM support
- Build outputs go to `./dist`, not scattered build folders
- Constants over magic numbers ALWAYS
- Hex values only in protocol code
- Test the real code, not mocks

## ✅ Task Completion
- Update `README.md` when features change or setup steps are modified

## 📎 Style & Conventions
- **TypeScript** with strict mode enabled
- **React 18+** with functional components and hooks
- **Tailwind CSS** for styling
- **Prettier** for formatting (configured in `.prettierrc`)
- **ESLint** with React/TypeScript rules
- **Conventional Commits** for git messages

### TypeScript Conventions
```typescript
// Use type imports
import type { ReaderState } from './types';

// Prefer interfaces for object shapes
interface DeviceConfig {
  name: string;
  timeout: number;
}

// Use enums for constants
enum OperationMode {
  IDLE = 0x00,
  INVENTORY = 0x01,
}

// Always type function parameters and returns
function processPacket(data: Uint8Array): ParsedPacket {
  // implementation
}
```

## 📚 Documentation & Explainability
- **Comment non-obvious code** with `// Reason:` explaining why
- **JSDoc for public APIs** and complex functions
- **Update README.md** for new features or changed dependencies
- **Keep ARCHITECTURE.md current** with design decisions
- **Document BLE quirks** and browser compatibility issues

## 🧠 AI Behavior Rules
- **Never assume missing context** - Ask questions if uncertain
- **Never hallucinate libraries** - Only use verified packages
- **Always confirm file paths exist** before referencing
- **Never delete code** unless explicitly instructed
- **Always run tests** before claiming completion
- **Report actual status** - no optimistic spin

## Project-Specific Context

### BLE Communication
- Web Bluetooth API has browser-specific quirks
- Always handle connection loss gracefully
- Implement exponential backoff for reconnection
- Clear characteristic subscriptions on disconnect

### CS108 Protocol
- Commands use 0xA7B3 prefix
- Responses use 0xB3A7 prefix
- Always parse complete packets before processing
- Handle fragmented BLE packets correctly

### State Management
- Zustand stores are the source of truth
- Never bypass stores for direct device access
- Subscribe to store changes in React components
- Use selectors to prevent unnecessary re-renders

### Performance
- Debounce rapid RFID tag reads
- Use React.memo for expensive components
- Virtualize long lists of tags
- Profile with React DevTools regularly

## Testing & Debugging

### E2E Test Log Capture
The ble-mcp-test server provides real-time log access via MCP for debugging E2E tests:

```bash
# Run tests with automatic log capture
pnpm test:e2e:with-logs

# Logs saved to: tests/e2e/logs/bridge-[timestamp].log
```

This captures all bridge server activity during tests, including:
- WebSocket connections and disconnections
- BLE device discovery and connection events
- Command/response traffic between test and hardware
- Error messages and timing information

Use log capture when:
- Debugging failing E2E tests
- Investigating connection issues
- Analyzing timing problems
- Verifying proper cleanup between tests

### Hardware Smoke Test - ALWAYS RUN THIS FIRST
**When debugging hardware issues, ALWAYS start here:**
```bash
pnpm test:hardware
# OR
pnpm test tests/integration/ble-mcp-test/connection.spec.ts
```

This test bypasses the entire worker and device stack, going straight to the bridge server with a test request/response cycle. **If this test passes**, you KNOW:
- ✅ The bridge server is running and accessible
- ✅ The CS108 hardware is connected and powered on
- ✅ The BLE connection is working
- ✅ Commands and responses are flowing correctly

**If this test fails, the problem is NOT in your code** - it's a hardware/bridge/connection issue.

### MCP BLE Monitoring Tools
The project includes MCP (Model Context Protocol) tools for real-time BLE monitoring during test development and debugging.

**Important**: The MCP tool is configured as `ble-mcp-test`:
- Use `mcp__ble-mcp-test__*` for all MCP tool invocations
- Check `.claude/settings.local.json` for available tool permissions

**Known Limitation - MCP Reconnection Required:**
If the bridge server restarts, you'll lose MCP connection. You must manually reconnect:
```
/mcp reconnect ble-mcp-test
```
This is a temporary limitation until the MCP service is separated from the bridge process.

**CRITICAL: ble-mcp-test connects to REAL HARDWARE**
- The ble-mcp-test bridge server is NOT a mock - it connects to actual CS108 hardware
- E2E tests tunnel through WebSocket to the bridge server which communicates with a physical CS108 device
- All packets logged are from real CS108 firmware, not simulated data
- Test tags (10018-10023) are physical RFID tags positioned in front of the reader
- See `docs/cs108-hardware-setup.jpg` for the physical test environment

**Best Practice**:
1. Always check for configured MCP tools first
2. Only run `pnpm dlx ble-mcp-test` inline if MCP tools aren't available
3. Avoid running duplicate server instances
4. Treat all packet data as real hardware behavior requiring proper implementation

```bash
# Common MCP BLE tools:
mcp__ble-mcp-test__status              # Check bridge server status
mcp__ble-mcp-test__get_connection_state # Get current BLE connection info
mcp__ble-mcp-test__scan_devices        # Scan for nearby BLE devices
mcp__ble-mcp-test__get_logs            # Retrieve recent BLE communication logs
mcp__ble-mcp-test__search_packets      # Search for hex patterns in packets
```

Use MCP BLE monitoring when:
- Developing new E2E tests with real CS108 hardware
- Debugging BLE connection or communication issues
- Analyzing packet flow between test and physical device
- Monitoring real-time test execution without log files
- Understanding actual CS108 firmware behavior

Example workflow:
1. Start your E2E test
2. Use `mcp__ble-mcp-test__get_connection_state` to verify connection
3. Use `mcp__ble-mcp-test__get_logs` to monitor real-time traffic
4. Use `mcp__ble-mcp-test__search_packets` to find specific commands/responses