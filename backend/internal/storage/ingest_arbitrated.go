package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// ArbitrateInput is what an ArbitrateFunc sees: every resolved read of the
// message, with its observed location, and the raw server receive time.
type ArbitrateInput struct {
	OrgID      int
	ReceivedAt time.Time
	Reads      []ResolvedRead
}

// ArbitratedLocation is where arbitration puts an asset for this message. Hold
// means it has decided but cannot write a row yet (a seeded location not heard
// since); nothing is stored for the asset this message.
type ArbitratedLocation struct {
	LocationID, ScanPointID int
	Hold                    bool
}

// ArbitrateFunc decides the stored location of each asset in the message. It
// runs on the scan transaction after the reads resolve and before any row is
// written. An asset it leaves out is stored as observed, the ordinary way. An
// error rolls the scan back.
type ArbitrateFunc func(ctx context.Context, tx pgx.Tx, in ArbitrateInput) (map[int]ArbitratedLocation, error)

// upsertArbitratedScan writes the asset's minute row at the arbitrated
// location. An existing row for the bucket moves only when the location
// differs, and only if a reader wrote it: a manual save (tag_scan_id NULL) beats
// the passive read, the same rule SaveInventoryScans enforces from its side.
// It returns the pre-statement location of that row so a same-minute switch has
// its origin; no row comes back when nothing was written.
const upsertArbitratedScan = `WITH old AS (
		SELECT location_id FROM trakrf.asset_scans
		WHERE timestamp = $1 AND org_id = $2 AND asset_id = $3
	)
	INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (timestamp, org_id, asset_id) DO UPDATE
	SET location_id = EXCLUDED.location_id,
	    scan_point_id = EXCLUDED.scan_point_id,
	    tag_scan_id = EXCLUDED.tag_scan_id
	WHERE trakrf.asset_scans.tag_scan_id IS NOT NULL
	  AND trakrf.asset_scans.location_id IS DISTINCT FROM EXCLUDED.location_id
	RETURNING (xmax = 0), (SELECT location_id FROM old)`

// PersistReadsArbitrated is PersistReadsDetect for a device type under zone
// arbitration: the stored location of each asset is the one arbitrate decides,
// not the reporting gateway's, and a mid-minute change updates the minute row
// rather than waiting for the next one. One row per asset per minute still
// holds (TRA-1118). Every resolved read still reaches detect with its observed
// location; the stored change, if any, rides on one of them as Move.
func (s *Storage) PersistReadsArbitrated(ctx context.Context, orgID, scanDeviceID int, tagScanID int64, receivedAt time.Time, reads []scanread.Read, arbitrate ArbitrateFunc, detect DetectFunc) (PersistResult, error) {
	bucket := receivedAt.Truncate(ScanGranularity)
	res := PersistResult{Dropped: map[string]int{}}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		for _, rd := range reads {
			resolved, ok, err := resolveRead(ctx, tx, orgID, scanDeviceID, rd, res.Dropped)
			if err != nil {
				return err
			}
			if ok {
				res.Resolved = append(res.Resolved, resolved)
			}
		}
		if len(res.Resolved) == 0 {
			return nil
		}
		decisions, err := arbitrate(ctx, tx, ArbitrateInput{OrgID: orgID, ReceivedAt: receivedAt, Reads: res.Resolved})
		if err != nil {
			return fmt.Errorf("arbitrate: %w", err)
		}
		if err := s.storeArbitrated(ctx, tx, orgID, tagScanID, receivedAt, bucket, decisions, &res); err != nil {
			return err
		}
		if detect != nil {
			return detect(ctx, tx, DetectInput{
				OrgID:        orgID,
				ScanDeviceID: scanDeviceID,
				TagScanID:    tagScanID,
				ReceivedAt:   receivedAt,
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

// pendingFrom is a freshly inserted arbitrated row whose origin is the asset's
// previous minute, looked up in one batch.
type pendingFrom struct {
	read int // index into res.Resolved that carries the move
	to   ArbitratedLocation
}

func (s *Storage) storeArbitrated(ctx context.Context, tx pgx.Tx, orgID int, tagScanID int64, receivedAt, bucket time.Time, decisions map[int]ArbitratedLocation, res *PersistResult) error {
	at := receivedAt.Truncate(time.Microsecond)
	done := map[int]bool{}
	fresh := map[int]pendingFrom{}
	for i := range res.Resolved {
		rd := &res.Resolved[i]
		d, decided := decisions[rd.AssetID]
		if !decided {
			stored, err := insertScanIfAbsent(ctx, tx, bucket, orgID, tagScanID, *rd)
			if err != nil {
				return err
			}
			rd.Stored = stored
			if stored {
				res.Inserted++
			} else {
				res.Dropped["conflict"]++
			}
			continue
		}
		rd.Arbitrated = true
		if d.Hold {
			res.Dropped["arbitration_hold"]++
			continue
		}
		if done[rd.AssetID] {
			continue
		}
		done[rd.AssetID] = true

		var inserted bool
		var old *int
		err := tx.QueryRow(ctx, upsertArbitratedScan,
			bucket, orgID, rd.AssetID, d.LocationID, d.ScanPointID, tagScanID).Scan(&inserted, &old)
		if errors.Is(err, pgx.ErrNoRows) {
			res.Dropped["conflict"]++
			continue
		}
		if err != nil {
			return fmt.Errorf("upsert arbitrated asset_scan: %w", err)
		}
		rd.Stored = true
		if inserted {
			res.Inserted++
			fresh[rd.AssetID] = pendingFrom{read: i, to: d}
			continue
		}
		// The update only fires on a different location, so this is a move.
		res.Updated++
		rd.Move = &ArbitratedMove{From: old, To: d.LocationID, ScanPointID: d.ScanPointID, At: at}
	}
	if len(fresh) == 0 {
		return nil
	}

	ids := make([]int, 0, len(fresh))
	for id := range fresh {
		ids = append(ids, id)
	}
	// Strictly before the bucket, so the row just inserted is not its own origin.
	prev, err := s.PreviousAssetLocationsTx(ctx, tx, orgID, ids, bucket)
	if err != nil {
		return err
	}
	for id, f := range fresh {
		p, seen := prev[id]
		if seen && p.LocationID != nil && *p.LocationID == f.to.LocationID {
			continue
		}
		var from *int
		if seen {
			from = p.LocationID
		}
		res.Resolved[f.read].Move = &ArbitratedMove{From: from, To: f.to.LocationID, ScanPointID: f.to.ScanPointID, At: at}
	}
	return nil
}
