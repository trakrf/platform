# CS463 ingestion load-test harness (TRA-994)

Drives a real CS463 reader firehose (or synthetic publish) into an isolated local
stack to characterize the ingestion path and tune the reader dedup window.

## Key finding

The **reader's MQTT publish path is the bottleneck**, not the backend. At
`dedup=0` a CS463 wedges its own publish in ~4 min (publish-buffer exhaustion);
the single-replica backend consumer handled everything one reader could emit
(~270 reads/s) with zero drops. See
`docs/superpowers/specs/2026-06-16-cs463-ingest-load-and-dedup-design.md`.

## Golden config (validated, live on cs463-212 → preview)

`TrakRF-data-format` (trimmed: only `epc, timeStampOfRead, antennaPort, rssi`) +
`duplicateEliminationWindow=500ms` + `antennaDifferentiation=ON`. Dedup window is
a documented customer knob (geofence → 500ms; storage-room-at-rest → larger).

## Run it

```bash
# 1. Bring up the isolated stack (default dev compose + this overlay):
docker compose -f docker-compose.yaml -f load-tests/ingest/docker-compose.loadtest.yaml up -d
# 2. Migrate + seed (org + cs463 device + scan_points + ~145 tags from real EPCs):
just backend migrate
psql "$PG_URL_LOCAL" -f load-tests/ingest/seed.sql
# 3. Point the reader at this host's broker (plaintext 1883) via the /API, then:
READER_API_PASS=... ./load-tests/ingest/sweep.sh 0 250 500 1000   # dedup cliff sweep
./load-tests/ingest/measure.sh 120 2                              # per-hop rate instrument
```

## Files

- `seed.sql` — local seed (org `ingest-loadtest`, device `trakrf.id/cs463-212/reads`, 4 scan_points, 145 tags).
- `mosquitto.conf` — plaintext anonymous broker, $SYS stats every 2s.
- `docker-compose.loadtest.yaml` — overlay: adds the broker, points backend ingest at it (`LOG_LEVEL=warn`).
- `measure.sh` — samples backend `/metrics` + broker `$SYS`; prints per-hop rates and the emit-vs-processed gap.
- `sweep.sh` — sets each dedup window via the reader `/API`, soaks, detects emit collapse. Needs `READER_API_PASS`.
- `reader-backup/` — gitignored; original reader config (holds credentials).
