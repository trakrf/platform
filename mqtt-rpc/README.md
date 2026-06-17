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
- `Reader.GetCapabilities` → `{contract_version, reader_model, antennas, tx_power:{min_dbm,max_dbm,per_antenna}, supports[], unsupported[]}`
- `Reader.GetConfig` → `{tx_power_dbm:[{antenna,power}], …}`
- `Reader.SetConfig(params=ReaderConfig, partial)` → `{applied: "immediate"|"pending_reload", effective_at?}`
- `Reader.GetStatus` → `{online, reading, active_profile?}`

**Capability-flagged management** (reserved; declared `unsupported` by the CS463 adapter): `Scan.Start`, `Scan.Stop`, `Gpo.Set`, `Reader.Reboot`.

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
| MQTT Server (CloudServer) | `TrakRF mqtt-rpc MQTT Server` | **No — pre-create out-of-band**, referenced by name |
| Operation Profile | `TrakRF mqtt-rpc Profile` | **Verify-exists** (fail if absent); antenna/TX-power stay with `SetConfig` |
| Data Format | `TrakRF mqtt-rpc Data Format` | Yes — trimmed JSON, numeric RSSI (`RSSI_Number`, parser PR #502) |
| Trigger | `TrakRF mqtt-rpc Trigger` | Yes — `Read Any Tags`, all antennas |
| Resultant Action | `TrakRF mqtt-rpc Action` | Yes — MQTT → server + format |
| Event | `TrakRF mqtt-rpc Event` | Yes — dedup=500ms, antennaDifferentiation=on, enable=on |

Reads/verify use `/API list*`; writes use `/API add*`/`mod*`. The write transport is
pluggable per entity (the `entitySpec` seam) so any entity can flip to the servlet
path if a firmware proves an `/API` write unreliable — **no firmware floor is
required**. Reconcile re-arms the golden event **only when something changed**, so a
no-op reconcile (routine restart) never interrupts inventory.

### Commissioning prerequisites (done in the same SSH session that installs the daemon)
1. Pre-create the `TrakRF mqtt-rpc MQTT Server` CloudServer entry (broker host/port/
   TLS/creds + the platform `scan_device.publish_topic`). The daemon reads it for its
   own broker connection and references it by name.
2. Pre-create the `TrakRF mqtt-rpc Profile` operation profile (antenna enablement + TX
   power; the daemon only verifies it exists and leaves those to `Reader.SetConfig`).
3. Existing readers provisioned under the old `TrakRF MQTT` name must set
   `READERD_CLOUDSERVER_ID` until migrated (the default is now the golden name).

### Config
- `READERD_RECONCILE` — `true` (default) runs reconcile on startup; `false` pins config.
- `READERD_CLOUDSERVER_ID` — defaults to `TrakRF mqtt-rpc MQTT Server`.

### Bench-verification checklist (cs463-212 — the remaining merge gate)
The Go unit tests cover parsing, drift detection, the reconcile decision table, the
single-session re-arm gating, and the dwell-clobber fix against fakes. The following
need real hardware (the fakes can't cover firmware behavior):
- [ ] **Per-entity write transport:** confirm `/API add*`/`mod*` works on a *clean*
      reader for Data Format, Trigger, Resultant Action, Event (only `modEvent` /
      `addDataFormat` were exercised in TRA-994); flip any flaky entity to servlet.
- [ ] **Tighten read-side diffs against captured `list*`:** verify `actionDrift`
      (action_mode/transport casing) and `dataFormatDrift` (field ordering) against a
      real `listResultantAction` / `listDataFormat` from the reader.
- [ ] **Reconcile lifecycle:** clean-reader create (all four add); converged-reader
      no-op (zero writes, no re-arm); drift correction (targeted `mod*` + re-arm).
- [ ] **End-to-end:** reads flow on the golden chain after a fresh commission; dwell
      stays at golden after a `SetConfig` TX-power change (no 2000 regression).
- [ ] **Partial-failure policy:** current behavior aborts on first entity error
      (logged, non-fatal to RPC). Revisit if bench shows a need for best-effort-continue.
