# Implementation Plan: TRA-901 Geofence rules engine
Generated: 2026-06-04
Specification: spec.md

## Understanding

Add a real-time geofence engine on the existing TRA-900 per-message ingest path. After `PersistReads`
derives `asset_scans` for membership-passing reads, hand those resolved reads to a new
`geofence.Engine.Evaluate`, which fires a boundary alarm when a registered asset is read at a boundary
capture point above an RSSI trip line and is not already latched. Firing = write an `alarm_events` row
(best-effort, RLS-scoped) + invoke a pluggable `Firer` (log-only here; Shelly in TRA-903). Membership is
the existing live tag-resolution (no cached set). Dedup latch reuses the `ratelimit/limiter.go` aging
pattern. All additive — new package + new table; no behavior change to existing scans.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/ratelimit/limiter.go` (lines 57–222) — sync.Map + `atomic.Int64` lastSeen + ticker
  `sweepLoop`/`sweep` + injectable `Clock`; mirror for the latch. `Clock`/`RealClock` in
  `backend/internal/ratelimit/clock.go`.
- `backend/internal/ingest/subscriber.go` (lines 79–150) — per-message orchestration; the `Evaluate`
  call goes right after `PersistReads` (the seam comment at line 130).
- `backend/internal/ingest/metrics.go` — Prometheus `promauto` counter declaration style; mirror for geofence.
- `backend/internal/ingest/config.go` — env-var config with defaults + `Enabled()`; mirror for geofence config.
- `backend/internal/storage/ingest.go` (lines 52–119) — `PersistResult`/`PersistReads`; enrich here.
- `backend/internal/storage/transactions.go` (`WithOrgTx`) — RLS wrapper for the alarm_events insert.
- `backend/migrations/000008_scan_hypertables.up.sql` (lines 36–76) — table + RLS policy
  (`org_isolation_<table>`, USING-only) convention; **note: no in-migration GRANT** (init-grants handles it).
- `backend/migrations/000012_drop_tag_scan_trigger.{up,down}.sql` — migration header/comment style + down form.
- `backend/internal/storage/ingest_integration_test.go` — integration harness (`//go:build integration`,
  `storage_test`, `testutil.SetupTestDBFull`, `db.Store`/`db.AdminPool`, `registerDevice`/`registerRFIDTag`).
- `backend/internal/cmd/serve/serve.go` (subscriber construct/Start/defer Stop block) — wiring pattern.

**Files to Create**:
- `backend/migrations/000013_alarm_events.up.sql` / `.down.sql` — alarm_events table + RLS + indexes.
- `backend/internal/geofence/config.go` — `Config{RSSIThreshold, LatchTTL, SweepInterval}` + `ConfigFromEnv`.
- `backend/internal/geofence/latch.go` — race-safe dedup latch (admit + sweep + Clock).
- `backend/internal/geofence/latch_test.go` — unit (fake clock, `-race`).
- `backend/internal/geofence/firer.go` — `Firer` interface + `LogFirer`.
- `backend/internal/geofence/metrics.go` — Prometheus counters.
- `backend/internal/geofence/engine.go` — `Engine`, `NewEngine`, `Start`/`Stop`, `Evaluate`, gate sequence.
- `backend/internal/geofence/engine_test.go` — unit (fake Firer + fake clock; gate sequence; per-point override).
- `backend/internal/storage/alarm_events.go` — `AlarmEventRow` + `InsertAlarmEvent` under `WithOrgTx`.
- `backend/internal/storage/alarm_events_integration_test.go` — RLS-isolation + insert.

**Files to Modify**:
- `backend/internal/storage/ingest.go` — add `ResolvedRead`, `PersistResult.Resolved`; select
  `is_boundary`; append resolved read on membership pass (incl. conflict).
- `backend/internal/ingest/subscriber.go` — add `eval ReadEvaluator` field + `ReadEvaluator` interface;
  call `Evaluate` after `PersistReads`; thread through `NewSubscriber`.
- `backend/internal/cmd/serve/serve.go` — construct `geofence.Engine`, inject into subscriber, `Start`/`defer Stop`.
- `backend/internal/storage/ingest_integration_test.go` — assert `Resolved` contents (boundary flag, conflict still present).

## Architecture Impact
- **Subsystems affected**: storage (enrich + new table writer), ingest (call seam), new `geofence` package, migrations, serve wiring.
- **New dependencies**: none (paho, prometheus, testify already vendored by TRA-900).
- **Breaking changes**: none. Purely additive; existing scan derivation untouched.

