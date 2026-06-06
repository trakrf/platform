# Output Event Rules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-output `egress`/`presence` rule modes with a configurable age-out, surface all rule config (mode, age-out, auto-off, RSSI threshold) on the output-device form, and drop the misaligned `is_boundary` gate/column and the hidden per-scan-point `rssi_threshold` knob.

**Architecture:** All rule config moves onto `output_device.metadata` (jsonb). The geofence engine now resolves `location → outputs` up front and keys its dedup/presence state per `(output, epc)`. Egress = fire ON, latch with per-output re-arm window (existing behavior, re-keyed). Presence = ON on first member, OFF when the last member ages out via the sweep goroutine. `scan_points.is_boundary` and `scan_points.metadata.rssi_threshold` are removed entirely (greenfield, unreleased — no data to migrate).

**Tech Stack:** Go (backend, geofence/alarm/storage), TimescaleDB migrations (golang-migrate), React/TypeScript + Vitest (frontend).

Spec: `docs/superpowers/specs/2026-06-06-tra-943-output-event-rules-design.md`

**Conventions:**
- Backend tests: `just backend test` (or `cd backend && go test ./internal/geofence/...`). Lint: `just backend lint`.
- Frontend tests: `just frontend test`. Typecheck: `just frontend typecheck`. Lint: `just frontend lint`.
- Commit per task (frequent commits). Conventional commits, `tra-943` scope. End commit messages with the Co-Authored-By trailer.
- Branch is already `feat/tra-943-output-event-rules`.

---

## Task 1: Migration — drop `scan_points.is_boundary`

**Files:**
- Create: `backend/migrations/000018_drop_scan_point_is_boundary.up.sql`
- Create: `backend/migrations/000018_drop_scan_point_is_boundary.down.sql`

- [ ] **Step 1: Write the up migration**

`backend/migrations/000018_drop_scan_point_is_boundary.up.sql`:
```sql
-- TRA-943: the is_boundary gate is removed. After output devices were mapped to
-- location (not scan point), a per-scan-point boundary flag no longer maps to a
-- clean per-portal concept; all rule config now lives visibly on the output
-- device. Greenfield (unreleased) — safe to drop.
ALTER TABLE trakrf.scan_points DROP COLUMN is_boundary;
```

- [ ] **Step 2: Write the down migration**

`backend/migrations/000018_drop_scan_point_is_boundary.down.sql`:
```sql
ALTER TABLE trakrf.scan_points
    ADD COLUMN is_boundary BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN trakrf.scan_points.is_boundary IS
    'Geofence boundary marker (TRA-901): true = portal/exit capture point.';
```

- [ ] **Step 3: Verify migrations embed + parse**

Run: `cd backend && go test ./migrations/...`
Expected: PASS (embed_test validates up/down pairing + naming).

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000018_drop_scan_point_is_boundary.*.sql
git commit -m "$(printf 'feat(tra-943): migration to drop scan_points.is_boundary\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 2: Output device metadata accessors (`Mode`, `AgeOutSeconds`, `RSSIThreshold`)

**Files:**
- Modify: `backend/internal/models/outputdevice/outputdevice.go`
- Test: `backend/internal/models/outputdevice/outputdevice_test.go` (create if absent; otherwise append)

- [ ] **Step 1: Write failing tests**

Append to `outputdevice_test.go` (create with `package outputdevice` + imports `testing` if new):
```go
func dev(meta map[string]any) OutputDevice { return OutputDevice{Metadata: meta} }

func TestMode(t *testing.T) {
	if got := dev(nil).Mode(); got != ModeEgress {
		t.Fatalf("nil metadata should default to egress, got %q", got)
	}
	if got := dev(map[string]any{"mode": "presence"}).Mode(); got != ModePresence {
		t.Fatalf("expected presence, got %q", got)
	}
	if got := dev(map[string]any{"mode": "garbage"}).Mode(); got != ModeEgress {
		t.Fatalf("unknown mode should default to egress, got %q", got)
	}
}

func TestAgeOutSeconds(t *testing.T) {
	if _, ok := dev(nil).AgeOutSeconds(); ok {
		t.Fatal("nil metadata should report no override")
	}
	if _, ok := dev(map[string]any{"age_out_seconds": float64(0)}).AgeOutSeconds(); ok {
		t.Fatal("zero should report no override (fall back to global)")
	}
	v, ok := dev(map[string]any{"age_out_seconds": float64(30)}).AgeOutSeconds()
	if !ok || v != 30 {
		t.Fatalf("expected (30,true), got (%d,%v)", v, ok)
	}
}

func TestRSSIThreshold(t *testing.T) {
	if _, ok := dev(nil).RSSIThreshold(); ok {
		t.Fatal("nil metadata should report no override")
	}
	// RSSI is dBm — negatives are valid and must NOT be rejected.
	v, ok := dev(map[string]any{"rssi_threshold": float64(-55)}).RSSIThreshold()
	if !ok || v != -55 {
		t.Fatalf("expected (-55,true), got (%d,%v)", v, ok)
	}
	if _, ok := dev(map[string]any{"rssi_threshold": "nope"}).RSSIThreshold(); ok {
		t.Fatal("non-numeric should report no override")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/models/outputdevice/...`
Expected: FAIL (undefined: ModeEgress / Mode / AgeOutSeconds / RSSIThreshold).

- [ ] **Step 3: Implement accessors + DRY the metadata int parsing**

In `outputdevice.go`, add the mode constants near the transport constants:
```go
// Rule modes (TRA-943). egress = fire ON on a crossing then latch; presence =
// ON while >=1 member tag is present, OFF when the last ages out.
const (
	ModeEgress   = "egress"
	ModePresence = "presence"
)
```

