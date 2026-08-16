# ADR 0005 — Asset location history has minute granularity; alarms do not read it

Date: 2026-08-16
Status: Proposed
Tracking: TRA-1118 (this change), TRA-1117 (report freshness, same surface, independent problem), TRA-1104 (whose commit message "ADR 0004" refers to a draft that never landed — see Numbering note)

## Context

`trakrf.asset_scans` stored one row per reader read at microsecond precision,
retained 365 days uncompressed. A tag parked in a fixed reader's field produces
a row for every read, all day. Measured on preview 2026-08-07: four soak-test
tags were 99.96% of all `asset_scans` writes (162.5 rows/min stored, 4/min of
information), and the table sat at 5,551 MB against the 488 MB of raw
`tag_scans` it derives from — ~349 bytes/row for ~48 bytes of data, the gap
being the PK plus four secondary indexes written per redundant read.
Extrapolated to a storage room with 100 parked assets: ~734 GB/year, versus
~18 GB/year after minute dedup.

Product decision (Mike + Tim): minute-level asset location history is enough
for every customer we can name. Sub-minute is unnecessary precision.

## Decision

**One row per (asset, minute).** Both write paths truncate `timestamp` to the
minute (`storage.ScanGranularity`); the existing PK
`(timestamp, org_id, asset_id)` does the dedup. No migration, no backfill —
old full-precision rows age out under the 365-day retention.

The granularity is a hardcoded constant. An org-level knob (10s–10min) was
rejected as YAGNI: it costs a migration, config surface, capability check and
test matrix for a customer that does not exist.

**Conflict handling is asymmetric by design:**

* **Ingest (`PersistReads`): `ON CONFLICT DO NOTHING`.** Runs at reader
  frequency (~40 reads/asset/min measured); a conflict must be a no-op that
  creates no tuple version, or the update churn re-creates the MVCC bloat this
  change removes.
* **Manual save (`SaveInventoryScans`): `ON CONFLICT DO UPDATE`.** Runs at
  human frequency, and an operator's explicit save beats a passive reader read
  in the same minute. Without a conflict clause, the first same-minute
  collision after truncation would raise a unique violation and 500 the save.
  `DO NOTHING` would be worse: the response `Count` derives from the request,
  so the toast would report success while writing nothing.

**Location stays out of the conflict target.** Adding it would require it in a
unique index, allowing one asset two rows in the same minute at different
locations with identical timestamps — and the report's
`last(location_id, timestamp)` would then break ties arbitrarily: silent,
non-deterministic wrongness on exactly the two-antenna doorway case. Accepted
instead: within a minute, the first observed location wins (operator saves
excepted, above), and the next minute's bucket re-asserts reality.

**Events emit only for observations that landed a row.** Truncation widens
storage's dedup window from one message to one minute, so a conflict-dropped
observation whose location differs from the frozen bucket would re-publish the
identical move once per message for the rest of the minute — a webhook
amplifier. Provenance is carried per-path:

* Ingest: `ResolvedRead.Stored` (from `RowsAffected`), filtered in
  `assetevent.Evaluator`, counted as `suppressed{reason="not_stored"}` so
  flapping stays visible in Prometheus. `res.Resolved` itself still carries
  every membership-passing read — the geofence engine must keep seeing every
  observation, so filtering is per-consumer, never done by trimming the slice.
* Handheld save: `SaveInventoryResult.InsertedAssetIDs`
  (`RETURNING xmax = 0`), filtered in the inventory handler. A same-minute
  re-save is a `DO UPDATE` that destroys the bucket's first-observed location,
  leaving nothing trustworthy to diff against: evaluating it emits a phantom
  first-sighting or a duplicate move on every re-save.

**The previous-location lookup bounds at the truncated minute.** Stored
timestamps sit at the minute floor while `receivedAt` does not, and
`PreviousAssetLocations` uses a strict `timestamp < before`. Bounding at raw
`receivedAt` would let the just-stored bucket qualify as its own predecessor
and suppress every genuine move as `no_change`. This is load-bearing; the
symptom of getting it wrong is total silence, not an error.

Accepted consequence, both paths: a genuine mid-minute move is delayed to the
next bucket — ≤60s for movement events and their webhooks. Bonus: the minute
bucket is a debounce window, at most one location change per asset per minute —
crude but real hysteresis on the two-antenna doorway problem.

## Why the alarm path is unaffected — and where to extend it

This is the finding that made a ≤60s movement-event delay acceptable at all:

* Alarms run on the **observation stream** via `ingest.MultiEvaluator`. The
  geofence engine latches per `(org, output device, epc)` with a re-arm TTL,
  never consults previous location, and never reads `asset_scans`
  ("best-effort and never blocks ingestion or the authoritative asset_scans
  write"). "Read a tag, ring a bell" is structurally independent of
  asset-location history. **History granularity must never become alarm
  latency** — any future change that routes alarm decisions through
  `asset_scans` re-couples them and inherits the ≤60s delay.
* Extension point for future alarm webhooks/notifications:
  `geofence.Engine.recordFire`. It fires once per read that drove at least one
  device on (guarded by `firedAny`), already post-latch and post-startup-grace,
  and already writes the durable `alarm_events` row — a publish added there
  inherits the dedup semantics for free, with no latch changes.

Also deliberately untouched: `assetevent.evaluate()`'s change-based suppression
(`suppressed{reason="no_change"}`). Movement events are change-based rather
than sample-based, which is strictly better than the minute-bucket rule.

## Numbering note

A proposed ADR 0004 was folded into 0003 by commit `6b54d3b2` and never landed;
TRA-1104's commit message "docs: record ADR 0004 — the migrating role owns
every object" refers to that never-landed draft. The 0004 that exists is
declared-platform-version (TRA-1126). This document is therefore 0005.
