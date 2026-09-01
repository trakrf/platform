/**
 * A refused command must be countable per op code, and a refusal must not read
 * as a quiet device.
 *
 * ## The gap this closes
 *
 * `[CommandManager] Command timeout: <name>` is logged from exactly one place —
 * the `setTimeout` callback in `command.ts`. TRA-1229 settles a refused command
 * from its `0xA101` reply in ~34 ms, which clears that timeout, so the line
 * never fires. Both dependent needles match it literally:
 *
 *   powerOffTimeouts       '[CommandManager] Command timeout: RFID_POWER_OFF'
 *   COMMAND_TIMEOUT_PREFIX '[CommandManager] Command timeout: '
 *
 * So after TRA-1229 a wedge in which the device refused ~1500 commands reports
 * `powerOffTimeouts 0` and `commandTimeouts {}` — and the summariser's own
 * silent-window text reads `powerOffTimeouts == 0` as "the device was quiet".
 * **A fault storm renders as a clean arm.** That is the failure this whole
 * signal table exists to prevent, arriving through the fix for the last one.
 *
 * ## Why parsed per op, not another needle
 *
 * Same reason as TRA-1226. A fixed list counts only what somebody enumerated
 * and reads a confident 0 for the rest; parsing the op name out of the line
 * means a newly-refused op code shows up without anyone having predicted it.
 * That property is what surfaced `0xA001` in the first place.
 *
 * Refs: TRA-1230, TRA-1229, TRA-1226.
 */

import { describe, it, expect } from 'vitest';
import {
  countCommandRejections,
  COMMAND_REJECTION_PREFIX,
} from '../../scripts/suite-run-signals.mjs';

const line = (op: string, code: string, desc: string) =>
  `[Worker] WARN: ${COMMAND_REJECTION_PREFIX}${op} — ${desc} (0x${code})`;

describe('countCommandRejections', () => {
  it('counts each refused op separately, so a newly-refused op needs no enumeration', () => {
    const text = [
      line('RFID_POWER_OFF', '0000', 'Wrong header prefix'),
      line('RFID_POWER_OFF', '0000', 'Wrong header prefix'),
      line('GET_TRIGGER_STATE', '0000', 'Wrong header prefix'),
      'unrelated noise',
    ].join('\n');

    expect(countCommandRejections(text)).toEqual({
      RFID_POWER_OFF: 2,
      GET_TRIGGER_STATE: 1,
    });
  });

  it('surfaces an op code that appears in no needle table', () => {
    const out = countCommandRejections(line('SOME_UNPREDICTED_OP', '0002', 'Unknown target'));
    expect(out).toEqual({ SOME_UNPREDICTED_OP: 1 });
  });

  /**
   * `{}` is a real measurement — nothing was refused. It must stay distinct
   * from the `null` a runner returns when it cannot observe these lines at all.
   */
  it('returns an empty object for a rep with no refusals, never null', () => {
    expect(countCommandRejections('a clean rep')).toEqual({});
  });

  it('does not confuse a timeout line for a rejection', () => {
    expect(countCommandRejections('[CommandManager] Command timeout: RFID_POWER_OFF')).toEqual({});
  });
});
