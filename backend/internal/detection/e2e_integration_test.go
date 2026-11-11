//go:build integration

package detection_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/assetevent"
	"github.com/trakrf/platform/backend/internal/crossing"
	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/geofence"
	"github.com/trakrf/platform/backend/internal/ingest"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/mustering"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// End-to-end through the real savepoints, alarm write and enqueue. Every store
// call runs on db.Store, whose pool connects as the non-superuser role the app
// uses, so RLS applies to it just as it does in production. The admin pool only
// builds fixtures and counts rows.

const (
	epcOne  = "E2801190A503006543E21224"
	epcTwo  = "E2801190A503006543E21225"
	antShed = 1
	antGate = 2

	rowsCrossing = `SELECT count(*) FROM trakrf.location_changes WHERE org_id = $1 AND origin = 'reader_crossing'`
	rowsMove     = `SELECT count(*) FROM trakrf.location_changes WHERE org_id = $1 AND origin = 'reader'`
	rowsChange   = `SELECT count(*) FROM trakrf.location_changes WHERE org_id = $1`
	rowsAlarm    = `SELECT count(*) FROM trakrf.alarm_events WHERE org_id = $1`
	rowsScan     = `SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1`
	rowsJob      = `SELECT count(*) FROM trakrf.river_job
		WHERE kind = 'notification_trigger' AND (args->>'org_id')::bigint = $1`
	rowsAlarmJob = rowsJob + ` AND args->>'source' = 'alarm'`
)

var errForced = errors.New("forced scan rollback")

// fakeRelay stands in for the output driver. onDrive runs inside the drive.
type fakeRelay struct {
	mu      sync.Mutex
	drives  int
	onDrive func()
}

func (f *fakeRelay) Set(context.Context, outputdevice.OutputDevice, bool, int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.drives++
	if f.onDrive != nil {
		f.onDrive()
	}
	return nil
}

// rig is one org with one reader whose antennas sit on two locations.
type rig struct {
	t               *testing.T
	db              *testutil.TestDB
	orgID, deviceID int
	assetID         int
	shed, gate      int
	gatePoint       int
	recorder        *detection.Recorder
	relay           *fakeRelay
	engine          *geofence.Engine
}

func newRig(t *testing.T) *rig {
	t.Helper()
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	topic := "trakrf.id/detection-e2e/reads"
	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Gate reader", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	inserter, err := outbox.NewInserter(db.AppPool)
	require.NoError(t, err)

	cfg := geofence.DefaultConfig()
	cfg.StartupGrace = 0
	relay, log := &fakeRelay{}, zerolog.Nop()
	engine := geofence.NewEngine(cfg, db.Store, relay, &log)
	t.Cleanup(engine.Stop)

	r := &rig{
		t: t, db: db, orgID: orgID, deviceID: dev.ID, relay: relay, engine: engine,
		recorder: detection.NewRecorder(db.Store, inserter),
	}
	r.shed, r.gate = r.location("shed", "Boat shed"), r.location("gate", "Outside gate")
	_, err = db.AdminPool.Exec(ctx,
		`UPDATE trakrf.scan_points SET location_id = $1 WHERE org_id = $2 AND scan_device_id = $3 AND antenna_port = $4`,
		r.shed, orgID, dev.ID, antShed)
	require.NoError(t, err)
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.scan_points (org_id, scan_device_id, name, antenna_port, location_id)
		 VALUES ($1, $2, 'Antenna 2', $3, $4) RETURNING id`,
		orgID, dev.ID, antGate, r.gate).Scan(&r.gatePoint))
	r.assetID = r.asset("BOAT-1", epcOne)
	return r
}

func (r *rig) location(key, name string) int {
	r.t.Helper()
	var id int
	require.NoError(r.t, r.db.AdminPool.QueryRow(context.Background(),
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, $2, $3) RETURNING id`,
		r.orgID, key, name).Scan(&id))
	return id
}

