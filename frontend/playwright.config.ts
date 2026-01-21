import { defineConfig, devices } from '@playwright/test';
import dotenv from 'dotenv';

// Load environment variables from .env.local
dotenv.config({ path: '.env.local' });

export default defineConfig({
  testDir: './tests/e2e',
  testIgnore: '**/to-fix/**',  // Skip all tests in to-fix directory
  // globalSetup: './tests/e2e/global-setup.ts',  // Disabled - was killing servers
  
  // IMPORTANT: 30 second timeout per test - fail fast instead of hanging!
  // If a test needs more time, it should be split into smaller tests
  timeout: 30 * 1000,
  
  // Assertions should also fail fast
  expect: {
    timeout: 5000
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1, // Force single worker to ensure sequential execution
  // TEMPORARY: Single worker + rate limiting in e2e.setup.ts to work around Noble.js listener leak
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: 'http://localhost:5173',
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
  webServer: process.env.CI ? {
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
  },
});