## Task Breakdown

### Task 1: Migration `000013_alarm_events`
**Files**: `backend/migrations/000013_alarm_events.up.sql` / `.down.sql` — CREATE
**Pattern**: 000008 asset_scans (RLS policy `org_isolation_alarm_events`, USING-only, no in-migration GRANT).
**up**: `SET search_path = trakrf, public;` then table per spec (IDENTITY id PK; FKs to organizations/
assets/scan_points/locations; `tag_scan_id` no FK; `fired_at`/`created_at` default now); two indexes
(`idx_alarm_events_org_time`, `idx_alarm_events_asset_time`); `ENABLE ROW LEVEL SECURITY`; policy.
**down**: `DROP TABLE IF EXISTS trakrf.alarm_events;` (indexes/policy cascade).
**Validation**: `just backend build`; migration applies in integration setup (Task 9).

### Task 2: `storage.ResolvedRead` + enrich `PersistReads`
**File**: `backend/internal/storage/ingest.go` — MODIFY
**Implementation**:
```go
type ResolvedRead struct {
    AssetID     int
    ScanPointID int
    LocationID  *int
    IsBoundary  bool
    EPC         string
    RSSI        int
}
// PersistResult gains: Resolved []ResolvedRead
// scan_point select: add is_boundary
//   SELECT id, location_id, is_boundary FROM trakrf.scan_points WHERE ...
// after asset lookup succeeds (membership passed), BEFORE the insert/conflict branch:
//   res.Resolved = append(res.Resolved, ResolvedRead{AssetID: assetID, ScanPointID: scanPointID,
//       LocationID: locationID, IsBoundary: isBoundary, EPC: rd.EPC, RSSI: rd.RSSI})
```
Append must precede the `ON CONFLICT` check so within-message duplicates still appear in `Resolved`
(presence is the geofence signal regardless of scan-row dedup).
**Validation**: `just backend lint && just backend build`.

### Task 3: geofence Config
**File**: `backend/internal/geofence/config.go` — CREATE
**Pattern**: `ingest/config.go`.
```go
type Config struct {
    RSSIThreshold int           // dBm; fire when read RSSI >= threshold
    LatchTTL      time.Duration
    SweepInterval time.Duration
}
func DefaultConfig() Config { return Config{RSSIThreshold: -65, LatchTTL: 60*time.Second, SweepInterval: 5*time.Minute} }
func ConfigFromEnv() Config // GEOFENCE_RSSI_THRESHOLD (int), GEOFENCE_LATCH_TTL, GEOFENCE_SWEEP_INTERVAL (durations); fall back to defaults on unset/parse-fail
```
**Validation**: `config_test.go` unit (covered in engine_test or its own); lint/build.

### Task 4: Latch (TDD)
**Files**: `backend/internal/geofence/latch.go` + `latch_test.go` — CREATE
**Pattern**: `ratelimit/limiter.go` (sync.Map, atomic lastSeen, sweepLoop/sweep, Clock).
```go
type Clock interface{ Now() time.Time } // local mirror to avoid ratelimit coupling; RealClock{}
type latch struct {
    ttl   time.Duration
    clk   Clock
    seen  sync.Map // key string -> *entry{ lastSeen atomic.Int64 }
    stop, done chan struct{}; closeOnce sync.Once
}
// admit(key) bool: true == "fire" (first sight OR aged-out); false == "suppress (latched)".
//   v, loaded := seen.LoadOrStore(key, &entry{lastSeen now})
//   if !loaded { return true }                      // first sight
//   prev := e.lastSeen.Swap(now)
//   return now - prev > ttl.Nanoseconds()           // re-armed after absence
// background sweep deletes entries with lastSeen < now-ttl (memory hygiene)
func keyFor(orgID, scanPointID int, epc string) string
```
**Tests (write first, `-race`)**: first-sight admits; within-TTL repeat suppresses; post-TTL re-admits
(fake clock advance); sweep evicts idle; concurrent admit of one key → exactly one `true`.
**Validation**: `go test -race ./internal/geofence/...`; lint/build.

