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
import { readFileSync } from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

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

/**
 * The version encoded in a pnpm store path, or null where the layout carries none.
 *
 * `require.resolve` does not hand back the symlink — it resolves THROUGH it to
 * `.pnpm/ble-mcp-test@<version>/…`. That is why the version is legible here at
 * all, and it is also the reason a cached resolution keeps serving an old
 * version after the symlink moves: the cached string names a version directory
 * that pnpm has not deleted.
 */
export function mockVersionFromPath(resolved: string): string | null {
  return /[/\\]\.pnpm[/\\]ble-mcp-test@([^/\\]+)/.exec(resolved)?.[1] ?? null;
}

/**
 * The version installed RIGHT NOW, read through the symlink on every call.
 *
 * Deliberately NOT `require.resolve`, and deliberately not memoised — the entire
 * defect is that the resolver's answer can be stale while the tree is correct.
 * `readFileSync` follows the symlink at read time, so this reports what pnpm
 * last installed rather than what this process once resolved.
 */
export function installedMockVersion(): string | null {
  const here = path.dirname(fileURLToPath(import.meta.url));
  try {
    const pkg = path.resolve(here, '../../node_modules/ble-mcp-test/package.json');
    return (JSON.parse(readFileSync(pkg, 'utf8')).version as string) ?? null;
  } catch {
    return null;
  }
}

/**
 * Throw when the bundle about to be served is provably not the installed one.
 *
 * Called from `transformIndexHtml`, so it runs per page load and holds no
 * process state — which is what lets it catch a resolution that went stale after
 * the server started. The existing missing-path guard cannot: that path stays
 * present and readable the whole time.
 *
 * ⚠ Silence on "unknown" is deliberate, and is the opposite of the original bug.
 * A mismatch we can PROVE is fatal; a version we cannot determine is not
 * evidence of anything, and throwing on it would break any layout this parser
 * does not recognise. The bridge-side counter (TRA-1211) exists as an
 * independent second detector precisely because this one can be blind.
 */
export function assertMockBundleCurrent(versions?: {
  resolved: string | null;
  installed: string | null;
}): void {
  const resolved = versions ? versions.resolved : mockVersionFromPath(resolveMockBundlePath());
  const installed = versions ? versions.installed : installedMockVersion();
  if (!resolved || !installed || resolved === installed) return;
  throw new Error(
    `[BLE Bridge Plugin] Stale ble-mcp-test mock bundle: serving ${resolved}, but ` +
      `${installed} is installed.\n` +
      `  This dev server resolved the bundle before the dependency changed, and Node ` +
      `caches that resolution for the life of the process. pnpm keeps the old version ` +
      `on disk, so the stale read succeeds silently — a clean tree and a correct ` +
      `lockfile do not rule it out.\n` +
      `  Restart the dev server (pnpm dev:bridge). See TRA-1200.`
  );
}
