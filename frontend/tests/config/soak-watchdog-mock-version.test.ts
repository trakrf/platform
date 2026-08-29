import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { mockVersionBreach } from '../../scripts/watch-soak-abort-criteria.mjs';

/**
 * TRA-1200 / TRA-1211. The bridge is the only party that can see what the
 * browser actually loaded, and during TRA-1200's arm it said so 150 times — into
 * a journal nothing in this harness reads.
 *
 * ⚠ WHY THE COUNTER, NOT THE POINT-IN-TIME FIELD.
 * `get_connection_state.mock_version_match` is only true evidence while a client
 * is attached. Between reps it reads held:false with no client, and with ~27s
 * reps against a 300s poll most samples land in exactly that gap — so a snapshot
 * is unmissable only if you happen to sample mid-rep. A monotonic counter cannot
 * be missed: baseline at start, compare at each poll. Same reasoning the
 * NRestarts check already uses for the daemon itself, where "is a daemon alive"
 * was replaced by "did this one restart" for the same reason.
 */
describe('mockVersionBreach', () => {
  it('does not fire when the counter has not moved', () => {
    expect(mockVersionBreach(0, 0).breached).toBe(false);
    expect(mockVersionBreach(3, 3).breached).toBe(false);
  });

  it('fires when the counter increments mid-run', () => {
    const out = mockVersionBreach(0, 1);

    expect(out.breached).toBe(true);
    expect(out.reason).toMatch(/0 -> 1/);
  });

  it('fires on any increase, not only by one', () => {
    expect(mockVersionBreach(2, 7).breached).toBe(true);
  });

  /**
   * Until ble-mcp-test 0.14.0 ships the field (TRA-1211) it is absent on every
   * poll. That is "cannot check", which must not read as "checked and clean" —
   * but it must not abort either: refusing to run against the bridge that is
   * actually deployed would make the instrument unusable, and the client-side
   * detector in resolve-mock-bundle.ts is the cover for exactly this window.
   * The reason string is what keeps the distinction visible in RUN-IDENTITY.
   */
  it('does not fire when the bridge does not publish the field, and says why', () => {
    const out = mockVersionBreach(null, null);

    expect(out.breached).toBe(false);
    expect(out.reason).toMatch(/cannot check/i);
    expect(out.reason).toMatch(/0\.14\.0|TRA-1211/);
  });

  /**
   * A bridge upgraded under a running soak is already its own abort — the
   * uptime check catches it and exits 2. This must not double-report that as a
   * version breach, which would send the reader hunting the wrong cause.
   */
  it('does not fire when the field appears mid-run having been absent at baseline', () => {
    expect(mockVersionBreach(null, 0).breached).toBe(false);
    expect(mockVersionBreach(null, 5).breached).toBe(false);
  });

  it('does not fire when the field vanishes mid-run', () => {
    expect(mockVersionBreach(0, null).breached).toBe(false);
  });
});
