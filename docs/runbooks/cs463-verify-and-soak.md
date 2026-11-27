# Verifying and soaking a CS463

The checks behind §9 and §10 of [Commissioning a CS463](commissioning-a-cs463.md),
for a reader with its CloudServer built and `mqtt-rpcd` installed. Section
numbers (§) on this page refer to that runbook.

Green is not clean: each check below names the evidence to read.

## The reader is actually reading

Read the reader's own log. `systemctl`, the daemon's journal and
`Reader.GetStatus` (`"reading": true`) all looked healthy on a reader whose
inventory was halted:

```sh
ssh cs463-xxyyzz 'cd /opt/EmbeddedGlassFish/logs
  grep -a "MAC Error" CS463System.log | tail -3
  grep -a "DataRate" CS463System.log | tail -5'
```

- **`DataRate` is non-zero** with tags in the field: about 10/s with two tags on
  the antenna.
- **No new `MAC Error : 0x0309 port N, reflected power …`.** That is the
  reflected-power guard: the reader sent power into port N and most of it came
  back. **It halts the whole inventory, every port, until the port is disabled
  and the event re-armed.** The cause is physical: nothing on the port, a
  loose connector, or a bad cable or antenna. If two antennas trip on the same
  cable, suspect the cable. A reading just over the threshold (241 against 240)
  is a marginal match that a nudge tips over: 30 dBm is the daemon's default,
  not a requirement, so lower it with `Reader.SetOperProfile`, since reflected
  power falls with it. Then `systemctl restart mqtt-rpcd` to re-arm. A reboot
  changes nothing.

The vendor web UI shows the same thing live: **Events → Tag Inventory → Capture
Tags Raw**, with an `Ant #` column. Its stream (`/TagInventory`) is empty
while the inventory is halted.

## The port is the one you think it is

Check the antenna port in the published reads, not the label on the case. Every
tag in a read message carries `antennaPort`. With known tags on the antenna,
all of their reads should report the port you registered as the scan point.

## Reads reach the platform

Subscribe to the topic with the environment's credential, passed in a mosquitto
client config file (`$XDG_CONFIG_HOME/mosquitto_sub`) rather than argv:

```sh
mosquitto_sub -t '<topic>' -t '<topic>/#' -v
```

Read messages are `{"sequenceNumber":…,"numberOfTags":…,"tags":[{"epc","timeStampOfRead","antennaPort","rssi"}]}`,
with `timeStampOfRead` in epoch microseconds UTC. Check one: a 2019 date means
the clock (§3).

Then check that the backend ingested them, read-only:

```sql
SELECT message_topic, count(*), min(created_at), max(created_at)
  FROM trakrf.tag_scans
 WHERE created_at > now() - interval '30 minutes' GROUP BY 1;
```

The backend's `ingest_*` counters (`/metrics` on the backend pod) should move
together: `ingest_messages_total{result="received"}` and
`ingest_reads_parsed_total`. Tags that are not attached to an asset in that org
count as `ingest_reads_dropped_total{reason="no_asset"}` and produce no
`asset_scans`. That is correct behaviour, not a fault.

## RPC round-trip

`mqtt-rpcctl call -base <base> -method Reader.GetCapabilities` (§8). The reply
carries `"reader_model":"csl_cs463","antennas":4`. `Reader.GetOperProfile` returns the
per-antenna enablement and power, and should show only the ports with something
connected. The scan device's **Antennas & Location** panel in the app uses the
same path; it shows "Reader did not respond" until this works.

## Daemon health

`systemctl show mqtt-rpcd -p NRestarts` is `0` after the reboot that ends §8.
An update restarts the daemon once, so between `update-to-release.sh` and that
reboot the count is not `0`, and that is not a fault. The status (§8) is
`api=healthy golden=applied`.

The journal has `credited the hardware RNG to the kernel` (with
`"source":"rng-caam"`), `golden-config reconcile complete` and
`connected; rpc subscribe…`, and no `"level":"error"` or `"level":"warn"` you
can't account for. These are accounted for:

| Line | When |
|---|---|
| a `502` while the vendor app is still starting | every boot |
| four `gpo off: vendor /API failed; drove the line off through sysfs` (warn) | every boot: the start-up sweep of the alarm outputs runs before the vendor app serves |
| `this build is an update on probation; …` (warn), then `update committed: answered an rpc through the broker` | the first start after an update |
| `reader api wedged` (error), `auto-recovery: restarting the reader app`, `reader app processes still running` (warn) | a wedged boot the daemon recovered. The journal does not survive a reboot, so after an automatic reboot the only trace is `auto_reboots=N` on the status line |

These are faults: `gpo safe state: port NOT driven off`, `outputs safe state
incomplete`, `golden-config reconcile failed; will retry` that keeps repeating,
and `not crediting a hardware RNG…` or `could not credit the hardware RNG…`,
which mean this unit reaches the broker minutes after a boot, not seconds.

## Soak

Leave it reading overnight on the bench before it ships. Record at the start,
then compare in the morning:

| Measure | Start | Morning, pass if |
|---|---|---|
| `systemctl show mqtt-rpcd -p NRestarts` | the value, normally `0` | unchanged |
| daemon `"level":"error"` / `"warn"` lines | count | no new ones beyond those listed above |
| `auto_reboots=` on the status line | absent | still absent |
| `MAC Error` lines in `CS463System.log` | count | no new ones |
| `DataRate` | ~10/s with test tags on the antenna | still non-zero |
| prod `tag_scans` for the topic | — | arriving every second, no gap of more than a few seconds |
| vendor web-app java RSS (`EmbeddedGlassFish.jar`, `VmRSS` in `/proc/<pid>/status`) | ~160 MB after boot | record it: the app freezes after long uptime |
