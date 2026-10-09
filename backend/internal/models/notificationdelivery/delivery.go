// Package notificationdelivery defines the durable audit record for one
// notification delivery attempt lineage (TRA-1192). It is intentionally
// separate from River's own job bookkeeping: this table is the 3-month
// operator/audit surface, and rows here are never touched by River's job
// cleanup.
package notificationdelivery

import "time"

type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

type State string

const (
	StatePending           State = "pending"
	StateInFlight          State = "in_flight"
	StateDelivered         State = "delivered"
	StatePermanentlyFailed State = "permanently_failed"
)

// NotificationDelivery is a notification_deliveries row.
type NotificationDelivery struct {
	ID                int64
	OrgID             int
	DeliveryID        string
	Channel           Channel
	RiverJobID        *int64
	State             State
	AttemptCount      int
	LastErrorKind     *string
	ProviderMessageID *string
	CreatedAt         time.Time
	LastAttemptedAt   *time.Time
	FinalizedAt       *time.Time
	// Routing context; nil on rows written before it was recorded.
	EventID     *string
	RecipientID *int
	AssetID     *int
	// Payload is the rendered message as raw JSON; nil means NULL. It holds
	// the recipient's address and message text, so never log it.
	Payload []byte
}
