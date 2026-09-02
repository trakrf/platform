/**
 * The soak instrument must be able to see `0xA101` fault storms.
 *
 * On the 2026-09-01 after-arm the device sent 1543 `ERROR_NOTIFICATION` frames
 * inside one 86-minute window — one per unanswered command — and the arm
 * reported nothing about them. Two reasons, both now fixed: `reader.ts`
 * discarded the frames outright, and the handler's rate limiter capped logging
 * at a few lines per code so no count survived to the log.
 *
 * Counting log LINES cannot work here, because the rate limiter is still there
 * and should be: it is what keeps an 18-per-minute fault storm from burying the
 * rep log. So the producer carries its own running total and the parser reads
 * the highest one. A rate-limited line still reports an accurate count.
 *
 * ⚠ These fixtures are SYNTHETIC — they were written by hand and so cannot
 * catch a producer that stops emitting what they describe. That is exactly how
 * the original design shipped: the parser was verified against a log that had
 * never been through the rate limiter. The test that closes that gap drives the
 * real handler and reads what it actually wrote —
 * `src/worker/cs108/error-count-survives-rate-limiting.test.ts`. Keep both:
 * this one pins the grammar, that one pins the contract.
 *
 * Refs: TRA-1229, TRA-1231.
 */

import { describe, it, expect } from 'vitest';
import { countErrorNotifications, ERROR_NOTIFICATION_TOTAL_RE } from '../../scripts/suite-run-signals.mjs';

describe('countErrorNotifications', () => {
  it('reads the running total rather than counting rate-limited lines', () => {
    const text = [
      '[Worker] WARN: [CS108 Error] fault-count total=1 code=0x0000',
      '[Worker] ERROR: [CS108 Error] Wrong header prefix (0x0000)',
      'noise',
      '[Worker] WARN: [CS108 Error] fault-count total=742 code=0x0000',
      '[Worker] WARN: [CS108 Error] fault-count total=1543 code=0x0000',
    ].join('\n');

    expect(countErrorNotifications(text)).toBe(1543);
  });

  /**
   * Zero is a real measurement — the device sent no faults — and must not be
   * confused with the `null` a runner returns when it cannot observe these at
   * all. Same null-vs-zero rule as the rest of the signal table.
   */
  it('returns 0 for a log that carried no error notifications', () => {
    expect(countErrorNotifications('a clean rep\nnothing here')).toBe(0);
  });

  it('is not fooled by a line that merely mentions the prefix', () => {
    expect(countErrorNotifications('[CS108 Error] something without a total')).toBe(0);
  });

  /**
   * ⚠ The descriptive line is RATE LIMITED and must not be the count's source.
   * Reading a total off it undercounts whenever a storm ends inside the
   * suppression window — measured at exactly 2x on a 3-rep mini arm.
   */
  it('ignores the rate-limited descriptive line', () => {
    expect(countErrorNotifications(
      '[Worker] ERROR: [CS108 Error] Wrong header prefix (0x0000) (9 occurrences in last 5s)'
    )).toBe(0);
  });

  /**
   * A REP IS NOT A SESSION, and taking a global maximum assumed it was.
   *
   * The counter lives on `ErrorNotificationHandler`, which is per worker
   * session. A wedged rep reconnects repeatedly, so each session starts its own
   * `total=1,2,3…` sequence and the highest number in the log belongs to
   * whichever session happened to run longest — not to the rep.
   *
   * Caught by an inequality that cannot hold. On the 2026-09-01 200-rep arm:
   *
   *   errorNotifications  208   (max-per-rep parse)
   *   commandRejections   247   (line count, exact)
   *
   * Every rejection IS an 0xA101 and not every 0xA101 produces a rejection, so
   * `rejections <= errorNotifications` must hold. It did not, and the gap was
   * concentrated exactly in the wedged reps — roughly 10x low through the
   * window the arm existed to measure, and correct everywhere else.
   *
   * ⚠ TRA-1231's test drove 50 arrivals through ONE handler. That input shape
   * passes under both the broken and the fixed parser, which is how this
   * survived. The fixture below is the shape that discriminates: two sessions,
   * each with its own sequence, concatenated.
   *
   * Refs: TRA-1236.
   */
  it('sums across worker sessions instead of taking the highest single total', () => {
    const session = (n: number) =>
      Array.from({ length: n }, (_, i) =>
        `[Worker] WARN: [CS108 Error] fault-count total=${i + 1} code=0x0000`
      );

    // A wedged rep: the worker reconnected twice, so three sequences restart.
    const text = [...session(4), 'reconnect', ...session(3), 'reconnect', ...session(36)].join('\n');

    // Not 36. The rep saw 43.
    expect(countErrorNotifications(text)).toBe(43);
  });

  /**
   * The boundary the run-splitting rule turns on: a repeated total is a NEW
   * session, not a continuation. A single session never emits the same total
   * twice — the counter is incremented before the line is written — so equality
   * can only mean the counter was reset.
   */
  it('treats a repeated total as a new session rather than a continuation', () => {
    const text = [
      '[Worker] WARN: [CS108 Error] fault-count total=1 code=0x0000',
      '[Worker] WARN: [CS108 Error] fault-count total=2 code=0x0000',
      '[Worker] WARN: [CS108 Error] fault-count total=2 code=0x0000',
    ].join('\n');

    expect(countErrorNotifications(text)).toBe(4);
  });

  /**
   * The healthy case must not regress: one session that climbs monotonically is
   * still read as one run, and a rate-limited gap in the middle of it does not
   * split it.
   */
  it('still reads a single rate-limited session as one run', () => {
    const text = [
      '[Worker] WARN: [CS108 Error] fault-count total=1 code=0x0000',
      '[Worker] WARN: [CS108 Error] fault-count total=742 code=0x0000',
      '[Worker] WARN: [CS108 Error] fault-count total=1543 code=0x0000',
    ].join('\n');

    expect(countErrorNotifications(text)).toBe(1543);
  });

  it('exports the pattern so the needle and the parser cannot drift apart', () => {
    expect(ERROR_NOTIFICATION_TOTAL_RE).toBeInstanceOf(RegExp);
  });
});
