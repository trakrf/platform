package routing

// Outcome is how one notification attempt ended: what the planner records
// against a trigger or a subscriber.
type Outcome string

const (
	OutcomeEnqueued             Outcome = "enqueued"
	OutcomeDuplicate            Outcome = "duplicate"
	OutcomeSkippedUnentitled    Outcome = "skipped_unentitled"
	OutcomeSkippedNoSubscribers Outcome = "skipped_no_subscribers"
	OutcomeSkippedAssetDeleted  Outcome = "skipped_asset_deleted"
	OutcomeSkippedInactive      Outcome = "skipped_inactive"
	OutcomeSkippedOptedOut      Outcome = "skipped_opted_out"
	OutcomeSkippedNoAddress     Outcome = "skipped_no_address"
	OutcomeFailed               Outcome = "failed"
	OutcomeInvalidTrigger       Outcome = "invalid_trigger"
)
