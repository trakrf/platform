/**
 * The one place TRA-1259's signature line is rendered.
 *
 * ## Why this is a module rather than a template literal in the store
 *
 * The line interpolates the previous reader state into itself, and the e2e
 * console forwarder is a list of case-sensitive substrings. When the warning
 * shipped in #670 the interpolation was the ONLY thing deciding whether an
 * occurrence reached a captured run log:
 *
 *   | prev state  | rendered line contains | forwarded? |
 *   | Configuring | no KEEP token          | NO         |
 *   | Connected   | `Connect`              | yes        |
 *   | Busy        | no KEEP token          | NO         |
 *   | Scanning    | no KEEP token          | NO         |
 *   | Error       | `Error`                | yes        |
 *
 * Three of five dropped, including `Scanning -> Disconnected` — the transition a
 * trigger-hold sweep produces, and therefore the one TRA-1259 most needs to see.
 * Nothing in the constant part of the sentence matched: `CONNECTED` is not
 * `Connect`, `Disconnected` is not `disconnect`. It is `console.warn`, so the
 * forwarder's `type === 'error'` short-circuit did not cover it either.
 *
 * That is the TRA-1209 class exactly, recurring one PR after the needle it was
 * meant to complete, and it is why the prefix is now invariant and asserted:
 * `tests/config/e2e-forwarder-keeps-state-interpolated-lines.test.ts` renders
 * this function for EVERY member of `ReaderState` and requires the forwarder to
 * keep all of them. Re-typing the sentence in the test would let the emitter be
 * reworded out from under it, so the test calls this renderer instead.
 *
 * ⚠ Keep every KEEP token in the INVARIANT prefix. Moving the state name back
 * into the matched region reintroduces the defect and the prefix-equality
 * assertion in that suite is what will catch it.
 */

import type { ReaderStateType } from '@/worker/types/reader';

/**
 * The invariant head of the line — matched verbatim by the e2e console
 * forwarder's `LOST_CONNECTED_PREFIX` and counted by
 * `scripts/suite-run-signals.mjs`'s `lostConnected` needle.
 *
 * The literal is duplicated in `tests/e2e/helpers/console-forwarding.ts` on
 * purpose: that module is deliberately dependency-free so a vitest suite can
 * import it without pulling in Playwright or the app. The two copies are held in
 * agreement by an assertion rather than by an import.
 */
export const LOST_CONNECTED_PREFIX = '[DeviceStore] Reader lost CONNECTED';

/**
 * Render the warning for a transition out of an established state into
 * DISCONNECTED.
 *
 * @param prevState the state the reader is leaving — interpolated AFTER the
 *   prefix, never inside it.
 */
export function lostConnectedMessage(prevState: ReaderStateType): string {
  return (
    `${LOST_CONNECTED_PREFIX}: ${prevState} -> Disconnected. ` +
    'Trace names the caller — TRA-1259.'
  );
}
