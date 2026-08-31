/**
 * Locate mask length variants — a leading-zero-stripped EPC must find its tag
 * at BOTH 96 and 128 bits.
 *
 * PERMANENT REGRESSION COVERAGE, not in-flight ticket work. This guards the
 * overlay of 96-bit and 128-bit Select masks, which is ongoing product
 * behaviour. Originally written to verify TRA-1120 (shipped); the ticket
 * reference is kept here for provenance and deliberately kept OUT of the
 * filename and describe block, so a failure reads as "mask-length overlay is
 * broken" rather than as a regression in closed work (TRA-1167).
 *
 * This is the part CI cannot reach. The fix ORs two Select descriptors by
 * giving the second one sel_action 001 (assert SL on match, do nothing on a
 * miss) while the first keeps the vendor default 000 (assert on match,
 * deassert on a miss) and runs first. That reasoning came off the Gen2 spec,
 * not off a reader, and it rests on three things only hardware can settle:
 *
 *   1. two enabled descriptors issue two Selects, in ascending index order
 *   2. sel_action 001 accumulates rather than replacing the previous verdict
 *   3. a 128-bit mask against a 96-bit tag reads as a MISS, not a match
 *
 * The two cases below are chosen so that each one fails if any of those is
 * wrong, given the bench population:
 *
 *   '533034313633' (stripped 128-bit tag)
 *     descriptor 0 masks 000000000000533034313633 -> matches NOTHING present
 *     descriptor 1 masks the full 32-char EPC     -> matches the tag
 *   Finding it therefore proves the OR: descriptor 0's miss must not cancel
 *   descriptor 1's hit. Under the old single-descriptor code this found
 *   nothing at all, which is the bug.
 *
 *   '10020' (stripped 96-bit tag)
 *     descriptor 0 masks 000000000000000000010020 -> matches the tag
 *     descriptor 1 masks it padded to 32 chars    -> matches NOTHING present
 *   Finding it proves the mirror image: descriptor 1's miss must not cancel
 *   descriptor 0's hit, and a 128-bit mask must not spuriously match a 96-bit
 *   tag.
 *
 * Run with the bench tags in front of the reader:
 *   pnpm exec vitest run tests/integration/cs108/locate-mask-length-variants.spec.ts
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { CS108WorkerTestHarness } from './CS108WorkerTestHarness';
import { ReaderMode, ReaderState } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';
import {
  LOCATE_TEST_TAG,
  LOCATE_TEST_TAG_128,
  LOCATE_TEST_TAG_128_STRIPPED,
  LOCATE_DECOY_TAG_128,
  LOCATE_DECOY_TAG_128_STRIPPED,
  EPC_FORMATS,
} from '@test-utils/constants';

// The WALDO probes. ASCII 'S04163' / 'S04164' — identical through hex char 24,
// differing only in the final char, inside the tail the old 96-bit mask never
// covered. Both are on the bench, which is what makes the decoy test real.
const TAG_128 = LOCATE_TEST_TAG_128;
const TAG_128_STRIPPED = LOCATE_TEST_TAG_128_STRIPPED;
const DECOY_128 = LOCATE_DECOY_TAG_128;
const DECOY_128_STRIPPED = LOCATE_DECOY_TAG_128_STRIPPED;

// One of the 1001x bench tags, in 96-bit form.
const TAG_96 = EPC_FORMATS.toFullEPC(LOCATE_TEST_TAG);
const TAG_96_STRIPPED = LOCATE_TEST_TAG;

const SCAN_MS = 4000;

/**
 * Reads within one scan below which an EPC is noise, not a selection.
 *
 * RFID is a lossy, fragmented stream — at mtu=23 every CS108 frame over 20
 * bytes is reassembled, and `parser.ts` deliberately byte-slides to resync on
 * an unrecognised header rather than stall. Recovering sync beats stalling, so
 * an occasional phantom record is normal and correct behaviour, not a defect.
 * The frame CRC does not cover it: the bytes are right, the walk through them
 * is not.
 *
 * So judge the STREAM, never a single read. A tag the Select genuinely admits
 * answers steadily for the whole scan; a phantom appears once. MEASURED over
 * SCAN_MS on 2026-08-23/24: genuine matches x32-x63, phantom x1. The floor sits
 * at 5 — 6x below the weakest genuine read observed, 5x above the phantom.
 *
 * Those are the numbers to overturn if you move it. Do not "tune" it: the point
 * is the separation, not the value, and anything from ~5 to ~30 splits the two
 * populations. Widening it does not buy accuracy, it buys the failure below.
 *
 * THE RISK IS ASYMMETRIC, WHICH IS WHY REJECTIONS ARE LOGGED. A phantom that
 * slips under a too-low floor produces a LOUD wrong answer. A genuine detection
 * floored by a too-high one produces "nothing found" — which, for the
 * `toEqual([])` assertions here, is a silent PASS. That direction is invisible,
 * and it is not hypothetical: the WALDO probes read x329 vs x66 on the same
 * board in the same run, so coupling varies ~5x with position alone. A weakly
 * coupled tag is exactly the case that could read low.
 *
 * So every discarded EPC is printed with its count. A floor that ate a real
 * detection stays observable in the log rather than vanishing into a pass —
 * otherwise this fix repeats the shape of the bug it fixes, discarding the
 * content at judgement, just on the other side of the threshold.
 *
 * Observed 2026-08-23: one x1 phantom failed `toEqual([])` and read as a mask
 * regression on the guard protecting the 128-bit EPC fix — where ~20% of field
 * tags live. Locate itself averages such a read away and no operator could
 * perceive it, so the spec was strictly more sensitive than the product it
 * guards.
 */
