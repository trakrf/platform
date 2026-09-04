/**
 * Seeding tags through the app's DEV-only store handles, without the race.
 *
 * `window.__ZUSTAND_STORES__` is not set synchronously at page load. main.tsx
 * assigns it from inside `import('./stores').then(...)` — a dynamic import, so
 * it lands one or more microtask turns after `page.goto()` has already
 * resolved. Every caller here used to read it straight after a `goto` and guard
 * the result with `if (tagStore)` or `tagStore?.`, which meant the seed
 * silently did nothing whenever the import had not finished yet.
 *
 * That failure is invisible at the seed and expensive at the assertion. In
 * share-functionality.spec.ts it surfaced four spec-files away from its cause:
 * no tags meant `hasItems` was false, which renders the Share control
 * `disabled` — still *visible*, so the spec's `isVisible()` guard passed — and
 * the subsequent `click()` sat waiting for an element that would never become
 * enabled until the 30s test timeout killed it. The reported error was
 * "locator.click: Target page, context or browser has been closed", which names
 * neither the store nor the seed (TRA-1246).
 *
 * So the rule these helpers encode: wait for the handle, and if it never
 * arrives, say so. Never proceed on the assumption that a missing store meant
 * there was nothing to do.
 */

import type { Page } from '@playwright/test';
import type { WindowWithStores } from '../types';

/** A tag as the store's `addTag` accepts it. */
export interface SeedTag {
  epc: string;
  displayEpc: string;
  rssi: number;
  count: number;
  timestamp: number;
  reconciled: boolean | null;
  description: string;
  location: string;
  source: 'scan';
}

/**
 * `__ZUSTAND_STORES__` is exposed only under `import.meta.env.DEV` or a
 * non-prod environment label (frontend/src/main.tsx). Against a production
 * build it is absent permanently rather than late, and no amount of waiting
 * will produce it — which is a different situation from the race above and
 * deserves a different message.
 */
const DEFAULT_TIMEOUT_MS = 10_000;

/**
 * Block until the DEV store handles exist on `window`.
 *
 * Throws — rather than returning a boolean the caller can drop on the floor —
 * because every current caller needs the stores to do anything meaningful, and
 * a silent skip here is what this module exists to stop.
 */
export async function waitForDevStores(
  page: Page,
  timeout: number = DEFAULT_TIMEOUT_MS
): Promise<void> {
  try {
    await page.waitForFunction(
      () => Boolean((window as WindowWithStores).__ZUSTAND_STORES__?.tagStore),
      undefined,
      { timeout }
    );
  } catch {
    throw new Error(
      `window.__ZUSTAND_STORES__.tagStore did not appear within ${timeout}ms.\n` +
        'It is assigned from an async import in src/main.tsx and only when\n' +
        "import.meta.env.DEV is set or the environment label is non-prod, so\n" +
        'either the app has not finished booting or this run is pointed at a\n' +
        'production build, which cannot be seeded this way.'
    );
  }
}

/**
 * Replace the tag list with `tags`, then confirm the store actually holds them.
 *
 * The read-back is the point. Injecting into a store that exists but has been
 * re-created, or whose `addTag` has changed shape, would otherwise leave the
 * page empty with the seed reporting success.
 */
export async function seedTags(page: Page, tags: SeedTag[]): Promise<void> {
  await waitForDevStores(page);

  const stored = await page.evaluate((seed) => {
    const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__!.tagStore;
    const state = tagStore.getState() as unknown as {
      clearTags: () => void;
      addTag: (tag: unknown) => void;
      tags: unknown[];
    };
    state.clearTags();
    seed.forEach((tag) => state.addTag(tag));
    return (tagStore.getState() as unknown as { tags: unknown[] }).tags.length;
  }, tags);

  if (stored < tags.length) {
    throw new Error(
      `seeded ${tags.length} tags but the store reports ${stored}. ` +
        'The injection ran against a live store and still did not take.'
    );
  }
}

/** Empty the tag list, waiting for the store first. */
export async function clearSeededTags(page: Page): Promise<void> {
  await waitForDevStores(page);
  await page.evaluate(() => {
    const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__!.tagStore;
    (tagStore.getState() as unknown as { clearTags: () => void }).clearTags();
  });
}
