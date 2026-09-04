/**
 * Scan-tab enrichment after the assets come into existence (TRA-1191).
 *
 * ## The customer shape
 *
 * You scan tags. Someone else — or you, in another tab — then creates the asset
 * those tags belong to. Your Scan tab should show the asset. The reported
 * defect is that it never does.
 *
 * ## Why this spec exists alongside inventory-save.spec.ts
 *
 * The original report came from `inventory-save.spec.ts` test 2, which is
 * `@hardware`: it needs a CS108 in front of real tags to obtain the EPCs it then
 * creates assets for. That makes the failure expensive to reproduce and
 * impossible to run in CI, and the spec cannot currently observe its own answer
 * — its 10 s enrichment wait is wrapped in a `try/catch`, and the describe's
 * shared 90 s budget expires before the assertion is reached, so the run dies at
 * an unrelated `page.evaluate` with a message that names neither enrichment nor
 * a timeout.
 *
 * Enrichment needs none of that. It needs tags in localStorage and an asset in
 * the registry, both of which this spec makes for itself. **No hardware, no
 * radio, no bridge.**
 *
 * ## Why it watches the network rather than the console
 *
 * The question the ticket could not answer was whether the lookup fires at all.
 * A console-only answer is weak here: until this branch, every line on the
 * enrichment path except the failure line was dropped by the e2e console
 * forwarder, so "no output" meant nothing. The POST to `/lookup/tags` cannot be
 * filtered, and its request and response bodies say exactly what was asked and
 * what came back. That distinguishes the three candidate causes the ticket lists
 * — never queued, queued but never flushed, flushed but matched nothing — which
 * a pass/fail on the store alone does not.
 *
 * ## Why the poll runs well past 10 s
 *
 * The ticket is explicit that "whether enrichment eventually completes after
 * 10 s, or never" was never measured, because the only instrument was a fixed
 * 10 s wait. Sampling to 25 s and reporting the samples answers that question on
 * every run instead of re-asking it.
 */

import { test, expect, type Page } from '@playwright/test';
import { shouldForwardConsoleLine } from './helpers/console-forwarding';
import {
  uniqueId,
  signupTestUser,
  loginTestUser,
  getAuthToken,
  clearAuthState,
} from './fixtures/org.fixture';

/**
 * Where the API actually is.
 *
 * Follows `apiBase()` in assert-preconditions.ts rather than the copy in
 * inventory-save.spec.ts. That copy derives the API from PLAYWRIGHT_BASE_URL
 * alone, which silently assumes the API is same-origin with the page — true on
 * preview, false for any local run whose `VITE_API_URL` points at a separate
 * backend host, which is how this repo's dev env is actually configured. Asking
 * the app's own setting first means this spec talks to the same backend the
 * browser does; getting that wrong creates the asset in one place and looks for
 * it in another, which would present as exactly the enrichment failure under
 * investigation.
 */
function getApiBaseUrl(): string {
  const configured = process.env.VITE_API_URL;
  if (configured) return configured.replace(/\/+$/, '');
  const base = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
  return `${base.replace(/\/$/, '')}/api/v1`;
}

/**
 * EPCs chosen to match the shapes in the captured failing runs rather than to be
 * tidy: one heavily leading-zeroed, two vendor-prefixed. Leading zeros are load
 * bearing — the backend matches on `LTRIM(value, '0')` and this repo has a
 * history of leading-zero defects (TRA-1108) — so a repro made only of
 * zero-free EPCs would not cover the reported case.
 */
function makeEpcs(salt: string): string[] {
  const tail = salt.replace(/[^0-9a-f]/gi, '').slice(-4).padStart(4, '0').toUpperCase();
  return [
    `00000000000000000001${tail}`,
    `E2801190A5020060164${tail}`.slice(0, 24).padEnd(24, '0'),
    `E2006B06000000000000${tail}`,
  ];
}

/** How many tags the store currently holds. */
async function countTags(page: Page): Promise<number> {
  return page.evaluate(() => {
    const stores = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
    return (stores?.tagStore?.getState().tags ?? []).length;
  });
}

interface EnrichmentSample {
  atMs: number;
  enriched: number;
}

/**
 * Poll the store, recording what was true when. Returns every sample rather than
 * only the verdict, so a failure reports the shape of the failure — flat zero
 * throughout reads differently from a count that climbs and stalls.
 */
