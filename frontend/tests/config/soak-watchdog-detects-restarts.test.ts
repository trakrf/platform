import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import path from 'path';
import {
  hasRestarted,
  fieldIsClear,
  readIdentity,
  restartVerdict,
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

/**
 * TRA-1205. The bridge publishes `instance_id` (ble-mcp-test 0.13.0+), which
 * answers "is this a different process" as an equality test — no tolerance
 * window for a real restart to hide in.
 *
 * It does NOT replace the uptime arithmetic. TRA-1204's acceptance originally
 * said consumers could drop it; that was wrong and was corrected. The two
 * answer different questions, and a host SUSPEND fires only the second:
 * CLOCK_MONOTONIC does not advance across suspend, so the daemon never
 * restarts, `instance_id` is unchanged and reports all-good, while wall-clock
 * time the run did not experience has passed. That run is void and
 * `instance_id` cannot see it.
 *
 * Measured on mssb 2026-08-29 — a manual `systemctl restart`:
 *   instance_id  d652f699… -> 0a396dc6…   CHANGED
 *   uptime       2051s     -> 24s         RESET
 *   NRestarts    0         -> 0           UNCHANGED
 * so systemd's NRestarts is not a superset of either and is not consulted.
 */
describe('readIdentity', () => {
  it('reads the identity fields from a current status payload', () => {
    expect(
      readIdentity({
        instance_id: 'abc',
        code_fingerprint: 'ff00',
        code_source_root: '/somewhere',
        uptime_seconds: 42,
      })
    ).toEqual({
      instanceId: 'abc',
      codeFingerprint: 'ff00',
      codeSourceRoot: '/somewhere',
      uptimeSeconds: 42,
    });
  });

  /**
   * The watchdog parses raw JSON with no schema, and that is the ONLY reason
   * this automated consumer was unaffected when the peer added fields on
   * 2026-08-29 while the interactive path broke. Nothing asserted it until now,
   * so the property was true by luck rather than by test.
   */
  it('ignores fields it has never heard of, and still reads uptime_seconds', () => {
    const out = readIdentity({
      uptime_seconds: 7,
      instance_id: 'abc',
      some_future_field: { nested: true },
      another: [1, 2, 3],
      version: '9.9.9',
    });

    expect(out.uptimeSeconds).toBe(7);
    expect(out.instanceId).toBe('abc');
  });

  it('reports absent fields as null rather than undefined or a guess', () => {
    expect(readIdentity({ uptime_seconds: 7 })).toEqual({
      instanceId: null,
      codeFingerprint: null,
      codeSourceRoot: null,
      uptimeSeconds: 7,
    });
  });

  it('survives a null or non-object status without throwing', () => {
    for (const bad of [null, undefined, 'nope', 42]) {
      expect(readIdentity(bad).instanceId).toBeNull();
      expect(readIdentity(bad).uptimeSeconds).toBeNull();
    }
  });

  /** An empty string is not an id. Treating it as one makes two daemons that
   * both failed to report compare EQUAL — which reads as "no restart". */
  it('treats an empty instance_id as absent', () => {
    expect(readIdentity({ instance_id: '', uptime_seconds: 1 }).instanceId).toBeNull();
  });
});

const uptimeOk = {
  uptimeStart: 100,
  wallStart: 1000,
  uptimeNow: 160,
  wallNow: 1060,
  toleranceSeconds: 5,
};
const uptimeRestarted = { ...uptimeOk, uptimeNow: 10 };

describe('restartVerdict', () => {
  it('is quiet when the id matches and the arithmetic agrees', () => {
    expect(restartVerdict({ startId: 'a', nowId: 'a', ...uptimeOk }).restarted).toBe(false);
  });

  it('fires on a changed instance_id, naming both values', () => {
    const v = restartVerdict({ startId: 'a', nowId: 'b', ...uptimeOk });

    expect(v.restarted).toBe(true);
    expect(v.by).toContain('instance_id');
    expect(v.reason).toMatch(/a.*b/);
  });

  /**
   * The case this ticket exists to protect, and the reason the arithmetic stays.
   * A host suspend does not restart the daemon: instance_id is unchanged and
   * reports all-good while wall time the run did not experience has passed.
   */
  it('fires on the arithmetic alone when the id is unchanged', () => {
    const v = restartVerdict({ startId: 'a', nowId: 'a', ...uptimeRestarted });

    expect(v.restarted).toBe(true);
    expect(v.by).toContain('uptime');
    expect(v.by).not.toContain('instance_id');
  });

  it('names both when both fire', () => {
    const v = restartVerdict({ startId: 'a', nowId: 'b', ...uptimeRestarted });

    expect(v.by).toEqual(expect.arrayContaining(['instance_id', 'uptime']));
  });

  /** Absence must degrade to the CURRENT behaviour, never to "no check" — the
   * incus container and any hand-started daemon may predate the field. */
  it('falls back to the arithmetic when no id is available at all', () => {
    expect(restartVerdict({ startId: null, nowId: null, ...uptimeRestarted }).restarted).toBe(true);
    expect(restartVerdict({ startId: null, nowId: null, ...uptimeOk }).restarted).toBe(false);
  });

  /** A daemon that identified itself at start and stops, while still answering
   * status, is not the same process — or was downgraded under the run. Either
   * way the continuity claim is gone. */
  it('fires when the id vanishes mid-run while status still answers', () => {
    const v = restartVerdict({ startId: 'a', nowId: null, ...uptimeOk });

    expect(v.restarted).toBe(true);
    expect(v.reason).toMatch(/stopped reporting/i);
  });

  /** The reverse is an upgrade mid-run. The arithmetic still governs; do not
   * report an id change that did not happen. */
  it('does not claim an id change when the id only APPEARS mid-run', () => {
    expect(restartVerdict({ startId: null, nowId: 'b', ...uptimeOk }).by).not.toContain(
      'instance_id'
    );
  });
});

describe('the watchdog wires both detectors into the run', () => {
  const source = codeOnly(readFileSync(path.join(FRONTEND_ROOT, WATCHDOG), 'utf8'));

  /**
   * These assert CALL SITES, not definitions, and the distinction cost a
   * revision. The first draft matched /restartVerdict\(/ — which its own
   * `export function restartVerdict(` satisfies — and matched `codeFingerprint`
   * against readIdentity's own return object. Three of four guards passed
   * against a watchdog that was not wired at all.
   *
   * A guard its own subject satisfies is not a guard.
   */
  const occurrences = (needle: string) => source.split(needle).length - 1;

  it('calls restartVerdict somewhere other than where it is defined', () => {
    expect(occurrences('restartVerdict')).toBeGreaterThan(1);
  });

  it('reads identity from both the start status and the current one', () => {
    expect(source).toMatch(/startIdentity/);
    expect(source).toMatch(/nowIdentity/);
  });

  /**
   * Scope item 3. "The daemon was the same process throughout" is evidence only
   * if it was written down at the time — reconstructed afterwards it is an
   * assumption wearing evidence's clothes. These strings live only in the
   * RUN-IDENTITY block, so no field name can satisfy them.
   */
  it('records the bridge instance and code in RUN-IDENTITY', () => {
    expect(source).toMatch(/bridge instance\s+\$\{/);
    expect(source).toMatch(/bridge code\s+\$\{/);
  });

  /**
   * The trap this ticket calls out: the daemon may run from ANY checkout of the
   * peer repo, including one of its worktrees. Hardcoding a path gives a wrong
   * denominator that presents as a stale daemon. The root is published
   * precisely so nobody resolves a pid's cwd or guesses at it.
   */
  it('reads the source root as published rather than guessing a path', () => {
    expect(occurrences('codeSourceRoot')).toBeGreaterThan(1);
    expect(source).not.toMatch(/home\/[a-z]+\/ble-mcp/);
    expect(source).not.toMatch(/proc\/.*\/cwd/);
  });

  /** code_fingerprint is REPORTED, never recomputed. A second implementation of
   * the hash is a second thing to keep in sync, and a currency check here would
   * be a second such gate — the peer's own pretest guard owns that question. */
  it('does not reimplement the fingerprint or gate on it', () => {
    expect(source).not.toMatch(/sha256|createHash/);
  });
});
