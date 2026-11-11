package geofence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	dto "github.com/prometheus/client_model/go"

	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// stalledStore is a store whose lookups hang until their context ends. stallCap
// stops a lookup with no deadline from hanging the test run.
type stalledStore struct{ fakeStore }

const stallCap = 10 * lookupTimeout

func stall(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(stallCap):
		return errors.New("lookup had no deadline")
	}
}

func (s *stalledStore) GetOrgGeofenceDefaults(ctx context.Context, _ int) (organization.GeofenceDefaults, error) {
	return organization.GeofenceDefaults{}, stall(ctx)
}

func (s *stalledStore) ListOutputDevicesForLocation(ctx context.Context, _, _ int) ([]outputdevice.OutputDevice, error) {
	return nil, stall(ctx)
}

// countingStore counts device lookups per location; failing locations return
// an error.
type countingStore struct {
	fakeStore
	mu      sync.Mutex
	lookups map[int]int
	failing map[int]bool
}

func (s *countingStore) ListOutputDevicesForLocation(_ context.Context, _, locationID int) ([]outputdevice.OutputDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lookups == nil {
		s.lookups = map[int]int{}
	}
	s.lookups[locationID]++
	if s.failing[locationID] {
		return nil, errors.New("lookup failed")
	}
	return s.devices, nil
}

func readAt(epc string, locationID int) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: 7, ScanPointID: 3, LocationID: ptr(locationID), EPC: epc, RSSI: -50}
}

func TestDecide_LookupTimeoutDoesNotBlock(t *testing.T) {
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, &stalledStore{}, &fakeDriver{})
	defer e.Stop()

	// The caller's context stands in for the one tx queries use.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errsBefore := fireErrors(t)
	start := time.Now()
	fires, _ := e.decide(ctx, 42, time.Unix(1000, 0), reads(readAt("EPC1", 9), readAt("EPC2", 9), readAt("EPC3", 9)))
	elapsed := time.Since(start)

	if len(fires) != 0 {
		t.Fatalf("a read with no devices cannot fire, got %d", len(fires))
	}
	// One stalled defaults lookup and one stalled device lookup. A timeout is
	// remembered, so the other reads at that location must not wait again.
	if limit := 2*lookupTimeout + lookupTimeout/2; elapsed > limit {
		t.Fatalf("decide took %v, want under %v", elapsed, limit)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("a lookup timeout must not touch the caller's context: %v", err)
	}
	if got := fireErrors(t) - errsBefore; got != 1 {
		t.Fatalf("one failed lookup must count one error, got %v", got)
	}
}

func TestDecide_LooksUpEachLocationOnce(t *testing.T) {
	s := &countingStore{
		fakeStore: fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}},
		failing:   map[int]bool{30: true},
	}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	fires, _ := e.decide(context.Background(), 42, time.Unix(1000, 0), reads(
		readAt("EPC1", 10), readAt("EPC2", 20), readAt("EPC3", 10),
		readAt("EPC4", 30), readAt("EPC5", 30), readAt("EPC6", 20),
	))

	if len(fires) != 4 {
		t.Fatalf("every read at a working location must fire, got %d", len(fires))
	}
	for _, loc := range []int{10, 20, 30} {
		if s.lookups[loc] != 1 {
			t.Fatalf("location %d looked up %d times, want 1", loc, s.lookups[loc])
		}
	}

	// The memo lasts one decide call, so device edits apply on the next message.
	e.decide(context.Background(), 42, time.Unix(1001, 0), reads(readAt("EPC1", 10)))
	if s.lookups[10] != 2 {
		t.Fatalf("a new decide must look up again, got %d lookups", s.lookups[10])
	}
}

// fakeAlarmTx records alarm writes. It fails the failAt-th one and panics on the
// panicAt-th one (both 1-based).
type fakeAlarmTx struct {
	rows     []storage.AlarmEventRow
	attempts int
	failAt   int
	panicAt  int
}

func (s *fakeAlarmTx) InsertAlarmEventTx(_ context.Context, _ pgx.Tx, _ int, ev storage.AlarmEventRow) (int, error) {
	s.attempts++
	if s.attempts == s.panicAt {
		panic("alarm write panicked")
	}
	if s.attempts == s.failAt {
		return 0, errors.New("alarm write failed")
	}
	s.rows = append(s.rows, ev)
	return 100 + len(s.rows), nil
}

