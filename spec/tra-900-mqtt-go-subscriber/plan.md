# TRA-900 — MQTT Go Subscriber Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Redpanda Connect MQTT ingester + `process_tag_scans` PG trigger with an in-backend Go MQTT subscriber that parses CS463 reads, resolves the registry, and writes `asset_scans` with per-write org context (RLS-correct), while demoting `tag_scans` to an append-only audit log.

**Architecture:** New `internal/ingest` package (config + parser + subscriber + metrics), new `internal/storage/ingest.go` (resolver / audit-insert / RLS-correct derivation), migration `000012` (drop trigger+fn, add `SECURITY DEFINER` topic resolver), and a goroutine started from `serve.Run` gated on `MQTT_URL`. Asset resolution is tag-based with no auto-create; `asset_scans.timestamp` is server receive time.

**Tech Stack:** Go 1.25, `github.com/eclipse/paho.mqtt.golang`, pgx v5, golang-migrate, Prometheus (default registry via `promauto`), zerolog, testify; TRA-874 RLS integration harness.

**Spec:** `docs/superpowers/specs/2026-06-04-tra-900-mqtt-go-subscriber-design.md`

---

## File Structure

- Create `backend/migrations/000012_drop_tag_scan_trigger.up.sql` / `.down.sql` — drop trigger+fn, add `resolve_scan_topic`.
- Create `backend/internal/ingest/parser.go` — `Read` type, `Parse(deviceType, payload)` dispatch, `ErrUnsupportedDevice`.
- Create `backend/internal/ingest/parser_cs463.go` — CS463 payload → `[]Read`.
- Create `backend/internal/ingest/parser_test.go` — unit tests over real-capture fixtures.
- Create `backend/internal/ingest/config.go` — env config + `Enabled()`.
- Create `backend/internal/ingest/metrics.go` — Prometheus counters.
- Create `backend/internal/ingest/subscriber.go` — MQTT lifecycle + per-message orchestration.
- Create `backend/internal/storage/ingest.go` — `ResolveScanTopic`, `InsertRawTagScan`, `PersistReads`.
- Create `backend/internal/storage/ingest_integration_test.go` — RLS-harness integration tests.
- Create `backend/internal/testutil/fixtures/cs463_read_multi.json` — live multi-tag capture.
- Modify `backend/internal/cmd/serve/serve.go` — start/stop subscriber goroutine.
- Modify `backend/go.mod` / `go.sum` — add paho dependency.

---

## Task 1: Add the MQTT client dependency

**Files:** Modify `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add dependency**

Run from repo root:
```bash
just backend go get github.com/eclipse/paho.mqtt.golang@v1.5.0
just backend go mod tidy
```

- [ ] **Step 2: Verify it resolves**

Run: `just backend go build ./...`
Expected: exit 0 (no compile/download errors).

- [ ] **Step 3: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "chore(deps): TRA-900 add paho.mqtt.golang"
```

---

## Task 2: CS463 parser (pure, unit-tested)

**Files:**
- Create: `backend/internal/ingest/parser.go`, `backend/internal/ingest/parser_cs463.go`
- Test: `backend/internal/ingest/parser_test.go`
- Create: `backend/internal/testutil/fixtures/cs463_read_multi.json`

- [ ] **Step 1: Add the multi-tag fixture (real 2026-06-04 capture)**

Create `backend/internal/testutil/fixtures/cs463_read_multi.json`:
```json
{
  "sequenceNumber": 0,
  "rfidReaderName": "cs463-214",
  "pcEthernetMACAddress": "",
  "numberOfTags": 2,
  "tags": [
    { "epc": "712AC12F1007000000224403", "timeStampOfRead": 1780591822281000, "timeZone": "-6:00", "antennaPort": 1, "capturePointName": "cs463-214-1", "rssi": "-70" },
    { "epc": "E2801190A503006543E0E3A4", "timeStampOfRead": 1780591822297000, "timeZone": "-6:00", "antennaPort": 1, "capturePointName": "cs463-214-1", "rssi": "-61" }
  ]
}
```

