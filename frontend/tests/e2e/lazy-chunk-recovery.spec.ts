/**
 * TRA-1054 AC2/AC3 — stale-chunk recovery against a real deployment.
 *
 * Vite content-hashes chunk filenames, so a deploy replaces `dist/assets/` and
 * every chunk name a still-open tab holds starts returning 404. This spec
 * reproduces that at the network layer — the chunk request is answered with a
 * real 404 — and asserts the tab recovers with exactly one automatic reload
 * instead of surfacing `TypeError: Failed to fetch dynamically imported module`
 * to the ErrorBoundary.
 *
 * Covers all three call sites the ticket names:
 *   - SortableHeader        (Scan tab, inventory table header)
 *   - PaginationControls    (Scan tab, inventory table footer)
 *   - react-gauge-component (Locate tab, signal strength gauge)
 *
 * Run against preview:
 *   PLAYWRIGHT_BASE_URL=$API_TEST_URL pnpm test:e2e tests/e2e/lazy-chunk-recovery.spec.ts
 *
 * Requires API_TEST_LOGIN / API_TEST_PASS for a user on the target deployment.
 */

import { test, expect, type Page } from '@playwright/test';

const EMAIL = process.env.API_TEST_LOGIN;
const PASS = process.env.API_TEST_PASS;

// The Scan tab renders its table only once there are rows. "Reconcile" is a
// client-side CSV upload, so it populates the table without a reader or any
// server-side fixture data.
const RECONCILE_CSV = [
  'Asset ID,Tag ID,Description,Location',
  ...Array.from({ length: 12 }, (_, i) => `ASSET-${String(i + 1).padStart(4, '0')},${1000 + i},Item ${i + 1},Main Warehouse`),
].join('\n');

/*
 * The credentials belong to a DEPLOYMENT, and this spec has to say so.
 *
 * `API_TEST_LOGIN` / `API_TEST_PASS` are for the user named by `API_TEST_URL`
 * (preview). The old guard checked only that the two variables were SET, which
 * they are in any direnv shell — so running the default local target ran these
 * four tests against `localhost:5173`, where that user does not exist. Login
 * then never completed and all four failed on `expect(input#password)
 * .toHaveCount(0)` timing out after 20s.
 *
 * That reads as four broken tests. It is one unmet precondition, and it cost
 * TRA-1253's inventory a whole spec's worth of misattribution: the ticket
 * carried these as untriaged failures needing a rot/defect verdict when the
 * answer was "wrong target". A guard that confirms the variables exist rather
 * than that they APPLY is the same self-confirming shape ADR 0018 is about.
 *
 * Compared against `API_TEST_URL` rather than hardcoding a hostname, so this
 * keeps working when preview moves.
 */
const TARGET = (process.env.PLAYWRIGHT_BASE_URL ?? '').replace(/\/+$/, '');
const CREDENTIALED_TARGET = (process.env.API_TEST_URL ?? '').replace(/\/+$/, '');
const TARGET_MATCHES_CREDENTIALS = !!TARGET && !!CREDENTIALED_TARGET && TARGET === CREDENTIALED_TARGET;

