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
import {
  LOST_CONNECTED_PREFIX,
  lostConnectedMessage,
} from '../../src/stores/lost-connected-message';
import { E2E_SIGNALS } from '../../scripts/suite-run-signals.mjs';

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
 * The same class again, one PR later — TRA-1259's own signature line.
 *
 * The warning added by #670 interpolates the previous reader state into the
 * middle of the sentence, and nothing in the constant part carried a KEEP token:
 * `CONNECTED` is not `Connect`, `Disconnected` is not `disconnect`. So whether
 * an occurrence reached the run log was decided by which state was interpolated
 * — `Connected` and `Error` matched by accident, `Configuring`, `Busy` and
 * `Scanning` did not. Three of five dropped.
 *
 * That is not a uniform undercount, which is what makes it worse than a zero:
 * the SURVIVING two are the ordinary teardown cases, and the dropped three
 * include `Scanning -> Disconnected`, the transition a trigger-hold sweep
 * produces. The instrument was blindest exactly where the ticket was looking.
 *
 * `console.warn` is Playwright type `warning`, so the `type === 'error'`
 * short-circuit never covered it either — the type is asserted below rather than
 * assumed, because passing `'error'` here would make this suite pass vacuously.
 */
describe('the lost-CONNECTED line survives the forwarder in every reader state', () => {
  // The states that can actually produce the warning: `setReaderState` fires it
  // only when leaving an ESTABLISHED state, i.e. not DISCONNECTED (the idempotent
  // teardown) and not CONNECTING (the sub-frame bring-up window, excluded by
  // design in #670). Looping over all of ReaderState anyway is deliberate — a
  // state that cannot reach the warning today still must not be able to make the
  // line vanish if the guard is ever widened.
  for (const state of ALL_READER_STATES) {
    it(`forwards the line rendered for \`${state}\``, () => {
      const line = lostConnectedMessage(state);
      expect(
        shouldForwardConsoleLine(line, 'warning'),
        `The forwarder DROPS the lost-CONNECTED line when the previous state is ${state}:\n` +
          `  ${line}\n` +
          'TRA-1259 is read out of this line. A dropped occurrence does not read as ' +
          'dropped — it reads as the store never having lost CONNECTED, which is ' +
          'the exact conclusion the ticket exists to test.'
      ).toBe(true);
    });
  }

  it('renders a byte-identical prefix for every state', () => {
    const prefixes = new Set(
      ALL_READER_STATES.map((s) => lostConnectedMessage(s).slice(0, LOST_CONNECTED_PREFIX.length))
    );
    expect([...prefixes]).toEqual([LOST_CONNECTED_PREFIX]);
  });

  it('forwards the bare invariant prefix on its own', () => {
    expect(shouldForwardConsoleLine(LOST_CONNECTED_PREFIX, 'warning')).toBe(true);
  });

  it('is kept on its own merits, not by the `error` short-circuit', () => {
    // The producer is console.warn. If this suite passed only for type 'error'
    // it would be asserting nothing about the path the line actually takes.
    const line = lostConnectedMessage(ReaderState.SCANNING);
    expect(shouldForwardConsoleLine(line, 'warning')).toBe(true);
    expect(shouldForwardConsoleLine(line, 'info')).toBe(true);
  });

  it('matches the needle the soak instruments count', () => {
    // A prefix the forwarder keeps but the signals module does not count, or vice
    // versa, is still an unobservable event. Same coupling `E2E_BROWSER_NEEDLES`
    // declares, asserted on the literal.
    expect(E2E_SIGNALS.lostConnected).toBe(LOST_CONNECTED_PREFIX);
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
