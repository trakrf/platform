/**
 * Reading TID and USER memory during an inventory scan, on real hardware.
 *
 * ## What only hardware can answer
 *
 * The unit tests prove the registers are packed correctly and that the parser
 * slices on the counts the packet reports. Neither says the radio accepted the
 * configuration, and neither can: a register write gets no response at all per
 * spec A.3, so "the sequence ran" and "the reader did what we asked" are
 * different claims. Only a tag answering with its own TID closes that gap.
 *
 * The spec also leaves two questions open that no amount of reading it will
 * settle, and both are answered here rather than guessed at:
 *
 *   1. Does a bank read the tag refuses still yield the EPC, or does the tag
 *      drop out of the inventory altogether? This decides whether capture is
 *      safe to leave switched on, or has to stay a deliberate mode.
 *   2. Does a 6-word TID read succeed against an arbitrary chip? Some carry
 *      only 2 words, which is why the length is a setting.
 *
 * ## Reading a failure here
 *
 * A tag must be in front of the reader. Zero tags means the test proved
 * nothing at all, so it fails loudly rather than passing vacuously — an
 * assertion that "no tag returned a bad TID" is satisfied by an empty field,
 * and that is exactly the false green this file exists to avoid.
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { CS108WorkerTestHarness } from './CS108WorkerTestHarness';
import { ReaderMode, ReaderState } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';

interface ScannedTag {
  epc: string;
  pc?: number;
  tid?: string;
  userData?: string;
}

describe('CS108 tag data capture (TRA-1251)', () => {
  let harness: CS108WorkerTestHarness;

  beforeAll(async () => {
    console.log('\n🔧 Initializing test harness and connecting...');
    harness = new CS108WorkerTestHarness();
    await harness.initialize();

    const connected = await harness.connect();
    expect(connected).toBe(true);
    console.log('✅ Connected to CS108');
  });

  afterAll(async () => {
    console.log('🔧 Cleaning up...');
    if (harness) {
      // cleanup() in finally: it is the only thing that releases the link for
      // the next spec file. TRA-1217.
      try {
        await harness.setMode(ReaderMode.IDLE);
      } catch (error) {
        console.error('Failed to set IDLE mode:', error);
      }
      try {
        await harness.disconnect();
      } finally {
        await harness.cleanup();
      }
    }
    console.log('✅ Disconnected');
  });

  /**
   * Push settings, enter INVENTORY, pull the trigger, collect what came back.
   *
   * Settings go in BEFORE setMode because the capture registers are written by
   * the mode-entry sequence, which reads them off readerSettings as it builds.
   * Setting them afterwards would configure the next scan, not this one.
   */
  async function scanWith(rfid: Record<string, unknown>, dwellMs = 2000): Promise<ScannedTag[]> {
    await harness.setSettings({ rfid: { transmitPower: 30, targetEPC: '', ...rfid } });
    await harness.waitForEvent(WorkerEventType.SETTINGS_UPDATED);

    await harness.setMode(ReaderMode.IDLE);
    const setModePromise = harness.setMode(ReaderMode.INVENTORY);
    await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.CONNECTED);
    await setModePromise;

    harness.clearEvents();

    await harness.simulateTriggerPress();
    await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === true, 8000);

    await new Promise(resolve => setTimeout(resolve, dwellMs));

    await harness.simulateTriggerRelease();
    await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === false, 8000);

    const events = harness.getEventsByType(WorkerEventType.TAG_READ);
    return events.flatMap(e => (e.payload?.tags || [])) as ScannedTag[];
  }

  it('returns real TID and USER data with capture on', { timeout: 60000 }, async () => {
    console.log('\n⚠️  Position at least one tag in front of the reader');

    const tags = await scanWith({
      captureAllTagData: true,
      tidWords: 6,
      userOffset: 0,
      userWords: 4
    });

    const epcs = [...new Set(tags.map(t => t.epc))];
    console.log(`    Tags read: ${epcs.length}`);
    for (const tag of tags.slice(0, 5)) {
      console.log(`    EPC=${tag.epc} PC=0x${(tag.pc ?? 0).toString(16)} TID=${tag.tid ?? '(none)'} USER=${tag.userData ?? '(none)'}`);
    }

    // Fail loudly on an empty field. Every assertion below is vacuously true
    // when nothing was read, so this is the one that keeps the rest honest.
    expect(tags.length, 'no tags were read at all — this run proved nothing').toBeGreaterThan(0);

    const withTid = tags.filter(t => t.tid && t.tid.length > 0);
    console.log(`    Tags carrying TID: ${withTid.length}/${tags.length}`);

    expect(
      withTid.length,
      'capture was on but no read returned any TID — the registers did not reach the radio'
    ).toBeGreaterThan(0);

    // 6 words requested is 12 bytes is 24 hex characters. A shorter answer
    // means the chip carries less TID than asked for, which is a real result
    // worth seeing rather than an assertion failure.
    const tidLengths = [...new Set(withTid.map(t => t.tid!.length))];
    console.log(`    TID hex lengths returned: ${tidLengths.join(', ')} (24 == the 6 words requested)`);

    // Gen2 TID starts with an allocation class identifier: 0xE2 for EPCglobal,
    // 0xE0 for ISO/IEC 7816-6. Anything else means the slice is misaligned and
    // we are reading something that is not TID.
    for (const tag of withTid) {
      expect(
        tag.tid!.slice(0, 2),
        `TID ${tag.tid} does not start with a Gen2 allocation class — the slice is misaligned`
      ).toMatch(/^E[02]$/);
    }
  });

  it('answers whether a refused bank read costs us the tag', { timeout: 60000 }, async () => {
    // Ask for 200 words of USER memory. Very few chips carry 400 bytes there,
    // so this should be refused by the tag.
    //
    // Pre-registered, so the result cannot be reinterpreted afterwards:
    //   tags still arrive  -> a refused read degrades to "no bank data", and
    //                         capture is safe to leave on
    //   tags vanish        -> capture is only usable once the chip is known,
    //                         and the UI must say so
    console.log('\n🧪 Requesting an over-long USER read to force a refusal');

    const tags = await scanWith({
      captureAllTagData: true,
      tidWords: 6,
      userOffset: 0,
      userWords: 200
    });

    const epcs = [...new Set(tags.map(t => t.epc))];
    const withUser = tags.filter(t => t.userData && t.userData.length > 0);

    console.log(`    Tags read with an over-long USER request: ${epcs.length}`);
    console.log(`    Tags carrying USER data: ${withUser.length}/${tags.length}`);
    console.log(
      tags.length > 0
        ? '    ANSWER: a refused bank read still yields the tag'
        : '    ANSWER: a refused bank read costs the tag entirely'
    );

    // Deliberately not asserted either way — this test exists to record the
    // answer, and both outcomes are legitimate findings about the hardware.
    // The assertion is only that the reader survived being asked.
    expect(harness.getReaderMode()).toBe(ReaderMode.INVENTORY);
  });

  it('needs a real mode change to pick up a capture toggle', { timeout: 90000 }, async () => {
    // Pins the reason DeviceManager.reapplyModeForCapture bounces through IDLE.
    //
    // The capture registers are written by the mode-entry sequence, and
    // Reader.setMode early-exits when the requested mode already equals its
    // target. So turning capture on while already in INVENTORY and re-requesting
    // INVENTORY writes nothing at all. This asserts that directly, because the
    // alternative is trusting a code reading — and a no-op fix here looks
    // exactly like a working one from the outside.

    // Start clean, capture off, in INVENTORY.
    await harness.setSettings({ rfid: { transmitPower: 30, targetEPC: '', captureAllTagData: false } });
    await harness.waitForEvent(WorkerEventType.SETTINGS_UPDATED);
    await harness.setMode(ReaderMode.IDLE);
    const enter = harness.setMode(ReaderMode.INVENTORY);
    await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      e => e.payload.readerState === ReaderState.CONNECTED);
    await enter;

    // Turn capture on and re-request the SAME mode — the early-exit path.
    await harness.setSettings({
      rfid: { transmitPower: 30, targetEPC: '', captureAllTagData: true, tidWords: 6, userWords: 0 }
    });
    await harness.waitForEvent(WorkerEventType.SETTINGS_UPDATED);
    await harness.setMode(ReaderMode.INVENTORY);

    harness.clearEvents();
    await harness.simulateTriggerPress();
    await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      e => e.payload.pressed === true, 8000);
    await new Promise(r => setTimeout(r, 2000));
    await harness.simulateTriggerRelease();
    await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      e => e.payload.pressed === false, 8000);

    const withoutBounce = (harness.getEventsByType(WorkerEventType.TAG_READ)
      .flatMap(e => (e.payload?.tags || [])) as ScannedTag[]).filter(t => t.tid);
    console.log(`    Re-requesting the same mode: ${withoutBounce.length} tags carried TID`);

    // Now bounce through IDLE, exactly as reapplyModeForCapture does.
    const afterBounce = await scanWith({
      captureAllTagData: true, tidWords: 6, userOffset: 0, userWords: 0
    });
    const withBounce = afterBounce.filter(t => t.tid);
    console.log(`    After bouncing through IDLE: ${withBounce.length} tags carried TID`);

    expect(
      withBounce.length,
      'the IDLE bounce must actually deliver bank data, or this test proves nothing'
    ).toBeGreaterThan(0);

    expect(
      withoutBounce.length,
      'setMode early-exits on an unchanged mode, so re-requesting it must write no capture registers — ' +
      'if this ever returns data, the bounce in reapplyModeForCapture is no longer needed'
    ).toBe(0);
  });

  it('still reads tags with capture off, unchanged', { timeout: 60000 }, async () => {
    // The other half of "off means byte-for-byte unchanged": compact mode must
    // still work after the capture path has been exercised, and must carry no
    // bank data.
    const tags = await scanWith({ captureAllTagData: false });

    console.log(`    Tags read with capture off: ${[...new Set(tags.map(t => t.epc))].length}`);

    expect(tags.length, 'no tags were read at all — this run proved nothing').toBeGreaterThan(0);
    expect(tags.every(t => !t.tid)).toBe(true);
    expect(tags.every(t => !t.userData)).toBe(true);
  });
});
