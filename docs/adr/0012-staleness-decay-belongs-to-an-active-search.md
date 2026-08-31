# ADR 0012 — Staleness decay belongs to an active search; a stopped search's last reading is a result, not stale data

Date: 2026-08-31
Status: Proposed
Tracking: TRA-1171 (this change), TRA-1080, TRA-1089 and TRA-1123 (the three defences this reconciles)

## Context

The Locate screen is a tag finder. Its primary claim is *"the tag is this close"*,
and its primary failure is a **false negative**: showing nothing, which an
operator reads as *"the item is not here."*

Three separate defects pushed the screen toward decaying its readings, and each
fix was right for the case it addressed:

- **TRA-1080** — reads arriving with the reader in a non-SCANNING state printed
  "No signal" beside a live dBm value. Fix: follow the read stream, not the
  reader's state machine, and floor the gauge to `DEFAULT_RSSI` once readings
  are more than a second old.
- **TRA-1123** — the Statistics panel's four numbers are recomputed only inside
  `addRssiReading()`, so a finished search left them frozen. A decoy EPC
  matching no tag showed −36 dBm at 14.5 Hz. Fix: decay them on the same
  staleness signal the gauge uses.
- **TRA-1089** — the gauge, the Status row and the Statistics panel must agree
  about whether the tag is being heard. Fix: one threshold, `STALE_THRESHOLD_MS`.

Taken together these read as a general rule: *stale data must never look live.*

**TRA-1171 is the case where that rule inverts.** On releasing the trigger, the
gauge kept moving and the audio kept sounding. The cause is that several tag
packets per stop keep arriving after the ABORT — measured on hardware, in every
run. They are **genuinely fresh**, so every one of the defences above passes
them through by design. By their own criterion nothing is stale.

And the operator has just done something deliberate: they released the trigger
*because they saw a reading*. A moment later, staleness fires and blanks it.
Applying the general rule here destroys the result of the action at the exact
instant the operator wants to read it — producing the same false negative that
TRA-1080 and TRA-1123 exist to prevent, reached from the opposite direction.

## Decision

**Staleness decay is a property of a RUNNING search.** Once scanning has
stopped, the last value is the **result** of the search that just ran, and it
holds: it does not clear and it does not decay.

Two mechanisms follow, and neither is optional:

1. **A release gate.** While no search is active, `addRssiReading()` refuses
   reads outright. Staleness cannot do this job, because the reads that cause
   the defect are fresh. Refusing them is the only thing that ends the tail;
   telling the UI about the release sooner merely shortens it.

2. **Two signals where there was one.** "What the gauge shows" and "is the
   target audible right now" must be asked separately —
   `getFilteredRSSI()` and `isHearingTag()`. They were interchangeable only
   while the display always decayed on its own. Hold the display without
   splitting them and the beeper runs forever on a number nobody is listening
   to.

The audio limb stops on release. The visual limbs — gauge, statistics, RSSI
trace — hold.

## Consequences

**TRA-1080's mechanism changes, its purpose does not.** That ticket made the
screen follow the read stream rather than the reader's state. With the gate,
reads arriving while no search is active are dropped, so the reader-in-ERROR
case now holds the last value instead of tracking the stream. It still never
shows a false "item not here" — which was the point — but a reader reading it
should know the mechanism moved.

**`STALE_THRESHOLD_MS` keeps TRA-1089's guarantee, narrowed.** The gauge, the
Status row and the Statistics panel still agree, and still use one threshold.
What changed is that the question they agree on — *is the tag being heard* — is
now distinct from what the gauge displays.

**The obvious tidy-up is wrong.** `setSearchActive(pressed)` collapses the
trigger asymmetry and breaks it in both directions: the reader drops a press
unless its state is exactly `CONNECTED`, so a press is not evidence a scan
started, and an open gate with no scan behind it admits stray reads. The gate is
opened by `READER_STATE_CHANGED → SCANNING` — which also covers the on-screen
scan button, since the button produces no trigger edge at all — and closed by
either a release or the reader leaving SCANNING. `closesLocateGate()` exists as
a named, tested predicate for exactly this reason.

**Retargeting still clears everything.** A held value belongs to the target it
was read for; `setTarget()` clears the buffer, so a new search never inherits
the previous tag's result.

**This does not generalise past Locate.** The rule holds because the screen
reports a *measurement the operator deliberately ended*. A screen showing a
continuously-updating stream with no operator-controlled stop has no equivalent
"result" state, and should keep decaying.
