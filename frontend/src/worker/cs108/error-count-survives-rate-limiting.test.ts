/**
 * The `0xA101` count must survive the log rate limiter.
 *
 * ## The defect this closes
 *
 * TRA-1229 put a running total on the `[CS108 Error]` line, reasoning that a
 * rate-limited line still reports an accurate count. That has a hole: **the
 * limiter eats the tail.** `ERROR_LOG_THRESHOLD` is 3, so a worker seeing six
 * arrivals inside the 5 s interval logs totals 1, 2, 3 and suppresses the rest.
 * The parser takes the highest total it can see, and the highest *logged* total
 * is not the final total.
 *
 * Measured on a deliberate 3-rep mini arm, 2026-09-01: the bridge saw 18
 * frames, the instrument reported 9. Exactly half.
 *
 * ## Why this test drives the real handler
 *
 * The original parser test asserted against a hand-written log — one that had
 * never been through the limiter, and in which every line therefore carried its
 * total. It passed, and it could not have failed, because the input was
 * constructed to contain the answer.
 *
 * So this drives arrivals through the actual `ErrorNotificationHandler`,
 * captures what it actually writes, and asks the real parser to recover the
 * count from that. The limiter is left switched on: it is doing a job worth
 * keeping, and the count has to survive it rather than be excused from it.
 *
 * Refs: TRA-1231, TRA-1229.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { NotificationManager } from './notification/manager';
import { countErrorNotifications } from '../../../scripts/suite-run-signals.mjs';
import type { CS108Packet, CS108Event } from './type';
import { ReaderMode, ReaderState } from '../types/reader';

function errorPacket(code: number): CS108Packet {
  return {
    event: {
      name: 'ERROR_NOTIFICATION',
      eventCode: 0xA101,
      module: 0xD9,
      isCommand: true,
      isNotification: true,
    } as CS108Event,
    payload: code,
    rawPayload: new Uint8Array([(code >> 8) & 0xFF, code & 0xFF]),
    rawData: new Uint8Array([]),
    timestamp: Date.now(),
  } as unknown as CS108Packet;
}

describe('the 0xA101 count survives the rate limiter', () => {
  let manager: NotificationManager;
  let router: ReturnType<NotificationManager['getRouter']>;
  let captured: string[];

  beforeEach(() => {
    vi.useFakeTimers();
    captured = [];
    // Capture what actually reaches the log, the way a rep log would.
    for (const level of ['error', 'warn', 'info'] as const) {
      vi.spyOn(console, level).mockImplementation((...args: unknown[]) => {
        captured.push(args.map(String).join(' '));
      });
    }
    globalThis.postMessage = vi.fn();
    manager = new NotificationManager(() => {}, {
      debug: false,
      getCurrentMode: () => ReaderMode.IDLE,
      getReaderState: () => ReaderState.CONNECTED,
    });
    router = manager.getRouter();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    manager.cleanup();
  });

  /**
   * 50 arrivals with no time advanced, so the limiter suppresses everything
   * past the third — the exact shape that undercounted on the mini arm.
   */
  it('recovers the true count from a log the limiter has thinned', () => {
    for (let i = 0; i < 50; i += 1) {
      router.handleNotification(errorPacket(0x0000));
    }

    const handler = manager.getErrorNotificationHandler();
    expect(handler.getTotalErrorCount()).toBe(50);

    // The limiter did its job on the DESCRIPTIVE line — the terse fault-count
    // line is unconditional by design and must be excluded from this check.
    const descriptive = captured.filter(
      l => l.includes('[CS108 Error]') && !l.includes('fault-count')
    );
    expect(descriptive.length).toBeLessThan(50);

    // And the unconditional line fired once per arrival, which is the property
    // the count depends on.
    const counted = captured.filter(l => l.includes('fault-count'));
    expect(counted.length).toBe(50);

    // And the count still survives into the log the parser reads.
    expect(countErrorNotifications(captured.join('\n'))).toBe(50);
  });

  it('agrees with the handler across several codes', () => {
    for (let i = 0; i < 20; i += 1) router.handleNotification(errorPacket(0x0000));
    for (let i = 0; i < 7; i += 1) router.handleNotification(errorPacket(0x0002));

    const handler = manager.getErrorNotificationHandler();
    expect(handler.getTotalErrorCount()).toBe(27);
    expect(countErrorNotifications(captured.join('\n'))).toBe(27);
  });

  it('still reports 0 for a rep that saw none', () => {
    expect(countErrorNotifications(captured.join('\n'))).toBe(0);
  });
});
