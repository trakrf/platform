# Feature: scan_devices / scan_points CRUD — device-type-aware model, CS463 path (TRA-899)

## Metadata
**Workspace**: monorepo (database + backend + frontend)
**Type**: feature
**Ticket**: https://linear.app/trakrf/issue/TRA-899
**Parent epic**: TRA-897 (Frederick Health fixed-reader asset-egress geofence demo). Foundational for TRA-900 (MQTT ingestion) and TRA-901 (geofence rules engine).

## Outcome
A logged-in operator can register, list, edit, and delete fixed-reader scan devices and their capture points (scan_points) through a management UI and REST API; the data model carries the device-type discriminator, transport, publish_topic, and a geofence-boundary flag the downstream ingestion/rules tickets key off; and the CS463 read path resolves against this CRUD-managed registry instead of auto-creating devices from scan traffic.

## User Story
As a TrakRF operator setting up a fixed-reader site
I want to register my CS463 reader and its antennas (capture points) and mark which ones are geofence boundaries
So that incoming reads resolve to known devices/points, boundary points feed the geofence rules engine, and unregistered hardware is ignored rather than silently auto-provisioned.

## Context

**Current state**
- `scan_devices` and `scan_points` tables exist in the frozen foundation migration `backend/migrations/000005_scan_devices_and_points.up.sql`, but with the **pre-rename** model:
  - natural key column is `identifier` (not `external_key`) — a miss from the TRA-475/TRA-554 `identifier → external_key` public rename, which only swept `assets` and `locations` because scans were internal-only at the time.
  - `type VARCHAR(50)` free string (no enum); no `transport`; no `publish_topic`.
  - `scan_points` has `antenna_port`, `location_id`, but no geofence-boundary marker.
- The CS463 "parser" already lives in the database: `process_tag_scans()` (AFTER-INSERT trigger on `tag_scans`, in frozen `backend/migrations/000010_stored_procedures.up.sql`) resolves `rfidReaderName → scan_devices.identifier` and `capturePointName → scan_points.identifier`, and **auto-creates** locations, scan_devices, and scan_points from message contents — a crutch from before this CRUD existed.
- There is **no** Go storage layer, **no** REST handlers, and **no** management UI for either table.
- Scan devices/points are **internal-only** (the public API v1 surface is logical data only — assets/locations/asset_scans; raw physical tables stay internal per the launched v1 scope).

**Desired state**
- A new increment migration `000011` (up + down) layered on the frozen foundation:
  1. Rename `identifier → external_key` on both `scan_devices` and `scan_points`, sweeping every dependent object (indexes, UNIQUE constraints, column comments).
  2. Introduce PG enum types and convert `scan_devices.type` to one (column name stays `type`; the value set is bounded):
     - `scan_device_type AS ENUM ('csl_cs463', 'gl_s10', 'esp32_ble_generic', 'csl_cs108')`
     - `scan_transport AS ENUM ('mqtt', 'web_ble')`
  3. Add `scan_devices.transport scan_transport NOT NULL DEFAULT 'mqtt'`.
  4. Add `scan_devices.publish_topic VARCHAR(255)` (nullable; the read channel / routing key TRA-900 routes on), with a partial UNIQUE index per org and a lookup index.
  5. Add `scan_points.is_boundary BOOLEAN NOT NULL DEFAULT false` (the geofence-boundary marker TRA-901 keys off; the associated zone is the existing `scan_points.location_id`).
  6. `CREATE OR REPLACE` `process_tag_scans()` to (a) use the renamed `external_key` columns, and (b) **drop the auto-create of scan_devices and scan_points** (and the now-orphaned auto-create of locations that existed only to back an auto scan_point). Reads from devices/points not present in the CRUD registry resolve to nothing and produce no `asset_scans` — silently ignored, consistent with TRA-901's membership-filter philosophy.
- Internal (session-authenticated) REST CRUD for both resources, following the existing internal management-route pattern (`middleware.Auth` subtree in `router.go`, not the public API-key/scope surface, not in `openapi.public`).
- A minimal management UI (list / add / edit / delete) for scan devices and their scan points, including the `publish_topic` field, matching the existing React/TanStack-Query/Tailwind patterns.
- Representative CS463 (and GL-S10, for schema coverage) payload **fixtures**, used to test the `process_tag_scans` resolution path and as contract-test data.

**Examples / patterns to follow**
- Migration increment convention: `backend/migrations/README.md` (000001–000010 frozen; 000011+ up+down). Enum precedent: `org_role`, `refresh_token_type` (`CREATE TYPE … AS ENUM`).
- Rename-sweep discipline: enumerate every renamed PG object, not just the table (column, index, UNIQUE constraint, comment, function body, cutover SQL, diff allowlist).
- Storage: `backend/internal/storage/assets.go` / `locations.go` — every method wrapped in `WithOrgTx(ctx, orgID, …)`; new storage files MUST be added to the `check-rls-guard` list in `backend/justfile`.
- Handlers (internal, session-auth): the `middleware.Auth` group in `backend/internal/cmd/serve/router.go:130-148` (where orgs/users/assets `RegisterRoutes` mount). Swaggo annotations tagged for the **internal** spec only (no `,public`).
- Frontend: `APIKeysScreen.tsx` (simple CRUD), `LocationsScreen.tsx` + `locations/LocationForm.tsx` (list + form modal), `hooks/locations/*`, `lib/api/locations/index.ts`, `types/locations/index.ts`. Modal gate pattern (return null when closed). ConfirmModal for delete.

