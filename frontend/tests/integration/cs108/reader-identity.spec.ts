/**
 * What the reader on the bench actually is.
 *
 * ## Why this is a spec and not a script
 *
 * A script would be run once, by whoever remembered, and its output would live
 * in whichever session happened to run it. A spec runs on every arm, so **every
 * arm records the firmware it was taken on** — which is the whole point of
 * TRA-1232. Four transport captures of a device-side defect exist today and not
 * one of them can say what firmware it was observed on; flashing destroys that
 * attribution permanently.
 *
 * ## What this spec is actually testing
 *
 * Not the decode. That is pinned by unit tests, against bytes.
 *
 * This asks the only questions unit tests cannot: **does the CS108 answer
 * these commands at all**, and does it answer the ones nobody has ever sent it.
 * `createFirmwareCommand` has had a `READ_REGISTER` branch since it was written
 * and, until this ticket, no caller — so `MAC_ERROR` and the RFID firmware
 * version travel a path that has never been exercised on hardware.
 *
 * ⚠ It also asserts the thing that would be worst to get wrong quietly: that a
 * register read does not leave a `Command timeout: RFID_FIRMWARE_COMMAND`
 * behind. That line is the TRA-1239 signal, and two of them per connection
 * would inflate a counter every current investigation reads.
 *
 * Refs: TRA-1232, TRA-1239.
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { CS108WorkerTestHarness } from './CS108WorkerTestHarness';
import { ReaderMode } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';
import type { ReaderDetails } from '@/worker/types/reader';

/** The most complete picture the reader gave us, across every update. */
function latestDetails(harness: CS108WorkerTestHarness): ReaderDetails {
  const events = harness.getEventsByType(WorkerEventType.READER_DETAILS);
  return (events[events.length - 1]?.payload as { details: ReaderDetails } | undefined)?.details
    ?? {};
}

describe('CS108 reader identity', () => {
  let harness: CS108WorkerTestHarness;

  /**
   * Every `console.warn` the worker made during connect and the first mode.
   *
   * Captured rather than read back off an event, because `CommandManager`'s
   * timeout line is a `logger.warn` straight to the console — it does not
   * travel as a `DEBUG_LOG` worker event. Asserting against the event stream
   * would have passed no matter what the reader did, which is the exact shape
   * of false green this repo keeps finding.
   */
  const workerWarnings: string[] = [];
  let restoreWarn: (() => void) | undefined;

  beforeAll(async () => {
    const originalWarn = console.warn;
    console.warn = (...args: unknown[]) => {
      workerWarnings.push(args.map(String).join(' '));
      originalWarn(...args);
    };
    restoreWarn = () => { console.warn = originalWarn; };

    harness = new CS108WorkerTestHarness();
    await harness.initialize();

    // The three Bluetooth-board reads ride this.
    const connected = await harness.connect();
    expect(connected).toBe(true);

    // The two RFID register reads ride the first mode that powers the radio.
    await harness.setMode(ReaderMode.INVENTORY);

    console.log(`[Test] Reader identity: ${JSON.stringify(latestDetails(harness))}`);
  }, 90000);

  afterAll(async () => {
    try {
      await harness.setMode(ReaderMode.IDLE);
    } catch {
      // Best effort. cleanup() is the only thing that must run — a teardown
      // that leaks the link when anything ahead of it fails cost 63 of 200 reps
      // in the 2026-08-31 arm (TRA-1217).
    }
    await harness.cleanup();
    restoreWarn?.();
  }, 60000);

  describe('the Bluetooth board', () => {
    it('reports its Silicon Labs firmware version', () => {
      const { siliconLabsFirmware } = latestDetails(harness);
      // A version, not a shape check: `0.0.0` would mean the read landed and
      // the board had nothing to say, which is a different failure from silence
      // and worth being able to tell apart.
      expect(siliconLabsFirmware, 'no answer to 0xB000').toMatch(/^\d+\.\d+\.\d+$/);
      expect(siliconLabsFirmware).not.toBe('0.0.0');
    });

    it('reports its Bluetooth firmware version', () => {
      const { bluetoothFirmware } = latestDetails(harness);
      expect(bluetoothFirmware, 'no answer to 0xC000').toMatch(/^\d+\.\d+\.\d+$/);
      expect(bluetoothFirmware).not.toBe('0.0.0');
    });

    it('reports a serial number', () => {
      const { serialNumber } = latestDetails(harness);
      // `parseSerialNumber` yields '' for an unreadable payload, matching the
      // vendor's own try/catch, so a non-empty string is the assertion that
      // separates a read from a shrug.
      expect(serialNumber, 'no answer to 0xB004').toBeTruthy();
    });
  });

  describe('the RFID processor', () => {
    /**
     * The path nobody has ever taken. If this fails, the R2000 half of
     * TRA-1232 does not work and the two register reads are dead weight —
     * which is a real answer, and one only a reader can give.
     */
    it('answers a register read at all', () => {
      const { rfidFirmware } = latestDetails(harness);
      expect(rfidFirmware, 'no REG_RESP for FIRMWARE_VER (0x0000)').toBeDefined();
    });

    it('reports a firmware version, not a byte-swapped lookalike', () => {
      const { rfidFirmware } = latestDetails(harness);
      expect(rfidFirmware).toMatch(/^\d+\.\d+\.\d+$/);
      expect(rfidFirmware).not.toBe('0.0.0');
    });

    /**
     * `MAC_Error` is the R2000's own account of what is wrong, and the only
     * fault we have ever been able to see is the Bluetooth board's `0x0000` —
     * the messenger's complaint, not the radio's.
     *
     * Zero is the expected value on a healthy reader and it is a VALUE, so
     * `toBeDefined` is the assertion rather than a truthiness check.
     */
    it('reports its own error register', () => {
      const { macError } = latestDetails(harness);
      expect(macError, 'no REG_RESP for MAC_ERROR (0x0005)').toBeDefined();
      console.log(`[Test] MAC_Error: 0x${macError!.toString(16).padStart(4, '0')}`);
    });
  });

  /**
   * ⚠ The check that gates merging this branch.
   *
   * `Command timeout: RFID_FIRMWARE_COMMAND` is the TRA-1239 signal, counted
   * per op by `suite-run-signals.mjs`. Two register reads per connection that
   * went unanswered would add to that counter on every rep of every arm — a
   * measurement change dressed as a feature, in the instrument every current
   * investigation reads.
   *
   * Asserting on the log is the only way to see it: the reads are
   * `toleratesFailure`, so a silent device produces a passing mode change and
   * a quietly inflated counter.
   */
  it('does not leave a firmware-command timeout behind', () => {
    // Prove the capture works before reading anything out of it. An empty
    // array is the answer whether the reader was quiet or the spy never
    // installed, and those are not the same result — a clean connect can
    // legitimately warn about nothing, so "did it capture anything" is the
    // wrong canary. Push a line through and check it lands.
    const canary = '[Test] console capture canary';
    console.warn(canary);
    expect(
      workerWarnings,
      'the console spy is not installed, so a zero below would say nothing about the device'
    ).toContain(canary);

    const timeouts = workerWarnings.filter((line) =>
      line.includes('Command timeout: RFID_FIRMWARE_COMMAND')
    );

    expect(
      timeouts,
      'a register read went unanswered — this inflates the TRA-1239 counter on every rep'
    ).toEqual([]);
  });
});
