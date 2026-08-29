import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import path from 'path';
import {
  hasRestarted,
  fieldIsClear,
} from '../../scripts/watch-soak-abort-criteria.mjs';

/**
 * TRA-1203 part B. The soak watchdog used to ask "is a daemon alive?" — and
 * under a supervised unit (`Restart=always`, ble-mcp-test TRA-1202) that
 * question is answered "yes" by a *different* process than the one the run
 * started with.
 *
 * The failure is silent and it corrupts evidence rather than stopping the run:
 * a mid-soak crash becomes a fresh daemon in ~5s, the night continues, and the
 * result set spans two daemons with no marker in the data. "One PID throughout"
 * is load-bearing for every soak conclusion we draw, and auto-restart makes it
 * unverifiable rather than false — which is worse, because nothing looks wrong.
 *
 * The replacement asks the bridge's own contract instead: `status.uptime_seconds`
 * over the MCP control socket. No unit name, no cgroup, no journald, no systemd.
 * That matters beyond tidiness — ble-mcp-test plans a second bridge on a
 * container where the supervisor may differ, and a watchdog that breaks when the
 * peer renames a unit is not a watchdog.
 */

const FRONTEND_ROOT = path.resolve(__dirname, '../..');
const WATCHDOG = 'scripts/watch-soak-abort-criteria.mjs';

/** Strip comments before scanning source, matching the house style of the
 * sibling guards: these files explain the traps they avoid, and naming a trap
 * must not trip the guard against it. */
function codeOnly(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1');
}

describe('restart detection via uptime arithmetic', () => {
  const TOLERANCE = 10;

  it('a steady daemon does not read as restarted', () => {
    // 600s of wall clock, 600s of uptime. The ordinary case.
    expect(
      hasRestarted({
        uptimeStart: 1000,
        wallStart: 0,
        uptimeNow: 1600,
        wallNow: 600,
        toleranceSeconds: TOLERANCE,
      })
    ).toBe(false);
  });

  it('detects a restart while uptime is still below where it started', () => {
    // The easy case: uptime visibly went backwards.
    expect(
      hasRestarted({
        uptimeStart: 1000,
        wallStart: 0,
        uptimeNow: 30,
        wallNow: 600,
        toleranceSeconds: TOLERANCE,
      })
    ).toBe(true);
  });

  /**
   * The case the naive check misses, and the reason this is arithmetic rather
   * than a comparison.
   *
   * A daemon that started 100s before the run restarts 10s in. By the time the
   * next poll lands 600s later it has been up for 590s — MORE than the 100s
   * recorded at run start. "Did uptime go down?" answers no, and the run
   * silently continues across two daemons.
   *
   * The arithmetic asks a different question: has this process been up for at
   * least as long as the wall clock says has elapsed? 590 < 700, so no.
   */
  it('detects a restart that has already grown past the starting uptime', () => {
    const naiveWouldMiss = 590 > 100;
    expect(naiveWouldMiss).toBe(true);

    expect(
      hasRestarted({
        uptimeStart: 100,
        wallStart: 0,
        uptimeNow: 590,
        wallNow: 600,
        toleranceSeconds: TOLERANCE,
      })
    ).toBe(true);
  });

  /**
   * Poll jitter only. `uptime_seconds` is monotonic-derived — ble-mcp-test
   * constructs its ControlServer with `started_at=time.monotonic()` — so it
   * cannot be walked by an NTP step and there is nothing here to defend against
   * except the two samples not being simultaneous. A large defensive fudge
   * would be a window in which a real restart hides.
   */
  it('tolerates sub-tolerance jitter between the two samples', () => {
    expect(
      hasRestarted({
        uptimeStart: 1000,
        wallStart: 0,
        uptimeNow: 1597,
        wallNow: 600,
        toleranceSeconds: TOLERANCE,
      })
    ).toBe(false);
  });

  it('fires once the shortfall exceeds tolerance', () => {
    expect(
      hasRestarted({
        uptimeStart: 1000,
        wallStart: 0,
        uptimeNow: 1589,
        wallNow: 600,
        toleranceSeconds: TOLERANCE,
      })
    ).toBe(true);
  });

  /**
   * An unreachable daemon is an abort condition, not an unknown.
   *
   * A status call that times out or refuses cannot be distinguished from a
   * daemon that is wedged, and either way the rest of the night is junk. The
   * old design's objection — "asking the daemon about its own failure is the
   * wrong direction" — dissolves because detection only needs the daemon to
   * answer *at all*, not to answer honestly.
   */
  it('treats a missing uptime reading as a restart', () => {
    expect(
      hasRestarted({
        uptimeStart: 1000,
        wallStart: 0,
        uptimeNow: null,
        wallNow: 600,
        toleranceSeconds: TOLERANCE,
      })
    ).toBe(true);
  });
});

describe('pre-flight field check', () => {
  /**
   * `observer_count > 0` is the hazard that never appears as a process and
   * cannot be seen in any log — most often a leftover mock-injected browser tab
   * holding the command path. On 2026-08-26 contention of exactly this kind
   * invalidated two hardware runs inside ten minutes.
   */
  it('accepts a genuinely clear field', () => {
    expect(fieldIsClear({ held: false, observer_count: 0 })).toBe(true);
  });

  it('rejects a held command path', () => {
    expect(fieldIsClear({ held: true, observer_count: 0 })).toBe(false);
  });

  it('rejects a field with observers attached', () => {
    expect(fieldIsClear({ held: false, observer_count: 1 })).toBe(false);
  });

  /** A malformed or absent reply is not evidence of a clear field. */
  it('rejects a reply that does not carry the fields', () => {
    expect(fieldIsClear(null)).toBe(false);
    expect(fieldIsClear({})).toBe(false);
  });
});

describe('the watchdog source', () => {
  const source = codeOnly(readFileSync(path.join(FRONTEND_ROOT, WATCHDOG), 'utf-8'));

  /**
   * The restart-blind check this ticket exists to remove. `pgrep -f` on the
   * daemon answers "is *a* daemon alive", which under Restart=always is
   * permanently yes.
   *
   * It also self-matches: `pgrep -f` scans argv, and the pattern appears in the
   * argv of the pipeline running it, which produced a false abort during
   * TRA-1189.
   */
  it('does not decide daemon liveness from a process name', () => {
    expect(source).not.toMatch(/pgrep/);
    expect(source).not.toMatch(/ble_bridge/);
  });

  /**
   * The old BRIDGE_LOG pointed into another Claude session's scratchpad, which
   * dies with that session. After it vanished the grep read an absent file and
   * silently found no errors — a check that cannot go red.
   *
   * Logs are forensics now, not detection: a human reads journald after an
   * abort, and journald outlives the process that a 100k-line in-memory ring
   * cannot.
   */
  it('does not read a session-scoped scratchpad path', () => {
    expect(source).not.toMatch(/\/tmp\/claude-/);
  });

  /** Absolute paths into one person's checkout do not survive promotion into
   * the repo, and a worktree is a different checkout every time. */
  it('hardcodes nobody\'s home directory', () => {
    expect(source).not.toMatch(/\/home\/[a-z]+\//);
  });
});
