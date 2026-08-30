# ADR 0011 — A device timing constraint belongs to the command layer, not to the caller

Date: 2026-08-30
Status: Proposed
Tracking: TRA-1197 (this change), TRA-1185 (the defect that produced it), TRA-1143 and TRA-1154 (the same pass, for the reason in Consequences)

## Context

The CS108 vendor specification states hard timing requirements between commands.
The one that produced this ADR, from
`CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.pdf` p.106:

> *"After the 'ABORT' command to stop inventory, a 2 seconds delay is required
> for the reader to clear buffer before it can execute another command"*

That requirement is about **the device's readiness to accept the next command**.
It says nothing about anyone waiting for anything.

`reader.ts` implemented it in the caller, and got it wrong in both directions at
once:

```js
await this.commandManager.executeSequence(RFID_STOP_SEQUENCE);

const packetMonitorStart = Date.now();
const maxWaitTime = 2000;                                  // 2 seconds per API documentation

await new Promise(resolve => setTimeout(resolve, 1000));   // Initial wait

const elapsedTime = Date.now() - packetMonitorStart;
if (elapsedTime >= maxWaitTime) {                          // can never be true
  // ...forced RFID_POWER_OFF recovery
}
```

- **Too slow where it did not matter.** The UI blocked for a second on every
  inventory and locate stop. This was the reported symptom: *"our locate stop is
  probably the weakest link right now — i've noticed that the scan stops ~1 sec
  after i release trigger."*
- **Too fast where it did.** The next command could be dispatched roughly one
  second into a reader the vendor says needs two, which presents as intermittent
  post-stop command failure rather than as a timing bug.
- **And the recovery could not fire.** `packetMonitorStart` is set immediately
  before the sleep meant to run it down, so `elapsedTime` is always ~1000 against
  a `>= 2000` test. A control that cannot go red reads as a control.

The citation was accurate and the code still did half of it. That is the tell:
the constraint was written down in the wrong place, so the number and its
enforcement drifted apart with nothing to hold them together.

**This is the third shape the same idea has taken in this codebase.**
`CS108Event.settlingDelay` waits after a success, inside the settle path.
`SequenceCommand.delay` waits after a step, blocking the sequence and therefore
its caller. The stop path added a third by hand. Three attempts, three
semantics, none of them the one the vendor text describes — which is why this is
an ADR and not a comment on a function.

## Decision

**A timing constraint imposed by the device is declared on the command that
triggers it and enforced by `CommandManager`, which charges it to the next
dispatch. Callers never sleep for the hardware.**

Concretely:

- `SequenceCommand.quietPeriodAfter` declares how long the device cannot accept
  another command after this one. `RFID_STOP_SEQUENCE` declares the vendor's
  2000 ms.
- `CommandManager` holds a single deadline and blocks the **next** dispatch —
  from any caller, in any sequence — until it passes. The caller that armed the
  window is not delayed by it, so the operator is told the scan stopped as soon
  as the ABORT is acknowledged.
- The window is armed **at send**, not at ack. The vendor text is worded "after
  the ABORT command", so the ack round trip is time the reader has already spent
  clearing. Charging it to the window is both faithful to the wording and the
  cheaper reading; the alternative is defensible and is why this sentence exists
  rather than being left to inference.
- The three wait concepts are **not interchangeable and must not be collapsed**:
  `settlingDelay` delays this command's own resolution; `delay` delays the
  sequence, and its caller with it; `quietPeriodAfter` delays nobody and gates
  the wire. Each is documented at its declaration.

**A constraint is not honoured by a doc comment.** If a new vendor requirement
appears, it gets a declaration and an enforcement point, or it is not enforced —
citing a figure next to code that does something else is what this ADR exists to
stop.

**Do not substitute an observable for a duration.** Monitoring the notification
stream until it goes quiet was proposed and rejected: packets ceasing is not the
buffer clearing. The spec gives a fixed duration, and stream silence cannot
establish buffer state. Watching the stream is a reasonable *additional* check
that the abort took effect; it is not a replacement for the interval.

## Consequences

- The perceptible ~1 s delay on every stop is gone, and the vendor's interval is
  honoured in full for the first time.
- **A restart after a stop is now slower on purpose.** A held trigger reconciles
  by restarting, and that restart waits the full 2 s rather than the old 1 s.
  This is the requirement being met rather than under-waited, but it is a real
  change in feel and wants a judgement on a reader.
- The dead `RFID_POWER_OFF` recovery was deleted rather than repaired. Restoring
  it needs a genuine observation of streaming packets, which is separate work.
  Deleting it removes nothing that worked.
