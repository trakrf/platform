/**
 * Resolve the ble-mcp-test browser bundle through the package's exports map.
 *
 * TRA-1177 §1, row A. The previous mechanism was a symlink at
 * `public/web-ble-mock.bundle.js` pointing into
 * `node_modules/ble-mcp-test/dist/`, read with `readFileSync`. That bypassed the
 * exports map entirely and depended on another repo's internal directory
 * layout, so a `dist/` restructure there became a silent failure here:
 * vite.config.ts caught the read error, logged it, and served the page with no
 * `navigator.bluetooth` at all. That presents as a dead reader, not a packaging
 * error — the silent-fallback class, where something succeeds against the wrong
 * input and nothing errors.
 *
 * Resolving through `ble-mcp-test/browser` makes keeping that path working
 * their contract rather than our assumption, and makes a break loud.
 *
 * Lives here rather than in `scripts/` so it sits beside the other
 * vite-consumed configuration, and so its test is inside the `tests/config/`
 * tree that `pnpm test` actually runs.
 */
import { createRequire } from 'module';

export const BUNDLE_SPECIFIER = 'ble-mcp-test/browser';

export function resolveMockBundlePath(): string {
  const require = createRequire(import.meta.url);

  try {
    return require.resolve(BUNDLE_SPECIFIER);
  } catch (err) {
    throw new Error(
      `[BLE Bridge Plugin] Could not resolve "${BUNDLE_SPECIFIER}" through the ` +
        `ble-mcp-test exports map. The package is installed but does not publish ` +
        `that entry point — check its "exports" field. Original error: ` +
        `${(err as Error).message}`
    );
  }
}
