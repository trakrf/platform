# ADR 0023 — A remote update is undone by what it cannot replace

Date: 2026-11-08
Status: Proposed
Tracking: TRA-1317

## Context

Customer CS463 readers have no SSH. After one install they are reachable only
through the MQTT broker, so the daemon updates itself (`Daemon.Update`). Whatever
update and rollback path a reader is installed with is the only one it will ever
have without a site visit. Two failure modes strand it: a new build that cannot
reach the broker, and a rollback that depends on the build being rolled back.

## Decision

1. **The rollback lives outside the thing it rolls back.**
   `deploy/edge/mqtt-rpc/mqtt-rpcd-guard` is a POSIX sh script run by systemd: as
   `ExecStartPre` (counting starts) and on a timer (counting ticks). It is
   installed only by `install.sh` and is **never updated over the air**. The
   daemon arms it (`update/pending`) and may disarm it by committing, but never
   performs or decides a rollback.

   A separate Go manager daemon was considered and rejected. It would need its
   own broker connection, config discovery, TLS, download and verification. All
   of that would be frozen too, so the code that can never be fixed remotely gets
   larger. Go plugins are unavailable in a static `CGO_ENABLED=0` armv7 build.

2. **Commit means "stayed up and answered through the broker", never
   "reconcile succeeded".** The new daemon commits only after it has been up
   3 min, has answered an RPC through the broker (its own loopback
   `Reader.GetCapabilities` counts), and its own updater preflight passes.

   The vendor `/API` is deliberately not part of the criterion. A wedged vendor
   app is exactly when a fix must be pushed, and making reconcile a criterion
   would roll that fix back.

3. **The guard only acts on what it has verified.** It repoints the link and
   confirms it, syncs, and only then deletes or records anything. A failed step
   leaves `pending` armed for the next run. The daemon's commit and the guard's
   rollback both claim `pending` by rename, so exactly one wins.

## Consequences

- A guard bug is permanent on installed readers. The guard is kept small, tested
  in Go against the real script (`internal/update/guard_test.go`), and
  shellchecked in `just test`. A change to it only reaches readers through
  `install.sh`.
- A committed build whose *updater* is broken can still strand a reader. Commit
  proves the build connects; it does not prove the build can update itself.
  Hence the release rule: a build goes to customer readers only after it has
  updated a bench reader to a newer build.
- Signing keys are compiled in. Retiring a key is rotation, not revocation:
  downgrades are allowed and old releases stay public (see `mqtt-rpc/README.md`).
- The state format (`update/*`, `key=value`) is a contract between Go and sh, and
  between today's guard and every future daemon. Change both, or neither.