## Technical Requirements

### Database (migration `000011_scan_device_model.up.sql` / `.down.sql`)
- **Frozen stack untouched**: do NOT edit `000005` or `000010`. All changes are `ALTER`/`CREATE OR REPLACE` in `000011`.
- Rename `scan_devices.identifier → external_key` and `scan_points.identifier → external_key`. Rename dependent indexes (`idx_scan_devices_identifier → idx_scan_devices_external_key`, same for points) and the UNIQUE constraints (`UNIQUE(org_id, external_key, valid_from)`). Update/replace the column comments.
- `CREATE TYPE scan_device_type AS ENUM (…)` and `scan_transport AS ENUM (…)`.
- Map any pre-existing `type` values to the enum before the cast — notably the legacy auto-create value `'rfid_reader'` → `'csl_cs463'` (CS463 is the only mqtt rfid reader in the demo). `ALTER COLUMN type TYPE scan_device_type USING …`.
- Add `transport`, `publish_topic` (+ `CREATE UNIQUE INDEX … ON scan_devices (org_id, publish_topic) WHERE publish_topic IS NOT NULL AND deleted_at IS NULL` and a plain index for routing lookups), `is_boundary`.
- `CREATE OR REPLACE FUNCTION process_tag_scans()`: external_key column refs; remove the scan_devices / scan_points / locations auto-create INSERT blocks; keep the asset_scans INSERT resolving against registered scan_points (`JOIN scan_points sp ON sp.org_id = … AND sp.external_key = capturePointName`). Asset/tag auto-create from EPC is **retained** as-is for this ticket (asset membership is TRA-901's concern) — flag in review.
- `.down.sql` reverses: restore `process_tag_scans` to its 000010 form, drop new columns/indexes/enums, rename `external_key → identifier`.
- **Rename sweep beyond migrations** (same PR):
  - `backend/database/cutover/05_scan_devices_and_points.sql` — target column list uses `external_key`; FDW source still reads cloud `s.identifier`; device-join uses `t_dev.external_key`.
  - `backend/database/test/expected_diff_allowlist.txt` — add a category covering the 000011 deltas (column rename, enum conversion, new columns, function-body change) so the schema-diff contract test stays green.
- A fresh `just backend migrate` against an empty DB applies 000001→000011 cleanly.

### Backend (Go)
- Models: `internal/models/scandevice/`, `internal/models/scanpoint/` (entity struct + Create/Update request DTOs + view), matching the `asset`/`location` model package shape. `type`/`transport` validated with `validator` `oneof` mirroring the enum.
- Storage: `internal/storage/scan_devices.go`, `internal/storage/scan_points.go` — Create / GetByID / List (paginated) / Update / Delete (soft-delete via `deleted_at`), all wrapped in `WithOrgTx`. Default `publish_topic` to `trakrf.id/{external_key}/reads` at the app layer when omitted/null. scan_points scoped to a parent device; deleting a device soft-deletes its scan_points.
- Add both new storage files to `check-rls-guard` `RLS_FILES` in `backend/justfile`.
- Handlers: `internal/handlers/scandevices/`, `internal/handlers/scanpoints/` — internal, session-auth, registered in the `middleware.Auth` group. Routes (final shape settled in plan): `/api/v1/scan-devices` (GET list, POST), `/api/v1/scan-devices/{scan_device_id}` (GET, PATCH merge-patch, DELETE); scan_points nested under a device for create/list, addressed by id for get/update/delete.
- Swaggo annotations for the **internal** spec only (no `,public` tag); `rename_public.go` is NOT touched. `just backend api-spec` regenerates with no drift; CI codegen smokes + drift check pass.

### Frontend (React/TS)
- `types/scandevices/index.ts` (hand-written, matching the API contract incl. `external_key`, `type`, `transport`, `publish_topic`, `metadata`, and scan_point `antenna_port` / `is_boundary` / `location_id`).
- `lib/api/scandevices/index.ts` (axios module), `hooks/scandevices/useScanDevices.ts` + `useScanDeviceMutations.ts` (TanStack Query), optional zustand store.
- `components/ScanDevicesScreen.tsx` + `components/scandevices/{ScanDeviceFormModal,ScanDeviceForm}.tsx` and scan-point management (list points per device, add/edit/delete; `is_boundary` toggle + zone/location select; `publish_topic` field with the default-convention hint). Delete via `ConfirmModal`. Register the tab in `App.tsx` (VALID_TABS, lazy import, tabComponents/loadingScreens) and `TabNavigation`.

### Tests
- Backend integration tests (`//go:build integration`, `testutil.SetupTestDatabase`): CRUD happy paths + org-scoping/RLS for both resources; publish_topic default + per-org uniqueness; soft-delete cascade device→points; enum rejection of bad `type`/`transport`.
- `process_tag_scans` behavior tests: a `tag_scans` insert whose `rfidReaderName`/`capturePointName` match **registered** device/point produces an `asset_scan`; an **unregistered** device produces **no** device/point row and **no** asset_scan (auto-create removed).
- Fixtures: representative CS463 + GL-S10 payloads under a fixtures dir, used by the above and available for contract tests. (If live broker capture isn't reachable in this session, build fixtures from the documented payload shapes — `rfidReaderName`, `capturePointName`, `antennaPort`, `rssi` as string, `timeStampOfRead` microseconds, empty `pcEthernetMACAddress`, `712AC12F` EPC prefix — and flag for validation against real captures.)
- Frontend: Vitest unit test for the form modal; happy-path coverage consistent with existing screens.

## Validation Criteria
- [ ] `000005` and `000010` are byte-for-byte unchanged; all schema changes live in `000011` (up + down).
- [ ] `just backend migrate` applies 000001→000011 clean on an empty DB; `000011` down reverses cleanly.
- [ ] `scan_devices`/`scan_points` expose `external_key` (no `identifier`); `type` and `transport` are enum-typed; `publish_topic` and `is_boundary` exist with the specified indexes/defaults.
- [ ] `process_tag_scans` no longer auto-creates scan_devices/scan_points (verified by the unregistered-device test); registered-device reads still land in `asset_scans`.
- [ ] REST CRUD works for both resources under session auth, org-scoped; not present in `openapi.public`; present in the internal spec.
- [ ] Management UI lists/creates/edits/deletes devices and their scan_points incl. `publish_topic` and `is_boundary`.
- [ ] `just backend lint` (incl. `check-rls-guard` with the new files), `just backend test`, `just backend test-integration`, `just backend api-spec` (no drift), and `just frontend validate` all pass.
- [ ] Schema-diff contract test passes with the updated `expected_diff_allowlist.txt`.

## Success Metrics
- [ ] Full CRUD + UI demoable: register a `csl_cs463` device with `publish_topic`, add antenna scan_points, mark one `is_boundary`, edit, delete.
- [ ] Zero RLS escapes: cross-org read/write of scan devices/points is impossible (RLS-enforced test role proves it).
- [ ] Ingestion correctness: registered device → asset_scan created; unregistered device → no rows created.
- [ ] All workspace validation gates green; OpenAPI internal spec regenerates with no manual edits and no public-surface leakage.

## Out of Scope (per ticket + epic)
- MQTT subscriber / ingestion wiring (TRA-900) and the geofence rules engine (TRA-901).
- GL-S10 / ESP32 BLE parser + ingestion paths (TRA-910); CS108 handheld attribution (TRA-911) — schema-supported only.
- `command_topic` / command channel; reader HTTP config (power/profiles).
- Moving parse logic from the DB function into Go (explicitly deferred — start with the DB function, adapt later if needed).
- Changing asset/tag auto-create in `process_tag_scans` (kept as-is; revisit under TRA-901).

## Open Decisions (made autonomously; flag at review)
1. Column stays named `type` (not renamed to `device_type`) — name is immaterial per Mike; value set bounded by enum. ✔ confirmed.
2. `identifier → external_key` rename applied to scan tables to fix the sweep miss. ✔ confirmed.
3. Removed location auto-create alongside scan_point auto-create in `process_tag_scans` (it existed only to back the auto scan_point); asset/tag auto-create retained. — judgment call, reviewable.
4. CRUD is internal/session-auth only (not public API). — follows v1 logical-only scope.
5. scan_points addressed via routes nested under their device. — settle exact route shape in plan.

## References
- Ticket TRA-899; epic TRA-897; TRA-900; TRA-901.
- `backend/migrations/000005_scan_devices_and_points.up.sql` (frozen baseline schema).
- `backend/migrations/000010_stored_procedures.up.sql:24-126` (frozen `process_tag_scans`).
- `backend/database/cutover/05_scan_devices_and_points.sql`; `backend/database/test/expected_diff_allowlist.txt`.
- `backend/internal/storage/{assets,locations}.go`; `backend/internal/storage/transactions.go` (`WithOrgTx`).
- `backend/internal/cmd/serve/router.go:130-148` (internal session-auth subtree).
- `backend/justfile` (`check-rls-guard`, `api-spec`, `migrate`, `test-integration`).
- Frontend: `src/components/{APIKeysScreen,LocationsScreen}.tsx`, `src/components/locations/*`, `src/hooks/locations/*`, `src/lib/api/locations/index.ts`, `src/types/locations/index.ts`, `src/App.tsx`.
</content>
</invoke>
