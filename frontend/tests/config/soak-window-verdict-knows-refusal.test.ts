/**
 * A refusal is a third state, and it must not be reported as a quiet device.
 *
 * `powerOffWindowTable` knew two: timeouts occurred (the window happened and
 * TRA-1217 absorbed it), or zero timeouts (the device was quiet and the arm
 * says nothing). After TRA-1229 there is a third, and it lands in the branch
 * built for the second:
 *
 *   the device REFUSED the command in ~34 ms, so no timeout ever fires
 *
 * On the 2026-09-01 arm that would have turned a window carrying ~1500
 * refusals into "the device never went silent in this run". A fault storm
 * reported as a clean arm is the exact failure this file guards against, and it
 * arrives through the fix for the previous one.
 *
 * Refs: TRA-1230, TRA-1229, TRA-1217.
 */

import { describe, it, expect } from 'vitest';
import { powerOffWindowTable } from '../../scripts/summarise-suite-runs.mjs';

const rep = (rep: number, signals: Record<string, unknown>) => ({
  rep,
  signals: {
    logMissing: false,
    powerOffTimeouts: 0,
    toleratedPowerOffs: 0,
    modeSwitchFailed: 0,
    commandRejections: {},
    ...signals,
  },
});

describe('powerOffWindowTable — refusal is not silence', () => {
  it('reports a refusal window as REFUSED, never as a quiet device', () => {
    const out = powerOffWindowTable([
      rep(1, {}),
      rep(2, {
        // No timeouts: TRA-1229 settles the command from its 0xA101 reply
        // before the 5s timer can fire. The refusals are the evidence.
        powerOffTimeouts: 0,
        commandRejections: { RFID_POWER_OFF: 27, GET_TRIGGER_STATE: 13 },
        toleratedPowerOffs: 14,
      }),
    ]);

    expect(out).toMatch(/REFUSED|refused/);
    // The old wording must not appear — it is the misreading this exists to stop.
    expect(out).not.toMatch(/never went silent/);
  });

  it('names the refused op codes, so a newly-refused one is visible', () => {
    const out = powerOffWindowTable([
      rep(1, { commandRejections: { GET_BATTERY_VOLTAGE: 4 } }),
    ]);
    expect(out).toContain('GET_BATTERY_VOLTAGE');
  });

  it('still says a genuinely quiet arm proves nothing', () => {
    const out = powerOffWindowTable([rep(1, {}), rep(2, {})]);
    expect(out).toMatch(/never went silent/);
  });

  it('still reports an absorbed timeout window as TRA-1217 earning its credit', () => {
    const out = powerOffWindowTable([
      rep(1, { powerOffTimeouts: 26, toleratedPowerOffs: 14 }),
    ]);
    expect(out).toMatch(/absorbed/);
  });
});
