# ADR 0014 — A terminal reader state is published only when it is terminal

Date: 2026-09-01
Status: Proposed
Tracking: TRA-1237 (this change), TRA-1225 (the same confusion one layer up), TRA-1122/TRA-1123 (the Locate failures it produced)

## Context

`ReaderState` mixes two kinds of value, and nothing in the type says which is which:

- **transient** — `BUSY`, `CONNECTING`. Something is in flight and a later state is coming.
- **terminal** — `CONNECTED`, `DISCONNECTED`, `ERROR`. The unit of work is over.

`Reader.waitForSettledState` is built directly on that distinction. It parks a caller until
the reader stops being transient, and every caller of it — today, the settings-apply path —
then decides what to do based on what it settled into. The whole mechanism is only correct if
a terminal state is published **when, and only when, it is actually terminal**.

`CommandManager` broke that twice, in the same function, for the same reason.

It announced `ERROR` from inside `runSequence`'s attempt loop, before `retryDelays` were
walked. A command that failed once and succeeded on the retry therefore published `ERROR` and
recovered from it about two seconds later:

```
Busy → Error → Connected
```

And its abort branch announced `ERROR` on a `SequenceAbortedError`, directly contradicting the
comment above it, which said an abort "does not publish ERROR on its way past: the setMode
taking over owns the state from here."

Both are the same category error: a state whose meaning is *"the hardware is in an unknown
condition"* used to describe *"this attempt failed and another is coming"* and *"a different
operation is taking over"*. Neither is an unknown condition. Both self-clear in about two
seconds.

The damage was not the flicker. A settings push parked on `BUSY` woke on the transient
`ERROR`, found the reader not `CONNECTED`, and dropped the `targetEPC` it was carrying —
so Locate ran its next search against the previous tag's mask, which is the only EPC filter
the radio has. In the 2026-09-01 200-rep arm, **27 of 33** `locate-mask-length-variants`
failures carry the resulting log line, against **166 of 166** clean reps without it.

That the state was transient is measurable, not inferred: `Cannot start scanning from state`
appears **zero times in all 200 reps**, so the reader was `CONNECTED` again by the time the
operator's trigger was pulled, every single time.

Four other consumers read `ERROR` — the scan button, the status badge, the Connect
affordance, the header styling. All four were being told the hardware had failed, several
times per session, by commands that worked.

## Decision

**A state that callers treat as terminal is published only at the point it is true.**

Concretely, in `CommandManager`:

- A failed **attempt** publishes nothing. It is not the sequence's verdict.
- `ERROR` is published once, at the point the sequence has genuinely failed — the retry
  schedule spent and the step not tolerated — immediately before the throw.
- An **abort** publishes nothing at all. It is a handoff; the operation taking the wire owns
  the state from there, and leaving `busyAnnounced` set keeps the reader `BUSY` across the
  handover rather than flashing a state nothing reached.

The waiter was **not** changed. Teaching `waitForSettledState` to ignore an `ERROR` it is
shown would have preserved a state that lies to its other four readers, and would have made
the reader sit through a genuine failure whose answer was already known.

## Consequences

- Adding a new failure path to `CommandManager` means deciding, explicitly, whether it is the
  sequence's verdict. If it is not, publish nothing. This is the rule that is easy to
  re-break: announcing eagerly *looks* more informative, and its cost is invisible at the call
  site — it is paid by a parked waiter in another file.
- `ERROR` becomes rarer and means more. A soak log counting `ERROR` transitions now counts
  failures rather than attempts.
- The four UI consumers stop being told the reader failed by commands that recovered.
- `waitForSettledState`'s transient set (`BUSY`, `CONNECTING`) stays the definition of
  transient. If a future state is added that self-clears, it belongs in that set — the
  alternative, publishing it and asking every reader to disbelieve it, is what this ADR
  rejects.
- Unit tests can pin the state **trace** rather than the presence of a state. Asserting only
  "no ERROR" passes on a manager that publishes nothing at all; the tests added with this
  change assert the whole sequence of published states.
