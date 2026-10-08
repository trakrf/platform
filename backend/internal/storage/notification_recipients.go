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

// DeleteNotificationRecipient soft-deletes the recipient and switches its
// subscriptions off in the same transaction, so none stays live for a deleted
// contact and their history is kept.
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
		_, err = tx.Exec(ctx, `
			UPDATE trakrf.asset_notification_recipients
			   SET is_active = false
			 WHERE recipient_id = $1 AND org_id = $2 AND is_active`, id, orgID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete notification recipient: %w", err)
	}
	return rowsAffected > 0, nil
}

const subscriptionColumns = `id, org_id, asset_id, recipient_id, channel, is_active, created_at, updated_at`

func scanSubscription(row pgx.Row, sub *notificationrecipient.Subscription) error {
	return row.Scan(&sub.ID, &sub.OrgID, &sub.AssetID, &sub.RecipientID, &sub.Channel, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt)
}

// checkSubscriptionTarget is the rule every active subscription must meet: a
// live asset, a live recipient, and the contact detail its channel needs. The
// trigger in migration 000047 enforces the same rules as a backstop.
func checkSubscriptionTarget(ctx context.Context, tx pgx.Tx, orgID, assetID, recipientID int, channel string) error {
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
	return nil
}

func isSubscriptionSentinel(err error) bool {
	return errors.Is(err, ErrSubscriptionAssetNotFound) || errors.Is(err, ErrNotificationRecipientNotFound) || errors.Is(err, ErrChannelContactMismatch)
}

// CreateAssetNotificationSubscription subscribes a recipient to an asset on a
// channel. It is idempotent: if that subscription already exists it is switched
// back on and returned, with created false.
func (s *Storage) CreateAssetNotificationSubscription(ctx context.Context, orgID, assetID, recipientID int, channel string) (*notificationrecipient.Subscription, bool, error) {
	var sub notificationrecipient.Subscription
	created := false
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		if err := checkSubscriptionTarget(ctx, tx, orgID, assetID, recipientID, channel); err != nil {
			return err
		}

		// The WHERE leaves an already-active row untouched, so a repeat call does
		// not bump updated_at; that case returns no row and is read below.
		err := tx.QueryRow(ctx, `
			INSERT INTO trakrf.asset_notification_recipients (org_id, asset_id, recipient_id, channel)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT ON CONSTRAINT asset_notification_recipients_unique
			DO UPDATE SET is_active = true WHERE NOT trakrf.asset_notification_recipients.is_active
			RETURNING `+subscriptionColumns+`, (xmax = 0)`,
			orgID, assetID, recipientID, channel).
			Scan(&sub.ID, &sub.OrgID, &sub.AssetID, &sub.RecipientID, &sub.Channel, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt, &created)
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return scanSubscription(tx.QueryRow(ctx, `
			SELECT `+subscriptionColumns+`
			  FROM trakrf.asset_notification_recipients
			 WHERE org_id = $1 AND asset_id = $2 AND recipient_id = $3 AND channel = $4`,
			orgID, assetID, recipientID, channel), &sub)
	})
	if err != nil {
		if isSubscriptionSentinel(err) {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("failed to create asset notification subscription: %w", err)
	}
	return &sub, created, nil
}

// ListAssetNotificationSubscriptions returns every subscription on the asset,
// switched-off ones included; is_active tells them apart.
func (s *Storage) ListAssetNotificationSubscriptions(ctx context.Context, orgID, assetID int) ([]notificationrecipient.Subscription, error) {
	query := `SELECT ` + subscriptionColumns + `
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
			if err := scanSubscription(rows, &sub); err != nil {
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

// UpdateAssetNotificationSubscription changes the channel or switches the
// subscription on or off, and returns (nil, nil) if it does not exist. A result
// that is active must still meet checkSubscriptionTarget; switching off always
// succeeds. Subscriptions are never deleted.
func (s *Storage) UpdateAssetNotificationSubscription(ctx context.Context, orgID, assetID, id int, req notificationrecipient.UpdateSubscriptionRequest) (*notificationrecipient.Subscription, error) {
	var sub notificationrecipient.Subscription
	found := true
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		err := scanSubscription(tx.QueryRow(ctx, `
			SELECT `+subscriptionColumns+`
			  FROM trakrf.asset_notification_recipients
			 WHERE id = $1 AND org_id = $2 AND asset_id = $3
			   FOR UPDATE`, id, orgID, assetID), &sub)
		if errors.Is(err, pgx.ErrNoRows) {
			found = false
			return nil
		}
		if err != nil {
			return err
		}

		channel, isActive := sub.Channel, sub.IsActive
		if req.Channel != nil {
			channel = *req.Channel
		}
		if req.IsActive != nil {
			isActive = *req.IsActive
		}
		if channel == sub.Channel && isActive == sub.IsActive {
			return nil
		}
		if isActive {
			if err := checkSubscriptionTarget(ctx, tx, orgID, assetID, sub.RecipientID, channel); err != nil {
				return err
			}
		}

		return scanSubscription(tx.QueryRow(ctx, `
			UPDATE trakrf.asset_notification_recipients
			   SET channel = $2, is_active = $3
			 WHERE id = $1
			RETURNING `+subscriptionColumns, id, channel, isActive), &sub)
	})
	if err != nil {
		if isSubscriptionSentinel(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to update asset notification subscription: %w", err)
	}
	if !found {
		return nil, nil
	}
	return &sub, nil
}
