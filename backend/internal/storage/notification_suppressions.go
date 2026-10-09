package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
)

// IsSuppressed reports whether address has an uncleared opt-out on channel.
// Case-insensitive, matching the unique index. A lookup error is returned and
// never read as "not suppressed": nothing may be sent while opt-out state is
// unknown. The address is PII and stays out of the error text.
func (s *Storage) IsSuppressed(ctx context.Context, orgID int, ch notificationdelivery.Channel, address string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM trakrf.notification_suppressions
			WHERE org_id = $1 AND channel = $2 AND lower(address) = lower($3) AND cleared_at IS NULL
		)`
	var suppressed bool
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, string(ch), address).Scan(&suppressed)
	})
	if err != nil {
		return false, fmt.Errorf("failed to check notification suppression (channel %s): %w", ch, err)
	}
	return suppressed, nil
}
