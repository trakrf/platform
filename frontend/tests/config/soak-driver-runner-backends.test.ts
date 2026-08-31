import { describe, it, expect } from 'vitest';
import {
  SIGNALS,
  E2E_SIGNALS,
  signalsFor,
  runnerOf,
  readSignals,
  resolveSignals,
  captureCanaryCount,
} from '../../scripts/suite-run-signals.mjs';
import {
  buildRunnerArgs,
  readVitestReport,
  readPlaywrightReport,
  suiteRootFor,
  assertShapeSupported,
} from '../../scripts/characterise-suite-runs.mjs';
import { isVoidCapture } from '../../scripts/watch-soak-abort-criteria.mjs';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

/**
 * TRA-1206. The soak driver could only build vitest argv and parse vitest's JSON
 * reporter, so it could not run the suite TRA-1200 actually measures —
 * `tests/e2e/inventory.spec.ts`, which is Playwright.
 *
 * These tests exist for the two things that are easy to get wrong and impossible
 * to see afterwards:
 *
 *   1. THE VITEST PATH MUST NOT MOVE. TRA-1189's 528 reps and TRA-1193's 200-rep
 *      verification are the comparison baseline for this whole milestone, and a
 *      refactor that "improves" the vitest argv silently invalidates its own
 *      history. The argv below is FROZEN on purpose. If a change makes this test
 *      fail, the change is the problem, not the test.
 *
 *   2. A SIGNAL THAT CANNOT EXIST ON A PATH MUST NOT READ AS ZERO. `[Harness]`
 *      is emitted by `tests/integration/cs108/CS108WorkerTestHarness.ts` and by
 *      nothing else, so no Playwright rep can ever produce one. Recording 0
 *      would make the watchdog's void-capture check (`harnessLines === 0`) abort
 *      e2e rep 1 every single time — a correct check firing on the absence of
 *      its emitter rather than on a void capture.
 */

const scratch = mkdtempSync(path.join(tmpdir(), 'soak-runner-'));
function fixture(name: string, body: unknown): string {
  const p = path.join(scratch, name);
  writeFileSync(p, typeof body === 'string' ? body : JSON.stringify(body));
  return p;
}

describe('the vitest path is frozen', () => {
  /**
   * The exact argv TRA-1189 and TRA-1193 were measured under.
   *
   * Written out rather than derived, because a derived expectation moves with
   * the code it is checking. `--hookTimeout=90000` in particular is one of THREE
   * copies of that number (both package.json scripts and the driver) and is a
   * prerequisite for the bridge's connect backstop — see the derivation in
   * `characterise-suite-runs.mjs`.
   */
  const FROZEN = [
    'vitest',
    'run',
    'tests/integration/',
    '--no-file-parallelism',
    '--hookTimeout=90000',
  ];

  it('builds the baseline argv for shape=fixed', () => {
    const { args, seed } = buildRunnerArgs('vitest', 'fixed', 1, null);
    expect(args).toEqual(FROZEN);
    expect(seed).toBeNull();
  });

  it('appends shuffle flags without disturbing the baseline prefix', () => {
    const { args, seed } = buildRunnerArgs('vitest', 'shuffle', 7, null);
    expect(args.slice(0, FROZEN.length)).toEqual(FROZEN);
    expect(args.slice(FROZEN.length)).toEqual([
      '--sequence.shuffle.files',
      '--sequence.seed=11670007',
    ]);
    // Derived from rep so the order is reproducible from the record alone.
    expect(seed).toBe(11670007);
  });

  it('narrows to --target for any shape, not just alone', () => {
    const { args } = buildRunnerArgs('vitest', 'cold', 1, 'tests/integration/cs108/locate.spec.ts');
    expect(args).toEqual([
      'vitest',
      'run',
      'tests/integration/cs108/locate.spec.ts',
      '--no-file-parallelism',
      '--hookTimeout=90000',
    ]);
  });

  it('still roots at tests/integration/', () => {
    expect(suiteRootFor('vitest')).toBe('tests/integration/');
  });
});

