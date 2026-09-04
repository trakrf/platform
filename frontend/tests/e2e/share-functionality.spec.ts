/**
 * E2E tests for share/export functionality
 * Tests the share dropdown and export modal without requiring a device.
 *
 * All four of the tests that click Share used to fail with a 30s timeout
 * reported as "locator.click: Target page, context or browser has been closed".
 * The cause was not the click: `beforeEach` seeded tags behind `if (tagStore)`,
 * and `window.__ZUSTAND_STORES__` is assigned from an async import in main.tsx,
 * so straight after `goto('/')` it was still undefined and the seed silently did
 * nothing. With no tags `hasItems` is false, the control renders `disabled` —
 * visible, so the old `isVisible()` guard passed — and the click then waited for
 * an element that would never become enabled (TRA-1246).
 *
 * Two things follow, and both are load-bearing here:
 *
 * 1. Seeding goes through `helpers/dev-stores`, which waits for the store and
 *    reads the count back rather than guarding and moving on.
 * 2. The `if (buttonExists) { ... } else { console.log(...) }` shape is gone.
 *    A test that logs "feature may not be implemented" and passes cannot fail,
 *    so it never told anyone the button had been unclickable for months. If the
 *    control is missing now, that is the finding, and the test says so.
 *
 * The assertions also describe the UI that exists. Share is a Headless UI menu,
 * not a modal: clicking it opens a dropdown of formats, and picking one opens
 * ShareModal. The modal is a plain div with no `role="dialog"`, so it is
 * addressed by its heading — `Export {format label}` — rather than by role.
 */

import { test, expect, type Page } from '@playwright/test';
import { seedTags, clearSeededTags, type SeedTag } from './helpers/dev-stores';

const TAG_COUNT = 5;

const mockTags = (): SeedTag[] =>
  Array.from({ length: TAG_COUNT }, (_, i) => {
    const n = i + 1;
    return {
      epc: `E28068940000000000000${n.toString(16).padStart(3, '0').toUpperCase()}`,
      displayEpc: `TEST-TAG-${n}`,
      rssi: -40 - n * 5,
      count: n * 10,
      timestamp: Date.now() - n * 1000,
      reconciled: n % 2 === 0,
      description: `Test Item ${n}`,
      location: `Shelf ${n}`,
      source: 'scan' as const,
    };
  });

/**
 * The labelled Share control from the desktop header. InventoryHeader also
 * renders a compact `share-button-compact` inside a `md:hidden` block; at the
 * project's 1280x720 viewport that one is hidden, and asserting visibility here
 * keeps a viewport change from silently selecting the wrong element.
 */
function shareButton(page: Page) {
  return page.getByTestId('share-button');
}

/** Open the format dropdown. Fails loudly if the control is absent or disabled. */
async function openFormatMenu(page: Page): Promise<void> {
  const button = shareButton(page);
  await expect(button).toBeVisible();
  await expect(button).toBeEnabled();
  await button.click();
}

/** The Export modal's heading for a given format, e.g. "Export CSV File". */
function modalHeading(page: Page, formatLabel: string) {
  return page.getByRole('heading', { name: `Export ${formatLabel}` });
}

