# CS463 status, updates and recovery

What a commissioned reader tells you over the broker, and how to update, reboot
or recover it once it has no SSH. It follows
[Commissioning a CS463](commissioning-a-cs463.md); section numbers (§) refer to
that page. The mechanics are in [`mqtt-rpc/README.md`](../../mqtt-rpc/README.md)
and the [installer README](../../deploy/edge/mqtt-rpc/README.md).

Every command here is `mqtt-rpcctl` with the broker URL in `MQTT_URL` (§8).

## Reading the status line

`mqtt-rpcctl status -base <base> -watch` prints one line per change.

| Line shows | Means | Do |
|---|---|---|
| `online <version> api=healthy golden=applied` | ready | nothing |
| `api=unreachable … golden=pending`, with a `detail` naming a 502 | the first 2–3 minutes of a boot: the vendor app is still starting | wait |
| `api=healthy golden=pending` that stays | the reconcile is failing. The usual cause is a wrong `READER_API_PASS`: a wrong password still counts as a serving API | see below |
| `api=degraded WEDGED recovery=app_restart` | `/API` has answered with an empty body for 6 minutes; the daemon is restarting the vendor app | wait |
| `recovery=reboot`, then `offline (auto-recovery)` | still wedged 6 minutes after the app restart; the daemon is rebooting the reader cleanly | wait |
| `auto_reboots=N` | N automatic reboots since `/API` was last healthy for a full hour | see "Before it ships" (§11) |
| `EXHAUSTED` | two automatic reboots are spent, or the count could not be read. The daemon stays up and stops trying | a person decides: `mqtt-rpcctl reboot`, or the recover login |
| `offline (reboot)` / `offline (shutdown)` | someone asked for it over RPC | for a shutdown, power-cycle on site |
| `offline` alone | the broker's Last Will: the daemon died or lost its link | check power and network |
| `cloudserver_fault="…"` | the reader's own publishing entry is wrong; reads have stopped or will | "CloudServer faults" below |

**`golden=pending` with a healthy API.** `-watch` leaves the detail out while
the API is healthy. Read the whole document with `mqtt-rpcctl status -base
<base>` (no `-watch`), or the reader's journal for `golden-config reconcile
failed; will retry`. The daemon retries every 5 minutes, so after fixing the
password in `/opt/trakrf/mqtt-rpcd.env` either restart it or wait.

**The recovery ladder, in full.** `/API` unhealthy for 6 minutes marks the
reader wedged. The daemon stops the vendor app, kills what is left of it and
starts it again; one recorded restart took about 2 minutes to serve. If `/API`
is still unhealthy 6 minutes after that, the daemon reboots the reader cleanly.
So the first automatic reboot comes at least 12 minutes into a wedge. It makes
at most 2 automatic reboots until `/API` has stayed healthy for 1 hour; the
count is kept on the reader and survives the reboots it counts.

**Time to the broker after a boot.** The daemon is on the broker within about
half a minute of power-on (18 to 27 s measured on one bench reader), because it
credits the hardware RNG to the kernel at start. The vendor app's own MQTT
client, which carries the reads, follows within seconds of `/API` serving,
about 2 minutes into a healthy boot. The image has no `ss`; use
`netstat -tn | grep 8883` on the reader to see both connections.

## CloudServer faults

The daemon re-reads the golden CloudServer and its trust bundle every 30 s, and
checks the broker's certificate chain on each of its own connections. It
reports a fault and **never rewrites the entry**. The reader cannot edit an
entry either, so the fix is always §7 again: delete and re-add it, or upload
the bundle again, then reboot.

| Fault text | Fix |
|---|---|
| `cleanSession is …; golden is true`, `qos is …; golden is 0`, `sslVersion is …` | delete the entry and re-add it with §7's values |
| `trust bundle … missing: the reader publishes no reads and logs nothing` | upload the bundle again |
| `first certificate in trust bundle … not a self-signed root` | the bundle leads with an intermediate or a leaf. Rebuild it with the root first (§0) and upload it |
| `first certificate … expired`, or `is not valid until … (is the reader clock right?)` | check the clock (§3) first, then the bundle |
| `the broker's certificate … does not chain to …` | the broker now presents a chain that does not reach the bundle's first certificate. Fix the broker's chain or the bundle |

## Updating a reader

```sh
mqtt-rpcctl update -base <base> -manifest https://github.com/trakrf/platform/releases/download/mqtt-rpcd-<version>/manifest.json
```

`update` returns as soon as the reader accepts the request. The outcome arrives
later, in `result.daemon.update.state` of
`mqtt-rpcctl call -base <base> -method Reader.GetCapabilities`:

