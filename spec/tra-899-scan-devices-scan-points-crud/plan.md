# TRA-899 scan_devices / scan_points CRUD — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a device-type-aware scan_devices/scan_points data model (enum type + transport + publish_topic + boundary), internal session-auth REST CRUD, and a management UI; rename the scan tables' `identifier → external_key`; and convert `process_tag_scans` to resolve against the CRUD registry instead of auto-creating devices.

**Architecture:** All schema change is a single new increment migration `000011` (up+down) layered on the frozen `000001–000010` foundation — `ALTER TABLE`/`CREATE TYPE`/`CREATE OR REPLACE FUNCTION`. Backend follows the established `models/ → storage/ (WithOrgTx) → handlers/ (chi) → router` layering; CRUD is internal-only (session JWT, `middleware.Auth` subtree), not the public API-key surface. Frontend follows the locations feature shape (axios module → TanStack-Query hooks → screen + form modal). The DB function stays the CS463 parser.

**Tech Stack:** Go, pgx/v5, golang-migrate, chi, go-playground/validator, swaggo; React 18 + TS + Vite, TanStack Query, Tailwind, Vitest/Playwright; Just task runner.

**Conventions (do not violate):**
- NEVER edit `000005` or `000010`. All DB change in `000011`.
- Every storage DB call wrapped in `s.WithOrgTx(ctx, orgID, …)`. New storage files MUST be added to `check-rls-guard` in `backend/justfile` or `just backend lint` fails by design.
- Run commands from repo root via `just backend <cmd>` / `just frontend <cmd>`.
- Conventional commits; commit after each task. Branch: `feat/tra-899-scan-devices-scan-points-crud` (already checked out in worktree).
- Integration tests carry `//go:build integration`; run with `just backend test-integration`.

---

## File Structure

**Database**
- Create: `backend/migrations/000011_scan_device_model.up.sql`
- Create: `backend/migrations/000011_scan_device_model.down.sql`
- Modify: `backend/database/cutover/05_scan_devices_and_points.sql` (rename sweep)
- Modify: `backend/database/test/expected_diff_allowlist.txt` (new diff category)

**Backend models** (mirror `internal/models/location/location.go` shape, simpler — no hierarchy)
- Create: `backend/internal/models/scandevice/scandevice.go`
- Create: `backend/internal/models/scanpoint/scanpoint.go`

**Backend storage** (mirror `internal/storage/locations.go` CRUD core)
- Create: `backend/internal/storage/scan_devices.go`
- Create: `backend/internal/storage/scan_points.go`
- Create: `backend/internal/storage/scan_devices_integration_test.go`
- Create: `backend/internal/storage/scan_points_integration_test.go`
- Create: `backend/internal/storage/process_tag_scans_integration_test.go`
- Modify: `backend/justfile` (`check-rls-guard` RLS_FILES list)

**Backend handlers** (internal, session-auth; mirror `internal/handlers/locations/locations.go` but simpler decode)
- Create: `backend/internal/handlers/scandevices/scandevices.go`
- Create: `backend/internal/handlers/scanpoints/scanpoints.go`
- Modify: `backend/internal/cmd/serve/router.go` (register in the `middleware.Auth` group, ~line 138)

**Fixtures**
- Create: `backend/internal/testutil/fixtures/cs463_read.json`
- Create: `backend/internal/testutil/fixtures/gls10_read.json`

**Frontend** (mirror the locations feature)
- Create: `frontend/src/types/scandevices/index.ts`
- Create: `frontend/src/lib/api/scandevices/index.ts`
- Create: `frontend/src/hooks/scandevices/useScanDevices.ts`
- Create: `frontend/src/hooks/scandevices/useScanDeviceMutations.ts`
- Create: `frontend/src/components/ScanDevicesScreen.tsx`
- Create: `frontend/src/components/scandevices/ScanDeviceFormModal.tsx`
- Create: `frontend/src/components/scandevices/ScanDeviceForm.tsx`
- Create: `frontend/src/components/scandevices/ScanPointsPanel.tsx`
- Create: `frontend/src/components/scandevices/ScanPointForm.tsx`
- Create: `frontend/src/components/scandevices/ScanDeviceFormModal.test.tsx`
- Modify: `frontend/src/App.tsx` (VALID_TABS, lazy import, tabComponents, loadingScreens)
- Modify: `frontend/src/components/TabNavigation.tsx` (nav entry)

---

## Route shape (settled)

Devices (top-level), scan_points nested under their device:
- `GET    /api/v1/scan-devices`                                  list (paginated)
- `POST   /api/v1/scan-devices`                                  create
- `GET    /api/v1/scan-devices/{scan_device_id}`                 get
- `PATCH  /api/v1/scan-devices/{scan_device_id}`                 update (merge-patch)
- `DELETE /api/v1/scan-devices/{scan_device_id}`                 delete (soft, cascades to its points)
- `GET    /api/v1/scan-devices/{scan_device_id}/scan-points`     list points for device
- `POST   /api/v1/scan-devices/{scan_device_id}/scan-points`     create point under device
- `GET    /api/v1/scan-points/{scan_point_id}`                   get
- `PATCH  /api/v1/scan-points/{scan_point_id}`                   update (merge-patch)
- `DELETE /api/v1/scan-points/{scan_point_id}`                   delete (soft)

All registered in the `middleware.Auth` (session-only) group. No `RequireScope`, no `,public` swagger tag.

---

