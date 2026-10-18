package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

// InsertLocationChangeIfAbsentTx records a detected move in the caller's
// transaction and returns its id. A move already recorded for the same asset
// at the same instant is not an error: it returns (0, false, nil), so a
// replayed detection does not record or notify twice.
func (s *Storage) InsertLocationChangeIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int, c notificationevent.LocationChange) (int, bool, error) {
	if c.OrgID != orgID {
		return 0, false, fmt.Errorf("%w: location change for org %d in a transaction for org %d",
			notificationevent.ErrInvalid, c.OrgID, orgID)
	}
	if err := c.Validate(); err != nil {
		return 0, false, err
	}

	const query = `INSERT INTO trakrf.location_changes
		(org_id, asset_id, from_location_id, to_location_id, origin, scan_point_id, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING
		RETURNING id`

	var id int
	err := tx.QueryRow(ctx, query, orgID, c.AssetID, c.FromLocationID, c.ToLocationID,
		string(c.Origin), c.ScanPointID, c.DetectedAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to insert location change: %w", err)
	}
	return id, true, nil
}
