# ADR 0016 — Gate an injected stimulus on the precondition that makes it meaningful

Date: 2026-09-03
Status: Proposed
Tracking: TRA-1245 (this change and the arm that paid for it), TRA-1080 (the first time this was diagnosed and then forgotten), TRA-1224 (the e2e failures this is one of), TRA-1247 (the mechanism, measured)

## Context

E2E tests drive the CS108's trigger by injecting a real trigger-press notification
through the mock's testing API. That is not a shortcut past the product's path — the
notification is the *entire* interface through which the worker can learn a press
happened, so injecting it exercises packet parse, notification routing, the trigger
handler, the worker state machine and the stores exactly as real hardware does.

The product relies on the trigger LEVEL, not just the edge. `reader.ts:229` acts on an
edge only when the reader is in the state that can act on it — `CONNECTED` for a press,
`SCANNING` for a release — and drops it with a `logger.debug` otherwise. Dropping is
correct: queueing work for edges the operator has already moved past is worse than
discarding them, and it is safe *because `convergeToTriggerState()` reconciles the level
once the reader settles*.

So the question that decides what a simulated trigger can test is: **how long does an
injected level survive?**

### It survives time. It does not survive a mode change.

Nothing decays a level. `triggerState` is a host-side latch written in exactly two
places — `reader.ts:179`, from a `TRIGGER_STATE_CHANGED` event, and to `false` on
disconnect — with no timer between them. The device pushes nothing unbidden: auto-reporting
(`0xA002`, `0xA003`, `0xA008`, `0xA009`) is off by decision, see ADR 0019.

What revokes it is **our own poll**:

1. `buildModeSequences()` prefixes `IDLE_SEQUENCE` to EVERY mode.
2. `IDLE_SEQUENCE` sends `GET_TRIGGER_STATE` (`0xA001`). The device answers in ~22 ms
   with the real switch position — measured on the wire, and specified in the vendor
   byte-stream API §10.1 as `0 = Released; 1 = Pushed`.
3. `CommandManager.handleCommandResponse()` forwards `0xA000` and `0xA001` replies to
   the notification handler as well as settling the command in flight
   (`command.ts:374-386`), so the answer reaches `TriggerStateHandler`.
4. That emits `TRIGGER_STATE_CHANGED`, and `reader.ts:179` overwrites the latch with
   what the device just said.

For an injected press the device's answer is "released", truthfully, because no switch
is held. A real finger survives the same poll.

| an injected level across… | survives? |
|---|---|
| **time** | **yes** — nothing decays it |
| a **non-mode-change BUSY** (a settings push, a start sequence) | **yes** — no poll runs |
| a **MODE CHANGE** | **no** — the poll re-reads the device and the device wins |
| the **device's own** firmware reaction to a physical trigger (e.g. `0xA004` abort-on-release) | **not even partially** — it never reaches the host |

Measured rather than argued:

- `frontend/tests/e2e/trigger-level-is-reread-on-mode-change.spec.ts` injects a press,
  holds it three seconds to show time revokes nothing, changes tab, and asserts the
  level goes false with **no release injected**.
- `frontend/src/worker/cs108/reader.test.ts`, under `a trigger notification arriving
  while BUSY`, holds a latched level through a BUSY reader past 750 ms and watches
  convergence act on it when the reader settles.
- `frontend/src/worker/cs108/command-contract.test.ts` pins the `0xA000`/`0xA001`
  forwarding that step 3 turns on, with the negative case so it cannot pass by
  forwarding everything.

So a test that injects a press into a reader mid-MODE-CHANGE is not testing a degraded
version of the real behaviour. It is testing a scenario that cannot occur with real
hardware, and asserting the product should recover from it.

### What that cost, twice

**2026-07-30, TRA-1080.** The worker logging `Trigger pressed ignored - reader state is
Busy` was read as a dropped-press bug. It was not; press-and-hold, including
press-during-init, was confirmed working on hardware.