Replace the body of `AutoOffSeconds` to reuse a shared parser, and add the new accessors:
```go
// metaInt reads metadata[key] as an int. Metadata arrives as map[string]any from
// jsonb, so numbers are float64; int/int64/json.Number are tolerated too. ok is
// false when the key is absent or not numeric.
func (d OutputDevice) metaInt(key string) (int, bool) {
	m, ok := d.Metadata.(map[string]any)
	if !ok {
		return 0, false
	}
	switch n := m[key].(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// AutoOffSeconds returns metadata.auto_off_seconds, or 0 when unset, zero,
// negative, or non-numeric. 0 = stay on until manual reset. Device-side
// (Shelly toggle_after); ignored by the engine in presence mode.
func (d OutputDevice) AutoOffSeconds() int {
	v, ok := d.metaInt("auto_off_seconds")
	if !ok || v < 0 {
		return 0
	}
	return v
}

// Mode returns metadata.mode, defaulting to ModeEgress for unset/unknown values.
func (d OutputDevice) Mode() string {
	m, ok := d.Metadata.(map[string]any)
	if ok {
		if s, _ := m["mode"].(string); s == ModePresence {
			return ModePresence
		}
	}
	return ModeEgress
}

// AgeOutSeconds returns the per-output age-out override from
// metadata.age_out_seconds. ok is false (caller falls back to the global TTL)
// when unset, non-numeric, or <= 0. Egress: re-arm window. Presence: departure
// window.
func (d OutputDevice) AgeOutSeconds() (int, bool) {
	v, ok := d.metaInt("age_out_seconds")
	if !ok || v <= 0 {
		return 0, false
	}
	return v, true
}

// RSSIThreshold returns the per-output RSSI trip line from
// metadata.rssi_threshold (dBm; negatives valid). ok is false (caller falls back
// to the global threshold) when unset or non-numeric.
func (d OutputDevice) RSSIThreshold() (int, bool) {
	return d.metaInt("rssi_threshold")
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `cd backend && go test ./internal/models/outputdevice/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/models/outputdevice/
git commit -m "$(printf 'feat(tra-943): output-device metadata accessors (mode, age-out, rssi)\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 3: Drop `is_boundary` from scan_point model + storage

**Files:**
- Modify: `backend/internal/models/scanpoint/scanpoint.go` (remove the 3 `IsBoundary` fields: lines ~16, ~32, ~42)
- Modify: `backend/internal/storage/scan_points.go` (columns line ~13, scan line ~18, create default ~30-32 + insert column ~41 + values ~48, update ~133-134)
- Modify: `backend/internal/storage/scan_devices.go` (insert column list line ~64 + its VALUES + scan, wherever is_boundary appears)
- Test: existing `backend/internal/storage/*_test.go` and `backend/internal/handlers/scanpoints/*_test.go` (remove is_boundary references)

- [ ] **Step 1: Remove the model fields**

In `scanpoint.go` delete the `IsBoundary` field from the `ScanPoint` struct, the `CreateScanPointRequest`, and the `UpdateScanPointRequest`.

- [ ] **Step 2: Remove from storage queries**

In `scan_points.go`:
- Remove `is_boundary,` from the shared column list (line ~13).
- Remove `&p.IsBoundary,` from the scan target (line ~18).
- Remove the `isBoundary` default block (lines ~30-32), drop `is_boundary` from the INSERT column list (~41) and its positional value (~48). Renumber the `$N` placeholders to stay contiguous.
- Remove the `if req.IsBoundary != nil { add("is_boundary", ...) }` block (~133-134).

In `scan_devices.go`: remove `is_boundary` from the INSERT at line ~64 (and its VALUES placeholder + arg). Renumber placeholders.

- [ ] **Step 3: Update tests that reference is_boundary**

Run: `cd backend && grep -rln "IsBoundary\|is_boundary" internal/ --include=*_test.go`
For each hit, delete the `IsBoundary`/`is_boundary` assertion or struct field. (Scan-point CRUD tests assert the round-trip; just drop the field.)