describe('the playwright path', () => {
  it('roots at tests/e2e/', () => {
    expect(suiteRootFor('e2e')).toBe('tests/e2e/');
  });

  it('builds playwright argv against the e2e root', () => {
    const { args, seed } = buildRunnerArgs('e2e', 'fixed', 1, null);
    expect(args[0]).toBe('playwright');
    expect(args[1]).toBe('test');
    expect(args[2]).toBe('tests/e2e/');
    // Playwright has no seeded file shuffle, so nothing derives a seed here.
    expect(seed).toBeNull();
  });

  it('does NOT pass --timeout, which the target spec overrides anyway', () => {
    // `inventory.spec.ts` sets `test.describe.configure({ timeout: 90000 })`,
    // and a describe-level timeout beats the CLI flag. Passing --timeout would
    // be a flag that changes nothing on the very spec the soak runs — a setting
    // that reads as covered while doing nothing.
    const { args } = buildRunnerArgs('e2e', 'fixed', 1, null);
    expect(args.some((a) => a.startsWith('--timeout'))).toBe(false);
  });

  it('bounds the whole rep so a wedged browser cannot stall the night', () => {
    const { args } = buildRunnerArgs('e2e', 'fixed', 1, null);
    const bound = args.find((a) => a.startsWith('--global-timeout'));
    expect(bound).toBeDefined();
    // Derived in the source from the spec's own 90s per-test budget; asserted
    // here so a casual edit to a rounder number has to argue with the derivation.
    expect(bound).toBe('--global-timeout=660000');
  });

  it('does not pass --workers, because the config already owns that', () => {
    // playwright.config.ts pins workers:1 for the shared reader. The driver's
    // standing invariant is that the suite cannot observe it is being
    // characterised, so it must not restate the suite's own settings.
    const { args } = buildRunnerArgs('e2e', 'fixed', 1, null);
    expect(args.some((a) => a.startsWith('--workers'))).toBe(false);
  });

  it('rejects shape=shuffle rather than silently running unshuffled', () => {
    // Playwright has no seeded file-order shuffle. Accepting the flag and
    // ignoring it would produce a record claiming a shape it never ran.
    expect(() => assertShapeSupported('e2e', 'shuffle')).toThrow(/shuffle/i);
    expect(() => assertShapeSupported('vitest', 'shuffle')).not.toThrow();
  });

  it('supports fixed, alone and cold', () => {
    for (const shape of ['fixed', 'alone', 'cold']) {
      expect(() => assertShapeSupported('e2e', shape)).not.toThrow();
    }
  });
});

