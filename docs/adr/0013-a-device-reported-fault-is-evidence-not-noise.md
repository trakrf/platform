# ADR 0013 — A device-reported fault is evidence; it is never filtered on a theory that it is spurious

Date: 2026-09-01
Status: Proposed
Tracking: TRA-1229 (this change), TRA-1223 (the phenomenon it made invisible), TRA-1197 (the arm that surfaced it)

## Context

The CS108 reports a refused command with `ERROR_NOTIFICATION` (`0xA101`). The
rejection arrives under `0xA101` and **never** under the op code being rejected,
so the command that caused it sees nothing and waits out its own timeout. That
detail is what makes the frame the only evidence a refusal happened at all.

`reader.ts` matched error code `0x0000` and dropped the packet before any
handler ran:

```js
// Always ignore "Wrong header prefix" errors - they're spurious from the hardware
// The CS108 firmware incorrectly interprets its own fragmented packets as commands
logger.debug('[Reader] Ignoring spurious "Wrong header prefix" error from CS108 hardware');
continue; // Skip this packet entirely
```

The reasoning is stated, plausible, and specific — and it was wrong in the way
that costs the most: it was wrong about a class of frame it had already decided
not to look at.

Measured on hardware, 2026-09-01, across one 86.6-minute window:

```
unanswered commands on the affected pair:   1558
0xA101 frames received:                     1543      ratio 0.990
lag from command to error frame:  median 34ms, p10 27ms, p90 38ms
                                  ALL 1543 within 100ms
healthy reply to the same op code:          26ms
```

The device answered essentially every command, in the same time it answers a
command it honours. **The frames were the replies.** Because they were dropped,
the window presented as a silent device — and was investigated as one for the
length of a 7.5-hour soak arm, across four occurrences, before a transport
capture taken *below* the application showed the traffic that the application
had thrown away.

Two further layers made the same mistake in milder form. The handler's error
table numbered every code one higher than the byte-stream spec, so `0x0000` —
the code the device actually sends — fell off the end of the map and rendered
as "Unknown error". And the log rate limiter capped output at three lines per
code per worker, turning 1716 arrivals into 8 lines with nothing counting the
rest.

## Decision

**A fault the device reports is data. It is handled before ordinary routing,
counted unconditionally, and never filtered on a belief about what it means.**

Three rules follow, and each maps to a way this failed:

1. **Handle it first.** `0xA101` is dispatched ahead of the
   `isCommand`/`isNotification` decision. Under that decision a fault arriving
   while a command is in flight goes to the command path and is never counted,
   while one arriving idle goes to the notification path and settles nothing.
   **A fault must not have to win a routing race to be seen.**

2. **Count before you rate-limit.** Capping *log volume* is legitimate — an
   18-per-minute fault storm should not bury a rep log. Capping *the record* is
   not. Counts are taken before the limiter, and the running total rides each
   line that does get logged, so a rate-limited line still reports an accurate
   figure.

3. **One table, matching the vendor spec.** Two decode tables that disagree
   means the same wire bytes get different names depending on which path reads
   them, and the wrong one is invisible until someone compares.

**Filtering is not forbidden — asserting is.** If a class of frame genuinely is
noise, that is a claim with a denominator, and it belongs in a counter and a
threshold rather than in a `continue` and a comment. The rejected alternative
was to keep the filter and widen the exception list; it fails because it
preserves the property that made this expensive — a decision to stop looking,
recorded only in prose.

## Consequences

- Refusals surface as immediate named failures instead of five-second timeouts,
  and a soak arm reports them (`errorNotifications`) alongside its per-op
  timeout table. Read together those separate two different defects:
  **unanswered commands with a matching rejection count are refusals; unanswered
  commands with none are silence.** Nothing before this could tell them apart.
- Log volume rises during a fault storm. That is the intended trade, bounded by
  the rate limiter, and the count is what carries the signal.
- This ADR does not claim the underlying device behaviour is fixed or
  understood. The meaning of code `0x0000` remains open — `0x9001` succeeds on
  the same link with an identical `A7 B3` prefix inside the same window, so
  "wrong header prefix" cannot be literally true.
- ⚠ **The temptation this exists to block will recur**, in exactly the form it
  took the first time: a flood of error frames in a log, an explanation for why
  they are harmless, and a filter. The next person to reach for that should add
  a counter and a threshold instead, and should be able to say what the rate
  was before and after.

## See also

- ADR 0008 — observe a peer through its contract, not its supervisor. The same
  property that made this discoverable: the instrument sat *below* the layer
  under suspicion, so it could contradict it.
- ADR 0009 — an instrument records its run conditions at the time.
