//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

const arbMAC = "C4DEE2000001"

// arbRig is two GL-S10 gateways in two rooms and one BLE-tagged asset.
type arbRig struct {
	db               *testutil.TestDB
	orgID            int
	devA, devB       int
	spA, spB         int
	locA, locB, locM int
	assetID          int
}

func newArbRig(t *testing.T) arbRig {
	t.Helper()
	ctx := context.Background()
	r := arbRig{db: testutil.SetupTestDBFull(t)}
	r.orgID = testutil.CreateTestAccount(t, r.db.AdminPool)
	r.devA = registerGLS10Device(t, r.db, r.orgID, "gw-a").ID
	r.devB = registerGLS10Device(t, r.db, r.orgID, "gw-b").ID
	registerBLETag(t, r.db, r.orgID, arbMAC)
	require.NoError(t, r.db.AdminPool.QueryRow(ctx,
		`SELECT asset_id FROM trakrf.tags WHERE org_id = $1 AND value = $2`, r.orgID, arbMAC).Scan(&r.assetID))
	for key, dst := range map[string]*int{"room-a": &r.locA, "room-b": &r.locB, "manual": &r.locM} {
		require.NoError(t, r.db.AdminPool.QueryRow(ctx,
			`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, $2, $2) RETURNING id`,
			r.orgID, key).Scan(dst))
	}
	for dev, pair := range map[int][2]*int{r.devA: {&r.locA, &r.spA}, r.devB: {&r.locB, &r.spB}} {
		require.NoError(t, r.db.AdminPool.QueryRow(ctx,
			`UPDATE trakrf.scan_points SET location_id = $1 WHERE org_id = $2 AND scan_device_id = $3 RETURNING id`,
			*pair[0], r.orgID, dev).Scan(pair[1]))
	}
	return r
}

// decide returns an ArbitrateFunc that assigns every asset to the given
// location, and records what it was handed.
func decide(loc, sp int, got *storage.ArbitrateInput) storage.ArbitrateFunc {
	return func(_ context.Context, _ pgx.Tx, in storage.ArbitrateInput) (map[int]storage.ArbitratedLocation, error) {
		if got != nil {
			*got = in
		}
		out := map[int]storage.ArbitratedLocation{}
		for _, rd := range in.Reads {
			out[rd.AssetID] = storage.ArbitratedLocation{LocationID: loc, ScanPointID: sp}
		}
		return out, nil
	}
}

func (r arbRig) persist(t *testing.T, dev int, at time.Time, arbitrate storage.ArbitrateFunc) storage.PersistResult {
	t.Helper()
	reads := []scanread.Read{{EPC: arbMAC, AntennaPort: 1, RSSI: -60}, {EPC: arbMAC, AntennaPort: 1, RSSI: -61}}
	res, err := r.db.Store.PersistReadsArbitrated(context.Background(), r.orgID, dev, 9, at, reads, arbitrate, nil)
	require.NoError(t, err)
	return res
}

type scanRow struct {
	location, scanPoint *int
	tagScan             *int64
}

func (r arbRig) row(t *testing.T, bucket time.Time) (scanRow, int) {
	t.Helper()
	var n int
	require.NoError(t, r.db.AdminPool.QueryRow(context.Background(),
		`SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1 AND asset_id = $2`, r.orgID, r.assetID).Scan(&n))
	var s scanRow
	err := r.db.AdminPool.QueryRow(context.Background(),
		`SELECT location_id, scan_point_id, tag_scan_id FROM trakrf.asset_scans
		 WHERE org_id = $1 AND asset_id = $2 AND timestamp = $3`, r.orgID, r.assetID, bucket).
		Scan(&s.location, &s.scanPoint, &s.tagScan)
	if errors.Is(err, pgx.ErrNoRows) {
		return scanRow{}, n
	}
	require.NoError(t, err)
	return s, n
}

// moved returns the read that carries the asset's arbitrated move, if any.
func moved(res storage.PersistResult) *storage.ArbitratedMove {
	for _, rd := range res.Resolved {
		if rd.Move != nil {
			return rd.Move
		}
	}
	return nil
}

