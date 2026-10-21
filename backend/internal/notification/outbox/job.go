package outbox

// DeliveryJobArgs is what actually goes into river_job.args (as JSON). It
// intentionally carries only the audit row's ID, never Command.Payload: the
// payload is persisted on the delivery row (notification_deliveries.payload)
// and is read from there, not from the job. This keeps River's job table free of
// PII and keeps it agnostic to email vs. SMS shapes.
type DeliveryJobArgs struct {
	OrgID                  int   `json:"org_id"`
	NotificationDeliveryID int64 `json:"notification_delivery_id"`
}

func (DeliveryJobArgs) Kind() string { return "notification_delivery" }
