# ADR 0016 — A simulated stimulus tests the edge it injects, not the level it cannot hold

Date: 2026-09-03
Amended: 2026-09-04 — see "Amendment: the rule is not symmetric across edges"
Amended: 2026-09-05 — see "Amendment: the level the HOST holds is simulatable after all"
Status: Proposed
Tracking: TRA-1245 (this change and the arm that paid for it), TRA-1080 (the first time this was diagnosed and then forgotten), TRA-1224 (the e2e failures this is one of), TRA-1247 (the second amendment, which narrows the claim below)

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

> ⚠ **That sentence and the "no" in the table above are wrong for the HOST-side level,
> and were narrowed on 2026-09-05.** No mechanism for the reversion survives inspection.
> See "Amendment: the level the HOST holds is simulatable after all" at the end. The
> Decision and its consequences are unaffected; what changes is the reason given for
> them, and what remains untestable.

This matters because the product deliberately relies on the level. `reader.ts:229`
acts on a trigger edge only when the reader is in the state that can act on it —
`CONNECTED` for a press, `SCANNING` for a release — and drops it with a `logger.debug`
otherwise. Dropping is correct, and `convergeToTriggerState()` documents why: queueing
work for edges the operator has already moved past is worse than dropping them, and
dropping is safe *because the level is reconciled once the reader settles*. A real
finger is still on the trigger at that moment. An injected one is long gone.

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
requiring a trigger genuinely held across a state transition — ⚠ narrowed 2026-09-05:
the HOST-side level is simulatable, see the second amendment; what follows is now only
the device half — including the device's
own firmware reaction to a real release, which never reaches the host at all — needs a
real finger on a real trigger. A test that appears to cover it is worse than no test,
because it reports a result. Where that matters, say so on the ticket rather than approximating it.

**Precondition failures must be loud.** The general form, which is what generalises past
the trigger: a helper that detects it cannot deliver its stimulus correctly must fail
where it detected that, not hand a doomed run to the next assertion.

**This is a rule about test design, not about the reader.** It applies to any injected
stimulus standing in for sustained physical state.

## Amendment: the rule is not symmetric across edges

*Added 2026-09-04, after the implementation of the Decision above broke three specs
within twelve hours of merging.*

The Decision names the trigger's two edges in one breath — `CONNECTED` for a press,
`SCANNING` for a release — and point 3 says failing to reach that state must throw.
Implemented literally, that is wrong for the release, and the cost was immediate:

| spec | why `SCANNING` is unreachable |
|---|---|
| `connection.spec.ts:130` | asserts trigger-state propagation on the Settings tab and deliberately never starts a scan |
| `inventory.spec.ts:111` | a `beforeEach` defensive release, run before anything has been pressed |
| `locate.spec.ts` `afterAll` | releases after `goto('/')`, against a reader that is already `Disconnected` |

The first two failed 2 of 2 reps. The third burned 15s and threw into a `catch` on 101
of 101 reps, which also skipped the `disconnectDevice()` that followed it — green
throughout, and defective throughout.

**What the two edges do not share is recoverability.** A dropped press loses work that
cannot be re-created: the level is already gone, so there is nothing left to reconcile
and the scan never starts. A dropped release loses nothing, because when no scan is
running there is nothing to stop; `reader.ts:237` drops it with a `logger.debug` and
the world is already in the state the release was asking for.

So the amended rule:

**Gate an injected stimulus on the precondition that makes it MEANINGFUL, not on the
state that would honour it.** Where the two coincide — a press — they are the same
gate. Where the stimulus is a no-op unless some other work is in flight — a release —
the precondition is "is that work in flight", and the answer "no" is a pass, not a
failure. A gate that a caller cannot satisfy by construction is not a strict gate; it
is a wrong one, and it fails a correct test for a reason that has nothing to do with
the product.

Two riders, both learned here:

- **A transient state is not an answer.** `BUSY` and `CONNECTING` could still resolve
  either way, so the gate settles first and then decides. Only a reader that never
  leaves a transient state still throws — which is the wedge the timeout was always
  for.
- **A gate verified through one call site is evidence about that call site.** #647 was
  measured on `locate.spec.ts` at 0/101 and the result was read as evidence about the
  shared helper. `locate` is the one spec whose usage satisfies the new precondition
  by construction, and therefore the one spec structurally incapable of detecting an
  over-constraint in it. Where a change lands in a shared helper, the arm has to cross
  the call sites that use it differently.

The Decision's fourth point — put the gate in the shared helper — stands, and this is
its price rather than an argument against it: one helper reaches six specs, so a gate
that is wrong for one shape of caller is wrong six times at once.

## Amendment: the level the HOST holds is simulatable after all

*Added 2026-09-05 under TRA-1247, which went looking for the mechanism behind the
"reverts to `false` roughly 500ms later" claim and could not find one.*

The Context above gives a mechanism for the reversion: "no physical switch is held and
the device's own notifications win". **The device sends no such notifications.**

- Device auto-reporting is off by decision, not by accident — ADR 0019. The four
  reporting commands (`0xA002`, `0xA003`, `0xA008`, `0xA009`) are defined, mapped, and
  never sent.
- The one poll that could report a level, `GET_TRIGGER_STATE` (`0xA001`) at the head of
  `IDLE_SEQUENCE`, does not answer on the firmware in hand. That is the whole reason
  the level is carried by edges.
- `triggerState` is a host-side latch. It is written in exactly two places: at
  `reader.ts:179` from a `TRIGGER_STATE_CHANGED` event, and to `false` on disconnect.
  There is no timer between them, and the harness injects no release —
  `cs108TriggerReleasePacket` is used only when the caller asks for one.

So nothing revokes an injected level with the passage of time, and the `~500ms` is
almost certainly the confirmation window inside `simulateTrigger` — which polled for
exactly 500ms and then reported `STATE_NOT_UPDATED` — read back as a property of the
device. A measurement of the harness, presented as a mechanism.

Tested rather than asserted: `a trigger notification arriving while BUSY` in
`frontend/src/worker/cs108/reader.test.ts` latches a level through a BUSY reader, holds
it past 750ms, and watches `convergeToTriggerState()` act on it when the reader settles.
`notification/system.test.ts` covers the routing half — nothing between the packet and
the latch consults `readerState`.

**What this narrows.** The table's Level row is "no" only for state the HOST cannot
observe. Host-side, an injected level is exactly as durable as a physical one, and
behaviour that depends on it — convergence after bring-up, a level cycled during BUSY —
is testable. What stays untestable by construction is the device's own firmware reaction
to a physical trigger, which never reaches the host at all: the `0xA004` abort-on-release
is the standing example, and no injected packet can produce it.

**What this does NOT change.** The Decision stands, and so does the gate in
`trigger-utils.ts` — but on its measurement rather than on the story that justified it.
A press injected into a BUSY reader is not lost; it simply starts no scan until
convergence runs at the end of bring-up, which for Locate is up to ~3.7s later
(TRA-1225) — longer than the windows the specs then sample. That is enough to have
produced 48/200 (24.0%) on its own, and waiting for the honouring state still removes it.

**Still unsettled, and it needs hardware.** Whether `0xA001` ever answers is the one
remaining path that could revert a level, and it is a device fact no amount of reading
settles. The experiment is on `system/sequences.ts`: send `0xA008` during bring-up, then
`0xA001`. Until that runs, cite this amendment for the host-side level and not for the
device.
