package assetevent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

// moveStore is the storage surface the move stage needs.
type moveStore interface {
	PreviousAssetLocationsTx(ctx context.Context, tx pgx.Tx, orgID int, assetIDs []int, before time.Time) (map[int]storage.PreviousLocation, error)
}

// changeRecorder records a detected location change and queues its alert.
type changeRecorder interface {
	RecordChange(ctx context.Context, tx pgx.Tx, c notificationevent.LocationChange) (bool, error)
}

// MoveStage records, inside the scan transaction, each stored read that shows
// an asset at a different location than its previous scan. It applies the same
// filters as Evaluator.Evaluate and holds no state.
type MoveStage struct {
	store moveStore
	rec   changeRecorder
	log   zerolog.Logger
}

var _ detection.Stage = (*MoveStage)(nil)

func NewMoveStage(store moveStore, rec changeRecorder) *MoveStage {
	return &MoveStage{store: store, rec: rec, log: log.With().Str("component", "assetevent").Logger()}
}

func (s *MoveStage) Name() string { return "reader_move" }

type moveCandidate struct {
	assetID, scanPointID, to int
}

func (s *MoveStage) Detect(ctx context.Context, tx pgx.Tx, in storage.DetectInput) (detection.Pending, error) {
	var candidates []moveCandidate
	seen := make(map[int]struct{}, len(in.Reads))
	for _, rd := range in.Reads {
		// Zone arbitration decided where this asset is stored and already worked
		// out whether that changed: record its move, never derive one here from
		// the observed location. One component owns each move.
		if rd.Arbitrated {
			if rd.Move != nil {
				if err := s.recordArbitrated(ctx, tx, in.OrgID, rd); err != nil {
					return detection.Pending{}, err
				}
			}
			continue
		}
		if !rd.Stored || rd.LocationID == nil {
			continue
		}
		if _, dup := seen[rd.AssetID]; dup {
			continue
		}
		seen[rd.AssetID] = struct{}{}
		candidates = append(candidates, moveCandidate{assetID: rd.AssetID, scanPointID: rd.ScanPointID, to: *rd.LocationID})
	}
	if len(candidates) == 0 {
		return detection.Pending{}, nil
	}

	assetIDs := make([]int, len(candidates))
	for i, c := range candidates {
		assetIDs[i] = c.assetID
	}
	// Stored rows sit on the truncated minute and the lookup is strictly
	// before it, so the row just stored cannot be its own predecessor.
	at := in.ReceivedAt.Truncate(storage.ScanGranularity)
	start := time.Now()
	prev, err := s.store.PreviousAssetLocationsTx(ctx, tx, in.OrgID, assetIDs, at)
	metricLookupSeconds.Observe(time.Since(start).Seconds())
	if err != nil {
		metricLookupErrors.Inc()
		s.log.Error().Err(err).Int("org_id", in.OrgID).Msg("previous-location lookup failed; no reader moves recorded for this message")
		return detection.Pending{}, fmt.Errorf("previous location lookup failed: %w", err)
	}

	for _, c := range candidates {
		p, ok := prev[c.assetID]
		if ok && p.LocationID != nil && *p.LocationID == c.to {
			continue
		}
		var from *int
		if ok {
			from = p.LocationID
		}
		scanPoint := c.scanPointID
		_, err := s.rec.RecordChange(ctx, tx, notificationevent.LocationChange{
			OrgID:          in.OrgID,
			AssetID:        c.assetID,
			FromLocationID: from,
			ToLocationID:   c.to,
			Origin:         notificationevent.OriginReader,
			ScanPointID:    &scanPoint,
			DetectedAt:     at,
		})
		if err != nil {
			s.log.Error().Err(err).Int("org_id", in.OrgID).Int("asset_id", c.assetID).Msg("recording reader move failed")
			return detection.Pending{}, fmt.Errorf("failed to record move for asset %d: %w", c.assetID, err)
		}
	}
	return detection.Pending{}, nil
}

// recordArbitrated records an arbitrated move at the instant it happened, not
// the minute floor, with the origin arbitration found.
func (s *MoveStage) recordArbitrated(ctx context.Context, tx pgx.Tx, orgID int, rd storage.ResolvedRead) error {
	mv := rd.Move
	scanPoint := mv.ScanPointID
	_, err := s.rec.RecordChange(ctx, tx, notificationevent.LocationChange{
		OrgID:          orgID,
		AssetID:        rd.AssetID,
		FromLocationID: mv.From,
		ToLocationID:   mv.To,
		Origin:         notificationevent.OriginReader,
		ScanPointID:    &scanPoint,
		DetectedAt:     mv.At,
	})
	if err != nil {
		s.log.Error().Err(err).Int("org_id", orgID).Int("asset_id", rd.AssetID).Msg("recording arbitrated move failed")
		return fmt.Errorf("failed to record arbitrated move for asset %d: %w", rd.AssetID, err)
	}
	return nil
}
