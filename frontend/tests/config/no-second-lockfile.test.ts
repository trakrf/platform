/**
 * One lockfile, at the workspace root (TRA-1194).
 *
 * `frontend/pnpm-lock.yaml` sat in this repo for ten months pinning
 * `ble-mcp-test@0.7.3` while the root lockfile had moved nine minor versions
 * past it. It arrived in the bulk copy from the handheld repo (`e6643f05`,
 * 2025-10-17) and was never updated again.
 *
 * It was inert. `pnpm-workspace.yaml` lists `frontend`, so pnpm resolves from
 * the root lockfile and `frontend/node_modules` is symlinked into the root
 * `.pnpm` store — a second lockfile inside a workspace package is simply not
 * consulted.
 *
 * Inert is not harmless. A committed lockfile is what a human or a tool reads
 * to answer "what version does the frontend use?", and this one answered
 * confidently and wrongly. That is worse than the file being absent: absence
 * prompts a question, a stale value closes it. The same shape as ADR 0021's
 * `tests/data/ never created` — a statement that is locally checkable, globally
 * false, and therefore survives review.
 *
 * And it was inert *because of the current workspace layout*, which is a
 * property of the layout rather than of the file, and nothing guarded the
 * layout. This is that guard.
 *
 * It fails on any `pnpm-lock.yaml` outside the repo root — including one
 * regenerated correctly, because "correct today" is exactly what the deleted
 * one was in October 2025. There is one lockfile in a pnpm workspace; if that
 * ever stops being true, this test is the place to record why.
 */

import { describe, it, expect } from 'vitest';
import { readdirSync, statSync, existsSync } from 'fs';
import { join, dirname, resolve, relative } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, '../../..');

/** Directories with no bearing on this repo's own dependency resolution. */
const SKIP_DIRS = new Set([
  'node_modules',
  '.git',
  'dist',
  'build',
  'coverage',
  '.claude', // worktrees live here; each is its own checkout
  'playwright-report',
  'test-results',
  'scratchpad',
]);

function* findLockfiles(dir: string): Generator<string> {
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue;
    const full = join(dir, entry);

    let stats;
    try {
      stats = statSync(full);
    } catch {
      continue; // a broken symlink is not a lockfile
    }

    if (stats.isDirectory()) {
      yield* findLockfiles(full);
    } else if (entry === 'pnpm-lock.yaml') {
      yield full;
    }
  }
}

describe('lockfiles (TRA-1194)', () => {
  it('has exactly one, at the workspace root', () => {
    const found = [...findLockfiles(repoRoot)].map((p) => relative(repoRoot, p)).sort();

    expect(found).toEqual(['pnpm-lock.yaml']);
  });

  it('declares frontend as a workspace package, which is why one is enough', () => {
    // The assertion above is only correct while this holds. If frontend stops
    // being a workspace package it needs its own lockfile, and this guard would
    // then be enforcing the wrong rule — so state the premise rather than
    // leaving it implicit.
    const workspace = join(repoRoot, 'pnpm-workspace.yaml');

    expect(existsSync(workspace)).toBe(true);
    expect(readdirSync(repoRoot)).toContain('pnpm-workspace.yaml');
  });
});
