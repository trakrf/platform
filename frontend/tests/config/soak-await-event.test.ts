import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import {
  progressBlocks,
  signalTotal,
  detectEvent,
  driverGoneReport,
} from '../../scripts/await-soak-event.mjs';

/**
 * The watcher's filter coverage, which is the whole correctness question.
 *
 * The contract is that this thing TERMINATES: it blocks until an arm produces
 * news, prints one event, and exits — and its exit is what re-invokes whoever
 * launched it. A chain of one-shots, not a stream.
 *
 * That makes an event it cannot see indistinguishable from a healthy quiet arm.
 * A watcher that greps only the happy path hangs through a crash, the chain
 * stops re-arming, and nobody hears anything for the remaining seven hours —
 * exactly the silence-is-not-success rule the abort criteria already follow. So
 * every terminal condition is tested here, and `detectEvent` is pure so that
 * they can be.
 *
 * Refs: TRA-1242.
 */

const BLOCK_1 = [
  '--- 10/200 · elapsed 22m · eta 7h01m --------------------',
  '  passed 10  failed 0  (0.0%)',
  '  failing specs: none',
  '  signals: rejections 0  timeouts 4  errNotif 0',
  '  last 10: ..........',
].join('\n');

const BLOCK_2 = [
  '--- 20/200 · elapsed 44m · eta 6h38m --------------------',
  '  passed 19  failed 1  (5.0%)',
  '  failing specs: locate 1',
  '  signals: rejections 0  timeouts 9  errNotif 0',
  '  last 20: ...........X........',
].join('\n');

const snapshot = (over: Record<string, unknown> = {}) => ({
  driverLog: '',
  watchdogLog: '',
  rows: [],
  alive: true,
  ...over,
});

describe('progressBlocks', () => {
  it('takes a block from its --- header through its last-N strip', () => {
    const log = `[suite-runs] rep 9/200\n${BLOCK_1}\n[suite-runs] rep 11/200\n`;

    expect(progressBlocks(log)).toEqual([BLOCK_1]);
  });

  it('separates consecutive blocks', () => {
    const log = `${BLOCK_1}\n[suite-runs] rep 11/200\n${BLOCK_2}\n`;

    expect(progressBlocks(log)).toEqual([BLOCK_1, BLOCK_2]);
  });

  it('is empty before the first block', () => {
    expect(progressBlocks('[suite-runs] runner=vitest shape=fixed reps=200\n')).toEqual([]);
  });
});

describe('signalTotal', () => {
  it('sums a scalar needle across rows', () => {
    const rows = [{ signals: { commandInFlight: 1 } }, { signals: { commandInFlight: 2 } }];

    expect(signalTotal(rows, 'commandInFlight')).toBe(3);
  });

  it('sums a per-op table', () => {
    const rows = [{ signals: { commandTimeouts: { '0x8002': 3, '0xA001': 12 } } }];

    expect(signalTotal(rows, 'commandTimeouts')).toBe(15);
  });

  it('skips a row that could not observe the needle rather than counting it as zero', () => {
    // null means "this runner cannot see this at all" — the null-vs-zero rule
    // the whole instrument turns on. Counting it as 0 would make a running
    // total that never moves look like a clean arm.
    const rows = [{ signals: { commandInFlight: null } }, { signals: { commandInFlight: 4 } }];

    expect(signalTotal(rows, 'commandInFlight')).toBe(4);
  });
});

describe('detectEvent', () => {
  it('returns nothing when nothing has changed — the watcher keeps blocking', () => {
    const s = snapshot({ driverLog: BLOCK_1 });

    expect(detectEvent(s, s, {})).toBeNull();
  });

  it('fires on a new progress block', () => {
    const before = snapshot({ driverLog: BLOCK_1 });
    const after = snapshot({ driverLog: `${BLOCK_1}\n${BLOCK_2}` });

    expect(detectEvent(before, after, {})).toMatchObject({ kind: 'progress' });
    expect(detectEvent(before, after, {})!.text).toContain('20/200');
  });

  it('fires on a new watchdog line', () => {
    const before = snapshot({ watchdogLog: '[..] pre-flight clear.\n' });
    const after = snapshot({ watchdogLog: '[..] pre-flight clear.\n[..] ABORT: void capture\n' });

    const event = detectEvent(before, after, {})!;
    expect(event.kind).toBe('watchdog');
    expect(event.text).toContain('ABORT');
    expect(event.text).not.toContain('pre-flight clear');
  });

  it('prefers the watchdog line over the driver being gone', () => {
    // Both are true on an aborted arm, and the watchdog line is the one that
    // says WHY. The chain re-arms, so the driver-gone event still lands next.
    const before = snapshot({ watchdogLog: 'a\n' });
    const after = snapshot({ watchdogLog: 'a\nABORT: the bridge did not answer\n', alive: false });

    expect(detectEvent(before, after, {})).toMatchObject({ kind: 'watchdog' });
  });

  it('fires when the driver is gone', () => {
    const before = snapshot();
    const after = snapshot({ alive: false, rows: [{ exitCode: 0 }, { exitCode: 1 }] });

    const event = detectEvent(before, after, {})!;
    expect(event.kind).toBe('driver-gone');
    expect(event.terminal).toBe(true);
  });

  it('fires when the driver is gone even on the very first observation', () => {
    // The arm may already have ended before the watcher was armed — after a
    // REPL crash, most obviously. Exiting immediately is what makes recovery
    // one command rather than a hang that looks like a healthy quiet arm.
    const gone = snapshot({ alive: false });

    expect(detectEvent(gone, gone, {})).toMatchObject({ kind: 'driver-gone' });
  });

  it('fires when the pre-registered signal moves', () => {
    const before = snapshot({ rows: [{ signals: { commandInFlight: 0 } }] });
    const after = snapshot({
      rows: [{ signals: { commandInFlight: 0 } }, { signals: { commandInFlight: 2 } }],
    });

    const event = detectEvent(before, after, { signal: 'commandInFlight' })!;
    expect(event.kind).toBe('signal');
    expect(event.text).toContain('commandInFlight');
    expect(event.text).toContain('2');
  });

  it('ignores the signal when none was pre-registered', () => {
    const before = snapshot({ rows: [{ signals: { commandInFlight: 0 } }] });
    const after = snapshot({ rows: [{ signals: { commandInFlight: 9 } }] });

    expect(detectEvent(before, after, {})).toBeNull();
  });
});

describe('driverGoneReport', () => {
  it('carries the final counts and the archive reminder', () => {
    // §6 has a deadline attached and the previous arm skipped it. The one
    // moment the operator is guaranteed to be reading is the moment the arm
    // ends, so the reminder rides the event that says so.
    const rows = [{ exitCode: 0 }, { exitCode: 1 }, { exitCode: 0 }];

    const text = driverGoneReport(rows);
    expect(text).toContain('3');
    expect(text).toContain('§6');
    expect(text).toContain('dump-bridge-ring.mjs');
  });
});
