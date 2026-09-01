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
 * Refs: TRA-1229.
 */

import { describe, it, expect } from 'vitest';
import { countErrorNotifications, ERROR_NOTIFICATION_TOTAL_RE } from '../../scripts/suite-run-signals.mjs';

describe('countErrorNotifications', () => {
  it('reads the running total rather than counting rate-limited lines', () => {
    const text = [
      '[Worker] ERROR: [CS108 Error] Wrong header prefix (0x0000) [1 seen this session]',
      'noise',
      '[Worker] ERROR: [CS108 Error] Wrong header prefix (0x0000) [742 seen this session]',
      '[Worker] ERROR: [CS108 Error] Wrong header prefix (0x0000) [1543 seen this session]',
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

  it('exports the pattern so the needle and the parser cannot drift apart', () => {
    expect(ERROR_NOTIFICATION_TOTAL_RE).toBeInstanceOf(RegExp);
  });
});
