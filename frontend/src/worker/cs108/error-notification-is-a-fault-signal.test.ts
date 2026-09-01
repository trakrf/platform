/**
 * `0xA101` is a fault signal, not background noise.
 *
 * The CS108 answers a command it will not honour with `ERROR_NOTIFICATION`
 * rather than with the op code being rejected. Measured on hardware during the
 * TRA-1197 after-arm of 2026-09-01: across an 86-minute window, 1558 commands
 * went unanswered and 1543 `0xA101` frames came back — one per command, median
 * lag 34 ms, every one inside 100 ms. A healthy response to the same op code
 * measured 26 ms. They are replies.
 *
 * Every one of them was thrown away, three different ways, and each test below
 * fails against the behaviour that shipped before it:
 *
 *   1. `reader.ts` matched code 0x0000 and `continue`d, on a comment asserting
 *      the frames were "spurious" and "don't indicate actual communication
 *      problems". 1542 of the 1543 carried exactly that code.
 *   2. `system/error.ts` numbered every code one higher than the byte-stream
 *      spec, so the device's 0x0000 rendered as "Unknown error" while
 *      `command.ts` — reading the same wire bytes — called it correctly.
 *   3. The rate limiter capped logging at 3 per code per worker, so 1716 frames
 *      produced 8 log lines and nothing counted the rest.
 *
 * Refs: TRA-1229, TRA-1223.
 */

import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest';
import { NotificationManager } from './notification/manager';
import { CS108ErrorCode, ERROR_DESCRIPTIONS } from './system/error';
import type { CS108Packet, CS108Event } from './type';
import { ReaderMode, ReaderState } from '../types/reader';

/** An `0xA101` carrying `code`, shaped as the router delivers it. */
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

describe('0xA101 error codes match the byte-stream spec', () => {
  /**
   * The spec's table, verbatim (§ "Payload – Notification", 0xA101):
   *
   *   0x0000 Wrong header prefix    0x0001 Payload length too large
   *   0x0002 Unknown target         0x0003 Unknown event
   *
   * `system/error.ts` shipped each of these one higher. The device's most
   * common code in the wild is 0x0000, which that numbering leaves unmapped.
   */
  it('numbers every code as the spec does', () => {
    expect(CS108ErrorCode.WRONG_HEADER_PREFIX).toBe(0x0000);
    expect(CS108ErrorCode.PAYLOAD_LENGTH_TOO_LARGE).toBe(0x0001);
    expect(CS108ErrorCode.UNKNOWN_TARGET).toBe(0x0002);
    expect(CS108ErrorCode.UNKNOWN_EVENT).toBe(0x0003);
  });

  it('describes 0x0000 — the code the device actually sends — rather than leaving it unmapped', () => {
    expect(ERROR_DESCRIPTIONS[0x0000]).toBe('Wrong header prefix');
  });
});

describe('0xA101 reaches a handler and is counted', () => {
  let manager: NotificationManager;
  let router: ReturnType<NotificationManager['getRouter']>;
  let postMessageSpy: Mock;

  beforeEach(() => {
    vi.useFakeTimers();
    postMessageSpy = vi.fn();
    // The handler emits through `postWorkerEvent`, which posts on the worker
    // global rather than through the manager's callback.
    globalThis.postMessage = postMessageSpy;
    manager = new NotificationManager(
      (event) => postMessageSpy(event),
      {
        debug: false,
        getCurrentMode: () => ReaderMode.IDLE,
        getReaderState: () => ReaderState.CONNECTED,
      }
    );
    router = manager.getRouter();
  });

  afterEach(() => {
    vi.useRealTimers();
    manager.cleanup();
  });

  it('reports code 0x0000 by its spec name, not as "Unknown error"', () => {
    router.handleNotification(errorPacket(0x0000));

    expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
      type: 'DEVICE_ERROR',
      payload: expect.objectContaining({
        message: 'Wrong header prefix',
        code: '0000',
      }),
    }));
  });

  /**
   * The rate limiter exists to keep the log readable and it should stay. What
   * it must not do is make the frames disappear: on the hardware run it turned
   * 1716 arrivals into 8 log lines, and nothing else recorded that they had
   * happened at all. A count is what a soak arm can read.
   */
  it('counts every arrival even when logging is rate-limited', () => {
    const handler = manager.getErrorNotificationHandler();

    for (let i = 0; i < 50; i += 1) {
      router.handleNotification(errorPacket(0x0000));
    }

    expect(handler.getErrorCount(0x0000)).toBe(50);
    expect(handler.getTotalErrorCount()).toBe(50);
  });

  it('counts each code separately', () => {
    const handler = manager.getErrorNotificationHandler();

    router.handleNotification(errorPacket(0x0000));
    router.handleNotification(errorPacket(0x0002));
    router.handleNotification(errorPacket(0x0002));

    expect(handler.getErrorCount(0x0000)).toBe(1);
    expect(handler.getErrorCount(0x0002)).toBe(2);
    expect(handler.getTotalErrorCount()).toBe(3);
  });
});
