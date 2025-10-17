# Test Commands Reference

## Core Commands

We keep scripts minimal and focused. All commands default to **run-and-done** behavior.

### Development Commands (3)

```bash
pnpm dev              # Standard development server
pnpm dev:https        # HTTPS for real device testing  
pnpm dev:mock         # Mock BLE for development without hardware
```

### Test Commands (5)

```bash
# Unit Tests
pnpm test              # Run unit tests once (default)
pnpm test:watch        # Watch mode - re-run on changes

# Integration Tests  
pnpm test:integration  # TDD with real CS108 hardware

# E2E Tests
pnpm test:e2e          # Run all E2E tests once
pnpm test:ui           # Interactive Playwright UI
```

### Build & Validation Commands (6)

```bash
pnpm build            # Production build
pnpm start            # Serve production build (for Railway)
pnpm preview          # Preview production build locally
pnpm validate         # Run typecheck + lint + unit tests
pnpm lint             # ESLint checking
pnpm typecheck        # TypeScript checking
```

## Test Types

| Type | Command | Hardware | Purpose |
|------|---------|----------|---------|
| **Unit** | `pnpm test` | No | Fast component/function tests (~30s) |
| **Integration** | `pnpm test:integration` | Yes | Worker TDD with real CS108 (~2s/test) |
| **E2E** | `pnpm test:e2e` | Optional | Full UI testing with Playwright |
| **Validation** | `pnpm validate` | No | Pre-commit/CI checks |

## E2E Test Tags

While we don't have separate commands for these, E2E tests are tagged for selective execution:

```bash
# Run specific test categories via Playwright
pnpm test:e2e --grep @ui-only     # No hardware needed
pnpm test:e2e --grep @hardware    # Requires CS108
pnpm test:e2e --grep @smoke       # Quick validation
pnpm test:e2e --grep @critical    # Production paths
```

## Philosophy

- **Minimal scripts**: 14 essential commands instead of 20+
- **Clear purpose**: Each command has one job
- **Run-and-done default**: CI/CD friendly
- **Explicit watch mode**: Use `:watch` when needed

## Common Workflows

```bash
# During development
pnpm test:watch        # Unit tests auto-run on changes

# Before committing
pnpm validate          # Full validation suite

# TDD for worker code
pnpm test:integration  # Test against real hardware

# Debug E2E tests
pnpm test:ui          # Interactive UI mode
```

## Notes

- Never use `--headed` with Playwright (no X Windows on this system)
- Integration tests require ble-mcp-test bridge server running
- E2E tests can run with or without real hardware (uses mocks if unavailable)