const MIN_READS_FOR_SELECTION = 5;

describe('Locate mask length variants — stripped EPC finds its tag at either width', () => {
  let harness: CS108WorkerTestHarness;

  beforeAll(async () => {
    harness = new CS108WorkerTestHarness();
    await harness.initialize();
    expect(await harness.connect()).toBe(true);

    const setModePromise = harness.setMode(ReaderMode.LOCATE, {
      rfid: { transmitPower: 30, targetEPC: TAG_96_STRIPPED }
    });
    await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.CONNECTED);
    await setModePromise;

    // Let the radio settle before the first search.
    await new Promise(resolve => setTimeout(resolve, 2000));
  }, 60000);

  afterAll(async () => {
    if (harness) {
      try {
        // INVENTORY then IDLE, so the IDLE sequence actually runs and powers
        // the radio down rather than being skipped as a no-op mode change.
        await harness.setMode(ReaderMode.INVENTORY);
        await harness.setMode(ReaderMode.IDLE);
      } catch {
        try { await harness.setMode(ReaderMode.IDLE); } catch { /* best effort */ }
      }
      // `cleanup()` in `finally`: it is the only thing that releases the link
      // for the next spec file, and the worker's own disconnect in front of it
      // can throw. See the note in locate.spec.ts. TRA-1217.
      try {
        await harness.disconnect();
      } finally {
        await harness.cleanup();
      }
    }
  }, 60000);

  /**
   * Point Locate at `targetEPC` and return the distinct EPCs that answered.
   *
   * Answering at all is the signal: LOCATE mode runs with tag_sel enabled, so
   * only tags a Select asserted SL on reply. What comes back is therefore the
   * Select's verdict, read off the air.
   */
  const locate = async (targetEPC: string): Promise<string[]> => {
    // Push the same SHAPE the app pushes. DeviceManager's settings
    // subscription sends the whole `rfid` slice of the store, so transmitPower
    // always rides along, and this mirrors that deliberately — fidelity to the
    // app, which is the only reason it is worth anything as a guard.
    //
    // It used to ride along because it HAD to: `hasHardwareSettings` gated the
    // whole apply block and did not list targetEPC, so a targetEPC-only push
    // was ignored at every level. That is fixed (TRA-1225) and a bare targetEPC
    // now applies, so the shape here is a choice rather than a workaround.
    // Keep it anyway. The point of an integration spec is to push what the app
    // pushes; narrowing it to the minimum that works would stop it covering
    // what ships.
    await harness.setSettings({ rfid: { transmitPower: 30, targetEPC } });
    await harness.waitForEvent(WorkerEventType.SETTINGS_UPDATED);

    harness.clearEvents();

    await harness.simulateTriggerPress();
    await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === true, 8000);

    await new Promise(resolve => setTimeout(resolve, SCAN_MS));

    await harness.simulateTriggerRelease();
    await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === false, 8000);

    const updates = harness.getEventsByType(WorkerEventType.LOCATE_UPDATE);

    // Count per EPC, not just distinct. A tag the Select genuinely admitted
    // answers steadily across the whole scan; a single stray read is a
    // different thing entirely, and a bare distinct-set hides the difference.
    const counts = new Map<string, number>();
    for (const update of updates as any[]) {
      counts.set(update.payload.epc, (counts.get(update.payload.epc) ?? 0) + 1);
    }
    const ranked = [...counts.keys()].sort((a, b) => counts.get(b)! - counts.get(a)!);

    console.log(`\n  locate('${targetEPC}') -> ${updates.length} updates, ${ranked.length} distinct EPC(s):`);
    for (const epc of ranked) {
      const n = counts.get(epc)!;
      console.log(`      ${epc}  x${n}${n < MIN_READS_FOR_SELECTION ? '   (below floor — not a selection)' : ''}`);
    }

    // Apply the floor the comment above describes. Returning the bare distinct
    // set threw that discrimination away and made every assertion here strictly
    // more sensitive than the product: `toEqual([])` failed on a single stray
    // read exactly as it would on a tag answering sixty times.
    return ranked.filter(epc => counts.get(epc)! >= MIN_READS_FOR_SELECTION);
  };

  it('finds a 96-bit tag from its stripped value', { timeout: 60000 }, async () => {
    // Descriptor 1's 128-bit mask matches nothing here, so this only passes if
    // its miss leaves descriptor 0's assert alone — and if a 128-bit mask does
    // not spuriously match a 96-bit tag.
    const found = await locate(TAG_96_STRIPPED);

    expect(found).toContain(TAG_96);
  });

  it('finds the 128-bit tag from its stripped value — the bug this fixes', { timeout: 60000 }, async () => {
    // The case that failed silently before. Descriptor 0's 96-bit reading
    // matches nothing on this bench, so the tag can only answer via
    // descriptor 1, and only if the two Selects accumulate as OR.
    const found = await locate(TAG_128_STRIPPED);

    expect(found).toContain(TAG_128);
  });

  it('does not turn Locate into an unfiltered scan', { timeout: 60000 }, async () => {
    // 26 tags are in front of the reader. If the Select were being cleared
    // wrongly — or SL left asserted from the previous search — Locate would
    // answer for tags it was not asked about, and the operator would be
    // walked toward the wrong one. This is the false-positive guard.
    const found = await locate(TAG_128_STRIPPED);

    expect(found).toEqual([TAG_128]);
  });

  it('tells the two WALDO probes apart from their stripped values', { timeout: 60000 }, async () => {
    // Both tags are in front of the reader and share every bit but the last,
    // which lives in the 32-bit tail. This is the TRA-1108 discrimination
    // test, now run through the stripped/ambiguous path: the OR must not
    // widen either search into the other's tag.
    expect(await locate(TAG_128_STRIPPED)).toEqual([TAG_128]);
    expect(await locate(DECOY_128_STRIPPED)).toEqual([DECOY_128]);
  });

  it('still finds the full-width EPC, and only it (TRA-1108 unregressed)', { timeout: 60000 }, async () => {
    // Over 24 chars takes the single-descriptor path, which also has to
    // DISABLE descriptor 1 — otherwise the stale 128-bit mask the previous
    // case left there would keep issuing its Select. Here that mask happens to
    // match the same tag, so the sharper check is the 96-bit case below.
    const found = await locate(TAG_128);

    expect(found).toEqual([TAG_128]);
  });

  it('does not carry a stale descriptor from an ambiguous search into a later one', { timeout: 60000 }, async () => {
    // Ambiguous search first, which enables descriptor 1 and leaves the WALDO
    // tag's 128-bit mask in it.
    expect(await locate(TAG_128_STRIPPED)).toContain(TAG_128);

    // Then an unambiguous search (>24 chars) for a value nothing on the bench
    // matches: the 96-bit tag 10020 padded out to 128 bits, which a 96-bit tag
    // cannot answer. The unambiguous path must DISABLE descriptor 1, so this
    // search should find nothing at all. If the stale descriptor were left
    // enabled, the WALDO tag would still be selected and answer here.
    const found = await locate(TAG_96_STRIPPED.padStart(32, '0'));

    expect(found).not.toContain(TAG_128);
    expect(found).toEqual([]);
  });
});
