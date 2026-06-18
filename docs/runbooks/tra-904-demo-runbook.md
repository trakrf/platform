# TRA-904 — Frederick Health demo runbook (ENGINEER / Mike version)

> **Status: DRAFT for review (2026-06-17).** This is the **engineer** runbook —
> it assumes shell access (podman / systemctl / SQL / curl) and is for Mike, not
> Tim. **Tim's version is a separate file:** `tra-904-tim-demo-card.md` — app
> surface + physical connections only, no shell. Keep them in sync.
>
> This is a rough first pass that brings the runbook up to the current system
> after a lot of fixed-reader work landed since the seed (PR #452, 2026-06-04).
> Items needing a decision are tagged **[REVIEW]**; items that can only be settled
> with the rig in hand are tagged **[TUNE ON-SITE]**.
>
> Epic: [TRA-897](https://linear.app/trakrf/issue/TRA-897) · this ticket:
> [TRA-904](https://linear.app/trakrf/issue/TRA-904). Edge box: [TRA-898](https://linear.app/trakrf/issue/TRA-898)
> (Done) · demo-tag/restart-safety: [TRA-966](https://linear.app/trakrf/issue/TRA-966) (last to land).

---

## What changed since the seed (orientation for reviewers)

The seed runbook predates these — they change the words Tim sees and where he clicks:

- **"Alarm Devices" → "Outputs"**, and the whole fixed-reader surface moved under
  **Settings** (TRA-929 rename, TRA-930 nav): **Settings → Readers / Outputs / Live feed**.
- **`is_boundary` is gone** (TRA-943). There is no "boundary scan_point" flag
  anymore. The geofence engine resolves **location → outputs**, and each **output**
  carries a **mode** (`egress` | `presence`). For this demo the door output is
  **`egress`**.
- **RSSI / age-out / auto-off are per-entity config**, not global env (TRA-955).
  Three tiers, most specific wins: **code default → org default → per-output**.
  Tuning the demo = setting these on the door output (or org), not a redeploy.
- **Reader auto-provisions** (TRA-1002 daemon golden config / TRA-1015 self-bootstrap):
  the CS463, on commission, gets a working TrakRF profile pushed to it. We mostly
  *verify* the reader rather than hand-build its config.
- **EPC normalization** (TRA-944): a tag registered by its short barcode value
  (e.g. `10023`) still matches the full-width EPC the reader emits. Less foot-gun
  when entering the armed tags.
- Ingest matches a read to its capture point by **(reader, antenna_port)** now
  (TRA-956), not a capturePointName string.

---

## 0. The demo in one breath

A registered, tagged monitor carried through the doorway makes the CS463 read it
at the door antenna; the in-box backend sees the read cross the RSSI threshold for
an **armed** asset at an **egress** output and fires the **Shelly Gen4 strobe**
over local HTTP within ~1s. Unregistered tags passing through do **not** fire.
Manual reset clears the strobe between runs. The whole thing runs **offline** on
the HP EliteDesk (`trakrf-demo`); Tim drives from his laptop on the Slate WiFi.

```
CS463 (UHF) ──MQTT :1883──> Mosquitto ──> backend Go subscriber
                                              │  raw → tag_scans → asset_scans
                                              │  geofence eval (in-process)
                                              └──HTTP──> Shelly Gen4 strobe
                          all on the box (192.168.8.10), no cloud, no DB round-trip
```

Tim's laptop → **https://app.demo.trakrf.id** (resolved by the Slate to the box).

---

## 1. Demo data preload  **[REVIEW — no repeatable fixture exists yet]**

> **Gap to settle with Tim:** there is currently **no demo-data seed fixture**.
> `deploy/edge/smoke-test.sh` borrows `contract_test_seed.sql` only to prove the
> ingest path; it explicitly notes asset_scans derivation reads 0 until a real
> scan_point + output exist. For Friday we either (a) provision once by hand via
> the UI and rely on the box's nightly `pg_dump` backup as the "save", or (b)
> write a small idempotent `demo-seed.sql` so a wipe/rebuild is one command.
> **Recommendation: (a) for Friday, (b) as the immediate follow-up** so the demo
> is truly repeatable from scratch. Decide tomorrow.

What has to exist for a fire (provision in this order, all at `app.demo.trakrf.id`):

1. **Org** — the demo org (the box is single-tenant; **[REVIEW]** confirm the org
   name/slug Tim wants, since slug becomes the MQTT topic root, TRA-922).
2. **Location** — the doorway location (e.g. "Main Door"). The geofence ties an
   asset read to outputs **through the location**.
3. **Reader (Settings → Readers)** — the CS463. Should appear / auto-provision on
   commission (TRA-1002). Set its **location** to the doorway location. Confirm a
   **scan_point** exists for the door antenna (antenna_port 1).
4. **Assets + tags** — the monitors to arm. Register each asset, then add its
   **tag** (the EPC). Short barcode value is fine (TRA-944). These armed EPCs are
   the membership set: only these fire.
5. **Output (Settings → Outputs)** — the Shelly strobe. **Transport = HTTP**,
   **Base URL** = the Shelly's LAN address, **Switch ID** = `0`, **Location** =
   the doorway location, **Mode = egress**. (Full Shelly provisioning §3.)
6. **One "decoy" unregistered tag** to demonstrate the negative case (passerby
   goods don't alarm). Just an EPC that is **not** registered to any asset.

---

## 2. Geofence rule config

There is no separate "rule" object — the rule **is** the wiring above plus the
thresholds on the output:

- **Who can fire:** assets that have a registered tag (membership set). Everything
  else is ignored.
- **Where:** the door **location** links the reader's reads to the door **output**.
- **When:** the output's **mode = egress** fires immediately on a qualifying read.
- **How sensitive:** **RSSI threshold** + **age-out** + **auto-off** on the output
  (or org default). See tuning, §5.

**[REVIEW]** exact field names / where each tier lives (`output_devices.metadata`
vs org `metadata.geofence_defaults`) — I described these from the shipped tier
model (TRA-955) but did not re-read the code for this draft. Verify before laminating.

---

## 3. Output (Shelly Gen4) provisioning

The edge box and the Shelly are on the **same LAN**, so the demo uses the **HTTP**
fire path (backend → device over local HTTP). The MQTT path (TRA-906) exists for
cloud/firewalled deployments and is documented at the end for completeness — it is
**not** the Friday path.

### HTTP path (the demo path)

1. Note the Shelly's local address, e.g. `http://192.168.8.66`.
2. **Settings → Outputs → New**: **Transport = HTTP**, **Base URL** = device
   address, **Switch ID** = relay channel (usually `0`), **Location** = the
   doorway location, **Mode = egress**.
3. **Test-fire** from the row — the relay pulses on→off (~2s). A **502** means the
   backend can't reach the device (wrong network / IP / device down) — on the box
   this is a real reachability failure, fix it before continuing.

### Auto-off / latch

- **Auto-off** (TRA-934): set `auto_off_seconds` on the output so the strobe
  self-clears after N seconds (device-side `toggle_after`). Good for a hands-off
  demo loop. **[TUNE ON-SITE]** pick a duration that reads well in the room.
- If you run **latching** instead (strobe stays on until reset), use the **Reset**
  button on the output row between runs.
- Gen4 note (TRA-941): the backend already sends the required `src` on the RPC;
  no operator action — just don't be surprised the device is a Gen4.

---

## 4. Cold-start checklist (edge box)

On a box whose DB volume is already initialized, **a power-on self-starts the whole
stack** (systemd linger + `Restart=always`). Tim's normal path is just: power on,
wait ~1–2 min, verify green.

**Green = all five containers up + health 200 + a synthetic read ingests.**

```bash
# on the box (shell over the tailnet, or locally):
podman ps --format '{{.Names}} {{.Status}}'   # expect: timescaledb mosquitto backend traefik cloudflared, all Up
curl -fsS http://127.0.0.1:8080/health        # expect: 200
deploy/edge/smoke-test.sh                      # expect: "PASS: broker -> subscriber -> ingest proven"
```

Then from Tim's laptop: browse **https://app.demo.trakrf.id**, log in, open
**Settings → Live feed** and confirm reads appear when a tag is near the antenna.

> First-time / fresh-box bring-up (secrets, db-init, TLS cert) is **not** Tim's
> job — that's `deploy/edge/README.md`. This runbook assumes a box Mike has
> already brought up and handed over.

---

## 5. Verify-green checklist (before each run / before Tim's audience walks up)

- [ ] `podman ps` → 5 containers **Up** (not Restarting).
- [ ] `curl /health` → **200**.
- [ ] **Live feed** shows reads when an armed tag is held near the door antenna.
- [ ] **Test-fire** the door output once → strobe pulses → **Reset** it.
- [ ] Strobe is **off / reset** (no leftover latch from a prior run).
- [ ] Armed tags are on the assets you'll carry; the **decoy** tag is on nothing.

---

## 6. Walk-through choreography  **[REVIEW with Tim — this is his script]**

Suggested sequence (Tim refines for his audience):

1. **Show normal state** — Live feed quiet, strobe off. "Nothing tagged is leaving."
2. **The catch** — carry the registered monitor through the doorway at a normal
   walking pace. Strobe fires within ~1s. "That monitor just walked out — caught."
3. **Reset** — clear the strobe (auto-off or Reset button). Back to quiet.
4. **The negative** — carry the **decoy** (unregistered) tag through. **No alarm.**
   "Random tagged goods don't cry wolf — only your tracked assets."
5. **The hard case [TUNE ON-SITE]** — concealed carry: monitor in a bag / against
   the body. This is the real catch-rate story. Tune §5 thresholds so this fires
   reliably; if it's marginal, set the honest 80–90% expectation (per the epic:
   ROI is device-replacement cost, so 80–90% is already a win).

---

## 7. Manual reset between runs

- **Auto-off configured:** wait for it; nothing to do.
- **Latching:** **Settings → Outputs → door row → Reset** (or, on the box,
  publish device-off — see MQTT appendix).
- If a fire seems "stuck," it's the latch, not a hang — reset clears it.

---

## 8. Recovery — "something looks wrong"

| Symptom | Likely cause | Fix |
|---|---|---|
| No reads in Live feed | reader down / not commissioned / wrong topic | power-cycle the CS463; check `Settings → Readers` shows it; give it ~1–2 min (GlassFish boot is slow) |
| Reads appear but **no fire** | tag not registered, wrong location wiring, RSSI too strict | confirm the asset+tag exist; output **mode=egress** at the **same location** as the reader; loosen RSSI (§5) |
| Test-fire **502** | backend can't reach Shelly | check Shelly power/IP/LAN; ping it from the box |
| Fires for **everything** | RSSI too loose / decoy actually registered | tighten RSSI; confirm decoy tag is unregistered |
| Strobe won't turn off | latched | Reset on the output row |
| Whole UI unreachable | box forward wedged / service down | `podman ps`; the rootlessport watchdog usually self-heals in ~30s; else `systemctl --user restart <svc>` |
| Box rebooted mid-demo | power blip | it self-starts (~1–2 min); re-run §5 |

Break-glass shell is over the tailnet: `systemctl --user …`, `journalctl --user -u <svc>`, `podman …`.

---

## 9. RSSI threshold + antenna placement tuning  **[TUNE ON-SITE — the real demo prep]**

This is the part that can't be written from a desk. Procedure:

1. Mount the antenna at the doorway; aim across the threshold, not down the hallway.
2. Open **Settings → Live feed**, watch **RSSI** as a body carries the monitor
   through at walking pace — normal carry, then bag/body-concealed carry.
3. Set the output **RSSI threshold** just **below** the weakest reliable
   concealed-carry read, and just **above** the ambient/standing-nearby reads, so
   walking *through* fires but standing *near* does not.
4. Set **age-out** so a single pass = one fire (no chatter), and **auto-off** to a
   duration that reads well.
5. Repeat until normal carry is ~100% and concealed carry is as high as the
   placement allows.

Starting point from prior live validation (cs463-212 rig): gate near **−65 dBm**
at close range / lower TX power — **but treat that as a guess for this room**; the
doorway geometry and antenna mount will move it.

Record the final values here once tuned:

- Antenna placement: _______
- TX power: _______
- Output RSSI threshold: _______  age-out: _______  auto-off: _______

---

## Appendix — MQTT fire path (NOT the Friday demo; cloud/firewalled only)

Kept for completeness; the edge box uses HTTP (§3). The MQTT path (TRA-906) is for
a cloud backend that can't reach a LAN relay over HTTP.

- Broker: `mqtt.preview.gke.trakrf.id:8883` (preview) / `…prod…` (prod), TLS 1.2,
  shared `trakrf-mqtt` creds (from the `trakrf-mosquitto-auth` secret).
- Shelly MQTT config (web UI or `MQTT.SetConfig`): server + TLS on, **CA cert** =
  the Let's Encrypt chain (leaf → YE2 → ISRG root) — same gotcha as the GL-S10
  readers; topic prefix e.g. `{org_slug}/dock-strobe`; enable MQTT control.
- Register output **Transport = MQTT**, **Command Topic** = the device prefix.
  Backend publishes to `<command_topic>/command/switch:<switch_id>`.
- Validate device-side independently (no backend):

  ```bash
  mosquitto_pub -h mqtt.preview.gke.trakrf.id -p 8883 --cafile <le-ca.pem> \
    -u trakrf-mqtt -P '<pass>' \
    -t '{org_slug}/dock-strobe/command/switch:0' -m on   # -m off to clear
  ```
- Test-fire semantics differ: HTTP is a real round-trip (502 = unreachable); MQTT
  is publish-and-trust (green = broker accepted, not relay-confirmed).

---

## ─────────  ENGINEER QUICK CARD (Mike — has shell)  ─────────

> Tim's laminated card is the separate `tra-904-tim-demo-card.md`. This one has
> shell commands and is for Mike.

**TrakRF egress demo — engineer card**     Box: `trakrf-demo` @ `192.168.8.10` (offline)
Laptop (Chrome, Secure DNS **off**) → **https://app.demo.trakrf.id**

**START**
1. Power on box → wait ~1–2 min.
2. `podman ps` → 5 containers **Up** · `curl -fsS http://127.0.0.1:8080/health` → 200
3. App → **Settings → Live feed**: reads show when an armed tag is at the door.
4. **Settings → Outputs → door → Test-fire** → strobe pulses → **Reset**.

**RUN**
- Carry **registered** monitor through door → **strobe fires <1s**.
- Reset (auto-off, or Outputs → Reset).
- Carry **decoy** (unregistered) tag → **no alarm** (this is the point).
- Concealed/bag carry → catch-rate story (honest 80–90% if marginal).

**IF WRONG**
- No reads → power-cycle CS463, wait ~2 min.
- Reads but no fire → tag registered? output **egress** + same **location**? RSSI too tight?
- Fires for everything → RSSI too loose / decoy is registered.
- 502 on test-fire → Shelly power/IP/LAN.
- Strobe stuck on → it's the latch → **Reset**.
- UI dead → wait 30s (self-heals); else break-glass shell: `systemctl --user restart <svc>`.

**NEVER** push updates / go online during a demo window (box is offline by design).