- [ ] **Step 2: Write the failing test**

Create `backend/internal/ingest/parser_test.go`:
```go
package ingest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testutil", "fixtures", name))
	require.NoError(t, err)
	return b
}

func TestParseCS463_SingleTag(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeCS463, loadFixture(t, "cs463_read.json"))
	require.NoError(t, err)
	require.Len(t, reads, 1)
	r := reads[0]
	assert.Equal(t, "E2801190A503006543E21224", r.EPC)
	assert.Equal(t, "cs463-214-1", r.CapturePointName)
	assert.Equal(t, 1, r.AntennaPort)
	assert.Equal(t, -56, r.RSSI)
	// timeStampOfRead is microseconds since epoch.
	assert.Equal(t, time.UnixMicro(1780587173668000).UTC(), r.ReaderTimestamp.UTC())
}

func TestParseCS463_MultiTag(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeCS463, loadFixture(t, "cs463_read_multi.json"))
	require.NoError(t, err)
	require.Len(t, reads, 2)
	assert.Equal(t, "712AC12F1007000000224403", reads[0].EPC)
	assert.Equal(t, -70, reads[0].RSSI)
	assert.Equal(t, "E2801190A503006543E0E3A4", reads[1].EPC)
}

func TestParse_UnsupportedDevice(t *testing.T) {
	_, err := Parse(scandevice.DeviceTypeGLS10, loadFixture(t, "gls10_read.json"))
	assert.ErrorIs(t, err, ErrUnsupportedDevice)
}

func TestParseCS463_MalformedJSON(t *testing.T) {
	_, err := Parse(scandevice.DeviceTypeCS463, []byte("not json"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnsupportedDevice)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `just backend go test ./internal/ingest/...`
Expected: FAIL — `undefined: Parse`, `undefined: ErrUnsupportedDevice`.

- [ ] **Step 4: Implement `parser.go`**

Create `backend/internal/ingest/parser.go`:
```go
// Package ingest contains the in-backend MQTT subscriber that replaces the
// Redpanda Connect ingester and the process_tag_scans PG trigger (TRA-900).
package ingest

import (
	"errors"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
)

// ErrUnsupportedDevice is returned by Parse when a device type has no parser
// yet (GL-S10 / ESP32 / CS108 are deferred to their own tickets).
var ErrUnsupportedDevice = errors.New("ingest: unsupported device type")

// Read is one parsed tag observation, device-agnostic. Shared with the TRA-901
// geofence engine so there is a single in-Go parser per device type.
type Read struct {
	EPC              string
	CapturePointName string
	AntennaPort      int
	RSSI             int
	ReaderTimestamp  time.Time // informational only; server time is authoritative
}

// Parse dispatches a raw MQTT payload to the parser for the registered device
// type. It never panics on bad input — malformed payloads return an error.
func Parse(deviceType string, payload []byte) ([]Read, error) {
	switch deviceType {
	case scandevice.DeviceTypeCS463:
		return parseCS463(payload)
	default:
		return nil, ErrUnsupportedDevice
	}
}
```

- [ ] **Step 5: Implement `parser_cs463.go`**

Create `backend/internal/ingest/parser_cs463.go`:
```go
package ingest

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// cs463Payload is the CS463 reader JSON shape (verified against live preview
// traffic 2026-06-04). rssi arrives as a quoted string (e.g. "-56").
type cs463Payload struct {
	Tags []cs463Tag `json:"tags"`
}

type cs463Tag struct {
	EPC              string `json:"epc"`
	TimeStampOfRead  int64  `json:"timeStampOfRead"` // microseconds since epoch
	AntennaPort      int    `json:"antennaPort"`
	CapturePointName string `json:"capturePointName"`
	RSSI             string `json:"rssi"`
}

