# TRA-901 — Geofence rules engine: membership filter + immediate-on-entry + RSSI gate + dedup

**Status:** Design approved (autonomous run, 2026-06-04)
**Ticket:** [TRA-901](https://linear.app/trakrf/issue/TRA-901) — parent TRA-897; blocked by TRA-900 (done) + TRA-899 (done); blocks TRA-904 (demo runbook); related TRA-903 (alarm device / Shelly)
**Branch:** `feat/tra-901-geofence-rules-engine`

## Problem

TRA-900 landed the in-backend MQTT subscriber: raw reads → `tag_scans` audit, then per-read
derivation into `asset_scans` under org context (`PersistReads`). That records *presence* but takes
**no action**. The fixed-reader demo needs the reader to **alarm in real time** when a registered
asset crosses a boundary — a registered tag read at a boundary capture point should immediately fire
the portal alarm, while a passerby's unregistered tagged goods and ambient bleed from a nearby
stationary tag must stay silent.

The clean seam is already in place: `subscriber.handleMessage` calls `PersistReads` and the comment at
`subscriber.go:130` marks `reads` as the geofence hand-off point. What's missing is the engine: the
membership gate, the RSSI trip line, the per-tag dedup latch, the alarm fire action, and the persisted
alarm event for history/reporting.

## Goal

A real-time geofence engine that, for every parsed read derived under org context, decides whether to
fire a boundary alarm and — when it fires — invokes a pluggable alarm action and writes an
`alarm_events` row. It runs in-process on the subscriber's per-message path (no new transport), reuses
the existing membership resolution, and is observable (structured logs + Prometheus counters) like the
rest of the ingest path.

Alarm decision = **registered asset** (membership) **× boundary capture point** (`scan_points.is_boundary`)
**× RSSI ≥ threshold** (trip line) **× not currently latched** (dedup).

## Scope (this ticket)

- **Membership filter — implemented as the *existing* live tag-resolution, not a separate cached set.**
  A read is "armed" iff `PersistReads` resolves it to a live `rfid` tag on an asset (and to a live
  scan_point). That join *is* the armed-EPC set, evaluated per read against the source of truth, so
  asset/tag CRUD is immediately effective with **no cache to refresh and no CRUD-notification plumbing**
  (none exists in the codebase today). See "Deviation from ticket wording" below.
- **Boundary gate:** only `scan_points.is_boundary = true` capture points can alarm. Non-boundary reads
  are recorded as `asset_scans` (unchanged) and never alarm.
- **RSSI gate (trip line):** fire only when the read's RSSI ≥ a configurable threshold. Default global
  threshold via env, with an optional per-scan-point override (`scan_points.metadata.rssi_threshold`)
  so a tight portal and a wide dock door can be tuned independently without a schema change. A read
  with no usable RSSI (the parser's `0` sentinel) does **not** fire (conservative — `0 dBm` is
  physically implausible for RFID and signals missing data).
- **Dedup latch:** in-memory latch keyed `(org_id, scan_point_id, epc)`; first qualifying read fires,
  subsequent reads while present are suppressed; the entry ages out on absence and a later re-entry
  fires again. Built on the **existing `ratelimit/limiter.go` aging pattern** (`sync.Map` +
  `atomic.Int64` lastSeen + background ticker sweep + injectable `Clock`). Admit decision is race-safe
  under paho's concurrent handler goroutines (`LoadOrStore` for first-sight, atomic `Swap` for
  expiry-vs-refresh).
- **Alarm fire action (seam):** a `geofence.Firer` interface, `Fire(ctx, AlarmEvent) error`. v1 ships a
  log-only implementation (`LogFirer`) that records the fire + increments a counter. TRA-903 (alarm
  device CRUD + Shelly Gen4) plugs the real device behind this interface — no engine change required.
- **Alarm event row:** new `trakrf.alarm_events` table (migration `000013`), RLS-scoped by `org_id` like
  `asset_scans`. Written on every fire (best-effort: a write failure logs + metrics but never blocks
  ingestion). This is the history/reporting surface.
- **Wiring:** engine constructed in `serve.Run` alongside the subscriber (gated on the same `MQTT_URL`
  enablement), injected into the subscriber, started/stopped with it; sweeper goroutine owned by the
  engine.