- [ ] **Step 4: Build to verify (geofence engine will still fail to compile — that's Task 7)**

Run: `cd backend && go build ./internal/models/... ./internal/storage/... ./internal/handlers/scanpoints/...`
Expected: PASS (these packages compile without is_boundary). Note `./internal/geofence/...` and `./internal/storage` (ingest.go) still reference IsBoundary until Task 4 — that's expected; build only the listed packages here.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/models/scanpoint/ backend/internal/storage/scan_points.go backend/internal/storage/scan_devices.go backend/internal/handlers/scanpoints/
git commit -m "$(printf 'feat(tra-943): remove is_boundary from scan_point model + storage\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 4: Drop `is_boundary` + `rssi_threshold` from `ResolvedRead` + `PersistReads`

**Files:**
- Modify: `backend/internal/storage/ingest.go` (ResolvedRead lines ~66-77; PersistReads query + scan + struct build lines ~93-139)
- Test: `backend/internal/storage/ingest_test.go` if it references the dropped fields

- [ ] **Step 1: Trim the ResolvedRead struct**

In `ingest.go`, change `ResolvedRead` to drop `IsBoundary` and `RSSIThresholdRaw`:
```go
type ResolvedRead struct {
	AssetID     int
	ScanPointID int
	LocationID  *int
	EPC         string
	RSSI        int // scanread.Read.RSSI; 0 == parser sentinel for "no usable RSSI"
}
```

- [ ] **Step 2: Trim the scan_points query + scan + build**

In `PersistReads`, replace the scan_point lookup block:
```go
				var scanPointID int
				var locationID *int
				err := tx.QueryRow(ctx,
					`SELECT id, location_id
					 FROM trakrf.scan_points
					 WHERE org_id = $1 AND external_key = $2 AND deleted_at IS NULL`,
					orgID, rd.CapturePointName,
				).Scan(&scanPointID, &locationID)
```
And the Resolved append:
```go
				res.Resolved = append(res.Resolved, ResolvedRead{
					AssetID:     assetID,
					ScanPointID: scanPointID,
					LocationID:  locationID,
					EPC:         rd.EPC,
					RSSI:        rd.RSSI,
				})
```
(Remove the now-unused `isBoundary` / `rssiThresholdRaw` locals.)

- [ ] **Step 3: Update ingest tests**

Run: `cd backend && grep -rln "IsBoundary\|RSSIThresholdRaw\|is_boundary\|rssi_threshold" internal/storage/*_test.go`
Remove references in any storage test. (If an integration test seeds `scan_points` with `is_boundary`, drop that column from the INSERT.)

- [ ] **Step 4: Build storage**

Run: `cd backend && go build ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/ingest.go backend/internal/storage/
git commit -m "$(printf 'feat(tra-943): drop is_boundary + per-scan-point rssi from ResolvedRead/PersistReads\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 5: Per-output age-out in the latch

**Files:**
- Modify: `backend/internal/geofence/latch.go`
- Test: `backend/internal/geofence/latch_test.go` (create)

The latch becomes ttl-per-call (each output passes its own re-arm window). `keyFor` keys by output id, not scan point.

- [ ] **Step 1: Write failing tests**

`backend/internal/geofence/latch_test.go`:
```go
package geofence

import (
	"testing"
	"time"
)

func TestLatch_AdmitFirstThenLatchThenReArm(t *testing.T) {
	l := newLatch(0, NewFakeClock(time.Unix(0, 0))) // no sweep goroutine
	defer l.Close()
	k := latchKey(1, 2, "EPC")
	ttl := time.Minute
	base := time.Unix(1000, 0)

	if !l.admit(k, base, ttl) {
		t.Fatal("first sight must admit")
	}
	if l.admit(k, base.Add(10*time.Second), ttl) {
		t.Fatal("within ttl must be latched")
	}
	if !l.admit(k, base.Add(2*time.Minute), ttl) {
		t.Fatal("after ttl must re-arm")
	}
}

func TestLatch_PerCallTTL(t *testing.T) {
	l := newLatch(0, NewFakeClock(time.Unix(0, 0)))
	defer l.Close()
	k := latchKey(1, 2, "EPC")
	base := time.Unix(1000, 0)

	l.admit(k, base, 10*time.Second)
	// 30s later: re-arms under a 10s window, stays latched under a 60s window.
	if !l.admit(k, base.Add(30*time.Second), 10*time.Second) {
		t.Fatal("30s gap should re-arm a 10s ttl")
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `cd backend && go test ./internal/geofence/ -run TestLatch`
Expected: FAIL (latchKey undefined; admit signature mismatch).

- [ ] **Step 3: Implement per-call ttl**

In `latch.go`:
- Rename `keyFor` to `latchKey` and rename the middle param to `outputID` (format unchanged):
```go
// latchKey builds the egress dedup key, scoped per (org, output device, epc).
func latchKey(orgID, outputID int, epc string) string {
	return fmt.Sprintf("%d:%d:%s", orgID, outputID, epc)
}
```
- Add `ttlNanos` to the entry:
```go
type latchEntry struct {
	lastSeen atomic.Int64 // unix nanos of the most recent admit() call
	ttlNanos atomic.Int64 // re-arm window for this key (per-output age-out)
}
```
- Drop the `ttl` field from the `latch` struct and from `newLatch`'s signature:
```go
func newLatch(sweepInterval time.Duration, clk Clock) *latch {
	if clk == nil {
		clk = RealClock{}
	}
	l := &latch{
		clk:      clk,
		sweepInt: sweepInterval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go l.sweepLoop()
	return l
}
```
- `admit` takes the ttl per call and records it:
```go
func (l *latch) admit(key string, now time.Time, ttl time.Duration) bool {
	nowNanos := now.UnixNano()
	fresh := &latchEntry{}
	fresh.lastSeen.Store(nowNanos)
	fresh.ttlNanos.Store(ttl.Nanoseconds())
	v, loaded := l.seen.LoadOrStore(key, fresh)
	if !loaded {
		return true // first sight
	}
	e := v.(*latchEntry)
	e.ttlNanos.Store(ttl.Nanoseconds())
	prev := e.lastSeen.Swap(nowNanos)
	return nowNanos-prev > ttl.Nanoseconds() // re-armed after absence
}
```
- `sweep` uses per-entry ttl:
```go
func (l *latch) sweep() {
	now := l.clk.Now().UnixNano()
	l.seen.Range(func(k, v any) bool {
		e := v.(*latchEntry)
		if e.lastSeen.Load() < now-e.ttlNanos.Load() {
			l.seen.Delete(k)
		}
		return true
	})
}
```

- [ ] **Step 4: Run latch tests**

Run: `cd backend && go test ./internal/geofence/ -run TestLatch`
Expected: PASS. (The package as a whole won't build until Task 7 rewrites engine.go/engine_test.go; run with `-run TestLatch` is still gated by package compile — if the package doesn't compile yet, defer running this until Task 7 and note it. Prefer ordering: do Task 7 in the same working session so the package compiles.)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/geofence/latch.go backend/internal/geofence/latch_test.go
git commit -m "$(printf 'feat(tra-943): latch supports per-output age-out, keyed per output\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 6: Presence tracker (ON on first member, OFF on last age-out)

**Files:**
- Create: `backend/internal/geofence/presence.go`
- Test: `backend/internal/geofence/presence_test.go`

- [ ] **Step 1: Write the presence tracker**

`backend/internal/geofence/presence.go`:
```go
package geofence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// presence tracks, per (org, output device), the set of member EPCs currently
// "here" and their last-seen time. observe() reports the 0->1 edge (fire ON);
// the sweeper detects the 1->0 edge (last member aged out) and drives the output
// OFF. This is the level/presence archetype: RFID emits no "tag left" event, so
// departure is *defined* by the per-output age-out window.
type presence struct {
	mu      sync.Mutex
	outputs map[string]*presenceOutput // "org:outputID" -> state

	driver   outputDriver
	clk      Clock
	log      zerolog.Logger
	sweepInt time.Duration

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

type presenceOutput struct {
	dev     outputdevice.OutputDevice // snapshot, so the sweeper can drive OFF
	ttl     time.Duration
	members map[string]int64 // epc -> last-seen unix nanos
}

func newPresence(driver outputDriver, sweepInterval time.Duration, clk Clock, log zerolog.Logger) *presence {
	if clk == nil {
		clk = RealClock{}
	}
	p := &presence{
		outputs:  map[string]*presenceOutput{},
		driver:   driver,
		clk:      clk,
		log:      log,
		sweepInt: sweepInterval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go p.sweepLoop()
	return p
}

func presenceKey(orgID, outputID int) string {
	return fmt.Sprintf("%d:%d", orgID, outputID)
}

// observe records that a member EPC was seen at an output, refreshing the output
// snapshot + ttl. It returns true only on the 0->1 transition (the caller fires
// ON); subsequent reads while present return false.
func (p *presence) observe(orgID int, dev outputdevice.OutputDevice, ttl time.Duration, epc string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := presenceKey(orgID, dev.ID)
	po := p.outputs[k]
	if po == nil {
		po = &presenceOutput{members: map[string]int64{}}
		p.outputs[k] = po
	}
	po.dev = dev
	po.ttl = ttl
	was := len(po.members) > 0
	po.members[epc] = now.UnixNano()
	return !was
}

func (p *presence) sweepLoop() {
	defer close(p.done)
	if p.sweepInt <= 0 {
		<-p.stop
		return
	}
	t := time.NewTicker(p.sweepInt)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.sweep(context.Background())
		}
	}
}

// sweep ages out members past each output's ttl and drives OFF any output whose
// last member just left. Set calls happen outside the lock (best-effort).
func (p *presence) sweep(ctx context.Context) {
	p.mu.Lock()
	now := p.clk.Now().UnixNano()
	var cleared []outputdevice.OutputDevice
	for k, po := range p.outputs {
		was := len(po.members) > 0
		for epc, ls := range po.members {
			if ls < now-po.ttl.Nanoseconds() {
				delete(po.members, epc)
			}
		}
		if was && len(po.members) == 0 {
			cleared = append(cleared, po.dev)
			delete(p.outputs, k)
		}
	}
	p.mu.Unlock()

	for _, d := range cleared {
		if err := p.driver.Set(ctx, d, false, 0); err != nil {
			p.log.Error().Err(err).Int("output_device_id", d.ID).Msg("presence off failed (best-effort)")
		}
	}
}

// Close stops the sweeper. Safe to call multiple times.
func (p *presence) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done
	})
}
```

- [ ] **Step 2: Write failing tests**

`backend/internal/geofence/presence_test.go`:
```go
package geofence

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

func TestPresence_FirstObserveFiresOnReObserveDoesNot(t *testing.T) {
	d := &fakeDriver{}
	p := newPresence(d, 0, NewFakeClock(time.Unix(0, 0)), zerolog.New(io.Discard))
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	if !p.observe(1, dev, time.Minute, "EPC", time.Unix(100, 0)) {
		t.Fatal("first observe must report ON edge")
	}
	if p.observe(1, dev, time.Minute, "EPC", time.Unix(110, 0)) {
		t.Fatal("re-observe while present must not report a new ON edge")
	}
}

func TestPresence_SweepFiresOffWhenLastMemberAgesOut(t *testing.T) {
	d := &fakeDriver{}
	clk := NewFakeClock(time.Unix(100, 0))
	p := newPresence(d, 0, clk, zerolog.New(io.Discard))
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	p.observe(1, dev, 10*time.Second, "EPC", time.Unix(100, 0))

	// 5s later: still present, no OFF.
	clk.Set(time.Unix(105, 0))
	p.sweep(context.Background())
	if d.offCount() != 0 {
		t.Fatalf("member still present; expected 0 off, got %d", d.offCount())
	}

	// 20s after last seen: aged out -> exactly one OFF for device 5.
	clk.Set(time.Unix(120, 0))
	p.sweep(context.Background())
	if d.offCount() != 1 {
		t.Fatalf("aged-out member should drive one OFF, got %d", d.offCount())
	}
}
```

> `fakeDriver` and `clk.Set` are defined in Task 7's test helpers (`engine_test.go`) and the existing clock helper. If `NewFakeClock` has no `Set`, add a `Set(t time.Time)` method to the fake clock in `clock.go` (check `backend/internal/geofence/clock.go` first). `fakeDriver.offCount()` counts `Set(..., on=false, ...)` calls.

- [ ] **Step 3: Run to verify fail, then pass**

Run: `cd backend && go test ./internal/geofence/ -run TestPresence`
Expected: FAIL first (compile / undefined helpers), PASS after Task 7 helpers + any `clock.go` `Set` exist.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/geofence/presence.go backend/internal/geofence/presence_test.go backend/internal/geofence/clock.go
git commit -m "$(printf 'feat(tra-943): presence tracker fires OFF when last member ages out\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 7: Engine restructure — resolve outputs, per-output egress/presence

**Files:**
- Modify: `backend/internal/geofence/engine.go`
- Delete: `backend/internal/geofence/firer.go` (Firer interface + LogFirer no longer used)
- Delete: `backend/internal/alarm/firer.go` + `backend/internal/alarm/firer_test.go` (lookup+loop moves into the engine)
- Modify: `backend/internal/geofence/metrics.go` (add `no_location` suppression label is automatic via labels; ensure `metricFireErrors` help text still fits — optional)
- Rewrite: `backend/internal/geofence/engine_test.go`

- [ ] **Step 1: Rewrite the engine**

Replace `engine.go` interfaces + struct + constructor + Evaluate. New seams (`engineStore`, `outputDriver`) and the per-output loop:
```go
package geofence

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// engineStore is the storage surface the engine needs; *storage.Storage
// satisfies it. Narrowed so unit tests can inject a fake.
type engineStore interface {
	InsertAlarmEvent(ctx context.Context, orgID int, ev storage.AlarmEventRow) error
	ListOutputDevicesForLocation(ctx context.Context, orgID, locationID int) ([]outputdevice.OutputDevice, error)
}

// outputDriver drives one output device on/off via its transport; alarm.Dispatcher
// satisfies it. Defined here (not imported from alarm) to avoid an import cycle.
type outputDriver interface {
	Set(ctx context.Context, dev outputdevice.OutputDevice, on bool, offAfterSec int) error
}

// Engine evaluates resolved reads and drives output devices. All rule config
// (mode, age-out, rssi_threshold, auto_off) lives on output_device.metadata.
type Engine struct {
	cfg      Config
	store    engineStore
	driver   outputDriver
	latch    *latch    // egress dedup, keyed per (org, output, epc)
	presence *presence // presence tracker, keyed per (org, output)
	log      zerolog.Logger
}

// NewEngine builds an engine with real-clock latch + presence sweepers.
func NewEngine(cfg Config, store *storage.Storage, driver outputDriver, log *zerolog.Logger) *Engine {
	l := log.With().Str("component", "geofence").Logger()
	clk := RealClock{}
	return &Engine{
		cfg:      cfg,
		store:    store,
		driver:   driver,
		latch:    newLatch(cfg.SweepInterval, clk),
		presence: newPresence(driver, cfg.SweepInterval, clk, l),
		log:      l,
	}
}

func (e *Engine) Start() {}

func (e *Engine) Stop() {
	if e.latch != nil {
		e.latch.Close()
	}
	if e.presence != nil {
		e.presence.Close()
	}
}

// Evaluate runs the rule decision over every membership-passing read of one MQTT
// message. Never returns an error: side effects are best-effort. For each read it
// resolves the location's active output devices and applies each device's mode.
func (e *Engine) Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead) {
	for _, rd := range reads {
		metricEvaluated.Inc()

		if rd.RSSI == 0 { // parser sentinel: no usable RSSI
			metricSuppressed.WithLabelValues("no_rssi").Inc()
			continue
		}
		if rd.LocationID == nil { // outputs are location-bound
			metricSuppressed.WithLabelValues("no_location").Inc()
			continue
		}

		devices, err := e.store.ListOutputDevicesForLocation(ctx, orgID, *rd.LocationID)
		if err != nil {
			e.log.Error().Err(err).Int("org_id", orgID).Int("location_id", *rd.LocationID).Msg("output device lookup failed")
			metricFireErrors.Inc()
			continue
		}

		firedAny := false
		for _, dev := range devices {
			threshold := e.cfg.RSSIThreshold
			if t, ok := dev.RSSIThreshold(); ok {
				threshold = t
			}
			if rd.RSSI < threshold {
				metricSuppressed.WithLabelValues("rssi_below_threshold").Inc()
				continue
			}

			ttl := e.cfg.LatchTTL
			if s, ok := dev.AgeOutSeconds(); ok {
				ttl = time.Duration(s) * time.Second
			}

			if dev.Mode() == outputdevice.ModePresence {
				if e.presence.observe(orgID, dev, ttl, rd.EPC, receivedAt) {
					firedAny = true
					e.drive(ctx, orgID, dev, true, 0) // presence ignores auto_off
				}
				continue
			}

			// Egress: fire ON, then latch for the per-output re-arm window.
			if !e.latch.admit(latchKey(orgID, dev.ID, rd.EPC), receivedAt, ttl) {
				metricSuppressed.WithLabelValues("latched").Inc()
				continue
			}
			firedAny = true
			e.drive(ctx, orgID, dev, true, dev.AutoOffSeconds())
		}

		if firedAny {
			e.recordFire(ctx, orgID, tagScanID, receivedAt, rd)
		}
	}
}

func (e *Engine) drive(ctx context.Context, orgID int, dev outputdevice.OutputDevice, on bool, offAfter int) {
	if err := e.driver.Set(ctx, dev, on, offAfter); err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Int("output_device_id", dev.ID).Bool("on", on).Msg("output device drive failed (best-effort)")
		metricFireErrors.Inc()
	}
}

func (e *Engine) recordFire(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, rd storage.ResolvedRead) {
	if err := e.store.InsertAlarmEvent(ctx, orgID, storage.AlarmEventRow{
		AssetID:     rd.AssetID,
		ScanPointID: rd.ScanPointID,
		LocationID:  rd.LocationID,
		EPC:         rd.EPC,
		RSSI:        rd.RSSI,
		TagScanID:   tagScanID,
		FiredAt:     receivedAt,
	}); err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Str("epc", rd.EPC).Msg("alarm_events write failed")
		metricEventWriteErrors.Inc()
	}
	metricFired.Inc()
	e.log.Info().Int("org_id", orgID).Int("asset_id", rd.AssetID).Str("epc", rd.EPC).Int("rssi", rd.RSSI).Msg("geofence rule fired")
}
```
Remove the old `AlarmEvent` struct, `alarmWriter` interface, `thresholdFor`, and the `strconv` import (no longer used). Keep `storage` import.

- [ ] **Step 2: Delete the obsolete firer files**

```bash
git rm backend/internal/geofence/firer.go backend/internal/alarm/firer.go backend/internal/alarm/firer_test.go
```

- [ ] **Step 3: Rewrite engine_test.go with new fakes**

`backend/internal/geofence/engine_test.go` — replace fakes + tests:
```go
package geofence

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

type fakeStore struct {
	rows      []storage.AlarmEventRow
	devices   []outputdevice.OutputDevice
	insertErr error
	listErr   error
}

func (s *fakeStore) InsertAlarmEvent(_ context.Context, _ int, ev storage.AlarmEventRow) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.rows = append(s.rows, ev)
	return nil
}

func (s *fakeStore) ListOutputDevicesForLocation(_ context.Context, _, _ int) ([]outputdevice.OutputDevice, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.devices, nil
}

type setCall struct {
	dev      outputdevice.OutputDevice
	on       bool
	offAfter int
}

type fakeDriver struct {
	mu    sync.Mutex
	calls []setCall
	err   error
}

func (d *fakeDriver) Set(_ context.Context, dev outputdevice.OutputDevice, on bool, offAfter int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.calls = append(d.calls, setCall{dev, on, offAfter})
	return nil
}

func (d *fakeDriver) onCount() int  { return d.count(true) }
func (d *fakeDriver) offCount() int { return d.count(false) }
func (d *fakeDriver) count(on bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, c := range d.calls {
		if c.on == on {
			n++
		}
	}
	return n
}

func newTestEngine(cfg Config, s engineStore, d outputDriver) *Engine {
	log := zerolog.New(io.Discard)
	clk := NewFakeClock(time.Unix(0, 0))
	return &Engine{
		cfg:      cfg,
		store:    s,
		driver:   d,
		latch:    newLatch(0, clk),
		presence: newPresence(d, 0, clk, log),
		log:      log,
	}
}

func ptr(i int) *int { return &i }

func read(epc string, rssi int) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: 7, ScanPointID: 3, LocationID: ptr(9), EPC: epc, RSSI: rssi}
}

func egressDev(id int, meta map[string]any) outputdevice.OutputDevice {
	return outputdevice.OutputDevice{ID: id, Metadata: meta}
}

func TestEvaluate_EgressFiresOnAndRecordsEvent(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{egressDev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	e.Evaluate(context.Background(), 42, 555, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})

	if d.onCount() != 1 || len(s.rows) != 1 {
		t.Fatalf("expected one ON + one alarm row, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
}

func TestEvaluate_SuppressBelowThresholdNoRSSINoLocation(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{egressDev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	noLoc := read("NOLOC", -50)
	noLoc.LocationID = nil
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{
		read("WEAK", -80), read("ZERO", 0), noLoc,
	})
	if d.onCount() != 0 {
		t.Fatalf("weak/no-rssi/no-location reads must not fire, got %d", d.onCount())
	}
}

func TestEvaluate_PerOutputRSSIOverride(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{egressDev(11, map[string]any{"rssi_threshold": float64(-55)})}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	// -60 passes global -65 but fails the stricter per-output -55.
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -60)})
	if d.onCount() != 0 {
		t.Fatalf("per-output -55 override must suppress a -60 read")
	}
}

func TestEvaluate_EgressLatchPerOutputThenReArm(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{egressDev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	base := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(10*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 1 {
		t.Fatalf("repeat within ttl must be latched, got %d", d.onCount())
	}
	e.Evaluate(context.Background(), 42, 3, base.Add(2*time.Minute), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 2 {
		t.Fatalf("re-entry after ttl must re-fire, got %d", d.onCount())
	}
}

func TestEvaluate_PresenceFiresOnceAutoOffIgnored(t *testing.T) {
	dev := egressDev(11, map[string]any{"mode": "presence", "auto_off_seconds": float64(99)})
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	base := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 1 {
		t.Fatalf("presence should fire ON once for a continuously present tag, got %d", d.onCount())
	}
	// auto_off must be ignored in presence (offAfter == 0).
	if d.calls[0].offAfter != 0 {
		t.Fatalf("presence ON must pass offAfter=0, got %d", d.calls[0].offAfter)
	}
}

func TestEvaluate_BestEffortOnErrors(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{egressDev(11, nil)}, insertErr: errors.New("db down")}
	d := &fakeDriver{err: errors.New("device offline")}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	// Must not panic.
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})
}
```

- [ ] **Step 4: Confirm `metrics.go` has the labels used**

`metricSuppressed` is a `*prometheus.CounterVec` with one label; `no_location` is just a new value, no code change needed. Verify `metricFireErrors`, `metricFired`, `metricEventWriteErrors`, `metricEvaluated`, `metricSuppressed` all still exist. No rename required.

- [ ] **Step 5: Run the full geofence package**

Run: `cd backend && go test ./internal/geofence/...`
Expected: PASS (engine + latch + presence).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/geofence/ backend/internal/alarm/
git commit -m "$(printf 'feat(tra-943): engine resolves outputs, per-output egress/presence modes\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 8: Wire the engine in serve.go

**Files:**
- Modify: `backend/internal/cmd/serve/serve.go:117-118`

- [ ] **Step 1: Pass the dispatcher directly as the driver**

Replace line ~118:
```go
		geofenceEngine := geofence.NewEngine(geofence.ConfigFromEnv(), store, alarmDispatcher, log)
```
Update the comment above to drop the `alarm.Firer` reference (the engine now drives the Dispatcher directly).

- [ ] **Step 2: Build the whole backend**

Run: `cd backend && go build ./...`
Expected: PASS (no remaining references to `alarm.NewFirer` / `geofence.Firer` / `geofence.LogFirer`).
If a reference remains, `grep -rn "NewFirer\|geofence.Firer\|LogFirer" backend/` and remove it.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/cmd/serve/serve.go
git commit -m "$(printf 'feat(tra-943): wire geofence engine to dispatcher directly\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 9: Backend full validation

- [ ] **Step 1: Full test + lint**

Run: `just backend test && just backend lint`
Expected: PASS. Fix any compile/test fallout from is_boundary removal (grep `git grep -n is_boundary backend/` should return only the 000018 + 000011 migration files).

- [ ] **Step 2: Commit any fixups**

```bash
git add -A backend/
git commit -m "$(printf 'test(tra-943): backend fixups after is_boundary removal\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')" || echo "nothing to commit"
```

---

## Task 10: Output-device form — mode, age-out, auto-off, RSSI fields

**Files:**
- Modify: `frontend/src/types/outputdevices/index.ts` (add `OutputDeviceMode`)
- Modify: `frontend/src/components/outputdevices/OutputDeviceForm.tsx`
- Test: `frontend/src/components/outputdevices/OutputDeviceFormModal.test.tsx`

- [ ] **Step 1: Add the mode type**

In `types/outputdevices/index.ts` add:
```ts
export type OutputDeviceMode = 'egress' | 'presence';
```

- [ ] **Step 2: Write failing tests**

In `OutputDeviceFormModal.test.tsx`, add cases (match existing render/submit helpers in that file):
```tsx
it('renders the rule-config fields', () => {
  // render the form in create mode (use the file's existing render helper)
  expect(screen.getByLabelText(/mode/i)).toBeInTheDocument();
  expect(screen.getByLabelText(/age-?out/i)).toBeInTheDocument();
  expect(screen.getByLabelText(/auto-?off/i)).toBeInTheDocument();
  expect(screen.getByLabelText(/rssi/i)).toBeInTheDocument();
});

it('disables auto-off when mode is presence', async () => {
  // render, select Presence in the mode dropdown
  await userEvent.selectOptions(screen.getByLabelText(/mode/i), 'presence');
  expect(screen.getByLabelText(/auto-?off/i)).toBeDisabled();
});

it('submits metadata with the rule fields', async () => {
  const onSubmit = vi.fn();
  // render with onSubmit, fill name + base_url, set age-out=30, rssi=-60, submit
  // expect onSubmit called with metadata: { mode:'egress', age_out_seconds:30, auto_off_seconds: <n|absent>, rssi_threshold:-60 }
  expect(onSubmit).toHaveBeenCalledWith(
    expect.objectContaining({
      metadata: expect.objectContaining({ age_out_seconds: 30, rssi_threshold: -60 }),
    })
  );
});
```
> Use the existing test file's render harness (it already mounts `OutputDeviceFormModal`). Mock `useLocations` as the file already does.

- [ ] **Step 3: Run to verify fail**

Run: `cd frontend && pnpm test OutputDeviceFormModal`
Expected: FAIL (fields not present).

- [ ] **Step 4: Implement the form fields**

In `OutputDeviceForm.tsx`:

Extend `OutputDeviceFormData` (after `is_active`):
```ts
  mode: OutputDeviceMode;
  age_out_seconds: string; // blank -> omit (system default)
  auto_off_seconds: string; // blank/0 -> latch until manual reset
  rssi_threshold: string; // dBm; blank -> omit (system default). May be negative.
```
Add `OutputDeviceMode` to the type import. Add to `FieldErrors`:
```ts
  age_out_seconds?: string;
  auto_off_seconds?: string;
  rssi_threshold?: string;
```
Extend `EMPTY_FORM`:
```ts
  mode: 'egress',
  age_out_seconds: '',
  auto_off_seconds: '',
  rssi_threshold: '',
```
In the edit-init `useEffect`, read from `device.metadata` (typed `Record<string, unknown>`):
```ts
        mode: (device.metadata?.mode as OutputDeviceMode) === 'presence' ? 'presence' : 'egress',
        age_out_seconds: metaNum(device.metadata?.age_out_seconds),
        auto_off_seconds: metaNum(device.metadata?.auto_off_seconds),
        rssi_threshold: metaNum(device.metadata?.rssi_threshold),
```
Add a module-level helper near `validateBaseURL`:
```ts
// metaNum renders a metadata numeric value as a form string ('' when unset).
function metaNum(v: unknown): string {
  return typeof v === 'number' ? String(v) : '';
}
// integer (optionally negative); '' allowed (means "unset").
function validateOptInt(s: string, opts: { allowNegative?: boolean }): string | null {
  const t = s.trim();
  if (t === '') return null;
  const re = opts.allowNegative ? /^-?\d+$/ : /^\d+$/;
  if (!re.test(t)) return opts.allowNegative ? 'Must be an integer' : 'Must be a non-negative integer';
  return null;
}
```
In `validateForm`, after the switch_id check:
```ts
    const ageErr = validateOptInt(formData.age_out_seconds, {});
    if (ageErr) errors.age_out_seconds = ageErr;
    if (formData.mode === 'egress') {
      const offErr = validateOptInt(formData.auto_off_seconds, {});
      if (offErr) errors.auto_off_seconds = offErr;
    }
    const rssiErr = validateOptInt(formData.rssi_threshold, { allowNegative: true });
    if (rssiErr) errors.rssi_threshold = rssiErr;
```
In `handleSubmit`, build the metadata object and add to `common`:
```ts
    const metadata: Record<string, number | string> = { mode: formData.mode };
    if (formData.age_out_seconds.trim() !== '') metadata.age_out_seconds = parseInt(formData.age_out_seconds.trim(), 10);
    if (formData.rssi_threshold.trim() !== '') metadata.rssi_threshold = parseInt(formData.rssi_threshold.trim(), 10);
    // auto_off only applies to egress (presence owns the OFF edge).
    if (formData.mode === 'egress' && formData.auto_off_seconds.trim() !== '') {
      metadata.auto_off_seconds = parseInt(formData.auto_off_seconds.trim(), 10);
    }
```
Add `metadata` to the `common` object literal.

Render the fields — add a new grid block after the Switch ID / Location grid (before the `is_active` checkbox). Mode select + a 3-up grid of numeric inputs:
```tsx
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <label htmlFor="mode" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Mode
          </label>
          <select
            id="mode"
            value={formData.mode}
            onChange={(e) => handleChange('mode', e.target.value as OutputDeviceMode)}
            disabled={loading}
            className={inputClass(false)}
          >
            <option value="egress">Egress — fire on crossing, then latch</option>
            <option value="presence">Presence — on while present, off when clear</option>
          </select>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Egress alerts when an asset crosses. Presence stays on while an asset is here and clears it when the last one leaves.
          </p>
        </div>

        <div>
          <label htmlFor="rssi_threshold" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            RSSI threshold (dBm)
          </label>
          <input
            type="number"
            id="rssi_threshold"
            value={formData.rssi_threshold}
            onChange={(e) => handleChange('rssi_threshold', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.rssi_threshold)}
            placeholder="System default"
          />
          {fieldErrors.rssi_threshold && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.rssi_threshold}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Minimum signal strength for this output to react (stronger is closer to 0). Blank = system default.
          </p>
        </div>

        <div>
          <label htmlFor="age_out_seconds" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Age-out (seconds)
          </label>
          <input
            type="number"
            id="age_out_seconds"
            min={0}
            value={formData.age_out_seconds}
            onChange={(e) => handleChange('age_out_seconds', e.target.value)}
            disabled={loading}
            className={inputClass(!!fieldErrors.age_out_seconds)}
            placeholder="System default"
          />
          {fieldErrors.age_out_seconds && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.age_out_seconds}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {formData.mode === 'presence'
              ? 'How long after the last read before the output clears.'
              : 'Re-arm window before the same tag can fire again.'}
          </p>
        </div>

        <div>
          <label htmlFor="auto_off_seconds" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Auto-off (seconds)
          </label>
          <input
            type="number"
            id="auto_off_seconds"
            min={0}
            value={formData.mode === 'presence' ? '' : formData.auto_off_seconds}
            onChange={(e) => handleChange('auto_off_seconds', e.target.value)}
            disabled={loading || formData.mode === 'presence'}
            className={inputClass(!!fieldErrors.auto_off_seconds)}
            placeholder="0 = until manual reset"
          />
          {fieldErrors.auto_off_seconds && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{fieldErrors.auto_off_seconds}</p>
          )}
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {formData.mode === 'presence'
              ? 'Managed automatically by presence detection.'
              : 'Device flips itself off after N seconds. 0 or blank = stay on until manual reset.'}
          </p>
        </div>
      </div>
```

- [ ] **Step 5: Run to verify pass**

Run: `cd frontend && pnpm test OutputDeviceFormModal && pnpm typecheck`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/outputdevices/ frontend/src/types/outputdevices/
git commit -m "$(printf 'feat(tra-935): output-device form fields for mode, age-out, auto-off, rssi\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 11: Remove `is_boundary` from the frontend

**Files:**
- Modify: `frontend/src/types/scandevices/index.ts` (lines ~72, ~88, ~98)
- Modify: `frontend/src/components/scandevices/ScanPointForm.tsx` (form data ~27/~51, init ~80, submit ~142/~152, the checkbox JSX ~262-272)
- Modify: `frontend/src/components/scandevices/SinglePointLocationField.tsx` (line ~64 — remove the `is_boundary: true` auto-set)
- Modify: `frontend/src/components/scandevices/ScanPointsPanel.tsx` (table column ~125 and its header)
- Test: `SinglePointLocationField.test.tsx`, `ScanPointsPanel.test.tsx`, `ScanPointForm` test (remove is_boundary)

- [ ] **Step 1: Remove from types**

Delete the `is_boundary` field from `ScanPoint`, `CreateScanPointRequest`, `UpdateScanPointRequest` in `types/scandevices/index.ts`.

- [ ] **Step 2: Remove from ScanPointForm**

Delete `is_boundary` from the form-data interface, `EMPTY` default, edit-init, both submit payload spots, and the checkbox JSX block (the `<input id="sp_is_boundary">` and its label).

- [ ] **Step 3: Remove from SinglePointLocationField + ScanPointsPanel**

In `SinglePointLocationField.tsx` remove the `is_boundary: true` key from the create payload (line ~64). In `ScanPointsPanel.tsx` remove the boundary `<td>` cell (line ~125) and its corresponding `<th>` header.

- [ ] **Step 4: Remove from tests**

Run: `cd frontend && grep -rln "is_boundary\|isBoundary" src/`
For each test hit, drop the `is_boundary` fixture key / assertion.

- [ ] **Step 5: Run to verify**

Run: `cd frontend && pnpm test scandevices && pnpm typecheck`
Expected: PASS. `grep -rn is_boundary frontend/src` returns nothing.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/
git commit -m "$(printf 'feat(tra-943): remove is_boundary from scan-point UI + types\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Task 12: Full validation + PR

- [ ] **Step 1: Run the full suite both workspaces**

Run: `just validate`
Expected: PASS (lint + typecheck + tests, frontend + backend).

- [ ] **Step 2: Final sanity grep**

Run: `git grep -n "is_boundary\|IsBoundary\|RSSIThresholdRaw\|alarm.NewFirer\|LogFirer"`
Expected: only the `000011` + `000018` migration files mention `is_boundary`; nothing else.

- [ ] **Step 3: Push + open PR (stop here for review)**

```bash
git push -u origin feat/tra-943-output-event-rules
gh pr create --base main --title "feat(tra-943/935): output event rules — egress/presence modes + visible config" --body "$(cat <<'EOF'
## Summary
Implements the TRA-943 demo slice + TRA-935. Per-output rule **mode** (egress|presence), configurable **age-out**, and visible rule config (**auto-off**, **RSSI threshold**) on `output_device.metadata`. Drops the misaligned `scan_points.is_boundary` gate/column and the hidden per-scan-point `rssi_threshold` knob.

- **Egress** (unchanged semantics, re-keyed per output): fire ON on a crossing, latch for the per-output re-arm window, device-side auto-off via TRA-934.
- **Presence** (new): ON while ≥1 member tag is present, OFF when the last ages out (latch sweep drives the OFF edge). Auto-off ignored — the engine owns OFF.
- Engine now resolves `location → outputs` up front and keys dedup/presence per `(output, epc)`.
- `is_boundary` removed end-to-end (migration `000018`, struct/query/UI). Greenfield/unreleased — no data migration.

Full rule-object decoupling (b) captured as future direction in the spec.

Spec: `docs/superpowers/specs/2026-06-06-tra-943-output-event-rules-design.md`
Plan: `docs/superpowers/plans/2026-06-06-tra-943-output-event-rules.md`

Output devices are internal-only → no docs PR.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 4: Report the PR URL and stop for Mike's diff review.**

---

## Self-review notes

- **Spec coverage:** mode (Task 2/7/10), age-out (Task 5/7/10), rssi-on-output (Task 2/7/10), auto-off UI (Task 10), is_boundary removal (Task 1/3/4/7/11), presence OFF edge (Task 6/7), engine restructure (Task 7), wiring (Task 8). All spec sections mapped.
- **Ordering caveat:** the `geofence` package will not compile between Tasks 4 and 7. Execute Tasks 5–7 as a unit (latch + presence + engine) before running geofence package tests; the per-task "run tests" steps inside 5/6 are validated at the end of Task 7. Commit boundaries are still per-task.
- **Type consistency:** `latchKey` (not `keyFor`), `outputDriver.Set(ctx, dev, on, offAfterSec)`, `engineStore`, `ModeEgress/ModePresence`, `AgeOutSeconds() (int,bool)`, `RSSIThreshold() (int,bool)` used consistently across tasks.
- **`clock.go` dependency:** Task 6 presence tests need a settable fake clock (`clk.Set`). Verify `NewFakeClock` API first; add a `Set` method if missing.