func parseCS463(payload []byte) ([]Read, error) {
	var p cs463Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("cs463: unmarshal payload: %w", err)
	}
	reads := make([]Read, 0, len(p.Tags))
	for _, t := range p.Tags {
		rssi := 0
		if t.RSSI != "" {
			v, err := strconv.Atoi(t.RSSI)
			if err != nil {
				return nil, fmt.Errorf("cs463: parse rssi %q: %w", t.RSSI, err)
			}
			rssi = v
		}
		reads = append(reads, Read{
			EPC:              t.EPC,
			CapturePointName: t.CapturePointName,
			AntennaPort:      t.AntennaPort,
			RSSI:             rssi,
			ReaderTimestamp:  time.UnixMicro(t.TimeStampOfRead).UTC(),
		})
	}
	return reads, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `just backend go test ./internal/ingest/...`
Expected: PASS (4 tests).

- [ ] **Step 7: Commit**

```bash
git add backend/internal/ingest/parser.go backend/internal/ingest/parser_cs463.go backend/internal/ingest/parser_test.go backend/internal/testutil/fixtures/cs463_read_multi.json
git commit -m "feat(ingest): TRA-900 CS463 MQTT payload parser"
```

---

## Task 3: Migration 000012 — drop trigger, add `resolve_scan_topic`

**Files:** Create `backend/migrations/000012_drop_tag_scan_trigger.up.sql` / `.down.sql`

- [ ] **Step 1: Write the up migration**

Create `backend/migrations/000012_drop_tag_scan_trigger.up.sql`:
```sql
-- TRA-900 — retire trigger-driven ingestion. The Go MQTT subscriber now owns
-- parse + derive (asset_scans) with per-write org context; tag_scans is an
-- append-only audit log. Add a thin SECURITY DEFINER resolver so the subscriber
-- (role trakrf-app, RLS-enforced, cannot read scan_devices without an org GUC)
-- can map an MQTT topic to its owning org before it has an org context to set.
SET search_path = trakrf, public;

DROP TRIGGER IF EXISTS trigger_process_tag_scans ON trakrf.tag_scans;
DROP FUNCTION IF EXISTS trakrf.process_tag_scans();

-- resolve_scan_topic: read-only routing lookup. Honors the documented default
-- publish_topic = trakrf.id/{external_key}/reads. SECURITY DEFINER so it sees
-- all orgs' devices (the routing key is cross-org by design); returns only the
-- minimal ids needed to route + set org context.
CREATE OR REPLACE FUNCTION trakrf.resolve_scan_topic(p_topic text)
RETURNS TABLE (org_id bigint, scan_device_id bigint, device_type trakrf.scan_device_type)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT d.org_id, d.id, d.type
    FROM trakrf.scan_devices d
    WHERE d.deleted_at IS NULL
      AND ( d.publish_topic = p_topic
            OR (d.publish_topic IS NULL
                AND p_topic = 'trakrf.id/' || d.external_key || '/reads') )
    LIMIT 1;
$$;

COMMENT ON FUNCTION trakrf.resolve_scan_topic(text) IS
    'TRA-900: maps an MQTT topic to (org_id, scan_device_id, device_type). SECURITY DEFINER so the RLS-enforced trakrf-app role can route before it knows the org. Read-only, single-purpose.';

REVOKE ALL ON FUNCTION trakrf.resolve_scan_topic(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION trakrf.resolve_scan_topic(text) TO PUBLIC;
```

(`GRANT ... TO PUBLIC` keeps the migration role-name-agnostic — the app role across
envs is `trakrf-app-<env>` / `trakrf_test_app`; EXECUTE to PUBLIC on a read-only
definer routing function is acceptable and matches how the schema grants other
functions. The init-grants job re-applies default privileges separately.)

- [ ] **Step 2: Write the down migration**