- **Observability:** `geofence_evaluated_total`, `geofence_alarms_fired_total`,
  `geofence_alarms_suppressed_total{reason}` (reasons: `not_boundary`, `rssi_below_threshold`,
  `no_rssi`, `latched`), `geofence_fire_errors_total`, `geofence_event_write_errors_total`; structured
  per-fire log line.

### Deviation from ticket wording (deliberate)

The ticket specifies "an in-memory armed-EPC set sourced from registered identifiers, refreshed on
asset/tag CRUD." We implement the same *behavior* — unregistered tags never alarm — by gating on the
live `PersistReads` tag-resolution instead of maintaining a parallel cached set. Rationale: (1) there
is no CRUD-notification/event-bus in the codebase, so a refreshed cache would require net-new plumbing;
(2) the live join is strictly more correct (no staleness window) and immediately reflects asset/tag
CRUD; (3) it reuses code that already exists and is RLS-correct. This matches the TRA-900 spec's note
that TRA-900 "adopts TRA-901's membership-filter intent" at the ingestion join. If a hot-path
performance problem ever appears (it won't at demo scale — single replica, low read volume), a cache
can be layered behind the same engine boundary later.

## Out of scope

- **Exit / absence-based alarms** (ticket v2). The latch's age-out is used only for dedup re-arming,
  not to emit an "exited" event.
- **Authorization / checked-out state** for legitimate transport (ticket v2).
- **The physical alarm device + Shelly Gen4 trigger** (TRA-903). We ship the `Firer` seam + log impl only.
- **Alarm device CRUD / alarm config tables** (TRA-903).
- **Multi-replica dedup.** The latch is per-process; under multi-replica GKE each replica would fire
  independently. The demo runs single-replica (consistent with TRA-900's multi-replica caveat and the
  TRA-907 deferral). Documented, not solved here.
- **Reporting API / UI for alarm_events.** The table + RLS land here; surfacing it is a follow-on.

## Architecture

New package `backend/internal/geofence`. The subscriber hands every message's membership-resolved reads
to the engine synchronously on the existing per-message path (after `PersistReads` commits). No new
transport, no queue.

```
subscriber.handleMessage(topic, payload)            [TRA-900, unchanged shape]
  1. InsertRawTagScan  ──▶ tag_scans
  2. ResolveScanTopic  ──▶ (org_id, device_type)        [SECURITY DEFINER]
  3. Parse             ──▶ []scanread.Read
  4. PersistReads(org_id, tag_scan_id, received_at, reads)
        WithOrgTx(org_id): per read resolve scan_point (+is_boundary) & rfid-tag→asset,
                           INSERT asset_scans ON CONFLICT DO NOTHING
        └─▶ returns PersistResult{ Inserted, Dropped, Resolved []ResolvedRead }   ← ENRICHED
  5. geofence.Evaluate(ctx, org_id, tag_scan_id, received_at, res.Resolved)        ← NEW
        per resolved read:
          is_boundary? ─no─▶ suppress(not_boundary)
          rssi usable & ≥ threshold? ─no─▶ suppress(rssi_below_threshold | no_rssi)
          latch admit (first sight or aged-out)? ─no─▶ suppress(latched)
          └─▶ FIRE: write alarm_events row (WithOrgTx, best-effort) + Firer.Fire(...)  (best-effort)
```

`Resolved` carries only reads that passed membership (registered asset at a registered scan_point),
**including reads whose `asset_scans` insert was a within-message dedup conflict** — presence at the
boundary is the geofence signal regardless of scan-row dedup. RSSI/boundary live on these reads.

### Why evaluate after PersistReads (not inside its tx)

Alarm side effects (event write, physical fire) are best-effort and must never roll back or block the
authoritative scan record. `asset_scans` is the source of truth; an alarm is a derived reaction. Keeping
`Evaluate` outside the `PersistReads` transaction means a slow/failed firer or event write degrades to a
logged + metriced miss, not a lost scan. The membership/boundary/location data needed by the engine is
returned cheaply from the resolution `PersistReads` already does (no extra queries).

### Components

| Unit | File | Responsibility | Tested by |
|---|---|---|---|
| Resolved read type | `internal/storage/ingest.go` | `ResolvedRead{AssetID, ScanPointID, LocationID, IsBoundary, EPC, RSSI}`; `PersistResult.Resolved` | integration |
| Enriched PersistReads | `internal/storage/ingest.go` | add `is_boundary` to scan_point select; append `Resolved` on membership pass | integration (RLS harness) |
| Engine | `internal/geofence/engine.go` | `Evaluate` orchestration; gate sequence; fire + event write | unit (fakes) + integration |
| Latch | `internal/geofence/latch.go` | `sync.Map` + atomic lastSeen + ticker sweep; race-safe `admit(key) bool` | unit (fake clock) |
| Config | `internal/geofence/config.go` | `GEOFENCE_RSSI_THRESHOLD`, `GEOFENCE_LATCH_TTL`, `GEOFENCE_SWEEP_INTERVAL`; defaults | unit |
| Firer | `internal/geofence/firer.go` | `Firer` interface + `LogFirer` impl | unit |
| Metrics | `internal/geofence/metrics.go` | Prometheus counters | — |
| Event storage | `internal/storage/alarm_events.go` | `InsertAlarmEvent(ctx, org_id, AlarmEvent)` under `WithOrgTx` | integration |
| Migration | `migrations/000013_alarm_events.{up,down}.sql` | create `alarm_events` + RLS + indexes; down drops | integration (build/run) |
| Wiring | `internal/cmd/serve/serve.go`, `internal/ingest/subscriber.go` | construct engine, inject into subscriber, start/stop sweeper | manual / build |

### Data types

```go
// storage: produced by PersistReads, consumed by the geofence engine.
type ResolvedRead struct {
    AssetID     int
    ScanPointID int
    LocationID  *int
    IsBoundary  bool
    EPC         string
    RSSI        int // scanread.Read.RSSI; 0 == parser sentinel for "no usable RSSI"
}

// geofence: one fired alarm, persisted to alarm_events and handed to the Firer.
type AlarmEvent struct {
    OrgID       int
    AssetID     int
    ScanPointID int
    LocationID  *int
    EPC         string
    RSSI        int
    TagScanID   int64
    FiredAt     time.Time
}

type Firer interface {
    Fire(ctx context.Context, ev AlarmEvent) error
}
```

The subscriber depends on a tiny consumer interface (defined in `ingest`, structurally satisfied by
`*geofence.Engine`) so there is no `ingest ↔ geofence` import cycle:

```go
// ingest
type ReadEvaluator interface {
    Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead)
}
```

`Evaluate` returns nothing — it is fire-and-log; the subscriber's summary log already covers the message.

### Migration `000013_alarm_events`

```sql
-- up
CREATE TABLE trakrf.alarm_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- event log id, not Feistel (matches tag_scans precedent)
    org_id        bigint      NOT NULL REFERENCES trakrf.organizations(id),
    asset_id      bigint      NOT NULL REFERENCES trakrf.assets(id),
    scan_point_id bigint      NOT NULL REFERENCES trakrf.scan_points(id),
    location_id   bigint               REFERENCES trakrf.locations(id),
    epc           text        NOT NULL,
    rssi          int,
    tag_scan_id   bigint,                                           -- provenance; no FK (tag_scans is a hypertable)
    fired_at      timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at    timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_alarm_events_org_time   ON trakrf.alarm_events (org_id, fired_at DESC);
CREATE INDEX idx_alarm_events_asset_time ON trakrf.alarm_events (asset_id, fired_at DESC);
ALTER TABLE trakrf.alarm_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY alarm_events_org_isolation ON trakrf.alarm_events
    USING (org_id = current_setting('app.current_org_id')::bigint);
-- GRANTs to the app role per the init-grants convention (TRA + infra #118)
```

Regular table, not a hypertable: alarm volume is tiny (deduped, one fire per entry/exit cycle). If it
ever grows, converting to a hypertable is a later, isolated change. `down` drops the table (and policy/
indexes cascade).

### Configuration

| Env | Default | Meaning |
|---|---|---|
| `GEOFENCE_RSSI_THRESHOLD` | `-65` | Global trip line in dBm; fire when `RSSI >= threshold`. Per-point override via `scan_points.metadata.rssi_threshold`. |
| `GEOFENCE_LATCH_TTL` | `60s` | Absence window; a tag silent this long re-arms and can fire again. |
| `GEOFENCE_SWEEP_INTERVAL` | `5m` | Latch GC cadence (memory hygiene; expiry is also enforced on access). |

The engine is constructed whenever the subscriber is (i.e. `MQTT_URL` set). With ingestion off, no
engine, no goroutine.

## Validation criteria

- [ ] A registered asset's rfid tag read at a **boundary** scan_point with RSSI ≥ threshold fires
      exactly one alarm: one `alarm_events` row + one `Firer.Fire` call.
- [ ] An **unregistered** EPC at the same boundary fires nothing (no `asset_scans`, no alarm) — membership.
- [ ] A registered asset at a **non-boundary** scan_point records `asset_scans` but fires no alarm.
- [ ] A read with RSSI **below** threshold (and one with the `0` sentinel) records `asset_scans` but
      fires no alarm.
- [ ] Repeated qualifying reads of the same `(org, boundary, epc)` within the TTL fire **once**
      (subsequent suppressed `latched`); after the TTL elapses with no reads, the next read fires again.
- [ ] Concurrent qualifying reads of the same key fire exactly once (race-safe admit).
- [ ] `alarm_events` is RLS-isolated: the row is only visible under its org's context (verified on the
      non-superuser RLS harness).
