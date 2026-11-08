# mqtt-rpc — MQTT-RPC reader control daemon (`mqtt-rpcd`)

A small, self-contained Go daemon that puts a fixed RFID reader behind a neutral
**MQTT JSON-RPC control contract**. It runs *on* the reader (or alongside it),
subscribes to an RPC topic, translates calls to the reader's native config API,
and replies over MQTT. The cloud never speaks the reader's native protocol — only
this contract.

First (and currently only) adapter: **CSL CS463** (Indy RS2000), which has no
inbound control channel reachable from the cloud — so control is brokered:
`cloud → MQTT RPC → mqtt-rpcd (on the reader) → localhost HTTP/servlet`.

Build: `go build ./cmd/mqtt-rpcd`. Deploy: see `/deploy/edge/mqtt-rpc/`.

---

## The contract (the durable artifact)

The contract is **the wire format below**, not a shared library. Each side (this
daemon, the TrakRF cloud backend, a future Rust adapter) implements these JSON
shapes independently. Conformance is guaranteed by the integration test, not by a
shared Go package — so a port to another language changes nothing here.

### Topics (built on the reader's reads topic `trakrf.id/{client_id}/reads`)
| topic | direction | purpose |
|---|---|---|
| `trakrf.id/{client_id}/rpc` | cloud → daemon | RPC requests (daemon subscribes) |
| *(the request's `src`)* | daemon → cloud | response, routed to the caller's `src` topic |
| `trakrf.id/{client_id}/status` | daemon → cloud | retained status document (presence, version, reader-API health) + MQTT LWT; see "Reader-API health and wedge recovery" |

The daemon connects with a **distinct** MQTT client id (`{client_id}-rpc`) so it
never evicts the reader's own reads connection.

### Frame (Shelly Gen4-style JSON-RPC)
```jsonc
// request → …/rpc
{"id": 42, "src": "trakrf-cloud/<inst>/reply/42", "method": "Reader.SetConfig",
 "params": {"tx_power_dbm": [{"antenna": 1, "power": 30.0}]}}

// success → topic named in src
{"id": 42, "dst": "trakrf-cloud/<inst>/reply/42",
 "result": {"applied": "pending_reload", "effective_at": "next_inventory_cycle"}}

// error → topic named in src   (JSON-RPC codes: -32601 method, -32602 params, -32603 internal)
{"id": 42, "dst": "trakrf-cloud/<inst>/reply/42",
 "error": {"code": -32603, "message": "…"}}
```

### Methods
**Portable core** (identical value-semantics across readers):
- `Reader.GetCapabilities` → `{contract_version, reader_model, antennas, tx_power:{min_dbm,max_dbm,per_antenna}, supports[], unsupported[], daemon:{version, update?:{state,target,error,at}}}` — static, never touches the vendor `/API`
- `Reader.GetConfig` → `{tx_power_dbm:[{antenna,power}], …}`
- `Reader.SetConfig(params=ReaderConfig, partial)` → `{applied: "immediate"|"pending_reload", effective_at?}`
- `Reader.GetStatus` → `{online, reading, active_profile?}`

**Capability-flagged management** (reserved; declared `unsupported` by the CS463 adapter): `Scan.Start`, `Scan.Stop`, `Gpo.Set`, `Reader.Reboot`.

**Daemon** (acts on mqtt-rpcd itself; never touches the vendor `/API`):
- `Daemon.Update(params={version, url, sha256, signature})` → `{accepted, from, to}` once verified; the install follows the reply. Refusals are error `-33002`. See "Remote update".

Rules: neutral config (no vendor terms like "operation profile" leak into the
wire); a new tunable is a field, not a method; `applied` is explicit; the contract
is versioned via `contract_version`.

### CS463 adapter specifics
- TX-power range is **10.0–31.5 dBm, step 0.5** (Indy RS2000 operational range;
  the firmware's raw field accepts 0–32 but that is not the calibrated envelope).
- `SetConfig` is `applied: pending_reload`: it writes the reader's active
  operation profile via the web **servlet** (`OperationProfileDetail`, cookie
  auth — the `/API` `setOperProfile` can't set antenna enablement on web-app
  1.3.23), then re-arms the inventory event so reads resume on the new config
  (a brief read pause), and verifies antenna enablement survived the write.

---

## Validating the contract (integration test)

The contract is proven end-to-end against real hardware, not via shared types:

1. Deploy `mqtt-rpcd` to a CS463 (`/deploy/edge/mqtt-rpc/install.sh`), pointed at
   a broker.
2. Publish `Reader.GetCapabilities` / `GetConfig` / `SetConfig` to
   `trakrf.id/{client_id}/rpc` with a `src` reply topic; assert the responses.
3. For `SetConfig`, verify the change landed on the reader independently of the
   RPC reply by reading `/opt/EmbeddedGlassFish/config/OperationProfileCS463`
   (`antennaTransmitPower` = dBm×10, `enableAntenna` = bool[16]) over SSH —
   contention-free (no `/API` login needed).
4. Restore the baseline.

This was run live on `cs463-212` through the preview broker
(`mqtt.preview.gke.trakrf.id`) with the TrakRF cloud backend as the RPC client
(see PR #495). The Go unit tests in this module cover the adapter, servlet
encoding, single-session auth sequencing, and the wipe guard against fakes; the
live test covers the wire + the firmware behavior the fakes can't.

---

## Golden-config reconcile (TRA-1002)

Once the reader's `/API` answers, the daemon **converges the reader to the golden `TrakRF mqtt-rpc`-named
entities** — the exact ingest wiring validated in TRA-994 — so every reader provably
has what ingest needs, idempotently and safe to re-run. The golden definitions are
owned as code (`internal/readerd/cs463/golden.go`); reconcile is list-then-add-or-mod.
The health monitor decides when it runs: when `/API` first answers after the daemon
starts, again after every outage, and every 5 min while one keeps failing. See
"Reader-API health and wedge recovery".

### Owned entities (all `TrakRF mqtt-rpc`-prefixed — the name *is* the ownership claim)
| Entity | Name | Reconciled |
|---|---|---|
| MQTT Server (CloudServer) | `TrakRF mqtt-rpc MQTT Server` | **No — hand-crafted out-of-band** (needs broker creds + TLS cert), referenced by name |
| Operation Profile | `TrakRF mqtt-rpc Profile` | **Create-if-absent** — daemon creates it (`setOperProfile` + servlet) with an **antenna-1-only @ 30 dBm** default (dwell 500 on every slot); **left untouched if it already exists** so operator `SetConfig` tuning survives |
| Data Format | `TrakRF mqtt-rpc Data Format` | Yes — trimmed JSON, numeric RSSI (`RSSI_Number`, parser PR #502) |
| Trigger | `TrakRF mqtt-rpc Trigger` | Yes — reader-side RSSI gate (`≥ -80 dBm` knob), all antennas |
| Resultant Action | `TrakRF mqtt-rpc Action` | Yes — MQTT → server + format |
| Event | `TrakRF mqtt-rpc Event` | Yes — dedup=500ms, antennaDifferentiation=on, enable=on |

**Ownership boundary: the daemon mutates ONLY `TrakRF mqtt-rpc *` entities** — never
stock/`Default`/`Example` entities. For a conflict it cannot own (e.g. another enabled
MQTT-publishing event that would double-publish), it *warns the operator*, it does not
disable or modify the foreign entity.

Reads/verify use `/API list*`; writes use `/API add*`/`mod*`. The write transport is
pluggable per entity (the `entitySpec` seam) so any entity can flip to the servlet
path if a firmware proves an `/API` write unreliable — **no firmware floor is
required**. Reconcile **always re-arms** (disable→enable) the golden event on startup,
defensively: the CS463 doesn't *reliably* auto-start inventory — an event with
`enable=true` sometimes publishes nothing until a disable→enable cycle kicks the engine
(operator-confirmed; a bare `enable(true)` is a no-op). A clean boot often auto-starts,
but the daemon can't tell, so it always arms to guarantee reads (cost: one inventory
cycle if reads were already flowing). (A future on-demand `Reader.Reconcile` RPC against
an already-reading reader should gate the re-arm on whether config changed.)

### Commissioning prerequisites (done in the same SSH session that installs the daemon)
1. Hand-craft the `TrakRF mqtt-rpc MQTT Server` CloudServer entry — `setServerID` (broker
   host/port/TLS/creds + the platform `scan_device.publish_topic`) plus `setServerCertificate`
   to upload the broker CA cert. The daemon reads it for its own broker connection and the
   golden chain references it by name.
2. **No profile to create** — the daemon creates `TrakRF mqtt-rpc Profile` itself on first
   reconcile (antenna-1-only @ 30 dBm default, dwell 500 on every slot), then leaves it
   alone. Tune antennas/TX-power per-reader afterward via `Reader.SetConfig`. The stock
   `Default Profile` is never touched.
3. Existing readers provisioned under the old `TrakRF MQTT` name must set
   `READERD_CLOUDSERVER_ID` until migrated (the default is now the golden name).

Enabling the golden event makes `TrakRF mqtt-rpc Profile` the active profile (the reader's
activation mechanism); on reboot the reader activates the enabled event's profile, but
inventory still needs the startup re-arm (above) to begin publishing.

### Config
- `READERD_RECONCILE` — `true` (default) runs reconcile whenever `/API` is healthy and the config is not yet applied; `false` pins config.
- `READERD_CLOUDSERVER_ID` — defaults to `TrakRF mqtt-rpc MQTT Server`.

### Bench verification (cs463-212, 2026-06-17) — VALIDATED on live hardware
Unit tests cover parsing, drift, the reconcile decision table, re-arm gating, and the
dwell fix against fakes. The full path was then run against the **live reader** (the
guarded `TestLiveReconcile`, `CS463_LIVE=1`), additive and self-restoring:
- [x] **Read parsing** — all five `list*` + `getOperProfile` parse correctly; real
      captures pinned as CI fixtures (`testdata/cs463-212_*.xml`, `realcapture_test.go`).
- [x] **Read-side diffs match reality** — `eventDrift`/`actionDrift`/`dataFormatDrift`/
      `triggerDrift` read the exact live attr names/casing; `action_mode` comes back as
      the human form `"Low Latency Alert to Server"`; golden Data Format is field-identical
      to the live `TrakRF-data-format`.
- [x] **`/API` write contract** — all four entity `add`/`mod`/`list`/`del` succeed on
      hardware with our exact golden params (`setOperProfile`'s footgun does not extend
      to event-engine entities).
- [x] **Full reconcile lifecycle** (real `reconcileGolden`/`Adapter.Reconcile` driving
      the reader): create → `changed=true` + all four present, no drift; second run →
      `changed=false` (idempotent no-op); injected dedup drift → `changed=true` + event
      reconciled back to 500; `verifyServerAndProfile` passes with prereqs / fails loudly
      without.
- [x] **End-to-end commission + reboot** — cs463-212 was stripped to stock (all
      non-`Example*` entities removed), the golden MQTT Server hand-crafted (`setServerID`
      + `setServerCertificate` → real preview broker), and the daemon's real
      `Adapter.Reconcile` stood up the golden chain on the stock `Default Profile`. After a
      **reboot** the reader came back **self-sufficient off the golden set alone**: active
      profile `Default Profile`, all five golden entities present, golden event enabled,
      both firmware + daemon MQTT clients reconnected to preview — zero dependency on any
      deleted old entity.

To re-run on a reader: `CS463_LIVE=1 CS463_IP=… CS463_WEB_PASS=… go test ./internal/readerd/cs463/ -run TestLiveReconcile`.

Open (non-blocking): **partial-failure policy** — reconcile aborts on first entity
error (logged, non-fatal to RPC); revisit if a real reader ever needs
best-effort-continue.

---

## Reader-API health and wedge recovery

The CS463 vendor app can come up **answering every `/API` request with HTTP 200
and an empty body**. When it does, the radio keeps reading, systemd says
`active`, the daemon stays on the broker, and no reads reach the cloud. Clean
reboots have caused it as well as power cuts. GlassFish's classloader throws
`NullPointerException` in `WebDirContext.lookupFromJars` on every request. The
daemon detects this itself and recovers from it.

### Probe
Every 30 s the daemon sends `GET /API?command=directIOInput&username=…&password=…`.
It is sessionless, so it never takes the reader's single session slot (a
`command=login` probe would). It mutates nothing, and it goes through the same
servlet path that fails. **The reply is judged on its parsed body, never on its
status code:**

| `api` | Means |
|---|---|
| `healthy` | `<CSL><Command>directIOInput</Command>` with `<Input …/>`, or with `<Error …/>` (a wrong password still means the servlet is serving, and no restart fixes a credential) |
| `degraded` | a 2xx that is empty, unparseable, or names another command. **This is the wedge.** |
| `unreachable` | no connection, or a non-2xx (nginx answers 502 while GlassFish is down or still deploying) |
| `unknown` | not probed yet |

Detection rests only on `/API` content. It does not scrape `lookupFromJars` from
the log, and a slow broker connection after a boot is not a wedge.

### Recovery ladder
`/API` unhealthy for **6 min** without a break (`READERD_WEDGE_AFTER`; a boot
takes about 3 min to serve) marks the reader `wedged`. Then, once per episode:

1. **Restart the vendor app.** `systemctl stop embeddedglassfish`, then SIGTERM
   whatever is left in the unit's cgroup, then SIGKILL after 30 s, then `start`.
   `systemctl restart` alone is not enough here. The vendor init script stops
   only the pid in a pidfile that is not always written, and the generated unit
   is `KillMode=process` with no main pid, so a plain restart can start a second
   java beside the first. If a process survives SIGKILL, the daemon does not
   start the app.
2. **Still unhealthy 6 min later: a clean reboot.** The daemon records the
   reboot, publishes the retained status offline with `reason: "auto-recovery"`,
   then runs `sync` and `systemctl reboot`. It never power-resets.

**Boot-loop cap:** at most **2** automatic reboots, counted in
`/opt/trakrf/recovery/auto-reboots.json`, which survives the reboots it counts.
The count resets after **1 h** of continuous health, measured on the process
clock. It is not a wall-clock window, because the CS463 RTC has booted at 2019
before NTP corrected it, and a timestamp window would then expire every entry.
Past the cap, or if the file cannot be read, the daemon stays up and reports
`auto_reboots_exhausted: true`, and a person decides.

Every unhealthy → healthy transition (two or more failed probes) re-runs the
golden-config reconcile, because an app restart stops inventory and the
reconcile re-arms it.

`READERD_AUTO_RECOVERY=false` keeps the probe, the status and the reconcile
retry, but takes no recovery action. Use it on a bench that has to hold a wedge
for diagnosis.

### Status document (retained `<base>/status`)
```jsonc
{"online": true, "at": "2026-11-08T17:40:00Z", "version": "v1.5.0-330-g…",
 "health": {"api": "degraded", "since": "2026-11-08T17:31:00Z",
            "detail": "http 200 with an empty body", "wedged": true,
            "recovery": "app_restart",            // or "reboot"; absent while healthy
            "auto_reboots": 1, "auto_reboots_exhausted": false,
            "golden_config": "pending",           // "applied" | "pending" | "off"
            "cloudserver_fault": "cleanSession is false; golden is true"}}  // absent when clean
// before an automatic reboot, and after it via Stop:
{"online": false, "reason": "auto-recovery", "version": "…", "health": {…}}
// MQTT Last Will (the daemon died without Stop):
{"online": false}
```
`cloudserver_fault` names every way the golden CloudServer would stop the
reader's own app publishing reads: a trust bundle CSL cannot use, or drift from
qos 0, cleanSession or TLSv1.2. The rules and the reasons for them are in
`deploy/edge/mqtt-rpc/README.md` ("The golden CloudServer"). It is re-checked on
every probe, independently of `/API`.

It is published when the health changes and on every reconnect, and not on
every probe. To read it: `MQTT_URL=… mqtt-rpcctl status -base <base topic>`
prints the retained document, and `-watch` prints one line per change.

---

## Remote update (`Daemon.Update`, TRA-1317)

Customer readers have no SSH; they are reachable only through the broker. So
after one SSH install (`deploy/edge/mqtt-rpc/install.sh`) every later daemon
build arrives over RPC, and a bad build must never strand the reader.
Why the rollback is a separate never-updated script, and why commit ignores
reconcile: `docs/adr/0024-a-remote-update-is-undone-by-what-it-cannot-replace.md`.

### Flow
1. **Request.** `Daemon.Update {version, url, sha256, signature}` — in practice a
   release's `manifest.json`, relayed by the backend
   (`POST /api/v1/scan-devices/{id}/reader-software/update {version}`) or sent by
   `mqtt-rpcctl update -manifest <url>`.
2. **Verify before touching anything** (refused with `-33002` otherwise): version
   format, `https://` URL, not the running version, an Ed25519 signature over
   `trakrf-mqtt-rpcd-release-v1\nversion=<v>\nsha256=<hex>\n` by a key compiled into
   the running binary, the rollback guard and its timer installed, no update
   already in flight or on probation. The reply goes out here.
3. **Install.** Download (https only, redirects included, 64 MiB cap) into
   `versions/`, hash, compare, fsync. Write `update/pending {target, from}` —
   arming the guard — then swap the `mqtt-rpcd` symlink atomically and exit;
   systemd restarts onto the new version. The new binary is **never executed** to
   check it: mqtt-rpcd has no flags, so running it starts a second daemon.
4. **Probation → commit.** The new daemon sends itself `Reader.GetCapabilities`
   through the broker (to its own `<base>/rpc`, reply to `<base>/selftest`) every
   30 s. Once it has been up 3 minutes and answers an RPC through the broker —
   that one or any cloud RPC — and its own updater preflight passes, it commits
   (removes `pending`). The uptime floor catches a build that connects and then
   crashes (a crash resets it, so the start count rolls it back) or wedges (the
   answers stop, so the tick count does).
   The commit criterion is deliberately **not** "reconcile succeeded": a wedged
   vendor app (`/API` answering 200 with an empty body) would otherwise roll back
   the very build sent to fix it.
5. **Rollback** is the guard's job, not the daemon's — a broken build cannot be
   trusted to undo itself. `deploy/edge/mqtt-rpc/mqtt-rpcd-guard` (POSIX sh,
   installed only by `install.sh`, never updated over the air):
   - `ExecStartPre` counts starts while pending; more than 5 = crash loop.
   - A 5-minute timer counts ticks while pending; 4 = no commit. Every start
     resets the tick count, so each boot gets the full 15–20 min — enough for the
     ~7 min first broker connect after a boot.
   - Either repoints the link at `from` and, only once the link has verifiably
     moved and been synced, deletes the failed version, records `update/last` and
     restarts. If the link cannot be moved, nothing is deleted or recorded and
     `pending` stays armed for the next run; a rollback interrupted mid-way
     resumes. Commit and rollback both claim `pending` by rename, so exactly one
     wins.

`update/last` is reported in `GetCapabilities().daemon.update`:
`committed`, `rolled_back` (`error`: `crash_loop` | `no_commit` | `manual`) or
`failed` (nothing replaced). The updater never reads or writes
`/opt/trakrf/mqtt-rpcd.env` — it holds the only on-device copy of the reader
password.

### Releases
`.github/workflows/mqtt-rpcd-release.yml` (manual dispatch) tests, cross-builds
`linux/armv7` static, signs, and publishes a GitHub **prerelease**
`mqtt-rpcd-<version>` with `mqtt-rpcd-linux-armv7` and `manifest.json`.
`<version>` is `git describe --tags --match 'v[0-9]*'` of the built commit.
`mqtt-rpcctl sign` refuses a binary whose `-X main.version` (read from its Go
build info) is not the version being signed, so release builds must not use
`-trimpath`, which drops that info.
The backend resolves versions under `MQTT_RPCD_RELEASE_BASE_URL` (default
`https://github.com/trakrf/platform/releases/download`).

**Release rule:** a build goes to customer readers only after it has itself
updated a bench reader (cs463-212) to a newer build. Commit proves the new build
connects; only that step proves its *updater* works.

### Signing keys
Two Ed25519 keys are compiled in (`internal/update/keys.go`); a release signed by
either is accepted.
- `ci-2026-11` — private half is the GitHub Actions secret
  `MQTT_RPCD_SIGNING_KEY` (base64 seed). Signs every CI release.
- `offline-2026-11` — private half lives offline (password manager), never in CI.
  It signs a release if the CI key is lost or compromised.

**Rotation:** ship a build trusting `{old, new}` signed by the old key; once every
reader runs it, ship one trusting `{new, offline}` signed by the new key.
`mqtt-rpcctl keygen` makes a key pair.

**Rotation is not revocation.** Downgrades are allowed and every release stays
public, so whoever holds a retired key *and* can publish on the broker can
replay an older genuine release that still trusts that key, then install a build
signed with it. Dropping a key from the list only retires it for honest use.
Revoking a compromised key needs a further build with a rollback floor (a
minimum version or a key epoch) — not implemented. The release URL is not
signed either, so a replayed manifest can point the reader's download at any
https URL; the download is hashed and fails closed, but the reader does make the
request. Broker publish rights are therefore part of the trust boundary.

### Recovery, cheapest first
1. Automatic: the guard (above).
2. Over RPC: `Daemon.Update` to any other signed version — downgrades allowed.
3. SSH from the reader's LAN: `deploy/edge/mqtt-rpc/rollback.sh <host> [version]`
   (on the reader: `/opt/trakrf/mqtt-rpcd-guard list|rollback <version>`). The
   newest 3 versions are retained, plus the active and previous ones.
4. On site, no developer: `ssh recover@<reader-ip>` runs
   `mqtt-rpcd-guard recover`: fetch the newest release from GitHub (hash-checked,
   not signature-checked), switch to it, reboot cleanly. See
   `deploy/edge/mqtt-rpc/README.md`.
5. `install.sh` again.
