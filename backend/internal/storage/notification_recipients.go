package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
)

var (
	ErrNotificationRecipientNotFound = errors.New("notification recipient not found")
	ErrSubscriptionAssetNotFound     = errors.New("asset not found")
	ErrChannelContactMismatch        = errors.New("channel requires a matching contact on the recipient")
)

const notificationRecipientColumns = `id, org_id, name, email, phone, is_active, created_at, updated_at, deleted_at`

func scanNotificationRecipient(row pgx.Row, r *notificationrecipient.Recipient) error {
	return row.Scan(&r.ID, &r.OrgID, &r.Name, &r.Email, &r.Phone, &r.IsActive, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt)
}

func (s *Storage) CreateNotificationRecipient(ctx context.Context, orgID int, req notificationrecipient.CreateRecipientRequest) (*notificationrecipient.Recipient, error) {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	query := `
		INSERT INTO trakrf.notification_recipients (org_id, name, email, phone, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + notificationRecipientColumns

	var r notificationrecipient.Recipient
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanNotificationRecipient(tx.QueryRow(ctx, query, orgID, req.Name, req.Email, req.Phone, isActive), &r)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create notification recipient: %w", err)
	}
	return &r, nil
}

func (s *Storage) ListNotificationRecipients(ctx context.Context, orgID int) ([]notificationrecipient.Recipient, error) {
	query := `SELECT ` + notificationRecipientColumns + `
		FROM trakrf.notification_recipients
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`
	out := []notificationrecipient.Recipient{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r notificationrecipient.Recipient
			if err := scanNotificationRecipient(rows, &r); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list notification recipients: %w", err)
	}
	return out, nil
}

// GetNotificationRecipient returns the live recipient or (nil, nil) if not found.
func (s *Storage) GetNotificationRecipient(ctx context.Context, orgID, id int) (*notificationrecipient.Recipient, error) {
	query := `SELECT ` + notificationRecipientColumns + `
		FROM trakrf.notification_recipients
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`
	var r notificationrecipient.Recipient
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanNotificationRecipient(tx.QueryRow(ctx, query, id, orgID), &r)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification recipient: %w", err)
	}
	return &r, nil
}

// UpdateNotificationRecipient applies only the fields that are set. Contacts
// cannot be cleared here, so the contact-present check always holds.
func (s *Storage) UpdateNotificationRecipient(ctx context.Context, orgID, id int, req notificationrecipient.UpdateRecipientRequest) (*notificationrecipient.Recipient, error) {
	query := `
		UPDATE trakrf.notification_recipients
		   SET name      = COALESCE($3, name),
		       email     = COALESCE($4, email),
		       phone     = COALESCE($5, phone),
		       is_active = COALESCE($6, is_active)
		 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING ` + notificationRecipientColumns

	var r notificationrecipient.Recipient
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return scanNotificationRecipient(tx.QueryRow(ctx, query, id, orgID, req.Name, req.Email, req.Phone, req.IsActive), &r)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update notification recipient: %w", err)
	}
	return &r, nil
}

// DeleteNotificationRecipient soft-deletes the recipient and removes its
// subscriptions in the same transaction, so no subscription points at a
// deleted contact.
func (s *Storage) DeleteNotificationRecipient(ctx context.Context, orgID, id int) (bool, error) {
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE trakrf.notification_recipients
			   SET deleted_at = NOW()
			 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		if rowsAffected == 0 {
			return nil
		}
		_, err = tx.Exec(ctx, `DELETE FROM trakrf.asset_notification_recipients WHERE recipient_id = $1 AND org_id = $2`, id, orgID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete notification recipient: %w", err)
	}
	return rowsAffected > 0, nil
}

// CreateAssetNotificationSubscription checks the asset and recipient exist and
// that the recipient has the contact the channel needs. The trigger in
// migration 000045 enforces the same rules as a backstop.
func (s *Storage) CreateAssetNotificationSubscription(ctx context.Context, orgID, assetID, recipientID int, channel string) (*notificationrecipient.Subscription, error) {
	var sub notificationrecipient.Subscription
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trakrf.assets WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL)`, assetID, orgID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrSubscriptionAssetNotFound
		}

		var email, phone *string
		err := tx.QueryRow(ctx, `SELECT email, phone FROM trakrf.notification_recipients WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, recipientID, orgID).Scan(&email, &phone)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotificationRecipientNotFound
		}
		if err != nil {
			return err
		}
		if (channel == notificationrecipient.ChannelEmail && email == nil) || (channel == notificationrecipient.ChannelSMS && phone == nil) {
			return ErrChannelContactMismatch
		}

		return tx.QueryRow(ctx, `
			INSERT INTO trakrf.asset_notification_recipients (org_id, asset_id, recipient_id, channel)
			VALUES ($1, $2, $3, $4)
			RETURNING id, org_id, asset_id, recipient_id, channel, created_at, updated_at`,
			orgID, assetID, recipientID, channel).
			Scan(&sub.ID, &sub.OrgID, &sub.AssetID, &sub.RecipientID, &sub.Channel, &sub.CreatedAt, &sub.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, ErrSubscriptionAssetNotFound) || errors.Is(err, ErrNotificationRecipientNotFound) || errors.Is(err, ErrChannelContactMismatch) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to create asset notification subscription: %w", err)
	}
	return &sub, nil
}

func (s *Storage) ListAssetNotificationSubscriptions(ctx context.Context, orgID, assetID int) ([]notificationrecipient.Subscription, error) {
	query := `SELECT id, org_id, asset_id, recipient_id, channel, created_at, updated_at
		FROM trakrf.asset_notification_recipients
		WHERE org_id = $1 AND asset_id = $2
		ORDER BY created_at, id`
	out := []notificationrecipient.Subscription{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, assetID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sub notificationrecipient.Subscription
			if err := rows.Scan(&sub.ID, &sub.OrgID, &sub.AssetID, &sub.RecipientID, &sub.Channel, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
				return err
			}
			out = append(out, sub)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list asset notification subscriptions: %w", err)
	}
	return out, nil
}

func (s *Storage) DeleteAssetNotificationSubscription(ctx context.Context, orgID, assetID, id int) (bool, error) {
	var rowsAffected int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			DELETE FROM trakrf.asset_notification_recipients
			 WHERE id = $1 AND org_id = $2 AND asset_id = $3`, id, orgID, assetID)
		if err != nil {
			return err
		}
		rowsAffected = result.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete asset notification subscription: %w", err)
	}
	return rowsAffected > 0, nil
}
