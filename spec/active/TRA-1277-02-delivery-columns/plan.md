# Implementation Plan: Persist routing context on delivery rows
Generated: 2026-10-09
Specification: spec.md · Parent: ../TRA-NaN-notification-service/spec.md

## ELI5
The new boxes from MR 01 can now be filled in. When we save a "message to send", we also
save who it's for, which asset and event it's about, and the message text. If the same message
is saved twice, the database politely says "I already have that one" instead of making a copy
or crashing.

## Files
**Create**
- none

**Modify**
- `backend/internal/models/notificationdelivery/delivery.go`
- `backend/internal/notification/outbox/contracts.go`
- `backend/internal/storage/notification_deliveries.go`
- `backend/internal/storage/notification_deliveries_integration_test.go`

**Reference patterns**
- backend/internal/storage/notification_deliveries.go: column list + scan order shared by two getters
- backend/internal/storage/notification_recipients.go: `scanNotificationRecipient` helper pattern

## Tasks
### Task 1: Model and `Command` fields
```go
// notificationdelivery.NotificationDelivery — append:
EventID     *string
RecipientID *int
AssetID     *int
Payload     []byte // raw JSON; nil means NULL

// outbox.Command — add (zero value is stored as NULL):
EventID     string
RecipientID int
AssetID     int
```
Rewrite the `Command.Payload` comment: it is now persisted to `notification_deliveries.payload`
by the batch path, and must never go into River job args.

### Task 2: `InsertNotificationDeliveryIfAbsentTx`
```go
// inserted=false means the delivery was enqueued earlier: do not insert a job.
func (s *Storage) InsertNotificationDeliveryIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int,
    d notificationdelivery.NotificationDelivery) (id int64, inserted bool, err error) {
    const q = `INSERT INTO trakrf.notification_deliveries
        (org_id, delivery_id, channel, state, event_id, recipient_id, asset_id, payload)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ON CONFLICT (org_id, delivery_id) DO NOTHING
        RETURNING id`
    // errors.Is(err, pgx.ErrNoRows) → return 0, false, nil
}
```
Pass `nil` when `len(d.Payload) == 0`. Keep `InsertNotificationDeliveryTx` as is (used by `Enqueue`).

### Task 3: Read the new columns
Extend `notificationDeliveryColumns` with `event_id, recipient_id, asset_id, payload`; add a
`scanNotificationDelivery(row pgx.Row, d *NotificationDelivery)` helper and use it in both
`GetNotificationDeliveryByDeliveryID` and `GetNotificationDeliveryByRiverJobID`.

### Task 4: Integration tests
Extend `notification_deliveries_integration_test.go` (seed a real recipient + asset for the FKs).

## Risks
* pgx v5 should send `[]byte` to a `jsonb` parameter as raw JSON. If the round-trip fails, switch to `json.RawMessage` or `$8::jsonb`.
* The worker reads rows by River job ID; it only gains fields. Run `go test ./internal/notification/outbox/` to confirm.

## Validation gates (every task)
`just bootstrap` once in a fresh worktree (else `go:embed` targets are missing), then after each change:
`just backend lint` → `go build ./...` → `just backend test` for the touched package →
`just backend test-integration <pkg>` where integration tests changed (needs local Postgres; `just database up`).
Any failure: fix and re-run. 3 failed attempts on the same gate → stop and ask.

## Ship
PR title `feat: …` (no ticket id in published prose); body opens with the ELI5 above, then the
Validation Criteria with the exact command output. Base the PR on `feat/tra-1277-01-delivery-schema`.