| `state` | Means |
|---|---|
| `committed` | the new build has been up 3 minutes and answered an RPC through the broker. Expect this no sooner than 3 minutes after the update |
| `rolled_back` | the build crash-looped (more than 5 starts) or never answered through the broker within 15–20 minutes, and the guard put the previous version back |
| `failed` | nothing was replaced |
| `unguarded` | the build never committed and no other version was installed to go back to. It is still running and can take another update |

- **A build that commits and then misbehaves is not rolled back.** Send another
  `update` to a different signed version; an older one is allowed.
- **Refusals.** The reader refuses an update to the version it already runs,
  and a second update while one is in progress or not yet committed. It refuses
  a reboot or shutdown while an update is downloading or installing.
- **The app does the same thing.** A CS463 scan device has a **Reader
  software** panel that sends the update and follows it. It is offered to an
  org admin of a paid org.
- **Release rule.** A release goes to shipped readers only after it has itself
  updated a bench reader to a newer build.

`deploy/edge/mqtt-rpc/update-to-release.sh <reader> [version|latest]` wraps the
same call for a reader you still have SSH to, and waits for the outcome. Its
endings other than `OK: … committed`:

| Ending | Means |
|---|---|
| `no release mqtt-rpcd-<v> on GitHub` | the version is wrong, or this machine cannot reach GitHub |
| `no golden CloudServer (…) on this reader` | §7 is not done, or the name is not exact |
| `FAILED: rolled_back` or `failed (…)` | the reader is on the version named in the message |
| `UNGUARDED: …` | as `unguarded` above: run it again with a good version |
| `TIMEOUT after 25 min` | no outcome yet. On probation the guard settles it; otherwise run it again |

## Reboot and shutdown

`mqtt-rpcctl reboot -base <base> -yes` and `mqtt-rpcctl shutdown -base <base>
-yes`. Both were run on a bench reader. Add `-by <who>` to name yourself in the
reader's log; it defaults to your user and host.

- The reader replies `{"action":…,"in_ms":2000}`, and 2 seconds later its status
  reads `offline (reboot)` or `offline (shutdown)`. Its stop drives every alarm
  output off first.
- A request is refused (`-33003`) while the reader is already going down, or
  while an update is downloading or installing.
- **Never publish an RPC as a retained message.** The daemon drops a retained
  request, because the broker would replay it on every reconnect. Clear one
  with `mosquitto_pub -r -n -t <base>/rpc`.
- After a shutdown the reader stays off until someone power-cycles it on site.

## The recover login

`ssh recover@<reader>`, with that reader's recover password, is the way in for
someone on the reader's network who is not us. It is the only credential a
site's IT is given.

- It has no shell. It runs one fixed command, prints each step, and logs out.
- It prints the link speed and the reader's addresses, moves the daemon to the
  newest release on GitHub if it is not already on it, and then **always
  reboots the reader cleanly**, even when nothing could be downloaded. The
  reader is back in about 5 minutes.
- The download is checked against the release's hash, not its signature.
- It needs outbound TCP 443 to `github.com`, `api.github.com` and
  `release-assets.githubusercontent.com`. Without them it still reboots.
- The last run is kept in `/opt/trakrf/update/recover.log`.
- The password must be at least 12 characters with no `:`. Set or change it
  with `deploy/edge/mqtt-rpc/set-recover-password.sh <reader>`, which needs SSH
  as `root`.

## Readers installed earlier

Daemon versions are `git describe` names such as `v1.5.0-404-ga9281202`; the
number after the tag is what to compare. Read it from the status line.

| Daemon older than | Lacks |
|---|---|
| `v1.5.0-326` | the health probe, wedge recovery and the status document (release `v1.5.0-312` has none of them) |
| `v1.5.0-341` | outputs driven off at stop and start, and the 30 s / 60 s on-time limits. An output that is on when the reader halts stays on while the board has power |
| `v1.5.0-351` | the CloudServer fault check |
| `v1.5.0-378` | `Reader.Reboot` and `Reader.Shutdown` (it answers "unsupported method") |
| `v1.5.0-399` | the hardware RNG credit: it reaches the broker minutes after a boot, not seconds |

Release `v1.5.0-404-ga9281202` has all of them, and an `update` brings any
reader with a working guard up to it.

**The rollback guard and the recover login never arrive over the air.** Only
`install.sh` installs them. A reader installed before the recover login existed
has none, and one whose guard timer is missing or not enabled refuses every
update with "re-run install.sh". Both need SSH, so fix them before a reader
ships (§8), or on a visit.
