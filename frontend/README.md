# TrakRF Handheld

React application for CS108 RFID handheld readers using Web Bluetooth technology.

**Note**: This is the `@trakrf/frontend` package within the TrakRF platform monorepo. For monorepo-wide commands, see the root README.md.

## 🚨 Hardware Troubleshooting - START HERE

**Before assuming hardware issues, ALWAYS run this test first:**

```bash
pnpm test:hardware
```

This test bypasses ALL application code and directly verifies:
- ✅ Bridge server is running and accessible
- ✅ CS108 hardware is connected and powered on  
- ✅ BLE connection is working
- ✅ Commands and responses are flowing

**If this test passes, the hardware is fine.** The issue is in your code, not the hardware.

To see the actual hardware communication:
```bash
# 1. Run the hardware test
pnpm test:hardware

# 2. Check the bridge logs (requires MCP tools)
mcp__ble-mcp-test__get_logs --since=30s
```

You'll see the actual command/response exchange:
```
RX: A7 B3 02 D9 82 37 00 00 A0 01     ← Command sent to CS108
TX: A7 B3 03 D9 82 9E 74 37 A0 01 00  ← CS108 responded
```

**Remember: If `pnpm test:hardware` passes, stop blaming the hardware.**

## Features

- **Device Connection**: Connect to CS108 RFID readers via Bluetooth
- **Tag Inventory**: Read and display RFID tags in real-time
- **Tag Search/Locate**: Find specific tags with signal strength indicator
- **Barcode Scanning**: Scan barcodes using the reader's built-in scanner
- **Settings Management**: Configure reader power, session, and other parameters
- **Export**: Export tag data to CSV format

> **Note**: USB HID support was investigated but found to provide read-only access on CS108 hardware. 
> See the [`feature/TRA-16-usb-transport`](https://github.com/trakrf/trakrf-handheld/tree/feature/TRA-16-usb-transport) branch for the experimental implementation.
> We are not pursuing USB support at this time as it does not meet our requirements for device control.

## Requirements

- Modern browser with Web Bluetooth API support (Chrome, Edge, Opera)
- HTTPS connection (required for Web Bluetooth)
- CS108 RFID reader

## Installation

**From frontend/ directory:**
```bash
# Install dependencies using pnpm
pnpm install

# (Optional) Set up trusted HTTPS certificates for local development
# This avoids browser security warnings
./setup-https.sh

# Start development server
pnpm dev          # Standard development
pnpm dev:https    # With HTTPS for real device testing
pnpm dev:bridge   # With BLE bridge server (no HTTPS needed, see ../docs/frontend/BRIDGE_USAGE_GUIDE.md)

# Build for production
pnpm build
```

**From project root (using Just task runner):**
```bash
# Install all dependencies
pnpm install

# Validation commands
just frontend           # Lint + typecheck + test + build
just frontend-lint      # ESLint
just frontend-typecheck # TypeScript
just frontend-test      # Vitest unit tests
just frontend-build     # Vite production build
```

## Development

```bash
# Run type checking
pnpm typecheck

# Run linting
pnpm lint

# Run tests
pnpm test              # Run unit tests once
pnpm test:watch        # Unit tests in watch mode
pnpm test:integration  # Integration tests with real CS108
pnpm test:e2e          # Run all E2E tests
pnpm test:ui           # Interactive Playwright UI

# Run validation (typecheck + lint + tests)
pnpm validate
```

## Testing

The project uses a comprehensive tag-based testing strategy for selective execution:

### Test Categories

- **@ui-only**: UI tests with BLE mocks (no hardware required)
- **@hardware**: Integration tests requiring CS108 device  
- **@smoke**: Critical path validation (< 2 minutes)
- **@critical**: Production-blocking functionality

### Quick Commands

```bash
pnpm test              # Run unit tests once
pnpm test:watch        # Watch mode for development
pnpm test:integration  # TDD with real CS108 hardware
pnpm test:e2e          # Full E2E test suite
```

### Unit Tests (Vitest)
- **Inline/colocated pattern**: Test files live next to the code they test
- **Naming**: Use `.test.ts` or `.test.tsx` suffix
- **Structure**:
  ```
  components/
    DeviceList.tsx
    DeviceList.test.tsx     ← Unit test colocated with component
  stores/
    deviceStore.ts
    deviceStore.test.ts     ← Unit test next to store
  ```
- Run with: `pnpm test:unit`

### E2E Tests (Playwright)
- Located in `tests/e2e/` with tag-based organization
- Automatic mock vs hardware mode detection
- Selective execution based on test requirements
- Physical test tags: 10018-10023, test barcode: 10021

### Development Workflows

```bash
# During development
pnpm dev                    # Development server
pnpm test:watch            # Auto-run tests on changes

# TDD for worker code
pnpm test:integration      # Test against real CS108

# Before committing
pnpm validate              # Full validation suite
```

## Technology Stack

- React 18
- TypeScript
- Vite
- Tailwind CSS
- Zustand (state management)
- Web Bluetooth API

## Architecture

The application follows a modular architecture:

- **Components**: UI components for different screens and functionality
- **Stores**: Zustand stores for state management
- **RFID Library**: CS108 protocol implementation with BLE transport
- **Types**: TypeScript type definitions

See `../docs/frontend/ARCHITECTURE.md` for detailed architecture documentation.

## Documentation Structure

Frontend documentation is organized in the monorepo at `docs/frontend/` (relative to project root):

- **`docs/frontend/ARCHITECTURE.md`**: System architecture and design patterns
- **`docs/frontend/cs108/`**: CS108 protocol-specific documentation
  - `README.md`: Protocol overview and quick reference
  - `CS108_and_CS463_*.md/.pdf`: Complete vendor specifications (6,708 lines)
  - `inventory-parsing.md`: Tag parsing quick reference
  - `CS108-PROTOCOL-QUIRKS.md`: Protocol quirks and gotchas
- **`docs/frontend/MOCK_USAGE_GUIDE.md`**: BLE mock development setup
- **`docs/frontend/TROUBLESHOOTING.md`**: Operational troubleshooting guide
- **`docs/frontend/README.md`**: Complete documentation index

For CS108 protocol details, start with `docs/frontend/cs108/README.md` from the project root.

## Browser Support

Web Bluetooth API is currently supported in:
- Chrome/Chromium (desktop & Android)
- Edge (desktop)
- Opera (desktop & Android)

Not supported in Firefox or Safari.

## Development Notes

- HTTPS is required for Web Bluetooth API (Vite dev server configured for HTTPS)
- All protocol values use hex format to match vendor specifications
- State management follows Zustand patterns with persistence for settings
- BLE packet fragmentation is handled automatically

## License

Licensed under the Business Source License 1.1. See [LICENSE](./LICENSE) for full terms.

**Summary:** Source-available for non-production use. Automatically converts to MIT License four years after public distribution.