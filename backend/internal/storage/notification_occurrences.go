package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

// GetAlarmOccurrence reads the asset, location and time of alarm id. found is
// false, with no error, when the alarm does not exist or belongs to another
// org.
func (s *Storage) GetAlarmOccurrence(ctx context.Context, orgID, id int) (notificationevent.Occurrence, bool, error) {
	const query = `
		SELECT asset_id, location_id, fired_at
		FROM trakrf.alarm_events
		WHERE org_id = $1 AND id = $2`
	return s.getOccurrence(ctx, orgID, id, notificationevent.SourceAlarm, query)
}

// GetLocationChangeOccurrence reads the asset, destination and time of
// location change id. found is false, with no error, when the move does not
// exist or belongs to another org.
func (s *Storage) GetLocationChangeOccurrence(ctx context.Context, orgID, id int) (notificationevent.Occurrence, bool, error) {
	const query = `
		SELECT asset_id, to_location_id, detected_at
		FROM trakrf.location_changes
		WHERE org_id = $1 AND id = $2`
	return s.getOccurrence(ctx, orgID, id, notificationevent.SourceLocationChange, query)
}

// getOccurrence runs a query that takes (org_id, id) and selects asset_id, a
// nullable location id and the time.
func (s *Storage) getOccurrence(ctx context.Context, orgID, id int, source notificationevent.Source, query string) (notificationevent.Occurrence, bool, error) {
	occ := notificationevent.Occurrence{Source: source, SourceID: id}
	var occurredAt time.Time
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&occ.AssetID, &occ.LocationID, &occurredAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return notificationevent.Occurrence{}, false, nil
	}
	if err != nil {
		return notificationevent.Occurrence{}, false, fmt.Errorf("failed to get %s occurrence %d: %w", source, id, err)
	}
	occ.OccurredAt = occurredAt
	return occ, true, nil
}
