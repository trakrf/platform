import { describe, it, expect } from 'vitest';
import { shouldForwardConsoleLine } from '../e2e/helpers/console-forwarding';
import { E2E_SIGNALS, E2E_BROWSER_NEEDLES } from '../../scripts/suite-run-signals.mjs';

const S = E2E_SIGNALS as Record<string, string>;

/**
 * The e2e console forwarder must pass everything the soak instruments count.
 *
 * ## The defect this closes (TRA-1209)
 *
 * `cs108-ble-transport.ts` emits three `[ble-timing]` lines with `console.info`.
 * Under e2e that code runs INSIDE THE BROWSER, so the only way those lines reach
 * a captured run log is the `page.on('console')` forwarder in
 * `helpers/connection.ts`. Its filter was:
 *
 *     text.includes('BLE') || text.includes('Connect') || text.includes('WebSocket')
 *       || text.includes('force') || text.includes('cleanup') || text.includes('disconnect')
 *
 * Every limb case-sensitive, against a lowercase `[ble-timing] connect`. Nothing
 * matched. `ack-latency-report.mjs` counts exactly those three lines, so it had
 * nothing to say about any e2e soak — and its own docstring calls a run with zero
 * of them a measurement of nothing rather than a clean link.
 *
 * ## Why this is a loop and not three string assertions
 *
 * Asserting the three `[ble-timing]` needles would fix today's bug and leave the
 * class open: the next browser-side needle added to `E2E_SIGNALS` would be
 * dropped by the forwarder in exactly the same silence, and would report a
 * confident 0.
 *
 * So the coupling is declared — `E2E_BROWSER_NEEDLES` names which needles have a
 * browser-side producer — and every one of them is checked against the
 * forwarder. Adding a browser-emitted needle without teaching the forwarder is
 * now a failing test rather than a signal that quietly reads zero.
 *
 * ## Why testing the bare needle is sufficient
 *
 * The predicate is a disjunction of `includes` calls, so it is monotone: if it
 * passes the needle on its own, it passes every line that contains the needle.
 * A representative full line would be a weaker test, not a stronger one, because
 * it could pass on a substring the needle does not contain.
 */

describe('every browser-emitted needle survives the forwarder', () => {
  it('declares at least one, so an empty list cannot pass vacuously', () => {
    expect(E2E_BROWSER_NEEDLES.length).toBeGreaterThan(0);
  });

  it('names only needles that are actually in E2E_SIGNALS', () => {
    // A declaration naming a needle that no longer exists is dead weight that
    // reads as coverage.
    const orphans = E2E_BROWSER_NEEDLES.filter((name: string) => !(name in E2E_SIGNALS));
    expect(orphans, `declared browser-emitted but not in E2E_SIGNALS: ${orphans.join(', ')}`).toEqual(
      []
    );
  });

  for (const name of E2E_BROWSER_NEEDLES as string[]) {
    it(`forwards \`${name}\``, () => {
      const needle = (E2E_SIGNALS as Record<string, string>)[name];
      expect(
        shouldForwardConsoleLine(needle, 'info'),
        `The forwarder drops ${JSON.stringify(needle)}, so this needle counts 0 on every e2e ` +
          `rep no matter what the reader did. Add a limb to shouldForwardConsoleLine.`
      ).toBe(true);
    });
  }
});

describe('the three lines this ticket was filed for', () => {
  // Named explicitly as well as covered by the loop above, so the regression has
  // a test that says what it was rather than only a generic one.
  const REAL_LINES = [
    '[ble-timing] connect t=1788015619705 ms=1832 outcome=ok',
    '[ble-timing] link-close t=1788015645102 inflight=0 queued=0',
    '[ble-timing] write-ack t=1788015620887 ms=41 attempt=1/3 outcome=ok',
  ];

  for (const line of REAL_LINES) {
    it(`forwards ${JSON.stringify(line.slice(0, 24))}…`, () => {
      expect(shouldForwardConsoleLine(line, 'info')).toBe(true);
    });
  }

  it('would have failed before the fix — none of the old limbs match', () => {
    // The old predicate, verbatim. Kept so the test states what was broken
    // rather than only that it now works.
    const OLD = (text: string, type: string) =>
      type === 'error' ||
      text.includes('Error') ||
      text.includes('Failed') ||
      text.includes('BLE') ||
      text.includes('Connect') ||
      text.includes('WebSocket') ||
      text.includes('force') ||
      text.includes('cleanup') ||
      text.includes('disconnect');

    for (const line of REAL_LINES) {
      expect(OLD(line, 'info'), `the old filter passed ${line} — premise of the ticket is wrong`).toBe(
        false
      );
    }
  });
});