### Task 5: Firer + metrics
**Files**: `backend/internal/geofence/firer.go`, `metrics.go` — CREATE
```go
type Firer interface { Fire(ctx context.Context, ev AlarmEvent) error }
type LogFirer struct{ log zerolog.Logger }
func (f LogFirer) Fire(ctx, ev) error // structured log + geofence_alarms_fired counter handled in engine; firer logs device action (none yet)
```
Metrics (promauto, mirror ingest/metrics.go): `geofence_evaluated_total`, `geofence_alarms_fired_total`,
`geofence_alarms_suppressed_total{reason}`, `geofence_fire_errors_total`, `geofence_event_write_errors_total`.
**Validation**: lint/build.

### Task 6: `AlarmEvent` type + Engine (TDD)
**Files**: `backend/internal/geofence/engine.go` + `engine_test.go` — CREATE
```go
type AlarmEvent struct { OrgID, AssetID, ScanPointID int; LocationID *int; EPC string; RSSI int; TagScanID int64; FiredAt time.Time }

type alarmWriter interface { // satisfied by *storage.Storage
    InsertAlarmEvent(ctx context.Context, orgID int, ev storage.AlarmEventRow) error
}
type Engine struct { cfg Config; store alarmWriter; firer Firer; latch *latch; clk Clock; log zerolog.Logger }
func NewEngine(cfg Config, store *storage.Storage, firer Firer, log *zerolog.Logger) *Engine
func (e *Engine) Start() // start latch sweeper
func (e *Engine) Stop()
func (e *Engine) Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead) {
  for _, rd := range reads {
    evaluated++
    if !rd.IsBoundary { suppress(not_boundary); continue }
    thr := e.cfg.RSSIThreshold // + per-point override hook (see note)
    if rd.RSSI == 0 { suppress(no_rssi); continue }       // parser sentinel
    if rd.RSSI < thr { suppress(rssi_below_threshold); continue }
    if !e.latch.admit(keyFor(orgID, rd.ScanPointID, rd.EPC)) { suppress(latched); continue }
    ev := AlarmEvent{...FiredAt: receivedAt}
    if err := e.store.InsertAlarmEvent(ctx, orgID, toRow(ev)); err != nil { log; event_write_errors++ } // best-effort
    if err := e.firer.Fire(ctx, ev); err != nil { log; fire_errors++ }                                  // best-effort
    fired++
  }
}
```
Per-point RSSI override: spec calls for `scan_points.metadata.rssi_threshold`. Since `PersistReads`
already has the scan_points row, the cheapest path is to add an optional `RSSIThreshold *int` to
`ResolvedRead` (selected from `metadata->>'rssi_threshold'`); engine uses it when non-nil. **If selecting
metadata complicates Task 2, ship the global threshold only and leave a `// TODO TRA-901 per-point` note**
— global threshold satisfies all validation criteria. Decide during build; prefer the override if it's a
one-line `COALESCE`/JSON extract.
**Tests (fake Firer capturing fires + fake clock + fake alarmWriter)**: each gate path (fire / not_boundary
/ no_rssi / rssi_below / latched); `AlarmEvent` field correctness incl. `FiredAt==receivedAt`; Firer error
swallowed and event-write still attempted; (if implemented) per-point override beats global.
**Validation**: `go test -race ./internal/geofence/...`; lint/build.

