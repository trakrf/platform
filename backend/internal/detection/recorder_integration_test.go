//go:build integration

package detection_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestRecordChange_InsideSavepoint(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	assetID := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-SAVEPOINT").ID
	var locationID, scanPointID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'sp-loc', 'Dock') RETURNING id`, orgID,
	).Scan(&locationID))
	topic := "trakrf.id/savepoint/reads"
	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader savepoint", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT id FROM trakrf.scan_points WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = 1`,
		orgID, dev.ID).Scan(&scanPointID))

	inserter, err := outbox.NewInserter(db.AppPool)
	require.NoError(t, err)
	recorder := detection.NewRecorder(db.Store, inserter)

	count := func(query string) int {
		var n int
		require.NoError(t, db.AdminPool.QueryRow(ctx, query, orgID).Scan(&n))
		return n
	}
	changes := func() int {
		return count(`SELECT count(*) FROM trakrf.location_changes WHERE org_id = $1`)
	}
	jobs := func() int {
		return count(`SELECT count(*) FROM trakrf.river_job WHERE kind = 'notification_trigger' AND (args->>'org_id')::bigint = $1`)
	}
	change := func(at time.Time) notificationevent.LocationChange {
		return notificationevent.LocationChange{
			OrgID: orgID, AssetID: assetID, ToLocationID: locationID,
			Origin: notificationevent.OriginReader, ScanPointID: &scanPointID, DetectedAt: at,
		}
	}

	// record runs RecordChange in a savepoint under the org transaction; the
	// savepoint rolls back when rollback is true, and the transaction commits.
	record := func(at time.Time, rollback bool) {
		require.NoError(t, db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
			sp, err := tx.Begin(ctx)
			require.NoError(t, err)
			recorded, err := recorder.RecordChange(ctx, sp, change(at))
			require.NoError(t, err)
			require.True(t, recorded)
			if rollback {
				return sp.Rollback(ctx)
			}
			return sp.Commit(ctx)
		}))
	}

	t.Run("savepoint commit keeps change and job", func(t *testing.T) {
		record(time.Now().UTC().Truncate(time.Microsecond), false)
		require.Equal(t, 1, changes())
		require.Equal(t, 1, jobs())
	})

	t.Run("savepoint rollback drops change and job", func(t *testing.T) {
		record(time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond), true)
		require.Equal(t, 1, changes(), "rolled-back change must not persist")
		require.Equal(t, 1, jobs(), "rolled-back job must not persist")
	})

}
