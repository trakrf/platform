# ADR 0019 — Periodic device state is polled by the host, not pushed by the device

Date: 2026-09-05
Status: Proposed
Tracking: TRA-1247 (the session that recovered this decision). The decision itself
predates the ADR practice and has no originating ticket — that is why it is being
written down a fault at a time rather than at the moment it was made.

## Context

The CS108 offers to report state on its own initiative. Four commands exist for
it, all defined and op-code-mapped in `frontend/src/worker/cs108/event.ts`:

| command | op code |
| --- | --- |
| `START_BATTERY_REPORTING` | `0xA002` |
| `STOP_BATTERY_REPORTING` | `0xA003` |
| `START_TRIGGER_REPORTING` | `0xA008` |
| `STOP_TRIGGER_REPORTING` | `0xA009` |

**None of them is ever sent.** Not in `IDLE_SEQUENCE`, not in any mode sequence,
not anywhere outside tests.

That reads as an oversight, and nothing in the code says otherwise. There is no
note beside the definitions, none in the sequences that would use them, and the
`IDLE_SEQUENCE` docblock actively misleads by claiming it *"powers down modules and
enables basic reporting"* — it enables nothing.

**This ADR exists because that gap has now cost real time.** On 2026-09-04 a
session tracing why `GET_TRIGGER_STATE` does not behave as its comment claims
reconstructed the whole path from scratch and was about to file *"we never enable
trigger reporting"* as a defect. The obvious remedy — add `0xA008` to bring-up —
would have reversed a deliberate decision that no one alive in the conversation
could see.

### Why device-side reporting was rejected

Four reasons, recorded from the decision-maker rather than reconstructed:

**1. The uplink already carries the inventory/locate firehose.** Tag reads arrive
continuously during a scan, and keeping up with that stream is the hard real-time
constraint on the handheld path — the same constraint ADR 0006 is about. Device
push traffic competes with the tag stream on the same channel for no functional
gain, since the same values can be fetched when they are actually wanted.

**2. Pushes broke response correlation.** Unsolicited reports share the uplink
with command responses. A command layer that cannot distinguish "the answer to
what I just asked" from "a thing the device decided to say" will eventually settle
the wrong command — which is precisely the defect TRA-1154 later named in full:
*"CommandManager matches no op code — any command-class packet settles the pending
command."* Removing an entire class of unsolicited traffic removes an entire class
of that failure.

**3. No control over cadence or volume.** The device decides when and how often.
There is no way to quiet it for a measurement run, so it becomes noise in every
soak capture and bridge log — and a measurement you cannot quiet is a measurement
you cannot attribute.

**4. The device's own reporting was unreliable anyway.** The trigger reporting did
not behave as the vendor specification advertised on the firmware in hand, so the
complexity bought nothing. In the words of the decision:

> *"we were concerned with keeping up with the inventory/locate firehose and i
> think that the trigger notification was not working as advertised with the
> current firmware version. it was just easier and cleaner to take control of both
> trigger and battery polling rather than leave it to the device"*

## Decision

**The host owns periodic device state.** We do not enable device-side
auto-reporting. Anything needed on a recurring basis is asked for by the protocol
layer, on a schedule the host chooses.

The four reporting commands stay defined and unsent. Deleting them is acceptable;
**sending one is not, without revisiting this ADR.**

## Consequences

**Battery is polled on a host timer.** `reader.ts` schedules
`getBatteryPercentage()` and emits `BATTERY_UPDATE` only when the value changes,
using `BATTERY_VOLTAGE_SEQUENCE` — a single `GET_BATTERY_VOLTAGE`. The TODO in
`system/sequences.ts` describing exactly this swap is the decision, half-recorded
as a task.

**Trigger state is carried by edge notifications**, `TRIGGER_PRESSED` (`0xA102`)
and `TRIGGER_RELEASED` (`0xA103`), latched host-side at `reader.ts:179` and
reconciled by `convergeToTriggerState()` when the reader settles. Verified by hand
on 2026-09-04: four of four presses started a scan across Scan and Locate, both
during and after the configure sequence.

**`GET_TRIGGER_STATE` (`0xA001`) is consistent with this decision and still does
not work.** A one-shot query issued by the host is host-driven polling, so it does
not contradict anything here — but it does not answer usefully on the firmware in
hand, and its comment claims it *"checks if trigger is already pressed on
connect"*, a behaviour it does not deliver. Whether it is repaired or removed is
TRA-1247's to decide; this ADR only establishes that its *presence* is not
evidence against the decision above.

**Accepted cost: an already-held trigger is unobservable.** With no working query
and no device reporting, an operator squeezing the trigger *before* the reader
connects produces no edge, so nothing tells us the trigger is down until it moves.
This is a real gap, accepted rather than overlooked.

**This does not contradict ADR 0013.** *A device-reported fault is evidence; it is
never filtered on a theory that it is spurious* governs **what to do with
unsolicited traffic that arrives** — an `ERROR_NOTIFICATION` (`0xA101`) the device
sends of its own accord must be named, counted and acted on, never discarded. This
ADR governs **what we ask the device to volunteer in the first place.** Listen to
everything the device says; do not ask it to narrate. The two pull in the same
direction: both are about the uplink carrying signal rather than chatter.

**Revisit condition: firmware.** Reason 4 is an observation about specific
firmware — Silicon Labs 1.0.15, Bluetooth 1.0.17, RFID processor 2.6.41, each one
release behind CSL's published versions as of 2026-09-02. A firmware update is a
reason to **re-test** that premise, not to assume it still holds and not to assume
it has changed. Reasons 1 through 3 are architectural and survive any firmware.

## Consequences for the code, outstanding

Two comments assert behaviour that does not exist and should be corrected or
removed as part of TRA-1247:

- `system/sequences.ts` — `GET_TRIGGER_STATE  // Check if trigger is already
  pressed on connect`
- the `IDLE_SEQUENCE` docblock — *"Powers down modules and enables basic
  reporting"*

A comment describing intent, read later as a description of behaviour, is what
sent the 2026-09-04 session down three wrong paths in a row. The cost of this ADR
is one file; the cost of not having had it was most of an evening.
