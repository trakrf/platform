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
 * The three `command.ts` lines the soak instruments count, and nothing else.
 *
 * Added by TRA-1253 to close a HALF-COUNT, which is worse than a dropped needle
 * because the number it produces looks plausible. One `CommandInFlightError`
 * occurrence emits two lines:
 *
 *   WARN  [CommandManager] RFID_POWER_OFF (0x8001) went unanswered after 2
 *         attempt(s): Command already active ... — tolerated, continuing the
 *         sequence                                              <- was DROPPED
 *   ERROR [setMode] Failed to set Idle mode: CommandInFlightError:
 *         Command already active - executeCommand called concurrently  <- kept
 *
 * The ERROR line carries `Failed`; the WARN line carried no KEEP token at all,
 * so an e2e arm returned roughly half the true value with no indication.
 * `powerOffTimeouts` and `toleratedPowerOffs` were dropped for the identical
 * reason — both are `logger.warn`.
 *
 * ⚠ NEEDLE TEXTS, NOT THE `[CommandManager]` PREFIX, and the difference was
 * settled by measurement rather than taste. The first version of this fix kept
 * the whole prefix. On the 2026-09-07 hardware arm that admitted 94
 * `[CommandManager]` lines where the previous arm had 49 — and 90 of the 94
 * were routine `logger.debug` chatter ('Response received: ...', 'Applying
 * 200ms settling delay'). Paying ~90 debug lines per run to make three WARN
 * needles visible is the wrong trade in a log every signal count is computed
 * from, and this file's own rule at the top says to add a SPECIFIC prefix
 * rather than loosen. These three strings cost nothing on a clean run and
 * capture exactly the lines that have a named consumer.
 *
 * Each entry must correspond to a needle in `scripts/suite-run-signals.mjs`:
 *   'Command timeout:'                  -> powerOffTimeouts, countCommandTimeouts
 *   'tolerated, continuing the sequence'-> toleratedPowerOffs, and the WARN half
 *                                          of a commandInFlight occurrence
 *   'Command already active'            -> commandInFlight, both halves
 */
const COMMAND_SIGNAL_NEEDLES = [
  'Command timeout:',
  'tolerated, continuing the sequence',
  'Command already active',
];

/**
 * Emitted by `src/worker/cs108/settle-deferral-message.ts`.
 *
 * Kept as an explicit prefix rather than relying on the sentence's words,
 * because the words used to include the reader state — so the line forwarded
 * when the state was `Connecting` and vanished when it was `Busy`. An entry must
 * match the event WHENEVER the event occurs, not merely sometimes; the invariant
 * prefix is what makes that provable, and
 * `tests/config/e2e-forwarder-keeps-state-interpolated-lines.test.ts` proves it
 * across every member of `ReaderState`. TRA-1253.
 */
const SETTLE_DEFERRAL_PREFIX = '[Reader] Settings push deferred';

/**
 * Emitted by `src/stores/deviceStore.ts` via console.warn — TRA-1259's signature,
 * the store leaving an established state for DISCONNECTED.
 *
 * A PREFIX, and the reason is the same one this file already learned the hard
 * way. The line reads `Reader lost CONNECTED: <prev> -> Disconnected.`, and
 * nothing in its constant part matched any KEEP limb: `CONNECTED` is not
 * `Connect`, `Disconnected` is not `disconnect`. So the only thing deciding
 * whether an occurrence survived was which state name got interpolated in —
 * `Connected` and `Error` matched by accident, `Configuring`, `Busy` and
 * `Scanning` did not. Three of five dropped, and the dropped three include the
 * one a trigger-hold sweep produces.
 *
 * It is `console.warn`, so Playwright's type is `warning` and the
 * `type === 'error'` short-circuit below never covered it.
 *
 * An entry must match the event WHENEVER the event occurs, not merely sometimes.
 * The invariant prefix is what makes that true here, exactly as it did for
 * `SETTLE_DEFERRAL_PREFIX` above.
 */
const LOST_CONNECTED_PREFIX = '[DeviceStore] Reader lost CONNECTED';

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
  ...COMMAND_SIGNAL_NEEDLES,
  SETTLE_DEFERRAL_PREFIX,
  LOST_CONNECTED_PREFIX,
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