# PHASE 1 — Database

### Task 1: Migration `000011` up — enums, rename, new columns

**Files:**
- Create: `backend/migrations/000011_scan_device_model.up.sql`

- [ ] **Step 1: Write the up migration (schema portion)**

```sql
-- TRA-899 — evolve scan_devices/scan_points on top of the frozen 000005 baseline.
--   * rename identifier -> external_key (sweep miss; assets/locations were renamed in TRA-475/554)
--   * bound `type` and add `transport` with PG enums
--   * add publish_topic (MQTT read channel / routing key, TRA-900)
--   * add scan_points.is_boundary (geofence boundary marker, TRA-901)
--   * process_tag_scans: external_key refs + drop scan_device/scan_point/location auto-create (Task 3)
SET search_path = trakrf, public;

-- ---- enums -----------------------------------------------------------------
CREATE TYPE scan_device_type AS ENUM ('csl_cs463', 'gl_s10', 'esp32_ble_generic', 'csl_cs108');
CREATE TYPE scan_transport   AS ENUM ('mqtt', 'web_ble');

-- ---- scan_devices ----------------------------------------------------------
-- rename natural key column + dependent objects
ALTER TABLE scan_devices RENAME COLUMN identifier TO external_key;
ALTER INDEX  idx_scan_devices_identifier RENAME TO idx_scan_devices_external_key;
-- the inline UNIQUE(org_id, identifier, valid_from) constraint was auto-named
-- scan_devices_org_id_identifier_valid_from_key; rename to match.
ALTER TABLE scan_devices
    RENAME CONSTRAINT scan_devices_org_id_identifier_valid_from_key
    TO scan_devices_org_id_external_key_valid_from_key;

-- map legacy free-string type values into the enum domain, then convert.
-- 'rfid_reader' was the only value process_tag_scans ever auto-inserted.
UPDATE scan_devices SET type = 'csl_cs463' WHERE type = 'rfid_reader';
UPDATE scan_devices SET type = 'csl_cs463' WHERE type NOT IN
    ('csl_cs463','gl_s10','esp32_ble_generic','csl_cs108');
ALTER TABLE scan_devices
    ALTER COLUMN type TYPE scan_device_type USING type::scan_device_type;

ALTER TABLE scan_devices
    ADD COLUMN transport     scan_transport NOT NULL DEFAULT 'mqtt',
    ADD COLUMN publish_topic  VARCHAR(255);

-- publish_topic is a routing key: unique per org among live rows, plus a
-- plain lookup index for the TRA-900 subscriber's topic->device resolution.
CREATE UNIQUE INDEX idx_scan_devices_publish_topic_unique
    ON scan_devices (org_id, publish_topic)
    WHERE publish_topic IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_scan_devices_publish_topic ON scan_devices (publish_topic);

COMMENT ON COLUMN scan_devices.external_key IS
    'Natural key / device self-reported identity (e.g., cs463-214 rfidReaderName, or GL-S10 dev_ble_mac). Appears in the MQTT topic and payload.';
COMMENT ON COLUMN scan_devices.publish_topic IS
    'Read channel the device publishes on (routing key, TRA-900). Defaults at the app layer to trakrf.id/{external_key}/reads when unset.';

-- ---- scan_points -----------------------------------------------------------
ALTER TABLE scan_points RENAME COLUMN identifier TO external_key;
ALTER INDEX  idx_scan_points_identifier RENAME TO idx_scan_points_external_key;
ALTER TABLE scan_points
    RENAME CONSTRAINT scan_points_org_id_identifier_valid_from_key
    TO scan_points_org_id_external_key_valid_from_key;

ALTER TABLE scan_points
    ADD COLUMN is_boundary BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN scan_points.external_key IS
    'Natural key / capture-point identity (e.g., cs463-214-1 capturePointName: reader + antenna port).';
COMMENT ON COLUMN scan_points.is_boundary IS
    'Marks this capture point as a geofence boundary (TRA-901). The associated zone is scan_points.location_id.';
```

- [ ] **Step 2: Verify constraint names against the live schema first**

Before trusting the auto-generated constraint names above, confirm them. Run:
`just backend migrate` against a scratch DB through 000010, then:
`psql "$PG_URL" -c "\d trakrf.scan_devices" -c "\d trakrf.scan_points"`
Expected: a UNIQUE constraint named `scan_devices_org_id_identifier_valid_from_key` (and the points equivalent). If PG truncated/named them differently, fix the `RENAME CONSTRAINT` lines to the actual names. (Postgres derives `<table>_<col>_<col>_<col>_key`; 60-char identifier truncation is possible — verify.)

- [ ] **Step 3: Commit (after Task 3 appends the function block to the same file)**

(Do not commit yet — Task 3 adds the `CREATE OR REPLACE FUNCTION` to this same up file. Commit at the end of Task 3.)

---

### Task 2: Migration `000011` down

**Files:**
- Create: `backend/migrations/000011_scan_device_model.down.sql`

- [ ] **Step 1: Write the down migration (schema portion)**

