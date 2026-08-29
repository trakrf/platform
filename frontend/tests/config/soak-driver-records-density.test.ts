import { describe, it, expect } from 'vitest';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
// @ts-expect-error — .mjs instrument module, no types by design
import { densityFor } from '../../scripts/characterise-suite-runs.mjs';

/**
 * TRA-1200. The driver must record field density on the runner that has one, and
 * must not invent the field on the runner that does not.
 *
 * The asymmetry is the same one `appPreflight` carries and it is not cosmetic: a
 * vitest rep runs no application and reads no tags, so a null density field would
 * describe a subject that does not exist. More concretely, adding ANY key to a
 * vitest record breaks the comparability that TRA-1189's 528 reps and TRA-1193's
 * 200 are the baseline for — those are the numbers this milestone is measured
 * against, and they are only comparable if the vitest path still records what it
 * recorded then.
 */
const withLog = (text: string) => {
  const dir = mkdtempSync(path.join(tmpdir(), 'density-'));
  const file = path.join(dir, 'out.log');
  writeFileSync(file, text);
  return file;
};

const E2E_LOG =
  '[Test] First read: 98 reads, 63 unique tags\n' +
  '[Test] Second read: 196 reads, 102 unique tags\n';

describe('densityFor', () => {
  it('returns the read-cycle values for an e2e rep', () => {
    expect(densityFor(withLog(E2E_LOG), 'e2e')).toEqual({
      firstReads: 98,
      firstUnique: 63,
      secondReads: 196,
      secondUnique: 102,
    });
  });

  /**
   * `undefined`, not an all-null object — the caller assigns conditionally, so
   * undefined is what keeps the key off a vitest record entirely rather than
   * present-and-null.
   */
  it('returns undefined for a vitest rep, so the key is absent rather than null', () => {
    expect(densityFor(withLog(E2E_LOG), 'vitest')).toBeUndefined();
  });

  it('still returns a full null-filled shape for an e2e rep whose log is gone', () => {
    expect(densityFor('/nonexistent/out.log', 'e2e')).toEqual({
      firstReads: null,
      firstUnique: null,
      secondReads: null,
      secondUnique: null,
    });
  });
});
