package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

// DeliveryMessage is the rendered message the planner stores on the delivery
// row (notification_deliveries.payload). It holds the recipient's address and
// the message text, so it is never logged or put in job args.
type DeliveryMessage struct {
	EventID     int                          `json:"event_id"`
	Source      notificationevent.Source     `json:"source"`
	Channel     notificationdelivery.Channel `json:"channel"`
	RecipientID int                          `json:"recipient_id"`
	To          string                       `json:"to"`                // email, or phone in E.164
	Subject     string                       `json:"subject,omitempty"` // email only
	Body        string                       `json:"body"`
	AssetID     int                          `json:"asset_id"`
	OccurredAt  time.Time                    `json:"occurred_at"`
}

// deliveryID is the stable identity of one event x recipient x channel: the
// same trigger planned twice yields the same id, so the outbox can refuse the
// second.
func deliveryID(eventID, recipientID int, ch notificationdelivery.Channel) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%s", eventID, recipientID, ch)))
	return hex.EncodeToString(sum[:])
}
