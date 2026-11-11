# ADR 0026 — An alarm output fails toward off

Date: 2026-11-08
Status: Proposed
Tracking: TRA-1332, TRA-1138

## Context

A CS463 GPO drives a horn strobe at a hospital entrance. Two incidents showed
that nothing on the reader ever turned one off unless the cloud said so:

- **A network drop latched it for ten hours (2026-08-16).** In presence mode the
  backend sent ON once, with no pulse, and OFF when the tag aged out. The OFF is
  one QoS0 message. It was lost, and an ON with no pulse meant "stay on until an
  explicit off".
- **A clean shutdown latched it until the PoE was pulled (2026-11-08).** The
  reader was powered off with the horn on. The release timer lived in the
  daemon's memory and died with it, the cloud's OFF raced the shutdown and was
  lost, and a halted board on PoE holds its last GPO state.

There is a third trap. The vendor `/API` is the only documented way to drive a
GPO, and it stops working when the vendor web app wedges: every request answers
200 with an empty body, and the pin does not move (cs463-212, 2026-07-25). That
wedge is common on the Frederick fleet, and its recovery path reboots the
reader. An off that only goes through `/API` fails exactly when it is needed.

## Decision

1. **No ON latches.** Every ON arms its own release: after the requested pulse,
   or after the default on-time (30 s) when it names none. **Nothing stays on
   past 60 s without a fresh ON**, whatever the cloud asks for. A longer request
   is cut, with a warning. The cloud keeps a wanted output on by re-sending ON.
   (`readerrpc.GpoDefaultOn`, `readerrpc.GpoMaxOn`; `cs463/gpo.go` `onTime`)
2. **The daemon puts every output in a safe state when it starts and when it
   stops.** Start clears whatever a crash, a `kill -9` or the previous version
   left on. Stop runs before anything else in shutdown, does not depend on the
   broker, and refuses every later ON. (`readerd/daemon.go`, `cs463/gpo.go`
   `OutputsOff`)
3. **Every OFF goes below the vendor app.** It writes the kernel GPIO line, then
   `/API`, then the line again. The second write is there because the vendor
   applies a write about 30 ms after acking it. ON goes through `/API` only, so
   a wedged app fails toward quiet. (`cs463/gpo.go` `driveOff`; the pin map and
   timings are in `docs/cs463-reader-references.md`.)

## Consequences

- **A genuine alarm can be cut short.** If the connection drops during a real
  event, the output stops within its on-time. This is accepted. A horn that
  sticks on erodes trust in every later alarm, and people start unplugging them.
- **The backend holds a wanted presence output on by re-sending ON.** Every
  presence ON carries the 30 s on-time, and it is re-sent every 10 s while a tag
  stays in view, so two refreshes in a row can be lost before the output drops.
  The refresh rides on incoming reads, so it stops when the reads do.
  (`geofence/presence.go` `presenceRefreshEvery`)
- **The same bounds hold where no daemon runs.** A Shelly has nothing between it
  and the cloud, so the backend dispatcher applies them: an ON with no on-time
  gets 30 s, and a longer one is cut to 60 s. (`alarm/dispatcher.go`
  `onTimeSeconds`)
- A new output type, a new reader family, or a "latch until reset" mode must
  meet the same three rules. A setting that lets an output stay on past 60 s
  needs this ADR superseded first.
- The sysfs path needs the daemon to run as root, and a pin map per hardware
  revision. A reader whose lines are not exported logs the failure per port;
  its OFFs then depend on `/API` alone.
