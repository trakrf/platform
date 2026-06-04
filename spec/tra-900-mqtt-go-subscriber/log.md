# TRA-900 Build Log

Branch: `feat/tra-900-mqtt-go-subscriber`
Built autonomously via superpowers spec → plan → build → PR, 2026-06-04.

## What shipped

Replaced the Redpanda Connect ingester **and** the `process_tag_scans` PG trigger with an
in-backend Go MQTT subscriber. `tag_scans` is now an append-only audit log; `asset_scans` are
derived in Go with per-write org context (RLS-correct).

### Database — migration `000012` (up + down), on top of the frozen `000001–000011` stack
- **Dropped** `trigger_process_tag_scans` + `process_tag_scans()`. The trigger was the source of the
  silent RLS-GUC swallow (its `EXCEPTION WHEN OTHERS → RETURN NEW` masked every fan-out failure),
  coupled raw landing to derivation, and was unobservable/untestable.
- **Added** `resolve_scan_topic(text) → (org_id, scan_device_id, device_type)`, a read-only
  `SECURITY DEFINER` resolver. `scan_devices` is RLS-protected, so the subscriber (role `trakrf-app`,
  no BYPASSRLS) cannot read `publish_topic → org_id` without already knowing the org — chicken-and-egg.
  The definer resolves the routing key cross-org (it *is* cross-org by design), honoring the documented
  `publish_topic = trakrf.id/{external_key}/reads` default. `GRANT EXECUTE … TO PUBLIC` (role-name-agnostic).
- down: drop the resolver, recreate the 000011 trigger + function verbatim.

### Backend
- `internal/models/scanread` — dependency-free `Read` type shared by the parser and (future) TRA-901
  geofence engine. Lives in its own leaf package to break an `ingest ↔ storage` import cycle.
- `internal/ingest` — `Parse(deviceType, payload)` device-dispatch (CS463 implemented; GL-S10 / ESP32 /
  CS108 return `ErrUnsupportedDevice`), `Config`/`ConfigFromEnv`, Prometheus `metrics`, and the
  `Subscriber` (paho MQTT client: TLS/`mqtts`, auto-reconnect, QoS 1, per-message panic recovery,
  observable per-outcome logging + metrics).
- `internal/storage/ingest.go` — `ResolveScanTopic`, `InsertRawTagScan` (audit log, no org ctx),
  `PersistReads` (the RLS-correct derivation inside `WithOrgTx`).
- `cmd/serve/serve.go` — starts/stops the subscriber goroutine; **disabled when `MQTT_URL` is unset**,
  so local dev / tests / pre-cutover prod stay inert.

### Key behavioral decisions
- **Asset resolution is tag-based with NO auto-create** (Mike's call this session): a read records an
  `asset_scan` only if its EPC already has a live `rfid` tag (`tags.value = epc → asset_id`). Unregistered
  EPCs record nothing; nothing is auto-created. This adopts TRA-901's membership-filter intent; TRA-901
  still owns the armed-EPC cache, dedup, RSSI gate, and immediate-on-entry alarm.
- **Server time wins** — `asset_scans.timestamp` is server receive time; reader `timeStampOfRead`/`timeZone`
  are parsed but informational. Within-message duplicate EPCs dedup on the content PK; cross-message dedup
  is TRA-901's concern.
- `tag_scan_id` now links each derived `asset_scan` to its source audit row (the old trigger left it NULL).
- **Org routing re-keyed** onto `scan_devices.publish_topic` (TRA-899), not `organizations.identifier`
  (the old `split_part(topic,'/',1)` matched no org for the live `trakrf.id/...` topics → 100% drop).

## Fixtures
Reused TRA-899's real broker captures (`cs463_read.json`, `gls10_read.json`) and added
`cs463_read_multi.json` (live 2026-06-04 two-tag capture from `mqtt.preview.gke.trakrf.id`). The parser
unit tests assert against the real shapes (number µs `timeStampOfRead`, string `rssi`, multi-tag arrays).

## Notable findings
- **Import cycle** (`ingest.Subscriber` → `storage`, `storage.PersistReads` → `ingest.Read`) — broken by
  moving the shared `Read` type into the neutral `internal/models/scanread` leaf package.
- **RLS chicken-and-egg** — only `organizations` lacks RLS (why the old trigger's org lookup worked);
  `scan_devices` has it, forcing the `SECURITY DEFINER` resolver rather than a plain query.
- **Multi-replica caveat (documented, not solved here):** without an MQTT shared subscription every
  replica receives every message and writes duplicate `asset_scans` (distinct `received_at` ⇒ distinct PK,
  so `ON CONFLICT` doesn't save us). Single-replica demo is unaffected; `$share/trakrf-ingest/...` via
  `MQTT_TOPIC` is the forward path, called out for the GKE cutover (TRA-907).
- The obsolete `process_tag_scans_integration_test.go` (tested the dropped trigger) was replaced by
  `ingest_integration_test.go` (tests `PersistReads` + the resolver on the RLS-enforced role).

## Validation gates
- `just backend lint` — check-rls-guard clean, fmt, vet ✓
- `go test ./...` (unit, whole backend) ✓ — incl. new `internal/ingest` parser + config suites
- `just backend test-integration` (`-p 1`) — full suite green; the 8 new `ingest` integration tests run
  on the non-superuser RLS role, which is what proves the GUC-unset landmine is gone (these would have
  thrown pre-fix). Migration `000012` applies + reverses cleanly (harness runs all 12 migrations).
- Live preview broker smoke — deferred to post-deploy (PR auto-deploys to preview with `MQTT_URL` set by
  infra). Confirm `asset_scans` populate for a registered device whose EPCs have rfid tags, and that
  `ingest_*` counters advance on `/metrics`.

## Out of scope (seams left clean)
- Geofence rules engine / dedup / RSSI gate / immediate-on-entry — TRA-901 (consumes `scanread.Read`).
- GL-S10 / ESP32 / CS108 parsers — their own tickets.
- MQTT topic ACL multi-tenant hardening — TRA-857.
- RC decommission + cloud cutover ($share, single-writer) — TRA-907.