describe('playwright report parsing', () => {
  /**
   * The real report's `config.rootDir` IS the testDir, and `suite.file` is
   * relative to it — a live run of inventory.spec.ts reports the bare
   * `inventory.spec.ts`, not a path. Modelled here because the first version of
   * these fixtures assumed a repo-relative `file` and passed against a shape
   * Playwright does not emit: a test whose fixture misrepresents the payload is
   * the same confident-wrong-answer this driver exists to avoid, one level up.
   */
  const ROOT_DIR = path.join(process.cwd(), 'tests', 'e2e');

  it('reads files in execution order, converting ISO startTime to epoch ms', () => {
    // The `files` array is read as EXECUTION ORDER downstream — the predecessor
    // table in summarise-suite-runs.mjs calls element i-1 the predecessor of
    // element i. Playwright reports ISO strings where vitest reports epoch ms.
    const report = fixture('pw-ok.json', {
      config: { rootDir: ROOT_DIR },
      suites: [
        {
          file: 'second.spec.ts',
          specs: [
            {
              title: 'b',
              ok: true,
              tests: [{ status: 'expected', results: [{ startTime: '2026-08-29T10:00:20.000Z' }] }],
            },
          ],
        },
        {
          file: 'first.spec.ts',
          specs: [
            {
              title: 'a',
              ok: true,
              tests: [{ status: 'expected', results: [{ startTime: '2026-08-29T10:00:10.000Z' }] }],
            },
          ],
        },
      ],
    });
    const { files, reportMissing } = readPlaywrightReport(report);
    expect(reportMissing).toBe(false);
    // Resolved against rootDir, so both runners write cwd-relative paths into
    // the same field and the recorded name matches the --target that was typed.
    expect(files.map((f) => f.name)).toEqual([
      'tests/e2e/first.spec.ts',
      'tests/e2e/second.spec.ts',
    ]);
    expect(files[0].startTime).toBe(Date.parse('2026-08-29T10:00:10.000Z'));
  });

  it('walks nested describe suites, which is where every real spec lives', () => {
    const report = fixture('pw-nested.json', {
      config: { rootDir: ROOT_DIR },
      suites: [
        {
          file: 'inventory.spec.ts',
          specs: [],
          suites: [
            {
              title: 'Consolidated Inventory Tests',
              specs: [
                {
                  title: '1. trigger press/release changes trigger state',
                  ok: false,
                  tests: [
                    { status: 'unexpected', results: [{ startTime: '2026-08-29T10:00:00.000Z' }] },
                  ],
                },
              ],
            },
          ],
        },
      ],
    });
    const { files } = readPlaywrightReport(report);
    expect(files).toHaveLength(1);
    expect(files[0].name).toBe('tests/e2e/inventory.spec.ts');
    expect(files[0].status).toBe('failed');
    expect(files[0].failed).toEqual([
      'Consolidated Inventory Tests > 1. trigger press/release changes trigger state',
    ]);
  });

  it('an all-skipped file is `skipped`, never a pass', () => {
    // `spec.ok` is true for a skipped spec, so a file where every test skipped
    // looks identical to one where every test passed. Over a soak that is forty
    // clean rows on a night where nothing executed.
    const report = fixture('pw-skipped.json', {
      config: { rootDir: ROOT_DIR },
      suites: [
        {
          file: 'inventory.spec.ts',
          specs: [
            { title: 'a', ok: true, tests: [{ status: 'skipped', results: [] }] },
            { title: 'b', ok: true, tests: [{ status: 'skipped', results: [] }] },
          ],
        },
      ],
    });
    const { files } = readPlaywrightReport(report);
    expect(files[0].status).toBe('skipped');
  });

  it('a partly-skipped file that ran something is a pass', () => {
    // The live shape: inventory.spec.ts is 2 active + 3 skipped.
    const report = fixture('pw-partial.json', {
      config: { rootDir: ROOT_DIR },
      suites: [
        {
          file: 'inventory.spec.ts',
          specs: [
            {
              title: 'a',
              ok: true,
              tests: [{ status: 'expected', results: [{ startTime: '2026-08-29T10:00:00.000Z' }] }],
            },
            { title: 'b', ok: true, tests: [{ status: 'skipped', results: [] }] },
          ],
        },
      ],
    });
    const { files } = readPlaywrightReport(report);
    expect(files[0].status).toBe('passed');
  });

  it('a load error cannot read as an empty pass', () => {
    // A spec that fails to compile produces top-level `errors` and zero specs.
    // Reporting `files: []` there is indistinguishable from "nothing failed".
    const report = fixture('pw-loaderror.json', {
      config: { rootDir: ROOT_DIR },
      suites: [],
      errors: [{ message: 'SyntaxError', location: { file: 'tests/e2e/inventory.spec.ts' } }],
    });
    const { files } = readPlaywrightReport(report);
    expect(files).toHaveLength(1);
    expect(files[0].status).toBe('failed');
  });

  it('a run-level error with no file still produces a failed row', () => {
    const report = fixture('pw-globalerror.json', {
      suites: [],
      errors: [{ message: 'global setup failed' }],
    });
    const { files } = readPlaywrightReport(report);
    expect(files).toHaveLength(1);
    expect(files[0].status).toBe('failed');
  });

  it('a missing report is recorded as missing, never as a pass', () => {
    expect(readPlaywrightReport(path.join(scratch, 'nope.json')).reportMissing).toBe(true);
    expect(readVitestReport(path.join(scratch, 'nope.json')).reportMissing).toBe(true);
  });

  it('an unparseable report is recorded as missing', () => {
    const bad = fixture('pw-bad.json', 'not json {');
    expect(readPlaywrightReport(bad).reportMissing).toBe(true);
  });
});