func (r *rig) asset(key, epc string) int {
	r.t.Helper()
	a := testutil.CreateTestAsset(r.t, r.db.AdminPool, r.orgID, key)
	_, err := r.db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.tags (org_id, asset_id, type, value) VALUES ($1, $2, 'rfid', $3)`,
		r.orgID, a.ID, epc)
	require.NoError(r.t, err)
	return a.ID
}

// output binds a relay with a 5s pulse to the gate location.
func (r *rig) output() {
	r.t.Helper()
	_, err := r.db.Store.CreateOutputDevice(context.Background(), r.orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Gate strobe", BaseURL: "http://relay.test", LocationID: &r.gate,
		Metadata: map[string]any{"auto_off_seconds": 5},
	})
	require.NoError(r.t, err)
}

// stages are the production stages, in production order.
func (r *rig) stages() []detection.Stage {
	return []detection.Stage{
		r.engine.Stage(r.db.Store, r.recorder),
		assetevent.NewMoveStage(r.db.Store, r.recorder),
		crossing.NewStage(crossing.NewTracker(crossing.DefaultConfig()), r.db.Store, r.recorder),
	}
}

func pipeline(stages ...detection.Stage) *detection.Pipeline {
	log := zerolog.Nop()
	return detection.NewPipeline(&log, stages...)
}

func (r *rig) tagScan() int64 {
	r.t.Helper()
	id, err := r.db.Store.InsertRawTagScan(context.Background(), "trakrf.id/detection-e2e/reads", []byte(`{}`))
	require.NoError(r.t, err)
	return id
}

// scan runs one message the way the subscriber does.
func (r *rig) scan(p *detection.Pipeline, at time.Time, reads ...scanread.Read) {
	r.t.Helper()
	run := p.Begin()
	_, err := r.db.Store.PersistReadsDetect(context.Background(), r.orgID, r.deviceID, r.tagScan(), at, reads, run.Detect)
	require.NoError(r.t, err)
	run.Committed()
}

// scanRollsBack runs one message whose transaction fails after detection.
func (r *rig) scanRollsBack(p *detection.Pipeline, at time.Time, reads ...scanread.Read) {
	r.t.Helper()
	run := p.Begin()
	_, err := r.db.Store.PersistReadsDetect(context.Background(), r.orgID, r.deviceID, r.tagScan(), at, reads,
		func(ctx context.Context, tx pgx.Tx, in storage.DetectInput) error {
			require.NoError(r.t, run.Detect(ctx, tx, in))
			return errForced
		})
	require.ErrorIs(r.t, err, errForced)
	run.RolledBack()
}

func (r *rig) count(query string) int {
	r.t.Helper()
	var n int
	require.NoError(r.t, r.db.AdminPool.QueryRow(context.Background(), query, r.orgID).Scan(&n))
	return n
}

func read(antenna, rssi int, at time.Time) scanread.Read {
	return scanread.Read{EPC: epcOne, AntennaPort: antenna, RSSI: rssi, ReaderTimestamp: at}
}

// recentMinute is a minute boundary inside the previous-location lookup's
// ten-minute tail, with room for a test to use the minutes after it.
func recentMinute() time.Time {
	return time.Now().UTC().Truncate(time.Minute).Add(-4 * time.Minute)
}

// TestDetection_ExitQueuesTrigger plays the spec's worked example, one read per
// message. The peak at 1.0s stays in the window, so only the 3.5s read settles it.
func TestDetection_ExitQueuesTrigger(t *testing.T) {
	r := newRig(t)
	p := pipeline(r.stages()...)
	start := recentMinute().Add(10 * time.Second)
	steps := []struct {
		after         time.Duration
		antenna, rssi int
	}{
		{0, antShed, -52},
		{1000 * time.Millisecond, antShed, -40},
		{1500 * time.Millisecond, antGate, -58},
		{2500 * time.Millisecond, antGate, -44},
		{3000 * time.Millisecond, antShed, -60},
	}
	for _, s := range steps {
		at := start.Add(s.after)
		r.scan(p, at, read(s.antenna, s.rssi, at))
		require.Zero(t, r.count(rowsCrossing), "no crossing before the deciding read")
	}

	at := start.Add(3500 * time.Millisecond)
	r.scan(p, at, read(antGate, -36, at))

	var id, from, to, scanPoint int
	var detected time.Time
	require.NoError(t, r.db.AdminPool.QueryRow(context.Background(),
		`SELECT id, from_location_id, to_location_id, scan_point_id, detected_at
		 FROM trakrf.location_changes WHERE org_id = $1 AND origin = 'reader_crossing'`, r.orgID,
	).Scan(&id, &from, &to, &scanPoint, &detected))
	assert.Equal(t, r.shed, from)
	assert.Equal(t, r.gate, to)
	assert.Equal(t, r.gatePoint, scanPoint)
	assert.True(t, at.Equal(detected), "detected_at must be the raw receive time, got %s", detected)

	var jobs int
	require.NoError(t, r.db.AdminPool.QueryRow(context.Background(),
		rowsJob+` AND args->>'source' = 'location_change' AND (args->>'source_id')::bigint = $2`, r.orgID, id,
	).Scan(&jobs))
	assert.Equal(t, 1, jobs, "the crossing queues one trigger")
}

func TestDetection_ScanRollbackLeavesNothing(t *testing.T) {
	r := newRig(t)
	r.output()
	p := pipeline(r.stages()...)
	start := recentMinute().Add(55 * time.Second)
	r.scan(p, start, read(antShed, -50, start))
	scans, jobs := r.count(rowsScan), r.count(rowsJob)
	moves := r.count(rowsMove)

	// The next minute, so the scan stores a row and a reader move is due as well.
	exit := start.Add(10 * time.Second)
	r.scanRollsBack(p, exit, read(antGate, -40, exit))

	assert.Equal(t, scans, r.count(rowsScan))
	assert.Equal(t, jobs, r.count(rowsJob))
	assert.Equal(t, moves, r.count(rowsMove))
	assert.Zero(t, r.count(rowsCrossing))
	assert.Zero(t, r.count(rowsAlarm))
	assert.Zero(t, r.relay.drives, "a rolled-back scan drives nothing")

	// The same read again finds the tracker and latch as they were: the leader is
	// still the shed, so it crosses, and the relay fires.
	retry := exit.Add(time.Second)
	r.scan(p, retry, read(antGate, -40, retry))

	assert.Equal(t, 1, r.count(rowsCrossing), "the tracker must not have moved on the rolled-back scan")
	assert.Equal(t, moves+1, r.count(rowsMove))
	assert.Equal(t, 1, r.count(rowsAlarm))
	assert.Equal(t, 1, r.relay.drives, "the latch must be restored after the rollback")
	assert.Equal(t, jobs+3, r.count(rowsJob))
}

// poisonStage records a change, then fails on a real database error, so the
// transaction is aborted the way a genuine stage failure leaves it.
type poisonStage struct {
	r *rig
}

func (s poisonStage) Name() string { return "poison" }

func (s poisonStage) Detect(ctx context.Context, tx pgx.Tx, in storage.DetectInput) (detection.Pending, error) {
	point := in.Reads[0].ScanPointID
	_, err := s.r.recorder.RecordChange(ctx, tx, notificationevent.LocationChange{
		OrgID: in.OrgID, AssetID: in.Reads[0].AssetID, ToLocationID: s.r.shed,
		Origin: notificationevent.OriginReader, ScanPointID: &point, DetectedAt: in.ReceivedAt.Add(-time.Hour),
	})
	require.NoError(s.r.t, err)
	_, err = tx.Exec(ctx, `SELECT 1 / 0`)
	require.Error(s.r.t, err)
	return detection.Pending{}, err
}

func TestDetection_StageErrorKeepsScan(t *testing.T) {
	r := newRig(t)
	r.output()
	st := r.stages()
	// Geofence runs before the failure, the move and crossing stages after it.
	p := pipeline(st[0], poisonStage{r: r}, st[1], st[2])
	start := recentMinute().Add(30 * time.Second)

	r.scan(p, start, read(antShed, -50, start))
	exit := start.Add(time.Minute)
	r.scan(p, exit, read(antGate, -40, exit))

	assert.Equal(t, 2, r.count(rowsScan), "a failing stage must keep the scan")
	assert.Equal(t, 1, r.count(rowsAlarm), "the stage before the failure keeps its row")
	assert.Equal(t, 1, r.count(rowsCrossing), "the stages after the failure keep their rows")
	assert.Equal(t, 2, r.count(rowsMove), "one per message, and none from the failed stage")
	assert.Equal(t, 4, r.count(rowsJob), "alarm, crossing and two moves; the failed stage's jobs are gone")
}

func TestDetection_SimulatorCreatesNoTrigger(t *testing.T) {
	r := newRig(t)
	r.output()
	ctx := context.Background()
	log := zerolog.Nop()
	dispatcher := assetevent.NewDispatcher(nil, &log)
	dispatcher.Start()
	t.Cleanup(dispatcher.Stop)
	// The simulator's fan-out, as serve.go builds it with ingestion on.
	evaluators := ingest.MultiEvaluator{
		r.engine,
		mustering.NewEngine(r.db.Store, mustering.NewBroadcaster(), &log),
		assetevent.NewEvaluator(r.db.Store, dispatcher, &log),
	}

	at := recentMinute().Add(10 * time.Second)
	tagScanID := r.tagScan()
	res, err := r.db.Store.PersistReads(ctx, r.orgID, r.deviceID, tagScanID, at, []scanread.Read{read(antGate, -40, at)})
	require.NoError(t, err)
	evaluators.Evaluate(ctx, r.orgID, tagScanID, at, res.Resolved)

	assert.Equal(t, 1, r.relay.drives, "the output must fire, or this proves nothing")
	assert.Equal(t, 1, r.count(rowsAlarm))
	assert.Zero(t, r.count(rowsJob), "a simulated read never queues a trigger")
	assert.Zero(t, r.count(rowsChange))
}

func TestDetection_AlarmQueuesTrigger(t *testing.T) {
	r := newRig(t)
	r.output()
	// The admin pool is another connection, so it only sees committed rows.
	var alarmsAtDrive, jobsAtDrive int
	r.relay.onDrive = func() {
		alarmsAtDrive, jobsAtDrive = r.count(rowsAlarm), r.count(rowsAlarmJob)
	}

	at := recentMinute().Add(10 * time.Second)
	r.scan(pipeline(r.stages()...), at, read(antGate, -40, at))

	assert.Equal(t, 1, r.relay.drives)
	assert.Equal(t, 1, alarmsAtDrive, "the drive must wait for the commit")
	assert.Equal(t, 1, jobsAtDrive, "the drive must wait for the commit")

	var id, jobs int
	require.NoError(t, r.db.AdminPool.QueryRow(context.Background(),
		`SELECT id FROM trakrf.alarm_events WHERE org_id = $1`, r.orgID).Scan(&id))
	require.NoError(t, r.db.AdminPool.QueryRow(context.Background(),
		rowsAlarmJob+` AND (args->>'source_id')::bigint = $2`, r.orgID, id).Scan(&jobs))
	assert.Equal(t, 1, jobs, "the trigger names the alarm row")
}

func TestDetection_BusyOutputStillQueuesAlarm(t *testing.T) {
	r := newRig(t)
	r.output()
	r.asset("BOAT-2", epcTwo)

	at := recentMinute().Add(10 * time.Second)
	second := read(antGate, -41, at)
	second.EPC = epcTwo
	r.scan(pipeline(r.stages()...), at, read(antGate, -40, at), second)

	assert.Equal(t, 2, r.count(rowsAlarm), "one alarm per tag")
	assert.Equal(t, 2, r.count(rowsAlarmJob))
	assert.Equal(t, 1, r.relay.drives, "the second tag finds the output busy")
}

// failingJobs refuses every enqueue.
type failingJobs struct{}

func (failingJobs) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, errors.New("queue unavailable")
}

func TestDetection_FailedAlarmWriteStillDrives(t *testing.T) {
	r := newRig(t)
	r.output()
	p := pipeline(r.engine.Stage(r.db.Store, detection.NewRecorder(r.db.Store, failingJobs{})))

	at := recentMinute().Add(10 * time.Second)
	r.scan(p, at, read(antGate, -40, at))

	assert.Equal(t, 1, r.relay.drives, "a failed alarm write still drives the relay")
	assert.Zero(t, r.count(rowsAlarm), "the alarm is rolled back with its enqueue")
	assert.Equal(t, 1, r.count(rowsScan))
}

// TestDetection_ColdStartLookupsUnderAppRole checks that the org context from
// WithOrgTx reaches both cold-start lookups under RLS. Each lookup finding nothing
// would look like a parked asset and record no crossing, so a failure here is
// silent in production: the crossing row is the evidence.
func TestDetection_ColdStartLookupsUnderAppRole(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()

	var super, bypass bool
	require.NoError(t, r.db.AppPool.QueryRow(ctx,
		`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super, &bypass))
	require.False(t, super, "the store must not connect as a superuser")
	require.False(t, bypass, "the store must not bypass RLS")

	start := recentMinute().Add(10 * time.Second)
	r.scan(pipeline(r.stages()...), start, read(antShed, -50, start))

	// A fresh tracker is a restart. The asset last seen at the shed now reads at
	// the gate, so only the lookups can tell it crossed.
	exit := start.Add(time.Minute)
	r.scan(pipeline(r.stages()...), exit, read(antGate, -50, exit))
	assert.Equal(t, 1, r.count(rowsCrossing), "both lookups must see this org's rows")

	lookups := func(contextOrg int) (map[int]storage.PreviousLocation, []int) {
		var prev map[int]storage.PreviousLocation
		var locs []int
		require.NoError(t, r.db.Store.WithOrgTx(ctx, contextOrg, func(tx pgx.Tx) error {
			var err error
			if prev, err = r.db.Store.PreviousAssetLocationsTx(ctx, tx, r.orgID, []int{r.assetID}, exit); err != nil {
				return err
			}
			locs, err = r.db.Store.ScanDeviceLocationIDsTx(ctx, tx, r.orgID, r.deviceID)
			return err
		}))
		return prev, locs
	}

	prev, locs := lookups(r.orgID)
	assert.Contains(t, prev, r.assetID)
	assert.ElementsMatch(t, []int{r.shed, r.gate}, locs)

	// Control: the same calls under another org's context find nothing, so RLS is
	// really filtering on this role and the result above is not a bypass.
	prev, locs = lookups(r.orgID + 1)
	assert.Empty(t, prev)
	assert.Empty(t, locs)
}
