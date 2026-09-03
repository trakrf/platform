# ADR 0016 — A simulated stimulus tests the edge it injects, not the level it cannot hold

Date: 2026-09-03
Status: Proposed
Tracking: TRA-1245 (this change and the arm that paid for it), TRA-1080 (the first time this was diagnosed and then forgotten), TRA-1224 (the e2e failures this is one of)

## Context

E2E tests drive the CS108's trigger by injecting a real trigger-press notification
through the mock's testing API. That is not a shortcut past the product's path — the
notification is the *entire* interface through which the worker can learn a press
happened, so injecting it exercises packet parse, notification routing, the trigger
handler, the worker state machine and the stores exactly as real hardware does.

From which a limitation follows **by construction rather than by observation**:

| | driven by | simulatable? |
|---|---|---|
| **Edge** (a press/release happened) | one notification event | yes, completely |
| **Level** (still held N seconds later) | sustained hardware state, re-asserted by ongoing reader notifications | **no** — one injected event, and the real hardware keeps reporting "not pressed" |

The injected trigger state reverts to `false` roughly 500ms later, because no physical
switch is held and the device's own notifications win.

This matters because the product deliberately relies on the level. `reader.ts:229`
acts on a trigger edge only when the reader is in the state that can act on it —
`CONNECTED` for a press, `SCANNING` for a release — and drops it with a `logger.debug`
otherwise. Dropping is correct, and `convergeToTriggerState()` documents why: queueing
work for edges the operator has already moved past is worse than dropping them, and
dropping is safe *because the level is reconciled once the reader settles*. A real
thumb is still on the trigger at that moment. An injected one is long gone.

So a test that injects a press into a busy reader is not testing a degraded version of
the real behaviour. It is testing a scenario that **cannot occur with real hardware**,
and asserting the product should recover from it.

### What that cost, twice

**2026-07-30, TRA-1080.** The worker logging `Trigger pressed ignored - reader state is
Busy` was read as a dropped-press bug. It was not; press-and-hold, including
press-during-init, was confirmed working on hardware.

**2026-09-02, TRA-1245.** A 200-rep arm measured `locate.spec.ts` failing 48/200
(24.0%). Every failure showed `readerState: Busy` / `status: Idle` for the whole sample
window, with the transport and the device provably clean underneath — 27,211 writes all
under 100ms, zero link closes, and 11 unanswered commands, all ABORTs that the retry
recovered. The failure was filed as a product defect in the reader's state machine,
against the wrong call site, and had to be withdrawn.

Between those two, the spec grew a comment asserting the opposite of the truth — that
the flake was "the product's, not this test's" — and a deliberate no-retry policy
justified by preserving a measurement of "how often a real press lands on a
non-CONNECTED state". An injected press is not a real press, so what that preserved was
a measurement of the harness's own timing, presented as a product signal.

The failure mode is not that the test is flaky. It is that **the test manufactures a red
state the product cannot be responsible for, and the red is indistinguishable from a
real defect** — so it consumes triage, hardware time, and eventually a wrongly-filed
ticket. It also decays the other direction: a suite with a known-flaky test teaches
people to discount reds.

## Decision

**A simulated stimulus may be used to assert behaviour that depends only on the edge it
injects. Where the behaviour depends on a level the simulation cannot sustain, the test
must establish the precondition that makes the edge sufficient, and fail loudly when it
cannot.**

Concretely, for the trigger:

1. The state that honours each action is named once, next to the product line that
   defines it — `STATE_THAT_HONOURS` in `tests/e2e/helpers/trigger-utils.ts`.
2. `simulateTrigger` waits for that state **immediately before injecting**, with
   nothing in between. A check followed by a sleep gates nothing: `gotoLocateWithEPC`
   waited for `CONNECTED` and then slept 250ms for the trigger debounce, and the reader
   re-entered `BUSY` inside that gap in 24% of reps.
3. Failing to reach that state **throws**, naming the state actually observed. It does
   not warn and continue. A helper that proceeds into a stimulus it knows will be
   discarded converts a precondition failure into an assertion failure somewhere else,
   which is how this arrived as "gauge should report dBm" rather than "the press was
   delivered too early".
4. The gate lives in the shared helper, not the call sites. Six specs press triggers
   through this path; the others have not lost the coin flip yet.

## Consequences

**A red in a trigger test now means the product.** That is the point, and it is what
makes the remaining reds worth the hardware time to chase.

**Some behaviour is not testable here, and that is the honest outcome.** Anything
requiring a trigger genuinely held across a state transition — including the device's
own firmware reaction to a real release, which never reaches the host at all — needs a
real thumb. A test that appears to cover it is worse than no test, because it reports a
result. Where that matters, say so on the ticket rather than approximating it.

**Precondition failures must be loud.** The general form, which is what generalises past
the trigger: a helper that detects it cannot deliver its stimulus correctly must fail
where it detected that, not hand a doomed run to the next assertion.

**This is a rule about test design, not about the reader.** It applies to any injected
stimulus standing in for sustained physical state.
