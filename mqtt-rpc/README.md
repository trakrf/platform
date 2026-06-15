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
