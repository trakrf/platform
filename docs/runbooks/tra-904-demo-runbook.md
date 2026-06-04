# TRA-904 — Frederick Health demo runbook (Tim-operable)

> **Status: WORK IN PROGRESS.** This runbook is being assembled as the
> demo tickets land. Sections marked **PENDING** are not ready yet. The alarm
> device / Shelly provisioning section below is complete and usable now.
>
> Epic: [TRA-897](https://linear.app/trakrf/issue/TRA-897) · this ticket:
> [TRA-904](https://linear.app/trakrf/issue/TRA-904). Related: TRA-903 (alarm
> CRUD + HTTP fire, PR #450), TRA-906 (MQTT fire, PR #451), TRA-898 (edge box),
> TRA-901 (geofence engine).

---

## Alarm device (Shelly Gen4) provisioning

There are **two fire paths**. Pick by where the firing backend runs relative to
the relay:

| Path | Use when | Reachability |
|------|----------|--------------|
| **HTTP** (`transport=http`) | backend runs **on the same LAN** as the Shelly (edge box / on-prem) | backend → device over local HTTP |
| **MQTT** (`transport=mqtt`) | backend runs in the **cloud** (preview/prod) or anywhere the device is behind a firewall | device → broker (outbound); backend → broker. Nothing inbound to the device |

> **Why MQTT exists:** a cloud backend cannot reach a LAN/firewalled relay over
> HTTP. Test-fire from cloud preview against a `192.168.x` device returns **502
> "no route"** — that's expected, not a bug. Use the MQTT path for cloud.

### A. HTTP path (edge / on-LAN)

1. Note the Shelly's local address, e.g. `http://192.168.50.66`.
2. In the app → **Alarm Devices** → New: set **Transport = HTTP**, **Base URL**
   = the device address, **Switch ID** = relay channel (usually `0`), and
   **Location** = the boundary location that should fire it.
3. **Test-fire** from the row — the relay pulses on→off (~2s). A 502 means the
   backend can't reach the device (wrong network / IP / device down).

### B. MQTT path (cloud / firewalled) — TRA-906

> Requires PR #451 deployed for the **backend publish** side. The **device
> config** and the `mosquitto_pub` validation below can be done any time,
> independently.

**Broker (self-hosted Mosquitto):**
- Host: `mqtt.preview.gke.trakrf.id:8883` (preview) / `mqtt.prod.gke.trakrf.id:8883` (prod)
- TLS **1.2**, `mqtts`
- Auth: shared `trakrf-mqtt` user/pass (from the `trakrf-mosquitto-auth` secret —
  ask infra / read the secret; not stored in this runbook)
- No per-topic ACL yet (TRA-857)

**Configure the Shelly** (web UI **or** `MQTT.SetConfig` RPC — this is an
on-LAN step; the cloud backend cannot provision the device):
1. **Server**: `mqtt.preview.gke.trakrf.id:8883`, **TLS enabled**.
2. **CA certificate** — the gotcha: the Shelly must trust the broker's Let's
   Encrypt chain (leaf → YE2 → ISRG root). A single leaf-issuer cert (YE2)
   worked for the GL-S10 readers; the ISRG root is more renewal-proof if the
   Shelly chain-builds. (Same issue as the readers — see
   project_mqtt_reader_cert_trust.)
3. **Username / password**: the `trakrf-mqtt` creds.
4. **MQTT topic prefix**: choose the device's prefix, e.g.
   `trakrf.id/dock-strobe`.
5. **Enable MQTT control** (so the device accepts `…/command/switch:0`).

**Register in the app** → **Alarm Devices** → New: **Transport = MQTT**,
**Command Topic** = the same prefix you set on the device
(`trakrf.id/dock-strobe`), **Switch ID** = `0`, **Location** = the boundary
location. The backend publishes `on`/`off` to
`<command_topic>/command/switch:<switch_id>`.

**Validate the device side independently** (no backend needed — confirms device
+ broker + cert + topic in one shot):

```bash
mosquitto_pub -h mqtt.preview.gke.trakrf.id -p 8883 --cafile <le-ca.pem> \
  -u trakrf-mqtt -P '<pass>' \
  -t 'trakrf.id/dock-strobe/command/switch:0' -m on
# relay should click on; -m off to clear
```

If the relay actuates, the backend's publish produces the identical message.

### Test-fire semantics
- **HTTP**: real round-trip — a green test-fire means the device responded; 502
  means unreachable.
- **MQTT** (publish-and-trust): a green test-fire means the **broker accepted**
  the publish, **not** that the relay confirmed (MQTT is fire-and-forget). If
  the relay doesn't move, check the device's broker connection / topic prefix /
  CA trust.

### Manual reset
After a real fire the relay **latches on** until reset. Use the **Reset** button
on the device row (or `mosquitto_pub … -m off` for the MQTT path) to clear it
between demo runs.

---

## PENDING — remaining TRA-904 scope

- [ ] **Demo data preload**: demo org, assets, armed-EPC tags, zones, boundary
      capture-point config (repeatable seed).
- [ ] **Geofence rule config**: which assets, which boundary scan_point, RSSI
      threshold.
- [ ] **Cold-start checklist** (edge box / TRA-898): bring-up order, what
      "green" looks like.
- [ ] **Verify-green checklist** before a run.
- [ ] **Walk-through sequence**: the actual demo choreography (normal carry +
      concealed/bag carry).
- [ ] **Recovery**: what to do if a reader/alarm/ingest looks wrong.
- [ ] **RSSI threshold + antenna placement tuning** against a monitor carried
      through a realistic doorway (the real demo prep).
- [ ] One-page **laminated** condensation of the above for Tim.
