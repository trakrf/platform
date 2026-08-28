import { defineConfig, devices } from '@playwright/test';
import dotenv from 'dotenv';

// Load environment variables from .env.local
dotenv.config({ path: '.env.local' });

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:5173';
const isRemote = !!process.env.PLAYWRIGHT_BASE_URL;

export default defineConfig({
  testDir: './tests/e2e',

  // IMPORTANT: 30 second timeout per test - fail fast instead of hanging!
  // If a test needs more time, it should be split into smaller tests
  timeout: 30 * 1000,

  // Assertions should also fail fast
  expect: {
    timeout: 5000
  },
  fullyParallel: false,

  /*
   * ONE WORKER, and the reason is the reader, not a library.
   *
   * This used to read "TEMPORARY: Single worker + rate limiting in
   * e2e.setup.ts to work around Noble.js listener leak". Every noun in that
   * sentence is gone: `noble` is not a dependency of this package, and
   * `tests/e2e/e2e.setup.ts` does not exist. It survived the Noble era as
   * scaffolding and read as a live constraint for as long as nobody checked.
   *
   * The setting is still right, for a reason that outlives any library: 11 of
   * these specs are tagged `@hardware` and reach ONE physical CS108 through one
   * bridge. Two workers would fight over the radio, and the loser's failure
   * would look like a device fault. A serial run is the price of a shared
   * reader.
   *
   * That also means the constraint is narrower than it looks: the non-hardware
   * specs have nothing to contend over. Splitting them into their own project
   * with real parallelism is available whenever the suite's wall-clock starts to
   * hurt — it is not blocked on anything, it has simply never been done.
   */
  workers: 1,

  /*
   * The `process.env.CI` branches below have never run.
   *
   * Verified 2026-08-28: no workflow under `.github/workflows/` references
   * playwright or `test:e2e`. Playwright e2e is deliberately not in CI — it
   * needs hardware — so green CI says nothing about this suite, and these
   * branches are kept for whenever that changes rather than because they are
   * exercised. Do not read their presence as coverage.
   */
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    ignoreHTTPSErrors: true,
    // CRITICAL: Always run headless - NO X WINDOWS ON THIS SYSTEM!
    headless: true
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // When PLAYWRIGHT_BASE_URL is set, tests run against a remote deployment
  // and no local webServer is needed.
  webServer: isRemote ? undefined : (process.env.CI ? {
    // In CI, always start fresh server
    command: process.env.USE_BRIDGE ? 'pnpm dev:bridge' : 'pnpm vite',
    port: 5173,
    timeout: 30 * 1000,
    reuseExistingServer: false,
    stdout: 'pipe',
    stderr: 'pipe',
  } : {
    // In dev, expect server to be running
    command: 'echo "\n⚠️  No dev server running on port 5173!\n\nPlease start the appropriate server first:\n  - For bridge testing: pnpm dev:bridge\n  - For real device: pnpm dev\n" && exit 1',
    port: 5173,
    timeout: 5 * 1000,  // Fail fast with helpful message
    reuseExistingServer: true,  // Always reuse if available
    stdout: 'pipe',
    stderr: 'pipe',
  }),
});