```sql
SET search_path = trakrf, public;

-- scan_points
ALTER TABLE scan_points DROP COLUMN is_boundary;
ALTER TABLE scan_points
    RENAME CONSTRAINT scan_points_org_id_external_key_valid_from_key
    TO scan_points_org_id_identifier_valid_from_key;
ALTER INDEX idx_scan_points_external_key RENAME TO idx_scan_points_identifier;
ALTER TABLE scan_points RENAME COLUMN external_key TO identifier;

-- scan_devices
DROP INDEX IF EXISTS idx_scan_devices_publish_topic;
DROP INDEX IF EXISTS idx_scan_devices_publish_topic_unique;
ALTER TABLE scan_devices DROP COLUMN publish_topic;
ALTER TABLE scan_devices DROP COLUMN transport;
ALTER TABLE scan_devices ALTER COLUMN type TYPE VARCHAR(50) USING type::text;
ALTER TABLE scan_devices
    RENAME CONSTRAINT scan_devices_org_id_external_key_valid_from_key
    TO scan_devices_org_id_identifier_valid_from_key;
ALTER INDEX idx_scan_devices_external_key RENAME TO idx_scan_devices_identifier;
ALTER TABLE scan_devices RENAME COLUMN external_key TO identifier;

DROP TYPE IF EXISTS scan_transport;
DROP TYPE IF EXISTS scan_device_type;

-- process_tag_scans restored to its 000010 form (Task 3 appends this).
```

- [ ] **Step 2: Commit** — deferred to end of Task 3.

---

### Task 3: `process_tag_scans` rewrite (drop device/point/location auto-create)

**Files:**
- Modify (append): `backend/migrations/000011_scan_device_model.up.sql`
- Modify (append): `backend/migrations/000011_scan_device_model.down.sql`

- [ ] **Step 1: Append the new function to the up migration**

Append to `000011_...up.sql`. This is the 000010 body with: (a) `identifier` → `external_key` on scan tables, (b) the locations / scan_devices / scan_points auto-create INSERT blocks removed, (c) asset + tag auto-create retained, (d) asset_scans insert now resolves only against CRUD-registered scan_points.

```sql
-- ---- process_tag_scans: registry-driven (no device/point/location auto-create) ----
CREATE OR REPLACE FUNCTION trakrf.process_tag_scans() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    topic_org_id BIGINT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %', NEW.message_topic;
        RETURN NEW;
    END IF;

    -- Assets + tags are still auto-registered from EPCs (asset membership is
    -- TRA-901's concern; left intact for this ticket). Devices, scan_points,
    -- and the locations that backed auto scan_points are NO LONGER auto-created
    -- (TRA-899): they are CRUD-managed. Reads from unregistered devices/points
    -- resolve to nothing below and produce no asset_scans.
    INSERT INTO assets (org_id, external_key, name)
    SELECT DISTINCT topic_org_id, t.tag ->> 'epc', t.tag ->> 'epc' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT 1 FROM assets a WHERE a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc')
      AND NOT EXISTS (SELECT 1 FROM tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO tags (org_id, asset_id, type, value)
    SELECT DISTINCT topic_org_id, a.id, 'rfid', t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    WHERE NOT EXISTS (SELECT 1 FROM tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0),
        topic_org_id, a.id, sp.location_id, sp.id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN scan_points sp ON sp.org_id = topic_org_id AND sp.external_key = t.tag ->> 'capturePointName'
    JOIN assets a       ON a.org_id  = topic_org_id AND a.external_key = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing tag_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;
```

- [ ] **Step 2: Append the 000010-original function to the down migration**

Append the exact body of `process_tag_scans` from `backend/migrations/000010_stored_procedures.up.sql:24-118` (verbatim `CREATE OR REPLACE FUNCTION … $$;`) to `000011_...down.sql`, but with `external_key` references changed back to `identifier` on the scan tables — i.e., the original 000010 text references `identifier` already, so paste it verbatim. (Copy lines 24-118 of 000010 exactly.)

- [ ] **Step 3: Commit migration + down + function**

```bash
git add backend/migrations/000011_scan_device_model.up.sql backend/migrations/000011_scan_device_model.down.sql
git commit -m "feat(db): TRA-899 000011 — scan device enums, external_key rename, publish_topic, is_boundary, registry-driven process_tag_scans"
```

---

### Task 4: Rename sweep — cutover SQL + diff allowlist

**Files:**
- Modify: `backend/database/cutover/05_scan_devices_and_points.sql`
- Modify: `backend/database/test/expected_diff_allowlist.txt`

- [ ] **Step 1: Update the cutover pull to target `external_key`**

In `05_scan_devices_and_points.sql`: the FDW *source* (`cloud_src.scan_devices`) still has the old `identifier` column, so source reads stay `s.identifier`; only the *target* column list and target joins change.
- scan_devices INSERT column list `(org_id, identifier, name, type, …)` → `(org_id, external_key, name, type, …)`.
- scan_points INSERT column list `(org_id, scan_device_id, location_id, identifier, …)` → `(… external_key …)`.
- The device self-join `t_dev.identifier = src_dev.identifier` → `t_dev.external_key = src_dev.identifier`.
- Update the header comment lines describing the natural key.
- (Note: `t_org.identifier`/`src_org.identifier` are ORGANIZATIONS columns — NOT renamed. Leave them.)

- [ ] **Step 2: Add a diff-allowlist category**

Append to `expected_diff_allowlist.txt` a new numbered category (e.g. `21.`) describing the TRA-899 deltas as intentional: scan_devices/scan_points `identifier → external_key` (column + index `idx_scan_*_external_key` + UNIQUE constraint rename); `scan_device_type`/`scan_transport` enums added and `scan_devices.type` retyped; `transport`/`publish_topic`/`is_boundary` columns + publish_topic indexes added; `process_tag_scans` body change (drops scan_device/scan_point/location auto-create blocks).