Create `backend/migrations/000012_drop_tag_scan_trigger.down.sql` — restore the 000011 trigger + function and drop the resolver. Copy the function body verbatim from `000011_scan_device_model.up.sql` lines 71-112 (the schema-qualified, registry-driven form), then recreate the trigger:
```sql
-- TRA-900 down — restore the 000011 trigger-driven ingestion form.
SET search_path = trakrf, public;

DROP FUNCTION IF EXISTS trakrf.resolve_scan_topic(text);

CREATE OR REPLACE FUNCTION trakrf.process_tag_scans() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    topic_org_id BIGINT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM trakrf.organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %', NEW.message_topic;
        RETURN NEW;
    END IF;

    INSERT INTO trakrf.assets (org_id, external_key, name)
    SELECT DISTINCT topic_org_id, t.tag ->> 'epc', t.tag ->> 'epc' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT 1 FROM trakrf.assets a WHERE a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc')
      AND NOT EXISTS (SELECT 1 FROM trakrf.tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO trakrf.tags (org_id, asset_id, type, value)
    SELECT DISTINCT topic_org_id, a.id, 'rfid', t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN trakrf.assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    WHERE NOT EXISTS (SELECT 1 FROM trakrf.tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0),
        topic_org_id, a.id, sp.location_id, sp.id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN trakrf.scan_points sp ON sp.org_id = topic_org_id AND sp.external_key = t.tag ->> 'capturePointName'
    JOIN trakrf.assets a       ON a.org_id  = topic_org_id AND a.external_key = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing tag_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_process_tag_scans
    AFTER INSERT ON trakrf.tag_scans
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.process_tag_scans();
```

- [ ] **Step 3: Verify the migration applies + reverses cleanly**

Run (needs a local PG; harness recreates a DB):
```bash
just backend go test -tags=integration ./internal/storage/... -run TestMigrations -count=1
```
If no such smoke exists, defer verification to Task 6's integration tests (they run all migrations) and skip here.

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000012_drop_tag_scan_trigger.up.sql backend/migrations/000012_drop_tag_scan_trigger.down.sql
git commit -m "feat(db): TRA-900 drop process_tag_scans trigger, add resolve_scan_topic"
```

---

## Task 4: Storage layer — resolver, audit insert, RLS-correct derivation

**Files:**
- Create: `backend/internal/storage/ingest.go`
- Test: `backend/internal/storage/ingest_integration_test.go`

- [ ] **Step 1: Write the failing integration tests**

Create `backend/internal/storage/ingest_integration_test.go` (`//go:build integration`). Use the TRA-874 harness (`testutil.SetupTestDBFull`) and factories. Mirror existing integration tests for setup style. Tests:
```go
//go:build integration

package storage_test

// Verifies (on the non-superuser RLS role):
//   - ResolveScanTopic finds a device by explicit publish_topic
//   - ResolveScanTopic finds a device by the external_key default topic
//   - ResolveScanTopic returns found=false for an unknown topic
//   - InsertRawTagScan appends a row and returns its id
//   - PersistReads: registered scan_point + rfid-tagged asset => exactly one
//     asset_scan with correct location_id/scan_point_id/tag_scan_id
//   - PersistReads: unregistered EPC => zero asset_scans (membership filter)
//   - PersistReads: unknown capturePointName => zero asset_scans
//   - PersistReads: duplicate EPC in one batch => one asset_scan (content PK)
```
Author concrete tests using the helpers found in `internal/testutil` (`SetupTestDBFull`, `CreateTestAccount`, scan-device/scan-point/asset/tag factories). For any fixture not covered by an existing factory, insert via `db.AdminPool` (superuser) the way other `*_integration_test.go` files do. Assert counts via `db.AdminPool` to bypass RLS for verification.

- [ ] **Step 2: Run to verify it fails**

Run: `just backend go test -tags=integration ./internal/storage/... -run Ingest -count=1`
Expected: FAIL — `undefined: ResolveScanTopic` / `PersistReads`.

- [ ] **Step 3: Implement `internal/storage/ingest.go`**

