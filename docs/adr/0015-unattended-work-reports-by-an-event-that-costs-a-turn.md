# ADR 0015 — Unattended work reports by an event that costs a turn; notifying is not invoking

Date: 2026-09-03
Status: Proposed
Tracking: TRA-1242 (this change, and the two arms that paid for it), TRA-1240 (the driver output this is the delivery half of), TRA-1224 (a checklist item nobody can fail)

## Context

A soak arm is a ~7.3 hour unattended run. TRA-1240 made its output good: one line
per repetition carrying *why*, and an aggregate block every ten reps with a
pass/fail strip, because a wedge is a **run** of consecutive failures and totals
cannot show a run.

```
--- 160/200 · elapsed 6h25m · eta 1h36m ---
  passed 156  failed 4  (2.5%)
  last 40: .X......................................
```

**Every one of those lines goes to a file.** TRA-1240's acceptance was entirely
about what the driver *emits*; nothing in it covered who reads it. That was a
safe assumption while the arm was typed into a terminal somebody could `tail -f`.
It is false when the arm is launched from an agent session: the log is a dead
end, the operator sees nothing for seven hours, and the only path to them is the
model choosing to speak. On two consecutive arms it did not, until asked.

Prose cannot close this and it is not a defect in the emitter. The driver is
already correct, and no sentence in a runbook makes a background process talk to
a REPL.

### Two mechanisms that look right, and fail in opposite directions

**A clock.** A 15-minute `/loop` (`CronCreate`) over a 7.3 h arm is ~29 model
turns, each re-reading context and re-running tools, to report a number that
changed about 20 times. It works, and it pays a full turn for every interval in
which nothing happened.

**A stream into context.** A `Monitor` wrapping an event-gated shell loop is
dramatically cheaper — the polling runs in the shell, outside the model, and
emits only on change. It is also structurally unable to deliver anything.

|  | invokes the model? | fires on | cost while quiet |
| -- | -- | -- | -- |
| `Monitor` | **no** — context only | every stdout line | zero |
| `CronCreate` / `/loop` | yes | a **clock**, event or not | a full turn per interval |
| a backgrounded command | **yes** | its **exit** | zero |

A `Monitor`'s lines reach the model's **context** and create no **turn**. Text
reaches the operator's terminal only when the model is given a turn, so in an
idle session the events sit there invisibly until something else makes it speak.
Confirmed 2026-09-01: the monitor fired correctly for reps 24, 25 and 26 while
the operator reported *"i see nothing other than your direct responses."*

That makes *"arm a monitor and relay each event"* structurally broken rather than
merely unreliable — there is no moment at which the relay happens. It was
proposed and re-proposed three times across two sessions, each time looking like
the cheap correct answer, because the cost table above is right and the delivery
column is the one that matters.

## Decision

**In-REPL delivery always costs one model turn. That is not avoidable, and the
only choice is what triggers it: a clock, or an event.** Choose the event.

So unattended work reports through **a chain of one-shots, not a stream**: a
backgrounded command that blocks until the next real event, prints it, and
`exit 0`. Its exit re-invokes the model, which reports and re-arms. One turn per
real event — the minimum possible — and none while quiet.

Three rules follow.

**1. The watcher terminates, and its name says so.** `await-soak-event.mjs`, not
`watch-`. A name saying "watch" invites the next person to turn it back into a
stream, which is the defect this exists to remove. It is the only one of the
three soak processes with a short lifetime, and that asymmetry is deliberate:

| | lifetime | job |
| -- | -- | -- |
| `characterise-suite-runs.mjs` | ~7 h, detached | **runner** — does the arm |
| `watch-soak-abort-criteria.mjs` | ~7 h, detached | **watchdog** — aborts on instrument faults |
| `await-soak-event.mjs` | seconds, disposable | **watcher** — blocks until news, prints it, **exits** |

**2. Covering every terminal condition is the correctness requirement, not
polish.** A watcher that can hang stops the chain, and a dead chain is
indistinguishable from a quiet arm — the same silence-is-not-success rule the
abort criteria already follow. So it must exit on the failure paths too, not only
on the happy one: the process disappearing, the watchdog speaking, the primary
signal breaking. It must also exit immediately when the work has *already* ended,
rather than waiting for a change that will never come.

**3. One instrument serves a human and an agent.** The same command blocks and
prints in a terminal, and re-invokes a model when backgrounded from a session.
An agent-specific reporting path is a second instrument that drifts from the
first, and the one nobody runs by hand is the one nobody notices is broken.

## Consequences

A watcher is now a **required** step when launching an arm from a session, named
in the runbook beside the watchdog rather than offered as a nicety. An arm nobody
can see is not being watched because a watchdog is armed: the watchdog aborts on
instrument faults and reports no progress, and the two are not substitutes.

Recovery after a session crash is deliberately **not** automatic, and the split
is the point: the expensive unrepeatable processes are detached and survive, the
cheap disposable one does not. Nothing re-arms it, so the work goes back to
invisible until a new session is told to pick it up. Everything needed is on
disk, so the runbook gives the one command rather than leaving it to be
reconstructed.

This rule is about **delivery**, not about what to say. It does not license
chattiness: the trigger is a real event, and a signal chosen because it moves
constantly turns the chain back into a stream with extra steps.

It generalises past soak arms to any long-running unattended job that has to
reach a session — a benchmark, a migration, a deploy watch. The question to ask
is not "how do I get the output somewhere" but "what exit does this operator want
to pay a turn for".
