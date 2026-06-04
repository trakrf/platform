# TRA-903 — Alarm device CRUD + Shelly Gen4 trigger

**Status:** Design
**Date:** 2026-06-04
**Ticket:** [TRA-903](https://linear.app/trakrf/issue/TRA-903) (parent TRA-897; related TRA-901 geofence engine, TRA-906 MQTT fire path)
**Branch:** `feat/tra-903-alarm-device-shelly`

## Goal

Model and manage alarm **output** devices, and fire a demo Shelly Gen4 relay when
the geofence engine (TRA-901) decides to fire. The CRUD + driver are independent
of the engine; only the auto-fire wiring depends on it via the existing
`geofence.Firer` seam.

This is **demo-track / pre-launch** work (the `fixed-reader` track is not prod-live).
Single-replica, internal-only, no public-API surface.

## Scope

In:
- `alarm_devices` model + migration `000014` (mirrors `scan_devices` structure).
- Internal-only REST CRUD (list/get/create/update/delete) + **test-fire** + **reset** actions.
- Shelly Gen4 driver: `Switch.Set` over local HTTP RPC; fail-quiet.
- `ShellyFirer` implementing `geofence.Firer`; bind an `AlarmEvent` → bound alarm devices.
- Minimal management UI (list / add / edit / delete) with a per-row **Test-fire** button.

Out (per ticket):
- Firing over MQTT (broker subscribe) — that's TRA-906.
- A separate "geofence rule" entity — none exists; binding is to a boundary `scan_point`.

## Architecture

### 1. Data model — migration `000014_alarm_devices.up.sql`

New PG enum + table, mirroring `scan_devices` (000005/000011) conventions:
obfuscated `id` via the `generate_obfuscated_id` trigger (wire-exposed by id in
the internal CRUD), `update_updated_at_column` trigger, RLS org-isolation policy,
soft delete, no in-migration GRANTs (infra init-grants Job + integration harness
handle them, same note as 000013).

```sql
SET search_path = trakrf, public;

CREATE TYPE alarm_device_type AS ENUM ('shelly_gen4');

CREATE TABLE alarm_devices (
    id            BIGINT PRIMARY KEY,                 -- Feistel-obfuscated via trigger
    org_id        BIGINT NOT NULL REFERENCES organizations(id),
    name          VARCHAR(255) NOT NULL,
    type          alarm_device_type NOT NULL DEFAULT 'shelly_gen4',
    base_url      VARCHAR(255) NOT NULL,              -- e.g. http://192.168.50.66
    switch_id     INT NOT NULL DEFAULT 0,             -- Shelly channel (Switch.Set id=)
    scan_point_id BIGINT REFERENCES scan_points(id),  -- nullable binding to a boundary point
    is_active     BOOLEAN NOT NULL DEFAULT true,
    metadata      JSONB DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX idx_alarm_devices_org         ON alarm_devices(org_id);
CREATE INDEX idx_alarm_devices_scan_point  ON alarm_devices(scan_point_id) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_alarm_device_id_trigger
    BEFORE INSERT ON alarm_devices
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_alarm_devices_updated_at
    BEFORE UPDATE ON alarm_devices
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE alarm_devices ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_alarm_devices ON alarm_devices
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
```

Down: `DROP TABLE IF EXISTS alarm_devices; DROP TYPE IF EXISTS alarm_device_type;`

**Binding rationale:** the alarm binds to a **logical `location_id`**, not a
scan point — we care that an asset was seen *at a location*, not which
reader/antenna observed it. The geofence engine already resolves the tripped
scan point to a location (`AlarmEvent.LocationID`), so the firer matches on that.
`location_id` is nullable so a device can be created and **test-fired before**
it's wired. The auto-firer only matches active devices whose `location_id`
equals the event's location; NULL location (unmapped scan point) or a location
with no alarm device is a **no-op**.

**Two firing invariants** (both already enforced upstream, no extra code):
1. Only EPCs that resolve to a **registered asset** ever reach the firer — the
   geofence engine evaluates `Resolved` reads only (membership filter from
   TRA-901's PersistReads tag→asset join). A random/unregistered tag (library
   book, handbag) never produces an AlarmEvent.
2. A read that resolves to a location with **no alarm device** (or to a scan
   point with no location) fires nothing — the firer's lookup returns empty.

### 2. Shelly Gen4 driver — `internal/alarm/shelly`

Pure transport, no DB. Drives one relay channel over local HTTP RPC per the
Gen2+ API. `Switch.Set` turns the relay on/off:

```
POST {base_url}/rpc   body: {"id":1,"method":"Switch.Set","params":{"id":<switch_id>,"on":<bool>}}
```

```go
type Client struct { http *http.Client }            // short timeout (~3s)
func New(timeout time.Duration) *Client
func (c *Client) Set(ctx, baseURL string, switchID int, on bool) error
```

**Fail-quiet:** all comms failures return an error that callers log but never
treat as fatal. The Shelly's own default-off behaviour (no comms ⇒ relay off) is
a device-config concern; from our side we only ever issue explicit Set calls.

### 3. `ShellyFirer` — `internal/alarm/firer.go` (implements `geofence.Firer`)

```go
type Firer struct { store deviceLookup; client *shelly.Client; log zerolog.Logger }
func (f Firer) Fire(ctx, ev geofence.AlarmEvent) error
```

On `Fire(ev)`: look up active `alarm_devices` for `ev.OrgID` bound to
`ev.ScanPointID` (one `WithOrgTx` SELECT), and `Set(on=true)` each via the
driver. Errors are logged per-device and aggregated; the engine already treats
`Fire` as best-effort (logs, increments `geofence_fire_errors`, never blocks
ingestion). The engine's latch already dedups, so each entry fires once; the
relay then **physically latches on** until a manual **reset** (Switch.Set off).

To keep the existing log line, the firer wraps the current behaviour: it logs
(as `LogFirer` did) **and** drives devices. Lives in a new `alarm` package to
avoid an import cycle (`alarm` imports `geofence` for the `AlarmEvent` type and
`Firer` interface; `geofence` does not import `alarm`).

### 4. REST CRUD + actions — `internal/handlers/alarmdevices`

Internal-only (session-auth group, `@Tags alarmdevices,internal`), mirroring
`scandevices`:

```
GET    /api/v1/alarm-devices                 list
POST   /api/v1/alarm-devices                 create
GET    /api/v1/alarm-devices/{id}            get
PATCH  /api/v1/alarm-devices/{id}            update (partial)
DELETE /api/v1/alarm-devices/{id}            soft delete
POST   /api/v1/alarm-devices/{id}/test       test-fire (pulse on→off)
POST   /api/v1/alarm-devices/{id}/reset      manual reset (off)
```

- **test-fire**: pulse the relay (on, wait ~2s, off) so Tim sees the strobe
  without leaving it latched; returns 200 on success, surfaces the driver error
  (e.g. 502) so the UI can show "device unreachable".
- **reset**: Switch.Set off — clears a latched alarm after a real fire.
- Storage layer `internal/storage/alarm_devices.go` mirrors `scan_devices.go`
  (CRUD via `WithOrgTx`), plus `ListAlarmDevicesForScanPoint(ctx, orgID, scanPointID)`
  used by the firer.

### 5. Frontend — `AlarmDevicesScreen` + form/modal

Mirror `ScanDevicesScreen`: types, api client, `useAlarmDevices` /
`useAlarmDeviceMutations` hooks, list table with add/edit/delete, plus a
per-row **Test-fire** button (calls `POST .../test`, toasts success/failure).
Register a new `alarm-devices` tab (owner/admin only, same gate as scan-devices),
add to `VALID_TABS` and the lazy screen map in `App.tsx`, and a `TabNavigation`
entry. Form fields: name, type (select, only `shelly_gen4`), base_url,
switch_id, scan_point_id (select from boundary scan points; optional), is_active.

### 6. Wiring — `serve.go`

The firer is only meaningful when ingestion runs (it's inside the
`mqttCfg.Enabled()` block). Replace:

```go
geofence.NewLogFirer(log)
```
with
```go
alarm.NewFirer(store, shelly.New(shellyTimeout), log)
```

CRUD + test-fire endpoints are always mounted (independent of ingestion).

## Error handling

- Driver: short HTTP timeout; non-2xx or transport error ⇒ wrapped error.
- Auto-fire path: best-effort, logged, never blocks ingestion (engine contract).
- Test-fire endpoint: returns the driver error to the operator (502 on comms
  failure) so the demo can be validated before go-time.
- CRUD: same validation/conflict/404 conventions as `scandevices`.

## Testing (TDD)

- **Driver** (`shelly`): `httptest.Server` asserts the RPC body
  (`Switch.Set`, correct `id`/`on`), timeout behaviour, non-2xx → error.
- **Firer** (`alarm`): fake `deviceLookup` + fake driver; assert it fires only
  active devices bound to the event's scan point, aggregates/logs errors, and
  never returns fatal.
- **Storage**: integration tests (RLS role) for CRUD + `ListAlarmDevicesForScanPoint`,
  mirroring `scan_devices` tests; org-isolation enforced.
- **Handlers**: table tests for CRUD + test/reset (driver faked), incl. 404 and
  validation paths.
- **Frontend**: component/hook tests mirroring scan-devices coverage.
- Full `just validate` (lint + test, both workspaces) before PR.

## Out of scope / follow-ups

- MQTT fire path → TRA-906.
- Multi-replica fan-out (single replica until ingestion scales) → TRA-907 family.
- Demo data + runbook → TRA-904 (blocked by this).
```
