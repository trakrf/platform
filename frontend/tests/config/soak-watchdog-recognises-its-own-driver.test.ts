import { describe, it, expect } from 'vitest';
import os from 'node:os';
import { fieldIsClear, expectedSessionId } from '../../scripts/watch-soak-abort-criteria.mjs';
import { getBleBridgeConfig } from './ble-bridge.config';
import { getViteBridgeConfig } from './vite-bridge.config';

/**
 * The pre-flight gate must not abort on its own driver (TRA-1209).
 *
 * ## What it did
 *
 * Arming the watchdog against a running soak driver aborted immediately:
 *
 *     ABORT before start: the field is not clear —
 *       {"held":true,"session":"trakrf-handheld-dev-mssb","observer_count":0,…}
 *     exit 5
 *
 * Nothing was contending. That session is the DRIVER'S OWN rep 1, which had
 * connected in the seconds between the driver launching and the pre-flight
 * running. The documented usage takes `--driver-pid <pid>`, so the driver
 * necessarily starts first and the gate necessarily races its first connect.
 *
 * The abort message then actively misdirects — "a leftover mock-injected browser
 * tab is the usual cause and appears in no process list" — sending the operator
 * after a tab that does not exist.
 *
 * ## The narrower question
 *
 * `fieldIsClear` asked "is anything holding it". The question it exists to
 * answer is "is anything OTHER THAN MY SUBJECT holding it". It failed safe,
 * which is why it survived design review, and it made correctness depend on
 * operator timing in exactly the unattended case where nobody is there to get
 * the timing right.
 *
 * The holder's identity was in the reply the whole time.
 */

describe('a field held by nobody', () => {
  it('is clear', () => {
    expect(fieldIsClear({ held: false, observer_count: 0 })).toBe(true);
  });

  it('is not clear when something is observing', () => {
    // An observer is contention whoever owns the hold — most often a leftover
    // mock-injected browser tab, which appears in no process list.
    expect(fieldIsClear({ held: false, observer_count: 1 })).toBe(false);
  });
});

describe('a field held by our own driver', () => {
  const OURS = 'trakrf-handheld-dev-mssb';

  it('is clear once the gate knows whose session it is', () => {
    expect(fieldIsClear({ held: true, session: OURS, observer_count: 0 }, OURS)).toBe(true);
  });

  it('is NOT clear if something is also observing', () => {
    // Our own hold plus an observer is still contention. The observer is the
    // hazard, and it is orthogonal to who holds the command path.
    expect(fieldIsClear({ held: true, session: OURS, observer_count: 1 }, OURS)).toBe(false);
  });

  it('is not clear when a DIFFERENT session holds it', () => {
    expect(fieldIsClear({ held: true, session: 'someone-else', observer_count: 0 }, OURS)).toBe(
      false
    );
  });
});

describe('the own-session match cannot be satisfied by absence', () => {
  /**
   * The trap in the obvious implementation: `state.session === ownSession` is
   * true when both are undefined. A bridge reply that omits `session`, checked
   * by a watchdog that was given no expected session, would then read a held
   * field as clear — turning a fail-safe gate into a fail-open one, which is
   * strictly worse than the bug being fixed.
   */
  it('a held field with no expected session is not clear', () => {
    expect(fieldIsClear({ held: true, session: 'anything', observer_count: 0 })).toBe(false);
  });

  it('a held field with neither side naming a session is not clear', () => {
    expect(fieldIsClear({ held: true, observer_count: 0 })).toBe(false);
    expect(fieldIsClear({ held: true, observer_count: 0 }, undefined)).toBe(false);
  });

  it('an empty-string session on either side is not a match', () => {
    expect(fieldIsClear({ held: true, session: '', observer_count: 0 }, '')).toBe(false);
    expect(fieldIsClear({ held: true, session: null, observer_count: 0 }, 'ours')).toBe(false);
  });
});

describe('a reply that does not carry the fields is not evidence', () => {
  it('rejects a missing or malformed reply', () => {
    expect(fieldIsClear(null)).toBe(false);
    expect(fieldIsClear({})).toBe(false);
  });

  it('rejects a reply missing held or observer_count, even with a matching session', () => {
    expect(fieldIsClear({ session: 'ours', observer_count: 0 }, 'ours')).toBe(false);
    expect(fieldIsClear({ held: true, session: 'ours' }, 'ours')).toBe(false);
  });
});

describe('the expected session is DERIVED, not declared', () => {
  /**
   * This is the TRA-1206 lesson applied to the fix rather than to the bug: a
   * check whose subject is chosen by configuration the check does not read is
   * checking a thing it picked, not the thing that will run.
   *
   * A `--session` flag alone would have been that mistake in a new place — the
   * operator would have to keep it in step with `BLE_SESSION_ID` by hand, and a
   * stale flag reads as contention exactly like the bug being fixed. So the
   * default is computed the same way the configs compute it, and this test is
   * what stops them drifting.
   *
   * ## THERE ARE THREE COPIES OF THIS DERIVATION, and the third is the one that
   * actually decides the e2e session
   *
   *   scripts/watch-soak-abort-criteria.mjs   expectedSessionId()  — the watchdog
   *   tests/config/ble-bridge.config.ts       getBleBridgeConfig() — integration
   *   tests/config/vite-bridge.config.ts      getViteBridgeConfig() — MOCK INJECTION
   *
   * The last one is what Vite injects into the browser, so under e2e it is the
   * string the bridge actually reports back in `get_connection_state.session` —
   * which is what the watchdog's gate compares against. Checking the watchdog
   * against `ble-bridge.config.ts` ALONE would have been the same two-legs-
   * scoped-differently defect this ticket is fixing: a guard on a leg that is
   * not the one under test, passing because the two happen to agree today.
   *
   * They cannot import each other (one is plain .mjs under bare node, the others
   * are TypeScript importing the transport for its UUID constants), so all three
   * are asserted equal here.
   */
  it('matches every config that derives the same session', () => {
    const fromWatchdog = expectedSessionId();
    expect(fromWatchdog, 'watchdog vs integration config').toBe(getBleBridgeConfig().session.id);
    expect(fromWatchdog, 'watchdog vs the config that injects the browser mock').toBe(
      getViteBridgeConfig().sessionId
    );
  });

  it('honours BLE_SESSION_ID, because the config does', () => {
    const prev = process.env.BLE_SESSION_ID;
    process.env.BLE_SESSION_ID = 'explicit-override-for-this-test';
    try {
      expect(expectedSessionId()).toBe('explicit-override-for-this-test');
    } finally {
      if (prev === undefined) delete process.env.BLE_SESSION_ID;
      else process.env.BLE_SESSION_ID = prev;
    }
  });

  it('falls back to the hostname-derived form', () => {
    const prev = process.env.BLE_SESSION_ID;
    delete process.env.BLE_SESSION_ID;
    try {
      expect(expectedSessionId()).toBe(`trakrf-handheld-dev-${os.hostname()}`);
    } finally {
      if (prev !== undefined) process.env.BLE_SESSION_ID = prev;
    }
  });
});
