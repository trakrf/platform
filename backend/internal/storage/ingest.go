package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// ScanGranularity is the resolution of asset location history (TRA-1118):
// asset_scans timestamps are truncated to this on insert, so the PK
// (timestamp, org_id, asset_id) keeps at most one row per asset per minute.
// Hardcoded by product decision — sub-minute history is unnecessary precision,
// and an org-level knob was rejected as YAGNI.
const ScanGranularity = time.Minute

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

// ListScanTopics returns every live mqtt device's publish_topic mapped to its
// route, for the subscription registry (TRA-922). SECURITY DEFINER under the
// hood, so no org context is needed — same as ResolveScanTopic.
func (s *Storage) ListScanTopics(ctx context.Context) (map[string]ScanRoute, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT org_id, scan_device_id, device_type, publish_topic FROM trakrf.list_active_scan_topics()`)
	if err != nil {
		return nil, fmt.Errorf("list scan topics: %w", err)
	}
	defer rows.Close()
	out := map[string]ScanRoute{}
	for rows.Next() {
		var r ScanRoute
		var topic string
		if err := rows.Scan(&r.OrgID, &r.ScanDeviceID, &r.DeviceType, &topic); err != nil {
			return nil, fmt.Errorf("scan scan-topic row: %w", err)
		}
		out[topic] = r
	}
	return out, rows.Err()
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
	// Updated counts minute rows an arbitrated message moved to a new location.
	Updated int
	Dropped map[string]int // reason -> count: no_scan_point, no_asset, conflict, arbitration_hold
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
	// ReaderTimestamp is the reader's own clock for the read. Informational:
	// stored history uses server time.
	ReaderTimestamp time.Time
	// Stored is true iff this read's asset_scans insert landed a row (TRA-1118).
	// A false value means the minute bucket already held a row for the asset, so
	// stored history did not change. Geofence ignores it — presence at the
	// boundary is its signal either way — but assetevent filters on it so a
	// conflict-dropped read cannot re-publish the same move for a whole minute.
	Stored bool
	// Arbitrated is true when zone arbitration decided this read's asset's
	// stored location (PersistReadsArbitrated). LocationID above is still what
	// was observed; the stored location may differ. Move consumers record only
	// Move for these reads and never derive one from LocationID.
	Arbitrated bool
	// Move is the location change the arbitrated row recorded, on at most one
	// read per asset per message. Nil when the stored location did not change.
	Move *ArbitratedMove
}

// ArbitratedMove is a change in an asset's stored location made by arbitration.
// From is nil for a first sighting or a previous row with no location. At is
// the receive time of the message that made it, not the minute bucket.
type ArbitratedMove struct {
	From            *int
	To, ScanPointID int
	At              time.Time
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
// reader clock is ignored, and the stored timestamp is truncated to
// ScanGranularity (TRA-1118) so the PK dedups to one row per asset per minute.
func (s *Storage) PersistReads(ctx context.Context, orgID, scanDeviceID int, tagScanID int64, receivedAt time.Time, reads []scanread.Read) (PersistResult, error) {
	return s.PersistReadsDetect(ctx, orgID, scanDeviceID, tagScanID, receivedAt, reads, nil)
}

// DetectInput is what a DetectFunc sees: the scan's resolved reads and the raw
// server receive time, not truncated to ScanGranularity.
type DetectInput struct {
	OrgID, ScanDeviceID int
	TagScanID           int64
	ReceivedAt          time.Time
	Reads               []ResolvedRead
}

// DetectFunc runs inside the scan transaction, after every asset_scans insert,
// so rows it writes on tx commit or roll back together with the scan. An error
// rolls the scan back.
type DetectFunc func(ctx context.Context, tx pgx.Tx, in DetectInput) error

// PersistReadsDetect is PersistReads with a hook that runs last in the same
// transaction, only when at least one read resolved. A nil hook is a no-op.
func (s *Storage) PersistReadsDetect(ctx context.Context, orgID, scanDeviceID int, tagScanID int64, receivedAt time.Time, reads []scanread.Read, detect DetectFunc) (PersistResult, error) {
	rawReceivedAt := receivedAt
	receivedAt = receivedAt.Truncate(ScanGranularity)
	res := PersistResult{Dropped: map[string]int{}}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		for _, rd := range reads {
			resolved, ok, err := resolveRead(ctx, tx, orgID, scanDeviceID, rd, res.Dropped)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			// Membership passed: record the resolved read for the geofence engine
			// before the dedup branch, so a within-message duplicate (conflict)
			// still counts as a boundary observation.
			res.Resolved = append(res.Resolved, resolved)
			stored, err := insertScanIfAbsent(ctx, tx, receivedAt, orgID, tagScanID, resolved)
			if err != nil {
				return err
			}
			res.Resolved[len(res.Resolved)-1].Stored = stored
			if !stored {
				res.Dropped["conflict"]++
				continue
			}
			res.Inserted++
		}
		if detect != nil && len(res.Resolved) > 0 {
			return detect(ctx, tx, DetectInput{
				OrgID:        orgID,
				ScanDeviceID: scanDeviceID,
				TagScanID:    tagScanID,
				ReceivedAt:   rawReceivedAt,
				Reads:        res.Resolved,
			})
		}
		return nil
	})
	if err != nil {
		return PersistResult{}, err
	}
	return res, nil
}

// resolveRead correlates a read to its scan_point by (device, antenna_port)
// (TRA-956) and its EPC to a tagged asset. The device is the one the topic
// routed to; the antenna is on the read, defaulting to 1 for single-antenna
// devices. A miss counts its reason in dropped and returns ok=false.
func resolveRead(ctx context.Context, tx pgx.Tx, orgID, scanDeviceID int, rd scanread.Read, dropped map[string]int) (ResolvedRead, bool, error) {
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
		dropped["no_scan_point"]++
		return ResolvedRead{}, false, nil
	}
	if err != nil {
		return ResolvedRead{}, false, fmt.Errorf("resolve scan_point for device %d antenna %d: %w", scanDeviceID, antennaPort, err)
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
		dropped["no_asset"]++
		return ResolvedRead{}, false, nil
	}
	if err != nil {
		return ResolvedRead{}, false, fmt.Errorf("resolve asset for epc %q: %w", rd.EPC, err)
	}
	return ResolvedRead{
		AssetID:         assetID,
		ScanPointID:     scanPointID,
		LocationID:      locationID,
		EPC:             rd.EPC,
		RSSI:            rd.RSSI,
		ReaderTimestamp: rd.ReaderTimestamp,
	}, true, nil
}

// insertScanIfAbsent writes the read's row for its minute bucket unless one is
// already there, and reports whether it landed.
func insertScanIfAbsent(ctx context.Context, tx pgx.Tx, bucket time.Time, orgID int, tagScanID int64, rd ResolvedRead) (bool, error) {
	ct, err := tx.Exec(ctx,
		`INSERT INTO trakrf.asset_scans
		   (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING`,
		bucket, orgID, rd.AssetID, rd.LocationID, rd.ScanPointID, tagScanID,
	)
	if err != nil {
		return false, fmt.Errorf("insert asset_scan: %w", err)
	}
	return ct.RowsAffected() > 0, nil
}