describe('the forwarder is still a filter, not a firehose', () => {
  /**
   * Widening this predicate is a measurement change: every extra line lands in
   * the captured log that every signal count is computed from. The fix matches
   * the `[ble-timing]` prefix explicitly rather than lowercasing the existing
   * limbs, because a case-insensitive `connect` would newly forward a large
   * volume of unrelated browser chatter.
   */
  it('does not forward ordinary browser chatter', () => {
    for (const line of [
      'Download the React DevTools for a better development experience',
      '[vite] connected.',
      'Fetching /api/v1/assets',
      '%c GET /health 200',
    ]) {
      expect(shouldForwardConsoleLine(line, 'log'), `should not forward: ${line}`).toBe(false);
    }
  });

  it('still forwards everything it forwarded before', () => {
    // The pre-existing limbs are load-bearing for other needles and for
    // ordinary debugging; widening must not narrow.
    //
    // Needle-derived rather than re-typed, for two reasons. It is the same
    // alias-don't-retype rule the signal tables follow — two spellings of one
    // signal is how a count silently means different things. And typing
    // `WebSocket error` or `ECONNREFUSED` verbatim here makes this file look
    // like a PRODUCER of those needles: the reverse-drift check in
    // every-signal-needle-has-a-producer.test.ts scans `tests/`, and it caught
    // exactly that when this test first went in.
    expect(shouldForwardConsoleLine(`${S.startScanFailed} timeout`, 'log')).toBe(true);
    expect(shouldForwardConsoleLine(`${S.stopScanFailed} timeout`, 'log')).toBe(true);
    expect(shouldForwardConsoleLine(S.transportUnreachable, 'log')).toBe(true);
    expect(shouldForwardConsoleLine(`connect ${S.transportRefused} 127.0.0.1:25153`, 'error')).toBe(
      true
    );
    expect(shouldForwardConsoleLine('anything at all', 'error')).toBe(true);
    expect(shouldForwardConsoleLine('BLE transport ready', 'log')).toBe(true);
    expect(shouldForwardConsoleLine('forcing cleanup', 'log')).toBe(true);
    expect(shouldForwardConsoleLine('device disconnect requested', 'log')).toBe(true);
  });
});

describe('the tag-enrichment path is observable (TRA-1191)', () => {
  /**
   * `src/stores/tagStore.ts` narrates the enrichment path with `console.log` and
   * `console.warn`, and `src/lib/auth/orgContext.ts` narrates the org-context
   * step every lookup awaits. Under e2e that code runs INSIDE THE BROWSER, so
   * those lines reach a captured run log only through this predicate.
   *
   * None of them matched any existing limb: `Auth subscription: login detected`
   * contains no `Error`/`Failed`/`BLE`/`Connect`/`WebSocket`/`force`/`cleanup`/
   * `disconnect`, and `clearing enrichment` is not `cleanup`. Only
   * `_flushLookupQueue: API error` survived, because it is a `console.error` and
   * type `error` is kept unconditionally.
   *
   * That asymmetry is the trap TRA-1191 hit. The single forwarded line is the
   * FAILURE line, so a run where the lookup ran and matched nothing was
   * indistinguishable from a run where the lookup never happened at all — both
   * print exactly nothing. Telling those two apart is the whole ticket, so the
   * predicate has to pass the success-path narration too.
   */
  const ENRICHMENT_LINES: Array<[string, string]> = [
    ['[TagStore] Auth subscription: login detected', 'log'],
    ['[TagStore] Auth subscription: logout detected, clearing enrichment', 'log'],
    ['[tagStore] Stale enrichment detected - central invalidation may have been bypassed', 'warning'],
    ['[OrgContext] JWT missing org_id claim, refreshing token', 'warning'],
    ['[OrgContext] JWT/profile drift detected, refreshing token', 'warning'],
    // The line that decides whether there is anything left to enrich at all.
    ['[OrgCache] tags: clearEnrichment()', 'log'],
    ['[OrgCache] Invalidating all org-scoped data', 'log'],
    ['[AuthStore] Setting org context org_id: 42', 'log'],
  ];

  for (const [line, type] of ENRICHMENT_LINES) {
    it(`forwards ${JSON.stringify(line.slice(0, 32))}…`, () => {
      expect(shouldForwardConsoleLine(line, type)).toBe(true);
    });
  }

  it('widened by prefix, not by loosening a limb', () => {
    // The hazard the module docstring names: loosening an existing limb (a
    // case-insensitive `Connect`, say) would sweep in `[vite] connected` and a
    // great deal of other chatter. These lines talk about stores and orgs but
    // carry no bracketed prefix, so a prefix-shaped widening leaves them
    // filtered and a careless one does not.
    for (const line of [
      'restoring tagStore from localStorage',
      'org context ready',
      'TagStore rehydrated',
    ]) {
      expect(shouldForwardConsoleLine(line, 'log'), `should not forward: ${line}`).toBe(false);
    }
  });
});