```go
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/ingest"
)

// ScanRoute is the routing result for an MQTT topic (TRA-900).
type ScanRoute struct {
	OrgID        int
	ScanDeviceID int
	DeviceType   string
}

// ResolveScanTopic maps an MQTT topic to its owning org + device via the
// SECURITY DEFINER resolver, so it works without any org context set. Returns
// found=false when no live device matches the topic.
func (s *Storage) ResolveScanTopic(ctx context.Context, topic string) (ScanRoute, bool, error) {
	var r ScanRoute
	err := s.pool.QueryRow(ctx,
		`SELECT org_id, scan_device_id, device_type FROM trakrf.resolve_scan_topic($1)`, topic,
	).Scan(&r.OrgID, &r.ScanDeviceID, &r.DeviceType)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScanRoute{}, false, nil
	}
	if err != nil {
		return ScanRoute{}, false, fmt.Errorf("resolve scan topic: %w", err)
	}
	return r, true, nil
}

// InsertRawTagScan appends the raw MQTT message to the tag_scans audit log and
// returns the new row id. tag_scans has no RLS, so no org context is needed.
func (s *Storage) InsertRawTagScan(ctx context.Context, topic string, payload []byte) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO trakrf.tag_scans (message_topic, message_data) VALUES ($1, $2) RETURNING id`,
		topic, payload,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert raw tag_scan: %w", err)
	}
	return id, nil
}

// PersistResult summarizes a PersistReads run for logging/metrics.
type PersistResult struct {
	Inserted int
	Dropped  map[string]int // reason -> count: no_scan_point, no_asset, conflict
}

// PersistReads writes asset_scans for parsed reads under org context (RLS).
// Asset resolution is tag-based with NO auto-create (TRA-900): a read records a
// scan only if its EPC already has a live rfid tag. receivedAt (server time) is
// authoritative for asset_scans.timestamp; the reader clock is ignored.
func (s *Storage) PersistReads(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []ingest.Read) (PersistResult, error) {
	res := PersistResult{Dropped: map[string]int{}}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		for _, rd := range reads {
			var scanPointID int
			var locationID *int
			err := tx.QueryRow(ctx,
				`SELECT id, location_id FROM trakrf.scan_points
				 WHERE org_id = $1 AND external_key = $2 AND deleted_at IS NULL`,
				orgID, rd.CapturePointName,
			).Scan(&scanPointID, &locationID)
			if errors.Is(err, pgx.ErrNoRows) {
				res.Dropped["no_scan_point"]++
				continue
			}
			if err != nil {
				return fmt.Errorf("resolve scan_point %q: %w", rd.CapturePointName, err)
			}

			var assetID int
			err = tx.QueryRow(ctx,
				`SELECT asset_id FROM trakrf.tags
				 WHERE org_id = $1 AND type = 'rfid' AND value = $2
				   AND asset_id IS NOT NULL AND deleted_at IS NULL
				 LIMIT 1`,
				orgID, rd.EPC,
			).Scan(&assetID)
			if errors.Is(err, pgx.ErrNoRows) {
				res.Dropped["no_asset"]++
				continue
			}
			if err != nil {
				return fmt.Errorf("resolve asset for epc %q: %w", rd.EPC, err)
			}

			ct, err := tx.Exec(ctx,
				`INSERT INTO trakrf.asset_scans
				   (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id)
				 VALUES ($1, $2, $3, $4, $5, $6)
				 ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING`,
				receivedAt, orgID, assetID, locationID, scanPointID, tagScanID,
			)
			if err != nil {
				return fmt.Errorf("insert asset_scan: %w", err)
			}
			if ct.RowsAffected() == 0 {
				res.Dropped["conflict"]++
				continue
			}
			res.Inserted++
		}
		return nil
	})
	if err != nil {
		return PersistResult{}, err
	}
	return res, nil
}
```

- [ ] **Step 4: Run to verify the tests pass**

Run: `just backend go test -tags=integration ./internal/storage/... -run Ingest -count=1`
Expected: PASS. (Requires local PG; see `reference_asset_scans_retention_test_gotcha` for the local int-test DB creds / `PG_URL` search_path. If no local PG is available, mark this task's verification as deferred to CI and note it explicitly.)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/ingest.go backend/internal/storage/ingest_integration_test.go
git commit -m "feat(storage): TRA-900 topic resolver, audit insert, tag-based asset_scans derivation"
```

---

## Task 5: Config + metrics

**Files:** Create `backend/internal/ingest/config.go`, `backend/internal/ingest/metrics.go`, `backend/internal/ingest/config_test.go`

