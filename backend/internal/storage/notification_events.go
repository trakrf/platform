package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

// InsertNotificationEventIfAbsentTx records a notification event in the
// caller's transaction and returns its id. An event already recorded for the
// same trigger is not an error: it returns (0, false, nil). An event whose
// asset or time differs from its trigger's is rejected by the foreign key and
// returned as an error.
func (s *Storage) InsertNotificationEventIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int, e notificationevent.NotificationEvent) (int, bool, error) {
	if e.OrgID != orgID {
		return 0, false, fmt.Errorf("%w: notification event for org %d in a transaction for org %d",
			notificationevent.ErrInvalid, e.OrgID, orgID)
	}
	if err := e.Validate(); err != nil {
		return 0, false, err
	}

	const query = `INSERT INTO trakrf.notification_events
		(org_id, source, asset_id, occurred_at, alarm_event_id, location_change_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
		RETURNING id`

	var id int
	err := tx.QueryRow(ctx, query, orgID, string(e.Source), e.AssetID, e.OccurredAt,
		e.AlarmEventID, e.LocationChangeID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to insert notification event: %w", err)
	}
	return id, true, nil
}
