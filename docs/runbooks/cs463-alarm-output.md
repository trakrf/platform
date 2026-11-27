# CS463 alarm output

Adds a horn/strobe, driven by one of the reader's GPO ports through the relay
box, to a reader already [commissioned](commissioning-a-cs463.md) and reading.
The platform decides when to alarm and sends `Gpo.Set` to the reader over its
RPC topic. Nothing is configured on the reader itself.

## Wire it

Five cables: Ethernet and the reader's own 12 V adapter to the reader; the
supplied HD15 cable from the reader's GPIO port to the relay box; a 24 V
adapter to the relay box; two wires from the relay box channel to the
horn/strobe, + to +.

- **Do not swap the adapters.** The plugs look alike: the reader takes 12 V and
  the relay box 24 V.
- **Use the supplied HD15 cable.** A VGA cable fits but can be missing wires.
- **Note which GPO port the channel is on.** That number goes in the output
  device below. Write it on the install record, not just the relay box.

## Create the output device

In the reader's org, **Outputs → Create**. The tab needs the org's geofence
capability; without it, it routes to the upsell.

| Field | Value |
|---|---|
| Name | what it is and where, e.g. `Dock door 3 horn strobe` |
| Type | CSL Reader GPO |
| Reader | the scan device, from the list |
| GPO port (1–4) | the port the relay channel is wired to |
| Location | the location the alarm covers (the reader's, from §6) |
| Mode | Presence: on while a tagged asset is being read here. Egress: fires on a crossing, then turns itself off after Auto-off. The form's option still says "then latch"; it does not latch |
| Age-out (seconds) | presence: how long after the last read the output clears. Egress: how long before the same tag can fire again |
| Auto-off (seconds) | egress only, and required: 1–60, unless the org's geofence default supplies one. Disabled in presence |

The output device belongs to that scan device. Moving the relay box to another
reader means creating an output device for that reader, and deleting or
deactivating the old one. **Delete drives the output off first.** If the old
reader cannot be reached, the delete is refused and the row kept; "Delete
anyway" removes it, and leaves the output as it is.

## Verify

Check the reader's status reads `api=healthy` first
([status page](cs463-status-and-updates.md)). An ON goes through the vendor
`/API` only, so a wedged reader does not alarm. An OFF still lands, because
the daemon also writes the output line directly.

**Wiring and port, without a tag.** Open the output's row and press
**Test-fire**: the output comes on for a short test pulse and releases itself.
**Reset** turns it off.

**Then with a tag.** Walk a tag that belongs to an asset in the org past the
antenna. A tag with no asset is dropped as `no_asset` and never alarms.

**The daemon does not log a `Gpo.Set`**, so the evidence of an ON is in the
reader's own log. Each write is a pair of lines, from `http-listener` (the
daemon's `/API` call) and then the GPIO controller:

```sh
ssh cs463-xxyyzz 'grep -a "writeGPIOOutputPortState" /opt/EmbeddedGlassFish/logs/CS463System.log | tail'
# … writeGPIOOutputPortState : Port 2 State 1 …
# … writeGPIOOutputPortState : Port 2 State 0 …
```

- **The port is the one you entered.** A wrong port fires the wrong channel, or
  one with nothing on it.
- **Presence:** State 1 follows the tag's arrival, and repeats about every
  10 s while the tag is still being read, because the platform re-sends the ON
  to hold it. One State 0 follows the tag's departure by about the age-out.
- **Egress:** one State 1 on the crossing, and State 0 after Auto-off, timed on
  the reader.
- **A daemon start or stop** adds a State 0 for all four ports.
- **The platform recorded it**, read-only. One row per alarm, not per State 1:
  a refresh writes no row.

  ```sql
  SELECT count(*), min(created_at), max(created_at)
    FROM trakrf.alarm_events WHERE created_at > now() - interval '30 minutes';
  ```

The daemon's journal shows the other half: each self-release (`gpo auto-off`),
each start and stop sweep (`gpo safe state: port off`), and any on-time it cut
(`gpo on-time above the hard cap; capped`):
`journalctl -u mqtt-rpcd | grep gpo`.

## What it does when things go wrong

**No ON latches.** Every ON releases itself (30 s by default, never later than
60 s), and the platform holds a wanted output on by re-sending it. The daemon
also drives all four ports off when it starts and when it stops. See "Alarm
outputs" in [`mqtt-rpc/README.md`](../../mqtt-rpc/README.md). A reader on a
[daemon older than `v1.5.0-341`](cs463-status-and-updates.md#readers-installed-earlier)
does none of this: update it before giving it an alarm.

- **Presence follows reads, not time.** The re-send rides on reads, so when a
  tag stops being read the output drops at the age-out or after 30 s,
  whichever comes first. An age-out above 30 s does not hold it on longer.
- **The default on-time** can be shortened per reader with
  `READERD_GPO_DEFAULT_ON` in `/opt/trakrf/mqtt-rpcd.env`. It must be above 0
  and at most 60 s, or the daemon refuses to start.

Three checks are worth doing on the bench before an alarm ships. Each passed on
a bench reader when the behaviour was built; repeat them on the unit and relay
box you are shipping:

1. **Hold a tag in view for over a minute.** The horn holds steady, with no
   flicker at the 10 s re-sends.
2. **With the horn on, take the reader down:** `systemctl stop mqtt-rpcd`,
   `mqtt-rpcctl reboot` or `mqtt-rpcctl shutdown`. It goes quiet.
3. **With a tag held in view, unplug the reader's network.** The horn drops
   within 30 s.

At the relay box, the green light is on with power, and the channel's red light
comes on with State 1. If the red light comes on and the horn stays silent,
check + and − at the horn: reversed, it does nothing.