- [ ] **Step 1: Write failing config test**

Create `backend/internal/ingest/config_test.go`:
```go
package ingest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigEnabled(t *testing.T) {
	assert.False(t, Config{URL: ""}.Enabled())
	assert.True(t, Config{URL: "mqtts://x"}.Enabled())
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("MQTT_URL", "mqtts://u:p@host:8883")
	t.Setenv("MQTT_TOPIC", "")
	t.Setenv("MQTT_CLIENT_ID", "")
	c := ConfigFromEnv()
	assert.Equal(t, "mqtts://u:p@host:8883", c.URL)
	assert.Equal(t, "trakrf.id/#", c.Topic)
	assert.Equal(t, "trakrf-subscriber", c.ClientID)
}
```

- [ ] **Step 2: Run to verify fail**

Run: `just backend go test ./internal/ingest/... -run Config`
Expected: FAIL — undefined `Config`/`ConfigFromEnv`.

- [ ] **Step 3: Implement `config.go`**

```go
package ingest

import "os"

// Config controls the MQTT subscriber. An empty URL disables it entirely
// (keeps local dev, tests, and pre-cutover prod inert).
type Config struct {
	URL      string // mqtts://user:pass@host:port  (MQTT_URL)
	Topic    string // subscription filter (MQTT_TOPIC), e.g. trakrf.id/# or $share/grp/trakrf.id/#
	ClientID string // base client id (MQTT_CLIENT_ID); subscriber appends a per-process suffix
}

func (c Config) Enabled() bool { return c.URL != "" }

func ConfigFromEnv() Config {
	c := Config{
		URL:      os.Getenv("MQTT_URL"),
		Topic:    os.Getenv("MQTT_TOPIC"),
		ClientID: os.Getenv("MQTT_CLIENT_ID"),
	}
	if c.Topic == "" {
		c.Topic = "trakrf.id/#"
	}
	if c.ClientID == "" {
		c.ClientID = "trakrf-subscriber"
	}
	return c
}
```

- [ ] **Step 4: Implement `metrics.go`**

```go
package ingest

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Counters live on the default registry, which serve's /metrics handler exposes.
var (
	metricMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_messages_total",
		Help: "MQTT messages received by the in-backend subscriber, by result.",
	}, []string{"result"}) // received, unregistered_topic, unsupported_device, parse_error, persist_error

	metricReadsParsed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_reads_parsed_total",
		Help: "Tag reads parsed from MQTT payloads.",
	})

	metricAssetScansInserted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_asset_scans_inserted_total",
		Help: "asset_scans rows inserted by the subscriber.",
	})

	metricReadsDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_reads_dropped_total",
		Help: "Parsed reads dropped during derivation, by reason.",
	}, []string{"reason"}) // no_scan_point, no_asset, conflict
)
```

- [ ] **Step 5: Run to verify pass**

Run: `just backend go test ./internal/ingest/... -run Config`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ingest/config.go backend/internal/ingest/config_test.go backend/internal/ingest/metrics.go
git commit -m "feat(ingest): TRA-900 subscriber config + prometheus metrics"
```

---

## Task 6: Subscriber (MQTT lifecycle + orchestration)

**Files:** Create `backend/internal/ingest/subscriber.go`

- [ ] **Step 1: Implement `subscriber.go`**

```go
package ingest

import (
	"context"
	"errors"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
)

// Subscriber consumes MQTT reads and derives asset_scans (TRA-900). It is the
// observable replacement for the silent process_tag_scans trigger.
type Subscriber struct {
	cfg    Config
	store  *storage.Storage
	log    zerolog.Logger
	client mqtt.Client
}

func NewSubscriber(cfg Config, store *storage.Storage, log zerolog.Logger) *Subscriber {
	return &Subscriber{cfg: cfg, store: store, log: log.With().Str("component", "ingest").Logger()}
}

