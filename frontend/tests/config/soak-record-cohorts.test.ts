import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { describeCohorts, cohortWarning } from '../../scripts/suite-run-signals.mjs';

/**
 * TRA-1200. Both analysis scripts read `.suite-runs/runs.jsonl` wholesale, with
 * no argument and no filter. On 2026-08-29 that file held TRA-1193's 200 vitest
 * rows when a 150-rep e2e arm was about to be analysed; they were moved aside by
 * hand, and nothing in the tooling would have objected if they had not been.
 *
 * Pooling is not always wrong — comparing two arms is a real thing to want — so
 * this warns rather than filtering or aborting. The failure being fixed is
 * SILENCE, not pooling: a summary that blends two runners' rows is confidently
 * wrong and looks exactly like one that does not.
 */
describe('describeCohorts', () => {
  it('reports a single cohort as homogeneous', () => {
    const rows = [
      { runner: 'e2e', note: 'arm A' },
      { runner: 'e2e', note: 'arm A' },
    ];

    expect(describeCohorts(rows).homogeneous).toBe(true);
  });

  it('splits on runner', () => {
    const out = describeCohorts([
      { runner: 'e2e', note: 'arm A' },
      { runner: 'vitest', note: 'arm A' },
    ]);

    expect(out.homogeneous).toBe(false);
    expect(out.groups).toHaveLength(2);
  });

  it('splits on note, because two arms of one runner are still two arms', () => {
    const out = describeCohorts([
      { runner: 'e2e', note: 'CPU-swap arm' },
      { runner: 'e2e', note: 'instrument validation' },
    ]);

    expect(out.homogeneous).toBe(false);
  });

  it('counts each cohort so the mix is legible, not merely flagged', () => {
    const out = describeCohorts([
      { runner: 'vitest', note: 'X' },
      { runner: 'vitest', note: 'X' },
      { runner: 'e2e', note: 'Y' },
    ]);

    expect(out.groups.find((g: { runner: string }) => g.runner === 'vitest').count).toBe(2);
    expect(out.groups.find((g: { runner: string }) => g.runner === 'e2e').count).toBe(1);
  });

  // Records predating TRA-1206 carry no `runner`; runnerOf() calls that vitest,
  // and treating it as a distinct cohort would flag every historical archive.
  it('treats an absent runner as vitest rather than as its own cohort', () => {
    expect(describeCohorts([{ note: 'X' }, { runner: 'vitest', note: 'X' }]).homogeneous).toBe(true);
  });
});

describe('cohortWarning', () => {
  it('is empty for a homogeneous record, so a clean run stays quiet', () => {
    expect(cohortWarning([{ runner: 'e2e', note: 'arm A' }])).toBe('');
  });

  it('names every cohort and its size when the record is mixed', () => {
    const text = cohortWarning([
      { runner: 'vitest', note: 'TRA-1193 verify soak' },
      { runner: 'e2e', note: 'TRA-1200 CPU-swap arm' },
    ]);

    expect(text).toMatch(/vitest/);
    expect(text).toMatch(/e2e/);
    expect(text).toMatch(/TRA-1193 verify soak/);
    expect(text).toMatch(/pooled|mixes|more than one/i);
  });
});
