package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
)

const notificationDeliveryColumns = `id, org_id, delivery_id, channel, river_job_id, state,
	attempt_count, last_error_kind, provider_message_id, created_at, last_attempted_at, finalized_at`

// InsertNotificationDeliveryTx inserts an audit row inside the caller's
// transaction (TRA-1192). Call this in the same tx as the River job insert
// (see outbox.Enqueue) so a caller rollback removes both together.
func (s *Storage) InsertNotificationDeliveryTx(ctx context.Context, tx pgx.Tx, orgID int, d notificationdelivery.NotificationDelivery) (int64, error) {
	const query = `INSERT INTO trakrf.notification_deliveries
		(org_id, delivery_id, channel, state)
		VALUES ($1, $2, $3, $4)
		RETURNING id`

	var id int64
	err := tx.QueryRow(ctx, query, orgID, d.DeliveryID, string(d.Channel), string(d.State)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert notification delivery: %w", err)
	}
	return id, nil
}

// SetNotificationDeliveryRiverJobIDTx records the River job ID against the
// audit row created in the same transaction, so the worker can look the
// audit row back up by job ID at work time.
func (s *Storage) SetNotificationDeliveryRiverJobIDTx(ctx context.Context, tx pgx.Tx, orgID int, id, riverJobID int64) error {
	const query = `UPDATE trakrf.notification_deliveries SET river_job_id = $1 WHERE id = $2 AND org_id = $3`
	_, err := tx.Exec(ctx, query, riverJobID, id, orgID)
	if err != nil {
		return fmt.Errorf("failed to link river job to notification delivery: %w", err)
	}
	return nil
}

// GetNotificationDeliveryByDeliveryID resolves one audit row by its
// event x recipient x channel identity, scoped to orgID by RLS.
func (s *Storage) GetNotificationDeliveryByDeliveryID(ctx context.Context, orgID int, deliveryID string) (*notificationdelivery.NotificationDelivery, error) {
	const query = `SELECT ` + notificationDeliveryColumns + `
		FROM trakrf.notification_deliveries
		WHERE org_id = $1 AND delivery_id = $2`

	var d notificationdelivery.NotificationDelivery
	var channel, state string
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, deliveryID).Scan(
			&d.ID, &d.OrgID, &d.DeliveryID, &channel, &d.RiverJobID, &state,
			&d.AttemptCount, &d.LastErrorKind, &d.ProviderMessageID,
			&d.CreatedAt, &d.LastAttemptedAt, &d.FinalizedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("notification delivery %q not found", deliveryID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get notification delivery: %w", err)
	}
	d.Channel = notificationdelivery.Channel(channel)
	d.State = notificationdelivery.State(state)
	return &d, nil
}

// GetNotificationDeliveryByRiverJobID resolves the audit row for a River job
// being worked.
func (s *Storage) GetNotificationDeliveryByRiverJobID(ctx context.Context, orgID int, riverJobID int64) (*notificationdelivery.NotificationDelivery, error) {
	const query = `SELECT ` + notificationDeliveryColumns + `
		FROM trakrf.notification_deliveries
		WHERE org_id = $1 AND river_job_id = $2`

	var d notificationdelivery.NotificationDelivery
	var channel, state string
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, riverJobID).Scan(
			&d.ID, &d.OrgID, &d.DeliveryID, &channel, &d.RiverJobID, &state,
			&d.AttemptCount, &d.LastErrorKind, &d.ProviderMessageID,
			&d.CreatedAt, &d.LastAttemptedAt, &d.FinalizedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("notification delivery for river job %d not found", riverJobID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get notification delivery by river job id: %w", err)
	}
	d.Channel = notificationdelivery.Channel(channel)
	d.State = notificationdelivery.State(state)
	return &d, nil
}

// UpdateNotificationDeliveryState records one attempt outcome. State
// transitions to StateDelivered or StatePermanentlyFailed also set
// finalized_at; a transition to StateInFlight does not.
func (s *Storage) UpdateNotificationDeliveryState(ctx context.Context, orgID int, id int64, state notificationdelivery.State, errorKind, providerMessageID *string) error {
	finalize := state == notificationdelivery.StateDelivered || state == notificationdelivery.StatePermanentlyFailed

	query := `UPDATE trakrf.notification_deliveries
		SET state = $1, attempt_count = attempt_count + 1, last_error_kind = $2,
			provider_message_id = COALESCE($3, provider_message_id),
			last_attempted_at = $4`
	args := []interface{}{string(state), errorKind, providerMessageID, time.Now().UTC()}
	if finalize {
		query += `, finalized_at = $5 WHERE id = $6 AND org_id = $7`
		args = append(args, time.Now().UTC(), id, orgID)
	} else {
		query += ` WHERE id = $5 AND org_id = $6`
		args = append(args, id, orgID)
	}

	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("notification delivery %d not found for org %d", id, orgID)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update notification delivery state: %w", err)
	}
	return nil
}