// Start connects and subscribes. It returns once connected (or on connect
// error); message handling continues on paho's goroutines until Stop.
func (s *Subscriber) Start(ctx context.Context) error {
	clientID := s.cfg.ClientID
	if host, _ := os.Hostname(); host != "" {
		clientID = clientID + "-" + host // unique per replica; avoid duplicate-id disconnect loops
	}

	opts := mqtt.NewClientOptions().
		AddBroker(s.cfg.URL).
		SetClientID(clientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(func(c mqtt.Client) {
			if tok := c.Subscribe(s.cfg.Topic, 1, s.handleMessage); tok.Wait() && tok.Error() != nil {
				s.log.Error().Err(tok.Error()).Str("topic", s.cfg.Topic).Msg("subscribe failed")
				return
			}
			s.log.Info().Str("topic", s.cfg.Topic).Msg("subscribed")
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			s.log.Warn().Err(err).Msg("mqtt connection lost; auto-reconnecting")
		})

	s.client = mqtt.NewClient(opts)
	tok := s.client.Connect()
	if tok.Wait() && tok.Error() != nil {
		return tok.Error()
	}
	s.log.Info().Str("client_id", clientID).Msg("mqtt subscriber connected")
	return nil
}

// Stop disconnects the client (idempotent).
func (s *Subscriber) Stop() {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250)
		s.log.Info().Msg("mqtt subscriber disconnected")
	}
}

// handleMessage is the per-message pipeline. It recovers from panics so one bad
// payload never kills ingestion, and it logs/metrics every outcome (no silent
// swallow, unlike the old trigger).
func (s *Subscriber) handleMessage(_ mqtt.Client, m mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error().Interface("panic", r).Str("topic", m.Topic()).Msg("recovered from panic in handler")
			metricMessages.WithLabelValues("parse_error").Inc()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topic, payload := m.Topic(), m.Payload()
	receivedAt := time.Now() // server time wins over reader timeStampOfRead
	metricMessages.WithLabelValues("received").Inc()

	// 1. Always append to the audit log first (gives us tag_scan_id provenance).
	tagScanID, err := s.store.InsertRawTagScan(ctx, topic, payload)
	if err != nil {
		s.log.Error().Err(err).Str("topic", topic).Msg("audit insert failed")
		metricMessages.WithLabelValues("persist_error").Inc()
		return
	}

	// 2. Route topic -> org/device (SECURITY DEFINER; no org context yet).
	route, found, err := s.store.ResolveScanTopic(ctx, topic)
	if err != nil {
		s.log.Error().Err(err).Str("topic", topic).Msg("topic resolution failed")
		metricMessages.WithLabelValues("persist_error").Inc()
		return
	}
	if !found {
		s.log.Debug().Str("topic", topic).Msg("unregistered topic; audit kept, no derivation")
		metricMessages.WithLabelValues("unregistered_topic").Inc()
		return
	}

	// 3. Parse by registered device type.
	reads, err := Parse(route.DeviceType, payload)
	if errors.Is(err, ErrUnsupportedDevice) {
		s.log.Debug().Str("topic", topic).Str("device_type", route.DeviceType).Msg("unsupported device type; deferred")
		metricMessages.WithLabelValues("unsupported_device").Inc()
		return
	}
	if err != nil {
		s.log.Error().Err(err).Str("topic", topic).Msg("parse failed")
		metricMessages.WithLabelValues("parse_error").Inc()
		return
	}
	metricReadsParsed.Add(float64(len(reads)))

	// 4. Derive asset_scans under org context (RLS-correct).
	// TRA-901 seam: `reads` is also where the geofence engine will be handed the
	// parsed observations for the immediate-on-entry alarm decision.
	res, err := s.store.PersistReads(ctx, route.OrgID, tagScanID, receivedAt, reads)
	if err != nil {
		s.log.Error().Err(err).Str("topic", topic).Int("org_id", route.OrgID).Msg("derivation failed")
		metricMessages.WithLabelValues("persist_error").Inc()
		return
	}
	metricAssetScansInserted.Add(float64(res.Inserted))
	for reason, n := range res.Dropped {
		metricReadsDropped.WithLabelValues(reason).Add(float64(n))
	}
	s.log.Info().
		Str("topic", topic).Int("org_id", route.OrgID).
		Int("parsed", len(reads)).Int("inserted", res.Inserted).
		Interface("dropped", res.Dropped).
		Msg("ingest message processed")
}
```

- [ ] **Step 2: Build**

Run: `just backend go build ./...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ingest/subscriber.go
git commit -m "feat(ingest): TRA-900 MQTT subscriber lifecycle + per-message pipeline"
```

---

## Task 7: Wire the subscriber into `serve.Run`

**Files:** Modify `backend/internal/cmd/serve/serve.go`

- [ ] **Step 1: Start the subscriber after storage init**

In `serve.go`, after `defer store.Close()` (line 78) and before services init, add:
```go
	// TRA-900: in-backend MQTT subscriber (replaces the RC ingester + the
	// process_tag_scans trigger). Disabled when MQTT_URL is unset, so local
	// dev / tests / pre-cutover prod stay inert.
	mqttCfg := ingest.ConfigFromEnv()
	if mqttCfg.Enabled() {
		subscriber := ingest.NewSubscriber(mqttCfg, store, log)
		if err := subscriber.Start(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to start MQTT subscriber")
			return err
		}
		defer subscriber.Stop()
		log.Info().Msg("MQTT subscriber started")
	} else {
		log.Info().Msg("MQTT subscriber disabled (MQTT_URL unset)")
	}