- **This decision depends on the queue landing in the same pass.** A quiet window
  charged to the next dispatch is only coherent if there is a single ordered
  dispatch point to charge it to. While `CommandManager` threw at a concurrent
  caller instead of queueing it, "the next command" was not a well-defined thing
  — which is a large part of why the constraint ended up in the caller in the
  first place. Anyone reverting the queue must revisit this.
- An abort raised while a dispatch is waiting out a window is reported when the
  window expires rather than immediately. The wait is a hardware constraint and
  is not skippable, so the alternative would be reporting an abort the hardware
  has not yet honoured.

## Alternatives considered and rejected

Both were raised in review of PR #621 and rejected deliberately. Recorded so
they are not re-opened without the thing that would change the answer.

**Scoping the window per module, so a barcode command could be issued inside an
RFID module's quiet window.** The CS108 addresses distinct modules (RFID `0xC2`,
barcode `0x6A`), the ABORT goes to the RFID module, and the buffer being cleared
is plausibly the R2000's rather than anything shared — so the narrowing is
defensible on the hardware's architecture. It was rejected because the spec says
"another command" unqualified, and "the reader" is ambiguous between the R2000
and the CS108 host MCU; if what drains is the host's uplink buffer, it is shared
across modules and the narrowing is wrong. That cannot be settled from the text.

⚠ **A second reason was given here originally and it was WRONG — corrected
2026-08-30, same day.** It read: *"it buys nothing, because `buildModeSequences()`
prefixes `IDLE_SEQUENCE` to every mode and `IDLE_SEQUENCE` opens with
`RFID_POWER_OFF`, so the first command after an ABORT is an RFID command whatever
mode is being entered."* That is true of the **mode-change** path and was
generalised from it without checking the others. On a **trigger cycle within one
mode** — press, release, press — no mode change happens, nothing is powered off,
and the command waiting on the window is a plain `START_INVENTORY`. Mike caught
it by asking the obvious question the analysis had not: *"is it really 2s per
trigger cycle? should not be. as long as we stay in RFID mode the radio stays
on."*

Recorded rather than quietly deleted, because the failure was reasoning from one
code path to "always" — and this document is partly about controls that cannot
go red. An argument checked against a single path is the same shape.

**A quiet window between `RFID_POWER_OFF` and `RFID_POWER_ON`, for the
mode → barcode → mode round trip.** Rejected for lack of any basis and against
direct evidence. The ABORT note is the *only* inter-command timing requirement
in the entire byte-stream API specification — one sentence, printed twice; the
200 ms and 500 ms `settlingDelay` values on the power events are empirical and
uncited. More conclusively, that off→on gap already occurs inside **every** mode
entry: `INVENTORY_CONFIG_SEQUENCE` and `LOCATE_CONFIG_SEQUENCE` each open with
`RFID_POWER_ON`, appended straight after `IDLE_SEQUENCE`'s `RFID_POWER_OFF` and
separated only by that 200 ms and three command round trips. If a power cycle
needed seconds, every mode change would be broken, including the hundreds
exercised per soak arm. They are not.

## The cost of honouring it literally, accepted with eyes open

The window gates **every** next command, so a trigger cycle in a scanning mode
costs ~2 s before the reader restarts. That is not hypothetical: cycling the
trigger on hardware produces a steady ~2.0 s cadence, two writes per cycle,
visible directly in the `[ble-timing] write-ack` timestamps.

⚠ **This is slower than what it replaced.** The old stop path slept 1000 ms in
the caller, so a cycle was already gated — at half this. Moving the wait off the
caller made the *stop* feel immediate, which was the reported symptom, and made
the *restart* twice as slow. Stops better, cycling worse.

It is honoured anyway, because the note is explicit and unqualified and we do not
have evidence to narrow it. What makes that safe rather than merely obedient is
that the reader now **converges to the trigger level** when it settles: edges
lost while it catches up are reconciled rather than stranded, so falling behind
the operator degrades responsiveness without losing state.

Worth knowing where the sentence sits, since it bears on how far it should be
stretched: both copies are in **Appendix C, "RFID Reader Firmware Command
Sequence Examples"** — C.2 *Stop Inventory* and C.5 *Stop Searching* — not in the
normative command reference. In both examples the ABORT ends the flow. **The
specification never illustrates an ABORT → START_INVENTORY restart**, which is
precisely the case being argued about. The literal reading is therefore an
extrapolation to a case the document does not cover. That is a reason for
humility about the decision, not a reason to override it.

**What would reopen either:** operators reporting the mode transition, or trigger
cycling, as slow — or a bench measurement of `POST_ABORT_QUIET_MS` at 0 / 1000 /
2000 under hard trigger cycling showing no misbehaviour at the shorter values.
Until then the added latency is covered by the transition spinner, and the
simpler rule — one constraint, on the command the vendor attaches it to — is
the one to keep. Guessing at extra windows costs responsiveness on every mode
change to buy protection against a requirement no document states.