- [ ] **Step 3: Commit**

```bash
git add backend/database/cutover/05_scan_devices_and_points.sql backend/database/test/expected_diff_allowlist.txt
git commit -m "chore(db): TRA-899 rename sweep — cutover pull + schema-diff allowlist"
```

---

### Task 5: Apply + verify migration end-to-end

- [ ] **Step 1: Migrate a clean DB**

Run: `just backend migrate`
Expected: applies 000001 → 000011 with no error; final "Migrations complete".

- [ ] **Step 2: Verify schema shape**

Run: `psql "$PG_URL" -c "\d trakrf.scan_devices" -c "\d trakrf.scan_points"`
Expected: `external_key` columns (no `identifier`); `type` is `scan_device_type`; `transport scan_transport not null default 'mqtt'`; `publish_topic` + its two indexes; `scan_points.is_boundary boolean not null default false`.

- [ ] **Step 3: Verify down reverses (scratch DB only)**

Run a forced down of just 000011 on a scratch DB (e.g. via the migrate CLI's down-to-version if exposed, or `migrate -path … -database … down 1`). Expected: returns to the 000010 shape (`identifier`, no enums). Do NOT run down against shared preview/prod.

- [ ] **Step 4: Commit** — nothing to commit (verification only).

---

# PHASE 2 — Backend models + storage

### Task 6: `scandevice` model package

**Files:**
- Create: `backend/internal/models/scandevice/scandevice.go`

- [ ] **Step 1: Write the model** (mirror `models/location/location.go`; no hierarchy; enum-validated fields)

```go
package scandevice

import "time"

// DeviceType / Transport mirror the PG enums scan_device_type / scan_transport.
const (
	DeviceTypeCS463       = "csl_cs463"
	DeviceTypeGLS10       = "gl_s10"
	DeviceTypeESP32BLE    = "esp32_ble_generic"
	DeviceTypeCS108       = "csl_cs108"
	TransportMQTT         = "mqtt"
	TransportWebBLE       = "web_ble"
)

type ScanDevice struct {
	ID           int            `json:"id"`
	OrgID        int            `json:"org_id"`
	ExternalKey  string         `json:"external_key"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Transport    string         `json:"transport"`
	PublishTopic *string        `json:"publish_topic,omitempty"`
	SerialNumber *string        `json:"serial_number,omitempty"`
	Model        *string        `json:"model,omitempty"`
	Description  string         `json:"description"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	ValidFrom    time.Time      `json:"valid_from"`
	ValidTo      *time.Time     `json:"valid_to,omitempty"`
	IsActive     bool           `json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    *time.Time     `json:"updated_at,omitempty"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty"`
}

type CreateScanDeviceRequest struct {
	ExternalKey  string         `json:"external_key" validate:"required,min=1,max=255"`
	Name         string         `json:"name" validate:"required,min=1,max=255"`
	Type         string         `json:"type" validate:"required,oneof=csl_cs463 gl_s10 esp32_ble_generic csl_cs108"`
	Transport    string         `json:"transport" validate:"omitempty,oneof=mqtt web_ble"`
	PublishTopic *string        `json:"publish_topic,omitempty" validate:"omitempty,min=1,max=255"`
	SerialNumber *string        `json:"serial_number,omitempty" validate:"omitempty,max=255"`
	Model        *string        `json:"model,omitempty" validate:"omitempty,max=100"`
	Description  *string        `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	IsActive     *bool          `json:"is_active,omitempty"`
}

type UpdateScanDeviceRequest struct {
	Name         *string        `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Type         *string        `json:"type,omitempty" validate:"omitempty,oneof=csl_cs463 gl_s10 esp32_ble_generic csl_cs108"`
	Transport    *string        `json:"transport,omitempty" validate:"omitempty,oneof=mqtt web_ble"`
	PublishTopic *string        `json:"publish_topic,omitempty" validate:"omitempty,min=1,max=255"`
	SerialNumber *string        `json:"serial_number,omitempty" validate:"omitempty,max=255"`
	Model        *string        `json:"model,omitempty" validate:"omitempty,max=100"`
	Description  *string        `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	IsActive     *bool          `json:"is_active,omitempty"`
}

// DefaultPublishTopic is the read-channel convention applied when publish_topic
// is omitted on create.
func DefaultPublishTopic(externalKey string) string {
	return "trakrf.id/" + externalKey + "/reads"
}
```

- [ ] **Step 2: Build** — Run: `just backend build` (or `go build ./...`). Expected: compiles.
- [ ] **Step 3: Commit** — `git commit -am "feat(models): TRA-899 scandevice model"`

---

### Task 7: `scanpoint` model package

**Files:**
- Create: `backend/internal/models/scanpoint/scanpoint.go`

- [ ] **Step 1: Write the model**

```go
package scanpoint

import "time"

type ScanPoint struct {
	ID           int            `json:"id"`
	OrgID        int            `json:"org_id"`
	ScanDeviceID int            `json:"scan_device_id"`
	LocationID   *int           `json:"location_id,omitempty"`
	ExternalKey  string         `json:"external_key"`
	Name         string         `json:"name"`
	AntennaPort  *int           `json:"antenna_port,omitempty"`
	IsBoundary   bool           `json:"is_boundary"`
	Description  string         `json:"description"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	ValidFrom    time.Time      `json:"valid_from"`
	ValidTo      *time.Time     `json:"valid_to,omitempty"`
	IsActive     bool           `json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    *time.Time     `json:"updated_at,omitempty"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty"`
}

