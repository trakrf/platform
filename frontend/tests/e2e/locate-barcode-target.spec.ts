/**
 * Locate barcode target acquisition (TRA-1121) — requires a CS108 via the bridge.
 *
 * The Locate tab is dual-mode by target: with no target the operator is still
 * acquiring one, so the reader parks in BARCODE and the physical trigger scans
 * a label; once a target is set it parks in LOCATE and the trigger searches.
 * Nothing binds the trigger to either action — the worker already starts and
 * stops whatever the current mode is — so the whole feature is the mode flip,
 * and the mode flip is what this file checks.
 *
 * Two bugs live here, both found on a reader and neither reachable from a unit
 * test:
 *
 *  - Clearing the field printed "Target cleared" but left the reader in LOCATE,
 *    still hunting the EPC that had just been deleted. The input committed on
 *    blur, and nobody blurs before reaching for a trigger.
 *  - The mode change was then lost a second way: it was issued from a separate
 *    settingsStore subscriber, concurrently with the settings push, and the
 *    second command into the non-re-entrant CommandManager lost the mutex with
 *    "Command already active" — silently, because nothing caught it.
 *
 * So the console assertions here are not decoration. A mode that fails to apply
 * looks exactly like a mode that was never requested.
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { getReaderMode } from './helpers/device-state';

const TARGET = '000000000000000000010021';
// Whichever rfidCollect label is physically in front of the scanner. The point
// of the assertion is that the scanned text becomes the target verbatim, not
// which label it was — pinning one turns a bench-aim change into a red test.
const BENCH_BARCODES = ['10020', '10021', '10022', '10023'];

let page: Page;
const consoleLines: string[] = [];

/** Navigate to Locate and wait for the input the screen is built around. */
async function gotoLocate(p: Page) {
  await p.goto('/#locate');
  await p.waitForSelector('[data-testid="target-epc-display"]', { timeout: 15000 });
}

/** The stored target, which is what decides the mode — not the field text. */
async function storedTarget(p: Page): Promise<string> {
  return p.evaluate(() =>
    (window as any).__ZUSTAND_STORES__?.settingsStore?.getState()?.rfid?.targetEPC ?? ''
  );
}

async function statusText(p: Page): Promise<string> {
  return (await p.locator('[data-testid="target-epc-display"]')
    .locator('xpath=../..')
    .textContent()) ?? '';
}

/**
 * Mode changes are worker round-trips (~1s on real hardware), so poll rather
 * than sampling once.
 */
async function waitForMode(p: Page, expected: string, timeoutMs = 12000): Promise<string> {
  const start = Date.now();
  let mode = await getReaderMode(p);
  while (mode !== expected && Date.now() - start < timeoutMs) {
    await p.waitForTimeout(250);
    mode = await getReaderMode(p);
  }
  return mode ?? 'null';
}

/** Set the field the way a person does, then commit it. */
async function typeTarget(p: Page, value: string) {
  const input = p.locator('[data-testid="target-epc-display"]');
  await input.fill(value);
  await input.blur();
}

test.describe.configure({ mode: 'serial' });

test.describe('Locate barcode target acquisition (TRA-1121) @hardware', () => {
  test.beforeAll(async ({ browser }) => {
    page = await browser.newPage();
    page.on('console', (msg) => consoleLines.push(`[${msg.type()}] ${msg.text()}`));
    await page.goto('/');
    await connectToDevice(page);
    await gotoLocate(page);
  });

  test.afterAll(async () => {
    await disconnectDevice(page).catch(() => { /* best effort */ });
    await page.close();
  });

  test('parks in LOCATE while a target is set', async () => {
    await typeTarget(page, TARGET);

    expect(await storedTarget(page)).toBe(TARGET);
    expect(await waitForMode(page, 'Locate')).toBe('Locate');
  });

  test('flips to BARCODE the moment the field is cleared, without a blur', async () => {
    const input = page.locator('[data-testid="target-epc-display"]');
    // Deliberately no blur: clearing and reaching straight for the trigger is
    // the gesture that used to leave the reader hunting the old EPC.
    await input.fill('');

    expect(await storedTarget(page)).toBe('');
    expect(await statusText(page)).toContain('Target cleared');
    expect(await waitForMode(page, 'Barcode')).toBe('Barcode');
  });

  test('flips back to LOCATE when a target is entered again', async () => {
    await typeTarget(page, TARGET);

    expect(await waitForMode(page, 'Locate')).toBe('Locate');
  });

  test('did not lose a command to the mutex during either flip', async () => {
    const collisions = consoleLines.filter((l) => /Command already active/.test(l));
    expect(collisions, `console:\n${collisions.join('\n')}`).toHaveLength(0);
  });

  test('refuses a non-hex value instead of targeting it', async () => {
    await typeTarget(page, TARGET);
    await typeTarget(page, 'S04163');

    // The scanned/typed text stays visible so it can be corrected, but it must
    // not become the target: masking on it hunts the wrong bits and reports
    // "no signal", which on a tag finder reads as "the item is not here".
    await expect(page.locator('[data-testid="target-epc-display"]')).toHaveValue('S04163');
    expect(await storedTarget(page)).toBe(TARGET);
    expect(await statusText(page)).toContain('Invalid EPC format');
  });

  test('captures a barcode into the target from the scan button', async () => {
    await typeTarget(page, '');
    await waitForMode(page, 'Barcode');

    await page.locator('[data-testid="locate-barcode-scan"]').click();

    // The read lands in barcodeStore and the screen takes it from there.
    await expect
      .poll(async () => storedTarget(page), { timeout: 20000, intervals: [500] })
      .not.toBe('');

    const captured = await storedTarget(page);
    expect(BENCH_BARCODES, `captured ${captured}`).toContain(captured);
    // Used verbatim: no registry lookup, no rewriting into a padded EPC.
    await expect(page.locator('[data-testid="target-epc-display"]')).toHaveValue(captured);
    // And acquiring a target hands the trigger back to searching.
    expect(await waitForMode(page, 'Locate')).toBe('Locate');
  });
});
