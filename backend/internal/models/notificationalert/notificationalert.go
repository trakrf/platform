// Package notificationalert is the read model for alert history: one sent
// notification to one recipient about one asset event. It is assembled from a
// delivered notification_deliveries row, its notification_events row, and the
// asset, recipient and location names behind them.
package notificationalert

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

// Alert is one sent notification. Type is the event's source (alarm or
// location_change). Subject is nil for SMS. Details carries event specifics:
// an alarm has "location"; a location change has "to" and, when the asset was
// somewhere known before, "from".
type Alert struct {
	ID               int64                        `json:"id"`
	AssetID          int                          `json:"asset_id"`
	AssetName        string                       `json:"asset_name"`
	AssetExternalKey string                       `json:"asset_external_key"`
	Type             notificationevent.Source     `json:"type"`
	OccurredAt       time.Time                    `json:"occurred_at"`
	SentAt           time.Time                    `json:"sent_at"`
	RecipientID      int                          `json:"recipient_id"`
	RecipientName    string                       `json:"recipient_name"`
	Channel          notificationdelivery.Channel `json:"channel"`
	Subject          *string                      `json:"subject"`
	Body             string                       `json:"body"`
	Details          map[string]string            `json:"details"`
}

// ListFilter narrows the alert list. Nil fields do not filter. From and To
// bound SentAt: From inclusive, To exclusive.
type ListFilter struct {
	AssetID     *int
	RecipientID *int
	Type        *notificationevent.Source
	Channel     *notificationdelivery.Channel
	From        *time.Time
	To          *time.Time
	Limit       int
	Offset      int
}