type CreateScanPointRequest struct {
	ExternalKey string         `json:"external_key" validate:"required,min=1,max=255"`
	Name        string         `json:"name" validate:"required,min=1,max=255"`
	LocationID  *int           `json:"location_id,omitempty" validate:"omitempty,min=1"`
	AntennaPort *int           `json:"antenna_port,omitempty" validate:"omitempty,min=0"`
	IsBoundary  *bool          `json:"is_boundary,omitempty"`
	Description *string        `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
}

type UpdateScanPointRequest struct {
	Name        *string        `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	LocationID  *int           `json:"location_id,omitempty" validate:"omitempty,min=1"`
	AntennaPort *int           `json:"antenna_port,omitempty" validate:"omitempty,min=0"`
	IsBoundary  *bool          `json:"is_boundary,omitempty"`
	Description *string        `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	// ClearLocationID is set by the PATCH handler on explicit null to detach the zone.
	ClearLocationID bool `json:"-"`
}
```

- [ ] **Step 2: Build** — `just backend build`. Expected: compiles.
- [ ] **Step 3: Commit** — `git commit -am "feat(models): TRA-899 scanpoint model"`

---

### Task 8: storage `scan_devices.go` (TDD) + RLS guard

**Files:**
- Create: `backend/internal/storage/scan_devices.go`
- Create: `backend/internal/storage/scan_devices_integration_test.go`
- Modify: `backend/justfile` (`check-rls-guard` RLS_FILES)

- [ ] **Step 1: Write the failing integration test**

Create `scan_devices_integration_test.go` (`//go:build integration`). Mirror existing storage integration tests (see `internal/storage/*_integration_test.go` and `testutil.SetupTestDatabase`/`CreateTestAccount`). Cover:
- `CreateScanDevice` returns a row with server-assigned `id`, `publish_topic` defaulted to `trakrf.id/{external_key}/reads` when omitted, `transport` defaulted to `mqtt`.
- `GetScanDeviceByID` round-trips; returns `(nil,nil)` for a missing id.
- `ListScanDevices` paginates and is org-scoped.
- `UpdateScanDevice` patches name + sets explicit publish_topic.
- `DeleteScanDevice` soft-deletes (row no longer in List; `deleted_at` set).
- Cross-org isolation: a device created under org A is invisible to a List/Get under org B.

```go
//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestScanDevice_CRUD(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	created, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		ExternalKey: "cs463-214", Name: "Dock Reader", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, "mqtt", created.Transport)
	require.NotNil(t, created.PublishTopic)
	require.Equal(t, "trakrf.id/cs463-214/reads", *created.PublishTopic)

	got, err := db.Store.GetScanDeviceByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Dock Reader", got.Name)

	list, err := db.Store.ListScanDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)

	ok, err := db.Store.DeleteScanDevice(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.True(t, ok)
	list, err = db.Store.ListScanDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Empty(t, list)
}
```

(Confirm the exact `testutil` constructors — `SetupTestDBFull`, `db.Store`, `db.AdminPool`, `CreateTestAccount` — against `internal/storage/rls_role_integration_test.go` and adapt names if they differ.)

- [ ] **Step 2: Run it, expect failure** — `just backend test-integration ./internal/storage/ -run TestScanDevice_CRUD`. Expected: compile error / undefined `CreateScanDevice`.

- [ ] **Step 3: Implement `scan_devices.go`** (mirror `locations.go` Create/Get/List/Update/Delete; every call in `WithOrgTx`; COALESCE nullable text; default publish_topic at app layer)

```go
package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
)

func (s *Storage) CreateScanDevice(ctx context.Context, orgID int, req scandevice.CreateScanDeviceRequest) (*scandevice.ScanDevice, error) {
	transport := req.Transport
	if transport == "" {
		transport = scandevice.TransportMQTT
	}
	var publishTopic *string
	if req.PublishTopic != nil {
		publishTopic = req.PublishTopic
	} else {
		dt := scandevice.DefaultPublishTopic(req.ExternalKey)
		publishTopic = &dt
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	query := `
		INSERT INTO trakrf.scan_devices
		(org_id, external_key, name, type, transport, publish_topic, serial_number, model, description, metadata, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, COALESCE($10,'{}'::jsonb), $11)
		RETURNING id, org_id, external_key, name, type, transport, publish_topic,
		          serial_number, model, COALESCE(description,''), metadata,
		          valid_from, valid_to, is_active, created_at, updated_at, deleted_at`
	var d scandevice.ScanDevice
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, req.ExternalKey, req.Name, req.Type, transport,
			publishTopic, req.SerialNumber, req.Model, req.Description, req.Metadata, isActive,
		).Scan(&d.ID, &d.OrgID, &d.ExternalKey, &d.Name, &d.Type, &d.Transport, &d.PublishTopic,
			&d.SerialNumber, &d.Model, &d.Description, &d.Metadata,
			&d.ValidFrom, &d.ValidTo, &d.IsActive, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("scan device with external_key %s already exists", req.ExternalKey)
		}
		return nil, fmt.Errorf("failed to create scan device: %w", err)
	}
	return &d, nil
}
```

Implement `GetScanDeviceByID`, `ListScanDevices(ctx, orgID, limit, offset)`, `CountScanDevices`, `UpdateScanDevice` (dynamic SET clause like `UpdateLocation`, always append `updated_at = NOW()`), and `DeleteScanDevice` (soft-delete `SET deleted_at = NOW() WHERE id=$1 AND org_id=$2 AND deleted_at IS NULL`, and cascade soft-delete its scan_points in the same `WithOrgTx`). Use the same SELECT column list everywhere. Return `(nil, nil)` on `pgx.ErrNoRows`.

- [ ] **Step 4: Add storage files to the RLS guard**

In `backend/justfile`, append `internal/storage/scan_devices.go internal/storage/scan_points.go` to the `RLS_FILES` variable in `check-rls-guard`.

- [ ] **Step 5: Run test, expect pass** — `just backend test-integration ./internal/storage/ -run TestScanDevice_CRUD`. Expected: PASS.
- [ ] **Step 6: Lint (RLS guard clean)** — `just backend lint`. Expected: `✓ check-rls-guard: clean`.
- [ ] **Step 7: Commit** — `git commit -am "feat(storage): TRA-899 scan_devices CRUD + RLS guard"`

---

### Task 9: storage `scan_points.go` (TDD)

**Files:**
- Create: `backend/internal/storage/scan_points.go`
- Create: `backend/internal/storage/scan_points_integration_test.go`

- [ ] **Step 1: Failing test** — mirror Task 8: create a device, then `CreateScanPoint(ctx, orgID, deviceID, req)`, `GetScanPointByID`, `ListScanPointsByDevice(ctx, orgID, deviceID)`, `UpdateScanPoint` (toggle `is_boundary`, set `location_id`), `DeleteScanPoint` (soft). Assert org-scoping and that `is_boundary` defaults false.
- [ ] **Step 2: Run, expect fail.**
- [ ] **Step 3: Implement `scan_points.go`** — same `WithOrgTx` pattern. `CreateScanPoint` takes the parent `scanDeviceID` explicitly and inserts it; validates the device exists & belongs to org (FK + RLS already enforce, but return a clean error on FK violation). `is_boundary` defaults false when nil. `UpdateScanPoint` honors `ClearLocationID` (sets `location_id = NULL`).
- [ ] **Step 4: Run, expect pass.**
- [ ] **Step 5: Lint** — `just backend lint` (RLS guard still clean).
- [ ] **Step 6: Commit** — `git commit -am "feat(storage): TRA-899 scan_points CRUD"`

---

### Task 10: `process_tag_scans` behavior test + fixtures

**Files:**
- Create: `backend/internal/testutil/fixtures/cs463_read.json`
- Create: `backend/internal/testutil/fixtures/gls10_read.json`
- Create: `backend/internal/storage/process_tag_scans_integration_test.go`

- [ ] **Step 1: Write the CS463 fixture** (from the documented payload shape; validate against a real broker capture later)

```json
{
  "rfidReaderName": "cs463-214",
  "pcEthernetMACAddress": "",
  "tags": [
    {
      "epc": "712AC12F00000000000000A1",
      "antennaPort": 1,
      "capturePointName": "cs463-214-1",
      "rssi": "-52",
      "timeStampOfRead": "1717500000000000"
    }
  ]
}
```

Also create `gls10_read.json` with the GL-S10 shape (`dev_ble_mac`, no antenna) for schema coverage — parser deferred, fixture captured per ticket.

- [ ] **Step 2: Write the behavior test** (`//go:build integration`)

Two cases, both insert into `trakrf.tag_scans (message_topic, message_data)` via the admin pool with topic `"<org_identifier>/cs463-214/reads"` and the fixture as `message_data`, then assert on `asset_scans`:
- **Registered path:** first CRUD-create the scan_device `cs463-214` and scan_point `cs463-214-1` (with a `location_id`). Insert the tag_scan. Assert exactly one `asset_scans` row for the org, with `scan_point_id` = the registered point.
- **Unregistered path:** fresh org, no device/point registered. Insert the tag_scan. Assert: zero `scan_devices`, zero `scan_points`, zero `asset_scans` created (auto-create removed). The asset/tag auto-create still fires (assert the asset row exists) — documents the retained behavior.

(`process_tag_scans` swallows errors via its EXCEPTION block; assert on resulting table state, not on insert errors.)

- [ ] **Step 3: Run, expect pass** — `just backend test-integration ./internal/storage/ -run TestProcessTagScans`. Expected: PASS (proves the 000011 function change).
- [ ] **Step 4: Commit** — `git commit -am "test(db): TRA-899 process_tag_scans registry-driven behavior + CS463/GL-S10 fixtures"`

---

# PHASE 3 — Backend handlers + routing + OpenAPI

### Task 11: `scandevices` handler

**Files:**
- Create: `backend/internal/handlers/scandevices/scandevices.go`

- [ ] **Step 1: Implement the handler** (mirror `handlers/locations/locations.go` structure, but internal — standard decode + `validate.Struct`, no public RejectFields/echo machinery). Methods: `List`, `Create`, `Get`, `Update`, `Delete`, plus nested `ListPoints`, `CreatePoint` (delegating to scan_points storage). Read org via `middleware.GetRequestOrgID(r)`; parse `{scan_device_id}` via `chi.URLParam`. JSON responses wrap data as `{"data": …}` and lists as `{"data": […], "pagination": …}` to match existing handlers. On create return 201 + `Location: /api/v1/scan-devices/{id}`.

Swaggo annotations on each method tagged **`scandevices`** only (NO `,public`) so they land in the internal spec. Example:

```go
// @Summary  Create a scan device
// @Tags     scandevices
// @ID       scandevices.create
// @Accept   json
// @Produce  json
// @Param    request body scandevice.CreateScanDeviceRequest true "Scan device"
// @Success  201 {object} scandevice.ScanDevice
// @Router   /api/v1/scan-devices [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) { /* … */ }
```

`type Handler struct { storage *storage.Storage }` + `func NewHandler(s *storage.Storage) *Handler`. Add a `RegisterRoutes(r chi.Router)` that wires the device routes + the two nested point routes.

- [ ] **Step 2: Build** — `just backend build`. Expected: compiles.
- [ ] **Step 3: Commit** — `git commit -am "feat(handlers): TRA-899 scan_devices internal CRUD handler"`

---

### Task 12: `scanpoints` handler

**Files:**
- Create: `backend/internal/handlers/scanpoints/scanpoints.go`

- [ ] **Step 1: Implement** the by-id handlers `Get`, `Update`, `Delete` for `/api/v1/scan-points/{scan_point_id}` (the device-nested list/create live in the scandevices handler). On PATCH, detect explicit JSON `null` for `location_id` to set `ClearLocationID`. Same struct/NewHandler/RegisterRoutes shape. Swaggo `@Tags scanpoints` (no public).
- [ ] **Step 2: Build** — `just backend build`.
- [ ] **Step 3: Commit** — `git commit -am "feat(handlers): TRA-899 scan_points by-id handler"`

---

### Task 13: Wire routes into the session-auth subtree

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`

- [ ] **Step 1: Add imports + handler construction + registration**

In the `middleware.Auth` group (the block at ~line 130-148 where `orgsHandler.RegisterRoutes`, `usersHandler.RegisterRoutes`, etc. are called), construct and register the new handlers:

```go
scanDevicesHandler := scandeviceshandler.NewHandler(store)
scanPointsHandler  := scanpointshandler.NewHandler(store)
// … inside the r.Group(middleware.Auth) block:
scanDevicesHandler.RegisterRoutes(r)
scanPointsHandler.RegisterRoutes(r)
```

Add the imports (`scandeviceshandler "…/internal/handlers/scandevices"`, `scanpointshandler "…/internal/handlers/scanpoints"`). Confirm `store` is the `*storage.Storage` in scope (it is — same as other handlers).

- [ ] **Step 2: Build + start smoke** — `just backend build`; optionally run the server and `curl` a route with a session token. Minimum: build passes.
- [ ] **Step 3: Add an HTTP-level integration test** (optional but recommended) under `internal/handlers/scandevices/` registering the routes on a chi router + `testutil` DB, exercising create→get→list→patch→delete. Mirror `handlers/locations/patch_round_trip_integration_test.go` setup.
- [ ] **Step 4: Commit** — `git commit -am "feat(server): TRA-899 mount scan_devices/scan_points routes (session-auth)"`

---

### Task 14: OpenAPI internal spec regen + drift check

**Files:**
- Generated: `backend/docs/*`, `backend/internal/handlers/swaggerspec/openapi.internal.*`

- [ ] **Step 1: Regenerate** — `just backend api-spec`. Expected: succeeds; the new `scandevices`/`scanpoints` operations appear in `openapi.internal.*` and are ABSENT from `openapi.public.*` (verify with `grep -i scan-devices backend/docs/api/openapi.public.yaml` → no matches; `grep` the internal spec → matches). Do NOT edit `rename_public.go`.
- [ ] **Step 2: Verify no drift** — run the repo's spec drift check (the CI `api-spec` drift step; e.g. `just backend api-spec` then `git diff --exit-code` on the generated files after committing them). Commit the regenerated spec artifacts.
- [ ] **Step 3: Commit** — `git commit -am "chore(api): TRA-899 regenerate internal OpenAPI spec"`

---

# PHASE 4 — Frontend

### Task 15: Types

**Files:**
- Create: `frontend/src/types/scandevices/index.ts`

- [ ] **Step 1: Write types** mirroring `src/types/locations/index.ts`: `ScanDevice`, `CreateScanDeviceRequest`, `UpdateScanDeviceRequest`, `ScanDeviceResponse`, `ListScanDevicesResponse`, and `ScanPoint`, `CreateScanPointRequest`, `UpdateScanPointRequest`, `ScanPointResponse`, `ListScanPointsResponse`. Use string-literal unions for the bounded fields:
```ts
export type ScanDeviceType = 'csl_cs463' | 'gl_s10' | 'esp32_ble_generic' | 'csl_cs108';
export type ScanTransport = 'mqtt' | 'web_ble';
```
Include `external_key`, `type`, `transport`, `publish_topic`, `serial_number`, `model`, `description`, `metadata`, `is_active` on the device; `external_key`, `name`, `scan_device_id`, `location_id`, `antenna_port`, `is_boundary` on the point.
- [ ] **Step 2: Typecheck** — `just frontend typecheck`. Expected: clean.
- [ ] **Step 3: Commit** — `git commit -am "feat(fe): TRA-899 scan device/point types"`

### Task 16: API client

**Files:** Create `frontend/src/lib/api/scandevices/index.ts`

- [ ] **Step 1:** Mirror `src/lib/api/locations/index.ts`. Export `scanDevicesApi` with `list`, `get(id)`, `create(data)`, `update(id,data)` (merge-patch CT header), `delete(id)`, `listPoints(deviceId)`, `createPoint(deviceId,data)`, and `scanPointsApi` with `get/update/delete(pointId)`. Use the shared `apiClient`.
- [ ] **Step 2: Typecheck** — `just frontend typecheck`.
- [ ] **Step 3: Commit** — `git commit -am "feat(fe): TRA-899 scan devices api client"`

### Task 17: Query/mutation hooks

**Files:** Create `frontend/src/hooks/scandevices/useScanDevices.ts`, `useScanDeviceMutations.ts`

- [ ] **Step 1:** Mirror `src/hooks/locations/useLocations.ts` + `useLocationMutations.ts`. `queryKey: ['scanDevices', currentOrg?.id]`; mutations invalidate `['scanDevices']`. Add a `useScanPoints(deviceId)` query (`queryKey: ['scanPoints', deviceId]`) + point mutations invalidating that key.
- [ ] **Step 2: Typecheck** — `just frontend typecheck`.
- [ ] **Step 3: Commit** — `git commit -am "feat(fe): TRA-899 scan device hooks"`

### Task 18: Screen + forms + modals

**Files:** Create `ScanDevicesScreen.tsx`, `scandevices/ScanDeviceFormModal.tsx`, `ScanDeviceForm.tsx`, `ScanPointsPanel.tsx`, `ScanPointForm.tsx`

- [ ] **Step 1:** Mirror `LocationsScreen.tsx` + `locations/LocationForm.tsx`/`LocationFormModal.tsx` and `shared/modals/ConfirmModal.tsx`. The screen lists devices in a table (external_key, name, type, transport, publish_topic, active); New/Edit open the form modal (gate pattern: return null when closed). Device form fields: external_key, name, type (`<select>` of the 4 enum values), transport (`<select>` mqtt/web_ble), publish_topic (text, placeholder showing the `trakrf.id/{external_key}/reads` default), serial_number, model, description, is_active. Expanding a device row reveals `ScanPointsPanel` listing its scan_points with add/edit/delete; point form fields: external_key, name, antenna_port, location_id (location picker — reuse the locations hook), is_boundary (checkbox), description. Delete both via `ConfirmModal`. Toasts on success/error.
- [ ] **Step 2: Typecheck + lint** — `just frontend typecheck` && `just frontend lint`.
- [ ] **Step 3: Commit** — `git commit -am "feat(fe): TRA-899 scan devices management UI"`

### Task 19: Register the tab

**Files:** Modify `frontend/src/App.tsx`, `frontend/src/components/TabNavigation.tsx`

- [ ] **Step 1:** Add `'scan-devices'` to `VALID_TABS`; `const ScanDevicesScreen = lazyWithRetry(() => import('@/components/ScanDevicesScreen'))`; add to `tabComponents` and `loadingScreens`; add a nav entry in `TabNavigation.tsx` (icon from lucide-react, e.g. `RadioTower`). Gate visibility to the same role(s) as other management tabs.
- [ ] **Step 2: Typecheck** — `just frontend typecheck`.
- [ ] **Step 3: Commit** — `git commit -am "feat(fe): TRA-899 register scan devices tab"`

### Task 20: Frontend unit test

**Files:** Create `frontend/src/components/scandevices/ScanDeviceFormModal.test.tsx`

- [ ] **Step 1:** Mirror `assets/AssetFormModal.test.tsx`: render the modal in create mode within a `QueryClientProvider`, fill required fields (external_key, name, type), submit, assert the create mutation/`onClose` fires. Assert the modal body unmounts when `isOpen={false}` (gate pattern).
- [ ] **Step 2: Run** — `just frontend test`. Expected: PASS.
- [ ] **Step 3: Commit** — `git commit -am "test(fe): TRA-899 scan device form modal"`

---

# PHASE 5 — Validation

### Task 21: Full gate run

- [ ] **Step 1: Backend** — `just backend lint && just backend test && just backend test-integration`. Expected: all green; `check-rls-guard` clean.
- [ ] **Step 2: Spec drift** — `just backend api-spec && git diff --exit-code -- backend/docs backend/internal/handlers/swaggerspec`. Expected: no diff (already committed).
- [ ] **Step 3: Frontend** — `just frontend validate`. Expected: lint + typecheck + test + build green.
- [ ] **Step 4: Update `log.md`** in the spec dir summarizing what shipped and gate results. Commit.

---

## Self-Review notes
- **Spec coverage:** migration/rename/enums/publish_topic/is_boundary (Tasks 1-5), auto-create removal + fixtures (Tasks 3,10), storage (8-9), internal CRUD handlers + routing (11-13), internal-only OpenAPI (14), UI incl. publish_topic + is_boundary (15-20), validation incl. RLS + diff allowlist (21, 4). All spec requirements mapped.
- **Constraint-name risk:** Task 1 Step 2 explicitly verifies auto-generated UNIQUE-constraint names before relying on the RENAME statements — the one place reality might differ from the plan.
- **Enum-cast risk:** Task 1 maps legacy/unknown `type` values to `csl_cs463` before the cast so `ALTER COLUMN … TYPE` cannot fail on a stray value.
- **Type consistency:** storage method names (`CreateScanDevice`, `GetScanDeviceByID`, `ListScanDevices`, `DeleteScanDevice`, `CreateScanPoint`, `ListScanPointsByDevice`) are used identically in tests (Tasks 8-9), handlers (11-12), and nowhere diverge.
</content>
