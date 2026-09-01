import { describe, it, expect } from 'vitest';
import { readFileSync, existsSync } from 'fs';
import os from 'os';
import path from 'path';
// @ts-expect-error — .mjs instrument module, no types by design
import { formatRepLine, formatProgressBlock } from '../../scripts/arm-progress.mjs';

/**
 * Making a 7-hour arm readable while it runs (TRA-1240).
 *
 * ⚠ The ticket's premise was WRONG and is corrected here so nobody re-derives it:
 * the driver already printed one line per rep, and vitest output already went to
 * per-rep files rather than the driver log. The real 2026-09-01 driver log is
 * 203 lines for a 200-rep arm. What was missing is the *why* on that line, and
 * any aggregate at all.
 *
 * These formatters are pure functions of the record, so the same code renders a
 * live rep and an archived one. That is what makes the five arms already on disk
 * readable, and it is why they are unit-testable at all.
 */

const rec = (over: Record<string, unknown> = {}) => ({
  rep: 1,
  durationMs: 134_000,
  exitCode: 0,
  files: [],
  reportMissing: false,
  signals: {
    logMissing: false,
    ackSamples: 302,
    errorNotifications: 0,
    commandTimeouts: {},
    commandRejections: {}
  },
  ...over
});

const failed = (...names: string[]) =>
  names.map((name) => ({ name, status: 'failed', failed: ['x'] }));

describe('formatRepLine', () => {
  it('renders a clean rep without a spec list', () => {
    const line = formatRepLine(rec(), 200);
    // Rep numbers are right-padded to the width of the total so the columns line
    // up down 200 lines — the whole reason this is scannable. Match loosely
    // rather than pinning the padding, which is presentation.
    expect(line).toMatch(/rep\s+1\/200/);
    expect(line).toContain('134s');
    expect(line).toContain('pass');
    expect(line).not.toMatch(/spec/);
  });

  it('names the failing specs by BASENAME, not full path', () => {
    // A wedge rep fails five specs. Printed as full paths that is one line of
    // ~350 characters — the reps that most need reading become the least
    // readable, which is how the 2026-09-01 wedge looked in the driver log.
    const line = formatRepLine(
      rec({
        exitCode: 1,
        files: failed(
          'tests/integration/cs108/barcode.spec.ts',
          'tests/integration/cs108/locate-mask-length-variants.spec.ts'
        )
      }),
      200
    );
    expect(line).toContain('FAIL');
    expect(line).toContain('barcode');
    expect(line).toContain('locate-mask-length-variants');
    expect(line).not.toContain('tests/integration');
  });

  it('carries the signals that discriminate, so a failure says why', () => {
    const line = formatRepLine(
      rec({
        exitCode: 1,
        signals: {
          logMissing: false,
          ackSamples: 53,
          errorNotifications: 15,
          commandTimeouts: { RFID_FIRMWARE_COMMAND: 1, RFID_POWER_OFF: 2 },
          commandRejections: { RFID_POWER_OFF: 24, GET_TRIGGER_STATE: 12 }
        }
      }),
      200
    );
    expect(line).toContain('rej=36');   // summed across ops
    expect(line).toContain('to=3');
    expect(line).toContain('err=15');
    expect(line).toContain('ack=53');
  });

  /**
   * ⚠ THE NULL-VS-ZERO RULE, which the whole instrument turns on.
   *
   * A runner that cannot observe a signal records `null`. Rendering that as `0`
   * asserts a clean measurement where none was taken — the exact failure
   * `suite-run-signals.mjs` documents at length for `logMissing`. This test
   * fails if the formatter prints a zero.
   */
  it('renders an unobserved signal as unknown, never as zero', () => {
    const line = formatRepLine(
      rec({ signals: { logMissing: true, ackSamples: null, errorNotifications: null, commandTimeouts: null, commandRejections: null } }),
      200
    );
    expect(line).toContain('rej=?');
    expect(line).toContain('to=?');
    expect(line).toContain('err=?');
    expect(line).toContain('ack=?');
    expect(line).not.toMatch(/rej=0|to=0|err=0|ack=0/);
  });
});

describe('formatProgressBlock', () => {
  const many = (outcomes: number[]) =>
    outcomes.map((exitCode, i) => rec({ rep: i + 1, exitCode, files: exitCode ? failed('tests/integration/cs108/locate-mask-length-variants.spec.ts') : [] }));

  it('counts passes, failures and the rate', () => {
    const block = formatProgressBlock(many([0, 0, 0, 1, 0]), 200, Date.now() - 600_000);
    expect(block).toContain('passed 4');
    expect(block).toContain('failed 1');
    expect(block).toContain('20.0%');
  });

  it('tallies failing specs by basename', () => {
    const block = formatProgressBlock(many([1, 1, 0]), 200, Date.now() - 600_000);
    expect(block).toContain('locate-mask-length-variants 2');
  });

  /**
   * THE POINT OF THE BLOCK. A wedge is a RUN of consecutive failures, and totals
   * cannot show one: seven scattered failures and seven consecutive are the same
   * number and completely different arms. The strip is what makes the 2026-09-01
   * wedge at reps 137-143 visible without opening anything.
   */
  it('distinguishes scattered failures from a consecutive run', () => {
    const scattered = formatProgressBlock(many([1, 0, 1, 0, 1, 0, 1, 0, 1, 0]), 200, Date.now() - 600_000);
    const wedged = formatProgressBlock(many([0, 0, 0, 0, 0, 1, 1, 1, 1, 1]), 200, Date.now() - 600_000);
    expect(scattered).not.toBe(wedged);
    // Same totals, so only the strip can tell them apart.
    expect(scattered).toContain('failed 5');
    expect(wedged).toContain('failed 5');
    expect(wedged).toMatch(/X{5}/);
  });

  it('derives ETA from observed durations rather than a constant', () => {
    const fast = formatProgressBlock(many([0, 0]).map((r) => ({ ...r, durationMs: 10_000 })), 200, Date.now() - 20_000);
    const slow = formatProgressBlock(many([0, 0]).map((r) => ({ ...r, durationMs: 200_000 })), 200, Date.now() - 400_000);
    expect(fast).not.toBe(slow);
  });

  it('does not throw or divide by zero on an empty run', () => {
    expect(() => formatProgressBlock([], 200, Date.now())).not.toThrow();
  });
});

/**
 * The archive is the real input shape, and a fixture cannot stand in for it —
 * that is the lesson from the ring parser, whose hand-built fixture used
 * different JSON whitespace from the producer and hid a filter that matched
 * nothing.
 */
describe('against the real 2026-09-01 arm', () => {
  const archive = path.join(os.homedir(), 'soak-archives/2026-09-01-tra1229-1230-1231-verification-arm/runs.jsonl');
  const available = existsSync(archive);

  it.skipIf(!available)('renders all 200 archived records without throwing', () => {
    const records = readFileSync(archive, 'utf8').trim().split('\n').map((l) => JSON.parse(l));
    expect(records).toHaveLength(200);
    for (const record of records) expect(() => formatRepLine(record, 200)).not.toThrow();

    // The wedge must be visible as a run in the strip.
    const block = formatProgressBlock(records.slice(130, 145), 200, Date.now() - 3600_000);
    expect(block).toMatch(/X{6,}/);
  });
});
