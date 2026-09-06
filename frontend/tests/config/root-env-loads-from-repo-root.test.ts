/**
 * The guard for TRA-1195: every `.env.local` read resolves from the repo root,
 * never from the process CWD.
 *
 * The defect this exists to keep fixed had no red state of its own. Three files
 * called `dotenv.config({ path: '.env.local' })`, which dotenv resolves against
 * `process.cwd()`. Playwright and vite both run from `frontend/`, so all three
 * looked for `frontend/.env.local` — a file that has never existed. dotenv does
 * not throw on a missing path; it returns an error object nobody reads and
 * loads nothing.
 *
 * The suite passed anyway because direnv had already put the same values in
 * `process.env` before the process started, so the dead call was invisible in
 * every interactive shell. It failed only where direnv does not fire: tool
 * shells, CI, a fresh clone before `direnv allow`. There, `PLAYWRIGHT_BASE_URL`
 * fell back to its documented default and the credentials were simply absent,
 * so the run failed pointing at the app rather than at the environment.
 *
 * Both assertions below are load-bearing, and they fail for different reasons:
 *
 *  1. `loadRootEnv` must resolve the same absolute path no matter what the CWD
 *     is. Resolution from `import.meta.url` is what makes that true; anything
 *     CWD-relative fails this even when it happens to work from `frontend/`.
 *
 *  2. No file may call `dotenv.config` directly. This is the one that catches a
 *     regression, because the original bug was three call sites drifting apart
 *     rather than one being wrong — the same shape as the BLE_MCP_WS_PORT
 *     defect that `resolve-bridge-port.ts` exists to prevent, where a fix
 *     repaired two of three readers and the guard listed the same two.
 */

import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'fs';
import { join, dirname, resolve } from 'path';
import { fileURLToPath } from 'url';

import { ROOT_ENV_PATH, repoRoot } from './load-root-env';

const __dirname = dirname(fileURLToPath(import.meta.url));
const frontendDir = resolve(__dirname, '../..');

/** Directories that hold vendored or generated code we do not own. */
const SKIP_DIRS = new Set(['node_modules', 'dist', 'build', 'coverage', '.git', 'playwright-report', 'test-results']);

/** Remove block and line comments so prose about the fix is not read as the bug. */
function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

/**
 * The argument text of a `dotenv.config(...)` call, with parens balanced.
 *
 * A `[^)]*` match stops at the first close paren, which for
 * `join(dirname(fileURLToPath(import.meta.url)), '..', '..', '.env.local')`
 * truncates before any of the '..' segments — so the depth check counted zero
 * and the guard failed correct code. Balancing is the whole point.
 */
function dotenvConfigArgs(source: string): string | null {
  const start = source.search(/dotenv\s*\.\s*config\s*\(/);
  if (start === -1) return null;

  const open = source.indexOf('(', start);
  let depth = 0;
  for (let i = open; i < source.length; i++) {
    if (source[i] === '(') depth++;
    else if (source[i] === ')') {
      depth--;
      if (depth === 0) return source.slice(open + 1, i);
    }
  }
  return null;
}

function* walk(dir: string): Generator<string> {
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      yield* walk(full);
    } else if (/\.(ts|js|mts|mjs)$/.test(entry)) {
      yield full;
    }
  }
}

describe('root .env.local resolution (TRA-1195)', () => {
  it('resolves to the repo root, not the process CWD', () => {
    // The repo root is the parent of frontend/, and that is a fact about the
    // layout rather than about where anyone happened to invoke the process.
    expect(repoRoot()).toBe(resolve(frontendDir, '..'));
    expect(ROOT_ENV_PATH).toBe(join(resolve(frontendDir, '..'), '.env.local'));
  });

  it('resolves identically from any working directory', () => {
    // This is the assertion the original code could not have passed. It is not
    // enough that the path is correct when run from frontend/; it has to be
    // correct from a tool shell invoked anywhere, which is exactly the context
    // where direnv has not fired and the fallback cannot save the run.
    const original = process.cwd();
    try {
      process.chdir('/');
      expect(ROOT_ENV_PATH).toBe(join(resolve(frontendDir, '..'), '.env.local'));
      process.chdir(frontendDir);
      expect(ROOT_ENV_PATH).toBe(join(resolve(frontendDir, '..'), '.env.local'));
    } finally {
      process.chdir(original);
    }
  });

  it('has no dotenv call site that resolves against the CWD', () => {
    // Counting callers is the wrong assertion here, and the repo already says
    // why: resolve-bridge-port.ts's docblock blesses dev-bridge.js keeping its
    // own copy, because it is plain JS run outside the TS build and cannot
    // import a .ts reader. What must hold is not "one caller" but "no caller
    // trusts the CWD" — so this checks each site's resolution instead.
    //
    // TS callers go through loadRootEnv(). Plain-JS scripts build the path from
    // import.meta.url, and the number of '..' segments has to match how deep
    // the file sits, which is the part dev-bridge.js got wrong: it used the
    // right technique with one '..' too few and landed on frontend/.env.local.
    const offenders: string[] = [];

    for (const file of walk(frontendDir)) {
      const rel = file.slice(frontendDir.length + 1);
      if (file === join(__dirname, 'load-root-env.ts')) continue;
      if (file === fileURLToPath(import.meta.url)) continue;

      // Comments are stripped first, or this guard reports the prose that
      // explains the fix as though it were the defect — which it did, on the
      // very comment describing the old CWD-relative call.
      const source = stripComments(readFileSync(file, 'utf8'));
      const args = dotenvConfigArgs(source);
      if (args === null) continue;

      // A bare relative literal is the original defect, verbatim.
      if (/path\s*:\s*['"`]\.?\.?\/?\.env/.test(args)) {
        offenders.push(`${rel} — CWD-relative literal`);
        continue;
      }

      // Anything else must anchor on this file's own location.
      if (!/__dirname|import\.meta\.url|ROOT_ENV_PATH/.test(args)) {
        offenders.push(`${rel} — path not anchored to import.meta.url`);
        continue;
      }

      // Depth check: frontend/scripts/x.js is 2 below the repo root, so it
      // needs two '..' segments. This is what makes the guard catch a right
      // technique applied at the wrong level.
      if (/__dirname|import\.meta\.url/.test(args)) {
        const depth = rel.split('/').length;
        const ups = (args.match(/['"`]\.\.['"`]/g) || []).length;
        if (ups !== depth) {
          offenders.push(`${rel} — resolves ${ups} level(s) up, needs ${depth}`);
        }
      }
    }

    expect(offenders).toEqual([]);
  });
});
