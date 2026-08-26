import { describe, it, expect } from 'vitest';
import { createRequire } from 'module';
import { existsSync } from 'fs';
import path from 'path';
import { resolveMockBundlePath, BUNDLE_SPECIFIER } from './resolve-mock-bundle';

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