func TestPersistReadsArbitrated_StoresTheArbitratedLocation(t *testing.T) {
	r := newArbRig(t)
	at := time.Now().UTC().Truncate(time.Minute).Add(12*time.Second + 345678*time.Nanosecond)

	// Gateway B reports, but the arbiter says room A.
	var in storage.ArbitrateInput
	res := r.persist(t, r.devB, at, decide(r.locA, r.spA, &in))

	assert.Equal(t, at, in.ReceivedAt, "arbiter sees the raw receive time")
	require.Len(t, in.Reads, 2)
	assert.Equal(t, r.locB, *in.Reads[0].LocationID, "arbiter sees what was observed")

	row, n := r.row(t, at.Truncate(time.Minute))
	assert.Equal(t, 1, n, "one row per asset per minute")
	assert.Equal(t, r.locA, *row.location)
	assert.Equal(t, r.spA, *row.scanPoint)
	assert.Equal(t, int64(9), *row.tagScan)

	assert.Equal(t, 1, res.Inserted)
	require.Len(t, res.Resolved, 2, "every read still reaches the detection stages")
	for _, rd := range res.Resolved {
		assert.True(t, rd.Arbitrated)
		assert.Equal(t, r.locB, *rd.LocationID, "resolved reads keep the observed location")
	}
	stored := 0
	for _, rd := range res.Resolved {
		if rd.Stored {
			stored++
		}
	}
	assert.Equal(t, 1, stored, "exactly one read carries the stored row")

	mv := moved(res)
	require.NotNil(t, mv, "first-ever sighting is a move")
	assert.Nil(t, mv.From)
	assert.Equal(t, r.locA, mv.To)
	assert.Equal(t, r.spA, mv.ScanPointID)
	assert.Equal(t, at.Truncate(time.Microsecond), mv.At, "move at the switch, not the minute floor")
}

func TestPersistReadsArbitrated_MidMinuteSwitchUpdatesTheRow(t *testing.T) {
	r := newArbRig(t)
	bucket := time.Now().UTC().Truncate(time.Minute)

	r.persist(t, r.devA, bucket.Add(5*time.Second), decide(r.locA, r.spA, nil))

	at := bucket.Add(41 * time.Second)
	res := r.persist(t, r.devA, at, decide(r.locB, r.spB, nil))
	row, n := r.row(t, bucket)
	assert.Equal(t, 1, n)
	assert.Equal(t, r.locB, *row.location, "the minute row records the arbitrated location at its end")
	assert.Equal(t, r.spB, *row.scanPoint)
	assert.Equal(t, 1, res.Updated)
	mv := moved(res)
	require.NotNil(t, mv)
	require.NotNil(t, mv.From)
	assert.Equal(t, r.locA, *mv.From, "origin is the row it replaced")
	assert.Equal(t, r.locB, mv.To)
	assert.Equal(t, at, mv.At)

	// Same location again: no write, no move.
	res = r.persist(t, r.devB, bucket.Add(50*time.Second), decide(r.locB, r.spB, nil))
	assert.Zero(t, res.Inserted+res.Updated)
	assert.Equal(t, 1, res.Dropped["conflict"])
	assert.Nil(t, moved(res))
	for _, rd := range res.Resolved {
		assert.False(t, rd.Stored)
	}
}

