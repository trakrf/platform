package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
)

// EntitlementChecker is the narrow surface Enqueue needs to gate delivery.
// *storage.Storage already satisfies it via OrgIsEntitled
// (backend/internal/storage/organizations.go), the same
// trakrf.org_is_entitled() check backend/internal/storage/webhooks.go
// joins into its own query — no new storage method needed.
type EntitlementChecker interface {
	OrgIsEntitled(ctx context.Context, orgID int) (bool, error)
}

// deliveryStore is the storage surface Enqueue needs, narrowed for testing.
// *storage.Storage satisfies this structurally — this file does not import
// the storage package directly.
type deliveryStore interface {
	WithOrgTx(ctx context.Context, orgID int, fn func(tx pgx.Tx) error) error
	InsertNotificationDeliveryTx(ctx context.Context, tx pgx.Tx, orgID int, d notificationdelivery.NotificationDelivery) (int64, error)
	SetNotificationDeliveryRiverJobIDTx(ctx context.Context, tx pgx.Tx, orgID int, id, riverJobID int64) error
}

// CheckEntitlement reports whether OrgID's enqueue should be skipped. An
// unentitled org is skipped (never buffered for replay on reinstatement,
// matching the existing webhook entitlement rule); a lookup failure is
// always an error, never silently treated as unentitled.
func CheckEntitlement(ctx context.Context, checker EntitlementChecker, orgID int) (skip bool, err error) {
	entitled, err := checker.OrgIsEntitled(ctx, orgID)
	if err != nil {
		return false, fmt.Errorf("failed to check entitlement: %w", err)
	}
	return !entitled, nil
}

// deliveryMaxAttempts bounds every delivery job: the TRA-398 ladder has 5
// rungs; original attempt + 5 retries = 6.
const deliveryMaxAttempts = 6

// Enqueue durably records cmd as a pending delivery, atomically with the
// audit row, inside one DB transaction. A rolled-back caller transaction
// (by using the same tx passed here) leaves neither row behind.
//
// Callers unentitled at enqueue time are skipped and counted, never
// buffered — reinstating a subscription must not dump a flood of stale
// notifications, the same reasoning as the existing webhook entitlement
// gate (backend/internal/webhook/sink.go).
func Enqueue(ctx context.Context, store deliveryStore, entitlement EntitlementChecker, riverClient *river.Client[pgx.Tx], metrics *Metrics, cmd Command) error {
	skip, err := CheckEntitlement(ctx, entitlement, cmd.OrgID)
	if err != nil {
		return err
	}
	if skip {
		metrics.RecordEnqueue(EnqueueSkippedUnentitled)
		return nil
	}

	return store.WithOrgTx(ctx, cmd.OrgID, func(tx pgx.Tx) error {
		deliveryDBID, err := store.InsertNotificationDeliveryTx(ctx, tx, cmd.OrgID, notificationdelivery.NotificationDelivery{
			DeliveryID: cmd.DeliveryID,
			Channel:    cmd.Channel,
			State:      notificationdelivery.StatePending,
		})
		if err != nil {
			return err
		}

		args := DeliveryJobArgs{OrgID: cmd.OrgID, NotificationDeliveryID: deliveryDBID}
		result, err := riverClient.InsertTx(ctx, tx, args, &river.InsertOpts{
			MaxAttempts: deliveryMaxAttempts,
		})
		if err != nil {
			return fmt.Errorf("failed to enqueue river job: %w", err)
		}

		if err := store.SetNotificationDeliveryRiverJobIDTx(ctx, tx, cmd.OrgID, deliveryDBID, result.Job.ID); err != nil {
			return err
		}

		metrics.RecordEnqueue(EnqueueOK)
		return nil
	})
}
