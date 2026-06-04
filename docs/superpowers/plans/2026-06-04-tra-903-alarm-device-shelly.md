# TRA-903 Alarm Device CRUD + Shelly Gen4 Trigger — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Model alarm output devices with internal CRUD + UI, drive a demo Shelly Gen4 relay over local HTTP, and auto-fire it from the geofence engine via the existing `geofence.Firer` seam.

**Architecture:** New `alarm_devices` table (mirrors `scan_devices`). A transport-only `shelly` driver issues `Switch.Set` RPC. An `alarm.Firer` (implements `geofence.Firer`) looks up devices bound to the firing scan point and drives them; wired into `serve.go` in place of `LogFirer`. Internal-only REST CRUD + test-fire/reset actions, plus a React management screen mirroring scan-devices.

**Tech Stack:** Go (pgx, chi, zerolog, validator), TimescaleDB/Postgres (RLS), React/TS (TanStack Query), `just` task runner.

---

## File Structure

Backend:
- `backend/migrations/000014_alarm_devices.{up,down}.sql` — table, enum, RLS, triggers
- `backend/internal/models/alarmdevice/alarmdevice.go` — model + request/response types
- `backend/internal/storage/alarm_devices.go` — CRUD + `ListAlarmDevicesForScanPoint`
- `backend/internal/storage/alarm_devices_test.go` — integration tests
- `backend/internal/alarm/shelly/client.go` — Gen4 RPC driver
- `backend/internal/alarm/shelly/client_test.go` — driver tests (httptest)
- `backend/internal/alarm/firer.go` — `Firer` implementing `geofence.Firer`
- `backend/internal/alarm/firer_test.go` — firer tests (fakes)
- `backend/internal/handlers/alarmdevices/alarmdevices.go` — CRUD + test/reset handlers
- `backend/internal/handlers/alarmdevices/alarmdevices_test.go` — handler tests
- `backend/internal/cmd/serve/serve.go`, `router.go` — wiring

Frontend:
- `frontend/src/types/alarmdevices/index.ts`
- `frontend/src/lib/api/alarmdevices/index.ts`
- `frontend/src/hooks/alarmdevices/{useAlarmDevices,useAlarmDeviceMutations,index}.ts`
- `frontend/src/components/AlarmDevicesScreen.tsx`
- `frontend/src/components/alarmdevices/{AlarmDeviceForm,AlarmDeviceFormModal}.tsx`
- `frontend/src/App.tsx`, `frontend/src/components/TabNavigation.tsx` — tab registration

---

## Task 1: Migration — alarm_devices table

**Files:** Create `backend/migrations/000014_alarm_devices.up.sql`, `000014_alarm_devices.down.sql`

- [ ] **Step 1: Write up migration** (mirrors 000013 header style + scan_devices triggers/RLS)

```sql
-- TRA-903 — alarm OUTPUT devices (demo: Shelly Gen4 relay). Internal-only CRUD.
-- Bound (optionally) to a boundary scan_point; the geofence.Firer (alarm.Firer)
-- drives every active device bound to a firing point. No in-migration GRANTs:
-- infra init-grants Job + integration harness handle them (same as 000013).
SET search_path = trakrf, public;

CREATE TYPE alarm_device_type AS ENUM ('shelly_gen4');

CREATE TABLE alarm_devices (
    id            BIGINT PRIMARY KEY,
    org_id        BIGINT NOT NULL REFERENCES organizations(id),
    name          VARCHAR(255) NOT NULL,
    type          alarm_device_type NOT NULL DEFAULT 'shelly_gen4',
    base_url      VARCHAR(255) NOT NULL,
    switch_id     INT NOT NULL DEFAULT 0,
    scan_point_id BIGINT REFERENCES scan_points(id),
    is_active     BOOLEAN NOT NULL DEFAULT true,
    metadata      JSONB DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX idx_alarm_devices_org        ON alarm_devices(org_id);
CREATE INDEX idx_alarm_devices_scan_point ON alarm_devices(scan_point_id) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_alarm_device_id_trigger
    BEFORE INSERT ON alarm_devices
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_alarm_devices_updated_at
    BEFORE UPDATE ON alarm_devices
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE alarm_devices ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_alarm_devices ON alarm_devices
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE alarm_devices IS 'TRA-903: alarm output devices (Shelly Gen4). Internal-only.';
```

