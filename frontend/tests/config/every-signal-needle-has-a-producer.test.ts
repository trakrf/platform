/**
 * Every needle in SIGNALS must correspond to something that can actually emit it.
 *
 * ## The gap this closes
 *
 * `scripts/suite-run-signals.mjs` is a table of log substrings the soak driver
 * counts. It is a CONSUMER-side waiter, and until now nothing checked the
 * producer side — nothing verified that each needle matches a line some code is
 * still capable of writing.
 *
 * The existing `harnessLines` canary does not cover this. It catches a wholly
 * VOID capture: zero of everything means the log was never written. It cannot
 * catch ONE DEAD NEEDLE AMONG LIVE ONES — the other counts are non-zero, the
 * canary is satisfied, and the dead needle reports a confident `0` that reads as
 * "this never happened" rather than "nothing can make this happen".
 *
 * That distinction is not hypothetical. It is the same shape as the cross-repo
 * retry list that held three strings the peer never sent: the list was MOSTLY
 * live, which is exactly why nobody looked at the dead entry (TRA-1187).
 *
 * ## Why a test rather than a grep
 *
 * The question "does anything emit this?" has to be asked at a moment when the
 * answer can fail the build. A needle goes dead not when someone edits this
 * table but when someone edits the code that logs — renames a prefix, drops a
 * log line as noise, replaces a harness. That edit is somewhere else entirely
 * and its author has no reason to look here.
 *
 * ## Externally-produced needles
 *
 * Some needles match text this repo does not write — a Node errno, a jsdom
 * WebSocket failure. Those cannot be found in `src/`, so they are declared
 * below WITH THEIR PRODUCER NAMED. Declaring is the point: an unexplained needle
 * fails, and the only way to pass is to say where the string comes from. That
 * keeps "I could not find it" from silently becoming "it must be external".
 */

import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { SIGNALS } from '../../scripts/suite-run-signals.mjs';

const FRONTEND_ROOT = path.resolve(__dirname, '../..');

/**
 * Needles produced outside this repo, each with the producer that emits it.
 *
 * A needle may only be listed here with a reason a reader can check. "Not found
 * in src" is not a reason.
 */
const EXTERNALLY_PRODUCED: Record<string, string> = {
  transportRefused:
    "Node's own errno text for a refused TCP connect; surfaces when a Node-side " +
    'caller reaches a bridge that is not listening.',
  transportUnreachable:
    "jsdom's WebSocket error text. The mock's transport uses whatever global " +
    'WebSocket the runtime provides, and under vitest that is jsdom, which reports ' +
    'a bare `WebSocket error` with no errno at all.',
};

/**
 * Needles whose text is BUILT AT RUNTIME, so the whole string never appears in
 * source. Each declares the two halves that do, and both are checked.
 *
 * This is not a loophole — it is stricter than nothing and honest about why. The
 * static half breaks if the message is reworded; the dynamic half breaks if the
 * value is renamed. What it cannot prove is that the two are ever combined,
 * which is exactly what a template makes unprovable from source alone. Say so
 * here rather than let the needle sit in the plain list looking verified.
 */
const COMPOSED_AT_RUNTIME: Record<string, { staticPart: string; dynamicPart: string }> = {
  // CS108WorkerTestHarness rejects with `Timeout waiting for event: ${eventType}`,
  // and the event name is a WorkerEventType member.
  triggerTimeout: {
    staticPart: 'Timeout waiting for event: ',
    dynamicPart: 'TRIGGER_STATE_CHANGED',
  },
};

/** Source trees that can plausibly emit a log line. */
const SEARCH_DIRS = ['src', 'tests', 'scripts'];

function collectSources(dir: string, acc: string[] = []): string[] {
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return acc;
  }
  for (const entry of entries) {
    if (entry === 'node_modules' || entry === 'dist' || entry.startsWith('.')) continue;
    const full = path.join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) {
      collectSources(full, acc);
    } else if (/\.(ts|tsx|mjs|js)$/.test(entry)) {
      acc.push(full);
    }
  }
  return acc;
}