**2026-09-02, TRA-1245.** A 200-rep arm measured `locate.spec.ts` failing 48/200
(24.0%). Every failure showed `readerState: Busy` / `status: Idle` for the whole sample
window, with the transport and the device provably clean underneath — 27,211 writes all
under 100 ms, zero link closes, and 11 unanswered commands, all ABORTs that the retry
recovered. That Busy is the LOCATE mode change, whose poll revoked the press it had just
dropped. The failure was filed as a product defect in the reader's state machine,
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

**Gate an injected stimulus on the precondition that makes it MEANINGFUL, and fail
loudly where that precondition cannot be established.**

Not on the state that would honour it. Where the two coincide the gate is the same;
where they do not, gating on the honouring state fails correct tests for reasons that
have nothing to do with the product.

Concretely, for the trigger:

1. The state that honours each action is named once, next to the product line that
   defines it — `STATE_THAT_HONOURS` in `tests/e2e/helpers/trigger-utils.ts`.
2. **A press** is meaningful only in `CONNECTED`. Every other state drops the edge, and
   when that state is a mode change the same mode change's poll then revokes the level,
   so convergence finds nothing held. `simulateTrigger` waits for `CONNECTED`
   **immediately before injecting**, with nothing in between, and throws if it never
   arrives. A check followed by a sleep gates nothing: `gotoLocateWithEPC` waited for
   `CONNECTED` and then slept 250 ms for the trigger debounce, and the reader re-entered
   `BUSY` inside that gap in 24% of reps.
3. **A release** is meaningful only when there is a scan to stop, so its precondition is
   "is that work in flight" — and the answer "no" is a pass, not a failure. Waiting for
   `SCANNING` there waits for a state that will never arrive:

   | spec | why `SCANNING` is unreachable |
   |---|---|
   | `connection.spec.ts:130` | asserts trigger-state propagation on the Settings tab and deliberately never starts a scan |
   | `inventory.spec.ts:111` | a `beforeEach` defensive release, run before anything has been pressed |
   | `locate.spec.ts` `afterAll` | releases after `goto('/')`, against a reader that is already `Disconnected` |

   The first two failed 2 of 2 reps under an honouring-state gate. The third burned 15 s
   and threw into a `catch` on 101 of 101 reps, which also skipped the
   `disconnectDevice()` that followed it — green throughout, and defective throughout.
4. **A transient state is not an answer.** `BUSY` and `CONNECTING` could still resolve
   either way, so the gate settles first and then decides. Only a reader that never
   leaves a transient state throws, which is the wedge the 15 s budget was always for.
5. **The gate lives in the shared helper, not the call sites.** Six specs press triggers
   through this path; the others have not lost the coin flip yet.

## Consequences

**A red in a trigger test now means the product.** That is the point, and it is what
makes the remaining reds worth the hardware time to chase.

**Some behaviour is not testable here, and that is the honest outcome.** A level held
across a MODE CHANGE, and anything the reader's own firmware does in response to a
physical trigger — `0xA004`'s abort-on-release, which never reaches the host at all —
need a real finger. A test that appears to cover them is worse than no test, because it
reports a result. Where that matters, say so on the ticket rather than approximating it.

**A level held WITHIN a mode is testable**, and should be tested rather than deferred to
the bench. That is most of what convergence does.

**Precondition failures must be loud.** The general form, which is what generalises past
the trigger: a helper that detects it cannot deliver its stimulus correctly must fail
where it detected that, not hand a doomed run to the next assertion. Throwing rather
than warning is deliberate — injecting into a dropping state produces a failure several
assertions downstream, which reads as a broken product rather than an edge delivered
into the wrong state.

**A gate verified through one call site is evidence about that call site.** The press
gate was measured on `locate.spec.ts` at 0/101 and read as evidence about the shared
helper. `locate` is the one spec whose usage satisfies the press precondition by
construction, and therefore the one spec structurally incapable of detecting an
over-constraint in it. Where a change lands in a shared helper, the arm has to cross the
call sites that use it differently. One helper reaches six specs, so a gate that is
wrong for one shape of caller is wrong six times at once — that is the price of point 5,
not an argument against it.

**This is a rule about test design, not about the reader.** It applies to any injected
stimulus standing in for sustained physical state.