/**
 * Needles no Playwright rep can EVER produce, because the emitter is a file no
 * browser loads: `tests/integration/cs108/CS108WorkerTestHarness.ts`.
 *
 * The three `[ble-timing]` needles used to be in this list and are not any more.
 * They were absent for an incidental reason — a case-sensitive console forwarder
 * — not a structural one, and TRA-1209 fixed the forwarder. That distinction is
 * exactly why they were recorded as `null` rather than `0`: a zero would have
 * read as "the transport did nothing" and nobody would have gone looking.
 *
 * ⚠ MIXED LIST as of TRA-1223, and the mixture is deliberate. The list means
 * "carries null on an e2e rep", which is the right membership test for this
 * file's purpose, but the REASONS differ and the difference decides whether an
 * entry may ever leave:
 *
 *   structural   harnessLines, triggerTimeout, modeSwitchFailed — the emitter is
 *                a file no browser loads. These can never leave.
 *   incidental   powerOffTimeouts, toleratedPowerOffs — the worker DOES run in
 *                the browser, but these are `logger.warn` and carry none of the
 *                console forwarder's KEEP tokens, so it drops them. Widening the
 *                forwarder would move them out, exactly as TRA-1209 moved the
 *                `[ble-timing]` needles out. Do that on a measurement, not on an
 *                assumption.
 */
const VITEST_ONLY = [
  'harnessLines',
  'triggerTimeout',
  'powerOffTimeouts',
  'toleratedPowerOffs',
  'modeSwitchFailed',
];

describe('signals are per-runner, and absence is not zero', () => {
  it('the e2e needle table omits every vitest-only needle', () => {
    for (const name of VITEST_ONLY) {
      expect(E2E_SIGNALS[name], `${name} cannot be produced by a Playwright rep`).toBeUndefined();
    }
  });

  it('keeps the needles a Playwright rep genuinely can produce', () => {
    // Aliased from SIGNALS rather than re-typed — two spellings of "the same"
    // signal is how a count silently means different things per runner. The
    // `[ble-timing]` three are here since TRA-1209 fixed the console forwarder
    // that was dropping them.
    for (const name of [
      'startScanFailed',
      'stopScanFailed',
      'transportRefused',
      'transportUnreachable',
      'ackSamples',
      'linkCloses',
      'connectSamples',
    ]) {
      expect(E2E_SIGNALS[name]).toBe(SIGNALS[name]);
    }
  });

  it('vitest signals are byte-identical to today — no e2e keys leak in', () => {
    const log = fixture('vitest.log', '[Harness] connect\n[Harness] disconnect\n');
    const counts = readSignals(log, 'vitest');
    // `commandTimeouts` is named explicitly because it is NOT a needle — it is a
    // parsed per-op map (TRA-1226), so deriving the expected key set from
    // SIGNALS alone cannot see it. Listing it here keeps this assertion exact
    // rather than quietly loosening it to a subset check: a genuine e2e leak
    // must still fail, and it does, below.
    expect(Object.keys(counts).sort()).toEqual(
      ['logMissing', 'commandTimeouts', ...Object.keys(SIGNALS)].sort()
    );
    expect(counts.harnessLines).toBe(2);
  });

  it('no e2e-only key reaches a vitest record', () => {
    // The guard the assertion above is named for, stated directly. It used to be
    // implied by an exact key-set match against SIGNALS; once a non-needle field
    // joined that record the implication got weaker, so the real rule is written
    // out rather than left to be inferred from a list.
    const log = fixture('vitest-leak.log', '[Harness] x\n');
    const counts = readSignals(log, 'vitest');
    const e2eOnly = Object.keys(E2E_SIGNALS).filter((name) => !(name in SIGNALS));

    expect(e2eOnly.length, 'the e2e table must have at least one exclusive key').toBeGreaterThan(0);
    for (const name of e2eOnly) {
      expect(counts, `${name} is an e2e-only needle and must not appear here`).not.toHaveProperty(
        name
      );
    }
  });

  it('an e2e rep records null for a signal it cannot produce, not 0', () => {
    const log = fixture('e2e.log', '[Connection] Connect button found, clicking...\n');
    const counts = readSignals(log, 'e2e');
    for (const name of VITEST_ONLY) {
      expect(counts[name], `${name} must be null (unavailable), never 0 (measured zero)`).toBeNull();
    }
    expect(counts.e2eConnectLines).toBe(1);
    expect(counts.startScanFailed).toBe(0);
  });

  it('defaults to the vitest table, so every existing call site is unchanged', () => {
    const log = fixture('default.log', '[Harness] x\n');
    expect(readSignals(log)).toEqual(readSignals(log, 'vitest'));
  });

  it('a missing log is missing regardless of runner', () => {
    expect(readSignals(path.join(scratch, 'gone.log'), 'e2e')).toEqual({ logMissing: true });
  });

  it('signalsFor rejects an unknown runner rather than defaulting', () => {
    expect(() => signalsFor('cypress')).toThrow(/cypress/);
  });
});

