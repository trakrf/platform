import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { useLocateStore, STALE_THRESHOLD_MS, DEFAULT_RSSI } from './locateStore';

/**
 * TRA-1123 / TRA-1089: Current / Average / Peak / Update Rate are recomputed
 * only inside addRssiReading(), so when reads stop they freeze on the last
 * value forever. The gauge and the Status row already decay, because both read
 * getFilteredRSSI(), which floors to DEFAULT_RSSI once the last read is stale.
 *
 * On a tag finder a frozen number from a finished search is a wrong answer, not
 * an old one: the operator reads "the tag is right here" off a search that has
 * returned nothing. All four statistics must follow the same staleness signal
 * the gauge already follows.
 */
describe('locateStore statistics staleness (TRA-1123 / TRA-1089)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    useLocateStore.getState().clearBuffer();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('reports the live reading while reads are arriving', () => {
    useLocateStore.getState().addRssiReading(-35);

    const stats = useLocateStore.getState().getStatistics();

    expect(stats.currentRSSI).toBe(-35);
    expect(stats.averageRSSI).toBe(-35);
    expect(stats.peakRSSI).toBe(-35);
    expect(stats.updateRate).toBeGreaterThan(0);
  });

  it('decays every statistic to "no signal" once reads stop', () => {
    useLocateStore.getState().addRssiReading(-35);

    vi.advanceTimersByTime(STALE_THRESHOLD_MS + 1);

    expect(useLocateStore.getState().getStatistics()).toEqual({
      currentRSSI: DEFAULT_RSSI,
      averageRSSI: DEFAULT_RSSI,
      peakRSSI: DEFAULT_RSSI,
      updateRate: 0
    });
  });

  it('agrees with getFilteredRSSI about when the signal is gone', () => {
    useLocateStore.getState().addRssiReading(-35);

    vi.advanceTimersByTime(STALE_THRESHOLD_MS + 1);

    expect(useLocateStore.getState().getFilteredRSSI()).toBe(DEFAULT_RSSI);
    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(DEFAULT_RSSI);
  });

  it('reports no signal from an empty buffer', () => {
    const stats = useLocateStore.getState().getStatistics();

    expect(stats.currentRSSI).toBe(DEFAULT_RSSI);
    expect(stats.updateRate).toBe(0);
  });

  it('recovers as soon as a fresh read arrives for the new search', () => {
    useLocateStore.getState().addRssiReading(-35);
    vi.advanceTimersByTime(STALE_THRESHOLD_MS + 1);
    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(DEFAULT_RSSI);

    useLocateStore.getState().addRssiReading(-70);

    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(-70);
  });
});

/**
 * TRA-1123: the ring buffer outlives the target that produced it. Retarget to
 * a different EPC and the screen keeps rendering the previous tag's readings,
 * so a search for something that is not there looks like a search that is
 * working. A reading only means anything about the target it was read for.
 */
describe('locateStore target tracking (TRA-1123)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    useLocateStore.getState().clearBuffer();
    useLocateStore.getState().setTarget('');
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('drops readings collected for a different target', () => {
    useLocateStore.getState().setTarget('E280689400000000001018DD');
    useLocateStore.getState().addRssiReading(-35);
    expect(useLocateStore.getState().rssiBuffer).toHaveLength(1);

    useLocateStore.getState().setTarget('E280689400000000001018EE');

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(0);
    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(DEFAULT_RSSI);
  });

  it('keeps readings when the target is re-asserted unchanged', () => {
    // The screen re-syncs the target on every mount and on every settings
    // change; re-asserting the same EPC must not wipe a live search.
    useLocateStore.getState().setTarget('E280689400000000001018DD');
    useLocateStore.getState().addRssiReading(-35);

    useLocateStore.getState().setTarget('E280689400000000001018DD');

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(1);
    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(-35);
  });

  it('records the target so the next change can be detected', () => {
    useLocateStore.getState().setTarget('E280689400000000001018DD');

    expect(useLocateStore.getState().targetEPC).toBe('E280689400000000001018DD');
  });
});

/**
 * Measured on the TRA-1120 turntable bench: both pre- and post-fix code
 * occasionally admit a read from a tag that was never the target — 2 strays in
 * 6,119 reads (0.033%) on shipped main, consistent with a tag at the edge of
 * sensitivity mis-decoding the Gen2 Select and asserting SL when it should not.
 *
 * One stray in a few thousand never dominates a search, but the gauge does not
 * need it to: a single sample is the difference between an honest "no signal"
 * and a brief plausible reading, which is this ticket's symptom reached by a
 * different route than the stale buffer.
 *
 * handler.ts has always claimed "the application layer (locateStore) will
 * filter for the target EPC". It never did — addRssiReading took no EPC at all,
 * so the hardware Select was the only line of defence and it is demonstrably
 * imperfect. The EPC is already on the LOCATE_UPDATE payload; this makes the
 * comment true.
 */
describe('locateStore rejects reads from other tags (stray admissions)', () => {
  const TARGET = '00000000000000000000533034313633';
  const OTHER  = '00000000000000000000533034313634';

  beforeEach(() => {
    vi.useFakeTimers();
    useLocateStore.getState().clearBuffer();
    useLocateStore.getState().setTarget('');
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('admits a read for the current target', () => {
    useLocateStore.getState().setTarget(TARGET);

    useLocateStore.getState().addRssiReading(-35, undefined, undefined, undefined, TARGET);

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(1);
    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(-35);
  });

  it('drops a stray read from a different tag', () => {
    useLocateStore.getState().setTarget(TARGET);

    useLocateStore.getState().addRssiReading(-35, undefined, undefined, undefined, OTHER);

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(0);
    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(DEFAULT_RSSI);
  });

  it('matches a leading-zero-stripped target against a full-width read', () => {
    // The operator types 533034313633; the tag reports the full 128 bits.
    // TRA-1108/TRA-1120 make the *mask* match both widths — this filter must
    // use the same equivalence or it would drop every legitimate read for a
    // stripped target and re-break locate for 128-bit EPCs.
    useLocateStore.getState().setTarget('533034313633');

    useLocateStore.getState().addRssiReading(-35, undefined, undefined, undefined, TARGET);

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(1);
  });

  it('matches case-insensitively', () => {
    useLocateStore.getState().setTarget(TARGET.toLowerCase());

    useLocateStore.getState().addRssiReading(-35, undefined, undefined, undefined, TARGET);

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(1);
  });

  it('admits everything when no target is set', () => {
    useLocateStore.getState().addRssiReading(-35, undefined, undefined, undefined, OTHER);

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(1);
  });

  it('admits a read whose source did not say which tag it came from', () => {
    // No EPC on the reading means no basis to reject it; dropping it would
    // silently break any caller that has not been updated.
    useLocateStore.getState().setTarget(TARGET);

    useLocateStore.getState().addRssiReading(-35);

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(1);
  });

  it('drops the in-flight read that arrives just after a retarget', () => {
    // The residual window from the mid-search retarget fix: the old mask is
    // still on the hardware for ~80ms, so one read for the previous tag lands
    // under the new target. Staleness used to hide it 1.25s later; this drops
    // it outright.
    useLocateStore.getState().setTarget(TARGET);
    useLocateStore.getState().addRssiReading(-35, undefined, undefined, undefined, TARGET);

    useLocateStore.getState().setTarget(OTHER);
    useLocateStore.getState().addRssiReading(-35, undefined, undefined, undefined, TARGET);

    expect(useLocateStore.getState().rssiBuffer).toHaveLength(0);
    expect(useLocateStore.getState().getStatistics().currentRSSI).toBe(DEFAULT_RSSI);
  });
});
