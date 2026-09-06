/**
 * Every entry in the e2e console monitor's two lists must match something the
 * BROWSER can actually print. TRA-1224.
 *
 * ## The asymmetry this closes
 *
 * An allowlist entry can fail two ways and only one of them is ever loud:
 *
 *   matches too much   suppresses a real error — eventually loud, because
 *                      something breaks downstream and the suppression is found
 *   matches nothing    NO SYMPTOM AT ALL. The gate stays green, nothing points
 *                      at the list, and nobody acquires a reason to look
 *
 * The second one decayed unobserved for months. When this guard was written, 14
 * of the 15 entries across both lists matched nothing — casualties of the
 * ble-mcp-test replatform and the CS108 worker rebuild, where the emissions were
 * reworded out from under strings nobody re-checked. Not near-misses: the real
 * line for three of them was `[CommandManager] Command timeout: RFID_POWER_OFF`,
 * a different prefix and a different name form with no duration at all.
 *
 * The critical list is the worse half. Those entries are supposed to FAIL a run,
 * so seven dead ones were a gate that could not fire.
 *
 * ## Why this shape, and not the two obvious alternatives
 *
 * Rewriting the strings in place was rejected on the ticket: it resets the decay
 * clock and adds no failure mode, so the list merely LOOKS correct and the same
 * state recurs. "Assert at least one entry matched this run" was rejected too —
 * it is satisfiable by coincidence, since any single live entry makes it green
 * while the rest rot exactly as before.
 *
 * Ask the diagnostic question instead: what edit to the code under test turns
 * this red? Renaming or deleting an emission does. That IS the failure being
 * guarded, rather than a proxy for it.
 *
 * ## Why `src/` only
 *
 * `ConsoleMonitor` watches the Playwright PAGE console, so a producer has to be
 * code the browser loads — which is `src/`, and nothing else. Test files are not
 * in the page.
 *
 * That is not a shortcut, it is the trap. `connection.spec.ts` passes
 * `'Connection timeout'`, `'Transport error'` and `'Failed to start battery auto
 * reporting'` as its own inline options. Those are CONSUMERS of the strings.
 * Search the wider tree and each one proves its own liveness — the guard goes
 * green on the exact entries it was built to condemn.
 *
 * Code injected into the page from outside `src/` (the ble-mcp-test mock) is a
 * real browser-side producer and is handled by EXTERNALLY_PRODUCED, which makes
 * you name the producer rather than let "not found" quietly become "external".
 */

import { describe, it, expect } from 'vitest';
import path from 'node:path';
import { buildHaystack } from './source-haystack';
import {
  DEFAULT_ALLOWED_ERRORS,
  DEFAULT_CRITICAL_ERRORS,
} from '../e2e/helpers/console-utils';

const FRONTEND_ROOT = path.resolve(__dirname, '../..');

/**
 * Entries whose text is printed into the page console by code outside `src/`,
 * each with the producer named.
 *
 * An entry may only be listed here with a reason a reader can check. "Not found
 * in src" is not a reason — that is the finding, not the exemption.
 */
const EXTERNALLY_PRODUCED: Record<string, string> = {};

/**
 * Browser-side source only. See the `src/`-only reasoning above.
 *
 * Nothing needs excluding here, and that is a property of the root rather than
 * luck: the lists live in `tests/`, every consumer that passes them as options
 * lives in `tests/`, and neither tree is scanned.
 */
const BROWSER_HAYSTACK = buildHaystack([path.join(FRONTEND_ROOT, 'src')]);

/**
 * The two lists get OPPOSITE emptiness rules, and the asymmetry is the ticket's
 * own thesis rather than a convenience.
 *
 * Emptying the CRITICAL list is a silent catastrophe: nothing can fail a run,
 * and the suite reports clean forever. That must be loud.
 *
 * Emptying the ALLOWED list is safe, and is currently the honest state. An
 * allowlist suppresses errors; holding none means nothing is suppressed, which
 * fails MORE runs rather than fewer. Its dangerous direction is too many entries,
 * never too few — so demanding it stay non-empty would be pressure to invent a
 * benign condition to list, which is how the dead entries got written in the
 * first place.
 */
const LISTS: Array<{
  name: string;
  list: readonly string[];
  why: string;
  mustBeNonEmpty: boolean;
}> = [
  {
    name: 'DEFAULT_ALLOWED_ERRORS',
    list: DEFAULT_ALLOWED_ERRORS,
    why: 'a dead allowed entry suppresses nothing, so the list only LOOKS like it is protecting the run',
    mustBeNonEmpty: false,
  },
  {
    name: 'DEFAULT_CRITICAL_ERRORS',
    list: DEFAULT_CRITICAL_ERRORS,
    why: 'a dead critical entry is a gate that cannot fire — the run reports clean because nothing can trip it',
    mustBeNonEmpty: true,
  },
];

describe('e2e console monitor allowlists', () => {
  for (const { name: listName, list, why, mustBeNonEmpty } of LISTS) {
    describe(listName, () => {
      if (mustBeNonEmpty) {
        it('is not empty, because an empty critical list is a gate that cannot fire', () => {
          // Also stops the per-entry assertions below from vanishing into zero
          // assertions on a green suite — the same shape of silence the list
          // itself suffers from.
          expect(list.length).toBeGreaterThan(0);
        });
      }

      if (list.length === 0) {
        it('is empty, which is a real state and not an oversight', () => {
          // Named rather than left as a hole, so the next reader does not assume
          // the list was lost in a merge. Every entry it held matched nothing and
          // was deleted; suppressing nothing is the safe direction for an
          // allowlist. Add an entry back only when a run can prove it matches.
          expect(list).toEqual([]);
        });
      }

      for (const entry of list) {
        it(`\`${entry}\` matches something the browser can print`, () => {
          if (entry in EXTERNALLY_PRODUCED) {
            expect(EXTERNALLY_PRODUCED[entry].length).toBeGreaterThan(0);
            return;
          }

          expect(
            BROWSER_HAYSTACK.includes(entry),
            `Nothing under frontend/src/ prints ${JSON.stringify(entry)}.\n\n` +
              `${why}.\n\n` +
              `Either the code that logged it was renamed or removed — in which case DELETE ` +
              `this entry rather than reword it, because rewording resets the decay clock and ` +
              `adds no failure mode — or it is printed into the page from outside src/ (the ` +
              `injected mock), in which case add it to EXTERNALLY_PRODUCED with its producer named.`
          ).toBe(true);
        });
      }
    });
  }

  it('declares nothing that is not an entry', () => {
    // An EXTERNALLY_PRODUCED declaration whose entry has since been deleted is
    // dead weight that reads as coverage.
    const all = [...DEFAULT_ALLOWED_ERRORS, ...DEFAULT_CRITICAL_ERRORS];
    const orphans = Object.keys(EXTERNALLY_PRODUCED).filter((e) => !all.includes(e));
    expect(orphans, `EXTERNALLY_PRODUCED names no longer in either list: ${orphans.join(', ')}`).toEqual(
      []
    );
  });

  it('lists no externally-produced entry that has since gained a src/ producer', () => {
    // The reverse drift: an entry declared external that the app now prints
    // itself. Harmless to the gate, but the declaration becomes a lie and the
    // next reader trusts it.
    const nowLocal = Object.keys(EXTERNALLY_PRODUCED).filter((e) => BROWSER_HAYSTACK.includes(e));
    expect(nowLocal, `declared external but now printed by src/: ${nowLocal.join(', ')}`).toEqual([]);
  });
});