// fakeAlarmRecorder records the alarm ids it is given, or fails every call.
type fakeAlarmRecorder struct {
	orgs, ids []int
	err       error
}

func (r *fakeAlarmRecorder) RecordAlarm(_ context.Context, _ pgx.Tx, orgID, alarmID int) error {
	if r.err != nil {
		return r.err
	}
	r.orgs = append(r.orgs, orgID)
	r.ids = append(r.ids, alarmID)
	return nil
}

type stageRig struct {
	engine *Engine
	stage  *Stage
	driver *fakeDriver
	alarms *fakeAlarmTx
	rec    *fakeAlarmRecorder
}

func newStageRig(t *testing.T, devices ...outputdevice.OutputDevice) *stageRig {
	t.Helper()
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, &fakeStore{devices: devices}, d)
	t.Cleanup(e.Stop)
	r := &stageRig{engine: e, driver: d, alarms: &fakeAlarmTx{}, rec: &fakeAlarmRecorder{}}
	r.stage = e.Stage(r.alarms, r.rec)
	return r
}

func detectInput(at time.Time, rs ...storage.ResolvedRead) storage.DetectInput {
	return storage.DetectInput{OrgID: 42, TagScanID: 5, ReceivedAt: at, Reads: rs}
}

func (r *stageRig) detect(t *testing.T, in storage.DetectInput) detection.Pending {
	t.Helper()
	pending, err := r.stage.Detect(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return pending
}

func firedTotal(t *testing.T) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := metricFired.Write(m); err != nil {
		t.Fatalf("read fired counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

var stageAt = time.Unix(1000, 0)

func TestStage_WritesAlarmAndTrigger(t *testing.T) {
	r := newStageRig(t, dev(11, pulse5s))

	if got := r.stage.Name(); got != "geofence" {
		t.Fatalf("Name() = %q", got)
	}
	r.detect(t, detectInput(stageAt, read("EPC1", -50)))

	if len(r.alarms.rows) != 1 {
		t.Fatalf("want one alarm row, got %d", len(r.alarms.rows))
	}
	row := r.alarms.rows[0]
	if row.EPC != "EPC1" || row.TagScanID != 5 || !row.FiredAt.Equal(stageAt) {
		t.Fatalf("alarm row does not describe the read: %+v", row)
	}
	if len(r.rec.ids) != 1 || r.rec.ids[0] != 101 || r.rec.orgs[0] != 42 {
		t.Fatalf("RecordAlarm must get the row's id and org, got ids=%v orgs=%v", r.rec.ids, r.rec.orgs)
	}
	if r.driver.onCount() != 0 {
		t.Fatalf("Detect must not drive, got %d", r.driver.onCount())
	}
}

func TestStage_WritesAlarmAndTriggerForAFireWithNoDrive(t *testing.T) {
	r := newStageRig(t, dev(11, pulse5s))

	pending := r.detect(t, detectInput(stageAt, read("EPC1", -50), read("EPC2", -50)))

	if len(r.alarms.rows) != 2 || len(r.rec.ids) != 2 {
		t.Fatalf("a tag dropped by a busy output still records: rows=%d triggers=%d", len(r.alarms.rows), len(r.rec.ids))
	}
	pending.OnCommit(true)
	if r.driver.onCount() != 1 {
		t.Fatalf("only the fire holding the pulse drives, got %d", r.driver.onCount())
	}
}

func TestStage_DrivesAfterCommit(t *testing.T) {
	r := newStageRig(t, dev(11, pulse5s))
	before := firedTotal(t)

	pending := r.detect(t, detectInput(stageAt, read("EPC1", -50)))
	pending.OnCommit(true)

	if r.driver.onCount() != 1 {
		t.Fatalf("commit must drive the output, got %d", r.driver.onCount())
	}
	if got := r.driver.first().offAfter; got != 5 {
		t.Fatalf("the drive must carry the auto-off, got %d", got)
	}
	if got := firedTotal(t) - before; got != 1 {
		t.Fatalf("a committed alarm counts once, got %v", got)
	}
}

func TestStage_AlarmWriteFailureStillDrives(t *testing.T) {
	r := newStageRig(t, dev(11, nil))
	r.alarms.failAt = 2
	before := firedTotal(t)

	pending, err := r.stage.Detect(context.Background(), nil,
		detectInput(stageAt, read("EPC1", -50), read("EPC2", -50), read("EPC3", -50)))

	if err == nil {
		t.Fatal("a failed write must return its error so the savepoint rolls back")
	}
	if r.alarms.attempts != 2 || len(r.rec.ids) != 1 {
		t.Fatalf("writing must stop at the failure: attempts=%d triggers=%d", r.alarms.attempts, len(r.rec.ids))
	}
	if r.driver.onCount() != 0 {
		t.Fatalf("Detect must not drive, got %d", r.driver.onCount())
	}
	pending.OnCommit(false)
	if r.driver.onCount() != 3 {
		t.Fatalf("every fire must still drive, including unwritten ones, got %d", r.driver.onCount())
	}
	if got := firedTotal(t) - before; got != 0 {
		t.Fatalf("no alarm was kept, so none counts, got %v", got)
	}
}

func TestStage_WritePanicStillDrives(t *testing.T) {
	r := newStageRig(t, dev(11, nil))
	r.alarms.panicAt = 2

	pending, err := r.stage.Detect(context.Background(), nil,
		detectInput(stageAt, read("EPC1", -50), read("EPC2", -50), read("EPC3", -50)))

	if err == nil {
		t.Fatal("a panic must return an error so the savepoint rolls back")
	}
	pending.OnCommit(false)
	if r.driver.onCount() != 3 {
		t.Fatalf("every fire must still drive, including unwritten ones, got %d", r.driver.onCount())
	}
}

func TestStage_WritePanicThenRollbackLetsTheNextReadFire(t *testing.T) {
	r := newStageRig(t, dev(11, nil))
	r.alarms.panicAt = 1

	pending, err := r.stage.Detect(context.Background(), nil, detectInput(stageAt, read("EPC1", -50)))
	if err == nil {
		t.Fatal("a panic must return an error so the savepoint rolls back")
	}
	pending.OnRollback()

	next := r.detect(t, detectInput(stageAt.Add(time.Second), read("EPC1", -50)))
	next.OnCommit(true)
	if len(r.alarms.rows) != 1 || r.driver.onCount() != 1 {
		t.Fatalf("the latch must be restored: rows=%d drives=%d", len(r.alarms.rows), r.driver.onCount())
	}
}

func TestStage_TriggerFailureStillDrives(t *testing.T) {
	r := newStageRig(t, dev(11, nil))
	r.rec.err = errors.New("enqueue failed")

	pending, err := r.stage.Detect(context.Background(), nil, detectInput(stageAt, read("EPC1", -50), read("EPC2", -50)))

	if err == nil {
		t.Fatal("a failed enqueue must return its error so the alarm row rolls back with it")
	}
	if r.alarms.attempts != 1 {
		t.Fatalf("writing must stop at the failure, got %d attempts", r.alarms.attempts)
	}
	pending.OnCommit(false)
	if r.driver.onCount() != 2 {
		t.Fatalf("every fire must still drive, got %d", r.driver.onCount())
	}
}

// The pipeline can lose a stage's rows after Detect returned nil, when releasing
// the savepoint fails, so rowsKept alone decides what OnCommit does.
func TestStage_RowsLostAfterASuccessfulDetectSettleAsAFailedWrite(t *testing.T) {
	r := newStageRig(t, dev(11, pulse5s))

	pending := r.detect(t, detectInput(stageAt, read("EPC1", -50), read("EPC2", -50)))
	pending.OnCommit(false)

	if r.driver.onCount() != 1 {
		t.Fatalf("the fire with a drive must still drive, got %d", r.driver.onCount())
	}
	r.detect(t, detectInput(stageAt.Add(time.Second), read("EPC2", -50)))
	if len(r.alarms.rows) != 3 {
		t.Fatalf("the fire with no drive must be able to record again, got %d rows", len(r.alarms.rows))
	}
}

func TestStage_AlarmWriteFailureUndoesFiresWithNoDrive(t *testing.T) {
	r := newStageRig(t, dev(11, pulse5s))
	r.alarms.failAt = 1

	pending, err := r.stage.Detect(context.Background(), nil, detectInput(stageAt, read("EPC1", -50), read("EPC2", -50)))
	if err == nil {
		t.Fatal("want the write error")
	}
	pending.OnCommit(false)
	if r.driver.onCount() != 1 {
		t.Fatalf("only the fire with a drive drives, got %d", r.driver.onCount())
	}

	// The output is still pulsing. The dropped tag records its lost alarm on its
	// next read; the driven tag stays latched, because its relay did fire.
	r.detect(t, detectInput(stageAt.Add(time.Second), read("EPC1", -50), read("EPC2", -50)))

	if len(r.alarms.rows) != 1 || r.alarms.rows[0].EPC != "EPC2" {
		t.Fatalf("want one row for EPC2, got %+v", r.alarms.rows)
	}
	if r.driver.onCount() != 1 {
		t.Fatalf("the busy output must not drive again, got %d", r.driver.onCount())
	}
}

func TestStage_RollbackDrivesNothingAndNextReadFires(t *testing.T) {
	r := newStageRig(t, dev(11, pulse5s))

	pending := r.detect(t, detectInput(stageAt, read("EPC1", -50), read("EPC2", -50)))
	pending.OnRollback()
	if r.driver.onCount() != 0 {
		t.Fatalf("a rolled-back scan must drive nothing, got %d", r.driver.onCount())
	}

	next := r.detect(t, detectInput(stageAt.Add(time.Second), read("EPC1", -50), read("EPC2", -50)))
	next.OnCommit(true)

	if r.driver.onCount() != 1 {
		t.Fatalf("the next read must drive the output, got %d", r.driver.onCount())
	}
	if len(r.alarms.rows) != 4 {
		t.Fatalf("both tags must fire again after the rollback, got %d rows", len(r.alarms.rows))
	}
}

func TestStage_NoFiresWritesNothing(t *testing.T) {
	r := newStageRig(t, dev(11, nil))

	pending := r.detect(t, detectInput(stageAt, read("EPC1", -90)))

	if pending.OnCommit != nil || pending.OnRollback != nil {
		t.Fatal("with no fires there is nothing to defer")
	}
	if r.alarms.attempts != 0 || len(r.rec.ids) != 0 {
		t.Fatalf("no fires must write nothing: attempts=%d triggers=%d", r.alarms.attempts, len(r.rec.ids))
	}
}

func TestEvaluate_EnqueuesNoTrigger(t *testing.T) {
	r := newStageRig(t, dev(11, nil))

	r.engine.Evaluate(context.Background(), 42, 5, stageAt, reads(read("EPC1", -50)))

	if r.driver.onCount() != 1 {
		t.Fatalf("Evaluate still drives, got %d", r.driver.onCount())
	}
	if r.alarms.attempts != 0 || len(r.rec.ids) != 0 {
		t.Fatalf("only the stage may write in the transaction or enqueue: attempts=%d triggers=%d", r.alarms.attempts, len(r.rec.ids))
	}
}

// A presence refresh is not an alarm: nothing is written, so it goes out whether
// the scan commits or rolls back, and a message with only refreshes still sends.
func TestStage_PresenceRefreshSentOnCommitAndRollback(t *testing.T) {
	r := newStageRig(t, dev(12, map[string]any{"mode": "presence"}))

	edge := r.detect(t, detectInput(stageAt, read("EPC1", -50)))
	edge.OnCommit(true)
	if r.driver.onCount() != 1 {
		t.Fatalf("the edge must drive once, got %d", r.driver.onCount())
	}

	later := stageAt.Add(presenceRefreshEvery)
	refresh := r.detect(t, detectInput(later, read("EPC1", -50)))
	if len(r.alarms.rows) != 1 {
		t.Fatalf("a refresh must write no alarm, got %d rows", len(r.alarms.rows))
	}
	refresh.OnRollback()
	if r.driver.onCount() != 2 {
		t.Fatalf("a rolled-back scan must still send the refresh, got %d drives", r.driver.onCount())
	}
	if got := r.driver.calls[1].offAfter; got != presenceOnSeconds {
		t.Fatalf("the refresh must carry the on-time, got %d", got)
	}

	again := r.detect(t, detectInput(later.Add(presenceRefreshEvery), read("EPC1", -50)))
	again.OnCommit(true)
	if r.driver.onCount() != 3 {
		t.Fatalf("a committed scan must send the refresh, got %d drives", r.driver.onCount())
	}
}
