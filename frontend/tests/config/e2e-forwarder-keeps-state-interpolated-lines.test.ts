/**
 * A forwarded line must not be forwarded only SOMETIMES.
 *
 * ## The defect (TRA-1253)
 *
 * `every-signal-needle-has-a-producer.test.ts` proves an emitter exists for each
 * needle. That is a necessary check and not a sufficient one: it says nothing
 * about whether the line the emitter actually renders survives
 * `shouldForwardConsoleLine` on every branch it can take.
 *
 * The settle-and-retry line added by TRA-1225 interpolated the reader state into
 * the middle of the sentence, and the forwarder is a list of case-sensitive
 * substrings. So:
 *
 *   | readerState at the push | rendered line contains | forwarded? |
 *   | Connecting              | `Connect`              | yes        |
 *   | Busy                    | no KEEP token          | NO         |
 *
 * One emitter, one needle list, and observability decided by which branch the
 * reader happened to be in. A grep returning zero is then indistinguishable from
 * the code path never running — worse than a dead entry, which at least reads
 * zero honestly.
 *
 * ## Why this is a loop over states rather than one assertion
 *
 * Asserting the `Busy` case alone fixes today's bug and leaves the class open.
 * The question is not "does Busy work now" but "can any state make this line
 * vanish", so the test asks it of EVERY member of `ReaderState`. A new state
 * added later is covered without anyone remembering to come back here.
 *
 * The line is rendered by the shipped renderer rather than re-typed, so a
 * reworded emitter fails this test instead of silently passing it.
 */

import { describe, it, expect } from 'vitest';
import path from 'node:path';
import { shouldForwardConsoleLine } from '../e2e/helpers/console-forwarding';
import { buildHaystack } from './source-haystack';
import { ReaderState } from '../../src/worker/types/reader';
import {
  SETTLE_DEFERRAL_PREFIX,
  settleDeferralMessage,
} from '../../src/worker/cs108/settle-deferral-message';

const FRONTEND_ROOT = path.resolve(__dirname, '../..');

const ALL_READER_STATES = Object.values(ReaderState);

describe('the settle-deferral line survives the forwarder in every reader state', () => {
  it('has states to check, so the loop cannot pass vacuously', () => {
    expect(ALL_READER_STATES.length).toBeGreaterThan(0);
  });

  for (const state of ALL_READER_STATES) {
    it(`forwards the line rendered for \`${state}\``, () => {
      const line = settleDeferralMessage(state);
      expect(
        shouldForwardConsoleLine(line, 'info'),
        `The forwarder DROPS the deferral line when readerState is ${state}:\n` +
          `  ${line}\n` +
          'That makes the event invisible on exactly the branch it fires on, and a ' +
          'grep for it then reads a confident 0. Keep the KEEP token in the ' +
          'invariant part of the line.'
      ).toBe(true);
    });
  }

  it('renders a byte-identical prefix for every state', () => {
    // This is what makes the forwarder's decision state-independent. If a future
    // edit moves the interpolation back into the matched region, the prefixes
    // diverge and this fails before anyone loses a measurement to it.
    const prefixes = new Set(
      ALL_READER_STATES.map((s) => settleDeferralMessage(s).slice(0, SETTLE_DEFERRAL_PREFIX.length))
    );
    expect([...prefixes]).toEqual([SETTLE_DEFERRAL_PREFIX]);
  });

  it('forwards the bare invariant prefix on its own', () => {
    // The predicate is a disjunction of `includes`, so passing the prefix alone
    // is what guarantees every line containing it passes.
    expect(shouldForwardConsoleLine(SETTLE_DEFERRAL_PREFIX, 'info')).toBe(true);
  });
});

/**
 * The other half of TRA-1253: a multi-line event must not survive by halves.
 *
 * `every-signal-needle-has-a-producer.test.ts` proves an emitter exists. It
 * cannot prove that EVERY LINE of a multi-line event survives the forwarder, and
 * a needle matching an event where only some lines carry a KEEP token produces a
 * count that is silently halved. That is worse than a zero: a low number reads
 * as a mostly-clean arm, where an honest `null` reads as unobserved.
 *
 * The lines below are rendered the way `src/worker/cs108/command.ts` renders
 * them, both limbs of the one occurrence.
 */
describe('both lines of a CommandInFlightError occurrence are forwarded', () => {
  // command.ts:734 — the tolerated step's WARN. Carries no `Failed`, and before
  // TRA-1253 carried no KEEP token at all.
  const TOLERATED_WARN =
    '[CommandManager] RFID_POWER_OFF (0x8001) went unanswered after 2 attempt(s): ' +
    'Command already active - executeCommand called concurrently — tolerated, continuing the sequence';

  // The failing sequence's ERROR. This half was always kept, via `Failed`.
  const SET_MODE_ERROR =
    '[setMode] Failed to set Idle mode: CommandInFlightError: ' +
    'Command already active - executeCommand called concurrently';

  it('forwards the tolerated WARN half', () => {
    expect(
      shouldForwardConsoleLine(TOLERATED_WARN, 'warning'),
      'The WARN half is dropped, so a CommandInFlightError occurrence counts once ' +
        'instead of twice and the arm reports roughly half the true value.'
    ).toBe(true);
  });

  it('forwards the ERROR half', () => {
    expect(shouldForwardConsoleLine(SET_MODE_ERROR, 'error')).toBe(true);
  });

  it('forwards the two `[CommandManager]` warns that were dropped for the same reason', () => {
    // powerOffTimeouts and toleratedPowerOffs are `logger.warn` and carried no
    // KEEP token either. Same cause, same fix — matching the emitter's prefix
    // rather than three individual message texts.
    const POWER_OFF_TIMEOUT = '[CommandManager] Command timeout: RFID_POWER_OFF';
    expect(shouldForwardConsoleLine(POWER_OFF_TIMEOUT, 'warning')).toBe(true);
    expect(TOLERATED_WARN).toContain('tolerated, continuing the sequence');
  });
});

describe('the deferral line has a real producer', () => {
  it('is emitted by something under src/', () => {
    // Same rule the sibling producer guards apply: the browser only loads src/,
    // and a string that appears only in a comment or a test is not a producer.
    const haystack = buildHaystack(
      [path.join(FRONTEND_ROOT, 'src')],
      (file) => file.endsWith('settle-deferral-message.ts')
    );
    expect(
      haystack.includes('settleDeferralMessage'),
      'Nothing under src/ calls settleDeferralMessage, so this line can no longer ' +
        'be emitted and the forwarder limb that keeps it is dead weight.'
    ).toBe(true);
  });
});
