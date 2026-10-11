# Implementation Plan: List an asset's switched-on subscribers
Generated: 2026-10-09
Specification: spec.md · Parent: ../TRA-NaN-notification-service/spec.md

## ELI5
Given an asset, we can now get the list of people who asked to hear about it. Only
subscriptions that are still switched on are listed. For each person we also say whether they
are paused or deleted, so the next step can explain why they were skipped.

## Files
**Create**
- `backend/internal/storage/notification_subscribers_integration_test.go`

**Modify**
- `backend/internal/models/notificationrecipient/notificationrecipient.go`
- `backend/internal/storage/notification_recipients.go`

**Reference patterns**
- backend/internal/storage/notification_recipients.go: `ListAssetNotificationSubscriptions` (empty-slice convention)
- backend/internal/storage/notification_recipients_integration_test.go: fixtures via `db.Store`

## Tasks
### Task 1: `Subscriber` type
```go
// Subscriber is one switched-on subscription joined with its recipient: what
// routing needs to decide eligibility and address a message.
type Subscriber struct {
    SubscriptionID     int
    RecipientID        int
    Channel            string
    Name               string
    Email              *string
    Phone              *string
    RecipientActive    bool
    RecipientDeletedAt *time.Time
}
```
It lives in `models` (not in `routing`) so `storage` never imports `routing`.

### Task 2: `ListSubscribersForAsset`
```sql
SELECT s.id, s.recipient_id, s.channel, r.name, r.email, r.phone, r.is_active, r.deleted_at
  FROM trakrf.asset_notification_recipients s
  JOIN trakrf.notification_recipients r ON r.id = s.recipient_id AND r.org_id = s.org_id
 WHERE s.org_id = $1 AND s.asset_id = $2 AND s.is_active
 ORDER BY s.id
```
Recipient `is_active` / `deleted_at` are returned, not filtered: eligibility reports them.

### Task 3: Integration tests
New file (the recipients test file is already 279 lines). Switch off with
`UpdateAssetNotificationSubscription(IsActive=false)`; pause with `UpdateNotificationRecipient(IsActive=false)`.

## Risks
* `notification_recipients.go` is 300 lines; adding ~40 keeps it under 500.

## Validation gates (every task)
`just bootstrap` once in a fresh worktree (else `go:embed` targets are missing), then after each change:
`just backend lint` → `go build ./...` → `just backend test` for the touched package →
`just backend test-integration <pkg>` where integration tests changed (needs local Postgres; `just database up`).
Any failure: fix and re-run. 3 failed attempts on the same gate → stop and ask.

## Ship
PR title `feat: …` (no ticket id in published prose); body opens with the ELI5 above, then the
Validation Criteria with the exact command output. Base the PR on `main`.
