package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AlarmEventRow is one geofence boundary alarm to persist (TRA-901). The geofence
// engine builds it on a fire; storage writes it under org context.
type AlarmEventRow struct {
	AssetID     int
	ScanPointID int
	LocationID  *int
	EPC         string
	RSSI        int
	TagScanID   int64
	FiredAt     time.Time
}

// InsertAlarmEvent appends a geofence alarm to trakrf.alarm_events under org
// context (RLS) and returns its id, which the drives that deliver it carry
// (output_drives.alarm_event_id). It is called best-effort from the geofence engine: a failure
// here is logged by the caller and never blocks ingestion or the asset_scans
// write, which is the authoritative record.
func (s *Storage) InsertAlarmEvent(ctx context.Context, orgID int, ev AlarmEventRow) (int64, error) {
	var id int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO trakrf.alarm_events
			   (org_id, asset_id, scan_point_id, location_id, epc, rssi, tag_scan_id, fired_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 RETURNING id`,
			orgID, ev.AssetID, ev.ScanPointID, ev.LocationID, ev.EPC, ev.RSSI, ev.TagScanID, ev.FiredAt,
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert alarm_event: %w", err)
		}
		return nil
	})
	return id, err
}
