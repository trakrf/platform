/**
 * Build a searchable body of source text that could actually EMIT a log line.
 *
 * Extracted from `every-signal-needle-has-a-producer.test.ts` (TRA-1226) when a
 * second table — the e2e console allowlist — needed the same question asked of
 * it (TRA-1224). One implementation rather than two, because the subtle parts
 * are the parts a copy would get wrong: comments are not producers, and the
 * table under test must never be in its own haystack.
 *
 * The two callers pass DIFFERENT roots, and that difference is the whole point.
 * A soak-driver needle can be emitted by anything the driver runs — src, tests
 * or scripts. A browser-console allowlist entry can only be emitted by code the
 * BROWSER loads, which is `src/` alone. Handing the console guard the wider tree
 * would let a spec that merely passes the string as an option count as proof
 * that something emits it.
 */

import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';

/**
 * Strip comments, because a needle mentioned in prose is not a producer.
 *
 * This is not tidiness — it is the defect being guarded against, one level down.
 * The dead `'Device busy'` retry arm looked live to a plain grep precisely
 * because every occurrence of the string in the shipped bundle was inside a
 * comment explaining it. A check that counts prose as evidence confirms itself
 * on documentation of the very thing that is missing.
 *
 * Verified by execution rather than assumed: `transportUnreachable` passed the
 * first version of this check on two comment hits and nothing else.
 */
export function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/[^\n]*/g, '$1');
}

/** Every source file under `dir` that could plausibly carry a log line. */
export function collectSources(dir: string, acc: string[] = []): string[] {
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return acc;
  }
  for (const entry of entries) {
    if (entry === 'node_modules' || entry === 'dist' || entry.startsWith('.')) continue;
    const full = path.join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) {
      collectSources(full, acc);
    } else if (/\.(ts|tsx|mjs|js)$/.test(entry)) {
      acc.push(full);
    }
  }
  return acc;
}

/**
 * Concatenated, comment-stripped source of everything under `roots` that could
 * emit a line, minus the files named in `exclude`.
 *
 * `exclude` is not an optimisation. Every entry in it is a file that CONTAINS
 * the strings being checked without being able to emit them — the table itself,
 * a consumer that passes them as options, a test that builds them as fixtures.
 * Leaving one in makes the guard satisfiable by the thing it is guarding, which
 * is the failure this whole mechanism exists to prevent.
 *
 * @param roots    absolute directories to scan
 * @param exclude  predicate returning true for a file that must not count as a producer
 */
export function buildHaystack(roots: string[], exclude: (file: string) => boolean = () => false) {
  const files = roots.flatMap((root) => collectSources(root));
  return files
    .filter((f) => !exclude(f))
    .map((f) => stripComments(readFileSync(f, 'utf8')))
    .join('\n');
}
