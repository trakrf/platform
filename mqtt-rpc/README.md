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
| `trakrf.id/{client_id}/status` | daemon → cloud | retained online/offline status + MQTT LWT |

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

On startup the daemon **converges the reader to the golden `TrakRF mqtt-rpc`-named
entities** — the exact ingest wiring validated in TRA-994 — so every reader provably
has what ingest needs, idempotently and safe to re-run. The golden definitions are
owned as code (`internal/readerd/cs463/golden.go`); reconcile is list-then-add-or-mod.

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
- `READERD_RECONCILE` — `true` (default) runs reconcile on startup; `false` pins config.
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

## Remote update (`Daemon.Update`, TRA-1317)

Customer readers have no SSH; they are reachable only through the broker. So
after one SSH install (`deploy/edge/mqtt-rpc/install.sh`) every later daemon
build arrives over RPC, and a bad build must never strand the reader.

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
   30 s. Once it has answered an RPC through the broker — that one or any cloud
   RPC — and its own updater preflight passes, it commits (removes `pending`).
   The commit criterion is deliberately **not** "reconcile succeeded": a wedged
   vendor app (`/API` answering 200 with an empty body) would otherwise roll back
   the very build sent to fix it.
5. **Rollback** is the guard's job, not the daemon's — a broken build cannot be
   trusted to undo itself. `deploy/edge/mqtt-rpc/mqtt-rpcd-guard` (POSIX sh,
   installed only by `install.sh`, never updated over the air):
   - `ExecStartPre` counts starts while pending; more than 5 = crash loop.
   - A 5-minute timer counts ticks while pending; 4 = no commit (15–20 min,
     enough for the ~7 min first broker connect after a boot).
   - Either repoints the link at `from`, deletes the failed version, records
     `update/last` and restarts. Commit and rollback both claim `pending` by
     rename, so exactly one wins.

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
reader runs it, ship one trusting `{new, offline}` signed by the new key. Readers
only ever trust the keys of the build they run, so a key that leaves the list
stops working fleet-wide as soon as the fleet has updated. `mqtt-rpcctl keygen`
makes a key pair.

### Recovery, cheapest first
1. Automatic: the guard (above).
2. Over RPC: `Daemon.Update` to any other signed version — downgrades allowed.
3. SSH from the reader's LAN: `deploy/edge/mqtt-rpc/rollback.sh <host> [version]`
   (on the reader: `/opt/trakrf/mqtt-rpcd-guard list|rollback <version>`). The
   newest 3 versions are retained, plus the active and previous ones.
4. `install.sh` again.