- [ ] **Step 2: Write down migration**

```sql
SET search_path = trakrf, public;
DROP TABLE IF EXISTS alarm_devices;
DROP TYPE IF EXISTS alarm_device_type;
```

- [ ] **Step 3: Apply + verify** — `just backend migrate-up` (or the repo's migrate recipe); confirm table + policy exist, then `migrate-down` once and back up to prove the down is clean. Commit.

---

## Task 2: Model — alarmdevice package

**Files:** Create `backend/internal/models/alarmdevice/alarmdevice.go`

- [ ] **Step 1: Write the types** (mirror `scandevice.go`; pointers for partial update)

```go
package alarmdevice

import "time"

const TypeShellyGen4 = "shelly_gen4"

type AlarmDevice struct {
	ID          int        `json:"id"`
	OrgID       int        `json:"org_id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	BaseURL     string     `json:"base_url"`
	SwitchID    int        `json:"switch_id"`
	ScanPointID *int       `json:"scan_point_id,omitempty"`
	IsActive    bool       `json:"is_active"`
	Metadata    any        `json:"metadata"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

type CreateAlarmDeviceRequest struct {
	Name        string         `json:"name" validate:"required,min=1,max=255"`
	Type        string         `json:"type,omitempty" validate:"omitempty,oneof=shelly_gen4"`
	BaseURL     string         `json:"base_url" validate:"required,url,max=255"`
	SwitchID    *int           `json:"switch_id,omitempty"`
	ScanPointID *int           `json:"scan_point_id,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateAlarmDeviceRequest struct {
	Name        *string         `json:"name,omitempty"`
	Type        *string         `json:"type,omitempty" validate:"omitempty,oneof=shelly_gen4"`
	BaseURL     *string         `json:"base_url,omitempty" validate:"omitempty,url,max=255"`
	SwitchID    *int            `json:"switch_id,omitempty"`
	ScanPointID *int            `json:"scan_point_id,omitempty"`
	IsActive    *bool           `json:"is_active,omitempty"`
	Metadata    *map[string]any `json:"metadata,omitempty"`
}

type AlarmDeviceResponse struct {
	Data AlarmDevice `json:"data"`
}
```

- [ ] **Step 2:** `just backend build` to confirm compiles. Commit.

---

## Task 3: Storage — CRUD + firer lookup (TDD)

**Files:** Create `backend/internal/storage/alarm_devices.go`, `alarm_devices_test.go`

- [ ] **Step 1: Write failing integration tests** mirroring `scan_devices_test.go`: create → get → list → update (partial) → delete (soft); plus `ListAlarmDevicesForScanPoint` returns only active, non-deleted, matching-point devices; plus org-isolation (a second org cannot see the row). Use the existing integration test harness (RLS role, `WithOrgTx`). Look at `scan_devices_test.go` for the exact harness setup (`setupTestStorage`/fixtures).

- [ ] **Step 2: Run, expect FAIL** (undefined methods).

- [ ] **Step 3: Implement storage** mirroring `scan_devices.go`:

```go
const alarmDeviceColumns = `id, org_id, name, type, base_url, switch_id,
	scan_point_id, is_active, metadata, created_at, updated_at, deleted_at`