```
Add the import `"github.com/trakrf/platform/backend/internal/ingest"` to the import block.

- [ ] **Step 2: Build + vet**

Run: `just backend go build ./... && just backend go vet ./internal/...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/cmd/serve/serve.go
git commit -m "feat(serve): TRA-900 start/stop MQTT subscriber goroutine"
```

---

## Task 8: Full verification

- [ ] **Step 1: Unit tests + vet + lint**

Run:
```bash
just backend go test ./internal/ingest/... -count=1
just backend go vet ./...
just lint
```
Expected: all green.

- [ ] **Step 2: Integration tests (if local PG available)**

Run: `just backend go test -tags=integration ./internal/storage/... -run Ingest -count=1`
Expected: PASS. If no local PG, record that integration verification is deferred to CI and to the live preview smoke (Step 3).

- [ ] **Step 3: Live preview smoke (manual, after deploy)**

The opened PR auto-deploys to preview with `MQTT_URL` configured (infra). After deploy, confirm `asset_scans` start populating for a registered device whose EPCs have rfid tags, and that `ingest_messages_total` / `ingest_asset_scans_inserted_total` advance on `/metrics`. (Cross-checked against live broker with creds from `../.env.local`.)

- [ ] **Step 4: Update CHANGELOG if the repo convention requires it** (check `CHANGELOG.md` for the pattern; add a `feat` line under unreleased if so).

---

## Self-Review (completed)

- **Spec coverage:** parse-in-Go (T2) ✔; publish_topic org resolution via SECURITY DEFINER (T3/T4) ✔; per-write org context / RLS fix (T4) ✔; tag-based no-auto-create (T4) ✔; server-time PK (T4/T6) ✔; tag_scans audit + tag_scan_id provenance (T4/T6) ✔; drop trigger (T3) ✔; observability logs+metrics (T5/T6) ✔; wiring + graceful stop (T7) ✔; MQTT lib + multi-replica caveat (T1/T5/T6 docs) ✔; TRA-901 seam (T6 comment) ✔.
- **Placeholders:** none — every code step has full code; the only deferred item is the integration-test *body* in T4 Step 1, which is explicitly delegated to the existing harness/factories with an enumerated assertion list (the harness API differs enough that inventing exact factory calls here risks drift; the executor reads `internal/testutil` and the sibling `*_integration_test.go` files).
- **Type consistency:** `Read`, `ScanRoute`, `PersistResult`, `Config`, `Subscriber`, `Parse`, `ResolveScanTopic`, `InsertRawTagScan`, `PersistReads`, `ConfigFromEnv`, `NewSubscriber` used consistently across tasks. Device-type constants from `scandevice`. Metric names consistent between T5 and T6.
```