### Task 7: `storage.InsertAlarmEvent`
**Files**: `backend/internal/storage/alarm_events.go` (+ integration test in Task 9) — CREATE
```go
type AlarmEventRow struct { AssetID, ScanPointID int; LocationID *int; EPC string; RSSI int; TagScanID int64; FiredAt time.Time }
func (s *Storage) InsertAlarmEvent(ctx, orgID int, ev AlarmEventRow) error {
  return s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
    _, err := tx.Exec(ctx, `INSERT INTO trakrf.alarm_events
      (org_id, asset_id, scan_point_id, location_id, epc, rssi, tag_scan_id, fired_at)
      VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, orgID, ev.AssetID, ev.ScanPointID, ev.LocationID, ev.EPC, ev.RSSI, ev.TagScanID, ev.FiredAt)
    return err
  })
}
```
**Validation**: lint/build; integration in Task 9.

### Task 8: Wire engine into subscriber + serve
**Files**: `subscriber.go`, `serve.go` — MODIFY
- `ingest`: add `type ReadEvaluator interface { Evaluate(ctx, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead) }`;
  `Subscriber` gains `eval ReadEvaluator`; `NewSubscriber(cfg, store, eval, log)`; after `PersistReads`
  success: `if s.eval != nil { s.eval.Evaluate(ctx, route.OrgID, tagScanID, receivedAt, res.Resolved) }`.
- `serve.go`: inside the `mqttCfg.Enabled()` block, `engine := geofence.NewEngine(geofence.ConfigFromEnv(), store, geofence.LogFirer{...}, log); engine.Start(); defer engine.Stop();`
  pass `engine` to `NewSubscriber`. `*geofence.Engine` structurally satisfies `ingest.ReadEvaluator`.
**Validation**: `just backend lint && just backend build && just backend test`.

### Task 9: Integration tests (RLS harness)
**Files**: `ingest_integration_test.go` (extend), `alarm_events_integration_test.go` (new) — MODIFY/CREATE
- Extend PersistReads tests: assert `res.Resolved` length/contents; add a boundary helper
  (`UPDATE trakrf.scan_points SET is_boundary=true WHERE ...`) and assert `IsBoundary`; duplicate-in-batch
  still yields the read in `Resolved` though `Dropped["conflict"]==1`.
- alarm_events: `InsertAlarmEvent` writes under org context; row visible only under that org's GUC (RLS);
  missing GUC throws (no superuser bypass) — mirror TRA-900's RLS-role assertions.
**Validation**: `just backend test` (unit) + integration-tagged run green.

### Task 10: Final validation + build log
- `just backend validate` (fmt, vet, build, test) green; `go test -race ./internal/geofence/...` green.
- Write `spec/tra-901-geofence-rules-engine/log.md` (build proof) per CSW.

## Risk Assessment
- **Risk**: import cycle ingest↔geofence. **Mitigation**: `ReadEvaluator` interface lives in `ingest`
  and references `storage.ResolvedRead`; geofence imports storage, not ingest; serve wires. No cycle.
- **Risk**: alarm_events INSERT lacks privilege on the RLS role in integration DB. **Mitigation**: mirror
  asset_scans (no in-migration grant — init-grants/default-priv path covers it); if the integration
  insert fails on privilege, add the same GRANT the harness expects (verify, don't assume).
- **Risk**: concurrent latch admit double-fires. **Mitigation**: LoadOrStore first-sight + atomic Swap
  expiry; covered by a `-race` concurrency test.
- **Risk**: RSSI `0` sentinel ambiguity. **Mitigation**: documented decision — `0` suppresses; test asserts it.
- **Risk**: per-point override scope creep in Task 2/6. **Mitigation**: explicit fallback to global-only
  with a TODO if the JSON extract isn't a clean one-liner.

## Integration Points
- Config: new `GEOFENCE_*` env vars (defaults make it inert-safe; engine only runs when `MQTT_URL` set).
- Serve wiring: engine lifecycle tied to subscriber lifecycle.
- DB: new `alarm_events` table; init-grants (infra) will need it for prod — note in PR (no cross-repo edit).

## VALIDATION GATES (MANDATORY)
After every task, from project root:
- Gate 1 (lint): `just backend lint`
- Gate 2 (build/vet): `just backend build` (vet is in lint)
- Gate 3 (unit tests): `just backend test`; for geofence concurrency: `cd backend && go test -race ./internal/geofence/...`
- Integration (after Task 9): the project's integration-tagged storage run.
If any gate fails → fix → re-run → loop. After 3 failed attempts on one task → stop and report.

## Validation Sequence
Per task: lint → build → test (scoped). Final: `just backend validate` + `-race` geofence + integration storage suite.

## Plan Quality Assessment
**Complexity Score**: 7/10 (HIGH by file count) — overridden: one atomic, non-splittable feature; mitigated by per-task commits + frequent gates.
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear spec; all design decisions resolved in spec.md
✅ Direct reference patterns: latch=ratelimit/limiter.go, persist=ingest.go, migration=000008, integration harness=ingest_integration_test.go
✅ No new dependencies
✅ Additive only — zero risk to existing scan path
⚠️ New `geofence` package (no prior geofence code) — but assembled entirely from established patterns
⚠️ alarm_events RLS-role privilege path to verify in integration (mitigated)

**Assessment**: High-confidence, pattern-driven build with one cohesive seam; the only unknowns are mechanical (grant path, metadata extract) and have explicit fallbacks.

**Estimated one-pass success probability**: 85%

**Reasoning**: Every component mirrors a concrete existing file; the architecture (evaluate-after-persist, interface seam, aging latch) is decided and cycle-free. Residual risk is integration-DB grant mechanics and the optional per-point override, both with documented fallbacks.
