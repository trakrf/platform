/**
 * Which browser console lines the e2e run forwards to its own stdout.
 *
 * ## Why this is its own module
 *
 * It used to be an inline `if` inside `connectToDevice`, which meant the rule
 * could only be exercised by running Playwright against real hardware. It was
 * wrong for weeks and nothing could have caught it (TRA-1209). A predicate in a
 * module with no imports is unit-testable, and
 * `tests/config/e2e-console-forwarding.test.ts` now checks it against every
 * browser-emitted needle the soak instruments count.
 *
 * Keep this file dependency-free. It is imported by a vitest suite that must not
 * pull in Playwright or the e2e config's top-level side effects.
 *
 * ## Why this filter matters beyond debugging noise
 *
 * Under e2e the application runs in the browser, so ANY line the soak signal
 * needles grep for is emitted there and reaches the captured run log only
 * through this predicate. A needle whose line is filtered out here does not read
 * as "filtered" — it reads as a confident `0`, which is indistinguishable from
 * "the reader never did that".
 *
 * ## Widening this is a measurement change
 *
 * Every extra line lands in the log that every signal count is computed from,
 * and `harnessLines`-style canaries count lines. Add a specific prefix rather
 * than loosening an existing limb: making `Connect` case-insensitive would sweep
 * in `[vite] connected`, every `connecting…`, and a great deal else.
 */

/** Emitted by `src/lib/device/transport/cs108-ble-transport.ts` via console.info. */
const BLE_TIMING_PREFIX = '[ble-timing]';

/**
 * Emitted by `src/stores/tagStore.ts`. Both spellings are live: the auth
 * subscription logs `[TagStore]`, the stale-enrichment canary logs `[tagStore]`.
 * Matching only one would drop half the path (TRA-1191).
 */
const TAG_STORE_PREFIXES = ['[TagStore]', '[tagStore]'];

/**
 * Emitted by `src/lib/auth/orgContext.ts`. Every tag lookup awaits
 * `ensureOrgContext()`, so a token/profile disagreement there is a step on the
 * enrichment path rather than a separate concern (TRA-1191).
 */
const ORG_CONTEXT_PREFIX = '[OrgContext]';

/**
 * Emitted by `src/lib/cache/orgScopedCache.ts` and `src/stores/authStore.ts`.
 * The org-scoped invalidation clears the tag store, and the auth store is what
 * calls it on login — so these lines are not adjacent to the enrichment path,
 * they are the step that decides whether there is anything left to enrich
 * (TRA-1191).
 */
const ORG_CACHE_PREFIXES = ['[OrgCache]', '[AuthStore]'];

/**
 * Substrings that mark a line as worth keeping.
 *
 * CASE-SENSITIVE, and deliberately so — that is what keeps the list narrow. It
 * is also exactly what broke: `[ble-timing] connect` matches neither `BLE` nor
 * `Connect`, and `disconnect` is not a substring of any of the three timing
 * lines. Both halves of that were true, so either one alone would have hidden
 * the other.
 */
const KEEP = [
  BLE_TIMING_PREFIX,
  ...TAG_STORE_PREFIXES,
  ORG_CONTEXT_PREFIX,
  ...ORG_CACHE_PREFIXES,
  'Error',
  'Failed',
  'BLE',
  'Connect',
  'WebSocket',
  'force',
  'cleanup',
  'disconnect',
];

/**
 * Should this browser console line be echoed into the run's captured output?
 *
 * `type` is Playwright's console message type; anything of type `error` is kept
 * regardless of its text.
 */
export function shouldForwardConsoleLine(text: string, type: string): boolean {
  if (type === 'error') return true;
  return KEEP.some((needle) => text.includes(needle));
}