- [ ] A failing `Firer` or event-write logs + metrics the error and does **not** break the message path
      or the `asset_scans` write.
- [ ] `just backend lint` / `vet` / `test` green; integration suite green on the RLS harness.

## Testing

- **Latch (unit, fake clock):** first-sight admits; within-TTL repeat suppresses; post-TTL re-admits;
  sweep evicts idle keys; concurrent admit of one key yields a single `true` (run under `-race`).
- **RSSI gate / config (unit):** threshold comparison incl. negative dBm; `0` sentinel suppressed;
  per-point `metadata.rssi_threshold` override beats the global default.
- **Engine (unit, fake Firer + fake clock):** full gate sequence drives fire vs each suppress reason;
  fire builds the correct `AlarmEvent`; Firer error is swallowed (logged) and event write still attempted.
- **PersistReads enrichment (integration, RLS harness):** `Resolved` contains exactly the
  membership-passing reads with correct `IsBoundary`/`LocationID`; within-message duplicate still
  appears in `Resolved` though its `asset_scans` insert is a `conflict`.
- **alarm_events storage (integration, RLS harness):** `InsertAlarmEvent` writes under org context and
  the row is RLS-visible only to that org; missing org GUC throws (proves no superuser bypass).
- **Migration (integration):** `000013` up creates the table/policy/indexes; down drops cleanly; RLS
  policy present.
- **Build/vet/lint** green; `just backend test` for the unit layer, integration tag for storage.

## Risks / decisions

- **Membership via live join, not cached set (deviation):** documented above; behavior-equivalent,
  simpler, no staleness. Re-flagging this as "missing the in-memory set" would be wrong.
- **RSSI `0` sentinel suppressed:** a genuine (astronomically unlikely) `0 dBm` read would be silenced;
  acceptable. The real fix (distinguishing "missing" from `0`) would touch the TRA-900 `scanread.Read`
  shape and is deferred unless a device emits legitimate `0`.
- **Best-effort alarm side effects:** an alarm can be missed on a transient DB/firer error; the scan is
  never lost. Correct priority for a presence-of-record system; the audit log (`tag_scans`) plus
  `asset_scans` allow reconstruction.
- **Per-process latch / multi-replica double-fire:** single-replica demo unaffected; shared-state dedup
  is a cutover concern (TRA-907 family). Documented.
- **Synchronous on the read path:** evaluation is in-memory + at most one small insert per *fire* (rare,
  post-dedup); negligible added latency at demo scale. If reader fan-out ever makes this hot, the engine
  boundary allows moving to an async channel without touching the subscriber contract.