/**
 * Strip comments, because a needle mentioned in prose is not a producer.
 *
 * This is not tidiness — it is the defect being guarded against, one level down.
 * The dead `'Device busy'` retry arm looked live to a plain grep precisely
 * because every occurrence of the string in the shipped bundle was inside a
 * comment explaining it. A check that counts prose as evidence confirms itself
 * on documentation of the very thing that is missing.
 *
 * Verified by execution rather than assumed: `transportUnreachable` passed the
 * first version of this test on two comment hits and nothing else.
 */
function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/[^\n]*/g, '$1');
}

/**
 * Source text of everything that could emit a log line, comments removed.
 *
 * Read once. The signals module itself is EXCLUDED — it contains every needle by
 * definition, so including it would make the check pass on its own table. A
 * check that can be satisfied by the thing it is checking is not a check.
 */
const HAYSTACK = (() => {
  const signalsModule = path.join(FRONTEND_ROOT, 'scripts', 'suite-run-signals.mjs');
  const files = SEARCH_DIRS.flatMap((d) => collectSources(path.join(FRONTEND_ROOT, d)));
  return files
    .filter((f) => f !== signalsModule && !f.endsWith('every-signal-needle-has-a-producer.test.ts'))
    .map((f) => stripComments(readFileSync(f, 'utf8')))
    .join('\n');
})();

describe('SIGNALS needles', () => {
  it('has needles to check, so an empty table cannot pass vacuously', () => {
    // Without this, deleting the table turns every assertion below into zero
    // assertions and the suite goes green — the same failure the table is for.
    expect(Object.keys(SIGNALS).length).toBeGreaterThan(0);
  });

  for (const [name, needle] of Object.entries(SIGNALS as Record<string, string>)) {
    it(`\`${name}\` matches something this repo can emit`, () => {
      if (name in EXTERNALLY_PRODUCED) {
        expect(EXTERNALLY_PRODUCED[name].length).toBeGreaterThan(0);
        return;
      }

      const composed = COMPOSED_AT_RUNTIME[name];
      if (composed) {
        // Both halves, because either one alone would keep passing through the
        // rename that kills the needle.
        expect(
          HAYSTACK.includes(composed.staticPart),
          `the static half ${JSON.stringify(composed.staticPart)} is gone — the message was reworded`
        ).toBe(true);
        expect(
          HAYSTACK.includes(composed.dynamicPart),
          `the dynamic half ${JSON.stringify(composed.dynamicPart)} is gone — the value was renamed`
        ).toBe(true);
        // And the declared halves must actually reconstruct the needle, or this
        // entry is describing a different string than the driver greps for.
        expect(composed.staticPart + composed.dynamicPart).toBe(needle);
        return;
      }

      // The needle must appear in source that is not the table itself. A log
      // line's literal contains the needle as a substring, so a substring test
      // is the right shape — the needle is what the driver greps for.
      expect(
        HAYSTACK.includes(needle),
        `No source under ${SEARCH_DIRS.join('/, ')}/ contains ${JSON.stringify(needle)}.\n` +
          `Either the code that logged it was renamed or removed — in which case this needle now ` +
          `reports a confident 0 that reads as "never happened" — or the string is produced ` +
          `outside this repo, in which case add it to EXTERNALLY_PRODUCED with its producer named.`
      ).toBe(true);
    });
  }

  it('lists no externally-produced needle that has since gained a local producer', () => {
    // The reverse drift: a needle declared external that this repo now emits
    // itself. Harmless to counting, but the declaration becomes a lie, and the
    // next reader trusts it. Cheap to check while we are here.
    const nowLocal = Object.keys(EXTERNALLY_PRODUCED).filter((name) => {
      const needle = (SIGNALS as Record<string, string>)[name];
      return needle && HAYSTACK.includes(needle);
    });
    expect(nowLocal, `declared external but now emitted locally: ${nowLocal.join(', ')}`).toEqual(
      []
    );
  });

  it('declares nothing that is not a needle', () => {
    // An entry in EXTERNALLY_PRODUCED whose needle no longer exists is dead
    // weight that reads as coverage.
    const orphans = [...Object.keys(EXTERNALLY_PRODUCED), ...Object.keys(COMPOSED_AT_RUNTIME)].filter(
      (name) => !(name in SIGNALS)
    );
    expect(orphans, `EXTERNALLY_PRODUCED names no longer in SIGNALS: ${orphans.join(', ')}`).toEqual(
      []
    );
  });
});