func scanAlarmDevice(row pgx.Row, d *alarmdevice.AlarmDevice) error {
	return row.Scan(&d.ID, &d.OrgID, &d.Name, &d.Type, &d.BaseURL, &d.SwitchID,
		&d.ScanPointID, &d.IsActive, &d.Metadata, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
}
```

Methods (all via `WithOrgTx`, same shape as scan_devices): `CreateAlarmDevice`, `GetAlarmDeviceByID`, `ListAlarmDevices(orgID,limit,offset)`, `CountAlarmDevices`, `UpdateAlarmDevice` (dynamic SET, `updated_at = NOW()`), `DeleteAlarmDevice` (soft). Create defaults: `type→shelly_gen4`, `switch_id→0`, `is_active→true`, `metadata→{}`. Plus:

```go
// ListAlarmDevicesForScanPoint returns active, non-deleted devices bound to the point.
func (s *Storage) ListAlarmDevicesForScanPoint(ctx context.Context, orgID, scanPointID int) ([]alarmdevice.AlarmDevice, error) {
	query := `SELECT ` + alarmDeviceColumns + `
		FROM trakrf.alarm_devices
		WHERE org_id = $1 AND scan_point_id = $2 AND is_active = true AND deleted_at IS NULL
		ORDER BY id`
	out := []alarmdevice.AlarmDevice{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, scanPointID)
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() {
			var d alarmdevice.AlarmDevice
			if err := scanAlarmDevice(rows, &d); err != nil { return err }
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil { return nil, fmt.Errorf("list alarm devices for scan point: %w", err) }
	return out, nil
}
```

- [ ] **Step 4: Run tests, expect PASS.** Commit.

---

## Task 4: Shelly driver (TDD)

**Files:** Create `backend/internal/alarm/shelly/client.go`, `client_test.go`

- [ ] **Step 1: Write failing test** with `httptest.Server`:
  - asserts request `POST /rpc`, JSON body `{"id":1,"method":"Switch.Set","params":{"id":2,"on":true}}` for `Set(ctx, srv.URL, 2, true)`.
  - non-2xx response → error.
  - server that sleeps past timeout → error (use a tiny timeout).

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement:**

```go
package shelly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct{ http *http.Client }

func New(timeout time.Duration) *Client {
	if timeout <= 0 { timeout = 3 * time.Second }
	return &Client{http: &http.Client{Timeout: timeout}}
}

type rpcReq struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// Set drives one relay channel via the Gen2+ Switch.Set RPC over local HTTP.
func (c *Client) Set(ctx context.Context, baseURL string, switchID int, on bool) error {
	body, _ := json.Marshal(rpcReq{ID: 1, Method: "Switch.Set",
		Params: map[string]any{"id": switchID, "on": on}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/rpc", bytes.NewReader(body))
	if err != nil { return fmt.Errorf("shelly: build request: %w", err) }
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil { return fmt.Errorf("shelly: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("shelly: unexpected status %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 4: Run tests, expect PASS.** Commit.

---

## Task 5: alarm.Firer (TDD)

**Files:** Create `backend/internal/alarm/firer.go`, `firer_test.go`

- [ ] **Step 1: Write failing test** with a fake lookup + fake driver:
  - `Fire(ev)` calls driver `Set(on=true)` once per device returned for `ev.ScanPointID`.
  - device with no bound devices → no driver calls, nil error.
  - driver error on one device → logged, `Fire` still returns nil (best-effort) OR returns aggregated error (engine logs it); choose **return aggregated error** so the engine's `fire_errors` metric increments, but never panics.

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement:**

```go
package alarm

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/geofence"
	"github.com/trakrf/platform/backend/internal/models/alarmdevice"
)

type deviceLookup interface {
	ListAlarmDevicesForScanPoint(ctx context.Context, orgID, scanPointID int) ([]alarmdevice.AlarmDevice, error)
}

type driver interface {
	Set(ctx context.Context, baseURL string, switchID int, on bool) error
}

type Firer struct {
	store  deviceLookup
	drv    driver
	log    zerolog.Logger
}

func NewFirer(store deviceLookup, drv driver, log *zerolog.Logger) Firer {
	return Firer{store: store, drv: drv, log: log.With().Str("component", "alarm").Logger()}
}

func (f Firer) Fire(ctx context.Context, ev geofence.AlarmEvent) error {
	f.log.Warn().Int("org_id", ev.OrgID).Int("asset_id", ev.AssetID).
		Int("scan_point_id", ev.ScanPointID).Str("epc", ev.EPC).Int("rssi", ev.RSSI).
		Time("fired_at", ev.FiredAt).Msg("geofence boundary alarm")

	devices, err := f.store.ListAlarmDevicesForScanPoint(ctx, ev.OrgID, ev.ScanPointID)
	if err != nil { return fmt.Errorf("alarm: lookup devices: %w", err) }

	var errs []error
	for _, d := range devices {
		if err := f.drv.Set(ctx, d.BaseURL, d.SwitchID, true); err != nil {
			f.log.Error().Err(err).Int("alarm_device_id", d.ID).Str("base_url", d.BaseURL).
				Msg("alarm device fire failed (fail-quiet)")
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
```

- [ ] **Step 4: Run, expect PASS.** Commit. Note: `*storage.Storage` satisfies `deviceLookup`; `*shelly.Client` satisfies `driver` — verified at wiring (Task 7).

---

## Task 6: Handlers — CRUD + test-fire + reset (TDD)

**Files:** Create `backend/internal/handlers/alarmdevices/alarmdevices.go`, `alarmdevices_test.go`

- [ ] **Step 1: Write failing handler tests** mirroring `scandevices` handler tests: list/create/get/update/delete happy + 404 + validation; `POST /{id}/test` pulses (driver fake records on then off) and returns 200; driver error → 502; `POST /{id}/reset` calls `Set(off)`. Inject the driver via an interface field so tests fake it.

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** mirroring `scandevices.go`. Handler holds `storage` + a `driver` interface (`Set`). CRUD identical pattern (org-context via `middleware.GetRequestOrgID`, `DecodeJSONStrict`, `validate.Struct`, `WriteJSON`). Actions:

```go
// @Summary Test-fire an alarm device
// @Tags alarmdevices,internal
// @ID alarmdevices.test
// @Router /api/v1/alarm-devices/{id}/test [post]
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r); if err != nil { httputil.RespondMissingOrgContext(w, r, reqID); return }
	id, ok := parseIDParam(r, "alarm_device_id"); if !ok { /* 400 */ }
	d, err := h.storage.GetAlarmDeviceByID(r.Context(), orgID, id)
	if err != nil { /* 500 */ }
	if d == nil { /* 404 */ }
	ctx := r.Context()
	if err := h.driver.Set(ctx, d.BaseURL, d.SwitchID, true); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrUpstream, err.Error(), reqID); return
	}
	time.Sleep(h.testPulse)            // ~2s default; configurable for tests (set 0)
	_ = h.driver.Set(ctx, d.BaseURL, d.SwitchID, false)  // best-effort off
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
// Reset: same shape, single Set(off), 200/502.
```

Route registration:

```go
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/alarm-devices", h.List)
	r.Post("/api/v1/alarm-devices", h.Create)
	r.Get("/api/v1/alarm-devices/{alarm_device_id}", h.Get)
	r.Patch("/api/v1/alarm-devices/{alarm_device_id}", h.Update)
	r.Delete("/api/v1/alarm-devices/{alarm_device_id}", h.Delete)
	r.Post("/api/v1/alarm-devices/{alarm_device_id}/test", h.Test)
	r.Post("/api/v1/alarm-devices/{alarm_device_id}/reset", h.Reset)
}
```

(Check the exact error-code constant for upstream/502 in `modelerrors`; if none, reuse the closest, e.g. `ErrInternal` with a clear message, or add `ErrUpstream`.)

- [ ] **Step 4: Run, expect PASS.** Commit.

---

## Task 7: Wire into serve.go + router.go

**Files:** Modify `backend/internal/cmd/serve/serve.go`, `router.go`

- [ ] **Step 1:** Construct the handler (always) and firer (inside the `mqttCfg.Enabled()` block):

```go
// serve.go (with other handlers ~line 118)
shellyClient := shelly.New(0) // default 3s timeout
alarmDevicesHandler := alarmdeviceshandler.NewHandler(store, shellyClient)
```
```go
// serve.go inside if mqttCfg.Enabled() — replace NewLogFirer:
geofenceEngine := geofence.NewEngine(geofence.ConfigFromEnv(), store,
	alarm.NewFirer(store, shellyClient, log), log)
```

- [ ] **Step 2:** Thread `alarmDevicesHandler` through `setupRouter(...)` and register in the session-auth group next to `scanDevicesHandler.RegisterRoutes(r)`.

- [ ] **Step 3:** `just backend build` then `just backend test`. Commit.

---

## Task 8: OpenAPI spec regen

**Files:** generated specs (do not hand-edit)

- [ ] **Step 1:** Run `just backend api-spec`. Confirm alarm-devices routes appear only in the internal spec (tagged `internal`), not the public spec, mirroring scan-devices. Commit regenerated artifacts.

---

## Task 9: Frontend types + api client + hooks

**Files:** Create the `alarmdevices` type/api/hook files (mirror `scandevices`).

- [ ] **Step 1:** `types/alarmdevices/index.ts` — `AlarmDevice`, `Create/UpdateAlarmDeviceRequest`, list/response types matching the Go JSON.
- [ ] **Step 2:** `lib/api/alarmdevices/index.ts` — `alarmDevicesApi` with `list/get/create/update/delete` + `test(id)` (`POST /alarm-devices/${id}/test`) + `reset(id)`.
- [ ] **Step 3:** `hooks/alarmdevices/useAlarmDevices.ts` + `useAlarmDeviceMutations.ts` (+ barrel `index.ts`) mirroring scan-devices, with a `test`/`reset` mutation.
- [ ] **Step 4:** `pnpm` typecheck (`just frontend lint` / build). Commit.

---

## Task 10: Frontend screen + form + tab

**Files:** Create `AlarmDevicesScreen.tsx`, `alarmdevices/AlarmDeviceForm.tsx`, `AlarmDeviceFormModal.tsx`; modify `App.tsx`, `TabNavigation.tsx`.

- [ ] **Step 1:** Build the screen mirroring `ScanDevicesScreen.tsx`: list table (name, type, base_url, bound scan point, active), add/edit/delete modals, **per-row Test-fire button** → `test` mutation → toast success / "device unreachable" on failure. Optional reset button.
- [ ] **Step 2:** Form fields: name, type (select: shelly_gen4), base_url, switch_id (number), scan_point_id (optional select of boundary scan points — reuse `useScanPoints`/lookup), is_active.
- [ ] **Step 3:** Register `alarm-devices` in `App.tsx` `VALID_TABS`, lazy screen map, loading map; add `TabNavigation` entry gated to owner/admin (same as scan-devices).
- [ ] **Step 4:** `just frontend test` + build. Commit.

---

## Task 11: Validate + ship

- [ ] **Step 1:** `just validate` (lint + test, both workspaces). All green.
- [ ] **Step 2:** Manual driver sanity is bench-only (Shelly at 192.168.50.66) — not in CI; note in PR that test-fire was validated against the bench relay if/when available, else flagged for Tim.
- [ ] **Step 3:** Push branch, open PR with summary + spec/plan links. **HOLD for Mike's diff review before merge** (no auto-merge).

---

## Self-Review

- **Spec coverage:** CRUD (Tasks 2,3,6,9,10) ✓; migration (1) ✓; Shelly driver (4) ✓; latch+manual-reset → engine latch dedups + `reset` endpoint (6) + relay physically latches ✓; fail-quiet (4,5) ✓; test-fire button (6,10) ✓; wire to engine fire action (5,7) ✓; binding to boundary scan_point (1,3,5) ✓; MQTT fire out-of-scope ✓.
- **Placeholders:** none — all steps carry concrete code; the only deferred lookups are exact existing helper names (`modelerrors.ErrUpstream`, integration harness setup) flagged to confirm against the scan-devices sibling at build time.
- **Type consistency:** `ListAlarmDevicesForScanPoint`, `Set(ctx,baseURL,switchID,on)`, `Fire(ctx,ev)`, `AlarmDevice` fields consistent across Tasks 3/4/5/6/7.
```