test.describe('stale chunk recovery (TRA-1054)', () => {
  test.skip(!EMAIL || !PASS, 'requires API_TEST_LOGIN / API_TEST_PASS');
  test.skip(
    !TARGET_MATCHES_CREDENTIALS,
    `these credentials are for ${CREDENTIALED_TARGET || 'API_TEST_URL (unset)'}, but this run ` +
      `targets ${TARGET || 'the default localhost'}. Run with ` +
      'PLAYWRIGHT_BASE_URL=$API_TEST_URL, which is what the header above documents.'
  );

  const login = async (page: Page) => {
    await page.goto('/#login');
    await page.locator('input#email').fill(EMAIL!);
    await page.locator('input#password').fill(PASS!);
    await page.locator('button[type="submit"]').click();
    await expect(page.locator('input#password')).toHaveCount(0, { timeout: 20000 });
  };

  const loadReconcileCsv = async (page: Page) => {
    await page.locator('input[type="file"]').first().setInputFiles({
      name: 'reconcile.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from(RECONCILE_CSV),
    });
  };

  /**
   * Answer the first `failures` requests for `pattern` with a 404, exactly as a
   * rolled-over deployment does, then let the chunk through.
   */
  const stale = async (page: Page, pattern: RegExp, failures: number) => {
    const seen: string[] = [];
    await page.route(pattern, async (route) => {
      if (seen.length < failures) {
        seen.push(route.request().url());
        await route.fulfill({ status: 404, contentType: 'text/plain', body: 'Not Found' });
        return;
      }
      await route.continue();
    });
    return seen;
  };

  const boundary = (page: Page) => page.locator('text=/Error in |Failed to fetch dynamically imported module/');

  test('Locate recovers from a stale gauge chunk with one reload', async ({ page }) => {
    await login(page);

    const failed = await stale(page, /\/assets\/gauge-.*\.js/, 1);

    // Note: only full document loads fire this. Navigating between hash routes
    // does not, so this counts reloads rather than route changes.
    let reloads = 0;
    page.on('load', () => { reloads++; });

    await page.goto('/#locate');

    // lazyWithRetry reloads once; the retried request is served normally and the
    // gauge mounts. Without the fix this throws straight to the ErrorBoundary.
    await expect(page.locator('[data-testid="proximity-display"] svg')).toBeVisible({ timeout: 25000 });

    expect(failed, 'the gauge chunk was actually 404ed').toHaveLength(1);
    expect(reloads, 'recovered via a reload, and only one').toBe(1);
    await expect(boundary(page)).toHaveCount(0);
  });

  test('Scan recovers from a stale SortableHeader chunk with one reload', async ({ page }) => {
    await login(page);
    await page.goto('/#scan');

    const failed = await stale(page, /\/assets\/SortableHeader-.*\.js/, 1);

    let reloads = 0;
    page.on('load', () => { reloads++; });

    await loadReconcileCsv(page);

    // The reload drops the CSV-loaded rows, so re-apply it to prove the retried
    // chunk now mounts rather than merely that the page survived.
    await page.waitForLoadState('networkidle');
    if ((await page.locator('input[type="file"]').count()) > 0) {
      await loadReconcileCsv(page);
    }

    await expect(page.getByText('Item ID', { exact: true })).toBeVisible({ timeout: 25000 });

    expect(failed, 'the SortableHeader chunk was actually 404ed').toHaveLength(1);
    expect(reloads, 'exactly one automatic reload').toBe(1);
    await expect(boundary(page)).toHaveCount(0);
  });

  test('Scan recovers from a stale PaginationControls chunk with one reload', async ({ page }) => {
    await login(page);
    await page.goto('/#scan');

    const failed = await stale(page, /\/assets\/PaginationControls-.*\.js/, 1);

    let reloads = 0;
    page.on('load', () => { reloads++; });

    await loadReconcileCsv(page);
    await page.waitForLoadState('networkidle');
    if ((await page.locator('input[type="file"]').count()) > 0) {
      await loadReconcileCsv(page);
    }

    await expect(page.getByText(/Rows per page|of \d+/).first()).toBeVisible({ timeout: 25000 });

    expect(failed, 'the PaginationControls chunk was actually 404ed').toHaveLength(1);
    expect(reloads, 'exactly one automatic reload').toBe(1);
    await expect(boundary(page)).toHaveCount(0);
  });

  test('a genuinely missing chunk stops at the ErrorBoundary without looping', async ({ page }) => {
    await login(page);

    // Never serve the chunk — a broken deploy, not a stale tab.
    const failed = await stale(page, /\/assets\/gauge-.*\.js/, Number.MAX_SAFE_INTEGER);

    let reloads = 0;
    page.on('load', () => { reloads++; });

    await page.goto('/#locate');

    await expect(boundary(page).first()).toBeVisible({ timeout: 25000 });

    // The loop this guards against ran at ~9 reloads/second, so a quiet window
    // here is the assertion that matters, not the exact count.
    await page.waitForTimeout(10000);
    expect(reloads, 'stops after the single retry reload').toBeLessThanOrEqual(2);
    expect(failed.length, 'chunk requested once per attempt, not continuously').toBeLessThanOrEqual(3);
  });
});