async function sampleEnrichment(
  page: Page,
  budgetMs: number,
  intervalMs = 500
): Promise<EnrichmentSample[]> {
  const started = Date.now();
  const samples: EnrichmentSample[] = [];

  for (;;) {
    const enriched = await page.evaluate(() => {
      const stores = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags ?? [];
      return tags.filter((t: any) => t.assetId !== undefined || t.locationId !== undefined).length;
    });

    const atMs = Date.now() - started;
    samples.push({ atMs, enriched });

    if (enriched > 0 || atMs >= budgetMs) return samples;
    await page.waitForTimeout(intervalMs);
  }
}

test.describe('Scan tab enrichment after matching assets exist (TRA-1191)', () => {
  /*
   * Its own budget, deliberately not HARDWARE_TEST_TIMEOUT_MS. That constant is
   * shared across the suite, and raising a shared budget to accommodate one
   * caller is the exact mistake this ticket's own comments warn about. This spec
   * touches no hardware; it needs room for a signup, a login, three asset
   * creations and a 25 s poll, and nothing else should inherit that.
   */
  test.describe.configure({ timeout: 90_000 });

  test('enriches tags scanned before the asset was created', async ({ page }) => {
    const testId = uniqueId();
    const email = `test-enrich-${testId}@example.com`;
    const password = 'TestPassword123!';
    const epcs = makeEpcs(testId);

    // Every POST to the lookup endpoint, with what was asked and what came back.
    // This is the evidence the ticket asks for: it separates "the lookup never
    // ran" from "the lookup ran and matched nothing".
    const lookups: Array<{ atMs: number; values: string[]; matched?: string[]; status?: number }> =
      [];
    const t0 = Date.now();

    // The enrichment path's own narration, through the same predicate the rest
    // of the suite uses. `ensureOrgContext()` is awaited BEFORE the POST, so a
    // throw there produces no request at all — indistinguishable at the network
    // layer from a lookup that was never triggered.
    const consoleLines: string[] = [];
    page.on('console', (msg) => {
      const text = msg.text();
      if (shouldForwardConsoleLine(text, msg.type())) {
        consoleLines.push(`+${Date.now() - t0}ms [${msg.type()}] ${text}`);
      }
    });
    // Stage markers interleaved into the same list. Without them the browser
    // timeline has to be aligned against the test's steps by guesswork, and
    // these steps are fast enough locally to be genuinely ambiguous.
    const stage = (name: string) => consoleLines.push(`+${Date.now() - t0}ms ===== ${name} =====`);

    page.on('response', async (res) => {
      if (!res.url().includes('/lookup/tags') || res.request().method() !== 'POST') return;
      const atMs = Date.now() - t0;
      let values: string[] = [];
      try {
        values = JSON.parse(res.request().postData() ?? '{}').values ?? [];
      } catch {
        /* a body we cannot parse is still worth recording as a call */
      }
      let matched: string[] | undefined;
      try {
        const body = await res.json();
        matched = Object.entries(body?.data ?? {})
          .filter(([, v]) => v !== null)
          .map(([k]) => k);
      } catch {
        /* non-JSON or already-consumed body */
      }
      lookups.push({ atMs, values, matched, status: res.status() });
    });

    // 1. A user with an org, and an empty registry.
    stage('signup');
    await signupTestUser(page, email, password, `Test Org ${testId}`);

    // 2. Back to anonymous. Signup leaves the browser logged in, but the
    //    reported sequence starts from an ANONYMOUS scan — that is the whole
    //    point of the workflow, and the login transition it depends on cannot
    //    happen from an already-authenticated page.
    stage('back to anonymous');
    await clearAuthState(page);
    await page.goto('/#scan');

    // 3. Tags in the store with no asset behind them, exactly as an anonymous
    //    scan leaves them. Written through the store so the persist middleware
    //    stores them the way the app itself would, rather than hand-rolling the
    //    localStorage payload and testing our own guess at its shape.
    await page.evaluate((epcList) => {
      const stores = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
      stores.tagStore.getState().setTags(
        epcList.map((epc: string) => ({
          epc,
          count: 1,
          rssi: -55,
          source: 'scan',
          type: 'unknown',
          timestamp: Date.now(),
        }))
      );
    }, epcs);

    stage('tags injected');
    const tagsWhileAnonymous = await countTags(page);
    expect(tagsWhileAnonymous, 'the anonymous scan must be in the store to begin with').toBe(
      epcs.length
    );

    // 4. Log in. This is where the auth-store subscription fires its lookup —
    //    and at this moment no matching asset exists, so it correctly finds
    //    nothing. Reproducing that ordering is the point.
    stage('login');
    await loginTestUser(page, email, password);
    stage('logged in');

    // Checkpoint, and the measurement that distinguishes the candidate causes
    // the ticket lists. If the tags are gone HERE, the question was never
    // "why did the lookup not enrich them" — there was nothing left to enrich,
    // and no lookup, retry or timeout could have made a difference.
    const tagsAfterLogin = await countTags(page);
    console.log(`[TRA-1191] tags before login=${tagsWhileAnonymous} after login=${tagsAfterLogin}`);

    // 5. NOW the assets come into existence, out of band, as they would if a
    //    colleague created them.
    stage('creating assets');
    const token = await getAuthToken(page);
    for (let i = 0; i < epcs.length; i++) {
      const response = await page.request.post(`${getApiBaseUrl()}/assets`, {
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        data: {
          name: `Enrich Asset ${i + 1} - ${testId}`,
          external_key: `ASSET-${uniqueId()}`,
          is_active: true,
          tags: [{ tag_type: 'rfid', value: epcs[i] }],
        },
      });
      expect(
        response.ok(),
        `asset creation must succeed or the rest of this test measures nothing: ` +
          `${response.status()} ${await response.text()}`
      ).toBe(true);
    }

    // 6. Come back to the Scan tab, the way a user would after being away.
    //
    //    `page.reload()`, NOT `page.goto('/#scan')`. The login fixture already
    //    leaves the browser on `/#scan`, so navigating there again differs only
    //    in the fragment — a same-document navigation that reloads nothing,
    //    runs no module again and re-mounts nothing. inventory-save.spec.ts
    //    does exactly that and its comment calls it a reload; measured here, no
    //    reload occurs and no enrichment can be triggered by it. A spec that
    //    reloads only when the URL happens to differ is testing the fixture's
    //    last redirect, not the app.
    stage('reload #scan');
    await page.reload();
    stage('polling');


    // 7. Measure.
    const samples = await sampleEnrichment(page, 25_000);

    // What the reloaded page actually believes. Printed rather than asserted:
    // when enrichment does not happen, the useful question is which of these
    // preconditions was not met, and a bare count cannot say.
    const postReload = await page.evaluate(() => {
      const s = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
      const auth = s?.authStore?.getState();
      const tags = s?.tagStore?.getState().tags ?? [];
      return {
        storesExposed: !!s,
        isAuthenticated: auth?.isAuthenticated,
        hasToken: !!auth?.token,
        hasProfile: !!auth?.profile,
        tagCount: tags.length,
        unenriched: tags.filter(
          (t: any) => t.assetId === undefined && t.locationId === undefined
        ).length,
      };
    });
    console.log('[TRA-1191] post-reload state:', JSON.stringify(postReload));
    const finalTags = await page.evaluate(() => {
      const stores = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
      return (stores?.tagStore?.getState().tags ?? []).map((t: any) => ({
        epc: t.epc,
        type: t.type,
        assetId: t.assetId,
        assetName: t.assetName,
      }));
    });

    const enrichedCount = finalTags.filter((t: any) => t.assetId !== undefined).length;

    console.log('[TRA-1191] enrichment-path console:\n' + consoleLines.join('\n'));
    console.log('[TRA-1191] lookup POSTs:', JSON.stringify(lookups, null, 2));
    console.log('[TRA-1191] enrichment samples:', JSON.stringify(samples));
    console.log('[TRA-1191] final tags:', JSON.stringify(finalTags, null, 2));

    // Logging in must not destroy the scan. Asserted before the enrichment
    // outcome and separately from it, because it is a different defect with a
    // different fix: enrichment cannot be repaired by any amount of retrying if
    // the tags are cleared out from under it.
    expect(
      tagsAfterLogin,
      'logging in cleared the anonymous scan — the tags the login is supposed to enrich'
    ).toBe(epcs.length);

    // The tags must also survive the reload. If this fails the rest is moot,
    // and the cause is persistence, not enrichment (cf. TRA-527).
    expect(finalTags.length, 'tags did not survive the reload').toBe(epcs.length);

    // A lookup must have been issued AFTER the assets existed. Asserted
    // separately from the outcome so a failure says which half broke: no such
    // call means nothing re-triggers the lookup, whereas a call that matched
    // nothing means the trigger is fine and the query is wrong.
    const lookupsAfterReload = lookups.filter((l) => l.values.some((v) => epcs.includes(v)));
    expect(
      lookupsAfterReload.length,
      'no lookup was issued for these EPCs at any point — nothing triggers enrichment'
    ).toBeGreaterThan(0);

    expect(
      enrichedCount,
      `expected all ${epcs.length} tags enriched; lookups=${JSON.stringify(lookups)}`
    ).toBe(epcs.length);
  });

  test('logging out keeps the bare scan and drops only the asset data', async ({ page }) => {
    /*
     * `tagStore`'s logout subscription has always been written to strip
     * enrichment and KEEP the scan:
     *
     *   // Clear enrichment data when user logs out (true -> false transition)
     *   useTagStore.getState().clearEnrichment();
     *
     * It had never once had an effect. `invalidateAllOrgScopedData` ran on the
     * same transition and cleared the store outright, so the later write won —
     * the same two-features-cancelling-out shape as the login defect, on the
     * sibling transition.
     *
     * Needs no enrichment and no hardware to state: seed tags that already
     * carry asset data, log out, and check what survives.
     */
    const testId = uniqueId();
    const email = `test-logout-${testId}@example.com`;
    const password = 'TestPassword123!';

    /*
     * The synchronisation here is load-bearing, and getting it wrong made this
     * test pass against the unfixed code.
     *
     * Logout does two things to the tag store. The auth subscription fires
     * SYNCHRONOUSLY and strips enrichment; `invalidateAllOrgScopedData` runs
     * LATER, from a floating promise behind two dynamic imports, and is what
     * would clear the store outright. Polling for "asset data is gone" is
     * satisfied by the first of those and returns before the second has landed,
     * so the tags are still present no matter which behaviour is configured.
     *
     * So gate on the invalidation having actually run — it names the method it
     * called — and only then ask what survived.
     */
    const orgCacheLines: string[] = [];
    page.on('console', (msg) => {
      const text = msg.text();
      if (text.includes('[OrgCache] tags:')) orgCacheLines.push(text);
    });

    await signupTestUser(page, email, password, `Test Org ${testId}`);

    await page.evaluate(() => {
      const stores = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
      stores.tagStore.getState().setTags([
        {
          epc: 'LOGOUT00000000000000AAAA',
          count: 2,
          rssi: -61,
          source: 'rfid',
          type: 'asset',
          timestamp: Date.now(),
          assetId: 4242,
          assetName: 'Should Not Survive',
          assetIdentifier: 'ASSET-4242',
        },
      ]);
    });

    // Signup itself invalidates, so ignore anything logged before this point.
    const linesBeforeLogout = orgCacheLines.length;

    await page.evaluate(async () => {
      const stores = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
      await stores.authStore.getState().logout();
    });

    // Wait for the org-scoped invalidation triggered BY THE LOGOUT to run.
    await expect
      .poll(() => orgCacheLines.length, {
        message: 'logout never ran the org-scoped invalidation at all',
        timeout: 5000,
      })
      .toBeGreaterThan(linesBeforeLogout);

    const invalidationLine = orgCacheLines[orgCacheLines.length - 1];
    console.log('[TRA-1191] logout invalidation:', invalidationLine);

    // Named explicitly: this is the line that decides the outcome, and asserting
    // it means a failure says which method ran rather than only that tags
    // vanished.
    expect(
      invalidationLine,
      'logout must strip the resolution, not clear the store'
    ).toContain('clearEnrichment()');

    const afterLogout = await page.evaluate(() => {
      const stores = (window as unknown as { __ZUSTAND_STORES__?: any }).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags ?? [];
      return {
        isAuthenticated: stores?.authStore?.getState().isAuthenticated,
        count: tags.length,
        first: tags[0],
      };
    });
    console.log('[TRA-1191] after logout:', JSON.stringify(afterLogout));

    expect(afterLogout.isAuthenticated, 'the user must actually be logged out').toBe(false);

    // The observation survives, intact.
    expect(afterLogout.count, 'logging out destroyed the scan').toBe(1);
    expect(afterLogout.first.epc).toBe('LOGOUT00000000000000AAAA');
    expect(afterLogout.first.count).toBe(2);
    expect(afterLogout.first.rssi).toBe(-61);

    // Everything the org resolved it to does not. A logged-out user holds bare
    // EPCs and no asset data whatsoever.
    expect(afterLogout.first.type).toBe('unknown');
    expect(afterLogout.first.assetId).toBeUndefined();
    expect(afterLogout.first.assetName).toBeUndefined();
    expect(afterLogout.first.assetIdentifier).toBeUndefined();
  });
});
