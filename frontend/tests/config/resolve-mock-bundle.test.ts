import { describe, it, expect } from 'vitest';
import { createRequire } from 'module';
import { existsSync } from 'fs';
import path from 'path';
import {
  resolveMockBundlePath,
  BUNDLE_SPECIFIER,
  mockVersionFromPath,
  installedMockVersion,
  assertMockBundleCurrent,
} from './resolve-mock-bundle';

/**
 * TRA-1177 §1. The bundle must be reached through the package's published
 * interface, not through its internal directory layout. Reading it by literal
 * path failed silently — vite logged and served a page with no
 * `navigator.bluetooth` — which presents as a dead reader rather than a
 * packaging error.
 */
describe('resolveMockBundlePath', () => {
  it('resolves through the ble-mcp-test exports map', () => {
    const require = createRequire(import.meta.url);

    expect(resolveMockBundlePath()).toBe(require.resolve(BUNDLE_SPECIFIER));
  });

  it('returns a path that exists', () => {
    expect(existsSync(resolveMockBundlePath())).toBe(true);
  });

  it('does not depend on the public/ symlink', () => {
    // The symlink this replaces is deleted; resolution must not need it.
    const symlink = path.resolve(__dirname, '../../public/web-ble-mock.bundle.js');

    expect(existsSync(symlink)).toBe(false);
    expect(existsSync(resolveMockBundlePath())).toBe(true);
  });
});

/**
 * TRA-1200. A 22h-old dev server served mock 0.12.0 against bridge 0.13.0 for
 * every one of a 150-rep hardware measurement's reps, and nothing in this repo
 * noticed. Every repo-level check was RIGHT and every one of them was answering
 * a question about the tree rather than about the running process: clean git
 * status, lockfile pinning 0.13.0, `node_modules` symlink pointing at 0.13.0.
 *
 * The mechanism: `require.resolve` does not return the symlink, it resolves
 * THROUGH it to a version-pinned pnpm store path — and Node caches that
 * resolution for the life of the process. pnpm retains every previously
 * installed version, so when the symlink moved the cached path still pointed at
 * a directory that was still there. The stale read succeeded, silently, forever.
 *
 * ⚠ The existing guard above could not catch this, and that is not a defect in
 * it. It protects against a path that is MISSING; this path was present and
 * readable throughout. "Can I read a bundle?" is a different question from
 * "am I reading THE bundle?"
 */
describe('stale mock bundle detection', () => {
  it('reads the version out of a pnpm store path', () => {
    expect(
      mockVersionFromPath(
        '/repo/node_modules/.pnpm/ble-mcp-test@0.12.0/node_modules/ble-mcp-test/dist/web-ble-mock.bundle.js'
      )
    ).toBe('0.12.0');
  });

  it('returns null for a layout carrying no version, rather than guessing one', () => {
    expect(
      mockVersionFromPath('/repo/node_modules/ble-mcp-test/dist/web-ble-mock.bundle.js')
    ).toBeNull();
  });

  it('reads the installed version through the symlink, not the resolver cache', () => {
    expect(installedMockVersion()).toMatch(/^\d+\.\d+\.\d+/);
  });

  it('passes on a correctly resolved tree', () => {
    expect(() => assertMockBundleCurrent()).not.toThrow();
  });

  it('throws naming BOTH versions when they differ', () => {
    expect(() => assertMockBundleCurrent({ resolved: '0.12.0', installed: '0.13.0' })).toThrow(
      /0\.12\.0[\s\S]*0\.13\.0/
    );
  });

  /**
   * "Could not determine" is not "mismatch", and conflating them fails in both
   * directions: throwing on unknown bricks any layout this parser does not
   * recognise, while reporting a match on unknown recreates the original
   * silence. Staying quiet here is why the bridge-side counter (TRA-1211) is a
   * genuinely independent second detector rather than a belt-and-braces copy —
   * it observes what arrived over the wire, and can see cases this one cannot.
   */
  it('does not throw when either version is unknown', () => {
    expect(() => assertMockBundleCurrent({ resolved: null, installed: '0.13.0' })).not.toThrow();
    expect(() => assertMockBundleCurrent({ resolved: '0.13.0', installed: null })).not.toThrow();
  });

  it('names the remedy, since the tree looks correct and the process is at fault', () => {
    expect(() => assertMockBundleCurrent({ resolved: '0.12.0', installed: '0.13.0' })).toThrow(
      /restart/i
    );
  });
});
