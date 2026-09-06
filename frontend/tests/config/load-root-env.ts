/**
 * The single reader for the repo-root `.env.local` (TRA-1195).
 *
 * This exists because `.env.local` was loaded in three places —
 * `playwright.config.ts`, `vite-bridge.config.ts` and `ble-bridge.config.ts` —
 * each calling `dotenv.config({ path: '.env.local' })`. dotenv resolves `path`
 * against `process.cwd()`, and both playwright and vite run from `frontend/`,
 * so all three looked for `frontend/.env.local`. That file has never existed;
 * the real one is at the repo root.
 *
 * dotenv does not throw on a missing file. It returns an error object nobody
 * reads and loads nothing, so all three calls were dead code that read as
 * configuration — the comment above them said "Load environment variables from
 * .env.local", the line looked like it did that, and it never once did.
 *
 * The reason nobody noticed is the part worth keeping: direnv loads the root
 * `.env.local` into the shell (`.envrc: dotenv_if_exists .env.local`) and
 * traverses upward, so `cd frontend` still finds it. The values were already in
 * `process.env` before the process started. That masks the defect everywhere a
 * human works and exposes it everywhere else — `bash -c`, CI, agent harnesses,
 * and any fresh clone before `direnv allow`. Nothing at runtime can tell those
 * two situations apart, which is why "we already depend on direnv's traverse-up"
 * is the bug rather than the fix.
 *
 * Resolution is from `import.meta.url`, not from the CWD and not by searching
 * upward for a marker file. The monorepo layout is fixed — this file is at
 * `frontend/tests/config/`, so the root is three levels up — and a search would
 * buy nothing while adding a way to find the wrong root. `vite.config.ts`
 * already does exactly this with `path.dirname(fileURLToPath(import.meta.url))`,
 * which is why vite was never affected: it resolves `envDir: '../'` itself.
 *
 * One reader, so the next change cannot be partially applied. The guard in
 * `root-env-loads-from-repo-root.test.ts` fails if a second caller appears.
 */

import * as dotenv from 'dotenv';
import { dirname, join, resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * The repo root, derived from this file's own location.
 *
 * `frontend/tests/config/` -> three levels up. Exported so the guard can assert
 * the derivation rather than trusting it.
 */
export function repoRoot(): string {
  return resolve(__dirname, '../../..');
}

/** Absolute path to the one `.env.local` this repo has. */
export const ROOT_ENV_PATH = join(repoRoot(), '.env.local');

/**
 * Load the repo-root `.env.local` into `process.env`.
 *
 * Deliberately does not throw when the file is absent. A fresh clone with no
 * `.env.local` is a legitimate state, and so is CI, where the values arrive as
 * real environment variables instead. What matters is that a value which IS on
 * disk is actually read — the failure this fixes was silence about a file that
 * was never opened, not silence about a file that does not exist.
 *
 * Callers that genuinely require a key should say so themselves, naming the key
 * and this path; `assert-preconditions.ts` does that for the e2e run.
 */
export function loadRootEnv(): void {
  dotenv.config({ path: ROOT_ENV_PATH });
}