describe('runner provenance on a record', () => {
  it('an explicit runner is honoured', () => {
    expect(runnerOf({ runner: 'e2e' })).toBe('e2e');
  });

  it('a record with no runner is vitest, because every historical record is', () => {
    // Every schema-1 and schema-2 record predates the field. Reading them as
    // anything else would recompute their signals against the wrong needle table.
    expect(runnerOf({ schema: 2 })).toBe('vitest');
  });
});

describe('the capture canary is named for its role, not its needle', () => {
  /**
   * `harnessLines` is named for the STRING. The watchdog and the summariser want
   * the ROLE — "did this rep produce any observable output at all" — and the
   * string that answers it differs per runner. Reading the vitest needle on an
   * e2e record is how a working check becomes a silent no-op.
   */
  it('reads [Harness] on a vitest record', () => {
    const record = { runner: 'vitest', signals: { logMissing: false, harnessLines: 12 } };
    expect(captureCanaryCount(record)).toBe(12);
  });

  it('reads [Connection] on an e2e record, not the null [Harness]', () => {
    const record = {
      runner: 'e2e',
      signals: { logMissing: false, harnessLines: null, e2eConnectLines: 4 },
    };
    expect(captureCanaryCount(record)).toBe(4);
  });

  it('a record with no runner reads the vitest canary', () => {
    expect(captureCanaryCount({ signals: { logMissing: false, harnessLines: 3 } })).toBe(3);
  });

  it('a missing log yields null — unknown, not zero', () => {
    expect(captureCanaryCount({ runner: 'e2e', signals: { logMissing: true } })).toBeNull();
  });
});

describe('the watchdog void-capture check works on both runners', () => {
  it('still aborts on a vitest rep that captured nothing', () => {
    expect(isVoidCapture({ signals: { logMissing: false, harnessLines: 0 } })).toBe(true);
  });

  it('aborts on an e2e rep that captured nothing', () => {
    expect(
      isVoidCapture({
        runner: 'e2e',
        signals: { logMissing: false, harnessLines: null, e2eConnectLines: 0 },
      })
    ).toBe(true);
  });

  it('does NOT abort on a healthy e2e rep whose [Harness] count is null', () => {
    // The regression this whole ticket turns on: `harnessLines === 0` would fire
    // on every single e2e rep, because the emitter is an integration-only file.
    expect(
      isVoidCapture({
        runner: 'e2e',
        signals: { logMissing: false, harnessLines: null, e2eConnectLines: 9 },
      })
    ).toBe(false);
  });

  it('treats an unknown canary as void rather than assuming health', () => {
    expect(isVoidCapture({ runner: 'e2e', signals: { logMissing: true } })).toBe(true);
  });
});

describe('resolveSignals is runner-aware', () => {
  it('does not recompute an e2e record against the vitest table', () => {
    // The trap: `isCurrent` checks that every needle in the table is present.
    // Against the vitest table an e2e record is always "stale", so it would be
    // recomputed from its log with the wrong needles — silently turning a null
    // into a 0, which is the exact conflation this ticket exists to prevent.
    const stored = { logMissing: false };
    for (const name of Object.keys(E2E_SIGNALS)) stored[name] = 0;
    for (const name of VITEST_ONLY) stored[name] = null;
    const { signals, source } = resolveSignals({ runner: 'e2e', signals: stored, outputLog: null });
    expect(source).toBe('record');
    expect(signals.harnessLines).toBeNull();
  });

  it('still recomputes a vitest record that predates a needle', () => {
    const log = fixture('recompute.log', '[Harness] x\n');
    const { source, signals } = resolveSignals({ signals: { logMissing: false }, outputLog: log });
    expect(source).toBe('recomputed');
    expect(signals.harnessLines).toBe(1);
  });
});
