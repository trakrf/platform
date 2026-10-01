package outbox

// DeliveryJobArgs is what actually goes into river_job.args (as JSON). It
// intentionally carries only the audit row's ID, never the payload — the
// payload lives in notification_deliveries (or, once a real ChannelAdapter
// exists, wherever it resolves recipient/content from). This keeps River's
// job table free of PII and keeps it agnostic to email vs. SMS shapes.
type DeliveryJobArgs struct {
	OrgID                  int   `json:"org_id"`
	NotificationDeliveryID int64 `json:"notification_delivery_id"`
}

func (DeliveryJobArgs) Kind() string { return "notification_delivery" }
