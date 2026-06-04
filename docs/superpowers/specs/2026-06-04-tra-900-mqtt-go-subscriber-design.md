# TRA-900 — MQTT ingestion: in-backend Go subscriber (replaces Redpanda Connect + `process_tag_scans` trigger)

**Status:** Design approved (autonomous run, 2026-06-04)
**Ticket:** [TRA-900](https://linear.app/trakrf/issue/TRA-900) — parent TRA-897; blocks TRA-901 (geofence engine), TRA-907 (RC decommission)
**Branch:** `feat/tra-900-mqtt-go-subscriber`

## Problem

Today an out-of-process Redpanda Connect (RC) job subscribes to the MQTT broker and does a raw
`INSERT INTO trakrf.tag_scans (message_topic, message_data)`. An `AFTER INSERT` trigger
(`process_tag_scans`) fans that raw row out into `assets` / `tags` / `asset_scans`.

Two live-verified defects make broker → `asset_scans` non-functional (preview) and latent-broken (prod):

1. **RLS-vs-unset-GUC landmine.** RC and the future Go subscriber both connect as `trakrf-app`
   (`NOSUPERUSER NOBYPASSRLS`). The trigger's fan-out `INSERT`s hit RLS policies of the form
   `org_id = current_setting('app.current_org_id')::bigint`. `current_setting` has no `missing_ok`
   arg, so an **unset** GUC **throws** (`unrecognized configuration parameter`) instead of filtering.
   The trigger's `EXCEPTION WHEN OTHERS → RAISE WARNING; RETURN NEW` then **silently rolls back** the
   derivation. Symptom: `tag_scans` hot, `asset_scans` never moves. Same class as TRA-865.
2. **Wrong org routing.** The trigger resolves org via `organizations.identifier =
   split_part(message_topic,'/',1)`. Live readers publish to `trakrf.id/{device}/reads`, and no org
   has `identifier = 'trakrf.id'`, so 100% of live reads drop at org resolution.

The trigger is also unobservable (errors swallowed), untestable, and couples raw landing to derivation.

## Goal

Replace **both halves** (RC subscribe + PG trigger derive) with one in-backend Go MQTT subscriber that
parses reads in Go, resolves the registry, and writes `asset_scans` directly with org context set
per-write (the API's `WithOrgTx` model). `tag_scans` becomes an **append-only audit log**, no longer a
pipeline stage. The `process_tag_scans` trigger + function are dropped.

This is the prerequisite for enabling prod broker → `asset_scans`, and the shared in-Go parser
TRA-901's geofence engine will reuse.

## Scope (this ticket)

- CS463 (`csl_cs463`) parser only. GL-S10 / ESP32 / CS108 dispatch stubs log "unsupported" and defer
  to their own tickets.
- Org/registry resolution re-keyed onto `scan_devices.publish_topic` (TRA-899), not `organizations.identifier`.
- **Asset resolution is tag-based with NO auto-create** (decided 2026-06-04): a read produces an
  `asset_scan` only if its EPC already has a live `rfid` tag. Unregistered EPCs produce nothing and
  nothing is auto-created. (This adopts TRA-901's membership-filter intent; TRA-901 still owns the
  in-memory armed-EPC set, dedup, RSSI gate, and immediate-on-entry alarm.)
- `asset_scans.timestamp` = **server receive time** (reader `timeStampOfRead` / `timeZone` are not
  authoritative).
- Drop the `process_tag_scans` trigger + function. Add a tiny `SECURITY DEFINER` topic→org resolver.
- Observable: structured logs + Prometheus counters for every message and every dropped read (by reason).

## Out of scope

- The geofence rules engine, armed-EPC cache, dedup, RSSI gate, immediate-on-entry (TRA-901). We leave
  a clean seam (parsed `Read` type + a place to hand reads off) but build no engine.
- GL-S10 / ESP32 / CS108 parsers (their own tickets).
- Multi-tenant topic ACL guardrails (TRA-857). `publish_topic` is unique *per org*; consumer-decides
  fan-out is accepted for the demo.
- Cloud cutover + `trakrf-ingester` decommission (TRA-907).

## Architecture

New package `backend/internal/ingest`, started as a goroutine from `serve.Run`, gated on `MQTT_URL`
being set (empty/unset ⇒ subscriber does not start — keeps local dev, tests, and pre-cutover prod inert).

```
MQTT broker ──(mqtts, QoS1)──▶ subscriber.go
                                  │  per message (topic, payload):
                                  ▼
                       1. InsertRawTagScan(topic, payload) ──▶ tag_scans (audit; no RLS, no org ctx)
                                  │  returns tag_scan_id, received_at
                                  ▼
                       2. ResolveScanTopic(topic) ──▶ (org_id, device_id, device_type)   [SECURITY DEFINER]
                                  │  no row ⇒ log "unregistered topic", stop (audit kept)
                                  ▼
                       3. parser.Parse(device_type, payload) ──▶ []Read     [pure, unit-tested]
                                  │  unsupported type ⇒ log, stop
                                  ▼
                       4. store.PersistReads(org_id, tag_scan_id, received_at, reads)
                                  │  WithOrgTx(org_id):                       [RLS-correct, integration-tested]
                                  │    per read: resolve scan_point by external_key,
                                  │              resolve asset by live rfid tag (value=epc),
                                  │              INSERT asset_scans ... ON CONFLICT DO NOTHING
                                  ▼
                       (TRA-901 seam: reads also available to hand to the geofence engine)
```

### Why a `SECURITY DEFINER` resolver

`scan_devices` is RLS-protected, so the subscriber can't read `publish_topic → org_id` without already
knowing the org (chicken-and-egg). `organizations` has no RLS, which is the only reason the old trigger's
org lookup worked. We add one minimal, read-only, single-purpose function owned by the schema owner:

```sql
CREATE FUNCTION trakrf.resolve_scan_topic(p_topic text)
RETURNS TABLE (org_id bigint, scan_device_id bigint, device_type trakrf.scan_device_type)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = trakrf, public AS $$
    SELECT d.org_id, d.id, d.type
    FROM trakrf.scan_devices d
    WHERE d.deleted_at IS NULL
      AND ( d.publish_topic = p_topic
            OR (d.publish_topic IS NULL
                AND p_topic = 'trakrf.id/' || d.external_key || '/reads') )
    LIMIT 1;
$$;
```

This is the "thin SQL helper" the decision comment explicitly allowed (a resolver, not a parser). It
honors the documented default `publish_topic = trakrf.id/{external_key}/reads`. `LIMIT 1` accepts the
per-org-unique-topic fan-out nuance for the demo (TRA-857 owns hardening). Granted `EXECUTE` to the app role.

### Components

| Unit | File | Responsibility | Tested by |
|---|---|---|---|
| Config | `internal/ingest/config.go` | Read `MQTT_URL`, `MQTT_TOPIC`, `MQTT_CLIENT_ID`; `Enabled()` | unit |
| Parser | `internal/ingest/parser.go`, `parser_cs463.go` | `Parse(deviceType, payload) ([]Read, error)`; dispatch by device type; pure | unit (fixtures) |
| Subscriber | `internal/ingest/subscriber.go` | MQTT client lifecycle, reconnect, per-message orchestration, panic recovery | unit (orchestration via fakes) |
| Metrics | `internal/ingest/metrics.go` | Prometheus counters | — |
| Storage | `internal/storage/ingest.go` | `ResolveScanTopic`, `InsertRawTagScan`, `PersistReads` | integration (RLS harness) |
| Migration | `migrations/000012_drop_tag_scan_trigger.{up,down}.sql` | Drop trigger+fn; add resolver | integration (build/run) |
| Wiring | `internal/cmd/serve/serve.go` | Start goroutine, graceful stop on `ctx.Done()` | manual / build |

### Data types

```go
// Read is one parsed tag observation, device-agnostic. Shared with TRA-901.
type Read struct {
    EPC              string    // tags[].epc
    CapturePointName string    // tags[].capturePointName  → scan_points.external_key
    AntennaPort      int       // tags[].antennaPort
    RSSI             int       // tags[].rssi (CS463 sends it as a string; parser coerces)
    ReaderTimestamp  time.Time // tags[].timeStampOfRead (microseconds); informational only
}
```

`PersistReads` returns a small result (`{Inserted int, Dropped map[string]int}`) for logging/metrics;
drop reasons: `no_scan_point`, `no_asset` (unregistered EPC), `conflict`.

### Per-write derivation (inside `WithOrgTx(org_id)`)

For each parsed `Read`:
1. `SELECT id, location_id FROM trakrf.scan_points WHERE org_id=$1 AND external_key=$2 AND deleted_at IS NULL` — miss ⇒ drop (`no_scan_point`).
2. `SELECT asset_id FROM trakrf.tags WHERE org_id=$1 AND type='rfid' AND value=$2 AND asset_id IS NOT NULL AND deleted_at IS NULL` — miss ⇒ drop (`no_asset`). **No auto-create.**
3. `INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id) VALUES (received_at, $1, asset_id, location_id, scan_point_id, tag_scan_id) ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING`.

`received_at` is one server timestamp per message, so duplicate EPCs *within* a message dedup on the
content PK, while reads across messages are preserved (cross-message dedup is TRA-901's concern).
`tag_scan_id` now links each derived row to its source audit row (the old trigger left it `NULL`).

### Observability (replaces the silent `EXCEPTION WHEN OTHERS`)

- Per-message panic recovery: a single malformed payload logs an error and is skipped; ingestion never dies.
- Structured `zerolog` lines for: connect/disconnect/resubscribe, unregistered topic, unsupported device
  type, parse error, and a per-message summary `{topic, parsed, inserted, dropped_by_reason}`.
- Prometheus counters: `ingest_messages_total{result}`, `ingest_reads_parsed_total`,
  `ingest_asset_scans_inserted_total`, `ingest_reads_dropped_total{reason}`.

### MQTT client

`github.com/eclipse/paho.mqtt.golang` (mature, TLS/`mqtts`, username+password from URL, auto-reconnect,
QoS). Config:
- `MQTT_URL` — e.g. `mqtts://user:pass@mqtt.preview.gke.trakrf.id:8883`. Unset ⇒ subscriber off.
- `MQTT_TOPIC` — default `trakrf.id/#`. Set to `$share/trakrf-ingest/trakrf.id/#` under multi-replica GKE.
- `MQTT_CLIENT_ID` — base id; the subscriber appends a per-process suffix (hostname) so replicas don't
  collide and trigger duplicate-client-id disconnect loops.
- QoS 1, `SetAutoReconnect(true)`, `OnConnect` resubscribes (survives broker restarts).

**Multi-replica caveat (documented, not solved here):** without an MQTT shared subscription every replica
receives every message and writes duplicate `asset_scans` (different `received_at` ⇒ different PK, so
`ON CONFLICT` does not save us). The demo runs single-replica; `$share/...` via `MQTT_TOPIC` is the
forward path and is called out for the GKE cutover (TRA-907).

## Migration `000012_drop_tag_scan_trigger`

- **up:** `DROP TRIGGER trigger_process_tag_scans ON trakrf.tag_scans; DROP FUNCTION trakrf.process_tag_scans();`
  then `CREATE FUNCTION trakrf.resolve_scan_topic(...)` + `GRANT EXECUTE` to the app role.
- **down:** `DROP FUNCTION resolve_scan_topic;` recreate `process_tag_scans` (the 000011 up form) +
  recreate the trigger. `tag_scans` table itself is unchanged (still the audit log).

## Testing

- **Parser (unit):** table-driven over the committed real-capture fixtures (`cs463_read.json`, plus the
  live multi-tag and `rssi`-as-string shapes captured 2026-06-04). Asserts EPC/capturePointName/antenna/
  rssi/timestamp extraction and that an unsupported device type returns a typed "unsupported" signal.
- **Storage (integration, `//go:build integration`, TRA-874 RLS harness):**
  - `ResolveScanTopic` finds a device by explicit `publish_topic` and by the `external_key` default;
    returns no row for an unknown topic; works without any org GUC set (proves `SECURITY DEFINER`).
  - `PersistReads`: registered device + registered scan_point + registered rfid-tagged asset ⇒ exactly
    one `asset_scan` with the right `location_id`/`scan_point_id`/`tag_scan_id`; **unregistered EPC ⇒ zero
    rows** (membership filter); unknown scan_point ⇒ zero; duplicate EPC in one message ⇒ one row.
  - Runs on the non-superuser RLS role — proves the GUC-unset landmine is gone (this would have thrown
    pre-fix).
  - A regression test asserting the `process_tag_scans` trigger no longer exists after migrate.
- **Build/vet/lint** green; `just backend test` for the unit layer.

## Risks / decisions

- **Tag-based, no auto-create (user decision):** demo assets must be pre-registered with `rfid` tags or
  live reads record nothing. Accepted; matches the membership-filter direction.
- **Server-time PK:** no cross-message dedup; acceptable (TRA-901 owns dedup). Within-message dedup intact.
- **`SECURITY DEFINER` surface:** read-only, returns only routing ids for a topic; minimal leakage, and
  it *is* the routing key by design.
- **Multi-replica duplicates:** documented; single-replica demo unaffected; `$share` is the fix at cutover.
```