func TestPersistReadsArbitrated_NewMinuteComparesWithThePreviousRow(t *testing.T) {
	r := newArbRig(t)
	bucket := time.Now().UTC().Truncate(time.Minute)
	_, err := r.db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id)
		 VALUES ($1, $2, $3, $4, $5, 1)`, bucket.Add(-time.Minute), r.orgID, r.assetID, r.locA, r.spA)
	require.NoError(t, err)

	res := r.persist(t, r.devB, bucket.Add(3*time.Second), decide(r.locA, r.spA, nil))
	assert.Equal(t, 1, res.Inserted)
	assert.Nil(t, moved(res), "same location as last minute is not a move")

	res = r.persist(t, r.devB, bucket.Add(time.Minute+3*time.Second), decide(r.locB, r.spB, nil))
	mv := moved(res)
	require.NotNil(t, mv)
	require.NotNil(t, mv.From)
	assert.Equal(t, r.locA, *mv.From)
	assert.Equal(t, r.locB, mv.To)
}

func TestPersistReadsArbitrated_NeverOverwritesAManualSave(t *testing.T) {
	r := newArbRig(t)
	bucket := time.Now().UTC().Truncate(time.Minute)
	_, err := r.db.AdminPool.Exec(context.Background(),
		`INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id)
		 VALUES ($1, $2, $3, $4, NULL, NULL)`, bucket, r.orgID, r.assetID, r.locM)
	require.NoError(t, err)

	res := r.persist(t, r.devA, bucket.Add(20*time.Second), decide(r.locA, r.spA, nil))
	row, _ := r.row(t, bucket)
	assert.Equal(t, r.locM, *row.location, "an operator's save beats the passive reader")
	assert.Nil(t, row.scanPoint)
	assert.Nil(t, moved(res))
	assert.Zero(t, res.Inserted+res.Updated)

	// Next minute the gateway's answer lands, and it is a move away from the save.
	res = r.persist(t, r.devA, bucket.Add(time.Minute+time.Second), decide(r.locA, r.spA, nil))
	mv := moved(res)
	require.NotNil(t, mv)
	require.NotNil(t, mv.From)
	assert.Equal(t, r.locM, *mv.From)
}

func TestPersistReadsArbitrated_HoldWritesNothing(t *testing.T) {
	r := newArbRig(t)
	hold := func(_ context.Context, _ pgx.Tx, in storage.ArbitrateInput) (map[int]storage.ArbitratedLocation, error) {
		return map[int]storage.ArbitratedLocation{in.Reads[0].AssetID: {Hold: true}}, nil
	}
	res := r.persist(t, r.devB, time.Now().UTC(), hold)
	_, n := r.row(t, time.Now().UTC().Truncate(time.Minute))
	assert.Zero(t, n)
	assert.Equal(t, 2, res.Dropped["arbitration_hold"])
	require.Len(t, res.Resolved, 2, "held reads still reach detection")
	for _, rd := range res.Resolved {
		assert.True(t, rd.Arbitrated)
		assert.False(t, rd.Stored)
	}
}

func TestPersistReadsArbitrated_UndecidedAssetIsStoredAsObserved(t *testing.T) {
	r := newArbRig(t)
	none := func(context.Context, pgx.Tx, storage.ArbitrateInput) (map[int]storage.ArbitratedLocation, error) {
		return nil, nil
	}
	at := time.Now().UTC()
	res := r.persist(t, r.devB, at, none)
	row, n := r.row(t, at.Truncate(time.Minute))
	assert.Equal(t, 1, n)
	assert.Equal(t, r.locB, *row.location)
	require.Len(t, res.Resolved, 2)
	assert.True(t, res.Resolved[0].Stored)
	assert.False(t, res.Resolved[0].Arbitrated, "an undecided asset takes the ordinary path")
	assert.Equal(t, 1, res.Dropped["conflict"])
}

func TestPersistReadsArbitrated_ArbitrateErrorRollsBack(t *testing.T) {
	r := newArbRig(t)
	boom := func(context.Context, pgx.Tx, storage.ArbitrateInput) (map[int]storage.ArbitratedLocation, error) {
		return nil, errors.New("boom")
	}
	reads := []scanread.Read{{EPC: arbMAC, AntennaPort: 1, RSSI: -60}}
	_, err := r.db.Store.PersistReadsArbitrated(context.Background(), r.orgID, r.devA, 9, time.Now().UTC(), reads, boom, nil)
	require.Error(t, err)
	_, n := r.row(t, time.Now().UTC().Truncate(time.Minute))
	assert.Zero(t, n)
}

func TestPersistReadsArbitrated_DetectSeesTheMove(t *testing.T) {
	r := newArbRig(t)
	var got storage.DetectInput
	detect := func(_ context.Context, _ pgx.Tx, in storage.DetectInput) error {
		got = in
		return nil
	}
	reads := []scanread.Read{{EPC: arbMAC, AntennaPort: 1, RSSI: -60}}
	_, err := r.db.Store.PersistReadsArbitrated(context.Background(), r.orgID, r.devB, 9, time.Now().UTC(), reads, decide(r.locA, r.spA, nil), detect)
	require.NoError(t, err)
	require.Len(t, got.Reads, 1)
	require.NotNil(t, got.Reads[0].Move)
	assert.Equal(t, r.locA, got.Reads[0].Move.To)
}
