import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { spawnSync } from 'child_process';
import path from 'path';

/**
 * TRA-1203 parts A and C. Two documents told readers to do things that stopped
 * being true, and one working directory was one `git add -A` away from being
 * committed into an unrelated PR.
 *
 * The common shape is a fact that was correct when written and decayed
 * silently. `docs/ble-hardware-access.md` even predicted its own decay — it
 * carried a block titled "Current implementation — expected to change" asking
 * to be re-verified after the Python replatform — and then sat unchanged
 * through the replatform, the port move, and the systemd unit.
 *
 * A doc that names deleted tooling is worse than a missing doc: it reads as
 * authoritative, and the reader who follows it spends their time on a
 * `pgrep` for a binary that no longer exists rather than concluding they need
 * to go and look.
 */

const REPO_ROOT = path.resolve(__dirname, '../../..');
const DOC = 'docs/ble-hardware-access.md';

function readDoc(): string {
  return readFileSync(path.join(REPO_ROOT, DOC), 'utf-8');
}

describe('scratchpad is not one `git add -A` from a PR', () => {
  /**
   * Part A. `scratchpad/` was untracked AND unignored, and csw:work Step 7 runs
   * `git add -A && git commit`. Any ticket worked from this checkout would have
   * swept the soak scratchpad — driver logs, run-identity notes, multi-hundred-MB
   * daemon logs — into whatever PR happened to be open.
   *
   * Asserted through `git check-ignore` rather than by grepping `.gitignore`,
   * because the question is whether git ignores the path, not whether some file
   * contains a string that looks like it should. A pattern in the wrong
   * `.gitignore`, or shadowed by a later negation, greps green and ignores
   * nothing.
   */
  it('git ignores scratchpad/', () => {
    const res = spawnSync('git', ['check-ignore', '-q', 'scratchpad/'], {
      cwd: REPO_ROOT,
    });

    expect(
      res.status,
      'git check-ignore says scratchpad/ is NOT ignored — `git add -A` would sweep soak working data into the next PR'
    ).toBe(0);
  });
});

describe('the hardware-access doc describes the bridge that exists', () => {
  it('does not name the deleted Rust bridge', () => {
    // Deleted in the Python replatform (ble-mcp-test TRA-1155 / TRA-1163).
    // Anyone grepping for it finds nothing and has no way to tell whether they
    // typed it wrong or the doc is stale.
    expect(readDoc()).not.toMatch(/rust-ble-test/);
  });

  /**
   * The rule that inverted. The Python bridge builds its transport inside the
   * per-connection handler, so no device is held until a client connects and
   * disconnecting releases it. The daemon holds the PORT, not the RADIO.
   *
   * Verified 2026-08-29 three ways, and the citable one is the ESPHome proxy's
   * own slot accounting at release — `used=0 free=4 limit=4 allocated=[]` — the
   * component that owns the connection slots reporting zero held, rather than
   * the daemon reporting that it *sent* a disconnect.
   */
  it('does not tell the reader to stop the process to free the radio', () => {
    const doc = readDoc();

    expect(doc).not.toMatch(/only\s+`?SIGTERM`?\s+frees/i);
    expect(doc).not.toMatch(/stop the bridge process/i);
    expect(doc).not.toMatch(/the bridge process must not be running/i);
  });

  /**
   * Positive assertions, because "does not say the wrong thing" is satisfied by
   * an empty file. These name the mechanism that replaced the process-grepping:
   * one call that answers who holds the command path, including the holder that
   * no process listing can see.
   */
  it('points at the one call that answers who holds the radio', () => {
    const doc = readDoc();

    expect(doc).toMatch(/get_connection_state/);
    expect(doc).toMatch(/observer_count/);
  });

  /**
   * The lifecycle half (C-2), written only once ble-mcp-test TRA-1202 actually
   * installed the unit — documenting commands that do not resolve is worse than
   * the stale text it replaces.
   */
  it('gives the supervised-unit lifecycle commands', () => {
    const doc = readDoc();

    expect(doc).toMatch(/systemctl --user/);
    expect(doc).toMatch(/ble-bridge/);
  });

  /**
   * `--user` is load-bearing rather than stylistic, and the doc has to say why.
   * The MCP control socket lives under `/run/user/<uid>`, which does not exist
   * for a system unit — so a system-scope install comes up looking perfectly
   * healthy with the entire MCP surface silently gone. It presents as "the MCP
   * tools are broken", never as "the unit is installed wrong", which is what
   * makes it worth a sentence in a doc rather than a footnote.
   */
  it('explains why the unit is user-scoped', () => {
    expect(readDoc()).toMatch(/\/run\/user/);
  });
});

describe('CLAUDE.md states the relaxed hardware rule', () => {
  const claude = readFileSync(path.join(REPO_ROOT, 'CLAUDE.md'), 'utf-8');

  /**
   * The line that was actively wrong. On 2026-08-29 a session read CLAUDE.md
   * rather than the current notes and told Mike to tear down a working daemon
   * on the strength of it; the bridge session pushed back and was right.
   *
   * A running bridge does not block hand-testing. A connected client does.
   */
  it('does not claim a running bridge blocks hand-testing', () => {
    expect(claude).not.toMatch(/a running bridge blocks/i);
  });

  it('still carries the hardware property that did not change', () => {
    // One connection at a time is a property of the CS108 and outlives every
    // bridge implementation. The relaxation must not read as "contention is
    // solved".
    expect(claude).toMatch(/One connection at a time/i);
  });
});
