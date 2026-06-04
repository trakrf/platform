# TRA-899 Build Log

Branch: `feat/tra-899-scan-devices-scan-points-crud`

## What shipped

### Database — migration `000011` (up + down), layered on the frozen `000001–000010` stack
- Renamed `identifier → external_key` on `scan_devices` and `scan_points` (+ indexes, UNIQUE constraints, comments) — completing the TRA-475/554 rename sweep that had skipped the (then internal-only) scan tables.
- New PG enums `scan_device_type` (`csl_cs463`/`gl_s10`/`esp32_ble_generic`/`csl_cs108`) and `scan_transport` (`mqtt`/`web_ble`); `scan_devices.type` retyped to the enum (legacy `rfid_reader` → `csl_cs463`).
- Added `scan_devices.transport` (NOT NULL default `mqtt`), `scan_devices.publish_topic` (nullable; partial-unique-per-org index + lookup index), `scan_points.is_boundary` (default false).
- `CREATE OR REPLACE process_tag_scans`: dropped scan_device/scan_point/location auto-create (devices/points are now CRUD-managed; unregistered reads resolve to nothing); kept asset/tag auto-create; **schema-qualified all table refs** so the trigger no longer depends on the connection `search_path`.
- Rename sweep: `database/cutover/05_scan_devices_and_points.sql` target columns; new category in `database/test/expected_diff_allowlist.txt`.

### Backend
- `models/scandevice`, `models/scanpoint` (enum-validated `type`/`transport`).
- `storage/scan_devices.go`, `storage/scan_points.go` — CRUD via `WithOrgTx`, soft-delete, device→points cascade, publish_topic default + per-org uniqueness; added to `check-rls-guard`.
- Internal (session-auth) handlers `handlers/scandevices`, `handlers/scanpoints`; wired into the `middleware.Auth` subtree in `cmd/serve/router.go` (+ `serve.go`, `serve_test.go`). Swagger `@Tags …,internal` → internal spec only, never the public API.

### Frontend
- types / axios api client / TanStack-Query hooks / `ScanDevicesScreen` + device & scan-point form modals (+ ConfirmModal deletes). Scan-point form uses a **location selector** (each antenna associates to a location/zone). Registered the `scan-devices` tab.

## Notable findings
- Flaky `process_tag_scans` test root cause: the fixture's 2024 `timeStampOfRead` vs the `asset_scans` 365-day retention policy — the retention worker intermittently reaped the just-created old chunk. Fixed by stamping the test scan at `now()`. The trigger itself was always correct.
- Decision (Mike): keep parse in the DB function for now; moving parse to Go (and possibly demoting `tag_scans` to a log) is backlogged, out of TRA-899 scope. TRA-900 (MQTT ingestion) and TRA-901 (geofence) remain separate.

## Validation gates (all green)
- `just backend lint` — check-rls-guard clean, fmt, vet ✓
- `just backend test` (unit) ✓
- `just backend test-integration` — full storage suite (incl. pre-existing) + scandevices handler suite ✓
- `just backend api-spec` — internal spec carries the endpoints; **no public-spec drift** ✓
- `just frontend typecheck / lint (0 errors) / test (6 new, 1090 total) / build` ✓

## Held
Per Mike's instruction: autonomous through PR, **hold for diff review before merge** (no-merge-without-diff-review).
