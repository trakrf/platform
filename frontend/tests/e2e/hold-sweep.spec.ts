/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Trigger-hold saturation sweep — how many unique tags does a single hold of
 * duration D acquire? Characterizes the accumulation curve of the tag field.
 * Scratch instrument for TRA-1150 density work; not part of the suite.
 */
import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { simulateTriggerPress, simulateTriggerRelease } from './helpers/trigger-utils';
import type { WindowWithStores } from './types';

const DURATIONS_MS = [1000, 2000, 3000, 4000, 5000, 7000, 10000, 15000, 20000];
const REPS = Number(process.env.SWEEP_REPS || 2);

test.describe('Trigger hold saturation sweep', () => {
  test.describe.configure({ timeout: 15 * 60 * 1000 });
  let page: Page;

  test.beforeAll(async ({ browser }) => {
    page = await browser.newPage();
    await page.goto('/');
    await connectToDevice(page);
    await page.click('[data-testid="menu-item-scan"]');
    await page.waitForTimeout(500);
    await page.waitForFunction(
      () => window.__ZUSTAND_STORES__?.deviceStore?.getState().readerMode === 'Inventory',
      { timeout: 10000 }
    );
    await page.waitForFunction(
      () => window.__ZUSTAND_STORES__?.deviceStore?.getState().readerState === 'Connected',
      { timeout: 10000 }
    );
    await page.waitForTimeout(3000);
  });

  /**
   * Hand the reader back. This file had NO teardown at all.
   *
   * It holds the command path for the whole sweep — minutes — and then simply
   * ended, leaving the bridge's single-writer ownership with a session nobody
   * was using. The next spec file's `beforeAll` connect then waited out its full
   * 30s timeout and reported "the reader would not connect", which is a true
   * statement about the wrong subject: the reader was fine, this file never let
   * go of it.
   *
   * That is why `inventory-save.spec.ts` — which runs immediately after this one
   * — failed in `connectToDevice` while its own assertions were never reached.
   */
  test.afterAll(async () => {
    if (!page || page.isClosed()) return;
    // Best-effort, but not silent — the same empty catch in locate.spec.ts hid
    // #647's release gate for 101 consecutive reps. TRA-1245.
    await simulateTriggerRelease(page).catch((error) => {
      console.warn('[SWEEP] teardown release did not complete:', error);
    });
    await disconnectDevice(page).catch(() => { /* best effort */ });
    await page.close();
  });

  test('sweep hold durations @hardware', async () => {
    const rows: Array<{ ms: number; rep: number; uniq: number; reads: number }> = [];

    for (let rep = 1; rep <= REPS; rep++) {
      for (const ms of DURATIONS_MS) {
        // Fresh field each measurement — single hold, no accumulation.
        await page.evaluate(() => {
          (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore?.getState().clearTags();
        });
        await page.waitForTimeout(400);

        await simulateTriggerPress(page);
        await page.waitForTimeout(ms);
        await simulateTriggerRelease(page);
        await page.waitForTimeout(800); // let trailing packets land

        const r = await page.evaluate(() => {
          const tags = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore?.getState().tags || [];
          return {
            uniq: tags.length,
            reads: tags.reduce((s: number, t: any) => s + (t.count || 1), 0)
          };
        });
        rows.push({ ms, rep, uniq: r.uniq, reads: r.reads });
        console.log(`[SWEEP] hold=${ms}ms rep=${rep} unique=${r.uniq} reads=${r.reads}`);
        await page.waitForTimeout(1200); // settle between cycles (setMode ~1s / BUSY)
      }
    }
    console.log('[SWEEP-JSON] ' + JSON.stringify(rows));
    expect(rows.length).toBe(DURATIONS_MS.length * REPS);
  });
});
