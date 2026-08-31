import { describe, it, expect } from 'vitest';
import { RETRYABLE_CONNECT_CODES } from 'ble-mcp-test';

/**
 * What the INSTALLED mock will retry, read from the package rather than claimed
 * by `package.json`.
 *
 * This is the guard that makes the 0.16.0 bump real. `frontend/package.json`
 * declared `"ble-mcp-test": "^0.15.0"`, and **a caret range on a 0.x version is
 * minor-locked** — `^0.15.0` means `>=0.15.0 <0.16.0`. So `pnpm update` resolves
 * 0.15.x forever and *reports success*: the bump appears to happen, exits 0, and
 * changes nothing. The range itself has to move.
 *
 * A version assertion would be the obvious guard and is the weaker one — it
 * pins a number that has to be edited every release and still proves nothing
 * about what the artifact contains. Asserting the CODE proves the thing we
 * actually depend on, and a 0.15.x resolution cannot satisfy it because the
 * code did not exist yet.
 *
 * It also outlives the bump it was written for. A downgrade, a lockfile
 * drifting back, or a resolution landing somewhere other than what the range
 * says all fail here — which is the point of a committed test over a command
 * someone runs once. Before this existed the bump was protected only by
 * accident, and protection nobody planned is protection nobody maintains.
 *
 * Refs: TRA-1216.
 */
describe('the installed ble-mcp-test mock, on what it will retry', () => {
  it('retries a busy refusal from our OWN releasing connection', () => {
    // `DEVICE_BUSY_SELF` (0.16.0) is the bridge refusing us while OUR previous
    // claim is still closing — measured at 12-21ms, median 16ms, over 63
    // refusals on a 200-rep arm. Waiting genuinely does clear that, so the mock
    // retries it inside its existing connect loop.
    expect(RETRYABLE_CONNECT_CODES).toContain('DEVICE_BUSY_SELF');
  });

  it('still refuses to retry a busy refusal from a FOREIGN holder', () => {
    // The other half of the same contract, and the half a well-meaning upstream
    // change could quietly undo. A foreign holder — another tab, another
    // machine, a hand-test on preview — is a loud refusal that no amount of
    // waiting fixes; retrying it converts a precise error into ~2.4s of pause
    // followed by the same failure. If this ever goes green-to-red, the split
    // has collapsed back and the retry policy is wrong in the expensive
    // direction.
    expect(RETRYABLE_CONNECT_CODES).not.toContain('DEVICE_BUSY');
  });
});
