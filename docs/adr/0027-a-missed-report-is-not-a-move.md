# ADR 0027 — A missed report is not a move

Date: 2026-11-27
Status: Proposed
Tracking: TRA-1365, TRA-1364

## Context

With BLE gateways at room density, every badge is heard by several gateways at
once. Until TRA-1365 the stored location was whichever gateway's message
arrived first each minute (`asset_scans` is one row per asset per minute,
ADR 0005, and the insert was `ON CONFLICT DO NOTHING`). On preview, four
stationary tags heard by the three homelab GL-S10s recorded 147–236 location
changes each in six hours (2026-11-27), and the stored room matched the
strongest gateway less than half the time.

Arbitrating by signal strength fixes the race. A capture from the same three
gateways shows the trap in the obvious version, though
(`backend/internal/arbitration/testdata/homelab_stationary_30m.csv.gz`). A
gateway publishes about once a second, but it routinely leaves a tag out of
its reports for 10–30 s. In two cases it left out, for four to five minutes, a
tag it hears at −37 dBm, while the next rooms went on hearing that tag at −71
and −80. A rule of "the assigned room is unheard for a window, so hand over to
whoever is heard" moves that tag to a room it never left, then back. Replayed
against the capture, that rule switched two of the four tags 15 and 22 times in
30 minutes.

## Decision

1. **BLE gateways store an arbitrated location.** For device types in
   `ARBITRATION_DEVICE_TYPES` (default: `gl_s10`, `moko_mk107`,
   `esp32_ble_generic`), the minute row holds the location the arbiter decides,
   not the reporting gateway's. A challenger must beat the assigned location by
   a margin (6 dB) for a dwell (5 s), over a window mean of RSSI (10 s). A
   mid-minute switch updates that minute's row
   (`backend/internal/storage/ingest_arbitrated.go`, `upsertArbitratedScan`).
2. **Unheard is not gone.** An assigned location that drops out of the window
   yields at once only to a location heard about as strongly as it last was
   (within the margin). That is evidence the asset turned up somewhere
   comparable, so it moved. Anything weaker waits out a long silence (10 min).
   A seeded location (after a restart) has no level, so it waits for the same
   silence (`backend/internal/arbitration/arbiter.go`, `decide`).

   **Except going straight back.** At a boundary that two rooms hear about
   equally, a still asset is handed back and forth on every gateway's routine
   10–30 s report gaps, because the other room always counts as about as strong.
   A walk does not go straight back to the room it just left. So a return to the
   location left less than 5 minutes ago (`ARBITRATION_RETURN_WINDOW`) needs its
   lead held for 30 s (`ARBITRATION_RETURN_DWELL`) instead of the dwell, and is
   never made on a gap alone. Replayed against a homelab capture of a tag on a
   bookshelf at such a boundary
   (`backend/internal/arbitration/testdata/homelab_boundary_57m.csv.gz`), this
   cuts 16 moves in 51 minutes to 5, and leaves a hand-timed walk through three
   rooms unchanged: all 12 arrivals stored, the worst 8.7 s late
   (`homelab_walk_4m.csv.gz`).
3. **One component owns each move.** Arbitration works out the stored change,
   with its origin and the instant of the switch, and hands it over as
   `ResolvedRead.Move`. `MoveStage` and the `asset.moved` evaluator record and
   emit only that for arbitrated reads, and derive nothing from the observed
   location (`backend/internal/assetevent/move_stage.go`, `evaluator.go`).
4. **What was observed still flows.** Resolved reads keep the observed
   location, so geofence and the other stages see every gateway's reads
   unchanged. Arbitration changes what is stored, not what is seen.

## Consequences

- Fixed RFID readers keep the ordinary path. Enabling arbitration for them is
  a config change, but it needs its own soak and two answers first: how it
  interacts with `crossing`, which already decides From/To at multi-antenna
  gates, and whether one margin fits RSSI that is not comparable across
  readers and antennas.
- A badge that leaves every room's strong coverage takes up to the silence
  (10 min) to be stored elsewhere. Rooms that each have a gateway are handled
  by rule 2 within seconds.
- **A still asset at a true equal boundary still moves now and then**: about
  six times an hour in the bookshelf capture, each one a real 6 dB lead or a
  10 s gap that came more than 5 minutes after the last move. RSSI cannot tell
  those from a short visit. Walking back into the room just left is stored up
  to 30 s late.
- So stored location is deliberately the strict tier. Two things are left for
  later: stored history that records stays rather than passes, and a live
  position (the strongest room right now, before these rules) for map views.
  Neither is decided here.
- Tuning: `TestSweep` (`ARBITRATION_SWEEP=1`) replays all three homelab
  captures across margin, dwell and the return settings. Rerun it with new
  captures before changing a default.
- The arbiter is in-memory and assumes one ingest consumer. The backend runs
  one replica (`trakrf/infra` `helm/trakrf-backend/values.yaml`, `replicaCount:
  1`; a second replica would already double-write `asset_scans`, TRA-907). On
  restart it seeds from the stored location.
- A manual save still wins its minute: the arbitrated upsert never overwrites
  a row with no `tag_scan_id`.
