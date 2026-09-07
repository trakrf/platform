/**
 * The one renderer for the "settings push landed mid-transition" log line.
 *
 * ## Why this line has a module
 *
 * It used to be built inline in `reader.ts`, interpolating the reader state
 * INTO the middle of the sentence:
 *
 *     `[Reader] Settings push arrived while ${this.readerState} - ` +
 *     'waiting for the reader to settle before applying'
 *
 * Under e2e the worker runs in the browser, so the line reaches a captured run
 * log only through `tests/e2e/helpers/console-forwarding.ts`, whose filter is a
 * list of case-sensitive substrings. That made the line's VISIBILITY a function
 * of runtime state:
 *
 *     readerState = Connecting  ->  "...while Connecting - ..."  contains
 *                                   `Connect`, which is a KEEP token   -> kept
 *     readerState = Busy        ->  "...while Busy - ..."       matches
 *                                   no KEEP token                      -> DROPPED
 *
 * Same emitter, same needle list, and whether the event is observable at all
 * decided by which branch the reader happened to be in. A grep returning zero
 * then means either "this never happened" or "it happened every time and was
 * filtered" — and nothing distinguishes them after the fact. That is strictly
 * worse than a dead needle, which at least reads zero honestly. TRA-1253.
 *
 * ## The rule this encodes
 *
 * An allowlist entry must match the event WHENEVER THE EVENT OCCURS, not merely
 * sometimes. The provability guard in
 * `tests/config/every-signal-needle-has-a-producer.test.ts` proves an emitter
 * exists; it cannot prove the emitted line survives the forwarder on every
 * branch. So the invariant text carries the KEEP token and the variable part is
 * appended AFTER it, where no matcher looks.
 *
 * Exported rather than inlined so the emitter and
 * `tests/config/e2e-forwarder-keeps-state-interpolated-lines.test.ts` render the
 * same string. A test that re-typed the template would go on passing after the
 * emitter was reworded, which is the failure mode this whole file is about.
 */

import type { ReaderStateType } from '../types/reader.js';

/**
 * The invariant head of the deferral line — no interpolation, ever.
 *
 * `tests/e2e/helpers/console-forwarding.ts` keeps lines containing this exact
 * text. Changing it without changing that list restores the TRA-1253 defect, so
 * the forwarder test asserts the two agree.
 */
export const SETTLE_DEFERRAL_PREFIX = '[Reader] Settings push deferred';

/**
 * Render the deferral line for a given reader state.
 *
 * The state goes in a trailing parenthetical, so every rendering shares a
 * byte-identical prefix and the forwarder's decision cannot depend on it.
 */
export function settleDeferralMessage(readerState: ReaderStateType): string {
  return (
    `${SETTLE_DEFERRAL_PREFIX} - waiting for the reader to settle ` +
    `before applying (state: ${readerState})`
  );
}
