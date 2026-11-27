//go:build integration

package arbitration_test

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/arbitration"
	"github.com/trakrf/platform/backend/internal/assetevent"
	"github.com/trakrf/platform/backend/internal/crossing"
	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// End to end through real storage, the production detection stages and the
// asset.moved evaluator, the way the subscriber drives them for a BLE gateway.

const badge = "C4DEE2000042"

type capture struct{ events []assetevent.AssetMoved }

func (c *capture) Enqueue(ev assetevent.AssetMoved) { c.events = append(c.events, ev) }

type rig struct {
	t              *testing.T
	db             *testutil.TestDB
	orgID, assetID int
	devA, devB     int
	roomA, roomB   int
	stage          *arbitration.Stage
	pipe           *detection.Pipeline
	eval           *assetevent.Evaluator
	moved          *capture
}

func newRig(t *testing.T) *rig {
	t.Helper()
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	r := &rig{t: t, db: db, orgID: testutil.CreateTestAccount(t, db.AdminPool), moved: &capture{}}
	for key, dst := range map[string]*int{"room-a": &r.roomA, "room-b": &r.roomB} {
		require.NoError(t, db.AdminPool.QueryRow(ctx,
			`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, $2, $2) RETURNING id`,
			r.orgID, key).Scan(dst))
	}
	for key, pair := range map[string][2]*int{"gw-a": {&r.devA, &r.roomA}, "gw-b": {&r.devB, &r.roomB}} {
		topic := "arbitration-e2e/" + key + "/reads"
		d, err := db.Store.CreateScanDevice(ctx, r.orgID, scandevice.CreateScanDeviceRequest{
			Name: key, Type: scandevice.DeviceTypeGLS10, PublishTopic: &topic,
		})
		require.NoError(t, err)
		*pair[0] = d.ID
		_, err = db.AdminPool.Exec(ctx,
			`UPDATE trakrf.scan_points SET location_id = $1 WHERE org_id = $2 AND scan_device_id = $3`,
			*pair[1], r.orgID, d.ID)
		require.NoError(t, err)
	}
	a := testutil.CreateTestAsset(t, db.AdminPool, r.orgID, "BADGE-1")
	r.assetID = a.ID
	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.tags (org_id, asset_id, type, value) VALUES ($1, $2, 'ble', $3)`, r.orgID, a.ID, badge)
	require.NoError(t, err)

	inserter, err := outbox.NewInserter(db.AppPool)
	require.NoError(t, err)
	rec := detection.NewRecorder(db.Store, inserter)
	log := zerolog.Nop()
	r.pipe = detection.NewPipeline(&log,
		assetevent.NewMoveStage(db.Store, rec),
		crossing.NewStage(crossing.NewTracker(crossing.DefaultConfig()), db.Store, rec),
	)
	r.stage = arbitration.NewStage(arbitration.New(arbitration.DefaultConfig()), db.Store)
	r.eval = assetevent.NewEvaluator(db.Store, r.moved, &log)
	return r
}

// scan runs one gateway message the way the subscriber does.
func (r *rig) scan(dev int, at time.Time, rssi int) {
	r.t.Helper()
	ctx := context.Background()
	tagScanID, err := r.db.Store.InsertRawTagScan(ctx, "arbitration-e2e", []byte(`{}`))
	require.NoError(r.t, err)
	arb, run := r.stage.Begin(), r.pipe.Begin()
	reads := []scanread.Read{{EPC: badge, AntennaPort: 1, RSSI: rssi}}
	res, err := r.db.Store.PersistReadsArbitrated(ctx, r.orgID, dev, tagScanID, at, reads, arb.Arbitrate, run.Detect)
	require.NoError(r.t, err)
	arb.Committed()
	run.Committed()
	r.eval.Evaluate(ctx, r.orgID, tagScanID, at, res.Resolved)
}

// interleave has both gateways report every second for n seconds from start.
func (r *rig) interleave(start time.Time, n, rssiA, rssiB int) {
	for i := 0; i < n; i++ {
		at := start.Add(time.Duration(i) * time.Second)
		r.scan(r.devB, at, rssiB) // the weaker room publishes first each round
		r.scan(r.devA, at.Add(300*time.Millisecond), rssiA)
	}
}

type change struct {
	from *int
	to   int
	at   time.Time
}

func (r *rig) changes() []change {
	r.t.Helper()
	rows, err := r.db.AdminPool.Query(context.Background(),
		`SELECT from_location_id, to_location_id, detected_at FROM trakrf.location_changes
		 WHERE org_id = $1 AND origin = 'reader' ORDER BY detected_at`, r.orgID)
	require.NoError(r.t, err)
	defer rows.Close()
	var out []change
	for rows.Next() {
		var c change
		require.NoError(r.t, rows.Scan(&c.from, &c.to, &c.at))
		out = append(out, c)
	}
	require.NoError(r.t, rows.Err())
	return out
}

func (r *rig) stored() (rows, location int) {
	r.t.Helper()
	require.NoError(r.t, r.db.AdminPool.QueryRow(context.Background(),
		`SELECT count(*), (SELECT location_id FROM trakrf.asset_scans WHERE org_id = $1 ORDER BY timestamp DESC LIMIT 1)
		 FROM trakrf.asset_scans WHERE org_id = $1`, r.orgID).Scan(&rows, &location))
	return rows, location
}

func (r *rig) currentLocation() int {
	r.t.Helper()
	items, err := r.db.Store.ListCurrentLocations(context.Background(), r.orgID, report.CurrentLocationFilter{Limit: 10})
	require.NoError(r.t, err)
	require.Len(r.t, items, 1)
	require.NotNil(r.t, items[0].LocationID)
	return *items[0].LocationID
}

func TestArbitration_InterleavedGatewaysStoreTheStrongerRoomAndMoveOnce(t *testing.T) {
	r := newRig(t)
	minute := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)

	// The badge was last stored in room A; then both gateways hear it inside
	// one minute, room B first every round. Unarbitrated, B's first message
	// would set the minute.
	_, err := r.db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, tag_scan_id)
		 VALUES ($1, $2, $3, $4, 1)`, minute.Add(-time.Minute), r.orgID, r.assetID, r.roomA)
	require.NoError(t, err)

	r.interleave(minute.Add(time.Second), 15, -45, -75)
	rows, loc := r.stored()
	assert.Equal(t, 2, rows, "one row per minute")
	assert.Equal(t, r.roomA, loc)
	assert.Equal(t, r.roomA, r.currentLocation())
	assert.Empty(t, r.changes(), "stationary in A: no move")
	assert.Empty(t, r.moved.events)

	// The badge walks to room B: one move, at the switch, and the minute row follows.
	switchStart := minute.Add(16 * time.Second)
	r.interleave(switchStart, 15, -75, -45)
	rows, loc = r.stored()
	assert.Equal(t, 2, rows, "the switch updated this minute's row")
	assert.Equal(t, r.roomB, loc)
	assert.Equal(t, r.roomB, r.currentLocation())

	ch := r.changes()
	require.Len(t, ch, 1, "exactly one recorded move")
	require.NotNil(t, ch[0].from)
	assert.Equal(t, r.roomA, *ch[0].from)
	assert.Equal(t, r.roomB, ch[0].to)
	assert.True(t, ch[0].at.After(switchStart.Add(5*time.Second-time.Millisecond)), "after the dwell: %s", ch[0].at)
	assert.True(t, ch[0].at.Before(minute.Add(time.Minute)), "inside the minute, not on its floor")

	require.Len(t, r.moved.events, 1, "exactly one asset.moved")
	ev := r.moved.events[0]
	assert.Equal(t, r.roomB, ev.To.ID)
	require.NotNil(t, ev.From)
	assert.Equal(t, r.roomA, ev.From.ID)
	assert.True(t, ev.OccurredAt.Equal(ch[0].at), "event and change share the switch time")

	// Staying in B into the next minute adds a row and no move.
	r.interleave(minute.Add(time.Minute+time.Second), 5, -75, -45)
	assert.Len(t, r.changes(), 1)
	assert.Len(t, r.moved.events, 1)
}
