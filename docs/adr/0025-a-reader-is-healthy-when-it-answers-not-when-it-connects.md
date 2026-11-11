# ADR 0025 — A reader is healthy when it answers, not when it connects

Date: 2026-11-08
Status: Proposed
Tracking: TRA-1041

## Context

The CS463's vendor web app can come up answering every `/API` request with
HTTP 200 and an empty body. GlassFish's classloader throws a
`NullPointerException` in `WebDirContext.lookupFromJars` on every request.
Every signal we had read green in that state:

- `systemctl is-active embeddedglassfish` said `active`.
- nginx answered 200.
- mqtt-rpcd held its broker connection and answered RPCs that do not touch `/API`.

Meanwhile the vendor app published no reads. It happened after power cuts and
after clean reboots (2 of 3 on one reader on 2026-11-08). It never cleared on its
own.

This is the second time a connectivity check has been defeated in this
codebase. The first was the demo box (TRA-989), whose broker port-forward was
TCP `ESTABLISHED` and bridged nothing.

Recovering from it turned up two more traps that a later change could easily
re-break.

- **`systemctl restart embeddedglassfish` is not a restart.** On the reader,
  `systemctl show embeddedglassfish` reports `KillMode=process`,
  `GuessMainPID=no` and `MainPID=0`. The unit is generated from
  `/etc/init.d/embeddedglassfish`, whose `stop` kills only the pid in
  `/opt/EmbeddedGlassFish/glassfish/domains/domain1/config/pid`. When that file
  is missing, systemd kills nothing and `start` launches a second java beside
  the first (observed on 2026-10-11).
- **The reader's wall clock is not trustworthy at boot.** Its RTC has started at
  2019-04-12 before NTP corrected it.

## Decision

1. **A health check asserts on returned content, never on whether the
   connection succeeded.** mqtt-rpcd's probe sends a sessionless `directIOInput`
   and judges the parsed XML. An empty 200 is `degraded`, and a well-formed
   `<Error>` is `healthy`, because the servlet is serving. It never uses
   `command=login`, which would take the reader's single session slot.
   (`mqtt-rpc/internal/readerd/cs463/probe.go`)
2. **Recovery restarts the app by emptying its cgroup, not by `systemctl
   restart`.** It runs `systemctl stop`, then SIGTERMs and after 30 s SIGKILLs
   every pid in the unit's `cgroup.procs`, and only then `start`s. If a process
   survives SIGKILL, it does not start a second instance and escalates to a
   reboot instead. (`mqtt-rpc/internal/readerd/host.go`)
3. **A boot-loop cap counts on sustained health, not on wall-clock time.** It
   allows at most two automatic reboots. The count lives on persistent disk and
   resets only after an hour of continuous health, measured on the process
   clock. A timestamp window would expire every entry when the clock jumps from
   2019 to the present, and allow unlimited reboots. A count that cannot be
   read means no reboot. (`mqtt-rpc/internal/readerd/rebootstore.go`,
   `health.go`)

## Consequences

- Any new liveness signal (a heartbeat, a canary tag, a UI health light) must
  name the content it checks. "It connected" is not a health result here.
- A reader past its reboot cap stays up, reports `auto_reboots_exhausted`, and
  waits for a person. Recovering it is now operational work the platform must
  surface. It does not yet (follow-up ticket).
- The probe proves only that `/API` is serving. It does not prove that reads
  are reaching the broker. Those travel through the vendor app's separate MQTT
  client, which an ingestion-side deadman has to watch (TRA-1109).
