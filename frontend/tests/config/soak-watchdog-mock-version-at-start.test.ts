import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { mockVersionMismatchAtStart } from '../../scripts/watch-soak-abort-criteria.mjs';

/**
 * The pre-flight that would have saved 2026-08-31's false arm start.
 *
 * `frontend/package.json` said `^0.16.0`, the bump was merged and `main` was
 * current — but `node_modules` in the checkout the driver ran from still held
 * 0.15.0, because `pnpm install` had only ever run inside a worktree that was
 * later deleted. The arm's first rep connected with the old mock. Seven and a
 * half hours would have measured a configuration that does not exist.
 *
 * Every fact needed to catch it was already on screen — `mock_version_match:
 * false` from `get_connection_state`, and a non-zero mismatch counter in
 * RUN-IDENTITY. Nothing read them, so this reads them.
 *
 * ⚠ WHY THE NULL CASE IS THE INTERESTING ONE. `mock_version_match` is
 * three-valued, and the watchdog normally starts BEFORE the driver's first
 * connect — so `null` ("cannot check") is the common reading at pre-flight.
 * Cost it wrong in either direction and the guard is useless: abort on null and
 * no arm can ever start; treat null as agreement and it never fires. Same
 * null-is-not-zero rule the rest of this instrument runs on.
 *
 * Refs: TRA-1216, TRA-1223.
 */
describe('mockVersionMismatchAtStart', () => {
  it('aborts on a definite disagreement, and names both versions', () => {
    // The exact shape the bridge returned on 2026-08-31.
    const out = mockVersionMismatchAtStart({
      mock_version: '0.15.0',
      mock_version_expected: '0.16.0',
      mock_version_match: false,
    });

    expect(out.mismatched).toBe(true);
    // Both halves in the message: which one is loaded, and which was wanted.
    // "versions differ" sends the reader to look up what it should have been.
    expect(out.reason).toContain('0.15.0');
    expect(out.reason).toContain('0.16.0');
  });

  it('does NOT abort when nothing is connected yet — the normal pre-flight case', () => {
    const out = mockVersionMismatchAtStart({
      mock_version: null,
      mock_version_expected: '0.16.0',
      mock_version_match: null,
    });

    expect(out.mismatched).toBe(false);
  });

  it('says "cannot check" rather than reporting agreement it never observed', () => {
    // The distinction the whole instrument turns on: unknown is not clean.
    // A reason reading "matches" here would be a lie the operator acts on.
    const out = mockVersionMismatchAtStart({ mock_version_match: null });

    expect(out.reason).toMatch(/cannot check/i);
    expect(out.reason).not.toMatch(/matches/i);
  });

  it('does not abort on a confirmed match', () => {
    const out = mockVersionMismatchAtStart({
      mock_version: '0.16.0',
      mock_version_expected: '0.16.0',
      mock_version_match: true,
    });

    expect(out.mismatched).toBe(false);
    expect(out.reason).toContain('0.16.0');
  });

  it('treats an absent state as cannot-check, not as a mismatch', () => {
    // A bridge that did not answer is exit 2's subject, not this guard's.
    // Reporting it here would send the operator hunting a stale install when
    // the actual problem is that nothing is listening.
    expect(mockVersionMismatchAtStart(null).mismatched).toBe(false);
    expect(mockVersionMismatchAtStart(undefined).mismatched).toBe(false);
  });
});
