# ADR 0008 — Observe a peer through its published contract, never through its supervisor, process name, or logs

Date: 2026-08-29
Status: Proposed
Tracking: TRA-1203 (this change), TRA-1202 (the systemd unit that forced the question), TRA-1189 (the false abort), TRA-1150/TRA-1200 (the soak campaign whose evidence depends on it)

## Context

The soak harness needed to answer one question about a peer process: *am I still
talking to the same bridge I started this run with?* Getting that wrong does not
fail a run — it silently corrupts one. A mid-soak restart under `Restart=always`
produces a fresh daemon in about five seconds, the run continues, and the night
yields rows spanning two daemons with no marker in the data. "One PID
throughout" is load-bearing for every conclusion the campaign draws.

The first draft answered it the obvious way: derive the unit name from the
daemon's cgroup, then read `journalctl --user -u <unit>` for restart banners.
That design coupled this repo's watchdog to **systemd specifically**, to the
unit's final name, to its log destination, and to its log level — four things
that belong to the peer and that the peer may change without telling us. A
second bridge is already planned on a container where the supervisor may differ.

That draft was overturned on the principle that produced this ADR: *a consumer
should not have to know its peer's implementation details.*

**The same defect had already shipped three times in this repo, each time
silently.** Every instance is a check that succeeded against the wrong subject,
which is the failure mode worth naming — not a check that failed:

- `pgrep -f 'rust-ble-test'` — a binary deleted in the Python replatform. The
  check returned "not running" forever, so `bridgePid` was `null` on every
  repetition and the summariser's "the bridge changed mid-soak" detector had
  nothing to compare. **A detector that reads as covered because the field
  exists.**
- `pgrep -f 'ble_bridge'` — the Python *module* name, which never appears in a
  process cmdline; the console script spells it `ble-bridge`, with a hyphen.
  Nobody chose that alias: `[project.scripts]` generates it, because
  distribution names conventionally hyphenate and module names must underscore.
- `pgrep -f` matching **its own shell's argv**, because the pattern appears in
  the command line of the pipeline running it. That produced a false abort
  mid-campaign during TRA-1189.

A fourth was avoided only because the peer pushed back: `systemctl show -p
MainPID` asks *"is the unit fresh"*, not *"is the process that will answer this
run fresh"*. Those diverge exactly when a stale ad-hoc daemon holds the port
while the unit sits stopped — the systemd form **passes** and the run is
answered by old code.

The peer's own contract already answered the question, and had all along:
`status.uptime_seconds`, over its published control socket.

## Decision

**A check about a peer's state asks the peer, through the interface the peer
publishes.** Anything else is inference from an implementation detail, and it is
a defect even while it happens to work.

Concretely, for any peer this repo observes — the BLE bridge today, `mqtt-rpcd`
on a CS463, any future service:

1. **Ask the contract.** State belongs to whoever owns it. `uptime_seconds`
   answers process identity; `get_connection_state` answers who holds the
   command path — including a browser tab, which no process listing can see.
2. **Do not identify a process by name.** If a pid is genuinely needed, resolve
   it by *behaviour* — whatever is serving the port **is** the peer, whatever it
   is called — or take it as an explicit argument. Never by a name match, which
   can match the matcher.
3. **Do not couple to the supervisor.** No unit names, no cgroups, no
   `systemctl`, no journald in an automated path. These are the peer's to
   rename.
4. **No answer is an abort, not an unknown.** A call that times out or refuses
   cannot be told apart from a wedged peer. Detection requires the peer to
   answer *at all*, not to answer honestly — which dissolves the objection that
   asking a component about its own failure is the wrong direction.
5. **Logs are forensics, not detection.** A human reads them after an abort. An
   automated dependency on a log format, path, or level is the coupling above
   wearing different clothes.

**Local tooling remains correct for local things.** Whether *this* host has a
listener on a port is a local question. The split is about subject, not
mechanism: the peer's interface for the peer's state.

## Consequences

The watchdog now works identically on the soak host, on a container with a
different supervisor, and against a hand-started daemon with no supervisor at
all — and it stopped being blocked on the unit's final shape, which is why parts
of TRA-1203 could be written before TRA-1202 landed.

**This costs something, and the cost is the honest part.** A contract-level
check can only ask what the contract exposes. When this was written `status`
exposed no process identity, so restart detection is arithmetic over
`uptime_seconds` rather than a direct comparison — correct, but indirect, and it
required a test for the case where uptime re-grows past its starting value.

**The rule for a missing field is to ask the peer to publish it, not to reach
around the contract for it** — raised as TRA-1204, and reaching around a missing
field is how all three defects above were introduced.

That request was taken up, and what came back is worth recording because the
obvious reading of it is wrong. A process identifier does **not** retire the
arithmetic, and the reason is not version skew across hosts — it is that the two
answer different questions. An identifier answers *is this a different process*.
The arithmetic answers *has this process been running for the whole interval I
measured*. A restart fires both; a **host suspend** fires only the second, since
`CLOCK_MONOTONIC` does not advance across it — same process, same identifier, and
an hour of wall clock the run did not experience. What the identifier removes is
the tolerance-sizing problem, which is the part that actually bit.

So the general form: **when a peer adds a field, check whether it answers your
question or a neighbouring one before deleting what you had.** A narrower
property wearing the contract's claim is the same defect this ADR opens with.

Where the contract is genuinely insufficient and the peer cannot extend it, say
so explicitly at the call site and treat it as a known coupling with a ticket —
not as a quiet local workaround, which is indistinguishable from the shipped
defects this ADR exists to prevent.

The bar this sets is deliberately not "avoid systemd". It is: **when a check
passes, be sure it passed about the thing you meant.** Every failure above
passed about something else.
