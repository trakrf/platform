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

func TestPersistReadsDetect_HookSeesRawTimeAndResolvedReads(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "detect-raw")
	registerRFIDTag(t, db, orgID, testEPC)

	received := time.Now().UTC().Truncate(time.Minute).Add(37*time.Second + 123*time.Millisecond)
	readerTS := received.Add(-2 * time.Second)
	var got storage.DetectInput
	called := 0
	_, err := db.Store.PersistReadsDetect(context.Background(), orgID, dev.ID, 42, received,
		[]scanread.Read{{EPC: testEPC, AntennaPort: 1, RSSI: -40, ReaderTimestamp: readerTS}},
		func(_ context.Context, _ pgx.Tx, in storage.DetectInput) error {
			called++
			got = in
			return nil
		})
	require.NoError(t, err)
	require.Equal(t, 1, called)
	assert.True(t, got.ReceivedAt.Equal(received), "ReceivedAt must not be minute-truncated")
	assert.Equal(t, orgID, got.OrgID)
	assert.Equal(t, dev.ID, got.ScanDeviceID)
	assert.Equal(t, int64(42), got.TagScanID)
	require.Len(t, got.Reads, 1)
	assert.True(t, got.Reads[0].ReaderTimestamp.Equal(readerTS))
}

func TestPersistReadsDetect_HookWriteCommitsWithScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "detect-commit")
	registerRFIDTag(t, db, orgID, testEPC)

	_, err := db.Store.PersistReadsDetect(ctx, orgID, dev.ID, 1, time.Now(),
		[]scanread.Read{{EPC: testEPC, AntennaPort: 1, RSSI: -40}},
		func(ctx context.Context, tx pgx.Tx, in storage.DetectInput) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO trakrf.alarm_events (org_id, asset_id, scan_point_id, location_id, epc, rssi, tag_scan_id, fired_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
				in.OrgID, in.Reads[0].AssetID, in.Reads[0].ScanPointID, in.Reads[0].LocationID,
				in.Reads[0].EPC, in.Reads[0].RSSI, in.TagScanID)
			return err
		})
	require.NoError(t, err)

	var n int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.alarm_events WHERE org_id = $1`, orgID).Scan(&n))
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, countAssetScans(t, db, orgID))
}

func TestPersistReadsDetect_HookErrorRollsBackScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "detect-fail")
	registerRFIDTag(t, db, orgID, testEPC)

	boom := errors.New("detector failed")
	_, err := db.Store.PersistReadsDetect(context.Background(), orgID, dev.ID, 1, time.Now(),
		[]scanread.Read{{EPC: testEPC, AntennaPort: 1, RSSI: -40}},
		func(context.Context, pgx.Tx, storage.DetectInput) error { return boom })
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 0, countAssetScans(t, db, orgID))
}
