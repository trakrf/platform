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
| Mode | Presence for "on while a tagged asset is here"; Egress for "fire on crossing, then latch" |
| Age-out (seconds) | how long after the last read the asset counts as gone |

The output device belongs to that scan device. Moving the relay box to another
reader means creating an output device for that reader, and deleting or
deactivating the old one.

## Verify

Walk a tag that belongs to an asset in the org past the antenna. A tag with no
asset is dropped as `no_asset` and never alarms.

**The daemon logs nothing for an RPC**, so the evidence is in the reader's own
log. Each activation is a pair of lines, from `http-listener` (the daemon's
`/API` call) and then the GPIO controller:

```sh
ssh cs463-xxyyzz 'grep -a "writeGPIOOutputPortState" /opt/EmbeddedGlassFish/logs/CS463System.log | tail'
# … writeGPIOOutputPortState : Port 2 State 1 …
# … writeGPIOOutputPortState : Port 2 State 0 …
```

- **The port is the one you entered.** A wrong port fires the wrong channel, or
  one with nothing on it.
- **State 1 follows the tag's arrival** after a gap, and in presence mode
  State 0 follows its departure by about the age-out.
- **The platform recorded it**, read-only:

  ```sql
  SELECT count(*), min(created_at), max(created_at)
    FROM trakrf.alarm_events WHERE created_at > now() - interval '30 minutes';
  ```

At the relay box, the green light is on with power, and the channel's red light
comes on with State 1. If the red light comes on and the horn stays silent,
check + and − at the horn: reversed, it does nothing.
