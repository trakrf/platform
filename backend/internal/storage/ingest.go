package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/scanread"
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
	// Resolved is every read that passed the membership filter (registered rfid
	// tag → asset AND registered scan_point), enriched with the data the geofence
	// engine (TRA-901) needs. A read appears here even when its asset_scans insert
	// was a within-message dedup conflict — presence at the boundary is the
	// geofence signal regardless of scan-row dedup.
	Resolved []ResolvedRead
}

// ResolvedRead is a membership-passing read with the fields the geofence engine
// (TRA-901) evaluates. Produced by PersistReads, consumed by geofence.Evaluate.
// Rule config (mode, age-out, rssi_threshold) lives on the output device, not the
// scan point, so it is resolved by the engine — not carried here (TRA-943).
type ResolvedRead struct {
	AssetID     int
	ScanPointID int
	LocationID  *int
	EPC         string
	RSSI        int // scanread.Read.RSSI; 0 == parser sentinel for "no usable RSSI"
}

// PersistReads writes asset_scans for parsed reads under org context (RLS).
// scanDeviceID is the device the MQTT topic routed to (resolve_scan_topic); each
// read is correlated to its scan_point by (scan_device_id, antenna_port) (TRA-956).
// Asset resolution is tag-based with NO auto-create (TRA-900): a read records a
// scan only if its EPC already has a live tag linked to an asset. Membership is
// tag-class agnostic (TRA-927) — the read identifier is matched against the tag
// value regardless of type, so a BLE gateway's MAC registered as type='ble'
// resolves the same as an rfid EPC. Matching is leading-zero / case-insensitive
// on the hex value (TRA-944), identical to the handheld getMatchingKey, so a tag
// registered by its short barcode value resolves the reader's full-width EPC.
// receivedAt (server time) is authoritative for asset_scans.timestamp; the
// reader clock is ignored.
func (s *Storage) PersistReads(ctx context.Context, orgID, scanDeviceID int, tagScanID int64, receivedAt time.Time, reads []scanread.Read) (PersistResult, error) {
	res := PersistResult{Dropped: map[string]int{}}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		for _, rd := range reads {
			// Correlate the read to its scan_point by (device, antenna_port)
			// (TRA-956). The device is the one the topic routed to; the antenna
			// is on the read, defaulting to 1 for single-antenna devices. A read
			// for an unprovisioned antenna is a clean no_scan_point miss.
			antennaPort := rd.AntennaPort
			if antennaPort < 1 {
				antennaPort = 1
			}
			var scanPointID int
			var locationID *int
			err := tx.QueryRow(ctx,
				`SELECT id, location_id
				 FROM trakrf.scan_points
				 WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = $3 AND deleted_at IS NULL`,
				orgID, scanDeviceID, antennaPort,
			).Scan(&scanPointID, &locationID)
			if errors.Is(err, pgx.ErrNoRows) {
				res.Dropped["no_scan_point"]++
				continue
			}
			if err != nil {
				return fmt.Errorf("resolve scan_point for device %d antenna %d: %w", scanDeviceID, antennaPort, err)
			}

			var assetID int
			err = tx.QueryRow(ctx,
				`SELECT asset_id FROM trakrf.tags
				 WHERE org_id = $1
				   AND normalized_value = trakrf.normalize_tag_value($2)
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

			// Membership passed: record the resolved read for the geofence engine
			// before the dedup branch, so a within-message duplicate (conflict)
			// still counts as a boundary observation.
			res.Resolved = append(res.Resolved, ResolvedRead{
				AssetID:     assetID,
				ScanPointID: scanPointID,
				LocationID:  locationID,
				EPC:         rd.EPC,
				RSSI:        rd.RSSI,
			})

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