test.describe('Share Functionality', () => {
  let sharedPage: Page;

  test.beforeAll(async ({ browser }) => {
    sharedPage = await browser.newPage();
  });

  test.afterAll(async () => {
    if (sharedPage) {
      await sharedPage.close();
    }
  });

  test.beforeEach(async () => {
    await sharedPage.goto('/');
    await seedTags(sharedPage, mockTags());
    await sharedPage.waitForTimeout(500);
  });

  test('Share button opens modal with format selection', async () => {
    await openFormatMenu(sharedPage);

    // The dropdown lists one entry per export format.
    for (const format of ['PDF', 'Excel', 'CSV']) {
      await expect(
        sharedPage.getByRole('menuitem').filter({ hasText: format })
      ).toBeVisible();
    }

    // Picking one is what actually opens the modal.
    await sharedPage.getByRole('menuitem').filter({ hasText: 'CSV' }).click();
    await expect(modalHeading(sharedPage, 'CSV File')).toBeVisible();
    await expect(sharedPage.getByText(`${TAG_COUNT} items ready`)).toBeVisible();
  });

  test('Can select different export formats', async () => {
    // Each format opens the modal titled for that format. The old version
    // clicked CSV and then looked for the PDF entry, by which point the menu had
    // closed and the modal was open — it could only ever have found nothing.
    const formats: Array<[menuText: string, heading: string]> = [
      ['CSV', 'CSV File'],
      ['PDF', 'PDF Report'],
      ['Excel', 'Excel Spreadsheet'],
    ];

    for (const [menuText, heading] of formats) {
      await openFormatMenu(sharedPage);
      await sharedPage.getByRole('menuitem').filter({ hasText: menuText }).click();

      await expect(modalHeading(sharedPage, heading)).toBeVisible();
      console.log(`[Test] ${menuText} opens "Export ${heading}"`);

      await sharedPage.getByRole('button', { name: 'Close' }).click();
      await expect(modalHeading(sharedPage, heading)).toBeHidden();
    }
  });

  test('Modal can be closed', async () => {
    await openFormatMenu(sharedPage);
    await sharedPage.getByRole('menuitem').filter({ hasText: 'CSV' }).click();

    const heading = modalHeading(sharedPage, 'CSV File');
    await expect(heading).toBeVisible();

    // The X in the modal header carries aria-label="Close"; the backdrop behind
    // it is aria-label="Close modal", which is a different element.
    await sharedPage.getByRole('button', { name: 'Close' }).click();
    await expect(heading).toBeHidden();

    // Cancel closes it too, and is the path a keyboard user is most likely to take.
    await openFormatMenu(sharedPage);
    await sharedPage.getByRole('menuitem').filter({ hasText: 'CSV' }).click();
    await expect(heading).toBeVisible();
    await sharedPage.getByRole('button', { name: 'Cancel' }).click();
    await expect(heading).toBeHidden();
  });

  test('Export button triggers action', async () => {
    await openFormatMenu(sharedPage);
    await sharedPage.getByRole('menuitem').filter({ hasText: 'CSV' }).click();
    await expect(modalHeading(sharedPage, 'CSV File')).toBeVisible();

    // Download, not Share: the Web Share API is unavailable in headless
    // Chromium, so ShareModal renders that button disabled and the download
    // path is the one a run here can actually exercise.
    const downloadPromise = sharedPage.waitForEvent('download');
    await sharedPage.getByRole('button', { name: 'Download' }).click();

    const download = await downloadPromise;
    expect(download.suggestedFilename()).toMatch(/\.csv$/i);
    console.log(`[Test] Download triggered: ${download.suggestedFilename()}`);

    // A completed export closes the modal (ShareModal only calls onClose when
    // the action actually completed).
    await expect(modalHeading(sharedPage, 'CSV File')).toBeHidden();
  });

  test('Shows appropriate message when no data to export', async () => {
    await clearSeededTags(sharedPage);
    await sharedPage.waitForTimeout(500);

    // With nothing to export the control is disabled rather than opening an
    // empty modal. This is the assertion that was already real in this file —
    // and the reason the four tests above sat on a disabled button for 30s each
    // without anyone reading this one as the explanation.
    await expect(shareButton(sharedPage)).toBeDisabled();
  });

  test('Export includes selected tags when selection is available', async () => {
    // This UI has no per-row selection: the inventory table renders no
    // checkboxes, so "export the selected tags" has no control to drive. What
    // the export does promise is a count, and the modal states it — so that is
    // what is asserted, against the seeded number rather than a hardcoded one.
    const checkboxes = sharedPage.locator('input[type="checkbox"]');
    expect(
      await checkboxes.count(),
      'the inventory table has grown row selection — this test should now drive it'
    ).toBe(0);

    await openFormatMenu(sharedPage);
    await sharedPage.getByRole('menuitem').filter({ hasText: 'CSV' }).click();

    await expect(sharedPage.getByText(`${TAG_COUNT} items ready`)).toBeVisible();
  });
});
